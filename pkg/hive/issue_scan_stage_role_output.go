package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const (
	IssueScanStageRoleOutputArtifactLabel = "issue_scan_stage_role_output"
	issueScanStageRoleOutputArtifactKind  = "issue_scan_stage_role_output"
)

// IssueScanStageRoleOutputEvidence is a durable output packet from one civic
// role participating in an issue-scan lifecycle stage. It records that role's
// contribution only; it is not stage completion, PR readiness, Human approval,
// merge, or deploy evidence.
type IssueScanStageRoleOutputEvidence struct {
	Kind                string                              `json:"kind,omitempty"`
	LifecycleVersion    string                              `json:"lifecycle_version,omitempty"`
	RunID               string                              `json:"run_id,omitempty"`
	FactoryOrderID      string                              `json:"factory_order_id,omitempty"`
	StageID             string                              `json:"stage_id,omitempty"`
	Role                string                              `json:"role"`
	Status              string                              `json:"status,omitempty"`
	Summary             string                              `json:"summary"`
	EvidenceRefs        []string                            `json:"evidence_refs,omitempty"`
	Outputs             []IssueScanStageRuntimeEvidenceItem `json:"outputs"`
	AuthorityBoundary   string                              `json:"authority_boundary,omitempty"`
	CompletionGate      string                              `json:"completion_gate,omitempty"`
	BoundaryDisclaimers []string                            `json:"boundary_disclaimers,omitempty"`
	SourceRefs          []string                            `json:"source_refs,omitempty"`
}

// IssueScanStageRoleOutputResult summarizes one validated role-output append.
type IssueScanStageRoleOutputResult struct {
	RunID              string
	FactoryOrderID     string
	ParentTaskID       types.EventID
	StageID            string
	StageTaskID        types.EventID
	Role               string
	OutputArtifactID   types.EventID
	Recorded           bool
	AlreadyRecorded    bool
	RequiredOutputKeys []string
}

// RecordIssueScanStageRoleOutput validates and records one role's required
// outputs for an issue-scan stage. It may materialize the queued run and stage
// task drafts so the output has a durable Work task target, but it does not
// unblock, complete, merge, deploy, or request Human approval.
func (r *Runtime) RecordIssueScanStageRoleOutput(runID, stageID string, output IssueScanStageRoleOutputEvidence) (IssueScanStageRoleOutputResult, error) {
	runID = strings.TrimSpace(runID)
	stageID = strings.TrimSpace(stageID)
	result := IssueScanStageRoleOutputResult{RunID: runID}
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
		return result, fmt.Errorf("dispatch queued issue-scan run %q before recording role output: %w", runID, err)
	}
	dispatched, err := dispatchedFactoryOrderIDs(r.store)
	if err != nil {
		return result, err
	}
	parentTaskID, ok := dispatched[orderID]
	if !ok || parentTaskID == (types.EventID{}) {
		return result, fmt.Errorf("queued run %q has not been dispatched to a FactoryOrder task", runID)
	}
	result.ParentTaskID = parentTaskID
	target, _, err := r.selectIssueScanStageAdvanceTarget(drafts, stageID, orderID)
	if err != nil {
		return result, err
	}
	if err := r.verifyIssueScanStageTaskContracts(target); err != nil {
		return result, err
	}
	result.StageID = target.Draft.StageID
	result.StageTaskID = target.TaskID

	normalized, step, err := normalizeIssueScanStageRoleOutput(content, orderID, target.Draft, output)
	if err != nil {
		return result, err
	}
	result.Role = normalized.Role
	result.RequiredOutputKeys = compactStrings(step.RequiredOutputs)
	body, err := issueScanStageRoleOutputBody(normalized)
	if err != nil {
		return result, err
	}
	artifactID, exists, err := r.findIssueScanStageRoleOutputArtifactID(target.TaskID, body)
	if err != nil {
		return result, err
	}
	if exists {
		result.OutputArtifactID = artifactID
		result.AlreadyRecorded = true
		return result, nil
	}
	causes := compactEventIDs([]types.EventID{request.ID(), parentTaskID, target.TaskID})
	if err := r.tasks.AddArtifact(r.humanID, target.TaskID, IssueScanStageRoleOutputArtifactLabel, issueScanStageRuntimeEvidenceMediaType, body, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
		return result, fmt.Errorf("record issue-scan role output: %w", err)
	}
	artifactID, exists, err = r.findIssueScanStageRoleOutputArtifactID(target.TaskID, body)
	if err != nil {
		return result, err
	}
	if !exists {
		return result, fmt.Errorf("issue-scan role output artifact was not found after append")
	}
	result.OutputArtifactID = artifactID
	result.Recorded = true
	return result, nil
}

// CompleteReadyIssueScanLifecycleStages completes the next incomplete stage for
// each dispatched issue-scan run only when recorded role-output artifacts cover
// every required role output and every required stage evidence key. Missing
// evidence is a normal "not ready yet" condition, not an error.
func (r *Runtime) CompleteReadyIssueScanLifecycleStages(result RunLaunchDispatchResult) ([]IssueScanStageCompletionResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	completions := make([]IssueScanStageCompletionResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		evidence, ready, err := r.issueScanStageRuntimeEvidenceFromRecordedOutputs(runID, "")
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if !ready {
			continue
		}
		completion, err := r.CompleteIssueScanLifecycleStage(runID, evidence.StageID, evidence, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q stage %q: %w", runID, evidence.StageID, err))
			continue
		}
		completions = append(completions, completion)
	}
	return completions, errors.Join(errs...)
}

func (r *Runtime) issueScanStageRuntimeEvidenceFromRecordedOutputs(runID, stageID string) (IssueScanStageRuntimeEvidence, bool, error) {
	runID = strings.TrimSpace(runID)
	stageID = strings.TrimSpace(stageID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return IssueScanStageRuntimeEvidence{}, false, err
	} else if parked {
		return IssueScanStageRuntimeEvidence{}, false, nil
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, false, err
	}
	if len(requests) == 0 {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return IssueScanStageRuntimeEvidence{}, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, false, err
	}
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("dispatch queued issue-scan run before stage auto-completion: %w", err)
	}
	target, _, err := r.selectIssueScanStageAdvanceTarget(drafts, stageID, orderID)
	if err != nil {
		if strings.Contains(err.Error(), "issue-scan lifecycle has no incomplete stages") {
			return IssueScanStageRuntimeEvidence{}, false, nil
		}
		return IssueScanStageRuntimeEvidence{}, false, err
	}
	if err := r.verifyIssueScanStageTaskContracts(target); err != nil {
		return IssueScanStageRuntimeEvidence{}, false, err
	}
	recorded, sourceRefs, err := r.issueScanStageRecordedRuntimeRoleOutputs(target.TaskID, content, orderID, target.Draft)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, false, err
	}
	if len(recorded) == 0 {
		return IssueScanStageRuntimeEvidence{}, false, nil
	}
	if err := requireIssueScanStageRuntimeRoleOutputs(target.Draft.AgentExecutionPlan, recorded); err != nil {
		return IssueScanStageRuntimeEvidence{}, false, nil
	}
	evidenceItems := issueScanStageEvidenceItemsFromRoleOutputs(target.Draft, recorded)
	if err := requireIssueScanStageRuntimeEvidenceKeys(compactStrings(target.Draft.Stage.RequiredEvidence), evidenceItems); err != nil {
		return IssueScanStageRuntimeEvidence{}, false, nil
	}
	return IssueScanStageRuntimeEvidence{
		StageID:       target.Draft.StageID,
		Summary:       fmt.Sprintf("auto-completed from recorded role outputs for %s", target.Draft.StageID),
		EvidenceItems: evidenceItems,
		RoleOutputs:   recorded,
		SourceRefs:    sourceRefs,
	}, true, nil
}

func normalizeIssueScanStageRoleOutput(content FactoryRunRequestedContent, orderID string, draft issueScanLifecycleStageTaskDraft, output IssueScanStageRoleOutputEvidence) (IssueScanStageRoleOutputEvidence, OperatorQueuedRunAgentPlanStep, error) {
	stageID := safeRunLaunchID(draft.StageID)
	if stageID == "" {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("stage id is required")
	}
	if strings.TrimSpace(output.Kind) != "" && strings.TrimSpace(output.Kind) != issueScanStageRoleOutputArtifactKind {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output kind %q does not match %q", output.Kind, issueScanStageRoleOutputArtifactKind)
	}
	if strings.TrimSpace(output.LifecycleVersion) != "" && strings.TrimSpace(output.LifecycleVersion) != issueScanLifecycleVersion {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output lifecycle_version %q does not match %q", output.LifecycleVersion, issueScanLifecycleVersion)
	}
	if strings.TrimSpace(output.RunID) != "" && strings.TrimSpace(output.RunID) != strings.TrimSpace(content.RunID) {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output run_id %q does not match %q", output.RunID, content.RunID)
	}
	if strings.TrimSpace(output.FactoryOrderID) != "" && strings.TrimSpace(output.FactoryOrderID) != orderID {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output factory_order_id %q does not match %q", output.FactoryOrderID, orderID)
	}
	if strings.TrimSpace(output.StageID) != "" && safeRunLaunchID(output.StageID) != stageID {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output stage_id %q does not match %q", output.StageID, stageID)
	}
	status := strings.TrimSpace(output.Status)
	switch status {
	case "", "recorded", "pass":
	default:
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output status %q is not recordable", status)
	}
	role := strings.TrimSpace(output.Role)
	if role == "" {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output role is required")
	}
	step, ok := issueScanStagePlanStepByRole(draft.AgentExecutionPlan, role)
	if !ok {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role %q is not required for stage %q", role, draft.StageID)
	}
	summary := strings.TrimSpace(output.Summary)
	if summary == "" {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output summary is required")
	}
	outputs, err := normalizeIssueScanStageRuntimeEvidenceItems(output.Outputs)
	if err != nil {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, fmt.Errorf("role output outputs: %w", err)
	}
	roleOutput := IssueScanStageRuntimeRoleOutput{
		Role:         role,
		Summary:      summary,
		EvidenceRefs: compactStrings(output.EvidenceRefs),
		Outputs:      outputs,
	}
	if err := requireIssueScanStageRuntimeRoleOutputs([]OperatorQueuedRunAgentPlanStep{step}, []IssueScanStageRuntimeRoleOutput{roleOutput}); err != nil {
		return IssueScanStageRoleOutputEvidence{}, OperatorQueuedRunAgentPlanStep{}, err
	}
	boundary := strings.TrimSpace(output.AuthorityBoundary)
	if boundary == "" {
		boundary = valueOr(step.AuthorityBoundary, draft.Stage.AuthorityBoundary)
	}
	completionGate := strings.TrimSpace(output.CompletionGate)
	if completionGate == "" {
		completionGate = valueOr(step.CompletionGate, draft.Stage.CompletionGate)
	}
	boundaries := compactStrings(append(output.BoundaryDisclaimers,
		"role_output_only",
		"not_stage_completion",
		"no_pr_readiness_claim",
		"no_human_approval_claim",
		"no_merge_or_deploy_claim",
	))
	return IssueScanStageRoleOutputEvidence{
		Kind:                issueScanStageRoleOutputArtifactKind,
		LifecycleVersion:    issueScanLifecycleVersion,
		RunID:               strings.TrimSpace(content.RunID),
		FactoryOrderID:      orderID,
		StageID:             stageID,
		Role:                role,
		Status:              "recorded",
		Summary:             summary,
		EvidenceRefs:        compactStrings(output.EvidenceRefs),
		Outputs:             outputs,
		AuthorityBoundary:   boundary,
		CompletionGate:      completionGate,
		BoundaryDisclaimers: boundaries,
		SourceRefs:          compactStrings(output.SourceRefs),
	}, step, nil
}

func issueScanStagePlanStepByRole(steps []OperatorQueuedRunAgentPlanStep, role string) (OperatorQueuedRunAgentPlanStep, bool) {
	role = strings.TrimSpace(role)
	for _, step := range steps {
		if strings.TrimSpace(step.Role) == role {
			return step, true
		}
	}
	return OperatorQueuedRunAgentPlanStep{}, false
}

func issueScanStageRoleOutputBody(output IssueScanStageRoleOutputEvidence) (string, error) {
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan role output: %w", err)
	}
	return string(encoded), nil
}

func (r *Runtime) findIssueScanStageRoleOutputArtifactID(taskID types.EventID, body string) (types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("find issue-scan role output artifact: %w", err)
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		if strings.TrimSpace(artifact.Label) == IssueScanStageRoleOutputArtifactLabel && strings.TrimSpace(artifact.Body) == strings.TrimSpace(body) {
			return artifact.ID, true, nil
		}
	}
	return types.EventID{}, false, nil
}

func (r *Runtime) issueScanStageEvidenceWithRecordedRoleOutputs(taskID types.EventID, content FactoryRunRequestedContent, orderID string, draft issueScanLifecycleStageTaskDraft, evidence IssueScanStageRuntimeEvidence) (IssueScanStageRuntimeEvidence, error) {
	recorded, refs, err := r.issueScanStageRecordedRuntimeRoleOutputs(taskID, content, orderID, draft)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, err
	}
	if len(recorded) == 0 {
		return evidence, nil
	}
	byRole := map[string]bool{}
	for _, existing := range evidence.RoleOutputs {
		role := strings.TrimSpace(existing.Role)
		if role != "" {
			byRole[role] = true
		}
	}
	for _, output := range recorded {
		if byRole[output.Role] {
			continue
		}
		evidence.RoleOutputs = append(evidence.RoleOutputs, output)
		byRole[output.Role] = true
	}
	evidence.SourceRefs = compactStrings(append(evidence.SourceRefs, refs...))
	evidence = issueScanStageEvidenceWithRecordedEvidenceItems(draft, recorded, evidence)
	return evidence, nil
}

func issueScanStageEvidenceWithRecordedEvidenceItems(draft issueScanLifecycleStageTaskDraft, roleOutputs []IssueScanStageRuntimeRoleOutput, evidence IssueScanStageRuntimeEvidence) IssueScanStageRuntimeEvidence {
	byKey := map[string]bool{}
	for _, item := range evidence.EvidenceItems {
		key := strings.TrimSpace(item.Key)
		if key != "" {
			byKey[key] = true
		}
	}
	derived := issueScanStageEvidenceItemsFromRoleOutputs(draft, roleOutputs)
	for _, item := range derived {
		if byKey[item.Key] {
			continue
		}
		evidence.EvidenceItems = append(evidence.EvidenceItems, item)
		byKey[item.Key] = true
	}
	return evidence
}

func issueScanStageEvidenceItemsFromRoleOutputs(draft issueScanLifecycleStageTaskDraft, roleOutputs []IssueScanStageRuntimeRoleOutput) []IssueScanStageRuntimeEvidenceItem {
	required := compactStrings(draft.Stage.RequiredEvidence)
	if len(required) == 0 || len(roleOutputs) == 0 {
		return nil
	}
	outputsByKey := map[string][]IssueScanStageRuntimeEvidenceItem{}
	refsByKey := map[string][]string{}
	for _, roleOutput := range roleOutputs {
		for _, output := range roleOutput.Outputs {
			key := strings.TrimSpace(output.Key)
			if key == "" {
				continue
			}
			output.Key = key
			output.Summary = strings.TrimSpace(output.Summary)
			output.EvidenceRefs = compactStrings(append(output.EvidenceRefs, roleOutput.EvidenceRefs...))
			outputsByKey[key] = append(outputsByKey[key], output)
			refsByKey[key] = append(refsByKey[key], output.EvidenceRefs...)
		}
	}
	out := make([]IssueScanStageRuntimeEvidenceItem, 0, len(required))
	for _, key := range required {
		outputs := outputsByKey[key]
		if len(outputs) == 0 {
			continue
		}
		summaries := make([]string, 0, len(outputs))
		for _, output := range outputs {
			if output.Summary != "" {
				summaries = append(summaries, output.Summary)
			}
		}
		summary := strings.TrimSpace(strings.Join(compactStrings(summaries), " | "))
		if summary == "" {
			summary = "derived from recorded role output"
		}
		out = append(out, IssueScanStageRuntimeEvidenceItem{
			Key:          key,
			Summary:      summary,
			EvidenceRefs: compactStrings(refsByKey[key]),
		})
	}
	return out
}

func (r *Runtime) issueScanStageRecordedRuntimeRoleOutputs(taskID types.EventID, content FactoryRunRequestedContent, orderID string, draft issueScanLifecycleStageTaskDraft) ([]IssueScanStageRuntimeRoleOutput, []string, error) {
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return nil, nil, fmt.Errorf("list issue-scan stage role output artifacts: %w", err)
	}
	creatorRoles, err := r.issueScanRoleOutputCreatorRoles()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve issue-scan role output creator roles: %w", err)
	}
	byRole := map[string]IssueScanStageRuntimeRoleOutput{}
	refsByRole := map[string][]string{}
	for _, artifact := range artifacts {
		parsed, ok, err := issueScanStageRoleOutputArtifact(artifact.ID.Value(), artifact.Label, artifact.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("parse issue-scan role output artifact %s: %w", artifact.ID.Value(), err)
		}
		if !ok {
			continue
		}
		normalized, _, err := normalizeIssueScanStageRoleOutput(content, orderID, draft, parsed)
		if err != nil {
			return nil, nil, fmt.Errorf("validate issue-scan role output artifact %s: %w", artifact.ID.Value(), err)
		}
		role := strings.TrimSpace(normalized.Role)
		if !issueScanStageRoleOutputCreatorAccepted(artifact.CreatedBy, role, creatorRoles) {
			continue
		}
		byRole[role] = IssueScanStageRuntimeRoleOutput{
			Role:         role,
			Summary:      strings.TrimSpace(normalized.Summary),
			EvidenceRefs: compactStrings(append([]string{artifact.ID.Value()}, normalized.EvidenceRefs...)),
			Outputs:      append([]IssueScanStageRuntimeEvidenceItem(nil), normalized.Outputs...),
		}
		refsByRole[role] = compactStrings(append([]string{artifact.ID.Value()}, normalized.SourceRefs...))
	}
	roles := make([]string, 0, len(byRole))
	for role := range byRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	out := make([]IssueScanStageRuntimeRoleOutput, 0, len(roles))
	refs := []string{}
	for _, role := range roles {
		out = append(out, byRole[role])
		refs = append(refs, refsByRole[role]...)
	}
	return out, compactStrings(refs), nil
}

func issueScanStageRoleOutputCreatorAccepted(createdBy types.ActorID, claimedRole string, creatorRoles map[string]string) bool {
	claimedRole = strings.TrimSpace(claimedRole)
	// Preserve existing human/system artifact paths: role binding is enforced
	// only when CreatedBy resolves to a known agent identity role.
	if createdBy == (types.ActorID{}) || claimedRole == "" {
		return true
	}
	creatorRole, ok := creatorRoles[createdBy.Value()]
	if !ok || strings.TrimSpace(creatorRole) == "" {
		return true
	}
	return strings.TrimSpace(creatorRole) == claimedRole
}

func (r *Runtime) issueScanRoleOutputCreatorRoles() (map[string]string, error) {
	roles := map[string]string{}
	if r == nil || r.store == nil {
		return roles, nil
	}
	setRole := func(actorID string, role string) {
		actorID = strings.TrimSpace(actorID)
		role = strings.TrimSpace(role)
		if actorID != "" && role != "" {
			roles[actorID] = role
		}
	}

	// Precedence is explicit: low-level EventGraph identity is the fallback,
	// runtime spawn can refine it, and Hive's registered identity from
	// hive.runtime.spawnAgent is canonical for managed runtime agents.
	identityEvents, err := eventsByTypePaginated(r.store, event.EventTypeAgentIdentityCreated, defaultOperatorProjectionLimit)
	if err != nil {
		return nil, err
	}
	for _, ev := range identityEvents {
		content, ok := ev.Content().(event.AgentIdentityCreatedContent)
		if ok {
			setRole(content.AgentID.Value(), content.AgentType)
		}
	}
	spawnedEvents, err := eventsByTypePaginated(r.store, EventTypeAgentSpawned, defaultOperatorProjectionLimit)
	if err != nil {
		return nil, err
	}
	for _, ev := range spawnedEvents {
		content, ok := ev.Content().(AgentSpawnedContent)
		if ok {
			setRole(content.ActorID, content.Role)
		}
	}
	registeredEvents, err := eventsByTypePaginated(r.store, EventTypeAgentIdentityRegistered, defaultOperatorProjectionLimit)
	if err != nil {
		return nil, err
	}
	for _, ev := range registeredEvents {
		content, ok := ev.Content().(AgentIdentityRegisteredContent)
		if ok {
			setRole(content.ActorID.Value(), content.Role)
		}
	}
	return roles, nil
}

func issueScanStageRoleOutputArtifact(eventRef, label, body string) (IssueScanStageRoleOutputEvidence, bool, error) {
	if strings.TrimSpace(label) != IssueScanStageRoleOutputArtifactLabel {
		return IssueScanStageRoleOutputEvidence{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return IssueScanStageRoleOutputEvidence{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload IssueScanStageRoleOutputEvidence
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return IssueScanStageRoleOutputEvidence{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageRoleOutputArtifactKind {
		return IssueScanStageRoleOutputEvidence{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageRoleOutputArtifactKind)
	}
	payload.SourceRefs = compactStrings(append([]string{strings.TrimSpace(eventRef)}, payload.SourceRefs...))
	return payload, true, nil
}
