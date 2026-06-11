package loop

import (
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// composeOperateInstruction builds the implementer's Operate prompt from the task
// title/description PLUS its readiness contract (definition_of_done,
// acceptance_criteria, test_plan). The implementer Operates as a headless
// subprocess seeing ONLY this string; passing it just title+description (the prior
// behavior) left it blind to the acceptance criteria the Planner attached, which
// let round 1 over-enumerate the roles catalog (46 roles vs the scoped 24). Gates
// are emitted in canonical order; non-readiness artifacts (e.g. the post-Operate
// "Operate result") are ignored. With no gates the output is byte-identical to the
// original title+description form (backward compatible).
func composeOperateInstruction(task work.Task, artifacts []work.ArtifactEvent, reopens []work.ReopenEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\n\n%s", task.Title, task.Description)

	gates := []struct{ label, heading string }{
		{work.GateDefinitionOfDone, "Definition of Done"},
		{work.GateAcceptanceCriteria, "Acceptance Criteria"},
		{work.GateTestPlan, "Test Plan"},
	}
	var contract strings.Builder
	for _, g := range gates {
		body := strings.TrimSpace(latestArtifactBody(artifacts, g.label))
		if body == "" {
			continue
		}
		fmt.Fprintf(&contract, "\n\n### %s\n%s", g.heading, body)
	}
	if contract.Len() > 0 {
		b.WriteString("\n\n== READINESS CONTRACT (your deliverable MUST satisfy ALL of the following) ==")
		b.WriteString(contract.String())
	}

	// Review feedback (run findings v12-F1): a reopened task arrives with the
	// reviewer's fix list — without it the producer would re-Operate on the
	// original instruction alone, blind to WHY its completion was rejected.
	// Bounded by the reviewer's per-task verdict cap; cumulative so a second
	// fix round still sees the first round's findings. With no reopens the
	// output is byte-identical to the prior form (backward compatible).
	if len(reopens) > 0 {
		b.WriteString("\n\n== REVIEW FEEDBACK (this task was REOPENED after review — your previous work on it was rejected; fix the issues below, then complete it again) ==")
		for i, r := range reopens {
			fmt.Fprintf(&b, "\n\nReopen %d — %s", i+1, r.Reason)
			for _, issue := range r.Issues {
				fmt.Fprintf(&b, "\n- %s", issue)
			}
		}
	}
	return b.String()
}

// latestArtifactBody returns the body of the most-recently-attached artifact whose
// label matches (after normalization), or "" if none. ListArtifacts returns artifacts
// chronologically, so the last match is the latest attach.
func latestArtifactBody(artifacts []work.ArtifactEvent, label string) string {
	want := normalizeGateLabel(label)
	body := ""
	for _, a := range artifacts {
		if normalizeGateLabel(a.Label) == want {
			body = a.Body
		}
	}
	return body
}

// normalizeGateLabel mirrors work.normalizeGateLabel (work/store.go): lower-case,
// trim, and fold "-"/" " to "_". Work readiness counts a gate present by this
// normalized form, so the Operate prompt must match the same way — otherwise a task
// that is Ready per Work (e.g. a "Acceptance Criteria" gate) would lose its body here
// and the implementer would Operate without its acceptance criteria. Keep in sync
// with work; a divergence reopens the silent-drop bug.
func normalizeGateLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	label = strings.ReplaceAll(label, "-", "_")
	label = strings.ReplaceAll(label, " ", "_")
	return label
}

// taskContextLister loads a task's readiness artifacts and reopen feedback.
// *work.TaskStore satisfies it; tests substitute failing implementations to
// exercise the fail-closed paths.
type taskContextLister interface {
	ListArtifacts(taskID types.EventID) ([]work.ArtifactEvent, error)
	ListReopens(taskID types.EventID) ([]work.ReopenEvent, error)
}

// operateInstructionFrom builds the Operate prompt for task, loading its readiness
// artifacts and reopen feedback via lister. A nil lister (no task store configured)
// yields the title+description form. If the lister returns an error, it FAILS
// CLOSED: a store exists but the task's context cannot be loaded, so we cannot
// prove whether the task carries gates or review feedback — Operating blind would
// let the implementer build past its acceptance_criteria, or re-do rejected work
// with no idea why it was rejected (run findings v12-F1). Refuse and let the
// caller escalate rather than silently degrade.
func operateInstructionFrom(lister taskContextLister, task work.Task) (string, error) {
	var artifacts []work.ArtifactEvent
	var reopens []work.ReopenEvent
	if lister != nil {
		a, err := lister.ListArtifacts(task.ID)
		if err != nil {
			return "", fmt.Errorf("load readiness artifacts for task %s: %w", task.ID, err)
		}
		artifacts = a
		r, err := lister.ListReopens(task.ID)
		if err != nil {
			return "", fmt.Errorf("load reopen feedback for task %s: %w", task.ID, err)
		}
		reopens = r
	}
	return composeOperateInstruction(task, artifacts, reopens), nil
}

// operateInstruction composes the Operate prompt for task, pulling its readiness
// artifacts from the task store. Returns an error (fail closed) when a configured
// store cannot load the artifacts.
func (l *Loop) operateInstruction(task work.Task) (string, error) {
	if l.config.TaskStore == nil {
		return operateInstructionFrom(nil, task)
	}
	return operateInstructionFrom(l.config.TaskStore, task)
}
