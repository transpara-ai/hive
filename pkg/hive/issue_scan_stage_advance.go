package hive

import (
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
