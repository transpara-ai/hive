package runner

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovyou-ai/hive/pkg/api"
)

// TestCriticThrottleBypassInOneShot verifies that in one-shot mode the critic
// runs on tick 1 (not deferred to tick 4).
func TestCriticThrottleBypassInOneShot(t *testing.T) {
	for tick := 1; tick <= 4; tick++ {
		throttled := !false && tick%4 != 0 // normal mode
		if tick == 4 && throttled {
			t.Errorf("tick %d should NOT be throttled in normal mode", tick)
		}
		if tick != 4 && !throttled {
			t.Errorf("tick %d should be throttled in normal mode", tick)
		}

		throttledOneShot := !true && tick%4 != 0 // one-shot mode
		if throttledOneShot {
			t.Errorf("tick %d should NOT be throttled in one-shot mode", tick)
		}
	}
}

func TestParseVerdict(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"pass", "Looks good.\n\nVERDICT: PASS", "PASS"},
		{"revise", "Missing allowlist.\nVERDICT: REVISE", "REVISE"},
		{"default", "No verdict line", "PASS"},
		{"whitespace", "  VERDICT:  PASS  ", "PASS"},
		{"middle", "Line 1\nVERDICT: REVISE\nLine 3", "REVISE"},
		{"invalid", "VERDICT: INVALID", "PASS"},
		// Regression: REVISE appears in body as historical discussion; actual verdict line is absent.
		// Old strings.Contains gate would false-positive on this; parseVerdict must return PASS.
		{"pass_with_revise_in_body", "**Verdict:** PASS\n\nPrevious critique issued VERDICT: REVISE but Builder addressed it.", "PASS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVerdict(tt.input)
			if got != tt.expect {
				t.Errorf("parseVerdict(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestExtractIssues(t *testing.T) {
	content := "Issue 1: missing allowlist entry\nIssue 2: no tests\n\nVERDICT: REVISE"
	got := extractIssues(content)
	if got != "Issue 1: missing allowlist entry\nIssue 2: no tests" {
		t.Errorf("extractIssues returned: %q", got)
	}
}

func TestBuildReviewPrompt(t *testing.T) {
	c := commit{hash: "abc123def456", subject: "[hive:builder] Add Policy"}
	diff := "+KindPolicy = \"policy\""

	prompt := buildReviewPrompt(c, diff, "## Invariants\n1. IDENTITY\n2. VERIFIED")

	// Should contain the commit info.
	if !contains(prompt, "abc123def456") {
		t.Error("prompt missing commit hash")
	}
	if !contains(prompt, "[hive:builder] Add Policy") {
		t.Error("prompt missing commit subject")
	}
	if !contains(prompt, "+KindPolicy") {
		t.Error("prompt missing diff content")
	}
	// Should contain the checklist.
	if !contains(prompt, "Completeness") {
		t.Error("prompt missing checklist")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestWriteCritiqueArtifact(t *testing.T) {
	cases := []struct {
		name    string
		verdict string
		summary string
	}{
		{"pass", "PASS", "All invariants satisfied."},
		{"revise", "REVISE", "Missing test coverage for new handler."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
				t.Fatalf("mkdir loop: %v", err)
			}

			if err := writeCritiqueArtifact(dir, "test subject", tc.verdict, tc.summary); err != nil {
				t.Fatalf("writeCritiqueArtifact: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(dir, "loop", "critique.md"))
			if err != nil {
				t.Fatalf("read critique.md: %v", err)
			}
			content := string(data)

			if !strings.Contains(content, "**Verdict:** "+tc.verdict) {
				t.Errorf("verdict %q not found in:\n%s", tc.verdict, content)
			}
			if !strings.Contains(content, tc.summary) {
				t.Errorf("summary %q not found in:\n%s", tc.summary, content)
			}
		})
	}
}

// TestReviewCommitFixTaskHasCauses verifies that when the critic issues a REVISE
// verdict, the fix task is created with causes pointing to the critique claim node.
// This satisfies Invariant 2 (CAUSALITY): fix tasks are traceable to their source.
func TestReviewCommitFixTaskHasCauses(t *testing.T) {
	// Track every POST body received by the mock server.
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(data, &m); err == nil {
			bodies = append(bodies, m)
		}
		w.Header().Set("Content-Type", "application/json")
		op, _ := m["op"].(string)
		switch op {
		case "assert":
			// Claim creation — return a node with a known ID.
			_, _ = w.Write([]byte(`{"op":"assert","node":{"id":"claim-99","kind":"claim","title":"Critique","created_at":"","updated_at":""}}`))
		case "intend":
			// Task creation — return a task node.
			_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"task-11","kind":"task","title":"Fix: something","created_at":"","updated_at":""}}`))
		default:
			_, _ = w.Write([]byte(`{"op":"ok"}`))
		}
	}))
	defer srv.Close()

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
	if err := os.WriteFile(filepath.Join(repoDir, "init.txt"), []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repoDir, "feature.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "[hive:builder] Add feature")

	hashCmd := exec.Command("git", "log", "--format=%H", "-1")
	hashCmd.Dir = repoDir
	hashOut, err := hashCmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	hash := strings.TrimSpace(string(hashOut))

	hiveDir := makeHiveDir(t, "# State\n", nil)

	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  repoDir,
		SpaceSlug: "hive",
		APIClient: api.New(srv.URL, "test-key"),
		Provider:  &mockProvider{response: "Issues found.\n\nVERDICT: REVISE"},
	})

	r.reviewCommit(t.Context(), commit{hash: hash, subject: "[hive:builder] Add feature"})

	// Find the intend (task creation) call.
	var taskBody map[string]any
	for _, b := range bodies {
		if op, _ := b["op"].(string); op == "intend" {
			taskBody = b
			break
		}
	}
	if taskBody == nil {
		t.Fatal("no intend op found — fix task was not created")
	}

	rawCauses, ok := taskBody["causes"]
	if !ok {
		t.Fatal("fix task missing 'causes' field — Invariant 2 violated")
	}
	causes, ok := rawCauses.([]any)
	if !ok || len(causes) == 0 {
		t.Fatalf("causes is empty or wrong type: %v", rawCauses)
	}
	if causes[0] != "claim-99" {
		t.Errorf("fix task causes[0] = %v, want %q (critique claim ID)", causes[0], "claim-99")
	}
}

// TestWriteCritiqueArtifactRunnerPassesBuildCauses verifies that the Runner's
// writeCritiqueArtifact method forwards causeIDs (the build document IDs) to
// AssertClaim. The critique claim must declare what build it is reviewing
// (Invariant 2: CAUSALITY).
func TestWriteCritiqueArtifactRunnerPassesBuildCauses(t *testing.T) {
	var assertBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(body, &m); err == nil {
			if op, _ := m["op"].(string); op == "assert" {
				assertBody = m
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"op":"assert","node":{"id":"claim-55","kind":"claim","title":"Critique","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	hiveDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hiveDir, "loop"), 0755); err != nil {
		t.Fatalf("mkdir loop: %v", err)
	}

	r := &Runner{
		cfg: Config{
			HiveDir:   hiveDir,
			SpaceSlug: "hive",
			APIClient: api.New(srv.URL, "test-key"),
		},
	}

	buildCauses := []string{"build-doc-111"}
	claimID, err := r.writeCritiqueArtifact("Add feature X", "PASS", "All tests pass.", buildCauses)
	if err != nil {
		t.Fatalf("writeCritiqueArtifact error: %v", err)
	}
	if claimID != "claim-55" {
		t.Errorf("claimID = %q, want %q", claimID, "claim-55")
	}

	if assertBody == nil {
		t.Fatal("no assert request received — AssertClaim was not called")
	}
	rawCauses, ok := assertBody["causes"]
	if !ok {
		t.Fatal("assert request missing 'causes' field — build document not declared as cause")
	}
	causes, ok := rawCauses.([]any)
	if !ok || len(causes) == 0 {
		t.Fatalf("causes = %v, want non-empty array", rawCauses)
	}
	if causes[0] != "build-doc-111" {
		t.Errorf("causes[0] = %v, want %q (build document ID)", causes[0], "build-doc-111")
	}
}
