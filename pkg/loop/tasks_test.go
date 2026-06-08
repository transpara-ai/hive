package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

func TestIsMetaTaskBody(t *testing.T) {
	tests := []struct {
		title       string
		description string
		want        bool
	}{
		// Positive — each meta-task pattern should be detected.
		{"op=complete task-123", "", true},
		{"Close task build feature", "", true},
		{"Mark done: implement auth", "", true},
		{"Close the following tasks", "", true},

		// Pattern in description rather than title.
		{"Follow-up work", "op=complete the previous task", true},
		{"Wrap up", "close task xyz after review", true},
		{"Update status", "mark done all items in backlog", true},
		{"Batch", "close the following: task-1, task-2", true},

		// Case-insensitive matching.
		{"OP=COMPLETE task", "", true},
		{"CLOSE TASK now", "", true},
		{"MARK DONE", "", true},
		{"CLOSE THE FOLLOWING", "", true},

		// Negative — genuine new task descriptions.
		{"Build authentication module", "implement OAuth2 flow", false},
		{"Fix bug in task parser", "the parser drops empty lines", false},
		{"Add tests for loop", "cover edge cases in signal parsing", false},
		{"Deploy to production", "run fly deploy after build", false},
		{"", "", false},
	}

	for _, tt := range tests {
		got := isMetaTaskBody(tt.title, tt.description)
		if got != tt.want {
			t.Errorf("isMetaTaskBody(%q, %q) = %v, want %v", tt.title, tt.description, got, tt.want)
		}
	}
}

func TestParseTaskCommandsMetaTaskNotFiltered(t *testing.T) {
	// parseTaskCommands itself does NOT filter meta-tasks — that happens in
	// execTaskCreate. Verify the command is still parsed so the guard can fire.
	response := `/task create {"title": "close task xyz", "description": "mark done"}`
	commands := parseTaskCommands(response)
	if len(commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(commands))
	}
	if commands[0].Action != "create" {
		t.Errorf("action = %q, want %q", commands[0].Action, "create")
	}
}

func TestParseTaskCommandsArtifact(t *testing.T) {
	response := `/task artifact {"task_id":"evt_00000000000000000000000000000001","label":"definition_of_done","body":"done means tested"}`
	commands := parseTaskCommands(response)
	if len(commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(commands))
	}
	if commands[0].Action != "artifact" {
		t.Errorf("action = %q, want artifact", commands[0].Action)
	}
}

func TestExecTaskCreateRejectsMetaTask(t *testing.T) {
	// execTaskCreate must return an error for meta-task payloads before
	// reaching TaskStore.Create. A nil TaskStore is safe because the guard
	// fires before any store call.
	cases := []struct {
		name    string
		payload string
	}{
		{"op=complete in title", `{"title": "op=complete task-123"}`},
		{"close task in title", `{"title": "Close Task xyz"}`},
		{"mark done in description", `{"title": "Wrap up", "description": "mark done all items"}`},
		{"close the following in description", `{"title": "Batch", "description": "close the following: t1, t2"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execTaskCreate([]byte(tc.payload), nil, types.ActorID{}, nil, types.ConversationID{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "meta-task rejected") {
				t.Errorf("error %q does not contain \"meta-task rejected\"", err.Error())
			}
		})
	}
}

func TestExecTaskAssignRejectsUngatedTask(t *testing.T) {
	ts, causes := newTaskCommandStore(t)
	agentID := types.MustActorID("actor_00000000000000000000000000000111")
	convID := types.MustConversationID("conv_00000000000000000000000000000111")
	task, err := ts.Create(agentID, "Implement gate", "", causes, convID, work.PriorityMedium)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = execTaskAssign([]byte(`{"task_id":"`+task.ID.Value()+`","assignee":"self"}`), ts, agentID, causes, convID, true)
	if err == nil {
		t.Fatal("expected assignment error for ungated task, got nil")
	}
	if !strings.Contains(err.Error(), "missing gates") {
		t.Fatalf("error = %q, want missing gates", err.Error())
	}
}

func TestExecTaskArtifactEnablesAssignment(t *testing.T) {
	ts, causes := newTaskCommandStore(t)
	agentID := types.MustActorID("actor_00000000000000000000000000000112")
	convID := types.MustConversationID("conv_00000000000000000000000000000112")
	task, err := ts.Create(agentID, "Implement ready gate", "", causes, convID, work.PriorityMedium)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, label := range work.RequiredReadinessGateLabels() {
		payload := []byte(`{"task_id":"` + task.ID.Value() + `","label":"` + label + `","media_type":"text/markdown","body":"gate"}`)
		if err := execTaskArtifact(payload, ts, agentID, causes, convID); err != nil {
			t.Fatalf("execTaskArtifact %s: %v", label, err)
		}
	}
	if err := execTaskAssign([]byte(`{"task_id":"`+task.ID.Value()+`","assignee":"self"}`), ts, agentID, causes, convID, true); err != nil {
		t.Fatalf("execTaskAssign: %v", err)
	}
}

// TestImplementationTaskAssignAndCompleteGuards proves the implementation-task
// ownership + completion invariant across the input domain:
//   - assignment of a ready implementation task requires a CanOperate actor;
//   - completion of ANY task carrying a readiness contract (a gate present, even
//     partially gated) is refused via the raw /task complete path, regardless of
//     actor — implementation tasks complete only through the commit-verified
//     Operate path (handleOperateResult). This closes both the non-Operate comment
//     bypass (round 2) and the implementer's duplicate raw completion (Codex
//     review on hive#132);
//   - a task with no readiness gates is never blocked.
func TestImplementationTaskAssignAndCompleteGuards(t *testing.T) {
	ts, causes := newTaskCommandStore(t)
	agentID := types.MustActorID("actor_00000000000000000000000000000113")
	convID := types.MustConversationID("conv_00000000000000000000000000000113")

	// A fully gated implementation task.
	ready, err := ts.Create(agentID, "Write the catalog file", "", causes, convID, work.PriorityMedium)
	if err != nil {
		t.Fatalf("Create ready: %v", err)
	}
	for _, label := range work.RequiredReadinessGateLabels() {
		payload := []byte(`{"task_id":"` + ready.ID.Value() + `","label":"` + label + `","media_type":"text/markdown","body":"gate"}`)
		if err := execTaskArtifact(payload, ts, agentID, causes, convID); err != nil {
			t.Fatalf("execTaskArtifact %s: %v", label, err)
		}
	}
	readyAssign := []byte(`{"task_id":"` + ready.ID.Value() + `","assignee":"self"}`)
	readyComplete := []byte(`{"task_id":"` + ready.ID.Value() + `","summary":"done"}`)

	// Assignment: non-Operate refused, CanOperate permitted.
	if err := execTaskAssign(readyAssign, ts, agentID, causes, convID, false); err == nil || !strings.Contains(err.Error(), "implementation task") {
		t.Fatalf("non-Operate assign: want \"implementation task\" error, got %v", err)
	}
	if err := execTaskAssign(readyAssign, ts, agentID, causes, convID, true); err != nil {
		t.Fatalf("CanOperate assign: %v", err)
	}

	// Completion via raw /task complete is refused for a fully gated impl task —
	// reserved for the commit-verified Operate path.
	if err := execTaskComplete(readyComplete, ts, agentID, causes, convID); err == nil || !strings.Contains(err.Error(), "implementation contract") {
		t.Fatalf("raw complete of ready impl task: want \"implementation contract\" error, got %v", err)
	}

	// A PARTIALLY gated task (one readiness gate) is still an implementation task;
	// its raw completion is refused too (the gate-building window, P2).
	partial, err := ts.Create(agentID, "Half-gated impl task", "", causes, convID, work.PriorityMedium)
	if err != nil {
		t.Fatalf("Create partial: %v", err)
	}
	oneGate := work.RequiredReadinessGateLabels()[0]
	if err := execTaskArtifact([]byte(`{"task_id":"`+partial.ID.Value()+`","label":"`+oneGate+`","media_type":"text/markdown","body":"gate"}`), ts, agentID, causes, convID); err != nil {
		t.Fatalf("execTaskArtifact partial: %v", err)
	}
	partialComplete := []byte(`{"task_id":"` + partial.ID.Value() + `","summary":"done"}`)
	if err := execTaskComplete(partialComplete, ts, agentID, causes, convID); err == nil || !strings.Contains(err.Error(), "implementation contract") {
		t.Fatalf("raw complete of partially gated impl task: want \"implementation contract\" error, got %v", err)
	}

	// A task with NO readiness gates (the seed order / an analysis task) is never
	// blocked by the completion guard.
	plain, err := ts.Create(agentID, "Summarize findings", "", causes, convID, work.PriorityMedium)
	if err != nil {
		t.Fatalf("Create plain: %v", err)
	}
	plainComplete := []byte(`{"task_id":"` + plain.ID.Value() + `","summary":"posted as comment"}`)
	if err := execTaskComplete(plainComplete, ts, agentID, causes, convID); err != nil && strings.Contains(err.Error(), "implementation contract") {
		t.Errorf("guard wrongly fired on a non-implementation task: %v", err)
	}
}

func newTaskCommandStore(t *testing.T) (*work.TaskStore, []types.EventID) {
	t.Helper()
	_, g := agentWithGraph(t, newMockProvider(`/signal {"signal":"IDLE"}`))
	factory := event.NewEventFactory(g.Registry())
	head, err := g.Store().Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.IsNone() {
		t.Fatal("expected agent boot event cause")
	}
	return work.NewTaskStore(g.Store(), factory, &testSigner{}), []types.EventID{head.Unwrap().ID()}
}

func TestIsMetaTaskBodyTitleDescriptionJoin(t *testing.T) {
	// The join is title + " " + description — a pattern can span the boundary.
	// This tests that documented behaviour rather than treating it as a surprise.
	// "close the" in title + "following tasks" in description → matched.
	got := isMetaTaskBody("close the", "following tasks")
	if !got {
		t.Error("expected match when pattern spans title/description join boundary, got false")
	}
	// Ensure it doesn't false-positive on unrelated fragments.
	got = isMetaTaskBody("close the", "release branch")
	if got {
		t.Error("expected no match for unrelated fragments across boundary, got true")
	}
}
