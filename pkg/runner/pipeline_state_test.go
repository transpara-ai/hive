package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lovyou-ai/hive/pkg/api"
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
	sm := NewPipelineStateMachine(r)

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
	sm := NewPipelineStateMachine(r)

	// Cancel immediately so the main loop exits without invoking agents.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sm.Run(ctx) //nolint:errcheck — we expect ctx.Err()

	if sm.State() != StateBuilding {
		t.Errorf("existing-tasks path: state = %s, want %s", sm.State(), StateBuilding)
	}
}
