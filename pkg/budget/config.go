package budget

import (
	"os"
	"strconv"
)

// Config holds all Allocator thresholds. Loaded from ALLOCATOR_* env vars
// with sensible defaults. Follows the same pattern as pkg/health.Config.
type Config struct {
	StabilizationWindow  int     // iterations: observe-only after boot
	AgentCooldown        int     // iterations: per-agent adjustment cooldown
	GlobalCooldown       int     // iterations: between any adjustments
	BudgetFloor          int     // minimum iterations per agent
	BudgetCeiling        int     // maximum iterations per agent
	InitialSpawnBudget   int     // default budget for newly spawned agents
	ConcentrationPct     float64 // single agent consuming > this % triggers review
	ExhaustionWarningPct float64 // agent at > this % of budget triggers increase
	IdleThresholdPct     float64 // agent using < this % across 3+ reports triggers decrease
	MarginalThresholdPct float64 // variance below this % is ignored
	DailyCapWarningPct   float64 // projected daily spend > this % triggers reduction
}

// DefaultConfig returns the default Allocator thresholds from the design spec.
func DefaultConfig() Config {
	return Config{
		StabilizationWindow:  10,
		AgentCooldown:        10,
		GlobalCooldown:       5,
		BudgetFloor:          20,
		BudgetCeiling:        500,
		InitialSpawnBudget:   50,
		ConcentrationPct:     40,
		ExhaustionWarningPct: 80,
		IdleThresholdPct:     10,
		MarginalThresholdPct: 5,
		DailyCapWarningPct:   90,
	}
}

// LoadConfig reads Allocator thresholds from ALLOCATOR_* environment variables,
// falling back to DefaultConfig() values for any that are unset or unparseable.
func LoadConfig() Config {
	cfg := DefaultConfig()
	cfg.StabilizationWindow = envInt("ALLOCATOR_STABILIZATION_WINDOW", cfg.StabilizationWindow)
	cfg.AgentCooldown = envInt("ALLOCATOR_AGENT_COOLDOWN", cfg.AgentCooldown)
	cfg.GlobalCooldown = envInt("ALLOCATOR_GLOBAL_COOLDOWN", cfg.GlobalCooldown)
	cfg.BudgetFloor = envInt("ALLOCATOR_BUDGET_FLOOR", cfg.BudgetFloor)
	cfg.BudgetCeiling = envInt("ALLOCATOR_BUDGET_CEILING", cfg.BudgetCeiling)
	cfg.InitialSpawnBudget = envInt("ALLOCATOR_INITIAL_SPAWN_BUDGET", cfg.InitialSpawnBudget)
	cfg.ConcentrationPct = envFloat("ALLOCATOR_CONCENTRATION_PCT", cfg.ConcentrationPct)
	cfg.ExhaustionWarningPct = envFloat("ALLOCATOR_EXHAUSTION_WARNING_PCT", cfg.ExhaustionWarningPct)
	cfg.IdleThresholdPct = envFloat("ALLOCATOR_IDLE_THRESHOLD_PCT", cfg.IdleThresholdPct)
	cfg.MarginalThresholdPct = envFloat("ALLOCATOR_MARGINAL_THRESHOLD_PCT", cfg.MarginalThresholdPct)
	cfg.DailyCapWarningPct = envFloat("ALLOCATOR_DAILY_CAP_WARNING_PCT", cfg.DailyCapWarningPct)
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

// envFloat reads a float64 from an environment variable, returning fallback
// if the variable is unset or cannot be parsed.
func envFloat(key string, fallback float64) float64 {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return v
}
