package runner

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
)

// runObserver looks at the product from a human's perspective.
// Uses Operate() — needs to grep code, fetch URLs, read multiple files.
func (r *Runner) runObserver(ctx context.Context) {
	// Run every 16th tick (~4 minutes) in continuous mode.
	if !r.cfg.OneShot && r.tick%16 != 0 {
		return
	}

	log.Printf("[observer] tick %d: observing product", r.tick)

	// Build the observation instruction.
	instruction := buildObserverInstruction(r.cfg.RepoPath)

	// Use Operate() — the Observer needs file access to grep patterns, read templates, etc.
	op, ok := r.cfg.Provider.(decision.IOperator)
	if !ok {
		log.Printf("[observer] provider does not support Operate, falling back to Reason")
		r.runObserverReason(ctx)
		return
	}

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.RepoPath,
		Instruction: instruction,
		AllowedTools: []string{"Read", "Glob", "Grep", "Bash"},
	})
	if err != nil {
		log.Printf("[observer] Operate error: %v", err)
		return
	}

	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[observer] observation done (cost=$%.4f)", result.Usage.CostUSD)

	// Parse tasks from the observation.
	tasks := parseObserverTasks(result.Summary)
	if len(tasks) == 0 {
		log.Printf("[observer] no actionable findings")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// Create tasks for the most important findings (max 2 per run).
	created := 0
	for _, t := range tasks {
		if created >= 2 {
			break
		}

		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, t.title, t.desc, t.priority)
		if err != nil {
			log.Printf("[observer] create task error: %v", err)
			continue
		}
		log.Printf("[observer] created task %s: %s", task.ID, t.title)

		if r.cfg.AgentID != "" {
			_ = r.cfg.APIClient.ClaimTask(r.cfg.SpaceSlug, task.ID)
		}
		created++
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

// runObserverReason is a fallback when Operate() isn't available.
// Gathers context manually and uses Reason().
func (r *Runner) runObserverReason(ctx context.Context) {
	// Gather what we can without tool access.
	routes := r.grepRoutes()
	kinds := r.grepEntityKinds()
	health := r.checkLiveHealth()

	prompt := fmt.Sprintf(`You are the Observer. Analyze this product for gaps.

## Routes registered
%s

## Entity kinds defined
%s

## Live health check
%s

Report findings as:
TASK_TITLE: <title>
TASK_PRIORITY: <priority>
TASK_DESCRIPTION: <description>

You may report up to 2 findings. If everything looks good, say "No issues found."`, routes, kinds, health)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[observer] Reason error: %v", err)
		return
	}
	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)

	tasks := parseObserverTasks(resp.Content())
	for _, t := range tasks {
		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, t.title, t.desc, t.priority)
		if err != nil {
			continue
		}
		log.Printf("[observer] created task %s: %s", task.ID, t.title)
		if r.cfg.AgentID != "" {
			_ = r.cfg.APIClient.ClaimTask(r.cfg.SpaceSlug, task.ID)
		}
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

type observerTask struct {
	title    string
	desc     string
	priority string
}

func parseObserverTasks(content string) []observerTask {
	var tasks []observerTask
	var current observerTask

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TASK_TITLE:") {
			if current.title != "" {
				tasks = append(tasks, current)
			}
			current = observerTask{title: strings.TrimSpace(strings.TrimPrefix(line, "TASK_TITLE:"))}
		} else if strings.HasPrefix(line, "TASK_PRIORITY:") {
			current.priority = strings.TrimSpace(strings.TrimPrefix(line, "TASK_PRIORITY:"))
		} else if strings.HasPrefix(line, "TASK_DESCRIPTION:") {
			current.desc = strings.TrimSpace(strings.TrimPrefix(line, "TASK_DESCRIPTION:"))
		}
	}
	if current.title != "" {
		tasks = append(tasks, current)
	}

	// Validate priorities.
	for i := range tasks {
		switch tasks[i].priority {
		case "urgent", "high", "medium", "low":
		default:
			tasks[i].priority = "medium"
		}
	}
	return tasks
}

func buildObserverInstruction(repoPath string) string {
	return fmt.Sprintf(`You are the Observer. Your job is to look at this product as a human user would and identify what's broken, inconsistent, or missing.

## Your repo: %s

## What to check

1. **Consistency audit:** Grep for all entity kind constants (KindTask, KindPost, etc). For EACH kind, verify:
   - Handler exists (handleX function)
   - Route registered (GET /app/{slug}/X)
   - Sidebar nav entry exists
   - Mobile nav entry exists
   - Search/filter works on the view
   - Create form exists
   - Added to intend allowlist (check the "if nodeKind != KindProject &&" line)

2. **Route health:** List all registered routes. Check for dead routes or missing handlers.

3. **Live check:** Run 'curl -s -o /dev/null -w "%%{http_code}" https://lovyou.ai/' and similar for key pages.

4. **User flow gaps:** Read the main templates. Is there a clear path from landing → sign in → create space → use the product? Any dead ends?

5. **Spec vs reality:** Read the CLAUDE.md or any spec files. Does the code deliver what was specified?

## Output format

For each finding, output:
TASK_TITLE: <one-line title>
TASK_PRIORITY: <urgent|high|medium|low>
TASK_DESCRIPTION: <2-3 sentences, specific files to change>

Report at most 2 findings — the most important ones. If everything looks good, say "No issues found."

## Honest limits
You cannot see the rendered UI. You cannot judge aesthetics or feel. Focus on what you CAN verify: code completeness, route health, pattern consistency, spec compliance.`, repoPath)
}

// Helper: grep registered routes from handlers.go.
func (r *Runner) grepRoutes() string {
	cmd := exec.Command("grep", "-n", "mux.Handle", "graph/handlers.go")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "(could not grep routes)"
	}
	return string(out)
}

// Helper: grep entity kind constants from store.go.
func (r *Runner) grepEntityKinds() string {
	cmd := exec.Command("grep", "-n", "Kind.*=", "graph/store.go")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "(could not grep kinds)"
	}
	return string(out)
}

// Helper: check if the live site is up.
func (r *Runner) checkLiveHealth() string {
	pages := []string{
		"https://lovyou.ai/",
		"https://lovyou.ai/discover",
		"https://lovyou.ai/search",
	}
	var results []string
	for _, url := range pages {
		cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "5", url)
		out, err := cmd.Output()
		if err != nil {
			results = append(results, fmt.Sprintf("%s → error", url))
		} else {
			results = append(results, fmt.Sprintf("%s → %s", url, string(out)))
		}
	}
	return strings.Join(results, "\n")
}
