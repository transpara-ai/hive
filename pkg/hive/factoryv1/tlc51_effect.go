package factoryv1

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TLC51EffectRequestSchema  = "tlc-effect-request/v1"
	TLC51EffectDecisionSchema = "tlc-effect-decision/v1"
	TLC51EffectReceiptSchema  = "factory-tlc51-effect-receipt/v1"
)

var ErrTLC51ProtectedEffectHumanRequired = errors.New("factory-tlc51/v1 protected effect requires Human intervention")

type TLC51EffectOperation struct {
	Effect         string `json:"effect"`
	OperationID    string `json:"operation_id"`
	IdempotencyKey string `json:"idempotency_key"`
	ReceiptDigest  string `json:"receipt_digest"`
	AttemptOrdinal uint32 `json:"attempt_ordinal"`
}

type TLC51EffectObservation struct {
	ExternalState     TLC51ExternalState `json:"external_state"`
	ObservationID     string             `json:"observation_id"`
	ObservationSHA256 string             `json:"observation_sha256"`
	ObservedAt        time.Time          `json:"observed_at"`
	BoundaryRequest   json.RawMessage    `json:"boundary_request"`
	EffectReceipt     *TLC51ExactJSON    `json:"effect_receipt,omitempty"`
}

type TLC51EffectBoundaryClient interface {
	CheckEffect(context.Context, json.RawMessage) (json.RawMessage, error)
}

type TLC51EffectDriver interface {
	ObserveEffect(context.Context, TLC51EffectOperation) (TLC51EffectObservation, error)
	ExecuteEffect(context.Context, TLC51EffectOperation) (TLC51ExactJSON, error)
}

type tlc51EffectDecision struct {
	SchemaVersion              string            `json:"schema_version"`
	Decision                   string            `json:"decision"`
	ProviderObservationID      string            `json:"provider_observation_id"`
	ProviderObservationDigest  string            `json:"provider_observation_digest"`
	AuthorityObservationID     string            `json:"authority_observation_id"`
	AuthorityObservationDigest string            `json:"authority_observation_digest"`
	ReservationTuple           map[string]string `json:"reservation_tuple"`
	ReservationDigest          string            `json:"reservation_digest"`
	EffectInvoked              bool              `json:"effect_invoked"`
	AuthorityGranted           []json.RawMessage `json:"authority_granted"`
	DecisionDigest             string            `json:"decision_digest"`
}

type tlc51EffectReceipt struct {
	SchemaVersion     string `json:"schema_version"`
	Effect            string `json:"effect"`
	SubjectDigest     string `json:"subject_digest"`
	GateReceiptDigest string `json:"gate_receipt_digest"`
	OperationID       string `json:"operation_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	AttemptOrdinal    uint32 `json:"attempt_ordinal"`
	ProviderEffectID  string `json:"provider_effect_id"`
	ProviderRecordURL string `json:"provider_record_url"`
	InvokedAt         string `json:"invoked_at"`
	Outcome           string `json:"outcome"`
	ReceiptDigest     string `json:"receipt_digest"`
}

func validateTLC51EffectReceipt(value TLC51ExactJSON, plan TLC51GatePlan, operation TLC51EffectOperation) (time.Time, error) {
	if value.SchemaVersion != TLC51EffectReceiptSchema || !validTLC51SHA(value.SHA256) || fmt.Sprintf("%x", sha256.Sum256([]byte(value.CanonicalJSON))) != value.SHA256 {
		return time.Time{}, errors.New("effect receipt exact JSON wrapper is invalid")
	}
	decoded, err := decodeTLC51CanonicalObject([]byte(value.CanonicalJSON), TLC51EffectReceiptSchema)
	if err != nil {
		return time.Time{}, err
	}
	if err := requireExactTLC51Fields(decoded,
		"schema_version", "effect", "subject_digest", "gate_receipt_digest", "operation_id",
		"idempotency_key", "attempt_ordinal", "provider_effect_id", "provider_record_url",
		"invoked_at", "outcome", "receipt_digest",
	); err != nil {
		return time.Time{}, fmt.Errorf("closed effect receipt object: %w", err)
	}
	expectedDigest, err := tlc51ObjectDigest(decoded, "receipt_digest")
	if err != nil {
		return time.Time{}, err
	}
	raw, err := json.Marshal(decoded)
	if err != nil {
		return time.Time{}, err
	}
	var receipt tlc51EffectReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return time.Time{}, err
	}
	if receipt.Effect != operation.Effect || receipt.SubjectDigest != plan.SubjectDigest || receipt.GateReceiptDigest != operation.ReceiptDigest || receipt.OperationID != operation.OperationID || receipt.IdempotencyKey != operation.IdempotencyKey || receipt.AttemptOrdinal != operation.AttemptOrdinal {
		return time.Time{}, errors.New("effect receipt does not bind the exact plan and operation tuple")
	}
	if strings.TrimSpace(receipt.ProviderEffectID) == "" || strings.TrimSpace(receipt.ProviderRecordURL) == "" || receipt.Outcome != "succeeded" || !validTLC51SHA(receipt.ReceiptDigest) || receipt.ReceiptDigest != expectedDigest {
		return time.Time{}, errors.New("effect receipt provider identity, outcome, or digest is invalid")
	}
	invokedAt, err := time.Parse(time.RFC3339, receipt.InvokedAt)
	if err != nil || invokedAt.Location() != time.UTC || !strings.HasSuffix(receipt.InvokedAt, "Z") {
		return time.Time{}, errors.New("effect receipt requires an explicit UTC invocation time")
	}
	return invokedAt, nil
}

func parseTLC51EffectDecision(raw []byte, plan TLC51GatePlan, operation TLC51EffectOperation, observation TLC51EffectObservation) (tlc51EffectDecision, error) {
	value, err := decodeTLC51CanonicalObject(raw, TLC51EffectDecisionSchema)
	if err != nil {
		return tlc51EffectDecision{}, err
	}
	expected, err := tlc51ObjectDigest(value, "decision_digest")
	if err != nil {
		return tlc51EffectDecision{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return tlc51EffectDecision{}, err
	}
	var decision tlc51EffectDecision
	if err := json.Unmarshal(encoded, &decision); err != nil {
		return tlc51EffectDecision{}, err
	}
	if decision.Decision != "pass" || decision.EffectInvoked || len(decision.AuthorityGranted) != 0 || decision.DecisionDigest != expected {
		return tlc51EffectDecision{}, errors.New("TLC effect boundary did not return a pure pass decision")
	}
	wanted := map[string]string{
		"receipt_digest": operation.ReceiptDigest, "effect": operation.Effect,
		"subject_digest": plan.SubjectDigest, "operation_id": operation.OperationID,
		"idempotency_key": operation.IdempotencyKey,
	}
	for key, value := range wanted {
		if decision.ReservationTuple[key] != value {
			return tlc51EffectDecision{}, fmt.Errorf("effect decision reservation tuple mismatches %s", key)
		}
	}
	tupleRaw, err := canonicalTLC51JSON(decision.ReservationTuple)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(tupleRaw)) != decision.ReservationDigest {
		return tlc51EffectDecision{}, errors.New("effect decision reservation digest mismatch")
	}
	if decision.ProviderObservationID != observation.ObservationID || decision.ProviderObservationDigest != observation.ObservationSHA256 {
		return tlc51EffectDecision{}, errors.New("effect decision does not bind the current provider observation")
	}
	if !validTLC51SHA(decision.ProviderObservationDigest) || !validTLC51SHA(decision.AuthorityObservationDigest) || !validTLC51SHA(decision.ReservationDigest) || !validTLC51SHA(decision.DecisionDigest) {
		return tlc51EffectDecision{}, errors.New("effect decision contains invalid digests")
	}
	return decision, nil
}

func validateTLC51EffectOperation(operation TLC51EffectOperation) error {
	if strings.TrimSpace(operation.Effect) == "" || strings.TrimSpace(operation.OperationID) == "" || strings.TrimSpace(operation.IdempotencyKey) == "" || !validTLC51SHA(operation.ReceiptDigest) || operation.AttemptOrdinal == 0 {
		return errors.New("effect operation requires effect, operation, idempotency, receipt, and attempt identities")
	}
	return nil
}

func validateTLC51EffectOperationForPlan(plan TLC51GatePlan, operation TLC51EffectOperation) error {
	if err := validateTLC51EffectOperation(operation); err != nil {
		return err
	}
	for _, effect := range append(append([]string(nil), plan.DerivedEffects...), plan.RequestedEffects...) {
		if effect == operation.Effect {
			return nil
		}
	}
	return fmt.Errorf("effect %q is not derived or requested by the exact TLC plan", operation.Effect)
}

func requireTLC51RecordedPassingDecision(history []TLC51HistoryEntry, plan TLC51GatePlan, operation TLC51EffectOperation) error {
	matchedIndex := -1
	for index := len(history) - 1; index >= 0; index-- {
		entry := history[index]
		if entry.Type != TLC51DecisionRecorded || entry.Identity.PlanDigest != plan.PlanDigest || entry.Identity.SubjectDigest != plan.SubjectDigest {
			continue
		}
		var payload struct {
			Receipt  TLC51ExactJSON `json:"receipt"`
			Decision string         `json:"decision"`
		}
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return err
		}
		if payload.Receipt.SchemaVersion != TLC51ReceiptSchema || !validTLC51SHA(payload.Receipt.SHA256) || fmt.Sprintf("%x", sha256.Sum256([]byte(payload.Receipt.CanonicalJSON))) != payload.Receipt.SHA256 {
			return errors.New("recorded TLC decision receipt wrapper is invalid")
		}
		parsed, err := ParseTLC51GateReceipt([]byte(payload.Receipt.CanonicalJSON), plan)
		if err != nil {
			return err
		}
		if parsed.ReceiptDigest != operation.ReceiptDigest {
			continue
		}
		if payload.Decision != "pass" || parsed.Decision != "pass" {
			return errors.New("recorded TLC decision is not pass")
		}
		var receipt struct {
			PredicateResults []struct {
				Status string `json:"status"`
			} `json:"predicate_results"`
			AuthorityReferences []struct {
				Effect       string `json:"effect"`
				RecordDigest string `json:"record_digest"`
			} `json:"authority_references"`
		}
		if err := json.Unmarshal(parsed.Raw, &receipt); err != nil {
			return err
		}
		if len(receipt.PredicateResults) == 0 {
			return errors.New("recorded TLC decision has no predicate results")
		}
		for _, predicate := range receipt.PredicateResults {
			if predicate.Status != "true" {
				return errors.New("recorded TLC decision contains a non-passing predicate")
			}
		}
		authorityFound := false
		for _, authority := range receipt.AuthorityReferences {
			if authority.Effect == operation.Effect && validTLC51SHA(authority.RecordDigest) {
				authorityFound = true
			}
		}
		if !authorityFound {
			return errors.New("recorded TLC decision lacks exact-effect authority evidence")
		}
		matchedIndex = index
		break
	}
	if matchedIndex < 0 {
		return errors.New("exact passing TLC decision receipt is not recorded")
	}
	for _, entry := range history[matchedIndex+1:] {
		if entry.Type == TLC51DecisionInvalidated && entry.Identity.PlanDigest == plan.PlanDigest {
			return errors.New("recorded TLC decision was invalidated")
		}
	}
	return nil
}

func validateTLC51EffectObservation(observation TLC51EffectObservation) error {
	if observation.ExternalState != TLC51ExternalAbsent && observation.ExternalState != TLC51ExternalExact && observation.ExternalState != TLC51ExternalConflict && observation.ExternalState != TLC51ExternalUnknown {
		return errors.New("invalid external effect observation state")
	}
	if strings.TrimSpace(observation.ObservationID) == "" || !validTLC51SHA(observation.ObservationSHA256) || observation.ObservedAt.IsZero() || observation.ObservedAt.Location() != time.UTC || !json.Valid(observation.BoundaryRequest) {
		return errors.New("effect observation requires explicit authenticated identity, digest, time, and boundary request")
	}
	var request struct {
		SchemaVersion  string             `json:"schema_version"`
		Effect         string             `json:"effect"`
		OperationID    string             `json:"operation_id"`
		IdempotencyKey string             `json:"idempotency_key"`
		ExternalState  TLC51ExternalState `json:"external_effect_state"`
	}
	if err := json.Unmarshal(observation.BoundaryRequest, &request); err != nil || request.SchemaVersion != TLC51EffectRequestSchema || request.ExternalState != observation.ExternalState {
		return errors.New("effect observation boundary request identity mismatch")
	}
	return nil
}

// ExecuteProtectedEffect enforces observe-before-invoke and observe-before-
// retry. Every authorization decision is fresh and operation-bound. A crash
// after proposal leaves durable intent, causing the next call to observe the
// provider before it can invoke the same operation again.
func (scheduler *TLC51Scheduler) ExecuteProtectedEffect(
	ctx context.Context,
	binding TLC51OrderBinding,
	plan TLC51GatePlan,
	operation TLC51EffectOperation,
	boundary TLC51EffectBoundaryClient,
	driver TLC51EffectDriver,
) (TLC51ExactJSON, error) {
	if boundary == nil || driver == nil {
		return TLC51ExactJSON{}, errors.New("protected effect requires boundary evaluator and provider driver")
	}
	if err := validateTLC51EffectOperationForPlan(plan, operation); err != nil {
		return TLC51ExactJSON{}, err
	}
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return TLC51ExactJSON{}, err
	}
	if err := scheduler.reconcileWork(ctx, history); err != nil {
		return TLC51ExactJSON{}, err
	}
	if !tlc51PlanIsLatest(history, plan.PlanDigest) {
		return TLC51ExactJSON{}, errors.New("protected effect plan is not the latest recorded plan")
	}
	if err := validateTLC51EffectReservationHistory(history, plan, operation); err != nil {
		return TLC51ExactJSON{}, err
	}
	if terminal, found, err := tlc51EffectTerminal(history, plan, operation); err != nil {
		return TLC51ExactJSON{}, err
	} else if found {
		return terminal, nil
	}
	proposed := tlc51EffectWasProposed(history, operation)
	observation, err := driver.ObserveEffect(ctx, operation)
	if err != nil {
		return TLC51ExactJSON{}, fmt.Errorf("observe protected effect: %w", err)
	}
	if err := validateTLC51EffectObservation(observation); err != nil {
		return TLC51ExactJSON{}, err
	}
	if err := validateTLC51EffectObservationOperation(observation, operation); err != nil {
		return TLC51ExactJSON{}, err
	}
	if err := scheduler.recordEffectObservation(ctx, binding, plan, operation, observation); err != nil {
		return TLC51ExactJSON{}, err
	}
	if observation.ExternalState == TLC51ExternalExact {
		if !proposed {
			return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "external effect exists without matching durable proposal")
		}
		if err := scheduler.recordEffectReconciliation(ctx, binding, plan, operation, observation.ExternalState, "settle"); err != nil {
			return TLC51ExactJSON{}, err
		}
		if observation.EffectReceipt == nil {
			return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "exact external effect lacks a provider effect receipt")
		}
		invokedAt, err := validateTLC51EffectReceipt(*observation.EffectReceipt, plan, operation)
		if err != nil {
			return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "provider effect receipt is invalid")
		}
		if observation.ObservedAt.Before(invokedAt) {
			return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "provider observation predates the effect receipt")
		}
		if err := scheduler.recordEffectTerminal(ctx, binding, plan, operation, *observation.EffectReceipt, "succeeded", "recovered exact provider effect without retry"); err != nil {
			return TLC51ExactJSON{}, err
		}
		return *observation.EffectReceipt, nil
	}
	if observation.ExternalState != TLC51ExternalAbsent {
		if err := scheduler.recordEffectReconciliation(ctx, binding, plan, operation, observation.ExternalState, "block"); err != nil {
			return TLC51ExactJSON{}, err
		}
		return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "external effect state is "+string(observation.ExternalState))
	}
	if err := requireTLC51RecordedPassingDecision(history, plan, operation); err != nil {
		return TLC51ExactJSON{}, err
	}
	decisionRaw, err := boundary.CheckEffect(ctx, observation.BoundaryRequest)
	if err != nil {
		return TLC51ExactJSON{}, fmt.Errorf("TLC effect boundary: %w", err)
	}
	if _, err := parseTLC51EffectDecision(decisionRaw, plan, operation, observation); err != nil {
		return TLC51ExactJSON{}, err
	}
	if proposed {
		if err := scheduler.recordEffectReconciliation(ctx, binding, plan, operation, TLC51ExternalAbsent, "retry"); err != nil {
			return TLC51ExactJSON{}, err
		}
	} else if err := scheduler.recordEffectProposal(ctx, binding, plan, operation); err != nil {
		return TLC51ExactJSON{}, err
	}
	receipt, err := driver.ExecuteEffect(ctx, operation)
	if err != nil {
		// Durable proposal intentionally remains for observe-before-retry.
		return TLC51ExactJSON{}, fmt.Errorf("protected effect execution left durable proposal: %w", err)
	}
	invokedAt, err := validateTLC51EffectReceipt(receipt, plan, operation)
	if err != nil {
		return TLC51ExactJSON{}, err
	}
	post, err := driver.ObserveEffect(ctx, operation)
	if err != nil {
		return TLC51ExactJSON{}, fmt.Errorf("post-effect observation: %w", err)
	}
	if err := validateTLC51EffectObservation(post); err != nil {
		return TLC51ExactJSON{}, err
	}
	if err := validateTLC51EffectObservationOperation(post, operation); err != nil {
		return TLC51ExactJSON{}, err
	}
	if post.ObservedAt.Before(invokedAt) {
		return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "post-effect observation predates the effect receipt")
	}
	if err := scheduler.recordEffectObservation(ctx, binding, plan, operation, post); err != nil {
		return TLC51ExactJSON{}, err
	}
	if post.ExternalState != TLC51ExternalExact {
		if err := scheduler.recordEffectReconciliation(ctx, binding, plan, operation, post.ExternalState, "block"); err != nil {
			return TLC51ExactJSON{}, err
		}
		return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "post-effect state is not exact")
	}
	if err := scheduler.recordEffectTerminal(ctx, binding, plan, operation, receipt, "succeeded", "provider effect observed exactly"); err != nil {
		return TLC51ExactJSON{}, err
	}
	return receipt, nil
}

func validateTLC51EffectObservationOperation(observation TLC51EffectObservation, operation TLC51EffectOperation) error {
	var request struct {
		Effect         string `json:"effect"`
		OperationID    string `json:"operation_id"`
		IdempotencyKey string `json:"idempotency_key"`
		Receipt        struct {
			ReceiptDigest string `json:"receipt_digest"`
		} `json:"receipt"`
		ProviderObservation struct {
			RecordID     string `json:"record_id"`
			RecordDigest string `json:"record_digest"`
		} `json:"provider_observation"`
	}
	if err := json.Unmarshal(observation.BoundaryRequest, &request); err != nil {
		return err
	}
	if request.Effect != operation.Effect || request.OperationID != operation.OperationID || request.IdempotencyKey != operation.IdempotencyKey || request.Receipt.ReceiptDigest != operation.ReceiptDigest {
		return errors.New("effect observation cannot be reused across operations")
	}
	if request.ProviderObservation.RecordID != observation.ObservationID || request.ProviderObservation.RecordDigest != observation.ObservationSHA256 {
		return errors.New("effect observation wrapper differs from the boundary provider observation")
	}
	return nil
}

func (scheduler *TLC51Scheduler) recordEffectObservation(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, operation TLC51EffectOperation, observation TLC51EffectObservation) error {
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, operation.AttemptOrdinal)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"effect": operation.Effect, "operation_id": operation.OperationID, "external_state": observation.ExternalState,
		"provider_observation_id": observation.ObservationID, "provider_observation_digest": observation.ObservationSHA256,
		"observed_at": observation.ObservedAt,
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51EffectObserved, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func (scheduler *TLC51Scheduler) recordEffectProposal(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, operation TLC51EffectOperation) error {
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, operation.AttemptOrdinal)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"effect": operation.Effect, "operation_id": operation.OperationID, "idempotency_key": operation.IdempotencyKey,
		"receipt_digest": operation.ReceiptDigest, "proposed_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51EffectProposed, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func (scheduler *TLC51Scheduler) recordEffectReconciliation(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, operation TLC51EffectOperation, state TLC51ExternalState, action string) error {
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, operation.AttemptOrdinal)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"effect": operation.Effect, "operation_id": operation.OperationID, "external_state": state,
		"action": action, "reconciled_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51EffectReconciled, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func (scheduler *TLC51Scheduler) recordEffectTerminal(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, operation TLC51EffectOperation, receipt TLC51ExactJSON, outcome, reason string) error {
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, operation.AttemptOrdinal)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"effect": operation.Effect, "operation_id": operation.OperationID,
		"idempotency_key": operation.IdempotencyKey, "gate_receipt_digest": operation.ReceiptDigest,
		"outcome": outcome, "effect_receipt": receipt, "reason": reason,
		"terminal_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51EffectTerminal, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	return err
}

func (scheduler *TLC51Scheduler) blockEffectForHuman(ctx context.Context, binding TLC51OrderBinding, plan TLC51GatePlan, operation TLC51EffectOperation, reason string) error {
	history, err := scheduler.journal.TLC51History(ctx, binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		return err
	}
	identity := scheduler.nextIdentity(binding.FactoryOrderID, plan, history, 0)
	payload, err := NewTLC51EventPayload(identity, map[string]any{
		"request_id": "human-" + HashText(operation.OperationID + "\x00" + reason)[:32], "boundary": "protected-effect:" + operation.OperationID,
		"reason": reason, "requested_at": scheduler.clock.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = scheduler.append(ctx, TLC51Append{Type: TLC51HumanRequested, Identity: identity, Payload: payload, OccurredAt: scheduler.clock.Now().UTC()})
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: %s", ErrTLC51ProtectedEffectHumanRequired, reason)
}

func tlc51EffectWasProposed(history []TLC51HistoryEntry, operation TLC51EffectOperation) bool {
	for _, entry := range history {
		if entry.Type != TLC51EffectProposed {
			continue
		}
		var payload struct {
			Effect         string `json:"effect"`
			OperationID    string `json:"operation_id"`
			IdempotencyKey string `json:"idempotency_key"`
			ReceiptDigest  string `json:"receipt_digest"`
		}
		if json.Unmarshal(entry.Payload, &payload) == nil && payload.Effect == operation.Effect && payload.OperationID == operation.OperationID && payload.IdempotencyKey == operation.IdempotencyKey && payload.ReceiptDigest == operation.ReceiptDigest {
			return true
		}
	}
	return false
}

// validateTLC51EffectReservationHistory enforces the durable reservation tuple
// before any provider observation or effect invocation. Receipt, operation,
// and idempotency identities are each single-use within a change series unless
// all five reservation fields identify the same retry.
func validateTLC51EffectReservationHistory(history []TLC51HistoryEntry, plan TLC51GatePlan, operation TLC51EffectOperation) error {
	type reservation struct {
		effect         string
		subjectDigest  string
		operationID    string
		idempotencyKey string
		receiptDigest  string
	}
	wanted := reservation{
		effect: operation.Effect, subjectDigest: plan.SubjectDigest,
		operationID: operation.OperationID, idempotencyKey: operation.IdempotencyKey,
		receiptDigest: operation.ReceiptDigest,
	}
	for _, entry := range history {
		if entry.Type != TLC51EffectProposed && entry.Type != TLC51EffectTerminal {
			continue
		}
		var payload struct {
			Effect            string `json:"effect"`
			OperationID       string `json:"operation_id"`
			IdempotencyKey    string `json:"idempotency_key"`
			ReceiptDigest     string `json:"receipt_digest"`
			GateReceiptDigest string `json:"gate_receipt_digest"`
		}
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return fmt.Errorf("decode protected-effect reservation history: %w", err)
		}
		receiptDigest := payload.ReceiptDigest
		if entry.Type == TLC51EffectTerminal {
			receiptDigest = payload.GateReceiptDigest
		}
		observed := reservation{
			effect: payload.Effect, subjectDigest: entry.Identity.SubjectDigest,
			operationID: payload.OperationID, idempotencyKey: payload.IdempotencyKey,
			receiptDigest: receiptDigest,
		}
		same := observed == wanted
		identityCollision := observed.receiptDigest == wanted.receiptDigest ||
			observed.operationID == wanted.operationID ||
			observed.idempotencyKey == wanted.idempotencyKey
		if identityCollision && !same {
			return errors.New("protected-effect reservation identity is already bound; tuple conflicts with a different operation")
		}
	}
	return nil
}

func tlc51EffectTerminal(history []TLC51HistoryEntry, plan TLC51GatePlan, operation TLC51EffectOperation) (TLC51ExactJSON, bool, error) {
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].Type != TLC51EffectTerminal {
			continue
		}
		var payload struct {
			Effect            string         `json:"effect"`
			OperationID       string         `json:"operation_id"`
			IdempotencyKey    string         `json:"idempotency_key"`
			GateReceiptDigest string         `json:"gate_receipt_digest"`
			Outcome           string         `json:"outcome"`
			EffectReceipt     TLC51ExactJSON `json:"effect_receipt"`
		}
		if err := json.Unmarshal(history[index].Payload, &payload); err != nil {
			return TLC51ExactJSON{}, false, err
		}
		if payload.Effect == operation.Effect && payload.OperationID == operation.OperationID {
			if history[index].Identity.SubjectDigest != plan.SubjectDigest || history[index].Identity.AttemptOrdinal != operation.AttemptOrdinal || payload.IdempotencyKey != operation.IdempotencyKey || payload.GateReceiptDigest != operation.ReceiptDigest {
				return TLC51ExactJSON{}, false, errors.New("terminal effect operation tuple conflicts with the requested operation")
			}
			if payload.Outcome != "succeeded" {
				return TLC51ExactJSON{}, false, errors.New("terminal effect operation did not succeed")
			}
			if _, err := validateTLC51EffectReceipt(payload.EffectReceipt, plan, operation); err != nil {
				return TLC51ExactJSON{}, false, err
			}
			return payload.EffectReceipt, true, nil
		}
	}
	return TLC51ExactJSON{}, false, nil
}

func TLC51ExactJSONDigest(value TLC51ExactJSON) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value.CanonicalJSON)))
}
