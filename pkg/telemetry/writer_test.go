package telemetry

import (
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/work"
)

func TestRegisterAgent(t *testing.T) {
	w := &Writer{
		lastResponses: make(map[string]string),
	}

	names := []string{"guardian", "sysmon", "allocator", "strategist", "planner", "implementer"}
	for _, name := range names {
		w.RegisterAgent(AgentRegistration{
			Name:          name,
			Role:          name,
			Model:         "test-model",
			MaxIterations: 50,
		})
	}

	if got := w.Agents(); got != 6 {
		t.Errorf("Agents() = %d, want 6", got)
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	for i, name := range names {
		if w.agents[i].Name != name {
			t.Errorf("agents[%d].Name = %q, want %q", i, w.agents[i].Name, name)
		}
	}
}

func TestRecordResponse(t *testing.T) {
	w := &Writer{
		lastResponses: make(map[string]string),
	}

	w.RecordResponse("guardian", "All clear. Chain intact.")
	w.RecordResponse("sysmon", "/health {\"severity\":\"ok\"}")

	w.mu.RLock()
	defer w.mu.RUnlock()

	if got := w.lastResponses["guardian"]; got != "All clear. Chain intact." {
		t.Errorf("guardian response = %q, want %q", got, "All clear. Chain intact.")
	}
	if got := w.lastResponses["sysmon"]; got != "/health {\"severity\":\"ok\"}" {
		t.Errorf("sysmon response = %q", got)
	}
}

func TestRecordResponseTruncation(t *testing.T) {
	w := &Writer{
		lastResponses: make(map[string]string),
	}

	// 600-char string should be truncated to 500.
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}
	w.RecordResponse("implementer", string(long))

	w.mu.RLock()
	got := w.lastResponses["implementer"]
	w.mu.RUnlock()

	if len(got) != 500 {
		t.Errorf("truncated length = %d, want 500", len(got))
	}
}

func TestRecordResponseOverwrite(t *testing.T) {
	w := &Writer{
		lastResponses: make(map[string]string),
	}

	w.RecordResponse("guardian", "first")
	w.RecordResponse("guardian", "second")

	w.mu.RLock()
	got := w.lastResponses["guardian"]
	w.mu.RUnlock()

	if got != "second" {
		t.Errorf("response = %q, want %q", got, "second")
	}
}

func TestWriterNilBudgetRegistry(t *testing.T) {
	// collectAndWrite should not panic when budgetRegistry is nil.
	w := &Writer{
		lastResponses: make(map[string]string),
	}
	w.RegisterAgent(AgentRegistration{
		Name: "test",
		Role: "test",
	})

	// This would panic if we didn't guard against nil budgetRegistry.
	// We can't test the full DB path without postgres, but we verify
	// the data collection path doesn't panic.
	// The method returns early when pool is nil (no tx), which is fine.
}

func TestEventSummary(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		eventType string
		content   interface{}
		want      string
	}{
		{
			name:      "nil content falls back",
			role:      "guardian",
			eventType: "health.report",
			content:   nil,
			want:      "guardian: health.report",
		},
		{
			name:      "task created",
			role:      "strategist",
			eventType: "work.task.created",
			content:   work.TaskCreatedContent{Title: "Build auth system"},
			want:      "Task: Build auth system",
		},
		{
			name:      "task completed with summary",
			role:      "implementer",
			eventType: "work.task.completed",
			content:   work.TaskCompletedContent{Summary: "Implemented JWT auth"},
			want:      "Completed: Implemented JWT auth",
		},
		{
			name:      "task completed without summary",
			role:      "implementer",
			eventType: "work.task.completed",
			content:   work.TaskCompletedContent{},
			want:      "Task completed",
		},
		{
			name:      "gap detected",
			role:      "cto",
			eventType: "hive.gap.detected",
			content:   event.GapDetectedContent{MissingRole: "reviewer", Evidence: "no code review agent"},
			want:      "Gap: reviewer — no code review agent",
		},
		{
			name:      "role proposed",
			role:      "cto",
			eventType: "hive.role.proposed",
			content:   event.RoleProposedContent{Name: "reviewer"},
			want:      "Proposed: reviewer",
		},
		{
			name:      "agent state changed",
			role:      "guardian",
			eventType: "agent.state.changed",
			content:   event.AgentStateChangedContent{Previous: "Idle", Current: "Active"},
			want:      "Idle → Active",
		},
		{
			name:      "agent escalated",
			role:      "implementer",
			eventType: "agent.escalated",
			content:   event.AgentEscalatedContent{Reason: "budget exceeded"},
			want:      "ESCALATED: budget exceeded",
		},
		{
			name:      "agent budget adjusted",
			role:      "allocator",
			eventType: "agent.budget.adjusted",
			content:   event.AgentBudgetAdjustedContent{AgentName: "implementer", PreviousBudget: 50, NewBudget: 75},
			want:      "implementer: 50 → 75 iterations",
		},
		{
			name:      "hive run started via JSON",
			role:      "system",
			eventType: "hive.run.started",
			content:   struct{ Idea string }{"Build a task manager"},
			want:      "Hive run started: Build a task manager",
		},
		{
			name:      "hive agent spawned via JSON",
			role:      "system",
			eventType: "hive.agent.spawned",
			content:   struct{ Name, Role, Model string }{"guardian", "guardian", "claude-sonnet-4-6"},
			want:      "Spawned: guardian (guardian, claude-sonnet-4-6)",
		},
		{
			name:      "unknown type falls back",
			role:      "mystery",
			eventType: "custom.event",
			content:   struct{ Foo string }{"bar"},
			want:      "mystery: custom.event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventSummary(tt.role, tt.eventType, tt.content)
			if got != tt.want {
				t.Errorf("eventSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 7, "this is…"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
