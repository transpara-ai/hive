package loop

import (
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/work"
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
