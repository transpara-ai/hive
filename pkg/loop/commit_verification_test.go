package loop

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
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

// ── classifyOperateCommit: combine HEAD movement with the self-report ──
func TestClassifyOperateCommit(t *testing.T) {
	tests := []struct {
		name      string
		pre, post string
		summary   string
		want      commitVerdict
	}{
		{"real commit advances HEAD", "aaaa", "bbbb", "committed in bbbb", commitVerified},
		{"real commit, terse summary", "aaaa", "bbbb", "done", commitVerified},
		{"first commit in fresh repo", "", "bbbb", "committed", commitVerified},
		{"confab: claims commit, HEAD unmoved", "aaaa", "aaaa", "committed in 89f8886", commitConfabulated},
		{"confab: wrong-repo commit, HEAD unmoved", "aaaa", "aaaa", "ran git commit -m docs", commitConfabulated},
		{"honest no-op: no claim, HEAD unmoved", "aaaa", "aaaa", "nothing to commit", commitWaivable},
		{"honest no-op: terse, HEAD unmoved", "aaaa", "aaaa", "no changes needed", commitWaivable},
		{"unverifiable HEAD + commit claim fails closed", "aaaa", "", "committed in deadbeef", commitConfabulated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyOperateCommit(tt.pre, tt.post, tt.summary); got != tt.want {
				t.Errorf("classifyOperateCommit(%q,%q,%q) = %v, want %v", tt.pre, tt.post, tt.summary, got, tt.want)
			}
		})
	}
}

// ── Loop wiring: a confabulated Operate must NOT complete the task ──
func TestHandleOperateResult_ConfabulatedCommitDoesNotComplete(t *testing.T) {
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}

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

	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: ".", TaskStore: ts})
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

// A genuine no-op (no commit claim, HEAD unmoved) still completes via waiver,
// so the gate does not over-correct and stall legitimate non-committing work.
func TestHandleOperateResult_HonestNoOpStillCompletes(t *testing.T) {
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}
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
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: ".", TaskStore: ts})
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
