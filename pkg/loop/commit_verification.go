package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/work"
)

// commitVerdict classifies the outcome of an implementer Operate that may or
// may not have produced a git commit.
type commitVerdict int

const (
	// commitVerified: the repo HEAD advanced — a real commit exists.
	commitVerified commitVerdict = iota
	// commitWaivable: no commit and no commit claim — a legitimate no-op.
	commitWaivable
	// commitConfabulated: the agent claimed a commit but HEAD did not advance.
	// The commit was confabulated or landed in another repo — never trust it.
	commitConfabulated
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
func classifyOperateCommit(preHead, postHead, summary string) commitVerdict {
	// HEAD advanced — a real commit exists, regardless of what the summary says.
	if postHead != "" && postHead != preHead {
		return commitVerified
	}
	// HEAD did not advance (or is unverifiable). If the agent nonetheless claims
	// a commit, the claim is false (or the commit landed in another repo).
	if claimsCommit(summary) {
		return commitConfabulated
	}
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
	postOperateHead := gitCommand(l.config.RepoPath, "rev-parse", "HEAD")
	switch classifyOperateCommit(preOperateHead, postOperateHead, summary) {
	case commitVerified:
		l.attachOperateArtifact(task)
		l.completeTask(ctx, task, summary)
		return true
	case commitConfabulated:
		l.failOperateTask(ctx, task, fmt.Sprintf(
			"agent reported a commit but %s HEAD did not advance (still %s) — refusing to complete on an unverified commit",
			l.config.RepoPath, shortHash(preOperateHead)))
		return false
	default: // commitWaivable
		l.waiveOperateArtifact(task, "Operate produced no new commits")
		l.completeTask(ctx, task, summary)
		return true
	}
}

// failOperateTask records that an Operate failed commit verification. It does
// NOT mark the task complete, and escalates to the human authority tier so the
// always-on run surfaces the failure instead of silently proceeding on a false
// commit. The task is left un-completed so the implementer can retry it.
func (l *Loop) failOperateTask(ctx context.Context, task work.Task, reason string) {
	fmt.Printf("[%s] ✗ commit verification FAILED: %s — %s\n", l.agent.Name(), task.ID.Value(), reason)
	if err := l.agent.Escalate(ctx, l.humanID,
		fmt.Sprintf("commit verification failed for task %s (%s): %s", task.ID.Value(), task.Title, reason)); err != nil {
		fmt.Printf("[%s] warning: escalation after commit-verification failure failed: %v\n", l.agent.Name(), err)
	}
	if l.sink != nil {
		l.captureBoundary(checkpoint.TaskBlocked, reason)
		l.lastCheckpointIter = l.iteration
	}
}
