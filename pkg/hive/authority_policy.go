package hive

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

type protectedActionRequest struct {
	Action            safety.ProtectedAction
	RequestingActor   types.ActorID
	RequestingRole    string
	Target            string
	Environment       string
	RiskClass         string
	RequestedOutcome  string
	Justification     string
	RiskSummary       string
	Scope             []string
	EvidenceReviewed  []types.EventID
	ProposedOperation string
	CausalEventIDs    []types.EventID
}

const (
	protectedActionEmulationModeSideEffectFreeLocal = "side_effect_free_local_emulation"
	policyCanonicalApprovalRequired                 = "approval_required"
	policyCanonicalForbidden                        = "forbidden"
)

type protectedActionLocalEmulationRequest struct {
	Action                   safety.ProtectedAction
	RequestingActor          types.ActorID
	RequestingRole           string
	Target                   string
	Environment              string
	Justification            string
	RiskSummary              string
	Mode                     string
	TargetStateBefore        string
	TargetStateAfter         string
	ProducedResourceIDs      []string
	CausalEventIDs           []types.EventID
	PolicyDecision           PolicyEngineAdapterDecisionContent
	ExpectedPolicyBundleHash string
}

type protectedActionLocalEmulationResult struct {
	RequestID                  types.EventID
	DecisionEventID            types.EventID
	PolicyAdapterDecisionID    types.EventID
	ExecutionReceiptID         types.EventID
	Blocker                    string
	RealSideEffectExecuted     bool
	RepositoryMutationExecuted bool
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
	if _, err := r.recordAuthorityDecision(requestID, req); err != nil {
		return requestID, fmt.Errorf("record authority decision for %s: %w", req.Action, err)
	}
	return requestID, nil
}

func (r *Runtime) recordProtectedActionLocalEmulation(req protectedActionLocalEmulationRequest) (protectedActionLocalEmulationResult, error) {
	result := protectedActionLocalEmulationResult{}
	if r == nil {
		return blockedProtectedActionLocalEmulation(result, "missing runtime dependency")
	}
	if r.store == nil {
		return blockedProtectedActionLocalEmulation(result, "missing audit store dependency")
	}
	if r.graph == nil || r.factory == nil || r.signer == nil {
		return blockedProtectedActionLocalEmulation(result, "missing graph dependency")
	}
	if req.Mode != protectedActionEmulationModeSideEffectFreeLocal {
		return blockedProtectedActionLocalEmulation(result, "real protected side effects are forbidden for mode %q", req.Mode)
	}
	if !isResidualClosureEmulationAction(req.Action) {
		return blockedProtectedActionLocalEmulation(result, "action %q is outside the authorized local emulation seam", req.Action)
	}
	if err := validatePolicyAdapterEvidence(req.Action, req.PolicyDecision, req.ExpectedPolicyBundleHash); err != nil {
		return blockedProtectedActionLocalEmulation(result, "%s", err.Error())
	}

	actorID := req.RequestingActor
	if actorID.IsZero() {
		actorID = r.humanID
	}
	authorityReq := protectedActionRequest{
		Action:            req.Action,
		RequestingActor:   actorID,
		RequestingRole:    req.RequestingRole,
		Target:            req.Target,
		Environment:       req.Environment,
		RiskClass:         safety.RiskClass(req.Action),
		RequestedOutcome:  protectedActionEmulationModeSideEffectFreeLocal,
		Justification:     req.Justification,
		RiskSummary:       req.RiskSummary,
		Scope:             []string{string(req.Action), protectedActionEmulationModeSideEffectFreeLocal},
		ProposedOperation: fmt.Sprintf("%s:%s", protectedActionEmulationModeSideEffectFreeLocal, req.Action),
		CausalEventIDs:    req.CausalEventIDs,
	}
	requestID, err := r.recordAuthorityRequest(authorityReq)
	if err != nil {
		return blockedProtectedActionLocalEmulation(result, "record authority request for %s: %v", req.Action, err)
	}
	result.RequestID = requestID

	if req.PolicyDecision.ProtectedActionType != string(req.Action) {
		return blockedProtectedActionLocalEmulation(result, "cross-action approval blocked: policy action %q does not match requested action %q", req.PolicyDecision.ProtectedActionType, req.Action)
	}
	if req.PolicyDecision.CanonicalDecision == policyCanonicalForbidden {
		policyID, err := r.recordPolicyEngineAdapterDecision(policyDecisionWithEvidence(req.PolicyDecision, requestID, types.EventID{}), []types.EventID{requestID})
		if err != nil {
			return blockedProtectedActionLocalEmulation(result, "record forbidden policy adapter decision: %v", err)
		}
		result.PolicyAdapterDecisionID = policyID
		return blockedProtectedActionLocalEmulation(result, "policy canonical decision forbidden for %s", req.Action)
	}
	if req.PolicyDecision.CanonicalDecision != policyCanonicalApprovalRequired {
		return blockedProtectedActionLocalEmulation(result, "policy canonical decision %q cannot authorize protected local emulation", req.PolicyDecision.CanonicalDecision)
	}
	if !r.approveRequests {
		return blockedProtectedActionLocalEmulation(result, "missing authority decision for approved local emulation")
	}

	decisionID, err := r.recordAuthorityDecision(requestID, authorityReq)
	if err != nil {
		return blockedProtectedActionLocalEmulation(result, "record authority decision for %s: %v", req.Action, err)
	}
	result.DecisionEventID = decisionID

	policyID, err := r.recordPolicyEngineAdapterDecision(policyDecisionWithEvidence(req.PolicyDecision, requestID, decisionID), []types.EventID{requestID, decisionID})
	if err != nil {
		return blockedProtectedActionLocalEmulation(result, "record policy adapter decision for %s: %v", req.Action, err)
	}
	result.PolicyAdapterDecisionID = policyID

	receiptID, err := r.recordAuthorityExecutionReceipt(req, actorID, requestID, decisionID, policyID)
	if err != nil {
		return blockedProtectedActionLocalEmulation(result, "record execution receipt for %s: %v", req.Action, err)
	}
	result.ExecutionReceiptID = receiptID
	return result, nil
}

func (r *Runtime) recordAuthorityRequest(req protectedActionRequest) (types.EventID, error) {
	actorID := req.RequestingActor
	if actorID.IsZero() {
		actorID = r.humanID
	}
	riskClass := req.RiskClass
	if riskClass == "" {
		riskClass = safety.RiskClass(req.Action)
	}
	scope := authorityScope(req)

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
		RequestingRole:    req.RequestingRole,
		ActionName:        string(req.Action),
		Target:            req.Target,
		Environment:       req.Environment,
		RiskClass:         riskClass,
		RequestedOutcome:  req.RequestedOutcome,
		Justification:     req.Justification,
		RiskSummary:       req.RiskSummary,
		Scope:             scope,
		EvidenceReviewed:  req.EvidenceReviewed,
		ProposedOperation: req.ProposedOperation,
		CausalEventIDs:    causes,
	}
	if _, err := r.graph.Record(EventTypeAuthorityRequestRecorded, r.humanID, detail, detailCauses, r.convID, r.signer); err != nil {
		return types.EventID{}, err
	}
	return stored.ID(), nil
}

func (r *Runtime) recordAuthorityDecision(requestID types.EventID, req protectedActionRequest) (types.EventID, error) {
	scope := authorityScope(req)
	content := AuthorityDecisionRecordedContent{
		DecisionID:       requestID.Value(),
		RequestID:        requestID,
		ApproverActor:    r.humanID,
		DeciderRole:      "operator",
		Outcome:          "approved",
		ApprovedTarget:   req.Target,
		ApprovedAction:   string(req.Action),
		Scope:            scope,
		EvidenceReviewed: req.EvidenceReviewed,
		Rationale:        "auto-approved via --approve-requests",
	}
	// Use r.graph.Record (not the store-only helper) so the event is published to
	// the in-process bus. Live subscribers — the telemetry writer and the agent
	// loop — depend on receiving authority.decision.recorded via the bus.
	// The store-only helper (appendAuthorityDecisionRecorded) exists only for the
	// busless ops-api process, which has no bus to publish to.
	ev, err := r.graph.Record(EventTypeAuthorityDecisionRecorded, r.humanID, content, []types.EventID{requestID}, r.convID, r.signer)
	if err != nil {
		return types.EventID{}, err
	}
	return ev.ID(), nil
}

// appendAuthorityDecisionRecorded constructs and persists an
// authority.decision.recorded event without requiring a full *Runtime. It is
// used exclusively by the store-only ops-api POST handler, which runs in a
// separate process that has no in-process event bus.
//
// The Runtime auto-approval path (Runtime.recordAuthorityDecision) uses
// r.graph.Record directly so that the event is published to the bus. Live
// subscribers — the telemetry writer and the agent loop — depend on that
// publish. Do NOT route the runtime path through this helper.
//
// The decision causes the request, and content.RequestID is what
// BuildOperatorProjection matches to drop the request out of PendingApprovals.
func appendAuthorityDecisionRecorded(s store.Store, factory *event.EventFactory, signer event.Signer, humanID types.ActorID, convID types.ConversationID, requestID types.EventID, content AuthorityDecisionRecordedContent) (types.EventID, error) {
	ev, err := factory.Create(EventTypeAuthorityDecisionRecorded, humanID, content, []types.EventID{requestID}, convID, s, signer)
	if err != nil {
		return types.EventID{}, err
	}
	stored, err := s.Append(ev)
	if err != nil {
		return types.EventID{}, err
	}
	return stored.ID(), nil
}

func authorityScope(req protectedActionRequest) []string {
	if len(req.Scope) > 0 {
		scope := make([]string, len(req.Scope))
		copy(scope, req.Scope)
		return scope
	}
	return []string{string(req.Action)}
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
	events, err := eventsByTypePaginated(r.store, EventTypeAuthorityRequestRecorded, defaultOperatorProjectionLimit)
	if err != nil {
		return false
	}
	for _, ev := range events {
		content, ok := ev.Content().(AuthorityRequestRecordedContent)
		if ok && content.ActionName == string(action) && content.Target == target {
			return true
		}
	}
	return false
}

func blockedProtectedActionLocalEmulation(result protectedActionLocalEmulationResult, format string, args ...any) (protectedActionLocalEmulationResult, error) {
	result.Blocker = fmt.Sprintf(format, args...)
	return result, fmt.Errorf("protected action local emulation blocked: %s", result.Blocker)
}

func isResidualClosureEmulationAction(action safety.ProtectedAction) bool {
	return action == safety.ActionRepoMergeMain || action == safety.ActionRepoPushDefaultBranch
}

func validatePolicyAdapterEvidence(action safety.ProtectedAction, content PolicyEngineAdapterDecisionContent, expectedHash string) error {
	if content.DecisionID == "" ||
		content.AdapterID == "" ||
		content.AdapterVersion == "" ||
		content.ProtectedActionType == "" ||
		content.ActorID == "" ||
		content.RawDecision == "" ||
		content.CanonicalDecision == "" ||
		len(content.InputFacts) == 0 ||
		len(content.ResourceRefs) == 0 ||
		len(content.ReasonCodes) == 0 {
		return fmt.Errorf("missing policy adapter decision evidence for %s", action)
	}
	if content.PolicyBundleID == "" || content.PolicyBundleHash == "" || expectedHash == "" {
		return fmt.Errorf("missing policy bundle evidence for %s", action)
	}
	if content.PolicyBundleHash != expectedHash {
		return fmt.Errorf("stale or mismatched policy_bundle_hash for %s: got %q want %q", action, content.PolicyBundleHash, expectedHash)
	}
	if content.LatencyMS < 0 {
		return fmt.Errorf("policy adapter latency_ms must be >= 0 for %s", action)
	}
	switch content.CanonicalDecision {
	case policyCanonicalApprovalRequired, policyCanonicalForbidden:
		return nil
	default:
		return fmt.Errorf("policy canonical decision %q is not valid for protected local emulation", content.CanonicalDecision)
	}
}

func policyDecisionWithEvidence(content PolicyEngineAdapterDecisionContent, requestID, decisionID types.EventID) PolicyEngineAdapterDecisionContent {
	out := content
	out.EvidenceRefs = appendEvidenceRefs(out.EvidenceRefs, requestID, decisionID)
	if decisionID.IsZero() {
		out.AuthorityDecisionRef = nil
	} else {
		out.AuthorityDecisionRef = &decisionID
	}
	return out
}

func (r *Runtime) recordPolicyEngineAdapterDecision(content PolicyEngineAdapterDecisionContent, causes []types.EventID) (types.EventID, error) {
	ev, err := r.graph.Record(EventTypePolicyEngineAdapterDecision, r.humanID, content, causes, r.convID, r.signer)
	if err != nil {
		return types.EventID{}, err
	}
	return ev.ID(), nil
}

func (r *Runtime) recordAuthorityExecutionReceipt(req protectedActionLocalEmulationRequest, actorID types.ActorID, requestID, decisionID, policyID types.EventID) (types.EventID, error) {
	before := req.TargetStateBefore
	if before == "" {
		before = "protected action not emulated"
	}
	after := req.TargetStateAfter
	if after == "" {
		after = "side-effect-free local emulation recorded"
	}
	produced := append([]string{}, req.ProducedResourceIDs...)
	produced = append(produced, "policy.adapter.decision:"+policyID.Value())
	content := AuthorityExecutionReceiptContent{
		RequestID:           requestID,
		DecisionEventID:     decisionID,
		ExecutingActor:      actorID,
		Operation:           fmt.Sprintf("%s:%s", protectedActionEmulationModeSideEffectFreeLocal, req.Action),
		TargetStateBefore:   before,
		TargetStateAfter:    after,
		ResultStatus:        "succeeded",
		ProducedResourceIDs: produced,
		CausalEventIDs:      appendEventIDs(req.CausalEventIDs, requestID, decisionID, policyID),
	}
	ev, err := r.graph.Record(EventTypeAuthorityExecutionReceipt, r.humanID, content, []types.EventID{requestID, decisionID, policyID}, r.convID, r.signer)
	if err != nil {
		return types.EventID{}, err
	}
	return ev.ID(), nil
}

func appendEvidenceRefs(existing []string, ids ...types.EventID) []string {
	out := append([]string{}, existing...)
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		out = append(out, id.Value())
	}
	return out
}

func appendEventIDs(existing []types.EventID, ids ...types.EventID) []types.EventID {
	out := append([]types.EventID{}, existing...)
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		out = append(out, id)
	}
	return out
}
