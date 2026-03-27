# Test Report: Fix Architect Operate path causality (Invariant 2)

- **Build:** Fix: [hive:builder] Zero causes links: graph is causally disconnected
- **Commit:** 274999c35fdcbc4a82b4700e79b91d247f2d496c
- **Result:** PASS — all tests green

## What Was Tested

The fix added `milestoneID` to `buildArchitectOperateInstruction` so the curl template the Architect gives to the LLM includes `"causes":["<milestoneID>"]`. Two paths through `runArchitect` create tasks: the Operate path and the Reason/fallback path. Both needed causality wires. The fix covered the Operate path (previously missing); the Reason path was already covered by a prior iteration.

## Tests Run

### New tests (this iteration)

| Test | File | Result |
|------|------|--------|
| `TestRunArchitectOperateInstructionIncludesCauses` | `pkg/runner/architect_test.go` | PASS |
| `TestRunArchitectSubtasksHaveCauses` | `pkg/runner/architect_test.go` | PASS |
| `TestWriteBuildArtifactDocumentCauses` | `pkg/runner/runner_test.go` | PASS |

**TestRunArchitectOperateInstructionIncludesCauses** — Core fix test. Sets up a milestone with ID `milestone-42`, uses `mockCaptureOperator` to intercept the `OperateTask.Instruction`, and asserts the instruction contains `"causes":["milestone-42"]`. Directly verifies the Operate path embeds the causes suffix. Would have failed before the fix (milestoneID was not passed to `buildArchitectOperateInstruction`).

**TestRunArchitectSubtasksHaveCauses** — Integration test of the Reason/fallback path. Uses a real `mockProvider` that returns SUBTASK_ format, captures all HTTP POST bodies to the fake API server, and asserts every `op=intend` request includes `"causes":["milestone-77"]`.

**TestWriteBuildArtifactDocumentCauses** — Verifies `writeBuildArtifact` calls `CreateDocument` with `causes:[task.ID]`, so build documents are causally linked to the task that triggered the build.

### Full suite

```
ok  github.com/lovyou-ai/hive/cmd/mcp-graph       1.355s
ok  github.com/lovyou-ai/hive/cmd/mcp-knowledge   0.711s
ok  github.com/lovyou-ai/hive/cmd/post            1.403s
ok  github.com/lovyou-ai/hive/pkg/api             1.301s
ok  github.com/lovyou-ai/hive/pkg/authority       0.807s
ok  github.com/lovyou-ai/hive/pkg/hive            1.029s
ok  github.com/lovyou-ai/hive/pkg/loop            0.982s
ok  github.com/lovyou-ai/hive/pkg/resources       0.856s
ok  github.com/lovyou-ai/hive/pkg/runner          4.041s
ok  github.com/lovyou-ai/hive/pkg/workspace       0.540s
```

No regressions.

## Edge Cases Considered

- **No milestone present** — `buildArchitectOperateInstruction` receives `milestoneID=""`, `causesSuffix` is empty, no `"causes"` key in the curl payload. Correct: without a milestone parent there is no causal ancestor to declare.
- **Operate path vs Reason path** — two distinct code paths, both tested for causality.
- **mockCaptureOperator vs mockProvider** — Operate path requires `IOperator`; Reason path uses `IProvider`. Tests use appropriate mocks so coverage is unambiguous.

## Coverage Notes

- `buildArchitectOperateInstruction`: the only change is injecting `causesSuffix`; tested directly by `TestRunArchitectOperateInstructionIncludesCauses`.
- `runArchitect` Operate path milestone ID extraction: covered by the same test.
- No untested code paths introduced.
