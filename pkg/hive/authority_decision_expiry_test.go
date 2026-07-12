package hive

import (
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

type authorityDecisionExpiryCase struct {
	name        string
	expiresAt   func() types.Timestamp
	wantAllowed bool
}

func authorityDecisionExpiryCases() []authorityDecisionExpiryCase {
	return []authorityDecisionExpiryCase{
		{name: "expired", expiresAt: func() types.Timestamp { return types.NewTimestamp(time.Now().Add(-time.Hour)) }},
		{name: "finite unexpired", expiresAt: func() types.Timestamp { return types.NewTimestamp(time.Now().Add(time.Hour)) }, wantAllowed: true},
		{name: "zero unbounded", expiresAt: types.ZeroTimestamp, wantAllowed: true},
	}
}

func TestLoadApprovedDraftPRTargetEnforcesDecisionExpiry(t *testing.T) {
	for _, tt := range authorityDecisionExpiryCases() {
		t.Run(tt.name, func(t *testing.T) {
			s, factory, signer, human, conv := newDecisionTestStore(t)
			requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
			want := gateDraftPRTarget()
			seedDraftPRDecisionWithExpiryForTest(t, s, factory, signer, human, conv, requestID, want, tt.expiresAt())

			got, err := LoadApprovedDraftPRTarget(s, requestID.Value())
			assertAuthorityDecisionExpiryResult(t, tt, err)
			if tt.wantAllowed && got != want {
				t.Fatalf("loaded target = %+v, want %+v", got, want)
			}
			if !tt.wantAllowed && got != (DraftPRTarget{}) {
				t.Fatalf("expired decision returned target %+v", got)
			}
		})
	}
}

func TestApprovedIssueScanDraftPRAuthorityRequestForRunEnforcesDecisionExpiry(t *testing.T) {
	for _, tt := range authorityDecisionExpiryCases() {
		t.Run(tt.name, func(t *testing.T) {
			rt, writer, runID, _, _, _ := issueScanReadyStageFixtureForTest(t)
			attachIssueScanAuthorityGraphForTest(t, rt)
			requestResult, err := rt.RaiseIssueScanDraftPRAuthorityRequest(runID, "main", "dddddddddddddddddddddddddddddddddddddddd", "nonce-issue-scan-pr")
			if err != nil {
				t.Fatalf("RaiseIssueScanDraftPRAuthorityRequest: %v", err)
			}
			seedIssueScanDraftPRAuthorityDecisionWithExpiryForTest(t, rt, writer, requestResult, tt.expiresAt())

			requestID, ready, err := rt.approvedIssueScanDraftPRAuthorityRequestForRun(runID)
			assertAuthorityDecisionExpiryResult(t, tt, err)
			if tt.wantAllowed {
				if !ready || requestID != requestResult.RequestID {
					t.Fatalf("ready/request = %t/%s, want true/%s", ready, requestID, requestResult.RequestID)
				}
			} else if ready || !requestID.IsZero() {
				t.Fatalf("expired decision returned ready/request = %t/%s", ready, requestID)
			}
		})
	}
}

func TestApprovedDraftPRAuthorityDecisionForReceiptEnforcesDecisionExpiry(t *testing.T) {
	for _, tt := range authorityDecisionExpiryCases() {
		t.Run(tt.name, func(t *testing.T) {
			s, factory, signer, human, conv := newDecisionTestStore(t)
			requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
			want := gateDraftPRTarget()
			seedDraftPRDecisionWithExpiryForTest(t, s, factory, signer, human, conv, requestID, want, tt.expiresAt())
			rt := &Runtime{store: s}
			receipt := draftPRReceiptForExpiryTest(want)

			_, gotRequestID, got, err := rt.approvedDraftPRAuthorityDecisionForReceipt(receipt)
			assertAuthorityDecisionExpiryResult(t, tt, err)
			if tt.wantAllowed {
				if gotRequestID != requestID || got != want {
					t.Fatalf("request/target = %s/%+v, want %s/%+v", gotRequestID, got, requestID, want)
				}
			} else if !gotRequestID.IsZero() || got != (DraftPRTarget{}) {
				t.Fatalf("expired decision returned request/target = %s/%+v", gotRequestID, got)
			}
		})
	}
}

func assertAuthorityDecisionExpiryResult(t *testing.T, tt authorityDecisionExpiryCase, err error) {
	t.Helper()
	if tt.wantAllowed {
		if err != nil {
			t.Fatalf("unexpired authority should authorize: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "expired") {
		t.Fatalf("expired authority must return an explicit expiry refusal, got %v", err)
	}
}

func seedDraftPRDecisionWithExpiryForTest(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID, requestID types.EventID, target DraftPRTarget, expiresAt types.Timestamp) types.EventID {
	t.Helper()
	content := AuthorityDecisionRecordedContent{
		DecisionID:     requestID.Value(),
		RequestID:      requestID,
		ApproverActor:  human,
		DeciderRole:    "human",
		Outcome:        draftPRApprovedOutcome,
		ApprovedTarget: target.Repository + " " + target.HeadRef,
		ApprovedAction: string(safety.ActionRepoPullRequestCreate),
		Scope:          target.Scope(),
		ExpiresAt:      expiresAt,
		Rationale:      "approved draft PR expiry test target",
	}
	decisionID, err := appendAuthorityDecisionRecorded(s, factory, signer, human, conv, requestID, content)
	if err != nil {
		t.Fatalf("append draft PR authority decision: %v", err)
	}
	return decisionID
}

func seedIssueScanDraftPRAuthorityDecisionWithExpiryForTest(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, requestResult IssueScanDraftPRAuthorityRequestResult, expiresAt types.Timestamp) types.EventID {
	t.Helper()
	target := requestResult.DraftPRTarget
	content := AuthorityDecisionRecordedContent{
		DecisionID:       requestResult.RequestID.Value(),
		RequestID:        requestResult.RequestID,
		ApproverActor:    writer.human,
		DeciderRole:      "human",
		Outcome:          draftPRApprovedOutcome,
		ApprovedTarget:   target.Repository + " " + target.HeadRef,
		ApprovedAction:   string(safety.ActionRepoPullRequestCreate),
		Scope:            target.Scope(),
		ExpiresAt:        expiresAt,
		EvidenceReviewed: []types.EventID{requestResult.ReadyStageTaskID, requestResult.ImplementationTaskID},
		Rationale:        "approved issue-scan draft PR expiry test target",
	}
	decisionID, err := appendAuthorityDecisionRecorded(rt.store, writer.factory, writer.signer, writer.human, writer.conv, requestResult.RequestID, content)
	if err != nil {
		t.Fatalf("append issue-scan draft PR authority decision: %v", err)
	}
	return decisionID
}

func draftPRReceiptForExpiryTest(target DraftPRTarget) TransparaAIDraftPRReceipt {
	return TransparaAIDraftPRReceipt{
		Kind:             transparaAIDraftPRReceiptKind,
		Repository:       target.Repository,
		PRNumber:         41,
		PRURL:            "https://github.com/transpara-ai/docs/pull/41",
		BaseRef:          target.BaseRef,
		BaseSHA:          target.BaseSHA,
		HeadRef:          target.HeadRef,
		HeadSHA:          target.HeadSHA,
		PolicyBundleID:   target.PolicyBundleID,
		PolicyBundleHash: target.PolicyBundleHash,
		AuthorityNonce:   target.SingleUseNonce,
	}
}
