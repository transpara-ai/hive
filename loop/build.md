# Build: Fix: test task

- **Commit:** (pending)
- **Subject:** [hive:builder] fix: skip fix task creation when no API key
- **Cost:** (pending)
- **Timestamp:** 2026-04-22T14:24:00Z

## Task

Critic review of commit 450678dad5c5 found issues:

Fix task creation returned `unauthorized` (no API key in this environment — expected). The verdict stands regardless.

## What Was Built

**Root cause:** Two related failures caused the Critic to report "Fix task creation returned `unauthorized`":

1. **LLM path:** When `LOVYOU_API_KEY == ""`, the LLM in Operate mode still had tool access (Bash/curl). The instruction said "the pipeline will create the fix task automatically" but didn't prohibit tool-based attempts. The LLM would try curl anyway and get 401, then report it in its output — which ended up in `loop/critique.md`.

2. **Go path:** In `reviewCommit()`, the REVISE branch called `r.cfg.APIClient.CreateTask()` unconditionally. When the APIClient was initialized with an empty key, this always returned 401 Unauthorized.

**Fix (`pkg/runner/critic.go`):**
- Moved `apiKey := os.Getenv("LOVYOU_API_KEY")` to function scope (before the `canOperate` block) so both paths share the same key-presence check
- Updated `buildCriticInstruction()` when `apiKey == ""` to explicitly prohibit tool-based task creation: "Do NOT attempt to create a task via curl, Bash, or any other tool — there is no API key in this environment and any such call will return 401 Unauthorized"
- Added guard in the REVISE switch case: `if apiKey == "" { log.Printf(...skip...); return }` — prevents the 401 error when no API key is configured

**Tests (`pkg/runner/critic_test.go`):**
- `TestBuildCriticInstruction_EmptyAPIKey` — updated to verify the explicit `"Do NOT attempt to create a task via curl"` prohibition is present in the instruction
- `TestReviewCommitFixTaskHasCauses` — added `t.Setenv("LOVYOU_API_KEY", "test-key")` so the apiKey guard doesn't skip task creation in the with-key test path
- `TestReviewCommit_NoAPIKey_SkipsCreateTask` — new test: verifies REVISE verdict does NOT call CreateTask when LOVYOU_API_KEY is empty

**Scout gap cross-reference:** `loop/scout.md` describes the `assertClaim` validation gap in `cmd/post/main.go`. Per `loop/state.md` (DONE item 8), this was already implemented in iter 408. The current fix addresses the Critic's task-creation loop — a separate infrastructure gap identified by this iteration.

ACTION: DONE

## Diff Stat

```
pkg/runner/critic.go      | 10 +++++++---
pkg/runner/critic_test.go | 70 ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
2 files changed, 77 insertions(+), 3 deletions(-)
```
