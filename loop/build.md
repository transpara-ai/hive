# Build: Fix — Verify GET /hive route and handler (iteration 337 correction)

## Task

The previous build.md claimed commit `6f7187d` in the hive repo added `GET /hive`. That commit only contained loop files. The Critic flagged this as a subject-content mismatch. This build corrects the record.

## Findings

The `GET /hive` route and handler **already exist** in the site repo — added in iterations 239–240. No new code was required.

**Site repo (`/c/src/matt/lovyou3/site`) — verified present:**

| File | What |
|------|------|
| `graph/handlers.go:130` | `mux.HandleFunc("GET /hive", h.handleHive)` |
| `graph/handlers.go:131` | `mux.HandleFunc("GET /hive/stats", h.handleHiveStats)` |
| `graph/handlers.go:132` | `mux.HandleFunc("GET /hive/status", h.handleHiveStatus)` |
| `graph/handlers.go:3661` | `handleHive` — fetches agent posts, stats, roles, tasks, renders `HiveView` |
| `graph/handlers.go:3688` | `handleHiveStatus` — HTMX partial, re-fetches and renders `#hive-content` |
| `graph/handlers.go:3713` | `handleHiveStats` — HTMX partial for live stats bar |
| `graph/handlers.go:3548` | `parseCostDollars`, `parseDurationStr` — cost/duration extraction helpers |
| `graph/handlers.go:3571` | `computeHiveStats` — aggregates Features/TotalCost/AvgCost |
| `graph/handlers.go:3608` | `computePipelineRoles` — Scout/Builder/Critic/Reflector active state |
| `graph/store.go:2265` | `GetHiveCurrentTask(ctx, actorID)` — open task for given agent |
| `graph/store.go:2307` | `GetHiveTotals(ctx, actorID)` — op count + last active for agent |
| `graph/store.go:2332` | `GetHiveAgentID(ctx)` — resolves agent actor ID from api_keys |
| `graph/hive_test.go` | `TestGetHive_PublicNoAuth`, `TestGetHive_RendersMetrics`, `TestGetHive_RendersCurrentlyBuilding`, `TestGetHiveCurrentTask_ScopedToActor`, `TestGetHiveTotals_ScopedToActor`, `TestGetHiveAgentID_IntegrationPath`, `TestGetHiveStatus_Partial`, `TestGetHiveStats_Partial` |

## Verification

```
cd /c/src/matt/lovyou3/site
go.exe build -buildvcs=false ./...   → ok
go.exe test ./...                    → ok (unit tests pass; DB tests skip without DATABASE_URL)

cd /c/src/matt/lovyou3/hive
go.exe test ./...                    → ok
```

## Root cause of loop confusion

The prior build.md referenced `bb6f804` (a hive repo commit that contained only loop files, not site code) as evidence that `site/templates/hive.templ` was created. The site uses `graph/views.templ`, not `site/templates/`. The route and handler have been in the site repo since iter 239–240. The loop attempted to re-add work that was already done, produced no code, and mislabeled the loop-file commit as the builder commit.

## What changed this iteration

- `loop/build.md` — corrected to accurately describe the existing implementation
- No code changes — the implementation was already complete and correct
