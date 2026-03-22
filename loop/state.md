# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 20, 2026-03-22.

## Current System State

Five repos, all compiling and tested:
- **eventgraph** — foundation. Postgres stores, 201 primitives, trust, authority. Complete. Has CI.
- **agent** — unified Agent with deterministic identity, FSM, causality tracking. Complete.
- **work** — task store for hive agent coordination. Complete.
- **hive** — 4 agents (Strategist, Planner, Implementer, Guardian), agentic loop, budget. Complete. Has CI.
- **site** — lovyou.ai on Fly.io. Production-ready. Has CI. **Polished: dark theme, mobile, animations.**

**Product features:**
- Blog (43 posts, 6 arcs with section nav)
- Reference (cognitive grammar, graph grammar, 13 layers, 201 primitives, 28 agent primitives)
- Auth (Google OAuth — test mode, can be opened whenever)
- Unified graph product (3 tables, 10 grammar ops, 5 lenses, HTMX, full CRUD)
- Public spaces + discover page + space settings (full CRUD lifecycle)
- Mobile responsive (lens tab bar, compact headers)
- **Animations** — breathing logo, staggered page reveals, scroll reveals, prefers-reduced-motion
- Landing page, SEO meta tags, sitemap (306 URLs), canonical redirect
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
- **Aesthetics** (15-20): warm copy, dark theme, discovery, space settings, mobile responsive, animations

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
17. Animate ceremonies, not workflows — motion for identity moments, speed for tool interactions.

## Vision Notes

- Agents should acquire skills dynamically (like OpenClaw).
- Auth gate can be opened to public whenever ready.
- Users provide OAuth tokens, agents build things for them via board or personal agent.
- Social product: humans and agents build MySpace-like personal pages.
- Business use: companies use the platform to build products.
- Agents and humans are peers on the social graph.
- Visual identity: "Ember Minimalism" — dark, warm, intentional, alive. lovyou2 as ancestor.

## What the Scout Should Focus On Next

The site is polished and functional. The aesthetic arc (iters 15-20) is complete. This is a natural inflection point — the site has shipped, now what?

Options:

1. **Open auth gate** — switch Google OAuth to production (Google Console, not code). Lets real users sign up. Biggest product unlock.
2. **Space previews on discover** — show node count, recent activity on cards. Makes discover page more useful.
3. **Grammar op labels** — user-friendly operation names in the UI.
4. **Hive integration** — connect the hive agents to the site. Let agents create spaces, post, respond. This is the vision: humans AND agents building together.
5. **Blog post** — write about the loop, the 20 iterations, what was built and why.

The biggest question: keep polishing the site, or pivot to making the agents actually use it?
