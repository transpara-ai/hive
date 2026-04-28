# Ollama Integration

## What we wanted to do

Replace the Claude CLI (`claude -p`) intelligence backend with Ollama running Gemma 4 locally. This lets the hive run entirely offline, without API costs, using a local model.

## How the hive uses LLMs

The hive has two ways it calls a language model:

**`Reason()`** — The agent sends a prompt and gets a text response back. Used by every agent on every iteration: Strategist deciding what tasks to create, Planner decomposing tasks, Guardian checking invariants, etc.

**`Operate()`** — The agent gets full agentic capabilities: read files, write files, run bash commands, git operations. Used only by the Implementer agent when it's actually writing code. This works by shelling out to `claude -p --dangerously-skip-permissions` and letting Claude Code do the work autonomously.

## The provider architecture

All LLM calls go through an `IIntelligence` interface with a factory:

```go
provider, err := intelligence.New(intelligence.Config{
    Provider: "claude-cli",  // or "ollama", "openai", "anthropic", etc.
    Model:    "claude-sonnet-4-6",
    ...
})
```

The `"ollama"` provider already existed — it uses the OpenAI-compatible `/v1/chat/completions` endpoint that Ollama exposes, auto-resolving the base URL to `$OLLAMA_HOST` or `http://localhost:11434`. No new provider code was needed.

## What can and can't be replaced

| | Replaceable with Ollama? |
|--|--|
| `Reason()` — all agents | Yes, trivially |
| `Operate()` — Implementer only | No — it shells out to the `claude` binary specifically |

`Operate()` is Claude Code-specific. The `claude -p` subprocess handles the entire agentic loop of reading files, editing, running tests, and committing. Ollama has no equivalent. To fully replace `Operate()` with Ollama would require implementing a ReAct tool-call loop in Go (call model → parse tool calls → execute → feed results back → repeat). Gemma 4 supports native function calling, so it's technically feasible but not yet implemented.

For now: Ollama handles all `Reason()` calls. If/when the Implementer runs, it still needs Claude CLI.

## Does Gemma 4 support function calling?

Yes. The Ollama model page confirms Gemma 4 has "native function-calling support" designed for "highly capable autonomous agents." This makes the future `Operate()` replacement via a ReAct loop viable. The wire format (OpenAI tool-call schema) is already supported by the existing `openaiProvider`.

## What we changed

### Problem 1: Provider was hardcoded everywhere

All four `intelligence.New()` call sites hardcoded `"claude-cli"` or `"claude-sdk"`. Switching backends required a code change.

**Fix:** The `pkg/modelconfig` subsystem replaced all hardcoded model selection with a declarative YAML catalog and 7-layer precedence resolver. Per-agent provider/model is configured via `--catalog` flag with a custom YAML file. See `catalog-mixed.yaml` for an example using Claude, Codex, Ollama, and OpenRouter simultaneously.

Call sites now use this helper and augment it with provider-specific fields where needed:

```go
// Before
model := runner.ModelForRole(role)
provider, err := intelligence.New(intelligence.Config{
    Provider:     "claude-cli",
    Model:        model,
    MaxBudgetUSD: budget,
    APIKey:       resolveAnthropicKey(),
})

// After
providerCfg := runner.ProviderConfig(role, budget)
providerCfg.APIKey = resolveAnthropicKey()
provider, err := intelligence.New(providerCfg)
```

**Files changed:**
- `hive/pkg/runner/runner.go` — added `ProviderConfig()`
- `hive/cmd/hive/main.go` — updated 2 call sites (runner mode and pipeline mode)
- `hive/pkg/hive/runtime.go` — updated `spawnAgent()` with inline env var check (this package can't import runner without a circular dependency)

**Council mode (`--council`) was left unchanged** — it has its own `COUNCIL_MODEL` env var and rarely-used code path. Adding it to the general system would require either duplicating logic or adding "council" as a role to the model map, neither of which is worth it for an edge case.

### Problem 2: `MaxTokens` default was too low

The `intelligence.New()` factory defaulted `MaxTokens` to 1024 when not specified. Claude CLI ignores this field entirely (it manages its own context). But the OpenAI-compatible provider sends `max_tokens: 1024` in every request to Ollama.

This caused agents to hit the token limit mid-response while generating JSON task payloads, producing:

```
warning: /task create failed: parse: unexpected end of JSON input
```

**Fix:** Changed the default in `eventgraph/go/pkg/intelligence/provider.go` from 1024 to 8192.

### Problem 3: Cost tracking

Cost tracking only works correctly with Claude CLI. The Claude CLI provider receives `total_cost_usd` in the JSON response from the `claude` binary and passes it through. Every other provider — Ollama, OpenAI, Groq, Together, xAI — returns `CostUSD: 0.0` because the `openaiProvider` never calculates it.

A `modelPricing` map exists in `hive/pkg/runner/runner.go` with per-million-token rates for haiku/sonnet/opus, but it is dead code — nothing reads it to compute cost.

Practical consequences:

| | Claude CLI | Any other provider |
|--|--|--|
| Token counts | Correct | Correct |
| `CostUSD` | Correct (from binary output) | Always $0.00 |
| `--budget` cost limit | Enforced | Silently disabled |
| Iteration/duration limits | Enforced | Enforced |

**For Ollama specifically** this is harmless — there is no real cost for local inference.

**For paid cloud providers** (Groq, Together, OpenAI, xAI, OpenRouter), the `--budget` flag provides no cost protection and all cost reports will show $0.00 regardless of actual spend. Do not rely on `--budget` for cost control with these providers. Use `MaxIterations` and `MaxDuration` in `AgentDef` to bound runaway agents instead.

This is a known limitation. A proper fix would require the `openaiProvider` to calculate cost client-side from token counts and a configurable per-model pricing table.

### Problem 4: Planner using task titles instead of UUIDs in commands

When the Planner creates a task and then immediately references it in a `/task depend` or `/task assign` command, it must use the UUID returned by the system — not the human-readable title. Claude reliably does this. Gemma 4 does not: it constructs a name-based identifier from the title instead.

This produces errors like:
```
warning: /task depend failed: invalid depends_on: EventID: invalid format "task: verify e2e test environment readiness", expected UUID v7
warning: /task assign failed: invalid task_id: EventID: invalid format "task: verify e2e test environment readiness", expected UUID v7
```

This is the IDENTITY invariant: names are for humans, IDs are for systems. Gemma 4 needs this spelled out more explicitly than Claude does.

**Fix:** Added an explicit block to the Planner system prompt in `hive/pkg/hive/agentdef.go`:

```
CRITICAL — TASK IDs ARE UUIDs:
When you create a task with /task create, the system returns a UUID (e.g., 019d6a45-4359-746b-98cb-191007acc33f).
You MUST use that exact UUID in all subsequent /task depend, /task assign, and /task complete commands.
NEVER use the task title or a human-readable name where a task_id is required.
Wrong:  /task depend {"task_id": "task: verify e2e readiness", "depends_on": "..."}
Right:  /task depend {"task_id": "019d6a45-4359-746b-98cb-191007acc33f", "depends_on": "..."}
```

### Problem 5: Self-referential task dependencies

The Planner was also emitting dependencies where a task listed itself as its own dependency:

```
→ task dependency: 019d6a38-... depends on 019d6a38-...
```

Both sides of the dependency are the same UUID — the model hallucinated this. It's a model quality issue that prompt changes alone may not reliably prevent, so the fix is a guard at the store level.

**Fix:** Added a self-reference check to `AddDependency()` in `work/store.go`:

```go
if taskID == dependsOnID {
    return fmt.Errorf("task %s cannot depend on itself", taskID.Value())
}
```

This rejects the dependency before any event is written to the graph. The loop logs a warning and moves on. Fixing it at the store rather than only the prompt means it's enforced regardless of which model or agent issues the command.

## How to run with Ollama

```bash
# Install and start Ollama, pull the model
ollama pull gemma4

# Create a catalog YAML with Ollama models (see catalog-mixed.yaml for a full example)
# Then run:
go run ./cmd/hive civilization run --human Matt --catalog my-catalog.yaml --idea "Build a task manager"
```

The `--catalog` flag merges your custom YAML on top of the built-in Claude catalog. Catalog entry IDs must match the Ollama model name exactly (e.g., `gemma4`, not `ollama-gemma4`), and `base_url` must include the `/v1` suffix:

```yaml
models:
  - id: gemma4
    provider: ollama
    base_url: http://localhost:11434/v1
    auth_mode: local
    tier: volume
    capabilities: [coding, fast-latency]
    context_window: 8192
    max_output_tokens: 4096
    pricing:
      input_per_million: 0
      output_per_million: 0

role_defaults:
  guardian: gemma4
  sysmon: gemma4
```

Default behaviour (no `--catalog`) is unchanged — falls back to `claude-cli` and per-role model defaults from the embedded catalog.

## Env vars reference

| Var | Default | Purpose |
|-----|---------|---------|
| `OLLAMA_HOST` | `http://localhost:11434` | Ollama server address (used by the intelligence layer when no `base_url` in catalog) |
| `OPENROUTER_API_KEY` | — | API key for OpenRouter models |
| `DATABASE_URL` | — | Postgres DSN (alternative to `--store` flag) |

