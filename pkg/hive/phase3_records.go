package hive

import (
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

var (
	EventTypeAgentIdentityRequested            = types.MustEventType("agent.identity.requested")
	EventTypeAgentIdentityGenerated            = types.MustEventType("agent.identity.generated")
	EventTypeAgentIdentityRejected             = types.MustEventType("agent.identity.rejected")
	EventTypeAgentIdentityRevoked              = types.MustEventType("agent.identity.revoked")
	EventTypeAgentIdentityRetired              = types.MustEventType("agent.identity.retired")
	EventTypeAgentLifecycleTransitionRequested = types.MustEventType("agent.lifecycle.transition.requested")
	EventTypeAgentLifecycleTransitioned        = types.MustEventType("agent.lifecycle.transitioned")
	EventTypeAgentLifecycleTransitionDenied    = types.MustEventType("agent.lifecycle.transition.denied")
	EventTypeAgentAuthorityScopeAssigned       = types.MustEventType("agent.authority.scope.assigned")
	EventTypeAgentAuthorityScopeReduced        = types.MustEventType("agent.authority.scope.reduced")
	EventTypeAgentAuthorityEscalationRequested = types.MustEventType("agent.authority.escalation.requested")
	EventTypeAgentAuthorityEscalationDenied    = types.MustEventType("agent.authority.escalation.denied")
	EventTypeAgentAuthorityEscalationApproved  = types.MustEventType("agent.authority.escalation.approved")
	EventTypeAgentKeyGenerated                 = types.MustEventType("agent.key.generated")
	EventTypeAgentKeyRegistered                = types.MustEventType("agent.key.registered")
	EventTypeAgentKeyRotated                   = types.MustEventType("agent.key.rotated")
	EventTypeAgentKeyRevoked                   = types.MustEventType("agent.key.revoked")
	EventTypeAuthorityRequestRecorded          = types.MustEventType("authority.request.recorded")
	EventTypeAuthorityDecisionRecorded         = types.MustEventType("authority.decision.recorded")
	EventTypeAuthorityExecutionReceipt         = types.MustEventType("authority.execution.receipt")
	EventTypeAgentAuditLinked                  = types.MustEventType("agent.audit.linked")
)

func phase3EventTypes() []types.EventType {
	return []types.EventType{
		EventTypeAgentIdentityRequested,
		EventTypeAgentIdentityGenerated,
		EventTypeAgentIdentityRejected,
		EventTypeAgentIdentityRevoked,
		EventTypeAgentIdentityRetired,
		EventTypeAgentLifecycleTransitionRequested,
		EventTypeAgentLifecycleTransitioned,
		EventTypeAgentLifecycleTransitionDenied,
		EventTypeAgentAuthorityScopeAssigned,
		EventTypeAgentAuthorityScopeReduced,
		EventTypeAgentAuthorityEscalationRequested,
		EventTypeAgentAuthorityEscalationDenied,
		EventTypeAgentAuthorityEscalationApproved,
		EventTypeAgentKeyGenerated,
		EventTypeAgentKeyRegistered,
		EventTypeAgentKeyRotated,
		EventTypeAgentKeyRevoked,
		EventTypeAuthorityRequestRecorded,
		EventTypeAuthorityDecisionRecorded,
		EventTypeAuthorityExecutionReceipt,
		EventTypeAgentAuditLinked,
	}
}

func registerPhase3ContentUnmarshalers() {
	event.RegisterContentUnmarshaler("agent.identity.requested", event.Unmarshal[AgentIdentityRequestedContent])
	event.RegisterContentUnmarshaler("agent.identity.generated", event.Unmarshal[AgentIdentityGeneratedContent])
	event.RegisterContentUnmarshaler("agent.identity.rejected", event.Unmarshal[AgentIdentityRejectedContent])
	event.RegisterContentUnmarshaler("agent.identity.revoked", event.Unmarshal[AgentIdentityRevokedContent])
	event.RegisterContentUnmarshaler("agent.identity.retired", event.Unmarshal[AgentIdentityRetiredContent])
	event.RegisterContentUnmarshaler("agent.lifecycle.transition.requested", event.Unmarshal[AgentLifecycleTransitionRequestedContent])
	event.RegisterContentUnmarshaler("agent.lifecycle.transitioned", event.Unmarshal[AgentLifecycleTransitionedContent])
	event.RegisterContentUnmarshaler("agent.lifecycle.transition.denied", event.Unmarshal[AgentLifecycleTransitionDeniedContent])
	event.RegisterContentUnmarshaler("agent.authority.scope.assigned", event.Unmarshal[AgentAuthorityScopeAssignedContent])
	event.RegisterContentUnmarshaler("agent.authority.scope.reduced", event.Unmarshal[AgentAuthorityScopeReducedContent])
	event.RegisterContentUnmarshaler("agent.authority.escalation.requested", event.Unmarshal[AgentAuthorityEscalationRequestedContent])
	event.RegisterContentUnmarshaler("agent.authority.escalation.denied", event.Unmarshal[AgentAuthorityEscalationDeniedContent])
	event.RegisterContentUnmarshaler("agent.authority.escalation.approved", event.Unmarshal[AgentAuthorityEscalationApprovedContent])
	event.RegisterContentUnmarshaler("agent.key.generated", event.Unmarshal[AgentKeyGeneratedContent])
	event.RegisterContentUnmarshaler("agent.key.registered", event.Unmarshal[AgentKeyRegisteredContent])
	event.RegisterContentUnmarshaler("agent.key.rotated", event.Unmarshal[AgentKeyRotatedContent])
	event.RegisterContentUnmarshaler("agent.key.revoked", event.Unmarshal[AgentKeyRevokedContent])
	event.RegisterContentUnmarshaler("authority.request.recorded", event.Unmarshal[AuthorityRequestRecordedContent])
	event.RegisterContentUnmarshaler("authority.decision.recorded", event.Unmarshal[AuthorityDecisionRecordedContent])
	event.RegisterContentUnmarshaler("authority.execution.receipt", event.Unmarshal[AuthorityExecutionReceiptContent])
	event.RegisterContentUnmarshaler("agent.audit.linked", event.Unmarshal[AgentAuditLinkedContent])
}

// AgentIdentityRequestedContent records a proposed or trial/persistent identity request.
type AgentIdentityRequestedContent struct {
	hiveContent
	DisplayName             string          `json:"DisplayName"`
	Role                    string          `json:"Role"`
	Purpose                 string          `json:"Purpose"`
	Environment             string          `json:"Environment"`
	IdentityMode            string          `json:"IdentityMode"`
	RequestingActor         types.ActorID   `json:"RequestingActor"`
	RequestedAuthorityScope string          `json:"RequestedAuthorityScope"`
	Justification           string          `json:"Justification"`
	TrialExpiresAt          types.Timestamp `json:"TrialExpiresAt,omitempty"`
	CausalEvidence          []types.EventID `json:"CausalEvidence,omitempty"`
}

func (c AgentIdentityRequestedContent) EventTypeName() string { return "agent.identity.requested" }

// AgentIdentityGeneratedContent records generated key material provenance without private key material.
type AgentIdentityGeneratedContent struct {
	hiveContent
	ActorID        types.ActorID   `json:"ActorID"`
	DisplayName    string          `json:"DisplayName"`
	PublicKey      types.PublicKey `json:"PublicKey"`
	KeyProvenance  string          `json:"KeyProvenance"`
	Environment    string          `json:"Environment"`
	IdentityMode   string          `json:"IdentityMode"`
	RequestEventID types.EventID   `json:"RequestEventID,omitempty"`
}

func (c AgentIdentityGeneratedContent) EventTypeName() string { return "agent.identity.generated" }

// AgentIdentityRejectedContent records rejection of an identity proposal.
type AgentIdentityRejectedContent struct {
	hiveContent
	DisplayName     string          `json:"DisplayName"`
	Environment     string          `json:"Environment"`
	IdentityMode    string          `json:"IdentityMode"`
	RequestingActor types.ActorID   `json:"RequestingActor"`
	RejectedBy      types.ActorID   `json:"RejectedBy"`
	Reason          string          `json:"Reason"`
	RequestEventID  types.EventID   `json:"RequestEventID,omitempty"`
	Evidence        []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentIdentityRejectedContent) EventTypeName() string { return "agent.identity.rejected" }

// AgentLifecycleTransitionRequestedContent records a requested lifecycle transition.
type AgentLifecycleTransitionRequestedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	PreviousState      string          `json:"PreviousState"`
	RequestedState     string          `json:"RequestedState"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	ProtectedAction    string          `json:"ProtectedAction,omitempty"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	Reason             string          `json:"Reason"`
	CausalEvidence     []types.EventID `json:"CausalEvidence,omitempty"`
}

func (c AgentLifecycleTransitionRequestedContent) EventTypeName() string {
	return "agent.lifecycle.transition.requested"
}

// AgentLifecycleTransitionedContent records a completed lifecycle transition.
type AgentLifecycleTransitionedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	PreviousState      string          `json:"PreviousState"`
	RequestedState     string          `json:"RequestedState"`
	ResultingState     string          `json:"ResultingState"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	DecisionEventID    types.EventID   `json:"DecisionEventID,omitempty"`
	Reason             string          `json:"Reason"`
	CausalEvidence     []types.EventID `json:"CausalEvidence,omitempty"`
}

func (c AgentLifecycleTransitionedContent) EventTypeName() string {
	return "agent.lifecycle.transitioned"
}

// AgentLifecycleTransitionDeniedContent records a denied or invalid lifecycle transition.
type AgentLifecycleTransitionDeniedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	PreviousState      string          `json:"PreviousState"`
	RequestedState     string          `json:"RequestedState"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	DecisionEventID    types.EventID   `json:"DecisionEventID,omitempty"`
	Reason             string          `json:"Reason"`
	CausalEvidence     []types.EventID `json:"CausalEvidence,omitempty"`
}

func (c AgentLifecycleTransitionDeniedContent) EventTypeName() string {
	return "agent.lifecycle.transition.denied"
}

// AgentAuthorityScopeAssignedContent records authority scope granted to an agent.
type AgentAuthorityScopeAssignedContent struct {
	hiveContent
	ActorID         types.ActorID   `json:"ActorID"`
	AuthorityScope  string          `json:"AuthorityScope"`
	AssignedBy      types.ActorID   `json:"AssignedBy"`
	DecisionEventID types.EventID   `json:"DecisionEventID,omitempty"`
	ExpiresAt       types.Timestamp `json:"ExpiresAt,omitempty"`
	Constraints     []string        `json:"Constraints,omitempty"`
	Evidence        []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentAuthorityScopeAssignedContent) EventTypeName() string {
	return "agent.authority.scope.assigned"
}

// AgentAuthorityScopeReducedContent records authority scope reduction.
type AgentAuthorityScopeReducedContent struct {
	hiveContent
	ActorID         types.ActorID   `json:"ActorID"`
	PreviousScope   string          `json:"PreviousScope"`
	ResultingScope  string          `json:"ResultingScope"`
	ReducedBy       types.ActorID   `json:"ReducedBy"`
	DecisionEventID types.EventID   `json:"DecisionEventID,omitempty"`
	Reason          string          `json:"Reason"`
	Evidence        []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentAuthorityScopeReducedContent) EventTypeName() string {
	return "agent.authority.scope.reduced"
}

// AgentAuthorityEscalationRequestedContent records a permission escalation request.
type AgentAuthorityEscalationRequestedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	CurrentScope       string          `json:"CurrentScope"`
	RequestedScope     string          `json:"RequestedScope"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	Reason             string          `json:"Reason"`
	TargetActions      []string        `json:"TargetActions,omitempty"`
	TargetEnvironments []string        `json:"TargetEnvironments,omitempty"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	Evidence           []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentAuthorityEscalationRequestedContent) EventTypeName() string {
	return "agent.authority.escalation.requested"
}

// AgentAuthorityEscalationDeniedContent records denied permission escalation.
type AgentAuthorityEscalationDeniedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	CurrentScope       string          `json:"CurrentScope"`
	RequestedScope     string          `json:"RequestedScope"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	DecisionEventID    types.EventID   `json:"DecisionEventID,omitempty"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	Reason             string          `json:"Reason"`
	Evidence           []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentAuthorityEscalationDeniedContent) EventTypeName() string {
	return "agent.authority.escalation.denied"
}

// AgentAuthorityEscalationApprovedContent records approved permission escalation.
type AgentAuthorityEscalationApprovedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	PreviousScope      string          `json:"PreviousScope"`
	ApprovedScope      string          `json:"ApprovedScope"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	DecisionEventID    types.EventID   `json:"DecisionEventID,omitempty"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	Constraints        []string        `json:"Constraints,omitempty"`
	ExpiresAt          types.Timestamp `json:"ExpiresAt,omitempty"`
	Evidence           []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentAuthorityEscalationApprovedContent) EventTypeName() string {
	return "agent.authority.escalation.approved"
}

// AgentKeyGeneratedContent records generated production-safe public key provenance.
type AgentKeyGeneratedContent struct {
	hiveContent
	ActorID       types.ActorID   `json:"ActorID"`
	PublicKey     types.PublicKey `json:"PublicKey"`
	KeyProvenance string          `json:"KeyProvenance"`
	Environment   string          `json:"Environment"`
	IdentityMode  string          `json:"IdentityMode"`
	GeneratedBy   types.ActorID   `json:"GeneratedBy"`
	Reason        string          `json:"Reason"`
}

func (c AgentKeyGeneratedContent) EventTypeName() string { return "agent.key.generated" }

// AgentKeyRegisteredContent records public key registration.
type AgentKeyRegisteredContent struct {
	hiveContent
	ActorID        types.ActorID   `json:"ActorID"`
	PublicKey      types.PublicKey `json:"PublicKey"`
	KeyProvenance  string          `json:"KeyProvenance"`
	Environment    string          `json:"Environment"`
	IdentityMode   string          `json:"IdentityMode"`
	ExternalKeyRef string          `json:"ExternalKeyRef,omitempty"`
	RequestEventID types.EventID   `json:"RequestEventID,omitempty"`
}

func (c AgentKeyRegisteredContent) EventTypeName() string { return "agent.key.registered" }

// AgentKeyRotatedContent records signing authority key rotation without private key material.
type AgentKeyRotatedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	OldPublicKey       types.PublicKey `json:"OldPublicKey"`
	NewPublicKey       types.PublicKey `json:"NewPublicKey"`
	Reason             string          `json:"Reason"`
	RequestingActor    types.ActorID   `json:"RequestingActor"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	DecisionEventID    types.EventID   `json:"DecisionEventID,omitempty"`
	EffectiveAt        types.Timestamp `json:"EffectiveAt"`
}

func (c AgentKeyRotatedContent) EventTypeName() string { return "agent.key.rotated" }

// AgentKeyRevokedContent records key revocation.
type AgentKeyRevokedContent struct {
	hiveContent
	ActorID            types.ActorID   `json:"ActorID"`
	RevokedPublicKey   types.PublicKey `json:"RevokedPublicKey"`
	Reason             string          `json:"Reason"`
	RevokingActor      types.ActorID   `json:"RevokingActor"`
	AuthorityRequestID types.EventID   `json:"AuthorityRequestID,omitempty"`
	DecisionEventID    types.EventID   `json:"DecisionEventID,omitempty"`
	EffectiveAt        types.Timestamp `json:"EffectiveAt"`
	Evidence           []types.EventID `json:"Evidence,omitempty"`
}

func (c AgentKeyRevokedContent) EventTypeName() string { return "agent.key.revoked" }

// AgentIdentityRevokedContent records identity revocation and affected keys.
type AgentIdentityRevokedContent struct {
	hiveContent
	ActorID             types.ActorID     `json:"ActorID"`
	RevokingActor       types.ActorID     `json:"RevokingActor"`
	Reason              string            `json:"Reason"`
	EffectiveAt         types.Timestamp   `json:"EffectiveAt"`
	AffectedKeys        []types.PublicKey `json:"AffectedKeys,omitempty"`
	ReplacementActor    types.ActorID     `json:"ReplacementActor,omitempty"`
	OpenWorkDisposition string            `json:"OpenWorkDisposition,omitempty"`
	AuditEvidence       []types.EventID   `json:"AuditEvidence,omitempty"`
}

func (c AgentIdentityRevokedContent) EventTypeName() string { return "agent.identity.revoked" }

// AgentIdentityRetiredContent records identity retirement and final audit disposition.
type AgentIdentityRetiredContent struct {
	hiveContent
	ActorID             types.ActorID     `json:"ActorID"`
	RetiringActor       types.ActorID     `json:"RetiringActor"`
	Reason              string            `json:"Reason"`
	EffectiveAt         types.Timestamp   `json:"EffectiveAt"`
	AffectedKeys        []types.PublicKey `json:"AffectedKeys,omitempty"`
	ReplacementActor    types.ActorID     `json:"ReplacementActor,omitempty"`
	OpenWorkDisposition string            `json:"OpenWorkDisposition,omitempty"`
	AuditEvidence       []types.EventID   `json:"AuditEvidence,omitempty"`
}

func (c AgentIdentityRetiredContent) EventTypeName() string { return "agent.identity.retired" }

// AuthorityRequestRecordedContent records the full Phase 3 authority request
// envelope linked to the canonical protected-action request event.
type AuthorityRequestRecordedContent struct {
	hiveContent
	RequestID         types.EventID   `json:"RequestID"`
	RequestingActor   types.ActorID   `json:"RequestingActor"`
	ActionName        string          `json:"ActionName"`
	Target            string          `json:"Target"`
	Environment       string          `json:"Environment"`
	RequestedOutcome  string          `json:"RequestedOutcome"`
	Justification     string          `json:"Justification"`
	RiskSummary       string          `json:"RiskSummary"`
	EvidenceReviewed  []types.EventID `json:"EvidenceReviewed,omitempty"`
	ProposedOperation string          `json:"ProposedOperation,omitempty"`
	CausalEventIDs    []types.EventID `json:"CausalEventIDs,omitempty"`
	ExpiresAt         types.Timestamp `json:"ExpiresAt,omitempty"`
}

func (c AuthorityRequestRecordedContent) EventTypeName() string {
	return "authority.request.recorded"
}

// AuthorityDecisionRecordedContent records a Phase 3 authority decision.
type AuthorityDecisionRecordedContent struct {
	hiveContent
	DecisionID       string          `json:"DecisionID"`
	RequestID        types.EventID   `json:"RequestID"`
	ApproverActor    types.ActorID   `json:"ApproverActor"`
	Outcome          string          `json:"Outcome"`
	ApprovedTarget   string          `json:"ApprovedTarget"`
	ApprovedAction   string          `json:"ApprovedAction"`
	Conditions       []string        `json:"Conditions,omitempty"`
	ExpiresAt        types.Timestamp `json:"ExpiresAt,omitempty"`
	EvidenceReviewed []types.EventID `json:"EvidenceReviewed,omitempty"`
	Rationale        string          `json:"Rationale"`
}

func (c AuthorityDecisionRecordedContent) EventTypeName() string {
	return "authority.decision.recorded"
}

// AuthorityExecutionReceiptContent records execution of an approved protected action.
type AuthorityExecutionReceiptContent struct {
	hiveContent
	RequestID           types.EventID   `json:"RequestID"`
	DecisionEventID     types.EventID   `json:"DecisionEventID"`
	ExecutingActor      types.ActorID   `json:"ExecutingActor"`
	Operation           string          `json:"Operation"`
	TargetStateBefore   string          `json:"TargetStateBefore"`
	TargetStateAfter    string          `json:"TargetStateAfter"`
	ResultStatus        string          `json:"ResultStatus"`
	FailureDetails      string          `json:"FailureDetails,omitempty"`
	ProducedResourceIDs []string        `json:"ProducedResourceIDs,omitempty"`
	CausalEventIDs      []types.EventID `json:"CausalEventIDs,omitempty"`
}

func (c AuthorityExecutionReceiptContent) EventTypeName() string {
	return "authority.execution.receipt"
}

// AgentAuditLinkedContent links lifecycle, authority, key, trust, and decision evidence.
type AgentAuditLinkedContent struct {
	hiveContent
	SubjectActorID    types.ActorID   `json:"SubjectActorID"`
	RecordKind        string          `json:"RecordKind"`
	TrustEvidence     []types.EventID `json:"TrustEvidence,omitempty"`
	DecisionEvidence  []types.EventID `json:"DecisionEvidence,omitempty"`
	AuthorityEvidence []types.EventID `json:"AuthorityEvidence,omitempty"`
	KeyEvidence       []types.EventID `json:"KeyEvidence,omitempty"`
	LifecycleEvidence []types.EventID `json:"LifecycleEvidence,omitempty"`
	AuditEvidence     []types.EventID `json:"AuditEvidence,omitempty"`
	Rationale         string          `json:"Rationale"`
}

func (c AgentAuditLinkedContent) EventTypeName() string { return "agent.audit.linked" }
