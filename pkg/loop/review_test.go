package loop

import (
	"testing"
)

// ════════════════════════════════════════════════════════════════════════
// parseReviewCommand
// ════════════════════════════════════════════════════════════════════════

func TestParseReviewCommand_Valid(t *testing.T) {
	response := `I've reviewed the implementation.
/review {"task_id":"task-abc-123","verdict":"approve","summary":"Clean implementation with good error handling.","issues":[],"confidence":0.9}
/signal {"signal": "IDLE"}`

	cmd := parseReviewCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil ReviewCommand")
	}
	if cmd.TaskID != "task-abc-123" {
		t.Errorf("TaskID = %q, want %q", cmd.TaskID, "task-abc-123")
	}
	if cmd.Verdict != "approve" {
		t.Errorf("Verdict = %q, want %q", cmd.Verdict, "approve")
	}
	if cmd.Summary != "Clean implementation with good error handling." {
		t.Errorf("Summary = %q, want %q", cmd.Summary, "Clean implementation with good error handling.")
	}
	if len(cmd.Issues) != 0 {
		t.Errorf("Issues = %v, want empty", cmd.Issues)
	}
	if cmd.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9", cmd.Confidence)
	}
}

func TestParseReviewCommand_NoCommand(t *testing.T) {
	response := `The code looks fine overall.
/signal {"signal": "IDLE"}`

	cmd := parseReviewCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseReviewCommand_MalformedJSON(t *testing.T) {
	response := `/review {not valid json`

	cmd := parseReviewCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

func TestParseReviewCommand_MultipleLines(t *testing.T) {
	response := `Analyzing the code diff...
Found unchecked error return.
Missing test coverage for edge case.
/review {"task_id":"task-xyz","verdict":"request_changes","summary":"Two issues found.","issues":["Unchecked error on line 47","Missing edge case test"],"confidence":0.85}
Will wait for fixes.
/signal {"signal": "IDLE"}`

	cmd := parseReviewCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil ReviewCommand")
	}
	if cmd.TaskID != "task-xyz" {
		t.Errorf("TaskID = %q, want %q", cmd.TaskID, "task-xyz")
	}
	if cmd.Verdict != "request_changes" {
		t.Errorf("Verdict = %q, want %q", cmd.Verdict, "request_changes")
	}
	if len(cmd.Issues) != 2 {
		t.Errorf("Issues count = %d, want 2", len(cmd.Issues))
	}
}

// ════════════════════════════════════════════════════════════════════════
// validateReviewCommand
// ════════════════════════════════════════════════════════════════════════

func TestValidateReviewCommand_Valid(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "approve",
		Summary:    "Clean implementation.",
		Issues:     []string{},
		Confidence: 0.9,
	}
	err := validateReviewCommand(cmd, 15)
	if err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateReviewCommand_InvalidVerdict(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "maybe",
		Summary:    "Not sure.",
		Issues:     []string{},
		Confidence: 0.8,
	}
	err := validateReviewCommand(cmd, 15)
	if err == nil {
		t.Error("expected error for invalid verdict, got nil")
	}
}

func TestValidateReviewCommand_EmptyTaskID(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "",
		Verdict:    "approve",
		Summary:    "Looks good.",
		Issues:     []string{},
		Confidence: 0.9,
	}
	err := validateReviewCommand(cmd, 15)
	if err == nil {
		t.Error("expected error for empty task_id, got nil")
	}
}

func TestValidateReviewCommand_ConfidenceOutOfRange(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
	}{
		{"negative", -0.1},
		{"above_1", 1.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &ReviewCommand{
				TaskID:     "task-123",
				Verdict:    "approve",
				Summary:    "Ok.",
				Issues:     []string{},
				Confidence: tt.confidence,
			}
			err := validateReviewCommand(cmd, 15)
			if err == nil {
				t.Errorf("expected error for confidence %.2f, got nil", tt.confidence)
			}
		})
	}
}

func TestValidateReviewCommand_EmptySummary(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "approve",
		Summary:    "",
		Issues:     []string{},
		Confidence: 0.9,
	}
	err := validateReviewCommand(cmd, 15)
	if err == nil {
		t.Error("expected error for empty summary, got nil")
	}
}

func TestValidateReviewCommand_StabilizationWindow(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "approve",
		Summary:    "Looks good.",
		Issues:     []string{},
		Confidence: 0.9,
	}
	for _, iter := range []int{0, 3, 9} {
		err := validateReviewCommand(cmd, iter)
		if err == nil {
			t.Errorf("iteration %d: expected stabilization error, got nil", iter)
		}
	}
}

func TestValidateReviewCommand_ConfidenceTooLow(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "approve",
		Summary:    "Not confident.",
		Issues:     []string{},
		Confidence: 0.3,
	}
	err := validateReviewCommand(cmd, 15)
	if err == nil {
		t.Error("expected error for low confidence, got nil")
	}
}

func TestValidateReviewCommand_NilIssues(t *testing.T) {
	cmd := &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "approve",
		Summary:    "Ok.",
		Issues:     nil,
		Confidence: 0.9,
	}
	err := validateReviewCommand(cmd, 15)
	if err == nil {
		t.Error("expected error for nil issues, got nil")
	}
}

// ════════════════════════════════════════════════════════════════════════
// reviewerState
// ════════════════════════════════════════════════════════════════════════

func TestReviewerState_TrackReview(t *testing.T) {
	s := newReviewerState()

	s.recordReview("task-1", "approve", []string{}, 10)

	rec, ok := s.reviewHistory["task-1"]
	if !ok {
		t.Fatal("expected review record for task-1")
	}
	if rec.reviewCount != 1 {
		t.Errorf("reviewCount = %d, want 1", rec.reviewCount)
	}
	if rec.lastVerdict != "approve" {
		t.Errorf("lastVerdict = %q, want %q", rec.lastVerdict, "approve")
	}
	if len(rec.iterations) != 1 || rec.iterations[0] != 10 {
		t.Errorf("iterations = %v, want [10]", rec.iterations)
	}
}

func TestReviewerState_ReviewCount(t *testing.T) {
	s := newReviewerState()

	if s.getReviewCount("task-1") != 0 {
		t.Error("expected 0 for unknown task")
	}

	s.recordReview("task-1", "request_changes", []string{"issue 1"}, 10)
	s.recordReview("task-1", "approve", []string{}, 15)

	if s.getReviewCount("task-1") != 2 {
		t.Errorf("reviewCount = %d, want 2", s.getReviewCount("task-1"))
	}

	// Verify last verdict updated.
	rec := s.reviewHistory["task-1"]
	if rec.lastVerdict != "approve" {
		t.Errorf("lastVerdict = %q, want %q", rec.lastVerdict, "approve")
	}
}

func TestReviewerState_CycleLimit(t *testing.T) {
	s := newReviewerState()

	// 2 reviews: not yet escalation-worthy.
	s.recordReview("task-1", "request_changes", []string{"fix A"}, 10)
	s.recordReview("task-1", "request_changes", []string{"fix B"}, 15)
	if s.shouldEscalate("task-1") {
		t.Error("should not escalate after 2 reviews")
	}

	// 3rd review: escalation threshold reached.
	s.recordReview("task-1", "request_changes", []string{"fix C"}, 20)
	if !s.shouldEscalate("task-1") {
		t.Error("should escalate after 3 reviews")
	}

	// Different task: independent count.
	if s.shouldEscalate("task-2") {
		t.Error("task-2 should not escalate (never reviewed)")
	}
}
