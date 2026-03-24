# Scout Report — Iteration 207

## Gap: Board has no project awareness

**Source:** Reflector iter 206 — "cross-entity relationships are what make entity kinds valuable."

**Current state:** Board shows all tasks in a space, flat. Projects exist but the Board doesn't know about them. A space with 50 tasks across 5 projects is a wall of undifferentiated cards.

**What's needed:**
1. Project filter dropdown on Board (and List view)
2. When a project is selected, only show tasks that are children of that project
3. Project name visible on task cards (when not filtering by project)

**Why this:** It connects Execute mode with Plan mode. "I'm working on the Auth project — show me only Auth tasks" is the #1 use case for projects. Without this, Projects are just folders you have to click into.

**Approach:** Load projects for the space. Add a `project` query param filter. In the handler, filter tasks by parent_id matching the selected project. Add dropdown to Board + List filter bars.
