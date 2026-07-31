package hive

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// ────────────────────────────────────────────────────────────────────
// dynamicAgentTracker tests
// ────────────────────────────────────────────────────────────────────

func TestDynamicAgentTracker_Track(t *testing.T) {
	d := newDynamicAgentTracker()

	if d.IsTracked("alpha") {
		t.Fatal("should not be tracked before Track() is called")
	}

	called := false
	cancel := func() { called = true }
	d.Track("alpha", cancel)

	if !d.IsTracked("alpha") {
		t.Fatal("should be tracked after Track() is called")
	}

	// Confirm cancel func is stored (call it and verify the closure ran).
	d.mu.Lock()
	fn := d.agents["alpha"]
	d.mu.Unlock()
	fn()
	if !called {
		t.Fatal("cancel func should have been called")
	}
}

func TestDynamicAgentTracker_Dedup(t *testing.T) {
	d := newDynamicAgentTracker()

	firstCalled := false
	secondCalled := false
	d.Track("beta", func() { firstCalled = true })
	d.Track("beta", func() { secondCalled = true }) // duplicate — must be no-op

	if !d.IsTracked("beta") {
		t.Fatal("should still be tracked after duplicate Track()")
	}

	// Only the first cancel func should be stored.
	d.mu.Lock()
	fn := d.agents["beta"]
	d.mu.Unlock()
	fn()

	if !firstCalled {
		t.Fatal("first cancel should have been called")
	}
	if secondCalled {
		t.Fatal("second cancel should not have been stored (duplicate)")
	}
}

func TestDynamicAgentTracker_Wait(t *testing.T) {
	d := newDynamicAgentTracker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Simulate a tracked goroutine that finishes quickly.
	d.wg.Add(1)
	d.Track("gamma", cancel)
	go func() {
		defer d.wg.Done()
	}()

	// Wait should return without hanging.
	done := make(chan struct{})
	go func() {
		d.Wait()
		close(done)
	}()

	select {
	case <-done:
		// pass
	case <-ctx.Done():
		t.Fatal("Wait() did not return")
	}
}

func TestDynamicAgentTracker_ConcurrentCap(t *testing.T) {
	d := newDynamicAgentTracker(OrganicV1MaximumDynamicActors)
	names := []string{"alpha", "beta", "gamma", "delta"}
	results := make(chan dynamicSlotResult, len(names))
	var callers sync.WaitGroup
	for _, name := range names {
		callers.Add(1)
		go func() {
			defer callers.Done()
			results <- d.Reserve(name)
		}()
	}
	callers.Wait()
	close(results)

	reserved := 0
	limited := 0
	for result := range results {
		switch result {
		case dynamicSlotReserved:
			reserved++
		case dynamicSlotLimitReached:
			limited++
		}
	}
	if reserved != OrganicV1MaximumDynamicActors || limited != 1 {
		t.Fatalf("reserved=%d limited=%d, want 3 and 1", reserved, limited)
	}
	if got := d.OccupiedCount(); got != OrganicV1MaximumDynamicActors {
		t.Fatalf("occupied count = %d, want %d", got, OrganicV1MaximumDynamicActors)
	}
}

func TestDynamicAgentTracker_LimitEventFailureCanRetryButCommitCannot(t *testing.T) {
	d := newDynamicAgentTracker(OrganicV1MaximumDynamicActors)
	const key = "conversation|proposal|organic-v1"
	if !d.BeginLimitEvent(key) {
		t.Fatal("first limit-event claim was refused")
	}
	if d.BeginLimitEvent(key) {
		t.Fatal("concurrent limit-event claim was admitted")
	}
	d.FinishLimitEvent(key, false)
	if !d.BeginLimitEvent(key) {
		t.Fatal("failed limit-event write did not become retryable")
	}
	d.FinishLimitEvent(key, true)
	if d.BeginLimitEvent(key) {
		t.Fatal("committed limit-event tuple became writable again")
	}
}

func TestDynamicAgentTracker_RecoveryAndNewCapacityCombinations(t *testing.T) {
	for recovered := 0; recovered <= OrganicV1MaximumDynamicActors; recovered++ {
		t.Run(fmt.Sprintf("%d+%d", recovered, OrganicV1MaximumDynamicActors-recovered), func(t *testing.T) {
			d := newDynamicAgentTracker(OrganicV1MaximumDynamicActors)
			for i := 0; i < OrganicV1MaximumDynamicActors; i++ {
				name := fmt.Sprintf("actor-%d", i)
				if result := d.Reserve(name); result != dynamicSlotReserved {
					t.Fatalf("reserve %s = %v", name, result)
				}
				if !d.Attach(name, func() {}, i < recovered) {
					t.Fatalf("attach %s failed", name)
				}
				d.Done()
			}
			if d.Count() != OrganicV1MaximumDynamicActors ||
				d.RecoveredCount() != recovered ||
				d.NewCount() != OrganicV1MaximumDynamicActors-recovered {
				t.Fatalf(
					"counts total=%d recovered=%d new=%d",
					d.Count(), d.RecoveredCount(), d.NewCount(),
				)
			}
			if result := d.Reserve("fourth"); result != dynamicSlotLimitReached {
				t.Fatalf("fourth reserve = %v, want limit", result)
			}
			d.Wait()
		})
	}
}

// ────────────────────────────────────────────────────────────────────
// mapModelName tests
// ────────────────────────────────────────────────────────────────────

func TestMapModelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"haiku", "claude-haiku-4-5-20251001"},
		{"sonnet", "claude-sonnet-4-6"},
		{"opus", "claude-opus-4-6"},
		// Full model identifiers should also pass through correctly.
		{"claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"claude-opus-4-6", "claude-opus-4-6"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := mapModelName(tt.input, nil)
			if err != nil {
				t.Fatalf("mapModelName(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("mapModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapModelName_UnknownReturnsError(t *testing.T) {
	unknowns := []string{"gpt-4", "unknown", "", "SONNET", "Haiku"}
	for _, name := range unknowns {
		t.Run(name, func(t *testing.T) {
			_, err := mapModelName(name, nil)
			if err == nil {
				t.Errorf("mapModelName(%q) should return error for unknown model", name)
			}
		})
	}
}
