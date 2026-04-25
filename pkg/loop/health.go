package loop

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/health"
)

// AgentVital is one entry in HealthCommand.AgentVitals — SysMon's per-agent
// slice of a health-report cycle as emitted in the LLM's /health text command.
// Field shapes match design v0.1.8 §5.1 / §5.2; severity values are lowercase
// per A13 ("ok" | "warning" | "critical"), matching the in-code
// pkg/health.Severity convention.
type AgentVital struct {
	AgentID               string  `json:"agent_id"`
	IterationsPct         float64 `json:"iterations_pct"`
	TrustScore            float64 `json:"trust_score"`
	BudgetBurnRatePerHour float64 `json:"budget_burn_rate_per_hour"`
	LastHeartbeatTicks    int64   `json:"last_heartbeat_ticks"`
	Severity              string  `json:"severity"`
}

// HealthCommand represents the parsed /health command from LLM output.
//
// The four summary fields (Severity, ChainOK, ActiveAgents, EventRate) are
// preserved unchanged for the existing health.report consumers (cto.go,
// budget.go). AgentVitals is additive and arrives via SysMon's role-prompt
// instructing the LLM to emit per-agent vitals — a behavioral contract with
// no compile-time signal (see design A13).
type HealthCommand struct {
	Severity     string       `json:"severity"`
	ChainOK      bool         `json:"chain_ok"`
	ActiveAgents int          `json:"active_agents"`
	EventRate    float64      `json:"event_rate"`
	AgentVitals  []AgentVital `json:"agent_vitals,omitempty"`
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

// unmappedSeverityLogged tracks which non-canonical severity strings have
// already been warned about, so we log once per unmapped value rather than
// per occurrence. Persistent across the process lifetime.
var unmappedSeverityLogged sync.Map

// normalizeAgentVitalSeverity normalizes the severity string on a per-agent
// vital to one of "ok" | "warning" | "critical" (matching the severity_level
// enum set per design §7.1). Anything outside this set is normalized to
// "warning" and logged once per unmapped value. We do not reject the entire
// /health command for one bad severity — better to keep the cycle's other
// vitals than drop everything.
func normalizeAgentVitalSeverity(s string) string {
	switch s {
	case "ok", "warning", "critical":
		return s
	}
	if _, loaded := unmappedSeverityLogged.LoadOrStore(s, true); !loaded {
		fmt.Printf("[loop] WARN: unmapped agent vital severity %q; normalizing to \"warning\"\n", s)
	}
	return "warning"
}

// emitHealthReport emits one health.report event followed by one
// agent.vital.reported event per entry in cmd.AgentVitals. All N+1 events
// share the same cycle_id (UUID) so the exporter can correlate per-agent
// vitals back to the umbrella report — see design v0.1.8 §5.2.
//
// The existing health.report content shape is unchanged; cycle_id lives on
// the agent.vital.reported events as HealthReportCycleID. (Adding a CycleID
// field to eventgraph's HealthReportContent is a follow-up eventgraph PR;
// this implementation defers that and uses the agent.vital.reported event's
// HealthReportCycleID as the sole carrier of cycle correlation. The runtime
// canary in §5.5 still works: it groups agent.vital.reported events by
// cycle_id and counts cycles, comparing against the count of recent
// health.report events.)
func (l *Loop) emitHealthReport(cmd *HealthCommand) error {
	cycleID := uuid.New().String()

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

	fmt.Printf("[%s] emitted health.report (severity=%s, agents=%d, rate=%.1f, cycle=%s, vitals=%d)\n",
		l.agent.Name(), cmd.Severity, cmd.ActiveAgents, cmd.EventRate, cycleID, len(cmd.AgentVitals))

	for _, vital := range cmd.AgentVitals {
		actorID, err := types.NewActorID(vital.AgentID)
		if err != nil {
			fmt.Printf("[%s] WARN: skipping vital with invalid agent_id %q: %v\n",
				l.agent.Name(), vital.AgentID, err)
			continue
		}
		vc := event.AgentVitalReportedContent{
			AgentID:               actorID,
			IterationsPct:         vital.IterationsPct,
			TrustScore:            vital.TrustScore,
			BudgetBurnRatePerHour: vital.BudgetBurnRatePerHour,
			LastHeartbeatTicks:    vital.LastHeartbeatTicks,
			Severity:              normalizeAgentVitalSeverity(vital.Severity),
			HealthReportCycleID:   cycleID,
		}
		if err := l.agent.EmitAgentVitalReported(vc); err != nil {
			return fmt.Errorf("emit agent.vital.reported for %s: %w", vital.AgentID, err)
		}
	}

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
