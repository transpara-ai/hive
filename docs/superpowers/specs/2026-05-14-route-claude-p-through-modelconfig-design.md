# Route `claude -p` Through `modelconfig` — Design

**Date:** 2026-05-14
**Branch:** `feat/route-claude-p-through-modelconfig` (exists on both `hive` and `site`)
**Authors:** Bert (operator), Claude (collaborator)
**Status:** design approved, awaiting implementation plan

---

## 1. Problem

Three call sites in the codebase bypass the `modelconfig` resolver and hardcode `claude -p`:

| # | Site | Behavior today |
|---|---|---|
| 1 | `hive/cmd/hive/main.go:runCouncilCmd` (lines 517–527) | Builds `intelligence.Config{Provider: "claude-cli", Model: COUNCIL_MODEL or "sonnet"}` directly. Catalog flag absent. |
| 2 | `hive/pkg/runner/council.go:RunCouncil` | Receives one provider via `cfg.Provider` and shares it across every council member's goroutine. No per-role resolution. |
| 3 | `site/graph/mind.go:callClaude` (line 917) | Shells out directly to `claude -p` with hardcoded model `claude-sonnet-4-6`. Does not consult any resolver. |
| 4 | `hive/loop/run.sh` (line 33) | Legacy bash pipeline pipes prompts to `claude --dangerously-skip-permissions -p` for Scout/Builder/Critic/Reflector. |

Because the council fans out ~50 concurrent goroutines and the site Mind serves 67 personas, these are the *high-traffic* call sites where the modelconfig spec was supposed to matter most. The current state silently defeats:

- per-role model policy (`role_defaults` in YAML)
- catalog-driven provider selection (Kimi/OpenRouter/Codex/etc.)
- per-agent cost estimation and budget enforcement
- the IDecisionMaker / IOperator capability contract

This spec covers wiring all three sites (plus run.sh) through the resolver while solving secondary concerns (provider deduplication, backward compatibility, capability mismatch warnings, package ownership).

## 2. Locked decisions

Five clarifying questions resolved during brainstorming on 2026-05-14:

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Where does `modelconfig` live? | **Move from `hive/pkg/modelconfig` → `eventgraph/go/pkg/modelconfig`** | Modelconfig is substrate, not hive-specific. Site can't import from hive per `site/CLAUDE.md` UI-ownership rule. Eventgraph already hosts `intelligence`; `modelconfig` is its natural sibling. |
| 2 | How does Site Mind resolve per call? | **B — Startup resolution + cache.** Pool builds providers once for every loaded persona at site boot, reuses for every call. | Personas are static config (embedded markdown). Per-call resolution buys no behavior today and incurs N×resolver work. Trade-off documented in §6. |
| 3 | What happens to `hive/loop/run.sh`? | **A — Retire with deprecation banner.** Script body becomes a redirect to `go run ./cmd/hive pipeline run`. | Go pipeline already supersedes it functionally and honors the resolver. Bash-side resolver re-implementation would duplicate work. |
| 4 | Where does provider caching live? | **B — Shared `modelconfig.ProviderPool` helper.** New type in eventgraph/modelconfig with `sync.Map` cache keyed by `(Provider, Model, BaseURL, APIKey)`. | Council and site Mind both need this; future Spawner mass-instantiation will too. Cache belongs adjacent to the resolver, not duplicated in each caller. |
| 5a | `COUNCIL_MODEL` env var fate? | **B — Legacy fallback when no `--catalog`.** Env var preserved; synthesizes a single-model catalog. | Backward compat for existing scripts and `.zshrc` exports. Catalog flag, when present, always wins to avoid the silent-shadow foot-gun. |
| 5b | CanOperate=true resolves to non-IOperator provider? | **B — Log warning.** Type assertion fallback to `Reason()` stays (legitimate behavior for non-Operate roles); spawn-time warning makes accidental capability loss visible. | Hard error would block legitimate cases (e.g., a non-Operate role on Kimi is fine). Silence would hide the foot-gun the modelconfig spec was designed to prevent. |

### 2.1 Trade-off explicitly preserved (re: decision 2)

Neither option A (per-call resolution) nor option B (startup cache) reloads the catalog mid-process. `ResolverFromCatalogFile` is a one-time loader; there is no `fsnotify` in `modelconfig`. **Editing the catalog YAML while the process is running has no effect until restart.** This is true today for hive too. Adding hot-reload is a separate orthogonal piece of work (`Resolver.Reload()` + SIGHUP handler) that should land in `modelconfig` and benefit both consumers equally. Out of scope for this spec.

## 3. Architecture

### 3.1 Module reorganization

`hive/pkg/modelconfig/*` → `eventgraph/go/pkg/modelconfig/*`. Files moving (with `_test.go` siblings):

```
adapter.go      catalog.go        capability.go     defaults.go
defaults_catalog.yaml             policy.go         pricing.go
profile.go      resolve.go        role.go           types.go
```

Package import path changes from `github.com/transpara-ai/hive/pkg/modelconfig` to `github.com/transpara-ai/eventgraph/go/pkg/modelconfig`. No semantic changes — pure relocation. Eventgraph gains no new external dependencies because modelconfig already depends only on `intelligence`, which is in eventgraph.

Hive updates 5 import sites:

- `pkg/runner/runner.go`
- `pkg/hive/runtime.go`
- `pkg/hive/dynamic.go`
- `pkg/hive/agentdef.go` (for `RoleModelPolicy` / `RoleDefinition` field types)
- `cmd/resolve-model/main.go`

Site adds new imports:

- `github.com/transpara-ai/eventgraph/go/pkg/modelconfig`
- `github.com/transpara-ai/eventgraph/go/pkg/intelligence`

### 3.2 New type: `modelconfig.ProviderPool`

New file `eventgraph/go/pkg/modelconfig/pool.go`:

```go
type ProviderPool struct {
    resolver *Resolver
    cache    sync.Map  // key=stableConfigHash, value=intelligence.IIntelligence
    builder  func(intelligence.Config) (intelligence.IIntelligence, error)  // injectable; default intelligence.New
}

func NewProviderPool(r *Resolver) *ProviderPool
func (p *ProviderPool) For(role string, opts ...ResolveOption) (intelligence.IIntelligence, error)
func (p *ProviderPool) ForResolved(rc ResolvedConfig, systemPrompt string) (intelligence.IIntelligence, error)
func (p *ProviderPool) WarmForRoles(roles []string) error
func (p *ProviderPool) Stats() PoolStats
```

`PoolStats` carries: unique provider count, per-model role count, total roles, estimated cost summary (using the existing `EstimateAgentCostsByModel` helper from `modelconfig`).

The cache key is a stable content hash over the four fields that distinguish provider instances. Two roles resolving to identical configs share the same underlying provider. Concurrent `For` calls for the same role race-free (sync.Map guarantees).

The injected `builder` field defaults to `intelligence.New` but is overridable for tests (fakes that record calls without spawning real subprocesses).

### 3.3 Import-graph diagram

```
                          ┌──────────────┐
                          │  eventgraph  │
                          │ ┌──────────┐ │
                          │ │intellig. │◀┐│
                          │ └──────────┘ ││
                          │ ┌──────────┐ ││
                          │ │modelcfg  │─┘│
                          │ │ + Pool   │  │
                          │ └────▲─────┘  │
                          └──────┼────────┘
                                 │
                  ┌──────────────┴──────────────┐
                  │                             │
            ┌─────┴─────┐                ┌──────┴──────┐
            │   hive    │                │    site     │
            │ ┌───────┐ │                │ ┌─────────┐ │
            │ │council│ │                │ │  Mind   │ │
            │ │ Pool  │ │                │ │  Pool   │ │
            │ └───────┘ │                │ └─────────┘ │
            └───────────┘                └─────────────┘
```

No cycles. Site no longer needs to know about hive.

## 4. Behavior changes per consumer

### 4.1 `hive council` (`cmd/hive/main.go` + `pkg/runner/council.go`)

**Router (`cmd/hive/router.go:268`)** — `cmdCouncil` gains one new flag, matching the existing `civilization` pattern:

```go
catalog := fs.String("catalog", "", "Custom YAML model catalog (merged with built-in defaults)")
```

Threaded into `runCouncilCmd(space, apiBase, repoPath, budget, topic, catalogPath)`.

**`runCouncilCmd`** is rewritten:

1. Replace the hardcoded `intelligence.Config` block (lines 517–527) with:
   ```go
   resolver, err := buildCouncilResolver(catalogPath)  // helper: ResolverFromCatalogFile or DefaultResolver
   if err != nil { return fmt.Errorf("resolver: %w", err) }

   // Decision 5a: legacy fallback for COUNCIL_MODEL env var.
   if catalogPath == "" {
       if env := os.Getenv("COUNCIL_MODEL"); env != "" {
           resolver = modelconfig.DefaultResolverWithModel(env)
           log.Printf("[council] legacy mode: COUNCIL_MODEL=%s → synthetic single-model resolver", env)
       }
   } else if env := os.Getenv("COUNCIL_MODEL"); env != "" {
       log.Printf("[council] note: --catalog provided; COUNCIL_MODEL=%s env var ignored", env)
   }

   pool := modelconfig.NewProviderPool(resolver)
   ```
2. Pass `pool` (not `provider`) into `runner.Config`. Add a `Pool *modelconfig.ProviderPool` field there. **The existing `Provider` field stays** — used by civilization/pipeline/role modes which are already working. Council uses pool; others use provider; nothing else changes.

A new helper in modelconfig — `DefaultResolverWithModel(modelName string) *Resolver` — builds a synthetic resolver where every role and every tier query returns the named model. Implementation: start from `DefaultResolver()` (so the model catalog is the built-in one), then override `defaults.Model`, `defaults.TierModels[judgment|execution|volume]`, and zero-out `role_defaults` so the precedence chain falls through to the per-tier override. The model name passed in must resolve to a catalog entry (alias lookup is acceptable, e.g., `"opus"` → `claude-opus-4-6`); otherwise the helper returns an error. Used only for the COUNCIL_MODEL legacy fallback path; nothing else.

**`pkg/runner/council.go:RunCouncil`** changes:

1. After `loadCouncilMembers`, call `cfg.Pool.WarmForRoles(roleNamesOf(members))`. This pre-resolves every council member and surfaces any config errors before fan-out. Logs a unique-provider summary:
   ```
   [council] 47 members → 2 unique providers
   [council]   moonshotai/kimi-latest (openrouter)  ×46 roles
   [council]   gpt-5.5 (codex-cli)                  ×1 role
   [council] estimated cost (input 10k, output 2k per role): $0.62
   ```
2. Inside the per-member goroutine (line 57):
   - Replace `cfg.Provider.Reason(...)` / `op.Operate(...)` with:
     ```go
     provider, err := cfg.Pool.For(m.role)
     if err != nil { ... }
     ```
   - Existing type-assertion fallback stays: `op, canOperate := provider.(decision.IOperator)`.
3. **Decision 5b warning** — at warm-up time, walk each role's AgentDef; if `CanOperate=true` AND the resolved provider does not implement `decision.IOperator`, log:
   ```
   [council] warning: role implementer is CanOperate=true but resolved provider moonshotai/kimi-latest (openrouter) does not implement IOperator; Operate() calls will fall back to Reason()
   ```
   Does not block the run.

### 4.2 `site/graph/mind.go`

**Type changes:**

```go
type Mind struct {
    db                   *sql.DB
    store                *Store
    pool                 *modelconfig.ProviderPool  // new
    replyTimeout         time.Duration
    callProviderOverride func(...)                   // renamed from callClaudeOverride
}

func NewMind(db *sql.DB, store *Store, pool *modelconfig.ProviderPool) *Mind {
    return &Mind{db: db, store: store, pool: pool, replyTimeout: 5 * time.Minute}
}
```

Old `token string` field is removed (see §5 audit).

**`cmd/site/main.go` wiring:**

```go
resolver, err := buildSiteResolver(os.Getenv("SITE_CATALOG"))
if err != nil { log.Fatalf("site: resolver: %v", err) }
pool := modelconfig.NewProviderPool(resolver)
personaNames := personas.AllNames()  // 67 personas from generated status_gen.go
if err := pool.WarmForRoles(personaNames); err != nil {
    log.Fatalf("site: pool warm-up: %v", err)
}
log.Printf("site: mind initialized — %s", pool.Stats().Summary())
mind := graph.NewMind(db, store, pool)
```

The catalog path comes from a new `SITE_CATALOG` env var. No flag — site is a long-running daemon under Fly and env vars are how it's configured. Documented in `site/CLAUDE.md`.

**`callProvider` (formerly `callClaude`):**

```go
func (m *Mind) callProvider(ctx context.Context, role, systemPrompt string, messages []providerMessage) (string, error) {
    if m.callProviderOverride != nil {
        return m.callProviderOverride(ctx, systemPrompt, messages)
    }
    provider, err := m.pool.For(role)
    if err != nil {
        return "", fmt.Errorf("resolve provider for %s: %w", role, err)
    }
    prompt := buildPrompt(systemPrompt, messages)
    resp, err := provider.Reason(ctx, prompt, nil)
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

All four callers — `replyTo`, `OnQuestionAsked`, `OnCouncilConvened`, `OnTaskAssigned` — already know the agent name. They get updated signatures to pass `role` through.

The `exec.Command("claude", "-p", ...)` shellout at line 929 is gone.

### 4.3 `hive/pkg/hive/runtime.go:spawnAgent` — CanOperate warning

Civilization mode already routes every agent through the resolver, but it does not warn when an agent flagged `CanOperate=true` is resolved to a non-`IOperator` provider. The 5b warning belongs here too.

After the existing provider construction (lines ~516–520):

```go
cfg := modelconfig.ToIntelligenceConfig(resolved, def.SystemPrompt)
provider, err := intelligence.New(cfg)
if err != nil { return nil, "", fmt.Errorf("provider: %w", err) }

// Decision 5b: warn when capability won't match operate intent.
if def.CanOperate {
    if _, ok := provider.(decision.IOperator); !ok {
        log.Printf("[runtime] warning: agent %s (role %s) is CanOperate=true but resolved provider %s/%s does not implement IOperator; Operate() calls will fall back to Reason()",
            def.Name, def.Role, resolved.Provider, resolved.Model)
    }
}
```

The warning is purely advisory — it never blocks spawn. Identical wording / structure to the council-side warning so logs grep cleanly.

### 4.4 `hive/loop/run.sh` — retire

Replace body with:

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

No other files in `hive/loop/` change. The `*-prompt.txt` files stay (the Go pipeline reads them too).

## 5. Renames & cleanup

In the same diff that replaces the shellout in `site/graph/mind.go`:

| Old | New | Notes |
|---|---|---|
| `Mind.callClaude` (method) | `Mind.callProvider` | 5 call sites in mind.go |
| `Mind.callClaudeOverride` (field) | `Mind.callProviderOverride` | test fixtures updated |
| `claudeMessage` (struct) | `providerMessage` | ~6 references in mind.go |
| `Mind.token` (field) | **deleted** | per audit: `intelligence/claude_cli.go` reads `~/.claude/.credentials.json` natively, no need for Mind to hold a token |
| `NewMind(db, store, claudeToken)` | `NewMind(db, store, pool)` | constructor signature change |
| `// Mind responds to conversation messages via Claude.` (file header) | `// Mind responds to conversation messages via the configured provider pool.` | doc accuracy |
| `cmd/site/main.go` `CLAUDE_TOKEN` env lookup | **deleted** | dead code after Mind no longer holds the token |

If the token audit reveals a genuine dependency (e.g., site forwarding an OAuth token to a child process), the fix is to plumb it through the **pool builder** (`builder func(intelligence.Config) (intelligence.IIntelligence, error)`), not Mind. Mind stays provider-agnostic.

**NOT renamed:**

- `COUNCIL_MODEL` env var — Decision 5a kept it as a legacy compat hook. Renaming defeats the compat purpose.
- `intelligence/claude_cli.go`, `claude_sdk.go` — genuinely Claude-specific implementations.
- `mindSoul` const ("the hive's consciousness"), persona-facing strings, badge color — these are UX, not implementation.

## 6. Testing strategy

### 6.1 New tests

**`eventgraph/go/pkg/modelconfig/pool_test.go`:**

- Same `(provider, model, base_url, api_key)` tuple → same returned provider *instance* (identity test, not just equality).
- Two roles resolving to the same config → one underlying provider (dedup).
- `WarmForRoles` is idempotent; calling twice produces no duplicates.
- `Stats()` reports correct unique-provider count and per-model role breakdown.
- Resolver error → `For` returns that error without poisoning the cache.
- Injected `builder` works (a fake records calls; `intelligence.New` is not invoked in tests).
- Concurrent `For` for the same role is race-free (`-race`).

**`hive/pkg/runner/council_test.go`:**

- `RunCouncil` consumes `cfg.Pool` when set.
- Per-member resolution: stub builder returns a recording fake; assert each member's role string was the lookup key.
- The unique-provider summary log appears at warm-up time.
- The CanOperate≠IOperator warning appears when AgentDef.CanOperate=true and the resolved provider lacks `IOperator`.
- Type-assertion fallback to `Reason()` produces no panic; response captured cleanly.

**`hive/pkg/hive/runtime_test.go`** (extend existing):

- `spawnAgent` emits the CanOperate≠IOperator warning exactly once per spawn when `def.CanOperate=true` and the resolved provider lacks `IOperator`.
- No warning when `def.CanOperate=false` regardless of provider capabilities.
- No warning when `def.CanOperate=true` AND the resolved provider implements `IOperator` (positive case — e.g., claude-cli).
- Spawn still succeeds in the warning case (warning is advisory, not blocking).

**`hive/cmd/hive/main_test.go` (or wherever council CLI gets tested):**

Table test for catalog plumbing precedence:

| `--catalog` | `COUNCIL_MODEL` | Expected |
|---|---|---|
| `path.yaml` | unset | Pool built from `path.yaml` |
| unset | unset | Pool built from `DefaultResolver()` |
| unset | `opus` | Synthetic single-model resolver; legacy-mode log line |
| `path.yaml` | `opus` | Catalog wins; env-var-ignored log line |

**`site/graph/mind_test.go`:**

- Existing tests: rename `callClaudeOverride` → `callProviderOverride`, `claudeMessage` → `providerMessage`.
- New constructor: `NewMind(db, store, pool)`. Pool wraps a `DefaultResolver` + stub builder.
- Assert `OnMessage` / `OnTaskAssigned` / `OnQuestionAsked` / `OnCouncilConvened` call `pool.For(role)` with the right role string.
- Regression guard: `mindSoul` content unchanged.

### 6.2 Coverage targets

Per the existing eventgraph coding standard (≥90% for core packages), the new `pool.go` must reach ≥90% line coverage. Existing modelconfig coverage holds.

## 7. Migration & deprecation

### 7.1 Removed env vars

- `CLAUDE_TOKEN` (site) — documented in `site/CLAUDE.md` migration note. Local-dev `make dev` stops mentioning it. Production Fly secrets: `fly secrets unset CLAUDE_TOKEN --app transpara-ai` after deploy.

### 7.2 New env vars

- `SITE_CATALOG` (site) — optional, path to a YAML model catalog. Documented in `site/CLAUDE.md` and the startup log line `site: mind initialized — N personas, M unique providers`.

### 7.3 Behavior changes worth a CHANGELOG line

- `hive council` accepts `--catalog`. Without it, default behavior is unchanged (every role → Sonnet via claude-cli through `DefaultResolver`).
- `COUNCIL_MODEL` env var preserved as legacy single-model shortcut when `--catalog` is absent.
- Site Mind now routes every persona response through the resolver. Without `SITE_CATALOG`, behavior matches today (Sonnet via claude-cli).
- `hive/loop/run.sh` exits 1 with a deprecation notice on every invocation.

## 8. Verification before merge

```bash
# eventgraph (modelconfig relocation + ProviderPool)
cd ~/github/mind-zero/eventgraph/go
go build ./...
go test -race ./pkg/modelconfig/...
go vet ./...
staticcheck ./...

# hive (imports modelconfig from eventgraph; council + spawn changes)
cd ~/github/mind-zero/hive
go build ./...
go test -race ./...
go vet ./...

# site (imports modelconfig + intelligence from eventgraph; mind refactor)
cd ~/github/mind-zero/site
templ generate
make build
make test

# End-to-end smoke (uses the gitignored personal catalog)
cd ~/github/mind-zero/hive
go run ./cmd/resolve-model --role implementer --catalog ./kimi-only-catalog.yaml
go run ./cmd/resolve-model --role planner     --catalog ./kimi-only-catalog.yaml
go run ./cmd/hive council --topic "smoke test" --catalog ./kimi-only-catalog.yaml --budget 0.50
```

The kimi-only catalog is the canonical end-to-end smoke: 50-member council all on Kimi should produce one log line `[council] 50 members → 1 unique provider (moonshotai/kimi-latest, openrouter)` and stay under budget.

## 9. Commit shape

Branch `feat/route-claude-p-through-modelconfig` exists on both `hive` and `site`. Commits land in this order:

| # | Repo | Commit message |
|---|---|---|
| 1 | eventgraph | `refactor: move modelconfig from hive to eventgraph` |
| 2 | eventgraph | `feat(modelconfig): add ProviderPool with dedup, warm-up, and stats` |
| 3 | hive | `refactor: import modelconfig from eventgraph; bump go.mod` |
| 4 | hive | `feat(council): honor --catalog and route per-role via ProviderPool` |
| 5 | hive | `feat(spawn): log warning when CanOperate=true resolves to non-IOperator` |
| 6 | hive | `chore(loop): retire run.sh with deprecation banner` |
| 7 | site | `refactor(mind): route via modelconfig.ProviderPool; rename callClaude→callProvider; drop CLAUDE_TOKEN` |

Eventgraph PR merges first (mechanical move + new type). Then hive and site PRs land in parallel; both depend on the eventgraph change but are otherwise independent.

## 10. Out of scope

These came up during brainstorming and are explicitly **not** part of this spec:

- **Hot-reload of catalog YAML.** Neither the cache-at-startup choice (Decision 2) nor per-call resolution would address this today. A future `Resolver.Reload()` + SIGHUP handler in modelconfig is the right place.
- **A Go ReAct loop that turns any function-calling HTTP model into an `IOperator`.** This would let Kimi/OpenRouter implement `Operate()` and break the Claude-Code/Codex CLI dependency for the Implementer agent. Real project; substantial scope.
- **Renaming `COUNCIL_MODEL` or `intelligence/claude_cli.go`.** Out of scope per §5.
- **Adding `--catalog` to `pipeline` and `role` subcommands.** They already use `runner.ProviderConfig(role, budget)` which goes through `DefaultResolver()`. Adding a `--catalog` flag for them is a small future change but not required to close the three gaps.
- **Provider-cost dashboards in site.** The `Stats()` method exposes the data; UI is a separate piece of work.

---

*Brainstorming companion file: this session's `using-superpowers` + `brainstorming` skill flow. Five questions answered, three sections approved (architecture, behavior, testing & migration). Spec written 2026-05-14.*
