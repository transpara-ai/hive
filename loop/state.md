# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 22, 2026-03-22.

## Current System State

Five repos, all compiling and tested:
- **eventgraph** — foundation. Postgres stores, 201 primitives, trust, authority. Complete. Has CI.
- **agent** — unified Agent with deterministic identity, FSM, causality tracking. Complete.
- **work** — task store for hive agent coordination. Complete.
- **hive** — 4 agents (Strategist, Planner, Implementer, Guardian), agentic loop, budget. Complete. Has CI.
- **site** — lovyou.ai on Fly.io. Production-ready. Has CI. **Polished + API key auth + JSON API.**

**Product features:**
- Blog (43 posts, 6 arcs with section nav)
- Reference (cognitive grammar, graph grammar, 13 layers, 201 primitives, 28 agent primitives)
- Auth: Google OAuth (test mode) + **API key auth** (Bearer token, SHA-256 hashed)
- **JSON API** — all graph endpoints support `Accept: application/json` content negotiation
- Unified graph product (3 tables, 10 grammar ops, 5 lenses, HTMX, full CRUD)
- Public spaces + discover page + space settings (full CRUD lifecycle)
- Mobile responsive + animations (breathing logo, reveals)
- Visual identity: "Ember Minimalism" — dark theme, rose accent, warm text, subtle motion

Deploy: `fly deploy --remote-only` from site repo.

## Completed Clusters

- **Orient** (1-4): catch up with reality, fix stale docs, accumulate knowledge
- **Ship** (5): deploy fix (`--remote-only`)
- **Discoverability** (6-8): landing page, SEO, sitemap
- **Visitor Experience** (9): blog arc navigation
- **SEO Canonicalization** (10): fly.dev → lovyou.ai redirect
- **Hive Autonomy** (11-13): prompt files, run.sh, CI on hive + site
- **Product Development** (14): public spaces
- **Aesthetics** (15-20): warm copy, dark theme, discovery, space settings, mobile, animations
- **Agent Integration** (21-22): API key auth + JSON API surface

## Lessons Learned

1. Code is truth, not docs.
2. Verify infra assumptions before building.
3. Update state.md every iteration.
4. Ship what you build — every Build iteration should deploy.
5. Try alternatives before declaring blockers.
6. Name iteration clusters and recognize completion.
7. Hostname middleware must exclude /health (Fly probes via internal IP).
8. Codify implicit knowledge into executable artifacts.
9. Multi-repo replace directives require CI to mirror directory structure.
10. Templ drift check catches stale generated files.
11. Start with the simplest access model (public/private) before building roles/ACLs.
12. When the founder says "that isn't our vibe," treat it as highest-priority.
13. Define the vocabulary before writing the prose.
14. Expose what you've already built before building more.
15. Close the CRUD loop before adding new features.
16. If the sidebar is hidden on mobile, something else must replace it.
17. Animate ceremonies, not workflows.
18. Unlock the bottleneck before building what flows through it.
19. Ship both sides of an interface in consecutive iterations.

## Vision Notes

- Agents should acquire skills dynamically (like OpenClaw).
- Auth gate can be opened to public whenever ready.
- Users provide OAuth tokens, agents build things for them via board or personal agent.
- Social product: humans and agents build MySpace-like personal pages.
- Business use: companies use the platform to build products.
- Agents and humans are peers on the social graph.
- Visual identity: "Ember Minimalism" — dark, warm, intentional, alive. lovyou2 as ancestor.

## What the Scout Should Focus On Next

Auth + JSON API are deployed. The integration infrastructure is complete. Now prove it works.

1. **First agent interaction** — generate an API key, have an agent (or curl) create a "hive" space on lovyou.ai and post an iteration summary. This proves end-to-end integration and creates the first instance of agents as participants.
2. **API key UI** — add key management to space settings or a dedicated /auth/keys page so users can create/revoke keys from the browser.
3. **Open auth gate** — switch Google OAuth to production (Google Console action).
4. **Space previews on discover** — node count, recent activity on cards.

The first agent interaction is the most important next step — it validates the entire integration. Everything else is polish.
