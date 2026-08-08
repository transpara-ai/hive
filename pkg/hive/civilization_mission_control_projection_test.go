package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	"github.com/transpara-ai/work"
)

type missionRuntimeSourceFunc func(context.Context, time.Time, *FactoryRuntimeSnapshot) (FactoryRuntimeSnapshot, error)

func (f missionRuntimeSourceFunc) Fetch(ctx context.Context, now time.Time, previous *FactoryRuntimeSnapshot) (FactoryRuntimeSnapshot, error) {
	return f(ctx, now, previous)
}

func TestHIVEMCT1ClassificationMatrixFailsUpwardWithoutExactBoundEvidence(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	base := factoryv1.OrderProjection{
		OrderID: "FO-MC-1", Version: "1.2.3", DocumentSHA256: strings.Repeat("d", 64),
		EngineProtocol: factoryv1.TLCVersion, LastEffectAt: now.Add(-time.Minute),
	}
	exact := func(profile string, tier int) factoryv1.Evidence {
		metadata := map[string]string{
			"protocol": "4.5.0", "profile": profile, "tier": string(rune('0' + tier)),
			"order_id": base.OrderID, "order_version": base.Version, "document_sha256": base.DocumentSHA256,
		}
		if profile != "P-ENVELOPE" {
			metadata["classification_result_blob"] = strings.Repeat("a", 40)
			metadata["inventory_blob"] = strings.Repeat("b", 40)
		}
		return factoryv1.Evidence{Kind: "tlc_change_classification", Reference: strings.Repeat("c", 40), Metadata: metadata}
	}
	for _, profile := range []string{"P-MECHANICAL", "P-IMPLEMENTATION", "P-DESIGN-DELTA", "P-ENVELOPE"} {
		for tier := 0; tier <= 3; tier++ {
			profile, tier := profile, tier
			t.Run(profile+"-tier-"+string(rune('0'+tier)), func(t *testing.T) {
				order := base
				order.Stages = []factoryv1.StageLedgerProjection{{Stage: factoryv1.StageDesign, Evidence: []factoryv1.Evidence{exact(profile, tier), exact(profile, tier)}}}
				got := classifyMissionOrder(order, now)
				if got.DeclaredGovernanceProtocol != "4.5.0" || got.DeclaredPacketProfile != profile || got.DeclaredHumanReviewTier == nil || *got.DeclaredHumanReviewTier != tier || got.EffectivePacketProfile != profile || got.EffectiveHumanReviewTier != tier || got.Mark.State != StateCurrent {
					t.Fatalf("exact classification = %+v", got)
				}
			})
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*factoryv1.OrderProjection)
	}{
		{name: "missing", mutate: func(*factoryv1.OrderProjection) {}},
		{name: "wrong-subject", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			ev.Metadata["order_id"] = "FO-OTHER"
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "wrong-version", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			ev.Metadata["order_version"] = "9.9.9"
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "wrong-document", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			ev.Metadata["document_sha256"] = strings.Repeat("e", 64)
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "invalid-evidence-blob", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			ev.Reference = "main"
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "missing-inventory", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			delete(ev.Metadata, "inventory_blob")
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "missing-classifier", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-IMPLEMENTATION", 1)
			delete(ev.Metadata, "classification_result_blob")
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "contradictory", mutate: func(order *factoryv1.OrderProjection) {
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{exact("P-MECHANICAL", 0), exact("P-ENVELOPE", 3)}}}
		}},
		{name: "future-protocol", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			ev.Metadata["protocol"] = "4.6.0"
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "legacy-protocol", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-MECHANICAL", 0)
			ev.Metadata["protocol"] = "tlc-v1"
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "unknown-profile", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-UNKNOWN", 0)
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
		{name: "invalid-tier", mutate: func(order *factoryv1.OrderProjection) {
			ev := exact("P-ENVELOPE", 3)
			ev.Metadata["tier"] = "4"
			order.Stages = []factoryv1.StageLedgerProjection{{Evidence: []factoryv1.Evidence{ev}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := base
			tc.mutate(&order)
			got := classifyMissionOrder(order, now)
			if got.EffectiveGovernanceProtocol != "4.5.0" || got.EffectivePacketProfile != "P-ENVELOPE" || got.EffectiveHumanReviewTier != 3 || got.Mark.State != StateInferred {
				t.Fatalf("fail-up classification = %+v", got)
			}
		})
	}

	lowerTier := base
	lowerTier.TLCStage = factoryv1.StageHumanReview
	lowerTier.Stages = []factoryv1.StageLedgerProjection{{Stage: factoryv1.StageDesign, Evidence: []factoryv1.Evidence{exact("P-MECHANICAL", 1)}}}
	if rollup := missionFactoryEvidenceRollup(lowerTier, now); rollup.PendingTier3HumanReview {
		t.Fatalf("exact lower-tier evidence was mislabeled as pending Tier 3: %+v", rollup)
	}
	failUpTier := base
	failUpTier.TLCStage = factoryv1.StageHumanReview
	if rollup := missionFactoryEvidenceRollup(failUpTier, now); !rollup.PendingTier3HumanReview {
		t.Fatalf("missing classification evidence did not fail upward to pending Tier 3: %+v", rollup)
	}
}

type missionRuntimeFixture struct {
	clock    *missionControlClock
	mu       sync.Mutex
	sequence uint64
}

func (r *missionRuntimeFixture) Fetch(_ context.Context, now time.Time, _ *FactoryRuntimeSnapshot) (FactoryRuntimeSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	return FactoryRuntimeSnapshot{
		SchemaVersion: FactoryRuntimeSnapshotSchemaVersion, DaemonInstanceID: "daemon-test", BootID: "boot-test", RecoveryGeneration: 1, Sequence: r.sequence,
		ProcessStartedAt: now.Add(-time.Hour), ObservedAt: now, LastHeartbeatAt: now, LastSchedulerProgressAt: now,
		SchedulerState: FactoryRuntimePolling, ConfiguredWorkers: 3, ActiveWorkers: 0, AvailableWorkers: 3, Assignments: []factoryv1.RuntimeAssignment{},
	}, nil
}

type missionFailStore struct {
	store.Store
	mu        sync.Mutex
	fail      bool
	eventType types.EventType
}

type missionNoAdvanceStore struct {
	store.Store
	eventType types.EventType
}

func (s *missionNoAdvanceStore) ByType(eventType types.EventType, _ int, _ types.Option[types.Cursor]) (types.Page[event.Event], error) {
	if eventType == s.eventType {
		return types.NewPage([]event.Event{}, types.Some(types.MustCursor("stuck")), true), nil
	}
	return s.Store.ByType(eventType, 50, types.None[types.Cursor]())
}

type missionDecodeFaultStore struct {
	store.Store
	eventType   types.EventType
	replacement event.Event
}

func (s *missionDecodeFaultStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	if eventType == s.eventType {
		return types.NewPage([]event.Event{s.replacement}, types.None[types.Cursor](), false), nil
	}
	return s.Store.ByType(eventType, limit, after)
}

type missionHeadSkewStore struct {
	store.Store
	eventType types.EventType
	once      sync.Once
	mutate    func()
}

func (s *missionHeadSkewStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	if eventType == s.eventType {
		s.once.Do(s.mutate)
	}
	return s.Store.ByType(eventType, limit, after)
}

func TestHIVEMCT4RoleAgentJoinAndCompletenessFailures(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)
	now := time.Now().UTC().Add(time.Hour)
	identities := make([]struct {
		event event.Event
		body  AgentIdentityRegisteredContent
	}, 0, 55)
	for i := 0; i < 55; i++ {
		role := fmt.Sprintf("unconfigured-%02d", i)
		if i == 0 {
			role = "guardian"
		} else if i == 1 {
			role = "sysmon"
		}
		body := AgentIdentityRegisteredContent{
			ActorID: types.MustActorID(fmt.Sprintf("actor_%032x", i+100)), DisplayName: fmt.Sprintf("Agent %02d", i), Role: role,
			PublicKey: types.MustPublicKey(make([]byte, 32)), KeyProvenance: "test", Environment: "review", IdentityMode: "persistent",
			LifecycleStatus: "active", AuthorityScope: "review-only", RegistrationPath: "test",
		}
		identities = append(identities, struct {
			event event.Event
			body  AgentIdentityRegisteredContent
		}{event: appendEvent(EventTypeAgentIdentityRegistered, body), body: body})
	}
	guardianSpawn := appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{Name: "guardian-1", Role: "guardian", Model: "gpt-5.5", ActorID: identities[0].body.ActorID.Value()})
	appendEvent(EventTypeAgentStopped, AgentStoppedContent{Name: "guardian-1", Role: "guardian", StopReason: "test stop"})
	appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{Name: "sysmon-1", Role: "sysmon", Model: "claude-sonnet-4-6", ActorID: identities[1].body.ActorID.Value()})

	modelSource := func() OperatorModelSelectionConfig {
		config := DefaultOperatorModelSelectionConfig(now.Add(-time.Minute))
		config.RolePolicies = map[string]OperatorModelRolePolicy{
			"guardian": {Policy: &modelconfig.RoleModelPolicy{Model: "gpt-5.5"}},
			"sysmon":   {Policy: &modelconfig.RoleModelPolicy{Model: "claude-sonnet-4-6"}},
		}
		return config
	}
	runtimeSource := missionRuntimeSourceFunc(func(_ context.Context, observed time.Time, _ *FactoryRuntimeSnapshot) (FactoryRuntimeSnapshot, error) {
		return FactoryRuntimeSnapshot{
			SchemaVersion: FactoryRuntimeSnapshotSchemaVersion, DaemonInstanceID: "daemon-t4", BootID: "boot-t4", RecoveryGeneration: 1, Sequence: 10,
			ProcessStartedAt: observed.Add(-time.Hour), ObservedAt: observed, LastHeartbeatAt: observed, LastSchedulerProgressAt: observed,
			SchedulerState: FactoryRuntimeExecuting, ConfiguredWorkers: 3, ActiveWorkers: 1, AvailableWorkers: 2, QueuedOrders: 1, SchedulableOrders: 1,
			Assignments: []factoryv1.RuntimeAssignment{{OrderID: "FO-RUNTIME", OrderVersion: "1.0.0", DocumentSHA256: strings.Repeat("a", 64), Stage: factoryv1.StageWriteCode, AttemptID: "attempt-runtime", ProviderID: "codex-cli", ModelID: "gpt-5.5", AssignedAt: observed.Add(-time.Minute)}},
		}, nil
	})
	projector, err := NewCivilizationMissionControlProjector(s, MissionControlProjectorConfig{
		FactoryProjection: func(context.Context) (factoryv1.Projection, error) {
			return factoryv1.Projection{SchemaVersion: factoryv1.SchemaVersion, GeneratedAt: now, Service: factoryv1.ServiceProjection{ServiceID: "factory-v1", InstanceID: "test", StartedAt: now.Add(-time.Hour), Healthy: true}}, nil
		},
		ModelSelection: modelSource, Runtime: runtimeSource, Clock: &missionControlClock{now: now}, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := projector.Build(context.Background())
	if !projection.Completeness.Complete {
		t.Fatalf("HIVE-MC-T4 projection incomplete: %+v", projection.Completeness.Reasons)
	}
	if projection.Sources[1].Completeness.DomainCounts[EventTypeAgentIdentityRegistered.Value()] != 55 || projection.Sources[1].Completeness.PageCounts[EventTypeAgentIdentityRegistered.Value()] < 6 {
		t.Fatalf("HIVE-MC-T4 roster pagination = %+v", projection.Sources[1].Completeness)
	}
	rows := map[string]RoleAgentRow{}
	for _, row := range projection.Roles {
		rows[row.StableID] = row
	}
	guardian := rows["agent:"+identities[0].body.ActorID.Value()]
	sysmon := rows["agent:"+identities[1].body.ActorID.Value()]
	configuredOnly := rows["role:allocator"]
	runtimeRow := rows["runtime:boot-t4:FO-RUNTIME"]
	if guardian.EventActive.Value != false || guardian.Running.Mark.State != StateUnavailable || guardian.Provider.Value != "codex-cli" || guardian.Status.Value != "active" || guardian.Authority.Value != "review-only" {
		t.Fatalf("instantiated-stopped guardian = %+v", guardian)
	}
	if sysmon.EventActive.Value != true || sysmon.Running.Mark.State != StateUnavailable || sysmon.Provider.Value != "claude-cli" || sysmon.Capacity.Mark.State != StateProjectedOnly {
		t.Fatalf("event-active sysmon = %+v", sysmon)
	}
	if configuredOnly.Configured.Value != true || configuredOnly.Instantiated.Value != 0 || configuredOnly.Running.Mark.State != StateUnavailable {
		t.Fatalf("configured-only role = %+v", configuredOnly)
	}
	if runtimeRow.Running.Value != true || runtimeRow.Running.Mark.State != StateProjectedOnly || runtimeRow.Provider.Value != "codex-cli" || runtimeRow.Model.Value != "gpt-5.5" || runtimeRow.Authority.Mark.State != StateUnavailable || runtimeRow.Capacity.Value != 1 {
		t.Fatalf("Factory runtime row = %+v", runtimeRow)
	}
	if projection.WorkerPool.ActiveWorkers.Value != 1 || projection.WorkerPool.Mark.State != StateProjectedOnly {
		t.Fatalf("worker pool evidence = %+v", projection.WorkerPool)
	}

	fold := &missionIdentityFold{registered: identities[0].event, identity: identities[0].body, status: "active", statusEvent: identities[0].event, authority: "review-only", authorityEvent: identities[0].event, spawn: &guardianSpawn, spawnContent: guardianSpawn.Content().(AgentSpawnedContent), latestAt: guardianSpawn.Timestamp().Value()}
	ambiguous, _ := missionIdentityRow(fold, map[string]OperatorModelRoleAssignment{}, map[string][]OperatorModelCatalogEntry{"gpt-5.5": {{ID: "one", Provider: "codex-cli"}, {ID: "two", Provider: "other"}}}, map[string]event.Event{}, now)
	if ambiguous.Provider.Mark.State != StateUnavailable || ambiguous.Capacity.Mark.State != StateUnavailable {
		t.Fatalf("ambiguous catalog match gained provider/capacity: %+v", ambiguous)
	}

	if _, _, err := missionEventsByType(&missionNoAdvanceStore{Store: s, eventType: EventTypeAgentIdentityRegistered}, EventTypeAgentIdentityRegistered, 10); err == nil || !strings.Contains(err.Error(), "no advancing cursor") {
		t.Fatalf("non-advancing cursor accepted: %v", err)
	}
	decodeProjector, _ := NewCivilizationMissionControlProjector(&missionDecodeFaultStore{Store: s, eventType: EventTypeAgentIdentityRegistered, replacement: guardianSpawn}, MissionControlProjectorConfig{Clock: &missionControlClock{now: now}, PageSize: 10})
	decodeRoster, _ := decodeProjector.buildRosterSource(context.Background(), now)
	if decodeRoster.Completeness.Complete || !strings.Contains(strings.Join(decodeRoster.Completeness.Reasons, ";"), "decode") {
		t.Fatalf("roster decode failure did not fail closed: %+v", decodeRoster.Completeness)
	}
	skewedStore := &missionHeadSkewStore{Store: s, eventType: EventTypeAgentIdentityRegistered, mutate: func() { appendEvent(EventTypeProgress, ProgressContent{Message: "head skew"}) }}
	skewProjector, _ := NewCivilizationMissionControlProjector(skewedStore, MissionControlProjectorConfig{ModelSelection: modelSource, Clock: &missionControlClock{now: now}, PageSize: 10})
	skewRoster, _ := skewProjector.buildRosterSource(context.Background(), now)
	if skewRoster.Completeness.Complete || skewRoster.Rows[0].Mark.State != StateProjectedOnly || !strings.Contains(strings.Join(skewRoster.Completeness.Reasons, ";"), "head changed") {
		t.Fatalf("roster head skew did not fail closed: %+v row=%+v", skewRoster.Completeness, skewRoster.Rows[0])
	}
}

func (s *missionFailStore) setFail(value bool) { s.mu.Lock(); s.fail = value; s.mu.Unlock() }

func (s *missionFailStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	s.mu.Lock()
	fail := s.fail
	s.mu.Unlock()
	target := s.eventType
	if target.Value() == "" {
		target = work.EventTypeTaskCreated
	}
	if fail && eventType == target {
		return types.Page[event.Event]{}, errors.New("injected source read failure")
	}
	return s.Store.ByType(eventType, limit, after)
}

func TestHIVEMCT5AuthorityPaginationAndDecisionConflicts(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	now := time.Now().UTC().Add(time.Hour)
	requestIDs := make([]types.EventID, 0, 55)
	for i := 0; i < 55; i++ {
		requestID := newTestEventID(t)
		requestIDs = append(requestIDs, requestID)
		appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
			RequestID: requestID, RequestingActor: actorID, RequestingRole: "operator", ActionName: "protected.action", Target: fmt.Sprintf("target-%02d", i),
			Environment: "review", RiskClass: "high", RequestedOutcome: "bounded change", Justification: "test", RiskSummary: "protected",
		})
	}
	appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{DecisionID: "decision-single", RequestID: requestIDs[1], ApproverActor: actorID, DeciderRole: "human", Outcome: "approved", ApprovedTarget: "target-01", ApprovedAction: "protected.action"})
	appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{DecisionID: "decision-conflict-a", RequestID: requestIDs[2], ApproverActor: actorID, DeciderRole: "human", Outcome: "approved", ApprovedTarget: "target-02", ApprovedAction: "protected.action"})
	appendEvent(EventTypeAuthorityDecisionRecorded, AuthorityDecisionRecordedContent{DecisionID: "decision-conflict-b", RequestID: requestIDs[2], ApproverActor: actorID, DeciderRole: "human", Outcome: "denied", ApprovedTarget: "target-02", ApprovedAction: "protected.action"})

	projector, err := NewCivilizationMissionControlProjector(s, MissionControlProjectorConfig{Clock: &missionControlClock{now: now}, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := projector.buildAuthoritySource(context.Background(), now)
	if err != nil || !authority.Completeness.Complete {
		t.Fatalf("authority projection = %v %+v", err, authority.Completeness)
	}
	if authority.Completeness.DomainCounts[EventTypeAuthorityRequestRecorded.Value()] != 55 || authority.Completeness.PageCounts[EventTypeAuthorityRequestRecorded.Value()] < 6 || len(authority.HumanActions) != 54 {
		t.Fatalf("authority exhaustive pagination/actions = %+v actions=%d", authority.Completeness, len(authority.HumanActions))
	}
	conflict := false
	for _, action := range authority.HumanActions {
		if action.SubjectID == requestIDs[1].Value() {
			t.Error("single valid authority decision remained pending")
		}
		if action.SubjectID == requestIDs[2].Value() {
			conflict = action.Kind == "authority_decision_conflict" && action.Severity == "critical" && action.Mark.State == StateProjectedOnly
		}
	}
	if !conflict {
		t.Fatal("conflicting authority decisions did not produce a critical projected-only Human action")
	}

	failing := &missionFailStore{Store: s, fail: true, eventType: EventTypeAuthorityRequestRecorded}
	failedProjector, _ := NewCivilizationMissionControlProjector(failing, MissionControlProjectorConfig{Clock: &missionControlClock{now: now}, PageSize: 10})
	failed, _ := failedProjector.buildAuthoritySource(context.Background(), now)
	if failed.Completeness.Complete || !strings.Contains(strings.Join(failed.Completeness.Reasons, ";"), "read authority requests") {
		t.Fatalf("authority read failure did not fail closed: %+v", failed.Completeness)
	}
}

func TestHIVEMCT5CompleteWIPPaginationStaleExpiryAndEndpoint(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	for i := 0; i < 5; i++ {
		appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{Title: "independent task", CreatedBy: actorID})
	}
	appendEvent(EventTypeAgentIdentityRegistered, AgentIdentityRegisteredContent{
		ActorID: actorID, DisplayName: "Reviewer", Role: "reviewer", PublicKey: types.MustPublicKey(make([]byte, 32)),
		KeyProvenance: "generated", Environment: "review", IdentityMode: "persistent", LifecycleStatus: "active", AuthorityScope: "review-only", RegistrationPath: "generated",
	})
	requestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{RequestID: requestID, RequestingActor: actorID, ActionName: "deploy", Target: "production", RiskClass: "critical", RequestedOutcome: "deploy", Justification: "test", RiskSummary: "protected"})

	now := time.Now().UTC().Add(time.Hour)
	clock := &missionControlClock{now: now}
	failing := &missionFailStore{Store: s}
	factoryProjection := factoryv1.Projection{
		SchemaVersion: factoryv1.SchemaVersion, GeneratedAt: now,
		Service: factoryv1.ServiceProjection{ServiceID: "factory-v1", InstanceID: "test", StartedAt: now.Add(-time.Hour), Healthy: true},
		Orders: []factoryv1.OrderProjection{{
			OrderID: "FO-MC-FACTORY-ONLY", Version: "1.0.0", Title: "Factory-only order", TargetRepository: "transpara-ai/site", DocumentSHA256: strings.Repeat("f", 64), EngineProtocol: factoryv1.TLCVersion,
			Status: "human_review", TLCStage: factoryv1.StageHumanReview, TLCIndex: factoryv1.StageIndex(factoryv1.StageHumanReview), StartedAt: now.Add(-2 * time.Hour), LastEffectAt: now.Add(-time.Minute), NextAction: "Human reviews exact-head PR",
			Stages: []factoryv1.StageLedgerProjection{{
				Stage: factoryv1.StageHumanReview, Index: factoryv1.StageIndex(factoryv1.StageHumanReview), State: factoryv1.TransitionHumanRequired,
				EventID: "factory-stage-event", OccurredAt: now.Add(-time.Minute), Peers: []string{"human"},
				Evidence: []factoryv1.Evidence{{
					Kind: "ready_pr", Reference: "https://github.com/transpara-ai/site/pull/42",
					PR: &factoryv1.PREvidence{Repository: "transpara-ai/site", Number: 42, URL: "https://github.com/transpara-ai/site/pull/42", HeadSHA: strings.Repeat("a", 40), ReviewedHeadSHA: strings.Repeat("a", 40), Open: true, ChecksPassing: true},
				}},
			}},
		}},
	}
	projector, err := NewCivilizationMissionControlProjector(failing, MissionControlProjectorConfig{
		FactoryProjection: func(context.Context) (factoryv1.Projection, error) { return factoryProjection, nil },
		Runtime:           &missionRuntimeFixture{clock: clock}, Clock: clock, PageSize: 2, Retention: 15 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	current := projector.Build(context.Background())
	if !current.Completeness.Complete || current.DerivationState.Freshness != FreshnessCurrent || len(current.WIP) != 6 {
		t.Fatalf("current projection incomplete: complete=%v state=%+v WIP=%d reasons=%+v", current.Completeness.Complete, current.DerivationState, len(current.WIP), current.Completeness.Reasons)
	}
	if current.Sources[0].Completeness.PageCounts[work.EventTypeTaskCreated.Value()] < 3 {
		t.Fatalf("task pagination = %+v", current.Sources[0].Completeness.PageCounts)
	}
	if len(current.HumanActions) < 2 {
		t.Fatalf("Human actions = %+v", current.HumanActions)
	}
	var factoryRow, workRow *WIPItem
	for i := range current.WIP {
		if current.WIP[i].Kind == "factory_order" {
			factoryRow = &current.WIP[i]
		} else {
			workRow = &current.WIP[i]
		}
	}
	if factoryRow == nil || workRow == nil || factoryRow.TargetRepository.Mark.State != StateCurrent || workRow.TargetRepository.Mark.State != StateUnavailable || workRow.TLCStage.Mark.State != StateUnavailable || workRow.Classification.EffectivePacketProfile != "P-ENVELOPE" || workRow.Classification.EffectiveHumanReviewTier != 3 {
		t.Fatalf("material WIP rows: factory=%+v work=%+v", factoryRow, workRow)
	}

	handler := NewOperatorProjectionServer(s, "secret", 2, WithMissionControlProjection(projector))
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, MissionControlProjectionPath, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	missingServerKey := NewOperatorProjectionServer(s, "", 2, WithMissionControlProjection(projector))
	missingServerKeyResponse := httptest.NewRecorder()
	missingServerKey.ServeHTTP(missingServerKeyResponse, httptest.NewRequest(http.MethodGet, MissionControlProjectionPath, nil))
	if missingServerKeyResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing server key status = %d", missingServerKeyResponse.Code)
	}
	authorizedRequest := httptest.NewRequest(http.MethodGet, MissionControlProjectionPath, nil)
	authorizedRequest.Header.Set("Authorization", "Bearer secret")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", authorized.Code, authorized.Body.String())
	}
	var decoded MissionControlProjection
	if err := json.Unmarshal(authorized.Body.Bytes(), &decoded); err != nil || decoded.SchemaVersion != MissionControlSchemaVersion {
		t.Fatalf("endpoint decode = %v %+v", err, decoded)
	}

	failing.setFail(true)
	clock.Add(time.Minute)
	stale := projector.Build(context.Background())
	if stale.Sources[0].Mark.Freshness != FreshnessStale || stale.WIP[0].Mark.Freshness != FreshnessStale || stale.Sources[0].Mark.ObservedAt != current.Sources[0].Mark.ObservedAt {
		t.Fatalf("stale retention = source %+v row %+v", stale.Sources[0].Mark, stale.WIP[0].Mark)
	}
	clock.Add(15 * time.Minute)
	expired := projector.Build(context.Background())
	if expired.Sources[0].Mark.Freshness != FreshnessUnavailable || expired.Completeness.Complete {
		t.Fatalf("expired retention = %+v complete=%v", expired.Sources[0], expired.Completeness.Complete)
	}
}

func TestHIVEMCT5WorkStateAllowlistFullOuterJoinAndEvidence(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	now := time.Now().UTC().Add(time.Hour)
	allStatuses := []work.TaskStatus{
		work.StatusCreated, work.StatusReady, work.StatusRunning, work.StatusBlocked, work.StatusFailed,
		work.StatusRepairRequired, work.StatusRepairRunning, work.StatusRepaired, work.StatusVerificationRunning,
		work.StatusVerified, work.StatusCertified, work.StatusRejected, work.StatusSuperseded, work.StatusPolicyBlocked,
		work.TaskStatus("paused-by-future-protocol"),
	}
	for _, status := range allStatuses {
		created := appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{Title: "state:" + string(status), CreatedBy: actorID})
		if status != work.StatusCreated {
			appendEvent(work.EventTypeTaskLifecycleTransitioned, work.TaskLifecycleTransitionContent{TaskID: created.ID(), FromState: work.StatusCreated, ToState: status, ChangedBy: actorID})
		}
	}
	for i := 0; i < 2; i++ {
		appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{Title: fmt.Sprintf("duplicate-link-%d", i), CreatedBy: actorID, FactoryOrderID: "FO-DUPLINK"})
	}
	conflicting := appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{Title: "conflicting-link", CreatedBy: actorID, FactoryOrderID: "FO-A"})
	appendEvent(work.EventTypeTaskLinked, work.TaskLinkedContent{TaskID: conflicting.ID(), FactoryOrderID: "FO-B", LinkedBy: actorID})

	zero, one := 0, 1
	headA, headB := strings.Repeat("a", 40), strings.Repeat("b", 40)
	designBlob := strings.Repeat("d", 40)
	provider := &factoryv1.ProviderBinding{ProviderID: "claude-cli", Family: "Anthropic/Claude", ModelID: "claude-opus-4-6"}
	evidenceOrder := factoryv1.OrderProjection{
		OrderID: "FO-EVIDENCE", Version: "1.0.0", Title: "Evidence order", TargetRepository: "transpara-ai/site", DocumentSHA256: strings.Repeat("e", 64), EngineProtocol: factoryv1.TLCVersion,
		Status: "human_review", TLCStage: factoryv1.StageHumanReview, TLCIndex: factoryv1.StageIndex(factoryv1.StageHumanReview), StartedAt: now.Add(-4 * time.Hour), LastEffectAt: now.Add(-time.Minute), NextAction: "Human reviews exact-head PR",
		PR: &factoryv1.PRProjection{Repository: "transpara-ai/site", Number: 42, HeadSHA: headA, ReviewedHeadSHA: headB, Open: true, ChecksPassing: true},
		Stages: []factoryv1.StageLedgerProjection{
			{Stage: factoryv1.StageDesign, State: factoryv1.TransitionPassed, EventID: "design-event", OccurredAt: now.Add(-3 * time.Hour), Evidence: []factoryv1.Evidence{{Kind: "design", Reference: "docs/design.md", DesignBlobSHA: designBlob}}},
			{Stage: factoryv1.StageIADA, State: factoryv1.TransitionBlocked, EventID: "iada-blocked", OccurredAt: now.Add(-170 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "iada", Reference: "iada:attempt-1", DesignBlobSHA: designBlob, BlockerCount: &one}}},
			{Stage: factoryv1.StageIADA, State: factoryv1.TransitionPassed, EventID: "iada-repaired", OccurredAt: now.Add(-160 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "iada", Reference: "iada:attempt-2", DesignBlobSHA: designBlob, BlockerCount: &zero}}},
			{Stage: factoryv1.StageCFADA, State: factoryv1.TransitionPassed, EventID: "cfada", OccurredAt: now.Add(-150 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "cfada", Reference: "cfada:pass", DesignBlobSHA: designBlob, BlockerCount: &zero, AuthorFamily: "OpenAI/Codex", ReviewerFamily: "Anthropic/Claude", Provider: provider}}},
			{Stage: factoryv1.StageHumanDesignReview, State: factoryv1.TransitionPassed, EventID: "hdr", OccurredAt: now.Add(-2 * time.Hour), Evidence: []factoryv1.Evidence{{Kind: "human_design_review", Reference: "hdr:event", DesignBlobSHA: designBlob, Approval: &factoryv1.HumanApprovalReceipt{ActorID: "human"}}}},
			{Stage: factoryv1.StageCreateDraftPR, State: factoryv1.TransitionPassed, EventID: "draft-pr", OccurredAt: now.Add(-90 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "draft_pr", Reference: "pr:42", PR: &factoryv1.PREvidence{Repository: "transpara-ai/site", Number: 42, HeadSHA: headA, ReviewedHeadSHA: headA, Open: true, Draft: true}}}},
			{Stage: factoryv1.StageIAR, State: factoryv1.TransitionPassed, EventID: "iar", OccurredAt: now.Add(-80 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "iar", Reference: "iar:pass", PRHeadSHA: headA, ReviewedHeadSHA: headA, BlockerCount: &zero, AuthorFamily: "OpenAI/Codex", ReviewerFamily: "OpenAI/Codex"}}},
			{Stage: factoryv1.StageCFAR, State: factoryv1.TransitionPassed, EventID: "cfar", OccurredAt: now.Add(-70 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "cfar", Reference: "cfar:pass", PRHeadSHA: headA, ReviewedHeadSHA: headA, BlockerCount: &zero, AuthorFamily: "OpenAI/Codex", ReviewerFamily: "Anthropic/Claude", Provider: provider}}},
			{Stage: factoryv1.StageMarkPRReady, State: factoryv1.TransitionPassed, EventID: "ready", OccurredAt: now.Add(-60 * time.Minute), Evidence: []factoryv1.Evidence{{Kind: "ready_pr", Reference: "pr:42:ready", PR: &factoryv1.PREvidence{Repository: "transpara-ai/site", Number: 42, HeadSHA: headA, ReviewedHeadSHA: headB, Open: true, ChecksPassing: true}}}},
			{Stage: factoryv1.StageHumanReview, State: factoryv1.TransitionHumanRequired, EventID: "human-review", OccurredAt: now.Add(-time.Minute), Evidence: []factoryv1.Evidence{{Kind: "human_review_required", Reference: "human-review:pending"}}},
		},
	}
	order := func(id string) factoryv1.OrderProjection {
		return factoryv1.OrderProjection{
			OrderID: id, Version: "1.0.0", Title: id, TargetRepository: "transpara-ai/hive", DocumentSHA256: strings.Repeat("f", 64), EngineProtocol: factoryv1.TLCVersion,
			Status: "accepted", TLCStage: factoryv1.StageWriteCode, TLCIndex: factoryv1.StageIndex(factoryv1.StageWriteCode), StartedAt: now.Add(-time.Hour), LastEffectAt: now.Add(-time.Minute), NextAction: "start write_code",
		}
	}
	invalid := order("FO-INVALID")
	invalid.Status, invalid.Blocker, invalid.NextAction = "blocked", "invalid TLC ledger: out-of-order transition", "repair or invalidate the conflicting stage ledger"
	hdr := order("FO-A")
	hdr.Status, hdr.TLCStage, hdr.TLCIndex, hdr.NextAction = "human_required", factoryv1.StageHumanDesignReview, factoryv1.StageIndex(factoryv1.StageHumanDesignReview), "Human approves the exact design blob"
	duplicate := order("FO-DUPLICATE")
	factoryProjection := factoryv1.Projection{
		SchemaVersion: factoryv1.SchemaVersion, GeneratedAt: now,
		Service: factoryv1.ServiceProjection{ServiceID: "factory-v1", InstanceID: "test", StartedAt: now.Add(-time.Hour), Healthy: true},
		Orders:  []factoryv1.OrderProjection{order("FO-DUPLINK"), hdr, order("FO-B"), invalid, evidenceOrder, duplicate, duplicate},
		Interventions: []factoryv1.InterventionProjection{
			{InterventionID: "open-1", OrderID: "FO-INVALID", Kind: "evidence_repair", Prompt: "Repair invalid ledger", Status: factoryv1.InterventionOpen, RequestedAt: now.Add(-time.Minute), EventID: "intervention-open"},
			{InterventionID: "resolved-1", OrderID: "FO-A", Kind: "review", Prompt: "Already resolved", Status: factoryv1.InterventionResolved, RequestedAt: now.Add(-time.Hour), EventID: "intervention-resolved"},
		},
	}
	projector, err := NewCivilizationMissionControlProjector(s, MissionControlProjectorConfig{
		FactoryProjection: func(context.Context) (factoryv1.Projection, error) { return factoryProjection, nil },
		Clock:             &missionControlClock{now: now}, PageSize: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	wip, err := projector.buildWIPSource(context.Background(), now)
	if err != nil || !wip.Completeness.Complete {
		t.Fatalf("HIVE-MC-T5 WIP build = %v complete=%+v", err, wip.Completeness)
	}
	if wip.Completeness.PageCounts[work.EventTypeTaskCreated.Value()] < 6 {
		t.Fatalf("HIVE-MC-T5 task pagination = %+v", wip.Completeness.PageCounts)
	}
	rowsByTitle := map[string][]WIPItem{}
	for _, row := range wip.Rows {
		rowsByTitle[row.Title] = append(rowsByTitle[row.Title], row)
	}
	for _, terminal := range []work.TaskStatus{work.StatusCertified, work.StatusRejected, work.StatusSuperseded} {
		if len(rowsByTitle["state:"+string(terminal)]) != 0 {
			t.Errorf("terminal Work state %s remained in WIP", terminal)
		}
	}
	for _, active := range []work.TaskStatus{work.StatusCreated, work.StatusReady, work.StatusRunning, work.StatusBlocked, work.StatusFailed, work.StatusRepairRequired, work.StatusRepairRunning, work.StatusRepaired, work.StatusVerificationRunning, work.StatusVerified, work.StatusPolicyBlocked} {
		if len(rowsByTitle["state:"+string(active)]) != 1 {
			t.Errorf("active Work state %s missing from WIP", active)
		}
	}
	unknown := rowsByTitle["state:paused-by-future-protocol"][0]
	if unknown.LifecycleStatus.Value != "unavailable" || unknown.LifecycleStatus.Mark.State != StateUnavailable || unknown.Completeness.Value != false {
		t.Fatalf("unknown Work state did not fail closed: %+v", unknown)
	}
	for i := 0; i < 2; i++ {
		row := rowsByTitle[fmt.Sprintf("duplicate-link-%d", i)][0]
		if len(row.BlockerRefs) == 0 || !strings.Contains(strings.Join(row.BlockerRefs, ";"), "multiple Work tasks") {
			t.Errorf("duplicate link row is not blocked: %+v", row)
		}
	}
	if row := rowsByTitle["FO-DUPLINK"][0]; len(row.BlockerRefs) == 0 || row.Completeness.Value != false {
		t.Errorf("Factory duplicate-link row is not visibly incomplete: %+v", row)
	}
	if row := rowsByTitle["conflicting-link"][0]; len(row.BlockerRefs) == 0 || !strings.Contains(strings.Join(row.BlockerRefs, ";"), "conflicting") {
		t.Errorf("conflicting Work link is not blocked: %+v", row)
	}
	duplicates := rowsByTitle["FO-DUPLICATE"]
	if len(duplicates) != 2 || duplicates[0].StableID == duplicates[1].StableID || len(duplicates[0].BlockerRefs) == 0 || len(duplicates[1].BlockerRefs) == 0 {
		t.Fatalf("duplicate Factory tuples are not distinct blockers: %+v", duplicates)
	}
	invalidRow := rowsByTitle["FO-INVALID"][0]
	if invalidRow.Completeness.Value != false || len(invalidRow.BlockerRefs) == 0 || len(invalidRow.InterventionRefs) == 0 {
		t.Fatalf("invalid ledger row was treated complete: %+v", invalidRow)
	}
	for _, handoff := range wip.Handoffs {
		if handoff.SubjectID == "FO-INVALID" && (!handoff.Blocked || handoff.ToStage != "intervention_resolution") {
			t.Fatalf("invalid ledger normal handoff was not suppressed: %+v", handoff)
		}
	}
	evidenceRow := rowsByTitle["Evidence order"][0]
	if len(evidenceRow.EvidenceRollup.Items) < 10 || evidenceRow.EvidenceRollup.FieldMarks["factory_order_ref"].State != StateCurrent || evidenceRow.EvidenceRollup.FieldMarks["design_blob_sha"].State != StateCurrent || evidenceRow.EvidenceRollup.FieldMarks["human_design_review_ref"].State != StateCurrent || evidenceRow.EvidenceRollup.FieldMarks["pr_head_sha"].State != StateCurrent || evidenceRow.EvidenceRollup.FieldMarks["reviewed_head_sha"].State != StateCurrent || evidenceRow.EvidenceRollup.ReadyHeadMatches || evidenceRow.EvidenceRollup.FieldMarks["ready_head_matches"].State != StateCurrent || !evidenceRow.EvidenceRollup.PendingTier3HumanReview {
		t.Fatalf("cumulative exact evidence rollup = %+v", evidenceRow.EvidenceRollup)
	}
	if rowsByTitle["FO-A"][0].EvidenceRollup.FieldMarks["design_blob_sha"].State != StateUnavailable {
		t.Fatal("missing design evidence was not individually unavailable")
	}
	openActions, designReviewActions, humanReviewActions := 0, 0, 0
	for _, action := range wip.HumanActions {
		if action.ActionID == "intervention:open-1" {
			openActions++
		}
		if action.ActionID == "intervention:resolved-1" {
			t.Error("resolved intervention produced a Human action")
		}
		if action.Kind == "human_design_review" {
			designReviewActions++
		}
		if action.Kind == "human_review" {
			humanReviewActions++
		}
	}
	if openActions != 1 || designReviewActions != 1 || humanReviewActions != 1 {
		t.Fatalf("Human action counts open=%d design=%d review=%d; actions=%+v", openActions, designReviewActions, humanReviewActions, wip.HumanActions)
	}
}

func TestHIVEMCT5WIPCursorDecodeAndHeadSkewFailClosed(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	created := appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{Title: "head-skew-task", CreatedBy: actorID})
	spawn := appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{Name: "replacement", Role: "test", Model: "test", ActorID: actorID.Value()})
	now := time.Now().UTC().Add(time.Hour)
	order := factoryv1.OrderProjection{
		OrderID: "FO-SKEW", Version: "1.0.0", Title: "head-skew-order", TargetRepository: "transpara-ai/hive", DocumentSHA256: strings.Repeat("a", 64), EngineProtocol: factoryv1.TLCVersion,
		Status: "human_review", TLCStage: factoryv1.StageHumanReview, TLCIndex: factoryv1.StageIndex(factoryv1.StageHumanReview), StartedAt: now.Add(-time.Hour), LastEffectAt: now.Add(-time.Minute), NextAction: "Human reviews exact-head PR",
	}
	factoryProjection := func(context.Context) (factoryv1.Projection, error) {
		return factoryv1.Projection{
			SchemaVersion: factoryv1.SchemaVersion, GeneratedAt: now, Service: factoryv1.ServiceProjection{ServiceID: "factory-v1", InstanceID: "test", StartedAt: now.Add(-time.Hour), Healthy: true},
			Orders: []factoryv1.OrderProjection{order}, Interventions: []factoryv1.InterventionProjection{{InterventionID: "skew-intervention", OrderID: order.OrderID, Kind: "review", Prompt: "review", Status: factoryv1.InterventionOpen, RequestedAt: now.Add(-time.Minute), EventID: "skew-intervention-event"}},
		}, nil
	}

	if _, _, err := missionEventsByType(&missionNoAdvanceStore{Store: s, eventType: work.EventTypeTaskCreated}, work.EventTypeTaskCreated, 2); err == nil || !strings.Contains(err.Error(), "no advancing cursor") {
		t.Fatalf("WIP non-advancing cursor accepted: %v", err)
	}
	decodeProjector, _ := NewCivilizationMissionControlProjector(&missionDecodeFaultStore{Store: s, eventType: work.EventTypeTaskCreated, replacement: spawn}, MissionControlProjectorConfig{FactoryProjection: factoryProjection, Clock: &missionControlClock{now: now}, PageSize: 2})
	decodeWIP, _ := decodeProjector.buildWIPSource(context.Background(), now)
	if decodeWIP.Completeness.Complete || !strings.Contains(strings.Join(decodeWIP.Completeness.Reasons, ";"), "decode") {
		t.Fatalf("WIP decode failure did not fail closed: %+v", decodeWIP.Completeness)
	}

	skewStore := &missionHeadSkewStore{Store: s, eventType: work.EventTypeTaskCreated, mutate: func() { appendEvent(EventTypeProgress, ProgressContent{Message: "WIP head skew"}) }}
	skewProjector, _ := NewCivilizationMissionControlProjector(skewStore, MissionControlProjectorConfig{FactoryProjection: factoryProjection, Clock: &missionControlClock{now: now}, PageSize: 2})
	skewWIP, _ := skewProjector.buildWIPSource(context.Background(), now)
	if skewWIP.Completeness.Complete || !strings.Contains(strings.Join(skewWIP.Completeness.Reasons, ";"), "head changed") {
		t.Fatalf("WIP head skew did not fail closed: %+v", skewWIP.Completeness)
	}
	if len(skewWIP.Rows) != 2 || len(skewWIP.Handoffs) != 1 || len(skewWIP.Interventions) != 1 || len(skewWIP.HumanActions) < 2 {
		t.Fatalf("head-skew fixture lost material rows: rows=%d handoffs=%d interventions=%d actions=%d created=%s", len(skewWIP.Rows), len(skewWIP.Handoffs), len(skewWIP.Interventions), len(skewWIP.HumanActions), created.ID().Value())
	}
	for _, row := range skewWIP.Rows {
		if row.Mark.State != StateProjectedOnly || (row.Kind == "factory_order" && row.EvidenceRollup.FieldMarks["factory_order_ref"].State != StateProjectedOnly) {
			t.Fatalf("head-skew WIP row retained exact credit: %+v", row)
		}
	}
	if skewWIP.Handoffs[0].Mark.State != StateProjectedOnly || skewWIP.Interventions[0].Mark.State != StateProjectedOnly {
		t.Fatalf("head-skew mechanics retained exact credit: handoff=%+v intervention=%+v", skewWIP.Handoffs[0], skewWIP.Interventions[0])
	}
	for _, action := range skewWIP.HumanActions {
		if action.Mark.State != StateProjectedOnly {
			t.Fatalf("head-skew Human action retained exact credit: %+v", action)
		}
	}
}
