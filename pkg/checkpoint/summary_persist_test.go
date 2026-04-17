package checkpoint_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/checkpoint"
)

// TestHiveSummary_PersistsSurvivesRestart simulates a full reboot-survival cycle:
//
//  1. "Runtime 1" captures a HiveSummary into the ThoughtStore.
//  2. "Runtime 2" boots fresh and calls RecoverAll on the same store.
//  3. Every agent's RecoveryState.HiveSummary contains the captured summary.
//
// This validates that the summary written by one runtime instance is
// retrievable by a subsequent instance pointing at the same persistent store.
func TestHiveSummary_PersistsSurvivesRestart(t *testing.T) {
	// ── "Runtime 1": format and capture a hive summary ────────────────────
	store := checkpoint.NewStubThoughtStore()

	agents := []checkpoint.AgentSummary{
		{Role: "guardian", State: "idle"},
		{Role: "implementer", State: "active"},
		{Role: "cto", State: "idle"},
	}
	tasks := checkpoint.TaskStats{
		Open:      3,
		Completed: 7,
		Details:   "task-42 in-progress",
	}
	budget := checkpoint.BudgetStats{
		TotalSpend: 2.50,
		DailyCap:   10.00,
	}

	summary := checkpoint.FormatHiveSummary(agents, tasks, budget)
	if err := store.Capture(summary); err != nil {
		t.Fatalf("Capture hive summary: %v", err)
	}

	// ── "Runtime 2": fresh RecoverAll pointing at the same store ──────────
	roles := []string{"guardian", "implementer", "cto"}
	result, err := checkpoint.RecoverAll(roles, store, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	// ── Verify every agent received the hive summary ─────────────────────
	for _, role := range roles {
		rs, ok := result[role]
		if !ok {
			t.Errorf("%s: missing from result map", role)
			continue
		}
		if rs.HiveSummary == "" {
			t.Errorf("%s: HiveSummary is empty — did not survive restart", role)
			continue
		}
		if !strings.Contains(rs.HiveSummary, "[HIVE SUMMARY]") {
			t.Errorf("%s: HiveSummary missing [HIVE SUMMARY] marker:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "3 agents active") {
			t.Errorf("%s: HiveSummary missing agent count:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "task-42 in-progress") {
			t.Errorf("%s: HiveSummary missing task details:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "$2.50 total spend") {
			t.Errorf("%s: HiveSummary missing budget info:\n%s", role, rs.HiveSummary)
		}
	}
}

// TestHiveSummary_CoexistsWithAgentCheckpoints verifies that individual agent
// checkpoints (warm-start) and the hive-wide summary are both recovered from
// the same store. This exercises the full two-tier recovery path where an agent
// warm-starts from its own checkpoint AND receives the global hive summary.
func TestHiveSummary_CoexistsWithAgentCheckpoints(t *testing.T) {
	store := checkpoint.NewStubThoughtStore()

	// ── "Runtime 1": capture an agent checkpoint + a hive summary ─────────
	snap := checkpoint.LoopSnapshot{
		Role:          "implementer",
		Iteration:     15,
		MaxIterations: 50,
		TokensUsed:    3000,
		CostUSD:       0.45,
		Signal:        "ACTIVE",
		CurrentTaskID: "task-99",
		CurrentTask:   "add validation to API",
		TaskStatus:    "in-progress",
	}
	cpText := checkpoint.FormatCheckpoint(checkpoint.TaskAssigned, snap, "write error tests", "", "")
	if err := store.Capture(cpText); err != nil {
		t.Fatalf("Capture checkpoint: %v", err)
	}

	summaryText := checkpoint.FormatHiveSummary(
		[]checkpoint.AgentSummary{
			{Role: "guardian", State: "idle"},
			{Role: "implementer", State: "active"},
		},
		checkpoint.TaskStats{Open: 1, Completed: 10, Details: "task-99 in-progress"},
		checkpoint.BudgetStats{TotalSpend: 3.00, DailyCap: 10.00},
	)
	if err := store.Capture(summaryText); err != nil {
		t.Fatalf("Capture hive summary: %v", err)
	}

	// ── "Runtime 2": RecoverAll ───────────────────────────────────────────
	roles := []string{"implementer", "guardian"}
	result, err := checkpoint.RecoverAll(roles, store, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	// Implementer should warm-start AND have the hive summary.
	impl := result["implementer"]
	if impl.Mode != checkpoint.ModeWarm {
		t.Errorf("implementer: Mode = %v, want warm", impl.Mode)
	}
	if impl.Iteration != 15 {
		t.Errorf("implementer: Iteration = %d, want 15", impl.Iteration)
	}
	if impl.Intent != "write error tests" {
		t.Errorf("implementer: Intent = %q, want %q", impl.Intent, "write error tests")
	}
	if impl.CurrentTaskID != "task-99" {
		t.Errorf("implementer: CurrentTaskID = %q, want %q", impl.CurrentTaskID, "task-99")
	}
	if impl.HiveSummary == "" {
		t.Error("implementer: HiveSummary empty despite warm start")
	}
	if !strings.Contains(impl.HiveSummary, "[HIVE SUMMARY]") {
		t.Errorf("implementer: HiveSummary missing marker:\n%s", impl.HiveSummary)
	}

	// Guardian cold-starts (no checkpoint) but should still get the hive summary.
	guard := result["guardian"]
	if guard.Mode != checkpoint.ModeCold {
		t.Errorf("guardian: Mode = %v, want cold", guard.Mode)
	}
	if guard.HiveSummary == "" {
		t.Error("guardian: HiveSummary empty despite summary in store")
	}
	if !strings.Contains(guard.HiveSummary, "[HIVE SUMMARY]") {
		t.Errorf("guardian: HiveSummary missing marker:\n%s", guard.HiveSummary)
	}

	// Both agents should have the same summary.
	if impl.HiveSummary != guard.HiveSummary {
		t.Error("implementer and guardian have different HiveSummary values — should be identical")
	}
}

// TestHiveSummary_StaleNotRecovered verifies that a hive summary captured
// beyond the staleness window is NOT recovered — agents start without
// stale context that could mislead them after a long outage.
func TestHiveSummary_StaleNotRecovered(t *testing.T) {
	store := checkpoint.NewStubThoughtStore()

	// Insert a hive summary with a timestamp 5 hours in the past.
	summary := checkpoint.FormatHiveSummary(
		[]checkpoint.AgentSummary{{Role: "guardian", State: "idle"}},
		checkpoint.TaskStats{Open: 1, Completed: 0},
		checkpoint.BudgetStats{TotalSpend: 0.10, DailyCap: 10.00},
	)
	store.Thoughts = append(store.Thoughts, checkpoint.Thought{
		Content:    summary,
		CapturedAt: time.Now().Add(-5 * time.Hour),
	})

	// Staleness window is 2 hours — the 5-hour-old summary should be ignored.
	roles := []string{"guardian", "implementer"}
	result, err := checkpoint.RecoverAll(roles, store, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	for _, role := range roles {
		rs := result[role]
		if rs.HiveSummary != "" {
			t.Errorf("%s: HiveSummary should be empty for stale summary, got:\n%s", role, rs.HiveSummary)
		}
	}
}

// TestHiveSummary_NilThoughtStore verifies graceful degradation when no
// ThoughtStore is available (e.g. no Open Brain credentials). All agents
// should start cold with no summary and no panic.
func TestHiveSummary_NilThoughtStore(t *testing.T) {
	roles := []string{"guardian", "implementer", "cto"}
	result, err := checkpoint.RecoverAll(roles, nil, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	for _, role := range roles {
		rs, ok := result[role]
		if !ok {
			t.Errorf("%s: missing from result map", role)
			continue
		}
		if rs.Mode != checkpoint.ModeCold {
			t.Errorf("%s: Mode = %v, want cold", role, rs.Mode)
		}
		if rs.HiveSummary != "" {
			t.Errorf("%s: HiveSummary should be empty when ThoughtStore is nil", role)
		}
	}
}
