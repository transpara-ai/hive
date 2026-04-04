package budget

import (
	"encoding/json"
	"testing"
)

// --- JSON round-trip tests ---

func TestAgentBudgetState_JSONRoundTrip(t *testing.T) {
	orig := AgentBudgetState{
		Name:           "implementer",
		MaxIterations:  100,
		UsedIterations: 45,
		State:          "Active",
		BurnRate:       0.12,
		IdlePercent:    5.0,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AgentBudgetState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, orig)
	}
}

func TestPoolState_JSONRoundTrip(t *testing.T) {
	orig := PoolState{
		TotalIterations:     750,
		UsedIterations:      287,
		RemainingIterations: 463,
		DailyCost:           2.15,
		DailyCap:            5.0,
		BurnRatePerHour:     0.38,
		ProjectedDailyPct:   91.2,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PoolState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, orig)
	}
}

func TestAdjustmentRecord_JSONRoundTrip(t *testing.T) {
	orig := AdjustmentRecord{
		Agent:     "implementer",
		Iteration: 45,
		Delta:     50,
		Reason:    "high-value work in progress",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AdjustmentRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", got, orig)
	}
}

func TestSysMonSummary_JSONRoundTrip(t *testing.T) {
	orig := SysMonSummary{
		Severity:     "warning",
		ChainOK:      true,
		ActiveAgents: 4,
		EventRate:    18.3,
		Anomalies:    []string{"implementer consuming 52.3% of total"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got SysMonSummary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Severity != orig.Severity {
		t.Errorf("Severity = %q, want %q", got.Severity, orig.Severity)
	}
	if got.ChainOK != orig.ChainOK {
		t.Errorf("ChainOK = %v, want %v", got.ChainOK, orig.ChainOK)
	}
	if got.ActiveAgents != orig.ActiveAgents {
		t.Errorf("ActiveAgents = %d, want %d", got.ActiveAgents, orig.ActiveAgents)
	}
	if got.EventRate != orig.EventRate {
		t.Errorf("EventRate = %f, want %f", got.EventRate, orig.EventRate)
	}
	if len(got.Anomalies) != 1 || got.Anomalies[0] != orig.Anomalies[0] {
		t.Errorf("Anomalies = %v, want %v", got.Anomalies, orig.Anomalies)
	}
}

func TestBudgetReport_JSONRoundTrip(t *testing.T) {
	orig := BudgetReport{
		Pool: PoolState{
			TotalIterations:     750,
			UsedIterations:      287,
			RemainingIterations: 463,
		},
		Agents: []AgentBudgetState{
			{Name: "guardian", MaxIterations: 200, UsedIterations: 45, State: "Active"},
			{Name: "sysmon", MaxIterations: 150, UsedIterations: 38, State: "Active"},
		},
		SysMonSummary: &SysMonSummary{
			Severity:     "ok",
			ChainOK:      true,
			ActiveAgents: 2,
			EventRate:    10.0,
		},
		History: []AdjustmentRecord{
			{Agent: "implementer", Iteration: 45, Delta: 50, Reason: "productive"},
		},
		Cooldowns: map[string]int{
			"implementer": 3,
			"guardian":     0,
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got BudgetReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Agents) != 2 {
		t.Fatalf("Agents len = %d, want 2", len(got.Agents))
	}
	if got.Pool.TotalIterations != 750 {
		t.Errorf("Pool.TotalIterations = %d, want 750", got.Pool.TotalIterations)
	}
	if got.SysMonSummary == nil {
		t.Fatal("SysMonSummary is nil")
	}
	if got.SysMonSummary.Severity != "ok" {
		t.Errorf("SysMonSummary.Severity = %q, want %q", got.SysMonSummary.Severity, "ok")
	}
	if len(got.History) != 1 {
		t.Fatalf("History len = %d, want 1", len(got.History))
	}
	if got.Cooldowns["implementer"] != 3 {
		t.Errorf("Cooldowns[implementer] = %d, want 3", got.Cooldowns["implementer"])
	}
}

func TestBudgetReport_NilSysMonSummary(t *testing.T) {
	orig := BudgetReport{
		Pool:      PoolState{TotalIterations: 100},
		Agents:    []AgentBudgetState{},
		Cooldowns: map[string]int{},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got BudgetReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SysMonSummary != nil {
		t.Errorf("SysMonSummary should be nil, got %+v", got.SysMonSummary)
	}
}

// --- Config tests ---

func TestDefaultConfigMatchesDesignSpec(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"StabilizationWindow", cfg.StabilizationWindow, 10},
		{"AgentCooldown", cfg.AgentCooldown, 10},
		{"GlobalCooldown", cfg.GlobalCooldown, 5},
		{"BudgetFloor", cfg.BudgetFloor, 20},
		{"BudgetCeiling", cfg.BudgetCeiling, 500},
		{"InitialSpawnBudget", cfg.InitialSpawnBudget, 50},
		{"ConcentrationPct", cfg.ConcentrationPct, 40.0},
		{"ExhaustionWarningPct", cfg.ExhaustionWarningPct, 80.0},
		{"IdleThresholdPct", cfg.IdleThresholdPct, 10.0},
		{"MarginalThresholdPct", cfg.MarginalThresholdPct, 5.0},
		{"DailyCapWarningPct", cfg.DailyCapWarningPct, 90.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch g := tt.got.(type) {
			case int:
				if g != tt.want.(int) {
					t.Errorf("%s = %d, want %d", tt.name, g, tt.want.(int))
				}
			case float64:
				if g != tt.want.(float64) {
					t.Errorf("%s = %f, want %f", tt.name, g, tt.want.(float64))
				}
			}
		})
	}
}

func TestDefaultConfigAllFieldsNonZero(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.StabilizationWindow == 0 {
		t.Error("StabilizationWindow is zero")
	}
	if cfg.AgentCooldown == 0 {
		t.Error("AgentCooldown is zero")
	}
	if cfg.GlobalCooldown == 0 {
		t.Error("GlobalCooldown is zero")
	}
	if cfg.BudgetFloor == 0 {
		t.Error("BudgetFloor is zero")
	}
	if cfg.BudgetCeiling == 0 {
		t.Error("BudgetCeiling is zero")
	}
	if cfg.InitialSpawnBudget == 0 {
		t.Error("InitialSpawnBudget is zero")
	}
	if cfg.ConcentrationPct == 0 {
		t.Error("ConcentrationPct is zero")
	}
	if cfg.ExhaustionWarningPct == 0 {
		t.Error("ExhaustionWarningPct is zero")
	}
	if cfg.IdleThresholdPct == 0 {
		t.Error("IdleThresholdPct is zero")
	}
	if cfg.MarginalThresholdPct == 0 {
		t.Error("MarginalThresholdPct is zero")
	}
	if cfg.DailyCapWarningPct == 0 {
		t.Error("DailyCapWarningPct is zero")
	}
}

func TestLoadConfigPicksUpEnvVars(t *testing.T) {
	t.Setenv("ALLOCATOR_STABILIZATION_WINDOW", "15")
	t.Setenv("ALLOCATOR_AGENT_COOLDOWN", "20")
	t.Setenv("ALLOCATOR_GLOBAL_COOLDOWN", "8")
	t.Setenv("ALLOCATOR_BUDGET_FLOOR", "30")
	t.Setenv("ALLOCATOR_BUDGET_CEILING", "1000")
	t.Setenv("ALLOCATOR_INITIAL_SPAWN_BUDGET", "75")
	t.Setenv("ALLOCATOR_CONCENTRATION_PCT", "50")
	t.Setenv("ALLOCATOR_EXHAUSTION_WARNING_PCT", "85")
	t.Setenv("ALLOCATOR_IDLE_THRESHOLD_PCT", "15")
	t.Setenv("ALLOCATOR_MARGINAL_THRESHOLD_PCT", "8")
	t.Setenv("ALLOCATOR_DAILY_CAP_WARNING_PCT", "95")

	cfg := LoadConfig()

	if cfg.StabilizationWindow != 15 {
		t.Errorf("StabilizationWindow = %d, want 15", cfg.StabilizationWindow)
	}
	if cfg.AgentCooldown != 20 {
		t.Errorf("AgentCooldown = %d, want 20", cfg.AgentCooldown)
	}
	if cfg.GlobalCooldown != 8 {
		t.Errorf("GlobalCooldown = %d, want 8", cfg.GlobalCooldown)
	}
	if cfg.BudgetFloor != 30 {
		t.Errorf("BudgetFloor = %d, want 30", cfg.BudgetFloor)
	}
	if cfg.BudgetCeiling != 1000 {
		t.Errorf("BudgetCeiling = %d, want 1000", cfg.BudgetCeiling)
	}
	if cfg.InitialSpawnBudget != 75 {
		t.Errorf("InitialSpawnBudget = %d, want 75", cfg.InitialSpawnBudget)
	}
	if cfg.ConcentrationPct != 50 {
		t.Errorf("ConcentrationPct = %f, want 50", cfg.ConcentrationPct)
	}
	if cfg.ExhaustionWarningPct != 85 {
		t.Errorf("ExhaustionWarningPct = %f, want 85", cfg.ExhaustionWarningPct)
	}
	if cfg.IdleThresholdPct != 15 {
		t.Errorf("IdleThresholdPct = %f, want 15", cfg.IdleThresholdPct)
	}
	if cfg.MarginalThresholdPct != 8 {
		t.Errorf("MarginalThresholdPct = %f, want 8", cfg.MarginalThresholdPct)
	}
	if cfg.DailyCapWarningPct != 95 {
		t.Errorf("DailyCapWarningPct = %f, want 95", cfg.DailyCapWarningPct)
	}
}

func TestLoadConfigFallsBackToDefaults(t *testing.T) {
	// No env vars set — t.Setenv not called.
	cfg := LoadConfig()
	def := DefaultConfig()
	if cfg != def {
		t.Errorf("LoadConfig() without env vars = %+v, want %+v", cfg, def)
	}
}

func TestLoadConfigIgnoresInvalidValues(t *testing.T) {
	t.Setenv("ALLOCATOR_STABILIZATION_WINDOW", "not-a-number")
	t.Setenv("ALLOCATOR_CONCENTRATION_PCT", "bad-float")
	t.Setenv("ALLOCATOR_AGENT_COOLDOWN", "")

	cfg := LoadConfig()
	def := DefaultConfig()

	if cfg.StabilizationWindow != def.StabilizationWindow {
		t.Errorf("StabilizationWindow = %d, want default %d", cfg.StabilizationWindow, def.StabilizationWindow)
	}
	if cfg.ConcentrationPct != def.ConcentrationPct {
		t.Errorf("ConcentrationPct = %f, want default %f", cfg.ConcentrationPct, def.ConcentrationPct)
	}
	if cfg.AgentCooldown != def.AgentCooldown {
		t.Errorf("AgentCooldown = %d, want default %d", cfg.AgentCooldown, def.AgentCooldown)
	}
}
