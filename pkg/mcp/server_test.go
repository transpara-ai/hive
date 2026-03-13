package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/statestore"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// testDeps creates a Deps with in-memory stores and a bootstrapped graph.
func testDeps(t *testing.T) Deps {
	t.Helper()
	return testDepsWithMinTrust(t, 0)
}

func testDepsWithMinTrust(t *testing.T, minTrust float64) Deps {
	t.Helper()

	s := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()
	states := statestore.NewInMemoryStateStore()
	trustModel := trust.NewDefaultTrustModel()

	convID, err := types.NewConversationID("conv_test_0000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}

	// Register actors — IDs are derived from public keys.
	// Keys must be exactly 32 bytes for Ed25519 public key validation.
	agentPub, err := types.NewPublicKey([]byte("agent-key-0000000000000000000000"))
	if err != nil {
		t.Fatal(err)
	}
	humanPub, err := types.NewPublicKey([]byte("human-key-0000000000000000000000"))
	if err != nil {
		t.Fatal(err)
	}
	agentActor, err := actors.Register(agentPub, "TestAgent", event.ActorTypeAI)
	if err != nil {
		t.Fatal(err)
	}
	humanActor, err := actors.Register(humanPub, "TestHuman", event.ActorTypeHuman)
	if err != nil {
		t.Fatal(err)
	}
	agentID := agentActor.ID()
	humanID := humanActor.ID()

	// Bootstrap the graph so there's a chain head.
	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)
	signer := &testSigner{}
	bootstrap, err := bsFactory.Init(agentID, signer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatal(err)
	}

	return Deps{
		Store:    s,
		Actors:   actors,
		States:   states,
		Trust:    trustModel,
		AgentID:  agentID,
		HumanID:  humanID,
		ConvID:   convID,
		MinTrust: minTrust,
	}
}

// testSigner is a minimal signer that returns a fixed signature.
type testSigner struct{}

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	// 64-byte Ed25519 signature (zeros are valid structurally).
	return types.NewSignature(make([]byte, 64))
}

func TestHandleInitialize(t *testing.T) {
	s := NewServer("test", "0.0.1")
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
	}
	resp := s.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.ServerInfo.Name != "test" {
		t.Errorf("got name %q, want %q", result.ServerInfo.Name, "test")
	}
}

func TestToolsList(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	}
	resp := s.handleRequest(req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}

	// Verify expected tools are registered.
	names := make(map[string]bool)
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}

	expected := []string{
		"query_events", "get_event", "get_actor", "list_actors",
		"get_trust", "query_self", "query_human", "emit_event",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}

	// Verify file tools are NOT registered (Claude CLI has them natively).
	removed := []string{"read_file", "write_file", "list_files", "run_command"}
	for _, name := range removed {
		if names[name] {
			t.Errorf("unexpected tool still registered: %s", name)
		}
	}
}

func TestQueryEvents(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	result := callTool(t, s, "query_events", map[string]any{"limit": 10})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	// Should have the bootstrap event.
	if result.Content[0].Text == "No events found." {
		t.Error("expected at least the bootstrap event")
	}
}

func TestEmitEvent(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	result := callTool(t, s, "emit_event", map[string]any{
		"event_type": "agent.acted",
		"content": map[string]any{
			"Action": "built MCP server",
			"Target": "hive",
		},
	})
	if result.IsError {
		t.Fatalf("emit_event failed: %s", result.Content[0].Text)
	}

	// Verify events were appended (bootstrap + acted + audit events).
	recent := callTool(t, s, "query_events", map[string]any{"limit": 100})
	if recent.IsError {
		t.Fatalf("query after emit failed: %s", recent.Content[0].Text)
	}

	var events []map[string]any
	if err := json.Unmarshal([]byte(recent.Content[0].Text), &events); err != nil {
		t.Fatalf("parse events: %v", err)
	}
	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}
}

func TestEmitEventRejectsUnknownType(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	result := callTool(t, s, "emit_event", map[string]any{
		"event_type": "bogus.type",
		"content":    map[string]any{},
	})
	if !result.IsError {
		t.Error("expected error for unknown event type")
	}
}

func TestEmitEventInjectsAgentID(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Emit an event — the agent shouldn't need to specify its own ID.
	result := callTool(t, s, "emit_event", map[string]any{
		"event_type": "agent.learned",
		"content": map[string]any{
			"Lesson": "MCP tools are useful",
			"Source": "experience",
		},
	})
	if result.IsError {
		t.Fatalf("emit_event failed: %s", result.Content[0].Text)
	}

	// Verify the source is the agent's ID.
	var ev map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &ev); err != nil {
		t.Fatal(err)
	}
	if ev["source"] != deps.AgentID.Value() {
		t.Errorf("source = %v, want %v", ev["source"], deps.AgentID.Value())
	}
}

func TestEmitEventAuthorityDenied(t *testing.T) {
	// Set minTrust high enough that the agent (trust 0.0) gets denied.
	deps := testDepsWithMinTrust(t, 0.5)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	result := callTool(t, s, "emit_event", map[string]any{
		"event_type": "agent.acted",
		"content": map[string]any{
			"Action": "should be blocked",
			"Target": "test",
		},
	})
	if !result.IsError {
		t.Error("expected authority denied error")
	}
	if result.Content[0].Text == "" {
		t.Error("expected error message")
	}
}

func TestEmitEventDeniedSuspended(t *testing.T) {
	deps := testDepsWithMinTrust(t, 0.01) // low trust gate, but status check comes first
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Suspend the agent.
	bootstrapHead, _ := deps.Store.Head()
	reason := bootstrapHead.Unwrap().ID()
	if _, err := deps.Actors.Suspend(deps.AgentID, reason); err != nil {
		t.Fatal(err)
	}

	result := callTool(t, s, "emit_event", map[string]any{
		"event_type": "agent.acted",
		"content":    map[string]any{"Action": "blocked", "Target": "test"},
	})
	if !result.IsError {
		t.Error("expected authority denied for suspended agent")
	}
	if !strings.Contains(result.Content[0].Text, "suspended") {
		t.Errorf("error should mention suspended status, got: %s", result.Content[0].Text)
	}
}

func TestEmitEventDeniedMemorial(t *testing.T) {
	deps := testDepsWithMinTrust(t, 0.01)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Memorialize the agent.
	bootstrapHead, _ := deps.Store.Head()
	reason := bootstrapHead.Unwrap().ID()
	if _, err := deps.Actors.Memorial(deps.AgentID, reason); err != nil {
		t.Fatal(err)
	}

	result := callTool(t, s, "emit_event", map[string]any{
		"event_type": "agent.acted",
		"content":    map[string]any{"Action": "blocked", "Target": "test"},
	})
	if !result.IsError {
		t.Error("expected authority denied for memorial agent")
	}
	if !strings.Contains(result.Content[0].Text, "memorial") {
		t.Errorf("error should mention memorial status, got: %s", result.Content[0].Text)
	}
}

func TestAuditLogging(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Call a read tool — should generate audit events.
	callTool(t, s, "query_self", map[string]any{})

	// Check that audit events were recorded (agent.acted with mcp.tool_call prefix).
	page, err := deps.Store.ByType(event.EventTypeAgentActed, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range page.Items() {
		content := fmt.Sprintf("%v", ev.Content())
		if strings.Contains(content, "mcp.tool_call:query_self") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected audit event for query_self tool call")
	}
}

func TestContextBuilder(t *testing.T) {
	deps := testDeps(t)
	builder := NewContextBuilder(deps.Store, deps.Actors, deps.Trust)

	ctx := builder.Build(deps.AgentID, deps.HumanID)

	if ctx == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(ctx, "Your Identity") {
		t.Error("missing identity section")
	}
	if !strings.Contains(ctx, "Human Operator") {
		t.Error("missing human section")
	}
	if !strings.Contains(ctx, "Recent Events") {
		t.Error("missing events section")
	}
}

func TestPing(t *testing.T) {
	s := NewServer("test", "0.0.1")
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`99`),
		Method:  "ping",
	}
	resp := s.handleRequest(req)
	if resp == nil || resp.Error != nil {
		t.Fatal("ping should succeed")
	}
}

func TestUnknownMethod(t *testing.T) {
	s := NewServer("test", "0.0.1")
	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`3`),
		Method:  "bogus/method",
	}
	resp := s.handleRequest(req)
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("got error code %d, want -32601", resp.Error.Code)
	}
}

func TestWorkCreateTask(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Missing title → error result (not an RPC error).
	result := callTool(t, s, "work_create_task", map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing title")
	}

	// Valid title → returns id and title.
	result = callTool(t, s, "work_create_task", map[string]any{
		"title":       "Fix the auth bug",
		"description": "login fails on mobile",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	var task map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &task); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if _, ok := task["id"]; !ok {
		t.Error("missing id in result")
	}
	if task["title"] != "Fix the auth bug" {
		t.Errorf("title = %v, want %q", task["title"], "Fix the auth bug")
	}
}

func TestWorkListTasks(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Empty graph → empty array.
	result := callTool(t, s, "work_list_tasks", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	var tasks []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &tasks); err != nil {
		t.Fatalf("parse empty response: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	// Create a task and list again.
	callTool(t, s, "work_create_task", map[string]any{"title": "List Me"})

	result = callTool(t, s, "work_list_tasks", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &tasks); err != nil {
		t.Fatalf("parse populated response: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["title"] != "List Me" {
		t.Errorf("title = %v, want %q", tasks[0]["title"], "List Me")
	}
}

func TestWorkAssignTask(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// Create a task to assign.
	result := callTool(t, s, "work_create_task", map[string]any{"title": "Assign Me"})
	if result.IsError {
		t.Fatalf("create task failed: %s", result.Content[0].Text)
	}
	var task map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &task); err != nil {
		t.Fatal(err)
	}
	taskID := task["id"].(string)

	// No assignee specified → defaults to calling agent.
	result = callTool(t, s, "work_assign_task", map[string]any{"task_id": taskID})
	if result.IsError {
		t.Fatalf("self-assign failed: %s", result.Content[0].Text)
	}
	var assignment map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &assignment); err != nil {
		t.Fatal(err)
	}
	if assignment["assignee"] != deps.AgentID.Value() {
		t.Errorf("self-assign: assignee = %v, want %v", assignment["assignee"], deps.AgentID.Value())
	}

	// Explicit assignee → uses the provided actor ID.
	result = callTool(t, s, "work_assign_task", map[string]any{
		"task_id":  taskID,
		"assignee": deps.HumanID.Value(),
	})
	if result.IsError {
		t.Fatalf("explicit assign failed: %s", result.Content[0].Text)
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &assignment); err != nil {
		t.Fatal(err)
	}
	if assignment["assignee"] != deps.HumanID.Value() {
		t.Errorf("explicit assign: assignee = %v, want %v", assignment["assignee"], deps.HumanID.Value())
	}
}

func TestWorkCompleteTask(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	createTask := func(title string) string {
		t.Helper()
		r := callTool(t, s, "work_create_task", map[string]any{"title": title})
		if r.IsError {
			t.Fatalf("create task %q failed: %s", title, r.Content[0].Text)
		}
		var task map[string]any
		if err := json.Unmarshal([]byte(r.Content[0].Text), &task); err != nil {
			t.Fatal(err)
		}
		return task["id"].(string)
	}

	// Complete without summary.
	taskID := createTask("Complete Me")
	result := callTool(t, s, "work_complete_task", map[string]any{"task_id": taskID})
	if result.IsError {
		t.Fatalf("complete without summary failed: %s", result.Content[0].Text)
	}
	var completion map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &completion); err != nil {
		t.Fatal(err)
	}
	if completion["task_id"] != taskID {
		t.Errorf("task_id = %v, want %v", completion["task_id"], taskID)
	}
	if completion["completed_by"] != deps.AgentID.Value() {
		t.Errorf("completed_by = %v, want %v", completion["completed_by"], deps.AgentID.Value())
	}

	// Complete with summary.
	taskID2 := createTask("Complete With Summary")
	result = callTool(t, s, "work_complete_task", map[string]any{
		"task_id": taskID2,
		"summary": "shipped in PR #42",
	})
	if result.IsError {
		t.Fatalf("complete with summary failed: %s", result.Content[0].Text)
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &completion); err != nil {
		t.Fatal(err)
	}
	if completion["summary"] != "shipped in PR #42" {
		t.Errorf("summary = %v, want %q", completion["summary"], "shipped in PR #42")
	}
}

func TestWorkQueryTasks(t *testing.T) {
	deps := testDeps(t)
	s := NewServer("test", "0.0.1")
	RegisterAllTools(s, deps)

	// No tasks → empty array.
	result := callTool(t, s, "work_query_tasks", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	var tasks []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &tasks); err != nil {
		t.Fatalf("parse empty response: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	// Create two tasks.
	createAndGetID := func(title string) string {
		t.Helper()
		r := callTool(t, s, "work_create_task", map[string]any{"title": title})
		if r.IsError {
			t.Fatalf("create %q failed: %s", title, r.Content[0].Text)
		}
		var task map[string]any
		if err := json.Unmarshal([]byte(r.Content[0].Text), &task); err != nil {
			t.Fatal(err)
		}
		return task["id"].(string)
	}
	taskAID := createAndGetID("Task A")
	createAndGetID("Task B")

	// No filter → all tasks returned.
	result = callTool(t, s, "work_query_tasks", map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &tasks); err != nil {
		t.Fatalf("parse all-tasks response: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	// Assign Task A to the human operator.
	r := callTool(t, s, "work_assign_task", map[string]any{
		"task_id":  taskAID,
		"assignee": deps.HumanID.Value(),
	})
	if r.IsError {
		t.Fatalf("assign failed: %s", r.Content[0].Text)
	}

	// Assignee filter → only Task A matches.
	result = callTool(t, s, "work_query_tasks", map[string]any{
		"assignee": deps.HumanID.Value(),
	})
	if result.IsError {
		t.Fatalf("assignee filter failed: %s", result.Content[0].Text)
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &tasks); err != nil {
		t.Fatalf("parse filtered response: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task for human assignee, got %d", len(tasks))
	}
	if tasks[0]["title"] != "Task A" {
		t.Errorf("title = %v, want %q", tasks[0]["title"], "Task A")
	}
}

// callTool invokes a tool on the server and returns the result.
func callTool(t *testing.T, s *Server, name string, args map[string]any) ToolCallResult {
	t.Helper()

	params := ToolCallParams{Name: name, Arguments: args}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	req := Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	resp := s.handleRequest(req)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("RPC error: %v", resp.Error)
	}

	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

