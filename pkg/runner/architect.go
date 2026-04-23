package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/hive/pkg/api"
)

// runArchitect reads the Scout's gap report and the PM's directive, then
// creates right-sized tasks for the Builder. Runs BEFORE the Builder.
// The Scout finds gaps. The PM sets direction. The Architect plans and creates tasks.
func (r *Runner) runArchitect(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%4 != 0 {
		return
	}

	// Find the PM's milestone on the board — an open high-priority task
	// with a body that looks like a directive (created by PM this cycle).
	// Fall back to Scout's gap report file if no milestone exists.
	milestone := r.findMilestone()
	scoutReport := ""
	if r.cfg.HiveDir != "" {
		if data, err := os.ReadFile(filepath.Join(r.cfg.HiveDir, "loop", "scout.md")); err == nil {
			scoutReport = string(data)
		}
	}

	context := ""
	if milestone != nil {
		context = fmt.Sprintf("## PM Milestone (from board)\nTitle: %s\n\n%s", milestone.Title, milestone.Body)
		log.Printf("[architect] decomposing milestone: %s", milestone.Title)
	} else if scoutReport != "" {
		context = scoutReport
		log.Printf("[architect] planning from Scout's gap report")
	} else {
		log.Printf("[architect] no milestone or scout report found")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// Use Operate() so the Architect can create tasks directly via the lovyou.ai API.
	// No text parsing — tasks are structured entities created via HTTP POST.
	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if canOperate {
		var milestoneID string
		if milestone != nil {
			milestoneID = milestone.ID
		}
		instruction := buildArchitectOperateInstruction(context, r.cfg.SpaceSlug, milestoneID, r.cfg.APIBase)
		result, err := op.Operate(ctx, decision.OperateTask{
			WorkDir:     r.cfg.RepoPath,
			Instruction: instruction,
		})
		if err != nil {
			log.Printf("[architect] Operate error: %v", err)
			r.appendDiagnostic(PhaseEvent{Phase: "architect", Outcome: "failure", Error: err.Error()})
			if r.cfg.OneShot {
				r.done = true
			}
			return
		}
		r.cost.Record(result.Usage)
		r.dailyBudget.Record(result.Usage.CostUSD)
		log.Printf("[architect] Operate done (cost=$%.4f)", result.Usage.CostUSD)

		// Count tasks created by checking the board.
		tasks, _ := r.cfg.APIClient.GetTasks(r.cfg.SpaceSlug, "")
		openCount := 0
		for _, t := range tasks {
			if t.Kind == "task" && t.State != "done" && t.State != "closed" {
				openCount++
			}
		}
		if openCount > 0 {
			log.Printf("[architect] %d open tasks on board after Operate", openCount)
		} else {
			log.Printf("[architect] no tasks created")
			r.appendDiagnostic(PhaseEvent{Phase: "architect", Outcome: "failure", Error: "Operate produced no tasks"})
		}

		// Complete the milestone.
		if milestone != nil {
			_ = r.cfg.APIClient.CompleteTask(r.cfg.SpaceSlug, milestone.ID)
			log.Printf("[architect] milestone completed: %s", milestone.Title)
		}
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// Fallback: Reason() with text parsing (legacy path).
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	repoCtx := r.readRepoContext()
	prompt := buildArchitectPrompt(sharedCtx, repoCtx, context)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[architect] Reason error: %v", err)
		r.appendDiagnostic(PhaseEvent{Phase: "architect", Outcome: "failure", Error: err.Error()})
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[architect] plan ready (cost=$%.4f)", resp.Usage().CostUSD)

	// Parse subtasks from the plan.
	subtasks := parseArchitectSubtasks(resp.Content())
	if len(subtasks) == 0 {
		// Capture first 2000 chars of the LLM response for diagnostics.
		preview := resp.Content()
		if len(preview) > 2000 {
			preview = preview[:2000]
		}
		log.Printf("[architect] no subtasks found in plan. Preview:\n%s", preview)
		// Emit diagnostic when a real LLM call (cost > 0) produced no parseable subtasks.
		// Error is set to the raw LLM response so PM/Scout can diagnose format failures
		// without re-running. Preview holds the same content (truncated).
		if usage := resp.Usage(); usage.CostUSD > 0 {
			r.appendDiagnostic(PhaseEvent{
				Phase:        "architect",
				Outcome:      "failure",
				Error:        preview,
				Preview:      preview,
				CostUSD:      usage.CostUSD,
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			})
		}
		// Complete the milestone so PM doesn't re-create it.
		if milestone != nil {
			_ = r.cfg.APIClient.CommentTask(r.cfg.SpaceSlug, milestone.ID,
				"Architect could not decompose this milestone into subtasks.")
			_ = r.cfg.APIClient.CompleteTask(r.cfg.SpaceSlug, milestone.ID)
		}
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// Create right-sized tasks for the Builder, caused by the milestone they decompose.
	var milestoneID string
	if milestone != nil {
		milestoneID = milestone.ID
	}
	for _, st := range subtasks {
		if st.title == "" {
			log.Printf("[architect] skipping subtask with empty title")
			continue
		}
		var causes []string
		if milestoneID != "" {
			causes = []string{milestoneID}
		}
		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, st.title, st.desc, st.priority, causes)
		if err != nil {
			log.Printf("[architect] create subtask error: %v", err)
			continue
		}
		if r.cfg.AgentID != "" {
			_ = r.cfg.APIClient.ClaimTask(r.cfg.SpaceSlug, task.ID)
		}
		log.Printf("[architect] created subtask: %s", st.title)
	}

	log.Printf("[architect] decomposed into %d subtasks", len(subtasks))

	// Complete the milestone — it's been decomposed into subtasks.
	if milestone != nil {
		_ = r.cfg.APIClient.CompleteTask(r.cfg.SpaceSlug, milestone.ID)
		log.Printf("[architect] milestone completed: %s", milestone.Title)
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

// findMilestone looks for an open high-priority task on the board that was
// created by the PM (has a directive-style body). Returns nil if not found.
func (r *Runner) findMilestone() *api.Node {
	tasks, err := r.cfg.APIClient.GetTasks(r.cfg.SpaceSlug, "")
	if err != nil {
		return nil
	}
	for _, t := range tasks {
		if t.Kind != "task" || t.State == "done" || t.State == "closed" {
			continue
		}
		// Milestones are high-priority tasks with substantial bodies (PM directives).
		if t.Priority == "high" && len(t.Body) > 200 {
			return &t
		}
	}
	return nil
}

type architectSubtask struct {
	title    string
	desc     string
	priority string
}

// buildArchitectOperateInstruction creates the instruction for the Architect
// when using Operate(). The agent creates tasks directly via HTTP POST to the
// lovyou.ai API — no text parsing needed.
// milestoneID, when non-empty, is embedded in the curl payload as "causes":[milestoneID]
// so every created task declares its causal parent (Invariant 2: CAUSALITY).
func buildArchitectOperateInstruction(context, spaceSlug, milestoneID, apiBase string) string {
	apiKey := os.Getenv("LOVYOU_API_KEY")
	causesSuffix := ""
	if milestoneID != "" {
		causesSuffix = fmt.Sprintf(`,"causes":["%s"]`, milestoneID)
	}
	return fmt.Sprintf(`You are the Architect. Read the milestone below and create 2-4 specific tasks for the Builder.

## Milestone
%s

## How to Create Tasks

Create each task by running a curl command. Do NOT output formatted text — execute the commands directly.

For each task, run:
curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"intend","kind":"task","title":"<TITLE>","description":"<DESCRIPTION>","priority":"high"%s}'

Create 2-4 tasks. Each should:
- Change 1-3 files maximum
- Be implementable in under 10 minutes
- Be specific: name the files, the functions, the changes
- The FIRST task should be the foundation the others build on

After creating all tasks, verify by listing the board:
curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "%s/app/%s/board" | head -200

Do NOT explain your plan in text. Just create the tasks.`, context, apiKey, apiBase, spaceSlug, causesSuffix, apiKey, apiBase, spaceSlug)
}

func buildArchitectPrompt(sharedCtx, repoCtx, scoutReport string) string {
	return fmt.Sprintf(`You are the Architect. The Scout identified a gap. The PM set direction. Your job: read the gap report and create 2-4 right-sized tasks for the Builder.

## Institutional Knowledge
%s

## Target Repo Context
%s

## Scout's Gap Report
%s

## Instructions

1. Read the Scout's report. Understand the gap, evidence, and scope.
2. Design a plan: which files change, what approach to take, what order.
3. Break into 2-4 tasks. Each should:
   - Change 1-3 files maximum
   - Be implementable in one Operate() call (under 10 minutes)
   - Be specific: name the files, the functions, the changes
   - Be self-contained where possible
4. The FIRST task should be the foundation the others build on.

## Output Format

For each task:

SUBTASK_TITLE: <one-line title>
SUBTASK_PRIORITY: <high|medium>
SUBTASK_DESCRIPTION: <2-3 sentences, specific files and changes>`, sharedCtx, repoCtx, scoutReport)
}

// normalizeArchitectResponse strips markdown formatting from LLM output
// so the parsers see clean content regardless of how the model wrapped it.
func normalizeArchitectResponse(content string) string {
	content = strings.TrimSpace(content)
	// Strip opening fence line: ```json, ```text, or plain ```
	if strings.HasPrefix(content, "```") {
		nl := strings.IndexByte(content, '\n')
		if nl >= 0 {
			content = strings.TrimSpace(content[nl+1:])
		}
	}
	// Strip closing fence
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSpace(content[:len(content)-3])
	}
	// Strip bold markers from SUBTASK_ prefixes: **SUBTASK_TITLE:** → SUBTASK_TITLE:
	content = strings.ReplaceAll(content, "**SUBTASK_TITLE:**", "SUBTASK_TITLE:")
	content = strings.ReplaceAll(content, "**SUBTASK_PRIORITY:**", "SUBTASK_PRIORITY:")
	content = strings.ReplaceAll(content, "**SUBTASK_DESCRIPTION:**", "SUBTASK_DESCRIPTION:")
	// Also handle without trailing colon in bold: **SUBTASK_TITLE**:
	content = strings.ReplaceAll(content, "**SUBTASK_TITLE**:", "SUBTASK_TITLE:")
	content = strings.ReplaceAll(content, "**SUBTASK_PRIORITY**:", "SUBTASK_PRIORITY:")
	content = strings.ReplaceAll(content, "**SUBTASK_DESCRIPTION**:", "SUBTASK_DESCRIPTION:")
	return content
}

// jsonSubtask is the expected shape when an LLM returns JSON instead of the
// structured-text format. Field names are lowercase ("title", "description",
// "priority") as commonly produced by LLMs.
type jsonSubtask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

// parseSubtasksJSON attempts to unmarshal content as a JSON array of tasks or
// as a {"tasks":[...]} wrapper object.  Returns nil when content is not valid
// JSON or when the array is empty.
func parseSubtasksJSON(content string) []architectSubtask {
	content = strings.TrimSpace(content)
	if content == "" || content[0] != '[' && content[0] != '{' {
		return nil
	}

	var raw []jsonSubtask

	// Try bare array first: [{...}, ...]
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		// Try wrapper object: {"tasks": [...]}
		var wrapper struct {
			Tasks []jsonSubtask `json:"tasks"`
		}
		if err2 := json.Unmarshal([]byte(content), &wrapper); err2 != nil {
			return nil
		}
		raw = wrapper.Tasks
	}

	if len(raw) == 0 {
		return nil
	}

	tasks := make([]architectSubtask, 0, len(raw))
	for _, j := range raw {
		if j.Title == "" {
			continue
		}
		prio := j.Priority
		switch prio {
		case "high", "medium", "low", "urgent":
		default:
			prio = "high"
		}
		tasks = append(tasks, architectSubtask{
			title:    j.Title,
			desc:     j.Description,
			priority: prio,
		})
	}
	return tasks
}

func parseArchitectSubtasks(content string) []architectSubtask {
	content = normalizeArchitectResponse(content)
	// Try JSON first — handles LLM responses that return raw JSON arrays or
	// {"tasks":[...]} objects instead of the structured-text format.
	if tasks := parseSubtasksJSON(content); len(tasks) > 0 {
		return tasks
	}
	// Try strict format next.
	tasks := parseSubtasksStrict(content)
	if len(tasks) > 0 {
		return tasks
	}
	// If strict markers are present but strict parsing returned nothing, log it
	// so we can diagnose the format mismatch rather than silently falling through.
	if strings.Contains(content, "SUBTASK_TITLE:") {
		log.Printf("[architect] strict format markers present but strict parse returned 0 tasks — falling back to markdown")
	}
	// Fall back to markdown parsing (numbered lists, bold titles, etc.).
	if tasks := parseSubtasksMarkdown(content); len(tasks) > 0 {
		return tasks
	}
	// Last resort: find the first '[' — handles LLMs that prepend prose before a JSON array.
	if idx := strings.Index(content, "["); idx >= 0 {
		if tasks := parseSubtasksJSON(content[idx:]); len(tasks) > 0 {
			return tasks
		}
	}
	return nil
}

// parseSubtasksStrict parses SUBTASK_TITLE:/SUBTASK_PRIORITY:/SUBTASK_DESCRIPTION: format.
// Description continuation lines (non-empty, non-fence lines following SUBTASK_DESCRIPTION:)
// are appended to the description until a blank line, code fence, or new SUBTASK_ prefix.
func parseSubtasksStrict(content string) []architectSubtask {
	var tasks []architectSubtask
	var current architectSubtask
	inDesc := false

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SUBTASK_TITLE:") {
			if current.title != "" {
				tasks = append(tasks, current)
			}
			current = architectSubtask{title: strings.TrimSpace(strings.TrimPrefix(line, "SUBTASK_TITLE:"))}
			inDesc = false
		} else if strings.HasPrefix(line, "SUBTASK_PRIORITY:") {
			current.priority = strings.TrimSpace(strings.TrimPrefix(line, "SUBTASK_PRIORITY:"))
			inDesc = false
		} else if strings.HasPrefix(line, "SUBTASK_DESCRIPTION:") {
			current.desc = strings.TrimSpace(strings.TrimPrefix(line, "SUBTASK_DESCRIPTION:"))
			inDesc = true
		} else if inDesc {
			if line == "" || strings.HasPrefix(line, "```") {
				inDesc = false
			} else {
				current.desc += " " + line
			}
		}
	}
	if current.title != "" {
		tasks = append(tasks, current)
	}

	for i := range tasks {
		switch tasks[i].priority {
		case "high", "medium", "low", "urgent":
		default:
			tasks[i].priority = "high"
		}
	}
	return tasks
}

// parseSubtasksMarkdown parses common LLM output formats:
// - "1. **Title** — description"
// - "1. Title\n   description"
// - "### Task 1: Title\ndescription"
// - "- **Title**: description"
func parseSubtasksMarkdown(content string) []architectSubtask {
	var tasks []architectSubtask
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		title := ""

		// Match: "1. **Title** — desc" or "1. **Title**: desc" or "1. Title"
		if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && (line[1] == '.' || (len(line) > 2 && line[2] == '.')) {
			// Strip number prefix: "1. " or "1) "
			after := line
			for j := 0; j < len(after) && (after[j] >= '0' && after[j] <= '9'); j++ {
				after = after[j+1:]
			}
			after = strings.TrimLeft(after, ".) ")
			title, _ = extractTitleAndDesc(after)
		}

		// Match: "### Title" or "## Task N: Title"
		if title == "" && strings.HasPrefix(line, "#") {
			h := strings.TrimLeft(line, "# ")
			h = strings.TrimPrefix(h, "Task ")
			// Strip leading number: "1: Title" or "1 — Title"
			for j := 0; j < len(h) && h[j] >= '0' && h[j] <= '9'; j++ {
				if j+1 < len(h) && (h[j+1] == ':' || h[j+1] == '.') {
					h = strings.TrimSpace(h[j+2:])
					break
				}
			}
			title = h
		}

		// Match: "- **Title**" or "* **Title**"
		if title == "" && (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")) {
			after := strings.TrimSpace(line[2:])
			if strings.HasPrefix(after, "**") {
				title, _ = extractTitleAndDesc(after)
			}
		}

		if title == "" {
			continue
		}

		// Collect description from following lines until next task or blank line.
		var descLines []string
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next == "" {
				break
			}
			// Stop if next line looks like another task.
			if len(next) > 2 && next[0] >= '1' && next[0] <= '9' && (next[1] == '.' || (len(next) > 2 && next[2] == '.')) {
				break
			}
			if strings.HasPrefix(next, "#") || strings.HasPrefix(next, "- **") || strings.HasPrefix(next, "* **") {
				break
			}
			if strings.HasPrefix(next, "SUBTASK_") {
				break
			}
			i++
			descLines = append(descLines, next)
		}

		desc := strings.Join(descLines, " ")
		// Also check if title line itself had a description after — or :
		if desc == "" {
			_, inlineDesc := extractTitleAndDesc(line)
			desc = inlineDesc
		}

		tasks = append(tasks, architectSubtask{
			title:    title,
			desc:     desc,
			priority: "high",
		})
	}

	// Cap at 6 tasks — more than that means the parser grabbed noise.
	if len(tasks) > 6 {
		tasks = tasks[:6]
	}
	return tasks
}

// extractTitleAndDesc splits "**Bold title** — description" or "**Bold**: desc" into parts.
func extractTitleAndDesc(s string) (title, desc string) {
	s = strings.TrimSpace(s)
	// Bold: **title** — desc
	if strings.HasPrefix(s, "**") {
		end := strings.Index(s[2:], "**")
		if end >= 0 {
			title = strings.TrimSpace(s[2 : end+2])
			rest := strings.TrimSpace(s[end+4:])
			rest = strings.TrimLeft(rest, "—–-:. ")
			return title, rest
		}
	}
	// Bold: __title__ — desc
	if strings.HasPrefix(s, "__") {
		end := strings.Index(s[2:], "__")
		if end >= 0 {
			title = strings.TrimSpace(s[2 : end+2])
			rest := strings.TrimSpace(s[end+4:])
			rest = strings.TrimLeft(rest, "—–-:. ")
			return title, rest
		}
	}
	// Backtick: `title` — desc
	if strings.HasPrefix(s, "`") {
		end := strings.Index(s[1:], "`")
		if end >= 0 {
			title = strings.TrimSpace(s[1 : end+1])
			rest := strings.TrimSpace(s[end+2:])
			rest = strings.TrimLeft(rest, "—–-:. ")
			return title, rest
		}
	}
	// No formatting — take first sentence or up to 80 chars.
	if idx := strings.IndexAny(s, ".;—–"); idx > 0 && idx < 80 {
		return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
	}
	if len(s) > 80 {
		return s[:80], ""
	}
	return s, ""
}
