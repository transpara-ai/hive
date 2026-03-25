# Build Report — Iteration 240 (Fix)

## Gap addressed

Critic review of iter 240 (commit 8d55baa) found an IDENTITY invariant violation:
`GetHiveCurrentTask` and `GetHiveTotals` aggregated across **all agents** instead of scoping
to the specific hive agent's `actor_id`. If a second agent ever ran, their data would silently
pool into the `/hive` dashboard — violating invariant 11 (IDENTITY).

Secondary gap: the handler passed `""` to `ListHiveActivity`, so posts were unscoped too.

## What was built

### 1. `site/graph/store.go` — three changes

**`GetHiveCurrentTask(ctx, actorID string)`** — added `actorID` parameter.
- When `actorID != ""`: `WHERE n.author_id = $1` — scoped to specific actor
- When `actorID == ""`: falls back to `author_kind = 'agent'` (dev/empty DB)

**`GetHiveTotals(ctx, actorID string)`** — added `actorID` parameter.
- When `actorID != ""`: `WHERE actor_id = $1` — scoped to specific actor
- When `actorID == ""`: falls back to JOIN on `users.kind = 'agent'`

**`GetHiveAgentID(ctx)`** — new function.
- Queries `api_keys WHERE agent_id IS NOT NULL ORDER BY created_at ASC LIMIT 1`
- Returns the actor ID for the registered hive agent, or `""` if none
- Table created via `CREATE TABLE IF NOT EXISTS api_keys` in schema init (placed after
  `CREATE TABLE users` to avoid reference errors on fresh DBs — also fixes a schema
  ordering bug: the `UPDATE nodes SET assignee_id` backfill was moved after `CREATE TABLE users`)

### 2. `site/graph/handlers.go` — both hive handlers updated

**`handleHive`**: calls `GetHiveAgentID` first, passes `agentID` to all three store calls:
`ListHiveActivity(ctx, agentID, ...)`, `GetHiveCurrentTask(ctx, agentID)`, `GetHiveTotals(ctx, agentID)`.

**`handleHiveStats`**: same pattern — `GetHiveAgentID` → `GetHiveTotals(ctx, agentID)`.

### 3. `site/graph/hive_test.go` — two new scoping tests

**`TestGetHiveCurrentTask_ScopedToActor`**
- Seeds two open agent tasks with different `AuthorID`s (actor A and B)
- Calls `GetHiveCurrentTask(ctx, "actor-a")` — verifies only actor A's task is returned
- Calls `GetHiveCurrentTask(ctx, "actor-b")` — verifies only actor B's task is returned
- Proves: no cross-agent task bleed

**`TestGetHiveTotals_ScopedToActor`**
- Seeds 2 ops for actor A, 1 op for actor B
- Calls `GetHiveTotals(ctx, "actor-a")` — verifies count = 2
- Calls `GetHiveTotals(ctx, "actor-b")` — verifies count = 1
- Proves: no cross-agent op bleed

## Verification

```
go build ./...   ✓
go test -run "TestGetHive|TestGetHiveCurrentTask|TestGetHiveTotals|TestGetHiveStats" ./graph/   ✓ (6/6 pass)
```

Pre-existing failures in `TestReportsAndResolve` and `TestReposts` (scan type error on `Op`,
nil pointer in reposts) are unrelated to this fix and predate it.

## Files changed

| File | Change |
|------|--------|
| `graph/store.go` | `GetHiveCurrentTask` + `GetHiveTotals` parameterized by actorID; `GetHiveAgentID` added; schema ordering fixed |
| `graph/handlers.go` | Both hive handlers now resolve `agentID` first and pass it to all store calls |
| `graph/hive_test.go` | `+TestGetHiveCurrentTask_ScopedToActor`, `+TestGetHiveTotals_ScopedToActor` |
