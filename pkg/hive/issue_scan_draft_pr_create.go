package hive

import (
	"context"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// IssueScanDraftPRCreationResult summarizes creation of the draft PR that will
// later be marked ready and reviewed before Human merge approval. This result
// proves draft PR creation and ready-stage receipt recording only.
type IssueScanDraftPRCreationResult struct {
	RunID                    string
	FactoryOrderID           string
	Repository               string
	RequestID                string
	WorkTaskID               types.EventID
	DraftPRReceipt           IssueScanDraftPRReceiptRecordResult
	PRNumber                 int
	PRURL                    string
	HeadSHA                  string
	Created                  bool
	NoReadyReviewMergeDeploy bool
}

// CreateIssueScanDraftPRFromApprovedRequest consumes an approved draft-PR
// authority request for a completed zero-blocker issue-scan run, creates exactly
// one draft PR through the existing Transpara-AI PR creator, and records the
// resulting receipt on the terminal ready stage. It never marks the PR ready,
// approves, merges, deploys, or completes the ready-for-Human stage.
func (r *Runtime) CreateIssueScanDraftPRFromApprovedRequest(ctx context.Context, runID, requestID string, client work.Epic11PullRequestCreator) (IssueScanDraftPRCreationResult, error) {
	runID = strings.TrimSpace(runID)
	requestID = strings.TrimSpace(requestID)
	result := IssueScanDraftPRCreationResult{RunID: runID, RequestID: requestID}
	if r == nil || r.store == nil || r.tasks == nil {
		return result, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return result, fmt.Errorf("run_id is required")
	}
	if requestID == "" {
		return result, fmt.Errorf("request_id is required")
	}
	if client == nil {
		return result, fmt.Errorf("pull request creator is required")
	}

	target, err := LoadApprovedDraftPRTarget(r.store, requestID)
	if err != nil {
		return result, err
	}
	requestContext, err := r.IssueScanDraftPRAuthorityRequestContext(runID, target.BaseRef, target.BaseSHA, target.SingleUseNonce)
	if err != nil {
		return result, err
	}
	if err := validateIssueScanApprovedDraftPRTarget(target, requestContext.DraftPRTarget); err != nil {
		return result, err
	}
	if err := VerifyDraftPRContent(target, requestContext.DraftPRTitle, requestContext.DraftPRBody); err != nil {
		return result, err
	}
	changedFiles, err := issueScanDraftPRChangedFiles(requestContext.ChangedFilesSummary)
	if err != nil {
		return result, err
	}

	requestEventID, err := types.NewEventID(requestID)
	if err != nil {
		return result, fmt.Errorf("parse request id: %w", err)
	}
	causes := compactEventIDs([]types.EventID{requestEventID})
	convID := runLaunchConversationID(requestContext.RunID, r.convID)
	run, err := CreateTransparaAIDraftPRFromApprovedDecision(ctx, r.tasks, r.humanID, convID, client, DraftPRArtifact{
		Target:         target,
		Title:          requestContext.DraftPRTitle,
		Body:           requestContext.DraftPRBody,
		ChangedFiles:   changedFiles,
		ActorRole:      "guardian",
		DeciderActorID: r.humanID.Value(),
		DeciderRole:    "human",
	}, causes...)
	if err != nil {
		return result, err
	}
	receiptResult, err := r.RecordIssueScanDraftPRReceipt(runID, run.Receipt)
	if err != nil {
		return result, err
	}
	result.FactoryOrderID = requestContext.FactoryOrderID
	result.Repository = requestContext.Repository
	result.WorkTaskID = run.WorkTask.ID
	result.DraftPRReceipt = receiptResult
	result.PRNumber = receiptResult.PRNumber
	result.PRURL = receiptResult.PRURL
	result.HeadSHA = receiptResult.HeadSHA
	result.Created = true
	result.NoReadyReviewMergeDeploy = true
	return result, nil
}

func validateIssueScanApprovedDraftPRTarget(approved, derived DraftPRTarget) error {
	approved, err := normalizeTransparaAIDraftPRTarget(approved)
	if err != nil {
		return fmt.Errorf("approved draft-PR target: %w", err)
	}
	derived, err = normalizeTransparaAIDraftPRTarget(derived)
	if err != nil {
		return fmt.Errorf("issue-scan run-derived draft-PR target: %w", err)
	}
	for _, check := range []struct {
		field    string
		approved string
		derived  string
	}{
		{"repository", approved.Repository, derived.Repository},
		{"base_ref", approved.BaseRef, derived.BaseRef},
		{"base_sha", approved.BaseSHA, derived.BaseSHA},
		{"head_ref", approved.HeadRef, derived.HeadRef},
		{"head_sha", approved.HeadSHA, derived.HeadSHA},
		{"title_hash", approved.TitleHash, derived.TitleHash},
		{"body_hash", approved.BodyHash, derived.BodyHash},
		{"policy_bundle_id", approved.PolicyBundleID, derived.PolicyBundleID},
		{"policy_bundle_hash", approved.PolicyBundleHash, derived.PolicyBundleHash},
		{"single_use_nonce", approved.SingleUseNonce, derived.SingleUseNonce},
	} {
		if strings.TrimSpace(check.approved) != strings.TrimSpace(check.derived) {
			return fmt.Errorf("approved draft-PR target %s %q does not match issue-scan run-derived %s %q", check.field, check.approved, check.field, check.derived)
		}
	}
	return nil
}

func issueScanDraftPRChangedFiles(summary string) ([]string, error) {
	lines := strings.Split(summary, "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		before, _, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		file := strings.TrimSpace(before)
		if file != "" {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("changed_files_summary does not contain repository-relative changed file paths")
	}
	return normalizePRChangedFiles(files)
}
