package hive

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/loop"
	"github.com/transpara-ai/hive/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/hive/pkg/telemetry"
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
			if r.approveRoles {
				r.autoApproveProposedRoles(ctx)
			}
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

// autoApproveProposedRoles scans for hive.role.proposed events that have no
// matching hive.role.approved event, and emits both approval and budget events
// automatically. Called when --approve-roles is set.
func (r *Runtime) autoApproveProposedRoles(_ context.Context) {
	proposedPage, err := r.store.ByType(roleProposedType, 100, types.None[types.Cursor]())
	if err != nil {
		return
	}

	for _, ev := range proposedPage.Items() {
		proposal, ok := ev.Content().(event.RoleProposedContent)
		if !ok {
			continue
		}

		name := proposal.Name

		// Already tracked (spawned or spawning) — skip.
		if r.dynamic.IsTracked(name) {
			continue
		}

		// Already approved — skip (processApprovedRoles will handle it).
		if _, found := r.findApproval(name); found {
			continue
		}

		// Emit approval.
		approvalContent := event.RoleApprovedContent{
			Name:       name,
			ApprovedBy: "auto",
			Reason:     "auto-approved via --approve-roles",
		}
		head, err := r.store.Head()
		if err != nil {
			continue
		}
		var causes []types.EventID
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		}

		approvalEv, err := r.graph.Record(
			roleApprovedType,
			r.humanID,
			approvalContent,
			causes,
			r.convID,
			r.signer,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[watcher] auto-approve failed for %q: %v\n", name, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[watcher] auto-approved role %q\n", name)

		// Emit budget.
		maxIter := proposal.MaxIterations
		if maxIter == 0 {
			maxIter = 200
		}
		budgetContent := event.AgentBudgetAdjustedContent{
			AgentID:   r.humanID,
			AgentName: name,
			Action:    "set",
			NewBudget: maxIter,
			Reason:    "auto-budgeted via --approve-roles",
		}

		if _, err := r.graph.Record(
			budgetAdjustedType,
			r.humanID,
			budgetContent,
			[]types.EventID{approvalEv.ID()},
			r.convID,
			r.signer,
		); err != nil {
			fmt.Fprintf(os.Stderr, "[watcher] auto-budget failed for %q: %v\n", name, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[watcher] auto-budgeted role %q (maxIter=%d)\n", name, maxIter)
	}
}

// findApproval searches for a hive.role.approved event with the given name.
func (r *Runtime) findApproval(name string) (event.RoleApprovedContent, bool) {
	page, err := r.store.ByType(roleApprovedType, 100, types.None[types.Cursor]())
	if err != nil {
		return event.RoleApprovedContent{}, false
	}
	for _, ev := range page.Items() {
		c, ok := ev.Content().(event.RoleApprovedContent)
		if ok && c.Name == name {
			return c, true
		}
	}
	return event.RoleApprovedContent{}, false
}

// spawnDynamicAgent creates, registers, and starts a Loop goroutine for a new
// agent derived from an approved role proposal + Allocator budget event.
func (r *Runtime) spawnDynamicAgent(ctx context.Context, proposal event.RoleProposedContent, budgetEv event.AgentBudgetAdjustedContent) error {
	// Allocator's NewBudget takes precedence; fall back to proposal's max_iterations.
	maxIter := proposal.MaxIterations
	if budgetEv.NewBudget > 0 {
		maxIter = budgetEv.NewBudget
	}

	// Inject output convention for non-operate agents. The Spawner's prompt
	// tells it to include this, but LLMs forget — enforce it structurally.
	prompt := proposal.Prompt + nonOperateOutputConvention

	modelID, err := mapModelName(proposal.Model, r.resolver.Catalog())
	if err != nil {
		return fmt.Errorf("resolve model for %s: %w", proposal.Name, err)
	}

	def := AgentDef{
		Name:          proposal.Name,
		Role:          proposal.Name, // name == role for dynamically spawned agents
		Model:         modelID,
		SystemPrompt:  prompt,
		WatchPatterns: proposal.WatchPatterns,
		CanOperate:    false, // trust must be earned; always false for spawned agents
		MaxIterations: maxIter,
		MaxDuration:   0,
		RoleDefinition: &modelconfig.RoleDefinition{
			Name:        proposal.Name,
			Description: proposal.Reason,
			Category:    "spawned",
			CanOperate:  false,
		},
	}

	agent, resolvedModel, err := r.spawnAgent(ctx, def)
	if err != nil {
		return fmt.Errorf("spawn agent: %w", err)
	}

	// Create budget and register with the shared BudgetRegistry.
	budgetCfg := resources.BudgetConfig{
		MaxIterations: def.EffectiveMaxIterations(),
		MaxDuration:   def.EffectiveMaxDuration(),
	}
	agentBudget := resources.NewBudget(budgetCfg)
	r.budgetRegistry.Register(def.Name, agentBudget, def.EffectiveMaxIterations(), resolvedModel)

	// Register with telemetry writer (if available).
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
			Origin:        "spawned",
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

		TaskStore:       r.tasks,
		PhaseGateStore:  r.phaseGates,
		ConvID:          r.convID,
		OnTaskCompleted: r.mirrorTaskCompletion,
		CanOperate:      false,
		RepoPath:        r.repoPath,
		Keepalive:       r.loop,
		KnowledgeStore:  r.knowledgeStore,
		Catalog:         r.resolver.Catalog(),
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
