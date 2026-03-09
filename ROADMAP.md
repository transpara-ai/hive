# Hive Roadmap

## Key Decisions

1. **Self-modification: yes.** The hive can and should modify its own codebase. PRs to lovyou-ai/hive, reviewed by human. This is how it builds its own tools.
2. **One service.** lovyou.ai is one product that does everything — not microservices. Web first, mobile later. The CTO/CTO-agent decides architecture.
3. **High scrutiny initially.** Every action reviewed in detail by human. Authority model starts strict (everything is "Required" approval). Trust accumulates through verified work — supervision decreases as the hive proves itself.
4. **CLI first, daemon soon.** Keep CLI for stepping through and debugging. Architect the code so the same pipeline can run as a long-running daemon. CLI and daemon share the same packages.

## Where We Are

The hive can take a product idea, research it, design a Code Graph spec, generate multi-file code, review it, test it, and push it to a GitHub repo. All agents share one event graph. Guardian checks integrity after every phase. Store is configurable (in-memory or Postgres). Actor IDs come from the actor store.

**What works today:**
- CLI pipeline: idea → research → design → simplify → build → review → test → integrate
- Per-role intelligence (Opus for judgment, Sonnet for execution)
- Multi-file code generation with review/rebuild loop
- Product repos with git commits at each phase
- Guardian integrity checks with HALT capability
- Postgres event store (via eventgraph pgstore)
- Actor registration (in-memory, human bootstrap via CLI)

## What's Missing (Foundation Gaps)

These must be solid before the hive builds real products.

### 1. Persistent Actor Store (Postgres)

**Gap:** Only InMemoryActorStore exists. Agents forget who they are between runs.

**Need:** Postgres-backed IActorStore in eventgraph, matching the pgstore pattern. Without this, the hive can't be a long-running service — every restart loses all actor registrations and trust history.

**Where:** `eventgraph/go/pkg/actor/pgactor/` (new package)

### 2. Auth Layer (Google OAuth)

**Gap:** Humans are bootstrapped via CLI flag with deterministic keys. No real auth.

**Need:** Google OAuth2 flow → register human in actor store → issue session. This is the entry point for lovyou.ai. Every human who uses any product goes through this.

**Where:** `hive/pkg/auth/` (new package)

### 3. Web Layer (HTTP Service)

**Gap:** Hive is a CLI tool that runs once and exits. No web interface.

**Need:** One HTTP service that is lovyou.ai — everything in one binary:
- Serves docs, blog, product UIs
- Handles Google auth
- Provides an API for products to interact with the graph
- Provides the human approval surface (authority requests)
- Runs on fly.io
- Web first, mobile apps later

The CLI (`cmd/hive`) and the daemon (`cmd/hived`) share the same packages. CLI is for stepping through and debugging. Daemon is the production service.

**Where:** `hive/cmd/hived/` (daemon entry point), `hive/pkg/web/` (HTTP handlers)

### 4. Human Approval Surface

**Gap:** Authority escalations go to the event graph but no human can see them.

**Need:** When the hive escalates "I want to spawn 4 agents", a human needs to see it and approve/reject. Initially EVERYTHING needs human approval — the hive starts with zero autonomy and earns it.

Early stage: CLI prompts (step-through debugging mode). Then: web dashboard showing pending authority requests with approve/reject buttons. As trust accumulates, more decisions move from "Required" to "Recommended" to "Notification".

**Where:** Part of the web layer. Starts as CLI prompts in `cmd/hive`.

### 5. Deployment Pipeline

**Gap:** Products get pushed to GitHub but not deployed.

**Need:** Products need to be:
- Built (Docker image)
- Deployed to fly.io (or similar)
- Routed via DNS (product.lovyou.ai)
- Health-checked

The Integrator role needs actual deployment capability, not just `git push`.

**Where:** `hive/pkg/deploy/` (new package), Integrator system prompt update

### 6. Self-Modification

**Gap:** The hive builds external products but can't improve itself.

**Need:** The hive should be able to:
- Identify gaps in its own capabilities
- Propose changes to its own codebase (lovyou-ai/hive)
- Submit PRs — human reviews every one initially
- Rebuild and redeploy itself after approval

This is the first thing the hive does. It builds tools for itself: the task manager, the communication layer, the governance framework. These aren't external products — they're self-improvement. The hive's first product is itself.

Guardian gets extra scrutiny for self-modification. All self-mod PRs require human approval (Required authority level, never auto-approve).

**Where:** Pipeline needs a "self" mode targeting the hive repo. Guardian needs self-modification audit rules.

### 7. Agent Spawning with Authority

**Gap:** Agents are created silently by the pipeline. No authority check.

**Need:** Creating a new agent should be:
- An explicit decision (by CTO or another agent)
- Subject to authority approval (human must approve for high-trust roles)
- Recorded as events on the graph
- Configurable (role, trust level, capability bounds)

The vision: agents specify roles using agent primitives (soul files, authority scopes, capability requirements), then escalate for approval.

**Where:** `hive/pkg/spawn/` or integrate into roles package

### 8. Agent Communication (Real-time)

**Gap:** Agents communicate only through events on the graph. No real-time notification.

**Need:** When an agent emits an event that another agent subscribes to, the subscriber should be notified immediately. The event graph is the source of truth, but real-time notification makes the society responsive.

**Where:** EventGraph's IBus interface handles this. Need to wire it into the hive.

### 9. Docker Compose (Local Dev)

**Gap:** No containerized local dev environment.

**Need:** `docker-compose.yml` with:
- Postgres (for event + actor store)
- The hive service itself
- Optional: pgAdmin for debugging

**Where:** `hive/docker-compose.yml`

### 10. CI/CD

**Gap:** No automated testing or deployment.

**Need:** GitHub Actions for:
- Build + test on PR
- Deploy to fly.io on merge to main

**Where:** `hive/.github/workflows/`

## Build Sequence

Dependency order — each tier unlocks the next.

### Tier 0: Foundation (Current)
- [x] Event graph with typed events and causal links
- [x] Agent runtime with identity and signing
- [x] CLI pipeline (research → build → deploy to GitHub)
- [x] Guardian integrity checks
- [x] Postgres event store
- [x] Actor store (in-memory)
- [x] Per-role intelligence (Opus/Sonnet)

### Tier 1: Persistence
- [ ] Postgres actor store (in eventgraph)
- [ ] Docker Compose for local Postgres
- [ ] Store selection wired end-to-end

### Tier 2: Web + Auth
- [ ] HTTP daemon (`hived`)
- [ ] Google OAuth2 → actor store registration
- [ ] Human approval dashboard (pending authority requests)
- [ ] Serve static content (docs, blog placeholder)

### Tier 3: Deployment
- [ ] Dockerfile for the hive
- [ ] fly.io deployment (manual first)
- [ ] CI/CD (GitHub Actions)
- [ ] Product deployment (Integrator deploys to fly.io, not just GitHub)

### Tier 4: Self-Improvement
- [ ] Hive can modify its own codebase (PR-based, human-approved)
- [ ] Hive builds its own task manager (Work Graph, layer 1)
- [ ] Hive builds its own communication layer
- [ ] Agent spawning with authority model

### Tier 5: First Products
- [ ] Task manager (Work Graph) — the hive's first real product
- [ ] Knowledge store (Knowledge Graph) — for the Researcher
- [ ] Governance dashboard (Social Graph) — norms, roles, consent
- [ ] lovyou.ai public site with docs, blog, product access

### Tier 6: Economy
- [ ] Revenue infrastructure (Stripe, billing)
- [ ] Market Graph — portable reputation, escrow
- [ ] Social Graph — user-owned social infrastructure
- [ ] Alignment Graph — AI accountability for regulators

## Neon vs Docker Postgres

- **Local dev:** Docker Postgres (docker-compose)
- **Staging/production:** Neon (serverless Postgres, scales to zero)
- **Connection string is the only difference** — pgstore handles both identically
- **fly.io** reads `DATABASE_URL` env var pointing to Neon

## Revenue Model

Each product generates revenue that funds the next:
- **Corporations pay.** Enterprise features, SLAs, compliance tools.
- **Individuals free.** Core functionality available to everyone.
- **Hosted persistence.** People who don't want to run their own infrastructure pay for hosted graph storage.
- **BSL → Apache 2.0.** Code is source-available, becomes fully open after change date.

Revenue funds agents. Agents build products. Products generate revenue. The civilisation builds the products that fund the civilisation.
