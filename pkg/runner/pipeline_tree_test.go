package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubFixTasker struct {
	calls []string
}

func (s *stubFixTasker) CreateTask(_ context.Context, title string) error {
	s.calls = append(s.calls, title)
	return nil
}

func TestPipelineTreeFailureWritesDiagnostic(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", nil)

	pt := &PipelineTree{
		cfg: Config{HiveDir: hiveDir},
		phases: []Phase{
			{
				Name: "stub",
				Run: func(_ context.Context) error {
					return fmt.Errorf("injected failure")
				},
			},
		},
	}

	_ = pt.Execute(context.Background())

	path := filepath.Join(hiveDir, "loop", "diagnostics.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("diagnostics.jsonl not created: %v", err)
	}

	sc := bufio.NewScanner(strings.NewReader(string(data)))
	if !sc.Scan() {
		t.Fatal("diagnostics.jsonl is empty")
	}

	var e PhaseEvent
	if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, sc.Bytes())
	}
	if e.Outcome != "failure" {
		t.Errorf("outcome: got %q, want %q", e.Outcome, "failure")
	}
	if e.Phase != "stub" {
		t.Errorf("phase: got %q, want %q", e.Phase, "stub")
	}
}

func TestPipelineTreeFixTaskerCalledOnDiagnosticWithNilReturn(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", nil)
	stub := &stubFixTasker{}

	pt := &PipelineTree{
		cfg: Config{HiveDir: hiveDir},
		phases: []Phase{
			{
				Name: "scout",
				Run: func(_ context.Context) error {
					_ = appendDiagnostic(hiveDir, PhaseEvent{
						Phase:   "scout",
						Outcome: "failure",
						Error:   "internal failure",
					})
					return nil
				},
			},
		},
		fixTasker: stub,
	}

	err := pt.Execute(context.Background())
	if err == nil {
		t.Fatal("Execute returned nil, want error")
	}
	if len(stub.calls) != 1 {
		t.Fatalf("CreateTask called %d times, want 1", len(stub.calls))
	}
	const wantTitle = "Fix: scout phase failed"
	if stub.calls[0] != wantTitle {
		t.Errorf("CreateTask title = %q, want %q", stub.calls[0], wantTitle)
	}
}

func TestPipelineTreeFixTaskerCalledOnDirectError(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", nil)
	stub := &stubFixTasker{}

	pt := &PipelineTree{
		cfg: Config{HiveDir: hiveDir},
		phases: []Phase{
			{
				Name: "builder",
				Run: func(_ context.Context) error {
					return fmt.Errorf("build failed")
				},
			},
		},
		fixTasker: stub,
	}

	err := pt.Execute(context.Background())
	if err == nil {
		t.Fatal("Execute returned nil, want error")
	}
	if len(stub.calls) != 1 {
		t.Fatalf("CreateTask called %d times, want 1", len(stub.calls))
	}
	const wantTitle = "Fix: builder phase failed"
	if stub.calls[0] != wantTitle {
		t.Errorf("CreateTask title = %q, want %q", stub.calls[0], wantTitle)
	}
}
