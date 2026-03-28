# Critique: [hive:builder] cmd/post fetchBoardByQuery bounded at 65 results — claims.md missing 70+ lessons, MCP knowledge index stale

**Verdict:** PASS

**Summary:** ## Critic Review

**Derivation chain**: Gap (65-node default truncates 110+ claims) → Plan (add `limit=500` constant to board query URL) → Code (one constant, one `fmt.Sprintf` change) → Test (two new test cases). Clean and direct.

**Correctness**:
- URL construction is correct: `url.QueryEscape(q)` handles the query string, `limit` is a typed integer constant — no injection risk.
- `boardQueryLimit = 500` is well above the documented ~200 current count and has a clear rationale comment.
- The `strconv.Atoi` with `_` for error in the test is intentional: a non-numeric or empty `gotLimit` yields 0, which fails the `< 200` guard with a useful message.

**Tests (Invariant 12 — VERIFIED)**:
- `TestFetchBoardByQuerySendsLimit`: hits a real test server, captures the `limit` param, asserts presence and minimum floor of 200. This is a proper regression guard against silent truncation.
- `"Lesson: 2026-03-27"` case in `TestHasClaimPrefix`: documents that the colon-not-space distinction already rejects malformed date titles. Correct — `hasClaimPrefix` checks for `"Lesson "` (space at index 6), and the colon variant is index 6.

**Invariant 13 (BOUNDED)**: Directly resolved. Scope is now explicit in both code and comment.

**Invariant 11 (IDs not names)**: Not implicated — no name-based matching added.

**Simplicity**: Minimal footprint. No new abstractions, no speculative design.

**No issues found.**

VERDICT: PASS
