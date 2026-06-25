package hive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const (
	IssueScanStageRuntimeEvidenceArtifactLabel = "issue_scan_stage_runtime_evidence"
	issueScanStageRuntimeEvidenceArtifactKind  = "issue_scan_stage_runtime_evidence"
	issueScanStageRuntimeEvidenceMediaType     = "application/json"
)

// IssueScanStageRuntimeEvidence is the governed completion packet for one
// issue-scan lifecycle stage. It records observed stage evidence only; it is not
// PR readiness, Human approval, merge, deploy, or proof of later stages.
type IssueScanStageRuntimeEvidence struct {
	Kind                string                              `json:"kind,omitempty"`
	LifecycleVersion    string                              `json:"lifecycle_version,omitempty"`
	RunID               string                              `json:"run_id,omitempty"`
	FactoryOrderID      string                              `json:"factory_order_id,omitempty"`
	StageID             string                              `json:"stage_id,omitempty"`
	Status              string                              `json:"status,omitempty"`
	Summary             string                              `json:"summary"`
	EvidenceItems       []IssueScanStageRuntimeEvidenceItem `json:"evidence_items"`
	RoleOutputs         []IssueScanStageRuntimeRoleOutput   `json:"role_outputs"`
	ResidualRisks       []string                            `json:"residual_risks,omitempty"`
	BoundaryDisclaimers []string                            `json:"boundary_disclaimers,omitempty"`
	SourceRefs          []string                            `json:"source_refs,omitempty"`
}

type IssueScanStageRuntimeEvidenceItem struct {
	Key          string   `json:"key"`
	Summary      string   `json:"summary,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type IssueScanStageRuntimeRoleOutput struct {
	Role         string                              `json:"role"`
	Summary      string                              `json:"summary,omitempty"`
	EvidenceRefs []string                            `json:"evidence_refs,omitempty"`
	Outputs      []IssueScanStageRuntimeEvidenceItem `json:"outputs"`
}

// IssueScanStageCompletionResult summarizes one governed stage completion.
type IssueScanStageCompletionResult struct {
	RunID              string
	FactoryOrderID     string
	ParentTaskID       types.EventID
	StageID            string
	StageTaskID        types.EventID
	EvidenceArtifactID types.EventID
	Completed          bool
	Advance            IssueScanStageAdvanceResult
	NextAdvance        *IssueScanStageAdvanceResult
	LifecycleComplete  bool
}

// CompleteIssueScanLifecycleStage records runtime evidence for one issue-scan
// lifecycle stage, validates that evidence against the stage's declared
// contracts, completes the stage Work task, and optionally releases the next
// stage scheduling barrier. It does not complete the parent FactoryOrder and
// does not claim PR readiness, Human approval, merge, or deploy evidence.
func (r *Runtime) CompleteIssueScanLifecycleStage(runID, stageID string, evidence IssueScanStageRuntimeEvidence, advanceNext bool) (IssueScanStageCompletionResult, error) {
	runID = strings.TrimSpace(runID)
	stageID = strings.TrimSpace(stageID)
	result := IssueScanStageCompletionResult{RunID: runID}
	if r == nil || r.store == nil || r.tasks == nil {
		return result, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return result, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return result, err
	} else if parked {
		return result, fmt.Errorf("issue-scan run %q is parked", runID)
	}

	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return result, err
	}
	if len(requests) == 0 {
		return result, fmt.Errorf("queued run %q not found", runID)
	}
	request := requests[0]
	content, ok := request.Content().(FactoryRunRequestedContent)
	if !ok {
		return result, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return result, fmt.Errorf("queued run %q is not an issue-scan run", runID)
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return result, err
	}
	result.FactoryOrderID = orderID
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return result, err
	}

	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return result, fmt.Errorf("dispatch queued issue-scan run %q before stage completion: %w", runID, err)
	}
	target, _, err := r.selectIssueScanStageAdvanceTarget(drafts, stageID, orderID)
	if err != nil {
		return result, err
	}
	result.StageID = target.Draft.StageID
	result.StageTaskID = target.TaskID
	if err := r.verifyIssueScanStageTaskContracts(target); err != nil {
		return result, err
	}
	evidence, err = r.issueScanStageEvidenceWithRecordedRoleOutputs(target.TaskID, content, orderID, target.Draft, evidence)
	if err != nil {
		return result, err
	}
	normalized, err := normalizeIssueScanStageRuntimeEvidence(content, orderID, target.Draft, evidence)
	if err != nil {
		return result, err
	}

	advance, err := r.AdvanceIssueScanLifecycleStage(runID, target.Draft.StageID)
	result.Advance = advance
	if err != nil {
		return result, fmt.Errorf("advance issue-scan stage before completion: %w", err)
	}
	result.ParentTaskID = advance.ParentTaskID
	if target.TaskID != advance.StageTaskID {
		return result, fmt.Errorf("selected stage task drifted: got %s, want %s", target.TaskID.Value(), advance.StageTaskID.Value())
	}
	if err := r.verifyIssueScanStageTaskContracts(target); err != nil {
		return result, err
	}
	blocked, err := r.tasks.IsBlocked(target.TaskID)
	if err != nil {
		return result, err
	}
	if blocked {
		return result, fmt.Errorf("stage %q remains blocked; advance must release scheduling before completion", target.Draft.StageID)
	}
	completed, err := r.issueScanStageTaskCompleted(target.TaskID)
	if err != nil {
		return result, err
	}
	if completed {
		return result, fmt.Errorf("stage %q is already completed", target.Draft.StageID)
	}

	body, err := issueScanStageRuntimeEvidenceBody(normalized)
	if err != nil {
		return result, err
	}
	causes := compactEventIDs([]types.EventID{request.ID(), advance.ParentTaskID, target.TaskID})
	artifactID, exists, err := r.findIssueScanStageRuntimeEvidenceArtifactID(target.TaskID, body)
	if err != nil {
		return result, err
	}
	if !exists {
		if err := r.tasks.AddArtifact(r.humanID, target.TaskID, IssueScanStageRuntimeEvidenceArtifactLabel, issueScanStageRuntimeEvidenceMediaType, body, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
			return result, fmt.Errorf("record issue-scan stage runtime evidence: %w", err)
		}
		artifactID, exists, err = r.findIssueScanStageRuntimeEvidenceArtifactID(target.TaskID, body)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf("issue-scan stage runtime evidence artifact was not found after append")
		}
	}
	result.EvidenceArtifactID = artifactID
	completeCauses := compactEventIDs(append(causes, artifactID))
	if err := r.tasks.Complete(r.humanID, target.TaskID, issueScanStageCompletionSummary(normalized), completeCauses, runLaunchConversationID(content.RunID, r.convID)); err != nil {
		return result, fmt.Errorf("complete issue-scan stage task %q: %w", target.Draft.StageID, err)
	}
	result.Completed = true

	allCompleted, err := r.issueScanLifecycleAllStagesCompleted(content, orderID)
	if err != nil {
		return result, fmt.Errorf("check issue-scan lifecycle completion after completing %q: %w", target.Draft.StageID, err)
	}
	result.LifecycleComplete = allCompleted
	if advanceNext && !result.LifecycleComplete {
		next, err := r.AdvanceIssueScanLifecycleStage(runID, "")
		if err != nil {
			return result, fmt.Errorf("advance next issue-scan stage after completing %q: %w", target.Draft.StageID, err)
		}
		result.NextAdvance = &next
	}
	return result, nil
}

func normalizeIssueScanStageRuntimeEvidence(content FactoryRunRequestedContent, orderID string, draft issueScanLifecycleStageTaskDraft, evidence IssueScanStageRuntimeEvidence) (IssueScanStageRuntimeEvidence, error) {
	stageID := safeRunLaunchID(draft.StageID)
	if stageID == "" {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("stage id is required")
	}
	if strings.TrimSpace(evidence.Kind) != "" && strings.TrimSpace(evidence.Kind) != issueScanStageRuntimeEvidenceArtifactKind {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence kind %q does not match %q", evidence.Kind, issueScanStageRuntimeEvidenceArtifactKind)
	}
	if strings.TrimSpace(evidence.LifecycleVersion) != "" && strings.TrimSpace(evidence.LifecycleVersion) != issueScanLifecycleVersion {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence lifecycle_version %q does not match %q", evidence.LifecycleVersion, issueScanLifecycleVersion)
	}
	if strings.TrimSpace(evidence.RunID) != "" && strings.TrimSpace(evidence.RunID) != strings.TrimSpace(content.RunID) {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence run_id %q does not match %q", evidence.RunID, content.RunID)
	}
	if strings.TrimSpace(evidence.FactoryOrderID) != "" && strings.TrimSpace(evidence.FactoryOrderID) != orderID {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence factory_order_id %q does not match %q", evidence.FactoryOrderID, orderID)
	}
	if strings.TrimSpace(evidence.StageID) != "" && safeRunLaunchID(evidence.StageID) != stageID {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence stage_id %q does not match %q", evidence.StageID, stageID)
	}
	status := strings.TrimSpace(evidence.Status)
	switch status {
	case "", "recorded", "completed", "pass":
	default:
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence status %q is not completable", status)
	}
	summary := strings.TrimSpace(evidence.Summary)
	if summary == "" {
		return IssueScanStageRuntimeEvidence{}, fmt.Errorf("runtime evidence summary is required")
	}

	items, err := normalizeIssueScanStageRuntimeEvidenceItems(evidence.EvidenceItems)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, err
	}
	if err := requireIssueScanStageRuntimeEvidenceKeys(compactStrings(draft.Stage.RequiredEvidence), items); err != nil {
		return IssueScanStageRuntimeEvidence{}, err
	}
	roleOutputs, err := normalizeIssueScanStageRuntimeRoleOutputs(evidence.RoleOutputs)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, err
	}
	if err := requireIssueScanStageRuntimeRoleOutputs(draft.AgentExecutionPlan, roleOutputs); err != nil {
		return IssueScanStageRuntimeEvidence{}, err
	}

	boundaries := compactStrings(append(evidence.BoundaryDisclaimers,
		"stage_completion_only",
		"no_pr_readiness_claim",
		"no_human_approval_claim",
		"no_merge_or_deploy_claim",
		"later_lifecycle_stages_not_completed_by_this_evidence",
	))
	return IssueScanStageRuntimeEvidence{
		Kind:                issueScanStageRuntimeEvidenceArtifactKind,
		LifecycleVersion:    issueScanLifecycleVersion,
		RunID:               strings.TrimSpace(content.RunID),
		FactoryOrderID:      orderID,
		StageID:             stageID,
		Status:              "completed",
		Summary:             summary,
		EvidenceItems:       items,
		RoleOutputs:         roleOutputs,
		ResidualRisks:       compactStrings(evidence.ResidualRisks),
		BoundaryDisclaimers: boundaries,
		SourceRefs:          compactStrings(evidence.SourceRefs),
	}, nil
}

func normalizeIssueScanStageRuntimeEvidenceItems(values []IssueScanStageRuntimeEvidenceItem) ([]IssueScanStageRuntimeEvidenceItem, error) {
	byKey := map[string]IssueScanStageRuntimeEvidenceItem{}
	for i, value := range values {
		key := strings.TrimSpace(value.Key)
		if key == "" {
			return nil, fmt.Errorf("evidence_items[%d].key is required", i)
		}
		if _, exists := byKey[key]; exists {
			return nil, fmt.Errorf("evidence_items has duplicate key %q", key)
		}
		value.Key = key
		value.Summary = strings.TrimSpace(value.Summary)
		value.EvidenceRefs = compactStrings(value.EvidenceRefs)
		byKey[key] = value
	}
	out := make([]IssueScanStageRuntimeEvidenceItem, 0, len(byKey))
	for _, value := range byKey {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func requireIssueScanStageRuntimeEvidenceKeys(required []string, items []IssueScanStageRuntimeEvidenceItem) error {
	byKey := map[string]IssueScanStageRuntimeEvidenceItem{}
	for _, item := range items {
		byKey[item.Key] = item
	}
	var missing []string
	var missingRefs []string
	for _, key := range required {
		item, ok := byKey[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if len(item.EvidenceRefs) == 0 {
			missingRefs = append(missingRefs, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("runtime evidence missing required evidence: %s", strings.Join(missing, ", "))
	}
	if len(missingRefs) > 0 {
		return fmt.Errorf("runtime evidence required evidence lacks evidence_refs: %s", strings.Join(missingRefs, ", "))
	}
	return nil
}

func normalizeIssueScanStageRuntimeRoleOutputs(values []IssueScanStageRuntimeRoleOutput) ([]IssueScanStageRuntimeRoleOutput, error) {
	byRole := map[string]IssueScanStageRuntimeRoleOutput{}
	for i, value := range values {
		role := strings.TrimSpace(value.Role)
		if role == "" {
			return nil, fmt.Errorf("role_outputs[%d].role is required", i)
		}
		if _, exists := byRole[role]; exists {
			return nil, fmt.Errorf("role_outputs has duplicate role %q", role)
		}
		outputs, err := normalizeIssueScanStageRuntimeEvidenceItems(value.Outputs)
		if err != nil {
			return nil, fmt.Errorf("role_outputs[%d].outputs: %w", i, err)
		}
		value.Role = role
		value.Summary = strings.TrimSpace(value.Summary)
		value.EvidenceRefs = compactStrings(value.EvidenceRefs)
		value.Outputs = outputs
		byRole[role] = value
	}
	out := make([]IssueScanStageRuntimeRoleOutput, 0, len(byRole))
	for _, value := range byRole {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out, nil
}

func requireIssueScanStageRuntimeRoleOutputs(steps []OperatorQueuedRunAgentPlanStep, roleOutputs []IssueScanStageRuntimeRoleOutput) error {
	byRole := map[string]IssueScanStageRuntimeRoleOutput{}
	for _, roleOutput := range roleOutputs {
		byRole[roleOutput.Role] = roleOutput
	}
	for _, step := range steps {
		role := strings.TrimSpace(step.Role)
		if role == "" {
			continue
		}
		roleOutput, ok := byRole[role]
		if !ok {
			return fmt.Errorf("runtime evidence missing role output for %q", role)
		}
		outputs := map[string]IssueScanStageRuntimeEvidenceItem{}
		for _, output := range roleOutput.Outputs {
			outputs[output.Key] = output
		}
		var missing []string
		var missingRefs []string
		for _, key := range compactStrings(step.RequiredOutputs) {
			item, ok := outputs[key]
			if !ok {
				missing = append(missing, key)
				continue
			}
			if len(item.EvidenceRefs) == 0 && len(roleOutput.EvidenceRefs) == 0 {
				missingRefs = append(missingRefs, key)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("runtime evidence role %q missing required outputs: %s", role, strings.Join(missing, ", "))
		}
		if len(missingRefs) > 0 {
			return fmt.Errorf("runtime evidence role %q outputs lack evidence_refs: %s", role, strings.Join(missingRefs, ", "))
		}
	}
	return nil
}

func issueScanStageRuntimeEvidenceBody(evidence IssueScanStageRuntimeEvidence) (string, error) {
	encoded, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan stage runtime evidence: %w", err)
	}
	return string(encoded), nil
}

func issueScanStageCompletionSummary(evidence IssueScanStageRuntimeEvidence) string {
	return fmt.Sprintf("Issue-scan lifecycle stage %s completed with governed runtime evidence: %s", evidence.StageID, evidence.Summary)
}

func (r *Runtime) findIssueScanStageRuntimeEvidenceArtifactID(taskID types.EventID, body string) (types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("find issue-scan stage runtime evidence artifact: %w", err)
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		if strings.TrimSpace(artifact.Label) == IssueScanStageRuntimeEvidenceArtifactLabel && strings.TrimSpace(artifact.Body) == strings.TrimSpace(body) {
			return artifact.ID, true, nil
		}
	}
	return types.EventID{}, false, nil
}

func (r *Runtime) issueScanLifecycleAllStagesCompleted(content FactoryRunRequestedContent, orderID string) (bool, error) {
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return false, err
	}
	for _, draft := range drafts {
		taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, draft.Options.CanonicalTaskID)
		if err != nil {
			return false, err
		}
		if !exists || strings.TrimSpace(factoryOrderID) != orderID {
			return false, nil
		}
		completed, err := r.issueScanStageTaskCompleted(taskID)
		if err != nil {
			return false, err
		}
		if !completed {
			return false, nil
		}
	}
	return len(drafts) > 0, nil
}
