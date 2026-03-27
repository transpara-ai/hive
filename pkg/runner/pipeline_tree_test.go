package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovyou-ai/hive/pkg/api"
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

// TestNewPipelineTreeWiresFixTasker verifies the production path: NewPipelineTree
// sets a non-nil fixTasker when the runner has an APIClient. Without this,
// callFixTasker silently skips task creation on every phase failure.
func TestNewPipelineTreeWiresFixTasker(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", nil)
	client := api.New("http://localhost", "test-key")
	r := New(Config{HiveDir: hiveDir, APIClient: client, SpaceSlug: "hive"})

	pt := NewPipelineTree(r)

	if pt.fixTasker == nil {
		t.Fatal("NewPipelineTree: fixTasker is nil when APIClient is set — fix tasks will never be created")
	}
}

// TestClientFixTaskerCallsAPI verifies the adapter forwards CreateTask to the
// api.Client with the right slug and title, bridging the interface mismatch.
func TestClientFixTaskerCallsAPI(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"new-1","kind":"task","title":"Fix: scout phase failed","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	ft := &clientFixTasker{
		client: api.New(srv.URL, "test-key"),
		slug:   "hive",
	}

	if err := ft.CreateTask(context.Background(), "Fix: scout phase failed"); err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if !strings.Contains(gotPath, "/hive/") {
		t.Errorf("request path %q does not contain space slug", gotPath)
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

// TestPipelineTreeTesterFailureWritesExactlyOneDiagnostic verifies that when
// the tester phase writes its own diagnostic and returns an error, Execute does
// not append a second diagnostic — total count must be exactly 1.
func TestPipelineTreeTesterFailureWritesExactlyOneDiagnostic(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", nil)

	pt := &PipelineTree{
		cfg: Config{HiveDir: hiveDir},
		phases: []Phase{
			{
				Name: "tester",
				Run: func(_ context.Context) error {
					_ = appendDiagnostic(hiveDir, PhaseEvent{
						Phase:   "tester",
						Outcome: "test_failure",
						Error:   "tests failed",
					})
					return fmt.Errorf("tests failed")
				},
			},
		},
	}

	_ = pt.Execute(context.Background())

	if got := countDiagnostics(hiveDir); got != 1 {
		t.Errorf("diagnostic count: got %d, want 1 (duplicate written)", got)
	}
}

// TestNewPipelineTreeHasSevenPhases verifies that NewPipelineTree wires exactly
// seven phases in order, with loop-clean-check immediately before reflector.
func TestNewPipelineTreeHasSevenPhases(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := New(Config{HiveDir: hiveDir})

	pt := NewPipelineTree(r)

	want := []string{"scout", "architect", "builder", "tester", "critic", "loop-clean-check", "reflector"}
	if len(pt.phases) != len(want) {
		t.Fatalf("phase count: got %d, want %d", len(pt.phases), len(want))
	}
	for i, name := range want {
		if pt.phases[i].Name != name {
			t.Errorf("phase[%d]: got %q, want %q", i, pt.phases[i].Name, name)
		}
	}
}

// TestLoopDirtyCheckBlocksReflector verifies that Execute returns an error and
// emits a diagnostic when loop/ contains uncommitted artifacts, and that the
// reflector phase is never reached.
func TestLoopDirtyCheckBlocksReflector(t *testing.T) {
	repoDir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	// Create an uncommitted loop/build.md in the repo.
	loopDir := filepath.Join(repoDir, "loop")
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loopDir, "build.md"), []byte("# Build\n"), 0644); err != nil {
		t.Fatal(err)
	}

	reflectorCalled := false
	pt := &PipelineTree{
		cfg: Config{HiveDir: repoDir, RepoPath: ""},
	}
	pt.phases = []Phase{
		{Name: "loop-clean-check", Run: func(ctx context.Context) error { return pt.loopDirtyCheck(ctx) }},
		{Name: "reflector", Run: func(_ context.Context) error {
			reflectorCalled = true
			return nil
		}},
	}

	err := pt.Execute(context.Background())
	if err == nil {
		t.Fatal("Execute returned nil, want error for dirty loop/ files")
	}
	if reflectorCalled {
		t.Error("reflector was called despite loop-clean-check failing")
	}
	if countDiagnostics(repoDir) == 0 {
		t.Error("no diagnostic emitted for loop-clean-check failure")
	}
}

// TestPipelineTreeReflectorSkippedOnRevise verifies the REVISE gate: when
// loop/critique.md contains VERDICT: REVISE, the reflector phase returns nil
// without calling runReflector. If the gate is absent, runReflector is called
// with a nil Provider and panics — which the test runner reports as a failure.
func TestPipelineTreeReflectorSkippedOnRevise(t *testing.T) {
	hiveDir := makeHiveDir(t, "# State\n", map[string]string{
		"critique.md": "VERDICT: REVISE\n",
	})

	r := New(Config{HiveDir: hiveDir})
	pt := NewPipelineTree(r)

	// Isolate the real reflector phase (which contains the REVISE gate).
	var reflectorPhase Phase
	for _, p := range pt.phases {
		if p.Name == "reflector" {
			reflectorPhase = p
			break
		}
	}
	pt.phases = []Phase{reflectorPhase}

	err := pt.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute returned error when reflector should be skipped via REVISE gate: %v", err)
	}
	if countDiagnostics(hiveDir) != 0 {
		t.Errorf("diagnostics written when reflector should have been skipped cleanly")
	}
}
