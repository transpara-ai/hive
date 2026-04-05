package hive

import (
	"context"
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

// ────────────────────────────────────────────────────────────────────
// mapModelName tests
// ────────────────────────────────────────────────────────────────────

func TestMapModelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"haiku", ModelHaiku},
		{"sonnet", ModelSonnet},
		{"opus", ModelOpus},
		// Full model identifiers should also pass through correctly.
		{ModelHaiku, ModelHaiku},
		{ModelSonnet, ModelSonnet},
		{ModelOpus, ModelOpus},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapModelName(tt.input)
			if got != tt.want {
				t.Errorf("mapModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapModelName_Default(t *testing.T) {
	unknowns := []string{"gpt-4", "unknown", "", "SONNET", "Haiku"}
	for _, name := range unknowns {
		t.Run(name, func(t *testing.T) {
			got := mapModelName(name)
			if got != ModelSonnet {
				t.Errorf("mapModelName(%q) = %q, want ModelSonnet (%q)", name, got, ModelSonnet)
			}
		})
	}
}
