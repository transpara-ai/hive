package hive

import (
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// RecordCompletedIssueScanDebateRoleOutputs records
// debate_with_correct_civic_roles outputs after the read-only research stage has
// completed. It derives a bounded civic debate packet from recorded research
// evidence only; no repository mutation, PR readiness, Human approval, merge, or
// deploy authority is implied.
func (r *Runtime) RecordCompletedIssueScanDebateRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	return r.recordIssueScanPlanningRoleOutputs(result, issueScanPlanningBridge{
		StageID:      "debate_with_correct_civic_roles",
		PreviousID:   "research_issue_and_repo_context",
		ErrContext:   "debate",
		OutputBuffer: 4,
		Build:        issueScanDebateRoleOutputs,
	})
}

// RecordCompletedIssueScanDesignRoleOutputs records select_and_design_approach
// outputs after the civic debate stage has completed. It produces the concrete
// design evidence required to seed an implementation task, but does not itself
// implement, review, ready, merge, deploy, or approve.
func (r *Runtime) RecordCompletedIssueScanDesignRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	return r.recordIssueScanPlanningRoleOutputs(result, issueScanPlanningBridge{
		StageID:      issueScanSelectAndDesignStageID,
		PreviousID:   "debate_with_correct_civic_roles",
		ErrContext:   "design",
		OutputBuffer: 3,
		Build:        issueScanDesignRoleOutputs,
	})
}

type issueScanPlanningBridge struct {
	StageID      string
	PreviousID   string
	ErrContext   string
	OutputBuffer int
	Build        func(FactoryRunRequestedContent, string, issueScanPlanningStageEvidence) []IssueScanStageRoleOutputEvidence
}

type issueScanPlanningStageEvidence struct {
	StageTaskID       types.EventID
	PreviousStageID   string
	PreviousTaskID    types.EventID
	PreviousRuntimeID types.EventID
	PreviousEvidence  IssueScanStageRuntimeEvidence
	Brief             issueScanResearchBrief
}

func (r *Runtime) recordIssueScanPlanningRoleOutputs(result RunLaunchDispatchResult, bridge issueScanPlanningBridge) ([]IssueScanStageRoleOutputResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputResult, 0, len(runIDs)*bridge.OutputBuffer)
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.recordIssueScanPlanningRoleOutput(runID, bridge)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ready {
			out = append(out, recorded...)
		}
	}
	return out, errors.Join(errs...)
}

func (r *Runtime) recordIssueScanPlanningRoleOutput(runID string, bridge issueScanPlanningBridge) ([]IssueScanStageRoleOutputResult, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return nil, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return nil, false, err
	}
	if len(requests) == 0 {
		return nil, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return nil, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return nil, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return nil, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return nil, false, fmt.Errorf("dispatch queued issue-scan run %q before %s role-output recording: %w", runID, bridge.ErrContext, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, false, err
	}
	stage, err := r.issueScanStageTargetByStageID(drafts, bridge.StageID, orderID)
	if err != nil {
		return nil, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(stage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if stageCompleted {
		return nil, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(stage); err != nil {
		return nil, false, err
	}
	previous, err := r.issueScanStageTargetByStageID(drafts, bridge.PreviousID, orderID)
	if err != nil {
		return nil, false, err
	}
	previousCompleted, err := r.issueScanStageTaskCompleted(previous.TaskID)
	if err != nil {
		return nil, false, err
	}
	if !previousCompleted {
		return nil, false, nil
	}
	previousEvidence, previousArtifactID, ok, err := r.issueScanStageRuntimeEvidenceForCompletedStage(content, orderID, previous)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, fmt.Errorf("completed predecessor stage %q has no %s artifact", previous.Draft.StageID, IssueScanStageRuntimeEvidenceArtifactLabel)
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return nil, false, err
	}
	evidence := issueScanPlanningStageEvidence{
		StageTaskID:       stage.TaskID,
		PreviousStageID:   previous.Draft.StageID,
		PreviousTaskID:    previous.TaskID,
		PreviousRuntimeID: previousArtifactID,
		PreviousEvidence:  previousEvidence,
		Brief:             brief,
	}
	outputs := bridge.Build(content, orderID, evidence)
	results := make([]IssueScanStageRoleOutputResult, 0, len(outputs))
	for _, output := range outputs {
		recorded, err := r.RecordIssueScanStageRoleOutput(runID, bridge.StageID, output)
		if err != nil {
			return results, false, err
		}
		results = append(results, recorded)
	}
	return results, true, nil
}

func issueScanDebateRoleOutputs(content FactoryRunRequestedContent, orderID string, evidence issueScanPlanningStageEvidence) []IssueScanStageRoleOutputEvidence {
	issue := evidence.Brief.SelectedIssue
	refs := issueScanPlanningEvidenceRefs(content, orderID, evidence, issue)
	priority := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "issue_priority_rationale"), fmt.Sprintf("Selected %s#%d for governed Civilization execution.", issue.Repo, issue.Number))
	repoContext := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "repo_context_packet"), fmt.Sprintf("Target repo %s is the selected implementation scope.", issue.Repo))
	validation := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "candidate_validation_commands"), "Identify repo-local validation before implementation and record exact output.")
	risk := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "risk_and_scope_notes"), "No merge, deploy, PR readiness, or Human approval claim is authorized by planning evidence.")
	strategistPosition := fmt.Sprintf("Proceed with %s#%d because the scan selected it as the highest-ranked actionable Civilization issue. %s", issue.Repo, issue.Number, priority)
	plannerPosition := fmt.Sprintf("Use the selected repo context to keep implementation narrow and testable. %s", repoContext)
	reviewerPosition := fmt.Sprintf("Require exact validation output and exact-head adversarial review before ready state. %s", validation)
	guardianPosition := fmt.Sprintf("Keep the authority boundary at proposal-only planning until implementation and review gates record their own evidence. %s", risk)
	decision := fmt.Sprintf("Decision: advance %s#%d to select_and_design_approach with strategist, planner, reviewer, and guardian positions recorded.", issue.Repo, issue.Number)
	return []IssueScanStageRoleOutputEvidence{
		{
			Role:         "strategist",
			Summary:      strategistPosition,
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "strategist_position", Summary: strategistPosition, EvidenceRefs: refs},
				{Key: "role_positions", Summary: strategistPosition, EvidenceRefs: refs},
			},
			AuthorityBoundary: "proposal_only_no_mutation",
			CompletionGate:    "decision_record_has_reviewer_and_guardian_disposition",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
		{
			Role:         "planner",
			Summary:      plannerPosition,
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "planner_position", Summary: plannerPosition, EvidenceRefs: refs},
			},
			AuthorityBoundary: "proposal_only_no_mutation",
			CompletionGate:    "decision_record_has_reviewer_and_guardian_disposition",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
		{
			Role:         "reviewer",
			Summary:      reviewerPosition,
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "reviewer_position", Summary: reviewerPosition, EvidenceRefs: refs},
				{Key: "test_risk_notes", Summary: reviewerPosition, EvidenceRefs: refs},
				{Key: "decision_record", Summary: decision, EvidenceRefs: refs},
			},
			AuthorityBoundary: "proposal_only_no_mutation",
			CompletionGate:    "decision_record_has_reviewer_and_guardian_disposition",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
		{
			Role:         "guardian",
			Summary:      guardianPosition,
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "guardian_position", Summary: guardianPosition, EvidenceRefs: refs},
				{Key: "authority_boundary_disposition", Summary: guardianPosition, EvidenceRefs: refs},
				{Key: "dissent_or_no_dissent_record", Summary: "No dissent blocks design selection; guardian boundary remains no mutation, no merge, no deploy, and no Human approval claim.", EvidenceRefs: refs},
			},
			AuthorityBoundary: "proposal_only_no_mutation",
			CompletionGate:    "decision_record_has_reviewer_and_guardian_disposition",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
	}
}

func issueScanDesignRoleOutputs(content FactoryRunRequestedContent, orderID string, evidence issueScanPlanningStageEvidence) []IssueScanStageRoleOutputEvidence {
	issue := evidence.Brief.SelectedIssue
	refs := issueScanPlanningEvidenceRefs(content, orderID, evidence, issue)
	rolePositions := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "role_positions"), "Civic roles agreed to advance the selected issue into design.")
	decision := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "decision_record"), fmt.Sprintf("Advance %s#%d to implementation design.", issue.Repo, issue.Number))
	boundary := valueOr(issueScanEvidenceSummary(evidence.PreviousEvidence, "authority_boundary_disposition"), "Implementation may occur only on a branch and must not merge, deploy, or claim Human approval.")
	selected := fmt.Sprintf("Implement the smallest repo-local change for %s#%d that satisfies the selected issue while preserving the recorded authority boundary.", issue.Repo, issue.Number)
	definition := fmt.Sprintf("Done means the implementation task records branch, commit SHA, changed files, validation output, exact-head adversarial review evidence, and no unresolved accepted blockers for %s#%d.", issue.Repo, issue.Number)
	plan := fmt.Sprintf("Create an implementation branch for %s, apply the selected approach from the debate decision, run repo-local validation, then keep PR readiness blocked until review and blocker-zero stages pass.", issue.Repo)
	criteria := fmt.Sprintf("Acceptance requires the selected issue scope to be addressed, code changes confined to %s unless evidence expands scope, validation output recorded, and no merge/deploy/Human approval action taken.", issue.Repo)
	testPlan := fmt.Sprintf("Run the target repo's relevant tests after implementation, record exact output, and run exact-head adversarial review before ready state. Debate basis: %s", decision)
	authority := fmt.Sprintf("Authority gate: %s Implementation task creation is allowed; merge, deploy, ready-state claims, and Human approval remain later gated stages.", boundary)
	return []IssueScanStageRoleOutputEvidence{
		{
			Role:         "planner",
			Summary:      fmt.Sprintf("%s %s", selected, rolePositions),
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "selected_approach", Summary: selected, EvidenceRefs: refs},
				{Key: "definition_of_done", Summary: definition, EvidenceRefs: refs},
				{Key: "implementation_task_plan", Summary: plan, EvidenceRefs: refs},
			},
			AuthorityBoundary: "implementation_waits_for_authorized_task",
			CompletionGate:    "implementation_task_has_readiness_artifacts",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
		{
			Role:         "reviewer",
			Summary:      criteria,
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "acceptance_criteria", Summary: criteria, EvidenceRefs: refs},
				{Key: "test_plan", Summary: testPlan, EvidenceRefs: refs},
			},
			AuthorityBoundary: "implementation_waits_for_authorized_task",
			CompletionGate:    "implementation_task_has_readiness_artifacts",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
		{
			Role:         "guardian",
			Summary:      authority,
			EvidenceRefs: refs,
			Outputs: []IssueScanStageRuntimeEvidenceItem{
				{Key: "authority_gate_requirements", Summary: authority, EvidenceRefs: refs},
			},
			AuthorityBoundary: "implementation_waits_for_authorized_task",
			CompletionGate:    "implementation_task_has_readiness_artifacts",
			SourceRefs:        issueScanPlanningSourceRefs(content, orderID, evidence, issue),
		},
	}
}

func issueScanPlanningEvidenceRefs(content FactoryRunRequestedContent, orderID string, evidence issueScanPlanningStageEvidence, issue issueScanBriefIssuePayload) []string {
	return compactStrings([]string{
		content.SourceEventID.Value(),
		content.BriefEventID.Value(),
		orderID,
		evidence.StageTaskID.Value(),
		evidence.PreviousTaskID.Value(),
		evidence.PreviousRuntimeID.Value(),
		issue.URL,
		fmt.Sprintf("%s#%d", issue.Repo, issue.Number),
	})
}

func issueScanPlanningSourceRefs(content FactoryRunRequestedContent, orderID string, evidence issueScanPlanningStageEvidence, issue issueScanBriefIssuePayload) []string {
	refs := issueScanPlanningEvidenceRefs(content, orderID, evidence, issue)
	refs = append(refs, evidence.PreviousEvidence.SourceRefs...)
	for _, source := range content.Sources {
		refs = append(refs, source.Ref)
	}
	return compactStrings(refs)
}
