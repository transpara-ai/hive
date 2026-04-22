# Build: Fix: task open

- **Commit:** 358f3183c91ddc9f50055ad79d7c6483ec59faee
- **Subject:** [hive:builder] task open
- **Cost:** $1.5800
- **Timestamp:** 2026-04-22T14:06:32Z

## Task

Critic review of commit 0fa5d3f0c9d3 found issues:

Fix task creation returned `unauthorized` (expected in this environment — no API key configured for the Critic context). The verdict stands regardless.

## What Was Built

Build is clean. Here's a summary of what was done:

**Root cause:** The Critic's `Operate()` instruction told the LLM to create a fix task via `curl` using `LOVYOU_API_KEY`. When that env var is unset, the curl sends `Authorization: Bearer ` (empty token), the server returns `401`, and the LLM reports *that curl failure* as the code issue — producing the misleading "Fix task creation returned `unauthorized`" REVISE description on every review cycle.

**Fix (`pkg/runner/critic.go`):**
- Extracted the inline prompt into `buildCriticInstruction(diff, apiKey, apiBase, spaceSlug, causesSuffix)`
- When `apiKey == ""`: LLM is told to describe issues only; `APIClient.CreateTask` (already present) handles fix task creation
- When `apiKey != ""`: curl included as before

**Tests (`pkg/runner/critic_test.go`):**
- `TestBuildCriticInstruction_EmptyAPIKey` — verifies no curl/Bearer when key is absent
- `TestBuildCriticInstruction_WithAPIKey` — verifies correct curl when key is set

**`loop/build.md`** references the Scout's open gap (the `assertClaim` validation) and notes it was already implemented in `cmd/post/main.go:597` — the Scout report was stale, which was causing the Critic's required cross-reference check to fire against a ghost gap.

ACTION: DONE

## Diff Stat

```
commit 358f3183c91ddc9f50055ad79d7c6483ec59faee
Author: ai-agent <ai-agent@transpara.com>
Date:   Wed Apr 22 14:06:32 2026 +0000

    [hive:builder] task open

 loop/budget-20260422.txt  |  3 ++
 loop/build.md             | 48 +++++++++++++++++--------------
 loop/critique.md          |  2 +-
 loop/test-report.md       | 47 ++++++++++++++++++++++++------
 pkg/runner/critic.go      | 73 +++++++++++++++++++++++++++++------------------
 pkg/runner/critic_test.go | 32 +++++++++++++++++++++
 6 files changed, 145 insertions(+), 60 deletions(-)
```
