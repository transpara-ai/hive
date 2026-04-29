package runner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

// PipelineState represents the pipeline's current state.
// Transitions are event-driven — graph events trigger state changes,
// state changes invoke agents.
type PipelineState string

const (
	StateIdle       PipelineState = "idle"       // board clear, nothing to do
	StateDirecting  PipelineState = "directing"  // PM deciding direction
	StateScouting   PipelineState = "scouting"   // Scout identifying gaps
	StatePlanning   PipelineState = "planning"   // Architect decomposing
	StateBuilding   PipelineState = "building"   // Builder working tasks
	StateTesting    PipelineState = "testing"    // Tester verifying
	StateReviewing  PipelineState = "reviewing"  // Critic reviewing
	StateReflecting PipelineState = "reflecting" // Reflector recording
	StateAuditing   PipelineState = "auditing"   // Observer checking integrity
	StateEscalated  PipelineState = "escalated"  // blocked, waiting for human resolution
)

// PipelineEvent triggers a state transition.
type PipelineEvent string

const (
	EventBoardClear         PipelineEvent = "board.clear"
	EventMilestoneCreated   PipelineEvent = "milestone.created"
	EventReportPosted       PipelineEvent = "report.posted"
	EventTasksCreated       PipelineEvent = "tasks.created"
	EventTaskDone           PipelineEvent = "task.done"
	EventTestsPass          PipelineEvent = "tests.pass"
	EventCritiquePass       PipelineEvent = "critique.pass"
	EventCritiqueRevise     PipelineEvent = "critique.revise"
	EventReflectionDone     PipelineEvent = "reflection.done"
	EventAuditDone          PipelineEvent = "audit.done"
	EventWorkExists         PipelineEvent = "work.exists"
	EventNoTasks            PipelineEvent = "no.tasks"
	EventEscalation         PipelineEvent = "escalation"
	EventEscalationResolved PipelineEvent = "escalation.resolved"
)

// Transition maps (state, event) → next state.
var pipelineTransitions = map[PipelineState]map[PipelineEvent]PipelineState{
	StateIdle: {
		EventBoardClear: StateDirecting,
	},
	StateDirecting: {
		EventMilestoneCreated: StateScouting,
		EventWorkExists:       StateBuilding, // existing work, skip scout/architect
		EventNoTasks:          StateIdle,     // PM found nothing to do
	},
	StateScouting: {
		EventReportPosted: StatePlanning,
	},
	StatePlanning: {
		EventTasksCreated: StateBuilding,
		EventNoTasks:      StateIdle, // Architect couldn't decompose
	},
	StateBuilding: {
		EventTaskDone:   StateTesting,
		EventEscalation: StateEscalated, // Builder couldn't proceed
	},
	StateEscalated: {
		EventEscalationResolved: StateBuilding, // human unblocked the task
	},
	StateTesting: {
		EventTestsPass: StateReviewing,
	},
	StateReviewing: {
		EventCritiquePass:   StateReflecting,
		EventCritiqueRevise: StateBuilding, // fix loop
	},
	StateReflecting: {
		EventReflectionDone: StateAuditing,
	},
	StateAuditing: {
		EventAuditDone:  StateIdle,
		EventBoardClear: StateIdle, // audit found nothing
	},
}

// Agent invoked at each state.
var stateAgents = map[PipelineState]string{
	StateDirecting:  "pm",
	StateScouting:   "scout",
	StatePlanning:   "architect",
	StateBuilding:   "builder",
	StateTesting:    "tester",
	StateReviewing:  "critic",
	StateReflecting: "reflector",
	StateAuditing:   "observer",
}

// PipelineStateMachine is the event-driven pipeline.
// Instead of a for-loop over roles, events trigger transitions
// RunnerFactory creates a Runner for a given role. Each role gets its own
// provider with its own session ID — no shared sessions across roles.
type RunnerFactory func(role string) (*Runner, error)

// PostPhaseFunc is called after each phase completes. role is the agent that
// just ran. Implementations can use this to persist session state, emit
// metrics, etc.
type PostPhaseFunc func(role string, provider interface{})

// PhaseObserverFunc receives the structured diagnostic for each completed
// phase after the next transition event has been inferred.
type PhaseObserverFunc func(PhaseEvent)

// and transitions invoke agents.
type PipelineStateMachine struct {
	state         PipelineState
	runner        *Runner           // current runner (changes per role)
	makeRunner    RunnerFactory     // creates a fresh runner per role
	reviseCount   int               // how many REVISE loops this cycle
	postPhase     PostPhaseFunc     // optional callback after each phase
	phaseObserver PhaseObserverFunc // optional structured telemetry hook
	cycleID       string            // stable id for one pipeline cycle
	worktree      *WorktreeContext  // persists across phases for merge-after-PASS
}

// NewPipelineStateMachine creates a state machine with a runner factory.
func NewPipelineStateMachine(defaultRunner *Runner, factory RunnerFactory) *PipelineStateMachine {
	return &PipelineStateMachine{
		state:      StateIdle,
		runner:     defaultRunner,
		makeRunner: factory,
	}
}

// SetPostPhase registers a callback that runs after each phase completes.
func (sm *PipelineStateMachine) SetPostPhase(fn PostPhaseFunc) { sm.postPhase = fn }

// SetPhaseObserver registers a callback for structured phase diagnostics.
func (sm *PipelineStateMachine) SetPhaseObserver(fn PhaseObserverFunc) { sm.phaseObserver = fn }

// SetCycleID tags every phase diagnostic emitted by this run.
func (sm *PipelineStateMachine) SetCycleID(id string) { sm.cycleID = id }

// CurrentRunner returns the runner for the phase that just completed.
// Useful in PostPhase callbacks for reading cost or other phase metrics.
func (sm *PipelineStateMachine) CurrentRunner() *Runner { return sm.runner }

// State returns the current state.
func (sm *PipelineStateMachine) State() PipelineState { return sm.state }

// Transition attempts to move to the next state based on an event.
// Returns the new state and the agent to invoke, or an error if
// the transition is invalid.
func (sm *PipelineStateMachine) Transition(event PipelineEvent) (PipelineState, string, error) {
	transitions, ok := pipelineTransitions[sm.state]
	if !ok {
		return sm.state, "", fmt.Errorf("no transitions from state %q", sm.state)
	}

	next, ok := transitions[event]
	if !ok {
		// List valid events for this state.
		var valid []PipelineEvent
		for e := range transitions {
			valid = append(valid, e)
		}
		return sm.state, "", fmt.Errorf("invalid event %q in state %q (valid: %v)", event, sm.state, valid)
	}

	prev := sm.state
	sm.state = next
	agent := stateAgents[next]

	if event == EventCritiqueRevise {
		sm.reviseCount++
	}

	log.Printf("[pipeline] %s --%s--> %s (invoke: %s)", prev, event, next, agent)
	return next, agent, nil
}

// Run executes the pipeline as a state machine. Starts from idle,
// processes events until returning to idle.
func (sm *PipelineStateMachine) Run(ctx context.Context) error {
	// Check board to determine starting event.
	tasks, _ := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, "")
	hasWork := false
	hasFixes := false
	for _, t := range tasks {
		if t.Kind != "task" || t.State == "done" || t.State == "closed" {
			continue
		}
		if t.Pinned {
			continue // pinned goals are direction, not work
		}
		if t.State != "active" && t.ChildCount > 0 && t.ChildDone < t.ChildCount {
			continue // blocked by children (unless explicitly active)
		}
		hasWork = true
		if len(t.Title) > 4 && t.Title[:4] == "Fix:" {
			hasFixes = true
		}
	}

	if hasWork || hasFixes {
		// Jump straight to building.
		sm.state = StateBuilding
		log.Printf("[pipeline] board has work — starting at %s", sm.state)
	} else {
		// Empty board — start full cycle.
		if _, _, err := sm.Transition(EventBoardClear); err != nil {
			return err
		}
	}

	// Run until we return to idle.
	for sm.state != StateIdle {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Escalated state — no agent to invoke. Poll until resolved or timeout.
		if sm.state == StateEscalated {
			log.Printf("[pipeline] ⚠ ESCALATED — waiting for human resolution (timeout: 10m)")
			resolved := false
			deadline := time.After(10 * time.Minute)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for !resolved {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-deadline:
					log.Printf("[pipeline] escalation timeout — returning to idle")
					sm.state = StateIdle
					resolved = true
				case <-ticker.C:
					// Check if any escalated task was unblocked.
					tasks, _ := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, "")
					stillEscalated := false
					for _, t := range tasks {
						if t.Kind == "task" && t.State == "escalated" {
							stillEscalated = true
							break
						}
					}
					if !stillEscalated {
						log.Printf("[pipeline] escalation resolved — resuming")
						if _, _, err := sm.Transition(EventEscalationResolved); err != nil {
							sm.state = StateIdle
						}
						resolved = true
					}
				}
			}
			continue
		}

		agent := stateAgents[sm.state]
		if agent == "" {
			return fmt.Errorf("no agent for state %q", sm.state)
		}

		// Create a fresh runner for this role (own provider, own session).
		log.Printf("[pipeline] ── %s ── (%s)", agent, sm.state)
		phaseStart := time.Now()
		if sm.makeRunner != nil {
			if r, err := sm.makeRunner(agent); err == nil {
				sm.runner = r
			}
		}
		sm.runner.cfg.Role = agent
		sm.runner.cfg.OneShot = true
		sm.runner.done = false

		// Per-phase timeout: no single phase should run longer than 20 minutes.
		// This catches hung CLI subprocesses that survive the Operate timeout.
		phaseCtx, phaseCancel := context.WithTimeout(ctx, 20*time.Minute)
		sm.runner.runTick(phaseCtx)
		phaseCancel()
		phaseDuration := time.Since(phaseStart)
		if phaseCtx.Err() == context.DeadlineExceeded {
			log.Printf("[pipeline] ⚠ phase %s timed out after %s", agent, phaseDuration.Round(time.Second))
		}

		// Capture worktree from Builder for merge after Critic PASS.
		if agent == "builder" && sm.runner.Worktree() != nil {
			sm.worktree = sm.runner.Worktree()
		}

		// Post-phase callback (e.g., persist session IDs).
		if sm.postPhase != nil {
			sm.postPhase(agent, sm.runner.cfg.Provider)
		}

		// Determine the next event based on what happened.
		event := sm.inferEvent(agent)

		// After Critic PASS, merge the worktree branch into main.
		if event == EventCritiquePass && sm.worktree != nil {
			if err := sm.worktree.MergeToMain(); err != nil {
				log.Printf("[pipeline] merge conflict — escalating: %v", err)
				event = EventEscalation
			} else {
				sm.worktree.Cleanup()
				sm.worktree = nil
			}
		}

		// Record diagnostic for every phase — the hive's nervous system.
		boardOpen := 0
		if tasks, err := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, ""); err == nil {
			for _, t := range tasks {
				if t.Kind == "task" && t.State != "done" && t.State != "closed" {
					boardOpen++
				}
			}
		}
		diag := PhaseEvent{
			CycleID:       sm.cycleID,
			Phase:         agent,
			WorkflowStage: workflowStageForAgent(agent),
			Outcome:       string(event),
			Summary:       phaseSummary(agent, event, boardOpen, sm.reviseCount),
			Repo:          filepath.Base(sm.runner.cfg.RepoPath),
			InputRef:      phaseInputRef(agent),
			OutputRef:     phaseOutputRef(agent, event),
			BoardOpen:     boardOpen,
			ReviseCount:   sm.reviseCount,
			DurationSecs:  phaseDuration.Seconds(),
			InputTokens:   sm.runner.cost.InputTokens,
			OutputTokens:  sm.runner.cost.OutputTokens,
			CostUSD:       sm.runner.cost.TotalCostUSD,
		}
		sm.runner.appendDiagnostic(diag)
		if sm.phaseObserver != nil {
			sm.phaseObserver(diag)
		}
		if _, _, err := sm.Transition(event); err != nil {
			log.Printf("[pipeline] transition error: %v — returning to idle", err)
			sm.state = StateIdle
		}
	}

	return nil
}

func workflowStageForAgent(agent string) string {
	switch agent {
	case "pm":
		return "intake"
	case "scout":
		return "discovery"
	case "architect":
		return "design"
	case "builder":
		return "emission"
	case "tester":
		return "validation"
	case "critic":
		return "review"
	case "reflector":
		return "reporting"
	case "observer":
		return "audit"
	default:
		return "unknown"
	}
}

func phaseInputRef(agent string) string {
	switch agent {
	case "pm":
		return "work.board:pinned-goal"
	case "scout":
		return "work.board:milestone"
	case "architect":
		return "loop/scout.md"
	case "builder":
		return "work.board:active-task"
	case "tester":
		return "git.diff"
	case "critic":
		return "loop/build.md"
	case "reflector":
		return "loop/critique.md"
	case "observer":
		return "telemetry+graph"
	default:
		return ""
	}
}

func phaseOutputRef(agent string, event PipelineEvent) string {
	if event == EventEscalation {
		return "work.task:escalated"
	}
	switch agent {
	case "pm":
		return "work.task:milestone"
	case "scout":
		return "loop/scout.md"
	case "architect":
		return "work.task:subtasks"
	case "builder":
		return "loop/build.md"
	case "tester":
		return "test.report"
	case "critic":
		return "loop/critique.md"
	case "reflector":
		return "loop/reflections.md"
	case "observer":
		return "loop/diagnostics.jsonl"
	default:
		return ""
	}
}

func phaseSummary(agent string, event PipelineEvent, boardOpen, reviseCount int) string {
	stage := workflowStageForAgent(agent)
	switch event {
	case EventNoTasks:
		return fmt.Sprintf("%s found no actionable work", stage)
	case EventEscalation:
		return fmt.Sprintf("%s escalated; human input required", stage)
	case EventCritiqueRevise:
		return fmt.Sprintf("review requested revision #%d; %d tasks remain open", reviseCount, boardOpen)
	case EventCritiquePass:
		return fmt.Sprintf("review passed; %d tasks remain open", boardOpen)
	default:
		return fmt.Sprintf("%s completed with outcome %s; %d tasks remain open", stage, event, boardOpen)
	}
}

// inferEvent determines what event just occurred based on the agent that ran.
func (sm *PipelineStateMachine) inferEvent(agent string) PipelineEvent {
	switch agent {
	case "pm":
		// Check if milestone was created or actionable work exists.
		tasks, _ := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, "")
		hasActionableWork := false
		for _, t := range tasks {
			if t.Kind != "task" || t.State == "done" || t.State == "closed" || t.Pinned {
				continue
			}
			// Milestone: high-priority task with detailed body (PM just created it).
			if t.Priority == "high" && len(t.Body) > 200 {
				return EventMilestoneCreated
			}
			// Any actionable task counts as existing work.
			if t.State == "active" || (t.State == "open" && (t.ChildCount == 0 || t.ChildDone >= t.ChildCount)) {
				hasActionableWork = true
			}
		}
		if hasActionableWork {
			return EventWorkExists
		}
		return EventNoTasks

	case "scout":
		return EventReportPosted

	case "architect":
		tasks, _ := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, "")
		for _, t := range tasks {
			if t.Kind == "task" && t.State != "done" && t.State != "closed" {
				return EventTasksCreated
			}
		}
		return EventNoTasks

	case "builder":
		// Check if any task was escalated — the Builder called ESCALATE.
		tasks, _ := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, "")
		for _, t := range tasks {
			if t.Kind == "task" && t.State == "escalated" {
				return EventEscalation
			}
		}
		return EventTaskDone

	case "tester":
		return EventTestsPass

	case "critic":
		// Read critique for verdict.
		critique := sm.runner.readFromGraph("Critique:")
		if critique == "" {
			critique = readLoopArtifact(sm.runner.cfg.HiveDir, "critique.md")
		}
		if parseVerdict(critique) == "REVISE" {
			return EventCritiqueRevise
		}
		return EventCritiquePass

	case "reflector":
		return EventReflectionDone

	case "observer":
		return EventAuditDone

	default:
		return EventAuditDone
	}
}
