# Test Report: Fix fetchBoardByQuery 65-node cap

**Date:** 2026-03-29

## What Was Tested

Two tests added by the Builder in this iteration:

### 1. `TestFetchBoardByQuerySendsLimit` (cmd/post/main_test.go:1535)
- Verifies `fetchBoardByQuery` sends a `limit` query parameter
- Asserts limit >= 200 (current lesson count ~200, constant is 500)
- **Catches the regression**: without this test, silent truncation at 65 nodes could silently return

### 2. `TestHasClaimPrefix` — `"Lesson: 2026-03-27"` case (cmd/post/main_test.go:1667)
- Added `{"Lesson: 2026-03-27", false}` case
- Documents that malformed "Lesson: date" titles (colon at index 6, not space) are rejected
- "Lesson " requires a space at position 7 — "Lesson:" has a colon, so prefix doesn't match

## Results

| Test | Result |
|------|--------|
| `TestFetchBoardByQuerySendsLimit` | PASS |
| `TestHasClaimPrefix` (all 11 cases incl. new date case) | PASS |

## Full Suite

```
go.exe test -count=1 ./...  — 13 packages, all pass (0.655s on cmd/post)
```

## Coverage Notes

- `fetchBoardByQuery` covered by 5 tests: Limit, ReturnsNodes, MalformedJSON, SendsAuthHeader, HTTPError — the new test completes the set
- `hasClaimPrefix` now has 11 cases including the malformed "Lesson: date" edge case
- `boardQueryLimit = 500` constant exercised via the limit check (>= 200 assertion)

## Verdict

**PASS.** Both new tests exercise exactly what changed. No gaps. No production code issues.

@Critic ready for review.
