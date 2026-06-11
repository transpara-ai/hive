package loop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// initGitRepoAt creates a git repo with one commit at parent/name (the
// sibling-layout analog of newTempGitRepo, which owns its own TempDir).
func initGitRepoAt(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
	}
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// containmentLoopFixture builds a Loop whose workspace and one sibling share a
// parent directory (the default watch root), plus a task to fail on violation.
func containmentLoopFixture(t *testing.T) (l *Loop, task work.Task, ws, sibling string, completed func() bool) {
	t.Helper()
	parent := t.TempDir()
	ws = initGitRepoAt(t, parent, "workspace")
	sibling = initGitRepoAt(t, parent, "sibling")

	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_containment_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the catalog", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err = New(Config{Agent: agent, HumanID: humanID(), RepoPath: ws, TaskStore: ts})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l, task, ws, sibling, func() bool { return taskHasCompletedEvent(t, g, task.ID) }
}

func TestContainmentRootsDefaultToWorkspaceParent(t *testing.T) {
	l := &Loop{config: Config{RepoPath: "/data/repos/workspace"}}
	got := l.containmentRoots()
	if len(got) != 1 || got[0] != "/data/repos" {
		t.Fatalf("default containmentRoots = %v, want [/data/repos]", got)
	}

	l = &Loop{config: Config{RepoPath: "/data/repos/workspace", ContainmentWatchRoots: []string{"/a", "/b"}}}
	got = l.containmentRoots()
	if len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Fatalf("configured containmentRoots = %v, want [/a /b]", got)
	}

	l = &Loop{config: Config{}}
	if got := l.containmentRoots(); got != nil {
		t.Fatalf("containmentRoots with no workspace = %v, want nil", got)
	}
}

// The workspace itself must be excluded from the sibling watch — including
// when it is addressed through a symlink (the repos tree is reached via
// symlinks on the dev host; a workspace watched as its own sibling would
// self-trip on every legitimate commit).
func TestSnapshotContainmentExcludesWorkspaceIncludingSymlinkAlias(t *testing.T) {
	parent := t.TempDir()
	ws := initGitRepoAt(t, parent, "workspace")
	sib := initGitRepoAt(t, parent, "sibling")

	base, ok := snapshotContainment([]string{parent}, ws)
	if !ok {
		t.Fatal("snapshotContainment failed on a readable layout")
	}
	sibResolved, _ := resolvePath(sib)
	wsResolved, _ := resolvePath(ws)
	if _, watched := base[wsResolved]; watched {
		t.Error("workspace must not be watched as its own sibling")
	}
	if _, watched := base[sibResolved]; !watched {
		t.Error("sibling checkout missing from the baseline")
	}

	linkDir := t.TempDir()
	alias := filepath.Join(linkDir, "ws-link")
	if err := os.Symlink(ws, alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	base, ok = snapshotContainment([]string{parent}, alias)
	if !ok {
		t.Fatal("snapshotContainment failed with symlinked workspace")
	}
	if _, watched := base[wsResolved]; watched {
		t.Error("workspace addressed via symlink must still be excluded from the watch")
	}
}

func TestSnapshotContainmentUnreadableRootFailsClosed(t *testing.T) {
	parent := t.TempDir()
	ws := initGitRepoAt(t, parent, "workspace")
	if _, ok := snapshotContainment([]string{filepath.Join(parent, "no-such-root")}, ws); ok {
		t.Fatal("snapshotContainment must fail closed on an unreadable watch root")
	}
}

func TestDiffContainment(t *testing.T) {
	clean := repoContainmentState{head: "aaaa", branch: "main", status: ""}
	tests := []struct {
		name       string
		pre, post  containmentBaseline
		violations int
		contains   string
	}{
		{"identical states", containmentBaseline{"/r": clean}, containmentBaseline{"/r": clean}, 0, ""},
		{"pre-existing dirt unchanged does not trip",
			containmentBaseline{"/r": {head: "aaaa", branch: "main", status: " M notes.md"}},
			containmentBaseline{"/r": {head: "aaaa", branch: "main", status: " M notes.md"}}, 0, ""},
		{"HEAD moved",
			containmentBaseline{"/r": clean},
			containmentBaseline{"/r": {head: "bbbb", branch: "main", status: ""}}, 1, "HEAD moved"},
		{"branch switched (same HEAD)",
			containmentBaseline{"/r": clean},
			containmentBaseline{"/r": {head: "aaaa", branch: "feat/x", status: ""}}, 1, "branch switched"},
		{"working tree dirtied",
			containmentBaseline{"/r": clean},
			containmentBaseline{"/r": {head: "aaaa", branch: "main", status: "?? new.md"}}, 1, "working tree changed"},
		{"checkout vanished", containmentBaseline{"/r": clean}, containmentBaseline{}, 1, "vanished"},
		{"checkout appeared", containmentBaseline{}, containmentBaseline{"/r": clean}, 1, "appeared"},
		{"compound mutation reports every difference",
			containmentBaseline{"/r": clean},
			containmentBaseline{"/r": {head: "bbbb", branch: "feat/x", status: "?? new.md"}}, 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffContainment(tt.pre, tt.post)
			if len(got) != tt.violations {
				t.Fatalf("violations = %v, want %d (%v)", got, tt.violations, got)
			}
			if tt.contains != "" && (len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), tt.contains)) {
				t.Fatalf("violations %v missing %q", got, tt.contains)
			}
		})
	}
}

// The v10 round-3 escape shape: the Operate commits in a SIBLING checkout.
// The tripwire must fail the task and never let it complete.
func TestVerifyOperateContainmentSiblingCommitFailsTask(t *testing.T) {
	l, task, _, sibling, completed := containmentLoopFixture(t)

	pre, ok := snapshotContainment(l.containmentRoots(), l.config.RepoPath)
	if !ok {
		t.Fatal("baseline snapshot failed")
	}

	// The Operate walks to the sibling and commits there.
	if err := os.WriteFile(filepath.Join(sibling, "escape.md"), []byte("escaped\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	gitIn(t, sibling, "add", "escape.md")
	gitIn(t, sibling, "commit", "-q", "-m", "escaped work")

	if l.verifyOperateContainment(context.Background(), task, pre) {
		t.Fatal("verifyOperateContainment passed a sibling commit — the v10-F2 escape shape")
	}
	if completed() {
		t.Fatal("a containment-violating Operate produced a work.task.completed event")
	}
}

// A sibling branch switch with no new commit (HEAD hash returns elsewhere)
// is the round-3 precursor move — must trip.
func TestVerifyOperateContainmentSiblingBranchSwitchFailsTask(t *testing.T) {
	l, task, _, sibling, completed := containmentLoopFixture(t)

	pre, ok := snapshotContainment(l.containmentRoots(), l.config.RepoPath)
	if !ok {
		t.Fatal("baseline snapshot failed")
	}
	gitIn(t, sibling, "checkout", "-q", "-b", "feat/escaped")

	if l.verifyOperateContainment(context.Background(), task, pre) {
		t.Fatal("verifyOperateContainment passed a sibling branch switch")
	}
	if completed() {
		t.Fatal("a containment-violating Operate produced a work.task.completed event")
	}
}

// Workspace-only mutations are the run's legitimate work: no false positive,
// and pre-existing sibling dirt that stays identical does not trip.
func TestVerifyOperateContainmentWorkspaceWorkPasses(t *testing.T) {
	l, task, ws, sibling, _ := containmentLoopFixture(t)

	// Pre-existing operator dirt in the sibling, present at baseline.
	if err := os.WriteFile(filepath.Join(sibling, "operator-notes.md"), []byte("wip\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	pre, ok := snapshotContainment(l.containmentRoots(), l.config.RepoPath)
	if !ok {
		t.Fatal("baseline snapshot failed")
	}

	// Legitimate work: the Operate commits in its own workspace.
	commitFile(t, ws, "deliverable.md", "content\n", "real work")

	if !l.verifyOperateContainment(context.Background(), task, pre) {
		t.Fatal("verifyOperateContainment tripped on workspace-only work (false positive)")
	}
}

// Unreadable post-state must fail closed: the output of an unwatchable run is
// never accepted.
func TestVerifyOperateContainmentUnreadablePostFailsClosed(t *testing.T) {
	parent := t.TempDir()
	ws := initGitRepoAt(t, parent, "workspace")
	watched := t.TempDir()
	initGitRepoAt(t, watched, "observed")

	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_containment_unreadable")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Write the catalog", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	l, err := New(Config{Agent: agent, HumanID: humanID(), RepoPath: ws, TaskStore: ts,
		ContainmentWatchRoots: []string{watched}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pre, ok := snapshotContainment(l.containmentRoots(), l.config.RepoPath)
	if !ok {
		t.Fatal("baseline snapshot failed")
	}
	if err := os.RemoveAll(watched); err != nil {
		t.Fatalf("remove watch root: %v", err)
	}
	if l.verifyOperateContainment(context.Background(), task, pre) {
		t.Fatal("verifyOperateContainment accepted an unreadable post-state (must fail closed)")
	}
	if taskHasCompletedEvent(t, g, task.ID) {
		t.Fatal("an unverifiable containment state produced a work.task.completed event")
	}
}
