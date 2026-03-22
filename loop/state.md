# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 3, 2026-03-22.

## Current System State

Five repos, all compiling and tested:
- **eventgraph** — foundation. Postgres stores, 201 primitives, trust, authority. Complete.
- **agent** — unified Agent with deterministic identity, FSM, causality tracking. Complete.
- **work** — task store for hive agent coordination. Complete.
- **hive** — 4 agents (Strategist, Planner, Implementer, Guardian), agentic loop, budget. Complete.
- **site** — lovyou.ai on Fly.io. Complete product:
  - Blog (markdown → HTML)
  - Reference (cognitive grammar, graph grammar, 13 layer grammars, 201 primitives, 28 agent primitives)
  - Auth (Google OAuth, anonymous fallback)
  - Unified graph product (3 tables: spaces/nodes/ops, 10 grammar operations, 5 lenses: Board/Feed/Threads/People/Activity, HTMX, full CRUD)

Entry point: `cmd/hive` (CLI only). No daemon, no web dashboard for the hive itself.

## Lessons Learned

### Iteration 1: Code is truth, not docs
The Scout read the roadmap, believed a milestone was incomplete, and identified a non-gap. **Always read code to assess current state.**

### Iteration 2: Doc cleanup
ROADMAP.md and ARCHITECTURE.md were stale. Rewritten to match code. Created state.md and reflections.md for knowledge accumulation.

### Iteration 3: The system is more complete than the loop knew
State.md called the graph product a "skeleton." It's a full implementation (595+754+1012 lines). The loop's own knowledge lagged behind reality. Dead code (site/work/) was deleted. **Update state.md every iteration to prevent phantom gaps.**

## Known Issues

- Site deploy stuck — Docker hanging on `fly deploy --local-only`. Cognitive grammar reference page committed and pushed but not live. Likely needs Docker Desktop restart.
- AUDIT.md in hive/docs/ is stale (March 9th, marks everything "Solid").
- Boot events re-emitted every agent run (chatty but harmless).

## What the Scout Should Focus On Next

The code gaps are mostly closed. The remaining gaps are:
- **Operational:** Fix the deploy (Docker issue, not code). Get cognitive grammar live.
- **Growth:** No users. No marketing. No way for people to discover lovyou.ai.
- **Hive autonomy:** Core loop prompt files don't exist yet. The loop runs manually.
- **Infrastructure:** No web daemon for the hive itself (only CLI).

The Scout should evaluate whether the next step is operational (fix deploy), growth-oriented (make lovyou.ai discoverable), or autonomy-oriented (make the loop executable).
