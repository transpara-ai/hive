package loop

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
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
// strands. These tests pin the fix: a store-backed, order-aware "reviewable
// work exists" gate (the hasAssignableWork analog) feeding the keepalive
// re-check timer.
//
// Fixture shape (round-1 review findings M-2/M-3/m-1): completions carry REAL
// commit ranges in a real git repo; historical events are written through a
// PRIOR STORE GENERATION (fresh graph/TaskStore/agent handles opened over the
// same underlying store, exactly what a daemon restart produces); historical
// reviews are emitted by a generation-1 REVIEWER identity distinct from the
// reviewer under test.
// ════════════════════════════════════════════════════════════════════════

// storeGeneration bundles the per-process handles (graph, TaskStore, conv ID)
// one daemon instance holds over the shared durable store.
type storeGeneration struct {
	g      *graph.Graph
	ts     *work.TaskStore
	convID types.ConversationID
}

// newStoreGeneration opens fresh handles over the shared store + actor store,
// as a new daemon process would after a restart. Generation handles are
// deliberately NOT closed mid-test: Graph.Close closes the SHARED store, and
// abandoning a generation's handles without cleanup is exactly what a killed
// daemon does.
func newStoreGeneration(t *testing.T, s store.Store, as actor.IActorStore, gen int) storeGeneration {
	t.Helper()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatalf("graph.Start (gen %d): %v", gen, err)
	}
	work.RegisterWithRegistry(g.Registry())
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID(fmt.Sprintf("conv_gov_recheck_gen%d", gen))
	return storeGeneration{g: g, ts: ts, convID: convID}
}

// generationAgent creates an agent of the given role bound to a generation's
// graph — each generation's agents have fresh ephemeral identities, exactly as
// production bootstrap identities do across restarts.
func generationAgent(t *testing.T, gen storeGeneration, role string) *hiveagent.Agent {
	t.Helper()
	n := atomic.AddUint32(&agentCounter, 1)
	a, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role(role),
		Name:     fmt.Sprintf("recheck-%s-%d", role, n),
		Graph:    gen.g,
		Provider: newMockProvider(`/signal {"signal":"IDLE"}`),
	})
	if err != nil {
		t.Fatalf("New %s agent: %v", role, err)
	}
	return a
}

// causesFor returns the agent's last event as the causal link for store writes.
func causesFor(a *hiveagent.Agent) []types.EventID {
	if !a.LastEvent().IsZero() {
		return []types.EventID{a.LastEvent()}
	}
	return nil
}

// completeNewTaskWithCommit persists create→artifact→complete through the real
// store APIs. The artifact carries a REAL verified commit range (base..head)
// produced by committing path into repo — the exact body
// buildOperateArtifactBody writes in production — so the completion is
// REVIEWABLE: the reviewer's observation resolves a real commit and diff.
func completeNewTaskWithCommit(t *testing.T, gen storeGeneration, completer *hiveagent.Agent, repo, path, title string) types.EventID {
	t.Helper()
	task, err := gen.ts.Create(completer.ID(), title, "desc", causesFor(completer), gen.convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	completeExistingTaskWithCommit(t, gen, completer, repo, path, task.ID)
	return task.ID
}

// completeExistingTaskWithCommit commits a new change to repo and records
// artifact+complete for an EXISTING task — the re-completion path the
// review→fix loop produces after a request_changes verdict.
func completeExistingTaskWithCommit(t *testing.T, gen storeGeneration, completer *hiveagent.Agent, repo, path string, taskID types.EventID) {
	t.Helper()
	base := gitCommand(repo, "rev-parse", "HEAD")
	commitFile(t, repo, path, "content for "+path, "implement "+path)
	head := gitCommand(repo, "rev-parse", "HEAD")
	body := buildOperateArtifactBody(repo, base, head)
	if err := gen.ts.AddArtifact(completer.ID(), taskID, "Operate result", "text/plain", body, causesFor(completer), gen.convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := gen.ts.Complete(completer.ID(), taskID, "implemented in "+head, causesFor(completer), gen.convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

// reviewAs records a code.review.submitted verdict through the production
// emission path, as the given (generation-bound) reviewer identity.
func reviewAs(t *testing.T, reviewer *hiveagent.Agent, taskID types.EventID, verdict string) {
	t.Helper()
	err := reviewer.EmitCodeReview(event.CodeReviewContent{
		TaskID:     taskID.Value(),
		Verdict:    verdict,
		Summary:    "prior-instance review",
		Issues:     []string{},
		Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("EmitCodeReview: %v", err)
	}
}

// newReviewerRecheckLoop constructs a keepalive reviewer Loop bound to a
// generation. Constructed AFTER the caller's fixtures, so anything already in
// the store is historical to this loop instance.
func newReviewerRecheckLoop(t *testing.T, gen storeGeneration, repoPath string, recheck time.Duration) *Loop {
	t.Helper()
	reviewer := generationAgent(t, gen, "reviewer")
	l, err := New(Config{
		Agent:           reviewer,
		HumanID:         humanID(),
		RepoPath:        repoPath,
		TaskStore:       gen.ts,
		ConvID:          gen.convID,
		CanOperate:      false,
		Keepalive:       true,
		RecheckInterval: recheck,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

// TestHasReviewableWork pins the governance re-check gate predicate (the
// hasAssignableWork analog for the Reviewer): false on an empty store; true
// once an unreviewed completion exists in a prior store generation; false for
// a completion the prior generation already reviewed (no re-review storm); and
// false again once this session reviews the only pending task.
func TestHasReviewableWork(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)

	// Generation 1 — the prior daemon instance: two completions with real
	// commit ranges; the second already reviewed by the gen-1 reviewer.
	gen1 := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen1, "implementer")
	gen1Reviewer := generationAgent(t, gen1, "reviewer")
	taskID := completeNewTaskWithCommit(t, gen1, completer, repo, "catalog-a.md", "implement catalog A")
	reviewedID := completeNewTaskWithCommit(t, gen1, completer, repo, "catalog-b.md", "implement catalog B")
	reviewAs(t, gen1Reviewer, reviewedID, "approve")

	// Generation 2 — the restarted daemon: fresh handles, fresh reviewer
	// identity, no bus delivery of anything above.
	gen2 := newStoreGeneration(t, s, as, 2)
	l := newReviewerRecheckLoop(t, gen2, repo, 10*time.Millisecond)

	if !l.hasReviewableWork() {
		t.Fatal("hasReviewableWork = false with an unreviewed historical completion; want true")
	}

	// The seeding that backs the gate must make the completion visible AND
	// reviewable on the next observation — otherwise the wake would be a
	// no-op: the agent wakes, renders "No tasks pending review", and parks.
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
	obs := l.enrichReviewObservation("base obs")
	if !strings.Contains(obs, "TASK UNDER REVIEW") || !strings.Contains(obs, taskID.Value()) {
		t.Fatal("review observation does not render the seeded historical task")
	}
	if strings.Contains(obs, "RECENT COMMIT:\n  (unavailable)") {
		t.Fatal("seeded completion's commit did not resolve; the completion is not actually reviewable")
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

	// Empty store: a reviewer over a fresh world has nothing reviewable.
	s2 := store.NewInMemoryStore()
	as2 := actor.NewInMemoryActorStore()
	genEmpty := newStoreGeneration(t, s2, as2, 1)
	l2 := newReviewerRecheckLoop(t, genEmpty, repo, 10*time.Millisecond)
	if l2.hasReviewableWork() {
		t.Fatal("hasReviewableWork = true on an empty store; want false")
	}
}

// TestHasReviewableWork_RecompletionAfterRequestChanges pins the order-aware
// dedup (round-1 finding B-1): the review→fix sequence completion →
// request_changes → RE-completion, all in a prior generation, must re-pend —
// the latest completion is causally newer than the latest review. Task-ID-only
// dedup suppressed exactly this, stranding the loop this packet exists to
// engage. Once a review NEWER than the re-completion exists, it stops pending.
func TestHasReviewableWork_RecompletionAfterRequestChanges(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)

	gen1 := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen1, "implementer")
	gen1Reviewer := generationAgent(t, gen1, "reviewer")
	taskID := completeNewTaskWithCommit(t, gen1, completer, repo, "fix-target.md", "implement the fix target")
	reviewAs(t, gen1Reviewer, taskID, "request_changes")
	completeExistingTaskWithCommit(t, gen1, completer, repo, "fix-target-round2.md", taskID)

	gen2 := newStoreGeneration(t, s, as, 2)
	l := newReviewerRecheckLoop(t, gen2, repo, 10*time.Millisecond)

	if !l.hasReviewableWork() {
		t.Fatal("hasReviewableWork = false for a re-completion newer than its request_changes review; the review→fix loop is stranded (B-1)")
	}
	if _, ok := l.reviewerState.completedTasks[taskID.Value()]; !ok {
		t.Fatal("re-completed task not seeded for the next observation")
	}

	// A review newer than the re-completion settles it: a FRESH generation
	// must see nothing pending.
	reviewAs(t, gen1Reviewer, taskID, "approve")
	gen3 := newStoreGeneration(t, s, as, 3)
	l3 := newReviewerRecheckLoop(t, gen3, repo, 10*time.Millisecond)
	if l3.hasReviewableWork() {
		t.Fatal("hasReviewableWork = true although the latest review is newer than the latest completion")
	}
}

// TestFindPendingReviews_RecoveredCountWithoutVerdictPends covers the recovery
// shape (round-1 B-1 adjacent): InitReviewerFromRecovery seeds review COUNTS
// but no verdicts, so a recovered record has lastVerdict == "". A known
// completion with an unknown last verdict must fail toward review — bounded by
// shouldEscalate's cycle cap — never silently drop out of the loop.
func TestFindPendingReviews_RecoveredCountWithoutVerdictPends(t *testing.T) {
	st := newReviewerState()
	taskID, _ := types.NewEventIDFromNew()
	st.completedTasks[taskID.Value()] = work.TaskCompletedContent{TaskID: taskID}
	st.reviewHistory[taskID.Value()] = &taskReviewRecord{taskID: taskID.Value(), reviewCount: 1}

	pending := st.findPendingReviews()
	found := false
	for _, id := range pending {
		if id == taskID.Value() {
			found = true
		}
	}
	if !found {
		t.Fatal("a completed task with a recovery-seeded (verdict-less) review record was not re-pended; recovery would strand it")
	}
}

// TestPaginateAllByType_FollowsCursors pins full pagination behind the
// historical scans (round-1 finding B-2): every page is walked via the cursor,
// so reviewable completions older than one page are never silently ignored.
func TestPaginateAllByType_FollowsCursors(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)

	gen1 := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen1, "implementer")
	const total = 7
	for i := 0; i < total; i++ {
		completeNewTaskWithCommit(t, gen1, completer, repo,
			fmt.Sprintf("page-%d.md", i), fmt.Sprintf("implement page %d", i))
	}

	events, err := paginateAllByType(s, work.EventTypeTaskCompleted, 2)
	if err != nil {
		t.Fatalf("paginateAllByType: %v", err)
	}
	if len(events) != total {
		t.Fatalf("paginateAllByType returned %d completions across pages; want %d (pagination must follow cursors)", len(events), total)
	}
}

// TestWaitForEvents_ReviewerRecheckPicksUpHistoricalCompletion is the F8
// keystone (run findings, finding 8; packet G-1.1 AC-1): a keepalive reviewer
// whose reviewable completed work was persisted by a PRIOR STORE GENERATION —
// the exact shape every production-driver round produced — must return from
// waitForEvents within one re-check interval, with no fresh
// work.task.completed wake. Under the pre-fix code a non-CanOperate keepalive
// agent blocks on the wake channel forever, which is why every round needed a
// daemon restart and the review→fix loop never engaged.
func TestWaitForEvents_ReviewerRecheckPicksUpHistoricalCompletion(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)

	gen1 := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen1, "implementer")
	completeNewTaskWithCommit(t, gen1, completer, repo, "keystone.md", "implement the keystone")

	gen2 := newStoreGeneration(t, s, as, 2)
	l := newReviewerRecheckLoop(t, gen2, repo, 10*time.Millisecond)

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
// reviewable — both an empty store and a store whose only completion was
// already reviewed by the prior generation — the agent stays parked. Returning
// here would re-ignite the wakeup storm at LLM-call cost.
func TestWaitForEvents_ReviewerRecheckStaysParkedWhenNothingReviewable(t *testing.T) {
	// Stage 1: empty store.
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	genEmpty := newStoreGeneration(t, s, as, 1)
	l := newReviewerRecheckLoop(t, genEmpty, repo, 10*time.Millisecond)

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

	// Stage 2: a completion exists but the prior generation already reviewed it.
	s2 := store.NewInMemoryStore()
	as2 := actor.NewInMemoryActorStore()
	gen1 := newStoreGeneration(t, s2, as2, 1)
	completer := generationAgent(t, gen1, "implementer")
	gen1Reviewer := generationAgent(t, gen1, "reviewer")
	reviewedID := completeNewTaskWithCommit(t, gen1, completer, repo, "reviewed.md", "implement reviewed work")
	reviewAs(t, gen1Reviewer, reviewedID, "approve")

	gen2 := newStoreGeneration(t, s2, as2, 2)
	l2 := newReviewerRecheckLoop(t, gen2, repo, 10*time.Millisecond)

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
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, s, as, 1)
	l := newReviewerRecheckLoop(t, gen, repo, time.Hour)

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
