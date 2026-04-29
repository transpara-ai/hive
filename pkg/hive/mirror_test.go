package hive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/api"
	"github.com/transpara-ai/hive/pkg/runner"
	"github.com/transpara-ai/work"
)

func TestMirrorTaskCompletionPostsSiteNodeAndRecordsMirror(t *testing.T) {
	ctx := context.Background()
	var got siteTaskMirrorPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hive/mirror" {
			t.Fatalf("path = %s, want /api/hive/mirror", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode mirror payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := testRuntime(t, api.New(srv.URL, "test-key"))
	op := runner.OpEvent{
		ID:        "site-op-1",
		SpaceID:   "journey-test",
		NodeID:    "site-node-1",
		NodeTitle: "Mirror me",
		Actor:     "operator",
		ActorID:   "operator-1",
		ActorKind: "human",
		Op:        "intend",
		Payload:   json.RawMessage(`{"description":"do mirrored work"}`),
		CreatedAt: time.Now().UTC(),
	}
	anchorID, err := rt.AnchorSiteOp(ctx, op)
	if err != nil {
		t.Fatalf("AnchorSiteOp: %v", err)
	}
	if err := rt.EmitSiteOp(ctx, op, anchorID); err != nil {
		t.Fatalf("EmitSiteOp: %v", err)
	}
	taskID := translatedTaskID(t, rt)
	if err := rt.tasks.WaiveArtifact(rt.humanID, taskID, "test completion", []types.EventID{taskID}, rt.convID); err != nil {
		t.Fatalf("WaiveArtifact: %v", err)
	}
	if err := rt.tasks.Complete(rt.humanID, taskID, "completed from test", []types.EventID{taskID}, rt.convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	completedID, ok, err := rt.findCompletedEventForTask(taskID)
	if err != nil {
		t.Fatalf("findCompletedEventForTask: %v", err)
	}
	if !ok {
		t.Fatal("completion event not found")
	}

	rt.mirrorTaskCompletion(ctx, work.Task{ID: taskID, Title: "Mirror me"}, "completed from test")

	if got.NodeID != "site-node-1" {
		t.Errorf("NodeID = %q, want site-node-1", got.NodeID)
	}
	if got.HiveTaskID != taskID.Value() {
		t.Errorf("HiveTaskID = %q, want %q", got.HiveTaskID, taskID.Value())
	}
	if got.HiveChainRef != completedID.Value() {
		t.Errorf("HiveChainRef = %q, want %q", got.HiveChainRef, completedID.Value())
	}
	if got.EventType != work.EventTypeTaskCompleted.Value() {
		t.Errorf("EventType = %q, want %q", got.EventType, work.EventTypeTaskCompleted.Value())
	}
	if got.State != "done" {
		t.Errorf("State = %q, want done", got.State)
	}

	page, err := rt.store.ByType(event.EventTypeSiteOpMirrored, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(site.op.mirrored): %v", err)
	}
	if len(page.Items()) != 1 {
		t.Fatalf("mirrored events = %d, want 1", len(page.Items()))
	}
	content := page.Items()[0].Content().(event.SiteOpMirroredContent)
	if content.ExternalRef.ID != "site-op-1" {
		t.Errorf("ExternalRef.ID = %q, want site-op-1", content.ExternalRef.ID)
	}
	if content.MirrorEventID != completedID {
		t.Errorf("MirrorEventID = %s, want %s", content.MirrorEventID.Value(), completedID.Value())
	}
}

func TestFindSiteMirrorTargetUsesAncestorTask(t *testing.T) {
	ctx := context.Background()
	rt := testRuntime(t, nil)
	op := runner.OpEvent{
		ID:        "site-op-parent",
		SpaceID:   "journey-test",
		NodeID:    "site-node-parent",
		NodeTitle: "Parent",
		Actor:     "operator",
		ActorID:   "operator-1",
		ActorKind: "human",
		Op:        "intend",
		Payload:   json.RawMessage(`{"description":"parent"}`),
		CreatedAt: time.Now().UTC(),
	}
	anchorID, err := rt.AnchorSiteOp(ctx, op)
	if err != nil {
		t.Fatalf("AnchorSiteOp: %v", err)
	}
	if err := rt.EmitSiteOp(ctx, op, anchorID); err != nil {
		t.Fatalf("EmitSiteOp: %v", err)
	}
	parentID := translatedTaskID(t, rt)
	child, err := rt.tasks.Create(rt.humanID, "Child", "child", []types.EventID{parentID}, rt.convID)
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}
	if err := rt.tasks.AddDependency(rt.humanID, child.ID, parentID, []types.EventID{child.ID}, rt.convID); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	target, ok, err := rt.findSiteMirrorTarget(child.ID)
	if err != nil {
		t.Fatalf("findSiteMirrorTarget: %v", err)
	}
	if !ok {
		t.Fatal("ancestor target not found")
	}
	if target.nodeID != "site-node-parent" {
		t.Errorf("nodeID = %q, want site-node-parent", target.nodeID)
	}
	if target.siteOpID != "site-op-parent" {
		t.Errorf("siteOpID = %q, want site-op-parent", target.siteOpID)
	}
}

func testRuntime(t *testing.T, client *api.Client) *Runtime {
	t.Helper()
	s := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()
	pk := make([]byte, 32)
	pk[0] = 1
	human, err := actors.Register(types.MustPublicKey(pk), "Human", event.ActorTypeHuman)
	if err != nil {
		t.Fatalf("register human: %v", err)
	}
	rt, err := New(context.Background(), Config{
		Store:     s,
		Actors:    actors,
		HumanID:   human.ID(),
		APIClient: client,
	})
	if err != nil {
		t.Fatalf("New runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	return rt
}

func translatedTaskID(t *testing.T, rt *Runtime) types.EventID {
	t.Helper()
	page, err := rt.store.ByType(event.EventTypeSiteOpTranslated, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(site.op.translated): %v", err)
	}
	items := page.Items()
	if len(items) == 0 {
		t.Fatal("no translated event found")
	}
	content := items[len(items)-1].Content().(event.SiteOpTranslatedContent)
	return content.BusEventID
}
