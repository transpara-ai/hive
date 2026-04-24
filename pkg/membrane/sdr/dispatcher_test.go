package sdr

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/transpara-ai/hive/pkg/membrane"
)

func TestDispatchApproval(t *testing.T) {
	sdrClient := &mockClient{}
	bridgeClient := &mockClient{
		responses: map[string]json.RawMessage{
			"/api/bridge/decisions?agent=sdr": json.RawMessage(`[
				{
					"id": "act-001",
					"agent_name": "sdr",
					"action_type": "approval",
					"status": "approved",
					"decided_by": "user-matt",
					"decision_notes": "",
					"domain_data": {"interaction_id": "int-001"}
				}
			]`),
		},
	}

	dispatcher := &Dispatcher{sdrClient: sdrClient, bridgeClient: bridgeClient}
	dispatched, err := dispatcher.PollAndDispatch(context.Background())
	if err != nil {
		t.Fatalf("PollAndDispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}
	if len(sdrClient.posts) != 1 {
		t.Fatalf("sdr posts = %d, want 1", len(sdrClient.posts))
	}
	if sdrClient.posts[0].Path != "/copilot/drafts/int-001/approve" {
		t.Errorf("path = %q, want /copilot/drafts/int-001/approve", sdrClient.posts[0].Path)
	}
}

func TestDispatchRejection(t *testing.T) {
	sdrClient := &mockClient{}
	bridgeClient := &mockClient{
		responses: map[string]json.RawMessage{
			"/api/bridge/decisions?agent=sdr": json.RawMessage(`[
				{
					"id": "act-002",
					"agent_name": "sdr",
					"action_type": "approval",
					"status": "rejected",
					"decided_by": "user-matt",
					"decision_notes": "Too aggressive",
					"domain_data": {"interaction_id": "int-002"}
				}
			]`),
		},
	}

	dispatcher := &Dispatcher{sdrClient: sdrClient, bridgeClient: bridgeClient}
	dispatched, err := dispatcher.PollAndDispatch(context.Background())
	if err != nil {
		t.Fatalf("PollAndDispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}
	if sdrClient.posts[0].Path != "/copilot/drafts/int-002/reject" {
		t.Errorf("path = %q, want /copilot/drafts/int-002/reject", sdrClient.posts[0].Path)
	}
}

func TestDispatchHandoffIsNoOp(t *testing.T) {
	sdrClient := &mockClient{}
	bridgeClient := &mockClient{
		responses: map[string]json.RawMessage{
			"/api/bridge/decisions?agent=sdr": json.RawMessage(`[
				{
					"id": "act-003",
					"agent_name": "sdr",
					"action_type": "handoff",
					"status": "approved",
					"decided_by": "user-matt",
					"decision_notes": "",
					"domain_data": {"lead_id": "lead-001"}
				}
			]`),
		},
	}

	dispatcher := &Dispatcher{sdrClient: sdrClient, bridgeClient: bridgeClient}
	dispatched, err := dispatcher.PollAndDispatch(context.Background())
	if err != nil {
		t.Fatalf("PollAndDispatch: %v", err)
	}
	if dispatched != 1 {
		t.Errorf("dispatched = %d, want 1", dispatched)
	}
	// Handoffs don't call the ai-sdr API
	if len(sdrClient.posts) != 0 {
		t.Errorf("sdr posts = %d, want 0 (handoffs are informational)", len(sdrClient.posts))
	}
}

func TestSetOperatingMode(t *testing.T) {
	sdrClient := &mockClient{}
	dispatcher := &Dispatcher{sdrClient: sdrClient}
	bands := membrane.TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6}

	tests := []struct {
		trust      float64
		wantCopilot bool
	}{
		{0.1, true},  // Required → copilot on
		{0.4, true},  // Recommended → copilot on
		{0.8, false}, // Notification → copilot off (autonomous)
	}

	for _, tt := range tests {
		sdrClient.posts = nil
		err := dispatcher.SetOperatingMode(context.Background(), tt.trust, bands)
		if err != nil {
			t.Fatalf("SetOperatingMode(%.1f): %v", tt.trust, err)
		}
		if len(sdrClient.posts) != 1 {
			t.Fatalf("posts = %d, want 1", len(sdrClient.posts))
		}
		if sdrClient.posts[0].Path != "/api/settings/mode" {
			t.Errorf("path = %q, want /api/settings/mode", sdrClient.posts[0].Path)
		}
	}
}
