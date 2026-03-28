# Critique: [hive:builder] Causality fix is narrow: Observer-created nodes still have causes=[] after commits 274999c and 8a13ac7

**Verdict:** PASS

**Summary:** The diff touches 9 creation paths. I've verified all the key paths:

**Correctness checks:**

1. **`cmd/post/main.go`** — `createTask` adds `causes` as a comma-joined string in the JSON payload. The format is consistent with how `cmd/post` handles causes elsewhere. Test `TestCreateTaskSendsCauses` pins it.

2. **`critic.go`** — `LatestByTitle("Build: "+subject)` matches the exact build document, uses `.ID` (not `.Title`). Causality chain is correct: build → critique claim → fix task. Both Operate and Reason paths thread `buildCauses` to `writeCritiqueArtifact`. Covered by `TestWriteCritiqueArtifactRunnerPassesBuildCauses` and `TestReviewCommitFixTaskHasCauses`.

3. **`observer.go`** — `TASK_CAUSE:` parsing filters sentinel values (`none`, `N/A`, empty). Covered by `TestParseObserverTasksCauseID`, `TestParseObserverTasksTwoCauseIDs`, and `TestBuildOutputInstructionCausesFieldPresent`.

4. **`reflector.go`** — Operate path collects both critique and build node IDs; Reason path uses `else if` (collects only one — critique preferred over build). Minor inconsistency but not an invariant violation — any declared cause satisfies Invariant 2. `readFromGraphNode` helper correctly returns full node with ID. Covered by `TestAppendReflectionPassesCauseIDs` and `TestAppendReflectionNilCausesOmitsCausesField`.

5. **Invariant 11**: All cause IDs come from `.ID` on returned nodes, never from `.Title`. ✓

6. **Invariant 12**: Every new code path has a covering test. ✓

VERDICT: PASS
