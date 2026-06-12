package loop

import (
	"errors"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// failingArtifactLister always errors, simulating a transient store/read failure.
type failingArtifactLister struct{ err error }

func (f failingArtifactLister) ListArtifacts(types.EventID) ([]work.ArtifactEvent, error) {
	return nil, f.err
}

func (failingArtifactLister) ListReopens(types.EventID) ([]work.ReopenEvent, error) {
	return nil, nil
}

// emptyArtifactLister returns no artifacts successfully (a task with no gates).
type emptyArtifactLister struct{}

func (emptyArtifactLister) ListArtifacts(types.EventID) ([]work.ArtifactEvent, error) {
	return nil, nil
}

func (emptyArtifactLister) ListReopens(types.EventID) ([]work.ReopenEvent, error) {
	return nil, nil
}

// TestOperateInstructionFailsClosedOnArtifactError guards Codex hive#133 P1: when a
// task store exists but its readiness artifacts cannot be loaded, the Operate prompt
// must FAIL CLOSED (error) instead of silently degrading to title+description — which
// would let the implementer Operate blind to its acceptance_criteria on a transient
// store error.
func TestOperateInstructionFailsClosedOnArtifactError(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	_, err := operateInstructionFrom(failingArtifactLister{err: errors.New("store unavailable")}, task)
	if err == nil {
		t.Fatal("operateInstructionFrom must fail closed when readiness artifacts cannot be loaded, got nil error")
	}
	if !strings.Contains(err.Error(), "store unavailable") {
		t.Errorf("error must wrap the underlying load failure, got: %v", err)
	}
}

// TestOperateInstructionSucceedsOnEmptyArtifacts: a successful (even empty) load is
// proof the task genuinely has no readiness contract, so the title+description form is
// correct and must NOT error (distinguishes "no gates" from "could not load gates").
func TestOperateInstructionSucceedsOnEmptyArtifacts(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	got, err := operateInstructionFrom(emptyArtifactLister{}, task)
	if err != nil {
		t.Fatalf("a successful empty load must not error: %v", err)
	}
	if got != "Task: T\n\nD\n\n"+operateGitDiscipline {
		t.Errorf("empty load must yield the title+description+discipline form, got %q", got)
	}
}

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

	got := composeOperateInstruction(task, artifacts, nil)

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
// is the title+description form plus exactly the standing git discipline —
// nothing else may sneak into a gateless task's prompt (v15-F2 made the
// discipline unconditional, so "backward compatible" now means this shape).
func TestComposeOperateInstruction_NoGatesBackwardCompatible(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	want := "Task: T\n\nD\n\n" + operateGitDiscipline
	if got := composeOperateInstruction(task, nil, nil); got != want {
		t.Errorf("no-gate instruction must equal title+description+discipline exactly, got %q", got)
	}
}

// TestComposeOperateInstruction_NormalizesGateLabels guards Codex hive#133 P2: Work
// readiness normalizes gate labels (lower/trim, "-"/" "->"_") before counting a gate
// present, so a task labeled "Acceptance Criteria" or "acceptance-criteria" is Ready
// per Work. An exact-match in the Operate prompt would silently drop those bodies, so
// a Ready task's implementer could still Operate without its acceptance criteria.
func TestComposeOperateInstruction_NormalizesGateLabels(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	artifacts := []work.ArtifactEvent{
		{Label: "Definition of Done", Body: "DOD_BODY"},
		{Label: "acceptance-criteria", Body: "AC_BODY"},
		{Label: "  Test Plan  ", Body: "TP_BODY"},
	}
	got := composeOperateInstruction(task, artifacts, nil)
	for _, want := range []string{"DOD_BODY", "AC_BODY", "TP_BODY"} {
		if !strings.Contains(got, want) {
			t.Errorf("gate body %q missing — spaced/hyphenated/cased labels must match like Work readiness; got:\n%s", want, got)
		}
	}
}
