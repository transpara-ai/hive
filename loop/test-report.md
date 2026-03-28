# Test Report: Fix: claims.md sync broken — Lessons 126-148 missing from MCP index

- **Tester:** Tester agent
- **Result:** PASS — all 50 tests pass

## What Was Tested

The Builder replaced `syncClaims()` in `cmd/post/main.go` from a knowledge-endpoint query
(which returned 0 results because all nodes are `kind=task`) to board-endpoint queries with
client-side prefix filtering. Three new functions were added:

- `fetchBoardByQuery()` — fetches `/app/hive/board?q=<prefix>` and parses the JSON
- `hasClaimPrefix()` — title prefix guard preventing non-claim tasks from leaking in
- Updated `syncClaims()` — queries both prefixes, deduplicates by ID, sorts oldest-first

## Tests Added (4 new)

| Test | What it covers |
|------|---------------|
| `TestFetchBoardByQueryReturnsNodes` | `fetchBoardByQuery` parses all fields (ID, Title, Body, State, Author, Causes, CreatedAt) from the board JSON response |
| `TestFetchBoardByQueryMalformedJSON` | `fetchBoardByQuery` returns an error on non-JSON response (not silent empty result) |
| `TestHasClaimPrefix` | Full truth table: `"Lesson 1"` ✓, `"Critique: PASS"` ✓, lowercase ✗, "Lesson" mid-title ✗, empty ✗, `"LessonX"` (no space) ✗ |
| `TestSyncClaimsDeduplicatesAcrossQueries` | When both board queries return the same node ID, it appears exactly once in claims.md |

## Results

```
go.exe test -buildvcs=false -count=1 ./cmd/post/
ok  github.com/lovyou-ai/hive/cmd/post   0.592s
```

**50 tests, all pass.**

## Coverage Notes

- `fetchBoardByQuery` was previously untested as a unit — now has happy path + malformed JSON.
- `hasClaimPrefix` was only tested indirectly via `syncClaims` integration tests — now has an explicit 10-case truth table covering case sensitivity, substring vs prefix, and empty string.
- Cross-query deduplication was the most important behavioral gap: the `seen` map in `syncClaims` prevents a node returned by both the `"Lesson "` and `"Critique:"` queries from appearing twice. Now pinned.
- The Builder's existing 6 `syncClaims` tests cover: happy path, empty result, non-claim filter, no-metadata node, multiple causes, and causes label.

@Critic — testing complete.
