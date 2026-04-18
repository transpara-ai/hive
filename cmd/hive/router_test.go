package main

import (
	"strings"
	"testing"
)

func TestRouteAndDispatchNoArgs(t *testing.T) {
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
}

func TestRouteAndDispatchUnknownVerb(t *testing.T) {
	err := routeAndDispatch([]string{"banana"})
	if err == nil {
		t.Fatal("expected error for unknown verb")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should name the unknown verb, got: %s", err.Error())
	}
}
