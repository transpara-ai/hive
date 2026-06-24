package hive

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
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
		SelectionPolicy      struct {
			PolicyID       string   `json:"policy_id"`
			SelectedRank   int      `json:"selected_rank"`
			CandidateCount int      `json:"candidate_count"`
			RankingInputs  []string `json:"ranking_inputs"`
			Rationale      string   `json:"rationale"`
		} `json:"selection_policy"`
		SelectedIssue struct {
			Rank   int    `json:"rank"`
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
	if brief.LifecycleVersion != "civilization_issue_to_human_ready_pr_v0.4" {
		t.Fatalf("lifecycle version = %q", brief.LifecycleVersion)
	}
	if brief.SelectedIssue.Rank != 1 || brief.SelectionPolicy.PolicyID != "deterministic_actionability_rank_v0.2" || brief.SelectionPolicy.SelectedRank != 1 || brief.SelectionPolicy.CandidateCount != 1 {
		t.Fatalf("selection policy = %+v selected=%+v", brief.SelectionPolicy, brief.SelectedIssue)
	}
	if !containsIssueScanValue(brief.SelectionPolicy.RankingInputs, "actionability_keyword_score") || !strings.Contains(brief.SelectionPolicy.Rationale, "actionability signals") {
		t.Fatalf("selection policy rationale/inputs = %+v", brief.SelectionPolicy)
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
	if len(result.DispatchedStageTaskIDs) != len(expectedStageIDs) {
		t.Fatalf("stage task ids = %+v, want %d lifecycle stage task drafts", result.DispatchedStageTaskIDs, len(expectedStageIDs))
	}
	tasks, err := rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(tasks) != 1+len(expectedStageIDs) {
		t.Fatalf("task count = %d, want parent plus %d stage drafts: %+v", len(tasks), len(expectedStageIDs), tasks)
	}
	taskIndex := -1
	for i := range tasks {
		if tasks[i].ID == result.DispatchedTaskIDs[0] {
			taskIndex = i
			break
		}
	}
	if taskIndex < 0 {
		t.Fatalf("parent task %s not found in %+v", result.DispatchedTaskIDs[0], tasks)
	}
	task := tasks[taskIndex]
	if task.FactoryOrderID != result.DispatchedOrderIDs[0] {
		t.Fatalf("task factory order = %q, want %q", task.FactoryOrderID, result.DispatchedOrderIDs[0])
	}
	order := work.FactoryOrder{
		ID:                     task.FactoryOrderID,
		RiskClass:              task.RiskClass,
		RequirementIDs:         append([]string(nil), task.RequirementIDs...),
		AcceptanceCriterionIDs: append([]string(nil), task.AcceptanceCriterionIDs...),
	}
	stageDrafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		t.Fatalf("issueScanLifecycleStageTaskDrafts: %v", err)
	}
	stageDraftsByStage := map[string]issueScanLifecycleStageTaskDraft{}
	for _, draft := range stageDrafts {
		stageDraftsByStage[draft.StageID] = draft
	}
	expectedOutputs := []string{
		"ready-for-Human result pull request",
		"exact-head adversarial review with zero blockers",
		"validation evidence and operator-facing status update",
	}
	if len(task.ExpectedOutputs) != len(expectedOutputs) {
		t.Fatalf("task expected outputs = %+v, want exactly %+v", task.ExpectedOutputs, expectedOutputs)
	}
	for _, expected := range expectedOutputs {
		if !containsIssueScanValue(task.ExpectedOutputs, expected) {
			t.Fatalf("task expected outputs = %+v, want %q", task.ExpectedOutputs, expected)
		}
	}
	for _, rendered := range []struct {
		field string
		value string
	}{
		{field: "expected outputs", value: strings.Join(task.ExpectedOutputs, "\n")},
		{field: "description", value: task.Description},
	} {
		lower := strings.ToLower(rendered.value)
		if strings.Contains(lower, "draft") || strings.Contains(lower, "governed execution artifact") {
			t.Fatalf("task %s = %q, must not advertise draft or governed execution artifact as final output", rendered.field, rendered.value)
		}
	}
	if !strings.Contains(task.Description, "https://github.com/transpara-ai/hive/issues/321") {
		t.Fatalf("task description does not include issue URL: %s", task.Description)
	}
	if !strings.Contains(task.Description, "\"agent_execution_plan\"") || !strings.Contains(task.Description, "human_approval_boundary_check") {
		t.Fatalf("task description does not include agent execution plan: %s", task.Description)
	}
	stageTaskIDsByStage := map[string]types.EventID{}
	var previousStageTaskID types.EventID
	wantStageCells := map[string]string{
		"research_issue_and_repo_context":   "planning",
		"debate_with_correct_civic_roles":   "planning",
		"select_and_design_approach":        "planning",
		"run_adversarial_review":            "review",
		"surface_ready_for_Human_result_PR": "governance",
	}
	for i, stageID := range expectedStageIDs {
		wantCanonicalID := issueScanLifecycleStageTaskCanonicalID(task.FactoryOrderID, stageID)
		stageTaskIndex := -1
		for j := range tasks {
			if tasks[j].CanonicalTaskID == wantCanonicalID {
				stageTaskIndex = j
				break
			}
		}
		if stageTaskIndex < 0 {
			t.Fatalf("missing stage task %q in %+v", wantCanonicalID, tasks)
		}
		stageTask := tasks[stageTaskIndex]
		if stageTask.FactoryOrderID != task.FactoryOrderID {
			t.Fatalf("stage task %q factory order = %q, want %q", stageID, stageTask.FactoryOrderID, task.FactoryOrderID)
		}
		if strings.Join(stageTask.RequirementIDs, "\n") != strings.Join(task.RequirementIDs, "\n") {
			t.Fatalf("stage task %q requirement ids = %+v, want parent ids %+v", stageID, stageTask.RequirementIDs, task.RequirementIDs)
		}
		if strings.Join(stageTask.AcceptanceCriterionIDs, "\n") != strings.Join(task.AcceptanceCriterionIDs, "\n") {
			t.Fatalf("stage task %q acceptance criterion ids = %+v, want parent ids %+v", stageID, stageTask.AcceptanceCriterionIDs, task.AcceptanceCriterionIDs)
		}
		wantCell := wantStageCells[stageID]
		if wantCell == "" {
			wantCell = "implementation"
		}
		if stageTask.Cell != wantCell {
			t.Fatalf("stage task %q cell = %q, want %q", stageID, stageTask.Cell, wantCell)
		}
		if !strings.Contains(stageTask.Description, "Stage ID: "+stageID) {
			t.Fatalf("stage task %q description = %s", stageID, stageTask.Description)
		}
		if !containsIssueScanValue(stageTask.ExpectedOutputs, "stage declaration artifact remains pending runtime evidence") {
			t.Fatalf("stage task %q expected outputs = %+v, want declaration boundary output", stageID, stageTask.ExpectedOutputs)
		}
		readiness, err := rt.tasks.Readiness(stageTask.ID)
		if err != nil {
			t.Fatalf("Readiness %s: %v", stageTask.ID, err)
		}
		if !readiness.Ready {
			t.Fatalf("stage task %q readiness = %+v, want ready with stage gates", stageID, readiness)
		}
		stageTaskArtifacts, err := rt.tasks.ListArtifacts(stageTask.ID)
		if err != nil {
			t.Fatalf("ListArtifacts %s: %v", stageTask.ID, err)
		}
		stageTaskArtifactLabels := issueScanArtifactLabels(stageTaskArtifacts)
		for _, label := range []string{work.GateDefinitionOfDone, work.GateAcceptanceCriteria, work.GateTestPlan, IssueScanStageRoleContractArtifactLabel, IssueScanStageOutputContractArtifactLabel} {
			if !containsIssueScanValue(stageTaskArtifactLabels, label) {
				t.Fatalf("stage task %q artifact labels = %+v, want %s", stageID, stageTaskArtifactLabels, label)
			}
		}
		roleContract := issueScanArtifactByLabel(stageTaskArtifacts, IssueScanStageRoleContractArtifactLabel)
		if roleContract == nil {
			t.Fatalf("stage task %q missing role contract artifact in %+v", stageID, stageTaskArtifactLabels)
		}
		var decodedRoleContract struct {
			Kind               string                      `json:"kind"`
			FactoryOrderID     string                      `json:"factory_order_id"`
			StageID            string                      `json:"stage_id"`
			Stage              issueScanBriefStageForTest  `json:"stage"`
			AgentExecutionPlan []issueScanBriefPlanForTest `json:"agent_execution_plan"`
		}
		if err := json.Unmarshal([]byte(roleContract.Body), &decodedRoleContract); err != nil {
			t.Fatalf("unmarshal role contract for %q: %v", stageID, err)
		}
		if decodedRoleContract.Kind != issueScanStageRoleContractArtifactKind || decodedRoleContract.FactoryOrderID != task.FactoryOrderID || decodedRoleContract.StageID != stageID {
			t.Fatalf("stage task %q role contract = %+v", stageID, decodedRoleContract)
		}
		expectedStage := issueScanStageByID(brief.DevelopmentLifecycle, stageID)
		if expectedStage == nil {
			t.Fatalf("missing expected stage %q", stageID)
		}
		for _, role := range expectedStage.RequiredRoles {
			if !containsIssueScanValue(decodedRoleContract.Stage.RequiredRoles, role) || issueScanPlanStepByRole(decodedRoleContract.AgentExecutionPlan, stageID, role) == nil {
				t.Fatalf("stage task %q role contract missing role %q: %+v", stageID, role, decodedRoleContract)
			}
		}
		outputContract := issueScanArtifactByLabel(stageTaskArtifacts, IssueScanStageOutputContractArtifactLabel)
		if outputContract == nil {
			t.Fatalf("stage task %q missing output contract artifact in %+v", stageID, stageTaskArtifactLabels)
		}
		var decodedOutputContract struct {
			Kind                string                                   `json:"kind"`
			FactoryOrderID      string                                   `json:"factory_order_id"`
			StageID             string                                   `json:"stage_id"`
			RequiredEvidence    []string                                 `json:"required_evidence"`
			ExpectedOutputs     []string                                 `json:"expected_outputs"`
			RoleOutputContracts []CivilizationAssemblyRoleOutputContract `json:"role_output_contracts"`
		}
		if err := json.Unmarshal([]byte(outputContract.Body), &decodedOutputContract); err != nil {
			t.Fatalf("unmarshal output contract for %q: %v", stageID, err)
		}
		if decodedOutputContract.Kind != issueScanStageOutputContractArtifactKind || decodedOutputContract.FactoryOrderID != task.FactoryOrderID || decodedOutputContract.StageID != stageID {
			t.Fatalf("stage task %q output contract = %+v", stageID, decodedOutputContract)
		}
		for _, evidence := range expectedStage.RequiredEvidence {
			if !containsIssueScanValue(decodedOutputContract.RequiredEvidence, evidence) || !containsIssueScanValue(decodedOutputContract.ExpectedOutputs, evidence) {
				t.Fatalf("stage task %q output contract missing required evidence %q: %+v", stageID, evidence, decodedOutputContract)
			}
		}
		for _, role := range expectedStage.RequiredRoles {
			plan := issueScanPlanStepByRole(brief.AgentExecutionPlan, stageID, role)
			outputs := issueScanRoleOutputContractByRole(decodedOutputContract.RoleOutputContracts, role)
			if plan == nil || outputs == nil {
				t.Fatalf("stage task %q output contract missing role %q: plan=%+v contract=%+v", stageID, role, plan, decodedOutputContract.RoleOutputContracts)
			}
			for _, output := range plan.RequiredOutputs {
				if !containsIssueScanValue(outputs.RequiredOutputs, output) {
					t.Fatalf("stage task %q output contract for role %q missing output %q: %+v", stageID, role, output, outputs)
				}
			}
		}
		draft, ok := stageDraftsByStage[safeRunLaunchID(stageID)]
		if !ok {
			t.Fatalf("missing stage draft %q in %+v", stageID, stageDraftsByStage)
		}
		if err := rt.attachIssueScanLifecycleStageTaskReadinessGates(content, order, draft, request.ID(), task.ID, stageTask.ID, writer.conv); err != nil {
			t.Fatalf("reattach stage task %q readiness gates: %v", stageID, err)
		}
		if err := rt.attachIssueScanLifecycleStageTaskRoleContract(content, order, draft, request.ID(), task.ID, stageTask.ID, writer.conv); err != nil {
			t.Fatalf("reattach stage task %q role contract: %v", stageID, err)
		}
		if err := rt.attachIssueScanLifecycleStageTaskOutputContract(content, order, draft, request.ID(), task.ID, stageTask.ID, writer.conv); err != nil {
			t.Fatalf("reattach stage task %q output contract: %v", stageID, err)
		}
		stageTaskArtifacts, err = rt.tasks.ListArtifacts(stageTask.ID)
		if err != nil {
			t.Fatalf("ListArtifacts after reattach %s: %v", stageTask.ID, err)
		}
		stageTaskArtifactLabels = issueScanArtifactLabels(stageTaskArtifacts)
		for _, label := range []string{work.GateDefinitionOfDone, work.GateAcceptanceCriteria, work.GateTestPlan, IssueScanStageRoleContractArtifactLabel, IssueScanStageOutputContractArtifactLabel} {
			if countIssueScanValues(stageTaskArtifactLabels, label) != 1 {
				t.Fatalf("stage task %q artifact labels after reattach = %+v, want one %s", stageID, stageTaskArtifactLabels, label)
			}
		}
		blocked, err := rt.tasks.IsBlocked(stageTask.ID)
		if err != nil {
			t.Fatalf("IsBlocked %s: %v", stageTask.ID, err)
		}
		if !blocked {
			t.Fatalf("stage task %q is not blocked, want parent task dependency barrier", stageID)
		}
		deps, err := rt.tasks.GetDependencies(stageTask.ID)
		if err != nil {
			t.Fatalf("GetDependencies %s: %v", stageTask.ID, err)
		}
		if i == 0 {
			if len(deps) != 1 || deps[0] != task.ID {
				t.Fatalf("first stage task dependencies = %+v, want parent task %s", deps, task.ID)
			}
		} else if len(deps) != 2 || !containsEventID(deps, task.ID) || !containsEventID(deps, previousStageTaskID) {
			t.Fatalf("stage task %q dependencies = %+v, want parent task %s and previous stage %s", stageID, deps, task.ID, previousStageTaskID)
		}
		stageTaskIDsByStage[stageID] = stageTask.ID
		previousStageTaskID = stageTask.ID
	}
	if len(stageTaskIDsByStage) != len(expectedStageIDs) {
		t.Fatalf("stage task ids by stage = %+v", stageTaskIDsByStage)
	}
	if err := rt.tasks.Complete(writer.human, task.ID, "parent FactoryOrder completed for stage draft barrier check", []types.EventID{task.ID}, writer.conv); err != nil {
		t.Fatalf("Complete parent task: %v", err)
	}
	for i, stageID := range expectedStageIDs {
		stageTaskID := stageTaskIDsByStage[stageID]
		readiness, err := rt.tasks.Readiness(stageTaskID)
		if err != nil {
			t.Fatalf("Readiness after parent completion %s: %v", stageTaskID, err)
		}
		if !readiness.Ready || len(readiness.MissingGates) != 0 {
			t.Fatalf("stage task %q readiness after parent completion = %+v, want ready with stage gates", stageID, readiness)
		}
		blocked, err := rt.tasks.IsBlocked(stageTaskID)
		if err != nil {
			t.Fatalf("IsBlocked after parent completion %s: %v", stageTaskID, err)
		}
		if i == 0 && blocked {
			t.Fatalf("first stage task %q is blocked after parent completion, want parent barrier released", stageID)
		}
		if i > 0 && !blocked {
			t.Fatalf("stage task %q is not blocked after parent completion, want previous stage dependency barrier", stageID)
		}
	}
	projection := BuildCivilizationAssemblyProjection(rt.store, 100)
	for i, stageID := range expectedStageIDs {
		taskID := stageTaskIDsByStage[stageID]
		taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, taskID.Value())
		if taskEvidence == nil {
			t.Fatalf("projection missing stage task %q evidence for %s", stageID, taskID)
		}
		expectedStage := issueScanStageByID(brief.DevelopmentLifecycle, stageID)
		if expectedStage == nil {
			t.Fatalf("missing expected stage %q", stageID)
		}
		for _, role := range expectedStage.RequiredRoles {
			if !containsIssueScanValue(taskEvidence.RequiredRoles, role) || civilizationProjectionPlanStepByRole(taskEvidence.AgentExecutionPlan, stageID, role) == nil {
				t.Fatalf("projected stage task %q role evidence missing %q: %+v", stageID, role, taskEvidence)
			}
		}
		if len(taskEvidence.RoleContractRefs) == 0 {
			t.Fatalf("projected stage task %q missing role contract refs: %+v", stageID, taskEvidence)
		}
		if len(taskEvidence.OutputContractRefs) == 0 {
			t.Fatalf("projected stage task %q missing output contract refs: %+v", stageID, taskEvidence)
		}
		for _, evidence := range expectedStage.RequiredEvidence {
			if !containsIssueScanValue(taskEvidence.RequiredEvidence, evidence) || !containsIssueScanValue(taskEvidence.ExpectedOutputs, evidence) {
				t.Fatalf("projected stage task %q output evidence missing %q: %+v", stageID, evidence, taskEvidence)
			}
		}
		for _, role := range expectedStage.RequiredRoles {
			plan := issueScanPlanStepByRole(brief.AgentExecutionPlan, stageID, role)
			outputs := issueScanRoleOutputContractByRole(taskEvidence.RoleOutputContracts, role)
			if plan == nil || outputs == nil {
				t.Fatalf("projected stage task %q output contract missing role %q: plan=%+v task=%+v", stageID, role, plan, taskEvidence)
			}
			for _, output := range plan.RequiredOutputs {
				if !containsIssueScanValue(outputs.RequiredOutputs, output) {
					t.Fatalf("projected stage task %q output contract for role %q missing output %q: %+v", stageID, role, output, outputs)
				}
			}
		}
		if i == 0 {
			if taskEvidence.Status != "work_task_ready" || !taskEvidence.Ready || taskEvidence.Blocked {
				t.Fatalf("projected first stage task %q evidence = %+v, want ready and unblocked", stageID, taskEvidence)
			}
		} else if taskEvidence.Status != "work_task_blocked" || taskEvidence.Ready || !taskEvidence.Blocked {
			t.Fatalf("projected dependent stage task %q evidence = %+v, want readiness gates plus dependency block", stageID, taskEvidence)
		}
	}
	firstStageTaskID := stageTaskIDsByStage[expectedStageIDs[0]]
	stageCompletion, err := rt.CompleteIssueScanLifecycleStage(content.RunID, expectedStageIDs[0], issueScanStageRuntimeEvidenceForTest(t, expectedStageIDs[0]), false)
	if err != nil {
		t.Fatalf("CompleteIssueScanLifecycleStage first stage: %v", err)
	}
	if !stageCompletion.Completed || stageCompletion.StageTaskID != firstStageTaskID || stageCompletion.EvidenceArtifactID == (types.EventID{}) {
		t.Fatalf("stage completion = %+v, want completed first stage with runtime evidence artifact", stageCompletion)
	}
	completedProjection := BuildCivilizationAssemblyProjection(rt.store, 100)
	firstStageEvidence := civilizationProjectionTaskEvidenceByID(completedProjection.WorkEvidenceSummary.Tasks, firstStageTaskID.Value())
	if firstStageEvidence == nil || firstStageEvidence.Status != "work_task_completed" || firstStageEvidence.Ready || firstStageEvidence.Blocked {
		t.Fatalf("projected completed first stage evidence = %+v, want completed Work task without ready/blocked flags", firstStageEvidence)
	}
	if firstStageEvidence.RuntimeEvidenceStatus != civilizationAssemblyQueuedStageCompletedStatus || !containsIssueScanValue(firstStageEvidence.RuntimeEvidenceRefs, stageCompletion.EvidenceArtifactID.Value()) {
		t.Fatalf("projected completed first stage runtime evidence = %+v, want %q ref %s", firstStageEvidence, civilizationAssemblyQueuedStageCompletedStatus, stageCompletion.EvidenceArtifactID)
	}
	completedQueued := completedProjection.QueuedRunRequest
	if completedQueued == nil {
		t.Fatalf("completed projection queued run request = nil")
	}
	completedResearchStage := queuedRunLifecycleStageByID(completedQueued.DevelopmentLifecycle, expectedStageIDs[0])
	if completedResearchStage == nil || completedResearchStage.EvidenceStatus != civilizationAssemblyQueuedStageCompletedStatus {
		t.Fatalf("completed research stage evidence = %+v, want completed runtime evidence status", completedResearchStage)
	}
	completedResearchStep := queuedRunAgentPlanStepByRole(completedQueued.AgentExecutionPlan, expectedStageIDs[0], "strategist")
	if completedResearchStep == nil || completedResearchStep.EvidenceStatus != civilizationAssemblyQueuedPlanStepCompletedStatus {
		t.Fatalf("completed research strategist step = %+v, want completed runtime evidence status", completedResearchStep)
	}
	completedStrategistOutput := issueScanRoleOutputContractByRole(firstStageEvidence.RoleOutputContracts, "strategist")
	if completedStrategistOutput == nil || completedStrategistOutput.EvidenceStatus != civilizationAssemblyQueuedPlanStepCompletedStatus {
		t.Fatalf("completed strategist output contract = %+v, want completed runtime evidence status", completedStrategistOutput)
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
	stageArtifactBodies := map[string]string{}
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
		if strings.HasPrefix(artifact.Label, IssueScanLifecycleStageArtifactPrefix) {
			if artifact.TaskID != task.ID {
				t.Fatalf("stage artifact task id = %s, want %s", artifact.TaskID, task.ID)
			}
			if artifact.MediaType != issueScanExecutionPlanArtifactMediaType {
				t.Fatalf("stage artifact media type = %q, want %q", artifact.MediaType, issueScanExecutionPlanArtifactMediaType)
			}
			if artifact.CreatedBy != writer.human {
				t.Fatalf("stage artifact created_by = %s, want %s", artifact.CreatedBy, writer.human)
			}
			stageArtifactBodies[artifact.Label] = artifact.Body
		}
	}
	if planArtifactBody == "" {
		t.Fatalf("missing %s artifact in %+v", IssueScanExecutionPlanArtifactLabel, artifacts)
	}
	if len(stageArtifactBodies) != len(expectedStageIDs) {
		t.Fatalf("stage artifacts = %+v, want %d issue-scan lifecycle stage artifacts", stageArtifactBodies, len(expectedStageIDs))
	}
	var artifactBrief struct {
		Kind             string `json:"kind"`
		LifecycleVersion string `json:"lifecycle_version"`
		SelectionPolicy  struct {
			PolicyID     string `json:"policy_id"`
			SelectedRank int    `json:"selected_rank"`
		} `json:"selection_policy"`
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
	if artifactBrief.SelectionPolicy.PolicyID != "deterministic_actionability_rank_v0.2" || artifactBrief.SelectionPolicy.SelectedRank != 1 {
		t.Fatalf("artifact selection policy = %+v", artifactBrief.SelectionPolicy)
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
	for i, stageID := range expectedStageIDs {
		label := IssueScanLifecycleStageArtifactLabel(stageID)
		body, ok := stageArtifactBodies[label]
		if !ok {
			t.Fatalf("missing stage artifact %q in labels %+v", label, stageArtifactBodies)
		}
		var stageArtifact struct {
			Kind             string `json:"kind"`
			LifecycleVersion string `json:"lifecycle_version"`
			RunID            string `json:"run_id"`
			StageIndex       int    `json:"stage_index"`
			StageCount       int    `json:"stage_count"`
			Stage            struct {
				ID                string   `json:"id"`
				RequiredRoles     []string `json:"required_roles"`
				RequiredEvidence  []string `json:"required_evidence"`
				AuthorityBoundary string   `json:"authority_boundary"`
				CompletionGate    string   `json:"completion_gate"`
				EvidenceStatus    string   `json:"evidence_status"`
			} `json:"stage"`
			AgentExecutionPlan []issueScanBriefPlanForTest `json:"agent_execution_plan"`
			EvidenceKind       string                      `json:"evidence_kind"`
			EvidenceStatus     string                      `json:"evidence_status"`
		}
		if err := json.Unmarshal([]byte(body), &stageArtifact); err != nil {
			t.Fatalf("unmarshal stage artifact %q: %v", label, err)
		}
		if stageArtifact.Kind != "issue_scan_lifecycle_stage" || stageArtifact.LifecycleVersion != issueScanLifecycleVersion || stageArtifact.RunID != content.RunID {
			t.Fatalf("stage artifact identity for %q = %+v", label, stageArtifact)
		}
		if stageArtifact.StageIndex != i+1 || stageArtifact.StageCount != len(expectedStageIDs) {
			t.Fatalf("stage artifact order for %q = %d/%d, want %d/%d", label, stageArtifact.StageIndex, stageArtifact.StageCount, i+1, len(expectedStageIDs))
		}
		if stageArtifact.Stage.ID != stageID || stageArtifact.Stage.EvidenceStatus != "expected_not_observed" {
			t.Fatalf("stage artifact stage for %q = %+v", label, stageArtifact.Stage)
		}
		if stageArtifact.EvidenceKind != "stage_declaration_not_completion" || stageArtifact.EvidenceStatus != "pending_runtime_evidence" {
			t.Fatalf("stage artifact evidence status for %q = %+v", label, stageArtifact)
		}
		for _, role := range stageArtifact.Stage.RequiredRoles {
			if issueScanPlanStepByRole(stageArtifact.AgentExecutionPlan, stageID, role) == nil {
				t.Fatalf("stage artifact %q missing role %q in plan %+v", label, role, stageArtifact.AgentExecutionPlan)
			}
		}
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

func TestQueueIssueScanRunLaunchRanksActionableIssueBeforeScannerOrder(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	req := IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{
			{
				Repo:   "transpara-ai/site",
				Number: 12,
				Title:  "General discussion cleanup",
				URL:    "https://github.com/transpara-ai/site/issues/12",
				Labels: []string{"discussion"},
			},
			{
				Repo:   "transpara-ai/hive",
				Number: 321,
				Title:  "Teach the Civilization to scan Transpara-AI repos",
				URL:    "https://github.com/transpara-ai/hive/issues/321",
				Body:   "Create a FactoryOrder, run adversarial review, drive zero blockers, and surface a ready-for-Human PR.",
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
		t.Fatalf("selected issue = %+v, want ranked hive#321 over first scanner result", queued.Selected)
	}
	if got := strings.Join(queued.TargetRepos, ","); got != "transpara-ai/hive" {
		t.Fatalf("target repos = %q, want selected repo only", got)
	}

	requestEvents := requireRunLaunchEvents(t, rt.store, EventTypeFactoryRunRequested, 1)
	content := requestEvents[0].Content().(FactoryRunRequestedContent)
	var brief struct {
		SelectedIssue struct {
			Rank   int    `json:"rank"`
			Repo   string `json:"repo"`
			Number int    `json:"number"`
		} `json:"selected_issue"`
		CandidateIssues []struct {
			Rank   int    `json:"rank"`
			Repo   string `json:"repo"`
			Number int    `json:"number"`
		} `json:"candidate_issues"`
		SelectionPolicy struct {
			PolicyID      string   `json:"policy_id"`
			RankingInputs []string `json:"ranking_inputs"`
		} `json:"selection_policy"`
	}
	if err := json.Unmarshal(content.Brief, &brief); err != nil {
		t.Fatalf("unmarshal brief: %v", err)
	}
	if brief.SelectedIssue.Rank != 1 || brief.SelectedIssue.Repo != "transpara-ai/hive" || brief.SelectedIssue.Number != 321 {
		t.Fatalf("brief selected issue = %+v", brief.SelectedIssue)
	}
	if len(brief.CandidateIssues) != 2 || brief.CandidateIssues[0].Number != 321 || brief.CandidateIssues[1].Number != 12 {
		t.Fatalf("candidate issue ranks = %+v, want actionable issue rank 1", brief.CandidateIssues)
	}
	if brief.SelectionPolicy.PolicyID != "deterministic_actionability_rank_v0.2" || !containsIssueScanValue(brief.SelectionPolicy.RankingInputs, "scanner_return_order_tie_breaker") {
		t.Fatalf("selection policy = %+v", brief.SelectionPolicy)
	}
}

func TestProgressIssueScanLifecycleRecordsResearchRoleOutputsAndCompletesStage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{
			{
				Repo:   "transpara-ai/hive",
				Number: 321,
				Title:  "Teach the Civilization to scan Transpara-AI repos",
				URL:    "https://github.com/transpara-ai/hive/issues/321",
				Body:   "The Civilization should scan Transpara-AI repos, create a FactoryOrder, debate, implement, review, and surface a ready-for-Human PR.",
				Labels: []string{"civilization", "autonomy"},
			},
		},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}

	progress, err := rt.ProgressIssueScanRunLifecycle(queued.RunID)
	if err != nil {
		t.Fatalf("ProgressIssueScanRunLifecycle: %v", err)
	}
	if progress.Dispatch.Dispatched != 1 {
		t.Fatalf("dispatch = %+v, want one dispatched issue-scan run", progress.Dispatch)
	}
	if countRecordedIssueScanRoleOutputs(progress.ResearchRoleOutputs) != 2 {
		t.Fatalf("research role outputs = %+v, want strategist and planner recorded", progress.ResearchRoleOutputs)
	}
	if countRecordedIssueScanRoleOutputs(progress.DebateRoleOutputs) != 4 {
		t.Fatalf("debate role outputs = %+v, want strategist/planner/reviewer/guardian recorded", progress.DebateRoleOutputs)
	}
	if countRecordedIssueScanRoleOutputs(progress.DesignRoleOutputs) != 3 {
		t.Fatalf("design role outputs = %+v, want planner/reviewer/guardian recorded", progress.DesignRoleOutputs)
	}
	researchCompletion := issueScanCompletionByStageForTest(progress.Completions, "research_issue_and_repo_context")
	if researchCompletion == nil || !researchCompletion.Completed {
		t.Fatalf("completions = %+v, want completed research stage", progress.Completions)
	}
	if researchCompletion.NextAdvance == nil || researchCompletion.NextAdvance.StageID != "debate_with_correct_civic_roles" || !researchCompletion.NextAdvance.Released {
		t.Fatalf("research next advance = %+v, want debate stage released", researchCompletion.NextAdvance)
	}
	debateCompletion := issueScanCompletionByStageForTest(progress.Completions, "debate_with_correct_civic_roles")
	if debateCompletion == nil || !debateCompletion.Completed {
		t.Fatalf("completions = %+v, want completed debate stage", progress.Completions)
	}
	designCompletion := issueScanCompletionByStageForTest(progress.Completions, "select_and_design_approach")
	if designCompletion == nil || !designCompletion.Completed {
		t.Fatalf("completions = %+v, want completed design stage", progress.Completions)
	}
	if designCompletion.NextAdvance == nil || designCompletion.NextAdvance.StageID != "implement_on_branch" || !designCompletion.NextAdvance.Released {
		t.Fatalf("design next advance = %+v, want implementation stage released", designCompletion.NextAdvance)
	}
	if len(progress.ImplementationTasks) != 1 || !progress.ImplementationTasks[0].Created {
		t.Fatalf("implementation task progress = %+v, want one created task", progress.ImplementationTasks)
	}

	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	researchTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "research_issue_and_repo_context"))
	if !ok {
		t.Fatalf("missing research stage task in %+v", tasks)
	}
	researchCompleted, err := rt.issueScanStageTaskCompleted(researchTask.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted research: %v", err)
	}
	if !researchCompleted {
		t.Fatalf("research stage task was not completed")
	}
	artifacts, err := rt.tasks.ListArtifacts(researchTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts research: %v", err)
	}
	roleOutputs := issueScanRoleOutputArtifactsForTest(t, artifacts)
	strategist := roleOutputs["strategist"]
	planner := roleOutputs["planner"]
	if strategist == nil || planner == nil {
		t.Fatalf("research role outputs = %+v, want strategist and planner", roleOutputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(strategist.Outputs, "issue_snapshot") == nil || issueScanStageRuntimeEvidenceItemByKey(strategist.Outputs, "risk_and_scope_notes") == nil {
		t.Fatalf("strategist outputs = %+v, want issue snapshot and risk/scope", strategist.Outputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(planner.Outputs, "repo_context") == nil || issueScanStageRuntimeEvidenceItemByKey(planner.Outputs, "repo_context_packet") == nil {
		t.Fatalf("planner outputs = %+v, want repo context", planner.Outputs)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) == nil {
		t.Fatalf("missing research runtime evidence artifact: %+v", artifacts)
	}
	debateTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "debate_with_correct_civic_roles"))
	if !ok {
		t.Fatalf("missing debate stage task in %+v", tasks)
	}
	debateArtifacts, err := rt.tasks.ListArtifacts(debateTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts debate: %v", err)
	}
	debateOutputs := issueScanRoleOutputArtifactsForTest(t, debateArtifacts)
	if debateOutputs["strategist"] == nil || debateOutputs["planner"] == nil || debateOutputs["reviewer"] == nil || debateOutputs["guardian"] == nil {
		t.Fatalf("debate role outputs = %+v, want all debate roles", debateOutputs)
	}
	if issueScanArtifactByLabel(debateArtifacts, IssueScanStageRuntimeEvidenceArtifactLabel) == nil {
		t.Fatalf("missing debate runtime evidence artifact: %+v", debateArtifacts)
	}
	designTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "select_and_design_approach"))
	if !ok {
		t.Fatalf("missing design stage task in %+v", tasks)
	}
	designArtifacts, err := rt.tasks.ListArtifacts(designTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts design: %v", err)
	}
	designOutputs := issueScanRoleOutputArtifactsForTest(t, designArtifacts)
	if designOutputs["planner"] == nil || designOutputs["reviewer"] == nil || designOutputs["guardian"] == nil {
		t.Fatalf("design role outputs = %+v, want planner/reviewer/guardian", designOutputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(designOutputs["planner"].Outputs, "implementation_task_plan") == nil {
		t.Fatalf("planner design outputs = %+v, want implementation_task_plan", designOutputs["planner"].Outputs)
	}
	if issueScanArtifactByLabel(designArtifacts, IssueScanStageRuntimeEvidenceArtifactLabel) == nil {
		t.Fatalf("missing design runtime evidence artifact: %+v", designArtifacts)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing concrete implementation task in %+v", tasks)
	}
	if implementationTask.ID != progress.ImplementationTasks[0].ImplementationTaskID {
		t.Fatalf("implementation task id = %s, progress id = %s", implementationTask.ID.Value(), progress.ImplementationTasks[0].ImplementationTaskID.Value())
	}

	progress, err = rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle second pass: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ResearchRoleOutputs) != 0 {
		t.Fatalf("research role outputs second pass = %+v, want no duplicates", progress.ResearchRoleOutputs)
	}
	if countRecordedIssueScanRoleOutputs(progress.DebateRoleOutputs) != 0 {
		t.Fatalf("debate role outputs second pass = %+v, want no duplicates", progress.DebateRoleOutputs)
	}
	if countRecordedIssueScanRoleOutputs(progress.DesignRoleOutputs) != 0 {
		t.Fatalf("design role outputs second pass = %+v, want no duplicates", progress.DesignRoleOutputs)
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

func TestAdvanceIssueScanLifecycleStageReleasesNextRunnableStage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
			Labels: []string{"civilization", "autonomy"},
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}

	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	parentTask, ok := findTaskByCanonicalTaskIDForTest(tasks, "")
	if !ok {
		t.Fatalf("missing parent FactoryOrder task in %+v", tasks)
	}
	firstStageID := "research_issue_and_repo_context"
	secondStageID := "debate_with_correct_civic_roles"
	firstStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, firstStageID))
	if !ok {
		t.Fatalf("missing first stage task in %+v", tasks)
	}
	secondStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, secondStageID))
	if !ok {
		t.Fatalf("missing second stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(firstStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked first stage: %v", err)
	}
	if !blocked {
		t.Fatalf("first stage starts unblocked; want dependency barrier before advance")
	}

	advance, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, "")
	if err != nil {
		t.Fatalf("AdvanceIssueScanLifecycleStage first: %v", err)
	}
	if !advance.Released || advance.AlreadyReady || advance.StageID != firstStageID || advance.StageTaskID != firstStageTask.ID {
		t.Fatalf("first advance = %+v, want released first stage", advance)
	}
	parentStatus, err := rt.tasks.GetCompatibilityStatus(parentTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus parent: %v", err)
	}
	if parentStatus == work.LegacyStatusCompleted {
		t.Fatalf("parent FactoryOrder task was completed by stage advance")
	}
	blocked, err = rt.tasks.IsBlocked(firstStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked first stage after advance: %v", err)
	}
	if blocked {
		t.Fatalf("first stage remains blocked after advance")
	}
	firstDepsAfterAdvance, err := rt.tasks.GetDependencies(firstStageTask.ID)
	if err != nil {
		t.Fatalf("GetDependencies first stage after advance: %v", err)
	}
	if len(firstDepsAfterAdvance) != 1 || firstDepsAfterAdvance[0] != parentTask.ID {
		t.Fatalf("first stage dependencies after advance = %+v, want preserved parent dependency %s", firstDepsAfterAdvance, parentTask.ID)
	}
	secondBlocked, err := rt.tasks.IsBlocked(secondStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked second stage: %v", err)
	}
	if !secondBlocked {
		t.Fatalf("second stage advanced before first stage completion")
	}
	again, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, "")
	if err != nil {
		t.Fatalf("AdvanceIssueScanLifecycleStage idempotent first: %v", err)
	}
	if again.Released || !again.AlreadyReady || again.StageID != firstStageID {
		t.Fatalf("idempotent first advance = %+v, want already ready first stage", again)
	}
	if _, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, secondStageID); err == nil || !strings.Contains(err.Error(), "previous stage") {
		t.Fatalf("explicit second stage before first completion error = %v, want previous stage blocker", err)
	}

	completed, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, firstStageID, issueScanStageRuntimeEvidenceForTest(t, firstStageID), true)
	if err != nil {
		t.Fatalf("CompleteIssueScanLifecycleStage first: %v", err)
	}
	if !completed.Completed || completed.StageID != firstStageID || completed.StageTaskID != firstStageTask.ID || completed.EvidenceArtifactID == (types.EventID{}) {
		t.Fatalf("completed first stage = %+v, want governed runtime evidence completion", completed)
	}
	if completed.NextAdvance == nil {
		t.Fatalf("completed first stage next advance = nil, want second stage release")
	}
	next := *completed.NextAdvance
	if !next.Released || next.StageID != secondStageID || next.PreviousStageID != firstStageID || next.PreviousStageTaskID != firstStageTask.ID {
		t.Fatalf("second advance = %+v, want released second stage after first completion", next)
	}
	secondBlocked, err = rt.tasks.IsBlocked(secondStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked second stage after advance: %v", err)
	}
	if secondBlocked {
		t.Fatalf("second stage remains blocked after advance")
	}
	completedProjection := BuildCivilizationAssemblyProjection(rt.store, 100)
	completedResearchStage := queuedRunLifecycleStageByID(completedProjection.QueuedRunRequest.DevelopmentLifecycle, firstStageID)
	if completedResearchStage == nil || completedResearchStage.EvidenceStatus != civilizationAssemblyQueuedStageCompletedStatus {
		t.Fatalf("completed projection research stage = %+v, want completed runtime evidence", completedResearchStage)
	}
	debateStage := queuedRunLifecycleStageByID(completedProjection.QueuedRunRequest.DevelopmentLifecycle, secondStageID)
	if debateStage == nil || debateStage.EvidenceStatus != civilizationAssemblyQueuedStageTaskDraftStatus {
		t.Fatalf("completed projection debate stage = %+v, want next stage task draft pending runtime evidence", debateStage)
	}
}

func TestStartDispatchedIssueScanLifecycleStagesReleasesFirstStage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
			Labels: []string{"civilization", "autonomy"},
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if len(dispatch.DispatchedIssueScanRunIDs) != 1 || dispatch.DispatchedIssueScanRunIDs[0] != queued.RunID {
		t.Fatalf("dispatched issue-scan run ids = %+v, want %s", dispatch.DispatchedIssueScanRunIDs, queued.RunID)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	firstStageID := "research_issue_and_repo_context"
	secondStageID := "debate_with_correct_civic_roles"
	firstStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, firstStageID))
	if !ok {
		t.Fatalf("missing first stage task in %+v", tasks)
	}
	secondStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, secondStageID))
	if !ok {
		t.Fatalf("missing second stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(firstStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked first stage before start: %v", err)
	}
	if !blocked {
		t.Fatalf("first stage was released by low-level dispatch; want starter to own release")
	}

	advances, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch)
	if err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	if len(advances) != 1 || !advances[0].Released || advances[0].StageID != firstStageID || advances[0].StageTaskID != firstStageTask.ID {
		t.Fatalf("advances = %+v, want first stage release", advances)
	}
	blocked, err = rt.tasks.IsBlocked(firstStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked first stage after start: %v", err)
	}
	if blocked {
		t.Fatalf("first stage remains blocked after starter")
	}
	secondBlocked, err := rt.tasks.IsBlocked(secondStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked second stage after start: %v", err)
	}
	if !secondBlocked {
		t.Fatalf("second stage released before first completion")
	}

	again, err := rt.StartDispatchedIssueScanLifecycleStages(RunLaunchDispatchResult{AlreadyDispatchedIssueScanRunIDs: []string{queued.RunID}})
	if err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages again: %v", err)
	}
	if len(again) != 1 || !again[0].AlreadyReady || again[0].StageID != firstStageID {
		t.Fatalf("second starter pass = %+v, want idempotent already-ready first stage", again)
	}
}

func TestCompleteReadyIssueScanLifecycleStagesSkipsUntilStageEvidenceCovered(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	for _, role := range []string{"strategist", "planner"} {
		if _, err := rt.RecordIssueScanStageRoleOutput(queued.RunID, stageID, issueScanStageRoleOutputForTest(t, stageID, role)); err != nil {
			t.Fatalf("RecordIssueScanStageRoleOutput %s: %v", role, err)
		}
	}
	completions, err := rt.CompleteReadyIssueScanLifecycleStages(dispatch)
	if err != nil {
		t.Fatalf("CompleteReadyIssueScanLifecycleStages: %v", err)
	}
	if len(completions) != 0 {
		t.Fatalf("completions = %+v, want none until stage-required evidence keys are recorded", completions)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	firstStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing first stage task in %+v", tasks)
	}
	completed, err := rt.issueScanStageTaskCompleted(firstStageTask.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted: %v", err)
	}
	if completed {
		t.Fatalf("stage completed without all stage evidence keys")
	}
}

func TestCompleteReadyIssueScanLifecycleStagesCompletesFromRecordedRoleOutputs(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	strategist := issueScanStageRoleOutputForTest(t, stageID, "strategist")
	strategist.Outputs = append(strategist.Outputs, issueScanEvidenceItemForTest(stageID, "strategist", "issue_snapshot"))
	planner := issueScanStageRoleOutputForTest(t, stageID, "planner")
	planner.Outputs = append(planner.Outputs, issueScanEvidenceItemForTest(stageID, "planner", "repo_context"))
	for _, output := range []IssueScanStageRoleOutputEvidence{strategist, planner} {
		if _, err := rt.RecordIssueScanStageRoleOutput(queued.RunID, stageID, output); err != nil {
			t.Fatalf("RecordIssueScanStageRoleOutput %s: %v", output.Role, err)
		}
	}
	completions, err := rt.CompleteReadyIssueScanLifecycleStages(dispatch)
	if err != nil {
		t.Fatalf("CompleteReadyIssueScanLifecycleStages: %v", err)
	}
	if len(completions) != 1 || !completions[0].Completed || completions[0].StageID != stageID || completions[0].EvidenceArtifactID == (types.EventID{}) {
		t.Fatalf("completions = %+v, want first stage auto-completed", completions)
	}
	if completions[0].NextAdvance == nil || !completions[0].NextAdvance.Released || completions[0].NextAdvance.StageID != "debate_with_correct_civic_roles" {
		t.Fatalf("next advance = %+v, want debate stage released after auto-completion", completions[0].NextAdvance)
	}
	body, ok := rt.tasks.GetArtifactBody(completions[0].EvidenceArtifactID)
	if !ok {
		t.Fatalf("missing runtime evidence body %s", completions[0].EvidenceArtifactID)
	}
	var evidence IssueScanStageRuntimeEvidence
	if err := json.Unmarshal([]byte(body), &evidence); err != nil {
		t.Fatalf("unmarshal runtime evidence: %v", err)
	}
	for _, key := range []string{"issue_snapshot", "repo_context", "risk_and_scope_notes"} {
		if issueScanStageRuntimeEvidenceItemByKey(evidence.EvidenceItems, key) == nil {
			t.Fatalf("auto-completed evidence items = %+v, missing %s", evidence.EvidenceItems, key)
		}
	}
}

func TestPostTaskCommandProgressCompletesIssueScanStageFromDirectRoleOutputArtifacts(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}

	strategist := issueScanStageRoleOutputForTest(t, stageID, "strategist")
	strategist.Outputs = append(strategist.Outputs, issueScanEvidenceItemForTest(stageID, "strategist", "issue_snapshot"))
	planner := issueScanStageRoleOutputForTest(t, stageID, "planner")
	planner.Outputs = append(planner.Outputs, issueScanEvidenceItemForTest(stageID, "planner", "repo_context"))
	for _, output := range []IssueScanStageRoleOutputEvidence{strategist, planner} {
		output.Kind = issueScanStageRoleOutputArtifactKind
		output.RunID = ""
		output.FactoryOrderID = ""
		output.StageID = ""
		body, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			t.Fatalf("marshal role output %s: %v", output.Role, err)
		}
		if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, IssueScanStageRoleOutputArtifactLabel, issueScanStageRuntimeEvidenceMediaType, string(body), []types.EventID{stageTask.ID}, writer.conv); err != nil {
			t.Fatalf("AddArtifact role output %s: %v", output.Role, err)
		}
	}

	rt.progressIssueScanLifecycleAfterTaskCommands(context.Background(), 2, 2)

	completed, err := rt.issueScanStageTaskCompleted(stageTask.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted: %v", err)
	}
	if !completed {
		t.Fatalf("stage %s was not completed after direct role-output artifacts", stageID)
	}
	artifacts, err := rt.tasks.ListArtifacts(stageTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts stage: %v", err)
	}
	runtimeEvidence := issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel)
	if runtimeEvidence == nil {
		t.Fatalf("missing %s after post-task progress: %+v", IssueScanStageRuntimeEvidenceArtifactLabel, artifacts)
	}
	nextStageID := "debate_with_correct_civic_roles"
	tasks, err = rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks after progress: %v", err)
	}
	nextStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, nextStageID))
	if !ok {
		t.Fatalf("missing next stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(nextStage.ID)
	if err != nil {
		t.Fatalf("IsBlocked next stage: %v", err)
	}
	if blocked {
		t.Fatalf("next stage %s remains blocked after first stage auto-completion", nextStageID)
	}
}

func TestProgressIssueScanLifecycleCreatesConcreteImplementationTaskAfterDesign(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle: %v", err)
	}
	if len(progress.ImplementationTasks) != 1 || !progress.ImplementationTasks[0].Created {
		t.Fatalf("implementation task progress = %+v, want one created task", progress.ImplementationTasks)
	}

	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing concrete implementation task in %+v", tasks)
	}
	if implementationTask.Cell != "implementation" {
		t.Fatalf("implementation task cell = %q, want implementation", implementationTask.Cell)
	}
	if implementationTask.ID != progress.ImplementationTasks[0].ImplementationTaskID {
		t.Fatalf("implementation task id = %s, progress id = %s", implementationTask.ID.Value(), progress.ImplementationTasks[0].ImplementationTaskID.Value())
	}
	readiness, err := rt.tasks.Readiness(implementationTask.ID)
	if err != nil {
		t.Fatalf("Readiness implementation task: %v", err)
	}
	if !readiness.Ready {
		t.Fatalf("implementation task readiness = %+v, want ready", readiness)
	}
	deps, err := rt.tasks.GetDependencies(implementationTask.ID)
	if err != nil {
		t.Fatalf("GetDependencies implementation task: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("implementation task dependencies = %+v, want none so implementer auto-assignment can claim it", deps)
	}
	artifacts, err := rt.tasks.ListArtifacts(implementationTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts implementation task: %v", err)
	}
	for _, label := range []string{work.GateDefinitionOfDone, work.GateAcceptanceCriteria, work.GateTestPlan, IssueScanImplementationTaskContextArtifactLabel} {
		if issueScanArtifactByLabel(artifacts, label) == nil {
			t.Fatalf("implementation task artifacts missing %s: %+v", label, artifacts)
		}
	}
	if !strings.Contains(implementationTask.Description, "Selected issue:") ||
		!strings.Contains(implementationTask.Description, "transpara-ai/hive#321") ||
		!strings.Contains(implementationTask.Description, "The Civilization should scan Transpara-AI repos.") {
		t.Fatalf("implementation task description does not carry selected issue context:\n%s", implementationTask.Description)
	}
	dod := issueScanArtifactByLabel(artifacts, work.GateDefinitionOfDone)
	if dod == nil || !strings.Contains(dod.Body, "transpara-ai/hive#321") {
		t.Fatalf("definition_of_done does not carry selected issue target: %+v", dod)
	}
	contextArtifact := issueScanArtifactByLabel(artifacts, IssueScanImplementationTaskContextArtifactLabel)
	if contextArtifact == nil ||
		!strings.Contains(contextArtifact.Body, `"repo": "transpara-ai/hive"`) ||
		!strings.Contains(contextArtifact.Body, `"target_repos":`) {
		t.Fatalf("implementation context artifact missing selected issue/target repos:\n%s", contextArtifact.Body)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRoleContractArtifactLabel) != nil || issueScanArtifactByLabel(artifacts, IssueScanStageOutputContractArtifactLabel) != nil {
		t.Fatalf("implementation task carried issue-scan stage contract labels: %+v", artifacts)
	}

	again, ready, err := rt.EnsureIssueScanImplementationTask(queued.RunID)
	if err != nil {
		t.Fatalf("EnsureIssueScanImplementationTask again: %v", err)
	}
	if !ready || !again.AlreadyExists || again.Created {
		t.Fatalf("second ensure = %+v ready=%v, want idempotent already-existing task", again, ready)
	}
}

func TestProgressIssueScanLifecycleRejectsWrongRepoImplementationTask(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	rt.repoPath = issueScanGitRepoWithOrigin(t, "https://github.com/transpara-ai/hive.git")
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/site",
			Number: 109,
			Title:  "Display Civilization runtime evidence",
			URL:    "https://github.com/transpara-ai/site/issues/109",
			Body:   "Site should show the runtime Civilization evidence for Transpara-AI operators.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}

	_, err = rt.progressIssueScanLifecycle()
	if err == nil || !strings.Contains(err.Error(), "refusing wrong-repo implementation task") {
		t.Fatalf("progressIssueScanLifecycle error = %v, want wrong-repo refusal", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if task, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID)); ok {
		t.Fatalf("wrong-repo implementation task was created: %+v", task)
	}
}

func TestProgressIssueScanLifecycleCreatesImplementationTaskForWorkspaceTargetRepo(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	workspaceRoot := t.TempDir()
	rt.repoPath = issueScanGitRepoAtOrigin(t, filepath.Join(workspaceRoot, "hive"), "https://github.com/transpara-ai/hive.git")
	rt.repoWorkspaceRoot = workspaceRoot
	sitePath := issueScanGitRepoAtOrigin(t, filepath.Join(workspaceRoot, "site"), "https://github.com/transpara-ai/site.git")
	absSitePath, err := filepath.Abs(sitePath)
	if err != nil {
		t.Fatalf("Abs sitePath: %v", err)
	}
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/site",
			Number: 109,
			Title:  "Display Civilization runtime evidence",
			URL:    "https://github.com/transpara-ai/site/issues/109",
			Body:   "Site should show the runtime Civilization evidence for Transpara-AI operators.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle: %v", err)
	}
	if len(progress.ImplementationTasks) != 1 || !progress.ImplementationTasks[0].Created {
		t.Fatalf("implementation task progress = %+v, want one created task", progress.ImplementationTasks)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing concrete implementation task in %+v", tasks)
	}
	selected, err := rt.taskWorkspaceProviderFor(AgentDef{CanOperate: true})(context.Background(), implementationTask, "implementer")
	if err != nil {
		t.Fatalf("taskWorkspaceProviderFor: %v", err)
	}
	if !selected.Applied || selected.RepoPath != absSitePath {
		t.Fatalf("selected workspace = %+v, want applied Site path %s", selected, absSitePath)
	}
	if len(selected.ContainmentWatchRoots) != 1 || selected.ContainmentWatchRoots[0] != workspaceRoot {
		t.Fatalf("selected containment roots = %+v, want [%s]", selected.ContainmentWatchRoots, workspaceRoot)
	}
}

func TestTransparaAIRepoSlugFromRemoteURL(t *testing.T) {
	tests := map[string]string{
		"https://github.com/transpara-ai/site":      "transpara-ai/site",
		"https://github.com/transpara-ai/site.git/": "transpara-ai/site",
		"git@github.com:transpara-ai/hive.git":      "transpara-ai/hive",
		"ssh://git@github.com/transpara-ai/work":    "transpara-ai/work",
	}
	for raw, want := range tests {
		got, ok := transparaAIRepoSlugFromRemoteURL(raw)
		if !ok || got != want {
			t.Fatalf("transparaAIRepoSlugFromRemoteURL(%q) = %q ok=%v, want %q true", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"https://github.com/example/hive.git", "https://example.com/transpara-ai/hive", "../hive"} {
		if got, ok := transparaAIRepoSlugFromRemoteURL(raw); ok {
			t.Fatalf("transparaAIRepoSlugFromRemoteURL(%q) = %q ok=true, want rejection", raw, got)
		}
	}
}

func TestProgressIssueScanLifecycleRecordsImplementationRoleOutputAndCompletesStage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}
	if progress, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle seed implementation task: %v", err)
	} else if len(progress.ImplementationTasks) != 1 || !progress.ImplementationTasks[0].Created {
		t.Fatalf("implementation task progress = %+v, want one created task", progress.ImplementationTasks)
	}

	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing implementation task in %+v", tasks)
	}
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", issueScanOperateResultBodyForTest(), []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive passed", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete implementation task: %v", err)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after implementation completion: %v", err)
	}
	if len(progress.ImplementationRoleOutputs) != 1 || !progress.ImplementationRoleOutputs[0].Recorded {
		t.Fatalf("implementation role outputs = %+v, want one recorded output", progress.ImplementationRoleOutputs)
	}
	if len(progress.Completions) == 0 {
		t.Fatalf("completions = %+v, want implement_on_branch auto-completion", progress.Completions)
	}

	tasks, err = rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks after progress: %v", err)
	}
	implementStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "implement_on_branch"))
	if !ok {
		t.Fatalf("missing implement stage task in %+v", tasks)
	}
	implemented, err := rt.issueScanStageTaskCompleted(implementStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted implement stage: %v", err)
	}
	if !implemented {
		t.Fatalf("implement_on_branch stage was not completed")
	}
	artifacts, err := rt.tasks.ListArtifacts(implementStage.ID)
	if err != nil {
		t.Fatalf("ListArtifacts implement stage: %v", err)
	}
	roleArtifact := issueScanArtifactByLabel(artifacts, IssueScanStageRoleOutputArtifactLabel)
	if roleArtifact == nil {
		t.Fatalf("missing implementation role output artifact: %+v", artifacts)
	}
	roleOutput, ok, err := issueScanStageRoleOutputArtifact(roleArtifact.ID.Value(), roleArtifact.Label, roleArtifact.Body)
	if err != nil || !ok {
		t.Fatalf("issueScanStageRoleOutputArtifact = %+v ok=%v err=%v", roleOutput, ok, err)
	}
	for _, key := range []string{"branch_name", "commit_sha", "changed_files", "validation_output"} {
		if issueScanStageRuntimeEvidenceItemByKey(roleOutput.Outputs, key) == nil {
			t.Fatalf("role output missing %s: %+v", key, roleOutput.Outputs)
		}
	}
	runtimeEvidence := issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel)
	if runtimeEvidence == nil {
		t.Fatalf("missing implementation runtime evidence artifact: %+v", artifacts)
	}
	reviewStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "run_adversarial_review"))
	if !ok {
		t.Fatalf("missing review stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(reviewStage.ID)
	if err != nil {
		t.Fatalf("IsBlocked review stage: %v", err)
	}
	if blocked {
		t.Fatalf("review stage remains blocked after implementation stage auto-completion")
	}
}

func TestProgressIssueScanLifecycleRecordsReviewRoleOutputsAndCompletesStage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle seed implementation task: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing implementation task in %+v", tasks)
	}
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", issueScanOperateResultBodyForTest(), []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive passed", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete implementation task: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle complete implementation stage: %v", err)
	}
	if err := appendIssueScanCodeReviewForTest(rt, writer, implementationTask.ID, "request_changes", "exact-head review found blockers", []string{"missing regression test"}); err != nil {
		t.Fatalf("appendIssueScanCodeReviewForTest: %v", err)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after review: %v", err)
	}
	if len(progress.ReviewRoleOutputs) != 2 {
		t.Fatalf("review role outputs = %+v, want reviewer and guardian", progress.ReviewRoleOutputs)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReviewRoleOutputs) != 2 {
		t.Fatalf("review role outputs = %+v, want both newly recorded", progress.ReviewRoleOutputs)
	}
	tasks, err = rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks after review progress: %v", err)
	}
	reviewStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "run_adversarial_review"))
	if !ok {
		t.Fatalf("missing review stage task in %+v", tasks)
	}
	reviewCompleted, err := rt.issueScanStageTaskCompleted(reviewStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted review stage: %v", err)
	}
	if !reviewCompleted {
		t.Fatalf("run_adversarial_review stage was not completed")
	}
	artifacts, err := rt.tasks.ListArtifacts(reviewStage.ID)
	if err != nil {
		t.Fatalf("ListArtifacts review stage: %v", err)
	}
	roleOutputs := issueScanRoleOutputArtifactsForTest(t, artifacts)
	for _, role := range []string{"reviewer", "guardian"} {
		if roleOutputs[role] == nil {
			t.Fatalf("missing %s role output in %+v", role, roleOutputs)
		}
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["reviewer"].Outputs, "exact_head_review_artifact") == nil {
		t.Fatalf("reviewer role output missing exact_head_review_artifact: %+v", roleOutputs["reviewer"].Outputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["guardian"].Outputs, "finding_disposition") == nil {
		t.Fatalf("guardian role output missing finding_disposition: %+v", roleOutputs["guardian"].Outputs)
	}
	runtimeEvidence := issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel)
	if runtimeEvidence == nil {
		t.Fatalf("missing review runtime evidence artifact: %+v", artifacts)
	}
	blockerStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "drive_blockers_to_zero"))
	if !ok {
		t.Fatalf("missing blocker stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(blockerStage.ID)
	if err != nil {
		t.Fatalf("IsBlocked blocker stage: %v", err)
	}
	if blocked {
		t.Fatalf("drive_blockers_to_zero remains blocked after review stage completion")
	}
}

func TestRecordIssueScanAdversarialReviewReceiptEmitsExactHeadReview(t *testing.T) {
	rt, _, queued, orderID, implementationTask := issueScanCompletedImplementationFixtureForTest(t)
	result, err := rt.RecordIssueScanAdversarialReview(queued.RunID, IssueScanAdversarialReviewReceipt{
		Repository:      "transpara-ai/hive",
		ReviewRef:       "artifact://adversarial-review/run-001/claude.result.md",
		ReviewedHeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verdict:         "request_changes",
		Summary:         "exact-head adversarial review found one blocker",
		Issues:          []string{"missing regression test"},
		Confidence:      0.93,
		Tool:            "claude",
	})
	if err != nil {
		t.Fatalf("RecordIssueScanAdversarialReview: %v", err)
	}
	if !result.Recorded {
		t.Fatalf("result = %+v, want newly recorded receipt/review", result)
	}
	if result.FactoryOrderID != orderID {
		t.Fatalf("FactoryOrderID = %q, want %q", result.FactoryOrderID, orderID)
	}
	if result.ImplementationTaskID != implementationTask.ID {
		t.Fatalf("ImplementationTaskID = %s, want %s", result.ImplementationTaskID, implementationTask.ID)
	}
	if result.ReceiptArtifactID == (types.EventID{}) || result.ReviewEventID == (types.EventID{}) {
		t.Fatalf("missing receipt or review event ids: %+v", result)
	}
	if !result.ReopenedImplementationTask {
		t.Fatalf("result = %+v, want request_changes to reopen implementation task", result)
	}
	status, err := rt.tasks.GetCompatibilityStatus(implementationTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus implementation task: %v", err)
	}
	if status == work.LegacyStatusCompleted {
		t.Fatalf("implementation task status = %s, want reopened for repair", status)
	}
	reviewEventID, review, _, ok, err := rt.latestIssueScanCodeReviewForTask(implementationTask.ID)
	if err != nil {
		t.Fatalf("latestIssueScanCodeReviewForTask: %v", err)
	}
	if !ok {
		t.Fatalf("missing code.review.submitted event")
	}
	if reviewEventID != result.ReviewEventID {
		t.Fatalf("review event = %s, want %s", reviewEventID, result.ReviewEventID)
	}
	if review.Verdict != "request_changes" || review.Confidence != 0.93 {
		t.Fatalf("review content = %+v", review)
	}
	if !strings.Contains(review.Summary, result.ReceiptArtifactID.Value()) {
		t.Fatalf("review summary does not reference receipt %s: %s", result.ReceiptArtifactID, review.Summary)
	}
	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after review receipt: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReviewRoleOutputs) != 2 {
		t.Fatalf("review role outputs = %+v, want reviewer and guardian", progress.ReviewRoleOutputs)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks after review progress: %v", err)
	}
	reviewStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "run_adversarial_review"))
	if !ok {
		t.Fatalf("missing review stage task in %+v", tasks)
	}
	reviewCompleted, err := rt.issueScanStageTaskCompleted(reviewStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted review stage: %v", err)
	}
	if !reviewCompleted {
		t.Fatalf("run_adversarial_review stage was not completed")
	}
}

func TestIssueScanAdversarialReviewRunContextCarriesExactOperateHead(t *testing.T) {
	rt, _, queued, orderID, implementationTask := issueScanCompletedImplementationFixtureForTest(t)
	reviewContext, err := rt.IssueScanAdversarialReviewRunContext(queued.RunID)
	if err != nil {
		t.Fatalf("IssueScanAdversarialReviewRunContext: %v", err)
	}
	if reviewContext.Kind != issueScanAdversarialReviewContextKind {
		t.Fatalf("kind = %q, want %q", reviewContext.Kind, issueScanAdversarialReviewContextKind)
	}
	if reviewContext.RunID != queued.RunID || reviewContext.FactoryOrderID != orderID {
		t.Fatalf("context run/order = %q/%q, want %q/%q", reviewContext.RunID, reviewContext.FactoryOrderID, queued.RunID, orderID)
	}
	if reviewContext.Repository != "transpara-ai/hive" || reviewContext.SelectedIssue.Number != 321 {
		t.Fatalf("selected issue = %+v repository=%q", reviewContext.SelectedIssue, reviewContext.Repository)
	}
	if reviewContext.ImplementationTaskID != implementationTask.ID.Value() {
		t.Fatalf("implementation task = %q, want %s", reviewContext.ImplementationTaskID, implementationTask.ID)
	}
	if reviewContext.OperateCommit != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("operate commit = %q", reviewContext.OperateCommit)
	}
	if reviewContext.ExpectedReceipt.ReviewedHeadSHA != reviewContext.OperateCommit {
		t.Fatalf("expected receipt head = %q, want operate commit %q", reviewContext.ExpectedReceipt.ReviewedHeadSHA, reviewContext.OperateCommit)
	}
	if reviewContext.ExpectedReceipt.TaskID != implementationTask.ID.Value() {
		t.Fatalf("expected receipt task = %q, want %s", reviewContext.ExpectedReceipt.TaskID, implementationTask.ID)
	}
	if !containsIssueScanString(reviewContext.BoundaryDisclaimers, "reviewed_head_sha must match operate_commit") {
		t.Fatalf("missing exact-head boundary disclaimer: %+v", reviewContext.BoundaryDisclaimers)
	}
}

func TestProgressIssueScanLifecycleRunsConfiguredAdversarialReviewOnce(t *testing.T) {
	rt, _, queued, _, implementationTask := issueScanCompletedImplementationFixtureForTest(t)
	calls := 0
	rt.issueScanAdversarialReviewRunner = func(ctx context.Context, reviewContext IssueScanAdversarialReviewContext) (IssueScanAdversarialReviewReceipt, error) {
		calls++
		if reviewContext.RunID != queued.RunID {
			t.Fatalf("review runner run id = %q, want %q", reviewContext.RunID, queued.RunID)
		}
		if reviewContext.ImplementationTaskID != implementationTask.ID.Value() {
			t.Fatalf("review runner task id = %q, want %s", reviewContext.ImplementationTaskID, implementationTask.ID)
		}
		receipt := reviewContext.ExpectedReceipt
		receipt.ReviewRef = "artifact://adversarial-review/configured-runner/result.md"
		receipt.Verdict = "approve"
		receipt.Summary = "configured runner approved the verified Operate head"
		receipt.Issues = []string{}
		receipt.Confidence = 0.95
		receipt.Tool = "test-configured-runner"
		return receipt, nil
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle with configured runner: %v", err)
	}
	if calls != 1 {
		t.Fatalf("review runner calls = %d, want 1", calls)
	}
	if len(progress.ReviewRuns) != 1 || !progress.ReviewRuns[0].Recorded {
		t.Fatalf("review runs = %+v, want one recorded exact-head review", progress.ReviewRuns)
	}
	if progress.ReviewRuns[0].ReviewedHeadSHA != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("reviewed head = %q", progress.ReviewRuns[0].ReviewedHeadSHA)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReviewRoleOutputs) != 2 {
		t.Fatalf("review role outputs = %+v, want reviewer and guardian", progress.ReviewRoleOutputs)
	}

	again, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("second progressIssueScanLifecycle: %v", err)
	}
	if calls != 1 {
		t.Fatalf("review runner calls after second progress = %d, want still 1", calls)
	}
	if len(again.ReviewRuns) != 0 {
		t.Fatalf("second review runs = %+v, want none after current completion was reviewed", again.ReviewRuns)
	}
}

func TestProgressIssueScanLifecycleRerunsConfiguredReviewAfterRepair(t *testing.T) {
	rt, writer, _, _, implementationTask := issueScanCompletedImplementationFixtureForTest(t)
	calls := 0
	rt.issueScanAdversarialReviewRunner = func(ctx context.Context, reviewContext IssueScanAdversarialReviewContext) (IssueScanAdversarialReviewReceipt, error) {
		calls++
		receipt := reviewContext.ExpectedReceipt
		receipt.Confidence = 0.95
		receipt.Tool = "test-configured-runner"
		if calls == 1 {
			receipt.ReviewRef = "artifact://adversarial-review/configured-runner/blocker.md"
			receipt.Verdict = "request_changes"
			receipt.Summary = "configured runner found a blocker"
			receipt.Issues = []string{"missing regression test"}
			return receipt, nil
		}
		receipt.ReviewRef = "artifact://adversarial-review/configured-runner/approved.md"
		receipt.Verdict = "approve"
		receipt.Summary = "configured runner approved repaired head"
		receipt.Issues = []string{}
		return receipt, nil
	}

	first, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("initial progressIssueScanLifecycle with configured runner: %v", err)
	}
	if calls != 1 {
		t.Fatalf("review runner calls = %d, want 1", calls)
	}
	if len(first.ReviewRuns) != 1 || first.ReviewRuns[0].Verdict != "request_changes" {
		t.Fatalf("first review runs = %+v, want one request_changes review", first.ReviewRuns)
	}
	if !first.ReviewRuns[0].ReopenedImplementationTask {
		t.Fatalf("first review run = %+v, want implementation task reopened", first.ReviewRuns[0])
	}
	status, err := rt.tasks.GetCompatibilityStatus(implementationTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus reopened implementation task: %v", err)
	}
	if status == work.LegacyStatusCompleted {
		t.Fatalf("implementation task status = %s, want reopened for repair", status)
	}
	repairedBody := issueScanOperateResultBodyForTestWith(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccc",
		"codex/run-issue-001-repair",
		"pkg/hive/example.go | 14 ++++++++++++--\npkg/hive/example_test.go | 18 ++++++++++++++++++\n2 files changed, 30 insertions(+), 2 deletions(-)",
	)
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", repairedBody, []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact repaired Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive passed after repair", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete repaired implementation task: %v", err)
	}

	second, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("second progressIssueScanLifecycle after repair: %v", err)
	}
	if calls != 2 {
		t.Fatalf("review runner calls after repair = %d, want 2", calls)
	}
	if len(second.ReviewRuns) != 1 || second.ReviewRuns[0].ReviewedHeadSHA != "cccccccccccccccccccccccccccccccccccccccc" || second.ReviewRuns[0].Verdict != "approve" {
		t.Fatalf("second review runs = %+v, want approved repaired head", second.ReviewRuns)
	}
	if countRecordedIssueScanRoleOutputs(second.BlockerRoleOutputs) != 3 {
		t.Fatalf("blocker role outputs = %+v, want implementer/reviewer/guardian zero-blocker evidence", second.BlockerRoleOutputs)
	}
}

func TestRecordIssueScanAdversarialReviewReceiptRejectsHeadMismatch(t *testing.T) {
	rt, _, queued, _, implementationTask := issueScanCompletedImplementationFixtureForTest(t)
	_, err := rt.RecordIssueScanAdversarialReview(queued.RunID, IssueScanAdversarialReviewReceipt{
		Repository:      "transpara-ai/hive",
		ReviewRef:       "artifact://adversarial-review/run-001/claude.result.md",
		ReviewedHeadSHA: "cccccccccccccccccccccccccccccccccccccccc",
		Verdict:         "approve",
		Summary:         "reviewed a different head",
		Issues:          []string{},
		Confidence:      0.93,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match implementation commit") {
		t.Fatalf("expected head mismatch error, got %v", err)
	}
	reviews, err := rt.issueScanCodeReviewsForTask(implementationTask.ID)
	if err != nil {
		t.Fatalf("issueScanCodeReviewsForTask: %v", err)
	}
	if len(reviews) != 0 {
		t.Fatalf("reviews = %+v, want none after rejected receipt", reviews)
	}
	artifacts, err := rt.tasks.ListArtifacts(implementationTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts implementation task: %v", err)
	}
	if artifact := issueScanArtifactByLabel(artifacts, IssueScanAdversarialReviewReceiptArtifactLabel); artifact != nil {
		t.Fatalf("unexpected adversarial review receipt artifact: %+v", artifact)
	}
}

func TestProgressIssueScanLifecycleRecordsBlockerRoleOutputsAndCompletesStage(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle seed implementation task: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing implementation task in %+v", tasks)
	}
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", issueScanOperateResultBodyForTest(), []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact initial Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive passed", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete initial implementation task: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle complete implementation stage: %v", err)
	}
	if err := appendIssueScanCodeReviewForTest(rt, writer, implementationTask.ID, "request_changes", "exact-head review found blockers", []string{"missing regression test"}); err != nil {
		t.Fatalf("append request_changes review: %v", err)
	}
	if err := rt.tasks.Reopen(writer.human, implementationTask.ID, "request_changes (review 1 of 3): exact-head review found blockers", []string{"missing regression test"}, []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Reopen implementation task: %v", err)
	}
	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after request_changes: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReviewRoleOutputs) != 2 {
		t.Fatalf("review role outputs after request_changes = %+v, want reviewer and guardian", progress.ReviewRoleOutputs)
	}
	if countRecordedIssueScanRoleOutputs(progress.BlockerRoleOutputs) != 0 {
		t.Fatalf("blocker role outputs after request_changes = %+v, want none before repair approval", progress.BlockerRoleOutputs)
	}

	repairedBody := issueScanOperateResultBodyForTestWith(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccc",
		"codex/run-issue-001-repair",
		"pkg/hive/example.go | 14 ++++++++++++--\npkg/hive/example_test.go | 18 ++++++++++++++++++\n2 files changed, 30 insertions(+), 2 deletions(-)",
	)
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", repairedBody, []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact repaired Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive -run TestProgressIssueScanLifecycleRecordsBlockerRoleOutputsAndCompletesStage passed", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete repaired implementation task: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle after repair completion: %v", err)
	}
	if err := appendIssueScanCodeReviewForTest(rt, writer, implementationTask.ID, "approve", "rerun review approved repaired exact head with zero blockers", nil); err != nil {
		t.Fatalf("append approving review: %v", err)
	}
	progress, err = rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after approving rerun: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.BlockerRoleOutputs) != 3 {
		t.Fatalf("blocker role outputs = %+v, want implementer/reviewer/guardian", progress.BlockerRoleOutputs)
	}

	tasks, err = rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks after blocker progress: %v", err)
	}
	blockerStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "drive_blockers_to_zero"))
	if !ok {
		t.Fatalf("missing blocker stage task in %+v", tasks)
	}
	blockerCompleted, err := rt.issueScanStageTaskCompleted(blockerStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted blocker stage: %v", err)
	}
	if !blockerCompleted {
		t.Fatalf("drive_blockers_to_zero stage was not completed")
	}
	artifacts, err := rt.tasks.ListArtifacts(blockerStage.ID)
	if err != nil {
		t.Fatalf("ListArtifacts blocker stage: %v", err)
	}
	roleOutputs := issueScanRoleOutputArtifactsForTest(t, artifacts)
	for _, role := range []string{"implementer", "reviewer", "guardian"} {
		if roleOutputs[role] == nil {
			t.Fatalf("missing %s blocker role output in %+v", role, roleOutputs)
		}
	}
	for _, key := range []string{"blocker_fixes", "rerun_validation"} {
		if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["implementer"].Outputs, key) == nil {
			t.Fatalf("implementer blocker role output missing %s: %+v", key, roleOutputs["implementer"].Outputs)
		}
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["reviewer"].Outputs, "rerun_review") == nil {
		t.Fatalf("reviewer blocker role output missing rerun_review: %+v", roleOutputs["reviewer"].Outputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["guardian"].Outputs, "zero_blocker_gate_disposition") == nil {
		t.Fatalf("guardian blocker role output missing zero_blocker_gate_disposition: %+v", roleOutputs["guardian"].Outputs)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) == nil {
		t.Fatalf("missing blocker runtime evidence artifact: %+v", artifacts)
	}
	readyStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "surface_ready_for_Human_result_PR"))
	if !ok {
		t.Fatalf("missing ready-for-Human stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(readyStage.ID)
	if err != nil {
		t.Fatalf("IsBlocked ready stage: %v", err)
	}
	if blocked {
		t.Fatalf("surface_ready_for_Human_result_PR remains blocked after blocker stage completion")
	}
}

func TestProgressIssueScanLifecycleRecordsReadyRoleOutputsAndCompletesStage(t *testing.T) {
	rt, writer, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	readyEvidence := IssueScanReadyPREvidence{
		RunID:                  runID,
		FactoryOrderID:         orderID,
		Repository:             "transpara-ai/hive",
		PRNumber:               321,
		PRURL:                  "https://github.com/transpara-ai/hive/pull/321",
		BaseRef:                "main",
		BaseSHA:                "dddddddddddddddddddddddddddddddddddddddd",
		HeadRef:                "codex/run-issue-001-repair",
		HeadSHA:                "cccccccccccccccccccccccccccccccccccccccc",
		State:                  "open",
		Draft:                  false,
		ReadyForReview:         true,
		MergeStateStatus:       "clean",
		CIStatus:               "success",
		ReadyStateReviewRef:    "https://github.com/transpara-ai/hive/pull/321#issuecomment-ready-state-review",
		ReadyStateReviewStatus: "passed",
		HumanApprovalRequired:  true,
		Summary:                "Ready-for-Human result PR is surfaced with exact-head ready-state review evidence and awaits Human approval.",
		SourceRefs:             []string{"test://ready-pr"},
	}
	if err := attachIssueScanDraftPRReceiptForReadyTest(t, rt, writer, readyStage.ID, readyEvidence); err != nil {
		t.Fatalf("attach draft PR receipt: %v", err)
	}
	body, err := IssueScanReadyPREvidenceArtifactBody(readyEvidence)
	if err != nil {
		t.Fatalf("IssueScanReadyPREvidenceArtifactBody: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, readyStage.ID, IssueScanReadyPREvidenceArtifactLabel, "application/json", body, []types.EventID{readyStage.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact ready PR evidence: %v", err)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after ready PR evidence: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 3 {
		t.Fatalf("ready role outputs = %+v, want strategist/reviewer/guardian", progress.ReadyRoleOutputs)
	}
	readyCompleted, err := rt.issueScanStageTaskCompleted(readyStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted ready stage: %v", err)
	}
	if !readyCompleted {
		t.Fatalf("surface_ready_for_Human_result_PR stage was not completed")
	}
	artifacts, err := rt.tasks.ListArtifacts(readyStage.ID)
	if err != nil {
		t.Fatalf("ListArtifacts ready stage: %v", err)
	}
	roleOutputs := issueScanRoleOutputArtifactsForTest(t, artifacts)
	for _, role := range []string{"strategist", "reviewer", "guardian"} {
		if roleOutputs[role] == nil {
			t.Fatalf("missing %s ready role output in %+v", role, roleOutputs)
		}
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["strategist"].Outputs, "ready_pr_url") == nil {
		t.Fatalf("strategist ready role output missing ready_pr_url: %+v", roleOutputs["strategist"].Outputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["strategist"].Outputs, "human_ready_summary") == nil {
		t.Fatalf("strategist ready role output missing human_ready_summary: %+v", roleOutputs["strategist"].Outputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["reviewer"].Outputs, "ready_state_review") == nil {
		t.Fatalf("reviewer ready role output missing ready_state_review: %+v", roleOutputs["reviewer"].Outputs)
	}
	if issueScanStageRuntimeEvidenceItemByKey(roleOutputs["guardian"].Outputs, "human_approval_boundary_check") == nil {
		t.Fatalf("guardian ready role output missing human_approval_boundary_check: %+v", roleOutputs["guardian"].Outputs)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) == nil {
		t.Fatalf("missing ready runtime evidence artifact: %+v", artifacts)
	}

	progress, err = rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after ready completion: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 0 {
		t.Fatalf("ready role outputs after completed stage = %+v, want no duplicate records", progress.ReadyRoleOutputs)
	}
}

func TestRecordIssueScanReadyPREvidenceCompletesReadyStage(t *testing.T) {
	rt, _, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	readyEvidence := IssueScanReadyPREvidence{
		RunID:                  runID,
		FactoryOrderID:         orderID,
		Repository:             "transpara-ai/hive",
		PRNumber:               321,
		PRURL:                  "https://github.com/transpara-ai/hive/pull/321",
		BaseRef:                "main",
		BaseSHA:                "dddddddddddddddddddddddddddddddddddddddd",
		HeadRef:                "codex/run-issue-001-repair",
		HeadSHA:                "cccccccccccccccccccccccccccccccccccccccc",
		State:                  "open",
		Draft:                  false,
		ReadyForReview:         true,
		MergeStateStatus:       "clean",
		CIStatus:               "success",
		ReadyStateReviewRef:    "https://github.com/transpara-ai/hive/pull/321#issuecomment-ready-state-review",
		ReadyStateReviewStatus: "passed",
		HumanApprovalRequired:  true,
		Summary:                "Ready-for-Human result PR is surfaced with exact-head ready-state review evidence and awaits Human approval.",
		SourceRefs:             []string{"test://ready-pr"},
	}
	receipt := TransparaAIDraftPRReceipt{
		Kind:                   transparaAIDraftPRReceiptKind,
		Repository:             "transpara-ai/hive",
		PRNumber:               readyEvidence.PRNumber,
		PRURL:                  readyEvidence.PRURL,
		BaseRef:                readyEvidence.BaseRef,
		BaseSHA:                readyEvidence.BaseSHA,
		HeadRef:                readyEvidence.HeadRef,
		HeadSHA:                readyEvidence.HeadSHA,
		RemoteHeadSHA:          readyEvidence.HeadSHA,
		ChangedFiles:           []string{"README.md"},
		Draft:                  true,
		State:                  "open",
		PolicyBundleID:         TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash:       TransparaAIDraftPRPolicyBundleHash(),
		AuthorityNonce:         "nonce-ready-pr-test",
		HumanApprovalRequired:  true,
		NoMergeOrDeployClaim:   true,
		ReadyForReviewRequired: true,
	}

	draftResult, err := rt.RecordIssueScanDraftPRReceipt(runID, receipt)
	if err != nil {
		t.Fatalf("RecordIssueScanDraftPRReceipt: %v", err)
	}
	if !draftResult.Recorded || draftResult.DraftPRReceiptArtifactID == (types.EventID{}) {
		t.Fatalf("draft result = %+v, want recorded receipt artifact", draftResult)
	}
	readyResult, err := rt.RecordIssueScanReadyPREvidence(runID, readyEvidence)
	if err != nil {
		t.Fatalf("RecordIssueScanReadyPREvidence: %v", err)
	}
	if !readyResult.Recorded || readyResult.ReadyPREvidenceArtifactID == (types.EventID{}) {
		t.Fatalf("ready result = %+v, want recorded ready PR evidence artifact", readyResult)
	}
	if readyResult.DraftPRReceiptRef != draftResult.DraftPRReceiptArtifactID.Value() {
		t.Fatalf("draft receipt ref = %q, want %s", readyResult.DraftPRReceiptRef, draftResult.DraftPRReceiptArtifactID)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle after ready PR recorder: %v", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 3 {
		t.Fatalf("ready role outputs = %+v, want strategist/reviewer/guardian", progress.ReadyRoleOutputs)
	}
	readyCompleted, err := rt.issueScanStageTaskCompleted(readyStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted ready stage: %v", err)
	}
	if !readyCompleted {
		t.Fatalf("surface_ready_for_Human_result_PR stage was not completed")
	}
}

func TestProgressIssueScanLifecycleRunsConfiguredReadyPRRunner(t *testing.T) {
	rt, _, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	calls := 0
	rt.issueScanReadyPRRunner = func(ctx context.Context, readyContext IssueScanReadyPRRunnerContext) (IssueScanReadyPRRunnerResult, error) {
		calls++
		if readyContext.RunID != runID || readyContext.FactoryOrderID != orderID {
			t.Fatalf("ready context run/order = %q/%q, want %q/%q", readyContext.RunID, readyContext.FactoryOrderID, runID, orderID)
		}
		if readyContext.ReadyStageTaskID != readyStage.ID.Value() {
			t.Fatalf("ready stage task = %q, want %s", readyContext.ReadyStageTaskID, readyStage.ID)
		}
		if readyContext.OperateCommit != "cccccccccccccccccccccccccccccccccccccccc" {
			t.Fatalf("operate commit = %q", readyContext.OperateCommit)
		}
		readyEvidence := readyContext.ExpectedReadyPREvidence
		readyEvidence.PRNumber = 321
		readyEvidence.PRURL = "https://github.com/transpara-ai/hive/pull/321"
		readyEvidence.BaseRef = "main"
		readyEvidence.BaseSHA = "dddddddddddddddddddddddddddddddddddddddd"
		readyEvidence.HeadRef = "codex/run-issue-001-repair"
		readyEvidence.MergeStateStatus = "clean"
		readyEvidence.CIStatus = "success"
		readyEvidence.ReadyStateReviewRef = "https://github.com/transpara-ai/hive/pull/321#issuecomment-ready-state-review"
		readyEvidence.ReadyStateReviewStatus = "passed"
		readyEvidence.Summary = "Ready-for-Human result PR is surfaced with exact-head ready-state review evidence and awaits Human approval."
		return IssueScanReadyPRRunnerResult{
			DraftPRReceipt: TransparaAIDraftPRReceipt{
				Kind:                   transparaAIDraftPRReceiptKind,
				Repository:             readyContext.Repository,
				PRNumber:               readyEvidence.PRNumber,
				PRURL:                  readyEvidence.PRURL,
				BaseRef:                readyEvidence.BaseRef,
				BaseSHA:                readyEvidence.BaseSHA,
				HeadRef:                readyEvidence.HeadRef,
				HeadSHA:                readyEvidence.HeadSHA,
				RemoteHeadSHA:          readyEvidence.HeadSHA,
				ChangedFiles:           []string{"README.md"},
				Draft:                  true,
				State:                  "open",
				PolicyBundleID:         TransparaAIDraftPRPolicyBundleID,
				PolicyBundleHash:       TransparaAIDraftPRPolicyBundleHash(),
				AuthorityNonce:         "nonce-ready-pr-test",
				HumanApprovalRequired:  true,
				NoMergeOrDeployClaim:   true,
				ReadyForReviewRequired: true,
			},
			ReadyPREvidence: readyEvidence,
		}, nil
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("progressIssueScanLifecycle with configured ready PR runner: %v", err)
	}
	if calls != 1 {
		t.Fatalf("ready PR runner calls = %d, want 1", calls)
	}
	if len(progress.ReadyPRRuns) != 1 || !progress.ReadyPRRuns[0].Recorded {
		t.Fatalf("ready PR runs = %+v, want one recorded packet", progress.ReadyPRRuns)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 3 {
		t.Fatalf("ready role outputs = %+v, want strategist/reviewer/guardian", progress.ReadyRoleOutputs)
	}
	readyCompleted, err := rt.issueScanStageTaskCompleted(readyStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted ready stage: %v", err)
	}
	if !readyCompleted {
		t.Fatalf("surface_ready_for_Human_result_PR stage was not completed")
	}

	again, err := rt.progressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("second progressIssueScanLifecycle: %v", err)
	}
	if calls != 1 {
		t.Fatalf("ready PR runner calls after completion = %d, want still 1", calls)
	}
	if len(again.ReadyPRRuns) != 0 {
		t.Fatalf("second ready PR runs = %+v, want none after ready stage completed", again.ReadyPRRuns)
	}
}

func TestProgressIssueScanLifecycleRejectsReadyPREvidenceWithoutDraftReceipt(t *testing.T) {
	rt, writer, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	readyEvidence := IssueScanReadyPREvidence{
		RunID:                  runID,
		FactoryOrderID:         orderID,
		Repository:             "transpara-ai/hive",
		PRNumber:               321,
		PRURL:                  "https://github.com/transpara-ai/hive/pull/321",
		BaseRef:                "main",
		BaseSHA:                "dddddddddddddddddddddddddddddddddddddddd",
		HeadRef:                "codex/run-issue-001-repair",
		HeadSHA:                "cccccccccccccccccccccccccccccccccccccccc",
		State:                  "open",
		Draft:                  false,
		ReadyForReview:         true,
		MergeStateStatus:       "clean",
		CIStatus:               "success",
		ReadyStateReviewRef:    "https://github.com/transpara-ai/hive/pull/321#issuecomment-ready-state-review",
		ReadyStateReviewStatus: "passed",
		HumanApprovalRequired:  true,
	}
	body, err := IssueScanReadyPREvidenceArtifactBody(readyEvidence)
	if err != nil {
		t.Fatalf("IssueScanReadyPREvidenceArtifactBody: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, readyStage.ID, IssueScanReadyPREvidenceArtifactLabel, "application/json", body, []types.EventID{readyStage.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact ready PR evidence: %v", err)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err == nil || !strings.Contains(err.Error(), TransparaAIDraftPRReceiptArtifactLabel+" artifact is required") {
		t.Fatalf("progressIssueScanLifecycle error = %v, want missing draft PR receipt rejection", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 0 {
		t.Fatalf("ready role outputs = %+v, want none for missing receipt", progress.ReadyRoleOutputs)
	}
	readyCompleted, err := rt.issueScanStageTaskCompleted(readyStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted ready stage: %v", err)
	}
	if readyCompleted {
		t.Fatalf("surface_ready_for_Human_result_PR completed without draft PR receipt")
	}
}

func TestProgressIssueScanLifecycleRejectsReadyPREvidenceWithStaleDraftReceipt(t *testing.T) {
	rt, writer, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	readyEvidence := IssueScanReadyPREvidence{
		RunID:                  runID,
		FactoryOrderID:         orderID,
		Repository:             "transpara-ai/hive",
		PRNumber:               321,
		PRURL:                  "https://github.com/transpara-ai/hive/pull/321",
		BaseRef:                "main",
		BaseSHA:                "dddddddddddddddddddddddddddddddddddddddd",
		HeadRef:                "codex/run-issue-001-repair",
		HeadSHA:                "cccccccccccccccccccccccccccccccccccccccc",
		State:                  "open",
		Draft:                  false,
		ReadyForReview:         true,
		MergeStateStatus:       "clean",
		CIStatus:               "success",
		ReadyStateReviewRef:    "https://github.com/transpara-ai/hive/pull/321#issuecomment-ready-state-review",
		ReadyStateReviewStatus: "passed",
		HumanApprovalRequired:  true,
	}
	if err := attachIssueScanDraftPRReceiptForReadyTestWith(t, rt, writer, readyStage.ID, readyEvidence, func(receipt *TransparaAIDraftPRReceipt) {
		receipt.PolicyBundleHash = "sha256:stale"
	}); err != nil {
		t.Fatalf("attach stale draft PR receipt: %v", err)
	}
	body, err := IssueScanReadyPREvidenceArtifactBody(readyEvidence)
	if err != nil {
		t.Fatalf("IssueScanReadyPREvidenceArtifactBody: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, readyStage.ID, IssueScanReadyPREvidenceArtifactLabel, "application/json", body, []types.EventID{readyStage.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact ready PR evidence: %v", err)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err == nil || !strings.Contains(err.Error(), "draft PR receipt policy_bundle_hash") {
		t.Fatalf("progressIssueScanLifecycle error = %v, want stale draft PR receipt rejection", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 0 {
		t.Fatalf("ready role outputs = %+v, want none for stale receipt", progress.ReadyRoleOutputs)
	}
	readyCompleted, err := rt.issueScanStageTaskCompleted(readyStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted ready stage: %v", err)
	}
	if readyCompleted {
		t.Fatalf("surface_ready_for_Human_result_PR completed with stale draft PR receipt")
	}
}

func TestProgressIssueScanLifecycleRejectsDraftReadyPREvidence(t *testing.T) {
	rt, writer, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	readyEvidence := IssueScanReadyPREvidence{
		RunID:                  runID,
		FactoryOrderID:         orderID,
		Repository:             "transpara-ai/hive",
		PRNumber:               321,
		PRURL:                  "https://github.com/transpara-ai/hive/pull/321",
		HeadSHA:                "cccccccccccccccccccccccccccccccccccccccc",
		State:                  "open",
		Draft:                  true,
		ReadyForReview:         false,
		MergeStateStatus:       "clean",
		CIStatus:               "success",
		ReadyStateReviewRef:    "https://github.com/transpara-ai/hive/pull/321#issuecomment-ready-state-review",
		ReadyStateReviewStatus: "passed",
		HumanApprovalRequired:  true,
	}
	if err := attachIssueScanDraftPRReceiptForReadyTest(t, rt, writer, readyStage.ID, readyEvidence); err != nil {
		t.Fatalf("attach draft PR receipt: %v", err)
	}
	body, err := IssueScanReadyPREvidenceArtifactBody(readyEvidence)
	if err != nil {
		t.Fatalf("IssueScanReadyPREvidenceArtifactBody: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, readyStage.ID, IssueScanReadyPREvidenceArtifactLabel, "application/json", body, []types.EventID{readyStage.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact draft ready PR evidence: %v", err)
	}

	progress, err := rt.progressIssueScanLifecycle()
	if err == nil || !strings.Contains(err.Error(), "PR must be non-draft and ready_for_review") {
		t.Fatalf("progressIssueScanLifecycle error = %v, want non-draft ready_for_review rejection", err)
	}
	if countRecordedIssueScanRoleOutputs(progress.ReadyRoleOutputs) != 0 {
		t.Fatalf("ready role outputs = %+v, want none for rejected draft evidence", progress.ReadyRoleOutputs)
	}
	readyCompleted, err := rt.issueScanStageTaskCompleted(readyStage.ID)
	if err != nil {
		t.Fatalf("issueScanStageTaskCompleted ready stage: %v", err)
	}
	if readyCompleted {
		t.Fatalf("surface_ready_for_Human_result_PR completed from draft evidence")
	}
}

func issueScanCompletedImplementationFixtureForTest(t *testing.T) (*Runtime, *operatorRunLaunchWriter, IssueScanRunLaunchResult, string, work.Task) {
	t.Helper()
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	dispatch, err := rt.DispatchQueuedRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if _, err := rt.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		t.Fatalf("StartDispatchedIssueScanLifecycleStages: %v", err)
	}
	for _, stageID := range []string{
		"research_issue_and_repo_context",
		"debate_with_correct_civic_roles",
		"select_and_design_approach",
	} {
		if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), true); err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stageID, err)
		}
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle seed implementation task: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	implementationTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanImplementationTaskCanonicalID(orderID))
	if !ok {
		t.Fatalf("missing implementation task in %+v", tasks)
	}
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", issueScanOperateResultBodyForTest(), []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact initial Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive passed", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete initial implementation task: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle complete implementation stage: %v", err)
	}
	return rt, writer, queued, orderID, implementationTask
}

func issueScanReadyStageFixtureForTest(t *testing.T) (*Runtime, *operatorRunLaunchWriter, string, string, work.Task, work.Task) {
	t.Helper()
	rt, writer, queued, orderID, implementationTask := issueScanCompletedImplementationFixtureForTest(t)
	if err := appendIssueScanCodeReviewForTest(rt, writer, implementationTask.ID, "request_changes", "exact-head review found blockers", []string{"missing regression test"}); err != nil {
		t.Fatalf("append request_changes review: %v", err)
	}
	if err := rt.tasks.Reopen(writer.human, implementationTask.ID, "request_changes (review 1 of 3): exact-head review found blockers", []string{"missing regression test"}, []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Reopen implementation task: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle after request_changes: %v", err)
	}
	repairedBody := issueScanOperateResultBodyForTestWith(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccc",
		"codex/run-issue-001-repair",
		"pkg/hive/example.go | 14 ++++++++++++--\npkg/hive/example_test.go | 18 ++++++++++++++++++\n2 files changed, 30 insertions(+), 2 deletions(-)",
	)
	if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "Operate result", "text/plain", repairedBody, []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact repaired Operate result: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, implementationTask.ID, "validation output: go test ./pkg/hive -run TestProgressIssueScanLifecycleRecordsReadyRoleOutputsAndCompletesStage passed", []types.EventID{implementationTask.ID}, writer.conv); err != nil {
		t.Fatalf("Complete repaired implementation task: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle after repair completion: %v", err)
	}
	if err := appendIssueScanCodeReviewForTest(rt, writer, implementationTask.ID, "approve", "rerun review approved repaired exact head with zero blockers", nil); err != nil {
		t.Fatalf("append approving review: %v", err)
	}
	if _, err := rt.progressIssueScanLifecycle(); err != nil {
		t.Fatalf("progressIssueScanLifecycle after approving rerun: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks after blocker progress: %v", err)
	}
	readyStage, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "surface_ready_for_Human_result_PR"))
	if !ok {
		t.Fatalf("missing ready-for-Human stage task in %+v", tasks)
	}
	blocked, err := rt.tasks.IsBlocked(readyStage.ID)
	if err != nil {
		t.Fatalf("IsBlocked ready stage: %v", err)
	}
	if blocked {
		t.Fatalf("ready stage remains blocked before ready evidence is attached")
	}
	return rt, writer, queued.RunID, orderID, implementationTask, readyStage
}

func attachIssueScanDraftPRReceiptForReadyTest(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, stageTaskID types.EventID, evidence IssueScanReadyPREvidence) error {
	t.Helper()
	return attachIssueScanDraftPRReceiptForReadyTestWith(t, rt, writer, stageTaskID, evidence, nil)
}

func attachIssueScanDraftPRReceiptForReadyTestWith(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, stageTaskID types.EventID, evidence IssueScanReadyPREvidence, mutate func(*TransparaAIDraftPRReceipt)) error {
	t.Helper()
	receipt := TransparaAIDraftPRReceipt{
		Kind:                   transparaAIDraftPRReceiptKind,
		Repository:             strings.ToLower(strings.TrimSpace(evidence.Repository)),
		PRNumber:               evidence.PRNumber,
		PRURL:                  strings.TrimSpace(evidence.PRURL),
		BaseRef:                strings.TrimSpace(evidence.BaseRef),
		BaseSHA:                strings.TrimSpace(evidence.BaseSHA),
		HeadRef:                strings.TrimSpace(evidence.HeadRef),
		HeadSHA:                strings.TrimSpace(evidence.HeadSHA),
		RemoteHeadSHA:          strings.TrimSpace(evidence.HeadSHA),
		ChangedFiles:           []string{"README.md"},
		Draft:                  true,
		State:                  "open",
		PolicyBundleID:         TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash:       TransparaAIDraftPRPolicyBundleHash(),
		AuthorityNonce:         "nonce-ready-pr-test",
		HumanApprovalRequired:  true,
		NoMergeOrDeployClaim:   true,
		ReadyForReviewRequired: true,
	}
	if mutate != nil {
		mutate(&receipt)
	}
	body, err := transparaAIDraftPRReceiptBody(receipt)
	if err != nil {
		return err
	}
	return rt.tasks.AddArtifact(writer.human, stageTaskID, TransparaAIDraftPRReceiptArtifactLabel, "application/json", body, []types.EventID{stageTaskID}, writer.conv)
}

func TestCompleteIssueScanLifecycleStageRejectsMissingRequiredEvidence(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}

	evidence := issueScanStageRuntimeEvidenceForTest(t, stageID)
	evidence.EvidenceItems = evidence.EvidenceItems[:len(evidence.EvidenceItems)-1]
	if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, evidence, false); err == nil || !strings.Contains(err.Error(), "missing required evidence") {
		t.Fatalf("CompleteIssueScanLifecycleStage missing evidence error = %v, want required evidence refusal", err)
	}
	blocked, err := rt.tasks.IsBlocked(stageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked stage: %v", err)
	}
	if !blocked {
		t.Fatalf("stage task barrier released despite missing required evidence")
	}
	status, err := rt.tasks.GetCompatibilityStatus(stageTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus stage: %v", err)
	}
	if status == work.LegacyStatusCompleted {
		t.Fatalf("stage task completed despite missing required evidence")
	}
	artifacts, err := rt.tasks.ListArtifacts(stageTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts stage: %v", err)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) != nil {
		t.Fatalf("runtime evidence artifact recorded despite missing required evidence: %+v", artifacts)
	}
}

func TestCompleteIssueScanLifecycleStageRejectsMissingRoleOutput(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}

	evidence := issueScanStageRuntimeEvidenceForTest(t, stageID)
	evidence.RoleOutputs = evidence.RoleOutputs[:len(evidence.RoleOutputs)-1]
	if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, evidence, false); err == nil || !strings.Contains(err.Error(), "missing role output") {
		t.Fatalf("CompleteIssueScanLifecycleStage missing role output error = %v, want role output refusal", err)
	}
	blocked, err := rt.tasks.IsBlocked(stageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked stage: %v", err)
	}
	if !blocked {
		t.Fatalf("stage task barrier released despite missing role output")
	}
	status, err := rt.tasks.GetCompatibilityStatus(stageTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus stage: %v", err)
	}
	if status == work.LegacyStatusCompleted {
		t.Fatalf("stage task completed despite missing role output")
	}
	artifacts, err := rt.tasks.ListArtifacts(stageTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts stage: %v", err)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) != nil {
		t.Fatalf("runtime evidence artifact recorded despite missing role output: %+v", artifacts)
	}
}

func TestRecordIssueScanStageRoleOutputProjectsRoleRuntimeEvidence(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	result, err := rt.RecordIssueScanStageRoleOutput(queued.RunID, stageID, issueScanStageRoleOutputForTest(t, stageID, "strategist"))
	if err != nil {
		t.Fatalf("RecordIssueScanStageRoleOutput: %v", err)
	}
	if !result.Recorded || result.Role != "strategist" || result.OutputArtifactID == (types.EventID{}) {
		t.Fatalf("role output result = %+v, want recorded strategist artifact", result)
	}
	artifacts, err := rt.tasks.ListArtifacts(result.StageTaskID)
	if err != nil {
		t.Fatalf("ListArtifacts stage: %v", err)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRoleOutputArtifactLabel) == nil {
		t.Fatalf("missing %s artifact in %+v", IssueScanStageRoleOutputArtifactLabel, artifacts)
	}

	projection := BuildCivilizationAssemblyProjection(rt.store, 100)
	taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, result.StageTaskID.Value())
	if taskEvidence == nil || taskEvidence.RuntimeEvidenceStatus != "" {
		t.Fatalf("task evidence = %+v, want role output without stage runtime status", taskEvidence)
	}
	strategistOutput := issueScanRoleOutputContractByRole(taskEvidence.RoleOutputContracts, "strategist")
	if strategistOutput == nil || strategistOutput.EvidenceStatus != civilizationAssemblyQueuedPlanStepRuntimeStatus {
		t.Fatalf("strategist role output = %+v, want runtime evidence status", strategistOutput)
	}
	plannerOutput := issueScanRoleOutputContractByRole(taskEvidence.RoleOutputContracts, "planner")
	if plannerOutput == nil || plannerOutput.EvidenceStatus != "required_not_observed" {
		t.Fatalf("planner role output = %+v, want still unobserved", plannerOutput)
	}
	projectedStage := queuedRunLifecycleStageByID(projection.QueuedRunRequest.DevelopmentLifecycle, stageID)
	if projectedStage == nil || projectedStage.EvidenceStatus != civilizationAssemblyQueuedStageTaskDraftStatus {
		t.Fatalf("projected stage = %+v, want task draft pending runtime evidence", projectedStage)
	}
	projectedStep := issueScanOperatorPlanStepByRole(projection.QueuedRunRequest.AgentExecutionPlan, stageID, "strategist")
	if projectedStep == nil || projectedStep.EvidenceStatus != civilizationAssemblyQueuedPlanStepRuntimeStatus {
		t.Fatalf("projected strategist step = %+v, want role runtime evidence", projectedStep)
	}
}

func TestCivilizationAssemblyProjectionProjectsDirectRoleOutputArtifact(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}
	output := issueScanStageRoleOutputForTest(t, stageID, "strategist")
	output.Kind = issueScanStageRoleOutputArtifactKind
	output.RunID = ""
	output.FactoryOrderID = ""
	output.StageID = ""
	body, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("marshal role output: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, IssueScanStageRoleOutputArtifactLabel, issueScanStageRuntimeEvidenceMediaType, string(body), []types.EventID{stageTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact role output: %v", err)
	}

	projection := BuildCivilizationAssemblyProjection(rt.store, 100)
	taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, stageTask.ID.Value())
	if taskEvidence == nil || taskEvidence.RuntimeEvidenceStatus != "" {
		t.Fatalf("task evidence = %+v, want direct role output without stage runtime status", taskEvidence)
	}
	strategistOutput := issueScanRoleOutputContractByRole(taskEvidence.RoleOutputContracts, "strategist")
	if strategistOutput == nil || strategistOutput.EvidenceStatus != civilizationAssemblyQueuedPlanStepRuntimeStatus {
		t.Fatalf("strategist output = %+v, want direct role-output runtime status", strategistOutput)
	}
	projectedStage := queuedRunLifecycleStageByID(projection.QueuedRunRequest.DevelopmentLifecycle, stageID)
	if projectedStage == nil || projectedStage.EvidenceStatus != civilizationAssemblyQueuedStageTaskDraftStatus {
		t.Fatalf("projected stage = %+v, want task draft pending runtime evidence", projectedStage)
	}
}

func TestCompleteIssueScanLifecycleStageUsesRecordedRoleOutputs(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	for _, role := range []string{"strategist", "planner"} {
		if _, err := rt.RecordIssueScanStageRoleOutput(queued.RunID, stageID, issueScanStageRoleOutputForTest(t, stageID, role)); err != nil {
			t.Fatalf("RecordIssueScanStageRoleOutput %s: %v", role, err)
		}
	}
	evidence := issueScanStageRuntimeEvidenceForTest(t, stageID)
	evidence.RoleOutputs = nil
	completed, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, evidence, false)
	if err != nil {
		t.Fatalf("CompleteIssueScanLifecycleStage with recorded role outputs: %v", err)
	}
	if !completed.Completed || completed.EvidenceArtifactID == (types.EventID{}) {
		t.Fatalf("completion = %+v, want completed stage with role outputs from artifacts", completed)
	}
	body, ok := rt.tasks.GetArtifactBody(completed.EvidenceArtifactID)
	if !ok {
		t.Fatalf("missing runtime evidence artifact body %s", completed.EvidenceArtifactID)
	}
	var recorded IssueScanStageRuntimeEvidence
	if err := json.Unmarshal([]byte(body), &recorded); err != nil {
		t.Fatalf("unmarshal runtime evidence artifact: %v", err)
	}
	if len(recorded.RoleOutputs) != 2 {
		t.Fatalf("recorded role outputs = %+v, want strategist and planner from artifacts", recorded.RoleOutputs)
	}
	for _, role := range []string{"strategist", "planner"} {
		output := issueScanRuntimeRoleOutputByRole(recorded.RoleOutputs, role)
		if output == nil || len(output.EvidenceRefs) == 0 {
			t.Fatalf("recorded role output %s = %+v, want artifact-backed evidence refs", role, output)
		}
	}
}

func TestCompleteIssueScanLifecycleStageRejectsIdentitySpoofing(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*IssueScanStageRuntimeEvidence)
		wantErr string
	}{
		{
			name:    "kind mismatch",
			mutate:  func(e *IssueScanStageRuntimeEvidence) { e.Kind = "wrong_kind" },
			wantErr: "kind",
		},
		{
			name:    "lifecycle version mismatch",
			mutate:  func(e *IssueScanStageRuntimeEvidence) { e.LifecycleVersion = "wrong_version" },
			wantErr: "lifecycle_version",
		},
		{
			name:    "run mismatch",
			mutate:  func(e *IssueScanStageRuntimeEvidence) { e.RunID = "run_other" },
			wantErr: "run_id",
		},
		{
			name:    "factory order mismatch",
			mutate:  func(e *IssueScanStageRuntimeEvidence) { e.FactoryOrderID = "fo_other" },
			wantErr: "factory_order_id",
		},
		{
			name:    "stage mismatch",
			mutate:  func(e *IssueScanStageRuntimeEvidence) { e.StageID = "debate_with_correct_civic_roles" },
			wantErr: "stage_id",
		},
		{
			name:    "bad status",
			mutate:  func(e *IssueScanStageRuntimeEvidence) { e.Status = "failed" },
			wantErr: "not completable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, writer := newRunLaunchDispatchRuntime(t)
			queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
				OperatorID: IssueScanOperatorID("Michael Saucier"),
				Issues: []GitHubIssueCandidate{{
					Repo:   "transpara-ai/hive",
					Number: 321,
					Title:  "Teach the Civilization to scan issues",
					URL:    "https://github.com/transpara-ai/hive/issues/321",
					Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
				}},
				Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
			}, nil)
			if err != nil {
				t.Fatalf("QueueIssueScanRunLaunch: %v", err)
			}
			if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
				t.Fatalf("DispatchQueuedRunLaunch: %v", err)
			}
			orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
			if err != nil {
				t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
			}
			tasks, err := rt.tasks.List(20)
			if err != nil {
				t.Fatalf("List tasks: %v", err)
			}
			stageID := "research_issue_and_repo_context"
			stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
			if !ok {
				t.Fatalf("missing stage task in %+v", tasks)
			}

			evidence := issueScanStageRuntimeEvidenceForTest(t, stageID)
			tc.mutate(&evidence)
			if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, evidence, false); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("CompleteIssueScanLifecycleStage %s error = %v, want %q", tc.name, err, tc.wantErr)
			}
			blocked, err := rt.tasks.IsBlocked(stageTask.ID)
			if err != nil {
				t.Fatalf("IsBlocked stage: %v", err)
			}
			if !blocked {
				t.Fatalf("stage task barrier released despite %s", tc.name)
			}
			status, err := rt.tasks.GetCompatibilityStatus(stageTask.ID)
			if err != nil {
				t.Fatalf("GetCompatibilityStatus stage: %v", err)
			}
			if status == work.LegacyStatusCompleted {
				t.Fatalf("stage task completed despite %s", tc.name)
			}
			artifacts, err := rt.tasks.ListArtifacts(stageTask.ID)
			if err != nil {
				t.Fatalf("ListArtifacts stage: %v", err)
			}
			if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) != nil {
				t.Fatalf("runtime evidence artifact recorded despite %s: %+v", tc.name, artifacts)
			}
		})
	}
}

func TestCivilizationAssemblyProjectionDoesNotCompleteStageWithoutRuntimeEvidence(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}
	if _, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, stageID); err != nil {
		t.Fatalf("AdvanceIssueScanLifecycleStage: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, stageTask.ID, "direct completion without governed runtime evidence", []types.EventID{stageTask.ID}, writer.conv); err != nil {
		t.Fatalf("direct Complete stage: %v", err)
	}

	projection := BuildCivilizationAssemblyProjection(rt.store, 100)
	taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, stageTask.ID.Value())
	if taskEvidence == nil || taskEvidence.Status != "work_task_completed" || len(taskEvidence.RuntimeEvidenceRefs) != 0 || taskEvidence.RuntimeEvidenceStatus != "" {
		t.Fatalf("direct-completed task evidence = %+v, want completed Work task without runtime evidence status", taskEvidence)
	}
	projectedStage := queuedRunLifecycleStageByID(projection.QueuedRunRequest.DevelopmentLifecycle, stageID)
	if projectedStage == nil || projectedStage.EvidenceStatus != civilizationAssemblyQueuedStageTaskDraftStatus {
		t.Fatalf("direct-completed queued stage = %+v, want task draft pending runtime evidence", projectedStage)
	}
}

func TestCivilizationAssemblyProjectionDoesNotCompleteStageWithThinRuntimeEvidence(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}
	if _, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, stageID); err != nil {
		t.Fatalf("AdvanceIssueScanLifecycleStage: %v", err)
	}
	thinEvidence := IssueScanStageRuntimeEvidence{
		Kind:             issueScanStageRuntimeEvidenceArtifactKind,
		LifecycleVersion: issueScanLifecycleVersion,
		RunID:            queued.RunID,
		FactoryOrderID:   orderID,
		StageID:          stageID,
		Status:           "completed",
		Summary:          "thin forged runtime evidence",
	}
	body, err := json.MarshalIndent(thinEvidence, "", "  ")
	if err != nil {
		t.Fatalf("marshal thin evidence: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, IssueScanStageRuntimeEvidenceArtifactLabel, issueScanStageRuntimeEvidenceMediaType, string(body), []types.EventID{stageTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact thin runtime evidence: %v", err)
	}
	if err := rt.tasks.Complete(writer.human, stageTask.ID, "direct completion with thin runtime evidence", []types.EventID{stageTask.ID}, writer.conv); err != nil {
		t.Fatalf("direct Complete stage: %v", err)
	}

	projection := BuildCivilizationAssemblyProjection(rt.store, 100)
	taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, stageTask.ID.Value())
	if taskEvidence == nil || taskEvidence.Status != "work_task_completed" || taskEvidence.RuntimeEvidenceStatus != civilizationAssemblyQueuedStageRuntimeStatus {
		t.Fatalf("thin-runtime task evidence = %+v, want recorded runtime evidence without completed runtime status", taskEvidence)
	}
	projectedStage := queuedRunLifecycleStageByID(projection.QueuedRunRequest.DevelopmentLifecycle, stageID)
	if projectedStage == nil || projectedStage.EvidenceStatus != civilizationAssemblyQueuedStageRuntimeStatus {
		t.Fatalf("thin-runtime queued stage = %+v, want runtime evidence recorded", projectedStage)
	}
}

func TestCompleteIssueScanLifecycleStageRejectsLaterStageBeforePredecessor(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageID := "debate_with_correct_civic_roles"
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}

	if _, err := rt.CompleteIssueScanLifecycleStage(queued.RunID, stageID, issueScanStageRuntimeEvidenceForTest(t, stageID), false); err == nil || !strings.Contains(err.Error(), "previous stage") {
		t.Fatalf("CompleteIssueScanLifecycleStage later-stage error = %v, want previous stage blocker", err)
	}
	blocked, err := rt.tasks.IsBlocked(stageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked later stage: %v", err)
	}
	if !blocked {
		t.Fatalf("later stage barrier released before predecessor completion")
	}
	status, err := rt.tasks.GetCompatibilityStatus(stageTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus later stage: %v", err)
	}
	if status == work.LegacyStatusCompleted {
		t.Fatalf("later stage completed before predecessor completion")
	}
	artifacts, err := rt.tasks.ListArtifacts(stageTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts later stage: %v", err)
	}
	if issueScanArtifactByLabel(artifacts, IssueScanStageRuntimeEvidenceArtifactLabel) != nil {
		t.Fatalf("runtime evidence artifact recorded for out-of-order completion: %+v", artifacts)
	}
}

func TestCompleteIssueScanLifecycleStageCompletesLifecycleWithoutCompletingFactoryOrder(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(50)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	parentTask, ok := findTaskByCanonicalTaskIDForTest(tasks, "")
	if !ok {
		t.Fatalf("missing parent FactoryOrder task in %+v", tasks)
	}

	lifecycle := issueScanDevelopmentLifecycle()
	var final IssueScanStageCompletionResult
	for i, stage := range lifecycle {
		advanceNext := i < len(lifecycle)-1
		final, err = rt.CompleteIssueScanLifecycleStage(queued.RunID, stage.ID, issueScanStageRuntimeEvidenceForTest(t, stage.ID), advanceNext)
		if err != nil {
			t.Fatalf("CompleteIssueScanLifecycleStage %s: %v", stage.ID, err)
		}
		if !final.Completed || final.StageID != safeRunLaunchID(stage.ID) || final.EvidenceArtifactID == (types.EventID{}) {
			t.Fatalf("completion %s = %+v, want completed stage with runtime evidence", stage.ID, final)
		}
		if advanceNext && final.NextAdvance == nil {
			t.Fatalf("completion %s next advance = nil, want next stage release", stage.ID)
		}
		if !advanceNext && final.NextAdvance != nil {
			t.Fatalf("completion %s next advance = %+v, want no next advance when advanceNext=false", stage.ID, final.NextAdvance)
		}
	}
	if !final.LifecycleComplete {
		t.Fatalf("final completion = %+v, want lifecycle complete marker after final stage", final)
	}
	parentStatus, err := rt.tasks.GetCompatibilityStatus(parentTask.ID)
	if err != nil {
		t.Fatalf("GetCompatibilityStatus parent: %v", err)
	}
	if parentStatus == work.LegacyStatusCompleted {
		t.Fatalf("parent FactoryOrder task completed by issue-scan stage lifecycle")
	}

	projection := BuildCivilizationAssemblyProjection(rt.store, 200)
	parentEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, parentTask.ID.Value())
	if parentEvidence == nil || parentEvidence.Status == "work_task_completed" || len(parentEvidence.RuntimeEvidenceRefs) > 0 {
		t.Fatalf("parent evidence = %+v, want incomplete parent without stage runtime refs", parentEvidence)
	}
	for _, stage := range lifecycle {
		projectedStage := queuedRunLifecycleStageByID(projection.QueuedRunRequest.DevelopmentLifecycle, stage.ID)
		if projectedStage == nil || projectedStage.EvidenceStatus != civilizationAssemblyQueuedStageCompletedStatus {
			t.Fatalf("projected stage %s = %+v, want completed runtime evidence", stage.ID, projectedStage)
		}
	}
	if len(projection.WorkEvidenceSummary.TestRunRefs) > 0 || len(projection.WorkEvidenceSummary.GateResultRefs) > 0 {
		t.Fatalf("stage completion produced test/gate refs: tests=%+v gates=%+v", projection.WorkEvidenceSummary.TestRunRefs, projection.WorkEvidenceSummary.GateResultRefs)
	}
}

func TestCivilizationAssemblyProjectionMarksRuntimeEvidenceRecordedBeforeTaskCompletion(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	stageID := "research_issue_and_repo_context"
	stageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, stageID))
	if !ok {
		t.Fatalf("missing stage task in %+v", tasks)
	}

	evidence := issueScanStageRuntimeEvidenceForTest(t, stageID)
	evidence.Kind = issueScanStageRuntimeEvidenceArtifactKind
	evidence.LifecycleVersion = issueScanLifecycleVersion
	evidence.RunID = queued.RunID
	evidence.FactoryOrderID = orderID
	evidence.StageID = stageID
	evidence.Status = "unexpected_artifact_status"
	body, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatalf("marshal runtime evidence: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, IssueScanStageRuntimeEvidenceArtifactLabel, issueScanExecutionPlanArtifactMediaType, string(body), []types.EventID{stageTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact runtime evidence: %v", err)
	}

	projection := BuildCivilizationAssemblyProjection(rt.store, 100)
	taskEvidence := civilizationProjectionTaskEvidenceByID(projection.WorkEvidenceSummary.Tasks, stageTask.ID.Value())
	if taskEvidence == nil || taskEvidence.Status == "work_task_completed" || taskEvidence.RuntimeEvidenceStatus != civilizationAssemblyQueuedStageRuntimeStatus {
		t.Fatalf("runtime-only task evidence = %+v, want recorded runtime evidence without task completion", taskEvidence)
	}
	projectedStage := queuedRunLifecycleStageByID(projection.QueuedRunRequest.DevelopmentLifecycle, stageID)
	if projectedStage == nil || projectedStage.EvidenceStatus != civilizationAssemblyQueuedStageRuntimeStatus {
		t.Fatalf("runtime-only queued stage = %+v, want runtime evidence recorded", projectedStage)
	}
	strategistOutput := issueScanRoleOutputContractByRole(taskEvidence.RoleOutputContracts, "strategist")
	if strategistOutput == nil || strategistOutput.EvidenceStatus != civilizationAssemblyQueuedPlanStepRuntimeStatus {
		t.Fatalf("runtime-only strategist output = %+v, want runtime evidence recorded", strategistOutput)
	}
}

func TestAdvanceIssueScanLifecycleStageReportsAlreadyReadyWithIncompletePredecessor(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	secondStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "debate_with_correct_civic_roles"))
	if !ok {
		t.Fatalf("missing second stage task in %+v", tasks)
	}
	if err := rt.tasks.UnblockTask(writer.human, secondStageTask.ID, []types.EventID{secondStageTask.ID}, writer.conv); err != nil {
		t.Fatalf("UnblockTask second stage: %v", err)
	}

	if _, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, "debate_with_correct_civic_roles"); err == nil || !strings.Contains(err.Error(), "already unblocked before previous stage") {
		t.Fatalf("AdvanceIssueScanLifecycleStage already-ready inconsistent error = %v, want predecessor warning", err)
	}
}

func TestAdvanceIssueScanLifecycleStageRejectsNonIssueScanBeforeDispatch(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	appendRunLaunchRequestWithBrief(t, rt.store, writer, "run_generic_advance", json.RawMessage(`{"goal":"generic queued run"}`))

	if _, err := rt.AdvanceIssueScanLifecycleStage("run_generic_advance", ""); err == nil || !strings.Contains(err.Error(), "not an issue-scan run") {
		t.Fatalf("AdvanceIssueScanLifecycleStage generic error = %v, want issue-scan rejection", err)
	}
	tasks, err := rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("generic run was dispatched before type validation: %+v", tasks)
	}
}

func TestAdvanceIssueScanLifecycleStageRejectsUnexpectedDependencies(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	queued, err := QueueIssueScanRunLaunch(rt.store, writer.factory, writer.signer, writer.human, writer.conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Michael Saucier"),
		Issues: []GitHubIssueCandidate{{
			Repo:   "transpara-ai/hive",
			Number: 321,
			Title:  "Teach the Civilization to scan issues",
			URL:    "https://github.com/transpara-ai/hive/issues/321",
			Body:   "The Civilization should scan Transpara-AI repos.\nThen create a ready-for-Human PR.",
		}},
		Budget: RunLaunchBudget{MaxIterations: 12, MaxCostUSD: 25},
	}, nil)
	if err != nil {
		t.Fatalf("QueueIssueScanRunLaunch: %v", err)
	}
	if _, err := rt.DispatchQueuedRunLaunch(queued.RunID); err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	orderID, err := factoryOrderIDForRunLaunch(queued.RunID)
	if err != nil {
		t.Fatalf("factoryOrderIDForRunLaunch: %v", err)
	}
	tasks, err := rt.tasks.List(20)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	firstStageTask, ok := findTaskByCanonicalTaskIDForTest(tasks, issueScanLifecycleStageTaskCanonicalID(orderID, "research_issue_and_repo_context"))
	if !ok {
		t.Fatalf("missing first stage task in %+v", tasks)
	}
	extra, err := rt.tasks.Create(writer.human, "Unexpected blocker", "Not part of the issue-scan stage scheduling chain.", []types.EventID{firstStageTask.ID}, writer.conv, work.PriorityMedium)
	if err != nil {
		t.Fatalf("Create unexpected blocker: %v", err)
	}
	if err := rt.tasks.AddDependency(writer.human, firstStageTask.ID, extra.ID, []types.EventID{firstStageTask.ID, extra.ID}, writer.conv); err != nil {
		t.Fatalf("AddDependency unexpected blocker: %v", err)
	}

	if _, err := rt.AdvanceIssueScanLifecycleStage(queued.RunID, ""); err == nil || !strings.Contains(err.Error(), "unexpected dependencies") {
		t.Fatalf("AdvanceIssueScanLifecycleStage unexpected dependency error = %v, want refusal", err)
	}
	blocked, err := rt.tasks.IsBlocked(firstStageTask.ID)
	if err != nil {
		t.Fatalf("IsBlocked first stage: %v", err)
	}
	if !blocked {
		t.Fatalf("first stage was unblocked despite unexpected dependency")
	}
}

func TestVerifyIssueScanStageTaskContractsRejectsMissingContract(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	head, err := rt.store.Head()
	if err != nil || head.IsNone() {
		t.Fatalf("Head: %v", err)
	}
	stageTask, err := rt.tasks.CreateV39(writer.human, work.TaskCreateOptions{
		Title:                  "Issue-scan stage: Missing output contract",
		CanonicalTaskID:        "tsk_missing_contract",
		FactoryOrderID:         "fo_missing_contract",
		RequirementIDs:         []string{"req_missing_contract"},
		AcceptanceCriterionIDs: []string{"ac_missing_contract"},
		Cell:                   "planning",
		RiskClass:              "high",
	}, []types.EventID{head.Unwrap().ID()}, writer.conv)
	if err != nil {
		t.Fatalf("CreateV39 stage task: %v", err)
	}
	for _, label := range []string{work.GateDefinitionOfDone, work.GateAcceptanceCriteria, work.GateTestPlan} {
		if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, label, "text/markdown", "present", []types.EventID{stageTask.ID}, writer.conv); err != nil {
			t.Fatalf("AddArtifact %s: %v", label, err)
		}
	}
	if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, IssueScanStageRoleContractArtifactLabel, issueScanExecutionPlanArtifactMediaType, `{"kind":"issue_scan_stage_role_contract"}`, []types.EventID{stageTask.ID}, writer.conv); err != nil {
		t.Fatalf("AddArtifact role contract: %v", err)
	}

	err = rt.verifyIssueScanStageTaskContracts(&issueScanStageAdvanceTarget{
		Draft:  issueScanLifecycleStageTaskDraft{StageID: "research_issue_and_repo_context"},
		TaskID: stageTask.ID,
	})
	if err == nil || !strings.Contains(err.Error(), IssueScanStageOutputContractArtifactLabel) {
		t.Fatalf("verifyIssueScanStageTaskContracts error = %v, want missing output contract", err)
	}
}

func TestVerifyIssueScanStageTaskContractsRejectsMissingReadinessGate(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	head, err := rt.store.Head()
	if err != nil || head.IsNone() {
		t.Fatalf("Head: %v", err)
	}
	stageTask, err := rt.tasks.CreateV39(writer.human, work.TaskCreateOptions{
		Title:                  "Issue-scan stage: Missing readiness gate",
		CanonicalTaskID:        "tsk_missing_gate",
		FactoryOrderID:         "fo_missing_gate",
		RequirementIDs:         []string{"req_missing_gate"},
		AcceptanceCriterionIDs: []string{"ac_missing_gate"},
		Cell:                   "planning",
		RiskClass:              "high",
	}, []types.EventID{head.Unwrap().ID()}, writer.conv)
	if err != nil {
		t.Fatalf("CreateV39 stage task: %v", err)
	}
	for _, artifact := range []struct {
		label     string
		mediaType string
		body      string
	}{
		{work.GateAcceptanceCriteria, "text/markdown", "present"},
		{work.GateTestPlan, "text/markdown", "present"},
		{IssueScanStageRoleContractArtifactLabel, issueScanExecutionPlanArtifactMediaType, `{"kind":"issue_scan_stage_role_contract"}`},
		{IssueScanStageOutputContractArtifactLabel, issueScanExecutionPlanArtifactMediaType, `{"kind":"issue_scan_stage_output_contract"}`},
	} {
		if err := rt.tasks.AddArtifact(writer.human, stageTask.ID, artifact.label, artifact.mediaType, artifact.body, []types.EventID{stageTask.ID}, writer.conv); err != nil {
			t.Fatalf("AddArtifact %s: %v", artifact.label, err)
		}
	}

	err = rt.verifyIssueScanStageTaskContracts(&issueScanStageAdvanceTarget{
		Draft:  issueScanLifecycleStageTaskDraft{StageID: "research_issue_and_repo_context"},
		TaskID: stageTask.ID,
	})
	if err == nil || !strings.Contains(err.Error(), work.GateDefinitionOfDone) {
		t.Fatalf("verifyIssueScanStageTaskContracts error = %v, want missing definition_of_done", err)
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

func issueScanArtifactLabels(artifacts []work.ArtifactEvent) []string {
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, artifact.Label)
	}
	return out
}

func issueScanArtifactByLabel(artifacts []work.ArtifactEvent, label string) *work.ArtifactEvent {
	for i := range artifacts {
		if artifacts[i].Label == label {
			return &artifacts[i]
		}
	}
	return nil
}

func issueScanCompletionByStageForTest(completions []IssueScanStageCompletionResult, stageID string) *IssueScanStageCompletionResult {
	for i := range completions {
		if completions[i].StageID == stageID {
			return &completions[i]
		}
	}
	return nil
}

func containsIssueScanValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func countIssueScanValues(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func containsEventID(values []types.EventID, want types.EventID) bool {
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

func civilizationProjectionPlanStepByRole(steps []OperatorQueuedRunAgentPlanStep, stageID, role string) *OperatorQueuedRunAgentPlanStep {
	for i := range steps {
		if steps[i].StageID == stageID && steps[i].Role == role {
			return &steps[i]
		}
	}
	return nil
}

func issueScanRoleOutputContractByRole(contracts []CivilizationAssemblyRoleOutputContract, role string) *CivilizationAssemblyRoleOutputContract {
	for i := range contracts {
		if contracts[i].Role == role {
			return &contracts[i]
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

func issueScanStageRuntimeEvidenceForTest(t *testing.T, stageID string) IssueScanStageRuntimeEvidence {
	t.Helper()
	lifecycle := issueScanDevelopmentLifecycle()
	stage, ok := issueScanLifecycleStageByID(lifecycle, stageID)
	if !ok {
		t.Fatalf("missing lifecycle stage %q", stageID)
	}
	plan, err := issueScanAgentExecutionPlan(lifecycle)
	if err != nil {
		t.Fatalf("issueScanAgentExecutionPlan: %v", err)
	}
	refBase := "test://" + safeRunLaunchID(stageID)
	items := make([]IssueScanStageRuntimeEvidenceItem, 0, len(stage.RequiredEvidence))
	for _, key := range stage.RequiredEvidence {
		items = append(items, IssueScanStageRuntimeEvidenceItem{
			Key:          key,
			Summary:      "test evidence for " + key,
			EvidenceRefs: []string{refBase + "/evidence/" + safeRunLaunchID(key)},
		})
	}
	roleOutputs := []IssueScanStageRuntimeRoleOutput{}
	for _, step := range plan {
		if step.StageID != stageID {
			continue
		}
		outputs := make([]IssueScanStageRuntimeEvidenceItem, 0, len(step.RequiredOutputs))
		for _, key := range step.RequiredOutputs {
			outputs = append(outputs, IssueScanStageRuntimeEvidenceItem{
				Key:          key,
				Summary:      "test output for " + key,
				EvidenceRefs: []string{refBase + "/roles/" + safeRunLaunchID(step.Role) + "/" + safeRunLaunchID(key)},
			})
		}
		roleOutputs = append(roleOutputs, IssueScanStageRuntimeRoleOutput{
			Role:         step.Role,
			Summary:      "test role output for " + step.Role,
			EvidenceRefs: []string{refBase + "/roles/" + safeRunLaunchID(step.Role)},
			Outputs:      outputs,
		})
	}
	return IssueScanStageRuntimeEvidence{
		Summary:       "test governed runtime evidence for " + stage.ID,
		EvidenceItems: items,
		RoleOutputs:   roleOutputs,
		SourceRefs:    []string{refBase + "/source"},
	}
}

func issueScanStageRoleOutputForTest(t *testing.T, stageID, role string) IssueScanStageRoleOutputEvidence {
	t.Helper()
	lifecycle := issueScanDevelopmentLifecycle()
	if _, ok := issueScanLifecycleStageByID(lifecycle, stageID); !ok {
		t.Fatalf("missing lifecycle stage %q", stageID)
	}
	plan, err := issueScanAgentExecutionPlan(lifecycle)
	if err != nil {
		t.Fatalf("issueScanAgentExecutionPlan: %v", err)
	}
	step := issueScanAgentPlanStepByRole(plan, stageID, role)
	if step == nil {
		t.Fatalf("missing plan step stage=%q role=%q", stageID, role)
	}
	refBase := "test://" + safeRunLaunchID(stageID) + "/roles/" + safeRunLaunchID(role)
	outputs := make([]IssueScanStageRuntimeEvidenceItem, 0, len(step.RequiredOutputs))
	for _, key := range step.RequiredOutputs {
		outputs = append(outputs, IssueScanStageRuntimeEvidenceItem{
			Key:          key,
			Summary:      "test output for " + key,
			EvidenceRefs: []string{refBase + "/" + safeRunLaunchID(key)},
		})
	}
	return IssueScanStageRoleOutputEvidence{
		Role:         role,
		Summary:      "test role output for " + role,
		EvidenceRefs: []string{refBase},
		Outputs:      outputs,
		SourceRefs:   []string{refBase + "/source"},
	}
}

func issueScanEvidenceItemForTest(stageID, role, key string) IssueScanStageRuntimeEvidenceItem {
	refBase := "test://" + safeRunLaunchID(stageID) + "/roles/" + safeRunLaunchID(role)
	return IssueScanStageRuntimeEvidenceItem{
		Key:          key,
		Summary:      "test stage evidence for " + key,
		EvidenceRefs: []string{refBase + "/stage-evidence/" + safeRunLaunchID(key)},
	}
}

func issueScanOperateResultBodyForTest() string {
	return issueScanOperateResultBodyForTestWith(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"codex/run-issue-001",
		"pkg/hive/example.go | 12 ++++++++++++\n1 file changed, 12 insertions(+)\n",
	)
}

func issueScanOperateResultBodyForTestWith(base, head, branch, stat string) string {
	base = strings.TrimSpace(base)
	head = strings.TrimSpace(head)
	branch = strings.TrimSpace(branch)
	stat = strings.TrimSpace(stat)
	if base == "" {
		base = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}
	if head == "" {
		head = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}
	if branch == "" {
		branch = "codex/run-issue-001"
	}
	if stat == "" {
		stat = "pkg/hive/example.go | 12 ++++++++++++\n1 file changed, 12 insertions(+)"
	}
	return "commit: " + head + "\n" +
		"base: " + base + "\n" +
		"head: " + head + "\n" +
		"range: " + base + ".." + head + "\n" +
		"branch: " + branch + "\n\n" +
		stat + "\n"
}

func appendIssueScanCodeReviewForTest(rt *Runtime, writer *operatorRunLaunchWriter, taskID types.EventID, verdict, summary string, issues []string) error {
	content := event.CodeReviewContent{
		TaskID:     taskID.Value(),
		Verdict:    verdict,
		Summary:    summary,
		Issues:     issues,
		Confidence: 0.92,
	}
	ev, err := writer.factory.Create(event.EventTypeCodeReviewSubmitted, writer.human, content, []types.EventID{taskID}, writer.conv, rt.store, writer.signer)
	if err != nil {
		return err
	}
	_, err = rt.store.Append(ev)
	return err
}

func issueScanRoleOutputArtifactsForTest(t *testing.T, artifacts []work.ArtifactEvent) map[string]*IssueScanStageRoleOutputEvidence {
	t.Helper()
	out := map[string]*IssueScanStageRoleOutputEvidence{}
	for _, artifact := range artifacts {
		parsed, ok, err := issueScanStageRoleOutputArtifact(artifact.ID.Value(), artifact.Label, artifact.Body)
		if err != nil {
			t.Fatalf("parse role output artifact %s: %v", artifact.ID.Value(), err)
		}
		if !ok {
			continue
		}
		value := parsed
		out[value.Role] = &value
	}
	return out
}

func issueScanStageRuntimeEvidenceItemByKey(items []IssueScanStageRuntimeEvidenceItem, key string) *IssueScanStageRuntimeEvidenceItem {
	for i := range items {
		if items[i].Key == key {
			return &items[i]
		}
	}
	return nil
}

func issueScanRuntimeRoleOutputByRole(outputs []IssueScanStageRuntimeRoleOutput, role string) *IssueScanStageRuntimeRoleOutput {
	for i := range outputs {
		if outputs[i].Role == role {
			return &outputs[i]
		}
	}
	return nil
}

func issueScanAgentPlanStepByRole(steps []issueScanAgentPlanStep, stageID, role string) *issueScanAgentPlanStep {
	for i := range steps {
		if steps[i].StageID == stageID && steps[i].Role == role {
			return &steps[i]
		}
	}
	return nil
}

func issueScanOperatorPlanStepByRole(steps []OperatorQueuedRunAgentPlanStep, stageID, role string) *OperatorQueuedRunAgentPlanStep {
	for i := range steps {
		if steps[i].StageID == stageID && steps[i].Role == role {
			return &steps[i]
		}
	}
	return nil
}

func issueScanGitRepoWithOrigin(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	return issueScanGitRepoAtOrigin(t, dir, remote)
}

func issueScanGitRepoAtOrigin(t *testing.T, dir, remote string) string {
	t.Helper()
	issueScanRunTestCommand(t, "", "git", "init", dir)
	issueScanRunTestCommand(t, dir, "git", "remote", "add", "origin", remote)
	return dir
}

func issueScanRunTestCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
}
