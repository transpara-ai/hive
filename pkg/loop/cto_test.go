package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/work"
)

// ════════════════════════════════════════════════════════════════════════
// parseGapCommand
// ════════════════════════════════════════════════════════════════════════

func TestParseGapCommand_Valid(t *testing.T) {
	response := `I've identified a structural gap.
/gap {"category":"quality","missing_role":"reviewer","evidence":"3 tasks completed without review","severity":"medium"}
/signal {"signal": "IDLE"}`

	cmd := parseGapCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil GapCommand")
	}
	if cmd.Category != "quality" {
		t.Errorf("Category = %q, want %q", cmd.Category, "quality")
	}
	if cmd.MissingRole != "reviewer" {
		t.Errorf("MissingRole = %q, want %q", cmd.MissingRole, "reviewer")
	}
	if cmd.Evidence != "3 tasks completed without review" {
		t.Errorf("Evidence = %q, want %q", cmd.Evidence, "3 tasks completed without review")
	}
	if cmd.Severity != "medium" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "medium")
	}
}

func TestParseGapCommand_NoCommand(t *testing.T) {
	response := `Everything looks fine structurally.
/signal {"signal": "IDLE"}`

	cmd := parseGapCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseGapCommand_MalformedJSON(t *testing.T) {
	response := `/gap {not valid json`

	cmd := parseGapCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

func TestParseGapCommand_MultipleLines(t *testing.T) {
	response := `Analyzing the task flow...
Task stall pattern observed.
No current agent handles this.
/gap {"category":"operations","missing_role":"incident-commander","evidence":"cascading failures with no coordinated response","severity":"high"}
No further action needed.
/signal {"signal": "IDLE"}`

	cmd := parseGapCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil GapCommand")
	}
	if cmd.Category != "operations" {
		t.Errorf("Category = %q, want %q", cmd.Category, "operations")
	}
	if cmd.MissingRole != "incident-commander" {
		t.Errorf("MissingRole = %q, want %q", cmd.MissingRole, "incident-commander")
	}
	if cmd.Severity != "high" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "high")
	}
}

// ════════════════════════════════════════════════════════════════════════
// parseDirectiveCommand
// ════════════════════════════════════════════════════════════════════════

func TestParseDirectiveCommand_Valid(t *testing.T) {
	response := `Work agents need course correction.
/directive {"target":"strategist","action":"focus on test coverage before new features","reason":"3 bugs found in last sprint","priority":"high"}
/signal {"signal": "IDLE"}`

	cmd := parseDirectiveCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil DirectiveCommand")
	}
	if cmd.Target != "strategist" {
		t.Errorf("Target = %q, want %q", cmd.Target, "strategist")
	}
	if cmd.Action != "focus on test coverage before new features" {
		t.Errorf("Action = %q, want %q", cmd.Action, "focus on test coverage before new features")
	}
	if cmd.Reason != "3 bugs found in last sprint" {
		t.Errorf("Reason = %q, want %q", cmd.Reason, "3 bugs found in last sprint")
	}
	if cmd.Priority != "high" {
		t.Errorf("Priority = %q, want %q", cmd.Priority, "high")
	}
}

func TestParseDirectiveCommand_NoCommand(t *testing.T) {
	response := `No course correction needed.
/signal {"signal": "IDLE"}`

	cmd := parseDirectiveCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

// ════════════════════════════════════════════════════════════════════════
// validateGapCommand
// ════════════════════════════════════════════════════════════════════════

func TestValidateGapCommand_StabilizationBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // StabilizationWindow = 15
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "technical", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	// Iteration at or below stabilization window should be rejected.
	for _, iter := range []int{1, 5, 15} {
		err := validateGapCommand(cmd, iter, cooldowns, cfg)
		if err == nil {
			t.Errorf("iteration %d: expected error during stabilization window, got nil", iter)
		}
	}
}

func TestValidateGapCommand_CooldownBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // GapCooldown = 15
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "technical", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	// Record an emission at iteration 20.
	cooldowns.gapByCategory["technical"] = 20

	// Iteration 30 is only 10 iterations after — cooldown (15) not expired.
	err := validateGapCommand(cmd, 30, cooldowns, cfg)
	if err == nil {
		t.Error("expected cooldown error, got nil")
	}

	// Iteration 36 is 16 iterations after — cooldown expired.
	err = validateGapCommand(cmd, 36, cooldowns, cfg)
	if err != nil {
		t.Errorf("cooldown should have expired at iteration 36, got: %v", err)
	}
}

func TestValidateGapCommand_DedupBlocks(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "technical", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	// Mark "reviewer" as already emitted.
	cooldowns.emittedGaps["reviewer"] = true

	err := validateGapCommand(cmd, 20, cooldowns, cfg)
	if err == nil {
		t.Error("expected dedup error for already-emitted missing_role, got nil")
	}
}

func TestValidateGapCommand_InvalidCategory(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "invalid-category", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	err := validateGapCommand(cmd, 20, cooldowns, cfg)
	if err == nil {
		t.Error("expected invalid category error, got nil")
	}
}

func TestValidateGapCommand_Valid(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "capability", MissingRole: "auditor", Evidence: "no security review process", Severity: "high"}

	// iteration 20 > StabilizationWindow (15), no cooldowns, category valid.
	err := validateGapCommand(cmd, 20, cooldowns, cfg)
	if err != nil {
		t.Errorf("expected valid gap command, got: %v", err)
	}
}

// ════════════════════════════════════════════════════════════════════════
// validateDirectiveCommand
// ════════════════════════════════════════════════════════════════════════

func TestValidateDirectiveCommand_StabilizationBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // StabilizationWindow = 15
	cooldowns := NewCTOCooldowns()
	cmd := &DirectiveCommand{Target: "strategist", Action: "slow down", Reason: "test", Priority: "low"}

	for _, iter := range []int{1, 10, 15} {
		err := validateDirectiveCommand(cmd, iter, cooldowns, cfg)
		if err == nil {
			t.Errorf("iteration %d: expected error during stabilization window, got nil", iter)
		}
	}
}

func TestValidateDirectiveCommand_CooldownBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // DirectiveCooldown = 5
	cooldowns := NewCTOCooldowns()
	cmd := &DirectiveCommand{Target: "strategist", Action: "slow down", Reason: "test", Priority: "low"}

	// Record an emission at iteration 20.
	cooldowns.directiveByTarget["strategist"] = 20

	// Iteration 24 is only 4 iterations after — cooldown (5) not expired.
	err := validateDirectiveCommand(cmd, 24, cooldowns, cfg)
	if err == nil {
		t.Error("expected cooldown error, got nil")
	}

	// Iteration 26 is 6 iterations after — cooldown expired.
	err = validateDirectiveCommand(cmd, 26, cooldowns, cfg)
	if err != nil {
		t.Errorf("cooldown should have expired at iteration 26, got: %v", err)
	}
}

func TestValidateDirectiveCommand_Valid(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &DirectiveCommand{Target: "all", Action: "pause new task creation", Reason: "queue overloaded", Priority: "medium"}

	// iteration 20 > StabilizationWindow (15), no cooldowns.
	err := validateDirectiveCommand(cmd, 20, cooldowns, cfg)
	if err != nil {
		t.Errorf("expected valid directive command, got: %v", err)
	}
}

// ════════════════════════════════════════════════════════════════════════
// enrichCTOObservation
// ════════════════════════════════════════════════════════════════════════

func TestEnrichCTOObservation_NonCTO(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "guardian", "test-guardian")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichCTOObservation(obs)
	if result != obs {
		t.Errorf("non-cto enrichment should be identity, got %q", result)
	}
}

func TestEnrichCTOObservation_CTO(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "cto", "test-cto")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichCTOObservation(obs)
	if result == obs {
		t.Error("cto enrichment should add leadership briefing")
	}
	if !containsStr(result, "=== LEADERSHIP BRIEFING ===") {
		t.Error("missing LEADERSHIP BRIEFING header")
	}
	if !containsStr(result, "TASK FLOW:") {
		t.Error("missing TASK FLOW section")
	}
	if !containsStr(result, "HEALTH (from SysMon):") {
		t.Error("missing HEALTH section")
	}
	if !containsStr(result, "BUDGET (from Allocator):") {
		t.Error("missing BUDGET section")
	}
}

// ════════════════════════════════════════════════════════════════════════
// FactoryOrder growth-loop — dormant regression guard (H6)
// ════════════════════════════════════════════════════════════════════════

// TestEnrichCTOObservation_FactoryOrderTaskVisible locks in that a
// work.task.created event — the exact event type that work.SeedFactoryOrder
// produces — is counted in the CTO briefing's TASK FLOW line.
//
// This is the dormant growth-loop hook: a FactoryOrder enters the graph as a
// work.task.created event (via work.TaskStore.Create / work.SeedFactoryOrder).
// enrichCTOObservation already counts pending work.task.* events, so the order
// flow is ALREADY visible to the CTO with no new product code. This test locks
// that invariant against regressions.
func TestEnrichCTOObservation_FactoryOrderTaskVisible(t *testing.T) {
	// Build a shared graph with work event types registered (required for
	// TaskStore.Create to produce a well-typed event).
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	work.RegisterWithRegistry(g.Registry())

	// Create a CTO agent on that graph.
	provider := newMockProvider("noop")
	ctoAgent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("cto"),
		Name:     "cto-growth-loop-test",
		Graph:    g,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("hiveagent.New: %v", err)
	}

	// Build the CTO loop.
	l, err := New(Config{
		Agent:   ctoAgent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatalf("loop.New: %v", err)
	}

	// Produce a real work.task.created event via TaskStore — the same code path
	// that work.SeedFactoryOrder uses. We need a cause event; use the chain head
	// which is the agent's boot event.
	head, err := g.Store().Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.IsNone() {
		t.Fatal("expected agent boot event as chain head")
	}
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_00000000000000000000000000000042")
	_, err = ts.Create(
		ctoAgent.ID(),
		"DarkFactory order: produce widget v2",
		"Seeded from FactoryOrder fo_test_001",
		[]types.EventID{head.Unwrap().ID()},
		convID,
		work.PriorityHigh,
	)
	if err != nil {
		t.Fatalf("TaskStore.Create: %v", err)
	}

	// Retrieve the stored work.task.created event and inject it into
	// pendingEvents so enrichCTOObservation can count it.
	page, err := g.Store().ByType(work.EventTypeTaskCreated, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType work.task.created: %v", err)
	}
	taskEvents := page.Items()
	if len(taskEvents) == 0 {
		t.Fatal("expected at least one work.task.created event in store")
	}

	l.mu.Lock()
	l.pendingEvents = append(l.pendingEvents, taskEvents...)
	l.mu.Unlock()

	// Enrich a CTO observation with the pending task event present.
	result := l.enrichCTOObservation("## Observation")

	// The TASK FLOW line must reflect created=1 — the FactoryOrder-seeded task
	// is visible to the CTO with no new product code.
	if !strings.Contains(result, "created=1") {
		t.Errorf("TASK FLOW should show created=1 for a FactoryOrder-seeded task; got briefing:\n%s", result)
	}
	// Sanity: the briefing structure is still present.
	if !containsStr(result, "TASK FLOW:") {
		t.Error("missing TASK FLOW section in briefing")
	}
}
