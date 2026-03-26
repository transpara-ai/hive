package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// runPM reads the backlog, recent work, and board state, then updates
// the Scout's directive in state.md with the next strategic priority.
// This prevents the Scout from spinning when specs are exhausted.
func (r *Runner) runPM(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%16 != 0 {
		return
	}

	log.Printf("[pm] tick %d: deciding next priority", r.tick)

	// Gather strategic context.
	backlog := r.readBacklog()
	recentCommits := r.recentGitLog()
	boardSummary := r.boardSummary()
	completedWork := r.completedTasksSummary()
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)

	prompt := buildPMPrompt(sharedCtx, backlog, recentCommits, boardSummary, completedWork)

	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[pm] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	log.Printf("[pm] decision made (cost=$%.4f)", resp.Usage().CostUSD)

	// Parse the directive from the response.
	directive := parsePMDirective(resp.Content())
	if directive == "" {
		log.Printf("[pm] no directive found in response")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	log.Printf("[pm] new directive: %s", truncateLog(directive, 100))

	// Update state.md with the new directive.
	if err := r.updateScoutDirective(directive); err != nil {
		log.Printf("[pm] update state.md error: %v", err)
	} else {
		log.Printf("[pm] state.md updated with new Scout directive")
	}

	if r.cfg.OneShot {
		r.done = true
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
	idx := strings.Index(s, marker)
	if idx < 0 {
		return fmt.Errorf("scout section not found in state.md")
	}

	// Find end of scout section.
	rest := s[idx+len(marker):]
	endIdx := strings.Index(rest, "\n## ")
	var after string
	if endIdx >= 0 {
		after = rest[endIdx:]
	}

	// Write new scout section.
	newSection := fmt.Sprintf("%s\n\n%s\n", marker, directive)
	newContent := s[:idx] + newSection + after

	return os.WriteFile(path, []byte(newContent), 0644)
}

func buildPMPrompt(sharedCtx, backlog, recentCommits, board, completedWork string) string {
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

## Your Task

The Scout has exhausted its current directive. It needs a new one.

1. Read the backlog. Read what was recently built. Identify what's MOST IMPORTANT to build next.
2. Consider: what would make the biggest difference for real users?
3. Don't repeat what was already built (check recent commits).
4. Pick ONE direction with 3-5 specific implementable tasks.
5. Write a directive the Scout can follow — specific files, specific changes.

## Output Format

Write a directive that will replace the Scout's current section in state.md:

DIRECTIVE_START
[Your directive here — include specific tasks the Scout should create,
which repo they target, and why this is the priority now.
Write it as if you're updating state.md directly.]
DIRECTIVE_END`, sharedCtx, backlog, recentCommits, board, completedWork)
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

func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
