package checkpoint_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/checkpoint"
)

// TestFormatParseCheckpoint_RoundTrip formats a checkpoint with all fields populated,
// parses it back, and verifies every extracted field matches the original values.
func TestFormatParseCheckpoint_RoundTrip(t *testing.T) {
	snap := checkpoint.LoopSnapshot{
		Role:          "implementer",
		Iteration:     7,
		MaxIterations: 20,
		TokensUsed:    4200,
		CostUSD:       0.63,
		Signal:        "ACTIVE",
		CurrentTaskID: "task-42",
		CurrentTask:   "add error handling to API",
		TaskStatus:    "in-progress",
	}

	intent := "write unit tests for the new endpoint"
	next := "run go test ./..."
	ctx := "PR #12, branch feat/api-errors"

	text := checkpoint.FormatCheckpoint(checkpoint.TaskAssigned, snap, intent, next, ctx)

	// Verify the raw text contains expected markers
	if !strings.Contains(text, "[CHECKPOINT]") {
		t.Fatalf("missing [CHECKPOINT] header in:\n%s", text)
	}
	if !strings.Contains(text, "implementer agent") {
		t.Fatalf("missing role in header:\n%s", text)
	}
	if !strings.Contains(text, "iteration ~7") {
		t.Fatalf("missing iteration in header:\n%s", text)
	}
	if !strings.Contains(text, "STATUS: ACTIVE") {
		t.Fatalf("missing STATUS line:\n%s", text)
	}
	if !strings.Contains(text, "TASK: task-42 -- add error handling to API -- in-progress") {
		t.Fatalf("missing TASK line:\n%s", text)
	}
	if !strings.Contains(text, "INTENT: "+intent) {
		t.Fatalf("missing INTENT line:\n%s", text)
	}
	if !strings.Contains(text, "NEXT: "+next) {
		t.Fatalf("missing NEXT line:\n%s", text)
	}
	if !strings.Contains(text, "CONTEXT: "+ctx) {
		t.Fatalf("missing CONTEXT line:\n%s", text)
	}

	// Parse back
	parsed, err := checkpoint.ParseCheckpoint(text)
	if err != nil {
		t.Fatalf("ParseCheckpoint returned error: %v", err)
	}

	if parsed.Role != "implementer" {
		t.Errorf("Role: got %q, want %q", parsed.Role, "implementer")
	}
	if parsed.ApproxIteration != 7 {
		t.Errorf("ApproxIteration: got %d, want 7", parsed.ApproxIteration)
	}
	if parsed.Timestamp.IsZero() {
		t.Error("Timestamp: got zero, want a valid time")
	}
	if parsed.Status != "ACTIVE" {
		t.Errorf("Status: got %q, want %q", parsed.Status, "ACTIVE")
	}
	if !strings.Contains(parsed.Budget, "7/20 iterations") {
		t.Errorf("Budget: got %q, expected to contain %q", parsed.Budget, "7/20 iterations")
	}
	if !strings.Contains(parsed.Budget, "4200 tokens") {
		t.Errorf("Budget: got %q, expected to contain %q", parsed.Budget, "4200 tokens")
	}
	if !strings.Contains(parsed.Budget, "$0.63") {
		t.Errorf("Budget: got %q, expected to contain %q", parsed.Budget, "$0.63")
	}
	if parsed.Task != "task-42 -- add error handling to API -- in-progress" {
		t.Errorf("Task: got %q", parsed.Task)
	}
	if parsed.Intent != intent {
		t.Errorf("Intent: got %q, want %q", parsed.Intent, intent)
	}
	if parsed.Next != next {
		t.Errorf("Next: got %q, want %q", parsed.Next, next)
	}
	if parsed.Context != ctx {
		t.Errorf("Context: got %q, want %q", parsed.Context, ctx)
	}
}

// TestParseCheckpoint_MissingFields verifies that a thought with only a STATUS line
// produces empty strings for all other fields with no error.
func TestParseCheckpoint_MissingFields(t *testing.T) {
	text := "[CHECKPOINT] guardian agent -- iteration ~3, 2026-04-09T12:00:00Z\n\nSTATUS: IDLE\n"

	parsed, err := checkpoint.ParseCheckpoint(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != "guardian" {
		t.Errorf("Role: got %q, want %q", parsed.Role, "guardian")
	}
	if parsed.ApproxIteration != 3 {
		t.Errorf("ApproxIteration: got %d, want 3", parsed.ApproxIteration)
	}
	if parsed.Status != "IDLE" {
		t.Errorf("Status: got %q, want %q", parsed.Status, "IDLE")
	}
	if parsed.Budget != "" {
		t.Errorf("Budget: got %q, want empty", parsed.Budget)
	}
	if parsed.Task != "" {
		t.Errorf("Task: got %q, want empty", parsed.Task)
	}
	if parsed.Intent != "" {
		t.Errorf("Intent: got %q, want empty", parsed.Intent)
	}
	if parsed.Next != "" {
		t.Errorf("Next: got %q, want empty", parsed.Next)
	}
	if parsed.Context != "" {
		t.Errorf("Context: got %q, want empty", parsed.Context)
	}
}

// TestParseCheckpoint_ExtraWhitespace verifies that leading/trailing whitespace
// and extra blank lines do not cause parse failures or corrupt field values.
func TestParseCheckpoint_ExtraWhitespace(t *testing.T) {
	text := `
  [CHECKPOINT] planner agent -- iteration ~1, 2026-04-09T08:30:00Z

  STATUS:   ESCALATE
  BUDGET:   1/10 iterations, 500 tokens, $0.05

  INTENT:   decompose the work graph task

`
	parsed, err := checkpoint.ParseCheckpoint(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != "planner" {
		t.Errorf("Role: got %q, want %q", parsed.Role, "planner")
	}
	if parsed.Status != "ESCALATE" {
		t.Errorf("Status: got %q, want %q", parsed.Status, "ESCALATE")
	}
	if parsed.Budget != "1/10 iterations, 500 tokens, $0.05" {
		t.Errorf("Budget: got %q", parsed.Budget)
	}
	if parsed.Intent != "decompose the work graph task" {
		t.Errorf("Intent: got %q", parsed.Intent)
	}
	if parsed.Task != "" {
		t.Errorf("Task: got %q, want empty", parsed.Task)
	}
}

// TestFormatHiveSummary verifies the hive summary contains expected structural markers.
func TestFormatHiveSummary(t *testing.T) {
	agents := []checkpoint.AgentSummary{
		{Role: "guardian", State: "idle"},
		{Role: "implementer", State: "active"},
	}
	tasks := checkpoint.TaskStats{
		Open:      2,
		Completed: 5,
		Details:   "task-77 in-progress",
	}
	budget := checkpoint.BudgetStats{
		TotalSpend: 1.50,
		DailyCap:   10.00,
	}

	text := checkpoint.FormatHiveSummary(agents, tasks, budget)

	if !strings.Contains(text, "[HIVE SUMMARY]") {
		t.Errorf("missing [HIVE SUMMARY] marker:\n%s", text)
	}
	if !strings.Contains(text, "2 agents active") {
		t.Errorf("missing agent count:\n%s", text)
	}
	if !strings.Contains(text, "guardian(idle)") {
		t.Errorf("missing guardian(idle):\n%s", text)
	}
	if !strings.Contains(text, "implementer(active)") {
		t.Errorf("missing implementer(active):\n%s", text)
	}
	if !strings.Contains(text, "2 open") {
		t.Errorf("missing open task count:\n%s", text)
	}
	if !strings.Contains(text, "task-77 in-progress") {
		t.Errorf("missing task details:\n%s", text)
	}
	if !strings.Contains(text, "5 completed") {
		t.Errorf("missing completed count:\n%s", text)
	}
	if !strings.Contains(text, "$1.50 total spend") {
		t.Errorf("missing total spend:\n%s", text)
	}
	if !strings.Contains(text, "$8.50 remaining daily cap") {
		t.Errorf("missing remaining cap:\n%s", text)
	}
}

// TestParseHeader_Timestamp verifies that role, iteration, and timestamp are all
// correctly extracted from a known header line.
func TestParseHeader_Timestamp(t *testing.T) {
	wantTime, _ := time.Parse(time.RFC3339, "2026-04-09T15:04:05Z")

	text := "[CHECKPOINT] strategist agent -- iteration ~12, 2026-04-09T15:04:05Z\n\nSTATUS: ACTIVE\n"

	parsed, err := checkpoint.ParseCheckpoint(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != "strategist" {
		t.Errorf("Role: got %q, want %q", parsed.Role, "strategist")
	}
	if parsed.ApproxIteration != 12 {
		t.Errorf("ApproxIteration: got %d, want 12", parsed.ApproxIteration)
	}
	if !parsed.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp: got %v, want %v", parsed.Timestamp, wantTime)
	}
}
