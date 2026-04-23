package runner

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/transpara-ai/hive/pkg/api"
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

// TestPhaseEventNewFieldsRoundTrip verifies that the fields added to PhaseEvent
// (TaskID, TaskTitle, Repo, GitHash, FilesChanged, ReviseCount, BoardOpen)
// survive a JSON marshal/unmarshal cycle.  These fields were added to give the
// Observer enough signal to detect inefficiency and scope creep — missing them
// would silently drop diagnostic data.
func TestPhaseEventNewFieldsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
		t.Fatal(err)
	}

	pe := PhaseEvent{
		Phase:        "builder",
		Outcome:      "task.done",
		TaskID:       "task-abc",
		TaskTitle:    "Add quorum logic",
		Repo:         "hive",
		GitHash:      "deadbeef",
		FilesChanged: 3,
		ReviseCount:  1,
		BoardOpen:    4,
		InputTokens:  200,
		OutputTokens: 80,
		DurationSecs: 12.5,
		CostUSD:      0.005,
		Timestamp:    "2026-03-28T00:00:00Z",
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

	if got.TaskID != pe.TaskID {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, pe.TaskID)
	}
	if got.TaskTitle != pe.TaskTitle {
		t.Errorf("TaskTitle: got %q, want %q", got.TaskTitle, pe.TaskTitle)
	}
	if got.Repo != pe.Repo {
		t.Errorf("Repo: got %q, want %q", got.Repo, pe.Repo)
	}
	if got.GitHash != pe.GitHash {
		t.Errorf("GitHash: got %q, want %q", got.GitHash, pe.GitHash)
	}
	if got.FilesChanged != pe.FilesChanged {
		t.Errorf("FilesChanged: got %d, want %d", got.FilesChanged, pe.FilesChanged)
	}
	if got.ReviseCount != pe.ReviseCount {
		t.Errorf("ReviseCount: got %d, want %d", got.ReviseCount, pe.ReviseCount)
	}
	if got.BoardOpen != pe.BoardOpen {
		t.Errorf("BoardOpen: got %d, want %d", got.BoardOpen, pe.BoardOpen)
	}
}

func TestCountDiagnostics(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
		t.Fatal(err)
	}

	if got := countDiagnostics(dir); got != 0 {
		t.Fatalf("expected 0 before any writes, got %d", got)
	}

	e := PhaseEvent{Phase: "test", Outcome: "failure", Timestamp: "2026-01-01T00:00:00Z"}
	if err := appendDiagnostic(dir, e); err != nil {
		t.Fatal(err)
	}
	if got := countDiagnostics(dir); got != 1 {
		t.Fatalf("expected 1 after one append, got %d", got)
	}

	if err := appendDiagnostic(dir, e); err != nil {
		t.Fatal(err)
	}
	if got := countDiagnostics(dir); got != 2 {
		t.Fatalf("expected 2 after two appends, got %d", got)
	}
}

// makeLoopDir creates a temp dir with the expected loop/ sub-directory.
func makeLoopDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRunnerAppendDiagnostic_WritesFileOnly verifies that when only HiveDir is
// set (no APIClient), the event is written to diagnostics.jsonl and no HTTP call
// is made.
func TestRunnerAppendDiagnostic_WritesFileOnly(t *testing.T) {
	dir := makeLoopDir(t)
	r := New(Config{HiveDir: dir})

	r.appendDiagnostic(PhaseEvent{Phase: "scout", Outcome: "success"})

	if got := countDiagnostics(dir); got != 1 {
		t.Errorf("diagnostics count = %d, want 1", got)
	}
}

// TestRunnerAppendDiagnostic_PostsOnly verifies that when only APIClient is set
// (no HiveDir), the event is POSTed but no file is written.
func TestRunnerAppendDiagnostic_PostsOnly(t *testing.T) {
	var postCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/hive/diagnostic" {
			postCount++
			io.Copy(io.Discard, r.Body)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	r := New(Config{APIClient: api.New(srv.URL, "key")})
	r.appendDiagnostic(PhaseEvent{Phase: "builder", Outcome: "success"})

	if postCount != 1 {
		t.Errorf("POST count = %d, want 1", postCount)
	}
}

// TestRunnerAppendDiagnostic_WritesBoth verifies that when both HiveDir and
// APIClient are set, the event is written to the file AND POSTed.
func TestRunnerAppendDiagnostic_WritesBoth(t *testing.T) {
	dir := makeLoopDir(t)

	var postCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/hive/diagnostic" {
			postCount++
			io.Copy(io.Discard, r.Body)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	r := New(Config{HiveDir: dir, APIClient: api.New(srv.URL, "key")})
	r.appendDiagnostic(PhaseEvent{Phase: "critic", Outcome: "revise"})

	if got := countDiagnostics(dir); got != 1 {
		t.Errorf("file count = %d, want 1", got)
	}
	if postCount != 1 {
		t.Errorf("POST count = %d, want 1", postCount)
	}
}

// TestRunnerAppendDiagnostic_NeitherSet verifies that with neither HiveDir nor
// APIClient set, appendDiagnostic does not panic.
func TestRunnerAppendDiagnostic_NeitherSet(t *testing.T) {
	r := New(Config{Role: "builder"})
	// Must not panic.
	r.appendDiagnostic(PhaseEvent{Phase: "builder", Outcome: "success"})
}
