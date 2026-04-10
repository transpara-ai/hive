package checkpoint

import (
	"testing"
)

// TestReplayBudgetFromStore_Empty verifies that a nil store returns an
// initialized empty map with no error.
func TestReplayBudgetFromStore_Empty(t *testing.T) {
	result, err := ReplayBudgetFromStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

// TestReplayCTOFromStore_Empty verifies that a nil store returns an
// initialized empty state with no error.
func TestReplayCTOFromStore_Empty(t *testing.T) {
	state, err := ReplayCTOFromStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state, got nil")
	}
	if state.GapByCategory == nil {
		t.Error("GapByCategory map is nil")
	}
	if state.DirectiveByTarget == nil {
		t.Error("DirectiveByTarget map is nil")
	}
	if state.EmittedGaps == nil {
		t.Error("EmittedGaps map is nil")
	}
	if len(state.GapByCategory) != 0 {
		t.Errorf("expected empty GapByCategory, got %d entries", len(state.GapByCategory))
	}
	if len(state.DirectiveByTarget) != 0 {
		t.Errorf("expected empty DirectiveByTarget, got %d entries", len(state.DirectiveByTarget))
	}
}

// TestReplaySpawnerFromStore_Empty verifies that a nil store returns an
// initialized empty state with no error.
func TestReplaySpawnerFromStore_Empty(t *testing.T) {
	state, err := ReplaySpawnerFromStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state, got nil")
	}
	if state.RecentRejections == nil {
		t.Error("RecentRejections map is nil")
	}
	if state.ProcessedGaps == nil {
		t.Error("ProcessedGaps map is nil")
	}
	if state.PendingProposal != "" {
		t.Errorf("expected empty PendingProposal, got %q", state.PendingProposal)
	}
}

// TestReplayReviewerFromStore_Empty verifies that a nil store returns an
// initialized empty state with no error.
func TestReplayReviewerFromStore_Empty(t *testing.T) {
	state, err := ReplayReviewerFromStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state, got nil")
	}
	if state.ReviewCounts == nil {
		t.Error("ReviewCounts map is nil")
	}
	if state.CompletedTasks == nil {
		t.Error("CompletedTasks map is nil")
	}
	if len(state.ReviewCounts) != 0 {
		t.Errorf("expected empty ReviewCounts, got %d entries", len(state.ReviewCounts))
	}
	if len(state.CompletedTasks) != 0 {
		t.Errorf("expected empty CompletedTasks, got %d entries", len(state.CompletedTasks))
	}
}

// TestReplayIterationFromStore_Empty verifies that a nil store returns an
// initialized empty map with no error.
func TestReplayIterationFromStore_Empty(t *testing.T) {
	result, err := ReplayIterationFromStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

// TestReplayDynamicAgentsFromStore_Empty verifies that a nil store returns nil, nil.
func TestReplayDynamicAgentsFromStore_Empty(t *testing.T) {
	result, err := ReplayDynamicAgentsFromStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

// Populated-store tests require constructing events through a proper graph
// (see pkg/loop/*_integration_test.go for the pattern). Out of scope here;
// the nil-store tests cover the initialization contract.
