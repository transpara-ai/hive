package loop

import (
	"testing"

	"github.com/transpara-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// parseGapCommand
// ════════════════════════════════════════════════════════════════════════

func TestParseGapCommand_Valid(t *testing.T) {
	response := `I've identified a structural gap.
/gap {"category":"quality","missing_role":"reviewer","evidence":"3 tasks completed without review","severity":"medium"}
/signal {"signal": "IDLE"}`

	cmd := parseGapCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil GapCommand")
	}
	if cmd.Category != "quality" {
		t.Errorf("Category = %q, want %q", cmd.Category, "quality")
	}
	if cmd.MissingRole != "reviewer" {
		t.Errorf("MissingRole = %q, want %q", cmd.MissingRole, "reviewer")
	}
	if cmd.Evidence != "3 tasks completed without review" {
		t.Errorf("Evidence = %q, want %q", cmd.Evidence, "3 tasks completed without review")
	}
	if cmd.Severity != "medium" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "medium")
	}
}

func TestParseGapCommand_NoCommand(t *testing.T) {
	response := `Everything looks fine structurally.
/signal {"signal": "IDLE"}`

	cmd := parseGapCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseGapCommand_MalformedJSON(t *testing.T) {
	response := `/gap {not valid json`

	cmd := parseGapCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

func TestParseGapCommand_MultipleLines(t *testing.T) {
	response := `Analyzing the task flow...
Task stall pattern observed.
No current agent handles this.
/gap {"category":"operations","missing_role":"incident-commander","evidence":"cascading failures with no coordinated response","severity":"high"}
No further action needed.
/signal {"signal": "IDLE"}`

	cmd := parseGapCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil GapCommand")
	}
	if cmd.Category != "operations" {
		t.Errorf("Category = %q, want %q", cmd.Category, "operations")
	}
	if cmd.MissingRole != "incident-commander" {
		t.Errorf("MissingRole = %q, want %q", cmd.MissingRole, "incident-commander")
	}
	if cmd.Severity != "high" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "high")
	}
}

// ════════════════════════════════════════════════════════════════════════
// parseDirectiveCommand
// ════════════════════════════════════════════════════════════════════════

func TestParseDirectiveCommand_Valid(t *testing.T) {
	response := `Work agents need course correction.
/directive {"target":"strategist","action":"focus on test coverage before new features","reason":"3 bugs found in last sprint","priority":"high"}
/signal {"signal": "IDLE"}`

	cmd := parseDirectiveCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil DirectiveCommand")
	}
	if cmd.Target != "strategist" {
		t.Errorf("Target = %q, want %q", cmd.Target, "strategist")
	}
	if cmd.Action != "focus on test coverage before new features" {
		t.Errorf("Action = %q, want %q", cmd.Action, "focus on test coverage before new features")
	}
	if cmd.Reason != "3 bugs found in last sprint" {
		t.Errorf("Reason = %q, want %q", cmd.Reason, "3 bugs found in last sprint")
	}
	if cmd.Priority != "high" {
		t.Errorf("Priority = %q, want %q", cmd.Priority, "high")
	}
}

func TestParseDirectiveCommand_NoCommand(t *testing.T) {
	response := `No course correction needed.
/signal {"signal": "IDLE"}`

	cmd := parseDirectiveCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

// ════════════════════════════════════════════════════════════════════════
// validateGapCommand
// ════════════════════════════════════════════════════════════════════════

func TestValidateGapCommand_StabilizationBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // StabilizationWindow = 15
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "technical", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	// Iteration at or below stabilization window should be rejected.
	for _, iter := range []int{1, 5, 15} {
		err := validateGapCommand(cmd, iter, cooldowns, cfg)
		if err == nil {
			t.Errorf("iteration %d: expected error during stabilization window, got nil", iter)
		}
	}
}

func TestValidateGapCommand_CooldownBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // GapCooldown = 15
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "technical", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	// Record an emission at iteration 20.
	cooldowns.gapByCategory["technical"] = 20

	// Iteration 30 is only 10 iterations after — cooldown (15) not expired.
	err := validateGapCommand(cmd, 30, cooldowns, cfg)
	if err == nil {
		t.Error("expected cooldown error, got nil")
	}

	// Iteration 36 is 16 iterations after — cooldown expired.
	err = validateGapCommand(cmd, 36, cooldowns, cfg)
	if err != nil {
		t.Errorf("cooldown should have expired at iteration 36, got: %v", err)
	}
}

func TestValidateGapCommand_DedupBlocks(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "technical", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	// Mark "reviewer" as already emitted.
	cooldowns.emittedGaps["reviewer"] = true

	err := validateGapCommand(cmd, 20, cooldowns, cfg)
	if err == nil {
		t.Error("expected dedup error for already-emitted missing_role, got nil")
	}
}

func TestValidateGapCommand_InvalidCategory(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "invalid-category", MissingRole: "reviewer", Evidence: "test", Severity: "medium"}

	err := validateGapCommand(cmd, 20, cooldowns, cfg)
	if err == nil {
		t.Error("expected invalid category error, got nil")
	}
}

func TestValidateGapCommand_Valid(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &GapCommand{Category: "capability", MissingRole: "auditor", Evidence: "no security review process", Severity: "high"}

	// iteration 20 > StabilizationWindow (15), no cooldowns, category valid.
	err := validateGapCommand(cmd, 20, cooldowns, cfg)
	if err != nil {
		t.Errorf("expected valid gap command, got: %v", err)
	}
}

// ════════════════════════════════════════════════════════════════════════
// validateDirectiveCommand
// ════════════════════════════════════════════════════════════════════════

func TestValidateDirectiveCommand_StabilizationBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // StabilizationWindow = 15
	cooldowns := NewCTOCooldowns()
	cmd := &DirectiveCommand{Target: "strategist", Action: "slow down", Reason: "test", Priority: "low"}

	for _, iter := range []int{1, 10, 15} {
		err := validateDirectiveCommand(cmd, iter, cooldowns, cfg)
		if err == nil {
			t.Errorf("iteration %d: expected error during stabilization window, got nil", iter)
		}
	}
}

func TestValidateDirectiveCommand_CooldownBlocks(t *testing.T) {
	cfg := DefaultCTOConfig() // DirectiveCooldown = 5
	cooldowns := NewCTOCooldowns()
	cmd := &DirectiveCommand{Target: "strategist", Action: "slow down", Reason: "test", Priority: "low"}

	// Record an emission at iteration 20.
	cooldowns.directiveByTarget["strategist"] = 20

	// Iteration 24 is only 4 iterations after — cooldown (5) not expired.
	err := validateDirectiveCommand(cmd, 24, cooldowns, cfg)
	if err == nil {
		t.Error("expected cooldown error, got nil")
	}

	// Iteration 26 is 6 iterations after — cooldown expired.
	err = validateDirectiveCommand(cmd, 26, cooldowns, cfg)
	if err != nil {
		t.Errorf("cooldown should have expired at iteration 26, got: %v", err)
	}
}

func TestValidateDirectiveCommand_Valid(t *testing.T) {
	cfg := DefaultCTOConfig()
	cooldowns := NewCTOCooldowns()
	cmd := &DirectiveCommand{Target: "all", Action: "pause new task creation", Reason: "queue overloaded", Priority: "medium"}

	// iteration 20 > StabilizationWindow (15), no cooldowns.
	err := validateDirectiveCommand(cmd, 20, cooldowns, cfg)
	if err != nil {
		t.Errorf("expected valid directive command, got: %v", err)
	}
}

// ════════════════════════════════════════════════════════════════════════
// enrichCTOObservation
// ════════════════════════════════════════════════════════════════════════

func TestEnrichCTOObservation_NonCTO(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "guardian", "test-guardian")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichCTOObservation(obs)
	if result != obs {
		t.Errorf("non-cto enrichment should be identity, got %q", result)
	}
}

func TestEnrichCTOObservation_CTO(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "cto", "test-cto")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichCTOObservation(obs)
	if result == obs {
		t.Error("cto enrichment should add leadership briefing")
	}
	if !containsStr(result, "=== LEADERSHIP BRIEFING ===") {
		t.Error("missing LEADERSHIP BRIEFING header")
	}
	if !containsStr(result, "TASK FLOW:") {
		t.Error("missing TASK FLOW section")
	}
	if !containsStr(result, "HEALTH (from SysMon):") {
		t.Error("missing HEALTH section")
	}
	if !containsStr(result, "BUDGET (from Allocator):") {
		t.Error("missing BUDGET section")
	}
}
