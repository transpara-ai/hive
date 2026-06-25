package hive

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
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
	if stage.ContextType != issueScanProbeContextTypeName(t, reflect.TypeOf(IssueScanStageRoleOutputRunnerContext{})) {
		t.Fatalf("stage role context type = %q", stage.ContextType)
	}
	if stage.StageID != issueScanResearchStageID || stage.StageTaskID == "" {
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
	if implementation.ContextType != issueScanProbeContextTypeName(t, reflect.TypeOf(IssueScanImplementationRunnerContext{})) || implementation.ContextKind != issueScanImplementationRunnerContextKind {
		t.Fatalf("implementation context metadata = %+v", implementation)
	}
	review := requireIssueScanProbe(t, doc, "adversarial_review_runner")
	if review.ContextType != issueScanProbeContextTypeName(t, reflect.TypeOf(IssueScanAdversarialReviewContext{})) || review.ContextKind != issueScanAdversarialReviewContextKind {
		t.Fatalf("review context metadata = %+v", review)
	}
	blocker := requireIssueScanProbe(t, doc, "blocker_repair_runner")
	if blocker.ContextType != issueScanProbeContextTypeName(t, reflect.TypeOf(IssueScanBlockerRepairRunnerContext{})) || blocker.ContextKind != issueScanBlockerRepairRunnerContextKind {
		t.Fatalf("blocker context metadata = %+v", blocker)
	}
	readyPR := requireIssueScanProbe(t, doc, "ready_pr_evidence_runner")
	if readyPR.ContextType != issueScanProbeContextTypeName(t, reflect.TypeOf(IssueScanReadyPRRunnerContext{})) || readyPR.ContextKind != issueScanReadyPRRunnerContextKind {
		t.Fatalf("ready PR context metadata = %+v", readyPR)
	}
	readyState := requireIssueScanProbe(t, doc, "ready_state_review_runner")
	if readyState.Ready || readyState.GeneratedBy != "managed_ready_pr_finalizer" {
		t.Fatalf("ready-state probe = %+v, want managed nested context", readyState)
	}
	if readyState.ContextType != issueScanProbeContextTypeName(t, reflect.TypeOf(IssueScanReadyStateReviewContext{})) || readyState.ContextKind != issueScanReadyStateReviewContextKind {
		t.Fatalf("ready-state context metadata = %+v", readyState)
	}
}

func TestProbeIssueScanRunnerContextsMaterializesOnlyScaffoldingAndIsIdempotent(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 216,
			Title:  "Pin probe scaffolding side effects",
			URL:    "https://github.com/transpara-ai/hive/issues/216",
			Labels: []string{"cc:pr-ready"},
		}},
		Budget: RunLaunchBudget{MaxIterations: 30, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}

	before := issueScanProbeStoreCountsForTest(t, rt)
	if before.WorkTasks != 0 || before.WorkArtifacts != 0 || before.WorkDependencies != 0 || before.WorkLifecycleTransitions != 0 {
		t.Fatalf("queued run should not have Work scaffolding before probe: %+v", before)
	}
	if before.RunnerOutputArtifacts != 0 || before.Reviews != 0 || before.DraftPRReceipts != 0 || before.ReadyPREvidence != 0 {
		t.Fatalf("queued run should not have runner results before probe: %+v", before)
	}

	if _, err := rt.ProbeIssueScanRunnerContexts(queued.RunID, false); err != nil {
		t.Fatalf("first ProbeIssueScanRunnerContexts: %v", err)
	}
	afterFirst := issueScanProbeStoreCountsForTest(t, rt)
	if afterFirst.WorkTasks <= before.WorkTasks || afterFirst.WorkArtifacts <= before.WorkArtifacts || afterFirst.WorkDependencies <= before.WorkDependencies {
		t.Fatalf("first probe did not materialize expected task scaffolding: before=%+v after=%+v", before, afterFirst)
	}
	if afterFirst.RunnerOutputArtifacts != 0 || afterFirst.Reviews != 0 || afterFirst.AuthorityRequests != 0 || afterFirst.AuthorityDecisions != 0 || afterFirst.DraftPRReceipts != 0 || afterFirst.ReadyPREvidence != 0 {
		t.Fatalf("probe recorded forbidden runner/authority/PR outputs: %+v", afterFirst)
	}

	if _, err := rt.ProbeIssueScanRunnerContexts(queued.RunID, false); err != nil {
		t.Fatalf("second ProbeIssueScanRunnerContexts: %v", err)
	}
	afterSecond := issueScanProbeStoreCountsForTest(t, rt)
	if afterSecond != afterFirst {
		t.Fatalf("second probe should be idempotent: first=%+v second=%+v", afterFirst, afterSecond)
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

func TestProbeIssueScanRunnerContextsHonorsCanceledContext(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 217,
			Title:  "Cancel runner context probe",
			URL:    "https://github.com/transpara-ai/hive/issues/217",
			Labels: []string{"cc:pr-ready"},
		}},
		Budget: RunLaunchBudget{MaxIterations: 30, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = rt.ProbeIssueScanRunnerContextsContext(ctx, queued.RunID, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ProbeIssueScanRunnerContextsContext error = %v, want context.Canceled", err)
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

type issueScanProbeStoreCounts struct {
	WorkTasks                int
	WorkArtifacts            int
	WorkDependencies         int
	WorkLifecycleTransitions int
	RunnerOutputArtifacts    int
	Reviews                  int
	AuthorityRequests        int
	AuthorityDecisions       int
	DraftPRReceipts          int
	ReadyPREvidence          int
}

func issueScanProbeStoreCountsForTest(t *testing.T, rt *Runtime) issueScanProbeStoreCounts {
	t.Helper()
	return issueScanProbeStoreCounts{
		WorkTasks:                issueScanProbeEventCountForTest(t, rt, work.EventTypeTaskCreated),
		WorkArtifacts:            issueScanProbeEventCountForTest(t, rt, work.EventTypeTaskArtifact),
		WorkDependencies:         issueScanProbeEventCountForTest(t, rt, work.EventTypeTaskDependencyAdded),
		WorkLifecycleTransitions: issueScanProbeEventCountForTest(t, rt, work.EventTypeTaskLifecycleTransitioned),
		RunnerOutputArtifacts:    issueScanProbeTaskArtifactLabelCountForTest(t, rt, IssueScanStageRoleOutputArtifactLabel),
		Reviews:                  issueScanProbeEventCountForTest(t, rt, event.EventTypeCodeReviewSubmitted),
		AuthorityRequests:        issueScanProbeEventCountForTest(t, rt, EventTypeAuthorityRequestRecorded),
		AuthorityDecisions:       issueScanProbeEventCountForTest(t, rt, EventTypeAuthorityDecisionRecorded),
		DraftPRReceipts:          issueScanProbeTaskArtifactLabelCountForTest(t, rt, TransparaAIDraftPRReceiptArtifactLabel),
		ReadyPREvidence:          issueScanProbeTaskArtifactLabelCountForTest(t, rt, IssueScanReadyPREvidenceArtifactLabel),
	}
}

func issueScanProbeEventCountForTest(t *testing.T, rt *Runtime, eventType types.EventType) int {
	t.Helper()
	page, err := rt.store.ByType(eventType, 1000, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("query %s: %v", eventType, err)
	}
	return len(page.Items())
}

func issueScanProbeTaskArtifactLabelCountForTest(t *testing.T, rt *Runtime, label string) int {
	t.Helper()
	page, err := rt.store.ByType(work.EventTypeTaskArtifact, 1000, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("query %s: %v", work.EventTypeTaskArtifact, err)
	}
	count := 0
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskArtifactContent)
		if ok && strings.TrimSpace(content.Label) == label {
			count++
		}
	}
	return count
}

func issueScanProbeContextTypeName(t *testing.T, typ reflect.Type) string {
	t.Helper()
	if typ.PkgPath() != "github.com/transpara-ai/hive/pkg/hive" {
		t.Fatalf("unexpected context package path %q for %s", typ.PkgPath(), typ)
	}
	return "hive." + typ.Name()
}
