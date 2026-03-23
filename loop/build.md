# Build Report — Iteration 125

## Dashboard task filtering — state tabs with query params

### Changes

**store.go:** `ListUserTasks` now accepts `stateFilter` param: "" (open), "active", "review", "done", "all". Switch-based SQL generation. Limit increased to 20.

**handlers.go:** Reads `?tasks=` query param, passes to store and template.

**views.templ:**
- Dashboard signature extended with `taskFilter string`
- Added filter tabs: Open / Active / Review / Done / All
- `dashboardTaskTab` component with brand highlight for active tab
- Context-aware empty state messages

**store_test.go:** Updated `ListUserTasks` call to include state filter param.

### Deployed
`ship.sh` — all green.
