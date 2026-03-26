package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// mockProvider is a test double for intelligence.Provider.
type mockProvider struct {
	response string
}

func (m *mockProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	score, _ := types.NewScore(0.9)
	return decision.NewResponse(m.response, score, decision.TokenUsage{}), nil
}

func (m *mockProvider) Name() string  { return "mock" }
func (m *mockProvider) Model() string { return "mock-model" }

func TestParseReflectorOutput(t *testing.T) {
	t.Run("bold markdown sections", func(t *testing.T) {
		input := `Here is my reflection.

**COVER:** We shipped the entity pipeline for Goal nodes. Connects to the prior work on Project.

**BLIND:** No tests were written for the new pipeline handler.

**ZOOM:** Three consecutive entity-kind iterations — pattern is converging on fixpoint.

**FORMALIZE:** Lesson 56: Entity pipelines share a single integration test template.`

		got := parseReflectorOutput(input)

		if !strings.Contains(got["COVER"], "entity pipeline for Goal") {
			t.Errorf("COVER = %q, want 'entity pipeline for Goal'", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "No tests") {
			t.Errorf("BLIND = %q, want 'No tests'", got["BLIND"])
		}
		if !strings.Contains(got["ZOOM"], "converging on fixpoint") {
			t.Errorf("ZOOM = %q, want 'converging on fixpoint'", got["ZOOM"])
		}
		if !strings.Contains(got["FORMALIZE"], "Lesson 56") {
			t.Errorf("FORMALIZE = %q, want 'Lesson 56'", got["FORMALIZE"])
		}
	})

	t.Run("plain KEY: sections", func(t *testing.T) {
		input := "COVER: Shipped the auth fix.\nBLIND: No rollback plan.\nZOOM: Auth hardening theme.\nFORMALIZE: No new lesson."

		got := parseReflectorOutput(input)

		if got["COVER"] != "Shipped the auth fix." {
			t.Errorf("COVER = %q", got["COVER"])
		}
		if got["BLIND"] != "No rollback plan." {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
		if got["ZOOM"] != "Auth hardening theme." {
			t.Errorf("ZOOM = %q", got["ZOOM"])
		}
		if got["FORMALIZE"] != "No new lesson." {
			t.Errorf("FORMALIZE = %q", got["FORMALIZE"])
		}
	})

	t.Run("missing sections return empty string", func(t *testing.T) {
		input := "**COVER:** Only this section present."

		got := parseReflectorOutput(input)

		if got["COVER"] == "" {
			t.Error("COVER should be non-empty")
		}
		if got["BLIND"] != "" {
			t.Errorf("BLIND should be empty, got %q", got["BLIND"])
		}
		if got["ZOOM"] != "" {
			t.Errorf("ZOOM should be empty, got %q", got["ZOOM"])
		}
		if got["FORMALIZE"] != "" {
			t.Errorf("FORMALIZE should be empty, got %q", got["FORMALIZE"])
		}
	})

	t.Run("empty input returns empty map", func(t *testing.T) {
		got := parseReflectorOutput("")
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})

	t.Run("section content is trimmed", func(t *testing.T) {
		input := "**COVER:**   padded content   \n**BLIND:** next"
		got := parseReflectorOutput(input)
		if got["COVER"] != "padded content" {
			t.Errorf("COVER not trimmed: %q", got["COVER"])
		}
	})
}

func TestBuildReflectorPrompt(t *testing.T) {
	prompt := buildReflectorPrompt(
		"## Scout\nGap: missing Goal entity",
		"## Build\nAdded KindGoal to store.go",
		"## Critique\nVERDICT: PASS",
		"## 2026-03-25\n**COVER:** shipped Project",
		"## Invariants\n1. IDENTITY",
	)

	// All artifact content must appear in the prompt.
	if !contains(prompt, "Gap: missing Goal entity") {
		t.Error("prompt missing scout content")
	}
	if !contains(prompt, "Added KindGoal to store.go") {
		t.Error("prompt missing build content")
	}
	if !contains(prompt, "VERDICT: PASS") {
		t.Error("prompt missing critique content")
	}
	if !contains(prompt, "shipped Project") {
		t.Error("prompt missing recent reflections")
	}
	if !contains(prompt, "## Invariants") {
		t.Error("prompt missing shared context")
	}

	// Must contain the four section headings the Reflector is expected to produce.
	for _, section := range []string{"COVER", "BLIND", "ZOOM", "FORMALIZE"} {
		if !contains(prompt, section) {
			t.Errorf("prompt missing section heading: %s", section)
		}
	}

	// Must instruct on conciseness and the BLIND priority.
	if !contains(prompt, "BLIND is the most important") {
		t.Error("prompt should highlight BLIND as most important")
	}
}

func TestFormatReflectionEntry(t *testing.T) {
	entry := formatReflectionEntry(
		"2026-03-26",
		"Shipped Goal entity kind.",
		"Integration tests not written.",
		"Entity pipeline iterations converging.",
		"Lesson 56: test the pipeline once per kind.",
	)

	// Must open with a date heading.
	if !strings.HasPrefix(entry, "## 2026-03-26") {
		preview := entry
		if len(preview) > 30 {
			preview = preview[:30]
		}
		t.Errorf("entry should start with '## 2026-03-26', got: %q", preview)
	}

	// Must contain all four labeled sections.
	for _, label := range []string{"**COVER:**", "**BLIND:**", "**ZOOM:**", "**FORMALIZE:**"} {
		if !contains(entry, label) {
			t.Errorf("entry missing label %s", label)
		}
	}

	// Must contain the supplied content.
	if !contains(entry, "Shipped Goal entity kind.") {
		t.Error("entry missing COVER content")
	}
	if !contains(entry, "Integration tests not written.") {
		t.Error("entry missing BLIND content")
	}
	if !contains(entry, "converging.") {
		t.Error("entry missing ZOOM content")
	}
	if !contains(entry, "Lesson 56") {
		t.Error("entry missing FORMALIZE content")
	}

	// Must end with a trailing newline (append-safe).
	if !strings.HasSuffix(entry, "\n") {
		t.Error("entry must end with newline for safe appending")
	}
}

// ─── TestRunReflector* — behavioral tests ────────────────────────────────────

// makeHiveDir creates a minimal hive directory structure with state.md and
// optional loop artifacts. Returns the path to the temp dir.
func makeHiveDir(t *testing.T, stateContent string, artifacts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	loopDir := filepath.Join(dir, "loop")
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatalf("mkdir loop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loopDir, "state.md"), []byte(stateContent), 0644); err != nil {
		t.Fatalf("write state.md: %v", err)
	}
	for name, content := range artifacts {
		if err := os.WriteFile(filepath.Join(loopDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestRunReflectorAppendsToReflections(t *testing.T) {
	stateContent := "# Loop State\n\nLast updated: Iteration 10, 2026-03-25.\n"
	artifacts := map[string]string{
		"scout.md":   "## Scout\nGap: missing reflector",
		"build.md":   "## Build\nAdded runReflector()",
		"critique.md": "VERDICT: PASS",
	}
	hiveDir := makeHiveDir(t, stateContent, artifacts)

	llmResponse := `**COVER:** Implemented runReflector closing the loop.

**BLIND:** No integration test for full pipeline run.

**ZOOM:** Infrastructure iteration — closed the open loop.

**FORMALIZE:** Lesson 57: The loop only learns when it writes back.`

	r := &Runner{
		cfg: Config{
			HiveDir: hiveDir,
			OneShot: true,
			Provider: &mockProvider{response: llmResponse},
		},
		tick: 1,
	}

	r.runReflector(context.Background())

	// reflections.md must exist and contain the four sections.
	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "reflections.md"))
	if err != nil {
		t.Fatalf("reflections.md not created: %v", err)
	}
	content := string(data)
	for _, want := range []string{"**COVER:**", "**BLIND:**", "**ZOOM:**", "**FORMALIZE:**"} {
		if !strings.Contains(content, want) {
			t.Errorf("reflections.md missing %s", want)
		}
	}
	if !strings.Contains(content, "Implemented runReflector") {
		t.Error("reflections.md missing COVER content")
	}
}

func TestRunReflectorAdvancesStateIteration(t *testing.T) {
	stateContent := "# Loop State\n\nLast updated: Iteration 232, 2026-03-25.\n\nRest of state."
	hiveDir := makeHiveDir(t, stateContent, nil)

	r := &Runner{
		cfg: Config{
			HiveDir: hiveDir,
			OneShot: true,
			Provider: &mockProvider{response: "**COVER:** x\n**BLIND:** y\n**ZOOM:** z\n**FORMALIZE:** No new lesson."},
		},
		tick: 1,
	}

	r.runReflector(context.Background())

	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md"))
	if err != nil {
		t.Fatalf("state.md read error: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "Iteration 232,") {
		t.Error("state.md still has old iteration 232 — counter not incremented")
	}
	if !strings.Contains(content, "Iteration 233,") {
		t.Errorf("state.md does not contain 'Iteration 233,' — got:\n%s", content)
	}
}

func TestRunReflectorMissingArtifactsNoError(t *testing.T) {
	// No scout.md, build.md, or critique.md — only state.md.
	stateContent := "# Loop State\n\nLast updated: Iteration 1, 2026-03-01.\n"
	hiveDir := makeHiveDir(t, stateContent, nil)

	r := &Runner{
		cfg: Config{
			HiveDir: hiveDir,
			OneShot: true,
			Provider: &mockProvider{response: "**COVER:** nothing\n**BLIND:** n/a\n**ZOOM:** n/a\n**FORMALIZE:** No new lesson."},
		},
		tick: 1,
	}

	// Must not panic.
	r.runReflector(context.Background())

	// state.md should still be updated.
	data, _ := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md"))
	if !strings.Contains(string(data), "Iteration 2,") {
		t.Error("state.md iteration not advanced even when artifacts are missing")
	}
}

func TestRunReflectorEmptySectionsDiagnostic(t *testing.T) {
	stateContent := "# Loop State\n\nLast updated: Iteration 5, 2026-03-25.\n"
	hiveDir := makeHiveDir(t, stateContent, nil)

	// Response has BLIND empty — only COVER, ZOOM, FORMALIZE present.
	llmResponse := "**COVER:** Shipped something.\n\n**ZOOM:** Zoomed out.\n\n**FORMALIZE:** No new lesson."

	r := &Runner{
		cfg: Config{
			HiveDir:  hiveDir,
			OneShot:  true,
			Provider: &mockProvider{response: llmResponse},
		},
		tick: 1,
	}

	r.runReflector(context.Background())

	// diagnostics.jsonl must exist and contain a PhaseEvent with outcome="empty_sections".
	path := filepath.Join(hiveDir, "loop", "diagnostics.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("diagnostics.jsonl not created: %v", err)
	}

	var found bool
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var e PhaseEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %v", err)
		}
		if e.Phase == "reflector" && e.Outcome == "empty_sections" {
			found = true
			break
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !found {
		t.Errorf("no PhaseEvent with phase=reflector outcome=empty_sections in diagnostics.jsonl:\n%s", data)
	}

	// The early return must prevent reflections.md from being written.
	reflPath := filepath.Join(hiveDir, "loop", "reflections.md")
	if _, err := os.Stat(reflPath); err == nil {
		t.Errorf("reflections.md should not exist after empty_sections early return, but it does")
	}

	// The early return must prevent the iteration counter from advancing.
	stateData, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md"))
	if err != nil {
		t.Fatalf("state.md not readable: %v", err)
	}
	if !strings.Contains(string(stateData), "Iteration 5,") {
		t.Errorf("state.md iteration counter was advanced despite empty_sections early return — got:\n%s", string(stateData))
	}
}

func TestIncrementIterationLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		date    string
		wantN   int
		wantSub string
	}{
		{
			name:    "normal increment",
			content: "# State\n\nLast updated: Iteration 232, 2026-03-25.\n\nMore content.",
			date:    "2026-03-26",
			wantN:   233,
			wantSub: "Last updated: Iteration 233, 2026-03-26.",
		},
		{
			name:    "from zero",
			content: "Last updated: Iteration 0, 2026-01-01.\n",
			date:    "2026-01-02",
			wantN:   1,
			wantSub: "Last updated: Iteration 1, 2026-01-02.",
		},
		{
			name:    "no match — content unchanged",
			content: "No iteration line here.",
			date:    "2026-03-26",
			wantN:   0,
			wantSub: "No iteration line here.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := incrementIterationLine(tt.content, tt.date)
			if n != tt.wantN {
				t.Errorf("n = %d, want %d", n, tt.wantN)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("result missing %q\ngot: %s", tt.wantSub, got)
			}
		})
	}
}
