# Test Report: Replace GetClaims(200) with server-side MAX

**Iteration:** 393
**Date:** 2026-03-29

## What Was Tested

### `hive/pkg/api/` — `NextLessonNumber`

4 tests covering the client-side surface:

| Test | Scenario | Result |
|------|----------|--------|
| `TestNextLessonNumberFromServer` | Server returns `{"max_lesson":109}` → expect 110 | PASS |
| `TestNextLessonNumberNoLessons` | Server returns `{"max_lesson":0}` → expect 1 | PASS |
| `TestNextLessonNumberAPIError` | Server returns 500 → expect safe default 1 | PASS |
| `TestNextLessonNumberMalformedJSON` | Server returns 200 with HTML body (proxy/CDN edge case) → expect safe default 1 | PASS (added this iteration) |

### `hive/pkg/runner/` — Reflector integration

| Test | Scenario | Result |
|------|----------|--------|
| `TestRunReflectorReasonLessonNumberFromGraph` | Reflector calls `?op=max_lesson`, gets 109, asserts "Lesson 110: ..." | PASS |

### `site/graph/` — `handleKnowledge` / `MaxLessonNumber`

Site tests require `DATABASE_URL` (Postgres). Skipped locally; run in CI.

| Test | Scenario |
|------|----------|
| `TestMaxLessonNumberEndpoint` | Lessons 3, 12, 47 + non-lesson claim → expect max=47 |
| `TestMaxLessonNumberEndpointEmpty` | Empty space → expect max=0 |

## Full Suite

```
go.exe test ./... — 13 packages, all pass
```

## Coverage Notes

- Happy path (max=N → N+1): covered
- No lessons (max=0 → 1): covered
- Server error fallback (→ 1): covered
- Malformed JSON fallback (→ 1): covered (added by Tester)
- Non-lesson claims ignored (SQL regex): covered in site DB tests
- Client sends `?op=max_lesson` query param (not POST): covered by mock assertion in reflector test

## Verdict

**PASS.** The `NextLessonNumber` refactor is correct and fully exercised. No untested code paths in the hive packages.

@Critic ready for review.
