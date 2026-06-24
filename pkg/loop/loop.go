// Package loop implements the agentic loop — sustained autonomy for agents.
//
// An agent doesn't just respond to a prompt — it observes the world,
// decides what to do, acts, and observes again. The loop runs until:
//   - Quiescence — no new events, nothing to do
//   - Escalation — the agent needs human approval. Terminal for one-shot
//     loops; a KEEPALIVE loop raises the escalation and PARKS instead
//     (waitForEvents), so an always-on daemon never loses a civic agent
//     to a transient condition (v8-F2).
//   - HALT — the Guardian stopped the agent (constitutional; terminal
//     everywhere, never masked)
//   - Budget — token/cost/iteration/time limit reached
//
// The loop transforms the hive from a pipeline into a society.
package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/bus"
	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/budget"
	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/hive/pkg/knowledge"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/work"
)

// StopReason describes why a loop stopped.
type StopReason string

const (
	StopQuiescence StopReason = "quiescence"
	StopEscalation StopReason = "escalation"
	StopHalt       StopReason = "halt"
	StopBudget     StopReason = "budget"
	StopError      StopReason = "error"
	StopCancelled  StopReason = "cancelled"
	StopTaskDone   StopReason = "task_done"
)

// Result is the outcome of a loop run.
type Result struct {
	Reason     StopReason
	Iterations int
	Budget     resources.BudgetSnapshot
	Detail     string // human-readable explanation
}

// TaskOperateProviderResult is the optional per-task provider selected for one
// Operate call. Applied=false means the loop should use the agent's default
// provider.
type TaskOperateProviderResult struct {
	Applied  bool
	Provider intelligence.Provider
}

// TaskOperateProviderFunc resolves a task-scoped provider for one Operate call.
// The callback must fail closed: returning an error stops the operation instead
// of falling back to the agent's default provider.
type TaskOperateProviderFunc func(ctx context.Context, task work.Task, role string) (TaskOperateProviderResult, error)

// TaskWorkspaceProviderResult is the optional per-task workspace selected for
// one Operate call. Applied=false means the loop should use Config.RepoPath.
type TaskWorkspaceProviderResult struct {
	Applied               bool
	RepoPath              string
	ContainmentWatchRoots []string
}

// TaskWorkspaceProviderFunc resolves a task-scoped checkout for one Operate
// call. Errors fail closed before any filesystem-capable subprocess launches.
type TaskWorkspaceProviderFunc func(ctx context.Context, task work.Task, role string) (TaskWorkspaceProviderResult, error)

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

	// PhaseGateStore enables /phase gate, approve, and reject commands through
	// the work graph.
	PhaseGateStore *work.PhaseGateStore

	// ConvID is the conversation ID for task operations.
	ConvID types.ConversationID

	// OnTaskCompleted is called after TaskStore.Complete succeeds. Optional.
	OnTaskCompleted func(ctx context.Context, task work.Task, summary string)

	// OnTaskCommandsExecuted is called after an agent successfully executes one
	// or more /task commands. It is a post-commit hook: the durable Work events
	// already exist when this runs, so runtime coordinators can derive follow-up
	// state from the store without trusting the agent response text.
	OnTaskCommandsExecuted func(ctx context.Context, executed, total int)

	// OnReviewCompleted is called after a /review command is durably emitted and
	// any request_changes return edge has been routed. Optional.
	OnReviewCompleted func(ctx context.Context, taskID, verdict string)

	// CanOperate indicates this agent has filesystem access.
	// When true and the agent has assigned tasks, the loop calls
	// Operate() instead of Reason() for implementation work.
	CanOperate bool

	// RepoPath is the working directory for Operate() calls.
	// Required when CanOperate is true.
	RepoPath string

	// ContainmentWatchRoots are directories whose immediate child git
	// checkouts the workspace-containment tripwire (v10-F2 / Finding 18)
	// watches around every Operate. Empty = watch the parent directory of
	// RepoPath (the sibling worktrees a run can walk to). There is no
	// disable switch: a CanOperate loop is always watched.
	ContainmentWatchRoots []string

	// TaskOperateProvider optionally selects a provider for this exact Work task.
	// Used for structured FactoryOrder model overrides. Errors fail closed.
	TaskOperateProvider TaskOperateProviderFunc

	// TaskWorkspaceProvider optionally selects a checkout for this exact Work
	// task. Operate, containment, commit verification, and Operate artifacts all
	// use the selected checkout together. Errors fail closed.
	TaskWorkspaceProvider TaskWorkspaceProviderFunc

	// Keepalive prevents agents from exiting on quiescence. When true,
	// waitForEvents blocks indefinitely on the bus wake channel instead
	// of timing out. Agents consume zero CPU/LLM while waiting. They
	// resume when a new event arrives on the bus.
	Keepalive bool

	// RecheckInterval is the slow periodic re-check for keepalive agents whose
	// duty can be recovered from durable state. The bus wake is an edge signal
	// (onEvent's non-blocking send): a wake that fires while the agent is busy
	// rather than parked is lost, and an event persisted by a PRIOR daemon
	// instance never fires at all — so an idle agent can sleep forever even
	// though its work exists (the race/stranding a daemon restart otherwise had
	// to clear). On this interval waitForEvents re-checks durable state and
	// returns if actionable work exists, converting lost edges into a
	// recoverable level check. Exactly two duties carry the re-check, each with
	// its own gate so an idle agent stays parked:
	//   - a CanOperate keepalive agent (the implementer), gated to
	//     hasAssignableWork (the hive#135 wakeup race);
	//   - a keepalive agent with a review duty (the reviewer), gated to
	//     hasReviewableWork, so a completion that became historical across a
	//     restart still engages the review→fix loop (run findings F8).
	// 0 takes the New() default for those two duties; <0 disables it; it has
	// no effect on keepalive agents with neither duty.
	RecheckInterval time.Duration

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

	// CostSummaryFunc returns a cost table for the allocator's observation.
	// Called each iteration for the allocator role. Returns empty string to skip.
	CostSummaryFunc func() string

	// Catalog is the model catalog for spawn validation. Optional.
	// When nil, falls back to modelconfig.DefaultCatalog().
	Catalog *modelconfig.ModelCatalog

	// RecoveryState holds recovered state from a prior run. When set and Mode
	// is ModeWarm, the loop seeds iteration counter, skips stabilization, and
	// injects intent into the first iteration. Nil means first boot.
	RecoveryState *checkpoint.RecoveryState

	// Sink receives boundary and heartbeat signals for checkpointing.
	// Nil means no checkpointing.
	Sink checkpoint.CheckpointSink

	// HeartbeatInterval is iterations between heartbeat emissions when no
	// boundary trigger has fired. Default 10.
	HeartbeatInterval int
}

// maxPendingEvents bounds the per-agent observation buffer. A keepalive agent
// that sleeps through a churn storm must not accumulate an unbounded backlog and
// dump it into the next observation context (invariant BOUNDED). When the cap is
// exceeded the oldest events are dropped — the newest context wins.
const maxPendingEvents = 256

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
	// Initialized in New() (including recovery seeding), then exclusively
	// accessed from the Run() goroutine — no mutex needed.
	ctoCooldowns *CTOCooldowns
	ctoConfig    CTOConfig

	// spawnerState is populated in New() when role == "spawner".
	// Initialized in New() (including recovery seeding), then exclusively
	// accessed from the Run() goroutine — no mutex needed.
	spawnerState *spawnerState

	// reviewerState is populated in New() when role == "reviewer".
	// Initialized in New() (including recovery seeding), then exclusively
	// accessed from the Run() goroutine — no mutex needed.
	reviewerState *reviewerState

	// parkAfterEscalation arms the keepalive raise-and-park path (slice-1 v8
	// run, finding v8-F2): a keepalive agent's ESCALATE raises agent.escalated
	// for the human and PARKS the loop instead of returning StopEscalation —
	// terminal escalation permanently removed civic agents from an always-on
	// daemon over transient conditions, and spawned replacements cannot
	// operate. Set by checkResponse/checkResponseText, consumed by Run().
	// Only accessed from the Run() goroutine.
	parkAfterEscalation    bool
	parkedEscalationReason string

	// reasonRetryBackoff is the wait schedule between Reason attempts inside
	// one iteration (slice-1 v13 run, finding v13-F1): a transient provider
	// error — API 529 during the 2026-06-11 opus incident — killed the loop
	// on its first failure, silently. Attempts = len()+1; waits are
	// context-aware and jittered. Only accessed from the Run() goroutine;
	// tests shrink the schedule.
	reasonRetryBackoff []time.Duration

	// reasonFailureEscalated dedupes the on-chain raise to ONE per
	// consecutive-failure episode (a parked loop that wakes, retries, and
	// fails again must not re-page the human); any successful Reason resets
	// it. Only accessed from the Run() goroutine.
	reasonFailureEscalated bool

	// budgetParkLogged dedupes the duration-park notice (v14-F3b): a parked
	// loop re-checks its budget on every wake/re-check cycle and must not
	// re-print per cycle. Cleared when a budget check passes again (resume).
	budgetParkLogged bool

	// budgetParkEmitted dedupes the on-chain agent.budget.exhausted raise
	// (v15-F1a): one event per park episode, no matter how many spurious
	// wakes re-enter the parked branch. Set ONLY on a successful chain
	// write (the v13-F1 dedup lesson: a failed raise must stay retryable,
	// never silently marked done). Cleared when a budget check passes
	// again (resume), so the next park episode raises again — and reset
	// when the duration limit CHANGES without unparking (codex r2 #3: an
	// insufficient renewal must produce a fresh raise, or the renewer acts
	// once on a stale picture and the deadlock resurrects).
	budgetParkEmitted bool

	// parkAckedLimit is the duration limit the current park episode has
	// ACKNOWLEDGED: set at episode entry and again on each successful
	// raise. A live limit that differs is an unacknowledged renewal — the
	// park's recheck tick re-enters the branch (codex r3: a renewal whose
	// budget.adjusted emit failed delivers NO wake, so the tick is the only
	// way the re-raise belt is reachable) and the belt raises afresh.
	// Run() goroutine only.
	parkAckedLimit time.Duration

	// allocParkSig is the park-set signature at the allocator recheck's
	// last fire (v15-F1b): the sorted, joined names of duration-parked
	// agents. The recheck fires only when a NON-EMPTY set differs from
	// this signature, and resets when the set empties — so an allocator
	// that declined to renew is not re-prompted every tick (a 50ms-30s
	// storm would burn its terminal iteration budget and kill the
	// renewer), while a new park or a renew-then-repark always fires.
	// Only accessed from the Run() goroutine (waitForEvents).
	allocParkSig string

	// sink receives checkpoint signals. Nil-safe — callers check before use.
	sink checkpoint.CheckpointSink

	// lastCheckpointIter tracks when last boundary or heartbeat fired.
	lastCheckpointIter int

	// heartbeatInterval is iterations between heartbeats. 0 means disabled.
	heartbeatInterval int
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
	// Default the keepalive re-check for the three re-check duties so all are
	// covered without every call site setting it: a CanOperate keepalive agent
	// (the implementer wakeup race, hive#135), a keepalive reviewer (the
	// historical-completion stranding, run findings F8), and a keepalive
	// allocator (the renewal deadlock, v15-F1b: the renewer must notice
	// duration-parked renewables even when the park's wake edge was lost)
	// with an unset interval get a slow safety-net re-check. <0 disables it.
	// Keepalive agents with none of these duties get no default — the
	// re-check stays disabled, so no idle ticker is added anywhere else.
	if cfg.Keepalive && cfg.RecheckInterval == 0 &&
		(cfg.CanOperate || string(cfg.Agent.Role()) == "reviewer" ||
			string(cfg.Agent.Role()) == "allocator") {
		cfg.RecheckInterval = 30 * time.Second
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
		// Two retries absorb provider blips; outages longer than ~80s land on
		// the raise-and-park path, where wake/re-check carries the long haul.
		reasonRetryBackoff: []time.Duration{20 * time.Second, 60 * time.Second},
	}

	if cfg.Sink != nil {
		l.sink = cfg.Sink
		// Wire the heartbeat emitter now that we have the agent.
		// The sink was constructed in runtime.go without the agent —
		// this closes the gap so OnHeartbeat actually writes to the chain.
		if ds, ok := l.sink.(*checkpoint.DefaultSink); ok {
			ds.SetHeartbeatEmitter(func(snap checkpoint.LoopSnapshot) error {
				hb := checkpoint.HeartbeatFromSnapshot(snap)
				return l.agent.EmitHeartbeat(hb)
			})
		}
	}
	l.heartbeatInterval = cfg.HeartbeatInterval
	if l.heartbeatInterval <= 0 {
		l.heartbeatInterval = 10
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

	// Seed recovered state into role-specific structs.
	if cfg.RecoveryState != nil {
		if l.ctoCooldowns != nil && cfg.RecoveryState.CTOState != nil {
			l.ctoCooldowns.InitCTOFromRecovery(cfg.RecoveryState.CTOState)
		}
		if l.spawnerState != nil && cfg.RecoveryState.SpawnerState != nil {
			l.spawnerState.InitSpawnerFromRecovery(cfg.RecoveryState.SpawnerState, cfg.RecoveryState.Iteration)
		}
		if l.reviewerState != nil && cfg.RecoveryState.ReviewerState != nil {
			l.reviewerState.InitReviewerFromRecovery(cfg.RecoveryState.ReviewerState)
		}
		// Seed consumed budget so a restarted agent honours the BUDGET
		// invariant instead of getting a full fresh allowance. Only runs when
		// the prior run actually recorded consumption — zero values on a cold
		// start are a no-op overwrite on the already-zero budget counters.
		if l.budget != nil && (cfg.RecoveryState.ConsumedTokens > 0 || cfg.RecoveryState.ConsumedCostUSD > 0) {
			l.budget.SeedConsumed(
				cfg.RecoveryState.Iteration,
				cfg.RecoveryState.ConsumedTokens,
				cfg.RecoveryState.ConsumedCostUSD,
			)
		}
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
	if l.config.RecoveryState != nil && l.config.RecoveryState.Mode == checkpoint.ModeWarm {
		iteration = l.config.RecoveryState.Iteration
		fmt.Fprintf(os.Stderr, "[%s] warm-started at iteration %d\n", l.agent.Name(), iteration)
	} else if l.config.RecoveryState != nil && l.config.RecoveryState.Iteration > 0 {
		// Cold-start with replayed iteration — seed the counter to prevent
		// stabilization window re-triggering and cooldown map incoherence.
		iteration = l.config.RecoveryState.Iteration
		fmt.Fprintf(os.Stderr, "[%s] cold-started at iteration %d (from chain replay)\n", l.agent.Name(), iteration)
	}
	consecutiveEmpty := 0

	for {
		// Check context cancellation.
		if ctx.Err() != nil {
			return l.result(StopCancelled, iteration, "context cancelled")
		}

		// Check budget before each iteration. DURATION exhaustion on a
		// keepalive loop with a bus PARKS instead of exiting (v14-F3b): the
		// 30m MaxDuration default ended every society epoch at 30 minutes —
		// eight simultaneous budget obituaries on the first epoch that lived
		// that long. The park joins the raise-and-park family (v8-F2
		// escalations, v13-F1 reason failures): the allocator renews the
		// limit on-chain (v14-F3c) and the next wake or gated re-check
		// passes this same check and resumes. The park is allowlisted to
		// exactly that proven shape — every other resource (iterations,
		// tokens, cost, anything future) and every non-keepalive or bus-less
		// loop keeps the terminal stop: exit is the default, parking is the
		// explicitly-proven branch (fail closed).
		if err := l.budget.Check(); err != nil {
			var exceeded *resources.BudgetExceededError
			if errors.As(err, &exceeded) &&
				exceeded.Resource == resources.ResourceDuration &&
				l.config.Keepalive && l.config.Bus != nil {
				if !l.budgetParkLogged {
					fmt.Printf("%s\n", formatBudgetPark(l.agent.Name(), err))
					l.budgetParkLogged = true
					// Fresh episode: acknowledge the limit we parked at, so
					// the renewal-change detector (tick side and belt below)
					// has a baseline even if the raise itself fails.
					l.parkAckedLimit = l.budget.MaxDuration()
				}
				// v15-F1(b): register the park BEFORE the raise (codex r1 #2)
				// so any observer the event wakes — however the bus schedules
				// delivery — reads PARKED(duration) for this agent. The clear
				// happens ONLY at proven resume (the budget check passes
				// below) or at shutdown — never on the wake edge (codex r1
				// #1: a spurious wake re-enters this branch still exhausted;
				// a marker flicker would let the allocator's empty-set reset
				// read the SAME park as a new episode).
				if reg := l.config.BudgetRegistry; reg != nil {
					reg.SetDurationParked(l.agent.Name(), true)
				}
				// v15-F1(a): the raise half. Round 5's park was stdout-only —
				// no chain event, so a quiescent society gave the allocator
				// no wake and the renewer slept while the renewables waited.
				// Emit agent.budget.exhausted once per park episode. An emit
				// failure never blocks the park (parking is the safe state;
				// the allocator's gated re-check is the fail-safe wake) and
				// leaves the flag unset so the next pass retries the raise.
				// A limit that CHANGED without unparking is an insufficient
				// renewal (codex r2 #3) — re-raise so the renewer gets a
				// fresh wake; bounded by renewal acts, not by wakes.
				if l.budgetParkEmitted && l.budget.MaxDuration() != l.parkAckedLimit {
					l.budgetParkEmitted = false
				}
				if !l.budgetParkEmitted {
					// Capture the limit BEFORE publishing (codex r4): the
					// acknowledged value must be the limit this raise was
					// issued against. Reading it after the publish lets a
					// fast renewal land inside the window and be swallowed
					// as already-acknowledged — if that renewal was
					// insufficient with a failed budget.adjusted emit, the
					// wake-free detector would go quiet (the r3 deadlock,
					// one window narrower). With capture-before-emit, any
					// change after the read is unacknowledged by
					// construction and fires the detector.
					limitAtRaise := l.budget.MaxDuration()
					if emitErr := l.agent.EmitBudgetExhausted(string(resources.ResourceDuration)); emitErr != nil {
						fmt.Printf("[%s] budget.exhausted raise failed (park proceeds; recheck pulse remains): %v\n", l.agent.Name(), emitErr)
					} else {
						l.budgetParkEmitted = true
						l.parkAckedLimit = limitAtRaise
					}
				}
				// waitForBudgetRenewal, not waitForEvents: the renewal event
				// may not match this agent's subscriptions, and a fully
				// parked society generates no other wakes — the park polls
				// the in-memory budget so a renewed agent always resumes.
				if l.waitForBudgetRenewal(ctx) {
					consecutiveEmpty = 0
					continue
				}
				// Shutdown while parked: result() clears the marker — every
				// loop death does (codex r2 #1/#2 made it the chokepoint).
				return l.result(StopBudget, iteration, "shutdown while parked on budget exhaustion: "+err.Error())
			}
			return l.result(StopBudget, iteration, err.Error())
		}
		// Proven resume: the budget check passed. Clear the park episode
		// state — the registry marker (set only while a live loop is parked)
		// and both per-episode dedup flags.
		if l.budgetParkLogged {
			if reg := l.config.BudgetRegistry; reg != nil {
				reg.SetDurationParked(l.agent.Name(), false)
			}
		}
		l.budgetParkLogged = false
		l.budgetParkEmitted = false

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
			// Operate sees ONLY this instruction (headless subprocess). Fold in the
			// task's readiness contract (DoD/acceptance_criteria/test_plan) so the
			// implementer builds to the criteria the Planner attached — title+desc
			// alone left round 1 blind to them and it over-enumerated the catalog.
			instruction, instrErr := l.operateInstruction(task)
			if instrErr != nil {
				// Fail closed: refuse to Operate blind to the readiness contract.
				// Escalate rather than degrade to title+description when a configured
				// store cannot load the task's gates.
				return l.result(StopEscalation, iteration, fmt.Sprintf("operate: %v", instrErr))
			}

			workspace, workspaceErr := l.taskWorkspace(ctx, task)
			if workspaceErr != nil {
				return l.result(StopEscalation, iteration, fmt.Sprintf("operate: %v", workspaceErr))
			}
			if strings.TrimSpace(workspace.RepoPath) == "" {
				return l.result(StopEscalation, iteration, "operate: no repo path configured for filesystem-capable task")
			}
			containmentRoots := l.containmentRootsFor(workspace.RepoPath, workspace.ContainmentWatchRoots)

			// Record HEAD before Operate so we can detect new commits. gitTry
			// reports whether the read succeeded: an unreadable pre-Operate HEAD
			// makes the post-Operate result unverifiable (it must not be mistaken
			// for a clean "first commit").
			preOperateHead, preHeadReadable := gitTry(workspace.RepoPath, "rev-parse", "HEAD")

			// Workspace-containment baseline (v10-F2 / Finding 18): snapshot the
			// watched sibling checkouts BEFORE the subprocess exists. Post-hoc
			// detection cannot undo sibling damage, so an unreadable baseline
			// refuses the launch outright (fail closed) instead of running
			// unwatched and apologizing later.
			preContainment, preContainOK := snapshotContainment(containmentRoots, workspace.RepoPath)
			if !preContainOK {
				return l.result(StopEscalation, iteration,
					"operate: containment baseline unreadable — refusing to launch a filesystem-capable Operate without a verifiable sibling-checkout watch")
			}

			result, opErr := l.operateTask(ctx, task, instruction, workspace.RepoPath)
			if opErr != nil {
				if errors.Is(opErr, errTaskOperateProvider) {
					return l.result(StopEscalation, iteration, fmt.Sprintf("operate: %v", opErr))
				}
				return l.result(StopError, iteration, fmt.Sprintf("operate: %v", opErr))
			}
			response = result.Summary
			usage = result.Usage
			// Record resource consumption immediately: a successful model call
			// consumed tokens whether or not the commit-verification gate below
			// passes. The early StopEscalation return on failure must NOT skip
			// budget accounting (BUDGET invariant) — a failed Operate still costs.
			l.budget.RecordUsage(usage)

			// Containment tripwire veto (v10-F2) — runs BEFORE commit
			// verification: a run that mutated a sibling checkout must never
			// auto-complete, however clean its workspace commit looks (the v10
			// round-3 escape shape was exactly that). failOperateTask already
			// escalated; the summary is untrusted and drives nothing further.
			if !l.verifyOperateContainmentInWorkspace(ctx, task, preContainment, workspace.RepoPath, containmentRoots) {
				return l.result(StopEscalation, iteration,
					"workspace containment violated; halting implementer for human review")
			}

			// Commit-verification gate: never trust the agent's self-report.
			// handleOperateResult compares HEAD before/after Operate and
			// cross-checks the summary — a confabulated (or wrong-repo) commit
			// fails the task instead of silently completing it.
			if l.handleOperateResultInWorkspace(ctx, task, workspace.RepoPath, preOperateHead, preHeadReadable, result.Summary) {
				if l.sink != nil {
					l.captureBoundary(checkpoint.TaskCompleted, response)
					l.lastCheckpointIter = l.iteration
				}
			} else {
				// Commit verification failed (confabulated / wrong-repo /
				// uncommitted work). The Operate summary is UNTRUSTED — halt and
				// escalate instead of letting it drive /task, /phase, /signal,
				// etc. failOperateTask already escalated to the human.
				return l.result(StopEscalation, iteration,
					"commit verification failed; halting implementer for human review")
			}
		} else {
			// Reason path: standard observe-reason loop.
			prompt := l.buildPrompt(observation, iteration)
			var reasonErr error
			response, usage, reasonErr = l.reasonWithRecovery(ctx, prompt)
			if reasonErr != nil {
				if ctx.Err() != nil {
					return l.result(StopCancelled, iteration, "context cancelled during reason")
				}
				if strings.Contains(reasonErr.Error(), "invalid transition") {
					// State-machine refusals keep their pre-v13-F1 terminal
					// semantics: suspension/retirement is constitutional
					// authority, not provider weather — neither retried,
					// escalated, nor parked around (codex r1 finding 1). The
					// RunConcurrent obituary makes the exit visible.
					return l.result(StopError, iteration, fmt.Sprintf("reason: %v", reasonErr))
				}
				// v13-F1: a Reason failure must NEVER kill the loop silently —
				// the v13 run lost its only CanOperate agent to one transient
				// API 529 and froze, healthy-looking, at the
				// decomposition→assignment boundary. Raise on-chain ONCE per
				// failure episode, then mirror v8-F2: keepalive parks for
				// wake/re-check (the recovery horizon for outages that outlast
				// the in-iteration retries); one-shot loops stop terminally
				// but VISIBLY.
				detail := fmt.Sprintf("reason failed after %d attempts (iteration %d): %v — prompt_chars=%d",
					len(l.reasonRetryBackoff)+1, iteration, reasonErr, len(prompt))
				fmt.Printf("[%s] %s\n", l.agent.Name(), detail)
				// The FINAL failed attempt has no post-backoff heal behind
				// it: if its cleanup write failed too, the agent arrives
				// here stranded in Processing and Escalate's own
				// Idle→Processing transition would refuse — the one raise
				// the contract guarantees would be lost (codex r3). Heal
				// first; gated, so authority states stay untouchable.
				l.agent.ResetIfStuckProcessing()
				if !l.reasonFailureEscalated {
					if err := l.agent.Escalate(ctx, l.humanID, detail); err != nil {
						// Flag stays UNSET on a failed chain write: the next
						// exhaustion must retry the raise, or no
						// agent.escalated ever reaches the chain for this
						// episode (codex r1 finding 2).
						fmt.Printf("warning: escalation event failed: %v\n", err)
					} else {
						l.reasonFailureEscalated = true
					}
				}
				if l.config.Keepalive && l.config.Bus != nil {
					fmt.Printf("[%s] parked pending wake/re-check after reason failure\n", l.agent.Name())
					if l.waitForEvents(ctx) {
						consecutiveEmpty = 0
						continue
					}
					return l.result(StopEscalation, iteration,
						"shutdown while parked on reason failure: "+detail)
				}
				return l.result(StopEscalation, iteration, detail)
			}
			l.reasonFailureEscalated = false
			// Record the reason call's usage. (The Operate branch records its own
			// above, before its gate, so neither path can skip budget accounting.)
			l.budget.RecordUsage(usage)
		}

		if l.config.OnIteration != nil {
			l.config.OnIteration(iteration, response)
		}

		if l.sink != nil && l.heartbeatInterval > 0 {
			if l.iteration-l.lastCheckpointIter >= l.heartbeatInterval {
				l.sink.OnHeartbeat(l.currentSnapshot())
				l.lastCheckpointIter = l.iteration
			}
		}

		// 2.5. PROCESS work graph commands from the response.
		l.processTaskCommands(ctx, response)
		l.processPhaseCommands(response)

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
			} else {
				if l.sink != nil {
					l.captureBoundary(checkpoint.BudgetAdjusted, response)
					l.lastCheckpointIter = l.iteration
				}
			}
		}

		// 2.8. PROCESS /gap and /directive commands from the response (CTO only).
		if l.ctoCooldowns != nil {
			if cmd := parseGapCommand(response); cmd != nil {
				if err := l.validateAndEmitGap(cmd, iteration); err != nil {
					fmt.Printf("warning: /gap rejected: %v\n", err)
				} else {
					if l.sink != nil {
						l.captureBoundary(checkpoint.GapEmitted, response)
						l.lastCheckpointIter = l.iteration
					}
				}
			}
			if cmd := parseDirectiveCommand(response); cmd != nil {
				if err := l.validateAndEmitDirective(cmd, iteration); err != nil {
					fmt.Printf("warning: /directive rejected: %v\n", err)
				} else {
					if l.sink != nil {
						l.captureBoundary(checkpoint.DirectiveEmitted, response)
						l.lastCheckpointIter = l.iteration
					}
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
				} else {
					if l.sink != nil {
						l.captureBoundary(checkpoint.RoleProposed, response)
						l.lastCheckpointIter = l.iteration
					}
				}
			}
		}

		// 2.10. PROCESS /review command from the response (Reviewer only).
		if l.reviewerState != nil {
			if cmd := parseReviewCommand(response); cmd != nil {
				if err := validateReviewCommand(cmd, l.iteration); err != nil {
					fmt.Printf("[%s] /review rejected: %v\n", l.agent.Name(), err)
				} else if l.reviewerState.shouldEscalate(cmd.TaskID) {
					// The cap blocks every further verdict for this task —
					// including approve — so the escalation must reach the
					// chain: a log line is observable by no other agent
					// (run findings v12-F1).
					fmt.Printf("[%s] review cycle limit reached for %s, escalating\n",
						l.agent.Name(), cmd.TaskID)
					l.emitReviewEscalationOnce(cmd.TaskID, cmd.Issues)
				} else {
					if err := l.emitCodeReview(cmd); err != nil {
						fmt.Printf("[%s] /review emit failed: %v\n", l.agent.Name(), err)
					} else {
						// Record directly at emission time. The chain fold
						// (foldChainEvent) skips source == self for exactly
						// this reason — recording own reviews twice would
						// inflate the escalation cap.
						l.reviewerState.recordReview(
							cmd.TaskID, cmd.Verdict, cmd.Issues)
						// The review→fix return edge (run findings v12-F1):
						// request_changes reopens the task for its producer,
						// or escalates when this verdict consumed the cap.
						l.routeReviewVerdict(cmd)
						if l.config.OnReviewCompleted != nil {
							l.config.OnReviewCompleted(ctx, cmd.TaskID, cmd.Verdict)
						}
						if l.sink != nil {
							l.captureBoundary(checkpoint.ReviewCompleted, response)
							l.lastCheckpointIter = l.iteration
						}
					}
				}
			}
		}

		// 2.11. PROCESS /approve and /reject commands from the response (Guardian only).
		if string(l.agent.Role()) == "guardian" {
			if cmd := parseApproveCommand(response); cmd != nil {
				if err := l.emitRoleApproved(cmd); err != nil {
					fmt.Printf("[%s] /approve emit failed: %v\n", l.agent.Name(), err)
				} else {
					if l.sink != nil {
						l.captureBoundary(checkpoint.RoleDecided, response)
						l.lastCheckpointIter = l.iteration
					}
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

		// 3.5. A keepalive escalation RAISES and PARKS (v8-F2): the escalation
		// event is already on the chain for the human; the loop waits for the
		// next wake or gated re-check and re-evaluates with fresh context —
		// iterating immediately would re-prompt into the same blocked state at
		// LLM-call cost. Without a bus there is no wake source, so parking is
		// impossible and the escalation stays terminal (fail closed to the
		// pre-fix semantics rather than spinning).
		if l.parkAfterEscalation {
			reason := l.parkedEscalationReason
			l.parkAfterEscalation = false
			l.parkedEscalationReason = ""
			if l.config.Bus == nil {
				return l.result(StopEscalation, iteration, reason)
			}
			fmt.Printf("[%s] escalation raised; keepalive loop parked pending wake/re-check\n", l.agent.Name())
			if l.waitForEvents(ctx) {
				consecutiveEmpty = 0
				continue
			}
			// waitForEvents returns false only on cancellation for keepalive
			// loops: retire with the outstanding, unanswered escalation on
			// record (mirroring the quiescence branch's wait-context result),
			// named as a shutdown so it cannot read as a live terminal stop.
			return l.result(StopEscalation, iteration, "shutdown while parked on escalation: "+reason)
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
	// Update reviewer cross-iteration state from this iteration's events, then
	// advance the projection from the chain watermark so the review context
	// reflects everything persisted up to now — including completions written
	// by external processes (work-server/CLI) the bus never delivers. Best
	// effort: on a store error the observation renders the previous projection
	// and the next evaluation resumes from the unadvanced watermark.
	if l.reviewerState != nil {
		l.reviewerState.update(pending)
		l.catchUpReviewProjection()
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

	if l.config.RecoveryState != nil && l.config.RecoveryState.Mode == checkpoint.ModeWarm && iteration <= l.config.RecoveryState.Iteration+1 {
		sb.WriteString("\n## Recovery Context\nYou are resuming after a restart. Your last checkpoint:\n")
		sb.WriteString(l.config.RecoveryState.Intent)
		sb.WriteString("\n\n")
		if l.config.RecoveryState.HiveSummary != "" {
			sb.WriteString("Hive context:\n")
			sb.WriteString(l.config.RecoveryState.HiveSummary)
			sb.WriteString("\n\n")
		}
		sb.WriteString("Resume from where you left off. Do not restart completed work.\n\n")
	}

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

var errTaskOperateProvider = errors.New("task operate provider")
var errTaskWorkspaceProvider = errors.New("task workspace provider")

type taskOperateWorkspace struct {
	RepoPath              string
	ContainmentWatchRoots []string
}

func (l *Loop) taskWorkspace(ctx context.Context, task work.Task) (taskOperateWorkspace, error) {
	workspace := taskOperateWorkspace{
		RepoPath:              l.config.RepoPath,
		ContainmentWatchRoots: append([]string(nil), l.config.ContainmentWatchRoots...),
	}
	if l.config.TaskWorkspaceProvider == nil {
		return workspace, nil
	}
	selected, err := l.config.TaskWorkspaceProvider(ctx, task, string(l.agent.Role()))
	if err != nil {
		return taskOperateWorkspace{}, fmt.Errorf("%w: %v", errTaskWorkspaceProvider, err)
	}
	if !selected.Applied {
		return workspace, nil
	}
	if strings.TrimSpace(selected.RepoPath) == "" {
		return taskOperateWorkspace{}, fmt.Errorf("%w: selected repo path is empty", errTaskWorkspaceProvider)
	}
	return taskOperateWorkspace{
		RepoPath:              strings.TrimSpace(selected.RepoPath),
		ContainmentWatchRoots: append([]string(nil), selected.ContainmentWatchRoots...),
	}, nil
}

func (l *Loop) operateTask(ctx context.Context, task work.Task, instruction, repoPath string) (decision.OperateResult, error) {
	if l.config.TaskOperateProvider != nil {
		selected, err := l.config.TaskOperateProvider(ctx, task, string(l.agent.Role()))
		if err != nil {
			return decision.OperateResult{}, fmt.Errorf("%w: %v", errTaskOperateProvider, err)
		}
		if selected.Applied {
			if selected.Provider == nil {
				return decision.OperateResult{}, fmt.Errorf("%w: selected provider is nil", errTaskOperateProvider)
			}
			return l.agent.OperateWithProvider(ctx, selected.Provider, repoPath, instruction)
		}
	}
	return l.agent.Operate(ctx, repoPath, instruction)
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
			// Heal a stranded-Processing state before retry — the failed
			// iteration may have left the agent mid-transition, which would
			// fail the next Reason() with Processing → Processing. Gated
			// (v13-F1 codex r1): only Processing is reset, never an
			// authority state.
			l.agent.ResetIfStuckProcessing()
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
			continue
		}
		return "", decision.TokenUsage{}, err
	}
	// Unreachable, but satisfies the compiler.
	return "", decision.TokenUsage{}, fmt.Errorf("reason: exhausted retries")
}

// reasonWithRecovery wraps reason with the v13-F1 retry schedule: a failed
// Reason call waits out reasonRetryBackoff[attempt] (jittered, context-aware)
// and tries again within the same iteration, absorbing transient provider
// errors — the 529 family — that previously killed the loop on first contact.
//
// State-machine refusals ("invalid transition") are NOT retryable here:
// they are authority, not weather. A suspended or retired agent fails the
// Idle→Processing transition by DESIGN, and a retry loop must neither spin
// on it nor reset the agent back to Idle — ResetToIdle from Suspended would
// silently override a Guardian suspension (codex r1 finding 1). The
// stranded-in-Processing recovery that does warrant a reset lives inside
// reason()'s chain-integrity retry, where the stranding cause is known.
func (l *Loop) reasonWithRecovery(ctx context.Context, prompt string) (string, decision.TokenUsage, error) {
	attempts := len(l.reasonRetryBackoff) + 1
	// v14-F1 observability: three 10-minute reason kills left no record of
	// what was sent (--no-session-persistence) — prompt size is the first
	// discriminator between prompt-bloat and provider hangs.
	fmt.Printf("%s\n", formatReasonPromptSize(l.agent.Name(), len(prompt), l.iteration))
	var lastErr error
	for attempt := 1; ; attempt++ {
		content, usage, err := l.reason(ctx, prompt)
		if err == nil {
			return content, usage, nil
		}
		lastErr = err
		if strings.Contains(err.Error(), "invalid transition") {
			if !l.agent.ResetIfStuckProcessing() {
				// An invalid transition with the agent NOT stranded in
				// Processing is authority, not weather — a Suspended or
				// Retired refusal must be neither retried nor reset around
				// (codex r1 finding 1). Not retryable.
				return "", decision.TokenUsage{}, lastErr
			}
			// Stranded in Processing by a failed cleanup write — the gated
			// reset healed exactly that shape (codex r2 finding 1); the
			// retry below is safe.
		}
		if ctx.Err() != nil || attempt >= attempts {
			return "", decision.TokenUsage{}, lastErr
		}
		delay := l.reasonRetryBackoff[attempt-1]
		if delay > 0 {
			// Jitter de-synchronizes a whole-society retry herd: a provider
			// incident hits all nine agents at once, and identical schedules
			// would re-aggregate their retries into the same overloaded second.
			delay += rand.N(delay/2 + 1)
		}
		fmt.Printf("[%s] reason attempt %d/%d failed: %v — retrying in %s\n",
			l.agent.Name(), attempt, attempts, err, delay.Round(time.Second))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", decision.TokenUsage{}, lastErr
		}
		// A provider error whose Processing→Idle cleanup write failed leaves
		// the agent stranded with the stranding SWALLOWED (the error text is
		// the provider's, not the transition's). Heal before the retry so it
		// does not burn an attempt on Processing → Processing. Gated: only a
		// provably-stranded Processing state is touched, never authority.
		l.agent.ResetIfStuckProcessing()
	}
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

// containsHaltDirective reports whether a HALT directive appears ANYWHERE in
// the response — as a line-start text directive or on ANY /signal JSON line,
// not just the last one parseSignal returns. HALT is constitutional and must
// never be masked: the keepalive escalation park consults this so a mixed
// HALT+ESCALATE response keeps its terminal stop instead of parking past the
// HALT (codex review of #149, finding 1).
func containsHaltDirective(response string) bool {
	if ContainsSignal(response, SignalHalt) {
		return true
	}
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/signal ") {
			continue
		}
		var sig Signal
		if err := json.Unmarshal([]byte(strings.TrimPrefix(trimmed, "/signal ")), &sig); err == nil &&
			strings.ToUpper(sig.Signal) == SignalHalt {
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
		if l.sink != nil {
			l.captureBoundary(checkpoint.HaltSignal, response)
			l.lastCheckpointIter = l.iteration
		}
		r := l.result(StopHalt, iteration, sig.Reason)
		return &r
	case SignalEscalate:
		if err := l.agent.Escalate(ctx, l.humanID,
			fmt.Sprintf("loop iteration %d: %s", iteration, sig.Reason)); err != nil {
			fmt.Printf("warning: escalation event failed: %v\n", err)
		}
		if l.sink != nil {
			l.captureBoundary(checkpoint.TaskBlocked, response)
			l.lastCheckpointIter = l.iteration
		}
		// Keepalive: raise-and-park (v8-F2) — UNLESS the response also carries
		// a HALT directive anywhere. parseSignal returns only the LAST /signal
		// line and suppresses the text fallback, so a HALT earlier in the
		// response would otherwise be parked past; HALT is constitutional and
		// keeps its terminal stop.
		if l.config.Keepalive && !containsHaltDirective(response) {
			l.parkAfterEscalation = true
			l.parkedEscalationReason = sig.Reason
			return nil
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
		if l.sink != nil {
			l.captureBoundary(checkpoint.HaltSignal, response)
			l.lastCheckpointIter = l.iteration
		}
		r := l.result(StopHalt, iteration, response)
		return &r
	}
	if ContainsSignal(response, "ESCALATE") {
		if l.sink != nil {
			l.captureBoundary(checkpoint.TaskBlocked, response)
			l.lastCheckpointIter = l.iteration
		}
		if err := l.agent.Escalate(ctx, l.humanID,
			fmt.Sprintf("loop iteration %d: %s", iteration, response)); err != nil {
			fmt.Printf("warning: escalation event failed: %v\n", err)
		}
		// Keepalive: raise-and-park (v8-F2), same semantics as the /signal path.
		if l.config.Keepalive {
			l.parkAfterEscalation = true
			l.parkedEscalationReason = response
			return nil
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

	observable := isObservable(ev.Type())
	wake := isWakeWorthy(ev.Type())
	// agent.state.changed is suppressed at the type level (the Idle⇄Processing
	// churn that formed the wakeup storm), but a SIGNIFICANT lifecycle transition
	// (Suspended, Retiring, Retired, ...) must stay visible to peers and wake idle
	// governance agents — hiding a peer's suspension/retirement is a
	// governance-visibility loss, not churn. Only the churn states are dropped.
	if ev.Type() == event.EventTypeAgentStateChanged && !isChurnStateChange(ev) {
		observable = true
		wake = true
	}

	// Queue for the next observation. Peer evaluations (agent.evaluated) and
	// significant lifecycle changes are queued for governance visibility even when
	// they do not wake the agent — observe() renders pendingEvents, and the
	// agent's own Memory is self-scoped, so the bus is the only path by which a
	// peer's event reaches it. Bounded so a long keepalive sleep cannot accumulate
	// an unbounded backlog (invariant BOUNDED).
	if observable {
		l.mu.Lock()
		l.pendingEvents = append(l.pendingEvents, ev)
		if over := len(l.pendingEvents) - maxPendingEvents; over > 0 {
			l.pendingEvents = l.pendingEvents[over:]
		}
		l.mu.Unlock()
	}

	// Only substantive work wakes a quiescent keepalive agent. Per-iteration
	// lifecycle/telemetry churn (idle/processing state changes, observations,
	// evaluations) must not re-wake idle governance agents — they stay subscribed
	// and see any queued events on their next substantive wake.
	if !wake {
		return
	}

	// Signal the wake channel (non-blocking).
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// waitForEvents blocks until new events arrive or quiescence timeout.
// Returns true if events arrived, false if timed out.
// In keepalive mode, there is no quiescence timeout. A CanOperate keepalive
// agent (the implementer) and a keepalive agent with a review duty (the
// reviewer) additionally re-check durable state on Config.RecheckInterval so a
// dropped wake edge — or an event that became historical across a daemon
// restart — cannot park them forever; every other keepalive agent blocks on
// the wake channel indefinitely, consuming zero CPU until a bus event arrives.
func (l *Loop) waitForEvents(ctx context.Context) bool {
	if l.config.Keepalive {
		// Re-check eligibility is an ALLOWLIST of exactly three duties, each
		// with its own "work exists" gate so an idle agent stays parked — no
		// re-ignition of the wakeup storm the per-iteration timers were removed
		// to kill:
		//   - implementer: a wake edge dropped while it was mid-Operate cannot
		//     park it forever (hive#135), gated to hasAssignableWork;
		//   - reviewer: a completion persisted by a prior daemon instance never
		//     fires a fresh wake, stranding the review→fix loop (run findings
		//     F8), gated to hasReviewableWork;
		//   - allocator: the renewer must notice duration-parked renewables
		//     whose wake edge was lost (v15-F1b), gated to a CHANGED non-empty
		//     park set (hasParkedRenewables) so a decline-to-renew can never
		//     storm the allocator's own terminal iteration budget.
		// Keepalive agents with none of these duties keep pure wake-blocking.
		operateRecheck := l.config.CanOperate && l.config.RepoPath != ""
		reviewRecheck := l.reviewerState != nil
		allocRecheck := l.config.BudgetRegistry != nil &&
			string(l.agent.Role()) == "allocator"
		if l.config.RecheckInterval > 0 && (operateRecheck || reviewRecheck || allocRecheck) {
			ticker := time.NewTicker(l.config.RecheckInterval)
			defer ticker.Stop()
			for {
				select {
				case <-l.wake:
					return true
				case <-ticker.C:
					if operateRecheck && l.hasAssignableWork() {
						return true
					}
					if reviewRecheck && l.hasReviewableWork() {
						return true
					}
					if allocRecheck && l.hasParkedRenewables() {
						return true
					}
					// Nothing actionable — stay parked; re-arm on the next tick.
				case <-ctx.Done():
					return false
				}
			}
		}
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

// result creates a Result with budget snapshot. It is the single chokepoint
// every loop death passes through, so it owns the duration-park marker's
// death-clear (codex r2 #1/#2): a returned loop is a corpse, and a corpse
// must never read as a renewable — whatever path it died through
// (cancellation winning a wake race, a non-duration budget failure after a
// renewal, or any future return). Unconditional and idempotent: clearing an
// unset marker or an unregistered name is a no-op.
func (l *Loop) result(reason StopReason, iterations int, detail string) Result {
	if reg := l.config.BudgetRegistry; reg != nil {
		reg.SetDurationParked(l.agent.Name(), false)
	}
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
		// v14-F2 (perception half): live completion outranks both — the
		// v3.9 lifecycle never advances on legacy-flow tasks, so agents
		// reasoned over "[created]" for delivered work and re-claimed it.
		// An explicit reopen supersedes the completion and reads open again.
		if t.LegacyStatus == work.LegacyStatusCompleted {
			status = "completed"
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

		// Render what the task DEMANDS, bounded. The completion-discipline
		// contract binds agents to "the form the task demands"; with only
		// [status] UUID: Title rendered, that criterion was unevaluable from a
		// reasoning prompt — the v8 strategist truthfully escalated "seed task
		// has no description" about a task carrying a 6287-char spec, and the
		// v9 spawner could not see the order demanded a repository file.
		demand := taskDemandExcerpt(t.Description, 240)
		if len(t.ExpectedOutputs) > 0 {
			// The structured demand outranks the prose excerpt: an order's
			// artifact paths must stay visible even when the description
			// excerpt truncates before naming them.
			outputs := "expected outputs: " + strings.Join(t.ExpectedOutputs, ", ")
			if demand != "" {
				demand = outputs + " — " + demand
			} else {
				demand = outputs
			}
		}
		readiness := ""
		if !t.Ready {
			if len(t.MissingGates) > 0 {
				readiness += fmt.Sprintf(" [missing gates: %s]", strings.Join(t.MissingGates, ", "))
			}
			if len(t.MissingFacts) > 0 {
				readiness += fmt.Sprintf(" [missing facts: %s]", strings.Join(t.MissingFacts, ", "))
			}
		}
		if demand != "" || readiness != "" {
			sb.WriteString(fmt.Sprintf("  demand: %s%s\n", demand, readiness))
		}
		if contract := l.issueScanTaskContractContext(t); contract != "" {
			sb.WriteString(contract)
		}
	}

	return sb.String()
}

const (
	issueScanStageRoleContractTaskContextLabel   = "issue_scan_stage_role_contract"
	issueScanStageOutputContractTaskContextLabel = "issue_scan_stage_output_contract"
	issueScanStageRoleOutputTaskContextLabel     = "issue_scan_stage_role_output"
)

type issueScanTaskContextContract struct {
	RunID               string
	FactoryOrderID      string
	StageID             string
	StageIndex          int
	StageCount          int
	RequiredRoles       []string
	RequiredEvidence    []string
	AuthorityBoundary   string
	CompletionGate      string
	RoleOutputContracts []issueScanTaskContextRoleOutputContract
}

type issueScanTaskContextStage struct {
	ID                string   `json:"id"`
	RequiredRoles     []string `json:"required_roles"`
	RequiredEvidence  []string `json:"required_evidence"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

type issueScanTaskContextRoleOutputContract struct {
	Role              string   `json:"role"`
	CanOperate        bool     `json:"can_operate"`
	RequiredOutputs   []string `json:"required_outputs"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

type issueScanTaskContextAgentPlanStep struct {
	Role              string   `json:"role"`
	CanOperate        bool     `json:"can_operate"`
	RequiredOutputs   []string `json:"required_outputs"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

// issueScanTaskContractContext exposes issue-scan lifecycle contracts that are
// otherwise buried in Work artifacts. Civic agents reason from buildTaskContext,
// and stage descriptions are intentionally truncated; without this compact view
// they cannot reliably know the exact role-output keys a valid
// issue_scan_stage_role_output artifact must carry.
func (l *Loop) issueScanTaskContractContext(t work.TaskSummary) string {
	if l == nil || l.config.TaskStore == nil || t.ArtifactCount == 0 {
		return ""
	}
	artifacts, err := l.config.TaskStore.ListArtifacts(t.ID)
	if err != nil {
		return fmt.Sprintf("  issue-scan contract: unavailable (%s); do not emit %s from a hidden contract\n",
			taskDemandExcerpt(err.Error(), 160), issueScanStageRoleOutputTaskContextLabel)
	}
	contract, ok, err := issueScanTaskContractFromArtifacts(artifacts)
	if !ok {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("  issue-scan contract: invalid (%s); do not emit %s until the contract is visible\n",
			taskDemandExcerpt(err.Error(), 160), issueScanStageRoleOutputTaskContextLabel)
	}
	return renderIssueScanTaskContractContext(contract, string(l.agent.Role()))
}

func issueScanTaskContractFromArtifacts(artifacts []work.ArtifactEvent) (issueScanTaskContextContract, bool, error) {
	var outputBody string
	var roleBody string
	for _, artifact := range artifacts {
		switch normalizeGateLabel(artifact.Label) {
		case issueScanStageOutputContractTaskContextLabel:
			outputBody = artifact.Body
		case issueScanStageRoleContractTaskContextLabel:
			roleBody = artifact.Body
		}
	}
	if strings.TrimSpace(outputBody) != "" {
		contract, err := parseIssueScanOutputContract(outputBody)
		return contract, true, err
	}
	if strings.TrimSpace(roleBody) != "" {
		contract, err := parseIssueScanRoleContract(roleBody)
		return contract, true, err
	}
	return issueScanTaskContextContract{}, false, nil
}

func parseIssueScanOutputContract(body string) (issueScanTaskContextContract, error) {
	var payload struct {
		Kind                string                                   `json:"kind"`
		RunID               string                                   `json:"run_id"`
		FactoryOrderID      string                                   `json:"factory_order_id"`
		StageID             string                                   `json:"stage_id"`
		StageIndex          int                                      `json:"stage_index"`
		StageCount          int                                      `json:"stage_count"`
		Stage               issueScanTaskContextStage                `json:"stage"`
		RequiredEvidence    []string                                 `json:"required_evidence"`
		RoleOutputContracts []issueScanTaskContextRoleOutputContract `json:"role_output_contracts"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return issueScanTaskContextContract{}, err
	}
	if kind := strings.TrimSpace(payload.Kind); kind != "" && kind != issueScanStageOutputContractTaskContextLabel {
		return issueScanTaskContextContract{}, fmt.Errorf("kind %q is not %q", kind, issueScanStageOutputContractTaskContextLabel)
	}
	contract := issueScanTaskContextContract{
		RunID:               strings.TrimSpace(payload.RunID),
		FactoryOrderID:      strings.TrimSpace(payload.FactoryOrderID),
		StageID:             firstNonEmptyString(payload.StageID, payload.Stage.ID),
		StageIndex:          payload.StageIndex,
		StageCount:          payload.StageCount,
		RequiredRoles:       compactTaskContextStrings(payload.Stage.RequiredRoles),
		RequiredEvidence:    compactTaskContextStrings(payload.RequiredEvidence),
		AuthorityBoundary:   strings.TrimSpace(payload.Stage.AuthorityBoundary),
		CompletionGate:      strings.TrimSpace(payload.Stage.CompletionGate),
		RoleOutputContracts: compactIssueScanTaskRoleOutputContracts(payload.RoleOutputContracts),
	}
	if len(contract.RequiredEvidence) == 0 {
		contract.RequiredEvidence = compactTaskContextStrings(payload.Stage.RequiredEvidence)
	}
	if len(contract.RequiredRoles) == 0 {
		contract.RequiredRoles = issueScanTaskRoleNames(contract.RoleOutputContracts)
	}
	return contract, nil
}

func parseIssueScanRoleContract(body string) (issueScanTaskContextContract, error) {
	var payload struct {
		Kind               string                              `json:"kind"`
		RunID              string                              `json:"run_id"`
		FactoryOrderID     string                              `json:"factory_order_id"`
		StageID            string                              `json:"stage_id"`
		StageIndex         int                                 `json:"stage_index"`
		StageCount         int                                 `json:"stage_count"`
		Stage              issueScanTaskContextStage           `json:"stage"`
		AgentExecutionPlan []issueScanTaskContextAgentPlanStep `json:"agent_execution_plan"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return issueScanTaskContextContract{}, err
	}
	if kind := strings.TrimSpace(payload.Kind); kind != "" && kind != issueScanStageRoleContractTaskContextLabel {
		return issueScanTaskContextContract{}, fmt.Errorf("kind %q is not %q", kind, issueScanStageRoleContractTaskContextLabel)
	}
	roleOutputContracts := make([]issueScanTaskContextRoleOutputContract, 0, len(payload.AgentExecutionPlan))
	for _, step := range payload.AgentExecutionPlan {
		roleOutputContracts = append(roleOutputContracts, issueScanTaskContextRoleOutputContract{
			Role:              step.Role,
			CanOperate:        step.CanOperate,
			RequiredOutputs:   step.RequiredOutputs,
			AuthorityBoundary: step.AuthorityBoundary,
			CompletionGate:    step.CompletionGate,
		})
	}
	contract := issueScanTaskContextContract{
		RunID:               strings.TrimSpace(payload.RunID),
		FactoryOrderID:      strings.TrimSpace(payload.FactoryOrderID),
		StageID:             firstNonEmptyString(payload.StageID, payload.Stage.ID),
		StageIndex:          payload.StageIndex,
		StageCount:          payload.StageCount,
		RequiredRoles:       compactTaskContextStrings(payload.Stage.RequiredRoles),
		RequiredEvidence:    compactTaskContextStrings(payload.Stage.RequiredEvidence),
		AuthorityBoundary:   strings.TrimSpace(payload.Stage.AuthorityBoundary),
		CompletionGate:      strings.TrimSpace(payload.Stage.CompletionGate),
		RoleOutputContracts: compactIssueScanTaskRoleOutputContracts(roleOutputContracts),
	}
	if len(contract.RequiredRoles) == 0 {
		contract.RequiredRoles = issueScanTaskRoleNames(contract.RoleOutputContracts)
	}
	return contract, nil
}

func renderIssueScanTaskContractContext(contract issueScanTaskContextContract, agentRole string) string {
	var b strings.Builder
	parts := []string{}
	if contract.RunID != "" {
		parts = append(parts, "run "+contract.RunID)
	}
	if contract.FactoryOrderID != "" {
		parts = append(parts, "FactoryOrder "+contract.FactoryOrderID)
	}
	stage := contract.StageID
	if stage == "" {
		stage = "unknown-stage"
	}
	stagePart := "stage " + stage
	if contract.StageIndex > 0 && contract.StageCount > 0 {
		stagePart += fmt.Sprintf(" (%d/%d)", contract.StageIndex, contract.StageCount)
	}
	parts = append(parts, stagePart)
	fmt.Fprintf(&b, "  issue-scan contract: %s\n", strings.Join(parts, ", "))
	if evidence := formatTaskContextList(contract.RequiredEvidence, 12); evidence != "" {
		fmt.Fprintf(&b, "  issue-scan required evidence: %s\n", evidence)
	}
	boundaries := []string{}
	if contract.AuthorityBoundary != "" {
		boundaries = append(boundaries, "authority "+contract.AuthorityBoundary)
	}
	if contract.CompletionGate != "" {
		boundaries = append(boundaries, "gate "+contract.CompletionGate)
	}
	if len(boundaries) > 0 {
		fmt.Fprintf(&b, "  issue-scan boundary: %s\n", strings.Join(boundaries, "; "))
	}

	agentRole = strings.TrimSpace(agentRole)
	roleContract, roleMatched := issueScanTaskRoleOutputContractForAgent(contract.RoleOutputContracts, agentRole)
	if roleMatched {
		outputs := formatTaskContextList(roleContract.RequiredOutputs, 12)
		if outputs == "" {
			outputs = "none declared; include any stage-required evidence keys you substantiate"
		}
		fmt.Fprintf(&b, "  issue-scan your role (%s) outputs: %s\n", roleContract.Role, outputs)
		fmt.Fprintf(&b, "  issue-scan role artifact: attach label %s with role=%s, summary, evidence_refs, and outputs covering your role keys plus any stage-required evidence keys you substantiate; this is not stage completion, PR readiness, Human approval, merge, or deploy\n",
			issueScanStageRoleOutputTaskContextLabel, roleContract.Role)
		return b.String()
	}

	roles := formatTaskContextList(contract.RequiredRoles, 12)
	if roles == "" {
		roles = formatTaskContextList(issueScanTaskRoleNames(contract.RoleOutputContracts), 12)
	}
	if roles != "" {
		if agentRole != "" {
			fmt.Fprintf(&b, "  issue-scan roles: %s; your role (%s) has no declared role-output contract for this stage\n", roles, agentRole)
		} else {
			fmt.Fprintf(&b, "  issue-scan roles: %s\n", roles)
		}
	}
	fmt.Fprintf(&b, "  issue-scan role artifact: emit %s only when your role is declared for this stage; this is not stage completion, PR readiness, Human approval, merge, or deploy\n",
		issueScanStageRoleOutputTaskContextLabel)
	return b.String()
}

func issueScanTaskRoleOutputContractForAgent(contracts []issueScanTaskContextRoleOutputContract, agentRole string) (issueScanTaskContextRoleOutputContract, bool) {
	agentRole = strings.TrimSpace(agentRole)
	if agentRole == "" {
		return issueScanTaskContextRoleOutputContract{}, false
	}
	for _, contract := range contracts {
		if strings.EqualFold(strings.TrimSpace(contract.Role), agentRole) {
			return contract, true
		}
	}
	return issueScanTaskContextRoleOutputContract{}, false
}

func compactIssueScanTaskRoleOutputContracts(values []issueScanTaskContextRoleOutputContract) []issueScanTaskContextRoleOutputContract {
	seen := map[string]bool{}
	out := make([]issueScanTaskContextRoleOutputContract, 0, len(values))
	for _, value := range values {
		role := strings.TrimSpace(value.Role)
		if role == "" {
			continue
		}
		key := strings.ToLower(role)
		if seen[key] {
			continue
		}
		seen[key] = true
		value.Role = role
		value.RequiredOutputs = compactTaskContextStrings(value.RequiredOutputs)
		value.AuthorityBoundary = strings.TrimSpace(value.AuthorityBoundary)
		value.CompletionGate = strings.TrimSpace(value.CompletionGate)
		out = append(out, value)
	}
	return out
}

func issueScanTaskRoleNames(contracts []issueScanTaskContextRoleOutputContract) []string {
	names := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		names = append(names, contract.Role)
	}
	return compactTaskContextStrings(names)
}

func compactTaskContextStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func formatTaskContextList(values []string, max int) string {
	values = compactTaskContextStrings(values)
	if len(values) == 0 {
		return ""
	}
	if max <= 0 || len(values) <= max {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:max], ", ") + fmt.Sprintf(" (+%d more)", len(values)-max)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// taskDemandExcerpt renders a one-line, rune-safe excerpt of a task
// description for the task list. Newlines collapse to spaces so the excerpt
// stays one line; truncation counts RUNES, never bytes — a byte cut can split
// a multibyte sequence and produce invalid UTF-8 (the v9-F1 telemetry class).
func taskDemandExcerpt(desc string, maxRunes int) string {
	desc = strings.Join(strings.Fields(desc), " ")
	if desc == "" {
		return ""
	}
	runes := []rune(desc)
	if len(runes) <= maxRunes {
		return desc
	}
	return string(runes[:maxRunes]) + "…"
}

// processTaskCommands extracts and executes /task commands from the response.
func (l *Loop) processTaskCommands(ctx context.Context, response string) {
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

	executed := executeTaskCommands(commands, l.config.TaskStore, l.agent.ID(), causes, l.config.ConvID, l.config.CanOperate)
	if executed > 0 {
		fmt.Printf("[%s] executed %d/%d task commands\n", l.agent.Name(), executed, len(commands))
		if l.config.OnTaskCommandsExecuted != nil {
			l.config.OnTaskCommandsExecuted(ctx, executed, len(commands))
		}
	}
}

// processPhaseCommands extracts and executes /phase commands from the response.
func (l *Loop) processPhaseCommands(response string) {
	if l.config.PhaseGateStore == nil {
		return
	}

	commands := parsePhaseCommands(response)
	if len(commands) == 0 {
		return
	}

	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}

	executed := executePhaseCommands(commands, l.config.PhaseGateStore, l.agent.ID(), causes, l.config.ConvID)
	if executed > 0 {
		fmt.Printf("[%s] executed %d/%d phase commands\n", l.agent.Name(), executed, len(commands))
	}
}

// autoAssignOpenTask finds the first open, unassigned task and assigns it to
// this agent. This lets the Operate path activate without waiting for the LLM
// to emit a /task assign command via Reason.
func (l *Loop) autoAssignOpenTask() {
	t, ok := l.firstAssignableOpenTask()
	if !ok {
		return
	}
	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}
	if err := l.config.TaskStore.Assign(l.agent.ID(), t.ID, l.agent.ID(), causes, l.config.ConvID); err == nil {
		fmt.Printf("  → auto-assigned canonical leaf: %s — %s\n", t.ID.Value(), t.Title)
		if l.sink != nil {
			l.sink.OnBoundary(checkpoint.TaskAssigned, l.currentSnapshot())
			l.lastCheckpointIter = l.iteration
		}
	}
}

// isOperableStatus reports whether a task in this v3.9 lifecycle status may be
// picked up for Operate. It is an ALLOWLIST — only the statuses the implementer
// loop is explicitly responsible for executing return true. Every other status
// is non-operable: blocked/terminal states, the repair and verification phases
// owned by other flows, and any unknown, future, or zero-value status. An
// execution barrier must default closed.
//
// StatusRunning is deliberately EXCLUDED: nothing in the runtime parks a task in
// Running for auto-pickup — it occurs only transiently inside the failure-block
// walk (Created→Ready→Running→Blocked). Treating Running as operable made a
// partially-applied block (a transition that advanced to Running but did not
// reach Blocked) fail open — the failed task would be re-Operated on restart.
// Excluding it keeps that case fail-closed. StatusReady stays operable because a
// blocked task is explicitly retried via Blocked→Ready.
func isOperableStatus(s work.TaskStatus) bool {
	switch s {
	case work.StatusCreated, // loop default; required by the auto-assignment path
		work.StatusReady: // fresh-planned work, or an explicit Blocked→Ready retry
		return true
	default:
		return false
	}
}

// taskIsOperable reports whether an assigned task may be Operated. It fails
// closed at every check: a status read/projection error, a legacy-completed
// task, a non-allowlisted v3.9 status, or a dependency block (IsBlocked) all make
// the task non-operable. An unverifiable or blocked task must never be treated as
// runnable.
func (l *Loop) taskIsOperable(taskID types.EventID) bool {
	legacy, err := l.config.TaskStore.GetCompatibilityStatus(taskID)
	if err != nil || legacy == work.LegacyStatusCompleted {
		return false
	}
	v39, err := l.config.TaskStore.GetStatus(taskID)
	if err != nil || !isOperableStatus(v39) {
		return false
	}
	if blocked, err := l.config.TaskStore.IsBlocked(taskID); err != nil || blocked {
		return false
	}
	return true
}

// hasAssignableWork reports whether the loop's next iteration would have operable
// work to run: a task already assigned to this agent, or an open unassigned ready
// leaf the auto-assign path would claim. It is the gate for the keepalive
// re-check timer (see Config.RecheckInterval) — sharing firstAssignableOpenTask
// with the auto-assign path so the gate can never drift from what the agent
// actually picks up.
func (l *Loop) hasAssignableWork() bool {
	if l.hasAssignedTask() {
		return true
	}
	_, ok := l.firstAssignableOpenTask()
	return ok
}

// firstAssignableOpenTask returns the open, unassigned, non-aggregate, ready
// task the auto-assign path would claim next, walking oldest→newest so the
// first canonical task wins over newer duplicate chains. An AGGREGATE — a task
// that declares dependencies — is never auto-assigned: it waits on its pieces
// (ListOpen hides it while any is uncompleted), and auto-assignment is never
// how it closes. Stated plainly: raw /task complete refuses readiness-gated
// tasks and the factory PR terminal path does not complete the order task, so
// the only remaining close-out for a gated aggregate is a CanOperate agent
// MANUALLY /task assign-ing it once unblocked (the command path does not apply
// this aggregate skip — the same command/auto predicate divergence as v9's
// blocked-state gap) and completing through the verified-Operate path. The
// aggregate lifecycle design — and unifying the command/auto predicates — is
// routed to G-2.x; the invariant THIS predicate owns is only "never
// auto-assign an aggregate". Skipping on dependents instead (the old
// childless-leaf rule) deadlocked against ListOpen's prerequisite semantics:
// a subtask depending on its parent was hidden as blocked while the parent was
// skipped for having a dependent — zero assignable tasks in either edge
// direction (run findings v11-F1). Issue-scan lifecycle stages are also
// skipped even when they have no Work dependencies: their contract artifacts
// make them governed aggregators that close only through recorded role-output
// evidence, not through the implementer's verified-Operate path. ok is false
// when none exists. Pure (no writes): shared by autoAssignOpenTask (which
// assigns the result) and hasAssignableWork (the re-check gate).
func (l *Loop) firstAssignableOpenTask() (work.Task, bool) {
	if l.config.TaskStore == nil {
		return work.Task{}, false
	}
	open, err := l.config.TaskStore.ListOpen()
	if err != nil || len(open) == 0 {
		return work.Task{}, false
	}
	summaries, sErr := l.config.TaskStore.ListSummaries(100)
	if sErr != nil {
		return work.Task{}, false
	}
	assignees := make(map[types.EventID]types.ActorID, len(summaries))
	for _, s := range summaries {
		assignees[s.ID] = s.Assignee
	}

	// ListOpen is store-order dependent. Walk from oldest to newest so the
	// first canonical task gets executed before newer duplicate chains.
	for i := len(open) - 1; i >= 0; i-- {
		t := open[i]
		if assignees[t.ID] != (types.ActorID{}) {
			continue
		}
		deps, depErr := l.config.TaskStore.GetDependencies(t.ID)
		if depErr != nil || len(deps) > 0 {
			continue
		}
		readiness, readyErr := l.config.TaskStore.Readiness(t.ID)
		if readyErr != nil || !readiness.Ready {
			continue
		}
		if hasIssueScanContract, contractErr := l.taskHasIssueScanStageContract(t.ID); contractErr != nil || hasIssueScanContract {
			continue
		}
		return t, true
	}
	return work.Task{}, false
}

func (l *Loop) taskHasIssueScanStageContract(taskID types.EventID) (bool, error) {
	if l == nil || l.config.TaskStore == nil {
		return false, nil
	}
	artifacts, err := l.config.TaskStore.ListArtifacts(taskID)
	if err != nil {
		return false, err
	}
	return artifactsContainIssueScanStageContract(artifacts), nil
}

func artifactsContainIssueScanStageContract(artifacts []work.ArtifactEvent) bool {
	for _, artifact := range artifacts {
		switch normalizeGateLabel(artifact.Label) {
		case issueScanStageRoleContractTaskContextLabel, issueScanStageOutputContractTaskContextLabel:
			return true
		}
	}
	return false
}

// hasAssignedTask returns true if the agent has any assigned, operable task.
func (l *Loop) hasAssignedTask() bool {
	if l.config.TaskStore == nil {
		return false
	}
	tasks, err := l.config.TaskStore.GetByAssignee(l.agent.ID())
	if err != nil || len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if l.taskIsOperable(t.ID) {
			return true
		}
	}
	return false
}

// nextAssignedTask returns the next operable task assigned to this agent.
func (l *Loop) nextAssignedTask() work.Task {
	tasks, _ := l.config.TaskStore.GetByAssignee(l.agent.ID())
	for _, t := range tasks {
		if l.taskIsOperable(t.ID) {
			return t
		}
	}
	return work.Task{}
}

// attachOperateArtifact captures the verified Operate commit range as a task
// artifact. Called after a successful Operate() and before completeTask() to
// satisfy the artifact gate in TaskStore.Complete().
// Note: causes may be empty on the agent's very first event (bootstrap case).
// The factory handles this by using the graph head as a fallback cause.
func (l *Loop) attachOperateArtifact(task work.Task, baseHead, postHead string) bool {
	return l.attachOperateArtifactInWorkspace(task, l.config.RepoPath, baseHead, postHead)
}

func (l *Loop) attachOperateArtifactInWorkspace(task work.Task, repoPath, baseHead, postHead string) bool {
	if l.config.TaskStore == nil {
		return true
	}
	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}

	body := buildOperateArtifactBody(repoPath, baseHead, postHead)
	err := l.config.TaskStore.AddArtifact(
		l.agent.ID(), task.ID,
		"Operate result",
		"text/plain",
		body,
		causes, l.config.ConvID,
	)
	if err != nil {
		fmt.Printf("[%s] warning: attach artifact failed: %v\n", l.agent.Name(), err)
		return false
	}
	return true
}

// buildOperateArtifactBody captures the verified commit range and changed file
// list from the repo. Returns a structured string the Reviewer can parse: a
// header block (commit:/base:/head:/range:) of machine-verified values, a blank
// line, then the `git diff --stat`. extractCommitRange parses only up to that
// blank line, so the agent-controlled filenames in the stat cannot spoof the
// header — preserve the blank-line separator if this format ever changes.
func buildOperateArtifactBody(repoPath, baseHead, postHead string) string {
	if repoPath == "" {
		return "(no repo path configured)"
	}
	if postHead == "" {
		postHead = gitCommand(repoPath, "log", "-1", "--format=%H")
	}
	if postHead == "" {
		return "(no commits)"
	}
	branch := gitCommand(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	branchLine := ""
	if branch != "" {
		branchLine = "\nbranch: " + branch
	}
	if baseHead == "" {
		stat := gitCommand(repoPath, "diff", postHead+"^.."+postHead, "--stat")
		return fmt.Sprintf("commit: %s%s\n\n%s", postHead, branchLine, stat)
	}
	diffRef := baseHead + ".." + postHead
	stat := gitCommand(repoPath, "diff", diffRef, "--stat")
	return fmt.Sprintf("commit: %s\nbase: %s\nhead: %s\nrange: %s%s\n\n%s", postHead, baseHead, postHead, diffRef, branchLine, stat)
}

// completeTask marks a task as completed in the task store.
func (l *Loop) completeTask(ctx context.Context, task work.Task, summary string) bool {
	if l.config.TaskStore == nil {
		return true
	}
	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}
	if err := l.config.TaskStore.Complete(l.agent.ID(), task.ID, summary, causes, l.config.ConvID); err != nil {
		fmt.Printf("warning: task complete failed: %v\n", err)
		return false
	} else {
		fmt.Printf("  → task completed: %s — %s\n", task.ID.Value(), task.Title)
		if l.config.OnTaskCompleted != nil {
			l.config.OnTaskCompleted(ctx, task, summary)
		}
	}
	return true
}

// currentSnapshot builds a LoopSnapshot from current loop state.
func (l *Loop) currentSnapshot() checkpoint.LoopSnapshot {
	snap := checkpoint.LoopSnapshot{
		Role:          string(l.agent.Role()),
		Iteration:     l.iteration,
		MaxIterations: l.config.Budget.MaxIterations,
		Signal:        checkpoint.SignalActive,
	}
	if l.budget != nil {
		bs := l.budget.Snapshot()
		snap.TokensUsed = bs.TokensUsed
		snap.CostUSD = bs.CostUSD
	}
	// Populate task fields from current assigned task, if any.
	if l.config.TaskStore != nil {
		if task := l.nextAssignedTask(); task.ID.Value() != "" {
			snap.CurrentTaskID = task.ID.Value()
			snap.CurrentTask = task.Title
			snap.TaskStatus = "in-progress"
		}
	}
	return snap
}

// AgentResult pairs a loop result with the agent's role and name,
// avoiding silent data loss when multiple agents share a role.
type AgentResult struct {
	Role   string
	Name   string
	Result Result
}

// RunConcurrent runs multiple agent loops concurrently and returns when all stop.
// formatLoopExit renders the one-line obituary RunConcurrent prints the
// moment any loop returns (v13-F1: results were invisible until every loop
// finished — a dead agent looked exactly like an idle one for the daemon's
// whole lifetime).
func formatLoopExit(r AgentResult) string {
	return fmt.Sprintf("[%s] loop exited: %s after %d iterations — %s",
		r.Name, r.Result.Reason, r.Result.Iterations, r.Result.Detail)
}

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
				fmt.Println(formatLoopExit(results[idx]))
				return
			}

			results[idx] = AgentResult{
				Role:   string(c.Agent.Role()),
				Name:   c.Agent.Name(),
				Result: l.Run(ctx),
			}
			fmt.Println(formatLoopExit(results[idx]))
		}(i, cfg)
	}

	wg.Wait()
	return results
}
