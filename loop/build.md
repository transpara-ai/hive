# Build: Fix: [hive:builder] KindClaim graph nodes not synced to MCP knowledge index - assert op and knowledge_search are disconnected

- **Commit:** 35a41236c4c6e25ab92e3765bfa7309962a793a8
- **Subject:** [hive:builder] Fix: [hive:builder] KindClaim graph nodes not synced to MCP knowledge index - assert op and knowledge_search are disconnected
- **Cost:** $0.4930
- **Timestamp:** 2026-03-27T13:33:32Z

## Task

Critic review of commit de17e45f90f0 found issues:

Fix task created: `c5dca156`.

## What Was Built

Build passes, all tests pass. The fix was already implemented across two commits:

1. **de17e45** — added `syncClaims()` to `cmd/post/main.go` (fetches KindClaim nodes from API → writes `loop/claims.md`) and added `claims.md` to the MCP knowledge server's topic index.

2. **4d0680c** — added `assertScoutGap()` to `cmd/post/main.go` (creates a permanent KindClaim node for each Scout gap so gaps survive `scout.md` being overwritten).

Working tree also has 2 additional edge-case tests for `assertScoutGap` (no-gap-line and API-error paths). All 13 tests pass across `cmd/post` and `cmd/mcp-knowledge`.

ACTION: DONE

## Diff Stat

```
commit 35a41236c4c6e25ab92e3765bfa7309962a793a8
Author: hive <hive@lovyou.ai>
Date:   Sat Mar 28 00:33:32 2026 +1100

    [hive:builder] Fix: [hive:builder] KindClaim graph nodes not synced to MCP knowledge index - assert op and knowledge_search are disconnected

 cmd/post/main_test.go    | 52 ++++++++++++++++++++++++++++++++++++++++++++++++
 loop/budget-20260328.txt |  5 +++++
 loop/build.md            | 43 ++++++++++++++++++++++++++++++---------
 loop/critique.md         | 27 +++++++++++++++++++++----
 loop/reflections.md      | 12 +++++++++++
 loop/test-report.md      | 34 +++++++++++++++++++++++++++++++
 6 files changed, 160 insertions(+), 13 deletions(-)
```
