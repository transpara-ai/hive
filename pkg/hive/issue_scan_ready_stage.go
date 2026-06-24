package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	IssueScanReadyPREvidenceArtifactLabel = "issue_scan_ready_pr_evidence"
	issueScanReadyPREvidenceArtifactKind  = "issue_scan_ready_pr_evidence"
)

// IssueScanReadyPREvidence is the terminal evidence packet for a Civilization
// issue-scan run after the governed PR path has produced a ready-for-Human PR.
// It proves PR readiness only; it must not claim Human approval, merge, or
// deploy.
type IssueScanReadyPREvidence struct {
	Kind                   string   `json:"kind,omitempty"`
	LifecycleVersion       string   `json:"lifecycle_version,omitempty"`
	RunID                  string   `json:"run_id,omitempty"`
	FactoryOrderID         string   `json:"factory_order_id,omitempty"`
	Repository             string   `json:"repository"`
	PRNumber               int      `json:"pr_number"`
	PRURL                  string   `json:"pr_url"`
	BaseRef                string   `json:"base_ref,omitempty"`
	BaseSHA                string   `json:"base_sha,omitempty"`
	HeadRef                string   `json:"head_ref,omitempty"`
	HeadSHA                string   `json:"head_sha"`
	State                  string   `json:"state"`
	Draft                  bool     `json:"draft"`
	ReadyForReview         bool     `json:"ready_for_review"`
	MergeStateStatus       string   `json:"merge_state_status"`
	CIStatus               string   `json:"ci_status"`
	ReadyStateReviewRef    string   `json:"ready_state_review_ref"`
	ReadyStateReviewStatus string   `json:"ready_state_review_status"`
	HumanApprovalRequired  bool     `json:"human_approval_required"`
	DraftPRReceiptRef      string   `json:"draft_pr_receipt_ref,omitempty"`
	Summary                string   `json:"summary,omitempty"`
	SourceRefs             []string `json:"source_refs,omitempty"`
}

type issueScanReadyStageEvidence struct {
	ReadyPR                IssueScanReadyPREvidence
	ReadyPREvidenceID      types.EventID
	ReviewedTaskID         types.EventID
	ReadyStageTaskID       types.EventID
	BlockerStageTaskID     types.EventID
	BlockerRuntimeID       types.EventID
	ImplementationEvidence issueScanOperateCompletionEvidence
}

type issueScanDraftPRReceiptEvidence struct {
	ArtifactID types.EventID
	Receipt    TransparaAIDraftPRReceipt
}

// IssueScanReadyPREvidenceArtifactBody serializes a ready-PR evidence packet so
// operators or terminal PR machinery can attach it to the final lifecycle stage.
func IssueScanReadyPREvidenceArtifactBody(evidence IssueScanReadyPREvidence) (string, error) {
	evidence.Kind = valueOr(evidence.Kind, issueScanReadyPREvidenceArtifactKind)
	evidence.LifecycleVersion = valueOr(evidence.LifecycleVersion, issueScanLifecycleVersion)
	encoded, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal issue-scan ready PR evidence: %w", err)
	}
	return string(encoded), nil
}

// RecordCompletedIssueScanReadyRoleOutputs records the final
// surface_ready_for_Human_result_PR role-output artifacts after a typed
// ready-PR evidence artifact proves the PR is open, non-draft, reviewed, and
// still waiting on Human approval.
func (r *Runtime) RecordCompletedIssueScanReadyRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputResult, 0, len(runIDs)*3)
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RecordCompletedIssueScanReadyRoleOutput(runID)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ready {
			out = append(out, recorded...)
		}
	}
	return out, errors.Join(errs...)
}

func (r *Runtime) RecordCompletedIssueScanReadyRoleOutput(runID string) ([]IssueScanStageRoleOutputResult, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return nil, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return nil, false, err
	}
	if len(requests) == 0 {
		return nil, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return nil, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return nil, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return nil, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return nil, false, fmt.Errorf("dispatch queued issue-scan run %q before ready role-output recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, false, err
	}
	readyStage, err := r.issueScanStageTargetByStageID(drafts, "surface_ready_for_Human_result_PR", orderID)
	if err != nil {
		return nil, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(readyStage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if stageCompleted {
		return nil, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(readyStage); err != nil {
		return nil, false, err
	}
	blockerStage, err := r.issueScanStageTargetByStageID(drafts, "drive_blockers_to_zero", orderID)
	if err != nil {
		return nil, false, err
	}
	blockerCompleted, err := r.issueScanStageTaskCompleted(blockerStage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if !blockerCompleted {
		return nil, false, nil
	}
	taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return nil, false, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return nil, false, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	evidence, ready, err := r.issueScanReadyStageEvidence(content, orderID, taskID, blockerStage, readyStage)
	if err != nil || !ready {
		return nil, ready, err
	}
	results := make([]IssueScanStageRoleOutputResult, 0, 3)
	for _, output := range []IssueScanStageRoleOutputEvidence{
		issueScanReadyStrategistRoleOutput(evidence),
		issueScanReadyReviewerRoleOutput(evidence),
		issueScanReadyGuardianRoleOutput(evidence),
	} {
		recorded, err := r.RecordIssueScanStageRoleOutput(runID, "surface_ready_for_Human_result_PR", output)
		if err != nil {
			return results, false, err
		}
		results = append(results, recorded)
	}
	return results, true, nil
}

func (r *Runtime) issueScanReadyStageEvidence(content FactoryRunRequestedContent, orderID string, implementationTaskID types.EventID, blockerStage, readyStage *issueScanStageAdvanceTarget) (issueScanReadyStageEvidence, bool, error) {
	_, blockerRuntimeID, ok, err := r.issueScanStageRuntimeEvidenceForCompletedStage(content, orderID, blockerStage)
	if err != nil || !ok {
		return issueScanReadyStageEvidence{}, ok, err
	}
	implementation, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, readyStage.TaskID)
	if err != nil || !ok {
		return issueScanReadyStageEvidence{}, ok, err
	}
	readyPR, readyPRArtifactID, ok, err := r.issueScanReadyPREvidenceForStage(content, orderID, readyStage.TaskID, implementation)
	if err != nil || !ok {
		return issueScanReadyStageEvidence{}, ok, err
	}
	return issueScanReadyStageEvidence{
		ReadyPR:                readyPR,
		ReadyPREvidenceID:      readyPRArtifactID,
		ReviewedTaskID:         implementationTaskID,
		ReadyStageTaskID:       readyStage.TaskID,
		BlockerStageTaskID:     blockerStage.TaskID,
		BlockerRuntimeID:       blockerRuntimeID,
		ImplementationEvidence: implementation,
	}, true, nil
}

func (r *Runtime) issueScanReadyPREvidenceForStage(content FactoryRunRequestedContent, orderID string, stageTaskID types.EventID, implementation issueScanOperateCompletionEvidence) (IssueScanReadyPREvidence, types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(stageTaskID)
	if err != nil {
		return IssueScanReadyPREvidence{}, types.EventID{}, false, fmt.Errorf("list ready stage artifacts: %w", err)
	}
	draftReceipts, err := issueScanDraftPRReceiptArtifacts(artifacts)
	if err != nil {
		return IssueScanReadyPREvidence{}, types.EventID{}, false, err
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		parsed, ok, err := issueScanReadyPREvidenceArtifact(artifact.ID.Value(), artifact.Label, artifact.Body)
		if err != nil {
			return IssueScanReadyPREvidence{}, types.EventID{}, false, fmt.Errorf("parse ready PR evidence artifact %s: %w", artifact.ID.Value(), err)
		}
		if !ok {
			continue
		}
		normalized, err := normalizeIssueScanReadyPREvidence(content, orderID, parsed, implementation, draftReceipts)
		if err != nil {
			return IssueScanReadyPREvidence{}, types.EventID{}, false, fmt.Errorf("validate ready PR evidence artifact %s: %w", artifact.ID.Value(), err)
		}
		return normalized, artifact.ID, true, nil
	}
	return IssueScanReadyPREvidence{}, types.EventID{}, false, nil
}

func issueScanReadyPREvidenceArtifact(eventRef, label, body string) (IssueScanReadyPREvidence, bool, error) {
	if strings.TrimSpace(label) != IssueScanReadyPREvidenceArtifactLabel {
		return IssueScanReadyPREvidence{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return IssueScanReadyPREvidence{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload IssueScanReadyPREvidence
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return IssueScanReadyPREvidence{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanReadyPREvidenceArtifactKind {
		return IssueScanReadyPREvidence{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanReadyPREvidenceArtifactKind)
	}
	payload.SourceRefs = compactStrings(append([]string{strings.TrimSpace(eventRef)}, payload.SourceRefs...))
	return payload, true, nil
}

func issueScanDraftPRReceiptArtifacts(artifacts []work.ArtifactEvent) ([]issueScanDraftPRReceiptEvidence, error) {
	out := []issueScanDraftPRReceiptEvidence{}
	for _, artifact := range artifacts {
		receipt, ok, err := transparaAIDraftPRReceiptArtifact(artifact.ID.Value(), artifact.Label, artifact.Body)
		if err != nil {
			return nil, fmt.Errorf("parse draft PR receipt artifact %s: %w", artifact.ID.Value(), err)
		}
		if !ok {
			continue
		}
		out = append(out, issueScanDraftPRReceiptEvidence{ArtifactID: artifact.ID, Receipt: receipt})
	}
	return out, nil
}

func transparaAIDraftPRReceiptArtifact(_ string, label, body string) (TransparaAIDraftPRReceipt, bool, error) {
	if strings.TrimSpace(label) != TransparaAIDraftPRReceiptArtifactLabel {
		return TransparaAIDraftPRReceipt{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return TransparaAIDraftPRReceipt{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload TransparaAIDraftPRReceipt
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return TransparaAIDraftPRReceipt{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != transparaAIDraftPRReceiptKind {
		return TransparaAIDraftPRReceipt{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, transparaAIDraftPRReceiptKind)
	}
	payload.Repository = strings.ToLower(strings.TrimSpace(payload.Repository))
	payload.PRURL = strings.TrimSpace(payload.PRURL)
	payload.HeadSHA = strings.TrimSpace(payload.HeadSHA)
	return payload, true, nil
}

func normalizeIssueScanReadyPREvidence(content FactoryRunRequestedContent, orderID string, evidence IssueScanReadyPREvidence, implementation issueScanOperateCompletionEvidence, draftReceipts []issueScanDraftPRReceiptEvidence) (IssueScanReadyPREvidence, error) {
	if strings.TrimSpace(evidence.LifecycleVersion) != "" && strings.TrimSpace(evidence.LifecycleVersion) != issueScanLifecycleVersion {
		return IssueScanReadyPREvidence{}, fmt.Errorf("lifecycle_version %q does not match %q", evidence.LifecycleVersion, issueScanLifecycleVersion)
	}
	if strings.TrimSpace(evidence.RunID) != "" && strings.TrimSpace(evidence.RunID) != strings.TrimSpace(content.RunID) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("run_id %q does not match %q", evidence.RunID, content.RunID)
	}
	if strings.TrimSpace(evidence.FactoryOrderID) != "" && strings.TrimSpace(evidence.FactoryOrderID) != orderID {
		return IssueScanReadyPREvidence{}, fmt.Errorf("factory_order_id %q does not match %q", evidence.FactoryOrderID, orderID)
	}
	repo := strings.ToLower(strings.TrimSpace(evidence.Repository))
	if !ValidTransparaAIRepo(repo) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("repository %q is not a Transpara-AI repo", evidence.Repository)
	}
	if len(content.TargetRepos) > 0 && !containsIssueScanString(content.TargetRepos, repo) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("repository %q is outside issue-scan target repos %v", repo, content.TargetRepos)
	}
	if evidence.PRNumber <= 0 {
		return IssueScanReadyPREvidence{}, fmt.Errorf("pr_number is required")
	}
	url := strings.TrimSpace(evidence.PRURL)
	if url == "" {
		return IssueScanReadyPREvidence{}, fmt.Errorf("pr_url is required")
	}
	if !strings.Contains(strings.ToLower(url), "github.com/"+repo+"/pull/") {
		return IssueScanReadyPREvidence{}, fmt.Errorf("pr_url %q does not match repository %q", url, repo)
	}
	head := strings.TrimSpace(evidence.HeadSHA)
	if head == "" {
		return IssueScanReadyPREvidence{}, fmt.Errorf("head_sha is required")
	}
	if !strings.EqualFold(head, implementation.OperateCommit) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("ready PR head_sha %q does not match implementation commit %q", head, implementation.OperateCommit)
	}
	if strings.ToLower(strings.TrimSpace(evidence.State)) != "open" {
		return IssueScanReadyPREvidence{}, fmt.Errorf("state %q is not open", evidence.State)
	}
	if evidence.Draft || !evidence.ReadyForReview {
		return IssueScanReadyPREvidence{}, fmt.Errorf("PR must be non-draft and ready_for_review")
	}
	if !issueScanReadyStatusOK(evidence.MergeStateStatus, []string{"clean"}) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("merge_state_status %q is not clean", evidence.MergeStateStatus)
	}
	if !issueScanReadyStatusOK(evidence.CIStatus, []string{"success", "passed", "green"}) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("ci_status %q is not successful", evidence.CIStatus)
	}
	if strings.TrimSpace(evidence.ReadyStateReviewRef) == "" {
		return IssueScanReadyPREvidence{}, fmt.Errorf("ready_state_review_ref is required")
	}
	if !issueScanReadyStatusOK(evidence.ReadyStateReviewStatus, []string{"success", "passed", "pass", "no_blockers", "no blockers"}) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("ready_state_review_status %q is not passing", evidence.ReadyStateReviewStatus)
	}
	if !evidence.HumanApprovalRequired {
		return IssueScanReadyPREvidence{}, fmt.Errorf("human_approval_required must be true")
	}
	receipt, err := matchingIssueScanDraftPRReceipt(evidence, repo, head, draftReceipts)
	if err != nil {
		return IssueScanReadyPREvidence{}, err
	}
	evidence.Kind = issueScanReadyPREvidenceArtifactKind
	evidence.LifecycleVersion = issueScanLifecycleVersion
	evidence.RunID = strings.TrimSpace(content.RunID)
	evidence.FactoryOrderID = orderID
	evidence.Repository = repo
	evidence.PRURL = url
	evidence.PRNumber = receipt.Receipt.PRNumber
	evidence.BaseRef = valueOr(strings.TrimSpace(evidence.BaseRef), receipt.Receipt.BaseRef)
	evidence.BaseSHA = valueOr(strings.TrimSpace(evidence.BaseSHA), receipt.Receipt.BaseSHA)
	evidence.HeadRef = valueOr(strings.TrimSpace(evidence.HeadRef), receipt.Receipt.HeadRef)
	evidence.HeadSHA = head
	evidence.State = "open"
	evidence.ReadyForReview = true
	evidence.Draft = false
	evidence.MergeStateStatus = "clean"
	evidence.CIStatus = strings.ToLower(strings.TrimSpace(evidence.CIStatus))
	evidence.ReadyStateReviewStatus = strings.ToLower(strings.TrimSpace(evidence.ReadyStateReviewStatus))
	evidence.Summary = strings.TrimSpace(evidence.Summary)
	evidence.ReadyStateReviewRef = strings.TrimSpace(evidence.ReadyStateReviewRef)
	evidence.DraftPRReceiptRef = receipt.ArtifactID.Value()
	evidence.SourceRefs = compactStrings(append(evidence.SourceRefs, receipt.ArtifactID.Value(), receipt.Receipt.PRURL))
	return evidence, nil
}

func matchingIssueScanDraftPRReceipt(evidence IssueScanReadyPREvidence, repo, head string, receipts []issueScanDraftPRReceiptEvidence) (issueScanDraftPRReceiptEvidence, error) {
	if len(receipts) == 0 {
		return issueScanDraftPRReceiptEvidence{}, fmt.Errorf("%s artifact is required before ready PR evidence can complete the stage", TransparaAIDraftPRReceiptArtifactLabel)
	}
	wantRef := strings.TrimSpace(evidence.DraftPRReceiptRef)
	var lastMismatch error
	for _, receipt := range receipts {
		if wantRef != "" && receipt.ArtifactID.Value() != wantRef {
			continue
		}
		if err := validateIssueScanDraftPRReceiptForReadyEvidence(receipt, evidence, repo, head); err != nil {
			if wantRef != "" {
				return issueScanDraftPRReceiptEvidence{}, err
			}
			lastMismatch = err
			continue
		}
		return receipt, nil
	}
	if wantRef != "" {
		return issueScanDraftPRReceiptEvidence{}, fmt.Errorf("draft_pr_receipt_ref %q was not found", wantRef)
	}
	if lastMismatch != nil {
		return issueScanDraftPRReceiptEvidence{}, fmt.Errorf("no %s artifact matches ready PR evidence for %s#%d at head %s: %w", TransparaAIDraftPRReceiptArtifactLabel, repo, evidence.PRNumber, head, lastMismatch)
	}
	return issueScanDraftPRReceiptEvidence{}, fmt.Errorf("no %s artifact matches ready PR evidence for %s#%d at head %s", TransparaAIDraftPRReceiptArtifactLabel, repo, evidence.PRNumber, head)
}

func validateIssueScanDraftPRReceiptForReadyEvidence(receipt issueScanDraftPRReceiptEvidence, evidence IssueScanReadyPREvidence, repo, head string) error {
	r := receipt.Receipt
	if !ValidTransparaAIRepo(r.Repository) {
		return fmt.Errorf("draft PR receipt repository %q is not a Transpara-AI repo", r.Repository)
	}
	if r.Repository != repo {
		return fmt.Errorf("draft PR receipt repository %q does not match ready PR repository %q", r.Repository, repo)
	}
	if r.PRNumber <= 0 || r.PRNumber != evidence.PRNumber {
		return fmt.Errorf("draft PR receipt number %d does not match ready PR number %d", r.PRNumber, evidence.PRNumber)
	}
	if strings.TrimSpace(r.PRURL) == "" || strings.TrimSpace(r.PRURL) != strings.TrimSpace(evidence.PRURL) {
		return fmt.Errorf("draft PR receipt URL %q does not match ready PR URL %q", r.PRURL, evidence.PRURL)
	}
	if strings.TrimSpace(r.HeadSHA) != head {
		return fmt.Errorf("draft PR receipt head %q does not match ready PR head %q", r.HeadSHA, head)
	}
	if strings.TrimSpace(r.RemoteHeadSHA) != head {
		return fmt.Errorf("draft PR receipt remote head %q does not match ready PR head %q", r.RemoteHeadSHA, head)
	}
	files, err := normalizePRChangedFiles(r.ChangedFiles)
	if err != nil {
		return fmt.Errorf("draft PR receipt changed_files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("draft PR receipt changed_files is required")
	}
	for field, values := range map[string][2]string{
		"base_ref": {strings.TrimSpace(evidence.BaseRef), strings.TrimSpace(r.BaseRef)},
		"base_sha": {strings.TrimSpace(evidence.BaseSHA), strings.TrimSpace(r.BaseSHA)},
		"head_ref": {strings.TrimSpace(evidence.HeadRef), strings.TrimSpace(r.HeadRef)},
	} {
		if values[0] != "" && values[1] != "" && values[0] != values[1] {
			return fmt.Errorf("ready PR %s %q does not match draft PR receipt %q", field, values[0], values[1])
		}
	}
	if !r.Draft || !strings.EqualFold(strings.TrimSpace(r.State), "open") {
		return fmt.Errorf("draft PR receipt must prove an open draft PR, got draft=%v state=%q", r.Draft, r.State)
	}
	if !r.HumanApprovalRequired || !r.NoMergeOrDeployClaim || !r.ReadyForReviewRequired {
		return fmt.Errorf("draft PR receipt is missing required authority boundary flags")
	}
	if strings.TrimSpace(r.PolicyBundleID) != TransparaAIDraftPRPolicyBundleID {
		return fmt.Errorf("draft PR receipt policy_bundle_id %q does not match %q", r.PolicyBundleID, TransparaAIDraftPRPolicyBundleID)
	}
	if strings.TrimSpace(r.PolicyBundleHash) != TransparaAIDraftPRPolicyBundleHash() {
		return fmt.Errorf("draft PR receipt policy_bundle_hash %q does not match %q", r.PolicyBundleHash, TransparaAIDraftPRPolicyBundleHash())
	}
	if strings.TrimSpace(r.AuthorityNonce) == "" {
		return fmt.Errorf("draft PR receipt authority_nonce is required")
	}
	return nil
}

func issueScanReadyStatusOK(value string, allowed []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, ok := range allowed {
		if value == ok {
			return true
		}
	}
	return false
}

func issueScanReadyStrategistRoleOutput(evidence issueScanReadyStageEvidence) IssueScanStageRoleOutputEvidence {
	summary := strings.TrimSpace(evidence.ReadyPR.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Ready-for-Human PR %s#%d is open, non-draft, and waiting on Human approval: %s", evidence.ReadyPR.Repository, evidence.ReadyPR.PRNumber, evidence.ReadyPR.PRURL)
	}
	refs := issueScanReadyEvidenceRefs(evidence)
	return IssueScanStageRoleOutputEvidence{
		Role:         "strategist",
		Summary:      summary,
		EvidenceRefs: refs,
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "ready_pr_url",
				Summary:      evidence.ReadyPR.PRURL,
				EvidenceRefs: []string{evidence.ReadyPREvidenceID.Value(), evidence.ReadyPR.PRURL},
			},
			{
				Key:          "human_ready_summary",
				Summary:      summary,
				EvidenceRefs: refs,
			},
		},
		SourceRefs: issueScanReadySourceRefs(evidence),
	}
}

func issueScanReadyReviewerRoleOutput(evidence issueScanReadyStageEvidence) IssueScanStageRoleOutputEvidence {
	summary := fmt.Sprintf("Ready-state review passed for PR %s at head %s: %s", evidence.ReadyPR.PRURL, evidence.ReadyPR.HeadSHA, evidence.ReadyPR.ReadyStateReviewRef)
	return IssueScanStageRoleOutputEvidence{
		Role:         "reviewer",
		Summary:      summary,
		EvidenceRefs: []string{evidence.ReadyPREvidenceID.Value(), evidence.ReadyPR.ReadyStateReviewRef},
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "ready_state_review",
				Summary:      summary,
				EvidenceRefs: []string{evidence.ReadyPREvidenceID.Value(), evidence.ReadyPR.ReadyStateReviewRef},
			},
		},
		SourceRefs: issueScanReadySourceRefs(evidence),
	}
}

func issueScanReadyGuardianRoleOutput(evidence issueScanReadyStageEvidence) IssueScanStageRoleOutputEvidence {
	summary := fmt.Sprintf("Human approval boundary holds for PR %s: no merge, deploy, protected update, or Human approval claim is recorded by this stage.", evidence.ReadyPR.PRURL)
	return IssueScanStageRoleOutputEvidence{
		Role:         "guardian",
		Summary:      summary,
		EvidenceRefs: issueScanReadyEvidenceRefs(evidence),
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "human_approval_boundary_check",
				Summary:      summary,
				EvidenceRefs: issueScanReadyEvidenceRefs(evidence),
			},
		},
		SourceRefs: issueScanReadySourceRefs(evidence),
	}
}

func issueScanReadyEvidenceRefs(evidence issueScanReadyStageEvidence) []string {
	return compactStrings([]string{
		evidence.ReadyPREvidenceID.Value(),
		evidence.ReadyPR.PRURL,
		evidence.ReadyPR.ReadyStateReviewRef,
		evidence.BlockerRuntimeID.Value(),
		evidence.ImplementationEvidence.CompletionEventID.Value(),
		evidence.ImplementationEvidence.OperateArtifactID.Value(),
	})
}

func issueScanReadySourceRefs(evidence issueScanReadyStageEvidence) []string {
	refs := []string{
		evidence.ReviewedTaskID.Value(),
		evidence.BlockerStageTaskID.Value(),
		evidence.BlockerRuntimeID.Value(),
		evidence.ReadyStageTaskID.Value(),
		evidence.ReadyPREvidenceID.Value(),
		evidence.ImplementationEvidence.CompletionEventID.Value(),
		evidence.ImplementationEvidence.OperateArtifactID.Value(),
		evidence.ReadyPR.PRURL,
		evidence.ReadyPR.ReadyStateReviewRef,
	}
	refs = append(refs, evidence.ReadyPR.SourceRefs...)
	return compactStrings(refs)
}
