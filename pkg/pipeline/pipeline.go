// Package pipeline orchestrates the product build pipeline.
package pipeline

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/loop"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/spawn"
	"github.com/lovyou-ai/hive/pkg/workspace"
)

// Phase represents a stage in the product pipeline.
type Phase string

const (
	PhaseResearch  Phase = "research"
	PhaseDesign    Phase = "design"
	PhaseBuild     Phase = "build"
	PhaseReview    Phase = "review"
	PhaseTest      Phase = "test"
	PhaseIntegrate Phase = "integrate"
)

// Action constants for pipeline events — no magic strings.
const (
	ActionWriteCode    = "write_code"
	ActionSeedBuild    = "seed_product_build"
	ActionIntegrate    = "integrate_staging"
)

// writeCodeAction returns the action string for a code generation event.
// Schema: "write_code:<language>" — made explicit to avoid magic string composition.
func writeCodeAction(lang string) string {
	return ActionWriteCode + ":" + lang
}

// ProductInput describes how a product idea enters the hive.
type ProductInput struct {
	Name        string // Product name (used for repo and directory). If empty, CTO derives one.
	URL         string // Read from URL (Substack post, docs, etc.)
	Description string // Natural language description
	SpecFile    string // Path to a Code Graph spec file
}

// Pipeline orchestrates agents through the product build phases.
type Pipeline struct {
	store     store.Store
	actors    actor.IActorStore
	humanID   types.ActorID
	humanName string
	ws        *workspace.Workspace
	product   *workspace.Product // current product being built
	spawner   *spawn.Spawner     // nil = direct creation (no approval)

	cto      *roles.Agent
	guardian *roles.Agent
	agents   map[roles.Role]*roles.Agent
}

// Config for creating a new pipeline.
type Config struct {
	Store   store.Store
	Actors  actor.IActorStore          // actor registry — humans via auth, agents via creation
	Trust   *trust.DefaultTrustModel   // trust model for gate enforcement
	HumanID types.ActorID              // pre-registered human operator (from auth/actor store)
	WorkDir string                     // Root directory for generated products
	Gate    *authority.Gate             // optional authority gate (nil = no approval required)
}


// New creates a pipeline and bootstraps the CTO and Guardian.
// The human operator must already be registered in the actor store (via auth).
func New(ctx context.Context, cfg Config) (*Pipeline, error) {
	// Verify the human exists in the actor store
	human, err := cfg.Actors.Get(cfg.HumanID)
	if err != nil {
		return nil, fmt.Errorf("human operator not found in actor store: %w", err)
	}
	fmt.Printf("Human operator: %s (%s)\n", human.DisplayName(), human.ID().Value())

	ws, err := workspace.New(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}

	p := &Pipeline{
		store:     cfg.Store,
		actors:    cfg.Actors,
		humanID:   cfg.HumanID,
		humanName: human.DisplayName(),
		ws:        ws,
		agents:    make(map[roles.Role]*roles.Agent),
	}

	// Wire up spawner if an authority gate is provided.
	if cfg.Gate != nil {
		// Deterministic signer derived from humanID — stable across restarts,
		// verifiable against the human's identity. Random ephemeral keys would
		// break INTEGRITY (signature verification requires knowing the key).
		signer := deriveSignerFromID(cfg.HumanID)
		registry := event.DefaultRegistry()
		factory := event.NewEventFactory(registry)
		convID, err := newConversationID()
		if err != nil {
			return nil, fmt.Errorf("spawn conversation ID: %w", err)
		}

		p.spawner = spawn.NewSpawner(spawn.Config{
			Store:   cfg.Store,
			Actors:  cfg.Actors,
			Trust:   cfg.Trust,
			Gate:    cfg.Gate,
			HumanID: cfg.HumanID,
			Signer:  signer,
			Factory: factory,
			ConvID:  convID,
		})
	}

	// Bootstrap CTO first — architectural oversight (Opus)
	cto, err := p.ensureAgent(ctx, roles.RoleCTO, "cto")
	if err != nil {
		return nil, fmt.Errorf("bootstrap CTO: %w", err)
	}
	p.cto = cto

	// Bootstrap Guardian — independent integrity monitor (Opus)
	guardian, err := p.ensureAgent(ctx, roles.RoleGuardian, "guardian")
	if err != nil {
		return nil, fmt.Errorf("bootstrap Guardian: %w", err)
	}
	p.guardian = guardian

	return p, nil
}

// providerForRole creates an intelligence provider with the model and system prompt
// appropriate for the role. Uses Claude CLI (flat rate via Max plan).
func (p *Pipeline) providerForRole(role roles.Role) (intelligence.Provider, error) {
	model := roles.PreferredModel(role)
	return intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        model,
		SystemPrompt: roles.SystemPrompt(role, p.humanName),
	})
}

// ensureAgent creates an agent of the given role if it doesn't exist yet.
// When a spawner is configured, spawn requests go through the authority gate
// (human approval). Without a spawner, agents are created directly.
func (p *Pipeline) ensureAgent(ctx context.Context, role roles.Role, name string) (*roles.Agent, error) {
	if agent, ok := p.agents[role]; ok {
		return agent, nil
	}

	var actorID types.ActorID

	if p.spawner != nil {
		// Spawn through authority gate — human must approve.
		result, err := p.spawner.Spawn(ctx, spawn.SpawnRequest{
			Role:          role,
			Name:          name,
			Justification: fmt.Sprintf("pipeline needs %s agent for product build", role),
			RequestedBy:   p.humanID,
		})
		if err != nil {
			return nil, fmt.Errorf("spawn %s: %w", name, err)
		}
		if !result.Approved {
			return nil, fmt.Errorf("spawn %s denied: %s", name, result.Reason)
		}
		actorID = result.ActorID
	} else {
		// Direct creation — no approval gate (bootstrap or testing).
		agentPub := spawn.DerivePublicKey("agent:" + name)
		agentPK, err := types.NewPublicKey([]byte(agentPub))
		if err != nil {
			return nil, fmt.Errorf("agent public key: %w", err)
		}
		agentActor, err := p.actors.Register(agentPK, name, event.ActorTypeAI)
		if err != nil {
			return nil, fmt.Errorf("create agent %s: %w", name, err)
		}
		actorID = agentActor.ID()
	}

	provider, err := p.providerForRole(role)
	if err != nil {
		return nil, fmt.Errorf("provider for %s: %w", role, err)
	}
	agent, err := roles.NewAgent(ctx, roles.AgentConfig{
		Role:     role,
		Name:     name,
		ActorID:  actorID,
		Store:    p.store,
		Provider: provider,
		HumanID:  p.humanID,
	})
	if err != nil {
		return nil, err
	}
	fmt.Printf("  ↳ %s agent %s using %s\n", role, actorID.Value(), roles.PreferredModel(role))
	p.agents[role] = agent
	return agent, nil
}

// ed25519Signer implements event.Signer for pipeline-emitted events.
type ed25519Signer struct {
	key ed25519.PrivateKey
}

func (s *ed25519Signer) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

// deriveSignerFromID creates a deterministic Ed25519 signer from an ActorID.
// Stable across restarts — the same humanID always produces the same key.
func deriveSignerFromID(id types.ActorID) *ed25519Signer {
	h := sha256.Sum256([]byte("signer:" + id.Value()))
	priv := ed25519.NewKeyFromSeed(h[:])
	return &ed25519Signer{key: priv}
}

// newConversationID generates a unique conversation ID for this pipeline run.
func newConversationID() (types.ConversationID, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return types.ConversationID{}, err
	}
	return types.NewConversationID("conv_spawn_" + hex.EncodeToString(b[:]))
}

// Run executes the full product pipeline for a given input.
func (p *Pipeline) Run(ctx context.Context, input ProductInput) error {
	// ── Phase 1: Research ──
	fmt.Println("═══ Phase 1: Research ═══")
	spec, err := p.research(ctx, input)
	if err != nil {
		return fmt.Errorf("research: %w", err)
	}
	if halt := p.guardianCheck(ctx, "research"); halt {
		return fmt.Errorf("guardian halted pipeline after research phase")
	}

	// Derive product name if not provided
	name := input.Name
	if name == "" {
		name, err = p.deriveName(ctx, spec)
		if err != nil {
			return fmt.Errorf("derive name: %w", err)
		}
	}

	// Initialize product repo
	product, err := p.ws.InitProduct(name)
	if err != nil {
		return fmt.Errorf("init product: %w", err)
	}
	p.product = product
	fmt.Printf("Product repo: %s → %s\n", product.Dir, product.Repo)

	// ── Phase 2: Design ──
	fmt.Println("═══ Phase 2: Design ═══")
	design, err := p.design(ctx, spec)
	if err != nil {
		return fmt.Errorf("design: %w", err)
	}
	if halt := p.guardianCheck(ctx, "design"); halt {
		return fmt.Errorf("guardian halted pipeline after design phase")
	}

	// ── Phase 2b: Simplify ──
	fmt.Println("═══ Phase 2b: Simplify ═══")
	design, err = p.simplify(ctx, design)
	if err != nil {
		return fmt.Errorf("simplify: %w", err)
	}

	// Save the final spec to the product repo
	if err := p.product.WriteFile("SPEC.md", design); err != nil {
		return fmt.Errorf("save spec: %w", err)
	}
	if err := p.product.Commit("docs: Code Graph specification"); err != nil {
		return fmt.Errorf("commit spec: %w", err)
	}
	fmt.Println("Spec committed to product repo.")

	// Extract language from the design
	lang := p.extractLanguage(design)
	fmt.Printf("Target language: %s\n", lang)

	// ── Phase 3: Build ──
	fmt.Println("═══ Phase 3: Build ═══")
	files, err := p.build(ctx, design, lang)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	if halt := p.guardianCheck(ctx, "build"); halt {
		return fmt.Errorf("guardian halted pipeline after build phase")
	}

	// ── Phase 4: Review → Rebuild loop ──
	const maxReviewRounds = 3
	for round := 1; round <= maxReviewRounds; round++ {
		fmt.Printf("═══ Phase 4: Review (round %d) ═══\n", round)
		feedback, approved, err := p.review(ctx, files, design, lang)
		if err != nil {
			return fmt.Errorf("review round %d: %w", round, err)
		}
		if halt := p.guardianCheck(ctx, "review"); halt {
			return fmt.Errorf("guardian halted pipeline after review phase")
		}

		if approved {
			fmt.Println("Code approved by reviewer.")
			break
		}

		if round == maxReviewRounds {
			fmt.Println("Max review rounds reached — proceeding with current code.")
			break
		}

		// Rebuild with reviewer feedback
		fmt.Printf("═══ Phase 4b: Rebuild from feedback (round %d) ═══\n", round)
		files, err = p.rebuild(ctx, files, feedback, design, lang)
		if err != nil {
			return fmt.Errorf("rebuild round %d: %w", round, err)
		}
	}

	// ── Phase 5: Test ──
	fmt.Println("═══ Phase 5: Test ═══")
	err = p.test(ctx, files, lang)
	if err != nil {
		return fmt.Errorf("test: %w", err)
	}
	if halt := p.guardianCheck(ctx, "test"); halt {
		return fmt.Errorf("guardian halted pipeline after test phase")
	}

	// ── Phase 6: Integrate ──
	fmt.Println("═══ Phase 6: Integrate ═══")
	err = p.integrate(ctx)
	if err != nil {
		return fmt.Errorf("integrate: %w", err)
	}
	if halt := p.guardianCheck(ctx, "integrate"); halt {
		return fmt.Errorf("guardian halted pipeline after integrate phase")
	}

	fmt.Println("═══ Pipeline Complete ═══")
	return nil
}

// LoopConfig configures the agentic loop mode of the pipeline.
type LoopConfig struct {
	// Budget limits for each execution agent loop.
	Budget resources.BudgetConfig

	// GuardianBudget limits for the Guardian's loop. The Guardian watches
	// everything and must outlive execution agents — defaults to 10x the
	// execution budget iterations and duration.
	GuardianBudget resources.BudgetConfig

	// OnIteration is called when any agent completes a loop iteration.
	OnIteration func(role roles.Role, iteration int, response string)
}

// DefaultLoopConfig returns sensible defaults for loop mode.
func DefaultLoopConfig() LoopConfig {
	return LoopConfig{
		Budget: resources.BudgetConfig{
			MaxIterations: 20,
			MaxCostUSD:    10.0,
		},
		GuardianBudget: resources.BudgetConfig{
			MaxIterations: 200, // 10x execution agents — Guardian outlives them
			MaxCostUSD:    20.0,
		},
	}
}

// RunLoop executes the pipeline in graph-driven agentic loop mode.
//
// Instead of a fixed phase sequence, the CTO seeds the work and agents
// self-direct by observing graph events. The Guardian runs its own loop
// watching everything. Agents communicate through events, not orchestration.
func (p *Pipeline) RunLoop(ctx context.Context, input ProductInput, cfg LoopConfig) ([]loop.AgentResult, error) {
	// Create event bus for real-time notification between agents.
	eventBus := bus.NewEventBus(p.store, 256)
	defer eventBus.Close()

	// Seed: CTO evaluates the idea and emits initial direction.
	fmt.Println("═══ Seeding: CTO evaluates idea ═══")
	seedTask, err := p.seedWork(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("seed work: %w", err)
	}

	// Bootstrap agents for the product pipeline.
	agentConfigs, err := p.buildLoopConfigs(ctx, seedTask, eventBus, cfg)
	if err != nil {
		return nil, fmt.Errorf("build loop configs: %w", err)
	}

	// Run all agent loops concurrently.
	fmt.Printf("═══ Starting %d agent loops ═══\n", len(agentConfigs))
	for _, c := range agentConfigs {
		fmt.Printf("  ↳ %s loop starting\n", c.Agent.Role)
	}

	results := loop.RunConcurrent(ctx, agentConfigs)

	fmt.Println("═══ All loops stopped ═══")
	for _, ar := range results {
		fmt.Printf("  %s (%s): stopped=%s iterations=%d tokens=%d\n",
			ar.Role, ar.Name, ar.Result.Reason, ar.Result.Iterations, ar.Result.Budget.TokensUsed)
	}

	return results, nil
}

// seedWork has the CTO evaluate the input and emit seed events on the graph.
// Returns a task description that other agents will observe.
func (p *Pipeline) seedWork(ctx context.Context, input ProductInput) (string, error) {
	var spec string
	if input.SpecFile != "" {
		content, err := p.ws.ReadFile(input.SpecFile)
		if err != nil {
			return "", fmt.Errorf("read spec: %w", err)
		}
		spec = content
	} else if input.URL != "" {
		spec = fmt.Sprintf("Research and build product from: %s", input.URL)
	} else if input.Description != "" {
		spec = input.Description
	}

	// CTO evaluates and seeds direction.
	_, evaluation, err := p.cto.Runtime.Evaluate(ctx, "seed_direction",
		fmt.Sprintf(`You are seeding a product build. Evaluate this idea and emit clear direction for the team.

What needs building? What's the architecture? What should the Builder do first?
Be specific and actionable — other agents will read your events and self-direct.

Product idea:
%s`, spec))
	if err != nil {
		return "", fmt.Errorf("CTO seed: %w", err)
	}

	// Record the CTO's direction as an action event.
	_, err = p.cto.Runtime.Act(ctx, ActionSeedBuild, spec)
	if err != nil {
		return "", fmt.Errorf("CTO seed action: %w", err)
	}

	fmt.Printf("CTO direction:\n%s\n", evaluation)
	return evaluation, nil
}

// buildLoopConfigs creates loop configurations for each pipeline agent.
func (p *Pipeline) buildLoopConfigs(ctx context.Context, seedTask string, eventBus bus.IBus, cfg LoopConfig) ([]loop.Config, error) {
	var configs []loop.Config

	// Builder — generates code based on CTO direction.
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, fmt.Errorf("ensure builder: %w", err)
	}
	configs = append(configs, loop.Config{
		Agent:   builder,
		HumanID: p.humanID,
		Budget:  cfg.Budget,
		Task:    fmt.Sprintf("Build the product based on this direction:\n%s", seedTask),
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleBuilder, i, resp)
			}
		},
	})

	// Reviewer — watches for build events and reviews code.
	reviewer, err := p.ensureAgent(ctx, roles.RoleReviewer, "reviewer")
	if err != nil {
		return nil, fmt.Errorf("ensure reviewer: %w", err)
	}
	configs = append(configs, loop.Config{
		Agent:   reviewer,
		HumanID: p.humanID,
		Budget:  cfg.Budget,
		Task:    "Watch for code generation events. Review code for quality, security, and spec compliance. Report issues.",
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleReviewer, i, resp)
			}
		},
	})

	// Tester — watches for build/review events and runs tests.
	tester, err := p.ensureAgent(ctx, roles.RoleTester, "tester")
	if err != nil {
		return nil, fmt.Errorf("ensure tester: %w", err)
	}
	configs = append(configs, loop.Config{
		Agent:   tester,
		HumanID: p.humanID,
		Budget:  cfg.Budget,
		Task:    "Watch for code changes. Run tests, analyze coverage, and report gaps.",
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleTester, i, resp)
			}
		},
	})

	// Guardian — watches everything, can HALT. Gets a larger budget than
	// execution agents so it outlives them (OBSERVABLE invariant).
	guardianBudget := cfg.GuardianBudget
	if guardianBudget == (resources.BudgetConfig{}) {
		// Fallback: scale all execution budget dimensions so Guardian outlives them.
		guardianBudget = cfg.Budget
		guardianBudget.MaxIterations *= 10
		guardianBudget.MaxCostUSD *= 2
		guardianBudget.MaxDuration *= 2
	}
	configs = append(configs, loop.Config{
		Agent:   p.guardian,
		HumanID: p.humanID,
		Budget:  guardianBudget,
		Task:    "Monitor all agent activity for policy violations, trust anomalies, and authority overreach. HALT if anything looks wrong.",
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleGuardian, i, resp)
			}
		},
	})

	return configs, nil
}

// deriveName asks the CTO to derive a short, kebab-case product name from the spec.
func (p *Pipeline) deriveName(ctx context.Context, spec string) (string, error) {
	_, name, err := p.cto.Runtime.Evaluate(ctx, "product_name",
		fmt.Sprintf(`Derive a short product name (kebab-case, 2-4 words, lowercase, no special characters) from this product idea. Reply with ONLY the name, nothing else.

Product idea:
%s`, spec))
	if err != nil {
		return "product", nil // fallback
	}
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	var clean []byte
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			clean = append(clean, c)
		}
	}
	if len(clean) == 0 {
		return "product", nil
	}
	fmt.Printf("CTO named product: %s\n", string(clean))
	return string(clean), nil
}

// research gathers information about the product idea.
func (p *Pipeline) research(ctx context.Context, input ProductInput) (string, error) {
	var spec string

	if input.SpecFile != "" {
		content, err := p.ws.ReadFile(input.SpecFile)
		if err != nil {
			return "", fmt.Errorf("read spec: %w", err)
		}
		spec = content
	} else if input.URL != "" {
		researcher, err := p.ensureAgent(ctx, roles.RoleResearcher, "researcher")
		if err != nil {
			return "", err
		}
		_, evaluation, err := researcher.Runtime.Research(ctx, input.URL,
			"extract the product idea, key entities, features, and requirements. Output in Code Graph vocabulary where possible.")
		if err != nil {
			return "", fmt.Errorf("research URL: %w", err)
		}
		spec = evaluation
	} else if input.Description != "" {
		// For a plain description, the CTO evaluates directly — no need
		// to bounce through the Researcher since there's nothing to research.
		spec = input.Description
	}

	// CTO evaluates feasibility
	_, ctoEval, err := p.cto.Runtime.Evaluate(ctx, "feasibility",
		fmt.Sprintf("Evaluate this product idea for feasibility. What agents are needed? What's the build sequence? Key risks?\n\n%s", spec))
	if err != nil {
		return "", fmt.Errorf("CTO evaluate: %w", err)
	}

	fmt.Printf("CTO Assessment:\n%s\n", ctoEval)
	return spec, nil
}

// design creates a full Code Graph spec from the product idea.
func (p *Pipeline) design(ctx context.Context, spec string) (string, error) {
	architect, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect")
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`Design the full system architecture. Output a complete Code Graph spec.
Remember: derive complexity from simple compositions. Each view should have the minimal elements needed — if a view feels heavy, decompose it. Elegant, simple, beautiful.

Also specify the target language/framework at the top of your spec in a line like:
LANGUAGE: go
or
LANGUAGE: typescript

Product idea:
%s`, spec)

	_, design, err := architect.Runtime.Evaluate(ctx, "architecture", prompt)
	if err != nil {
		return "", fmt.Errorf("architect design: %w", err)
	}

	// CTO reviews the architecture
	_, review, err := p.cto.Runtime.Evaluate(ctx, "architecture_review",
		fmt.Sprintf("Review this architecture. Check: Are views minimal? Is complexity derived from composition rather than accumulated? Are there any bloated entities or views that should be decomposed? Is it elegant and simple?\n\n%s", design))
	if err != nil {
		return "", fmt.Errorf("CTO review design: %w", err)
	}

	fmt.Printf("Architecture Review:\n%s\n", review)
	return design, nil
}

// simplify reviews the Code Graph spec and reduces it to its minimal form.
func (p *Pipeline) simplify(ctx context.Context, design string) (string, error) {
	architect, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect")
	if err != nil {
		return "", err
	}

	const maxRounds = 3
	current := design

	for round := 1; round <= maxRounds; round++ {
		_, analysis, err := architect.Runtime.Evaluate(ctx, "simplify",
			fmt.Sprintf(`Review this Code Graph spec for simplification opportunities.

For each View: can it be composed from fewer elements? Are any elements redundant or derivable from others?
For each Entity: is it as small as possible? Should it be split or can properties be derived?
For each State machine: are there too many states? Can transitions be reduced?
For each Layout: does it have too many children? Can sub-views be composed instead?

If you find simplifications, output the REVISED spec with the changes applied.
If the spec is already minimal, respond with exactly: MINIMAL

Current spec:
%s`, current))
		if err != nil {
			return "", fmt.Errorf("simplify round %d: %w", round, err)
		}

		upper := strings.ToUpper(strings.TrimSpace(analysis))
		if upper == "MINIMAL" || strings.HasPrefix(upper, "MINIMAL") {
			fmt.Printf("Simplification complete after %d round(s) — spec is minimal.\n", round)
			return current, nil
		}

		fmt.Printf("Simplification round %d applied.\n", round)
		current = analysis
	}

	fmt.Printf("Simplification capped at %d rounds.\n", maxRounds)
	return current, nil
}

// extractLanguage pulls the target language from the design spec.
// Looks for "LANGUAGE: xxx" in the spec. Defaults to "go".
func (p *Pipeline) extractLanguage(design string) string {
	for _, line := range strings.Split(design, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "LANGUAGE:") {
			lang := strings.TrimSpace(line[len("LANGUAGE:"):])
			lang = strings.ToLower(lang)
			if lang != "" {
				return lang
			}
		}
	}
	return "go"
}

// build generates multi-file code from the design spec.
// Uses Evaluate (not CodeWrite) to avoid the "return ONLY code" instruction
// conflicting with our multi-file --- FILE: path --- format.
func (p *Pipeline) build(ctx context.Context, design string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`Generate production-quality %s code from this specification.

Output ALL files needed for a complete, runnable project. Use this exact format for each file:

--- FILE: path/to/file.ext ---
<file contents>

Include:
- Project config (go.mod, package.json, Cargo.toml, etc.)
- Source files (organized in packages/modules)
- Test files alongside the code they test
- A README.md with build and run instructions

Do NOT include explanation text outside of file blocks. Every line of output must be inside a file block.

Specification:
%s`, lang, design)

	// Use Evaluate instead of CodeWrite — CodeWrite prepends "Return ONLY the code"
	// which conflicts with our multi-file format.
	_, code, err := builder.Runtime.Evaluate(ctx, "code_generation", prompt)
	if err != nil {
		return nil, fmt.Errorf("builder code: %w", err)
	}

	// Record the build action
	if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "multi-file generation from spec"); err != nil {
		fmt.Printf("warning: write_code action event failed: %v\n", err)
	}

	// Parse multi-file output
	files := parseFiles(code)
	if len(files) == 0 {
		// Fallback: treat entire output as a single file
		ext := langExtension(lang)
		files = map[string]string{"main" + ext: code}
	}

	// Write all files to product repo
	for path, content := range files {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit(fmt.Sprintf("feat: initial %s code generation from spec", lang)); err != nil {
		return nil, fmt.Errorf("commit code: %w", err)
	}

	fmt.Printf("Generated %d files, committed.\n", len(files))
	return files, nil
}

// rebuild sends reviewer feedback to the builder and generates revised code.
func (p *Pipeline) rebuild(ctx context.Context, currentFiles map[string]string, feedback string, design string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	var filesSummary strings.Builder
	for path, content := range currentFiles {
		filesSummary.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n", path, content))
	}

	prompt := fmt.Sprintf(`The reviewer provided feedback on the code. Fix the issues and output ALL files again using the same format.

Reviewer feedback:
%s

Original specification:
%s

Current code:
%s

Output the COMPLETE revised files using --- FILE: path --- markers. Include ALL files, not just changed ones.`, feedback, design, filesSummary.String())

	_, code, err := builder.Runtime.Evaluate(ctx, "code_revision", prompt)
	if err != nil {
		return nil, fmt.Errorf("rebuild: %w", err)
	}

	files := parseFiles(code)
	if len(files) == 0 {
		return currentFiles, nil // no parseable output, keep current
	}

	for path, content := range files {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit("fix: address reviewer feedback"); err != nil {
		return nil, fmt.Errorf("commit rebuild: %w", err)
	}

	fmt.Printf("Rebuilt %d files from feedback, committed.\n", len(files))
	return files, nil
}

// review checks code quality and spec compliance. Returns feedback and whether approved.
func (p *Pipeline) review(ctx context.Context, files map[string]string, design string, lang string) (feedback string, approved bool, err error) {
	reviewer, err := p.ensureAgent(ctx, roles.RoleReviewer, "reviewer")
	if err != nil {
		return "", false, err
	}

	// Build code summary for review
	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, content))
	}
	allCode := codeSummary.String()

	// Code review
	_, codeReview, err := reviewer.Runtime.CodeReview(ctx, allCode, lang)
	if err != nil {
		return "", false, fmt.Errorf("code review: %w", err)
	}

	// Spec compliance
	_, specReview, err := reviewer.Runtime.Evaluate(ctx, "spec_compliance",
		fmt.Sprintf("Does this code match the design spec? Flag any deviations.\n\nDesign:\n%s\n\nCode:\n%s", design, allCode))
	if err != nil {
		return "", false, fmt.Errorf("spec review: %w", err)
	}

	// Simplicity check
	_, simplicityReview, err := reviewer.Runtime.Evaluate(ctx, "simplicity_check",
		fmt.Sprintf(`Review this code for unnecessary complexity:
- Components that could be derived from simpler compositions?
- Redundant abstractions or over-engineered patterns?
- Did the builder add extras beyond the spec?

Code:
%s`, allCode))
	if err != nil {
		return "", false, fmt.Errorf("simplicity review: %w", err)
	}

	// Final verdict
	_, verdict, err := reviewer.Runtime.Decide(ctx, "approve_or_reject",
		fmt.Sprintf(`Based on your reviews, should this code be APPROVED or does it need CHANGES?

Code Review: %s
Spec Compliance: %s
Simplicity: %s

Reply with APPROVED if the code is ready, or CHANGES NEEDED: followed by the specific issues to fix.`, codeReview, specReview, simplicityReview))
	if err != nil {
		return "", false, fmt.Errorf("verdict: %w", err)
	}

	fmt.Printf("Code Review:\n%s\n\nSpec Compliance:\n%s\n\nSimplicity:\n%s\n\nVerdict: %s\n",
		codeReview, specReview, simplicityReview, verdict)

	// APPROVED unless explicitly requesting changes
	upper := strings.ToUpper(verdict)
	approved = !strings.Contains(upper, "CHANGES NEEDED") &&
		!strings.Contains(upper, "CHANGES REQUIRED") &&
		!strings.Contains(upper, "REJECT")
	return verdict, approved, nil
}

// test installs deps, runs tests, and has the tester analyze gaps.
func (p *Pipeline) test(ctx context.Context, files map[string]string, lang string) error {
	tester, err := p.ensureAgent(ctx, roles.RoleTester, "tester")
	if err != nil {
		return err
	}

	// Install dependencies first
	p.installDeps(lang)

	// Run tests
	testCmd, testArgs := langTestCommand(lang)
	fmt.Printf("Running: %s %s\n", testCmd, strings.Join(testArgs, " "))

	cmd := exec.Command(testCmd, testArgs...)
	cmd.Dir = p.product.Dir
	testOutput, testErr := cmd.CombinedOutput()

	testResult := string(testOutput)
	if testErr != nil {
		fmt.Printf("Tests failed:\n%s\n", testResult)
	} else {
		fmt.Printf("Tests passed:\n%s\n", testResult)
	}

	// Have the tester analyze results and coverage gaps
	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, content))
	}

	_, testEval, err := tester.Runtime.Evaluate(ctx, "test_analysis",
		fmt.Sprintf(`Analyze the test results and code. Are there coverage gaps? What additional tests are needed?

Test output:
%s

Code:
%s`, testResult, codeSummary.String()))
	if err != nil {
		return fmt.Errorf("test analysis: %w", err)
	}

	fmt.Printf("Test Analysis:\n%s\n", testEval)

	// If tests failed, have the builder fix them
	if testErr != nil {
		fmt.Println("Attempting to fix failing tests...")
		builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
		if err != nil {
			return err
		}

		fixPrompt := fmt.Sprintf(`The tests are failing. Fix the code so tests pass.

Test output:
%s

Current code:
%s

Output ALL files using --- FILE: path --- markers.`, testResult, codeSummary.String())

		_, fixedCode, err := builder.Runtime.Evaluate(ctx, "test_fix", fixPrompt)
		if err != nil {
			return fmt.Errorf("fix tests: %w", err)
		}

		fixedFiles := parseFiles(fixedCode)
		for path, content := range fixedFiles {
			if err := p.product.WriteFile(path, content); err != nil {
				return fmt.Errorf("write fix %s: %w", path, err)
			}
		}
		if len(fixedFiles) > 0 {
			_ = p.product.Commit("fix: address failing tests")
		}

		// Re-run tests
		cmd2 := exec.Command(testCmd, testArgs...)
		cmd2.Dir = p.product.Dir
		retryOutput, retryErr := cmd2.CombinedOutput()
		if retryErr != nil {
			fmt.Printf("Tests still failing after fix attempt:\n%s\n", string(retryOutput))
		} else {
			fmt.Printf("Tests now passing:\n%s\n", string(retryOutput))
		}
	}

	return nil
}

// installDeps runs dependency installation for the target language.
func (p *Pipeline) installDeps(lang string) {
	var cmd *exec.Cmd

	switch strings.ToLower(lang) {
	case "go", "golang":
		cmd = exec.Command("go", "mod", "tidy")
	case "typescript", "ts", "javascript", "js":
		cmd = exec.Command("npm", "install")
	case "python", "py":
		// Check if requirements.txt exists
		if p.product != nil {
			cmd = exec.Command("pip", "install", "-r", "requirements.txt")
		}
	case "rust", "rs":
		cmd = exec.Command("cargo", "build")
	case "csharp", "c#", "cs":
		cmd = exec.Command("dotnet", "restore")
	default:
		return
	}

	if cmd != nil && p.product != nil {
		cmd.Dir = p.product.Dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Dep install warning: %s\n", string(out))
		} else {
			fmt.Printf("Dependencies installed.\n")
		}
	}
}

// integrate assembles and prepares for deployment.
func (p *Pipeline) integrate(ctx context.Context) error {
	integrator, err := p.ensureAgent(ctx, roles.RoleIntegrator, "integrator")
	if err != nil {
		return err
	}

	_, err = integrator.Runtime.Act(ctx, ActionIntegrate, "staging")
	if err != nil {
		return fmt.Errorf("integration: %w", err)
	}

	// Push to GitHub
	if err := p.product.Push(); err != nil {
		fmt.Printf("Push failed (may need manual push): %v\n", err)
	} else {
		fmt.Printf("Pushed to https://github.com/%s\n", p.product.Repo)
	}

	// Escalate to human for production approval
	humanID := p.humanID
	_, err = integrator.Runtime.Escalate(ctx, humanID, "Product ready for human review before production deploy")
	if err != nil {
		return fmt.Errorf("escalate: %w", err)
	}

	fmt.Println("Product assembled and ready for human review.")
	return nil
}

// guardianCheck runs the Guardian's integrity check after a phase.
// Returns true if the Guardian issued a HALT — the pipeline should stop.
func (p *Pipeline) guardianCheck(ctx context.Context, phase string) bool {
	events, err := p.guardian.Runtime.Memory(20)
	if err != nil || len(events) == 0 {
		return false
	}

	var summary strings.Builder
	for _, ev := range events {
		summary.WriteString(fmt.Sprintf("[%s] %s: %s\n", ev.Type().Value(), ev.Source().Value(), ev.ID().Value()))
	}

	_, eval, err := p.guardian.Runtime.Evaluate(ctx, "integrity_check_"+phase,
		fmt.Sprintf(`Review these recent events (after %s phase) for policy violations, trust anomalies, or authority overreach.

EXTRA SCRUTINY for:
- Agent spawn events (agent.role.assigned, authority.requested, authority.resolved) — verify authority and trust levels
- Self-modification events — flag for human review
- Revenue-affecting decisions — verify alignment

Events:
%s`,
			phase, summary.String()))
	if err != nil {
		fmt.Printf("Guardian check failed: %v\n", err)
		return false
	}

	if loop.ContainsSignal(eval, "HALT") {
		fmt.Printf("🛑 Guardian HALT (after %s):\n%s\n", phase, eval)
		// NOTE: Emit is context-unaware (eventgraph Runtime.Emit doesn't take ctx).
		// This is acceptable — the HALT event is best-effort observability, not
		// control flow. The pipeline stops regardless of whether the event persists.
		if _, err := p.guardian.Runtime.Emit(event.AgentEscalatedContent{
			AgentID:   p.guardian.Runtime.ID(),
			Authority: p.humanID,
			Reason:    fmt.Sprintf("[HALT after %s] %s", phase, eval),
		}); err != nil {
			fmt.Printf("warning: HALT escalation event failed: %v\n", err)
		}
		return true
	}

	if containsAlert(eval) {
		fmt.Printf("⚠ Guardian Alert (after %s):\n%s\n", phase, eval)
		if _, err := p.guardian.Runtime.Emit(event.AgentEscalatedContent{
			AgentID:   p.guardian.Runtime.ID(),
			Authority: p.humanID,
			Reason:    fmt.Sprintf("[%s phase] %s", phase, eval),
		}); err != nil {
			fmt.Printf("warning: alert escalation event failed: %v\n", err)
		}
	}

	return false
}

// ════════════════════════════════════════════════════════════════════════
// File parsing utilities
// ════════════════════════════════════════════════════════════════════════

// parseFiles extracts files from builder output using --- FILE: path --- markers.
func parseFiles(output string) map[string]string {
	files := make(map[string]string)
	lines := strings.Split(output, "\n")

	var currentPath string
	var currentContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- FILE:") && strings.HasSuffix(trimmed, "---") {
			// Save previous file if any
			if currentPath != "" {
				files[currentPath] = strings.TrimRight(currentContent.String(), "\n")
			}
			// Extract new path
			path := strings.TrimSpace(trimmed[len("--- FILE:") : len(trimmed)-len("---")])
			currentPath = path
			currentContent.Reset()
		} else if currentPath != "" {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}
	// Save last file
	if currentPath != "" {
		files[currentPath] = strings.TrimRight(currentContent.String(), "\n")
	}

	return files
}

// langExtension returns the default file extension for a language.
func langExtension(lang string) string {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return ".go"
	case "typescript", "ts":
		return ".ts"
	case "javascript", "js":
		return ".js"
	case "python", "py":
		return ".py"
	case "rust", "rs":
		return ".rs"
	case "csharp", "c#", "cs":
		return ".cs"
	default:
		return ".go"
	}
}

// langTestCommand returns the test command and args for a language.
func langTestCommand(lang string) (string, []string) {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return "go", []string{"test", "./..."}
	case "typescript", "ts":
		return "npx", []string{"vitest", "run"}
	case "javascript", "js":
		return "npm", []string{"test"}
	case "python", "py":
		return "python", []string{"-m", "pytest"}
	case "rust", "rs":
		return "cargo", []string{"test"}
	case "csharp", "c#", "cs":
		return "dotnet", []string{"test"}
	default:
		return "go", []string{"test", "./..."}
	}
}

// containsAlert checks if the Guardian's evaluation contains an alert keyword.
// HALT is handled separately by the explicit HALT check in guardianCheck —
// it is not included here to avoid duplicate event emission.
func containsAlert(eval string) bool {
	upper := strings.ToUpper(eval)
	for _, keyword := range []string{"ALERT", "VIOLATION", "QUARANTINE"} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

// Store returns the shared event graph.
func (p *Pipeline) Store() store.Store {
	return p.store
}

// Agents returns all active agents.
func (p *Pipeline) Agents() map[roles.Role]*roles.Agent {
	return p.agents
}
