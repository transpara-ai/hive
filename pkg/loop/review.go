package loop

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/work"
)

// ────────────────────────────────────────────────────────────────────
// Types
// ────────────────────────────────────────────────────────────────────

// ReviewCommand represents the parsed /review command from Reviewer LLM output.
type ReviewCommand struct {
	TaskID     string   `json:"task_id"`
	Verdict    string   `json:"verdict"` // "approve", "request_changes", "reject"
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

	// replayHead is the chain watermark: the ID of the newest event already
	// folded into this projection from the durable store. Zero means the
	// boot-time replay has not happened yet (or the store was empty at boot).
	// catchUpReviewProjection advances it; nothing else writes it.
	replayHead types.EventID
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

// InitReviewerFromRecovery seeds reviewer state from chain replay.
func (s *reviewerState) InitReviewerFromRecovery(state *checkpoint.ReviewerRecoveredState) {
	if state == nil {
		return
	}
	for taskID, count := range state.ReviewCounts {
		s.reviewHistory[taskID] = &taskReviewRecord{
			reviewCount: count,
		}
	}
}

// update advances the reviewer's iteration counter. Called once per loop
// iteration with the pending events from the bus.
//
// Deliberately NO projection mutation happens here: an event can be visible
// to the chain replay AND still sit in the bus queue (Graph.Record appends to
// the store before the bus publishes asynchronously), so folding bus payloads
// would double-count across the replay/live boundary and inflate the
// escalation cap. The chain watermark walk (catchUpReviewProjection) is the
// projection's single mutation source; bus deliveries wake the loop and
// decorate the observation text, nothing more.
func (s *reviewerState) update(events []event.Event) {
	s.iteration++
	_ = events
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
			continue
		}
		// A record with no verdict is the recovery shape: InitReviewerFromRecovery
		// seeds review COUNTS only, never verdicts. An unknown last verdict with a
		// known completion fails toward review — bounded by shouldEscalate's cycle
		// cap — instead of silently dropping the task out of the review→fix loop.
		if rec.lastVerdict == "" {
			pending = append(pending, taskID)
		}
	}
	return pending
}

// recordReview records a review verdict for a task. A settling verdict
// (approve/reject) also evicts the task's completion from the projection:
// settled work cannot pend (findPendingReviews) and must not stay resident
// forever — an always-on reviewer over a long-lived store is a parked gate,
// not a full-history in-memory projection. A re-completion re-enters via the
// chain fold.
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

	if verdict == "approve" || verdict == "reject" {
		delete(s.completedTasks, taskID)
	}
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
// Governance re-check gate (slice-1 finding F8)
// ────────────────────────────────────────────────────────────────────

// hasReviewableWork reports whether the Reviewer has completed-but-unreviewed
// work to pick up — the governance analog of hasAssignableWork, and the gate
// for the keepalive re-check timer (slice-1 finding F8).
//
// The reviewer projection (completedTasks + reviewHistory) is maintained from
// the chain alone: a one-time replay of the prefix up to the boot-time head,
// then an incremental Since(watermark) catch-up on every evaluation. This
// makes the gate writer-agnostic (round-3 finding B-1: Work's HTTP server and
// CLI complete tasks through the shared store from separate binaries — no
// in-process bus delivery ever reaches this loop for those writes), bounded
// per tick by the actual event delta (round-2 finding M-1), and immune to
// the replay/bus double-count boundary by construction: the chain walk is the
// projection's ONLY mutation source, each event folds exactly once in chain
// order, and bus payloads merely wake the loop and decorate observations
// (round-3 finding B-2).
//
// Failure direction: any store error leaves the gate returning false (fail
// toward parked, mirroring hasAssignableWork's error handling) with the
// watermark unadvanced — the catch-up resumes from the same position on the
// next tick, never half-applied.
func (l *Loop) hasReviewableWork() bool {
	if l.reviewerState == nil {
		return false
	}
	if !l.catchUpReviewProjection() {
		return false
	}
	return len(l.reviewerState.findPendingReviews()) > 0
}

// scanPageSize is the per-page size for the chain replay and catch-up walks,
// matching pkg/checkpoint's replay paginator.
const scanPageSize = 1000

// foldChainEvent folds one chain event into the reviewer projection, in chain
// order (callers iterate oldest→newest). The chain walk is the projection's
// only mutation source, so each event folds exactly once: latest-wins for
// completion contents, recordReview for peer verdicts. Own review events are
// skipped — they were recorded directly at emission time (the Run loop's
// recordReview after emitCodeReview) and folding them again would inflate the
// escalation cap.
func (s *reviewerState) foldChainEvent(ev event.Event, selfID types.ActorID) {
	switch c := ev.Content().(type) {
	case work.TaskCompletedContent:
		s.completedTasks[c.TaskID.Value()] = c
	case event.CodeReviewContent:
		if ev.Source() == selfID {
			return
		}
		s.recordReview(c.TaskID, c.Verdict, c.Issues, s.iteration)
	}
}

// replayChainPrefix collects the chain's completion and review events up to
// and including the given head, returned oldest-first for folding. The walk
// pages the store's globally-ordered Recent feed newest-first via cursors
// (round-1 finding B-2 — no bounded window) and IGNORES anything newer than
// head: events appended mid-walk belong to the catch-up's (head, ∞) range, so
// replay and catch-up partition the chain exactly and no event can ever fold
// twice (round-3 finding B-2). Chain position alone decides order — no
// wall-clock timestamp is compared anywhere (round-2 finding B-1).
//
// ok=false means a page read failed or the head was never encountered on the
// walk; callers must treat the replay as not having happened (fail toward
// parked) rather than trust a partial prefix.
func replayChainPrefix(st store.Store, head types.EventID, pageSize int) ([]event.Event, bool) {
	var collected []event.Event
	cursor := types.None[types.Cursor]()
	seenHead := false
	for {
		page, err := st.Recent(pageSize, cursor)
		if err != nil {
			return nil, false
		}
		items := page.Items()
		if len(items) == 0 {
			break
		}
		for _, ev := range items {
			if !seenHead {
				if ev.ID() != head {
					continue // newer than the watermark: the catch-up's range
				}
				seenHead = true
			}
			switch ev.Content().(type) {
			case work.TaskCompletedContent, event.CodeReviewContent:
				collected = append(collected, ev)
			}
		}
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	if !seenHead {
		return nil, false
	}
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected, true
}

// catchUpReviewProjection advances the reviewer projection to the current
// chain head. First call (or empty-store boot): replay the chain prefix up to
// the boot-time head — the same moment-and-meaning as pkg/checkpoint's replay
// — which is what makes a historical completion visible at all: the bus only
// delivers events fired while THIS loop instance is alive, so the restart
// that beat the implementer wakeup race stranded the review→fix loop on
// exactly those events (finding F8). Every later call: fold the Since
// (watermark) delta, which is bounded by the events actually appended since
// the last evaluation and catches every writer — in-process or not.
//
// The projection deliberately mirrors what uninterrupted live delivery would
// have built, so the pending decision lives in ONE place (findPendingReviews)
// and a restarted reviewer behaves identically to one that never restarted:
// an unreviewed completion pends (F8 closed); a re-completion after
// request_changes pends (round-1 B-1); a task whose latest verdict is
// approve/reject does not pend, no matter who reviewed it (AC-2 — settled
// work is never re-reviewed), and leaves the projection entirely.
func (l *Loop) catchUpReviewProjection() bool {
	g := l.agent.Graph()
	if g == nil {
		return false
	}
	st := g.Store()
	if st == nil {
		return false
	}
	s := l.reviewerState
	selfID := l.agent.ID()

	if s.replayHead.IsZero() {
		headOpt, err := st.Head()
		if err != nil {
			return false
		}
		if headOpt.IsNone() {
			// Empty store: trivially up to date. The watermark stays zero so
			// the next evaluation re-checks for a first head.
			return true
		}
		head := headOpt.Unwrap().ID()
		events, ok := replayChainPrefix(st, head, scanPageSize)
		if !ok {
			return false
		}
		for _, ev := range events {
			s.foldChainEvent(ev, selfID)
		}
		s.replayHead = head
		return true
	}

	cur := s.replayHead
	for {
		page, err := st.Since(cur, scanPageSize)
		if err != nil {
			return false
		}
		items := page.Items()
		if len(items) == 0 {
			break
		}
		for _, ev := range items {
			s.foldChainEvent(ev, selfID)
		}
		cur = items[len(items)-1].ID()
		if !page.HasMore() {
			break
		}
	}
	s.replayHead = cur
	return true
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
		commitHash, diffRef := l.resolveCommitForTask(task, ok)

		commit := gitCommand(l.config.RepoPath, "log", "--oneline", "-1", commitHash)
		fileStat := gitCommand(l.config.RepoPath, "diff", diffRef, "--stat")
		diff := gitCommand(l.config.RepoPath, "diff", diffRef)

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

// resolveCommitForTask determines the correct commit hash and diff reference for
// a completed task. Uses three strategies in priority order:
//  0. Use ArtifactRef to fetch the artifact body and extract the verified
//     Operate range, falling back to its commit hash for legacy artifacts.
//  1. Extract commit hash from the task summary text (heuristic).
//  2. Fall back to HEAD~1 (legacy, race-prone with concurrent completions).
//
// Returns (commitHash, diffRef) where commitHash is for `git log -1 <hash>`
// and diffRef is for `git diff <ref>` (e.g., "base..head" or
// "abc1234^..abc1234").
func (l *Loop) resolveCommitForTask(task work.TaskCompletedContent, taskFound bool) (string, string) {
	repo := l.config.RepoPath

	// Strategy 0: use ArtifactRef → fetch artifact body → extract Operate range.
	if taskFound && !task.ArtifactRef.IsZero() {
		body, isArtifact := l.fetchArtifactBody(task.ArtifactRef)
		if isArtifact && body != "" {
			if base, head := extractCommitRange(body, repo); base != "" && head != "" {
				return head, base + ".." + head
			}
			// Legacy artifact bodies recorded only a single commit hash. Keep the old
			// one-commit behavior for those artifacts.
			if hash := extractCommitHash(body, repo); hash != "" {
				return hash, hash + "^.." + hash
			}
		} else if !isArtifact {
			// ArtifactRef points to a waiver — no commit body available.
			fmt.Printf("[%s] note: ArtifactRef is a waiver, falling through to summary heuristic\n", l.agent.Name())
		}
	}

	// Strategy 1: extract hash from summary text.
	if taskFound && task.Summary != "" {
		if hash := extractCommitHash(task.Summary, repo); hash != "" {
			return hash, hash + "^.." + hash
		}
	}

	// Strategy 2: fall back to HEAD~1 (race-prone with concurrent completions).
	fmt.Printf("[%s] warning: no commit hash found for task, falling back to HEAD~1\n", l.agent.Name())
	return "HEAD", "HEAD~1"
}

// fetchArtifactBody reads a work.task.artifact event by ID and returns its Body.
// Returns (body, true) for artifacts, ("", false) for waivers or missing events.
func (l *Loop) fetchArtifactBody(artifactID types.EventID) (string, bool) {
	if l.config.TaskStore == nil {
		return "", false
	}
	return l.config.TaskStore.GetArtifactBody(artifactID)
}

// extractCommitHash scans text for a 7-40 character hex string that looks like
// a git commit hash and verifies it exists in the repo. Returns the full hash
// if found, empty string otherwise. Caps at maxRevParseAttempts to satisfy
// the BOUNDED invariant.
func extractCommitHash(text, repoPath string) string {
	const maxRevParseAttempts = 5
	attempts := 0
	for _, word := range strings.Fields(text) {
		// Strip trailing punctuation (commas, periods, parens).
		cleaned := strings.TrimRight(word, ".,;:()[]")
		if len(cleaned) < 7 || len(cleaned) > 40 {
			continue
		}
		if !isHex(cleaned) {
			continue
		}
		// Verify the hash exists in the repo.
		if attempts++; attempts > maxRevParseAttempts {
			break
		}
		full := gitCommand(repoPath, "rev-parse", "--verify", cleaned+"^{commit}")
		if full != "" {
			return full
		}
	}
	return ""
}

// extractCommitRange scans a structured Operate artifact body for base/head
// commit lines and verifies both commits exist in the repo. The resulting range
// must also be a forward ancestry path so reviewer diffs align with the
// commit-verification gate.
func extractCommitRange(text, repoPath string) (string, string) {
	var baseToken, headToken string
	for _, line := range strings.Split(text, "\n") {
		// Parse only the machine-written header block. buildOperateArtifactBody
		// emits the commit:/base:/head:/range: lines, then a blank line, then the
		// `git diff --stat` section — whose filenames are agent-controlled. Stop at
		// the blank line so a file named like a header key (e.g. "head:<hex>")
		// cannot override the verified range and force a single-commit fallback that
		// re-hides an earlier commit from the reviewer.
		if strings.TrimSpace(line) == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "base":
			baseToken = strings.TrimSpace(value)
		case "head":
			headToken = strings.TrimSpace(value)
		}
	}
	base := verifyCommitToken(baseToken, repoPath)
	head := verifyCommitToken(headToken, repoPath)
	if base == "" || head == "" {
		return "", ""
	}
	if base == head {
		return "", ""
	}
	if yes, ok := isAncestor(repoPath, base, head); !ok || !yes {
		return "", ""
	}
	return base, head
}

func verifyCommitToken(token, repoPath string) string {
	token = strings.TrimRight(strings.TrimSpace(token), ".,;:()[]")
	if len(token) < 7 || len(token) > 40 || !isHex(token) {
		return ""
	}
	return gitCommand(repoPath, "rev-parse", "--verify", token+"^{commit}")
}

// isHex returns true if s contains only hexadecimal characters.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

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

// gitTry runs a git command and reports whether it succeeded. Unlike
// gitCommand, it distinguishes a genuinely empty result (ok=true — e.g. a clean
// `status --porcelain`) from a git failure (ok=false — e.g. the directory is not
// a checkout), so callers that must verify repo state can fail closed on the
// latter instead of mistaking an error for "clean / no commit".
func gitTry(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// isAncestor reports whether ancestor is an ancestor of descendant in dir's repo
// — i.e. descendant is a true forward advance from ancestor. yes is the answer;
// ok is false when ancestry could not be determined (invalid hash, git failure),
// so callers can fail closed rather than guess. Uses `git merge-base
// --is-ancestor`, which exits 0 for ancestor and 1 for not-ancestor.
func isAncestor(dir, ancestor, descendant string) (yes bool, ok bool) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return true, true
	}
	if ee, isExit := err.(*exec.ExitError); isExit && ee.ExitCode() == 1 {
		return false, true // definitively not an ancestor
	}
	return false, false // indeterminate — bad revision or git failure
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
