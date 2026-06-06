package loop

import (
	"context"
	"os/exec"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/work"
)

// ── claimsCommit: detect affirmative commit assertions in an Operate summary ──
//
// The commit-verification gate exists because, on the 2026-06-06 autonomous run,
// the implementer reported "committed in 89f8886" while the intended repo HEAD
// never advanced (the commit landed in the wrong repo). The detector must flag
// affirmative commit claims while leaving honest no-ops alone.
func TestClaimsCommit(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		want    bool
	}{
		{"real confab hash", "Wrote the catalog in docs/civilization-roles.md, committed in 89f8886", true},
		{"ran git commit", "Created the file and ran git commit -m 'docs: add catalog'", true},
		{"past tense committed", "Successfully committed the changes to the branch.", true},
		{"created a commit", "I created a commit with the new documentation.", true},
		{"nothing to commit", "Nothing to commit, working tree clean.", false},
		{"no changes to commit", "No changes to commit; the file already matches.", false},
		{"did not commit", "I did not commit because there was nothing to change.", false},
		{"no work claim", "Analyzed the directory structure; made no changes.", false},
		{"idle signal", `/signal {"signal":"IDLE"}`, false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimsCommit(tt.summary); got != tt.want {
				t.Errorf("claimsCommit(%q) = %v, want %v", tt.summary, got, tt.want)
			}
		})
	}
}

// ── classifyOperateCommit: HEAD movement + self-report + working-tree state ──
func TestClassifyOperateCommit(t *testing.T) {
	tests := []struct {
		name      string
		pre, post string
		advanced  bool
		summary   string
		dirty     bool
		want      commitVerdict
	}{
		{"advance: real commit", "aaaa", "bbbb", true, "committed in bbbb", false, commitVerified},
		{"advance: terse summary", "aaaa", "bbbb", true, "done", false, commitVerified},
		// A commit that advances HEAD but leaves the tree dirty is a PARTIAL
		// commit: uncommitted side effects the agent never captured.
		{"advance but tree dirty fails closed", "aaaa", "bbbb", true, "done", true, commitDirty},
		{"advance + commit claim + dirty fails closed", "aaaa", "bbbb", true, "committed", true, commitDirty},
		// HEAD moved but NOT to a descendant — reset / branch switch / history
		// rewrite. "HEAD changed" is not "HEAD advanced".
		{"diverged: moved but not a descendant", "aaaa", "cccc", false, "done", false, commitDiverged},
		{"diverged: claim does not rescue a non-advance", "aaaa", "cccc", false, "committed in cccc", false, commitDiverged},
		{"diverged beats dirty", "aaaa", "cccc", false, "done", true, commitDiverged},
		{"confab: claims commit, HEAD unmoved", "aaaa", "aaaa", false, "committed in 89f8886", false, commitConfabulated},
		{"confab: claim beats dirty", "aaaa", "aaaa", false, "committed in 89f8886", true, commitConfabulated},
		{"confab: wrong-repo commit, HEAD unmoved", "aaaa", "aaaa", false, "ran git commit -m docs", false, commitConfabulated},
		{"dirty: edited but never committed, no claim", "aaaa", "aaaa", false, "Updated the catalog", true, commitDirty},
		{"honest no-op: no claim, clean, HEAD unmoved", "aaaa", "aaaa", false, "nothing to commit", false, commitWaivable},
		{"honest no-op: terse, clean", "aaaa", "aaaa", false, "no changes needed", false, commitWaivable},
		// Unverifiable repo HEAD (git unreadable) must fail closed.
		{"unverifiable HEAD fails closed", "aaaa", "", false, "committed in deadbeef", false, commitUnverifiable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyOperateCommit(tt.pre, tt.post, tt.advanced, tt.summary, tt.dirty); got != tt.want {
				t.Errorf("classifyOperateCommit(%q,%q,adv=%v,%q,dirty=%v) = %v, want %v", tt.pre, tt.post, tt.advanced, tt.summary, tt.dirty, got, tt.want)
			}
		})
	}
}

// ── Loop wiring: a confabulated Operate must NOT complete the task ──
func TestHandleOperateResult_ConfabulatedCommitDoesNotComplete(t *testing.T) {
	repo := newTempGitRepo(t)
	head := gitCommand(repo, "rev-parse", "HEAD")

	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_confab_test")

	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the civilization roles catalog", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// The agent claims a commit, but RepoPath HEAD did not move (no commit made).
	completed := l.handleOperateResult(context.Background(), task, head, true,
		"Wrote docs/civilization-roles.md and committed in 89f8886")

	if completed {
		t.Fatal("handleOperateResult returned completed=true for a confabulated commit")
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("confabulated Operate produced a work.task.completed event; the gate must refuse to complete")
	}
}

// An Operate that edits files but never commits (and never claims a commit) must
// NOT be waived+completed — unreviewable filesystem side effects must fail.
func TestHandleOperateResult_DirtyUncommittedDoesNotComplete(t *testing.T) {
	repo := newTempGitRepo(t)
	head := gitCommand(repo, "rev-parse", "HEAD")
	// Simulate Operate editing a file without committing.
	if err := exec.Command("bash", "-c", "echo edited > "+repo+"/notes.md").Run(); err != nil {
		t.Fatalf("dirty the repo: %v", err)
	}

	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_dirty_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write notes", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Summary describes work but does not claim a commit; HEAD did not move; tree dirty.
	completed := l.handleOperateResult(context.Background(), task, head, true, "Updated notes.md with the new content.")
	if completed {
		t.Fatal("handleOperateResult completed a task with uncommitted working-tree changes")
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("dirty uncommitted Operate produced a work.task.completed event")
	}
}

// A genuine no-op (no commit claim, clean tree, HEAD unmoved) still completes via
// waiver, so the gate does not over-correct and stall legitimate work.
func TestHandleOperateResult_HonestNoOpStillCompletes(t *testing.T) {
	repo := newTempGitRepo(t)
	head := gitCommand(repo, "rev-parse", "HEAD")
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_noop_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Check the catalog", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	completed := l.handleOperateResult(context.Background(), task, head, true,
		"Nothing to commit; the catalog already exists.")
	if !completed {
		t.Fatal("handleOperateResult returned completed=false for an honest no-op; it should waive and complete")
	}
	if !taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("honest no-op Operate did not produce a work.task.completed event")
	}
}

// P1a: on commit-verification failure the loop must short-circuit (stop +
// escalate) and NOT process the untrusted Operate summary for /task, /signal,
// etc. The confabulated summary below embeds a TASK_DONE signal; if the loop
// processed it, Run would return StopTaskDone instead of StopEscalation.
func TestRun_ConfabulatedOperateDoesNotProcessUntrustedResponse(t *testing.T) {
	repo := newTempGitRepo(t)
	op := &mockOperator{
		reasonResp:     `/signal {"signal":"IDLE"}`,
		operateSummary: "Wrote the file and committed in deadbeef.\n/signal {\"signal\":\"TASK_DONE\"}",
	}
	agent, g := agentWithGraph(t, op)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_p1a_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the doc", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ts.Assign(agent.ID(), task.ID, agent.ID(), causes, convID); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	l, err := New(Config{
		Agent:      agent,
		HumanID:    humanID(),
		RepoPath:   repo,
		TaskStore:  ts,
		CanOperate: true,
		Budget:     resources.BudgetConfig{MaxIterations: 5},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := l.Run(context.Background())
	if res.Reason != StopEscalation {
		t.Fatalf("confabulated Operate: reason = %s, want StopEscalation (embedded TASK_DONE must not be processed; detail=%q)", res.Reason, res.Detail)
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("confabulated Operate produced a work.task.completed event")
	}
}

// #2: an unverifiable repo (RepoPath is not a git checkout, or git is
// unavailable) must fail closed, never silently waive+complete. gitCommand
// returns "" on any error, which previously fell through to commitWaivable.
func TestHandleOperateResult_UnverifiableRepoDoesNotComplete(t *testing.T) {
	nonRepo := t.TempDir() // a directory that is NOT a git repository
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_unverifiable_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write something", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: nonRepo, TaskStore: ts, ConvID: convID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No commit claim + a non-git repo: HEAD is unreadable, so the gate cannot
	// verify the Operate. It must refuse to complete, not waive.
	completed := l.handleOperateResult(context.Background(), task, "priorhead", true, "Analyzed the directory; made some edits.")
	if completed {
		t.Fatal("handleOperateResult completed a task in an unverifiable (non-git) repo")
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("unverifiable repo produced a work.task.completed event")
	}
}

// #4: failing an Operate must reflect in Work task state — a blocked (retryable)
// task — not leave it looking assigned/in-progress. Loop tasks start at
// StatusCreated (the loop never enters the v3.9 lifecycle), so the fail path
// must advance Created→Ready→Running→Blocked.
func TestFailOperateTask_TransitionsTaskToBlocked(t *testing.T) {
	repo := newTempGitRepo(t)
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_block_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the doc", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts, ConvID: convID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.failOperateTask(context.Background(), task, "commit verification failed (test)")

	status, err := ts.GetStatus(task.ID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status != work.StatusBlocked {
		t.Fatalf("after failOperateTask, task status = %s, want %s", status, work.StatusBlocked)
	}
}

// #P1-B: "HEAD moved" is not "HEAD advanced". A destructive backward reset
// (or branch jump / history rewrite) changes HEAD to a non-descendant; with a
// clean tree the old gate marked it verified. An autonomy guard must fail closed
// on non-ancestor movement, not record it as a real commit.
func TestHandleOperateResult_HeadResetDoesNotVerify(t *testing.T) {
	repo := newTempGitRepo(t)
	// Advance HEAD: init -> c2. Capture c2 as the pre-Operate HEAD.
	if err := exec.Command("bash", "-c", "cd "+repo+" && git commit -q --allow-empty -m c2").Run(); err != nil {
		t.Fatalf("make c2: %v", err)
	}
	preHead := gitCommand(repo, "rev-parse", "HEAD")
	// Simulate Operate destructively resetting HEAD backward to the init commit.
	if err := exec.Command("bash", "-c", "cd "+repo+" && git reset --hard HEAD~1").Run(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if preHead == gitCommand(repo, "rev-parse", "HEAD") {
		t.Fatal("setup: HEAD did not change after reset")
	}

	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_reset_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the doc", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts, ConvID: convID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// HEAD moved (preHead -> older commit) but the new HEAD is NOT a descendant of
	// preHead — a backward reset. The gate must NOT verify it as a commit.
	completed := l.handleOperateResult(context.Background(), task, preHead, true, "done")
	if completed {
		t.Fatal("handleOperateResult verified a backward HEAD reset as a real commit")
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("backward HEAD reset produced a work.task.completed event")
	}
}

// Caution from review: the pre-Operate HEAD capture is best-effort. If the
// baseline could not be read, a post-Operate commit cannot be confirmed as a
// genuine advance (preflight failure must not masquerade as a "first commit").
func TestHandleOperateResult_UnreadablePreHeadFailsClosed(t *testing.T) {
	repo := newTempGitRepo(t)
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_pre_unreadable")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the doc", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts, ConvID: convID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// preHeadReadable=false: the baseline HEAD was unreadable. Even with a clean
	// post-state, the gate must fail closed rather than assume a first commit.
	completed := l.handleOperateResult(context.Background(), task, "", false, "committed the file")
	if completed {
		t.Fatal("handleOperateResult completed against an unreadable pre-Operate HEAD")
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("unreadable pre-Operate HEAD produced a work.task.completed event")
	}
}

// isOperableStatus must exclude blocked/terminal v3.9 statuses so a failed
// (blocked) task is not re-Operated, while keeping the live working states.
func TestIsOperableStatus(t *testing.T) {
	notOperable := []work.TaskStatus{
		work.StatusBlocked, work.StatusPolicyBlocked, work.StatusFailed,
		work.StatusRejected, work.StatusSuperseded, work.StatusVerified, work.StatusCertified,
	}
	for _, s := range notOperable {
		if isOperableStatus(s) {
			t.Errorf("isOperableStatus(%s) = true, want false (blocked/terminal must not be operable)", s)
		}
	}
	operable := []work.TaskStatus{
		work.StatusCreated, work.StatusReady, work.StatusRunning, work.StatusRepaired,
	}
	for _, s := range operable {
		if !isOperableStatus(s) {
			t.Errorf("isOperableStatus(%s) = false, want true (live working states must be operable)", s)
		}
	}
}

// #P1-A: blocking a task must be a real EXECUTION barrier, not a dashboard
// label. After a commit-verification failure blocks the task, the implementer
// loop must not treat it as runnable — otherwise a restart re-Operates the same
// blocked task and the human-review barrier is not durable.
func TestHasAssignedTask_ExcludesBlockedTask(t *testing.T) {
	repo := newTempGitRepo(t)
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_blocked_excl")
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
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts, CanOperate: true, ConvID: convID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Sanity: an assigned, non-blocked task IS operable.
	if !l.hasAssignedTask() {
		t.Fatal("assigned non-blocked task should be operable before blocking")
	}

	// Block it (the commit-verification failure path).
	if err := l.blockTaskForFailure(task, "verification failed"); err != nil {
		t.Fatalf("blockTaskForFailure: %v", err)
	}

	if l.hasAssignedTask() {
		t.Fatal("blocked task is still reported operable — the human-review barrier is not durable")
	}
	if id := l.nextAssignedTask().ID; id.Value() != "" {
		t.Fatalf("nextAssignedTask returned a blocked task (%s); blocked tasks must be excluded", id.Value())
	}
}

// #P1-A: a subsequent Run with CanOperate=true must NOT call Operate for a
// blocked task until an explicit unblock transition makes it runnable again.
func TestRun_BlockedTaskIsNotOperated(t *testing.T) {
	repo := newTempGitRepo(t)
	op := &countingOperator{reasonResp: `/signal {"signal":"IDLE"}`, operateSummary: `/signal {"signal":"IDLE"}`}
	agent, g := agentWithGraph(t, op)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_blocked_not_operated")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Blocked work", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ts.Assign(agent.ID(), task.ID, agent.ID(), causes, convID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	l, err := New(Config{
		Agent: agent, HumanID: humanID(), RepoPath: repo, TaskStore: ts,
		CanOperate: true, ConvID: convID, Budget: resources.BudgetConfig{MaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Block the task (simulate a prior commit-verification failure).
	if err := l.blockTaskForFailure(task, "prior verification failure"); err != nil {
		t.Fatalf("blockTaskForFailure: %v", err)
	}

	l.Run(context.Background())

	if op.operateCalls != 0 {
		t.Fatalf("blocked task was Operated %d time(s); a blocked task must not be re-operated without an explicit unblock", op.operateCalls)
	}
}

// ── helpers ──

// countingOperator records how many times Operate is invoked, to prove the loop
// does not Operate a blocked task.
type countingOperator struct {
	reasonResp     string
	operateSummary string
	operateCalls   int
}

func (m *countingOperator) Name() string  { return "countop" }
func (m *countingOperator) Model() string { return "countop-model" }
func (m *countingOperator) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	c, _ := types.NewScore(0.8)
	return decision.NewResponse(m.reasonResp, c, decision.TokenUsage{InputTokens: 10, OutputTokens: 10}), nil
}
func (m *countingOperator) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	m.operateCalls++
	return decision.OperateResult{Summary: m.operateSummary, Usage: decision.TokenUsage{InputTokens: 10, OutputTokens: 10}}, nil
}

var (
	_ intelligence.Provider = (*countingOperator)(nil)
	_ decision.IOperator    = (*countingOperator)(nil)
)

// newTempGitRepo creates an isolated, clean git repo with one commit.
func newTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// mockOperator is a provider that supports both Reason and Operate, with a
// controllable Operate summary (no filesystem side effects).
type mockOperator struct {
	reasonResp     string
	operateSummary string
}

func (m *mockOperator) Name() string  { return "mockop" }
func (m *mockOperator) Model() string { return "mockop-model" }
func (m *mockOperator) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	c, _ := types.NewScore(0.8)
	return decision.NewResponse(m.reasonResp, c, decision.TokenUsage{InputTokens: 10, OutputTokens: 10}), nil
}
func (m *mockOperator) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	return decision.OperateResult{Summary: m.operateSummary, Usage: decision.TokenUsage{InputTokens: 10, OutputTokens: 10}}, nil
}

var (
	_ intelligence.Provider = (*mockOperator)(nil)
	_ decision.IOperator    = (*mockOperator)(nil)
)

// taskHasCompletedEvent reports whether a work.task.completed event exists for taskID.
func taskHasCompletedEvent(t *testing.T, g *graph.Graph, taskID types.EventID) bool {
	t.Helper()
	page, err := g.Store().ByType(work.EventTypeTaskCompleted, 1000, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	for _, ev := range page.Items() {
		if c, ok := ev.Content().(work.TaskCompletedContent); ok && c.TaskID == taskID {
			return true
		}
	}
	return false
}
