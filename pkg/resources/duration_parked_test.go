package resources

import (
	"reflect"
	"testing"
	"time"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v15-F1(b): the registry carries the duration-park marker.
//
// The allocator's recheck gate must key on the EXPLICITLY-SET parked state,
// never on derived duration exhaustion: Budget elapsed keeps growing for
// agents whose loops already EXITED (one-shots, iteration exhaustion), so
// "any budget past its duration limit" turns permanently true the moment
// any agent completes — and a permanently-true gate is a 30-second LLM
// storm that burns the allocator's iteration budget. Only a live loop
// entering the park sets the marker; only that loop (resuming) or its
// death clears it.
// ════════════════════════════════════════════════════════════════════════

func registryWithAgents(names ...string) *BudgetRegistry {
	reg := NewBudgetRegistry()
	for _, n := range names {
		b := NewBudget(BudgetConfig{MaxDuration: 30 * time.Minute})
		reg.Register(n, b, 100, "test-model")
	}
	return reg
}

func TestSetDurationParked_MarksAndClearsEntry(t *testing.T) {
	reg := registryWithAgents("reviewer", "implementer")

	reg.SetDurationParked("reviewer", true)
	parked := map[string]bool{}
	for _, e := range reg.Snapshot() {
		parked[e.Name] = e.DurationParked
	}
	if !parked["reviewer"] {
		t.Errorf("reviewer DurationParked = false after SetDurationParked(true)")
	}
	if parked["implementer"] {
		t.Errorf("implementer DurationParked = true; marker must be per-agent")
	}

	reg.SetDurationParked("reviewer", false)
	for _, e := range reg.Snapshot() {
		if e.DurationParked {
			t.Errorf("%s DurationParked = true after clear", e.Name)
		}
	}
}

func TestSetDurationParked_UnknownAgentIsNoOp(t *testing.T) {
	reg := registryWithAgents("reviewer")
	// Must not panic or create a phantom entry.
	reg.SetDurationParked("ghost", true)
	if got := len(reg.Snapshot()); got != 1 {
		t.Fatalf("registry entries after unknown-name set = %d; want 1 (no phantom entries)", got)
	}
	if names := reg.DurationParkedNames(); len(names) != 0 {
		t.Fatalf("DurationParkedNames after unknown-name set = %v; want empty", names)
	}
}

func TestDurationParkedNames_SortedAndLive(t *testing.T) {
	reg := registryWithAgents("zeta", "alpha", "mid")

	if names := reg.DurationParkedNames(); len(names) != 0 {
		t.Fatalf("DurationParkedNames on fresh registry = %v; want empty", names)
	}

	reg.SetDurationParked("zeta", true)
	reg.SetDurationParked("alpha", true)
	want := []string{"alpha", "zeta"}
	if got := reg.DurationParkedNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DurationParkedNames = %v; want %v (sorted, parked only)", got, want)
	}

	reg.SetDurationParked("zeta", false)
	want = []string{"alpha"}
	if got := reg.DurationParkedNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DurationParkedNames after unpark = %v; want %v", got, want)
	}
}
