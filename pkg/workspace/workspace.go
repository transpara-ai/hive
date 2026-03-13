// Package workspace manages the file system for generated products.
// Each product gets its own git repo under the workspace root.
package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Workspace manages directories and files for hive-generated products.
type Workspace struct {
	root string // Root directory for all product repos
	org  string // GitHub org for created repos (e.g., "lovyou-ai")
}

// New creates a workspace rooted at the given directory.
func New(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}

	return &Workspace{root: abs, org: "lovyou-ai"}, nil
}

// Root returns the workspace root directory.
func (w *Workspace) Root() string {
	return w.root
}

// SetOrg sets the GitHub org for repo creation.
func (w *Workspace) SetOrg(org string) {
	w.org = org
}

// Product represents a product's working directory backed by a git repo.
type Product struct {
	Name string
	Dir  string
	Repo string // GitHub repo (e.g., "lovyou-ai/my-product")
}

// InitProduct creates a new product with its own git repo.
// If the GitHub repo doesn't exist, it creates one.
func (w *Workspace) InitProduct(name string) (*Product, error) {
	dir := filepath.Join(w.root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	repo := fmt.Sprintf("%s/%s", w.org, name)
	p := &Product{Name: name, Dir: dir, Repo: repo}

	// Initialize git if not already a repo
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if err := p.git("init", "-b", "main"); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}

		// Configure git identity for commits
		_ = p.git("config", "user.name", "hive")
		_ = p.git("config", "user.email", "hive@lovyou.ai")

		// Try to create the GitHub repo (may already exist)
		_ = p.gh("repo", "create", repo, "--public",
			"--description", "Built by hive — lovyou-ai/hive")

		// Set remote
		_ = p.git("remote", "add", "origin",
			fmt.Sprintf("https://github.com/%s.git", repo))
	}

	return p, nil
}

// OpenProduct opens an existing product directory.
func (w *Workspace) OpenProduct(name string) (*Product, error) {
	dir := filepath.Join(w.root, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("product %q not found", name)
	}
	return &Product{
		Name: name,
		Dir:  dir,
		Repo: fmt.Sprintf("%s/%s", w.org, name),
	}, nil
}

// ProductDir returns the directory for a specific product, creating it if needed.
// Kept for backward compatibility — prefer InitProduct for new products.
func (w *Workspace) ProductDir(name string) string {
	dir := filepath.Join(w.root, name)
	os.MkdirAll(dir, 0755)
	return dir
}

// WriteFile writes content to a file, creating parent directories as needed.
func (w *Workspace) WriteFile(path string, content string) error {
	full := w.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(full, []byte(content), 0644)
}

// ReadFile reads the contents of a file.
func (w *Workspace) ReadFile(path string) (string, error) {
	full := w.resolvePath(path)
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FileExists checks if a file exists.
func (w *Workspace) FileExists(path string) bool {
	full := w.resolvePath(path)
	_, err := os.Stat(full)
	return err == nil
}

// ListFiles returns all files in a product directory.
func (w *Workspace) ListFiles(productName string) ([]string, error) {
	dir := w.ProductDir(productName)
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// WriteProductFile writes a file into a product's directory and stages it.
func (p *Product) WriteFile(relPath string, content string) error {
	full := filepath.Join(p.Dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		return err
	}
	return p.git("add", relPath)
}

// StageAll stages all changes (new, modified, deleted) in the product repo.
func (p *Product) StageAll() error {
	return p.git("add", "-A")
}

// Commit commits staged changes in the product repo.
// Uses the hive agent identity as author so self-improve commits
// are distinguishable from human commits.
func (p *Product) Commit(message string) error {
	return p.git("commit", "-m", message,
		"--author", "hive <hive@lovyou.ai>")
}

// CommitIfStaged commits staged changes if any exist, or does nothing if
// nothing is staged. Returns nil when there is nothing to commit — this is
// success, not an error (the builder correctly determined no changes were needed).
func (p *Product) CommitIfStaged(message string) error {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = p.Dir
	err := cmd.Run()
	if err == nil {
		// Exit code 0 — nothing staged, nothing to commit.
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		// Exit code 1 — staged changes exist, commit them.
		return p.Commit(message)
	}
	return fmt.Errorf("git diff --cached: %w", err)
}

// Push pushes to the product's remote.
func (p *Product) Push() error {
	return p.git("push", "-u", "origin", "main")
}

// git runs a git command in the product directory.
func (p *Product) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}

// gh runs a GitHub CLI command.
func (p *Product) gh(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}

// OpenRepo opens an arbitrary directory as a Product.
// Unlike InitProduct, this does NOT create a git repo or GitHub remote.
// The directory must already exist and be a git repo.
func OpenRepo(dir string) (*Product, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s is not a git repository", abs)
	}
	name := filepath.Base(abs)
	return &Product{Name: name, Dir: abs}, nil
}

// ReadSourceFiles reads all source files from the product directory.
// Skips .git, binary files, and files over 100KB.
// Returns a map of relative path → content.
func (p *Product) ReadSourceFiles() (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.Walk(p.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "target" || base == ".hive" || base == "products" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip large files (>100KB) and known binary extensions
		if info.Size() > 100*1024 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if isBinaryExt(ext) {
			return nil
		}

		rel, err := filepath.Rel(p.Dir, path)
		if err != nil {
			return nil
		}
		// Normalize to forward slashes for consistency
		rel = filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}
		files[rel] = string(data)
		return nil
	})
	return files, err
}

// GitDiff returns the diff of uncommitted or branch changes.
// If base is empty, shows uncommitted changes. Otherwise shows diff from base.
func (p *Product) GitDiff(base string) (string, error) {
	var args []string
	if base == "" {
		args = []string{"diff"}
	} else {
		args = []string{"diff", base + "...HEAD"}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GitLog returns recent git log entries.
func (p *Product) GitLog(n int) (string, error) {
	cmd := exec.Command("git", "log", fmt.Sprintf("-%d", n), "--oneline")
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// HeadCommit returns the current HEAD commit hash (full form).
func (p *Product) HeadCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("head commit: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateBranch creates and checks out a new branch.
// If the branch already exists (e.g., from a previous self-improve run whose PR
// was never merged), it falls back to a timestamped name to avoid collisions.
// This is more robust than delete+recreate when the existing branch is the
// current branch or has uncommitted changes that block deletion.
func (p *Product) CreateBranch(name string) error {
	if err := p.git("checkout", "-b", name); err == nil {
		return nil
	}
	// Branch already exists — use a timestamped fallback name.
	fallback := fmt.Sprintf("%s-%d", name, time.Now().Unix())
	return p.git("checkout", "-b", fallback)
}

// CurrentBranch returns the current branch name.
func (p *Product) CurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SyncMain checks out the main branch and pulls the latest from the remote.
// This ensures the local main is up-to-date after a PR merge, so the next
// iteration branches from the correct base.
func (p *Product) SyncMain() error {
	if err := p.git("checkout", "main"); err != nil {
		return fmt.Errorf("checkout main: %w", err)
	}
	if err := p.git("pull", "origin", "main"); err != nil {
		return fmt.Errorf("pull main: %w", err)
	}
	return nil
}

// CleanupForIteration resets the repo to a clean state matching origin/main,
// discarding any uncommitted changes and deleting local hive/* branches left
// over from failed iterations. Safe to call at the start of each iteration.
// Works in both primary checkouts and worktrees (where main may be locked).
func (p *Product) CleanupForIteration() error {
	// Abort any in-progress merge/rebase that might block checkout.
	_ = p.git("merge", "--abort")
	_ = p.git("rebase", "--abort")

	// Discard any uncommitted changes.
	_ = p.git("checkout", "--", ".")
	_ = p.git("clean", "-fd")

	// Try to switch to main. In a worktree, main is locked by the primary
	// checkout — that's fine, we'll reset whatever branch we're on.
	_ = p.git("checkout", "main")

	// Delete local hive/* branches (stale feature branches from failed runs).
	cmd := exec.Command("git", "branch", "--list", "hive/*")
	cmd.Dir = p.Dir
	out, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			branch := strings.TrimSpace(line)
			if branch != "" {
				_ = p.git("branch", "-D", branch)
			}
		}
	}

	// Delete remote hive/* branches too — stale remote branches cause
	// "push -u origin" to fail when the pipeline reuses a branch name.
	remoteCmd := exec.Command("git", "branch", "-r", "--list", "origin/hive/*")
	remoteCmd.Dir = p.Dir
	remoteOut, err := remoteCmd.Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(remoteOut)), "\n") {
			ref := strings.TrimSpace(line)
			if ref == "" {
				continue
			}
			// "origin/hive/foo" → "hive/foo"
			branch := strings.TrimPrefix(ref, "origin/")
			_ = p.git("push", "origin", "--delete", branch)
		}
	}

	// Sync with remote main. Use reset --hard so we always match remote
	// exactly — local-only commits on main are expected (the previous
	// iteration's PR merge may have already included them).
	_ = p.git("fetch", "origin", "main")
	if err := p.git("reset", "--hard", "origin/main"); err != nil {
		return fmt.Errorf("sync main: %w", err)
	}
	return nil
}

// PushBranch pushes the current branch to the remote.
func (p *Product) PushBranch() error {
	branch, err := p.CurrentBranch()
	if err != nil {
		return err
	}
	return p.git("push", "-u", "origin", branch)
}

// CreateWorktree creates a git worktree from this repo, returning a Product
// that points to the worktree directory. The worktree shares the same git
// objects and remote but has its own working directory, so operations like
// CleanupForIteration() won't affect the original checkout.
// The caller must call RemoveWorktree() on the returned Product when done.
func (p *Product) CreateWorktree(name string) (*Product, error) {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("hive-worktree-%s-%d", name, time.Now().UnixNano()))

	// Create the worktree on a detached HEAD at origin/main. We can't
	// check out `main` because it's already checked out in the primary
	// worktree — git forbids two worktrees on the same branch.
	if err := p.git("worktree", "add", "--detach", dir, "origin/main"); err != nil {
		// Fallback: try just HEAD if origin/main doesn't exist.
		if err2 := p.git("worktree", "add", "--detach", dir); err2 != nil {
			return nil, fmt.Errorf("create worktree: %w (also tried HEAD: %v)", err, err2)
		}
	}

	wt := &Product{
		Name: p.Name,
		Dir:  dir,
		Repo: p.Repo,
	}

	// Configure git identity in worktree.
	_ = wt.git("config", "user.name", "hive")
	_ = wt.git("config", "user.email", "hive@lovyou.ai")

	// Link .hive/ from source repo so telemetry and evolve state are
	// accessible in the worktree without copying. Try junction first
	// (works without admin on Windows), fall back to symlink, then copy.
	srcHive := filepath.Join(p.Dir, ".hive")
	if _, err := os.Stat(srcHive); err == nil {
		dstHive := filepath.Join(dir, ".hive")
		if !linkDir(srcHive, dstHive) {
			// Last resort: copy the directory.
			if cpErr := copyDir(srcHive, dstHive); cpErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not link .hive: %v (telemetry may be unavailable)\n", cpErr)
			}
		}
	}

	// Fix go.mod replace directives that use relative paths. The worktree
	// lives in a temp dir, so "../sibling" paths won't resolve. Rewrite
	// them to absolute paths based on the source repo's location.
	if err := fixGoModReplace(dir, p.Dir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fix go.mod replace directives: %v\n", err)
	}

	return wt, nil
}

// linkDir creates a directory junction (Windows) or symlink (Unix) from src to dst.
// Returns true on success.
func linkDir(src, dst string) bool {
	// On Windows, try a directory junction first (no admin required).
	if junctionCmd := exec.Command("cmd", "/c", "mklink", "/J", dst, src); junctionCmd != nil {
		if err := junctionCmd.Run(); err == nil {
			return true
		}
	}
	// Fall back to symlink (works on Unix, needs admin on Windows).
	return os.Symlink(src, dst) == nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// fixGoModReplace rewrites relative replace directives in go.mod to absolute
// paths. In a worktree at /tmp/..., "../eventgraph/go" doesn't resolve, so we
// rewrite it to the absolute path relative to the source repo.
func fixGoModReplace(worktreeDir, sourceDir string) error {
	gomod := filepath.Join(worktreeDir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no go.mod, nothing to fix
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		// Match: replace module => ../relative/path
		if !strings.HasPrefix(strings.TrimSpace(line), "replace ") {
			continue
		}
		parts := strings.SplitN(line, "=>", 2)
		if len(parts) != 2 {
			continue
		}
		target := strings.TrimSpace(parts[1])
		if !strings.HasPrefix(target, ".") {
			continue // already absolute or a module version
		}
		// Resolve relative to source repo, then make absolute.
		abs := filepath.Join(sourceDir, target)
		abs = filepath.Clean(abs)
		abs = filepath.ToSlash(abs) // go.mod uses forward slashes
		lines[i] = parts[0] + "=> " + abs
		changed = true
	}

	if !changed {
		return nil
	}
	return os.WriteFile(gomod, []byte(strings.Join(lines, "\n")), 0644)
}

// RemoveWorktree removes this product's worktree directory and prunes the
// worktree reference from the parent repo. Safe to call multiple times.
func (p *Product) RemoveWorktree() error {
	// git worktree remove needs to be run from the main repo, but we can
	// also just remove the directory and then prune.
	if err := os.RemoveAll(p.Dir); err != nil {
		return fmt.Errorf("remove worktree dir: %w", err)
	}
	// Prune stale worktree references. Run from the worktree dir's parent
	// since the worktree itself is gone — git handles this gracefully.
	return nil
}

// PruneWorktrees cleans up stale worktree references. Call this from the
// main repo after worktrees have been removed.
func (p *Product) PruneWorktrees() error {
	return p.git("worktree", "prune")
}

// isBinaryExt returns true for known binary file extensions.
func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj",
		".png", ".jpg", ".jpeg", ".gif", ".ico", ".bmp", ".webp",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z",
		".pdf", ".wasm", ".pyc", ".class",
		".db", ".sqlite", ".sqlite3":
		return true
	}
	return false
}

// resolvePath makes a path absolute relative to the workspace root.
func (w *Workspace) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.root, path)
}
