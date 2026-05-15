package runner

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// callLog is a thread-safe append-only record of prompt strings.
// The council fans out per-member goroutines onto a shared provider,
// so the recording slice must be guarded against races.
type callLog struct {
	mu    sync.Mutex
	items []string
}

func (c *callLog) Append(s string) {
	c.mu.Lock()
	c.items = append(c.items, s)
	c.mu.Unlock()
}

func (c *callLog) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// recordingFakeProvider records every Reason() call so tests can
// assert which roles invoked it. Does not implement IOperator.
type recordingFakeProvider struct {
	name  string
	model string
	calls *callLog
}

func (f *recordingFakeProvider) Name() string  { return f.name }
func (f *recordingFakeProvider) Model() string { return f.model }
func (f *recordingFakeProvider) Reason(_ context.Context, prompt string, _ []event.Event) (decision.Response, error) {
	f.calls.Append(prompt)
	return decision.NewResponse("fake reply from "+f.name, types.MustScore(0.7), decision.TokenUsage{}), nil
}

// fakeBuilderFor returns an intelligence.Provider builder that records
// every Reason() call grouped by "provider/model".
func fakeBuilderFor(callsByModel map[string]*callLog) func(intelligence.Config) (intelligence.Provider, error) {
	var mu sync.Mutex
	return func(cfg intelligence.Config) (intelligence.Provider, error) {
		key := cfg.Provider + "/" + cfg.Model
		mu.Lock()
		log, ok := callsByModel[key]
		if !ok {
			log = &callLog{}
			callsByModel[key] = log
		}
		mu.Unlock()
		return &recordingFakeProvider{name: cfg.Provider, model: cfg.Model, calls: log}, nil
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureLog redirects log.Writer() to a buffer so tests can assert log content.
type logCapture struct {
	original io.Writer
	buf      *bytes.Buffer
}

func captureLog(t *testing.T) *logCapture {
	t.Helper()
	buf := &bytes.Buffer{}
	orig := log.Writer()
	log.SetOutput(buf)
	return &logCapture{original: orig, buf: buf}
}

func (l *logCapture) String() string { return l.buf.String() }
func (l *logCapture) restore()       { log.SetOutput(l.original) }

// mapKeys returns the keys of m for inclusion in error messages.
func mapKeys(m map[string]*callLog) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestRunCouncil_UsesPoolWhenSet(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "agents/planner.md"), "# Planner\nYou are the Planner.")
	mustWrite(t, filepath.Join(tmp, "agents/strategist.md"), "# Strategist\nYou are the Strategist.")

	calls := map[string]*callLog{}
	r := modelconfig.DefaultResolver()
	pool := modelconfig.NewProviderPoolWithBuilder(r, fakeBuilderFor(calls))

	err := RunCouncil(context.Background(), Config{
		HiveDir:      tmp,
		Pool:         pool,
		BudgetUSD:    10.0,
		CouncilTopic: "test topic",
	})
	if err != nil {
		t.Fatalf("RunCouncil: %v", err)
	}

	// 2 members → 1 provider → 2 Reason() calls.
	// Verify dedup invariants, not which model was selected.
	if len(calls) != 1 {
		t.Fatalf("expected 1 unique provider in cache (both members share one model); got %d entries: %v", len(calls), mapKeys(calls))
	}
	for key, c := range calls {
		if got := c.Len(); got != 2 {
			t.Errorf("provider %s: expected 2 Reason calls (one per council member); got %d", key, got)
		}
	}
}

func TestRunCouncil_RoutesYAMLCatalogPerRole(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "agents/planner.md"), "# Planner\nYou are the Planner.")
	mustWrite(t, filepath.Join(tmp, "agents/implementer.md"), "# Implementer\nYou are the Implementer.")

	catalogPath := filepath.Join(tmp, "catalog.yaml")
	if err := os.WriteFile(catalogPath, []byte(testYAMLCatalog), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	r, err := modelconfig.ResolverFromCatalogFile(catalogPath)
	if err != nil {
		t.Fatalf("ResolverFromCatalogFile: %v", err)
	}

	type seenCfg struct {
		Provider string
		Model    string
		BaseURL  string
	}
	var (
		mu   sync.Mutex
		seen []seenCfg
	)
	builder := func(cfg intelligence.Config) (intelligence.Provider, error) {
		mu.Lock()
		seen = append(seen, seenCfg{Provider: cfg.Provider, Model: cfg.Model, BaseURL: cfg.BaseURL})
		mu.Unlock()
		return &recordingFakeProvider{name: cfg.Provider, model: cfg.Model, calls: &callLog{}}, nil
	}
	pool := modelconfig.NewProviderPoolWithBuilder(r, builder)

	err = RunCouncil(context.Background(), Config{
		HiveDir:      tmp,
		Pool:         pool,
		BudgetUSD:    10.0,
		CouncilTopic: "test",
	})
	if err != nil {
		t.Fatalf("RunCouncil: %v", err)
	}

	// Two roles → two distinct resolved configs → two builder invocations.
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("expected 2 builder invocations (one per unique config); got %d: %v", len(seen), seen)
	}

	wantPlanner := seenCfg{Provider: "openai-compatible", Model: "test-planner-model", BaseURL: "https://example.test/api/v1"}
	wantImpl := seenCfg{Provider: "openai-compatible", Model: "test-implementer-model", BaseURL: "https://example.test/api/v2"}

	foundPlanner, foundImpl := false, false
	for _, s := range seen {
		switch s {
		case wantPlanner:
			foundPlanner = true
		case wantImpl:
			foundImpl = true
		default:
			t.Errorf("unexpected config seen by builder: %+v", s)
		}
	}
	if !foundPlanner {
		t.Errorf("planner config not seen; got: %v", seen)
	}
	if !foundImpl {
		t.Errorf("implementer config not seen; got: %v", seen)
	}
}

const testYAMLCatalog = `
models:
  - id: test-planner-model
    aliases: [planner-alias]
    provider: openai-compatible
    base_url: https://example.test/api/v1
    auth_mode: api-key
    tier: judgment
    capabilities: [tools, reasoning]
    context_window: 100000
    max_output_tokens: 4096
    pricing:
      input_per_million: 0.50
      output_per_million: 1.00

  - id: test-implementer-model
    aliases: [implementer-alias]
    provider: openai-compatible
    base_url: https://example.test/api/v2
    auth_mode: api-key
    tier: judgment
    capabilities: [tools, reasoning, operate]
    context_window: 200000
    max_output_tokens: 8192
    pricing:
      input_per_million: 1.00
      output_per_million: 2.00

tier_defaults:
  judgment: planner-alias
  execution: planner-alias
  volume: planner-alias

role_defaults:
  planner: planner-alias
  implementer: implementer-alias
`

func TestRunCouncil_WarnsOnCanOperateMismatch(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "agents/implementer.md"), "# Implementer\nYou are the Implementer.")

	calls := map[string]*callLog{}
	r := modelconfig.DefaultResolver()
	pool := modelconfig.NewProviderPoolWithBuilder(r, fakeBuilderFor(calls))

	logBuf := captureLog(t)
	defer logBuf.restore()

	err := RunCouncil(context.Background(), Config{
		HiveDir:          tmp,
		Pool:             pool,
		CanOperateLookup: func(role string) bool { return role == "implementer" },
	})
	if err != nil {
		t.Fatalf("RunCouncil: %v", err)
	}

	out := logBuf.String()
	if !strings.Contains(out, "warning: role implementer is CanOperate=true") {
		t.Errorf("expected CanOperate warning in log; got: %s", out)
	}
	if !strings.Contains(out, "fall back to Reason()") {
		t.Errorf("expected fallback note in log; got: %s", out)
	}
}
