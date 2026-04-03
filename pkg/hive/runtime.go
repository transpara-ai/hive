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
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/graph"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/lovyou-ai/agent"
	"github.com/lovyou-ai/hive/pkg/loop"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/work"
)

// Runtime is the hive runtime. It manages agents, the shared graph,
// the event bus, and the task store.
type Runtime struct {
	store   store.Store
	actors  actor.IActorStore
	graph   *graph.Graph
	humanID types.ActorID
	defs    []AgentDef

	// Event infrastructure.
	signer  event.Signer
	factory *event.EventFactory
	convID  types.ConversationID

	// Task store for agent coordination.
	tasks *work.TaskStore

	// Options.
	autoApprove bool
	repoPath    string
	keepalive   bool
}

// Config holds the configuration needed to create a Runtime.
type Config struct {
	Store       store.Store
	Actors      actor.IActorStore
	HumanID     types.ActorID
	AutoApprove bool   // --yes flag
	RepoPath    string // --repo flag (for Implementer's Operate)
	Keepalive   bool   // --keepalive flag: agents block on bus instead of quiescing
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

	// Create event factory for the task store.
	factory := event.NewEventFactory(registry)
	convID, err := newConversationID()
	if err != nil {
		return nil, fmt.Errorf("conversation ID: %w", err)
	}

	tasks := work.NewTaskStore(cfg.Store, factory, signer)

	return &Runtime{
		store:       cfg.Store,
		actors:      cfg.Actors,
		graph:       g,
		humanID:     cfg.HumanID,
		signer:      signer,
		factory:     factory,
		convID:      convID,
		tasks:       tasks,
		autoApprove: cfg.AutoApprove,
		repoPath:    cfg.RepoPath,
		keepalive:   cfg.Keepalive,
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

	// Build loop configs for all agents.
	configs := make([]loop.Config, 0, len(r.defs))
	for _, def := range r.defs {
		agent, err := r.spawnAgent(ctx, def)
		if err != nil {
			return fmt.Errorf("spawn %s: %w", def.Name, err)
		}

		cfg := loop.Config{
			Agent:   agent,
			HumanID: r.humanID,
			Budget: resources.BudgetConfig{
				MaxIterations: def.EffectiveMaxIterations(),
				MaxDuration:   def.EffectiveMaxDuration(),
			},
			Bus:  r.graph.Bus(),
			Task: seedIdea,

			// Task coordination.
			TaskStore:  r.tasks,
			ConvID:     r.convID,
			CanOperate: def.CanOperate,
			RepoPath:   r.repoPath,
			Keepalive:  r.keepalive,

			OnIteration: func(iteration int, response string) {
				fmt.Fprintf(os.Stderr, "[%s] iteration %d (%d chars)\n",
					def.Name, iteration, len(response))
			},
		}
		configs = append(configs, cfg)
	}

	fmt.Fprintf(os.Stderr, "\nStarting %d agents...\n", len(configs))

	// Run all agents concurrently.
	results := loop.RunConcurrent(ctx, configs)

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

// spawnAgent creates a hiveagent.Agent from an AgentDef.
func (r *Runtime) spawnAgent(ctx context.Context, def AgentDef) (*hiveagent.Agent, error) {
	// Create the intelligence provider.
	provider, err := intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        def.Model,
		SystemPrompt: def.SystemPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("provider: %w", err)
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
		return nil, fmt.Errorf("create agent: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  ↳ %s (%s) using %s [%s]\n",
		def.Name, def.Role, def.Model, agent.ID().Value())
	r.emit(EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    def.Name,
		Role:    def.Role,
		Model:   def.Model,
		ActorID: agent.ID().Value(),
	})

	return agent, nil
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

// newConversationID generates a unique conversation ID for this run.
func newConversationID() (types.ConversationID, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return types.ConversationID{}, err
	}
	return types.NewConversationID("conv_hive_" + hex.EncodeToString(b[:]))
}
