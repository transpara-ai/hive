# Build Report — Iteration 195

## For You Feed (Endorsement-Weighted Ranking)

**Store:**
- `ListPostsByEngagement(spaceID, limit)` — new query with engagement scoring
- Score formula: `endorsements * 3 + reposts * 2 + replies + GREATEST(0, 7 - days_old)`
- ORDER BY score DESC, created_at DESC (tiebreaker)
- Same full Node scan (28 columns) as ListNodes for consistency

**Handler:**
- Feed handler: when `tab=foryou` (and no search query), uses engagement-sorted query
- Falls back to chronological ListNodes for search queries on For You tab

**Template:**
- "For You" tab pill added between Following and the search bar
- Same pill styling as All/Following (brand active, edge inactive)

**Scoring rationale:**
- Endorsements weighted 3x (our unique signal — quality/trust)
- Reposts weighted 2x (propagation signal)
- Replies weighted 1x (engagement signal)
- Recency bonus: up to 7 points for posts < 7 days old, preventing stale content from dominating

**Files changed:**
- `graph/store.go` — `ListPostsByEngagement`
- `graph/handlers.go` — foryou tab branch
- `graph/views.templ` — For You tab pill
