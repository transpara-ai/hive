package loop

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
)

// ════════════════════════════════════════════════════════════════════════
// Mock provider — returns canned responses in sequence
// ════════════════════════════════════════════════════════════════════════

type mockProvider struct {
	responses []string
	callCount atomic.Int32
}

func newMockProvider(responses ...string) *mockProvider {
	return &mockProvider{responses: responses}
}

func (m *mockProvider) Name() string  { return "mock" }
func (m *mockProvider) Model() string { return "mock-model" }

func (m *mockProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	idx := int(m.callCount.Add(1)) - 1
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	confidence, _ := types.NewScore(0.8)
	return decision.NewResponse(m.responses[idx], confidence, 50), nil
}

var _ intelligence.Provider = (*mockProvider)(nil)

// ════════════════════════════════════════════════════════════════════════
// Test helpers
// ════════════════════════════════════════════════════════════════════════

func testAgent(t *testing.T, provider intelligence.Provider) *roles.Agent {
	t.Helper()
	agentID := types.MustActorID("actor_00000000000000000000000000000001")
	humanID := types.MustActorID("actor_00000000000000000000000000000099")

	rt, err := intelligence.NewRuntime(context.Background(), intelligence.RuntimeConfig{
		AgentID:  agentID,
		Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = rt.Boot("ai", "mock-model", "standard", []string{"test"}, types.MustDomainScope("test"), humanID)
	if err != nil {
		t.Fatal(err)
	}

	return &roles.Agent{
		Runtime: rt,
		Role:    roles.RoleBuilder,
		Name:    "test-builder",
	}
}

func humanID() types.ActorID {
	return types.MustActorID("actor_00000000000000000000000000000099")
}

// ════════════════════════════════════════════════════════════════════════
// Tests
// ════════════════════════════════════════════════════════════════════════

func TestLoopTaskDone(t *testing.T) {
	// Agent immediately says TASK_DONE.
	provider := newMockProvider("I've completed the work. TASK_DONE")
	agent := testAgent(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopTaskDone {
		t.Errorf("reason = %s, want %s", result.Reason, StopTaskDone)
	}
	if result.Iterations != 1 {
		t.Errorf("iterations = %d, want 1", result.Iterations)
	}
}

func TestLoopEscalation(t *testing.T) {
	provider := newMockProvider("I need human approval. ESCALATE: this requires authority")
	agent := testAgent(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopEscalation {
		t.Errorf("reason = %s, want %s", result.Reason, StopEscalation)
	}
}

func TestLoopHalt(t *testing.T) {
	provider := newMockProvider("HALT: integrity violation detected")
	agent := testAgent(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopHalt {
		t.Errorf("reason = %s, want %s", result.Reason, StopHalt)
	}
}

func TestLoopBudgetExceeded(t *testing.T) {
	// Agent never finishes — budget stops it.
	provider := newMockProvider("working on it...", "still working...", "more work...")
	agent := testAgent(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopBudget {
		t.Errorf("reason = %s, want %s", result.Reason, StopBudget)
	}
	if result.Iterations != 2 {
		t.Errorf("iterations = %d, want 2", result.Iterations)
	}
}

func TestLoopContextCancelled(t *testing.T) {
	provider := newMockProvider("working...")
	agent := testAgent(t, provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 100},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(ctx)
	if result.Reason != StopCancelled {
		t.Errorf("reason = %s, want %s", result.Reason, StopCancelled)
	}
}

func TestLoopQuiescence(t *testing.T) {
	// Agent says IDLE twice — triggers quiescence without bus.
	provider := newMockProvider("IDLE", "IDLE")
	agent := testAgent(t, provider)

	l, err := New(Config{
		Agent:             agent,
		HumanID:           humanID(),
		Budget:            resources.BudgetConfig{MaxIterations: 10},
		QuiescenceDelay:   10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopQuiescence {
		t.Errorf("reason = %s, want %s", result.Reason, StopQuiescence)
	}
}

func TestLoopMultiIteration(t *testing.T) {
	// Agent works for 3 iterations then says done.
	provider := newMockProvider(
		"Writing the code...",
		"Running tests...",
		"All tests pass. TASK_DONE",
	)
	agent := testAgent(t, provider)

	var iterations []int
	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
		OnIteration: func(i int, _ string) {
			iterations = append(iterations, i)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Reason != StopTaskDone {
		t.Errorf("reason = %s, want %s", result.Reason, StopTaskDone)
	}
	if result.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", result.Iterations)
	}
	if len(iterations) != 3 {
		t.Errorf("callback count = %d, want 3", len(iterations))
	}
}

func TestLoopBudgetTracking(t *testing.T) {
	provider := newMockProvider("doing work", "TASK_DONE")
	agent := testAgent(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxTokens: 10000},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.Run(context.Background())
	if result.Budget.TokensUsed == 0 {
		t.Error("expected non-zero token usage")
	}
	if result.Budget.Iterations != 2 {
		t.Errorf("budget iterations = %d, want 2", result.Budget.Iterations)
	}
}

func TestLoopWithBus(t *testing.T) {
	// Agent says IDLE, then bus delivers an event, agent wakes and completes.
	provider := newMockProvider("IDLE", "Got new event! TASK_DONE")
	agent := testAgent(t, provider)

	s := store.NewInMemoryStore()
	eventBus := bus.NewEventBus(s, 16)
	defer eventBus.Close()

	// Channel-based synchronisation — no time.Sleep.
	firstIterDone := make(chan struct{}, 1)

	l, err := New(Config{
		Agent:           agent,
		HumanID:         humanID(),
		Budget:          resources.BudgetConfig{MaxIterations: 10},
		Bus:             eventBus,
		QuiescenceDelay: 200 * time.Millisecond,
		OnIteration: func(i int, _ string) {
			if i == 1 {
				select {
				case firstIterDone <- struct{}{}:
				default:
				}
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan Result, 1)
	go func() {
		done <- l.Run(context.Background())
	}()

	// Wait for first iteration to complete (agent said IDLE), then publish event.
	<-firstIterDone
	otherAgent := types.MustActorID("actor_00000000000000000000000000000002")
	mockEv := createMockEvent(t, s, otherAgent)
	eventBus.Publish(mockEv)

	result := <-done
	if result.Reason != StopTaskDone {
		t.Errorf("reason = %s, want %s (detail: %s)", result.Reason, StopTaskDone, result.Detail)
	}
}

func TestRunConcurrent(t *testing.T) {
	provider1 := newMockProvider("TASK_DONE")
	provider2 := newMockProvider("TASK_DONE")

	agent1 := testAgentWithRole(t, provider1, roles.RoleBuilder, "builder")
	agent2 := testAgentWithRole(t, provider2, roles.RoleTester, "tester")

	results := RunConcurrent(context.Background(), []Config{
		{Agent: agent1, HumanID: humanID(), Budget: resources.BudgetConfig{MaxIterations: 5}},
		{Agent: agent2, HumanID: humanID(), Budget: resources.BudgetConfig{MaxIterations: 5}},
	})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	for _, ar := range results {
		if ar.Result.Reason != StopTaskDone {
			t.Errorf("%s (%s): reason = %s, want %s", ar.Role, ar.Name, ar.Result.Reason, StopTaskDone)
		}
	}
}

func TestNewLoopRequiresAgent(t *testing.T) {
	_, err := New(Config{Budget: resources.BudgetConfig{MaxIterations: 1}})
	if err == nil {
		t.Fatal("expected error for nil agent")
	}
}

// ════════════════════════════════════════════════════════════════════════
// Additional helpers
// ════════════════════════════════════════════════════════════════════════

// agentCounter generates unique actor IDs for test agents.
var agentCounter uint32

func testAgentWithRole(t *testing.T, provider intelligence.Provider, role roles.Role, name string) *roles.Agent {
	t.Helper()
	n := atomic.AddUint32(&agentCounter, 1)
	agentID := types.MustActorID(fmt.Sprintf("actor_000000000000000000000000000000%02d", n))
	humanID := types.MustActorID("actor_00000000000000000000000000000099")

	rt, err := intelligence.NewRuntime(context.Background(), intelligence.RuntimeConfig{
		AgentID:  agentID,
		Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.Boot("ai", "mock-model", "standard", []string{"test"}, types.MustDomainScope("test"), humanID)
	if err != nil {
		t.Fatal(err)
	}

	return &roles.Agent{
		Runtime: rt,
		Role:    role,
		Name:    name,
	}
}

// createMockEvent creates a minimal event for bus testing.
func createMockEvent(t *testing.T, _ store.Store, source types.ActorID) event.Event {
	t.Helper()
	// Create a lightweight event using the event builder pattern.
	// For bus testing, we need a valid event that passes pattern matching.
	registry := event.DefaultRegistry()
	factory := event.NewEventFactory(registry)
	memStore := store.NewInMemoryStore()

	// Bootstrap the temp store.
	bsFactory := event.NewBootstrapFactory(registry)
	signer := &testSigner{}
	bootstrap, err := bsFactory.Init(source, signer)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := memStore.Append(bootstrap)
	if err != nil {
		t.Fatal(err)
	}

	convID, _ := types.NewConversationID("conv_test_0000000000000000000000001")
	ev, err := factory.Create(
		event.EventTypeAgentActed,
		source,
		event.AgentActedContent{AgentID: source, Action: "test_event", Target: "loop"},
		[]types.EventID{stored.ID()},
		convID,
		memStore,
		signer,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

type testSigner struct{}

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	return types.NewSignature(make([]byte, 64))
}
