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

Build order: Work Graph first (the hive needs it), then Mind (the hive needs to learn from experience), then Market, Social, Knowledge, Alignment. The Mind comes before Market/Social because reputation and relationships require judgment that only accumulated experience provides. Each product is derived using the [derivation method](https://github.com/lovyou-ai/eventgraph/blob/main/docs/derivation-method.md).

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
| CTO | Architectural oversight, escalation filtering | Sonnet | 0.1 | Human |
| Guardian | Independent integrity, halt/rollback/quarantine | Sonnet | 0.1 | Human (directly) |
| SysMon | System health, error detection, anomaly tracking | Haiku | 0.1 | Guardian |
| Spawner | Identify workforce gaps, propose new agents | Sonnet | 0.5 | CTO |
| Allocator | Resource allocation, model selection, budget enforcement | Haiku | 0.3 | CTO |

### Product Pipeline

| Role | Responsibility | Intelligence | Trust Gate | Reports To |
|------|---------------|-------------|------------|-----------|
| PM | Product vision, prioritization, user needs, launch readiness | Sonnet | 0.3 | Human |
| Researcher | Read URLs, extract product ideas | Sonnet | 0.3 | CTO |
| Architect | Design systems via derivation method | Sonnet | 0.3 | CTO |
| Builder | Generate code + tests from specs | Sonnet | 0.3 | CTO |
| Reviewer | Code quality, security, derivation compliance | Sonnet | 0.5 | CTO |
| Tester | Run tests, validate behaviour | Sonnet | 0.3 | CTO |
| Integrator | Assemble, deploy, health check | Sonnet | 0.7 | CTO |

The Spawner grows the workforce through the growth loop: something breaks → "what role should have caught that?" → if none exists, propose one → human approves. See [ROLES.md](docs/ROLES.md) for the full role architecture.

## Dev Setup

```bash
cd hive
docker compose up -d postgres   # local Postgres for event/actor/state stores
go build ./...
go test ./...
```

## Running

All examples use `--store` for Postgres persistence. Omit it for in-memory (ephemeral). Can also set `DATABASE_URL` env var instead.

```bash
# Local dev — sequential pipeline
go run ./cmd/hive --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a task management app with kanban boards"

# Auto-approve all agent spawns (dev/testing — skips interactive prompts)
go run ./cmd/hive --human Matt --yes --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a task management app with kanban boards"

# Fast dev mode — auto-approve + skip Guardian checks (NOT recommended, Guardian should run)
go run ./cmd/hive --human Matt --yes --skip-guardian --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a task management app with kanban boards"

# Agentic loop mode — concurrent self-directing agents
go run ./cmd/hive --human Matt --loop --store "postgres://hive:hive@localhost:5432/hive" --idea "Build a task management app with kanban boards"

# From a URL with an explicit product name
go run ./cmd/hive --human Matt --name social-grammar --store "postgres://hive:hive@localhost:5432/hive" --url "https://mattsearles2.substack.com/p/the-missing-social-grammar"

# From a Code Graph spec file
go run ./cmd/hive --human Matt --store "postgres://hive:hive@localhost:5432/hive" --spec path/to/spec.cg

# Targeted mode — modify existing code (creates branch + PR)
go run ./cmd/hive --human Matt --yes --store "postgres://hive:hive@localhost:5432/hive" --repo /path/to/repo --idea "add a has command"

# Self-improvement — analyze telemetry + codebase, apply fixes (up to 10 iterations)
go run ./cmd/hive --human Matt --yes --self-improve --store "postgres://hive:hive@localhost:5432/hive"

# Evolution — build new capabilities and features (up to 5 iterations)
go run ./cmd/hive --human Matt --yes --evolve --store "postgres://hive:hive@localhost:5432/hive"

# Evolution with human direction
go run ./cmd/hive --human Matt --yes --evolve --store "postgres://hive:hive@localhost:5432/hive" --idea "add agent communication channels"

# Query pipeline events from the event graph
go run ./cmd/hive --store "postgres://hive:hive@localhost:5432/hive" -q
go run ./cmd/hive --store "postgres://hive:hive@localhost:5432/hive" --query phase
go run ./cmd/hive --store "postgres://hive:hive@localhost:5432/hive" --query telemetry
```

## Key Files

- `pkg/roles/` — Agent role definitions and system prompts
- `pkg/pipeline/` — Product pipeline orchestration (sequential + agentic loop modes)
- `pkg/loop/` — Agentic loop runner (observe-reason-act-reflect cycles)
- `pkg/resources/` — Budget enforcement (tokens, cost, iterations, duration)
- `pkg/spawn/` — Agent spawning with authority gates and trust checks
- `pkg/authority/` — Three-tier approval model (Required/Recommended/Notification)
- `pkg/mcp/` — MCP server for agent tools
- `pkg/workspace/` — File system and git management for generated code
- `cmd/hive/` — CLI entry point

## Coding Standards

See `docs/CODING-STANDARDS.md` for full details. The cardinal rules:

- **No magic values** — every event type, authority level, actor type, role uses defined constants/enums. Never bare strings with implicit meaning. If a constant exists, use it. If one doesn't exist, create it. Magic values are the root of all evil.
- **Always-valid domain models** — validate at construction, guaranteed valid for lifetime
- **Make illegal states unrepresentable** — constrained types, state machines, typed IDs
- **Typed errors** — domain error types, not string messages you have to parse
- **Explicit optionality** — `Option[T]`, no nil/zero-value-means-absent

## Intelligence

All inference runs through **Claude CLI** (Max plan, flat rate). NOT the Anthropic API — CLI is cheaper and better for our use case. The pipeline creates `claude-cli` providers automatically.

### Authentication

The CLI authenticates via OAuth token stored in `~/.claude/.credentials.json`. To rotate a token:

1. Generate a new OAuth token from your Max plan account
2. Replace the `accessToken` value in `~/.claude/.credentials.json`

Token format: `sk-ant-oat01-...` (OAuth access token). The `refreshToken` and other fields remain unchanged. The hive's `claude-cli` provider inherits whatever auth Claude Code already has — no separate credentials needed.

**Never commit `.credentials.json` or tokens to the repo.**

### Model Assignment

Model assignment by role (two tiers):
- **Sonnet** (`claude-sonnet-4-6`): CTO, Guardian, Architect, Builder, Reviewer, Tester, Integrator, Researcher, Spawner — all judgment and execution tasks
- **Haiku** (`claude-haiku-4-5-20251001`): SysMon, Allocator — high-volume, simple tasks

## Pipeline Modes

### Sequential — Full Pipeline (default)
Fixed phase sequence: Research → Design → Simplify → Build → Review → Test → Integrate. For greenfield projects — generating new products from ideas/specs. Guardian checks after each phase. Human approves agent spawns.

### Sequential — Targeted Mode (`--repo`)
For modifying existing code: Context Load → Understand → Modify → Review → Test → PR. Skips research/design/simplify. Builder and reviewer receive existing codebase as context. Creates branch and PR, not direct commits. Used for self-improvement and feature additions.

### Self-Improve (`--self-improve`)
CTO analyzes telemetry + full codebase, identifies bugs and correctness issues, runs targeted pipeline to fix them. Up to 10 iterations per session, stops when CTO finds nothing worth fixing.

### Evolve (`--evolve`)
CTO reads full codebase + architecture roadmap, proposes new capabilities and features to build. Unlike self-improve (which fixes bugs), evolve builds what's missing. Up to 5 iterations per session. Accepts `--idea` for human direction ("build agent communication channels"). Guardian stays active for integrity checks on new features.

### Agentic Loop (`--loop`)
CTO seeds work, then agents run concurrent observe-reason-act-reflect loops. They communicate through events on the shared graph. IBus provides real-time event notification. Budget enforcement prevents runaway agents. Agents stop on: quiescence (nothing to do), escalation (needs human), HALT (Guardian), or budget limit.

## Sequential Pipeline Detail

### Full Pipeline (greenfield)
1. **Research** — Researcher reads URLs/ideas, CTO evaluates feasibility
2. **Design** — Architect creates Code Graph spec, CTO reviews for minimalism
3. **Simplify** — Architect reduces spec to minimal form (up to 2 rounds)
4. **Build** — Builder generates multi-file project, committed to product repo
5. **Review → Rebuild** — Reviewer checks quality/compliance/simplicity (up to 3 rounds)
6. **Test** — Tester runs actual test suite, Builder fixes failures. **Pipeline halts if tests still fail after fix attempt.**
7. **Integrate** — Integrator pushes to GitHub, escalates to human for approval

### Targeted Pipeline (existing code)
1. **Context Load** — Read existing files, git history, SPEC.md from target repo
2. **Understand** — CTO evaluates the change request against existing codebase
3. **Modify** — Builder modifies specific files (receives full codebase as context)
4. **Review** — Reviewer checks diff against existing code
5. **Test** — Run existing test suite + any new tests
6. **PR** — Create branch, commit, open pull request

Guardian runs integrity checks after every phase in both modes. Can HALT the pipeline.

## Design Philosophy

The Architect enforces **derivation over accumulation**:
- Each view has the minimal elements required
- Complexity emerges from composing simple atoms, not adding parts
- A simplification pass runs after every design phase (up to 3 rounds)
- The Reviewer checks generated code for unnecessary complexity
- System prompts are wired to each agent's provider — roles have real context

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

**Accept and Release are stopping conditions.** The original method had none — it iterated forever. The self-derivation produced the method's own recognition that some gaps should remain gaps. This applies directly: the reviewer uses Accept (don't block for style nits), the pipeline uses Release (move on when good enough).

See `eventgraph/docs/generator-function.md` and `eventgraph/docs/generator-function-self-derivation.md`.

## Store

All three stores (event, actor, state) use the same backend, selected via `--store` flag or `DATABASE_URL` env var:
- **No flag**: in-memory (ephemeral, lost on exit)
- **`postgres://...`**: PostgreSQL (Docker locally, Neon in production)

Tables auto-create on first connection (`CREATE TABLE IF NOT EXISTS`). Local Postgres runs via `docker compose up -d postgres` with DSN `postgres://hive:hive@localhost:5432/hive`.

Trust state persists across runs in the state store. Events accumulate on the graph. Self-improve telemetry is also written to `.hive/telemetry/` as JSON files for CTO analysis.

## Timeouts

Claude CLI calls have default timeouts to prevent hung subprocesses:
- **Reason()**: 5 minutes (CTO analysis, reviews, evaluations)
- **Operate()**: 10 minutes (code generation, test runs)
- **Self-improve iteration**: 15 minutes (CTO analysis + full targeted pipeline)

Timeouts only apply when the parent context has no explicit deadline.

## Division of Labour: Claude Code vs The Hive

The hive's fifth invariant is **SELF-EVOLVE** — agents fix agents, not humans. Claude Code (you) handles infrastructure; the hive handles product work.

**Claude Code should:**
- Fix hive infrastructure (pipeline modes, flags, parsing, prompts)
- Add new pipeline capabilities (e.g. `--resume`, self-heal)
- Fix broken LLM prompt engineering or JSON parsing
- Make architectural decisions about the hive itself
- Monitor runs, commit, push, clean up branches/PRs
- Fix compilation errors or test failures in hive infrastructure

**The hive should (via `--evolve` or `--self-improve`):**
- Build product features (Work Graph, Market Graph, etc.)
- Fix its own bugs found through telemetry analysis
- Write application code and tests for the thirteen products
- Wire new capabilities into the pipeline (e.g. task tracking)

**Rule of thumb:** If it's infrastructure/plumbing for the hive CLI or pipeline orchestration, Claude Code does it. If it's product work the hive can build autonomously, let the hive do it with `--evolve --idea "..."`.

**Critical workflow rule:** Always commit AND push to origin before running `hive --evolve --repo .` or `hive --self-improve --repo .`. The hive's `CleanupForIteration()` runs `git reset --hard origin/main`, which destroys any unpushed local commits.

## Dependencies

- `github.com/lovyou-ai/eventgraph/go` — event graph, agent runtime, intelligence, pgstore
- Claude CLI — intelligence backend (flat rate via Max plan, no API key needed)
