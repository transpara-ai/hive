# Scout Report — Iteration 195

## Gap: Endorsement-weighted feed ("For You" tab)

**Source:** social-spec.md SquareMode — "For You" tab. Phase 3 composition.

**Current state:** Feed has All and Following tabs. Both sort by `pinned DESC, created_at` (chronological). Endorsements exist but don't affect visibility or ranking.

**What's needed:**
1. A "For You" tab that ranks posts by engagement signals (endorsements, reposts, replies)
2. Scoring: endorsement_count as primary signal, with reply count and repost count as secondary, time decay so old posts don't dominate
3. Tab pill added to the existing All / Following row

**Why this:** Endorsement is our differentiator (Code Graph primitive). Making it the ranking signal means endorsing a post actually does something — it makes the post more visible. This is the first time a Code Graph primitive directly affects the user experience beyond a counter.

**Approach:**
- SQL scoring: `(endorsement_count * 3 + repost_count * 2 + reply_count) + recency_days_bonus`
- Recency bonus: posts < 7 days old get `(7 - days_old)` added to score
- New store method: `ListPostsByEngagement(spaceID, limit)` with the scoring ORDER BY
- Handler: when `tab=foryou`, use the engagement-sorted query
- Template: add "For You" tab pill

**Risk:** Low. One new store method, one handler branch, one template pill. The scoring formula can be tuned later.
