package budget

import "testing"

// --- CheckConcentration ---

func TestCheckConcentration_FlagsAt50Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "implementer", UsedIterations: 50},
		{Name: "guardian", UsedIterations: 30},
		{Name: "sysmon", UsedIterations: 20},
	}
	pool := PoolState{UsedIterations: 100}
	cfg := Config{ConcentrationPct: 40}

	warnings := CheckConcentration(agents, pool, cfg)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
	if warnings[0] == "" {
		t.Error("warning is empty")
	}
}

func TestCheckConcentration_ClearAt30Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "a", UsedIterations: 30},
		{Name: "b", UsedIterations: 35},
		{Name: "c", UsedIterations: 35},
	}
	pool := PoolState{UsedIterations: 100}
	cfg := Config{ConcentrationPct: 40}

	warnings := CheckConcentration(agents, pool, cfg)
	if len(warnings) != 0 {
		t.Errorf("got %d warnings, want 0", len(warnings))
	}
}

func TestCheckConcentration_ZeroUsed(t *testing.T) {
	agents := []AgentBudgetState{{Name: "a", UsedIterations: 0}}
	pool := PoolState{UsedIterations: 0}
	cfg := Config{ConcentrationPct: 40}

	warnings := CheckConcentration(agents, pool, cfg)
	if len(warnings) != 0 {
		t.Errorf("got %d warnings for zero usage, want 0", len(warnings))
	}
}

// --- CheckExhaustion ---

func TestCheckExhaustion_FlagsAt85Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "implementer", MaxIterations: 100, UsedIterations: 85},
	}
	cfg := Config{ExhaustionWarningPct: 80}

	warnings := CheckExhaustion(agents, cfg)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
}

func TestCheckExhaustion_ClearAt50Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "guardian", MaxIterations: 200, UsedIterations: 100},
	}
	cfg := Config{ExhaustionWarningPct: 80}

	warnings := CheckExhaustion(agents, cfg)
	if len(warnings) != 0 {
		t.Errorf("got %d warnings, want 0", len(warnings))
	}
}

func TestCheckExhaustion_ExactThreshold(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "sysmon", MaxIterations: 100, UsedIterations: 80},
	}
	cfg := Config{ExhaustionWarningPct: 80}

	warnings := CheckExhaustion(agents, cfg)
	if len(warnings) != 1 {
		t.Fatalf("exact threshold should flag: got %d warnings, want 1", len(warnings))
	}
}

// --- CheckIdleAgents ---

func TestCheckIdleAgents_FlagsActiveAt5Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "planner", MaxIterations: 100, UsedIterations: 5, State: "Active"},
	}
	cfg := Config{IdleThresholdPct: 10}

	warnings := CheckIdleAgents(agents, cfg)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
}

func TestCheckIdleAgents_SkipsQuiescedAt0Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "strategist", MaxIterations: 100, UsedIterations: 0, State: "Quiesced"},
	}
	cfg := Config{IdleThresholdPct: 10}

	warnings := CheckIdleAgents(agents, cfg)
	if len(warnings) != 0 {
		t.Errorf("quiesced agent should be excluded: got %d warnings, want 0", len(warnings))
	}
}

func TestCheckIdleAgents_ClearAt15Pct(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "guardian", MaxIterations: 100, UsedIterations: 15, State: "Active"},
	}
	cfg := Config{IdleThresholdPct: 10}

	warnings := CheckIdleAgents(agents, cfg)
	if len(warnings) != 0 {
		t.Errorf("got %d warnings, want 0", len(warnings))
	}
}

// --- CheckDailyBurnRate ---

func TestCheckDailyBurnRate_FlagsAt95Pct(t *testing.T) {
	pool := PoolState{DailyCap: 5.0, ProjectedDailyPct: 95}
	cfg := Config{DailyCapWarningPct: 90}

	warning := CheckDailyBurnRate(pool, cfg)
	if warning == nil {
		t.Fatal("expected warning, got nil")
	}
}

func TestCheckDailyBurnRate_ClearAt60Pct(t *testing.T) {
	pool := PoolState{DailyCap: 5.0, ProjectedDailyPct: 60}
	cfg := Config{DailyCapWarningPct: 90}

	warning := CheckDailyBurnRate(pool, cfg)
	if warning != nil {
		t.Errorf("expected nil, got %q", *warning)
	}
}

func TestCheckDailyBurnRate_ZeroCap(t *testing.T) {
	pool := PoolState{DailyCap: 0, ProjectedDailyPct: 200}
	cfg := Config{DailyCapWarningPct: 90}

	warning := CheckDailyBurnRate(pool, cfg)
	if warning != nil {
		t.Errorf("zero cap should return nil, got %q", *warning)
	}
}

// --- CooldownRemaining ---

func TestCooldownRemaining_Active(t *testing.T) {
	history := []AdjustmentRecord{
		{Agent: "implementer", Iteration: 47},
	}
	cfg := Config{AgentCooldown: 10}

	got := CooldownRemaining("implementer", history, 50, cfg)
	if got != 7 {
		t.Errorf("cooldown = %d, want 7", got)
	}
}

func TestCooldownRemaining_Clear(t *testing.T) {
	history := []AdjustmentRecord{
		{Agent: "implementer", Iteration: 35},
	}
	cfg := Config{AgentCooldown: 10}

	got := CooldownRemaining("implementer", history, 50, cfg)
	if got != 0 {
		t.Errorf("cooldown = %d, want 0", got)
	}
}

func TestCooldownRemaining_NoHistory(t *testing.T) {
	cfg := Config{AgentCooldown: 10}

	got := CooldownRemaining("guardian", nil, 50, cfg)
	if got != 0 {
		t.Errorf("cooldown = %d, want 0", got)
	}
}

func TestCooldownRemaining_UsesLatestEntry(t *testing.T) {
	history := []AdjustmentRecord{
		{Agent: "implementer", Iteration: 10},
		{Agent: "implementer", Iteration: 45},
	}
	cfg := Config{AgentCooldown: 10}

	got := CooldownRemaining("implementer", history, 50, cfg)
	if got != 5 {
		t.Errorf("cooldown = %d, want 5 (should use latest entry)", got)
	}
}

// --- GlobalCooldownRemaining ---

func TestGlobalCooldownRemaining_Active(t *testing.T) {
	history := []AdjustmentRecord{
		{Agent: "planner", Iteration: 48},
	}
	cfg := Config{GlobalCooldown: 5}

	got := GlobalCooldownRemaining(history, 50, cfg)
	if got != 3 {
		t.Errorf("global cooldown = %d, want 3", got)
	}
}

func TestGlobalCooldownRemaining_Clear(t *testing.T) {
	history := []AdjustmentRecord{
		{Agent: "planner", Iteration: 40},
	}
	cfg := Config{GlobalCooldown: 5}

	got := GlobalCooldownRemaining(history, 50, cfg)
	if got != 0 {
		t.Errorf("global cooldown = %d, want 0", got)
	}
}

func TestGlobalCooldownRemaining_Empty(t *testing.T) {
	cfg := Config{GlobalCooldown: 5}

	got := GlobalCooldownRemaining(nil, 50, cfg)
	if got != 0 {
		t.Errorf("global cooldown = %d, want 0", got)
	}
}

// --- InStabilizationWindow ---

func TestInStabilizationWindow_Inside(t *testing.T) {
	cfg := Config{StabilizationWindow: 10}
	if !InStabilizationWindow(5, cfg) {
		t.Error("iteration 5 should be inside window of 10")
	}
}

func TestInStabilizationWindow_Boundary(t *testing.T) {
	cfg := Config{StabilizationWindow: 10}
	if !InStabilizationWindow(9, cfg) {
		t.Error("iteration 9 should be inside window of 10 (< 10)")
	}
}

func TestInStabilizationWindow_Outside(t *testing.T) {
	cfg := Config{StabilizationWindow: 10}
	if InStabilizationWindow(10, cfg) {
		t.Error("iteration 10 should be outside window of 10 (>= 10)")
	}
}

func TestInStabilizationWindow_Zero(t *testing.T) {
	cfg := Config{StabilizationWindow: 10}
	if !InStabilizationWindow(0, cfg) {
		t.Error("iteration 0 should be inside window")
	}
}

// --- BuildReport ---

func TestBuildReport_AllFieldsPopulated(t *testing.T) {
	agents := []AgentBudgetState{
		{Name: "guardian", MaxIterations: 200, UsedIterations: 45, State: "Active"},
		{Name: "sysmon", MaxIterations: 150, UsedIterations: 38, State: "Active"},
		{Name: "implementer", MaxIterations: 100, UsedIterations: 85, State: "Active"},
	}
	pool := PoolState{
		TotalIterations:     450,
		UsedIterations:      168,
		RemainingIterations: 282,
		DailyCap:            5.0,
		ProjectedDailyPct:   60,
	}
	sysmon := &SysMonSummary{
		Severity:     "ok",
		ChainOK:      true,
		ActiveAgents: 3,
		EventRate:    15.0,
	}
	history := []AdjustmentRecord{
		{Agent: "implementer", Iteration: 45, Delta: 50, Reason: "productive"},
	}
	cfg := Config{AgentCooldown: 10, GlobalCooldown: 5, StabilizationWindow: 10}

	report := BuildReport(agents, pool, sysmon, history, cfg, 50)

	if report.Pool.TotalIterations != 450 {
		t.Errorf("Pool.TotalIterations = %d, want 450", report.Pool.TotalIterations)
	}
	if len(report.Agents) != 3 {
		t.Errorf("Agents len = %d, want 3", len(report.Agents))
	}
	if report.SysMonSummary == nil {
		t.Fatal("SysMonSummary is nil")
	}
	if report.SysMonSummary.Severity != "ok" {
		t.Errorf("SysMonSummary.Severity = %q, want ok", report.SysMonSummary.Severity)
	}
	if len(report.History) != 1 {
		t.Errorf("History len = %d, want 1", len(report.History))
	}

	// Cooldowns: implementer was adjusted at 45, current is 50, cooldown 10 → 5 remaining.
	if cd, ok := report.Cooldowns["implementer"]; !ok || cd != 5 {
		t.Errorf("Cooldowns[implementer] = %d, want 5", cd)
	}
	// guardian and sysmon: no history → 0.
	if cd := report.Cooldowns["guardian"]; cd != 0 {
		t.Errorf("Cooldowns[guardian] = %d, want 0", cd)
	}
	if cd := report.Cooldowns["sysmon"]; cd != 0 {
		t.Errorf("Cooldowns[sysmon] = %d, want 0", cd)
	}
}

func TestBuildReport_NilSysMon(t *testing.T) {
	agents := []AgentBudgetState{{Name: "guardian", MaxIterations: 200}}
	pool := PoolState{TotalIterations: 200}
	cfg := Config{AgentCooldown: 10}

	report := BuildReport(agents, pool, nil, nil, cfg, 5)
	if report.SysMonSummary != nil {
		t.Error("SysMonSummary should be nil")
	}
}
