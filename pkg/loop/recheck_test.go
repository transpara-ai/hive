package loop

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

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

// TestFirstAssignableOpenTask locks in the shared open-task predicate (the single
// source of truth for both auto-assign and the re-check gate): an open task is
// only assignable once unassigned, non-aggregate (no declared dependencies), AND
// readiness-gated; assigning it removes it from the open-unassigned set.
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

func TestFirstAssignableOpenTaskSkipsIssueScanStageAggregator(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, true, 10*time.Millisecond)
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}

	stageTask, err := ts.CreateV39(agent.ID(), work.TaskCreateOptions{
		Title:                  "Issue-scan stage: Research issue and repo context",
		Description:            "Governed stage aggregator. Role artifacts complete this task.",
		FactoryOrderID:         "fo_issue_001",
		RequirementIDs:         []string{"req_issue_001"},
		AcceptanceCriterionIDs: []string{"ac_issue_001"},
		CanonicalTaskID:        "tsk_issue_001_research_issue_and_repo_context",
		Cell:                   "planning",
		RiskClass:              "medium",
		ExpectedOutputs:        []string{"stage declaration artifact remains pending runtime evidence"},
	}, causes, convID)
	if err != nil {
		t.Fatalf("CreateV39 stage: %v", err)
	}
	for _, label := range work.RequiredReadinessGateLabels() {
		if err := ts.AddArtifact(agent.ID(), stageTask.ID, label, "text/markdown", "gate body", causes, convID); err != nil {
			t.Fatalf("AddArtifact stage gate %s: %v", label, err)
		}
	}
	outputContract := `{
  "kind": "issue_scan_stage_output_contract",
  "run_id": "run_issue_001",
  "factory_order_id": "fo_issue_001",
  "stage_id": "research_issue_and_repo_context",
  "role_output_contracts": [
    {
      "role": "strategist",
      "required_outputs": ["issue_priority_rationale"]
    }
  ]
}`
	if err := ts.AddArtifact(agent.ID(), stageTask.ID, "issue_scan_stage_output_contract", "application/json", outputContract, causes, convID); err != nil {
		t.Fatalf("AddArtifact stage contract: %v", err)
	}

	if got, ok := l.firstAssignableOpenTask(); ok {
		t.Fatalf("firstAssignableOpenTask returned issue-scan stage aggregator %s (%s); want none", got.ID.Value(), got.Title)
	}

	normalTask, err := ts.Create(agent.ID(), "Implement selected issue-scan patch", "Concrete implementation work", causes, convID)
	if err != nil {
		t.Fatalf("Create normal task: %v", err)
	}
	for _, label := range work.RequiredReadinessGateLabels() {
		if err := ts.AddArtifact(agent.ID(), normalTask.ID, label, "text/markdown", "gate body", causes, convID); err != nil {
			t.Fatalf("AddArtifact normal gate %s: %v", label, err)
		}
	}

	got, ok := l.firstAssignableOpenTask()
	if !ok {
		t.Fatal("firstAssignableOpenTask = false with ready normal task present; want true")
	}
	if got.ID != normalTask.ID {
		t.Fatalf("firstAssignableOpenTask returned %s; want normal task %s", got.ID.Value(), normalTask.ID.Value())
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

	// The replay that backs the gate must make the completion visible AND
	// reviewable on the next observation — otherwise the wake would be a
	// no-op: the agent wakes, renders "No tasks pending review", and parks.
	c, ok := l.reviewerState.completedTasks[taskID.Value()]
	if !ok {
		t.Fatal("historical completion not replayed into reviewerState.completedTasks")
	}
	if c.content.CompletedBy != completer.ID() {
		t.Fatalf("replayed completion lost CompletedBy: got %s, want %s", c.content.CompletedBy.Value(), completer.ID().Value())
	}
	if c.content.ArtifactRef.IsZero() {
		t.Fatal("replayed completion lost ArtifactRef; review git context would degrade to heuristics")
	}
	obs := l.enrichReviewObservation("base obs")
	if !strings.Contains(obs, "TASK UNDER REVIEW") || !strings.Contains(obs, taskID.Value()) {
		t.Fatal("review observation does not render the replayed historical task")
	}
	if strings.Contains(obs, "RECENT COMMIT:\n  (unavailable)") {
		t.Fatal("replayed completion's commit did not resolve; the completion is not actually reviewable")
	}

	// The already-reviewed historical task must NOT be pending — its latest
	// verdict was replayed into reviewHistory exactly as a live bus review
	// would have been recorded, so the uniform findPendingReviews rule
	// excludes it (a restart never re-reviews settled work).
	for _, id := range l.reviewerState.findPendingReviews() {
		if id == reviewedID.Value() {
			t.Fatal("historically-reviewed completion is pending; a restart would re-review settled work")
		}
	}

	// A review recorded THIS session clears the pending state.
	l.reviewerState.recordReview(taskID.Value(), "approve", []string{})
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

// TestCatchUp_ExternalReviewSettlesPending pins the settle half of round-2/3
// finding B-2: a code.review.submitted event from ANOTHER actor that reaches
// the chain after the replay must settle the task via the watermark catch-up,
// so a stale local completion stops pending and the reviewer never re-reviews
// work a peer instance settled — regardless of whether the bus delivered it.
func TestCatchUp_ExternalReviewSettlesPending(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen, "implementer")
	externalReviewer := generationAgent(t, gen, "reviewer")
	taskID := completeNewTaskWithCommit(t, gen, completer, repo, "settle.md", "implement settle target")

	l := newReviewerRecheckLoop(t, gen, repo, 10*time.Millisecond)
	if !l.hasReviewableWork() { // replay: the completion pends
		t.Fatal("fixture: task should be pending before the external review arrives")
	}

	// The external review lands on the chain AFTER the replay.
	reviewAs(t, externalReviewer, taskID, "approve")

	if l.hasReviewableWork() {
		t.Fatalf("external approve on the chain did not settle the task; still pending: %v", l.reviewerState.findPendingReviews())
	}
}

// TestHasReviewableWork_PostSeedStoreOnlyCompletion is the round-3 B-1 pin:
// Work's HTTP server and CLI complete tasks through the SHARED STORE from
// separate binaries — no in-process bus delivery ever reaches the Hive loop
// for those writes. A completion appended after the one-time replay, with no
// onEvent call at all, must still fire the gate via the watermark catch-up.
func TestHasReviewableWork_PostSeedStoreOnlyCompletion(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen, "implementer")

	l := newReviewerRecheckLoop(t, gen, repo, 10*time.Millisecond)
	if l.hasReviewableWork() { // performs the one-time replay over an empty history
		t.Fatal("hasReviewableWork = true on an empty store; want false")
	}

	// A completion lands in the STORE ONLY — the work-server/CLI writer shape.
	// Deliberately no onEvent: the bus never hears about it.
	taskID := completeNewTaskWithCommit(t, gen, completer, repo, "external.md", "implement external target")

	if !l.hasReviewableWork() {
		t.Fatalf("hasReviewableWork = false for a store-only post-seed completion (task %s); external writers are invisible (B-1)", taskID.Value())
	}
	if _, ok := l.reviewerState.completedTasks[taskID.Value()]; !ok {
		t.Fatal("store-only completion not folded into the projection for the next observation")
	}
}

// TestReplayBoundary_ReviewCountedExactlyOnce is the round-3 B-2 pin: an
// event can be visible to the replay walk AND still sit in pendingEvents
// (Graph.Record appends to the store before the bus publishes), so the
// projection must fold each chain event exactly once. The chain watermark is
// the single mutation source — bus payloads never mutate the projection — so
// a review reaching both paths cannot inflate the escalation cap.
func TestReplayBoundary_ReviewCountedExactlyOnce(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen, "implementer")
	externalReviewer := generationAgent(t, gen, "reviewer")
	taskID := completeNewTaskWithCommit(t, gen, completer, repo, "boundary.md", "implement boundary target")
	reviewAs(t, externalReviewer, taskID, "request_changes")

	l := newReviewerRecheckLoop(t, gen, repo, 10*time.Millisecond)
	if !l.hasReviewableWork() { // replay folds the completion AND the request_changes review
		t.Fatal("fixture: request_changes task should be pending after replay")
	}
	if got := l.reviewerState.getReviewCount(taskID.Value()); got != 1 {
		t.Fatalf("replay folded the review %d times; want exactly 1", got)
	}

	// The same review event also arrives via the bus path (it was appended
	// before the bus published — the replay/live boundary). update() must not
	// count it again.
	page, err := s.ByType(event.EventTypeCodeReviewSubmitted, 1, types.None[types.Cursor]())
	if err != nil || len(page.Items()) != 1 {
		t.Fatalf("fetch review event: err=%v items=%d", err, len(page.Items()))
	}
	l.reviewerState.update([]event.Event{page.Items()[0]})

	if got := l.reviewerState.getReviewCount(taskID.Value()); got != 1 {
		t.Fatalf("review counted %d times across the replay/bus boundary; want exactly 1 (escalation cap inflation)", got)
	}
	// And a second catch-up must not re-fold it either.
	l.hasReviewableWork()
	if got := l.reviewerState.getReviewCount(taskID.Value()); got != 1 {
		t.Fatalf("review counted %d times after a second catch-up; want exactly 1", got)
	}
}

// flakySinceStore wraps a store to force tiny Since pages and inject
// failures mid-walk — the only way to prove the catch-up's per-page watermark
// commit and the stale-projection fail-closed behavior.
type flakySinceStore struct {
	store.Store
	pageLimit          int  // overrides the limit passed by the caller
	armFailAfterReview bool // arm: fail the NEXT Since call after a page served a review
	failNext           bool
	failAll            bool
}

func (f *flakySinceStore) Since(afterID types.EventID, limit int) (types.Page[event.Event], error) {
	if f.failAll || f.failNext {
		f.failNext = false
		return types.Page[event.Event]{}, fmt.Errorf("injected Since failure")
	}
	pageLimit := limit
	if f.pageLimit > 0 {
		pageLimit = f.pageLimit
	}
	page, err := f.Store.Since(afterID, pageLimit)
	if err == nil && f.armFailAfterReview {
		for _, ev := range page.Items() {
			if _, ok := ev.Content().(event.CodeReviewContent); ok {
				f.failNext = true
				f.armFailAfterReview = false
			}
		}
	}
	return page, err
}

// TestCatchUp_PerPageWatermarkCommit is the round-4 B-1 pin: the incremental
// Since walk must commit the watermark per folded page, so a failure on a
// LATER page never replays the folds of an earlier one. The wrapper serves
// one event per page and fails the call right after the page that carried the
// peer review; if the watermark only commits at the end of a full walk, the
// retry re-folds that review and the escalation count inflates to 2.
func TestCatchUp_PerPageWatermarkCommit(t *testing.T) {
	base := store.NewInMemoryStore()
	flaky := &flakySinceStore{Store: base, pageLimit: 1}
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, flaky, as, 1)
	completer := generationAgent(t, gen, "implementer")
	externalReviewer := generationAgent(t, gen, "reviewer")

	// Establish a NONZERO watermark first (round-5 finding): a boot replay
	// over an EMPTY store returns success with a zero watermark, and every
	// later evaluation then takes the boot-replay path (Recent) — the armed
	// Since wrapper would never fire and this test would be a false negative
	// for the exact regression it pins. A settled pre-watermark task makes
	// the boot replay run and pin the watermark at the chain head.
	preID := completeNewTaskWithCommit(t, gen, completer, repo, "pre.md", "implement pre-watermark task")
	reviewAs(t, externalReviewer, preID, "approve")

	l := newReviewerRecheckLoop(t, gen, repo, 10*time.Millisecond)
	if l.hasReviewableWork() { // boot replay folds the settled prefix
		t.Fatal("hasReviewableWork = true over a fully settled prefix; want false")
	}
	if l.reviewerState.replayHead.IsZero() {
		t.Fatal("fixture: boot replay left a zero watermark; the incremental Since path would never be exercised")
	}

	// Post-watermark fixtures: a completion, a peer request_changes review,
	// and a second task ensuring more pages exist AFTER the review's page.
	t1 := completeNewTaskWithCommit(t, gen, completer, repo, "page-a.md", "implement page a")
	reviewAs(t, externalReviewer, t1, "request_changes")
	completeNewTaskWithCommit(t, gen, completer, repo, "page-b.md", "implement page b")

	flaky.armFailAfterReview = true
	if l.hasReviewableWork() { // the armed walk must die on the page after the review folded
		t.Fatal("catch-up reported reviewable work although the armed Since walk must fail mid-delta")
	}
	if !l.reviewerState.projectionStale {
		t.Fatal("failed incremental walk did not mark the projection stale; the injected failure never fired")
	}

	if !l.hasReviewableWork() { // healed: resumes from the per-page watermark
		t.Fatal("recovered catch-up found nothing pending; want the request_changes task to pend")
	}
	if got := l.reviewerState.getReviewCount(t1.Value()); got != 1 {
		t.Fatalf("peer review folded %d times across a failed walk; want exactly 1 (per-page watermark commit)", got)
	}
}

// TestStaleProjection_FailsClosed is the round-4 B-2 pin: when the chain
// catch-up fails, the projection is STALE — the observation must say so
// instead of rendering possibly-settled work as pending, and review emission
// must refuse mechanically (a verdict from stale state could re-review
// settled work). Recovery clears the staleness.
func TestStaleProjection_FailsClosed(t *testing.T) {
	base := store.NewInMemoryStore()
	flaky := &flakySinceStore{Store: base}
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, flaky, as, 1)
	completer := generationAgent(t, gen, "implementer")
	taskID := completeNewTaskWithCommit(t, gen, completer, repo, "stale.md", "implement stale target")

	l := newReviewerRecheckLoop(t, gen, repo, 10*time.Millisecond)
	if !l.hasReviewableWork() { // healthy boot replay
		t.Fatal("fixture: the completion should pend after a healthy replay")
	}
	if l.reviewerState.projectionStale {
		t.Fatal("healthy projection marked stale")
	}

	flaky.failAll = true
	if l.catchUpReviewProjection() {
		t.Fatal("catch-up reported success while the store fails")
	}
	if !l.reviewerState.projectionStale {
		t.Fatal("failed catch-up did not mark the projection stale")
	}

	// Mechanical fail-closed at the emission chokepoint.
	err := l.emitCodeReview(&ReviewCommand{TaskID: taskID.Value(), Verdict: "approve", Summary: "s", Issues: []string{}, Confidence: 0.9})
	if err == nil {
		t.Fatal("emitCodeReview succeeded on a stale projection; verdicts must fail closed")
	}
	// The observation declares the staleness instead of rendering stale pendings.
	obs := l.enrichReviewObservation("base obs")
	if !strings.Contains(obs, "PROJECTION STALE") {
		t.Fatal("stale projection not declared in the review observation")
	}
	if strings.Contains(obs, "TASK UNDER REVIEW") {
		t.Fatal("stale observation still renders a task under review; stale pendings must not be presented for verdicts")
	}

	// Recovery clears the staleness and emission works again.
	flaky.failAll = false
	if !l.catchUpReviewProjection() {
		t.Fatal("catch-up failed after the store healed")
	}
	if l.reviewerState.projectionStale {
		t.Fatal("staleness not cleared by a successful catch-up")
	}
	if err := l.emitCodeReview(&ReviewCommand{TaskID: taskID.Value(), Verdict: "approve", Summary: "s", Issues: []string{}, Confidence: 0.9}); err != nil {
		t.Fatalf("emitCodeReview failed on a healthy projection: %v", err)
	}
}

// TestSettledTasksLeaveTheProjection is the round-3 M-1 pin: the projection
// holds only reviewable state, not full history. A task whose latest verdict
// is approve (or reject) leaves completedTasks — an always-on reviewer over a
// long-lived store must not become a permanent full-history in-memory
// projection — and a re-completion brings it back.
func TestSettledTasksLeaveTheProjection(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)
	gen := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen, "implementer")
	gen1Reviewer := generationAgent(t, gen, "reviewer")
	settledID := completeNewTaskWithCommit(t, gen, completer, repo, "settled.md", "implement settled target")
	reviewAs(t, gen1Reviewer, settledID, "approve")
	pendingID := completeNewTaskWithCommit(t, gen, completer, repo, "pending.md", "implement pending target")

	gen2 := newStoreGeneration(t, s, as, 2)
	l := newReviewerRecheckLoop(t, gen2, repo, 10*time.Millisecond)
	if !l.hasReviewableWork() {
		t.Fatal("fixture: the unreviewed completion should pend")
	}
	if _, resident := l.reviewerState.completedTasks[settledID.Value()]; resident {
		t.Fatal("settled (approved) task is resident in the projection; full history would accumulate unbounded (M-1)")
	}
	if _, resident := l.reviewerState.completedTasks[pendingID.Value()]; !resident {
		t.Fatal("pending task missing from the projection")
	}

	// A re-completion of the settled task re-enters the projection. It does
	// not re-pend (latest verdict approve — the pre-existing live rule), but
	// the projection must reflect the latest completion again.
	completeExistingTaskWithCommit(t, gen, completer, repo, "settled-round2.md", settledID)
	l.hasReviewableWork()
	if _, resident := l.reviewerState.completedTasks[settledID.Value()]; !resident {
		t.Fatal("re-completed task did not re-enter the projection")
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
	st.completedTasks[taskID.Value()] = completedRecord{content: work.TaskCompletedContent{TaskID: taskID}}
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

// TestReplayChainPrefix_FollowsCursors pins the boot-time replay walk
// (round-1 finding B-2; round-2 finding B-1): every page of the store's
// globally-ordered Recent feed is walked via cursors — chain position, not
// wall-clock timestamps, decides order — and the collected prefix comes back
// oldest-first, ready to fold exactly as live delivery would have. A tiny
// page size forces many pages over the interleaved event history.
func TestReplayChainPrefix_FollowsCursors(t *testing.T) {
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	repo := newTempGitRepo(t)

	gen1 := newStoreGeneration(t, s, as, 1)
	completer := generationAgent(t, gen1, "implementer")
	gen1Reviewer := generationAgent(t, gen1, "reviewer")
	const total = 7
	ids := make([]types.EventID, 0, total)
	for i := 0; i < total; i++ {
		ids = append(ids, completeNewTaskWithCommit(t, gen1, completer, repo,
			fmt.Sprintf("page-%d.md", i), fmt.Sprintf("implement page %d", i)))
	}
	reviewAs(t, gen1Reviewer, ids[0], "approve")

	headOpt, err := s.Head()
	if err != nil || headOpt.IsNone() {
		t.Fatalf("Head: err=%v none=%v", err, headOpt.IsNone())
	}
	events, ok := replayChainPrefix(s, headOpt.Unwrap().ID(), 3)
	if !ok {
		t.Fatal("replayChainPrefix reported failure on a healthy store")
	}

	var completions, reviews int
	for _, ev := range events {
		switch ev.Content().(type) {
		case work.TaskCompletedContent:
			completions++
		case event.CodeReviewContent:
			reviews++
		}
	}
	if completions != total || reviews != 1 {
		t.Fatalf("replay collected %d completions and %d reviews across pages; want %d and 1 (cursors must be followed)", completions, reviews, total)
	}
	// Oldest-first: the first collected completion is the chain-oldest task.
	first, isCompletion := events[0].Content().(work.TaskCompletedContent)
	if !isCompletion || first.TaskID != ids[0] {
		t.Fatalf("replay prefix is not oldest-first: first event is %T for %v, want completion of %s", events[0].Content(), events[0].ID().Value(), ids[0].Value())
	}

	// The watermark partition: a head older than the newest event excludes
	// everything newer than it, so replay and catch-up never overlap.
	page, err := s.ByType(work.EventTypeTaskCompleted, 1, types.None[types.Cursor]())
	if err != nil || len(page.Items()) != 1 {
		t.Fatalf("fetch newest completion: err=%v", err)
	}
	newestCompletion := page.Items()[0]
	older, ok := replayChainPrefix(s, newestCompletion.ID(), 3)
	if !ok {
		t.Fatal("replayChainPrefix failed for an interior watermark")
	}
	for _, ev := range older {
		if _, isReview := ev.Content().(event.CodeReviewContent); isReview {
			t.Fatal("replay prefix included the review appended after the interior watermark; the partition leaks")
		}
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

// TestBuildTaskContextRendersDemand guards the codex finding on #150: the
// completion-discipline contract binds agents to "the form the task demands",
// but buildTaskContext rendered only [status] UUID: Title — descriptions and
// readiness gates were invisible, so the criterion was unevaluable from a
// reasoning prompt (and the v8 strategist truthfully escalated "seed task has
// no description" about a task carrying a 6287-char spec). The task list must
// render a bounded, rune-safe demand excerpt and the readiness state.
func TestBuildTaskContextRendersDemand(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, true, time.Hour)
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}

	// Long multibyte description: truncation must never split a rune.
	longDesc := strings.Repeat("é", 300) + " produce dark-factory/fo_roles_catalog.md in the repository"
	if _, err := ts.Create(agent.ID(), "authoritative roles catalog", longDesc, causes, convID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ctx := l.buildTaskContext()
	if !strings.Contains(ctx, "demand:") {
		t.Fatal("task context renders no demand excerpt; the completion-discipline criterion is unevaluable")
	}
	if !strings.Contains(ctx, "é") {
		t.Fatal("demand excerpt lost the description content")
	}
	if strings.Contains(ctx, "fo_roles_catalog.md") {
		t.Fatal("demand excerpt not truncated (300-rune prefix should have cut the tail)")
	}
	if !utf8.ValidString(ctx) {
		t.Fatal("task context is not valid UTF-8; truncation split a rune (the v9-F1 class)")
	}
	if !strings.Contains(ctx, "missing gates:") {
		t.Fatal("task context does not render readiness state for a gateless implementation task")
	}

	// Structured demand (codex re-review of #150): ExpectedOutputs is the
	// precise artifact demand when the order carries one — it must render even
	// though the description excerpt may truncate before naming the file. A
	// fact requirement must surface as missing facts, not vanish.
	if _, err := ts.CreateV39(agent.ID(), work.TaskCreateOptions{
		Title:           "catalog with structured outputs",
		Description:     "short",
		ExpectedOutputs: []string{"dark-factory/fo_roles_catalog.md"},
	}, causes, convID); err != nil {
		t.Fatalf("CreateWithOptions: %v", err)
	}
	ctx = l.buildTaskContext()
	if !strings.Contains(ctx, "expected outputs: dark-factory/fo_roles_catalog.md") {
		t.Fatal("task context does not render ExpectedOutputs; the structured demand stays invisible")
	}
}

func TestBuildTaskContextRendersIssueScanRoleOutputContract(t *testing.T) {
	l, ts, agent, convID := newRecheckLoop(t, false, time.Hour)
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}

	longDesc := strings.Repeat("generic lifecycle stage context ", 40)
	task, err := ts.CreateV39(agent.ID(), work.TaskCreateOptions{
		Title:           "Issue-scan stage: Research issue and repo context",
		Description:     longDesc,
		ExpectedOutputs: []string{"stage declaration artifact remains pending runtime evidence"},
	}, causes, convID)
	if err != nil {
		t.Fatalf("CreateV39: %v", err)
	}
	outputContract := `{
  "kind": "issue_scan_stage_output_contract",
  "run_id": "run_issue_001",
  "factory_order_id": "fo_issue_001",
  "stage_id": "research_issue_and_repo_context",
  "stage_index": 1,
  "stage_count": 5,
  "stage": {
    "id": "research_issue_and_repo_context",
    "required_roles": ["strategist", "planner"],
    "required_evidence": ["repo_context_packet", "issue_intake_packet"],
    "authority_boundary": "research only; no implementation, merge, or deploy",
    "completion_gate": "all declared role outputs and evidence refs are recorded"
  },
  "required_evidence": ["repo_context_packet", "issue_intake_packet"],
  "role_output_contracts": [
    {
      "role": "strategist",
      "required_outputs": ["strategy_brief", "repo_context_packet"],
      "authority_boundary": "strategy only",
      "completion_gate": "artifact evidence refs"
    },
    {
      "role": "planner",
      "required_outputs": ["plan_packet"]
    }
  ]
}`
	if err := ts.AddArtifact(agent.ID(), task.ID, "issue_scan_stage_output_contract", "application/json", outputContract, causes, convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}

	ctx := l.buildTaskContext()
	want := []string{
		"issue-scan contract: run run_issue_001, FactoryOrder fo_issue_001, stage research_issue_and_repo_context (1/5)",
		"issue-scan required evidence: repo_context_packet, issue_intake_packet",
		"issue-scan boundary: authority research only; no implementation, merge, or deploy; gate all declared role outputs and evidence refs are recorded",
		"issue-scan your role (strategist) outputs: strategy_brief, repo_context_packet",
		"issue-scan role artifact: attach label issue_scan_stage_role_output with role=strategist",
	}
	for _, fragment := range want {
		if !strings.Contains(ctx, fragment) {
			t.Fatalf("task context missing %q\ncontext:\n%s", fragment, ctx)
		}
	}
	if strings.Contains(ctx, "plan_packet") {
		t.Fatalf("task context leaked another role's output contract to strategist:\n%s", ctx)
	}
}
