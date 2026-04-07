package telemetry

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAgentSnapshot_JSONRoundTrip(t *testing.T) {
	trust := 0.85
	original := AgentSnapshot{
		ID:            42,
		RecordedAt:    time.Date(2026, 4, 4, 14, 30, 0, 0, time.UTC),
		AgentRole:     "guardian",
		ActorID:       "actor_00d5d8ac",
		State:         "Idle",
		Model:         "claude-sonnet-4-6",
		Iteration:     30,
		MaxIterations: 200,
		TokensUsed:    12450,
		CostUSD:       0.089,
		TrustScore:    &trust,
		LastEventType: "health.report",
		LastMessage:   "Chain integrity verified.",
		Errors:        0,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AgentSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != original.ID {
		t.Errorf("ID: got %d, want %d", got.ID, original.ID)
	}
	if got.AgentRole != original.AgentRole {
		t.Errorf("AgentRole: got %q, want %q", got.AgentRole, original.AgentRole)
	}
	if got.State != original.State {
		t.Errorf("State: got %q, want %q", got.State, original.State)
	}
	if got.Model != original.Model {
		t.Errorf("Model: got %q, want %q", got.Model, original.Model)
	}
	if got.Iteration != original.Iteration {
		t.Errorf("Iteration: got %d, want %d", got.Iteration, original.Iteration)
	}
	if got.MaxIterations != original.MaxIterations {
		t.Errorf("MaxIterations: got %d, want %d", got.MaxIterations, original.MaxIterations)
	}
	if got.TokensUsed != original.TokensUsed {
		t.Errorf("TokensUsed: got %d, want %d", got.TokensUsed, original.TokensUsed)
	}
	if got.CostUSD != original.CostUSD {
		t.Errorf("CostUSD: got %f, want %f", got.CostUSD, original.CostUSD)
	}
	if got.TrustScore == nil || *got.TrustScore != *original.TrustScore {
		t.Errorf("TrustScore: got %v, want %v", got.TrustScore, original.TrustScore)
	}
	if got.LastEventType != original.LastEventType {
		t.Errorf("LastEventType: got %q, want %q", got.LastEventType, original.LastEventType)
	}
	if got.LastMessage != original.LastMessage {
		t.Errorf("LastMessage: got %q, want %q", got.LastMessage, original.LastMessage)
	}
	if got.Errors != original.Errors {
		t.Errorf("Errors: got %d, want %d", got.Errors, original.Errors)
	}
	if !got.RecordedAt.Equal(original.RecordedAt) {
		t.Errorf("RecordedAt: got %v, want %v", got.RecordedAt, original.RecordedAt)
	}
}

func TestHiveSnapshot_JSONRoundTrip(t *testing.T) {
	rate := 23.5
	cost := 0.42
	cap := 5.0
	original := HiveSnapshot{
		ID:           1,
		RecordedAt:   time.Date(2026, 4, 4, 14, 30, 0, 0, time.UTC),
		ActiveAgents: 6,
		TotalActors:  7,
		ChainLength:  1008,
		ChainOK:      true,
		EventRate:    &rate,
		DailyCost:    &cost,
		DailyCap:     &cap,
		Severity:     "ok",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got HiveSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ActiveAgents != original.ActiveAgents {
		t.Errorf("ActiveAgents: got %d, want %d", got.ActiveAgents, original.ActiveAgents)
	}
	if got.ChainLength != original.ChainLength {
		t.Errorf("ChainLength: got %d, want %d", got.ChainLength, original.ChainLength)
	}
	if got.ChainOK != original.ChainOK {
		t.Errorf("ChainOK: got %v, want %v", got.ChainOK, original.ChainOK)
	}
	if got.EventRate == nil || *got.EventRate != *original.EventRate {
		t.Errorf("EventRate: got %v, want %v", got.EventRate, original.EventRate)
	}
	if got.DailyCost == nil || *got.DailyCost != *original.DailyCost {
		t.Errorf("DailyCost: got %v, want %v", got.DailyCost, original.DailyCost)
	}
	if got.DailyCap == nil || *got.DailyCap != *original.DailyCap {
		t.Errorf("DailyCap: got %v, want %v", got.DailyCap, original.DailyCap)
	}
	if got.Severity != original.Severity {
		t.Errorf("Severity: got %q, want %q", got.Severity, original.Severity)
	}
}

func TestPhase_JSONRoundTrip(t *testing.T) {
	started := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	completed := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	exitCriteria := "All foundation agents coordinating via events and tasks."

	tests := []struct {
		name  string
		phase Phase
	}{
		{
			name: "complete phase with timestamps",
			phase: Phase{
				Phase:        0,
				Label:        "Foundation",
				Status:       "complete",
				StartedAt:    &started,
				CompletedAt:  &completed,
				Notes:        "Strategist, Planner, Implementer, Guardian running.",
				ExitCriteria: &exitCriteria,
			},
		},
		{
			name: "blocked phase with nil timestamps",
			phase: Phase{
				Phase:        2,
				Label:        "Technical leadership",
				Status:       "blocked",
				StartedAt:    nil,
				CompletedAt:  nil,
				Notes:        "CTO + Reviewer — no AgentDefs.",
				ExitCriteria: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.phase)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got Phase
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.Phase != tt.phase.Phase {
				t.Errorf("Phase: got %d, want %d", got.Phase, tt.phase.Phase)
			}
			if got.Label != tt.phase.Label {
				t.Errorf("Label: got %q, want %q", got.Label, tt.phase.Label)
			}
			if got.Status != tt.phase.Status {
				t.Errorf("Status: got %q, want %q", got.Status, tt.phase.Status)
			}
			if got.Notes != tt.phase.Notes {
				t.Errorf("Notes: got %q, want %q", got.Notes, tt.phase.Notes)
			}

			if tt.phase.StartedAt == nil {
				if got.StartedAt != nil {
					t.Errorf("StartedAt: got %v, want nil", got.StartedAt)
				}
			} else if got.StartedAt == nil || !got.StartedAt.Equal(*tt.phase.StartedAt) {
				t.Errorf("StartedAt: got %v, want %v", got.StartedAt, tt.phase.StartedAt)
			}

			if tt.phase.CompletedAt == nil {
				if got.CompletedAt != nil {
					t.Errorf("CompletedAt: got %v, want nil", got.CompletedAt)
				}
			} else if got.CompletedAt == nil || !got.CompletedAt.Equal(*tt.phase.CompletedAt) {
				t.Errorf("CompletedAt: got %v, want %v", got.CompletedAt, tt.phase.CompletedAt)
			}

			if tt.phase.ExitCriteria == nil {
				if got.ExitCriteria != nil {
					t.Errorf("ExitCriteria: got %v, want nil", got.ExitCriteria)
				}
			} else if got.ExitCriteria == nil || *got.ExitCriteria != *tt.phase.ExitCriteria {
				t.Errorf("ExitCriteria: got %v, want %v", got.ExitCriteria, tt.phase.ExitCriteria)
			}
		})
	}
}

func TestEventStreamEntry_JSONRoundTrip(t *testing.T) {
	original := EventStreamEntry{
		ID:         99,
		RecordedAt: time.Date(2026, 4, 4, 14, 31, 55, 0, time.UTC),
		EventType:  "health.report",
		ActorRole:  "sysmon",
		Summary:    "Health OK: 6 agents active, chain intact",
		RawContent: json.RawMessage(`{"severity":"ok","active_agents":6}`),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got EventStreamEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != original.ID {
		t.Errorf("ID: got %d, want %d", got.ID, original.ID)
	}
	if got.EventType != original.EventType {
		t.Errorf("EventType: got %q, want %q", got.EventType, original.EventType)
	}
	if got.ActorRole != original.ActorRole {
		t.Errorf("ActorRole: got %q, want %q", got.ActorRole, original.ActorRole)
	}
	if got.Summary != original.Summary {
		t.Errorf("Summary: got %q, want %q", got.Summary, original.Summary)
	}
	if string(got.RawContent) != string(original.RawContent) {
		t.Errorf("RawContent: got %s, want %s", got.RawContent, original.RawContent)
	}
}

func TestAgentSnapshot_NullableTrustScore(t *testing.T) {
	snapshot := AgentSnapshot{
		ID:            1,
		RecordedAt:    time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
		AgentRole:     "implementer",
		ActorID:       "actor_abc123",
		State:         "Processing",
		Model:         "claude-opus-4-6",
		Iteration:     10,
		MaxIterations: 100,
		TrustScore:    nil,
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify null appears in JSON.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw["trust_score"] != nil {
		t.Errorf("trust_score: got %v, want null", raw["trust_score"])
	}

	// Round-trip preserves nil.
	var got AgentSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TrustScore != nil {
		t.Errorf("TrustScore: got %v, want nil", got.TrustScore)
	}
}
