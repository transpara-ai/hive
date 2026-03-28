# Critique: [hive:builder] claims.md sync broken: Lessons 126-148 missing from MCP index

**Verdict:** PASS

**Summary:** All tests pass. Here is my audit:

**Derivation chain:** The gap was that `syncClaims` used `/knowledge?tab=claims` which returned 0 results because Lessons/Critiques are stored as `kind=task` on the board, not `kind=claim` on the knowledge lens. The fix switches to `/board?q=<prefix>` with a client-side title-prefix filter and a dedup map (for the two-query fan-out).

**Correctness:**
- `url.QueryEscape` is used correctly on the query param — no injection risk.
- `seen` map by node ID prevents duplication when both queries return the same node.
- `hasClaimPrefix` filter is applied _after_ the API call, guarding against board search returning false positives (e.g. "Fix the Lesson tracker bug"). This is exactly right.
- `time.Time` in `boardNode.CreatedAt` — JSON `"2026-03-01T00:00:00Z"` unmarshals into `time.Time` correctly via Go's standard `encoding/json` (RFC3339 is the default format).
- Sort oldest-first is correct.
- `TestSyncClaimsEmptyDoesNotWrite` correctly tests _both_ prefixes returning empty (the test server returns `{"nodes":[]}` for any path, covering both `q=Lesson+` and `q=Critique%3A` calls).

**Invariant 11 (IDs not names):** `seen` map keys on `n.ID` — correct. Dedup is by ID, not title.

**Invariant 12 (VERIFIED):** Six `TestSyncClaims*` tests covering: happy path (both prefix types), empty (no write), prefix filter, no metadata, API error, multiple causes, causes-written. New `TestSyncClaimsFiltersNonClaimNodes` is a clean addition that exercises the exact guard path. Coverage is thorough.

**No concerns.** The `TestSyncClaimsAPIError` test still sends to `/app/hive/board` path (any path returns 403 in that test), so it correctly exercises the error path through `fetchBoardByQuery`.

VERDICT: PASS
