package loop

import (
	"fmt"
	"strings"

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

// latestArtifactBody returns the body of the most-recently-attached artifact with
// the given label, or "" if none. ListArtifacts returns artifacts chronologically,
// so the last match is the latest attach.
func latestArtifactBody(artifacts []work.ArtifactEvent, label string) string {
	body := ""
	for _, a := range artifacts {
		if a.Label == label {
			body = a.Body
		}
	}
	return body
}

// operateInstruction composes the Operate prompt for task, pulling its readiness
// artifacts from the task store. A missing store or fetch error degrades to the
// title+description form rather than failing the Operate.
func (l *Loop) operateInstruction(task work.Task) string {
	var artifacts []work.ArtifactEvent
	if l.config.TaskStore != nil {
		if a, err := l.config.TaskStore.ListArtifacts(task.ID); err == nil {
			artifacts = a
		}
	}
	return composeOperateInstruction(task, artifacts)
}
