package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	defaultRunLaunchDispatchLimit    = 100
	defaultRunLaunchDispatchInterval = 15 * time.Second

	IssueScanExecutionPlanArtifactLabel       = "issue_scan_execution_plan"
	IssueScanLifecycleStageArtifactPrefix     = "issue_scan_lifecycle_stage_"
	IssueScanStageRoleContractArtifactLabel   = "issue_scan_stage_role_contract"
	IssueScanStageOutputContractArtifactLabel = "issue_scan_stage_output_contract"
	issueScanExecutionPlanArtifactMediaType   = "application/json"
	issueScanStageRoleContractArtifactKind    = "issue_scan_stage_role_contract"
	issueScanStageOutputContractArtifactKind  = "issue_scan_stage_output_contract"
)

type issueScanDispatchArtifact struct {
	Label     string
	MediaType string
	Body      string
}

// RunLaunchDispatchResult summarizes one dispatcher pass over queued
// factory.run.requested events.
type RunLaunchDispatchResult struct {
	Scanned                int
	Dispatched             int
	AlreadyDispatched      int
	SkippedNonQueued       int
	Failed                 int
	DispatchedTaskIDs      []types.EventID
	DispatchedStageTaskIDs []types.EventID
	DispatchedOrderIDs     []string
	AlreadyDispatchedIDs   []string
}

// DispatchQueuedRunLaunches binds queued POST /api/hive/runs requests into the
// Work task path. Model overrides are re-resolved through the Runtime's active
// resolver before the FactoryOrder is seeded, and the later Operate path
// revalidates the same structured override artifact before provider creation.
func (r *Runtime) DispatchQueuedRunLaunches(limit int) (RunLaunchDispatchResult, error) {
	return r.dispatchQueuedRunLaunches(limit, "")
}

// DispatchQueuedRunLaunch binds one queued factory.run.requested event into the
// Work task path. It is intended for operator commands that queue a single run
// and want to dispatch only that run instead of flushing the daemon backlog.
func (r *Runtime) DispatchQueuedRunLaunch(runID string) (RunLaunchDispatchResult, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return RunLaunchDispatchResult{}, fmt.Errorf("run_id is required")
	}
	return r.dispatchQueuedRunLaunches(defaultRunLaunchDispatchLimit, runID)
}

func (r *Runtime) dispatchQueuedRunLaunches(limit int, onlyRunID string) (RunLaunchDispatchResult, error) {
	var result RunLaunchDispatchResult
	if r == nil || r.store == nil || r.tasks == nil {
		return result, nil
	}
	r.runLaunchDispatchMu.Lock()
	defer r.runLaunchDispatchMu.Unlock()

	if limit <= 0 {
		limit = defaultRunLaunchDispatchLimit
	}

	dispatched, err := dispatchedFactoryOrderIDs(r.store)
	if err != nil {
		return result, err
	}
	requests, err := fetchFactoryRunRequestedEvents(r.store, limit)
	if onlyRunID != "" {
		requests, err = fetchFactoryRunRequestedEventByRunID(r.store, onlyRunID)
	}
	if err != nil {
		return result, err
	}

	var errs []error
	matchedRequestedRun := onlyRunID == ""
	for _, request := range requests {
		result.Scanned++
		content, ok := request.Content().(FactoryRunRequestedContent)
		if !ok {
			continue
		}
		if onlyRunID != "" && content.RunID != onlyRunID {
			continue
		}
		matchedRequestedRun = true
		if status := strings.TrimSpace(content.Status); status != "" && !strings.EqualFold(status, "queued") {
			result.SkippedNonQueued++
			continue
		}
		orderID, err := factoryOrderIDForRunLaunch(content.RunID)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}
		if taskID, ok := dispatched[orderID]; ok {
			planArtifactBody, hasPlanArtifact, err := issueScanExecutionPlanArtifactBody(content)
			if err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
				continue
			}
			stageArtifacts, err := issueScanLifecycleStageArtifacts(content)
			if err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
				continue
			}
			if hasPlanArtifact {
				if err := r.ensureIssueScanExecutionPlanArtifact(content, request.ID(), taskID, planArtifactBody); err != nil {
					result.Failed++
					errs = append(errs, fmt.Errorf("run %q: repair issue-scan execution plan artifact: %w", content.RunID, err))
					continue
				}
			}
			if err := r.ensureIssueScanDispatchArtifacts(content, request.ID(), taskID, stageArtifacts); err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: repair issue-scan lifecycle stage artifacts: %w", content.RunID, err))
				continue
			}
			order := factoryOrderFromRunLaunch(content, orderID)
			if _, err := r.ensureIssueScanLifecycleStageTaskDrafts(content, order, request.ID(), taskID, runLaunchConversationID(content.RunID, r.convID)); err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: repair issue-scan lifecycle stage task drafts: %w", content.RunID, err))
				continue
			}
			result.AlreadyDispatched++
			result.AlreadyDispatchedIDs = append(result.AlreadyDispatchedIDs, taskID.Value())
			continue
		}
		if err := r.validateRunLaunchDispatchModelOverrides(content); err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}
		stageArtifacts, err := issueScanLifecycleStageArtifacts(content)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}
		planArtifactBody, hasPlanArtifact, err := issueScanExecutionPlanArtifactBody(content)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}

		convID := runLaunchConversationID(content.RunID, r.convID)
		order := factoryOrderFromRunLaunch(content, orderID)
		task, err := work.SeedFactoryOrder(r.tasks, r.humanID, order, []types.EventID{request.ID()}, convID)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: seed factory order: %w", content.RunID, err))
			continue
		}
		dispatched[orderID] = task.ID
		if hasPlanArtifact {
			if err := r.ensureIssueScanExecutionPlanArtifact(content, request.ID(), task.ID, planArtifactBody); err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: attach issue-scan execution plan artifact: %w", content.RunID, err))
				continue
			}
		}
		if err := r.ensureIssueScanDispatchArtifacts(content, request.ID(), task.ID, stageArtifacts); err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: attach issue-scan lifecycle stage artifacts: %w", content.RunID, err))
			continue
		}
		stageTaskIDs, err := r.ensureIssueScanLifecycleStageTaskDrafts(content, order, request.ID(), task.ID, convID)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: create issue-scan lifecycle stage task drafts: %w", content.RunID, err))
			continue
		}
		result.Dispatched++
		result.DispatchedTaskIDs = append(result.DispatchedTaskIDs, task.ID)
		result.DispatchedStageTaskIDs = append(result.DispatchedStageTaskIDs, stageTaskIDs...)
		result.DispatchedOrderIDs = append(result.DispatchedOrderIDs, orderID)
	}
	if !matchedRequestedRun {
		errs = append(errs, fmt.Errorf("queued run %q not found", onlyRunID))
	}

	return result, errors.Join(errs...)
}

func (r *Runtime) ensureIssueScanExecutionPlanArtifact(content FactoryRunRequestedContent, requestID, taskID types.EventID, body string) error {
	return r.ensureIssueScanDispatchArtifact(content, requestID, taskID, issueScanDispatchArtifact{
		Label:     IssueScanExecutionPlanArtifactLabel,
		MediaType: issueScanExecutionPlanArtifactMediaType,
		Body:      body,
	})
}

func (r *Runtime) ensureIssueScanDispatchArtifacts(content FactoryRunRequestedContent, requestID, taskID types.EventID, artifacts []issueScanDispatchArtifact) error {
	for _, artifact := range artifacts {
		if err := r.ensureIssueScanDispatchArtifact(content, requestID, taskID, artifact); err != nil {
			return fmt.Errorf("%s: %w", artifact.Label, err)
		}
	}
	return nil
}

func (r *Runtime) ensureIssueScanDispatchArtifact(content FactoryRunRequestedContent, requestID, taskID types.EventID, artifact issueScanDispatchArtifact) error {
	label := strings.TrimSpace(artifact.Label)
	if label == "" {
		return fmt.Errorf("artifact label is required")
	}
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return fmt.Errorf("list task artifacts: %w", err)
	}
	for _, existing := range artifacts {
		if existing.Label == label {
			return nil
		}
	}
	return r.tasks.AddArtifact(
		r.humanID,
		taskID,
		label,
		artifact.MediaType,
		artifact.Body,
		[]types.EventID{requestID, taskID},
		runLaunchConversationID(content.RunID, r.convID),
	)
}

func IssueScanLifecycleStageArtifactLabel(stageID string) string {
	return IssueScanLifecycleStageArtifactPrefix + safeRunLaunchID(stageID)
}

func issueScanExecutionPlanArtifactBody(content FactoryRunRequestedContent) (string, bool, error) {
	raw := bytes.TrimSpace(content.Brief)
	if len(raw) == 0 {
		return "", false, nil
	}
	if raw[0] != '{' {
		return "", false, nil
	}
	var meta struct {
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", false, fmt.Errorf("decode run launch brief for execution plan artifact: %w", err)
	}
	if len(meta.Kind) == 0 {
		return "", false, nil
	}
	var kind string
	if err := json.Unmarshal(meta.Kind, &kind); err != nil {
		return "", false, nil
	}
	if strings.TrimSpace(kind) != issueScanBriefKind {
		return "", false, nil
	}
	if _, _, lifecycle, agentPlan, err := queuedRunLifecycleFromBrief(raw); err != nil {
		return "", false, fmt.Errorf("validate issue-scan execution plan artifact: %w", err)
	} else if len(lifecycle) == 0 && len(agentPlan) == 0 {
		return "", false, nil
	}
	var encoded bytes.Buffer
	if err := json.Indent(&encoded, raw, "", "  "); err != nil {
		return "", false, fmt.Errorf("format issue-scan execution plan artifact: %w", err)
	}
	return encoded.String(), true, nil
}

func issueScanLifecycleStageArtifacts(content FactoryRunRequestedContent) ([]issueScanDispatchArtifact, error) {
	raw := bytes.TrimSpace(content.Brief)
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] != '{' {
		return nil, nil
	}
	var meta struct {
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("decode run launch brief for lifecycle stage artifacts: %w", err)
	}
	if len(meta.Kind) == 0 {
		return nil, nil
	}
	var kind string
	if err := json.Unmarshal(meta.Kind, &kind); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(kind) != issueScanBriefKind {
		return nil, nil
	}
	briefKind, lifecycleVersion, lifecycle, agentPlan, err := queuedRunLifecycleFromBrief(raw)
	if err != nil {
		return nil, fmt.Errorf("derive issue-scan lifecycle stage artifacts: %w", err)
	}
	if briefKind != issueScanBriefKind || len(lifecycle) == 0 {
		return nil, nil
	}
	return issueScanLifecycleStageArtifactsFromLifecycle(content.RunID, lifecycleVersion, lifecycle, agentPlan)
}

func issueScanLifecycleStageArtifactsFromLifecycle(runID, lifecycleVersion string, lifecycle []OperatorQueuedRunLifecycleStage, agentPlan []OperatorQueuedRunAgentPlanStep) ([]issueScanDispatchArtifact, error) {
	planByStage := map[string][]OperatorQueuedRunAgentPlanStep{}
	for _, step := range agentPlan {
		planByStage[step.StageID] = append(planByStage[step.StageID], step)
	}
	out := make([]issueScanDispatchArtifact, 0, len(lifecycle))
	labelsByStageID := map[string]string{}
	for i, stage := range lifecycle {
		label := IssueScanLifecycleStageArtifactLabel(stage.ID)
		if label == IssueScanLifecycleStageArtifactPrefix {
			return nil, fmt.Errorf("stage[%d] id %q does not produce a usable artifact label", i, stage.ID)
		}
		if existingStageID, ok := labelsByStageID[label]; ok {
			return nil, fmt.Errorf("stage[%d] id %q collides with stage id %q for artifact label %q", i, stage.ID, existingStageID, label)
		}
		labelsByStageID[label] = stage.ID
		body := struct {
			Kind               string                           `json:"kind"`
			LifecycleVersion   string                           `json:"lifecycle_version"`
			RunID              string                           `json:"run_id"`
			StageIndex         int                              `json:"stage_index"`
			StageCount         int                              `json:"stage_count"`
			Stage              OperatorQueuedRunLifecycleStage  `json:"stage"`
			AgentExecutionPlan []OperatorQueuedRunAgentPlanStep `json:"agent_execution_plan,omitempty"`
			EvidenceKind       string                           `json:"evidence_kind"`
			EvidenceStatus     string                           `json:"evidence_status"`
		}{
			Kind:               "issue_scan_lifecycle_stage",
			LifecycleVersion:   lifecycleVersion,
			RunID:              runID,
			StageIndex:         i + 1,
			StageCount:         len(lifecycle),
			Stage:              stage,
			AgentExecutionPlan: append([]OperatorQueuedRunAgentPlanStep(nil), planByStage[stage.ID]...),
			EvidenceKind:       "stage_declaration_not_completion",
			EvidenceStatus:     "pending_runtime_evidence",
		}
		encoded, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal issue-scan lifecycle stage artifact %q: %w", stage.ID, err)
		}
		out = append(out, issueScanDispatchArtifact{
			Label:     label,
			MediaType: issueScanExecutionPlanArtifactMediaType,
			Body:      string(encoded),
		})
	}
	return out, nil
}

func (r *Runtime) ensureIssueScanLifecycleStageTaskDrafts(content FactoryRunRequestedContent, order work.FactoryOrder, requestID, parentTaskID types.EventID, convID types.ConversationID) ([]types.EventID, error) {
	if r == nil || r.tasks == nil {
		return nil, nil
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, err
	}
	out := make([]types.EventID, 0, len(drafts))
	var previous types.EventID
	for _, draft := range drafts {
		stageTaskID, err := r.ensureIssueScanLifecycleStageTaskDraft(draft, requestID, parentTaskID, convID)
		if err != nil {
			return out, err
		}
		if err := r.ensureIssueScanStageTaskDependency(stageTaskID, parentTaskID, requestID, parentTaskID, convID); err != nil {
			return out, fmt.Errorf("link stage task %q to parent task: %w", draft.StageID, err)
		}
		if err := r.attachIssueScanLifecycleStageTaskReadinessGates(content, order, draft, requestID, parentTaskID, stageTaskID, convID); err != nil {
			return out, fmt.Errorf("attach stage task %q readiness gates: %w", draft.StageID, err)
		}
		if err := r.attachIssueScanLifecycleStageTaskRoleContract(content, order, draft, requestID, parentTaskID, stageTaskID, convID); err != nil {
			return out, fmt.Errorf("attach stage task %q role contract: %w", draft.StageID, err)
		}
		if err := r.attachIssueScanLifecycleStageTaskOutputContract(content, order, draft, requestID, parentTaskID, stageTaskID, convID); err != nil {
			return out, fmt.Errorf("attach stage task %q output contract: %w", draft.StageID, err)
		}
		if previous != (types.EventID{}) {
			if err := r.ensureIssueScanStageTaskDependency(stageTaskID, previous, requestID, parentTaskID, convID); err != nil {
				return out, fmt.Errorf("link stage task %q after previous stage: %w", draft.StageID, err)
			}
		}
		previous = stageTaskID
		out = append(out, stageTaskID)
	}
	return out, nil
}

func (r *Runtime) ensureIssueScanLifecycleStageTaskDraft(draft issueScanLifecycleStageTaskDraft, requestID, parentTaskID types.EventID, convID types.ConversationID) (types.EventID, error) {
	canonicalTaskID := strings.TrimSpace(draft.Options.CanonicalTaskID)
	if canonicalTaskID == "" {
		return types.EventID{}, fmt.Errorf("stage task %q canonical task id is required", draft.StageID)
	}
	taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, canonicalTaskID)
	if err != nil {
		return types.EventID{}, fmt.Errorf("find stage task %q by canonical id: %w", draft.StageID, err)
	}
	if exists {
		if strings.TrimSpace(factoryOrderID) != strings.TrimSpace(draft.Options.FactoryOrderID) {
			return types.EventID{}, fmt.Errorf("stage task %q canonical id %q belongs to factory order %q, want %q", draft.StageID, canonicalTaskID, factoryOrderID, draft.Options.FactoryOrderID)
		}
		return taskID, nil
	}
	stageTask, err := r.tasks.CreateV39(r.humanID, draft.Options, []types.EventID{requestID, parentTaskID}, convID)
	if err != nil {
		return types.EventID{}, fmt.Errorf("create stage task %q: %w", draft.StageID, err)
	}
	return stageTask.ID, nil
}

func (r *Runtime) ensureIssueScanStageTaskDependency(taskID, dependsOnID, requestID, parentTaskID types.EventID, convID types.ConversationID) error {
	if taskID == (types.EventID{}) || dependsOnID == (types.EventID{}) {
		return nil
	}
	if taskID == dependsOnID {
		return nil
	}
	deps, err := r.tasks.GetDependencies(taskID)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		if dep == dependsOnID {
			return nil
		}
	}
	return r.tasks.AddDependency(r.humanID, taskID, dependsOnID, compactEventIDs([]types.EventID{requestID, parentTaskID, dependsOnID, taskID}), convID)
}

func workTaskByCanonicalTaskID(s store.Store, canonicalTaskID string) (types.EventID, string, bool, error) {
	canonicalTaskID = strings.TrimSpace(canonicalTaskID)
	if s == nil || canonicalTaskID == "" {
		return types.EventID{}, "", false, nil
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(work.EventTypeTaskCreated, 100, cursor)
		if err != nil {
			return types.EventID{}, "", false, fmt.Errorf("fetch work.task.created events: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(work.TaskCreatedContent)
			if !ok {
				continue
			}
			if strings.TrimSpace(content.CanonicalTaskID) == canonicalTaskID {
				return ev.ID(), content.FactoryOrderID, true, nil
			}
		}
		if !page.HasMore() {
			return types.EventID{}, "", false, nil
		}
		cursor = page.Cursor()
	}
}

type issueScanLifecycleStageTaskDraft struct {
	StageID            string
	Stage              OperatorQueuedRunLifecycleStage
	AgentExecutionPlan []OperatorQueuedRunAgentPlanStep
	StageIndex         int
	StageCount         int
	Options            work.TaskCreateOptions
}

func issueScanLifecycleStageTaskDrafts(content FactoryRunRequestedContent, order work.FactoryOrder) ([]issueScanLifecycleStageTaskDraft, error) {
	raw := bytes.TrimSpace(content.Brief)
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] != '{' {
		return nil, nil
	}
	var meta struct {
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("decode run launch brief for lifecycle stage task drafts: %w", err)
	}
	if len(meta.Kind) == 0 {
		return nil, nil
	}
	var kind string
	if err := json.Unmarshal(meta.Kind, &kind); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(kind) != issueScanBriefKind {
		return nil, nil
	}
	briefKind, _, lifecycle, agentPlan, err := queuedRunLifecycleFromBrief(raw)
	if err != nil {
		return nil, fmt.Errorf("derive issue-scan lifecycle stage task drafts: %w", err)
	}
	if briefKind != issueScanBriefKind || len(lifecycle) == 0 {
		return nil, nil
	}
	return issueScanLifecycleStageTaskDraftsFromLifecycle(content, order, lifecycle, agentPlan)
}

func issueScanLifecycleStageTaskDraftsFromLifecycle(content FactoryRunRequestedContent, order work.FactoryOrder, lifecycle []OperatorQueuedRunLifecycleStage, agentPlan []OperatorQueuedRunAgentPlanStep) ([]issueScanLifecycleStageTaskDraft, error) {
	planByStage := map[string][]OperatorQueuedRunAgentPlanStep{}
	for _, step := range agentPlan {
		planByStage[step.StageID] = append(planByStage[step.StageID], step)
	}
	requirementIDs := factoryOrderRequirementIDs(order)
	acceptanceCriterionIDs := factoryOrderAcceptanceCriterionIDs(order)
	out := make([]issueScanLifecycleStageTaskDraft, 0, len(lifecycle))
	canonicalTaskIDsByStageID := map[string]string{}
	for i, stage := range lifecycle {
		stageID := safeRunLaunchID(stage.ID)
		if stageID == "" {
			return nil, fmt.Errorf("stage %d has empty id", i+1)
		}
		canonicalTaskID := issueScanLifecycleStageTaskCanonicalID(order.ID, stage.ID)
		if canonicalTaskID == "" {
			return nil, fmt.Errorf("stage %d has empty canonical task id", i+1)
		}
		if existingStageID, ok := canonicalTaskIDsByStageID[canonicalTaskID]; ok {
			return nil, fmt.Errorf("stage %d id %q collides with stage id %q for canonical task id %q", i+1, stage.ID, existingStageID, canonicalTaskID)
		}
		canonicalTaskIDsByStageID[canonicalTaskID] = stage.ID
		steps := append([]OperatorQueuedRunAgentPlanStep(nil), planByStage[stage.ID]...)
		riskClass := valueOr(order.RiskClass, "high")
		out = append(out, issueScanLifecycleStageTaskDraft{
			StageID:            stageID,
			Stage:              stage,
			AgentExecutionPlan: steps,
			StageIndex:         i + 1,
			StageCount:         len(lifecycle),
			Options: work.TaskCreateOptions{
				Title:                  issueScanLifecycleStageTaskTitle(stage),
				Description:            issueScanLifecycleStageTaskDescription(content, stage, steps, i+1, len(lifecycle)),
				CanonicalTaskID:        canonicalTaskID,
				FactoryOrderID:         order.ID,
				RequirementIDs:         requirementIDs,
				AcceptanceCriterionIDs: acceptanceCriterionIDs,
				Cell:                   issueScanLifecycleStageTaskCell(stage),
				RiskClass:              riskClass,
				ExpectedOutputs:        issueScanLifecycleStageTaskExpectedOutputs(stage, steps),
				Priority:               issueScanLifecycleStageTaskPriority(riskClass),
			},
		})
	}
	return out, nil
}

func (r *Runtime) attachIssueScanLifecycleStageTaskReadinessGates(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft, requestID, parentTaskID, stageTaskID types.EventID, convID types.ConversationID) error {
	if r == nil || r.tasks == nil {
		return nil
	}
	causes := []types.EventID{requestID, parentTaskID, stageTaskID}
	existingArtifacts, err := r.tasks.ListArtifacts(stageTaskID)
	if err != nil {
		return fmt.Errorf("list stage task %q artifacts: %w", draft.StageID, err)
	}
	existingLabels := map[string]bool{}
	for _, artifact := range existingArtifacts {
		existingLabels[strings.TrimSpace(artifact.Label)] = true
	}
	gates := []struct{ label, body string }{
		{work.GateDefinitionOfDone, issueScanLifecycleStageTaskDefinitionOfDone(content, order, draft)},
		{work.GateAcceptanceCriteria, issueScanLifecycleStageTaskAcceptanceCriteria(content, order, draft)},
		{work.GateTestPlan, issueScanLifecycleStageTaskTestPlan(content, order, draft)},
	}
	for _, gate := range gates {
		if strings.TrimSpace(gate.body) == "" {
			return fmt.Errorf("stage task %q readiness gate %q body is empty", draft.StageID, gate.label)
		}
		if existingLabels[strings.TrimSpace(gate.label)] {
			continue
		}
		if err := r.tasks.AddArtifact(r.humanID, stageTaskID, gate.label, "text/markdown", gate.body, causes, convID); err != nil {
			return err
		}
		existingLabels[strings.TrimSpace(gate.label)] = true
	}
	return nil
}

func (r *Runtime) attachIssueScanLifecycleStageTaskRoleContract(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft, requestID, parentTaskID, stageTaskID types.EventID, convID types.ConversationID) error {
	if r == nil || r.tasks == nil {
		return nil
	}
	body, err := issueScanLifecycleStageTaskRoleContractBody(content, order, draft)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("stage task %q role contract body is empty", draft.StageID)
	}
	existingArtifacts, err := r.tasks.ListArtifacts(stageTaskID)
	if err != nil {
		return fmt.Errorf("list stage task %q artifacts: %w", draft.StageID, err)
	}
	for _, artifact := range existingArtifacts {
		if strings.TrimSpace(artifact.Label) == IssueScanStageRoleContractArtifactLabel {
			return nil
		}
	}
	causes := []types.EventID{requestID, parentTaskID, stageTaskID}
	return r.tasks.AddArtifact(r.humanID, stageTaskID, IssueScanStageRoleContractArtifactLabel, issueScanExecutionPlanArtifactMediaType, body, causes, convID)
}

func (r *Runtime) attachIssueScanLifecycleStageTaskOutputContract(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft, requestID, parentTaskID, stageTaskID types.EventID, convID types.ConversationID) error {
	if r == nil || r.tasks == nil {
		return nil
	}
	body, err := issueScanLifecycleStageTaskOutputContractBody(content, order, draft)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("stage task %q output contract body is empty", draft.StageID)
	}
	existingArtifacts, err := r.tasks.ListArtifacts(stageTaskID)
	if err != nil {
		return fmt.Errorf("list stage task %q artifacts: %w", draft.StageID, err)
	}
	for _, artifact := range existingArtifacts {
		if strings.TrimSpace(artifact.Label) == IssueScanStageOutputContractArtifactLabel {
			return nil
		}
	}
	causes := []types.EventID{requestID, parentTaskID, stageTaskID}
	return r.tasks.AddArtifact(r.humanID, stageTaskID, IssueScanStageOutputContractArtifactLabel, issueScanExecutionPlanArtifactMediaType, body, causes, convID)
}

func issueScanLifecycleStageTaskTitle(stage OperatorQueuedRunLifecycleStage) string {
	name := strings.TrimSpace(stage.Name)
	if name == "" {
		name = strings.TrimSpace(stage.ID)
	}
	return strings.TrimSpace("Issue-scan stage: " + name)
}

func issueScanLifecycleStageTaskDescription(content FactoryRunRequestedContent, stage OperatorQueuedRunLifecycleStage, steps []OperatorQueuedRunAgentPlanStep, index, count int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Issue-scan lifecycle stage %d/%d for queued run %s.\n\n", index, count, strings.TrimSpace(content.RunID))
	fmt.Fprintf(&b, "Stage ID: %s\n", strings.TrimSpace(stage.ID))
	fmt.Fprintf(&b, "Authority boundary: %s\n", valueOr(stage.AuthorityBoundary, "not projected"))
	fmt.Fprintf(&b, "Completion gate: %s\n", valueOr(stage.CompletionGate, "not projected"))
	fmt.Fprintf(&b, "Required roles: %s\n", strings.Join(stage.RequiredRoles, ", "))
	fmt.Fprintf(&b, "Required evidence: %s\n\n", strings.Join(stage.RequiredEvidence, ", "))
	if len(steps) > 0 {
		b.WriteString("Agent execution plan:\n")
		for _, step := range steps {
			mode := "review"
			if step.CanOperate {
				mode = "operate"
			}
			fmt.Fprintf(&b, "- %s (%s): %s", strings.TrimSpace(step.Role), mode, strings.TrimSpace(step.Objective))
			if len(step.RequiredOutputs) > 0 {
				fmt.Fprintf(&b, " Outputs: %s.", strings.Join(step.RequiredOutputs, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Status boundary: this task draft declares expected governed work for the stage. It is not runtime progress, blocker resolution, PR readiness, approval, merge, or deploy evidence.")
	return strings.TrimSpace(b.String())
}

func issueScanLifecycleStageTaskDefinitionOfDone(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft) string {
	stage := draft.Stage
	var b strings.Builder
	fmt.Fprintf(&b, "Stage %d/%d `%s` for queued run `%s` completes only when its required evidence is recorded for FactoryOrder `%s`.\n\n", draft.StageIndex, draft.StageCount, strings.TrimSpace(stage.ID), strings.TrimSpace(content.RunID), strings.TrimSpace(order.ID))
	fmt.Fprintf(&b, "Completion gate: %s\n", valueOr(stage.CompletionGate, "not projected"))
	fmt.Fprintf(&b, "Authority boundary: %s\n", valueOr(stage.AuthorityBoundary, "not projected"))
	if len(stage.RequiredRoles) > 0 {
		fmt.Fprintf(&b, "Required roles: %s\n", strings.Join(compactStrings(stage.RequiredRoles), ", "))
	}
	if len(stage.RequiredEvidence) > 0 {
		b.WriteString("\nRequired evidence:\n")
		for _, item := range compactStrings(stage.RequiredEvidence) {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	b.WriteString("\nCompletion of this task is not merge, deploy, Human approval, or proof that later lifecycle stages completed.")
	return strings.TrimSpace(b.String())
}

func issueScanLifecycleStageTaskAcceptanceCriteria(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft) string {
	stage := draft.Stage
	var b strings.Builder
	fmt.Fprintf(&b, "- Evidence is tied to queued run `%s`, FactoryOrder `%s`, and lifecycle stage `%s`.\n", strings.TrimSpace(content.RunID), strings.TrimSpace(order.ID), strings.TrimSpace(stage.ID))
	fmt.Fprintf(&b, "- The stage authority boundary `%s` is obeyed.\n", valueOr(stage.AuthorityBoundary, "not projected"))
	fmt.Fprintf(&b, "- The completion gate `%s` is satisfied by durable evidence refs, not prose-only assertion.\n", valueOr(stage.CompletionGate, "not projected"))
	if len(stage.RequiredEvidence) > 0 {
		fmt.Fprintf(&b, "- Required evidence is present: %s.\n", strings.Join(compactStrings(stage.RequiredEvidence), ", "))
	}
	if len(draft.AgentExecutionPlan) > 0 {
		b.WriteString("- Required role outputs are accounted for:")
		for _, step := range draft.AgentExecutionPlan {
			outputs := compactStrings(step.RequiredOutputs)
			if len(outputs) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n  - %s: %s", strings.TrimSpace(step.Role), strings.Join(outputs, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("- Any blocker is recorded as blocked evidence instead of silently completing the task.")
	return strings.TrimSpace(b.String())
}

func issueScanLifecycleStageTaskTestPlan(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft) string {
	stage := draft.Stage
	var b strings.Builder
	fmt.Fprintf(&b, "1. Inspect Work artifacts/source refs for FactoryOrder `%s` and stage `%s`.\n", strings.TrimSpace(order.ID), strings.TrimSpace(stage.ID))
	fmt.Fprintf(&b, "2. Confirm the queued run `%s` remains bounded by `%s`.\n", strings.TrimSpace(content.RunID), valueOr(stage.AuthorityBoundary, "not projected"))
	fmt.Fprintf(&b, "3. Verify required evidence refs cover: %s.\n", strings.Join(compactStrings(stage.RequiredEvidence), ", "))
	if len(draft.AgentExecutionPlan) > 0 {
		b.WriteString("4. Confirm each declared role output is present before marking the stage task complete:")
		for _, step := range draft.AgentExecutionPlan {
			fmt.Fprintf(&b, "\n   - %s: %s", strings.TrimSpace(step.Role), strings.Join(compactStrings(step.RequiredOutputs), ", "))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("4. Confirm the stage has no required role outputs before marking it complete.\n")
	}
	b.WriteString("5. Confirm no merge, deploy, or Human-approval claim is made unless this stage explicitly requires and records that evidence.")
	return strings.TrimSpace(b.String())
}

func issueScanLifecycleStageTaskRoleContractBody(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft) (string, error) {
	stage := draft.Stage
	payload := struct {
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
	}{
		Kind:               issueScanStageRoleContractArtifactKind,
		LifecycleVersion:   issueScanLifecycleVersion,
		RunID:              strings.TrimSpace(content.RunID),
		FactoryOrderID:     strings.TrimSpace(order.ID),
		StageID:            strings.TrimSpace(stage.ID),
		StageIndex:         draft.StageIndex,
		StageCount:         draft.StageCount,
		Stage:              stage,
		AgentExecutionPlan: append([]OperatorQueuedRunAgentPlanStep(nil), draft.AgentExecutionPlan...),
		EvidenceKind:       "required_role_contract_not_runtime_execution",
		EvidenceStatus:     "pending_runtime_evidence",
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan stage role contract %q: %w", draft.StageID, err)
	}
	return string(encoded), nil
}

func issueScanLifecycleStageTaskOutputContractBody(content FactoryRunRequestedContent, order work.FactoryOrder, draft issueScanLifecycleStageTaskDraft) (string, error) {
	stage := draft.Stage
	payload := struct {
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
	}{
		Kind:                issueScanStageOutputContractArtifactKind,
		LifecycleVersion:    issueScanLifecycleVersion,
		RunID:               strings.TrimSpace(content.RunID),
		FactoryOrderID:      strings.TrimSpace(order.ID),
		StageID:             strings.TrimSpace(stage.ID),
		StageIndex:          draft.StageIndex,
		StageCount:          draft.StageCount,
		Stage:               stage,
		RequiredEvidence:    compactStrings(stage.RequiredEvidence),
		ExpectedOutputs:     issueScanLifecycleStageTaskExpectedOutputs(stage, draft.AgentExecutionPlan),
		RoleOutputContracts: issueScanStageRoleOutputContracts(stage, draft.AgentExecutionPlan),
		EvidenceKind:        "required_output_contract_not_runtime_execution",
		EvidenceStatus:      "pending_runtime_evidence",
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan stage output contract %q: %w", draft.StageID, err)
	}
	return string(encoded), nil
}

func issueScanStageRoleOutputContracts(stage OperatorQueuedRunLifecycleStage, steps []OperatorQueuedRunAgentPlanStep) []CivilizationAssemblyRoleOutputContract {
	out := make([]CivilizationAssemblyRoleOutputContract, 0, len(steps))
	for _, step := range steps {
		out = append(out, CivilizationAssemblyRoleOutputContract{
			Role:              strings.TrimSpace(step.Role),
			CanOperate:        step.CanOperate,
			RequiredOutputs:   compactStrings(step.RequiredOutputs),
			AuthorityBoundary: valueOr(step.AuthorityBoundary, stage.AuthorityBoundary),
			CompletionGate:    valueOr(step.CompletionGate, stage.CompletionGate),
			EvidenceStatus:    "required_not_observed",
		})
	}
	return compactCivilizationAssemblyRoleOutputContracts(out)
}

func issueScanLifecycleStageTaskCanonicalID(orderID, stageID string) string {
	suffix := factoryOrderIDSuffix(orderID)
	stageID = safeRunLaunchID(stageID)
	if suffix == "" || stageID == "" {
		return ""
	}
	return "tsk_" + suffix + "_" + stageID
}

func issueScanLifecycleStageTaskCell(stage OperatorQueuedRunLifecycleStage) string {
	switch safeRunLaunchID(stage.ID) {
	case "research_issue_and_repo_context", "debate_with_correct_civic_roles", "select_and_design_approach":
		return "planning"
	case "run_adversarial_review":
		return "review"
	case "surface_ready_for_human_result_pr":
		return "governance"
	default:
		return "implementation"
	}
}

func issueScanLifecycleStageTaskExpectedOutputs(stage OperatorQueuedRunLifecycleStage, steps []OperatorQueuedRunAgentPlanStep) []string {
	out := []string{"stage declaration artifact remains pending runtime evidence"}
	out = append(out, stage.RequiredEvidence...)
	for _, step := range steps {
		out = append(out, step.RequiredOutputs...)
	}
	return compactStrings(out)
}

func issueScanLifecycleStageTaskPriority(riskClass string) work.TaskPriority {
	switch strings.TrimSpace(riskClass) {
	case "critical":
		return work.PriorityCritical
	case "high":
		return work.PriorityHigh
	case "low":
		return work.PriorityLow
	default:
		return work.PriorityMedium
	}
}

func factoryOrderRequirementIDs(order work.FactoryOrder) []string {
	if len(order.RequirementIDs) > 0 {
		return compactStrings(order.RequirementIDs)
	}
	return []string{"req_" + factoryOrderIDSuffix(order.ID)}
}

func factoryOrderAcceptanceCriterionIDs(order work.FactoryOrder) []string {
	if len(order.AcceptanceCriterionIDs) > 0 {
		return compactStrings(order.AcceptanceCriterionIDs)
	}
	return []string{"ac_" + factoryOrderIDSuffix(order.ID)}
}

func factoryOrderIDSuffix(orderID string) string {
	orderID = strings.TrimSpace(orderID)
	if suffix, ok := strings.CutPrefix(orderID, "fo_"); ok {
		return safeRunLaunchID(suffix)
	}
	return safeRunLaunchID(orderID)
}

func compactEventIDs(values []types.EventID) []types.EventID {
	if len(values) == 0 {
		return nil
	}
	out := make([]types.EventID, 0, len(values))
	seen := make(map[types.EventID]bool, len(values))
	for _, value := range values {
		if value == (types.EventID{}) || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (r *Runtime) runRunLaunchDispatchLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := r.DispatchQueuedRunLaunches(defaultRunLaunchDispatchLimit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: run-launch dispatch failed closed: %v\n", err)
			}
			if result.Dispatched > 0 {
				fmt.Fprintf(os.Stderr, "Run-launch dispatcher: seeded %d FactoryOrder task(s)\n", result.Dispatched)
			}
		}
	}
}

func effectiveRunLaunchDispatchInterval(configured time.Duration) time.Duration {
	if configured < 0 {
		return 0
	}
	if configured == 0 {
		return defaultRunLaunchDispatchInterval
	}
	return configured
}

func fetchFactoryRunRequestedEvents(s store.Store, limit int) ([]event.Event, error) {
	if limit <= 0 {
		limit = defaultRunLaunchDispatchLimit
	}
	var out []event.Event
	cursor := types.None[types.Cursor]()
	for len(out) < limit {
		pageSize := limit - len(out)
		if pageSize > 100 {
			pageSize = 100
		}
		page, err := s.ByType(EventTypeFactoryRunRequested, pageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch factory.run.requested events: %w", err)
		}
		out = append(out, page.Items()...)
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	return out, nil
}

func fetchFactoryRunRequestedEventByRunID(s store.Store, runID string) ([]event.Event, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(EventTypeFactoryRunRequested, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch factory.run.requested events: %w", err)
		}
		for _, item := range page.Items() {
			content, ok := item.Content().(FactoryRunRequestedContent)
			if ok && content.RunID == runID {
				return []event.Event{item}, nil
			}
		}
		if !page.HasMore() {
			return nil, nil
		}
		cursor = page.Cursor()
	}
}

func dispatchedFactoryOrderIDs(s store.Store) (map[string]types.EventID, error) {
	out := map[string]types.EventID{}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(work.EventTypeTaskCreated, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch work.task.created events: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(work.TaskCreatedContent)
			if !ok {
				continue
			}
			orderID := strings.TrimSpace(content.FactoryOrderID)
			if orderID != "" {
				// Work v3.9 requires FactoryOrderID when a stage draft carries a
				// stable CanonicalTaskID. The parent FactoryOrder task seeded by
				// work.SeedFactoryOrder is therefore identified by the empty
				// CanonicalTaskID; stage drafts are one-to-many declaration children
				// and must never become the dispatcher repair target. Current Hive
				// audit: this dispatcher repair map is the only runtime path that
				// resolves a single task from FactoryOrderID; Civilization Assembly
				// projection intentionally aggregates all matching TaskRefs.
				if strings.TrimSpace(content.CanonicalTaskID) == "" {
					out[orderID] = ev.ID()
					continue
				}
				if _, exists := out[orderID]; !exists {
					out[orderID] = ev.ID()
				}
			}
		}
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	return out, nil
}

func (r *Runtime) validateRunLaunchDispatchModelOverrides(content FactoryRunRequestedContent) error {
	if len(content.ModelOverrides) == 0 {
		return nil
	}
	raw := make([]ModelOverrideRequest, 0, len(content.ModelOverrides))
	for _, override := range content.ModelOverrides {
		raw = append(raw, ModelOverrideRequest{
			Role:                 override.Role,
			Model:                override.Model,
			Provider:             override.Provider,
			Profile:              override.Profile,
			AuthMode:             override.RequestedAuthMode,
			PreferredTier:        override.PreferredTier,
			RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
			MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(override.MaxCostPerCallUSD),
		})
	}
	source := modelSelectionSourceWithRolePolicyUpdates(r.store, func() OperatorModelSelectionConfig {
		return OperatorModelSelectionConfig{
			Resolver:      r.currentResolver(),
			CatalogSource: "runtime-dispatcher",
			LoadedAt:      types.Now().Value(),
			ReloadMode:    operatorModelCatalogReloadMode,
			HotReload:     r.catalogReloadInterval > 0,
		}
	}, defaultOperatorProjectionLimit)
	validated, err := ValidateModelOverrides(raw, source)
	if err != nil {
		return fmt.Errorf("model overrides failed dispatch-time validation: %w", err)
	}
	if len(validated) != len(content.ModelOverrides) {
		return fmt.Errorf("model overrides validation count changed from %d to %d", len(content.ModelOverrides), len(validated))
	}
	for i := range validated {
		if err := compareRunLaunchModelOverride(content.ModelOverrides[i], validated[i]); err != nil {
			return fmt.Errorf("model_overrides[%d] for role %q failed dispatch-time validation: %w", i, content.ModelOverrides[i].Role, err)
		}
	}
	return nil
}

func compareRunLaunchModelOverride(stored, current RunLaunchModelOverride) error {
	switch {
	case !sameRole(stored.Role, current.Role):
		return fmt.Errorf("stored role %q but current validation produced %q", stored.Role, current.Role)
	case strings.TrimSpace(stored.ResolvedModel) != strings.TrimSpace(current.ResolvedModel):
		return fmt.Errorf("stored resolved_model %q but current resolver produced %q", stored.ResolvedModel, current.ResolvedModel)
	case strings.TrimSpace(stored.ResolvedProvider) != strings.TrimSpace(current.ResolvedProvider):
		return fmt.Errorf("stored resolved_provider %q but current resolver produced %q", stored.ResolvedProvider, current.ResolvedProvider)
	case strings.TrimSpace(stored.AuthMode) != strings.TrimSpace(current.AuthMode):
		return fmt.Errorf("stored auth_mode %q but current resolver produced %q", stored.AuthMode, current.AuthMode)
	default:
		return nil
	}
}

func factoryOrderFromRunLaunch(content FactoryRunRequestedContent, orderID string) work.FactoryOrder {
	return work.FactoryOrder{
		Kind:               work.OrderSoftwarePR,
		ID:                 orderID,
		Title:              content.Title,
		Intent:             runLaunchIntent(content),
		Cell:               "implementation",
		RiskClass:          runLaunchRiskClass(content.Authority.InitialLevel),
		DefinitionOfDone:   runLaunchDefinitionOfDone(content),
		AcceptanceCriteria: runLaunchAcceptanceCriteria(content),
		TestPlan:           runLaunchTestPlan(content),
		ExpectedOutputs:    runLaunchExpectedOutputs(content),
		ModelOverrides:     workModelOverridesFromRunLaunch(content.ModelOverrides),
	}
}

func runLaunchExpectedOutputs(content FactoryRunRequestedContent) []string {
	if isIssueScanRunLaunch(content) {
		return []string{
			"ready-for-Human result pull request",
			"exact-head adversarial review with zero blockers",
			"validation evidence and operator-facing status update",
		}
	}
	return []string{
		"draft pull request or governed execution artifact",
		"validation evidence",
		"operator-facing status update",
	}
}

func isIssueScanRunLaunch(content FactoryRunRequestedContent) bool {
	var brief struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(content.Brief, &brief); err != nil {
		return false
	}
	return strings.TrimSpace(brief.Kind) == issueScanBriefKind
}

func workModelOverridesFromRunLaunch(overrides []RunLaunchModelOverride) []work.FactoryOrderModelOverride {
	if len(overrides) == 0 {
		return nil
	}
	out := make([]work.FactoryOrderModelOverride, 0, len(overrides))
	for _, override := range overrides {
		out = append(out, work.FactoryOrderModelOverride{
			Role:                 override.Role,
			Model:                override.Model,
			Provider:             override.Provider,
			Profile:              override.Profile,
			RequestedAuthMode:    override.RequestedAuthMode,
			PreferredTier:        override.PreferredTier,
			RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
			MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(override.MaxCostPerCallUSD),
			ResolvedModel:        override.ResolvedModel,
			ResolvedProvider:     override.ResolvedProvider,
			AuthMode:             override.AuthMode,
		})
	}
	return out
}

func runLaunchIntent(content FactoryRunRequestedContent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Operator-requested Hive run %s.\n\n", content.RunID)
	if len(content.TargetRepos) > 0 {
		fmt.Fprintf(&b, "Target repositories: %s.\n\n", strings.Join(content.TargetRepos, ", "))
	}
	if len(content.Sources) > 0 {
		b.WriteString("Sources:\n")
		for _, source := range content.Sources {
			label := strings.TrimSpace(source.Title)
			if label == "" {
				label = strings.TrimSpace(source.Ref)
			}
			fmt.Fprintf(&b, "- %s: %s\n", strings.TrimSpace(source.Type), label)
		}
		b.WriteString("\n")
	}
	if brief := prettyRunLaunchBrief(content.Brief); brief != "" {
		b.WriteString("Brief:\n")
		b.WriteString(brief)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func runLaunchDefinitionOfDone(content FactoryRunRequestedContent) string {
	return fmt.Sprintf("Complete the operator-requested Hive run %s against the declared target repositories without exceeding the recorded authority and budget envelope.", content.RunID)
}

func runLaunchAcceptanceCriteria(content FactoryRunRequestedContent) string {
	var b strings.Builder
	b.WriteString("- Work is causally traceable to the factory.run.requested event.\n")
	b.WriteString("- Changes stay within the declared authority scope and target repositories.\n")
	b.WriteString("- Any model overrides remain structured FactoryOrder state and pass dispatch-time and Operate-time resolver validation.\n")
	if content.Budget.MaxIterations > 0 {
		fmt.Fprintf(&b, "- Iterations stay within the requested maximum of %d.\n", content.Budget.MaxIterations)
	}
	if content.Budget.MaxCostUSD >= 0 {
		fmt.Fprintf(&b, "- Cost stays within the requested maximum of %.2f USD.\n", content.Budget.MaxCostUSD)
	}
	return strings.TrimSpace(b.String())
}

func runLaunchTestPlan(content FactoryRunRequestedContent) string {
	if len(content.TargetRepos) == 0 {
		return "Run the repository-native validation commands for every touched repository and record the evidence."
	}
	return fmt.Sprintf("Run repository-native validation for %s and record the exact commands and results.", strings.Join(content.TargetRepos, ", "))
}

func prettyRunLaunchBrief(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	encoded, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func runLaunchRiskClass(level event.AuthorityLevel) string {
	switch strings.ToLower(strings.TrimSpace(string(level))) {
	case strings.ToLower(string(event.AuthorityLevelNotification)):
		return "low"
	case strings.ToLower(string(event.AuthorityLevelRecommended)):
		return "medium"
	case strings.ToLower(string(event.AuthorityLevelRequired)):
		return "high"
	default:
		return "medium"
	}
}

func factoryOrderIDForRunLaunch(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	suffix := strings.TrimPrefix(runID, "run_")
	var b strings.Builder
	for _, r := range suffix {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
		case r == '-':
			b.WriteByte('_')
		}
	}
	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		return "", fmt.Errorf("run_id %q does not contain a usable factory-order suffix", runID)
	}
	return "fo_run_" + normalized, nil
}

func cloneRunLaunchFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
