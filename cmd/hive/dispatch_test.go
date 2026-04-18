package main

import (
	"strings"
	"testing"
)

// TestRunNoFlags verifies that running with no verb produces a help-style
// error mentioning the available verbs.
func TestRunNoFlags(t *testing.T) {
	err := routeAndDispatch(nil)
	if err == nil {
		t.Fatal("expected error when no verb given")
	}
	msg := err.Error()
	for _, want := range []string{"civilization", "pipeline", "role", "ingest", "council"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q, got: %s", want, msg)
		}
	}
	for _, gone := range []string{"--pipeline", "--role", "--human", "--loop", "--one-shot"} {
		if strings.Contains(msg, gone) {
			t.Errorf("error should NOT mention removed flag %q, got: %s", gone, msg)
		}
	}
}
