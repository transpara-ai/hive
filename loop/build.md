# Build: Fix: task open

- **Branch:** feat/20260422-task-open
- **Build date:** 2026-04-22

## Scout Gap Reference

**Scout gap (loop/scout.md, Iteration 406):** Missing typed `assertClaim` guard in `hive/cmd/post` — empty causeIDs reach the graph unvalidated (Lesson 167, CAUSALITY GATE 1).

**Status:** Already implemented. `assertClaim` in `cmd/post/main.go:597` validates `len(causeIDs) == 0` and returns an error before any HTTP call. `TestAssertClaim_RejectsEmptyCauseIDs` (cmd/post/main_test.go:2265) covers both nil and empty-slice cases. Scout report is stale.

## Task

Critic review of commit 0fa5d3f0c9d3 found issues: "Fix task creation returned `unauthorized` (expected in this environment — no API key configured for the Critic context). The verdict stands regardless."

**Root cause:** When `LOVYOU_API_KEY` is unset, the Critic's `Operate()` instruction told the LLM to create a fix task via curl with an empty Bearer token. The curl returned `unauthorized`. The LLM reported this failure as the issue text, producing a misleading REVISE description on every cycle. The Go code (`APIClient.CreateTask`) already handles fix task creation independently of the curl — the curl was redundant and harmful when the key is absent.

## What Was Built

**`pkg/runner/critic.go`**

- Extracted inline Operate-mode instruction into `buildCriticInstruction(diff, apiKey, apiBase, spaceSlug, causesSuffix string) string`
- When `apiKey == ""`: instruction tells LLM to list issues only; pipeline creates the fix task automatically via `APIClient.CreateTask`
- When `apiKey != ""`: instruction includes the curl command as before
- Follows the same pattern as `observer.go:buildPart2Instruction` and `observer.go:buildOutputInstruction`

**`pkg/runner/critic_test.go`**

- `TestBuildCriticInstruction_EmptyAPIKey`: verifies no curl/Bearer token in instruction when key is absent; verifies pipeline-fallback message present
- `TestBuildCriticInstruction_WithAPIKey`: verifies curl with correct Bearer token when key is set

## Verification

```
go build -buildvcs=false ./...    → ok
go test -buildvcs=false ./...     → all pass (pkg/runner: 2 new tests)
```
