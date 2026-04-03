package health

import "fmt"

// ClassifyHeartbeat returns the liveness status for an agent based on how many
// ticks have elapsed since its last event. A negative value means no events
// have ever been observed.
func ClassifyHeartbeat(ticksSinceLastEvent int64, cfg Config) HeartbeatStatus {
	if ticksSinceLastEvent < 0 {
		return HeartbeatSilent
	}
	if ticksSinceLastEvent >= int64(cfg.HeartbeatCritical) {
		return HeartbeatCritical
	}
	if ticksSinceLastEvent >= int64(cfg.HeartbeatWarning) {
		return HeartbeatWarning
	}
	return HeartbeatOK
}

// ClassifyIterationBurn returns the severity for an agent's iteration
// consumption. If max is zero (unlimited), always returns OK.
func ClassifyIterationBurn(used, max int, cfg Config) Severity {
	if max <= 0 {
		return SeverityOK
	}
	pct := float64(used) * 100.0 / float64(max)
	if pct >= float64(cfg.IterationCriticalPct) {
		return SeverityCritical
	}
	if pct >= float64(cfg.IterationWarningPct) {
		return SeverityWarning
	}
	return SeverityOK
}

// CheckBudgetProjection evaluates daily budget consumption by projecting
// current spend to end-of-day. Returns the severity and the projected
// daily percentage. If dailyCapUSD or hoursElapsed is zero, returns OK
// with the raw daily percentage.
func CheckBudgetProjection(costUSD, dailyCapUSD, hoursElapsed float64, cfg Config) (Severity, float64) {
	if dailyCapUSD <= 0 {
		return SeverityOK, 0
	}

	var projectedPct float64
	if hoursElapsed > 0 {
		burnRate := costUSD / hoursElapsed
		projectedDaily := burnRate * 24.0
		projectedPct = projectedDaily * 100.0 / dailyCapUSD
	} else {
		projectedPct = costUSD * 100.0 / dailyCapUSD
	}

	if projectedPct >= float64(cfg.BudgetCriticalPct) {
		return SeverityCritical, projectedPct
	}
	if projectedPct >= float64(cfg.BudgetWarningPct) {
		return SeverityWarning, projectedPct
	}
	return SeverityOK, projectedPct
}

// CheckBudgetConcentration checks whether any single agent consumes more
// than the given threshold percentage of total iterations across all agents.
// Returns anomalies for any agent exceeding the threshold.
func CheckBudgetConcentration(vitals []AgentVital, thresholdPct float64) []Anomaly {
	var totalIterations int
	for _, v := range vitals {
		totalIterations += v.IterationsUsed
	}
	if totalIterations == 0 {
		return nil
	}

	var anomalies []Anomaly
	for _, v := range vitals {
		pct := float64(v.IterationsUsed) * 100.0 / float64(totalIterations)
		if pct > thresholdPct {
			anomalies = append(anomalies, Anomaly{
				Severity:    SeverityWarning,
				Category:    "budget",
				Agent:       v.Name,
				Description: fmt.Sprintf("budget concentration at %.0f%% (threshold %.0f%%)", pct, thresholdPct),
			})
		}
	}
	return anomalies
}

// CheckThroughput compares current event throughput against the established
// baseline. Returns severity and the throughput as a percentage of baseline.
// If baseline is zero (not yet established), returns OK.
func CheckThroughput(current, baseline int64, cfg Config) (Severity, float64) {
	if baseline <= 0 {
		return SeverityOK, 0
	}
	pct := float64(current) * 100.0 / float64(baseline)
	if pct < float64(cfg.ThroughputLowPct) {
		return SeverityWarning, pct
	}
	if pct > float64(cfg.ThroughputHighPct) {
		return SeverityWarning, pct
	}
	return SeverityOK, pct
}

// ComputeOverallSeverity returns the worst severity across all anomalies.
// Uses worst-child semantics: if any anomaly is Critical, the overall
// result is Critical. Same pattern as Transpara's KPI status roll-ups.
func ComputeOverallSeverity(anomalies []Anomaly) Severity {
	if len(anomalies) == 0 {
		return SeverityOK
	}
	for _, a := range anomalies {
		if a.Severity == SeverityCritical {
			return SeverityCritical
		}
	}
	return SeverityWarning
}

// BuildReportInput holds the raw data needed to assemble a MonitorReport.
type BuildReportInput struct {
	Tick               int64
	Interval           int64
	AgentVitals        []AgentVital
	Budget             BudgetHealth
	Hive               HiveHealth
	AdditionalAnomalies []Anomaly
	Cfg                Config
}

// BuildReport assembles a complete MonitorReport from raw inputs.
// It runs all anomaly checks and computes the overall severity.
func BuildReport(in BuildReportInput) MonitorReport {
	var anomalies []Anomaly

	// Check agent vitals for anomalies.
	for _, v := range in.AgentVitals {
		if v.Heartbeat == HeartbeatCritical || v.Heartbeat == HeartbeatSilent {
			anomalies = append(anomalies, Anomaly{
				Severity:    SeverityCritical,
				Category:    "agent",
				Agent:       v.Name,
				Description: fmt.Sprintf("heartbeat %s", v.Heartbeat),
			})
		} else if v.Heartbeat == HeartbeatWarning {
			anomalies = append(anomalies, Anomaly{
				Severity:    SeverityWarning,
				Category:    "agent",
				Agent:       v.Name,
				Description: fmt.Sprintf("heartbeat %s", v.Heartbeat),
			})
		}

		iterSeverity := ClassifyIterationBurn(v.IterationsUsed, v.IterationsMax, in.Cfg)
		if iterSeverity != SeverityOK {
			anomalies = append(anomalies, Anomaly{
				Severity:    iterSeverity,
				Category:    "agent",
				Agent:       v.Name,
				Description: fmt.Sprintf("iteration burn at %.0f%%", v.IterationsPct),
			})
		}
	}

	// Check budget concentration.
	concentrationAnomalies := CheckBudgetConcentration(
		in.AgentVitals,
		float64(in.Cfg.BudgetConcentrationPct),
	)
	anomalies = append(anomalies, concentrationAnomalies...)

	// Check budget projection.
	if in.Budget.ProjectedDailyPct >= float64(in.Cfg.BudgetCriticalPct) {
		anomalies = append(anomalies, Anomaly{
			Severity:    SeverityCritical,
			Category:    "budget",
			Description: fmt.Sprintf("projected daily spend at %.0f%%", in.Budget.ProjectedDailyPct),
		})
	} else if in.Budget.ProjectedDailyPct >= float64(in.Cfg.BudgetWarningPct) {
		anomalies = append(anomalies, Anomaly{
			Severity:    SeverityWarning,
			Category:    "budget",
			Description: fmt.Sprintf("projected daily spend at %.0f%%", in.Budget.ProjectedDailyPct),
		})
	}

	// Check budget exhaustion events.
	if in.Budget.ExhaustionCount > 0 {
		anomalies = append(anomalies, Anomaly{
			Severity:    SeverityCritical,
			Category:    "budget",
			Description: fmt.Sprintf("%d budget exhaustion events", in.Budget.ExhaustionCount),
		})
	}

	// Check throughput.
	throughputSeverity, _ := CheckThroughput(
		in.Hive.EventThroughput,
		in.Hive.ThroughputBaseline,
		in.Cfg,
	)
	if throughputSeverity != SeverityOK {
		anomalies = append(anomalies, Anomaly{
			Severity:    throughputSeverity,
			Category:    "hive",
			Description: fmt.Sprintf("event throughput at %.0f%% of baseline", in.Hive.ThroughputPct),
		})
	}

	// Check agent count.
	if in.Hive.AgentsExpected > 0 && in.Hive.AgentsActive < in.Hive.AgentsExpected {
		sev := SeverityWarning
		if in.Hive.AgentsActive <= 2 {
			sev = SeverityCritical
		}
		anomalies = append(anomalies, Anomaly{
			Severity:    sev,
			Category:    "hive",
			Description: fmt.Sprintf("agents active %d/%d", in.Hive.AgentsActive, in.Hive.AgentsExpected),
		})
	}

	// Append any additional anomalies from the caller.
	anomalies = append(anomalies, in.AdditionalAnomalies...)

	return MonitorReport{
		Tick:        in.Tick,
		Interval:    in.Interval,
		Severity:    ComputeOverallSeverity(anomalies),
		AgentVitals: in.AgentVitals,
		Budget:      in.Budget,
		Hive:        in.Hive,
		Anomalies:   anomalies,
	}
}
