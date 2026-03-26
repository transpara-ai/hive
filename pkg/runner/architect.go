package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// runArchitect reads the Scout's gap report and the PM's directive, then
// creates right-sized tasks for the Builder. Runs BEFORE the Builder.
// The Scout finds gaps. The PM sets direction. The Architect plans and creates tasks.
func (r *Runner) runArchitect(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%4 != 0 {
		return
	}

	// Read the Scout's gap report.
	scoutReport := ""
	if r.cfg.HiveDir != "" {
		if data, err := os.ReadFile(filepath.Join(r.cfg.HiveDir, "loop", "scout.md")); err == nil {
			scoutReport = string(data)
		}
	}

	if scoutReport == "" {
		log.Printf("[architect] no scout report found")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	log.Printf("[architect] planning from Scout's gap report")

	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	repoCtx := r.readRepoContext()

	prompt := buildArchitectPrompt(sharedCtx, repoCtx, scoutReport)

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

	if r.cfg.OneShot {
		r.done = true
	}
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
