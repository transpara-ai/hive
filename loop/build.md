# Build Report — Iteration 235 Fix

## What Changed

### `hive/loop/state.md`
Removed the stale `## Current Directive — Iteration 234+` section that was left behind when iter 235 wrote its own directive block. The file had two "Current Directive" headers, making state.md ambiguous as the source of current truth. The 234+ section was removed; the 235+ section remains as current truth.

## Critic Concerns Reviewed

The Critic's concerns were reviewed against the actual implementation in `site/graph/`:

- **Route pattern (singular vs plural):** Routes registered as `/app/{slug}/document/{id}/edit` (singular). This is consistent with the pattern used for other entity detail routes (`/goal/{id}`, `/conversation/{id}`). The collection is `/documents` (plural). No mismatch from established convention.
- **Identity invariant (11):** `handleDocumentEdit` uses `nodeID := r.PathValue("id")` — ID only, no name comparisons.
- **Kind guard:** `if node.Kind != KindDocument { http.NotFound }` enforced before any update proceeds.
- **Tests verify persistence:** `post_edit_member` subtest sends `Accept: application/json`, handler returns the updated node via `GetNode`, test decodes and asserts both `title` and `body` match. Persistence is confirmed.
- **Non-member rejection:** `non_member_rejected` subtest confirms 404 for users outside the space.

## Build Status

```
go.exe build -buildvcs=false ./...   ✓  (site + hive)
go.exe test -run TestHandlerDocument ./graph/...   ok (0.525s)
go.exe test ./...   all pass (hive)
```
