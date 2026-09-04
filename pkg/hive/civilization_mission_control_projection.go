package hive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	MissionControlSchemaVersion   = "civilization-mission-control/v2"
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
	PRRepository     string                  `json:"pr_repository"`
	PRNumber         int                     `json:"pr_number"`
	PRState          string                  `json:"pr_state"`
	PRHeadSHA        string                  `json:"pr_head_sha"`
	ReviewedHeadSHA  string                  `json:"reviewed_head_sha"`
	ReadyHeadMatches bool                    `json:"ready_head_matches"`
	Items            []MissionEvidenceItem   `json:"items"`
	FieldMarks       map[string]EvidenceMark `json:"field_marks"`
	Mark             EvidenceMark            `json:"mark"`
}

type WIPItem struct {
	Kind                string         `json:"kind"`
	StableID            string         `json:"stable_id"`
	FactoryOrderID      string         `json:"factory_order_id"`
	FactoryOrderVersion string         `json:"factory_order_version"`
	DocumentSHA256      string         `json:"document_sha256"`
	WorkTaskID          string         `json:"work_task_id"`
	Title               string         `json:"title"`
	TargetRepository    MarkedValue    `json:"target_repository"`
	Assignment          MarkedValue    `json:"assignment"`
	LifecycleStatus     MarkedValue    `json:"lifecycle_status"`
	EngineProtocol      MarkedValue    `json:"engine_protocol"`
	ItemStartedAt       MarkedValue    `json:"item_started_at"`
	LastEffectAt        MarkedValue    `json:"last_effect_at"`
	ElapsedMS           MarkedValue    `json:"elapsed_ms"`
	NextHandoff         MarkedValue    `json:"next_handoff"`
	Completeness        MarkedValue    `json:"completeness"`
	BlockerRefs         []string       `json:"blocker_refs"`
	InterventionRefs    []string       `json:"intervention_refs"`
	EvidenceRollup      EvidenceRollup `json:"evidence_rollup"`
	Mark                EvidenceMark   `json:"mark"`
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

type MissionClock interface {
	Now() time.Time
}

type missionWallClock struct{}

func (missionWallClock) Now() time.Time { return time.Now().UTC() }

type MissionControlProjectorConfig struct {
	ModelSelection OperatorModelSelectionSource
	Clock          MissionClock
	PageSize       int
	Retention      time.Duration
}

type cachedMissionSource[T any] struct {
	value      T
	observedAt time.Time
	valid      bool
}

type missionWIPSource struct {
	GeneratedAt   time.Time
	Rows          []WIPItem
	Interventions []MissionIntervention
	Handoffs      []Handoff
	HumanActions  []HumanAction
	Completeness  MissionCompleteness
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
	modelSelection OperatorModelSelectionSource
	clock          MissionClock
	pageSize       int
	retention      time.Duration

	mu             sync.Mutex
	wipCache       cachedMissionSource[missionWIPSource]
	rosterCache    cachedMissionSource[missionRosterSource]
	authorityCache cachedMissionSource[missionAuthoritySource]
}

func NewCivilizationMissionControlProjector(s store.Store, config MissionControlProjectorConfig) (*CivilizationMissionControlProjector, error) {
	if s == nil {
		return nil, errors.New("mission control projector requires EventGraph store")
	}
	if config.Clock == nil {
		config.Clock = missionWallClock{}
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
		store: s, modelSelection: config.ModelSelection,
		clock: config.Clock, pageSize: config.PageSize, retention: config.Retention,
	}, nil
}

func (p *CivilizationMissionControlProjector) Build(ctx context.Context) MissionControlProjection {
	now := p.clock.Now().UTC()
	wip, wipMark := p.acquireWIP(ctx, now)
	roster, rosterMark := p.acquireRoster(ctx, now)
	authority, authorityMark := p.acquireAuthority(ctx, now)

	roles := append([]RoleAgentRow(nil), roster.Rows...)
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
	}
	services := missionServiceHealth(now, wip, wipMark, rosterMark, authorityMark)
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
		Roles: roles, HumanActions: actions,
		Interventions: append([]MissionIntervention(nil), wip.Interventions...), Handoffs: append([]Handoff(nil), wip.Handoffs...),
		ResidualRisks: []string{
			"worker capacity is not part of the stable Mission Control evidence contract",
			"head changes during exhaustive reads make the affected source projected-only or stale",
			"process-local stale retention is lost on restart and never exceeds 15 minutes from original observation",
			"spawn/stop events prove event-active state, not general Agent process liveness",
			"Work HTTP reachability is a separate Site-owned observation and does not prove EventGraph head equality",
		},
		NonAuthorizations: []string{
			"Mission Control is read-only and grants no approval, gate, merge, deploy, runtime, configuration, authority, or production action.",
			"Cached or projected-only data cannot authorize a transition.",
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
		row.ItemStartedAt = staleMarkedValue(row.ItemStartedAt, now, reason)
		row.LastEffectAt = staleMarkedValue(row.LastEffectAt, now, reason)
		row.ElapsedMS = staleMarkedValue(row.ElapsedMS, now, reason)
		row.NextHandoff = staleMarkedValue(row.NextHandoff, now, reason)
		row.Completeness = staleMarkedValue(row.Completeness, now, reason)
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

	for _, fold := range folds {
		status, terminal, statusMark := missionWorkStatus(fold, now)
		if terminal {
			continue
		}
		conflictReason := ""
		if len(fold.linkedOrderIDs) > 1 {
			conflictReason = "Work task contains conflicting historical FactoryOrderID links"
		}
		if len(fold.targetRepositories) > 1 {
			if conflictReason != "" {
				conflictReason += "; "
			}
			conflictReason += "Work task contains conflicting exact target repositories"
		}
		row := missionIndependentWorkWIPRow(fold, status, statusMark, now, conflictReason)
		if len(fold.linkedOrderIDs) == 1 {
			for orderID := range fold.linkedOrderIDs {
				row.FactoryOrderID = orderID
			}
		}
		result.Rows = append(result.Rows, row)
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
		EngineProtocol: missionMarked("work-v3.9", NewEvidenceMark(FreshnessCurrent, BasisInferred, "work_eventgraph", fold.created.Timestamp().Value(), now, []string{id}, "Work event vocabulary identifies the lifecycle family; TLC routing remains external")),
		ItemStartedAt:  missionMarked(fold.created.Timestamp().Value(), NewEvidenceMark(FreshnessCurrent, BasisExact, "work_eventgraph", fold.created.Timestamp().Value(), now, []string{id}, "")),
		LastEffectAt:   missionMarked(fold.latestAt, mark), ElapsedMS: missionElapsed(fold.created.Timestamp().Value(), now, "work_eventgraph", fold.evidenceRefs),
		NextHandoff:  missionMarked(nil, missionUnavailableMark("work_eventgraph", now, "no exact external workflow handoff is present in this Work projection")),
		Completeness: missionMarked(rowComplete, completenessMark),
		BlockerRefs:  blockers, InterventionRefs: []string{}, EvidenceRollup: EvidenceRollup{
			Items: []MissionEvidenceItem{}, FieldMarks: missionUnavailableEvidenceFieldMarks("work_eventgraph", now, "external workflow evidence is not present in this Work projection"),
			Mark: missionUnavailableMark("work_eventgraph", now, "external workflow evidence is not present in this Work projection"),
		}, Mark: mark,
	}
}

var missionEvidenceFieldNames = []string{
	"pr_repository", "pr_number", "pr_state", "pr_head_sha", "reviewed_head_sha", "ready_head_matches",
}

func missionUnavailableEvidenceFieldMarks(source string, now time.Time, reason string) map[string]EvidenceMark {
	marks := make(map[string]EvidenceMark, len(missionEvidenceFieldNames))
	for _, field := range missionEvidenceFieldNames {
		marks[field] = missionUnavailableMark(source, now, field+": "+reason)
	}
	return marks
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
	row.ItemStartedAt = convert(row.ItemStartedAt)
	row.LastEffectAt = convert(row.LastEffectAt)
	row.ElapsedMS = convert(row.ElapsedMS)
	row.NextHandoff = convert(row.NextHandoff)
	row.Completeness = convert(row.Completeness)
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

func missionServiceHealth(now time.Time, wip missionWIPSource, wipMark, rosterMark, authorityMark EvidenceMark) []ServiceHealth {
	eventGraphMark := missionAggregateMark(now, []SourceEnvelope{
		{SourceID: "eventgraph_wip_evidence", Required: true, Completeness: wip.Completeness, Mark: wipMark},
		{SourceID: "roster_routing", Required: true, Mark: rosterMark},
		{SourceID: "authority_actions", Required: true, Mark: authorityMark},
	})
	eventGraphStatus := "healthy"
	if eventGraphMark.Freshness == FreshnessUnavailable {
		eventGraphStatus = "unavailable"
	} else if eventGraphMark.State != StateCurrent || !wip.Completeness.Complete {
		eventGraphStatus = "degraded"
	}
	workStatus := "healthy"
	if wipMark.Freshness == FreshnessUnavailable {
		workStatus = "unavailable"
	} else if wipMark.Freshness != FreshnessCurrent || !wip.Completeness.Complete {
		workStatus = "degraded"
	}
	opsStatus := eventGraphStatus
	opsMark := projectedOnlyMissionMark(eventGraphMark, now, "projection response proves this handler path, not independent process liveness")
	health := []ServiceHealth{
		{ServiceID: "eventgraph", Label: "EventGraph evidence", OperationalStatus: eventGraphStatus, Detail: "Head-bracketed WIP, roster, routing, and authority evidence.", Mark: eventGraphMark},
		{ServiceID: "work_projection", Label: "Work projection", OperationalStatus: workStatus, Detail: "Exhaustive EventGraph Work fold; separate from Work HTTP liveness.", Mark: wipMark},
		{ServiceID: "hive_ops_api", Label: "Hive ops API", OperationalStatus: opsStatus, Detail: "Current Mission Control projection handler; no embedded TLC runtime.", Mark: opsMark},
	}
	aggregateStatus := "healthy"
	for _, service := range health {
		if service.OperationalStatus == "unavailable" {
			aggregateStatus = "unavailable"
			break
		}
		if service.OperationalStatus != "healthy" {
			aggregateStatus = "degraded"
		}
	}
	aggregateMark := missionAggregateMark(now, []SourceEnvelope{
		{SourceID: "eventgraph", Required: true, Mark: eventGraphMark},
		{SourceID: "ops_api", Required: true, Mark: opsMark},
	})
	return append([]ServiceHealth{{
		ServiceID: "civilization", Label: "Civilization", OperationalStatus: aggregateStatus,
		Detail: "Read-only aggregate; health grants no control authority.", Mark: aggregateMark,
	}}, health...)
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
