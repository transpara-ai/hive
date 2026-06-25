package hive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProbeIssueScanRunnerContextsReportsPlanningContext(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 214,
			Title:  "Probe issue-scan runner contexts",
			URL:    "https://github.com/transpara-ai/hive/issues/214",
			Body:   "Expose the current stored runner context before invoking external commands.",
			Labels: []string{"cc:pr-ready", "civilization"},
		}},
		Budget: RunLaunchBudget{MaxIterations: 30, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}

	doc, err := rt.ProbeIssueScanRunnerContexts(queued.RunID, true)
	if err != nil {
		t.Fatalf("ProbeIssueScanRunnerContexts: %v", err)
	}
	if doc.Kind != issueScanRunnerContextProbeKind || doc.LifecycleVersion != issueScanLifecycleVersion {
		t.Fatalf("probe metadata = %+v", doc)
	}
	if doc.RunID != queued.RunID || doc.FactoryOrderID == "" || doc.Repository != "transpara-ai/hive" {
		t.Fatalf("probe run identity = %+v, queued=%+v", doc, queued)
	}
	if doc.ReadyCount != 1 || doc.ErrorCount != 0 {
		t.Fatalf("ready/error counts = %d/%d, contexts=%+v", doc.ReadyCount, doc.ErrorCount, doc.Contexts)
	}

	stage := requireIssueScanProbe(t, doc, "stage_role_output_runner")
	if !stage.Ready || stage.ContextKind != issueScanStageRoleOutputRunnerContextKind {
		t.Fatalf("stage role probe = %+v", stage)
	}
	if stage.StageID != "research_issue_and_repo_context" || stage.StageTaskID == "" {
		t.Fatalf("stage role target = %+v", stage)
	}
	if !strings.Contains(stage.StandaloneCommand, "run-issue-scan-stage-role-output") {
		t.Fatalf("stage role standalone command = %q", stage.StandaloneCommand)
	}
	var stageContext IssueScanStageRoleOutputRunnerContext
	if err := json.Unmarshal(stage.ContextPayload, &stageContext); err != nil {
		t.Fatalf("decode stage role context payload: %v", err)
	}
	if stageContext.Kind != issueScanStageRoleOutputRunnerContextKind || stageContext.StageID != stage.StageID || stageContext.Repository != "transpara-ai/hive" {
		t.Fatalf("stage role context = %+v, probe=%+v", stageContext, stage)
	}
	if len(stageContext.RequestedRoleSteps) == 0 {
		t.Fatalf("stage role context requested no role steps: %+v", stageContext)
	}

	implementation := requireIssueScanProbe(t, doc, "implementation_runner")
	if implementation.Ready || implementation.NotReadyReason == "" {
		t.Fatalf("implementation probe = %+v, want not-ready reason", implementation)
	}
	readyState := requireIssueScanProbe(t, doc, "ready_state_review_runner")
	if readyState.Ready || readyState.GeneratedBy != "managed_ready_pr_finalizer" {
		t.Fatalf("ready-state probe = %+v, want managed nested context", readyState)
	}
}

func TestProbeIssueScanRunnerContextsOmitsPayloadWhenNotRequested(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 215,
			Title:  "Omit probe payloads by default",
			URL:    "https://github.com/transpara-ai/hive/issues/215",
			Labels: []string{"cc:pr-ready"},
		}},
		Budget: RunLaunchBudget{MaxIterations: 30, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}

	doc, err := rt.ProbeIssueScanRunnerContexts(queued.RunID, false)
	if err != nil {
		t.Fatalf("ProbeIssueScanRunnerContexts: %v", err)
	}
	stage := requireIssueScanProbe(t, doc, "stage_role_output_runner")
	if !stage.Ready {
		t.Fatalf("stage role probe = %+v, want ready", stage)
	}
	if len(stage.ContextPayload) != 0 {
		t.Fatalf("context payload should be omitted when includePayload=false: %s", string(stage.ContextPayload))
	}
}

func TestProbeIssueScanRunnerContextsMissingRunFails(t *testing.T) {
	rt, _ := newRunLaunchDispatchRuntime(t)
	_, err := rt.ProbeIssueScanRunnerContexts("run_missing", false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ProbeIssueScanRunnerContexts error = %v, want not found", err)
	}
}

func requireIssueScanProbe(t *testing.T, doc IssueScanRunnerContextProbeDocument, id string) IssueScanRunnerContextProbe {
	t.Helper()
	for _, probe := range doc.Contexts {
		if probe.ID == id {
			return probe
		}
	}
	t.Fatalf("missing probe %q in %+v", id, doc.Contexts)
	return IssueScanRunnerContextProbe{}
}
