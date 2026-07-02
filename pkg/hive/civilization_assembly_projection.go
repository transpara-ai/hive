package hive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	civilizationAssemblyProjectionSchemaVersion = "1.6.0"
	civilizationAssemblyProjectionSubject       = "civilization_assembly"

	civilizationAssemblyStatusComplete    = "complete"
	civilizationAssemblyStatusPartial     = "partial"
	civilizationAssemblyFieldAvailable    = "available"
	civilizationAssemblyFieldUnavailable  = "unavailable"
	civilizationAssemblyReadOnlyRoutePath = "GET /api/hive/civilization/assembly-projection"
	civilizationAssemblyWorkTaskRef       = "event_type:work.task.created field:FactoryOrderID"

	civilizationAssemblyQueuedStageDeclaredStatus     = "declared_pending_runtime_evidence"
	civilizationAssemblyQueuedPlanStepDeclaredStatus  = "stage_declared_pending_runtime_evidence"
	civilizationAssemblyQueuedStageTaskDraftStatus    = "task_draft_seeded_pending_runtime_evidence"
	civilizationAssemblyQueuedPlanStepTaskDraftStatus = "stage_task_draft_seeded_pending_runtime_evidence"
	civilizationAssemblyQueuedStageRuntimeStatus      = "runtime_evidence_recorded"
	civilizationAssemblyQueuedPlanStepRuntimeStatus   = "stage_runtime_evidence_recorded"
	civilizationAssemblyQueuedStageCompletedStatus    = "stage_completed_runtime_evidence_recorded"
	civilizationAssemblyQueuedPlanStepCompletedStatus = "stage_step_completed_runtime_evidence_recorded"
	civilizationAssemblyIssueScanStageArtifactKind    = "issue_scan_lifecycle_stage"
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
	QueuedRunRequest                   *CivilizationAssemblyQueuedRunRequest  `json:"queued_run_request,omitempty"`
	IssueIntakeProjection              CivilizationIssueIntakeProjection      `json:"issue_intake_projection,omitempty"`
	IssueScanProjection                CivilizationIssueScanProjection        `json:"issue_scan_projection,omitempty"`
	// RecentIssueScanRuns has NO omitempty: it is always populated (status
	// becomes "unavailable" with zero runs and an honest summary when the
	// store has no qualifying evidence) so Site can rely on `status` being
	// present without special-casing a missing field.
	RecentIssueScanRuns         CivilizationRecentIssueScanRuns        `json:"recent_issue_scan_runs"`
	SiteConsumerStatus          CivilizationAssemblyFieldStatus        `json:"site_consumer_status"`
	OpenGateSummary             []CivilizationAssemblyGateSummary      `json:"open_gate_summary"`
	ResidualRiskSummary         []CivilizationAssemblyResidualRisk     `json:"residual_risk_summary"`
	WithheldOrUnavailableFields []CivilizationAssemblyUnavailableField `json:"withheld_or_unavailable_fields"`
	BoundaryFlags               []string                               `json:"boundary_flags"`
	ProvenanceRefs              []string                               `json:"provenance_refs"`
	ValidationRefs              []string                               `json:"validation_refs"`
	FailureReasons              []string                               `json:"failure_reasons,omitempty"`
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
	Status   string                             `json:"status"`
	Summary  string                             `json:"summary"`
	TaskRefs []string                           `json:"task_refs,omitempty"`
	Tasks    []CivilizationAssemblyTaskEvidence `json:"tasks,omitempty"`
	// ArtifactRefs are opaque evidence refs for display/audit. Runtime artifacts
	// use their logical artifact ID when present; Work task artifacts have no
	// separate logical artifact ID, so their EventGraph event ID is the ref.
	ArtifactRefs []string `json:"artifact_refs,omitempty"`
	// Artifacts contains structured metadata for Work task artifacts only;
	// runtime artifacts remain exposed through ArtifactRefs.
	Artifacts       []CivilizationAssemblyArtifactEvidence `json:"artifacts,omitempty"`
	TestRunRefs     []string                               `json:"test_run_refs,omitempty"`
	GateResultRefs  []string                               `json:"gate_result_refs,omitempty"`
	AuditReportRefs []string                               `json:"audit_report_refs,omitempty"`
	SourceRefs      []string                               `json:"source_refs,omitempty"`
}

type CivilizationAssemblyTaskEvidence struct {
	ID                      string                                   `json:"id"`
	CanonicalTaskID         string                                   `json:"canonical_task_id,omitempty"`
	FactoryOrderID          string                                   `json:"factory_order_id,omitempty"`
	LifecycleStageID        string                                   `json:"lifecycle_stage_id,omitempty"`
	Title                   string                                   `json:"title"`
	Cell                    string                                   `json:"cell,omitempty"`
	RiskClass               string                                   `json:"risk_class,omitempty"`
	Status                  string                                   `json:"status"`
	Ready                   bool                                     `json:"ready"`
	Blocked                 bool                                     `json:"blocked"`
	RequirementRefs         []string                                 `json:"requirement_refs,omitempty"`
	AcceptanceCriterionRefs []string                                 `json:"acceptance_criterion_refs,omitempty"`
	ExpectedOutputs         []string                                 `json:"expected_outputs,omitempty"`
	DependsOnRefs           []string                                 `json:"depends_on_refs,omitempty"`
	SourceRefs              []string                                 `json:"source_refs,omitempty"`
	RequiredRoles           []string                                 `json:"required_roles,omitempty"`
	RoleContractRefs        []string                                 `json:"role_contract_refs,omitempty"`
	AgentExecutionPlan      []OperatorQueuedRunAgentPlanStep         `json:"agent_execution_plan,omitempty"`
	RequiredEvidence        []string                                 `json:"required_evidence,omitempty"`
	OutputContractRefs      []string                                 `json:"output_contract_refs,omitempty"`
	RoleOutputContracts     []CivilizationAssemblyRoleOutputContract `json:"role_output_contracts,omitempty"`
	RuntimeEvidenceRefs     []string                                 `json:"runtime_evidence_refs,omitempty"`
	RuntimeEvidenceStatus   string                                   `json:"runtime_evidence_status,omitempty"`
}

type CivilizationAssemblyRoleOutputContract struct {
	Role              string   `json:"role"`
	CanOperate        bool     `json:"can_operate"`
	RequiredOutputs   []string `json:"required_outputs,omitempty"`
	AuthorityBoundary string   `json:"authority_boundary,omitempty"`
	CompletionGate    string   `json:"completion_gate,omitempty"`
	EvidenceStatus    string   `json:"evidence_status,omitempty"`
}

type CivilizationAssemblyArtifactEvidence struct {
	ID         string   `json:"id"`
	TaskRef    string   `json:"task_ref"`
	Label      string   `json:"label"`
	MediaType  string   `json:"media_type,omitempty"`
	SourceRefs []string `json:"source_refs,omitempty"`
}

// CivilizationAssemblyQueuedRunRequest is the read-only issue-scan run intent
// shown on the Site Civilization Assembly page. It intentionally reuses the
// operator projection's queued-run contract so Site renders one canonical view
// of expected lifecycle evidence.
type CivilizationAssemblyQueuedRunRequest = OperatorQueuedRunRequestEvidence

type CivilizationIssueRef struct {
	Repo        string   `json:"repo"`
	Number      int      `json:"number"`
	URL         string   `json:"url,omitempty"`
	Title       string   `json:"title,omitempty"`
	State       string   `json:"state,omitempty"`
	StateReason string   `json:"state_reason,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

type CivilizationIssueIntakeProjection struct {
	Status            string                                  `json:"status,omitempty"`
	Summary           string                                  `json:"summary,omitempty"`
	Issues            []CivilizationIssueIntakeProjected      `json:"issues,omitempty"`
	Groups            []CivilizationIssueIntakeGroupProjected `json:"groups,omitempty"`
	SourceRefs        []string                                `json:"source_refs,omitempty"`
	ScannerBoundaries []string                                `json:"scanner_boundaries,omitempty"`
}

type CivilizationIssueIntakeProjected struct {
	Repo              string   `json:"repo"`
	Number            int      `json:"number"`
	URL               string   `json:"url,omitempty"`
	Title             string   `json:"title,omitempty"`
	State             string   `json:"state,omitempty"`
	StateReason       string   `json:"state_reason,omitempty"`
	Labels            []string `json:"labels,omitempty"`
	PrimaryRepo       string   `json:"primary_repo,omitempty"`
	TouchedSubstrate  string   `json:"touched_substrate,omitempty"`
	TouchedSubstrates []string `json:"touched_substrates,omitempty"`
	RiskClass         string   `json:"risk_class,omitempty"`
	RiskClasses       []string `json:"risk_classes,omitempty"`
	UnrecognizedRisk  []string `json:"unrecognized_risk_terms,omitempty"`
	Readiness         string   `json:"readiness,omitempty"`
	ReadinessStates   []string `json:"readiness_states,omitempty"`
	PRReadyWhen       string   `json:"pr_ready_when,omitempty"`
	AuthorityBoundary string   `json:"authority_boundary,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	SourceRefs        []string `json:"source_refs,omitempty"`
}

type CivilizationIssueIntakeGroupProjected struct {
	GroupID          string                 `json:"group_id,omitempty"`
	Summary          string                 `json:"summary,omitempty"`
	PrimaryRepo      string                 `json:"primary_repo,omitempty"`
	TouchedSubstrate string                 `json:"touched_substrate,omitempty"`
	RiskClass        string                 `json:"risk_class,omitempty"`
	Readiness        string                 `json:"readiness,omitempty"`
	Recommendation   string                 `json:"recommendation,omitempty"`
	IssueRefs        []CivilizationIssueRef `json:"issue_refs,omitempty"`
	Blockers         []string               `json:"blockers,omitempty"`
	SourceRefs       []string               `json:"source_refs,omitempty"`
}

type CivilizationIssueScanProjection struct {
	Runs     []CivilizationIssueScanRunProjected     `json:"runs,omitempty"`
	Stages   []CivilizationIssueScanStageProjected   `json:"stages,omitempty"`
	Blockers []CivilizationIssueScanBlockerProjected `json:"blockers,omitempty"`
	Lineage  []CivilizationIssueScanLineageProjected `json:"lineage,omitempty"`
}

type CivilizationIssueScanRunProjected struct {
	RunID            string                 `json:"run_id"`
	FactoryOrderID   string                 `json:"factory_order_id,omitempty"`
	LifecycleVersion string                 `json:"lifecycle_version"`
	State            string                 `json:"state"`
	TargetIssue      CivilizationIssueRef   `json:"target_issue"`
	SelectedIssue    CivilizationIssueRef   `json:"selected_issue"`
	CandidateIssues  []CivilizationIssueRef `json:"candidate_issues,omitempty"`
	SourceRefs       []string               `json:"source_refs,omitempty"`
	EvidenceRefs     []string               `json:"evidence_refs,omitempty"`
}

type CivilizationIssueScanStageProjected struct {
	RunID             string   `json:"run_id"`
	FactoryOrderID    string   `json:"factory_order_id,omitempty"`
	StageID           string   `json:"stage_id"`
	StageNumber       int      `json:"stage_number"`
	StageCount        int      `json:"stage_count,omitempty"`
	CanonicalTaskID   string   `json:"canonical_task_id"`
	TaskID            string   `json:"task_id,omitempty"`
	CurrentState      string   `json:"current_state"`
	CompletionGate    string   `json:"completion_gate"`
	AuthorityBoundary string   `json:"authority_boundary"`
	AssignedAgentIDs  []string `json:"assigned_agent_ids,omitempty"`
	TouchingAgentIDs  []string `json:"touching_agent_ids,omitempty"`
	EvidenceRefs      []string `json:"evidence_refs,omitempty"`
	SourceRefs        []string `json:"source_refs,omitempty"`
}

type CivilizationIssueScanBlockerProjected struct {
	RunID          string   `json:"run_id"`
	FactoryOrderID string   `json:"factory_order_id,omitempty"`
	StageID        string   `json:"stage_id,omitempty"`
	BlockerType    string   `json:"blocker_type"`
	Reason         string   `json:"reason,omitempty"`
	RequiredAction string   `json:"required_action"`
	EvidenceRefs   []string `json:"evidence_refs,omitempty"`
	SourceRefs     []string `json:"source_refs,omitempty"`
}

type CivilizationIssueScanLineageProjected struct {
	RunID             string   `json:"run_id"`
	FactoryOrderID    string   `json:"factory_order_id,omitempty"`
	StageID           string   `json:"stage_id,omitempty"`
	CanonicalTaskID   string   `json:"canonical_task_id"`
	PrimaryTaskID     string   `json:"primary_task_id,omitempty"`
	TaskIDs           []string `json:"task_ids"`
	DuplicateTaskIDs  []string `json:"duplicate_task_ids,omitempty"`
	DuplicateOf       string   `json:"duplicate_of,omitempty"`
	SupersededTaskIDs []string `json:"superseded_task_ids,omitempty"`
	SourceRefs        []string `json:"source_refs,omitempty"`
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
	Tasks                       []CivilizationAssemblyTaskEvidence
	ArtifactRefs                []string
	Artifacts                   []CivilizationAssemblyArtifactEvidence
	StageArtifactRefs           []civilizationAssemblyStageArtifactRef
	TestRunRefs                 []string
	GateResultRefs              []string
	SourceRefs                  []string
	ArtifactSourceTruncated     bool
	DependencySourceTruncated   bool
	LifecycleSourceTruncated    bool
	VerificationSourceTruncated bool
}

type civilizationAssemblyIssueScanProjectionEvidence struct {
	Intake     CivilizationIssueIntakeProjection
	Scan       CivilizationIssueScanProjection
	SourceRefs []string
	Truncated  bool
	Errors     []string
}

type civilizationAssemblyTaskArtifactEvidence struct {
	ArtifactRefs             []string
	Artifacts                []CivilizationAssemblyArtifactEvidence
	StageArtifactRefs        []civilizationAssemblyStageArtifactRef
	RoleContract             civilizationAssemblyTaskRoleContract
	RoleContractConflicted   bool
	OutputContract           civilizationAssemblyTaskOutputContract
	OutputContractConflicted bool
	RuntimeEvidence          civilizationAssemblyTaskRuntimeEvidence
	SourceRefs               []string
}

type civilizationAssemblyStageArtifactRef struct {
	RunID    string
	StageID  string
	EventRef string
}

type civilizationAssemblyTaskRoleContract struct {
	StageID            string
	FactoryOrderID     string
	RequiredRoles      []string
	AgentExecutionPlan []OperatorQueuedRunAgentPlanStep
	SourceRefs         []string
}

type civilizationAssemblyTaskOutputContract struct {
	StageID             string
	FactoryOrderID      string
	RequiredEvidence    []string
	ExpectedOutputs     []string
	RoleOutputContracts []CivilizationAssemblyRoleOutputContract
	SourceRefs          []string
}

type civilizationAssemblyTaskRuntimeEvidence struct {
	StageID          string
	FactoryOrderID   string
	Status           string
	RequiredEvidence []string
	Roles            []string
	RoleOutputRoles  []string
	RoleOutputRefs   []string
	SourceRefs       []string
}

type civilizationAssemblyTaskLifecycleEvidence struct {
	SourceRefs []string
}

type civilizationAssemblyTaskDependencyEvidence struct {
	DependsOnRefs []string
	SourceRefs    []string
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
	// The parked-run page is fetched ONCE here and feeds BOTH the board fold
	// (issue intake/issue-scan projections) and the recent-runs rail fold.
	// parkedOK and parkedFetched are DIFFERENT signals (see
	// civilizationAssemblyNormalizedParkedRuns and civilizationRecentIssueScanRuns
	// doc comments): parkedOK is the board fold's short-circuit (false for a
	// fetch failure OR a confirmably-empty page); parkedFetched is narrower
	// and only false when the page could not be read at all, which is what
	// the rail fold needs to distinguish "parked-absence proven" from
	// "parked-absence unknowable".
	normalizedParkedRuns, parkedTruncated, parkedEvidence, parkedOK, parkedFetched := civilizationAssemblyNormalizedParkedRuns(s, limit)
	issueScanEvidence := civilizationAssemblyIssueScanProjections(normalizedParkedRuns, parkedTruncated, parkedEvidence, parkedOK, limit)
	operatorProjection.Errors = append(operatorProjection.Errors, issueScanEvidence.Errors...)
	requestedEvents, requestedTruncated, requestedErr := civilizationAssemblyFactoryRunRequestedEvents(s, limit)
	requestedFetched := requestedErr == nil
	if requestedErr != nil {
		operatorProjection.Errors = append(operatorProjection.Errors, "project recent issue-scan requested runs: "+requestedErr.Error())
	}
	recentIssueScanRuns := civilizationRecentIssueScanRuns(normalizedParkedRuns, parkedTruncated, parkedFetched, requestedEvents, requestedTruncated, requestedFetched, factoryOrders, factoryOrderWorkEvidence, factoryOrdersQueryFailed, factoryOrdersTruncated)
	sourceRefs := compactStrings(append(append(append(civilizationAssemblySourceRefs(operatorProjection), factoryOrderWorkEvidence.SourceRefs...), issueScanEvidence.SourceRefs...), civilizationRecentIssueScanSourceRefs(recentIssueScanRuns)...))
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
		QueuedRunRequest:                   civilizationAssemblyQueuedRunRequestWithStageEvidence(operatorProjection.RuntimeEvidence.LastQueuedRunRequest, factoryOrderWorkEvidence.StageArtifactRefs, factoryOrderWorkEvidence.Tasks),
		IssueIntakeProjection:              issueScanEvidence.Intake,
		IssueScanProjection:                issueScanEvidence.Scan,
		RecentIssueScanRuns:                recentIssueScanRuns,
		SiteConsumerStatus: CivilizationAssemblyFieldStatus{
			Status:     civilizationAssemblyFieldAvailable,
			Summary:    "Hive exposes a read-only Civilization Assembly projection for Site rendering; Site is not a graph writer or executor.",
			SourceRefs: []string{civilizationAssemblyReadOnlyRoutePath},
		},
		OpenGateSummary:             civilizationAssemblyOpenGates(operatorProjection),
		ResidualRiskSummary:         civilizationAssemblyResidualRisks(operatorProjection, factoryOrdersTruncated, factoryOrderWorkEvidence, issueScanEvidence.Truncated, limit),
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

func civilizationAssemblyIssueScanProjections(normalized []civilizationAssemblyNormalizedParkedRun, truncated bool, evidence civilizationAssemblyIssueScanProjectionEvidence, ok bool, limit int) civilizationAssemblyIssueScanProjectionEvidence {
	if !ok {
		return evidence
	}
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	intake := evidence.Intake
	scan := evidence.Scan
	if len(normalized) == 0 {
		return civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, Truncated: truncated}
	}

	sourceRefs := make([]string, 0, len(normalized))
	intake.Issues = make([]CivilizationIssueIntakeProjected, 0, len(normalized))
	scan.Runs = make([]CivilizationIssueScanRunProjected, 0, len(normalized))
	scan.Blockers = make([]CivilizationIssueScanBlockerProjected, 0, len(normalized))
	for _, run := range normalized {
		sourceRefs = append(sourceRefs, run.Refs...)
		intake.Issues = append(intake.Issues, CivilizationIssueIntakeProjected{
			Repo:              run.Issue.Repo,
			Number:            run.Issue.Number,
			URL:               run.Issue.URL,
			Title:             run.Issue.Title,
			State:             run.Issue.State,
			StateReason:       run.Issue.StateReason,
			Labels:            append([]string(nil), run.Issue.Labels...),
			PrimaryRepo:       run.Issue.Repo,
			TouchedSubstrate:  civilizationAssemblyIssueTouchedSubstrate(run.Issue.Repo),
			TouchedSubstrates: []string{civilizationAssemblyIssueTouchedSubstrate(run.Issue.Repo)},
			RiskClass:         run.RiskClass,
			RiskClasses:       run.RiskClasses,
			Readiness:         run.Readiness,
			ReadinessStates:   run.ReadinessStates,
			PRReadyWhen:       run.RawRequiredAction,
			AuthorityBoundary: run.AuthorityBoundary,
			UpdatedAt:         run.EventTimestamp.UTC().Format(time.RFC3339Nano),
			SourceRefs:        run.Refs,
		})
		scan.Runs = append(scan.Runs, CivilizationIssueScanRunProjected{
			RunID:            run.RunID,
			FactoryOrderID:   run.FactoryOrderID,
			LifecycleVersion: run.LifecycleVersion,
			State:            run.State,
			TargetIssue:      run.Issue,
			SelectedIssue:    run.Issue,
			CandidateIssues:  []CivilizationIssueRef{run.Issue},
			SourceRefs:       run.Refs,
			EvidenceRefs:     []string{run.EventID},
		})
		scan.Blockers = append(scan.Blockers, run.Blockers...)
	}
	if len(intake.Issues) == 0 {
		return civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, SourceRefs: compactStrings(sourceRefs), Truncated: truncated}
	}
	sort.Slice(intake.Issues, func(i, j int) bool {
		if intake.Issues[i].Repo == intake.Issues[j].Repo {
			return intake.Issues[i].Number < intake.Issues[j].Number
		}
		return intake.Issues[i].Repo < intake.Issues[j].Repo
	})
	sort.Slice(scan.Runs, func(i, j int) bool {
		return scan.Runs[i].RunID < scan.Runs[j].RunID
	})
	sort.Slice(scan.Blockers, func(i, j int) bool {
		if scan.Blockers[i].RunID == scan.Blockers[j].RunID {
			return scan.Blockers[i].BlockerType < scan.Blockers[j].BlockerType
		}
		return scan.Blockers[i].RunID < scan.Blockers[j].RunID
	})
	intake.Status = civilizationAssemblyFieldAvailable
	intake.Summary = fmt.Sprintf("%d scanner-visible issue(s) projected from parked issue-scan evidence.", len(intake.Issues))
	if truncated {
		intake.Summary = fmt.Sprintf("%s Projection is truncated at %d parked issue-scan event(s).", intake.Summary, limit)
	}
	intake.SourceRefs = compactStrings(sourceRefs)
	return civilizationAssemblyIssueScanProjectionEvidence{Intake: intake, Scan: scan, SourceRefs: compactStrings(sourceRefs), Truncated: truncated}
}

func civilizationAssemblyIssueRiskClasses(content IssueScanRunParkedContent) []string {
	classes := []string{civilizationAssemblyIssueRiskClass(content.BlockerType)}
	labels := issueScanLabelSet(content.TargetIssueLabels)
	if _, ok := labels[IssueScanProtectedActionLabel]; ok {
		classes = append(classes, "protected-action")
	}
	if _, ok := labels[IssueScanNeedsHumanScopeLabel]; ok {
		classes = append(classes, "human-scope")
	}
	if _, ok := labels[IssueScanPRDeferredLabel]; ok {
		classes = append(classes, "human-scope")
	}
	if strings.TrimSpace(content.BlockerType) == IssueScanParkBlockerNotPRReady {
		classes = append(classes, "not-pr-ready")
	}
	return compactStrings(classes)
}

func civilizationAssemblyIssueScanLifecycleVersion(content IssueScanRunParkedContent) string {
	if value := strings.TrimSpace(content.LifecycleVersion); value != "" {
		return value
	}
	if civilizationAssemblyIssueScanParkedEventIsCanary(content) {
		return IssueScanParkLifecycleLevel1Canary
	}
	if strings.TrimSpace(content.FactoryOrderID) != "" || strings.TrimSpace(content.StageID) != "" {
		return issueScanLifecycleVersion
	}
	return "issue_scan_parked_lifecycle_unavailable"
}

func civilizationAssemblyIssueScanAuthorityBoundary(content IssueScanRunParkedContent) string {
	if value := strings.TrimSpace(content.AuthorityBoundary); value != "" {
		return value
	}
	if civilizationAssemblyIssueScanParkedEventIsCanary(content) {
		return IssueScanParkAuthorityBoundaryLevel1Canary
	}
	return IssueScanParkAuthorityBoundaryLifecycle
}

func civilizationAssemblyIssueScanParkedEventIsCanary(content IssueScanRunParkedContent) bool {
	return strings.TrimSpace(content.EvidenceClass) == IssueScanParkEvidenceClassLevel1Canary
}

func civilizationAssemblyIssuePrimaryBlockerType(content IssueScanRunParkedContent) string {
	blockerType := strings.TrimSpace(content.BlockerType)
	labels := issueScanLabelSet(content.TargetIssueLabels)
	if blockerType == IssueScanParkBlockerProtectedAction {
		return IssueScanParkBlockerProtectedAction
	}
	if _, ok := labels[IssueScanProtectedActionLabel]; ok {
		return IssueScanParkBlockerProtectedAction
	}
	if blockerType == IssueScanParkBlockerHumanScope {
		return IssueScanParkBlockerHumanScope
	}
	if _, ok := labels[IssueScanNeedsHumanScopeLabel]; ok {
		return IssueScanParkBlockerHumanScope
	}
	if _, ok := labels[IssueScanPRDeferredLabel]; ok {
		return IssueScanParkBlockerHumanScope
	}
	return blockerType
}

func civilizationAssemblyIssueReadinessStates(content IssueScanRunParkedContent) []string {
	states := []string{civilizationAssemblyIssueReadiness(content.BlockerType)}
	labels := issueScanLabelSet(content.TargetIssueLabels)
	if _, ok := labels[IssueScanProtectedActionLabel]; ok {
		states = append(states, civilizationAssemblyIssueReadiness(IssueScanParkBlockerProtectedAction))
	}
	if _, ok := labels[IssueScanNeedsHumanScopeLabel]; ok {
		states = append(states, civilizationAssemblyIssueReadiness(IssueScanParkBlockerHumanScope))
	}
	if _, ok := labels[IssueScanPRDeferredLabel]; ok {
		states = append(states, "parked: PR deferred")
	}
	if strings.TrimSpace(content.BlockerType) == IssueScanParkBlockerNotPRReady {
		states = append(states, civilizationAssemblyIssueReadiness(IssueScanParkBlockerNotPRReady))
	}
	return compactStrings(states)
}

func civilizationAssemblyIssueScanBlockersFromParked(content IssueScanRunParkedContent, eventRef string, refs []string) []CivilizationIssueScanBlockerProjected {
	runID := strings.TrimSpace(content.RunID)
	factoryOrderID := strings.TrimSpace(content.FactoryOrderID)
	stageID := strings.TrimSpace(content.StageID)
	seen := map[string]bool{}
	blockers := make([]CivilizationIssueScanBlockerProjected, 0, 3)
	add := func(blockerType, reason, requiredAction string) {
		blockerType = strings.TrimSpace(blockerType)
		if blockerType == "" || seen[blockerType] {
			return
		}
		seen[blockerType] = true
		blockers = append(blockers, CivilizationIssueScanBlockerProjected{
			RunID:          runID,
			FactoryOrderID: factoryOrderID,
			StageID:        stageID,
			BlockerType:    blockerType,
			Reason:         strings.TrimSpace(reason),
			RequiredAction: strings.TrimSpace(requiredAction),
			EvidenceRefs:   []string{eventRef},
			SourceRefs:     refs,
		})
	}
	add(content.BlockerType, content.Detail, content.RequiredAction)

	labels := issueScanLabelSet(content.TargetIssueLabels)
	if _, ok := labels[IssueScanProtectedActionLabel]; ok {
		add(
			IssueScanParkBlockerProtectedAction,
			fmt.Sprintf("%s#%d is labeled %s", strings.TrimSpace(content.Repository), content.IssueNumber, IssueScanProtectedActionLabel),
			"human must authorize the protected-action boundary before Hive may continue",
		)
	}
	if _, ok := labels[IssueScanNeedsHumanScopeLabel]; ok {
		add(
			IssueScanParkBlockerHumanScope,
			fmt.Sprintf("%s#%d is labeled %s", strings.TrimSpace(content.Repository), content.IssueNumber, IssueScanNeedsHumanScopeLabel),
			"human must clarify scope and remove the human-scope blocker before Hive may continue",
		)
	}
	if _, ok := labels[IssueScanPRDeferredLabel]; ok {
		add(
			IssueScanParkBlockerHumanScope,
			fmt.Sprintf("%s#%d is labeled %s", strings.TrimSpace(content.Repository), content.IssueNumber, IssueScanPRDeferredLabel),
			"human must move the issue to PR-ready before Hive may continue",
		)
	}
	return blockers
}

func civilizationAssemblyIssueHasHumanActionBlocker(blockers []CivilizationIssueScanBlockerProjected) bool {
	for _, blocker := range blockers {
		switch blockerType := strings.TrimSpace(blocker.BlockerType); blockerType {
		case IssueScanParkBlockerHumanScope, IssueScanParkBlockerProtectedAction, IssueScanParkBlockerNotPRReady, IssueScanParkBlockerStaleTarget, IssueScanParkBlockerDuplicateChain:
			return true
		case "":
			continue
		default:
			return true
		}
	}
	return false
}

func civilizationAssemblyIssueRefFromParked(content IssueScanRunParkedContent) CivilizationIssueRef {
	labels := compactStrings(content.TargetIssueLabels)
	url := ""
	for _, ref := range content.SourceRefs {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, "https://github.com/") {
			url = ref
			break
		}
	}
	return CivilizationIssueRef{
		Repo:   strings.TrimSpace(content.Repository),
		Number: content.IssueNumber,
		URL:    url,
		State:  valueOr(strings.TrimSpace(content.TargetIssueState), "unknown"),
		Labels: labels,
	}
}

func civilizationAssemblyIssueRiskClass(blockerType string) string {
	switch strings.TrimSpace(blockerType) {
	case IssueScanParkBlockerProtectedAction:
		return "protected-action"
	case IssueScanParkBlockerHumanScope:
		return "human-scope"
	case IssueScanParkBlockerStaleTarget:
		return "stale-target"
	case IssueScanParkBlockerDuplicateChain:
		return "duplicate-chain"
	case IssueScanParkBlockerNotPRReady:
		return "not-pr-ready"
	default:
		return "unknown"
	}
}

func civilizationAssemblyIssueReadiness(blockerType string) string {
	switch strings.TrimSpace(blockerType) {
	case IssueScanParkBlockerProtectedAction:
		return "parked: protected-action authority required"
	case IssueScanParkBlockerHumanScope:
		return "parked: human scope required"
	case IssueScanParkBlockerStaleTarget:
		return "parked: stale target"
	case IssueScanParkBlockerDuplicateChain:
		return "parked: duplicate chain"
	case IssueScanParkBlockerNotPRReady:
		return "parked: not PR-ready"
	default:
		return "parked: unknown blocker"
	}
}

func civilizationAssemblyIssueTouchedSubstrate(repo string) string {
	repo = strings.TrimSpace(repo)
	_, name, ok := strings.Cut(repo, "/")
	if !ok || strings.TrimSpace(name) == "" {
		return "unknown"
	}
	return strings.TrimSpace(name)
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
	lifecycleUpdatedAt := map[string]time.Time{}
	for _, item := range p.Lifecycle {
		actorID := strings.TrimSpace(item.ActorID)
		if actorID == "" {
			continue
		}
		lifecycleUpdatedAt[actorID] = item.UpdatedAt
		actors[actorID] = CivilizationAssemblyActorSummary{
			ID:           "lifecycle:" + actorID,
			ActorID:      actorID,
			ActorType:    "agent",
			IdentityMode: valueOr(item.IdentityMode, "projected"),
			Status:       valueOr(item.LifecycleStatus, "projected"),
		}
	}
	activeActors := civilizationAssemblyActiveRuntimeActors(p.RuntimeEvidence.AgentEvents.ActiveAgents)
	for _, item := range civilizationAssemblyRuntimeAgents(p.RuntimeEvidence.AgentEvents) {
		actorID := civilizationAssemblyRuntimeActorID(item)
		if actorID == "" {
			continue
		}
		status := civilizationAssemblyRuntimeAgentStatus(p.RuntimeEvidence.Status, actorID, activeActors)
		if existing, ok := actors[actorID]; ok {
			if civilizationAssemblyRuntimeObservationSupersedesLifecycle(p.RuntimeEvidence, item, lifecycleUpdatedAt[actorID]) {
				existing.Status = status
			}
			actors[actorID] = existing
			continue
		}
		actors[actorID] = CivilizationAssemblyActorSummary{
			ID:           valueOr(item.SpawnedEventID, "runtime:"+actorID),
			ActorID:      actorID,
			ActorType:    "agent",
			IdentityMode: civilizationAssemblyRuntimeIdentityMode(status),
			Status:       status,
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
	runtimeAgents := civilizationAssemblyRuntimeAgents(p.RuntimeEvidence.AgentEvents)
	out := make([]CivilizationAssemblyRoleBinding, 0, len(p.Lifecycle)+len(runtimeAgents))
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
	for _, item := range runtimeAgents {
		actorID := civilizationAssemblyRuntimeActorID(item)
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
	runtimeAgents := civilizationAssemblyRuntimeAgents(p.RuntimeEvidence.AgentEvents)
	activeActors := civilizationAssemblyActiveRuntimeActors(p.RuntimeEvidence.AgentEvents.ActiveAgents)
	byActor := make(map[string]CivilizationAssemblyLifecycleSummary, len(p.Lifecycle)+len(runtimeAgents))
	lifecycleUpdatedAt := map[string]time.Time{}
	for _, item := range p.Lifecycle {
		actorID := strings.TrimSpace(item.ActorID)
		if actorID == "" {
			continue
		}
		lifecycleUpdatedAt[actorID] = item.UpdatedAt
		byActor[actorID] = CivilizationAssemblyLifecycleSummary{
			ID:      "lifecycle:" + item.ActorID,
			ActorID: actorID,
			ToState: item.LifecycleStatus,
			Status:  item.LifecycleStatus,
		}
	}
	for _, item := range runtimeAgents {
		actorID := civilizationAssemblyRuntimeActorID(item)
		if actorID == "" {
			continue
		}
		status := civilizationAssemblyRuntimeAgentStatus(p.RuntimeEvidence.Status, actorID, activeActors)
		existing := byActor[actorID]
		if existing.ID == "" {
			existing.ID = valueOr(item.SpawnedEventID, "runtime:"+actorID)
		}
		existing.ActorID = actorID
		if civilizationAssemblyRuntimeObservationSupersedesLifecycle(p.RuntimeEvidence, item, lifecycleUpdatedAt[actorID]) {
			existing.ToState = status
			existing.Status = status
		}
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
	artifactByTask, artifactTruncated, artifactErrors, err := civilizationAssemblyWorkTaskArtifactEvidence(s, limit)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("project Work task artifact refs: %v", err))
	}
	p.Errors = append(p.Errors, artifactErrors...)
	workEvidence.ArtifactSourceTruncated = artifactTruncated
	dependencyByTask, dependencyTruncated, dependencyErrors, err := civilizationAssemblyWorkTaskDependencyEvidence(s, limit)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("project Work task dependency source refs: %v", err))
	}
	p.Errors = append(p.Errors, dependencyErrors...)
	workEvidence.DependencySourceTruncated = dependencyTruncated
	lifecycleByTask, lifecycleTruncated, lifecycleErrors, err := civilizationAssemblyWorkTaskLifecycleEvidence(s, limit)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("project Work task lifecycle source refs: %v", err))
	}
	p.Errors = append(p.Errors, lifecycleErrors...)
	workEvidence.LifecycleSourceTruncated = lifecycleTruncated
	verificationByTask, verificationTruncated, verificationErrors, err := civilizationAssemblyWorkTaskVerificationEvidence(s, limit)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("project Work task verification evidence refs: %v", err))
	}
	p.Errors = append(p.Errors, verificationErrors...)
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
		if artifactEvidence, ok := artifactByTask[ev.ID()]; ok {
			workEvidence.ArtifactRefs = append(workEvidence.ArtifactRefs, artifactEvidence.ArtifactRefs...)
			workEvidence.Artifacts = append(workEvidence.Artifacts, artifactEvidence.Artifacts...)
			workEvidence.StageArtifactRefs = append(workEvidence.StageArtifactRefs, artifactEvidence.StageArtifactRefs...)
			workEvidence.SourceRefs = append(workEvidence.SourceRefs, artifactEvidence.SourceRefs...)
		}
		var dependencyEvidence civilizationAssemblyTaskDependencyEvidence
		if projectedDependencyEvidence, ok := dependencyByTask[ev.ID()]; ok {
			dependencyEvidence = projectedDependencyEvidence
			workEvidence.SourceRefs = append(workEvidence.SourceRefs, dependencyEvidence.SourceRefs...)
		}
		if lifecycleEvidence, ok := lifecycleByTask[ev.ID()]; ok {
			workEvidence.SourceRefs = append(workEvidence.SourceRefs, lifecycleEvidence.SourceRefs...)
		}
		if verificationEvidence, ok := verificationByTask[ev.ID()]; ok {
			workEvidence.TestRunRefs = append(workEvidence.TestRunRefs, verificationEvidence.TestRunRefs...)
			workEvidence.GateResultRefs = append(workEvidence.GateResultRefs, verificationEvidence.GateResultRefs...)
			workEvidence.SourceRefs = append(workEvidence.SourceRefs, verificationEvidence.SourceRefs...)
		}

		taskProjection, legacyProjection, err := civilizationAssemblyProjectWorkTask(taskStore, ev.ID())
		if err != nil {
			p.Errors = append(p.Errors, fmt.Sprintf("project Work task %s for civilization factory order %s: %v", ev.ID().Value(), orderID, err))
			continue
		}
		var roleContract civilizationAssemblyTaskRoleContract
		var outputContract civilizationAssemblyTaskOutputContract
		var runtimeEvidence civilizationAssemblyTaskRuntimeEvidence
		if artifactEvidence, ok := artifactByTask[ev.ID()]; ok {
			roleContract = artifactEvidence.RoleContract
			outputContract = artifactEvidence.OutputContract
			runtimeEvidence = artifactEvidence.RuntimeEvidence
		}
		taskEvidence := civilizationAssemblyTaskEvidence(ev.ID(), content, taskProjection, legacyProjection, dependencyEvidence.DependsOnRefs, dependencyEvidence.SourceRefs, roleContract, outputContract, runtimeEvidence)
		workEvidence.Tasks = append(workEvidence.Tasks, taskEvidence)
		order.Status = civilizationAssemblyFactoryOrderStatus(order.Status, civilizationAssemblyProjectedWorkTaskStatus(taskProjection, legacyProjection))
	}
	workEvidence.TaskRefs = compactStrings(workEvidence.TaskRefs)
	workEvidence.Tasks = compactCivilizationAssemblyTaskEvidence(workEvidence.Tasks)
	workEvidence.ArtifactRefs = compactStrings(workEvidence.ArtifactRefs)
	workEvidence.Artifacts = compactCivilizationAssemblyArtifactEvidence(workEvidence.Artifacts)
	workEvidence.StageArtifactRefs = compactCivilizationAssemblyStageArtifactRefs(workEvidence.StageArtifactRefs)
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

func civilizationAssemblyTaskEvidence(taskID types.EventID, content work.TaskCreatedContent, taskProjection work.TaskProjection, legacyProjection work.LegacyTaskProjection, dependencies []string, sourceRefs []string, roleContract civilizationAssemblyTaskRoleContract, outputContract civilizationAssemblyTaskOutputContract, runtimeEvidence civilizationAssemblyTaskRuntimeEvidence) CivilizationAssemblyTaskEvidence {
	lifecycleStageID := civilizationAssemblyTaskLifecycleStageID(content)
	if roleContract.StageID != "" && (safeRunLaunchID(roleContract.StageID) != safeRunLaunchID(lifecycleStageID) || roleContract.FactoryOrderID != strings.TrimSpace(content.FactoryOrderID)) {
		roleContract = civilizationAssemblyTaskRoleContract{}
	}
	if outputContract.StageID != "" && (safeRunLaunchID(outputContract.StageID) != safeRunLaunchID(lifecycleStageID) || outputContract.FactoryOrderID != strings.TrimSpace(content.FactoryOrderID)) {
		outputContract = civilizationAssemblyTaskOutputContract{}
	}
	if runtimeEvidence.StageID != "" && (safeRunLaunchID(runtimeEvidence.StageID) != safeRunLaunchID(lifecycleStageID) || runtimeEvidence.FactoryOrderID != strings.TrimSpace(content.FactoryOrderID)) {
		runtimeEvidence = civilizationAssemblyTaskRuntimeEvidence{}
	}
	status := civilizationAssemblyProjectedWorkTaskStatus(taskProjection, legacyProjection)
	ready, blocked := civilizationAssemblyProjectedWorkTaskReadiness(status, taskProjection, legacyProjection)
	expectedOutputs := compactStrings(content.ExpectedOutputs)
	if len(outputContract.ExpectedOutputs) > 0 {
		expectedOutputs = compactStrings(append(expectedOutputs, outputContract.ExpectedOutputs...))
	}
	runtimeStatus := strings.TrimSpace(runtimeEvidence.Status)
	runtimeEvidenceCoversContracts := civilizationAssemblyRuntimeEvidenceCoversContracts(runtimeEvidence, outputContract, roleContract)
	if runtimeStatus != "" && status == "work_task_completed" && runtimeEvidenceCoversContracts {
		runtimeStatus = civilizationAssemblyQueuedStageCompletedStatus
	}
	roleOutputContracts := civilizationAssemblyRoleOutputContractsWithRuntimeEvidence(outputContract.RoleOutputContracts, runtimeEvidence, runtimeStatus)
	return CivilizationAssemblyTaskEvidence{
		ID:                      taskID.Value(),
		CanonicalTaskID:         strings.TrimSpace(content.CanonicalTaskID),
		FactoryOrderID:          strings.TrimSpace(content.FactoryOrderID),
		LifecycleStageID:        lifecycleStageID,
		Title:                   strings.TrimSpace(content.Title),
		Cell:                    strings.TrimSpace(content.Cell),
		RiskClass:               strings.TrimSpace(content.RiskClass),
		Status:                  status,
		Ready:                   ready,
		Blocked:                 blocked,
		RequirementRefs:         compactStrings(content.RequirementIDs),
		AcceptanceCriterionRefs: compactStrings(content.AcceptanceCriterionIDs),
		ExpectedOutputs:         expectedOutputs,
		DependsOnRefs:           compactStrings(dependencies),
		SourceRefs:              compactStrings(append(append(append(append([]string{taskID.Value()}, sourceRefs...), roleContract.SourceRefs...), outputContract.SourceRefs...), runtimeEvidence.SourceRefs...)),
		RequiredRoles:           compactStrings(roleContract.RequiredRoles),
		RoleContractRefs:        compactStrings(roleContract.SourceRefs),
		AgentExecutionPlan:      compactOperatorQueuedRunAgentPlanSteps(roleContract.AgentExecutionPlan),
		RequiredEvidence:        compactStrings(outputContract.RequiredEvidence),
		OutputContractRefs:      compactStrings(outputContract.SourceRefs),
		RoleOutputContracts:     roleOutputContracts,
		RuntimeEvidenceRefs:     compactStrings(runtimeEvidence.SourceRefs),
		RuntimeEvidenceStatus:   runtimeStatus,
	}
}

func civilizationAssemblyRoleOutputContractsWithRuntimeEvidence(contracts []CivilizationAssemblyRoleOutputContract, evidence civilizationAssemblyTaskRuntimeEvidence, runtimeStatus string) []CivilizationAssemblyRoleOutputContract {
	out := compactCivilizationAssemblyRoleOutputContracts(contracts)
	if evidence.StageID == "" || (len(evidence.Roles) == 0 && len(evidence.RoleOutputRoles) == 0) {
		return out
	}
	evidenceStatus := civilizationAssemblyQueuedPlanStepRuntimeStatus
	if strings.TrimSpace(runtimeStatus) == civilizationAssemblyQueuedStageCompletedStatus {
		evidenceStatus = civilizationAssemblyQueuedPlanStepCompletedStatus
	}
	covered := map[string]bool{}
	for _, role := range evidence.Roles {
		covered[strings.TrimSpace(role)] = true
	}
	roleOutputCovered := map[string]bool{}
	for _, role := range evidence.RoleOutputRoles {
		role = strings.TrimSpace(role)
		covered[role] = true
		roleOutputCovered[role] = true
	}
	for i := range out {
		if covered[out[i].Role] {
			out[i].EvidenceStatus = evidenceStatus
		}
		if roleOutputCovered[out[i].Role] && strings.TrimSpace(runtimeStatus) != civilizationAssemblyQueuedStageCompletedStatus {
			out[i].EvidenceStatus = civilizationAssemblyQueuedPlanStepRuntimeStatus
		}
	}
	return compactCivilizationAssemblyRoleOutputContracts(out)
}

func civilizationAssemblyRuntimeEvidenceCoversContracts(runtimeEvidence civilizationAssemblyTaskRuntimeEvidence, outputContract civilizationAssemblyTaskOutputContract, roleContract civilizationAssemblyTaskRoleContract) bool {
	if runtimeEvidence.StageID == "" {
		return false
	}
	requiredEvidence := compactStrings(outputContract.RequiredEvidence)
	if len(requiredEvidence) > 0 && !civilizationAssemblyStringSetContainsAll(runtimeEvidence.RequiredEvidence, requiredEvidence) {
		return false
	}
	requiredRoles := append([]string(nil), roleContract.RequiredRoles...)
	for _, step := range roleContract.AgentExecutionPlan {
		requiredRoles = append(requiredRoles, step.Role)
	}
	for _, contract := range outputContract.RoleOutputContracts {
		requiredRoles = append(requiredRoles, contract.Role)
	}
	requiredRoles = compactStrings(requiredRoles)
	if len(requiredRoles) > 0 && !civilizationAssemblyStringSetContainsAll(runtimeEvidence.Roles, requiredRoles) {
		return false
	}
	return true
}

func civilizationAssemblyStringSetContainsAll(have, required []string) bool {
	byValue := map[string]bool{}
	for _, value := range have {
		value = strings.TrimSpace(value)
		if value != "" {
			byValue[value] = true
		}
	}
	for _, value := range required {
		value = strings.TrimSpace(value)
		if value != "" && !byValue[value] {
			return false
		}
	}
	return true
}

func civilizationAssemblyTaskLifecycleStageID(content work.TaskCreatedContent) string {
	canonicalTaskID := strings.TrimSpace(content.CanonicalTaskID)
	factoryOrderID := strings.TrimSpace(content.FactoryOrderID)
	factoryOrderSuffix := factoryOrderIDSuffix(factoryOrderID)
	if canonicalTaskID == "" || factoryOrderSuffix == "" {
		return ""
	}
	prefix := "tsk_" + factoryOrderSuffix + "_"
	if !strings.HasPrefix(canonicalTaskID, prefix) {
		return ""
	}
	stageID := strings.TrimSpace(strings.TrimPrefix(canonicalTaskID, prefix))
	if stageID == "" || safeRunLaunchID(stageID) != stageID {
		return ""
	}
	if !civilizationAssemblyIssueScanStageIDAllowed(stageID) {
		return ""
	}
	return stageID
}

func civilizationAssemblyIssueScanStageIDAllowed(stageID string) bool {
	stageID = safeRunLaunchID(stageID)
	if stageID == "" {
		return false
	}
	for _, stage := range issueScanDevelopmentLifecycle() {
		if safeRunLaunchID(stage.ID) == stageID {
			return true
		}
	}
	return false
}

func civilizationAssemblyWorkTaskArtifactEvidence(s store.Store, limit int) (map[types.EventID]civilizationAssemblyTaskArtifactEvidence, bool, []string, error) {
	page, err := s.ByType(work.EventTypeTaskArtifact, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, nil, fmt.Errorf("fetch %s events: %w", work.EventTypeTaskArtifact.Value(), err)
	}
	byTask := map[types.EventID]civilizationAssemblyTaskArtifactEvidence{}
	projectionErrors := []string{}
	taskCreatedContentByID := map[types.EventID]work.TaskCreatedContent{}
	taskCreatedPage, err := s.ByType(work.EventTypeTaskCreated, limit, types.None[types.Cursor]())
	if err != nil {
		projectionErrors = append(projectionErrors, fmt.Sprintf("fetch %s for task artifact identity fallback: %v", work.EventTypeTaskCreated.Value(), err))
	} else {
		for _, taskEvent := range taskCreatedPage.Items() {
			taskContent, ok := taskEvent.Content().(work.TaskCreatedContent)
			if !ok {
				continue
			}
			taskCreatedContentByID[taskEvent.ID()] = taskContent
		}
	}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskArtifactContent)
		if !ok {
			projectionErrors = append(projectionErrors, contentTypeError(ev, "work.TaskArtifactContent"))
			continue
		}
		evidence := byTask[content.TaskID]
		label := strings.TrimSpace(content.Label)
		evidence.ArtifactRefs = append(evidence.ArtifactRefs, ev.ID().Value())
		evidence.Artifacts = append(evidence.Artifacts, CivilizationAssemblyArtifactEvidence{
			ID:         ev.ID().Value(),
			TaskRef:    content.TaskID.Value(),
			Label:      label,
			MediaType:  strings.TrimSpace(content.MediaType),
			SourceRefs: []string{ev.ID().Value()},
		})
		stageRef, ok, err := civilizationAssemblyIssueScanStageArtifactRef(ev.ID().Value(), label, content.Body)
		if err != nil {
			projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan lifecycle stage artifact %s: %v", ev.ID().Value(), err))
		} else if ok {
			evidence.StageArtifactRefs = append(evidence.StageArtifactRefs, stageRef)
		}
		roleContract, ok, err := civilizationAssemblyIssueScanStageRoleContract(ev.ID().Value(), label, content.Body)
		if err != nil {
			projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage role contract %s: %v", ev.ID().Value(), err))
		} else if ok && !evidence.RoleContractConflicted {
			merged, diverged := mergeCivilizationAssemblyTaskRoleContract(evidence.RoleContract, roleContract)
			if diverged {
				projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage role contract %s: divergent role contract for task %s (existing stage=%q order=%q, candidate stage=%q order=%q)", ev.ID().Value(), content.TaskID.Value(), evidence.RoleContract.StageID, evidence.RoleContract.FactoryOrderID, roleContract.StageID, roleContract.FactoryOrderID))
				evidence.RoleContract = civilizationAssemblyTaskRoleContract{}
				evidence.RoleContractConflicted = true
			} else {
				evidence.RoleContract = merged
			}
		}
		outputContract, ok, err := civilizationAssemblyIssueScanStageOutputContract(ev.ID().Value(), label, content.Body)
		if err != nil {
			projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage output contract %s: %v", ev.ID().Value(), err))
		} else if ok && !evidence.OutputContractConflicted {
			merged, diverged := mergeCivilizationAssemblyTaskOutputContract(evidence.OutputContract, outputContract)
			if diverged {
				projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage output contract %s: divergent output contract for task %s (existing stage=%q order=%q, candidate stage=%q order=%q)", ev.ID().Value(), content.TaskID.Value(), evidence.OutputContract.StageID, evidence.OutputContract.FactoryOrderID, outputContract.StageID, outputContract.FactoryOrderID))
				evidence.OutputContract = civilizationAssemblyTaskOutputContract{}
				evidence.OutputContractConflicted = true
			} else {
				evidence.OutputContract = merged
			}
		}
		runtimeEvidence, ok, err := civilizationAssemblyIssueScanStageRuntimeEvidence(ev.ID().Value(), label, content.Body)
		if err != nil {
			projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage runtime evidence %s: %v", ev.ID().Value(), err))
		} else if ok {
			merged, diverged := mergeCivilizationAssemblyTaskRuntimeEvidence(evidence.RuntimeEvidence, runtimeEvidence)
			if diverged {
				projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage runtime evidence %s: divergent runtime evidence for task %s (existing stage=%q order=%q, candidate stage=%q order=%q)", ev.ID().Value(), content.TaskID.Value(), evidence.RuntimeEvidence.StageID, evidence.RuntimeEvidence.FactoryOrderID, runtimeEvidence.StageID, runtimeEvidence.FactoryOrderID))
				evidence.RuntimeEvidence = civilizationAssemblyTaskRuntimeEvidence{}
			} else {
				evidence.RuntimeEvidence = merged
			}
		}
		roleOutputEvidence, ok, err := civilizationAssemblyIssueScanStageRoleOutput(ev.ID().Value(), label, content.Body, taskCreatedContentByID[content.TaskID])
		if err != nil {
			projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage role output %s: %v", ev.ID().Value(), err))
		} else if ok {
			merged, diverged := mergeCivilizationAssemblyTaskRuntimeEvidence(evidence.RuntimeEvidence, roleOutputEvidence)
			if diverged {
				projectionErrors = append(projectionErrors, fmt.Sprintf("project issue-scan stage role output %s: divergent role output for task %s (existing stage=%q order=%q, candidate stage=%q order=%q)", ev.ID().Value(), content.TaskID.Value(), evidence.RuntimeEvidence.StageID, evidence.RuntimeEvidence.FactoryOrderID, roleOutputEvidence.StageID, roleOutputEvidence.FactoryOrderID))
				evidence.RuntimeEvidence = civilizationAssemblyTaskRuntimeEvidence{}
			} else {
				evidence.RuntimeEvidence = merged
			}
		}
		evidence.SourceRefs = append(evidence.SourceRefs, ev.ID().Value())
		byTask[content.TaskID] = evidence
	}
	for taskID, evidence := range byTask {
		evidence.ArtifactRefs = compactStrings(evidence.ArtifactRefs)
		evidence.Artifacts = compactCivilizationAssemblyArtifactEvidence(evidence.Artifacts)
		evidence.StageArtifactRefs = compactCivilizationAssemblyStageArtifactRefs(evidence.StageArtifactRefs)
		evidence.RoleContract = compactCivilizationAssemblyTaskRoleContract(evidence.RoleContract)
		evidence.OutputContract = compactCivilizationAssemblyTaskOutputContract(evidence.OutputContract)
		evidence.RuntimeEvidence = compactCivilizationAssemblyTaskRuntimeEvidence(evidence.RuntimeEvidence)
		evidence.SourceRefs = compactStrings(evidence.SourceRefs)
		byTask[taskID] = evidence
	}
	return byTask, page.HasMore(), projectionErrors, nil
}

func civilizationAssemblyIssueScanStageArtifactRef(eventRef, label, body string) (civilizationAssemblyStageArtifactRef, bool, error) {
	stageID, ok := civilizationAssemblyIssueScanStageArtifactKey(label)
	if !ok {
		return civilizationAssemblyStageArtifactRef{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return civilizationAssemblyStageArtifactRef{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload struct {
		Kind  string `json:"kind"`
		RunID string `json:"run_id"`
		Stage struct {
			ID string `json:"id"`
		} `json:"stage"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return civilizationAssemblyStageArtifactRef{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != civilizationAssemblyIssueScanStageArtifactKind {
		return civilizationAssemblyStageArtifactRef{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, civilizationAssemblyIssueScanStageArtifactKind)
	}
	runID := safeRunLaunchID(payload.RunID)
	if runID == "" {
		return civilizationAssemblyStageArtifactRef{}, false, fmt.Errorf("missing run_id")
	}
	bodyStageID := safeRunLaunchID(payload.Stage.ID)
	if bodyStageID == "" {
		return civilizationAssemblyStageArtifactRef{}, false, fmt.Errorf("missing stage.id")
	}
	if bodyStageID != stageID {
		return civilizationAssemblyStageArtifactRef{}, false, fmt.Errorf("label stage %q does not match body stage %q", stageID, bodyStageID)
	}
	return civilizationAssemblyStageArtifactRef{
		RunID:    runID,
		StageID:  stageID,
		EventRef: strings.TrimSpace(eventRef),
	}, true, nil
}

func civilizationAssemblyIssueScanStageArtifactKey(label string) (string, bool) {
	suffix, ok := strings.CutPrefix(strings.TrimSpace(label), IssueScanLifecycleStageArtifactPrefix)
	if !ok {
		return "", false
	}
	stageID := safeRunLaunchID(suffix)
	return stageID, stageID != ""
}

func civilizationAssemblyIssueScanStageRoleContract(eventRef, label, body string) (civilizationAssemblyTaskRoleContract, bool, error) {
	if strings.TrimSpace(label) != IssueScanStageRoleContractArtifactLabel {
		return civilizationAssemblyTaskRoleContract{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload struct {
		Kind               string                           `json:"kind"`
		FactoryOrderID     string                           `json:"factory_order_id"`
		StageID            string                           `json:"stage_id"`
		Stage              OperatorQueuedRunLifecycleStage  `json:"stage"`
		AgentExecutionPlan []OperatorQueuedRunAgentPlanStep `json:"agent_execution_plan"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageRoleContractArtifactKind {
		return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageRoleContractArtifactKind)
	}
	stageID := strings.TrimSpace(payload.StageID)
	if stageID == "" {
		stageID = strings.TrimSpace(payload.Stage.ID)
	}
	if safeRunLaunchID(stageID) == "" {
		return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("missing stage_id")
	}
	if payload.Stage.ID != "" && safeRunLaunchID(payload.Stage.ID) != safeRunLaunchID(stageID) {
		return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("stage_id %q does not match stage.id %q", stageID, payload.Stage.ID)
	}
	factoryOrderID := strings.TrimSpace(payload.FactoryOrderID)
	if factoryOrderID == "" {
		return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("missing factory_order_id")
	}
	for i, step := range payload.AgentExecutionPlan {
		if safeRunLaunchID(step.StageID) != safeRunLaunchID(stageID) {
			return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("agent_execution_plan[%d].stage_id %q does not match stage %q", i, step.StageID, stageID)
		}
		if strings.TrimSpace(step.Role) == "" {
			return civilizationAssemblyTaskRoleContract{}, false, fmt.Errorf("agent_execution_plan[%d].role is required", i)
		}
	}
	return civilizationAssemblyTaskRoleContract{
		StageID:            stageID,
		FactoryOrderID:     factoryOrderID,
		RequiredRoles:      compactStrings(payload.Stage.RequiredRoles),
		AgentExecutionPlan: compactOperatorQueuedRunAgentPlanSteps(payload.AgentExecutionPlan),
		SourceRefs:         compactStrings([]string{eventRef}),
	}, true, nil
}

func civilizationAssemblyIssueScanStageOutputContract(eventRef, label, body string) (civilizationAssemblyTaskOutputContract, bool, error) {
	if strings.TrimSpace(label) != IssueScanStageOutputContractArtifactLabel {
		return civilizationAssemblyTaskOutputContract{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload struct {
		Kind                string                                   `json:"kind"`
		FactoryOrderID      string                                   `json:"factory_order_id"`
		StageID             string                                   `json:"stage_id"`
		Stage               OperatorQueuedRunLifecycleStage          `json:"stage"`
		RequiredEvidence    []string                                 `json:"required_evidence"`
		ExpectedOutputs     []string                                 `json:"expected_outputs"`
		RoleOutputContracts []CivilizationAssemblyRoleOutputContract `json:"role_output_contracts"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageOutputContractArtifactKind {
		return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageOutputContractArtifactKind)
	}
	stageID := strings.TrimSpace(payload.StageID)
	if stageID == "" {
		stageID = strings.TrimSpace(payload.Stage.ID)
	}
	if safeRunLaunchID(stageID) == "" {
		return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("missing stage_id")
	}
	if payload.Stage.ID != "" && safeRunLaunchID(payload.Stage.ID) != safeRunLaunchID(stageID) {
		return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("stage_id %q does not match stage.id %q", stageID, payload.Stage.ID)
	}
	factoryOrderID := strings.TrimSpace(payload.FactoryOrderID)
	if factoryOrderID == "" {
		return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("missing factory_order_id")
	}
	contractByRole := map[string]CivilizationAssemblyRoleOutputContract{}
	for i, contract := range payload.RoleOutputContracts {
		role := strings.TrimSpace(contract.Role)
		if role == "" {
			return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("role_output_contracts[%d].role is required", i)
		}
		contractByRole[role] = contract
	}
	for _, role := range payload.Stage.RequiredRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := contractByRole[role]; !ok {
			return civilizationAssemblyTaskOutputContract{}, false, fmt.Errorf("missing role_output_contract for required role %q", role)
		}
	}
	requiredEvidence := compactStrings(append(payload.RequiredEvidence, payload.Stage.RequiredEvidence...))
	return civilizationAssemblyTaskOutputContract{
		StageID:             stageID,
		FactoryOrderID:      factoryOrderID,
		RequiredEvidence:    requiredEvidence,
		ExpectedOutputs:     compactStrings(payload.ExpectedOutputs),
		RoleOutputContracts: compactCivilizationAssemblyRoleOutputContracts(payload.RoleOutputContracts),
		SourceRefs:          compactStrings([]string{eventRef}),
	}, true, nil
}

func civilizationAssemblyIssueScanStageRuntimeEvidence(eventRef, label, body string) (civilizationAssemblyTaskRuntimeEvidence, bool, error) {
	if strings.TrimSpace(label) != IssueScanStageRuntimeEvidenceArtifactLabel {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, nil
	}
	raw := strings.TrimSpace(body)
	if raw == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("label %q has empty artifact body", label)
	}
	var payload IssueScanStageRuntimeEvidence
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("decode artifact body: %w", err)
	}
	if strings.TrimSpace(payload.Kind) != issueScanStageRuntimeEvidenceArtifactKind {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("kind %q does not match %q", payload.Kind, issueScanStageRuntimeEvidenceArtifactKind)
	}
	stageID := safeRunLaunchID(payload.StageID)
	if stageID == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("missing stage_id")
	}
	factoryOrderID := strings.TrimSpace(payload.FactoryOrderID)
	if factoryOrderID == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("missing factory_order_id")
	}
	// Projection never trusts the artifact's self-declared status; completion
	// is derived from the Work task status in civilizationAssemblyTaskEvidence.
	status := civilizationAssemblyQueuedStageRuntimeStatus
	requiredEvidence := make([]string, 0, len(payload.EvidenceItems))
	for i, item := range payload.EvidenceItems {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("evidence_items[%d].key is required", i)
		}
		requiredEvidence = append(requiredEvidence, key)
	}
	roles := make([]string, 0, len(payload.RoleOutputs))
	for i, roleOutput := range payload.RoleOutputs {
		role := strings.TrimSpace(roleOutput.Role)
		if role == "" {
			return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("role_outputs[%d].role is required", i)
		}
		roles = append(roles, role)
	}
	return civilizationAssemblyTaskRuntimeEvidence{
		StageID:          stageID,
		FactoryOrderID:   factoryOrderID,
		Status:           status,
		RequiredEvidence: compactStrings(requiredEvidence),
		Roles:            compactStrings(roles),
		SourceRefs:       compactStrings(append([]string{strings.TrimSpace(eventRef)}, payload.SourceRefs...)),
	}, true, nil
}

func civilizationAssemblyIssueScanStageRoleOutput(eventRef, label, body string, taskContent work.TaskCreatedContent) (civilizationAssemblyTaskRuntimeEvidence, bool, error) {
	payload, ok, err := issueScanStageRoleOutputArtifact(eventRef, label, body)
	if err != nil || !ok {
		return civilizationAssemblyTaskRuntimeEvidence{}, ok, err
	}
	stageID := safeRunLaunchID(payload.StageID)
	if stageID == "" {
		stageID = safeRunLaunchID(civilizationAssemblyTaskLifecycleStageID(taskContent))
	}
	if stageID == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("missing stage_id")
	}
	factoryOrderID := strings.TrimSpace(payload.FactoryOrderID)
	if factoryOrderID == "" {
		factoryOrderID = strings.TrimSpace(taskContent.FactoryOrderID)
	}
	if factoryOrderID == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("missing factory_order_id")
	}
	role := strings.TrimSpace(payload.Role)
	if role == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("missing role")
	}
	for i, output := range payload.Outputs {
		if strings.TrimSpace(output.Key) == "" {
			return civilizationAssemblyTaskRuntimeEvidence{}, false, fmt.Errorf("outputs[%d].key is required", i)
		}
	}
	return civilizationAssemblyTaskRuntimeEvidence{
		StageID:         stageID,
		FactoryOrderID:  factoryOrderID,
		RoleOutputRoles: compactStrings([]string{role}),
		RoleOutputRefs:  compactStrings(payload.SourceRefs),
		SourceRefs:      compactStrings(payload.SourceRefs),
	}, true, nil
}

func civilizationAssemblyWorkTaskDependencyEvidence(s store.Store, limit int) (map[types.EventID]civilizationAssemblyTaskDependencyEvidence, bool, []string, error) {
	page, err := s.ByType(work.EventTypeTaskDependencyAdded, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, nil, fmt.Errorf("fetch %s events: %w", work.EventTypeTaskDependencyAdded.Value(), err)
	}
	byTask := map[types.EventID]civilizationAssemblyTaskDependencyEvidence{}
	projectionErrors := []string{}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskDependencyContent)
		if !ok {
			projectionErrors = append(projectionErrors, contentTypeError(ev, "work.TaskDependencyContent"))
			continue
		}
		evidence := byTask[content.TaskID]
		evidence.DependsOnRefs = append(evidence.DependsOnRefs, content.DependsOnID.Value())
		evidence.SourceRefs = append(evidence.SourceRefs, ev.ID().Value())
		byTask[content.TaskID] = evidence
	}
	for taskID, evidence := range byTask {
		evidence.DependsOnRefs = compactStrings(evidence.DependsOnRefs)
		evidence.SourceRefs = compactStrings(evidence.SourceRefs)
		byTask[taskID] = evidence
	}
	return byTask, page.HasMore(), projectionErrors, nil
}

func civilizationAssemblyWorkTaskLifecycleEvidence(s store.Store, limit int) (map[types.EventID]civilizationAssemblyTaskLifecycleEvidence, bool, []string, error) {
	page, err := s.ByType(work.EventTypeTaskLifecycleTransitioned, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, nil, fmt.Errorf("fetch %s events: %w", work.EventTypeTaskLifecycleTransitioned.Value(), err)
	}
	byTask := map[types.EventID]civilizationAssemblyTaskLifecycleEvidence{}
	projectionErrors := []string{}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskLifecycleTransitionContent)
		if !ok {
			projectionErrors = append(projectionErrors, contentTypeError(ev, "work.TaskLifecycleTransitionContent"))
			continue
		}
		evidence := byTask[content.TaskID]
		evidence.SourceRefs = append(evidence.SourceRefs, ev.ID().Value())
		byTask[content.TaskID] = evidence
	}
	for taskID, evidence := range byTask {
		evidence.SourceRefs = compactStrings(evidence.SourceRefs)
		byTask[taskID] = evidence
	}
	return byTask, page.HasMore(), projectionErrors, nil
}

func civilizationAssemblyWorkTaskVerificationEvidence(s store.Store, limit int) (map[types.EventID]civilizationAssemblyTaskVerificationEvidence, bool, []string, error) {
	page, err := s.ByType(work.EventTypeTaskVerificationAttached, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, false, nil, fmt.Errorf("fetch %s events: %w", work.EventTypeTaskVerificationAttached.Value(), err)
	}
	byTask := map[types.EventID]civilizationAssemblyTaskVerificationEvidence{}
	projectionErrors := []string{}
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskVerificationAttachedContent)
		if !ok {
			projectionErrors = append(projectionErrors, contentTypeError(ev, "work.TaskVerificationAttachedContent"))
			continue
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
	return byTask, page.HasMore(), projectionErrors, nil
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

func civilizationAssemblyProjectedWorkTaskReadiness(status string, taskProjection work.TaskProjection, legacyProjection work.LegacyTaskProjection) (bool, bool) {
	if taskProjection.Status != "" && taskProjection.Status != work.StatusCreated {
		return civilizationAssemblyNormalizeTaskReadiness(status, taskProjection.Ready, taskProjection.Blocked)
	}
	switch legacyProjection.Status {
	case work.LegacyStatusBlocked:
		return civilizationAssemblyNormalizeTaskReadiness(status, false, true)
	case work.LegacyStatusReady:
		return civilizationAssemblyNormalizeTaskReadiness(status, true, false)
	case work.LegacyStatusAssigned, work.LegacyStatusCompleted:
		return civilizationAssemblyNormalizeTaskReadiness(status, false, false)
	}
	return civilizationAssemblyNormalizeTaskReadiness(status, taskProjection.Ready || legacyProjection.Ready, taskProjection.Blocked || legacyProjection.Blocked)
}

func civilizationAssemblyNormalizeTaskReadiness(status string, ready, blocked bool) (bool, bool) {
	switch strings.TrimSpace(status) {
	case "work_task_blocked", "work_task_policy_blocked":
		return false, true
	case "work_task_ready":
		return true, false
	}
	if blocked {
		return false, true
	}
	return ready, false
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
	case "work_task_seeded", "work_task_pending", "work_task_created", "work_task_superseded":
		return 10
	default:
		return 20
	}
}

func civilizationAssemblyRuntimeAgents(agentEvents OperatorRuntimeAgentEvents) []OperatorRuntimeAgentEvidence {
	agentsByActor := map[string]OperatorRuntimeAgentEvidence{}
	add := func(agent OperatorRuntimeAgentEvidence) {
		actorID := civilizationAssemblyRuntimeActorID(agent)
		if actorID == "" {
			return
		}
		agentsByActor[actorID] = agent
	}
	for _, agent := range agentEvents.ObservedAgents {
		add(agent)
	}
	for _, agent := range agentEvents.ActiveAgents {
		add(agent)
	}
	out := make([]OperatorRuntimeAgentEvidence, 0, len(agentsByActor))
	for _, agent := range agentsByActor {
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool {
		left := civilizationAssemblyRuntimeActorID(out[i])
		right := civilizationAssemblyRuntimeActorID(out[j])
		if left == right {
			return out[i].SpawnedEventID < out[j].SpawnedEventID
		}
		return left < right
	})
	return out
}

func civilizationAssemblyActiveRuntimeActors(activeAgents []OperatorRuntimeAgentEvidence) map[string]bool {
	active := map[string]bool{}
	for _, agent := range activeAgents {
		actorID := civilizationAssemblyRuntimeActorID(agent)
		if actorID != "" {
			active[actorID] = true
		}
	}
	return active
}

func civilizationAssemblyRuntimeActorID(agent OperatorRuntimeAgentEvidence) string {
	actorID := strings.TrimSpace(agent.ActorID)
	if actorID == "" {
		actorID = strings.TrimSpace(agent.Name)
	}
	return actorID
}

func civilizationAssemblyRuntimeAgentStatus(runtimeStatus, actorID string, activeActors map[string]bool) string {
	if activeActors[actorID] {
		return "active"
	}
	if runtimeStatus == "completed" {
		return "observed_completed_run"
	}
	return "observed"
}

func civilizationAssemblyRuntimeIdentityMode(status string) string {
	if status == "active" {
		return "runtime"
	}
	return "runtime_observed"
}

func civilizationAssemblyRuntimeObservationSupersedesLifecycle(runtimeEvidence OperatorRuntimeEvidence, agent OperatorRuntimeAgentEvidence, lifecycleUpdatedAt time.Time) bool {
	runtimeObservedAt := civilizationAssemblyRuntimeObservedAt(runtimeEvidence, agent)
	if lifecycleUpdatedAt.IsZero() || runtimeObservedAt.IsZero() {
		return true
	}
	return !runtimeObservedAt.Before(lifecycleUpdatedAt)
}

func civilizationAssemblyRuntimeObservedAt(runtimeEvidence OperatorRuntimeEvidence, agent OperatorRuntimeAgentEvidence) time.Time {
	observedAt := agent.SpawnedAt
	if runtimeEvidence.LastRun != nil && runtimeEvidence.LastRun.CompletedAt != nil && runtimeEvidence.LastRun.CompletedAt.After(observedAt) {
		observedAt = *runtimeEvidence.LastRun.CompletedAt
	}
	return observedAt
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
		Tasks:          compactCivilizationAssemblyTaskEvidence(factoryOrderWorkEvidence.Tasks),
		ArtifactRefs:   compactStrings(append(artifactRefs, factoryOrderWorkEvidence.ArtifactRefs...)),
		Artifacts:      compactCivilizationAssemblyArtifactEvidence(factoryOrderWorkEvidence.Artifacts),
		TestRunRefs:    compactStrings(factoryOrderWorkEvidence.TestRunRefs),
		GateResultRefs: compactStrings(factoryOrderWorkEvidence.GateResultRefs),
		SourceRefs:     sourceRefs,
	}
}

func civilizationAssemblyQueuedRunRequestWithStageEvidence(queued *OperatorQueuedRunRequestEvidence, stageArtifactRefs []civilizationAssemblyStageArtifactRef, tasks []CivilizationAssemblyTaskEvidence) *CivilizationAssemblyQueuedRunRequest {
	if queued == nil {
		return nil
	}
	out := *queued
	out.TargetRepos = append([]string(nil), queued.TargetRepos...)
	if queued.RoleSeparationPolicy != nil {
		policy := *queued.RoleSeparationPolicy
		policy.GlobalNonClaims = append([]string(nil), queued.RoleSeparationPolicy.GlobalNonClaims...)
		policy.ActorPolicies = append([]OperatorQueuedRunRoleSeparationActorPolicy(nil), queued.RoleSeparationPolicy.ActorPolicies...)
		for i := range policy.ActorPolicies {
			policy.ActorPolicies[i].MapsToStageRoles = append([]string(nil), policy.ActorPolicies[i].MapsToStageRoles...)
			policy.ActorPolicies[i].RequiredEvidence = append([]string(nil), policy.ActorPolicies[i].RequiredEvidence...)
			policy.ActorPolicies[i].ForbiddenWithoutHuman = append([]string(nil), policy.ActorPolicies[i].ForbiddenWithoutHuman...)
		}
		out.RoleSeparationPolicy = &policy
	}
	if queued.AutonomyGuardPolicy != nil {
		policy := *queued.AutonomyGuardPolicy
		policy.EvidenceInputs = append([]string(nil), queued.AutonomyGuardPolicy.EvidenceInputs...)
		policy.FailClosedOutputs = append([]string(nil), queued.AutonomyGuardPolicy.FailClosedOutputs...)
		policy.HumanScopeTriggers = append([]string(nil), queued.AutonomyGuardPolicy.HumanScopeTriggers...)
		policy.ForbiddenAuthorityClaims = append([]string(nil), queued.AutonomyGuardPolicy.ForbiddenAuthorityClaims...)
		out.AutonomyGuardPolicy = &policy
	}
	if queued.HumanRequiredClassificationPolicy != nil {
		policy := *queued.HumanRequiredClassificationPolicy
		policy.HumanRequiredTriggers = append([]string(nil), queued.HumanRequiredClassificationPolicy.HumanRequiredTriggers...)
		policy.EscalationOutputs = append([]string(nil), queued.HumanRequiredClassificationPolicy.EscalationOutputs...)
		policy.ForbiddenWithoutHuman = append([]string(nil), queued.HumanRequiredClassificationPolicy.ForbiddenWithoutHuman...)
		policy.NonAuthorityClaims = append([]string(nil), queued.HumanRequiredClassificationPolicy.NonAuthorityClaims...)
		out.HumanRequiredClassificationPolicy = &policy
	}
	if queued.AuthorityRecommendationPolicy != nil {
		policy := *queued.AuthorityRecommendationPolicy
		policy.EvidenceInputs = append([]string(nil), queued.AuthorityRecommendationPolicy.EvidenceInputs...)
		policy.RecommendationOutputs = append([]string(nil), queued.AuthorityRecommendationPolicy.RecommendationOutputs...)
		policy.RequiredHumanAuthority = append([]string(nil), queued.AuthorityRecommendationPolicy.RequiredHumanAuthority...)
		policy.ForbiddenAuthorityClaims = append([]string(nil), queued.AuthorityRecommendationPolicy.ForbiddenAuthorityClaims...)
		out.AuthorityRecommendationPolicy = &policy
	}
	if queued.ValueAllocationBoundaryPolicy != nil {
		policy := *queued.ValueAllocationBoundaryPolicy
		policy.ClassificationOutputs = append([]string(nil), queued.ValueAllocationBoundaryPolicy.ClassificationOutputs...)
		policy.RequiredHumanAuthority = append([]string(nil), queued.ValueAllocationBoundaryPolicy.RequiredHumanAuthority...)
		policy.ForbiddenSystemActions = append([]string(nil), queued.ValueAllocationBoundaryPolicy.ForbiddenSystemActions...)
		policy.ForbiddenAuthorityClaims = append([]string(nil), queued.ValueAllocationBoundaryPolicy.ForbiddenAuthorityClaims...)
		policy.NonAuthorityClaims = append([]string(nil), queued.ValueAllocationBoundaryPolicy.NonAuthorityClaims...)
		out.ValueAllocationBoundaryPolicy = &policy
	}
	out.DevelopmentLifecycle = append([]OperatorQueuedRunLifecycleStage(nil), queued.DevelopmentLifecycle...)
	for i := range out.DevelopmentLifecycle {
		out.DevelopmentLifecycle[i].RequiredRoles = append([]string(nil), out.DevelopmentLifecycle[i].RequiredRoles...)
		out.DevelopmentLifecycle[i].RequiredEvidence = append([]string(nil), out.DevelopmentLifecycle[i].RequiredEvidence...)
	}
	out.AgentExecutionPlan = append([]OperatorQueuedRunAgentPlanStep(nil), queued.AgentExecutionPlan...)
	for i := range out.AgentExecutionPlan {
		out.AgentExecutionPlan[i].RequiredInputs = append([]string(nil), out.AgentExecutionPlan[i].RequiredInputs...)
		out.AgentExecutionPlan[i].RequiredOutputs = append([]string(nil), out.AgentExecutionPlan[i].RequiredOutputs...)
	}

	declaredStages := map[string]bool{}
	stageTaskDrafts := map[string]bool{}
	stageRuntimeEvidence := map[string]bool{}
	stageCompletedRuntimeEvidence := map[string]bool{}
	roleOutputRuntimeEvidence := map[string]bool{}
	runID := safeRunLaunchID(out.RunID)
	queuedFactoryOrderID, err := factoryOrderIDForRunLaunch(out.RunID)
	if err != nil {
		queuedFactoryOrderID = ""
	}
	for _, ref := range stageArtifactRefs {
		if ref.RunID != runID || ref.StageID == "" || ref.EventRef == "" {
			continue
		}
		declaredStages[ref.StageID] = true
	}
	for _, task := range tasks {
		stageID := safeRunLaunchID(task.LifecycleStageID)
		if queuedFactoryOrderID == "" || strings.TrimSpace(task.FactoryOrderID) != queuedFactoryOrderID || stageID == "" || strings.TrimSpace(task.ID) == "" {
			continue
		}
		stageTaskDrafts[stageID] = true
		if strings.TrimSpace(task.RuntimeEvidenceStatus) != "" {
			stageRuntimeEvidence[stageID] = true
		}
		if strings.TrimSpace(task.RuntimeEvidenceStatus) == civilizationAssemblyQueuedStageCompletedStatus {
			stageCompletedRuntimeEvidence[stageID] = true
		}
		for _, contract := range task.RoleOutputContracts {
			role := strings.TrimSpace(contract.Role)
			if role == "" {
				continue
			}
			if strings.TrimSpace(contract.EvidenceStatus) == civilizationAssemblyQueuedPlanStepRuntimeStatus {
				roleOutputRuntimeEvidence[stageID+"|"+role] = true
			}
		}
	}
	if len(declaredStages) == 0 && len(stageTaskDrafts) == 0 && len(stageRuntimeEvidence) == 0 && len(stageCompletedRuntimeEvidence) == 0 {
		return &out
	}
	for i := range out.DevelopmentLifecycle {
		stageID := safeRunLaunchID(out.DevelopmentLifecycle[i].ID)
		if stageCompletedRuntimeEvidence[stageID] {
			out.DevelopmentLifecycle[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.DevelopmentLifecycle[i].EvidenceStatus, civilizationAssemblyQueuedStageCompletedStatus)
		} else if stageRuntimeEvidence[stageID] {
			out.DevelopmentLifecycle[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.DevelopmentLifecycle[i].EvidenceStatus, civilizationAssemblyQueuedStageRuntimeStatus)
		} else if stageTaskDrafts[stageID] {
			out.DevelopmentLifecycle[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.DevelopmentLifecycle[i].EvidenceStatus, civilizationAssemblyQueuedStageTaskDraftStatus)
		} else if declaredStages[stageID] {
			out.DevelopmentLifecycle[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.DevelopmentLifecycle[i].EvidenceStatus, civilizationAssemblyQueuedStageDeclaredStatus)
		}
	}
	for i := range out.AgentExecutionPlan {
		stageID := safeRunLaunchID(out.AgentExecutionPlan[i].StageID)
		role := strings.TrimSpace(out.AgentExecutionPlan[i].Role)
		if stageCompletedRuntimeEvidence[stageID] {
			out.AgentExecutionPlan[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.AgentExecutionPlan[i].EvidenceStatus, civilizationAssemblyQueuedPlanStepCompletedStatus)
		} else if stageRuntimeEvidence[stageID] {
			out.AgentExecutionPlan[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.AgentExecutionPlan[i].EvidenceStatus, civilizationAssemblyQueuedPlanStepRuntimeStatus)
		} else if roleOutputRuntimeEvidence[stageID+"|"+role] {
			out.AgentExecutionPlan[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.AgentExecutionPlan[i].EvidenceStatus, civilizationAssemblyQueuedPlanStepRuntimeStatus)
		} else if stageTaskDrafts[stageID] {
			out.AgentExecutionPlan[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.AgentExecutionPlan[i].EvidenceStatus, civilizationAssemblyQueuedPlanStepTaskDraftStatus)
		} else if declaredStages[stageID] {
			out.AgentExecutionPlan[i].EvidenceStatus = civilizationAssemblyDeclaredEvidenceStatus(out.AgentExecutionPlan[i].EvidenceStatus, civilizationAssemblyQueuedPlanStepDeclaredStatus)
		}
	}
	return &out
}

func civilizationAssemblyDeclaredEvidenceStatus(current, declared string) string {
	switch strings.TrimSpace(current) {
	case "", "expected_not_observed", declared:
		return declared
	default:
		return current
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

func civilizationAssemblyResidualRisks(p OperatorProjection, factoryOrdersTruncated bool, factoryOrderWorkEvidence civilizationAssemblyFactoryOrderWorkEvidence, issueScanTruncated bool, limit int) []CivilizationAssemblyResidualRisk {
	risks := make([]CivilizationAssemblyResidualRisk, 0, len(p.RuntimeEvidence.Limitations)+len(p.Errors)+5)
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
	if len(factoryOrderWorkEvidence.TaskRefs) > 0 && factoryOrderWorkEvidence.ArtifactSourceTruncated {
		if limit <= 0 {
			limit = defaultOperatorProjectionLimit
		}
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       "factory_order_artifact_source_limit_01",
			Kind:     "factory_order_projection_limit",
			Severity: "info",
			Status:   "open",
			Summary:  fmt.Sprintf("FactoryOrder artifact evidence is bounded to %d work.task.artifact events; artifact refs outside that projection page may be omitted.", limit),
		})
	}
	if len(factoryOrderWorkEvidence.TaskRefs) > 0 && factoryOrderWorkEvidence.DependencySourceTruncated {
		if limit <= 0 {
			limit = defaultOperatorProjectionLimit
		}
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       "factory_order_dependency_source_limit_01",
			Kind:     "factory_order_projection_limit",
			Severity: "info",
			Status:   "open",
			Summary:  fmt.Sprintf("FactoryOrder task dependency evidence is bounded to %d work.task.dependency.added events; dependency refs outside that projection page may be omitted.", limit),
		})
	}
	if len(factoryOrderWorkEvidence.TaskRefs) > 0 && factoryOrderWorkEvidence.LifecycleSourceTruncated {
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
	if len(factoryOrderWorkEvidence.TaskRefs) > 0 && factoryOrderWorkEvidence.VerificationSourceTruncated {
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
	if issueScanTruncated {
		if limit <= 0 {
			limit = defaultOperatorProjectionLimit
		}
		risks = append(risks, CivilizationAssemblyResidualRisk{
			ID:       "issue_scan_parked_source_limit_01",
			Kind:     "issue_scan_projection_limit",
			Severity: "info",
			Status:   "open",
			Summary:  fmt.Sprintf("Issue-scan parked evidence is bounded to %d hive.issuescan.run.parked events; parked issue refs outside that projection page may be omitted.", limit),
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
	if len(p.Lifecycle) == 0 && len(civilizationAssemblyRuntimeAgents(p.RuntimeEvidence.AgentEvents)) == 0 {
		fields = append(fields, CivilizationAssemblyUnavailableField{
			Field:  "actor_roster",
			Status: civilizationAssemblyFieldUnavailable,
			Reason: "no lifecycle or runtime agent records were projected",
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

func mergeCivilizationAssemblyTaskRoleContract(existing, candidate civilizationAssemblyTaskRoleContract) (civilizationAssemblyTaskRoleContract, bool) {
	existing = compactCivilizationAssemblyTaskRoleContract(existing)
	candidate = compactCivilizationAssemblyTaskRoleContract(candidate)
	if existing.StageID == "" {
		return candidate, false
	}
	if candidate.StageID == "" {
		return existing, false
	}
	if safeRunLaunchID(existing.StageID) != safeRunLaunchID(candidate.StageID) || existing.FactoryOrderID != candidate.FactoryOrderID {
		return existing, true
	}
	existing.RequiredRoles = append(existing.RequiredRoles, candidate.RequiredRoles...)
	existing.AgentExecutionPlan = append(existing.AgentExecutionPlan, candidate.AgentExecutionPlan...)
	existing.SourceRefs = append(existing.SourceRefs, candidate.SourceRefs...)
	return compactCivilizationAssemblyTaskRoleContract(existing), false
}

func compactCivilizationAssemblyTaskRoleContract(value civilizationAssemblyTaskRoleContract) civilizationAssemblyTaskRoleContract {
	value.StageID = strings.TrimSpace(value.StageID)
	value.FactoryOrderID = strings.TrimSpace(value.FactoryOrderID)
	value.RequiredRoles = compactStrings(value.RequiredRoles)
	value.AgentExecutionPlan = compactOperatorQueuedRunAgentPlanSteps(value.AgentExecutionPlan)
	value.SourceRefs = compactStrings(value.SourceRefs)
	if value.StageID == "" || value.FactoryOrderID == "" {
		return civilizationAssemblyTaskRoleContract{}
	}
	return value
}

func mergeCivilizationAssemblyTaskOutputContract(existing, candidate civilizationAssemblyTaskOutputContract) (civilizationAssemblyTaskOutputContract, bool) {
	existing = compactCivilizationAssemblyTaskOutputContract(existing)
	candidate = compactCivilizationAssemblyTaskOutputContract(candidate)
	if existing.StageID == "" {
		return candidate, false
	}
	if candidate.StageID == "" {
		return existing, false
	}
	if safeRunLaunchID(existing.StageID) != safeRunLaunchID(candidate.StageID) || existing.FactoryOrderID != candidate.FactoryOrderID {
		return existing, true
	}
	existing.RequiredEvidence = append(existing.RequiredEvidence, candidate.RequiredEvidence...)
	existing.ExpectedOutputs = append(existing.ExpectedOutputs, candidate.ExpectedOutputs...)
	existing.RoleOutputContracts = append(existing.RoleOutputContracts, candidate.RoleOutputContracts...)
	existing.SourceRefs = append(existing.SourceRefs, candidate.SourceRefs...)
	return compactCivilizationAssemblyTaskOutputContract(existing), false
}

func compactCivilizationAssemblyTaskOutputContract(value civilizationAssemblyTaskOutputContract) civilizationAssemblyTaskOutputContract {
	value.StageID = strings.TrimSpace(value.StageID)
	value.FactoryOrderID = strings.TrimSpace(value.FactoryOrderID)
	value.RequiredEvidence = compactStrings(value.RequiredEvidence)
	value.ExpectedOutputs = compactStrings(value.ExpectedOutputs)
	value.RoleOutputContracts = compactCivilizationAssemblyRoleOutputContracts(value.RoleOutputContracts)
	value.SourceRefs = compactStrings(value.SourceRefs)
	if value.StageID == "" || value.FactoryOrderID == "" {
		return civilizationAssemblyTaskOutputContract{}
	}
	return value
}

func mergeCivilizationAssemblyTaskRuntimeEvidence(existing, candidate civilizationAssemblyTaskRuntimeEvidence) (civilizationAssemblyTaskRuntimeEvidence, bool) {
	existing = compactCivilizationAssemblyTaskRuntimeEvidence(existing)
	candidate = compactCivilizationAssemblyTaskRuntimeEvidence(candidate)
	if existing.StageID == "" {
		return candidate, false
	}
	if candidate.StageID == "" {
		return existing, false
	}
	if safeRunLaunchID(existing.StageID) != safeRunLaunchID(candidate.StageID) || existing.FactoryOrderID != candidate.FactoryOrderID {
		return existing, true
	}
	existing.Status = civilizationAssemblyTaskRuntimeStatus(existing.Status, candidate.Status)
	existing.RequiredEvidence = append(existing.RequiredEvidence, candidate.RequiredEvidence...)
	existing.Roles = append(existing.Roles, candidate.Roles...)
	existing.RoleOutputRoles = append(existing.RoleOutputRoles, candidate.RoleOutputRoles...)
	existing.RoleOutputRefs = append(existing.RoleOutputRefs, candidate.RoleOutputRefs...)
	existing.SourceRefs = append(existing.SourceRefs, candidate.SourceRefs...)
	return compactCivilizationAssemblyTaskRuntimeEvidence(existing), false
}

func compactCivilizationAssemblyTaskRuntimeEvidence(value civilizationAssemblyTaskRuntimeEvidence) civilizationAssemblyTaskRuntimeEvidence {
	value.StageID = strings.TrimSpace(value.StageID)
	value.FactoryOrderID = strings.TrimSpace(value.FactoryOrderID)
	value.Status = civilizationAssemblyTaskRuntimeStatus(value.Status, "")
	value.RequiredEvidence = compactStrings(value.RequiredEvidence)
	value.Roles = compactStrings(value.Roles)
	value.RoleOutputRoles = compactStrings(value.RoleOutputRoles)
	value.RoleOutputRefs = compactStrings(value.RoleOutputRefs)
	value.SourceRefs = compactStrings(value.SourceRefs)
	if value.StageID == "" || value.FactoryOrderID == "" {
		return civilizationAssemblyTaskRuntimeEvidence{}
	}
	return value
}

func civilizationAssemblyTaskRuntimeStatus(existing, candidate string) string {
	existing = strings.TrimSpace(existing)
	candidate = strings.TrimSpace(candidate)
	if existing == "" {
		return candidate
	}
	if candidate == "" {
		return existing
	}
	if civilizationAssemblyTaskRuntimeStatusRank(candidate) > civilizationAssemblyTaskRuntimeStatusRank(existing) {
		return candidate
	}
	return existing
}

func civilizationAssemblyTaskRuntimeStatusRank(status string) int {
	switch strings.TrimSpace(status) {
	case civilizationAssemblyQueuedStageCompletedStatus, civilizationAssemblyQueuedPlanStepCompletedStatus, "completed", "pass":
		return 30
	case civilizationAssemblyQueuedStageRuntimeStatus, civilizationAssemblyQueuedPlanStepRuntimeStatus, "recorded":
		return 20
	default:
		if strings.TrimSpace(status) == "" {
			return 0
		}
		return 10
	}
}

func compactCivilizationAssemblyRoleOutputContracts(values []CivilizationAssemblyRoleOutputContract) []CivilizationAssemblyRoleOutputContract {
	byRole := map[string]CivilizationAssemblyRoleOutputContract{}
	for _, value := range values {
		value.Role = strings.TrimSpace(value.Role)
		if value.Role == "" {
			continue
		}
		value.RequiredOutputs = compactStrings(value.RequiredOutputs)
		value.AuthorityBoundary = strings.TrimSpace(value.AuthorityBoundary)
		value.CompletionGate = strings.TrimSpace(value.CompletionGate)
		value.EvidenceStatus = strings.TrimSpace(value.EvidenceStatus)
		if existing, ok := byRole[value.Role]; ok {
			value.CanOperate = value.CanOperate || existing.CanOperate
			value.RequiredOutputs = compactStrings(append(existing.RequiredOutputs, value.RequiredOutputs...))
			if value.AuthorityBoundary == "" {
				value.AuthorityBoundary = existing.AuthorityBoundary
			}
			if value.CompletionGate == "" {
				value.CompletionGate = existing.CompletionGate
			}
			if value.EvidenceStatus == "" {
				value.EvidenceStatus = existing.EvidenceStatus
			}
		}
		byRole[value.Role] = value
	}
	out := make([]CivilizationAssemblyRoleOutputContract, 0, len(byRole))
	for _, value := range byRole {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Role < out[j].Role
	})
	return out
}

func compactOperatorQueuedRunAgentPlanSteps(values []OperatorQueuedRunAgentPlanStep) []OperatorQueuedRunAgentPlanStep {
	byID := map[string]OperatorQueuedRunAgentPlanStep{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		if value.ID == "" {
			value.ID = strings.TrimSpace(value.StageID) + ":" + strings.TrimSpace(value.Role)
		}
		value.StageID = strings.TrimSpace(value.StageID)
		value.Role = strings.TrimSpace(value.Role)
		value.Objective = strings.TrimSpace(value.Objective)
		value.RequiredInputs = compactStrings(value.RequiredInputs)
		value.RequiredOutputs = compactStrings(value.RequiredOutputs)
		value.AuthorityBoundary = strings.TrimSpace(value.AuthorityBoundary)
		value.CompletionGate = strings.TrimSpace(value.CompletionGate)
		value.EvidenceStatus = strings.TrimSpace(value.EvidenceStatus)
		if value.StageID == "" || value.Role == "" {
			continue
		}
		byID[value.ID] = value
	}
	out := make([]OperatorQueuedRunAgentPlanStep, 0, len(byID))
	for _, value := range byID {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func compactCivilizationAssemblyTaskEvidence(values []CivilizationAssemblyTaskEvidence) []CivilizationAssemblyTaskEvidence {
	byID := map[string]CivilizationAssemblyTaskEvidence{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		if value.ID == "" {
			continue
		}
		value.CanonicalTaskID = strings.TrimSpace(value.CanonicalTaskID)
		value.FactoryOrderID = strings.TrimSpace(value.FactoryOrderID)
		value.LifecycleStageID = strings.TrimSpace(value.LifecycleStageID)
		value.Title = strings.TrimSpace(value.Title)
		value.Cell = strings.TrimSpace(value.Cell)
		value.RiskClass = strings.TrimSpace(value.RiskClass)
		value.Status = strings.TrimSpace(value.Status)
		value.RequirementRefs = compactStrings(value.RequirementRefs)
		value.AcceptanceCriterionRefs = compactStrings(value.AcceptanceCriterionRefs)
		value.ExpectedOutputs = compactStrings(value.ExpectedOutputs)
		value.DependsOnRefs = compactStrings(value.DependsOnRefs)
		value.SourceRefs = compactStrings(value.SourceRefs)
		value.RequiredRoles = compactStrings(value.RequiredRoles)
		value.RoleContractRefs = compactStrings(value.RoleContractRefs)
		value.AgentExecutionPlan = compactOperatorQueuedRunAgentPlanSteps(value.AgentExecutionPlan)
		value.RequiredEvidence = compactStrings(value.RequiredEvidence)
		value.OutputContractRefs = compactStrings(value.OutputContractRefs)
		value.RoleOutputContracts = compactCivilizationAssemblyRoleOutputContracts(value.RoleOutputContracts)
		if existing, ok := byID[value.ID]; ok {
			value.SourceRefs = compactStrings(append(existing.SourceRefs, value.SourceRefs...))
			value.DependsOnRefs = compactStrings(append(existing.DependsOnRefs, value.DependsOnRefs...))
			if value.CanonicalTaskID == "" {
				value.CanonicalTaskID = existing.CanonicalTaskID
			}
			if value.FactoryOrderID == "" {
				value.FactoryOrderID = existing.FactoryOrderID
			}
			if value.LifecycleStageID == "" {
				value.LifecycleStageID = existing.LifecycleStageID
			}
			if value.Title == "" {
				value.Title = existing.Title
			}
			if value.Cell == "" {
				value.Cell = existing.Cell
			}
			if value.RiskClass == "" {
				value.RiskClass = existing.RiskClass
			}
			if value.Status == "" {
				value.Status = existing.Status
			}
			value.Ready, value.Blocked = civilizationAssemblyNormalizeTaskReadiness(value.Status, value.Ready || existing.Ready, value.Blocked || existing.Blocked)
			value.RequirementRefs = compactStrings(append(existing.RequirementRefs, value.RequirementRefs...))
			value.AcceptanceCriterionRefs = compactStrings(append(existing.AcceptanceCriterionRefs, value.AcceptanceCriterionRefs...))
			value.ExpectedOutputs = compactStrings(append(existing.ExpectedOutputs, value.ExpectedOutputs...))
			value.RequiredRoles = compactStrings(append(existing.RequiredRoles, value.RequiredRoles...))
			value.RoleContractRefs = compactStrings(append(existing.RoleContractRefs, value.RoleContractRefs...))
			value.AgentExecutionPlan = compactOperatorQueuedRunAgentPlanSteps(append(existing.AgentExecutionPlan, value.AgentExecutionPlan...))
			value.RequiredEvidence = compactStrings(append(existing.RequiredEvidence, value.RequiredEvidence...))
			value.OutputContractRefs = compactStrings(append(existing.OutputContractRefs, value.OutputContractRefs...))
			value.RoleOutputContracts = compactCivilizationAssemblyRoleOutputContracts(append(existing.RoleOutputContracts, value.RoleOutputContracts...))
		}
		byID[value.ID] = value
	}
	out := make([]CivilizationAssemblyTaskEvidence, 0, len(byID))
	for _, value := range byID {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FactoryOrderID != out[j].FactoryOrderID {
			return out[i].FactoryOrderID < out[j].FactoryOrderID
		}
		if out[i].LifecycleStageID != out[j].LifecycleStageID {
			if out[i].LifecycleStageID == "" {
				return true
			}
			if out[j].LifecycleStageID == "" {
				return false
			}
			return out[i].LifecycleStageID < out[j].LifecycleStageID
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func compactCivilizationAssemblyArtifactEvidence(values []CivilizationAssemblyArtifactEvidence) []CivilizationAssemblyArtifactEvidence {
	byID := map[string]CivilizationAssemblyArtifactEvidence{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		if value.ID == "" {
			continue
		}
		value.TaskRef = strings.TrimSpace(value.TaskRef)
		value.Label = strings.TrimSpace(value.Label)
		value.MediaType = strings.TrimSpace(value.MediaType)
		value.SourceRefs = compactStrings(value.SourceRefs)
		if existing, ok := byID[value.ID]; ok {
			value.SourceRefs = compactStrings(append(existing.SourceRefs, value.SourceRefs...))
			if value.TaskRef == "" {
				value.TaskRef = existing.TaskRef
			}
			if value.Label == "" {
				value.Label = existing.Label
			}
			if value.MediaType == "" {
				value.MediaType = existing.MediaType
			}
		}
		byID[value.ID] = value
	}
	out := make([]CivilizationAssemblyArtifactEvidence, 0, len(byID))
	for _, value := range byID {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func compactCivilizationAssemblyStageArtifactRefs(values []civilizationAssemblyStageArtifactRef) []civilizationAssemblyStageArtifactRef {
	seen := map[string]bool{}
	out := make([]civilizationAssemblyStageArtifactRef, 0, len(values))
	for _, value := range values {
		value.RunID = safeRunLaunchID(value.RunID)
		value.StageID = safeRunLaunchID(value.StageID)
		value.EventRef = strings.TrimSpace(value.EventRef)
		if value.RunID == "" || value.StageID == "" || value.EventRef == "" {
			continue
		}
		key := value.RunID + "\x00" + value.StageID + "\x00" + value.EventRef
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RunID != out[j].RunID {
			return out[i].RunID < out[j].RunID
		}
		if out[i].StageID != out[j].StageID {
			return out[i].StageID < out[j].StageID
		}
		return out[i].EventRef < out[j].EventRef
	})
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
