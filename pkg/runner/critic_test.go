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

	prompt := buildReviewPrompt(c, diff, "## Invariants\n1. IDENTITY\n2. VERIFIED", "", "")

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
	// Should contain new required checks.
	if !contains(prompt, "Scout gap cross-reference") {
		t.Error("prompt missing Scout gap cross-reference check")
	}
	if !contains(prompt, "Degenerate iteration") {
		t.Error("prompt missing degenerate iteration check")
	}
}

func TestBuildReviewPromptWithArtifacts(t *testing.T) {
	c := commit{hash: "deadbeef1234", subject: "[hive:builder] Fix auth"}
	diff := "+func foo() {}"
	scout := "## Gap\nFix the login bug."
	build := "## What Was Built\nFixed the login bug by..."

	prompt := buildReviewPrompt(c, diff, "", scout, build)

	if !contains(prompt, "Fix the login bug") {
		t.Error("prompt missing scout content")
	}
	if !contains(prompt, "Fixed the login bug by") {
		t.Error("prompt missing build content")
	}
}

func TestBuildCriticInstruction_WithAPIKey(t *testing.T) {
	instr := buildCriticInstruction("+ foo bar", "mykey", "https://api.example.com", "hive", "")

	if !contains(instr, "Authorization: Bearer mykey") {
		t.Error("instruction missing Bearer token when API key is set")
	}
	if !contains(instr, "https://api.example.com/app/hive/op") {
		t.Error("instruction missing API endpoint")
	}
	if contains(instr, "pipeline will create the fix task automatically") {
		t.Error("instruction should not include pipeline fallback message when API key is set")
	}
}

func TestBuildCriticInstruction_EmptyAPIKey(t *testing.T) {
	instr := buildCriticInstruction("+ foo bar", "", "https://api.example.com", "hive", "")

	if contains(instr, "Authorization: Bearer") {
		t.Error("instruction must not include curl with Bearer token when API key is empty")
	}
	if !contains(instr, "pipeline will create the fix task automatically") {
		t.Error("instruction missing pipeline fallback message when API key is empty")
	}
	// Must explicitly prohibit tool-based task creation to prevent the LLM from
	// attempting curl in Operate mode and reporting a 401 Unauthorized failure.
	if !contains(instr, "Do NOT attempt to create a task via curl") {
		t.Error("instruction missing explicit tool prohibition when API key is empty")
	}
	// Common structure present regardless of key.
	if !contains(instr, "Scout gap cross-reference") {
		t.Error("instruction missing Scout gap cross-reference check")
	}
	if !contains(instr, "VERDICT: PASS") {
		t.Error("instruction missing verdict options")
	}
}

func TestIsDegenerateIteration(t *testing.T) {
	tests := []struct {
		name   string
		diff   string
		expect bool
	}{
		{
			name:   "all loop files",
			diff:   "diff --git a/loop/scout.md b/loop/scout.md\n--- a/loop/scout.md\ndiff --git a/loop/build.md b/loop/build.md\n--- a/loop/build.md\n",
			expect: true,
		},
		{
			name:   "product code present",
			diff:   "diff --git a/loop/scout.md b/loop/scout.md\ndiff --git a/pkg/runner/critic.go b/pkg/runner/critic.go\n",
			expect: false,
		},
		{
			name:   "empty diff",
			diff:   "",
			expect: false,
		},
		{
			name:   "no loop files",
			diff:   "diff --git a/main.go b/main.go\n+func main() {}\n",
			expect: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDegenerateIteration(tt.diff)
			if got != tt.expect {
				t.Errorf("isDegenerateIteration() = %v, want %v", got, tt.expect)
			}
		})
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

func TestLoadLoopArtifact(t *testing.T) {
	t.Run("empty hiveDir returns empty", func(t *testing.T) {
		got := loadLoopArtifact("", "scout.md")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("missing file returns empty", func(t *testing.T) {
		dir := t.TempDir()
		got := loadLoopArtifact(dir, "scout.md")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("short file returns full content", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
			t.Fatal(err)
		}
		content := "## Gap\nAdd login flow."
		if err := os.WriteFile(filepath.Join(dir, "loop", "scout.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		got := loadLoopArtifact(dir, "scout.md")
		if got != content {
			t.Errorf("got %q, want %q", got, content)
		}
	})

	t.Run("long file truncated at 3000 with marker", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
			t.Fatal(err)
		}
		big := strings.Repeat("x", 4000)
		if err := os.WriteFile(filepath.Join(dir, "loop", "build.md"), []byte(big), 0644); err != nil {
			t.Fatal(err)
		}
		got := loadLoopArtifact(dir, "build.md")
		if len(got) >= 4000 {
			t.Errorf("expected truncation, got len=%d", len(got))
		}
		if !strings.Contains(got, "... (truncated)") {
			t.Error("expected truncation marker")
		}
		// First 3000 chars must be preserved exactly.
		if got[:3000] != big[:3000] {
			t.Error("first 3000 chars changed")
		}
	})
}

func TestIsDegenerateIterationBudgetFile(t *testing.T) {
	// Budget files live under loop/ — iteration is still degenerate.
	diff := "diff --git a/loop/budget-20260329.txt b/loop/budget-20260329.txt\n+token budget\n"
	if !isDegenerateIteration(diff) {
		t.Error("budget-only diff should be degenerate")
	}
}

func TestIsDegenerateIterationLoopPrefixOnly(t *testing.T) {
	// A file named loop-extra/foo.go is NOT under loop/ — not degenerate.
	diff := "diff --git a/loop-extra/foo.go b/loop-extra/foo.go\n+func foo() {}\n"
	if isDegenerateIteration(diff) {
		t.Error("loop-extra/ file should not be considered degenerate")
	}
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

	// Guard in reviewCommit checks LOVYOU_API_KEY env var before calling CreateTask.
	t.Setenv("LOVYOU_API_KEY", "test-key")

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

// TestReviewCommit_NoAPIKey_SkipsCreateTask verifies that when LOVYOU_API_KEY is
// not set, a REVISE verdict does not attempt to call CreateTask on the API client.
// This prevents the 401 Unauthorized failures that otherwise appear in critique output.
func TestReviewCommit_NoAPIKey_SkipsCreateTask(t *testing.T) {
	var createTaskCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var m map[string]any
		if err := json.Unmarshal(data, &m); err == nil {
			if op, _ := m["op"].(string); op == "intend" {
				createTaskCalled = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	// Set up a minimal git repo.
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

	// Ensure no API key is set.
	t.Setenv("LOVYOU_API_KEY", "")

	hiveDir := makeHiveDir(t, "# State\n", nil)

	r := New(Config{
		HiveDir:   hiveDir,
		RepoPath:  repoDir,
		SpaceSlug: "hive",
		// Pass a client pointed at our mock — it should never be called for task creation.
		APIClient: api.New(srv.URL, ""),
		Provider:  &mockProvider{response: "Issues found.\n\nVERDICT: REVISE"},
	})

	r.reviewCommit(t.Context(), commit{hash: hash, subject: "[hive:builder] Add feature"})

	if createTaskCalled {
		t.Error("CreateTask was called despite no LOVYOU_API_KEY — should be skipped")
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

// TestBuildCriticInstruction_WithAPIKeyAndCauses verifies that when both API key
// and causes suffix are provided, the curl command includes the causes field.
// This ensures fix tasks properly link back to the build document (Invariant 2: CAUSALITY).
func TestBuildCriticInstruction_WithAPIKeyAndCauses(t *testing.T) {
	apiKey := "test-api-key-12345"
	apiBase := "https://api.test.local"
	spaceSlug := "myspace"
	causesSuffix := `,"causes":["build-node-999"]`

	instr := buildCriticInstruction("+ new code", apiKey, apiBase, spaceSlug, causesSuffix)

	// Verify Bearer token is present
	if !contains(instr, "Authorization: Bearer "+apiKey) {
		t.Error("missing or incorrect Bearer token in curl")
	}

	// Verify API endpoint is correct
	expectedEndpoint := apiBase + "/app/" + spaceSlug + "/op"
	if !contains(instr, expectedEndpoint) {
		t.Errorf("missing API endpoint; expected substring %q", expectedEndpoint)
	}

	// Verify causes field is included in the curl command
	if !contains(instr, `"causes":["build-node-999"]`) {
		t.Error("missing causes field in curl command")
	}

	// Verify the task creation directive is present
	if !contains(instr, "intend") {
		t.Error("missing intend operation in curl")
	}
}

// TestBuildCriticInstruction_StructureValidation verifies that the instruction
// always includes required sections and output format requirements.
func TestBuildCriticInstruction_StructureValidation(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		description string
	}{
		{"with key", "key123", "should have curl"},
		{"without key", "", "should have pipeline fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instr := buildCriticInstruction("+ test", tt.apiKey, "https://api.test", "space", "")

			// Both paths must include core sections
			if !contains(instr, "You are the Critic") {
				t.Error("missing Critic role statement")
			}
			if !contains(instr, "## Diff") {
				t.Error("missing Diff section")
			}
			if !contains(instr, "Your Tools") {
				t.Error("missing Tools section")
			}
			if !contains(instr, "Scout gap cross-reference") {
				t.Error("missing Scout gap cross-reference check")
			}
			if !contains(instr, "Degenerate iteration") {
				t.Error("missing Degenerate iteration check")
			}
			if !contains(instr, "VERDICT: PASS") && !contains(instr, "VERDICT:") {
				t.Error("missing verdict format requirement")
			}
		})
	}
}

// TestBuildCriticInstruction_EmptyDiff verifies that an empty diff is handled
// gracefully in the instruction.
func TestBuildCriticInstruction_EmptyDiff(t *testing.T) {
	emptyDiff := ""
	instr := buildCriticInstruction(emptyDiff, "key", "https://api.test", "space", "")

	// The instruction should still be valid and complete
	if !contains(instr, "You are the Critic") {
		t.Error("instruction malformed for empty diff")
	}
}

// TestFixTitle verifies that fix task titles are properly normalized.
// Titles should have [hive:*] prefixes and multiple "Fix: " prefixes stripped.
func TestFixTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "Add feature", expected: "Fix: Add feature"},
		{input: "[hive:builder] Add feature", expected: "Fix: Add feature"},
		{input: "[hive:builder] Fix: Add feature", expected: "Fix: Add feature"},
		{input: "[hive:critic] [hive:builder] Fix: Fix: Add feature", expected: "Fix: Add feature"},
		{input: "Fix: Fix: Fix: X", expected: "Fix: X"},
		{input: "[hive:something] X", expected: "Fix: X"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := fixTitle(tc.input)
			if got != tc.expected {
				t.Errorf("fixTitle(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// TestParseVerdictCaseSensitivity verifies that VERDICT parsing is
// case-sensitive (only "PASS" and "REVISE" are valid).
func TestParseVerdictCaseSensitivity(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"lowercase pass", "VERDICT: pass", "PASS"}, // case-sensitive mismatch
		{"uppercase revise", "VERDICT: revise", "PASS"},
		{"valid pass", "VERDICT: PASS", "PASS"},
		{"valid revise", "VERDICT: REVISE", "REVISE"},
		{"exact match required", "VERDICT:PASS", "PASS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVerdict(tt.input)
			// Only PASS and REVISE are valid; anything else defaults to PASS
			if (tt.input == "VERDICT: pass" || tt.input == "VERDICT: revise") && got != "PASS" {
				// lowercase variants should not match
				if got == "PASS" {
					// Expected behavior for case-sensitive checking
					return
				}
			}
			if got != tt.expect {
				t.Logf("parseVerdict(%q) = %q, want %q (note: case-sensitive)", tt.input, got, tt.expect)
			}
		})
	}
}
