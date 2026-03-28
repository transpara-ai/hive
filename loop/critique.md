# Critique: [hive:builder] Fix: commit main_test.go, remove dead retractedLesson type, fix no-op em-dash replace in republish-lessons

**Verdict:** PASS

**Summary:** All three items from the previous REVISE are addressed:

**1. `main_test.go` committed** ✓  
File exists at `cmd/republish-lessons/main_test.go`, 13 tests covering all three exported functions and the short-ID slicing boundary.

**2. `retractedLesson` struct removed** ✓  
Not present in `main.go` — only `claimNode` remains.

**3. No-op `strings.ReplaceAll` removed** ✓  
No `strings` import, no such call anywhere in `main.go`.

**Code correctness:**
- `queryMaxLessonNumber`: regex `^Lesson (\d+)` anchored, case-sensitive, bounded at 200. Returns 0 on no match (safe). ✓
- `fetchRetractedClaims`: `state=retracted`, limit=200. ✓
- `assertClaim`: Posts `op=assert` with title+body. ✓
- `var baseURL` override pattern is idiomatic Go, no parallel test issues (no `t.Parallel()` calls). ✓

**Invariants:**
- **12 (VERIFIED)**: All functions covered. Short-ID slicing covered by `TestShortIDExtraction`. ✓
- **13 (BOUNDED)**: `limit=200` on both fetches. ✓
- **11 (IDENTITY)**: Short IDs (UUID prefixes) used for matching, not names. ✓

The guard `if maxNum != 183` is intentionally not tested — correctly documented as a one-shot migration invariant that no longer applies.

VERDICT: PASS
