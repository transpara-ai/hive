package hive

import (
	"fmt"
	"strings"

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
