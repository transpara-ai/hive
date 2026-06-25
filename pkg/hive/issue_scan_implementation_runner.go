package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const issueScanImplementationRunnerContextKind = "issue_scan_implementation_runner_context"

// IssueScanImplementationRunner receives the concrete implementation Work task
// context after the design stage has completed. It may implement on a branch and
// must return an Operate result packet; the runtime records that packet through
// the normal Work artifact/completion path before any issue-scan stage advances.
type IssueScanImplementationRunner func(context.Context, IssueScanImplementationRunnerContext) (IssueScanImplementationRunnerResult, error)

type IssueScanImplementationRunnerArtifact struct {
	Label     string `json:"label"`
	MediaType string `json:"media_type,omitempty"`
	Body      string `json:"body"`
}

type IssueScanImplementationRunnerContext struct {
	Kind                         string                                  `json:"kind"`
	LifecycleVersion             string                                  `json:"lifecycle_version"`
	RunID                        string                                  `json:"run_id"`
	FactoryOrderID               string                                  `json:"factory_order_id"`
	Repository                   string                                  `json:"repository"`
	RepoPath                     string                                  `json:"repo_path"`
	ContainmentWatchRoots        []string                                `json:"containment_watch_roots,omitempty"`
	ImplementationTaskID         string                                  `json:"implementation_task_id"`
	ImplementationStageTaskID    string                                  `json:"implementation_stage_task_id"`
	DesignStageTaskID            string                                  `json:"design_stage_task_id"`
	DesignRuntimeEvidenceRef     string                                  `json:"design_runtime_evidence_ref"`
	SelectedIssue                IssueScanStageRoleOutputIssue           `json:"selected_issue"`
	TargetRepos                  []string                                `json:"target_repos,omitempty"`
	DesignOutputs                []IssueScanStageRuntimeEvidenceItem     `json:"design_outputs,omitempty"`
	ImplementationTaskContext    json.RawMessage                         `json:"implementation_task_context,omitempty"`
	ImplementationReadinessGates []IssueScanImplementationRunnerArtifact `json:"implementation_readiness_gates,omitempty"`
	BoundaryDisclaimers          []string                                `json:"boundary_disclaimers,omitempty"`
}

type IssueScanImplementationRunnerResult struct {
	OperateResultBody string `json:"operate_result_body"`
	CompletionSummary string `json:"completion_summary"`
}

type IssueScanImplementationRunnerRecordResult struct {
	RunID                     string
	FactoryOrderID            string
	Repository                string
	ImplementationTaskID      types.EventID
	ImplementationStageTaskID types.EventID
	OperateArtifactID         types.EventID
	CompletionEventID         types.EventID
	OperateBranch             string
	OperateCommit             string
	Recorded                  bool
}

// IssueScanImplementationRunnerContext returns the current concrete
// implementation context for a queued issue-scan run. It is ready only after
// select/design has completed, the implementation task exists, and that task is
// unblocked and not already completed.
func (r *Runtime) IssueScanImplementationRunnerContext(runID string) (IssueScanImplementationRunnerContext, error) {
	runnerContext, ready, err := r.issueScanImplementationRunnerContext(runID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, err
	}
	if !ready {
		return IssueScanImplementationRunnerContext{}, fmt.Errorf("issue-scan run %q has no implementation task ready to run", strings.TrimSpace(runID))
	}
	return runnerContext, nil
}

func (r *Runtime) RunConfiguredIssueScanImplementationRunners(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanImplementationRunnerRecordResult, error) {
	if r == nil || r.issueScanImplementationRunner == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanImplementationRunnerRecordResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RunConfiguredIssueScanImplementationRunner(ctx, runID)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ready {
			out = append(out, recorded)
		}
	}
	return out, errors.Join(errs...)
}

func (r *Runtime) RunConfiguredIssueScanImplementationRunner(ctx context.Context, runID string) (IssueScanImplementationRunnerRecordResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanImplementationRunnerRecordResult{RunID: runID}
	if r == nil || r.issueScanImplementationRunner == nil {
		return result, false, nil
	}
	runnerContext, ready, err := r.issueScanImplementationRunnerContext(runID)
	if err != nil || !ready {
		return result, ready, err
	}
	runnerResult, err := r.issueScanImplementationRunner(ctx, runnerContext)
	if err != nil {
		return result, true, err
	}
	recorded, err := r.RecordIssueScanImplementationRunnerResult(runID, runnerResult)
	return recorded, true, err
}

func (r *Runtime) RecordIssueScanImplementationRunnerResult(runID string, runnerResult IssueScanImplementationRunnerResult) (IssueScanImplementationRunnerRecordResult, error) {
	runID = strings.TrimSpace(runID)
	runnerContext, ready, err := r.issueScanImplementationRunnerContext(runID)
	if err != nil {
		return IssueScanImplementationRunnerRecordResult{RunID: runID}, err
	}
	if !ready {
		return IssueScanImplementationRunnerRecordResult{RunID: runID}, fmt.Errorf("issue-scan run %q has no implementation task ready to record", runID)
	}
	return r.recordIssueScanImplementationRunnerResult(runnerContext, runnerResult)
}

func (r *Runtime) issueScanImplementationRunnerContext(runID string) (IssueScanImplementationRunnerContext, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	if len(requests) == 0 {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return IssueScanImplementationRunnerContext{}, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	implementation, ready, err := r.EnsureIssueScanImplementationTask(runID)
	if err != nil || !ready {
		return IssueScanImplementationRunnerContext{}, ready, err
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	designTarget, err := r.issueScanStageTargetByStageID(drafts, issueScanSelectAndDesignStageID, orderID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(implementationStage.TaskID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	if stageCompleted {
		return IssueScanImplementationRunnerContext{}, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(implementationStage); err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	status, err := r.tasks.GetCompatibilityStatus(implementation.ImplementationTaskID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("read implementation task status: %w", err)
	}
	if status == work.LegacyStatusCompleted {
		return IssueScanImplementationRunnerContext{}, false, nil
	}
	blocked, err := r.tasks.IsBlocked(implementation.ImplementationTaskID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	if blocked || status == work.LegacyStatusBlocked {
		return IssueScanImplementationRunnerContext{}, false, nil
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	repo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if repo == "" && len(content.TargetRepos) > 0 {
		repo = strings.ToLower(strings.TrimSpace(content.TargetRepos[0]))
	}
	if !ValidTransparaAIRepo(repo) {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("selected issue repository %q is not a Transpara-AI repo", repo)
	}
	resolved, err := r.resolveIssueScanWorkspaceForRepo(repo)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("resolve implementation workspace for %s: %w", repo, err)
	}
	designEvidence, _, ok, err := r.issueScanStageRuntimeEvidenceForCompletedStage(content, orderID, designTarget)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	if !ok {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("completed design stage has no %s artifact", IssueScanStageRuntimeEvidenceArtifactLabel)
	}
	implementationArtifacts, err := r.tasks.ListArtifacts(implementation.ImplementationTaskID)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("list implementation task artifacts: %w", err)
	}
	taskContext, ok, err := issueScanImplementationTaskContextArtifactRaw(implementationArtifacts)
	if err != nil {
		return IssueScanImplementationRunnerContext{}, false, err
	}
	if !ok {
		return IssueScanImplementationRunnerContext{}, false, fmt.Errorf("implementation task %s has no %s artifact", implementation.ImplementationTaskID.Value(), IssueScanImplementationTaskContextArtifactLabel)
	}
	return IssueScanImplementationRunnerContext{
		Kind:                         issueScanImplementationRunnerContextKind,
		LifecycleVersion:             issueScanLifecycleVersion,
		RunID:                        strings.TrimSpace(content.RunID),
		FactoryOrderID:               orderID,
		Repository:                   repo,
		RepoPath:                     resolved.RepoPath,
		ContainmentWatchRoots:        append([]string(nil), resolved.ContainmentWatchRoots...),
		ImplementationTaskID:         implementation.ImplementationTaskID.Value(),
		ImplementationStageTaskID:    implementationStage.TaskID.Value(),
		DesignStageTaskID:            implementation.DesignStageTaskID.Value(),
		DesignRuntimeEvidenceRef:     implementation.DesignEvidenceArtifactID.Value(),
		SelectedIssue:                issueScanStageRoleOutputIssueFromBriefIssue(brief.SelectedIssue),
		TargetRepos:                  append([]string(nil), content.TargetRepos...),
		DesignOutputs:                issueScanEvidenceItemsForImplementationRunner(designEvidence),
		ImplementationTaskContext:    taskContext,
		ImplementationReadinessGates: issueScanImplementationReadinessGateArtifacts(implementationArtifacts),
		BoundaryDisclaimers: compactStrings([]string{
			"runner may implement only on a branch in the configured repository",
			"runner output must be an Operate result for the concrete implementation task",
			"runner output is not stage completion by itself",
			"runner output is not adversarial review evidence",
			"runner output is not PR creation or PR readiness",
			"runner output is not Human approval",
			"runner output is not merge or deploy authorization",
		}),
	}, true, nil
}

func (r *Runtime) recordIssueScanImplementationRunnerResult(runnerContext IssueScanImplementationRunnerContext, runnerResult IssueScanImplementationRunnerResult) (IssueScanImplementationRunnerRecordResult, error) {
	result := IssueScanImplementationRunnerRecordResult{
		RunID:          runnerContext.RunID,
		FactoryOrderID: runnerContext.FactoryOrderID,
		Repository:     runnerContext.Repository,
	}
	taskID, err := types.NewEventID(runnerContext.ImplementationTaskID)
	if err != nil {
		return result, fmt.Errorf("parse implementation task id: %w", err)
	}
	stageTaskID, err := types.NewEventID(runnerContext.ImplementationStageTaskID)
	if err != nil {
		return result, fmt.Errorf("parse implementation stage task id: %w", err)
	}
	result.ImplementationTaskID = taskID
	result.ImplementationStageTaskID = stageTaskID
	body := strings.TrimSpace(runnerResult.OperateResultBody)
	if body == "" {
		return result, fmt.Errorf("implementation runner operate_result_body is required")
	}
	parsed, err := parseIssueScanOperateResultArtifact(body)
	if err != nil {
		return result, fmt.Errorf("implementation runner operate_result_body is invalid: %w", err)
	}
	summary := strings.TrimSpace(runnerResult.CompletionSummary)
	if summary == "" {
		return result, fmt.Errorf("implementation runner completion_summary is required")
	}
	causes := issueScanImplementationRunnerRecordCauses(runnerContext, taskID, stageTaskID)
	if err := r.tasks.AddArtifact(r.humanID, taskID, "Operate result", "text/plain", body, causes, r.convID); err != nil {
		return result, fmt.Errorf("record implementation Operate result artifact: %w", err)
	}
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return result, fmt.Errorf("list implementation task artifacts after Operate result: %w", err)
	}
	operate, ok := latestIssueScanOperateResultArtifact(artifacts)
	if !ok {
		return result, fmt.Errorf("implementation task %s has no Operate result artifact after runner recording", taskID.Value())
	}
	completeCauses := compactEventIDs(append(causes, operate.ID))
	if err := r.tasks.Complete(r.humanID, taskID, summary, completeCauses, r.convID); err != nil {
		return result, fmt.Errorf("complete implementation task from runner result: %w", err)
	}
	completion, ok, err := r.issueScanImplementationCompletionEvidence(taskID, stageTaskID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, fmt.Errorf("implementation task %s has no live completion after runner recording", taskID.Value())
	}
	result.OperateArtifactID = operate.ID
	result.CompletionEventID = completion.CompletionEventID
	result.OperateBranch = parsed.Branch
	result.OperateCommit = parsed.Commit
	result.Recorded = true
	return result, nil
}

func issueScanImplementationTaskContextArtifactRaw(artifacts []work.ArtifactEvent) (json.RawMessage, bool, error) {
	artifact, ok := latestIssueScanImplementationTaskContextArtifact(artifacts)
	if !ok {
		return nil, false, nil
	}
	raw := strings.TrimSpace(artifact.Body)
	if _, err := parseIssueScanImplementationTaskContext(raw); err != nil {
		return nil, false, err
	}
	return json.RawMessage(raw), true, nil
}

func issueScanImplementationReadinessGateArtifacts(artifacts []work.ArtifactEvent) []IssueScanImplementationRunnerArtifact {
	out := []IssueScanImplementationRunnerArtifact{}
	for _, artifact := range artifacts {
		switch strings.TrimSpace(artifact.Label) {
		case work.GateDefinitionOfDone, work.GateAcceptanceCriteria, work.GateTestPlan:
			out = append(out, IssueScanImplementationRunnerArtifact{
				Label:     artifact.Label,
				MediaType: artifact.MediaType,
				Body:      artifact.Body,
			})
		}
	}
	return out
}

func issueScanEvidenceItemsForImplementationRunner(evidence IssueScanStageRuntimeEvidence) []IssueScanStageRuntimeEvidenceItem {
	return issueScanEvidenceItemsForKeys(evidence, []string{
		"selected_approach",
		"definition_of_done",
		"implementation_task_plan",
		"acceptance_criteria",
		"test_plan",
		"authority_gate_requirements",
	})
}

func issueScanImplementationRunnerRecordCauses(runnerContext IssueScanImplementationRunnerContext, taskID, stageTaskID types.EventID) []types.EventID {
	ids := []types.EventID{taskID, stageTaskID}
	for _, raw := range []string{runnerContext.DesignStageTaskID, runnerContext.DesignRuntimeEvidenceRef} {
		if id, err := types.NewEventID(strings.TrimSpace(raw)); err == nil {
			ids = append(ids, id)
		}
	}
	return compactEventIDs(ids)
}
