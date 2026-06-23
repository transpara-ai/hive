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
		Kind                 string                       `json:"kind"`
		LifecycleVersion     string                       `json:"lifecycle_version"`
		RequiredAgentFlow    []string                     `json:"required_agent_flow"`
		DevelopmentLifecycle []issueScanBriefStageForTest `json:"development_lifecycle"`
		AgentExecutionPlan   []issueScanBriefPlanForTest  `json:"agent_execution_plan"`
		AuthorityBoundaries  []string                     `json:"authority_boundaries"`
		SelectedIssue        struct {
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
	if brief.LifecycleVersion != "civilization_issue_to_human_ready_pr_v0.3" {
		t.Fatalf("lifecycle version = %q", brief.LifecycleVersion)
	}
	if !containsIssueScanValue(brief.RequiredAgentFlow, "run_adversarial_review") || !containsIssueScanValue(brief.RequiredAgentFlow, "surface_ready_for_Human_result_PR") {
		t.Fatalf("required agent flow missing review/ready PR: %+v", brief.RequiredAgentFlow)
	}
	expectedStageIDs := []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
		"implement_on_branch",
		"run_adversarial_review",
		"drive_blockers_to_zero",
		"surface_ready_for_Human_result_PR",
	}
	if got := strings.Join(brief.RequiredAgentFlow, ","); got != strings.Join(expectedStageIDs, ",") {
		t.Fatalf("required agent flow = %+v, want %+v", brief.RequiredAgentFlow, expectedStageIDs)
	}
	if len(brief.DevelopmentLifecycle) != len(expectedStageIDs) {
		t.Fatalf("development lifecycle length = %d, want %d", len(brief.DevelopmentLifecycle), len(expectedStageIDs))
	}
	for i, stage := range brief.DevelopmentLifecycle {
		if stage.ID != expectedStageIDs[i] {
			t.Fatalf("stage[%d].id = %q, want %q", i, stage.ID, expectedStageIDs[i])
		}
	}
	roles := StarterRoleDefinitions()
	for _, stage := range brief.DevelopmentLifecycle {
		for _, role := range stage.RequiredRoles {
			if roles[role] == nil {
				t.Fatalf("stage %q requires unknown role %q", stage.ID, role)
			}
		}
	}
	reviewStage := issueScanStageByID(brief.DevelopmentLifecycle, "run_adversarial_review")
	if reviewStage == nil {
		t.Fatalf("development lifecycle missing adversarial review stage: %+v", brief.DevelopmentLifecycle)
	}
	if !containsIssueScanValue(reviewStage.RequiredRoles, "reviewer") || !containsIssueScanValue(reviewStage.RequiredRoles, "guardian") {
		t.Fatalf("review stage roles = %+v", reviewStage.RequiredRoles)
	}
	if !containsIssueScanValue(reviewStage.RequiredEvidence, "exact_head_review_artifact") || !containsIssueScanValue(reviewStage.RequiredEvidence, "finding_disposition") {
		t.Fatalf("review stage evidence = %+v", reviewStage.RequiredEvidence)
	}
	readyStage := issueScanStageByID(brief.DevelopmentLifecycle, "surface_ready_for_Human_result_PR")
	if readyStage == nil {
		t.Fatalf("development lifecycle missing ready-for-Human stage: %+v", brief.DevelopmentLifecycle)
	}
	if readyStage.AuthorityBoundary != "human_approval_required_no_merge" {
		t.Fatalf("ready stage authority boundary = %q", readyStage.AuthorityBoundary)
	}
	if !containsIssueScanValue(readyStage.RequiredRoles, "reviewer") {
		t.Fatalf("ready stage roles = %+v, want reviewer for ready-state review evidence", readyStage.RequiredRoles)
	}
	if !containsIssueScanValue(readyStage.RequiredEvidence, "ready_pr_url") || containsIssueScanValue(readyStage.RequiredEvidence, "draft_pr_url") {
		t.Fatalf("ready stage evidence = %+v, want ready_pr_url and no draft_pr_url", readyStage.RequiredEvidence)
	}
	implementStage := issueScanStageByID(brief.DevelopmentLifecycle, "implement_on_branch")
	if implementStage == nil {
		t.Fatalf("development lifecycle missing implementation stage: %+v", brief.DevelopmentLifecycle)
	}
	if !containsIssueScanValue(implementStage.RequiredRoles, "implementer") || !roles["implementer"].CanOperate {
		t.Fatalf("implementation stage roles = %+v, implementer role = %+v", implementStage.RequiredRoles, roles["implementer"])
	}
	if len(brief.AgentExecutionPlan) != 18 {
		t.Fatalf("agent execution plan length = %d, want 18: %+v", len(brief.AgentExecutionPlan), brief.AgentExecutionPlan)
	}
	firstPlan := brief.AgentExecutionPlan[0]
	if firstPlan.StageID != "research_issue_and_repo_context" || firstPlan.Role != "strategist" || firstPlan.CanOperate {
		t.Fatalf("first agent plan step = %+v, want non-operating strategist research step", firstPlan)
	}
	implementPlan := issueScanPlanStepByRole(brief.AgentExecutionPlan, "implement_on_branch", "implementer")
	if implementPlan == nil || !implementPlan.CanOperate || !containsIssueScanValue(implementPlan.RequiredOutputs, "validation_output") {
		t.Fatalf("implementer plan step = %+v", implementPlan)
	}
	reviewPlan := issueScanPlanStepByRole(brief.AgentExecutionPlan, "run_adversarial_review", "reviewer")
	if reviewPlan == nil || reviewPlan.CanOperate || !containsIssueScanValue(reviewPlan.RequiredOutputs, "exact_head_review_artifact") {
		t.Fatalf("reviewer plan step = %+v", reviewPlan)
	}
	guardianReadyPlan := issueScanPlanStepByRole(brief.AgentExecutionPlan, "surface_ready_for_Human_result_PR", "guardian")
	if guardianReadyPlan == nil || guardianReadyPlan.AuthorityBoundary != "human_approval_required_no_merge" || !containsIssueScanValue(guardianReadyPlan.RequiredOutputs, "human_approval_boundary_check") {
		t.Fatalf("guardian ready plan step = %+v", guardianReadyPlan)
	}
	for _, step := range brief.AgentExecutionPlan {
		role := roles[step.Role]
		if role == nil {
			t.Fatalf("agent plan step %q requires unknown role %q", step.ID, step.Role)
		}
		if role.CanOperate != step.CanOperate {
			t.Fatalf("agent plan step %q can_operate=%v, role=%+v", step.ID, step.CanOperate, role)
		}
		if issueScanStageByID(brief.DevelopmentLifecycle, step.StageID) == nil {
			t.Fatalf("agent plan step %q references missing stage %q", step.ID, step.StageID)
		}
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
	if !containsIssueScanValue(task.ExpectedOutputs, "ready-for-Human result pull request") {
		t.Fatalf("task expected outputs = %+v, want ready-for-Human result pull request", task.ExpectedOutputs)
	}
	if !containsIssueScanValue(task.ExpectedOutputs, "exact-head adversarial review with zero blockers") {
		t.Fatalf("task expected outputs = %+v, want exact-head adversarial review with zero blockers", task.ExpectedOutputs)
	}
	if strings.Contains(strings.Join(task.ExpectedOutputs, "\n"), "draft pull request") {
		t.Fatalf("task expected outputs = %+v, must not advertise draft PR as final output", task.ExpectedOutputs)
	}
	if !strings.Contains(task.Description, "https://github.com/transpara-ai/hive/issues/321") {
		t.Fatalf("task description does not include issue URL: %s", task.Description)
	}
	if !strings.Contains(task.Description, "\"agent_execution_plan\"") || !strings.Contains(task.Description, "human_approval_boundary_check") {
		t.Fatalf("task description does not include agent execution plan: %s", task.Description)
	}
	storedTask, err := rt.store.Get(task.ID)
	if err != nil {
		t.Fatalf("get task event: %v", err)
	}
	if causes := storedTask.Causes(); len(causes) != 1 || causes[0] != request.ID() {
		t.Fatalf("task causes = %+v, want factory.run.requested %s", causes, request.ID())
	}

	artifacts, err := rt.tasks.ListArtifacts(task.ID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	var planArtifactBody string
	var planArtifactID string
	planArtifactIndex := -1
	for i, artifact := range artifacts {
		if artifact.Label == IssueScanExecutionPlanArtifactLabel {
			if artifact.TaskID != task.ID {
				t.Fatalf("artifact task id = %s, want %s", artifact.TaskID, task.ID)
			}
			if artifact.MediaType != issueScanExecutionPlanArtifactMediaType {
				t.Fatalf("artifact media type = %q, want %q", artifact.MediaType, issueScanExecutionPlanArtifactMediaType)
			}
			if artifact.CreatedBy != writer.human {
				t.Fatalf("artifact created_by = %s, want %s", artifact.CreatedBy, writer.human)
			}
			planArtifactBody = artifact.Body
			planArtifactID = artifact.ID.Value()
			planArtifactIndex = i
		}
	}
	if planArtifactBody == "" {
		t.Fatalf("missing %s artifact in %+v", IssueScanExecutionPlanArtifactLabel, artifacts)
	}
	var artifactBrief struct {
		Kind                 string                       `json:"kind"`
		LifecycleVersion     string                       `json:"lifecycle_version"`
		RequiredAgentFlow    []string                     `json:"required_agent_flow"`
		DevelopmentLifecycle []issueScanBriefStageForTest `json:"development_lifecycle"`
		AgentExecutionPlan   []issueScanBriefPlanForTest  `json:"agent_execution_plan"`
		AuthorityBoundaries  []string                     `json:"authority_boundaries"`
	}
	if err := json.Unmarshal([]byte(planArtifactBody), &artifactBrief); err != nil {
		t.Fatalf("unmarshal execution plan artifact: %v", err)
	}
	if artifactBrief.Kind != issueScanBriefKind || artifactBrief.LifecycleVersion != issueScanLifecycleVersion {
		t.Fatalf("artifact brief identity = %+v", artifactBrief)
	}
	if !containsIssueScanValue(artifactBrief.RequiredAgentFlow, "surface_ready_for_Human_result_PR") {
		t.Fatalf("artifact required agent flow = %+v, want ready-for-Human PR stage", artifactBrief.RequiredAgentFlow)
	}
	if issueScanStageByID(artifactBrief.DevelopmentLifecycle, "run_adversarial_review") == nil {
		t.Fatalf("artifact lifecycle missing adversarial review stage: %+v", artifactBrief.DevelopmentLifecycle)
	}
	if issueScanPlanStepByRole(artifactBrief.AgentExecutionPlan, "surface_ready_for_Human_result_PR", "guardian") == nil {
		t.Fatalf("artifact agent execution plan missing guardian ready-for-Human step: %+v", artifactBrief.AgentExecutionPlan)
	}
	if !containsIssueScanValue(artifactBrief.AuthorityBoundaries, "no_merge") {
		t.Fatalf("artifact authority boundaries = %+v, want no_merge", artifactBrief.AuthorityBoundaries)
	}
	storedArtifact, err := rt.store.Get(artifacts[planArtifactIndex].ID)
	if err != nil {
		t.Fatalf("get artifact event: %v", err)
	}
	if storedArtifact.ID().Value() != planArtifactID {
		t.Fatalf("stored artifact id = %s, want %s", storedArtifact.ID(), planArtifactID)
	}
	if causes := storedArtifact.Causes(); len(causes) != 2 || causes[0] != request.ID() || causes[1] != task.ID {
		t.Fatalf("artifact causes = %+v, want request %s and task %s", causes, request.ID(), task.ID)
	}
}

func TestIssueScanAgentExecutionPlanCoversEveryRequiredStageRole(t *testing.T) {
	lifecycle := issueScanDevelopmentLifecycle()
	plan, err := issueScanAgentExecutionPlan(lifecycle)
	if err != nil {
		t.Fatalf("issueScanAgentExecutionPlan: %v", err)
	}

	byStage := map[string]map[string]bool{}
	for _, step := range plan {
		if byStage[step.StageID] == nil {
			byStage[step.StageID] = map[string]bool{}
		}
		if byStage[step.StageID][step.Role] {
			t.Fatalf("duplicate step for stage %q role %q", step.StageID, step.Role)
		}
		byStage[step.StageID][step.Role] = true
	}
	for _, stage := range lifecycle {
		if len(byStage[stage.ID]) != len(stage.RequiredRoles) {
			t.Fatalf("stage %q plan roles = %+v, want %+v", stage.ID, byStage[stage.ID], stage.RequiredRoles)
		}
		for _, role := range stage.RequiredRoles {
			if !byStage[stage.ID][role] {
				t.Fatalf("stage %q missing role %q in plan %+v", stage.ID, role, plan)
			}
		}
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

type issueScanBriefStageForTest struct {
	ID                string   `json:"id"`
	RequiredRoles     []string `json:"required_roles"`
	RequiredEvidence  []string `json:"required_evidence"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

type issueScanBriefPlanForTest struct {
	ID                string   `json:"id"`
	StageID           string   `json:"stage_id"`
	Role              string   `json:"role"`
	CanOperate        bool     `json:"can_operate"`
	RequiredInputs    []string `json:"required_inputs"`
	RequiredOutputs   []string `json:"required_outputs"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

func issueScanStageByID(stages []issueScanBriefStageForTest, id string) *issueScanBriefStageForTest {
	for i := range stages {
		if stages[i].ID == id {
			return &stages[i]
		}
	}
	return nil
}

func issueScanPlanStepByRole(steps []issueScanBriefPlanForTest, stageID, role string) *issueScanBriefPlanForTest {
	for i := range steps {
		if steps[i].StageID == stageID && steps[i].Role == role {
			return &steps[i]
		}
	}
	return nil
}
