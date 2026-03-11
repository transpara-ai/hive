// Package pipeline orchestrates the product build pipeline.
package pipeline

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
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
	CTOAnalysis string // Pre-computed CTO analysis (skips Understand phase when set)
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
	reviewerModel  string // model override for targeted reviews (empty = role default)
	builderModel   string // model override for targeted builds (empty = role default)
	ctoModel       string // model override for self-improve CTO analysis (empty = role default)
	guardianModel  string // model override for Guardian integrity checks (empty = Sonnet default)
	architectModel string // model override for Architect design and simplify calls (empty = Sonnet default)

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

	// BuilderModel overrides the model used for targeted builds.
	// Empty string = use role default (Sonnet). Targeted builds are CTO-directed
	// with small, well-scoped modifications — Sonnet handles these well.
	BuilderModel string

	// CTOModel overrides the model used for self-improve CTO analysis.
	// Empty string = use Sonnet default. The task is structured JSON output
	// from telemetry data — identify one improvement, list files, output JSON.
	// Deep architectural reasoning is not required.
	CTOModel string

	// GuardianModel overrides the model used for Guardian integrity checks.
	// Empty string = use Sonnet default. Guardian's task is a binary HALT/pass
	// classification — read phase events, emit HALT or pass. No deep reasoning needed.
	GuardianModel string

	// ArchitectModel overrides the model used for Architect design and simplify calls.
	// Empty string = use Sonnet default. Spec-writing and simplification are structured
	// generation tasks — same reasoning as the CTO Understand switch that saved ~$1.60/run.
	ArchitectModel string
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
		reviewerModel:  cfg.ReviewerModel,
		builderModel:   cfg.BuilderModel,
		ctoModel:       cfg.CTOModel,
		guardianModel:  cfg.GuardianModel,
		architectModel: cfg.ArchitectModel,
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
	prompt := roles.SystemPrompt(role, p.humanName)

	// Bake pipeline-mode context into the Guardian's system prompt at creation
	// time so every Guardian call gets it — not just per-phase Evaluate() calls.
	if role == roles.RoleGuardian && p.autoApprove {
		prompt += `

== PIPELINE CONTEXT ==
The --yes flag is active (auto-approve mode). Missing authority.requested/authority.resolved events are EXPECTED — the approval gate is bypassed. Do not flag this as a violation.`
	}

	return intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        model,
		SystemPrompt: prompt,
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
	resolver := res.Resolver
	if resolver == (types.ActorID{}) {
		resolver = p.humanID
	}
	content := event.AuthorityResolvedContent{
		RequestID: reqEventID,
		Approved:  res.Approved,
		Resolver:  resolver,
		Reason:    reason,
	}
	source := resolver
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

// Store returns the shared event graph.
func (p *Pipeline) Store() store.Store {
	return p.store
}

// Agents returns all active agents.
func (p *Pipeline) Agents() map[roles.Role]*roles.Agent {
	return p.agents
}
