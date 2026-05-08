package hive

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

type protectedActionRequest struct {
	Action            safety.ProtectedAction
	RequestingActor   types.ActorID
	Target            string
	Environment       string
	RequestedOutcome  string
	Justification     string
	RiskSummary       string
	EvidenceReviewed  []types.EventID
	ProposedOperation string
	CausalEventIDs    []types.EventID
}

func (r *Runtime) authorizeProtectedAction(req protectedActionRequest) (types.EventID, error) {
	outcome := safety.DefaultOutcome(req.Action)
	if outcome == safety.Forbidden {
		return types.EventID{}, safety.AuthorityError{Action: req.Action, Outcome: outcome}
	}
	if outcome == safety.Autonomous || outcome == safety.Notify {
		return types.EventID{}, nil
	}

	requestID, err := r.recordAuthorityRequest(req)
	if err != nil {
		return types.EventID{}, fmt.Errorf("record authority request for %s: %w", req.Action, err)
	}

	if !r.approveRequests {
		return requestID, safety.AuthorityError{Action: req.Action, Outcome: outcome}
	}
	if !safety.ApprovalAllowsAction(req.Action, req.Action) {
		return requestID, safety.AuthorityError{Action: req.Action, Outcome: safety.Forbidden}
	}
	if err := r.recordAuthorityDecision(requestID, req); err != nil {
		return requestID, fmt.Errorf("record authority decision for %s: %w", req.Action, err)
	}
	return requestID, nil
}

func (r *Runtime) recordAuthorityRequest(req protectedActionRequest) (types.EventID, error) {
	actorID := req.RequestingActor
	if actorID.IsZero() {
		actorID = r.humanID
	}

	causes, err := r.authorityCauses(req.CausalEventIDs)
	if err != nil {
		return types.EventID{}, err
	}

	content := event.AuthorityRequestContent{
		Action:        string(req.Action),
		Actor:         actorID,
		Level:         event.AuthorityLevelRequired,
		Justification: req.Justification,
		Causes:        types.MustNonEmpty(causes),
	}
	ev, err := r.factory.Create(event.EventTypeAuthorityRequested, r.humanID, content, causes, r.convID, r.store, r.signer)
	if err != nil {
		return types.EventID{}, err
	}
	stored, err := r.store.Append(ev)
	if err != nil {
		return types.EventID{}, err
	}

	detailCauses := []types.EventID{stored.ID()}
	detail := AuthorityRequestRecordedContent{
		RequestID:         stored.ID(),
		RequestingActor:   actorID,
		ActionName:        string(req.Action),
		Target:            req.Target,
		Environment:       req.Environment,
		RequestedOutcome:  req.RequestedOutcome,
		Justification:     req.Justification,
		RiskSummary:       req.RiskSummary,
		EvidenceReviewed:  req.EvidenceReviewed,
		ProposedOperation: req.ProposedOperation,
		CausalEventIDs:    causes,
	}
	if _, err := r.graph.Record(EventTypeAuthorityRequestRecorded, r.humanID, detail, detailCauses, r.convID, r.signer); err != nil {
		return types.EventID{}, err
	}
	return stored.ID(), nil
}

func (r *Runtime) recordAuthorityDecision(requestID types.EventID, req protectedActionRequest) error {
	content := AuthorityDecisionRecordedContent{
		DecisionID:       requestID.Value(),
		RequestID:        requestID,
		ApproverActor:    r.humanID,
		Outcome:          "approved",
		ApprovedTarget:   req.Target,
		ApprovedAction:   string(req.Action),
		EvidenceReviewed: req.EvidenceReviewed,
		Rationale:        "auto-approved via --approve-requests",
	}
	_, err := r.graph.Record(EventTypeAuthorityDecisionRecorded, r.humanID, content, []types.EventID{requestID}, r.convID, r.signer)
	return err
}

func (r *Runtime) authorityCauses(causalEventIDs []types.EventID) ([]types.EventID, error) {
	if len(causalEventIDs) > 0 {
		causes := make([]types.EventID, len(causalEventIDs))
		copy(causes, causalEventIDs)
		return causes, nil
	}
	head, err := r.store.Head()
	if err != nil {
		return nil, err
	}
	if !head.IsSome() {
		return nil, fmt.Errorf("no chain head")
	}
	return []types.EventID{head.Unwrap().ID()}, nil
}

func (r *Runtime) hasAuthorityRequest(action safety.ProtectedAction, target string) bool {
	page, err := r.store.ByType(EventTypeAuthorityRequestRecorded, 1000, types.None[types.Cursor]())
	if err != nil {
		return false
	}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(AuthorityRequestRecordedContent)
		if ok && content.ActionName == string(action) && content.Target == target {
			return true
		}
	}
	return false
}
