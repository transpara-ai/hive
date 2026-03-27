# Build: site/handlers/hive.go: standalone hive dashboard handlers

## Task

Create `site/handlers/hive.go` with two standalone `http.HandlerFunc`-compatible functions:
- `HiveDashboard(w, r)` — renders full hive dashboard page
- `HiveFeed(w, r)` — returns JSON of last 10 phase history entries

## What Was Built

**New files:**
- `site/handlers/hive.go` — new `handlers` package with:
  - `DiagEntry{Phase,Outcome,Cost,Timestamp string}` — all-string type for JSON marshaling
  - `HiveDashboardData{Iteration,Phase,LastBuildTitle,BuildCost,PhaseHistory,RecentCommits}` — collected dashboard state
  - `readHiveState(loopDir)` — parses `Iteration:` and `Phase:` from `state.md`
  - `readHiveBuild(loopDir)` — extracts first H1 title and `$X.XX` cost from `build.md`
  - `readHiveDiagnostics(loopDir, limit)` — reads last N lines from `diagnostics.jsonl`, newest-first, malformed lines skipped
  - `readHiveCommits(repoDir)` — runs `git log --oneline -10` (bounded constant, not user input)
  - `buildHiveDashboardData()` — collects all data using `HIVE_REPO_PATH` env var or `../hive` sibling default
  - `HiveDashboard` — builds data, converts to `graph.*` types, renders `graph.HivePage`
  - `HiveFeed` — builds data, caps at `maxFeedEntries=10`, returns JSON with `Content-Type: application/json`

- `site/handlers/hive_test.go` — 14 tests covering:
  - `readHiveState`: happy path, missing file
  - `readHiveBuild`: happy path, missing file
  - `readHiveDiagnostics`: empty file (nil), malformed line skip, limit enforcement, cost formatting, zero cost, empty dir
  - `HiveFeed`: JSON response, empty dir (no 500), max entries cap
  - `HiveDashboard`: returns 200 with loop files, returns 200 with no files (no 500s)

**No existing files modified.**

## Build verification

```
go.exe build -buildvcs=false ./...   → exit 0
go.exe test -buildvcs=false ./...    → all pass (auth, graph, handlers)
```

## Addresses

- Critic issue 4: `loopDir` production default — now uses `HIVE_REPO_PATH` env var; no empty-string default in handlers
- Critic issue 3: git subprocess uses `maxCommits=10` constant, not user-controlled input
- Critic issue 5: all new helpers have tests
