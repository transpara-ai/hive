package loop

import (
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v14-F3(c): the /budget command grows a resource dimension
//
// "iterations" (default, every pre-v14 command) or "duration" (Amount in
// minutes). The resource value is an ALLOWLIST: anything else is refused —
// a typo'd resource silently applied to iterations would be an invisible
// mis-adjustment.
// ════════════════════════════════════════════════════════════════════════

func TestParseBudgetCommandResource(t *testing.T) {
	cmd := parseBudgetCommand(`/budget {"agent":"cto","action":"set","amount":100,"reason":"x"}`)
	if cmd == nil || cmd.Resource != "" {
		t.Fatalf("legacy command Resource = %+v; want empty (reads as iterations)", cmd)
	}
	cmd = parseBudgetCommand(`/budget {"agent":"implementer","action":"set","amount":120,"reason":"renew","resource":"duration"}`)
	if cmd == nil || cmd.Resource != "duration" {
		t.Fatalf("duration command parsed = %+v; want Resource \"duration\"", cmd)
	}
}

func TestValidateBudgetCommandResourceAllowlist(t *testing.T) {
	reg := resources.NewBudgetRegistry()
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 50, MaxDuration: 30 * time.Minute}), 50, "m")
	l := &Loop{config: Config{BudgetRegistry: reg}}

	for _, ok := range []string{"", "iterations", "duration"} {
		cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 60, Reason: "r", Resource: ok}
		if err := l.validateBudgetCommand(cmd, 50); err != nil {
			t.Fatalf("validate resource %q: %v; want accepted", ok, err)
		}
	}
	for _, bad := range []string{"tokens", "Duration", "minutes", "time", "cost"} {
		cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 60, Reason: "r", Resource: bad}
		if err := l.validateBudgetCommand(cmd, 50); err == nil {
			t.Fatalf("validate resource %q accepted; want allowlist refusal", bad)
		}
	}
}

func TestValidateBudgetCommandDurationSkipsIterationPoolHeadroom(t *testing.T) {
	// One agent whose iteration pool is fully consumed: an iteration INCREASE
	// must fail on headroom, but a duration increase has no iteration-pool
	// dimension and must validate.
	reg := resources.NewBudgetRegistry()
	b := resources.NewBudget(resources.BudgetConfig{MaxIterations: 1, MaxDuration: 30 * time.Minute})
	b.Record(0, 0) // consume the single iteration: pool used == pool total
	reg.Register("implementer", b, 1, "m")
	l := &Loop{config: Config{BudgetRegistry: reg}}

	iter := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 10, Reason: "r"}
	if err := l.validateBudgetCommand(iter, 50); err == nil {
		t.Fatal("iteration increase with zero pool headroom validated; want headroom refusal")
	}
	dur := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 60, Reason: "r", Resource: "duration"}
	if err := l.validateBudgetCommand(dur, 50); err != nil {
		t.Fatalf("duration increase rejected by iteration-pool headroom: %v; duration has no iteration pool", err)
	}
}

func TestApplyBudgetAdjustmentDuration(t *testing.T) {
	provider := newMockProvider("ok")
	agent, g := agentWithGraph(t, provider)
	reg := resources.NewBudgetRegistry()
	b := resources.NewBudgetForTest(resources.BudgetConfig{MaxDuration: 30 * time.Minute}, time.Now())
	reg.Register("implementer", b, 50, "m")

	l, err := New(Config{Agent: agent, HumanID: humanID(), BudgetRegistry: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 120, Reason: "renew parked implementer", Resource: "duration"}
	if err := l.applyBudgetAdjustment(cmd, 50); err != nil {
		t.Fatalf("applyBudgetAdjustment(duration set 120): %v", err)
	}
	if got := b.MaxDuration(); got != 120*time.Minute {
		t.Fatalf("MaxDuration after apply = %v; want 120m", got)
	}

	// The chain event must name the adjusted resource.
	page, err := g.Store().ByType(event.EventTypeAgentBudgetAdjusted, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(budget.adjusted): %v", err)
	}
	if len(page.Items()) != 1 {
		t.Fatalf("budget.adjusted events = %d; want 1", len(page.Items()))
	}
	content, ok := page.Items()[0].Content().(event.AgentBudgetAdjustedContent)
	if !ok {
		t.Fatal("budget.adjusted content has wrong type")
	}
	if content.Resource != "duration" {
		t.Fatalf("event Resource = %q; want \"duration\"", content.Resource)
	}
	if content.PreviousBudget != 30 || content.NewBudget != 120 {
		t.Fatalf("event budgets = (%d, %d); want (30, 120) minutes", content.PreviousBudget, content.NewBudget)
	}
}

func TestApplyBudgetAdjustmentIterationsEventNamesResource(t *testing.T) {
	provider := newMockProvider("ok")
	agent, g := agentWithGraph(t, provider)
	reg := resources.NewBudgetRegistry()
	reg.Register("cto", resources.NewBudget(resources.BudgetConfig{MaxIterations: 50}), 50, "m")

	l, err := New(Config{Agent: agent, HumanID: humanID(), BudgetRegistry: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cmd := &BudgetCommand{Agent: "cto", Action: "set", Amount: 100, Reason: "more headroom"}
	if err := l.applyBudgetAdjustment(cmd, 50); err != nil {
		t.Fatalf("applyBudgetAdjustment(iterations): %v", err)
	}
	page, err := g.Store().ByType(event.EventTypeAgentBudgetAdjusted, 10, types.None[types.Cursor]())
	if err != nil || len(page.Items()) != 1 {
		t.Fatalf("budget.adjusted events = %d (%v); want 1", len(page.Items()), err)
	}
	content := page.Items()[0].Content().(event.AgentBudgetAdjustedContent)
	if content.Resource != "iterations" {
		t.Fatalf("iterations event Resource = %q; want \"iterations\" (explicit, not empty)", content.Resource)
	}
}

// v14-F3(c) observability: the allocator can only renew what it can see —
// per-agent wall-clock consumption joins the iteration columns.
func TestEnrichBudgetObservationIncludesDuration(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	result := l.enrichBudgetObservation("obs", 15)
	if !strings.Contains(result, "dur=") {
		t.Fatalf("allocator budget metrics missing duration columns (want \"dur=\"):\n%s", result)
	}
}
