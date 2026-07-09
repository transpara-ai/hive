package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestProgressIssueScanLifecycleParksClosedTargetBeforeConfiguredRunners(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued := queueIssueScanParkingRun(t, rt, writer, 225)
	client := &fakeIssueScanMarkerClient{}
	rt.issueScanSourceIssueMarkerClient = client
	rt.issueScanSourceIssueMarkerActivation = mockedIssueScanSourceIssueMarkerActivation("transpara-ai/hive", 225)
	rt.issueScanTargetStateResolver = func(context.Context, string, int) (IssueScanTargetState, error) {
		return IssueScanTargetState{
			Repository:  "transpara-ai/hive",
			Number:      225,
			State:       "closed",
			StateReason: "completed",
			Labels:      []string{IssueScanPRReadyLabel},
		}, nil
	}

	runnerCalls := 0
	rt.issueScanStageRoleOutputRunner = func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error) {
		runnerCalls++
		return IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("parked run must not invoke configured runners")
	}

	progress, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
	if err != nil {
		t.Fatalf("ProgressIssueScanRunLifecycleWithConfiguredRunners: %v", err)
	}
	assertIssueScanParked(t, progress, IssueScanParkBlockerStaleTarget)
	if countReleasedIssueScanStageAdvances(progress.Advances) != 0 {
		t.Fatalf("advances = %+v, want none for parked run", progress.Advances)
	}
	if runnerCalls != 0 || len(progress.StageRoleOutputRuns) != 0 {
		t.Fatalf("runner calls/progress = %d/%+v, want none", runnerCalls, progress.StageRoleOutputRuns)
	}
	if _, ready, err := rt.RunConfiguredIssueScanStageRoleOutputRunner(context.Background(), queued.RunID); err != nil || ready {
		t.Fatalf("direct configured runner ready=%v err=%v, want parked run to be not ready without error", ready, err)
	}
	if runnerCalls != 0 {
		t.Fatalf("direct configured runner calls = %d, want none", runnerCalls)
	}
	if count := issueScanParkedEventCount(t, rt); count != 1 {
		t.Fatalf("parked event count = %d, want 1", count)
	}
	if len(progress.ParkedRuns) != 1 {
		t.Fatalf("parked runs = %+v, want one", progress.ParkedRuns)
	}
	marker := progress.ParkedRuns[0].SourceIssueMarker
	if marker.Transition != IssueScanSourceIssueMarkerParked || !marker.Applied || !marker.CommentCreated {
		t.Fatalf("source marker = %+v, want applied parked marker", marker)
	}
	if !containsIssueScanValue(marker.LabelsAdded, IssueScanFactoryStatusLabelParked) {
		t.Fatalf("marker labels added = %+v, want parked label", marker.LabelsAdded)
	}
	if len(client.comments) != 2 || !strings.Contains(client.comments[1], "Factory issue-scan marker: parked") {
		t.Fatalf("client comments = %+v, want parked marker", client.comments)
	}
	addedCount := len(client.addedLabels)
	removedCount := len(client.removedLabels)

	again, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
	if err != nil {
		t.Fatalf("second ProgressIssueScanRunLifecycleWithConfiguredRunners: %v", err)
	}
	if len(again.ParkedRuns) != 1 || !again.ParkedRuns[0].AlreadyParked {
		t.Fatalf("second parked runs = %+v, want one already-parked result", again.ParkedRuns)
	}
	if count := issueScanParkedEventCount(t, rt); count != 1 {
		t.Fatalf("parked event count after second pass = %d, want 1", count)
	}
	if len(again.ParkedRuns) != 1 || !again.ParkedRuns[0].SourceIssueMarker.CommentSkipped {
		t.Fatalf("second parked marker = %+v, want duplicate comment skipped", again.ParkedRuns)
	}
	if len(client.comments) != 2 {
		t.Fatalf("comments after parked replay = %+v, want no duplicate", client.comments)
	}
	if len(client.addedLabels) != addedCount || len(client.removedLabels) != removedCount {
		t.Fatalf("label mutations changed on parked replay: added %d->%d removed %d->%d", addedCount, len(client.addedLabels), removedCount, len(client.removedLabels))
	}
}

func TestParkedIssueScanRunStopsDirectLifecycleHelpers(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued := queueIssueScanParkingRun(t, rt, writer, 225)
	rt.issueScanTargetStateResolver = func(context.Context, string, int) (IssueScanTargetState, error) {
		return IssueScanTargetState{
			Repository:  "transpara-ai/hive",
			Number:      225,
			State:       "closed",
			StateReason: "completed",
			Labels:      []string{IssueScanPRReadyLabel},
		}, nil
	}

	progress, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
	if err != nil {
		t.Fatalf("ProgressIssueScanRunLifecycleWithConfiguredRunners: %v", err)
	}
	assertIssueScanParked(t, progress, IssueScanParkBlockerStaleTarget)
	dispatch := RunLaunchDispatchResult{AlreadyDispatchedIssueScanRunIDs: []string{queued.RunID}}
	if advances, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil || len(advances) != 0 {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages parked = %+v, %v; want no advances and no error", advances, err)
	}
	if completions, err := rt.CompleteReadyIssueScanLifecycleStages(dispatch); err != nil || len(completions) != 0 {
		t.Fatalf("CompleteReadyIssueScanLifecycleStages parked = %+v, %v; want no completions and no error", completions, err)
	}
	if task, ready, err := rt.EnsureIssueScanImplementationTask(queued.RunID); err != nil || ready {
		t.Fatalf("EnsureIssueScanImplementationTask parked = %+v ready=%v err=%v; want no-op", task, ready, err)
	}
	if recorded, ready, err := rt.RecordCompletedIssueScanImplementationRoleOutput(queued.RunID); err != nil || ready {
		t.Fatalf("RecordCompletedIssueScanImplementationRoleOutput parked = %+v ready=%v err=%v; want no-op", recorded, ready, err)
	}
	if recorded, ready, err := rt.RecordCompletedIssueScanReviewRoleOutput(queued.RunID); err != nil || ready || len(recorded) != 0 {
		t.Fatalf("RecordCompletedIssueScanReviewRoleOutput parked = %+v ready=%v err=%v; want no-op", recorded, ready, err)
	}
	if recorded, ready, err := rt.RecordCompletedIssueScanBlockerRoleOutput(queued.RunID); err != nil || ready || len(recorded) != 0 {
		t.Fatalf("RecordCompletedIssueScanBlockerRoleOutput parked = %+v ready=%v err=%v; want no-op", recorded, ready, err)
	}
	if recorded, ready, err := rt.RecordCompletedIssueScanReadyRoleOutput(queued.RunID); err != nil || ready || len(recorded) != 0 {
		t.Fatalf("RecordCompletedIssueScanReadyRoleOutput parked = %+v ready=%v err=%v; want no-op", recorded, ready, err)
	}
	if _, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, ""); err == nil || !strings.Contains(err.Error(), "is parked") {
		t.Fatalf("AdvanceIssueScanLifecycleStage parked error = %v, want parked refusal", err)
	}
	if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, "research_issue_and_repo_context", IssueScanStageRuntimeEvidence{}, false); err == nil || !strings.Contains(err.Error(), "is parked") {
		t.Fatalf("CompleteIssueScanLifecycleStage parked error = %v, want parked refusal", err)
	}
}

func TestProgressIssueScanLifecycleParksWhenSourceIssueMarkerClientFails(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued := queueIssueScanParkingRun(t, rt, writer, 225)
	rt.issueScanSourceIssueMarkerClient = &failingIssueScanMarkerClient{err: errors.New("github marker unavailable")}
	rt.issueScanSourceIssueMarkerActivation = mockedIssueScanSourceIssueMarkerActivation("transpara-ai/hive", 225)
	rt.issueScanTargetStateResolver = func(context.Context, string, int) (IssueScanTargetState, error) {
		return IssueScanTargetState{
			Repository:  "transpara-ai/hive",
			Number:      225,
			State:       "closed",
			StateReason: "completed",
			Labels:      []string{IssueScanPRReadyLabel},
		}, nil
	}
	runnerCalls := 0
	rt.issueScanStageRoleOutputRunner = func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error) {
		runnerCalls++
		return IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("parked run must not invoke configured runners")
	}

	progress, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
	if err == nil || !strings.Contains(err.Error(), "github marker unavailable") {
		t.Fatalf("ProgressIssueScanRunLifecycleWithConfiguredRunners error = %v, want surfaced marker error", err)
	}
	assertIssueScanParked(t, progress, IssueScanParkBlockerStaleTarget)
	if runnerCalls != 0 || len(progress.StageRoleOutputRuns) != 0 {
		t.Fatalf("runner calls/progress = %d/%+v, want none", runnerCalls, progress.StageRoleOutputRuns)
	}
	if count := issueScanParkedEventCount(t, rt); count != 1 {
		t.Fatalf("parked event count = %d, want 1 despite marker projection error", count)
	}
}

func TestIssueScanRunParkedScansBeyondProjectionPage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	requestEvent := appendValidatedRunLaunch(t, rt.store, writer, nil)
	runIDs := make([]string, 0, defaultOperatorProjectionLimit+5)
	for i := 0; i < defaultOperatorProjectionLimit+5; i++ {
		runID := fmt.Sprintf("run_parked_%03d", i)
		runIDs = append(runIDs, runID)
		if _, err := rt.recordIssueScanRunParked(runID, issueScanRunParkingDecision{
			FactoryOrderID:   fmt.Sprintf("fo_issue_scan_parked_%03d", i),
			Repository:       "transpara-ai/hive",
			IssueNumber:      225 + i,
			BlockerType:      IssueScanParkBlockerStaleTarget,
			Detail:           "target is closed",
			RequiredAction:   "queue a fresh run against a live target",
			TargetIssueState: "closed",
			SourceRefs:       []string{requestEvent.ID().Value()},
		}); err != nil {
			t.Fatalf("recordIssueScanRunParked %d: %v", i, err)
		}
	}

	firstPage, err := rt.store.ByType(EventTypeIssueScanRunParked, defaultOperatorProjectionLimit, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType first page: %v", err)
	}
	firstPageRunIDs := map[string]struct{}{}
	for _, ev := range firstPage.Items() {
		content, ok := ev.Content().(IssueScanRunParkedContent)
		if ok {
			firstPageRunIDs[content.RunID] = struct{}{}
		}
	}
	targetRunID := ""
	for _, runID := range runIDs {
		if _, seen := firstPageRunIDs[runID]; !seen {
			targetRunID = runID
			break
		}
	}
	if targetRunID == "" {
		t.Fatalf("expected at least one parked run beyond first page of %d", defaultOperatorProjectionLimit)
	}

	content, eventID, ok, err := rt.issueScanRunParked(targetRunID)
	if err != nil {
		t.Fatalf("issueScanRunParked: %v", err)
	}
	if !ok {
		t.Fatalf("issueScanRunParked did not find %s after more than one projection page", targetRunID)
	}
	if eventID.IsZero() {
		t.Fatal("issueScanRunParked returned zero event id")
	}
	if content.RunID != targetRunID {
		t.Fatalf("parked run = %q; want %q", content.RunID, targetRunID)
	}
}

func TestProgressIssueScanLifecycleParksHumanScopeAndProtectedLabels(t *testing.T) {
	for _, tc := range []struct {
		name        string
		label       string
		blockerType string
	}{
		{name: "human_scope", label: IssueScanNeedsHumanScopeLabel, blockerType: IssueScanParkBlockerHumanScope},
		{name: "deferred", label: IssueScanPRDeferredLabel, blockerType: IssueScanParkBlockerHumanScope},
		{name: "protected", label: IssueScanProtectedActionLabel, blockerType: IssueScanParkBlockerProtectedAction},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, writer := newRunLaunchDispatchRuntime(t)
			queued := queueIssueScanParkingRun(t, rt, writer, 225)
			client := &fakeIssueScanMarkerClient{}
			rt.issueScanSourceIssueMarkerClient = client
			rt.issueScanSourceIssueMarkerActivation = mockedIssueScanSourceIssueMarkerActivation("transpara-ai/hive", 225)
			rt.issueScanTargetStateResolver = func(context.Context, string, int) (IssueScanTargetState, error) {
				return IssueScanTargetState{
					Repository: "transpara-ai/hive",
					Number:     225,
					State:      "open",
					Labels:     []string{IssueScanPRReadyLabel, tc.label},
				}, nil
			}

			runnerCalls := 0
			rt.issueScanStageRoleOutputRunner = func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error) {
				runnerCalls++
				return IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("parked run must not invoke configured runners")
			}

			progress, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
			if err != nil {
				t.Fatalf("ProgressIssueScanRunLifecycleWithConfiguredRunners: %v", err)
			}
			assertIssueScanParked(t, progress, tc.blockerType)
			if runnerCalls != 0 || len(progress.StageRoleOutputRuns) != 0 {
				t.Fatalf("runner calls/progress = %d/%+v, want none", runnerCalls, progress.StageRoleOutputRuns)
			}
			if len(progress.ParkedRuns) != 1 {
				t.Fatalf("parked runs = %+v, want one", progress.ParkedRuns)
			}
			marker := progress.ParkedRuns[0].SourceIssueMarker
			if marker.Transition != IssueScanSourceIssueMarkerHumanAction || !marker.Applied || !marker.CommentCreated {
				t.Fatalf("source marker = %+v, want applied human_action marker", marker)
			}
			if !containsIssueScanValue(marker.LabelsAdded, IssueScanFactoryStatusLabelParked) {
				t.Fatalf("marker labels added = %+v, want parked label", marker.LabelsAdded)
			}
			if len(client.comments) != 2 || !strings.Contains(client.comments[1], "Factory issue-scan marker: human_action") {
				t.Fatalf("client comments = %+v, want human_action marker", client.comments)
			}
		})
	}
}

func TestProgressIssueScanLifecycleResolverFailureFailsClosed(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued := queueIssueScanParkingRun(t, rt, writer, 225)
	rt.issueScanTargetStateResolver = func(context.Context, string, int) (IssueScanTargetState, error) {
		return IssueScanTargetState{}, errors.New("github unavailable")
	}
	runnerCalls := 0
	rt.issueScanStageRoleOutputRunner = func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error) {
		runnerCalls++
		return IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("resolver failure must not invoke configured runners")
	}

	progress, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
	if err == nil || !strings.Contains(err.Error(), "issue-scan run parking") {
		t.Fatalf("error = %v, want issue-scan parking error", err)
	}
	if len(progress.ParkedRuns) != 0 {
		t.Fatalf("parked runs = %+v, want none when park event could not be recorded", progress.ParkedRuns)
	}
	if countReleasedIssueScanStageAdvances(progress.Advances) != 0 {
		t.Fatalf("advances = %+v, want none after fail-closed resolver error", progress.Advances)
	}
	if runnerCalls != 0 || len(progress.StageRoleOutputRuns) != 0 {
		t.Fatalf("runner calls/progress = %d/%+v, want none", runnerCalls, progress.StageRoleOutputRuns)
	}
}

func TestProgressIssueScanLifecycleParksDuplicateCanonicalStageChain(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued := queueIssueScanParkingRun(t, rt, writer, 225)
	initial, err := rt.ProgressIssueScanRunLifecycleContext(context.Background(), queued.RunID)
	if err != nil {
		t.Fatalf("initial ProgressIssueScanRunLifecycleContext: %v", err)
	}
	if countReleasedIssueScanStageAdvances(initial.Advances) != 1 {
		t.Fatalf("initial advances = %+v, want first stage released before duplicate injection", initial.Advances)
	}

	requests, err := fetchFactoryRunRequestedEventByRunID(rt.store, queued.RunID)
	if err != nil {
		t.Fatalf("fetchFactoryRunRequestedEventByRunID: %v", err)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		t.Fatalf("request content has type %T, want FactoryRunRequestedContent", requests[0].Content())
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, factoryOrderFromRunLaunch(content, orderID))
	if err != nil {
		t.Fatalf("issueScanLifecycleStageTaskDrafts: %v", err)
	}
	if len(drafts) == 0 {
		t.Fatal("issue-scan lifecycle produced no stage drafts")
	}
	duplicateOptions := drafts[0].Options
	duplicateOptions.Title = "Duplicate canonical stage task"
	if _, err := rt.tasks.CreateV39(writer.human, duplicateOptions, []types.EventID{requests[0].ID()}, writer.conv); err != nil {
		t.Fatalf("create duplicate stage task: %v", err)
	}

	runnerCalls := 0
	rt.issueScanStageRoleOutputRunner = func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error) {
		runnerCalls++
		return IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("duplicate-chain parked run must not invoke configured runners")
	}
	progress, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), queued.RunID)
	if err != nil {
		t.Fatalf("ProgressIssueScanRunLifecycleWithConfiguredRunners: %v", err)
	}
	assertIssueScanParked(t, progress, IssueScanParkBlockerDuplicateChain)
	if progress.ParkedRuns[0].StageID != drafts[0].StageID {
		t.Fatalf("parked stage = %q, want %q", progress.ParkedRuns[0].StageID, drafts[0].StageID)
	}
	if runnerCalls != 0 || len(progress.StageRoleOutputRuns) != 0 {
		t.Fatalf("runner calls/progress = %d/%+v, want none", runnerCalls, progress.StageRoleOutputRuns)
	}
}

func queueIssueScanParkingRun(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, number int) IssueScanRunLaunchResult {
	t.Helper()
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: number,
			Title:  "Park unsafe issue-scan runs",
			URL:    fmt.Sprintf("https://github.com/transpara-ai/hive/issues/%d", number),
			Body:   "The Civilization should park unsafe issue-scan runs without burning worker tokens.",
			Labels: []string{"civilization", "issue-scan", IssueScanPRReadyLabel},
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	return queued
}

func assertIssueScanParked(t *testing.T, progress IssueScanLifecycleProgress, blockerType string) {
	t.Helper()
	if len(progress.ParkedRuns) != 1 {
		t.Fatalf("parked runs = %+v, want one", progress.ParkedRuns)
	}
	parked := progress.ParkedRuns[0]
	if !parked.Parked || parked.AlreadyParked {
		t.Fatalf("parked result = %+v, want newly parked run", parked)
	}
	if parked.BlockerType != blockerType {
		t.Fatalf("blocker type = %q, want %q (result %+v)", parked.BlockerType, blockerType, parked)
	}
	if parked.RequiredAction == "" {
		t.Fatalf("parked result missing required action: %+v", parked)
	}
}

func issueScanParkedEventCount(t *testing.T, rt *Runtime) int {
	t.Helper()
	events, err := eventsByTypePaginated(rt.store, EventTypeIssueScanRunParked, defaultOperatorProjectionLimit)
	if err != nil {
		t.Fatalf("eventsByTypePaginated parked: %v", err)
	}
	return len(events)
}
