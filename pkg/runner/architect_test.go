package runner

import (
	"strings"
	"testing"
)

func TestParseSubtasksMarkdown(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCount  int
		wantTitles []string
	}{
		{
			name: "numbered list with bold title and dash",
			input: `1. **Add event store migration** — Create the SQL migration file for the agent_memories table.
2. **Implement MemoryStore interface** — Add pkg/store/memory_store.go with the interface and pgMemoryStore.
3. **Wire MemoryStore into agent loop** — Update loop.go to inject the store before each Reason() call.`,
			wantCount:  3,
			wantTitles: []string{"Add event store migration", "Implement MemoryStore interface", "Wire MemoryStore into agent loop"},
		},
		{
			name: "heading format",
			input: `### Add persona resolver
Implement pkg/store/persona_store.go resolving actor IDs to display names at render time.

### Wire resolver into templates
Update the render pipeline to call persona_store before writing HTML.`,
			wantCount:  2,
			wantTitles: []string{"Add persona resolver", "Wire resolver into templates"},
		},
		{
			name: "bullet with bold title",
			input: `- **Add budget enforcement** — Extend pkg/resources/budget.go with daily cap logic.
- **Hook budget into runner** — Call budget.Check() before each Reason() call in runner.go.`,
			wantCount:  2,
			wantTitles: []string{"Add budget enforcement", "Hook budget into runner"},
		},
		{
			name:      "empty string",
			input:     "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubtasksMarkdown(tt.input)
			if len(got) != tt.wantCount {
				t.Fatalf("parseSubtasksMarkdown returned %d tasks, want %d\ninput:\n%s", len(got), tt.wantCount, tt.input)
			}
			for i, title := range tt.wantTitles {
				if got[i].title != title {
					t.Errorf("task[%d].title = %q, want %q", i, got[i].title, title)
				}
			}
		})
	}
}

func TestParseArchitectSubtasks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCount   int
		wantTitles  []string
		wantPrios   []string
		wantDescKey []string // substring each desc must contain
	}{
		{
			name: "canonical single-line descriptions",
			input: `SUBTASK_TITLE: Add agent_memories table migration
SUBTASK_PRIORITY: high
SUBTASK_DESCRIPTION: Create pkg/store/pgstore/migrations/add_agent_memories.sql with the agent_memories table schema.

SUBTASK_TITLE: Implement MemoryStore interface
SUBTASK_PRIORITY: high
SUBTASK_DESCRIPTION: Add pkg/store/memory_store.go defining MemoryStore interface and pgMemoryStore implementation.

SUBTASK_TITLE: Wire MemoryStore into agent loop
SUBTASK_PRIORITY: medium
SUBTASK_DESCRIPTION: Update pkg/loop/loop.go to inject MemoryStore into AgentContext before each Reason() call.`,
			wantCount:  3,
			wantTitles: []string{"Add agent_memories table migration", "Implement MemoryStore interface", "Wire MemoryStore into agent loop"},
			wantPrios:  []string{"high", "high", "medium"},
			wantDescKey: []string{
				"add_agent_memories.sql",
				"memory_store.go",
				"loop.go",
			},
		},
		{
			name: "multi-line description spanning two lines",
			input: `SUBTASK_TITLE: Refactor event bus subscription handling
SUBTASK_PRIORITY: high
SUBTASK_DESCRIPTION: Update pkg/hive/runtime.go to decouple subscription registration from agent spawning.
This ensures agents registered after bus.Start() receive events correctly.`,
			wantCount:  1,
			wantTitles: []string{"Refactor event bus subscription handling"},
			wantPrios:  []string{"high"},
			wantDescKey: []string{"agents registered after bus.Start() receive events correctly"},
		},
		{
			name:  "fence-wrapped response",
			input: "```\nSUBTASK_TITLE: Add persona resolver\nSUBTASK_PRIORITY: medium\nSUBTASK_DESCRIPTION: Implement pkg/store/persona_store.go resolving actor IDs to display names at render time.\n```",
			wantCount:  1,
			wantTitles: []string{"Add persona resolver"},
			wantPrios:  []string{"medium"},
			wantDescKey: []string{"persona_store.go"},
		},
		{
			name:      "empty string",
			input:     "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArchitectSubtasks(tt.input)
			if len(got) != tt.wantCount {
				t.Fatalf("parseArchitectSubtasks returned %d tasks, want %d\ninput:\n%s", len(got), tt.wantCount, tt.input)
			}
			for i, title := range tt.wantTitles {
				if got[i].title != title {
					t.Errorf("task[%d].title = %q, want %q", i, got[i].title, title)
				}
			}
			for i, prio := range tt.wantPrios {
				if got[i].priority != prio {
					t.Errorf("task[%d].priority = %q, want %q", i, got[i].priority, prio)
				}
			}
			for i, key := range tt.wantDescKey {
				if !strings.Contains(got[i].desc, key) {
					t.Errorf("task[%d].desc = %q, want it to contain %q", i, got[i].desc, key)
				}
			}
		})
	}
}
