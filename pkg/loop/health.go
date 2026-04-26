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
// Field shapes match design v0.1.8 §5.1 / §5.2.
//
// Severity is the canonical lowercase enum reconciled in design v0.1.8 §7.1.
// The eventgraph AgentVitalReportedContent doc-comment still describes mixed
// case ("OK" | "Warning" | "Critical") — that doc is stale relative to v0.1.8
// and tracked for follow-up; the wire format is lowercase.
//
// AgentID is a raw string at the JSON-wire boundary, converted to
// types.ActorID at emit time. We deliberately avoid typing this field as
// types.ActorID: ActorID.UnmarshalJSON rejects empty values, which would
// drop the entire /health command on a single bad LLM-supplied agent_id.
// Validation happens per-vital in emitHealthReport so one bad entry only
// loses itself, not the whole cycle.
type AgentVital struct {
	AgentID               string          `json:"agent_id"`
	IterationsPct         float64         `json:"iterations_pct"`
	TrustScore            float64         `json:"trust_score"`
	BudgetBurnRatePerHour float64         `json:"budget_burn_rate_per_hour"`
	LastHeartbeatTicks    int64           `json:"last_heartbeat_ticks"`
	Severity              health.Severity `json:"severity"`
}

// HealthCommand represents the parsed /health command from LLM output.
//
// The four summary fields (Severity, ChainOK, ActiveAgents, EventRate) are
// preserved unchanged for the existing health.report consumers (cto.go,
// budget.go). AgentVitals is additive and arrives via SysMon's role-prompt
// instructing the LLM to emit per-agent vitals — a behavioral contract with
// no compile-time signal (see design A13).
//
// agentVitalsOmitted is set by parseHealthCommand when the JSON payload is
// missing the "agent_vitals" key entirely (distinct from "key present, value
// is []"). It drives the loud-warning log path in emitHealthReport that
// surfaces SysMon role-prompt regression. Unexported because it is not part
// of the wire contract.
type HealthCommand struct {
	Severity     string       `json:"severity"`
	ChainOK      bool         `json:"chain_ok"`
	ActiveAgents int          `json:"active_agents"`
	EventRate    float64      `json:"event_rate"`
	AgentVitals  []AgentVital `json:"agent_vitals,omitempty"`

	agentVitalsOmitted bool
}

// parseHealthCommand extracts the /health JSON payload from LLM output.
// Returns nil if no /health command found or JSON is malformed.
// Follows the same scanning pattern as parseTaskCommands.
//
// Detects whether the agent_vitals key was present in the raw JSON (distinct
// from "present but empty"); the omitted-vs-empty distinction drives the
// loud-warning path in emitHealthReport per design A13.
func parseHealthCommand(response string) *HealthCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/health ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/health ")
		// First-pass parse to detect raw key presence; second pass into the
		// typed struct. Cheaper than a custom UnmarshalJSON and keeps the
		// struct shape clean for callers and tests.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
			return nil
		}
		var cmd HealthCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		_, present := raw["agent_vitals"]
		cmd.agentVitalsOmitted = !present
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

// unmappedSeverityLogged tracks which non-canonical severity values have
// already been warned about, so we log once per unmapped value rather than
// per occurrence. Persistent across the process lifetime.
var unmappedSeverityLogged sync.Map

// normalizeAgentVitalSeverity normalizes the per-vital severity to one of the
// canonical health.Severity constants (matching the severity_level enum set
// reconciled in design v0.1.8 §7.1). Anything outside the set is normalized
// to SeverityWarning and logged once per unmapped value. We do not reject
// the entire /health command for one bad severity — better to keep the
// cycle's other vitals than drop everything.
func normalizeAgentVitalSeverity(s health.Severity) health.Severity {
	switch s {
	case health.SeverityOK, health.SeverityWarning, health.SeverityCritical:
		return s
	}
	if _, loaded := unmappedSeverityLogged.LoadOrStore(s, true); !loaded {
		fmt.Printf("[loop] WARN: unmapped agent vital severity %q; normalizing to %q\n",
			string(s), string(health.SeverityWarning))
	}
	return health.SeverityWarning
}

// emitHealthReport emits one health.report event followed by one
// agent.vital.reported event per entry in cmd.AgentVitals — see design
// v0.1.8 §5.2. The N agent.vital.reported events for a single cycle share
// the same cycle_id (UUID) on their HealthReportCycleID field so consumers
// can group them.
//
// Cycle correlation is currently one-sided: the umbrella health.report does
// NOT carry cycle_id (eventgraph's HealthReportContent has no such field).
// Adding it is tracked as a follow-up eventgraph change; until then the
// agent.vital.reported events are the sole carrier of cycle_id. The runtime
// canary in §5.5 works under this constraint by counting distinct cycle_ids
// across vitals and comparing the count to the number of recent
// health.report events on the chain — see TestEmitHealthReport_RuntimeCanary
// for the precise scope of what the canary verifies.
//
// Per-vital emit failures use warn-and-continue, matching the emitGap /
// emitDirective recover-and-continue pattern in cto.go: one bad vital
// (transient store error, panicking constructor) does not abort the rest of
// the cycle. The umbrella health.report has already been emitted by the
// time vitals are processed; a later partial failure leaves a known-shape
// gap on the chain rather than killing the entire cycle.
func (l *Loop) emitHealthReport(cmd *HealthCommand) error {
	if cmd.agentVitalsOmitted {
		// Loud-warning path: SysMon's role prompt requires the agent_vitals
		// field on every /health command (empty array allowed, key absent
		// not). Surface this so a regression of the prompt is visible in
		// stderr in addition to the canary test failure.
		fmt.Printf("[%s] WARN: /health command omitted agent_vitals key; "+
			"SysMon role-prompt may have regressed (see design A13)\n",
			l.agent.Name())
	}

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
		l.emitOneAgentVital(vital, cycleID)
	}

	return nil
}

// emitOneAgentVital emits a single agent.vital.reported event, recovering
// from constructor panics and logging-then-continuing on errors. Mirrors the
// emitGap / emitDirective pattern (cto.go) so one bad vital cannot abort the
// surrounding health cycle.
func (l *Loop) emitOneAgentVital(vital AgentVital, cycleID string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[%s] WARN: panic emitting agent.vital.reported for %q: %v\n",
				l.agent.Name(), vital.AgentID, r)
		}
	}()

	actorID, err := types.NewActorID(vital.AgentID)
	if err != nil {
		fmt.Printf("[%s] WARN: skipping vital with invalid agent_id %q: %v\n",
			l.agent.Name(), vital.AgentID, err)
		return
	}
	vc := event.AgentVitalReportedContent{
		AgentID:               actorID,
		IterationsPct:         vital.IterationsPct,
		TrustScore:            vital.TrustScore,
		BudgetBurnRatePerHour: vital.BudgetBurnRatePerHour,
		LastHeartbeatTicks:    vital.LastHeartbeatTicks,
		Severity:              string(normalizeAgentVitalSeverity(vital.Severity)),
		HealthReportCycleID:   cycleID,
	}
	if err := l.agent.EmitAgentVitalReported(vc); err != nil {
		fmt.Printf("[%s] WARN: emit agent.vital.reported for %s failed: %v\n",
			l.agent.Name(), vital.AgentID, err)
	}
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
