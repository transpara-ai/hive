package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
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

	// Look up the Build: document for this commit to use as a cause for the
	// critique claim and any fix tasks (Invariant 2: CAUSALITY). Uses the
	// same normalization as the commit-subject writer in runner.go, so the
	// lookup key matches the title under which Build: documents are created.
	var buildCauses []string
	if r.cfg.APIClient != nil {
		subject := stripRetryPrefixes(c.subject)
		if buildNode := r.cfg.APIClient.LatestByTitle(r.cfg.SpaceSlug, "Build: "+subject); buildNode != nil {
			buildCauses = []string{buildNode.ID}
		}
	}

	// Use Operate() if available — Critic can search knowledge for invariants,
	// check primitives, and read prior critiques to ground the review.
	op, canOperate := r.cfg.Provider.(decision.IOperator)
	var content string
	if canOperate {
		apiKey := os.Getenv("LOVYOU_API_KEY")
		causesSuffix := ""
		if len(buildCauses) > 0 {
			causesSuffix = fmt.Sprintf(`,"causes":["%s"]`, buildCauses[0])
		}
		instruction := fmt.Sprintf(`You are the Critic. Review this diff and decide: PASS or REVISE.

## Diff
%s

## Your Tools
- Use knowledge.search to check relevant invariants and conventions
- Use knowledge.get to read the CLAUDE.md or coding standards
- Use Read/Grep to verify the change in context
- Read loop/scout.md to find the Scout's open gap
- Read loop/build.md to check what the Builder claims was built

## Rules
- PASS if the code is correct, tested, and follows conventions
- REVISE if there are bugs, missing tests, security issues, or invariant violations
- Check Invariant 11 (IDs not names) and Invariant 12 (VERIFIED — tests exist)

## Required Checks (Lessons 168/171/200)
1. **Scout gap cross-reference:** Read loop/scout.md. Read loop/build.md. If build.md does not explicitly reference the Scout's open gap, issue VERDICT: REVISE.
2. **Degenerate iteration:** If ALL changed files in the diff are under loop/ with no product code changes, issue VERDICT: REVISE.

## Output
End your response with exactly one of:
VERDICT: PASS
VERDICT: REVISE

If REVISE, create a fix task (causes links to the build being reviewed — Invariant 2: CAUSALITY):
curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"intend","kind":"task","title":"Fix: <subject>","description":"<what needs fixing>","priority":"high"%s}'
`, diff, apiKey, r.cfg.APIBase, r.cfg.SpaceSlug, causesSuffix)

		result, err := op.Operate(ctx, decision.OperateTask{
			WorkDir:     r.cfg.RepoPath,
			Instruction: instruction,
		})
		if err != nil {
			log.Printf("[critic] Operate error: %v", err)
			return
		}
		r.cost.Record(result.Usage)
		r.dailyBudget.Record(result.Usage.CostUSD)
		log.Printf("[critic] review done (cost=$%.4f)", result.Usage.CostUSD)
		content = result.Summary
	} else {
		// Fallback: Reason() without tools.
		sharedCtx := LoadSharedContext(r.cfg.HiveDir)
		scoutContent := loadLoopArtifact(r.cfg.HiveDir, "scout.md")
		buildContent := loadLoopArtifact(r.cfg.HiveDir, "build.md")
		prompt := buildReviewPrompt(c, diff, sharedCtx, scoutContent, buildContent)
		resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
		if err != nil {
			log.Printf("[critic] Reason error: %v", err)
			return
		}
		r.cost.Record(resp.Usage())
		r.dailyBudget.Record(resp.Usage().CostUSD)
		log.Printf("[critic] review done (cost=$%.4f)", resp.Usage().CostUSD)
		content = resp.Content()
	}

	verdict := parseVerdict(content)
	log.Printf("[critic] verdict: %s", verdict)

	// Write critique artifact, caused by the build document it reviews.
	// Returns the claim node ID for causality threading of fix tasks.
	claimID, writeErr := r.writeCritiqueArtifact(c.subject, verdict, content, buildCauses)
	if writeErr != nil {
		log.Printf("[critic] write critique artifact error: %v", writeErr)
	}

	switch verdict {
	case "REVISE":
		// Extract the issues and create a fix task caused by the critique claim.
		issues := extractIssues(content)
		title := fixTitle(c.subject)
		desc := fmt.Sprintf("Critic review of commit %s found issues:\n\n%s", c.hash[:12], issues)

		var causes []string
		if claimID != "" {
			causes = []string{claimID}
		}
		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, title, desc, "high", causes)
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

func buildReviewPrompt(c commit, diff, sharedCtx, scoutContent, buildContent string) string {
	scoutSection := ""
	if scoutContent != "" {
		scoutSection = fmt.Sprintf("\n## Scout Report (loop/scout.md)\n%s\n", scoutContent)
	}
	buildSection := ""
	if buildContent != "" {
		buildSection = fmt.Sprintf("\n## Build Report (loop/build.md)\n%s\n", buildContent)
	}

	return fmt.Sprintf(`You are the Critic. Review this commit for correctness and completeness.

## Institutional Knowledge (invariants, coding standards, lessons)
%s
%s%s
## Commit
%s: %s

## Diff
%s

## Review Checklist

1. **Scout gap cross-reference (Lessons 168/171):** Does the build report (loop/build.md) explicitly reference the open gap from the Scout report (loop/scout.md)? If build.md does not name the Scout's gap, issue VERDICT: REVISE.
2. **Degenerate iteration (Lesson 200):** Are ALL changed files in this diff under loop/? If every file is a loop artifact (scout.md, build.md, critique.md, etc.) with no product code changes, issue VERDICT: REVISE.
3. **Completeness:** If a new constant/kind is added, is it present in ALL relevant guards, allowlists, and switch statements? Search the diff for patterns like "!= KindX && != KindY" — the new kind must be there too.
4. **Identity (invariant 11):** Are IDs used for matching/JOINs, never display names?
5. **Bounded (invariant 13):** Do queries have LIMIT? Do loops have bounds?
6. **Correctness:** SQL injection? Race conditions? Nil handling?
7. **Tests:** Are there tests for the new code? (Note: test debt is a known systemic issue — flag but don't REVISE for it alone.)

## Output Format

Start with your analysis, then end with exactly one of:
VERDICT: PASS
VERDICT: REVISE

If REVISE, list the specific issues that must be fixed.`, sharedCtx, scoutSection, buildSection, c.hash[:12], c.subject, diff)
}

// isDegenerateIteration returns true when every changed file in the diff is a
// loop artifact (under loop/). A degenerate iteration produces no product code.
func isDegenerateIteration(diff string) bool {
	if diff == "" {
		return false
	}
	hasFile := false
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		hasFile = true
		// "diff --git a/path b/path" — extract the a/ path.
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return false
		}
		path := strings.TrimPrefix(parts[2], "a/")
		if !strings.HasPrefix(path, "loop/") {
			return false
		}
	}
	return hasFile
}

// loadLoopArtifact reads a file from loop/ in the hive directory.
// Returns empty string if the file does not exist or hiveDir is unset.
func loadLoopArtifact(hiveDir, name string) string {
	if hiveDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", name))
	if err != nil {
		return ""
	}
	// Cap artifact content to avoid blowing up prompts.
	if len(data) > 3000 {
		data = append(data[:3000], []byte("\n... (truncated)")...)
	}
	return string(data)
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

// fixTitle returns "Fix: {core}", where core is the subject with all leading
// [hive:*] role prefixes and "Fix: " prefixes stripped. Without full
// normalization, inputs like "[hive:builder] Fix: X" would produce
// "Fix: [hive:builder] Fix: X" — compounding across review cycles.
func fixTitle(subject string) string {
	return "Fix: " + stripRetryPrefixes(subject)
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

// writeCritiqueArtifact writes loop/critique.md with a structured review record.
func writeCritiqueArtifact(hiveDir, subject, verdict, summary string) error {
	content := fmt.Sprintf("# Critique: %s\n\n**Verdict:** %s\n\n**Summary:** %s\n", subject, verdict, summary)
	path := filepath.Join(hiveDir, "loop", "critique.md")
	return os.WriteFile(path, []byte(content), 0644)
}

// writeCritiqueArtifact writes loop/critique.md and asserts a claim on the graph.
// causeIDs should contain the build document node ID being reviewed (Invariant 2: CAUSALITY).
// Returns the claim node ID (or "" if API unavailable or claim failed) for causality threading.
func (r *Runner) writeCritiqueArtifact(subject, verdict, summary string, causeIDs []string) (string, error) {
	if err := writeCritiqueArtifact(r.cfg.HiveDir, subject, verdict, summary); err != nil {
		return "", err
	}
	if r.cfg.APIClient == nil {
		return "", nil
	}
	// Critique is a claim — a verifiable assertion about code quality.
	content := fmt.Sprintf("**Verdict:** %s\n\n%s", verdict, summary)
	title := fmt.Sprintf("Critique: %s — %s", verdict, subject)
	node, err := r.cfg.APIClient.AssertClaim(r.cfg.SpaceSlug, title, content, causeIDs)
	if err != nil || node == nil {
		return "", nil // non-fatal: file was written, graph sync failed
	}
	return node.ID, nil
}
