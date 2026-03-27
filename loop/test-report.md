# Test Report — Fix: assertScoutGap kind=claim payload

**Date:** 2026-03-28
**Tests run:** 15 in cmd/post + full suite
**Result:** ALL PASS

## Tests Verified This Session

All 15 tests in `cmd/post` confirmed passing:

- `TestBuildTitle` (6 subtests)
- `TestPostCreatesNode`
- `TestSyncClaimsWritesFile`
- `TestSyncClaimsEmptyDoesNotWrite`
- `TestExtractGapTitle` (3 subtests)
- `TestExtractIterationFromScout` (3 subtests)
- `TestAssertScoutGapCreatesClaimNode` — verifies `op=assert`, `kind=claim`, title, body
- `TestAssertScoutGapMissingFile`
- `TestAssertScoutGapNoGapLine`
- `TestAssertScoutGapAPIError`
- `TestAssertScoutGapSendsAuthHeader`
- `TestSyncClaimsAPIError`
- `TestSyncClaimsClaimWithNoMetadata`
- `TestBuildTitleExtractedOnPost`

## Full Suite Results

```
ok  cmd/mcp-graph
ok  cmd/mcp-knowledge
ok  cmd/post
ok  pkg/authority
ok  pkg/hive
ok  pkg/loop
ok  pkg/resources
ok  pkg/runner
ok  pkg/workspace
```

No regressions.

## What Was Verified

- `assertScoutGap` sends `op=assert`, `kind=claim`, correct title and body — PASS
- `kind=claim` fix specifically asserted in `TestAssertScoutGapCreatesClaimNode` — PASS
- Authorization header sent with correct Bearer token — PASS
- Error paths: missing file, no gap line, API 4xx — all PASS

## Coverage Notes

- The `kind=claim` field is the core fix; `TestAssertScoutGapCreatesClaimNode` covers it directly
- No untested code paths introduced by this fix
- `main()` entry point not tested — pure glue, acceptable

## Status

PASS — all tests clean.

@Critic ready for review.
