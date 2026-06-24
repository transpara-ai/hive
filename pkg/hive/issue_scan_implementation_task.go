package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	IssueScanImplementationTaskContextArtifactLabel = "issue_scan_implementation_task_context"

	issueScanImplementationTaskContextArtifactKind = "issue_scan_implementation_task_context"
	issueScanSelectAndDesignStageID                = "select_and_design_approach"
	issueScanConcreteImplementationTaskID          = "concrete_implementation_work"
)

// IssueScanImplementationTaskResult summarizes the concrete Work item seeded
// from the completed select/design stage. This is ordinary implementation work
// for the implementer to Operate; the lifecycle stage itself remains a governed
// role-output aggregator.
type IssueScanImplementationTaskResult struct {
	RunID                    string
	FactoryOrderID           string
	ParentTaskID             types.EventID
	DesignStageTaskID        types.EventID
	DesignEvidenceArtifactID types.EventID
	ImplementationTaskID     types.EventID
	Created                  bool
	AlreadyExists            bool
}

type issueScanOperateCompletionEvidence struct {
	TaskID                  types.EventID
	CompletionEventID       types.EventID
	CompletionTimestamp     time.Time
	CompletionSummary       string
	OperateArtifactID       types.EventID
	OperateBranch           string
	OperateCommit           string
	OperateRange            string
	ChangedFilesSummary     string
	ImplementationStageTask types.EventID
}

// EnsureIssueScanImplementationTasks materializes concrete implementation work
// for dispatched issue-scan runs whose select/design stage has completed.
func (r *Runtime) EnsureIssueScanImplementationTasks(result RunLaunchDispatchResult) ([]IssueScanImplementationTaskResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanImplementationTaskResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		task, ready, err := r.EnsureIssueScanImplementationTask(runID)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ready {
			out = append(out, task)
		}
	}
	return out, errors.Join(errs...)
}

// RecordCompletedIssueScanImplementationRoleOutputs turns completed concrete
// implementation Work tasks into implement_on_branch role-output artifacts.
// Missing or incomplete implementation tasks are normal not-ready states; a
// completed task without a parseable Operate result fails closed.
func (r *Runtime) RecordCompletedIssueScanImplementationRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RecordCompletedIssueScanImplementationRoleOutput(runID)
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

// RecordCompletedIssueScanImplementationRoleOutput records the implementer's
// required stage output after the concrete implementation task has completed
// through the verified Operate path.
func (r *Runtime) RecordCompletedIssueScanImplementationRoleOutput(runID string) (IssueScanStageRoleOutputResult, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanStageRoleOutputResult{}, false, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}
	if len(requests) == 0 {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("dispatch queued issue-scan run %q before implementation role-output recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(implementationStage.TaskID)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}
	if stageCompleted {
		return IssueScanStageRoleOutputResult{RunID: runID, FactoryOrderID: orderID, StageID: implementationStage.Draft.StageID, StageTaskID: implementationStage.TaskID}, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(implementationStage); err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}

	taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return IssueScanStageRoleOutputResult{RunID: runID, FactoryOrderID: orderID, StageID: implementationStage.Draft.StageID, StageTaskID: implementationStage.TaskID}, false, nil
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	status, err := r.tasks.GetCompatibilityStatus(taskID)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("read implementation task status: %w", err)
	}
	if status != work.LegacyStatusCompleted {
		return IssueScanStageRoleOutputResult{RunID: runID, FactoryOrderID: orderID, StageID: implementationStage.Draft.StageID, StageTaskID: implementationStage.TaskID}, false, nil
	}
	completion, ok, err := r.issueScanImplementationCompletionEvidence(taskID, implementationStage.TaskID)
	if err != nil {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, err
	}
	if !ok {
		return IssueScanStageRoleOutputResult{RunID: runID}, false, fmt.Errorf("completed implementation task %s has no live completion evidence", taskID.Value())
	}
	output := issueScanImplementationRoleOutputFromCompletion(completion)
	recorded, err := r.RecordIssueScanStageRoleOutput(runID, "implement_on_branch", output)
	if err != nil {
		return recorded, false, err
	}
	return recorded, true, nil
}

// EnsureIssueScanImplementationTask creates or repairs the concrete
// implementation task for one issue-scan run after design evidence exists. The
// bool return is false when the design stage has not completed yet.
func (r *Runtime) EnsureIssueScanImplementationTask(runID string) (IssueScanImplementationTaskResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanImplementationTaskResult{RunID: runID}
	if r == nil || r.store == nil || r.tasks == nil {
		return result, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return result, false, fmt.Errorf("run_id is required")
	}

	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return result, false, err
	}
	if len(requests) == 0 {
		return result, false, fmt.Errorf("queued run %q not found", runID)
	}
	request := requests[0]
	content, ok := request.Content().(FactoryRunRequestedContent)
	if !ok {
		return result, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return result, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return result, false, err
	}
	result.FactoryOrderID = orderID
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return result, false, fmt.Errorf("dispatch queued issue-scan run %q before implementation task materialization: %w", runID, err)
	}
	dispatched, err := dispatchedFactoryOrderIDs(r.store)
	if err != nil {
		return result, false, err
	}
	parentTaskID, ok := dispatched[orderID]
	if !ok || parentTaskID == (types.EventID{}) {
		return result, false, fmt.Errorf("queued run %q has not been dispatched to a FactoryOrder task", runID)
	}
	result.ParentTaskID = parentTaskID
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return result, false, err
	}
	designTarget, err := r.issueScanStageTargetByStageID(drafts, issueScanSelectAndDesignStageID, orderID)
	if err != nil {
		return result, false, err
	}
	result.DesignStageTaskID = designTarget.TaskID
	completed, err := r.issueScanStageTaskCompleted(designTarget.TaskID)
	if err != nil {
		return result, false, err
	}
	if !completed {
		return result, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(designTarget); err != nil {
		return result, false, err
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return result, false, err
	}
	if err := r.verifyIssueScanImplementationWorkspace(content, brief.SelectedIssue); err != nil {
		return result, false, err
	}
	evidence, evidenceArtifactID, ok, err := r.issueScanStageRuntimeEvidenceForCompletedStage(content, orderID, designTarget)
	if err != nil {
		return result, false, err
	}
	if !ok {
		return result, false, fmt.Errorf("completed design stage %q has no %s artifact", designTarget.Draft.StageID, IssueScanStageRuntimeEvidenceArtifactLabel)
	}
	result.DesignEvidenceArtifactID = evidenceArtifactID

	taskID, created, already, err := r.ensureIssueScanImplementationTask(content, order, parentTaskID, designTarget, evidenceArtifactID, evidence, request.ID(), brief.SelectedIssue)
	if err != nil {
		return result, false, err
	}
	result.ImplementationTaskID = taskID
	result.Created = created
	result.AlreadyExists = already
	return result, true, nil
}

func (r *Runtime) issueScanStageTargetByStageID(drafts []issueScanLifecycleStageTaskDraft, stageID, orderID string) (*issueScanStageAdvanceTarget, error) {
	want := safeRunLaunchID(stageID)
	if want == "" {
		return nil, fmt.Errorf("stage id %q is unsafe", stageID)
	}
	for _, draft := range drafts {
		if safeRunLaunchID(draft.StageID) != want {
			continue
		}
		taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, draft.Options.CanonicalTaskID)
		if err != nil {
			return nil, fmt.Errorf("find stage task %q: %w", draft.StageID, err)
		}
		if !exists {
			return nil, fmt.Errorf("stage task %q has not been created", draft.StageID)
		}
		if strings.TrimSpace(factoryOrderID) != orderID {
			return nil, fmt.Errorf("stage task %q belongs to factory order %q, want %q", draft.StageID, factoryOrderID, orderID)
		}
		return &issueScanStageAdvanceTarget{Draft: draft, TaskID: taskID}, nil
	}
	return nil, fmt.Errorf("stage %q not found", stageID)
}

func (r *Runtime) issueScanStageRuntimeEvidenceForCompletedStage(content FactoryRunRequestedContent, orderID string, target *issueScanStageAdvanceTarget) (IssueScanStageRuntimeEvidence, types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(target.TaskID)
	if err != nil {
		return IssueScanStageRuntimeEvidence{}, types.EventID{}, false, fmt.Errorf("list stage task %q artifacts: %w", target.Draft.StageID, err)
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		parsed, ok, err := issueScanStageRuntimeEvidenceArtifact(artifact.Label, artifact.Body)
		if err != nil {
			return IssueScanStageRuntimeEvidence{}, types.EventID{}, false, fmt.Errorf("parse runtime evidence artifact %s: %w", artifact.ID.Value(), err)
		}
		if !ok {
			continue
		}
		normalized, err := normalizeIssueScanStageRuntimeEvidence(content, orderID, target.Draft, parsed)
		if err != nil {
			return IssueScanStageRuntimeEvidence{}, types.EventID{}, false, fmt.Errorf("validate runtime evidence artifact %s: %w", artifact.ID.Value(), err)
		}
		return normalized, artifact.ID, true, nil
	}
	return IssueScanStageRuntimeEvidence{}, types.EventID{}, false, nil
}

func issueScanStageRuntimeEvidenceArtifact(label, body string) (IssueScanStageRuntimeEvidence, bool, error) {
	if strings.TrimSpace(label) != IssueScanStageRuntimeEvidenceArtifactLabel {
		return IssueScanStageRuntimeEvidence{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload IssueScanStageRuntimeEvidence
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageRuntimeEvidenceArtifactKind {
		return IssueScanStageRuntimeEvidence{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageRuntimeEvidenceArtifactKind)
	}
	return payload, true, nil
}

func (r *Runtime) ensureIssueScanImplementationTask(content FactoryRunRequestedContent, order work.FactoryOrder, parentTaskID types.EventID, designTarget *issueScanStageAdvanceTarget, evidenceArtifactID types.EventID, evidence IssueScanStageRuntimeEvidence, requestID types.EventID, issue issueScanBriefIssuePayload) (types.EventID, bool, bool, error) {
	canonicalTaskID := issueScanImplementationTaskCanonicalID(order.ID)
	if canonicalTaskID == "" {
		return types.EventID{}, false, false, fmt.Errorf("implementation task canonical id is empty for FactoryOrder %q", order.ID)
	}
	taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, canonicalTaskID)
	if err != nil {
		return types.EventID{}, false, false, fmt.Errorf("find implementation task by canonical id: %w", err)
	}
	if exists && strings.TrimSpace(factoryOrderID) != strings.TrimSpace(order.ID) {
		return types.EventID{}, false, false, fmt.Errorf("implementation task canonical id %q belongs to factory order %q, want %q", canonicalTaskID, factoryOrderID, order.ID)
	}
	created := false
	if !exists {
		task, err := r.tasks.CreateV39(r.humanID, issueScanImplementationTaskCreateOptions(content, order, evidence, issue), compactEventIDs([]types.EventID{requestID, parentTaskID, designTarget.TaskID, evidenceArtifactID}), runLaunchConversationID(content.RunID, r.convID))
		if err != nil {
			return types.EventID{}, false, false, fmt.Errorf("create issue-scan implementation task: %w", err)
		}
		taskID = task.ID
		created = true
	}
	if err := r.attachIssueScanImplementationTaskReadinessGates(content, order, taskID, designTarget.TaskID, evidenceArtifactID, evidence, issue); err != nil {
		return types.EventID{}, false, false, err
	}
	if err := r.attachIssueScanImplementationTaskContext(content, order, taskID, designTarget.TaskID, evidenceArtifactID, evidence, issue); err != nil {
		return types.EventID{}, false, false, err
	}
	return taskID, created, exists, nil
}

func issueScanImplementationTaskCanonicalID(orderID string) string {
	suffix := factoryOrderIDSuffix(orderID)
	if suffix == "" {
		return ""
	}
	return "tsk_" + suffix + "_" + issueScanConcreteImplementationTaskID
}

func issueScanImplementationTaskCreateOptions(content FactoryRunRequestedContent, order work.FactoryOrder, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) work.TaskCreateOptions {
	riskClass := valueOr(order.RiskClass, "high")
	title := "Implement selected issue-scan approach"
	if selected := issueScanEvidenceSummary(evidence, "selected_approach"); selected != "" {
		title = truncateRunLaunchText("Implement: "+strings.Join(strings.Fields(selected), " "), 180)
	}
	return work.TaskCreateOptions{
		Title:                  title,
		Description:            issueScanImplementationTaskDescription(content, order, evidence, issue),
		CanonicalTaskID:        issueScanImplementationTaskCanonicalID(order.ID),
		FactoryOrderID:         order.ID,
		RequirementIDs:         factoryOrderRequirementIDs(order),
		AcceptanceCriterionIDs: factoryOrderAcceptanceCriterionIDs(order),
		Cell:                   "implementation",
		RiskClass:              riskClass,
		ExpectedOutputs: []string{
			"branch_name",
			"commit_sha",
			"changed_files",
			"validation_output",
		},
		Priority: issueScanLifecycleStageTaskPriority(riskClass),
	}
}

func issueScanImplementationTaskDescription(content FactoryRunRequestedContent, order work.FactoryOrder, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Concrete implementation task for issue-scan run `%s` and FactoryOrder `%s`.\n\n", strings.TrimSpace(content.RunID), strings.TrimSpace(order.ID))
	appendIssueScanImplementationSelectedIssueSection(&b, issue)
	appendIssueScanImplementationEvidenceSection(&b, "Selected approach", evidence, "selected_approach")
	appendIssueScanImplementationEvidenceSection(&b, "Implementation task plan", evidence, "implementation_task_plan")
	appendIssueScanImplementationEvidenceSection(&b, "Acceptance criteria", evidence, "acceptance_criteria")
	appendIssueScanImplementationEvidenceSection(&b, "Test plan", evidence, "test_plan")
	appendIssueScanImplementationEvidenceSection(&b, "Authority gate requirements", evidence, "authority_gate_requirements")
	b.WriteString("Operate boundary: implement on a branch in the configured repository after the runtime has verified it matches the selected Transpara-AI repo. Do not merge, deploy, request protected production actions, or claim Human approval.")
	return strings.TrimSpace(b.String())
}

func (r *Runtime) attachIssueScanImplementationTaskReadinessGates(content FactoryRunRequestedContent, order work.FactoryOrder, taskID, designStageTaskID, evidenceArtifactID types.EventID, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) error {
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return fmt.Errorf("list implementation task artifacts: %w", err)
	}
	existing := map[string]bool{}
	for _, artifact := range artifacts {
		existing[strings.TrimSpace(artifact.Label)] = true
	}
	gates := []struct {
		label string
		body  string
	}{
		{work.GateDefinitionOfDone, issueScanImplementationDefinitionOfDone(content, order, evidence, issue)},
		{work.GateAcceptanceCriteria, issueScanImplementationAcceptanceCriteria(content, order, evidence, issue)},
		{work.GateTestPlan, issueScanImplementationTestPlan(content, order, evidence, issue)},
	}
	causes := compactEventIDs([]types.EventID{taskID, designStageTaskID, evidenceArtifactID})
	for _, gate := range gates {
		if strings.TrimSpace(gate.body) == "" {
			return fmt.Errorf("implementation task readiness gate %q body is empty", gate.label)
		}
		if existing[strings.TrimSpace(gate.label)] {
			continue
		}
		if err := r.tasks.AddArtifact(r.humanID, taskID, gate.label, "text/markdown", gate.body, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
			return fmt.Errorf("attach implementation task gate %q: %w", gate.label, err)
		}
		existing[strings.TrimSpace(gate.label)] = true
	}
	return nil
}

func (r *Runtime) attachIssueScanImplementationTaskContext(content FactoryRunRequestedContent, order work.FactoryOrder, taskID, designStageTaskID, evidenceArtifactID types.EventID, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) error {
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return fmt.Errorf("list implementation task context artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Label) == IssueScanImplementationTaskContextArtifactLabel {
			return nil
		}
	}
	body, err := issueScanImplementationTaskContextBody(content, order, designStageTaskID, evidenceArtifactID, evidence, issue)
	if err != nil {
		return err
	}
	causes := compactEventIDs([]types.EventID{taskID, designStageTaskID, evidenceArtifactID})
	return r.tasks.AddArtifact(r.humanID, taskID, IssueScanImplementationTaskContextArtifactLabel, issueScanExecutionPlanArtifactMediaType, body, causes, runLaunchConversationID(content.RunID, r.convID))
}

func (r *Runtime) issueScanImplementationCompletionEvidence(taskID, implementationStageTaskID types.EventID) (issueScanOperateCompletionEvidence, bool, error) {
	completionID, completion, completionAt, ok, err := r.liveIssueScanImplementationCompletion(taskID)
	if err != nil || !ok {
		return issueScanOperateCompletionEvidence{}, ok, err
	}
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("list implementation task artifacts: %w", err)
	}
	operate, ok := issueScanOperateResultArtifactByID(artifacts, completion.ArtifactRef)
	if !ok {
		operate, ok = latestIssueScanOperateResultArtifact(artifacts)
	}
	if !ok {
		return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("completed implementation task %s has no Operate result artifact", taskID.Value())
	}
	parsed, err := parseIssueScanOperateResultArtifact(operate.Body)
	if err != nil {
		return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("parse Operate result artifact %s: %w", operate.ID.Value(), err)
	}
	summary := strings.TrimSpace(completion.Summary)
	if summary == "" {
		summary = "implementation task completed through verified Operate path"
	}
	return issueScanOperateCompletionEvidence{
		TaskID:                  taskID,
		CompletionEventID:       completionID,
		CompletionTimestamp:     completionAt,
		CompletionSummary:       summary,
		OperateArtifactID:       operate.ID,
		OperateBranch:           parsed.Branch,
		OperateCommit:           parsed.Commit,
		OperateRange:            parsed.Range,
		ChangedFilesSummary:     parsed.ChangedFilesSummary,
		ImplementationStageTask: implementationStageTaskID,
	}, true, nil
}

func (r *Runtime) liveIssueScanImplementationCompletion(taskID types.EventID) (types.EventID, work.TaskCompletedContent, time.Time, bool, error) {
	completedEvents, err := eventsByTypePaginated(r.store, work.EventTypeTaskCompleted, defaultOperatorProjectionLimit)
	if err != nil {
		return types.EventID{}, work.TaskCompletedContent{}, time.Time{}, false, fmt.Errorf("work.task.completed: %w", err)
	}
	reopenedEvents, err := eventsByTypePaginated(r.store, work.EventTypeTaskReopened, defaultOperatorProjectionLimit)
	if err != nil {
		return types.EventID{}, work.TaskCompletedContent{}, time.Time{}, false, fmt.Errorf("work.task.reopened: %w", err)
	}
	superseded := map[types.EventID]bool{}
	for _, ev := range reopenedEvents {
		if c, ok := ev.Content().(work.TaskReopenedContent); ok {
			for _, ref := range c.CompletionRefs {
				superseded[ref] = true
			}
		}
	}
	for _, ev := range completedEvents {
		c, ok := ev.Content().(work.TaskCompletedContent)
		if !ok || superseded[ev.ID()] || c.TaskID != taskID {
			continue
		}
		return ev.ID(), c, ev.Timestamp().Value(), true, nil
	}
	return types.EventID{}, work.TaskCompletedContent{}, time.Time{}, false, nil
}

func issueScanOperateResultArtifactByID(artifacts []work.ArtifactEvent, id types.EventID) (work.ArtifactEvent, bool) {
	if id == (types.EventID{}) {
		return work.ArtifactEvent{}, false
	}
	for _, artifact := range artifacts {
		if artifact.ID == id && strings.EqualFold(strings.TrimSpace(artifact.Label), "Operate result") {
			return artifact, true
		}
	}
	return work.ArtifactEvent{}, false
}

func latestIssueScanOperateResultArtifact(artifacts []work.ArtifactEvent) (work.ArtifactEvent, bool) {
	var best work.ArtifactEvent
	for _, artifact := range artifacts {
		if !strings.EqualFold(strings.TrimSpace(artifact.Label), "Operate result") {
			continue
		}
		if best.ID == (types.EventID{}) || artifact.Timestamp.After(best.Timestamp) || (artifact.Timestamp.Equal(best.Timestamp) && artifact.ID.Value() > best.ID.Value()) {
			best = artifact
		}
	}
	if best.ID == (types.EventID{}) {
		return work.ArtifactEvent{}, false
	}
	return best, true
}

type parsedIssueScanOperateResult struct {
	Branch              string
	Commit              string
	Range               string
	ChangedFilesSummary string
}

func parseIssueScanOperateResultArtifact(body string) (parsedIssueScanOperateResult, error) {
	var out parsedIssueScanOperateResult
	header := true
	statLines := []string{}
	for _, line := range strings.Split(body, "\n") {
		if header {
			if strings.TrimSpace(line) == "" {
				header = false
				continue
			}
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "commit":
				if out.Commit == "" {
					out.Commit = strings.TrimSpace(value)
				}
			case "head":
				out.Commit = strings.TrimSpace(value)
			case "range":
				out.Range = strings.TrimSpace(value)
			case "branch":
				out.Branch = strings.TrimSpace(value)
			}
			continue
		}
		if strings.TrimSpace(line) != "" {
			statLines = append(statLines, line)
		}
	}
	out.Commit = strings.TrimSpace(out.Commit)
	out.Branch = strings.TrimSpace(out.Branch)
	out.Range = strings.TrimSpace(out.Range)
	out.ChangedFilesSummary = strings.TrimSpace(strings.Join(statLines, "\n"))
	switch {
	case out.Commit == "":
		return parsedIssueScanOperateResult{}, fmt.Errorf("commit/head is required")
	case out.Branch == "":
		return parsedIssueScanOperateResult{}, fmt.Errorf("branch is required")
	case out.ChangedFilesSummary == "":
		return parsedIssueScanOperateResult{}, fmt.Errorf("changed file stat is required")
	}
	return out, nil
}

func issueScanImplementationRoleOutputFromCompletion(evidence issueScanOperateCompletionEvidence) IssueScanStageRoleOutputEvidence {
	refs := compactStrings([]string{evidence.OperateArtifactID.Value(), evidence.CompletionEventID.Value()})
	commitSummary := evidence.OperateCommit
	if evidence.OperateRange != "" {
		commitSummary = fmt.Sprintf("%s (%s)", evidence.OperateCommit, evidence.OperateRange)
	}
	return IssueScanStageRoleOutputEvidence{
		Role:         "implementer",
		Summary:      fmt.Sprintf("Implementation task %s completed on branch %s with verified Operate commit %s.", evidence.TaskID.Value(), evidence.OperateBranch, evidence.OperateCommit),
		EvidenceRefs: refs,
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "branch_name",
				Summary:      evidence.OperateBranch,
				EvidenceRefs: []string{evidence.OperateArtifactID.Value()},
			},
			{
				Key:          "commit_sha",
				Summary:      commitSummary,
				EvidenceRefs: []string{evidence.OperateArtifactID.Value()},
			},
			{
				Key:          "changed_files",
				Summary:      evidence.ChangedFilesSummary,
				EvidenceRefs: []string{evidence.OperateArtifactID.Value()},
			},
			{
				Key:          "validation_output",
				Summary:      evidence.CompletionSummary,
				EvidenceRefs: refs,
			},
		},
		SourceRefs: compactStrings([]string{
			evidence.TaskID.Value(),
			evidence.ImplementationStageTask.Value(),
			evidence.OperateArtifactID.Value(),
			evidence.CompletionEventID.Value(),
		}),
	}
}

func issueScanImplementationDefinitionOfDone(content FactoryRunRequestedContent, order work.FactoryOrder, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Implementation for run `%s` / FactoryOrder `%s` is done when the selected approach is implemented on a branch and commit evidence is recorded.\n\n", strings.TrimSpace(content.RunID), strings.TrimSpace(order.ID))
	appendIssueScanImplementationSelectedIssueSection(&b, issue)
	appendIssueScanImplementationEvidenceSection(&b, "Definition of done", evidence, "definition_of_done")
	appendIssueScanImplementationEvidenceSection(&b, "Selected approach", evidence, "selected_approach")
	appendIssueScanImplementationEvidenceSection(&b, "Implementation task plan", evidence, "implementation_task_plan")
	appendIssueScanImplementationEvidenceSection(&b, "Authority gate requirements", evidence, "authority_gate_requirements")
	b.WriteString("Completion requires a verified forward commit, clean working tree, branch name, commit SHA, changed files, and validation output. Completion is not merge, deploy, PR readiness, or Human approval.")
	return strings.TrimSpace(b.String())
}

func issueScanImplementationAcceptanceCriteria(content FactoryRunRequestedContent, order work.FactoryOrder, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Acceptance criteria for run `%s` / FactoryOrder `%s`.\n\n", strings.TrimSpace(content.RunID), strings.TrimSpace(order.ID))
	appendIssueScanImplementationSelectedIssueSection(&b, issue)
	appendIssueScanImplementationEvidenceSection(&b, "Acceptance criteria", evidence, "acceptance_criteria")
	appendIssueScanImplementationEvidenceSection(&b, "Selected approach", evidence, "selected_approach")
	appendIssueScanImplementationEvidenceSection(&b, "Authority gate requirements", evidence, "authority_gate_requirements")
	b.WriteString("Keep changes inside the configured repository and recorded authority envelope. Do not merge, deploy, or mark any PR ready from this task.")
	return strings.TrimSpace(b.String())
}

func issueScanImplementationTestPlan(content FactoryRunRequestedContent, order work.FactoryOrder, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation plan for run `%s` / FactoryOrder `%s`.\n\n", strings.TrimSpace(content.RunID), strings.TrimSpace(order.ID))
	if repo := strings.TrimSpace(issue.Repo); repo != "" {
		fmt.Fprintf(&b, "Target repo: %s\n\n", repo)
	}
	appendIssueScanImplementationEvidenceSection(&b, "Test plan", evidence, "test_plan")
	b.WriteString("Record exact validation output after implementation. If validation cannot run, record the blocker instead of completing the task.")
	return strings.TrimSpace(b.String())
}

func issueScanImplementationTaskContextBody(content FactoryRunRequestedContent, order work.FactoryOrder, designStageTaskID, evidenceArtifactID types.EventID, evidence IssueScanStageRuntimeEvidence, issue issueScanBriefIssuePayload) (string, error) {
	keys := []string{
		"selected_approach",
		"definition_of_done",
		"implementation_task_plan",
		"acceptance_criteria",
		"test_plan",
		"authority_gate_requirements",
	}
	payload := struct {
		Kind                     string                              `json:"kind"`
		LifecycleVersion         string                              `json:"lifecycle_version"`
		RunID                    string                              `json:"run_id"`
		FactoryOrderID           string                              `json:"factory_order_id"`
		SourceStageID            string                              `json:"source_stage_id"`
		SourceStageTaskID        string                              `json:"source_stage_task_id"`
		SourceRuntimeEvidenceRef string                              `json:"source_runtime_evidence_ref"`
		SelectedIssue            issueScanBriefIssuePayload          `json:"selected_issue"`
		TargetRepos              []string                            `json:"target_repos"`
		Outputs                  []IssueScanStageRuntimeEvidenceItem `json:"outputs"`
		EvidenceKind             string                              `json:"evidence_kind"`
		EvidenceStatus           string                              `json:"evidence_status"`
		BoundaryDisclaimers      []string                            `json:"boundary_disclaimers"`
	}{
		Kind:                     issueScanImplementationTaskContextArtifactKind,
		LifecycleVersion:         issueScanLifecycleVersion,
		RunID:                    strings.TrimSpace(content.RunID),
		FactoryOrderID:           strings.TrimSpace(order.ID),
		SourceStageID:            issueScanSelectAndDesignStageID,
		SourceStageTaskID:        designStageTaskID.Value(),
		SourceRuntimeEvidenceRef: evidenceArtifactID.Value(),
		SelectedIssue:            issue,
		TargetRepos:              append([]string(nil), content.TargetRepos...),
		Outputs:                  issueScanEvidenceItemsForKeys(evidence, keys),
		EvidenceKind:             "implementation_task_seeded_from_design_stage",
		EvidenceStatus:           "ready_for_implementer",
		BoundaryDisclaimers: []string{
			"implementation_task_only",
			"not_stage_completion",
			"not_pr_readiness",
			"not_human_approval",
			"no_merge_or_deploy_claim",
		},
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan implementation task context: %w", err)
	}
	return string(encoded), nil
}

func (r *Runtime) verifyIssueScanImplementationWorkspace(content FactoryRunRequestedContent, issue issueScanBriefIssuePayload) error {
	targetRepo := strings.TrimSpace(issue.Repo)
	if targetRepo == "" && len(content.TargetRepos) > 0 {
		targetRepo = strings.TrimSpace(content.TargetRepos[0])
	}
	if targetRepo == "" {
		return fmt.Errorf("issue-scan target repo is required before implementation task creation")
	}
	if r == nil || (strings.TrimSpace(r.repoPath) == "" && strings.TrimSpace(r.repoWorkspaceRoot) == "") {
		return nil
	}
	if _, err := r.resolveIssueScanWorkspaceForRepo(targetRepo); err != nil {
		return fmt.Errorf("verify issue-scan implementation workspace: %w", err)
	}
	return nil
}

func transparaAIRepoSlugForPath(repoPath string) (string, bool, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return "", false, nil
	}
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("git remote get-url origin: %w", err)
	}
	slug, ok := transparaAIRepoSlugFromRemoteURL(string(out))
	return slug, ok, nil
}

func transparaAIRepoSlugFromRemoteURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	var path string
	if strings.HasPrefix(raw, "git@github.com:") {
		path = strings.TrimPrefix(raw, "git@github.com:")
	} else if parsed, err := url.Parse(raw); err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
		path = strings.TrimPrefix(parsed.Path, "/")
	} else if strings.HasPrefix(raw, "github.com/") {
		path = strings.TrimPrefix(raw, "github.com/")
	}
	path = strings.TrimSuffix(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.ToLower(strings.TrimSpace(path))
	if !ValidTransparaAIRepo(path) {
		return "", false
	}
	return path, true
}

func appendIssueScanImplementationSelectedIssueSection(b *strings.Builder, issue issueScanBriefIssuePayload) {
	repo := strings.TrimSpace(issue.Repo)
	title := strings.TrimSpace(issue.Title)
	if repo == "" && issue.Number <= 0 && title == "" {
		return
	}
	number := ""
	if issue.Number > 0 {
		number = fmt.Sprintf("#%d", issue.Number)
	}
	fmt.Fprintf(b, "Selected issue:\n")
	if repo != "" || number != "" {
		fmt.Fprintf(b, "%s%s", repo, number)
		if title != "" {
			fmt.Fprintf(b, " - %s", title)
		}
		b.WriteString("\n")
	} else if title != "" {
		fmt.Fprintf(b, "%s\n", title)
	}
	if url := strings.TrimSpace(issue.URL); url != "" {
		fmt.Fprintf(b, "URL: %s\n", url)
	}
	if labels := compactStrings(issue.Labels); len(labels) > 0 {
		fmt.Fprintf(b, "Labels: %s\n", strings.Join(labels, ", "))
	}
	if body := strings.TrimSpace(issue.Body); body != "" {
		fmt.Fprintf(b, "Body excerpt:\n%s\n", truncateRunLaunchText(body, 1200))
	}
	b.WriteString("\n")
}

func appendIssueScanImplementationEvidenceSection(b *strings.Builder, title string, evidence IssueScanStageRuntimeEvidence, key string) {
	item, ok := issueScanEvidenceItem(evidence, key)
	if !ok {
		return
	}
	fmt.Fprintf(b, "%s:\n%s\n", title, valueOr(item.Summary, key))
	if len(item.EvidenceRefs) > 0 {
		fmt.Fprintf(b, "Evidence refs: %s\n", strings.Join(item.EvidenceRefs, ", "))
	}
	b.WriteString("\n")
}

func issueScanEvidenceSummary(evidence IssueScanStageRuntimeEvidence, key string) string {
	item, ok := issueScanEvidenceItem(evidence, key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(item.Summary)
}

func issueScanEvidenceItemsForKeys(evidence IssueScanStageRuntimeEvidence, keys []string) []IssueScanStageRuntimeEvidenceItem {
	out := make([]IssueScanStageRuntimeEvidenceItem, 0, len(keys))
	for _, key := range keys {
		item, ok := issueScanEvidenceItem(evidence, key)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func issueScanEvidenceItem(evidence IssueScanStageRuntimeEvidence, key string) (IssueScanStageRuntimeEvidenceItem, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return IssueScanStageRuntimeEvidenceItem{}, false
	}
	items := make([]IssueScanStageRuntimeEvidenceItem, 0, len(evidence.EvidenceItems)+len(evidence.RoleOutputs))
	items = append(items, evidence.EvidenceItems...)
	for _, roleOutput := range evidence.RoleOutputs {
		for _, output := range roleOutput.Outputs {
			output.EvidenceRefs = compactStrings(append(output.EvidenceRefs, roleOutput.EvidenceRefs...))
			items = append(items, output)
		}
	}
	summaries := []string{}
	refs := []string{}
	for _, item := range items {
		if strings.TrimSpace(item.Key) != key {
			continue
		}
		if strings.TrimSpace(item.Summary) != "" {
			summaries = append(summaries, strings.TrimSpace(item.Summary))
		}
		refs = append(refs, item.EvidenceRefs...)
	}
	if len(summaries) == 0 && len(refs) == 0 {
		return IssueScanStageRuntimeEvidenceItem{}, false
	}
	sort.Strings(summaries)
	return IssueScanStageRuntimeEvidenceItem{
		Key:          key,
		Summary:      strings.Join(compactStrings(summaries), "\n"),
		EvidenceRefs: compactStrings(refs),
	}, true
}
