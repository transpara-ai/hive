package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/api"
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

	t.Run("bold-colon-outside variant **KEY**:", func(t *testing.T) {
		input := "**COVER**: Shipped auth fix.\n**BLIND**: No rollback.\n**ZOOM**: Pattern.\n**FORMALIZE**: No new lesson."
		got := parseReflectorOutput(input)
		if !strings.Contains(got["COVER"], "Shipped auth fix") {
			t.Errorf("COVER = %q", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "No rollback") {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
	})

	t.Run("bold-space-colon variant **KEY** :", func(t *testing.T) {
		input := "**COVER** : Done.\n**BLIND** : Missed.\n**ZOOM** : Big picture.\n**FORMALIZE** : No new lesson."
		got := parseReflectorOutput(input)
		if !strings.Contains(got["COVER"], "Done") {
			t.Errorf("COVER = %q", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "Missed") {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
	})

	t.Run("h3 heading ### KEY:", func(t *testing.T) {
		input := "### COVER:\nShipped.\n\n### BLIND:\nMissed.\n\n### ZOOM:\nBig picture.\n\n### FORMALIZE:\nNo new lesson."
		got := parseReflectorOutput(input)
		if !strings.Contains(got["COVER"], "Shipped") {
			t.Errorf("COVER = %q", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "Missed") {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
	})

	t.Run("h2 heading ## KEY:", func(t *testing.T) {
		input := "## COVER:\nShipped.\n\n## BLIND:\nMissed.\n\n## ZOOM:\nBig picture.\n\n## FORMALIZE:\nNo new lesson."
		got := parseReflectorOutput(input)
		if !strings.Contains(got["COVER"], "Shipped") {
			t.Errorf("COVER = %q", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "Missed") {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
	})

	t.Run("lowercase key:", func(t *testing.T) {
		input := "cover: Shipped.\nblind: Missed.\nzoom: Big picture.\nformalize: No new lesson."
		got := parseReflectorOutput(input)
		if !strings.Contains(got["COVER"], "Shipped") {
			t.Errorf("COVER = %q", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "Missed") {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
	})

	t.Run("flat JSON object", func(t *testing.T) {
		input := `{"cover":"Shipped the Goal entity pipeline.","blind":"No integration tests added.","zoom":"Entity iterations converging.","formalize":"Lesson 56: test each entity kind once."}`

		got := parseReflectorOutput(input)

		if !strings.Contains(got["COVER"], "Goal entity pipeline") {
			t.Errorf("COVER = %q, want 'Goal entity pipeline'", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "integration tests") {
			t.Errorf("BLIND = %q, want 'integration tests'", got["BLIND"])
		}
		if !strings.Contains(got["ZOOM"], "converging") {
			t.Errorf("ZOOM = %q, want 'converging'", got["ZOOM"])
		}
		if !strings.Contains(got["FORMALIZE"], "Lesson 56") {
			t.Errorf("FORMALIZE = %q, want 'Lesson 56'", got["FORMALIZE"])
		}
	})

	t.Run("wrapper JSON reflection field", func(t *testing.T) {
		input := `{"reflection":{"cover":"Shipped the auth fix.","blind":"No rollback plan.","zoom":"Auth hardening theme.","formalize":"No new lesson."}}`

		got := parseReflectorOutput(input)

		if !strings.Contains(got["COVER"], "auth fix") {
			t.Errorf("COVER = %q, want 'auth fix'", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "rollback") {
			t.Errorf("BLIND = %q, want 'rollback'", got["BLIND"])
		}
		if !strings.Contains(got["ZOOM"], "hardening") {
			t.Errorf("ZOOM = %q, want 'hardening'", got["ZOOM"])
		}
		if got["FORMALIZE"] != "No new lesson." {
			t.Errorf("FORMALIZE = %q, want 'No new lesson.'", got["FORMALIZE"])
		}
	})

	t.Run("prose preamble before JSON block", func(t *testing.T) {
		input := "Here is my reflection for this iteration.\n\n" +
			`{"cover":"Closed the event loop.","blind":"Memory store not tested.","zoom":"Infrastructure maturing.","formalize":"Lesson 57: close the loop before adding features."}`

		got := parseReflectorOutput(input)

		if !strings.Contains(got["COVER"], "event loop") {
			t.Errorf("COVER = %q, want 'event loop'", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "Memory store") {
			t.Errorf("BLIND = %q, want 'Memory store'", got["BLIND"])
		}
		if !strings.Contains(got["ZOOM"], "maturing") {
			t.Errorf("ZOOM = %q, want 'maturing'", got["ZOOM"])
		}
		if !strings.Contains(got["FORMALIZE"], "Lesson 57") {
			t.Errorf("FORMALIZE = %q, want 'Lesson 57'", got["FORMALIZE"])
		}
	})

	t.Run("long prose 4000+ chars before JSON block", func(t *testing.T) {
		// Regression: scan-for-first-'{' path must handle large prose preambles without
		// losing the JSON block buried deep in the response.
		sentence := "This iteration we explored the structure of the event graph and the composition grammar. The scout identified a gap in the Work layer. The builder addressed it by adding new entity kinds. "
		preamble := strings.Repeat(sentence, 25) // ~4650 chars
		if len(preamble) < 4000 {
			t.Fatalf("test setup error: preamble is only %d chars, need 4000+", len(preamble))
		}
		jsonBlock := `{"cover":"Closed the event loop.","blind":"Memory store not tested.","zoom":"Infrastructure maturing.","formalize":"Lesson 57: close the loop before adding features."}`
		input := preamble + "\n\n" + jsonBlock

		got := parseReflectorOutput(input)

		if !strings.Contains(got["COVER"], "event loop") {
			t.Errorf("COVER = %q, want 'event loop'", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "Memory store") {
			t.Errorf("BLIND = %q, want 'Memory store'", got["BLIND"])
		}
		if !strings.Contains(got["ZOOM"], "maturing") {
			t.Errorf("ZOOM = %q, want 'maturing'", got["ZOOM"])
		}
		if !strings.Contains(got["FORMALIZE"], "Lesson 57") {
			t.Errorf("FORMALIZE = %q, want 'Lesson 57'", got["FORMALIZE"])
		}
	})

	t.Run("mixed formats boundary detection", func(t *testing.T) {
		// COVER uses **COVER:**, BLIND uses ## BLIND: — tests that the boundary
		// for COVER is found even though BLIND uses a different format.
		input := "**COVER:** Shipped the entity pipeline.\n\n## BLIND:\nNo tests written.\n\n### ZOOM:\nPattern converging.\n\n**FORMALIZE**: No new lesson."
		got := parseReflectorOutput(input)
		if !strings.Contains(got["COVER"], "Shipped the entity pipeline") {
			t.Errorf("COVER = %q", got["COVER"])
		}
		// COVER must NOT bleed into BLIND's content
		if strings.Contains(got["COVER"], "No tests written") {
			t.Errorf("COVER bled into BLIND: %q", got["COVER"])
		}
		if !strings.Contains(got["BLIND"], "No tests written") {
			t.Errorf("BLIND = %q", got["BLIND"])
		}
		if !strings.Contains(got["ZOOM"], "Pattern converging") {
			t.Errorf("ZOOM = %q", got["ZOOM"])
		}
		if !strings.Contains(got["FORMALIZE"], "No new lesson") {
			t.Errorf("FORMALIZE = %q", got["FORMALIZE"])
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

	// Must contain the four JSON field names the Reflector is expected to produce.
	for _, field := range []string{"cover", "blind", "zoom", "formalize"} {
		if !contains(prompt, field) {
			t.Errorf("prompt missing JSON field: %s", field)
		}
	}

	// Must instruct on conciseness and the BLIND priority.
	if !contains(prompt, "BLIND is the most important") {
		t.Error("prompt should highlight BLIND as most important")
	}

	// Format constraint must be front-loaded — appears before the first artifact section.
	// This prevents "lost in the middle" failures on long prompts.
	formatIdx := strings.Index(prompt, "Return ONLY")
	scoutIdx := strings.Index(prompt, "## Scout Report")
	if formatIdx < 0 {
		t.Error("prompt missing 'Return ONLY' format constraint")
	} else if scoutIdx >= 0 && formatIdx > scoutIdx {
		t.Errorf("format constraint (pos %d) appears after scout section (pos %d) — must be front-loaded", formatIdx, scoutIdx)
	}

	// Format constraint must precede the very first ## section header in the prompt,
	// not just ## Scout Report. This confirms the constraint is front-loaded before
	// ALL context, including Institutional Knowledge.
	firstHeaderIdx := strings.Index(prompt, "\n## ")
	if firstHeaderIdx < 0 {
		t.Error("prompt has no ## section headers")
	} else if formatIdx >= 0 && formatIdx > firstHeaderIdx {
		t.Errorf("format constraint (pos %d) appears after first ## header (pos %d) — must be front-loaded before all context sections", formatIdx, firstHeaderIdx)
	}
}

func TestTruncateArtifact(t *testing.T) {
	t.Run("short string unchanged", func(t *testing.T) {
		s := "short"
		if got := truncateArtifact(s, 100); got != s {
			t.Errorf("got %q, want %q", got, s)
		}
	})

	t.Run("exact length unchanged", func(t *testing.T) {
		s := strings.Repeat("x", 100)
		if got := truncateArtifact(s, 100); got != s {
			t.Errorf("expected unchanged at exact limit")
		}
	})

	t.Run("over limit truncated with marker", func(t *testing.T) {
		s := strings.Repeat("a", 200)
		got := truncateArtifact(s, 100)
		if !strings.HasSuffix(got, "\n... (truncated)") {
			t.Errorf("missing truncation marker: %q", got)
		}
		if strings.Contains(got[:100], "... (truncated)") {
			t.Error("truncation marker must appear after the kept content")
		}
		// Kept content must be exactly max bytes long.
		kept := strings.TrimSuffix(strings.TrimSuffix(got, "\n... (truncated)"), "\n... (truncated)")
		if len(kept) != 100 {
			t.Errorf("kept content length = %d, want 100", len(kept))
		}
	})

	t.Run("empty string unchanged", func(t *testing.T) {
		if got := truncateArtifact("", 100); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
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
	var foundEvent PhaseEvent
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var e PhaseEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %v", err)
		}
		if e.Phase == "reflector" && e.Outcome == "empty_sections" {
			found = true
			foundEvent = e
			break
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !found {
		t.Errorf("no PhaseEvent with phase=reflector outcome=empty_sections in diagnostics.jsonl:\n%s", data)
	}
	if foundEvent.Preview == "" {
		t.Errorf("PhaseEvent.Preview must be non-empty so PM can diagnose the format failure without re-running")
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

func TestRunReflectorEmptySectionsNoSideEffects(t *testing.T) {
	// Verify that an early return on empty_sections prevents BOTH side effects:
	// reflections.md must not be created and state.md must not be modified.
	stateContent := "# Loop State\n\nLast updated: Iteration 100, 2026-03-27.\n"
	hiveDir := makeHiveDir(t, stateContent, nil)

	// Response missing BLIND — triggers empty_sections early return.
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

	// reflections.md must NOT exist.
	reflPath := filepath.Join(hiveDir, "loop", "reflections.md")
	if _, err := os.Stat(reflPath); err == nil {
		t.Error("reflections.md must not exist after empty_sections early return")
	}

	// state.md must still contain the original iteration counter.
	stateData, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md"))
	if err != nil {
		t.Fatalf("state.md not readable: %v", err)
	}
	if !strings.Contains(string(stateData), "Iteration 100,") {
		t.Errorf("state.md iteration counter was advanced despite empty_sections early return — got:\n%s", string(stateData))
	}
}

func TestRunReflectorReviseBlocked(t *testing.T) {
	stateContent := "# Loop State\n\nLast updated: Iteration 42, 2026-03-27.\n"
	artifacts := map[string]string{
		"critique.md": "The code is missing error handling.\n\nVERDICT: REVISE",
	}
	hiveDir := makeHiveDir(t, stateContent, artifacts)

	r := &Runner{
		cfg: Config{
			HiveDir:  hiveDir,
			OneShot:  true,
			Provider: &mockProvider{response: "should not be called"},
		},
		tick: 1,
	}

	r.runReflector(context.Background())

	// reflections.md must NOT be created.
	reflPath := filepath.Join(hiveDir, "loop", "reflections.md")
	if _, err := os.Stat(reflPath); err == nil {
		t.Error("reflections.md must not exist when critique contains VERDICT: REVISE")
	}

	// state.md must still contain the original iteration number.
	stateData, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md"))
	if err != nil {
		t.Fatalf("state.md not readable: %v", err)
	}
	if !strings.Contains(string(stateData), "Iteration 42,") {
		t.Errorf("state.md iteration counter was advanced despite VERDICT: REVISE — got:\n%s", string(stateData))
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

// ─── Causality tests added in iteration 374 ──────────────────────────────────

// TestAppendReflectionPassesCauseIDs verifies that appendReflection forwards
// causeIDs to CreateDocument. Reflections must declare the iteration artifacts
// that produced them (Invariant 2: CAUSALITY).
func TestAppendReflectionPassesCauseIDs(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"op":"intend","node":{"id":"doc-1","kind":"document","title":"Reflection","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := &Runner{
		cfg: Config{
			HiveDir:   hiveDir,
			SpaceSlug: "hive",
			APIClient: api.New(srv.URL, "test-key"),
		},
	}

	causeIDs := []string{"build-node-42", "critique-node-99"}
	entry := "## 2026-03-28\n\n**COVER:** shipped something\n"
	if err := r.appendReflection(entry, causeIDs); err != nil {
		t.Fatalf("appendReflection error: %v", err)
	}

	rawCauses, ok := received["causes"]
	if !ok {
		t.Fatal("CreateDocument request missing 'causes' field — Invariant 2 violated")
	}
	causes, ok := rawCauses.([]any)
	if !ok || len(causes) != 2 {
		t.Fatalf("causes = %v, want 2-element array", rawCauses)
	}
	if causes[0] != "build-node-42" {
		t.Errorf("causes[0] = %v, want %q", causes[0], "build-node-42")
	}
	if causes[1] != "critique-node-99" {
		t.Errorf("causes[1] = %v, want %q", causes[1], "critique-node-99")
	}
}

// TestAppendReflectionNilCausesOmitsCausesField verifies that appendReflection
// does NOT send a causes field when causeIDs is nil — avoids sending empty arrays.
func TestAppendReflectionNilCausesOmitsCausesField(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"op":"intend","node":{"id":"doc-2","kind":"document","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	hiveDir := makeHiveDir(t, "# State\n", nil)
	r := &Runner{
		cfg: Config{
			HiveDir:   hiveDir,
			SpaceSlug: "hive",
			APIClient: api.New(srv.URL, "test-key"),
		},
	}

	if err := r.appendReflection("## 2026-03-28\n**COVER:** x\n", nil); err != nil {
		t.Fatalf("appendReflection error: %v", err)
	}

	if _, hasCauses := received["causes"]; hasCauses {
		t.Error("causes field must be absent when causeIDs is nil")
	}
}

// TestReadFromGraphNodeStalenessFilter verifies that readFromGraphNode filters
// out nodes older than 2 hours to avoid surfacing stale data from prior cycles.
func TestReadFromGraphNodeStalenessFilter(t *testing.T) {
	freshAt := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	staleAt := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)

	makeDocServer := func(t *testing.T, createdAt string) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.Contains(r.URL.Path, "/documents"):
				json.NewEncoder(w).Encode(map[string]any{
					"documents": []map[string]any{
						{
							"id": "node-xyz", "title": "Build: the thing",
							"kind": "document", "body": "some body",
							"created_at": createdAt, "updated_at": createdAt,
						},
					},
				})
			default:
				// board and knowledge endpoints return empty
				json.NewEncoder(w).Encode(map[string]any{"nodes": []any{}, "claims": []any{}})
			}
		}))
	}

	t.Run("fresh node (30 min old) is returned", func(t *testing.T) {
		srv := makeDocServer(t, freshAt)
		defer srv.Close()

		r := &Runner{cfg: Config{SpaceSlug: "hive", APIClient: api.New(srv.URL, "test-key")}}
		node := r.readFromGraphNode("Build:")
		if node == nil {
			t.Fatal("expected fresh node to be returned, got nil")
		}
		if node.ID != "node-xyz" {
			t.Errorf("node.ID = %q, want %q", node.ID, "node-xyz")
		}
	})

	t.Run("stale node (3 hours old) is filtered out", func(t *testing.T) {
		srv := makeDocServer(t, staleAt)
		defer srv.Close()

		r := &Runner{cfg: Config{SpaceSlug: "hive", APIClient: api.New(srv.URL, "test-key")}}
		node := r.readFromGraphNode("Build:")
		if node != nil {
			t.Errorf("expected nil for stale node, got node.ID=%q", node.ID)
		}
	})

	t.Run("nil APIClient returns nil without panic", func(t *testing.T) {
		r := &Runner{cfg: Config{SpaceSlug: "hive"}}
		node := r.readFromGraphNode("Build:")
		if node != nil {
			t.Errorf("expected nil when APIClient is nil, got %+v", node)
		}
	})
}

// TestRunReflectorReasonLessonNumberFromGraph verifies that runReflectorReason
// derives the lesson number from the knowledge claims endpoint rather than
// state.md. This prevents duplicate lesson numbers when runs overlap or retry.
func TestRunReflectorReasonLessonNumberFromGraph(t *testing.T) {
	var assertedTitle string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.RawQuery, "op=max_lesson"):
			// Server-side aggregate: highest lesson number is 109.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"max_lesson":109}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/op"):
			// Capture the op request to inspect the lesson title.
			var fields map[string]any
			data, _ := io.ReadAll(r.Body)
			json.Unmarshal(data, &fields)
			if op, _ := fields["op"].(string); op == "assert" {
				assertedTitle, _ = fields["title"].(string)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"op":"assert","node":{"id":"claim-1","title":"","created_at":"","updated_at":""}}`))
		default:
			// Board, documents, and other endpoints return empty.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"nodes":[],"documents":[],"claims":[],"posts":[]}`))
		}
	}))
	defer srv.Close()

	stateContent := "# Loop State\n\nLast updated: Iteration 109, 2026-03-29.\n"
	artifacts := map[string]string{
		"scout.md":    "## Scout\nGap: test",
		"build.md":    "## Build\nFixed test",
		"critique.md": "VERDICT: PASS",
	}
	hiveDir := makeHiveDir(t, stateContent, artifacts)

	llmResponse := `{"cover":"Shipped test fix.","blind":"No rollback.","zoom":"Stable.","formalize":"Lesson 110: always query the graph for lesson numbers."}`

	r := &Runner{
		cfg: Config{
			HiveDir:   hiveDir,
			OneShot:   true,
			SpaceSlug: "hive",
			APIClient: api.New(srv.URL, "test-key"),
			Provider:  &mockProvider{response: llmResponse},
		},
		tick: 1,
	}

	r.runReflector(context.Background())

	// The asserted claim title must use graph-queried number 110 (= 109 + 1),
	// not any number from state.md.
	if assertedTitle == "" {
		t.Fatal("no assert op was sent — lesson was not posted to graph")
	}
	if !strings.HasPrefix(assertedTitle, "Lesson 110:") {
		t.Errorf("lesson title = %q, want prefix %q — should use graph-queried number not state.md", assertedTitle, "Lesson 110:")
	}
}
