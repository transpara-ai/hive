package loop

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// isWakeWorthy must suppress high-volume per-iteration agent lifecycle/telemetry
// events (the wakeup storm — 907 agent.state.changed + 443 agent.evaluated on
// the 2026-06-06 run) while still waking on substantive work. Fail-safe: any
// unknown/future event type wakes, so no real work is ever missed.
func TestIsWakeWorthy(t *testing.T) {
	// Only pure per-iteration lifecycle/telemetry events are suppressed.
	noise := []types.EventType{
		event.EventTypeAgentStateChanged,
		event.EventTypeAgentEvaluated,
		event.EventTypeAgentObserved,
	}
	for _, et := range noise {
		if isWakeWorthy(et) {
			t.Errorf("isWakeWorthy(%s) = true, want false (lifecycle noise must not wake)", et.Value())
		}
	}
	substantive := []types.EventType{
		work.EventTypeTaskCreated,
		work.EventTypeTaskCompleted,
		// agent.acted marks SIGNIFICANT actions (Agent.Act: "write_code",
		// "integrate", ...), not pure churn — it must still wake.
		event.EventTypeAgentActed,
		types.MustEventType("hive.gap.detected"),
		types.MustEventType("hive.role.proposed"),
		types.MustEventType("authority.request.recorded"),
		types.MustEventType("some.unknown.future.event"),
	}
	for _, et := range substantive {
		if !isWakeWorthy(et) {
			t.Errorf("isWakeWorthy(%s) = false, want true (substantive work must wake)", et.Value())
		}
	}
}

// onEvent must DROP lifecycle/telemetry noise entirely — neither waking the
// agent nor appending it to pendingEvents. Otherwise a sleeping keepalive agent
// accumulates the whole churn storm in memory and dumps it into the observation
// context on the next substantive wake. Substantive events are appended + wake.
func TestOnEvent_NoiseDroppedAndDoesNotWake(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	l, err := New(Config{Agent: agent, HumanID: humanID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	factory := event.NewEventFactory(g.Registry())
	signer := &testSigner{}
	conv := types.MustConversationID("conv_wake_test")
	cause := agent.LastEvent()
	if cause.IsZero() {
		t.Fatal("agent has no bootstrap event to use as a cause")
	}
	otherActor := types.MustActorID("actor_00000000000000000000000000000042")

	noiseEv, err := factory.Create(event.EventTypeAgentStateChanged, otherActor,
		event.AgentStateChangedContent{AgentID: otherActor, Previous: "idle", Current: "processing"},
		[]types.EventID{cause}, conv, g.Store(), signer)
	if err != nil {
		t.Fatalf("create noise event: %v", err)
	}

	ts := work.NewTaskStore(g.Store(), factory, signer)
	task, err := ts.Create(otherActor, "Some real work", "desc", []types.EventID{cause}, conv)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	substantiveEv := eventByID(t, g, task.ID)

	drainWake(l)

	before := pendingLen(l)
	l.onEvent(noiseEv)
	if pendingLen(l) != before {
		t.Fatal("noise event was appended to pendingEvents — it must be dropped to avoid unbounded accumulation while sleeping")
	}
	if wakeSignaled(l) {
		t.Fatal("noise event signaled wake; lifecycle churn must not wake sleeping agents")
	}

	l.onEvent(substantiveEv)
	if pendingLen(l) != before+1 {
		t.Fatal("substantive event was not appended to pendingEvents")
	}
	if !wakeSignaled(l) {
		t.Fatal("substantive event did not signal wake")
	}
}

func drainWake(l *Loop) {
	select {
	case <-l.wake:
	default:
	}
}

func wakeSignaled(l *Loop) bool {
	select {
	case <-l.wake:
		return true
	default:
		return false
	}
}

func pendingLen(l *Loop) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pendingEvents)
}

func eventByID(t *testing.T, g *graph.Graph, id types.EventID) event.Event {
	t.Helper()
	page, err := g.Store().ByType(work.EventTypeTaskCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	for _, ev := range page.Items() {
		if ev.ID() == id {
			return ev
		}
	}
	t.Fatalf("event %s not found", id.Value())
	return event.Event{}
}
