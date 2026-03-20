// Package pipeline orchestrates the product build pipeline.
package pipeline

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/graph"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/lovyou-ai/agent"
	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/mind"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/spawn"
	"github.com/lovyou-ai/hive/pkg/work"
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
	PhaseEvolve      Phase = "evolve"
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
	// ContextFiles, when non-nil, replaces ReadSourceFiles() in the targeted
	// pipeline. Used by self-improve to pass pre-filtered pipeline files to the
	// Builder, preventing context bloat from unrelated packages.
	ContextFiles map[string]string
}

// Pipeline orchestrates agents through the product build phases.
type Pipeline struct {
	store     store.Store
	actors    actor.IActorStore
	graph     *graph.Graph       // shared graph facade (bus-integrated, mutex-safe)
	humanID   types.ActorID
	humanName string
	ws        *workspace.Workspace
	product   *workspace.Product // current product being built
	spawner   *spawn.Spawner     // nil = direct creation (no approval)

	trustModel    trust.ITrustModel // for RecordVerifiedWork after successful phases
	cto           *hiveagent.Agent
	guardian      *hiveagent.Agent
	agents        map[roles.Role]*hiveagent.Agent
	trackers      map[roles.Role]*resources.TrackingProvider // per-agent token tracking
	skipGuardian  bool
	skipReviewer  bool
	skipSimplify  bool
	autoApprove   bool   // --yes flag active (authority requests auto-approved)
	reviewerModel  string // model override for targeted reviews (empty = role default)
	builderModel   string // model override for targeted builds (empty = role default)
	ctoModel       string // model override for self-improve CTO analysis (empty = role default)
	guardianModel  string // model override for Guardian integrity checks (empty = Sonnet default)
	architectModel string // model override for Architect design and simplify calls (empty = Sonnet default)
	resume         bool   // resume evolve session from saved state

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

	// Resume resumes an evolve session from saved state (.hive/evolve-state.json),
	// skipping iterations that have already completed successfully.
	Resume bool
}


// New creates a pipeline and bootstraps the CTO and Guardian.
// The human operator must already be registered in the actor store (via auth).
func New(ctx context.Context, cfg Config) (*Pipeline, error) {
	// Verify the human exists in the actor store
	human, err := cfg.Actors.Get(cfg.HumanID)
	if err != nil {
		return nil, fmt.Errorf("human operator not found in actor store: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Human operator: %s (%s)\n", human.DisplayName(), human.ID().Value())

	ws, err := workspace.New(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}

	// Create the shared graph facade — used by hiveagent.New() for bus-integrated,
	// mutex-safe event recording with proper causality tracking.
	g := graph.New(cfg.Store, cfg.Actors, graph.WithSigner(deriveSignerFromID(cfg.HumanID)))
	if err := g.Start(); err != nil {
		return nil, fmt.Errorf("start graph: %w", err)
	}

	p := &Pipeline{
		store:         cfg.Store,
		actors:        cfg.Actors,
		graph:         g,
		trustModel:    cfg.Trust,
		humanID:       cfg.HumanID,
		humanName:     human.DisplayName(),
		ws:            ws,
		agents:        make(map[roles.Role]*hiveagent.Agent),
		trackers:      make(map[roles.Role]*resources.TrackingProvider),
		skipGuardian:  cfg.SkipGuardian,
		skipSimplify:  cfg.SkipSimplify,
		autoApprove:   cfg.AutoApprove,
		reviewerModel:  cfg.ReviewerModel,
		builderModel:   cfg.BuilderModel,
		ctoModel:       cfg.CTOModel,
		guardianModel:  cfg.GuardianModel,
		architectModel: cfg.ArchitectModel,
		resume:         cfg.Resume,
	}

	// Always initialize event infrastructure — needed for OBSERVABLE invariant
	// (all agent spawns emit authority events, even in no-gate dev mode).
	// Deterministic signer derived from humanID — stable across restarts,
	// verifiable against the human's identity. Random ephemeral keys would
	// break INTEGRITY (signature verification requires knowing the key).
	signer := deriveSignerFromID(cfg.HumanID)
	registry := event.DefaultRegistry()
	registerPipelineEvents(registry)
	work.RegisterWithRegistry(registry)
	mind.RegisterWithRegistry(registry)
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
		fmt.Fprintln(os.Stderr, "  ↳ Guardian: SKIPPED (--skip-guardian)")
		p.emitProgress("", "Guardian: SKIPPED (--skip-guardian)")
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
func (p *Pipeline) ensureAgent(ctx context.Context, role roles.Role, name string) (*hiveagent.Agent, error) {
	if a, ok := p.agents[role]; ok {
		return a, nil
	}

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
		// Spawner registered the actor and emitted lifecycle events.
		// Fall through to create the hiveagent.Agent below.
		_ = result.ActorID
	} else {
		// Direct creation — no approval gate (bootstrap or testing).
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

	// Create the unified hiveagent.Agent — handles actor registration, boot
	// events, state machine, and causality tracking internally.
	// All agents share the pipeline's conversation ID for unified threading.
	a, err := hiveagent.New(ctx, hiveagent.Config{
		Role:           hiveagent.Role(role),
		Name:           name,
		Graph:          p.graph,
		Provider:       tracker,
		Model:          model,
		CostTier:       "opus",
		SoulValues:     roles.SoulValues(role),
		ConversationID: p.convID,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent %s: %w", name, err)
	}
	fmt.Fprintf(os.Stderr, "  ↳ %s agent %s using %s\n", role, a.ID().Value(), model)
	p.emitAgentSpawned(string(role), a.ID().Value(), model)
	p.agents[role] = a
	return a, nil
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
// Uses CTO causality when available; falls back to store.Head().
func (p *Pipeline) emitAuthorityRequested(action string, justification string) (types.EventID, error) {
	var causeID types.EventID
	if p.cto != nil {
		causeID = p.cto.LastEvent()
	}
	if causeID.IsZero() {
		head, err := p.store.Head()
		if err != nil {
			return types.EventID{}, fmt.Errorf("store head: %w", err)
		}
		if !head.IsSome() {
			return types.EventID{}, fmt.Errorf("graph not bootstrapped")
		}
		causeID = head.Unwrap().ID()
	}

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

// ctoCause returns the CTO agent's last event ID for causality linking.
// Falls back to store.Head() if CTO hasn't emitted yet.
func (p *Pipeline) ctoCause() types.EventID {
	if p.cto != nil {
		if id := p.cto.LastEvent(); !id.IsZero() {
			return id
		}
	}
	head, err := p.store.Head()
	if err != nil || !head.IsSome() {
		return types.EventID{}
	}
	return head.Unwrap().ID()
}

// recordTrust records verified work for an agent after a successful phase.
// Uses the agent's own last event as evidence — not a global store query,
// which would credit the wrong agent in a multi-agent pipeline.
// Best-effort — logs a warning on failure, never blocks the pipeline.
func (p *Pipeline) recordTrust(ctx context.Context, a *hiveagent.Agent, phase string) {
	if p.trustModel == nil || a == nil {
		return
	}
	lastID := a.LastEvent()
	if lastID.IsZero() {
		return
	}
	ev, err := p.store.Get(lastID)
	if err != nil {
		return
	}
	if err := a.RecordVerifiedWork(ctx, p.trustModel, ev); err != nil {
		fmt.Fprintf(os.Stderr, "warning: trust update for %s after %s failed: %v\n", a.Role(), phase, err)
	}
}

// Store returns the shared event graph.
func (p *Pipeline) Store() store.Store {
	return p.store
}

// Agents returns all active agents.
func (p *Pipeline) Agents() map[roles.Role]*hiveagent.Agent {
	return p.agents
}
