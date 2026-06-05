package hive

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

// gateApprovedTitle / gateApprovedBody are the exact content a human approved in
// the seeded decision below. The seeded scope's TitleHash/BodyHash are the real
// sha256HexPrefixed digests of these strings, so VerifyDraftPRContent PASSES for
// them and FAILS for anything else.
const (
	gateApprovedTitle = "[codex] Document the civic roles"
	gateApprovedBody  = "## Summary\nDocument the civic roles.\n"
)

// gateDraftPRTarget is the authoritative target a decision approves. Its
// TitleHash/BodyHash bind to gateApprovedTitle/gateApprovedBody.
func gateDraftPRTarget() DraftPRTarget {
	return DraftPRTarget{
		Repository:       "transpara-ai/docs",
		BaseRef:          "main",
		BaseSHA:          "basesha-approved",
		HeadRef:          "codex/civic-roles",
		HeadSHA:          "headsha-approved",
		TitleHash:        sha256HexPrefixed([]byte(gateApprovedTitle)),
		BodyHash:         sha256HexPrefixed([]byte(gateApprovedBody)),
		PolicyBundleID:   "df-v3.9.20-docs-draft-pr-create-only",
		PolicyBundleHash: "sha256:bundle",
		SingleUseNonce:   "nonce-approved-1",
	}
}

// seedDraftPRDecision appends an authority.decision.recorded event for the given
// requestID, outcome, and approved target scope — directly via the factory, the
// same store-only path the ops-api POST handler uses (appendAuthorityDecisionRecorded).
// It mirrors the H3 newDecisionTestStore/seedPendingDraftPRRequest pattern so the
// gate can be exercised entirely offline (no GitHub, no loop, no HTTP).
func seedDraftPRDecision(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID, requestID types.EventID, outcome string, target DraftPRTarget) {
	t.Helper()
	content := AuthorityDecisionRecordedContent{
		DecisionID:     requestID.Value(),
		RequestID:      requestID,
		ApproverActor:  human,
		DeciderRole:    "human",
		Outcome:        outcome,
		ApprovedTarget: target.Repository + " " + target.HeadRef,
		ApprovedAction: string(safety.ActionRepoPullRequestCreate),
		Scope:          target.Scope(),
		Rationale:      "reviewed civic-roles.md",
	}
	if _, err := appendAuthorityDecisionRecorded(s, factory, signer, human, conv, requestID, content); err != nil {
		t.Fatalf("append authority.decision.recorded (%s): %v", outcome, err)
	}
}

// TestLoadApprovedDraftPRTargetApproved proves the happy path: an approved
// decision yields the authoritative DraftPRTarget from the approved scope, and
// content-hash verification PASSES for the approved title/body.
func TestLoadApprovedDraftPRTargetApproved(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
	want := gateDraftPRTarget()
	seedDraftPRDecision(t, s, factory, signer, human, conv, requestID, "approved", want)

	got, err := LoadApprovedDraftPRTarget(s, requestID.Value())
	if err != nil {
		t.Fatalf("LoadApprovedDraftPRTarget(approved): unexpected error %v", err)
	}
	if got != want {
		t.Fatalf("loaded target mismatch:\n got %+v\nwant %+v", got, want)
	}

	// The authoritative target carries the approved nonce — not any CLI value.
	if got.SingleUseNonce != "nonce-approved-1" {
		t.Fatalf("nonce = %q, want the approved nonce-approved-1", got.SingleUseNonce)
	}

	if err := VerifyDraftPRContent(got, gateApprovedTitle, gateApprovedBody); err != nil {
		t.Fatalf("VerifyDraftPRContent should PASS for approved title/body, got %v", err)
	}
}

// TestLoadApprovedDraftPRTargetDenied proves a denied decision is refused: no
// target, an error mentioning the outcome.
func TestLoadApprovedDraftPRTargetDenied(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
	seedDraftPRDecision(t, s, factory, signer, human, conv, requestID, "denied", gateDraftPRTarget())

	got, err := LoadApprovedDraftPRTarget(s, requestID.Value())
	if err == nil {
		t.Fatalf("LoadApprovedDraftPRTarget(denied) must refuse, got target %+v", got)
	}
	if got != (DraftPRTarget{}) {
		t.Fatalf("denied decision must return the zero target, got %+v", got)
	}
}

// TestLoadApprovedDraftPRTargetNoDecisionPendingOnly proves that a request which
// was raised but never decided (pending only) is refused — there is no recorded
// approval to load.
func TestLoadApprovedDraftPRTargetNoDecisionPendingOnly(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
	// No decision seeded.

	got, err := LoadApprovedDraftPRTarget(s, requestID.Value())
	if err == nil {
		t.Fatalf("LoadApprovedDraftPRTarget(pending-only) must refuse, got target %+v", got)
	}
	if got != (DraftPRTarget{}) {
		t.Fatalf("undecided request must return the zero target, got %+v", got)
	}
}

// TestLoadApprovedDraftPRTargetNoDecisionNothingSeeded proves that an unknown
// request id (nothing recorded at all) is refused.
func TestLoadApprovedDraftPRTargetNoDecisionNothingSeeded(t *testing.T) {
	s, _, _, _, _ := newDecisionTestStore(t)

	got, err := LoadApprovedDraftPRTarget(s, "auth_does_not_exist")
	if err == nil {
		t.Fatalf("LoadApprovedDraftPRTarget(unknown id) must refuse, got target %+v", got)
	}
	if got != (DraftPRTarget{}) {
		t.Fatalf("unknown request must return the zero target, got %+v", got)
	}
}

// TestVerifyDraftPRContentMismatchRefuses proves the content-hash gate: an
// approved decision loads fine, but a DIFFERENT title or body is refused because
// it does not hash-match what the human approved.
func TestVerifyDraftPRContentMismatchRefuses(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
	seedDraftPRDecision(t, s, factory, signer, human, conv, requestID, "approved", gateDraftPRTarget())

	target, err := LoadApprovedDraftPRTarget(s, requestID.Value())
	if err != nil {
		t.Fatalf("LoadApprovedDraftPRTarget(approved): %v", err)
	}

	t.Run("title mismatch", func(t *testing.T) {
		if err := VerifyDraftPRContent(target, "DIFFERENT title the human never approved", gateApprovedBody); err == nil {
			t.Fatalf("VerifyDraftPRContent must refuse a title that differs from the approved one")
		}
	})
	t.Run("body mismatch", func(t *testing.T) {
		if err := VerifyDraftPRContent(target, gateApprovedTitle, "## Summary\nDIFFERENT body.\n"); err == nil {
			t.Fatalf("VerifyDraftPRContent must refuse a body that differs from the approved one")
		}
	})
	t.Run("both match", func(t *testing.T) {
		if err := VerifyDraftPRContent(target, gateApprovedTitle, gateApprovedBody); err != nil {
			t.Fatalf("VerifyDraftPRContent must PASS when both title and body match: %v", err)
		}
	})
}
