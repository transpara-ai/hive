package hive

import (
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
		RepoPath: "/Transpara/transpara-ai/data/repos/hive",
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
	if run.SeedIdea != "prove runtime evidence" || run.RepoPath != "/Transpara/transpara-ai/data/repos/hive" {
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

func appendOperatorProjectionEventWithConversation(t *testing.T, s *store.InMemoryStore, actorID types.ActorID, convID types.ConversationID, eventType types.EventType, content event.EventContent) event.Event {
	t.Helper()
	RegisterEventTypes()
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)
	signer := deriveSignerFromID(actorID)

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
