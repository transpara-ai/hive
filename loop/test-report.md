# Test Report: Causality fix is narrow — iteration 374

**Timestamp:** 2026-03-28

## What Was Tested

The build added `causes` fields across 9 creation call sites (Observer, PM, Critic, Reflector). The existing test `TestCreateTaskSendsCauses` covered only the `cmd/post` path. This report covers the 10 new tests added to pin the remaining causality invariants.

## New Tests

### `pkg/runner/observer_test.go`

**`TestParseObserverTasksCauseID`** (6 sub-cases)
- Tests `TASK_CAUSE:` parsing in `parseObserverTasks`
- Valid node ID → `causeID` set
- Sentinel values (`none`, `N/A`, empty string) → `causeID` empty
- Whitespace trimmed from ID
- Missing `TASK_CAUSE:` line → `causeID` empty

**`TestParseObserverTasksTwoCauseIDs`**
- Two tasks in one LLM response, each with a different `TASK_CAUSE:` → each `causeID` correctly isolated to its task

**`TestBuildOutputInstructionCausesFieldPresent`**
- Verifies `"causes"` field appears in the curl template when API key is set
- Without this, the Observer's Operate path never declares causes (Invariant 2)

**`TestBuildOutputInstructionNoCausesWhenNoKey`**
- No-key fallback (text-only output) must NOT contain `"causes"`

### `pkg/runner/reflector_test.go`

**`TestAppendReflectionPassesCauseIDs`**
- Mock HTTP server records `CreateDocument` request
- Verifies both cause IDs are forwarded in the `causes` field
- Pins causality threading from critique/build nodes → reflection document

**`TestAppendReflectionNilCausesOmitsCausesField`**
- Nil `causeIDs` → no `causes` field in the request
- Avoids sending empty arrays

**`TestReadFromGraphNodeStalenessFilter`** (3 sub-cases)
- Fresh node (30 min old) → returned
- Stale node (3 hours old) → filtered out (2-hour threshold)
- Nil `APIClient` → returns nil without panic

### `pkg/runner/critic_test.go`

**`TestWriteCritiqueArtifactRunnerPassesBuildCauses`**
- Mock server records the `assert` (claim) request
- Verifies the build document ID appears in `causes`
- Pins: critique claims must declare the build they review

## Results

```
All 13 packages: PASS
```

## Coverage Notes

**Covered:**
- `parseObserverTasks` TASK_CAUSE parsing (all branches: valid, none, N/A, empty, missing)
- `buildOutputInstruction` causes field in curl template
- `appendReflection` → `CreateDocument` causeIDs threading
- `readFromGraphNode` staleness filter (fresh/stale/nil)
- `writeCritiqueArtifact` (Runner method) → `AssertClaim` causeIDs threading
- `createTask` in cmd/post sends causes (pre-existing, from Builder)

**Not covered (Operate paths):**
- PM/Critic/Reflector Operate: curl template cause substitution — these are instruction strings passed to an LLM, not testable logic paths
