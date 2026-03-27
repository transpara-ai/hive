# Build Report

**Task:** Fix: PipelineStateMachine — swallowed error, dead init, missing tests
**Status:** DONE
**Cost:** ~$0.00 (no LLM calls)

## Changes

### `cmd/hive/main.go`

**Issue 1 — swallowed error in `makeRunner`:**
Changed `makeRunner` from `func(role string) *runner.Runner` to `func(role string) (*runner.Runner, error)`. Restored `return nil, fmt.Errorf("provider for %s: %w", role, err)` on `intelligence.New` failure. Updated the single call site to propagate the error.

**Issue 2 — dead first `sm` creation:**
Removed the dead `sm := runner.NewPipelineStateMachine(makeRunner("builder"))` at line 286 that was immediately overwritten. The live creation now uses `:=` directly: `smRunner, err := makeRunner("builder")` → `sm := runner.NewPipelineStateMachine(smRunner)`.

### `pkg/runner/pipeline_state_test.go` (new file)

Added four tests covering the untested `PipelineStateMachine`:

- **`TestPipelineTransitionValid`** — table-driven; exercises all 13 valid (state, event) → (next state, agent) transitions.
- **`TestPipelineTransitionInvalid`** — verifies that an event invalid for the current state returns an error and leaves state unchanged.
- **`TestRunBoardClearStartsAtDirecting`** — mock HTTP server returns empty board; cancelled context stops the loop immediately; asserts state is `StateDirecting`.
- **`TestRunExistingTasksStartsAtBuilding`** — mock server returns one open task; cancelled context stops the loop immediately; asserts state is `StateBuilding`.

## Verification

```
go.exe build -buildvcs=false ./...   ✓ no errors
go.exe test ./...                    ✓ all pass
```
