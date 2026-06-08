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
func composeOperateInstruction(task work.Task, artifacts []work.ArtifactEvent) string {
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

// artifactLister loads a task's readiness artifacts. *work.TaskStore satisfies it;
// tests substitute a failing implementation to exercise the fail-closed path.
type artifactLister interface {
	ListArtifacts(taskID types.EventID) ([]work.ArtifactEvent, error)
}

// operateInstructionFrom builds the Operate prompt for task, loading its readiness
// artifacts via lister. A nil lister (no task store configured) yields the
// title+description form. If the lister returns an error, it FAILS CLOSED: a store
// exists but its readiness contract cannot be loaded, so we cannot prove whether the
// task carries gates — Operating on title+description alone would let the implementer
// build blind to its acceptance_criteria (the exact failure the operate-instruction
// fold closes). Refuse and let the caller escalate rather than silently degrade.
func operateInstructionFrom(lister artifactLister, task work.Task) (string, error) {
	var artifacts []work.ArtifactEvent
	if lister != nil {
		a, err := lister.ListArtifacts(task.ID)
		if err != nil {
			return "", fmt.Errorf("load readiness artifacts for task %s: %w", task.ID, err)
		}
		artifacts = a
	}
	return composeOperateInstruction(task, artifacts), nil
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
