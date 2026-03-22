# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 12, 2026-03-22.

## Current System State

Five repos, all compiling and tested:
- **eventgraph** — foundation. Postgres stores, 201 primitives, trust, authority. Complete. Has CI.
- **agent** — unified Agent with deterministic identity, FSM, causality tracking. Complete.
- **work** — task store for hive agent coordination. Complete.
- **hive** — 4 agents (Strategist, Planner, Implementer, Guardian), agentic loop, budget. Complete. **Has CI** (build + test on push/PR).
- **site** — lovyou.ai on Fly.io. Production-ready. No CI yet.

**Core loop infrastructure:**
- `loop/run.sh` — orchestrates Scout → Builder → Critic → Reflector via `claude -p`
- Four phase prompt files (scout, builder, critic, reflector)
- `.github/workflows/ci.yml` — build + test on push, PR, and workflow_dispatch
- Run: `cd /c/src/matt/lovyou3/hive && ./loop/run.sh`

Deploy: `fly deploy --remote-only` from site repo.
Fly/Neon resources can be scaled up per user authorization.

## Completed Clusters

- **Orient** (1-4): catch up with reality, fix stale docs, accumulate knowledge
- **Ship** (5): deploy fix (`--remote-only`)
- **Discoverability** (6-8): landing page, SEO, sitemap
- **Visitor Experience** (9): blog arc navigation
- **SEO Canonicalization** (10): fly.dev → lovyou.ai redirect
- **Hive Autonomy: Foundation** (11): executable prompt files + run.sh
- **Hive Autonomy: CI** (12): GitHub Actions build + test

## Lessons Learned

1. Code is truth, not docs.
2. Verify infra assumptions before building.
3. Update state.md every iteration.
4. Ship what you build — every Build iteration should deploy.
5. Try alternatives before declaring blockers.
6. Name iteration clusters and recognize completion.
7. Hostname middleware must exclude /health (Fly probes via internal IP).
8. Codify implicit knowledge into executable artifacts — conversation context is ephemeral, files persist.
9. Multi-repo replace directives require CI to mirror the local directory structure (checkout siblings).

## Vision Notes

- Agents should acquire skills dynamically (like OpenClaw) — email, invoicing, payments, public accounting, any skill.
- Auth gate can be opened to public whenever ready.

## What the Scout Should Focus On Next

Hive Autonomy cluster is progressing (11: prompts, 12: CI). Options:

1. **Hive autonomy: scheduled runs** — add a cron or workflow_dispatch job that runs `./loop/run.sh`. Requires Claude Code CLI or API key in GitHub Actions. Continues the cluster.
2. **Product development** — the grammar-first unified product plan exists. Verify if it's already implemented in the site. If not, build it. If so, consider opening the auth gate.
3. **Site CI** — the site also has no CI. Could be a quick win.

The Hive Autonomy cluster could be closed here (foundation + CI is a natural stopping point) or extended with scheduled runs. The loop should decide based on what has the most compounding value.
