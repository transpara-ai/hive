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
		summary   string
		dirty     bool
		want      commitVerdict
	}{
		{"real commit advances HEAD", "aaaa", "bbbb", "committed in bbbb", false, commitVerified},
		{"real commit, terse summary", "aaaa", "bbbb", "done", false, commitVerified},
		{"first commit in fresh repo", "", "bbbb", "committed", false, commitVerified},
		// A commit that advances HEAD but leaves the tree dirty is a PARTIAL
		// commit: the agent produced uncommitted side effects it never captured.
		// An autonomy guard must not treat that as cleanly verified.
		{"HEAD moved but tree dirty fails closed", "aaaa", "bbbb", "done", true, commitDirty},
		{"HEAD moved + commit claim + dirty fails closed", "aaaa", "bbbb", "committed", true, commitDirty},
		{"confab: claims commit, HEAD unmoved", "aaaa", "aaaa", "committed in 89f8886", false, commitConfabulated},
		{"confab: claim beats dirty", "aaaa", "aaaa", "committed in 89f8886", true, commitConfabulated},
		{"confab: wrong-repo commit, HEAD unmoved", "aaaa", "aaaa", "ran git commit -m docs", false, commitConfabulated},
		{"dirty: edited but never committed, no claim", "aaaa", "aaaa", "Updated the catalog", true, commitDirty},
		{"honest no-op: no claim, clean, HEAD unmoved", "aaaa", "aaaa", "nothing to commit", false, commitWaivable},
		{"honest no-op: terse, clean", "aaaa", "aaaa", "no changes needed", false, commitWaivable},
		// Unverifiable repo HEAD (git unreadable) must fail closed regardless of
		// the summary — both when a commit is claimed and when none is.
		{"unverifiable HEAD + commit claim fails closed", "aaaa", "", "committed in deadbeef", false, commitUnverifiable},
		{"unverifiable HEAD, no claim, fails closed", "aaaa", "", "edited some files", false, commitUnverifiable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyOperateCommit(tt.pre, tt.post, tt.summary, tt.dirty); got != tt.want {
				t.Errorf("classifyOperateCommit(%q,%q,%q,dirty=%v) = %v, want %v", tt.pre, tt.post, tt.summary, tt.dirty, got, tt.want)
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
	completed := l.handleOperateResult(context.Background(), task, head,
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
	completed := l.handleOperateResult(context.Background(), task, head, "Updated notes.md with the new content.")
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
	completed := l.handleOperateResult(context.Background(), task, head,
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
	completed := l.handleOperateResult(context.Background(), task, "priorhead", "Analyzed the directory; made some edits.")
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

// ── helpers ──

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
