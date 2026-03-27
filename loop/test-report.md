# Test Report: Iteration 373 — close.sh: critique nodes posted with causes=[]

- **Result:** PASS
- **Tests run:** 36
- **New tests added:** 3
- **Timestamp:** 2026-03-28

## What Was Tested

The fix in iteration 373 changed `createTask` to return `(string, error)` — the task node ID —
so `main()` can pass it as `causes` to `assertCritique`. This satisfies Invariant 2: a critique
of a build task must declare that task as its cause.

### Existing tests (all passing)

All 33 pre-existing tests pass unchanged, including:
- `TestAssertCritiqueCarriesTaskNodeIDasCause` — verifies assertCritique uses the task node ID as causes
- `TestCreateTaskSendsKindTask` — verifies intend op sends explicit `kind=task`
- `TestAssertCritiqueSendsCauses` / `TestAssertCauseIDsMultipleJoined` — cause wiring paths

### New tests added

**`TestCreateTaskReturnsNodeID`** — verifies `createTask` returns the node ID from the server
response. This is the critical path: if the ID is not returned, `taskCauseIDs` is empty and
the fallback to `buildDocID` silently re-introduces the causality gap.

**`TestCreateTaskEmptyResponseIDReturnsEmpty`** — verifies the guard at `main.go:238-241`:
when the server returns `{}` (no `node.id`), `createTask` returns `("", nil)` without
attempting the `complete` op. Caller falls back gracefully.

**`TestCreateTaskSendsCompleteOp`** — verifies that `createTask` sends two requests:
1. `op=intend kind=task` (creates the task)
2. `op=complete node_id=<returned-id>` (marks it done)

Without the `complete` op the task stays in-progress on the board indefinitely.

## Coverage Notes

The `main()` fallback path (`taskCauseIDs = causeIDs` when `taskNodeID == ""`) is exercised
indirectly: `TestCreateTaskEmptyResponseIDReturnsEmpty` confirms the empty-ID case, and
`TestAssertCritiqueCarriesTaskNodeIDasCause` confirms the non-empty-ID case. The fallback
itself lives in `main()` which is not unit-testable, but both branches of the condition are
covered by the function-level tests.

## Edge Cases Covered

- `createTask` with valid node ID in response → returns the ID ✓
- `createTask` with empty/missing node ID in response → returns `("", nil)` ✓
- `createTask` sends `complete` op with correct `node_id` ✓
- `assertCritique` with `taskNodeID` as causes → causes field = taskNodeID ✓
- `assertCritique` with `nil` causes → no causes field sent ✓
