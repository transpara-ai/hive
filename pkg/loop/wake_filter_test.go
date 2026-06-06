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

// isObservable must keep peer evaluations (governance signal) and all
// substantive events queueable, while dropping only pure lifecycle telemetry
// (state changes, observations) that would otherwise churn observation context.
func TestIsObservable(t *testing.T) {
	// Pure telemetry: dropped from observation entirely.
	dropped := []types.EventType{
		event.EventTypeAgentStateChanged,
		event.EventTypeAgentObserved,
	}
	for _, et := range dropped {
		if isObservable(et) {
			t.Errorf("isObservable(%s) = true, want false (pure telemetry must not be queued)", et.Value())
		}
	}
	// Kept for visibility: peer evaluations (do not wake, but must be seen) and
	// every substantive event.
	kept := []types.EventType{
		event.EventTypeAgentEvaluated,
		event.EventTypeAgentActed,
		work.EventTypeTaskCreated,
		work.EventTypeTaskCompleted,
		types.MustEventType("authority.request.recorded"),
		types.MustEventType("some.unknown.future.event"),
	}
	for _, et := range kept {
		if !isObservable(et) {
			t.Errorf("isObservable(%s) = false, want true (governance/substantive events must stay visible)", et.Value())
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

// #3: peer evaluations (agent.evaluated) carry governance signal — another
// agent's judgment. They must NOT wake a sleeping keepalive agent (that was the
// churn storm), but they MUST be queued for the agent's next observation.
// Dropping them entirely blinds peers to each other's evaluations. The wake
// decision and the queue decision are separate.
func TestOnEvent_EvaluationQueuedButDoesNotWake(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	l, err := New(Config{Agent: agent, HumanID: humanID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	factory := event.NewEventFactory(g.Registry())
	signer := &testSigner{}
	conv := types.MustConversationID("conv_eval_test")
	cause := agent.LastEvent()
	if cause.IsZero() {
		t.Fatal("agent has no bootstrap event to use as a cause")
	}
	otherActor := types.MustActorID("actor_00000000000000000000000000000043")

	conf, _ := types.NewScore(0.8)
	evalEv, err := factory.Create(event.EventTypeAgentEvaluated, otherActor,
		event.AgentEvaluatedContent{AgentID: otherActor, Subject: "peer work", Confidence: conf, Result: "ok"},
		[]types.EventID{cause}, conv, g.Store(), signer)
	if err != nil {
		t.Fatalf("create evaluation event: %v", err)
	}

	drainWake(l)
	before := pendingLen(l)
	l.onEvent(evalEv)
	if pendingLen(l) != before+1 {
		t.Fatal("peer agent.evaluated was not queued in pendingEvents — sleeping peers lose visibility into evaluations")
	}
	if wakeSignaled(l) {
		t.Fatal("peer agent.evaluated signaled wake; evaluations must be visible but must not wake idle agents")
	}
}

// pendingEvents must be bounded so a long keepalive sleep cannot accumulate an
// unbounded backlog (invariant BOUNDED). When the cap is exceeded the oldest
// events are dropped.
func TestOnEvent_PendingEventsBounded(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	l, err := New(Config{Agent: agent, HumanID: humanID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	factory := event.NewEventFactory(g.Registry())
	signer := &testSigner{}
	conv := types.MustConversationID("conv_bound_test")
	cause := agent.LastEvent()
	if cause.IsZero() {
		t.Fatal("no bootstrap cause")
	}
	otherActor := types.MustActorID("actor_00000000000000000000000000000044")

	ts := work.NewTaskStore(g.Store(), factory, signer)
	task, err := ts.Create(otherActor, "work", "desc", []types.EventID{cause}, conv)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	substantiveEv := eventByID(t, g, task.ID)

	for i := 0; i < maxPendingEvents+50; i++ {
		l.onEvent(substantiveEv)
	}
	if got := pendingLen(l); got != maxPendingEvents {
		t.Fatalf("pendingEvents len = %d, want capped at %d", got, maxPendingEvents)
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
