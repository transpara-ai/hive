package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	issueScanReadyPRFinalizerMutationKind = "issue_scan_ready_pr_finalizer_mutation"
	issueScanReadyStateReviewContextKind  = "issue_scan_ready_state_review_context"
	issueScanReadyPRBlockedEvidenceKind   = "issue_scan_ready_pr_blocked"

	// IssueScanReadyPRBlockedEvidenceArtifactLabel labels the durable Work
	// artifact recording a blocked managed ready transition.
	IssueScanReadyPRBlockedEvidenceArtifactLabel = "issue_scan_ready_pr_blocked"
)

var errIssueScanReadyPRFinalizerAwaitingDraftReceipt = errors.New("issue-scan ready PR finalizer awaiting draft PR receipt")

// IssueScanReadyPRFinalizerClient performs the PR-state mutations in the
// terminal issue-scan stage: marking the already-created draft PR ready for
// review, reporting live PR state for validation, and — only under a recorded
// human approval carrying ReDraftOnFailure — returning the PR to draft when
// ready-state review fails after the mutation. It must not merge, approve,
// deploy, retarget, push, or change protected settings.
type IssueScanReadyPRFinalizerClient interface {
	MarkReadyForReview(context.Context, IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error)
	FetchReadyPRState(context.Context, IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error)
	ConvertToDraft(context.Context, IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error)
}

// MarkReadyApprovalLookup resolves the recorded human mark-ready approval for
// a run-derived mutation. A lookup error refuses the ready transition before
// the PR is touched (fail closed).
type MarkReadyApprovalLookup func(context.Context, IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error)

// MarkReadyApprovalConsumer durably records single-use consumption of the
// recorded approval BEFORE the ready mutation, erroring when the nonce was
// already consumed or the record cannot be written and read back (fail
// closed). Without it, a re-draft would return the PR to draft state and the
// same recorded approval would authorize a second flip.
type MarkReadyApprovalConsumer func(context.Context, IssueScanReadyPRFinalizerMutation, MarkReadyTarget) error

// ErrIssueScanMarkReadyNotAuthorized is returned when no recorded, approved,
// exactly-matching pull_request.mark_ready decision covers the run-derived
// target. A draft-PR creation approval never satisfies this gate.
var ErrIssueScanMarkReadyNotAuthorized = errors.New("issue-scan mark-ready is not authorized by a recorded approval")

// ErrIssueScanMarkReadyNotMutated marks a MarkReadyForReview failure the
// client has PROVEN left the PR un-mutated (a refusal before any GraphQL
// call, or a post-failure reconcile fetch showing the PR still draft). Only
// errors wrapping this sentinel bypass blocked evidence; any unproven failure
// is treated as a possible mutation and produces durable blocked evidence.
var ErrIssueScanMarkReadyNotMutated = errors.New("mark-ready failed with the PR proven un-mutated")

// IssueScanReadyPRRemediation names what the finalizer did after a
// post-mutation failure. The set is an allowlist; there is no default.
type IssueScanReadyPRRemediation string

const (
	IssueScanReadyPRRemediationReDrafted           IssueScanReadyPRRemediation = "re_drafted"
	IssueScanReadyPRRemediationReDraftUnauthorized IssueScanReadyPRRemediation = "re_draft_unauthorized"
	IssueScanReadyPRRemediationReDraftFailed       IssueScanReadyPRRemediation = "re_draft_failed"
)

// IssueScanReadyPRBlockedEvidence is the durable record of a managed ready
// transition that mutated the PR and then failed ready-state review (or its
// verification). It is recorded as a Work artifact on the ready stage; its
// presence means the PR is NOT Human-ready and the chain stopped here.
type IssueScanReadyPRBlockedEvidence struct {
	Kind                string                      `json:"kind"`
	LifecycleVersion    string                      `json:"lifecycle_version"`
	RunID               string                      `json:"run_id"`
	FactoryOrderID      string                      `json:"factory_order_id"`
	Repository          string                      `json:"repository"`
	PRNumber            int                         `json:"pr_number"`
	PRURL               string                      `json:"pr_url"`
	HeadSHA             string                      `json:"head_sha"`
	FailureReason       string                      `json:"failure_reason"`
	Remediation         IssueScanReadyPRRemediation `json:"remediation"`
	RemediationError    string                      `json:"remediation_error,omitempty"`
	ReviewRef           string                      `json:"review_ref,omitempty"`
	SingleUseNonce      string                      `json:"single_use_nonce"`
	BoundaryDisclaimers []string                    `json:"boundary_disclaimers,omitempty"`
}

// IssueScanReadyPRBlockedError carries blocked evidence out of the finalizer
// so the runtime records it durably; it always wraps the causing error.
type IssueScanReadyPRBlockedError struct {
	Evidence IssueScanReadyPRBlockedEvidence
	Cause    error
}

func (e *IssueScanReadyPRBlockedError) Error() string {
	return fmt.Sprintf("issue-scan ready PR blocked (%s): %v", e.Evidence.Remediation, e.Cause)
}

func (e *IssueScanReadyPRBlockedError) Unwrap() error { return e.Cause }

type IssueScanReadyPRFinalizerMutation struct {
	Kind                  string `json:"kind"`
	LifecycleVersion      string `json:"lifecycle_version"`
	RunID                 string `json:"run_id"`
	FactoryOrderID        string `json:"factory_order_id"`
	Repository            string `json:"repository"`
	PRNumber              int    `json:"pr_number"`
	PRURL                 string `json:"pr_url"`
	BaseRef               string `json:"base_ref"`
	BaseSHA               string `json:"base_sha"`
	HeadRef               string `json:"head_ref"`
	HeadSHA               string `json:"head_sha"`
	DraftPRReceiptRef     string `json:"draft_pr_receipt_ref"`
	HumanApprovalRequired bool   `json:"human_approval_required"`
	NoMergeOrDeployClaim  bool   `json:"no_merge_or_deploy_claim"`
}

type IssueScanReadyPRLiveState struct {
	Repository       string   `json:"repository"`
	PRNumber         int      `json:"pr_number"`
	PRURL            string   `json:"pr_url"`
	BaseRef          string   `json:"base_ref,omitempty"`
	BaseSHA          string   `json:"base_sha,omitempty"`
	HeadRef          string   `json:"head_ref,omitempty"`
	HeadSHA          string   `json:"head_sha"`
	State            string   `json:"state"`
	Draft            bool     `json:"draft"`
	ReadyForReview   bool     `json:"ready_for_review"`
	MergeStateStatus string   `json:"merge_state_status"`
	CIStatus         string   `json:"ci_status"`
	ReviewDecision   string   `json:"review_decision,omitempty"`
	SourceRefs       []string `json:"source_refs,omitempty"`
}

type IssueScanReadyStateReviewRunner func(context.Context, IssueScanReadyStateReviewContext) (IssueScanReadyStateReviewReceipt, error)

type IssueScanReadyStateReviewContext struct {
	Kind                    string                    `json:"kind"`
	LifecycleVersion        string                    `json:"lifecycle_version"`
	RunID                   string                    `json:"run_id"`
	FactoryOrderID          string                    `json:"factory_order_id"`
	Repository              string                    `json:"repository"`
	PRNumber                int                       `json:"pr_number"`
	PRURL                   string                    `json:"pr_url"`
	ReadyStageTaskID        string                    `json:"ready_stage_task_id"`
	ImplementationTaskID    string                    `json:"implementation_task_id"`
	OperateCommit           string                    `json:"operate_commit"`
	ReadyPRState            IssueScanReadyPRLiveState `json:"ready_pr_state"`
	ExpectedReadyPREvidence IssueScanReadyPREvidence  `json:"expected_ready_pr_evidence"`
	BoundaryDisclaimers     []string                  `json:"boundary_disclaimers,omitempty"`
}

type IssueScanReadyStateReviewReceipt struct {
	ReviewRef       string   `json:"review_ref"`
	ReviewedHeadSHA string   `json:"reviewed_head_sha"`
	Status          string   `json:"status"`
	Summary         string   `json:"summary,omitempty"`
	SourceRefs      []string `json:"source_refs,omitempty"`
}

func NewIssueScanReadyPRFinalizerRunner(client IssueScanReadyPRFinalizerClient, reviewer IssueScanReadyStateReviewRunner) IssueScanReadyPRRunner {
	return func(ctx context.Context, readyContext IssueScanReadyPRRunnerContext) (IssueScanReadyPRRunnerResult, error) {
		if readyContext.MarkReadyApprovalLookup == nil {
			return IssueScanReadyPRRunnerResult{}, fmt.Errorf("%w: no mark-ready approval lookup configured on the run context", ErrIssueScanMarkReadyNotAuthorized)
		}
		mutation, _, err := issueScanReadyPRFinalizerMutation(readyContext)
		if err != nil {
			return IssueScanReadyPRRunnerResult{}, err
		}
		approval, err := readyContext.MarkReadyApprovalLookup(ctx, mutation)
		if err != nil {
			return IssueScanReadyPRRunnerResult{}, fmt.Errorf("%w: %v", ErrIssueScanMarkReadyNotAuthorized, err)
		}
		return RunIssueScanReadyPRFinalizer(ctx, readyContext, client, reviewer, approval)
	}
}

// RunIssueScanReadyPRFinalizer gates the draft→ready transition on a
// recorded human mark-ready approval, marks the recorded draft PR ready, runs
// a ready-state exact-head review, verifies live state, and returns terminal
// ready-for-Human evidence for the existing ready-stage recorder. Any failure
// AFTER the ready mutation returns *IssueScanReadyPRBlockedError carrying
// durable blocked evidence (re-drafting only when the recorded approval
// permits it); ready-for-Human evidence is never produced on that path. It
// does not approve, merge, deploy, close, retarget, or request Human
// approval.
func RunIssueScanReadyPRFinalizer(ctx context.Context, readyContext IssueScanReadyPRRunnerContext, client IssueScanReadyPRFinalizerClient, reviewer IssueScanReadyStateReviewRunner, approval MarkReadyTarget) (IssueScanReadyPRRunnerResult, error) {
	if client == nil {
		return IssueScanReadyPRRunnerResult{}, fmt.Errorf("ready PR finalizer client is required")
	}
	if reviewer == nil {
		return IssueScanReadyPRRunnerResult{}, fmt.Errorf("ready-state review runner is required")
	}
	mutation, receipt, err := issueScanReadyPRFinalizerMutation(readyContext)
	if err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	if err := validateMarkReadyApproval(mutation, approval); err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	if readyContext.ConsumeMarkReadyApproval == nil {
		return IssueScanReadyPRRunnerResult{}, fmt.Errorf("%w: no single-use approval consumer configured on the run context", ErrIssueScanMarkReadyNotAuthorized)
	}
	if err := readyContext.ConsumeMarkReadyApproval(ctx, mutation, approval); err != nil {
		return IssueScanReadyPRRunnerResult{}, fmt.Errorf("%w: %v", ErrIssueScanMarkReadyNotAuthorized, err)
	}
	marked, err := client.MarkReadyForReview(ctx, mutation)
	if err != nil {
		if errors.Is(err, ErrIssueScanMarkReadyNotMutated) {
			// The client PROVED the PR was untouched: plain refusal, nothing
			// to remediate.
			return IssueScanReadyPRRunnerResult{}, err
		}
		// Unproven: fail safe — the PR may have been mutated, so record
		// durable blocked evidence and remediate under the recorded scope.
		return IssueScanReadyPRRunnerResult{}, issueScanReadyPRBlocked(ctx, client, mutation, approval, "", err)
	}
	if err := validateIssueScanReadyPRLiveState("mark ready", mutation, marked, false); err != nil {
		return IssueScanReadyPRRunnerResult{}, issueScanReadyPRBlocked(ctx, client, mutation, approval, "", err)
	}
	reviewContext := issueScanReadyStateReviewContext(readyContext, mutation, marked)
	review, err := reviewer(ctx, reviewContext)
	if err != nil {
		return IssueScanReadyPRRunnerResult{}, issueScanReadyPRBlocked(ctx, client, mutation, approval, "", err)
	}
	if err := validateIssueScanReadyStateReviewReceipt(mutation, review); err != nil {
		return IssueScanReadyPRRunnerResult{}, issueScanReadyPRBlocked(ctx, client, mutation, approval, review.ReviewRef, err)
	}
	live, err := client.FetchReadyPRState(ctx, mutation)
	if err != nil {
		return IssueScanReadyPRRunnerResult{}, issueScanReadyPRBlocked(ctx, client, mutation, approval, review.ReviewRef, err)
	}
	if err := validateIssueScanReadyPRLiveState("ready evidence", mutation, live, true); err != nil {
		return IssueScanReadyPRRunnerResult{}, issueScanReadyPRBlocked(ctx, client, mutation, approval, review.ReviewRef, err)
	}
	evidence := readyContext.ExpectedReadyPREvidence
	evidence.Repository = strings.ToLower(strings.TrimSpace(live.Repository))
	evidence.PRNumber = live.PRNumber
	evidence.PRURL = strings.TrimSpace(live.PRURL)
	evidence.BaseRef = strings.TrimSpace(live.BaseRef)
	evidence.BaseSHA = strings.TrimSpace(mutation.BaseSHA)
	if evidence.BaseSHA == "" {
		evidence.BaseSHA = strings.TrimSpace(live.BaseSHA)
	}
	evidence.HeadRef = strings.TrimSpace(live.HeadRef)
	evidence.HeadSHA = strings.TrimSpace(live.HeadSHA)
	evidence.State = "open"
	evidence.Draft = false
	evidence.ReadyForReview = true
	evidence.MergeStateStatus = strings.ToLower(strings.TrimSpace(live.MergeStateStatus))
	evidence.CIStatus = strings.ToLower(strings.TrimSpace(live.CIStatus))
	evidence.ReadyStateReviewRef = strings.TrimSpace(review.ReviewRef)
	evidence.ReadyStateReviewStatus = strings.ToLower(strings.TrimSpace(review.Status))
	evidence.HumanApprovalRequired = true
	evidence.DraftPRReceiptRef = strings.TrimSpace(mutation.DraftPRReceiptRef)
	evidence.Summary = strings.TrimSpace(review.Summary)
	if evidence.Summary == "" {
		evidence.Summary = "Ready-for-Human result PR is open, non-draft, exact-head reviewed, and waiting on Human approval."
	}
	evidence.SourceRefs = compactStrings(append(evidence.SourceRefs, live.SourceRefs...))
	evidence.SourceRefs = compactStrings(append(evidence.SourceRefs, review.SourceRefs...))
	return IssueScanReadyPRRunnerResult{DraftPRReceipt: receipt, ReadyPREvidence: evidence}, nil
}

func issueScanReadyPRFinalizerMutation(readyContext IssueScanReadyPRRunnerContext) (IssueScanReadyPRFinalizerMutation, TransparaAIDraftPRReceipt, error) {
	if strings.TrimSpace(readyContext.RunID) == "" {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(readyContext.FactoryOrderID) == "" {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("factory_order_id is required")
	}
	if strings.TrimSpace(readyContext.OperateCommit) == "" {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("operate_commit is required")
	}
	if strings.TrimSpace(readyContext.DraftPRReceiptRef) == "" || readyContext.DraftPRReceipt == nil {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("%w: draft PR receipt is required before marking an issue-scan PR ready", errIssueScanReadyPRFinalizerAwaitingDraftReceipt)
	}
	receipt := *readyContext.DraftPRReceipt
	repo := strings.ToLower(strings.TrimSpace(receipt.Repository))
	if !ValidTransparaAIRepo(repo) {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("draft PR receipt repository %q is not a Transpara-AI repo", receipt.Repository)
	}
	if strings.ToLower(strings.TrimSpace(readyContext.Repository)) != repo {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("ready context repository %q does not match draft PR receipt repository %q", readyContext.Repository, repo)
	}
	if receipt.PRNumber <= 0 {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("draft PR receipt pr_number is required")
	}
	if strings.TrimSpace(receipt.PRURL) == "" {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("draft PR receipt pr_url is required")
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.HeadSHA), strings.TrimSpace(readyContext.OperateCommit)) {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("draft PR receipt head_sha %q does not match operate_commit %q", receipt.HeadSHA, readyContext.OperateCommit)
	}
	if strings.TrimSpace(receipt.BaseRef) == "" {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("draft PR receipt base_ref is required")
	}
	if !receipt.Draft || !receipt.ReadyForReviewRequired || !receipt.HumanApprovalRequired || !receipt.NoMergeOrDeployClaim {
		return IssueScanReadyPRFinalizerMutation{}, TransparaAIDraftPRReceipt{}, fmt.Errorf("draft PR receipt must prove draft=true, ready-for-review required, Human approval required, and no merge/deploy claim")
	}
	return IssueScanReadyPRFinalizerMutation{
		Kind:                  issueScanReadyPRFinalizerMutationKind,
		LifecycleVersion:      issueScanLifecycleVersion,
		RunID:                 strings.TrimSpace(readyContext.RunID),
		FactoryOrderID:        strings.TrimSpace(readyContext.FactoryOrderID),
		Repository:            repo,
		PRNumber:              receipt.PRNumber,
		PRURL:                 strings.TrimSpace(receipt.PRURL),
		BaseRef:               strings.TrimSpace(receipt.BaseRef),
		BaseSHA:               strings.TrimSpace(receipt.BaseSHA),
		HeadRef:               strings.TrimSpace(receipt.HeadRef),
		HeadSHA:               strings.TrimSpace(receipt.HeadSHA),
		DraftPRReceiptRef:     strings.TrimSpace(readyContext.DraftPRReceiptRef),
		HumanApprovalRequired: true,
		NoMergeOrDeployClaim:  true,
	}, receipt, nil
}

func issueScanReadyStateReviewContext(readyContext IssueScanReadyPRRunnerContext, mutation IssueScanReadyPRFinalizerMutation, state IssueScanReadyPRLiveState) IssueScanReadyStateReviewContext {
	expected := readyContext.ExpectedReadyPREvidence
	expected.PRNumber = mutation.PRNumber
	expected.PRURL = mutation.PRURL
	expected.BaseRef = mutation.BaseRef
	expected.BaseSHA = mutation.BaseSHA
	expected.HeadRef = mutation.HeadRef
	expected.HeadSHA = mutation.HeadSHA
	return IssueScanReadyStateReviewContext{
		Kind:                    issueScanReadyStateReviewContextKind,
		LifecycleVersion:        issueScanLifecycleVersion,
		RunID:                   mutation.RunID,
		FactoryOrderID:          mutation.FactoryOrderID,
		Repository:              mutation.Repository,
		PRNumber:                mutation.PRNumber,
		PRURL:                   mutation.PRURL,
		ReadyStageTaskID:        readyContext.ReadyStageTaskID,
		ImplementationTaskID:    readyContext.ImplementationTaskID,
		OperateCommit:           readyContext.OperateCommit,
		ReadyPRState:            state,
		ExpectedReadyPREvidence: expected,
		BoundaryDisclaimers: compactStrings([]string{
			"ready-state review is not Human approval",
			"ready-state review is not merge or deploy authorization",
			"reviewed_head_sha must match operate_commit",
		}),
	}
}

func validateIssueScanReadyPRLiveState(label string, mutation IssueScanReadyPRFinalizerMutation, state IssueScanReadyPRLiveState, requireCleanCI bool) error {
	repo := strings.ToLower(strings.TrimSpace(state.Repository))
	if repo != mutation.Repository {
		return fmt.Errorf("%s live PR repository %q does not match %q", label, state.Repository, mutation.Repository)
	}
	if state.PRNumber != mutation.PRNumber {
		return fmt.Errorf("%s live PR number %d does not match %d", label, state.PRNumber, mutation.PRNumber)
	}
	if url := strings.TrimSpace(state.PRURL); url != "" && !strings.EqualFold(url, mutation.PRURL) {
		return fmt.Errorf("%s live PR url %q does not match %q", label, url, mutation.PRURL)
	}
	if baseRef := strings.TrimSpace(mutation.BaseRef); baseRef != "" {
		liveBaseRef := strings.TrimSpace(state.BaseRef)
		if liveBaseRef == "" {
			return fmt.Errorf("%s live PR base_ref is required", label)
		}
		if !strings.EqualFold(liveBaseRef, baseRef) {
			return fmt.Errorf("%s live PR base_ref %q does not match approved base_ref %q", label, liveBaseRef, baseRef)
		}
	}
	if !strings.EqualFold(strings.TrimSpace(state.HeadSHA), mutation.HeadSHA) {
		return fmt.Errorf("%s live PR head_sha %q does not match approved head %q", label, state.HeadSHA, mutation.HeadSHA)
	}
	if strings.ToLower(strings.TrimSpace(state.State)) != "open" {
		return fmt.Errorf("%s live PR state %q is not open", label, state.State)
	}
	if state.Draft || !state.ReadyForReview {
		return fmt.Errorf("%s live PR must be non-draft and ready_for_review", label)
	}
	if requireCleanCI {
		if !issueScanReadyStatusOK(state.MergeStateStatus, []string{"clean", "blocked"}) {
			return fmt.Errorf("%s merge_state_status %q is not clean or blocked by required Human review", label, state.MergeStateStatus)
		}
		if !issueScanReadyStatusOK(state.CIStatus, []string{"success", "passed", "green"}) {
			return fmt.Errorf("%s ci_status %q is not successful", label, state.CIStatus)
		}
		switch strings.ToLower(strings.TrimSpace(state.ReviewDecision)) {
		case "", "review_required":
		case "approved":
			return fmt.Errorf("%s review_decision %q indicates Human approval is already satisfied", label, state.ReviewDecision)
		default:
			return fmt.Errorf("%s review_decision %q is not waiting on Human approval", label, state.ReviewDecision)
		}
	}
	return nil
}

// issueScanReadyStateReviewPassingStatuses is the allowlist of ready-state
// receipt statuses the managed finalizer records; anything else is rejected.
var issueScanReadyStateReviewPassingStatuses = []string{"success", "passed", "pass", "no_blockers", "no blockers"}

// ValidateIssueScanReadyStateReviewReceiptShape checks the receipt properties
// that are deterministic from the receipt alone — review_ref presence and a
// passing status from the finalizer's allowlist. The managed chain records
// only passing ready-state receipts (a non-passing receipt stops the chain
// and is never recordable evidence), so local runner-suite package validation
// applies the same rule to expected stdout fixtures. Head binding stays in
// the runtime validator.
func ValidateIssueScanReadyStateReviewReceiptShape(receipt IssueScanReadyStateReviewReceipt) error {
	if strings.TrimSpace(receipt.ReviewRef) == "" {
		return fmt.Errorf("ready-state review_ref is required")
	}
	if !issueScanReadyStatusOK(receipt.Status, issueScanReadyStateReviewPassingStatuses) {
		return fmt.Errorf("ready-state review status %q is not passing", receipt.Status)
	}
	return nil
}

func validateIssueScanReadyStateReviewReceipt(mutation IssueScanReadyPRFinalizerMutation, receipt IssueScanReadyStateReviewReceipt) error {
	if err := ValidateIssueScanReadyStateReviewReceiptShape(receipt); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.ReviewedHeadSHA), mutation.HeadSHA) {
		return fmt.Errorf("ready-state reviewed_head_sha %q does not match approved head %q", receipt.ReviewedHeadSHA, mutation.HeadSHA)
	}
	return nil
}

// validateMarkReadyApproval requires the recorded approval to exactly cover
// the run-derived mutation: same repository (case-insensitive), same PR
// number, same PR URL (case-insensitive), same head SHA (case-insensitive,
// runtime EqualFold semantics), with a non-empty single-use nonce. Anything
// less refuses with ErrIssueScanMarkReadyNotAuthorized before the PR is
// touched.
func validateMarkReadyApproval(mutation IssueScanReadyPRFinalizerMutation, approval MarkReadyTarget) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrIssueScanMarkReadyNotAuthorized, fmt.Sprintf(format, args...))
	}
	if strings.TrimSpace(approval.SingleUseNonce) == "" {
		return refuse("approval carries no single-use nonce")
	}
	if !strings.EqualFold(strings.TrimSpace(approval.Repository), mutation.Repository) {
		return refuse("approved repository %q does not match %q", approval.Repository, mutation.Repository)
	}
	if approval.PRNumber != mutation.PRNumber {
		return refuse("approved PR number %d does not match %d", approval.PRNumber, mutation.PRNumber)
	}
	if !strings.EqualFold(strings.TrimSpace(approval.PRURL), mutation.PRURL) {
		return refuse("approved PR url %q does not match %q", approval.PRURL, mutation.PRURL)
	}
	if !strings.EqualFold(strings.TrimSpace(approval.HeadSHA), mutation.HeadSHA) {
		return refuse("approved head %q does not match %q", approval.HeadSHA, mutation.HeadSHA)
	}
	return nil
}

// issueScanReadyPRBlocked builds the durable blocked outcome for any failure
// after the ready mutation. Re-drafting runs only when the recorded approval
// carries ReDraftOnFailure; its own failure is recorded, never swallowed.
func issueScanReadyPRBlocked(ctx context.Context, client IssueScanReadyPRFinalizerClient, mutation IssueScanReadyPRFinalizerMutation, approval MarkReadyTarget, reviewRef string, cause error) error {
	evidence := IssueScanReadyPRBlockedEvidence{
		Kind:             issueScanReadyPRBlockedEvidenceKind,
		LifecycleVersion: issueScanLifecycleVersion,
		RunID:            mutation.RunID,
		FactoryOrderID:   mutation.FactoryOrderID,
		Repository:       mutation.Repository,
		PRNumber:         mutation.PRNumber,
		PRURL:            mutation.PRURL,
		HeadSHA:          mutation.HeadSHA,
		FailureReason:    cause.Error(),
		ReviewRef:        strings.TrimSpace(reviewRef),
		SingleUseNonce:   approval.SingleUseNonce,
		BoundaryDisclaimers: compactStrings([]string{
			"blocked evidence is not Human approval",
			"the PR is not represented as Human-ready",
			"merge and deploy remain separate governed authorities",
		}),
	}
	if !approval.ReDraftOnFailure {
		evidence.Remediation = IssueScanReadyPRRemediationReDraftUnauthorized
		return &IssueScanReadyPRBlockedError{Evidence: evidence, Cause: cause}
	}
	state, convertErr := client.ConvertToDraft(ctx, mutation)
	if convertErr == nil && (!state.Draft || state.PRNumber != mutation.PRNumber || !strings.EqualFold(strings.TrimSpace(state.Repository), mutation.Repository)) {
		// A "successful" conversion is only successful when the returned live
		// state proves THIS PR is draft again.
		convertErr = fmt.Errorf("convert-to-draft returned unproven state: repository %q pr %d draft %t (want %q %d draft)", state.Repository, state.PRNumber, state.Draft, mutation.Repository, mutation.PRNumber)
	}
	if convertErr != nil {
		evidence.Remediation = IssueScanReadyPRRemediationReDraftFailed
		evidence.RemediationError = convertErr.Error()
		return &IssueScanReadyPRBlockedError{Evidence: evidence, Cause: cause}
	}
	evidence.Remediation = IssueScanReadyPRRemediationReDrafted
	return &IssueScanReadyPRBlockedError{Evidence: evidence, Cause: cause}
}
