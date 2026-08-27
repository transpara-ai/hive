package factoryv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type TLC51OrderBinding struct {
	Order           FactoryOrder `json:"order"`
	FactoryOrderID  string       `json:"factory_order_id"`
	AcceptedEventID string       `json:"accepted_event_id"`
}

func NewTLC51OrderBinding(order FactoryOrder, acceptedEventID string) (TLC51OrderBinding, error) {
	if err := ValidateFactoryOrder(order); err != nil {
		return TLC51OrderBinding{}, err
	}
	if strings.TrimSpace(acceptedEventID) == "" {
		return TLC51OrderBinding{}, errors.New("TLC 5.1 order binding requires accepted EventGraph event id")
	}
	return TLC51OrderBinding{Order: order, FactoryOrderID: TLC51FactoryOrderID(order.DocID, order.Version), AcceptedEventID: acceptedEventID}, nil
}

type TLC51ObligationOutcome string

const (
	TLC51ObligationPassed    TLC51ObligationOutcome = "passed"
	TLC51ObligationFailed    TLC51ObligationOutcome = "failed"
	TLC51ObligationBlocked   TLC51ObligationOutcome = "blocked"
	TLC51ObligationCancelled TLC51ObligationOutcome = "cancelled"
	TLC51ObligationUnknown   TLC51ObligationOutcome = "unknown"
)

func (outcome TLC51ObligationOutcome) valid() bool {
	switch outcome {
	case TLC51ObligationPassed, TLC51ObligationFailed, TLC51ObligationBlocked, TLC51ObligationCancelled, TLC51ObligationUnknown:
		return true
	default:
		return false
	}
}

type TLC51ObligationRequest struct {
	Operation       string          `json:"operation"`
	Order           FactoryOrder    `json:"order"`
	FactoryOrderID  string          `json:"factory_order_id"`
	ChangeSeriesID  string          `json:"change_series_id"`
	PlanDigest      string          `json:"plan_digest"`
	SubjectDigest   string          `json:"subject_digest"`
	Obligation      TLC51Obligation `json:"obligation"`
	AttemptOrdinal  uint32          `json:"attempt_ordinal"`
	Provider        ProviderBinding `json:"provider"`
	BudgetRemaining TLC51Budget     `json:"budget_remaining"`
}

type TLC51ObligationResult struct {
	Outcome          TLC51ObligationOutcome `json:"outcome"`
	Reason           string                 `json:"reason"`
	EvidenceRecordID string                 `json:"evidence_record_id,omitempty"`
	EvidenceRecord   *TLC51ExactJSON        `json:"evidence_record,omitempty"`
	Usage            Usage                  `json:"usage"`
	Provider         ProviderBinding        `json:"provider"`
}

type TLC51ExternalState string

const (
	TLC51ExternalAbsent   TLC51ExternalState = "absent"
	TLC51ExternalExact    TLC51ExternalState = "exact"
	TLC51ExternalConflict TLC51ExternalState = "conflict"
	TLC51ExternalUnknown  TLC51ExternalState = "unknown"
)

type TLC51ReconcileResult struct {
	ExternalState     TLC51ExternalState    `json:"external_state"`
	ObservationID     string                `json:"observation_id"`
	ObservationSHA256 string                `json:"observation_sha256"`
	ObservedAt        time.Time             `json:"observed_at"`
	Result            TLC51ObligationResult `json:"result"`
}

type TLC51ObligationRunner interface {
	Execute(context.Context, TLC51ObligationRequest) (TLC51ObligationResult, error)
	Reconcile(context.Context, TLC51ObligationRequest) (TLC51ReconcileResult, error)
}

type TLC51Budget struct {
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

type TLC51SchedulerConfig struct {
	WorkerCount int
	Providers   map[string]ProviderBinding
}

type TLC51Scheduler struct {
	journal  TLC51Journal
	work     TLC51WorkLinker
	runner   TLC51ObligationRunner
	clock    Clock
	config   TLC51SchedulerConfig
	appendMu sync.Mutex
}

func NewTLC51Scheduler(journal TLC51Journal, work TLC51WorkLinker, runner TLC51ObligationRunner, clock Clock, config TLC51SchedulerConfig) (*TLC51Scheduler, error) {
	if journal == nil || work == nil || runner == nil {
		return nil, errors.New("TLC 5.1 scheduler requires EventGraph journal, Work linker, and runner")
	}
	if clock == nil {
		clock = WallClock{}
	}
	if config.WorkerCount == 0 {
		config.WorkerCount = DefaultWorkerCount
	}
	if config.WorkerCount < 1 || config.WorkerCount > 64 {
		return nil, errors.New("TLC 5.1 worker count must be between 1 and 64")
	}
	if len(config.Providers) == 0 {
		return nil, errors.New("TLC 5.1 scheduler requires explicit per-obligation providers")
	}
	for obligationID, provider := range config.Providers {
		if strings.TrimSpace(obligationID) == "" || errBinding(provider) != nil {
			return nil, fmt.Errorf("invalid TLC 5.1 provider binding for %q", obligationID)
		}
	}
	return &TLC51Scheduler{journal: journal, work: work, runner: runner, clock: clock, config: config}, nil
}

// RecordPlan writes the trusted exact plan before any obligation can be
// materialized. Replans first supersede the prior plan and may not lower the
// retained floor observed in the same change series.
func (scheduler *TLC51Scheduler) RecordPlan(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan) ([]TLC51HistoryEntry, error) {
	if plan.Repository != binding.Order.TargetRepository {
		return nil, errors.New("TLC 5.1 plan repository does not match FactoryOrder target")
	}
	if plan.InformationState == TLC51Classified && plan.Track == nil {
		return nil, errors.New("classified TLC 5.1 plan has no track")
	}
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return nil, err
	}
	if err := scheduler.reconcileWork(ctx, history); err != nil {
		return nil, err
	}
	var prior *TLC51GatePlan
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].Type != TLC51PlanRecorded {
			continue
		}
		var payload struct {
			Plan TLC51ExactJSON `json:"plan"`
		}
		if err := json.Unmarshal(history[index].Payload, &payload); err != nil {
			return nil, err
		}
		parsed, err := ParseTLC51GatePlan([]byte(payload.Plan.CanonicalJSON))
		if err != nil {
			return nil, err
		}
		prior = &parsed
		break
	}
	if prior != nil && prior.PlanDigest == plan.PlanDigest {
		return history, nil
	}
	if prior != nil {
		if tlc51TrackRank(prior.RetainedFloor) > tlc51TrackRank(plan.RetainedFloor) {
			return nil, errors.New("TLC 5.1 replan lowered the retained floor")
		}
		identity := scheduler.nextIdentity(binding.FactoryOrderID, *prior, history, 0)
		payload, err := NewTLC51EventPayload(identity, map[string]any{
			"superseding_plan_digest": plan.PlanDigest,
			"reason":                  "trusted TLC replan for the same change series",
			"superseded_at":           scheduler.clock.Now().UTC(),
		})
		if err != nil {
			return nil, err
		}
		entry, err := scheduler.append(ctx, TLC51Append{Type: TLC51PlanSuperseded, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
		if err != nil {
			return nil, err
		}
		history = append(history, entry)
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, 0)
	exact := TLC51ExactJSON{SchemaVersion: TLC51PlanSchema, CanonicalJSON: string(plan.Raw), SHA256: fmt.Sprintf("%x", sha256.Sum256(plan.Raw))}
	payload, err := NewTLC51EventPayload(identity, map[string]any{"plan": exact, "recorded_at": scheduler.clock.Now().UTC()})
	if err != nil {
		return nil, err
	}
	causes := []string(nil)
	if len(history) == 0 {
		causes = []string{binding.AcceptedEventID}
	}
	entry, err := scheduler.append(ctx, TLC51Append{Type: TLC51PlanRecorded, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC(), Causes: causes})
	if err != nil {
		return nil, err
	}
	return append(history, entry), nil
}

func tlc51TrackRank(value *string) int {
	if value == nil {
		return 0
	}
	switch *value {
	case "M":
		return 1
	case "I":
		return 2
	case "D":
		return 3
	case "H":
		return 4
	default:
		return 5
	}
}

func (scheduler *TLC51Scheduler) nextIdentity(factoryOrderID string, plan TLC51GatePlan, history []TLC51HistoryEntry, attempt uint32) TLC51EventIdentity {
	return TLC51EventIdentity{
		ProtocolVersion: TLC51ProtocolVersion, FactoryOrderID: factoryOrderID, ChangeSeriesID: plan.ChangeSeriesID,
		PlanDigest: plan.PlanDigest, SubjectDigest: plan.SubjectDigest,
		EventOrdinal: uint64(len(history) + 1), AttemptOrdinal: attempt,
	}
}

func (scheduler *TLC51Scheduler) append(ctx context.Context, input TLC51Append) (TLC51HistoryEntry, error) {
	scheduler.appendMu.Lock()
	defer scheduler.appendMu.Unlock()
	// Concurrent obligation results must take the next durable ordinal rather
	// than using the ordinal observed before another worker committed.
	history, err := scheduler.journal.TLC51History(ctx, input.Identity.FactoryOrderID, input.Identity.ChangeSeriesID)
	if err != nil {
		return TLC51HistoryEntry{}, err
	}
	input.Identity.EventOrdinal = uint64(len(history) + 1)
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(input.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return TLC51HistoryEntry{}, err
	}
	for _, key := range []string{"protocol_version", "factory_order_id", "change_series_id", "plan_digest", "subject_digest", "event_ordinal", "attempt_ordinal"} {
		delete(fields, key)
	}
	input.Payload, err = NewTLC51EventPayload(input.Identity, fields)
	if err != nil {
		return TLC51HistoryEntry{}, err
	}
	entry, err := scheduler.journal.AppendTLC51(ctx, input)
	if err != nil {
		return TLC51HistoryEntry{}, err
	}
	if err := scheduler.work.LinkTLC51Event(ctx, entry); err != nil {
		return TLC51HistoryEntry{}, fmt.Errorf("EventGraph event %s committed before Work twin: %w", entry.EventID, err)
	}
	return entry, nil
}

func (scheduler *TLC51Scheduler) reconcileWork(ctx context.Context, history []TLC51HistoryEntry) error {
	if len(history) == 0 {
		return nil
	}
	workArtifacts, err := scheduler.work.TLC51WorkArtifacts(ctx, history[0].Identity.FactoryOrderID, history[0].Identity.ChangeSeriesID)
	if err != nil {
		return err
	}
	byOrdinal := make(map[uint64]TLC51WorkArtifact, len(workArtifacts))
	for _, artifact := range workArtifacts {
		if existing, duplicate := byOrdinal[artifact.EventOrdinal]; duplicate && !tlc51WorkArtifactsEqual(existing, artifact) {
			return fmt.Errorf("%w: conflicting Work twins at ordinal %d", ErrTLC51HistoryConflict, artifact.EventOrdinal)
		}
		byOrdinal[artifact.EventOrdinal] = artifact
	}
	for _, entry := range history {
		wanted := TLC51WorkArtifactFromEntry(entry)
		if existing, ok := byOrdinal[wanted.EventOrdinal]; ok {
			if !tlc51WorkArtifactsEqual(existing, wanted) {
				return fmt.Errorf("%w: EventGraph/Work split at ordinal %d", ErrTLC51HistoryConflict, wanted.EventOrdinal)
			}
			continue
		}
		if err := scheduler.work.LinkTLC51Event(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func tlc51WorkArtifactsEqual(left, right TLC51WorkArtifact) bool {
	return left.FactoryOrderID == right.FactoryOrderID && left.ChangeSeriesID == right.ChangeSeriesID && left.EventOrdinal == right.EventOrdinal && left.EventType == right.EventType && left.PayloadSHA256 == right.PayloadSHA256 && bytes.Equal(left.Payload, right.Payload)
}

// RunOnce materializes and executes only currently ready, non-Human
// obligations. It returns after one bounded wave so daemon policy controls
// pacing and budgets.
func (scheduler *TLC51Scheduler) RunOnce(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan) error {
	if plan.InformationState != TLC51Classified {
		return fmt.Errorf("TLC 5.1 plan is %s; no obligation execution admitted", plan.InformationState)
	}
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	if err := scheduler.reconcileWork(ctx, history); err != nil {
		return err
	}
	if !tlc51PlanIsLatest(history, plan.PlanDigest) {
		return errors.New("TLC 5.1 plan is not the latest recorded plan")
	}
	states, err := projectTLC51Obligations(plan, history)
	if err != nil {
		return err
	}
	budget := deriveTLC51Budget(binding.Order.Budget, history)
	if budget.Exhausted {
		return scheduler.requestBudgetHuman(ctx, binding, plan, history)
	}
	var recoveries []tlc51Runnable
	var ready []tlc51Runnable
	for _, obligation := range plan.Obligations {
		state := states[obligation.ID]
		if state.Terminal != "" || state.HumanWaiting {
			continue
		}
		if state.RunningAttempt > 0 {
			provider, err := scheduler.providerFor(plan, obligation)
			if err != nil {
				return err
			}
			recoveries = append(recoveries, tlc51Runnable{Obligation: obligation, Attempt: state.RunningAttempt, Provider: provider, Reconcile: true})
			continue
		}
		if !tlc51PrerequisitesPassed(obligation, states) {
			continue
		}
		attempt := state.MaxAttempt + 1
		if tlc51HumanObligation(obligation.Kind) {
			if err := scheduler.requestObligationHuman(ctx, binding, plan, obligation, attempt); err != nil {
				return err
			}
			continue
		}
		provider, err := scheduler.providerFor(plan, obligation)
		if err != nil {
			return err
		}
		ready = append(ready, tlc51Runnable{Obligation: obligation, Attempt: attempt, Provider: provider})
	}
	runnable := append(recoveries, ready...)
	if len(runnable) > scheduler.config.WorkerCount {
		runnable = runnable[:scheduler.config.WorkerCount]
	}
	var wait sync.WaitGroup
	var errorsMu sync.Mutex
	var runErrors []error
	for _, item := range runnable {
		item := item
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := scheduler.runObligation(ctx, binding, plan, item, budget); err != nil {
				errorsMu.Lock()
				runErrors = append(runErrors, fmt.Errorf("obligation %s: %w", item.Obligation.ID, err))
				errorsMu.Unlock()
			}
		}()
	}
	wait.Wait()
	return errors.Join(runErrors...)
}

type tlc51ObligationState struct {
	Terminal       TLC51ObligationOutcome
	MaxAttempt     uint32
	RunningAttempt uint32
	HumanWaiting   bool
}

type tlc51Runnable struct {
	Obligation TLC51Obligation
	Attempt    uint32
	Provider   ProviderBinding
	Reconcile  bool
}

func projectTLC51Obligations(plan TLC51GatePlan, history []TLC51HistoryEntry) (map[string]tlc51ObligationState, error) {
	states := make(map[string]tlc51ObligationState, len(plan.Obligations))
	humanObligationByRequest := make(map[string]string)
	for _, obligation := range plan.Obligations {
		states[obligation.ID] = tlc51ObligationState{}
	}
	for _, entry := range history {
		if entry.Identity.PlanDigest != plan.PlanDigest {
			continue
		}
		var payload struct {
			ObligationID string                 `json:"obligation_id"`
			Outcome      TLC51ObligationOutcome `json:"outcome"`
			Boundary     string                 `json:"boundary"`
			RequestID    string                 `json:"request_id"`
		}
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return nil, err
		}
		obligationID := payload.ObligationID
		if entry.Type == TLC51HumanResolved && obligationID == "" {
			obligationID = humanObligationByRequest[payload.RequestID]
		}
		state, exists := states[obligationID]
		if !exists {
			continue
		}
		if entry.Identity.AttemptOrdinal > state.MaxAttempt {
			state.MaxAttempt = entry.Identity.AttemptOrdinal
		}
		switch entry.Type {
		case TLC51ObligationRunning:
			state.RunningAttempt = entry.Identity.AttemptOrdinal
		case TLC51ObligationTerminal:
			state.Terminal = payload.Outcome
			state.RunningAttempt = 0
		case TLC51HumanRequested:
			if payload.Boundary == "obligation:"+payload.ObligationID {
				state.HumanWaiting = true
				humanObligationByRequest[payload.RequestID] = payload.ObligationID
			}
		case TLC51HumanResolved:
			state.HumanWaiting = false
		}
		states[obligationID] = state
	}
	return states, nil
}

func tlc51PrerequisitesPassed(obligation TLC51Obligation, states map[string]tlc51ObligationState) bool {
	for _, prerequisite := range obligation.Prerequisites {
		if states[prerequisite].Terminal != TLC51ObligationPassed {
			return false
		}
	}
	return true
}

func tlc51HumanObligation(kind string) bool {
	return kind == "human-design-review" || kind == "human-review" || kind == "effect-authority"
}

func (scheduler *TLC51Scheduler) providerFor(plan TLC51GatePlan, obligation TLC51Obligation) (ProviderBinding, error) {
	provider, ok := scheduler.config.Providers[obligation.ID]
	if !ok || errBinding(provider) != nil {
		return ProviderBinding{}, fmt.Errorf("obligation %s has no explicit provider binding", obligation.ID)
	}
	// An empty admission list means TLC left provider selection to the trusted
	// per-obligation adapter configuration. A non-empty list is an additional
	// closed allow-list and is enforced exactly.
	if len(obligation.AdmittedActorFamilies) > 0 && !contains(obligation.AdmittedActorFamilies, provider.Family) {
		return ProviderBinding{}, fmt.Errorf("provider family %q is not admitted for %s", provider.Family, obligation.ID)
	}
	if obligation.Kind == "cfada" || obligation.Kind == "cfar" || obligation.Kind == "domain-specialist-review" {
		if contains(plan.AuthorLineages, provider.Family) {
			return ProviderBinding{}, fmt.Errorf("independent provider family %q is an author lineage", provider.Family)
		}
	}
	return provider, nil
}

func (scheduler *TLC51Scheduler) runObligation(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, item tlc51Runnable, budget TLC51Budget) error {
	request := TLC51ObligationRequest{
		Operation: "execute", Order: binding.Order, FactoryOrderID: binding.FactoryOrderID,
		ChangeSeriesID: plan.ChangeSeriesID, PlanDigest: plan.PlanDigest, SubjectDigest: plan.SubjectDigest,
		Obligation: item.Obligation, AttemptOrdinal: item.Attempt, Provider: item.Provider, BudgetRemaining: budget,
	}
	if item.Reconcile {
		request.Operation = "reconcile"
		observation, err := scheduler.runner.Reconcile(ctx, request)
		if err != nil {
			return fmt.Errorf("reconcile before retry: %w", err)
		}
		if observation.ObservedAt.IsZero() || observation.ObservedAt.Location() != time.UTC || observation.ObservationID == "" || !validTLC51SHA(observation.ObservationSHA256) {
			return errors.New("reconciliation lacks an explicit authenticated observation")
		}
		switch observation.ExternalState {
		case TLC51ExternalExact:
			return scheduler.finishObligation(ctx, binding, plan, item, observation.Result)
		case TLC51ExternalAbsent:
			request.Operation = "execute"
		case TLC51ExternalConflict, TLC51ExternalUnknown:
			result := observation.Result
			result.Outcome = TLC51ObligationBlocked
			if strings.TrimSpace(result.Reason) == "" {
				result.Reason = "external effect reconciliation is " + string(observation.ExternalState)
			}
			if err := scheduler.finishObligation(ctx, binding, plan, item, result); err != nil {
				return err
			}
			return scheduler.requestObligationHuman(ctx, binding, plan, item.Obligation, item.Attempt+1)
		default:
			return fmt.Errorf("invalid reconciliation external state %q", observation.ExternalState)
		}
	} else {
		if err := scheduler.startObligation(ctx, binding, plan, item); err != nil {
			return err
		}
	}
	result, err := scheduler.runner.Execute(ctx, request)
	if err != nil {
		// A durable running event deliberately remains. Restart must reconcile
		// the same attempt before Execute can be called again.
		return fmt.Errorf("execute left durable running attempt: %w", err)
	}
	return scheduler.finishObligation(ctx, binding, plan, item, result)
}

func (scheduler *TLC51Scheduler) startObligation(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, item tlc51Runnable) error {
	providerRaw, err := json.Marshal(item.Provider)
	if err != nil {
		return err
	}
	providerDigest := fmt.Sprintf("%x", sha256.Sum256(providerRaw))
	events := []struct {
		kind   TLC51EventType
		fields map[string]any
	}{
		{TLC51ObligationReady, map[string]any{"obligation_id": item.Obligation.ID, "ready_at": scheduler.clock.Now().UTC()}},
		{TLC51ObligationClaimed, map[string]any{"obligation_id": item.Obligation.ID, "provider_binding_id": item.Provider.ProviderID, "provider_binding_sha256": providerDigest, "claimed_at": scheduler.clock.Now().UTC()}},
		{TLC51ObligationRunning, map[string]any{"obligation_id": item.Obligation.ID, "invocation_id": tlc51InvocationID(binding.FactoryOrderID, plan.PlanDigest, item.Obligation.ID, item.Attempt), "running_at": scheduler.clock.Now().UTC()}},
	}
	for _, spec := range events {
		history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
		if err != nil {
			return err
		}
		identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, item.Attempt)
		payload, err := NewTLC51EventPayload(identity, spec.fields)
		if err != nil {
			return err
		}
		if _, err := scheduler.append(ctx, TLC51Append{Type: spec.kind, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()}); err != nil {
			return err
		}
	}
	return nil
}

func tlc51InvocationID(factoryOrderID, planDigest, obligationID string, attempt uint32) string {
	return "invoke-" + HashText(fmt.Sprintf("%s\x00%s\x00%s\x00%d", factoryOrderID, planDigest, obligationID, attempt))[:32]
}

func (scheduler *TLC51Scheduler) finishObligation(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, item tlc51Runnable, result TLC51ObligationResult) error {
	if !result.Outcome.valid() || strings.TrimSpace(result.Reason) == "" || result.Usage.Tokens < 0 || result.Usage.CostMicros < 0 || !reflect.DeepEqual(result.Provider, item.Provider) {
		return errors.New("invalid TLC 5.1 obligation terminal result")
	}
	if result.Outcome == TLC51ObligationPassed {
		if result.EvidenceRecord == nil || strings.TrimSpace(result.EvidenceRecordID) == "" {
			return errors.New("passed TLC 5.1 obligation requires exact evidence")
		}
		if err := ValidateTLC51ExactJSON(*result.EvidenceRecord, TLC51RecordSchema); err != nil {
			return err
		}
		history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
		if err != nil {
			return err
		}
		identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, item.Attempt)
		payload, err := NewTLC51EventPayload(identity, map[string]any{
			"obligation_id": resultEvidenceObligation(item.Obligation.ID), "evidence_record_id": result.EvidenceRecordID,
			"evidence_record": *result.EvidenceRecord, "linked_at": scheduler.clock.Now().UTC(),
		})
		if err != nil {
			return err
		}
		if _, err := scheduler.append(ctx, TLC51Append{Type: TLC51EvidenceLinked, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()}); err != nil {
			return err
		}
	}
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, item.Attempt)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"obligation_id": item.Obligation.ID, "outcome": result.Outcome, "reason": result.Reason,
		"usage": result.Usage, "terminal_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51ObligationTerminal, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func resultEvidenceObligation(obligationID string) string { return obligationID }

func ValidateTLC51ExactJSON(value TLC51ExactJSON, schema string) error {
	raw := []byte(value.CanonicalJSON)
	decoded, err := decodeTLC51CanonicalObject(raw, schema)
	if err != nil {
		return err
	}
	if value.SchemaVersion != schema || !validTLC51SHA(value.SHA256) || fmt.Sprintf("%x", sha256.Sum256(raw)) != value.SHA256 {
		return errors.New("exact TLC 5.1 JSON identity or digest mismatch")
	}
	_ = decoded
	return nil
}

func (scheduler *TLC51Scheduler) requestObligationHuman(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, obligation TLC51Obligation, attempt uint32) error {
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	state, err := projectTLC51Obligations(plan, history)
	if err != nil {
		return err
	}
	if state[obligation.ID].HumanWaiting {
		return nil
	}
	readyIdentity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, attempt)
	readyPayload, err := NewTLC51EventPayload(readyIdentity, map[string]any{
		"obligation_id": obligation.ID, "ready_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if _, err := scheduler.append(ctx, TLC51Append{Type: TLC51ObligationReady, Identity: readyIdentity, Payload: readyPayload, OccurredAt: scheduler.clock.Now().UTC()}); err != nil {
		return err
	}
	history, err = scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, 0)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"request_id": "human-" + HashText(binding.FactoryOrderID + "\x00" + plan.PlanDigest + "\x00" + obligation.ID)[:32],
		"boundary":   "obligation:" + obligation.ID, "obligation_id": obligation.ID,
		"reason":       "adapter-authenticated Human record required; runner credit forbidden",
		"requested_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51HumanRequested, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func (scheduler *TLC51Scheduler) requestBudgetHuman(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, history []TLC51HistoryEntry) error {
	for _, entry := range history {
		if entry.Type == TLC51HumanRequested && entry.Identity.PlanDigest == plan.PlanDigest {
			var payload struct {
				Boundary string `json:"boundary"`
			}
			if json.Unmarshal(entry.Payload, &payload) == nil && payload.Boundary == "budget" {
				return nil
			}
		}
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, 0)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"request_id": "human-" + HashText(binding.FactoryOrderID + "\x00" + plan.PlanDigest + "\x00budget")[:32],
		"boundary":   "budget", "reason": "TLC 5.1 order budget exhausted", "requested_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51HumanRequested, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func deriveTLC51Budget(limit BudgetLimit, history []TLC51HistoryEntry) TLC51Budget {
	result := TLC51Budget{MaxAttempts: limit.MaxAttempts, MaxTokens: limit.MaxTokens, MaxCostMicros: limit.MaxCostMicros}
	for _, entry := range history {
		if entry.Type == TLC51ObligationRunning {
			result.ConsumedAttempts++
		}
		if entry.Type == TLC51ObligationTerminal {
			var payload struct {
				Usage Usage `json:"usage"`
			}
			if json.Unmarshal(entry.Payload, &payload) == nil {
				result.ConsumedTokens += payload.Usage.Tokens
				result.ConsumedCostMicros += payload.Usage.CostMicros
			}
		}
	}
	result.RemainingAttempts = max(0, result.MaxAttempts-result.ConsumedAttempts)
	result.RemainingTokens = max(int64(0), result.MaxTokens-result.ConsumedTokens)
	result.RemainingCostMicros = max(int64(0), result.MaxCostMicros-result.ConsumedCostMicros)
	result.Exhausted = result.RemainingAttempts == 0 || result.RemainingTokens == 0 || result.RemainingCostMicros == 0
	return result
}

func tlc51PlanIsLatest(history []TLC51HistoryEntry, planDigest string) bool {
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].Type == TLC51PlanRecorded {
			return history[index].Identity.PlanDigest == planDigest
		}
	}
	return false
}

// RecordDecision persists the exact pure evaluator result. It invokes no
// protected effect and its receipt is not authority by itself.
func (scheduler *TLC51Scheduler) RecordDecision(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, receipt TLC51GateReceipt) (TLC51HistoryEntry, error) {
	parsed, err := ParseTLC51GateReceipt(receipt.Raw, plan)
	if err != nil {
		return TLC51HistoryEntry{}, err
	}
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return TLC51HistoryEntry{}, err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, 0)
	exact := TLC51ExactJSON{SchemaVersion: TLC51ReceiptSchema, CanonicalJSON: string(parsed.Raw), SHA256: fmt.Sprintf("%x", sha256.Sum256(parsed.Raw))}
	payload, err := NewTLC51EventPayload(identity, map[string]any{"receipt": exact, "decision": parsed.Decision, "recorded_at": scheduler.clock.Now().UTC()})
	if err != nil {
		return TLC51HistoryEntry{}, err
	}
	return scheduler.append(ctx, TLC51Append{Type: TLC51DecisionRecorded, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
}

func sortedTLC51ObligationIDs(plan TLC51GatePlan) []string {
	result := make([]string, 0, len(plan.Obligations))
	for _, obligation := range plan.Obligations {
		result = append(result, obligation.ID)
	}
	sort.Strings(result)
	return result
}
