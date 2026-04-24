package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/transpara-ai/hive/pkg/api"
)

// TestPipelineTransitionValid verifies that valid (state, event) pairs produce
// the expected next state and agent name.
func TestPipelineTransitionValid(t *testing.T) {
	cases := []struct {
		from      PipelineState
		event     PipelineEvent
		wantState PipelineState
		wantAgent string
	}{
		{StateIdle, EventBoardClear, StateDirecting, "pm"},
		{StateDirecting, EventMilestoneCreated, StateScouting, "scout"},
		{StateDirecting, EventWorkExists, StateBuilding, "builder"},
		{StateDirecting, EventNoTasks, StateIdle, ""},
		{StateScouting, EventReportPosted, StatePlanning, "architect"},
		{StatePlanning, EventTasksCreated, StateBuilding, "builder"},
		{StatePlanning, EventNoTasks, StateIdle, ""},
		{StateBuilding, EventTaskDone, StateTesting, "tester"},
		{StateTesting, EventTestsPass, StateReviewing, "critic"},
		{StateReviewing, EventCritiquePass, StateReflecting, "reflector"},
		{StateReviewing, EventCritiqueRevise, StateBuilding, "builder"},
		{StateReflecting, EventReflectionDone, StateAuditing, "observer"},
		{StateAuditing, EventAuditDone, StateIdle, ""},
		{StateAuditing, EventBoardClear, StateIdle, ""},
	}

	for _, tc := range cases {
		sm := &PipelineStateMachine{state: tc.from}
		gotState, gotAgent, err := sm.Transition(tc.event)
		if err != nil {
			t.Errorf("Transition(%s, %s): unexpected error: %v", tc.from, tc.event, err)
			continue
		}
		if gotState != tc.wantState {
			t.Errorf("Transition(%s, %s): state = %s, want %s", tc.from, tc.event, gotState, tc.wantState)
		}
		if gotAgent != tc.wantAgent {
			t.Errorf("Transition(%s, %s): agent = %q, want %q", tc.from, tc.event, gotAgent, tc.wantAgent)
		}
	}
}

// TestPipelineTransitionInvalid verifies that an event not valid for the
// current state returns an error and the state is unchanged.
func TestPipelineTransitionInvalid(t *testing.T) {
	sm := &PipelineStateMachine{state: StateIdle}
	_, _, err := sm.Transition(EventTaskDone) // EventTaskDone is not valid from StateIdle
	if err == nil {
		t.Error("expected error for invalid event, got nil")
	}
	if sm.state != StateIdle {
		t.Errorf("state changed on invalid transition: got %s, want %s", sm.state, StateIdle)
	}
}

// newTasksServer creates a test HTTP server that returns tasks in BoardResponse format.
func newTasksServer(t *testing.T, tasks []api.Node) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := api.BoardResponse{Nodes: tasks}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode board response: %v", err)
		}
	}))
}

// TestRunBoardClearStartsAtDirecting verifies that when the board has no open
// tasks, Run() transitions through EventBoardClear and starts at StateDirecting.
func TestRunBoardClearStartsAtDirecting(t *testing.T) {
	srv := newTasksServer(t, []api.Node{}) // empty board
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  t.TempDir(),
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
	})
	sm := NewPipelineStateMachine(r, nil)

	// Cancel immediately so the main loop exits without invoking agents.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sm.Run(ctx) //nolint:errcheck — we expect ctx.Err()

	if sm.State() != StateDirecting {
		t.Errorf("board-clear path: state = %s, want %s", sm.State(), StateDirecting)
	}
}

// TestRunExistingTasksStartsAtBuilding verifies that when the board has open
// tasks, Run() skips Directing and starts at StateBuilding.
func TestRunExistingTasksStartsAtBuilding(t *testing.T) {
	openTask := api.Node{
		ID:    "task-1",
		Kind:  "task",
		State: "open",
		Title: "Some open task",
	}
	srv := newTasksServer(t, []api.Node{openTask})
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  t.TempDir(),
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
	})
	sm := NewPipelineStateMachine(r, nil)

	// Cancel immediately so the main loop exits without invoking agents.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sm.Run(ctx) //nolint:errcheck — we expect ctx.Err()

	if sm.State() != StateBuilding {
		t.Errorf("existing-tasks path: state = %s, want %s", sm.State(), StateBuilding)
	}
}

// TestRunActiveTaskWithChildrenStartsAtBuilding verifies that an active task
// with incomplete children is still treated as actionable work (not skipped).
func TestRunActiveTaskWithChildrenStartsAtBuilding(t *testing.T) {
	activeTask := api.Node{
		ID:         "task-1",
		Kind:       "task",
		State:      "active",
		Title:      "Active task with children",
		ChildCount: 1,
		ChildDone:  0,
	}
	srv := newTasksServer(t, []api.Node{activeTask})
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  t.TempDir(),
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
	})
	sm := NewPipelineStateMachine(r, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sm.Run(ctx) //nolint:errcheck — we expect ctx.Err()

	if sm.State() != StateBuilding {
		t.Errorf("active-with-children path: state = %s, want %s", sm.State(), StateBuilding)
	}
}

// TestInferEventPMWorkExists verifies that inferEvent("pm") returns
// EventWorkExists when actionable tasks exist but no milestone was created.
func TestInferEventPMWorkExists(t *testing.T) {
	activeTask := api.Node{
		ID:    "task-1",
		Kind:  "task",
		State: "active",
		Title: "Some active task",
		Body:  "Short body",
	}
	srv := newTasksServer(t, []api.Node{activeTask})
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
	})
	sm := NewPipelineStateMachine(r, nil)

	got := sm.inferEvent("pm")
	if got != EventWorkExists {
		t.Errorf("inferEvent(pm) with active task: got %s, want %s", got, EventWorkExists)
	}
}

// TestInferEventPMMilestoneCreated verifies that inferEvent("pm") returns
// EventMilestoneCreated for a high-priority task with a detailed body.
func TestInferEventPMMilestoneCreated(t *testing.T) {
	milestone := api.Node{
		ID:       "task-1",
		Kind:     "task",
		State:    "open",
		Title:    "Build auth system",
		Priority: "high",
		Body:     string(make([]byte, 250)), // > 200 chars
	}
	srv := newTasksServer(t, []api.Node{milestone})
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
	})
	sm := NewPipelineStateMachine(r, nil)

	got := sm.inferEvent("pm")
	if got != EventMilestoneCreated {
		t.Errorf("inferEvent(pm) with milestone: got %s, want %s", got, EventMilestoneCreated)
	}
}

// TestPipelineTransitionFromUnknownState verifies that transitioning from a
// state not in the transition table returns an error and leaves state unchanged.
func TestPipelineTransitionFromUnknownState(t *testing.T) {
	unknown := PipelineState("nonexistent")
	sm := &PipelineStateMachine{state: unknown}
	_, _, err := sm.Transition(EventBoardClear)
	if err == nil {
		t.Error("expected error for state with no transitions, got nil")
	}
	if sm.state != unknown {
		t.Errorf("state changed on error: got %s, want %s", sm.state, unknown)
	}
}

// TestInferEventCriticRevise verifies that inferEvent("critic") returns
// EventCritiqueRevise when critique.md contains "VERDICT: REVISE".
func TestInferEventCriticRevise(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", map[string]string{
		"critique.md": "Some review.\n\nVERDICT: REVISE",
	})
	// nil APIClient → readFromGraph returns "" → falls back to critique.md
	r := New(Config{HiveDir: hiveDir})
	sm := NewPipelineStateMachine(r, nil)

	got := sm.inferEvent("critic")
	if got != EventCritiqueRevise {
		t.Errorf("inferEvent(critic) with REVISE: got %s, want %s", got, EventCritiqueRevise)
	}
}

// TestInferEventCriticPass verifies that inferEvent("critic") returns
// EventCritiquePass when critique.md contains "VERDICT: PASS".
func TestInferEventCriticPass(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", map[string]string{
		"critique.md": "Looks good.\n\nVERDICT: PASS",
	})
	r := New(Config{HiveDir: hiveDir})
	sm := NewPipelineStateMachine(r, nil)

	got := sm.inferEvent("critic")
	if got != EventCritiquePass {
		t.Errorf("inferEvent(critic) with PASS: got %s, want %s", got, EventCritiquePass)
	}
}

// TestReviseCountIncrementsOnCritiqueRevise verifies that the reviseCount field
// increments each time EventCritiqueRevise is applied and does not increment
// for other events.
func TestReviseCountIncrementsOnCritiqueRevise(t *testing.T) {
	sm := &PipelineStateMachine{state: StateReviewing}

	// First REVISE loop.
	if _, _, err := sm.Transition(EventCritiqueRevise); err != nil {
		t.Fatalf("first revise transition: %v", err)
	}
	if sm.reviseCount != 1 {
		t.Errorf("reviseCount after first REVISE = %d, want 1", sm.reviseCount)
	}

	// Transition to reviewing again and apply a second REVISE.
	sm.state = StateReviewing
	if _, _, err := sm.Transition(EventCritiqueRevise); err != nil {
		t.Fatalf("second revise transition: %v", err)
	}
	if sm.reviseCount != 2 {
		t.Errorf("reviseCount after second REVISE = %d, want 2", sm.reviseCount)
	}

	// A non-REVISE transition must not affect reviseCount.
	sm.state = StateReviewing
	if _, _, err := sm.Transition(EventCritiquePass); err != nil {
		t.Fatalf("pass transition: %v", err)
	}
	if sm.reviseCount != 2 {
		t.Errorf("reviseCount changed on non-REVISE event: got %d, want 2", sm.reviseCount)
	}
}
