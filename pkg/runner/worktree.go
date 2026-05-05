package runner

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/hive/pkg/safety"
)

// mergeToMainMu serializes all MergeToMain calls. git checkout+merge is not
// atomic; concurrent calls would interleave and corrupt the repo state.
var mergeToMainMu sync.Mutex

// WorktreeContext wraps a git worktree for a single Builder task.
// Created before Operate, merged or cleaned up after.
type WorktreeContext struct {
	Dir       string // worktree working directory
	Branch    string // branch name in the worktree
	SourceDir string // original repo directory (for merge-back)
	TaskID    string // for tracking
}

// CreateTaskWorktree creates an isolated git worktree for a Builder task.
// The worktree starts on a new branch `hive/{slug}-{unix}` based on HEAD.
func CreateTaskWorktree(repoDir, taskTitle, taskID string) (*WorktreeContext, error) {
	now := time.Now()
	slug := branchSlug(taskTitle, now)
	ts := now.Unix()
	branch := fmt.Sprintf("hive/%s-%d", slug, ts)
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("hive-wt-%s-%d", slug, ts))

	// Create the worktree detached, then create the branch inside it.
	cmd := exec.Command("git", "worktree", "add", "--detach", dir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("create worktree: %w\n%s", err, out)
	}

	// Create and switch to the feature branch.
	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Cleanup on failure.
		os.RemoveAll(dir)
		gitPrune(repoDir)
		return nil, fmt.Errorf("create branch: %w\n%s", err, out)
	}

	// Configure git identity in the worktree (cmd.Dir = dir, not main repo).
	if err := gitIn(dir, "config", "user.name", "hive"); err != nil {
		log.Printf("[worktree] warning: git config user.name: %v", err)
	}
	if err := gitIn(dir, "config", "user.email", "hive@lovyou.ai"); err != nil {
		log.Printf("[worktree] warning: git config user.email: %v", err)
	}

	// Fix go.mod replace directives — worktree is in temp dir, not next to siblings.
	if err := linkReplaceTargets(dir, repoDir); err != nil {
		log.Printf("[worktree] warning: go.mod link fixup: %v", err)
	}

	log.Printf("[worktree] created %s → %s", branch, dir)
	return &WorktreeContext{
		Dir:       dir,
		Branch:    branch,
		SourceDir: repoDir,
		TaskID:    taskID,
	}, nil
}

// MergeToMain merges the worktree branch into main in the source repo.
// Uses --no-ff to preserve branch history in the audit trail.
// Serialized by mergeToMainMu: git checkout+merge is not atomic and concurrent
// calls would interleave and corrupt the repository state in daemon mode.
func (wc *WorktreeContext) MergeToMain() error {
	mergeToMainMu.Lock()
	defer mergeToMainMu.Unlock()

	if err := safety.RequireAuthorized(safety.ActionRepoMergeMain); err != nil {
		log.Printf("[worktree] main_merge.blocked branch=%s source=%s: %v", wc.Branch, wc.SourceDir, err)
		return err
	}

	// Switch source repo to main.
	if err := gitIn(wc.SourceDir, "checkout", "main"); err != nil {
		return fmt.Errorf("checkout main: %w", err)
	}

	// Pull latest to minimize conflicts.
	_ = gitIn(wc.SourceDir, "pull", "--ff-only")

	// Merge the worktree branch.
	if err := gitIn(wc.SourceDir, "merge", "--no-ff", wc.Branch, "-m",
		fmt.Sprintf("[hive:merge] %s", wc.Branch)); err != nil {
		// Abort the failed merge to leave repo clean.
		_ = gitIn(wc.SourceDir, "merge", "--abort")
		return fmt.Errorf("merge %s into main: %w (merge conflict — escalate)", wc.Branch, err)
	}

	log.Printf("[worktree] merged %s into main", wc.Branch)
	return nil
}

// Cleanup removes the worktree directory, prunes references, and deletes
// the feature branch. Safe to call multiple times.
func (wc *WorktreeContext) Cleanup() {
	if wc.Dir != "" {
		os.RemoveAll(wc.Dir)
	}
	gitPrune(wc.SourceDir)
	// Delete the branch (may fail if not merged — that's fine).
	_ = gitIn(wc.SourceDir, "branch", "-D", wc.Branch)
	log.Printf("[worktree] cleaned up %s", wc.Branch)
}

// linkReplaceTargets creates junctions/symlinks for go.mod replace directives
// that use relative paths. Handles both single-line and multi-line replace blocks.
func linkReplaceTargets(worktreeDir, sourceDir string) error {
	gomod := filepath.Join(worktreeDir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Extract all relative replace targets from both formats:
	// replace X => ../relative
	// replace ( X => ../relative \n Y => ../other )
	lines := strings.Split(string(data), "\n")
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "replace (" {
			inBlock = true
			continue
		}
		if inBlock && trimmed == ")" {
			inBlock = false
			continue
		}

		var target string
		if inBlock {
			// Inside replace block: "module => ../path"
			parts := strings.SplitN(trimmed, "=>", 2)
			if len(parts) == 2 {
				target = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(trimmed, "replace ") {
			// Single-line: "replace module => ../path"
			parts := strings.SplitN(trimmed, "=>", 2)
			if len(parts) == 2 {
				target = strings.TrimSpace(parts[1])
			}
		}

		if target == "" || !strings.HasPrefix(target, ".") {
			continue
		}

		realPath := filepath.Clean(filepath.Join(sourceDir, target))
		if _, err := os.Stat(realPath); err != nil {
			continue
		}
		expectedPath := filepath.Clean(filepath.Join(worktreeDir, target))
		if _, err := os.Stat(expectedPath); err == nil {
			continue // already exists
		}

		os.MkdirAll(filepath.Dir(expectedPath), 0755)
		// Symlink (works on Linux, macOS). On Windows, try junction first
		// (works without admin), fall back to symlink.
		if runtime.GOOS == "windows" {
			cmd := exec.Command("cmd", "/c", "mklink", "/J", expectedPath, realPath)
			if cmd.Run() == nil {
				continue
			}
		}
		if err := os.Symlink(realPath, expectedPath); err != nil {
			log.Printf("[worktree] warning: could not link %s → %s: %v", target, realPath, err)
		}
	}
	return nil
}

func gitIn(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

func gitPrune(dir string) {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = dir
	_ = cmd.Run()
}
