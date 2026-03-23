# Scout Report — Iteration 122

## Gap: Task dependencies are invisible

The `depend` op creates dependencies (stored in `node_deps` table). `BlockerCount` is calculated and shown as a red "X blocked" badge on Board task cards. But that's a dead end — users can't see WHICH tasks are blocking them or what they're blocking. Clicking through reveals nothing.

When Mind decomposes tasks and adds dependencies, the dependency structure is invisible. Users see "2 blocked" and have no way to understand why or navigate the chain.

The infrastructure exists (table, handler, count calculation). The gap is purely UI: show blocking/blocked tasks as clickable links on the node detail page.

**Scope:** Add a "Dependencies" section to NodeDetailView showing blockers (tasks this depends on) and dependents (tasks that depend on this) as navigable links with status indicators.
