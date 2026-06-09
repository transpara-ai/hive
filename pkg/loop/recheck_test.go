package loop

import (
	"context"
	"testing"
	"time"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// newRecheckLoop builds a keepalive Loop wired to a real in-memory TaskStore for
// exercising the CanOperate re-check timer. recheck is the periodic interval;
// canOperate selects the implementer (ticker) vs governance (wake-only) path.
func newRecheckLoop(t *testing.T, canOperate bool, recheck time.Duration) (*Loop, *work.TaskStore, *hiveagent.Agent, types.ConversationID) {
	t.Helper()
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_recheck_test")
	l, err := New(Config{
		Agent:           agent,
		HumanID:         humanID(),
		RepoPath:        t.TempDir(),
		TaskStore:       ts,
		ConvID:          convID,
		CanOperate:      canOperate,
		Keepalive:       true,
		RecheckInterval: recheck,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l, ts, agent, convID
}

// seedAssignedTask creates a fresh (Created/operable) task and assigns it to the
// agent, so hasAssignedTask — and therefore hasAssignableWork — is true.
func seedAssignedTask(t *testing.T, ts *work.TaskStore, agent *hiveagent.Agent, convID types.ConversationID) {
	t.Helper()
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "work", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ts.Assign(agent.ID(), task.ID, agent.ID(), causes, convID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
}

// TestHasAssignableWork covers the re-check gate predicate: nothing to do on an
// empty store, but an assigned operable task counts as assignable work.
func TestHasAssignableWork(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, true, 10*time.Millisecond)

	if l.hasAssignableWork() {
		t.Fatal("hasAssignableWork = true on an empty store; want false")
	}

	seedAssignedTask(t, ts, agent, convID)

	if !l.hasAssignableWork() {
		t.Fatal("hasAssignableWork = false with an assigned operable task; want true")
	}
}

// TestFirstAssignableOpenTask locks in the shared open-leaf predicate (the single
// source of truth for both auto-assign and the re-check gate): an open task is
// only assignable once unassigned, childless, AND readiness-gated; assigning it
// removes it from the open-unassigned set.
func TestFirstAssignableOpenTask(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, true, 10*time.Millisecond)
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}

	task, err := ts.Create(agent.ID(), "implement feature", "", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Created but readiness gates unmet → not yet assignable.
	if _, ok := l.firstAssignableOpenTask(); ok {
		t.Fatal("firstAssignableOpenTask returned a task with unmet readiness gates")
	}

	// Satisfy the required readiness gates → the open, unassigned leaf is assignable.
	for _, label := range work.RequiredReadinessGateLabels() {
		if err := ts.AddArtifact(agent.ID(), task.ID, label, "text/markdown", "gate body", causes, convID); err != nil {
			t.Fatalf("AddArtifact %s: %v", label, err)
		}
	}
	got, ok := l.firstAssignableOpenTask()
	if !ok {
		t.Fatal("firstAssignableOpenTask = false for an open, unassigned, ready leaf; want true")
	}
	if got.ID != task.ID {
		t.Fatalf("firstAssignableOpenTask returned %s; want %s", got.ID.Value(), task.ID.Value())
	}

	// Once assigned, it is no longer an open *unassigned* leaf.
	if err := ts.Assign(agent.ID(), task.ID, agent.ID(), causes, convID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if _, ok := l.firstAssignableOpenTask(); ok {
		t.Fatal("firstAssignableOpenTask returned an already-assigned task")
	}
}

// TestWaitForEvents_RechecksAssignableWorkWithoutWakeSignal is the keystone: a
// CanOperate keepalive agent with assignable work but NO wake signal (the
// dropped-edge race) must still return via the periodic re-check, instead of
// blocking forever the way a daemon restart otherwise had to clear.
func TestWaitForEvents_RechecksAssignableWorkWithoutWakeSignal(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, true, 10*time.Millisecond)
	seedAssignedTask(t, ts, agent, convID)

	done := make(chan bool, 1)
	go func() { done <- l.waitForEvents(context.Background()) }()

	select {
	case got := <-done:
		if !got {
			t.Fatal("waitForEvents returned false; the gated re-check should wake on assignable work")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForEvents never returned; the periodic re-check did not fire for assignable work (the wakeup race persists)")
	}
}

// TestWaitForEvents_StaysParkedWhenNoAssignableWork proves the gate: with the
// ticker firing but nothing assignable, the agent must stay parked — returning
// here would re-ignite the wakeup storm the per-iteration timers were removed to
// kill.
func TestWaitForEvents_StaysParkedWhenNoAssignableWork(t *testing.T) {
	l, _, _, _ := newRecheckLoop(t, true, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- l.waitForEvents(ctx) }()

	select {
	case <-done:
		t.Fatal("waitForEvents returned with no assignable work; the gate must keep the agent parked")
	case <-time.After(120 * time.Millisecond): // ~12 ticks with nothing to do
		// Correct: stayed parked. Release the goroutine.
	}
	cancel()
	<-done
}

// TestWaitForEvents_WakeSignalReturnsUnderTicker ensures the re-check branch does
// not swallow real wake signals: an explicit wake returns promptly even with the
// ticker armed (set far in the future so only the wake can return).
func TestWaitForEvents_WakeSignalReturnsUnderTicker(t *testing.T) {
	l, _, _, _ := newRecheckLoop(t, true, time.Hour)

	done := make(chan bool, 1)
	go func() { done <- l.waitForEvents(context.Background()) }()

	time.Sleep(20 * time.Millisecond) // let the goroutine park
	select {
	case l.wake <- struct{}{}:
	default:
		t.Fatal("wake channel unexpectedly full before signalling")
	}

	select {
	case got := <-done:
		if !got {
			t.Fatal("waitForEvents returned false after a wake signal; want true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForEvents did not return after a wake signal under the re-check branch")
	}
}

// TestWaitForEvents_NonOperateKeepaliveBlocksOnWakeOnly scopes the fix: a
// governance agent (Keepalive but NOT CanOperate) was the wakeup-storm victim and
// must keep pure wake-blocking — it grows no re-check timer even when assignable
// work and a RecheckInterval are both present.
func TestWaitForEvents_NonOperateKeepaliveBlocksOnWakeOnly(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, false, 10*time.Millisecond)
	seedAssignedTask(t, ts, agent, convID) // work exists, but a non-operate agent ignores it

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- l.waitForEvents(ctx) }()

	select {
	case <-done:
		t.Fatal("non-CanOperate keepalive agent returned without a wake; it must not grow a re-check timer")
	case <-time.After(120 * time.Millisecond):
		// Correct: blocked on wake only.
	}
	cancel()
	<-done
}
