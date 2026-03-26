package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/hive/pkg/api"
)

func TestParseAction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"done", "I've finished the work.\n\nACTION: DONE", "DONE"},
		{"progress", "Still working.\nACTION: PROGRESS", "PROGRESS"},
		{"escalate", "Need help.\nACTION: ESCALATE", "ESCALATE"},
		{"default", "No action line here.", "DONE"},
		{"with whitespace", "  ACTION:  DONE  \n", "DONE"},
		{"middle of text", "Line 1\nACTION: DONE\nLine 3", "DONE"},
		{"invalid action", "ACTION: INVALID", "DONE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAction(tt.input)
			if got != tt.expect {
				t.Errorf("parseAction(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestPickHighestPriority(t *testing.T) {
	nodes := []api.Node{
		{ID: "1", Priority: "low"},
		{ID: "2", Priority: "urgent"},
		{ID: "3", Priority: "high"},
		{ID: "4", Priority: "medium"},
	}
	got := pickHighestPriority(nodes)
	if got.ID != "2" {
		t.Errorf("pickHighestPriority returned ID=%s, want 2 (urgent)", got.ID)
	}
}

func TestPickHighestPriorityEmpty(t *testing.T) {
	nodes := []api.Node{
		{ID: "1", Priority: ""},
		{ID: "2", Priority: "medium"},
	}
	got := pickHighestPriority(nodes)
	if got.ID != "2" {
		t.Errorf("pickHighestPriority returned ID=%s, want 2 (medium over empty)", got.ID)
	}
}

func TestPickHighestPriorityRecencyTiebreak(t *testing.T) {
	// Same priority: newest should win.
	nodes := []api.Node{
		{ID: "old", Priority: "high", CreatedAt: "2026-03-22T00:00:00Z"},
		{ID: "new", Priority: "high", CreatedAt: "2026-03-24T00:00:00Z"},
	}
	got := pickHighestPriority(nodes)
	if got.ID != "new" {
		t.Errorf("pickHighestPriority returned ID=%s, want 'new' (recency tiebreak)", got.ID)
	}
}

func TestPickHighestPriorityPriorityBeatsRecency(t *testing.T) {
	// Higher priority beats newer date.
	nodes := []api.Node{
		{ID: "new-low", Priority: "low", CreatedAt: "2026-03-24T00:00:00Z"},
		{ID: "old-urgent", Priority: "urgent", CreatedAt: "2026-03-20T00:00:00Z"},
	}
	got := pickHighestPriority(nodes)
	if got.ID != "old-urgent" {
		t.Errorf("pickHighestPriority returned ID=%s, want 'old-urgent' (priority beats recency)", got.ID)
	}
}

func TestCostTracker(t *testing.T) {
	ct := CostTracker{BudgetUSD: 10.0}

	if ct.IsOverBudget() {
		t.Error("should not be over budget initially")
	}

	ct.Record(decision.TokenUsage{
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      5.0,
	})

	if ct.CallCount != 1 {
		t.Errorf("CallCount = %d, want 1", ct.CallCount)
	}
	if ct.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", ct.InputTokens)
	}
	if ct.TotalCostUSD != 5.0 {
		t.Errorf("TotalCostUSD = %f, want 5.0", ct.TotalCostUSD)
	}
	if ct.IsOverBudget() {
		t.Error("should not be over budget at $5/$10")
	}

	ct.Record(decision.TokenUsage{CostUSD: 6.0})
	if !ct.IsOverBudget() {
		t.Error("should be over budget at $11/$10")
	}
}

func TestModelForRole(t *testing.T) {
	tests := []struct {
		role  string
		want  string
	}{
		{"builder", "sonnet"},
		{"scout", "haiku"},
		{"critic", "sonnet"},
		{"unknown", "haiku"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := ModelForRole(tt.role)
			if got != tt.want {
				t.Errorf("ModelForRole(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestExtractSummary(t *testing.T) {
	short := "hello"
	if extractSummary(short) != short {
		t.Error("short string should be returned as-is")
	}

	long := string(make([]byte, 600))
	got := extractSummary(long)
	if len(got) != 500 {
		t.Errorf("long string should be truncated to 500, got %d", len(got))
	}
}

func TestCritiqueArtifactWritten(t *testing.T) {
	// Set up a minimal git repo with two commits so hash~1 is valid.
	repoDir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")

	// Initial commit (so hash~1 exists for the builder commit).
	if err := os.WriteFile(filepath.Join(repoDir, "init.txt"), []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	// Builder commit to review.
	if err := os.WriteFile(filepath.Join(repoDir, "feature.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "[hive:builder] Add feature")

	// Get the commit hash.
	hashCmd := exec.Command("git", "log", "--format=%H", "-1")
	hashCmd.Dir = repoDir
	hashOut, err := hashCmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	hash := strings.TrimSpace(string(hashOut))

	hiveDir := makeHiveDir(t, "# State\n", nil)

	r := New(Config{
		HiveDir:  hiveDir,
		RepoPath: repoDir,
		Provider: &mockProvider{response: "Looks good.\n\nVERDICT: PASS"},
	})

	r.reviewCommit(context.Background(), commit{hash: hash, subject: "[hive:builder] Add feature"})

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "critique.md"))
	if err != nil {
		t.Fatalf("critique.md not written: %v", err)
	}
	if !strings.Contains(string(data), "PASS") {
		t.Errorf("critique.md does not contain PASS:\n%s", string(data))
	}
}
