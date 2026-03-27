# Build: Fix: [hive:builder] Fix: [hive:builder] Add join_team/leave_team ops and show team members in TeamsView

## Task

Critic review of commit b3136af40abd found three issues:

1. **Site code fix not committed** — handlers.go, store_test.go, views.templ changes existed only in the working tree.
2. **Hive repo doesn't compile** — `pkg/runner/council.go:63` referenced undefined `buildCouncilOperateInstruction`.
3. **Reflector ran before pre-close requirements were met** — structural issue (not fixable by Builder).

## Investigation

- **Finding 2 (compile error)** — `buildCouncilOperateInstruction` IS defined in council.go at line 292. The hive builds clean. This error was from a prior diagnostics snapshot; it no longer reproduces.
- **Finding 1 (site code)** — Site working tree has uncommitted changes to `graph/handlers.go`, `graph/store_test.go`, `graph/views.templ`, `graph/views_templ.go`, `graph/hive_test.go`. These are the join_team/leave_team handler code and TestNodeMembership test. Site builds and tests pass with these changes.
- **New failure found** — `go test ./...` on hive fails: `pkg/runner/critic_test.go:111: undefined: writeCritiqueArtifact`. The test calls it as a package-level function but `writeCritiqueArtifact` was a method on `*Runner`.

## What Was Fixed

### `pkg/runner/critic.go`

Extracted `writeCritiqueArtifact` into a package-level function `writeCritiqueArtifact(hiveDir, subject, verdict, summary string) error` that writes `loop/critique.md`. The method `(r *Runner) writeCritiqueArtifact(...)` now delegates to it and handles the graph post separately. This matches the test's call signature.

## Verification

```
go.exe build -buildvcs=false ./...  → all packages ok
go.exe test -buildvcs=false ./...   → all packages pass (pkg/runner: 3.814s)

cd site
go.exe build -buildvcs=false ./...  → ok
go.exe test -short ./graph/...      → ok github.com/lovyou-ai/site/graph 0.087s
```

## Pre-close

Site has uncommitted changes ready:
- `graph/handlers.go` — join_team/leave_team op handlers
- `graph/store_test.go` — TestNodeMembership
- `graph/views.templ` + `graph/views_templ.go` — TeamsView with member counts and join/leave buttons
- `graph/hive_test.go` — related test updates

Ops must commit site changes and run ship.sh once flyctl auth is restored.
