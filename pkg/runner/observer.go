package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/hive/pkg/api"
)

// runObserver looks at the product from a human's perspective.
// Uses Operate() — needs to grep code, fetch URLs, read multiple files.
func (r *Runner) runObserver(ctx context.Context) {
	// Run every 16th tick (~4 minutes) in continuous mode.
	if !r.cfg.OneShot && r.tick%16 != 0 {
		return
	}

	log.Printf("[observer] tick %d: observing product", r.tick)

	// Pre-fetch claims so the Observer has ground-truth count injected into context.
	// Without this, the Observer only calls /board (kind=task only) and concludes zero claims.
	var claimsSummary string
	var fallbackCauseID string // first claim ID used as cause when LLM emits TASK_CAUSE:none
	if r.cfg.APIClient != nil {
		claims, err := r.cfg.APIClient.GetClaims(r.cfg.SpaceSlug, 50)
		if err != nil {
			log.Printf("[observer] could not pre-fetch claims: %v", err)
		} else {
			claimsSummary = buildClaimsSummary(claims)
			if claimsSummary != "" {
				log.Printf("[observer] pre-fetched %d claims for context injection", len(claims))
			}
			if len(claims) > 0 {
				fallbackCauseID = claims[0].ID
			}
		}
	}

	// Build the observation instruction.
	apiKey := os.Getenv("LOVYOU_API_KEY")
	if apiKey == "" {
		log.Printf("[observer] LOVYOU_API_KEY not set; graph integrity audit will be skipped")
	}
	instruction := buildObserverInstruction(r.cfg.RepoPath, r.cfg.SpaceSlug, apiKey, claimsSummary, r.cfg.APIBase)

	// Use Operate() — the Observer needs file access to grep patterns, read templates, etc.
	op, ok := r.cfg.Provider.(decision.IOperator)
	if !ok {
		log.Printf("[observer] provider does not support Operate, falling back to Reason")
		r.runObserverReason(ctx, claimsSummary, fallbackCauseID)
		return
	}

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:      r.cfg.RepoPath,
		Instruction:  instruction,
		AllowedTools: []string{"Read", "Glob", "Grep", "Bash", "mcp__knowledge__knowledge_search"},
	})
	if err != nil {
		log.Printf("[observer] Operate error: %v", err)
		return
	}

	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[observer] observation done (cost=$%.4f)", result.Usage.CostUSD)

	// Observer creates tasks directly via Operate() — no text parsing needed.

	if r.cfg.OneShot {
		r.done = true
	}
}

// runObserverReason is a fallback when Operate() isn't available.
// Gathers context manually and uses Reason().
// fallbackCauseID is used when the LLM emits TASK_CAUSE:none — ensures CAUSALITY invariant holds.
func (r *Runner) runObserverReason(ctx context.Context, claimsSummary, fallbackCauseID string) {
	// Gather what we can without tool access.
	routes := r.grepRoutes()
	kinds := r.grepEntityKinds()
	health := r.checkLiveHealth()

	claimsSection := ""
	if claimsSummary != "" {
		claimsSection = fmt.Sprintf("\n## Claims (ground truth, pre-fetched)\n%s\n", claimsSummary)
	}

	prompt := fmt.Sprintf(`You are the Observer. Analyze this product for gaps.

## Routes registered
%s

## Entity kinds defined
%s

## Live health check
%s
%s
Report findings as:
TASK_TITLE: <title>
TASK_PRIORITY: <priority>
TASK_DESCRIPTION: <description>
TASK_CAUSE: <node_id_of_triggering_graph_node_or_none>

TASK_CAUSE must be the ID of the specific board node or claim that triggered this finding
(Invariant 2: CAUSALITY — every created node must declare its cause). Use "none" only if
there is genuinely no triggering node.

You may report up to 2 findings. If everything looks good, say "No issues found."`, routes, kinds, health, claimsSection)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[observer] Reason error: %v", err)
		return
	}
	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)

	tasks := parseObserverTasks(resp.Content())
	for _, t := range tasks {
		causeID := t.causeID
		if causeID == "" {
			causeID = fallbackCauseID // Invariant 2: CAUSALITY — fall back to first known node
		} else if r.cfg.APIClient != nil {
			// Validate LLM-provided cause ID — hallucinated IDs silently break CAUSALITY (Lesson 170).
			if !r.cfg.APIClient.NodeExists(r.cfg.SpaceSlug, causeID) {
				log.Printf("[observer] warning: LLM cause ID %q not found on graph; using fallback", causeID)
				causeID = fallbackCauseID
			}
		}
		var causes []string
		if causeID != "" {
			causes = []string{causeID}
		}
		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, t.title, t.desc, t.priority, causes)
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
	causeID  string // node ID that triggered this finding (Invariant 2: CAUSALITY)
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
		} else if strings.HasPrefix(line, "TASK_CAUSE:") {
			id := strings.TrimSpace(strings.TrimPrefix(line, "TASK_CAUSE:"))
			if id != "" && id != "none" && id != "N/A" {
				current.causeID = id
			}
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

// buildClaimsSummary formats pre-fetched claims as a grounding context string.
// Returns empty string if no claims — callers should handle the empty case gracefully.
func buildClaimsSummary(claims []api.Node) string {
	if len(claims) == 0 {
		return ""
	}
	const maxSample = 5
	var titles []string
	for i := range claims {
		if i >= maxSample {
			break
		}
		titles = append(titles, fmt.Sprintf("%q", claims[i].Title))
	}
	suffix := ""
	if len(claims) > maxSample {
		suffix = fmt.Sprintf(" (and %d more)", len(claims)-maxSample)
	}
	return fmt.Sprintf("%d claims exist%s. Titles: %s", len(claims), suffix, strings.Join(titles, ", "))
}

func buildObserverInstruction(repoPath, spaceSlug, apiKey, claimsSummary, apiBase string) string {
	part2 := buildPart2Instruction(spaceSlug, apiKey, claimsSummary, apiBase)
	return fmt.Sprintf(`You are the Observer. Audit both the product AND the hive's own graph for integrity.

## Your repo: %s

## Part 1: Product Audit

1. **Consistency:** Grep entity kind constants. For each, verify handler, route, sidebar nav, create form exist.
2. **Route health:** curl key pages — the base URL, /discover, /hive — check status codes.
3. **User flow:** Is there a clear path from landing → sign in → create space → use product?

%s

Also use mcp__knowledge__knowledge_search to check:
- Are lessons (claims) being asserted? Or just documents?
- Are reflections searchable? Or only in flat files?
- Do agents use the right entity kind for each artifact?

## Output

%s

If everything looks good, say "No issues found."`, repoPath, part2, buildOutputInstruction(spaceSlug, apiKey, apiBase))
}

func buildPart2Instruction(spaceSlug, apiKey, claimsSummary, apiBase string) string {
	if apiKey == "" {
		return `## Part 2: Graph Integrity Audit

(Skipped — LOVYOU_API_KEY not set. Authenticated requests require an API key.)`
	}

	groundTruth := ""
	if claimsSummary != "" {
		groundTruth = fmt.Sprintf("\n**Ground truth (pre-fetched by runner — do not contradict):**\n%s\n", claimsSummary)
	}

	return fmt.Sprintf(`## Part 2: Graph Integrity Audit
%s
Check the hive's own data on the board for structural issues:

curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "%s/app/%s/board"

Also fetch claims to audit knowledge integrity (claims exist — do not report zero without checking):

curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "%s/app/%s/knowledge?tab=claims&limit=50"

Look for:
1. **Unlinked nodes** — critiques/claims that don't reference the build they review
2. **Stale data** — open tasks that have been active for days with no progress
3. **Title compounding** — tasks with "Fix: Fix: Fix:..." prefixes (should be stripped)
4. **Schema violations** — any field using display names where IDs should be used (Invariant 11)
5. **Orphaned milestones** — PM milestones still open after their subtasks completed
6. **Claim integrity** — claims with no body, no causes, or stuck in challenged state with no resolution
7. **Meta-tasks** — any task whose sole purpose is to close/complete another task. These are board noise. Close BOTH the meta-task AND the target task inline using op=complete. Do not create a new task for this.

**Board hygiene rule:** If you find a task that says "close task X" or "complete task Y" or "mark Z as done", that is a meta-task created by a prior Observer bug. Close the meta-task itself with op=complete. Then evaluate whether the target task should also be closed.`, groundTruth, apiKey, apiBase, spaceSlug, apiKey, apiBase, spaceSlug)
}

func buildOutputInstruction(spaceSlug, apiKey, apiBase string) string {
	if apiKey == "" {
		return `Report the most important findings (max 2) as:
TASK_TITLE: <title>
TASK_PRIORITY: <priority>
TASK_DESCRIPTION: <description>`
	}
	return fmt.Sprintf(`## Acting on findings — two categories, different responses

**IMPORTANT: Do NOT create a task to close another task. That is the defect you must avoid.**

### Category A — Administrative corrections (act NOW, inline)
If the action requires no code change — closing a false-positive, completing a stale task,
removing board noise — execute it directly with op=complete or op=edit:

Close a false-positive task:
curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"complete","node_id":"<NODE_ID>"}'

Mark a task active/in-progress:
curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"edit","node_id":"<NODE_ID>","state":"active"}'

Heuristic: if you can fix it without writing code, fix it now. Do not defer.

### Category B — Code changes needed (create a task, max 2)
Only create a task if the finding requires a Builder to write or change code:

curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"intend","kind":"task","title":"<TITLE>","description":"<DESCRIPTION>","priority":"<PRIORITY>","causes":["<NODE_ID>"]}'

Replace <NODE_ID> with the ID of the specific board node, claim, or document that
triggered this finding (Invariant 2: CAUSALITY — every intend op must declare its cause).
Use the ID you found in the board or knowledge query above.

**Rule:** Creating a task to close a task is always wrong. Close it yourself.`, apiKey, apiBase, spaceSlug, apiKey, apiBase, spaceSlug, apiKey, apiBase, spaceSlug)
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
		r.cfg.APIBase + "/",
		r.cfg.APIBase + "/discover",
		r.cfg.APIBase + "/search",
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
