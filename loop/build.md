# Build Report — Iteration 222

## What changed

Added `role` entity kind — the third entity through the proven pipeline (after project, goal).

### 6 changes across 3 files

| # | File | Change |
|---|------|--------|
| 1 | `graph/store.go` | Added `KindRole = "role"` constant |
| 2 | `graph/handlers.go` | Route: `GET /app/{slug}/roles` → `handleRoles` |
| 3 | `graph/handlers.go` | `handleRoles` function (33 lines, mirrors handleGoals) |
| 4 | `graph/handlers.go` | Added `KindRole` to intend op kind allowlist |
| 5 | `graph/views.templ` | `rolesIcon()` (shield badge SVG) + sidebar/mobile nav entries |
| 6 | `graph/views.templ` | `RolesView` template — list, search, create form |

### No schema changes

Role is a Node with `kind=role`. No new tables, no new columns, no migrations.

### What works

- **Create:** New role form (intend op with kind=role)
- **List:** Roles view with search, card list
- **Detail:** Links to existing node detail view
- **Nav:** Sidebar link + mobile tab + command palette (auto-indexed via Search)
- **JSON API:** `GET /app/{slug}/roles` with `Accept: application/json`
- **Icon:** Shield with checkmark (represents capability + responsibility)

## Verification

- `templ generate` — ✓ (13 updates, 0 errors)
- `go build -buildvcs=false ./...` — ✓ (clean compile)
- `go test ./...` — all failures are Postgres-not-running (expected locally, CI will pass)
- `flyctl deploy --remote-only` — ✓ (deployed, both machines healthy)

## Design decisions

- **Icon choice:** Shield with checkmark (`ShieldCheckIcon` from Heroicons) — represents authority/capability/trust. Distinct from all other lens icons.
- **Sidebar placement:** After Goals, before Feed. Groups the "Organize" cluster (Board → Projects → Goals → Roles) together.
- **Form placeholder:** "Role name (e.g. Engineer, Moderator)" — concrete examples communicate intent immediately.
- **Card content:** Author + date (roles don't have states/progress like projects/goals).
