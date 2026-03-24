# Build Report — Iteration 190

## Endorse on Posts

**Store:**
- `GetBulkEndorsementCounts(targetIDs) map[string]int` — single query for all post endorsement counts
- `GetBulkUserEndorsements(userID, targetIDs) map[string]bool` — which posts the user has endorsed
- Reuses existing `endorsements` table (from_id, to_id). No schema changes.

**Handler:**
- New `endorse` grammar op — toggles endorsement (endorse if not yet, unendorse if already)
- Records op + notifies post author on endorse (not on unendorse)
- HTMX response: returns `endorseButton` component for inline swap
- JSON response: `{"op": "endorse", "endorsed": true/false}`

**Feed handler:**
- Loads bulk endorsement counts + user endorsement state for all posts
- Passes both maps to FeedView

**Template:**
- `FeedView` accepts `endorseCounts map[string]int, userEndorsed map[string]bool`
- `FeedCard` accepts `endorseCount int, endorsed bool`
- `endorseButton` component: thumbs-up icon + count, brand-colored when endorsed, HTMX toggle
- Filled icon when endorsed, outline when not

**Files changed:**
- `graph/store.go` — `GetBulkEndorsementCounts`, `GetBulkUserEndorsements`
- `graph/handlers.go` — `endorse` op case, feed handler wiring
- `graph/views.templ` — `FeedView`, `FeedCard`, `endorseButton` signatures + template
