package runner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendDiagnosticCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
		t.Fatal(err)
	}

	pe := PhaseEvent{
		Phase:        "builder",
		Outcome:      "failure",
		Error:        "build failed",
		CostUSD:      0.0012,
		InputTokens:  100,
		OutputTokens: 50,
	}
	if err := appendDiagnostic(dir, pe); err != nil {
		t.Fatalf("appendDiagnostic: %v", err)
	}

	path := filepath.Join(dir, "loop", "diagnostics.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	var got PhaseEvent
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, data)
	}
	if got.Phase != pe.Phase {
		t.Errorf("Phase: got %q, want %q", got.Phase, pe.Phase)
	}
	if got.Timestamp == "" {
		t.Error("Timestamp must be set")
	}
}

func TestAppendDiagnosticAppendsLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
		t.Fatal(err)
	}

	first := PhaseEvent{Phase: "scout", Outcome: "failure", Error: "timeout"}
	second := PhaseEvent{Phase: "critic", Outcome: "failure", Error: "parse error"}

	if err := appendDiagnostic(dir, first); err != nil {
		t.Fatalf("first appendDiagnostic: %v", err)
	}
	if err := appendDiagnostic(dir, second); err != nil {
		t.Fatalf("second appendDiagnostic: %v", err)
	}

	path := filepath.Join(dir, "loop", "diagnostics.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var events []PhaseEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e PhaseEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %v", err)
		}
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(events))
	}
	if events[0].Phase != first.Phase {
		t.Errorf("line 1 Phase: got %q, want %q", events[0].Phase, first.Phase)
	}
	if events[1].Phase != second.Phase {
		t.Errorf("line 2 Phase: got %q, want %q", events[1].Phase, second.Phase)
	}
}
