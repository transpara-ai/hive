# Build: Fix: test task

- **Commit:** 9d00000343094caef010f69234f771e50199fc49
- **Subject:** [hive:builder] test task
- **Cost:** $1.6306
- **Timestamp:** 2026-04-22T14:24:48Z

## Task

Critic review of commit 450678dad5c5 found issues:

Fix task creation returned `unauthorized` (no API key in this environment — expected). The verdict stands regardless.

## What Was Built

All tests pass. Here's what was fixed:

**Root cause (two paths):**
1. The LLM in Operate mode has Bash/curl tool access. "The pipeline will create the fix task automatically" wasn't a constraint — the LLM still tried curl and got 401, reporting it in critique output.
2. `reviewCommit()` called `r.cfg.APIClient.CreateTask()` unconditionally in the REVISE path, also getting 401 when no key was configured.

**Changes in `pkg/runner/critic.go`:**
- Moved `apiKey` to function scope so both paths share one check
- Added explicit prohibition to the no-key instruction: "Do NOT attempt to create a task via curl, Bash, or any other tool..."
- Added guard before `CreateTask`: skips with a log message when `apiKey == ""`

**Changes in `pkg/runner/critic_test.go`:**
- `TestBuildCriticInstruction_EmptyAPIKey`: added assertion for the explicit prohibition
- `TestReviewCommitFixTaskHasCauses`: added `t.Setenv("LOVYOU_API_KEY", "test-key")` so the guard doesn't block the with-key test path
- `TestReviewCommit_NoAPIKey_SkipsCreateTask` (new): verifies no `CreateTask` call when key is absent

Build clean, all 33 packages pass.

ACTION: DONE

## Diff Stat

```
commit 9d00000343094caef010f69234f771e50199fc49
Author: ai-agent <ai-agent@transpara.com>
Date:   Wed Apr 22 14:24:48 2026 +0000

    [hive:builder] test task

 loop/budget-20260422.txt  |   4 ++
 loop/build.md             |  49 ++++++-------
 loop/critique.md          |  22 ++----
 loop/test-report.md       | 177 ++++++++++++++++++++++++++--------------------
 pkg/runner/critic.go      |  12 +++-
 pkg/runner/critic_test.go |  80 +++++++++++++++++++++
 6 files changed, 219 insertions(+), 125 deletions(-)
```
