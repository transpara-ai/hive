package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/work"
)

// TestComposeOperateInstruction guards the 4th production-driver fix: the
// implementer Operates as a headless subprocess seeing ONLY this instruction
// string. Round 1 passed it just title+description, so it never saw its
// acceptance_criteria and over-enumerated (46 roles vs the scoped 24). The
// instruction must carry the readiness contract (definition_of_done,
// acceptance_criteria, test_plan) in canonical order, ignore non-readiness
// artifacts, and stay backward-compatible when no gates are present.
func TestComposeOperateInstruction(t *testing.T) {
	task := work.Task{
		Title:       "Research and write the roles catalog",
		Description: "Produce dark-factory/civic-roles.md.",
	}
	// Deliberately out of canonical order, with a non-readiness artifact mixed in.
	artifacts := []work.ArtifactEvent{
		{Label: "Operate result", Body: "commit abc1234"},
		{Label: work.GateTestPlan, Body: "Diff the catalog role list against agentdef.go."},
		{Label: work.GateDefinitionOfDone, Body: "One markdown file superseding civic-roles.md."},
		{Label: work.GateAcceptanceCriteria, Body: "Enumerate EVERY role the cited source lists; remove unsourced roles."},
	}

	got := composeOperateInstruction(task, artifacts)

	if !strings.Contains(got, "Task: Research and write the roles catalog") {
		t.Errorf("instruction missing title; got:\n%s", got)
	}
	if !strings.Contains(got, "Produce dark-factory/civic-roles.md.") {
		t.Errorf("instruction missing description")
	}
	for _, want := range []string{
		"One markdown file superseding civic-roles.md.",
		"Enumerate EVERY role the cited source lists; remove unsourced roles.",
		"Diff the catalog role list against agentdef.go.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("instruction missing readiness body %q; got:\n%s", want, got)
		}
	}

	// Canonical order: DoD before AC before Test Plan, regardless of input order.
	iDoD := strings.Index(got, "Definition of Done")
	iAC := strings.Index(got, "Acceptance Criteria")
	iTP := strings.Index(got, "Test Plan")
	if iDoD < 0 || iAC < 0 || iTP < 0 || !(iDoD < iAC && iAC < iTP) {
		t.Errorf("readiness gates out of canonical order: DoD=%d AC=%d TP=%d", iDoD, iAC, iTP)
	}

	// A non-readiness artifact (the post-Operate result) must not leak in.
	if strings.Contains(got, "commit abc1234") {
		t.Errorf("non-readiness artifact leaked into the Operate instruction")
	}
}

// TestComposeOperateInstruction_NoGatesBackwardCompatible verifies the fallback
// is byte-identical to the original title+description form, so a task without a
// readiness contract behaves exactly as before.
func TestComposeOperateInstruction_NoGatesBackwardCompatible(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	if got := composeOperateInstruction(task, nil); got != "Task: T\n\nD" {
		t.Errorf("no-gate instruction must equal the original form %q, got %q", "Task: T\n\nD", got)
	}
}
