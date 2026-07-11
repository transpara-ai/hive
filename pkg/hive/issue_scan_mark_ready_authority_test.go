package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

func markReadyTestTarget() MarkReadyTarget {
	return MarkReadyTarget{
		Repository:       "transpara-ai/docs",
		PRNumber:         41,
		PRURL:            "https://github.com/transpara-ai/docs/pull/41",
		HeadSHA:          "headsha-approved",
		ReDraftOnFailure: true,
		SingleUseNonce:   "mark-ready-nonce-1",
	}
}

// seedMarkReadyAnchor appends an authority.requested anchor so the decision's
// causal link (CAUSALITY invariant) references a real stored event, mirroring
// seedPendingDraftPRRequest.
func seedMarkReadyAnchor(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID) types.EventID {
	t.Helper()
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	anchorCauses := []types.EventID{head.Unwrap().ID()}
	anchor, err := factory.Create(event.EventTypeAuthorityRequested, human, event.AuthorityRequestContent{
		Action:        string(safety.ActionRepoPullRequestMarkReady),
		Actor:         human,
		Level:         event.AuthorityLevelRequired,
		Justification: "Mark the reviewed draft PR ready",
		Causes:        types.MustNonEmpty(anchorCauses),
	}, anchorCauses, conv, s, signer)
	if err != nil {
		t.Fatalf("create authority.requested: %v", err)
	}
	storedAnchor, err := s.Append(anchor)
	if err != nil {
		t.Fatalf("append authority.requested: %v", err)
	}
	return storedAnchor.ID()
}

// seedMarkReadyDecision mirrors seedDraftPRDecision for the mark-ready action.
func seedMarkReadyDecision(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID, requestID types.EventID, outcome string, target MarkReadyTarget) {
	t.Helper()
	seedMarkReadyDecisionWithRole(t, s, factory, signer, human, conv, requestID, outcome, target, "human")
}

func seedMarkReadyDecisionWithRole(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID, requestID types.EventID, outcome string, target MarkReadyTarget, deciderRole string) {
	t.Helper()
	content := AuthorityDecisionRecordedContent{
		DecisionID:     requestID.Value(),
		RequestID:      requestID,
		ApproverActor:  human,
		DeciderRole:    deciderRole,
		Outcome:        outcome,
		ApprovedTarget: target.Repository + " #41",
		ApprovedAction: string(safety.ActionRepoPullRequestMarkReady),
		Scope:          target.Scope(),
		Rationale:      "reviewed the draft PR",
	}
	if _, err := appendAuthorityDecisionRecorded(s, factory, signer, human, conv, requestID, content); err != nil {
		t.Fatalf("append authority.decision.recorded (%s): %v", outcome, err)
	}
}

func TestMarkReadyActionIsProtected(t *testing.T) {
	if !safety.IsProtectedAction(safety.ActionRepoPullRequestMarkReady) {
		t.Fatal("pull_request.mark_ready must be a protected action")
	}
	if got := safety.DefaultOutcome(safety.ActionRepoPullRequestMarkReady); got != safety.ApprovalRequired {
		t.Fatalf("default outcome = %q, want ApprovalRequired", got)
	}
}

func TestMarkReadyScopeRoundTrip(t *testing.T) {
	target := markReadyTestTarget()
	got, err := ParseMarkReadyScope(target.Scope())
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if got != target {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, target)
	}
}

func TestParseMarkReadyScopeRejectsInvalid(t *testing.T) {
	valid := markReadyTestTarget().Scope()
	cases := []struct {
		name  string
		scope []string
	}{
		{"nil", nil},
		{"short", valid[:len(valid)-1]},
		{"long", append(append([]string{}, valid...), "extra")},
		{"wrong discriminator", append([]string{string(safety.ActionRepoPullRequestCreate)}, valid[1:]...)},
		{"non-numeric pr number", func() []string { s := append([]string{}, valid...); s[2] = "forty-one"; return s }()},
		{"zero pr number", func() []string { s := append([]string{}, valid...); s[2] = "0"; return s }()},
		{"non-bool re-draft flag", func() []string { s := append([]string{}, valid...); s[5] = "yes"; return s }()},
		{"empty repository", func() []string { s := append([]string{}, valid...); s[1] = ""; return s }()},
		{"empty head sha", func() []string { s := append([]string{}, valid...); s[4] = ""; return s }()},
		{"empty nonce", func() []string { s := append([]string{}, valid...); s[6] = ""; return s }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseMarkReadyScope(tc.scope); err == nil {
				t.Fatalf("expected parse error for %v", tc.scope)
			}
		})
	}
}

// TestFindApprovedMarkReadyTarget proves the store-backed lookup over its whole
// input domain: approved-and-matching succeeds; everything else refuses.
func TestFindApprovedMarkReadyTarget(t *testing.T) {
	target := markReadyTestTarget()

	seed := func(t *testing.T, outcome string, seedTarget MarkReadyTarget) *store.InMemoryStore {
		s, factory, signer, human, conv := newDecisionTestStore(t)
		requestID := seedMarkReadyAnchor(t, s, factory, signer, human, conv)
		seedMarkReadyDecision(t, s, factory, signer, human, conv, requestID, outcome, seedTarget)
		return s
	}

	t.Run("approved and matching", func(t *testing.T) {
		s := seed(t, "approved", target)
		got, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, target.HeadSHA)
		if err != nil {
			t.Fatalf("expected approval, got %v", err)
		}
		if got != target {
			t.Fatalf("got %+v want %+v", got, target)
		}
	})

	t.Run("no decision recorded", func(t *testing.T) {
		s, _, _, _, _ := newDecisionTestStore(t)
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, target.HeadSHA); err == nil {
			t.Fatal("expected refusal with no recorded decision")
		}
	})

	t.Run("denied decision refuses", func(t *testing.T) {
		s := seed(t, "denied", target)
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, target.HeadSHA); err == nil {
			t.Fatal("expected refusal for denied decision")
		}
	})

	t.Run("repository mismatch refuses", func(t *testing.T) {
		s := seed(t, "approved", target)
		if _, err := FindApprovedMarkReadyTarget(s, "transpara-ai/hive", target.PRNumber, target.HeadSHA); err == nil {
			t.Fatal("expected refusal for repository mismatch")
		}
	})

	t.Run("pr number mismatch refuses", func(t *testing.T) {
		s := seed(t, "approved", target)
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, 42, target.HeadSHA); err == nil {
			t.Fatal("expected refusal for pr number mismatch")
		}
	})

	t.Run("head sha mismatch refuses", func(t *testing.T) {
		s := seed(t, "approved", target)
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, "some-other-head"); err == nil {
			t.Fatal("expected refusal for head mismatch")
		}
	})

	t.Run("head sha match is case-insensitive like the runtime", func(t *testing.T) {
		s := seed(t, "approved", target)
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, strings.ToUpper(target.HeadSHA)); err != nil {
			t.Fatalf("expected case-insensitive head match, got %v", err)
		}
	})

	t.Run("non-human decider never authorizes", func(t *testing.T) {
		s, factory, signer, human, conv := newDecisionTestStore(t)
		requestID := seedMarkReadyAnchor(t, s, factory, signer, human, conv)
		seedMarkReadyDecisionWithRole(t, s, factory, signer, human, conv, requestID, "approved", target, "operator")
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, target.HeadSHA); err == nil {
			t.Fatal("a non-human decision must never satisfy the human mark-ready boundary")
		}
	})

	t.Run("non-human decision never shadows a human decision", func(t *testing.T) {
		s, factory, signer, human, conv := newDecisionTestStore(t)
		requestID := seedMarkReadyAnchor(t, s, factory, signer, human, conv)
		seedMarkReadyDecision(t, s, factory, signer, human, conv, requestID, "approved", target)
		seedMarkReadyDecisionWithRole(t, s, factory, signer, human, conv, requestID, "denied", target, "operator")
		if _, err := FindApprovedMarkReadyTarget(s, target.Repository, target.PRNumber, target.HeadSHA); err != nil {
			t.Fatalf("only human decisions carry mark-ready authority in either direction, got %v", err)
		}
	})

	t.Run("draft-pr-create approval never authorizes readying", func(t *testing.T) {
		s, factory, signer, human, conv := newDecisionTestStore(t)
		requestID := seedMarkReadyAnchor(t, s, factory, signer, human, conv)
		seedDraftPRDecision(t, s, factory, signer, human, conv, requestID, "approved", gateDraftPRTarget())
		if _, err := FindApprovedMarkReadyTarget(s, gateDraftPRTarget().Repository, 41, gateDraftPRTarget().HeadSHA); err == nil {
			t.Fatal("a pull_request.create approval must never satisfy the mark-ready gate")
		}
	})
}

// seedMarkReadyApprovalForReadyTest records an approved mark-ready decision
// matching the ready evidence a lifecycle test attached, so the finalizer's
// authority gate (hive#263) passes and the test exercises the behavior beyond
// it. Tests proving the gate itself refuse by NOT calling this.
func seedMarkReadyApprovalForReadyTest(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, evidence IssueScanReadyPREvidence, reDraft bool) {
	t.Helper()
	seedMarkReadyApprovalTargetForTest(t, rt, writer, MarkReadyTarget{
		Repository:       strings.ToLower(strings.TrimSpace(evidence.Repository)),
		PRNumber:         evidence.PRNumber,
		PRURL:            strings.TrimSpace(evidence.PRURL),
		HeadSHA:          strings.TrimSpace(evidence.HeadSHA),
		ReDraftOnFailure: reDraft,
		SingleUseNonce:   "mark-ready-nonce-lifecycle-test",
	})
}

func seedMarkReadyApprovalTargetForTest(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, target MarkReadyTarget) {
	t.Helper()
	requestID := seedMarkReadyAnchor(t, rt.store, writer.factory, writer.signer, writer.human, writer.conv)
	seedMarkReadyDecision(t, rt.store, writer.factory, writer.signer, writer.human, writer.conv, requestID, "approved", target)
}

func markReadyConsumptionFixturesForTest(runID, orderID string, evidence IssueScanReadyPREvidence, nonce string) (IssueScanReadyPRFinalizerMutation, MarkReadyTarget) {
	mutation := IssueScanReadyPRFinalizerMutation{
		RunID:          runID,
		FactoryOrderID: orderID,
		Repository:     strings.ToLower(strings.TrimSpace(evidence.Repository)),
		PRNumber:       evidence.PRNumber,
		PRURL:          strings.TrimSpace(evidence.PRURL),
		HeadSHA:        strings.TrimSpace(evidence.HeadSHA),
	}
	target := MarkReadyTarget{
		Repository:     mutation.Repository,
		PRNumber:       mutation.PRNumber,
		PRURL:          mutation.PRURL,
		HeadSHA:        mutation.HeadSHA,
		SingleUseNonce: nonce,
	}
	return mutation, target
}

// TestIssueScanMarkReadyApprovalConsumerIsSingleUse proves the durable
// consumption record: the same nonce never authorizes twice, even after a
// re-draft returns the PR to draft state (CFAR hive#272 round 1, finding 1).
func TestIssueScanMarkReadyApprovalConsumerIsSingleUse(t *testing.T) {
	rt, _, runID, orderID, _, _ := issueScanReadyStageFixtureForTest(t)
	evidence := issueScanReadyPREvidenceForTest(runID, orderID)
	mutation, target := markReadyConsumptionFixturesForTest(runID, orderID, evidence, "nonce-single-use-1")
	consume := rt.issueScanMarkReadyApprovalConsumer(runID)
	if err := consume(context.Background(), mutation, target); err != nil {
		t.Fatalf("first consumption: %v", err)
	}
	if err := consume(context.Background(), mutation, target); err == nil {
		t.Fatal("second consumption of the same nonce must refuse")
	}
	other := target
	other.SingleUseNonce = "nonce-single-use-2"
	if err := consume(context.Background(), mutation, other); err != nil {
		t.Fatalf("an independent nonce must remain consumable: %v", err)
	}
}

// TestIssueScanMarkReadyApprovalConsumerFailsClosedOnUnreadableRecord proves
// the doctrine case: a consumption record that cannot be parsed can never
// prove the nonce unconsumed, so consumption refuses.
func TestIssueScanMarkReadyApprovalConsumerFailsClosedOnUnreadableRecord(t *testing.T) {
	rt, writer, runID, orderID, _, readyStage := issueScanReadyStageFixtureForTest(t)
	if err := rt.tasks.AddArtifact(writer.human, readyStage.ID, IssueScanMarkReadyConsumptionArtifactLabel, "application/json", "{not-json", []types.EventID{readyStage.ID}, writer.conv); err != nil {
		t.Fatalf("seed unreadable consumption record: %v", err)
	}
	evidence := issueScanReadyPREvidenceForTest(runID, orderID)
	mutation, target := markReadyConsumptionFixturesForTest(runID, orderID, evidence, "nonce-single-use-1")
	consume := rt.issueScanMarkReadyApprovalConsumer(runID)
	if err := consume(context.Background(), mutation, target); err == nil {
		t.Fatal("an unreadable consumption record must refuse consumption (fail closed)")
	}
}

func appendMarkReadyConsumptionRecordForTest(t *testing.T, rt *Runtime, writer *operatorRunLaunchWriter, taskID types.EventID, nonce, claimID string) {
	t.Helper()
	record := issueScanMarkReadyConsumptionRecord{
		Kind:             issueScanMarkReadyConsumptionKind,
		LifecycleVersion: issueScanLifecycleVersion,
		RunID:            "run-foreign-0001",
		SingleUseNonce:   nonce,
		ClaimID:          claimID,
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatalf("marshal foreign consumption record: %v", err)
	}
	if err := rt.tasks.AddArtifact(writer.human, taskID, IssueScanMarkReadyConsumptionArtifactLabel, "application/json", string(body), []types.EventID{taskID}, writer.conv); err != nil {
		t.Fatalf("append foreign consumption record: %v", err)
	}
}

// TestIssueScanMarkReadyConsumptionIsGlobalAcrossTasks proves one human
// approval is ONE transition regardless of which run consumes it: a
// consumption record on any other task still refuses this run's consumer
// (CFAR hive#272 round 2, finding 2).
func TestIssueScanMarkReadyConsumptionIsGlobalAcrossTasks(t *testing.T) {
	rt, writer, runID, orderID, implementationTask, _ := issueScanReadyStageFixtureForTest(t)
	appendMarkReadyConsumptionRecordForTest(t, rt, writer, implementationTask.ID, "nonce-single-use-1", "claim-foreign")
	evidence := issueScanReadyPREvidenceForTest(runID, orderID)
	mutation, target := markReadyConsumptionFixturesForTest(runID, orderID, evidence, "nonce-single-use-1")
	consume := rt.issueScanMarkReadyApprovalConsumer(runID)
	if err := consume(context.Background(), mutation, target); err == nil {
		t.Fatal("a consumption record on another task must still refuse the nonce (global single-use)")
	}
}

// TestIssueScanMarkReadyNonceClaimsAreOrderedOldestFirst proves the total
// order the concurrent-claim resolution relies on: the chain's event order
// makes exactly one claimant the oldest, so append-then-verify-winner is
// race-safe without store-level compare-and-set (CFAR hive#272 round 2,
// finding 3).
func TestIssueScanMarkReadyNonceClaimsAreOrderedOldestFirst(t *testing.T) {
	rt, writer, _, _, implementationTask, readyStage := issueScanReadyStageFixtureForTest(t)
	appendMarkReadyConsumptionRecordForTest(t, rt, writer, readyStage.ID, "nonce-race-1", "claim-a")
	appendMarkReadyConsumptionRecordForTest(t, rt, writer, implementationTask.ID, "nonce-race-1", "claim-b")
	claims, err := rt.issueScanMarkReadyNonceClaims("nonce-race-1")
	if err != nil {
		t.Fatalf("issueScanMarkReadyNonceClaims: %v", err)
	}
	if len(claims) != 2 || claims[0].ClaimID != "claim-a" || claims[1].ClaimID != "claim-b" {
		t.Fatalf("claims = %+v, want oldest-first [claim-a claim-b]", claims)
	}
}

// TestIssueScanMarkReadyChecksSurviveArtifactPagination proves the
// authorization reads page through the WHOLE store: a consumption record and
// blocked evidence buried under more than one page of newer artifact events
// still refuse reuse and re-runs (CFAR hive#272 round 2, finding 4).
func TestIssueScanMarkReadyChecksSurviveArtifactPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("appends >1000 events")
	}
	rt, writer, runID, orderID, implementationTask, readyStage := issueScanReadyStageFixtureForTest(t)
	readyEvidence := issueScanReadyPREvidenceForTest(runID, orderID)
	if err := attachIssueScanDraftPRReceiptForReadyTest(t, rt, writer, readyStage.ID, readyEvidence); err != nil {
		t.Fatalf("attach draft PR receipt: %v", err)
	}
	mutation, target := markReadyConsumptionFixturesForTest(runID, orderID, readyEvidence, "nonce-buried-1")
	consume := rt.issueScanMarkReadyApprovalConsumer(runID)
	if err := consume(context.Background(), mutation, target); err != nil {
		t.Fatalf("first consumption: %v", err)
	}
	if _, err := rt.RecordIssueScanReadyPRBlocked(runID, IssueScanReadyPRBlockedEvidence{
		RunID:          runID,
		FactoryOrderID: orderID,
		Repository:     readyEvidence.Repository,
		PRNumber:       readyEvidence.PRNumber,
		PRURL:          readyEvidence.PRURL,
		HeadSHA:        readyEvidence.HeadSHA,
		FailureReason:  "ready-state review failed after the mutation",
		Remediation:    IssueScanReadyPRRemediationReDraftUnauthorized,
		SingleUseNonce: "nonce-buried-1",
	}); err != nil {
		t.Fatalf("record blocked evidence: %v", err)
	}
	for i := 0; i < 1050; i++ {
		if err := rt.tasks.AddArtifact(writer.human, implementationTask.ID, "pagination-noise", "text/plain", fmt.Sprintf("noise %d", i), []types.EventID{implementationTask.ID}, writer.conv); err != nil {
			t.Fatalf("append noise artifact %d: %v", i, err)
		}
	}
	if err := consume(context.Background(), mutation, target); err == nil {
		t.Fatal("a consumption record buried beyond one artifact page must still refuse reuse")
	}
	_, _, err := rt.issueScanReadyPRRunnerContext(runID)
	if err == nil || !errors.Is(err, ErrIssueScanReadyPRBlockedPendingHuman) {
		t.Fatalf("blocked evidence buried beyond one artifact page must still be terminal, got %v", err)
	}
}
