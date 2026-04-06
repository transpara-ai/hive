package loop

import (
	"context"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// Tier 1 — Deterministic framework tests (no LLM)
// ════════════════════════════════════════════════════════════════════════

func TestGapCommandToEvent(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "cto", "test-cto")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &GapCommand{
		Category:    "technical",
		MissingRole: "reviewer",
		Evidence:    "3 tasks completed without review in last 20 events",
		Severity:    "warning",
	}

	if err := l.emitGap(cmd); err != nil {
		t.Fatalf("emitGap: %v", err)
	}

	// Query the store for hive.gap.detected events.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeGapDetected,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}

	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no hive.gap.detected events found in store")
	}

	ev := events[len(events)-1]
	content, ok := ev.Content().(event.GapDetectedContent)
	if !ok {
		t.Fatalf("event content is %T, want GapDetectedContent", ev.Content())
	}

	// emitGap normalizes to title case.
	if content.Category != "Technical" {
		t.Errorf("Category = %q, want %q", content.Category, "Technical")
	}
	if content.MissingRole != "reviewer" {
		t.Errorf("MissingRole = %q, want %q", content.MissingRole, "reviewer")
	}
	if content.Evidence != "3 tasks completed without review in last 20 events" {
		t.Errorf("Evidence = %q, want %q", content.Evidence, "3 tasks completed without review in last 20 events")
	}
	if content.Severity != "Warning" {
		t.Errorf("Severity = %q, want %q", content.Severity, "Warning")
	}

	// Verify the event source is the CTO agent.
	if ev.Source() != agent.ID() {
		t.Errorf("Source = %v, want %v", ev.Source(), agent.ID())
	}
}

func TestDirectiveCommandToEvent(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "cto", "test-cto")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &DirectiveCommand{
		Target:   "strategist",
		Action:   "focus on test coverage before new features",
		Reason:   "3 bugs found in last sprint",
		Priority: "high",
	}

	if err := l.emitDirective(cmd); err != nil {
		t.Fatalf("emitDirective: %v", err)
	}

	// Query the store for hive.directive.issued events.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeDirectiveIssued,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}

	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no hive.directive.issued events found in store")
	}

	ev := events[len(events)-1]
	content, ok := ev.Content().(event.DirectiveIssuedContent)
	if !ok {
		t.Fatalf("event content is %T, want DirectiveIssuedContent", ev.Content())
	}

	if content.Target != "strategist" {
		t.Errorf("Target = %q, want %q", content.Target, "strategist")
	}
	if content.Action != "focus on test coverage before new features" {
		t.Errorf("Action = %q, want %q", content.Action, "focus on test coverage before new features")
	}
	if content.Reason != "3 bugs found in last sprint" {
		t.Errorf("Reason = %q, want %q", content.Reason, "3 bugs found in last sprint")
	}
	// emitDirective normalizes to title case.
	if content.Priority != "High" {
		t.Errorf("Priority = %q, want %q", content.Priority, "High")
	}

	if ev.Source() != agent.ID() {
		t.Errorf("Source = %v, want %v", ev.Source(), agent.ID())
	}
}

func TestCTOObservationEnrichmentFormat(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "cto", "test-cto")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	baseObs := "## Recent Events\n- [work.task.created] task-1 by actor1\n"
	result := l.enrichCTOObservation(baseObs)

	// Must contain the base observation.
	if !containsStr(result, "## Recent Events") {
		t.Error("base observation missing from enriched output")
	}

	// Must contain the leadership briefing block.
	if !containsStr(result, "=== LEADERSHIP BRIEFING ===") {
		t.Error("missing === LEADERSHIP BRIEFING === header")
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
	if !containsStr(result, "GAPS (previously detected):") {
		t.Error("missing GAPS section")
	}
	if !containsStr(result, "DIRECTIVES (active):") {
		t.Error("missing DIRECTIVES section")
	}
	if !containsStr(result, "===") {
		t.Error("missing closing === delimiter")
	}
}

func TestCTOObservationEnrichmentSkipsNonCTO(t *testing.T) {
	tests := []struct {
		role string
	}{
		{"guardian"},
		{"sysmon"},
		{"allocator"},
		{"strategist"},
		{"planner"},
		{"implementer"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			provider := newMockProvider(`/signal {"signal": "IDLE"}`)
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
			result := l.enrichCTOObservation(obs)
			if result != obs {
				t.Errorf("role %q should not enrich observation, got %q", tt.role, result)
			}
		})
	}
}

func TestGapCommandInLoop(t *testing.T) {
	// Verify /gap command in LLM response produces a hive.gap.detected event
	// via the full loop execution path.
	//
	// Set stabilization window to 0 so the gap is accepted on iteration 1 — the
	// unit tests cover the stabilization constraint independently.
	t.Setenv("CTO_STABILIZATION_WINDOW", "0")

	provider := newMockProvider(
		`/gap {"category":"technical","missing_role":"reviewer","evidence":"tasks completed without review","severity":"warning"}` +
			"\n" + `/signal {"signal": "TASK_DONE"}`,
	)
	agent := testHiveAgent(t, provider, "cto", "test-cto")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 20},
		Task:    "watch the hive",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopTaskDone {
		t.Fatalf("reason = %s, want %s (detail: %s)", result.Reason, StopTaskDone, result.Detail)
	}

	// Verify hive.gap.detected event was emitted.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeGapDetected,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(page.Items()) == 0 {
		t.Fatal("no hive.gap.detected event emitted during loop execution")
	}

	content, ok := page.Items()[len(page.Items())-1].Content().(event.GapDetectedContent)
	if !ok {
		t.Fatal("event content is not GapDetectedContent")
	}
	if content.Category != "Technical" {
		t.Errorf("Category = %q, want %q", content.Category, "Technical")
	}
	if content.MissingRole != "reviewer" {
		t.Errorf("MissingRole = %q, want %q", content.MissingRole, "reviewer")
	}
}
