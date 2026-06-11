package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/work"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v14-F2: /task assign must refuse a live-completed task
//
// The v14 resume epoch re-claimed the completed-and-approved catalog task
// through the /task assign command path — the auto-assign predicate
// (firstAssignableOpenTask → ListOpen) hides live-completed tasks, but the
// command path never consulted completion state at all: the predicate
// divergence the v11-F1 fix warned about. The implementer then sat
// "assigned" to a task taskIsOperable correctly refused to operate,
// reasoning in circles over contradictory state.
//
// A completed task re-enters circulation through exactly one door: an
// explicit reopen (work.task.reopened, the v12-F1 return edge).
// ════════════════════════════════════════════════════════════════════════

// completedImplementationTask creates, gates, assigns, and completes a task,
// returning the store and IDs for re-assignment attempts.
func completedImplementationTask(t *testing.T) (*work.TaskStore, work.Task, types.ActorID, types.ConversationID, []types.EventID) {
	t.Helper()
	ts, causes := newTaskCommandStore(t)
	agentID := types.MustActorID("actor_00000000000000000000000000000142")
	convID := types.MustConversationID("conv_00000000000000000000000000000142")
	task, err := ts.Create(agentID, "Implement roles catalog", "", causes, convID, work.PriorityMedium)
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
		t.Fatalf("first execTaskAssign: %v", err)
	}
	if err := ts.Complete(agentID, task.ID, "delivered as commit 4282e1c", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return ts, task, agentID, convID, causes
}

func TestExecTaskAssignRefusesCompletedTask(t *testing.T) {
	ts, task, agentID, convID, causes := completedImplementationTask(t)

	err := execTaskAssign([]byte(`{"task_id":"`+task.ID.Value()+`","assignee":"self"}`), ts, agentID, causes, convID, true)
	if err == nil {
		t.Fatal("execTaskAssign on a live-completed task succeeded; want refusal — completed work re-enters circulation only via an explicit reopen")
	}
	if !strings.Contains(err.Error(), "completed") {
		t.Fatalf("refusal error = %q; must name the completed state", err.Error())
	}
}

func TestExecTaskAssignAllowsReopenedTask(t *testing.T) {
	ts, task, agentID, convID, causes := completedImplementationTask(t)

	if err := ts.Reopen(agentID, task.ID, "review found omissions", []string{"missing tier column"}, causes, convID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := execTaskAssign([]byte(`{"task_id":"`+task.ID.Value()+`","assignee":"self"}`), ts, agentID, causes, convID, true); err != nil {
		t.Fatalf("execTaskAssign after explicit reopen: %v; the reopen door must stay open", err)
	}
}

// v14-F2 perception half: agents reasoned over "[created]" for a task whose
// live completion the store knew about — buildTaskContext rendered the v3.9
// lifecycle status, which legacy-flow tasks never advance. The rendered
// status must fold live completion (and an explicit reopen must read open
// again).
func TestBuildTaskContextShowsCompletedStatus(t *testing.T) {
	ts, task, agentID, convID, causes := completedImplementationTask(t)

	provider := newMockProvider("ok")
	agent, _ := agentWithGraph(t, provider)
	l, err := New(Config{Agent: agent, HumanID: humanID(), TaskStore: ts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := l.buildTaskContext()
	if !strings.Contains(ctx, "[completed] "+task.ID.Value()) {
		t.Fatalf("task context must render the live-completed task as [completed]; got:\n%s", ctx)
	}

	if err := ts.Reopen(agentID, task.ID, "review found omissions", nil, causes, convID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	ctx = l.buildTaskContext()
	if strings.Contains(ctx, "[completed] "+task.ID.Value()) {
		t.Fatalf("a reopened task must not render [completed]; got:\n%s", ctx)
	}
}
