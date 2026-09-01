package tlcbridge

import (
	"os"
	"strings"
	"testing"
)

func TestREADMEStaysWithinLibraryBoundary(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	normalized := strings.Join(strings.Fields(text), " ")
	for _, required := range []string{
		"tlcbridge.Bind",
		"IdempotencyKey",
		"Effects.WorktreeRepository = transpara-ai/repo-x",
		"Effects.PullRequestRepository = transpara-ai/repo-x",
		"Source.Repository",
		"Both repository effects come only from the independently supplied `Source.Repository`; the public TLC brief cannot override either target.",
		"Changing the source identity changes the idempotency key. Repeating the same normalized source and brief produces the same key.",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("README is missing %q", required)
		}
	}
	for _, prohibited := range []string{
		"adopt",
		"runtime",
		"enforc",
		"deploy",
		"durable-dispatch input",
	} {
		if strings.Contains(strings.ToLower(text), prohibited) {
			t.Errorf("README contains prohibited guidance %q", prohibited)
		}
	}
}

func TestPackageCommentsDoNotClaimExistingDispatchUse(t *testing.T) {
	content, err := os.ReadFile("bridge.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	const packageComment = `// Package tlcbridge validates the thin TLC change brief at Hive's ingress.
//
// TLC owns route selection and the short brief. Hive adds source identity,
// repository-effect containment, and an idempotency key for a caller to record.
// This package deliberately owns no dispatcher integration or second workflow
// state machine and performs no persistence or effects.`
	const boundRequestComment = `// BoundRequest is the validated library result returned to a caller. It is not
// an input type for any existing durable dispatcher. The TLC brief contains no
// source chain, retry, worktree, provider, or effect state.`

	for marker, want := range map[string]string{
		"package tlcbridge\n":      packageComment,
		"type BoundRequest struct": boundRequestComment,
	} {
		got, ok := precedingLineComment(text, marker)
		if !ok {
			t.Fatalf("bridge.go is missing marker %q", marker)
		}
		if got != want {
			t.Errorf("comment immediately before %q changed:\n--- got ---\n%s\n--- want ---\n%s", marker, got, want)
		}
	}
}

func precedingLineComment(text, marker string) (string, bool) {
	index := strings.Index(text, marker)
	if index < 0 {
		return "", false
	}
	lines := strings.Split(strings.TrimSuffix(text[:index], "\n"), "\n")
	start := len(lines)
	for start > 0 && strings.HasPrefix(lines[start-1], "//") {
		start--
	}
	if start == len(lines) {
		return "", false
	}
	for _, line := range lines[start:] {
		if !strings.HasPrefix(line, "//") {
			return "", false
		}
	}
	return strings.Join(lines[start:], "\n"), true
}
