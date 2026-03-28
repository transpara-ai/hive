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
	StateTesting    PipelineState = "testing"     // Tester verifying
	StateReviewing  PipelineState = "reviewing"   // Critic reviewing
	StateReflecting PipelineState = "reflecting" // Reflector recording
	StateAuditing   PipelineState = "auditing"   // Observer checking integrity
)

// PipelineEvent triggers a state transition.
type PipelineEvent string

const (
	EventBoardClear      PipelineEvent = "board.clear"
	EventMilestoneCreated PipelineEvent = "milestone.created"
	EventReportPosted    PipelineEvent = "report.posted"
	EventTasksCreated    PipelineEvent = "tasks.created"
	EventTaskDone        PipelineEvent = "task.done"
	EventTestsPass       PipelineEvent = "tests.pass"
	EventCritiquePass    PipelineEvent = "critique.pass"
	EventCritiqueRevise  PipelineEvent = "critique.revise"
	EventReflectionDone  PipelineEvent = "reflection.done"
	EventAuditDone       PipelineEvent = "audit.done"
	EventNoTasks         PipelineEvent = "no.tasks"
)

// Transition maps (state, event) → next state.
var pipelineTransitions = map[PipelineState]map[PipelineEvent]PipelineState{
	StateIdle: {
		EventBoardClear: StateDirecting,
	},
	StateDirecting: {
		EventMilestoneCreated: StateScouting,
		EventNoTasks:          StateIdle, // PM found nothing to do
	},
	StateScouting: {
		EventReportPosted: StatePlanning,
	},
	StatePlanning: {
		EventTasksCreated: StateBuilding,
		EventNoTasks:      StateIdle, // Architect couldn't decompose
	},
	StateBuilding: {
		EventTaskDone: StateTesting,
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

// and transitions invoke agents.
type PipelineStateMachine struct {
	state        PipelineState
	runner       *Runner       // current runner (changes per role)
	makeRunner   RunnerFactory // creates a fresh runner per role
	reviseCount  int           // how many REVISE loops this cycle
}

// NewPipelineStateMachine creates a state machine with a runner factory.
func NewPipelineStateMachine(defaultRunner *Runner, factory RunnerFactory) *PipelineStateMachine {
	return &PipelineStateMachine{
		state:      StateIdle,
		runner:     defaultRunner,
		makeRunner: factory,
	}
}

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
	hasOpen := false
	hasFixes := false
	for _, t := range tasks {
		if t.Kind == "task" && t.State != "done" && t.State != "closed" && t.ChildCount == 0 {
			hasOpen = true
			if len(t.Title) > 4 && t.Title[:4] == "Fix:" {
				hasFixes = true
			}
		}
	}

	if hasOpen || hasFixes {
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
		sm.runner.runTick(ctx)
		phaseDuration := time.Since(phaseStart)

		// Determine the next event based on what happened.
		event := sm.inferEvent(agent)

		// Record diagnostic for every phase — the hive's nervous system.
		boardOpen := 0
		if tasks, err := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, ""); err == nil {
			for _, t := range tasks {
				if t.Kind == "task" && t.State != "done" && t.State != "closed" {
					boardOpen++
				}
			}
		}
		sm.runner.appendDiagnostic(PhaseEvent{
			Phase:        agent,
			Outcome:      string(event),
			Repo:         filepath.Base(sm.runner.cfg.RepoPath),
			BoardOpen:    boardOpen,
			ReviseCount:  sm.reviseCount,
			DurationSecs: phaseDuration.Seconds(),
			CostUSD:      sm.runner.cost.TotalCostUSD,
		})
		if _, _, err := sm.Transition(event); err != nil {
			log.Printf("[pipeline] transition error: %v — returning to idle", err)
			sm.state = StateIdle
		}
	}

	return nil
}

// inferEvent determines what event just occurred based on the agent that ran.
func (sm *PipelineStateMachine) inferEvent(agent string) PipelineEvent {
	switch agent {
	case "pm":
		// Check if milestone was created.
		tasks, _ := sm.runner.cfg.APIClient.GetTasks(sm.runner.cfg.SpaceSlug, "")
		for _, t := range tasks {
			if t.Kind == "task" && t.State != "done" && t.State != "closed" && t.Priority == "high" && len(t.Body) > 200 {
				return EventMilestoneCreated
			}
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
