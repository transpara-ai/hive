# Build: Wire failure detection into PipelineTree.Execute

## What changed

**`pkg/runner/pipeline_tree.go`**
- `FixTasker` interface with `CreateTask(ctx, title)` — adapted to `api.Client`
- `clientFixTasker` adapter wires the interface to `api.Client.CreateTask(slug, title, "", "high")`
- `fixTasker FixTasker` field on `PipelineTree`
- `NewPipelineTree`: sets `fixTasker` from `r.cfg.APIClient` when non-nil
- `Execute`: snapshots `diagnosticCount` before each phase; on error OR count increase, calls `callFixTasker` then returns error
- Updated stale comment in `NewPipelineTree` — Phase 2 is done, no longer pending

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass (pkg/runner: 1.143s)
  - `TestPipelineTreeFailureWritesDiagnostic`
  - `TestPipelineTreeFixTaskerCalledOnDiagnosticWithNilReturn`
  - `TestNewPipelineTreeWiresFixTasker`
  - `TestClientFixTaskerCallsAPI`
  - `TestPipelineTreeFixTaskerCalledOnDirectError`
