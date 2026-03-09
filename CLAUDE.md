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

Each product layer from EventGraph becomes a deployable product on lovyou.ai:

| Layer | Product | What It Solves |
|-------|---------|---------------|
| 0 | Foundation | The graph itself — events, trust, authority |
| 1 | Work Graph | Task management with agent collaboration and accountability |
| 2 | Market Graph | Portable reputation, escrow as events, no platform rent |
| 3 | Social Graph | User-owned social infrastructure, community self-governance |
| 4 | Knowledge Graph | Claim provenance, challenge events, source reputation |
| 5 | Research Graph | Open access research, replication infrastructure |
| 6 | Justice Graph | Dispute resolution, precedent, due process |
| 7 | Identity Graph | Decentralized identity, trust accumulation |
| 8 | Governance Graph | Community governance, norm evolution, consent |
| 9 | Exchange Graph | Value exchange, resource allocation |
| 10 | Health Graph | Health data sovereignty, care coordination |
| 11 | Education Graph | Learning paths, credential verification |
| 12 | Media Graph | Content provenance, attribution, fair compensation |
| 13 | Alignment Graph | AI accountability dashboard for regulators |

Revenue model: charge corporations, free for individuals. Revenue from hosted persistence for people who don't want to run their own infrastructure.

## Architecture

- All agents share one event graph (one Store) and one actor store (IActorStore)
- Every actor (human + agents) is registered in the actor store — no magic strings
- Humans register via Google auth, agents are created by humans or other agents
- Actor IDs are derived from public keys in the actor store
- Each agent is an `AgentRuntime` with its own identity and signing key
- Communication is through events on the shared graph
- The Guardian watches everything independently — outside the hierarchy
- Trust accumulates through verified work (0.0-1.0, asymmetric, non-transitive)
- Authority model: Required / Recommended / Notification (three-tier approval)

## Roles

| Role | Responsibility | Intelligence | Trust Gate |
|------|---------------|-------------|------------|
| CTO | Architectural oversight, escalation filtering | Opus | 0.1 (bootstrapped) |
| Guardian | Independent integrity, halt/rollback/quarantine | Opus | 0.1 (bootstrapped) |
| Researcher | Read URLs, extract product ideas | Sonnet | 0.3 |
| Architect | Design systems in Code Graph | Opus | 0.3 |
| Builder | Generate code + tests | Sonnet | 0.3 |
| Reviewer | Code review, security audit | Opus | 0.5 |
| Tester | Run tests, validate behavior | Sonnet | 0.3 |
| Integrator | Assemble, deploy | Sonnet | 0.7 |

Agents can specify new roles and request permission to spawn them. The hive grows its own workforce.

## Dev Setup

```bash
cd hive
go build ./...
go test ./...
```

## Running

```bash
# Local dev (in-memory store)
go run ./cmd/hive --human Matt --idea "Build a task management app with kanban boards"

# With Postgres (Docker locally, Neon in production)
go run ./cmd/hive --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "..."

# Or via DATABASE_URL env var
export DATABASE_URL="postgres://hive:hive@localhost:5432/hive"
go run ./cmd/hive --human Matt --idea "..."

# From a URL with an explicit product name
go run ./cmd/hive --human Matt --name social-grammar --url "https://mattsearles2.substack.com/p/the-missing-social-grammar"

# From a Code Graph spec file
go run ./cmd/hive --human Matt --spec path/to/spec.cg
```

## Key Files

- `pkg/roles/` — Agent role definitions and system prompts
- `pkg/pipeline/` — Product pipeline orchestration
- `pkg/workspace/` — File system and git management for generated code
- `cmd/hive/` — CLI entry point

## Intelligence

All inference runs through **Claude CLI** (Max plan, flat rate). NOT the Anthropic API — CLI is cheaper and better for our use case. The pipeline creates `claude-cli` providers automatically.

Model assignment by role:
- **Opus** (`claude-opus-4-6`): CTO, Architect, Reviewer, Guardian — high-judgment tasks
- **Sonnet** (`claude-sonnet-4-6`): Builder, Tester, Integrator, Researcher — execution tasks

## Pipeline

1. **Research** — Researcher reads URLs/ideas, CTO evaluates feasibility
2. **Design** — Architect creates Code Graph spec, CTO reviews for minimalism
3. **Simplify** — Architect reduces spec to minimal form (up to 3 rounds)
4. **Build** — Builder generates multi-file project, committed to product repo
5. **Review → Rebuild** — Reviewer checks quality/compliance/simplicity (up to 3 rounds)
6. **Test** — Tester runs actual test suite, Builder fixes failures
7. **Integrate** — Integrator pushes to GitHub, escalates to human for approval

Guardian runs integrity checks after every phase. Can HALT the pipeline.

## Design Philosophy

The Architect enforces **derivation over accumulation**:
- Each view has the minimal elements required
- Complexity emerges from composing simple atoms, not adding parts
- A simplification pass runs after every design phase (up to 3 rounds)
- The Reviewer checks generated code for unnecessary complexity
- System prompts are wired to each agent's provider — roles have real context

## Store

Event store backend is selected via `--store` flag or `DATABASE_URL` env var:
- **No flag**: in-memory (local dev, ephemeral)
- **`postgres://...`**: PostgreSQL (Docker locally, Neon in production)

Actor store is in-memory for now — will move to Postgres alongside the event store.

## Dependencies

- `github.com/lovyou-ai/eventgraph/go` — event graph, agent runtime, intelligence, pgstore
- Claude CLI — intelligence backend (flat rate via Max plan, no API key needed)
