package hive

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/transpara-ai/work"
)

func TestParseIntendPayload(t *testing.T) {
	tests := []struct {
		name         string
		payload      map[string]string
		wantDesc     string
		wantPriority work.TaskPriority
	}{
		{
			name:         "description only",
			payload:      map[string]string{"description": "short summary"},
			wantDesc:     "short summary",
			wantPriority: work.DefaultPriority,
		},
		{
			name:         "body only",
			payload:      map[string]string{"body": "full content"},
			wantDesc:     "full content",
			wantPriority: work.DefaultPriority,
		},
		{
			name:         "description and body combined",
			payload:      map[string]string{"description": "summary", "body": "full doc"},
			wantDesc:     "summary\n\nfull doc",
			wantPriority: work.DefaultPriority,
		},
		{
			name:         "valid priority high",
			payload:      map[string]string{"description": "task", "priority": "high"},
			wantDesc:     "task",
			wantPriority: work.PriorityHigh,
		},
		{
			name:         "valid priority critical",
			payload:      map[string]string{"description": "task", "priority": "critical"},
			wantDesc:     "task",
			wantPriority: work.PriorityCritical,
		},
		{
			name:         "invalid priority falls back to default",
			payload:      map[string]string{"description": "task", "priority": "urgent"},
			wantDesc:     "task",
			wantPriority: work.DefaultPriority,
		},
		{
			name:         "empty priority defaults",
			payload:      map[string]string{"description": "task"},
			wantDesc:     "task",
			wantPriority: work.DefaultPriority,
		},
		{
			name:         "empty payload",
			payload:      nil,
			wantDesc:     "",
			wantPriority: work.DefaultPriority,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw json.RawMessage
			if tt.payload != nil {
				raw, _ = json.Marshal(tt.payload)
			}
			got := parseIntendPayload(raw)
			if got.Desc != tt.wantDesc {
				t.Errorf("Desc = %q; want %q", got.Desc, tt.wantDesc)
			}
			if got.Priority != tt.wantPriority {
				t.Errorf("Priority = %q; want %q", got.Priority, tt.wantPriority)
			}
		})
	}
}

func TestParseIntendPayload_LargeBody(t *testing.T) {
	// Verify that description + body combination preserves full content.
	body := strings.Repeat("x", 10000)
	payload, _ := json.Marshal(map[string]string{
		"description": "summary",
		"body":        body,
	})
	got := parseIntendPayload(payload)
	if !strings.Contains(got.Desc, body) {
		t.Error("body content was truncated or lost")
	}
	if !strings.HasPrefix(got.Desc, "summary\n\n") {
		t.Error("description prefix missing")
	}
}

func TestIsSafeRefineryAutoTarget(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{state: "requirement.clarifying", want: true},
		{state: "requirement.investigating", want: true},
		{state: "spec.draft", want: true},
		{state: "needs_attention", want: true},
		{state: "spec.review", want: false},
		{state: "spec.normative", want: false},
		{state: "build.ready", want: false},
		{state: "shipped", want: false},
		{state: "parked", want: false},
		{state: "deleted", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := isSafeRefineryAutoTarget(tt.state)
			if got != tt.want {
				t.Fatalf("isSafeRefineryAutoTarget(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
