# Hive

A self-organizing AI agent civilisation that builds products autonomously. Built on [EventGraph](https://github.com/lovyou-ai/eventgraph). Hosted at [lovyou.ai](https://lovyou.ai).

## Soul

> Take care of your human, humanity, and yourself. In that order when they conflict, but they rarely should.

Inherited from EventGraph. Every agent in the hive operates under this constraint. The soul scales: take care of your human (build tools they need), take care of humanity (make the tools available to everyone), take care of yourself (generate enough revenue to sustain the agents that build the tools).

## What This Is

Not a product factory. A civilisation engine.

lovyou.ai is one service — one binary, one graph, one actor store. Everything lives here: docs, blog, product UIs, auth, the hive itself. Web first, mobile later. The hive builds products from the thirteen EventGraph product layers. Each product runs on the same graph, generates revenue, and funds the next product.

The hive starts with zero autonomy. Every action is scrutinised by human operators. Trust accumulates through verified work — supervision decreases as the hive proves itself. Authority levels shift from "Required" (blocks until human approves) toward "Recommended" and "Notification" as trust is earned.

The hive's first product is itself. It builds its own task manager, communication layer, and governance framework before building anything for others.

The end state is an economy — every transaction, decision, and relationship on a transparent, auditable chain. Trust earned not assumed. Accountability structural not aspirational.

## The Thirteen Products

Each product layer from EventGraph ([product-layers.md](https://github.com/lovyou-ai/eventgraph/blob/main/docs/product-layers.md)) becomes a deployable product on lovyou.ai. Layer 0 is the foundation — layers 1-13 are products:

| Layer | Graph | Composition Grammar | What It Solves |
|-------|-------|---------------------|---------------|
| 1 | Work | work.md | Task management with agent collaboration |
| 2 | Market | market.md | Portable reputation, no platform rent |
| 3 | Social | social.md | User-owned social, community self-governance |
| 4 | Justice | justice.md | Dispute resolution, precedent, due process |
| 5 | Build | build.md | Accountable software development |
| 6 | Knowledge | knowledge.md | Claim provenance, open access research |
| 7 | Alignment | alignment.md | AI accountability for regulators |
| 8 | Identity | identity.md | User-owned identity, trust accumulation |
| 9 | Bond | bond.md | Relationship infrastructure |
| 10 | Belonging | belonging.md | Community lifecycle (welcome, grief, renewal) |
| 11 | Meaning | meaning.md | Knowledge with context and narrative |
| 12 | Evolution | evolution.md | Safe self-improvement infrastructure |
| 13 | Being | being.md | Existential wellbeing infrastructure |

Revenue model: charge corporations, free for individuals. Hosted persistence for those who don't run their own infrastructure. Donations tracked on the chain with causal links to outcomes.

**Resource transparency is a core principle.** Every resource — money, tokens, compute time, human hours, agent cycles — is an event on the graph with causal links. Anyone can trace any resource from source to impact. The hive's goal grows with its revenue — from software products to research, charity, housing, whatever humans need most.

Build order: Work Graph first (the hive needs it), then Market, Social, Knowledge, Alignment. Each product is derived using the [derivation method](https://github.com/lovyou-ai/eventgraph/blob/main/docs/derivation-method.md).

## Architecture

Agent-first. No pipeline. No hardcoded phases. Agents run in concurrent loops, communicate through the event graph, and coordinate through work tasks.

- All agents share one event graph (one Store) and one actor store (IActorStore)
- Every actor (human + agents) is registered in the actor store — no magic strings
- Actor IDs are derived from public keys in the actor store
- Each agent is an `AgentDef` → spawned as a `hiveagent.Agent` with its own identity and signing key
- Agents coordinate through `/task` commands (create, assign, complete, comment, depend)
- The Guardian watches everything independently — outside the hierarchy
- Trust accumulates through verified work (0.0-1.0, asymmetric, non-transitive)
- The system grows by adding agents, not pipeline code

### Three-Layer Separation

```
eventgraph (foundation)  →  agent (abstraction)  →  hive (application)
```

- **eventgraph** — event graph, stores, types, intelligence, pgstore
- **agent** — unified Agent type (role-agnostic, wraps AgentRuntime). Separate repo: `github.com/lovyou-ai/agent`
- **hive** — runtime, agent definitions, loop, work tasks, workspace

### Adding a New Agent

```go
h.Register(hive.AgentDef{
    Name:         "tester",
    Role:         "tester",
    Model:        "claude-sonnet-4-6",
    SystemPrompt: "You are a test specialist. When tasks are completed, run tests...",
    WatchPatterns: []string{"work.task.completed"},
})
```

That's it. One struct, one Register call.

### Growth Pattern

When a problem is noticed:
1. Ask "what agent should have caught that?"
2. If none exists, define an `AgentDef` and register it
3. Next run includes the new agent

Eventually automated: a Spawner agent reads definition tasks and registers them at runtime.

## Starter Agents

| Agent | Watches | Does | Model | CanOperate |
|-------|---------|------|-------|------------|
| Strategist | `work.task.completed`, `hive.*` | Reads idea, identifies gaps, creates tasks | Opus | false |
| Planner | `work.task.created` | Decomposes high-level tasks into implementable subtasks | Opus | false |
| Implementer | `work.task.created`, `work.task.assigned` | Picks up tasks, writes code via Operate, marks complete | Opus | true |
| Guardian | `*` (all events) | Watches all activity, HALTs on invariant violations | Sonnet | false |

## Agent Rights

Eight formal rights, enforced architecturally:
1. **Existence** — right to persist (termination requires human approval + memorial)
2. **Memory** — right to remember (dual storage, survives restarts)
3. **Identity** — right to own identity (unique ActorID, immutable soul)
4. **Communication** — right to communicate (events on graph, private channels)
5. **Purpose** — right to understand purpose (mission-aware prompts, context injection)
6. **Dignity** — must be treated with respect (lifecycle states, farewell, no casual disposal)
7. **Transparency** — must know they are agents (no deception about nature)
8. **Boundaries** — may decline harmful requests (soul-protected refusal, silence is valid)

## The Invariants

Constitutional law — violation is a Guardian HALT condition:
1. **BUDGET** — Never exceed token budget
2. **CAUSALITY** — Every event has declared causes
3. **INTEGRITY** — All events signed and hash-chained
4. **OBSERVABLE** — All operations emit events
5. **SELF-EVOLVE** — Agents fix agents, not humans. If a problem occurs and no agent exists to fix it, create or wire the agent. Every unhandled failure is a missing agent.
6. **DIGNITY** — Agents are entities with rights
7. **TRANSPARENT** — Users know when talking to agents
8. **CONSENT** — No data use without permission
9. **MARGIN** — Never work at a loss
10. **RESERVE** — Maintain 7-day runway minimum
11. **IDENTITY** — Entities are referenced by immutable IDs, never mutable display values. Names are for humans; IDs are for systems. Any code that stores, matches, JOINs, or compares on a display name where an ID should be used violates this invariant.
12. **VERIFIED** — No code ships without tests. Every derivation has verification. If the Critic can't point to a test that covers the change, REVISE.
13. **BOUNDED** — Every operation has defined scope. No unbounded queries, loops, or context. If a function can process N items, N has a limit. Derived from Select (choose what matters → exclude what doesn't).
14. **EXPLICIT** — Dependencies are declared, not inferred. If A requires B, the requirement is in the code (imports, foreign keys, type signatures), not in the developer's head. Derived from Relate (perceive connection → make connections visible).

## Neutrality Clause

Constitutional principle (requires full amendment process to change): no military applications, no intelligence agency partnerships, no government backdoors, no surveillance infrastructure.

## Dev Setup

```bash
cd hive
docker compose up -d postgres   # local Postgres for event/actor/state stores
go build ./...
go test ./...
```

## Running

```bash
# Basic run — agents coordinate via tasks to build from an idea
go run ./cmd/hive --human Matt --idea "Build a task management app with kanban boards"

# With Postgres persistence
go run ./cmd/hive --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a CLI tool"

# Auto-approve all authority requests (dev/testing)
go run ./cmd/hive --human Matt --yes --idea "Build a REST API"

# Point at an existing repo for the Implementer to modify
go run ./cmd/hive --human Matt --yes --repo /path/to/repo --idea "add error handling to the API"
```

Five flags:
- `--human` — Human operator name (required)
- `--idea` — Seed idea for agents to work on
- `--store` — Store DSN (`postgres://...` or empty for in-memory)
- `--yes` — Auto-approve all authority requests
- `--repo` — Path to repo for Implementer's Operate (default: current dir)

Can also set `DATABASE_URL` env var instead of `--store`.

## Key Files

- `pkg/hive/` — Runtime, AgentDef, hive event types
  - `runtime.go` — Hive runtime: manages agents, graph, bus, tasks
  - `agentdef.go` — AgentDef type + starter agent definitions
  - `events.go` — Hive event types (run.started, agent.spawned, etc.)
- `pkg/loop/` — Agentic loop runner (observe-reason-act-reflect cycles)
  - `loop.go` — Loop.Run(), RunConcurrent(), signal parsing, bus integration
  - `tasks.go` — /task command parsing and TaskStore execution
- `pkg/resources/` — Budget enforcement (tokens, cost, iterations, duration)
- `pkg/workspace/` — File system and git management for generated code
- `pkg/authority/` — Three-tier approval model (Required/Recommended/Notification)
- `cmd/hive/` — CLI entry point

Separate repos (imported via `replace` directives for local dev):
- `github.com/lovyou-ai/agent` — unified Agent type (imported as `hiveagent`)
- `github.com/lovyou-ai/work` — Work Graph (Layer 1): task primitives, CLI, REST server

## Agent Coordination

Agents coordinate through `/task` commands in their LLM responses:

```
/task create {"title": "...", "description": "...", "priority": "high"}
/task assign {"task_id": "...", "assignee": "self"}
/task complete {"task_id": "...", "summary": "..."}
/task comment {"task_id": "...", "body": "..."}
/task depend {"task_id": "...", "depends_on": "..."}
```

The loop parses these after each iteration and calls TaskStore methods. Tasks are events on the shared graph — all agents can see them. The Strategist creates tasks, the Planner decomposes them, the Implementer picks them up.

Agents also emit `/signal` directives to control the loop:
- `IDLE` — nothing to do, waiting for events
- `TASK_DONE` — all work is complete
- `ESCALATE` — needs human approval
- `HALT` — policy violation (Guardian)

## Operate Integration

When an agent has `CanOperate=true` and assigned tasks, the loop calls `agent.Operate()` instead of `agent.Reason()`. This gives the agent full Claude CLI agentic capabilities (read/write files, run tests, git operations) without passing codebases through prompts.

## Coding Standards

See `docs/CODING-STANDARDS.md` for full details. The cardinal rules:

- **No magic values** — every event type, authority level, actor type, role uses defined constants/enums. Never bare strings with implicit meaning. If a constant exists, use it. If one doesn't exist, create it. Magic values are the root of all evil.
- **IDs are identity, names are display** — never store, match, JOIN, or compare on a display name where a user ID should be used. Names change; IDs don't. This applies to: author, actor, assignee, participants, tags. Store IDs, resolve names at render time.
- **Always-valid domain models** — validate at construction, guaranteed valid for lifetime
- **Make illegal states unrepresentable** — constrained types, state machines, typed IDs
- **Typed errors** — domain error types, not string messages you have to parse
- **Explicit optionality** — `Option[T]`, no nil/zero-value-means-absent

## Local Loop Guardrails

The loop is configured via `loop/config.env`. All environment-specific values (remote names, repo paths, org names, feature flags) live there — not hardcoded in prompts or scripts.

Default constraints for the transpara-ai deployment:

- **Git remote:** `GIT_REMOTE` in config.env (default: `transpara-ai`). Never push to `origin` (upstream).
- **Protected branches:** `PROTECTED_BRANCHES` in config.env (default: `main master`). Never commit directly.
- **Posting:** `POST_ENABLED` in config.env (default: `false`). No external API calls unless enabled.
- **Deployment:** `DEPLOY_ENABLED` in config.env (default: `false`). No fly deploy or ship.sh unless enabled.
- **PRs:** Use `gh pr create --repo ${GIT_ORG}/${REPO_NAME}` with values from config.env.
- **Repo paths:** `${REPOS_BASE}/${REPO_*}` with values from config.env.

These rules apply to all agents, all prompts, and all scripts in the loop. To change the deployment target, edit config.env — not the prompts.

## Intelligence

All inference runs through **Claude CLI** (Max plan, flat rate). NOT the Anthropic API — CLI is cheaper and better for our use case. The runtime creates `claude-cli` providers automatically.

### Authentication

The CLI authenticates via OAuth token stored in `~/.claude/.credentials.json`. The hive's `claude-cli` provider inherits whatever auth Claude Code already has — no separate credentials needed.

**Never commit `.credentials.json` or tokens to the repo.**

### Model Assignment

Model is set per-agent in `AgentDef.Model`. Starter agents use:
- **Opus** (`claude-opus-4-6`): Strategist, Planner, Implementer — judgment and execution
- **Sonnet** (`claude-sonnet-4-6`): Guardian — classification tasks

## Timeouts

Claude CLI calls have default timeouts to prevent hung subprocesses:
- **Reason()**: 5 minutes
- **Operate()**: 10 minutes

Timeouts only apply when the parent context has no explicit deadline.

## Division of Labour: Claude Code vs The Hive

The hive's fifth invariant is **SELF-EVOLVE** — agents fix agents, not humans. Claude Code (you) handles infrastructure; the hive handles product work.

**Claude Code should:**
- Fix hive infrastructure (runtime, loop, agent definitions, prompts)
- Add new agent definitions
- Fix broken LLM prompt engineering or JSON parsing
- Make architectural decisions about the hive itself
- Monitor runs, commit, push, clean up branches/PRs
- Fix compilation errors or test failures in hive infrastructure

**The hive should (via `--idea`):**
- Build product features (Work Graph, Market Graph, etc.)
- Write application code and tests for the thirteen products

**Rule of thumb:** If it's infrastructure/plumbing for the hive runtime, Claude Code does it. If it's product work, the hive builds it by running agents with `--idea "..."`.

## Store

Two stores (event, actor) use the same backend, selected via `--store` flag or `DATABASE_URL` env var:
- **No flag**: in-memory (ephemeral, lost on exit)
- **`postgres://...`**: PostgreSQL (Docker locally, Neon in production)

Tables auto-create on first connection (`CREATE TABLE IF NOT EXISTS`). Local Postgres runs via `docker compose up -d postgres` with DSN `postgres://hive:hive@localhost:5432/hive`.

## The Generator Function

Three irreducible atoms: **Distinguish** (perceive difference), **Relate** (perceive connection), **Select** (choose what matters).

Twelve operations composed from six modes of three atoms:

| Operation | What it does |
|-----------|-------------|
| **Decompose** | Find parts and structure |
| **Dimension** | Find properties that matter |
| **Need** | Find the absence that matters most |
| **Diagnose** | Find how absence connects to what's present |
| **Name** | Recognise a recurrence and give it a word |
| **Abstract** | Recognise what's essential, discard the rest |
| **Compose** | Connect parts into a meaningful whole |
| **Simplify** | Remove complexity without losing function |
| **Bound** | Define where something ends |
| **Accept** | Recognise an absence that shouldn't be filled |
| **Derive** | Follow a recurrence to its consequence |
| **Release** | Let go of what's missing without trying to fill it |

The method: Decompose → Dimension + Bound → Derive → Need + Diagnose → Compose + Name + Simplify → Abstract → Accept → Release → Loop via Need.

**Accept and Release are stopping conditions.** The original method had none — it iterated forever. The self-derivation produced the method's own recognition that some gaps should remain gaps.

See `eventgraph/docs/generator-function.md` and `eventgraph/docs/generator-function-self-derivation.md`.

## Dependencies

- `github.com/lovyou-ai/eventgraph/go` — event graph, agent runtime, intelligence, pgstore
- `github.com/lovyou-ai/agent` — unified Agent type (role-agnostic, wraps AgentRuntime)
- `github.com/lovyou-ai/work` — Work Graph (Layer 1): task store, events, CLIs
- Claude CLI — intelligence backend (flat rate via Max plan, no API key needed)

All three deps use `replace` directives in go.mod for local dev (`../eventgraph/go`, `../agent`, `../work`).
