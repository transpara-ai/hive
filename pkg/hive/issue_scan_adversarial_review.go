package hive

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const (
	IssueScanAdversarialReviewReceiptArtifactLabel = "issue_scan_adversarial_review_receipt"
	issueScanAdversarialReviewReceiptArtifactKind  = "issue_scan_adversarial_review_receipt"
)

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
	RunID                  string
	FactoryOrderID         string
	Repository             string
	ImplementationTaskID   types.EventID
	ReviewStageTaskID      types.EventID
	ReceiptArtifactID      types.EventID
	ReviewEventID          types.EventID
	ReviewRef              string
	ReviewedHeadSHA        string
	Verdict                string
	Recorded               bool
	ReceiptAlreadyRecorded bool
	ReviewAlreadyRecorded  bool
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
		causes := compactEventIDs([]types.EventID{request.ID(), implementationTaskID, implementationStage.TaskID, reviewStage.TaskID, completion.CompletionEventID, completion.OperateArtifactID})
		if err := r.tasks.AddArtifact(r.humanID, implementationTaskID, IssueScanAdversarialReviewReceiptArtifactLabel, issueScanStageRuntimeEvidenceMediaType, body, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
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
		return result, nil
	}
	reviewContent := event.CodeReviewContent{
		TaskID:     implementationTaskID.Value(),
		Verdict:    normalized.Verdict,
		Summary:    issueScanAdversarialReviewEventSummary(normalized, receiptArtifactID),
		Issues:     append([]string(nil), normalized.Issues...),
		Confidence: normalized.Confidence,
	}
	causes := compactEventIDs([]types.EventID{request.ID(), implementationTaskID, implementationStage.TaskID, reviewStage.TaskID, completion.CompletionEventID, completion.OperateArtifactID, receiptArtifactID})
	ev, err := r.factory.Create(event.EventTypeCodeReviewSubmitted, r.humanID, reviewContent, causes, runLaunchConversationID(content.RunID, r.convID), r.store, r.signer)
	if err != nil {
		return result, fmt.Errorf("create code.review.submitted: %w", err)
	}
	stored, err := r.store.Append(ev)
	if err != nil {
		return result, fmt.Errorf("append code.review.submitted: %w", err)
	}
	result.ReviewEventID = stored.ID()
	result.Recorded = true
	return result, nil
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
