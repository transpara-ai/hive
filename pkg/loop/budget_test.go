package loop

import (
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/budget"
	"github.com/transpara-ai/hive/pkg/resources"
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

// --- applyBudgetAdjustment action variants ---

func TestApplyBudgetAdjustment_Decrease(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "guardian", Action: "decrease", Amount: 30, Reason: "idle agent"}
	if err := l.applyBudgetAdjustment(cmd, 20); err != nil {
		t.Fatalf("applyBudgetAdjustment: %v", err)
	}

	for _, e := range l.config.BudgetRegistry.Snapshot() {
		if e.Name == "guardian" {
			// 200 - 30 = 170
			if e.MaxIterations != 170 {
				t.Errorf("MaxIterations = %d, want 170", e.MaxIterations)
			}
			return
		}
	}
	t.Fatal("guardian not found in registry")
}

func TestApplyBudgetAdjustment_Set(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 80, Reason: "normalize"}
	if err := l.applyBudgetAdjustment(cmd, 20); err != nil {
		t.Fatalf("applyBudgetAdjustment: %v", err)
	}

	for _, e := range l.config.BudgetRegistry.Snapshot() {
		if e.Name == "implementer" {
			// Was 100, set to 80
			if e.MaxIterations != 80 {
				t.Errorf("MaxIterations = %d, want 80", e.MaxIterations)
			}
			return
		}
	}
	t.Fatal("implementer not found in registry")
}

func TestApplyBudgetAdjustment_CeilingClamp(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	// Increase by 1000 from 100 → would be 1100, but ceiling is 500.
	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 1000, Reason: "max out"}
	if err := l.applyBudgetAdjustment(cmd, 20); err != nil {
		t.Fatalf("applyBudgetAdjustment: %v", err)
	}

	for _, e := range l.config.BudgetRegistry.Snapshot() {
		if e.Name == "implementer" {
			if e.MaxIterations != 500 {
				t.Errorf("MaxIterations = %d, want 500 (ceiling clamp)", e.MaxIterations)
			}
			return
		}
	}
	t.Fatal("implementer not found in registry")
}

func TestApplyBudgetAdjustment_RecordsHistory(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	if len(l.adjustmentHistory) != 0 {
		t.Fatalf("initial history should be empty, got %d entries", len(l.adjustmentHistory))
	}

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "productive"}
	if err := l.applyBudgetAdjustment(cmd, 20); err != nil {
		t.Fatalf("applyBudgetAdjustment: %v", err)
	}

	if len(l.adjustmentHistory) != 1 {
		t.Fatalf("history length = %d, want 1", len(l.adjustmentHistory))
	}
	rec := l.adjustmentHistory[0]
	if rec.Agent != "implementer" {
		t.Errorf("Agent = %q, want %q", rec.Agent, "implementer")
	}
	if rec.Iteration != 20 {
		t.Errorf("Iteration = %d, want 20", rec.Iteration)
	}
	if rec.Delta != 25 {
		t.Errorf("Delta = %d, want 25", rec.Delta)
	}
	if rec.Reason != "productive" {
		t.Errorf("Reason = %q, want %q", rec.Reason, "productive")
	}
}

// --- validateBudgetCommand additional cases ---

func TestValidateBudgetCommand_NilRegistry(t *testing.T) {
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

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "test"}
	err = l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
	if !strings.Contains(err.Error(), "no budget registry") {
		t.Errorf("error should mention no budget registry: %v", err)
	}
}

func TestValidateBudgetCommand_NegativeAmount(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: -5, Reason: "test"}
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("error should mention positive: %v", err)
	}
}

func TestValidateBudgetCommand_DecreaseSkipsPoolCheck(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	// Decrease should not check pool headroom — only increases do.
	cmd := &BudgetCommand{Agent: "guardian", Action: "decrease", Amount: 50, Reason: "idle"}
	err := l.validateBudgetCommand(cmd, 15)
	if err != nil {
		t.Errorf("decrease should pass validation, got: %v", err)
	}
}

func TestValidateBudgetCommand_SetAction(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 80, Reason: "normalize"}
	err := l.validateBudgetCommand(cmd, 15)
	if err != nil {
		t.Errorf("set action should pass validation, got: %v", err)
	}
}

// --- parseBudgetCommand additional cases ---

func TestParseBudgetCommand_EmptyPayload(t *testing.T) {
	response := `/budget `
	cmd := parseBudgetCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for empty payload, got %+v", cmd)
	}
}

func TestParseBudgetCommand_MissingFields(t *testing.T) {
	// JSON is valid but missing required fields — parser accepts it (zero values).
	response := `/budget {"agent":"implementer"}`
	cmd := parseBudgetCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil for partial JSON")
	}
	if cmd.Agent != "implementer" {
		t.Errorf("Agent = %q, want %q", cmd.Agent, "implementer")
	}
	if cmd.Action != "" {
		t.Errorf("Action = %q, want empty", cmd.Action)
	}
	if cmd.Amount != 0 {
		t.Errorf("Amount = %d, want 0", cmd.Amount)
	}
}

func TestParseBudgetCommand_MultipleTakesFirst(t *testing.T) {
	response := `/budget {"agent":"first","action":"increase","amount":10,"reason":"a"}
/budget {"agent":"second","action":"decrease","amount":20,"reason":"b"}`

	cmd := parseBudgetCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil")
	}
	if cmd.Agent != "first" {
		t.Errorf("Agent = %q, want %q (should take first)", cmd.Agent, "first")
	}
}

// --- enrichBudgetObservation additional cases ---

func TestEnrichBudgetObservation_HistoryTruncation(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	// Add 7 history entries — should only show last 5.
	for i := 1; i <= 7; i++ {
		l.adjustmentHistory = append(l.adjustmentHistory, budget.AdjustmentRecord{
			Agent:     "implementer",
			Iteration: i * 10,
			Delta:     i * 5,
			Reason:    fmt.Sprintf("reason-%d", i),
		})
	}

	result := l.enrichBudgetObservation("obs", 100)
	if !strings.Contains(result, "ADJUSTMENT HISTORY (last 5)") {
		t.Error("expected truncation header")
	}
	// Entry 1-2 (iter 10, 20) should be truncated; 3-7 should remain.
	if strings.Contains(result, "reason-1") {
		t.Error("reason-1 should be truncated")
	}
	if strings.Contains(result, "reason-2") {
		t.Error("reason-2 should be truncated")
	}
	if !strings.Contains(result, "reason-3") {
		t.Error("expected reason-3 in last 5")
	}
	if !strings.Contains(result, "reason-7") {
		t.Error("expected reason-7 in last 5")
	}
}

func TestEnrichBudgetObservation_CooldownsDisplayed(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")

	// Recent adjustment for guardian at iter 95 — at iter 100 with cooldown 10 → 5 remaining.
	l.adjustmentHistory = []budget.AdjustmentRecord{
		{Agent: "guardian", Iteration: 95, Delta: 10, Reason: "recent"},
	}

	result := l.enrichBudgetObservation("obs", 100)
	if !strings.Contains(result, "COOLDOWNS:") {
		t.Error("expected COOLDOWNS section")
	}
	// Guardian should show remaining cooldown.
	if !strings.Contains(result, "guardian") {
		t.Error("expected guardian in cooldowns")
	}
	if !strings.Contains(result, "remaining") {
		t.Error("expected 'remaining' indicator for guardian cooldown")
	}
	// Implementer should be clear (no recent adjustment).
	if !strings.Contains(result, "clear") {
		t.Error("expected 'clear' for agents without cooldown")
	}
}
