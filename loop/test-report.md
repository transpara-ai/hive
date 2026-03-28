# Test Report: Validate LLM-generated cause IDs in Observer

**Build commit:** bc7722f — Validate LLM-generated cause IDs in Observer before posting
**Timestamp:** 2026-03-29

## What Was Tested

### New function: `NodeExists` (`pkg/api/client.go`)

The Builder added `NodeExists(slug, id string) bool` with no unit tests. Added 6 tests in `pkg/api/client_test.go`:

| Test | What it guards |
|------|---------------|
| `TestNodeExists_Returns200_ReturnsTrue` | HTTP 200 → true |
| `TestNodeExists_Returns404_ReturnsFalse` | HTTP 404 → false (hallucinated ID) |
| `TestNodeExists_Returns500_ReturnsFalse` | HTTP 500 → false (server error = non-existence) |
| `TestNodeExists_URLFormat` | Request hits `/app/{slug}/node/{id}?format=json` exactly |
| `TestNodeExists_SendsBearerAuth` | `Authorization: Bearer <key>` header sent |
| `TestNodeExists_UsesGETMethod` | Uses GET, not POST |

### New validation path: `runObserverReason` (`pkg/runner/observer.go`)

Builder's own test covers the primary path; prior iteration tests cover surrounding logic:

| Test | What it guards | Verdict |
|------|----------------|---------|
| `TestRunObserverReason_HallucinatedCauseIDGetsReplaced` | Ghost ID → 404 → fallback used | PASS |
| `TestRunObserverReason_FallbackCause` | TASK_CAUSE:none → fallback applied | PASS |
| `TestRunObserverReason_OwnCauseTakesPrecedence` | Valid LLM cause → NodeExists returns 200 → preserved | PASS |
| `TestRunObserverReason_FallbackCause_WhenFallbackEmpty` | No panic when ghost ID + empty graph | PASS |

## Results

```
ok  github.com/lovyou-ai/hive/pkg/api     0.564s   (6 new tests)
ok  github.com/lovyou-ai/hive/pkg/runner  4.705s
```

All tests pass. No regressions.

## Coverage Notes

- `NodeExists` is now fully exercised at unit level: 200/404/500, URL format, auth header, HTTP method.
- `TestRunObserverReason_OwnCauseTakesPrecedence` now implicitly tests the NodeExists=true path (server returns 200 for GET, cause is preserved).
- Network error path in `NodeExists` (client.Do error) not tested directly — fragile to test with httptest; the guard is a two-line early return.

@Critic — testing complete.
