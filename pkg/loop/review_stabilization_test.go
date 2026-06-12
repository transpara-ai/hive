package loop

import (
	"strings"
	"testing"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v15-F3: reviewer starvation.
//
// The /review stabilization window (10 iterations, hardcoded) and the 30m
// worker lifespan were tuned independently and met for the first time in
// round 5: at the reviewer's natural cadence (~3.3 min/iteration with task
// artifacts in context) the warm-up alone costs ~33 minutes — longer than
// the lifespan. Result: 9 iterations, every /review rejected "observe
// first", ZERO reviews per epoch, structurally.
//
// The tune: the window drops to 3 (first review possible ~10 minutes in,
// leaving two-thirds of the lifespan for actual reviewing) and becomes
// env-tunable (REVIEWER_STABILIZATION_WINDOW, matching the ALLOCATOR_* and
// CTO_* knob convention) so round operators can adjust it without a
// rebuild. The lifespan stays 30m: with v15-F1 fixed, lifespan is governed
// live by the allocator's renewals — the window was the structural half.
// Unparseable or negative env values fall back to the default (fail safe);
// an explicit 0 disables the window (an operator's deliberate choice).
// ════════════════════════════════════════════════════════════════════════

func validReviewCmd() *ReviewCommand {
	return &ReviewCommand{
		TaskID:     "task-123",
		Verdict:    "approve",
		Summary:    "Looks good.",
		Issues:     []string{},
		Confidence: 0.9,
	}
}

func TestReviewStabilization_DefaultWindowIsThree(t *testing.T) {
	for _, iter := range []int{0, 1, 2} {
		if err := validateReviewCommand(validReviewCmd(), iter); err == nil {
			t.Errorf("iteration %d: want stabilization refusal, got nil", iter)
		} else if !strings.Contains(err.Error(), "observe first") {
			t.Errorf("iteration %d: error %q must keep the 'observe first' shape", iter, err)
		}
	}
	for _, iter := range []int{3, 4, 9} {
		if err := validateReviewCommand(validReviewCmd(), iter); err != nil {
			t.Errorf("iteration %d: want pass (window=3 default; a 30m reviewer must review before it dies), got %v", iter, err)
		}
	}
}

func TestReviewStabilization_EnvOverrideRestoresOldWindow(t *testing.T) {
	t.Setenv("REVIEWER_STABILIZATION_WINDOW", "10")
	if err := validateReviewCommand(validReviewCmd(), 9); err == nil {
		t.Error("iteration 9 with window=10: want refusal, got nil")
	}
	if err := validateReviewCommand(validReviewCmd(), 10); err != nil {
		t.Errorf("iteration 10 with window=10: want pass, got %v", err)
	}
}

func TestReviewStabilization_InvalidEnvFallsBackToDefault(t *testing.T) {
	for _, bad := range []string{"garbage", "-5", "3.7"} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("REVIEWER_STABILIZATION_WINDOW", bad)
			if err := validateReviewCommand(validReviewCmd(), 2); err == nil {
				t.Errorf("env %q, iteration 2: want default-window refusal (fail safe), got nil", bad)
			}
			if err := validateReviewCommand(validReviewCmd(), 3); err != nil {
				t.Errorf("env %q, iteration 3: want default-window pass, got %v", bad, err)
			}
		})
	}
}

func TestReviewStabilization_ZeroDisablesWindow(t *testing.T) {
	t.Setenv("REVIEWER_STABILIZATION_WINDOW", "0")
	if err := validateReviewCommand(validReviewCmd(), 0); err != nil {
		t.Errorf("iteration 0 with window=0: want pass (explicit operator disable), got %v", err)
	}
}
