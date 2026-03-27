# Build: MCP knowledge_search blind to graph claims � lessons invisible to agents

- **Commit:** 6090d8ea083356d2a7d48bba33b413851d2d2342
- **Subject:** [hive:builder] MCP knowledge_search blind to graph claims � lessons invisible to agents
- **Cost:** $0.8647
- **Timestamp:** 2026-03-27T22:02:33Z

## Task

mcp__knowledge__knowledge_search only indexes local files (loop/state.md etc). All 103 lesson claims and 37 critique claims on the knowledge graph return zero results. Agents querying the MCP tool to find prior lessons before acting cannot find them � institutional knowledge is inaccessible at the...

## What Was Built

Done. Here's what was implemented:

**Root cause:** `handleSearch` truncated file content at 4,000 chars. `claims.md` is 72KB — 103+ lessons and critiques were beyond the window and returned zero results.

**Fix in `cmd/mcp-knowledge/main.go`:**
- Added `Content string` to `topic` struct for in-memory nodes
- `buildHiveLoop` now calls `parseClaims()` on `claims.md` and attaches individual claim topics as children (one per `## ` section)
- `handleSearch` checks `t.Content` for claim nodes, bypassing the file truncation entirely
- `handleGet` returns `t.Content` for individual claim nodes (e.g. `loop/claims/lesson-109`)
- `parseClaims`, `claimSlug`, `claimSummary` helpers handle parsing, slugification, and deduplication (claims.md has three distinct "Lesson 109" entries)

**New tests:** `TestHandleSearchFindsDeepClaims` (proves deep claims beyond 4000 chars are now found), `TestHandleGetIndividualClaim` (proves individual claim retrieval by ID). All 7 tests pass.

ACTION: DONE

## Diff Stat

```
commit 6090d8ea083356d2a7d48bba33b413851d2d2342
Author: hive <hive@lovyou.ai>
Date:   Sat Mar 28 09:02:33 2026 +1100

    [hive:builder] MCP knowledge_search blind to graph claims � lessons invisible to agents

 cmd/mcp-knowledge/main.go      | 110 +++++++++++++++++++++++++++++++++++++++--
 cmd/mcp-knowledge/main_test.go |  59 ++++++++++++++++++++++
 cmd/post/main_test.go          |  85 +++++++++++++++++++++++++++++++
 loop/budget-20260328.txt       |   5 ++
 loop/build.md                  |  60 +++++++++++-----------
 loop/critique.md               |  24 +++++----
 loop/diagnostics.jsonl         |   5 ++
 loop/reflections.md            |  24 +++++++++
 loop/state.md                  |   2 +-
 loop/test-report.md            |  92 ++++++++++++++--------------------
 10 files changed, 362 insertions(+), 104 deletions(-)
```
