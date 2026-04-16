package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDaemonResetToMain verifies that daemonResetToMain force-checks out main
// even when the working tree is dirty (simulating loop/ artifacts).
func TestDaemonResetToMain(t *testing.T) {
	// Create a temporary bare repo to act as "origin".
	bare := t.TempDir()
	gitRun(t, bare, "git", "init", "--bare")

	// Clone it to get a working repo with origin configured.
	repo := filepath.Join(t.TempDir(), "repo")
	gitRun(t, "", "git", "clone", bare, repo)

	// Configure git user for commits.
	gitRun(t, repo, "git", "config", "user.email", "test@test.com")
	gitRun(t, repo, "git", "config", "user.name", "test")

	// Create an initial commit on main so the branch exists.
	initialFile := filepath.Join(repo, "init.txt")
	if err := os.WriteFile(initialFile, []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "git", "add", ".")
	gitRun(t, repo, "git", "commit", "-m", "initial commit")

	// Ensure we're on main (some git versions default to "master").
	// Try to rename; ignore error if already named main.
	exec.Command("git", "-C", repo, "branch", "-M", "main").Run()
	gitRun(t, repo, "git", "push", "-u", "origin", "main")

	// Create and switch to a feature branch.
	gitRun(t, repo, "git", "checkout", "-b", "feat/dirty-test")

	// Create a tracked file, commit it, then modify it (dirty tree).
	tracked := filepath.Join(repo, "loop", "daemon.status")
	if err := os.MkdirAll(filepath.Dir(tracked), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracked, []byte("clean"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "git", "add", ".")
	gitRun(t, repo, "git", "commit", "-m", "add loop artifact")

	// Now dirty the working tree.
	if err := os.WriteFile(tracked, []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify we're on the feature branch with dirty state.
	branch := currentBranch(t, repo)
	if branch != "feat/dirty-test" {
		t.Fatalf("expected feat/dirty-test, got %s", branch)
	}

	// Call the function under test.
	daemonResetToMain(repo)

	// Verify we're back on main.
	after := currentBranch(t, repo)
	if after != "main" {
		t.Fatalf("expected main after reset, got %s", after)
	}
}

func gitRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}

func currentBranch(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}
