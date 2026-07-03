package hive

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPlanIssueScanSourceIssueMarkerAcquiredUsesProjectionBoundary(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition: IssueScanSourceIssueMarkerAcquired,
		Issue: GitHubIssueCandidate{
			Repo:   "transpara-ai/docs",
			Number: 256,
			Title:  "Factory-order acquisition marker and source-of-truth boundary",
			URL:    "https://github.com/transpara-ai/docs/issues/256",
			Labels: []string{IssueScanPRReadyLabel, "cc:civilization-presence"},
		},
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
		StageID:        "research_issue_and_repo_context",
		StageState:     "ready",
		ActorRole:      "dispatcher",
		WorkRefs:       []string{"work:task:tsk_issue_scan_docs_256_research"},
		EventGraphRefs: []string{"eventgraph:issuescan.run.projected:run_docs_256"},
		EvidenceRefs:   []string{"github:https://github.com/transpara-ai/docs/issues/256"},
		GeneratedAt:    time.Date(2026, 7, 3, 10, 45, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}

	if plan.Repo != "transpara-ai/docs" || plan.IssueNumber != 256 {
		t.Fatalf("target = %s#%d, want transpara-ai/docs#256", plan.Repo, plan.IssueNumber)
	}
	if len(plan.AddLabels) != 1 || plan.AddLabels[0] != IssueScanFactoryStatusLabelAcquired {
		t.Fatalf("add labels = %+v, want acquired factory label", plan.AddLabels)
	}
	for _, label := range append(append([]string(nil), plan.AddLabels...), plan.RemoveLabels...) {
		if strings.HasPrefix(label, "cc:") {
			t.Fatalf("marker plan mutates change-control label %q", label)
		}
	}
	for _, wantRemoved := range []string{
		IssueScanFactoryStatusLabelParked,
		IssueScanFactoryStatusLabelReadyForHuman,
		IssueScanFactoryStatusLabelCompleted,
		IssueScanFactoryStatusLabelAbandoned,
		IssueScanFactoryStatusLabelSuperseded,
	} {
		if !containsIssueScanValue(plan.RemoveLabels, wantRemoved) {
			t.Fatalf("remove labels = %+v, want %s", plan.RemoveLabels, wantRemoved)
		}
	}
	for _, want := range []string{
		"<!-- " + plan.IdempotencyKey + " -->",
		"Factory issue-scan marker: acquired",
		"run_id: `issue-scan-docs-256`",
		"factory_order_id: `fo_issue_scan_docs_256`",
		"stage_id: `research_issue_and_repo_context`",
		"work:task:tsk_issue_scan_docs_256_research",
		"eventgraph:issuescan.run.projected:run_docs_256",
		"Do not parse this comment as workflow state or authority.",
		"does not authorize protected actions",
	} {
		if !strings.Contains(plan.CommentBody, want) {
			t.Fatalf("comment body missing %q:\n%s", want, plan.CommentBody)
		}
	}
	if !IssueScanSourceIssueMarkerCommentExists([]string{"unrelated", plan.CommentBody}, plan) {
		t.Fatalf("planned marker was not detected by idempotency key")
	}
	if IssueScanSourceIssueMarkerCommentExists([]string{"unrelated"}, plan) {
		t.Fatalf("unrelated comments matched marker idempotency key")
	}
}

func TestPlanIssueScanSourceIssueMarkerHumanActionUsesParkedStatus(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerHumanAction,
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
		StageID:        "implement_on_branch",
		StageState:     "policy_blocked",
		EvidenceRefs:   []string{"github:https://github.com/transpara-ai/docs/issues/256"},
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}
	if len(plan.AddLabels) != 1 || plan.AddLabels[0] != IssueScanFactoryStatusLabelParked {
		t.Fatalf("add labels = %+v, want parked label for human-action marker", plan.AddLabels)
	}
	if !containsIssueScanValue(plan.RemoveLabels, IssueScanFactoryStatusLabelAcquired) {
		t.Fatalf("remove labels = %+v, want acquired removed when parking", plan.RemoveLabels)
	}
	if !strings.Contains(plan.CommentBody, "Factory issue-scan marker: human_action") || !strings.Contains(plan.CommentBody, "stage_state: `policy_blocked`") {
		t.Fatalf("human-action comment body missing transition/state:\n%s", plan.CommentBody)
	}
}

func TestPlanIssueScanSourceIssueMarkerRejectsIncompleteCanonicalRefs(t *testing.T) {
	_, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition: IssueScanSourceIssueMarkerAcquired,
		Issue:      markerTestIssue(),
		RunID:      "issue-scan-docs-256",
	})
	if err == nil || !strings.Contains(err.Error(), "factory_order_id is required") {
		t.Fatalf("missing factory order error = %v, want factory_order_id required", err)
	}

	_, err = PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerTransition("step_update"),
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown issue-scan source marker transition") {
		t.Fatalf("unknown transition error = %v", err)
	}
}

func TestApplyIssueScanSourceIssueMarkerAddsLabelsAndSkipsDuplicateComment(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerAcquired,
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}
	client := &fakeIssueScanMarkerClient{}
	result, err := ApplyIssueScanSourceIssueMarker(context.Background(), client, plan)
	if err != nil {
		t.Fatalf("ApplyIssueScanSourceIssueMarker first: %v", err)
	}
	if !result.CommentCreated || result.CommentSkipped {
		t.Fatalf("first apply result = %+v, want comment created", result)
	}
	if len(client.comments) != 1 || client.comments[0] != plan.CommentBody {
		t.Fatalf("comments = %+v, want one planned comment", client.comments)
	}
	if strings.Join(client.addedLabels, ",") != strings.Join(plan.AddLabels, ",") {
		t.Fatalf("added labels = %+v, want %+v", client.addedLabels, plan.AddLabels)
	}
	if strings.Join(client.removedLabels, ",") != strings.Join(plan.RemoveLabels, ",") {
		t.Fatalf("removed labels = %+v, want %+v", client.removedLabels, plan.RemoveLabels)
	}

	result, err = ApplyIssueScanSourceIssueMarker(context.Background(), client, plan)
	if err != nil {
		t.Fatalf("ApplyIssueScanSourceIssueMarker replay: %v", err)
	}
	if result.CommentCreated || !result.CommentSkipped {
		t.Fatalf("replay result = %+v, want comment skipped", result)
	}
	if len(client.comments) != 1 {
		t.Fatalf("comments after replay = %+v, want no duplicate", client.comments)
	}
}

func markerTestIssue() GitHubIssueCandidate {
	return GitHubIssueCandidate{
		Repo:   "transpara-ai/docs",
		Number: 256,
		Title:  "Factory-order acquisition marker and source-of-truth boundary",
		URL:    "https://github.com/transpara-ai/docs/issues/256",
		Labels: []string{IssueScanPRReadyLabel, "cc:civilization-presence"},
	}
}

type fakeIssueScanMarkerClient struct {
	addedLabels   []string
	removedLabels []string
	comments      []string
}

func (c *fakeIssueScanMarkerClient) AddLabels(_ context.Context, _ string, _ int, labels []string) error {
	c.addedLabels = append(c.addedLabels, labels...)
	return nil
}

func (c *fakeIssueScanMarkerClient) RemoveLabels(_ context.Context, _ string, _ int, labels []string) error {
	c.removedLabels = append(c.removedLabels, labels...)
	return nil
}

func (c *fakeIssueScanMarkerClient) ListCommentBodies(_ context.Context, _ string, _ int) ([]string, error) {
	return append([]string(nil), c.comments...), nil
}

func (c *fakeIssueScanMarkerClient) CreateComment(_ context.Context, _ string, _ int, body string) error {
	c.comments = append(c.comments, body)
	return nil
}
