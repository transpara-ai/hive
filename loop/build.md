# Build: task open (fix)

- **Branch:** feat/20260422-task-open
- **Timestamp:** 2026-04-22

## What Was Fixed

The previous builder's commit (`9e33ff3`) only touched `budget.txt` — the `TestOpenNode` store test was written but never committed. The implementation itself was already correct in `5bef18f`.

This iteration confirms and commits the missing test:

- **`pkg/localapi/store_test.go`**: Added `TestOpenNode` — creates a task, completes it, reopens it via `store.OpenNode()`, and asserts state transitions open→done→open.

## Verification

```
go build -buildvcs=false ./...   → PASS (no errors)
go test -buildvcs=false ./...    → PASS (all packages)
pkg/localapi: 15/15 tests pass
  TestOpenNode                   ✅ new
  TestRoundTrip_OpenTask         ✅ existing
  TestRoundTrip_CreateAndListTasks ✅
  TestRoundTrip_CompleteTask     ✅
  TestRoundTrip_NodeExists       ✅
  TestHealth                     ✅
  TestUnauthorized               ✅
  TestUnknownOp                  ✅
  + 7 store unit tests           ✅
```

No regressions across the full suite.

ACTION: DONE
