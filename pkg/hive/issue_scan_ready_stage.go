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

const (
	IssueScanReadyPREvidenceArtifactLabel = "issue_scan_ready_pr_evidence"
	issueScanReadyPREvidenceArtifactKind  = "issue_scan_ready_pr_evidence"
	issueScanReadyPRRunnerContextKind     = "issue_scan_ready_pr_runner_context"
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

// IssueScanDraftPRReceiptRecordResult summarizes a draft-PR receipt append to
// the terminal issue-scan ready stage. It proves only draft PR creation; marking
// the PR ready, ready-state review, Human approval, merge, and deploy remain
// separate governed steps.
type IssueScanDraftPRReceiptRecordResult struct {
	RunID                         string
	FactoryOrderID                string
	ReadyStageTaskID              types.EventID
	DraftPRReceiptArtifactID      types.EventID
	Repository                    string
	PRNumber                      int
	PRURL                         string
	HeadSHA                       string
	Recorded                      bool
	DraftPRReceiptAlreadyRecorded bool
}

// IssueScanReadyPREvidenceRecordResult summarizes a ready-for-Human PR
// evidence append to the terminal issue-scan ready stage.
type IssueScanReadyPREvidenceRecordResult struct {
	RunID                          string
	FactoryOrderID                 string
	ReadyStageTaskID               types.EventID
	ReadyPREvidenceArtifactID      types.EventID
	DraftPRReceiptRef              string
	Repository                     string
	PRNumber                       int
	PRURL                          string
	HeadSHA                        string
	Recorded                       bool
	ReadyPREvidenceAlreadyRecorded bool
}

// IssueScanReadyPRRunner receives exact implementation/blocker context and
// returns the draft-PR receipt plus ready-for-Human PR evidence to validate and
// record. The runtime performs all authoritative validation before completing
// the terminal ready stage.
type IssueScanReadyPRRunner func(context.Context, IssueScanReadyPRRunnerContext) (IssueScanReadyPRRunnerResult, error)

type IssueScanReadyPRRunnerContext struct {
	Kind                    string                     `json:"kind"`
	LifecycleVersion        string                     `json:"lifecycle_version"`
	RunID                   string                     `json:"run_id"`
	FactoryOrderID          string                     `json:"factory_order_id"`
	Repository              string                     `json:"repository"`
	DraftPRReceiptRef       string                     `json:"draft_pr_receipt_ref,omitempty"`
	DraftPRReceipt          *TransparaAIDraftPRReceipt `json:"-"`
	ReadyStageTaskID        string                     `json:"ready_stage_task_id"`
	BlockerStageTaskID      string                     `json:"blocker_stage_task_id"`
	ImplementationTaskID    string                     `json:"implementation_task_id"`
	OperateBranch           string                     `json:"operate_branch"`
	OperateCommit           string                     `json:"operate_commit"`
	OperateRange            string                     `json:"operate_range,omitempty"`
	ChangedFilesSummary     string                     `json:"changed_files_summary,omitempty"`
	ExpectedReadyPREvidence IssueScanReadyPREvidence   `json:"expected_ready_pr_evidence"`
	BoundaryDisclaimers     []string                   `json:"boundary_disclaimers,omitempty"`
}

type IssueScanReadyPRRunnerResult struct {
	DraftPRReceipt  TransparaAIDraftPRReceipt `json:"draft_pr_receipt"`
	ReadyPREvidence IssueScanReadyPREvidence  `json:"ready_pr_evidence"`
}

// ValidateIssueScanReadyPRRunnerResultForContext checks the deterministic,
// non-store-backed runtime rules for an external ready-PR runner packet. The
// authoritative record path still verifies live authority decisions and Work
// artifacts; this helper prevents package fixtures from certifying a result
// whose shape or context binding that record path would reject first.
func ValidateIssueScanReadyPRRunnerResultForContext(context IssueScanReadyPRRunnerContext, result IssueScanReadyPRRunnerResult) error {
	if strings.TrimSpace(context.Kind) != issueScanReadyPRRunnerContextKind {
		return fmt.Errorf("context kind %q does not match %q", context.Kind, issueScanReadyPRRunnerContextKind)
	}
	if strings.TrimSpace(context.LifecycleVersion) != issueScanLifecycleVersion {
		return fmt.Errorf("context lifecycle_version %q does not match %q", context.LifecycleVersion, issueScanLifecycleVersion)
	}
	for field, value := range map[string]string{
		"run_id":                 context.RunID,
		"factory_order_id":       context.FactoryOrderID,
		"repository":             context.Repository,
		"ready_stage_task_id":    context.ReadyStageTaskID,
		"blocker_stage_task_id":  context.BlockerStageTaskID,
		"implementation_task_id": context.ImplementationTaskID,
		"operate_branch":         context.OperateBranch,
		"operate_commit":         context.OperateCommit,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("context %s is required", field)
		}
	}
	repo := strings.ToLower(strings.TrimSpace(context.Repository))
	if !ValidTransparaAIRepo(repo) {
		return fmt.Errorf("context repository %q is not a Transpara-AI repo", context.Repository)
	}
	expected := context.ExpectedReadyPREvidence
	for field, values := range map[string][2]string{
		"kind":              {strings.TrimSpace(expected.Kind), issueScanReadyPREvidenceArtifactKind},
		"lifecycle_version": {strings.TrimSpace(expected.LifecycleVersion), issueScanLifecycleVersion},
		"run_id":            {strings.TrimSpace(expected.RunID), strings.TrimSpace(context.RunID)},
		"factory_order_id":  {strings.TrimSpace(expected.FactoryOrderID), strings.TrimSpace(context.FactoryOrderID)},
		"repository":        {strings.ToLower(strings.TrimSpace(expected.Repository)), repo},
	} {
		if values[0] != values[1] {
			return fmt.Errorf("context expected_ready_pr_evidence %s %q does not match %q", field, values[0], values[1])
		}
	}
	if !strings.EqualFold(strings.TrimSpace(expected.HeadSHA), strings.TrimSpace(context.OperateCommit)) {
		return fmt.Errorf("context expected_ready_pr_evidence head_sha %q does not match operate_commit %q", expected.HeadSHA, context.OperateCommit)
	}
	if !strings.EqualFold(strings.TrimSpace(expected.State), "open") || expected.Draft || !expected.ReadyForReview || !expected.HumanApprovalRequired {
		return fmt.Errorf("context expected_ready_pr_evidence must require an open, non-draft, ready-for-review PR with Human approval still required")
	}

	content := FactoryRunRequestedContent{TargetRepos: []string{repo}}
	if err := validateIssueScanDraftPRReceiptForRecord(content, result.DraftPRReceipt); err != nil {
		return fmt.Errorf("draft_pr_receipt: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(result.DraftPRReceipt.Repository)) != repo {
		return fmt.Errorf("draft_pr_receipt repository %q does not match context %q", result.DraftPRReceipt.Repository, context.Repository)
	}
	if strings.TrimSpace(result.DraftPRReceipt.HeadSHA) != strings.TrimSpace(context.OperateCommit) {
		return fmt.Errorf("draft_pr_receipt head_sha %q does not match context operate_commit %q", result.DraftPRReceipt.HeadSHA, context.OperateCommit)
	}

	evidence := result.ReadyPREvidence
	for field, values := range map[string][2]string{
		"kind":              {strings.TrimSpace(evidence.Kind), issueScanReadyPREvidenceArtifactKind},
		"lifecycle_version": {strings.TrimSpace(evidence.LifecycleVersion), issueScanLifecycleVersion},
		"run_id":            {strings.TrimSpace(evidence.RunID), strings.TrimSpace(context.RunID)},
		"factory_order_id":  {strings.TrimSpace(evidence.FactoryOrderID), strings.TrimSpace(context.FactoryOrderID)},
		"repository":        {strings.ToLower(strings.TrimSpace(evidence.Repository)), repo},
	} {
		if values[0] != values[1] {
			return fmt.Errorf("ready_pr_evidence %s %q does not match context %q", field, values[0], values[1])
		}
	}
	if evidence.PRNumber <= 0 {
		return fmt.Errorf("ready_pr_evidence pr_number is required")
	}
	if strings.TrimSpace(evidence.PRURL) == "" || !strings.Contains(strings.ToLower(strings.TrimSpace(evidence.PRURL)), "github.com/"+repo+"/pull/") {
		return fmt.Errorf("ready_pr_evidence pr_url %q does not match repository %q", evidence.PRURL, repo)
	}
	if !strings.EqualFold(strings.TrimSpace(evidence.HeadSHA), strings.TrimSpace(context.OperateCommit)) {
		return fmt.Errorf("ready_pr_evidence head_sha %q does not match context operate_commit %q", evidence.HeadSHA, context.OperateCommit)
	}
	if !strings.EqualFold(strings.TrimSpace(evidence.State), "open") {
		return fmt.Errorf("ready_pr_evidence state %q is not open", evidence.State)
	}
	if evidence.Draft || !evidence.ReadyForReview {
		return fmt.Errorf("ready_pr_evidence must prove a non-draft ready-for-review PR")
	}
	if !issueScanReadyStatusOK(evidence.MergeStateStatus, []string{"clean", "blocked"}) {
		return fmt.Errorf("ready_pr_evidence merge_state_status %q is not clean or blocked", evidence.MergeStateStatus)
	}
	if !issueScanReadyStatusOK(evidence.CIStatus, []string{"success", "passed", "green"}) {
		return fmt.Errorf("ready_pr_evidence ci_status %q is not successful", evidence.CIStatus)
	}
	if strings.TrimSpace(evidence.ReadyStateReviewRef) == "" {
		return fmt.Errorf("ready_pr_evidence ready_state_review_ref is required")
	}
	if !issueScanReadyStatusOK(evidence.ReadyStateReviewStatus, []string{"success", "passed", "pass", "no_blockers", "no blockers"}) {
		return fmt.Errorf("ready_pr_evidence ready_state_review_status %q is not passing", evidence.ReadyStateReviewStatus)
	}
	if !evidence.HumanApprovalRequired {
		return fmt.Errorf("ready_pr_evidence human_approval_required must be true")
	}
	if strings.TrimSpace(evidence.DraftPRReceiptRef) != strings.TrimSpace(context.DraftPRReceiptRef) {
		return fmt.Errorf("ready_pr_evidence draft_pr_receipt_ref %q does not match context %q", evidence.DraftPRReceiptRef, context.DraftPRReceiptRef)
	}
	if err := validateIssueScanDraftPRReceiptForReadyEvidence(issueScanDraftPRReceiptEvidence{Receipt: result.DraftPRReceipt}, evidence, repo, strings.TrimSpace(evidence.HeadSHA)); err != nil {
		return fmt.Errorf("ready_pr_evidence: %w", err)
	}
	return nil
}

type IssueScanReadyPRRunnerRecordResult struct {
	RunID            string
	FactoryOrderID   string
	ReadyStageTaskID types.EventID
	DraftPRReceipt   IssueScanDraftPRReceiptRecordResult
	ReadyPREvidence  IssueScanReadyPREvidenceRecordResult
	Recorded         bool
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

func (r *Runtime) IssueScanReadyPRRunnerContext(runID string) (IssueScanReadyPRRunnerContext, error) {
	readyContext, ready, err := r.issueScanReadyPRRunnerContext(runID)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, err
	}
	if !ready {
		return IssueScanReadyPRRunnerContext{}, fmt.Errorf("issue-scan run %q is not ready for ready-PR evidence collection", strings.TrimSpace(runID))
	}
	return readyContext, nil
}

func (r *Runtime) RunConfiguredIssueScanReadyPRRunners(ctx context.Context, result RunLaunchDispatchResult) ([]IssueScanReadyPRRunnerRecordResult, error) {
	if r == nil || r.issueScanReadyPRRunner == nil {
		return nil, nil
	}
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanReadyPRRunnerRecordResult, 0, len(runIDs))
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RunConfiguredIssueScanReadyPRRunner(ctx, runID)
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

func (r *Runtime) RunConfiguredIssueScanReadyPRRunner(ctx context.Context, runID string) (IssueScanReadyPRRunnerRecordResult, bool, error) {
	runID = strings.TrimSpace(runID)
	result := IssueScanReadyPRRunnerRecordResult{RunID: runID}
	if r == nil || r.issueScanReadyPRRunner == nil {
		return result, false, nil
	}
	readyContext, ready, err := r.issueScanReadyPRRunnerContext(runID)
	if err != nil || !ready {
		return result, ready, err
	}
	runnerResult, err := r.issueScanReadyPRRunner(ctx, readyContext)
	if err != nil {
		if errors.Is(err, errIssueScanReadyPRFinalizerAwaitingDraftReceipt) {
			return result, false, nil
		}
		return result, true, err
	}
	draftResult, err := r.RecordIssueScanDraftPRReceipt(runID, runnerResult.DraftPRReceipt)
	if err != nil {
		return result, true, err
	}
	readyResult, err := r.RecordIssueScanReadyPREvidence(runID, runnerResult.ReadyPREvidence)
	if err != nil {
		return result, true, err
	}
	result.FactoryOrderID = readyContext.FactoryOrderID
	result.ReadyStageTaskID = draftResult.ReadyStageTaskID
	result.DraftPRReceipt = draftResult
	result.ReadyPREvidence = readyResult
	result.Recorded = draftResult.Recorded || readyResult.Recorded
	return result, true, nil
}

// RecordIssueScanDraftPRReceipt records the draft-PR creation receipt on the
// issue-scan ready stage so later ready-PR evidence can prove it is the same
// PR/head authorized by the governed draft-PR path.
func (r *Runtime) RecordIssueScanDraftPRReceipt(runID string, receipt TransparaAIDraftPRReceipt) (IssueScanDraftPRReceiptRecordResult, error) {
	content, orderID, requestID, readyStage, err := r.issueScanReadyStageTarget(runID)
	result := IssueScanDraftPRReceiptRecordResult{RunID: strings.TrimSpace(runID), FactoryOrderID: orderID}
	if err != nil {
		return result, err
	}
	result.ReadyStageTaskID = readyStage.TaskID
	receipt.Kind = valueOr(strings.TrimSpace(receipt.Kind), transparaAIDraftPRReceiptKind)
	receipt.Repository = strings.ToLower(strings.TrimSpace(receipt.Repository))
	receipt.PRURL = strings.TrimSpace(receipt.PRURL)
	receipt.HeadSHA = strings.TrimSpace(receipt.HeadSHA)
	receipt.RemoteHeadSHA = strings.TrimSpace(receipt.RemoteHeadSHA)
	if err := validateIssueScanDraftPRReceiptForRecord(content, receipt); err != nil {
		return result, err
	}
	authorityDecisionID, authorityRequestID, _, err := r.approvedDraftPRAuthorityDecisionForReceipt(receipt)
	if err != nil {
		return result, err
	}
	receipt.AuthorityDecisionRef = authorityDecisionID.Value()
	receipt.AuthorityRequestID = authorityRequestID.Value()
	body, err := transparaAIDraftPRReceiptBody(receipt)
	if err != nil {
		return result, err
	}
	artifactID, exists, err := r.findIssueScanReadyStageArtifactID(readyStage.TaskID, TransparaAIDraftPRReceiptArtifactLabel, body)
	if err != nil {
		return result, err
	}
	if !exists {
		causes := compactEventIDs([]types.EventID{requestID, readyStage.TaskID, authorityRequestID, authorityDecisionID})
		if err := r.tasks.AddArtifact(r.humanID, readyStage.TaskID, TransparaAIDraftPRReceiptArtifactLabel, "application/json", body, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
			return result, fmt.Errorf("record issue-scan draft PR receipt: %w", err)
		}
		artifactID, exists, err = r.findIssueScanReadyStageArtifactID(readyStage.TaskID, TransparaAIDraftPRReceiptArtifactLabel, body)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf("issue-scan draft PR receipt artifact was not found after append")
		}
		result.Recorded = true
	} else {
		result.DraftPRReceiptAlreadyRecorded = true
	}
	result.DraftPRReceiptArtifactID = artifactID
	result.Repository = receipt.Repository
	result.PRNumber = receipt.PRNumber
	result.PRURL = receipt.PRURL
	result.HeadSHA = receipt.HeadSHA
	return result, nil
}

// RecordIssueScanReadyPREvidence records terminal ready-for-Human PR evidence
// on the issue-scan ready stage after validating it against the latest
// implementation completion and the draft-PR receipt already linked to the
// stage.
func (r *Runtime) RecordIssueScanReadyPREvidence(runID string, evidence IssueScanReadyPREvidence) (IssueScanReadyPREvidenceRecordResult, error) {
	content, orderID, requestID, readyStage, err := r.issueScanReadyStageTarget(runID)
	result := IssueScanReadyPREvidenceRecordResult{RunID: strings.TrimSpace(runID), FactoryOrderID: orderID}
	if err != nil {
		return result, err
	}
	result.ReadyStageTaskID = readyStage.TaskID
	blockerStage, implementationTaskID, implementation, err := r.issueScanReadyPrerequisites(content, orderID, readyStage)
	if err != nil {
		return result, err
	}
	artifacts, err := r.tasks.ListArtifacts(readyStage.TaskID)
	if err != nil {
		return result, fmt.Errorf("list ready stage artifacts: %w", err)
	}
	draftReceipts, err := issueScanDraftPRReceiptArtifacts(artifacts)
	if err != nil {
		return result, err
	}
	normalized, err := normalizeIssueScanReadyPREvidence(content, orderID, evidence, implementation, draftReceipts)
	if err != nil {
		return result, err
	}
	authorityDecisionID, err := r.approvedDraftPRAuthorityDecisionForReadyEvidence(normalized, implementation, draftReceipts)
	if err != nil {
		return result, err
	}
	normalized.SourceRefs = compactStrings(append(normalized.SourceRefs, authorityDecisionID.Value()))
	body, err := IssueScanReadyPREvidenceArtifactBody(normalized)
	if err != nil {
		return result, err
	}
	artifactID, exists, err := r.findIssueScanReadyStageArtifactID(readyStage.TaskID, IssueScanReadyPREvidenceArtifactLabel, body)
	if err != nil {
		return result, err
	}
	if !exists {
		causes := compactEventIDs([]types.EventID{requestID, readyStage.TaskID, blockerStage.TaskID, implementationTaskID, implementation.CompletionEventID, implementation.OperateArtifactID, authorityDecisionID})
		if err := r.tasks.AddArtifact(r.humanID, readyStage.TaskID, IssueScanReadyPREvidenceArtifactLabel, "application/json", body, causes, runLaunchConversationID(content.RunID, r.convID)); err != nil {
			return result, fmt.Errorf("record issue-scan ready PR evidence: %w", err)
		}
		artifactID, exists, err = r.findIssueScanReadyStageArtifactID(readyStage.TaskID, IssueScanReadyPREvidenceArtifactLabel, body)
		if err != nil {
			return result, err
		}
		if !exists {
			return result, fmt.Errorf("issue-scan ready PR evidence artifact was not found after append")
		}
		result.Recorded = true
	} else {
		result.ReadyPREvidenceAlreadyRecorded = true
	}
	result.ReadyPREvidenceArtifactID = artifactID
	result.DraftPRReceiptRef = normalized.DraftPRReceiptRef
	result.Repository = normalized.Repository
	result.PRNumber = normalized.PRNumber
	result.PRURL = normalized.PRURL
	result.HeadSHA = normalized.HeadSHA
	return result, nil
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
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return nil, false, err
	} else if parked {
		return nil, false, nil
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

func (r *Runtime) issueScanReadyStageTarget(runID string) (FactoryRunRequestedContent, string, types.EventID, *issueScanStageAdvanceTarget, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, err
	}
	if len(requests) == 0 {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, fmt.Errorf("queued run %q is not an issue-scan run", runID)
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, fmt.Errorf("dispatch queued issue-scan run %q before ready evidence recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, err
	}
	readyStage, err := r.issueScanStageTargetByStageID(drafts, "surface_ready_for_Human_result_PR", orderID)
	if err != nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, err
	}
	if err := r.verifyIssueScanStageTaskContracts(readyStage); err != nil {
		return FactoryRunRequestedContent{}, "", types.EventID{}, nil, err
	}
	return content, orderID, requests[0].ID(), readyStage, nil
}

func (r *Runtime) issueScanReadyPRRunnerContext(runID string) (IssueScanReadyPRRunnerContext, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return IssueScanReadyPRRunnerContext{}, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return IssueScanReadyPRRunnerContext{}, false, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	} else if parked {
		return IssueScanReadyPRRunnerContext{}, false, nil
	}
	content, orderID, _, readyStage, err := r.issueScanReadyStageTarget(runID)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(readyStage.TaskID)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	if stageCompleted {
		return IssueScanReadyPRRunnerContext{}, false, nil
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	blockerStage, err := r.issueScanStageTargetByStageID(drafts, "drive_blockers_to_zero", orderID)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	blockerCompleted, err := r.issueScanStageTaskCompleted(blockerStage.TaskID)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	if !blockerCompleted {
		return IssueScanReadyPRRunnerContext{}, false, nil
	}
	implementationTaskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return IssueScanReadyPRRunnerContext{}, false, nil
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return IssueScanReadyPRRunnerContext{}, false, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	implementation, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, readyStage.TaskID)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	if !ok {
		return IssueScanReadyPRRunnerContext{}, false, nil
	}
	if _, ready, err := r.issueScanReadyStageEvidence(content, orderID, implementationTaskID, blockerStage, readyStage); err != nil || ready {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	repo, err := issueScanReadyRunnerRepository(content)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	draftReceipt, draftReceiptRef, err := r.issueScanReadyRunnerDraftPRReceipt(content, readyStage.TaskID, repo, implementation.OperateCommit)
	if err != nil {
		return IssueScanReadyPRRunnerContext{}, false, err
	}
	expected := IssueScanReadyPREvidence{
		Kind:                  issueScanReadyPREvidenceArtifactKind,
		LifecycleVersion:      issueScanLifecycleVersion,
		RunID:                 strings.TrimSpace(content.RunID),
		FactoryOrderID:        orderID,
		Repository:            repo,
		HeadSHA:               implementation.OperateCommit,
		State:                 "open",
		Draft:                 false,
		ReadyForReview:        true,
		HumanApprovalRequired: true,
	}
	return IssueScanReadyPRRunnerContext{
		Kind:                    issueScanReadyPRRunnerContextKind,
		LifecycleVersion:        issueScanLifecycleVersion,
		RunID:                   strings.TrimSpace(content.RunID),
		FactoryOrderID:          orderID,
		Repository:              repo,
		DraftPRReceiptRef:       draftReceiptRef,
		DraftPRReceipt:          draftReceipt,
		ReadyStageTaskID:        readyStage.TaskID.Value(),
		BlockerStageTaskID:      blockerStage.TaskID.Value(),
		ImplementationTaskID:    implementationTaskID.Value(),
		OperateBranch:           implementation.OperateBranch,
		OperateCommit:           implementation.OperateCommit,
		OperateRange:            implementation.OperateRange,
		ChangedFilesSummary:     implementation.ChangedFilesSummary,
		ExpectedReadyPREvidence: expected,
		BoundaryDisclaimers: compactStrings([]string{
			"ready PR evidence is not Human approval",
			"ready PR evidence is not merge or deploy authorization",
			"ready PR head_sha must match operate_commit",
		}),
	}, true, nil
}

func (r *Runtime) issueScanReadyRunnerDraftPRReceipt(content FactoryRunRequestedContent, readyStageTaskID types.EventID, repo, head string) (*TransparaAIDraftPRReceipt, string, error) {
	if readyStageTaskID.IsZero() {
		return nil, "", nil
	}
	repo = strings.ToLower(strings.TrimSpace(repo))
	head = strings.TrimSpace(head)
	if repo == "" || head == "" {
		return nil, "", nil
	}
	artifacts, err := r.tasks.ListArtifacts(readyStageTaskID)
	if err != nil {
		return nil, "", fmt.Errorf("list ready stage artifacts for draft PR receipt: %w", err)
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		receipt, ok, err := transparaAIDraftPRReceiptArtifact(artifacts[i].ID.Value(), artifacts[i].Label, artifacts[i].Body)
		if err != nil || !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(receipt.Repository)) != repo {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(receipt.HeadSHA), head) {
			continue
		}
		if err := validateIssueScanDraftPRReceiptForRecord(content, receipt); err != nil {
			return nil, "", fmt.Errorf("draft PR receipt artifact %s for %s at head %s is invalid: %w", artifacts[i].ID.Value(), repo, head, err)
		}
		if _, _, _, err := r.approvedDraftPRAuthorityDecisionForReceipt(receipt); err != nil {
			return nil, "", fmt.Errorf("draft PR receipt artifact %s for %s at head %s has no approved authority decision: %w", artifacts[i].ID.Value(), repo, head, err)
		}
		copied := receipt
		return &copied, artifacts[i].ID.Value(), nil
	}
	return nil, "", nil
}

func (r *Runtime) issueScanReadyPrerequisites(content FactoryRunRequestedContent, orderID string, readyStage *issueScanStageAdvanceTarget) (*issueScanStageAdvanceTarget, types.EventID, issueScanOperateCompletionEvidence, error) {
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, err
	}
	blockerStage, err := r.issueScanStageTargetByStageID(drafts, "drive_blockers_to_zero", orderID)
	if err != nil {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, err
	}
	blockerCompleted, err := r.issueScanStageTaskCompleted(blockerStage.TaskID)
	if err != nil {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, err
	}
	if !blockerCompleted {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, fmt.Errorf("drive_blockers_to_zero stage has not completed")
	}
	implementationTaskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, fmt.Errorf("implementation task for FactoryOrder %q has not been created", orderID)
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	implementation, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, readyStage.TaskID)
	if err != nil {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, err
	}
	if !ok {
		return nil, types.EventID{}, issueScanOperateCompletionEvidence{}, fmt.Errorf("implementation task %s has no live completion evidence", implementationTaskID.Value())
	}
	return blockerStage, implementationTaskID, implementation, nil
}

func issueScanReadyRunnerRepository(content FactoryRunRequestedContent) (string, error) {
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return "", err
	}
	repo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
	if repo == "" && len(content.TargetRepos) > 0 {
		repo = strings.ToLower(strings.TrimSpace(content.TargetRepos[0]))
	}
	if !ValidTransparaAIRepo(repo) {
		return "", fmt.Errorf("selected issue repository %q is not a Transpara-AI repo", repo)
	}
	return repo, nil
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
		authorityDecisionID, err := r.approvedDraftPRAuthorityDecisionForReadyEvidence(normalized, implementation, draftReceipts)
		if err != nil {
			return IssueScanReadyPREvidence{}, types.EventID{}, false, fmt.Errorf("validate ready PR authority for artifact %s: %w", artifact.ID.Value(), err)
		}
		normalized.SourceRefs = compactStrings(append(normalized.SourceRefs, authorityDecisionID.Value()))
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
	payload.AuthorityDecisionRef = strings.TrimSpace(payload.AuthorityDecisionRef)
	payload.AuthorityRequestID = strings.TrimSpace(payload.AuthorityRequestID)
	return payload, true, nil
}

func (r *Runtime) approvedDraftPRAuthorityDecisionForReadyEvidence(evidence IssueScanReadyPREvidence, implementation issueScanOperateCompletionEvidence, receipts []issueScanDraftPRReceiptEvidence) (types.EventID, error) {
	repo := strings.ToLower(strings.TrimSpace(evidence.Repository))
	head := strings.TrimSpace(evidence.HeadSHA)
	if head == "" {
		head = strings.TrimSpace(implementation.OperateCommit)
	}
	receipt, err := matchingIssueScanDraftPRReceipt(evidence, repo, head, receipts)
	if err != nil {
		return types.EventID{}, err
	}
	decisionID, _, _, err := r.approvedDraftPRAuthorityDecisionForReceipt(receipt.Receipt)
	if err != nil {
		return types.EventID{}, err
	}
	return decisionID, nil
}

func (r *Runtime) approvedDraftPRAuthorityDecisionForReceipt(receipt TransparaAIDraftPRReceipt) (types.EventID, types.EventID, DraftPRTarget, error) {
	if r == nil || r.store == nil {
		return types.EventID{}, types.EventID{}, DraftPRTarget{}, fmt.Errorf("runtime store is required to verify draft PR authority decision")
	}
	cursor := types.None[types.Cursor]()
	var lastMismatch error
	for {
		page, err := r.store.ByType(EventTypeAuthorityDecisionRecorded, defaultOperatorProjectionLimit, cursor)
		if err != nil {
			return types.EventID{}, types.EventID{}, DraftPRTarget{}, fmt.Errorf("load authority decisions for draft PR receipt: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(AuthorityDecisionRecordedContent)
			if !ok || strings.TrimSpace(content.Outcome) != draftPRApprovedOutcome {
				continue
			}
			target, err := ParseDraftPRScope(content.Scope)
			if err != nil {
				continue
			}
			if err := validateDraftPRTargetMatchesReceipt(target, receipt); err != nil {
				lastMismatch = err
				continue
			}
			if ref := strings.TrimSpace(receipt.AuthorityDecisionRef); ref != "" && ref != ev.ID().Value() {
				lastMismatch = fmt.Errorf("receipt authority_decision_ref %q does not match approved decision %s", ref, ev.ID().Value())
				continue
			}
			if ref := strings.TrimSpace(receipt.AuthorityRequestID); ref != "" && ref != content.RequestID.Value() {
				lastMismatch = fmt.Errorf("receipt authority_request_id %q does not match approved request %s", ref, content.RequestID.Value())
				continue
			}
			return ev.ID(), content.RequestID, target, nil
		}
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	if lastMismatch != nil {
		return types.EventID{}, types.EventID{}, DraftPRTarget{}, fmt.Errorf("no approved draft PR authority decision matches receipt for %s#%d: %w", receipt.Repository, receipt.PRNumber, lastMismatch)
	}
	return types.EventID{}, types.EventID{}, DraftPRTarget{}, fmt.Errorf("no approved draft PR authority decision matches receipt for %s#%d", receipt.Repository, receipt.PRNumber)
}

func validateDraftPRTargetMatchesReceipt(target DraftPRTarget, receipt TransparaAIDraftPRReceipt) error {
	for field, values := range map[string][2]string{
		"repository":         {strings.ToLower(strings.TrimSpace(receipt.Repository)), strings.ToLower(strings.TrimSpace(target.Repository))},
		"base_ref":           {strings.TrimSpace(receipt.BaseRef), strings.TrimSpace(target.BaseRef)},
		"base_sha":           {strings.TrimSpace(receipt.BaseSHA), strings.TrimSpace(target.BaseSHA)},
		"head_ref":           {strings.TrimSpace(receipt.HeadRef), strings.TrimSpace(target.HeadRef)},
		"head_sha":           {strings.TrimSpace(receipt.HeadSHA), strings.TrimSpace(target.HeadSHA)},
		"policy_bundle_id":   {strings.TrimSpace(receipt.PolicyBundleID), strings.TrimSpace(target.PolicyBundleID)},
		"policy_bundle_hash": {strings.TrimSpace(receipt.PolicyBundleHash), strings.TrimSpace(target.PolicyBundleHash)},
		"authority_nonce":    {strings.TrimSpace(receipt.AuthorityNonce), strings.TrimSpace(target.SingleUseNonce)},
	} {
		if values[0] != values[1] {
			return fmt.Errorf("%s %q does not match approved target %q", field, values[0], values[1])
		}
	}
	return nil
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
	if !issueScanReadyStatusOK(evidence.MergeStateStatus, []string{"clean", "blocked"}) {
		return IssueScanReadyPREvidence{}, fmt.Errorf("merge_state_status %q is not clean or blocked by required Human review", evidence.MergeStateStatus)
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
	evidence.MergeStateStatus = strings.ToLower(strings.TrimSpace(evidence.MergeStateStatus))
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

func validateIssueScanDraftPRReceiptForRecord(content FactoryRunRequestedContent, receipt TransparaAIDraftPRReceipt) error {
	if strings.TrimSpace(receipt.Kind) != transparaAIDraftPRReceiptKind {
		return fmt.Errorf("kind %q does not match %q", receipt.Kind, transparaAIDraftPRReceiptKind)
	}
	repo := strings.ToLower(strings.TrimSpace(receipt.Repository))
	if !ValidTransparaAIRepo(repo) {
		return fmt.Errorf("repository %q is not a Transpara-AI repo", receipt.Repository)
	}
	if len(content.TargetRepos) > 0 && !containsIssueScanString(content.TargetRepos, repo) {
		return fmt.Errorf("repository %q is outside issue-scan target repos %v", repo, content.TargetRepos)
	}
	if receipt.PRNumber <= 0 {
		return fmt.Errorf("pr_number is required")
	}
	if strings.TrimSpace(receipt.PRURL) == "" || !strings.Contains(strings.ToLower(strings.TrimSpace(receipt.PRURL)), "github.com/"+repo+"/pull/") {
		return fmt.Errorf("pr_url %q does not match repository %q", receipt.PRURL, repo)
	}
	if strings.TrimSpace(receipt.HeadSHA) == "" || strings.TrimSpace(receipt.RemoteHeadSHA) == "" {
		return fmt.Errorf("head_sha and remote_head_sha are required")
	}
	if strings.TrimSpace(receipt.HeadSHA) != strings.TrimSpace(receipt.RemoteHeadSHA) {
		return fmt.Errorf("head_sha %q does not match remote_head_sha %q", receipt.HeadSHA, receipt.RemoteHeadSHA)
	}
	if _, err := normalizePRChangedFiles(receipt.ChangedFiles); err != nil {
		return fmt.Errorf("changed_files: %w", err)
	}
	if len(receipt.ChangedFiles) == 0 {
		return fmt.Errorf("changed_files is required")
	}
	if !receipt.Draft || !strings.EqualFold(strings.TrimSpace(receipt.State), "open") {
		return fmt.Errorf("draft PR receipt must prove an open draft PR, got draft=%v state=%q", receipt.Draft, receipt.State)
	}
	if strings.TrimSpace(receipt.PolicyBundleID) != TransparaAIDraftPRPolicyBundleID {
		return fmt.Errorf("policy_bundle_id %q does not match %q", receipt.PolicyBundleID, TransparaAIDraftPRPolicyBundleID)
	}
	if strings.TrimSpace(receipt.PolicyBundleHash) != TransparaAIDraftPRPolicyBundleHash() {
		return fmt.Errorf("policy_bundle_hash %q does not match %q", receipt.PolicyBundleHash, TransparaAIDraftPRPolicyBundleHash())
	}
	if strings.TrimSpace(receipt.AuthorityNonce) == "" {
		return fmt.Errorf("authority_nonce is required")
	}
	if !receipt.HumanApprovalRequired || !receipt.NoMergeOrDeployClaim || !receipt.ReadyForReviewRequired {
		return fmt.Errorf("draft PR receipt is missing required authority boundary flags")
	}
	return nil
}

func (r *Runtime) findIssueScanReadyStageArtifactID(stageTaskID types.EventID, label, body string) (types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(stageTaskID)
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("list ready stage artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Label) != strings.TrimSpace(label) {
			continue
		}
		if strings.TrimSpace(artifact.Body) != strings.TrimSpace(body) {
			continue
		}
		return artifact.ID, true, nil
	}
	return types.EventID{}, false, nil
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
