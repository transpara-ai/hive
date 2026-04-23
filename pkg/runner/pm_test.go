package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/api"
)

// trackingOperator records whether Operate was called.
type trackingOperator struct {
	called atomic.Bool
}

func (t *trackingOperator) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	score, _ := types.NewScore(0.9)
	return decision.NewResponse("", score, decision.TokenUsage{}), nil
}
func (t *trackingOperator) Name() string  { return "tracking-operator" }
func (t *trackingOperator) Model() string { return "mock-model" }
func (t *trackingOperator) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	t.called.Store(true)
	return decision.OperateResult{Summary: "Board has work. No action needed."}, nil
}

func boardServer(nodes []api.Node) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := api.BoardResponse{Nodes: nodes}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestRunPM_SkipsLLM_NoPinnedGoal(t *testing.T) {
	// Board has items but none are pinned — PM should skip the LLM call.
	srv := boardServer([]api.Node{
		{ID: "doc-1", Kind: "document", State: "open", Title: "Build report"},
		{ID: "claim-1", Kind: "claim", State: "open", Title: "Lesson 1"},
	})
	defer srv.Close()

	op := &trackingOperator{}
	r := New(Config{
		Role:      "pm",
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		APIBase:   srv.URL,
		Provider:  op,
		OneShot:   true,
		HiveDir:   t.TempDir(),
	})

	r.runPM(context.Background())

	if op.called.Load() {
		t.Error("runPM called Operate when no pinned goal exists — should have short-circuited")
	}
}

func TestRunPM_SkipsLLM_OpenTasksExist(t *testing.T) {
	// Board has a pinned goal AND open work tasks — PM should skip (work already exists).
	srv := boardServer([]api.Node{
		{ID: "goal-1", Kind: "task", State: "open", Pinned: true, Title: "Priority Queue", Priority: "high"},
		{ID: "task-1", Kind: "task", State: "open", Title: "Build feature X", Priority: "high"},
	})
	defer srv.Close()

	op := &trackingOperator{}
	r := New(Config{
		Role:      "pm",
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		APIBase:   srv.URL,
		Provider:  op,
		OneShot:   true,
		HiveDir:   t.TempDir(),
	})

	r.runPM(context.Background())

	if op.called.Load() {
		t.Error("runPM called Operate when open tasks exist — should have short-circuited")
	}
}

func TestRunPM_CallsLLM_PinnedGoalNoOpenTasks(t *testing.T) {
	// Board has a pinned goal but no open work tasks — PM should call the LLM.
	srv := boardServer([]api.Node{
		{ID: "goal-1", Kind: "task", State: "open", Pinned: true, Title: "Priority Queue", Priority: "high"},
		{ID: "doc-1", Kind: "document", State: "open", Title: "Build report"},
	})
	defer srv.Close()

	op := &trackingOperator{}
	r := New(Config{
		Role:      "pm",
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		APIBase:   srv.URL,
		Provider:  op,
		OneShot:   true,
		HiveDir:   t.TempDir(),
	})

	r.runPM(context.Background())

	if !op.called.Load() {
		t.Error("runPM did NOT call Operate when pinned goal exists and no open tasks — should have called LLM")
	}
}

func TestRunPM_SkipsLLM_DoneTasksIgnored(t *testing.T) {
	// Board has a pinned goal and only done/closed tasks — PM should call the LLM
	// (done tasks don't count as open work).
	srv := boardServer([]api.Node{
		{ID: "goal-1", Kind: "task", State: "open", Pinned: true, Title: "Priority Queue", Priority: "high"},
		{ID: "task-1", Kind: "task", State: "done", Title: "Completed feature", Priority: "high"},
		{ID: "task-2", Kind: "task", State: "closed", Title: "Rejected task", Priority: "medium"},
	})
	defer srv.Close()

	op := &trackingOperator{}
	r := New(Config{
		Role:      "pm",
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		APIBase:   srv.URL,
		Provider:  op,
		OneShot:   true,
		HiveDir:   t.TempDir(),
	})

	r.runPM(context.Background())

	if !op.called.Load() {
		t.Error("runPM did NOT call Operate when only done/closed tasks exist — should treat board as needing work")
	}
}
