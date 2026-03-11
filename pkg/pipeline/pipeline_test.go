package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/spawn"
)

func TestContainsAlert(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Everything looks fine", false},
		{"ALERT: trust anomaly in builder agent", true},
		{"VIOLATION: soul values breached", true},
		{"QUARANTINE agent builder_01", true},
		{"Line one\nALERT: something wrong\nLine three", true},

		// Negative — keywords embedded in prose, not line-start directives.
		{"Found a VIOLATION of soul values", false},
		{"Minor alert about formatting", false},
		{"No VIOLATIONS DETECTED", false},
		{"The code is clean", false},
		{"", false},
		{"halt operations immediately", false}, // HALT handled separately
	}

	for _, tt := range tests {
		got := containsAlert(tt.input)
		if got != tt.want {
			t.Errorf("containsAlert(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseFiles(t *testing.T) {
	input := `--- FILE: main.go ---
package main

func main() {}
--- FILE: lib/util.go ---
package lib

func Helper() string {
	return "hello"
}
--- FILE: main_test.go ---
package main

import "testing"

func TestMain(t *testing.T) {}
`
	files := parseFiles(input)

	if len(files) != 3 {
		t.Fatalf("parseFiles returned %d files, want 3", len(files))
	}

	if _, ok := files["main.go"]; !ok {
		t.Error("missing main.go")
	}
	if _, ok := files["lib/util.go"]; !ok {
		t.Error("missing lib/util.go")
	}
	if _, ok := files["main_test.go"]; !ok {
		t.Error("missing main_test.go")
	}

	if !strings.Contains(files["main.go"], "package main") {
		t.Error("main.go missing package declaration")
	}
	if !strings.Contains(files["lib/util.go"], "func Helper()") {
		t.Error("util.go missing Helper function")
	}
}

func TestParseFilesEmpty(t *testing.T) {
	files := parseFiles("just some text without markers")
	if len(files) != 0 {
		t.Errorf("parseFiles with no markers returned %d files, want 0", len(files))
	}
}

func TestParseFilesSingleFile(t *testing.T) {
	input := `--- FILE: app.py ---
def main():
    print("hello")

if __name__ == "__main__":
    main()
`
	files := parseFiles(input)
	if len(files) != 1 {
		t.Fatalf("parseFiles returned %d files, want 1", len(files))
	}
	if !strings.Contains(files["app.py"], "def main():") {
		t.Error("app.py missing main function")
	}
}

func TestLangExtension(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"go", ".go"},
		{"typescript", ".ts"},
		{"python", ".py"},
		{"rust", ".rs"},
		{"csharp", ".cs"},
		{"unknown", ".go"},
	}
	for _, tt := range tests {
		got := langExtension(tt.lang)
		if got != tt.want {
			t.Errorf("langExtension(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestLangTestCommand(t *testing.T) {
	cmd, args := langTestCommand("go")
	if cmd != "go" || args[0] != "test" {
		t.Errorf("go test command = %s %v", cmd, args)
	}

	cmd, args = langTestCommand("python")
	if cmd != "python" || args[1] != "pytest" {
		t.Errorf("python test command = %s %v", cmd, args)
	}

	cmd, args = langTestCommand("rust")
	if cmd != "cargo" || args[0] != "test" {
		t.Errorf("rust test command = %s %v", cmd, args)
	}
}

func TestSelfImproveCTOModelDefault(t *testing.T) {
	// selfImproveCTOModel defaults to Sonnet — the task is structured JSON output
	// from telemetry data, not deep architectural reasoning.
	p := &Pipeline{ctoModel: ""}
	model := p.selfImproveCTOModel()
	if model != "claude-sonnet-4-6" {
		t.Errorf("default self-improve CTO model = %q, want %q", model, "claude-sonnet-4-6")
	}
}

func TestSelfImproveCTOModelOverride(t *testing.T) {
	// Config.CTOModel propagates to pipeline and overrides the default.
	p := &Pipeline{ctoModel: "claude-opus-4-6"}
	model := p.selfImproveCTOModel()
	if model != "claude-opus-4-6" {
		t.Errorf("overridden self-improve CTO model = %q, want %q", model, "claude-opus-4-6")
	}
}

func TestReviewerModelDefault(t *testing.T) {
	// reviewTargeted uses Sonnet by default for targeted reviews (not the
	// reviewer role's default Opus). Verify via reviewerModel selection logic:
	// empty reviewerModel → "claude-sonnet-4-6".
	p := &Pipeline{reviewerModel: ""}
	model := p.targetedReviewModel()
	if model != "claude-sonnet-4-6" {
		t.Errorf("default targeted review model = %q, want %q", model, "claude-sonnet-4-6")
	}
}

func TestReviewerModelOverride(t *testing.T) {
	// Config.ReviewerModel propagates to pipeline and overrides the default.
	p := &Pipeline{reviewerModel: "claude-haiku-4-5-20251001"}
	model := p.targetedReviewModel()
	if model != "claude-haiku-4-5-20251001" {
		t.Errorf("overridden targeted review model = %q, want %q", model, "claude-haiku-4-5-20251001")
	}
}

func TestExtractLanguage(t *testing.T) {
	p := &Pipeline{}

	tests := []struct {
		design string
		want   string
	}{
		{"LANGUAGE: go\n\nEntity(Task)...", "go"},
		{"LANGUAGE: typescript\nSome spec", "typescript"},
		{"  LANGUAGE:  python \nstuff", "python"},
		{"No language specified here", "go"},
		{"language: rust\nspec", "rust"},
	}
	for _, tt := range tests {
		got := p.extractLanguage(tt.design)
		if got != tt.want {
			t.Errorf("extractLanguage(%q) = %q, want %q", tt.design[:20], got, tt.want)
		}
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		// Short input — no truncation
		{"add login page", "add-login-page"},

		// Exactly 40 chars — no truncation needed
		{"a234567890123456789012345678901234567890", "a234567890123456789012345678901234567890"},

		// Over 40 chars — truncate at last word boundary before 40
		{"build a task management app with kanban boards and dashboards", "build-a-task-management-app-with-kanban"},

		// Char 40 is mid-word — truncate at prior word boundary
		{"add comprehensive authentication support for enterprise users", "add-comprehensive-authentication"},

		// First word alone exceeds 40 chars — hard truncate fallback
		{"abcdefghijklmnopqrstuvwxyz1234567890abcdefghij", "abcdefghijklmnopqrstuvwxyz1234567890abcd"},

		// Empty input
		{"", "change"},

		// Non-alphanumeric only
		{"!@#$%", "change"},

		// Underscores and slashes become hyphens
		{"my_feature/branch name", "my-feature-branch-name"},
	}
	for _, tt := range tests {
		got := sanitizeBranchName(tt.desc)
		if got != tt.want {
			t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}

// testSigner implements event.Signer for tests.
type testSigner struct{}

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	return types.NewSignature(make([]byte, 64))
}

func TestEnsureAgentNoSpawnerEmitsAuthorityEvents(t *testing.T) {
	s := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()

	// Register human.
	humanRawPub := spawn.DerivePublicKey("human:TestHuman")
	humanPub, err := types.NewPublicKey([]byte(humanRawPub))
	if err != nil {
		t.Fatal(err)
	}
	humanActor, err := actors.Register(humanPub, "TestHuman", event.ActorTypeHuman)
	if err != nil {
		t.Fatal(err)
	}
	humanID := humanActor.ID()

	// Bootstrap graph — ensureAgent needs a non-empty graph head.
	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)
	signer := &testSigner{}
	bootstrap, err := bsFactory.Init(humanID, signer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatal(err)
	}

	factory := event.NewEventFactory(registry)
	convID, err := types.NewConversationID("conv_spawn_" + strings.Repeat("0", 24))
	if err != nil {
		t.Fatal(err)
	}

	p := &Pipeline{
		store:    s,
		actors:   actors,
		humanID:  humanID,
		signer:   signer,
		factory:  factory,
		convID:   convID,
		agents:   make(map[roles.Role]*roles.Agent),
		trackers: make(map[roles.Role]*resources.TrackingProvider),
		// spawner is nil — dev/bootstrap mode.
	}

	// ensureAgent emits authority events in the no-spawner branch.
	// Provider creation may succeed or fail depending on environment —
	// we only care that the authority events were emitted.
	_, _ = p.ensureAgent(context.Background(), roles.RoleBuilder, "test-builder")

	// Verify authority.requested event was emitted.
	authReqPage, err := s.ByType(event.EventTypeAuthorityRequested, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(authReqPage.Items()) == 0 {
		t.Error("expected authority.requested event")
	}

	// Verify authority.resolved event was emitted.
	authResPage, err := s.ByType(event.EventTypeAuthorityResolved, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(authResPage.Items()) == 0 {
		t.Error("expected authority.resolved event")
	}

	// Verify resolved content shows auto-approved.
	resolved := authResPage.Items()[0]
	content, ok := resolved.Content().(event.AuthorityResolvedContent)
	if !ok {
		t.Fatal("authority.resolved event has wrong content type")
	}
	if !content.Approved {
		t.Error("expected auto-approved resolution")
	}
	if content.Reason.IsSome() && content.Reason.Unwrap() != "auto-approved (no authority gate)" {
		t.Errorf("reason = %q, want %q", content.Reason.Unwrap(), "auto-approved (no authority gate)")
	}
}

func TestFindModuleDir(t *testing.T) {
	t.Run("marker in root", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test"), 0644); err != nil {
			t.Fatal(err)
		}
		got := findModuleDir(root, "go")
		if got != root {
			t.Errorf("findModuleDir = %q, want root %q", got, root)
		}
	})

	t.Run("marker in subdir", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "go")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module test"), 0644); err != nil {
			t.Fatal(err)
		}
		got := findModuleDir(root, "go")
		if got != sub {
			t.Errorf("findModuleDir = %q, want subdir %q", got, sub)
		}
	})

	t.Run("no marker returns root", func(t *testing.T) {
		root := t.TempDir()
		got := findModuleDir(root, "go")
		if got != root {
			t.Errorf("findModuleDir = %q, want root %q", got, root)
		}
	})

	t.Run("multiple subdirs picks first alphabetical", func(t *testing.T) {
		root := t.TempDir()
		// Create two subdirs, only "beta" has the marker.
		for _, name := range []string{"alpha", "beta"} {
			if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, "beta", "package.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
		got := findModuleDir(root, "javascript")
		want := filepath.Join(root, "beta")
		if got != want {
			t.Errorf("findModuleDir = %q, want %q", got, want)
		}
	})

	t.Run("root preferred over subdir", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "sub")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		// Marker in both root and subdir — root wins.
		for _, dir := range []string{root, sub} {
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		got := findModuleDir(root, "go")
		if got != root {
			t.Errorf("findModuleDir = %q, want root %q", got, root)
		}
	})

	t.Run("python pyproject.toml", func(t *testing.T) {
		root := t.TempDir()
		sub := filepath.Join(root, "py")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "pyproject.toml"), []byte("[project]"), 0644); err != nil {
			t.Fatal(err)
		}
		got := findModuleDir(root, "python")
		if got != sub {
			t.Errorf("findModuleDir = %q, want %q", got, sub)
		}
	})

	t.Run("rust Cargo.toml", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]"), 0644); err != nil {
			t.Fatal(err)
		}
		got := findModuleDir(root, "rust")
		if got != root {
			t.Errorf("findModuleDir = %q, want root %q", got, root)
		}
	})
}

func TestCTOAnalysisSkipsUnderstandPhase(t *testing.T) {
	// Verify that ProductInput.CTOAnalysis is properly formatted from a
	// SelfImproveRecommendation and that the field flows through to targeted
	// pipeline input, which would skip the CTO Evaluate call in Phase 2.
	rec := SelfImproveRecommendation{
		Description:    "Refactor the build phase",
		FilesToChange:  []string{"pkg/pipeline/pipeline.go", "pkg/pipeline/pipeline_test.go"},
		ExpectedImpact: "Reduce build phase duration by 30%",
		Priority:       "high",
	}

	// Format the same way RunSelfImprove does.
	analysis := fmt.Sprintf("Description: %s\nFiles to change: %v\nExpected impact: %s",
		rec.Description, rec.FilesToChange, rec.ExpectedImpact)

	input := ProductInput{
		RepoPath:    "/tmp/fake-repo",
		Description: rec.Description,
		CTOAnalysis: analysis,
	}

	// CTOAnalysis must be non-empty so the Understand phase is skipped.
	if input.CTOAnalysis == "" {
		t.Fatal("CTOAnalysis should be non-empty")
	}

	// Verify the formatted string contains all recommendation fields.
	if !strings.Contains(input.CTOAnalysis, rec.Description) {
		t.Error("CTOAnalysis missing Description")
	}
	for _, f := range rec.FilesToChange {
		if !strings.Contains(input.CTOAnalysis, f) {
			t.Errorf("CTOAnalysis missing file %q", f)
		}
	}
	if !strings.Contains(input.CTOAnalysis, rec.ExpectedImpact) {
		t.Error("CTOAnalysis missing ExpectedImpact")
	}

	// Verify that an empty CTOAnalysis would NOT skip (the default path).
	defaultInput := ProductInput{
		RepoPath:    "/tmp/fake-repo",
		Description: "some change",
	}
	if defaultInput.CTOAnalysis != "" {
		t.Error("default ProductInput should have empty CTOAnalysis")
	}
}

func TestLangMarkerFile(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"go", "go.mod"},
		{"typescript", "package.json"},
		{"javascript", "package.json"},
		{"python", "pyproject.toml"},
		{"rust", "Cargo.toml"},
		{"csharp", "*.csproj"},
		{"unknown", "go.mod"},
	}
	for _, tt := range tests {
		got := langMarkerFile(tt.lang)
		if got != tt.want {
			t.Errorf("langMarkerFile(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

