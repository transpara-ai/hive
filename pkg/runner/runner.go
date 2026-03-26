// Package runner implements the hive agent tick loop.
// One Runner per agent role. Polls lovyou.ai for tasks, dispatches to
// role-specific handlers, commits results, and tracks costs.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/hive/pkg/api"
)

// modelPricing maps short model names to per-million-token costs.
var modelPricing = map[string]struct{ Input, Output float64 }{
	"haiku":  {0.80, 4.00},
	"sonnet": {3.00, 15.00},
	"opus":   {15.00, 75.00},
}

// roleModel maps agent roles to default model names.
var roleModel = map[string]string{
	"scout":     "haiku",
	"architect": "sonnet",
	"builder":   "sonnet",
	"tester":    "haiku",
	"critic":    "sonnet",
	"reflector": "haiku",
	"ops":       "haiku",
	"observer":  "sonnet",
	"guardian":  "haiku",
	"monitor":   "haiku",
	"pm":        "sonnet",
}

// Config holds everything a Runner needs.
type Config struct {
	Role       string // e.g. "builder"
	AgentID    string // lovyou.ai user ID for this agent (filters task assignment)
	SpaceSlug  string // lovyou.ai space slug (e.g. "hive")
	RepoPath   string // absolute path to the repo to operate on
	HiveDir    string // path to hive repo (for state.md, role prompts)
	APIClient  *api.Client
	Provider   intelligence.Provider
	RolePrompt string // loaded from agents/{role}.md
	Interval   time.Duration
	BudgetUSD     float64 // daily budget, 0 = $10 default
	OneShot       bool    // if true, work one task then exit (for testing)
	NoPush        bool              // if true, commit but don't push (pipeline pushes after Critic PASS)
	PRMode        bool              // if true, create a feature branch before committing
	CouncilTopic  string            // optional: focus the council on a specific question
	RepoMap       map[string]string // named repos: name → absolute path (for multi-repo pipeline)
}

// CostTracker records per-call spending.
type CostTracker struct {
	TotalCostUSD float64
	BudgetUSD    float64
	CallCount    int
	InputTokens  int
	OutputTokens int
}

func (ct *CostTracker) Record(usage decision.TokenUsage) {
	ct.InputTokens += usage.InputTokens
	ct.OutputTokens += usage.OutputTokens
	ct.TotalCostUSD += usage.CostUSD
	ct.CallCount++
}

func (ct *CostTracker) IsOverBudget() bool {
	return ct.BudgetUSD > 0 && ct.TotalCostUSD >= ct.BudgetUSD
}

// Runner is a long-running agent process.
type Runner struct {
	cfg         Config
	cost        CostTracker
	dailyBudget *DailyBudget
	tick        int
	done        bool // set by one-shot mode after task completes
}

// New creates a Runner.
func New(cfg Config) *Runner {
	if cfg.Interval == 0 {
		cfg.Interval = 15 * time.Second
	}
	budget := cfg.BudgetUSD
	if budget == 0 {
		budget = 10.0
	}
	return &Runner{
		cfg:         cfg,
		cost:        CostTracker{BudgetUSD: budget},
		dailyBudget: NewDailyBudget(cfg.HiveDir),
	}
}

// Run starts the tick loop. Blocks until context is cancelled or budget exceeded.
func (r *Runner) Run(ctx context.Context) error {
	log.Printf("[%s] runner started (repo=%s, space=%s, interval=%s, budget=$%.2f)",
		r.cfg.Role, r.cfg.RepoPath, r.cfg.SpaceSlug, r.cfg.Interval, r.cost.BudgetUSD)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[%s] shutting down: %v", r.cfg.Role, ctx.Err())
			r.printCostSummary()
			return nil
		default:
		}

		r.tick++

		if r.done {
			log.Printf("[%s] one-shot complete", r.cfg.Role)
			r.printCostSummary()
			return nil
		}

		if r.cost.IsOverBudget() {
			log.Printf("[%s] budget exceeded ($%.2f / $%.2f), stopping",
				r.cfg.Role, r.cost.TotalCostUSD, r.cost.BudgetUSD)
			r.printCostSummary()
			return nil
		}

		if remaining := r.dailyBudget.Remaining(r.cost.BudgetUSD); remaining <= 0 {
			log.Printf("[%s] daily budget ceiling reached ($%.2f spent today), sleeping until next cycle",
				r.cfg.Role, r.dailyBudget.Spent())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(r.cfg.Interval):
			}
			continue
		}

		r.runTick(ctx)

		select {
		case <-ctx.Done():
			r.printCostSummary()
			return nil
		case <-time.After(r.cfg.Interval):
		}
	}
}

func (r *Runner) runTick(ctx context.Context) {
	switch r.cfg.Role {
	case "builder":
		r.runBuilder(ctx)
	case "scout":
		r.runScout(ctx)
	case "critic":
		r.runCritic(ctx)
	case "pm":
		r.runPM(ctx)
	case "architect":
		r.runArchitect(ctx)
	case "observer":
		r.runObserver(ctx)
	case "monitor":
		r.runMonitor(ctx)
	case "reflector":
		r.runReflector(ctx)
	default:
		log.Printf("[%s] tick %d: no handler for role", r.cfg.Role, r.tick)
	}
}

// ─── Builder ─────────────────────────────────────────────────────────

func (r *Runner) runBuilder(ctx context.Context) {
	// 1. Get all open tasks from the board.
	tasks, err := r.cfg.APIClient.GetTasks(r.cfg.SpaceSlug, "")
	if err != nil {
		log.Printf("[builder] tick %d: GetTasks error: %v", r.tick, err)
		return
	}

	// Filter to open/active tasks. Fix tasks (REVISE) get priority.
	var fixTasks []api.Node
	var myTasks []api.Node
	var unassigned []api.Node
	for _, t := range tasks {
		if t.Kind != "task" {
			continue
		}
		if t.State == "done" || t.State == "closed" {
			continue
		}
		// When agent-id is set, only work tasks assigned to this agent.
		// When agent-id is empty, work ALL open tasks (assigned or not).
		if t.AssigneeID != "" && r.cfg.AgentID != "" && t.AssigneeID != r.cfg.AgentID {
			continue // assigned to someone else
		}
		if strings.HasPrefix(t.Title, "Fix:") {
			fixTasks = append(fixTasks, t)
		} else if t.AssigneeID == "" {
			unassigned = append(unassigned, t)
		} else {
			myTasks = append(myTasks, t)
		}
	}

	// REVISE GATE: fix tasks before new work (lesson 47, council directive).
	if len(fixTasks) > 0 {
		log.Printf("[builder] tick %d: %d fix tasks — working fixes before new work", r.tick, len(fixTasks))
		myTasks = fixTasks
	}

	// 2. If none assigned to us, claim the highest priority unassigned task.
	if len(myTasks) == 0 && len(unassigned) > 0 {
		t := pickHighestPriority(unassigned)
		log.Printf("[builder] tick %d: claiming task %s: %s", r.tick, t.ID, t.Title)
		if err := r.cfg.APIClient.ClaimTask(r.cfg.SpaceSlug, t.ID); err != nil {
			log.Printf("[builder] claim error: %v", err)
			return
		}
		myTasks = append(myTasks, t)
	}

	if len(myTasks) == 0 {
		if r.tick%4 == 0 {
			log.Printf("[builder] tick %d: no tasks", r.tick)
		}
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// 3. Work the highest priority task.
	t := pickHighestPriority(myTasks)
	r.workTask(ctx, t)

	// In one-shot mode, signal completion.
	if r.cfg.OneShot {
		r.done = true
	}
}

func (r *Runner) workTask(ctx context.Context, t api.Node) {
	log.Printf("[builder] working task %s: %s", t.ID, t.Title)

	// Mark in-progress.
	_ = r.cfg.APIClient.UpdateTaskStatus(r.cfg.SpaceSlug, t.ID, "active")

	// Build the prompt.
	prompt := r.buildPrompt(t)

	// Execute with Claude CLI (full tool access).
	op, ok := r.cfg.Provider.(decision.IOperator)
	if !ok {
		log.Printf("[builder] provider does not support Operate")
		return
	}

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.RepoPath,
		Instruction: prompt,
	})
	if err != nil {
		log.Printf("[builder] Operate error: %v", err)
		_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
			fmt.Sprintf("Operate failed: %v", err))
		return
	}

	// Record cost (in-process and file-backed daily tracker).
	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[builder] Operate done (cost=$%.4f, tokens=%d+%d)",
		result.Usage.CostUSD, result.Usage.InputTokens, result.Usage.OutputTokens)

	// Parse action from response.
	action := parseAction(result.Summary)
	log.Printf("[builder] action: %s", action)

	switch action {
	case "DONE":
		// Verify build passes.
		if err := r.verifyBuild(); err != nil {
			log.Printf("[builder] build failed: %v", err)
			_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
				fmt.Sprintf("Build failed after implementation, fixing...\n```\n%s\n```", err))
			return // stay in-progress
		}

		// Check for file changes.
		hasChanges := r.hasUncommittedChanges()
		if !hasChanges {
			// Builder said DONE with no changes — work was already applied in a prior commit.
			// Complete the task to avoid spin loops.
			log.Printf("[builder] DONE but no file changes — completing (work already applied)")
			_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
				"Operate returned DONE but no files were changed. Work was likely applied in a prior commit.")
			_ = r.cfg.APIClient.CompleteTask(r.cfg.SpaceSlug, t.ID)
			return
		}

		// Commit and push.
		if err := r.commitAndPush(t); err != nil {
			log.Printf("[builder] commit/push error: %v", err)
			_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
				fmt.Sprintf("Commit/push failed: %v", err))
			return
		}

		// Close task.
		if err := r.cfg.APIClient.CompleteTask(r.cfg.SpaceSlug, t.ID); err != nil {
			log.Printf("[builder] complete error: %v", err)
			return
		}
		log.Printf("[builder] task %s DONE: %s", t.ID, t.Title)

		// Post cost summary as comment.
		_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
			fmt.Sprintf("Completed. Cost: $%.4f (%d calls total)", r.cost.TotalCostUSD, r.cost.CallCount))

	case "PROGRESS":
		summary := extractSummary(result.Summary)
		_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
			fmt.Sprintf("Progress: %s", summary))

	case "ESCALATE":
		summary := extractSummary(result.Summary)
		_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
			fmt.Sprintf("ESCALATE: %s", summary))
	}
}

func (r *Runner) buildPrompt(t api.Node) string {
	var b strings.Builder

	// Shared institutional knowledge — every agent gets this.
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	if sharedCtx != "" {
		b.WriteString(sharedCtx)
		b.WriteString("\n\n---\n\n")
	}

	// Role context.
	if r.cfg.RolePrompt != "" {
		b.WriteString(r.cfg.RolePrompt)
		b.WriteString("\n\n---\n\n")
	}

	// Task.
	b.WriteString(fmt.Sprintf("# Task: %s\n\n", t.Title))
	if t.Body != "" {
		b.WriteString(t.Body)
		b.WriteString("\n\n")
	}
	if t.Priority != "" {
		b.WriteString(fmt.Sprintf("Priority: %s\n", t.Priority))
	}

	// Instructions.
	b.WriteString(`
## Instructions

1. Implement the task described above.
2. Run ` + "`go.exe build -buildvcs=false ./...`" + ` to verify compilation.
3. Run ` + "`go.exe test ./...`" + ` to verify tests pass.
4. If you edit any .templ files, run ` + "`/c/Users/matt_/go/bin/templ generate`" + ` first.

When done, end your response with exactly:
ACTION: DONE

If you need more work or are partially complete:
ACTION: PROGRESS

If you're stuck and need human help:
ACTION: ESCALATE
`)
	return b.String()
}

// ─── Build verification ──────────────────────────────────────────────

func (r *Runner) verifyBuild() error {
	cmd := exec.Command("go.exe", "build", "-buildvcs=false", "./...")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s\n%s", err, string(out))
	}
	return nil
}

// ─── Git operations ──────────────────────────────────────────────────

func (r *Runner) hasUncommittedChanges() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.cfg.RepoPath
	out, _ := cmd.Output()
	return len(bytes.TrimSpace(out)) > 0
}

func (r *Runner) commitAndPush(t api.Node) error {
	branch := buildBranchName(r.cfg, t.Title)

	// When PRMode is active, create (or reset) the feature branch before committing.
	// Use -B so retries after a failed push don't hit "branch already exists".
	if branch != "" {
		if err := r.git("checkout", "-B", branch); err != nil {
			return fmt.Errorf("git checkout -B %s: %w", branch, err)
		}
		log.Printf("[builder] on feature branch: %s", branch)
	}

	// Stage all changes.
	if err := r.git("add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Commit.
	msg := fmt.Sprintf("[hive:%s] %s", r.cfg.Role, t.Title)
	if err := r.git("commit", "-m", msg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Push (unless NoPush — pipeline pushes after Critic PASS).
	if r.cfg.NoPush {
		log.Printf("[builder] committed (no push — waiting for Critic): %s", msg)
		return nil
	}

	// When on a feature branch, push with upstream tracking set.
	if branch != "" {
		if err := r.git("push", "--set-upstream", "origin", branch); err != nil {
			return fmt.Errorf("git push feature branch: %w", err)
		}
		log.Printf("[builder] pushed feature branch: %s", branch)
		return nil
	}

	if err := r.git("push"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	log.Printf("[builder] committed and pushed: %s", msg)
	return nil
}

// Push pushes the current branch. Called by the pipeline after Critic PASS.
func (r *Runner) Push() error {
	return r.git("push")
}

func (r *Runner) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

// ─── Monitor ─────────────────────────────────────────────────────────

func (r *Runner) runMonitor(ctx context.Context) {
	if r.tick%4 == 0 {
		log.Printf("[monitor] tick %d: monitoring (not yet implemented)", r.tick)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────

var priorityOrder = map[string]int{
	"urgent": 0,
	"high":   1,
	"medium": 2,
	"low":    3,
	"":       4,
}

func pickHighestPriority(nodes []api.Node) api.Node {
	best := nodes[0]
	for _, n := range nodes[1:] {
		np := priorityOrder[n.Priority]
		bp := priorityOrder[best.Priority]
		if np < bp {
			best = n
		} else if np == bp && n.CreatedAt > best.CreatedAt {
			// Same priority: prefer newest (most likely to be a fresh assignment).
			best = n
		}
	}
	return best
}

func parseAction(summary string) string {
	lines := strings.Split(summary, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "ACTION:") {
			action := strings.TrimSpace(strings.TrimPrefix(line, "ACTION:"))
			switch action {
			case "DONE", "PROGRESS", "ESCALATE":
				return action
			}
		}
	}
	// Default to DONE if the response doesn't explicitly say.
	return "DONE"
}

func extractSummary(s string) string {
	// Return last 500 chars as summary.
	if len(s) > 500 {
		return s[len(s)-500:]
	}
	return s
}

func (r *Runner) printCostSummary() {
	log.Printf("[%s] cost summary: $%.4f / $%.2f (calls=%d, in=%d, out=%d)",
		r.cfg.Role, r.cost.TotalCostUSD, r.cost.BudgetUSD,
		r.cost.CallCount, r.cost.InputTokens, r.cost.OutputTokens)
}

// branchSlug converts a task title to a git branch name.
// Format: feat/YYYYMMDD-{slug}, where the slug portion is lowercase
// alphanumeric with hyphens, truncated at 40 characters.
func branchSlug(title string, date time.Time) string {
	dateStr := date.Format("20060102")

	var b strings.Builder
	prevHyphen := true // start true to suppress leading hyphens
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return fmt.Sprintf("feat/%s-%s", dateStr, s)
}

// buildBranchName returns the branch to create before committing, or "" when
// PRMode is disabled (caller should skip the git checkout -b step).
func buildBranchName(cfg Config, title string) string {
	if !cfg.PRMode {
		return ""
	}
	return branchSlug(title, time.Now())
}

// ModelForRole returns the default model short name for a role.
// Override with AGENT_MODEL env var.
func ModelForRole(role string) string {
	if override := os.Getenv("AGENT_MODEL"); override != "" {
		return override
	}
	if m, ok := roleModel[role]; ok {
		return m
	}
	return "haiku"
}

// maybeCreatePR creates a GitHub PR for a Critic-PASS commit when PRMode is enabled.
// Logs and skips gracefully if gh is not found.
func (r *Runner) maybeCreatePR(c commit) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		log.Printf("[critic] gh not found, skipping PR creation")
		return
	}

	branch, err := r.featBranchForCommit(c.hash)
	if err != nil {
		log.Printf("[critic] branch lookup failed for %s (skipping PR): %v", c.hash[:12], err)
		return
	}
	if branch == "" {
		log.Printf("[critic] no feat/ branch found for %s, skipping PR", c.hash[:12])
		return
	}

	title := prTitleFromSubject(c.subject)
	body := fmt.Sprintf("%s\n\nCommit: %s", c.subject, c.hash[:12])

	cmd := exec.Command(ghPath, "pr", "create",
		"--title", title,
		"--body", body,
		"--head", branch,
	)
	cmd.Dir = r.cfg.RepoPath
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		log.Printf("[critic] gh pr create failed (non-fatal): %v\n%s", runErr, string(out))
		return
	}
	log.Printf("[critic] PR created for branch %s: %s", branch, strings.TrimSpace(string(out)))
}

// featBranchForCommit returns the feat/ branch that contains the given commit, or "".
func (r *Runner) featBranchForCommit(hash string) (string, error) {
	cmd := exec.Command("git", "branch", "-r", "--contains", hash, "--format=%(refname:short)")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git branch -r --contains %s: %w", hash, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(strings.TrimPrefix(line, "origin/"))
		if strings.HasPrefix(name, "feat/") {
			return name, nil
		}
	}
	return "", nil
}

// prTitleFromSubject extracts the PR title from a commit subject by stripping
// the "[hive:builder]" prefix that the Builder adds.
func prTitleFromSubject(subject string) string {
	return strings.TrimSpace(strings.TrimPrefix(subject, "[hive:builder]"))
}

// LoadRolePrompt reads the role prompt from agents/{role}.md.
func LoadRolePrompt(hiveDir, role string) string {
	path := filepath.Join(hiveDir, "agents", role+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("warning: no role prompt at %s: %v", path, err)
		return ""
	}
	return string(data)
}
