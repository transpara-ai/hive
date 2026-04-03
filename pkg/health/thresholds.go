package health

import (
	"os"
	"strconv"
)

// Config holds all configurable monitoring thresholds for SysMon.
type Config struct {
	ReportInterval      int // ticks between regular reports
	SelfcheckInterval   int // ticks between self-assessment
	BaselineWindow      int // ticks to establish baseline metrics
	HeartbeatWarning    int // ticks without event -> warning
	HeartbeatCritical   int // ticks without event -> critical
	BudgetWarningPct    int // projected daily % -> warning
	BudgetCriticalPct   int // projected daily % -> critical
	BudgetConcentrationPct int // single agent share % -> flag
	IterationWarningPct int // agent iteration burn % -> warning
	IterationCriticalPct int // agent iteration burn % -> critical
	ThroughputLowPct    int // below baseline % -> warning (quiet)
	ThroughputHighPct   int // above baseline % -> warning (storm)
	ErrorMultiplier     int // errors > Nx baseline -> alert
}

// DefaultConfig returns the default monitoring thresholds from the design spec.
func DefaultConfig() Config {
	return Config{
		ReportInterval:         5,
		SelfcheckInterval:      20,
		BaselineWindow:         10,
		HeartbeatWarning:       2,
		HeartbeatCritical:      5,
		BudgetWarningPct:       80,
		BudgetCriticalPct:      95,
		BudgetConcentrationPct: 40,
		IterationWarningPct:    70,
		IterationCriticalPct:   90,
		ThroughputLowPct:       30,
		ThroughputHighPct:      300,
		ErrorMultiplier:        2,
	}
}

// LoadConfig reads monitoring thresholds from SYSMON_* environment variables,
// falling back to DefaultConfig() values for any that are unset or unparseable.
func LoadConfig() Config {
	cfg := DefaultConfig()
	cfg.ReportInterval = envInt("SYSMON_REPORT_INTERVAL", cfg.ReportInterval)
	cfg.SelfcheckInterval = envInt("SYSMON_SELFCHECK_INTERVAL", cfg.SelfcheckInterval)
	cfg.BaselineWindow = envInt("SYSMON_BASELINE_WINDOW", cfg.BaselineWindow)
	cfg.HeartbeatWarning = envInt("SYSMON_HEARTBEAT_WARNING", cfg.HeartbeatWarning)
	cfg.HeartbeatCritical = envInt("SYSMON_HEARTBEAT_CRITICAL", cfg.HeartbeatCritical)
	cfg.BudgetWarningPct = envInt("SYSMON_BUDGET_WARNING_PCT", cfg.BudgetWarningPct)
	cfg.BudgetCriticalPct = envInt("SYSMON_BUDGET_CRITICAL_PCT", cfg.BudgetCriticalPct)
	cfg.BudgetConcentrationPct = envInt("SYSMON_BUDGET_CONCENTRATION_PCT", cfg.BudgetConcentrationPct)
	cfg.IterationWarningPct = envInt("SYSMON_ITERATION_WARNING_PCT", cfg.IterationWarningPct)
	cfg.IterationCriticalPct = envInt("SYSMON_ITERATION_CRITICAL_PCT", cfg.IterationCriticalPct)
	cfg.ThroughputLowPct = envInt("SYSMON_THROUGHPUT_LOW_PCT", cfg.ThroughputLowPct)
	cfg.ThroughputHighPct = envInt("SYSMON_THROUGHPUT_HIGH_PCT", cfg.ThroughputHighPct)
	cfg.ErrorMultiplier = envInt("SYSMON_ERROR_MULTIPLIER", cfg.ErrorMultiplier)
	return cfg
}

// envInt reads an integer from an environment variable, returning fallback
// if the variable is unset or cannot be parsed.
func envInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
