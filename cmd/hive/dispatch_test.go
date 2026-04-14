package main

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// TestRunNoFlags verifies that run() with no mode flags returns an error
// listing the available modes. This ensures the dispatch logic is intact
// after refactoring (e.g. merging --daemon into --pipeline).
func TestRunNoFlags(t *testing.T) {
	// Reset flag state so run() sees a clean set.
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	err := run()
	if err == nil {
		t.Fatal("expected error when no mode flags are set")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--pipeline") {
		t.Errorf("error should mention --pipeline, got: %s", msg)
	}
	if !strings.Contains(msg, "--role") {
		t.Errorf("error should mention --role, got: %s", msg)
	}
	if !strings.Contains(msg, "--human") {
		t.Errorf("error should mention --human, got: %s", msg)
	}
	// --daemon should NOT appear (it was removed).
	if strings.Contains(msg, "--daemon") {
		t.Errorf("error should not mention --daemon (removed), got: %s", msg)
	}
}
