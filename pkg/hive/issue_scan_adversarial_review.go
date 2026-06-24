package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	IssueScanAdversarialReviewReceiptArtifactLabel = "issue_scan_adversarial_review_receipt"
	issueScanAdversarialReviewReceiptArtifactKind  = "issue_scan_adversarial_review_receipt"
	issueScanAdversarialReviewContextKind          = "issue_scan_adversarial_review_context"
)

// IssueScanAdversarialReviewIssue summarizes the selected issue for an external
// review runner without exposing the package-internal issue-scan brief type.
type IssueScanAdversarialReviewIssue struct {
	Repo   string   `json:"repo"`
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Labels []string `json:"labels,omitempty"`
}

// IssueScanAdversarialReviewContext is the exact-head packet sent to an
// external/adversarial review runner. The runner must return an
// IssueScanAdversarialReviewReceipt; RecordIssueScanAdversarialReview performs
// the authoritative validation before any review event is stored.
type IssueScanAdversarialReviewContext struct {
	Kind                  string                            `json:"kind"`
	LifecycleVersion      string                            `json:"lifecycle_version"`
	RunID                 string                            `json:"run_id"`
	FactoryOrderID        string                            `json:"factory_order_id"`
	Repository            string                            `json:"repository"`
	SelectedIssue         IssueScanAdversarialReviewIssue   `json:"selected_issue"`
	ImplementationTaskID  string                            `json:"implementation_task_id"`
	ReviewStageTaskID     string                            `json:"review_stage_task_id"`
	RepoPath              string                            `json:"repo_path,omitempty"`
	ContainmentWatchRoots []string                          `json:"containment_watch_roots,omitempty"`
	OperateBranch         string                            `json:"operate_branch"`
	OperateCommit         string                            `json:"operate_commit"`
	OperateRange          string                            `json:"operate_range,omitempty"`
	ChangedFilesSummary   string                            `json:"changed_files_summary"`
	ExpectedReceipt       IssueScanAdversarialReviewReceipt `json:"expected_receipt"`
	SourceRefs            []string                          `json:"source_refs,omitempty"`
	BoundaryDisclaimers   []string                          `json:"boundary_disclaimers,omitempty"`
}

// IssueScanAdversarialReviewRunner invokes an external exact-head review tool.
// Runtime records the returned receipt only after validating it against the
// verified Operate completion evidence for the implementation task.
type IssueScanAdversarialReviewRunner func(context.Context, IssueScanAdversarialReviewContext) (IssueScanAdversarialReviewReceipt, error)

// IssueScanAdversarialReviewReceipt is a durable external review packet for
// the run_adversarial_review stage. It proves only that an exact-head review was
// recorded against a verified Operate commit; blocker repair, PR readiness,
// Human approval, merge, and deploy remain separate governed stages.
type IssueScanAdversarialReviewReceipt struct {
	Kind                string   `json:"kind,omitempty"`
	LifecycleVersion    string   `json:"lifecycle_version,omitempty"`
	RunID               string   `json:"run_id,omitempty"`
	FactoryOrderID      string   `json:"factory_order_id,omitempty"`
	Repository          string   `json:"repository"`
	TaskID              string   `json:"task_id,omitempty"`
	ReviewRef           string   `json:"review_ref"`
	ReviewedHeadSHA     string   `json:"reviewed_head_sha"`
	Verdict             string   `json:"verdict"`
	Summary             string   `json:"summary"`
	Issues              []string `json:"issues"`
	Confidence          float64  `json:"confidence"`
	Tool                string   `json:"tool,omitempty"`
	ReviewStartedAt     string   `json:"review_started_at,omitempty"`
	ReviewCompletedAt   string   `json:"review_completed_at,omitempty"`
	SourceRefs          []string `json:"source_refs,omitempty"`
	BoundaryDisclaimers []string `json:"boundary_disclaimers,omitempty"`
}

// IssueScanAdversarialReviewRecordResult summarizes an exact-head review
// receipt append and the corresponding code.review.submitted event.
type IssueScanAdversarialReviewRecordResult struct {
	RunID                      string
	FactoryOrderID             string
	Repository                 string
	ImplementationTaskID       types.EventID
	ReviewStageTaskID          types.EventID
	ReceiptArtifactID          types.EventID
	ReviewEventID              types.EventID
	ReviewRef                  string
	ReviewedHeadSHA            string
	Verdict                    string
	Recorded                   bool
	ReopenedImplementationTask bool
	ReceiptAlreadyRecorded     bool
	ReviewAlreadyRecorded      bool
}

// IssueScanAdversarialReviewReceiptBody serializes a receipt for durable Work
// artifact storage.
func IssueScanAdversarialReviewReceiptBody(receipt IssueScanAdversarialReviewReceipt) (string, error) {
	receipt.Kind = valueOr(receipt.Kind, issueScanAdversarialReviewReceiptArtifactKind)
	receipt.LifecycleVersion = valueOr(receipt.LifecycleVersion, issueScanLifecycleVersion)
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan adversarial review receipt: %w", err)
	}
	return string(encoded), nil
}

// IssueScanAdversarialReviewRunContext returns the exact-head packet that a
// configured review runner should inspect before producing a receipt.
func (r *Runtime) IssueScanAdversarialReviewRunContext(runID string) (IssueScanAdversarialReviewContext, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	if len(requests) == 0 {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("queued run %q is not an issue-scan run", runID)
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("dispatch queued issue-scan run %q before review context creation: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	reviewStage, err := r.issueScanStageTargetByStageID(drafts, "run_adversarial_review", orderID)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	if err := r.verifyIssueScanStageTaskContracts(reviewStage); err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	implementationTaskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("implementation task for FactoryOrder %q has not been created", orderID)
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	completion, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, implementationStage.TaskID)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	if !ok {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("implementation task %s has no verified Operate completion evidence", implementationTaskID.Value())
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanAdversarialReviewContext{}, err
	}
	repo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if repo == "" && len(content.TargetRepos) > 0 {
		repo = strings.ToLower(strings.TrimSpace(content.TargetRepos[0]))
	}
	if !ValidTransparaAIRepo(repo) {
		return IssueScanAdversarialReviewContext{}, fmt.Errorf("selected issue repository %q is not a Transpara-AI repo", repo)
	}
	receipt := IssueScanAdversarialReviewReceipt{
		Kind:             issueScanAdversarialReviewReceiptArtifactKind,
		LifecycleVersion: issueScanLifecycleVersion,
		RunID:            strings.TrimSpace(content.RunID),
		FactoryOrderID:   orderID,
		Repository:       repo,
		TaskID:           implementationTaskID.Value(),
		ReviewedHeadSHA:  completion.OperateCommit,
		Issues:           []string{},
	}
	reviewContext := IssueScanAdversarialReviewContext{
		Kind:                 issueScanAdversarialReviewContextKind,
		LifecycleVersion:     issueScanLifecycleVersion,
		RunID:                strings.TrimSpace(content.RunID),
		FactoryOrderID:       orderID,
		Repository:           repo,
		SelectedIssue:        issueScanAdversarialReviewIssueFromBrief(brief.SelectedIssue),
		ImplementationTaskID: implementationTaskID.Value(),
		ReviewStageTaskID:    reviewStage.TaskID.Value(),
		OperateBranch:        completion.OperateBranch,
		OperateCommit:        completion.OperateCommit,
		OperateRange:         completion.OperateRange,
		ChangedFilesSummary:  completion.ChangedFilesSummary,
		ExpectedReceipt:      receipt,
		SourceRefs:           compactStrings([]string{requests[0].ID().Value(), implementationTaskID.Value(), implementationStage.TaskID.Value(), reviewStage.TaskID.Value(), completion.CompletionEventID.Value(), completion.OperateArtifactID.Value()}),
		BoundaryDisclaimers: compactStrings([]string{
			"review runner output is not Human approval",
			"review runner output is not merge or deploy authorization",
			"reviewed_head_sha must match operate_commit",
		}),
	}
	if resolved, err := r.resolveIssueScanWorkspaceForRepo(repo); err == nil {
		reviewContext.RepoPath = resolved.RepoPath
		reviewContext.ContainmentWatchRoots = append([]string(nil), resolved.ContainmentWatchRoots...)
	}
	return reviewContext, nil
}

// RunConfiguredIssueScanAdversarialReviews invokes the optional runtime-level
// exact-head review runner for issue-scan runs that have completed
// implementation evidence and no current review for that completion.
func (r *Runtime) RunConfiguredIssueScanAdversarialReviews(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanAdversarialReviewRecordResult, error) {
	if r == nil || r.issueScanAdversarialReviewRunner == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanAdversarialReviewRecordResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RunConfiguredIssueScanAdversarialReview(ctx, runID)
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

// RunConfiguredIssueScanAdversarialReview runs the configured review command
// for one issue-scan run, then records its returned receipt through the same
// exact-head validation path used by the manual CLI command.
func (r *Runtime) RunConfiguredIssueScanAdversarialReview(ctx context.Context, runID string) (IssueScanAdversarialReviewRecordResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanAdversarialReviewRecordResult{RunID: runID}
	if r == nil || r.issueScanAdversarialReviewRunner == nil {
		return result, false, nil
	}
	shouldRun, err := r.issueScanAdversarialReviewRunnerShouldRun(runID)
	if err != nil || !shouldRun {
		return result, false, err
	}
	reviewContext, err := r.IssueScanAdversarialReviewRunContext(runID)
	if err != nil {
		return result, false, err
	}
	receipt, err := r.issueScanAdversarialReviewRunner(ctx, reviewContext)
	if err != nil {
		return result, true, err
	}
	recorded, err := r.RecordIssueScanAdversarialReview(runID, receipt)
	return recorded, true, err
}

func (r *Runtime) issueScanAdversarialReviewRunnerShouldRun(runID string) (bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return false, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return false, err
	}
	if len(requests) == 0 {
		return false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return false, fmt.Errorf("dispatch queued issue-scan run %q before review runner: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return false, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return false, err
	}
	reviewStage, err := r.issueScanStageTargetByStageID(drafts, "run_adversarial_review", orderID)
	if err != nil {
		return false, err
	}
	if err := r.verifyIssueScanStageTaskContracts(reviewStage); err != nil {
		return false, err
	}
	implementationStageCompleted, err := r.issueScanStageTaskCompleted(implementationStage.TaskID)
	if err != nil {
		return false, err
	}
	if !implementationStageCompleted {
		return false, nil
	}
	implementationTaskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return false, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return false, nil
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return false, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	completion, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, implementationStage.TaskID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("completed implementation stage %s has no verified Operate completion evidence", implementationStage.TaskID.Value())
	}
	_, _, reviewAt, hasReview, err := r.latestIssueScanCodeReviewForTask(implementationTaskID)
	if err != nil {
		return false, err
	}
	if hasReview && (completion.CompletionTimestamp.IsZero() || !reviewAt.Before(completion.CompletionTimestamp)) {
		return false, nil
	}
	return true, nil
}

// RecordIssueScanAdversarialReview records an external/adversarial exact-head
// review as both a typed receipt artifact and the code.review.submitted event
// consumed by the issue-scan lifecycle. The reviewed head must match the
// verified Operate commit for the concrete implementation task.
func (r *Runtime) RecordIssueScanAdversarialReview(runID string, receipt IssueScanAdversarialReviewReceipt) (IssueScanAdversarialReviewRecordResult, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanAdversarialReviewRecordResult{RunID: runID}
	if r == nil || r.store == nil || r.tasks == nil || r.factory == nil || r.signer == nil {
		return result, fmt.Errorf("runtime store, task store, event factory, and signer are required")
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
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return result, fmt.Errorf("dispatch queued issue-scan run %q before review receipt recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return result, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return result, err
	}
	reviewStage, err := r.issueScanStageTargetByStageID(drafts, "run_adversarial_review", orderID)
	if err != nil {
		return result, err
	}
	if err := r.verifyIssueScanStageTaskContracts(reviewStage); err != nil {
		return result, err
	}
	implementationTaskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return result, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return result, fmt.Errorf("implementation task for FactoryOrder %q has not been created", orderID)
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return result, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	result.ImplementationTaskID = implementationTaskID
	result.ReviewStageTaskID = reviewStage.TaskID
	completion, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, implementationStage.TaskID)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, fmt.Errorf("implementation task %s has no verified Operate completion evidence", implementationTaskID.Value())
	}
	normalized, err := normalizeIssueScanAdversarialReviewReceipt(content, orderID, implementationTaskID, completion, receipt)
	if err != nil {
		return result, err
	}
	result.Repository = normalized.Repository
	result.ReviewRef = normalized.ReviewRef
	result.ReviewedHeadSHA = normalized.ReviewedHeadSHA
	result.Verdict = normalized.Verdict
	body, err := IssueScanAdversarialReviewReceiptBody(normalized)
	if err != nil {
		return result, err
	}
	receiptArtifactID, exists, err := r.findIssueScanAdversarialReviewReceiptArtifactID(implementationTaskID, body)
	if err != nil {
		return result, err
	}
	if !exists {
		artifactCauses := compactEventIDs([]types.EventID{request.ID(), implementationTaskID, implementationStage.TaskID, reviewStage.TaskID, completion.CompletionEventID, completion.OperateArtifactID})
		if err := r.tasks.AddArtifact(r.humanID, implementationTaskID, IssueScanAdversarialReviewReceiptArtifactLabel, issueScanStageRuntimeEvidenceMediaType, body, artifactCauses, runLaunchConversationID(content.RunID, r.convID)); err != nil {
			return result, fmt.Errorf("record issue-scan adversarial review receipt: %w", err)
		}
		receiptArtifactID, exists, err = r.findIssueScanAdversarialReviewReceiptArtifactID(implementationTaskID, body)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf("issue-scan adversarial review receipt artifact was not found after append")
		}
		result.Recorded = true
	} else {
		result.ReceiptAlreadyRecorded = true
	}
	result.ReceiptArtifactID = receiptArtifactID

	reviewEventID, exists, err := r.findIssueScanCodeReviewEventForReceipt(implementationTaskID, receiptArtifactID)
	if err != nil {
		return result, err
	}
	if exists {
		result.ReviewEventID = reviewEventID
		result.ReviewAlreadyRecorded = true
		reopened, err := r.reopenIssueScanImplementationTaskForReviewBlockers(content, implementationTaskID, normalized, compactEventIDs([]types.EventID{request.ID(), implementationTaskID, implementationStage.TaskID, reviewStage.TaskID, completion.CompletionEventID, completion.OperateArtifactID, receiptArtifactID, reviewEventID}))
		if err != nil {
			return result, err
		}
		result.ReopenedImplementationTask = reopened
		return result, nil
	}
	reviewContent := event.CodeReviewContent{
		TaskID:     implementationTaskID.Value(),
		Verdict:    normalized.Verdict,
		Summary:    issueScanAdversarialReviewEventSummary(normalized, receiptArtifactID),
		Issues:     append([]string(nil), normalized.Issues...),
		Confidence: normalized.Confidence,
	}
	reviewCauses := compactEventIDs([]types.EventID{request.ID(), implementationTaskID, implementationStage.TaskID, reviewStage.TaskID, completion.CompletionEventID, completion.OperateArtifactID, receiptArtifactID})
	ev, err := r.factory.Create(event.EventTypeCodeReviewSubmitted, r.humanID, reviewContent, reviewCauses, runLaunchConversationID(content.RunID, r.convID), r.store, r.signer)
	if err != nil {
		return result, fmt.Errorf("create code.review.submitted: %w", err)
	}
	stored, err := r.store.Append(ev)
	if err != nil {
		return result, fmt.Errorf("append code.review.submitted: %w", err)
	}
	result.ReviewEventID = stored.ID()
	result.Recorded = true
	reopened, err := r.reopenIssueScanImplementationTaskForReviewBlockers(content, implementationTaskID, normalized, compactEventIDs(append(reviewCauses, stored.ID())))
	if err != nil {
		return result, err
	}
	result.ReopenedImplementationTask = reopened
	return result, nil
}

func (r *Runtime) reopenIssueScanImplementationTaskForReviewBlockers(content FactoryRunRequestedContent, implementationTaskID types.EventID, receipt IssueScanAdversarialReviewReceipt, causes []types.EventID) (bool, error) {
	if strings.TrimSpace(receipt.Verdict) != "request_changes" {
		return false, nil
	}
	status, err := r.tasks.GetCompatibilityStatus(implementationTaskID)
	if err != nil {
		return false, fmt.Errorf("read implementation task status before blocker reopen: %w", err)
	}
	if status != work.LegacyStatusCompleted {
		return false, nil
	}
	issues := compactStrings(receipt.Issues)
	if len(issues) == 0 {
		return false, fmt.Errorf("request_changes review %q has no issues to reopen", receipt.ReviewRef)
	}
	reason := fmt.Sprintf("request_changes from exact-head issue-scan adversarial review %s at %s: %s", receipt.ReviewRef, receipt.ReviewedHeadSHA, receipt.Summary)
	if err := r.tasks.Reopen(r.humanID, implementationTaskID, reason, issues, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
		return false, fmt.Errorf("reopen implementation task for issue-scan blockers: %w", err)
	}
	return true, nil
}

func normalizeIssueScanAdversarialReviewReceipt(content FactoryRunRequestedContent, orderID string, implementationTaskID types.EventID, completion issueScanOperateCompletionEvidence, receipt IssueScanAdversarialReviewReceipt) (IssueScanAdversarialReviewReceipt, error) {
	if strings.TrimSpace(receipt.Kind) != "" && strings.TrimSpace(receipt.Kind) != issueScanAdversarialReviewReceiptArtifactKind {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("kind %q does not match %q", receipt.Kind, issueScanAdversarialReviewReceiptArtifactKind)
	}
	if strings.TrimSpace(receipt.LifecycleVersion) != "" && strings.TrimSpace(receipt.LifecycleVersion) != issueScanLifecycleVersion {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("lifecycle_version %q does not match %q", receipt.LifecycleVersion, issueScanLifecycleVersion)
	}
	if strings.TrimSpace(receipt.RunID) != "" && strings.TrimSpace(receipt.RunID) != strings.TrimSpace(content.RunID) {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("run_id %q does not match %q", receipt.RunID, content.RunID)
	}
	if strings.TrimSpace(receipt.FactoryOrderID) != "" && strings.TrimSpace(receipt.FactoryOrderID) != orderID {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("factory_order_id %q does not match %q", receipt.FactoryOrderID, orderID)
	}
	repo := strings.ToLower(strings.TrimSpace(receipt.Repository))
	if !ValidTransparaAIRepo(repo) {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("repository %q is not a Transpara-AI repo", receipt.Repository)
	}
	if len(content.TargetRepos) > 0 && !containsIssueScanString(content.TargetRepos, repo) {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("repository %q is outside issue-scan target repos %v", repo, content.TargetRepos)
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanAdversarialReviewReceipt{}, err
	}
	selectedRepo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if selectedRepo != "" && repo != selectedRepo {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("repository %q does not match selected issue repository %q", repo, selectedRepo)
	}
	if strings.TrimSpace(receipt.TaskID) != "" && strings.TrimSpace(receipt.TaskID) != implementationTaskID.Value() {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("task_id %q does not match implementation task %q", receipt.TaskID, implementationTaskID.Value())
	}
	reviewRef := strings.TrimSpace(receipt.ReviewRef)
	if reviewRef == "" {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("review_ref is required")
	}
	head := strings.TrimSpace(receipt.ReviewedHeadSHA)
	if head == "" {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("reviewed_head_sha is required")
	}
	if !strings.EqualFold(head, completion.OperateCommit) {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("reviewed_head_sha %q does not match implementation commit %q", head, completion.OperateCommit)
	}
	verdict := strings.ToLower(strings.TrimSpace(receipt.Verdict))
	switch verdict {
	case "approve", "request_changes", "reject":
	default:
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("verdict %q is not valid", receipt.Verdict)
	}
	summary := strings.TrimSpace(receipt.Summary)
	if summary == "" {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("summary is required")
	}
	if receipt.Issues == nil {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("issues must be present; use [] when no blockers remain")
	}
	issues := compactStrings(receipt.Issues)
	if verdict == "approve" && len(issues) > 0 {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("approve verdict requires zero issues")
	}
	if verdict != "approve" && len(issues) == 0 {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("%s verdict requires at least one issue", verdict)
	}
	if receipt.Confidence < 0.5 || receipt.Confidence > 1.0 {
		return IssueScanAdversarialReviewReceipt{}, fmt.Errorf("confidence %.2f must be between 0.50 and 1.00", receipt.Confidence)
	}
	receipt.Kind = issueScanAdversarialReviewReceiptArtifactKind
	receipt.LifecycleVersion = issueScanLifecycleVersion
	receipt.RunID = strings.TrimSpace(content.RunID)
	receipt.FactoryOrderID = orderID
	receipt.Repository = repo
	receipt.TaskID = implementationTaskID.Value()
	receipt.ReviewRef = reviewRef
	receipt.ReviewedHeadSHA = head
	receipt.Verdict = verdict
	receipt.Summary = summary
	receipt.Issues = issues
	receipt.Tool = strings.TrimSpace(receipt.Tool)
	receipt.ReviewStartedAt = strings.TrimSpace(receipt.ReviewStartedAt)
	receipt.ReviewCompletedAt = strings.TrimSpace(receipt.ReviewCompletedAt)
	receipt.SourceRefs = compactStrings(append(receipt.SourceRefs, implementationTaskID.Value(), completion.CompletionEventID.Value(), completion.OperateArtifactID.Value(), completion.OperateCommit, reviewRef))
	receipt.BoundaryDisclaimers = compactStrings(append(receipt.BoundaryDisclaimers,
		"review receipt is not Human approval",
		"review receipt is not merge or deploy authorization",
	))
	return receipt, nil
}

func issueScanAdversarialReviewEventSummary(receipt IssueScanAdversarialReviewReceipt, receiptArtifactID types.EventID) string {
	var b strings.Builder
	fmt.Fprintf(&b, "External exact-head adversarial review %s for %s at %s: %s", receipt.ReviewRef, receipt.TaskID, receipt.ReviewedHeadSHA, receipt.Summary)
	if receipt.Tool != "" {
		fmt.Fprintf(&b, " tool=%s", receipt.Tool)
	}
	if receiptArtifactID != (types.EventID{}) {
		fmt.Fprintf(&b, " receipt=%s", receiptArtifactID.Value())
	}
	return strings.TrimSpace(b.String())
}

func (r *Runtime) findIssueScanAdversarialReviewReceiptArtifactID(taskID types.EventID, body string) (types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("list implementation task artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Label) == IssueScanAdversarialReviewReceiptArtifactLabel && strings.TrimSpace(artifact.Body) == strings.TrimSpace(body) {
			return artifact.ID, true, nil
		}
	}
	return types.EventID{}, false, nil
}

func (r *Runtime) findIssueScanCodeReviewEventForReceipt(taskID, receiptArtifactID types.EventID) (types.EventID, bool, error) {
	page, err := r.store.ByType(event.EventTypeCodeReviewSubmitted, 1000, types.None[types.Cursor]())
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("code.review.submitted: %w", err)
	}
	var latest types.EventID
	for _, ev := range page.Items() {
		content, ok := ev.Content().(event.CodeReviewContent)
		if !ok || strings.TrimSpace(content.TaskID) != taskID.Value() {
			continue
		}
		if !containsIssueScanEventID(ev.Causes(), receiptArtifactID) {
			continue
		}
		if latest == (types.EventID{}) || ev.ID().Value() > latest.Value() {
			latest = ev.ID()
		}
	}
	if latest == (types.EventID{}) {
		return types.EventID{}, false, nil
	}
	return latest, true, nil
}

func containsIssueScanEventID(values []types.EventID, want types.EventID) bool {
	if want == (types.EventID{}) {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func issueScanAdversarialReviewIssueFromBrief(issue issueScanBriefIssuePayload) IssueScanAdversarialReviewIssue {
	return IssueScanAdversarialReviewIssue{
		Repo:   strings.ToLower(strings.TrimSpace(issue.Repo)),
		Number: issue.Number,
		Title:  strings.TrimSpace(issue.Title),
		URL:    strings.TrimSpace(issue.URL),
		Labels: compactStrings(issue.Labels),
	}
}
