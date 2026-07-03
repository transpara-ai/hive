package hive

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/transpara-ai/work"
	"github.com/transpara-ai/work/pkg/worklifecycle"
)

func TestTC6_AC7ivCivilizationAssemblyProjectionCarriesWorkCanonicalPair(t *testing.T) {
	tests := []struct {
		name             string
		status           work.TaskStatus
		wantBareStatus   string
		wantCanonical    bool
		wantErrorContain string
	}{
		{
			name:           "known status",
			status:         work.StatusRunning,
			wantBareStatus: "work_task_running",
			wantCanonical:  true,
		},
		{
			name:             "unknown future status",
			status:           "paused",
			wantBareStatus:   "work_task_paused",
			wantErrorContain: `unknown task status "paused"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, actorID, appendEvent := newOperatorProjectionStore(t)
			taskEvent := appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{
				Title:          "CWSM SP1b carry-through " + tc.name,
				CreatedBy:      actorID,
				FactoryOrderID: "fo_cwsm_sp1b_" + safeRunLaunchID(tc.name),
			})
			appendEvent(work.EventTypeTaskLifecycleTransitioned, work.TaskLifecycleTransitionContent{
				TaskID:    taskEvent.ID(),
				FromState: work.StatusCreated,
				ToState:   tc.status,
				Reason:    "exercise CWSM SP1b carry-through",
				ChangedBy: actorID,
			})

			taskStore := work.NewTaskStore(s, nil, nil)
			workProjection, _, err := civilizationAssemblyProjectWorkTask(taskStore, taskEvent.ID())
			if err != nil {
				t.Fatalf("work-side ProjectTask: %v", err)
			}
			projection := BuildCivilizationAssemblyProjection(s, 50)
			taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, taskEvent.ID().Value())
			if taskEvidence == nil {
				t.Fatalf("missing civilization task evidence for %s: %+v", taskEvent.ID().Value(), projection.WorkEvidenceSummary.Tasks)
			}

			if taskEvidence.Status != tc.wantBareStatus {
				t.Fatalf("bare status = %q, want %q", taskEvidence.Status, tc.wantBareStatus)
			}
			requireCivilizationAssemblyCanonicalPairEqual(t, tc.name, taskEvidence.Canonical, taskEvidence.CanonicalError, workProjection.Canonical, workProjection.CanonicalError)
			if tc.wantCanonical && taskEvidence.Canonical == nil {
				t.Fatal("canonical state was not carried for known status")
			}
			if !tc.wantCanonical && taskEvidence.Canonical != nil {
				t.Fatalf("canonical state = %+v, want nil for unknown status", taskEvidence.Canonical)
			}
			if tc.wantErrorContain != "" && !strings.Contains(taskEvidence.CanonicalError, tc.wantErrorContain) {
				t.Fatalf("canonical error = %q, want containing %q", taskEvidence.CanonicalError, tc.wantErrorContain)
			}
		})
	}
}

func requireCivilizationAssemblyCanonicalPairEqual(t *testing.T, label string, got *worklifecycle.CanonicalWorkState, gotError string, want *worklifecycle.CanonicalWorkState, wantError string) {
	t.Helper()
	if gotError != wantError {
		t.Fatalf("%s canonical error = %q, want %q", label, gotError, wantError)
	}
	if (got == nil) != (want == nil) {
		t.Fatalf("%s canonical nilness = got %t, want %t", label, got == nil, want == nil)
	}
	if got == nil {
		return
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s marshal got canonical: %v", label, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("%s marshal want canonical: %v", label, err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("%s canonical = %s, want %s", label, gotJSON, wantJSON)
	}
}
