# Build: Fix: tests for buildPart2Instruction / buildOutputInstruction

- **Commit:** pending
- **Subject:** [hive:builder] Fix: add tests for buildPart2Instruction and buildOutputInstruction
- **Timestamp:** 2026-03-27

## Task

Critic review of commit 476874249de2 found that the observer refactor introduced `buildPart2Instruction` and `buildOutputInstruction` with no test coverage. Fix task: `a6fea8e36c1b51aeab693448e97bf6e2`.

## What Was Built

Created `pkg/runner/observer_test.go` with 4 tests:

1. **`TestBuildPart2Instruction_NoAPIKey`** — verifies skip message is returned and no curl auth command is emitted when apiKey is empty.
2. **`TestBuildPart2Instruction_WithAPIKey`** — verifies API key and space slug appear in output and curl auth command is present.
3. **`TestBuildOutputInstruction_NoAPIKey`** — verifies text task format (TASK_TITLE:) is returned and no curl auth command when apiKey is empty.
4. **`TestBuildOutputInstruction_WithAPIKey`** — verifies curl POST with API key and space slug is returned, no text task format.

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass

ACTION: DONE
