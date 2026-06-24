package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

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
		return content.RequestID, true, nil
	}
	return types.EventID{}, false, nil
}
