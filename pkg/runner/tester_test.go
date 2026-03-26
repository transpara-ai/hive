package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// makeTempGoModule creates a minimal Go module in a temp dir with one Go file
// containing a test function. passingTest controls whether the test passes or fails.
func makeTempGoModule(t *testing.T, passingTest bool) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testpkg\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	var testBody string
	if passingTest {
		testBody = `package testpkg

import "testing"

func TestPass(t *testing.T) {}
`
	} else {
		testBody = `package testpkg

import "testing"

func TestFail(t *testing.T) {
	t.Fatal("intentional failure")
}
`
	}

	if err := os.WriteFile(filepath.Join(dir, "pkg_test.go"), []byte(testBody), 0644); err != nil {
		t.Fatalf("write pkg_test.go: %v", err)
	}
	return dir
}

func TestRunTester_pass(t *testing.T) {
	hiveDir := makeHiveDir(t, "# Loop State\n\nLast updated: Iteration 1, 2026-01-01.\n", nil)
	repoPath := makeTempGoModule(t, true)

	r := &Runner{
		cfg: Config{
			HiveDir:  hiveDir,
			RepoPath: repoPath,
		},
	}

	err := r.runTester(context.Background())
	if err != nil {
		t.Fatalf("runTester returned error on passing tests: %v", err)
	}

	if count := countDiagnostics(hiveDir); count != 0 {
		t.Errorf("expected 0 diagnostics, got %d", count)
	}
}

func TestRunTester_fail(t *testing.T) {
	hiveDir := makeHiveDir(t, "# Loop State\n\nLast updated: Iteration 1, 2026-01-01.\n", nil)
	repoPath := makeTempGoModule(t, false)

	r := &Runner{
		cfg: Config{
			HiveDir:  hiveDir,
			RepoPath: repoPath,
		},
	}

	err := r.runTester(context.Background())
	if err == nil {
		t.Fatal("runTester should return error on failing tests")
	}

	// Read diagnostics.jsonl and assert a PhaseEvent with outcome="test_failure" exists.
	path := filepath.Join(hiveDir, "loop", "diagnostics.jsonl")
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("diagnostics.jsonl not created: %v", readErr)
	}

	var found bool
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var e PhaseEvent
		if jsonErr := json.Unmarshal(sc.Bytes(), &e); jsonErr != nil {
			t.Fatalf("invalid JSON line: %v", jsonErr)
		}
		if e.Phase == "tester" && e.Outcome == "test_failure" {
			found = true
			break
		}
	}
	if sc.Err() != nil {
		t.Fatalf("scan: %v", sc.Err())
	}
	if !found {
		t.Errorf("no PhaseEvent with phase=tester outcome=test_failure in diagnostics.jsonl:\n%s", data)
	}
}
