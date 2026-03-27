package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
)

// runPM reads the backlog, recent work, and board state, then creates
// a milestone on the board with the next strategic priority.
func (r *Runner) runPM(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%16 != 0 {
		return
	}

	log.Printf("[pm] tick %d: deciding next priority", r.tick)

	// Use Operate() so the PM can search knowledge, read the board, and
	// create the milestone directly — all as agent actions on the graph.
	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if canOperate {
		r.runPMOperate(ctx, op)
	} else {
		r.runPMReason(ctx)
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

// runPMOperate uses Operate() — the PM searches knowledge, reads the board,
// and creates milestones directly via the API.
func (r *Runner) runPMOperate(ctx context.Context, op decision.IOperator) {
	apiKey := os.Getenv("LOVYOU_API_KEY")
	repoNames := ""
	for name := range r.cfg.RepoMap {
		repoNames += name + ", "
	}

	instruction := fmt.Sprintf(`You are the PM for the lovyou.ai hive. Decide what to build next.

## Your Tools
- Use knowledge.search to find existing work, avoid rediscovering what's done
- Use knowledge.get to read the backlog, design docs, and prior reflections
- Use Bash to check the board (curl), read recent git log, and create tasks

## Available Repos: %s

## Steps
1. Search knowledge for "backlog" — read it to understand priorities
2. Search knowledge for "Director mandate" — check if there are binding instructions
3. Check the board for open tasks: curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "https://lovyou.ai/app/%s/board"
4. Check recent git commits: git log --oneline -20
5. Read diagnostics for pipeline failures: cat loop/diagnostics.jsonl | tail -20
6. Decide the MOST IMPORTANT thing to build next
7. Create a milestone on the board:
   curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "https://lovyou.ai/app/%s/op" -d '{"op":"intend","kind":"task","title":"<MILESTONE TITLE>","description":"<FULL DIRECTIVE WITH TARGET REPO AND TASKS>","priority":"high"}'
8. Update state.md with the directive (for target repo resolution):
   Write the directive to loop/state.md in the "What the Scout Should Focus On Next" section.
   MUST include "**Target repo:** <name>" line.

## Rules
- Search before proposing — the hive's worst failure mode is rediscovering what it already knows
- Check the Director mandate in the backlog — it may override your preference
- One direction per milestone, 3-5 specific tasks
- ALWAYS include Target repo in the directive
`, repoNames, apiKey, r.cfg.SpaceSlug, apiKey, r.cfg.SpaceSlug)

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.HiveDir,
		Instruction: instruction,
	})
	if err != nil {
		log.Printf("[pm] Operate error: %v", err)
		return
	}

	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[pm] decision made (cost=$%.4f)", result.Usage.CostUSD)
}

// runPMReason is the legacy fallback — gathers context manually, calls Reason().
func (r *Runner) runPMReason(ctx context.Context) {
	backlog := r.readBacklog()
	recentCommits := r.recentGitLog()
	boardSummary := r.boardSummary()
	completedWork := r.completedTasksSummary()
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)
	currentDirective := r.readScoutSection()
	recentFailures := readRecentDiagnostics(r.cfg.HiveDir)

	prompt := buildPMPrompt(sharedCtx, backlog, recentCommits, boardSummary, completedWork, currentDirective, recentFailures, r.cfg.RepoMap)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[pm] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[pm] decision made (cost=$%.4f)", resp.Usage().CostUSD)

	directive := parsePMDirective(resp.Content())
	if directive == "" {
		log.Printf("[pm] no directive found in response")
		return
	}

	log.Printf("[pm] new directive: %s", truncateLog(directive, 100))

	title := extractDirectiveTitle(directive)
	if title != "" {
		if _, err := r.cfg.APIClient.CreateTask(r.cfg.SpaceSlug, title, directive, "high"); err != nil {
			log.Printf("[pm] create milestone error: %v", err)
		} else {
			log.Printf("[pm] milestone created on board: %s", title)
		}
	}

	if err := r.updateScoutDirective(directive); err != nil {
		log.Printf("[pm] update state.md error: %v", err)
	}
}

// completedTasksSummary returns recently completed tasks so the PM knows what's done.
func (r *Runner) completedTasksSummary() string {
	tasks, err := r.cfg.APIClient.GetTasks(r.cfg.SpaceSlug, "")
	if err != nil {
		return "(completed tasks unavailable)"
	}

	var completed []string
	for _, t := range tasks {
		if t.Kind != "task" {
			continue
		}
		if t.State == "done" {
			completed = append(completed, fmt.Sprintf("- [DONE] %s", t.Title))
			if len(completed) >= 30 {
				break
			}
		}
	}

	if len(completed) == 0 {
		return "No recently completed tasks."
	}
	return fmt.Sprintf("Recently completed (%d tasks):\n%s", len(completed), strings.Join(completed, "\n"))
}

func (r *Runner) readBacklog() string {
	if r.cfg.HiveDir == "" {
		return "(backlog not available)"
	}
	path := filepath.Join(r.cfg.HiveDir, "loop", "backlog.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "(could not read backlog.md)"
	}
	s := string(data)
	if len(s) > 6000 {
		s = s[:6000] + "\n... (truncated)"
	}
	return s
}

func (r *Runner) updateScoutDirective(directive string) error {
	if r.cfg.HiveDir == "" {
		return fmt.Errorf("HiveDir not set")
	}
	path := filepath.Join(r.cfg.HiveDir, "loop", "state.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	s := string(data)
	marker := "## What the Scout Should Focus On Next"

	// Remove ALL existing scout sections (there may be duplicates from prior runs).
	for {
		idx := strings.Index(s, marker)
		if idx < 0 {
			break
		}
		rest := s[idx+len(marker):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			s = s[:idx] + rest[endIdx+1:] // +1 to consume the \n
		} else {
			s = strings.TrimRight(s[:idx], "\n")
		}
	}

	// Append the single new scout section.
	newSection := fmt.Sprintf("\n\n%s\n\n%s\n", marker, directive)
	s = strings.TrimRight(s, "\n") + newSection

	return os.WriteFile(path, []byte(s), 0644)
}

func buildPMPrompt(sharedCtx, backlog, recentCommits, board, completedWork, currentDirective, recentFailures string, repoMap map[string]string) string {
	repoSection := ""
	if len(repoMap) > 0 {
		var lines []string
		for name, path := range repoMap {
			lines = append(lines, fmt.Sprintf("- **%s** → %s", name, path))
		}
		repoSection = fmt.Sprintf(`## Available Repos

The pipeline can target any of these repos. Your directive MUST include a "**Target repo:** <name>" line.

%s

`, strings.Join(lines, "\n"))
	}

	return fmt.Sprintf(`You are the PM. You decide WHAT the hive should build next.

## Institutional Knowledge
%s

## Backlog (ideas waiting to become work)
%s

## Recent Commits (what was recently built)
%s

## Current Board (open tasks)
%s

## COMPLETED WORK (what is ALREADY DONE — do NOT recreate these)
%s

## Recent Pipeline Failures
Phases that failed recently. Avoid directing work that depends on broken infrastructure until these are resolved.

%s

## Current Scout Directive (DO NOT REPEAT)
This is what the Scout is already working on or was last directed to do.
Do NOT issue a directive that repeats or overlaps with this work:

%s

%s## Your Task

The Scout has exhausted its current directive. It needs a new one.

1. Read the backlog. Read what was recently built. Identify what's MOST IMPORTANT to build next.
2. Consider: what would make the biggest difference for real users?
3. Don't repeat what was already built (check recent commits).
4. Don't repeat the current Scout directive above.
5. Pick ONE direction with 3-5 specific implementable tasks.
6. Write a directive the Scout can follow — specific files, specific changes.
7. ALWAYS include "**Target repo:** <name>" — the pipeline needs to know which repo to operate on.

## Output Format

Write a directive that will replace the Scout's current section in state.md:

DIRECTIVE_START
[Your directive here — include specific tasks the Scout should create,
which repo they target, and why this is the priority now.
MUST include a "**Target repo:** <name>" line.]
DIRECTIVE_END`, sharedCtx, backlog, recentCommits, board, completedWork, recentFailures, currentDirective, repoSection)
}

// readRecentDiagnostics reads the last 20 lines of loop/diagnostics.jsonl and
// returns a human-readable failure summary for inclusion in the PM prompt.
func readRecentDiagnostics(hiveDir string) string {
	if hiveDir == "" {
		return "(diagnostics not available)"
	}
	path := filepath.Join(hiveDir, "loop", "diagnostics.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return "(no pipeline failures recorded)"
	}
	defer f.Close()

	const maxLines = 20
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	if len(lines) == 0 {
		return "(no pipeline failures recorded)"
	}

	var sb strings.Builder
	for _, line := range lines {
		var e PhaseEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("- [%s] phase=%s outcome=%s cost=$%.4f", e.Timestamp, e.Phase, e.Outcome, e.CostUSD))
		if e.Error != "" {
			sb.WriteString(fmt.Sprintf(" error=%q", e.Error))
		}
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return "(no pipeline failures recorded)"
	}
	return sb.String()
}

func parsePMDirective(content string) string {
	start := strings.Index(content, "DIRECTIVE_START")
	end := strings.Index(content, "DIRECTIVE_END")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	directive := strings.TrimSpace(content[start+len("DIRECTIVE_START") : end])
	return directive
}

// extractDirectiveTitle pulls a short title from the directive for the board milestone.
// Looks for "**Priority: X**" or first bold text, falls back to first line.
func extractDirectiveTitle(directive string) string {
	for _, line := range strings.Split(directive, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip markdown bold markers.
		title := strings.ReplaceAll(line, "**", "")
		title = strings.TrimPrefix(title, "Priority: ")
		title = strings.TrimPrefix(title, "Priority — ")
		// Truncate to reasonable task title length.
		if len(title) > 100 {
			title = title[:100]
		}
		return title
	}
	return ""
}

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
