// Package pipeline orchestrates the product build pipeline.
package pipeline

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
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
	PhaseResearch    Phase = "research"
	PhaseDesign      Phase = "design"
	PhaseBuild       Phase = "build"
	PhaseReview      Phase = "review"
	PhaseTest        Phase = "test"
	PhaseIntegrate   Phase = "integrate"
	PhaseMerge       Phase = "merge"
	PhaseSelfImprove Phase = "self-improve"
)

// Action constants for pipeline events — no magic strings.
const (
	ActionWriteCode    = "write_code"
	ActionSeedBuild    = "seed_product_build"
	ActionIntegrate    = "integrate_staging"
	ActionMergePR      = "merge_pr"
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
	RepoPath    string // Path to existing repo (targeted mode)
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

	cto           *roles.Agent
	guardian      *roles.Agent
	agents        map[roles.Role]*roles.Agent
	trackers      map[roles.Role]*resources.TrackingProvider // per-agent token tracking
	skipGuardian  bool
	skipSimplify  bool
	autoApprove   bool   // --yes flag active (authority requests auto-approved)
	reviewerModel string // model override for targeted reviews (empty = role default)

	// Authority infrastructure — always initialized; gate is optional.
	gate    *authority.Gate
	signer  event.Signer
	factory *event.EventFactory
	convID  types.ConversationID

	// telemetry accumulates data during a pipeline run. Set at the start of
	// Run/RunTargeted, written to disk at the end. Nil outside a run.
	telemetry *PipelineResult
}

// Config for creating a new pipeline.
type Config struct {
	Store   store.Store
	Actors  actor.IActorStore          // actor registry — humans via auth, agents via creation
	Trust   *trust.DefaultTrustModel   // trust model for gate enforcement
	HumanID types.ActorID              // pre-registered human operator (from auth/actor store)
	WorkDir string                     // Root directory for generated products
	Gate    *authority.Gate             // optional authority gate (nil = no approval required)

	// SkipGuardian disables Guardian integrity checks after each phase.
	// Saves ~6 LLM calls per pipeline run. Use for dev/testing only.
	SkipGuardian bool

	// SkipSimplify disables the simplification loop after design.
	// The Architect's design prompt already includes self-review instructions,
	// so this is often redundant for simple projects. Saves 1-2 Opus calls.
	SkipSimplify bool

	// AutoApprove indicates the --yes flag is active (all authority requests
	// auto-approved). Passed to Guardian so it doesn't flag missing
	// authority.requested/authority.resolved events as violations.
	AutoApprove bool

	// ReviewerModel overrides the model used for targeted reviews.
	// Empty string = use role default. Targeted reviews only check a focused
	// git diff, so Sonnet is sufficient — no deep architectural reasoning needed.
	ReviewerModel string
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
		store:         cfg.Store,
		actors:        cfg.Actors,
		humanID:       cfg.HumanID,
		humanName:     human.DisplayName(),
		ws:            ws,
		agents:        make(map[roles.Role]*roles.Agent),
		trackers:      make(map[roles.Role]*resources.TrackingProvider),
		skipGuardian:  cfg.SkipGuardian,
		skipSimplify:  cfg.SkipSimplify,
		autoApprove:   cfg.AutoApprove,
		reviewerModel: cfg.ReviewerModel,
	}

	// Always initialize event infrastructure — needed for OBSERVABLE invariant
	// (all agent spawns emit authority events, even in no-gate dev mode).
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

	p.signer = signer
	p.factory = factory
	p.convID = convID

	// Wire up spawner and authority gate if provided.
	if cfg.Gate != nil {
		p.gate = cfg.Gate

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
	// Skipped in dev/testing mode to save tokens.
	if !cfg.SkipGuardian {
		guardian, err := p.ensureAgent(ctx, roles.RoleGuardian, "guardian")
		if err != nil {
			return nil, fmt.Errorf("bootstrap Guardian: %w", err)
		}
		p.guardian = guardian
	} else {
		fmt.Println("  ↳ Guardian: SKIPPED (--skip-guardian)")
	}

	return p, nil
}

// providerForRole creates an intelligence provider with the model and system prompt
// appropriate for the role. Uses Claude CLI (flat rate via Max plan).
func (p *Pipeline) providerForRole(role roles.Role) (intelligence.Provider, error) {
	return p.providerForRoleWithModel(role, roles.PreferredModel(role))
}

// providerForRoleWithModel creates an intelligence provider with the given model
// and the system prompt for the role. Used when the model needs to differ from
// the role's default — e.g., targeted reviews use Sonnet instead of Opus.
func (p *Pipeline) providerForRoleWithModel(role roles.Role, model string) (intelligence.Provider, error) {
	return intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        model,
		SystemPrompt: roles.SystemPrompt(role, p.humanName),
	})
}

// ensureAgent creates an agent of the given role if it doesn't exist yet.
// When a spawner is configured, spawn requests go through the authority gate
// (human approval). Without a spawner, agents are created directly.
// Always uses the role's preferred model — callers needing a different model
// should create a temporary provider via providerForRoleWithModel instead.
func (p *Pipeline) ensureAgent(ctx context.Context, role roles.Role, name string) (*roles.Agent, error) {
	if agent, ok := p.agents[role]; ok {
		return agent, nil
	}

	var actorID types.ActorID
	var agentPK types.PublicKey

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
		// Derive the same public key the Spawner registered.
		pk, err := types.NewPublicKey([]byte(spawn.DerivePublicKey("agent:" + name)))
		if err != nil {
			return nil, fmt.Errorf("agent public key: %w", err)
		}
		agentPK = pk
	} else {
		// Direct creation — no approval gate (bootstrap or testing).
		agentPub := spawn.DerivePublicKey("agent:" + name)
		pk, err := types.NewPublicKey([]byte(agentPub))
		if err != nil {
			return nil, fmt.Errorf("agent public key: %w", err)
		}
		agentPK = pk
		agentActor, err := p.actors.Register(agentPK, name, event.ActorTypeAI)
		if err != nil {
			return nil, fmt.Errorf("create agent %s: %w", name, err)
		}
		actorID = agentActor.ID()

		// Emit authority events for OBSERVABLE invariant — all spawns are auditable,
		// even in dev/bootstrap mode without an authority gate.
		action := fmt.Sprintf("spawn agent %q as %s", name, role)
		reqEventID, err := p.emitAuthorityRequested(action, "auto-approved (no authority gate)")
		if err != nil {
			return nil, fmt.Errorf("emit authority.requested: %w", err)
		}
		if _, err := p.emitAuthorityResolved(reqEventID, authority.Resolution{
			Approved: true,
			Resolver: p.humanID,
			Reason:   "auto-approved (no authority gate)",
		}); err != nil {
			return nil, fmt.Errorf("emit authority.resolved: %w", err)
		}
	}

	model := roles.PreferredModel(role)
	rawProvider, err := p.providerForRoleWithModel(role, model)
	if err != nil {
		return nil, fmt.Errorf("provider for %s: %w", role, err)
	}
	tracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[role] = tracker
	agent, err := roles.NewAgent(ctx, roles.AgentConfig{
		Role:      role,
		Name:      name,
		ActorID:   actorID,
		PublicKey: agentPK,
		Store:     p.store,
		Provider:  tracker,
		HumanID:   p.humanID,
	})
	if err != nil {
		return nil, err
	}
	fmt.Printf("  ↳ %s agent %s using %s\n", role, actorID.Value(), model)
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
	pipelineStart := time.Now()
	p.telemetry = &PipelineResult{
		Mode:             "full",
		InputDescription: input.Description,
		StartedAt:        pipelineStart,
	}
	defer func() {
		p.telemetry.collectTokenUsage(p.trackers)
		writeTelemetry(p.telemetryBaseDir(), p.telemetry)
		p.telemetry = nil
	}()

	// ── Phase 1: Research ──
	fmt.Println("═══ Phase 1: Research ═══")
	phaseStart := time.Now()
	spec, ctoEval, err := p.research(ctx, input)
	if err != nil {
		return fmt.Errorf("research: %w", err)
	}
	p.telemetry.addPhaseTiming("Research", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "research"); halt {
		return fmt.Errorf("guardian halted pipeline after research phase")
	}

	// Extract product name from CTO evaluation or use provided name.
	name := input.Name
	if name == "" {
		name = extractName(ctoEval)
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
	phaseStart = time.Now()
	design, err := p.design(ctx, spec)
	if err != nil {
		return fmt.Errorf("design: %w", err)
	}
	if halt := p.guardianCheck(ctx, "design"); halt {
		return fmt.Errorf("guardian halted pipeline after design phase")
	}

	// ── Phase 2b: Simplify ──
	if !p.skipSimplify {
		fmt.Println("═══ Phase 2b: Simplify ═══")
		design, err = p.simplify(ctx, design)
		if err != nil {
			return fmt.Errorf("simplify: %w", err)
		}
	} else {
		fmt.Println("═══ Phase 2b: Simplify — SKIPPED ═══")
	}
	p.telemetry.addPhaseTiming("Design", time.Since(phaseStart))

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
	phaseStart = time.Now()
	files, err := p.build(ctx, design, lang)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	p.telemetry.addPhaseTiming("Build", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "build"); halt {
		return fmt.Errorf("guardian halted pipeline after build phase")
	}

	// ── Phase 4: Review → Rebuild loop ──
	phaseStart = time.Now()
	const maxReviewRounds = 3
	for round := 1; round <= maxReviewRounds; round++ {
		fmt.Printf("═══ Phase 4: Review (round %d) ═══\n", round)
		feedback, approved, err := p.review(ctx, files, design, lang)
		if err != nil {
			return fmt.Errorf("review round %d: %w", round, err)
		}
		p.telemetry.addReviewSignal(approved)
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
	p.telemetry.addPhaseTiming("Review", time.Since(phaseStart))

	// ── Phase 5: Test ──
	fmt.Println("═══ Phase 5: Test ═══")
	phaseStart = time.Now()
	err = p.test(ctx, files, lang)
	if err != nil {
		return fmt.Errorf("test: %w", err)
	}
	p.telemetry.addPhaseTiming("Test", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "test"); halt {
		return fmt.Errorf("guardian halted pipeline after test phase")
	}

	// ── Phase 6: Integrate ──
	fmt.Println("═══ Phase 6: Integrate ═══")
	phaseStart = time.Now()
	err = p.integrate(ctx)
	if err != nil {
		return fmt.Errorf("integrate: %w", err)
	}
	p.telemetry.addPhaseTiming("Integrate", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "integrate"); halt {
		return fmt.Errorf("guardian halted pipeline after integrate phase")
	}

	fmt.Println("═══ Pipeline Complete ═══")
	p.PrintTokenSummary()
	return nil
}

// RunTargeted executes the targeted pipeline for modifying existing code.
// Skips research/design/simplify — goes straight to understand → modify → review → test.
func (p *Pipeline) RunTargeted(ctx context.Context, input ProductInput) error {
	if input.RepoPath == "" {
		return fmt.Errorf("RunTargeted requires RepoPath")
	}
	if input.Description == "" {
		return fmt.Errorf("RunTargeted requires Description (what to change)")
	}

	pipelineStart := time.Now()
	p.telemetry = &PipelineResult{
		Mode:             "targeted",
		InputDescription: input.Description,
		StartedAt:        pipelineStart,
	}
	defer func() {
		p.telemetry.collectTokenUsage(p.trackers)
		writeTelemetry(p.telemetryBaseDir(), p.telemetry)
		p.telemetry = nil
	}()
	type phaseTiming struct {
		name     string
		duration time.Duration
	}
	var timings []phaseTiming

	// Open existing repo
	product, err := workspace.OpenRepo(input.RepoPath)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}
	p.product = product
	fmt.Printf("Repo: %s\n", product.Dir)

	// ── Phase 1: Context Load ──
	fmt.Println("═══ Phase 1: Context Load ═══")
	phaseStart := time.Now()
	existingFiles, err := product.ReadSourceFiles()
	if err != nil {
		return fmt.Errorf("read source files: %w", err)
	}
	fmt.Printf("Loaded %d source files.\n", len(existingFiles))

	gitLog, _ := product.GitLog(10)
	if gitLog != "" {
		fmt.Printf("Recent history:\n%s\n", gitLog)
	}

	// Build a lightweight file listing for CTO (not full contents)
	fileListing := buildFileListing(existingFiles)

	// Detect language from existing files
	lang := detectLanguage(existingFiles)
	fmt.Printf("Detected language: %s\n", lang)

	// Include key context files (CLAUDE.md, README, etc.) for CTO — not the full codebase
	keyContext := extractKeyFiles(existingFiles)

	contextLoadDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Context Load", contextLoadDuration})
	p.telemetry.addPhaseTiming("Context Load", contextLoadDuration)

	// ── Phase 2: Understand ──
	fmt.Println("═══ Phase 2: Understand ═══")
	phaseStart = time.Now()
	_, ctoAnalysis, err := p.cto.Runtime.Evaluate(ctx, "change_analysis",
		fmt.Sprintf(`Analyze this change request. Be BRIEF — the Builder reads files itself.

Output ONLY:
- Which files to change (paths + what to do in each, 1 line per file)
- Key risks (1-2 sentences max)
- Nothing else. No tables, no code blocks, no headers.

Change request: %s

Git history:
%s

Project structure:
%s

%s`, input.Description, gitLog, fileListing, keyContext))
	if err != nil {
		return fmt.Errorf("CTO analysis: %w", err)
	}
	fmt.Printf("CTO Analysis:\n%s\n", ctoAnalysis)
	if halt := p.guardianCheck(ctx, "understand"); halt {
		return fmt.Errorf("guardian halted pipeline after understand phase")
	}

	// Create branch for the changes
	branchName := "hive/" + sanitizeBranchName(input.Description)
	if err := product.CreateBranch(branchName); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}
	fmt.Printf("Branch: %s\n", branchName)

	// Capture base commit before building — reviewer diffs against this.
	baseCommit, err := product.HeadCommit()
	if err != nil {
		return fmt.Errorf("capture base commit: %w", err)
	}

	understandDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Understand", understandDuration})
	p.telemetry.addPhaseTiming("Understand", understandDuration)

	// ── Phase 3: Modify ──
	fmt.Println("═══ Phase 3: Modify ═══")
	phaseStart = time.Now()
	files, err := p.modify(ctx, existingFiles, ctoAnalysis, input.Description, lang)
	if err != nil {
		return fmt.Errorf("modify: %w", err)
	}
	if halt := p.guardianCheck(ctx, "modify"); halt {
		return fmt.Errorf("guardian halted pipeline after modify phase")
	}

	modifyDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Modify", modifyDuration})
	p.telemetry.addPhaseTiming("Modify", modifyDuration)

	// ── Phase 4: Review ──
	phaseStart = time.Now()
	const maxReviewRounds = 3
	for round := 1; round <= maxReviewRounds; round++ {
		fmt.Printf("═══ Phase 4: Review (round %d) ═══\n", round)
		feedback, approved, err := p.reviewTargeted(ctx, baseCommit, ctoAnalysis, input.Description, lang)
		if err != nil {
			return fmt.Errorf("review round %d: %w", round, err)
		}
		p.telemetry.addReviewSignal(approved)
		if halt := p.guardianCheck(ctx, "review"); halt {
			return fmt.Errorf("guardian halted pipeline after review phase")
		}

		if approved {
			fmt.Println("Changes approved by reviewer.")
			break
		}

		if round == maxReviewRounds {
			fmt.Println("Max review rounds reached — proceeding with current code.")
			break
		}

		fmt.Printf("═══ Phase 4b: Revise from feedback (round %d) ═══\n", round)
		files, err = p.revise(ctx, files, feedback, input.Description, lang)
		if err != nil {
			return fmt.Errorf("revise round %d: %w", round, err)
		}
	}
	reviewDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Review", reviewDuration})
	p.telemetry.addPhaseTiming("Review", reviewDuration)

	// ── Phase 5: Test ──
	fmt.Println("═══ Phase 5: Test ═══")
	phaseStart = time.Now()
	err = p.test(ctx, files, lang)
	if err != nil {
		return fmt.Errorf("test: %w", err)
	}
	if halt := p.guardianCheck(ctx, "test"); halt {
		return fmt.Errorf("guardian halted pipeline after test phase")
	}

	testDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Test", testDuration})
	p.telemetry.addPhaseTiming("Test", testDuration)

	// ── Phase 6: PR ──
	fmt.Println("═══ Phase 6: PR ═══")
	phaseStart = time.Now()
	prURL, prErr := p.openPR(ctx, product, branchName, input.Description, ctoAnalysis)
	if prErr != nil {
		fmt.Printf("PR creation failed (may need manual push): %v\n", prErr)
	}
	p.telemetry.PRURL = prURL
	prDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"PR", prDuration})
	p.telemetry.addPhaseTiming("PR", prDuration)

	// ── Phase 7: Merge ──
	if prURL != "" {
		fmt.Println("═══ Phase 7: Merge ═══")
		phaseStart = time.Now()
		integrator, mergeErr := p.ensureAgent(ctx, roles.RoleIntegrator, "integrator")
		if mergeErr != nil {
			fmt.Printf("Merge phase skipped (integrator unavailable): %v\n", mergeErr)
		} else {
			approved := true
			if p.gate != nil {
				approved = p.requestMergeApproval(prURL)
			}
			if approved {
				if err := p.mergePR(product, prURL); err != nil {
					fmt.Printf("PR merge failed (may need manual merge): %v\n", err)
				} else {
					p.telemetry.Merged = true
					if _, err := integrator.Runtime.Act(ctx, ActionMergePR, prURL); err != nil {
						fmt.Printf("warning: merge_pr action event failed: %v\n", err)
					}
				}
			} else {
				fmt.Println("PR merge skipped — approval denied.")
			}
		}
		mergeDuration := time.Since(phaseStart)
		timings = append(timings, phaseTiming{"Merge", mergeDuration})
		p.telemetry.addPhaseTiming("Merge", mergeDuration)
	}

	fmt.Println("═══ Pipeline Complete ═══")
	p.PrintTokenSummary()

	totalDuration := time.Since(pipelineStart)
	fmt.Println("\n═══ Timing Summary ═══")
	fmt.Printf("  %-16s %s\n", "Phase", "Duration")
	fmt.Printf("  %-16s %s\n", "─────", "────────")
	for _, t := range timings {
		fmt.Printf("  %-16s %s\n", t.name, t.duration.Round(time.Millisecond))
	}
	fmt.Printf("  %-16s %s\n", "TOTAL", totalDuration.Round(time.Millisecond))
	return nil
}

// maxSelfImproveIterations is the maximum number of improvements per session.
const maxSelfImproveIterations = 3

// SelfImproveRecommendation is the CTO's structured response from telemetry analysis.
type SelfImproveRecommendation struct {
	Description  string   `json:"description"`
	FilesToChange []string `json:"files_to_change"`
	ExpectedImpact string `json:"expected_impact"`
	Priority     string   `json:"priority"`
	SkipReason   string   `json:"skip_reason"`
}

// RunSelfImprove enters self-improvement mode: reads telemetry + codebase,
// has the CTO identify improvements, then runs targeted pipeline iterations.
// Loops up to maxSelfImproveIterations times, stopping when the CTO says
// nothing is worth fixing.
func (p *Pipeline) RunSelfImprove(ctx context.Context, input ProductInput) error {
	if input.RepoPath == "" {
		return fmt.Errorf("RunSelfImprove requires RepoPath")
	}

	for iteration := 1; iteration <= maxSelfImproveIterations; iteration++ {
		fmt.Printf("\n═══ Self-Improve: Iteration %d/%d ═══\n", iteration, maxSelfImproveIterations)

		// Step 1: Read telemetry
		telemetryResults, err := ReadTelemetry(input.RepoPath)
		if err != nil {
			return fmt.Errorf("read telemetry: %w", err)
		}
		fmt.Printf("Telemetry: %d past run(s) found.\n", len(telemetryResults))

		// Step 2: Load codebase context (reuse targeted mode Phase 1)
		product, err := workspace.OpenRepo(input.RepoPath)
		if err != nil {
			return fmt.Errorf("open repo: %w", err)
		}
		existingFiles, err := product.ReadSourceFiles()
		if err != nil {
			return fmt.Errorf("read source files: %w", err)
		}
		fileListing := buildFileListing(existingFiles)
		keyContext := extractKeyFiles(existingFiles)

		// Step 3: Build telemetry summary for CTO
		telemetrySummary := summarizeTelemetry(telemetryResults)

		// Step 4: CTO analysis
		fmt.Println("CTO analyzing telemetry + codebase...")
		_, ctoResponse, err := p.cto.Runtime.Evaluate(ctx, "self_improve_analysis",
			fmt.Sprintf(`You are analyzing this codebase and its pipeline telemetry to identify the single highest-impact improvement.

TELEMETRY DATA (from past pipeline runs):
%s

PROJECT STRUCTURE:
%s

%s

Look for:
- Recurring Guardian alerts (same alert across multiple runs = wasted spend)
- High-cost roles (is Guardian worth the spend? which roles dominate cost?)
- Slow phases (which phases take disproportionate time?)
- Reviewer friction patterns (CHANGES NEEDED signals that are false alarms)
- Code quality issues visible in the codebase itself

Respond with ONLY a JSON object (no markdown, no explanation outside the JSON):
{
  "description": "what to change — be specific and actionable",
  "files_to_change": ["path/to/file1.go", "path/to/file2.go"],
  "expected_impact": "cost/time/quality improvement expected",
  "priority": "high|medium|low",
  "skip_reason": "if nothing is worth fixing, explain why here; otherwise empty string"
}`, telemetrySummary, fileListing, keyContext))
		if err != nil {
			return fmt.Errorf("CTO self-improve analysis: %w", err)
		}

		// Step 5: Parse recommendation
		rec, err := parseSelfImproveRecommendation(ctoResponse)
		if err != nil {
			return fmt.Errorf("parse CTO recommendation: %w", err)
		}

		fmt.Printf("CTO recommendation (priority=%s): %s\n", rec.Priority, rec.Description)
		if rec.SkipReason != "" {
			fmt.Printf("CTO says nothing worth fixing: %s\n", rec.SkipReason)
			break
		}
		if rec.Description == "" {
			fmt.Println("CTO returned empty recommendation — stopping.")
			break
		}

		fmt.Printf("Expected impact: %s\n", rec.ExpectedImpact)
		fmt.Printf("Files to change: %v\n", rec.FilesToChange)

		// Step 6: Run targeted pipeline with the recommendation
		targetedInput := ProductInput{
			RepoPath:    input.RepoPath,
			Description: rec.Description,
		}
		fmt.Printf("\n═══ Self-Improve: Running targeted pipeline ═══\n")
		if err := p.RunTargeted(ctx, targetedInput); err != nil {
			return fmt.Errorf("self-improve iteration %d: %w", iteration, err)
		}

		fmt.Printf("═══ Self-Improve: Iteration %d complete ═══\n", iteration)
	}

	fmt.Println("\n═══ Self-Improve: Session Complete ═══")
	return nil
}

// summarizeTelemetry builds a human-readable summary of past pipeline runs for the CTO.
func summarizeTelemetry(results []PipelineResult) string {
	if len(results) == 0 {
		return "No telemetry data available (first run)."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d past pipeline run(s):\n\n", len(results)))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("--- Run %d (mode=%s, %s) ---\n", i+1, r.Mode, r.StartedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("  Input: %s\n", r.InputDescription))
		if r.PRURL != "" {
			sb.WriteString(fmt.Sprintf("  PR: %s (merged=%v)\n", r.PRURL, r.Merged))
		}

		// Phase timings
		if len(r.PhaseTimings) > 0 {
			sb.WriteString("  Phase timings:\n")
			for _, pt := range r.PhaseTimings {
				sb.WriteString(fmt.Sprintf("    %-16s %s\n", pt.Phase, pt.Duration.Round(time.Millisecond)))
			}
		}

		// Token usage
		if len(r.TokenUsage) > 0 {
			var totalCost float64
			sb.WriteString("  Token usage by role:\n")
			for _, tu := range r.TokenUsage {
				sb.WriteString(fmt.Sprintf("    %-12s %s: %d tokens, $%.4f\n", tu.Role, tu.Model, tu.TotalTokens, tu.CostUSD))
				totalCost += tu.CostUSD
			}
			sb.WriteString(fmt.Sprintf("  Total cost: $%.4f\n", totalCost))
		}

		// Guardian alerts
		if len(r.GuardianAlerts) > 0 {
			sb.WriteString(fmt.Sprintf("  Guardian alerts (%d):\n", len(r.GuardianAlerts)))
			for _, a := range r.GuardianAlerts {
				sb.WriteString(fmt.Sprintf("    - %s\n", a))
			}
		}

		// Review signals
		if len(r.ReviewSignals) > 0 {
			sb.WriteString(fmt.Sprintf("  Review signals: %s\n", strings.Join(r.ReviewSignals, ", ")))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// parseSelfImproveRecommendation extracts a SelfImproveRecommendation from LLM output.
// Handles JSON embedded in markdown code blocks or plain text.
func parseSelfImproveRecommendation(response string) (SelfImproveRecommendation, error) {
	var rec SelfImproveRecommendation

	// Try direct unmarshal first.
	if err := json.Unmarshal([]byte(response), &rec); err == nil {
		return rec, nil
	}

	// Extract JSON from markdown code blocks or surrounding text.
	jsonStr := extractJSONBlock(response)
	if jsonStr == "" {
		return rec, fmt.Errorf("no JSON found in CTO response")
	}

	if err := json.Unmarshal([]byte(jsonStr), &rec); err != nil {
		return rec, fmt.Errorf("parse recommendation JSON: %w", err)
	}
	return rec, nil
}

// extractJSONBlock finds the first JSON object in a string, handling markdown
// code blocks (```json ... ```) or bare JSON.
func extractJSONBlock(s string) string {
	// Try markdown code block first.
	if idx := strings.Index(s, "```json"); idx != -1 {
		start := idx + len("```json")
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx != -1 {
		start := idx + len("```")
		if end := strings.Index(s[start:], "```"); end != -1 {
			candidate := strings.TrimSpace(s[start : start+end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate
			}
		}
	}

	// Try to find bare JSON object.
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	// Find matching closing brace.
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// modify uses the builder in agentic mode to modify existing code directly.
// Falls back to text-based modify if the provider doesn't support Operate.
func (p *Pipeline) modify(ctx context.Context, existingFiles map[string]string, ctoAnalysis string, changeReq string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	// Try agentic mode first — builder reads/writes files directly
	tracker := p.trackers[roles.RoleBuilder]
	if tracker != nil {
		instruction := fmt.Sprintf(`You are working in a %s repository. Implement the following change:

CHANGE REQUEST: %s

CTO ANALYSIS: %s

Read the existing code, make the changes, and run tests to verify they pass.
If tests fail, fix the issues and re-run until tests pass.
Use the project's existing test commands (e.g., go test ./... for Go).
Preserve existing code style and conventions.
Do NOT add unnecessary changes beyond what's requested.`, lang, changeReq, ctoAnalysis)

		result, err := tracker.Operate(ctx, decision.OperateTask{
			WorkDir:     p.product.Dir,
			Instruction: instruction,
		})
		if err == nil {
			fmt.Printf("Builder (agentic): %s\n", truncate(result.Summary, 200))

			// Record the action event
			if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "agentic modification"); err != nil {
				fmt.Printf("warning: write_code action event failed: %v\n", err)
			}

			// Stage and commit whatever the builder changed
			_ = p.product.StageAll()
			_ = p.product.Commit(fmt.Sprintf("feat: %s", truncate(changeReq, 60)))

			// Re-read files from disk (builder may have changed anything)
			updatedFiles, readErr := p.product.ReadSourceFiles()
			if readErr != nil {
				return existingFiles, nil
			}
			return updatedFiles, nil
		}
		// If Operate isn't supported, fall through to text mode
		fmt.Printf("Agentic mode unavailable (%v), falling back to text mode.\n", err)
	}

	// Fallback: text-based modify — build full code string lazily (only when needed)
	var codeContext strings.Builder
	for path, content := range existingFiles {
		codeContext.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n\n", path, content))
	}
	return p.modifyText(ctx, existingFiles, codeContext.String(), ctoAnalysis, changeReq, lang)
}

// modifyText is the text-based fallback for modify when Operate isn't available.
func (p *Pipeline) modifyText(ctx context.Context, existingFiles map[string]string, existingCode string, ctoAnalysis string, changeReq string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`Modify this existing %s codebase to implement the requested change.

CHANGE REQUEST:
%s

CTO ANALYSIS:
%s

EXISTING CODEBASE:
%s

CRITICAL OUTPUT FORMAT RULES:
- Output ONLY file blocks using --- FILE: path --- markers
- Inside each file block, output ONLY raw source code — no markdown fences, no prose, no explanations
- Include ONLY files that changed or are new — do not re-output unchanged files
- Every line of your response must be inside a file block
- Preserve existing code style and conventions`, lang, changeReq, ctoAnalysis, existingCode)

	_, code, err := builder.Runtime.Evaluate(ctx, "code_modification", prompt)
	if err != nil {
		return nil, fmt.Errorf("builder modify: %w", err)
	}

	if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "targeted modification"); err != nil {
		fmt.Printf("warning: write_code action event failed: %v\n", err)
	}

	changedFiles := parseFiles(code)
	if len(changedFiles) == 0 {
		return nil, fmt.Errorf("builder produced no parseable file output")
	}

	sanitizeGoMod(changedFiles)

	// Merge: start with existing files, overlay changes
	merged := make(map[string]string, len(existingFiles)+len(changedFiles))
	for k, v := range existingFiles {
		merged[k] = v
	}
	for k, v := range changedFiles {
		merged[k] = v
	}

	for path, content := range changedFiles {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit(fmt.Sprintf("feat: %s", truncate(changeReq, 60))); err != nil {
		return nil, fmt.Errorf("commit changes: %w", err)
	}

	fmt.Printf("Modified %d files, committed.\n", len(changedFiles))
	return merged, nil
}

// targetedReviewModel returns the model to use for targeted reviews.
// Defaults to Sonnet — targeted reviews only check a focused diff.
func (p *Pipeline) targetedReviewModel() string {
	if p.reviewerModel != "" {
		return p.reviewerModel
	}
	return "claude-sonnet-4-6"
}

// reviewTargeted reviews changes against the original codebase using git diff.
// Uses a temporary Sonnet provider — targeted reviews only check a focused diff,
// not deep architectural reasoning. Override with Config.ReviewerModel.
// The provider is created fresh each call, avoiding the agent cache entirely.
func (p *Pipeline) reviewTargeted(ctx context.Context, baseCommit string, ctoAnalysis string, changeReq string, lang string) (string, bool, error) {
	// Use git diff from the base commit — only the builder's changes, not history.
	diff, err := p.product.GitDiff(baseCommit)
	if err != nil {
		return "", false, fmt.Errorf("git diff from %s: %w", baseCommit, err)
	}
	if diff == "" {
		return "No changes to review.", true, nil
	}

	// Targeted reviews use a temporary Sonnet provider — no need for a full
	// reviewer agent. This avoids the agent cache entirely (an Opus reviewer
	// cached from a greenfield run would silently ignore a model override).
	model := p.targetedReviewModel()
	rawProvider, err := p.providerForRoleWithModel(roles.RoleReviewer, model)
	if err != nil {
		return "", false, fmt.Errorf("review provider: %w", err)
	}
	// Wrap in tracking so token usage appears in the summary.
	tracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleReviewer] = tracker
	fmt.Printf("  ↳ targeted review using %s\n", model)

	prompt := fmt.Sprintf(`Review this diff to a %s codebase. Be CONCISE — no tables, no headers.

CHANGE REQUEST: %s
CTO ANALYSIS: %s

DIFF:
%s

Only BLOCK for: correctness bugs, logic errors, security issues, or broken tests.
Do NOT block for: style nits, missing tests, doc comments, scope creep, or nice-to-haves.
Mention non-blocking concerns briefly but still approve.

End with: APPROVED or CHANGES NEEDED: <specific blocking issues>`, lang, changeReq, ctoAnalysis, diff)

	resp, err := tracker.Reason(ctx, prompt, nil)
	if err != nil {
		return "", false, fmt.Errorf("review: %w", err)
	}
	review := resp.Content()

	fmt.Printf("Review:\n%s\n", review)

	return review, detectApproval(review), nil
}

// revise applies reviewer feedback — agentic mode preferred, text fallback.
func (p *Pipeline) revise(ctx context.Context, files map[string]string, feedback string, changeReq string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	// Try agentic mode
	tracker := p.trackers[roles.RoleBuilder]
	if tracker != nil {
		instruction := fmt.Sprintf(`You are working in a %s repository. Fix the reviewer's feedback:

ORIGINAL CHANGE REQUEST: %s

REVIEWER FEEDBACK:
%s

Read the current code, apply the fixes, and run tests to verify they pass.`, lang, changeReq, feedback)

		result, err := tracker.Operate(ctx, decision.OperateTask{
			WorkDir:     p.product.Dir,
			Instruction: instruction,
		})
		if err == nil {
			fmt.Printf("Builder (agentic revision): %s\n", truncate(result.Summary, 200))

			// Record the action event
			if _, actErr := builder.Runtime.Act(ctx, writeCodeAction(lang), "agentic revision"); actErr != nil {
				fmt.Printf("warning: write_code action event failed: %v\n", actErr)
			}

			_ = p.product.StageAll()
			_ = p.product.Commit("fix: address reviewer feedback")

			updatedFiles, readErr := p.product.ReadSourceFiles()
			if readErr != nil {
				return files, nil
			}
			return updatedFiles, nil
		}
		fmt.Printf("Agentic mode unavailable (%v), falling back to text mode.\n", err)
	}

	// Fallback: text-based revise
	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n\n", path, content))
	}

	prompt := fmt.Sprintf(`Fix the reviewer's issues with these changes to an existing %s codebase.

ORIGINAL CHANGE REQUEST: %s

REVIEWER FEEDBACK:
%s

CURRENT CODE (after modifications):
%s

Output ONLY the files that need further changes using --- FILE: path --- markers.`, lang, changeReq, feedback, codeSummary.String())

	_, code, err := builder.Runtime.Evaluate(ctx, "code_revision", prompt)
	if err != nil {
		return nil, fmt.Errorf("revise: %w", err)
	}

	if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "text revision"); err != nil {
		fmt.Printf("warning: write_code action event failed: %v\n", err)
	}

	revisedFiles := parseFiles(code)
	if len(revisedFiles) == 0 {
		return files, nil
	}

	sanitizeGoMod(revisedFiles)

	for k, v := range revisedFiles {
		files[k] = v
	}
	for path, content := range revisedFiles {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit("fix: address reviewer feedback"); err != nil {
		return nil, fmt.Errorf("commit revision: %w", err)
	}

	fmt.Printf("Revised %d files from feedback, committed.\n", len(revisedFiles))
	return files, nil
}

// openPR pushes the branch and opens a pull request.
// Returns the PR URL on success (empty string on failure).
func (p *Pipeline) openPR(ctx context.Context, product *workspace.Product, branch string, changeReq string, analysis string) (string, error) {
	// Push the branch
	if err := product.PushBranch(); err != nil {
		return "", fmt.Errorf("push branch: %w", err)
	}

	// Open PR via gh CLI
	title := truncate(changeReq, 70)
	body := fmt.Sprintf("## Change Request\n%s\n\n## CTO Analysis\n%s\n\n---\nGenerated by hive", changeReq, analysis)

	cmd := exec.Command("gh", "pr", "create", "--title", title, "--body", body)
	cmd.Dir = product.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create: %s: %w", string(out), err)
	}
	prURL := strings.TrimSpace(string(out))
	fmt.Printf("PR created: %s\n", prURL)
	return prURL, nil
}

// mergePR squash-merges a pull request via gh CLI.
// Non-fatal — logs and returns error if merge fails (e.g., branch protection).
func (p *Pipeline) mergePR(product *workspace.Product, prURL string) error {
	cmd := exec.Command("gh", "pr", "merge", prURL, "--squash")
	cmd.Dir = product.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh pr merge: %s: %w", string(out), err)
	}
	fmt.Printf("PR merged: %s\n", strings.TrimSpace(string(out)))
	return nil
}

// requestMergeApproval emits an authority.requested event for PR merge and
// checks approval through the Gate. Returns true if approved.
func (p *Pipeline) requestMergeApproval(prURL string) bool {
	// Emit authority.requested on the graph.
	action := fmt.Sprintf("%s: %s", ActionMergePR, prURL)
	reqEventID, err := p.emitAuthorityRequested(action, "PR passed review and tests — requesting merge approval")
	if err != nil {
		fmt.Printf("warning: authority.requested event failed: %v\n", err)
		// Fall through — still check the gate for human approval.
		// Use a zero EventID; the gate doesn't depend on it.
	}

	authReq := authority.Request{
		ID:            reqEventID,
		Action:        action,
		Actor:         p.humanID,
		Level:         event.AuthorityLevelRequired,
		Justification: "PR passed review and tests — requesting merge approval",
		CreatedAt:     time.Now(),
	}
	resolution := p.gate.Check(authReq)

	// Emit authority.resolved — causally linked to authority.requested.
	if reqEventID != (types.EventID{}) {
		if _, err := p.emitAuthorityResolved(reqEventID, resolution); err != nil {
			fmt.Printf("warning: authority.resolved event failed: %v\n", err)
		}
	}

	return resolution.Approved
}

// emitAuthorityRequested records an authority.requested event on the graph.
func (p *Pipeline) emitAuthorityRequested(action string, justification string) (types.EventID, error) {
	head, err := p.store.Head()
	if err != nil {
		return types.EventID{}, fmt.Errorf("store head: %w", err)
	}
	if !head.IsSome() {
		return types.EventID{}, fmt.Errorf("graph not bootstrapped")
	}
	causeID := head.Unwrap().ID()

	content := event.AuthorityRequestContent{
		Action:        action,
		Actor:         p.humanID,
		Level:         event.AuthorityLevelRequired,
		Justification: justification,
		Causes:        types.MustNonEmpty([]types.EventID{causeID}),
	}
	ev, err := p.factory.Create(event.EventTypeAuthorityRequested, p.humanID, content, []types.EventID{causeID}, p.convID, p.store, p.signer)
	if err != nil {
		return types.EventID{}, fmt.Errorf("create event: %w", err)
	}
	appended, err := p.store.Append(ev)
	if err != nil {
		return types.EventID{}, fmt.Errorf("append event: %w", err)
	}
	return appended.ID(), nil
}

// emitAuthorityResolved records an authority.resolved event on the graph.
func (p *Pipeline) emitAuthorityResolved(reqEventID types.EventID, res authority.Resolution) (types.EventID, error) {
	reason := types.None[string]()
	if res.Reason != "" {
		reason = types.Some(res.Reason)
	}
	content := event.AuthorityResolvedContent{
		RequestID: reqEventID,
		Approved:  res.Approved,
		Resolver:  res.Resolver,
		Reason:    reason,
	}
	source := p.humanID
	if res.Resolver != (types.ActorID{}) {
		source = res.Resolver
	}
	ev, err := p.factory.Create(event.EventTypeAuthorityResolved, source, content, []types.EventID{reqEventID}, p.convID, p.store, p.signer)
	if err != nil {
		return types.EventID{}, fmt.Errorf("create event: %w", err)
	}
	appended, err := p.store.Append(ev)
	if err != nil {
		return types.EventID{}, fmt.Errorf("append event: %w", err)
	}
	return appended.ID(), nil
}

// detectApproval checks the last portion of a review for the approval verdict.
// Only looks at the last 20 lines to avoid false positives from earlier text
// (e.g., "CHANGES NEEDED" appearing in quoted instructions or examples).
func detectApproval(review string) bool {
	lines := strings.Split(review, "\n")
	// Look at the last 20 lines for the verdict
	start := len(lines) - 20
	if start < 0 {
		start = 0
	}
	verdict := strings.ToUpper(strings.Join(lines[start:], "\n"))

	hasChanges := strings.Contains(verdict, "CHANGES NEEDED") ||
		strings.Contains(verdict, "CHANGES REQUIRED") ||
		strings.Contains(verdict, "REJECT")
	hasApproved := strings.Contains(verdict, "APPROVED")
	return hasApproved && !hasChanges
}

// detectLanguage infers the language from existing project files.
func detectLanguage(files map[string]string) string {
	if _, ok := files["go.mod"]; ok {
		return "go"
	}
	if _, ok := files["package.json"]; ok {
		return "typescript"
	}
	if _, ok := files["Cargo.toml"]; ok {
		return "rust"
	}
	if _, ok := files["requirements.txt"]; ok {
		return "python"
	}
	if _, ok := files["setup.py"]; ok {
		return "python"
	}
	if _, ok := files["pyproject.toml"]; ok {
		return "python"
	}
	for path := range files {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			return "go"
		case ".rs":
			return "rust"
		case ".py":
			return "python"
		case ".ts", ".tsx":
			return "typescript"
		case ".js", ".jsx":
			return "javascript"
		case ".cs":
			return "csharp"
		}
	}
	return "go"
}

// buildFileListing creates a compact listing of files with line counts.
// This gives the CTO enough context to identify relevant files without
// sending the full codebase (~90% token reduction vs full content).
func buildFileListing(files map[string]string) string {
	var b strings.Builder
	b.WriteString("Files:\n")
	for path, content := range files {
		lines := strings.Count(content, "\n") + 1
		b.WriteString(fmt.Sprintf("  %s (%d lines)\n", path, lines))
	}
	return b.String()
}

// extractKeyFiles returns the content of project-level context files
// (CLAUDE.md, README, etc.) that help the CTO understand the project
// without needing the full codebase.
func extractKeyFiles(files map[string]string) string {
	keyNames := []string{"CLAUDE.md", "README.md", "README", "SPEC.md", "ARCHITECTURE.md"}
	var b strings.Builder
	for _, name := range keyNames {
		if content, ok := files[name]; ok {
			b.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", name, content))
		}
	}
	return b.String()
}

// sanitizeBranchName converts a description into a valid git branch name.
func sanitizeBranchName(desc string) string {
	// Truncate at word boundary before 40 chars, lowercase, replace non-alphanumeric with hyphens
	s := strings.ToLower(desc)
	if len(s) > 40 {
		// Find last space/separator at or before 40 chars
		cut := strings.LastIndexAny(s[:40], " _/")
		if cut > 0 {
			s = s[:cut]
		} else {
			// First word exceeds 40 chars — hard truncate
			s = s[:40]
		}
	}
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if c == ' ' || c == '_' || c == '/' {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "change"
	}
	return result
}

// truncate returns s truncated to n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
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
		b := ar.Result.Budget
		fmt.Printf("  %s (%s): stopped=%s iterations=%d tokens=%d (in=%d out=%d cache_read=%d cache_write=%d) cost=$%.4f\n",
			ar.Role, ar.Name, ar.Result.Reason, ar.Result.Iterations,
			b.TokensUsed, b.InputTokens, b.OutputTokens, b.CacheReadTokens, b.CacheWriteTokens, b.CostUSD)
	}
	printTokenSummary(results)

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
	if p.guardian != nil {
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
	}

	return configs, nil
}

// extractName pulls a product name from the CTO's evaluation response.
// Looks for "NAME: kebab-case-name" in the text. Falls back to "product".
func extractName(ctoEval string) string {
	for _, line := range strings.Split(ctoEval, "\n") {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "NAME:") {
			name := strings.TrimSpace(trimmed[len("NAME:"):])
			name = strings.ToLower(name)
			name = strings.ReplaceAll(name, " ", "-")
			var clean []byte
			for i := 0; i < len(name); i++ {
				c := name[i]
				if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
					clean = append(clean, c)
				}
			}
			if len(clean) > 0 {
				result := string(clean)
				fmt.Printf("CTO named product: %s\n", result)
				return result
			}
		}
	}
	fmt.Println("CTO did not provide a product name — using default.")
	return "product"
}

// research gathers information about the product idea.
// Returns the spec text and the CTO's evaluation (which includes the derived product name).
func (p *Pipeline) research(ctx context.Context, input ProductInput) (spec string, ctoEval string, err error) {
	if input.SpecFile != "" {
		content, err := p.ws.ReadFile(input.SpecFile)
		if err != nil {
			return "", "", fmt.Errorf("read spec: %w", err)
		}
		spec = content
	} else if input.URL != "" {
		researcher, err := p.ensureAgent(ctx, roles.RoleResearcher, "researcher")
		if err != nil {
			return "", "", err
		}
		_, evaluation, err := researcher.Runtime.Research(ctx, input.URL,
			"extract the product idea, key entities, features, and requirements. Output in Code Graph vocabulary where possible.")
		if err != nil {
			return "", "", fmt.Errorf("research URL: %w", err)
		}
		spec = evaluation
	} else if input.Description != "" {
		spec = input.Description
	}

	// CTO evaluates feasibility and derives a product name in one call.
	_, ctoEval, err = p.cto.Runtime.Evaluate(ctx, "feasibility",
		fmt.Sprintf(`Evaluate this product idea for feasibility. What agents are needed? What's the build sequence? Key risks?

On the LAST LINE of your response, output ONLY a kebab-case product name (2-4 words, lowercase, no special characters) like:
NAME: my-product-name

Product idea:
%s`, spec))
	if err != nil {
		return "", "", fmt.Errorf("CTO evaluate: %w", err)
	}

	fmt.Printf("CTO Assessment:\n%s\n", ctoEval)
	return spec, ctoEval, nil
}

// design creates a full Code Graph spec from the product idea.
// The Architect self-reviews for minimality — no separate CTO review call needed.
func (p *Pipeline) design(ctx context.Context, spec string) (string, error) {
	architect, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect")
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`Design the full system architecture. Output a complete Code Graph spec.

CRITICAL CONSTRAINTS — review your own output before responding:
- Derive complexity from simple compositions. Each view: minimal elements needed.
- If a view feels heavy, decompose it. Elegant, simple, beautiful.
- Are views minimal? Is complexity derived from composition, not accumulated?
- Are there bloated entities or views that should be decomposed?
- Count your elements — can any be removed without losing functionality?

Specify the target language/framework at the top of your spec:
LANGUAGE: go

Product idea:
%s`, spec)

	_, design, err := architect.Runtime.Evaluate(ctx, "architecture", prompt)
	if err != nil {
		return "", fmt.Errorf("architect design: %w", err)
	}

	return design, nil
}

// simplify reviews the Code Graph spec and reduces it to its minimal form.
func (p *Pipeline) simplify(ctx context.Context, design string) (string, error) {
	architect, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect")
	if err != nil {
		return "", err
	}

	const maxRounds = 2
	current := design

	for round := 1; round <= maxRounds; round++ {
		_, analysis, err := architect.Runtime.Evaluate(ctx, "simplify",
			fmt.Sprintf(`Review this Code Graph spec for simplification. Apply ALL simplifications in ONE pass.

- Can any View be composed from fewer elements? Any redundant or derivable?
- Can any Entity be smaller? Properties derived instead of stored?
- Can any State machine have fewer states or transitions?

If you find simplifications, output the COMPLETE REVISED spec.
If already minimal, respond with exactly: MINIMAL

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

CRITICAL OUTPUT FORMAT RULES:
- Every line of output must be inside a file block
- Inside file blocks, output ONLY raw source code — no markdown fences (no ` + "```" + `), no prose, no explanations
- Do NOT include any text outside of file blocks

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

	// Sanitize go.mod — LLMs often embed newlines in the module path.
	sanitizeGoMod(files)

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

	sanitizeGoMod(files)

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

// review checks code quality and spec compliance in a single LLM call.
// Returns feedback and whether approved.
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

	// Single comprehensive review call — replaces 4 separate calls.
	_, review, err := reviewer.Runtime.Evaluate(ctx, "code_review",
		fmt.Sprintf(`Review this %s code comprehensively. Cover ALL of the following in ONE response:

## 1. Code Quality
Bugs, security issues, error handling, test coverage, best practices.

## 2. Spec Compliance
Does the code match this design spec? Flag deviations.
Design:
%s

## 3. Simplicity
- Unnecessary complexity? Over-engineered patterns?
- Components that could be derived from simpler compositions?
- Extras beyond the spec?

## 4. Verdict
End with exactly one of:
- APPROVED — code is ready
- CHANGES NEEDED: followed by the specific issues to fix

Code:
%s`, lang, design, allCode))
	if err != nil {
		return "", false, fmt.Errorf("review: %w", err)
	}

	fmt.Printf("Review:\n%s\n", review)

	approved = detectApproval(review)
	return review, approved, nil
}

// test installs deps, runs tests, and has the tester analyze failures.
// Skips the analysis LLM call if tests pass — no need to spend tokens on "looks good".
func (p *Pipeline) test(ctx context.Context, files map[string]string, lang string) error {
	// Install dependencies first
	p.installDeps(lang)

	// Run tests
	testCmd, testArgs := langTestCommand(lang)
	fmt.Printf("Running: %s %s\n", testCmd, strings.Join(testArgs, " "))

	moduleDir := findModuleDir(p.product.Dir, lang)
	cmd := exec.Command(testCmd, testArgs...)
	cmd.Dir = moduleDir
	testOutput, testErr := cmd.CombinedOutput()

	testResult := string(testOutput)
	if testErr == nil {
		fmt.Printf("Tests passed:\n%s\n", testResult)
		return nil // No need for tester analysis or builder fixes.
	}

	fmt.Printf("Tests failed:\n%s\n", testResult)

	// Tests failed — have the tester analyze what went wrong.
	tester, err := p.ensureAgent(ctx, roles.RoleTester, "tester")
	if err != nil {
		return err
	}

	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, content))
	}

	_, testEval, err := tester.Runtime.Evaluate(ctx, "test_analysis",
		fmt.Sprintf(`Tests are failing. Analyze the failures and identify root causes.

Test output:
%s

Code:
%s`, testResult, codeSummary.String()))
	if err != nil {
		return fmt.Errorf("test analysis: %w", err)
	}

	fmt.Printf("Test Analysis:\n%s\n", testEval)

	// Have the builder fix the failures.
	{
		fmt.Println("Attempting to fix failing tests...")
		builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
		if err != nil {
			return err
		}

		fixed := false

		// Try agentic mode first — builder reads/writes files directly
		tracker := p.trackers[roles.RoleBuilder]
		if tracker != nil {
			instruction := fmt.Sprintf(`You are working in a %s repository. The tests are failing. Fix the code so tests pass.

Test output:
%s

Read the failing tests and the code under test, fix the issues, and run tests to verify they pass.
Use the project's existing test commands (e.g., go test ./... for Go).
Preserve existing code style and conventions.`, lang, testResult)

			result, opErr := tracker.Operate(ctx, decision.OperateTask{
				WorkDir:     moduleDir,
				Instruction: instruction,
			})
			if opErr == nil {
				fmt.Printf("Builder (agentic fix): %s\n", truncate(result.Summary, 200))

				if _, actErr := builder.Runtime.Act(ctx, writeCodeAction(lang), "agentic test fix"); actErr != nil {
					fmt.Printf("warning: write_code action event failed: %v\n", actErr)
				}

				_ = p.product.StageAll()
				_ = p.product.Commit("fix: address failing tests")
				fixed = true
			} else {
				fmt.Printf("Agentic mode unavailable (%v), falling back to text mode.\n", opErr)
			}
		}

		// Fallback: text-based fix
		if !fixed {
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
			sanitizeGoMod(fixedFiles)
			for path, content := range fixedFiles {
				if err := p.product.WriteFile(path, content); err != nil {
					return fmt.Errorf("write fix %s: %w", path, err)
				}
			}
			if len(fixedFiles) > 0 {
				_ = p.product.Commit("fix: address failing tests")
			}
		}

		// Re-run tests
		cmd2 := exec.Command(testCmd, testArgs...)
		cmd2.Dir = moduleDir
		retryOutput, retryErr := cmd2.CombinedOutput()
		if retryErr != nil {
			fmt.Printf("Tests still failing after fix attempt:\n%s\n", string(retryOutput))
			return fmt.Errorf("tests still failing after fix attempt")
		}
		fmt.Printf("Tests now passing:\n%s\n", string(retryOutput))
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
		cmd.Dir = findModuleDir(p.product.Dir, lang)
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
	if p.skipGuardian {
		return false
	}
	events, err := p.guardian.Runtime.Memory(200)
	if err != nil || len(events) == 0 {
		return false
	}

	var summary strings.Builder
	for _, ev := range events {
		summary.WriteString(fmt.Sprintf("[%s] %s: %s\n", ev.Type().Value(), ev.Source().Value(), ev.ID().Value()))
	}

	// Build pipeline context so Guardian doesn't flag expected behavior.
	var pipelineCtx string
	if p.autoApprove {
		pipelineCtx = `
Pipeline context: --yes flag is active (auto-approve mode). Missing authority.requested/authority.resolved events are EXPECTED — the approval gate is bypassed. Do not flag this as a violation.
`
	}

	_, eval, err := p.guardian.Runtime.Evaluate(ctx, "integrity_check_"+phase,
		fmt.Sprintf(`Review these recent events (after %s phase) for policy violations, trust anomalies, or authority overreach.
%s
EXTRA SCRUTINY for:
- Agent spawn events (agent.role.assigned, authority.requested, authority.resolved) — verify authority and trust levels
- Self-modification events — flag for human review
- Revenue-affecting decisions — verify alignment

Events:
%s`,
			phase, pipelineCtx, summary.String()))
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
		if p.telemetry != nil {
			p.telemetry.addGuardianAlert(fmt.Sprintf("[%s phase] %s", phase, eval))
		}
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

// sanitizeGoMod fixes common LLM go.mod corruption.
// The most common issue is newlines embedded in the module path string,
// producing `module "github.com/\nfoo/bar"` which Go rejects.
// Fix: rejoin any line that looks like a continuation of a module directive.
func sanitizeGoMod(files map[string]string) {
	content, ok := files["go.mod"]
	if !ok {
		return
	}

	lines := strings.Split(content, "\n")
	var cleaned []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// If a module/go/require line is split across multiple lines, rejoin.
		if strings.HasPrefix(trimmed, "module ") || trimmed == "module" {
			// Collect continuation lines until we have the full module path.
			joined := trimmed
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next == "" || strings.HasPrefix(next, "go ") ||
					strings.HasPrefix(next, "require") ||
					strings.HasPrefix(next, "replace") ||
					strings.HasPrefix(next, "module ") {
					break
				}
				// This line is a continuation of the module directive.
				joined += next
				i++
			}
			// Remove any quotes around the module path.
			joined = strings.ReplaceAll(joined, "\"", "")
			cleaned = append(cleaned, joined)
		} else {
			cleaned = append(cleaned, line)
		}
	}
	files["go.mod"] = strings.Join(cleaned, "\n")
}

// parseFiles extracts files from builder output using --- FILE: path --- markers.
// Strips markdown code fences (```lang / ```) that LLMs sometimes wrap around file content.
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
				files[currentPath] = stripMarkdownFences(strings.TrimRight(currentContent.String(), "\n"))
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
		files[currentPath] = stripMarkdownFences(strings.TrimRight(currentContent.String(), "\n"))
	}

	return files
}

// stripMarkdownFences removes markdown code fences and trailing prose from file content.
// Handles: opening ```lang on first line, closing ``` on last code line,
// and any trailing markdown text after the closing fence.
func stripMarkdownFences(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	start := 0
	end := len(lines)

	// Strip leading markdown fence (```go, ```python, etc.)
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		start = 1
	}

	// Find and strip trailing markdown fence + any prose after it
	for i := end - 1; i >= start; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "```" {
			end = i
			break
		}
	}

	if start >= end {
		return content // safety: don't return empty if something went wrong
	}

	return strings.Join(lines[start:end], "\n")
}

// langMarkerFile returns the build-system marker file for a language.
// Used by findModuleDir to locate the module root.
func langMarkerFile(lang string) string {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return "go.mod"
	case "typescript", "ts", "javascript", "js":
		return "package.json"
	case "python", "py":
		return "pyproject.toml"
	case "rust", "rs":
		return "Cargo.toml"
	case "csharp", "c#", "cs":
		// *.csproj — use a glob pattern handled specially below.
		return "*.csproj"
	default:
		return "go.mod"
	}
}

// findModuleDir locates the directory containing the language's module marker
// file (e.g. go.mod for Go). It checks productDir first, then walks one level
// of subdirectories. Returns productDir as fallback if no marker is found.
// When multiple subdirectories contain the marker, the first alphabetically
// is returned for determinism.
func findModuleDir(productDir, lang string) string {
	marker := langMarkerFile(lang)

	// Check root first.
	if markerExists(productDir, marker) {
		return productDir
	}

	// Walk one level of subdirectories.
	entries, err := os.ReadDir(productDir)
	if err != nil {
		return productDir
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(productDir, entry.Name())
		if markerExists(subdir, marker) {
			return subdir
		}
	}

	return productDir
}

// markerExists checks whether a marker file exists in dir.
// Supports glob patterns (e.g. "*.csproj").
func markerExists(dir, marker string) bool {
	if strings.Contains(marker, "*") {
		matches, err := filepath.Glob(filepath.Join(dir, marker))
		return err == nil && len(matches) > 0
	}
	_, err := os.Stat(filepath.Join(dir, marker))
	return err == nil
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

// containsAlert checks if the Guardian's evaluation contains an alert directive.
// Uses line-start matching (via ContainsSignal) to avoid false positives from
// prose like "NO VIOLATIONS DETECTED".
// HALT is handled separately by the explicit HALT check in guardianCheck —
// it is not included here to avoid duplicate event emission.
func containsAlert(eval string) bool {
	for _, keyword := range []string{"ALERT", "VIOLATION", "QUARANTINE"} {
		if loop.ContainsSignal(eval, keyword) {
			return true
		}
	}
	return false
}

// PrintTokenSummary prints per-agent token usage from tracking providers.
func (p *Pipeline) PrintTokenSummary() {
	fmt.Println("\n═══ Token Usage Summary ═══")
	fmt.Printf("  %-12s %-8s %8s %8s %8s %10s %10s %10s\n",
		"Role", "Model", "Input", "Output", "Total", "CacheRead", "CacheWrite", "Cost")
	fmt.Printf("  %-12s %-8s %8s %8s %8s %10s %10s %10s\n",
		"────", "─────", "─────", "──────", "─────", "─────────", "──────────", "────")

	var totalIn, totalOut, totalTokens, totalCacheRead, totalCacheWrite int
	var totalCost float64

	for role, tracker := range p.trackers {
		s := tracker.Snapshot()
		fmt.Printf("  %-12s %-8s %8d %8d %8d %10d %10d %10s\n",
			role, tracker.Model(), s.InputTokens, s.OutputTokens, s.TokensUsed,
			s.CacheReadTokens, s.CacheWriteTokens, fmt.Sprintf("$%.4f", s.CostUSD))
		totalIn += s.InputTokens
		totalOut += s.OutputTokens
		totalTokens += s.TokensUsed
		totalCacheRead += s.CacheReadTokens
		totalCacheWrite += s.CacheWriteTokens
		totalCost += s.CostUSD
	}

	fmt.Printf("  %-12s %-8s %8d %8d %8d %10d %10d %10s\n",
		"TOTAL", "", totalIn, totalOut, totalTokens,
		totalCacheRead, totalCacheWrite, fmt.Sprintf("$%.4f", totalCost))
}

// printTokenSummary prints an aggregate token usage table from loop results.
func printTokenSummary(results []loop.AgentResult) {
	var totalIn, totalOut, totalCacheRead, totalCacheWrite, totalTokens int
	var totalCost float64

	fmt.Println("\n═══ Token Usage Summary ═══")
	fmt.Printf("  %-12s %8s %8s %8s %10s %10s %10s\n",
		"Role", "Input", "Output", "Total", "CacheRead", "CacheWrite", "Cost")
	fmt.Printf("  %-12s %8s %8s %8s %10s %10s %10s\n",
		"────", "─────", "──────", "─────", "─────────", "──────────", "────")

	for _, ar := range results {
		b := ar.Result.Budget
		fmt.Printf("  %-12s %8d %8d %8d %10d %10d %10s\n",
			ar.Role, b.InputTokens, b.OutputTokens, b.TokensUsed,
			b.CacheReadTokens, b.CacheWriteTokens, fmt.Sprintf("$%.4f", b.CostUSD))
		totalIn += b.InputTokens
		totalOut += b.OutputTokens
		totalTokens += b.TokensUsed
		totalCacheRead += b.CacheReadTokens
		totalCacheWrite += b.CacheWriteTokens
		totalCost += b.CostUSD
	}

	fmt.Printf("  %-12s %8d %8d %8d %10d %10d %10s\n",
		"TOTAL", totalIn, totalOut, totalTokens,
		totalCacheRead, totalCacheWrite, fmt.Sprintf("$%.4f", totalCost))
}

// Store returns the shared event graph.
func (p *Pipeline) Store() store.Store {
	return p.store
}

// Agents returns all active agents.
func (p *Pipeline) Agents() map[roles.Role]*roles.Agent {
	return p.agents
}
