package hive

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestBuildOperatorProjectionPendingAndDecisions(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)

	pendingRequestID := appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:         newTestEventID(t),
		RequestingActor:   actorID,
		ActionName:        "agent.retire",
		Target:            "actor_target",
		Environment:       "production",
		RequestedOutcome:  "retire agent",
		Justification:     "operator requested retirement",
		RiskSummary:       "protected lifecycle action",
		ProposedOperation: "retire",
	}).Content().(AuthorityRequestRecordedContent).RequestID

	decidedRequestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        decidedRequestID,
		RequestingActor:  actorID,
		ActionName:       "agent.revoke",
		Target:           "actor_revoked",
		Environment:      "production",
		RequestedOutcome: "revoke agent",
		Justification:    "compromised key",
		RiskSummary:      "key compromise",
	})
	appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{
		DecisionID:     "decision-1",
		RequestID:      decidedRequestID,
		ApproverActor:  actorID,
		Outcome:        "approved",
		ApprovedTarget: "actor_revoked",
		ApprovedAction: "agent.revoke",
		Rationale:      "valid revocation evidence",
	})

	projection := BuildOperatorProjection(s, 50)

	if len(projection.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1: %+v", len(projection.PendingApprovals), projection.PendingApprovals)
	}
	if projection.PendingApprovals[0].RequestID != pendingRequestID.Value() {
		t.Fatalf("pending request ID = %q, want %q", projection.PendingApprovals[0].RequestID, pendingRequestID.Value())
	}
	if len(projection.AuthorityDecisions) != 1 {
		t.Fatalf("authority decisions = %d, want 1", len(projection.AuthorityDecisions))
	}
	decision := projection.AuthorityDecisions[0]
	if decision.RequestID != decidedRequestID.Value() || decision.Outcome != "approved" {
		t.Fatalf("decision projection = %+v", decision)
	}
	if decision.RequestedAction != "agent.revoke" || decision.RequestedTarget != "actor_revoked" {
		t.Fatalf("decision did not join request details: %+v", decision)
	}
}

func TestBuildOperatorProjectionLifecycleAndKeyAudit(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	publicKey := types.MustPublicKey(make([]byte, 32))
	newPublicKeyBytes := make([]byte, 32)
	newPublicKeyBytes[0] = 1
	newPublicKey := types.MustPublicKey(newPublicKeyBytes)
	requestID := newTestEventID(t)
	decisionID := newTestEventID(t)

	appendEvent(EventTypeAgentIdentityRegistered, AgentIdentityRegisteredContent{
		ActorID:          actorID,
		DisplayName:      "builder",
		Role:             "builder",
		PublicKey:        publicKey,
		KeyProvenance:    "generated",
		Environment:      "production",
		IdentityMode:     "persistent",
		LifecycleStatus:  "active",
		AuthorityScope:   "hive:read",
		RegistrationPath: "generated",
	})
	appendEvent(EventTypeAgentAuthorityScopeAssigned, AgentAuthorityScopeAssignedContent{
		ActorID:         actorID,
		AuthorityScope:  "hive:operate",
		AssignedBy:      actorID,
		DecisionEventID: decisionID,
	})
	appendEvent(EventTypeAgentLifecycleTransitioned, AgentLifecycleTransitionedContent{
		ActorID:            actorID,
		PreviousState:      "active",
		RequestedState:     "retired",
		ResultingState:     "retired",
		RequestingActor:    actorID,
		AuthorityRequestID: requestID,
		DecisionEventID:    decisionID,
		Reason:             "completed mandate",
	})
	appendEvent(EventTypeAgentKeyRegistered, AgentKeyRegisteredContent{
		ActorID:       actorID,
		PublicKey:     publicKey,
		KeyProvenance: "generated",
		Environment:   "production",
		IdentityMode:  "persistent",
	})
	appendEvent(EventTypeAgentKeyRotated, AgentKeyRotatedContent{
		ActorID:            actorID,
		OldPublicKey:       publicKey,
		NewPublicKey:       newPublicKey,
		Reason:             "scheduled rotation",
		RequestingActor:    actorID,
		AuthorityRequestID: requestID,
		DecisionEventID:    decisionID,
		EffectiveAt:        types.Now(),
	})
	appendEvent(EventTypeAgentAuditLinked, AgentAuditLinkedContent{
		SubjectActorID: actorID,
		RecordKind:     "key-rotation",
		KeyEvidence:    []types.EventID{requestID},
		Rationale:      "rotation linked to authority decision",
	})

	projection := BuildOperatorProjection(s, 50)

	if len(projection.Lifecycle) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(projection.Lifecycle))
	}
	lifecycle := projection.Lifecycle[0]
	if lifecycle.ActorID != actorID.Value() || lifecycle.LifecycleStatus != "retired" {
		t.Fatalf("lifecycle projection = %+v", lifecycle)
	}
	if lifecycle.AuthorityScope != "hive:operate" {
		t.Fatalf("authority scope = %q, want hive:operate", lifecycle.AuthorityScope)
	}

	if len(projection.KeyAuditTraces) != 3 {
		t.Fatalf("key audit traces = %d, want 3: %+v", len(projection.KeyAuditTraces), projection.KeyAuditTraces)
	}
	if projection.KeyAuditTraces[0].EventType != EventTypeAgentAuditLinked.Value() {
		t.Fatalf("newest key audit trace = %q, want %q", projection.KeyAuditTraces[0].EventType, EventTypeAgentAuditLinked.Value())
	}
}

func newOperatorProjectionStore(t *testing.T) (*store.InMemoryStore, types.ActorID, func(types.EventType, event.EventContent) event.Event) {
	t.Helper()
	RegisterEventTypes()
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)

	s := store.NewInMemoryStore()
	actorID := types.MustActorID("actor_00000000000000000000000000000077")
	signer := deriveSignerFromID(actorID)
	bootstrap, err := event.NewBootstrapFactory(registry).Init(actorID, signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	factory := event.NewEventFactory(registry)
	convID := types.MustConversationID("conv_00000000000000000000000000000077")

	appendEvent := func(eventType types.EventType, content event.EventContent) event.Event {
		t.Helper()
		head, err := s.Head()
		if err != nil {
			t.Fatalf("head: %v", err)
		}
		ev, err := factory.Create(eventType, actorID, content, []types.EventID{head.Unwrap().ID()}, convID, s, signer)
		if err != nil {
			t.Fatalf("create %s: %v", eventType.Value(), err)
		}
		stored, err := s.Append(ev)
		if err != nil {
			t.Fatalf("append %s: %v", eventType.Value(), err)
		}
		return stored
	}

	return s, actorID, appendEvent
}
