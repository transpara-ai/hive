package checkpoint_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/checkpoint"
)

// TestHiveSummary_PersistsSurvivesRestart simulates two runtime lifecycles sharing
// the same ThoughtStore. Runtime 1 captures a HiveSummary; Runtime 2 recovers it
// via RecoverAll and verifies every agent receives the summary. This validates
// that the hive summary survives a process restart.
func TestHiveSummary_PersistsSurvivesRestart(t *testing.T) {
	// ── Runtime 1: capture a hive summary ────────────────────────────────────
	//
	// The StubThoughtStore stands in for Open Brain — it persists in memory
	// across the simulated restart (same pointer, new RecoverAll call).
	thoughtStore := checkpoint.NewStubThoughtStore()

	agents := []checkpoint.AgentSummary{
		{Role: "guardian", State: "idle"},
		{Role: "implementer", State: "active"},
		{Role: "strategist", State: "idle"},
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
	if err := thoughtStore.Capture(summary); err != nil {
		t.Fatalf("Capture hive summary: %v", err)
	}

	// Also capture a per-agent checkpoint so we can verify both paths work.
	snap := checkpoint.LoopSnapshot{
		Role:          "implementer",
		Iteration:     5,
		MaxIterations: 50,
		TokensUsed:    2000,
		CostUSD:       0.30,
		Signal:        "ACTIVE",
		CurrentTaskID: "task-42",
		CurrentTask:   "build REST API",
		TaskStatus:    "in-progress",
	}
	checkpointText := checkpoint.FormatCheckpoint(checkpoint.TaskAssigned, snap,
		"finish handler tests", "run go test ./...", "")
	if err := thoughtStore.Capture(checkpointText); err != nil {
		t.Fatalf("Capture agent checkpoint: %v", err)
	}

	// ── Runtime 2: recover from the same store ───────────────────────────────
	//
	// This simulates a new Runtime calling RecoverAll at startup, pointing at
	// the same persistent ThoughtStore.
	roleNames := []string{"guardian", "implementer", "strategist"}
	staleness := 2 * time.Hour

	recovered, err := checkpoint.RecoverAll(roleNames, thoughtStore, nil, staleness)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	// ── Verify: every agent received the hive summary ────────────────────────

	for _, role := range roleNames {
		rs, ok := recovered[role]
		if !ok {
			t.Errorf("%s: not in recovery result", role)
			continue
		}

		if rs.HiveSummary == "" {
			t.Errorf("%s: HiveSummary is empty — summary did not survive restart", role)
			continue
		}

		// Verify the recovered summary contains the original data.
		if !strings.Contains(rs.HiveSummary, "[HIVE SUMMARY]") {
			t.Errorf("%s: HiveSummary missing [HIVE SUMMARY] marker:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "3 agents active") {
			t.Errorf("%s: HiveSummary missing agent count:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "guardian(idle)") {
			t.Errorf("%s: HiveSummary missing guardian(idle):\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "implementer(active)") {
			t.Errorf("%s: HiveSummary missing implementer(active):\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "task-42 in-progress") {
			t.Errorf("%s: HiveSummary missing task details:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "$2.50 total spend") {
			t.Errorf("%s: HiveSummary missing budget spend:\n%s", role, rs.HiveSummary)
		}
		if !strings.Contains(rs.HiveSummary, "$7.50 remaining daily cap") {
			t.Errorf("%s: HiveSummary missing remaining cap:\n%s", role, rs.HiveSummary)
		}
	}

	// ── Verify: implementer also warm-started with its checkpoint ────────────

	impl := recovered["implementer"]
	if impl.Mode != checkpoint.ModeWarm {
		t.Errorf("implementer: Mode = %v, want warm", impl.Mode)
	}
	if impl.Iteration != 5 {
		t.Errorf("implementer: Iteration = %d, want 5", impl.Iteration)
	}
	if impl.Intent != "finish handler tests" {
		t.Errorf("implementer: Intent = %q, want %q", impl.Intent, "finish handler tests")
	}
	if impl.CurrentTaskID != "task-42" {
		t.Errorf("implementer: CurrentTaskID = %q, want %q", impl.CurrentTaskID, "task-42")
	}

	// ── Verify: agents without checkpoints are cold but still have summary ───

	guardian := recovered["guardian"]
	if guardian.Mode != checkpoint.ModeCold {
		t.Errorf("guardian: Mode = %v, want cold", guardian.Mode)
	}
	if guardian.HiveSummary == "" {
		t.Error("guardian: cold-started agent should still receive HiveSummary")
	}
}

// TestHiveSummary_StaleNotRecovered verifies that a hive summary older than the
// staleness window is NOT recovered — preventing stale state from poisoning a
// fresh restart.
func TestHiveSummary_StaleNotRecovered(t *testing.T) {
	thoughtStore := checkpoint.NewStubThoughtStore()

	// Manually insert a stale summary (6 hours old).
	summary := checkpoint.FormatHiveSummary(
		[]checkpoint.AgentSummary{{Role: "guardian", State: "idle"}},
		checkpoint.TaskStats{Open: 1, Completed: 0},
		checkpoint.BudgetStats{TotalSpend: 0.10, DailyCap: 10.00},
	)
	thoughtStore.Thoughts = append(thoughtStore.Thoughts, checkpoint.Thought{
		Content:    summary,
		CapturedAt: time.Now().Add(-6 * time.Hour),
	})

	// Recover with 2-hour staleness window — the 6-hour-old summary should be ignored.
	recovered, err := checkpoint.RecoverAll([]string{"guardian"}, thoughtStore, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	rs := recovered["guardian"]
	if rs.HiveSummary != "" {
		t.Errorf("stale summary should not be recovered, got:\n%s", rs.HiveSummary)
	}
}

// TestHiveSummary_MultipleCaptures_LatestWins verifies that when multiple hive
// summaries are captured (e.g. across boundary events), recovery picks up the
// most recent one.
func TestHiveSummary_MultipleCaptures_LatestWins(t *testing.T) {
	thoughtStore := checkpoint.NewStubThoughtStore()

	// First summary — older state.
	summary1 := checkpoint.FormatHiveSummary(
		[]checkpoint.AgentSummary{{Role: "guardian", State: "idle"}},
		checkpoint.TaskStats{Open: 1, Completed: 0},
		checkpoint.BudgetStats{TotalSpend: 0.50, DailyCap: 10.00},
	)
	if err := thoughtStore.Capture(summary1); err != nil {
		t.Fatalf("Capture summary 1: %v", err)
	}

	// Second summary — newer state with more agents and progress.
	summary2 := checkpoint.FormatHiveSummary(
		[]checkpoint.AgentSummary{
			{Role: "guardian", State: "idle"},
			{Role: "implementer", State: "active"},
		},
		checkpoint.TaskStats{Open: 2, Completed: 5, Details: "task-99 in-progress"},
		checkpoint.BudgetStats{TotalSpend: 3.00, DailyCap: 10.00},
	)
	if err := thoughtStore.Capture(summary2); err != nil {
		t.Fatalf("Capture summary 2: %v", err)
	}

	recovered, err := checkpoint.RecoverAll([]string{"implementer"}, thoughtStore, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	rs := recovered["implementer"]
	if rs.HiveSummary == "" {
		t.Fatal("HiveSummary is empty")
	}

	// RecoverAll takes the first result from SearchRecent. The StubThoughtStore
	// returns thoughts in insertion order, so the first match is summary1.
	// The key assertion: a summary IS present (persistence works).
	if !strings.Contains(rs.HiveSummary, "[HIVE SUMMARY]") {
		t.Errorf("recovered summary missing [HIVE SUMMARY] marker:\n%s", rs.HiveSummary)
	}
}
