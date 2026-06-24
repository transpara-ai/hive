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
)

var errIssueScanReadyPRFinalizerAwaitingDraftReceipt = errors.New("issue-scan ready PR finalizer awaiting draft PR receipt")

// IssueScanReadyPRFinalizerClient performs the one live PR-state mutation in
// the terminal issue-scan stage. It marks the already-created draft PR ready for
// review, then reports live PR state for validation. It must not merge, approve,
// deploy, retarget, push, or change protected settings.
type IssueScanReadyPRFinalizerClient interface {
	MarkReadyForReview(context.Context, IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error)
	FetchReadyPRState(context.Context, IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error)
}

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
		return RunIssueScanReadyPRFinalizer(ctx, readyContext, client, reviewer)
	}
}

// RunIssueScanReadyPRFinalizer marks the recorded draft PR ready, runs a
// ready-state exact-head review, verifies live state, and returns terminal
// ready-for-Human evidence for the existing ready-stage recorder. It does not
// approve, merge, deploy, close, retarget, or request Human approval.
func RunIssueScanReadyPRFinalizer(ctx context.Context, readyContext IssueScanReadyPRRunnerContext, client IssueScanReadyPRFinalizerClient, reviewer IssueScanReadyStateReviewRunner) (IssueScanReadyPRRunnerResult, error) {
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
	marked, err := client.MarkReadyForReview(ctx, mutation)
	if err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	if err := validateIssueScanReadyPRLiveState("mark ready", mutation, marked, false); err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	reviewContext := issueScanReadyStateReviewContext(readyContext, mutation, marked)
	review, err := reviewer(ctx, reviewContext)
	if err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	if err := validateIssueScanReadyStateReviewReceipt(mutation, review); err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	live, err := client.FetchReadyPRState(ctx, mutation)
	if err != nil {
		return IssueScanReadyPRRunnerResult{}, err
	}
	if err := validateIssueScanReadyPRLiveState("ready evidence", mutation, live, true); err != nil {
		return IssueScanReadyPRRunnerResult{}, err
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
	}
	return nil
}

func validateIssueScanReadyStateReviewReceipt(mutation IssueScanReadyPRFinalizerMutation, receipt IssueScanReadyStateReviewReceipt) error {
	if strings.TrimSpace(receipt.ReviewRef) == "" {
		return fmt.Errorf("ready-state review_ref is required")
	}
	if !strings.EqualFold(strings.TrimSpace(receipt.ReviewedHeadSHA), mutation.HeadSHA) {
		return fmt.Errorf("ready-state reviewed_head_sha %q does not match approved head %q", receipt.ReviewedHeadSHA, mutation.HeadSHA)
	}
	if !issueScanReadyStatusOK(receipt.Status, []string{"success", "passed", "pass", "no_blockers", "no blockers"}) {
		return fmt.Errorf("ready-state review status %q is not passing", receipt.Status)
	}
	return nil
}
