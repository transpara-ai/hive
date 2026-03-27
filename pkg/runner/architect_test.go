package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/api"
)

// mockCostProvider is a test double that returns a configurable TokenUsage.
type mockCostProvider struct {
	response string
	usage    decision.TokenUsage
}

func (m *mockCostProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	score, _ := types.NewScore(0.9)
	return decision.NewResponse(m.response, score, m.usage), nil
}
func (m *mockCostProvider) Name() string  { return "mock-cost" }
func (m *mockCostProvider) Model() string { return "mock-model" }

func TestRunArchitectParseFailureWritesDiagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", map[string]string{
		"scout.md": "## Scout\nGap: missing feature X",
	})

	r := New(Config{
		HiveDir:   hiveDir,
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		Provider: &mockCostProvider{
			response: "This response has no subtask markers at all.",
			usage:    decision.TokenUsage{InputTokens: 100, OutputTokens: 50, CostUSD: 0.0042},
		},
		OneShot: true,
	})

	r.runArchitect(context.Background())

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "diagnostics.jsonl"))
	if err != nil {
		t.Fatalf("diagnostics.jsonl not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, `"phase":"architect"`) {
		t.Errorf("diagnostics.jsonl missing phase=architect:\n%s", body)
	}
	if !strings.Contains(body, `"outcome":"failure"`) {
		t.Errorf("diagnostics.jsonl missing outcome=failure:\n%s", body)
	}
	if !strings.Contains(body, `"error"`) {
		t.Errorf("diagnostics.jsonl missing error field:\n%s", body)
	}
	if !strings.Contains(body, `"preview"`) {
		t.Errorf("diagnostics.jsonl missing preview field:\n%s", body)
	}
	if !strings.Contains(body, "no subtask markers") {
		t.Errorf("diagnostics.jsonl preview should contain LLM response content:\n%s", body)
	}
}

func TestRunArchitectParseFailurePreviewTruncatedAt2000(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", map[string]string{
		"scout.md": "## Scout\nGap: missing feature X",
	})

	// Build a response longer than 2000 chars with no subtask markers.
	longResponse := strings.Repeat("x", 2500)

	r := New(Config{
		HiveDir:   hiveDir,
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		Provider: &mockCostProvider{
			response: longResponse,
			usage:    decision.TokenUsage{InputTokens: 200, OutputTokens: 300, CostUSD: 0.005},
		},
		OneShot: true,
	})

	r.runArchitect(context.Background())

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "diagnostics.jsonl"))
	if err != nil {
		t.Fatalf("diagnostics.jsonl not written: %v", err)
	}

	var got PhaseEvent
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, data)
	}
	if len(got.Preview) != 2000 {
		t.Errorf("Preview length = %d, want 2000", len(got.Preview))
	}
}

func TestRunArchitectErrorFieldContainsLLMResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", map[string]string{
		"scout.md": "## Scout\nGap: missing feature X",
	})

	llmResponse := "This is the raw LLM response with no parseable subtasks."

	r := New(Config{
		HiveDir:   hiveDir,
		SpaceSlug: "test",
		APIClient: api.New(srv.URL, "test-key"),
		Provider: &mockCostProvider{
			response: llmResponse,
			usage:    decision.TokenUsage{InputTokens: 100, OutputTokens: 50, CostUSD: 0.0042},
		},
		OneShot: true,
	})

	r.runArchitect(context.Background())

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "diagnostics.jsonl"))
	if err != nil {
		t.Fatalf("diagnostics.jsonl not written: %v", err)
	}

	var got PhaseEvent
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatalf("invalid JSON: %v\ncontent: %s", err, data)
	}
	if got.Error != llmResponse {
		t.Errorf("Error field = %q, want %q", got.Error, llmResponse)
	}
}

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

func TestParseSubtasksJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCount  int
		wantTitles []string
		wantPrios  []string
	}{
		{
			name: "bare JSON array",
			input: `[
				{"title":"Add event store migration","description":"Create the SQL migration file.","priority":"high"},
				{"title":"Implement MemoryStore","description":"Add pkg/store/memory_store.go.","priority":"medium"}
			]`,
			wantCount:  2,
			wantTitles: []string{"Add event store migration", "Implement MemoryStore"},
			wantPrios:  []string{"high", "medium"},
		},
		{
			name: "tasks wrapper object",
			input: `{"tasks":[
				{"title":"Wire MemoryStore into loop","description":"Update loop.go.","priority":"high"}
			]}`,
			wantCount:  1,
			wantTitles: []string{"Wire MemoryStore into loop"},
			wantPrios:  []string{"high"},
		},
		{
			name:      "invalid JSON returns nil",
			input:     "not json at all",
			wantCount: 0,
		},
		{
			name:      "empty array returns nil",
			input:     "[]",
			wantCount: 0,
		},
		{
			name:      "empty string returns nil",
			input:     "",
			wantCount: 0,
		},
		{
			name:  "unknown priority defaults to high",
			input: `[{"title":"Do something","description":"desc","priority":"unknown"}]`,
			wantCount:  1,
			wantTitles: []string{"Do something"},
			wantPrios:  []string{"high"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubtasksJSON(tt.input)
			if len(got) != tt.wantCount {
				t.Fatalf("parseSubtasksJSON returned %d tasks, want %d\ninput:\n%s", len(got), tt.wantCount, tt.input)
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
		})
	}
}

func TestParseArchitectSubtasksJSON(t *testing.T) {
	// JSON input should be parsed before strict/markdown formats are tried.
	input := `[{"title":"Add persona resolver","description":"Implement pkg/store/persona_store.go.","priority":"high"}]`
	got := parseArchitectSubtasks(input)
	if len(got) != 1 {
		t.Fatalf("parseArchitectSubtasks returned %d tasks, want 1", len(got))
	}
	if got[0].title != "Add persona resolver" {
		t.Errorf("title = %q, want %q", got[0].title, "Add persona resolver")
	}
	if got[0].priority != "high" {
		t.Errorf("priority = %q, want %q", got[0].priority, "high")
	}
	if !strings.Contains(got[0].desc, "persona_store.go") {
		t.Errorf("desc = %q, want it to contain %q", got[0].desc, "persona_store.go")
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
			name:  "JSON array input",
			input: `[{"title":"Add budget enforcement","description":"Extend pkg/resources/budget.go with daily cap logic.","priority":"high"}]`,
			wantCount:  1,
			wantTitles: []string{"Add budget enforcement"},
			wantPrios:  []string{"high"},
			wantDescKey: []string{"budget.go"},
		},
		{
			name:  "JSON tasks wrapper",
			input: `{"tasks":[{"title":"Wire budget into runner","description":"Call budget.Check() before each Reason() call in runner.go.","priority":"medium"}]}`,
			wantCount:  1,
			wantTitles: []string{"Wire budget into runner"},
			wantPrios:  []string{"medium"},
			wantDescKey: []string{"runner.go"},
		},
		{
			name: "prose preamble before SUBTASK_TITLE markers",
			input: `Here is my plan for the next iteration. I've identified two subtasks that should be implemented sequentially.

SUBTASK_TITLE: Add event bus listener
SUBTASK_PRIORITY: high
SUBTASK_DESCRIPTION: Register a bus.Subscribe() handler in pkg/hive/runtime.go for work.task.created events.

SUBTASK_TITLE: Emit task events
SUBTASK_PRIORITY: medium
SUBTASK_DESCRIPTION: Update pkg/work/store.go to emit events on task creation.`,
			wantCount:  2,
			wantTitles: []string{"Add event bus listener", "Emit task events"},
			wantPrios:  []string{"high", "medium"},
			wantDescKey: []string{"runtime.go", "store.go"},
		},
		{
			// Exact format that caused the 06:08:12Z architect failure (commit c600069):
			// LLM wraps SUBTASK_ keys in bold with the colon inside the markers.
			name:  "bold-colon format: **SUBTASK_TITLE:** Title here",
			input: "**SUBTASK_TITLE:** Add budget enforcement\n**SUBTASK_PRIORITY:** high\n**SUBTASK_DESCRIPTION:** Extend pkg/resources/budget.go with daily cap logic.",
			wantCount:  1,
			wantTitles: []string{"Add budget enforcement"},
			wantPrios:  []string{"high"},
			wantDescKey: []string{"budget.go"},
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
