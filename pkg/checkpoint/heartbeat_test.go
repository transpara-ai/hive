package checkpoint_test

import (
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/checkpoint"
)

// TestHeartbeatContent_Fields verifies that HeartbeatContent carries the
// expected fields and that EventTypeName returns the correct value.
func TestHeartbeatContent_Fields(t *testing.T) {
	hb := checkpoint.HeartbeatContent{
		Role:          "implementer",
		Iteration:     5,
		MaxIterations: 20,
		TokensUsed:    1500,
		CostUSD:       0.23,
		Signal:        "ACTIVE",
		CurrentTaskID: "task-99",
	}

	if hb.Role != "implementer" {
		t.Errorf("Role: got %q, want %q", hb.Role, "implementer")
	}
	if hb.Iteration != 5 {
		t.Errorf("Iteration: got %d, want 5", hb.Iteration)
	}
	if hb.MaxIterations != 20 {
		t.Errorf("MaxIterations: got %d, want 20", hb.MaxIterations)
	}
	if hb.TokensUsed != 1500 {
		t.Errorf("TokensUsed: got %d, want 1500", hb.TokensUsed)
	}
	if hb.CostUSD != 0.23 {
		t.Errorf("CostUSD: got %f, want 0.23", hb.CostUSD)
	}
	if hb.Signal != "ACTIVE" {
		t.Errorf("Signal: got %q, want %q", hb.Signal, "ACTIVE")
	}
	if hb.CurrentTaskID != "task-99" {
		t.Errorf("CurrentTaskID: got %q, want %q", hb.CurrentTaskID, "task-99")
	}
	if hb.EventTypeName() != "hive.agent.heartbeat" {
		t.Errorf("EventTypeName: got %q, want %q", hb.EventTypeName(), "hive.agent.heartbeat")
	}
}

// TestHeartbeatFromSnapshot verifies that HeartbeatFromSnapshot correctly
// maps all LoopSnapshot fields onto HeartbeatContent.
func TestHeartbeatFromSnapshot(t *testing.T) {
	snap := checkpoint.LoopSnapshot{
		Role:          "guardian",
		Iteration:     3,
		MaxIterations: 15,
		TokensUsed:    800,
		CostUSD:       0.12,
		Signal:        "IDLE",
		CurrentTaskID: "task-42",
		CurrentTask:   "review PR",
		TaskStatus:    "reviewing",
	}

	hb := checkpoint.HeartbeatFromSnapshot(snap)

	if hb.Role != snap.Role {
		t.Errorf("Role: got %q, want %q", hb.Role, snap.Role)
	}
	if hb.Iteration != snap.Iteration {
		t.Errorf("Iteration: got %d, want %d", hb.Iteration, snap.Iteration)
	}
	if hb.MaxIterations != snap.MaxIterations {
		t.Errorf("MaxIterations: got %d, want %d", hb.MaxIterations, snap.MaxIterations)
	}
	if hb.TokensUsed != snap.TokensUsed {
		t.Errorf("TokensUsed: got %d, want %d", hb.TokensUsed, snap.TokensUsed)
	}
	if hb.CostUSD != snap.CostUSD {
		t.Errorf("CostUSD: got %f, want %f", hb.CostUSD, snap.CostUSD)
	}
	if hb.Signal != snap.Signal {
		t.Errorf("Signal: got %q, want %q", hb.Signal, snap.Signal)
	}
	if hb.CurrentTaskID != snap.CurrentTaskID {
		t.Errorf("CurrentTaskID: got %q, want %q", hb.CurrentTaskID, snap.CurrentTaskID)
	}
}

// TestQueryLatestHeartbeat_NoResults verifies that a nil store returns
// nil, nil without panicking.
func TestQueryLatestHeartbeat_NoResults(t *testing.T) {
	result, err := checkpoint.QueryLatestHeartbeat(nil, "implementer", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got: %+v", result)
	}
}
