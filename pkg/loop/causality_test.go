// causality_test.go pins Invariant 2 (CAUSALITY: every event has declared causes)
// to CI. Three node-creation code paths are exercised; each must produce an event
// or HTTP request with len(causes) > 0.
// Additional edge-case tests follow the three path tests.
package loop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/api"
	"github.com/transpara-ai/work"
)

// agentWithGraph creates an agent and returns it with its graph, so tests can
// share the same store for both the agent and work.TaskStore. The EventGraph
// enforces causal links — cause event IDs must exist in the same store.
func agentWithGraph(t *testing.T, provider intelligence.Provider) (*hiveagent.Agent, *graph.Graph) {
	t.Helper()
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })

	// Register work event types before any event creation.
	work.RegisterWithRegistry(g.Registry())

	a, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("strategist"),
		Name:     "causality-test-agent",
		Graph:    g,
		Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, g
}

// TestCausality_LoopTaskCommandPath verifies that when the loop processes a
// /task create command, the resulting task event in the work store has at least
// one cause. The cause is agent.LastEvent() — set during agent boot.
func TestCausality_LoopTaskCommandPath(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent, g := agentWithGraph(t, provider)

	if agent.LastEvent().IsZero() {
		t.Fatal("agent.LastEvent() zero after boot — boot events must set lastEvent")
	}

	// TaskStore shares the agent's store so cause event IDs resolve.
	// Build the factory from the graph's registry (work types already registered).
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})

	convID := types.MustConversationID("conv_00000000000000000000000000000042")
	l := &Loop{
		agent:  agent,
		config: Config{TaskStore: ts, ConvID: convID},
	}

	l.processTaskCommands(`/task create {"title": "observe gap", "description": "handler missing"}`)

	tasks, err := ts.List(5)
	if err != nil || len(tasks) == 0 {
		t.Fatalf("expected 1 task, got err=%v tasks=%d", err, len(tasks))
	}
	ev, err := g.Store().Get(tasks[0].ID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if len(ev.Causes()) == 0 {
		t.Error("CAUSALITY violated: loop task command created event with no causes")
	}
}

// TestCausality_DirectAPICallPath verifies that api.Client.CreateTask sends
// causes in the HTTP request body — the direct API call code path used by
// agents creating tasks programmatically.
func TestCausality_DirectAPICallPath(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"n1","kind":"task","title":"t","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	c := api.New(srv.URL, "test-key")
	_, err := c.CreateTask("hive", "Fix: bug", "details", "high", []string{"doc-node-123"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	causeList, ok := fields["causes"].([]any)
	if !ok || len(causeList) == 0 {
		t.Errorf("CAUSALITY violated: CreateTask request missing causes, body=%s", body)
	}
}

// TestCausality_LoopTaskCommandPath_MultipleTasks verifies that when the response
// contains several /task create commands, EVERY resulting task event has causes —
// not just the first one. Regression guard: causes is passed once to
// executeTaskCommands; a refactor that computes it per-command must not lose it.
func TestCausality_LoopTaskCommandPath_MultipleTasks(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent, g := agentWithGraph(t, provider)

	if agent.LastEvent().IsZero() {
		t.Fatal("agent.LastEvent() zero after boot")
	}

	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})

	convID := types.MustConversationID("conv_00000000000000000000000000000043")
	l := &Loop{
		agent:  agent,
		config: Config{TaskStore: ts, ConvID: convID},
	}

	response := `/task create {"title": "first task", "description": "do this"}
/task create {"title": "second task", "description": "then this"}
/task create {"title": "third task", "description": "finally this"}`
	l.processTaskCommands(response)

	tasks, err := ts.List(10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		ev, err := g.Store().Get(task.ID)
		if err != nil {
			t.Fatalf("Store.Get(%v): %v", task.ID, err)
		}
		if len(ev.Causes()) == 0 {
			t.Errorf("CAUSALITY violated: task %q created with no causes", task.Title)
		}
	}
}

// TestCausality_CmdPostPath verifies that api.Client.AssertClaim (used by cmd/post
// to record scout gaps and critiques) sends causes in the HTTP request body.
// cmd/post links every claim back to the build task that generated it.
func TestCausality_CmdPostPath(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"op":"assert","node":{"id":"c1","kind":"claim","title":"t","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	c := api.New(srv.URL, "test-key")
	_, err := c.AssertClaim("hive", "Lesson: causality must hold", "every node cites its cause", []string{"task-node-456"})
	if err != nil {
		t.Fatalf("AssertClaim: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	causeList, ok := fields["causes"].([]any)
	if !ok || len(causeList) == 0 {
		t.Errorf("CAUSALITY violated: AssertClaim request missing causes, body=%s", body)
	}
}
