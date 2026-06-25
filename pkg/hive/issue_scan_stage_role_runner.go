package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const issueScanStageRoleOutputRunnerContextKind = "issue_scan_stage_role_output_runner_context"

// IssueScanStageRoleOutputRunner receives a bounded planning-stage context and
// returns role-output artifacts for the runtime to validate and record.
type IssueScanStageRoleOutputRunner func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error)

type IssueScanStageRoleOutputIssue struct {
	Repo        string   `json:"repo"`
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Labels      []string `json:"labels,omitempty"`
	BodyExcerpt string   `json:"body_excerpt,omitempty"`
}

type IssueScanStageRoleOutputSelectionPolicy struct {
	PolicyID       string   `json:"policy_id"`
	SelectedRank   int      `json:"selected_rank"`
	CandidateCount int      `json:"candidate_count"`
	RankingInputs  []string `json:"ranking_inputs,omitempty"`
	Rationale      string   `json:"rationale,omitempty"`
}

type IssueScanStageRoleOutputRunnerContext struct {
	Kind                     string                                  `json:"kind"`
	LifecycleVersion         string                                  `json:"lifecycle_version"`
	RunID                    string                                  `json:"run_id"`
	FactoryOrderID           string                                  `json:"factory_order_id"`
	Repository               string                                  `json:"repository"`
	RepoPath                 string                                  `json:"repo_path,omitempty"`
	ContainmentWatchRoots    []string                                `json:"containment_watch_roots,omitempty"`
	WorkspaceResolutionError string                                  `json:"workspace_resolution_error,omitempty"`
	StageID                  string                                  `json:"stage_id"`
	StageTaskID              string                                  `json:"stage_task_id"`
	StageIndex               int                                     `json:"stage_index"`
	StageCount               int                                     `json:"stage_count"`
	Stage                    OperatorQueuedRunLifecycleStage         `json:"stage"`
	SelectedIssue            IssueScanStageRoleOutputIssue           `json:"selected_issue"`
	SelectionPolicy          IssueScanStageRoleOutputSelectionPolicy `json:"selection_policy"`
	ScannedRepos             []string                                `json:"scanned_repos,omitempty"`
	ScannedIssueCount        int                                     `json:"scanned_issue_count,omitempty"`
	CandidateIssues          []IssueScanStageRoleOutputIssue         `json:"candidate_issues,omitempty"`
	AgentExecutionPlan       []OperatorQueuedRunAgentPlanStep        `json:"agent_execution_plan"`
	RequestedRoleSteps       []OperatorQueuedRunAgentPlanStep        `json:"requested_role_steps"`
	MissingRoles             []string                                `json:"missing_roles,omitempty"`
	MissingRoleOutputKeys    map[string][]string                     `json:"missing_role_output_keys,omitempty"`
	MissingEvidenceKeys      []string                                `json:"missing_evidence_keys,omitempty"`
	ExistingRoleOutputs      []IssueScanStageRuntimeRoleOutput       `json:"existing_role_outputs,omitempty"`
	ExistingEvidenceItems    []IssueScanStageRuntimeEvidenceItem     `json:"existing_evidence_items,omitempty"`
	SourceRefs               []string                                `json:"source_refs,omitempty"`
	BoundaryDisclaimers      []string                                `json:"boundary_disclaimers,omitempty"`
}

type IssueScanStageRoleOutputRunnerResult struct {
	RoleOutputs []IssueScanStageRoleOutputEvidence `json:"role_outputs"`
}

type IssueScanStageRoleOutputRunnerRecordResult struct {
	RunID          string
	FactoryOrderID string
	StageID        string
	StageTaskID    types.EventID
	RoleOutputs    []IssueScanStageRoleOutputResult
	Recorded       bool
}

// IssueScanStageRoleOutputRunnerContext returns the current planning-stage
// role-output context for a queued issue-scan run. It intentionally stops before
// implementation/review/ready stages; those are governed by separate evidence
// bridges.
func (r *Runtime) IssueScanStageRoleOutputRunnerContext(runID string) (IssueScanStageRoleOutputRunnerContext, error) {
	runnerContext, ready, err := r.issueScanStageRoleOutputRunnerContext(runID)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, err
	}
	if !ready {
		return IssueScanStageRoleOutputRunnerContext{}, fmt.Errorf("issue-scan run %q has no planning-stage role outputs ready to collect", strings.TrimSpace(runID))
	}
	return runnerContext, nil
}

func (r *Runtime) RunConfiguredIssueScanStageRoleOutputRunners(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputRunnerRecordResult, error) {
	if r == nil || r.issueScanStageRoleOutputRunner == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputRunnerRecordResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RunConfiguredIssueScanStageRoleOutputRunner(ctx, runID)
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

func (r *Runtime) RunConfiguredIssueScanStageRoleOutputRunner(ctx context.Context, runID string) (IssueScanStageRoleOutputRunnerRecordResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanStageRoleOutputRunnerRecordResult{RunID: runID}
	if r == nil || r.issueScanStageRoleOutputRunner == nil {
		return result, false, nil
	}
	runnerContext, ready, err := r.issueScanStageRoleOutputRunnerContext(runID)
	if err != nil || !ready {
		return result, ready, err
	}
	runnerResult, err := r.issueScanStageRoleOutputRunner(ctx, runnerContext)
	if err != nil {
		return result, true, err
	}
	if len(runnerResult.RoleOutputs) == 0 {
		return result, true, fmt.Errorf("issue-scan stage role-output runner returned no role outputs for stage %q", runnerContext.StageID)
	}
	result.FactoryOrderID = runnerContext.FactoryOrderID
	result.StageID = runnerContext.StageID
	stageTaskID, err := types.NewEventID(runnerContext.StageTaskID)
	if err != nil {
		return result, true, fmt.Errorf("parse stage task id from runner context: %w", err)
	}
	result.StageTaskID = stageTaskID
	for _, output := range runnerResult.RoleOutputs {
		recorded, err := r.RecordIssueScanStageRoleOutput(runID, runnerContext.StageID, output)
		if err != nil {
			return result, true, err
		}
		if recorded.Recorded {
			result.Recorded = true
		}
		result.RoleOutputs = append(result.RoleOutputs, recorded)
	}
	return result, true, nil
}

func (r *Runtime) issueScanStageRoleOutputRunnerContext(runID string) (IssueScanStageRoleOutputRunnerContext, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	} else if parked {
		return IssueScanStageRoleOutputRunnerContext{}, false, nil
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	if len(requests) == 0 {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("queued run %q not found", runID)
	}
	request := requests[0]
	content, ok := request.Content().(FactoryRunRequestedContent)
	if !ok {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return IssueScanStageRoleOutputRunnerContext{}, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	dispatch, err := r.DispatchQueuedRunLaunch(runID)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("dispatch queued issue-scan run %q before role-output runner context: %w", runID, err)
	}
	if _, err := r.StartDispatchedIssueScanLifecycleStages(dispatch); err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("start issue-scan lifecycle stage before role-output runner context: %w", err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	target, _, err := r.selectIssueScanStageAdvanceTarget(drafts, "", orderID)
	if err != nil {
		if strings.Contains(err.Error(), "issue-scan lifecycle has no incomplete stages") {
			return IssueScanStageRoleOutputRunnerContext{}, false, nil
		}
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	if !issueScanStageRoleOutputRunnerEligibleStage(target.Draft.StageID) {
		return IssueScanStageRoleOutputRunnerContext{}, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(target); err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	blocked, err := r.tasks.IsBlocked(target.TaskID)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	if blocked {
		return IssueScanStageRoleOutputRunnerContext{}, false, nil
	}
	recorded, sourceRefs, err := r.issueScanStageRecordedRuntimeRoleOutputs(target.TaskID, content, orderID, target.Draft)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	existingEvidence := issueScanStageEvidenceItemsFromRoleOutputs(target.Draft, recorded)
	missingRoles, missingRoleOutputKeys, requestedSteps := issueScanRequestedRoleOutputRunnerSteps(target.Draft, recorded)
	missingEvidenceKeys := issueScanMissingStageRuntimeEvidenceKeys(target.Draft.Stage.RequiredEvidence, existingEvidence)
	if len(missingEvidenceKeys) > 0 && len(requestedSteps) == 0 {
		requestedSteps = append([]OperatorQueuedRunAgentPlanStep(nil), target.Draft.AgentExecutionPlan...)
	}
	if len(requestedSteps) == 0 && len(missingEvidenceKeys) == 0 {
		return IssueScanStageRoleOutputRunnerContext{}, false, nil
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanStageRoleOutputRunnerContext{}, false, err
	}
	repo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if repo == "" && len(content.TargetRepos) > 0 {
		repo = strings.ToLower(strings.TrimSpace(content.TargetRepos[0]))
	}
	if !ValidTransparaAIRepo(repo) {
		return IssueScanStageRoleOutputRunnerContext{}, false, fmt.Errorf("selected issue repository %q is not a Transpara-AI repo", repo)
	}
	runnerContext := IssueScanStageRoleOutputRunnerContext{
		Kind:                  issueScanStageRoleOutputRunnerContextKind,
		LifecycleVersion:      issueScanLifecycleVersion,
		RunID:                 strings.TrimSpace(content.RunID),
		FactoryOrderID:        orderID,
		Repository:            repo,
		StageID:               target.Draft.StageID,
		StageTaskID:           target.TaskID.Value(),
		StageIndex:            target.Draft.StageIndex,
		StageCount:            target.Draft.StageCount,
		Stage:                 target.Draft.Stage,
		SelectedIssue:         issueScanStageRoleOutputIssueFromBriefIssue(brief.SelectedIssue),
		SelectionPolicy:       issueScanStageRoleOutputSelectionPolicyFromBrief(brief.SelectionPolicy),
		ScannedRepos:          compactStrings(brief.ScannedRepos),
		ScannedIssueCount:     brief.ScannedIssueCount,
		CandidateIssues:       issueScanStageRoleOutputIssuesFromBrief(brief.CandidateIssues),
		AgentExecutionPlan:    append([]OperatorQueuedRunAgentPlanStep(nil), target.Draft.AgentExecutionPlan...),
		RequestedRoleSteps:    requestedSteps,
		MissingRoles:          missingRoles,
		MissingRoleOutputKeys: missingRoleOutputKeys,
		MissingEvidenceKeys:   missingEvidenceKeys,
		ExistingRoleOutputs:   append([]IssueScanStageRuntimeRoleOutput(nil), recorded...),
		ExistingEvidenceItems: existingEvidence,
		SourceRefs:            compactStrings(append([]string{request.ID().Value(), target.TaskID.Value()}, sourceRefs...)),
		BoundaryDisclaimers: compactStrings([]string{
			"runner output is role-output evidence only",
			"runner output is not stage completion",
			"runner output is not implementation authority",
			"runner output is not PR readiness",
			"runner output is not Human approval",
			"runner output is not merge or deploy authorization",
		}),
	}
	if resolved, err := r.resolveIssueScanWorkspaceForRepo(repo); err == nil {
		runnerContext.RepoPath = resolved.RepoPath
		runnerContext.ContainmentWatchRoots = append([]string(nil), resolved.ContainmentWatchRoots...)
	} else {
		runnerContext.WorkspaceResolutionError = err.Error()
		runnerContext.BoundaryDisclaimers = compactStrings(append(runnerContext.BoundaryDisclaimers,
			"repo workspace resolution failed; repo_path is unavailable and the runner must not infer authority from process cwd",
		))
	}
	return runnerContext, true, nil
}

func issueScanStageRoleOutputRunnerEligibleStage(stageID string) bool {
	switch safeRunLaunchID(stageID) {
	case "research_issue_and_repo_context", "debate_with_correct_civic_roles", "select_and_design_approach":
		return true
	default:
		return false
	}
}

func issueScanRequestedRoleOutputRunnerSteps(draft issueScanLifecycleStageTaskDraft, recorded []IssueScanStageRuntimeRoleOutput) ([]string, map[string][]string, []OperatorQueuedRunAgentPlanStep) {
	byRole := map[string]IssueScanStageRuntimeRoleOutput{}
	for _, output := range recorded {
		byRole[strings.TrimSpace(output.Role)] = output
	}
	missingRoles := []string{}
	missingOutputKeys := map[string][]string{}
	requested := []OperatorQueuedRunAgentPlanStep{}
	for _, step := range draft.AgentExecutionPlan {
		role := strings.TrimSpace(step.Role)
		if role == "" {
			continue
		}
		output, ok := byRole[role]
		if !ok {
			missingRoles = append(missingRoles, role)
			missingOutputKeys[role] = compactStrings(step.RequiredOutputs)
			requested = append(requested, step)
			continue
		}
		missing := issueScanMissingRoleOutputKeys(step.RequiredOutputs, output)
		if len(missing) > 0 {
			missingOutputKeys[role] = missing
			requested = append(requested, step)
		}
	}
	if len(missingOutputKeys) == 0 {
		missingOutputKeys = nil
	}
	return compactStrings(missingRoles), missingOutputKeys, requested
}

func issueScanMissingRoleOutputKeys(required []string, output IssueScanStageRuntimeRoleOutput) []string {
	byKey := map[string]IssueScanStageRuntimeEvidenceItem{}
	for _, item := range output.Outputs {
		byKey[strings.TrimSpace(item.Key)] = item
	}
	missing := []string{}
	for _, key := range compactStrings(required) {
		item, ok := byKey[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if len(item.EvidenceRefs) == 0 && len(output.EvidenceRefs) == 0 {
			missing = append(missing, key)
		}
	}
	return missing
}

func issueScanMissingStageRuntimeEvidenceKeys(required []string, existing []IssueScanStageRuntimeEvidenceItem) []string {
	byKey := map[string]IssueScanStageRuntimeEvidenceItem{}
	for _, item := range existing {
		byKey[strings.TrimSpace(item.Key)] = item
	}
	missing := []string{}
	for _, key := range compactStrings(required) {
		item, ok := byKey[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if len(item.EvidenceRefs) == 0 {
			missing = append(missing, key)
		}
	}
	return missing
}

func issueScanStageRoleOutputIssueFromBriefIssue(issue issueScanBriefIssuePayload) IssueScanStageRoleOutputIssue {
	return IssueScanStageRoleOutputIssue{
		Repo:        strings.TrimSpace(issue.Repo),
		Number:      issue.Number,
		Title:       strings.TrimSpace(issue.Title),
		URL:         strings.TrimSpace(issue.URL),
		Labels:      compactStrings(issue.Labels),
		BodyExcerpt: strings.TrimSpace(issue.Body),
	}
}

func issueScanStageRoleOutputIssuesFromBrief(issues []issueScanBriefIssuePayload) []IssueScanStageRoleOutputIssue {
	out := make([]IssueScanStageRoleOutputIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issueScanStageRoleOutputIssueFromBriefIssue(issue))
	}
	return out
}

func issueScanStageRoleOutputSelectionPolicyFromBrief(policy issueScanSelectionPolicyPayload) IssueScanStageRoleOutputSelectionPolicy {
	return IssueScanStageRoleOutputSelectionPolicy{
		PolicyID:       strings.TrimSpace(policy.PolicyID),
		SelectedRank:   policy.SelectedRank,
		CandidateCount: policy.CandidateCount,
		RankingInputs:  compactStrings(policy.RankingInputs),
		Rationale:      strings.TrimSpace(policy.Rationale),
	}
}
