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
// "Operate result") are ignored. Every instruction ends with the standing
// workspace git discipline (v15-F2) — the one section present regardless of
// gates or reopens.
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
	// fix round still sees the first round's findings.
	if len(reopens) > 0 {
		b.WriteString("\n\n== REVIEW FEEDBACK (this task was REOPENED after review — your previous work on it was rejected; fix the issues below, then complete it again) ==")
		for i, r := range reopens {
			fmt.Fprintf(&b, "\n\nReopen %d — %s", i+1, r.Reason)
			for _, issue := range r.Issues {
				fmt.Fprintf(&b, "\n- %s", issue)
			}
		}
	}

	// Workspace git discipline (v15-F2): UNCONDITIONAL and LAST. Round 5's
	// operate #2 "fixed" a filename-case discrepancy by amending the
	// delivered commit on a switched branch; the integrity gate refused
	// commit verification and halted the implementer — correctly, but the
	// instruction layer had never stated the append-only contract the gate
	// enforces. Every operate now carries it, after every per-task section,
	// so no readiness contract or review feedback can displace it.
	b.WriteString("\n\n" + operateGitDiscipline)
	return b.String()
}

// operateGitDiscipline is the standing workspace law every Operate prompt
// ends with (v15-F2). It names the exact v15 incident (a filename-case fix
// done as an amend on a switched branch) so the provider recognizes the
// temptation, and states the consequence the integrity gate enforces.
const operateGitDiscipline = `== WORKSPACE GIT DISCIPLINE (constitutional — the workspace integrity gate enforces this) ==
Work on the CURRENT branch at its CURRENT HEAD. Stack every change as a NEW commit on top.
- NEVER amend, rebase, reset, cherry-pick, or force-push — existing commits are immutable history.
- NEVER switch branches, create branches, or detach HEAD.
- To correct an earlier commit — wrong content, wrong filename case, anything — make a NEW commit on top with the fix.
History rewrites move HEAD off the verified lineage; the integrity gate refuses them and HALTS you for human review. A stacked fix commit is always the right move.`

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
