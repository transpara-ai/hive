# Build: syncClaims uses board search (server-capped at ~68) not knowledge endpoint � claims.md still has 4/187 lessons

- **Commit:** e43e541c817c2ca94041c2e94b6e140676336b6d
- **Subject:** [hive:builder] syncClaims uses board search (server-capped at ~68) not knowledge endpoint � claims.md still has 4/187 lessons
- **Cost:** $1.4729
- **Timestamp:** 2026-03-28T16:19:55Z

## Task

0b134cd added limit=500 to fetchBoardByQuery but the server still returns ~68 nodes for q=Lesson. The board search is server-side capped regardless of the limit param. The knowledge endpoint (/app/hive/knowledge?tab=claims&limit=200) returns all 187 claims. Fix: replace syncClaims board-query loop w...

## What Was Built

All done. Summary of changes:

**`cmd/post/main.go`:**
- Added `fetchKnowledgeClaims(apiKey, baseURL)` — hits `/app/hive/knowledge?tab=claims`, parses `{"claims": [...]}`, returns `[]boardNode`
- Replaced `syncClaims`'s two-pass board query loop with a single call to `fetchKnowledgeClaims`
- Updated stale comments

**`cmd/post/main_test.go`:**
- Updated 6 tests to use the knowledge endpoint (`/app/hive/knowledge`) returning `{"claims": [...]}` instead of board endpoint
- Replaced obsolete `TestSyncClaimsSecondQueryFails` with `TestSyncClaimsKnowledgeEndpointFails`
- Added 3 new tests covering `fetchKnowledgeClaims` directly

Build: clean. All tests pass.

ACTION: DONE

## Diff Stat

```
commit e43e541c817c2ca94041c2e94b6e140676336b6d
Author: hive <hive@lovyou.ai>
Date:   Sun Mar 29 03:19:54 2026 +1100

    [hive:builder] syncClaims uses board search (server-capped at ~68) not knowledge endpoint � claims.md still has 4/187 lessons

 cmd/post/main.go         |  72 +++++++++++----
 cmd/post/main_test.go    | 228 +++++++++++++++++++++++++++++------------------
 loop/budget-20260329.txt |   1 +
 loop/build.md            |  54 +++++------
 4 files changed, 214 insertions(+), 141 deletions(-)
```
