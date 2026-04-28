package hive

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/checkpoint"
)

// registerTestHuman creates a deterministic human actor for tests.
func registerTestHuman(t *testing.T, actors actor.IActorStore, name string) types.ActorID {
	t.Helper()
	h := sha256.Sum256([]byte("human:" + name))
	priv := ed25519.NewKeyFromSeed(h[:])
	pub := priv.Public().(ed25519.PublicKey)

	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		t.Fatalf("public key: %v", err)
	}

	a, err := actors.Register(pk, name, event.ActorTypeHuman)
	if err != nil {
		t.Fatalf("register human: %v", err)
	}
	return a.ID()
}

// TestHiveSummary_RuntimeRestart validates that a HiveSummary captured during
// one Runtime lifecycle is recoverable by a second Runtime pointing at the same
// backing stores. This is the integration-level version of the checkpoint
// package's unit test — it goes through hive.New() to exercise the full
// runtime construction path.
func TestHiveSummary_RuntimeRestart(t *testing.T) {
	ctx := context.Background()

	// Shared stores — these outlive individual runtimes, simulating a durable
	// backing store (Postgres + Open Brain in production).
	eventStore := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()
	thoughtStore := checkpoint.NewStubThoughtStore()

	humanID := registerTestHuman(t, actors, "TestOperator")

	// ── Runtime 1: create, capture a hive summary, tear down ────────────

	rt1, err := New(ctx, Config{
		Store:   eventStore,
		Actors:  actors,
		HumanID: humanID,
	})
	if err != nil {
		t.Fatalf("Runtime 1 New: %v", err)
	}

	// The runtime is alive. Simulate what the checkpoint sink does at a
	// boundary event: format and capture a hive summary.
	agents := []checkpoint.AgentSummary{
		{Role: "guardian", State: "idle"},
		{Role: "implementer", State: "active"},
		{Role: "strategist", State: "idle"},
	}
	tasks := checkpoint.TaskStats{
		Open:      2,
		Completed: 5,
		Details:   "task-99 in-progress",
	}
	budget := checkpoint.BudgetStats{
		TotalSpend: 1.75,
		DailyCap:   10.00,
	}

	summary := checkpoint.FormatHiveSummary(agents, tasks, budget)
	if err := thoughtStore.Capture(summary); err != nil {
		t.Fatalf("Runtime 1 capture summary: %v", err)
	}

	// Verify runtime 1 constructed successfully (sanity check).
	if rt1.store == nil {
		t.Fatal("Runtime 1 store is nil")
	}

	// ── Simulate process restart ─────────────────────────────────────────
	// Runtime 1 is gone. The event store, actor store, and thought store
	// survive because they're backed by durable storage.

	// ── Runtime 2: create from the same stores, recover ──────────────────

	rt2, err := New(ctx, Config{
		Store:   eventStore,
		Actors:  actors,
		HumanID: humanID,
	})
	if err != nil {
		t.Fatalf("Runtime 2 New: %v", err)
	}

	// Verify runtime 2 got a fresh conversation ID (new lifecycle).
	if rt1.convID == rt2.convID {
		t.Error("Runtime 2 should have a new conversation ID")
	}

	// Recover using the same thought store — this is what Run() does at
	// startup, but we call RecoverAll directly to avoid needing a full
	// agent loop with intelligence providers.
	roleNames := []string{"guardian", "implementer", "strategist"}
	staleness := 2 * time.Hour

	recovered, err := checkpoint.RecoverAll(roleNames, thoughtStore, eventStore, staleness)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	// ── Verify: every agent received the hive summary ────────────────────

	for _, role := range roleNames {
		rs, ok := recovered[role]
		if !ok {
			t.Errorf("%s: missing from recovery result", role)
			continue
		}

		if rs.HiveSummary == "" {
			t.Errorf("%s: HiveSummary is empty — did not survive restart", role)
			continue
		}

		// Verify structural markers.
		if !strings.Contains(rs.HiveSummary, "[HIVE SUMMARY]") {
			t.Errorf("%s: missing [HIVE SUMMARY] marker:\n%s", role, rs.HiveSummary)
		}

		// Verify agent data round-tripped.
		if !strings.Contains(rs.HiveSummary, "3 agents active") {
			t.Errorf("%s: missing agent count", role)
		}
		if !strings.Contains(rs.HiveSummary, "implementer(active)") {
			t.Errorf("%s: missing implementer(active)", role)
		}

		// Verify task data round-tripped.
		if !strings.Contains(rs.HiveSummary, "task-99 in-progress") {
			t.Errorf("%s: missing task details", role)
		}

		// Verify budget data round-tripped.
		if !strings.Contains(rs.HiveSummary, "$1.75 total spend") {
			t.Errorf("%s: missing budget spend", role)
		}
		if !strings.Contains(rs.HiveSummary, "$8.25 remaining daily cap") {
			t.Errorf("%s: missing remaining cap", role)
		}
	}
}

// TestHiveSummary_RuntimeRestart_WithCheckpoint extends the restart test to
// include a per-agent checkpoint. This verifies that warm-started agents get
// both their individual checkpoint state AND the shared hive summary.
func TestHiveSummary_RuntimeRestart_WithCheckpoint(t *testing.T) {
	ctx := context.Background()

	eventStore := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()
	thoughtStore := checkpoint.NewStubThoughtStore()

	humanID := registerTestHuman(t, actors, "TestOperator")

	// ── Runtime 1 ────────────────────────────────────────────────────────

	_, err := New(ctx, Config{
		Store:   eventStore,
		Actors:  actors,
		HumanID: humanID,
	})
	if err != nil {
		t.Fatalf("Runtime 1 New: %v", err)
	}

	// Capture a per-agent checkpoint (implementer was mid-task).
	snap := checkpoint.LoopSnapshot{
		Role:          "implementer",
		Iteration:     8,
		MaxIterations: 50,
		TokensUsed:    4500,
		CostUSD:       0.65,
		Signal:        "ACTIVE",
		CurrentTaskID: "task-77",
		CurrentTask:   "add error handling",
		TaskStatus:    "in-progress",
	}
	cpText := checkpoint.FormatCheckpoint(checkpoint.TaskAssigned, snap,
		"add retry logic to HTTP client", "run go test ./...", "")
	if err := thoughtStore.Capture(cpText); err != nil {
		t.Fatalf("capture checkpoint: %v", err)
	}

	// Capture the hive summary.
	summary := checkpoint.FormatHiveSummary(
		[]checkpoint.AgentSummary{
			{Role: "guardian", State: "idle"},
			{Role: "implementer", State: "active"},
		},
		checkpoint.TaskStats{Open: 1, Completed: 3, Details: "task-77 in-progress"},
		checkpoint.BudgetStats{TotalSpend: 0.65, DailyCap: 10.00},
	)
	if err := thoughtStore.Capture(summary); err != nil {
		t.Fatalf("capture summary: %v", err)
	}

	// ── Runtime 2: recover ───────────────────────────────────────────────

	_, err = New(ctx, Config{
		Store:   eventStore,
		Actors:  actors,
		HumanID: humanID,
	})
	if err != nil {
		t.Fatalf("Runtime 2 New: %v", err)
	}

	recovered, err := checkpoint.RecoverAll(
		[]string{"guardian", "implementer"},
		thoughtStore, eventStore, 2*time.Hour,
	)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	// ── Verify implementer: warm-started with checkpoint + summary ───────

	impl := recovered["implementer"]
	if impl.Mode != checkpoint.ModeWarm {
		t.Errorf("implementer: Mode = %v, want warm", impl.Mode)
	}
	if impl.Iteration != 8 {
		t.Errorf("implementer: Iteration = %d, want 8", impl.Iteration)
	}
	if impl.Intent != "add retry logic to HTTP client" {
		t.Errorf("implementer: Intent = %q", impl.Intent)
	}
	if impl.CurrentTaskID != "task-77" {
		t.Errorf("implementer: CurrentTaskID = %q, want task-77", impl.CurrentTaskID)
	}
	if impl.ConsumedTokens != 4500 {
		t.Errorf("implementer: ConsumedTokens = %d, want 4500", impl.ConsumedTokens)
	}
	if impl.HiveSummary == "" {
		t.Error("implementer: warm-started but HiveSummary is empty")
	}

	// ── Verify guardian: cold-started but still has hive summary ──────────

	guardian := recovered["guardian"]
	if guardian.Mode != checkpoint.ModeCold {
		t.Errorf("guardian: Mode = %v, want cold", guardian.Mode)
	}
	if guardian.HiveSummary == "" {
		t.Error("guardian: cold-started agent should still receive HiveSummary")
	}
	if guardian.HiveSummary != impl.HiveSummary {
		t.Error("guardian and implementer should receive the same HiveSummary")
	}
}
