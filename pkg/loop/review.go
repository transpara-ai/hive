package loop

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/work"
)

// ────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────

// ReviewCommand represents the parsed /review command from Reviewer LLM output.
type ReviewCommand struct {
	TaskID     string   `json:"task_id"`
	Verdict    string   `json:"verdict"`    // "approve", "request_changes", "reject"
	Summary    string   `json:"summary"`
	Issues     []string `json:"issues"`
	Confidence float64  `json:"confidence"` // 0.0–1.0
}

// validVerdicts is the set of accepted verdict values.
var validVerdicts = map[string]bool{
	"approve":         true,
	"request_changes": true,
	"reject":          true,
}

// reviewerState tracks cross-iteration state for the Reviewer agent.
// Created in New() when role == "reviewer". Only accessed from the
// Run() goroutine — no mutex needed.
type reviewerState struct {
	iteration      int
	reviewHistory  map[string]*taskReviewRecord
	completedTasks map[string]work.TaskCompletedContent // keyed by TaskID string
}

// taskReviewRecord tracks review history for a single task.
type taskReviewRecord struct {
	taskID      string
	reviewCount int
	lastVerdict string
	lastIssues  []string
	iterations  []int
}

// newReviewerState initialises a zeroed reviewerState.
func newReviewerState() *reviewerState {
	return &reviewerState{
		reviewHistory:  make(map[string]*taskReviewRecord),
		completedTasks: make(map[string]work.TaskCompletedContent),
	}
}

// update processes the current batch of pending events and tracks completed tasks.
// Called once per loop iteration with the pending events from the bus.
func (s *reviewerState) update(events []event.Event) {
	s.iteration++

	for _, ev := range events {
		if c, ok := ev.Content().(work.TaskCompletedContent); ok {
			taskID := c.TaskID.Value()
			s.completedTasks[taskID] = c
		}
		// Track our own reviews to exclude already-reviewed tasks.
		if c, ok := ev.Content().(event.CodeReviewContent); ok {
			s.recordReview(c.TaskID, c.Verdict, c.Issues, s.iteration)
		}
	}
}

// findPendingReviews returns task IDs that have been completed but not yet
// reviewed (or were reviewed with request_changes and re-completed).
func (s *reviewerState) findPendingReviews() []string {
	var pending []string
	for taskID := range s.completedTasks {
		rec, reviewed := s.reviewHistory[taskID]
		if !reviewed {
			pending = append(pending, taskID)
			continue
		}
		// Re-review if last verdict was request_changes (implementer may have fixed).
		if rec.lastVerdict == "request_changes" {
			pending = append(pending, taskID)
		}
	}
	return pending
}

// recordReview records a review verdict for a task.
func (s *reviewerState) recordReview(taskID, verdict string, issues []string, iteration int) {
	rec, ok := s.reviewHistory[taskID]
	if !ok {
		rec = &taskReviewRecord{taskID: taskID}
		s.reviewHistory[taskID] = rec
	}
	rec.reviewCount++
	rec.lastVerdict = verdict
	rec.lastIssues = issues
	rec.iterations = append(rec.iterations, iteration)
}

// getReviewCount returns the number of times a task has been reviewed.
func (s *reviewerState) getReviewCount(taskID string) int {
	rec, ok := s.reviewHistory[taskID]
	if !ok {
		return 0
	}
	return rec.reviewCount
}

// shouldEscalate returns true if a task has been reviewed 3+ times,
// indicating a review cycle that needs human or CTO intervention.
func (s *reviewerState) shouldEscalate(taskID string) bool {
	return s.getReviewCount(taskID) >= 3
}

// ────────────────────────────────────────────────────────────────────
// Parsing
// ────────────────────────────────────────────────────────────────────

// parseReviewCommand extracts the /review JSON payload from LLM output.
// Returns nil if no /review command is found or the JSON is malformed.
// Follows the same line-scanning pattern as parseSpawnCommand.
func parseReviewCommand(response string) *ReviewCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/review ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/review ")
		var cmd ReviewCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────
// Validation
// ────────────────────────────────────────────────────────────────────

// validateReviewCommand checks all constraints before emitting a review event.
// Returns a descriptive error if any constraint is violated, nil if valid.
func validateReviewCommand(cmd *ReviewCommand, iteration int) error {
	// 1. Stabilization window.
	if iteration < 10 {
		return fmt.Errorf("stabilization window active (iteration %d < 10): observe first", iteration)
	}

	// 2. TaskID non-empty.
	if cmd.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}

	// 3. Valid verdict.
	if !validVerdicts[cmd.Verdict] {
		return fmt.Errorf("invalid verdict %q: must be approve, request_changes, or reject", cmd.Verdict)
	}

	// 4. Summary non-empty.
	if cmd.Summary == "" {
		return fmt.Errorf("summary is required")
	}

	// 5. Issues must be non-nil.
	if cmd.Issues == nil {
		return fmt.Errorf("issues must be non-nil (use empty array [] for approve)")
	}

	// 6. Confidence in range.
	if cmd.Confidence < 0.0 || cmd.Confidence > 1.0 {
		return fmt.Errorf("confidence %.2f out of range [0.0, 1.0]", cmd.Confidence)
	}

	// 7. Confidence too low to issue a verdict.
	if cmd.Confidence < 0.5 {
		return fmt.Errorf("confidence %.2f too low: escalate instead of issuing a verdict", cmd.Confidence)
	}

	return nil
}

// ────────────────────────────────────────────────────────────────────
// Emission
// ────────────────────────────────────────────────────────────────────

// emitCodeReview constructs a CodeReviewContent from the validated
// ReviewCommand and records it on the event chain via agent.EmitCodeReview.
func (l *Loop) emitCodeReview(cmd *ReviewCommand) error {
	content := event.CodeReviewContent{
		TaskID:     cmd.TaskID,
		Verdict:    cmd.Verdict,
		Summary:    cmd.Summary,
		Issues:     cmd.Issues,
		Confidence: cmd.Confidence,
	}
	if err := l.agent.EmitCodeReview(content); err != nil {
		return fmt.Errorf("emit code.review.submitted: %w", err)
	}
	fmt.Printf("[%s] emitted code.review.submitted (task=%s verdict=%s confidence=%.2f)\n",
		l.agent.Name(), cmd.TaskID, cmd.Verdict, cmd.Confidence)
	return nil
}

// ────────────────────────────────────────────────────────────────────
// Observation Enrichment
// ────────────────────────────────────────────────────────────────────

// enrichReviewObservation appends pre-computed code review context to the
// observation string for the Reviewer. Only activates when l.reviewerState
// is non-nil (i.e., when role == "reviewer").
func (l *Loop) enrichReviewObservation(obs string) string {
	if l.reviewerState == nil {
		return obs
	}

	pending := l.reviewerState.findPendingReviews()
	if len(pending) == 0 {
		return obs + "\n\n=== CODE REVIEW CONTEXT ===\nNo tasks pending review.\n==="
	}

	// One task per iteration — focus produces better reviews.
	taskID := pending[0]
	task, ok := l.reviewerState.completedTasks[taskID]

	var sb strings.Builder
	sb.WriteString("\n\n=== CODE REVIEW CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("PENDING REVIEWS: %d\n\n", len(pending)))

	// Task metadata.
	sb.WriteString("TASK UNDER REVIEW:\n")
	sb.WriteString(fmt.Sprintf("  id: %s\n", taskID))
	if ok {
		sb.WriteString(fmt.Sprintf("  completed_by: %s\n", task.CompletedBy.Value()))
		if task.Summary != "" {
			sb.WriteString(fmt.Sprintf("  summary: %s\n", task.Summary))
		}
	}
	sb.WriteString("\n")

	// Git context — only if RepoPath is configured.
	if l.config.RepoPath != "" {
		commit := gitCommand(l.config.RepoPath, "log", "--oneline", "-1")
		fileStat := gitCommand(l.config.RepoPath, "diff", "HEAD~1", "--stat")
		diff := gitCommand(l.config.RepoPath, "diff", "HEAD~1")

		sb.WriteString("RECENT COMMIT:\n")
		if commit != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", commit))
		} else {
			sb.WriteString("  (unavailable)\n")
		}
		sb.WriteString("\n")

		sb.WriteString("CHANGED FILES:\n")
		if fileStat != "" {
			for _, line := range strings.Split(fileStat, "\n") {
				if line != "" {
					sb.WriteString(fmt.Sprintf("  %s\n", line))
				}
			}
		} else {
			sb.WriteString("  (unavailable)\n")
		}
		sb.WriteString("\n")

		truncated := truncateDiff(diff, 300)
		sb.WriteString("DIFF:\n")
		if truncated != "" {
			sb.WriteString(truncated)
			sb.WriteString("\n")
		} else {
			sb.WriteString("  (unavailable)\n")
		}
		sb.WriteString("\n")
	}

	// Previous review history for this task.
	rec := l.reviewerState.reviewHistory[taskID]
	if rec != nil && rec.reviewCount > 0 {
		sb.WriteString(fmt.Sprintf("PREVIOUS REVIEWS FOR THIS TASK: %d (last verdict: %s)\n", rec.reviewCount, rec.lastVerdict))
		if len(rec.lastIssues) > 0 {
			sb.WriteString("PREVIOUS ISSUES:\n")
			for _, issue := range rec.lastIssues {
				sb.WriteString(fmt.Sprintf("  - %s\n", issue))
			}
		}
	} else {
		sb.WriteString("PREVIOUS REVIEWS FOR THIS TASK: none\n")
	}

	sb.WriteString("===\n")
	return obs + sb.String()
}

// ────────────────────────────────────────────────────────────────────
// Git Helpers
// ────────────────────────────────────────────────────────────────────

// gitCommand runs a git command in the given directory and returns stdout.
// Returns empty string on any error (best-effort).
func gitCommand(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// truncateDiff applies the three-tier truncation strategy from the design spec.
//   - ≤ maxLines: include full diff
//   - maxLines+1 to 1000: first 200 lines + last 50 lines + omission note
//   - > 1000: "Diff too large for inline review."
func truncateDiff(diff string, maxLines int) string {
	if diff == "" {
		return ""
	}
	lines := strings.Split(diff, "\n")
	total := len(lines)

	if total <= maxLines {
		return diff
	}

	if total <= 1000 {
		head := strings.Join(lines[:200], "\n")
		tail := strings.Join(lines[total-50:], "\n")
		return head + fmt.Sprintf("\n\n... %d lines omitted ...\n\n", total-250) + tail
	}

	return fmt.Sprintf("Diff too large for inline review (%d lines). See CHANGED FILES above.", total)
}
