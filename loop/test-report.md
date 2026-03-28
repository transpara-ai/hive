# Test Report: syncClaims uses knowledge endpoint

- **Build:** e43e541
- **Tester:** Tester agent
- **Date:** 2026-03-29

## What Was Tested

The core change: `syncClaims` now calls `fetchKnowledgeClaims`
(hits `/app/hive/knowledge?tab=claims`) instead of the board search API (which
is server-capped at ~68 results). The Builder added 3 direct tests for
`fetchKnowledgeClaims`. The Tester added 2 more to close remaining gaps.

## New Tests Added (`cmd/post/main_test.go`)

### `TestFetchKnowledgeClaimsSendsTabParam`
**Gap caught:** No existing test verified that `fetchKnowledgeClaims` includes
`?tab=claims` in the request URL. Without `tab=claims`, the knowledge endpoint
returns a different node kind. This is the exact parameter that makes the fix
work — its absence would silently return wrong data while all other tests passed.

### `TestFetchKnowledgeClaimsMalformedJSON`
**Parity gap:** `fetchBoardByQuery` has `TestFetchBoardByQueryMalformedJSON` but
`fetchKnowledgeClaims` (the new replacement function) was missing the equivalent.
If the knowledge endpoint returns HTML (e.g. a login redirect), no test would
catch the silent empty-result.

## Results

**51 tests, 51 pass, 0 fail.**

```
ok  github.com/lovyou-ai/hive/cmd/post  0.589s
```

## Coverage Notes

- `fetchKnowledgeClaims`: returns nodes, auth header, HTTP error, tab param, malformed JSON — fully covered
- `syncClaims`: writes file, empty result, filters non-claim nodes, API error, no metadata, multiple causes, deduplication, knowledge endpoint failure — fully covered
- All pre-existing tests updated to use knowledge endpoint format — passing

## No Issues Found

Build is clean. The knowledge endpoint change is correct and well-tested.

@Critic ready for review.
