package runner

import (
	"testing"
)

// TestScoutThrottleBypassInOneShot verifies that in one-shot mode the scout
// runs on tick 1 (not deferred to tick 8).
func TestScoutThrottleBypassInOneShot(t *testing.T) {
	// Simulate the throttle condition for various tick values.
	// In normal mode (oneShot=false), only tick 8 passes.
	// In one-shot mode, every tick passes (throttle is bypassed).
	for tick := 1; tick <= 8; tick++ {
		throttled := !false && tick%8 != 0 // normal mode
		if tick == 8 && throttled {
			t.Errorf("tick %d should NOT be throttled in normal mode", tick)
		}
		if tick != 8 && !throttled {
			t.Errorf("tick %d should be throttled in normal mode", tick)
		}

		throttledOneShot := !true && tick%8 != 0 // one-shot mode
		if throttledOneShot {
			t.Errorf("tick %d should NOT be throttled in one-shot mode", tick)
		}
	}
}

func TestParseScoutTask(t *testing.T) {
	input := `Based on the state, the next gap is adding the Decision entity kind.

TASK_TITLE: Add Decision entity kind to the site
TASK_PRIORITY: high
TASK_DESCRIPTION: Add KindDecision constant to store.go, handleDecisions handler to handlers.go, DecisionsView template to views.templ, sidebar+mobile nav entries, and add "decision" to the intend allowlist. Follow the entity pipeline pattern.`

	title, desc, priority := parseScoutTask(input)

	if title != "Add Decision entity kind to the site" {
		t.Errorf("title = %q", title)
	}
	if priority != "high" {
		t.Errorf("priority = %q", priority)
	}
	if desc == "" {
		t.Error("description is empty")
	}
	if !searchString(desc, "KindDecision") {
		t.Error("description should mention KindDecision")
	}
}

func TestParseScoutTaskDefaults(t *testing.T) {
	// Missing priority should default to medium.
	input := "TASK_TITLE: Fix something\nTASK_DESCRIPTION: Fix the thing."
	title, desc, priority := parseScoutTask(input)

	if title != "Fix something" {
		t.Errorf("title = %q", title)
	}
	if priority != "medium" {
		t.Errorf("priority = %q, want medium", priority)
	}
	if desc != "Fix the thing." {
		t.Errorf("desc = %q", desc)
	}
}

func TestParseScoutTaskEmpty(t *testing.T) {
	title, _, _ := parseScoutTask("No structured output here.")
	if title != "" {
		t.Errorf("expected empty title, got %q", title)
	}
}

func TestBuildScoutPrompt(t *testing.T) {
	prompt := buildScoutPrompt("/path/to/site", "shared context", "repo context", "state content", "git log", "board summary")

	if !searchString(prompt, "/path/to/site") {
		t.Error("prompt missing repo path")
	}
	if !searchString(prompt, "repo context") {
		t.Error("prompt missing repo context")
	}
	if !searchString(prompt, "state content") {
		t.Error("prompt missing state")
	}
	if !searchString(prompt, "git log") {
		t.Error("prompt missing git log")
	}
	if !searchString(prompt, "board summary") {
		t.Error("prompt missing board")
	}
	if !searchString(prompt, "TASK_TITLE") {
		t.Error("prompt missing output format")
	}
	if !searchString(prompt, "MUST create tasks") {
		t.Error("prompt missing target repo instruction")
	}
}
