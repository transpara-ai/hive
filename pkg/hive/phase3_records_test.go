package hive

import (
	"encoding/json"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestPhase3RecordTypesRegisteredAndCreatable(t *testing.T) {
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)

	s := store.NewInMemoryStore()
	actorID := types.MustActorID("actor_00000000000000000000000000000042")
	signer := deriveSignerFromID(actorID)
	bootstrap, err := event.NewBootstrapFactory(registry).Init(actorID, signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}

	factory := event.NewEventFactory(registry)
	convID := types.MustConversationID("conv_00000000000000000000000000000042")
	for _, tc := range phase3RecordContentCases(t, actorID) {
		t.Run(tc.eventType.Value(), func(t *testing.T) {
			if !registry.IsRegistered(tc.eventType) {
				t.Fatalf("%s is not registered", tc.eventType.Value())
			}
			head, err := s.Head()
			if err != nil {
				t.Fatalf("head: %v", err)
			}
			ev, err := factory.Create(tc.eventType, actorID, tc.content, []types.EventID{head.Unwrap().ID()}, convID, s, signer)
			if err != nil {
				t.Fatalf("create %s: %v", tc.eventType.Value(), err)
			}
			if ev.Content().EventTypeName() != tc.eventType.Value() {
				t.Fatalf("content EventTypeName = %q, want %q", ev.Content().EventTypeName(), tc.eventType.Value())
			}
			if _, err := s.Append(ev); err != nil {
				t.Fatalf("append %s: %v", tc.eventType.Value(), err)
			}
		})
	}
}

func TestRegisterEventTypesIncludesPhase3Unmarshalers(t *testing.T) {
	RegisterEventTypes()

	for _, tc := range phase3RecordContentCases(t, types.MustActorID("actor_00000000000000000000000000000043")) {
		t.Run(tc.eventType.Value(), func(t *testing.T) {
			got, err := event.UnmarshalContent(tc.eventType.Value(), mustJSON(t, tc.content))
			if err != nil {
				t.Fatalf("unmarshal %s: %v", tc.eventType.Value(), err)
			}
			if got.EventTypeName() != tc.eventType.Value() {
				t.Fatalf("unmarshaled EventTypeName = %q, want %q", got.EventTypeName(), tc.eventType.Value())
			}
		})
	}
}

func phase3RecordContentCases(t *testing.T, actorID types.ActorID) []struct {
	eventType types.EventType
	content   event.EventContent
} {
	t.Helper()
	eventID := newTestEventID(t)
	decisionID := newTestEventID(t)
	publicKey := types.MustPublicKey(make([]byte, 32))
	replacementActor := types.MustActorID("actor_00000000000000000000000000000044")
	now := types.Now()

	return []struct {
		eventType types.EventType
		content   event.EventContent
	}{
		{EventTypeAgentIdentityRequested, AgentIdentityRequestedContent{
			DisplayName: "builder", Role: "builder", Purpose: "implementation", Environment: "production",
			IdentityMode: "generated", RequestingActor: actorID, RequestedAuthorityScope: "hive",
			Justification: "phase 3 identity", CausalEvidence: []types.EventID{eventID},
		}},
		{EventTypeAgentIdentityGenerated, AgentIdentityGeneratedContent{
			ActorID: actorID, DisplayName: "builder", PublicKey: publicKey, KeyProvenance: "generated",
			Environment: "production", IdentityMode: "generated", RequestEventID: eventID,
		}},
		{EventTypeAgentIdentityRejected, AgentIdentityRejectedContent{
			DisplayName: "builder", Environment: "production", IdentityMode: "deterministic_fixture",
			RequestingActor: actorID, RejectedBy: actorID, Reason: "fixtures are not production",
			RequestEventID: eventID, Evidence: []types.EventID{eventID},
		}},
		{EventTypeAgentIdentityRevoked, AgentIdentityRevokedContent{
			ActorID: actorID, RevokingActor: actorID, Reason: "compromise", EffectiveAt: now,
			AffectedKeys: []types.PublicKey{publicKey}, ReplacementActor: replacementActor,
			OpenWorkDisposition: "reassigned", AuditEvidence: []types.EventID{eventID},
		}},
		{EventTypeAgentIdentityRetired, AgentIdentityRetiredContent{
			ActorID: actorID, RetiringActor: actorID, Reason: "superseded", EffectiveAt: now,
			AffectedKeys: []types.PublicKey{publicKey}, ReplacementActor: replacementActor,
			OpenWorkDisposition: "drained", AuditEvidence: []types.EventID{eventID},
		}},
		{EventTypeAgentLifecycleTransitionRequested, AgentLifecycleTransitionRequestedContent{
			ActorID: actorID, PreviousState: "trial", RequestedState: "active", RequestingActor: actorID,
			ProtectedAction: "agent.spawn.persistent", AuthorityRequestID: eventID, Reason: "persistence approved",
			CausalEvidence: []types.EventID{eventID},
		}},
		{EventTypeAgentLifecycleTransitioned, AgentLifecycleTransitionedContent{
			ActorID: actorID, PreviousState: "trial", RequestedState: "active", ResultingState: "active",
			RequestingActor: actorID, AuthorityRequestID: eventID, DecisionEventID: decisionID,
			Reason: "approved", CausalEvidence: []types.EventID{eventID},
		}},
		{EventTypeAgentLifecycleTransitionDenied, AgentLifecycleTransitionDeniedContent{
			ActorID: actorID, PreviousState: "trial", RequestedState: "retired", RequestingActor: actorID,
			AuthorityRequestID: eventID, DecisionEventID: decisionID, Reason: "invalid transition",
			CausalEvidence: []types.EventID{eventID},
		}},
		{EventTypeAgentAuthorityScopeAssigned, AgentAuthorityScopeAssignedContent{
			ActorID: actorID, AuthorityScope: "hive:read", AssignedBy: actorID, DecisionEventID: decisionID,
			ExpiresAt: now, Constraints: []string{"no protected side effects"}, Evidence: []types.EventID{eventID},
		}},
		{EventTypeAgentAuthorityScopeReduced, AgentAuthorityScopeReducedContent{
			ActorID: actorID, PreviousScope: "hive:write", ResultingScope: "hive:read", ReducedBy: actorID,
			DecisionEventID: decisionID, Reason: "risk reduction", Evidence: []types.EventID{eventID},
		}},
		{EventTypeAgentAuthorityEscalationRequested, AgentAuthorityEscalationRequestedContent{
			ActorID: actorID, CurrentScope: "hive:read", RequestedScope: "hive:write",
			RequestingActor: actorID, Reason: "task requires write", TargetActions: []string{"repo.mutate.cross_repo"},
			TargetEnvironments: []string{"development"}, AuthorityRequestID: eventID, Evidence: []types.EventID{eventID},
		}},
		{EventTypeAgentAuthorityEscalationDenied, AgentAuthorityEscalationDeniedContent{
			ActorID: actorID, CurrentScope: "hive:read", RequestedScope: "hive:write",
			RequestingActor: actorID, DecisionEventID: decisionID, AuthorityRequestID: eventID,
			Reason: "insufficient evidence", Evidence: []types.EventID{eventID},
		}},
		{EventTypeAgentAuthorityEscalationApproved, AgentAuthorityEscalationApprovedContent{
			ActorID: actorID, PreviousScope: "hive:read", ApprovedScope: "hive:write",
			RequestingActor: actorID, DecisionEventID: decisionID, AuthorityRequestID: eventID,
			Constraints: []string{"feature branch only"}, ExpiresAt: now, Evidence: []types.EventID{eventID},
		}},
		{EventTypeAgentKeyGenerated, AgentKeyGeneratedContent{
			ActorID: actorID, PublicKey: publicKey, KeyProvenance: "generated", Environment: "production",
			IdentityMode: "generated", GeneratedBy: actorID, Reason: "agent birth",
		}},
		{EventTypeAgentKeyRegistered, AgentKeyRegisteredContent{
			ActorID: actorID, PublicKey: publicKey, KeyProvenance: "externally_managed", Environment: "production",
			IdentityMode: "externally_managed", ExternalKeyRef: "kms://prod/agents/builder", RequestEventID: eventID,
		}},
		{EventTypeAgentKeyRotated, AgentKeyRotatedContent{
			ActorID: actorID, OldPublicKey: publicKey, NewPublicKey: publicKey, Reason: "rotation",
			RequestingActor: actorID, AuthorityRequestID: eventID, DecisionEventID: decisionID, EffectiveAt: now,
		}},
		{EventTypeAgentKeyRevoked, AgentKeyRevokedContent{
			ActorID: actorID, RevokedPublicKey: publicKey, Reason: "revocation",
			RevokingActor: actorID, AuthorityRequestID: eventID, DecisionEventID: decisionID,
			EffectiveAt: now, Evidence: []types.EventID{eventID},
		}},
		{EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
			RequestID: eventID, RequestingActor: actorID, ActionName: "agent.spawn.persistent",
			Target: "agent:builder", Environment: "production", RequestedOutcome: "promote trial to active",
			RequestingRole: "operator", RiskClass: "high", Justification: "persistent identity is required",
			RiskSummary: "long-lived agent authority", Scope: []string{"agent.spawn.persistent"},
			EvidenceReviewed: []types.EventID{eventID}, ProposedOperation: "register active identity",
			CausalEventIDs: []types.EventID{eventID}, ExpiresAt: now,
		}},
		{EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{
			DecisionID: decisionID.Value(), RequestID: eventID, ApproverActor: actorID,
			Outcome: "approved", ApprovedTarget: "agent:builder", ApprovedAction: "agent.spawn.persistent",
			DeciderRole: "operator", Scope: []string{"agent.spawn.persistent"}, Conditions: []string{"scope=hive"},
			ExpiresAt: now, EvidenceReviewed: []types.EventID{eventID}, Rationale: "bounded persistence",
		}},
		{EventTypeAuthorityExecutionReceipt, AuthorityExecutionReceiptContent{
			RequestID: eventID, DecisionEventID: decisionID, ExecutingActor: actorID,
			Operation: "agent identity registration", TargetStateBefore: "trial", TargetStateAfter: "active",
			ResultStatus: "success", ProducedResourceIDs: []string{actorID.Value()}, CausalEventIDs: []types.EventID{eventID},
		}},
		{EventTypeAgentAuditLinked, AgentAuditLinkedContent{
			SubjectActorID: actorID, RecordKind: "identity.lifecycle",
			TrustEvidence: []types.EventID{eventID}, DecisionEvidence: []types.EventID{decisionID},
			AuthorityEvidence: []types.EventID{eventID}, KeyEvidence: []types.EventID{eventID},
			LifecycleEvidence: []types.EventID{eventID}, AuditEvidence: []types.EventID{eventID},
			Rationale: "phase 3 audit linkage",
		}},
	}
}

func newTestEventID(t *testing.T) types.EventID {
	t.Helper()
	id, err := types.NewEventIDFromNew()
	if err != nil {
		t.Fatalf("NewEventIDFromNew: %v", err)
	}
	return id
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return b
}
