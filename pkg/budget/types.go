// Package budget provides types and pure functions for the Allocator agent's
// budget management. The Allocator observes per-agent consumption via BudgetRegistry
// snapshots, pre-computes metrics, and decides whether to redistribute iterations.
//
// This package contains no side effects — all functions are pure and testable.
// Data comes from the BudgetRegistry (cross-agent visibility) and bus events.
package budget

// AgentBudgetState represents one agent's current budget status.
// MaxIterations and State come from BudgetRegistry; UsedIterations from
// resources.Budget.Snapshot().Iterations.
type AgentBudgetState struct {
	Name           string  `json:"name"`
	MaxIterations  int     `json:"max_iterations"`
	UsedIterations int     `json:"used_iterations"`
	State          string  `json:"state"`       // "Active", "Quiesced", "Stopped"
	BurnRate       float64 `json:"burn_rate"`    // iterations per minute
	IdlePercent    float64 `json:"idle_percent"` // percentage of iterations spent idle
}

// PoolState represents the total budget pool across all agents.
type PoolState struct {
	TotalIterations     int     `json:"total_iterations"`
	UsedIterations      int     `json:"used_iterations"`
	RemainingIterations int     `json:"remaining_iterations"`
	DailyCost           float64 `json:"daily_cost"`
	DailyCap            float64 `json:"daily_cap"`
	BurnRatePerHour     float64 `json:"burn_rate_per_hour"`
	ProjectedDailyPct   float64 `json:"projected_daily_pct"`
}

// AdjustmentRecord tracks a previous budget adjustment for cooldown tracking.
type AdjustmentRecord struct {
	Agent     string `json:"agent"`
	Iteration int    `json:"iteration"`
	Delta     int    `json:"delta"`
	Reason    string `json:"reason"`
}

// SysMonSummary is a digest of the latest health.report event.
type SysMonSummary struct {
	Severity     string   `json:"severity"`
	ChainOK      bool     `json:"chain_ok"`
	ActiveAgents int      `json:"active_agents"`
	EventRate    float64  `json:"event_rate"`
	Anomalies    []string `json:"anomalies"`
}

// BudgetReport is the pre-computed summary given to the Allocator's LLM.
// Built by BuildReport() from registry snapshots, pool state, SysMon data,
// and adjustment history.
type BudgetReport struct {
	Pool          PoolState          `json:"pool"`
	Agents        []AgentBudgetState `json:"agents"`
	SysMonSummary *SysMonSummary     `json:"sysmon_summary"` // nil if no health.report seen yet
	History       []AdjustmentRecord `json:"history"`
	Cooldowns     map[string]int     `json:"cooldowns"` // agent name -> iterations remaining
}
