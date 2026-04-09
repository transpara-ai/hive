package loop

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/checkpoint"
)

// ────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────

// SpawnCommand represents the parsed /spawn command from Spawner LLM output.
// The Spawner outputs this when it has designed a new role to propose.
type SpawnCommand struct {
	Name          string   `json:"name"`
	Model         string   `json:"model"`         // "haiku", "sonnet", or "opus"
	WatchPatterns []string `json:"watch_patterns"`
	CanOperate    bool     `json:"can_operate"`
	MaxIterations int      `json:"max_iterations"`
	Prompt        string   `json:"prompt"`
	Reason        string   `json:"reason"`
}

// spawnerState tracks cross-iteration state for the Spawner agent.
// Added to Loop struct; initialized in New() when role == "spawner".
// Only accessed from the Run() goroutine — no mutex needed.
//
// Architecture note: l.pendingEvents is flushed each iteration, so
// cross-iteration state (pending proposals, rejection history) cannot
// rely on scanning pendingEvents. This struct persists across iterations
// analogous to ctoCooldowns for the CTO.
type spawnerState struct {
	pendingProposal  string         // name of role currently proposed (empty = none)
	recentRejections map[string]int // role name → iteration when rejected
	processedGaps    map[string]bool
	iteration        int // current iteration counter
}

// newSpawnerState initialises a zeroed spawnerState.
func newSpawnerState() *spawnerState {
	return &spawnerState{
		recentRejections: make(map[string]int),
		processedGaps:    make(map[string]bool),
	}
}

// InitSpawnerFromRecovery seeds spawner state from chain replay.
// iteration is the recovered loop iteration count — needed to correctly
// evaluate stabilization windows and rejection cooldowns.
func (s *spawnerState) InitSpawnerFromRecovery(state *checkpoint.SpawnerRecoveredState, iteration int) {
	if state == nil {
		return
	}
	s.iteration = iteration
	for k := range state.RecentRejections {
		// Set rejection iteration to a value that preserves the cooldown window.
		// We don't know the exact iteration of rejection, so use iteration - 1
		// to allow re-proposal after one more cooldown window (50 iterations).
		s.recentRejections[k] = iteration - 1
	}
	for k, v := range state.ProcessedGaps {
		s.processedGaps[k] = v
	}
	s.pendingProposal = state.PendingProposal
}

// update processes the current batch of pending events and increments the
// iteration counter. Called once per loop iteration with l.pendingEvents.
//
//   - hive.role.proposed  → records pendingProposal (so the Spawner knows one is in-flight)
//   - hive.role.approved  → clears pendingProposal
//   - hive.role.rejected  → clears pendingProposal, records rejection for cooldown
//   - hive.gap.detected   → marks gap event ID as processed
func (s *spawnerState) update(events []event.Event) {
	s.iteration++
	for _, ev := range events {
		switch ev.Type() {
		case types.MustEventType("hive.role.proposed"):
			if c, ok := ev.Content().(event.RoleProposedContent); ok {
				s.pendingProposal = c.Name
			}
		case types.MustEventType("hive.role.approved"):
			s.pendingProposal = ""
		case types.MustEventType("hive.role.rejected"):
			s.pendingProposal = ""
			if c, ok := ev.Content().(event.RoleRejectedContent); ok {
				s.recentRejections[c.Name] = s.iteration
			}
		case types.MustEventType("hive.gap.detected"):
			s.processedGaps[ev.ID().Value()] = true
		}
	}
}

// SpawnContext is the snapshot passed to validateSpawnCommand.
// Built from spawnerState plus BudgetRegistry data once per iteration.
type SpawnContext struct {
	Iteration          int
	HasPendingProposal bool
	AgentRoster        []string       // agent names from BudgetRegistry.Snapshot()
	RecentRejections   map[string]int // role name → iteration when rejected
}

// RosterContains returns true if name is already in the agent roster.
func (ctx *SpawnContext) RosterContains(name string) bool {
	for _, n := range ctx.AgentRoster {
		if n == name {
			return true
		}
	}
	return false
}

// RecentlyRejected returns true if name was rejected within the given window
// of iterations (i.e., ctx.Iteration - rejectedAt < window).
func (ctx *SpawnContext) RecentlyRejected(name string, window int) bool {
	rejectedAt, ok := ctx.RecentRejections[name]
	if !ok {
		return false
	}
	return ctx.Iteration-rejectedAt < window
}

// ────────────────────────────────────────────────────────────────────
// Model constants (mirror pkg/hive/agentdef.go — no import to avoid cycle)
// ────────────────────────────────────────────────────────────────────

const (
	spawnModelHaiku  = "claude-haiku-4-5-20251001"
	spawnModelSonnet = "claude-sonnet-4-6"
	spawnModelOpus   = "claude-opus-4-6"
)

// resolveModel maps the human-readable tier name ("haiku", "sonnet", "opus")
// to the actual model identifier string used in AgentDef.Model.
func resolveModel(name string) string {
	switch name {
	case "haiku":
		return spawnModelHaiku
	case "sonnet":
		return spawnModelSonnet
	case "opus":
		return spawnModelOpus
	}
	return ""
}

// ────────────────────────────────────────────────────────────────────
// Reserved names — bootstrap agents that cannot be overwritten
// ────────────────────────────────────────────────────────────────────

var reservedRoleNames = map[string]bool{
	"guardian":    true,
	"sysmon":      true,
	"allocator":   true,
	"cto":         true,
	"spawner":     true,
	"strategist":  true,
	"planner":     true,
	"implementer": true,
}

// kebabCaseRE matches valid kebab-case role names:
// lowercase letters/digits, separated by single hyphens, 2–50 chars.
var kebabCaseRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// isValidRoleName returns true if name is a valid role name:
//   - 2–50 characters
//   - kebab-case only (lowercase letters, digits, single hyphens between words)
//   - cannot start or end with a hyphen
//   - cannot contain consecutive hyphens (--) — enforced by the regex above
//   - cannot be a reserved bootstrap agent name
func isValidRoleName(name string) bool {
	if len(name) < 2 || len(name) > 50 {
		return false
	}
	if !kebabCaseRE.MatchString(name) {
		return false
	}
	return !reservedRoleNames[name]
}

// ────────────────────────────────────────────────────────────────────
// Parsing
// ────────────────────────────────────────────────────────────────────

// parseSpawnCommand extracts the /spawn JSON payload from LLM output.
// Returns nil if no /spawn command is found or the JSON is malformed.
// Follows the same line-scanning pattern as parseGapCommand.
func parseSpawnCommand(response string) *SpawnCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/spawn ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/spawn ")
		var cmd SpawnCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────
// Validation
// ────────────────────────────────────────────────────────────────────

// validateSpawnCommand checks all safety constraints before emitting a
// hive.role.proposed event. Returns a descriptive error if any constraint
// is violated, nil if the proposal is safe to emit.
//
// Rules enforced (from design spec section 7):
//  1. Stabilization window: first 20 iterations are observe-only
//  2. Pending proposal: only one proposal in-flight at a time
//  3. Name: valid kebab-case, no collision with existing roster
//  4. Model: must be "haiku", "sonnet", or "opus"
//  5. MaxIterations: 10–200
//  6. Prompt: >= 100 characters
//  7. WatchPatterns: non-empty, no bare wildcard ("*")
//  8. Rejection cooldown: 50-iteration wait after rejection of same name
//  9. CanOperate: must be false (trust must be earned first)
func validateSpawnCommand(cmd *SpawnCommand, ctx *SpawnContext) error {
	// 1. Stabilization window.
	if ctx.Iteration < 20 {
		return fmt.Errorf("stabilization window active (iteration %d < 20): observe first", ctx.Iteration)
	}

	// 2. Pending proposal.
	if ctx.HasPendingProposal {
		return fmt.Errorf("proposal already pending: wait for approval or rejection before proposing another")
	}

	// 3. Name validation.
	if !isValidRoleName(cmd.Name) {
		return fmt.Errorf("invalid role name %q: must be kebab-case, 2-50 chars, not reserved", cmd.Name)
	}
	if ctx.RosterContains(cmd.Name) {
		return fmt.Errorf("role %q already exists in agent roster", cmd.Name)
	}

	// 4. Model validation.
	if resolveModel(cmd.Model) == "" {
		return fmt.Errorf("invalid model %q: must be haiku, sonnet, or opus", cmd.Model)
	}

	// 5. MaxIterations bounds.
	if cmd.MaxIterations < 10 || cmd.MaxIterations > 200 {
		return fmt.Errorf("invalid max_iterations %d: must be 10-200", cmd.MaxIterations)
	}

	// 6. Prompt length.
	if len(cmd.Prompt) < 100 {
		return fmt.Errorf("prompt too short (%d chars): must be >= 100", len(cmd.Prompt))
	}

	// 7. WatchPatterns.
	if len(cmd.WatchPatterns) == 0 {
		return fmt.Errorf("watch_patterns must be non-empty: specify which events this role handles")
	}
	for _, p := range cmd.WatchPatterns {
		if p == "*" {
			return fmt.Errorf("watch_patterns cannot contain bare wildcard %q: only Guardian watches everything", p)
		}
	}

	// 8. Rejection cooldown (50 iterations).
	if ctx.RecentlyRejected(cmd.Name, 50) {
		return fmt.Errorf("role %q was recently rejected: wait 50 iterations before reproposing", cmd.Name)
	}

	// 9. CanOperate blocked for all spawned roles.
	if cmd.CanOperate {
		return fmt.Errorf("can_operate must be false: new roles must earn trust before operating on files")
	}

	return nil
}

// ────────────────────────────────────────────────────────────────────
// Emission
// ────────────────────────────────────────────────────────────────────

// emitRoleProposed constructs a RoleProposedContent from the validated
// SpawnCommand and records it on the event chain via agent.EmitRoleProposed.
// The model tier name ("haiku", "sonnet", "opus") is resolved to the actual
// model identifier string before emission.
func (l *Loop) emitRoleProposed(cmd *SpawnCommand) error {
	content := event.RoleProposedContent{
		Name:          cmd.Name,
		Model:         resolveModel(cmd.Model),
		WatchPatterns: cmd.WatchPatterns,
		CanOperate:    cmd.CanOperate,
		MaxIterations: cmd.MaxIterations,
		Prompt:        cmd.Prompt,
		Reason:        cmd.Reason,
		ProposedBy:    "spawner",
	}
	if err := l.agent.EmitRoleProposed(content); err != nil {
		return fmt.Errorf("emit hive.role.proposed: %w", err)
	}
	fmt.Printf("[%s] emitted hive.role.proposed (name=%s model=%s)\n",
		l.agent.Name(), cmd.Name, cmd.Model)
	return nil
}

// ────────────────────────────────────────────────────────────────────
// Observation Enrichment
// ────────────────────────────────────────────────────────────────────

// enrichSpawnObservation appends the pre-computed SPAWN CONTEXT block to the
// observation string for the Spawner. Only activates when l.spawnerState != nil
// (i.e., when role == "spawner").
//
// Reads cross-iteration state from l.spawnerState (already updated from the
// current iteration's pending events before this is called) and live data from
// the BudgetRegistry.
func (l *Loop) enrichSpawnObservation(obs string) string {
	if l.spawnerState == nil {
		return obs
	}

	reg := l.config.BudgetRegistry

	var sb strings.Builder
	sb.WriteString("\n=== SPAWN CONTEXT ===\n")

	// Roster from BudgetRegistry.
	sb.WriteString("ROSTER:\n")
	if reg != nil {
		entries := reg.Snapshot()
		// Sort by name for deterministic output.
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})
		for _, e := range entries {
			snap := e.Budget.Snapshot()
			marker := ""
			if e.Name == l.agent.Name() {
				marker = "  (you)"
			}
			sb.WriteString(fmt.Sprintf("  %-14s %-8s iter=%d/%d%s\n",
				e.Name+":", e.AgentState, snap.Iterations, e.MaxIterations, marker))
		}
	} else {
		sb.WriteString("  [no budget registry available]\n")
	}

	// Pending proposals from spawnerState.
	sb.WriteString("\nPENDING PROPOSALS: ")
	if l.spawnerState.pendingProposal == "" {
		sb.WriteString("none\n")
	} else {
		sb.WriteString(l.spawnerState.pendingProposal + "\n")
	}

	// Recent gaps from spawnerState.processedGaps.
	sb.WriteString("\nRECENT GAPS (last 50 iterations):\n")
	if len(l.spawnerState.processedGaps) == 0 {
		sb.WriteString("  [none yet]\n")
	} else {
		sb.WriteString(fmt.Sprintf("  %d gap(s) detected this session\n", len(l.spawnerState.processedGaps)))
	}

	// Recent outcomes from spawnerState.recentRejections.
	sb.WriteString("\nRECENT OUTCOMES:\n")
	if len(l.spawnerState.recentRejections) == 0 {
		sb.WriteString("  (none yet)\n")
	} else {
		// Sort for deterministic output.
		names := make([]string, 0, len(l.spawnerState.recentRejections))
		for name := range l.spawnerState.recentRejections {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			iter := l.spawnerState.recentRejections[name]
			sb.WriteString(fmt.Sprintf("  [iter %d] %s REJECTED\n", iter, name))
		}
	}

	// Budget pool from BudgetRegistry.
	sb.WriteString("\nBUDGET POOL:\n")
	if reg != nil {
		totalPool := reg.TotalPool()
		totalUsed := reg.TotalUsed()
		available := totalPool - totalUsed
		sb.WriteString(fmt.Sprintf("  total=%d used=%d available=%d\n", totalPool, totalUsed, available))
	} else {
		sb.WriteString("  [no budget registry available]\n")
	}

	sb.WriteString("===\n")
	return obs + sb.String()
}

// buildSpawnContext constructs a SpawnContext from spawnerState and BudgetRegistry.
// Called once per iteration when a /spawn command is found in the response.
func (l *Loop) buildSpawnContext() *SpawnContext {
	ctx := &SpawnContext{
		Iteration:          l.spawnerState.iteration,
		HasPendingProposal: l.spawnerState.pendingProposal != "",
		RecentRejections:   l.spawnerState.recentRejections,
	}

	if reg := l.config.BudgetRegistry; reg != nil {
		for _, e := range reg.Snapshot() {
			ctx.AgentRoster = append(ctx.AgentRoster, e.Name)
		}
	}

	return ctx
}
