package membrane

import (
	"context"
	"testing"
	"time"
)

func TestAuthorityGateRequired(t *testing.T) {
	gate := NewAuthorityGate(TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6})

	decision := make(chan ActionDecision, 1)
	actionID := "act-001"

	go func() {
		d, err := gate.Gate(context.Background(), actionID, 0.1, "test action")
		if err != nil {
			t.Errorf("Gate: %v", err)
			return
		}
		decision <- d
	}()

	// Should not have decided yet
	select {
	case <-decision:
		t.Fatal("Required action should block until resolved")
	case <-time.After(50 * time.Millisecond):
		// expected
	}

	gate.Resolve(actionID, ActionDecision{Decision: "approved", DecidedBy: "human-1"})

	select {
	case d := <-decision:
		if d.Decision != "approved" {
			t.Errorf("decision = %q, want approved", d.Decision)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resolution")
	}
}

func TestAuthorityGateNotification(t *testing.T) {
	gate := NewAuthorityGate(TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6})

	d, err := gate.Gate(context.Background(), "act-002", 0.8, "test action")
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if d.Decision != "auto_approved" {
		t.Errorf("decision = %q, want auto_approved", d.Decision)
	}
}

func TestAuthorityGateRecommendedTimeout(t *testing.T) {
	gate := NewAuthorityGate(TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6})
	gate.RecommendedTimeout = 100 * time.Millisecond

	d, err := gate.Gate(context.Background(), "act-003", 0.4, "test action")
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if d.Decision != "auto_approved" {
		t.Errorf("decision = %q, want auto_approved", d.Decision)
	}
}

func TestAuthorityGateContextCancellation(t *testing.T) {
	gate := NewAuthorityGate(TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := gate.Gate(ctx, "act-004", 0.1, "test action")
	if err == nil {
		t.Fatal("expected context error")
	}
}
