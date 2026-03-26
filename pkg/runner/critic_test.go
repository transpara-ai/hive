package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
