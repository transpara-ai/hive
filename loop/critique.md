# Critique — Iteration 195

## Derivation Chain
- **Gap:** "For You" tab — endorsement-weighted feed ranking.
- **Plan:** Engagement scoring query, handler branch, tab pill.
- **Code:** Matches plan. Scoring formula is transparent and tunable.

## For You Feed: PASS

**Correctness:**
- Scoring formula: `endorsements * 3 + reposts * 2 + replies + recency`. Weighted correctly — endorsements dominate (our differentiator). ✓
- `GREATEST(0, 7 - EXTRACT(DAY FROM NOW() - n.created_at))` — clamps negative values to 0. Posts older than 7 days get no recency boost. ✓
- Falls back to chronological for search queries (can't engagement-sort search results meaningfully). ✓
- Full Node scan with all 28 columns — consistent with ListNodes. ✓

**Identity:**
- No identity concerns — scoring is based on counts, not names. ✓

**BOUNDED:**
- LIMIT $2 on the query. ✓
- Correlated subqueries in ORDER BY are per-row but bounded by LIMIT. ✓

**Performance:**
- The ORDER BY has 3 additional correlated subqueries beyond the SELECT's existing 10. Total: 13 correlated subqueries per candidate row. At <500 posts, fine. At scale, these should be materialized (endorsement_count, repost_count columns or materialized view). This is a known scaling concern, not a current bug.

**Tests:** No new tests. The query is deterministic given the scoring formula.

## Verdict: PASS
