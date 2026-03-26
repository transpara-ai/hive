package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

)

// maxAgentTasks is the cap on open tasks assigned to the agent.
// Scout won't create new tasks if the agent already has this many.
const maxAgentTasks = 3

// runScout reads project state and creates concrete tasks for the builder.
func (r *Runner) runScout(ctx context.Context) {
	// Run every 8th tick (~2 minutes at 15s interval). Always run in one-shot mode.
	if !r.cfg.OneShot && r.tick%8 != 0 {
		return
	}

	// Check how many open tasks the agent already has.
	agentTasks, err := r.countAgentTasks()
	if err != nil {
		log.Printf("[scout] tick %d: error counting tasks: %v", r.tick, err)
		return
	}
	if agentTasks >= maxAgentTasks {
		log.Printf("[scout] tick %d: agent has %d tasks (max %d), skipping", r.tick, agentTasks, maxAgentTasks)
		return
	}

	log.Printf("[scout] tick %d: scouting (agent has %d/%d tasks)", r.tick, agentTasks, maxAgentTasks)

	// Gather context.
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	stateContext := r.readScoutSection()
	repoContext := r.readRepoContext()
	gitLog := r.recentGitLog()
	boardSummary := r.boardSummary()

	// Build the scouting prompt.
	prompt := buildScoutPrompt(r.cfg.RepoPath, sharedCtx, repoContext, stateContext, gitLog, boardSummary)

	// Call Reason() — no tools, just thinking.
	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[scout] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[scout] Reason done (cost=$%.4f)", resp.Usage().CostUSD)

	// Write the gap report to loop/scout.md for PM and Architect to read.
	// The Scout identifies gaps. The Architect creates tasks.
	report := resp.Content()
	if r.cfg.HiveDir != "" {
		reportPath := filepath.Join(r.cfg.HiveDir, "loop", "scout.md")
		if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
			log.Printf("[scout] write report error: %v", err)
		} else {
			log.Printf("[scout] gap report written to loop/scout.md")
		}
	}

	// In one-shot mode, signal completion.
	if r.cfg.OneShot {
		r.done = true
	}
}

func (r *Runner) countAgentTasks() (int, error) {
	tasks, err := r.cfg.APIClient.GetTasks(r.cfg.SpaceSlug, "")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, t := range tasks {
		if t.Kind != "task" || t.State == "done" || t.State == "closed" {
			continue
		}
		if r.cfg.AgentID != "" && t.AssigneeID == r.cfg.AgentID {
			count++
		}
	}
	return count, nil
}

// readScoutSection extracts the "What the Scout Should Focus On Next" section
// from state.md. Falls back to the last 2000 chars if the section isn't found.
func (r *Runner) readScoutSection() string {
	if r.cfg.HiveDir == "" {
		return "(state.md not available)"
	}
	path := filepath.Join(r.cfg.HiveDir, "loop", "state.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("(could not read state.md: %v)", err)
	}

	s := string(data)

	// Try to extract just the scout section.
	marker := "## What the Scout Should Focus On Next"
	idx := strings.Index(s, marker)
	if idx >= 0 {
		section := s[idx:]
		// Find the next ## heading (end of section).
		nextH2 := strings.Index(section[len(marker):], "\n## ")
		if nextH2 >= 0 {
			section = section[:len(marker)+nextH2]
		}
		if len(section) > 3000 {
			section = section[:3000] + "\n... (truncated)"
		}
		return section
	}

	// Fallback: last 2000 chars.
	if len(s) > 2000 {
		return "..." + s[len(s)-2000:]
	}
	return s
}

// readRepoContext reads the target repo's CLAUDE.md for product context.
func (r *Runner) readRepoContext() string {
	// Try CLAUDE.md in the repo root.
	path := filepath.Join(r.cfg.RepoPath, "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "(no CLAUDE.md in target repo)"
	}
	s := string(data)
	if len(s) > 3000 {
		s = s[:3000] + "\n... (truncated)"
	}
	return s
}

func (r *Runner) recentGitLog() string {
	cmd := exec.Command("git", "log", "--oneline", "-20")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.Output()
	if err != nil {
		return "(git log unavailable)"
	}
	return string(out)
}

func (r *Runner) boardSummary() string {
	tasks, err := r.cfg.APIClient.GetTasks(r.cfg.SpaceSlug, "")
	if err != nil {
		return "(board unavailable)"
	}

	var open, assigned int
	var titles []string
	for _, t := range tasks {
		if t.Kind != "task" || t.State == "done" || t.State == "closed" {
			continue
		}
		open++
		if t.AssigneeID != "" {
			assigned++
		}
		if len(titles) < 10 {
			titles = append(titles, fmt.Sprintf("- [%s] %s", t.Priority, t.Title))
		}
	}

	return fmt.Sprintf("Open tasks: %d (%d assigned)\nRecent:\n%s", open, assigned, strings.Join(titles, "\n"))
}

func buildScoutPrompt(repoPath, sharedCtx, repoContext, state, gitLog, board string) string {
	return fmt.Sprintf(`You are the Scout. Your job is to identify ONE concrete, implementable gap and produce a task for the Builder.

## Institutional Knowledge
%s

## CRITICAL: Target Repo

The Builder operates on: %s
You MUST create tasks that can be implemented in THIS repo. Do NOT create tasks about the hive runtime, agent infrastructure, or other repos.

## Target Repo Context
%s

## Recent Commits (target repo)
%s

## What To Build Next (from state.md)
%s

## Current Board
%s

## Instructions

1. Read the repo context and state. Identify the SINGLE highest-priority product gap.
2. Product features outrank infrastructure. What would make the product better for users?
3. Reference specific files, functions, patterns you found.
4. Check recent commits — don't report gaps that were already filled.
5. You write a GAP REPORT. You do NOT create tasks. The Architect creates tasks from your report.

## Output Format

Write a gap report. Include:
- **Gap:** What's missing, in one sentence
- **Evidence:** What you found in the code/state/board that proves this is a gap
- **Impact:** Why this matters for users
- **Scope:** Which files/areas are involved
- **Suggestion:** Your recommendation for what to build (the Architect decides the actual plan)`, sharedCtx, repoPath, repoContext, gitLog, state, board)
}

func parseScoutTask(content string) (title, desc, priority string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TASK_TITLE:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "TASK_TITLE:"))
		} else if strings.HasPrefix(line, "TASK_PRIORITY:") {
			priority = strings.TrimSpace(strings.TrimPrefix(line, "TASK_PRIORITY:"))
		} else if strings.HasPrefix(line, "TASK_DESCRIPTION:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "TASK_DESCRIPTION:"))
		}
	}
	// Validate priority.
	switch priority {
	case "urgent", "high", "medium", "low":
		// ok
	default:
		priority = "medium"
	}
	return title, desc, priority
}
