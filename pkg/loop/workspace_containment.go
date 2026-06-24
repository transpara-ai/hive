package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/transpara-ai/work"
)

// Workspace containment (v10-F2 / Finding 18): the -repo workspace assignment
// alone does not contain a filesystem-capable Operate — the v10 round-3
// implementer simply walked to the sibling main checkout. This tripwire
// snapshots the watched sibling checkouts around the Operate and fails the
// run on any observed checkout-state delta — HEAD, current branch,
// working-tree status, checkout presence — in the immediate-child checkouts
// of the watch roots.
//
// SCOPE / THREAT MODEL — detection with run-failing teeth, NOT prevention and
// NOT an OS sandbox. A Bash-capable subprocess can still write anywhere its
// uid can; what it can no longer do is have work accepted after mutating a
// WATCHED state vector: that fails the task, halts the implementer, and
// escalates to the human BEFORE the untrusted summary can drive anything.
// Explicitly OUTSIDE the watched vectors (out of scope here, routed with the
// hard boundary to G-2.x/G-8.2): checkouts nested deeper than one level under
// a watch root, bare repos, repos reached by absolute path outside every
// watch root, non-git files in sibling directories, .git internals invisible
// to the watched vectors (remote-tracking refs, tags, refs created without a
// checkout, config), and mutations restored to the same observable state
// before the post-snapshot. The hard boundary (separate uid / container /
// namespace, credential-less run workspaces) remains routed to G-2.x/G-8.2,
// unchanged. Complements eventgraph#50 (credential isolation: the subprocess
// cannot push) and hive#151 (contract layer: gates must not demand pushes) —
// this is the runtime ring between them, per the take-3 mechanism: every
// layer bars the behavior again at its own altitude.
//
// PARALLELISM HONESTY: the tripwire attributes any watched-sibling delta to
// THIS loop's Operate. Two concurrent Operates in sibling workspaces would
// cross-trip each other. Slice-1 society runs launch a single Operate at a
// time; parallel FactoryOrders (G-7.x) must move attribution into the runtime
// — which sees every live run's workspace — before launching concurrent
// Operates. Recorded here as a residual, not a silent cap.

// repoContainmentState is the observed git state of one watched checkout.
type repoContainmentState struct {
	head   string // rev-parse HEAD ("(unborn)" for a commit-less repo)
	branch string // rev-parse --abbrev-ref HEAD (catches branch switches that return HEAD to the same hash)
	status string // status --porcelain (uncommitted-change fingerprint)
}

// containmentBaseline maps each watched checkout's resolved path to its state.
type containmentBaseline map[string]repoContainmentState

// containmentRoots returns the directories whose immediate child git
// checkouts the tripwire watches: the configured roots when set, otherwise
// the parent directory of the assigned workspace (which holds the sibling
// worktrees a run could walk to). Nil only when the loop has no workspace.
func (l *Loop) containmentRoots() []string {
	return l.containmentRootsFor(l.config.RepoPath, l.config.ContainmentWatchRoots)
}

func (l *Loop) containmentRootsFor(repoPath string, configured []string) []string {
	if len(configured) > 0 {
		return configured
	}
	if repoPath == "" {
		return nil
	}
	return []string{filepath.Dir(repoPath)}
}

// resolvePath canonicalizes a path for identity comparison — the repos tree
// is commonly reached through symlinks, and a workspace addressed via a
// symlink must still be excluded from the sibling watch (a workspace watched
// as its own sibling would self-trip on every legitimate commit). ok=false on
// any resolution failure; callers fail closed.
func resolvePath(p string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", false
	}
	return abs, true
}

// snapshotContainment captures the git state of every checkout directly under
// roots, excluding the assigned workspace itself. A directory is a checkout
// when it contains .git (a directory for a primary checkout, a FILE for a
// linked worktree — os.Stat covers both). ok=false means watched state could
// not be read (unreadable root, unresolvable path, unreadable repo) — an
// unreadable baseline verifies nothing, so callers MUST fail closed rather
// than proceed unwatched. A repo with no commits yet is readable state
// ("(unborn)"), not an error.
func snapshotContainment(roots []string, workspace string) (containmentBaseline, bool) {
	wsResolved, wsOK := resolvePath(workspace)
	if !wsOK {
		return nil, false
	}
	base := containmentBaseline{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, false
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				if os.IsNotExist(err) {
					continue // definitively not a git checkout
				}
				// Permission, IO, or broken-worktree-metadata errors mean a
				// child that MIGHT be a checkout cannot be read. Proceeding
				// would launch the subprocess with that child unwatched —
				// fail closed instead (the same contract as an unreadable
				// root: unreadable watched state verifies nothing).
				return nil, false
			}
			resolved, ok := resolvePath(dir)
			if !ok {
				return nil, false
			}
			if resolved == wsResolved {
				continue // the assigned workspace is the run's to mutate
			}
			// status failing means the checkout itself is unreadable — fail
			// closed. A failing rev-parse on a readable repo is the unborn
			// (no-commit) state: a state to fingerprint, not an error.
			status, statusOK := gitTry(dir, "status", "--porcelain")
			if !statusOK {
				return nil, false
			}
			head, headOK := gitTry(dir, "rev-parse", "HEAD")
			if !headOK {
				head = "(unborn)"
			}
			branch, branchOK := gitTry(dir, "rev-parse", "--abbrev-ref", "HEAD")
			if !branchOK {
				branch = "(unborn)"
			}
			base[resolved] = repoContainmentState{head: head, branch: branch, status: status}
		}
	}
	return base, true
}

// diffContainment compares baselines and returns one violation per
// difference. ANY difference is a violation: HEAD moved, branch switched,
// working tree changed, a checkout vanished, or a new checkout appeared. The
// comparison is state-to-state, not cleanliness: a sibling that was already
// dirty at baseline and is identically dirty after does NOT trip — operator
// working state must not false-positive the gate.
func diffContainment(pre, post containmentBaseline) []string {
	var v []string
	for path, p := range pre {
		q, ok := post[path]
		if !ok {
			v = append(v, fmt.Sprintf("%s: checkout vanished during Operate", path))
			continue
		}
		if q.head != p.head {
			v = append(v, fmt.Sprintf("%s: HEAD moved %s -> %s", path, shortHash(p.head), shortHash(q.head)))
		}
		if q.branch != p.branch {
			v = append(v, fmt.Sprintf("%s: branch switched %q -> %q", path, p.branch, q.branch))
		}
		if q.status != p.status {
			v = append(v, fmt.Sprintf("%s: working tree changed during Operate", path))
		}
	}
	for path := range post {
		if _, ok := pre[path]; !ok {
			v = append(v, fmt.Sprintf("%s: new checkout appeared during Operate", path))
		}
	}
	sort.Strings(v)
	return v
}

// verifyOperateContainment is the post-Operate tripwire verdict. It returns
// true only when every watched sibling is identical in state to the baseline.
// On any violation — or unreadable post-state — it fails the task through the
// same escalation path as the commit-verification gate and returns false.
// This veto runs BEFORE commit verification: a run that mutated a sibling
// checkout must never auto-complete, however clean its workspace commit looks
// (the v10 round-3 escape shape was exactly that).
func (l *Loop) verifyOperateContainment(ctx context.Context, task work.Task, pre containmentBaseline) bool {
	return l.verifyOperateContainmentInWorkspace(ctx, task, pre, l.config.RepoPath, l.containmentRoots())
}

func (l *Loop) verifyOperateContainmentInWorkspace(ctx context.Context, task work.Task, pre containmentBaseline, repoPath string, roots []string) bool {
	post, ok := snapshotContainment(roots, repoPath)
	if !ok {
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"post-Operate containment state unreadable for watch roots %v — refusing to accept any output of an unwatchable run (fail closed)",
			roots))
		return false
	}
	if violations := diffContainment(pre, post); len(violations) > 0 {
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"workspace containment violated (v10-F2 tripwire) — the Operate produced checkout-state deltas outside its assigned workspace %s:\n  %s\nThe run's outputs are untrusted; the operator must inspect and restore the listed checkouts.",
			repoPath, strings.Join(violations, "\n  ")))
		return false
	}
	return true
}
