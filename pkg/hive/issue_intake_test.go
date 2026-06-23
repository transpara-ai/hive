package hive

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

func TestQueueIssueScanRunLaunchDispatchesFactoryOrder(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	req := IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{
			{
				Repo:   "transpara-ai/hive",
				Number: 321,
				Title:  "Teach the Civilization to scan issues",
				URL:    "https://github.com/transpara-ai/hive/issues/321",
				Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
				Labels: []string{"civilization", "autonomy"},
			},
		},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}

	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, req, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if queued.Selected.Repo != "transpara-ai/hive" || queued.Selected.Number != 321 {
		t.Fatalf("selected issue = %+v, want transpara-ai/hive#321", queued.Selected)
	}
	if queued.IntakeID != "intake_github_issue_transpara_ai_hive_321" {
		t.Fatalf("intake id = %q", queued.IntakeID)
	}

	requestEvents := requireRunLaunchEvents(t, rt.store, EventTypeFactoryRunRequested, 1)
	request := requestEvents[0]
	content := request.Content().(FactoryRunRequestedContent)
	if content.IntakeID != queued.IntakeID || content.OperatorID != "operator_michael_saucier" {
		t.Fatalf("request content ids = %+v, queued=%+v", content, queued)
	}
	if content.Authority.InitialLevel != event.AuthorityLevelRequired {
		t.Fatalf("authority level = %q, want Required", content.Authority.InitialLevel)
	}
	if content.TargetRepos[0] != "transpara-ai/hive" {
		t.Fatalf("target repos = %+v", content.TargetRepos)
	}
	if len(content.Sources) != 1 || content.Sources[0].Type != issueScanSourceType || content.Sources[0].Ref != "https://github.com/transpara-ai/hive/issues/321" {
		t.Fatalf("sources = %+v", content.Sources)
	}

	var brief struct {
		Kind                string   `json:"kind"`
		RequiredAgentFlow   []string `json:"required_agent_flow"`
		AuthorityBoundaries []string `json:"authority_boundaries"`
		SelectedIssue       struct {
			Repo   string `json:"repo"`
			Number int    `json:"number"`
			Title  string `json:"title"`
		} `json:"selected_issue"`
	}
	if err := json.Unmarshal(content.Brief, &brief); err != nil {
		t.Fatalf("unmarshal brief: %v", err)
	}
	if brief.Kind != "transpara_ai_github_issue_scan" || brief.SelectedIssue.Repo != "transpara-ai/hive" || brief.SelectedIssue.Number != 321 {
		t.Fatalf("brief = %+v", brief)
	}
	if !containsIssueScanValue(brief.RequiredAgentFlow, "run_adversarial_review") || !containsIssueScanValue(brief.RequiredAgentFlow, "surface_ready_for_Human_result_PR") {
		t.Fatalf("required agent flow missing review/ready PR: %+v", brief.RequiredAgentFlow)
	}
	if !containsIssueScanValue(brief.AuthorityBoundaries, "no_merge") || !containsIssueScanValue(brief.AuthorityBoundaries, "ready_for_Human_result_PR_only") {
		t.Fatalf("authority boundaries = %+v", brief.AuthorityBoundaries)
	}

	result, err := rt.DispatchQueuedRunLaunches(10)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunches: %v", err)
	}
	if result.Dispatched != 1 || result.Failed != 0 {
		t.Fatalf("dispatch result = %+v, want one dispatched and no failures", result)
	}
	tasks, err := rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if task.FactoryOrderID != result.DispatchedOrderIDs[0] {
		t.Fatalf("task factory order = %q, want %q", task.FactoryOrderID, result.DispatchedOrderIDs[0])
	}
	if !strings.Contains(task.Description, "https://github.com/transpara-ai/hive/issues/321") {
		t.Fatalf("task description does not include issue URL: %s", task.Description)
	}
	storedTask, err := rt.store.Get(task.ID)
	if err != nil {
		t.Fatalf("get task event: %v", err)
	}
	if causes := storedTask.Causes(); len(causes) != 1 || causes[0] != request.ID() {
		t.Fatalf("task causes = %+v, want factory.run.requested %s", causes, request.ID())
	}
}

func TestQueueIssueScanRunLaunchRejectsNonTransparaAIRepo(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	_, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: "operator_michael",
		Issues: []GitHubIssueCandidate{{
			Repo:   "example/hive",
			Number: 1,
			Title:  "wrong org",
		}},
		Budget: RunLaunchBudget{MaxIterations: 1, MaxCostUSD: 0},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("error = %v, want transpara-ai repo rejection", err)
	}
}

func TestIssueScanOperatorIDIsRunLaunchSafe(t *testing.T) {
	got := IssueScanOperatorID("Michael Saucier")
	if got != "operator_michael_saucier" {
		t.Fatalf("IssueScanOperatorID = %q", got)
	}
}

func containsIssueScanValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
