package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParseEvolveRecommendationBareJSON verifies that a clean JSON response
// is parsed directly into an EvolveRecommendation.
func TestParseEvolveRecommendationBareJSON(t *testing.T) {
	input := `{"description":"add agent communication channels","files_to_change":["pkg/pipeline/pipeline.go"],"new_files":["pkg/channels/channels.go"],"expected_impact":"agents can coordinate without polling","priority":"high","category":"feature","skip_reason":""}`
	rec, err := parseEvolveRecommendation(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Description != "add agent communication channels" {
		t.Errorf("description = %q, want %q", rec.Description, "add agent communication channels")
	}
	if len(rec.FilesToChange) != 1 || rec.FilesToChange[0] != "pkg/pipeline/pipeline.go" {
		t.Errorf("files_to_change = %v, want [pkg/pipeline/pipeline.go]", rec.FilesToChange)
	}
	if len(rec.NewFiles) != 1 || rec.NewFiles[0] != "pkg/channels/channels.go" {
		t.Errorf("new_files = %v, want [pkg/channels/channels.go]", rec.NewFiles)
	}
	if rec.Priority != "high" {
		t.Errorf("priority = %q, want %q", rec.Priority, "high")
	}
	if rec.Category != "feature" {
		t.Errorf("category = %q, want %q", rec.Category, "feature")
	}
	if rec.SkipReason != "" {
		t.Errorf("skip_reason = %q, want empty", rec.SkipReason)
	}
}

// TestParseEvolveRecommendationMarkdownBlock verifies extraction from a
// markdown fenced code block — LLMs often wrap JSON in ```json ... ```.
func TestParseEvolveRecommendationMarkdownBlock(t *testing.T) {
	input := "Here is my recommendation:\n```json\n{\"description\":\"improve CTO prompts\",\"files_to_change\":[\"pkg/roles/roles.go\"],\"new_files\":[],\"expected_impact\":\"better feature proposals\",\"priority\":\"medium\",\"category\":\"capability\",\"skip_reason\":\"\"}\n```\n"
	rec, err := parseEvolveRecommendation(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Description != "improve CTO prompts" {
		t.Errorf("description = %q, want %q", rec.Description, "improve CTO prompts")
	}
	if rec.Category != "capability" {
		t.Errorf("category = %q, want %q", rec.Category, "capability")
	}
}

// TestParseEvolveRecommendationSkipReason verifies that a skip_reason causes
// the returned recommendation to carry that reason (pipeline stops).
func TestParseEvolveRecommendationSkipReason(t *testing.T) {
	input := `{"description":"","files_to_change":[],"new_files":[],"expected_impact":"","priority":"low","category":"","skip_reason":"everything is already implemented"}`
	rec, err := parseEvolveRecommendation(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.SkipReason != "everything is already implemented" {
		t.Errorf("skip_reason = %q, want %q", rec.SkipReason, "everything is already implemented")
	}
}

// TestParseEvolveRecommendationProseWithFiles verifies the prose fallback:
// when JSON parsing fails but the response contains pkg/... file paths, they
// are extracted and the first paragraph becomes the description.
func TestParseEvolveRecommendationProseWithFiles(t *testing.T) {
	input := "We should improve the Work Graph implementation.\n\nThe main files to touch are pkg/work/task.go and cmd/hive/main.go.\nThis will enable better task tracking."
	rec, err := parseEvolveRecommendation(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Description == "" {
		t.Error("expected non-empty description from prose fallback")
	}
	found := false
	for _, f := range rec.FilesToChange {
		if f == "pkg/work/task.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("prose fallback missing pkg/work/task.go; got %v", rec.FilesToChange)
	}
}

// TestParseEvolveRecommendationNoFilesInProse verifies that a plain prose
// response with no recognisable file paths becomes a skip (SkipReason set).
func TestParseEvolveRecommendationNoFilesInProse(t *testing.T) {
	input := "I think everything looks fine and there is nothing to build right now."
	rec, err := parseEvolveRecommendation(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.SkipReason == "" {
		t.Error("expected SkipReason to be set when no parseable content found")
	}
	if rec.Description != "" {
		t.Errorf("description should be empty for skip; got %q", rec.Description)
	}
}

// TestParseEvolveProse exercises the prose parser directly.
func TestParseEvolveProse(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantFiles     []string
		wantNoFiles   bool
		wantDescEmpty bool
	}{
		{
			name:      "pkg path extracted",
			input:     "Improve the guardian.\n\nModify pkg/pipeline/pipeline.go to add checks.",
			wantFiles: []string{"pkg/pipeline/pipeline.go"},
		},
		{
			name:      "cmd path extracted",
			input:     "Update the CLI entry point cmd/hive/main.go for better flags.",
			wantFiles: []string{"cmd/hive/main.go"},
		},
		{
			name:      "multiple paths, deduplication",
			input:     "pkg/roles/roles.go and pkg/roles/roles.go again, plus pkg/spawn/spawn.go.",
			wantFiles: []string{"pkg/roles/roles.go", "pkg/spawn/spawn.go"},
		},
		{
			name:        "no paths returns empty",
			input:       "Nothing to change here at all.",
			wantNoFiles: true,
		},
		{
			name:  "first non-empty non-header line is description",
			input: "Add better telemetry.\n\nChange pkg/pipeline/telemetry.go to record more data.",
			wantFiles: []string{"pkg/pipeline/telemetry.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := parseEvolveProse(tt.input)

			if tt.wantNoFiles {
				if len(rec.FilesToChange) != 0 || rec.Description != "" {
					t.Errorf("expected empty result; got description=%q files=%v", rec.Description, rec.FilesToChange)
				}
				return
			}

			fileSet := make(map[string]bool, len(rec.FilesToChange))
			for _, f := range rec.FilesToChange {
				fileSet[f] = true
			}
			for _, want := range tt.wantFiles {
				if !fileSet[want] {
					t.Errorf("missing file %q in %v", want, rec.FilesToChange)
				}
			}

			// Deduplication check.
			seen := make(map[string]bool)
			for _, f := range rec.FilesToChange {
				if seen[f] {
					t.Errorf("duplicate file %q", f)
				}
				seen[f] = true
			}

			if rec.Description == "" {
				t.Error("expected non-empty description")
			}
			if rec.Priority != "high" {
				t.Errorf("priority = %q, want %q", rec.Priority, "high")
			}
			if rec.Category != "capability" {
				t.Errorf("category = %q, want %q", rec.Category, "capability")
			}
		})
	}
}

// TestFilterEvolveFiles verifies that filterEvolveFiles keeps .go files and
// key config files, and drops everything else.
func TestFilterEvolveFiles(t *testing.T) {
	input := map[string]string{
		"pkg/pipeline/pipeline.go":      "package pipeline",
		"pkg/pipeline/pipeline_test.go": "package pipeline",
		"cmd/hive/main.go":              "package main",
		"CLAUDE.md":                     "# Hive",
		"go.mod":                        "module github.com/lovyou-ai/hive",
		"SPEC.md":                       "# Spec",
		"README.md":                     "# README",
		"Dockerfile":                    "FROM golang",
		"scripts/build.sh":              "#!/bin/bash",
		"docs/design.md":                "## Design",  // not a key config file
		".github/workflows/ci.yml":      "on: push",
		"assets/logo.png":               "binary",
	}

	got := filterEvolveFiles(input)

	mustInclude := []string{
		"pkg/pipeline/pipeline.go",
		"pkg/pipeline/pipeline_test.go",
		"cmd/hive/main.go",
		"CLAUDE.md",
		"go.mod",
		"SPEC.md",
		"README.md",
	}
	mustExclude := []string{
		"Dockerfile",
		"scripts/build.sh",
		"docs/design.md",
		".github/workflows/ci.yml",
		"assets/logo.png",
	}

	for _, path := range mustInclude {
		if _, ok := got[path]; !ok {
			t.Errorf("filterEvolveFiles missing %q", path)
		}
	}
	for _, path := range mustExclude {
		if _, ok := got[path]; ok {
			t.Errorf("filterEvolveFiles should not include %q", path)
		}
	}
}

// TestFilterEvolveFilesEmpty verifies behaviour on an empty input map.
func TestFilterEvolveFilesEmpty(t *testing.T) {
	got := filterEvolveFiles(map[string]string{})
	if len(got) != 0 {
		t.Errorf("got %d files, want 0", len(got))
	}
}

// TestEvolveStateRoundTrip verifies load/save/clear state persistence.
func TestEvolveStateRoundTrip(t *testing.T) {
	root := t.TempDir()

	// Load on missing file returns empty state (no error).
	state, err := loadEvolveState(root)
	if err != nil {
		t.Fatalf("loadEvolveState (missing): %v", err)
	}
	if state == nil {
		t.Fatal("loadEvolveState returned nil state")
	}
	if state.LastIteration != 0 || len(state.Completed) != 0 || len(state.Failed) != 0 {
		t.Errorf("unexpected non-zero state on fresh load: %+v", state)
	}

	// Populate and save.
	now := time.Now().UTC().Truncate(time.Second)
	original := &EvolveState{
		StartedAt:     now,
		LastIteration: 2,
		TotalCost:     1.23,
		Completed: []EvolveRecommendation{
			{Description: "add Work Graph", Priority: "high", Category: "feature", ExpectedImpact: "task tracking"},
		},
		Failed: []EvolveRecommendation{
			{Description: "add Market Graph", Priority: "medium", Category: "feature", SkipReason: "compile error"},
		},
	}
	if err := saveEvolveState(root, original); err != nil {
		t.Fatalf("saveEvolveState: %v", err)
	}

	// Verify file exists at expected path.
	stateFile := evolveStatePath(root)
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("state file not created at %s: %v", stateFile, err)
	}

	// Verify JSON is valid and contains expected fields.
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}

	// Load back and compare.
	loaded, err := loadEvolveState(root)
	if err != nil {
		t.Fatalf("loadEvolveState (after save): %v", err)
	}
	if loaded.LastIteration != original.LastIteration {
		t.Errorf("LastIteration = %d, want %d", loaded.LastIteration, original.LastIteration)
	}
	if loaded.TotalCost != original.TotalCost {
		t.Errorf("TotalCost = %f, want %f", loaded.TotalCost, original.TotalCost)
	}
	if len(loaded.Completed) != 1 || loaded.Completed[0].Description != "add Work Graph" {
		t.Errorf("Completed = %v, want [{add Work Graph ...}]", loaded.Completed)
	}
	if len(loaded.Failed) != 1 || loaded.Failed[0].Description != "add Market Graph" {
		t.Errorf("Failed = %v, want [{add Market Graph ...}]", loaded.Failed)
	}

	// Clear and verify file is gone.
	if err := clearEvolveState(root); err != nil {
		t.Fatalf("clearEvolveState: %v", err)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Errorf("state file still exists after clear: %v", err)
	}

	// Clear again — must be idempotent (no error on missing file).
	if err := clearEvolveState(root); err != nil {
		t.Fatalf("clearEvolveState (idempotent): %v", err)
	}

	// Load after clear returns fresh empty state.
	afterClear, err := loadEvolveState(root)
	if err != nil {
		t.Fatalf("loadEvolveState (after clear): %v", err)
	}
	if afterClear.LastIteration != 0 || len(afterClear.Completed) != 0 {
		t.Errorf("unexpected state after clear: %+v", afterClear)
	}
}

// TestEvolveStatePathLocation verifies the state file path is under .hive/.
func TestEvolveStatePathLocation(t *testing.T) {
	got := evolveStatePath("/tmp/myrepo")
	want := filepath.Join("/tmp/myrepo", ".hive", "evolve-state.json")
	if got != want {
		t.Errorf("evolveStatePath = %q, want %q", got, want)
	}
}

// TestEvolveStateSaveCreatesDir verifies saveEvolveState creates .hive/ if absent.
func TestEvolveStateSaveCreatesDir(t *testing.T) {
	root := t.TempDir()
	// .hive dir does not exist yet.
	state := &EvolveState{LastIteration: 1}
	if err := saveEvolveState(root, state); err != nil {
		t.Fatalf("saveEvolveState: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hive")); err != nil {
		t.Errorf(".hive dir not created: %v", err)
	}
}

// TestEvolveCTOModelDefault verifies the default model is Sonnet.
func TestEvolveCTOModelDefault(t *testing.T) {
	p := &Pipeline{ctoModel: ""}
	model := p.evolveCTOModel()
	if model != "claude-sonnet-4-6" {
		t.Errorf("default evolve CTO model = %q, want %q", model, "claude-sonnet-4-6")
	}
}

// TestEvolveCTOModelOverride verifies that Config.CTOModel propagates.
func TestEvolveCTOModelOverride(t *testing.T) {
	p := &Pipeline{ctoModel: "claude-opus-4-6"}
	model := p.evolveCTOModel()
	if model != "claude-opus-4-6" {
		t.Errorf("overridden evolve CTO model = %q, want %q", model, "claude-opus-4-6")
	}
}
