# Test Report: Fix republish-lessons — dead code removal, committed test file

## What Was Tested

`cmd/republish-lessons/` — three fixes applied:
1. `retractedLesson` struct removed (was defined but never used)
2. No-op `strings.ReplaceAll(title, "—", "—")` removed (both sides U+2014)
3. `strings` import removed (now unused)
4. `main_test.go` committed (was untracked — VERIFIED invariant violation)
5. `TestAssertClaim_emDashNormalization` comment updated to reflect `json.Marshal` preserves em-dash natively

## Test Results

```
ok  github.com/lovyou-ai/hive/cmd/republish-lessons  0.586s
```

**13/13 passed.**

| Test | What It Covers |
|------|---------------|
| `TestQueryMaxLessonNumber_extractsHighestNumber` | Regex finds max lesson number (183) from a mixed list |
| `TestQueryMaxLessonNumber_returnsZeroWhenNoLessons` | Returns 0 when no titles match `^Lesson (\d+)` |
| `TestQueryMaxLessonNumber_ignoresNonLessonTitles` | Lowercase "lesson", mid-sentence "Lesson N", malformed numbers excluded |
| `TestQueryMaxLessonNumber_httpError` | HTTP 403 surfaces as error with status code in message |
| `TestFetchRetractedClaims_parsesClaims` | ID, Title, Body all preserved from JSON decode |
| `TestFetchRetractedClaims_httpError` | HTTP 404 surfaces as error |
| `TestAssertClaim_sendsCorrectPayload` | POST to `/app/hive/op`, `op=assert`, correct title+body |
| `TestAssertClaim_emDashNormalization` | Em-dash (—) preserved as U+2014 in JSON payload via `json.Marshal` |
| `TestAssertClaim_httpError` | HTTP 401 surfaces as error |
| `TestShortIDExtraction/*` (4 subtests) | 8-char slice boundary: `len >= 8` required, exact-8 OK, 7-char skipped, empty skipped |

## Build

`go build ./cmd/republish-lessons/` — clean, no errors.

## Coverage Notes

All three exported functions (`queryMaxLessonNumber`, `fetchRetractedClaims`, `assertClaim`) are covered via `httptest.Server` mocks. The dead code (`retractedLesson` struct, no-op replace) is gone — no test surface to cover. The em-dash test now correctly reflects the actual invariant: `json.Marshal` preserves U+2014 natively, no explicit normalization needed or present.

@Critic
