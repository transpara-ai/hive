package budget

import "fmt"

// CheckConcentration flags agents consuming more than ConcentrationPct of
// the total pool's used iterations. Returns human-readable warnings.
func CheckConcentration(agents []AgentBudgetState, pool PoolState, config Config) []string {
	if pool.UsedIterations == 0 {
		return nil
	}
	var warnings []string
	for _, a := range agents {
		pct := float64(a.UsedIterations) * 100.0 / float64(pool.UsedIterations)
		if pct > config.ConcentrationPct {
			warnings = append(warnings, fmt.Sprintf(
				"%s consuming %.1f%% of total (threshold: %.0f%%)",
				a.Name, pct, config.ConcentrationPct))
		}
	}
	return warnings
}

// CheckExhaustion flags agents whose used iterations exceed
// ExhaustionWarningPct of their max. Returns human-readable warnings.
func CheckExhaustion(agents []AgentBudgetState, config Config) []string {
	var warnings []string
	for _, a := range agents {
		if a.MaxIterations <= 0 {
			continue
		}
		pct := float64(a.UsedIterations) * 100.0 / float64(a.MaxIterations)
		if pct >= config.ExhaustionWarningPct {
			warnings = append(warnings, fmt.Sprintf(
				"%s at %.1f%% of budget (%d/%d)",
				a.Name, pct, a.UsedIterations, a.MaxIterations))
		}
	}
	return warnings
}

// CheckIdleAgents flags agents with utilization below IdleThresholdPct.
// Agents with State=="Quiesced" are EXCLUDED — they are waiting for work,
// not stuck. This is a direct lesson from SysMon graduation.
func CheckIdleAgents(agents []AgentBudgetState, config Config) []string {
	var warnings []string
	for _, a := range agents {
		if a.State == "Quiesced" {
			continue
		}
		if a.MaxIterations <= 0 {
			continue
		}
		pct := float64(a.UsedIterations) * 100.0 / float64(a.MaxIterations)
		if pct < config.IdleThresholdPct {
			warnings = append(warnings, fmt.Sprintf(
				"%s using %.1f%% of allocation (threshold: %.0f%%)",
				a.Name, pct, config.IdleThresholdPct))
		}
	}
	return warnings
}

// CheckDailyBurnRate flags if projected daily spend exceeds DailyCapWarningPct.
// Returns nil if within threshold or if daily cap is zero.
func CheckDailyBurnRate(pool PoolState, config Config) *string {
	if pool.DailyCap <= 0 {
		return nil
	}
	if pool.ProjectedDailyPct >= config.DailyCapWarningPct {
		msg := fmt.Sprintf(
			"projected daily spend at %.1f%% of cap (threshold: %.0f%%)",
			pool.ProjectedDailyPct, config.DailyCapWarningPct)
		return &msg
	}
	return nil
}

// CooldownRemaining returns the number of iterations until the named agent
// can be adjusted again. Returns 0 if no cooldown is active.
func CooldownRemaining(agent string, history []AdjustmentRecord, currentIter int, config Config) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Agent == agent {
			elapsed := currentIter - history[i].Iteration
			remaining := config.AgentCooldown - elapsed
			if remaining > 0 {
				return remaining
			}
			return 0
		}
	}
	return 0
}

// GlobalCooldownRemaining returns the number of iterations until any
// adjustment is allowed. Checks the most recent adjustment across all agents.
func GlobalCooldownRemaining(history []AdjustmentRecord, currentIter int, config Config) int {
	if len(history) == 0 {
		return 0
	}
	last := history[len(history)-1]
	elapsed := currentIter - last.Iteration
	remaining := config.GlobalCooldown - elapsed
	if remaining > 0 {
		return remaining
	}
	return 0
}

// InStabilizationWindow returns true if the current iteration is within the
// observe-only boot phase. The Allocator must not emit /budget commands
// during this window.
func InStabilizationWindow(currentIter int, config Config) bool {
	return currentIter < config.StabilizationWindow
}

// BuildReport assembles a BudgetReport from runtime data. Computes cooldowns
// for all agents and includes the provided data as-is.
func BuildReport(
	agents []AgentBudgetState,
	pool PoolState,
	sysmon *SysMonSummary,
	history []AdjustmentRecord,
	config Config,
	currentIteration int,
) BudgetReport {
	cooldowns := make(map[string]int, len(agents))
	for _, a := range agents {
		cooldowns[a.Name] = CooldownRemaining(a.Name, history, currentIteration, config)
	}
	return BudgetReport{
		Pool:          pool,
		Agents:        agents,
		SysMonSummary: sysmon,
		History:       history,
		Cooldowns:     cooldowns,
	}
}
