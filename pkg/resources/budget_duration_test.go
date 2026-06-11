package resources

import (
	"testing"
	"time"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v14-F3(c): duration renewal
//
// A keepalive agent parked on duration exhaustion resumes only if its
// budget's wall-clock limit can be raised at runtime and its own Check()
// observes the raise. The registry adjusts THROUGH the agent's Budget —
// no duplicated duration state anywhere.
// ════════════════════════════════════════════════════════════════════════

func TestSetMaxDurationRenewsExhaustedBudget(t *testing.T) {
	start := time.Now().Add(-45 * time.Minute)
	b := NewBudgetForTest(BudgetConfig{MaxDuration: 30 * time.Minute}, start)

	if err := b.Check(); err == nil {
		t.Fatal("Check() on a 45m-old budget with a 30m limit = nil; want duration-exceeded")
	}
	b.SetMaxDuration(2 * time.Hour)
	if err := b.Check(); err != nil {
		t.Fatalf("Check() after SetMaxDuration(2h) = %v; want nil — renewal must unblock the parked loop", err)
	}
	if got := b.MaxDuration(); got != 2*time.Hour {
		t.Fatalf("MaxDuration() = %v; want 2h", got)
	}
}

func TestAdjustMaxDurationWritesThroughAndClamps(t *testing.T) {
	r := NewBudgetRegistry()
	b := NewBudgetForTest(BudgetConfig{MaxDuration: 30 * time.Minute}, time.Now())
	r.Register("implementer", b, 50, "claude-opus-4-6")

	prev, now, err := r.AdjustMaxDuration("implementer", +90, 10, 720)
	if err != nil {
		t.Fatalf("AdjustMaxDuration(+90): %v", err)
	}
	if prev != 30 || now != 120 {
		t.Fatalf("AdjustMaxDuration(+90) = (%d, %d); want (30, 120)", prev, now)
	}
	if got := b.MaxDuration(); got != 120*time.Minute {
		t.Fatalf("budget MaxDuration after adjust = %v; want 120m (registry must write through to the live budget)", got)
	}

	// Ceiling clamp.
	_, now, err = r.AdjustMaxDuration("implementer", 1_000_000, 10, 720)
	if err != nil || now != 720 {
		t.Fatalf("ceiling clamp = (%d, %v); want (720, nil)", now, err)
	}
	// Floor clamp.
	_, now, err = r.AdjustMaxDuration("implementer", -1_000_000, 10, 720)
	if err != nil || now != 10 {
		t.Fatalf("floor clamp = (%d, %v); want (10, nil)", now, err)
	}
	// Unknown agent refuses.
	if _, _, err := r.AdjustMaxDuration("ghost", 1, 10, 720); err == nil {
		t.Fatal("AdjustMaxDuration(unknown agent) = nil error; want refusal")
	}
}
