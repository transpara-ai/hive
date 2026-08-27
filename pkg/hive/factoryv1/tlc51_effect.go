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

func parseTLC51EffectDecision(raw []byte, operation TLC51EffectOperation, observation TLC51EffectObservation) (tlc51EffectDecision, error) {
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
		"operation_id": operation.OperationID, "idempotency_key": operation.IdempotencyKey,
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
	if err := validateTLC51EffectOperation(operation); err != nil {
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
	if terminal, found, err := tlc51EffectTerminal(history, operation); err != nil {
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
		if err := ValidateTLC51ExactJSON(*observation.EffectReceipt, TLC51EffectReceiptSchema); err != nil {
			return TLC51ExactJSON{}, scheduler.blockEffectForHuman(ctx, binding, plan, operation, "provider effect receipt is invalid")
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
	decisionRaw, err := boundary.CheckEffect(ctx, observation.BoundaryRequest)
	if err != nil {
		return TLC51ExactJSON{}, fmt.Errorf("TLC effect boundary: %w", err)
	}
	if _, err := parseTLC51EffectDecision(decisionRaw, operation, observation); err != nil {
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
	if err := ValidateTLC51ExactJSON(receipt, TLC51EffectReceiptSchema); err != nil {
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
	}
	if err := json.Unmarshal(observation.BoundaryRequest, &request); err != nil {
		return err
	}
	if request.Effect != operation.Effect || request.OperationID != operation.OperationID || request.IdempotencyKey != operation.IdempotencyKey || request.Receipt.ReceiptDigest != operation.ReceiptDigest {
		return errors.New("effect observation cannot be reused across operations")
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
		"effect": operation.Effect, "operation_id": operation.OperationID, "outcome": outcome,
		"effect_receipt": receipt, "reason": reason, "terminal_at": scheduler.clock.Now().UTC(),
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
	return err
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

func tlc51EffectTerminal(history []TLC51HistoryEntry, operation TLC51EffectOperation) (TLC51ExactJSON, bool, error) {
	for index := len(history) - 1; index >= 0; index-- {
		if history[index].Type != TLC51EffectTerminal {
			continue
		}
		var payload struct {
			Effect        string         `json:"effect"`
			OperationID   string         `json:"operation_id"`
			Outcome       string         `json:"outcome"`
			EffectReceipt TLC51ExactJSON `json:"effect_receipt"`
		}
		if err := json.Unmarshal(history[index].Payload, &payload); err != nil {
			return TLC51ExactJSON{}, false, err
		}
		if payload.Effect == operation.Effect && payload.OperationID == operation.OperationID && payload.Outcome == "succeeded" {
			if err := ValidateTLC51ExactJSON(payload.EffectReceipt, TLC51EffectReceiptSchema); err != nil {
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
