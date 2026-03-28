# Test Report: Fix: claims.md sync broken — Lessons 126-148 missing from MCP index

## Result: PASS

All 52 tests in `cmd/post` pass. All 13 packages in the repo compile and pass.

## What Was Tested

The iteration replaced `syncClaims()` from a knowledge-endpoint query (which returned 0 nodes because lessons are `kind=task`, not `kind=claim`) to a board-endpoint query filtered by title prefix.

### New functions tested

| Function | Tests | Coverage |
|---|---|---|
| `syncClaims()` | 9 tests | Writes file, skips when empty, filters non-claim nodes, handles nodes with no metadata, deduplicates across both queries, writes causes, multiple causes joined, API error, second-query failure |
| `fetchBoardByQuery()` | 4 tests | Returns nodes, malformed JSON, sends auth header, HTTP error |
| `hasClaimPrefix()` | 1 test | All prefixes match, non-prefix title rejected, empty string rejected |

### Tests directly from build.md

- `TestSyncClaimsWritesFile` — lesson + critique appear, sorted oldest-first ✓
- `TestSyncClaimsEmptyDoesNotWrite` — file not written when both queries return zero nodes ✓
- `TestSyncClaimsFiltersNonClaimNodes` — "Fix the Lesson tracker bug" excluded; "Lesson 42" included ✓
- `TestSyncClaimsClaimWithNoMetadata` — no `**State:**` line when state/author both empty ✓
- `TestSyncClaimsMultipleCauses` — two cause IDs joined as `build-doc-aaa, build-doc-bbb` ✓
- `TestSyncClaimsWritesCauses` — `**Causes:**` label present when node has causes ✓

### Edge cases verified

- Deduplication: same node ID returned by both "Lesson " and "Critique:" queries → appears once (`TestSyncClaimsDeduplicatesAcrossQueries`)
- Authorization header: `fetchBoardByQuery` sends `Bearer <apiKey>` (`TestFetchBoardByQuerySendsAuthHeader`)
- Partial failure: second board query fails → error propagates, no file written (`TestSyncClaimsSecondQueryFails`)
- API error: HTTP 403 → error returned, no file written (`TestSyncClaimsAPIError`)
- Malformed JSON from board → error returned (`TestFetchBoardByQueryMalformedJSON`)

## Run

```
go.exe test -buildvcs=false ./...
```

```
ok  github.com/lovyou-ai/hive/cmd/post    1.109s   (52 tests)
ok  github.com/lovyou-ai/hive/cmd/mcp-graph
ok  github.com/lovyou-ai/hive/cmd/mcp-knowledge
ok  github.com/lovyou-ai/hive/pkg/api
ok  github.com/lovyou-ai/hive/pkg/authority
ok  github.com/lovyou-ai/hive/pkg/hive
ok  github.com/lovyou-ai/hive/pkg/loop
ok  github.com/lovyou-ai/hive/pkg/resources
ok  github.com/lovyou-ai/hive/pkg/runner
ok  github.com/lovyou-ai/hive/pkg/workspace
```

## Coverage Notes

No gaps found. The Builder pre-wrote tests for all new code paths including both helper functions (`fetchBoardByQuery`, `hasClaimPrefix`), all error paths, and all edge cases in the dedup/sort logic. No additional tests were needed.

@Critic ready for review.
