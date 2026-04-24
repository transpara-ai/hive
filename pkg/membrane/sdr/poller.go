package sdr

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/transpara-ai/hive/pkg/membrane"
)

// Poller checks the ai-sdr API for drafts needing approval and lead state changes.
type Poller struct {
	client  membrane.ServiceClient
	bridgeURL string // base URL of the site bridge API
	bridgeClient membrane.ServiceClient
}

// NewPoller creates an SDR poller that talks to the ai-sdr and bridge APIs.
func NewPoller(sdrClient membrane.ServiceClient, bridgeURL string) *Poller {
	return &Poller{
		client:      sdrClient,
		bridgeURL:   bridgeURL,
		bridgeClient: membrane.NewHTTPServiceClient(bridgeURL, "bearer", nil),
	}
}

// PollDrafts checks for pending co-pilot drafts and creates bridge actions for each.
func (p *Poller) PollDrafts(ctx context.Context, trustScore float64, bands membrane.TrustBands) (int, error) {
	level := bands.AuthorityFor(trustScore)

	// At Notification level, the ai-sdr runs autonomously — no drafts to approve
	if level == membrane.AuthNotification {
		return 0, nil
	}

	resp, err := p.client.Get(ctx, "/copilot/drafts?per_page=50&sort=created_at&sort_direction=asc")
	if err != nil {
		return 0, fmt.Errorf("poll drafts: %w", err)
	}

	var result struct {
		Drafts []Draft `json:"drafts"`
		Count  int     `json:"count"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, fmt.Errorf("parse drafts: %w", err)
	}

	created := 0
	for _, d := range result.Drafts {
		action := BridgeActionRequest{
			AgentName:   "sdr",
			ActionType:  "approval",
			Summary:     fmt.Sprintf("Outbound email to %s (%s)", d.LeadName, d.Company),
			Authority:   level.String(),
			TargetHuman: "", // filled by escalation config
			DomainData: map[string]interface{}{
				"interaction_id": d.InteractionID,
				"lead_id":        d.LeadID,
				"lead_name":      d.LeadName,
				"company":        d.Company,
				"email":          d.Email,
				"subject":        d.Subject,
				"body_preview":   truncate(d.BodyText, 200),
				"score_total":    d.ScoreTotal,
				"score_company":  d.ScoreCompanyFit,
				"score_pain":     d.ScorePainNeed,
				"score_authority": d.ScoreAuthority,
				"score_timing":   d.ScoreTiming,
				"stage":          d.CurrentState,
				"internal_notes": d.InternalNotes,
			},
		}

		if _, err := p.bridgeClient.Post(ctx, "/api/bridge/action", action); err != nil {
			log.Printf("sdr poller: create bridge action for draft %s: %v", d.InteractionID, err)
			continue
		}
		created++
	}

	return created, nil
}

// PollLeadStates checks for leads in key states and emits bridge events.
func (p *Poller) PollLeadStates(ctx context.Context) error {
	// Check for qualified handoffs
	resp, err := p.client.Get(ctx, "/api/leads?state=qualified_handoff&per_page=50")
	if err != nil {
		return fmt.Errorf("poll qualified leads: %w", err)
	}

	var result struct {
		Items []LeadSummary `json:"items"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("parse leads: %w", err)
	}

	for _, lead := range result.Items {
		action := BridgeActionRequest{
			AgentName:   "sdr",
			ActionType:  "handoff",
			Summary:     fmt.Sprintf("Qualified lead: %s (%s) — score %d", lead.Name, lead.Company, lead.Score),
			Authority:   "required", // handoffs always require approval
			TargetHuman: "",
			DomainData: map[string]interface{}{
				"lead_id":  lead.ID,
				"name":     lead.Name,
				"company":  lead.Company,
				"score":    lead.Score,
				"stage":    lead.Stage,
				"state":    lead.State,
				"email":    lead.Email,
				"title":    lead.Title,
			},
		}

		if _, err := p.bridgeClient.Post(ctx, "/api/bridge/action", action); err != nil {
			log.Printf("sdr poller: create handoff action for %s: %v", lead.ID, err)
			continue
		}
	}

	return nil
}

// PollStats fetches dashboard stats and emits a bridge event.
func (p *Poller) PollStats(ctx context.Context) (*DashboardStats, error) {
	resp, err := p.client.Get(ctx, "/api/dashboard/stats")
	if err != nil {
		return nil, fmt.Errorf("poll stats: %w", err)
	}

	var stats DashboardStats
	if err := json.Unmarshal(resp, &stats); err != nil {
		return nil, fmt.Errorf("parse stats: %w", err)
	}

	// Emit as bridge event
	eventPayload, _ := json.Marshal(stats)
	p.bridgeClient.Post(ctx, "/api/bridge/event", map[string]interface{}{
		"agent_name": "sdr",
		"event_type": "membrane.service.polled",
		"payload":    json.RawMessage(eventPayload),
	})

	return &stats, nil
}

// CheckHealth verifies the ai-sdr is running.
func (p *Poller) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	resp, err := p.client.Get(ctx, "/health")
	if err != nil {
		return nil, fmt.Errorf("health check: %w", err)
	}

	var health HealthStatus
	if err := json.Unmarshal(resp, &health); err != nil {
		return nil, fmt.Errorf("parse health: %w", err)
	}

	return &health, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "..."
}

// Draft represents a co-pilot draft from the ai-sdr API.
type Draft struct {
	InteractionID  string  `json:"interaction_id"`
	LeadID         string  `json:"lead_id"`
	LeadName       string  `json:"lead_name"`
	Company        string  `json:"company"`
	Email          string  `json:"email"`
	Subject        string  `json:"subject"`
	BodyText       string  `json:"body_text"`
	ScoreTotal     int     `json:"score_total"`
	ScoreCompanyFit int    `json:"score_company_fit"`
	ScorePainNeed  int     `json:"score_pain_need"`
	ScoreAuthority int     `json:"score_authority"`
	ScoreTiming    int     `json:"score_timing"`
	CurrentState   string  `json:"current_state"`
	InternalNotes  string  `json:"internal_notes"`
	CreatedAt      string  `json:"created_at"`
}

// LeadSummary matches the ai-sdr's LeadSummary response.
type LeadSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Company  string `json:"company"`
	Title    string `json:"title"`
	Email    string `json:"email"`
	Score    int    `json:"score"`
	Stage    string `json:"stage"`
	State    string `json:"state"`
}

// DashboardStats matches the ai-sdr's /api/dashboard/stats response.
type DashboardStats struct {
	ActiveLeads    int     `json:"active_leads"`
	ResponseRate   float64 `json:"response_rate"`
	AvgScore       float64 `json:"avg_score"`
	QualifiedCount int     `json:"qualified_count"`
}

// HealthStatus matches the ai-sdr's /health response.
type HealthStatus struct {
	Status string `json:"status"` // "healthy", "degraded", "unhealthy"
}

// BridgeActionRequest is the payload for POST /api/bridge/action.
type BridgeActionRequest struct {
	AgentName   string      `json:"agent_name"`
	ActionType  string      `json:"action_type"`
	Summary     string      `json:"summary"`
	Authority   string      `json:"authority"`
	TargetHuman string      `json:"target_human"`
	DomainData  interface{} `json:"domain_data"`
}
