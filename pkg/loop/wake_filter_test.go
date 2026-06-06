package loop

import (
	"testing"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
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
		event.AgentStateChangedContent{AgentID: otherActor, Previous: egagent.StateIdle.String(), Current: egagent.StateProcessing.String()},
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

// isChurnStateChange must suppress ONLY the Idle⇄Processing toggle (BOTH
// endpoints churn). A transition touching any significant state at EITHER end —
// including a resume (Suspended→Idle) that merely ends in Idle — stays visible.
func TestIsChurnStateChange(t *testing.T) {
	agent, g := agentWithGraph(t, newMockProvider(`/signal {"signal":"IDLE"}`))
	factory := event.NewEventFactory(g.Registry())
	signer := &testSigner{}
	conv := types.MustConversationID("conv_churn")
	actor := types.MustActorID("actor_00000000000000000000000000000099")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	mk := func(previous, current string) event.Event {
		ev, err := factory.Create(event.EventTypeAgentStateChanged, actor,
			event.AgentStateChangedContent{AgentID: actor, Previous: previous, Current: current},
			causes, conv, g.Store(), signer)
		if err != nil {
			t.Fatalf("create state event (%s→%s): %v", previous, current, err)
		}
		return ev
	}
	idle, proc := egagent.StateIdle.String(), egagent.StateProcessing.String()
	susp, retiring := egagent.StateSuspended.String(), egagent.StateRetiring.String()

	cases := []struct {
		previous, current string
		churn             bool
	}{
		{idle, proc, true},                     // pure churn toggle
		{proc, idle, true},                     // pure churn toggle
		{idle, idle, true},                     // churn
		{susp, idle, false},                    // RESUME — significant, ends in Idle
		{susp, proc, false},                    // resume into processing — significant
		{idle, susp, false},                    // suspend — significant
		{proc, retiring, false},                // retiring — significant
		{idle, egagent.StateRetired.String(), false},
		{idle, egagent.StateWaiting.String(), false},
		{idle, "SomeFutureState", false},       // unknown — keep visible
	}
	for _, c := range cases {
		if got := isChurnStateChange(mk(c.previous, c.current)); got != c.churn {
			t.Errorf("isChurnStateChange(%s→%s) = %v, want %v", c.previous, c.current, got, c.churn)
		}
	}
}

// #P2 (round-5): significant lifecycle transitions (Suspended/Retiring/Retired)
// must stay visible to peers AND wake idle governance agents — only Idle/Processing
// churn is suppressed. Hiding a peer's suspension/retirement is a governance loss.
func TestOnEvent_SuspensionStateChangeIsObservableAndWakes(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	l, err := New(Config{Agent: agent, HumanID: humanID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	factory := event.NewEventFactory(g.Registry())
	signer := &testSigner{}
	conv := types.MustConversationID("conv_suspend")
	cause := agent.LastEvent()
	if cause.IsZero() {
		t.Fatal("no bootstrap cause")
	}
	otherActor := types.MustActorID("actor_00000000000000000000000000000055")
	suspendEv, err := factory.Create(event.EventTypeAgentStateChanged, otherActor,
		event.AgentStateChangedContent{AgentID: otherActor, Previous: egagent.StateProcessing.String(), Current: egagent.StateSuspended.String()},
		[]types.EventID{cause}, conv, g.Store(), signer)
	if err != nil {
		t.Fatalf("create suspend event: %v", err)
	}

	drainWake(l)
	before := pendingLen(l)
	l.onEvent(suspendEv)
	if pendingLen(l) != before+1 {
		t.Fatal("a peer Suspended state change was not queued — lifecycle transitions must stay visible to peers")
	}
	if !wakeSignaled(l) {
		t.Fatal("a peer Suspended state change did not wake the agent — significant lifecycle changes must wake idle governance agents")
	}
}

// #P2 (round-6): the RESUME transition Suspended→Idle has Current=Idle but is a
// significant recovery signal, not churn. A transition FROM a significant state
// must stay visible to peers and wake them — classifying by Current alone hid it.
func TestOnEvent_ResumeStateChangeIsObservableAndWakes(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	l, err := New(Config{Agent: agent, HumanID: humanID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	factory := event.NewEventFactory(g.Registry())
	signer := &testSigner{}
	conv := types.MustConversationID("conv_resume")
	cause := agent.LastEvent()
	if cause.IsZero() {
		t.Fatal("no bootstrap cause")
	}
	otherActor := types.MustActorID("actor_00000000000000000000000000000056")
	// Resume: Suspended -> Idle.
	resumeEv, err := factory.Create(event.EventTypeAgentStateChanged, otherActor,
		event.AgentStateChangedContent{AgentID: otherActor, Previous: egagent.StateSuspended.String(), Current: egagent.StateIdle.String()},
		[]types.EventID{cause}, conv, g.Store(), signer)
	if err != nil {
		t.Fatalf("create resume event: %v", err)
	}

	drainWake(l)
	before := pendingLen(l)
	l.onEvent(resumeEv)
	if pendingLen(l) != before+1 {
		t.Fatal("a peer Suspended→Idle resume was not queued — recovery transitions must stay visible to peers")
	}
	if !wakeSignaled(l) {
		t.Fatal("a peer Suspended→Idle resume did not wake the agent — a recovery signal must reach sleeping governance peers")
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
