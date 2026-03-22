# Roadmap

## Where We Are

The hive is a working agent-first runtime. Four agents run concurrent loops, coordinate through work tasks on a shared event graph, and produce code via Claude CLI's Operate mode. Identity is persistent (deterministic key derivation, Postgres actor store). Budget enforcement, stopping conditions, and Guardian oversight are all functional.

**What exists (by repo):**

| Repo | What it does | Status |
|------|-------------|--------|
| eventgraph | Foundation: event store, actor store, trust, authority, intelligence providers, 201 primitives | Complete, tested |
| agent | Unified Agent type: FSM, lifecycle, causality tracking, trust hooks | Complete, tested |
| work | Work Graph: task store, priorities, dependencies, assignment, events | Complete, tested |
| hive | Runtime: 4 agents, agentic loop, budget, workspace, git | Complete, tested |
| site | lovyou.ai: blog, reference, auth, unified graph product | Deployed on Fly.io |

**What works today:**
- Agent loops: observe → reason/operate → check stopping → repeat
- Task coordination: Strategist creates, Planner decomposes, Implementer picks up and builds
- Code generation: Implementer uses Operate for full filesystem access
- Persistence: pgstore + pgactor on Postgres, deterministic agent identity across runs
- Guardian: independent integrity monitor with HALT authority
- Budget: iteration limits, duration limits, token tracking

## How We Decide What's Next

The core loop (see [CORE-LOOP.md](CORE-LOOP.md)). Scout reads the current state, identifies the most load-bearing gap, Builder fills it, Critic verifies, Reflector captures lessons. No fixed milestone sequence — the loop determines priority from first principles each iteration.

## What's Not Built Yet

Roughly in dependency order, but the core loop decides actual priority:

**Agent capabilities**
- MCP tools — agents querying the graph mid-reasoning
- Agent spawning protocol — creating new agents with authority checks
- Trust decay and enforcement — trust scores affecting authority levels

**Infrastructure**
- Web daemon (cmd/hived) — long-running HTTP service
- Auth integration — Google OAuth for human operators via web
- Approval dashboard — web UI for authority requests
- CI/CD — GitHub Actions, automated deploy

**Products**
- Unified graph product — grammar-first Board/Feed/Threads on lovyou.ai
- Additional product layers (Market, Social, Justice, etc.)
- Revenue infrastructure — billing, subscriptions

**Autonomy**
- Self-directing loop — hive decides its own work without --idea flag
- Persistent conversation threading across runs
- Scheduling — cron or event-driven loop triggers

## Key Decisions

1. **Self-modification: yes.** PRs to lovyou-ai/*, reviewed by human.
2. **One service.** lovyou.ai does everything. Web first.
3. **High scrutiny initially.** Everything starts as Required authority.
4. **Core loop over milestones.** The loop decides what to build, not a checklist.
5. **Code is truth.** Docs describe intent and vision. Code describes reality.
