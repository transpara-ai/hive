package factoryv1

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Projection struct {
	SchemaVersion string                   `json:"schema_version"`
	GeneratedAt   time.Time                `json:"generated_at"`
	Service       ServiceProjection        `json:"service"`
	Orders        []OrderProjection        `json:"orders"`
	Ideas         []IdeaProjection         `json:"ideas"`
	Interventions []InterventionProjection `json:"interventions"`
}

type ServiceProjection struct {
	ServiceID          string    `json:"service_id"`
	InstanceID         string    `json:"instance_id"`
	RecoveryGeneration int       `json:"recovery_generation"`
	StartedAt          time.Time `json:"started_at"`
	Healthy            bool      `json:"healthy"`
	Detail             string    `json:"detail,omitempty"`
}

type OrderProjection struct {
	OrderID              string                  `json:"order_id"`
	Version              string                  `json:"version"`
	Title                string                  `json:"title"`
	Channel              Channel                 `json:"channel"`
	TargetRepository     string                  `json:"target_repository"`
	SourceRef            SourceReference         `json:"source_ref"`
	DocumentSHA256       string                  `json:"document_sha256"`
	EngineProtocol       string                  `json:"engine_protocol"`
	Status               string                  `json:"status"`
	TLCStage             Stage                   `json:"tlc_stage"`
	TLCIndex             int                     `json:"tlc_index"`
	ElapsedMS            int64                   `json:"elapsed_ms"`
	StartedAt            time.Time               `json:"started_at"`
	LastEffectAt         time.Time               `json:"last_effect_at"`
	ActiveAttemptID      string                  `json:"active_attempt_id"`
	ActivelyExecuting    bool                    `json:"actively_executing"`
	Peers                []string                `json:"peers"`
	GateState            string                  `json:"gate_state"`
	Evidence             []Evidence              `json:"evidence"`
	Blocker              string                  `json:"blocker"`
	NextAction           string                  `json:"next_action"`
	Budget               BudgetProjection        `json:"budget"`
	PR                   *PRProjection           `json:"pr"`
	HumanApprovalBasis   ApprovalBasis           `json:"human_approval_basis"`
	HumanApprovalReceipt *HumanApprovalReceipt   `json:"human_approval_receipt"`
	Stages               []StageLedgerProjection `json:"stages"`
}

type StageLedgerProjection struct {
	Stage          Stage           `json:"stage"`
	Index          int             `json:"index"`
	State          TransitionState `json:"state"`
	AttemptID      string          `json:"attempt_id"`
	Ordinal        int             `json:"ordinal"`
	EventID        string          `json:"event_id"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Peers          []string        `json:"peers"`
	Evidence       []Evidence      `json:"evidence"`
	WorkArtifactID string          `json:"work_artifact_id,omitempty"`
	Recovered      bool            `json:"recovered"`
}

type BudgetProjection struct {
	MaxAttempts         int   `json:"max_attempts"`
	ConsumedAttempts    int   `json:"consumed_attempts"`
	RemainingAttempts   int   `json:"remaining_attempts"`
	MaxTokens           int64 `json:"max_tokens"`
	ConsumedTokens      int64 `json:"consumed_tokens"`
	RemainingTokens     int64 `json:"remaining_tokens"`
	MaxCostMicros       int64 `json:"max_cost_micros"`
	ConsumedCostMicros  int64 `json:"consumed_cost_micros"`
	RemainingCostMicros int64 `json:"remaining_cost_micros"`
	Exhausted           bool  `json:"exhausted"`
}

type PRProjection struct {
	Repository        string    `json:"repository"`
	Number            int       `json:"number"`
	URL               string    `json:"url"`
	State             string    `json:"state"`
	HeadSHA           string    `json:"head_sha"`
	ReviewedHeadSHA   string    `json:"reviewed_head_sha"`
	Open              bool      `json:"open"`
	Draft             bool      `json:"draft"`
	ChecksPassing     bool      `json:"checks_passing"`
	MergeStateStatus  string    `json:"merge_state_status,omitempty"`
	ObservedAt        time.Time `json:"observed_at,omitempty"`
	ObservationSource string    `json:"observation_source"`
	ObservationStatus string    `json:"observation_status"`
	ObservationDetail string    `json:"observation_detail,omitempty"`
}

// PRObservation is a current read-only observation of a pull request. The
// reviewed head deliberately remains ledger-owned so the projector can compare
// the durable gate result with external GitHub truth without transferring
// review credit.
type PRObservation struct {
	Repository       string
	Number           int
	URL              string
	State            string
	HeadSHA          string
	Draft            bool
	ChecksPassing    bool
	MergeStateStatus string
	ObservedAt       time.Time
	Source           string
	Detail           string
}

type PRObserver interface {
	ObservePR(ctx context.Context, repository string, number int) (PRObservation, error)
}

type InterventionProjection struct {
	InterventionID string             `json:"intervention_id"`
	OrderID        string             `json:"order_id"`
	Kind           string             `json:"kind"`
	Prompt         string             `json:"prompt"`
	Status         InterventionStatus `json:"status"`
	RequestedAt    time.Time          `json:"requested_at"`
	EventID        string             `json:"event_id"`
}

type IdeaProjection struct {
	IdeaID           string         `json:"idea_id"`
	Title            string         `json:"title"`
	TargetRepository string         `json:"target_repository"`
	Status           string         `json:"status"`
	CurrentRevision  int            `json:"current_revision"`
	Revisions        []IdeaRevision `json:"revisions"`
}

type Projector struct {
	store      Store
	work       WorkStore
	clock      Clock
	service    ServiceProjection
	prObserver PRObserver
}

type ProjectorOption func(*Projector) error

func WithPRObserver(observer PRObserver) ProjectorOption {
	return func(projector *Projector) error {
		if observer == nil {
			return errors.New("factory v1 PR observer is nil")
		}
		projector.prObserver = observer
		return nil
	}
}

func NewProjector(store Store, work WorkStore, clock Clock, service ServiceProjection, options ...ProjectorOption) (*Projector, error) {
	if store == nil || work == nil {
		return nil, errors.New("factory v1 projector requires EventGraph and Work stores")
	}
	if clock == nil {
		clock = WallClock{}
	}
	if service.ServiceID == "" {
		service.ServiceID = "hive-factory-v1"
	}
	projector := &Projector{store: store, work: work, clock: clock, service: service}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("factory v1 projector option is nil")
		}
		if err := option(projector); err != nil {
			return nil, err
		}
	}
	return projector, nil
}

func (p *Projector) Build(ctx context.Context) (Projection, error) {
	events, err := p.store.List(ctx)
	if err != nil {
		return Projection{}, err
	}
	events = eventsByTime(events)
	projection := Projection{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   p.clock.Now().UTC(),
		Service:       p.service,
		Orders:        []OrderProjection{},
		Ideas:         []IdeaProjection{},
		Interventions: []InterventionProjection{},
	}
	projection.Orders, err = p.projectOrders(ctx, events, projection.GeneratedAt)
	if err != nil {
		return Projection{}, err
	}
	projection.Ideas, err = projectIdeas(events)
	if err != nil {
		return Projection{}, err
	}
	projection.Interventions, err = projectInterventions(events)
	if err != nil {
		return Projection{}, err
	}
	return projection, nil
}

func (p *Projector) projectOrders(ctx context.Context, events []Event, now time.Time) ([]OrderProjection, error) {
	var orders []OrderProjection
	for _, acceptedEvent := range events {
		if acceptedEvent.Type != EventOrderAccepted {
			continue
		}
		accepted, err := decodeEvent[OrderAcceptedPayload](acceptedEvent)
		if err != nil {
			return nil, err
		}
		order := accepted.Document.Order
		item := OrderProjection{
			OrderID: order.DocID, Version: order.Version, Title: order.Title, Channel: order.Channel,
			TargetRepository: order.TargetRepository, DocumentSHA256: accepted.Document.SHA256,
			EngineProtocol: TLCVersion, Status: "accepted",
			TLCStage: StageIngestWork, TLCIndex: 0,
			ElapsedMS: max(int64(0), now.Sub(acceptedEvent.OccurredAt).Milliseconds()),
			StartedAt: acceptedEvent.OccurredAt, LastEffectAt: acceptedEvent.OccurredAt,
			Peers: PeersForStage(StageIngestWork), GateState: "unavailable",
			Evidence: []Evidence{}, NextAction: "start ingest_work",
			HumanApprovalBasis:   accepted.HumanApprovalBasis,
			HumanApprovalReceipt: accepted.HumanApprovalReceipt,
			Stages:               []StageLedgerProjection{},
		}
		if len(order.SourceReferences) > 0 {
			item.SourceRef = order.SourceReferences[0]
		} else {
			item.Status, item.Blocker, item.NextAction = "blocked", "accepted order has no immutable source reference", "repair accepted EventGraph evidence"
		}
		item.Budget = deriveBudget(order.Budget, nil)
		transitions, ledger, _, err := orderTransitions(events, order.DocID)
		if err != nil {
			return nil, err
		}
		item.Stages = ledger
		if len(ledger) > 0 {
			item.LastEffectAt = ledger[len(ledger)-1].OccurredAt
		}
		item.Budget = deriveBudget(order.Budget, transitions)
		if transitionErr := validateProjectedTransitions(accepted.Document.SHA256, transitions); transitionErr != nil {
			applyTransitions(&item, transitions)
			item.Status = "blocked"
			item.ActivelyExecuting = false
			item.ActiveAttemptID = ""
			item.GateState = "unavailable"
			item.Blocker = "invalid TLC ledger: " + transitionErr.Error()
			item.NextAction = "repair or invalidate the conflicting stage ledger"
		} else {
			applyTransitions(&item, transitions)
		}
		if item.Budget.Exhausted && item.Status != "human_required" && item.Status != "human_review" {
			item.Status = "blocked"
			item.Blocker = "per-order budget exhausted"
			item.NextAction = "resolve budget intervention"
		}
		link, workErr := p.work.GetFactoryOrder(ctx, order.DocID, order.Version)
		if workErr != nil || link.DocumentSHA256 != accepted.Document.SHA256 || link.AcceptedEventID != acceptedEvent.ID || link.Quarantined {
			item.Status = "blocked"
			item.ActivelyExecuting = false
			item.Blocker = "Work linkage is missing, conflicting, or quarantined"
			item.NextAction = "reconcile accepted EventGraph event to Work"
		}
		if item.PR != nil {
			p.reconcilePR(ctx, &item)
		}
		if hasOpenIntervention(events, order.DocID) {
			item.Status = "human_required"
			item.ActivelyExecuting = false
			item.NextAction = "resolve the open intervention"
		}
		orders = append(orders, item)
	}
	sort.Slice(orders, func(i, j int) bool {
		return workTuple(orders[i].OrderID, orders[i].Version) < workTuple(orders[j].OrderID, orders[j].Version)
	})
	return orders, nil
}

func (p *Projector) reconcilePR(ctx context.Context, item *OrderProjection) {
	pr := item.PR
	if p.prObserver == nil {
		pr.State = stateFromLedger(pr.Open, pr.Draft)
		pr.ObservationSource = "factory_v1_ledger"
		pr.ObservationStatus = "ledger_only"
		item.Status = "blocked"
		item.ActivelyExecuting = false
		item.ActiveAttemptID = ""
		item.GateState = "unavailable"
		item.Blocker = "current GitHub PR observation is not configured"
		item.NextAction = "configure read-only GitHub observation and reconcile the exact PR head"
		return
	}
	observation, err := p.prObserver.ObservePR(ctx, pr.Repository, pr.Number)
	if err != nil {
		pr.State = "unknown"
		pr.HeadSHA = ""
		pr.Open = false
		pr.Draft = false
		pr.ChecksPassing = false
		pr.ObservedAt = p.clock.Now().UTC()
		pr.ObservationSource = "github"
		pr.ObservationStatus = "unavailable"
		pr.ObservationDetail = "read-only PR observation failed"
		item.Status = "blocked"
		item.ActivelyExecuting = false
		item.ActiveAttemptID = ""
		item.GateState = "unavailable"
		item.Blocker = "current GitHub PR state is unavailable"
		item.NextAction = "restore read-only GitHub observation and reconcile the exact PR head"
		return
	}

	state := strings.ToLower(strings.TrimSpace(observation.State))
	pr.Repository = observation.Repository
	pr.Number = observation.Number
	pr.URL = observation.URL
	pr.State = state
	pr.HeadSHA = observation.HeadSHA
	pr.Open = state == "open"
	pr.Draft = observation.Draft
	pr.ChecksPassing = observation.ChecksPassing
	pr.MergeStateStatus = strings.ToLower(strings.TrimSpace(observation.MergeStateStatus))
	pr.ObservedAt = observation.ObservedAt.UTC()
	if pr.ObservedAt.IsZero() {
		pr.ObservedAt = p.clock.Now().UTC()
	}
	pr.ObservationSource = strings.TrimSpace(observation.Source)
	if pr.ObservationSource == "" {
		pr.ObservationSource = "github"
	}
	pr.ObservationStatus = "current"
	pr.ObservationDetail = observation.Detail

	if state != "open" {
		item.Status = "blocked"
		item.ActivelyExecuting = false
		item.ActiveAttemptID = ""
		item.Blocker = fmt.Sprintf("output PR is no longer open (current state: %s)", state)
		item.NextAction = "admit a current order for a new unmerged output PR; preserve the historical result"
		return
	}
	if pr.Draft && item.Status == "human_review" {
		item.Status = "blocked"
		item.Blocker = "output PR is still a draft"
		item.NextAction = "complete exact-head gates and mark the PR ready for Human Review"
		return
	}
	if item.Status != "human_review" {
		return
	}
	if pr.MergeStateStatus == "behind" || pr.MergeStateStatus == "dirty" {
		item.Status = "blocked"
		item.Blocker = "live PR branch is not synchronized with its base branch"
		item.NextAction = "update the branch, then rerun exact-head IAR and CFAR"
		return
	}
	if pr.HeadSHA == "" || pr.ReviewedHeadSHA == "" || pr.HeadSHA != pr.ReviewedHeadSHA {
		item.Status = "blocked"
		item.Blocker = "live PR head does not match the exact reviewed head"
		item.NextAction = "run exact-head IAR and CFAR for the current PR head"
		return
	}
	if !pr.ChecksPassing {
		item.Status = "blocked"
		item.Blocker = "required checks are not passing on the live PR head"
		item.NextAction = "repair or complete required checks on the exact reviewed head"
	}
}

func stateFromLedger(open, draft bool) string {
	if !open {
		return "closed"
	}
	if draft {
		return "draft"
	}
	return "open"
}

func validateProjectedTransitions(documentSHA256 string, transitions []StageTransitionPayload) error {
	seenTerminal := make(map[string]TransitionState)
	for index, transition := range transitions {
		if err := ValidateTransitionForDocument(documentSHA256, transitions[:index], transition); err != nil {
			return err
		}
		if transition.State == TransitionRunning {
			continue
		}
		if state, exists := seenTerminal[transition.AttemptID]; exists {
			return fmt.Errorf("attempt %s has duplicate terminal states %s and %s", transition.AttemptID, state, transition.State)
		}
		seenTerminal[transition.AttemptID] = transition.State
	}
	return nil
}

func applyTransitions(item *OrderProjection, transitions []StageTransitionPayload) {
	if len(transitions) == 0 {
		return
	}
	latest := transitions[len(transitions)-1]
	item.TLCStage = latest.Stage
	item.TLCIndex = latest.StageIndex
	item.Peers = append([]string(nil), latest.Peers...)
	item.Evidence = append([]Evidence(nil), latest.Evidence...)
	item.Blocker = latest.Blocker
	item.NextAction = latest.NextAction
	item.ActiveAttemptID = ""
	item.ActivelyExecuting = false
	switch latest.State {
	case TransitionRunning:
		item.Status = "progressing"
		item.ActiveAttemptID = latest.AttemptID
		item.ActivelyExecuting = true
		item.NextAction = "complete or reconcile " + string(latest.Stage)
	case TransitionBlocked:
		item.Status = "blocked"
		if item.NextAction == "" {
			item.NextAction = "resolve stage blocker"
		}
	case TransitionHumanRequired:
		item.Status = "human_required"
		if latest.Stage == StageHumanReview {
			item.Status = "human_review"
			item.NextAction = "Human reviews the exact-head ready PR"
		}
	case TransitionPassed:
		if latest.Stage == StageHumanReview {
			item.Status = "human_review"
			item.NextAction = "Human reviews the exact-head ready PR"
		} else if next, ok := NextStage(passedStages(transitions)); ok {
			item.Status = "accepted"
			item.TLCStage = next
			item.TLCIndex = StageIndex(next)
			item.Peers = PeersForStage(next)
			item.NextAction = "start " + string(next)
		}
	}
	item.GateState = deriveGateState(transitions)
	for _, transition := range transitions {
		for _, evidence := range transition.Evidence {
			if evidence.PR != nil {
				pr := evidence.PR
				item.PR = &PRProjection{Repository: pr.Repository, Number: pr.Number, URL: pr.URL, HeadSHA: pr.HeadSHA, ReviewedHeadSHA: pr.ReviewedHeadSHA, Open: pr.Open, Draft: pr.Draft, ChecksPassing: pr.ChecksPassing}
			}
		}
	}
	if latest.State == TransitionPassed && len(latest.Evidence) == 0 {
		item.Status = "blocked"
		item.TLCStage = latest.Stage
		item.TLCIndex = latest.StageIndex
		item.Peers = append([]string(nil), latest.Peers...)
		item.Blocker = "stage is marked passed without durable evidence"
		item.NextAction = "repair or invalidate the unsupported stage transition"
		item.GateState = "unavailable"
	}
}

func deriveGateState(transitions []StageTransitionPayload) string {
	gates := [...]Stage{StageIADA, StageCFADA, StageIAR, StageCFAR}
	latest := make(map[Stage]StageTransitionPayload, len(gates))
	for _, transition := range transitions {
		switch transition.Stage {
		case StageIADA, StageCFADA, StageIAR, StageCFAR:
			latest[transition.Stage] = transition
		}
	}

	// A later repaired attempt replaces an earlier blocked attempt. Among
	// current blocking outcomes, the furthest gate in TLC order determines the
	// truthful operator-facing state.
	for index := len(gates) - 1; index >= 0; index-- {
		transition, ok := latest[gates[index]]
		if ok && (transition.State == TransitionBlocked || transition.State == TransitionHumanRequired) {
			return string(transition.State)
		}
	}
	for _, gate := range gates {
		if transition, ok := latest[gate]; ok && transition.State == TransitionRunning {
			return "running"
		}
	}

	passed := 0
	for _, gate := range gates {
		transition, ok := latest[gate]
		if !ok || transition.State != TransitionPassed || validateStageEvidence(gate, transition.Evidence) != nil {
			continue
		}
		passed++
	}
	if passed == len(gates) {
		return "all_required_gates_passed"
	}
	if passed > 0 {
		return "current_gate_passed_later_gates_pending"
	}
	return "unavailable"
}

func orderTransitions(events []Event, orderID string) ([]StageTransitionPayload, []StageLedgerProjection, []Event, error) {
	var transitions []StageTransitionPayload
	var ledger []StageLedgerProjection
	var matched []Event
	for _, event := range events {
		if event.Type != EventStageTransitioned || event.OrderID != orderID {
			continue
		}
		payload, err := decodeEvent[StageTransitionPayload](event)
		if err != nil {
			return nil, nil, nil, err
		}
		transitions = append(transitions, payload)
		matched = append(matched, event)
		ledger = append(ledger, StageLedgerProjection{Stage: payload.Stage, Index: payload.StageIndex, State: payload.State, AttemptID: payload.AttemptID, Ordinal: payload.Ordinal, EventID: event.ID, OccurredAt: event.OccurredAt, Peers: append([]string(nil), payload.Peers...), Evidence: append([]Evidence(nil), payload.Evidence...), WorkArtifactID: payload.WorkArtifactID, Recovered: payload.Recovered})
	}
	return transitions, ledger, matched, nil
}

func deriveBudget(limit BudgetLimit, transitions []StageTransitionPayload) BudgetProjection {
	budget := BudgetProjection{MaxAttempts: limit.MaxAttempts, MaxTokens: limit.MaxTokens, MaxCostMicros: limit.MaxCostMicros}
	for _, transition := range transitions {
		if transition.State == TransitionRunning {
			continue
		}
		budget.ConsumedAttempts++
		budget.ConsumedTokens += transition.Usage.Tokens
		budget.ConsumedCostMicros += transition.Usage.CostMicros
	}
	budget.RemainingAttempts = max(0, budget.MaxAttempts-budget.ConsumedAttempts)
	budget.RemainingTokens = max(int64(0), budget.MaxTokens-budget.ConsumedTokens)
	budget.RemainingCostMicros = max(int64(0), budget.MaxCostMicros-budget.ConsumedCostMicros)
	budget.Exhausted = budget.RemainingAttempts == 0 || budget.RemainingTokens == 0 || budget.RemainingCostMicros == 0
	return budget
}

func projectIdeas(events []Event) ([]IdeaProjection, error) {
	byID := make(map[string][]IdeaRevision)
	submitted := make(map[string]bool)
	for _, event := range events {
		switch event.Type {
		case EventIdeaRecorded, EventIdeaRefined:
			payload, err := decodeEvent[IdeaRevisionPayload](event)
			if err != nil {
				return nil, err
			}
			byID[payload.IdeaID] = append(byID[payload.IdeaID], IdeaRevision{Revision: payload.Revision, Note: payload.Note, Candidate: payload.Candidate, CandidateSHA256: payload.CandidateSHA256, ValidationErrors: payload.ValidationErrors, EventID: event.ID, RecordedAt: event.OccurredAt})
		case EventOrderAccepted:
			payload, err := decodeEvent[OrderAcceptedPayload](event)
			if err != nil {
				return nil, err
			}
			const prefix = "human-idea:"
			if len(payload.SourceIdentity) > len(prefix) && payload.SourceIdentity[:len(prefix)] == prefix {
				submitted[payload.SourceIdentity[len(prefix):]] = true
			}
		}
	}
	result := make([]IdeaProjection, 0, len(byID))
	for ideaID, revisions := range byID {
		sort.Slice(revisions, func(i, j int) bool { return revisions[i].Revision < revisions[j].Revision })
		current := revisions[len(revisions)-1]
		status := "refining"
		if submitted[ideaID] {
			status = "submitted"
		}
		result = append(result, IdeaProjection{IdeaID: ideaID, Title: current.Candidate.Title, TargetRepository: current.Candidate.TargetRepository, Status: status, CurrentRevision: current.Revision, Revisions: revisions})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].IdeaID < result[j].IdeaID })
	return result, nil
}

func projectInterventions(events []Event) ([]InterventionProjection, error) {
	result := make(map[string]InterventionProjection)
	for _, event := range events {
		switch event.Type {
		case EventInterventionRequested:
			payload, err := decodeEvent[InterventionRequestedPayload](event)
			if err != nil {
				return nil, err
			}
			result[payload.InterventionID] = InterventionProjection{InterventionID: payload.InterventionID, OrderID: payload.OrderID, Kind: payload.Kind, Prompt: payload.Prompt, Status: InterventionOpen, RequestedAt: payload.RequestedAt, EventID: event.ID}
		case EventInterventionResolved:
			payload, err := decodeEvent[InterventionResolvedPayload](event)
			if err != nil {
				return nil, err
			}
			item := result[payload.InterventionID]
			item.Status = InterventionResolved
			result[payload.InterventionID] = item
		}
	}
	list := make([]InterventionProjection, 0, len(result))
	for _, item := range result {
		list = append(list, item)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Status != list[j].Status {
			return list[i].Status == InterventionOpen
		}
		return list[i].RequestedAt.Before(list[j].RequestedAt)
	})
	return list, nil
}

func hasOpenIntervention(events []Event, orderID string) bool {
	list, err := projectInterventions(events)
	if err != nil {
		return true
	}
	for _, item := range list {
		if item.OrderID == orderID && item.Status == InterventionOpen {
			return true
		}
	}
	return false
}

func (p Projection) Order(orderID string) (OrderProjection, error) {
	for _, order := range p.Orders {
		if order.OrderID == orderID {
			return order, nil
		}
	}
	return OrderProjection{}, fmt.Errorf("projection order %q not found", orderID)
}
