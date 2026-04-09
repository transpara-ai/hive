// Package checkpoint provides reboot survival infrastructure: thought formatting/parsing,
// heartbeat events, Open Brain integration, chain replay, and recovery orchestration.
// Core types (LoopSnapshot, BoundaryTrigger, format/parse) use pure stdlib.
// Heartbeat and replay modules import eventgraph and work for store access.
package checkpoint

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LoopSnapshot is a flat snapshot of an agent loop at a checkpoint boundary.
type LoopSnapshot struct {
	Role          string
	Iteration     int
	MaxIterations int
	TokensUsed    int
	CostUSD       float64

	// Signal is one of: ACTIVE, IDLE, ESCALATE, HALT
	Signal string

	// Task fields — empty when no task is active.
	CurrentTaskID string
	CurrentTask   string // title
	TaskStatus    string // assigned, in-progress, reviewing, blocked
}

// BoundaryTrigger describes what event caused the checkpoint to be captured.
type BoundaryTrigger string

const (
	TaskAssigned     BoundaryTrigger = "task_assigned"
	TaskCompleted    BoundaryTrigger = "task_completed"
	TaskBlocked      BoundaryTrigger = "task_blocked"
	StrategyChange   BoundaryTrigger = "strategy_change"
	ReviewCompleted  BoundaryTrigger = "review_completed"
	RoleProposed     BoundaryTrigger = "role_proposed"
	RoleDecided      BoundaryTrigger = "role_decided"
	GapEmitted       BoundaryTrigger = "gap_emitted"
	DirectiveEmitted BoundaryTrigger = "directive_emitted"
	BudgetAdjusted   BoundaryTrigger = "budget_adjusted"
	HaltSignal       BoundaryTrigger = "halt_signal"
)

// RebootSurvival classifies how well an agent survives a reboot.
type RebootSurvival string

const (
	SurvivalFull     RebootSurvival = "full"      // warm-started from Open Brain thought
	SurvivalRoleOnly RebootSurvival = "role-only"  // cold-started from chain replay
	SurvivalNone     RebootSurvival = "none"       // did not spawn
)

// AgentRole constants for role-specific recovery state routing.
// These match the Name/Role strings in agentdef.go StarterAgents.
const (
	RoleCTO      = "cto"
	RoleSpawner  = "spawner"
	RoleReviewer = "reviewer"
)

// SignalActive is the default signal value for agents that are running normally.
// Other signal values (IDLE, ESCALATE, HALT, TASK_DONE) are defined in pkg/loop.
const SignalActive = "ACTIVE"

// FormatCheckpoint produces a human-readable, embedding-friendly thought record.
//
// Format:
//
//	[CHECKPOINT] {role} agent -- iteration ~{N}, {RFC3339 timestamp}
//
//	STATUS: {signal}
//	BUDGET: {iteration}/{max} iterations, {tokens} tokens, ${cost}
//	TASK: {taskID} -- {title} -- {status}
//	INTENT: {intent}
//	NEXT: {next}
//	CONTEXT: {context}
//
// TASK, INTENT, NEXT, and CONTEXT lines are omitted when their values are empty.
func FormatCheckpoint(trigger BoundaryTrigger, snap LoopSnapshot, intent, next, context string) string {
	ts := time.Now().UTC().Format(time.RFC3339)

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "[CHECKPOINT] %s agent -- iteration ~%d, %s\n", snap.Role, snap.Iteration, ts)
	b.WriteByte('\n')

	// Fixed fields
	fmt.Fprintf(&b, "TRIGGER: %s\n", string(trigger))
	fmt.Fprintf(&b, "STATUS: %s\n", snap.Signal)
	fmt.Fprintf(&b, "BUDGET: %d/%d iterations, %d tokens, $%.2f\n",
		snap.Iteration, snap.MaxIterations, snap.TokensUsed, snap.CostUSD)

	// Optional fields
	if snap.CurrentTaskID != "" || snap.CurrentTask != "" || snap.TaskStatus != "" {
		fmt.Fprintf(&b, "TASK: %s -- %s -- %s\n", snap.CurrentTaskID, snap.CurrentTask, snap.TaskStatus)
	}
	if intent != "" {
		fmt.Fprintf(&b, "INTENT: %s\n", intent)
	}
	if next != "" {
		fmt.Fprintf(&b, "NEXT: %s\n", next)
	}
	if context != "" {
		fmt.Fprintf(&b, "CONTEXT: %s\n", context)
	}

	return b.String()
}

// ParsedCheckpoint holds the structured fields extracted from a checkpoint thought.
type ParsedCheckpoint struct {
	Role            string
	ApproxIteration int
	Timestamp       time.Time

	Trigger string
	Status  string
	Budget  string
	Task    string
	Intent  string
	Next    string
	Context string
}

// ParseCheckpoint extracts fields from a formatted checkpoint thought.
// It is tolerant of missing fields — absent fields are left as zero values.
// Returns an error only if the header line is present but unparseable.
func ParseCheckpoint(text string) (ParsedCheckpoint, error) {
	var p ParsedCheckpoint
	lines := strings.Split(text, "\n")

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "[CHECKPOINT]"):
			if err := parseHeader(line, &p); err != nil {
				return p, err
			}
		case strings.HasPrefix(line, "TRIGGER:"):
			p.Trigger = strings.TrimSpace(strings.TrimPrefix(line, "TRIGGER:"))
		case strings.HasPrefix(line, "STATUS:"):
			p.Status = strings.TrimSpace(strings.TrimPrefix(line, "STATUS:"))
		case strings.HasPrefix(line, "BUDGET:"):
			p.Budget = strings.TrimSpace(strings.TrimPrefix(line, "BUDGET:"))
		case strings.HasPrefix(line, "TASK:"):
			p.Task = strings.TrimSpace(strings.TrimPrefix(line, "TASK:"))
		case strings.HasPrefix(line, "INTENT:"):
			p.Intent = strings.TrimSpace(strings.TrimPrefix(line, "INTENT:"))
		case strings.HasPrefix(line, "NEXT:"):
			p.Next = strings.TrimSpace(strings.TrimPrefix(line, "NEXT:"))
		case strings.HasPrefix(line, "CONTEXT:"):
			p.Context = strings.TrimSpace(strings.TrimPrefix(line, "CONTEXT:"))
		}
	}

	return p, nil
}

// parseHeader parses: [CHECKPOINT] {role} agent -- iteration ~{N}, {timestamp}
func parseHeader(line string, p *ParsedCheckpoint) error {
	// Strip the "[CHECKPOINT] " prefix
	rest := strings.TrimPrefix(line, "[CHECKPOINT]")
	rest = strings.TrimSpace(rest)

	// Split on " -- " to separate "{role} agent" from "iteration ~{N}, {timestamp}"
	parts := strings.SplitN(rest, " -- ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("checkpoint: malformed header: %q", line)
	}

	// Extract role — strip trailing " agent"
	rolePart := strings.TrimSuffix(strings.TrimSpace(parts[0]), " agent")
	p.Role = strings.TrimSpace(rolePart)

	// Extract iteration and timestamp from the right side
	// Format: "iteration ~{N}, {timestamp}"
	right := strings.TrimSpace(parts[1])
	// Find the comma separating iteration from timestamp
	commaIdx := strings.Index(right, ", ")
	if commaIdx < 0 {
		return fmt.Errorf("checkpoint: malformed header right side: %q", right)
	}

	iterPart := strings.TrimSpace(right[:commaIdx])  // "iteration ~{N}"
	tsPart := strings.TrimSpace(right[commaIdx+2:])  // "{timestamp}"

	// Parse iteration number — strip "iteration ~"
	iterPart = strings.TrimPrefix(iterPart, "iteration ~")
	n, err := strconv.Atoi(iterPart)
	if err != nil {
		return fmt.Errorf("checkpoint: cannot parse iteration %q: %w", iterPart, err)
	}
	p.ApproxIteration = n

	// Parse timestamp
	ts, err := time.Parse(time.RFC3339, tsPart)
	if err != nil {
		return fmt.Errorf("checkpoint: cannot parse timestamp %q: %w", tsPart, err)
	}
	p.Timestamp = ts

	return nil
}

// AgentSummary is a compact view of a single agent's current state.
type AgentSummary struct {
	Role  string
	State string // e.g. idle, active, halted
}

// TaskStats summarises the open and completed task counts for the hive.
type TaskStats struct {
	Open      int
	Completed int
	Details   string // e.g. "task-77 in-progress"
}

// BudgetStats summarises token spend against the daily cap.
type BudgetStats struct {
	TotalSpend float64
	DailyCap   float64
}

// FormatHiveSummary produces a human-readable hive-wide summary for Open Brain capture.
//
// Format:
//
//	[HIVE SUMMARY] -- {count} agents active, 0 dynamic, {timestamp}
//
//	AGENTS: guardian(idle), implementer(active)
//	TASKS: 2 open (task-77 in-progress), 5 completed
//	BUDGET: $1.50 total spend, $8.50 remaining daily cap
func FormatHiveSummary(agents []AgentSummary, tasks TaskStats, budget BudgetStats) string {
	ts := time.Now().UTC().Format(time.RFC3339)

	var b strings.Builder

	fmt.Fprintf(&b, "[HIVE SUMMARY] -- %d agents active, 0 dynamic, %s\n", len(agents), ts)
	b.WriteByte('\n')

	// AGENTS line
	agentParts := make([]string, len(agents))
	for i, a := range agents {
		agentParts[i] = fmt.Sprintf("%s(%s)", a.Role, a.State)
	}
	fmt.Fprintf(&b, "AGENTS: %s\n", strings.Join(agentParts, ", "))

	// TASKS line
	if tasks.Details != "" {
		fmt.Fprintf(&b, "TASKS: %d open (%s), %d completed\n", tasks.Open, tasks.Details, tasks.Completed)
	} else {
		fmt.Fprintf(&b, "TASKS: %d open, %d completed\n", tasks.Open, tasks.Completed)
	}

	// BUDGET line
	remaining := budget.DailyCap - budget.TotalSpend
	fmt.Fprintf(&b, "BUDGET: $%.2f total spend, $%.2f remaining daily cap\n", budget.TotalSpend, remaining)

	return b.String()
}
