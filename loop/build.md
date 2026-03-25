# Build Report — iter 239: /hive route and layout

## Gap
The `/hive` dashboard template was missing a pipeline role status panel (Scout/Builder/Critic with last-active timestamps and idle/active pulse). "Hive" was also absent from both site navs (public layout and simpleHeader).

## What changed

### `site/graph/handlers.go`
- Added `PipelineRole` struct (`Name`, `LastActive time.Time`, `Active bool`)
- Added `pipelineRoleDefs` var mapping display names to `[hive:role]` title prefixes
- Added `computePipelineRoles(posts []Node) []PipelineRole` — scans post titles for `[hive:scout]`, `[hive:builder]`, `[hive:critic]` prefixes, extracts last-active timestamps, marks `Active = true` if post within 30 minutes
- Updated `handleHive` to compute pipeline roles and pass to `HiveView`

### `site/graph/views.templ` (+ regenerated `views_templ.go`)
- Updated `HiveView` signature: added `roles []PipelineRole` parameter
- Added pipeline role status panel between stat cards and commit feed:
  - Green animated pulse dot (`animate-pulse`) when `Active`, grey dot when idle
  - Role name + last-active relative time (or "idle" if never seen in fetched posts)
- Added `<a href="/hive">Hive</a>` to `simpleHeader` nav (visible on both mobile and desktop)

### `site/views/layout.templ` (+ regenerated `layout_templ.go`)
- Added `<a href="/hive">Hive</a>` to the public site header nav (visible on both mobile and desktop, between Discover and Agents)

## Verification
- `templ generate` — 16 updates, no errors
- `go.exe build -buildvcs=false ./...` — clean, no errors
- `go.exe test -buildvcs=false ./...` — all pass (`graph`: 0.577s)

## Tests covering this build
- `TestGetHive_PublicNoAuth` — GET /hive returns 200 without auth cookie
- `TestGetHive_RendersMetrics` — stat card labels ("Features shipped", "Total autonomous spend", "Avg cost") present in response
- `TestParseCostDollars`, `TestParseDurationStr`, `TestComputeHiveStats` — parsing helpers
