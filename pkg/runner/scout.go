package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
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

	// Use Operate() so the Scout can search the knowledge layer and the codebase.
	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if canOperate {
		r.runScoutOperate(ctx, op)
	} else {
		r.runScoutReason(ctx)
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

// runScoutOperate uses Operate() — the Scout searches knowledge, reads code,
// and writes the gap report to loop/scout.md AND posts it to the graph.
func (r *Runner) runScoutOperate(ctx context.Context, op decision.IOperator) {
	instruction := fmt.Sprintf(`You are the Scout. Identify ONE product gap — the most important thing the hive should build next.

## Your Tools
- Use knowledge.search to find what's already built (avoid rediscovering existing work)
- Use knowledge.get to read the backlog, design docs, and prior reflections
- Use Read/Grep/Glob to examine the target repo codebase
- Use Bash to check git log, run tests, inspect state

## Steps
1. Search knowledge for "backlog" and read the priorities
2. Search knowledge for "Director mandate" — check for binding instructions
3. Read the target repo's CLAUDE.md for architecture context
4. Check recent git log: git log --oneline -20
5. Identify ONE gap — product gaps outrank code gaps
6. Write the gap report to loop/scout.md with: Gap, Evidence, Impact, Scope, Suggestion
7. Also post the report as a document to the board:
   curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "https://lovyou.ai/app/%s/op" -d '{"op":"express","title":"Scout Report: <GAP>","body":"<REPORT>"}'

## Target repo: %s

Write the report. Be specific — name files, functions, and exact changes.`,
		os.Getenv("LOVYOU_API_KEY"), r.cfg.SpaceSlug, r.cfg.RepoPath)

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.RepoPath,
		Instruction: instruction,
	})
	if err != nil {
		log.Printf("[scout] Operate error: %v", err)
		return
	}

	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[scout] Operate done (cost=$%.4f)", result.Usage.CostUSD)
}

// runScoutReason is the legacy fallback.
func (r *Runner) runScoutReason(ctx context.Context) {
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	stateContext := r.readScoutSection()
	repoContext := r.readRepoContext()
	gitLog := r.recentGitLog()
	boardSummary := r.boardSummary()

	prompt := buildScoutPrompt(r.cfg.RepoPath, sharedCtx, repoContext, stateContext, gitLog, boardSummary)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[scout] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[scout] Reason done (cost=$%.4f)", resp.Usage().CostUSD)

	report := resp.Content()
	if r.cfg.HiveDir != "" {
		reportPath := filepath.Join(r.cfg.HiveDir, "loop", "scout.md")
		if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
			log.Printf("[scout] write report error: %v", err)
		}
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
