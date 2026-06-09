package hive

import (
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

type projectionEvent struct {
	event event.Event
}

type operatorProjectionOptions struct {
	modelSelection OperatorModelSelectionConfig
}

// OperatorProjectionOption configures BuildOperatorProjection.
type OperatorProjectionOption func(*operatorProjectionOptions)

// WithOperatorModelSelection uses config to build the read-only model-selection
// slice of the Site-facing operator projection.
func WithOperatorModelSelection(config OperatorModelSelectionConfig) OperatorProjectionOption {
	return func(o *operatorProjectionOptions) {
		o.modelSelection = config
	}
}

// BuildOperatorProjection reads Hive Phase 3 records and returns bounded,
// display-ready operator state. It never appends or mutates EventGraph.
func BuildOperatorProjection(s store.Store, limit int, opts ...OperatorProjectionOption) OperatorProjection {
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	options := operatorProjectionOptions{
		modelSelection: DefaultOperatorModelSelectionConfig(time.Time{}),
	}
	for _, opt := range opts {
		opt(&options)
	}
	p := OperatorProjection{
		GeneratedAt:    time.Now().UTC(),
		Source:         "eventgraph",
		ModelSelection: BuildOperatorModelSelection(options.modelSelection),
	}

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

func contentTypeError(ev event.Event, want string) string {
	return fmt.Sprintf("%s %s has unexpected content type %T, want %s", ev.Type().Value(), ev.ID().Value(), ev.Content(), want)
}
