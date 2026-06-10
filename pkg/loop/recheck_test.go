package loop

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
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

// TestWaitForEvents_NonOperateKeepaliveBlocksOnWakeOnly scopes the re-check: a
// keepalive governance agent WITHOUT a review duty (this fixture's agent is a
// strategist) keeps pure wake-blocking — it grows no re-check timer even when
// assignable work and a RecheckInterval are both present. Only the implementer
// (assignable-work gate) and the reviewer (reviewable-work gate, finding F8)
// carry a periodic re-check; every other keepalive agent stays parked on the
// wake channel exactly as before.
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

// ════════════════════════════════════════════════════════════════════════
// Governance (reviewer) re-check — slice-1 finding F8
//
// The reviewer triggers on a FRESH work.task.completed bus wake. A completion
// persisted by a prior daemon instance is historical: the bus never re-delivers
// it, reviewerState.completedTasks never learns of it, and the review→fix loop
// strands. These tests pin the fix: a store-backed "reviewable work exists"
// gate (the hasAssignableWork analog) feeding the keepalive re-check timer.
// ════════════════════════════════════════════════════════════════════════

// sharedGraphFixture builds one graph with a completer agent and a TaskStore
// over the same store, so tests can persist REAL completions through the store
// BEFORE a reviewer loop exists — the "prior store generation" shape of a
// daemon restart (run findings F8): the bus never delivers those events to the
// new loop; only the store remembers them.
func sharedGraphFixture(t *testing.T) (*graph.Graph, *work.TaskStore, *hiveagent.Agent, types.ConversationID) {
	t.Helper()
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	completer, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_gov_recheck_test")
	return g, ts, completer, convID
}

// newReviewerRecheckLoop constructs a keepalive reviewer Loop over an existing
// graph + store. Constructed AFTER the caller's fixtures, so any completion
// already in the store is historical to this loop instance.
func newReviewerRecheckLoop(t *testing.T, g *graph.Graph, ts *work.TaskStore, convID types.ConversationID, recheck time.Duration) *Loop {
	t.Helper()
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	n := atomic.AddUint32(&agentCounter, 1)
	reviewer, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("reviewer"),
		Name:     fmt.Sprintf("recheck-reviewer-%d", n),
		Graph:    g,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("New reviewer agent: %v", err)
	}
	l, err := New(Config{
		Agent:           reviewer,
		HumanID:         humanID(),
		RepoPath:        t.TempDir(),
		TaskStore:       ts,
		ConvID:          convID,
		CanOperate:      false,
		Keepalive:       true,
		RecheckInterval: recheck,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

// completeTaskHistorically persists create→artifact→complete through the real
// store APIs (the artifact gate auto-populates ArtifactRef exactly as
// production completions carry it) and returns the task ID.
func completeTaskHistorically(t *testing.T, ts *work.TaskStore, completer *hiveagent.Agent, convID types.ConversationID) types.EventID {
	t.Helper()
	var causes []types.EventID
	if !completer.LastEvent().IsZero() {
		causes = []types.EventID{completer.LastEvent()}
	}
	task, err := ts.Create(completer.ID(), "implement the catalog", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ts.AddArtifact(completer.ID(), task.ID, "Operate result", "text/plain", "commit: deadbeef", causes, convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := ts.Complete(completer.ID(), task.ID, "implemented", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return task.ID
}

// persistHistoricalReview appends a code.review.submitted event for taskID as
// if a prior reviewer instance (a different ephemeral actor — exactly what a
// daemon restart produces) had already reviewed it.
func persistHistoricalReview(t *testing.T, g *graph.Graph, source *hiveagent.Agent, convID types.ConversationID, taskID types.EventID) {
	t.Helper()
	factory := event.NewEventFactory(g.Registry())
	var causes []types.EventID
	if !source.LastEvent().IsZero() {
		causes = []types.EventID{source.LastEvent()}
	}
	ev, err := factory.Create(event.EventTypeCodeReviewSubmitted, source.ID(), event.CodeReviewContent{
		TaskID:     taskID.Value(),
		Verdict:    "approve",
		Summary:    "looks correct",
		Issues:     []string{},
		Confidence: 0.9,
	}, causes, convID, g.Store(), &testSigner{})
	if err != nil {
		t.Fatalf("create review event: %v", err)
	}
	if _, err := g.Store().Append(ev); err != nil {
		t.Fatalf("append review event: %v", err)
	}
}

// TestHasReviewableWork pins the governance re-check gate predicate (the
// hasAssignableWork analog for the Reviewer): false on an empty store; true
// once a completion exists that the bus never delivered (it predates this loop
// instance); false when the store also holds a review for it — a restart must
// not re-review already-reviewed work (no re-review storm); and false for work
// this session already reviewed.
func TestHasReviewableWork(t *testing.T) {
	g, ts, completer, convID := sharedGraphFixture(t)
	// Historical fixtures persisted BEFORE the reviewer loop exists.
	taskID := completeTaskHistorically(t, ts, completer, convID)
	reviewedID := completeTaskHistorically(t, ts, completer, convID)
	persistHistoricalReview(t, g, completer, convID, reviewedID)

	l := newReviewerRecheckLoop(t, g, ts, convID, 10*time.Millisecond)

	if !l.hasReviewableWork() {
		t.Fatal("hasReviewableWork = false with an unreviewed historical completion; want true")
	}

	// The seeding that backs the gate must also make the completion visible to
	// the reviewer's next observation — otherwise the wake would be a no-op:
	// the agent wakes, renders "No tasks pending review", and parks again.
	c, ok := l.reviewerState.completedTasks[taskID.Value()]
	if !ok {
		t.Fatal("historical completion not seeded into reviewerState.completedTasks")
	}
	if c.CompletedBy != completer.ID() {
		t.Fatalf("seeded completion lost CompletedBy: got %s, want %s", c.CompletedBy.Value(), completer.ID().Value())
	}
	if c.ArtifactRef.IsZero() {
		t.Fatal("seeded completion lost ArtifactRef; review git context would degrade to heuristics")
	}
	// The already-reviewed historical task must NOT be seeded as pending.
	if _, ok := l.reviewerState.completedTasks[reviewedID.Value()]; ok {
		t.Fatal("historically-reviewed completion was seeded; a restart would re-review it")
	}

	// A review recorded THIS session clears the pending state.
	l.reviewerState.recordReview(taskID.Value(), "approve", []string{}, 1)
	if l.hasReviewableWork() {
		t.Fatal("hasReviewableWork = true after this session reviewed the only pending task; want false")
	}

	// Empty store: a reviewer over a fresh graph has nothing reviewable.
	g2, ts2, _, convID2 := sharedGraphFixture(t)
	l2 := newReviewerRecheckLoop(t, g2, ts2, convID2, 10*time.Millisecond)
	if l2.hasReviewableWork() {
		t.Fatal("hasReviewableWork = true on an empty store; want false")
	}
}

// TestWaitForEvents_ReviewerRecheckPicksUpHistoricalCompletion is the F8
// keystone (run findings, finding 8; packet G-1.1 AC-1): a keepalive reviewer
// whose reviewable completed work was persisted BEFORE this loop instance
// existed — a prior store generation, the exact shape every production-driver
// round produced — must return from waitForEvents within one re-check
// interval, with no fresh work.task.completed wake. Under the pre-fix code a
// non-CanOperate keepalive agent blocks on the wake channel forever, which is
// why every round needed a daemon restart and the review→fix loop never
// engaged.
func TestWaitForEvents_ReviewerRecheckPicksUpHistoricalCompletion(t *testing.T) {
	g, ts, completer, convID := sharedGraphFixture(t)
	completeTaskHistorically(t, ts, completer, convID) // prior generation
	l := newReviewerRecheckLoop(t, g, ts, convID, 10*time.Millisecond)

	done := make(chan bool, 1)
	go func() { done <- l.waitForEvents(context.Background()) }()

	select {
	case got := <-done:
		if !got {
			t.Fatal("waitForEvents returned false; the governance re-check should wake on reviewable work")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForEvents never returned; the reviewer re-check did not fire for a historical completion (F8 persists)")
	}
}

// TestWaitForEvents_ReviewerRecheckStaysParkedWhenNothingReviewable proves the
// gate (packet G-1.1 AC-2): with the reviewer ticker armed but nothing
// reviewable — both an empty store and a store whose only completion is
// already reviewed — the agent stays parked. Returning here would re-ignite
// the wakeup storm at LLM-call cost.
func TestWaitForEvents_ReviewerRecheckStaysParkedWhenNothingReviewable(t *testing.T) {
	// Stage 1: empty store.
	g, ts, _, convID := sharedGraphFixture(t)
	l := newReviewerRecheckLoop(t, g, ts, convID, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- l.waitForEvents(ctx) }()

	select {
	case <-done:
		cancel()
		t.Fatal("reviewer returned from waitForEvents with an empty store; the gate must keep it parked")
	case <-time.After(120 * time.Millisecond): // ~12 ticks with nothing to do
	}
	cancel()
	<-done

	// Stage 2: a completion exists but is already reviewed (historically).
	g2, ts2, completer2, convID2 := sharedGraphFixture(t)
	reviewedID := completeTaskHistorically(t, ts2, completer2, convID2)
	persistHistoricalReview(t, g2, completer2, convID2, reviewedID)
	l2 := newReviewerRecheckLoop(t, g2, ts2, convID2, 10*time.Millisecond)

	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan bool, 1)
	go func() { done2 <- l2.waitForEvents(ctx2) }()

	select {
	case <-done2:
		cancel2()
		t.Fatal("reviewer returned from waitForEvents for an already-reviewed completion; a restart must not re-review")
	case <-time.After(120 * time.Millisecond):
	}
	cancel2()
	<-done2
}

// TestNew_DefaultsRecheckForReviewDuty pins the New() defaulting that makes
// the governance re-check reach production: the runtime never sets
// RecheckInterval, so a keepalive reviewer with an unset interval must get the
// same slow safety-net default a CanOperate keepalive agent gets. A keepalive
// agent with neither duty gets NO default (disabled-by-default for agents with
// no governance review duty), and an explicit <0 stays disabled.
func TestNew_DefaultsRecheckForReviewDuty(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)

	reviewer := testHiveAgent(t, provider, "reviewer", "recheck-default-reviewer")
	l, err := New(Config{Agent: reviewer, HumanID: humanID(), Keepalive: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l.config.RecheckInterval != 30*time.Second {
		t.Fatalf("keepalive reviewer RecheckInterval = %v; want the 30s default", l.config.RecheckInterval)
	}

	reviewerOff := testHiveAgent(t, provider, "reviewer", "recheck-disabled-reviewer")
	l2, err := New(Config{Agent: reviewerOff, HumanID: humanID(), Keepalive: true, RecheckInterval: -1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l2.config.RecheckInterval >= 0 {
		t.Fatalf("explicit negative RecheckInterval was overridden to %v; <0 must stay disabled", l2.config.RecheckInterval)
	}

	strategist := testHiveAgent(t, provider, "strategist", "recheck-default-strategist")
	l3, err := New(Config{Agent: strategist, HumanID: humanID(), Keepalive: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l3.config.RecheckInterval != 0 {
		t.Fatalf("keepalive agent with no re-check duty got RecheckInterval = %v; want 0 (no default)", l3.config.RecheckInterval)
	}

	nonKeepaliveReviewer := testHiveAgent(t, provider, "reviewer", "recheck-nonkeepalive-reviewer")
	l4, err := New(Config{Agent: nonKeepaliveReviewer, HumanID: humanID()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l4.config.RecheckInterval != 0 {
		t.Fatalf("non-keepalive reviewer got RecheckInterval = %v; want 0 (re-check is a keepalive concern)", l4.config.RecheckInterval)
	}
}

// TestWaitForEvents_ReviewerWakeSignalReturnsUnderTicker ensures the reviewer
// re-check branch does not swallow real wake signals: an explicit wake returns
// promptly even with the ticker armed (set far in the future so only the wake
// can return) — mirroring the CanOperate guarantee.
func TestWaitForEvents_ReviewerWakeSignalReturnsUnderTicker(t *testing.T) {
	g, ts, _, convID := sharedGraphFixture(t)
	l := newReviewerRecheckLoop(t, g, ts, convID, time.Hour)

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
		t.Fatal("waitForEvents did not return after a wake signal under the reviewer re-check branch")
	}
}
