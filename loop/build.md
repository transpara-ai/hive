# Build: Fix: Architect Operate path does not thread causes

## Task

`buildArchitectOperateInstruction` did not accept a milestone ID and the curl template it provided to the LLM omitted the `causes` field. In production (`claude-cli` implements `IOperator`), `canOperate=true` and the fallback `Reason()` path is never reached — so subtasks created in production had no causality link.

## What Was Built

This fix was already implemented in commit `274999c` (the immediately prior commit). Verification confirms the implementation is complete and correct.

**Fix** (`pkg/runner/architect.go`):
- `buildArchitectOperateInstruction` accepts `milestoneID string` as 3rd parameter
- When non-empty, injects `,"causes":["<milestoneID>"]` into the curl payload template
- Also adds explicit LLM instruction: "set causes on every task"
- Call site (`runArchitect`) extracts `milestone.ID` and passes it through

**Tests** (`pkg/runner/architect_test.go`):
- `mockCaptureOperator` — implements `decision.IOperator`, captures `OperateTask.Instruction`
- `TestRunArchitectOperateInstructionIncludesCauses` — asserts instruction contains `"causes":["milestone-42"]` when Operate path is taken
- `TestRunArchitectSubtasksHaveCauses` — asserts subtasks created via Reason() fallback path also carry `causes:[milestoneID]`

## Verification

```
go.exe build -buildvcs=false ./...  → OK (no errors)
go.exe test ./...                   → all pass
```

Both new tests pass:
- `TestRunArchitectSubtasksHaveCauses` PASS
- `TestRunArchitectOperateInstructionIncludesCauses` PASS

ACTION: DONE
