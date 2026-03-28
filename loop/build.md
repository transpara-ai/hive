# Build: Fix: All 103 claims have causes=[] — close.sh assertion pipeline never sets causes

## Gap
Every claim on the knowledge board (lessons + critiques) had `causes=[]`, violating Invariant 2 (CAUSALITY). Root cause: `assertScoutGap` was passing `buildDocID` as cause (not `taskNodeID`), and no retroactive backfill existed for the 103 pre-existing empty claims.

## What Was Built

### 1. `site/graph/store.go` — `UpdateNodeCauses`
Added `UpdateNodeCauses(ctx context.Context, nodeID string, causes []string) error` that executes `UPDATE nodes SET causes = $1 WHERE id = $2`. Used by the extended `op=edit` handler to retroactively set causes on existing claim nodes.

### 2. `site/graph/handlers.go` — extend `op=edit` to support causes
Extended `case "edit"` to accept an optional `causes` field (comma-separated node IDs). Validation now requires `node_id` plus either `body` or `causes` (not necessarily both). When `causes` is provided, calls `UpdateNodeCauses` and records an op. Body-only and causes-only edits both work independently.

### 3. `cmd/post/main.go` — fix `assertScoutGap`, add `backfillClaimCauses`

**Fix `assertScoutGap`**: Moved `taskCauseIDs` computation before both `assertScoutGap` and `assertCritique` calls. Both now use `taskCauseIDs` (task node ID, falling back to `buildDocID` if task creation failed). Previously `assertScoutGap` was using `causeIDs` (build doc ID) while `assertCritique` used `taskCauseIDs` — inconsistent.

**Add `backfillClaimCauses(apiKey, baseURL, taskNodeID string) error`**:
- Fetches all claims from `/app/hive/knowledge?tab=claims&limit=200`
- For each claim with `causes=[]` and non-empty ID, POSTs `op=edit` with `causes=taskNodeID`
- Skips claims that already have causes
- Called from `main()` after `createTask()` succeeds
- Returns error on any HTTP failure; non-fatal from `main()`

**Updated `syncClaims` struct** to include `ID string json:"id"` (was missing).

### 4. `cmd/post/main_test.go` — 4 new tests
- `TestBackfillClaimCausesUpdatesEmptyClaims` — verifies only empty-cause claims are updated
- `TestBackfillClaimCausesSkipsAlreadyCaused` — verifies already-caused claims are not touched
- `TestBackfillClaimCausesEmptyTaskID` — verifies error on empty taskNodeID
- `TestBackfillClaimCausesAPIError` — verifies error on API 4xx

## Verification
- `go.exe build -buildvcs=false ./...` — passes (hive + site)
- `go.exe test ./...` — all 13 packages pass
- All 4 new tests pass
- All existing tests pass

## Files Changed
- `site/graph/store.go` — `UpdateNodeCauses` added
- `site/graph/handlers.go` — `op=edit` extended for causes
- `cmd/post/main.go` — `assertScoutGap` fix, `backfillClaimCauses` added, `syncClaims` struct updated
- `cmd/post/main_test.go` — 4 new backfill tests
