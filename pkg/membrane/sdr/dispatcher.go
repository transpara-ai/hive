package sdr

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/transpara-ai/hive/pkg/membrane"
)

// Dispatcher translates human decisions from the bridge into ai-sdr API calls.
type Dispatcher struct {
	sdrClient    membrane.ServiceClient
	bridgeClient membrane.ServiceClient
}

// NewDispatcher creates an SDR dispatcher that reads decisions from the bridge
// and executes them against the ai-sdr API.
func NewDispatcher(sdrClient membrane.ServiceClient, bridgeURL string) *Dispatcher {
	return &Dispatcher{
		sdrClient:    sdrClient,
		bridgeClient: membrane.NewHTTPServiceClient(bridgeURL, "bearer", nil),
	}
}

// BridgeDecision matches the bridge_actions row format from the site API.
type BridgeDecision struct {
	ID            string          `json:"id"`
	AgentName     string          `json:"agent_name"`
	ActionType    string          `json:"action_type"`
	Summary       string          `json:"summary"`
	Authority     string          `json:"authority"`
	Status        string          `json:"status"` // "approved", "rejected"
	DecidedBy     string          `json:"decided_by"`
	DecisionNotes string          `json:"decision_notes"`
	DomainData    json.RawMessage `json:"domain_data"`
}

// PollAndDispatch checks the bridge for decided actions and dispatches them to the ai-sdr.
func (d *Dispatcher) PollAndDispatch(ctx context.Context) (int, error) {
	resp, err := d.bridgeClient.Get(ctx, "/api/bridge/decisions?agent=sdr")
	if err != nil {
		return 0, fmt.Errorf("poll decisions: %w", err)
	}

	var decisions []BridgeDecision
	if err := json.Unmarshal(resp, &decisions); err != nil {
		return 0, fmt.Errorf("parse decisions: %w", err)
	}

	dispatched := 0
	for _, dec := range decisions {
		if err := d.dispatch(ctx, dec); err != nil {
			log.Printf("sdr dispatcher: failed to dispatch %s (%s): %v", dec.ID, dec.ActionType, err)
			continue
		}
		dispatched++
	}

	return dispatched, nil
}

func (d *Dispatcher) dispatch(ctx context.Context, dec BridgeDecision) error {
	switch dec.ActionType {
	case "approval":
		return d.dispatchDraftDecision(ctx, dec)
	case "handoff":
		// Handoffs are informational — the human accepts or returns to SDR.
		// No ai-sdr API call needed; the bridge action status is the record.
		log.Printf("sdr dispatcher: handoff %s decided: %s by %s", dec.ID, dec.Status, dec.DecidedBy)
		return nil
	case "escalation":
		log.Printf("sdr dispatcher: escalation %s decided: %s by %s", dec.ID, dec.Status, dec.DecidedBy)
		return nil
	default:
		return fmt.Errorf("unknown action type: %s", dec.ActionType)
	}
}

func (d *Dispatcher) dispatchDraftDecision(ctx context.Context, dec BridgeDecision) error {
	var domain struct {
		InteractionID string `json:"interaction_id"`
	}
	if err := json.Unmarshal(dec.DomainData, &domain); err != nil {
		return fmt.Errorf("parse domain data: %w", err)
	}
	if domain.InteractionID == "" {
		return fmt.Errorf("missing interaction_id in domain data")
	}

	switch dec.Status {
	case "approved":
		path := fmt.Sprintf("/copilot/drafts/%s/approve", domain.InteractionID)
		_, err := d.sdrClient.Post(ctx, path, map[string]string{})
		if err != nil {
			return fmt.Errorf("approve draft %s: %w", domain.InteractionID, err)
		}
		log.Printf("sdr dispatcher: approved draft %s (decided by %s)", domain.InteractionID, dec.DecidedBy)

	case "rejected":
		path := fmt.Sprintf("/copilot/drafts/%s/reject", domain.InteractionID)
		body := map[string]string{}
		if dec.DecisionNotes != "" {
			body["reason"] = dec.DecisionNotes
		}
		_, err := d.sdrClient.Post(ctx, path, body)
		if err != nil {
			return fmt.Errorf("reject draft %s: %w", domain.InteractionID, err)
		}
		log.Printf("sdr dispatcher: rejected draft %s (decided by %s, reason: %s)", domain.InteractionID, dec.DecidedBy, dec.DecisionNotes)

	default:
		return fmt.Errorf("unexpected draft decision status: %s", dec.Status)
	}

	return nil
}

// SetOperatingMode sets the ai-sdr's co-pilot mode based on trust score.
func (d *Dispatcher) SetOperatingMode(ctx context.Context, trustScore float64, bands membrane.TrustBands) error {
	level := bands.AuthorityFor(trustScore)

	// At Notification level (high trust), disable co-pilot mode (full autonomous)
	// At Required/Recommended (low/medium trust), enable co-pilot mode (drafts held)
	copilotMode := level != membrane.AuthNotification

	_, err := d.sdrClient.Post(ctx, "/api/settings/mode", map[string]bool{
		"copilot_mode": copilotMode,
	})
	if err != nil {
		return fmt.Errorf("set copilot mode: %w", err)
	}

	log.Printf("sdr dispatcher: copilot_mode=%v (trust=%.2f, authority=%s)", copilotMode, trustScore, level)
	return nil
}
