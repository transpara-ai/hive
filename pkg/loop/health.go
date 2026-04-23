package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/health"
)

// HealthCommand represents the parsed /health command from LLM output.
type HealthCommand struct {
	Severity     string  `json:"severity"`
	ChainOK      bool    `json:"chain_ok"`
	ActiveAgents int     `json:"active_agents"`
	EventRate    float64 `json:"event_rate"`
}

// parseHealthCommand extracts the /health JSON payload from LLM output.
// Returns nil if no /health command found or JSON is malformed.
// Follows the same scanning pattern as parseTaskCommands.
func parseHealthCommand(response string) *HealthCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/health ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/health ")
		var cmd HealthCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// severityToScore maps SysMon severity strings to eventgraph Score values.
func severityToScore(s string) types.Score {
	switch s {
	case "critical":
		return types.MustScore(0.0)
	case "warning":
		return types.MustScore(0.5)
	default:
		return types.MustScore(1.0)
	}
}

// emitHealthReport creates and records a health.report event on the chain.
// Delegates to Agent.EmitHealthReport which handles signing, causal chaining,
// and graph recording — following the same pattern as EmitBudgetAllocated.
func (l *Loop) emitHealthReport(cmd *HealthCommand) error {
	content := event.NewHealthReportContent(
		severityToScore(cmd.Severity),
		cmd.ChainOK,
		nil, // no primitive health map for SysMon reports
		cmd.ActiveAgents,
		cmd.EventRate,
	)

	if err := l.agent.EmitHealthReport(content); err != nil {
		return fmt.Errorf("emit health.report: %w", err)
	}

	fmt.Printf("[%s] emitted health.report (severity=%s, agents=%d, rate=%.1f)\n",
		l.agent.Name(), cmd.Severity, cmd.ActiveAgents, cmd.EventRate)
	return nil
}

// enrichHealthObservation appends pre-computed health metrics to the
// observation string for SysMon. Only activates for the "sysmon" role.
//
// Computes metrics from what's available on the Loop: own budget snapshot
// and pending event count. Each agent has its own Loop instance, so vitals
// for other agents are NOT available here — Haiku infers them from the
// agent.state.* and other events it receives via WatchPatterns.
func (l *Loop) enrichHealthObservation(obs string) string {
	if string(l.agent.Role()) != "sysmon" {
		return obs
	}

	cfg := health.LoadConfig()
	snap := l.budget.Snapshot()

	// Count pending events for throughput estimate.
	l.mu.Lock()
	eventCount := int64(len(l.pendingEvents))
	l.mu.Unlock()

	// Check throughput against baseline (self-referential on first pass).
	var anomalies []health.Anomaly
	throughputPct := float64(0)
	if eventCount > 0 {
		_, pct := health.CheckThroughput(eventCount, eventCount, cfg)
		throughputPct = pct
	}

	// Format the enrichment block.
	var sb strings.Builder
	sb.WriteString("\n=== HEALTH METRICS ===\n")

	sb.WriteString("BUDGET:\n")
	sb.WriteString(fmt.Sprintf("  tokens=%d cost=$%.2f iterations=%d\n",
		snap.TokensUsed, snap.CostUSD, snap.Iterations))

	sb.WriteString(fmt.Sprintf("\nHIVE:\n  throughput=%devents(%.0f%% baseline)\n",
		eventCount, throughputPct))

	if len(anomalies) > 0 {
		sb.WriteString("\nANOMALIES (pre-detected):\n")
		for _, a := range anomalies {
			sb.WriteString(fmt.Sprintf("  - [%s] %s: %s\n",
				strings.ToUpper(string(a.Severity)), a.Category, a.Description))
		}
	}

	sb.WriteString("===\n")
	return obs + sb.String()
}
