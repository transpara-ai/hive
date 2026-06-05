package hive

import (
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

// DraftPRTarget carries the immutable target details for a guardian-raised
// draft pull request authority request. All fields are required. The Scope()
// encoding is the canonical wire format Epic 11 evidence consumes.
type DraftPRTarget struct {
	Repository       string
	BaseRef          string
	BaseSHA          string
	HeadRef          string
	HeadSHA          string
	TitleHash        string
	BodyHash         string
	PolicyBundleID   string
	PolicyBundleHash string
	SingleUseNonce   string
}

// Scope encodes the target in the fixed order Epic 11 evidence consumes.
// The first element is always "pull_request.create" so ParseDraftPRScope can
// verify the discriminator without additional context.
func (t DraftPRTarget) Scope() []string {
	return []string{
		string(safety.ActionRepoPullRequestCreate),
		t.Repository,
		t.BaseRef,
		t.BaseSHA,
		t.HeadRef,
		t.HeadSHA,
		t.TitleHash,
		t.BodyHash,
		t.PolicyBundleID,
		t.PolicyBundleHash,
		t.SingleUseNonce,
	}
}

// ParseDraftPRScope reconstructs a DraftPRTarget from an operator projection
// Scope slice. Returns an error if the slice is not a valid draft-pr scope.
func ParseDraftPRScope(scope []string) (DraftPRTarget, error) {
	if len(scope) != 11 || scope[0] != string(safety.ActionRepoPullRequestCreate) {
		return DraftPRTarget{}, fmt.Errorf("not a draft-pr scope: %v", scope)
	}
	return DraftPRTarget{
		Repository:       scope[1],
		BaseRef:          scope[2],
		BaseSHA:          scope[3],
		HeadRef:          scope[4],
		HeadSHA:          scope[5],
		TitleHash:        scope[6],
		BodyHash:         scope[7],
		PolicyBundleID:   scope[8],
		PolicyBundleHash: scope[9],
		SingleUseNonce:   scope[10],
	}, nil
}

// draftPRApprovedOutcome is the only authority decision outcome that authorizes
// a real draft-PR creation. It matches the vocabulary recorded by both the
// ops-api POST handler and the runtime auto-approval path ("approved").
const draftPRApprovedOutcome = "approved"

// findAuthorityDecisionByRequestID scans authority.decision.recorded events for
// the one whose content RequestID matches requestID. It mirrors
// findAuthorityRequestByID (the request-side reader) so the decision read path
// uses the same RequestID key that BuildOperatorProjection joins on. found is
// false when no decision was ever recorded for that request id. ByType scans
// newest-first, so if multiple decisions were ever recorded for the same request
// id, the MOST RECENT (latest-wins) decision is returned.
func findAuthorityDecisionByRequestID(s store.Store, requestID string, limit int) (AuthorityDecisionRecordedContent, bool, error) {
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(EventTypeAuthorityDecisionRecorded, limit, cursor)
		if err != nil {
			return AuthorityDecisionRecordedContent{}, false, err
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(AuthorityDecisionRecordedContent)
			if ok && content.RequestID.Value() == requestID {
				return content, true, nil
			}
		}
		if !page.HasMore() {
			return AuthorityDecisionRecordedContent{}, false, nil
		}
		cursor = page.Cursor()
	}
}

// LoadApprovedDraftPRTarget is the GOVERNANCE gate for create-pr. It loads the
// RECORDED authority decision for requestID from the store and returns the
// AUTHORITATIVE draft-PR target encoded in that decision's approved Scope.
//
// It refuses (returns an error, no target) unless ALL hold:
//  1. a decision was recorded for requestID (a request that was never decided —
//     only pending, or absent entirely — has no decision and is refused);
//  2. the decision Outcome is "approved" (a "denied" or any non-approved
//     outcome is refused);
//  3. the approved Scope decodes to a valid draft-pr DraftPRTarget.
//
// The returned target's repo/baseRef/baseSHA/headRef/headSHA/titleHash/bodyHash/
// policyBundleID/policyBundleHash/nonce all come from what the human approved —
// never from fresh CLI input. Callers MUST then VerifyDraftPRContent the
// supplied title/body against the returned TitleHash/BodyHash before creating
// any PR. This function performs only read access; it never writes the graph.
func LoadApprovedDraftPRTarget(s store.Store, requestID string) (DraftPRTarget, error) {
	if requestID == "" {
		return DraftPRTarget{}, fmt.Errorf("request id is required to load an approved draft-PR decision")
	}
	decision, found, err := findAuthorityDecisionByRequestID(s, requestID, defaultOperatorProjectionLimit)
	if err != nil {
		return DraftPRTarget{}, fmt.Errorf("load authority decision for request %s: %w", requestID, err)
	}
	if !found {
		return DraftPRTarget{}, fmt.Errorf("no authority decision recorded for request %s: refusing to create a PR for an undecided request", requestID)
	}
	if decision.Outcome != draftPRApprovedOutcome {
		return DraftPRTarget{}, fmt.Errorf("authority decision for request %s has outcome %q, not %q: refusing to create a PR", requestID, decision.Outcome, draftPRApprovedOutcome)
	}
	target, err := ParseDraftPRScope(decision.Scope)
	if err != nil {
		return DraftPRTarget{}, fmt.Errorf("approved decision for request %s does not carry a valid draft-pr scope: %w", requestID, err)
	}
	return target, nil
}

// VerifyDraftPRContent confirms the supplied title/body hash to exactly what the
// human approved, per the loaded target's TitleHash/BodyHash. It uses the same
// "sha256:"+hex(sha256(x)) digest the request scope was built with. A mismatch
// means the operator is trying to create a PR whose title or body differs from
// what was approved; this returns an error so the caller refuses to proceed.
func VerifyDraftPRContent(target DraftPRTarget, title, body string) error {
	if got := sha256HexPrefixed([]byte(title)); got != target.TitleHash {
		return fmt.Errorf("title does not match the approved decision: got %s, approved %s", got, target.TitleHash)
	}
	if got := sha256HexPrefixed([]byte(body)); got != target.BodyHash {
		return fmt.Errorf("body does not match the approved decision: got %s, approved %s", got, target.BodyHash)
	}
	return nil
}

// RaiseDraftPRAuthorityRequest raises the required authority request for a
// guardian-initiated draft PR against transpara-ai/docs. With --approve-requests
// OFF (the default), authorizeProtectedAction records the request and returns
// safety.AuthorityError — the gate HOLDS and the returned requestID is non-zero.
// The pending request surfaces via BuildOperatorProjection(...).PendingApprovals
// with the DraftPRTarget encoded in Scope for Site to read.
func (r *Runtime) RaiseDraftPRAuthorityRequest(target DraftPRTarget, guardian types.ActorID, justification string) (types.EventID, error) {
	if !strings.EqualFold(target.Repository, "transpara-ai/docs") {
		return types.EventID{}, fmt.Errorf("draft-pr target repo must be transpara-ai/docs, got %q", target.Repository)
	}
	return r.authorizeProtectedAction(protectedActionRequest{
		Action:            safety.ActionRepoPullRequestCreate,
		RequestingActor:   guardian,
		RequestingRole:    "guardian",
		Target:            target.Repository + " " + target.HeadRef,
		Environment:       string(AgentIdentityEnvironmentProduction),
		RequestedOutcome:  "create draft PR",
		Justification:     justification,
		RiskSummary:       "creates one reversible draft PR; no branch push, merge, or deploy",
		Scope:             target.Scope(),
		ProposedOperation: "createDraftPR",
	})
}
