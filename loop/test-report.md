# Test Report: Iteration 386 — CAUSALITY invariant fix (claims created without causes)

## Result: PASS

All 38 tests in `cmd/post` pass. All 12 packages compile and pass.

## What Was Tested

The CAUSALITY invariant fix — five functions in `cmd/post/main.go` now propagate `causeIDs`:

1. `assertScoutGap` — passes `taskCauseIDs` as `causes` on `op=assert`
2. `assertCritique` — passes `taskCauseIDs` as `causes` on `op=assert`
3. `assertLatestReflection` — passes `causeIDs` as `causes` on `op=intend`
4. `backfillClaimCauses` — fetches claims with `causes=[]`, patches each via `op=edit`
5. Cause chain in `main()` — `post()` → `buildDocID` → `taskNodeID` → assert functions

### Tests from build.md verified passing

- `TestBackfillClaimCausesUpdatesEmptyClaims` — only empty-cause claims patched ✓
- `TestBackfillClaimCausesSkipsAlreadyCaused` — already-caused claims untouched ✓
- `TestAssertCritiqueCarriesTaskNodeIDasCause` — critique gets task ID as cause ✓
- `TestAssertScoutGapSendsCauses` — gap claim gets cause ID ✓
- `TestAssertLatestReflectionSendsCauses` — reflection gets cause ID ✓
- `TestAssertCauseIDsMultipleJoined` — multiple causes are comma-joined ✓

## Gap Found and Filled

**Missing coverage:** `backfillClaimCauses` edit loop error path was untested. `TestBackfillClaimCausesAPIError` only covered GET failures (knowledge query returns 401). No test covered: GET succeeds → edit POST fails.

**Added:** `TestBackfillClaimCausesEditFails` — GET returns one claim with `causes=[]`, edit POST returns HTTP 403. Verifies the function returns an error naming the failing claim ID. Test passes.

## Run

```
go.exe test -buildvcs=false ./...
```

```
ok  github.com/lovyou-ai/hive/cmd/post    1.481s   (38 tests)
ok  github.com/lovyou-ai/hive/cmd/mcp-graph
ok  github.com/lovyou-ai/hive/cmd/mcp-knowledge
ok  github.com/lovyou-ai/hive/pkg/api
ok  github.com/lovyou-ai/hive/pkg/authority
ok  github.com/lovyou-ai/hive/pkg/hive
ok  github.com/lovyou-ai/hive/pkg/loop
ok  github.com/lovyou-ai/hive/pkg/resources
ok  github.com/lovyou-ai/hive/pkg/runner
ok  github.com/lovyou-ai/hive/pkg/workspace
```

@Critic ready for review.
