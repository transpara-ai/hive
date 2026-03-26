package runner

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// runCritic scans for recent builder commits and reviews them.
// Uses Reason() (no tools, fast, cheap) to analyze diffs.
func (r *Runner) runCritic(ctx context.Context) {
	// Only run every 4th tick (60s at default interval). Always run in one-shot mode.
	if !r.cfg.OneShot && r.tick%4 != 0 {
		return
	}

	// Find recent builder commits not yet reviewed.
	commits, err := r.findUnreviewedCommits()
	if err != nil {
		log.Printf("[critic] tick %d: error finding commits: %v", r.tick, err)
		return
	}

	if len(commits) == 0 {
		return
	}

	log.Printf("[critic] tick %d: found %d unreviewed commits", r.tick, len(commits))

	for _, c := range commits {
		r.reviewCommit(ctx, c)

		// In one-shot mode, stop after first review.
		if r.cfg.OneShot {
			r.done = true
			return
		}
	}
}

type commit struct {
	hash    string
	subject string
}

// findUnreviewedCommits returns recent [hive:builder] commits.
// Uses git log to find commits from the last 24 hours with the builder prefix.
func (r *Runner) findUnreviewedCommits() ([]commit, error) {
	cmd := exec.Command("git", "log", "--oneline", "--since=24 hours ago",
		"--grep=\\[hive:builder\\]", "--format=%H %s")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var commits []commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		commits = append(commits, commit{hash: parts[0], subject: parts[1]})
	}
	return commits, nil
}

// reviewCommit reviews a single builder commit.
func (r *Runner) reviewCommit(ctx context.Context, c commit) {
	log.Printf("[critic] reviewing %s: %s", c.hash[:12], c.subject)

	// Get the diff.
	diff, err := r.getCommitDiff(c.hash)
	if err != nil {
		log.Printf("[critic] diff error: %v", err)
		return
	}

	if len(diff) == 0 {
		log.Printf("[critic] empty diff, skipping")
		return
	}

	// Truncate very large diffs to avoid blowing up the context.
	if len(diff) > 15000 {
		diff = diff[:15000] + "\n... (truncated)"
	}

	// Build the review prompt.
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	prompt := buildReviewPrompt(c, diff, sharedCtx)

	// Call Reason() — no tools, just thinking.
	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[critic] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[critic] review done (cost=$%.4f)", resp.Usage().CostUSD)

	// Parse the verdict.
	content := resp.Content()
	verdict := parseVerdict(content)
	log.Printf("[critic] verdict: %s", verdict)

	switch verdict {
	case "REVISE":
		// Extract the issues and create a fix task.
		issues := extractIssues(content)
		title := fixTitle(c.subject)
		desc := fmt.Sprintf("Critic review of commit %s found issues:\n\n%s", c.hash[:12], issues)

		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, title, desc, "high")
		if err != nil {
			log.Printf("[critic] create fix task error: %v", err)
			return
		}
		log.Printf("[critic] created fix task: %s", task.ID)

		// Assign to our agent so the Builder picks it up.
		if r.cfg.AgentID != "" {
			if err := r.cfg.APIClient.ClaimTask(r.cfg.SpaceSlug, task.ID); err != nil {
				log.Printf("[critic] assign fix task error (non-fatal): %v", err)
			}
		}

	case "PASS":
		log.Printf("[critic] PASS: %s", c.hash[:12])
		if r.cfg.PRMode {
			r.maybeCreatePR(c)
		}
	}
}

func (r *Runner) getCommitDiff(hash string) (string, error) {
	cmd := exec.Command("git", "diff", hash+"~1.."+hash)
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}

func buildReviewPrompt(c commit, diff, sharedCtx string) string {
	return fmt.Sprintf(`You are the Critic. Review this commit for correctness and completeness.

## Institutional Knowledge (invariants, coding standards, lessons)
%s

## Commit
%s: %s

## Diff
%s

## Review Checklist

1. **Completeness:** If a new constant/kind is added, is it present in ALL relevant guards, allowlists, and switch statements? Search the diff for patterns like "!= KindX && != KindY" — the new kind must be there too.
2. **Identity (invariant 11):** Are IDs used for matching/JOINs, never display names?
3. **Bounded (invariant 13):** Do queries have LIMIT? Do loops have bounds?
4. **Correctness:** SQL injection? Race conditions? Nil handling?
5. **Tests:** Are there tests for the new code? (Note: test debt is a known systemic issue — flag but don't REVISE for it alone.)

## Output Format

Start with your analysis, then end with exactly one of:
VERDICT: PASS
VERDICT: REVISE

If REVISE, list the specific issues that must be fixed.`, sharedCtx, c.hash[:12], c.subject, diff)
}

func parseVerdict(content string) string {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "VERDICT:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "VERDICT:"))
			switch v {
			case "PASS", "REVISE":
				return v
			}
		}
	}
	return "PASS" // default to pass if no verdict found
}

// fixTitle returns "Fix: {subject}" but avoids double-prefixing when the
// subject already starts with "Fix: ".
func fixTitle(subject string) string {
	if strings.HasPrefix(subject, "Fix: ") {
		return subject
	}
	return "Fix: " + subject
}

func extractIssues(content string) string {
	// Return everything after the last "VERDICT: REVISE" or "Issues:" header.
	idx := strings.LastIndex(content, "VERDICT: REVISE")
	if idx > 0 {
		// Return everything before the verdict as the issues.
		return strings.TrimSpace(content[:idx])
	}
	// Fallback: return last 1000 chars.
	if len(content) > 1000 {
		return content[len(content)-1000:]
	}
	return content
}
