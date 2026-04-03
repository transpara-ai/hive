package health

import (
	"testing"
)

func TestDefaultConfigMatchesDesignSpec(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  int
		want int
	}{
		{"ReportInterval", cfg.ReportInterval, 5},
		{"SelfcheckInterval", cfg.SelfcheckInterval, 20},
		{"BaselineWindow", cfg.BaselineWindow, 10},
		{"HeartbeatWarning", cfg.HeartbeatWarning, 2},
		{"HeartbeatCritical", cfg.HeartbeatCritical, 5},
		{"BudgetWarningPct", cfg.BudgetWarningPct, 80},
		{"BudgetCriticalPct", cfg.BudgetCriticalPct, 95},
		{"BudgetConcentrationPct", cfg.BudgetConcentrationPct, 40},
		{"IterationWarningPct", cfg.IterationWarningPct, 70},
		{"IterationCriticalPct", cfg.IterationCriticalPct, 90},
		{"ThroughputLowPct", cfg.ThroughputLowPct, 30},
		{"ThroughputHighPct", cfg.ThroughputHighPct, 300},
		{"ErrorMultiplier", cfg.ErrorMultiplier, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestDefaultConfigAllFieldsNonZero(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ReportInterval == 0 {
		t.Error("ReportInterval is zero")
	}
	if cfg.SelfcheckInterval == 0 {
		t.Error("SelfcheckInterval is zero")
	}
	if cfg.BaselineWindow == 0 {
		t.Error("BaselineWindow is zero")
	}
	if cfg.HeartbeatWarning == 0 {
		t.Error("HeartbeatWarning is zero")
	}
	if cfg.HeartbeatCritical == 0 {
		t.Error("HeartbeatCritical is zero")
	}
	if cfg.BudgetWarningPct == 0 {
		t.Error("BudgetWarningPct is zero")
	}
	if cfg.BudgetCriticalPct == 0 {
		t.Error("BudgetCriticalPct is zero")
	}
	if cfg.BudgetConcentrationPct == 0 {
		t.Error("BudgetConcentrationPct is zero")
	}
	if cfg.IterationWarningPct == 0 {
		t.Error("IterationWarningPct is zero")
	}
	if cfg.IterationCriticalPct == 0 {
		t.Error("IterationCriticalPct is zero")
	}
	if cfg.ThroughputLowPct == 0 {
		t.Error("ThroughputLowPct is zero")
	}
	if cfg.ThroughputHighPct == 0 {
		t.Error("ThroughputHighPct is zero")
	}
	if cfg.ErrorMultiplier == 0 {
		t.Error("ErrorMultiplier is zero")
	}
}

func TestLoadConfigPicksUpEnvVars(t *testing.T) {
	t.Setenv("SYSMON_REPORT_INTERVAL", "10")
	t.Setenv("SYSMON_SELFCHECK_INTERVAL", "50")
	t.Setenv("SYSMON_BASELINE_WINDOW", "20")
	t.Setenv("SYSMON_HEARTBEAT_WARNING", "3")
	t.Setenv("SYSMON_HEARTBEAT_CRITICAL", "8")
	t.Setenv("SYSMON_BUDGET_WARNING_PCT", "70")
	t.Setenv("SYSMON_BUDGET_CRITICAL_PCT", "90")
	t.Setenv("SYSMON_BUDGET_CONCENTRATION_PCT", "50")
	t.Setenv("SYSMON_ITERATION_WARNING_PCT", "60")
	t.Setenv("SYSMON_ITERATION_CRITICAL_PCT", "85")
	t.Setenv("SYSMON_THROUGHPUT_LOW_PCT", "20")
	t.Setenv("SYSMON_THROUGHPUT_HIGH_PCT", "400")
	t.Setenv("SYSMON_ERROR_MULTIPLIER", "3")

	cfg := LoadConfig()

	tests := []struct {
		name string
		got  int
		want int
	}{
		{"ReportInterval", cfg.ReportInterval, 10},
		{"SelfcheckInterval", cfg.SelfcheckInterval, 50},
		{"BaselineWindow", cfg.BaselineWindow, 20},
		{"HeartbeatWarning", cfg.HeartbeatWarning, 3},
		{"HeartbeatCritical", cfg.HeartbeatCritical, 8},
		{"BudgetWarningPct", cfg.BudgetWarningPct, 70},
		{"BudgetCriticalPct", cfg.BudgetCriticalPct, 90},
		{"BudgetConcentrationPct", cfg.BudgetConcentrationPct, 50},
		{"IterationWarningPct", cfg.IterationWarningPct, 60},
		{"IterationCriticalPct", cfg.IterationCriticalPct, 85},
		{"ThroughputLowPct", cfg.ThroughputLowPct, 20},
		{"ThroughputHighPct", cfg.ThroughputHighPct, 400},
		{"ErrorMultiplier", cfg.ErrorMultiplier, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestLoadConfigFallsBackToDefaults(t *testing.T) {
	// No env vars set — t.Setenv not called, so all SYSMON_* are unset.
	cfg := LoadConfig()
	def := DefaultConfig()

	if cfg != def {
		t.Errorf("LoadConfig() without env vars = %+v, want %+v", cfg, def)
	}
}

func TestLoadConfigIgnoresInvalidValues(t *testing.T) {
	t.Setenv("SYSMON_REPORT_INTERVAL", "not-a-number")
	t.Setenv("SYSMON_HEARTBEAT_WARNING", "")

	cfg := LoadConfig()
	def := DefaultConfig()

	if cfg.ReportInterval != def.ReportInterval {
		t.Errorf("ReportInterval = %d, want default %d", cfg.ReportInterval, def.ReportInterval)
	}
	if cfg.HeartbeatWarning != def.HeartbeatWarning {
		t.Errorf("HeartbeatWarning = %d, want default %d", cfg.HeartbeatWarning, def.HeartbeatWarning)
	}
}
