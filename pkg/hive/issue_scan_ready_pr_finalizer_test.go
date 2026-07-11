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

func (m *finalizerMockClient) ConvertToDraft(_ context.Context, mutation IssueScanReadyPRFinalizerMutation) (IssueScanReadyPRLiveState, error) {
	m.convertCalls++
	m.lastConvertInput = mutation
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
	result, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContext(), client, passingReviewer, finalizerApproval(false))
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if client.markReadyCalls != 1 || client.convertCalls != 0 {
		t.Fatalf("unexpected client calls: markReady=%d convert=%d", client.markReadyCalls, client.convertCalls)
	}
	if !result.ReadyPREvidence.ReadyForReview || result.ReadyPREvidence.ReadyStateReviewStatus != "pass" {
		t.Fatalf("unexpected evidence: %+v", result.ReadyPREvidence)
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
			_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContext(), client, passingReviewer, tc.approval)
			if err == nil {
				t.Fatal("expected refusal")
			}
			if !errors.Is(err, ErrIssueScanMarkReadyNotAuthorized) {
				t.Fatalf("expected ErrIssueScanMarkReadyNotAuthorized, got %v", err)
			}
			if client.markReadyCalls != 0 {
				t.Fatalf("MarkReadyForReview must not be called on refusal (called %d times)", client.markReadyCalls)
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
// never ready evidence; re-draft only under the recorded flag.
func TestFinalizerBlockedRemediation(t *testing.T) {
	cases := []struct {
		name            string
		reDraft         bool
		reviewer        IssueScanReadyStateReviewRunner
		convertErr      error
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := happyMockClient()
			client.convertErr = tc.convertErr
			client.fetchErr = tc.fetchErr
			_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContext(), client, tc.reviewer, finalizerApproval(tc.reDraft))
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

// TestFinalizerPreMutationClientErrorIsNotBlocked proves a MarkReadyForReview
// transport error (PR never mutated) surfaces as a plain error, not blocked
// evidence, and never records ready evidence or converts.
func TestFinalizerPreMutationClientErrorIsNotBlocked(t *testing.T) {
	client := happyMockClient()
	client.markReadyErr = fmt.Errorf("github unavailable")
	_, err := RunIssueScanReadyPRFinalizer(context.Background(), finalizerTestContext(), client, passingReviewer, finalizerApproval(true))
	if err == nil {
		t.Fatal("expected error")
	}
	var blocked *IssueScanReadyPRBlockedError
	if errors.As(err, &blocked) {
		t.Fatalf("pre-mutation failure must not be a blocked error: %v", err)
	}
	if client.convertCalls != 0 {
		t.Fatalf("ConvertToDraft must not run when the PR was never marked ready")
	}
}
