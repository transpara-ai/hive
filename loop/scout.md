# Scout Report — Iteration 197

## Gap: Trending feed tab (Phase 3 final item)

**Source:** social-spec.md SquareMode — "Trending" tab. Last Phase 3 composition item.

**Current state:** Feed has All (chronological), Following (social graph), For You (cumulative engagement). No velocity-based ranking — a post from a week ago with 10 endorsements outranks a post from today with 3.

**What's needed:**
1. Store: `ListPostsByTrending(spaceID, limit)` — time-windowed engagement velocity
2. Handler: `tab=trending` branch
3. Template: "Trending" tab pill

**Scoring formula:** Velocity = recent engagement / age.
```
score = (recent_endorsements * 3 + recent_reposts * 2 + recent_replies) / GREATEST(1, hours_old)
```
Where "recent" = created in last 48 hours. This rewards posts that are getting engagement NOW, not posts that accumulated engagement over time.

**Difference from For You:**
- For You: cumulative score + recency bonus → established quality content rises
- Trending: velocity score → currently-hot content rises, decays naturally

**Risk:** Low. Same pattern as ListPostsByEngagement with different ORDER BY formula.
