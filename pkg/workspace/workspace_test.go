package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	ws, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ws.Root() != dir {
		// On Windows, TempDir may not match abs exactly
		abs, _ := filepath.Abs(dir)
		if ws.Root() != abs {
			t.Errorf("Root = %q, want %q", ws.Root(), dir)
		}
	}
}

func TestWriteAndReadFile(t *testing.T) {
	ws, _ := New(t.TempDir())
	content := "package main\n\nfunc main() {}\n"

	err := ws.WriteFile("myproject/main.go", content)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ws.ReadFile("myproject/main.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != content {
		t.Errorf("ReadFile = %q, want %q", got, content)
	}
}

func TestProductDir(t *testing.T) {
	ws, _ := New(t.TempDir())
	dir := ws.ProductDir("alpha")

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("ProductDir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("ProductDir is not a directory")
	}
}

func TestFileExists(t *testing.T) {
	ws, _ := New(t.TempDir())

	if ws.FileExists("nonexistent.go") {
		t.Error("FileExists should return false for missing file")
	}

	ws.WriteFile("exists.go", "package x")
	if !ws.FileExists("exists.go") {
		t.Error("FileExists should return true for existing file")
	}
}

func TestReadSourceFilesSkipsHiveAndProducts(t *testing.T) {
	dir := t.TempDir()
	prod := &Product{Name: "test", Dir: dir}

	// Create source files that should be included
	writeFile(t, dir, "main.go", "package main")
	writeFile(t, dir, "lib/util.go", "package lib")

	// Create .hive telemetry files that should be skipped
	writeFile(t, dir, ".hive/telemetry.json", `{"tokens": 100}`)
	writeFile(t, dir, ".hive/run-001.json", `{"status": "done"}`)

	// Create products files that should be skipped
	writeFile(t, dir, "products/app/main.go", "package main // generated")
	writeFile(t, dir, "products/app/go.mod", "module app")

	files, err := prod.ReadSourceFiles()
	if err != nil {
		t.Fatalf("ReadSourceFiles: %v", err)
	}

	// Should include source files
	if _, ok := files["main.go"]; !ok {
		t.Error("missing main.go")
	}
	if _, ok := files["lib/util.go"]; !ok {
		t.Error("missing lib/util.go")
	}

	// Should NOT include .hive or products files
	for path := range files {
		if strings.HasPrefix(path, ".hive") {
			t.Errorf("should skip .hive file: %s", path)
		}
		if strings.HasPrefix(path, "products") {
			t.Errorf("should skip products file: %s", path)
		}
	}

	if len(files) != 2 {
		t.Errorf("ReadSourceFiles = %d files, want 2; got: %v", len(files), keys(files))
	}
}

// initGitRepo initialises dir as a git repository with a single empty commit
// so tests have a valid HEAD to build on.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "test")
	run("config", "user.email", "test@test.com")
	run("commit", "--allow-empty", "-m", "initial")
}

// TestCommitUsesAgentIdentity locks BOTH the author and committer of a product
// commit to the transpara agent identity and guards against any reintroduction
// of a transpara identity — even when the product repo's local git config still
// carries the old identity (a repo created before the migration). git records
// the committer from repo config, so setting --author alone is not sufficient.
func TestCommitUsesAgentIdentity(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	p := &Product{Name: "test", Dir: dir}

	// Simulate a pre-migration product repo whose local config is still transpara.
	setGit := func(k, v string) {
		t.Helper()
		c := exec.Command("git", "config", k, v)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %s: %v", k, out, err)
		}
	}
	setGit("user.name", "hive")
	setGit("user.email", "hive@transpara.ai")

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := p.StageAll(); err != nil {
		t.Fatalf("StageAll: %v", err)
	}
	if err := p.Commit("add f"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	cmd := exec.Command("git", "log", "-1", "--pretty=%ae%n%ce")
	cmd.Dir = dir
	out, _ := cmd.Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected author+committer email on two lines, got %q", string(out))
	}

	for _, f := range []struct{ field, got string }{
		{"author", lines[0]},
		{"committer", lines[1]},
	} {
		if f.got != "ai-agent@transpara.com" {
			t.Errorf("commit %s email = %q, want %q", f.field, f.got, "ai-agent@transpara.com")
		}
		if strings.Contains(f.got, "transpara") {
			t.Errorf("commit %s email %q must not reference transpara", f.field, f.got)
		}
	}
}

func TestCreateWorktree_HappyPath(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	p := &Product{Name: "test", Dir: dir}

	wt, err := p.CreateWorktree("myworktree")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(wt.Dir) })

	if _, err := os.Stat(wt.Dir); err != nil {
		t.Fatalf("worktree dir does not exist: %v", err)
	}

	// git worktrees contain a .git file (not a directory).
	gitEntry := filepath.Join(wt.Dir, ".git")
	if _, err := os.Stat(gitEntry); err != nil {
		t.Fatalf("worktree has no .git entry: %v", err)
	}

	p.PruneWorktrees() // step 5: clean up stale refs
}

func TestRemoveWorktree(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	p := &Product{Name: "test", Dir: dir}

	wt, err := p.CreateWorktree("removeme")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	wtDir := wt.Dir

	if err := wt.RemoveWorktree(); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	if _, statErr := os.Stat(wtDir); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir still exists after RemoveWorktree")
	}
}

func TestCreateWorktree_NonGitDirErrors(t *testing.T) {
	dir := t.TempDir() // plain directory — no .git
	p := &Product{Name: "test", Dir: dir}

	_, err := p.CreateWorktree("fail")
	if err == nil {
		t.Fatal("expected error for non-git dir, got nil")
	}
}

func writeFile(t *testing.T, base, rel, content string) {
	t.Helper()
	path := filepath.Join(base, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestListFiles(t *testing.T) {
	ws, _ := New(t.TempDir())
	ws.WriteFile("proj/main.go", "package main")
	ws.WriteFile("proj/lib/util.go", "package lib")

	files, err := ws.ListFiles("proj")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ListFiles = %d files, want 2", len(files))
	}
}
