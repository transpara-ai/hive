package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// finalizerMockClient records calls and returns configurable results for each
// of the three client operations.
type finalizerMockClient struct {
	markReadyCalls   int
	fetchCalls       int
	convertCalls     int
	markReadyState   IssueScanReadyPRLiveState
	markReadyErr     error
	fetchState       IssueScanReadyPRLiveState
	fetchErr         error
	convertState     IssueScanReadyPRLiveState
	convertErr       error
	lastConvertInput IssueScanReadyPRFinalizerMutation
}

func (m *finalizerMockClient) MarkReadyForReview(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error) {
	m.markReadyCalls++
	return m.markReadyState, m.markReadyErr
}

func (m *finalizerMockClient) FetchReadyPRState(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error) {
	m.fetchCalls++
	return m.fetchState, m.fetchErr
}

func (m *finalizerMockClient) ConvertToDraft(ctx context.Context, mutation IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error) {
	m.convertCalls++
	m.lastConvertInput = mutation
	if err := ctx.Err(); err != nil {
		return IssueScanReadyPRLiveState{}, err
	}
	return m.convertState, m.convertErr
}

const (
	finalizerTestRepo  = "transpara-ai/docs"
	finalizerTestPR    = 41
	finalizerTestPRURL = "https://github.com/transpara-ai/docs/pull/41"
	finalizerTestHead  = "headsha-approved"
	finalizerTestRunID = "run-finalizer-0001"
	finalizerTestOrder = "fo-finalizer-0001"
)

func finalizerTestReceipt() TransparaAIDraftPRReceipt {
	return TransparaAIDraftPRReceipt{
		Kind:                   "transpara_ai_draft_pr_receipt",
		Repository:             finalizerTestRepo,
		PRNumber:               finalizerTestPR,
		PRURL:                  finalizerTestPRURL,
		BaseRef:                "main",
		BaseSHA:                "basesha",
		HeadRef:                "codex/civic-roles",
		HeadSHA:                finalizerTestHead,
		Draft:                  true,
		HumanApprovalRequired:  true,
		NoMergeOrDeployClaim:   true,
		ReadyForReviewRequired: true,
	}
}

func finalizerTestContext() IssueScanReadyPRRunnerContext {
	receipt := finalizerTestReceipt()
	return IssueScanReadyPRRunnerContext{
		RunID:                finalizerTestRunID,
		FactoryOrderID:       finalizerTestOrder,
		Repository:           finalizerTestRepo,
		DraftPRReceiptRef:    "artifact-receipt-1",
		DraftPRReceipt:       &receipt,
		ReadyStageTaskID:     "task-ready-1",
		BlockerStageTaskID:   "task-blocker-1",
		ImplementationTaskID: "task-impl-1",
		OperateBranch:        "codex/civic-roles",
		OperateCommit:        finalizerTestHead,
	}
}

// finalizerTestContextWithConsumer equips the test context with a counting
// no-op consumer so direct finalizer calls pass the single-use gate; tests
// proving the consumer gate itself construct contexts without it.
func finalizerTestContextWithConsumer(consumeCalls *int) IssueScanReadyPRRunnerContext {
	readyContext := finalizerTestContext()
	readyContext.ConsumeMarkReadyApproval = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation, _ MarkReadyTarget) error {
		*consumeCalls++
		return nil
	}
	return readyContext
}

// finalizerTestContextEchoing additionally equips the context with a lookup
// that keeps returning the given approval, satisfying the pre-mutation
// authority-currency re-check for tests not aimed at that re-check.
func finalizerTestContextEchoing(approval MarkReadyTarget, consumeCalls *int) IssueScanReadyPRRunnerContext {
	readyContext := finalizerTestContextWithConsumer(consumeCalls)
	readyContext.MarkReadyApprovalLookup = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
		return approval, nil
	}
	return readyContext
}

func finalizerLiveState(clean bool) IssueScanReadyPRLiveState {
	state := IssueScanReadyPRLiveState{
		Repository:     finalizerTestRepo,
		PRNumber:       finalizerTestPR,
		PRURL:          finalizerTestPRURL,
		BaseRef:        "main",
		BaseSHA:        "basesha",
		HeadRef:        "codex/civic-roles",
		HeadSHA:        finalizerTestHead,
		State:          "open",
		Draft:          false,
		ReadyForReview: true,
	}
	if clean {
		state.MergeStateStatus = "blocked"
		state.CIStatus = "success"
	}
	return state
}

func passingReviewer(_ context.Context, reviewContext IssueScanReadyStateReviewContext) (IssueScanReadyStateReviewReceipt, error) {
	return IssueScanReadyStateReviewReceipt{
		ReviewRef:       "ready-review-1",
		ReviewedHeadSHA: reviewContext.OperateCommit,
		Status:          "pass",
	}, nil
}

func failingReviewer(_ context.Context, reviewContext IssueScanReadyStateReviewContext) (IssueScanReadyStateReviewReceipt, error) {
	return IssueScanReadyStateReviewReceipt{
		ReviewRef:       "ready-review-1",
		ReviewedHeadSHA: reviewContext.OperateCommit,
		Status:          "request_changes",
	}, nil
}

func finalizerApproval(reDraft bool) MarkReadyTarget {
	return MarkReadyTarget{
		Repository:       finalizerTestRepo,
		PRNumber:         finalizerTestPR,
		PRURL:            finalizerTestPRURL,
		HeadSHA:          finalizerTestHead,
		ReDraftOnFailure: reDraft,
		SingleUseNonce:   "mark-ready-nonce-1",
	}
}

func happyMockClient() *finalizerMockClient {
	return &finalizerMockClient{
		markReadyState: finalizerLiveState(false),
		fetchState:     finalizerLiveState(true),
		convertState:   IssueScanReadyPRLiveState{Repository: finalizerTestRepo, PRNumber: finalizerTestPR, Draft: true, State: "open"},
	}
}

func TestFinalizerSucceedsWithMatchingApproval(t *testing.T) {
	client := happyMockClient()
	consumeCalls := 0
	result, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContextEchoing(finalizerApproval(false), &consumeCalls), client, passingReviewer, finalizerApproval(false))
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if client.markReadyCalls != 1 || client.convertCalls != 0 {
		t.Fatalf("unexpected client calls: markReady=%d convert=%d", client.markReadyCalls, client.convertCalls)
	}
	if consumeCalls != 1 {
		t.Fatalf("approval consumption calls = %d, want exactly 1", consumeCalls)
	}
	if !result.ReadyPREvidence.ReadyForReview || result.ReadyPREvidence.ReadyStateReviewStatus != "pass" {
		t.Fatalf("unexpected evidence: %+v", result.ReadyPREvidence)
	}
}

// TestFinalizerConsumesApprovalBeforeMutation proves the single-use record is
// written durably BEFORE the PR is touched, so a crash after the mutation can
// never leave the approval reusable.
func TestFinalizerConsumesApprovalBeforeMutation(t *testing.T) {
	client := happyMockClient()
	readyContext := finalizerTestContext()
	readyContext.MarkReadyApprovalLookup = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
		return finalizerApproval(false), nil
	}
	readyContext.ConsumeMarkReadyApproval = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation, _ MarkReadyTarget) error {
		if client.markReadyCalls != 0 {
			t.Fatal("approval must be consumed before MarkReadyForReview is called")
		}
		return nil
	}
	if _, err := RunIssueScanReadyPRFinalizer(context.Background(), readyContext, client, passingReviewer, finalizerApproval(false)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

// TestFinalizerRefusesWithoutConsumer proves the fail-closed default: a
// context that never received the runtime-injected single-use consumer
// refuses before any mutation.
func TestFinalizerRefusesWithoutConsumer(t *testing.T) {
	client := happyMockClient()
	readyContext := finalizerTestContext()
	readyContext.MarkReadyApprovalLookup = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
		return finalizerApproval(false), nil
	}
	_, err := RunIssueScanReadyPRFinalizer(context.Background(), readyContext, client, passingReviewer, finalizerApproval(false))
	if err == nil || !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) || client.markReadyCalls != 0 {
		t.Fatalf("expected fail-closed refusal with no consumer, err=%v markReadyCalls=%d", err, client.markReadyCalls)
	}
}

// TestFinalizerRefusesWhenConsumptionFails proves an already-consumed (or
// unrecordable) single-use approval refuses before any mutation.
func TestFinalizerRefusesWhenConsumptionFails(t *testing.T) {
	client := happyMockClient()
	readyContext := finalizerTestContext()
	readyContext.MarkReadyApprovalLookup = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
		return finalizerApproval(false), nil
	}
	readyContext.ConsumeMarkReadyApproval = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation, _ MarkReadyTarget) error {
		return fmt.Errorf("single-use nonce already consumed")
	}
	_, err := RunIssueScanReadyPRFinalizer(context.Background(), readyContext, client, passingReviewer, finalizerApproval(false))
	if err == nil || !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) || client.markReadyCalls != 0 {
		t.Fatalf("expected consumption refusal before mutation, err=%v markReadyCalls=%d", err, client.markReadyCalls)
	}
}

// TestFinalizerRefusesWithoutMatchingApproval proves the gate refuses BEFORE
// any GitHub mutation for the whole mismatch domain.
func TestFinalizerRefusesWithoutMatchingApproval(t *testing.T) {
	cases := []struct {
		name     string
		approval MarkReadyTarget
	}{
		{"zero approval", MarkReadyTarget{}},
		{"repository mismatch", func() MarkReadyTarget { a := finalizerApproval(false); a.Repository = "transpara-ai/hive"; return a }()},
		{"pr number mismatch", func() MarkReadyTarget { a := finalizerApproval(false); a.PRNumber = 7; return a }()},
		{"head sha mismatch", func() MarkReadyTarget { a := finalizerApproval(false); a.HeadSHA = "other-head"; return a }()},
		{"pr url mismatch", func() MarkReadyTarget {
			a := finalizerApproval(false)
			a.PRURL = "https://github.com/transpara-ai/docs/pull/9"
			return a
		}()},
		{"empty nonce", func() MarkReadyTarget { a := finalizerApproval(false); a.SingleUseNonce = ""; return a }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := happyMockClient()
			consumeCalls := 0
			_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContextEchoing(tc.approval, &consumeCalls), client, passingReviewer, tc.approval)
			if err == nil {
				t.Fatal("expected refusal")
			}
			if !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) {
				t.Fatalf("expected ErrIssueScanMarkReadyNotAuthorized, got %v", err)
			}
			if client.markReadyCalls != 0 {
				t.Fatalf("MarkReadyForReview must not be called on refusal (called %d times)", client.markReadyCalls)
			}
			if consumeCalls != 0 {
				t.Fatalf("a mismatched approval must never be consumed (consumed %d times)", consumeCalls)
			}
		})
	}
}

func TestFinalizerRunnerRefusesWhenLookupFails(t *testing.T) {
	client := happyMockClient()
	readyContext := finalizerTestContext()
	readyContext.MarkReadyApprovalLookup = func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
		return MarkReadyTarget{}, fmt.Errorf("no recorded mark-ready approval")
	}
	runner := NewIssueScanReadyPRFinalizerRunner(client, passingReviewer)
	_, err := runner(context.Background(), readyContext)
	if err == nil || !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) || client.markReadyCalls != 0 {
		t.Fatalf("expected authorization refusal before mutation, err=%v markReadyCalls=%d", err, client.markReadyCalls)
	}
}

// TestFinalizerRunnerRefusesWithoutLookup proves the fail-closed default: a
// context that never received the runtime-injected lookup refuses outright.
func TestFinalizerRunnerRefusesWithoutLookup(t *testing.T) {
	client := happyMockClient()
	runner := NewIssueScanReadyPRFinalizerRunner(client, passingReviewer)
	_, err := runner(context.Background(), finalizerTestContext())
	if err == nil || !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) || client.markReadyCalls != 0 {
		t.Fatalf("expected fail-closed refusal with no lookup, err=%v markReadyCalls=%d", err, client.markReadyCalls)
	}
}

// TestFinalizerBlockedRemediation proves the post-mutation failure domain:
// never ready evidence; re-draft only under the recorded flag; a re-draft is
// reported successful only when the returned live state proves Draft.
func TestFinalizerBlockedRemediation(t *testing.T) {
	cases := []struct {
		name            string
		reDraft         bool
		reviewer        IssueScanReadyStateReviewRunner
		convertErr      error
		convertState    *IssueScanReadyPRLiveState
		fetchErr        error
		wantRemediation IssueScanReadyPRRemediation
		wantConverts    int
	}{
		{
			name:            "review fails without re-draft scope",
			reDraft:         false,
			reviewer:        failingReviewer,
			wantRemediation: IssueScanReadyPRRemediationReDraftUnauthorized,
			wantConverts:    0,
		},
		{
			name:            "review fails with re-draft scope",
			reDraft:         true,
			reviewer:        failingReviewer,
			wantRemediation: IssueScanReadyPRRemediationReDrafted,
			wantConverts:    1,
		},
		{
			name:            "review fails and re-draft fails",
			reDraft:         true,
			reviewer:        failingReviewer,
			convertErr:      fmt.Errorf("graphql: convert rejected"),
			wantRemediation: IssueScanReadyPRRemediationReDraftFailed,
			wantConverts:    1,
		},
		{
			name:    "reviewer returns error",
			reDraft: true,
			reviewer: func(_ context.Context, _ IssueScanReadyStateReviewContext) (IssueScanReadyStateReviewReceipt, error) {
				return IssueScanReadyStateReviewReceipt{}, fmt.Errorf("review runner crashed")
			},
			wantRemediation: IssueScanReadyPRRemediationReDrafted,
			wantConverts:    1,
		},
		{
			name:            "final live fetch fails",
			reDraft:         true,
			reviewer:        passingReviewer,
			fetchErr:        fmt.Errorf("github unavailable"),
			wantRemediation: IssueScanReadyPRRemediationReDrafted,
			wantConverts:    1,
		},
		{
			name:            "re-draft returns non-draft state",
			reDraft:         true,
			reviewer:        failingReviewer,
			convertState:    &IssueScanReadyPRLiveState{Repository: finalizerTestRepo, PRNumber: finalizerTestPR, Draft: false, State: "open"},
			wantRemediation: IssueScanReadyPRRemediationReDraftFailed,
			wantConverts:    1,
		},
		{
			name:            "re-draft returns mismatched PR identity",
			reDraft:         true,
			reviewer:        failingReviewer,
			convertState:    &IssueScanReadyPRLiveState{Repository: finalizerTestRepo, PRNumber: 999, Draft: true, State: "open"},
			wantRemediation: IssueScanReadyPRRemediationReDraftFailed,
			wantConverts:    1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := happyMockClient()
			client.convertErr = tc.convertErr
			client.fetchErr = tc.fetchErr
			if tc.convertState != nil {
				client.convertState = *tc.convertState
			}
			consumeCalls := 0
			_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContextEchoing(finalizerApproval(tc.reDraft), &consumeCalls), client, tc.reviewer, finalizerApproval(tc.reDraft))
			if err == nil {
				t.Fatal("expected blocked error")
			}
			var blocked *IssueScanReadyPRBlockedError
			if !errors.As(err, &blocked) {
				t.Fatalf("expected IssueScanReadyPRBlockedError, got %v", err)
			}
			evidence := blocked.Evidence
			if evidence.Remediation != tc.wantRemediation {
				t.Fatalf("remediation = %q, want %q", evidence.Remediation, tc.wantRemediation)
			}
			if tc.wantRemediation == IssueScanReadyPRRemediationReDraftFailed && evidence.RemediationError == "" {
				t.Fatalf("re_draft_failed evidence must carry the remediation error: %+v", evidence)
			}
			if client.convertCalls != tc.wantConverts {
				t.Fatalf("ConvertToDraft calls = %d, want %d", client.convertCalls, tc.wantConverts)
			}
			if evidence.Repository != finalizerTestRepo || evidence.PRNumber != finalizerTestPR || !strings.EqualFold(evidence.HeadSHA, finalizerTestHead) {
				t.Fatalf("evidence identity wrong: %+v", evidence)
			}
			if evidence.FailureReason == "" || evidence.RunID != finalizerTestRunID || evidence.FactoryOrderID != finalizerTestOrder {
				t.Fatalf("evidence incomplete: %+v", evidence)
			}
			if evidence.SingleUseNonce != "mark-ready-nonce-1" {
				t.Fatalf("evidence must carry the consumed approval nonce, got %q", evidence.SingleUseNonce)
			}
		})
	}
}

// TestFinalizerRefusesWithoutLookupDirect proves the authority-currency
// re-check is fail-closed at the core, not just in the runner wrapper: a
// context with no lookup refuses before any mutation.
func TestFinalizerRefusesWithoutLookupDirect(t *testing.T) {
	client := happyMockClient()
	consumeCalls := 0
	_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContextWithConsumer(&consumeCalls), client, passingReviewer, finalizerApproval(false))
	if err == nil || !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) || client.markReadyCalls != 0 {
		t.Fatalf("expected fail-closed refusal with no lookup on the context, err=%v markReadyCalls=%d", err, client.markReadyCalls)
	}
}

// TestFinalizerRevalidatesApprovalBeforeMutation proves the authority is
// re-checked immediately before the side effect: an approval that expires or
// is superseded between consumption and mutation refuses (CFAR hive#272
// round 4, finding 1).
func TestFinalizerRevalidatesApprovalBeforeMutation(t *testing.T) {
	cases := []struct {
		name   string
		lookup MarkReadyApprovalLookup
	}{
		{
			name: "authority withdrawn after consumption",
			lookup: func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
				return MarkReadyTarget{}, fmt.Errorf("latest mark-ready decision is now a denial")
			},
		},
		{
			name: "authority replaced by a different approval",
			lookup: func(_ context.Context, _ IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
				replaced := finalizerApproval(false)
				replaced.SingleUseNonce = "a-newer-different-nonce"
				return replaced, nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := happyMockClient()
			consumeCalls := 0
			readyContext := finalizerTestContextWithConsumer(&consumeCalls)
			readyContext.MarkReadyApprovalLookup = tc.lookup
			_, err := RunIssueScanReadyPRFinalizer(context.Background(), readyContext, client, passingReviewer, finalizerApproval(false))
			if err == nil || !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) {
				t.Fatalf("expected authority-currency refusal, got %v", err)
			}
			if client.markReadyCalls != 0 {
				t.Fatalf("MarkReadyForReview must not run after stale authority (called %d times)", client.markReadyCalls)
			}
		})
	}
}

// TestFinalizerReDraftSurvivesCallerCancellation proves the authorized
// remediation runs even when the caller's context was canceled after the
// mutation: the compensating request must not be disabled by shutdown (CFAR
// hive#272 round 4, finding 3).
func TestFinalizerReDraftSurvivesCallerCancellation(t *testing.T) {
	client := happyMockClient()
	consumeCalls := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancellingReviewer := func(_ context.Context, _ IssueScanReadyStateReviewContext) (IssueScanReadyStateReviewReceipt, error) {
		cancel()
		return IssueScanReadyStateReviewReceipt{}, fmt.Errorf("review interrupted by shutdown")
	}
	_, err := RunIssueScanReadyPRFinalizer(ctx, finalizerTestContextEchoing(finalizerApproval(true), &consumeCalls), client, cancellingReviewer, finalizerApproval(true))
	if err == nil {
		t.Fatal("expected blocked error")
	}
	var blocked *IssueScanReadyPRBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected blocked error, got %v", err)
	}
	if blocked.Evidence.Remediation != IssueScanReadyPRRemediationReDrafted || client.convertCalls != 1 {
		t.Fatalf("remediation = %q (error %q), convertCalls = %d; the re-draft must run under a detached context", blocked.Evidence.Remediation, blocked.Evidence.RemediationError, client.convertCalls)
	}
}

// TestFinalizerProvenNotMutatedErrorIsNotBlocked proves the ONLY path that
// bypasses blocked evidence on a MarkReadyForReview error: the client proved
// the PR was never mutated by wrapping ErrIssueScanMarkReadyNotMutated.
func TestFinalizerProvenNotMutatedErrorIsNotBlocked(t *testing.T) {
	client := happyMockClient()
	consumeCalls := 0
	client.markReadyErr = fmt.Errorf("%w: preflight head mismatch", ErrIssueScanMarkReadyNotMutated)
	_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContextEchoing(finalizerApproval(true), &consumeCalls), client, passingReviewer, finalizerApproval(true))
	if err == nil {
		t.Fatal("expected error")
	}
	var blocked *IssueScanReadyPRBlockedError
	if errors.As(err, &blocked) {
		t.Fatalf("proven not-mutated failure must not be a blocked error: %v", err)
	}
	if client.convertCalls != 0 {
		t.Fatalf("ConvertToDraft must not run when the PR was proven unmutated")
	}
}

// TestFinalizerUnprovenMarkReadyErrorIsBlocked proves the fail-safe default:
// a MarkReadyForReview error WITHOUT the not-mutated proof is treated as an
// indeterminate/post-mutation failure — durable blocked evidence plus the
// recorded remediation, never a silent plain error.
func TestFinalizerUnprovenMarkReadyErrorIsBlocked(t *testing.T) {
	client := happyMockClient()
	consumeCalls := 0
	client.markReadyErr = fmt.Errorf("github unavailable while reconciling mutation state")
	_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContextEchoing(finalizerApproval(true), &consumeCalls), client, passingReviewer, finalizerApproval(true))
	if err == nil {
		t.Fatal("expected error")
	}
	var blocked *IssueScanReadyPRBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("unproven mark-ready failure must produce blocked evidence, got %v", err)
	}
	if blocked.Evidence.Remediation != IssueScanReadyPRRemediationReDrafted || client.convertCalls != 1 {
		t.Fatalf("remediation = %q, convertCalls = %d; want re_drafted under the recorded flag", blocked.Evidence.Remediation, client.convertCalls)
	}
}
