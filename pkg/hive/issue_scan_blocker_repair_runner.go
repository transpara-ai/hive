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

const issueScanBlockerRepairRunnerContextKind = "issue_scan_blocker_repair_runner_context"

// IssueScanBlockerRepairRunner receives the reopened concrete implementation
// task plus the request_changes review findings. It may repair on a branch and
// must return a new Operate result packet; the runtime records that packet
// through the normal Work artifact/completion path before review is rerun.
type IssueScanBlockerRepairRunner func(context.Context, IssueScanBlockerRepairRunnerContext) (IssueScanBlockerRepairRunnerResult, error)

type IssueScanBlockerRepairRunnerContext struct {
	Kind                           string                                  `json:"kind"`
	LifecycleVersion               string                                  `json:"lifecycle_version"`
	RunID                          string                                  `json:"run_id"`
	FactoryOrderID                 string                                  `json:"factory_order_id"`
	Repository                     string                                  `json:"repository"`
	RepoPath                       string                                  `json:"repo_path"`
	ContainmentWatchRoots          []string                                `json:"containment_watch_roots,omitempty"`
	SelectedIssue                  IssueScanStageRoleOutputIssue           `json:"selected_issue"`
	ImplementationTaskID           string                                  `json:"implementation_task_id"`
	ImplementationStageTaskID      string                                  `json:"implementation_stage_task_id"`
	ReviewStageTaskID              string                                  `json:"review_stage_task_id"`
	BlockerStageTaskID             string                                  `json:"blocker_stage_task_id"`
	RequestChangesReviewEventID    string                                  `json:"request_changes_review_event_id"`
	RequestChangesReviewSummary    string                                  `json:"request_changes_review_summary"`
	RequestChangesReviewIssues     []string                                `json:"request_changes_review_issues"`
	RequestChangesReviewConfidence float64                                 `json:"request_changes_review_confidence"`
	ReopenEventID                  string                                  `json:"reopen_event_id"`
	ReopenReason                   string                                  `json:"reopen_reason"`
	ReopenIssues                   []string                                `json:"reopen_issues,omitempty"`
	PreviousOperateBranch          string                                  `json:"previous_operate_branch"`
	PreviousOperateCommit          string                                  `json:"previous_operate_commit"`
	PreviousOperateRange           string                                  `json:"previous_operate_range,omitempty"`
	PreviousChangedFilesSummary    string                                  `json:"previous_changed_files_summary"`
	ImplementationTaskContext      json.RawMessage                         `json:"implementation_task_context,omitempty"`
	ImplementationReadinessGates   []IssueScanImplementationRunnerArtifact `json:"implementation_readiness_gates,omitempty"`
	SourceRefs                     []string                                `json:"source_refs,omitempty"`
	BoundaryDisclaimers            []string                                `json:"boundary_disclaimers,omitempty"`
}

type IssueScanBlockerRepairRunnerResult struct {
	OperateResultBody string `json:"operate_result_body"`
	CompletionSummary string `json:"completion_summary"`
}

type IssueScanBlockerRepairRunnerRecordResult struct {
	RunID                  string
	FactoryOrderID         string
	Repository             string
	ImplementationTaskID   types.EventID
	BlockerStageTaskID     types.EventID
	RequestChangesReviewID types.EventID
	ReopenEventID          types.EventID
	OperateArtifactID      types.EventID
	CompletionEventID      types.EventID
	OperateBranch          string
	OperateCommit          string
	Recorded               bool
}

func (r *Runtime) IssueScanBlockerRepairRunnerContext(runID string) (IssueScanBlockerRepairRunnerContext, error) {
	runnerContext, ready, err := r.issueScanBlockerRepairRunnerContext(runID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, err
	}
	if !ready {
		return IssueScanBlockerRepairRunnerContext{}, fmt.Errorf("issue-scan run %q has no blocker repair ready to run", strings.TrimSpace(runID))
	}
	return runnerContext, nil
}

func (r *Runtime) RunConfiguredIssueScanBlockerRepairRunners(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanBlockerRepairRunnerRecordResult, error) {
	if r == nil || r.issueScanBlockerRepairRunner == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanBlockerRepairRunnerRecordResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RunConfiguredIssueScanBlockerRepairRunner(ctx, runID)
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

func (r *Runtime) RunConfiguredIssueScanBlockerRepairRunner(ctx context.Context, runID string) (IssueScanBlockerRepairRunnerRecordResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanBlockerRepairRunnerRecordResult{RunID: runID}
	if r == nil || r.issueScanBlockerRepairRunner == nil {
		return result, false, nil
	}
	runnerContext, ready, err := r.issueScanBlockerRepairRunnerContext(runID)
	if err != nil || !ready {
		return result, ready, err
	}
	runnerResult, err := r.issueScanBlockerRepairRunner(ctx, runnerContext)
	if err != nil {
		return result, true, err
	}
	recorded, err := r.RecordIssueScanBlockerRepairRunnerResult(runID, runnerResult)
	return recorded, true, err
}

func (r *Runtime) RecordIssueScanBlockerRepairRunnerResult(runID string, runnerResult IssueScanBlockerRepairRunnerResult) (IssueScanBlockerRepairRunnerRecordResult, error) {
	runID = strings.TrimSpace(runID)
	runnerContext, ready, err := r.issueScanBlockerRepairRunnerContext(runID)
	if err != nil {
		return IssueScanBlockerRepairRunnerRecordResult{RunID: runID}, err
	}
	if !ready {
		return IssueScanBlockerRepairRunnerRecordResult{RunID: runID}, fmt.Errorf("issue-scan run %q has no blocker repair ready to record", runID)
	}
	return r.recordIssueScanBlockerRepairRunnerResult(runnerContext, runnerResult)
}

func (r *Runtime) issueScanBlockerRepairRunnerContext(runID string) (IssueScanBlockerRepairRunnerContext, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	} else if parked {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	if len(requests) == 0 {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("dispatch queued issue-scan run %q before blocker repair context: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	reviewStage, err := r.issueScanStageTargetByStageID(drafts, "run_adversarial_review", orderID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	blockerStage, err := r.issueScanStageTargetByStageID(drafts, "drive_blockers_to_zero", orderID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	reviewCompleted, err := r.issueScanStageTaskCompleted(reviewStage.TaskID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	if !reviewCompleted {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	blockerCompleted, err := r.issueScanStageTaskCompleted(blockerStage.TaskID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	if blockerCompleted {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(blockerStage); err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	implementation, implementationReady, err := r.EnsureIssueScanImplementationTask(runID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("ensure issue-scan implementation task before blocker repair context: %w", err)
	}
	if !implementationReady || implementation.ImplementationTaskID == (types.EventID{}) {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	if strings.TrimSpace(implementation.FactoryOrderID) != orderID {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("implementation task belongs to factory order %q, want %q", implementation.FactoryOrderID, orderID)
	}
	implementationTaskID := implementation.ImplementationTaskID
	status, err := r.tasks.GetCompatibilityStatus(implementationTaskID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("read implementation task status: %w", err)
	}
	if status == work.LegacyStatusCompleted {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	blocked, err := r.tasks.IsBlocked(implementationTaskID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	if blocked || status == work.LegacyStatusBlocked {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	reviews, err := r.issueScanCodeReviewsForTask(implementationTaskID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	if len(reviews) == 0 {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	latestReview := reviews[len(reviews)-1]
	if strings.TrimSpace(latestReview.Review.Verdict) != "request_changes" {
		return IssueScanBlockerRepairRunnerContext{}, false, nil
	}
	if strings.TrimSpace(latestReview.Review.Summary) == "" {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("request_changes review %s summary is empty", latestReview.EventID.Value())
	}
	if len(compactStrings(latestReview.Review.Issues)) == 0 {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("request_changes review %s has no issues", latestReview.EventID.Value())
	}
	reopen, ok, err := r.latestIssueScanReopenForTaskAfter(implementationTaskID, latestReview.Timestamp)
	if err != nil || !ok {
		return IssueScanBlockerRepairRunnerContext{}, ok, err
	}
	previousCompletion, ok, err := r.issueScanSupersededImplementationCompletionEvidence(implementationTaskID, implementationStage.TaskID, reopen)
	if err != nil || !ok {
		return IssueScanBlockerRepairRunnerContext{}, ok, err
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	repo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if repo == "" && len(content.TargetRepos) > 0 {
		repo = strings.ToLower(strings.TrimSpace(content.TargetRepos[0]))
	}
	if !ValidTransparaAIRepo(repo) {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("selected issue repository %q is not a Transpara-AI repo", repo)
	}
	resolved, err := r.resolveIssueScanWorkspaceForRepo(repo)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("resolve blocker repair workspace for %s: %w", repo, err)
	}
	implementationArtifacts, err := r.tasks.ListArtifacts(implementationTaskID)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("list implementation task artifacts: %w", err)
	}
	taskContext, ok, err := issueScanImplementationTaskContextArtifactRaw(implementationArtifacts)
	if err != nil {
		return IssueScanBlockerRepairRunnerContext{}, false, err
	}
	if !ok {
		return IssueScanBlockerRepairRunnerContext{}, false, fmt.Errorf("implementation task %s has no %s artifact", implementationTaskID.Value(), IssueScanImplementationTaskContextArtifactLabel)
	}
	return IssueScanBlockerRepairRunnerContext{
		Kind:                           issueScanBlockerRepairRunnerContextKind,
		LifecycleVersion:               issueScanLifecycleVersion,
		RunID:                          strings.TrimSpace(content.RunID),
		FactoryOrderID:                 orderID,
		Repository:                     repo,
		RepoPath:                       resolved.RepoPath,
		ContainmentWatchRoots:          append([]string(nil), resolved.ContainmentWatchRoots...),
		SelectedIssue:                  issueScanStageRoleOutputIssueFromBriefIssue(brief.SelectedIssue),
		ImplementationTaskID:           implementationTaskID.Value(),
		ImplementationStageTaskID:      implementationStage.TaskID.Value(),
		ReviewStageTaskID:              reviewStage.TaskID.Value(),
		BlockerStageTaskID:             blockerStage.TaskID.Value(),
		RequestChangesReviewEventID:    latestReview.EventID.Value(),
		RequestChangesReviewSummary:    strings.TrimSpace(latestReview.Review.Summary),
		RequestChangesReviewIssues:     compactStrings(latestReview.Review.Issues),
		RequestChangesReviewConfidence: latestReview.Review.Confidence,
		ReopenEventID:                  reopen.EventID.Value(),
		ReopenReason:                   reopen.Reason,
		ReopenIssues:                   compactStrings(reopen.Issues),
		PreviousOperateBranch:          previousCompletion.OperateBranch,
		PreviousOperateCommit:          previousCompletion.OperateCommit,
		PreviousOperateRange:           previousCompletion.OperateRange,
		PreviousChangedFilesSummary:    previousCompletion.ChangedFilesSummary,
		ImplementationTaskContext:      taskContext,
		ImplementationReadinessGates:   issueScanImplementationReadinessGateArtifacts(implementationArtifacts),
		SourceRefs: compactStrings([]string{
			requests[0].ID().Value(),
			implementationTaskID.Value(),
			implementationStage.TaskID.Value(),
			reviewStage.TaskID.Value(),
			blockerStage.TaskID.Value(),
			latestReview.EventID.Value(),
			reopen.EventID.Value(),
			previousCompletion.CompletionEventID.Value(),
			previousCompletion.OperateArtifactID.Value(),
		}),
		BoundaryDisclaimers: compactStrings([]string{
			"repair runner may only address accepted request_changes findings for the reopened implementation task",
			"repair runner output must be a new Operate result for the concrete implementation task",
			"repair runner output is not zero-blocker proof by itself",
			"repair runner output is not adversarial review evidence",
			"repair runner output is not PR creation or PR readiness",
			"repair runner output is not Human approval",
			"repair runner output is not merge or deploy authorization",
		}),
	}, true, nil
}

func (r *Runtime) recordIssueScanBlockerRepairRunnerResult(runnerContext IssueScanBlockerRepairRunnerContext, runnerResult IssueScanBlockerRepairRunnerResult) (IssueScanBlockerRepairRunnerRecordResult, error) {
	result := IssueScanBlockerRepairRunnerRecordResult{
		RunID:          runnerContext.RunID,
		FactoryOrderID: runnerContext.FactoryOrderID,
		Repository:     runnerContext.Repository,
	}
	taskID, err := types.NewEventID(runnerContext.ImplementationTaskID)
	if err != nil {
		return result, fmt.Errorf("parse implementation task id: %w", err)
	}
	stageTaskID, err := types.NewEventID(runnerContext.BlockerStageTaskID)
	if err != nil {
		return result, fmt.Errorf("parse blocker stage task id: %w", err)
	}
	reviewID, err := types.NewEventID(runnerContext.RequestChangesReviewEventID)
	if err != nil {
		return result, fmt.Errorf("parse request_changes review id: %w", err)
	}
	reopenID, err := types.NewEventID(runnerContext.ReopenEventID)
	if err != nil {
		return result, fmt.Errorf("parse reopen id: %w", err)
	}
	result.ImplementationTaskID = taskID
	result.BlockerStageTaskID = stageTaskID
	result.RequestChangesReviewID = reviewID
	result.ReopenEventID = reopenID
	body := strings.TrimSpace(runnerResult.OperateResultBody)
	if body == "" {
		return result, fmt.Errorf("blocker repair runner operate_result_body is required")
	}
	parsed, err := parseIssueScanOperateResultArtifact(body)
	if err != nil {
		return result, fmt.Errorf("blocker repair runner operate_result_body is invalid: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(parsed.Commit), strings.TrimSpace(runnerContext.PreviousOperateCommit)) {
		return result, fmt.Errorf("blocker repair runner commit %q must differ from previous reviewed commit", parsed.Commit)
	}
	summary := strings.TrimSpace(runnerResult.CompletionSummary)
	if summary == "" {
		return result, fmt.Errorf("blocker repair runner completion_summary is required")
	}
	causes := issueScanBlockerRepairRunnerRecordCauses(runnerContext, taskID, stageTaskID, reviewID, reopenID)
	if err := r.tasks.AddArtifact(r.humanID, taskID, "Operate result", "text/plain", body, causes, r.convID); err != nil {
		return result, fmt.Errorf("record blocker repair Operate result artifact: %w", err)
	}
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return result, fmt.Errorf("list implementation task artifacts after blocker repair Operate result: %w", err)
	}
	operate, ok := latestIssueScanOperateResultArtifact(artifacts)
	if !ok {
		return result, fmt.Errorf("implementation task %s has no Operate result artifact after blocker repair recording", taskID.Value())
	}
	if err := r.tasks.Complete(r.humanID, taskID, summary, compactEventIDs(append(causes, operate.ID)), r.convID); err != nil {
		return result, fmt.Errorf("complete implementation task from blocker repair runner result: %w", err)
	}
	completion, ok, err := r.issueScanImplementationCompletionEvidence(taskID, stageTaskID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, fmt.Errorf("implementation task %s has no live completion after blocker repair recording", taskID.Value())
	}
	result.OperateArtifactID = operate.ID
	result.CompletionEventID = completion.CompletionEventID
	result.OperateBranch = parsed.Branch
	result.OperateCommit = parsed.Commit
	result.Recorded = true
	return result, nil
}

func (r *Runtime) issueScanSupersededImplementationCompletionEvidence(taskID, stageTaskID types.EventID, reopen issueScanReopenEvidence) (issueScanOperateCompletionEvidence, bool, error) {
	if len(reopen.CompletionRefs) == 0 {
		return issueScanOperateCompletionEvidence{}, false, nil
	}
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("list implementation task artifacts: %w", err)
	}
	var best issueScanOperateCompletionEvidence
	for _, completionRef := range reopen.CompletionRefs {
		ev, err := r.store.Get(completionRef)
		if err != nil {
			return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("read superseded completion %s: %w", completionRef.Value(), err)
		}
		completion, ok := ev.Content().(work.TaskCompletedContent)
		if !ok || completion.TaskID != taskID {
			continue
		}
		operate, ok := issueScanOperateResultArtifactByID(artifacts, completion.ArtifactRef)
		if !ok {
			return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("superseded completion %s has no Operate result artifact", completionRef.Value())
		}
		parsed, err := parseIssueScanOperateResultArtifact(operate.Body)
		if err != nil {
			return issueScanOperateCompletionEvidence{}, false, fmt.Errorf("parse superseded Operate result artifact %s: %w", operate.ID.Value(), err)
		}
		summary := strings.TrimSpace(completion.Summary)
		if summary == "" {
			summary = "superseded implementation completion"
		}
		candidate := issueScanOperateCompletionEvidence{
			TaskID:                  taskID,
			CompletionEventID:       ev.ID(),
			CompletionTimestamp:     ev.Timestamp().Value(),
			CompletionSummary:       summary,
			OperateArtifactID:       operate.ID,
			OperateBranch:           parsed.Branch,
			OperateCommit:           parsed.Commit,
			OperateRange:            parsed.Range,
			ChangedFilesSummary:     parsed.ChangedFilesSummary,
			ImplementationStageTask: stageTaskID,
		}
		if best.CompletionEventID == (types.EventID{}) || candidate.CompletionTimestamp.After(best.CompletionTimestamp) || (candidate.CompletionTimestamp.Equal(best.CompletionTimestamp) && candidate.CompletionEventID.Value() > best.CompletionEventID.Value()) {
			best = candidate
		}
	}
	if best.CompletionEventID == (types.EventID{}) {
		return issueScanOperateCompletionEvidence{}, false, nil
	}
	return best, true, nil
}

func issueScanBlockerRepairRunnerRecordCauses(runnerContext IssueScanBlockerRepairRunnerContext, taskID, stageTaskID, reviewID, reopenID types.EventID) []types.EventID {
	ids := []types.EventID{taskID, stageTaskID, reviewID, reopenID}
	for _, raw := range []string{runnerContext.ImplementationStageTaskID, runnerContext.ReviewStageTaskID} {
		if id, err := types.NewEventID(strings.TrimSpace(raw)); err == nil {
			ids = append(ids, id)
		}
	}
	for _, raw := range runnerContext.SourceRefs {
		if id, err := types.NewEventID(strings.TrimSpace(raw)); err == nil {
			ids = append(ids, id)
		}
	}
	return compactEventIDs(ids)
}
