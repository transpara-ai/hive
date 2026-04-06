// Package loop implements the agentic loop — sustained autonomy for agents.
//
// An agent doesn't just respond to a prompt — it observes the world,
// decides what to do, acts, and observes again. The loop runs until:
//   - Quiescence — no new events, nothing to do
//   - Escalation — the agent needs human approval
//   - HALT — the Guardian stopped the agent
//   - Budget — token/cost/iteration/time limit reached
//
// The loop transforms the hive from a pipeline into a society.
package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/lovyou-ai/agent"
	"github.com/lovyou-ai/hive/pkg/budget"
	"github.com/lovyou-ai/hive/pkg/knowledge"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/work"
)

// StopReason describes why a loop stopped.
type StopReason string

const (
	StopQuiescence  StopReason = "quiescence"
	StopEscalation  StopReason = "escalation"
	StopHalt        StopReason = "halt"
	StopBudget      StopReason = "budget"
	StopError       StopReason = "error"
	StopCancelled   StopReason = "cancelled"
	StopTaskDone    StopReason = "task_done"
)

// Result is the outcome of a loop run.
type Result struct {
	Reason     StopReason
	Iterations int
	Budget     resources.BudgetSnapshot
	Detail     string // human-readable explanation
}

// Config configures an agentic loop.
type Config struct {
	// Agent is the unified hive agent to run. Required.
	// Provides state machine, causality tracking, and trust hooks.
	Agent *hiveagent.Agent

	// HumanID is the human operator's ID (for escalation attribution).
	HumanID types.ActorID

	// Budget limits for this loop run. Required.
	Budget resources.BudgetConfig

	// ObservationWindow is how many recent events to include in context.
	// Defaults to 20.
	ObservationWindow int

	// Task is the initial task description that seeds the loop.
	// If empty, the agent observes the graph and self-directs.
	Task string

	// Bus is an optional event bus for real-time notifications.
	// When set, the loop subscribes to relevant events and wakes
	// the agent when new work arrives.
	Bus bus.IBus

	// QuiescenceDelay is how long to wait for new events before
	// declaring quiescence. Defaults to 5 seconds.
	QuiescenceDelay time.Duration

	// OnIteration is called after each loop iteration (for monitoring).
	// Optional.
	OnIteration func(iteration int, response string)

	// TaskStore enables /task command processing. When set, agents can
	// create, assign, complete, and comment on tasks through the work graph.
	TaskStore *work.TaskStore

	// ConvID is the conversation ID for task operations.
	ConvID types.ConversationID

	// CanOperate indicates this agent has filesystem access.
	// When true and the agent has assigned tasks, the loop calls
	// Operate() instead of Reason() for implementation work.
	CanOperate bool

	// RepoPath is the working directory for Operate() calls.
	// Required when CanOperate is true.
	RepoPath string

	// Keepalive prevents agents from exiting on quiescence. When true,
	// waitForEvents blocks indefinitely on the bus wake channel instead
	// of timing out. Agents consume zero CPU/LLM while waiting. They
	// resume when a new event arrives on the bus.
	Keepalive bool

	// BudgetInstance is an externally-created Budget tracker. When set,
	// the Loop uses this instead of creating a new one from Budget config.
	// This allows the Runtime to register the Budget in a shared registry
	// before the Loop starts.
	BudgetInstance *resources.Budget

	// BudgetRegistry provides cross-agent budget visibility. Optional.
	// When set, observation enrichment can query all agents' budget states.
	BudgetRegistry *resources.BudgetRegistry

	// ActorResolver maps actor IDs to display names for task context.
	// Optional. When nil, task context omits creator information.
	ActorResolver func(types.ActorID) string

	// KnowledgeStore provides access to distilled insights for context
	// enrichment. Optional. When nil, agents run without knowledge injection.
	KnowledgeStore knowledge.KnowledgeStore
}

// Loop runs an agent's observe-reason-act-reflect cycle.
type Loop struct {
	agent   *hiveagent.Agent
	humanID types.ActorID
	budget  *resources.Budget
	config  Config

	// mu protects pendingEvents.
	mu            sync.Mutex
	pendingEvents []event.Event
	wake          chan struct{} // signaled when new events arrive via bus

	// iteration tracks the current loop iteration for budget stabilization
	// and cooldown checks. Only accessed from the Run() goroutine.
	iteration int

	// adjustmentHistory tracks budget adjustments for cooldown enforcement.
	// Only accessed from the Run() goroutine.
	adjustmentHistory []budget.AdjustmentRecord

	// ctoCooldowns and ctoConfig are populated in New() when role == "cto".
	// Only accessed from the Run() goroutine.
	ctoCooldowns *CTOCooldowns
	ctoConfig    CTOConfig

	// spawnerState is populated in New() when role == "spawner".
	// Only accessed from the Run() goroutine.
	spawnerState *spawnerState

	// reviewerState is populated in New() when role == "reviewer".
	// Only accessed from the Run() goroutine.
	reviewerState *reviewerState
}

// New creates a new agentic loop.
func New(cfg Config) (*Loop, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
	if cfg.ObservationWindow <= 0 {
		cfg.ObservationWindow = 20
	}
	if cfg.QuiescenceDelay <= 0 {
		cfg.QuiescenceDelay = 5 * time.Second
	}

	budget := cfg.BudgetInstance
	if budget == nil {
		budget = resources.NewBudget(cfg.Budget)
	}

	l := &Loop{
		agent:   cfg.Agent,
		humanID: cfg.HumanID,
		budget:  budget,
		config:  cfg,
		wake:    make(chan struct{}, 1),
	}

	if string(cfg.Agent.Role()) == "cto" {
		l.ctoCooldowns = NewCTOCooldowns()
		l.ctoConfig = LoadCTOConfig()
	}

	if string(cfg.Agent.Role()) == "spawner" {
		l.spawnerState = newSpawnerState()
	}

	if string(cfg.Agent.Role()) == "reviewer" {
		l.reviewerState = newReviewerState()
	}

	return l, nil
}

// Run executes the agentic loop until a stopping condition is met.
func (l *Loop) Run(ctx context.Context) Result {
	// Subscribe to bus events if available.
	var subID bus.SubscriptionID
	if l.config.Bus != nil {
		// Subscribe to all events — the agent sees everything on the graph.
		pattern := types.MustSubscriptionPattern("*")
		subID = l.config.Bus.Subscribe(pattern, l.onEvent)
		defer l.config.Bus.Unsubscribe(subID)
	}

	iteration := 0
	consecutiveEmpty := 0

	for {
		// Check context cancellation.
		if ctx.Err() != nil {
			return l.result(StopCancelled, iteration, "context cancelled")
		}

		// Check budget before each iteration.
		if err := l.budget.Check(); err != nil {
			return l.result(StopBudget, iteration, err.Error())
		}

		iteration++
		l.iteration = iteration

		// 1. OBSERVE — gather context from the graph.
		observation, err := l.observe(ctx)
		if err != nil {
			return l.result(StopError, iteration, fmt.Sprintf("observe: %v", err))
		}

		// 2. REASON or OPERATE — choose based on agent capabilities.
		var response string
		var usage decision.TokenUsage

		// Auto-assign: if this agent can operate and there are open unassigned
		// tasks, grab the first one so the Operate path activates immediately
		// instead of requiring a Reason round-trip to emit /task assign.
		if l.config.CanOperate && l.config.RepoPath != "" && !l.hasAssignedTask() {
			l.autoAssignOpenTask()
		}

		if l.config.CanOperate && l.config.RepoPath != "" && l.hasAssignedTask() {
			// Operate path: agent has filesystem access and assigned work.
			task := l.nextAssignedTask()
			instruction := fmt.Sprintf("Task: %s\n\n%s", task.Title, task.Description)
			result, opErr := l.agent.Operate(ctx, l.config.RepoPath, instruction)
			if opErr != nil {
				return l.result(StopError, iteration, fmt.Sprintf("operate: %v", opErr))
			}
			response = result.Summary
			usage = result.Usage

			// Auto-complete the task after successful Operate.
			l.completeTask(task, result.Summary)
		} else {
			// Reason path: standard observe-reason loop.
			prompt := l.buildPrompt(observation, iteration)
			var reasonErr error
			response, usage, reasonErr = l.reason(ctx, prompt)
			if reasonErr != nil {
				return l.result(StopError, iteration, fmt.Sprintf("reason: %v", reasonErr))
			}
		}

		// Record resource consumption.
		l.budget.RecordUsage(usage)

		if l.config.OnIteration != nil {
			l.config.OnIteration(iteration, response)
		}

		// 2.5. PROCESS task commands from the response.
		l.processTaskCommands(response)

		// 2.6. PROCESS /health command from the response.
		if cmd := parseHealthCommand(response); cmd != nil {
			if err := l.emitHealthReport(cmd); err != nil {
				fmt.Printf("warning: /health command failed: %v\n", err)
			}
		}

		// 2.7. PROCESS /budget command from the response.
		if cmd := parseBudgetCommand(response); cmd != nil {
			if err := l.validateBudgetCommand(cmd, iteration); err != nil {
				fmt.Printf("[%s] /budget rejected: %v\n", l.agent.Name(), err)
			} else if err := l.applyBudgetAdjustment(cmd, iteration); err != nil {
				fmt.Printf("[%s] /budget failed: %v\n", l.agent.Name(), err)
			}
		}

		// 2.8. PROCESS /gap and /directive commands from the response (CTO only).
		if l.ctoCooldowns != nil {
			if cmd := parseGapCommand(response); cmd != nil {
				if err := l.validateAndEmitGap(cmd, iteration); err != nil {
					fmt.Printf("warning: /gap rejected: %v\n", err)
				}
			}
			if cmd := parseDirectiveCommand(response); cmd != nil {
				if err := l.validateAndEmitDirective(cmd, iteration); err != nil {
					fmt.Printf("warning: /directive rejected: %v\n", err)
				}
			}
		}

		// 2.9. PROCESS /spawn command from the response (Spawner only).
		if l.spawnerState != nil {
			if cmd := parseSpawnCommand(response); cmd != nil {
				spawnCtx := l.buildSpawnContext()
				if err := validateSpawnCommand(cmd, spawnCtx); err != nil {
					fmt.Printf("[%s] /spawn rejected: %v\n", l.agent.Name(), err)
				} else if err := l.emitRoleProposed(cmd); err != nil {
					fmt.Printf("[%s] /spawn emit failed: %v\n", l.agent.Name(), err)
				}
			}
		}

		// 2.10. PROCESS /approve and /reject commands from the response (Guardian only).
		if string(l.agent.Role()) == "guardian" {
			if cmd := parseApproveCommand(response); cmd != nil {
				if err := l.emitRoleApproved(cmd); err != nil {
					fmt.Printf("[%s] /approve emit failed: %v\n", l.agent.Name(), err)
				}
			}
			if cmd := parseRejectCommand(response); cmd != nil {
				if err := l.emitRoleRejected(cmd); err != nil {
					fmt.Printf("[%s] /reject emit failed: %v\n", l.agent.Name(), err)
				}
			}
		}

		// 3. CHECK stopping conditions in the response.
		if stop := l.checkResponse(ctx, response, iteration); stop != nil {
			return *stop
		}

		// 4. Check for quiescence — agent said nothing useful, no new events.
		if l.isQuiescent(response) {
			consecutiveEmpty++
			if consecutiveEmpty >= 2 {
				// Wait for new events from bus or timeout.
				if l.config.Bus != nil {
					if l.waitForEvents(ctx) {
						consecutiveEmpty = 0
						continue
					}
				}
				return l.result(StopQuiescence, iteration, "no new work after multiple iterations")
			}
		} else {
			consecutiveEmpty = 0
		}
	}
}

// observe gathers recent events from the graph as context.
func (l *Loop) observe(ctx context.Context) (string, error) {
	// Record observation via hiveagent (drives state machine + causality).
	_, err := l.agent.Observe(ctx, l.config.ObservationWindow)
	if err != nil {
		return "", err
	}

	// Get recent events for context.
	events, err := l.agent.Memory(l.config.ObservationWindow)
	if err != nil {
		return "", err
	}

	// Also include any pending bus events.
	l.mu.Lock()
	pending := l.pendingEvents
	l.pendingEvents = nil
	l.mu.Unlock()

	// Update spawner cross-iteration state from this iteration's events.
	if l.spawnerState != nil {
		l.spawnerState.update(pending)
	}
	// Update reviewer cross-iteration state from this iteration's events.
	if l.reviewerState != nil {
		l.reviewerState.update(pending)
	}

	var sb strings.Builder
	sb.WriteString("## Recent Events\n")
	for _, ev := range events {
		sb.WriteString(fmt.Sprintf("- [%s] %s by %s\n",
			ev.Type().Value(), ev.ID().Value(), ev.Source().Value()))
	}

	if len(pending) > 0 {
		sb.WriteString("\n## New Events (since last check)\n")
		for _, ev := range pending {
			sb.WriteString(fmt.Sprintf("- [%s] %s by %s\n",
				ev.Type().Value(), ev.ID().Value(), ev.Source().Value()))
		}
	}

	// Enrich observation with pre-computed health metrics for SysMon.
	enriched := l.enrichHealthObservation(sb.String())
	// Enrich observation with pre-computed budget metrics for Allocator.
	enriched = l.enrichBudgetObservation(enriched, l.iteration)
	// Enrich observation with leadership briefing for CTO.
	enriched = l.enrichCTOObservation(enriched)
	// Enrich observation with spawn context for Spawner.
	enriched = l.enrichSpawnObservation(enriched)
	// Enrich observation with code review context for Reviewer.
	enriched = l.enrichReviewObservation(enriched)
	// Enrich observation with institutional knowledge for ALL agents.
	enriched = l.enrichKnowledgeObservation(enriched)
	return enriched, nil
}

// buildPrompt constructs the reasoning prompt for this iteration.
func (l *Loop) buildPrompt(observation string, iteration int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are %s (%s), iteration %d of your agentic loop.\n\n",
		l.agent.Name(), l.agent.Role(), iteration))

	if l.config.Task != "" && iteration == 1 {
		sb.WriteString(fmt.Sprintf("## Your Task\n%s\n\n", l.config.Task))
	}

	// Include task context if TaskStore is available.
	if l.config.TaskStore != nil {
		if taskCtx := l.buildTaskContext(); taskCtx != "" {
			sb.WriteString(taskCtx)
			sb.WriteString("\n")
		}
	}

	sb.WriteString(observation)

	sb.WriteString(`

## Instructions
Based on the events and tasks above, decide what to do next.

End your response with exactly one signal on its own line, in this exact JSON format:
/signal {"signal": "IDLE"}

Valid signals:
- IDLE       — nothing to do right now, waiting for events
- TASK_DONE  — all work is complete
- ESCALATE   — need human approval or decision (include "reason")
- HALT       — policy violation or integrity issue (include "reason")

Examples:
  I reviewed the latest build events and found no issues.
  /signal {"signal": "IDLE"}

  The code violates security policy.
  /signal {"signal": "HALT", "reason": "SQL injection in user input handler"}

Respond concisely. Focus on actions, not explanations.
Every response MUST end with exactly one /signal line.
`)

	return sb.String()
}

// reason calls the agent's LLM and returns the response text and token usage.
// Uses the unified Agent.Reason() which drives state machine transitions.
//
// Retries up to 3 times on chain integrity violations. These occur when a
// concurrent goroutine (e.g., autoAssignOpenTask via TaskStore.Assign) appends
// directly to the event store between the graph.Record mutex's head-read and
// store-append, causing the spawner's prevHash to stale. On retry, graph.Record
// re-reads the fresh head and the append succeeds.
//
// State transition errors (e.g., Processing → Processing) are NOT retryable —
// they indicate the agent is stuck in a state that retrying won't fix.
func (l *Loop) reason(ctx context.Context, prompt string) (string, decision.TokenUsage, error) {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		content, err := l.agent.Reason(ctx, prompt)
		if err == nil {
			return content, decision.TokenUsage{}, nil
		}
		// State transition errors are not retryable.
		if strings.Contains(err.Error(), "invalid transition") {
			return "", decision.TokenUsage{}, err
		}
		if strings.Contains(err.Error(), "chain integrity violation") && attempt < maxRetries-1 {
			// Reset agent state before retry — the failed iteration may have
			// left the agent in Processing, which would cause the next
			// Reason() call to fail with Processing → Processing.
			l.agent.ResetToIdle()
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
			continue
		}
		return "", decision.TokenUsage{}, err
	}
	// Unreachable, but satisfies the compiler.
	return "", decision.TokenUsage{}, fmt.Errorf("reason: exhausted retries")
}

// Signal is the structured JSON signal emitted by agents at the end of each response.
type Signal struct {
	Signal string `json:"signal"` // IDLE, TASK_DONE, ESCALATE, HALT
	Reason string `json:"reason,omitempty"`
}

// SignalIDLE is the signal value for "nothing to do".
const SignalIDLE = "IDLE"

// SignalTaskDone is the signal value for "work complete".
const SignalTaskDone = "TASK_DONE"

// SignalEscalate is the signal value for "needs human".
const SignalEscalate = "ESCALATE"

// SignalHalt is the signal value for "policy violation".
const SignalHalt = "HALT"

// parseSignal extracts the /signal JSON line from a response.
// Returns nil if no valid signal is found.
func parseSignal(response string) *Signal {
	lines := strings.Split(response, "\n")
	// Search from the bottom — signal should be the last line.
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "/signal ") {
			jsonStr := strings.TrimPrefix(trimmed, "/signal ")
			var sig Signal
			if err := json.Unmarshal([]byte(jsonStr), &sig); err == nil && sig.Signal != "" {
				sig.Signal = strings.ToUpper(sig.Signal)
				return &sig
			}
		}
	}
	return nil
}

// ContainsSignal checks whether a signal keyword appears as a directive in the
// response. A directive must appear at the start of a line (possibly after
// whitespace). This is the text-based fallback when the LLM doesn't emit
// a /signal JSON line.
//
// Valid examples: "HALT: violation", "  HALT", "\nHALT\n", "TASK_DONE"
// Invalid: "No HALT required", "we should not ESCALATE"
func ContainsSignal(response, signal string) bool {
	signal = strings.ToUpper(signal)
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.ToUpper(strings.TrimSpace(line))
		if trimmed == signal || strings.HasPrefix(trimmed, signal+":") || strings.HasPrefix(trimmed, signal+" ") {
			return true
		}
	}
	return false
}

// checkResponse examines the LLM response for stopping signals.
// Priority: HALT > ESCALATE > TASK_DONE (HALT is constitutional, must never be masked).
//
// Prefers structured /signal JSON parsing. Falls back to line-start text matching
// if the LLM doesn't emit the JSON signal line.
func (l *Loop) checkResponse(ctx context.Context, response string, iteration int) *Result {
	sig := parseSignal(response)
	if sig == nil {
		// Fallback: scan for text-based signals.
		return l.checkResponseText(ctx, response, iteration)
	}

	switch sig.Signal {
	case SignalHalt:
		r := l.result(StopHalt, iteration, sig.Reason)
		return &r
	case SignalEscalate:
		if err := l.agent.Escalate(ctx, l.humanID,
			fmt.Sprintf("loop iteration %d: %s", iteration, sig.Reason)); err != nil {
			fmt.Printf("warning: escalation event failed: %v\n", err)
		}
		r := l.result(StopEscalation, iteration, sig.Reason)
		return &r
	case SignalTaskDone:
		if err := l.agent.Learn(ctx,
			"task completed after loop iteration "+fmt.Sprint(iteration), "loop"); err != nil {
			fmt.Printf("warning: completion event failed: %v\n", err)
		}
		r := l.result(StopTaskDone, iteration, sig.Reason)
		return &r
	case SignalIDLE:
		// Not a stop — handled by isQuiescent.
		return nil
	default:
		fmt.Printf("warning: unknown signal %q from %s\n", sig.Signal, l.agent.Name())
		return nil
	}
}

// checkResponseText is the text-based fallback for signal detection.
func (l *Loop) checkResponseText(ctx context.Context, response string, iteration int) *Result {
	if ContainsSignal(response, "HALT") {
		r := l.result(StopHalt, iteration, response)
		return &r
	}
	if ContainsSignal(response, "ESCALATE") {
		if err := l.agent.Escalate(ctx, l.humanID,
			fmt.Sprintf("loop iteration %d: %s", iteration, response)); err != nil {
			fmt.Printf("warning: escalation event failed: %v\n", err)
		}
		r := l.result(StopEscalation, iteration, response)
		return &r
	}
	if ContainsSignal(response, "TASK_DONE") {
		if err := l.agent.Learn(ctx,
			"task completed after loop iteration "+fmt.Sprint(iteration), "loop"); err != nil {
			fmt.Printf("warning: completion event failed: %v\n", err)
		}
		r := l.result(StopTaskDone, iteration, response)
		return &r
	}
	return nil
}

// isQuiescent returns true if the response indicates the agent has nothing to do.
// Checks for /signal JSON first, then falls back to text matching.
func (l *Loop) isQuiescent(response string) bool {
	if sig := parseSignal(response); sig != nil {
		return sig.Signal == SignalIDLE
	}
	return ContainsSignal(response, "IDLE")
}

// onEvent is called by the bus when a new event arrives.
func (l *Loop) onEvent(ev event.Event) {
	// Skip our own events to avoid infinite loops.
	if ev.Source() == l.agent.ID() {
		return
	}

	l.mu.Lock()
	l.pendingEvents = append(l.pendingEvents, ev)
	l.mu.Unlock()

	// Signal the wake channel (non-blocking).
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// waitForEvents blocks until new events arrive or quiescence timeout.
// Returns true if events arrived, false if timed out.
// In keepalive mode, there is no timeout — the agent blocks on the wake
// channel indefinitely, consuming zero CPU until a bus event arrives.
func (l *Loop) waitForEvents(ctx context.Context) bool {
	if l.config.Keepalive {
		select {
		case <-l.wake:
			return true
		case <-ctx.Done():
			return false
		}
	}

	timer := time.NewTimer(l.config.QuiescenceDelay)
	defer timer.Stop()

	select {
	case <-l.wake:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// result creates a Result with budget snapshot.
func (l *Loop) result(reason StopReason, iterations int, detail string) Result {
	return Result{
		Reason:     reason,
		Iterations: iterations,
		Budget:     l.budget.Snapshot(),
		Detail:     detail,
	}
}

// ────────────────────────────────────────────────────────────────────
// Task integration helpers
// ────────────────────────────────────────────────────────────────────

// buildTaskContext builds a task summary section for the prompt.
func (l *Loop) buildTaskContext() string {
	summaries, err := l.config.TaskStore.ListSummaries(50)
	if err != nil || len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Work Tasks\n")

	for _, t := range summaries {
		status := string(t.Status)
		if t.Blocked {
			status = "blocked"
		}
		assignee := ""
		if t.Assignee != (types.ActorID{}) {
			if t.Assignee == l.agent.ID() {
				assignee = " [assigned to you]"
			} else {
				assignee = fmt.Sprintf(" [assigned to %s]", t.Assignee.Value())
			}
		}
		createdBy := ""
		if l.config.ActorResolver != nil && t.CreatedBy != (types.ActorID{}) {
			if name := l.config.ActorResolver(t.CreatedBy); name != "" {
				if t.CreatedBy == l.agent.ID() {
					createdBy = " (created by you)"
				} else {
					createdBy = fmt.Sprintf(" (created by %s)", name)
				}
			}
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s%s%s\n",
			status, t.ID.Value(), t.Title, assignee, createdBy))
	}

	return sb.String()
}

// processTaskCommands extracts and executes /task commands from the response.
func (l *Loop) processTaskCommands(response string) {
	if l.config.TaskStore == nil {
		return
	}

	commands := parseTaskCommands(response)
	if len(commands) == 0 {
		return
	}

	// Use agent's last event as cause for task operations.
	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}

	executed := executeTaskCommands(commands, l.config.TaskStore, l.agent.ID(), causes, l.config.ConvID)
	if executed > 0 {
		fmt.Printf("[%s] executed %d/%d task commands\n", l.agent.Name(), executed, len(commands))
	}
}

// autoAssignOpenTask finds the first open, unassigned task and assigns it to
// this agent. This lets the Operate path activate without waiting for the LLM
// to emit a /task assign command via Reason.
func (l *Loop) autoAssignOpenTask() {
	if l.config.TaskStore == nil {
		return
	}
	open, err := l.config.TaskStore.ListOpen()
	if err != nil || len(open) == 0 {
		return
	}
	// Find first task not assigned to anyone.
	for _, t := range open {
		summary, sErr := l.config.TaskStore.ListSummaries(100)
		if sErr != nil {
			continue
		}
		for _, s := range summary {
			if s.ID == t.ID && s.Assignee == (types.ActorID{}) {
				var causes []types.EventID
				if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
					causes = []types.EventID{lastEv}
				}
				if err := l.config.TaskStore.Assign(l.agent.ID(), t.ID, l.agent.ID(), causes, l.config.ConvID); err == nil {
					fmt.Printf("  → auto-assigned: %s — %s\n", t.ID.Value(), t.Title)
				}
				return
			}
		}
	}
}

// hasAssignedTask returns true if the agent has any assigned (non-completed) tasks.
func (l *Loop) hasAssignedTask() bool {
	if l.config.TaskStore == nil {
		return false
	}
	tasks, err := l.config.TaskStore.GetByAssignee(l.agent.ID())
	if err != nil || len(tasks) == 0 {
		return false
	}
	// Check if any assigned task is not yet completed.
	for _, t := range tasks {
		status, _ := l.config.TaskStore.GetStatus(t.ID)
		if status != work.StatusCompleted {
			return true
		}
	}
	return false
}

// nextAssignedTask returns the next non-completed task assigned to this agent.
func (l *Loop) nextAssignedTask() work.Task {
	tasks, _ := l.config.TaskStore.GetByAssignee(l.agent.ID())
	for _, t := range tasks {
		status, _ := l.config.TaskStore.GetStatus(t.ID)
		if status != work.StatusCompleted {
			return t
		}
	}
	return work.Task{}
}

// completeTask marks a task as completed in the task store. Best-effort.
func (l *Loop) completeTask(task work.Task, summary string) {
	if l.config.TaskStore == nil {
		return
	}
	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}
	if err := l.config.TaskStore.Complete(l.agent.ID(), task.ID, summary, causes, l.config.ConvID); err != nil {
		fmt.Printf("warning: task complete failed: %v\n", err)
	} else {
		fmt.Printf("  → task completed: %s — %s\n", task.ID.Value(), task.Title)
	}
}

// AgentResult pairs a loop result with the agent's role and name,
// avoiding silent data loss when multiple agents share a role.
type AgentResult struct {
	Role   string
	Name   string
	Result Result
}

// RunConcurrent runs multiple agent loops concurrently and returns when all stop.
// Each loop runs in its own goroutine. Returns one result per agent.
func RunConcurrent(ctx context.Context, configs []Config) []AgentResult {
	results := make([]AgentResult, len(configs))
	var wg sync.WaitGroup

	for i, cfg := range configs {
		wg.Add(1)
		go func(idx int, c Config) {
			defer wg.Done()

			l, err := New(c)
			if err != nil {
				results[idx] = AgentResult{
					Role:   string(c.Agent.Role()),
					Name:   c.Agent.Name(),
					Result: Result{Reason: StopError, Detail: err.Error()},
				}
				return
			}

			results[idx] = AgentResult{
				Role:   string(c.Agent.Role()),
				Name:   c.Agent.Name(),
				Result: l.Run(ctx),
			}
		}(i, cfg)
	}

	wg.Wait()
	return results
}

