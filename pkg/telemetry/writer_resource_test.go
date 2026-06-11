package telemetry

import (
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

// codex r1 #2 (v14-F3c follow-through): a duration adjustment's NewBudget is
// MINUTES — rendering it as "iterations" misreports renewals on every
// dashboard reading the event stream.
func TestEventSummaryBudgetAdjustedByResource(t *testing.T) {
	iter := eventSummary("allocator", "agent.budget.adjusted",
		event.AgentBudgetAdjustedContent{AgentName: "cto", PreviousBudget: 50, NewBudget: 100})
	if !strings.Contains(iter, "iterations") {
		t.Fatalf("legacy/iterations adjustment summary = %q; must say iterations", iter)
	}
	dur := eventSummary("allocator", "agent.budget.adjusted",
		event.AgentBudgetAdjustedContent{AgentName: "implementer", PreviousBudget: 30, NewBudget: 120, Resource: "duration"})
	if strings.Contains(dur, "iterations") {
		t.Fatalf("duration adjustment rendered as iterations: %q", dur)
	}
	if !strings.Contains(dur, "minutes") {
		t.Fatalf("duration adjustment summary = %q; must say minutes", dur)
	}
}
