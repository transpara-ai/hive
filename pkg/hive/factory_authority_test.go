package hive

import (
	"errors"
	"testing"

	"github.com/transpara-ai/hive/pkg/safety"
)

func TestRaiseDraftPRAuthorityRequestHoldsAndRecords(t *testing.T) {
	// newIdentityTestRuntime: approveRequests is false by default (the gate holds).
	r := newIdentityTestRuntime(t)
	target := DraftPRTarget{
		Repository: "transpara-ai/docs", BaseRef: "main", BaseSHA: "basesha",
		HeadRef: "codex/civic-roles", HeadSHA: "headsha",
		TitleHash: "sha256:aaa", BodyHash: "sha256:bbb",
		PolicyBundleID: "df-v3.9.20-docs-draft-pr-create-only", PolicyBundleHash: "sha256:ccc",
		SingleUseNonce: "nonce-1",
	}

	// reuse the only pre-registered actor as the requesting actor; identity is immaterial to this test.
	requestID, err := r.RaiseDraftPRAuthorityRequest(target, r.humanID, "Draft PR for civic-roles.md")
	var authErr safety.AuthorityError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthorityError (gate holds), got %v", err)
	}
	if authErr.Outcome != safety.ApprovalRequired {
		t.Fatalf("outcome = %q, want ApprovalRequired", authErr.Outcome)
	}
	if requestID.IsZero() {
		t.Fatal("expected a recorded request id")
	}
	proj := BuildOperatorProjection(r.store, 50)
	if len(proj.PendingApprovals) != 1 || proj.PendingApprovals[0].ActionName != "pull_request.create" {
		t.Fatalf("expected one pending pull_request.create approval, got %+v", proj.PendingApprovals)
	}
	got, perr := ParseDraftPRScope(proj.PendingApprovals[0].Scope)
	if perr != nil || got != target {
		t.Fatalf("scope round-trip failed: %v / %+v", perr, got)
	}
}

func TestParseDraftPRScopeRejectsInvalid(t *testing.T) {
	// wrong discriminator
	if _, err := ParseDraftPRScope([]string{"wrong.action", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}); err == nil {
		t.Fatal("expected error for wrong discriminator")
	}
	// wrong length (too short)
	if _, err := ParseDraftPRScope([]string{"pull_request.create"}); err == nil {
		t.Fatal("expected error for short scope")
	}
}
