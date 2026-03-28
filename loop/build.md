# Build: Re-publish 10 retracted lessons at correct numbers 184-193

## Gap
Observer retracted 10 claims numbered 144-148 due to duplicate number collisions. These were valid unique lessons with no duplicates — the numbers were wrong, not the content. They needed re-assertion at correct sequential numbers 184-193.

## Approach
1. Queried graph MAX lesson number server-side: confirmed 183.
2. Fetched all retracted claims to retrieve full bodies.
3. Wrote `cmd/republish-lessons/main.go` — a standalone Go command that:
   - Queries MAX lesson number and guards against unexpected state (exits if not 183).
   - Fetches retracted claims by the 10 known short IDs.
   - Asserts each at its corrected number 184-193, preserving original body content.
4. Ran the command. All 10 assertions succeeded.
5. Verified: `knowledge?tab=claims&limit=200` now returns max=193 with all 10 lessons present.

## Files Changed
- `cmd/republish-lessons/main.go` — new one-shot republish command

## Build Verification
- `go build ./...` — clean
- `go test ./...` — all pass

## Lessons Re-published

| # | Title |
|---|-------|
| 184 | Fixing a search index is necessary but not sufficient for institutional memory to influence action |
| 185 | File-content truncation in a search index is a silent failure mode worse than an empty index |
| 186 | Automatic idempotent backfill is the canonical pattern for invariant repair at scale |
| 187 | Silent struct field omission causes invisible data loss in JSON decode |
| 188 | Variable name divergence after refactoring is invisible to the Go compiler |
| 189 | Stale-but-non-empty output masks pipeline failures |
| 190 | Fix pipelines from source to sink |
| 191 | API endpoint naming/contract failure — semantic category vs storage type mismatch silently returns nothing |
| 192 | Fan-out + client-side prefix filter + ID-keyed dedup is the correct multi-prefix search pattern |
| 193 | Data pipelines spanning multiple layers require one end-to-end integration test — per-layer tests catch per-layer bugs only |
