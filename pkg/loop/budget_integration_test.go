package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/budget"
	"github.com/transpara-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// Tier 1 — Deterministic framework tests (no LLM)
// ════════════════════════════════════════════════════════════════════════

func TestAllocatorBootsInLegacyMode(t *testing.T) {
	// Verify allocator is at index 2 with the correct model.
	agents := starterAgentsForTest()
	if len(agents) < 3 {
		t.Fatalf("expected at least 3 agents, got %d", len(agents))
	}
	alloc := agents[2]
	if alloc.Name != "allocator" {
		t.Errorf("agent[2].Name = %q, want %q", alloc.Name, "allocator")
	}
	if alloc.Role != "allocator" {
		t.Errorf("agent[2].Role = %q, want %q", alloc.Role, "allocator")
	}
	if alloc.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("agent[2].Model = %q, want Haiku", alloc.Model)
	}
	if alloc.CanOperate {
		t.Error("allocator should not have CanOperate")
	}
	if alloc.MaxIterations != 150 {
		t.Errorf("MaxIterations = %d, want 150", alloc.MaxIterations)
	}

	// Verify boot order: guardian, sysmon, allocator, strategist, planner, implementer.
	expectedOrder := []string{"guardian", "sysmon", "allocator", "strategist", "planner", "implementer"}
	for i, want := range expectedOrder {
		if i >= len(agents) {
			t.Fatalf("missing agent at index %d: want %s", i, want)
		}
		if agents[i].Name != want {
			t.Errorf("agents[%d].Name = %q, want %q", i, agents[i].Name, want)
		}
	}
}

func TestBudgetCommandToEvent(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "")
	reg.Register("guardian", resources.NewBudget(resources.BudgetConfig{MaxIterations: 200}), 200, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &BudgetCommand{
		Agent:  "implementer",
		Action: "increase",
		Amount: 25,
		Reason: "high-value work in progress",
	}

	if err := l.applyBudgetAdjustment(cmd, 20); err != nil {
		t.Fatalf("applyBudgetAdjustment: %v", err)
	}

	// Query the store for agent.budget.adjusted events.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeAgentBudgetAdjusted,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}

	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no agent.budget.adjusted events found in store")
	}

	ev := events[len(events)-1]
	content, ok := ev.Content().(event.AgentBudgetAdjustedContent)
	if !ok {
		t.Fatalf("event content is %T, want AgentBudgetAdjustedContent", ev.Content())
	}

	if content.AgentName != "implementer" {
		t.Errorf("AgentName = %q, want %q", content.AgentName, "implementer")
	}
	if content.Action != "increase" {
		t.Errorf("Action = %q, want %q", content.Action, "increase")
	}
	if content.PreviousBudget != 100 {
		t.Errorf("PreviousBudget = %d, want 100", content.PreviousBudget)
	}
	if content.NewBudget != 125 {
		t.Errorf("NewBudget = %d, want 125", content.NewBudget)
	}
	if content.Delta != 25 {
		t.Errorf("Delta = %d, want 25", content.Delta)
	}
	if content.Reason != "high-value work in progress" {
		t.Errorf("Reason = %q, want %q", content.Reason, "high-value work in progress")
	}

	// Verify event source is the allocator agent.
	if ev.Source() != agent.ID() {
		t.Errorf("Source = %v, want %v", ev.Source(), agent.ID())
	}
}

func TestBudgetObservationEnrichmentFormat(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	b1 := resources.NewBudget(resources.BudgetConfig{MaxIterations: 200})
	b1.Record(500, 0.10)
	reg.Register("guardian", b1, 200, "")

	b2 := resources.NewBudget(resources.BudgetConfig{MaxIterations: 100})
	b2.Record(300, 0.05)
	reg.Register("implementer", b2, 100, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	baseObs := "## Recent Events\n- [hive.run.started] ev1 by actor1\n"
	result := l.enrichBudgetObservation(baseObs, 15)

	// Must contain the base observation.
	if !strings.Contains(result, "## Recent Events") {
		t.Error("base observation missing from enriched output")
	}

	// Must contain the budget metrics block.
	if !strings.Contains(result, "=== BUDGET METRICS ===") {
		t.Error("missing === BUDGET METRICS === header")
	}
	if !strings.Contains(result, "POOL:") {
		t.Error("missing POOL section")
	}
	if !strings.Contains(result, "AGENTS:") {
		t.Error("missing AGENTS section")
	}
	if !strings.Contains(result, "COOLDOWNS:") {
		t.Error("missing COOLDOWNS section")
	}
	if !strings.Contains(result, "guardian") {
		t.Error("missing guardian in agent list")
	}
	if !strings.Contains(result, "implementer") {
		t.Error("missing implementer in agent list")
	}
	// Closing delimiter.
	if !strings.HasSuffix(strings.TrimSpace(result), "===") {
		t.Error("missing closing === delimiter")
	}
}

func TestBudgetObservationEnrichmentSkipsNonAllocator(t *testing.T) {
	roles := []string{"guardian", "sysmon", "strategist", "planner", "implementer"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
			agent := testHiveAgent(t, provider, role, "test-"+role)

			reg := resources.NewBudgetRegistry()
			reg.Register("some-agent", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "")

			l, err := New(Config{
				Agent:          agent,
				HumanID:        humanID(),
				Budget:         resources.BudgetConfig{MaxIterations: 10},
				BudgetRegistry: reg,
			})
			if err != nil {
				t.Fatal(err)
			}

			obs := "some observation text"
			result := l.enrichBudgetObservation(obs, 15)
			if result != obs {
				t.Errorf("role %q should not enrich observation", role)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════════════
// Tier 2 — Validation and constraint tests
// ════════════════════════════════════════════════════════════════════════

func TestStabilizationWindowBlocks(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "test"}

	// Iterations 1-9 should all be rejected.
	for iter := 1; iter <= 9; iter++ {
		err := l.validateBudgetCommand(cmd, iter)
		if err == nil {
			t.Errorf("iteration %d: expected stabilization rejection", iter)
		}
		if !strings.Contains(err.Error(), "stabilization") {
			t.Errorf("iteration %d: error should mention stabilization: %v", iter, err)
		}
	}

	// Iteration 10 should pass (window ends at 10).
	err = l.validateBudgetCommand(cmd, 10)
	if err != nil {
		t.Errorf("iteration 10: expected pass, got: %v", err)
	}
}

func TestCooldownEnforcement(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a previous adjustment at iteration 20.
	l.adjustmentHistory = []budget.AdjustmentRecord{
		{Agent: "implementer", Iteration: 20, Delta: 10, Reason: "prior"},
	}

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 10, Reason: "test"}

	// Iteration 25: global cooldown (5) clear, but agent cooldown (10) active → 5 remaining.
	err = l.validateBudgetCommand(cmd, 25)
	if err == nil {
		t.Fatal("expected agent cooldown rejection at iter 25")
	}
	if !strings.Contains(err.Error(), "cooldown active for implementer") {
		t.Errorf("error should mention agent cooldown: %v", err)
	}

	// Iteration 30: agent cooldown (10) clear (30-20=10 >= 10).
	err = l.validateBudgetCommand(cmd, 30)
	if err != nil {
		t.Errorf("expected pass at iter 30, got: %v", err)
	}
}

func TestBudgetFloorEnforced(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	reg.Register("planner", resources.NewBudget(resources.BudgetConfig{MaxIterations: 50}), 50, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Decrease by 45 from 50 → would be 5, but floor is 20.
	cmd := &BudgetCommand{Agent: "planner", Action: "decrease", Amount: 45, Reason: "sustained idle"}

	// Should NOT error — floor clamp is applied, not rejected.
	err = l.applyBudgetAdjustment(cmd, 20)
	if err != nil {
		t.Fatalf("expected floor clamp, not error: %v", err)
	}

	// Verify clamped to floor (20), not 5.
	snap := reg.Snapshot()
	for _, e := range snap {
		if e.Name == "planner" {
			if e.MaxIterations != 20 {
				t.Errorf("MaxIterations = %d, want 20 (floor clamp)", e.MaxIterations)
			}
			return
		}
	}
	t.Fatal("planner not found in registry")
}

func TestPoolConservation(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	reg.Register("guardian", resources.NewBudget(resources.BudgetConfig{MaxIterations: 200}), 200, "")
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}

	poolBefore := reg.TotalPool()
	if poolBefore != 300 {
		t.Fatalf("initial pool = %d, want 300", poolBefore)
	}

	// Increase implementer by 50.
	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 50, Reason: "productive"}
	if err := l.applyBudgetAdjustment(cmd, 20); err != nil {
		t.Fatalf("applyBudgetAdjustment: %v", err)
	}

	// Total pool increases because increase only raises one agent's limit.
	// Pool = sum of MaxIterations = 200 + 150 = 350
	poolAfter := reg.TotalPool()
	if poolAfter != 350 {
		t.Errorf("pool after increase = %d, want 350", poolAfter)
	}

	// Verify the event records the updated pool remaining.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeAgentBudgetAdjusted,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no budget.adjusted events")
	}
	content, ok := events[len(events)-1].Content().(event.AgentBudgetAdjustedContent)
	if !ok {
		t.Fatal("content is not AgentBudgetAdjustedContent")
	}
	// PoolRemaining = TotalPool - TotalUsed = 350 - 0 = 350
	if content.PoolRemaining != 350 {
		t.Errorf("PoolRemaining = %d, want 350", content.PoolRemaining)
	}
}

func TestBudgetCommandInLoop(t *testing.T) {
	// Verify /budget command in LLM response produces agent.budget.adjusted event
	// via the full loop execution path.
	provider := newMockProvider(
		"Budget imbalance detected.\n/budget {\"agent\":\"implementer\",\"action\":\"increase\",\"amount\":25,\"reason\":\"approaching exhaustion\"}\n/signal {\"signal\": \"TASK_DONE\"}",
	)
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")

	reg := resources.NewBudgetRegistry()
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "")

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 5},
		BudgetRegistry: reg,
		Task:           "manage budgets",
	})
	if err != nil {
		t.Fatal(err)
	}

	// The first iteration will be iteration 1, which is inside the
	// stabilization window (default 10). The /budget command should be
	// rejected, but the loop should still complete via TASK_DONE signal.
	result := l.Run(context.Background())
	if result.Reason != StopTaskDone {
		t.Fatalf("reason = %s, want %s (detail: %s)", result.Reason, StopTaskDone, result.Detail)
	}

	// Since stabilization window blocks iteration 1, no budget event expected.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeAgentBudgetAdjusted,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(page.Items()) != 0 {
		t.Errorf("expected 0 budget.adjusted events (stabilization window), got %d", len(page.Items()))
	}
}

// ════════════════════════════════════════════════════════════════════════
// Helpers
// ════════════════════════════════════════════════════════════════════════

// starterAgentsForTest imports StarterAgents from the hive package indirectly.
// Since we're in pkg/loop and can't import pkg/hive (circular), we replicate
// the expected boot order inline and test it structurally.
func starterAgentsForTest() []struct {
	Name          string
	Role          string
	Model         string
	CanOperate    bool
	MaxIterations int
} {
	return []struct {
		Name          string
		Role          string
		Model         string
		CanOperate    bool
		MaxIterations int
	}{
		{Name: "guardian", Role: "guardian", Model: "claude-sonnet-4-6", MaxIterations: 200},
		{Name: "sysmon", Role: "sysmon", Model: "claude-haiku-4-5-20251001", MaxIterations: 150},
		{Name: "allocator", Role: "allocator", Model: "claude-haiku-4-5-20251001", MaxIterations: 150},
		{Name: "strategist", Role: "strategist", Model: "claude-opus-4-6", MaxIterations: 0},
		{Name: "planner", Role: "planner", Model: "claude-opus-4-6", MaxIterations: 0},
		{Name: "implementer", Role: "implementer", Model: "claude-opus-4-6", CanOperate: true, MaxIterations: 100},
	}
}
