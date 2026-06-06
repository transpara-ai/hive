package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/work"
)

// commitVerdict classifies the outcome of an implementer Operate that may or
// may not have produced a git commit.
type commitVerdict int

const (
	// commitVerified: the repo HEAD advanced — a real commit exists.
	commitVerified commitVerdict = iota
	// commitWaivable: no commit, clean tree, no commit claim — a legitimate no-op.
	commitWaivable
	// commitConfabulated: the agent claimed a commit but HEAD did not advance.
	// The commit was confabulated or landed in another repo — never trust it.
	commitConfabulated
	// commitDirty: the working tree has uncommitted changes — the agent produced
	// filesystem side effects it never committed. True whether or not HEAD also
	// advanced: a commit that leaves dirt is a partial, unreviewable commit.
	commitDirty
	// commitUnverifiable: the repo state could not be read (git unavailable, not
	// a checkout, or a failed inspection). An autonomy guard must fail closed on
	// unverifiable state rather than treat it as an honest no-op.
	commitUnverifiable
)

// claimsCommit reports whether an Operate summary affirmatively asserts that a
// git commit was made. It deliberately ignores honest disclaimers ("nothing to
// commit") so the gate does not fail legitimate no-op Operates.
func claimsCommit(summary string) bool {
	s := strings.ToLower(summary)
	// Honest disclaimers that no commit was made take precedence so the gate
	// never fails a legitimate no-op Operate.
	for _, neg := range []string{
		"nothing to commit",
		"no changes to commit",
		"no files to commit",
		"did not commit",
		"didn't commit",
		"not committed",
		"no commit",
		"without committing",
		"no new commit",
	} {
		if strings.Contains(s, neg) {
			return false
		}
	}
	// Affirmative assertions that a commit was made.
	for _, pos := range []string{
		"committed",
		"committing",
		"git commit",
		"created a commit",
		"made a commit",
		"commit hash",
	} {
		if strings.Contains(s, pos) {
			return true
		}
	}
	return false
}

// classifyOperateCommit cross-checks repo HEAD movement against the agent's
// self-reported summary. Never trust the self-report: a commit claim that the
// repo HEAD does not corroborate is a confabulation (or a wrong-repo commit).
func classifyOperateCommit(preHead, postHead, summary string, dirty bool) commitVerdict {
	// Unverifiable repo HEAD: a valid HEAD is never empty, so "" means the state
	// could not be read. Fail closed — never treat unverifiable state as a no-op.
	if postHead == "" {
		return commitUnverifiable
	}
	headMoved := postHead != preHead
	// HEAD did not advance but the agent claims a commit: the claim is false (or
	// the commit landed in another repo). The most specific failure — diagnose it
	// before the broader dirty check.
	if !headMoved && claimsCommit(summary) {
		return commitConfabulated
	}
	// Uncommitted changes remain in the working tree — uncaptured, unreviewable
	// work — whether or not a commit also landed. A commit that leaves the tree
	// dirty is a partial commit, not a clean verification.
	if dirty {
		return commitDirty
	}
	// Clean tree and HEAD advanced: a real, fully-captured commit.
	if headMoved {
		return commitVerified
	}
	// Clean tree, HEAD unmoved, no commit claim: a legitimate no-op.
	return commitWaivable
}

// shortHash truncates a git hash for log/escalation messages.
func shortHash(h string) string {
	if h == "" {
		return "(none)"
	}
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// handleOperateResult applies the commit-verification gate after an implementer
// Operate. It returns true if the task was completed (verified commit or honest
// no-op) and false if the task was failed (confabulated/unverifiable commit).
func (l *Loop) handleOperateResult(ctx context.Context, task work.Task, preOperateHead, summary string) bool {
	// Inspect repo state with explicit success signals: gitTry distinguishes a
	// genuine empty result (clean tree) from a git failure. If either inspection
	// fails, the state is unverifiable — fail closed rather than guess.
	postOperateHead, headOK := gitTry(l.config.RepoPath, "rev-parse", "HEAD")
	status, statusOK := gitTry(l.config.RepoPath, "status", "--porcelain")
	if !headOK || !statusOK {
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"could not inspect repo state in %s (git unavailable or not a checkout) — refusing to complete on unverifiable state",
			l.config.RepoPath))
		return false
	}
	dirty := status != ""
	switch classifyOperateCommit(preOperateHead, postOperateHead, summary, dirty) {
	case commitVerified:
		l.attachOperateArtifact(task)
		l.completeTask(ctx, task, summary)
		return true
	case commitConfabulated:
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"agent reported a commit but %s HEAD did not advance (still %s) — refusing to complete on an unverified commit",
			l.config.RepoPath, shortHash(preOperateHead)))
		return false
	case commitDirty:
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"Operate left uncommitted changes in %s — refusing to complete unreviewed, uncommitted filesystem work",
			l.config.RepoPath))
		return false
	case commitUnverifiable:
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"could not read %s HEAD after Operate — refusing to complete on unverifiable state",
			l.config.RepoPath))
		return false
	default: // commitWaivable
		l.waiveOperateArtifact(task, "Operate produced no new commits")
		l.completeTask(ctx, task, summary)
		return true
	}
}

// nextTowardBlocked returns the next legal lifecycle hop from cur toward
// StatusBlocked, or ok=false if Blocked is unreachable. Loop tasks start at
// StatusCreated (the loop never enters the v3.9 lifecycle), so the path is
// Created→Ready→Running→Blocked; a factory task already Running blocks directly.
func nextTowardBlocked(cur work.TaskStatus) (work.TaskStatus, bool) {
	switch cur {
	case work.StatusCreated:
		return work.StatusReady, true
	case work.StatusReady:
		return work.StatusRunning, true
	case work.StatusRunning:
		return work.StatusBlocked, true
	default:
		return "", false
	}
}

// blockTaskForFailure advances task to StatusBlocked via the minimal legal
// lifecycle path from its current state, so status consumers see a blocked
// (retryable: Blocked→Ready→Running) task rather than an assigned/in-progress
// one. Bounded by the longest legal path (Created→Ready→Running→Blocked).
func (l *Loop) blockTaskForFailure(task work.Task, reason string) error {
	ts := l.config.TaskStore
	if ts == nil {
		return nil
	}
	var causes []types.EventID
	if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
		causes = []types.EventID{lastEv}
	}
	// hopBudget bounds the walk (invariant BOUNDED). The longest legal path is
	// three hops; four is a safe backstop against an unexpected cycle.
	const hopBudget = 4
	for hop := 0; hop < hopBudget; hop++ {
		cur, err := ts.GetStatus(task.ID)
		if err != nil {
			return err
		}
		if cur == work.StatusBlocked {
			return nil
		}
		next, ok := nextTowardBlocked(cur)
		if !ok {
			return fmt.Errorf("cannot reach blocked from status %q", cur)
		}
		if err := ts.TransitionTask(l.agent.ID(), task.ID, next, reason, nil, causes, l.config.ConvID); err != nil {
			return err
		}
	}
	return fmt.Errorf("task %s did not reach blocked within %d hops", task.ID.Value(), hopBudget)
}

// failOperateTask records that an Operate failed commit verification. It does
// NOT mark the task complete, and escalates to the human authority tier so the
// always-on run surfaces the failure instead of silently proceeding on a false
// commit. The task is left un-completed so the implementer can retry it.
func (l *Loop) failOperateTask(ctx context.Context, task work.Task, reason string) {
	fmt.Printf("[%s] ✗ commit verification FAILED: %s — %s\n", l.agent.Name(), task.ID.Value(), reason)
	// Reflect the failure in Work task state so status consumers see a blocked
	// (retryable) task rather than an assigned, non-completed one. Best-effort:
	// a transition error must not crash the loop, so it is logged, not returned.
	if err := l.blockTaskForFailure(task, reason); err != nil {
		fmt.Printf("[%s] warning: could not block task %s after commit-verification failure: %v\n", l.agent.Name(), task.ID.Value(), err)
	}
	if err := l.agent.Escalate(ctx, l.humanID,
		fmt.Sprintf("commit verification failed for task %s (%s): %s", task.ID.Value(), task.Title, reason)); err != nil {
		fmt.Printf("[%s] warning: escalation after commit-verification failure failed: %v\n", l.agent.Name(), err)
	}
	if l.sink != nil {
		l.captureBoundary(checkpoint.TaskBlocked, reason)
		l.lastCheckpointIter = l.iteration
	}
}
