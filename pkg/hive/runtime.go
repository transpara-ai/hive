// Package hive implements the agent-first hive runtime.
//
// The runtime manages agents that run in concurrent loops, communicate
// through the event graph, and coordinate through work tasks. Adding a
// new agent is: define an AgentDef, call runtime.Register().
package hive

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/hive/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/knowledge"
	"github.com/transpara-ai/hive/pkg/loop"
	"github.com/transpara-ai/hive/pkg/membrane"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/hive/pkg/telemetry"
	"github.com/transpara-ai/work"
)

// Runtime is the hive runtime. It manages agents, the shared graph,
// the event bus, and the task store.
type Runtime struct {
	store   store.Store
	actors  actor.IActorStore
	graph   *graph.Graph
	humanID types.ActorID
	defs         []AgentDef
	membraneDefs []membrane.MembraneConfig

	// Event infrastructure.
	signer  event.Signer
	factory *event.EventFactory
	convID  types.ConversationID

	// Task store for agent coordination.
	tasks *work.TaskStore

	// Budget registry for cross-agent budget visibility and mutation.
	budgetRegistry *resources.BudgetRegistry

	// Telemetry writer (optional, nil when no postgres available).
	telemetryWriter *telemetry.Writer

	// System actor for infrastructure events (knowledge, telemetry, etc.).
	systemID types.ActorID

	// Knowledge store for distilled insights (survives reboot via chain replay).
	knowledgeStore knowledge.KnowledgeStore

	// Model resolver for agent provider/model selection. Set once during Run()
	// and never swapped — closures that capture it are safe without synchronization.
	resolver *modelconfig.Resolver

	// Dynamic agent lifecycle tracker (agents spawned after boot).
	dynamic *dynamicAgentTracker

	// Bridge actor for synchronous site-op anchors. Constructed lazily on
	// first AnchorSiteOp call via bridgeOnce. bridgeMu serialises emit+read
	// pairs against LastEvent() so concurrent webhooks each observe their
	// own anchor ID.
	bridgeAgent *hiveagent.Agent
	bridgeOnce  sync.Once
	bridgeMu    sync.Mutex

	// Options.
	approveRequests bool
	approveRoles    bool
	repoPath        string
	loop            bool
	catalogPath     string
}

// Config holds the configuration needed to create a Runtime.
type Config struct {
	Store       store.Store
	Actors      actor.IActorStore
	HumanID     types.ActorID
	ApproveRequests bool   // --approve-requests: auto-approve authority requests
	ApproveRoles    bool   // --approve-roles: auto-approve role proposals
	RepoPath        string // --repo: path to repo for Operate
	Loop            bool   // --loop: agents block on bus instead of quiescing
	CatalogPath     string // --catalog: custom YAML catalog file (merged with defaults)

	// TelemetryWriter snapshots agent and hive state to postgres. Optional.
	TelemetryWriter *telemetry.Writer
}

// New creates a new hive Runtime.
func New(ctx context.Context, cfg Config) (*Runtime, error) {
	// Verify the human exists in the actor store.
	human, err := cfg.Actors.Get(cfg.HumanID)
	if err != nil {
		return nil, fmt.Errorf("human operator not found: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Human operator: %s (%s)\n", human.DisplayName(), human.ID().Value())

	// Create the shared graph facade.
	signer := deriveSignerFromID(cfg.HumanID)
	g := graph.New(cfg.Store, cfg.Actors, graph.WithSigner(signer))
	if err := g.Start(); err != nil {
		return nil, fmt.Errorf("start graph: %w", err)
	}

	// Register event types.
	registry := g.Registry()
	RegisterWithRegistry(registry)
	work.RegisterWithRegistry(registry)

	// Register the system actor for infrastructure events.
	systemID, err := registerSystemActor(cfg.Actors)
	if err != nil {
		return nil, fmt.Errorf("register system actor: %w", err)
	}

	// Create event factory for the task store.
	factory := event.NewEventFactory(registry)
	convID, err := newConversationID()
	if err != nil {
		return nil, fmt.Errorf("conversation ID: %w", err)
	}

	tasks := work.NewTaskStore(cfg.Store, factory, signer)

	return &Runtime{
		store:           cfg.Store,
		actors:          cfg.Actors,
		graph:           g,
		humanID:         cfg.HumanID,
		systemID:        systemID,
		signer:          signer,
		factory:         factory,
		convID:          convID,
		tasks:           tasks,
		approveRequests: cfg.ApproveRequests,
		approveRoles:    cfg.ApproveRoles,
		repoPath:        cfg.RepoPath,
		loop:            cfg.Loop,
		catalogPath:     cfg.CatalogPath,
		telemetryWriter: cfg.TelemetryWriter,
	}, nil
}

// Register adds an agent definition to the runtime.
// Agents are created and started when Run() is called.
func (r *Runtime) Register(def AgentDef) error {
	if err := def.Validate(); err != nil {
		return err
	}
	r.defs = append(r.defs, def)
	return nil
}

// RegisterMembrane registers a membrane agent definition.
// Membrane agents wrap external services and run their own poll-based loop.
func (r *Runtime) RegisterMembrane(cfg membrane.MembraneConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	r.membraneDefs = append(r.membraneDefs, cfg)
	return nil
}

// Run creates all registered agents and runs their loops concurrently.
// It returns when all agents stop.
func (r *Runtime) Run(ctx context.Context, seedIdea string) error {
	if len(r.defs) == 0 {
		return fmt.Errorf("no agents registered")
	}

	start := time.Now()

	// Emit run started event.
	r.emit(EventTypeRunStarted, RunStartedContent{
		Idea:     seedIdea,
		RepoPath: r.repoPath,
	})

	// Create a seed task from the idea if provided.
	if seedIdea != "" {
		head, err := r.store.Head()
		if err != nil {
			return fmt.Errorf("store head: %w", err)
		}
		var causes []types.EventID
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		}
		task, err := r.tasks.Create(r.humanID, "Seed: "+seedIdea, seedIdea, causes, r.convID)
		if err != nil {
			return fmt.Errorf("create seed task: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Seed task: %s\n", task.ID.Value())
	}

	// Create the budget registry for cross-agent visibility.
	r.budgetRegistry = resources.NewBudgetRegistry()

	// Create the dynamic agent tracker (manages post-boot spawned agents).
	r.dynamic = newDynamicAgentTracker()

	// Initialize model resolver for agent spawning.
	if r.catalogPath != "" {
		var err error
		r.resolver, err = modelconfig.ResolverFromCatalogFile(r.catalogPath)
		if err != nil {
			return fmt.Errorf("custom catalog: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Model catalog: %s (merged with defaults)\n", r.catalogPath)
	} else {
		r.resolver = modelconfig.DefaultResolver()
	}

	// Wire budget registry into telemetry writer now that it exists.
	if r.telemetryWriter != nil {
		r.telemetryWriter.SetBudgetRegistry(r.budgetRegistry)
	}

	// Create knowledge store and replay state from the event chain.
	r.knowledgeStore = knowledge.NewStore()
	if err := knowledge.ReplayFromStore(r.store, r.knowledgeStore); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: knowledge replay: %v (starting with empty store)\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Knowledge: replayed %d active insights from chain\n", r.knowledgeStore.ActiveCount())
	}
	go knowledge.RunPruner(ctx, r.knowledgeStore, 15*time.Minute)

	// Subscribe to live knowledge events on the bus.
	knowledgePattern := types.MustSubscriptionPattern("knowledge.*")
	r.graph.Bus().Subscribe(knowledgePattern, func(ev event.Event) {
		switch c := ev.Content().(type) {
		case event.KnowledgeInsightContent:
			insight := knowledge.ConvertFromEventContent(c, ev.Timestamp().Value())
			_ = r.knowledgeStore.Record(insight)
			if c.SupersedesID.IsSome() {
				_ = r.knowledgeStore.Supersede(c.SupersedesID.Unwrap(), c.InsightID)
			}
		case event.KnowledgeSupersessionContent:
			_ = r.knowledgeStore.Supersede(c.OldInsightID, c.NewInsightID)
		case event.KnowledgeExpirationContent:
			_ = r.knowledgeStore.Expire(c.InsightID)
		}
	})

	// Start knowledge distiller with system actor.
	systemSigner := deriveSignerFromID(r.systemID)
	emitter := &systemEmitter{
		store:   r.store,
		factory: r.factory,
		signer:  systemSigner,
		actorID: r.systemID,
		convID:  r.convID,
	}
	distiller := knowledge.NewDistiller(r.store, r.knowledgeStore, emitter, 5*time.Minute)
	go distiller.Run(ctx)
	fmt.Fprintf(os.Stderr, "Knowledge: distiller started (5m interval)\n")

	// --- Checkpoint recovery ---
	var thoughtStore checkpoint.ThoughtStore
	var recoveryStates map[string]*checkpoint.RecoveryState

	openBrainURL := os.Getenv("OPEN_BRAIN_URL")
	openBrainKey := os.Getenv("OPEN_BRAIN_KEY")
	if openBrainURL != "" {
		thoughtStore = checkpoint.NewOpenBrainClient(openBrainURL, openBrainKey)
	}

	staleness := 2 * time.Hour
	if s := os.Getenv("CHECKPOINT_STALENESS"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			staleness = d
		}
	}

	heartbeatInterval := 10
	if s := os.Getenv("CHECKPOINT_HEARTBEAT_INTERVAL"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			heartbeatInterval = n
		}
	}

	// Collect role names for recovery — starters + any dynamically spawned
	// agents discovered from hive.role.approved events on the chain.
	var roleNames []string
	for _, def := range r.defs {
		roleNames = append(roleNames, def.Name)
	}
	dynamicNames, dynErr := checkpoint.ReplayDynamicAgentsFromStore(r.store)
	if dynErr != nil {
		fmt.Fprintf(os.Stderr, "WARNING: dynamic agent discovery: %v\n", dynErr)
	}
	starterSet := make(map[string]bool, len(roleNames))
	for _, n := range roleNames {
		starterSet[n] = true
	}
	for _, n := range dynamicNames {
		if !starterSet[n] {
			roleNames = append(roleNames, n)
		}
	}

	recoveryStates, recoverErr := checkpoint.RecoverAll(roleNames, thoughtStore, r.store, staleness)
	if recoverErr != nil {
		fmt.Fprintf(os.Stderr, "WARNING: checkpoint recovery: %v\n", recoverErr)
	}

	// Log recovery summary.
	warmCount := 0
	for _, rs := range recoveryStates {
		if rs != nil && rs.Mode == checkpoint.ModeWarm {
			warmCount++
		}
	}
	if len(recoveryStates) > 0 {
		fmt.Fprintf(os.Stderr, "Checkpoint: %d/%d agents warm-started\n", warmCount, len(recoveryStates))
	}

	if r.telemetryWriter != nil {
		for role, rs := range recoveryStates {
			survival := checkpoint.SurvivalRoleOnly
			if rs != nil && rs.Mode == checkpoint.ModeWarm {
				survival = checkpoint.SurvivalFull
			}
			r.telemetryWriter.UpdateRebootSurvival(role, string(survival))
		}
	}

	// Build loop configs for all agents.
	configs := make([]loop.Config, 0, len(r.defs))
	for _, def := range r.defs {
		agent, resolvedModel, err := r.spawnAgent(ctx, def)
		if err != nil {
			return fmt.Errorf("spawn %s: %w", def.Name, err)
		}

		// Create the budget tracker and register it for cross-agent visibility.
		budgetCfg := resources.BudgetConfig{
			MaxIterations: def.EffectiveMaxIterations(),
			MaxDuration:   def.EffectiveMaxDuration(),
		}
		agentBudget := resources.NewBudget(budgetCfg)
		r.budgetRegistry.Register(def.Name, agentBudget, def.EffectiveMaxIterations(), resolvedModel)

		// Register agent with telemetry writer.
		if r.telemetryWriter != nil {
			r.telemetryWriter.RegisterAgent(telemetry.AgentRegistration{
				Name:          def.Name,
				Role:          def.Role,
				Model:         resolvedModel,
				Agent:         agent,
				MaxIterations: def.EffectiveMaxIterations(),
				WatchPatterns: def.WatchPatterns,
				CanOperate:    def.CanOperate,
				Tier:          def.EffectiveTier(),
				Origin:        "bootstrap",
			})
		}

		cfg := loop.Config{
			Agent:          agent,
			HumanID:        r.humanID,
			Budget:         budgetCfg,
			BudgetInstance: agentBudget,
			BudgetRegistry: r.budgetRegistry,
			Bus:            r.graph.Bus(),
			Task:           seedIdea,

			// Task coordination.
			TaskStore:      r.tasks,
			ConvID:         r.convID,
			CanOperate:     def.CanOperate,
			RepoPath:       r.repoPath,
			Keepalive:      r.loop,
			KnowledgeStore: r.knowledgeStore,
			CostSummaryFunc: func() string {
				entries := r.budgetRegistry.Snapshot()
				agents := make([]modelconfig.AgentModelEntry, 0, len(entries))
				for _, e := range entries {
					agents = append(agents, modelconfig.AgentModelEntry{
						Agent: e.Name,
						Model: e.ResolvedModel,
					})
				}
				summaries := modelconfig.EstimateAgentCostsByModel(r.resolver.Catalog(), agents, 10_000, 2_000)
				return modelconfig.FormatCostSummary(summaries)
			},
			Catalog:        r.resolver.Catalog(),
			ActorResolver: func(id types.ActorID) string {
				a, err := r.actors.Get(id)
				if err != nil {
					return ""
				}
				return a.DisplayName()
			},

			// Checkpoint recovery and sink.
			RecoveryState:     recoveryStates[def.Name],
			Sink:              buildCheckpointSink(thoughtStore, def.Name),
			HeartbeatInterval: heartbeatInterval,

			OnIteration: func(iteration int, response string) {
				fmt.Fprintf(os.Stderr, "[%s] iteration %d (%d chars)\n",
					def.Name, iteration, len(response))
				if r.telemetryWriter != nil {
					r.telemetryWriter.RecordResponse(def.Name, response)
				}
			},
		}
		configs = append(configs, cfg)
	}

	fmt.Fprintf(os.Stderr, "\nStarting %d agents...\n", len(configs))

	// Start telemetry writer and event stream capture before agents run.
	if r.telemetryWriter != nil {
		go r.telemetryWriter.Start(ctx)
		r.telemetryWriter.SubscribeToBus(r.graph.Bus())
		fmt.Fprintf(os.Stderr, "Telemetry: writer started (%d agents registered)\n", r.telemetryWriter.Agents())
	}

	// TODO: wire hive summary capture trigger here (bus subscription for run-boundary events).

	// Watch for approved role proposals and spawn new agents mid-session.
	go r.watchForApprovedRoles(ctx)

	// Run all bootstrap agents concurrently.
	results := loop.RunConcurrent(ctx, configs)

	// Wait for any dynamically spawned agents to finish.
	r.dynamic.Wait()

	// Report results and emit stop events.
	fmt.Fprintf(os.Stderr, "\n── Results ──\n")
	for _, ar := range results {
		fmt.Fprintf(os.Stderr, "  %s (%s): %s after %d iterations — %s\n",
			ar.Name, ar.Role, ar.Result.Reason, ar.Result.Iterations, ar.Result.Detail)
		r.emit(EventTypeAgentStopped, AgentStoppedContent{
			Name:       ar.Name,
			Role:       ar.Role,
			StopReason: string(ar.Result.Reason),
			Iterations: ar.Result.Iterations,
			Detail:     ar.Result.Detail,
		})
	}

	dur := time.Since(start)
	r.emit(EventTypeRunCompleted, RunCompletedContent{
		AgentCount: len(r.defs),
		DurationMs: dur.Milliseconds(),
	})

	fmt.Fprintf(os.Stderr, "\nCompleted in %s\n", dur.Round(time.Second))
	return nil
}

// buildCheckpointSink constructs a CheckpointSink for an agent.
// Returns nil when thoughtStore is nil (disables checkpointing gracefully).
// The heartbeat emitter is nil here — the loop wires it in Run() where it
// has access to the agent's event emission path.
func buildCheckpointSink(thoughts checkpoint.ThoughtStore, role string) checkpoint.CheckpointSink {
	if thoughts == nil {
		return nil
	}
	return checkpoint.NewDefaultSink(thoughts, nil, role)
}

// spawnAgent creates a hiveagent.Agent from an AgentDef.
// It returns the agent and the resolved model name so callers can pass it to telemetry.
func (r *Runtime) spawnAgent(ctx context.Context, def AgentDef) (*hiveagent.Agent, string, error) {
	// Resolve model/provider through the precedence chain.
	input := modelconfig.ResolutionInput{
		Role:          def.Role,
		AgentDefModel: def.Model,
		Policy:        def.EffectiveModelPolicy(),
		CanOperate:    def.CanOperate,
	}
	resolved, err := r.resolver.Resolve(input)
	if err != nil {
		return nil, "", fmt.Errorf("resolve model for %s: %w", def.Name, err)
	}
	cfg := modelconfig.ToIntelligenceConfig(resolved, def.SystemPrompt)
	provider, err := intelligence.New(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("provider: %w", err)
	}

	// Wrap in tracking provider for token accounting.
	tracker := resources.NewTrackingProvider(provider)

	// Create the unified hiveagent.Agent.
	agent, err := hiveagent.New(ctx, hiveagent.Config{
		Role:           hiveagent.Role(def.Role),
		Name:           def.Name,
		Graph:          r.graph,
		Provider:       tracker,
		ConversationID: r.convID,
	})
	if err != nil {
		return nil, "", fmt.Errorf("create agent: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  ↳ %s (%s) using %s/%s [%s] [%s]\n",
		def.Name, def.Role, resolved.Provider, resolved.Model, resolved.AuthMode, agent.ID().Value())
	r.emit(EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    def.Name,
		Role:    def.Role,
		Model:   resolved.Model,
		ActorID: agent.ID().Value(),
	})

	// Emit role definition as a first-class event (queryable, versionable).
	if def.RoleDefinition != nil {
		origin := "spawned"
		if def.RoleDefinition.Tier != "" {
			origin = "bootstrap" // bootstrap agents always have RoleDefinition.Tier set
		}
		r.emit(EventTypeRoleDefinition, RoleDefinitionContent{
			Name:        def.RoleDefinition.Name,
			Description: def.RoleDefinition.Description,
			Category:    def.RoleDefinition.Category,
			Tier:        def.RoleDefinition.Tier,
			CanOperate:  def.RoleDefinition.CanOperate,
			Origin:      origin,
		})
	}

	return agent, resolved.Model, nil
}

// emit appends a hive event to the graph. Best-effort.
func (r *Runtime) emit(eventType types.EventType, content event.EventContent) {
	var causeID types.EventID
	head, err := r.store.Head()
	if err != nil || !head.IsSome() {
		return
	}
	causeID = head.Unwrap().ID()

	ev, err := r.factory.Create(eventType, r.humanID, content, []types.EventID{causeID}, r.convID, r.store, r.signer)
	if err != nil {
		return
	}
	_, _ = r.store.Append(ev)
}

// ────────────────────────────────────────────────────────────────────
// Helpers ported from pipeline.go
// ────────────────────────────────────────────────────────────────────

// ed25519Signer implements event.Signer.
type ed25519Signer struct {
	key ed25519.PrivateKey
}

func (s *ed25519Signer) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

// deriveSignerFromID creates a deterministic Ed25519 signer from an ActorID.
func deriveSignerFromID(id types.ActorID) *ed25519Signer {
	h := sha256.Sum256([]byte("signer:" + id.Value()))
	priv := ed25519.NewKeyFromSeed(h[:])
	return &ed25519Signer{key: priv}
}

// registerSystemActor registers a deterministic system actor for infrastructure
// events. Idempotent — returns the existing actor on reboot.
func registerSystemActor(actors actor.IActorStore) (types.ActorID, error) {
	h := sha256.Sum256([]byte("system:hive"))
	priv := ed25519.NewKeyFromSeed(h[:])
	pub := priv.Public().(ed25519.PublicKey)

	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		return types.ActorID{}, fmt.Errorf("public key: %w", err)
	}

	a, err := actors.Register(pk, "system", event.ActorTypeSystem)
	if err != nil {
		return types.ActorID{}, err
	}
	return a.ID(), nil
}

// SystemID returns the system actor's ID.
func (r *Runtime) SystemID() types.ActorID {
	return r.systemID
}

// KnowledgeStore returns the runtime's knowledge store.
func (r *Runtime) KnowledgeStore() knowledge.KnowledgeStore {
	return r.knowledgeStore
}

// systemEmitter implements knowledge.EventEmitter using the system actor.
type systemEmitter struct {
	store   store.Store
	factory *event.EventFactory
	signer  event.Signer
	actorID types.ActorID
	convID  types.ConversationID
}

func (e *systemEmitter) Emit(eventType types.EventType, content event.EventContent) error {
	head, err := e.store.Head()
	if err != nil || !head.IsSome() {
		return fmt.Errorf("no chain head")
	}
	causeID := head.Unwrap().ID()

	ev, err := e.factory.Create(eventType, e.actorID, content, []types.EventID{causeID}, e.convID, e.store, e.signer)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	_, err = e.store.Append(ev)
	return err
}

// newConversationID generates a unique conversation ID for this run.
func newConversationID() (types.ConversationID, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return types.ConversationID{}, err
	}
	return types.NewConversationID("conv_hive_" + hex.EncodeToString(b[:]))
}
