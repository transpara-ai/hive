package loop

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/checkpoint"
)

// GapCommand represents the parsed /gap command from CTO LLM output.
type GapCommand struct {
	Category    string `json:"category"`     // "leadership", "technical", "process", "staffing", "capability"
	MissingRole string `json:"missing_role"` // suggested kebab-case role name
	Evidence    string `json:"evidence"`     // what the CTO observed
	Severity    string `json:"severity"`     // "info", "warning", "serious", "critical"
}

// DirectiveCommand represents the parsed /directive command from CTO LLM output.
type DirectiveCommand struct {
	Target   string `json:"target"`   // agent name or "all"
	Action   string `json:"action"`   // what to do
	Reason   string `json:"reason"`   // why
	Priority string `json:"priority"` // "low", "medium", "high"
}

// parseGapCommand extracts the /gap JSON payload from LLM output.
// Returns nil if no /gap command found or JSON is malformed.
// Follows the same scanning pattern as parseHealthCommand.
func parseGapCommand(response string) *GapCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/gap ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/gap ")
		var cmd GapCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// parseDirectiveCommand extracts the /directive JSON payload from LLM output.
// Returns nil if no /directive command found or JSON is malformed.
// Follows the same scanning pattern as parseHealthCommand.
func parseDirectiveCommand(response string) *DirectiveCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/directive ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/directive ")
		var cmd DirectiveCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// CTOConfig holds all CTO thresholds. Loaded from CTO_* env vars
// with sensible defaults from the design spec section 11.
type CTOConfig struct {
	StabilizationWindow int      // iterations: observe-only after boot
	GapCooldown         int      // iterations: between /gap in same category
	DirectiveCooldown   int      // iterations: between /directive to same target
	ValidCategories     []string // allowed gap categories
}

// DefaultCTOConfig returns the default CTO thresholds from design spec section 11.
func DefaultCTOConfig() CTOConfig {
	return CTOConfig{
		StabilizationWindow: 15,
		GapCooldown:         15,
		DirectiveCooldown:   5,
		ValidCategories:     []string{"leadership", "technical", "process", "staffing", "capability"},
	}
}

// LoadCTOConfig reads CTO thresholds from CTO_* environment variables,
// falling back to DefaultCTOConfig() values for any that are unset or unparseable.
func LoadCTOConfig() CTOConfig {
	cfg := DefaultCTOConfig()
	cfg.StabilizationWindow = ctoEnvInt("CTO_STABILIZATION_WINDOW", cfg.StabilizationWindow)
	cfg.GapCooldown = ctoEnvInt("CTO_GAP_COOLDOWN", cfg.GapCooldown)
	cfg.DirectiveCooldown = ctoEnvInt("CTO_DIRECTIVE_COOLDOWN", cfg.DirectiveCooldown)
	if v := os.Getenv("CTO_GAP_CATEGORIES"); v != "" {
		cfg.ValidCategories = strings.Split(v, ",")
	}
	return cfg
}

func ctoEnvInt(key string, fallback int) int {
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

// CTOCooldowns tracks per-category and per-target emission history
// for the CTO agent. Only accessed from the Run() goroutine.
type CTOCooldowns struct {
	gapByCategory     map[string]int  // category → last emission iteration
	directiveByTarget map[string]int  // target → last emission iteration
	emittedGaps       map[string]bool // missing_role → already emitted this session
}

// NewCTOCooldowns creates a zeroed CTOCooldowns.
func NewCTOCooldowns() *CTOCooldowns {
	return &CTOCooldowns{
		gapByCategory:     make(map[string]int),
		directiveByTarget: make(map[string]int),
		emittedGaps:       make(map[string]bool),
	}
}

// InitCTOFromRecovery seeds CTO cooldown state from chain replay.
func (c *CTOCooldowns) InitCTOFromRecovery(state *checkpoint.CTORecoveredState) {
	if state == nil {
		return
	}
	for k, v := range state.GapByCategory {
		c.gapByCategory[k] = v
	}
	for k, v := range state.DirectiveByTarget {
		c.directiveByTarget[k] = v
	}
	for k, v := range state.EmittedGaps {
		c.emittedGaps[k] = v
	}
}

// validateGapCommand checks all safety constraints before emitting a gap event.
// Returns nil if valid, descriptive error if rejected.
func validateGapCommand(cmd *GapCommand, iteration int, cooldowns *CTOCooldowns, cfg CTOConfig) error {
	// 1. Stabilization window.
	if iteration <= cfg.StabilizationWindow {
		return fmt.Errorf("stabilization window active (iteration %d ≤ %d)", iteration, cfg.StabilizationWindow)
	}

	// 2. Valid category (case-insensitive — LLM may output any case).
	validCategory := false
	for _, c := range cfg.ValidCategories {
		if strings.EqualFold(cmd.Category, c) {
			validCategory = true
			break
		}
	}
	if !validCategory {
		return fmt.Errorf("invalid category %q (valid: %s)", cmd.Category, strings.Join(cfg.ValidCategories, ", "))
	}

	// 3. Category cooldown.
	if last, ok := cooldowns.gapByCategory[cmd.Category]; ok {
		remaining := cfg.GapCooldown - (iteration - last)
		if remaining > 0 {
			return fmt.Errorf("gap cooldown active for category %q (%d iterations remaining)", cmd.Category, remaining)
		}
	}

	// 4. Dedup: same missing_role already emitted this session.
	if cooldowns.emittedGaps[cmd.MissingRole] {
		return fmt.Errorf("gap for missing_role %q already emitted this session", cmd.MissingRole)
	}

	return nil
}

// validateDirectiveCommand checks all safety constraints before emitting a directive event.
// Returns nil if valid, descriptive error if rejected.
func validateDirectiveCommand(cmd *DirectiveCommand, iteration int, cooldowns *CTOCooldowns, cfg CTOConfig) error {
	// 1. Stabilization window.
	if iteration <= cfg.StabilizationWindow {
		return fmt.Errorf("stabilization window active (iteration %d ≤ %d)", iteration, cfg.StabilizationWindow)
	}

	// 2. Target cooldown.
	if last, ok := cooldowns.directiveByTarget[cmd.Target]; ok {
		remaining := cfg.DirectiveCooldown - (iteration - last)
		if remaining > 0 {
			return fmt.Errorf("directive cooldown active for target %q (%d iterations remaining)", cmd.Target, remaining)
		}
	}

	return nil
}

// emitGap constructs a GapDetectedContent and calls agent.EmitGapDetected.
// Normalizes case from LLM output (e.g., "quality" → "Quality") and recovers
// from constructor panics so bad LLM output never crashes the hive.
func (l *Loop) emitGap(cmd *GapCommand) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("emit hive.gap.detected: %v", r)
		}
	}()
	category := event.GapCategory(titleCase(cmd.Category))
	severity := event.SeverityLevel(titleCase(cmd.Severity))
	content := event.NewGapDetectedContent(category, cmd.MissingRole, cmd.Evidence, severity)
	if err := l.agent.EmitGapDetected(content); err != nil {
		return fmt.Errorf("emit hive.gap.detected: %w", err)
	}
	fmt.Printf("[%s] emitted hive.gap.detected (category=%s missing_role=%s severity=%s)\n",
		l.agent.Name(), category, cmd.MissingRole, severity)
	return nil
}

// emitDirective constructs a DirectiveIssuedContent and calls agent.EmitDirective.
// Normalizes case from LLM output and recovers from constructor panics.
func (l *Loop) emitDirective(cmd *DirectiveCommand) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("emit hive.directive.issued: %v", r)
		}
	}()
	priority := event.DirectivePriority(titleCase(cmd.Priority))
	content := event.NewDirectiveIssuedContent(cmd.Target, cmd.Action, cmd.Reason, priority)
	if err := l.agent.EmitDirective(content); err != nil {
		return fmt.Errorf("emit hive.directive.issued: %w", err)
	}
	fmt.Printf("[%s] emitted hive.directive.issued (target=%s priority=%s)\n",
		l.agent.Name(), cmd.Target, priority)
	return nil
}

// titleCase normalizes a string to title case (e.g., "high" → "High").
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// validateAndEmitGap validates and, if valid, emits a gap event and records the cooldown.
func (l *Loop) validateAndEmitGap(cmd *GapCommand, iteration int) error {
	if err := validateGapCommand(cmd, iteration, l.ctoCooldowns, l.ctoConfig); err != nil {
		return err
	}
	if err := l.emitGap(cmd); err != nil {
		return err
	}
	l.ctoCooldowns.gapByCategory[cmd.Category] = iteration
	l.ctoCooldowns.emittedGaps[cmd.MissingRole] = true
	return nil
}

// validateAndEmitDirective validates and, if valid, emits a directive event and records the cooldown.
func (l *Loop) validateAndEmitDirective(cmd *DirectiveCommand, iteration int) error {
	if err := validateDirectiveCommand(cmd, iteration, l.ctoCooldowns, l.ctoConfig); err != nil {
		return err
	}
	if err := l.emitDirective(cmd); err != nil {
		return err
	}
	l.ctoCooldowns.directiveByTarget[cmd.Target] = iteration
	return nil
}

// enrichCTOObservation appends a pre-computed leadership briefing to the
// observation string for the CTO. Only activates for the "cto" role.
//
// Assembles task flow, health summary, budget summary, and previous gap/directive
// counts from pending events and the shared BudgetRegistry.
func (l *Loop) enrichCTOObservation(obs string) string {
	if string(l.agent.Role()) != "cto" {
		return obs
	}

	// Snapshot pending events under lock for task/health/gap/directive counting.
	l.mu.Lock()
	pending := make([]event.Event, len(l.pendingEvents))
	copy(pending, l.pendingEvents)
	l.mu.Unlock()

	// a. Task flow: count pending events by work.task.* subtype.
	taskCounts := map[string]int{
		"created":   0,
		"assigned":  0,
		"completed": 0,
		"blocked":   0,
	}
	for _, ev := range pending {
		t := ev.Type().Value()
		if !strings.HasPrefix(t, "work.task.") {
			continue
		}
		suffix := strings.TrimPrefix(t, "work.task.")
		// Normalise compound subtypes (e.g. "dependency.added") to root.
		root := strings.SplitN(suffix, ".", 2)[0]
		if _, known := taskCounts[root]; known {
			taskCounts[root]++
		}
	}

	// b. Health: find most recent health.report in pending events.
	var healthSeverity string
	var healthAgents int
	var healthRate float64
	var healthChainOK bool
	healthFound := false
	for i := len(pending) - 1; i >= 0; i-- {
		if pending[i].Type() == types.MustEventType("health.report") {
			healthFound = true
			if hrc, ok := pending[i].Content().(event.HealthReportContent); ok {
				v := hrc.Overall.Value()
				switch {
				case v >= 0.9:
					healthSeverity = "ok"
				case v >= 0.4:
					healthSeverity = "warning"
				default:
					healthSeverity = "critical"
				}
				healthChainOK = hrc.ChainIntegrity
				healthAgents = hrc.ActiveActors
				healthRate = hrc.EventRate
			}
			break
		}
	}

	// c. Budget: BudgetRegistry snapshot.
	reg := l.config.BudgetRegistry

	// d. Previous gaps and directives in pending events.
	gapCount := 0
	directiveCount := 0
	for _, ev := range pending {
		switch ev.Type() {
		case types.MustEventType("hive.gap.detected"):
			gapCount++
		case types.MustEventType("hive.directive.issued"):
			directiveCount++
		}
	}

	// Format the leadership briefing.
	var sb strings.Builder
	sb.WriteString("\n=== LEADERSHIP BRIEFING ===\n")

	sb.WriteString("TASK FLOW:\n")
	sb.WriteString(fmt.Sprintf("  created=%d assigned=%d completed=%d blocked=%d\n",
		taskCounts["created"], taskCounts["assigned"], taskCounts["completed"], taskCounts["blocked"]))

	sb.WriteString("\nHEALTH (from SysMon):\n")
	if healthFound {
		chainStr := "ok"
		if !healthChainOK {
			chainStr = "fail"
		}
		sb.WriteString(fmt.Sprintf("  severity=%s chain=%s agents=%d event_rate=%.1f/min\n",
			healthSeverity, chainStr, healthAgents, healthRate))
	} else {
		sb.WriteString("  [no health.report received yet]\n")
	}

	sb.WriteString("\nBUDGET (from Allocator):\n")
	if reg != nil {
		totalPool := reg.TotalPool()
		totalUsed := reg.TotalUsed()
		usedPct := float64(0)
		if totalPool > 0 {
			usedPct = float64(totalUsed) * 100.0 / float64(totalPool)
		}
		sb.WriteString(fmt.Sprintf("  pool=%d/%d(%.1f%%)\n", totalUsed, totalPool, usedPct))
		sb.WriteString("  AGENTS:\n")
		for _, e := range reg.Snapshot() {
			snap := e.Budget.Snapshot()
			pct := float64(0)
			if e.MaxIterations > 0 {
				pct = float64(snap.Iterations) * 100.0 / float64(e.MaxIterations)
			}
			sb.WriteString(fmt.Sprintf("    %-14s max=%-4d used=%-4d(%.1f%%)  state=%s\n",
				e.Name+":", e.MaxIterations, snap.Iterations, pct, e.AgentState))
		}
	} else {
		sb.WriteString("  [no budget registry available]\n")
	}

	sb.WriteString("\nGAPS (previously detected): ")
	if gapCount == 0 {
		sb.WriteString("[none yet]\n")
	} else {
		sb.WriteString(fmt.Sprintf("%d\n", gapCount))
	}

	sb.WriteString("\nDIRECTIVES (active): ")
	if directiveCount == 0 {
		sb.WriteString("[none yet]\n")
	} else {
		sb.WriteString(fmt.Sprintf("%d\n", directiveCount))
	}

	sb.WriteString("===\n")
	return obs + sb.String()
}
