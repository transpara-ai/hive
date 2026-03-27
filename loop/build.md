# Build: Add REVISE Gate Before Reflector in Pipeline

**Iteration:** 339
**Date:** 2026-03-27

## What Changed

### `pkg/runner/pipeline_tree.go`
- Added `"log"` to imports
- Added REVISE gate in the reflector phase: reads `loop/critique.md` via `readLoopArtifact`, calls `parseVerdict`, and returns `nil` early (skipping `runReflector`) when verdict is `"REVISE"`. Logs `[pipeline] skipping reflector — critic verdict is REVISE`.
- Gate is a no-op when `HiveDir` is empty or `critique.md` doesn't exist (`readLoopArtifact` returns `""`, `parseVerdict` defaults to `"PASS"`).

### `pkg/runner/pipeline_tree_test.go`
- Added `TestPipelineTreeReflectorSkippedOnRevise`: creates a hiveDir with `critique.md` containing `VERDICT: REVISE`, extracts the real reflector phase from `NewPipelineTree`, and verifies Execute returns nil with no diagnostics. If the gate is missing, `runReflector` is called with a nil Provider and panics — which the test runner catches as a failure.

### `pkg/runner/architect_test.go`
- Added `bold-colon format: **SUBTASK_TITLE:** Title here` case to `TestParseArchitectSubtasks`. This is the exact format that caused the 06:08:12Z architect failure (the normalizer fix in c600069 handles it; this test pins the regression).

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass
  - `TestPipelineTreeReflectorSkippedOnRevise` — PASS (gate fires, reflector skipped)
  - `TestParseArchitectSubtasks/bold-colon_format:_**SUBTASK_TITLE:**_Title_here` — PASS

## Root Cause Addressed

8 of 11 recent pipeline failures were `reflector outcome=empty_sections`. Root cause: `pipeline_tree.go` called `runReflector` unconditionally, even when Critic said REVISE. The Reflector LLM correctly refused to produce sections, but the pipeline treated empty output as failure, burned $0.04–$0.11 per false failure, and aborted. This fix gates on the critique verdict before calling the reflector.

ACTION: DONE
