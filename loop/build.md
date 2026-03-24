# Build Report — Iteration 197

## Trending Feed (Velocity Scoring)

**Store:**
- `ListPostsByTrending(spaceID, limit)` — engagement velocity ranking
- Score: `(recent_endorsements * 3 + recent_reposts * 2 + recent_replies) / GREATEST(1, hours_old)`
- "Recent" = created in last 48 hours (`created_at > NOW() - INTERVAL '48 hours'`)
- Age in hours via `EXTRACT(EPOCH FROM NOW() - n.created_at) / 3600`
- Cast to float for division: `::float`
- Same full Node scan as other methods

**Handler:**
- `tab=trending` branch → `ListPostsByTrending`
- Falls back to chronological for search queries on Trending tab

**Template:**
- "Trending" tab pill added after "For You"

**Difference from For You:**
- For You: cumulative engagement + recency bonus → quality over time
- Trending: recent engagement / age → what's hot RIGHT NOW

**Files changed:**
- `graph/store.go` — `ListPostsByTrending`
- `graph/handlers.go` — trending branch
- `graph/views.templ` — Trending tab pill
