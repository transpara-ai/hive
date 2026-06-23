package hive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
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
		RequestingRole:   "operator",
		ActionName:       "agent.revoke",
		Target:           "actor_revoked",
		Environment:      "production",
		RiskClass:        "critical",
		RequestedOutcome: "revoke agent",
		Justification:    "compromised key",
		RiskSummary:      "key compromise",
		Scope:            []string{"agent.revoke"},
	})
	appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{
		DecisionID:     "decision-1",
		RequestID:      decidedRequestID,
		ApproverActor:  actorID,
		DeciderRole:    "operator",
		Outcome:        "approved",
		ApprovedTarget: "actor_revoked",
		ApprovedAction: "agent.revoke",
		Scope:          []string{"agent.revoke"},
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
	if decision.DeciderRole != "operator" {
		t.Fatalf("decision decider role = %q, want operator", decision.DeciderRole)
	}
	if len(decision.Scope) != 1 || decision.Scope[0] != "agent.revoke" {
		t.Fatalf("decision scope = %#v, want [agent.revoke]", decision.Scope)
	}
}

func TestBuildCivilizationAssemblyProjectionDerivesOperatorRuntimeState(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	requestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        requestID,
		RequestingActor:  actorID,
		RequestingRole:   "operator",
		ActionName:       "pull_request.create",
		Target:           "pr://transpara-ai/site/42",
		Environment:      "review",
		RiskClass:        "medium",
		RequestedOutcome: "draft PR",
		Justification:    "bounded implementation ready",
		RiskSummary:      "protected GitHub write",
		Scope:            []string{"repo:transpara-ai/site", "head:abc123"},
	})
	appendEvent(EventTypeAgentIdentityRegistered, AgentIdentityRegisteredContent{
		ActorID:          actorID,
		DisplayName:      "Reviewer",
		Role:             "reviewer",
		PublicKey:        types.MustPublicKey(make([]byte, 32)),
		KeyProvenance:    "generated",
		Environment:      "review",
		IdentityMode:     "persistent",
		LifecycleStatus:  "active",
		AuthorityScope:   "hive:review",
		RegistrationPath: "generated",
	})
	appendEvent(EventTypeRunStarted, RunStartedContent{
		Idea:     "runtime visualization",
		RepoPath: "/Transpara/transpara-ai/repos/site",
	})
	appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "builder",
		Role:    "implementer",
		Model:   "claude",
		ActorID: actorID.Value(),
	})

	projection := BuildCivilizationAssemblyProjection(s, 50)

	if projection.ProjectionSubject != civilizationAssemblyProjectionSubject {
		t.Fatalf("subject = %q, want %q", projection.ProjectionSubject, civilizationAssemblyProjectionSubject)
	}
	if projection.DerivationStatus != civilizationAssemblyStatusComplete {
		t.Fatalf("derivation status = %q, want complete; failures=%+v", projection.DerivationStatus, projection.FailureReasons)
	}
	if projection.AuthorityState.Status != civilizationAssemblyFieldAvailable || len(projection.AuthorityState.AuthorityRequests) != 1 {
		t.Fatalf("authority state = %+v, want one available request", projection.AuthorityState)
	}
	if got := projection.AuthorityState.AuthorityRequests[0].TargetType; got != "pr" {
		t.Fatalf("target type = %q, want pr", got)
	}
	if projection.ExternalCommitteeState.Status != civilizationAssemblyFieldUnavailable || len(projection.ExternalCommitteeState.DecisionRefs) != 0 {
		t.Fatalf("external committee state = %+v, want unavailable with no Hive decision refs", projection.ExternalCommitteeState)
	}
	if len(projection.ActorRoster) == 0 {
		t.Fatalf("actor roster empty: %+v", projection)
	}
	if !civilizationProjectionHasRoleBinding(projection, actorID.Value(), "reviewer") {
		t.Fatalf("missing reviewer role binding: %+v", projection.RoleBindings)
	}
	if projection.WorkEvidenceSummary.Status != civilizationAssemblyFieldAvailable {
		t.Fatalf("work evidence = %+v, want available runtime evidence", projection.WorkEvidenceSummary)
	}
	if !civilizationProjectionHasLifecycleStatus(projection, actorID.Value(), "active") {
		t.Fatalf("missing normalized active lifecycle status: %+v", projection.AgentLifecycleSummary)
	}
	if got := civilizationProjectionLifecycleCount(projection, actorID.Value()); got != 1 {
		t.Fatalf("lifecycle rows for %s = %d, want 1: %+v", actorID.Value(), got, projection.AgentLifecycleSummary)
	}
	if projection.FactoryOrderSummary == nil || projection.RoleBindings == nil || projection.AgentLifecycleSummary == nil || projection.ResidualRiskSummary == nil {
		t.Fatalf("top-level slices must serialize as arrays, got factory=%#v bindings=%#v lifecycle=%#v risks=%#v", projection.FactoryOrderSummary, projection.RoleBindings, projection.AgentLifecycleSummary, projection.ResidualRiskSummary)
	}
	if len(projection.OpenGateSummary) != 1 || projection.OpenGateSummary[0].ID != requestID.Value() {
		t.Fatalf("open gates = %+v, want pending request %s", projection.OpenGateSummary, requestID.Value())
	}
	if projection.SourceEventGraphHeadOrStateVersion == "" || projection.SourceEventGraphHeadOrStateVersion == "eventgraph head unavailable" {
		t.Fatalf("source head unavailable: %q", projection.SourceEventGraphHeadOrStateVersion)
	}
	if !containsString(projection.BoundaryFlags, "read_only_site_consumer") || !containsString(projection.ValidationRefs, civilizationAssemblyReadOnlyRoutePath) {
		t.Fatalf("missing read-only boundary evidence: flags=%+v validation=%+v", projection.BoundaryFlags, projection.ValidationRefs)
	}
}

func TestBuildCivilizationAssemblyProjectionEmptyStoreKeepsUnavailableFieldsAndArrays(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)

	projection := BuildCivilizationAssemblyProjection(s, 50)

	if projection.DerivationStatus != civilizationAssemblyStatusComplete {
		t.Fatalf("derivation status = %q, want complete", projection.DerivationStatus)
	}
	if projection.ActorRoster == nil || projection.RoleBindings == nil || projection.AgentLifecycleSummary == nil || projection.FactoryOrderSummary == nil || projection.ResidualRiskSummary == nil {
		t.Fatalf("top-level slices must be arrays, got actor=%#v bindings=%#v lifecycle=%#v factory=%#v risks=%#v", projection.ActorRoster, projection.RoleBindings, projection.AgentLifecycleSummary, projection.FactoryOrderSummary, projection.ResidualRiskSummary)
	}
	if projection.WorkEvidenceSummary.Status != civilizationAssemblyFieldUnavailable || len(projection.WorkEvidenceSummary.SourceRefs) != 0 {
		t.Fatalf("work evidence = %+v, want unavailable with no refs", projection.WorkEvidenceSummary)
	}
	if !civilizationProjectionHasUnavailableField(projection, "actor_roster") {
		t.Fatalf("missing actor_roster unavailable field: %+v", projection.WithheldOrUnavailableFields)
	}
}

func TestBuildCivilizationAssemblyProjectionToleratesOpaqueSharedStoreHead(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	opaqueHead := appendOpaqueSharedStoreEvent(t, s, actorID)

	projection := BuildCivilizationAssemblyProjection(s, 50)

	if projection.DerivationStatus != civilizationAssemblyStatusComplete {
		t.Fatalf("derivation status = %q, want complete; failures=%+v", projection.DerivationStatus, projection.FailureReasons)
	}
	if len(projection.FailureReasons) != 0 {
		t.Fatalf("failure reasons = %+v, want none", projection.FailureReasons)
	}
	if projection.SourceEventGraphHeadOrStateVersion != opaqueHead.ID().Value() {
		t.Fatalf("source head = %q, want opaque head %q", projection.SourceEventGraphHeadOrStateVersion, opaqueHead.ID().Value())
	}
}

func TestCivilizationAssemblyTargetType(t *testing.T) {
	tests := map[string]string{
		"pr://transpara-ai/site/42": "pr",
		"actor_123":                 "actor",
		"repo:transpara-ai/site":    "repo",
		"plain-target":              "target",
		"":                          "",
	}
	for target, want := range tests {
		if got := civilizationAssemblyTargetType(target); got != want {
			t.Fatalf("target type for %q = %q, want %q", target, got, want)
		}
	}
}

func civilizationProjectionHasRoleBinding(projection CivilizationAssemblyProjection, actorID, role string) bool {
	for _, binding := range projection.RoleBindings {
		if binding.ActorID == actorID && binding.Role == role {
			return true
		}
	}
	return false
}

func civilizationProjectionHasLifecycleStatus(projection CivilizationAssemblyProjection, actorID, status string) bool {
	for _, lifecycle := range projection.AgentLifecycleSummary {
		if lifecycle.ActorID == actorID && lifecycle.Status == status {
			return true
		}
	}
	return false
}

func civilizationProjectionLifecycleCount(projection CivilizationAssemblyProjection, actorID string) int {
	count := 0
	for _, lifecycle := range projection.AgentLifecycleSummary {
		if lifecycle.ActorID == actorID {
			count++
		}
	}
	return count
}

func civilizationProjectionHasUnavailableField(projection CivilizationAssemblyProjection, field string) bool {
	for _, item := range projection.WithheldOrUnavailableFields {
		if item.Field == field {
			return true
		}
	}
	return false
}

func TestBuildOperatorProjectionPendingScansBeyondDecisionDisplayLimit(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)

	decidedRequestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        decidedRequestID,
		RequestingActor:  actorID,
		ActionName:       "agent.retire",
		Target:           "actor_decided",
		Environment:      "production",
		RequestedOutcome: "retire agent",
		Justification:    "completed mandate",
		RiskSummary:      "protected lifecycle action",
	})
	appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{
		DecisionID:     "decision-older",
		RequestID:      decidedRequestID,
		ApproverActor:  actorID,
		Outcome:        "approved",
		ApprovedTarget: "actor_decided",
		ApprovedAction: "agent.retire",
		Rationale:      "valid evidence",
	})

	pendingRequestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        pendingRequestID,
		RequestingActor:  actorID,
		ActionName:       "agent.revoke",
		Target:           "actor_pending",
		Environment:      "production",
		RequestedOutcome: "revoke agent",
		Justification:    "needs operator decision",
		RiskSummary:      "protected lifecycle action",
	})

	for i := 0; i < 3; i++ {
		appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{
			DecisionID:     "decision-unrelated",
			RequestID:      newTestEventID(t),
			ApproverActor:  actorID,
			Outcome:        "approved",
			ApprovedTarget: "unrelated",
			ApprovedAction: "agent.spawn.persistent",
			Rationale:      "unrelated history",
		})
	}

	projection := BuildOperatorProjection(s, 2)

	if len(projection.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1: %+v", len(projection.PendingApprovals), projection.PendingApprovals)
	}
	if projection.PendingApprovals[0].RequestID != pendingRequestID.Value() {
		t.Fatalf("pending request ID = %q, want %q", projection.PendingApprovals[0].RequestID, pendingRequestID.Value())
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

func TestBuildOperatorProjectionRuntimeEvidenceDistinguishesQueuedIntent(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)
	sourceEventID := newTestEventID(t)
	briefEventID := newTestEventID(t)

	appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
		RunID:      "run_queued",
		IntakeID:   "intake_001",
		OperatorID: "operator_001",
		Title:      "Test 001 evidence run",
		Status:     "queued",
		Authority: RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        "test-001",
			PolicyRef:    "DF-GATE-K",
			Rationale:    "operator request requires explicit authority",
		},
		Budget: RunLaunchBudget{
			MaxIterations: 3,
			MaxCostUSD:    12.5,
		},
		TargetRepos:   []string{"transpara-ai/hive", "transpara-ai/eventgraph"},
		SourceEventID: sourceEventID,
		BriefEventID:  briefEventID,
	})

	projection := BuildOperatorProjection(s, 50)
	runtimeEvidence := projection.RuntimeEvidence

	if runtimeEvidence.Status != "not_observed" {
		t.Fatalf("runtime status = %q, want not_observed", runtimeEvidence.Status)
	}
	if runtimeEvidence.LastRun != nil {
		t.Fatalf("last run = %+v, want nil for queued request only", runtimeEvidence.LastRun)
	}
	if runtimeEvidence.LastQueuedRunRequest == nil {
		t.Fatal("last queued run request is nil")
	}
	queued := runtimeEvidence.LastQueuedRunRequest
	if queued.RunID != "run_queued" || queued.EvidenceKind != "queued_request_not_runtime_start" {
		t.Fatalf("queued evidence = %+v", queued)
	}
	if queued.ConversationID == "" {
		t.Fatalf("queued conversation ID is empty: %+v", queued)
	}
	if queued.AuthorityInitialLevel != string(event.AuthorityLevelRequired) || queued.AuthorityScope != "test-001" {
		t.Fatalf("queued authority evidence = %+v", queued)
	}
	if queued.BudgetMaxIterations == nil || *queued.BudgetMaxIterations != 3 || queued.BudgetMaxCostUSD == nil || *queued.BudgetMaxCostUSD != 12.5 {
		t.Fatalf("queued budget evidence = %+v", queued)
	}
	if len(queued.TargetRepos) != 2 || queued.TargetRepos[0] != "transpara-ai/hive" {
		t.Fatalf("queued target repos = %+v", queued.TargetRepos)
	}
	if runtimeEvidence.AgentEvents.Scope != "none" || runtimeEvidence.AgentEvents.ObservedActive != 0 {
		t.Fatalf("agent evidence = %+v, want no runtime agents", runtimeEvidence.AgentEvents)
	}
	if !containsModelProjectionString(runtimeEvidence.Limitations, "factory.run.requested is queued launch intent, not runtime-start proof") {
		t.Fatalf("limitations = %+v, want queued-intent boundary", runtimeEvidence.Limitations)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceIgnoresEventsWithoutRunStart(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)

	appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "orphan-builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_orphan",
	})
	appendEvent(EventTypeAgentStopped, AgentStoppedContent{
		Name:       "orphan-builder",
		Role:       "implementer",
		StopReason: "orphaned",
		Iterations: 1,
	})
	appendEvent(EventTypeRunCompleted, RunCompletedContent{
		AgentCount: 0,
		DurationMs: 0,
		TotalCost:  0,
	})

	projection := BuildOperatorProjection(s, 50)
	runtimeEvidence := projection.RuntimeEvidence
	if runtimeEvidence.Status != "not_observed" {
		t.Fatalf("runtime status = %q, want not_observed", runtimeEvidence.Status)
	}
	if runtimeEvidence.LastRun != nil {
		t.Fatalf("last run = %+v, want nil without run start", runtimeEvidence.LastRun)
	}
	if runtimeEvidence.AgentEvents.Spawned != 0 || runtimeEvidence.AgentEvents.Stopped != 0 || runtimeEvidence.AgentEvents.ObservedActive != 0 {
		t.Fatalf("agent events = %+v, want no pre-start events counted", runtimeEvidence.AgentEvents)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceKeepsRunAnchorWithAgentChurnLimit(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	convID := types.MustConversationID("conv_runtime_evidence_churn")
	started := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     "long running",
		RepoPath: "/tmp/churn",
	})
	for i := 0; i < 5; i++ {
		appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentSpawned, AgentSpawnedContent{
			Name:    "builder",
			Role:    "implementer",
			Model:   "claude-opus-4-6",
			ActorID: "actor_churn_builder",
		})
	}

	projection := BuildOperatorProjection(s, 1)
	runtimeEvidence := projection.RuntimeEvidence
	if runtimeEvidence.Status != "running" {
		t.Fatalf("status = %q, want running even when conversation window excludes start", runtimeEvidence.Status)
	}
	if runtimeEvidence.LastRun == nil || runtimeEvidence.LastRun.StartedEventID != started.ID().Value() {
		t.Fatalf("last run = %+v, want anchored start %s", runtimeEvidence.LastRun, started.ID().Value())
	}
	if runtimeEvidence.AgentEvents.Spawned != 1 || runtimeEvidence.AgentEvents.ObservedActive != 1 {
		t.Fatalf("agent events = %+v, want bounded latest agent evidence without evicting run anchor", runtimeEvidence.AgentEvents)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceCombinesQueuedIntentAndRuntimeEvents(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeFactoryRunRequested, FactoryRunRequestedContent{
		RunID:      "run_queued",
		IntakeID:   "intake_001",
		OperatorID: "operator_001",
		Title:      "Queued request",
		Status:     "queued",
		Authority: RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
		},
		Budget: RunLaunchBudget{
			MaxIterations: 0,
			MaxCostUSD:    0,
		},
		TargetRepos:   []string{"transpara-ai/hive"},
		SourceEventID: newTestEventID(t),
		BriefEventID:  newTestEventID(t),
	})
	convID := types.MustConversationID("conv_runtime_evidence_combined")
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     "runtime event",
		RepoPath: "/tmp/runtime",
	})

	projection := BuildOperatorProjection(s, 50)
	runtimeEvidence := projection.RuntimeEvidence
	if runtimeEvidence.LastQueuedRunRequest == nil || runtimeEvidence.LastRun == nil {
		t.Fatalf("runtime evidence = %+v, want queued request and runtime run", runtimeEvidence)
	}
	if runtimeEvidence.LastQueuedRunRequest.RunID != "run_queued" {
		t.Fatalf("queued evidence = %+v", runtimeEvidence.LastQueuedRunRequest)
	}
	if runtimeEvidence.LastQueuedRunRequest.BudgetMaxIterations == nil || *runtimeEvidence.LastQueuedRunRequest.BudgetMaxIterations != 0 {
		t.Fatalf("zero queued budget iterations were not preserved: %+v", runtimeEvidence.LastQueuedRunRequest)
	}
	if runtimeEvidence.LastQueuedRunRequest.BudgetMaxCostUSD == nil || *runtimeEvidence.LastQueuedRunRequest.BudgetMaxCostUSD != 0 {
		t.Fatalf("zero queued budget cost was not preserved: %+v", runtimeEvidence.LastQueuedRunRequest)
	}
	if runtimeEvidence.LastRun.ConversationID != convID.Value() || runtimeEvidence.LastRun.SeedIdea != "runtime event" {
		t.Fatalf("last run evidence = %+v", runtimeEvidence.LastRun)
	}
	if runtimeEvidence.LastQueuedRunRequest.RunID == runtimeEvidence.LastRun.ConversationID {
		t.Fatalf("queued run id was incorrectly joined to runtime conversation: queued=%+v run=%+v", runtimeEvidence.LastQueuedRunRequest, runtimeEvidence.LastRun)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceIgnoresUnrelatedConversationEvents(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	convID := types.MustConversationID("conv_runtime_evidence_progress")
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     "runtime with progress",
		RepoPath: "/tmp/runtime",
	})
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeProgress, ProgressContent{
		Message: "unrelated progress event for runtime evidence",
	})
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_builder",
	})

	projection := BuildOperatorProjection(s, 50)
	if len(projection.Errors) != 0 {
		t.Fatalf("projection errors = %+v, want none for unrelated conversation events", projection.Errors)
	}
	runtimeEvidence := projection.RuntimeEvidence
	if runtimeEvidence.Status != "running" || runtimeEvidence.AgentEvents.Spawned != 1 || runtimeEvidence.AgentEvents.ObservedActive != 1 {
		t.Fatalf("runtime evidence = %+v, want unrelated progress ignored and agent counted", runtimeEvidence)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceKeepsDistinctActiveAgentsWithSameNameRole(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	convID := types.MustConversationID("conv_runtime_evidence_duplicate_agents")
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     "duplicate names",
		RepoPath: "/tmp/runtime",
	})
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_builder_one",
	})
	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_builder_two",
	})

	runningProjection := BuildOperatorProjection(s, 50)
	runningEvidence := runningProjection.RuntimeEvidence
	if runningEvidence.AgentEvents.Spawned != 2 || runningEvidence.AgentEvents.ObservedActive != 2 {
		t.Fatalf("running agent events = %+v, want two distinct active agents", runningEvidence.AgentEvents)
	}

	appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentStopped, AgentStoppedContent{
		Name:       "builder",
		Role:       "implementer",
		StopReason: "complete",
		Iterations: 1,
	})
	stoppedProjection := BuildOperatorProjection(s, 50)
	stoppedEvidence := stoppedProjection.RuntimeEvidence
	if stoppedEvidence.AgentEvents.Spawned != 2 || stoppedEvidence.AgentEvents.Stopped != 1 || stoppedEvidence.AgentEvents.ObservedActive != 1 {
		t.Fatalf("stopped agent events = %+v, want one duplicate-name agent remaining", stoppedEvidence.AgentEvents)
	}
	if len(stoppedEvidence.AgentEvents.ActiveAgents) != 1 || stoppedEvidence.AgentEvents.ActiveAgents[0].ActorID != "actor_builder_one" {
		t.Fatalf("active agents after stop = %+v, want first actor remaining", stoppedEvidence.AgentEvents.ActiveAgents)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceFromRunEvents(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)

	started := appendEvent(EventTypeRunStarted, RunStartedContent{
		Idea:     "prove runtime evidence",
		RepoPath: "/Transpara/transpara-ai/repos/hive",
	})
	spawned := appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_builder",
	})

	runningProjection := BuildOperatorProjection(s, 50)
	runningEvidence := runningProjection.RuntimeEvidence
	if runningEvidence.Status != "running" {
		t.Fatalf("running status = %q, want running", runningEvidence.Status)
	}
	if runningEvidence.LastRun == nil || runningEvidence.LastRun.StartedEventID != started.ID().Value() {
		t.Fatalf("running last run = %+v, want started event %s", runningEvidence.LastRun, started.ID().Value())
	}
	if runningEvidence.LastRun.ConversationID == "" {
		t.Fatalf("running last run conversation ID is empty: %+v", runningEvidence.LastRun)
	}
	if runningEvidence.AgentEvents.Spawned != 1 || runningEvidence.AgentEvents.ObservedActive != 1 {
		t.Fatalf("running agent events = %+v, want one active spawn", runningEvidence.AgentEvents)
	}
	if len(runningEvidence.AgentEvents.ActiveAgents) != 1 {
		t.Fatalf("active agents = %+v, want one agent", runningEvidence.AgentEvents.ActiveAgents)
	}
	active := runningEvidence.AgentEvents.ActiveAgents[0]
	if active.Name != "builder" || active.ActorID != "actor_builder" || active.SpawnedEventID != spawned.ID().Value() {
		t.Fatalf("active agent evidence = %+v", active)
	}

	stopped := appendEvent(EventTypeAgentStopped, AgentStoppedContent{
		Name:       "builder",
		Role:       "implementer",
		StopReason: "complete",
		Iterations: 2,
	})
	completed := appendEvent(EventTypeRunCompleted, RunCompletedContent{
		AgentCount: 1,
		DurationMs: 1234,
		TotalCost:  0.25,
	})

	completedProjection := BuildOperatorProjection(s, 50)
	completedEvidence := completedProjection.RuntimeEvidence
	if completedEvidence.Status != "completed" {
		t.Fatalf("completed status = %q, want completed", completedEvidence.Status)
	}
	if completedEvidence.LastRun == nil {
		t.Fatal("completed last run is nil")
	}
	run := completedEvidence.LastRun
	if run.StartedEventID != started.ID().Value() || run.CompletedEventID != completed.ID().Value() {
		t.Fatalf("run evidence = %+v", run)
	}
	if run.SeedIdea != "prove runtime evidence" || run.RepoPath != "/Transpara/transpara-ai/repos/hive" {
		t.Fatalf("run start metadata = %+v", run)
	}
	if run.CompletedAt == nil || run.AgentCount == nil || *run.AgentCount != 1 || run.DurationMs == nil || *run.DurationMs != 1234 || run.TotalCost == nil || *run.TotalCost != 0.25 {
		t.Fatalf("run completion metadata = %+v", run)
	}
	if completedEvidence.AgentEvents.Spawned != 1 || completedEvidence.AgentEvents.Stopped != 1 || completedEvidence.AgentEvents.ObservedActive != 0 {
		t.Fatalf("completed agent events = %+v", completedEvidence.AgentEvents)
	}
	if completedEvidence.AgentEvents.LastAgentEventID != stopped.ID().Value() {
		t.Fatalf("last agent event = %q, want %q", completedEvidence.AgentEvents.LastAgentEventID, stopped.ID().Value())
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceIncludesArtifactsAndCausalGraph(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	convID := types.MustConversationID("conv_runtime_artifact_graph")

	started := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     "render artifacts",
		RepoPath: "/tmp/runtime",
	})
	spawned := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_builder",
	})
	artifact := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeFactoryArtifactCreated, FactoryArtifactCreatedContent{
		RunID:           "run_artifact_graph",
		ArtifactID:      "artifact_brief_001",
		Label:           "factory_brief",
		Title:           "Factory brief",
		MediaType:       "text/markdown",
		URI:             "eventgraph://artifact/artifact_brief_001",
		Summary:         "brief ready for operator inspection",
		ProducerActorID: "actor_builder",
	})
	authorityRequest := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        newTestEventID(t),
		RequestingActor:  actorID,
		RequestingRole:   "operator",
		ActionName:       "repo.pull_request.create",
		Target:           "transpara-ai/site",
		Environment:      "test",
		RequestedOutcome: "inspect artifact without raw authority payload exposure",
		Justification:    "sensitive rationale must not be exposed through raw run event content",
		RiskSummary:      "raw authority payload exposure",
		Scope:            []string{"repo:transpara-ai/site"},
	})
	runRequest := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeFactoryRunRequested, FactoryRunRequestedContent{
		RunID:      "run_artifact_graph",
		IntakeID:   "intake_artifact_graph",
		OperatorID: "operator_001",
		Title:      "Launch artifact graph",
		Status:     "queued",
		Authority: RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        "operator-launch",
		},
		Budget: RunLaunchBudget{
			MaxIterations: 4,
			MaxCostUSD:    12.5,
		},
		TargetRepos:   []string{"transpara-ai/hive"},
		SourceEventID: newTestEventID(t),
		BriefEventID:  newTestEventID(t),
		Brief:         []byte(`{"goal":"sensitive launch brief"}`),
	})

	projection := BuildOperatorProjection(s, 50)
	runtimeEvidence := projection.RuntimeEvidence
	if runtimeEvidence.LastRun == nil || runtimeEvidence.LastRun.StartedEventID != started.ID().Value() {
		t.Fatalf("last run evidence = %+v, want start %s", runtimeEvidence.LastRun, started.ID().Value())
	}
	if len(runtimeEvidence.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v, want one artifact", runtimeEvidence.Artifacts)
	}
	projectedArtifact := runtimeEvidence.Artifacts[0]
	if projectedArtifact.EventID != artifact.ID().Value() || projectedArtifact.ArtifactID != "artifact_brief_001" {
		t.Fatalf("artifact projection = %+v", projectedArtifact)
	}
	if projectedArtifact.CauseStatus != "caused" {
		t.Fatalf("artifact cause status = %q, want caused", projectedArtifact.CauseStatus)
	}
	if len(projectedArtifact.Causes) != 1 || projectedArtifact.Causes[0].EventID != spawned.ID().Value() || projectedArtifact.Causes[0].EventType != EventTypeAgentSpawned.Value() || projectedArtifact.Causes[0].Scope != "run" {
		t.Fatalf("artifact causes = %+v, want run-scoped spawned event %s", projectedArtifact.Causes, spawned.ID().Value())
	}

	inspectorEvent, ok := findRuntimeEventEvidence(runtimeEvidence.RunEvents, artifact.ID().Value())
	if !ok {
		t.Fatalf("run_events missing artifact inspector payload: %+v", runtimeEvidence.RunEvents)
	}
	if inspectorEvent.InspectorKind != runtimeInspectorKindCuratedEventgraphEvent ||
		!strings.Contains(string(inspectorEvent.Content), `"artifact_id":"artifact_brief_001"`) ||
		!strings.Contains(string(inspectorEvent.Content), `"producer_actor_id":"actor_builder"`) {
		t.Fatalf("artifact inspector event = %+v", inspectorEvent)
	}
	omittedEvent, ok := findRuntimeEventEvidence(runtimeEvidence.RunEvents, authorityRequest.ID().Value())
	if !ok {
		t.Fatalf("run_events missing authority request event: %+v", runtimeEvidence.RunEvents)
	}
	if len(omittedEvent.Content) != 0 || !strings.Contains(omittedEvent.ContentError, "not in the runtime inspector allowlist") {
		t.Fatalf("authority request inspector event = %+v, want curated content omission", omittedEvent)
	}
	omittedRunRequest, ok := findRuntimeEventEvidence(runtimeEvidence.RunEvents, runRequest.ID().Value())
	if !ok {
		t.Fatalf("run_events missing factory run request event: %+v", runtimeEvidence.RunEvents)
	}
	if len(omittedRunRequest.Content) != 0 || !strings.Contains(omittedRunRequest.ContentError, "not in the runtime inspector allowlist") {
		t.Fatalf("factory run request inspector event = %+v, want curated content omission", omittedRunRequest)
	}

	graph := runtimeEvidence.CausalGraph
	if graph.Scope != "latest_run_conversation" || graph.ConversationID != convID.Value() {
		t.Fatalf("causal graph metadata = %+v", graph)
	}
	if len(graph.Nodes) < 3 {
		t.Fatalf("causal graph nodes = %+v, want run start, agent, and artifact nodes", graph.Nodes)
	}
	if !hasRuntimeGraphEdge(graph.Edges, spawned.ID().Value(), artifact.ID().Value(), "run") {
		t.Fatalf("causal graph edges = %+v, want spawned -> artifact edge", graph.Edges)
	}
}

func TestBuildOperatorProjectionRuntimeCausalGraphReportsTruncatedConversation(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	convID := types.MustConversationID("conv_runtime_graph_truncated")
	started := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     "truncate graph",
		RepoPath: "/tmp/runtime",
	})
	latest := started
	for i := 0; i < 3; i++ {
		latest = appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeAgentSpawned, AgentSpawnedContent{
			Name:    "builder",
			Role:    "implementer",
			Model:   "claude-opus-4-6",
			ActorID: "actor_builder",
		})
	}

	projection := BuildOperatorProjection(s, 1)
	graph := projection.RuntimeEvidence.CausalGraph
	if !graph.Truncated {
		t.Fatalf("causal graph truncated = false, want true for limit 1 with extra conversation events: %+v", graph)
	}
	if graph.Limit != 1 {
		t.Fatalf("causal graph limit = %d, want 1", graph.Limit)
	}
	if !hasRuntimeGraphNode(graph.Nodes, started.ID().Value()) || !hasRuntimeGraphNode(graph.Nodes, latest.ID().Value()) {
		t.Fatalf("causal graph nodes = %+v, want anchored start and latest bounded event", graph.Nodes)
	}
}

func TestBuildOperatorProjectionRuntimeArtifactCauseStatusDistinguishesExternalOnly(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	runConvID := types.MustConversationID("conv_runtime_artifact_external")
	externalConvID := types.MustConversationID("conv_external_artifact_cause")

	appendOperatorProjectionEventWithConversation(t, s, actorID, runConvID, EventTypeRunStarted, RunStartedContent{
		Idea:     "render external artifact causes",
		RepoPath: "/tmp/runtime",
	})
	externalCause := appendOperatorProjectionEventWithConversation(t, s, actorID, externalConvID, EventTypeProgress, ProgressContent{
		Message: "external producer signal",
	})
	artifact := appendOperatorProjectionEventWithConversationAndCauses(t, s, actorID, runConvID, EventTypeFactoryArtifactCreated, FactoryArtifactCreatedContent{
		RunID:      "run_external_artifact",
		ArtifactID: "artifact_external_001",
		Label:      "external_artifact",
		MediaType:  "text/plain",
	}, []types.EventID{externalCause.ID()})
	secondArtifact := appendOperatorProjectionEventWithConversationAndCauses(t, s, actorID, runConvID, EventTypeFactoryArtifactCreated, FactoryArtifactCreatedContent{
		RunID:      "run_external_artifact",
		ArtifactID: "artifact_external_002",
		Label:      "second_external_artifact",
		MediaType:  "text/plain",
	}, []types.EventID{externalCause.ID()})

	projection := BuildOperatorProjection(s, 50)
	runtimeEvidence := projection.RuntimeEvidence
	if len(runtimeEvidence.Artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want two artifacts", runtimeEvidence.Artifacts)
	}
	projectedArtifact, ok := findRuntimeArtifactEvidence(runtimeEvidence.Artifacts, artifact.ID().Value())
	if !ok {
		t.Fatalf("artifacts = %+v, want event %s", runtimeEvidence.Artifacts, artifact.ID().Value())
	}
	if projectedArtifact.CauseStatus != "caused_external_only" {
		t.Fatalf("artifact cause status = %q, want caused_external_only", projectedArtifact.CauseStatus)
	}
	if len(projectedArtifact.Causes) != 1 || projectedArtifact.Causes[0].EventID != externalCause.ID().Value() || projectedArtifact.Causes[0].Scope != "external_or_out_of_window" {
		t.Fatalf("artifact causes = %+v, want external-only cause %s", projectedArtifact.Causes, externalCause.ID().Value())
	}
	if !hasRuntimeGraphEdge(runtimeEvidence.CausalGraph.Edges, externalCause.ID().Value(), artifact.ID().Value(), "external_or_out_of_window") {
		t.Fatalf("causal graph edges = %+v, want external cause -> artifact edge", runtimeEvidence.CausalGraph.Edges)
	}
	if !hasRuntimeGraphEdge(runtimeEvidence.CausalGraph.Edges, externalCause.ID().Value(), secondArtifact.ID().Value(), "external_or_out_of_window") {
		t.Fatalf("causal graph edges = %+v, want repeated external cause -> second artifact edge", runtimeEvidence.CausalGraph.Edges)
	}
	for _, edge := range runtimeEvidence.CausalGraph.Edges {
		if edge.FromEventID == externalCause.ID().Value() && edge.Scope != "external_or_out_of_window" {
			t.Fatalf("edge from external cause mislabeled: %+v", edge)
		}
	}
}

func TestBuildOperatorProjectionRuntimeInspectorOmitsOversizedCuratedContent(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	convID := types.MustConversationID("conv_runtime_inspector_oversized")
	started := appendOperatorProjectionEventWithConversation(t, s, actorID, convID, EventTypeRunStarted, RunStartedContent{
		Idea:     strings.Repeat("x", maxOperatorRuntimeInspectorContentBytes+1),
		RepoPath: "/tmp/runtime",
	})

	projection := BuildOperatorProjection(s, 50)
	if len(projection.Errors) != 0 {
		t.Fatalf("projection errors = %+v, want none for intentionally omitted oversized inspector content", projection.Errors)
	}
	inspectorEvent, ok := findRuntimeEventEvidence(projection.RuntimeEvidence.RunEvents, started.ID().Value())
	if !ok {
		t.Fatalf("run_events missing start event: %+v", projection.RuntimeEvidence.RunEvents)
	}
	if len(inspectorEvent.Content) != 0 || !strings.Contains(inspectorEvent.ContentError, "exceeds") {
		t.Fatalf("start inspector event = %+v, want oversized content omitted", inspectorEvent)
	}
}

func TestRuntimeArtifactCauseStatusValues(t *testing.T) {
	if got := runtimeArtifactCauseStatus(nil); got != "missing_causes" {
		t.Fatalf("empty cause status = %q, want missing_causes", got)
	}
	if got := runtimeArtifactCauseStatus([]OperatorRuntimeCauseEvidence{{EventID: "evt_external", Scope: "external_or_out_of_window"}}); got != "caused_external_only" {
		t.Fatalf("external-only cause status = %q, want caused_external_only", got)
	}
	if got := runtimeArtifactCauseStatus([]OperatorRuntimeCauseEvidence{{EventID: "evt_run", Scope: "run"}}); got != "caused" {
		t.Fatalf("run cause status = %q, want caused", got)
	}
}

func TestBuildOperatorProjectionRuntimeEvidenceReanchorsToLatestConversation(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	firstConversation := types.MustConversationID("conv_runtime_evidence_first")
	secondConversation := types.MustConversationID("conv_runtime_evidence_second")

	appendOperatorProjectionEventWithConversation(t, s, actorID, firstConversation, EventTypeRunStarted, RunStartedContent{
		Idea:     "first run",
		RepoPath: "/tmp/first",
	})
	appendOperatorProjectionEventWithConversation(t, s, actorID, firstConversation, EventTypeRunCompleted, RunCompletedContent{
		AgentCount: 2,
		DurationMs: 100,
		TotalCost:  1.5,
	})
	secondStarted := appendOperatorProjectionEventWithConversation(t, s, actorID, secondConversation, EventTypeRunStarted, RunStartedContent{
		Idea:     "second run",
		RepoPath: "/tmp/second",
	})
	secondSpawned := appendOperatorProjectionEventWithConversation(t, s, actorID, secondConversation, EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "second-builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_second_builder",
	})
	appendOperatorProjectionEventWithConversation(t, s, actorID, firstConversation, EventTypeRunCompleted, RunCompletedContent{
		AgentCount: 9,
		DurationMs: 999,
		TotalCost:  9.99,
	})

	runningProjection := BuildOperatorProjection(s, 50)
	runningEvidence := runningProjection.RuntimeEvidence
	if runningEvidence.Status != "running" {
		t.Fatalf("status after stale completion = %q, want running", runningEvidence.Status)
	}
	if runningEvidence.LastRun == nil {
		t.Fatal("last run is nil")
	}
	if runningEvidence.LastRun.StartedEventID != secondStarted.ID().Value() || runningEvidence.LastRun.ConversationID != secondConversation.Value() {
		t.Fatalf("latest run evidence = %+v, want second conversation", runningEvidence.LastRun)
	}
	if runningEvidence.LastRun.CompletedEventID != "" || runningEvidence.LastRun.AgentCount != nil {
		t.Fatalf("stale completion was attached to latest run: %+v", runningEvidence.LastRun)
	}
	if runningEvidence.AgentEvents.Spawned != 1 || runningEvidence.AgentEvents.ObservedActive != 1 {
		t.Fatalf("running agent events = %+v, want one active second-run spawn", runningEvidence.AgentEvents)
	}
	if len(runningEvidence.AgentEvents.ActiveAgents) != 1 || runningEvidence.AgentEvents.ActiveAgents[0].SpawnedEventID != secondSpawned.ID().Value() {
		t.Fatalf("active agents = %+v, want second-run agent", runningEvidence.AgentEvents.ActiveAgents)
	}
	if runningEvidence.AgentEvents.LastAgentEventID != secondSpawned.ID().Value() {
		t.Fatalf("last agent event = %q, want second-run spawn %q", runningEvidence.AgentEvents.LastAgentEventID, secondSpawned.ID().Value())
	}

	appendOperatorProjectionEventWithConversation(t, s, actorID, secondConversation, EventTypeAgentStopped, AgentStoppedContent{
		Name:       "second-builder",
		Role:       "implementer",
		StopReason: "complete",
		Iterations: 1,
	})
	secondCompleted := appendOperatorProjectionEventWithConversation(t, s, actorID, secondConversation, EventTypeRunCompleted, RunCompletedContent{
		AgentCount: 1,
		DurationMs: 200,
		TotalCost:  0,
	})

	completedProjection := BuildOperatorProjection(s, 50)
	completedEvidence := completedProjection.RuntimeEvidence
	if completedEvidence.Status != "completed" {
		t.Fatalf("status = %q, want completed", completedEvidence.Status)
	}
	if completedEvidence.LastRun == nil || completedEvidence.LastRun.CompletedEventID != secondCompleted.ID().Value() {
		t.Fatalf("completed latest run evidence = %+v", completedEvidence.LastRun)
	}
	if completedEvidence.LastRun.TotalCost == nil || *completedEvidence.LastRun.TotalCost != 0 {
		t.Fatalf("zero total cost was not preserved as observed evidence: %+v", completedEvidence.LastRun)
	}
	if completedEvidence.AgentEvents.Spawned != 1 || completedEvidence.AgentEvents.Stopped != 1 || completedEvidence.AgentEvents.ObservedActive != 0 {
		t.Fatalf("completed agent events = %+v", completedEvidence.AgentEvents)
	}

	appendOperatorProjectionEventWithConversation(t, s, actorID, secondConversation, EventTypeAgentSpawned, AgentSpawnedContent{
		Name:    "late-builder",
		Role:    "implementer",
		Model:   "claude-opus-4-6",
		ActorID: "actor_late_builder",
	})
	afterLateSpawnProjection := BuildOperatorProjection(s, 50)
	afterLateSpawnEvidence := afterLateSpawnProjection.RuntimeEvidence
	if afterLateSpawnEvidence.Status != "completed" {
		t.Fatalf("status after late same-conversation spawn = %q, want completed", afterLateSpawnEvidence.Status)
	}
	if afterLateSpawnEvidence.AgentEvents.ObservedActive != 0 || len(afterLateSpawnEvidence.AgentEvents.ActiveAgents) != 0 {
		t.Fatalf("late spawn reopened active state: %+v", afterLateSpawnEvidence.AgentEvents)
	}
}

func TestBuildOperatorProjectionIncludesStaticModelSelection(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)

	projection := BuildOperatorProjection(s, 50)
	models := projection.ModelSelection

	if models.Source != "hive" {
		t.Fatalf("model selection source = %q, want hive", models.Source)
	}
	if models.CatalogSource != operatorModelCatalogSourceEmbedded {
		t.Fatalf("catalog source = %q, want %q", models.CatalogSource, operatorModelCatalogSourceEmbedded)
	}
	if models.ReloadMode != operatorModelCatalogReloadMode || models.HotReload {
		t.Fatalf("reload metadata = mode %q hot_reload %v, want startup-static false", models.ReloadMode, models.HotReload)
	}
	if models.LoadedAt.IsZero() {
		t.Fatal("model catalog loaded_at is zero")
	}
	if len(models.Models) == 0 {
		t.Fatal("expected projected model catalog entries")
	}
	if len(models.Errors) != 0 {
		t.Fatalf("default role assignments produced errors: %+v", models.Errors)
	}

	implementer := requireModelAssignment(t, models, "implementer")
	if !implementer.CanOperate {
		t.Fatalf("implementer can_operate = false, want true")
	}
	if implementer.Provider != "claude-cli" || implementer.AuthMode != string(modelconfig.AuthSubscription) {
		t.Fatalf("implementer assignment = %+v, want claude-cli subscription", implementer)
	}
	if !containsModelProjectionString(implementer.RequiredCapabilities, string(modelconfig.CapOperate)) {
		t.Fatalf("implementer required capabilities = %+v, want operate", implementer.RequiredCapabilities)
	}

	for _, assignment := range models.Assignments {
		if assignment.Model == "" {
			t.Fatalf("empty model assignment for %s: %+v", assignment.Role, assignment)
		}
		if assignment.AuthMode != string(modelconfig.AuthSubscription) {
			t.Fatalf("default role %s auth_mode = %q, want subscription (no silent API-key default): %+v", assignment.Role, assignment.AuthMode, assignment)
		}
	}
}

func TestBuildOperatorProjectionModelSelectionRequiresExplicitAPIKeyOptIn(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)

	defaultProjection := BuildOperatorProjection(s, 50)
	if got := requireModelAssignment(t, defaultProjection.ModelSelection, "guardian").AuthMode; got != string(modelconfig.AuthSubscription) {
		t.Fatalf("default guardian auth_mode = %q, want subscription", got)
	}

	config := testModelSelectionConfigWithRoleDefault("guardian", "api-sonnet")
	projection := BuildOperatorProjection(s, 50, WithOperatorModelSelection(config))
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.Model != "api-claude-sonnet-4-6" || guardian.Provider != "anthropic" || guardian.AuthMode != string(modelconfig.AuthAPIKey) {
		t.Fatalf("guardian explicit API-key opt-in assignment = %+v", guardian)
	}

	implementer := requireModelAssignment(t, projection.ModelSelection, "implementer")
	if implementer.Provider != "claude-cli" || implementer.AuthMode != string(modelconfig.AuthSubscription) {
		t.Fatalf("explicit guardian API-key opt-in leaked to implementer: %+v", implementer)
	}
}

func TestOperatorModelSelectionManagerReloadUpdatesProjection(t *testing.T) {
	catalogPath := writeRoleDefaultCatalog(t, "guardian", "api-sonnet")
	manager, err := NewOperatorModelSelectionManager(catalogPath, time.Unix(1_700_000_000, 0).UTC(), true)
	if err != nil {
		t.Fatalf("NewOperatorModelSelectionManager: %v", err)
	}
	first := BuildOperatorModelSelection(manager.Snapshot())
	firstGuardian := requireModelAssignment(t, first, "guardian")
	if first.ReloadMode != operatorModelCatalogReloadModeHot || !first.HotReload {
		t.Fatalf("reload metadata = %q/%v, want hot-reload true", first.ReloadMode, first.HotReload)
	}
	if firstGuardian.AuthMode != string(modelconfig.AuthAPIKey) {
		t.Fatalf("initial guardian auth = %q, want api-key", firstGuardian.AuthMode)
	}

	writeRoleDefaultCatalogAt(t, catalogPath, "guardian", "haiku")
	if err := manager.Reload(time.Unix(1_700_000_100, 0).UTC()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	reloaded := BuildOperatorModelSelection(manager.Snapshot())
	reloadedGuardian := requireModelAssignment(t, reloaded, "guardian")
	if reloaded.LastReloadAt == nil {
		t.Fatal("last_reload_at is nil after reload")
	}
	if reloadedGuardian.AuthMode != string(modelconfig.AuthSubscription) {
		t.Fatalf("reloaded guardian auth = %q, want subscription", reloadedGuardian.AuthMode)
	}
	if reloadedGuardian.Model == firstGuardian.Model {
		t.Fatalf("guardian model did not change after reload: %q", reloadedGuardian.Model)
	}
}

func TestOperatorModelSelectionManagerReloadFailureIsVisibleAndKeepsPreviousResolver(t *testing.T) {
	catalogPath := writeRoleDefaultCatalog(t, "guardian", "api-sonnet")
	manager, err := NewOperatorModelSelectionManager(catalogPath, time.Unix(1_700_000_000, 0).UTC(), true)
	if err != nil {
		t.Fatalf("NewOperatorModelSelectionManager: %v", err)
	}
	before := requireModelAssignment(t, BuildOperatorModelSelection(manager.Snapshot()), "guardian")
	if before.AuthMode != string(modelconfig.AuthAPIKey) {
		t.Fatalf("initial guardian auth = %q, want api-key", before.AuthMode)
	}

	if err := os.WriteFile(catalogPath, []byte("models: [\n"), 0o644); err != nil {
		t.Fatalf("write invalid catalog: %v", err)
	}
	if err := manager.Reload(time.Unix(1_700_000_100, 0).UTC()); err == nil {
		t.Fatal("Reload invalid catalog succeeded, want error")
	}
	afterProjection := BuildOperatorModelSelection(manager.Snapshot())
	after := requireModelAssignment(t, afterProjection, "guardian")
	if after.AuthMode != string(modelconfig.AuthAPIKey) || after.Model != before.Model {
		t.Fatalf("reload failure corrupted active assignment: before=%+v after=%+v", before, after)
	}
	if len(afterProjection.Errors) == 0 || !strings.Contains(afterProjection.Errors[0], "catalog reload") {
		t.Fatalf("projection errors = %+v, want catalog reload error", afterProjection.Errors)
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

func appendOpaqueSharedStoreEvent(t *testing.T, s *store.InMemoryStore, actorID types.ActorID) event.Event {
	t.Helper()
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	eventType := types.MustEventType("foreign.event.type")
	content := event.RawContent{
		TypeName: eventType.Value(),
		Data:     json.RawMessage(`{"source":"shared-store"}`),
	}
	eventID := newTestEventID(t)
	convID := types.MustConversationID("conv_00000000000000000000000000000088")
	causes := []types.EventID{head.Unwrap().ID()}
	prevHash := head.Unwrap().Hash()
	tmp := event.NewEvent(event.CurrentEventVersion, eventID, eventType, types.Now(), actorID, content, causes, convID, types.ZeroHash(), prevHash, types.Signature{})
	hash, err := event.ComputeHash(event.CanonicalForm(tmp))
	if err != nil {
		t.Fatalf("hash opaque event: %v", err)
	}
	ev := event.NewEvent(event.CurrentEventVersion, eventID, eventType, tmp.Timestamp(), actorID, content, causes, convID, hash, prevHash, types.Signature{})
	stored, err := s.Append(ev)
	if err != nil {
		t.Fatalf("append opaque event: %v", err)
	}
	return stored
}

func appendOperatorProjectionEventWithConversation(t *testing.T, s *store.InMemoryStore, actorID types.ActorID, convID types.ConversationID, eventType types.EventType, content event.EventContent) event.Event {
	t.Helper()
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	return appendOperatorProjectionEventWithConversationAndCauses(t, s, actorID, convID, eventType, content, []types.EventID{head.Unwrap().ID()})
}

func appendOperatorProjectionEventWithConversationAndCauses(t *testing.T, s *store.InMemoryStore, actorID types.ActorID, convID types.ConversationID, eventType types.EventType, content event.EventContent, causes []types.EventID) event.Event {
	t.Helper()
	RegisterEventTypes()
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)
	signer := deriveSignerFromID(actorID)

	ev, err := factory.Create(eventType, actorID, content, causes, convID, s, signer)
	if err != nil {
		t.Fatalf("create %s: %v", eventType.Value(), err)
	}
	stored, err := s.Append(ev)
	if err != nil {
		t.Fatalf("append %s: %v", eventType.Value(), err)
	}
	return stored
}

func testModelSelectionConfigWithRoleDefault(role, model string) OperatorModelSelectionConfig {
	base := modelconfig.DefaultResolver()
	defaults := base.Defaults()
	tierModels := make(map[modelconfig.ModelTier]string, len(defaults.TierModels))
	for tier, model := range defaults.TierModels {
		tierModels[tier] = model
	}
	roleModels := make(map[string]string, len(defaults.RoleModels)+1)
	for role, model := range defaults.RoleModels {
		roleModels[role] = model
	}
	roleModels[role] = model
	defaults.TierModels = tierModels
	defaults.RoleModels = roleModels
	return OperatorModelSelectionConfig{
		Resolver:      modelconfig.NewResolver(base.Catalog(), nil, defaults),
		CatalogSource: "test-explicit-role-default",
		LoadedAt:      time.Unix(1_700_000_000, 0).UTC(),
		ReloadMode:    operatorModelCatalogReloadMode,
		HotReload:     false,
	}
}

func requireModelAssignment(t *testing.T, projection OperatorModelSelection, role string) OperatorModelRoleAssignment {
	t.Helper()
	for _, assignment := range projection.Assignments {
		if assignment.Role == role {
			return assignment
		}
	}
	t.Fatalf("missing model assignment for role %q in %+v", role, projection.Assignments)
	return OperatorModelRoleAssignment{}
}

func containsModelProjectionString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func findRuntimeEventEvidence(events []OperatorRuntimeEventEvidence, eventID string) (OperatorRuntimeEventEvidence, bool) {
	for _, ev := range events {
		if ev.EventID == eventID {
			return ev, true
		}
	}
	return OperatorRuntimeEventEvidence{}, false
}

func findRuntimeArtifactEvidence(artifacts []OperatorRuntimeArtifactEvidence, eventID string) (OperatorRuntimeArtifactEvidence, bool) {
	for _, artifact := range artifacts {
		if artifact.EventID == eventID {
			return artifact, true
		}
	}
	return OperatorRuntimeArtifactEvidence{}, false
}

func hasRuntimeGraphEdge(edges []OperatorRuntimeCausalEdge, from, to, scope string) bool {
	for _, edge := range edges {
		if edge.FromEventID == from && edge.ToEventID == to && edge.Scope == scope {
			return true
		}
	}
	return false
}

func hasRuntimeGraphNode(nodes []OperatorRuntimeCausalNode, eventID string) bool {
	for _, node := range nodes {
		if node.EventID == eventID {
			return true
		}
	}
	return false
}

func writeRoleDefaultCatalog(t *testing.T, role, model string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.yaml")
	writeRoleDefaultCatalogAt(t, path, role, model)
	return path
}

func writeRoleDefaultCatalogAt(t *testing.T, path, role, model string) {
	t.Helper()
	body := "role_defaults:\n  " + role + ": " + model + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}
