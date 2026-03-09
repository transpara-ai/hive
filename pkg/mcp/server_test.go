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
	agentPub, _ := types.NewPublicKey([]byte("agent-key-00000000000000000000000"))
	humanPub, _ := types.NewPublicKey([]byte("human-key-00000000000000000000000"))
	agentActor, err := actors.Register(agentPub, "TestAgent", "AI")
	if err != nil {
		t.Fatal(err)
	}
	humanActor, err := actors.Register(humanPub, "TestHuman", "Human")
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

