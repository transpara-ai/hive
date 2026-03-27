# Critique: [hive:builder] close.sh: critique nodes posted with causes=[] � Invariant 2 violation on every iteration

**Verdict:** PASS

**Summary:** **Derivation trace:**

- **Gap**: Critique claim nodes were posted with `causes=[]` — Invariant 2 violated on every iteration because `createTask` returned `error` (no ID) and `assertCritique` received `causeIDs` (build doc), not the task node ID.
- **Fix**: `createTask` now returns `(string, error)` — the task node ID. `main()` threads it into `taskCauseIDs` for `assertCritique`. Fallback to build doc ID when task creation fails (non-fatal path).
- **Test**: `TestAssertCritiqueCarriesTaskNodeIDasCause` sends `[]string{"task-node-abc123"}` to `assertCritique`, captures the HTTP body, and asserts `received["causes"] == "task-node-abc123"`. Correct.
- **Existing test updated**: `TestCreateTaskSendsKindTask` updated to `_, err := createTask(...)` — compiles, no regression.

**Invariant checks:**
- **Invariant 2 (CAUSALITY)**: Every critique claim now cites a declared cause — the task node ID. When task creation fails, falls back to build doc ID (still a cause, not empty). ✓
- **Invariant 11 (IDENTITY)**: Cause is a node ID (`taskNodeID = result.Node.ID`), never a name. ✓
- **Invariant 12 (VERIFIED)**: Test exists and exercises the specific new behavior. ✓

**One edge case confirmed correct**: When `createTask` returns `("", err)`, `taskNodeID == ""` triggers the fallback. The nil-check on `taskNodeID` is the right guard, not checking `err` (which is already consumed for logging above).

The code is minimal, correct, and the test pins the invariant.

VERDICT: PASS
