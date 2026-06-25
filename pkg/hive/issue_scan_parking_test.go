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
