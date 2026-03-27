# site/templates/hive.templ + hive-feed.templ: dashboard UI with HTMX polling

## What changed

### New files
- `site/graph/hive.templ` — `HivePage` template: full page with ember minimalism dark theme
- `site/graph/hive_feed.templ` — `HiveDiagFeed` template: standalone partial for phase timeline

### Modified files
- `site/graph/handlers.go`
  - Added `os/exec` import
  - Extended `LoopState` with `BuildCost float64`
  - Updated `readLoopState` to parse cost from `build.md` via `parseCostDollars`
  - Added `DiagEntry` type and `readDiagnostics()` — reads `diagnostics.jsonl`, returns last N entries newest-first
  - Added `RecentCommit` type and `readRecentCommits()` — runs `git log --oneline -N` in repo dir
  - Added `hivePhaseClass()` — Tailwind badge classes per phase (amber/indigo/emerald/orange/violet)
  - Added `diagOutcomeIcon()` — ✓/↻/✗/○ symbols per outcome
  - Added `diagOutcomeColor()` — text color per outcome
  - Added `maxHiveDiagEntries = 10` constant
  - `handleHive`: now renders `HivePage` (loop state + diagnostics + commits)
  - `handleHiveFeed`: now renders `HiveDiagFeed` partial (diagnostics only, no HTML shell)
- `site/graph/views.templ` — removed unused `HiveView` and `HiveFeedView`
- `site/graph/hive_test.go` — updated `TestGetHiveFeed_PublicNoAuth` to check for `hive-feed` partial element

## What it shows

**`/hive`** (HivePage):
1. Large iteration counter (text-7xl, rose/brand accent)
2. Current phase pill (Scout=amber, Architect=indigo, Builder=emerald, Critic=orange, Reflector=violet)
3. Last build title + cost (from build.md)
4. Phase timeline — id=hive-feed, polls /hive/feed every 5s, shows DiagEntry rows
5. Recent commits — git log --oneline -10 from hive repo dir

**`/hive/feed`** (HiveDiagFeed):
- Standalone partial, no HTML wrapper
- Each row: outcome icon + phase pill + outcome label + cost + relative timestamp

## Verification

- templ generate: 16 updates, no errors
- go build -buildvcs=false ./...: success
- go test ./...: all pass
