package hive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// IssueScanStageAdvanceResult summarizes one audited attempt to release the
// dependency barrier for the next issue-scan lifecycle stage.
type IssueScanStageAdvanceResult struct {
	RunID               string
	FactoryOrderID      string
	ParentTaskID        types.EventID
	StageID             string
	StageTaskID         types.EventID
	PreviousStageID     string
	PreviousStageTaskID types.EventID
	Released            bool
	AlreadyReady        bool
	Dispatch            RunLaunchDispatchResult
}

// AdvanceIssueScanLifecycleStage makes the next selected issue-scan stage
// runnable after verifying its declared contracts are present. It does not
// complete the parent FactoryOrder and does not claim stage completion, PR
// readiness, Human approval, merge, or deploy evidence.
func (r *Runtime) AdvanceIssueScanLifecycleStage(runID, stageID string) (IssueScanStageAdvanceResult, error) {
	runID = strings.TrimSpace(runID)
	stageID = strings.TrimSpace(stageID)
	result := IssueScanStageAdvanceResult{RunID: runID}
	if r == nil || r.store == nil || r.tasks == nil {
		return result, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return result, fmt.Errorf("run_id is required")
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

	dispatch, err := r.DispatchQueuedRunLaunch(runID)
	result.Dispatch = dispatch
	if err != nil {
		return result, fmt.Errorf("dispatch queued issue-scan run %q: %w", runID, err)
	}

	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return result, err
	}
	result.FactoryOrderID = orderID
	dispatched, err := dispatchedFactoryOrderIDs(r.store)
	if err != nil {
		return result, err
	}
	parentTaskID, ok := dispatched[orderID]
	if !ok || parentTaskID == (types.EventID{}) {
		return result, fmt.Errorf("queued run %q has not been dispatched to a FactoryOrder task", runID)
	}
	result.ParentTaskID = parentTaskID

	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return result, err
	}
	if len(drafts) == 0 {
		return result, fmt.Errorf("queued run %q has no issue-scan lifecycle stage tasks", runID)
	}

	target, previous, err := r.selectIssueScanStageAdvanceTarget(drafts, stageID, orderID)
	if err != nil {
		return result, err
	}
	result.StageID = target.Draft.StageID
	result.StageTaskID = target.TaskID
	if previous != nil {
		result.PreviousStageID = previous.Draft.StageID
		result.PreviousStageTaskID = previous.TaskID
	}

	if err := r.verifyIssueScanStageTaskContracts(target); err != nil {
		return result, err
	}
	if err := r.verifyIssueScanStageTaskDependencies(target, previous, parentTaskID); err != nil {
		return result, err
	}
	blocked, err := r.tasks.IsBlocked(target.TaskID)
	if err != nil {
		return result, err
	}
	if !blocked {
		if previous != nil {
			completed, err := r.issueScanStageTaskCompleted(previous.TaskID)
			if err != nil {
				return result, err
			}
			if !completed {
				return result, fmt.Errorf("stage %q is already unblocked before previous stage %q completed", target.Draft.StageID, previous.Draft.StageID)
			}
		}
		result.AlreadyReady = true
		return result, nil
	}
	if previous != nil {
		completed, err := r.issueScanStageTaskCompleted(previous.TaskID)
		if err != nil {
			return result, err
		}
		if !completed {
			return result, fmt.Errorf("stage %q is blocked until previous stage %q completes", target.Draft.StageID, previous.Draft.StageID)
		}
	}

	causes := compactEventIDs([]types.EventID{request.ID(), parentTaskID, result.PreviousStageTaskID, target.TaskID})
	if err := r.tasks.UnblockTask(r.humanID, target.TaskID, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
		return result, fmt.Errorf("release issue-scan stage %q dependency barrier: %w", target.Draft.StageID, err)
	}
	result.Released = true
	comment := issueScanStageAdvanceComment(result)
	if err := r.tasks.AddComment(target.TaskID, comment, r.humanID, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
		return result, fmt.Errorf("record issue-scan stage advance comment after release: %w", err)
	}
	return result, nil
}

type issueScanStageAdvanceTarget struct {
	Draft  issueScanLifecycleStageTaskDraft
	TaskID types.EventID
}

func (r *Runtime) selectIssueScanStageAdvanceTarget(drafts []issueScanLifecycleStageTaskDraft, stageID, orderID string) (*issueScanStageAdvanceTarget, *issueScanStageAdvanceTarget, error) {
	wantStageID := safeRunLaunchID(stageID)
	if strings.TrimSpace(stageID) != "" && wantStageID == "" {
		return nil, nil, fmt.Errorf("stage id %q is unsafe", stageID)
	}
	var previous *issueScanStageAdvanceTarget
	for _, draft := range drafts {
		taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, draft.Options.CanonicalTaskID)
		if err != nil {
			return nil, nil, fmt.Errorf("find stage task %q: %w", draft.StageID, err)
		}
		if !exists {
			return nil, nil, fmt.Errorf("stage task %q has not been created", draft.StageID)
		}
		if strings.TrimSpace(factoryOrderID) != orderID {
			return nil, nil, fmt.Errorf("stage task %q belongs to factory order %q, want %q", draft.StageID, factoryOrderID, orderID)
		}
		target := &issueScanStageAdvanceTarget{Draft: draft, TaskID: taskID}
		completed, err := r.issueScanStageTaskCompleted(taskID)
		if err != nil {
			return nil, nil, err
		}
		if wantStageID != "" {
			if safeRunLaunchID(draft.StageID) == wantStageID {
				if completed {
					return nil, nil, fmt.Errorf("stage %q is already completed", draft.StageID)
				}
				return target, previous, nil
			}
			previous = target
			continue
		}
		if !completed {
			return target, previous, nil
		}
		previous = target
	}
	if wantStageID != "" {
		return nil, nil, fmt.Errorf("stage %q not found", stageID)
	}
	return nil, nil, fmt.Errorf("issue-scan lifecycle has no incomplete stages")
}

func (r *Runtime) issueScanStageTaskCompleted(taskID types.EventID) (bool, error) {
	status, err := r.tasks.GetCompatibilityStatus(taskID)
	if err != nil {
		return false, fmt.Errorf("project stage task %s compatibility status: %w", taskID, err)
	}
	return status == work.LegacyStatusCompleted, nil
}

func (r *Runtime) verifyIssueScanStageTaskContracts(target *issueScanStageAdvanceTarget) error {
	readiness, err := r.tasks.Readiness(target.TaskID)
	if err != nil {
		return fmt.Errorf("read stage task %q readiness: %w", target.Draft.StageID, err)
	}
	if !readiness.Ready {
		return fmt.Errorf("stage task %q is missing readiness gates: %s", target.Draft.StageID, strings.Join(readiness.MissingGates, ", "))
	}
	artifacts, err := r.tasks.ListArtifacts(target.TaskID)
	if err != nil {
		return fmt.Errorf("list stage task %q artifacts: %w", target.Draft.StageID, err)
	}
	required := map[string]bool{
		work.GateDefinitionOfDone:                 false,
		work.GateAcceptanceCriteria:               false,
		work.GateTestPlan:                         false,
		IssueScanStageRoleContractArtifactLabel:   false,
		IssueScanStageOutputContractArtifactLabel: false,
	}
	for _, artifact := range artifacts {
		label := strings.TrimSpace(artifact.Label)
		if _, ok := required[label]; ok && strings.TrimSpace(artifact.Body) != "" {
			required[label] = true
		}
		switch label {
		case IssueScanStageRoleContractArtifactLabel:
			if err := verifyIssueScanStageTaskRoleContractBody(target.Draft, artifact.Body); err != nil {
				return fmt.Errorf("stage task %q role contract %s: %w", target.Draft.StageID, artifact.ID.Value(), err)
			}
		case IssueScanStageOutputContractArtifactLabel:
			if err := verifyIssueScanStageTaskOutputContractBody(target.Draft, artifact.Body); err != nil {
				return fmt.Errorf("stage task %q output contract %s: %w", target.Draft.StageID, artifact.ID.Value(), err)
			}
		}
	}
	var missing []string
	for label, ok := range required {
		if !ok {
			missing = append(missing, label)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("stage task %q is missing issue-scan contracts: %s", target.Draft.StageID, strings.Join(missing, ", "))
	}
	return nil
}

func verifyIssueScanStageTaskRoleContractBody(draft issueScanLifecycleStageTaskDraft, body string) error {
	raw := strings.TrimSpace(body)
	if raw == "" {
		return fmt.Errorf("body is empty")
	}
	var payload struct {
		Kind               string                           `json:"kind"`
		LifecycleVersion   string                           `json:"lifecycle_version"`
		RunID              string                           `json:"run_id"`
		FactoryOrderID     string                           `json:"factory_order_id"`
		StageID            string                           `json:"stage_id"`
		StageIndex         int                              `json:"stage_index"`
		StageCount         int                              `json:"stage_count"`
		Stage              OperatorQueuedRunLifecycleStage  `json:"stage"`
		AgentExecutionPlan []OperatorQueuedRunAgentPlanStep `json:"agent_execution_plan"`
		EvidenceKind       string                           `json:"evidence_kind"`
		EvidenceStatus     string                           `json:"evidence_status"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageRoleContractArtifactKind {
		return fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageRoleContractArtifactKind)
	}
	if err := verifyIssueScanStageTaskContractEnvelope(draft, payload.LifecycleVersion, payload.RunID, payload.FactoryOrderID, payload.StageID, payload.StageIndex, payload.StageCount, payload.Stage); err != nil {
		return err
	}
	if err := verifyIssueScanStageTaskAgentPlan(payload.AgentExecutionPlan, draft.AgentExecutionPlan); err != nil {
		return err
	}
	if strings.TrimSpace(payload.EvidenceKind) != "required_role_contract_not_runtime_execution" {
		return fmt.Errorf("evidence_kind %q does not match required_role_contract_not_runtime_execution", payload.EvidenceKind)
	}
	if strings.TrimSpace(payload.EvidenceStatus) != "pending_runtime_evidence" {
		return fmt.Errorf("evidence_status %q does not match pending_runtime_evidence", payload.EvidenceStatus)
	}
	return nil
}

func verifyIssueScanStageTaskOutputContractBody(draft issueScanLifecycleStageTaskDraft, body string) error {
	raw := strings.TrimSpace(body)
	if raw == "" {
		return fmt.Errorf("body is empty")
	}
	var payload struct {
		Kind                string                                   `json:"kind"`
		LifecycleVersion    string                                   `json:"lifecycle_version"`
		RunID               string                                   `json:"run_id"`
		FactoryOrderID      string                                   `json:"factory_order_id"`
		StageID             string                                   `json:"stage_id"`
		StageIndex          int                                      `json:"stage_index"`
		StageCount          int                                      `json:"stage_count"`
		Stage               OperatorQueuedRunLifecycleStage          `json:"stage"`
		RequiredEvidence    []string                                 `json:"required_evidence"`
		ExpectedOutputs     []string                                 `json:"expected_outputs"`
		RoleOutputContracts []CivilizationAssemblyRoleOutputContract `json:"role_output_contracts"`
		EvidenceKind        string                                   `json:"evidence_kind"`
		EvidenceStatus      string                                   `json:"evidence_status"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageOutputContractArtifactKind {
		return fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageOutputContractArtifactKind)
	}
	if err := verifyIssueScanStageTaskContractEnvelope(draft, payload.LifecycleVersion, payload.RunID, payload.FactoryOrderID, payload.StageID, payload.StageIndex, payload.StageCount, payload.Stage); err != nil {
		return err
	}
	if !sameIssueScanStageTaskContractStrings(payload.RequiredEvidence, draft.Stage.RequiredEvidence) {
		return fmt.Errorf("required_evidence %v does not match expected %v", payload.RequiredEvidence, draft.Stage.RequiredEvidence)
	}
	expectedOutputs := issueScanLifecycleStageTaskExpectedOutputs(draft.Stage, draft.AgentExecutionPlan)
	if !sameIssueScanStageTaskContractStrings(payload.ExpectedOutputs, expectedOutputs) {
		return fmt.Errorf("expected_outputs %v does not match expected %v", payload.ExpectedOutputs, expectedOutputs)
	}
	if err := verifyIssueScanStageTaskRoleOutputContracts(payload.RoleOutputContracts, issueScanStageRoleOutputContracts(draft.Stage, draft.AgentExecutionPlan)); err != nil {
		return err
	}
	if strings.TrimSpace(payload.EvidenceKind) != "required_output_contract_not_runtime_execution" {
		return fmt.Errorf("evidence_kind %q does not match required_output_contract_not_runtime_execution", payload.EvidenceKind)
	}
	if strings.TrimSpace(payload.EvidenceStatus) != "pending_runtime_evidence" {
		return fmt.Errorf("evidence_status %q does not match pending_runtime_evidence", payload.EvidenceStatus)
	}
	return nil
}

func verifyIssueScanStageTaskContractEnvelope(draft issueScanLifecycleStageTaskDraft, lifecycleVersion, runID, factoryOrderID, stageID string, stageIndex, stageCount int, stage OperatorQueuedRunLifecycleStage) error {
	if strings.TrimSpace(lifecycleVersion) != issueScanLifecycleVersion {
		return fmt.Errorf("lifecycle_version %q does not match %q", lifecycleVersion, issueScanLifecycleVersion)
	}
	if strings.TrimSpace(draft.RunID) == "" {
		return fmt.Errorf("draft run_id is required")
	}
	if strings.TrimSpace(runID) != strings.TrimSpace(draft.RunID) {
		return fmt.Errorf("run_id %q does not match %q", runID, draft.RunID)
	}
	if strings.TrimSpace(draft.Options.FactoryOrderID) == "" {
		return fmt.Errorf("draft factory_order_id is required")
	}
	if strings.TrimSpace(factoryOrderID) != strings.TrimSpace(draft.Options.FactoryOrderID) {
		return fmt.Errorf("factory_order_id %q does not match %q", factoryOrderID, draft.Options.FactoryOrderID)
	}
	if safeRunLaunchID(stageID) == "" || safeRunLaunchID(stageID) != safeRunLaunchID(draft.StageID) {
		return fmt.Errorf("stage_id %q does not match %q", stageID, draft.StageID)
	}
	if stageIndex != draft.StageIndex {
		return fmt.Errorf("stage_index %d does not match %d", stageIndex, draft.StageIndex)
	}
	if stageCount != draft.StageCount {
		return fmt.Errorf("stage_count %d does not match %d", stageCount, draft.StageCount)
	}
	return verifyIssueScanStageTaskContractStage(stage, draft.Stage)
}

func verifyIssueScanStageTaskContractStage(got, want OperatorQueuedRunLifecycleStage) error {
	if strings.TrimSpace(got.ID) != strings.TrimSpace(want.ID) {
		return fmt.Errorf("stage.id %q does not match %q", got.ID, want.ID)
	}
	if strings.TrimSpace(got.Name) != strings.TrimSpace(want.Name) {
		return fmt.Errorf("stage.name %q does not match %q", got.Name, want.Name)
	}
	if !sameIssueScanStageTaskContractStrings(got.RequiredRoles, want.RequiredRoles) {
		return fmt.Errorf("stage.required_roles %v does not match expected %v", got.RequiredRoles, want.RequiredRoles)
	}
	if !sameIssueScanStageTaskContractStrings(got.RequiredEvidence, want.RequiredEvidence) {
		return fmt.Errorf("stage.required_evidence %v does not match expected %v", got.RequiredEvidence, want.RequiredEvidence)
	}
	if strings.TrimSpace(got.AuthorityBoundary) != strings.TrimSpace(want.AuthorityBoundary) {
		return fmt.Errorf("stage.authority_boundary %q does not match %q", got.AuthorityBoundary, want.AuthorityBoundary)
	}
	if strings.TrimSpace(got.CompletionGate) != strings.TrimSpace(want.CompletionGate) {
		return fmt.Errorf("stage.completion_gate %q does not match %q", got.CompletionGate, want.CompletionGate)
	}
	return nil
}

func verifyIssueScanStageTaskAgentPlan(got, want []OperatorQueuedRunAgentPlanStep) error {
	got = compactOperatorQueuedRunAgentPlanSteps(got)
	want = compactOperatorQueuedRunAgentPlanSteps(want)
	if len(got) != len(want) {
		return fmt.Errorf("agent_execution_plan length %d does not match expected %d", len(got), len(want))
	}
	for i := range got {
		if err := verifyIssueScanStageTaskAgentPlanStep(i, got[i], want[i]); err != nil {
			return err
		}
	}
	return nil
}

func verifyIssueScanStageTaskAgentPlanStep(index int, got, want OperatorQueuedRunAgentPlanStep) error {
	prefix := fmt.Sprintf("agent_execution_plan[%d]", index)
	if strings.TrimSpace(got.ID) != strings.TrimSpace(want.ID) {
		return fmt.Errorf("%s.id %q does not match %q", prefix, got.ID, want.ID)
	}
	if strings.TrimSpace(got.StageID) != strings.TrimSpace(want.StageID) {
		return fmt.Errorf("%s.stage_id %q does not match %q", prefix, got.StageID, want.StageID)
	}
	if strings.TrimSpace(got.Role) != strings.TrimSpace(want.Role) {
		return fmt.Errorf("%s.role %q does not match %q", prefix, got.Role, want.Role)
	}
	if got.CanOperate != want.CanOperate {
		return fmt.Errorf("%s.can_operate %v does not match %v", prefix, got.CanOperate, want.CanOperate)
	}
	if strings.TrimSpace(got.Objective) != strings.TrimSpace(want.Objective) {
		return fmt.Errorf("%s.objective %q does not match %q", prefix, got.Objective, want.Objective)
	}
	if !sameIssueScanStageTaskContractStrings(got.RequiredInputs, want.RequiredInputs) {
		return fmt.Errorf("%s.required_inputs %v does not match expected %v", prefix, got.RequiredInputs, want.RequiredInputs)
	}
	if !sameIssueScanStageTaskContractStrings(got.RequiredOutputs, want.RequiredOutputs) {
		return fmt.Errorf("%s.required_outputs %v does not match expected %v", prefix, got.RequiredOutputs, want.RequiredOutputs)
	}
	if strings.TrimSpace(got.AuthorityBoundary) != strings.TrimSpace(want.AuthorityBoundary) {
		return fmt.Errorf("%s.authority_boundary %q does not match %q", prefix, got.AuthorityBoundary, want.AuthorityBoundary)
	}
	if strings.TrimSpace(got.CompletionGate) != strings.TrimSpace(want.CompletionGate) {
		return fmt.Errorf("%s.completion_gate %q does not match %q", prefix, got.CompletionGate, want.CompletionGate)
	}
	if strings.TrimSpace(got.EvidenceStatus) != strings.TrimSpace(want.EvidenceStatus) {
		return fmt.Errorf("%s.evidence_status %q does not match %q", prefix, got.EvidenceStatus, want.EvidenceStatus)
	}
	return nil
}

func verifyIssueScanStageTaskRoleOutputContracts(got, want []CivilizationAssemblyRoleOutputContract) error {
	got = compactCivilizationAssemblyRoleOutputContracts(got)
	want = compactCivilizationAssemblyRoleOutputContracts(want)
	if len(got) != len(want) {
		return fmt.Errorf("role_output_contracts length %d does not match expected %d", len(got), len(want))
	}
	for i := range got {
		prefix := fmt.Sprintf("role_output_contracts[%d]", i)
		if strings.TrimSpace(got[i].Role) != strings.TrimSpace(want[i].Role) {
			return fmt.Errorf("%s.role %q does not match %q", prefix, got[i].Role, want[i].Role)
		}
		if got[i].CanOperate != want[i].CanOperate {
			return fmt.Errorf("%s.can_operate %v does not match %v", prefix, got[i].CanOperate, want[i].CanOperate)
		}
		if !sameIssueScanStageTaskContractStrings(got[i].RequiredOutputs, want[i].RequiredOutputs) {
			return fmt.Errorf("%s.required_outputs %v does not match expected %v", prefix, got[i].RequiredOutputs, want[i].RequiredOutputs)
		}
		if strings.TrimSpace(got[i].AuthorityBoundary) != strings.TrimSpace(want[i].AuthorityBoundary) {
			return fmt.Errorf("%s.authority_boundary %q does not match %q", prefix, got[i].AuthorityBoundary, want[i].AuthorityBoundary)
		}
		if strings.TrimSpace(got[i].CompletionGate) != strings.TrimSpace(want[i].CompletionGate) {
			return fmt.Errorf("%s.completion_gate %q does not match %q", prefix, got[i].CompletionGate, want[i].CompletionGate)
		}
		if strings.TrimSpace(got[i].EvidenceStatus) != strings.TrimSpace(want[i].EvidenceStatus) {
			return fmt.Errorf("%s.evidence_status %q does not match %q", prefix, got[i].EvidenceStatus, want[i].EvidenceStatus)
		}
	}
	return nil
}

func sameIssueScanStageTaskContractStrings(got, want []string) bool {
	return sameOperatorProjectionStrings(compactStrings(got), compactStrings(want))
}

func (r *Runtime) verifyIssueScanStageTaskDependencies(target, previous *issueScanStageAdvanceTarget, parentTaskID types.EventID) error {
	deps, err := r.tasks.GetDependencies(target.TaskID)
	if err != nil {
		return fmt.Errorf("read stage task %q dependencies: %w", target.Draft.StageID, err)
	}
	// Work UnblockTask records an override and preserves dependency edges. The
	// exact-set check prevents this command from clearing unrelated blockers.
	expected := []types.EventID{parentTaskID}
	if previous != nil {
		expected = append(expected, previous.TaskID)
	}
	if len(deps) != len(expected) {
		return fmt.Errorf("stage task %q has unexpected dependencies: got %s, want %s", target.Draft.StageID, formatIssueScanStageDependencyIDs(deps), formatIssueScanStageDependencyIDs(expected))
	}
	for _, want := range expected {
		if !issueScanStageDependencyContains(deps, want) {
			return fmt.Errorf("stage task %q has unexpected dependencies: got %s, want %s", target.Draft.StageID, formatIssueScanStageDependencyIDs(deps), formatIssueScanStageDependencyIDs(expected))
		}
	}
	return nil
}

func issueScanStageDependencyContains(values []types.EventID, want types.EventID) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func formatIssueScanStageDependencyIDs(values []types.EventID) string {
	if len(values) == 0 {
		return "none"
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Value())
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func issueScanStageAdvanceComment(result IssueScanStageAdvanceResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Issue-scan stage advance released the Work dependency barrier for lifecycle stage `%s`.\n\n", result.StageID)
	fmt.Fprintf(&b, "Run: `%s`\n", result.RunID)
	fmt.Fprintf(&b, "FactoryOrder: `%s`\n", result.FactoryOrderID)
	fmt.Fprintf(&b, "Parent task: `%s`\n", result.ParentTaskID.Value())
	if result.PreviousStageID != "" {
		fmt.Fprintf(&b, "Previous stage: `%s` (`%s`)\n", result.PreviousStageID, result.PreviousStageTaskID.Value())
	}
	b.WriteString("\nThis releases scheduling only. It is not stage completion, PR readiness, Human approval, merge, or deploy evidence.")
	return strings.TrimSpace(b.String())
}
