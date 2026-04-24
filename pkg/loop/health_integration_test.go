package loop

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// Tier 1 — Deterministic framework tests (no LLM)
// ════════════════════════════════════════════════════════════════════════

func TestHealthCommandToEvent(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "sysmon", "test-sysmon")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &HealthCommand{
		Severity:     "warning",
		ChainOK:      true,
		ActiveAgents: 4,
		EventRate:    23.5,
	}

	if err := l.emitHealthReport(cmd); err != nil {
		t.Fatalf("emitHealthReport: %v", err)
	}

	// Query the store for health.report events.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeHealthReport,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}

	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no health.report events found in store")
	}

	ev := events[len(events)-1] // most recent
	content, ok := ev.Content().(event.HealthReportContent)
	if !ok {
		t.Fatalf("event content is %T, want HealthReportContent", ev.Content())
	}

	// Verify fields match the command.
	wantScore := types.MustScore(0.5) // "warning" → 0.5
	if content.Overall != wantScore {
		t.Errorf("Overall = %v, want %v", content.Overall, wantScore)
	}
	if content.ChainIntegrity != true {
		t.Errorf("ChainIntegrity = %v, want true", content.ChainIntegrity)
	}
	if content.ActiveActors != 4 {
		t.Errorf("ActiveActors = %d, want 4", content.ActiveActors)
	}
	if content.EventRate != 23.5 {
		t.Errorf("EventRate = %f, want 23.5", content.EventRate)
	}

	// Verify the event source is the sysmon agent.
	if ev.Source() != agent.ID() {
		t.Errorf("Source = %v, want %v", ev.Source(), agent.ID())
	}
}

func TestObservationEnrichmentFormat(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "sysmon", "test-sysmon")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Record some budget usage so the enrichment has data.
	l.budget.Record(1000, 0.05)

	baseObs := "## Recent Events\n- [hive.run.started] ev1 by actor1\n"
	result := l.enrichHealthObservation(baseObs)

	// Must contain the base observation.
	if !containsStr(result, "## Recent Events") {
		t.Error("base observation missing from enriched output")
	}

	// Must contain the health metrics block.
	if !containsStr(result, "=== HEALTH METRICS ===") {
		t.Error("missing === HEALTH METRICS === header")
	}
	if !containsStr(result, "BUDGET:") {
		t.Error("missing BUDGET section")
	}
	if !containsStr(result, "HIVE:") {
		t.Error("missing HIVE section")
	}
	if !containsStr(result, "===") {
		t.Error("missing closing === delimiter")
	}

	// Verify budget data appears (we recorded 1000 tokens, $0.05).
	if !containsStr(result, "tokens=1000") {
		t.Error("missing token count in budget section")
	}
	if !containsStr(result, "cost=$0.05") {
		t.Error("missing cost in budget section")
	}
}

func TestObservationEnrichmentSkipsNonSysmon(t *testing.T) {
	tests := []struct {
		role string
	}{
		{"guardian"},
		{"strategist"},
		{"planner"},
		{"implementer"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
			agent := testHiveAgent(t, provider, tt.role, "test-"+tt.role)

			l, err := New(Config{
				Agent:   agent,
				HumanID: humanID(),
				Budget:  resources.BudgetConfig{MaxIterations: 10},
			})
			if err != nil {
				t.Fatal(err)
			}

			obs := "some observation text"
			result := l.enrichHealthObservation(obs)
			if result != obs {
				t.Errorf("role %q should not enrich observation, got %q", tt.role, result)
			}
		})
	}
}

func TestHealthCommandCausalChain(t *testing.T) {
	provider := newMockProvider("/signal {\"signal\": \"IDLE\"}")
	agent := testHiveAgent(t, provider, "sysmon", "test-sysmon")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Emit first health report.
	cmd1 := &HealthCommand{Severity: "ok", ChainOK: true, ActiveAgents: 4, EventRate: 20.0}
	if err := l.emitHealthReport(cmd1); err != nil {
		t.Fatalf("first emitHealthReport: %v", err)
	}

	// Emit second health report.
	cmd2 := &HealthCommand{Severity: "warning", ChainOK: true, ActiveAgents: 4, EventRate: 15.0}
	if err := l.emitHealthReport(cmd2); err != nil {
		t.Fatalf("second emitHealthReport: %v", err)
	}

	// Query for health.report events.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeHealthReport,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}

	events := page.Items()
	if len(events) < 2 {
		t.Fatalf("expected >= 2 health.report events, got %d", len(events))
	}

	second := events[len(events)-1]

	// The second report must have at least one cause (causal chain is maintained).
	// The agent's recordAndTrack updates lastEvent after each emission, so
	// each subsequent event causally links to the previous one.
	if len(second.Causes()) == 0 {
		t.Error("second health.report has no causes — causal chain is broken")
	}

	// Verify both reports have distinct IDs (not duplicates).
	first := events[len(events)-2]
	if first.ID() == second.ID() {
		t.Error("first and second health.report have the same ID")
	}

	// Verify severity values are distinct (ok vs warning).
	c1, ok1 := first.Content().(event.HealthReportContent)
	c2, ok2 := second.Content().(event.HealthReportContent)
	if !ok1 || !ok2 {
		t.Fatal("content type assertion failed")
	}
	if c1.Overall == c2.Overall {
		t.Errorf("expected different severities, both are %v", c1.Overall)
	}
}

func TestHealthCommandInLoop(t *testing.T) {
	// Verify /health command in LLM response produces a health.report event
	// via the full loop execution path.
	provider := newMockProvider(
		"Health looks good.\n/health {\"severity\":\"ok\",\"chain_ok\":true,\"active_agents\":4,\"event_rate\":10.0}\n/signal {\"signal\": \"TASK_DONE\"}",
	)
	agent := testHiveAgent(t, provider, "sysmon", "test-sysmon")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 5},
		Task:    "monitor health",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopTaskDone {
		t.Fatalf("reason = %s, want %s (detail: %s)", result.Reason, StopTaskDone, result.Detail)
	}

	// Verify health.report event was emitted.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeHealthReport,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(page.Items()) == 0 {
		t.Fatal("no health.report event emitted during loop execution")
	}

	content, ok := page.Items()[len(page.Items())-1].Content().(event.HealthReportContent)
	if !ok {
		t.Fatal("event content is not HealthReportContent")
	}
	if content.Overall != types.MustScore(1.0) {
		t.Errorf("Overall = %v, want 1.0 (ok)", content.Overall)
	}
}
