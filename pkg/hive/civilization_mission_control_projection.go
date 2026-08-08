package hive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	"github.com/transpara-ai/work"
)

const (
	MissionControlSchemaVersion   = "civilization-mission-control/v1"
	MissionControlProjectionPath  = "/api/hive/civilization/mission-control-projection"
	missionControlRetention       = 15 * time.Minute
	missionControlDefaultPageSize = 50
)

type EvidenceFreshness string
type EvidenceBasis string
type EvidenceState string

const (
	FreshnessCurrent     EvidenceFreshness = "current"
	FreshnessStale       EvidenceFreshness = "stale"
	FreshnessUnavailable EvidenceFreshness = "unavailable"
	BasisExact           EvidenceBasis     = "exact"
	BasisInferred        EvidenceBasis     = "inferred"
	BasisProjectedOnly   EvidenceBasis     = "projected_only"
	BasisUnavailable     EvidenceBasis     = "unavailable"
	StateCurrent         EvidenceState     = "current"
	StateStale           EvidenceState     = "stale"
	StateUnavailable     EvidenceState     = "unavailable"
	StateInferred        EvidenceState     = "inferred"
	StateProjectedOnly   EvidenceState     = "projected_only"
)

type EvidenceMark struct {
	State        EvidenceState     `json:"state"`
	Freshness    EvidenceFreshness `json:"freshness"`
	Basis        EvidenceBasis     `json:"basis"`
	SourceID     string            `json:"source_id"`
	ObservedAt   time.Time         `json:"observed_at"`
	GeneratedAt  time.Time         `json:"generated_at"`
	EvidenceRefs []string          `json:"evidence_refs"`
	Reason       string            `json:"reason,omitempty"`
}

func NewEvidenceMark(freshness EvidenceFreshness, basis EvidenceBasis, sourceID string, observedAt, generatedAt time.Time, refs []string, reason string) EvidenceMark {
	mark := EvidenceMark{
		Freshness: freshness, Basis: basis, SourceID: strings.TrimSpace(sourceID),
		ObservedAt: observedAt.UTC(), GeneratedAt: generatedAt.UTC(),
		EvidenceRefs: compactStrings(refs), Reason: boundedMissionReason(reason),
	}
	mark.State = deriveEvidenceState(freshness, basis)
	if mark.State == StateUnavailable && mark.Reason == "" {
		mark.Reason = "evidence is unavailable or invalid"
	}
	return mark
}

func deriveEvidenceState(freshness EvidenceFreshness, basis EvidenceBasis) EvidenceState {
	if freshness == FreshnessUnavailable {
		return StateUnavailable
	}
	if freshness != FreshnessCurrent && freshness != FreshnessStale {
		return StateUnavailable
	}
	if basis != BasisExact && basis != BasisInferred && basis != BasisProjectedOnly {
		return StateUnavailable
	}
	if freshness == FreshnessStale {
		return StateStale
	}
	switch basis {
	case BasisExact:
		return StateCurrent
	case BasisInferred:
		return StateInferred
	case BasisProjectedOnly:
		return StateProjectedOnly
	default:
		return StateUnavailable
	}
}

func boundedMissionReason(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	if len(value) > 1024 {
		value = value[:1024]
	}
	return value
}

type MarkedValue struct {
	Value any          `json:"value"`
	Mark  EvidenceMark `json:"mark"`
}

type MissionCompleteness struct {
	Complete             bool           `json:"complete"`
	Reasons              []string       `json:"reasons"`
	SourceEventGraphHead string         `json:"source_eventgraph_head"`
	StartHead            string         `json:"start_head"`
	EndHead              string         `json:"end_head"`
	DomainCounts         map[string]int `json:"domain_counts"`
	PageCounts           map[string]int `json:"page_counts"`
}

type SourceEnvelope struct {
	SourceID     string              `json:"source_id"`
	Required     bool                `json:"required"`
	Completeness MissionCompleteness `json:"completeness"`
	Mark         EvidenceMark        `json:"mark"`
}

type ServiceHealth struct {
	ServiceID         string       `json:"service_id"`
	Label             string       `json:"label"`
	OperationalStatus string       `json:"operational_status"`
	Detail            string       `json:"detail"`
	Mark              EvidenceMark `json:"mark"`
}

type MissionClassification struct {
	EngineProtocol              string       `json:"engine_protocol"`
	DeclaredGovernanceProtocol  string       `json:"declared_governance_protocol"`
	DeclaredPacketProfile       string       `json:"declared_packet_profile"`
	DeclaredHumanReviewTier     *int         `json:"declared_human_review_tier"`
	EffectiveGovernanceProtocol string       `json:"effective_governance_protocol"`
	EffectivePacketProfile      string       `json:"effective_packet_profile"`
	EffectiveHumanReviewTier    int          `json:"effective_human_review_tier"`
	Mark                        EvidenceMark `json:"mark"`
	EvidenceRefs                []string     `json:"evidence_refs"`
}

type MissionEvidenceItem struct {
	Kind            string       `json:"kind"`
	Stage           string       `json:"stage,omitempty"`
	State           string       `json:"state,omitempty"`
	Reference       string       `json:"reference"`
	BlobSHA         string       `json:"blob_sha,omitempty"`
	PRHeadSHA       string       `json:"pr_head_sha,omitempty"`
	ReviewedHeadSHA string       `json:"reviewed_head_sha,omitempty"`
	BlockerCount    *int         `json:"blocker_count,omitempty"`
	AuthorFamily    string       `json:"author_family,omitempty"`
	ReviewerFamily  string       `json:"reviewer_family,omitempty"`
	ProviderID      string       `json:"provider_id,omitempty"`
	Mark            EvidenceMark `json:"mark"`
}

type EvidenceRollup struct {
	FactoryOrderRef         string                  `json:"factory_order_ref"`
	DesignBlobSHA           string                  `json:"design_blob_sha"`
	HumanDesignReviewRef    string                  `json:"human_design_review_ref"`
	PRRepository            string                  `json:"pr_repository"`
	PRNumber                int                     `json:"pr_number"`
	PRState                 string                  `json:"pr_state"`
	PRHeadSHA               string                  `json:"pr_head_sha"`
	ReviewedHeadSHA         string                  `json:"reviewed_head_sha"`
	ReadyHeadMatches        bool                    `json:"ready_head_matches"`
	PendingTier3HumanReview bool                    `json:"pending_tier_3_human_review"`
	Items                   []MissionEvidenceItem   `json:"items"`
	FieldMarks              map[string]EvidenceMark `json:"field_marks"`
	Mark                    EvidenceMark            `json:"mark"`
}

type WIPItem struct {
	Kind                string                `json:"kind"`
	StableID            string                `json:"stable_id"`
	FactoryOrderID      string                `json:"factory_order_id"`
	FactoryOrderVersion string                `json:"factory_order_version"`
	DocumentSHA256      string                `json:"document_sha256"`
	WorkTaskID          string                `json:"work_task_id"`
	Title               string                `json:"title"`
	TargetRepository    MarkedValue           `json:"target_repository"`
	Assignment          MarkedValue           `json:"assignment"`
	LifecycleStatus     MarkedValue           `json:"lifecycle_status"`
	EngineProtocol      MarkedValue           `json:"engine_protocol"`
	TLCStage            MarkedValue           `json:"tlc_stage"`
	TLCStageIndex       MarkedValue           `json:"tlc_stage_index"`
	ItemStartedAt       MarkedValue           `json:"item_started_at"`
	LastEffectAt        MarkedValue           `json:"last_effect_at"`
	ElapsedMS           MarkedValue           `json:"elapsed_ms"`
	NextHandoff         MarkedValue           `json:"next_handoff"`
	Completeness        MarkedValue           `json:"completeness"`
	Classification      MissionClassification `json:"classification"`
	BlockerRefs         []string              `json:"blocker_refs"`
	InterventionRefs    []string              `json:"intervention_refs"`
	EvidenceRollup      EvidenceRollup        `json:"evidence_rollup"`
	Mark                EvidenceMark          `json:"mark"`
}

type RoleAgentRow struct {
	StableID     string       `json:"stable_id"`
	Role         string       `json:"role"`
	ActorID      string       `json:"actor_id"`
	Configured   MarkedValue  `json:"configured"`
	Instantiated MarkedValue  `json:"instantiated"`
	EventActive  MarkedValue  `json:"event_active"`
	Running      MarkedValue  `json:"running"`
	Provider     MarkedValue  `json:"provider"`
	Model        MarkedValue  `json:"model"`
	Authority    MarkedValue  `json:"authority"`
	Capacity     MarkedValue  `json:"capacity"`
	Status       MarkedValue  `json:"status"`
	Assignment   MarkedValue  `json:"assignment"`
	Mark         EvidenceMark `json:"mark"`
}

type WorkerPool struct {
	ConfiguredWorkers  MarkedValue                   `json:"configured_workers"`
	ActiveWorkers      MarkedValue                   `json:"active_workers"`
	AvailableWorkers   MarkedValue                   `json:"available_workers"`
	QueuedOrders       MarkedValue                   `json:"queued_orders"`
	SchedulableOrders  MarkedValue                   `json:"schedulable_orders"`
	Assignments        []factoryv1.RuntimeAssignment `json:"assignments"`
	UtilizationPercent MarkedValue                   `json:"utilization_percent"`
	Mark               EvidenceMark                  `json:"mark"`
}

type HumanAction struct {
	ActionID       string       `json:"action_id"`
	Kind           string       `json:"kind"`
	Severity       string       `json:"severity"`
	OwningStage    string       `json:"owning_stage"`
	SubjectID      string       `json:"subject_id"`
	Summary        string       `json:"summary"`
	RequiredAction string       `json:"required_action"`
	SourceTime     time.Time    `json:"source_time"`
	EvidenceRefs   []string     `json:"evidence_refs"`
	Link           string       `json:"link"`
	Mark           EvidenceMark `json:"mark"`
}

type MissionIntervention struct {
	InterventionID string       `json:"intervention_id"`
	OrderID        string       `json:"order_id"`
	Kind           string       `json:"kind"`
	Status         string       `json:"status"`
	Prompt         string       `json:"prompt"`
	RequestedAt    time.Time    `json:"requested_at"`
	EvidenceRefs   []string     `json:"evidence_refs"`
	Mark           EvidenceMark `json:"mark"`
}

type Handoff struct {
	HandoffID           string       `json:"handoff_id"`
	SubjectID           string       `json:"subject_id"`
	FromStage           string       `json:"from_stage"`
	ToStage             string       `json:"to_stage"`
	ExpectedRoles       []string     `json:"expected_roles"`
	CompletionPredicate string       `json:"completion_predicate"`
	Blocked             bool         `json:"blocked"`
	EvidenceRefs        []string     `json:"evidence_refs"`
	Mark                EvidenceMark `json:"mark"`
}

type MissionControlProjection struct {
	SchemaVersion     string                `json:"schema_version"`
	GeneratedAt       time.Time             `json:"generated_at"`
	DerivationState   EvidenceMark          `json:"derivation_state"`
	OperationalStatus string                `json:"operational_status"`
	Completeness      MissionCompleteness   `json:"completeness"`
	Sources           []SourceEnvelope      `json:"sources"`
	Services          []ServiceHealth       `json:"services"`
	WIP               []WIPItem             `json:"wip"`
	Roles             []RoleAgentRow        `json:"roles"`
	WorkerPool        WorkerPool            `json:"worker_pool"`
	HumanActions      []HumanAction         `json:"human_actions"`
	Interventions     []MissionIntervention `json:"interventions"`
	Handoffs          []Handoff             `json:"handoffs"`
	ResidualRisks     []string              `json:"residual_risks"`
	NonAuthorizations []string              `json:"non_authorizations"`
}

func missionUnavailableMark(source string, now time.Time, reason string) EvidenceMark {
	return NewEvidenceMark(FreshnessUnavailable, BasisUnavailable, source, time.Time{}, now, nil, reason)
}

func missionMarked(value any, mark EvidenceMark) MarkedValue {
	return MarkedValue{Value: value, Mark: mark}
}

func missionProfileRank(profile string) int {
	switch profile {
	case "P-MECHANICAL":
		return 0
	case "P-IMPLEMENTATION":
		return 1
	case "P-DESIGN-DELTA":
		return 2
	case "P-ENVELOPE":
		return 3
	default:
		return -1
	}
}

func classifyMissionOrder(order factoryv1.OrderProjection, now time.Time) MissionClassification {
	mark := NewEvidenceMark(FreshnessCurrent, BasisInferred, "factory_v1_ledger", order.LastEffectAt, now, nil, "missing or unsupported exact TLC 4.5.0 classification evidence; fail upward")
	result := MissionClassification{
		EngineProtocol: order.EngineProtocol, EffectiveGovernanceProtocol: "4.5.0",
		EffectivePacketProfile: "P-ENVELOPE", EffectiveHumanReviewTier: 3,
		Mark: mark, EvidenceRefs: []string{},
	}
	var candidates []factoryv1.Evidence
	for _, stage := range order.Stages {
		for _, evidence := range stage.Evidence {
			if evidence.Kind == "tlc_change_classification" {
				candidates = append(candidates, evidence)
			}
		}
	}
	if len(candidates) == 0 {
		return result
	}
	metadata := candidates[0].Metadata
	result.DeclaredGovernanceProtocol = metadata["protocol"]
	result.DeclaredPacketProfile = metadata["profile"]
	if tier, err := strconv.Atoi(metadata["tier"]); err == nil {
		result.DeclaredHumanReviewTier = &tier
	}
	valid := true
	var reason string
	for _, evidence := range candidates {
		m := evidence.Metadata
		tier, tierErr := strconv.Atoi(m["tier"])
		profileRank := missionProfileRank(m["profile"])
		if !isExactGitOrDocumentHash(evidence.Reference) || m["protocol"] != "4.5.0" || profileRank < 0 || tierErr != nil || tier < 0 || tier > 3 || m["order_id"] != order.OrderID || m["order_version"] != order.Version || m["document_sha256"] != order.DocumentSHA256 {
			valid, reason = false, "classification evidence is malformed or bound to another subject"
			break
		}
		if profileRank < missionProfileRank("P-ENVELOPE") && (!isExactGitOrDocumentHash(m["classification_result_blob"]) || !isExactGitOrDocumentHash(m["inventory_blob"])) {
			valid, reason = false, "reduced-profile evidence lacks exact classifier or inventory blobs"
			break
		}
		if m["protocol"] != metadata["protocol"] || m["profile"] != metadata["profile"] || m["tier"] != metadata["tier"] {
			valid, reason = false, "classification evidence conflicts for the exact subject"
			break
		}
		result.EvidenceRefs = append(result.EvidenceRefs, evidence.Reference)
	}
	if !valid {
		result.Mark.Reason = reason
		result.EvidenceRefs = compactStrings(result.EvidenceRefs)
		return result
	}
	tier, _ := strconv.Atoi(metadata["tier"])
	result.EffectiveGovernanceProtocol = metadata["protocol"]
	result.EffectivePacketProfile = metadata["profile"]
	result.EffectiveHumanReviewTier = tier
	result.Mark = NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", order.LastEffectAt, now, result.EvidenceRefs, "")
	result.EvidenceRefs = compactStrings(result.EvidenceRefs)
	return result
}

type FactoryProjectionBuilder func(context.Context) (factoryv1.Projection, error)

type MissionRuntimeSource interface {
	Fetch(context.Context, time.Time, *FactoryRuntimeSnapshot) (FactoryRuntimeSnapshot, error)
}

type MissionControlProjectorConfig struct {
	FactoryProjection FactoryProjectionBuilder
	ModelSelection    OperatorModelSelectionSource
	Runtime           MissionRuntimeSource
	Clock             factoryv1.Clock
	PageSize          int
	Retention         time.Duration
}

type cachedMissionSource[T any] struct {
	value      T
	observedAt time.Time
	valid      bool
}

type missionWIPSource struct {
	GeneratedAt    time.Time
	Rows           []WIPItem
	Interventions  []MissionIntervention
	Handoffs       []Handoff
	HumanActions   []HumanAction
	FactoryService factoryv1.ServiceProjection
	Completeness   MissionCompleteness
}

type missionRosterSource struct {
	GeneratedAt  time.Time
	Rows         []RoleAgentRow
	Completeness MissionCompleteness
}

type missionAuthoritySource struct {
	GeneratedAt  time.Time
	HumanActions []HumanAction
	Completeness MissionCompleteness
}

type CivilizationMissionControlProjector struct {
	store          store.Store
	factory        FactoryProjectionBuilder
	modelSelection OperatorModelSelectionSource
	runtime        MissionRuntimeSource
	clock          factoryv1.Clock
	pageSize       int
	retention      time.Duration

	mu             sync.Mutex
	wipCache       cachedMissionSource[missionWIPSource]
	rosterCache    cachedMissionSource[missionRosterSource]
	authorityCache cachedMissionSource[missionAuthoritySource]
	runtimeCache   cachedMissionSource[FactoryRuntimeSnapshot]
}

func NewCivilizationMissionControlProjector(s store.Store, config MissionControlProjectorConfig) (*CivilizationMissionControlProjector, error) {
	if s == nil {
		return nil, errors.New("mission control projector requires EventGraph store")
	}
	if config.Clock == nil {
		config.Clock = factoryv1.WallClock{}
	}
	if config.PageSize <= 0 {
		config.PageSize = missionControlDefaultPageSize
	}
	if config.PageSize > 1000 {
		return nil, errors.New("mission control page size exceeds 1000")
	}
	if config.Retention == 0 {
		config.Retention = missionControlRetention
	}
	if config.Retention < time.Second || config.Retention > missionControlRetention {
		return nil, errors.New("mission control retention must be between 1s and 15m")
	}
	return &CivilizationMissionControlProjector{
		store: s, factory: config.FactoryProjection, modelSelection: config.ModelSelection,
		runtime: config.Runtime, clock: config.Clock, pageSize: config.PageSize, retention: config.Retention,
	}, nil
}

func (p *CivilizationMissionControlProjector) Build(ctx context.Context) MissionControlProjection {
	now := p.clock.Now().UTC()
	wip, wipMark := p.acquireWIP(ctx, now)
	roster, rosterMark := p.acquireRoster(ctx, now)
	authority, authorityMark := p.acquireAuthority(ctx, now)
	runtimeSnapshot, runtimeMark := p.acquireRuntime(ctx, now)

	workerPool := missionWorkerPool(runtimeSnapshot, runtimeMark)
	roles := append([]RoleAgentRow(nil), roster.Rows...)
	roles = append(roles, runtimeRoleRows(runtimeSnapshot, runtimeMark)...)
	sort.Slice(roles, func(i, j int) bool { return roles[i].StableID < roles[j].StableID })
	actions := append([]HumanAction(nil), wip.HumanActions...)
	actions = append(actions, authority.HumanActions...)
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Severity == actions[j].Severity {
			return actions[i].ActionID < actions[j].ActionID
		}
		return missionSeverityRank(actions[i].Severity) > missionSeverityRank(actions[j].Severity)
	})

	sources := []SourceEnvelope{
		{SourceID: "eventgraph_wip_evidence", Required: true, Completeness: wip.Completeness, Mark: wipMark},
		{SourceID: "roster_routing", Required: true, Completeness: roster.Completeness, Mark: rosterMark},
		{SourceID: "authority_actions", Required: true, Completeness: authority.Completeness, Mark: authorityMark},
		{SourceID: "factory_runtime", Required: true, Completeness: missionRuntimeCompleteness(runtimeSnapshot, runtimeMark), Mark: runtimeMark},
	}
	services := missionServiceHealth(now, wip, wipMark, rosterMark, authorityMark, runtimeSnapshot, runtimeMark)
	complete, reasons := true, []string{}
	for _, source := range sources {
		if !source.Completeness.Complete || source.Mark.Freshness == FreshnessUnavailable {
			complete = false
			reasons = append(reasons, source.SourceID+": "+missionSourceReason(source))
		}
	}
	operational := "healthy"
	for _, service := range services {
		if service.OperationalStatus == "unavailable" {
			operational = "unavailable"
			break
		}
		if service.OperationalStatus != "healthy" || service.Mark.Freshness != FreshnessCurrent {
			operational = "degraded"
		}
	}
	if !wip.Completeness.Complete && operational == "healthy" {
		operational = "degraded"
	}
	derivation := missionAggregateMark(now, sources)
	projection := MissionControlProjection{
		SchemaVersion: MissionControlSchemaVersion, GeneratedAt: now,
		DerivationState: derivation, OperationalStatus: operational,
		Completeness: MissionCompleteness{
			Complete: complete, Reasons: compactStrings(reasons), SourceEventGraphHead: wip.Completeness.SourceEventGraphHead,
			StartHead: wip.Completeness.StartHead, EndHead: wip.Completeness.EndHead,
			DomainCounts: cloneMissionIntMap(wip.Completeness.DomainCounts), PageCounts: cloneMissionIntMap(wip.Completeness.PageCounts),
		},
		Sources: sources, Services: services, WIP: append([]WIPItem(nil), wip.Rows...),
		Roles: roles, WorkerPool: workerPool, HumanActions: actions,
		Interventions: append([]MissionIntervention(nil), wip.Interventions...), Handoffs: append([]Handoff(nil), wip.Handoffs...),
		ResidualRisks: []string{
			"daemon loopback runtime observation is unavailable unless separately configured and running",
			"head changes during exhaustive reads make the affected source projected-only or stale",
			"process-local stale retention is lost on restart and never exceeds 15 minutes from original observation",
			"historical or unbound classification evidence fails upward to TLC 4.5.0 P-ENVELOPE Tier 3",
			"spawn/stop events prove event-active state, not general Agent process liveness",
			"Work HTTP reachability is a separate Site-owned observation and does not prove EventGraph head equality",
		},
		NonAuthorizations: []string{
			"Mission Control is read-only and grants no approval, gate, merge, deploy, runtime, configuration, authority, or production action.",
			"Cached or projected-only data cannot satisfy a TLC gate or authorize a transition.",
			"No EventGraph or Work mutation endpoint is part of this projection.",
		},
	}
	if projection.WIP == nil {
		projection.WIP = []WIPItem{}
	}
	if projection.Roles == nil {
		projection.Roles = []RoleAgentRow{}
	}
	if projection.HumanActions == nil {
		projection.HumanActions = []HumanAction{}
	}
	if projection.Interventions == nil {
		projection.Interventions = []MissionIntervention{}
	}
	if projection.Handoffs == nil {
		projection.Handoffs = []Handoff{}
	}
	return projection
}

func missionSeverityRank(value string) int {
	switch value {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func missionSourceReason(source SourceEnvelope) string {
	if source.Mark.Reason != "" {
		return source.Mark.Reason
	}
	if len(source.Completeness.Reasons) > 0 {
		return strings.Join(source.Completeness.Reasons, "; ")
	}
	return "source incomplete"
}

func cloneMissionIntMap(input map[string]int) map[string]int {
	if input == nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func missionHeadIdentity(s store.Store) (string, error) {
	head, err := s.Head()
	if err != nil {
		return "", err
	}
	if head.IsNone() {
		return "empty", nil
	}
	return head.Unwrap().ID().Value(), nil
}

func missionEventsByType(s store.Store, eventType types.EventType, pageSize int) ([]event.Event, int, error) {
	if s == nil {
		return nil, 0, errors.New("store is required")
	}
	if pageSize <= 0 {
		pageSize = missionControlDefaultPageSize
	}
	cursor := types.None[types.Cursor]()
	var out []event.Event
	pages := 0
	for {
		page, err := s.ByType(eventType, pageSize, cursor)
		if err != nil {
			return out, pages, err
		}
		pages++
		out = append(out, page.Items()...)
		if !page.HasMore() {
			return out, pages, nil
		}
		next := page.Cursor()
		if next.IsNone() || (!cursor.IsNone() && next.Unwrap().Value() == cursor.Unwrap().Value()) {
			return out, pages, fmt.Errorf("%s pagination returned no advancing cursor", eventType.Value())
		}
		cursor = next
	}
}

func (p *CivilizationMissionControlProjector) acquireWIP(ctx context.Context, now time.Time) (missionWIPSource, EvidenceMark) {
	value, err := p.buildWIPSource(ctx, now)
	if err == nil && value.Completeness.Complete {
		mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "eventgraph_wip_evidence", value.GeneratedAt, now, []string{value.Completeness.SourceEventGraphHead}, "")
		p.mu.Lock()
		p.wipCache = cachedMissionSource[missionWIPSource]{value: value, observedAt: value.GeneratedAt, valid: true}
		p.mu.Unlock()
		return value, mark
	}
	reason := "WIP evidence acquisition incomplete"
	if err != nil {
		reason = reason + ": " + err.Error()
	} else if len(value.Completeness.Reasons) > 0 {
		reason = reason + ": " + strings.Join(value.Completeness.Reasons, "; ")
	}
	p.mu.Lock()
	cache := p.wipCache
	p.mu.Unlock()
	if cache.valid && !now.Before(cache.observedAt) && now.Sub(cache.observedAt) <= p.retention {
		stale := staleMissionWIPSource(cache.value, now, reason)
		return stale, NewEvidenceMark(FreshnessStale, BasisExact, "eventgraph_wip_evidence", cache.observedAt, now, []string{cache.value.Completeness.SourceEventGraphHead}, reason)
	}
	if value.Completeness.DomainCounts == nil {
		value = emptyMissionWIPSource(now, reason)
	}
	return value, missionUnavailableMark("eventgraph_wip_evidence", now, reason)
}

func (p *CivilizationMissionControlProjector) acquireRoster(ctx context.Context, now time.Time) (missionRosterSource, EvidenceMark) {
	value, err := p.buildRosterSource(ctx, now)
	if err == nil && value.Completeness.Complete {
		mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "roster_routing", value.GeneratedAt, now, []string{value.Completeness.SourceEventGraphHead}, "")
		p.mu.Lock()
		p.rosterCache = cachedMissionSource[missionRosterSource]{value: value, observedAt: value.GeneratedAt, valid: true}
		p.mu.Unlock()
		return value, mark
	}
	reason := "roster evidence acquisition incomplete"
	if err != nil {
		reason += ": " + err.Error()
	} else if len(value.Completeness.Reasons) > 0 {
		reason += ": " + strings.Join(value.Completeness.Reasons, "; ")
	}
	p.mu.Lock()
	cache := p.rosterCache
	p.mu.Unlock()
	if cache.valid && !now.Before(cache.observedAt) && now.Sub(cache.observedAt) <= p.retention {
		stale := staleMissionRosterSource(cache.value, now, reason)
		return stale, NewEvidenceMark(FreshnessStale, BasisExact, "roster_routing", cache.observedAt, now, []string{cache.value.Completeness.SourceEventGraphHead}, reason)
	}
	if value.Completeness.DomainCounts == nil {
		value = emptyMissionRosterSource(now, reason)
	}
	return value, missionUnavailableMark("roster_routing", now, reason)
}

func (p *CivilizationMissionControlProjector) acquireAuthority(ctx context.Context, now time.Time) (missionAuthoritySource, EvidenceMark) {
	value, err := p.buildAuthoritySource(ctx, now)
	if err == nil && value.Completeness.Complete {
		mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "authority_actions", value.GeneratedAt, now, []string{value.Completeness.SourceEventGraphHead}, "")
		p.mu.Lock()
		p.authorityCache = cachedMissionSource[missionAuthoritySource]{value: value, observedAt: value.GeneratedAt, valid: true}
		p.mu.Unlock()
		return value, mark
	}
	reason := "authority evidence acquisition incomplete"
	if err != nil {
		reason += ": " + err.Error()
	} else if len(value.Completeness.Reasons) > 0 {
		reason += ": " + strings.Join(value.Completeness.Reasons, "; ")
	}
	p.mu.Lock()
	cache := p.authorityCache
	p.mu.Unlock()
	if cache.valid && !now.Before(cache.observedAt) && now.Sub(cache.observedAt) <= p.retention {
		stale := staleMissionAuthoritySource(cache.value, now, reason)
		return stale, NewEvidenceMark(FreshnessStale, BasisExact, "authority_actions", cache.observedAt, now, []string{cache.value.Completeness.SourceEventGraphHead}, reason)
	}
	if value.Completeness.DomainCounts == nil {
		value = emptyMissionAuthoritySource(now, reason)
	}
	return value, missionUnavailableMark("authority_actions", now, reason)
}

func (p *CivilizationMissionControlProjector) acquireRuntime(ctx context.Context, now time.Time) (FactoryRuntimeSnapshot, EvidenceMark) {
	p.mu.Lock()
	cache := p.runtimeCache
	p.mu.Unlock()
	var previous *FactoryRuntimeSnapshot
	if cache.valid {
		copy := cache.value
		previous = &copy
	}
	var snapshot FactoryRuntimeSnapshot
	var err error
	if p.runtime == nil {
		err = errors.New("factory runtime source is not configured")
	} else {
		snapshot, err = p.runtime.Fetch(ctx, now, previous)
		if err == nil && !FactoryRuntimeSnapshotCurrentHealthy(snapshot, now) {
			err = errors.New("factory runtime snapshot is not current and healthy")
		}
	}
	if err == nil {
		mark := NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "factory_runtime", snapshot.LastHeartbeatAt, now, []string{snapshot.BootID, strconv.FormatUint(snapshot.Sequence, 10)}, "ephemeral daemon observation; not durable TLC evidence")
		p.mu.Lock()
		p.runtimeCache = cachedMissionSource[FactoryRuntimeSnapshot]{value: snapshot, observedAt: snapshot.LastHeartbeatAt, valid: true}
		p.mu.Unlock()
		return snapshot, mark
	}
	reason := "factory runtime unavailable: " + err.Error()
	if cache.valid && !now.Before(cache.observedAt) && now.Sub(cache.observedAt) <= p.retention {
		return cache.value, NewEvidenceMark(FreshnessStale, BasisProjectedOnly, "factory_runtime", cache.observedAt, now, []string{cache.value.BootID, strconv.FormatUint(cache.value.Sequence, 10)}, reason)
	}
	return FactoryRuntimeSnapshot{SchemaVersion: FactoryRuntimeSnapshotSchemaVersion, Assignments: []factoryv1.RuntimeAssignment{}}, missionUnavailableMark("factory_runtime", now, reason)
}

func emptyMissionWIPSource(now time.Time, reason string) missionWIPSource {
	return missionWIPSource{GeneratedAt: now, Rows: []WIPItem{}, Interventions: []MissionIntervention{}, Handoffs: []Handoff{}, HumanActions: []HumanAction{}, Completeness: MissionCompleteness{Complete: false, Reasons: []string{reason}, DomainCounts: map[string]int{}, PageCounts: map[string]int{}}}
}

func emptyMissionRosterSource(now time.Time, reason string) missionRosterSource {
	return missionRosterSource{GeneratedAt: now, Rows: []RoleAgentRow{}, Completeness: MissionCompleteness{Complete: false, Reasons: []string{reason}, DomainCounts: map[string]int{}, PageCounts: map[string]int{}}}
}

func emptyMissionAuthoritySource(now time.Time, reason string) missionAuthoritySource {
	return missionAuthoritySource{GeneratedAt: now, HumanActions: []HumanAction{}, Completeness: MissionCompleteness{Complete: false, Reasons: []string{reason}, DomainCounts: map[string]int{}, PageCounts: map[string]int{}}}
}

func staleMissionMark(mark EvidenceMark, now time.Time, reason string) EvidenceMark {
	if mark.Freshness == FreshnessUnavailable || mark.Basis == BasisUnavailable {
		return mark
	}
	return NewEvidenceMark(FreshnessStale, mark.Basis, mark.SourceID, mark.ObservedAt, now, mark.EvidenceRefs, reason)
}

func staleMarkedValue(value MarkedValue, now time.Time, reason string) MarkedValue {
	value.Mark = staleMissionMark(value.Mark, now, reason)
	return value
}

func staleMissionWIPSource(value missionWIPSource, now time.Time, reason string) missionWIPSource {
	for i := range value.Rows {
		row := &value.Rows[i]
		row.TargetRepository = staleMarkedValue(row.TargetRepository, now, reason)
		row.Assignment = staleMarkedValue(row.Assignment, now, reason)
		row.LifecycleStatus = staleMarkedValue(row.LifecycleStatus, now, reason)
		row.EngineProtocol = staleMarkedValue(row.EngineProtocol, now, reason)
		row.TLCStage = staleMarkedValue(row.TLCStage, now, reason)
		row.TLCStageIndex = staleMarkedValue(row.TLCStageIndex, now, reason)
		row.ItemStartedAt = staleMarkedValue(row.ItemStartedAt, now, reason)
		row.LastEffectAt = staleMarkedValue(row.LastEffectAt, now, reason)
		row.ElapsedMS = staleMarkedValue(row.ElapsedMS, now, reason)
		row.NextHandoff = staleMarkedValue(row.NextHandoff, now, reason)
		row.Completeness = staleMarkedValue(row.Completeness, now, reason)
		row.Classification.Mark = staleMissionMark(row.Classification.Mark, now, reason)
		row.EvidenceRollup.Mark = staleMissionMark(row.EvidenceRollup.Mark, now, reason)
		for key, mark := range row.EvidenceRollup.FieldMarks {
			row.EvidenceRollup.FieldMarks[key] = staleMissionMark(mark, now, reason)
		}
		for j := range row.EvidenceRollup.Items {
			row.EvidenceRollup.Items[j].Mark = staleMissionMark(row.EvidenceRollup.Items[j].Mark, now, reason)
		}
		row.Mark = staleMissionMark(row.Mark, now, reason)
	}
	for i := range value.Interventions {
		value.Interventions[i].Mark = staleMissionMark(value.Interventions[i].Mark, now, reason)
	}
	for i := range value.Handoffs {
		value.Handoffs[i].Mark = staleMissionMark(value.Handoffs[i].Mark, now, reason)
	}
	for i := range value.HumanActions {
		value.HumanActions[i].Mark = staleMissionMark(value.HumanActions[i].Mark, now, reason)
	}
	return value
}

func staleMissionRosterSource(value missionRosterSource, now time.Time, reason string) missionRosterSource {
	for i := range value.Rows {
		row := &value.Rows[i]
		row.Configured = staleMarkedValue(row.Configured, now, reason)
		row.Instantiated = staleMarkedValue(row.Instantiated, now, reason)
		row.EventActive = staleMarkedValue(row.EventActive, now, reason)
		row.Running = staleMarkedValue(row.Running, now, reason)
		row.Provider = staleMarkedValue(row.Provider, now, reason)
		row.Model = staleMarkedValue(row.Model, now, reason)
		row.Authority = staleMarkedValue(row.Authority, now, reason)
		row.Capacity = staleMarkedValue(row.Capacity, now, reason)
		row.Status = staleMarkedValue(row.Status, now, reason)
		row.Assignment = staleMarkedValue(row.Assignment, now, reason)
		row.Mark = staleMissionMark(row.Mark, now, reason)
	}
	return value
}

func staleMissionAuthoritySource(value missionAuthoritySource, now time.Time, reason string) missionAuthoritySource {
	for i := range value.HumanActions {
		value.HumanActions[i].Mark = staleMissionMark(value.HumanActions[i].Mark, now, reason)
	}
	return value
}

type missionWorkTaskFold struct {
	created               event.Event
	content               work.TaskCreatedContent
	assignment            *event.Event
	assignedTo            string
	lifecycle             *event.Event
	lifecycleStatus       work.TaskStatus
	link                  *event.Event
	linkedOrderIDs        map[string]struct{}
	targetRepositories    map[string]event.Event
	completed             map[string]event.Event
	supersededCompletions map[string]struct{}
	latestAt              time.Time
	evidenceRefs          []string
	decodeConflict        bool
}

var missionWorkEventTypes = []types.EventType{
	work.EventTypeTaskCreated,
	work.EventTypeTaskAssigned,
	work.EventTypeTaskCompleted,
	work.EventTypeTaskReopened,
	work.EventTypeTaskDependencyAdded,
	work.EventTypeTaskPrioritySet,
	work.EventTypeTaskComment,
	work.EventTypeTaskUnblocked,
	work.EventTypeTaskArtifact,
	work.EventTypeTaskArtifactWaived,
	work.EventTypeTaskLifecycleTransitioned,
	work.EventTypeTaskLinked,
	work.EventTypeTaskVerificationAttached,
	work.EventTypeTaskFailureRepairAttached,
	work.EventTypeTaskFactRequired,
	work.EventTypeIssueScanStageBlocked,
	work.EventTypeIssueScanStageGateSatisfied,
	work.EventTypeRuntimeEnvelopeRecorded,
	work.EventTypeRuntimeResultRecorded,
}

func (p *CivilizationMissionControlProjector) buildWIPSource(ctx context.Context, now time.Time) (missionWIPSource, error) {
	result := emptyMissionWIPSource(now, "")
	result.Completeness.Reasons = nil
	startHead, err := missionHeadIdentity(p.store)
	if err != nil {
		return result, fmt.Errorf("read WIP start head: %w", err)
	}
	result.Completeness.StartHead = startHead
	result.Completeness.SourceEventGraphHead = startHead
	allEvents := make(map[types.EventType][]event.Event, len(missionWorkEventTypes))
	for _, eventType := range missionWorkEventTypes {
		events, pages, readErr := missionEventsByType(p.store, eventType, p.pageSize)
		key := eventType.Value()
		result.Completeness.DomainCounts[key] = len(events)
		result.Completeness.PageCounts[key] = pages
		allEvents[eventType] = events
		if readErr != nil {
			result.Completeness.Reasons = append(result.Completeness.Reasons, fmt.Sprintf("read %s: %v", key, readErr))
		}
	}

	var factoryProjection factoryv1.Projection
	if p.factory == nil {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "Factory v1 projection source is not configured")
	} else {
		factoryProjection, err = p.factory(ctx)
		if err != nil {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "Factory v1 projection: "+err.Error())
		}
	}
	result.Completeness.DomainCounts["factory_v1_orders"] = len(factoryProjection.Orders)
	result.Completeness.DomainCounts["factory_v1_interventions"] = len(factoryProjection.Interventions)
	result.Completeness.PageCounts["factory_v1_projection"] = 1
	result.FactoryService = factoryProjection.Service

	folds := make(map[string]*missionWorkTaskFold)
	for _, ev := range allEvents[work.EventTypeTaskCreated] {
		content, ok := ev.Content().(work.TaskCreatedContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+work.EventTypeTaskCreated.Value()+" "+ev.ID().Value())
			continue
		}
		id := ev.ID().Value()
		fold := &missionWorkTaskFold{created: ev, content: content, linkedOrderIDs: map[string]struct{}{}, targetRepositories: map[string]event.Event{}, completed: map[string]event.Event{}, supersededCompletions: map[string]struct{}{}, latestAt: ev.Timestamp().Value(), evidenceRefs: []string{id}}
		if strings.TrimSpace(content.FactoryOrderID) != "" {
			fold.linkedOrderIDs[strings.TrimSpace(content.FactoryOrderID)] = struct{}{}
		}
		folds[id] = fold
	}
	apply := func(eventType types.EventType, fn func(*missionWorkTaskFold, event.Event) bool) {
		for _, ev := range allEvents[eventType] {
			var taskID string
			switch content := ev.Content().(type) {
			case work.TaskAssignedContent:
				taskID = content.TaskID.Value()
			case work.TaskCompletedContent:
				taskID = content.TaskID.Value()
			case work.TaskReopenedContent:
				taskID = content.TaskID.Value()
			case work.TaskDependencyContent:
				taskID = content.TaskID.Value()
			case work.TaskPrioritySetContent:
				taskID = content.TaskID.Value()
			case work.CommentContent:
				taskID = content.TaskID.Value()
			case work.TaskUnblockedContent:
				taskID = content.TaskID.Value()
			case work.TaskArtifactContent:
				taskID = content.TaskID.Value()
			case work.TaskArtifactWaivedContent:
				taskID = content.TaskID.Value()
			case work.TaskLifecycleTransitionContent:
				taskID = content.TaskID.Value()
			case work.TaskLinkedContent:
				taskID = content.TaskID.Value()
			case work.TaskVerificationAttachedContent:
				taskID = content.TaskID.Value()
			case work.TaskFailureRepairAttachedContent:
				taskID = content.TaskID.Value()
			case work.TaskFactRequiredContent:
				taskID = content.TaskID.Value()
			case work.IssueScanStageBlockedContent:
				taskID = content.TaskID.Value()
			case work.IssueScanStageGateSatisfiedContent:
				taskID = content.TaskID.Value()
			case work.RuntimeEnvelopeRecordedContent:
				taskID = content.Envelope.TaskID.Value()
			case work.RuntimeResultRecordedContent:
				taskID = content.Result.TaskID.Value()
			default:
				result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+eventType.Value()+" "+ev.ID().Value())
				continue
			}
			fold, ok := folds[taskID]
			if !ok {
				result.Completeness.Reasons = append(result.Completeness.Reasons, eventType.Value()+" references unknown task "+taskID)
				continue
			}
			if !fn(fold, ev) {
				fold.decodeConflict = true
				result.Completeness.Reasons = append(result.Completeness.Reasons, "fold "+eventType.Value()+" "+ev.ID().Value())
				continue
			}
			if ev.Timestamp().Value().After(fold.latestAt) {
				fold.latestAt = ev.Timestamp().Value()
			}
			fold.evidenceRefs = append(fold.evidenceRefs, ev.ID().Value())
		}
	}
	apply(work.EventTypeTaskAssigned, func(f *missionWorkTaskFold, ev event.Event) bool {
		content, ok := ev.Content().(work.TaskAssignedContent)
		if !ok {
			return false
		}
		if f.assignment == nil || ev.Timestamp().Value().After(f.assignment.Timestamp().Value()) {
			copy := ev
			f.assignment = &copy
			f.assignedTo = content.AssignedTo.Value()
		}
		return true
	})
	apply(work.EventTypeTaskCompleted, func(f *missionWorkTaskFold, ev event.Event) bool {
		_, ok := ev.Content().(work.TaskCompletedContent)
		if ok {
			f.completed[ev.ID().Value()] = ev
		}
		return ok
	})
	apply(work.EventTypeTaskReopened, func(f *missionWorkTaskFold, ev event.Event) bool {
		content, ok := ev.Content().(work.TaskReopenedContent)
		if !ok {
			return false
		}
		for _, ref := range content.CompletionRefs {
			f.supersededCompletions[ref.Value()] = struct{}{}
		}
		return true
	})
	apply(work.EventTypeTaskLifecycleTransitioned, func(f *missionWorkTaskFold, ev event.Event) bool {
		content, ok := ev.Content().(work.TaskLifecycleTransitionContent)
		if !ok {
			return false
		}
		if f.lifecycle == nil || ev.Timestamp().Value().After(f.lifecycle.Timestamp().Value()) {
			copy := ev
			f.lifecycle = &copy
			f.lifecycleStatus = content.ToState
		}
		return true
	})
	apply(work.EventTypeTaskLinked, func(f *missionWorkTaskFold, ev event.Event) bool {
		content, ok := ev.Content().(work.TaskLinkedContent)
		if !ok {
			return false
		}
		if strings.TrimSpace(content.FactoryOrderID) != "" {
			f.linkedOrderIDs[strings.TrimSpace(content.FactoryOrderID)] = struct{}{}
		}
		if f.link == nil || ev.Timestamp().Value().After(f.link.Timestamp().Value()) {
			copy := ev
			f.link = &copy
		}
		return true
	})
	for _, eventType := range []types.EventType{work.EventTypeTaskDependencyAdded, work.EventTypeTaskPrioritySet, work.EventTypeTaskComment, work.EventTypeTaskUnblocked, work.EventTypeTaskArtifact, work.EventTypeTaskArtifactWaived, work.EventTypeTaskVerificationAttached, work.EventTypeTaskFailureRepairAttached, work.EventTypeTaskFactRequired, work.EventTypeRuntimeEnvelopeRecorded, work.EventTypeRuntimeResultRecorded} {
		apply(eventType, func(_ *missionWorkTaskFold, _ event.Event) bool { return true })
	}
	for _, eventType := range []types.EventType{work.EventTypeIssueScanStageBlocked, work.EventTypeIssueScanStageGateSatisfied} {
		apply(eventType, func(f *missionWorkTaskFold, ev event.Event) bool {
			repository := ""
			switch content := ev.Content().(type) {
			case work.IssueScanStageBlockedContent:
				repository = strings.TrimSpace(content.TargetRepo)
			case work.IssueScanStageGateSatisfiedContent:
				repository = strings.TrimSpace(content.TargetRepo)
			default:
				return false
			}
			if repository != "" {
				f.targetRepositories[repository] = ev
			}
			return true
		})
	}

	ordersByID := make(map[string][]int)
	activeVersions := make(map[string]int)
	tupleCounts := make(map[string]int)
	for i, order := range factoryProjection.Orders {
		ordersByID[order.OrderID] = append(ordersByID[order.OrderID], i)
		if !missionFactoryOrderTerminal(order) {
			activeVersions[order.OrderID]++
			tupleCounts[missionFactoryTuple(order)]++
		}
	}
	tasksByOrder := make(map[string][]*missionWorkTaskFold)
	for _, fold := range folds {
		if len(fold.linkedOrderIDs) == 1 {
			for orderID := range fold.linkedOrderIDs {
				if len(ordersByID[orderID]) == 1 {
					tasksByOrder[orderID] = append(tasksByOrder[orderID], fold)
				}
			}
		}
	}
	usedTasks := map[string]struct{}{}
	tupleOrdinals := make(map[string]int)
	for _, order := range factoryProjection.Orders {
		if missionFactoryOrderTerminal(order) {
			continue
		}
		tuple := missionFactoryTuple(order)
		tupleOrdinals[tuple]++
		var linked *missionWorkTaskFold
		conflictReason := ""
		if tupleCounts[tuple] > 1 {
			conflictReason = "duplicate Factory order tuple"
		} else if activeVersions[order.OrderID] > 1 {
			conflictReason = "multiple active Factory order versions"
		}
		if len(tasksByOrder[order.OrderID]) == 1 && conflictReason == "" {
			linked = tasksByOrder[order.OrderID][0]
			usedTasks[linked.created.ID().Value()] = struct{}{}
		} else if len(tasksByOrder[order.OrderID]) > 1 {
			conflictReason = "multiple Work tasks link to this Factory order"
		}
		if linked != nil {
			if len(linked.targetRepositories) > 1 {
				conflictReason = "linked Work task contains conflicting exact target repositories"
			} else {
				for repository := range linked.targetRepositories {
					if repository != order.TargetRepository {
						conflictReason = "linked Work target repository conflicts with canonical FactoryOrder"
					}
				}
			}
		}
		row, actions, handoff := missionFactoryWIPRow(order, linked, now, conflictReason)
		if tupleCounts[tuple] > 1 {
			row.StableID += ":duplicate:" + strconv.Itoa(tupleOrdinals[tuple])
		}
		result.Rows = append(result.Rows, row)
		result.HumanActions = append(result.HumanActions, actions...)
		result.Handoffs = append(result.Handoffs, handoff)
	}
	for _, fold := range folds {
		if _, used := usedTasks[fold.created.ID().Value()]; used {
			continue
		}
		status, terminal, statusMark := missionWorkStatus(fold, now)
		if terminal {
			continue
		}
		conflictReason := ""
		if len(fold.linkedOrderIDs) > 1 {
			conflictReason = "Work task contains conflicting FactoryOrderID links"
		}
		if len(fold.linkedOrderIDs) == 1 {
			for orderID := range fold.linkedOrderIDs {
				if len(ordersByID[orderID]) == 0 {
					conflictReason = "Work task references unavailable Factory order " + orderID
				}
				if len(ordersByID[orderID]) > 1 {
					conflictReason = "Work task references ambiguous Factory order " + orderID
				}
				if len(tasksByOrder[orderID]) > 1 {
					conflictReason = "multiple Work tasks link to Factory order " + orderID
				}
			}
		}
		if len(fold.targetRepositories) > 1 {
			if conflictReason != "" {
				conflictReason += "; "
			}
			conflictReason += "Work task contains conflicting exact target repositories"
		}
		result.Rows = append(result.Rows, missionIndependentWorkWIPRow(fold, status, statusMark, now, conflictReason))
	}

	for _, intervention := range factoryProjection.Interventions {
		mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", intervention.RequestedAt, now, []string{intervention.EventID}, "")
		result.Interventions = append(result.Interventions, MissionIntervention{InterventionID: intervention.InterventionID, OrderID: intervention.OrderID, Kind: intervention.Kind, Status: string(intervention.Status), Prompt: boundedMissionReason(intervention.Prompt), RequestedAt: intervention.RequestedAt, EvidenceRefs: []string{intervention.EventID}, Mark: mark})
		for i := range result.Rows {
			if result.Rows[i].FactoryOrderID != intervention.OrderID {
				continue
			}
			result.Rows[i].InterventionRefs = compactStrings(append(result.Rows[i].InterventionRefs, intervention.InterventionID, intervention.EventID))
			if intervention.Status == factoryv1.InterventionOpen {
				result.Rows[i].BlockerRefs = compactStrings(append(result.Rows[i].BlockerRefs, intervention.EventID))
				result.Rows[i].Completeness = missionMarked(false, NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "factory_v1_ledger", intervention.RequestedAt, now, []string{intervention.EventID}, "open Factory intervention blocks a complete normal handoff"))
			}
		}
		if intervention.Status == factoryv1.InterventionOpen {
			for i := range result.Handoffs {
				if result.Handoffs[i].SubjectID == intervention.OrderID {
					result.Handoffs[i].Blocked = true
					result.Handoffs[i].ToStage = "intervention_resolution"
					result.Handoffs[i].EvidenceRefs = compactStrings(append(result.Handoffs[i].EvidenceRefs, intervention.EventID))
				}
			}
			result.HumanActions = append(result.HumanActions, HumanAction{ActionID: "intervention:" + intervention.InterventionID, Kind: "factory_intervention", Severity: "high", SubjectID: intervention.OrderID, Summary: boundedMissionReason(intervention.Prompt), RequiredAction: "Resolve the open Factory intervention through its governed authority path.", SourceTime: intervention.RequestedAt, EvidenceRefs: []string{intervention.EventID}, Mark: mark})
		}
	}
	endHead, headErr := missionHeadIdentity(p.store)
	if headErr != nil {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "read WIP end head: "+headErr.Error())
	}
	result.Completeness.EndHead = endHead
	if startHead != endHead {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "EventGraph head changed during WIP enumeration")
		for i := range result.Rows {
			result.Rows[i] = projectedOnlyWIPRow(result.Rows[i], now, "EventGraph head changed during WIP enumeration")
		}
		for i := range result.Interventions {
			result.Interventions[i].Mark = projectedOnlyMissionMark(result.Interventions[i].Mark, now, "EventGraph head changed during WIP enumeration")
		}
		for i := range result.Handoffs {
			result.Handoffs[i].Mark = projectedOnlyMissionMark(result.Handoffs[i].Mark, now, "EventGraph head changed during WIP enumeration")
		}
		for i := range result.HumanActions {
			result.HumanActions[i].Mark = projectedOnlyMissionMark(result.HumanActions[i].Mark, now, "EventGraph head changed during WIP enumeration")
		}
	}
	result.Completeness.Complete = len(result.Completeness.Reasons) == 0
	result.Completeness.Reasons = compactStrings(result.Completeness.Reasons)
	sort.Slice(result.Rows, func(i, j int) bool { return result.Rows[i].StableID < result.Rows[j].StableID })
	sort.Slice(result.Interventions, func(i, j int) bool {
		return result.Interventions[i].InterventionID < result.Interventions[j].InterventionID
	})
	sort.Slice(result.Handoffs, func(i, j int) bool { return result.Handoffs[i].HandoffID < result.Handoffs[j].HandoffID })
	sort.Slice(result.HumanActions, func(i, j int) bool { return result.HumanActions[i].ActionID < result.HumanActions[j].ActionID })
	return result, nil
}

func missionFactoryTuple(order factoryv1.OrderProjection) string {
	return order.OrderID + "\x00" + order.Version + "\x00" + order.DocumentSHA256
}

func missionFactoryOrderTerminal(order factoryv1.OrderProjection) bool {
	if len(order.Stages) == 0 {
		return false
	}
	latest := order.Stages[len(order.Stages)-1]
	return latest.Stage == factoryv1.StageHumanReview && latest.State == factoryv1.TransitionPassed
}

func missionWorkStatus(fold *missionWorkTaskFold, now time.Time) (string, bool, EvidenceMark) {
	refs := compactStrings(fold.evidenceRefs)
	if fold.decodeConflict {
		return "unavailable", false, missionUnavailableMark("work_eventgraph", now, "Work fold contains invalid evidence")
	}
	if fold.lifecycle != nil {
		if !missionKnownWorkStatus(fold.lifecycleStatus) {
			return "unavailable", false, missionUnavailableMark("work_eventgraph", now, "unknown Work lifecycle state "+string(fold.lifecycleStatus))
		}
		status := string(fold.lifecycleStatus)
		terminal := fold.lifecycleStatus == work.StatusCertified || fold.lifecycleStatus == work.StatusRejected || fold.lifecycleStatus == work.StatusSuperseded
		return status, terminal, NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.lifecycle.Timestamp().Value(), now, refs, "")
	}
	liveCompletion := false
	var completedAt time.Time
	for id, ev := range fold.completed {
		if _, superseded := fold.supersededCompletions[id]; superseded {
			continue
		}
		liveCompletion = true
		if ev.Timestamp().Value().After(completedAt) {
			completedAt = ev.Timestamp().Value()
		}
	}
	if liveCompletion {
		return string(work.LegacyStatusCompleted), true, NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", completedAt, now, refs, "legacy completion event without explicit v3.9 lifecycle state")
	}
	if fold.assignment != nil {
		return string(work.LegacyStatusAssigned), false, NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.assignment.Timestamp().Value(), now, refs, "legacy assignment state")
	}
	return string(work.StatusCreated), false, NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.created.Timestamp().Value(), now, refs, "")
}

func missionKnownWorkStatus(status work.TaskStatus) bool {
	switch status {
	case work.StatusCreated, work.StatusReady, work.StatusRunning, work.StatusBlocked, work.StatusFailed,
		work.StatusRepairRequired, work.StatusRepairRunning, work.StatusRepaired, work.StatusVerificationRunning,
		work.StatusVerified, work.StatusCertified, work.StatusRejected, work.StatusSuperseded, work.StatusPolicyBlocked:
		return true
	default:
		return false
	}
}

func missionElapsed(start, now time.Time, source string, refs []string) MarkedValue {
	if start.IsZero() || start.After(now) {
		return missionMarked(nil, missionUnavailableMark(source, now, "start time is missing or in the future"))
	}
	return missionMarked(now.Sub(start).Milliseconds(), NewEvidenceMark(FreshnessCurrent, BasisExact, source, start, now, refs, "computed from exact source time and projection clock"))
}

func missionFactoryWIPRow(order factoryv1.OrderProjection, linked *missionWorkTaskFold, now time.Time, conflictReason string) (WIPItem, []HumanAction, Handoff) {
	refs := []string{order.DocumentSHA256}
	mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", order.LastEffectAt, now, refs, conflictReason)
	assignment := ""
	assignmentMark := missionUnavailableMark("factory_v1_ledger", now, "no active Factory runner or exact Work assignment")
	workID := ""
	status := order.Status
	statusMark := mark
	if linked != nil {
		workID = linked.created.ID().Value()
		if linked.assignedTo != "" {
			assignment = linked.assignedTo
			assignmentMark = NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", linked.assignment.Timestamp().Value(), now, []string{linked.assignment.ID().Value()}, "")
		}
		workStatus, _, workMark := missionWorkStatus(linked, now)
		status = order.Status + " / work:" + workStatus
		statusMark = workMark
	}
	if order.ActivelyExecuting && order.ActiveAttemptID != "" {
		assignment = order.ActiveAttemptID
		assignmentMark = mark
	}
	blockers := []string{}
	if order.Blocker != "" {
		blockers = append(blockers, order.Blocker)
	}
	if conflictReason != "" {
		blockers = append(blockers, conflictReason)
	}
	rowComplete := len(blockers) == 0
	completenessMark := mark
	if !rowComplete {
		completenessMark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "factory_v1_ledger", order.LastEffectAt, now, refs, strings.Join(blockers, "; "))
	}
	rollup := missionFactoryEvidenceRollup(order, now)
	next := order.NextAction
	if conflictReason != "" {
		next = "repair conflicting evidence before any normal handoff"
	}
	handoffMark := mark
	if next == "" {
		handoffMark = missionUnavailableMark("factory_v1_ledger", now, "no validated next handoff")
	}
	row := WIPItem{
		Kind: "factory_order", StableID: "factory:" + order.OrderID + "@" + order.Version + ":" + order.DocumentSHA256,
		FactoryOrderID: order.OrderID, FactoryOrderVersion: order.Version, DocumentSHA256: order.DocumentSHA256, WorkTaskID: workID, Title: order.Title,
		TargetRepository: missionMarked(order.TargetRepository, mark), Assignment: missionMarked(assignment, assignmentMark), LifecycleStatus: missionMarked(status, statusMark),
		EngineProtocol: missionMarked(order.EngineProtocol, mark), TLCStage: missionMarked(string(order.TLCStage), mark), TLCStageIndex: missionMarked(order.TLCIndex, mark),
		ItemStartedAt: missionMarked(order.StartedAt, mark), LastEffectAt: missionMarked(order.LastEffectAt, mark), ElapsedMS: missionElapsed(order.StartedAt, now, "factory_v1_ledger", refs),
		NextHandoff: missionMarked(next, handoffMark), Completeness: missionMarked(rowComplete, completenessMark),
		Classification: classifyMissionOrder(order, now), BlockerRefs: compactStrings(blockers), EvidenceRollup: rollup, Mark: mark,
	}
	actions := []HumanAction{}
	if order.Status == "human_review" || order.TLCStage == factoryv1.StageHumanReview {
		actions = append(actions, HumanAction{ActionID: "human-review:" + order.OrderID + "@" + order.Version, Kind: "human_review", Severity: "high", OwningStage: string(factoryv1.StageHumanReview), SubjectID: order.OrderID, Summary: "Exact-head PR is waiting for Tier 3 Human Review.", RequiredAction: "Review the merge-ready PR and explicitly approve, reject, or request changes; this view grants no merge authority.", SourceTime: order.LastEffectAt, EvidenceRefs: rollup.Mark.EvidenceRefs, Mark: mark})
	} else if order.TLCStage == factoryv1.StageHumanDesignReview && (order.Status == "human_required" || order.Status == "blocked") {
		actions = append(actions, HumanAction{ActionID: "human-design-review:" + order.OrderID + "@" + order.Version, Kind: "human_design_review", Severity: "high", OwningStage: string(factoryv1.StageHumanDesignReview), SubjectID: order.OrderID, Summary: "Design is waiting for scoped Human approval.", RequiredAction: "Approve or reject the exact design blob; approval authorizes implementation only.", SourceTime: order.LastEffectAt, EvidenceRefs: rollup.Mark.EvidenceRefs, Mark: mark})
	}
	handoff := Handoff{HandoffID: "handoff:" + order.OrderID + "@" + order.Version, SubjectID: order.OrderID, FromStage: string(order.TLCStage), ToStage: missionNextStage(order), ExpectedRoles: append([]string(nil), order.Peers...), CompletionPredicate: missionCompletionPredicate(order.TLCStage), Blocked: len(blockers) > 0 || order.Status == "blocked", EvidenceRefs: rollup.Mark.EvidenceRefs, Mark: handoffMark}
	if handoff.Blocked {
		handoff.ToStage = "evidence_repair"
	}
	return row, actions, handoff
}

func missionIndependentWorkWIPRow(fold *missionWorkTaskFold, status string, statusMark EvidenceMark, now time.Time, conflictReason string) WIPItem {
	id := fold.created.ID().Value()
	mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.latestAt, now, fold.evidenceRefs, conflictReason)
	assignmentMark := missionUnavailableMark("work_eventgraph", now, "no exact Work assignment")
	assignment := ""
	if fold.assignment != nil {
		assignment = fold.assignedTo
		assignmentMark = NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.assignment.Timestamp().Value(), now, []string{fold.assignment.ID().Value()}, "")
	}
	blockers := []string{}
	if conflictReason != "" {
		blockers = append(blockers, conflictReason)
	}
	if fold.lifecycle != nil {
		switch fold.lifecycleStatus {
		case work.StatusBlocked, work.StatusFailed, work.StatusRepairRequired, work.StatusPolicyBlocked:
			blockers = append(blockers, fold.lifecycle.ID().Value())
		}
	}
	rowComplete := !fold.decodeConflict && conflictReason == "" && statusMark.Freshness != FreshnessUnavailable
	completenessMark := mark
	if !rowComplete {
		reason := "Work item evidence is conflicting or incomplete"
		if statusMark.Reason != "" {
			reason += ": " + statusMark.Reason
		}
		completenessMark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "work_eventgraph", fold.latestAt, now, fold.evidenceRefs, reason)
	}
	targetRepository := any(nil)
	targetRepositoryMark := missionUnavailableMark("work_eventgraph", now, "no exact Work target-repository field")
	if len(fold.targetRepositories) == 1 {
		for repository, sourceEvent := range fold.targetRepositories {
			targetRepository = repository
			targetRepositoryMark = NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", sourceEvent.Timestamp().Value(), now, []string{sourceEvent.ID().Value()}, "typed Work issue-scan target repository evidence")
		}
	}
	return WIPItem{
		Kind: "independent_work_task", StableID: "work:" + id, WorkTaskID: id, Title: fold.content.Title,
		TargetRepository: missionMarked(targetRepository, targetRepositoryMark),
		Assignment:       missionMarked(assignment, assignmentMark), LifecycleStatus: missionMarked(status, statusMark),
		EngineProtocol: missionMarked("work-v3.9", NewEvidenceMark(FreshnessCurrent, BasisInferred, "work_eventgraph", fold.created.Timestamp().Value(), now, []string{id}, "Work event vocabulary identifies the lifecycle family; this is not a TLC ledger")),
		TLCStage:       missionMarked(nil, missionUnavailableMark("work_eventgraph", now, "independent Work task has no exact TLC ledger")), TLCStageIndex: missionMarked(nil, missionUnavailableMark("work_eventgraph", now, "independent Work task has no exact TLC ledger")),
		ItemStartedAt: missionMarked(fold.created.Timestamp().Value(), NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.created.Timestamp().Value(), now, []string{id}, "")),
		LastEffectAt:  missionMarked(fold.latestAt, mark), ElapsedMS: missionElapsed(fold.created.Timestamp().Value(), now, "work_eventgraph", fold.evidenceRefs),
		NextHandoff:    missionMarked(nil, missionUnavailableMark("work_eventgraph", now, "no validated TLC handoff for independent Work task")),
		Completeness:   missionMarked(rowComplete, completenessMark),
		Classification: MissionClassification{EngineProtocol: "work-v3.9", EffectiveGovernanceProtocol: "4.5.0", EffectivePacketProfile: "P-ENVELOPE", EffectiveHumanReviewTier: 3, Mark: NewEvidenceMark(FreshnessCurrent, BasisInferred, "work_eventgraph", fold.latestAt, now, fold.evidenceRefs, "independent Work task has no exact TLC 4.5.0 classification evidence")},
		BlockerRefs:    blockers, InterventionRefs: []string{}, EvidenceRollup: EvidenceRollup{
			Items: []MissionEvidenceItem{}, FieldMarks: missionUnavailableEvidenceFieldMarks("work_eventgraph", now, "independent Work task has no exact Factory/TLC evidence rollup"),
			Mark: missionUnavailableMark("work_eventgraph", now, "independent Work task has no exact Factory/TLC evidence rollup"),
		}, Mark: mark,
	}
}

func missionFactoryEvidenceRollup(order factoryv1.OrderProjection, now time.Time) EvidenceRollup {
	fieldMarks := missionUnavailableEvidenceFieldMarks("factory_v1_ledger", now, "exact evidence field has not been observed")
	factoryOrderMark := missionUnavailableMark("factory_v1_ledger", now, "accepted FactoryOrder document SHA-256 is missing or invalid")
	if isExactGitOrDocumentHash(order.DocumentSHA256) {
		factoryOrderMark = NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", order.StartedAt, now, []string{order.DocumentSHA256}, "accepted canonical FactoryOrder document SHA-256")
	}
	fieldMarks["factory_order_ref"] = factoryOrderMark
	rollup := EvidenceRollup{FactoryOrderRef: order.DocumentSHA256, FieldMarks: fieldMarks, Items: []MissionEvidenceItem{{Kind: "factory_order_document", Stage: string(factoryv1.StageCraftFactoryOrder), State: "accepted", Reference: order.DocumentSHA256, Mark: factoryOrderMark}}}
	refs := []string{order.DocumentSHA256}
	for _, stage := range order.Stages {
		for _, item := range stage.Evidence {
			observed := stage.OccurredAt
			mark := NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", observed, now, []string{item.Reference}, "")
			projected := MissionEvidenceItem{Kind: item.Kind, Stage: string(stage.Stage), State: string(stage.State), Reference: item.Reference, BlobSHA: item.DesignBlobSHA, PRHeadSHA: item.PRHeadSHA, ReviewedHeadSHA: item.ReviewedHeadSHA, BlockerCount: item.BlockerCount, AuthorFamily: item.AuthorFamily, ReviewerFamily: item.ReviewerFamily, Mark: mark}
			if item.Provider != nil {
				projected.ProviderID = item.Provider.ProviderID
			}
			if item.Kind == "factory_order" || item.Kind == "factory_order_document" {
				rollup.FactoryOrderRef = item.Reference
			}
			if item.DesignBlobSHA != "" {
				rollup.DesignBlobSHA = item.DesignBlobSHA
				if isExactGitOrDocumentHash(item.DesignBlobSHA) {
					rollup.FieldMarks["design_blob_sha"] = NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", observed, now, []string{item.DesignBlobSHA, item.Reference}, "")
				} else {
					rollup.FieldMarks["design_blob_sha"] = missionUnavailableMark("factory_v1_ledger", now, "design evidence is not an exact blob SHA")
				}
			}
			if item.Approval != nil && stage.Stage == factoryv1.StageHumanDesignReview {
				rollup.HumanDesignReviewRef = item.Reference
				if strings.TrimSpace(item.Reference) != "" && isExactGitOrDocumentHash(item.DesignBlobSHA) {
					rollup.FieldMarks["human_design_review_ref"] = NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", observed, now, []string{item.Reference, item.DesignBlobSHA}, "exact scoped Human Design Review receipt")
				} else {
					rollup.FieldMarks["human_design_review_ref"] = missionUnavailableMark("factory_v1_ledger", now, "Human Design Review receipt is missing an exact approved design blob")
				}
			}
			if item.PR != nil {
				missionApplyPREvidence(&rollup, item.PR.Repository, item.PR.Number, item.PR.HeadSHA, item.PR.ReviewedHeadSHA, item.PR.Open, item.PR.Draft, observed, now, []string{item.Reference})
			}
			rollup.Items = append(rollup.Items, projected)
			refs = append(refs, item.Reference)
		}
	}
	if order.PR != nil {
		missionApplyPREvidence(&rollup, order.PR.Repository, order.PR.Number, order.PR.HeadSHA, order.PR.ReviewedHeadSHA, order.PR.Open, order.PR.Draft, order.LastEffectAt, now, nil)
	}
	classification := classifyMissionOrder(order, now)
	rollup.PendingTier3HumanReview = order.TLCStage == factoryv1.StageHumanReview && !missionFactoryOrderTerminal(order) && classification.EffectiveHumanReviewTier == 3
	rollup.FieldMarks["pending_tier_3_human_review"] = NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", order.LastEffectAt, now, refs, "derived from validated exact Factory stage state")
	refs = compactStrings(append(refs, order.DocumentSHA256))
	rollup.Mark = missionEvidenceFieldAggregateMark(rollup.FieldMarks, order.LastEffectAt, now, refs)
	return rollup
}

var missionEvidenceFieldNames = []string{
	"factory_order_ref", "design_blob_sha", "human_design_review_ref", "pr_repository", "pr_number",
	"pr_state", "pr_head_sha", "reviewed_head_sha", "ready_head_matches", "pending_tier_3_human_review",
}

func missionUnavailableEvidenceFieldMarks(source string, now time.Time, reason string) map[string]EvidenceMark {
	marks := make(map[string]EvidenceMark, len(missionEvidenceFieldNames))
	for _, field := range missionEvidenceFieldNames {
		marks[field] = missionUnavailableMark(source, now, field+": "+reason)
	}
	return marks
}

func missionApplyPREvidence(rollup *EvidenceRollup, repository string, number int, head, reviewed string, open, draft bool, observed, now time.Time, refs []string) {
	rollup.PRRepository, rollup.PRNumber = repository, number
	rollup.PRHeadSHA, rollup.ReviewedHeadSHA = head, reviewed
	if !open {
		rollup.PRState = "closed"
	} else if draft {
		rollup.PRState = "draft"
	} else {
		rollup.PRState = "ready"
	}
	exact := func(field string, valid bool, reason string, evidenceRefs ...string) {
		if valid {
			rollup.FieldMarks[field] = NewEvidenceMark(FreshnessCurrent, BasisExact, "factory_v1_ledger", observed, now, compactStrings(append(refs, evidenceRefs...)), "")
		} else {
			rollup.FieldMarks[field] = missionUnavailableMark("factory_v1_ledger", now, reason)
		}
	}
	exact("pr_repository", strings.TrimSpace(repository) != "", "PR repository is unavailable", repository)
	exact("pr_number", number > 0, "PR number is unavailable", strconv.Itoa(number))
	exact("pr_state", rollup.PRState == "closed" || rollup.PRState == "draft" || rollup.PRState == "ready", "PR state is unavailable", rollup.PRState)
	exact("pr_head_sha", isExactGitOrDocumentHash(head), "PR head is not an exact commit SHA", head)
	exact("reviewed_head_sha", isExactGitOrDocumentHash(reviewed), "reviewed head is not an exact commit SHA", reviewed)
	rollup.ReadyHeadMatches = isExactGitOrDocumentHash(head) && isExactGitOrDocumentHash(reviewed) && head == reviewed
	exact("ready_head_matches", isExactGitOrDocumentHash(head) && isExactGitOrDocumentHash(reviewed), "exact-head equality is unavailable until both exact SHAs exist", head, reviewed)
}

func missionEvidenceFieldAggregateMark(fields map[string]EvidenceMark, observed, now time.Time, refs []string) EvidenceMark {
	freshness, basis := FreshnessCurrent, BasisExact
	reasons := []string{}
	for _, field := range missionEvidenceFieldNames {
		mark, exists := fields[field]
		if !exists || mark.Freshness == FreshnessUnavailable {
			freshness, basis = FreshnessUnavailable, BasisUnavailable
			if exists {
				reasons = append(reasons, mark.Reason)
			} else {
				reasons = append(reasons, field+": evidence mark missing")
			}
		}
	}
	return NewEvidenceMark(freshness, basis, "factory_v1_ledger", observed, now, refs, strings.Join(compactStrings(reasons), "; "))
}

func missionNextStage(order factoryv1.OrderProjection) string {
	if order.NextAction == "" {
		return ""
	}
	index := factoryv1.StageIndex(order.TLCStage)
	if index >= 0 && index+1 < len(factoryv1.TLCStages) {
		return string(factoryv1.TLCStages[index+1])
	}
	return ""
}

func missionCompletionPredicate(stage factoryv1.Stage) string {
	switch stage {
	case factoryv1.StageDesign:
		return "exact design blob is present"
	case factoryv1.StageIADA:
		return "exact design blob matches and IADA blocker count is zero"
	case factoryv1.StageCFADA:
		return "exact design blob matches, CFADA blocker count is zero, and reviewer family is independent"
	case factoryv1.StageWriteCode:
		return "exact branch/head and named passing validation are present"
	case factoryv1.StageCreateDraftPR:
		return "open draft PR binds the exact head"
	case factoryv1.StageIAR:
		return "IAR reviewed head equals PR head and blocker count is zero"
	case factoryv1.StageCFAR:
		return "CFAR reviewed head equals PR head, blocker count is zero, and reviewer family is independent"
	case factoryv1.StageMarkPRReady:
		return "PR is open, non-draft, checks pass, and ready head equals reviewed head"
	case factoryv1.StageHumanDesignReview, factoryv1.StageHumanReview:
		return "exact scoped Human approval or review receipt is present"
	default:
		return "existing ordered-stage validator accepts exact stage evidence"
	}
}

func projectedOnlyWIPRow(row WIPItem, now time.Time, reason string) WIPItem {
	convert := func(value MarkedValue) MarkedValue {
		if value.Mark.Freshness == FreshnessUnavailable {
			return value
		}
		value.Mark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, value.Mark.SourceID, value.Mark.ObservedAt, now, value.Mark.EvidenceRefs, reason)
		return value
	}
	row.TargetRepository = convert(row.TargetRepository)
	row.Assignment = convert(row.Assignment)
	row.LifecycleStatus = convert(row.LifecycleStatus)
	row.EngineProtocol = convert(row.EngineProtocol)
	row.TLCStage = convert(row.TLCStage)
	row.TLCStageIndex = convert(row.TLCStageIndex)
	row.ItemStartedAt = convert(row.ItemStartedAt)
	row.LastEffectAt = convert(row.LastEffectAt)
	row.ElapsedMS = convert(row.ElapsedMS)
	row.NextHandoff = convert(row.NextHandoff)
	row.Completeness = convert(row.Completeness)
	row.Classification.Mark = projectedOnlyMissionMark(row.Classification.Mark, now, reason)
	row.EvidenceRollup.Mark = projectedOnlyMissionMark(row.EvidenceRollup.Mark, now, reason)
	for key, mark := range row.EvidenceRollup.FieldMarks {
		row.EvidenceRollup.FieldMarks[key] = projectedOnlyMissionMark(mark, now, reason)
	}
	for i := range row.EvidenceRollup.Items {
		row.EvidenceRollup.Items[i].Mark = projectedOnlyMissionMark(row.EvidenceRollup.Items[i].Mark, now, reason)
	}
	row.Mark = projectedOnlyMissionMark(row.Mark, now, reason)
	return row
}

func projectedOnlyMissionMark(mark EvidenceMark, now time.Time, reason string) EvidenceMark {
	if mark.Freshness == FreshnessUnavailable || mark.Basis == BasisUnavailable {
		return mark
	}
	return NewEvidenceMark(mark.Freshness, BasisProjectedOnly, mark.SourceID, mark.ObservedAt, now, mark.EvidenceRefs, reason)
}

type missionIdentityFold struct {
	registered     event.Event
	identity       AgentIdentityRegisteredContent
	status         string
	statusEvent    event.Event
	authority      string
	authorityEvent event.Event
	spawn          *event.Event
	spawnContent   AgentSpawnedContent
	latestAt       time.Time
}

var missionRosterEventTypes = []types.EventType{
	EventTypeAgentIdentityRegistered,
	EventTypeAgentLifecycleTransitioned,
	EventTypeAgentIdentityRevoked,
	EventTypeAgentIdentityRetired,
	EventTypeAgentAuthorityScopeAssigned,
	EventTypeAgentAuthorityScopeReduced,
	EventTypeAgentSpawned,
	EventTypeAgentStopped,
	EventTypeModelRolePolicyUpdated,
}

func (p *CivilizationMissionControlProjector) buildRosterSource(_ context.Context, now time.Time) (missionRosterSource, error) {
	result := emptyMissionRosterSource(now, "")
	result.Completeness.Reasons = nil
	startHead, err := missionHeadIdentity(p.store)
	if err != nil {
		return result, fmt.Errorf("read roster start head: %w", err)
	}
	result.Completeness.StartHead = startHead
	result.Completeness.SourceEventGraphHead = startHead
	allEvents := map[types.EventType][]event.Event{}
	for _, eventType := range missionRosterEventTypes {
		events, pages, readErr := missionEventsByType(p.store, eventType, p.pageSize)
		allEvents[eventType] = events
		result.Completeness.DomainCounts[eventType.Value()] = len(events)
		result.Completeness.PageCounts[eventType.Value()] = pages
		if readErr != nil {
			result.Completeness.Reasons = append(result.Completeness.Reasons, fmt.Sprintf("read %s: %v", eventType.Value(), readErr))
		}
	}
	selectionSource := modelSelectionSourceWithRolePolicyUpdates(p.store, p.modelSelection, p.pageSize)
	selection := BuildOperatorModelSelection(selectionSource())
	for _, selectionErr := range selection.Errors {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "model selection: "+selectionErr)
	}

	identities := map[string]*missionIdentityFold{}
	for _, ev := range allEvents[EventTypeAgentIdentityRegistered] {
		content, ok := ev.Content().(AgentIdentityRegisteredContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+EventTypeAgentIdentityRegistered.Value()+" "+ev.ID().Value())
			continue
		}
		actorID := content.ActorID.Value()
		if actorID == "" {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "registered identity has empty actor ID "+ev.ID().Value())
			continue
		}
		if current, exists := identities[actorID]; exists {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "duplicate registered identity "+actorID)
			if !ev.Timestamp().Value().After(current.registered.Timestamp().Value()) {
				continue
			}
		}
		identities[actorID] = &missionIdentityFold{registered: ev, identity: content, status: content.LifecycleStatus, statusEvent: ev, authority: content.AuthorityScope, authorityEvent: ev, latestAt: ev.Timestamp().Value()}
	}
	for _, ev := range allEvents[EventTypeAgentLifecycleTransitioned] {
		content, ok := ev.Content().(AgentLifecycleTransitionedContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+EventTypeAgentLifecycleTransitioned.Value()+" "+ev.ID().Value())
			continue
		}
		if fold := identities[content.ActorID.Value()]; fold != nil && ev.Timestamp().Value().After(fold.statusEvent.Timestamp().Value()) {
			fold.status, fold.statusEvent = content.ResultingState, ev
			if ev.Timestamp().Value().After(fold.latestAt) {
				fold.latestAt = ev.Timestamp().Value()
			}
		}
	}
	for _, eventType := range []types.EventType{EventTypeAgentIdentityRevoked, EventTypeAgentIdentityRetired} {
		for _, ev := range allEvents[eventType] {
			actorID, status, ok := "", "", false
			switch content := ev.Content().(type) {
			case AgentIdentityRevokedContent:
				actorID, status, ok = content.ActorID.Value(), "revoked", true
			case AgentIdentityRetiredContent:
				actorID, status, ok = content.ActorID.Value(), "retired", true
			}
			if !ok {
				result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+eventType.Value()+" "+ev.ID().Value())
				continue
			}
			if fold := identities[actorID]; fold != nil && ev.Timestamp().Value().After(fold.statusEvent.Timestamp().Value()) {
				fold.status, fold.statusEvent = status, ev
				if ev.Timestamp().Value().After(fold.latestAt) {
					fold.latestAt = ev.Timestamp().Value()
				}
			}
		}
	}
	for _, eventType := range []types.EventType{EventTypeAgentAuthorityScopeAssigned, EventTypeAgentAuthorityScopeReduced} {
		for _, ev := range allEvents[eventType] {
			actorID, authority, ok := "", "", false
			switch content := ev.Content().(type) {
			case AgentAuthorityScopeAssignedContent:
				actorID, authority, ok = content.ActorID.Value(), content.AuthorityScope, true
			case AgentAuthorityScopeReducedContent:
				actorID, authority, ok = content.ActorID.Value(), content.ResultingScope, true
			}
			if !ok {
				result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+eventType.Value()+" "+ev.ID().Value())
				continue
			}
			if fold := identities[actorID]; fold != nil && ev.Timestamp().Value().After(fold.authorityEvent.Timestamp().Value()) {
				fold.authority, fold.authorityEvent = authority, ev
				if ev.Timestamp().Value().After(fold.latestAt) {
					fold.latestAt = ev.Timestamp().Value()
				}
			}
		}
	}
	latestSpawns := map[string]event.Event{}
	spawnContent := map[string]AgentSpawnedContent{}
	for _, ev := range allEvents[EventTypeAgentSpawned] {
		content, ok := ev.Content().(AgentSpawnedContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+EventTypeAgentSpawned.Value()+" "+ev.ID().Value())
			continue
		}
		actorID := strings.TrimSpace(content.ActorID)
		if actorID == "" {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "spawn event has no actor identity "+ev.ID().Value())
			continue
		}
		if prior, exists := latestSpawns[actorID]; !exists || ev.Timestamp().Value().After(prior.Timestamp().Value()) {
			latestSpawns[actorID], spawnContent[actorID] = ev, content
		}
	}
	latestStops := map[string]event.Event{}
	for _, ev := range allEvents[EventTypeAgentStopped] {
		content, ok := ev.Content().(AgentStoppedContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode "+EventTypeAgentStopped.Value()+" "+ev.ID().Value())
			continue
		}
		key := strings.TrimSpace(content.Name) + "\x00" + strings.TrimSpace(content.Role)
		if prior, exists := latestStops[key]; !exists || ev.Timestamp().Value().After(prior.Timestamp().Value()) {
			latestStops[key] = ev
		}
	}
	for actorID, spawn := range latestSpawns {
		if fold := identities[actorID]; fold != nil {
			copy := spawn
			fold.spawn = &copy
			fold.spawnContent = spawnContent[actorID]
			if spawn.Timestamp().Value().After(fold.latestAt) {
				fold.latestAt = spawn.Timestamp().Value()
			}
		}
	}

	configuredRoles := map[string]OperatorModelRoleAssignment{}
	for _, assignment := range selection.Assignments {
		configuredRoles[assignment.Role] = assignment
	}
	modelMatches := missionModelMatches(selection.Models)
	instantiatedByRole, eventActiveByRole := map[string]int{}, map[string]int{}
	for _, fold := range identities {
		instantiatedByRole[fold.identity.Role]++
		row, active := missionIdentityRow(fold, configuredRoles, modelMatches, latestStops, now)
		if active {
			eventActiveByRole[fold.identity.Role]++
		}
		result.Rows = append(result.Rows, row)
	}
	loadedAt := selection.LoadedAt
	if loadedAt.IsZero() {
		loadedAt = now
	}
	for _, assignment := range selection.Assignments {
		mark := NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "hive_model_selection", loadedAt, now, compactStrings([]string{assignment.PolicyEventID}), "configured role and routing projection; not process liveness")
		providerMark, modelMark := mark, mark
		if assignment.Provider == "" {
			providerMark = missionUnavailableMark("hive_model_selection", now, "configured role has no resolved provider")
		}
		if assignment.Model == "" {
			modelMark = missionUnavailableMark("hive_model_selection", now, "configured role has no resolved model")
		}
		capacity := any(nil)
		capacityMark := missionUnavailableMark("hive_model_selection", now, "configured route has no unique model capacity")
		if matches := modelMatches[assignment.Model]; len(matches) == 1 {
			capacity = matches[0].MaxOutputTokens
			capacityMark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "hive_model_selection", loadedAt, now, nil, "catalog per-call maximum; not worker capacity")
		}
		result.Rows = append(result.Rows, RoleAgentRow{
			StableID: "role:" + assignment.Role, Role: assignment.Role,
			Configured: missionMarked(true, mark), Instantiated: missionMarked(instantiatedByRole[assignment.Role], NewEvidenceMark(FreshnessCurrent, BasisExact, "eventgraph_roster", now, now, nil, "count from exhaustive identity registration domain")),
			EventActive: missionMarked(eventActiveByRole[assignment.Role], NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "eventgraph_roster", now, now, nil, "spawn-minus-stop event correlation; not general process liveness")),
			Running:     missionMarked(nil, missionUnavailableMark("eventgraph_roster", now, "general Agent process liveness is not observed")),
			Provider:    missionMarked(assignment.Provider, providerMark), Model: missionMarked(assignment.Model, modelMark),
			Authority: missionMarked(map[string]any{"can_operate": assignment.CanOperate, "auth_mode": assignment.AuthMode}, mark), Capacity: missionMarked(capacity, capacityMark),
			Status: missionMarked("configured", mark), Assignment: missionMarked(nil, missionUnavailableMark("hive_model_selection", now, "configured role is not a runtime assignment")), Mark: mark,
		})
	}
	endHead, headErr := missionHeadIdentity(p.store)
	if headErr != nil {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "read roster end head: "+headErr.Error())
	}
	result.Completeness.EndHead = endHead
	if startHead != endHead {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "EventGraph head changed during roster enumeration")
		for i := range result.Rows {
			result.Rows[i] = projectedOnlyRoleRow(result.Rows[i], now, "EventGraph head changed during roster enumeration")
		}
	}
	result.Completeness.Complete = len(result.Completeness.Reasons) == 0
	result.Completeness.Reasons = compactStrings(result.Completeness.Reasons)
	sort.Slice(result.Rows, func(i, j int) bool { return result.Rows[i].StableID < result.Rows[j].StableID })
	return result, nil
}

func missionModelMatches(models []OperatorModelCatalogEntry) map[string][]OperatorModelCatalogEntry {
	result := map[string][]OperatorModelCatalogEntry{}
	for _, model := range models {
		keys := append([]string{model.ID}, model.Aliases...)
		for _, key := range compactStrings(keys) {
			result[key] = append(result[key], model)
		}
	}
	return result
}

func missionIdentityRow(fold *missionIdentityFold, configured map[string]OperatorModelRoleAssignment, models map[string][]OperatorModelCatalogEntry, latestStops map[string]event.Event, now time.Time) (RoleAgentRow, bool) {
	identityRef := fold.registered.ID().Value()
	exact := NewEvidenceMark(FreshnessCurrent, BasisExact, "eventgraph_roster", fold.latestAt, now, []string{identityRef}, "")
	_, configuredRole := configured[fold.identity.Role]
	configuredMark := NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "hive_model_selection", now, now, nil, "role-name join to configured routing")
	eventActive := false
	eventActiveMark := NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "eventgraph_roster", fold.latestAt, now, []string{identityRef}, "no active spawn event for this actor; event-active is not process liveness")
	model := ""
	modelMark := missionUnavailableMark("eventgraph_roster", now, "identity has no exact active spawn model")
	if fold.spawn != nil {
		key := strings.TrimSpace(fold.spawnContent.Name) + "\x00" + strings.TrimSpace(fold.spawnContent.Role)
		stop, stopped := latestStops[key]
		eventActive = !stopped || fold.spawn.Timestamp().Value().After(stop.Timestamp().Value())
		reason := "spawn event has no later correlated stop event; this does not prove process liveness"
		refs := []string{fold.spawn.ID().Value()}
		if stopped {
			refs = append(refs, stop.ID().Value())
			reason = "spawn/stop correlation; this does not prove process liveness"
		}
		eventActiveMark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "eventgraph_roster", fold.spawn.Timestamp().Value(), now, refs, reason)
		model = fold.spawnContent.Model
		modelMark = NewEvidenceMark(FreshnessCurrent, BasisExact, "eventgraph_roster", fold.spawn.Timestamp().Value(), now, []string{fold.spawn.ID().Value()}, "model bound to this spawn event")
	}
	provider := any(nil)
	providerMark := missionUnavailableMark("hive_model_selection", now, "active model has no unique catalog provider match")
	capacity := any(nil)
	capacityMark := missionUnavailableMark("hive_model_selection", now, "active model has no unique catalog capacity match")
	if matches := models[model]; model != "" && len(matches) == 1 {
		provider = matches[0].Provider
		providerMark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "hive_model_selection", now, now, []string{fold.spawn.ID().Value()}, "provider enriched from unique model-catalog match")
		capacity = matches[0].MaxOutputTokens
		capacityMark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, "hive_model_selection", now, now, []string{fold.spawn.ID().Value()}, "catalog per-call maximum; not worker capacity")
	}
	authorityMark := NewEvidenceMark(FreshnessCurrent, BasisExact, "eventgraph_roster", fold.authorityEvent.Timestamp().Value(), now, []string{fold.authorityEvent.ID().Value()}, "recorded authority posture only; grants no new authority")
	statusMark := NewEvidenceMark(FreshnessCurrent, BasisExact, "eventgraph_roster", fold.statusEvent.Timestamp().Value(), now, []string{fold.statusEvent.ID().Value()}, "")
	return RoleAgentRow{
		StableID: "agent:" + fold.identity.ActorID.Value(), Role: fold.identity.Role, ActorID: fold.identity.ActorID.Value(),
		Configured: missionMarked(configuredRole, configuredMark), Instantiated: missionMarked(true, exact), EventActive: missionMarked(eventActive, eventActiveMark), Running: missionMarked(nil, missionUnavailableMark("eventgraph_roster", now, "general Agent process liveness is not observed")),
		Provider: missionMarked(provider, providerMark), Model: missionMarked(model, modelMark), Authority: missionMarked(fold.authority, authorityMark), Capacity: missionMarked(capacity, capacityMark), Status: missionMarked(fold.status, statusMark), Assignment: missionMarked(nil, missionUnavailableMark("eventgraph_roster", now, "no runtime assignment is bound to this durable identity")), Mark: exact,
	}, eventActive
}

func projectedOnlyRoleRow(row RoleAgentRow, now time.Time, reason string) RoleAgentRow {
	convert := func(value MarkedValue) MarkedValue {
		if value.Mark.Freshness == FreshnessUnavailable {
			return value
		}
		value.Mark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, value.Mark.SourceID, value.Mark.ObservedAt, now, value.Mark.EvidenceRefs, reason)
		return value
	}
	row.Configured = convert(row.Configured)
	row.Instantiated = convert(row.Instantiated)
	row.EventActive = convert(row.EventActive)
	row.Running = convert(row.Running)
	row.Provider = convert(row.Provider)
	row.Model = convert(row.Model)
	row.Authority = convert(row.Authority)
	row.Capacity = convert(row.Capacity)
	row.Status = convert(row.Status)
	row.Assignment = convert(row.Assignment)
	row.Mark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, row.Mark.SourceID, row.Mark.ObservedAt, now, row.Mark.EvidenceRefs, reason)
	return row
}

func (p *CivilizationMissionControlProjector) buildAuthoritySource(_ context.Context, now time.Time) (missionAuthoritySource, error) {
	result := emptyMissionAuthoritySource(now, "")
	result.Completeness.Reasons = nil
	startHead, err := missionHeadIdentity(p.store)
	if err != nil {
		return result, fmt.Errorf("read authority start head: %w", err)
	}
	result.Completeness.StartHead = startHead
	result.Completeness.SourceEventGraphHead = startHead
	requests, requestPages, requestErr := missionEventsByType(p.store, EventTypeAuthorityRequestRecorded, p.pageSize)
	decisions, decisionPages, decisionErr := missionEventsByType(p.store, EventTypeAuthorityDecisionRecorded, p.pageSize)
	result.Completeness.DomainCounts[EventTypeAuthorityRequestRecorded.Value()] = len(requests)
	result.Completeness.DomainCounts[EventTypeAuthorityDecisionRecorded.Value()] = len(decisions)
	result.Completeness.PageCounts[EventTypeAuthorityRequestRecorded.Value()] = requestPages
	result.Completeness.PageCounts[EventTypeAuthorityDecisionRecorded.Value()] = decisionPages
	if requestErr != nil {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "read authority requests: "+requestErr.Error())
	}
	if decisionErr != nil {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "read authority decisions: "+decisionErr.Error())
	}
	type recordedRequest struct {
		event   event.Event
		content AuthorityRequestRecordedContent
	}
	type recordedDecision struct {
		event   event.Event
		content AuthorityDecisionRecordedContent
	}
	requestByID := map[string]recordedRequest{}
	for _, ev := range requests {
		content, ok := ev.Content().(AuthorityRequestRecordedContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode authority request "+ev.ID().Value())
			continue
		}
		requestID := content.RequestID.Value()
		if requestID == "" {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "authority request has empty RequestID "+ev.ID().Value())
			continue
		}
		if prior, exists := requestByID[requestID]; exists {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "duplicate authority request envelope "+requestID)
			if !ev.Timestamp().Value().After(prior.event.Timestamp().Value()) {
				continue
			}
		}
		requestByID[requestID] = recordedRequest{event: ev, content: content}
	}
	decisionsByRequest := map[string][]recordedDecision{}
	for _, ev := range decisions {
		content, ok := ev.Content().(AuthorityDecisionRecordedContent)
		if !ok {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "decode authority decision "+ev.ID().Value())
			continue
		}
		requestID := content.RequestID.Value()
		if requestID == "" || (content.Outcome != "approved" && content.Outcome != "denied") || strings.TrimSpace(content.DecisionID) == "" {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "invalid authority decision "+ev.ID().Value())
			continue
		}
		decisionsByRequest[requestID] = append(decisionsByRequest[requestID], recordedDecision{event: ev, content: content})
	}
	for requestID, request := range requestByID {
		valid := []recordedDecision{}
		for _, decision := range decisionsByRequest[requestID] {
			if !decision.event.Timestamp().Value().Before(request.event.Timestamp().Value()) {
				valid = append(valid, decision)
			}
		}
		if len(valid) == 1 {
			continue
		}
		severity := missionRiskSeverity(request.content.RiskClass)
		refs := []string{request.event.ID().Value(), requestID}
		kind, summary, action := "authority_request", "Protected action is waiting for a Human authority decision.", "Review the exact request scope and record an explicit approval or denial through the governed authority endpoint."
		markBasis := BasisExact
		if len(valid) > 1 {
			kind = "authority_decision_conflict"
			summary = "Conflicting decisions exist for the same authority request; no decision receives credit."
			action = "Repair the conflicting authority evidence through an audited Human process before executing the protected action."
			markBasis = BasisProjectedOnly
			severity = "critical"
			for _, decision := range valid {
				refs = append(refs, decision.event.ID().Value())
			}
		}
		mark := NewEvidenceMark(FreshnessCurrent, markBasis, "authority_eventgraph", request.event.Timestamp().Value(), now, refs, summary)
		result.HumanActions = append(result.HumanActions, HumanAction{
			ActionID: "authority:" + requestID, Kind: kind, Severity: severity, OwningStage: "authority", SubjectID: requestID,
			Summary: summary + " " + boundedMissionReason(request.content.ActionName+" on "+request.content.Target), RequiredAction: action,
			SourceTime: request.event.Timestamp().Value(), EvidenceRefs: compactStrings(refs), Mark: mark,
		})
	}
	for requestID, decisionSet := range decisionsByRequest {
		if _, exists := requestByID[requestID]; exists {
			continue
		}
		for _, decision := range decisionSet {
			result.Completeness.Reasons = append(result.Completeness.Reasons, "authority decision references unavailable request "+decision.event.ID().Value())
		}
	}
	endHead, headErr := missionHeadIdentity(p.store)
	if headErr != nil {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "read authority end head: "+headErr.Error())
	}
	result.Completeness.EndHead = endHead
	if startHead != endHead {
		result.Completeness.Reasons = append(result.Completeness.Reasons, "EventGraph head changed during authority enumeration")
		for i := range result.HumanActions {
			mark := result.HumanActions[i].Mark
			result.HumanActions[i].Mark = NewEvidenceMark(FreshnessCurrent, BasisProjectedOnly, mark.SourceID, mark.ObservedAt, now, mark.EvidenceRefs, "EventGraph head changed during authority enumeration")
		}
	}
	result.Completeness.Complete = len(result.Completeness.Reasons) == 0
	result.Completeness.Reasons = compactStrings(result.Completeness.Reasons)
	sort.Slice(result.HumanActions, func(i, j int) bool { return result.HumanActions[i].ActionID < result.HumanActions[j].ActionID })
	return result, nil
}

func missionRiskSeverity(risk string) string {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func missionWorkerPool(snapshot FactoryRuntimeSnapshot, mark EvidenceMark) WorkerPool {
	unavailable := func(reason string) MarkedValue {
		return missionMarked(nil, missionUnavailableMark("factory_runtime", mark.GeneratedAt, reason))
	}
	if mark.Freshness == FreshnessUnavailable {
		return WorkerPool{
			ConfiguredWorkers: unavailable("worker capacity is unavailable"), ActiveWorkers: unavailable("worker utilization is unavailable"),
			AvailableWorkers: unavailable("worker availability is unavailable"), QueuedOrders: unavailable("runtime demand is unavailable"),
			SchedulableOrders: unavailable("schedulable demand is unavailable"), Assignments: []factoryv1.RuntimeAssignment{},
			UtilizationPercent: unavailable("worker utilization is unavailable"), Mark: mark,
		}
	}
	utilization := float64(0)
	if snapshot.ConfiguredWorkers > 0 {
		utilization = float64(snapshot.ActiveWorkers) * 100 / float64(snapshot.ConfiguredWorkers)
	}
	derived := NewEvidenceMark(mark.Freshness, mark.Basis, "factory_runtime", mark.ObservedAt, mark.GeneratedAt, mark.EvidenceRefs, "derived from validated ephemeral daemon worker arithmetic")
	return WorkerPool{
		ConfiguredWorkers: missionMarked(snapshot.ConfiguredWorkers, mark), ActiveWorkers: missionMarked(snapshot.ActiveWorkers, mark), AvailableWorkers: missionMarked(snapshot.AvailableWorkers, mark),
		QueuedOrders: missionMarked(snapshot.QueuedOrders, mark), SchedulableOrders: missionMarked(snapshot.SchedulableOrders, mark), Assignments: append([]factoryv1.RuntimeAssignment(nil), snapshot.Assignments...),
		UtilizationPercent: missionMarked(utilization, derived), Mark: mark,
	}
}

func runtimeRoleRows(snapshot FactoryRuntimeSnapshot, mark EvidenceMark) []RoleAgentRow {
	if mark.Freshness == FreshnessUnavailable {
		return []RoleAgentRow{}
	}
	rows := make([]RoleAgentRow, 0, len(snapshot.Assignments))
	for _, assignment := range snapshot.Assignments {
		assignmentRefs := compactStrings([]string{snapshot.BootID, assignment.OrderID, assignment.AttemptID, assignment.DocumentSHA256})
		projected := NewEvidenceMark(mark.Freshness, BasisProjectedOnly, "factory_runtime", mark.ObservedAt, mark.GeneratedAt, assignmentRefs, "boot-scoped daemon assignment; not durable TLC or Agent-identity evidence")
		providerMark, modelMark := projected, projected
		if assignment.ProviderID == "" {
			providerMark = missionUnavailableMark("factory_runtime", mark.GeneratedAt, "runtime assignment has no provider binding")
		}
		if assignment.ModelID == "" {
			modelMark = missionUnavailableMark("factory_runtime", mark.GeneratedAt, "runtime assignment has no model binding")
		}
		rows = append(rows, RoleAgentRow{
			StableID: "runtime:" + snapshot.BootID + ":" + assignment.OrderID, Role: "factory_runner",
			Configured: missionMarked(true, projected), Instantiated: missionMarked(nil, missionUnavailableMark("factory_runtime", mark.GeneratedAt, "runtime assignment has no durable actor identity")),
			EventActive: missionMarked(nil, missionUnavailableMark("factory_runtime", mark.GeneratedAt, "runtime assignment is not a spawn/stop Agent observation")), Running: missionMarked(true, projected),
			Provider: missionMarked(assignment.ProviderID, providerMark), Model: missionMarked(assignment.ModelID, modelMark),
			Authority: missionMarked(nil, missionUnavailableMark("factory_runtime", mark.GeneratedAt, "runtime assignment snapshot does not grant or project authority")), Capacity: missionMarked(1, projected),
			Status: missionMarked(string(snapshot.SchedulerState), projected), Assignment: missionMarked(assignment, projected), Mark: projected,
		})
	}
	return rows
}

func missionRuntimeCompleteness(snapshot FactoryRuntimeSnapshot, mark EvidenceMark) MissionCompleteness {
	reasons := []string{}
	complete := mark.Freshness != FreshnessUnavailable
	if !complete {
		reasons = append(reasons, mark.Reason)
	}
	return MissionCompleteness{
		Complete: complete, Reasons: compactStrings(reasons), SourceEventGraphHead: "not_applicable_ephemeral_runtime",
		StartHead: snapshot.BootID, EndHead: snapshot.BootID,
		DomainCounts: map[string]int{"assignments": len(snapshot.Assignments)}, PageCounts: map[string]int{"runtime_snapshot": func() int {
			if complete {
				return 1
			}
			return 0
		}()},
	}
}

func missionServiceHealth(now time.Time, wip missionWIPSource, wipMark, rosterMark, authorityMark EvidenceMark, runtime FactoryRuntimeSnapshot, runtimeMark EvidenceMark) []ServiceHealth {
	health := []ServiceHealth{}
	eventGraphStatus := "healthy"
	eventGraphMark := missionAggregateMark(now, []SourceEnvelope{
		{SourceID: "eventgraph_wip_evidence", Required: true, Completeness: wip.Completeness, Mark: wipMark},
		{SourceID: "roster_routing", Required: true, Mark: rosterMark}, {SourceID: "authority_actions", Required: true, Mark: authorityMark},
	})
	if eventGraphMark.Freshness == FreshnessUnavailable {
		eventGraphStatus = "unavailable"
	} else if eventGraphMark.State != StateCurrent || !wip.Completeness.Complete {
		eventGraphStatus = "degraded"
	}
	health = append(health, ServiceHealth{ServiceID: "eventgraph", Label: "EventGraph evidence", OperationalStatus: eventGraphStatus, Detail: "Head-bracketed WIP, roster, routing, and authority evidence.", Mark: eventGraphMark})
	workStatus := "healthy"
	if wipMark.Freshness == FreshnessUnavailable {
		workStatus = "unavailable"
	} else if wipMark.Freshness != FreshnessCurrent || !wip.Completeness.Complete {
		workStatus = "degraded"
	}
	health = append(health, ServiceHealth{ServiceID: "work_projection", Label: "Work projection", OperationalStatus: workStatus, Detail: "Exhaustive EventGraph Work fold; separate from Work HTTP liveness.", Mark: wipMark})
	factoryStatus, factoryDetail := "healthy", wip.FactoryService.Detail
	factoryMark := projectedOnlyMissionMark(wipMark, now, "same-process Hive ops API projection receipt; Factory process liveness comes only from daemon runtime evidence")
	if strings.TrimSpace(wip.FactoryService.ServiceID) == "" || wipMark.Freshness == FreshnessUnavailable {
		factoryStatus = "unavailable"
		factoryDetail = "Factory v1 projection is unavailable."
	} else if !wip.FactoryService.Healthy || wipMark.Freshness != FreshnessCurrent {
		factoryStatus = "degraded"
	}
	health = append(health, ServiceHealth{ServiceID: "hive_ops_api", Label: "Hive ops API", OperationalStatus: factoryStatus, Detail: factoryDetail, Mark: factoryMark})
	runtimeStatus := "healthy"
	if runtimeMark.Freshness == FreshnessUnavailable {
		runtimeStatus = "unavailable"
	} else if runtimeMark.Freshness != FreshnessCurrent || runtime.SchedulerState == FactoryRuntimeDegraded || runtime.SchedulerState == FactoryRuntimeStopping {
		runtimeStatus = "degraded"
	}
	health = append(health, ServiceHealth{ServiceID: "factory_runtime", Label: "Factory worker runtime", OperationalStatus: runtimeStatus, Detail: func() string {
		if runtimeMark.Reason != "" {
			return runtimeMark.Reason
		}
		return string(runtime.SchedulerState)
	}(), Mark: runtimeMark})
	aggregateStatus := "healthy"
	aggregateMark := missionAggregateMark(now, []SourceEnvelope{{SourceID: "eventgraph", Required: true, Mark: eventGraphMark}, {SourceID: "factory", Required: true, Mark: factoryMark}, {SourceID: "runtime", Required: true, Mark: runtimeMark}})
	for _, service := range health {
		if service.OperationalStatus == "unavailable" {
			aggregateStatus = "unavailable"
			break
		}
		if service.OperationalStatus != "healthy" {
			aggregateStatus = "degraded"
		}
	}
	health = append([]ServiceHealth{{ServiceID: "civilization", Label: "Civilization", OperationalStatus: aggregateStatus, Detail: "Read-only aggregate; health grants no control authority.", Mark: aggregateMark}}, health...)
	return health
}

func missionAggregateMark(now time.Time, sources []SourceEnvelope) EvidenceMark {
	freshness := FreshnessCurrent
	basis := BasisExact
	refs := []string{}
	reasons := []string{}
	observed := now
	for _, source := range sources {
		mark := source.Mark
		refs = append(refs, mark.EvidenceRefs...)
		if mark.Reason != "" {
			reasons = append(reasons, source.SourceID+": "+mark.Reason)
		}
		if !mark.ObservedAt.IsZero() && mark.ObservedAt.Before(observed) {
			observed = mark.ObservedAt
		}
		if mark.Freshness == FreshnessUnavailable {
			freshness = FreshnessUnavailable
		} else if mark.Freshness == FreshnessStale && freshness != FreshnessUnavailable {
			freshness = FreshnessStale
		}
		if mark.Basis == BasisUnavailable {
			basis = BasisUnavailable
		} else if mark.Basis == BasisProjectedOnly && basis != BasisUnavailable {
			basis = BasisProjectedOnly
		} else if mark.Basis == BasisInferred && basis == BasisExact {
			basis = BasisInferred
		}
	}
	if freshness == FreshnessUnavailable {
		basis = BasisUnavailable
	}
	return NewEvidenceMark(freshness, basis, "civilization_mission_control", observed, now, refs, strings.Join(compactStrings(reasons), "; "))
}
