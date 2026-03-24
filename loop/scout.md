# Scout Report — Iteration 190

## Gap: Endorse on posts (Phase 2 item 1 — first Square feature)

**Source:** social-spec.md Phase 2, board milestone. First differentiator feature.

**Current state:** Endorsements exist for users only (endorsements table: from_id → to_id). Profile pages show endorsement count + toggle button. No endorsement on content (posts, threads, claims, etc.).

**What's needed:**
1. `endorse` grammar op — toggles endorsement on a node (endorse/unendorse)
2. Bulk endorsement loading — counts per node + user's endorsement state, for Feed rendering
3. Endorsement button on FeedCard — HTMX toggle with count, brand-colored when endorsed
4. Endorsement count on node detail

**Why endorse first:** It's our differentiator. Reactions (emoji) are acknowledgment. Endorsement is a quality/trust signal — "I vouch for this." No other platform has this on content. It maps directly to the Code Graph Endorse primitive.

**Approach:** Reuse existing endorsements table (from_id, to_id). Node IDs and user IDs are in different namespaces (both random hex, but stored in different tables). `CountEndorsements(nodeID)` works as-is. Add bulk operations for Feed efficiency. Follow the reaction pattern: `GetBulkReactions` → `GetBulkEndorsementCounts` + `GetBulkUserEndorsements`.

**Risk:** Low. No schema changes. Existing store methods work for nodes. Just need bulk variants + handler op + template buttons.
