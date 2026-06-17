package hive

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const defaultOperatorProjectionLimit = 50

// OperatorProjection is the read-only Site-facing view over Hive Phase 3
// EventGraph records. It is derived state; EventGraph remains the authority.
type OperatorProjection struct {
	GeneratedAt        time.Time                     `json:"generated_at"`
	Source             string                        `json:"source"`
	ModelSelection     OperatorModelSelection        `json:"model_selection"`
	RuntimeEvidence    OperatorRuntimeEvidence       `json:"runtime_evidence"`
	PendingApprovals   []OperatorApprovalProjection  `json:"pending_approvals"`
	AuthorityDecisions []OperatorDecisionProjection  `json:"authority_decisions"`
	Lifecycle          []OperatorLifecycleProjection `json:"lifecycle"`
	KeyAuditTraces     []OperatorKeyAuditProjection  `json:"key_audit_traces"`
	Errors             []string                      `json:"errors,omitempty"`
}

type OperatorApprovalProjection struct {
	EventID           string    `json:"event_id"`
	RequestID         string    `json:"request_id"`
	RequestingActor   string    `json:"requesting_actor"`
	RequestingRole    string    `json:"requesting_role,omitempty"`
	ActionName        string    `json:"action_name"`
	Target            string    `json:"target"`
	Environment       string    `json:"environment"`
	RiskClass         string    `json:"risk_class"`
	RequestedOutcome  string    `json:"requested_outcome"`
	Justification     string    `json:"justification"`
	RiskSummary       string    `json:"risk_summary"`
	Scope             []string  `json:"scope,omitempty"`
	ProposedOperation string    `json:"proposed_operation,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type OperatorDecisionProjection struct {
	EventID         string    `json:"event_id"`
	DecisionID      string    `json:"decision_id"`
	RequestID       string    `json:"request_id"`
	ApproverActor   string    `json:"approver_actor"`
	DeciderRole     string    `json:"decider_role,omitempty"`
	Outcome         string    `json:"outcome"`
	ApprovedAction  string    `json:"approved_action"`
	ApprovedTarget  string    `json:"approved_target"`
	Scope           []string  `json:"scope,omitempty"`
	Rationale       string    `json:"rationale"`
	RequestedAction string    `json:"requested_action,omitempty"`
	RequestedTarget string    `json:"requested_target,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type OperatorLifecycleProjection struct {
	ActorID         string    `json:"actor_id"`
	DisplayName     string    `json:"display_name,omitempty"`
	Role            string    `json:"role,omitempty"`
	LifecycleStatus string    `json:"lifecycle_status"`
	AuthorityScope  string    `json:"authority_scope,omitempty"`
	KeyProvenance   string    `json:"key_provenance,omitempty"`
	Environment     string    `json:"environment,omitempty"`
	IdentityMode    string    `json:"identity_mode,omitempty"`
	LastEventType   string    `json:"last_event_type"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OperatorKeyAuditProjection struct {
	EventID          string    `json:"event_id"`
	EventType        string    `json:"event_type"`
	ActorID          string    `json:"actor_id,omitempty"`
	SubjectActorID   string    `json:"subject_actor_id,omitempty"`
	KeyProvenance    string    `json:"key_provenance,omitempty"`
	Environment      string    `json:"environment,omitempty"`
	IdentityMode     string    `json:"identity_mode,omitempty"`
	PublicKey        string    `json:"public_key,omitempty"`
	OldPublicKey     string    `json:"old_public_key,omitempty"`
	NewPublicKey     string    `json:"new_public_key,omitempty"`
	ExternalKeyRef   string    `json:"external_key_ref,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	RecordKind       string    `json:"record_kind,omitempty"`
	Rationale        string    `json:"rationale,omitempty"`
	AuthorityRequest string    `json:"authority_request,omitempty"`
	DecisionEvent    string    `json:"decision_event,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type OperatorRuntimeEvidence struct {
	Source               string                            `json:"source"`
	Status               string                            `json:"status"`
	LastRun              *OperatorRuntimeRunEvidence       `json:"last_run,omitempty"`
	AgentEvents          OperatorRuntimeAgentEvents        `json:"agent_events"`
	LastQueuedRunRequest *OperatorQueuedRunRequestEvidence `json:"last_queued_run_request,omitempty"`
	Artifacts            []OperatorRuntimeArtifactEvidence `json:"artifacts"`
	RunEvents            []OperatorRuntimeEventEvidence    `json:"run_events"`
	CausalGraph          OperatorRuntimeCausalGraph        `json:"causal_graph"`
	Limitations          []string                          `json:"limitations,omitempty"`
}

type OperatorRuntimeRunEvidence struct {
	StartedEventID   string     `json:"started_event_id"`
	ConversationID   string     `json:"conversation_id"`
	StartedAt        time.Time  `json:"started_at"`
	SeedIdea         string     `json:"seed_idea,omitempty"`
	RepoPath         string     `json:"repo_path,omitempty"`
	CompletedEventID string     `json:"completed_event_id,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	AgentCount       *int       `json:"agent_count,omitempty"`
	DurationMs       *int64     `json:"duration_ms,omitempty"`
	TotalCost        *float64   `json:"total_cost,omitempty"`
}

type OperatorRuntimeAgentEvents struct {
	Scope            string                         `json:"scope"`
	Spawned          int                            `json:"spawned"`
	Stopped          int                            `json:"stopped"`
	ObservedActive   int                            `json:"observed_active"`
	ActiveAgents     []OperatorRuntimeAgentEvidence `json:"active_agents,omitempty"`
	LastAgentEventID string                         `json:"last_agent_event_id,omitempty"`
	LastAgentEventAt *time.Time                     `json:"last_agent_event_at,omitempty"`
}

type OperatorRuntimeAgentEvidence struct {
	Name           string    `json:"name"`
	Role           string    `json:"role"`
	Model          string    `json:"model,omitempty"`
	ActorID        string    `json:"actor_id,omitempty"`
	SpawnedEventID string    `json:"spawned_event_id"`
	SpawnedAt      time.Time `json:"spawned_at"`
}

type OperatorQueuedRunRequestEvidence struct {
	EventID               string    `json:"event_id"`
	ConversationID        string    `json:"conversation_id"`
	RunID                 string    `json:"run_id"`
	Title                 string    `json:"title"`
	OperatorID            string    `json:"operator_id,omitempty"`
	Status                string    `json:"status"`
	TargetRepos           []string  `json:"target_repos,omitempty"`
	AuthorityInitialLevel string    `json:"authority_initial_level,omitempty"`
	AuthorityScope        string    `json:"authority_scope,omitempty"`
	BudgetMaxIterations   *int      `json:"budget_max_iterations,omitempty"`
	BudgetMaxCostUSD      *float64  `json:"budget_max_cost_usd,omitempty"`
	SourceEventID         string    `json:"source_event_id,omitempty"`
	BriefEventID          string    `json:"brief_event_id,omitempty"`
	EvidenceKind          string    `json:"evidence_kind"`
	CreatedAt             time.Time `json:"created_at"`
}

type OperatorRuntimeArtifactEvidence struct {
	EventID         string                         `json:"event_id"`
	RunID           string                         `json:"run_id"`
	ArtifactID      string                         `json:"artifact_id"`
	Label           string                         `json:"label"`
	Title           string                         `json:"title,omitempty"`
	MediaType       string                         `json:"media_type"`
	URI             string                         `json:"uri,omitempty"`
	Summary         string                         `json:"summary,omitempty"`
	ProducerActorID string                         `json:"producer_actor_id,omitempty"`
	Causes          []OperatorRuntimeCauseEvidence `json:"causes"`
	CauseStatus     string                         `json:"cause_status"`
	CreatedAt       time.Time                      `json:"created_at"`
}

type OperatorRuntimeCauseEvidence struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type,omitempty"`
	Scope     string `json:"scope"`
}

type OperatorRuntimeEventEvidence struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	ConversationID string          `json:"conversation_id"`
	CreatedAt      time.Time       `json:"created_at"`
	Causes         []string        `json:"causes"`
	InspectorKind  string          `json:"inspector_kind"`
	Content        json.RawMessage `json:"content,omitempty"`
	ContentError   string          `json:"content_error,omitempty"`
}

type OperatorRuntimeCausalGraph struct {
	Scope          string                      `json:"scope"`
	ConversationID string                      `json:"conversation_id,omitempty"`
	Limit          int                         `json:"limit"`
	Truncated      bool                        `json:"truncated"`
	Nodes          []OperatorRuntimeCausalNode `json:"nodes"`
	Edges          []OperatorRuntimeCausalEdge `json:"edges"`
}

type OperatorRuntimeCausalNode struct {
	EventID    string    `json:"event_id"`
	EventType  string    `json:"event_type"`
	Label      string    `json:"label"`
	ArtifactID string    `json:"artifact_id,omitempty"`
	Scope      string    `json:"scope"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

type OperatorRuntimeCausalEdge struct {
	FromEventID string `json:"from_event_id"`
	ToEventID   string `json:"to_event_id"`
	Scope       string `json:"scope"`
}

type projectionEvent struct {
	event event.Event
}

type operatorProjectionOptions struct {
	modelSelectionSource OperatorModelSelectionSource
}

// OperatorProjectionOption configures BuildOperatorProjection.
type OperatorProjectionOption func(*operatorProjectionOptions)

// WithOperatorModelSelection uses config to build the read-only model-selection
// slice of the Site-facing operator projection.
func WithOperatorModelSelection(config OperatorModelSelectionConfig) OperatorProjectionOption {
	return func(o *operatorProjectionOptions) {
		o.modelSelectionSource = func() OperatorModelSelectionConfig { return config }
	}
}

// WithOperatorModelSelectionSource uses source to build the read-only
// model-selection slice at projection time. This lets long-running Hive
// processes expose catalog reloads without restarting Site.
func WithOperatorModelSelectionSource(source OperatorModelSelectionSource) OperatorProjectionOption {
	return func(o *operatorProjectionOptions) {
		o.modelSelectionSource = source
	}
}

// BuildOperatorProjection reads Hive Phase 3 records and returns bounded,
// display-ready operator state. It never appends or mutates EventGraph.
func BuildOperatorProjection(s store.Store, limit int, opts ...OperatorProjectionOption) OperatorProjection {
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	options := operatorProjectionOptions{
		modelSelectionSource: func() OperatorModelSelectionConfig {
			return DefaultOperatorModelSelectionConfig(time.Time{})
		},
	}
	for _, opt := range opts {
		opt(&options)
	}
	modelSelection := DefaultOperatorModelSelectionConfig(time.Time{})
	if options.modelSelectionSource != nil {
		modelSelection = options.modelSelectionSource()
	}
	p := OperatorProjection{
		GeneratedAt: time.Now().UTC(),
		Source:      "eventgraph",
	}
	modelSelection = applyModelRolePolicyUpdates(&p, s, modelSelection, limit)
	p.ModelSelection = BuildOperatorModelSelection(modelSelection)
	p.RuntimeEvidence = buildRuntimeEvidenceProjection(&p, s, limit)

	requestEvents := readProjectionEvents(&p, s, EventTypeAuthorityRequestRecorded, limit)
	decisionEvents := readProjectionEvents(&p, s, EventTypeAuthorityDecisionRecorded, limit)

	requests := make(map[string]OperatorApprovalProjection)
	requestIDs := make(map[string]struct{})
	for _, pe := range requestEvents {
		content, ok := pe.event.Content().(AuthorityRequestRecordedContent)
		if !ok {
			p.Errors = append(p.Errors, contentTypeError(pe.event, "AuthorityRequestRecordedContent"))
			continue
		}
		requestIDs[content.RequestID.Value()] = struct{}{}
		requests[content.RequestID.Value()] = OperatorApprovalProjection{
			EventID:           pe.event.ID().Value(),
			RequestID:         content.RequestID.Value(),
			RequestingActor:   content.RequestingActor.Value(),
			RequestingRole:    content.RequestingRole,
			ActionName:        content.ActionName,
			Target:            content.Target,
			Environment:       content.Environment,
			RiskClass:         content.RiskClass,
			RequestedOutcome:  content.RequestedOutcome,
			Justification:     content.Justification,
			RiskSummary:       content.RiskSummary,
			Scope:             append([]string(nil), content.Scope...),
			ProposedOperation: content.ProposedOperation,
			CreatedAt:         pe.event.Timestamp().Value(),
		}
	}

	decided := make(map[string]struct{})
	for _, pe := range decisionEvents {
		content, ok := pe.event.Content().(AuthorityDecisionRecordedContent)
		if !ok {
			p.Errors = append(p.Errors, contentTypeError(pe.event, "AuthorityDecisionRecordedContent"))
			continue
		}
		requestID := content.RequestID.Value()
		decided[requestID] = struct{}{}
		decision := OperatorDecisionProjection{
			EventID:        pe.event.ID().Value(),
			DecisionID:     content.DecisionID,
			RequestID:      requestID,
			ApproverActor:  content.ApproverActor.Value(),
			DeciderRole:    content.DeciderRole,
			Outcome:        content.Outcome,
			ApprovedAction: content.ApprovedAction,
			ApprovedTarget: content.ApprovedTarget,
			Scope:          append([]string(nil), content.Scope...),
			Rationale:      content.Rationale,
			CreatedAt:      pe.event.Timestamp().Value(),
		}
		if request, ok := requests[requestID]; ok {
			decision.RequestedAction = request.ActionName
			decision.RequestedTarget = request.Target
		}
		p.AuthorityDecisions = append(p.AuthorityDecisions, decision)
	}
	sort.Slice(p.AuthorityDecisions, func(i, j int) bool {
		return p.AuthorityDecisions[i].CreatedAt.After(p.AuthorityDecisions[j].CreatedAt)
	})
	decided = readDecisionRequestIDsForCandidates(&p, s, requestIDs, limit)

	for requestID, request := range requests {
		if _, ok := decided[requestID]; !ok {
			p.PendingApprovals = append(p.PendingApprovals, request)
		}
	}
	sort.Slice(p.PendingApprovals, func(i, j int) bool {
		return p.PendingApprovals[i].CreatedAt.After(p.PendingApprovals[j].CreatedAt)
	})

	p.Lifecycle = buildLifecycleProjection(&p, s, limit)
	p.KeyAuditTraces = buildKeyAuditProjection(&p, s, limit)
	return p
}

func readProjectionEvents(p *OperatorProjection, s store.Store, eventType types.EventType, limit int) []projectionEvent {
	page, err := s.ByType(eventType, limit, types.None[types.Cursor]())
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("read %s: %v", eventType.Value(), err))
		return nil
	}
	items := page.Items()
	events := make([]projectionEvent, 0, len(items))
	for _, item := range items {
		events = append(events, projectionEvent{event: item})
	}
	return events
}

func readDecisionRequestIDsForCandidates(p *OperatorProjection, s store.Store, requestIDs map[string]struct{}, limit int) map[string]struct{} {
	decided := make(map[string]struct{})
	if len(requestIDs) == 0 {
		return decided
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(EventTypeAuthorityDecisionRecorded, limit, cursor)
		if err != nil {
			p.Errors = append(p.Errors, fmt.Sprintf("read %s for pending resolution: %v", EventTypeAuthorityDecisionRecorded.Value(), err))
			return decided
		}
		for _, item := range page.Items() {
			content, ok := item.Content().(AuthorityDecisionRecordedContent)
			if !ok {
				p.Errors = append(p.Errors, contentTypeError(item, "AuthorityDecisionRecordedContent"))
				continue
			}
			requestID := content.RequestID.Value()
			if _, ok := requestIDs[requestID]; ok {
				decided[requestID] = struct{}{}
				if len(decided) == len(requestIDs) {
					return decided
				}
			}
		}
		if !page.HasMore() {
			return decided
		}
		cursor = page.Cursor()
	}
}

func buildLifecycleProjection(p *OperatorProjection, s store.Store, limit int) []OperatorLifecycleProjection {
	eventTypes := []types.EventType{
		EventTypeAgentIdentityRegistered,
		EventTypeAgentLifecycleTransitioned,
		EventTypeAgentAuthorityScopeAssigned,
		EventTypeAgentAuthorityScopeReduced,
		EventTypeAgentIdentityRevoked,
		EventTypeAgentIdentityRetired,
	}
	events := readProjectionEventsByTypes(p, s, eventTypes, limit)
	sort.Slice(events, func(i, j int) bool {
		return events[i].event.Timestamp().Value().Before(events[j].event.Timestamp().Value())
	})

	byActor := map[string]OperatorLifecycleProjection{}
	for _, pe := range events {
		timestamp := pe.event.Timestamp().Value()
		switch content := pe.event.Content().(type) {
		case AgentIdentityRegisteredContent:
			actorID := content.ActorID.Value()
			byActor[actorID] = OperatorLifecycleProjection{
				ActorID:         actorID,
				DisplayName:     content.DisplayName,
				Role:            content.Role,
				LifecycleStatus: content.LifecycleStatus,
				AuthorityScope:  content.AuthorityScope,
				KeyProvenance:   content.KeyProvenance,
				Environment:     content.Environment,
				IdentityMode:    content.IdentityMode,
				LastEventType:   pe.event.Type().Value(),
				UpdatedAt:       timestamp,
			}
		case AgentLifecycleTransitionedContent:
			updateLifecycleState(byActor, content.ActorID.Value(), pe.event.Type().Value(), content.ResultingState, timestamp)
		case AgentAuthorityScopeAssignedContent:
			updateAuthorityScope(byActor, content.ActorID.Value(), pe.event.Type().Value(), content.AuthorityScope, timestamp)
		case AgentAuthorityScopeReducedContent:
			updateAuthorityScope(byActor, content.ActorID.Value(), pe.event.Type().Value(), content.ResultingScope, timestamp)
		case AgentIdentityRevokedContent:
			updateLifecycleState(byActor, content.ActorID.Value(), pe.event.Type().Value(), "revoked", timestamp)
		case AgentIdentityRetiredContent:
			updateLifecycleState(byActor, content.ActorID.Value(), pe.event.Type().Value(), "retired", timestamp)
		default:
			p.Errors = append(p.Errors, contentTypeError(pe.event, "lifecycle content"))
		}
	}

	lifecycle := make([]OperatorLifecycleProjection, 0, len(byActor))
	for _, item := range byActor {
		lifecycle = append(lifecycle, item)
	}
	sort.Slice(lifecycle, func(i, j int) bool {
		return lifecycle[i].UpdatedAt.After(lifecycle[j].UpdatedAt)
	})
	if len(lifecycle) > limit {
		lifecycle = lifecycle[:limit]
	}
	return lifecycle
}

func readProjectionEventsByTypes(p *OperatorProjection, s store.Store, eventTypes []types.EventType, limit int) []projectionEvent {
	var events []projectionEvent
	for _, eventType := range eventTypes {
		events = append(events, readProjectionEvents(p, s, eventType, limit)...)
	}
	return events
}

func updateLifecycleState(items map[string]OperatorLifecycleProjection, actorID, eventType, state string, updatedAt time.Time) {
	item := items[actorID]
	item.ActorID = actorID
	item.LifecycleStatus = state
	item.LastEventType = eventType
	item.UpdatedAt = updatedAt
	items[actorID] = item
}

func updateAuthorityScope(items map[string]OperatorLifecycleProjection, actorID, eventType, scope string, updatedAt time.Time) {
	item := items[actorID]
	item.ActorID = actorID
	item.AuthorityScope = scope
	item.LastEventType = eventType
	item.UpdatedAt = updatedAt
	items[actorID] = item
}

func buildKeyAuditProjection(p *OperatorProjection, s store.Store, limit int) []OperatorKeyAuditProjection {
	eventTypes := []types.EventType{
		EventTypeAgentKeyGenerated,
		EventTypeAgentKeyRegistered,
		EventTypeAgentKeyRotated,
		EventTypeAgentKeyRevoked,
		EventTypeAgentAuditLinked,
	}
	events := readProjectionEventsByTypes(p, s, eventTypes, limit)
	sort.Slice(events, func(i, j int) bool {
		return events[i].event.Timestamp().Value().After(events[j].event.Timestamp().Value())
	})

	traces := make([]OperatorKeyAuditProjection, 0, len(events))
	for _, pe := range events {
		base := OperatorKeyAuditProjection{
			EventID:   pe.event.ID().Value(),
			EventType: pe.event.Type().Value(),
			CreatedAt: pe.event.Timestamp().Value(),
		}
		switch content := pe.event.Content().(type) {
		case AgentKeyGeneratedContent:
			base.ActorID = content.ActorID.Value()
			base.PublicKey = content.PublicKey.String()
			base.KeyProvenance = content.KeyProvenance
			base.Environment = content.Environment
			base.IdentityMode = content.IdentityMode
			base.Reason = content.Reason
		case AgentKeyRegisteredContent:
			base.ActorID = content.ActorID.Value()
			base.PublicKey = content.PublicKey.String()
			base.KeyProvenance = content.KeyProvenance
			base.Environment = content.Environment
			base.IdentityMode = content.IdentityMode
			base.ExternalKeyRef = content.ExternalKeyRef
		case AgentKeyRotatedContent:
			base.ActorID = content.ActorID.Value()
			base.OldPublicKey = content.OldPublicKey.String()
			base.NewPublicKey = content.NewPublicKey.String()
			base.Reason = content.Reason
			base.AuthorityRequest = content.AuthorityRequestID.Value()
			base.DecisionEvent = content.DecisionEventID.Value()
		case AgentKeyRevokedContent:
			base.ActorID = content.ActorID.Value()
			base.PublicKey = content.RevokedPublicKey.String()
			base.Reason = content.Reason
			base.AuthorityRequest = content.AuthorityRequestID.Value()
			base.DecisionEvent = content.DecisionEventID.Value()
		case AgentAuditLinkedContent:
			base.SubjectActorID = content.SubjectActorID.Value()
			base.RecordKind = content.RecordKind
			base.Rationale = content.Rationale
		default:
			p.Errors = append(p.Errors, contentTypeError(pe.event, "key/audit content"))
			continue
		}
		traces = append(traces, base)
	}
	if len(traces) > limit {
		traces = traces[:limit]
	}
	return traces
}

func buildRuntimeEvidenceProjection(p *OperatorProjection, s store.Store, limit int) OperatorRuntimeEvidence {
	evidence := OperatorRuntimeEvidence{
		Source: "eventgraph",
		Status: "not_observed",
		AgentEvents: OperatorRuntimeAgentEvents{
			Scope: "none",
		},
		Artifacts: []OperatorRuntimeArtifactEvidence{},
		RunEvents: []OperatorRuntimeEventEvidence{},
		CausalGraph: OperatorRuntimeCausalGraph{
			Scope: "none",
			Limit: limit,
			Nodes: []OperatorRuntimeCausalNode{},
			Edges: []OperatorRuntimeCausalEdge{},
		},
		Limitations: []string{
			"factory.run.requested is queued launch intent, not runtime-start proof",
			"hive.run.started and hive.run.completed prove Hive runtime event emission, not production deployment",
			"runtime start, agent, and completion events are correlated by EventGraph conversation ID",
			"runtime event order follows EventGraph store order, not wall-clock timestamp order",
			"artifact and causal graph projections are bounded by the operator projection limit",
		},
	}

	requestEvents := readProjectionEvents(p, s, EventTypeFactoryRunRequested, limit)
	if len(requestEvents) > 0 {
		pe := requestEvents[0]
		eventID := pe.event.ID().Value()
		conversationID := pe.event.ConversationID().Value()
		timestamp := pe.event.Timestamp().Value()
		content, ok := pe.event.Content().(FactoryRunRequestedContent)
		if !ok {
			p.Errors = append(p.Errors, contentTypeError(pe.event, "FactoryRunRequestedContent"))
		} else {
			budgetMaxIterations := content.Budget.MaxIterations
			budgetMaxCostUSD := content.Budget.MaxCostUSD
			evidence.LastQueuedRunRequest = &OperatorQueuedRunRequestEvidence{
				EventID:               eventID,
				ConversationID:        conversationID,
				RunID:                 content.RunID,
				Title:                 content.Title,
				OperatorID:            content.OperatorID,
				Status:                content.Status,
				TargetRepos:           append([]string(nil), content.TargetRepos...),
				AuthorityInitialLevel: string(content.Authority.InitialLevel),
				AuthorityScope:        content.Authority.Scope,
				BudgetMaxIterations:   &budgetMaxIterations,
				BudgetMaxCostUSD:      &budgetMaxCostUSD,
				SourceEventID:         content.SourceEventID.Value(),
				BriefEventID:          content.BriefEventID.Value(),
				EvidenceKind:          "queued_request_not_runtime_start",
				CreatedAt:             timestamp,
			}
		}
	}

	runStartEvents := readProjectionEvents(p, s, EventTypeRunStarted, limit)
	if len(runStartEvents) == 0 {
		return evidence
	}
	startEvent := runStartEvents[0]
	startContent, ok := startEvent.event.Content().(RunStartedContent)
	if !ok {
		p.Errors = append(p.Errors, contentTypeError(startEvent.event, "RunStartedContent"))
		return evidence
	}
	latestRunConversationID := startEvent.event.ConversationID()
	evidence.Status = "running"
	evidence.LastRun = &OperatorRuntimeRunEvidence{
		StartedEventID: startEvent.event.ID().Value(),
		ConversationID: latestRunConversationID.Value(),
		StartedAt:      startEvent.event.Timestamp().Value(),
		SeedIdea:       startContent.Idea,
		RepoPath:       startContent.RepoPath,
	}
	evidence.AgentEvents = OperatorRuntimeAgentEvents{
		Scope: "events_since_latest_hive.run.started",
	}

	conversationEvents, conversationTruncated := readProjectionEventsByConversation(p, s, latestRunConversationID, limit)
	runEvents := runtimeRunEvents(startEvent, conversationEvents)
	evidence.RunEvents = buildRuntimeEventEvidence(p, runEvents)
	evidence.Artifacts = buildRuntimeArtifactEvidence(runEvents)
	evidence.CausalGraph = buildRuntimeCausalGraph(runEvents, latestRunConversationID, limit, conversationTruncated)

	activeAgents := map[string]OperatorRuntimeAgentEvidence{}
	activeAgentKeysByNameRole := map[string][]string{}
	runClosed := false
	for _, pe := range runEvents {
		eventID := pe.event.ID().Value()
		timestamp := pe.event.Timestamp().Value()
		switch content := pe.event.Content().(type) {
		case AgentSpawnedContent:
			if runClosed {
				continue
			}
			agent := OperatorRuntimeAgentEvidence{
				Name:           content.Name,
				Role:           content.Role,
				Model:          content.Model,
				ActorID:        content.ActorID,
				SpawnedEventID: eventID,
				SpawnedAt:      timestamp,
			}
			agentKey := runtimeAgentKey(content.ActorID, content.Name, content.Role, eventID)
			if _, exists := activeAgents[agentKey]; !exists {
				nameRoleKey := runtimeAgentNameRoleKey(content.Name, content.Role)
				activeAgentKeysByNameRole[nameRoleKey] = append(activeAgentKeysByNameRole[nameRoleKey], agentKey)
			}
			activeAgents[agentKey] = agent
			evidence.AgentEvents.Spawned++
			setRuntimeLastAgentEvent(&evidence.AgentEvents, eventID, timestamp)
		case AgentStoppedContent:
			if runClosed {
				continue
			}
			deleteLatestRuntimeAgent(activeAgents, activeAgentKeysByNameRole, content.Name, content.Role)
			evidence.AgentEvents.Stopped++
			setRuntimeLastAgentEvent(&evidence.AgentEvents, eventID, timestamp)
		case RunCompletedContent:
			if runClosed {
				continue
			}
			completedAt := timestamp
			agentCount := content.AgentCount
			durationMs := content.DurationMs
			totalCost := content.TotalCost
			evidence.Status = "completed"
			evidence.LastRun.CompletedEventID = eventID
			evidence.LastRun.CompletedAt = &completedAt
			evidence.LastRun.AgentCount = &agentCount
			evidence.LastRun.DurationMs = &durationMs
			evidence.LastRun.TotalCost = &totalCost
			activeAgents = map[string]OperatorRuntimeAgentEvidence{}
			runClosed = true
		case RunStartedContent:
			continue
		case FactoryRunRequestedContent:
			continue
		default:
			continue
		}
	}

	evidence.AgentEvents.ActiveAgents = sortedRuntimeActiveAgents(activeAgents)
	evidence.AgentEvents.ObservedActive = len(evidence.AgentEvents.ActiveAgents)
	return evidence
}

func readProjectionEventsByConversation(p *OperatorProjection, s store.Store, conversationID types.ConversationID, limit int) ([]projectionEvent, bool) {
	// Store implementations return newest-first pages. buildRuntimeEvidenceProjection
	// walks this slice in reverse so run events are applied in EventGraph store order.
	page, err := s.ByConversation(conversationID, limit, types.None[types.Cursor]())
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("read conversation %s: %v", conversationID.Value(), err))
		return nil, false
	}
	items := page.Items()
	events := make([]projectionEvent, 0, len(items))
	for _, item := range items {
		events = append(events, projectionEvent{event: item})
	}
	return events, page.HasMore()
}

func runtimeRunEvents(startEvent projectionEvent, conversationEvents []projectionEvent) []projectionEvent {
	events := []projectionEvent{startEvent}
	seen := map[types.EventID]struct{}{startEvent.event.ID(): {}}
	startSeen := false
	startInWindow := containsProjectionEvent(conversationEvents, startEvent.event.ID())
	for i := len(conversationEvents) - 1; i >= 0; i-- {
		pe := conversationEvents[i]
		if pe.event.ID() == startEvent.event.ID() {
			startSeen = true
			continue
		}
		if !startSeen && startInWindow {
			continue
		}
		if _, ok := seen[pe.event.ID()]; ok {
			continue
		}
		events = append(events, pe)
		seen[pe.event.ID()] = struct{}{}
	}
	return events
}

func buildRuntimeEventEvidence(p *OperatorProjection, events []projectionEvent) []OperatorRuntimeEventEvidence {
	out := make([]OperatorRuntimeEventEvidence, 0, len(events))
	for _, pe := range events {
		content, contentErr := runtimeEventContent(pe.event)
		if contentErr != "" {
			p.Errors = append(p.Errors, fmt.Sprintf("marshal event inspector content for %s: %s", pe.event.ID().Value(), contentErr))
		}
		out = append(out, OperatorRuntimeEventEvidence{
			EventID:        pe.event.ID().Value(),
			EventType:      pe.event.Type().Value(),
			ConversationID: pe.event.ConversationID().Value(),
			CreatedAt:      pe.event.Timestamp().Value(),
			Causes:         eventCauseIDs(pe.event),
			InspectorKind:  "eventgraph_event",
			Content:        content,
			ContentError:   contentErr,
		})
	}
	return out
}

func buildRuntimeArtifactEvidence(events []projectionEvent) []OperatorRuntimeArtifactEvidence {
	nodeByID := runtimeEventTypeByID(events)
	artifacts := make([]OperatorRuntimeArtifactEvidence, 0)
	for _, pe := range events {
		content, ok := pe.event.Content().(FactoryArtifactCreatedContent)
		if !ok {
			continue
		}
		causes := runtimeCauseEvidence(pe.event, nodeByID)
		causeStatus := "caused"
		if len(causes) == 0 {
			causeStatus = "missing_causes"
		}
		artifacts = append(artifacts, OperatorRuntimeArtifactEvidence{
			EventID:         pe.event.ID().Value(),
			RunID:           content.RunID,
			ArtifactID:      content.ArtifactID,
			Label:           content.Label,
			Title:           content.Title,
			MediaType:       content.MediaType,
			URI:             content.URI,
			Summary:         content.Summary,
			ProducerActorID: content.ProducerActorID,
			Causes:          causes,
			CauseStatus:     causeStatus,
			CreatedAt:       pe.event.Timestamp().Value(),
		})
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].CreatedAt.After(artifacts[j].CreatedAt)
	})
	return artifacts
}

func buildRuntimeCausalGraph(events []projectionEvent, conversationID types.ConversationID, limit int, truncated bool) OperatorRuntimeCausalGraph {
	graph := OperatorRuntimeCausalGraph{
		Scope:          "latest_run_conversation",
		ConversationID: conversationID.Value(),
		Limit:          limit,
		Truncated:      truncated,
		Nodes:          []OperatorRuntimeCausalNode{},
		Edges:          []OperatorRuntimeCausalEdge{},
	}
	nodeByID := map[string]OperatorRuntimeCausalNode{}
	for _, pe := range events {
		node := runtimeCausalNode(pe, "run")
		graph.Nodes = append(graph.Nodes, node)
		nodeByID[node.EventID] = node
	}
	for _, pe := range events {
		toID := pe.event.ID().Value()
		for _, cause := range pe.event.Causes() {
			fromID := cause.Value()
			scope := "run"
			if _, ok := nodeByID[fromID]; !ok {
				scope = "external_or_out_of_window"
				node := OperatorRuntimeCausalNode{
					EventID:   fromID,
					EventType: "external_or_out_of_window",
					Label:     "External or out-of-window cause",
					Scope:     scope,
				}
				graph.Nodes = append(graph.Nodes, node)
				nodeByID[fromID] = node
			}
			graph.Edges = append(graph.Edges, OperatorRuntimeCausalEdge{
				FromEventID: fromID,
				ToEventID:   toID,
				Scope:       scope,
			})
		}
	}
	return graph
}

func runtimeEventTypeByID(events []projectionEvent) map[string]string {
	out := make(map[string]string, len(events))
	for _, pe := range events {
		out[pe.event.ID().Value()] = pe.event.Type().Value()
	}
	return out
}

func runtimeCauseEvidence(ev event.Event, nodeByID map[string]string) []OperatorRuntimeCauseEvidence {
	causes := ev.Causes()
	out := make([]OperatorRuntimeCauseEvidence, 0, len(causes))
	for _, cause := range causes {
		item := OperatorRuntimeCauseEvidence{
			EventID: cause.Value(),
			Scope:   "external_or_out_of_window",
		}
		if eventType, ok := nodeByID[cause.Value()]; ok {
			item.EventType = eventType
			item.Scope = "run"
		}
		out = append(out, item)
	}
	return out
}

func runtimeCausalNode(pe projectionEvent, scope string) OperatorRuntimeCausalNode {
	node := OperatorRuntimeCausalNode{
		EventID:   pe.event.ID().Value(),
		EventType: pe.event.Type().Value(),
		Label:     runtimeEventLabel(pe.event),
		Scope:     scope,
		CreatedAt: pe.event.Timestamp().Value(),
	}
	if content, ok := pe.event.Content().(FactoryArtifactCreatedContent); ok {
		node.ArtifactID = content.ArtifactID
	}
	return node
}

func runtimeEventLabel(ev event.Event) string {
	switch content := ev.Content().(type) {
	case RunStartedContent:
		if content.Idea != "" {
			return "Run started: " + content.Idea
		}
		return "Run started"
	case AgentSpawnedContent:
		return "Agent spawned: " + content.Role + "/" + content.Name
	case AgentStoppedContent:
		return "Agent stopped: " + content.Role + "/" + content.Name
	case RunCompletedContent:
		return "Run completed"
	case FactoryArtifactCreatedContent:
		if content.Title != "" {
			return "Artifact: " + content.Title
		}
		if content.Label != "" {
			return "Artifact: " + content.Label
		}
		return "Artifact created"
	default:
		return ev.Type().Value()
	}
}

func runtimeEventContent(ev event.Event) (json.RawMessage, string) {
	content, err := json.Marshal(ev.Content())
	if err != nil {
		return nil, err.Error()
	}
	return content, ""
}

func eventCauseIDs(ev event.Event) []string {
	causes := ev.Causes()
	out := make([]string, 0, len(causes))
	for _, cause := range causes {
		out = append(out, cause.Value())
	}
	return out
}

func containsProjectionEvent(events []projectionEvent, eventID types.EventID) bool {
	for _, pe := range events {
		if pe.event.ID() == eventID {
			return true
		}
	}
	return false
}

func deleteLatestRuntimeAgent(active map[string]OperatorRuntimeAgentEvidence, keysByNameRole map[string][]string, name, role string) {
	nameRoleKey := runtimeAgentNameRoleKey(name, role)
	keys := keysByNameRole[nameRoleKey]
	for len(keys) > 0 {
		lastIndex := len(keys) - 1
		agentKey := keys[lastIndex]
		keys = keys[:lastIndex]
		if _, ok := active[agentKey]; ok {
			delete(active, agentKey)
			break
		}
	}
	if len(keys) == 0 {
		delete(keysByNameRole, nameRoleKey)
		return
	}
	keysByNameRole[nameRoleKey] = keys
}

func setRuntimeLastAgentEvent(agentEvents *OperatorRuntimeAgentEvents, eventID string, timestamp time.Time) {
	eventAt := timestamp
	agentEvents.LastAgentEventID = eventID
	agentEvents.LastAgentEventAt = &eventAt
}

func sortedRuntimeActiveAgents(active map[string]OperatorRuntimeAgentEvidence) []OperatorRuntimeAgentEvidence {
	agents := make([]OperatorRuntimeAgentEvidence, 0, len(active))
	for _, agent := range active {
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Role != agents[j].Role {
			return agents[i].Role < agents[j].Role
		}
		if agents[i].Name != agents[j].Name {
			return agents[i].Name < agents[j].Name
		}
		if agents[i].ActorID != agents[j].ActorID {
			return agents[i].ActorID < agents[j].ActorID
		}
		return agents[i].SpawnedEventID < agents[j].SpawnedEventID
	})
	return agents
}

func runtimeAgentKey(actorID, name, role, spawnedEventID string) string {
	if actorID != "" {
		return actorID
	}
	return runtimeAgentNameRoleKey(name, role) + "/" + spawnedEventID
}

func runtimeAgentNameRoleKey(name, role string) string {
	return role + "/" + name
}

func contentTypeError(ev event.Event, want string) string {
	return fmt.Sprintf("%s %s has unexpected content type %T, want %s", ev.Type().Value(), ev.ID().Value(), ev.Content(), want)
}
