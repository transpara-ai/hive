package hive

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

// MarkReadyTarget carries the immutable target details for a recorded human
// approval of the managed draft→ready transition (hive#263). A draft-PR
// creation approval never authorizes readying; this is its own protected
// action with its own recorded decision. ReDraftOnFailure is the explicit,
// recorded permission for the finalizer to return the PR to draft when
// ready-state review fails after the mutation — never an ambient default.
type MarkReadyTarget struct {
	Repository       string
	PRNumber         int
	PRURL            string
	HeadSHA          string
	ReDraftOnFailure bool
	SingleUseNonce   string
}

// markReadyScopeLen is the fixed Scope() encoding length, discriminator first.
const markReadyScopeLen = 7

// Scope encodes the target in fixed order, discriminator first, mirroring
// DraftPRTarget.Scope() so operator projections read both the same way.
func (t MarkReadyTarget) Scope() []string {
	return []string{
		string(safety.ActionRepoPullRequestMarkReady),
		t.Repository,
		strconv.Itoa(t.PRNumber),
		t.PRURL,
		t.HeadSHA,
		strconv.FormatBool(t.ReDraftOnFailure),
		t.SingleUseNonce,
	}
}

// ParseMarkReadyScope reconstructs a MarkReadyTarget from a recorded decision
// Scope slice. Every field is required; anything malformed is refused.
func ParseMarkReadyScope(scope []string) (MarkReadyTarget, error) {
	if len(scope) != markReadyScopeLen || scope[0] != string(safety.ActionRepoPullRequestMarkReady) {
		return MarkReadyTarget{}, fmt.Errorf("not a mark-ready scope: %v", scope)
	}
	prNumber, err := strconv.Atoi(scope[2])
	if err != nil || prNumber <= 0 {
		return MarkReadyTarget{}, fmt.Errorf("mark-ready scope pr number %q is invalid", scope[2])
	}
	reDraft, err := strconv.ParseBool(scope[5])
	if err != nil {
		return MarkReadyTarget{}, fmt.Errorf("mark-ready scope re-draft flag %q is invalid", scope[5])
	}
	target := MarkReadyTarget{
		Repository:       strings.TrimSpace(scope[1]),
		PRNumber:         prNumber,
		PRURL:            strings.TrimSpace(scope[3]),
		HeadSHA:          strings.TrimSpace(scope[4]),
		ReDraftOnFailure: reDraft,
		SingleUseNonce:   strings.TrimSpace(scope[6]),
	}
	if target.Repository == "" || target.PRURL == "" || target.HeadSHA == "" || target.SingleUseNonce == "" {
		return MarkReadyTarget{}, fmt.Errorf("mark-ready scope has empty required fields: %v", scope)
	}
	return target, nil
}

// FindApprovedMarkReadyTarget is the GOVERNANCE gate lookup for the managed
// draft→ready transition. It scans recorded authority decisions (newest
// first, latest-wins like findAuthorityDecisionByRequestID) for an APPROVED,
// HUMAN-decided pull_request.mark_ready decision whose scope exactly matches
// the run-derived target: repository (case-insensitive), PR number, and head
// SHA (case-insensitive, matching the runtime's EqualFold semantics). It
// refuses — no target — when no such decision exists, when the newest
// matching human decision is not approved, or when the store is unreadable.
// Non-human decisions are skipped entirely: they can neither authorize nor
// shadow a human decision. It never writes.
func FindApprovedMarkReadyTarget(s store.Store, repository string, prNumber int, headSHA string) (MarkReadyTarget, error) {
	repository = strings.ToLower(strings.TrimSpace(repository))
	headSHA = strings.TrimSpace(headSHA)
	if repository == "" || prNumber <= 0 || headSHA == "" {
		return MarkReadyTarget{}, fmt.Errorf("mark-ready lookup requires repository, pr number, and head sha")
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(EventTypeAuthorityDecisionRecorded, defaultOperatorProjectionLimit, cursor)
		if err != nil {
			return MarkReadyTarget{}, fmt.Errorf("scan authority decisions for mark-ready approval: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(AuthorityDecisionRecordedContent)
			if !ok || content.ApprovedAction != string(safety.ActionRepoPullRequestMarkReady) {
				continue
			}
			// Only HUMAN decisions carry mark-ready authority, in either
			// direction — mirroring the draft-PR authority path. A non-human
			// record can neither authorize nor shadow a human decision.
			if !strings.EqualFold(strings.TrimSpace(content.DeciderRole), "human") {
				continue
			}
			target, err := ParseMarkReadyScope(content.Scope)
			if err != nil {
				continue // a malformed mark-ready decision can never authorize anything
			}
			if strings.ToLower(target.Repository) != repository || target.PRNumber != prNumber || !strings.EqualFold(target.HeadSHA, headSHA) {
				continue
			}
			// Newest-first scan: the first matching decision is latest-wins.
			if content.Outcome != draftPRApprovedOutcome {
				return MarkReadyTarget{}, fmt.Errorf("latest mark-ready decision for %s#%d@%s has outcome %q, not %q: refusing to mark ready", repository, prNumber, headSHA, content.Outcome, draftPRApprovedOutcome)
			}
			return target, nil
		}
		if !page.HasMore() {
			return MarkReadyTarget{}, fmt.Errorf("no approved mark-ready decision recorded for %s#%d@%s: refusing to mark ready (a draft-PR creation approval does not authorize readying)", repository, prNumber, headSHA)
		}
		cursor = page.Cursor()
	}
}

// NewStoreMarkReadyApprovalLookup adapts FindApprovedMarkReadyTarget to the
// finalizer's per-run lookup shape, deriving the target from the run's
// mutation. Fail-closed by construction: any lookup error refuses the ready
// transition before the PR is touched.
func NewStoreMarkReadyApprovalLookup(s store.Store) MarkReadyApprovalLookup {
	return func(_ context.Context, mutation IssueScanReadyPRFinalizerMutation) (MarkReadyTarget, error) {
		if s == nil {
			return MarkReadyTarget{}, fmt.Errorf("mark-ready approval lookup requires a store")
		}
		return FindApprovedMarkReadyTarget(s, mutation.Repository, mutation.PRNumber, mutation.HeadSHA)
	}
}
