# Critique: [hive:builder] claims.md sync broken: Lessons 126-148 missing from MCP index

**Verdict:** PASS

**Summary:** The diff adds three new tests to `cmd/post/main_test.go`. Let me trace each against the production code:

**TestFetchBoardByQuerySendsAuthHeader**
- Production `fetchBoardByQuery` (line 387): `req.Header.Set("Authorization", "Bearer "+apiKey)` — header is set. Test verifies this with a real HTTP server. ✓

**TestFetchBoardByQueryHTTPError**
- Production (lines 396–399): `if resp.StatusCode >= 400 { return nil, fmt.Errorf(...) }` — 401 ≥ 400, error returned. ✓

**TestSyncClaimsSecondQueryFails**
- `claimTitlePrefixes = []string{"Lesson ", "Critique:"}` — exactly 2 prefixes. callCount=1 → success, callCount=2 → 500 error.
- `syncClaims` returns error on first failed `fetchBoardByQuery` (line 330–331), before `os.WriteFile` — file is never written. ✓
- Mock node `"Lesson 1: first lesson"` passes `hasClaimPrefix` (prefix `"Lesson "`). ✓
- `"created_at": "2026-01-01T00:00:00Z"` parses into `time.Time` via RFC3339. ✓
- `callCount` shared by closure with no mutex — safe because `syncClaims` issues requests sequentially in a for loop. ✓

**Invariants:**
- **IDENTITY (11)**: Test mock uses `"id": "node-1"`, production code filters on `n.ID`. ✓
- **VERIFIED (12)**: All three functions now have direct unit test coverage. ✓

The loop artifact files (budget, diagnostics, reflections) are standard loop outputs.

VERDICT: PASS
