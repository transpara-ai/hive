# Migrating to the Model Configuration System

## What changed

Previously, model/provider selection was controlled by:
- Hardcoded `Model:` fields on each `AgentDef` in Go code
- `HIVE_PROVIDER` and `HIVE_MODEL` environment variables for global overrides

Both are gone. Model selection is now declarative via YAML catalogs and a `--catalog` CLI flag.

**If you were using the defaults (no env vars, no code changes), nothing changes.** The embedded catalog reproduces the exact same model assignments. You only need to act if you were using `HIVE_PROVIDER` or `HIVE_MODEL`.

## Quick comparison

| Before | After |
|--------|-------|
| `HIVE_PROVIDER=ollama HIVE_MODEL=llama3 go run ./cmd/hive ...` | `go run ./cmd/hive civilization run --catalog my-catalog.yaml ...` |
| Edit Go code to change a single agent's model | Edit one line in a YAML file |
| All non-Operate agents forced to same model | Per-agent model/provider assignment |
| No cost visibility | Allocator sees per-agent cost estimates |

## Step 1: Start with the defaults

If you run without `--catalog`, the embedded catalog is used automatically. It matches the old hardcoded defaults exactly:

| Agent | Model | Provider |
|-------|-------|----------|
| guardian | claude-sonnet-4-6 | claude-cli |
| sysmon | claude-haiku-4-5-20251001 | claude-cli |
| allocator | claude-haiku-4-5-20251001 | claude-cli |
| cto | claude-opus-4-6 | claude-cli |
| spawner | claude-sonnet-4-6 | claude-cli |
| reviewer | claude-sonnet-4-6 | claude-cli |
| strategist | claude-sonnet-4-6 | claude-cli |
| planner | claude-sonnet-4-6 | claude-cli |
| implementer | claude-opus-4-6 | claude-cli |

This is the zero-change migration path. Everything works as before.

## Step 2: Create a custom catalog to change things

To override any agent's model or provider, create a YAML file and pass it with `--catalog`. Your file is **merged on top of** the embedded defaults — you only need to include what you're changing.

### Minimal example: change one agent

```yaml
# catalog-custom.yaml — only override what you need
role_defaults:
  guardian: haiku    # downgrade guardian from sonnet to haiku
```

```bash
go run ./cmd/hive civilization run --human Matt --catalog catalog-custom.yaml --idea "..."
```

All other agents keep their embedded defaults. Only guardian changes.

### Full defaults example

This file reproduces the exact embedded defaults. Copy it and modify as needed:

```yaml
# catalog-defaults.yaml — identical to the embedded catalog
# Copy this file, then change individual role_defaults to customize.

role_defaults:
  guardian: sonnet
  sysmon: haiku
  allocator: haiku
  cto: opus
  spawner: sonnet
  reviewer: sonnet
  strategist: sonnet
  planner: sonnet
  implementer: opus
```

Since these match the embedded defaults, running with this file produces the same behavior as running without `--catalog`. Edit individual lines to change agents incrementally.

## Step 3: Add non-Claude models

To use Ollama, OpenRouter, Codex, or other providers, add model entries to your catalog. The catalog ID must match the exact model name the provider API expects.

### Ollama example

```yaml
models:
  - id: gemma4                    # must match what `ollama list` shows
    provider: ollama
    base_url: http://localhost:11434/v1   # /v1 suffix required
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

### OpenRouter example

```yaml
models:
  - id: deepseek/deepseek-v4-pro  # exact OpenRouter model ID
    aliases: [deepseek]
    provider: openrouter
    auth_mode: api-key
    tier: execution
    capabilities: [tools, reasoning, coding, large-context]
    context_window: 1048576
    max_output_tokens: 16384
    pricing:
      input_per_million: 0.435
      output_per_million: 0.87

role_defaults:
  strategist: deepseek
```

Requires `OPENROUTER_API_KEY` env var.

### Codex CLI example

```yaml
models:
  - id: gpt-5.5                   # exact model name for codex exec -m
    aliases: [gpt5.5]
    provider: codex-cli
    auth_mode: subscription
    tier: judgment
    capabilities: [tools, reasoning, coding, operate]
    context_window: 200000
    max_output_tokens: 16384
    pricing:
      input_per_million: 2.00
      output_per_million: 8.00

role_defaults:
  cto: gpt5.5
```

### Anthropic API (pay-per-token instead of CLI subscription)

The embedded catalog already includes `api-opus`, `api-sonnet`, and `api-haiku` entries. No custom catalog needed — just reference the aliases:

```yaml
role_defaults:
  strategist: api-sonnet    # uses Anthropic API instead of Claude CLI
  planner: api-sonnet
```

Requires `ANTHROPIC_API_KEY` env var.

## Step 4: Mix providers

You can assign different providers per agent in one catalog:

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

  - id: deepseek/deepseek-v4-pro
    aliases: [deepseek]
    provider: openrouter
    auth_mode: api-key
    tier: execution
    capabilities: [tools, reasoning, coding, large-context]
    context_window: 1048576
    max_output_tokens: 16384
    pricing:
      input_per_million: 0.435
      output_per_million: 0.87

role_defaults:
  guardian: gemma4          # Ollama (free, local)
  sysmon: gemma4            # Ollama
  allocator: gemma4         # Ollama
  spawner: gemma4           # Ollama
  cto: deepseek             # OpenRouter
  reviewer: deepseek        # OpenRouter
  strategist: sonnet        # Claude CLI (built-in)
  planner: sonnet           # Claude CLI (built-in)
  implementer: opus         # Claude CLI (built-in, forced by CanOperate)
```

See `catalog-mixed.yaml` in the repo root for a complete working example.

## Testing your catalog

Use the `resolve-model` CLI to verify resolution without running agents:

```bash
# Check what model/provider a role resolves to
go run ./cmd/resolve-model --role guardian --catalog my-catalog.yaml

# Check with CanOperate constraint (implementer)
go run ./cmd/resolve-model --role implementer --catalog my-catalog.yaml --can-operate

# Check cost estimate
go run ./cmd/resolve-model --role cto --catalog my-catalog.yaml --estimate-cost --input-tokens 10000 --output-tokens 2000
```

## Key rules

1. **Catalog IDs must match the provider's API model name.** Use `gemma4` not `ollama-gemma4`. Use `deepseek/deepseek-v4-pro` not `my-deepseek`.

2. **Ollama base_url needs the `/v1` suffix.** The provider appends `/chat/completions` to whatever you set.

3. **CanOperate agents can only use `claude-cli` or `codex-cli`.** If you assign an Ollama or OpenRouter model to the implementer, resolution will fail with an error. Other providers don't support filesystem access.

4. **`base_url` is hardcoded per catalog entry.** The `OLLAMA_HOST` env var does not override it. To use a remote Ollama, change the `base_url` in your YAML.

5. **Your catalog merges with defaults — it doesn't replace them.** Built-in Claude models (opus, sonnet, haiku) are always available. Your entries add to them or replace by ID.

## Migrating from HIVE_PROVIDER / HIVE_MODEL

| Old command | New equivalent |
|-------------|---------------|
| `HIVE_PROVIDER=ollama HIVE_MODEL=gemma4 go run ./cmd/hive civilization run ...` | Create a catalog with `role_defaults` pointing all roles to `gemma4`, run with `--catalog` |
| `HIVE_MODEL=claude-opus-4-6 go run ./cmd/hive civilization run ...` | Create a catalog with all `role_defaults` set to `opus` |
| No env vars (default Claude) | No change needed — works without `--catalog` |

The env vars applied the same model to every agent. The catalog system lets you be specific per-agent, but if you want the old "everything uses one model" behavior, just set every role to the same alias in `role_defaults`.
