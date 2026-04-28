package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/api"
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
		{"default", "No action line here.", "PROGRESS"},
		{"with whitespace", "  ACTION:  DONE  \n", "DONE"},
		{"middle of text", "Line 1\nACTION: DONE\nLine 3", "DONE"},
		{"invalid action", "ACTION: INVALID", "PROGRESS"},
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
		{"builder", "claude-sonnet-4-6"},
		{"scout", "claude-haiku-4-5-20251001"},
		{"critic", "claude-sonnet-4-6"},
		{"unknown", "claude-sonnet-4-6"}, // resolver falls back to system default (sonnet)
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

func TestBuildArtifactWritten(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoDir, "file.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "Add feature X")

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{HiveDir: hiveDir, RepoPath: repoDir})

	task := api.Node{ID: "task-1", Title: "Add feature X", Kind: "task"}
	r.writeBuildArtifact(task, 0.0042, "")

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "build.md"))
	if err != nil {
		t.Fatalf("build.md not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "Add feature X") {
		t.Errorf("build.md does not contain task title:\n%s", body)
	}
	if !strings.Contains(body, "0.0042") {
		t.Errorf("build.md does not contain cost:\n%s", body)
	}
}

func TestBuildArtifactContainsSummary(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoDir, "file.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "Add feature Y")

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{HiveDir: hiveDir, RepoPath: repoDir})

	task := api.Node{ID: "task-2", Title: "Add feature Y", Kind: "task"}
	summary := "Implemented the new handler and added unit tests covering all branches."
	r.writeBuildArtifact(task, 0.0010, summary)

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "build.md"))
	if err != nil {
		t.Fatalf("build.md not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "## What Was Built") {
		t.Errorf("build.md missing '## What Was Built' section:\n%s", body)
	}
	if !strings.Contains(body, summary) {
		t.Errorf("build.md missing operate summary:\n%s", body)
	}
}

func TestBuildArtifactSummaryTruncated(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoDir, "file.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "Add feature Z")

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{HiveDir: hiveDir, RepoPath: repoDir})

	task := api.Node{ID: "task-3", Title: "Add feature Z", Kind: "task"}
	longSummary := strings.Repeat("x", 3000)
	r.writeBuildArtifact(task, 0.0, longSummary)

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "build.md"))
	if err != nil {
		t.Fatalf("build.md not written: %v", err)
	}
	if !strings.Contains(string(data), strings.Repeat("x", 2000)) {
		t.Errorf("build.md should contain 2000 x's (truncated summary)")
	}
	if strings.Contains(string(data), strings.Repeat("x", 2001)) {
		t.Errorf("build.md summary was not truncated to 2000 chars")
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

// TestBranchResetOnDaemonCycle verifies the guard condition used in
// runDaemon: when PRMode is false, buildBranchName returns "" so the
// git-reset-to-main step is skipped for non-PR workflows.
func TestBranchResetOnDaemonCycle(t *testing.T) {
	cfg := Config{PRMode: false}
	if got := buildBranchName(cfg, "some task title"); got != "" {
		t.Errorf("buildBranchName with PRMode=false: expected \"\", got %q", got)
	}
}

// mockErrorOperator implements intelligence.Provider + decision.IOperator,
// returning an error from Operate.
type mockErrorOperator struct {
	operateErr error
}

// mockDoneOperator implements intelligence.Provider + decision.IOperator,
// returning a DONE response from Operate (no error).
type mockDoneOperator struct{}

func (m *mockDoneOperator) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	score, _ := types.NewScore(0.9)
	return decision.NewResponse("", score, decision.TokenUsage{}), nil
}
func (m *mockDoneOperator) Name() string  { return "mock-done-operator" }
func (m *mockDoneOperator) Model() string { return "mock-model" }
func (m *mockDoneOperator) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	return decision.OperateResult{Summary: "ACTION: DONE"}, nil
}

func (m *mockErrorOperator) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	score, _ := types.NewScore(0.9)
	return decision.NewResponse("", score, decision.TokenUsage{}), nil
}
func (m *mockErrorOperator) Name() string  { return "mock-operator" }
func (m *mockErrorOperator) Model() string { return "mock-model" }
func (m *mockErrorOperator) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	return decision.OperateResult{}, m.operateErr
}

func TestWorkTaskOperateErrorWritesDiagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"op":"ok"}`))
	}))
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)

	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  t.TempDir(),
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		Provider:  &mockErrorOperator{operateErr: fmt.Errorf("claude CLI failed: exit status 1")},
	})

	task := api.Node{ID: "task-1", Title: "Add feature X", Kind: "task"}
	r.workTask(context.Background(), task)

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "diagnostics.jsonl"))
	if err != nil {
		t.Fatalf("diagnostics.jsonl not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `"phase":"builder"`) {
		t.Errorf("diagnostics.jsonl missing phase=builder:\n%s", body)
	}
	if !strings.Contains(body, "claude CLI failed") {
		t.Errorf("diagnostics.jsonl missing error message:\n%s", body)
	}
}

func TestWorkTaskBuildVerifyFailureWritesDiagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"op":"ok"}`))
	}))
	defer srv.Close()

	// Empty repo dir — go.exe build ./... will fail (no Go files / go.mod, or go.exe not found on non-Windows).
	repoDir := t.TempDir()
	hiveDir := makeHiveDir(t, "# State\n", nil)

	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  repoDir,
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		Provider:  &mockDoneOperator{},
	})

	task := api.Node{ID: "task-1", Title: "Add feature X", Kind: "task"}
	r.workTask(context.Background(), task)

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "diagnostics.jsonl"))
	if err != nil {
		t.Fatalf("diagnostics.jsonl not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `"phase":"builder"`) {
		t.Errorf("diagnostics.jsonl missing phase=builder:\n%s", body)
	}
	if !strings.Contains(body, `"error"`) {
		t.Errorf("diagnostics.jsonl missing error field:\n%s", body)
	}
}

// TestWriteBuildArtifactDocumentCauses verifies that writeBuildArtifact calls
// CreateDocument with causes: [task.ID], satisfying Invariant 2 (CAUSALITY).
// The build document is causally linked to the task that triggered the build.
func TestWriteBuildArtifactDocumentCauses(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			bodies = append(bodies, m)
		}
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"doc-1","kind":"document","title":"Build: task","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

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
	if err := os.WriteFile(filepath.Join(repoDir, "file.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "Add feature")

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  repoDir,
		SpaceSlug: "hive",
		APIClient: api.New(srv.URL, "test-key"),
	})

	task := api.Node{ID: "task-42", Title: "Add feature", Kind: "task"}
	r.writeBuildArtifact(task, 0.001, "summary")

	// Find the document creation request.
	var docBody map[string]any
	for _, b := range bodies {
		if kind, _ := b["kind"].(string); kind == "document" {
			docBody = b
			break
		}
	}
	if docBody == nil {
		t.Fatal("no document creation request found — writeBuildArtifact did not call CreateDocument")
	}

	rawCauses, ok := docBody["causes"]
	if !ok {
		t.Fatal("build document missing 'causes' field — Invariant 2 violated")
	}
	causes, ok := rawCauses.([]any)
	if !ok || len(causes) == 0 {
		t.Fatalf("causes is empty or wrong type: %v", rawCauses)
	}
	if causes[0] != "task-42" {
		t.Errorf("document causes[0] = %v, want %q (the task that triggered the build)", causes[0], "task-42")
	}
}

// TestWriteBuildArtifactTitleNormalized verifies that the Build: document
// title strips retry prefixes from the task title, so that critic.go's
// lookup (which also normalizes the commit subject) finds the document
// across retry cycles. Without this, retries store "Build: Fix: Fix: X"
// but look up "Build: X" — permanent miss.
func TestWriteBuildArtifactTitleNormalized(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			bodies = append(bodies, m)
		}
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"doc-1","kind":"document","title":"Build: X","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

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
	if err := os.WriteFile(filepath.Join(repoDir, "file.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "Add feature")

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  repoDir,
		SpaceSlug: "hive",
		APIClient: api.New(srv.URL, "test-key"),
	})

	task := api.Node{ID: "task-42", Title: "[hive:builder] Fix: [hive:builder] Fix: X", Kind: "task"}
	r.writeBuildArtifact(task, 0.001, "summary")

	var docTitle string
	for _, b := range bodies {
		if kind, _ := b["kind"].(string); kind == "document" {
			docTitle, _ = b["title"].(string)
			break
		}
	}
	want := "Build: X"
	if docTitle != want {
		t.Errorf("build document title = %q, want %q (retry prefixes must be stripped)", docTitle, want)
	}
}

func TestStripHivePrefix(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"no prefix unchanged", "add feature X", "add feature X"},
		{"single prefix stripped", "[hive:builder] add feature X", "add feature X"},
		{"double nested prefix stripped", "[hive:builder] [hive:builder] add feature X", "add feature X"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHivePrefix(tt.input)
			if got != tt.expect {
				t.Errorf("stripHivePrefix(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
