package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/budget"
)

// BudgetCommand represents the parsed /budget command from LLM output.
type BudgetCommand struct {
	Agent  string `json:"agent"`
	Action string `json:"action"` // "increase", "decrease", "set"
	Amount int    `json:"amount"`
	Reason string `json:"reason"`
}

// parseBudgetCommand extracts the /budget JSON payload from LLM output.
// Returns nil if no /budget command found or JSON is malformed.
func parseBudgetCommand(response string) *BudgetCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/budget ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/budget ")
		var cmd BudgetCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// validateBudgetCommand checks all safety constraints before applying.
// Returns nil if valid, descriptive error if rejected.
func (l *Loop) validateBudgetCommand(cmd *BudgetCommand, iteration int) error {
	cfg := budget.LoadConfig()

	// 1. Stabilization window.
	if budget.InStabilizationWindow(iteration, cfg) {
		return fmt.Errorf("stabilization window active (iteration %d < %d)", iteration, cfg.StabilizationWindow)
	}

	// 2. Agent exists.
	reg := l.config.BudgetRegistry
	if reg == nil {
		return fmt.Errorf("no budget registry available")
	}
	found := false
	for _, e := range reg.Snapshot() {
		if e.Name == cmd.Agent {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown agent: %s", cmd.Agent)
	}

	// 3. Amount > 0.
	if cmd.Amount <= 0 {
		return fmt.Errorf("amount must be positive, got %d", cmd.Amount)
	}

	// 4. Valid action.
	switch cmd.Action {
	case "increase", "decrease", "set":
	default:
		return fmt.Errorf("invalid action %q (must be increase, decrease, or set)", cmd.Action)
	}

	// 5. Global cooldown.
	globalRemaining := budget.GlobalCooldownRemaining(l.adjustmentHistory, iteration, cfg)
	if globalRemaining > 0 {
		return fmt.Errorf("global cooldown active (%d iterations remaining)", globalRemaining)
	}

	// 6. Agent cooldown.
	agentRemaining := budget.CooldownRemaining(cmd.Agent, l.adjustmentHistory, iteration, cfg)
	if agentRemaining > 0 {
		return fmt.Errorf("cooldown active for %s (%d iterations remaining)", cmd.Agent, agentRemaining)
	}

	// 7. Pool headroom for increases.
	if cmd.Action == "increase" {
		totalPool := reg.TotalPool()
		totalUsed := reg.TotalUsed()
		headroom := totalPool - totalUsed
		if cmd.Amount > headroom {
			return fmt.Errorf("insufficient pool headroom: want %d, available %d", cmd.Amount, headroom)
		}
	}

	return nil
}

// applyBudgetAdjustment modifies the target agent's budget and emits a chain event.
func (l *Loop) applyBudgetAdjustment(cmd *BudgetCommand, iteration int) error {
	cfg := budget.LoadConfig()
	reg := l.config.BudgetRegistry

	// Compute delta based on action.
	// For "set", we need to find the current value first.
	var delta int
	switch cmd.Action {
	case "increase":
		delta = cmd.Amount
	case "decrease":
		delta = -cmd.Amount
	case "set":
		// Find current max to compute delta.
		for _, e := range reg.Snapshot() {
			if e.Name == cmd.Agent {
				delta = cmd.Amount - e.MaxIterations
				break
			}
		}
	}

	prev, newMax, err := reg.AdjustMaxIterations(cmd.Agent, delta, cfg.BudgetFloor, cfg.BudgetCeiling)
	if err != nil {
		return fmt.Errorf("adjust %s: %w", cmd.Agent, err)
	}

	// Log if floor/ceiling clamped.
	actualDelta := newMax - prev
	if actualDelta != delta {
		fmt.Printf("[%s] budget adjustment clamped: requested delta=%d, actual delta=%d (floor=%d, ceiling=%d)\n",
			l.agent.Name(), delta, actualDelta, cfg.BudgetFloor, cfg.BudgetCeiling)
	}

	// Emit agent.budget.adjusted event on chain.
	content := event.AgentBudgetAdjustedContent{
		AgentID:        l.agent.ID(),
		AgentName:      cmd.Agent,
		Action:         cmd.Action,
		PreviousBudget: prev,
		NewBudget:      newMax,
		Delta:          actualDelta,
		Reason:         cmd.Reason,
		PoolRemaining:  reg.TotalPool() - reg.TotalUsed(),
	}

	// Use the Allocator agent's ID as the event source, since the Allocator
	// is the agent emitting this event, not the target agent.
	if err := l.agent.EmitBudgetAdjusted(content); err != nil {
		return fmt.Errorf("emit budget.adjusted: %w", err)
	}

	// Record adjustment for cooldown tracking.
	l.adjustmentHistory = append(l.adjustmentHistory, budget.AdjustmentRecord{
		Agent:     cmd.Agent,
		Iteration: iteration,
		Delta:     actualDelta,
		Reason:    cmd.Reason,
	})

	fmt.Printf("[%s] budget adjusted: %s %s %+d (%d→%d) reason=%q\n",
		l.agent.Name(), cmd.Agent, cmd.Action, actualDelta, prev, newMax, cmd.Reason)
	return nil
}

// enrichBudgetObservation appends pre-computed budget metrics to the
// observation string for the Allocator. Only activates for the "allocator" role.
// Data source: BudgetRegistry (NOT the self-referential SysMon path).
func (l *Loop) enrichBudgetObservation(obs string, iteration int) string {
	if string(l.agent.Role()) != "allocator" {
		return obs
	}
	reg := l.config.BudgetRegistry
	if reg == nil {
		return obs
	}

	cfg := budget.LoadConfig()
	entries := reg.Snapshot()

	var sb strings.Builder
	sb.WriteString("\n=== BUDGET METRICS ===\n")

	// Pool summary.
	totalPool := reg.TotalPool()
	totalUsed := reg.TotalUsed()
	remaining := totalPool - totalUsed
	usedPct := float64(0)
	remainPct := float64(0)
	if totalPool > 0 {
		usedPct = float64(totalUsed) * 100.0 / float64(totalPool)
		remainPct = float64(remaining) * 100.0 / float64(totalPool)
	}
	sb.WriteString(fmt.Sprintf("POOL:\n  total_iterations=%d used=%d(%.1f%%) remaining=%d(%.1f%%)\n",
		totalPool, totalUsed, usedPct, remaining, remainPct))

	// Per-agent detail.
	sb.WriteString("\nAGENTS:\n")
	for _, e := range entries {
		snap := e.Budget.Snapshot()
		pct := float64(0)
		if e.MaxIterations > 0 {
			pct = float64(snap.Iterations) * 100.0 / float64(e.MaxIterations)
		}
		sb.WriteString(fmt.Sprintf("  %-14s max=%-4d used=%-4d(%.1f%%)  state=%-10s\n",
			e.Name+":", e.MaxIterations, snap.Iterations, pct, e.AgentState))
	}

	// SysMon summary from recent pending events (extract last health.report).
	l.mu.Lock()
	var lastHealthSeverity string
	var lastHealthAgents int
	var lastHealthRate float64
	var lastHealthChainOK bool
	foundHealth := false
	for i := len(l.pendingEvents) - 1; i >= 0; i-- {
		if l.pendingEvents[i].Type() == types.MustEventType("health.report") {
			foundHealth = true
			// Extract from content if possible; fallback to defaults.
			if hrc, ok := l.pendingEvents[i].Content().(event.HealthReportContent); ok {
				v := hrc.Overall.Value()
				if v >= 0.9 {
					lastHealthSeverity = "ok"
				} else if v >= 0.4 {
					lastHealthSeverity = "warning"
				} else {
					lastHealthSeverity = "critical"
				}
				lastHealthChainOK = hrc.ChainIntegrity
				lastHealthAgents = hrc.ActiveActors
				lastHealthRate = hrc.EventRate
			}
			break
		}
	}
	l.mu.Unlock()

	if foundHealth {
		sb.WriteString(fmt.Sprintf("\nSYSMON SUMMARY (last report):\n  severity=%s chain_ok=%t active_agents=%d event_rate=%.1f\n",
			lastHealthSeverity, lastHealthChainOK, lastHealthAgents, lastHealthRate))
	}

	// Adjustment history.
	if len(l.adjustmentHistory) > 0 {
		sb.WriteString("\nADJUSTMENT HISTORY (last 5):\n")
		start := 0
		if len(l.adjustmentHistory) > 5 {
			start = len(l.adjustmentHistory) - 5
		}
		for _, rec := range l.adjustmentHistory[start:] {
			sb.WriteString(fmt.Sprintf("  iter=%d: %s %+d (reason: %s)\n",
				rec.Iteration, rec.Agent, rec.Delta, rec.Reason))
		}
	}

	// Cooldowns.
	sb.WriteString("\nCOOLDOWNS:\n")
	for _, e := range entries {
		cd := budget.CooldownRemaining(e.Name, l.adjustmentHistory, iteration, cfg)
		if cd > 0 {
			sb.WriteString(fmt.Sprintf("  %s: %d iterations remaining\n", e.Name, cd))
		} else {
			sb.WriteString(fmt.Sprintf("  %s: clear\n", e.Name))
		}
	}

	sb.WriteString("===\n")
	return obs + sb.String()
}
