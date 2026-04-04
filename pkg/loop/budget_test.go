package loop

import (
	"strings"
	"testing"

	"github.com/lovyou-ai/hive/pkg/budget"
	"github.com/lovyou-ai/hive/pkg/resources"
)

// --- parseBudgetCommand ---

func TestParseBudgetCommand_Valid(t *testing.T) {
	response := `Assessing budget metrics...
/budget {"agent":"implementer","action":"increase","amount":25,"reason":"high-value work in progress"}
/signal {"signal": "IDLE"}`

	cmd := parseBudgetCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil BudgetCommand")
	}
	if cmd.Agent != "implementer" {
		t.Errorf("Agent = %q, want %q", cmd.Agent, "implementer")
	}
	if cmd.Action != "increase" {
		t.Errorf("Action = %q, want %q", cmd.Action, "increase")
	}
	if cmd.Amount != 25 {
		t.Errorf("Amount = %d, want 25", cmd.Amount)
	}
	if cmd.Reason != "high-value work in progress" {
		t.Errorf("Reason = %q, want %q", cmd.Reason, "high-value work in progress")
	}
}

func TestParseBudgetCommand_NoCommand(t *testing.T) {
	response := `Budget looks balanced. No adjustment needed.
/signal {"signal": "IDLE"}`

	cmd := parseBudgetCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseBudgetCommand_MalformedJSON(t *testing.T) {
	response := `/budget {bad json here`

	cmd := parseBudgetCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

func TestParseBudgetCommand_BuriedInOutput(t *testing.T) {
	response := `I've analyzed the budget metrics. The implementer is at 85% utilization
and producing good work. The strategist has been quiesced for a while.
I'll increase the implementer's budget.
/budget {"agent":"implementer","action":"increase","amount":50,"reason":"approaching exhaustion with active tasks"}
This should keep them running for the next phase.
/signal {"signal": "IDLE"}`

	cmd := parseBudgetCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil BudgetCommand buried in output")
	}
	if cmd.Agent != "implementer" {
		t.Errorf("Agent = %q, want %q", cmd.Agent, "implementer")
	}
	if cmd.Amount != 50 {
		t.Errorf("Amount = %d, want 50", cmd.Amount)
	}
}

func TestParseBudgetCommand_Indented(t *testing.T) {
	response := `  /budget {"agent":"planner","action":"decrease","amount":20,"reason":"sustained idle"}`

	cmd := parseBudgetCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil BudgetCommand for indented line")
	}
	if cmd.Action != "decrease" {
		t.Errorf("Action = %q, want %q", cmd.Action, "decrease")
	}
}

// --- validateBudgetCommand ---

func makeLoopWithRegistry(t *testing.T, role string) *Loop {
	t.Helper()
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, role, "test-"+role)

	reg := resources.NewBudgetRegistry()
	reg.Register("guardian", resources.NewBudget(resources.BudgetConfig{MaxIterations: 200}), 200)
	reg.Register("sysmon", resources.NewBudget(resources.BudgetConfig{MaxIterations: 150}), 150)
	reg.Register("implementer", resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100)

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 150},
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestValidateBudgetCommand_Passes(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "test"}
	// Iteration 15 is past stabilization window (default 10).
	err := l.validateBudgetCommand(cmd, 15)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateBudgetCommand_StabilizationWindow(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "test"}
	err := l.validateBudgetCommand(cmd, 5) // Within default window of 10
	if err == nil {
		t.Fatal("expected stabilization window error")
	}
	if !strings.Contains(err.Error(), "stabilization") {
		t.Errorf("error should mention stabilization: %v", err)
	}
}

func TestValidateBudgetCommand_UnknownAgent(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "nonexistent", Action: "increase", Amount: 25, Reason: "test"}
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected unknown agent error")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("error should mention unknown agent: %v", err)
	}
}

func TestValidateBudgetCommand_ZeroAmount(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 0, Reason: "test"}
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected amount error")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("error should mention positive: %v", err)
	}
}

func TestValidateBudgetCommand_AgentCooldown(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	l.adjustmentHistory = []budget.AdjustmentRecord{
		{Agent: "implementer", Iteration: 18, Delta: 10, Reason: "prior"},
	}

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "test"}
	// Iteration 25: global cooldown (5) clear (25-18=7 > 5), but agent cooldown (10) active (25-18=7 < 10 → 3 remaining)
	err := l.validateBudgetCommand(cmd, 25)
	if err == nil {
		t.Fatal("expected agent cooldown error")
	}
	if !strings.Contains(err.Error(), "cooldown active for implementer") {
		t.Errorf("error should mention agent cooldown: %v", err)
	}
}

func TestValidateBudgetCommand_GlobalCooldown(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	l.adjustmentHistory = []budget.AdjustmentRecord{
		{Agent: "guardian", Iteration: 14, Delta: 10, Reason: "prior"},
	}

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "test"}
	// Iteration 15, last global adjustment at 14, global cooldown 5 → remaining 4
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected global cooldown error")
	}
	if !strings.Contains(err.Error(), "global cooldown") {
		t.Errorf("error should mention global cooldown: %v", err)
	}
}

func TestValidateBudgetCommand_InsufficientPool(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	// Total pool is 450 (200+150+100), total used is 0 → headroom 450
	// But request 500 which exceeds headroom... actually 500 > 450
	// Actually let's make used > 0 to make the headroom smaller.
	// Register agents with some usage.
	reg := resources.NewBudgetRegistry()
	b := resources.NewBudget(resources.BudgetConfig{MaxIterations: 200})
	for i := 0; i < 190; i++ {
		b.Record(1, 0.01) // 190 iterations used
	}
	reg.Register("guardian", b, 200)

	b2 := resources.NewBudget(resources.BudgetConfig{MaxIterations: 150})
	for i := 0; i < 140; i++ {
		b2.Record(1, 0.01) // 140 iterations used
	}
	reg.Register("sysmon", b2, 150)

	b3 := resources.NewBudget(resources.BudgetConfig{MaxIterations: 100})
	for i := 0; i < 95; i++ {
		b3.Record(1, 0.01) // 95 iterations used
	}
	reg.Register("implementer", b3, 100)

	l.config.BudgetRegistry = reg

	// Total pool=450, total used=425 → headroom=25
	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 50, Reason: "test"}
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected insufficient pool error")
	}
	if !strings.Contains(err.Error(), "insufficient pool") {
		t.Errorf("error should mention insufficient pool: %v", err)
	}
}

func TestValidateBudgetCommand_InvalidAction(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "delete", Amount: 25, Reason: "test"}
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected invalid action error")
	}
	if !strings.Contains(err.Error(), "invalid action") {
		t.Errorf("error should mention invalid action: %v", err)
	}
}

// --- enrichBudgetObservation ---

func TestEnrichBudgetObservation_NonAllocator(t *testing.T) {
	l := makeLoopWithRegistry(t, "guardian")

	obs := "some observation"
	result := l.enrichBudgetObservation(obs, 15)
	if result != obs {
		t.Errorf("non-allocator enrichment should be identity, got extra content")
	}
}

func TestEnrichBudgetObservation_Allocator(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	obs := "## Recent Events\n- some event\n"
	result := l.enrichBudgetObservation(obs, 15)

	if !strings.Contains(result, "=== BUDGET METRICS ===") {
		t.Error("expected BUDGET METRICS header")
	}
	if !strings.Contains(result, "POOL:") {
		t.Error("expected POOL section")
	}
	if !strings.Contains(result, "AGENTS:") {
		t.Error("expected AGENTS section")
	}
	if !strings.Contains(result, "COOLDOWNS:") {
		t.Error("expected COOLDOWNS section")
	}
	if !strings.Contains(result, "guardian") {
		t.Error("expected guardian in agent list")
	}
	if !strings.Contains(result, "implementer") {
		t.Error("expected implementer in agent list")
	}
}

func TestEnrichBudgetObservation_NoRegistry(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")
	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 150},
		// No BudgetRegistry
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichBudgetObservation(obs, 15)
	if result != obs {
		t.Errorf("enrichment without registry should be identity")
	}
}

func TestEnrichBudgetObservation_WithHistory(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	l.adjustmentHistory = []budget.AdjustmentRecord{
		{Agent: "implementer", Iteration: 12, Delta: 50, Reason: "productive"},
	}

	result := l.enrichBudgetObservation("obs", 15)
	if !strings.Contains(result, "ADJUSTMENT HISTORY") {
		t.Error("expected ADJUSTMENT HISTORY section")
	}
	if !strings.Contains(result, "implementer") {
		t.Error("expected implementer in history")
	}
}
