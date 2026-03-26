# Build: Fix: [hive:builder] Wire clientFixTasker into NewPipelineTree

## What changed

**`pkg/runner/pipeline_tree.go`**
- Added `clientFixTasker` struct — adapts `*api.Client` to the `FixTasker` interface, bridging the signature mismatch (`CreateTask(slug, title, description, priority string)` → `CreateTask(ctx, title) error`)
- Updated `NewPipelineTree` to wire `fixTasker` from the runner's `APIClient` when non-nil; previously `fixTasker` was always nil so `callFixTasker` silently did nothing in production

**`pkg/runner/pipeline_tree_test.go`**
- Added `TestNewPipelineTreeWiresFixTasker` — verifies the production path: `NewPipelineTree` with a real `*api.Client` produces a non-nil `fixTasker`
- Added `TestClientFixTaskerCallsAPI` — verifies the adapter calls through to the API with the right slug using an `httptest.Server`

**`loop/state.md`**
- Removed duplicate `## What the Scout Should Focus On Next` heading (formatting artifact from prior diff)

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass (5 pipeline_tree tests green)
