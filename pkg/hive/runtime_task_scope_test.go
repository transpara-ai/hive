package hive

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestRuntimeTaskScopeRequiresCurrentConversationAndWorkspace(t *testing.T) {
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	rt, err := New(context.Background(), Config{
		Store:           store.NewInMemoryStore(),
		Actors:          actors,
		HumanID:         humanID,
		RepoPath:        t.TempDir(),
		IsolateRunTasks: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if !rt.isolateRunTasks {
		t.Fatal("IsolateRunTasks was not wired into Runtime")
	}
	bootstrap, err := event.NewBootstrapFactory(rt.graph.Registry()).Init(rt.humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}

	head, err := rt.store.Head()
	if err != nil || head.IsNone() {
		t.Fatalf("head = %v, %v", head, err)
	}
	causes := []types.EventID{head.Unwrap().ID()}
	current, err := rt.tasks.CreateInWorkspace(rt.humanID, "current", "", rt.repoPath, causes, rt.convID)
	if err != nil {
		t.Fatalf("create current: %v", err)
	}
	oldConv := types.MustConversationID("conv_00000000000000000000000000000105")
	old, err := rt.tasks.CreateInWorkspace(rt.humanID, "old", "", rt.repoPath, causes, oldConv)
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	wrongWorkspace, err := rt.tasks.CreateInWorkspace(rt.humanID, "wrong workspace", "", t.TempDir(), causes, rt.convID)
	if err != nil {
		t.Fatalf("create wrong workspace: %v", err)
	}

	if !rt.taskInCurrentRun(current.ID) {
		t.Fatal("current conversation/workspace task rejected")
	}
	if rt.taskInCurrentRun(old.ID) {
		t.Fatal("prior conversation task accepted")
	}
	if rt.taskInCurrentRun(wrongWorkspace.ID) {
		t.Fatal("wrong workspace task accepted")
	}
	currentEvent, err := rt.store.Get(current.ID)
	if err != nil {
		t.Fatalf("get current event: %v", err)
	}
	oldEvent, err := rt.store.Get(old.ID)
	if err != nil {
		t.Fatalf("get old event: %v", err)
	}
	if !rt.eventInCurrentRun(currentEvent) || rt.eventInCurrentRun(oldEvent) {
		t.Fatal("persistent role-event conversation scope does not match current run")
	}
}
