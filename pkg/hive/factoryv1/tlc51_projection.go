package factoryv1

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type TLC51MissionControlOrder struct {
	SchemaVersion        string                      `json:"schema_version"`
	ProtocolVersion      string                      `json:"protocol_version"`
	FactoryOrderID       string                      `json:"factory_order_id"`
	OrderID              string                      `json:"order_id"`
	OrderVersion         string                      `json:"order_version"`
	ChangeSeriesID       string                      `json:"change_series_id"`
	ReleaseIdentity      json.RawMessage             `json:"release_identity"`
	AdapterIdentity      json.RawMessage             `json:"adapter_identity"`
	Repository           string                      `json:"repository"`
	InformationState     TLC51InformationState       `json:"information_state"`
	Track                *string                     `json:"track"`
	RetainedFloor        *string                     `json:"retained_floor"`
	Subject              json.RawMessage             `json:"subject"`
	SubjectDigest        string                      `json:"subject_digest"`
	PlanDigest           string                      `json:"plan_digest"`
	Obligations          []TLC51ObligationProjection `json:"obligations"`
	Blockers             []string                    `json:"blockers"`
	HumanWaits           []TLC51HumanWaitProjection  `json:"human_waits"`
	Decision             string                      `json:"decision"`
	ReceiptDigest        string                      `json:"receipt_digest,omitempty"`
	Effects              []TLC51EffectProjection     `json:"effects"`
	WorkReconciliation   string                      `json:"work_reconciliation"`
	EventGraphEventCount int                         `json:"eventgraph_event_count"`
	WorkArtifactCount    int                         `json:"work_artifact_count"`
	GeneratedAt          time.Time                   `json:"generated_at"`
	AuthorityGranted     bool                        `json:"authority_granted"`
}

type TLC51ObligationProjection struct {
	ID                string                 `json:"id"`
	Kind              string                 `json:"kind"`
	Prerequisites     []string               `json:"prerequisites"`
	ParallelSafe      bool                   `json:"parallel_safe"`
	Status            string                 `json:"status"`
	Ready             bool                   `json:"ready"`
	AttemptOrdinal    uint32                 `json:"attempt_ordinal"`
	ProviderBindingID string                 `json:"provider_binding_id,omitempty"`
	EvidenceRecordIDs []string               `json:"evidence_record_ids"`
	Outcome           TLC51ObligationOutcome `json:"outcome,omitempty"`
}

type TLC51HumanWaitProjection struct {
	RequestID string `json:"request_id"`
	Boundary  string `json:"boundary"`
	Reason    string `json:"reason"`
	Status    string `json:"status"`
}

type TLC51EffectProjection struct {
	Effect               string             `json:"effect"`
	OperationID          string             `json:"operation_id"`
	IdempotencyKey       string             `json:"idempotency_key,omitempty"`
	ExternalState        TLC51ExternalState `json:"external_state"`
	ReconciliationAction string             `json:"reconciliation_action,omitempty"`
	Outcome              string             `json:"outcome,omitempty"`
	ObservationID        string             `json:"observation_id,omitempty"`
	ObservedAt           time.Time          `json:"observed_at,omitempty"`
}

// ProjectTLC51MissionControl produces a read-only row without evaluating TLC
// policy or granting authority. Legacy tlc-v1 rows are intentionally outside
// this projection and remain separately replayable.
func ProjectTLC51MissionControl(binding TLC51OrderBinding, plan TLC51GatePlan, history []TLC51HistoryEntry, work []TLC51WorkArtifact, generatedAt time.Time) (TLC51MissionControlOrder, error) {
	if generatedAt.IsZero() || generatedAt.Location() != time.UTC {
		return TLC51MissionControlOrder{}, fmt.Errorf("Mission Control generated_at must be explicit UTC")
	}
	var rawPlan struct {
		ReleaseIdentity json.RawMessage `json:"release_identity"`
		AdapterIdentity json.RawMessage `json:"adapter_identity"`
	}
	if err := json.Unmarshal(plan.Raw, &rawPlan); err != nil || len(rawPlan.ReleaseIdentity) == 0 || len(rawPlan.AdapterIdentity) == 0 {
		return TLC51MissionControlOrder{}, fmt.Errorf("plan release/adapter identity is unavailable")
	}
	row := TLC51MissionControlOrder{
		SchemaVersion: "factory-tlc51-mission-control/v1", ProtocolVersion: TLC51ProtocolVersion,
		FactoryOrderID: binding.FactoryOrderID, OrderID: binding.Order.DocID, OrderVersion: binding.Order.Version,
		ChangeSeriesID: plan.ChangeSeriesID, ReleaseIdentity: append(json.RawMessage(nil), rawPlan.ReleaseIdentity...),
		AdapterIdentity: append(json.RawMessage(nil), rawPlan.AdapterIdentity...), Repository: plan.Repository,
		InformationState: plan.InformationState, Track: cloneStringPointer(plan.Track), RetainedFloor: cloneStringPointer(plan.RetainedFloor),
		Subject: append(json.RawMessage(nil), plan.Subject...), SubjectDigest: plan.SubjectDigest, PlanDigest: plan.PlanDigest,
		Decision: "unknown", WorkReconciliation: "unknown", GeneratedAt: generatedAt,
		Obligations: []TLC51ObligationProjection{}, Blockers: []string{}, HumanWaits: []TLC51HumanWaitProjection{},
		Effects: []TLC51EffectProjection{}, AuthorityGranted: false,
	}
	if plan.InformationState != TLC51Classified {
		row.Blockers = append(row.Blockers, "information_state:"+string(plan.InformationState))
	}
	states, err := projectTLC51Obligations(plan, history)
	if err != nil {
		return TLC51MissionControlOrder{}, err
	}
	providerByObligation := map[string]string{}
	evidenceByObligation := map[string][]string{}
	human := map[string]TLC51HumanWaitProjection{}
	effects := map[string]TLC51EffectProjection{}
	for _, entry := range SortTLC51History(history) {
		if entry.Identity.FactoryOrderID != binding.FactoryOrderID || entry.Identity.ChangeSeriesID != plan.ChangeSeriesID {
			return TLC51MissionControlOrder{}, fmt.Errorf("history identity differs from projected order")
		}
		if entry.Identity.PlanDigest != plan.PlanDigest && entry.Type != TLC51PlanSuperseded && entry.Type != TLC51PlanRecorded {
			continue
		}
		var payload struct {
			ObligationID          string             `json:"obligation_id"`
			ProviderBindingID     string             `json:"provider_binding_id"`
			EvidenceRecordID      string             `json:"evidence_record_id"`
			RequestID             string             `json:"request_id"`
			Boundary              string             `json:"boundary"`
			Reason                string             `json:"reason"`
			Resolution            string             `json:"resolution"`
			Effect                string             `json:"effect"`
			OperationID           string             `json:"operation_id"`
			IdempotencyKey        string             `json:"idempotency_key"`
			ExternalState         TLC51ExternalState `json:"external_state"`
			Action                string             `json:"action"`
			Outcome               string             `json:"outcome"`
			ProviderObservationID string             `json:"provider_observation_id"`
			ObservedAt            time.Time          `json:"observed_at"`
			Decision              string             `json:"decision"`
			Receipt               TLC51ExactJSON     `json:"receipt"`
		}
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return TLC51MissionControlOrder{}, err
		}
		switch entry.Type {
		case TLC51ObligationClaimed:
			providerByObligation[payload.ObligationID] = payload.ProviderBindingID
		case TLC51EvidenceLinked:
			evidenceByObligation[payload.ObligationID] = append(evidenceByObligation[payload.ObligationID], payload.EvidenceRecordID)
		case TLC51HumanRequested:
			human[payload.RequestID] = TLC51HumanWaitProjection{RequestID: payload.RequestID, Boundary: payload.Boundary, Reason: payload.Reason, Status: "waiting"}
		case TLC51HumanResolved:
			wait := human[payload.RequestID]
			wait.RequestID = payload.RequestID
			wait.Status = "resolved"
			human[payload.RequestID] = wait
		case TLC51DecisionRecorded:
			row.Decision = payload.Decision
			var receipt struct {
				ReceiptDigest string `json:"receipt_digest"`
			}
			if json.Unmarshal([]byte(payload.Receipt.CanonicalJSON), &receipt) == nil {
				row.ReceiptDigest = receipt.ReceiptDigest
			}
		case TLC51DecisionInvalidated:
			row.Decision = "unknown"
			row.Blockers = append(row.Blockers, "decision_invalidated")
		case TLC51EffectProposed, TLC51EffectObserved, TLC51EffectReconciled, TLC51EffectTerminal:
			key := payload.Effect + "\x00" + payload.OperationID
			effect := effects[key]
			effect.Effect, effect.OperationID = payload.Effect, payload.OperationID
			if payload.IdempotencyKey != "" {
				effect.IdempotencyKey = payload.IdempotencyKey
			}
			if payload.ExternalState != "" {
				effect.ExternalState = payload.ExternalState
			}
			if payload.Action != "" {
				effect.ReconciliationAction = payload.Action
			}
			if payload.Outcome != "" {
				effect.Outcome = payload.Outcome
			}
			if payload.ProviderObservationID != "" {
				effect.ObservationID, effect.ObservedAt = payload.ProviderObservationID, payload.ObservedAt
			}
			effects[key] = effect
		}
	}
	for _, obligation := range plan.Obligations {
		state := states[obligation.ID]
		status := "pending"
		ready := tlc51PrerequisitesPassed(obligation, states)
		if state.HumanWaiting {
			status = "human_required"
		} else if state.Terminal != "" {
			status = string(state.Terminal)
		} else if state.RunningAttempt > 0 {
			status = "running"
		} else if providerByObligation[obligation.ID] != "" {
			status = "claimed"
		} else if ready {
			status = "ready"
		}
		row.Obligations = append(row.Obligations, TLC51ObligationProjection{
			ID: obligation.ID, Kind: obligation.Kind, Prerequisites: append([]string(nil), obligation.Prerequisites...),
			ParallelSafe: obligation.ParallelSafe, Status: status, Ready: ready, AttemptOrdinal: state.MaxAttempt,
			ProviderBindingID: providerByObligation[obligation.ID], EvidenceRecordIDs: append([]string(nil), evidenceByObligation[obligation.ID]...), Outcome: state.Terminal,
		})
		if state.Terminal != "" && state.Terminal != TLC51ObligationPassed {
			row.Blockers = append(row.Blockers, obligation.ID+":"+string(state.Terminal))
		}
	}
	for _, wait := range human {
		row.HumanWaits = append(row.HumanWaits, wait)
		if wait.Status == "waiting" {
			row.Blockers = append(row.Blockers, "human_wait:"+wait.RequestID)
		}
	}
	sort.Slice(row.HumanWaits, func(i, j int) bool { return row.HumanWaits[i].RequestID < row.HumanWaits[j].RequestID })
	for _, effect := range effects {
		row.Effects = append(row.Effects, effect)
	}
	sort.Slice(row.Effects, func(i, j int) bool {
		return row.Effects[i].Effect+"\x00"+row.Effects[i].OperationID < row.Effects[j].Effect+"\x00"+row.Effects[j].OperationID
	})
	row.EventGraphEventCount = len(history)
	row.WorkArtifactCount = len(work)
	row.WorkReconciliation = tlc51WorkReconciliationState(history, work)
	if row.WorkReconciliation != "match" {
		row.Blockers = append(row.Blockers, "work_reconciliation:"+row.WorkReconciliation)
	}
	row.Blockers = uniqueSortedTLC51Strings(row.Blockers)
	return row, nil
}

func tlc51WorkReconciliationState(history []TLC51HistoryEntry, work []TLC51WorkArtifact) string {
	if len(history) == 0 && len(work) == 0 {
		return "missing_both"
	}
	if len(history) == 0 {
		return "quarantine_missing_eventgraph"
	}
	byOrdinal := map[uint64]TLC51WorkArtifact{}
	for _, artifact := range work {
		if existing, ok := byOrdinal[artifact.EventOrdinal]; ok && !tlc51WorkArtifactsEqual(existing, artifact) {
			return "quarantine_conflict"
		}
		byOrdinal[artifact.EventOrdinal] = artifact
	}
	for _, entry := range history {
		artifact, ok := byOrdinal[entry.Identity.EventOrdinal]
		if !ok {
			return "repair_work_required"
		}
		if !tlc51WorkArtifactsEqual(artifact, TLC51WorkArtifactFromEntry(entry)) {
			return "quarantine_conflict"
		}
	}
	if len(byOrdinal) != len(history) {
		return "quarantine_orphan_work"
	}
	return "match"
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func uniqueSortedTLC51Strings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
