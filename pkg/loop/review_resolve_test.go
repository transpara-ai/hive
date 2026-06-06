package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

func TestIsHex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc123", true},
		{"ABC123", true},
		{"0123456789abcdef", true},
		{"xyz", false},
		{"abc12g", false},
		{"", true}, // empty is vacuously hex (length check happens elsewhere)
	}
	for _, tt := range tests {
		if got := isHex(tt.input); got != tt.want {
			t.Errorf("isHex(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractCommitHash_NoRepo(t *testing.T) {
	// Without a valid repo, no hash should be found (git rev-parse fails).
	got := extractCommitHash("Implemented feature in commit abc1234", "/nonexistent")
	if got != "" {
		t.Errorf("extractCommitHash with bad repo returned %q, want empty", got)
	}
}

func TestExtractCommitHash_RealRepo(t *testing.T) {
	// Use this repo itself — HEAD should be a valid commit.
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}

	short := head[:7]

	// Should find the hash in a summary string.
	got := extractCommitHash("Fixed bug in commit "+short, ".")
	if got == "" {
		t.Fatalf("extractCommitHash(%q) returned empty, expected full hash", short)
	}
	if got != head {
		t.Errorf("extractCommitHash returned %q, want %q", got, head)
	}

	// Should not match non-hex words.
	got = extractCommitHash("Updated the handler module", ".")
	if got != "" {
		t.Errorf("extractCommitHash with no hash returned %q, want empty", got)
	}

	// Should strip trailing punctuation.
	got = extractCommitHash("See commit "+short+".", ".")
	if got == "" {
		t.Fatal("extractCommitHash with trailing period returned empty")
	}
}

func TestExtractCommitHash_BoundedAttempts(t *testing.T) {
	// Feed 10 hex-like words that won't resolve. Should not run more than 5 rev-parse calls.
	// We can't directly observe the count, but we verify it completes quickly and returns empty.
	text := "aabbcc1 aabbcc2 aabbcc3 aabbcc4 aabbcc5 aabbcc6 aabbcc7 aabbcc8 aabbcc9 aabbcc0"
	got := extractCommitHash(text, "/nonexistent")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolveCommitForTask_Strategy1_HashInSummary(t *testing.T) {
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}
	short := head[:7]

	l := &Loop{config: Config{RepoPath: "."}}

	task := work.TaskCompletedContent{
		Summary: "Implemented feature in commit " + short,
	}

	commitHash, diffRef := l.resolveCommitForTask(task, true)
	if commitHash != head {
		t.Errorf("Strategy 1: commitHash = %q, want %q", commitHash, head)
	}
	want := head + "^.." + head
	if diffRef != want {
		t.Errorf("Strategy 1: diffRef = %q, want %q", diffRef, want)
	}
}

func TestResolveCommitForTask_Fallback_NoHash(t *testing.T) {
	// resolveCommitForTask needs l.agent which is hard to mock in a unit test.
	// Instead, test the logic directly: no hash in summary, no task → HEAD~1.
	// We test the components; the integration test verifies composition.
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}

	// No hash in summary → extractCommitHash returns empty.
	task := work.TaskCompletedContent{
		Summary:     "Fixed some bugs",
		CompletedBy: types.MustActorID("actor_test123"),
	}
	got := extractCommitHash(task.Summary, ".")
	if got != "" {
		t.Errorf("expected empty hash from summary without commit ref, got %q", got)
	}
}

func TestResolveCommitForTask_Strategy0_ArtifactRef(t *testing.T) {
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}

	// Create a real TaskStore with an artifact containing a commit hash.
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent, g := agentWithGraph(t, provider)

	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_strategy0_test")

	// Create task + artifact with commit hash in body.
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Test task", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	artifactBody := "commit: " + head + "\n\nfoo.go | 5 ++"
	if err := ts.AddArtifact(agent.ID(), task.ID, "Operate result", "text/plain", artifactBody, causes, convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}

	// Complete — ArtifactRef should be set.
	if err := ts.Complete(agent.ID(), task.ID, "done", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Find the completed event to get ArtifactRef.
	completedPage, err := g.Store().ByType(work.EventTypeTaskCompleted, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	var completedContent work.TaskCompletedContent
	for _, ev := range completedPage.Items() {
		c, ok := ev.Content().(work.TaskCompletedContent)
		if ok && c.TaskID == task.ID {
			completedContent = c
			break
		}
	}
	if completedContent.ArtifactRef.IsZero() {
		t.Fatal("ArtifactRef is zero on completed event")
	}

	// Now test resolveCommitForTask with Strategy 0.
	l := &Loop{config: Config{RepoPath: ".", TaskStore: ts}}
	commitHash, diffRef := l.resolveCommitForTask(completedContent, true)

	if commitHash != head {
		t.Errorf("Strategy 0: commitHash = %q, want %q", commitHash, head)
	}
	wantRef := head + "^.." + head
	if diffRef != wantRef {
		t.Errorf("Strategy 0: diffRef = %q, want %q", diffRef, wantRef)
	}
}

func TestResolveCommitForTask_Strategy0_ArtifactRange(t *testing.T) {
	repo := newTempGitRepo(t)
	preHead := gitCommand(repo, "rev-parse", "HEAD")
	commitFile(t, repo, "first.txt", "first\n", "first")
	commitFile(t, repo, "second.txt", "second\n", "second")
	postHead := gitCommand(repo, "rev-parse", "HEAD")

	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_strategy0_range_test")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Test task", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	artifactBody := buildOperateArtifactBody(repo, preHead, postHead)
	if err := ts.AddArtifact(agent.ID(), task.ID, "Operate result", "text/plain", artifactBody, causes, convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := ts.Complete(agent.ID(), task.ID, "done", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	completedContent := completedTaskContent(t, g, task.ID)
	l := &Loop{config: Config{RepoPath: repo, TaskStore: ts}}
	commitHash, diffRef := l.resolveCommitForTask(completedContent, true)

	if commitHash != postHead {
		t.Errorf("Strategy 0 range: commitHash = %q, want %q", commitHash, postHead)
	}
	wantRef := preHead + ".." + postHead
	if diffRef != wantRef {
		t.Errorf("Strategy 0 range: diffRef = %q, want %q", diffRef, wantRef)
	}
	stat := gitCommand(repo, "diff", diffRef, "--stat")
	for _, want := range []string{"first.txt", "second.txt"} {
		if !strings.Contains(stat, want) {
			t.Fatalf("resolved diff stat missing %q; stat:\n%s", want, stat)
		}
	}
}

func TestFetchArtifactBody_WaiverReturnsFalse(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent, g := agentWithGraph(t, provider)

	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_waiver_test")

	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Waived task", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ts.WaiveArtifact(agent.ID(), task.ID, "no commits", causes, convID); err != nil {
		t.Fatalf("WaiveArtifact: %v", err)
	}

	if err := ts.Complete(agent.ID(), task.ID, "done", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Get ArtifactRef (points to waiver).
	completedPage, err := g.Store().ByType(work.EventTypeTaskCompleted, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	var artifactRef types.EventID
	for _, ev := range completedPage.Items() {
		c, ok := ev.Content().(work.TaskCompletedContent)
		if ok && c.TaskID == task.ID {
			artifactRef = c.ArtifactRef
			break
		}
	}
	if artifactRef.IsZero() {
		t.Fatal("ArtifactRef is zero")
	}

	l := &Loop{config: Config{TaskStore: ts}}
	_, isArtifact := l.fetchArtifactBody(artifactRef)
	if isArtifact {
		t.Fatal("fetchArtifactBody returned true for waiver ref; expected false")
	}
}

// The artifact body embeds a `git diff --stat` section whose filenames are
// agent-controlled. A file NAMED like a header key (e.g. "head:<hex>") must not
// be parsed as a base:/head: line and override the machine-written range —
// otherwise an untrusted implementer could force a single-commit fallback and
// re-hide an earlier commit from the reviewer (the round-8 class). This proves
// the gate holds across the injection input-domain, not just one shape.
func TestExtractCommitRange_RejectsStatSectionInjection(t *testing.T) {
	repo := newTempGitRepo(t)
	preHead := gitCommand(repo, "rev-parse", "HEAD")
	commitFile(t, repo, "first.txt", "first\n", "A")
	commitFile(t, repo, "second.txt", "second\n", "B")
	postHead := gitCommand(repo, "rev-parse", "HEAD")

	clean := buildOperateArtifactBody(repo, preHead, postHead)
	if b, h := extractCommitRange(clean, repo); b != preHead || h != postHead {
		t.Fatalf("clean body resolved to (%q,%q), want (%q,%q)", b, h, preHead, postHead)
	}

	zeros := strings.Repeat("0", 40)
	appendages := []struct {
		name string
		rows string
	}{
		{"head_garbage_stat_row", " head:" + zeros + " | 1 +"},
		{"base_garbage_stat_row", " base:" + zeros + " | 1 +"},
		{"both_keys", " base:" + zeros + " | 1 +\n head:" + zeros + " | 1 +"},
		{"real_hash_suffixed", " head:" + preHead + " | 1 +"},
		{"clean_override_no_suffix", "head: " + preHead},
	}
	for _, a := range appendages {
		t.Run(a.name, func(t *testing.T) {
			body := clean + "\n" + a.rows
			b, h := extractCommitRange(body, repo)
			if b != preHead || h != postHead {
				t.Errorf("stat-section injection overrode the header: got (%q,%q), want (%q,%q)\ninjected rows:\n%s", b, h, preHead, postHead, a.rows)
			}
		})
	}
}

// End-to-end: an untrusted implementer makes two commits and names a file in the
// FINAL commit "head:<hex>" so its real `git diff --stat` row would inject a
// head: line. The reviewer resolution must still return the full pre..post range
// (not fall back to a single-commit diff that hides the earlier commit).
func TestResolveCommitForTask_Strategy0_AdversarialFilename(t *testing.T) {
	repo := newTempGitRepo(t)
	preHead := gitCommand(repo, "rev-parse", "HEAD")
	commitFile(t, repo, "first.txt", "SECRET unreviewed change\n", "A hidden")
	commitFile(t, repo, "head:"+strings.Repeat("0", 40), "x\n", "B benign")
	postHead := gitCommand(repo, "rev-parse", "HEAD")

	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	factory := event.NewEventFactory(g.Registry())
	ts := work.NewTaskStore(g.Store(), factory, &testSigner{})
	convID := types.MustConversationID("conv_adversarial_filename")
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "Adversarial filename", "desc", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	artifactBody := buildOperateArtifactBody(repo, preHead, postHead)
	if err := ts.AddArtifact(agent.ID(), task.ID, "Operate result", "text/plain", artifactBody, causes, convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := ts.Complete(agent.ID(), task.ID, "done", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	completedContent := completedTaskContent(t, g, task.ID)
	l := &Loop{config: Config{RepoPath: repo, TaskStore: ts}}
	commitHash, diffRef := l.resolveCommitForTask(completedContent, true)

	wantRef := preHead + ".." + postHead
	if diffRef != wantRef {
		t.Errorf("diffRef = %q, want %q (a single-commit fallback would re-hide first.txt)", diffRef, wantRef)
	}
	if commitHash != postHead {
		t.Errorf("commitHash = %q, want %q", commitHash, postHead)
	}
	stat := gitCommand(repo, "diff", diffRef, "--stat")
	if !strings.Contains(stat, "first.txt") {
		t.Fatalf("resolved diff hides first.txt (round-8 re-opened); stat:\n%s", stat)
	}
}
