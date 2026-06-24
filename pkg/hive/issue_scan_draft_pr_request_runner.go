package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

const issueScanDraftPRAuthorityRequestRunnerContextKind = "issue_scan_draft_pr_authority_request_runner_context"

// IssueScanDraftPRAuthorityRequester supplies the base ref/SHA and nonce needed
// to raise the protected draft-PR authority request after an issue-scan run has
// reached the zero-blocker PR-surfacing gate. The runtime raises only the
// request; Human approval remains required before any PR can be created.
type IssueScanDraftPRAuthorityRequester func(context.Context, IssueScanDraftPRAuthorityRequestRunnerContext) (IssueScanDraftPRAuthorityRequestRunnerResult, error)

type IssueScanDraftPRAuthorityRequestRunnerContext struct {
	Kind                  string                        `json:"kind"`
	LifecycleVersion      string                        `json:"lifecycle_version"`
	RunID                 string                        `json:"run_id"`
	FactoryOrderID        string                        `json:"factory_order_id"`
	Repository            string                        `json:"repository"`
	RepoPath              string                        `json:"repo_path,omitempty"`
	ContainmentWatchRoots []string                      `json:"containment_watch_roots,omitempty"`
	ReadyStageTaskID      string                        `json:"ready_stage_task_id"`
	BlockerStageTaskID    string                        `json:"blocker_stage_task_id"`
	ImplementationTaskID  string                        `json:"implementation_task_id"`
	SelectedIssue         IssueScanStageRoleOutputIssue `json:"selected_issue"`
	OperateBranch         string                        `json:"operate_branch"`
	OperateCommit         string                        `json:"operate_commit"`
	OperateRange          string                        `json:"operate_range,omitempty"`
	ChangedFilesSummary   string                        `json:"changed_files_summary,omitempty"`
	BoundaryDisclaimers   []string                      `json:"boundary_disclaimers,omitempty"`
}

type IssueScanDraftPRAuthorityRequestRunnerResult struct {
	BaseRef string `json:"base_ref"`
	BaseSHA string `json:"base_sha"`
	Nonce   string `json:"nonce"`
}

type IssueScanDraftPRAuthorityRequestRunnerRecordResult struct {
	RunID               string
	FactoryOrderID      string
	Repository          string
	RequestID           types.EventID
	DraftPRTarget       DraftPRTarget
	Raised              bool
	AlreadyRaised       bool
	HeldPendingApproval bool
	AutoApproved        bool
}

func (r *Runtime) RunConfiguredIssueScanDraftPRAuthorityRequests(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanDraftPRAuthorityRequestRunnerRecordResult, error) {
	if r == nil || r.issueScanDraftPRAuthorityRequester == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanDraftPRAuthorityRequestRunnerRecordResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RunConfiguredIssueScanDraftPRAuthorityRequest(ctx, runID)
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

func (r *Runtime) RunConfiguredIssueScanDraftPRAuthorityRequest(ctx context.Context, runID string) (IssueScanDraftPRAuthorityRequestRunnerRecordResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanDraftPRAuthorityRequestRunnerRecordResult{RunID: runID}
	if r == nil || r.issueScanDraftPRAuthorityRequester == nil {
		return result, false, nil
	}
	requestContext, ready, err := r.issueScanDraftPRAuthorityRequestRunnerContext(runID)
	if err != nil || !ready {
		return result, ready, err
	}
	if existing, found, err := r.existingIssueScanDraftPRAuthorityRequestForRunnerContext(runID, requestContext); err != nil {
		return result, true, err
	} else if found {
		return existing, true, nil
	}
	runnerResult, err := r.issueScanDraftPRAuthorityRequester(ctx, requestContext)
	if err != nil {
		return result, true, err
	}
	baseRef := valueOr(strings.TrimSpace(runnerResult.BaseRef), "main")
	baseSHA := strings.TrimSpace(runnerResult.BaseSHA)
	nonce := strings.TrimSpace(runnerResult.Nonce)
	if baseSHA == "" {
		return result, true, fmt.Errorf("issue-scan draft PR authority requester returned empty base_sha")
	}
	if nonce == "" {
		return result, true, fmt.Errorf("issue-scan draft PR authority requester returned empty nonce")
	}
	raised, err := r.RaiseIssueScanDraftPRAuthorityRequest(runID, baseRef, baseSHA, nonce)
	if err != nil {
		return result, true, err
	}
	result.RunID = raised.RunID
	result.FactoryOrderID = raised.FactoryOrderID
	result.Repository = raised.Repository
	result.RequestID = raised.RequestID
	result.DraftPRTarget = raised.DraftPRTarget
	result.Raised = raised.Raised
	result.AlreadyRaised = raised.AlreadyRaised
	result.HeldPendingApproval = raised.HeldPendingApproval
	result.AutoApproved = raised.AutoApproved
	return result, true, nil
}

func (r *Runtime) existingIssueScanDraftPRAuthorityRequestForRunnerContext(runID string, requestContext IssueScanDraftPRAuthorityRequestRunnerContext) (IssueScanDraftPRAuthorityRequestRunnerRecordResult, bool, error) {
	result := IssueScanDraftPRAuthorityRequestRunnerRecordResult{
		RunID:          strings.TrimSpace(runID),
		FactoryOrderID: requestContext.FactoryOrderID,
		Repository:     requestContext.Repository,
	}
	if r == nil || r.store == nil {
		return result, false, fmt.Errorf("runtime store is required")
	}
	events, err := eventsByTypePaginated(r.store, EventTypeAuthorityRequestRecorded, defaultOperatorProjectionLimit)
	if err != nil {
		return result, false, fmt.Errorf("load authority requests: %w", err)
	}
	for _, ev := range events {
		content, ok := ev.Content().(AuthorityRequestRecordedContent)
		if !ok || content.ActionName != string(safety.ActionRepoPullRequestCreate) {
			continue
		}
		existingTarget, err := ParseDraftPRScope(content.Scope)
		if err != nil {
			continue
		}
		if !sameIssueScanDraftPRAuthorityRunnerTarget(existingTarget, requestContext) {
			continue
		}
		derived, ready, err := r.issueScanDraftPRAuthorityRequestContext(runID, existingTarget.BaseRef, existingTarget.BaseSHA, existingTarget.SingleUseNonce)
		if err != nil {
			return result, false, err
		}
		if !ready {
			return result, false, nil
		}
		if !sameIssueScanDraftPRAuthorityTarget(existingTarget, derived.DraftPRTarget) {
			continue
		}
		result.RunID = derived.RunID
		result.FactoryOrderID = derived.FactoryOrderID
		result.Repository = derived.Repository
		result.RequestID = content.RequestID
		result.DraftPRTarget = existingTarget
		result.AlreadyRaised = true
		return result, true, nil
	}
	return result, false, nil
}

func sameIssueScanDraftPRAuthorityRunnerTarget(target DraftPRTarget, requestContext IssueScanDraftPRAuthorityRequestRunnerContext) bool {
	return strings.EqualFold(strings.TrimSpace(target.Repository), strings.TrimSpace(requestContext.Repository)) &&
		strings.TrimSpace(target.HeadRef) == strings.TrimSpace(requestContext.OperateBranch) &&
		strings.TrimSpace(target.HeadSHA) == strings.TrimSpace(requestContext.OperateCommit)
}

func (r *Runtime) issueScanDraftPRAuthorityRequestRunnerContext(runID string) (IssueScanDraftPRAuthorityRequestRunnerContext, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, fmt.Errorf("run_id is required")
	}
	content, orderID, _, readyStage, err := r.issueScanReadyStageTarget(runID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(readyStage.TaskID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	if stageCompleted {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, nil
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	blockerStage, err := r.issueScanStageTargetByStageID(drafts, "drive_blockers_to_zero", orderID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	blockerCompleted, err := r.issueScanStageTaskCompleted(blockerStage.TaskID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	if !blockerCompleted {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, nil
	}
	implementationTaskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, nil
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	implementation, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, readyStage.TaskID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	if !ok {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, nil
	}
	if _, ready, err := r.issueScanReadyStageEvidence(content, orderID, implementationTaskID, blockerStage, readyStage); err != nil || ready {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	readyArtifacts, err := r.tasks.ListArtifacts(readyStage.TaskID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, fmt.Errorf("list ready stage artifacts: %w", err)
	}
	draftReceipts, err := issueScanDraftPRReceiptArtifacts(readyArtifacts)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	if len(draftReceipts) > 0 {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, nil
	}
	repo, err := issueScanReadyRunnerRepository(content)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	var repoPath string
	var roots []string
	if strings.TrimSpace(r.repoPath) != "" || strings.TrimSpace(r.repoWorkspaceRoot) != "" {
		resolved, err := r.resolveIssueScanWorkspaceForRepo(repo)
		if err != nil {
			return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, fmt.Errorf("resolve draft PR authority request workspace for %s: %w", repo, err)
		}
		repoPath = resolved.RepoPath
		roots = append([]string(nil), resolved.ContainmentWatchRoots...)
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestRunnerContext{}, false, err
	}
	return IssueScanDraftPRAuthorityRequestRunnerContext{
		Kind:                  issueScanDraftPRAuthorityRequestRunnerContextKind,
		LifecycleVersion:      issueScanLifecycleVersion,
		RunID:                 strings.TrimSpace(content.RunID),
		FactoryOrderID:        orderID,
		Repository:            repo,
		RepoPath:              repoPath,
		ContainmentWatchRoots: roots,
		ReadyStageTaskID:      readyStage.TaskID.Value(),
		BlockerStageTaskID:    blockerStage.TaskID.Value(),
		ImplementationTaskID:  implementationTaskID.Value(),
		SelectedIssue:         issueScanStageRoleOutputIssueFromBriefIssue(brief.SelectedIssue),
		OperateBranch:         implementation.OperateBranch,
		OperateCommit:         implementation.OperateCommit,
		OperateRange:          implementation.OperateRange,
		ChangedFilesSummary:   implementation.ChangedFilesSummary,
		BoundaryDisclaimers: compactStrings([]string{
			"authority requester may raise a draft PR creation request only",
			"authority request is not Human approval",
			"authority request is not PR creation or ready-for-review state",
			"authority request is not merge or deploy authorization",
			"approved head_sha must match operate_commit before draft PR creation",
			"base branch may advance before draft PR creation; head_sha is the pinned authority invariant",
		}),
	}, true, nil
}
