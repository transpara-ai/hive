package factoryv1

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const DefaultWorkerCount = 3

type RunRequest struct {
	Operation       string           `json:"operation"`
	Order           FactoryOrder     `json:"order"`
	OrderMarkdown   string           `json:"order_markdown"`
	DocumentSHA256  string           `json:"document_sha256"`
	Stage           Stage            `json:"stage"`
	AttemptID       string           `json:"attempt_id"`
	Ordinal         int              `json:"ordinal"`
	RepositoryRoot  string           `json:"repository_root"`
	PriorEvidence   []Evidence       `json:"prior_evidence"`
	AuthorityScope  AuthorityScope   `json:"authority_scope"`
	BudgetRemaining BudgetProjection `json:"budget_remaining"`
	Peers           []string         `json:"peers"`
	Provider        ProviderBinding  `json:"provider"`
}

type RunResult struct {
	Status     RunnerStatus    `json:"status"`
	Evidence   []Evidence      `json:"evidence"`
	Blocker    string          `json:"blocker,omitempty"`
	NextAction string          `json:"next_action,omitempty"`
	Usage      Usage           `json:"usage"`
	Provider   ProviderBinding `json:"provider"`
}

type ReconcileResult struct {
	EffectExists bool      `json:"effect_exists"`
	Conflict     bool      `json:"conflict"`
	Result       RunResult `json:"result"`
}

type Runner interface {
	Execute(ctx context.Context, request RunRequest) (RunResult, error)
	Reconcile(ctx context.Context, request RunRequest) (ReconcileResult, error)
}

type SchedulerConfig struct {
	WorkerCount      int
	RepositoryRoot   func(order FactoryOrder) string
	StageProviders   map[Stage]ProviderBinding
	AuthorFamily     string
	StandingApproval *StandingApprovalBinding
}

type Scheduler struct {
	store  Store
	work   WorkStore
	runner Runner
	clock  Clock
	config SchedulerConfig

	mu      sync.Mutex
	working map[string]struct{}
}

func NewScheduler(store Store, work WorkStore, runner Runner, clock Clock, config SchedulerConfig) (*Scheduler, error) {
	if store == nil || work == nil || runner == nil {
		return nil, errors.New("factory v1 scheduler requires EventGraph, Work, and runner boundaries")
	}
	if clock == nil {
		clock = WallClock{}
	}
	if config.WorkerCount == 0 {
		config.WorkerCount = DefaultWorkerCount
	}
	if config.WorkerCount < 1 || config.WorkerCount > 64 {
		return nil, errors.New("factory v1 worker count must be between 1 and 64")
	}
	if config.RepositoryRoot == nil {
		config.RepositoryRoot = func(order FactoryOrder) string { return order.TargetRepository }
	}
	if config.StageProviders == nil {
		return nil, errors.New("factory v1 scheduler requires explicit per-stage provider bindings")
	}
	if strings.TrimSpace(config.AuthorFamily) == "" {
		return nil, errors.New("factory v1 scheduler requires an explicit author family")
	}
	for _, stage := range TLCStages {
		binding, ok := config.StageProviders[stage]
		if !ok || errBinding(binding) != nil {
			return nil, fmt.Errorf("stage %s has no valid provider binding", stage)
		}
		if (stage == StageCFADA || stage == StageCFAR) && binding.Family == config.AuthorFamily {
			return nil, fmt.Errorf("stage %s reviewer family must differ from author family", stage)
		}
	}
	return &Scheduler{store: store, work: work, runner: runner, clock: clock, config: config, working: make(map[string]struct{})}, nil
}

// RunOnce admits up to WorkerCount non-waiting orders and waits for those
// workers. A worker advances its order until a bounded wait or Human Review.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	intake, err := NewIntake(s.store, s.work, s.clock)
	if err != nil {
		return err
	}
	if err := intake.ReplayAndRepair(ctx); err != nil {
		return err
	}
	events, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	records, err := acceptedRecords(events)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, s.config.WorkerCount)
	var wg sync.WaitGroup
	var errorsMu sync.Mutex
	var runErrors []error
	launched := 0
	for _, record := range records {
		if launched >= s.config.WorkerCount {
			break
		}
		if !s.scheduleable(events, record.Document.Order.DocID) || !s.claim(record.Document.Order.DocID) {
			continue
		}
		launched++
		wg.Add(1)
		sem <- struct{}{}
		go func(record acceptedRecord) {
			defer wg.Done()
			defer func() { <-sem; s.release(record.Document.Order.DocID) }()
			if workerErr := s.runOrder(ctx, record); workerErr != nil {
				errorsMu.Lock()
				runErrors = append(runErrors, fmt.Errorf("order %s: %w", record.Document.Order.DocID, workerErr))
				errorsMu.Unlock()
			}
		}(record)
	}
	wg.Wait()
	return errors.Join(runErrors...)
}

func (s *Scheduler) ActiveWorkers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.working)
}

func (s *Scheduler) claim(orderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.working[orderID]; exists {
		return false
	}
	s.working[orderID] = struct{}{}
	return true
}

func (s *Scheduler) release(orderID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.working, orderID)
}

func (s *Scheduler) scheduleable(events []Event, orderID string) bool {
	if orderAtHumanReview(events, orderID) || hasOpenIntervention(events, orderID) {
		return false
	}
	transitions, _, _, err := orderTransitions(events, orderID)
	if err != nil || len(transitions) == 0 {
		return err == nil
	}
	latest := transitions[len(transitions)-1]
	if latest.State == TransitionBlocked || latest.State == TransitionHumanRequired {
		return interventionResolvedAfter(events, orderID, latest.AttemptID)
	}
	return true
}

func interventionResolvedAfter(events []Event, orderID, attemptID string) bool {
	_, found := resolvedInterventionEvent(events, orderID, attemptID)
	return found
}

func resolvedInterventionEvent(events []Event, orderID, attemptID string) (Event, bool) {
	requests := make(map[string]Event)
	for _, event := range eventsByTime(events) {
		switch event.Type {
		case EventInterventionRequested:
			payload, err := decodeEvent[InterventionRequestedPayload](event)
			if err == nil && payload.OrderID == orderID && payload.AttemptID == attemptID {
				requests[payload.InterventionID] = event
			}
		case EventInterventionResolved:
			payload, err := decodeEvent[InterventionResolvedPayload](event)
			if err == nil {
				if _, exists := requests[payload.InterventionID]; exists {
					return event, true
				}
			}
		}
	}
	return Event{}, false
}

func (s *Scheduler) runOrder(ctx context.Context, record acceptedRecord) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		events, err := s.store.List(ctx)
		if err != nil {
			return err
		}
		transitions, _, transitionEvents, err := orderTransitions(events, record.Document.Order.DocID)
		if err != nil {
			return err
		}
		if orderAtHumanReview(events, record.Document.Order.DocID) {
			return nil
		}
		stage, ok := NextStage(passedStages(transitions))
		if !ok {
			return nil
		}
		budget := deriveBudget(record.Document.Order.Budget, transitions)
		if budget.Exhausted {
			return s.blockForBudget(ctx, record, stage, transitions)
		}
		latestRunning, interrupted := interruptedAttempt(transitions, stage)
		if interrupted {
			runningEvent, found := transitionEvent(transitionEvents, latestRunning.AttemptID, TransitionRunning)
			if !found {
				return errors.New("running transition payload has no durable event identity")
			}
			stop, err := s.reconcileAttempt(ctx, record, transitions, latestRunning, runningEvent, budget)
			if err != nil || stop {
				return err
			}
			continue
		}
		latest := latestForStage(transitions, stage)
		if latest != nil && (latest.State == TransitionBlocked || latest.State == TransitionHumanRequired) {
			if !interventionResolvedAfter(events, record.Document.Order.DocID, latest.AttemptID) {
				return nil
			}
		}
		ordinal := nextOrdinal(transitions, stage)
		attemptID, err := AttemptID(record.Document.SHA256, stage, ordinal)
		if err != nil {
			return err
		}
		provider := s.config.StageProviders[stage]
		running := StageTransitionPayload{
			TLCVersion: TLCVersion, Stage: stage, StageIndex: StageIndex(stage), State: TransitionRunning,
			AttemptID: attemptID, Ordinal: ordinal, Peers: PeersForStage(stage), Runner: provider,
		}
		if err := ValidateTransitionForDocument(record.Document.SHA256, transitions, running); err != nil {
			return err
		}
		causes := []string{record.Event.ID}
		if len(transitionEvents) != 0 {
			causes = []string{transitionEvents[len(transitionEvents)-1].ID}
		}
		if latest != nil && (latest.State == TransitionBlocked || latest.State == TransitionHumanRequired) {
			if resolution, found := resolvedInterventionEvent(events, record.Document.Order.DocID, latest.AttemptID); found {
				causes = append(causes, resolution.ID)
			}
		}
		runningEvent, err := AppendTyped(ctx, s.store, EventStageTransitioned, record.Document.Order.DocID,
			"stage-running:"+record.Document.Order.DocID+":"+attemptID, causes, running)
		if err != nil {
			return err
		}
		request := s.runRequest("execute", record, running, budget, priorEvidence(transitions))
		result, err := s.runner.Execute(ctx, request)
		if err != nil {
			return fmt.Errorf("runner execute: %w", err)
		}
		stop, err := s.finishAttempt(ctx, record, transitions, running, runningEvent, result, false)
		if err != nil || stop {
			return err
		}
	}
}

func (s *Scheduler) reconcileAttempt(ctx context.Context, record acceptedRecord, transitions []StageTransitionPayload, running StageTransitionPayload, runningEvent Event, budget BudgetProjection) (bool, error) {
	request := s.runRequest("reconcile", record, running, budget, priorEvidence(transitions))
	reconciled, err := s.runner.Reconcile(ctx, request)
	if err != nil {
		return true, fmt.Errorf("runner reconcile: %w", err)
	}
	observation, priorRecovery, err := s.nextRecoveryObservation(ctx, record.Document.Order.DocID, running.AttemptID)
	if err != nil {
		return true, err
	}
	recovery := RecoveryPayload{
		OrderID: record.Document.Order.DocID, Stage: running.Stage, AttemptID: running.AttemptID,
		Observation: observation,
		EffectFound: reconciled.EffectExists, Conflict: reconciled.Conflict, Evidence: reconciled.Result.Evidence,
		RecoveredFrom: "running_without_terminal", Result: reconciled.Result.Status,
	}
	recoveryCauses := []string{runningEvent.ID}
	if priorRecovery.ID != "" {
		recoveryCauses = []string{priorRecovery.ID}
	}
	recoveryEvent, err := AppendTyped(ctx, s.store, EventRecoveryRecorded, record.Document.Order.DocID,
		fmt.Sprintf("recovery:%s:%s:%d", record.Document.Order.DocID, running.AttemptID, observation), recoveryCauses, recovery)
	if err != nil {
		return true, err
	}
	if reconciled.Conflict {
		result := reconciled.Result
		result.Status = RunnerBlocked
		if result.Blocker == "" {
			result.Blocker = "reconciliation found conflicting external state"
		}
		if len(result.Evidence) == 0 {
			result.Evidence = []Evidence{{Kind: "reconciliation_conflict", Reference: recoveryEvent.ID}}
		}
		return s.finishAttempt(ctx, record, transitions, running, recoveryEvent, result, true)
	}
	if reconciled.EffectExists {
		return s.finishAttempt(ctx, record, transitions, running, recoveryEvent, reconciled.Result, true)
	}
	// The exact same attempt may execute only after reconciliation proves the
	// effect absent. No second running transition is appended.
	result, err := s.runner.Execute(ctx, requestWithOperation(request, "execute"))
	if err != nil {
		return true, fmt.Errorf("runner execute after reconcile: %w", err)
	}
	return s.finishAttempt(ctx, record, transitions, running, recoveryEvent, result, true)
}

func (s *Scheduler) nextRecoveryObservation(ctx context.Context, orderID, attemptID string) (int, Event, error) {
	events, err := s.store.List(ctx)
	if err != nil {
		return 0, Event{}, err
	}
	count := 0
	var latest Event
	for _, event := range eventsByTime(events) {
		if event.Type != EventRecoveryRecorded || event.OrderID != orderID {
			continue
		}
		payload, decodeErr := decodeEvent[RecoveryPayload](event)
		if decodeErr != nil {
			return 0, Event{}, decodeErr
		}
		if payload.AttemptID == attemptID {
			count++
			latest = event
		}
	}
	return count + 1, latest, nil
}

func requestWithOperation(request RunRequest, operation string) RunRequest {
	request.Operation = operation
	return request
}

func (s *Scheduler) finishAttempt(ctx context.Context, record acceptedRecord, transitions []StageTransitionPayload, running StageTransitionPayload, cause Event, result RunResult, recovered bool) (bool, error) {
	if !result.Status.valid() {
		return true, fmt.Errorf("runner returned status outside allowlist: %q", result.Status)
	}
	if !reflect.DeepEqual(result.Provider, running.Runner) {
		return true, errors.New("runner provider output does not match configured stage binding")
	}
	if len(result.Evidence) == 0 {
		return true, errors.New("runner terminal result has no durable evidence")
	}
	if result.Status == RunnerBlocked && strings.TrimSpace(result.Blocker) == "" {
		return true, errors.New("blocked runner result must name its blocker")
	}
	if result.Status == RunnerHumanRequired && strings.TrimSpace(result.NextAction) == "" {
		return true, errors.New("human_required runner result must name the bounded Human action")
	}
	if err := s.validateResult(ctx, record, running.Stage, result); err != nil {
		return true, err
	}
	state := TransitionPassed
	switch result.Status {
	case RunnerBlocked:
		state = TransitionBlocked
	case RunnerHumanRequired:
		state = TransitionHumanRequired
	}
	terminal := StageTransitionPayload{
		TLCVersion: TLCVersion, Stage: running.Stage, StageIndex: running.StageIndex, State: state,
		AttemptID: running.AttemptID, Ordinal: running.Ordinal, Peers: append([]string(nil), running.Peers...),
		Evidence: append([]Evidence(nil), result.Evidence...), Blocker: result.Blocker, NextAction: result.NextAction,
		Usage: result.Usage, Runner: running.Runner, Recovered: recovered,
	}
	terminal.WorkArtifactID = "work-stage-" + HashText(record.Document.Order.DocID + "\x00" + string(running.Stage) + "\x00" + running.AttemptID)[:24]
	if err := ValidateTransitionForDocument(record.Document.SHA256, transitions, terminal); err != nil {
		return true, err
	}
	event, err := AppendTyped(ctx, s.store, EventStageTransitioned, record.Document.Order.DocID,
		"stage-terminal:"+record.Document.Order.DocID+":"+running.AttemptID, []string{cause.ID}, terminal)
	if err != nil {
		return true, err
	}
	_, err = s.work.AttachStageArtifact(ctx, WorkArtifact{ArtifactID: terminal.WorkArtifactID, OrderID: record.Document.Order.DocID, Stage: running.Stage, AttemptID: running.AttemptID, StageEventID: event.ID, Evidence: result.Evidence})
	if err != nil {
		return true, fmt.Errorf("stage event %s committed before Work artifact failed: %w", event.ID, err)
	}
	if state == TransitionBlocked || (state == TransitionHumanRequired && running.Stage != StageHumanReview) {
		kind := "stage_blocked"
		if state == TransitionHumanRequired {
			kind = "human_required"
		}
		prompt := result.NextAction
		if prompt == "" {
			prompt = "Resolve the bounded stage wait and resume the same order."
		}
		intake, intakeErr := NewIntake(s.store, s.work, s.clock)
		if intakeErr != nil {
			return true, intakeErr
		}
		if _, err := intake.requestIntervention(ctx, record.Document.Order.DocID, running.Stage, kind, prompt, running.AttemptID, []string{event.ID}); err != nil {
			return true, err
		}
		return true, nil
	}
	return state == TransitionHumanRequired, nil
}

func (s *Scheduler) validateResult(ctx context.Context, record acceptedRecord, stage Stage, result RunResult) error {
	if result.Usage.Tokens < 0 || result.Usage.CostMicros < 0 {
		return errors.New("runner usage cannot be negative")
	}
	if result.Status != RunnerPassed {
		return nil
	}
	if stage == StageHumanReview {
		return errors.New("Human Review is a waiting boundary and cannot be runner-asserted as passed")
	}
	if err := validateStageEvidence(stage, result.Evidence); err != nil {
		return err
	}
	if stage == StageIngestWork || stage == StageCraftFactoryOrder {
		link, err := s.work.GetFactoryOrder(ctx, record.Document.Order.DocID, record.Document.Order.Version)
		if err != nil || link.DocumentSHA256 != record.Document.SHA256 || link.AcceptedEventID != record.Event.ID {
			return errors.New("ingest/canonical stage predicate lacks exact Work linkage")
		}
	}
	if stage == StageHumanDesignReview {
		for _, item := range result.Evidence {
			if item.Approval != nil {
				if err := ValidateApprovalReceipt(record.Document, *item.Approval); err != nil {
					return err
				}
				if item.Approval.Basis == ApprovalStandingScoped {
					if s.config.StandingApproval == nil || !standingReceiptMatches(*s.config.StandingApproval, *item.Approval) {
						return errors.New("standing approval receipt does not match the configured exact source and FactoryOrder binding")
					}
				}
				return nil
			}
		}
		return errors.New("Human Design Review requires an exact scoped approval receipt")
	}
	if stage == StageCFADA || stage == StageCFAR {
		for _, item := range result.Evidence {
			if item.AuthorFamily == s.config.AuthorFamily && item.ReviewerFamily == result.Provider.Family && item.AuthorFamily != item.ReviewerFamily && item.Provider != nil && reflect.DeepEqual(*item.Provider, result.Provider) {
				return nil
			}
		}
		return errors.New("cross-family evidence does not match configured author and reviewer lineage")
	}
	return nil
}

func standingReceiptMatches(binding StandingApprovalBinding, receipt HumanApprovalReceipt) bool {
	return binding.ActorID == receipt.ActorID && binding.CredentialKeyID == receipt.CredentialKeyID &&
		binding.SourceSHA256 == receipt.SourceSHA256 && binding.FactoryOrderBlobSHA == receipt.FactoryOrderBlobSHA &&
		binding.ApprovalSentence == receipt.ApprovalSentence && binding.ApprovalSourceEventID == receipt.ApprovalSourceEventID
}

func (s *Scheduler) runRequest(operation string, record acceptedRecord, running StageTransitionPayload, budget BudgetProjection, prior []Evidence) RunRequest {
	return RunRequest{
		Operation: operation, Order: record.Document.Order, OrderMarkdown: record.Document.Markdown,
		DocumentSHA256: record.Document.SHA256, Stage: running.Stage, AttemptID: running.AttemptID,
		Ordinal: running.Ordinal, RepositoryRoot: s.config.RepositoryRoot(record.Document.Order),
		PriorEvidence: append([]Evidence(nil), prior...), AuthorityScope: record.Document.Order.Authority,
		BudgetRemaining: budget, Peers: append([]string(nil), running.Peers...), Provider: running.Runner,
	}
}

func (s *Scheduler) blockForBudget(ctx context.Context, record acceptedRecord, stage Stage, transitions []StageTransitionPayload) error {
	ordinal := nextOrdinal(transitions, stage)
	attemptID, err := AttemptID(record.Document.SHA256, stage, ordinal)
	if err != nil {
		return err
	}
	intake, err := NewIntake(s.store, s.work, s.clock)
	if err != nil {
		return err
	}
	_, err = intake.requestIntervention(ctx, record.Document.Order.DocID, stage, "budget_exhausted", "Approve a bounded budget change or terminate the order.", attemptID, nil)
	return err
}

func acceptedRecords(events []Event) ([]acceptedRecord, error) {
	var records []acceptedRecord
	for _, event := range eventsByTime(events) {
		if event.Type != EventOrderAccepted {
			continue
		}
		payload, err := decodeEvent[OrderAcceptedPayload](event)
		if err != nil {
			return nil, err
		}
		records = append(records, acceptedRecord{Event: event, Payload: payload, Document: payload.Document})
	}
	return records, nil
}

func nextOrdinal(transitions []StageTransitionPayload, stage Stage) int {
	maxOrdinal := 0
	for _, transition := range transitions {
		if transition.Stage == stage && transition.Ordinal > maxOrdinal {
			maxOrdinal = transition.Ordinal
		}
	}
	return maxOrdinal + 1
}

func latestForStage(transitions []StageTransitionPayload, stage Stage) *StageTransitionPayload {
	for i := len(transitions) - 1; i >= 0; i-- {
		if transitions[i].Stage == stage {
			copy := transitions[i]
			return &copy
		}
	}
	return nil
}

func interruptedAttempt(transitions []StageTransitionPayload, stage Stage) (StageTransitionPayload, bool) {
	for i := len(transitions) - 1; i >= 0; i-- {
		transition := transitions[i]
		if transition.Stage != stage {
			continue
		}
		if transition.State != TransitionRunning {
			return StageTransitionPayload{}, false
		}
		for _, later := range transitions[i+1:] {
			if later.Stage == stage && later.AttemptID == transition.AttemptID && later.State != TransitionRunning {
				return StageTransitionPayload{}, false
			}
		}
		return transition, true
	}
	return StageTransitionPayload{}, false
}

func transitionEvent(events []Event, attemptID string, state TransitionState) (Event, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		payload, err := decodeEvent[StageTransitionPayload](events[i])
		if err == nil && payload.AttemptID == attemptID && payload.State == state {
			return events[i], true
		}
	}
	return Event{}, false
}

func priorEvidence(transitions []StageTransitionPayload) []Evidence {
	var result []Evidence
	for _, transition := range transitions {
		result = append(result, transition.Evidence...)
	}
	return result
}

func errBinding(binding ProviderBinding) error {
	if strings.TrimSpace(binding.ProviderID) == "" || strings.TrimSpace(binding.Family) == "" || strings.TrimSpace(binding.ExecutableRealpath) == "" || !hexPattern.MatchString(binding.ExecutableSHA256) || strings.TrimSpace(binding.ModelID) == "" || strings.TrimSpace(binding.CredentialSourceID) == "" {
		return errors.New("provider binding fields are required")
	}
	return nil
}
