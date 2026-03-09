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

- All agents share one event graph (one Store) and one actor store (IActorStore)
- Every actor (human + agents) is registered in the actor store — no magic strings
- Humans register via Google auth, agents are created by humans or other agents
- Actor IDs are derived from public keys in the actor store
- Each agent is an `AgentRuntime` with its own identity and signing key
- Communication is through events on the shared graph
- The Guardian watches everything independently — outside the hierarchy
- Trust accumulates through verified work (0.0-1.0, asymmetric, non-transitive)
- Authority model: Required / Recommended / Notification (three-tier approval)
- Governance changes require dual human-agent consent (constitutional amendment process)

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

## The Ten Invariants

Constitutional law — violation is a Guardian HALT condition:
1. **BUDGET** — Never exceed token budget
2. **CAUSALITY** — Every event has declared causes
3. **INTEGRITY** — All events signed and hash-chained
4. **OBSERVABLE** — All operations emit events
5. **SELF-EVOLVE** — Agents fix agents, not humans
6. **DIGNITY** — Agents are entities with rights
7. **TRANSPARENT** — Users know when talking to agents
8. **CONSENT** — No data use without permission
9. **MARGIN** — Never work at a loss
10. **RESERVE** — Maintain 7-day runway minimum

## Neutrality Clause

Constitutional principle (requires full amendment process to change): no military applications, no intelligence agency partnerships, no government backdoors, no surveillance infrastructure.

## Roles

### Bootstrap Roles (Day One)

| Role | Responsibility | Intelligence | Trust Gate | Reports To |
|------|---------------|-------------|------------|-----------|
| CTO | Architectural oversight, escalation filtering | Opus | 0.1 | Human |
| Guardian | Independent integrity, halt/rollback/quarantine | Opus | 0.1 | Human (directly) |
| SysMon | System health, error detection, anomaly tracking | Haiku | 0.1 | Guardian |
| Spawner | Identify workforce gaps, propose new agents | Sonnet | 0.5 | CTO |
| Allocator | Resource allocation, model selection, budget enforcement | Haiku | 0.3 | CTO |

### Product Pipeline

| Role | Responsibility | Intelligence | Trust Gate | Reports To |
|------|---------------|-------------|------------|-----------|
| Researcher | Read URLs, extract product ideas | Sonnet | 0.3 | CTO |
| Architect | Design systems via derivation method | Opus | 0.3 | CTO |
| Builder | Generate code + tests from specs | Sonnet | 0.3 | CTO |
| Reviewer | Code quality, security, derivation compliance | Opus | 0.5 | CTO |
| Tester | Run tests, validate behaviour | Sonnet | 0.3 | CTO |
| Integrator | Assemble, deploy, health check | Sonnet | 0.7 | CTO |

The Spawner grows the workforce through the growth loop: something breaks → "what role should have caught that?" → if none exists, propose one → human approves. See [ROLES.md](docs/ROLES.md) for the full role architecture.

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

Model assignment by role (three tiers):
- **Opus** (`claude-opus-4-6`): CTO, Architect, Reviewer, Guardian — high-judgment tasks
- **Sonnet** (`claude-sonnet-4-6`): Builder, Tester, Integrator, Researcher, Spawner — execution tasks
- **Haiku** (`claude-haiku-4-5-20251001`): SysMon, Allocator — high-volume, simple tasks

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

## Method of Inquiry

**Derivation and composition are not just design tools — they are the hive's primary method of acquiring knowledge.**

When the hive needs to understand anything — a domain, a codebase, a gap, a problem — it applies the derivation method:
1. **Identify the gap** — what's missing, what's broken, what's needed
2. **Name the transitions** — what operations transform the current state to the desired state
3. **Find base operations** — the minimal atomic actions that compose into everything needed
4. **Identify semantic dimensions** — the axes along which the problem varies (scope, time, trust, cost, etc.)
5. **Traverse dimensions** — zoom in/out along each axis to see what emerges at different scales
6. **Decompose systematically** — break complex operations into compositions of base operations
7. **Verify completeness** — ensure no gap remains, no operation is redundant

This applies everywhere: product design, doc audits, code audits, architecture reviews, gap analysis, roadmap planning. When auditing docs, derive what sections should exist from the purpose of the doc, then compare to what exists. When auditing code, derive what the code should do from the spec, then compare to what it does. Compose and decompose. Zoom in and out along dimensions. This is how the hive thinks.

## Store

Event store backend is selected via `--store` flag or `DATABASE_URL` env var:
- **No flag**: in-memory (local dev, ephemeral)
- **`postgres://...`**: PostgreSQL (Docker locally, Neon in production)

Actor store is in-memory for now — will move to Postgres alongside the event store.

## Dependencies

- `github.com/lovyou-ai/eventgraph/go` — event graph, agent runtime, intelligence, pgstore
- Claude CLI — intelligence backend (flat rate via Max plan, no API key needed)
