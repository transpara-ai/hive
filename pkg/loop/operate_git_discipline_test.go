package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/work"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v15-F2: operate-instruction git discipline.
//
// Round 5's operate #2 hit a filename-case discrepancy between child-task
// specs and "fixed" it by amending the catalog commit and switching
// branches — moving HEAD to a non-descendant. The workspace integrity gate
// refused commit verification and halted the implementer: CORRECT (Michael:
// integrity halts stay terminal — integrity is constitutional, not
// weather). The missing half was instruction-layer: nothing ever told the
// operate provider that history is append-only in a round workspace.
//
// The discipline block is UNCONDITIONAL (every operate, every task — a
// constitution, not a per-task gate) and LAST (review feedback must never
// displace it).
// ════════════════════════════════════════════════════════════════════════

// disciplinePhrases are the load-bearing phrases every operate prompt must
// carry. Phrased as a slice so a partial rewrite of the block cannot
// silently drop one rule.
var disciplinePhrases = []string{
	"WORKSPACE GIT DISCIPLINE",
	"NEW commit on top",
	"NEVER amend",
	"rebase",
	"force-push",
	"NEVER switch branches",
	"wrong filename case",
	"integrity gate refuses",
}

func TestComposeOperateInstruction_AlwaysCarriesGitDiscipline(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	gates := []work.ArtifactEvent{{Label: "definition_of_done", Body: "DOD"}}
	reopens := []work.ReopenEvent{{Reason: "fix it", Issues: []string{"issue-1"}}}

	cases := map[string]string{
		"bare":          composeOperateInstruction(task, nil, nil),
		"with gates":    composeOperateInstruction(task, gates, nil),
		"with reopens":  composeOperateInstruction(task, nil, reopens),
		"gates+reopens": composeOperateInstruction(task, gates, reopens),
	}
	for name, got := range cases {
		for _, phrase := range disciplinePhrases {
			if !strings.Contains(got, phrase) {
				t.Errorf("%s: operate instruction lacks discipline phrase %q\n%s", name, phrase, got)
			}
		}
	}
}

func TestComposeOperateInstruction_DisciplineIsFinalSection(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	gates := []work.ArtifactEvent{{Label: "acceptance_criteria", Body: "AC"}}
	reopens := []work.ReopenEvent{{Reason: "r", Issues: []string{"i"}}}

	got := composeOperateInstruction(task, gates, reopens)
	idx := strings.Index(got, "WORKSPACE GIT DISCIPLINE")
	if idx < 0 {
		t.Fatal("discipline block missing")
	}
	if ri := strings.Index(got, "REVIEW FEEDBACK"); ri > idx {
		t.Errorf("REVIEW FEEDBACK appears after the discipline block; discipline must be the final section")
	}
	if ci := strings.Index(got, "READINESS CONTRACT"); ci > idx {
		t.Errorf("READINESS CONTRACT appears after the discipline block; discipline must be the final section")
	}
}
