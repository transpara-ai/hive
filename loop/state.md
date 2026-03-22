# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 10, 2026-03-22.

## Current System State

Five repos, all compiling and tested:
- **eventgraph** — foundation. Postgres stores, 201 primitives, trust, authority. Complete.
- **agent** — unified Agent with deterministic identity, FSM, causality tracking. Complete.
- **work** — task store for hive agent coordination. Complete.
- **hive** — 4 agents (Strategist, Planner, Implementer, Guardian), agentic loop, budget. Complete.
- **site** — lovyou.ai on Fly.io. **Production-ready:**
  - Blog (43 posts, 6 arcs with section nav)
  - Reference (cognitive grammar, graph grammar, 13 layer grammars, 201 primitives, 28 agent primitives)
  - Auth (Google OAuth — test mode, can be opened whenever)
  - Unified graph product (3 tables, 10 grammar operations, 5 lenses, HTMX, full CRUD)
  - Landing page, SEO meta tags, sitemap (305 URLs), robots.txt
  - Canonical redirect (fly.dev → lovyou.ai)
  - All secrets configured on Fly

Deploy: `fly deploy --remote-only` from site repo.
Fly/Neon resources can be scaled up per user authorization.

## Completed Clusters

- **Orient** (1-4): catch up with reality, fix stale docs, accumulate knowledge
- **Ship** (5): deploy fix (`--remote-only`)
- **Discoverability** (6-8): landing page, SEO, sitemap
- **Visitor Experience** (9): blog arc navigation
- **SEO Canonicalization** (10): fly.dev → lovyou.ai redirect

## Lessons Learned

1. Code is truth, not docs.
2. Verify infra assumptions before building.
3. Update state.md every iteration.
4. Ship what you build — every Build iteration should deploy.
5. Try alternatives before declaring blockers.
6. Name iteration clusters and recognize completion.
7. Hostname middleware must exclude /health (Fly probes via internal IP).

## Vision Notes

- Agents should acquire skills dynamically (like OpenClaw) — email, invoicing, payments, public accounting, any skill.
- Auth gate can be opened to public whenever ready.

## What the Scout Should Focus On Next

The site is production-ready. Stop polishing it. The next cluster should be about:

1. **Hive autonomy** — make the core loop self-running. Highest compounding value. Currently requires manual Claude Code CLI invocation. Even a simple cron-triggered script would be a step toward autonomy.
2. **Product development** — new features in the graph product, or new content.
3. **Agent skill architecture** — design how agents acquire and use skills dynamically.

The Reflector recommends hive autonomy as the next cluster.
