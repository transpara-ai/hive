package hive

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	civilizationAssemblyProjectionSchemaVersion = "1.0.0"
	civilizationAssemblyProjectionSubject       = "civilization_assembly"

	civilizationAssemblyStatusComplete    = "complete"
	civilizationAssemblyStatusPartial     = "partial"
	civilizationAssemblyFieldAvailable    = "available"
	civilizationAssemblyFieldUnavailable  = "unavailable"
	civilizationAssemblyReadOnlyRoutePath = "GET /api/hive/civilization/assembly-projection"
	civilizationAssemblyWorkTaskRef       = "event_type:work.task.created field:FactoryOrderID"
)

// CivilizationAssemblyProjection is Hive's read-only Site-facing Civilization
// visualization payload. It is derived from the existing operator projection so
// EventGraph remains the source of truth and Site remains a consumer only.
type CivilizationAssemblyProjection struct {
	ProjectionID                       string                                 `json:"projection_id"`
	ProjectionSchemaVersion            string                                 `json:"projection_schema_version"`
	ProjectionSubject                  string                                 `json:"projection_subject"`
	GeneratedAt                        time.Time                              `json:"generated_at"`
	SourceEventGraphHeadOrStateVersion string                                 `json:"source_eventgraph_head_or_state_version"`
	SourceEventIDsOrQueryWindow        []string                               `json:"source_event_ids_or_query_window"`
	DerivationStatus                   string                                 `json:"derivation_status"`
	AuthorityState                     CivilizationAssemblyAuthorityState     `json:"authority_state"`
	ExternalCommitteeState             CivilizationAssemblyCommitteeState     `json:"external_committee_state"`
	ActorRoster                        []CivilizationAssemblyActorSummary     `json:"actor_roster"`
	RoleBindings                       []CivilizationAssemblyRoleBinding      `json:"role_bindings"`
	AgentLifecycleSummary              []CivilizationAssemblyLifecycleSummary `json:"agent_lifecycle_summary"`
	FactoryOrderSummary                []CivilizationAssemblyFactoryOrder     `json:"factory_order_summary"`
	WorkEvidenceSummary                CivilizationAssemblyWorkEvidence       `json:"work_evidence_summary"`
	SiteConsumerStatus                 CivilizationAssemblyFieldStatus        `json:"site_consumer_status"`
	OpenGateSummary                    []CivilizationAssemblyGateSummary      `json:"open_gate_summary"`
	ResidualRiskSummary                []CivilizationAssemblyResidualRisk     `json:"residual_risk_summary"`
	WithheldOrUnavailableFields        []CivilizationAssemblyUnavailableField `json:"withheld_or_unavailable_fields"`
	BoundaryFlags                      []string                               `json:"boundary_flags"`
	ProvenanceRefs                     []string                               `json:"provenance_refs"`
	ValidationRefs                     []string                               `json:"validation_refs"`
	FailureReasons                     []string                               `json:"failure_reasons,omitempty"`
}

type CivilizationAssemblyFieldStatus struct {
	Status     string   `json:"status"`
	Summary    string   `json:"summary"`
	SourceRefs []string `json:"source_refs,omitempty"`
}

type CivilizationAssemblyUnavailableField struct {
	Field  string `json:"field"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type CivilizationAssemblyAuthorityState struct {
	Status             string                                  `json:"status"`
	Summary            string                                  `json:"summary"`
	AuthorityRequests  []CivilizationAssemblyAuthorityRequest  `json:"authority_requests,omitempty"`
	AuthorityDecisions []CivilizationAssemblyAuthorityDecision `json:"authority_decisions,omitempty"`
	ExecutionReceipts  []CivilizationAssemblyExecutionReceipt  `json:"execution_receipts,omitempty"`
	SourceRefs         []string                                `json:"source_refs,omitempty"`
}

type CivilizationAssemblyAuthorityRequest struct {
	ID         string `json:"id"`
	ActorID    string `json:"actor_id"`
	ActorRole  string `json:"actor_role"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	RiskClass  string `json:"risk_class"`
	Status     string `json:"status,omitempty"`
}

type CivilizationAssemblyAuthorityDecision struct {
	ID                 string   `json:"id"`
	AuthorityRequestID string   `json:"authority_request_id"`
	DeciderActorID     string   `json:"decider_actor_id"`
	DeciderRole        string   `json:"decider_role"`
	Decision           string   `json:"decision"`
	Status             string   `json:"status,omitempty"`
	Scope              []string `json:"scope,omitempty"`
}

type CivilizationAssemblyExecutionReceipt struct {
	ID                  string `json:"id"`
	AuthorityDecisionID string `json:"authority_decision_id"`
	Action              string `json:"action"`
	TargetID            string `json:"target_id"`
	Result              string `json:"result"`
	Status              string `json:"status,omitempty"`
}

type CivilizationAssemblyCommitteeState struct {
	Status         string   `json:"status"`
	Summary        string   `json:"summary"`
	DecisionRefs   []string `json:"decision_refs,omitempty"`
	ApprovalRefs   []string `json:"approval_refs,omitempty"`
	CommitteeRoles []string `json:"committee_roles,omitempty"`
}

type CivilizationAssemblyActorSummary struct {
	ID           string `json:"id"`
	ActorID      string `json:"actor_id"`
	ActorType    string `json:"actor_type"`
	IdentityMode string `json:"identity_mode"`
	Status       string `json:"status,omitempty"`
}

type CivilizationAssemblyRoleBinding struct {
	ActorID    string `json:"actor_id"`
	Role       string `json:"role"`
	SourceRef  string `json:"source_ref"`
	SourceType string `json:"source_type"`
}

type CivilizationAssemblyLifecycleSummary struct {
	ID                  string  `json:"id"`
	ActorID             string  `json:"actor_id"`
	FromState           string  `json:"from_state,omitempty"`
	ToState             string  `json:"to_state,omitempty"`
	TrustLevel          string  `json:"trust_level,omitempty"`
	AuthorityDecisionID *string `json:"authority_decision_id,omitempty"`
	Status              string  `json:"status,omitempty"`
}

type CivilizationAssemblyFactoryOrder struct {
	ID                      string   `json:"id"`
	Status                  string   `json:"status,omitempty"`
	RiskClass               string   `json:"risk_class"`
	ReleasePolicy           string   `json:"release_policy"`
	RequirementRefs         []string `json:"requirement_refs,omitempty"`
	AcceptanceCriterionRefs []string `json:"acceptance_criterion_refs,omitempty"`
	TaskRefs                []string `json:"task_refs,omitempty"`
	ReleaseCandidateRefs    []string `json:"release_candidate_refs,omitempty"`
}

type CivilizationAssemblyWorkEvidence struct {
	Status          string   `json:"status"`
	Summary         string   `json:"summary"`
	TaskRefs        []string `json:"task_refs,omitempty"`
	ArtifactRefs    []string `json:"artifact_refs,omitempty"`
	TestRunRefs     []string `json:"test_run_refs,omitempty"`
	GateResultRefs  []string `json:"gate_result_refs,omitempty"`
	AuditReportRefs []string `json:"audit_report_refs,omitempty"`
	SourceRefs      []string `json:"source_refs,omitempty"`
}

type CivilizationAssemblyGateSummary struct {
	ID                 string   `json:"id"`
	GateName           string   `json:"gate_name"`
	Status             string   `json:"status,omitempty"`
	FactoryOrderID     string   `json:"factory_order_id,omitempty"`
	ReleaseCandidateID *string  `json:"release_candidate_id,omitempty"`
	EvidenceRefs       []string `json:"evidence_refs,omitempty"`
}

type CivilizationAssemblyResidualRisk struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity,omitempty"`
	Status   string `json:"status,omitempty"`
	Summary  string `json:"summary"`
}

type civilizationAssemblyFactoryOrderWorkEvidence struct {
	TaskRefs                    []string
	TestRunRefs                 []string
	GateResultRefs              []string
	SourceRefs                  []string
	LifecycleSourceTruncated    bool
	VerificationSourceTruncated bool
}

type civilizationAssemblyTaskLifecycleEvidence struct {
	SourceRefs []string
}

type civilizationAssemblyTaskVerificationEvidence struct {
	TestRunRefs    []string
	GateResultRefs []string
	SourceRefs     []string
}

// BuildCivilizationAssemblyProjection derives the Site visualization projection
// from the Hive operator projection. It is read-only and performs no EventGraph
// writes, protected actions, or runtime execution.
func BuildCivilizationAssemblyProjection(s store.Store, limit int, opts ...OperatorProjectionOption) CivilizationAssemblyProjection {
	operatorProjection := BuildOperatorProjection(s, limit, opts...)
	factoryOrders, factoryOrderWorkEvidence, factoryOrdersTruncated, factoryOrdersQueryFailed := civilizationAssemblyFactoryOrders(&operatorProjection, s, limit)
	sourceRefs := compactStrings(append(civilizationAssemblySourceRefs(operatorProjection), factoryOrderWorkEvidence.SourceRefs...))
	head := civilizationAssemblyHead(s, &operatorProjection)
	status := civilizationAssemblyStatus(operatorProjection)

	projection := CivilizationAssemblyProjection{
		ProjectionID:                       civilizationAssemblyProjectionID(head, operatorProjection.GeneratedAt),
		ProjectionSchemaVersion:            civilizationAssemblyProjectionSchemaVersion,
		ProjectionSubject:                  civilizationAssemblyProjectionSubject,
		GeneratedAt:                        operatorProjection.GeneratedAt,
		SourceEventGraphHeadOrStateVersion: head,
		SourceEventIDsOrQueryWindow:        sourceRefs,
		DerivationStatus:                   status,
		AuthorityState:                     civilizationAssemblyAuthorityState(operatorProjection),
		ExternalCommitteeState:             civilizationAssemblyCommitteeState(operatorProjection),
		ActorRoster:                        civilizationAssemblyActorRoster(operatorProjection),
		RoleBindings:                       civilizationAssemblyRoleBindings(operatorProjection),
		AgentLifecycleSummary:              civilizationAssemblyLifecycle(operatorProjection),
		FactoryOrderSummary:                factoryOrders,
		WorkEvidenceSummary:                civilizationAssemblyWorkEvidence(operatorProjection, factoryOrders, factoryOrderWorkEvidence),
		SiteConsumerStatus: CivilizationAssemblyFieldStatus{
			Status:     civilizationAssemblyFieldAvailable,
			Summary:    "Hive exposes a read-only Civilization Assembly projection for Site rendering; Site is not a graph writer or executor.",
			SourceRefs: []string{civilizationAssemblyReadOnlyRoutePath},
		},
		OpenGateSummary:             civilizationAssemblyOpenGates(operatorProjection),
		ResidualRiskSummary:         civilizationAssemblyResidualRisks(operatorProjection, factoryOrdersTruncated, factoryOrderWorkEvidence, limit),
		WithheldOrUnavailableFields: civilizationAssemblyUnavailableFields(operatorProjection, factoryOrders, factoryOrdersQueryFailed),
		BoundaryFlags: []string{
			"eventgraph_derived",
			"hive_owned_projection",
			"read_only_site_consumer",
			"no_site_eventgraph_writes",
			"no_runtime_execution",
			"no_protected_action_approval",
		},
		ProvenanceRefs: sourceRefs,
		ValidationRefs: []string{
			"BuildOperatorProjection",
			civilizationAssemblyWorkTaskRef,
			civilizationAssemblyReadOnlyRoutePath,
		},
		FailureReasons: append([]string(nil), operatorProjection.Errors...),
	}
	sortCivilizationAssemblyProjection(&projection)
	return projection
}

func civilizationAssemblyStatus(p OperatorProjection) string {
	if len(p.Errors) > 0 {
		return civilizationAssemblyStatusPartial
	}
	return civilizationAssemblyStatusComplete
}

func civilizationAssemblyHead(s store.Store, p *OperatorProjection) string {
	head, err := s.Head()
	if err != nil {
		p.Errors = append(p.Errors, "read eventgraph head: "+err.Error())
		return "eventgraph head unavailable"
	}
	if !head.IsSome() {
		return "eventgraph head unavailable"
	}
	return head.Unwrap().ID().Value()
}

func civilizationAssemblyProjectionID(head string, generatedAt time.Time) string {
	if head == "" || head == "eventgraph head unavailable" {
		return "civ-assembly-" + generatedAt.UTC().Format("20060102T150405Z")
	}
	return "civ-assembly-" + sanitizeProjectionToken(head)
}

func civilizationAssemblyAuthorityState(p OperatorProjection) CivilizationAssemblyAuthorityState {
	requests := make([]CivilizationAssemblyAuthorityRequest, 0, len(p.PendingApprovals))
	refs := make([]string, 0, len(p.PendingApprovals)+len(p.AuthorityDecisions))
	for _, request := range p.PendingApprovals {
		requests = append(requests, CivilizationAssemblyAuthorityRequest{
			ID:         valueOr(request.RequestID, request.EventID),
			ActorID:    request.RequestingActor,
			ActorRole:  request.RequestingRole,
			Action:     request.ActionName,
			TargetType: civilizationAssemblyTargetType(request.Target),
			TargetID:   request.Target,
			RiskClass:  request.RiskClass,
			Status:     "pending",
		})
		refs = append(refs, request.EventID)
	}
	decisions := make([]CivilizationAssemblyAuthorityDecision, 0, len(p.AuthorityDecisions))
	for _, decision := range p.AuthorityDecisions {
		decisions = append(decisions, CivilizationAssemblyAuthorityDecision{
			ID:                 valueOr(decision.DecisionID, decision.EventID),
			AuthorityRequestID: decision.RequestID,
			DeciderActorID:     decision.ApproverActor,
			DeciderRole:        decision.DeciderRole,
			Decision:           decision.Outcome,
			Status:             decision.Outcome,
			Scope:              append([]string(nil), decision.Scope...),
		})
		refs = append(refs, decision.EventID)
	}
	status := civilizationAssemblyFieldUnavailable
	summary := "no authority requests or decisions are projected by Hive operator state"
	if len(requests)+len(decisions) > 0 {
		status = civilizationAssemblyFieldAvailable
		summary = fmt.Sprintf("%d pending request(s), %d recorded decision(s) projected from Hive operator state", len(requests), len(decisions))
	}
	return CivilizationAssemblyAuthorityState{
		Status:             status,
		Summary:            summary,
		AuthorityRequests:  requests,
		AuthorityDecisions: decisions,
		SourceRefs:         compactStrings(refs),
	}
}

func civilizationAssemblyCommitteeState(p OperatorProjection) CivilizationAssemblyCommitteeState {
	return CivilizationAssemblyCommitteeState{
		Status:         civilizationAssemblyFieldUnavailable,
		Summary:        "External Committee approval records are not independently projected by Hive operator state.",
		CommitteeRoles: []string{"External Committee"},
	}
}

func civilizationAssemblyActorRoster(p OperatorProjection) []CivilizationAssemblyActorSummary {
	actors := map[string]CivilizationAssemblyActorSummary{}
	for _, item := range p.Lifecycle {
		actorID := strings.TrimSpace(item.ActorID)
		if actorID == "" {
			continue
		}
		actors[actorID] = CivilizationAssemblyActorSummary{
			ID:           "lifecycle:" + actorID,
			ActorID:      actorID,
			ActorType:    "agent",
			IdentityMode: valueOr(item.IdentityMode, "projected"),
			Status:       valueOr(item.LifecycleStatus, "projected"),
		}
	}
	for _, item := range p.RuntimeEvidence.AgentEvents.ActiveAgents {
		actorID := strings.TrimSpace(item.ActorID)
		if actorID == "" {
			actorID = strings.TrimSpace(item.Name)
		}
		if actorID == "" {
			continue
		}
		if existing, ok := actors[actorID]; ok {
			if existing.Status == "" || existing.Status == "projected" {
				existing.Status = "active"
			}
			actors[actorID] = existing
			continue
		}
		actors[actorID] = CivilizationAssemblyActorSummary{
			ID:           valueOr(item.SpawnedEventID, "runtime:"+actorID),
			ActorID:      actorID,
			ActorType:    "agent",
			IdentityMode: "runtime",
			Status:       "active",
		}
	}
	out := make([]CivilizationAssemblyActorSummary, 0, len(actors))
	for _, actor := range actors {
		out = append(out, actor)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ActorID < out[j].ActorID
	})
	return out
}

func civilizationAssemblyRoleBindings(p OperatorProjection) []CivilizationAssemblyRoleBinding {
	out := make([]CivilizationAssemblyRoleBinding, 0, len(p.Lifecycle)+len(p.RuntimeEvidence.AgentEvents.ActiveAgents))
	seen := map[string]bool{}
	add := func(actorID, role, sourceRef, sourceType string) {
		actorID = strings.TrimSpace(actorID)
		role = strings.TrimSpace(role)
		if actorID == "" || role == "" {
			return
		}
		key := actorID + "\x00" + role
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, CivilizationAssemblyRoleBinding{
			ActorID:    actorID,
			Role:       role,
			SourceRef:  valueOr(sourceRef, sourceType),
			SourceType: sourceType,
		})
	}
	for _, item := range p.Lifecycle {
		add(item.ActorID, item.Role, item.LastEventType, "agent.lifecycle")
	}
	for _, item := range p.RuntimeEvidence.AgentEvents.ActiveAgents {
		actorID := valueOr(item.ActorID, item.Name)
		add(actorID, item.Role, item.SpawnedEventID, "hive.agent.spawned")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role == out[j].Role {
			return out[i].ActorID < out[j].ActorID
		}
		return out[i].Role < out[j].Role
	})
	return out
}

func civilizationAssemblyLifecycle(p OperatorProjection) []CivilizationAssemblyLifecycleSummary {
	byActor := make(map[string]CivilizationAssemblyLifecycleSummary, len(p.Lifecycle)+len(p.RuntimeEvidence.AgentEvents.ActiveAgents))
	for _, item := range p.Lifecycle {
		actorID := strings.TrimSpace(item.ActorID)
		if actorID == "" {
			continue
		}
		byActor[actorID] = CivilizationAssemblyLifecycleSummary{
			ID:      "lifecycle:" + item.ActorID,
			ActorID: actorID,
			ToState: item.LifecycleStatus,
			Status:  item.LifecycleStatus,
		}
	}
	for _, item := range p.RuntimeEvidence.AgentEvents.ActiveAgents {
		actorID := valueOr(item.ActorID, item.Name)
		if actorID == "" {
			continue
		}
		existing := byActor[actorID]
		if existing.ID == "" {
			existing.ID = valueOr(item.SpawnedEventID, "runtime:"+actorID)
		}
		existing.ActorID = actorID
		existing.ToState = "active"
		existing.Status = "active"
		byActor[actorID] = existing
	}
	out := make([]CivilizationAssemblyLifecycleSummary, 0, len(byActor))
	for _, item := range byActor {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActorID == out[j].ActorID {
			return out[i].ID < out[j].ID
		}
		return out[i].ActorID < out[j].ActorID
	})
	return out
}

func civilizationAssemblyFactoryOrders(p *OperatorProjection, s store.Store, limit int) ([]CivilizationAssemblyFactoryOrder, civilizationAssemblyFactoryOrderWorkEvidence, bool, bool) {
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	page, err := s.ByType(work.EventTypeTaskCreated, limit, types.None[types.Cursor]())
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("read %s for civilization factory orders: %v", work.EventTypeTaskCreated.Value(), err))
		return []CivilizationAssemblyFactoryOrder{}, civilizationAssemblyFactoryOrderWorkEvidence{}, false, true
	}
	truncated := page.HasMore()

	byID := map[string]*CivilizationAssemblyFactoryOrder{}
	taskStore := work.NewTaskStore(s, nil, nil)
	workEvidence := civilizationAssemblyFactoryOrderWorkEvidence{}
	lifecycleByTask, lifecycleTruncated, err := civilizationAssemblyWorkTaskLifecycleEvidence(s, limit)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("project Work task lifecycle source refs: %v", err))
	}
	workEvidence.LifecycleSourceTruncated = lifecycleTruncated
	verificationByTask, verificationTruncated, err := civilizationAssemblyWorkTaskVerificationEvidence(s, limit)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("project Work task verification evidence refs: %v", err))
	}
	workEvidence.VerificationSourceTruncated = verificationTruncated
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskCreatedContent)
		if !ok {
			p.Errors = append(p.Errors, contentTypeError(ev, "work.TaskCreatedContent"))
			continue
		}
		orderID := strings.TrimSpace(content.FactoryOrderID)
		if orderID == "" {
			continue
		}
		order := byID[orderID]
		if order == nil {
			order = &CivilizationAssemblyFactoryOrder{
				ID:            orderID,
				Status:        "work_task_seeded",
				RiskClass:     valueOr(content.RiskClass, "unknown"),
				ReleasePolicy: "human_required_before_merge",
			}
			byID[orderID] = order
		}
		order.RiskClass = civilizationAssemblyFactoryOrderRiskClass(order.RiskClass, content.RiskClass)
		order.RequirementRefs = compactStrings(append(order.RequirementRefs, content.RequirementIDs...))
		order.AcceptanceCriterionRefs = compactStrings(append(order.AcceptanceCriterionRefs, content.AcceptanceCriterionIDs...))
		order.TaskRefs = compactStrings(append(order.TaskRefs, ev.ID().Value()))
		workEvidence.TaskRefs = append(workEvidence.TaskRefs, ev.ID().Value())
		workEvidence.SourceRefs = append(workEvidence.SourceRefs, ev.ID().Value())

		taskProjection, legacyProjection, err := civilizationAssemblyProjectWorkTask(taskStore, ev.ID())
		if err != nil {
			p.Errors = append(p.Errors, fmt.Sprintf("project Work task %s for civilization factory order %s: %v", ev.ID().Value(), orderID, err))
			continue
		}
		order.Status = civilizationAssemblyFactoryOrderStatus(order.Status, civilizationAssemblyProjectedWorkTaskStatus(taskProjection, legacyProjection))
		if lifecycleEvidence, ok := lifecycleByTask[ev.ID()]; ok {
			workEvidence.SourceRefs = append(workEvidence.SourceRefs, lifecycleEvidence.SourceRefs...)
		}
		if verificationEvidence, ok := verificationByTask[ev.ID()]; ok {
			workEvidence.TestRunRefs = append(workEvidence.TestRunRefs, verificationEvidence.TestRunRefs...)
			workEvidence.GateResultRefs = append(workEvidence.GateResultRefs, verificationEvidence.GateResultRefs...)
			workEvidence.SourceRefs = append(workEvidence.SourceRefs, verificationEvidence.SourceRefs...)
		}
	}
	workEvidence.TaskRefs = compactStrings(workEvidence.TaskRefs)
	workEvidence.TestRunRefs = compactStrings(workEvidence.TestRunRefs)
	workEvidence.GateResultRefs = compactStrings(workEvidence.GateResultRefs)
	workEvidence.SourceRefs = compactStrings(workEvidence.SourceRefs)

	out := make([]CivilizationAssemblyFactoryOrder, 0, len(byID))
	for _, order := range byID {
		sort.Strings(order.RequirementRefs)
		sort.Strings(order.AcceptanceCriterionRefs)
		sort.Strings(order.TaskRefs)
		out = append(out, *order)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, workEvidence, truncated, false
}

func civilizationAssemblyProjectWorkTask(taskStore *work.TaskStore, taskID types.EventID) (work.TaskProjection, work.LegacyTaskProjection, error) {
	taskProjection, err := taskStore.ProjectTask(taskID)
	if err != nil {
		return work.TaskProjection{}, work.LegacyTaskProjection{}, err
	}
	legacyProjection, err := taskStore.ProjectLegacyTask(taskID)
	if err != nil {
		return work.TaskProjection{}, work.LegacyTaskProjection{}, err
	}
	return taskProjection, legacyProjection, nil
}

func civilizationAssemblyWorkTaskLifecycleEvidence(s store.Store, limit int) (map[types.EventID]civilizationAssemblyTaskLifecycleEvidence, bool, error) {
	page, err := s.ByType(work.EventTypeTaskLifecycleTransitioned, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, fmt.Errorf("fetch %s events: %w", work.EventTypeTaskLifecycleTransitioned.Value(), err)
	}
	byTask := map[types.EventID]civilizationAssemblyTaskLifecycleEvidence{}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskLifecycleTransitionContent)
		if !ok {
			return nil, false, fmt.Errorf("%s", contentTypeError(ev, "work.TaskLifecycleTransitionContent"))
		}
		evidence := byTask[content.TaskID]
		evidence.SourceRefs = append(evidence.SourceRefs, ev.ID().Value())
		byTask[content.TaskID] = evidence
	}
	for taskID, evidence := range byTask {
		evidence.SourceRefs = compactStrings(evidence.SourceRefs)
		byTask[taskID] = evidence
	}
	return byTask, page.HasMore(), nil
}

func civilizationAssemblyWorkTaskVerificationEvidence(s store.Store, limit int) (map[types.EventID]civilizationAssemblyTaskVerificationEvidence, bool, error) {
	page, err := s.ByType(work.EventTypeTaskVerificationAttached, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, fmt.Errorf("fetch %s events: %w", work.EventTypeTaskVerificationAttached.Value(), err)
	}
	byTask := map[types.EventID]civilizationAssemblyTaskVerificationEvidence{}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskVerificationAttachedContent)
		if !ok {
			return nil, false, fmt.Errorf("%s", contentTypeError(ev, "work.TaskVerificationAttachedContent"))
		}
		evidence := byTask[content.TaskID]
		evidence.TestRunRefs = append(evidence.TestRunRefs, content.TestRunIDs...)
		evidence.GateResultRefs = append(evidence.GateResultRefs, content.GateResultIDs...)
		evidence.SourceRefs = append(evidence.SourceRefs, ev.ID().Value())
		byTask[content.TaskID] = evidence
	}
	for taskID, evidence := range byTask {
		evidence.TestRunRefs = compactStrings(evidence.TestRunRefs)
		evidence.GateResultRefs = compactStrings(evidence.GateResultRefs)
		evidence.SourceRefs = compactStrings(evidence.SourceRefs)
		byTask[taskID] = evidence
	}
	return byTask, page.HasMore(), nil
}

func civilizationAssemblyFactoryOrderRiskClass(existing, candidate string) string {
	existing = valueOr(existing, "unknown")
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return existing
	}
	if civilizationAssemblyRiskClassRank(candidate) > civilizationAssemblyRiskClassRank(existing) {
		return candidate
	}
	return existing
}

func civilizationAssemblyRiskClassRank(riskClass string) int {
	switch strings.ToLower(strings.TrimSpace(riskClass)) {
	case "critical":
		return 50
	case "high":
		return 40
	case "medium":
		return 30
	case "low":
		return 20
	case "info", "informational":
		return 10
	case "unknown", "":
		return 0
	default:
		return 5
	}
}

func civilizationAssemblyProjectedWorkTaskStatus(taskProjection work.TaskProjection, legacyProjection work.LegacyTaskProjection) string {
	if taskProjection.Status != "" && taskProjection.Status != work.StatusCreated {
		return "work_task_" + string(taskProjection.Status)
	}
	switch legacyProjection.Status {
	case work.LegacyStatusCompleted:
		return "work_task_completed"
	case work.LegacyStatusBlocked:
		return "work_task_blocked"
	case work.LegacyStatusAssigned:
		return "work_task_assigned"
	case work.LegacyStatusReady:
		return "work_task_ready"
	}
	if taskProjection.Blocked || legacyProjection.Blocked {
		return "work_task_blocked"
	}
	if taskProjection.Ready || legacyProjection.Ready {
		return "work_task_ready"
	}
	return "work_task_seeded"
}

func civilizationAssemblyFactoryOrderStatus(existing, candidate string) string {
	if existing == "" {
		return candidate
	}
	if candidate == "" {
		return existing
	}
	if civilizationAssemblyFactoryOrderStatusRank(candidate) > civilizationAssemblyFactoryOrderStatusRank(existing) {
		return candidate
	}
	return existing
}

// FactoryOrder status is a highest-attention rollup across its Work task refs:
// failure/blocking states dominate active work, which dominates terminal
// success, then assignment/readiness/seeded states.
func civilizationAssemblyFactoryOrderStatusRank(status string) int {
	switch status {
	case "work_task_failed", "work_task_repair_required", "work_task_rejected", "work_task_policy_blocked", "work_task_blocked":
		return 100
	case "work_task_repair_running":
		return 90
	case "work_task_running", "work_task_verification_running":
		return 80
	case "work_task_repaired":
		return 70
	case "work_task_certified", "work_task_verified", "work_task_completed":
		return 60
	case "work_task_assigned":
		return 50
	case "work_task_ready":
		return 40
	case "work_task_seeded", "work_task_pending", "work_task_created":
		return 10
	default:
		return 20
	}
}

func civilizationAssemblyWorkEvidence(p OperatorProjection, factoryOrders []CivilizationAssemblyFactoryOrder, factoryOrderWorkEvidence civilizationAssemblyFactoryOrderWorkEvidence) CivilizationAssemblyWorkEvidence {
	refs := []string{}
	if queued := p.RuntimeEvidence.LastQueuedRunRequest; queued != nil {
		refs = append(refs, queued.EventID, queued.SourceEventID, queued.BriefEventID)
	}
	if run := p.RuntimeEvidence.LastRun; run != nil {
		refs = append(refs, run.StartedEventID, run.CompletedEventID)
	}
	artifactRefs := make([]string, 0, len(p.RuntimeEvidence.Artifacts))
	for _, artifact := range p.RuntimeEvidence.Artifacts {
		artifactRefs = append(artifactRefs, valueOr(artifact.ArtifactID, artifact.EventID))
		refs = append(refs, artifact.EventID)
	}
	for _, event := range p.RuntimeEvidence.RunEvents {
		refs = append(refs, event.EventID)
	}
	taskRefs := factoryOrderWorkEvidence.TaskRefs
	refs = append(refs, factoryOrderWorkEvidence.SourceRefs...)
	sourceRefs := compactStrings(refs)
	status := civilizationAssemblyFieldUnavailable
	summary := "Hive operator projection has not observed queued run or runtime evidence."
	if len(factoryOrders) > 0 {
		status = civilizationAssemblyFieldAvailable
		runtimeStatus := strings.TrimSpace(p.RuntimeEvidence.Status)
		if runtimeStatus == "" || runtimeStatus == "not_observed" {
			summary = fmt.Sprintf("%d Work FactoryOrder seed task(s) projected; no runtime run is observed in this projection.", len(factoryOrders))
		} else {
			summary = fmt.Sprintf("%d Work FactoryOrder seed task(s) projected with runtime evidence status %q", len(factoryOrders), runtimeStatus)
		}
	} else if len(sourceRefs) > 0 || p.RuntimeEvidence.Status != "" && p.RuntimeEvidence.Status != "not_observed" {
		status = civilizationAssemblyFieldAvailable
		summary = fmt.Sprintf("runtime evidence status %q derived from Hive operator projection", p.RuntimeEvidence.Status)
	}
	return CivilizationAssemblyWorkEvidence{
		Status:         status,
		Summary:        summary,
		TaskRefs:       compactStrings(taskRefs),
		ArtifactRefs:   compactStrings(artifactRefs),
		TestRunRefs:    compactStrings(factoryOrderWorkEvidence.TestRunRefs),
		GateResultRefs: compactStrings(factoryOrderWorkEvidence.GateResultRefs),
		SourceRefs:     sourceRefs,
	}
}

func civilizationAssemblyOpenGates(p OperatorProjection) []CivilizationAssemblyGateSummary {
	gates := make([]CivilizationAssemblyGateSummary, 0, len(p.PendingApprovals))
	for _, request := range p.PendingApprovals {
		gates = append(gates, CivilizationAssemblyGateSummary{
			ID:           valueOr(request.RequestID, request.EventID),
			GateName:     valueOr(request.ActionName, "pending authority request"),
			Status:       "pending",
			EvidenceRefs: compactStrings([]string{request.EventID}),
		})
	}
	return gates
}

func civilizationAssemblyResidualRisks(p OperatorProjection, factoryOrdersTruncated bool, factoryOrderWorkEvidence civilizationAssemblyFactoryOrderWorkEvidence, limit int) []CivilizationAssemblyResidualRisk {
	risks := make([]CivilizationAssemblyResidualRisk, 0, len(p.RuntimeEvidence.Limitations)+len(p.Errors)+3)
	for i, limitation := range p.RuntimeEvidence.Limitations {
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       fmt.Sprintf("runtime_limitation_%02d", i+1),
			Kind:     "runtime_projection_limitation",
			Severity: "info",
			Status:   "open",
			Summary:  limitation,
		})
	}
	for i, err := range p.Errors {
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       fmt.Sprintf("projection_error_%02d", i+1),
			Kind:     "operator_projection_error",
			Severity: "high",
			Status:   "open",
			Summary:  err,
		})
	}
	if factoryOrdersTruncated {
		if limit <= 0 {
			limit = defaultOperatorProjectionLimit
		}
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       "factory_order_summary_limit_01",
			Kind:     "factory_order_projection_limit",
			Severity: "info",
			Status:   "open",
			Summary:  fmt.Sprintf("FactoryOrder summary is bounded to %d work.task.created events; records outside that projection page may be omitted.", limit),
		})
	}
	if factoryOrderWorkEvidence.LifecycleSourceTruncated {
		if limit <= 0 {
			limit = defaultOperatorProjectionLimit
		}
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       "factory_order_lifecycle_source_limit_01",
			Kind:     "factory_order_projection_limit",
			Severity: "info",
			Status:   "open",
			Summary:  fmt.Sprintf("FactoryOrder lifecycle provenance is bounded to %d work.task.lifecycle.transitioned events; transition source refs outside that projection page may be omitted.", limit),
		})
	}
	if factoryOrderWorkEvidence.VerificationSourceTruncated {
		if limit <= 0 {
			limit = defaultOperatorProjectionLimit
		}
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       "factory_order_verification_source_limit_01",
			Kind:     "factory_order_projection_limit",
			Severity: "info",
			Status:   "open",
			Summary:  fmt.Sprintf("FactoryOrder verification evidence is bounded to %d work.task.verification.attached events; evidence refs outside that projection page may be omitted.", limit),
		})
	}
	return risks
}

func civilizationAssemblyUnavailableFields(p OperatorProjection, factoryOrders []CivilizationAssemblyFactoryOrder, factoryOrdersQueryFailed bool) []CivilizationAssemblyUnavailableField {
	fields := []CivilizationAssemblyUnavailableField{
		{
			Field:  "external_committee_state",
			Status: civilizationAssemblyFieldUnavailable,
			Reason: "Hive operator projection does not independently certify External Committee approval.",
		},
	}
	if len(factoryOrders) == 0 {
		reason := "No Work FactoryOrder seed task with FactoryOrderID has been observed in the projection window."
		if factoryOrdersQueryFailed {
			reason = "Work FactoryOrder seed task query failed; see FailureReasons and ResidualRiskSummary for the store error."
		}
		fields = append(fields, CivilizationAssemblyUnavailableField{
			Field:  "factory_order_summary",
			Status: civilizationAssemblyFieldUnavailable,
			Reason: reason,
		})
	}
	fields = append(fields, CivilizationAssemblyUnavailableField{
		Field:  "execution_receipts",
		Status: civilizationAssemblyFieldUnavailable,
		Reason: "Hive operator projection does not expose protected-action execution receipt records.",
	})
	if len(p.Lifecycle) == 0 && len(p.RuntimeEvidence.AgentEvents.ActiveAgents) == 0 {
		fields = append(fields, CivilizationAssemblyUnavailableField{
			Field:  "actor_roster",
			Status: civilizationAssemblyFieldUnavailable,
			Reason: "no lifecycle or active runtime agent records were projected",
		})
	}
	return fields
}

func civilizationAssemblySourceRefs(p OperatorProjection) []string {
	refs := []string{}
	for _, request := range p.PendingApprovals {
		refs = append(refs, request.EventID)
	}
	for _, decision := range p.AuthorityDecisions {
		refs = append(refs, decision.EventID)
	}
	for _, audit := range p.KeyAuditTraces {
		refs = append(refs, audit.EventID)
	}
	if queued := p.RuntimeEvidence.LastQueuedRunRequest; queued != nil {
		refs = append(refs, queued.EventID, queued.SourceEventID, queued.BriefEventID)
	}
	if run := p.RuntimeEvidence.LastRun; run != nil {
		refs = append(refs, run.StartedEventID, run.CompletedEventID)
	}
	for _, event := range p.RuntimeEvidence.RunEvents {
		refs = append(refs, event.EventID)
	}
	for _, artifact := range p.RuntimeEvidence.Artifacts {
		refs = append(refs, artifact.EventID)
	}
	return compactStrings(refs)
}

func sortCivilizationAssemblyProjection(p *CivilizationAssemblyProjection) {
	sort.Strings(p.BoundaryFlags)
	sort.Strings(p.ProvenanceRefs)
	sort.Strings(p.ValidationRefs)
	sort.Strings(p.FailureReasons)
	sort.Strings(p.SourceEventIDsOrQueryWindow)
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func civilizationAssemblyTargetType(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if before, _, ok := strings.Cut(target, "://"); ok && before != "" {
		return before
	}
	if strings.HasPrefix(target, "actor_") || strings.HasPrefix(target, "actor-") {
		return "actor"
	}
	if before, _, ok := strings.Cut(target, ":"); ok && before != "" {
		return before
	}
	return "target"
}

func valueOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func sanitizeProjectionToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(":", "-", "/", "-", "\\", "-", " ", "-").Replace(value)
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}
