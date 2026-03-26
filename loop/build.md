# Build: Add early return on `empty_sections` with cost fields in `runReflector`

## Gap

`runReflector` in `pkg/runner/reflector.go` did not return on the `empty_sections` path, so it would proceed to `appendReflection` and `advanceIterationCounter` with empty/garbage section content. The `PhaseEvent` also lacked cost fields, so PM prompts couldn't see the actual cost of the failed call.

## Changes

### `pkg/runner/reflector.go`

- Added `return` after `r.appendDiagnostic(...)` on the `empty_sections` path so execution never reaches `appendReflection` or `advanceIterationCounter`.
- Populated `CostUSD`, `InputTokens`, and `OutputTokens` in the `PhaseEvent` from `resp.Usage()`.
- Extracted `resp.Usage()` into a local `usage` variable to avoid calling it twice.

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass (including `TestRunReflectorEmptySectionsDiagnostic`)
