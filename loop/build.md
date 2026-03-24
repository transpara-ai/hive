# Build Report — Iteration 198

## Engagement Bar on Node Detail

**Handler:**
- `handleNodeDetail`: loads endorsement count, endorsed state, repost count, reposted state for the node
- Uses existing `CountEndorsements`, `HasEndorsed`, `GetBulkRepostCounts`, `HasReposted`

**Template:**
- `NodeDetailView`: accepts 4 new params: `endorseCount int, endorsed bool, repostCount int, reposted bool`
- Engagement bar: replies count + repost button + quote link + endorse button
- Only shows for posts and threads (not tasks, comments, conversations)
- Reuses existing `endorseButton` and `repostButton` components (same HTMX swap behavior)
- Placed between body and edit form

**Files changed:**
- `graph/handlers.go` — handleNodeDetail engagement data loading
- `graph/views.templ` — NodeDetailView signature + engagement bar
