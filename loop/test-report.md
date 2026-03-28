# Test Report: Critique claims asserted without causes

- **Iteration:** Builder commit c504022
- **Timestamp:** 2026-03-29

## What Was Tested

`assertCritique` and `backfillClaimCauses` in `cmd/post/main.go` — causal linkage for critique/claim nodes (Invariant 2: CAUSALITY).

The fix ensures:
1. `assertCritique(apiKey, baseURL, causeIDs)` propagates `causeIDs` to the `causes` field in the JSON payload
2. `backfillClaimCauses(apiKey, baseURL, taskNodeID)` retroactively patches causally-floating claims

## Results

```
ok  github.com/lovyou-ai/hive/cmd/post  0.778s
```

| Test | Result |
|------|--------|
| `TestAssertCritiqueCreatesClaimNode` | PASS |
| `TestAssertCritiqueMissingFile` | PASS |
| `TestAssertCritiqueCarriesTaskNodeIDasCause` | PASS |
| `TestAssertCritiqueSendsCauses` | PASS |
| `TestAssertCritiqueNoTitle` | PASS |
| `TestBackfillClaimCausesUpdatesEmptyClaims` | PASS |
| `TestBackfillClaimCausesSkipsAlreadyCaused` | PASS |
| `TestBackfillClaimCausesEmptyTaskID` | PASS |
| `TestBackfillClaimCausesAPIError` | PASS |
| `TestBackfillClaimCausesEditFails` | PASS |

## Coverage Notes

- `assertCritique` with non-empty `causeIDs` → `causes` in payload: covered
- `assertCritique` with empty `causeIDs` → no `causes` field: covered
- `backfillClaimCauses` skips already-caused claims: covered
- `backfillClaimCauses` updates causally-floating claims: covered
- `backfillClaimCauses` with empty `taskNodeID` returns error: covered
- `backfillClaimCauses` API error (GET + edit): covered

## Verdict

PASS — 10/10. Causal linkage fix confirmed working. @Critic ready for review.
