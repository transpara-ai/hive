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
	for _, gone := range []string{"--pipeline", "--role", "--human", "--loop", "--one-shot"} {
		if strings.Contains(msg, gone) {
			t.Errorf("error should NOT mention removed flag %q, got: %s", gone, msg)
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

func TestCmdCivilizationRequiresSubverb(t *testing.T) {
	err := cmdCivilization(nil)
	if err == nil || !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("expected subverb-required error, got: %v", err)
	}
}

func TestCmdCivilizationRunRequiresHuman(t *testing.T) {
	err := cmdCivilization([]string{"run"})
	if err == nil || !strings.Contains(err.Error(), "--human") {
		t.Fatalf("expected --human required error, got: %v", err)
	}
}

func TestCmdCivilizationUnknownSubverb(t *testing.T) {
	err := cmdCivilization([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}

func TestCmdPipelineRequiresSubverb(t *testing.T) {
	err := cmdPipeline(nil)
	if err == nil || !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("expected subverb-required error, got: %v", err)
	}
}

func TestCmdPipelineUnknownSubverb(t *testing.T) {
	err := cmdPipeline([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}

func TestCmdRoleRequiresName(t *testing.T) {
	err := cmdRole(nil)
	if err == nil || !strings.Contains(err.Error(), "role name") {
		t.Fatalf("expected role-name-required error, got: %v", err)
	}
}

func TestCmdRoleRequiresSubverb(t *testing.T) {
	err := cmdRole([]string{"builder"})
	if err == nil || !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "daemon") {
		t.Fatalf("expected subverb-required error, got: %v", err)
	}
}

func TestCmdRoleUnknownSubverb(t *testing.T) {
	err := cmdRole([]string{"builder", "frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got: %v", err)
	}
}

func TestCmdIngestRequiresFile(t *testing.T) {
	err := cmdIngest(nil)
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected file-required error, got: %v", err)
	}
}

func TestCmdCouncilRejectsUnknownFlag(t *testing.T) {
	err := cmdCouncil([]string{"--banana", "x"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Fatalf("error should mention unknown flag name, got: %v", err)
	}
}
