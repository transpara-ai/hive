# Architecture

## One Service

lovyou.ai is one Go binary. Not microservices. One event graph, one actor store, one database.

```
lovyou.ai
├── Hive runtime (agent registration, spawning, event emission)
├── Agentic loop (observe → reason/operate → check stopping → repeat)
├── Agents (Strategist, Planner, Implementer, Guardian)
├── Work graph (task coordination via events)
├── Workspace (git repos, branches, PRs)
├── Event graph (shared store, all events)
└── Actor store (humans and agents, persistent via pgactor)
```

One entry point today:
- `cmd/hive` — CLI for running agent loops

## Layers

### EventGraph (substrate)

The foundation. Hash-chained, append-only, causal event graph.

- **Store** — event persistence (in-memory or Postgres via pgstore)
- **IActorStore** — actor persistence (in-memory or Postgres via pgactor)
- **AgentRuntime** — identity, signing key, intelligence provider
- **IBus** — real-time event notification (agents subscribe to patterns)
- **Authority** — three-tier approval (Required/Recommended/Notification)
- **Trust** — 0.0-1.0, asymmetric, non-transitive, contextual

### Agent (abstraction)

Unified Agent type wrapping EventGraph's AgentRuntime.

- **State machine** — Idle → Processing → Idle, enforced transitions
- **Operations** — Reason (LLM response), Operate (filesystem + tools via Claude CLI), Observe (query graph), Evaluate (judgment)
- **Identity** — deterministic key derivation from agent name; same name = same ActorID across runs
- **Causality** — each event caused by the agent's previous event, not store.Head()
- **Trust hooks** — integrated with EventGraph trust accumulation

### Hive (civilisation)

The agent society built on EventGraph + Agent.

- **Runtime** — manages agent registration, spawning, shared graph, task store
- **Loop** — observe-reason-act cycle with stopping conditions (quiescence, escalation, HALT, budget, task done)
- **Resources** — budget enforcement (iterations, duration, tokens, cost)
- **Workspace** — git management (clone, branch, commit, push, PR)

### lovyou.ai (surface)

Separate repo (lovyou-ai/site). Go + templ + HTMX + Tailwind.

- **Blog** — markdown posts rendered to HTML
- **Reference** — 201 primitives, 14 layers, grammars (cognitive, graph, layer), agent primitives
- **Auth** — Google OAuth, session cookies
- **Products** — unified graph product (spaces, nodes, grammar operations)
- **Deployed** on Fly.io with Neon Postgres

## Agents

Four starter agents, defined in `pkg/hive/agentdef.go`:

| Agent | Role | Model | What it does |
|-------|------|-------|-------------|
| Strategist | strategist | Opus | Reads seed idea, creates high-level tasks, watches completions, creates follow-up work |
| Planner | planner | Opus | Decomposes large tasks into implementable subtasks with dependencies |
| Implementer | implementer | Opus | Picks tasks, self-assigns, writes code via Operate, runs tests, marks complete |
| Guardian | guardian | Sonnet | Watches ALL events, detects violations, emits ALERT/HALT directives |

Agents coordinate through work tasks on the shared event graph. No direct communication — only events.

## Intelligence

Claude CLI (`claude -p`). Uses Claude Code's existing OAuth.

Supports `--mcp-config` for giving agents MCP tool access (not yet wired).

## Storage

```
Local dev:     Docker Postgres (docker-compose)
Production:    Neon (serverless Postgres)
Connection:    --store flag or DATABASE_URL env var
In-memory:     default when no DSN provided
```

Postgres holds:
- Events (pgstore: events, event_causes, edges)
- Actors (pgactor: actors, actor_keys)

## Code Structure

```
hive/
├── cmd/hive/main.go        — CLI entry point
├── pkg/
│   ├── hive/
│   │   ├── runtime.go      — Runtime (agent spawn, graph, events)
│   │   ├── agentdef.go     — AgentDef type + 4 starter agents
│   │   └── events.go       — Hive event types (run.started, agent.spawned, etc.)
│   ├── loop/
│   │   └── loop.go         — Agentic loop (observe → reason → check → repeat)
│   ├── resources/
│   │   ├── budget.go       — Budget enforcement
│   │   └── tracking.go     — Token/cost tracking provider
│   └── workspace/
│       └── workspace.go    — Git operations
├── docs/                    — Architecture, vision, governance specs
└── loop/                    — Core loop artifacts (scout, build, critique, reflections)
```

## Self-Modification

The hive modifies its own codebase via the core loop (see [CORE-LOOP.md](CORE-LOOP.md)):

1. Scout identifies gap in capabilities
2. Builder writes code, creates branch, opens PR
3. Critic reviews for correctness and safety
4. Reflector captures lessons
5. Human approves PR

## How It Runs

```bash
go run ./cmd/hive \
  --human Matt \
  --store postgres://hive:hive@localhost:5432/hive \
  --repo /path/to/target \
  --idea "Build feature X" \
  --yes
```

All agents start concurrently. They observe the graph, pick up tasks, write code, coordinate via events, and stop when quiescent or budget-exhausted.
