package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovyou-ai/hive/pkg/api"
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

	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	repoCtx := r.readRepoContext()

	prompt := buildArchitectPrompt(sharedCtx, repoCtx, context)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[architect] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[architect] plan ready (cost=$%.4f)", resp.Usage().CostUSD)

	// Parse subtasks from the plan.
	subtasks := parseArchitectSubtasks(resp.Content())
	if len(subtasks) == 0 {
		log.Printf("[architect] no subtasks found in plan")
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

	// Create right-sized tasks for the Builder.
	for _, st := range subtasks {
		task, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, st.title, st.desc, st.priority)
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

func parseArchitectSubtasks(content string) []architectSubtask {
	// Try strict format first.
	tasks := parseSubtasksStrict(content)
	if len(tasks) > 0 {
		return tasks
	}
	// Fall back to markdown parsing (numbered lists, bold titles, etc.).
	return parseSubtasksMarkdown(content)
}

// parseSubtasksStrict parses SUBTASK_TITLE:/SUBTASK_PRIORITY:/SUBTASK_DESCRIPTION: format.
func parseSubtasksStrict(content string) []architectSubtask {
	var tasks []architectSubtask
	var current architectSubtask

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SUBTASK_TITLE:") {
			if current.title != "" {
				tasks = append(tasks, current)
			}
			current = architectSubtask{title: strings.TrimSpace(strings.TrimPrefix(line, "SUBTASK_TITLE:"))}
		} else if strings.HasPrefix(line, "SUBTASK_PRIORITY:") {
			current.priority = strings.TrimSpace(strings.TrimPrefix(line, "SUBTASK_PRIORITY:"))
		} else if strings.HasPrefix(line, "SUBTASK_DESCRIPTION:") {
			current.desc = strings.TrimSpace(strings.TrimPrefix(line, "SUBTASK_DESCRIPTION:"))
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
			after := strings.TrimLeft(line, "-* ")
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
