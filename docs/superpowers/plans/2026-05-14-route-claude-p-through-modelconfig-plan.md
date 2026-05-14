# Route claude -p Through modelconfig — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `modelconfig` from hive to eventgraph, add a shared `ProviderPool`, and route the three remaining hardcoded `claude -p` call sites (hive council, site Mind, run.sh) through it so every agent honors the catalog.

**Architecture:** `modelconfig` relocates to `eventgraph/go/pkg/modelconfig` (substrate package, sibling of `intelligence`). A new `ProviderPool` deduplicates `intelligence.IIntelligence` instances by `(Provider, Model, BaseURL, APIKey)` tuple. Hive council adds `--catalog` and resolves per-role through the pool; site Mind warms the pool at startup for all loaded personas. `run.sh` retires.

**Tech Stack:** Go 1.25, eventgraph/go module, hive/go.mod with `replace ../eventgraph/go`, site/go.mod with `replace ../eventgraph/go`, sync.Map for pool cache, table-driven tests.

**Reference:** Design spec `hive/docs/superpowers/specs/2026-05-14-route-claude-p-through-modelconfig-design.md` (commit `bb2aaa4` on branch `feat/route-claude-p-through-modelconfig`).

---

## Pre-flight

### Task 0: Confirm clean slate on each repo's feature branch

**Files:** none modified — verification only.

- [ ] **Step 1: Verify branches and current state**

```bash
for repo in /Users/brewskie/github/mind-zero/eventgraph \
            /Users/brewskie/github/mind-zero/hive \
            /Users/brewskie/github/mind-zero/site; do
  echo "=== $repo ==="
  git -C "$repo" branch --show-current
  git -C "$repo" status --short | head -20
  echo
done
```

Expected output:
- `eventgraph`: branch `main` (no feat branch needed yet; commits 1 & 2 land here on a new branch we'll create in Task 1)
- `hive`: branch `feat/route-claude-p-through-modelconfig`, possibly with pre-existing dirty files (`go.mod`, `.claude/settings.local.json`, untracked docs)
- `site`: branch `feat/route-claude-p-through-modelconfig`, with templ regenerations and `go.mod`/`go.sum` bumps from the local templ upgrade

- [ ] **Step 2: Stash any pre-existing local-only work on hive and site to keep this PR diff clean**

```bash
git -C /Users/brewskie/github/mind-zero/hive stash push -u -m "pre-modelconfig-routing local work" -- \
  .claude/settings.local.json go.mod docs/designs/model-config-follow-ups.md \
  docs/superpowers/plans/2026-04-28-modelconfig-pr-review-fixes.md loop.test resources.test
git -C /Users/brewskie/github/mind-zero/site stash push -u -m "pre-modelconfig-routing local work"
```

Expected: both repos report saved stash. The hive `go.mod` stashed copy will be re-applied after Commit 3 since Commit 3 also bumps `go.mod` and any unrelated changes can be reconciled separately.

- [ ] **Step 3: Verify post-stash state is clean**

```bash
git -C /Users/brewskie/github/mind-zero/hive status --short
git -C /Users/brewskie/github/mind-zero/site status --short
```

Expected: empty output from both. If either still shows uncommitted work that is genuinely needed for this PR (e.g., a templ regen that won't go away), pause and ask the user how to handle it.

- [ ] **Step 4: Create branch on eventgraph**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph
git checkout -b feat/route-claude-p-through-modelconfig
git branch --show-current
```

Expected: `feat/route-claude-p-through-modelconfig`.

---

## Commit 1 (eventgraph): Move modelconfig from hive to eventgraph

### Task 1.1: Copy modelconfig files into eventgraph

**Files:**
- Create: `eventgraph/go/pkg/modelconfig/` (entire directory mirror of hive's)
- Modify: every `.go` file's package import paths (no change to package name itself)

Files to move (17 total):
```
adapter.go            catalog.go           pricing.go        role.go
adapter_test.go       catalog_test.go      pricing_test.go   role_test.go
capability.go         defaults.go          profile.go        types.go
capability_test.go    defaults_catalog.yaml resolve.go
defaults_test.go      policy.go            resolve_test.go
```

- [ ] **Step 1: Copy files**

```bash
mkdir -p /Users/brewskie/github/mind-zero/eventgraph/go/pkg/modelconfig
cp -a /Users/brewskie/github/mind-zero/hive/pkg/modelconfig/. \
      /Users/brewskie/github/mind-zero/eventgraph/go/pkg/modelconfig/
ls /Users/brewskie/github/mind-zero/eventgraph/go/pkg/modelconfig/
```

Expected: lists all 17 files.

- [ ] **Step 2: Update internal import paths in modelconfig files**

The modelconfig package currently imports `github.com/transpara-ai/eventgraph/go/pkg/intelligence`. That import is unchanged (intelligence stays where it is). Search for any internal hive-specific import to be safe:

```bash
grep -rn "github.com/transpara-ai/hive" /Users/brewskie/github/mind-zero/eventgraph/go/pkg/modelconfig/
```

Expected: empty output. If anything matches, fix it inline (replace with the eventgraph equivalent or remove if hive-specific).

- [ ] **Step 3: Run the relocated tests in place**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph/go
go test -race ./pkg/modelconfig/... 2>&1 | tail -20
```

Expected: `ok  github.com/transpara-ai/eventgraph/go/pkg/modelconfig`. All tests pass.

- [ ] **Step 4: Verify the whole eventgraph module still builds**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph/go
go build ./...
go vet ./...
```

Expected: no output (clean build, clean vet).

- [ ] **Step 5: Commit eventgraph side of the move**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph
git add go/pkg/modelconfig/
git commit -m "$(cat <<'EOF'
refactor: move modelconfig from hive to eventgraph

modelconfig is provider-agnostic substrate (model selection and
resolution), sibling to the intelligence package. Hive happens to
have been the first consumer; site needs to import it too and cannot
depend on hive per the site UI-ownership rule. Mechanical move with
no semantic changes — every file is byte-identical to its hive
counterpart aside from the package path that callers will use.

The matching deletion from hive lands in a coordinated commit on the
hive feature branch once eventgraph's PR merges.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
git log --oneline -1
```

Expected: commit subject `refactor: move modelconfig from hive to eventgraph`, with the modelconfig files staged as additions.

---

## Commit 2 (eventgraph): Add ProviderPool

### Task 2.1: Add stableConfigHash test

**Files:**
- Create: `eventgraph/go/pkg/modelconfig/pool_test.go`
- Will create later: `eventgraph/go/pkg/modelconfig/pool.go`

- [ ] **Step 1: Write the failing test for the config-hash key function**

Create `eventgraph/go/pkg/modelconfig/pool_test.go`:

```go
package modelconfig

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
)

func TestStableConfigHash_DeduplicatesByFourFields(t *testing.T) {
	a := intelligence.Config{Provider: "openrouter", Model: "moonshotai/kimi-latest", BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-1"}
	b := intelligence.Config{Provider: "openrouter", Model: "moonshotai/kimi-latest", BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-1"}
	c := intelligence.Config{Provider: "openrouter", Model: "moonshotai/kimi-latest", BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-2"}
	d := intelligence.Config{Provider: "claude-cli", Model: "sonnet"}

	if stableConfigHash(a) != stableConfigHash(b) {
		t.Errorf("identical configs hashed differently: %q vs %q", stableConfigHash(a), stableConfigHash(b))
	}
	if stableConfigHash(a) == stableConfigHash(c) {
		t.Errorf("configs differing in APIKey hashed same")
	}
	if stableConfigHash(a) == stableConfigHash(d) {
		t.Errorf("different providers hashed same")
	}
}
```

- [ ] **Step 2: Run the test to verify failure**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph/go
go test -run TestStableConfigHash_DeduplicatesByFourFields ./pkg/modelconfig/
```

Expected: compile error — `undefined: stableConfigHash`.

- [ ] **Step 3: Implement stableConfigHash in a new pool.go**

Create `eventgraph/go/pkg/modelconfig/pool.go`:

```go
// Package modelconfig adds a provider deduplication pool on top of
// the resolver. The pool returns the same intelligence.IIntelligence
// instance for any two roles that resolve to identical underlying
// configuration, so N council members on the same model spawn one
// shared provider rather than N.
package modelconfig

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
)

// stableConfigHash returns a deterministic hex hash over the four
// fields that uniquely identify a provider instance.
func stableConfigHash(cfg intelligence.Config) string {
	h := sha256.New()
	h.Write([]byte(cfg.Provider))
	h.Write([]byte{0})
	h.Write([]byte(cfg.Model))
	h.Write([]byte{0})
	h.Write([]byte(cfg.BaseURL))
	h.Write([]byte{0})
	h.Write([]byte(cfg.APIKey))
	return hex.EncodeToString(h.Sum(nil))
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph/go
go test -run TestStableConfigHash_DeduplicatesByFourFields ./pkg/modelconfig/ -v
```

Expected: `--- PASS: TestStableConfigHash_DeduplicatesByFourFields`.

### Task 2.2: ProviderPool struct + For() returns same instance for same role

- [ ] **Step 1: Write the failing identity test**

Append to `eventgraph/go/pkg/modelconfig/pool_test.go`:

```go
// fakeProvider satisfies intelligence.IIntelligence for tests so
// nothing actually spawns a subprocess or hits a network.
type fakeProvider struct{ name string }

func (f *fakeProvider) Name() string  { return f.name }
func (f *fakeProvider) Model() string { return "fake" }
func (f *fakeProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	return decision.Response{}, nil
}

func newFakePool(t *testing.T) *ProviderPool {
	t.Helper()
	r := DefaultResolver()
	calls := 0
	builder := func(cfg intelligence.Config) (intelligence.IIntelligence, error) {
		calls++
		return &fakeProvider{name: cfg.Provider + "/" + cfg.Model}, nil
	}
	return NewProviderPoolWithBuilder(r, builder)
}

func TestProviderPool_For_SameRoleReturnsSameInstance(t *testing.T) {
	p := newFakePool(t)
	a, err := p.For("planner")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	b, err := p.For("planner")
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if a != b {
		t.Errorf("same role returned different provider instances: %p vs %p", a, b)
	}
}
```

Also add the imports at the top of the test file:

```go
import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
)
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -run TestProviderPool_For_SameRoleReturnsSameInstance ./pkg/modelconfig/
```

Expected: compile error — `undefined: ProviderPool`, `undefined: NewProviderPoolWithBuilder`.

- [ ] **Step 3: Implement ProviderPool**

Append to `eventgraph/go/pkg/modelconfig/pool.go`:

```go
import (
	"fmt"
	"sync"
)

// ProviderPool resolves roles to providers and caches one
// intelligence.IIntelligence per unique (Provider, Model, BaseURL,
// APIKey) tuple. Safe for concurrent use.
type ProviderPool struct {
	resolver *Resolver
	cache    sync.Map // key=stableConfigHash, value=intelligence.IIntelligence
	builder  func(intelligence.Config) (intelligence.IIntelligence, error)
}

// NewProviderPool builds a pool whose builder is intelligence.New.
func NewProviderPool(r *Resolver) *ProviderPool {
	return &ProviderPool{resolver: r, builder: intelligence.New}
}

// NewProviderPoolWithBuilder allows tests to inject a fake builder.
func NewProviderPoolWithBuilder(r *Resolver, b func(intelligence.Config) (intelligence.IIntelligence, error)) *ProviderPool {
	return &ProviderPool{resolver: r, builder: b}
}

// For resolves the role through the resolver and returns the cached
// provider for the resolved config (or builds one if absent).
func (p *ProviderPool) For(role string) (intelligence.IIntelligence, error) {
	resolved, err := p.resolver.Resolve(ResolutionInput{Role: role})
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", role, err)
	}
	cfg := ToIntelligenceConfig(resolved, "")
	return p.providerFor(cfg)
}

func (p *ProviderPool) providerFor(cfg intelligence.Config) (intelligence.IIntelligence, error) {
	key := stableConfigHash(cfg)
	if existing, ok := p.cache.Load(key); ok {
		return existing.(intelligence.IIntelligence), nil
	}
	created, err := p.builder(cfg)
	if err != nil {
		return nil, fmt.Errorf("build provider for %s/%s: %w", cfg.Provider, cfg.Model, err)
	}
	actual, _ := p.cache.LoadOrStore(key, created)
	return actual.(intelligence.IIntelligence), nil
}
```

Note: `LoadOrStore` ensures that even under a race only one provider becomes the canonical one in the cache (the others are GC-collected).

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -run TestProviderPool_For_SameRoleReturnsSameInstance ./pkg/modelconfig/ -v
```

Expected: PASS.

### Task 2.3: Different roles with same resolved config share provider

- [ ] **Step 1: Write the dedup test**

Append to `pool_test.go`:

```go
func TestProviderPool_For_DifferentRolesSameConfigShareProvider(t *testing.T) {
	// DefaultResolver routes both "planner" and "scout" to sonnet/claude-cli.
	// Verify they share the underlying provider.
	p := newFakePool(t)
	a, err := p.For("planner")
	if err != nil {
		t.Fatalf("For planner: %v", err)
	}
	b, err := p.For("scout")
	if err != nil {
		t.Fatalf("For scout: %v", err)
	}
	if a != b {
		t.Errorf("planner and scout (both → sonnet/claude-cli) returned different providers: %p vs %p", a, b)
	}
}
```

Note: planner and scout don't resolve to the same model in DefaultResolver (sonnet vs haiku). Use roles that genuinely share resolution. Looking at `defaults_catalog.yaml`: planner=sonnet, strategist=sonnet, reviewer=sonnet, critic=sonnet. Swap to those:

```go
	a, err := p.For("planner")    // → sonnet
	b, err := p.For("strategist") // → sonnet
```

- [ ] **Step 2: Run the test to verify it passes (no new impl needed)**

```bash
go test -run TestProviderPool_For_DifferentRolesSameConfigShareProvider ./pkg/modelconfig/ -v
```

Expected: PASS. The `providerFor` logic already deduplicates by hash; this test asserts the resolver+pool composition works end-to-end.

### Task 2.4: WarmForRoles batches resolution

- [ ] **Step 1: Write failing test**

Append to `pool_test.go`:

```go
func TestProviderPool_WarmForRoles_PrebuildsCacheAndIsIdempotent(t *testing.T) {
	calls := 0
	r := DefaultResolver()
	builder := func(cfg intelligence.Config) (intelligence.IIntelligence, error) {
		calls++
		return &fakeProvider{name: cfg.Provider + "/" + cfg.Model}, nil
	}
	p := NewProviderPoolWithBuilder(r, builder)

	roles := []string{"planner", "strategist", "reviewer", "implementer"}
	if err := p.WarmForRoles(roles); err != nil {
		t.Fatalf("WarmForRoles: %v", err)
	}
	first := calls
	if first == 0 {
		t.Fatal("expected builder calls after warm-up; got 0")
	}

	// Calling again should not invoke the builder.
	if err := p.WarmForRoles(roles); err != nil {
		t.Fatalf("WarmForRoles second: %v", err)
	}
	if calls != first {
		t.Errorf("WarmForRoles not idempotent: builder called %d times after second warm-up (expected %d)", calls, first)
	}
}
```

- [ ] **Step 2: Verify failure**

```bash
go test -run TestProviderPool_WarmForRoles_PrebuildsCacheAndIsIdempotent ./pkg/modelconfig/
```

Expected: compile error — `undefined: WarmForRoles`.

- [ ] **Step 3: Implement WarmForRoles**

Append to `pool.go`:

```go
// WarmForRoles resolves every role and pre-populates the cache. Safe
// to call multiple times; the underlying providerFor short-circuits
// on cache hit. Returns the first error if any role fails to
// resolve; the cache may be partially populated on error.
func (p *ProviderPool) WarmForRoles(roles []string) error {
	for _, role := range roles {
		if _, err := p.For(role); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Verify pass**

```bash
go test -run TestProviderPool_WarmForRoles_PrebuildsCacheAndIsIdempotent ./pkg/modelconfig/ -v
```

Expected: PASS.

### Task 2.5: Stats reports unique providers and per-model counts

- [ ] **Step 1: Write failing test**

Append to `pool_test.go`:

```go
func TestProviderPool_Stats_ReportsUniqueProvidersAndPerModelCounts(t *testing.T) {
	p := newFakePool(t)
	roles := []string{"planner", "strategist", "reviewer", "implementer", "guardian"}
	// planner, strategist, reviewer → sonnet (1 unique)
	// implementer → opus (1 unique)
	// guardian → sonnet (already counted)
	if err := p.WarmForRoles(roles); err != nil {
		t.Fatalf("WarmForRoles: %v", err)
	}

	stats := p.Stats()
	if stats.TotalRoles != 5 {
		t.Errorf("TotalRoles=%d; want 5", stats.TotalRoles)
	}
	if stats.UniqueProviders != 2 {
		t.Errorf("UniqueProviders=%d; want 2 (sonnet+opus)", stats.UniqueProviders)
	}
	if got := stats.RolesPerProvider["claude-cli/claude-sonnet-4-6"]; got != 4 {
		t.Errorf("RolesPerProvider[sonnet]=%d; want 4", got)
	}
	if got := stats.RolesPerProvider["claude-cli/claude-opus-4-6"]; got != 1 {
		t.Errorf("RolesPerProvider[opus]=%d; want 1", got)
	}
}
```

- [ ] **Step 2: Verify failure**

```bash
go test -run TestProviderPool_Stats_ReportsUniqueProvidersAndPerModelCounts ./pkg/modelconfig/
```

Expected: compile error — `undefined: Stats`, `undefined: PoolStats`.

- [ ] **Step 3: Implement Stats**

The current implementation of For/providerFor doesn't track which roles map to which hash; we need a small side-table. Modify the `ProviderPool` struct and `For` method:

In `pool.go`, replace the struct definition and `For` method with:

```go
type ProviderPool struct {
	resolver *Resolver
	cache    sync.Map // key=stableConfigHash, value=intelligence.IIntelligence
	builder  func(intelligence.Config) (intelligence.IIntelligence, error)

	mu              sync.Mutex
	roleHashes      map[string]string // role → stableConfigHash
	hashDescription map[string]string // stableConfigHash → "provider/model"
}

func NewProviderPool(r *Resolver) *ProviderPool {
	return &ProviderPool{resolver: r, builder: intelligence.New, roleHashes: map[string]string{}, hashDescription: map[string]string{}}
}

func NewProviderPoolWithBuilder(r *Resolver, b func(intelligence.Config) (intelligence.IIntelligence, error)) *ProviderPool {
	return &ProviderPool{resolver: r, builder: b, roleHashes: map[string]string{}, hashDescription: map[string]string{}}
}

func (p *ProviderPool) For(role string) (intelligence.IIntelligence, error) {
	resolved, err := p.resolver.Resolve(ResolutionInput{Role: role})
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", role, err)
	}
	cfg := ToIntelligenceConfig(resolved, "")
	provider, err := p.providerFor(cfg)
	if err != nil {
		return nil, err
	}
	hash := stableConfigHash(cfg)
	p.mu.Lock()
	p.roleHashes[role] = hash
	p.hashDescription[hash] = fmt.Sprintf("%s/%s", cfg.Provider, cfg.Model)
	p.mu.Unlock()
	return provider, nil
}
```

Then add `Stats()` and `PoolStats`:

```go
// PoolStats describes the current state of the pool's caches.
type PoolStats struct {
	TotalRoles       int            // distinct role names seen via For()
	UniqueProviders  int            // distinct provider instances in the cache
	RolesPerProvider map[string]int // key="provider/model", value=role count
}

// Summary returns a single human-readable line.
func (s PoolStats) Summary() string {
	parts := make([]string, 0, len(s.RolesPerProvider))
	for k, v := range s.RolesPerProvider {
		parts = append(parts, fmt.Sprintf("%s×%d", k, v))
	}
	return fmt.Sprintf("%d roles → %d unique providers [%s]", s.TotalRoles, s.UniqueProviders, strings.Join(parts, ", "))
}

func (p *ProviderPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	rpp := map[string]int{}
	for _, hash := range p.roleHashes {
		desc := p.hashDescription[hash]
		rpp[desc]++
	}
	return PoolStats{
		TotalRoles:       len(p.roleHashes),
		UniqueProviders:  len(rpp),
		RolesPerProvider: rpp,
	}
}
```

Also add `"strings"` to the imports.

- [ ] **Step 4: Verify the Stats test passes**

```bash
go test -run TestProviderPool_Stats_ReportsUniqueProvidersAndPerModelCounts ./pkg/modelconfig/ -v
```

Expected: PASS.

### Task 2.6: Resolver error propagates without poisoning cache

- [ ] **Step 1: Write failing test**

Append to `pool_test.go`:

```go
func TestProviderPool_For_ResolverErrorDoesNotPoisonCache(t *testing.T) {
	// Build a pool with an empty catalog so unknown roles error out.
	empty := &Catalog{}
	r, err := NewResolver(empty, ResolverDefaults{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	calls := 0
	builder := func(cfg intelligence.Config) (intelligence.IIntelligence, error) {
		calls++
		return &fakeProvider{name: cfg.Provider + "/" + cfg.Model}, nil
	}
	p := NewProviderPoolWithBuilder(r, builder)

	if _, err := p.For("nonexistent-role"); err == nil {
		t.Fatal("expected error for unknown role; got nil")
	}
	if calls != 0 {
		t.Errorf("builder invoked despite resolver error: %d calls", calls)
	}

	stats := p.Stats()
	if stats.TotalRoles != 0 {
		t.Errorf("Stats.TotalRoles=%d after failed For; want 0 (cache must not be polluted)", stats.TotalRoles)
	}
}
```

If `NewResolver` doesn't exist with that signature in the modelconfig package, replace with whatever constructor the existing tests use. Check `resolve_test.go` for the right pattern.

- [ ] **Step 2: Verify pass**

```bash
go test -run TestProviderPool_For_ResolverErrorDoesNotPoisonCache ./pkg/modelconfig/ -v
```

Expected: PASS without any new implementation (the existing For/providerFor already propagates errors).

### Task 2.7: Concurrent For calls are race-free

- [ ] **Step 1: Write the race test**

Append to `pool_test.go`:

```go
func TestProviderPool_For_ConcurrentCallsRaceFree(t *testing.T) {
	p := newFakePool(t)
	const goroutines = 50
	const calls = 20
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*calls)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < calls; j++ {
				if _, err := p.For("planner"); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent For: %v", err)
	}

	stats := p.Stats()
	if stats.TotalRoles != 1 {
		t.Errorf("TotalRoles=%d after concurrent same-role calls; want 1", stats.TotalRoles)
	}
}
```

Add `"sync"` to the imports if not present.

- [ ] **Step 2: Run with -race to verify**

```bash
go test -race -run TestProviderPool_For_ConcurrentCallsRaceFree ./pkg/modelconfig/ -v
```

Expected: PASS, no race detector output.

### Task 2.8: DefaultResolverWithModel synthesizes single-model resolver

- [ ] **Step 1: Write failing test**

Append to `pool_test.go`:

```go
func TestDefaultResolverWithModel_EveryRoleResolvesToNamedModel(t *testing.T) {
	r, err := DefaultResolverWithModel("opus")
	if err != nil {
		t.Fatalf("DefaultResolverWithModel: %v", err)
	}
	for _, role := range []string{"planner", "implementer", "guardian", "advocate"} {
		resolved, err := r.Resolve(ResolutionInput{Role: role})
		if err != nil {
			t.Fatalf("Resolve %s: %v", role, err)
		}
		if resolved.Model != "claude-opus-4-6" {
			t.Errorf("Resolve(%s).Model=%q; want claude-opus-4-6 (via opus alias)", role, resolved.Model)
		}
	}
}

func TestDefaultResolverWithModel_UnknownAliasErrors(t *testing.T) {
	if _, err := DefaultResolverWithModel("definitely-not-a-real-model"); err == nil {
		t.Fatal("expected error for unknown alias; got nil")
	}
}
```

- [ ] **Step 2: Verify failure**

```bash
go test -run TestDefaultResolverWithModel ./pkg/modelconfig/
```

Expected: compile error — `undefined: DefaultResolverWithModel`.

- [ ] **Step 3: Implement DefaultResolverWithModel**

Add to `pool.go` (or a new `defaults_with_model.go` if you prefer separation):

```go
// DefaultResolverWithModel returns a resolver where every role and
// every tier query resolves to the named model. Used by the council
// COUNCIL_MODEL legacy fallback. The model name must resolve via the
// built-in catalog's alias table (e.g., "opus", "sonnet", "haiku" or
// a full id like "claude-opus-4-6"). Returns an error if unknown.
func DefaultResolverWithModel(modelNameOrAlias string) (*Resolver, error) {
	r := DefaultResolver()
	entry, ok := r.Catalog().Lookup(modelNameOrAlias)
	if !ok {
		return nil, fmt.Errorf("DefaultResolverWithModel: %q is not a known model id or alias", modelNameOrAlias)
	}

	// Override defaults so every resolution path lands on this entry.
	r.defaults.Model = entry.ID
	r.defaults.TierModels = map[ModelTier]string{
		TierJudgment:  entry.ID,
		TierExecution: entry.ID,
		TierVolume:    entry.ID,
	}
	// Clear per-role defaults so the precedence chain falls through to
	// the tier defaults (which all point at the same entry).
	r.defaults.RoleModels = nil

	return r, nil
}
```

This requires that `Resolver` exposes `defaults` (or has a setter) and `Catalog()` is a public method. If either is currently unexported, add minimal accessors in their respective files (`resolve.go`):

```go
// In resolve.go — add if not already present.
func (r *Resolver) Catalog() *Catalog { return r.catalog }
```

If `defaults` is unexported and there's no setter, add one:

```go
// In resolve.go — add if not already present.
func (r *Resolver) SetDefaults(d ResolverDefaults) { r.defaults = d }
```

(Adjust field names to match what the actual `resolve.go` has — confirm by reading the file before writing.)

- [ ] **Step 4: Verify both tests pass**

```bash
go test -run TestDefaultResolverWithModel ./pkg/modelconfig/ -v
```

Expected: both PASS.

### Task 2.9: Run full pool test suite + module-wide tests

- [ ] **Step 1: Run all pool tests with -race**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph/go
go test -race ./pkg/modelconfig/... -v 2>&1 | tail -30
```

Expected: all tests PASS, no race detector warnings.

- [ ] **Step 2: Run module-wide build + tests to verify nothing else broke**

```bash
go build ./...
go test -race ./...
go vet ./...
```

Expected: clean build, all tests pass.

- [ ] **Step 3: Run staticcheck**

```bash
staticcheck ./pkg/modelconfig/...
```

Expected: empty output.

### Task 2.10: Commit ProviderPool

- [ ] **Step 1: Stage and commit**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph
git add go/pkg/modelconfig/pool.go go/pkg/modelconfig/pool_test.go go/pkg/modelconfig/resolve.go
git status --short
```

Expected: `pool.go`, `pool_test.go`, and (if you added accessors) `resolve.go` listed as additions/modifications.

```bash
git commit -m "$(cat <<'EOF'
feat(modelconfig): add ProviderPool with dedup, warm-up, and stats

ProviderPool wraps a Resolver and caches one intelligence.IIntelligence
per unique (Provider, Model, BaseURL, APIKey) tuple. Multiple roles
resolving to the same config share a provider instance — important
for council fan-out (50 members on one model = 1 provider) and for
site Mind warming a pool over many personas at startup.

API:
  NewProviderPool(r)                   default builder = intelligence.New
  NewProviderPoolWithBuilder(r, b)     injectable for tests
  pool.For(role)                       resolve + lazy cache
  pool.WarmForRoles([]role)            batch pre-resolve
  pool.Stats()                         per-model role count + summary

Also adds DefaultResolverWithModel(name) for the COUNCIL_MODEL legacy
fallback path in hive (next commit).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
git log --oneline -2
```

Expected: two commits on `feat/route-claude-p-through-modelconfig` (move + pool).

---

## Commit 3 (hive): Switch hive imports to eventgraph/modelconfig

### Task 3.1: Bump eventgraph commit pin and update import paths

**Files:**
- Modify: `hive/pkg/runner/runner.go`
- Modify: `hive/pkg/hive/runtime.go`
- Modify: `hive/pkg/hive/dynamic.go`
- Modify: `hive/pkg/hive/agentdef.go`
- Modify: `hive/cmd/resolve-model/main.go`
- Delete: `hive/pkg/modelconfig/` (entire directory)
- Modify: `hive/go.mod` (no path change required since `replace ../eventgraph/go` already points at the right place, but `go mod tidy` may rewrite hashes)

- [ ] **Step 1: Switch to hive feature branch**

```bash
cd /Users/brewskie/github/mind-zero/hive
git branch --show-current
```

Expected: `feat/route-claude-p-through-modelconfig`. If not, `git checkout feat/route-claude-p-through-modelconfig`.

- [ ] **Step 2: Replace import paths across all 5 files**

```bash
cd /Users/brewskie/github/mind-zero/hive
grep -rln "github.com/transpara-ai/hive/pkg/modelconfig" \
  pkg/runner pkg/hive cmd/resolve-model
```

Expected output:
```
pkg/runner/runner.go
pkg/hive/runtime.go
pkg/hive/dynamic.go
pkg/hive/agentdef.go
cmd/resolve-model/main.go
```

Now replace:

```bash
find pkg/runner pkg/hive cmd/resolve-model -name "*.go" -type f -exec \
  sed -i '' 's|github.com/transpara-ai/hive/pkg/modelconfig|github.com/transpara-ai/eventgraph/go/pkg/modelconfig|g' {} \;

grep -rn "github.com/transpara-ai/hive/pkg/modelconfig" pkg/runner pkg/hive cmd/resolve-model
```

Expected: second grep returns empty.

- [ ] **Step 3: Delete the now-duplicate hive/pkg/modelconfig directory**

```bash
cd /Users/brewskie/github/mind-zero/hive
git rm -r pkg/modelconfig/
```

Expected: 17 files staged for deletion.

- [ ] **Step 4: Verify hive builds cleanly**

```bash
go build ./...
```

If any import is broken, the build fails with `undefined: modelconfig.X` for some symbol. Fix that import.

- [ ] **Step 5: Run hive tests**

```bash
go test ./...
```

Expected: all tests PASS. If any test directly imported `github.com/transpara-ai/hive/pkg/modelconfig`, it will fail at compile. Find with `grep -rn "github.com/transpara-ai/hive/pkg/modelconfig" .` and fix.

- [ ] **Step 6: Run go mod tidy in case anything shifted**

```bash
go mod tidy
go build ./...
```

Expected: no changes (or minor go.sum updates). Build remains clean.

- [ ] **Step 7: Commit the hive import migration**

```bash
git status --short
git add pkg/runner/runner.go pkg/hive/runtime.go pkg/hive/dynamic.go \
        pkg/hive/agentdef.go cmd/resolve-model/main.go go.mod go.sum
git commit -m "$(cat <<'EOF'
refactor: import modelconfig from eventgraph; drop hive/pkg/modelconfig

modelconfig now lives in eventgraph/go/pkg/modelconfig (see eventgraph
commits "refactor: move modelconfig from hive to eventgraph" and
"feat(modelconfig): add ProviderPool with dedup, warm-up, and stats").
Hive imports it from there. The hive copy is deleted; behavior is
unchanged.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
git log --oneline -1
```

Expected: commit `refactor: import modelconfig from eventgraph; drop hive/pkg/modelconfig` on the feature branch.

---

## Commit 4 (hive): Council honors --catalog via ProviderPool

### Task 4.1: Add Pool field to runner.Config

**Files:**
- Modify: `hive/pkg/runner/runner.go` (Config struct)

- [ ] **Step 1: Read the current Config struct**

```bash
grep -A 30 "^type Config struct" /Users/brewskie/github/mind-zero/hive/pkg/runner/runner.go
```

Expected: list of fields. Note where `Provider` is declared and add the new field beneath it.

- [ ] **Step 2: Add Pool field**

Edit `hive/pkg/runner/runner.go`. Find the `Config` struct and add (preserving the package's `modelconfig` import which now points at eventgraph):

```go
type Config struct {
	// ... existing fields ...
	Provider     intelligence.IIntelligence
	Pool         *modelconfig.ProviderPool // set by council; nil for civilization/pipeline/role modes
	// ... rest of existing fields ...
}
```

Place it directly after `Provider` so reviewers see them as a pair.

- [ ] **Step 3: Verify build**

```bash
cd /Users/brewskie/github/mind-zero/hive
go build ./...
```

Expected: clean.

### Task 4.2: Add --catalog flag to council router

**Files:**
- Modify: `hive/cmd/hive/router.go` (`cmdCouncil`)

- [ ] **Step 1: Write a failing CLI flag test**

Create `hive/cmd/hive/router_test.go` (extend if it exists). Add:

```go
func TestCmdCouncil_AcceptsCatalogFlag(t *testing.T) {
	// We don't actually run the council; we just verify the flag set
	// is parsed without error.
	fs := newCouncilFlagSet()
	args := []string{"--catalog", "/tmp/test-catalog.yaml", "--topic", "x"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := fs.Lookup("catalog").Value.String(); got != "/tmp/test-catalog.yaml" {
		t.Errorf("--catalog value=%q; want /tmp/test-catalog.yaml", got)
	}
}
```

This requires factoring out the flag set construction. Add a helper near `cmdCouncil` in router.go:

```go
func newCouncilFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("council", flag.ContinueOnError)
	fs.String("space", "hive", "lovyou.ai space slug")
	fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	fs.String("repo", "", "Path to repo (default: current dir)")
	fs.Float64("budget", 10.0, "Daily budget in USD")
	fs.String("topic", "", "Focus the council on a specific question")
	fs.String("catalog", "", "Custom YAML model catalog (merged with built-in defaults)")
	return fs
}
```

And refactor `cmdCouncil` to use it:

```go
func cmdCouncil(args []string) error {
	fs := newCouncilFlagSet()
	if err := fs.Parse(args); err != nil {
		return err
	}
	space := fs.Lookup("space").Value.String()
	apiBase := fs.Lookup("api").Value.String()
	repo := fs.Lookup("repo").Value.String()
	budget, _ := strconv.ParseFloat(fs.Lookup("budget").Value.String(), 64)
	topic := fs.Lookup("topic").Value.String()
	catalog := fs.Lookup("catalog").Value.String()
	return runCouncilCmd(space, apiBase, repo, budget, topic, catalog)
}
```

Add the `"strconv"` import if not present.

- [ ] **Step 2: Run the new test (will fail until runCouncilCmd signature is bumped)**

```bash
cd /Users/brewskie/github/mind-zero/hive
go test -run TestCmdCouncil_AcceptsCatalogFlag ./cmd/hive/
```

Expected: compile error — `runCouncilCmd` takes 5 args, not 6. Continue to Task 4.3.

### Task 4.3: Rewrite runCouncilCmd to build a pool

**Files:**
- Modify: `hive/cmd/hive/main.go` (`runCouncilCmd` function around line 504)

- [ ] **Step 1: Update the runCouncilCmd signature and body**

In `hive/cmd/hive/main.go`, replace the current `runCouncilCmd` function (lines 504-542) with:

```go
func runCouncilCmd(space, apiBase, repoPath string, budget float64, topic, catalogPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiKey := os.Getenv("LOVYOU_API_KEY")
	client := api.New(apiBase, apiKey)

	if repoPath == "" {
		repoPath = "."
	}
	absRepo, _ := filepath.Abs(repoPath)
	hiveDir := findHiveDir()

	resolver, err := buildCouncilResolver(catalogPath)
	if err != nil {
		return fmt.Errorf("resolver: %w", err)
	}
	pool := modelconfig.NewProviderPool(resolver)

	return runner.RunCouncil(ctx, runner.Config{
		SpaceSlug:    space,
		RepoPath:     absRepo,
		HiveDir:      hiveDir,
		APIClient:    client,
		APIBase:      apiBase,
		Pool:         pool,
		BudgetUSD:    budget,
		CouncilTopic: topic,
	})
}

// buildCouncilResolver returns a Resolver for the council. Precedence:
//   1. --catalog flag wins if non-empty.
//   2. COUNCIL_MODEL env var fallback ONLY when --catalog is unset.
//   3. DefaultResolver().
// Decision 5a of the design spec.
func buildCouncilResolver(catalogPath string) (*modelconfig.Resolver, error) {
	if catalogPath != "" {
		if env := os.Getenv("COUNCIL_MODEL"); env != "" {
			log.Printf("[council] note: --catalog provided; COUNCIL_MODEL=%s env var ignored", env)
		}
		return modelconfig.ResolverFromCatalogFile(catalogPath)
	}
	if env := os.Getenv("COUNCIL_MODEL"); env != "" {
		r, err := modelconfig.DefaultResolverWithModel(env)
		if err != nil {
			return nil, fmt.Errorf("COUNCIL_MODEL=%q: %w", env, err)
		}
		log.Printf("[council] legacy mode: COUNCIL_MODEL=%s → synthetic single-model resolver", env)
		return r, nil
	}
	return modelconfig.DefaultResolver(), nil
}
```

Add the `modelconfig` import if not already present:

```go
"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
```

Remove unused imports (the old hardcoded provider config no longer needs `"github.com/transpara-ai/eventgraph/go/pkg/intelligence"` here if it's not used elsewhere in main.go; `goimports` will tidy on save).

- [ ] **Step 2: Verify the build**

```bash
go build ./...
```

Expected: clean. If `runner.Config` doesn't have `Pool` yet, Task 4.1 wasn't applied.

- [ ] **Step 3: Verify the router test now passes**

```bash
go test -run TestCmdCouncil_AcceptsCatalogFlag ./cmd/hive/
```

Expected: PASS.

### Task 4.4: Write failing council Pool integration test

**Files:**
- Modify: `hive/pkg/runner/council_test.go` (create if absent)

- [ ] **Step 1: Inspect existing council tests**

```bash
ls /Users/brewskie/github/mind-zero/hive/pkg/runner/ | grep council
```

If a `council_test.go` exists, append tests to it; otherwise create a new one.

- [ ] **Step 2: Write the Pool-consumption test**

Append (or create) `hive/pkg/runner/council_test.go`:

```go
package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
)

// recordingFakeProvider records every Reason() call so tests can
// assert on which roles invoked it. Does not implement IOperator.
type recordingFakeProvider struct {
	name  string
	model string
	calls *[]string
}

func (f *recordingFakeProvider) Name() string  { return f.name }
func (f *recordingFakeProvider) Model() string { return f.model }
func (f *recordingFakeProvider) Reason(_ context.Context, prompt string, _ []event.Event) (decision.Response, error) {
	*f.calls = append(*f.calls, prompt)
	return decision.NewResponse("fake reply from "+f.name, decision.TokenUsage{}), nil
}

// helper that wraps the recording fake with a per-role label so
// we can identify which member called us.
func fakeBuilderFor(callsByModel map[string]*[]string) func(intelligence.Config) (intelligence.IIntelligence, error) {
	return func(cfg intelligence.Config) (intelligence.IIntelligence, error) {
		key := cfg.Provider + "/" + cfg.Model
		if _, ok := callsByModel[key]; !ok {
			calls := []string{}
			callsByModel[key] = &calls
		}
		return &recordingFakeProvider{name: cfg.Provider, model: cfg.Model, calls: callsByModel[key]}, nil
	}
}

func TestRunCouncil_UsesPoolWhenSet(t *testing.T) {
	tmp := t.TempDir()
	// Set up minimal hiveDir/agents structure with two role files.
	mustWrite(t, filepath.Join(tmp, "agents/planner.md"), "# Planner\nYou are the Planner.")
	mustWrite(t, filepath.Join(tmp, "agents/strategist.md"), "# Strategist\nYou are the Strategist.")

	calls := map[string]*[]string{}
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

	// Planner + Strategist both resolve to sonnet/claude-cli → one cached provider.
	key := "claude-cli/claude-sonnet-4-6"
	if calls[key] == nil {
		t.Fatalf("expected calls under %s; got map=%v", key, calls)
	}
	if got := len(*calls[key]); got != 2 {
		t.Errorf("expected 2 calls to shared provider (one per member); got %d", got)
	}
	if len(calls) != 1 {
		t.Errorf("expected 1 unique provider in cache; got %d", len(calls))
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
```

Add the `"os"` import to the test file.

- [ ] **Step 3: Run the test to verify it fails**

```bash
cd /Users/brewskie/github/mind-zero/hive
go test -run TestRunCouncil_UsesPoolWhenSet ./pkg/runner/
```

Expected: failure — `RunCouncil` still uses `cfg.Provider`, not `cfg.Pool`, so the recording fake is never called. Continue to Task 4.5.

### Task 4.5: Refactor RunCouncil to use Pool per-member

**Files:**
- Modify: `hive/pkg/runner/council.go` (`RunCouncil` function)

- [ ] **Step 1: Replace the per-member dispatch logic**

In `hive/pkg/runner/council.go`, replace the body of the per-member goroutine (currently around lines 55-97 — the `go func(m *councilMember) { ... }` block) with:

```go
		go func(m *councilMember) {
			defer wg.Done()

			provider, err := cfg.Pool.For(m.role)
			if err != nil {
				log.Printf("[council] %s: pool.For: %v", m.role, err)
				m.response = fmt.Sprintf("(could not contribute: %v)", err)
				return
			}

			op, canOperate := provider.(decision.IOperator)

			if canOperate {
				apiKey := os.Getenv("LOVYOU_API_KEY")
				instruction := buildCouncilOperateInstruction(m.role, m.prompt, cfg.CouncilTopic, cfg.SpaceSlug, apiKey, cfg.APIBase)
				result, err := op.Operate(ctx, decision.OperateTask{
					WorkDir:     cfg.HiveDir,
					Instruction: instruction,
				})
				if err != nil {
					log.Printf("[council] %s: error: %v", m.role, err)
					m.response = fmt.Sprintf("(could not contribute: %v)", err)
					return
				}
				mu.Lock()
				totalCost += result.Usage.CostUSD
				mu.Unlock()
				m.response = result.Summary
				log.Printf("[council] %s spoke ($%.4f)", m.role, result.Usage.CostUSD)
			} else {
				prompt := buildCouncilPrompt(m.role, m.prompt, sharedContext, cfg.CouncilTopic)
				resp, err := provider.Reason(ctx, prompt, nil)
				if err != nil {
					log.Printf("[council] %s: error: %v", m.role, err)
					m.response = fmt.Sprintf("(could not contribute: %v)", err)
					return
				}
				mu.Lock()
				totalCost += resp.Usage().CostUSD
				mu.Unlock()
				m.response = resp.Content()
				log.Printf("[council] %s spoke ($%.4f)", m.role, resp.Usage().CostUSD)
			}
		}(&members[i])
```

The key change: `provider, err := cfg.Pool.For(m.role)` is called inside the goroutine and replaces `cfg.Provider`. The type assertion `op, canOperate := provider.(decision.IOperator)` is now against the resolved per-role provider, not the shared one.

Also remove the now-unused top-level type assertion above the loop (the `op, canOperate := cfg.Provider.(decision.IOperator)` line and its preceding comment) — that decision is now per-member, not global.

- [ ] **Step 2: Add a warm-up step before fan-out**

In the same `RunCouncil` function, insert this block immediately after the `log.Printf("[council] %d agents assembled: ...")` line:

```go
	// Pre-resolve every member's provider so config errors surface before fan-out
	// and so the unique-provider summary is logged once.
	roleNames := make([]string, 0, len(members))
	for _, m := range members {
		roleNames = append(roleNames, m.role)
	}
	if err := cfg.Pool.WarmForRoles(roleNames); err != nil {
		return fmt.Errorf("council pool warm-up: %w", err)
	}
	stats := cfg.Pool.Stats()
	log.Printf("[council] %s", stats.Summary())
```

- [ ] **Step 3: Run the council test**

```bash
go test -run TestRunCouncil_UsesPoolWhenSet ./pkg/runner/ -v
```

Expected: PASS.

### Task 4.6: Council CanOperate≠IOperator warning

- [ ] **Step 1: Write the failing warning test**

Append to `hive/pkg/runner/council_test.go`:

```go
// fakeOperator implements IOperator so we can flip behaviors.
type fakeOperator struct {
	*recordingFakeProvider
}

func (f *fakeOperator) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	return decision.OperateResult{Summary: "operated"}, nil
}

func TestRunCouncil_WarnsOnCanOperateMismatch(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "agents/implementer.md"), "# Implementer\nYou are the Implementer.")

	// Stub AgentDef registry so implementer is known to be CanOperate=true.
	defer overrideCouncilAgentDefs(map[string]bool{
		"implementer": true,
	})()

	calls := map[string]*[]string{}
	r := modelconfig.DefaultResolver()
	pool := modelconfig.NewProviderPoolWithBuilder(r, fakeBuilderFor(calls)) // recording fake does NOT implement IOperator

	logBuf := captureLog(t)
	defer logBuf.restore()

	err := RunCouncil(context.Background(), Config{
		HiveDir: tmp,
		Pool:    pool,
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
```

This test references two helpers we need to write: `overrideCouncilAgentDefs` (a test hook that maps role → CanOperate flag) and `captureLog` (redirects `log` output to a buffer). Add them at the bottom of `council_test.go`:

```go
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

// overrideCouncilAgentDefs swaps the package-level lookup that the
// council uses to check CanOperate. Returns a restore function.
func overrideCouncilAgentDefs(canOperateByRole map[string]bool) func() {
	orig := lookupCanOperateForRole
	lookupCanOperateForRole = func(role string) bool { return canOperateByRole[role] }
	return func() { lookupCanOperateForRole = orig }
}
```

Add imports: `"bytes"`, `"io"`, `"log"`, `"strings"`.

- [ ] **Step 2: Verify failure**

```bash
go test -run TestRunCouncil_WarnsOnCanOperateMismatch ./pkg/runner/
```

Expected: compile error — `undefined: lookupCanOperateForRole`. Continue.

- [ ] **Step 3: Add the lookup hook and warning code**

In `hive/pkg/runner/council.go`, add near the top of the file:

```go
// lookupCanOperateForRole returns whether a role corresponds to a
// registered AgentDef with CanOperate=true. Indirected through a
// package variable so tests can swap it.
var lookupCanOperateForRole = func(role string) bool {
	defs := hive.StarterAgentDefs()
	for _, def := range defs {
		if def.Role == role {
			return def.CanOperate
		}
	}
	return false
}
```

Add the import `"github.com/transpara-ai/hive/pkg/hive"` if not already present. If `StarterAgentDefs()` doesn't exist or returns under a different name, look in `pkg/hive/agentdef.go` for the actual export and use that — there is an exported function that returns the registered AgentDef slice.

Then, in the warm-up block (just below the `log.Printf("[council] %s", stats.Summary())` line you added in Task 4.5), add:

```go
	for _, m := range members {
		if !lookupCanOperateForRole(m.role) {
			continue
		}
		provider, err := cfg.Pool.For(m.role) // cache hit; just a read
		if err != nil {
			continue
		}
		if _, ok := provider.(decision.IOperator); !ok {
			model := "?"
			if named, ok := provider.(interface{ Name() string; Model() string }); ok {
				model = named.Name() + "/" + named.Model()
			}
			log.Printf("[council] warning: role %s is CanOperate=true but resolved provider %s does not implement IOperator; Operate() calls will fall back to Reason()", m.role, model)
		}
	}
```

- [ ] **Step 4: Verify the warning test passes**

```bash
go test -run TestRunCouncil_WarnsOnCanOperateMismatch ./pkg/runner/ -v
```

Expected: PASS.

### Task 4.7: Test catalog-precedence (table-driven)

- [ ] **Step 1: Write the table test**

Append to `hive/cmd/hive/main_test.go` (create if absent):

```go
package main

import (
	"os"
	"strings"
	"testing"
)

func TestBuildCouncilResolver_CatalogPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		catalogPath string
		envValue    string
		wantErr     bool
		// We do not assert on resolver internals — only that buildCouncilResolver
		// returns the right shape for each precedence path.
		logMustContain string
	}{
		{
			name:           "no catalog, no env: default resolver",
			catalogPath:    "",
			envValue:       "",
			wantErr:        false,
			logMustContain: "",
		},
		{
			name:           "env set, no catalog: legacy mode logged",
			catalogPath:    "",
			envValue:       "sonnet",
			wantErr:        false,
			logMustContain: "legacy mode: COUNCIL_MODEL=sonnet",
		},
		{
			name:           "catalog and env: env ignored, note logged",
			catalogPath:    writeMinimalCatalog(t),
			envValue:       "sonnet",
			wantErr:        false,
			logMustContain: "COUNCIL_MODEL=sonnet env var ignored",
		},
		{
			name:           "unknown COUNCIL_MODEL: error",
			catalogPath:    "",
			envValue:       "not-a-real-model",
			wantErr:        true,
			logMustContain: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("COUNCIL_MODEL", tt.envValue)
			} else {
				os.Unsetenv("COUNCIL_MODEL")
			}
			logBuf := captureLogMain(t)
			defer logBuf.restore()

			_, err := buildCouncilResolver(tt.catalogPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildCouncilResolver: err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.logMustContain != "" && !strings.Contains(logBuf.String(), tt.logMustContain) {
				t.Errorf("log missing %q; got: %s", tt.logMustContain, logBuf.String())
			}
		})
	}
}

func writeMinimalCatalog(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/catalog.yaml"
	body := `models:
  - id: claude-sonnet-4-6
    aliases: [sonnet]
    provider: claude-cli
    auth_mode: subscription
    tier: execution
    capabilities: [tools, reasoning]
    context_window: 200000
    max_output_tokens: 8192
    pricing:
      input_per_million: 3.00
      output_per_million: 15.00

tier_defaults:
  judgment: sonnet
  execution: sonnet
  volume: sonnet
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// captureLogMain is the main-package equivalent of council_test.go's helper.
type logCaptureMain struct {
	original interface{ Write(p []byte) (int, error) }
	buf      *bytes.Buffer
}

func captureLogMain(t *testing.T) *logCaptureMain {
	t.Helper()
	buf := &bytes.Buffer{}
	orig := log.Writer()
	log.SetOutput(buf)
	return &logCaptureMain{original: orig, buf: buf}
}

func (l *logCaptureMain) String() string { return l.buf.String() }
func (l *logCaptureMain) restore() {
	if w, ok := l.original.(io.Writer); ok {
		log.SetOutput(w)
	}
}
```

Add imports: `"bytes"`, `"io"`, `"log"`.

- [ ] **Step 2: Verify the test passes**

```bash
cd /Users/brewskie/github/mind-zero/hive
go test -run TestBuildCouncilResolver_CatalogPrecedence ./cmd/hive/ -v
```

Expected: all 4 subtests PASS.

### Task 4.8: Run all hive tests with -race

- [ ] **Step 1: Full suite**

```bash
go test -race ./... 2>&1 | tail -30
go vet ./...
```

Expected: all PASS, no vet warnings.

### Task 4.9: Commit council changes

- [ ] **Step 1: Stage and commit**

```bash
git add cmd/hive/main.go cmd/hive/router.go cmd/hive/router_test.go cmd/hive/main_test.go \
        pkg/runner/runner.go pkg/runner/council.go pkg/runner/council_test.go
git status --short
```

Expected: those files staged, no others modified.

```bash
git commit -m "$(cat <<'EOF'
feat(council): honor --catalog and route per-role via ProviderPool

cmd/hive council [...] now accepts a --catalog flag. Replaces the
hardcoded intelligence.Config{Provider: "claude-cli", ...} block with
a ProviderPool built from the resolved catalog. Each council member
resolves through the pool inside its goroutine, so 50 members sharing
one model spawn 1 provider rather than 50.

Backward-compat (decision 5a of the design):
  --catalog absent + COUNCIL_MODEL set    → synthetic single-model
  --catalog absent + COUNCIL_MODEL unset  → DefaultResolver()
  --catalog present + COUNCIL_MODEL set   → catalog wins; env ignored
                                            (logged so debugging is easy)

Logs the unique-provider summary at warm-up time:
  [council] 47 members → 2 unique providers ...

Decision 5b warning: when a council member's role matches a registered
AgentDef with CanOperate=true AND the resolved provider does not
implement decision.IOperator, log a warning that Operate() calls will
fall back to Reason(). Advisory only — does not block the council run.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

Expected: commit `feat(council): honor --catalog and route per-role via ProviderPool` on the feature branch.

---

## Commit 5 (hive): Spawn warning

### Task 5.1: Write failing test for spawn warning

**Files:**
- Modify: `hive/pkg/hive/runtime_test.go` (extend existing)
- Modify: `hive/pkg/hive/runtime.go` (`spawnAgent`)

- [ ] **Step 1: Check runtime_test.go structure**

```bash
ls /Users/brewskie/github/mind-zero/hive/pkg/hive/runtime_test.go && head -40 /Users/brewskie/github/mind-zero/hive/pkg/hive/runtime_test.go
```

If the file already exists, append. Otherwise, create it with the package declaration and imports needed (`testing`, `bytes`, `log`, `strings`).

- [ ] **Step 2: Append warning tests**

Append to `hive/pkg/hive/runtime_test.go`:

```go
func TestSpawnAgent_WarnsWhenCanOperateButProviderLacksIOperator(t *testing.T) {
	// Test setup: a runtime with a stub resolver/pool that returns a
	// non-IOperator provider for the implementer role, paired with an
	// AgentDef that has CanOperate=true.
	tests := []struct {
		name          string
		canOperate    bool
		providerOps   bool
		expectWarning bool
	}{
		{name: "CanOperate=true + non-IOperator → warn", canOperate: true, providerOps: false, expectWarning: true},
		{name: "CanOperate=false + non-IOperator → no warn", canOperate: false, providerOps: false, expectWarning: false},
		{name: "CanOperate=true + IOperator → no warn", canOperate: true, providerOps: true, expectWarning: false},
		{name: "CanOperate=false + IOperator → no warn", canOperate: false, providerOps: true, expectWarning: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			origOut := log.Writer()
			log.SetOutput(buf)
			defer log.SetOutput(origOut)

			emitWarning := canOperateMismatch(tt.canOperate, tt.providerOps)
			if emitWarning != tt.expectWarning {
				t.Errorf("canOperateMismatch returned %v; want %v", emitWarning, tt.expectWarning)
			}
		})
	}
}
```

The test references `canOperateMismatch` — a small helper we will extract from `spawnAgent` so it's directly testable without standing up a full runtime.

- [ ] **Step 3: Verify failure**

```bash
go test -run TestSpawnAgent_WarnsWhenCanOperateButProviderLacksIOperator ./pkg/hive/
```

Expected: compile error — `undefined: canOperateMismatch`. Continue.

### Task 5.2: Implement canOperateMismatch + integrate in spawnAgent

- [ ] **Step 1: Add the helper**

In `hive/pkg/hive/runtime.go`, near the top of the file:

```go
// canOperateMismatch returns true when an agent flagged CanOperate=true
// has been resolved to a provider that does not implement IOperator.
// The check is factored out so it can be unit-tested without standing
// up a full runtime.
func canOperateMismatch(canOperate, providerSupportsOperate bool) bool {
	return canOperate && !providerSupportsOperate
}
```

- [ ] **Step 2: Wire it into spawnAgent**

In `hive/pkg/hive/runtime.go`, in `spawnAgent` immediately after the `intelligence.New(cfg)` call (around line 517–520):

```go
	provider, err := intelligence.New(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("provider: %w", err)
	}

	// Decision 5b warning.
	_, supportsOperate := provider.(decision.IOperator)
	if canOperateMismatch(def.CanOperate, supportsOperate) {
		log.Printf("[runtime] warning: agent %s (role %s) is CanOperate=true but resolved provider %s/%s does not implement IOperator; Operate() calls will fall back to Reason()",
			def.Name, def.Role, resolved.Provider, resolved.Model)
	}
```

Add `"github.com/transpara-ai/eventgraph/go/pkg/decision"` and `"log"` to the imports if not already present.

- [ ] **Step 3: Verify tests pass**

```bash
go test -run TestSpawnAgent ./pkg/hive/ -v
```

Expected: all 4 subtests PASS.

### Task 5.3: Run hive-wide tests

```bash
go test -race ./... 2>&1 | tail -20
```

Expected: clean.

### Task 5.4: Commit spawn warning

- [ ] **Step 1: Stage and commit**

```bash
git add pkg/hive/runtime.go pkg/hive/runtime_test.go
git commit -m "$(cat <<'EOF'
feat(spawn): log warning when CanOperate=true resolves to non-IOperator

Decision 5b of the design spec. Mirrors the warning already added to
the council flow but at civilization-mode agent spawn time. The
fallback to Reason() is legitimate behavior; the warning makes
accidental capability loss visible (e.g., when someone configures
implementer to route to a model whose provider — OpenRouter, Anthropic
HTTP, etc. — does not implement IOperator).

Factors out canOperateMismatch as a small, table-tested helper so the
boolean truth table is verified without standing up a full Runtime.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

Expected: commit on the feature branch.

---

## Commit 6 (hive): Retire run.sh

### Task 6.1: Replace run.sh body

**Files:**
- Modify: `hive/loop/run.sh`

- [ ] **Step 1: Replace the entire script content**

Overwrite `/Users/brewskie/github/mind-zero/hive/loop/run.sh` with:

```bash
#!/usr/bin/env bash
# DEPRECATED 2026-05-14 — replaced by the Go pipeline.
# This bash runner bypassed the modelconfig resolver and pinned every role
# to `claude -p`. The Go pipeline honors --catalog and per-role policy.
echo "hive/loop/run.sh is deprecated as of 2026-05-14." >&2
echo "Use: go run ./cmd/hive pipeline run --catalog <path>" >&2
echo "See hive/CLAUDE.md for current pipeline usage." >&2
exit 1
```

- [ ] **Step 2: Verify executability is preserved**

```bash
ls -l /Users/brewskie/github/mind-zero/hive/loop/run.sh
```

Expected: still executable (`-rwxr-xr-x` or similar).

- [ ] **Step 3: Smoke test**

```bash
/Users/brewskie/github/mind-zero/hive/loop/run.sh
echo "exit code: $?"
```

Expected output:
```
hive/loop/run.sh is deprecated as of 2026-05-14.
Use: go run ./cmd/hive pipeline run --catalog <path>
See hive/CLAUDE.md for current pipeline usage.
exit code: 1
```

### Task 6.2: Commit

- [ ] **Step 1: Stage and commit**

```bash
cd /Users/brewskie/github/mind-zero/hive
git add loop/run.sh
git commit -m "$(cat <<'EOF'
chore(loop): retire run.sh with deprecation banner

The legacy bash pipeline runner shelled out directly to
`claude --dangerously-skip-permissions -p` and bypassed the
modelconfig resolver. The Go pipeline at `cmd/hive pipeline run`
supersedes it functionally and honors --catalog. Script body is
replaced with a redirect notice and `exit 1` so existing invocations
fail loudly with a migration path.

Decision 3 of the design spec.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

Expected: commit on the feature branch.

---

## Commit 7 (site): Mind refactor

### Task 7.1: Switch to site feature branch + verify clean state

**Files:** none modified — verification only.

- [ ] **Step 1: Switch branches**

```bash
cd /Users/brewskie/github/mind-zero/site
git branch --show-current
```

Expected: `feat/route-claude-p-through-modelconfig`. If not, `git checkout feat/route-claude-p-through-modelconfig`.

- [ ] **Step 2: Verify clean tree**

```bash
git status --short
```

Expected: empty. If anything shows (e.g., templ regenerations from earlier), they were stashed in Task 0; if they reappeared, re-stash:

```bash
git stash push -u -m "pre-modelconfig-routing local work"
```

### Task 7.2: Bump go.mod to include eventgraph modelconfig + rename callClaude → callProvider

**Files:**
- Modify: `site/go.mod` (may auto-update on `go mod tidy`)
- Modify: `site/graph/mind.go` (renames)
- Modify: `site/graph/mind_test.go` (rename fixtures)

- [ ] **Step 1: Apply renames mechanically**

```bash
cd /Users/brewskie/github/mind-zero/site
# Method name
sed -i '' 's/\bcallClaude\b/callProvider/g' graph/mind.go graph/mind_test.go
# Field name
sed -i '' 's/\bcallClaudeOverride\b/callProviderOverride/g' graph/mind.go graph/mind_test.go
# Type name
sed -i '' 's/\bclaudeMessage\b/providerMessage/g' graph/mind.go graph/mind_test.go
grep -n "callClaude\|claudeMessage" graph/mind.go graph/mind_test.go
```

Expected: second grep returns empty.

- [ ] **Step 2: Build to confirm renames are consistent**

```bash
go build ./graph/...
```

Expected: clean. If anything fails, fix the missing reference.

### Task 7.3: Add Pool to Mind and update NewMind signature

**Files:**
- Modify: `site/graph/mind.go`

- [ ] **Step 1: Update the Mind struct and constructor**

Edit `site/graph/mind.go`. Replace the struct declaration (around lines 18-26) and `NewMind` (lines 29-36) with:

```go
// Mind responds to conversation messages via the configured provider pool.
// Event-driven: triggered by handlers when a human messages in an agent conversation.
type Mind struct {
	db                   *sql.DB
	store                *Store
	pool                 *modelconfig.ProviderPool
	replyTimeout         time.Duration
	// callProviderOverride is set in tests to stub out provider calls without a real backend.
	callProviderOverride func(ctx context.Context, systemPrompt string, messages []providerMessage) (string, error)
}

// NewMind creates a Mind that auto-replies in agent conversations using the given provider pool.
func NewMind(db *sql.DB, store *Store, pool *modelconfig.ProviderPool) *Mind {
	return &Mind{
		db:           db,
		store:        store,
		pool:         pool,
		replyTimeout: 5 * time.Minute,
	}
}
```

Add the import:

```go
"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
```

- [ ] **Step 2: Audit token-field references**

```bash
grep -n "m\.token\b\|\.token\b" /Users/brewskie/github/mind-zero/site/graph/mind.go
```

The earlier `m.token` was passed to `exec.Command`'s env via `CLAUDE_CODE_OAUTH_TOKEN`. After we delete `callProvider`'s exec path (next task), `m.token` will be unreferenced. If grep returns any references outside the `callProvider` body that we're about to rewrite, surface them — they need explicit handling. Most likely none exist.

### Task 7.4: Rewrite callProvider to use the pool

**Files:**
- Modify: `site/graph/mind.go` (the `callProvider` function, formerly `callClaude`)

- [ ] **Step 1: Write the failing test first**

Append (or modify) `site/graph/mind_test.go` with:

```go
func TestMind_CallProvider_RoutesThroughPool(t *testing.T) {
	r := modelconfig.DefaultResolver()
	called := 0
	builder := func(cfg intelligence.Config) (intelligence.IIntelligence, error) {
		return &recordingFakeProvider{
			name:     cfg.Provider,
			model:    cfg.Model,
			incCalls: func() { called++ },
		}, nil
	}
	pool := modelconfig.NewProviderPoolWithBuilder(r, builder)

	m := NewMind(nil, nil, pool)
	out, err := m.callProvider(context.Background(), "planner", "system prompt", []providerMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("callProvider: %v", err)
	}
	if out == "" {
		t.Errorf("empty response")
	}
	if called != 1 {
		t.Errorf("expected 1 provider call; got %d", called)
	}
}

// recordingFakeProvider is the local-to-site test fake.
type recordingFakeProvider struct {
	name, model string
	incCalls    func()
}

func (f *recordingFakeProvider) Name() string  { return f.name }
func (f *recordingFakeProvider) Model() string { return f.model }
func (f *recordingFakeProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	f.incCalls()
	return decision.NewResponse("fake reply", decision.TokenUsage{}), nil
}
```

Imports needed in the test file: `"context"`, `"testing"`, plus the eventgraph packages.

- [ ] **Step 2: Verify failure**

```bash
cd /Users/brewskie/github/mind-zero/site
go test -run TestMind_CallProvider_RoutesThroughPool ./graph/
```

Expected: compile error — `callProvider` still uses `exec.Command`, doesn't take `role`. Continue.

- [ ] **Step 3: Replace the callProvider implementation**

In `site/graph/mind.go`, replace the entire `callProvider` function body (formerly `callClaude` at line ~917) with:

```go
func (m *Mind) callProvider(ctx context.Context, role, systemPrompt string, messages []providerMessage) (string, error) {
	if m.callProviderOverride != nil {
		return m.callProviderOverride(ctx, systemPrompt, messages)
	}
	provider, err := m.pool.For(role)
	if err != nil {
		return "", fmt.Errorf("resolve provider for %s: %w", role, err)
	}
	var prompt strings.Builder
	prompt.WriteString(systemPrompt)
	prompt.WriteString("\n== MESSAGES ==\n")
	for _, msg := range messages {
		prompt.WriteString(msg.Content)
		prompt.WriteString("\n\n")
	}
	resp, err := provider.Reason(ctx, prompt.String(), nil)
	if err != nil {
		return "", fmt.Errorf("intelligence: %w", err)
	}
	text := strings.TrimSpace(resp.Content())
	if text == "" {
		return "", fmt.Errorf("empty response from %s", role)
	}
	return text, nil
}
```

Remove the now-unused `"os/exec"` import.

- [ ] **Step 4: Update the four callers to pass `role`**

Each caller of `callProvider` already knows the agent name. Update the signatures in `site/graph/mind.go`:

```bash
grep -n "callProvider(" /Users/brewskie/github/mind-zero/site/graph/mind.go
```

For each call site, change `m.callProvider(ctx, systemPrompt, messages)` to `m.callProvider(ctx, agentName, systemPrompt, messages)`. The variable name carrying the role differs per call site — look at each one. Typical names: `agentName` in `replyTo`, `personaName` in `OnCouncilConvened`, etc.

- [ ] **Step 5: Verify the test passes**

```bash
go test -run TestMind_CallProvider_RoutesThroughPool ./graph/ -v
```

Expected: PASS.

### Task 7.5: Delete the Mind.token field after audit

- [ ] **Step 1: Re-audit token references**

```bash
grep -n "\.token\b\|m\.token\b" /Users/brewskie/github/mind-zero/site/graph/mind.go
```

If empty: proceed to deletion. If anything still references `m.token`, surface those references — they need explicit handling (likely the audit reveals a previously-overlooked call site).

- [ ] **Step 2: Remove the field**

In `site/graph/mind.go`, edit the `Mind` struct to remove the `token string` field and the trailing comment `// Claude OAuth token`. The struct should now be exactly:

```go
type Mind struct {
	db                   *sql.DB
	store                *Store
	pool                 *modelconfig.ProviderPool
	replyTimeout         time.Duration
	callProviderOverride func(ctx context.Context, systemPrompt string, messages []providerMessage) (string, error)
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./graph/...
```

Expected: clean.

### Task 7.6: Update cmd/site/main.go to build pool + drop CLAUDE_TOKEN

**Files:**
- Modify: `site/cmd/site/main.go`

- [ ] **Step 1: Locate the existing NewMind call**

```bash
grep -n "NewMind\|CLAUDE_TOKEN\|claudeToken" /Users/brewskie/github/mind-zero/site/cmd/site/main.go
```

Expected: shows the wiring line that constructs the Mind today.

- [ ] **Step 2: Replace the wiring block**

Replace the existing block (the `claudeToken := os.Getenv("CLAUDE_TOKEN")` line and the `NewMind(db, store, claudeToken)` call) with:

```go
resolver, err := buildSiteResolver(os.Getenv("SITE_CATALOG"))
if err != nil {
	log.Fatalf("site: resolver: %v", err)
}
pool := modelconfig.NewProviderPool(resolver)
personaNames := personas.AllNames()
if err := pool.WarmForRoles(personaNames); err != nil {
	log.Fatalf("site: pool warm-up: %v", err)
}
log.Printf("site: mind initialized — %s", pool.Stats().Summary())
mind := graph.NewMind(db, store, pool)
```

And add the helper at the bottom of the file (or in a new `site/cmd/site/resolver.go`):

```go
func buildSiteResolver(catalogPath string) (*modelconfig.Resolver, error) {
	if catalogPath == "" {
		return modelconfig.DefaultResolver(), nil
	}
	return modelconfig.ResolverFromCatalogFile(catalogPath)
}
```

Add the imports:

```go
"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
"github.com/transpara-ai/site/graph/personas"
```

If `personas.AllNames()` doesn't exist, check what exported helper the package offers. The generated `status_gen.go` typically has a map; if so, add an `AllNames() []string` that returns the keys.

- [ ] **Step 3: Verify build**

```bash
cd /Users/brewskie/github/mind-zero/site
templ generate
go build ./...
```

Expected: clean. If `personas.AllNames` is missing, add it in `site/graph/personas/`:

```go
// personas/api.go
package personas

func AllNames() []string {
	names := make([]string, 0, len(HiveStatus))
	for name := range HiveStatus {
		names = append(names, name)
	}
	return names
}
```

(Adjust `HiveStatus` to whatever the generated identifier is.)

### Task 7.7: Update site/CLAUDE.md docs

**Files:**
- Modify: `site/CLAUDE.md`

- [ ] **Step 1: Replace the env-vars table row for CLAUDE_TOKEN**

In `site/CLAUDE.md` find the Environment Variables table. Remove the `CLAUDE_TOKEN` row, add a `SITE_CATALOG` row:

```markdown
| Variable | Purpose |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `PORT` | HTTP port (default 8080) |
| `GOOGLE_OAUTH_ID` / `GOOGLE_OAUTH_SECRET` | OAuth2 credentials |
| `SITE_CATALOG` | Optional path to a YAML model catalog. Defaults to the built-in `eventgraph/go/pkg/modelconfig` catalog. |
| `HIVE_REPO_PATH` | Path to sibling hive repo (default: `../hive`) |
```

- [ ] **Step 2: Add a migration note at the top of the Environment Variables section**

Insert just below the section heading:

```markdown
> **Migration 2026-05-14:** `CLAUDE_TOKEN` is no longer used. The Mind now reads
> credentials through the standard `intelligence` package, which inherits whatever
> auth the configured provider already has (`~/.claude/.credentials.json` for
> Claude CLI, env-var-backed API keys for HTTP providers).
```

### Task 7.8: Run site tests + build

```bash
cd /Users/brewskie/github/mind-zero/site
templ generate
go test -race ./...
go vet ./...
make build
```

Expected: all tests PASS, vet clean, build succeeds.

### Task 7.9: Commit

- [ ] **Step 1: Stage and commit**

```bash
git status --short
git add cmd/site/main.go graph/mind.go graph/mind_test.go graph/personas/ \
        CLAUDE.md go.mod go.sum
git status --short
```

Expected: only the files above staged.

```bash
git commit -m "$(cat <<'EOF'
refactor(mind): route via modelconfig.ProviderPool; rename callClaude→callProvider; drop CLAUDE_TOKEN

Site Mind previously shelled out directly to `claude -p` via
exec.Command, bypassing the modelconfig resolver entirely. After this
change, Mind holds a *modelconfig.ProviderPool initialized at site
startup that pre-warms one provider per loaded persona. Every reply,
council-convene, question-answer, and task-assigned response routes
through the pool.

Renames within mind.go for accuracy:
  callClaude            → callProvider
  callClaudeOverride    → callProviderOverride
  claudeMessage         → providerMessage

The Mind.token field is removed; intelligence.claude_cli reads from
~/.claude/.credentials.json natively and the HTTP providers read env
vars themselves. CLAUDE_TOKEN env var is no longer consulted by site
(migration note added to CLAUDE.md).

New optional env var SITE_CATALOG points at a YAML catalog; without
it, site uses the built-in default resolver (every persona → Sonnet
via claude-cli, matching current behavior).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
git log --oneline -1
```

Expected: commit `refactor(mind): route via modelconfig.ProviderPool; rename callClaude→callProvider; drop CLAUDE_TOKEN` on the feature branch.

---

## Post-implementation verification

### Task 8.1: End-to-end smoke

- [ ] **Step 1: Build everything one more time across all 3 repos**

```bash
cd /Users/brewskie/github/mind-zero/eventgraph/go && go build ./... && go test -race ./pkg/modelconfig/...
cd /Users/brewskie/github/mind-zero/hive          && go build ./... && go test -race ./...
cd /Users/brewskie/github/mind-zero/site          && templ generate && make build && go test -race ./...
```

Expected: all clean.

- [ ] **Step 2: Resolve-model CLI smoke (with the gitignored personal Kimi catalog)**

```bash
cd /Users/brewskie/github/mind-zero/hive
go run ./cmd/resolve-model --role implementer --catalog ./kimi-only-catalog.yaml
go run ./cmd/resolve-model --role planner     --catalog ./kimi-only-catalog.yaml
```

Expected:
- `implementer` → `provider=codex-cli` `model=gpt-5.5`
- `planner` → `provider=openrouter` `model=moonshotai/kimi-latest`

- [ ] **Step 3: Council dry-run with the Kimi catalog**

```bash
cd /Users/brewskie/github/mind-zero/hive
go run ./cmd/hive council --topic "smoke: kimi catalog routing" \
  --catalog ./kimi-only-catalog.yaml --budget 0.50
```

Expected log line:
```
[council] N members → ≤2 unique providers ...
```

If the implementer role is on Codex per the Kimi catalog, you'll see a `[council] warning: role implementer is CanOperate=true but resolved provider gpt-5.5 (codex-cli) does not implement IOperator...` line **only if** codex-cli isn't installed locally; with codex-cli present, no warning fires.

- [ ] **Step 4: Final commit log review**

```bash
for repo in eventgraph hive site; do
  echo "=== $repo ==="
  git -C /Users/brewskie/github/mind-zero/$repo log --oneline -10
  echo
done
```

Expected order:
- eventgraph branch shows the design-spec commit (existing on hive) is irrelevant for eventgraph; eventgraph's branch shows 2 new commits (move + ProviderPool).
- hive branch shows 5 new commits (design spec + 4 implementation: import bump, council, spawn warning, run.sh).
- site branch shows 1 new commit (mind refactor).

### Task 8.2: Restore stashed work (if any)

- [ ] **Step 1: List stashes**

```bash
for repo in hive site; do
  echo "=== $repo ==="
  git -C /Users/brewskie/github/mind-zero/$repo stash list | head -5
done
```

If stash entries from Task 0 remain, decide per-repo whether to:
- Pop them onto the feature branch (`git stash pop`) — only if the changes are genuinely part of this feature work.
- Pop them onto a separate housekeeping branch (`git checkout -b chore/pre-modelconfig-housekeeping && git stash pop`) — typical case for ambient local-dev work like templ regens.
- Drop them (`git stash drop`) — only if confirmed safe.

Do not silently merge unrelated work into the feature branch.

---

## Self-review (post-write)

Re-running the brainstorming spec's §10 (out of scope) and §6.1 test list against this plan:

- **Spec §2 decisions (5+1 trade-off):** each gets a task. Q1 (move) = Task 1.1; Q2 (cache at startup) = Task 7.6; Q3 (run.sh retire) = Task 6.1; Q4 (ProviderPool) = Tasks 2.1–2.10; Q5a (COUNCIL_MODEL fallback) = Tasks 4.3 + 4.7; Q5b (CanOperate warning) = Tasks 4.6 + 5.1; Q2 trade-off doc = handled in spec §6 reload caveat (no plan task needed).
- **Spec §3.1 (5 import sites):** Tasks 3.1 lists all 5 explicitly.
- **Spec §4.1 (council changes):** §4.1.1 (router flag) = Task 4.2; §4.1.2 (runCouncilCmd rewrite) = Task 4.3; §4.1.3 (RunCouncil per-member) = Task 4.5; warm-up summary = Task 4.5 Step 2; CanOperate warning = Task 4.6.
- **Spec §4.2 (site Mind):** Tasks 7.3–7.6.
- **Spec §4.3 (spawnAgent warning):** Tasks 5.1–5.2.
- **Spec §4.4 (run.sh):** Task 6.1.
- **Spec §5 (renames):** Task 7.2 + Task 7.5 (token deletion).
- **Spec §6 (tests):** pool_test.go (Tasks 2.1–2.7), council_test.go (Tasks 4.4, 4.6), main_test.go (Task 4.7), runtime_test.go (Task 5.1), mind_test.go (Task 7.4).
- **Spec §8 (verification commands):** Task 8.1 runs all of them.
- **Spec §9 (commit order):** Tasks 1–7 map 1:1 to commits 1–7.

No placeholders found.

No type/name inconsistencies between tasks (`stableConfigHash`, `ProviderPool`, `For`, `WarmForRoles`, `Stats`, `PoolStats`, `Summary`, `DefaultResolverWithModel`, `canOperateMismatch`, `lookupCanOperateForRole`, `buildCouncilResolver`, `buildSiteResolver` — all referenced consistently across tasks where they appear).

---

Plan complete and saved to `hive/docs/superpowers/plans/2026-05-14-route-claude-p-through-modelconfig-plan.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
