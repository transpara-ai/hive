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

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/hive/pkg/api"
	"github.com/transpara-ai/hive/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/registry"
)


// Config holds everything a Runner needs.
type Config struct {
	Role       string // e.g. "builder"
	AgentID    string // lovyou.ai user ID for this agent (filters task assignment)
	SpaceSlug  string // lovyou.ai space slug (e.g. "hive")
	RepoPath   string // absolute path to the repo to operate on
	HiveDir    string // path to hive repo (for state.md, role prompts)
	APIClient  *api.Client
	APIBase    string // Base URL for agent curl commands (e.g. "http://localhost:8082")
	Provider   intelligence.Provider
	RolePrompt string // loaded from agents/{role}.md
	Interval   time.Duration
	BudgetUSD     float64 // daily budget, 0 = $10 default
	OneShot       bool    // if true, work one task then exit (for testing)
	NoPush        bool              // if true, commit but don't push (pipeline pushes after Critic PASS)
	PRMode        bool              // if true, create a feature branch before committing
	CouncilTopic    string            // optional: focus the council on a specific question
	RepoMap         map[string]string // named repos: name → absolute path (for multi-repo pipeline)
	BaselineCommit  string            // commit hash before Builder ran — Critic only reviews after this
	Registry        *registry.Registry // repo metadata for prompts, build/test commands
	UseWorktrees    bool              // if true, each Builder task gets its own git worktree
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
	done        bool             // set by one-shot mode after task completes
	worktree    *WorktreeContext // set by Builder when UseWorktrees is true
}

// Worktree returns the active worktree context, or nil if not using worktrees.
func (r *Runner) Worktree() *WorktreeContext { return r.worktree }

// Cost returns a snapshot of the runner's cost tracker.
func (r *Runner) Cost() CostTracker { return r.cost }

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
			log.Printf("[%s] daily budget ceiling reached ($%.2f spent today)",
				r.cfg.Role, r.dailyBudget.Spent())
			if r.cfg.OneShot {
				r.printCostSummary()
				return nil
			}
			// Daemon mode: sleep and retry next cycle.
			log.Printf("[%s] sleeping until next cycle", r.cfg.Role)
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
	case "pipeline":
		_ = NewPipelineTree(r).Execute(ctx)
	case "observer":
		r.runObserver(ctx)
	case "monitor":
		r.runMonitor(ctx)
	case "reflector":
		r.runReflector(ctx)
	case "spawner":
		r.runSpawner(ctx)
	case "scribe":
		r.runScribe(ctx)
	default:
		// Any agent in agents/ can be invoked by name.
		prompt := LoadRolePrompt(r.cfg.HiveDir, r.cfg.Role)
		if prompt != "" {
			log.Printf("[%s] tick %d: invoking as dynamic agent", r.cfg.Role, r.tick)
			_, _ = r.InvokeAgent(ctx, r.cfg.Role, "Perform your role. Check the board, search knowledge, and act.")
		} else {
			log.Printf("[%s] tick %d: no handler for role", r.cfg.Role, r.tick)
		}
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
		// Skip pinned goals — these are strategic direction, not work items.
		if t.Pinned {
			continue
		}
		// Tasks with incomplete children are blocked — set state, skip.
		if t.ChildCount > 0 && t.ChildDone < t.ChildCount {
			if t.State != "blocked" {
				_ = r.cfg.APIClient.UpdateTaskStatus(r.cfg.SpaceSlug, t.ID, "blocked")
			}
			continue
		}
		// Unblock tasks whose children are now all done.
		if t.State == "blocked" && (t.ChildCount == 0 || t.ChildDone >= t.ChildCount) {
			_ = r.cfg.APIClient.UpdateTaskStatus(r.cfg.SpaceSlug, t.ID, "active")
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

	// Worktree isolation: each task gets its own branch in a temp worktree.
	workDir := r.cfg.RepoPath
	var wt *WorktreeContext
	if r.cfg.UseWorktrees {
		var err error
		wt, err = CreateTaskWorktree(r.cfg.RepoPath, t.Title, t.ID)
		if err != nil {
			log.Printf("[builder] worktree creation failed, falling back to direct: %v", err)
		} else {
			workDir = wt.Dir
			// Store worktree context on runner for pipeline merge step.
			r.worktree = wt
		}
	}

	// Build the prompt.
	prompt := r.buildPrompt(t)

	// Execute with Claude CLI (full tool access).
	op, ok := r.cfg.Provider.(decision.IOperator)
	if !ok {
		log.Printf("[builder] provider does not support Operate")
		return
	}

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     workDir,
		Instruction: prompt,
	})
	if err != nil {
		log.Printf("[builder] Operate error: %v", err)
		r.appendDiagnostic(PhaseEvent{Phase: "builder", Error: err.Error(), CostUSD: r.cost.TotalCostUSD})
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
		// When using worktrees, verify build in the worktree dir.
		origRepoPath := r.cfg.RepoPath
		if wt != nil {
			r.cfg.RepoPath = wt.Dir
		}
		defer func() { r.cfg.RepoPath = origRepoPath }()

		// Verify build passes.
		if err := r.verifyBuild(); err != nil {
			log.Printf("[builder] build failed: %v", err)
			r.appendDiagnostic(PhaseEvent{Phase: "builder", Error: err.Error(), CostUSD: r.cost.TotalCostUSD})
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
		r.writeBuildArtifact(t, r.cost.TotalCostUSD, result.Summary)

		// Post cost summary as comment.
		_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
			fmt.Sprintf("Completed. Cost: $%.4f (%d calls total)", r.cost.TotalCostUSD, r.cost.CallCount))

	case "PROGRESS":
		summary := extractSummary(result.Summary)
		_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
			fmt.Sprintf("Progress: %s", summary))

	case "ESCALATE":
		summary := extractSummary(result.Summary)
		// Set task to "escalated" and notify the space owner.
		if err := r.cfg.APIClient.EscalateTask(r.cfg.SpaceSlug, t.ID, summary, ""); err != nil {
			log.Printf("[builder] escalation API error: %v", err)
			_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, t.ID,
				fmt.Sprintf("ESCALATE: %s", summary))
		}
		r.appendDiagnostic(PhaseEvent{
			Phase:   "builder",
			Outcome: "escalated",
			Error:   summary,
			CostUSD: r.cost.TotalCostUSD,
		})
		// Clean up worktree on escalation — work is abandoned.
		if wt != nil {
			wt.Cleanup()
			r.worktree = nil
		}
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

	// Repo context from registry — Builder knows which repo and how to build/test.
	buildCmd := "go build -buildvcs=false ./..."
	testCmd := "go test -buildvcs=false ./..."
	repoName := filepath.Base(r.cfg.RepoPath)
	if reg := r.cfg.Registry; reg != nil {
		if repo, found := reg.ForPath(r.cfg.RepoPath); found {
			repoName = repo.Name
			if repo.BuildCmd != "" {
				buildCmd = repo.BuildCmd
			}
			if repo.TestCmd != "" {
				testCmd = repo.TestCmd
			}
			if repo.ClaudeMD != "" {
				b.WriteString(fmt.Sprintf("## Repository: %s\n\n", repo.Name))
				b.WriteString(repo.ClaudeMD)
				b.WriteString("\n\n---\n\n")
			}
		}
		// Show all available repos so the Builder knows the landscape.
		if summary := reg.Summary(); summary != "" {
			b.WriteString(summary)
			b.WriteString("\n---\n\n")
		}
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

	b.WriteString(fmt.Sprintf("\nYou are working in the **%s** repository.\n", repoName))

	// Instructions — build/test commands from registry, not hardcoded.
	b.WriteString(fmt.Sprintf(`
## Instructions

1. Implement the task described above.
2. Run `+"`%s`"+` to verify compilation.
3. Run `+"`%s`"+` to verify tests pass.
4. If you edit any .templ files, run `+"`templ generate`"+` first.

When done, end your response with exactly:
ACTION: DONE

If you need more work or are partially complete:
ACTION: PROGRESS

If you're stuck and need human help:
ACTION: ESCALATE
`, buildCmd, testCmd))
	return b.String()
}

// ─── Build artifact ──────────────────────────────────────────────────

// writeBuildArtifact writes loop/build.md summarising the completed task.
func (r *Runner) writeBuildArtifact(t api.Node, costUSD float64, operateSummary string) {
	hash := r.gitHash()
	subject := r.gitSubject()
	diffStat := r.gitDiffStat()

	body := t.Body
	if len(body) > 300 {
		body = body[:300] + "..."
	}

	summary := operateSummary
	if len(summary) > 2000 {
		summary = summary[:2000]
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Build: %s\n\n", t.Title))
	b.WriteString(fmt.Sprintf("- **Commit:** %s\n", hash))
	b.WriteString(fmt.Sprintf("- **Subject:** %s\n", subject))
	b.WriteString(fmt.Sprintf("- **Cost:** $%.4f\n", costUSD))
	b.WriteString(fmt.Sprintf("- **Timestamp:** %s\n", time.Now().UTC().Format(time.RFC3339)))
	if body != "" {
		b.WriteString(fmt.Sprintf("\n## Task\n\n%s\n", body))
	}
	if summary != "" {
		b.WriteString(fmt.Sprintf("\n## What Was Built\n\n%s\n", summary))
	}
	if diffStat != "" {
		b.WriteString(fmt.Sprintf("\n## Diff Stat\n\n```\n%s\n```\n", diffStat))
	}

	content := b.String()
	path := filepath.Join(r.cfg.HiveDir, "loop", "build.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Printf("[builder] write build.md: %v", err)
	}

	// Also post to graph as a document caused by the task that triggered the build.
	// Title is normalized so the lookup in critic.go (which normalizes the
	// commit subject the same way) finds this document on retry cycles.
	if r.cfg.APIClient != nil {
		title := fmt.Sprintf("Build: %s", stripRetryPrefixes(t.Title))
		_, _ = r.cfg.APIClient.CreateDocument(r.cfg.SpaceSlug, title, content, []string{t.ID})
	}
}

// gitHash returns the latest commit hash from r.cfg.RepoPath, or "unknown".
func (r *Runner) gitHash() string {
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// gitSubject returns the latest commit subject line, or "unknown".
func (r *Runner) gitSubject() string {
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// gitDiffStat returns the diff stat for HEAD, truncated to 1000 chars.
func (r *Runner) gitDiffStat() string {
	cmd := exec.Command("git", "show", "--stat", "HEAD")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if len(s) > 1000 {
		s = s[:1000] + "\n... (truncated)"
	}
	return s
}

// ─── Build verification ──────────────────────────────────────────────

func (r *Runner) verifyBuild() error {
	// Use build command from registry if available.
	buildCmd := "go build -buildvcs=false ./..."
	if reg := r.cfg.Registry; reg != nil {
		if repo, found := reg.ForPath(r.cfg.RepoPath); found && repo.BuildCmd != "" {
			buildCmd = repo.BuildCmd
		}
	}
	parts := strings.Fields(buildCmd)
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = r.cfg.RepoPath
	// Inherit GOPATH/GOCACHE from environment. Don't override — let the
	// system default work on Linux VMs where these aren't customized.
	cmd.Env = os.Environ()
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

	// Commit. Use full retry-prefix stripping (not just [hive:*]) so that a
	// task titled "Fix: X" and a later "Fix: Fix: X" both produce the same
	// commit subject "[hive:builder] X" — otherwise the Build: document
	// lookup in critic.go (which also normalizes) drifts from the commit key.
	msg := fmt.Sprintf("[hive:%s] %s", r.cfg.Role, stripRetryPrefixes(t.Title))
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

// HasChanges returns true if the repo has uncommitted changes.
func (r *Runner) HasChanges() bool {
	return r.hasUncommittedChanges()
}

// CommitAll stages all changes and commits with the given message.
func (r *Runner) CommitAll(msg string) error {
	if err := r.git("add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	return r.git("commit", "-m", msg)
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

// stripHivePrefix removes leading [hive:xxx] prefixes from s, handling nested
// duplicates produced by prior commit message formatting.
func stripHivePrefix(s string) string {
	for strings.HasPrefix(s, "[hive:") {
		end := strings.Index(s, "]")
		if end == -1 {
			break
		}
		s = strings.TrimSpace(s[end+1:])
	}
	return s
}

// stripRetryPrefixes strips the layers that Critic-driven retries add to a
// task title: leading [hive:*] role prefixes and "Fix: " prefixes, in any
// interleaving. Returns the core title.
//
// Examples:
//   "[hive:builder] Fix: X"                        → "X"
//   "[hive:builder] Fix: [hive:builder] Fix: X"    → "X"
//   "[hive:critic] [hive:builder] Fix: Fix: X"     → "X"
func stripRetryPrefixes(s string) string {
	for {
		before := s
		s = stripHivePrefix(s)
		for strings.HasPrefix(s, "Fix: ") {
			s = strings.TrimPrefix(s, "Fix: ")
		}
		if s == before {
			return s
		}
	}
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
	// Default to PROGRESS — explicit ACTION: DONE is required to close a task.
	// A missing ACTION line means the response was incomplete or an error text,
	// not a confirmation that the work is finished.
	return "PROGRESS"
}

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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
//
// The title is normalized before sluggification: leading [hive:*] prefixes
// and repeated "Fix: " prefixes are collapsed to the core title. Without
// this, Critic-driven retries compound the title into
// "[hive:builder] Fix: [hive:builder] Fix: X", which slugs to
// "fix-hive-builder-fix-hive-builder-..." and loses the meaningful tail
// to the 40-char truncation.
func branchSlug(title string, date time.Time) string {
	dateStr := date.Format("20060102")

	title = stripRetryPrefixes(title)

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

// ModelForRole returns the default model name for a role.
// Override with AGENT_MODEL env var.
func ModelForRole(role string) string {
	if override := os.Getenv("AGENT_MODEL"); override != "" {
		return override
	}
	resolver := modelconfig.DefaultResolver()
	resolved, err := resolver.Resolve(modelconfig.ResolutionInput{Role: role})
	if err != nil {
		return "haiku"
	}
	return resolved.Model
}

// ProviderConfig returns an intelligence.Config for the given role.
func ProviderConfig(role string, budget float64) intelligence.Config {
	resolver := modelconfig.DefaultResolver()
	resolved, err := resolver.Resolve(modelconfig.ResolutionInput{Role: role})
	if err != nil {
		return intelligence.Config{Provider: "claude-cli", Model: "haiku", MaxBudgetUSD: budget}
	}
	cfg := modelconfig.ToIntelligenceConfig(resolved, "")
	cfg.MaxBudgetUSD = budget
	return cfg
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
// any leading [hive:*] prefix (handles compounded prefixes like [hive:builder] [hive:builder]).
func prTitleFromSubject(subject string) string {
	return stripHivePrefix(subject)
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
