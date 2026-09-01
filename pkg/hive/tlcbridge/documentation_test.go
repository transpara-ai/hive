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
	for _, required := range []string{
		"tlcbridge.Bind",
		"IdempotencyKey",
		"Effects.WorktreeRepository = transpara-ai/repo-x",
		"Effects.PullRequestRepository = transpara-ai/repo-x",
		"Source.Repository",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("README is missing %q", required)
		}
	}
	for _, prohibited := range []string{
		"adopt this repository",
		"enable runtime",
		"enable enforcement",
		"deploy to production",
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
	for _, prohibited := range []string{
		"used by its existing durable EventGraph/Work dispatch",
		"BoundRequest is ready for Hive's existing durable dispatcher",
	} {
		if strings.Contains(text, prohibited) {
			t.Errorf("bridge.go retains superseded claim %q", prohibited)
		}
	}
	for _, required := range []string{
		"idempotency key for a caller to record",
		"input type for any existing durable dispatcher",
		"performs no persistence or effects",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("bridge.go is missing boundary statement %q", required)
		}
	}
}
