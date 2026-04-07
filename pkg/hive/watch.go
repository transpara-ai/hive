package hive

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/loop"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/telemetry"
)

const watchPollInterval = 5 * time.Second

var (
	roleApprovedType   = types.MustEventType("hive.role.approved")
	roleProposedType   = types.MustEventType("hive.role.proposed")
	budgetAdjustedType = types.MustEventType("agent.budget.adjusted")
)

// watchForApprovedRoles polls the event store for approved role proposals and
// spawns new agent goroutines as each role is both approved and budgeted.
// Runs as a goroutine alongside RunConcurrent() until ctx is cancelled.
func (r *Runtime) watchForApprovedRoles(ctx context.Context) {
	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processApprovedRoles(ctx)
		}
	}
}

// processApprovedRoles is the polling body of watchForApprovedRoles.
// Separated for testability with mock events.
func (r *Runtime) processApprovedRoles(ctx context.Context) {
	approvedPage, err := r.store.ByType(roleApprovedType, 100, types.None[types.Cursor]())
	if err != nil {
		fmt.Fprintf(os.Stderr, "watchForApprovedRoles: query approved roles: %v\n", err)
		return
	}

	for _, approvedEv := range approvedPage.Items() {
		content, ok := approvedEv.Content().(event.RoleApprovedContent)
		if !ok {
			continue
		}

		name := content.Name

		// Dedup: already spawned (or currently spawning) — skip.
		if r.dynamic.IsTracked(name) {
			continue
		}

		// Find the matching hive.role.proposed event.
		proposal, found := r.findRoleProposal(name)
		if !found {
			fmt.Fprintf(os.Stderr, "[watcher] approval found for %q but no matching proposal — skipping\n", name)
			continue
		}

		// Find an agent.budget.adjusted event with matching AgentName.
		// If missing, Allocator may not have budgeted yet — retry on next poll.
		budgetEv, found := r.findBudgetForRole(name)
		if !found {
			continue
		}

		if ctx.Err() != nil {
			return
		}

		if err := r.spawnDynamicAgent(ctx, proposal, budgetEv); err != nil {
			fmt.Fprintf(os.Stderr, "[watcher] failed to spawn %q: %v\n", name, err)
			// Don't track — allow retry on next poll.
		}
	}
}

// findRoleProposal searches the event store for a hive.role.proposed event
// with the given role name. Returns the content and true if found.
func (r *Runtime) findRoleProposal(name string) (event.RoleProposedContent, bool) {
	page, err := r.store.ByType(roleProposedType, 100, types.None[types.Cursor]())
	if err != nil {
		return event.RoleProposedContent{}, false
	}
	for _, ev := range page.Items() {
		c, ok := ev.Content().(event.RoleProposedContent)
		if ok && c.Name == name {
			return c, true
		}
	}
	return event.RoleProposedContent{}, false
}

// findBudgetForRole searches the event store for an agent.budget.adjusted event
// whose AgentName matches the given role name. The Allocator sets AgentName when
// assigning budget to a newly approved role.
func (r *Runtime) findBudgetForRole(name string) (event.AgentBudgetAdjustedContent, bool) {
	page, err := r.store.ByType(budgetAdjustedType, 100, types.None[types.Cursor]())
	if err != nil {
		return event.AgentBudgetAdjustedContent{}, false
	}
	for _, ev := range page.Items() {
		c, ok := ev.Content().(event.AgentBudgetAdjustedContent)
		if ok && c.AgentName == name {
			return c, true
		}
	}
	return event.AgentBudgetAdjustedContent{}, false
}

// spawnDynamicAgent creates, registers, and starts a Loop goroutine for a new
// agent derived from an approved role proposal + Allocator budget event.
func (r *Runtime) spawnDynamicAgent(ctx context.Context, proposal event.RoleProposedContent, budgetEv event.AgentBudgetAdjustedContent) error {
	// Allocator's NewBudget takes precedence; fall back to proposal's max_iterations.
	maxIter := proposal.MaxIterations
	if budgetEv.NewBudget > 0 {
		maxIter = budgetEv.NewBudget
	}

	def := AgentDef{
		Name:          proposal.Name,
		Role:          proposal.Name, // name == role for dynamically spawned agents
		Model:         mapModelName(proposal.Model),
		SystemPrompt:  proposal.Prompt,
		WatchPatterns: proposal.WatchPatterns,
		CanOperate:    false, // trust must be earned; always false for spawned agents
		MaxIterations: maxIter,
		MaxDuration:   0,
	}

	agent, err := r.spawnAgent(ctx, def)
	if err != nil {
		return fmt.Errorf("spawn agent: %w", err)
	}

	// Create budget and register with the shared BudgetRegistry.
	budgetCfg := resources.BudgetConfig{
		MaxIterations: def.EffectiveMaxIterations(),
		MaxDuration:   def.EffectiveMaxDuration(),
	}
	agentBudget := resources.NewBudget(budgetCfg)
	r.budgetRegistry.Register(def.Name, agentBudget, def.EffectiveMaxIterations())

	// Register with telemetry writer (if available).
	if r.telemetryWriter != nil {
		r.telemetryWriter.RegisterAgent(telemetry.AgentRegistration{
			Name:          def.Name,
			Role:          def.Role,
			Model:         def.Model,
			Agent:         agent,
			MaxIterations: def.EffectiveMaxIterations(),
			WatchPatterns: def.WatchPatterns,
			CanOperate:    def.CanOperate,
			Tier:          def.EffectiveTier(),
		})
	}

	cfg := loop.Config{
		Agent:          agent,
		HumanID:        r.humanID,
		Budget:         budgetCfg,
		BudgetInstance: agentBudget,
		BudgetRegistry: r.budgetRegistry,
		Bus:            r.graph.Bus(),
		Task:           "", // dynamic agents have no seed task

		TaskStore:      r.tasks,
		ConvID:         r.convID,
		CanOperate:     false,
		RepoPath:       r.repoPath,
		Keepalive:      r.keepalive,
		KnowledgeStore: r.knowledgeStore,
		ActorResolver: func(id types.ActorID) string {
			a, err := r.actors.Get(id)
			if err != nil {
				return ""
			}
			return a.DisplayName()
		},

		OnIteration: func(iteration int, response string) {
			fmt.Fprintf(os.Stderr, "[%s] iteration %d (%d chars)\n",
				def.Name, iteration, len(response))
			if r.telemetryWriter != nil {
				r.telemetryWriter.RecordResponse(def.Name, response)
			}
		},
	}

	// Track and start the goroutine. Track before wg.Add so IsTracked()
	// returns true immediately (dedup guard for the next poll cycle).
	agentCtx, cancel := context.WithCancel(ctx)
	r.dynamic.Track(def.Name, cancel)
	r.dynamic.wg.Add(1)
	go func() {
		defer r.dynamic.wg.Done()
		l, loopErr := loop.New(cfg)
		if loopErr != nil {
			fmt.Fprintf(os.Stderr, "[%s] loop init failed: %v\n", def.Name, loopErr)
			cancel()
			return
		}
		l.Run(agentCtx)
	}()

	fmt.Fprintf(os.Stderr, "dynamic agent spawned: %s (model=%s, maxIter=%d)\n",
		def.Name, def.Model, def.EffectiveMaxIterations())
	return nil
}
