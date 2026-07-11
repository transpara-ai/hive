package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

func TestOneShotTaskScopeFiltersPerceptionAndAutoAssignment(t *testing.T) {
	ts, causes := newTaskCommandStore(t)
	actorID := humanID()
	oldConv := types.MustConversationID("conv_00000000000000000000000000000101")
	currentConv := types.MustConversationID("conv_00000000000000000000000000000102")
	workspace := t.TempDir()

	oldTask, err := ts.CreateInWorkspace(actorID, "Old run task", "must never appear", workspace, causes, oldConv)
	if err != nil {
		t.Fatalf("create old task: %v", err)
	}
	currentTask, err := ts.CreateInWorkspace(actorID, "Current run task", "current evidence", workspace, causes, currentConv)
	if err != nil {
		t.Fatalf("create current task: %v", err)
	}
	for _, id := range []types.EventID{oldTask.ID, currentTask.ID} {
		for _, label := range []string{work.GateDefinitionOfDone, work.GateAcceptanceCriteria, work.GateTestPlan} {
			if err := ts.AddArtifact(actorID, id, label, "text/markdown", "required", causes, currentConv); err != nil {
				t.Fatalf("add %s to %s: %v", label, id.Value(), err)
			}
		}
	}

	l := &Loop{config: Config{
		TaskStore: ts,
		TaskScope: func(id types.EventID) bool { return id == currentTask.ID },
	}}
	ctx := l.buildTaskContext()
	if strings.Contains(ctx, oldTask.ID.Value()) || strings.Contains(ctx, oldTask.Title) {
		t.Fatalf("task context leaked old run task:\n%s", ctx)
	}
	if !strings.Contains(ctx, currentTask.ID.Value()) || !strings.Contains(ctx, currentTask.Title) {
		t.Fatalf("task context omitted current run task:\n%s", ctx)
	}
	got, ok := l.firstAssignableOpenTask()
	if !ok || got.ID != currentTask.ID {
		t.Fatalf("first assignable = %s, %v; want current task %s", got.ID.Value(), ok, currentTask.ID.Value())
	}
}

func TestOneShotTaskScopeRejectsOldCommandAndStampsDerivedWorkspace(t *testing.T) {
	ts, causes := newTaskCommandStore(t)
	actorID := humanID()
	oldConv := types.MustConversationID("conv_00000000000000000000000000000103")
	currentConv := types.MustConversationID("conv_00000000000000000000000000000104")
	workspace := t.TempDir()
	oldTask, err := ts.CreateInWorkspace(actorID, "Old run task", "out of scope", workspace, causes, oldConv)
	if err != nil {
		t.Fatalf("create old task: %v", err)
	}
	scope := func(id types.EventID) bool { return id != oldTask.ID }

	assign := TaskCommand{Action: "assign", Payload: []byte(`{"task_id":"` + oldTask.ID.Value() + `","assignee":"self"}`)}
	if got := executeTaskCommandsScoped([]TaskCommand{assign}, ts, actorID, causes, currentConv, true, scope, workspace); got != 0 {
		t.Fatalf("executed out-of-scope assignment = %d, want 0", got)
	}

	create := TaskCommand{Action: "create", Payload: []byte(`{"title":"Derived current task","description":"bounded","priority":"high"}`)}
	if got := executeTaskCommandsScoped([]TaskCommand{create}, ts, actorID, causes, currentConv, true, scope, workspace); got != 1 {
		t.Fatalf("executed scoped create = %d, want 1", got)
	}
	tasks, err := ts.ListByWorkspace(workspace, 20)
	if err != nil {
		t.Fatalf("list workspace: %v", err)
	}
	found := false
	for _, task := range tasks {
		if task.Title == "Derived current task" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("derived task missing workspace %q: %+v", workspace, tasks)
	}
}

func TestOneShotTaskScopeRejectsReviewEmission(t *testing.T) {
	taskID := types.MustEventID("019f51b4-1d9e-7852-8eb4-7fff8008b644")
	l := &Loop{config: Config{
		TaskScope: func(types.EventID) bool { return false },
	}}
	err := l.emitCodeReview(&ReviewCommand{
		TaskID:     taskID.Value(),
		Verdict:    "approve",
		Summary:    "stale task",
		Issues:     []string{},
		Confidence: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "outside the current run boundary") {
		t.Fatalf("emitCodeReview error = %v, want run-boundary rejection", err)
	}
}
