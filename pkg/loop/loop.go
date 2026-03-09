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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
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
	// Agent is the agent to run. Required.
	Agent *roles.Agent

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
}

// Loop runs an agent's observe-reason-act-reflect cycle.
type Loop struct {
	agent   *roles.Agent
	humanID types.ActorID
	budget  *resources.Budget
	config  Config

	// mu protects pendingEvents.
	mu            sync.Mutex
	pendingEvents []event.Event
	wake          chan struct{} // signaled when new events arrive via bus
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

	return &Loop{
		agent:   cfg.Agent,
		humanID: cfg.HumanID,
		budget:  resources.NewBudget(cfg.Budget),
		config:  cfg,
		wake:    make(chan struct{}, 1),
	}, nil
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

		// 1. OBSERVE — gather context from the graph.
		observation, err := l.observe(ctx)
		if err != nil {
			return l.result(StopError, iteration, fmt.Sprintf("observe: %v", err))
		}

		// 2. REASON + ACT — kick the agent with context and task.
		prompt := l.buildPrompt(observation, iteration)
		response, tokens, err := l.reason(ctx, prompt)
		if err != nil {
			return l.result(StopError, iteration, fmt.Sprintf("reason: %v", err))
		}

		// Record resource consumption.
		l.budget.Record(tokens, 0) // cost tracked separately if needed

		if l.config.OnIteration != nil {
			l.config.OnIteration(iteration, response)
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
	rt := l.agent.Runtime

	// Record observation.
	_, err := rt.Observe(ctx, l.config.ObservationWindow)
	if err != nil {
		return "", err
	}

	// Get recent events for context.
	events, err := rt.Memory(l.config.ObservationWindow)
	if err != nil {
		return "", err
	}

	// Also include any pending bus events.
	l.mu.Lock()
	pending := l.pendingEvents
	l.pendingEvents = nil
	l.mu.Unlock()

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

	return sb.String(), nil
}

// buildPrompt constructs the reasoning prompt for this iteration.
func (l *Loop) buildPrompt(observation string, iteration int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are %s (%s), iteration %d of your agentic loop.\n\n",
		l.agent.Name, l.agent.Role, iteration))

	if l.config.Task != "" && iteration == 1 {
		sb.WriteString(fmt.Sprintf("## Your Task\n%s\n\n", l.config.Task))
	}

	sb.WriteString(observation)

	sb.WriteString(`

## Instructions
Based on the events above, decide what to do next:
- If work needs doing: describe what you'll do and emit the appropriate events
- If you need human approval: say ESCALATE and explain why
- If your work is complete: say TASK_DONE
- If nothing needs doing: say IDLE

Respond concisely. Focus on actions, not explanations.
`)

	return sb.String()
}

// reason calls the agent's LLM and returns the response text and tokens used.
func (l *Loop) reason(ctx context.Context, prompt string) (string, int, error) {
	rt := l.agent.Runtime
	memory, _ := rt.Memory(10)
	resp, err := rt.Provider().Reason(ctx, prompt, memory)
	if err != nil {
		return "", 0, err
	}
	return resp.Content(), resp.TokensUsed(), nil
}

// checkResponse examines the LLM response for stopping signals.
func (l *Loop) checkResponse(ctx context.Context, response string, iteration int) *Result {
	upper := strings.ToUpper(response)

	if strings.Contains(upper, "ESCALATE") {
		// Record escalation event.
		_, _ = l.agent.Runtime.Escalate(ctx, l.humanID,
			fmt.Sprintf("loop iteration %d: %s", iteration, response))
		r := l.result(StopEscalation, iteration, response)
		return &r
	}

	if strings.Contains(upper, "HALT") {
		r := l.result(StopHalt, iteration, response)
		return &r
	}

	if strings.Contains(upper, "TASK_DONE") {
		// Record completion.
		_, _ = l.agent.Runtime.Learn(ctx,
			"task completed after loop iteration "+fmt.Sprint(iteration), "loop")
		r := l.result(StopTaskDone, iteration, response)
		return &r
	}

	return nil
}

// isQuiescent returns true if the response indicates the agent has nothing to do.
func (l *Loop) isQuiescent(response string) bool {
	upper := strings.ToUpper(strings.TrimSpace(response))
	return upper == "IDLE" || strings.HasPrefix(upper, "IDLE")
}

// onEvent is called by the bus when a new event arrives.
func (l *Loop) onEvent(ev event.Event) {
	// Skip our own events to avoid infinite loops.
	if ev.Source() == l.agent.Runtime.ID() {
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
func (l *Loop) waitForEvents(ctx context.Context) bool {
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

// AgentResult pairs a loop result with the agent's role and name,
// avoiding silent data loss when multiple agents share a role.
type AgentResult struct {
	Role   roles.Role
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
					Role:   c.Agent.Role,
					Name:   c.Agent.Name,
					Result: Result{Reason: StopError, Detail: err.Error()},
				}
				return
			}

			results[idx] = AgentResult{
				Role:   c.Agent.Role,
				Name:   c.Agent.Name,
				Result: l.Run(ctx),
			}
		}(i, cfg)
	}

	wg.Wait()
	return results
}

