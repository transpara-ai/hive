package sdr

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/lovyou-ai/hive/pkg/membrane"
)

type mockClient struct {
	responses map[string]json.RawMessage
	posts     []postRecord
	getCalls  atomic.Int32
}

type postRecord struct {
	Path string
	Body interface{}
}

func (m *mockClient) Get(ctx context.Context, path string) (json.RawMessage, error) {
	m.getCalls.Add(1)
	if resp, ok := m.responses[path]; ok {
		return resp, nil
	}
	return json.RawMessage(`{}`), nil
}

func (m *mockClient) Post(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	m.posts = append(m.posts, postRecord{Path: path, Body: body})
	return json.RawMessage(`{"status":"ok","action_id":"test-123"}`), nil
}

func TestPollDraftsCreatesActions(t *testing.T) {
	sdrClient := &mockClient{
		responses: map[string]json.RawMessage{
			"/copilot/drafts?per_page=50&sort=created_at&sort_direction=asc": json.RawMessage(`{
				"drafts": [
					{
						"interaction_id": "int-001",
						"lead_id": "lead-001",
						"lead_name": "Sarah Chen",
						"company": "Aurex Mining",
						"email": "sarah@aurex.com",
						"subject": "Operational visibility",
						"body_text": "Hi Sarah...",
						"score_total": 62,
						"score_company_fit": 18,
						"score_pain_need": 15,
						"score_authority": 14,
						"score_timing": 15,
						"current_state": "outreach_active",
						"internal_notes": "Good fit",
						"created_at": "2026-03-30T10:00:00Z"
					}
				],
				"count": 1
			}`),
		},
	}

	bridgeClient := &mockClient{}
	poller := &Poller{client: sdrClient, bridgeClient: bridgeClient}

	bands := membrane.TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6}
	created, err := poller.PollDrafts(context.Background(), 0.1, bands) // low trust = Required
	if err != nil {
		t.Fatalf("PollDrafts: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1", created)
	}
	if len(bridgeClient.posts) != 1 {
		t.Fatalf("bridge posts = %d, want 1", len(bridgeClient.posts))
	}
	if bridgeClient.posts[0].Path != "/api/bridge/action" {
		t.Errorf("path = %q, want /api/bridge/action", bridgeClient.posts[0].Path)
	}
}

func TestPollDraftsSkipsAtHighTrust(t *testing.T) {
	sdrClient := &mockClient{}
	bridgeClient := &mockClient{}
	poller := &Poller{client: sdrClient, bridgeClient: bridgeClient}

	bands := membrane.TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6}
	created, err := poller.PollDrafts(context.Background(), 0.8, bands) // high trust = Notification
	if err != nil {
		t.Fatalf("PollDrafts: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 (should skip at high trust)", created)
	}
	if sdrClient.getCalls.Load() != 0 {
		t.Error("should not poll ai-sdr at Notification level")
	}
}

func TestPollStats(t *testing.T) {
	sdrClient := &mockClient{
		responses: map[string]json.RawMessage{
			"/api/dashboard/stats": json.RawMessage(`{
				"active_leads": 47,
				"response_rate": 18.5,
				"avg_score": 72.3,
				"qualified_count": 4
			}`),
		},
	}
	bridgeClient := &mockClient{}
	poller := &Poller{client: sdrClient, bridgeClient: bridgeClient}

	stats, err := poller.PollStats(context.Background())
	if err != nil {
		t.Fatalf("PollStats: %v", err)
	}
	if stats.ActiveLeads != 47 {
		t.Errorf("active_leads = %d, want 47", stats.ActiveLeads)
	}
	if stats.QualifiedCount != 4 {
		t.Errorf("qualified_count = %d, want 4", stats.QualifiedCount)
	}
}

func TestCheckHealth(t *testing.T) {
	sdrClient := &mockClient{
		responses: map[string]json.RawMessage{
			"/health": json.RawMessage(`{"status": "healthy"}`),
		},
	}
	poller := &Poller{client: sdrClient}

	health, err := poller.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if health.Status != "healthy" {
		t.Errorf("status = %q, want healthy", health.Status)
	}
}
