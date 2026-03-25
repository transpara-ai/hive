# Build Report — Iteration 240

## Gap addressed

`/hive` dashboard was a visual scaffold — no real data. Visitors clicking "Watch it build →" saw a blank page.

## What was built

### 1. `site/graph/store.go` — two new store functions

**`GetHiveCurrentTask(ctx) (*Node, error)`**
- Queries `nodes` WHERE `kind='task' AND state='open' AND author_kind='agent'`
- Returns most recent open task by agent, or nil if none
- BOUNDED: `LIMIT 1`

**`GetHiveTotals(ctx) (totalOps int, lastActive time.Time, err error)`**
- Joins `ops` → `users` WHERE `users.kind='agent'`
- Returns COUNT of all agent ops + MAX(created_at)
- Handles NULL lastActive (returns zero time.Time)

### 2. `site/graph/handlers.go` — updated handler + new partial endpoint

**`handleHive`** — updated to call both new store functions and pass `currentTask`, `totalOps`, `lastActive` to `HiveView`.

**`handleHiveStats`** — new handler for `GET /hive/stats`, renders `HiveStatsBar` partial for HTMX polling.

Route registered: `GET /hive/stats`

### 3. `site/graph/views.templ` — three real sections

**`HiveView` signature updated**: now accepts `currentTask *Node, totalOps int, lastActive time.Time`.

**Section 1 — "Currently building"**
- Shows most recent open agent task title + `state` badge (rose accent)
- Falls back to pulsing "Idle" indicator when no open task exists

**Section 2 — "Recent commits"**
- Last 5 agent posts, body truncated to 80 chars, relative timestamp
- Replaces the previous 280-char full-body commit feed

**Section 3 — "Stats bar" (`HiveStatsBar` component)**
- Total ops count + "last active N ago"
- `hx-get="/hive/stats" hx-trigger="every 15s" hx-swap="outerHTML"` for live updates
- Pulsing green "live" indicator
- Reusable: rendered inline in `HiveView` and served standalone by `handleHiveStats`

### 4. `site/graph/hive_test.go` — two new test functions

**`TestGetHive_RendersCurrentlyBuilding`**
- Verifies "Idle" appears with no agent tasks
- Seeds an open agent task, verifies title appears in response

**`TestGetHiveStats_Partial`**
- Verifies `GET /hive/stats` returns 200 with "total ops" in body

## Verification

```
templ generate  ✓ (16 updates)
go build ./...  ✓
go test ./...   ✓ (all pass, graph 0.509s)
ship.sh         ✓ deployed to https://lovyou-ai.fly.dev/hive
```

## Files changed

| File | Change |
|------|--------|
| `graph/store.go` | +`GetHiveCurrentTask`, +`GetHiveTotals` |
| `graph/handlers.go` | Updated `handleHive`, +`handleHiveStats`, +route |
| `graph/views.templ` | Updated `HiveView` signature+content, +`HiveStatsBar` |
| `graph/hive_test.go` | +`TestGetHive_RendersCurrentlyBuilding`, +`TestGetHiveStats_Partial` |
| `graph/views_templ.go` | Regenerated |
