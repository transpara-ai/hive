package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

const issueScanDraftPRCreationRunnerContextKind = "issue_scan_draft_pr_creation_runner_context"

// IssueScanDraftPRCreationRunnerContext summarizes the approved draft-PR
// creation gate for probes and operators. It is read-only context; creation
// still requires RunConfiguredIssueScanDraftPRCreation with a configured
// creator, and it never marks a PR ready, approves, merges, or deploys.
type IssueScanDraftPRCreationRunnerContext struct {
	Kind                 string                        `json:"kind"`
	LifecycleVersion     string                        `json:"lifecycle_version"`
	RunID                string                        `json:"run_id"`
	FactoryOrderID       string                        `json:"factory_order_id"`
	Repository           string                        `json:"repository"`
	RequestID            string                        `json:"request_id"`
	ReadyStageTaskID     string                        `json:"ready_stage_task_id"`
	BlockerStageTaskID   string                        `json:"blocker_stage_task_id"`
	ImplementationTaskID string                        `json:"implementation_task_id"`
	SelectedIssue        IssueScanStageRoleOutputIssue `json:"selected_issue"`
	DraftPRTitle         string                        `json:"draft_pr_title"`
	DraftPRTarget        DraftPRTarget                 `json:"draft_pr_target"`
	ApprovedHeadSHA      string                        `json:"approved_head_sha"`
	ChangedFilesSummary  string                        `json:"changed_files_summary,omitempty"`
	BoundaryDisclaimers  []string                      `json:"boundary_disclaimers,omitempty"`
}

// RunConfiguredIssueScanDraftPRCreations creates approved issue-scan draft PRs
// for dispatched runs when the daemon is explicitly configured with a creator.
// It consumes only recorded approved authority decisions; without an approval it
// makes no GitHub call.
func (r *Runtime) RunConfiguredIssueScanDraftPRCreations(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanDraftPRCreationResult, error) {
	if r == nil || r.issueScanDraftPRCreator == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanDraftPRCreationResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		created, ready, err := r.RunConfiguredIssueScanDraftPRCreation(ctx, runID)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ready {
			out = append(out, created)
		}
	}
	return out, errors.Join(errs...)
}

func (r *Runtime) RunConfiguredIssueScanDraftPRCreation(ctx context.Context, runID string) (IssueScanDraftPRCreationResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanDraftPRCreationResult{RunID: runID}
	if r == nil || r.issueScanDraftPRCreator == nil {
		return result, false, nil
	}
	requestID, ready, err := r.approvedIssueScanDraftPRAuthorityRequestForRun(runID)
	if err != nil || !ready {
		return result, ready, err
	}
	created, err := r.CreateIssueScanDraftPRFromApprovedRequest(ctx, runID, requestID.Value(), r.issueScanDraftPRCreator)
	return created, true, err
}

func (r *Runtime) approvedIssueScanDraftPRAuthorityRequestForRun(runID string) (types.EventID, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil {
		return types.EventID{}, false, fmt.Errorf("runtime store is required")
	}
	if runID == "" {
		return types.EventID{}, false, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return types.EventID{}, false, err
	} else if parked {
		return types.EventID{}, false, nil
	}
	events, err := eventsByTypePaginated(r.store, EventTypeAuthorityDecisionRecorded, defaultOperatorProjectionLimit)
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("load authority decisions for issue-scan draft PR creation: %w", err)
	}
	for _, ev := range events {
		content, ok := ev.Content().(AuthorityDecisionRecordedContent)
		if !ok || strings.TrimSpace(content.Outcome) != draftPRApprovedOutcome || strings.TrimSpace(content.ApprovedAction) != string(safety.ActionRepoPullRequestCreate) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(content.DeciderRole), "human") {
			continue
		}
		target, err := ParseDraftPRScope(content.Scope)
		if err != nil {
			continue
		}
		requestContext, ready, err := r.issueScanDraftPRAuthorityRequestContext(runID, target.BaseRef, target.BaseSHA, target.SingleUseNonce)
		if err != nil || !ready {
			continue
		}
		if err := validateIssueScanApprovedDraftPRTarget(target, requestContext.DraftPRTarget); err != nil {
			continue
		}
		if content.RequestID.IsZero() {
			return types.EventID{}, false, fmt.Errorf("approved draft PR decision %s has empty request_id", ev.ID().Value())
		}
		if !content.ExpiresAt.IsZero() && !time.Now().Before(content.ExpiresAt.Value()) {
			return types.EventID{}, false, fmt.Errorf("approved draft PR decision %s for run %s expired at %s: refusing draft PR creation", ev.ID().Value(), runID, content.ExpiresAt.String())
		}
		return content.RequestID, true, nil
	}
	return types.EventID{}, false, nil
}

func (r *Runtime) issueScanDraftPRCreationRunnerContext(runID string) (IssueScanDraftPRCreationRunnerContext, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil {
		return IssueScanDraftPRCreationRunnerContext{}, false, fmt.Errorf("runtime store is required")
	}
	requestID, ready, err := r.approvedIssueScanDraftPRAuthorityRequestForRun(runID)
	if err != nil || !ready {
		return IssueScanDraftPRCreationRunnerContext{}, ready, err
	}
	target, err := LoadApprovedDraftPRTarget(r.store, requestID.Value())
	if err != nil {
		return IssueScanDraftPRCreationRunnerContext{}, false, err
	}
	requestContext, ready, err := r.issueScanDraftPRAuthorityRequestContext(runID, target.BaseRef, target.BaseSHA, target.SingleUseNonce)
	if err != nil || !ready {
		return IssueScanDraftPRCreationRunnerContext{}, ready, err
	}
	if err := validateIssueScanApprovedDraftPRTarget(target, requestContext.DraftPRTarget); err != nil {
		return IssueScanDraftPRCreationRunnerContext{}, false, err
	}
	return IssueScanDraftPRCreationRunnerContext{
		Kind:                 issueScanDraftPRCreationRunnerContextKind,
		LifecycleVersion:     issueScanLifecycleVersion,
		RunID:                requestContext.RunID,
		FactoryOrderID:       requestContext.FactoryOrderID,
		Repository:           requestContext.Repository,
		RequestID:            requestID.Value(),
		ReadyStageTaskID:     requestContext.ReadyStageTaskID,
		BlockerStageTaskID:   requestContext.BlockerStageTaskID,
		ImplementationTaskID: requestContext.ImplementationTaskID,
		SelectedIssue:        requestContext.SelectedIssue,
		DraftPRTitle:         requestContext.DraftPRTitle,
		DraftPRTarget:        target,
		ApprovedHeadSHA:      strings.TrimSpace(target.HeadSHA),
		ChangedFilesSummary:  requestContext.ChangedFilesSummary,
		BoundaryDisclaimers: compactStrings([]string{
			"draft PR creation requires recorded Human approval for this exact target",
			"draft PR creation records a draft PR receipt only",
			"draft PR creation is not ready-for-review state",
			"draft PR creation is not Human merge approval",
			"draft PR creation is not merge or deploy authorization",
			"approved head_sha must match operate_commit before PR creation",
		}),
	}, true, nil
}
