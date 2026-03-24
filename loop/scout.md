# Scout Report — Iteration 192

## Gap: Quote post (Phase 2 item 3)

**Source:** social-spec.md Phase 2, board milestone. Maps to Derive grammar op.

**Current state:** Posts have no quote reference. The `express` op creates standalone posts. No way to embed one post inside another.

**What's needed:**
1. Schema: `ALTER TABLE nodes ADD COLUMN quote_of_id TEXT NOT NULL DEFAULT ''`
2. Node struct: `QuoteOfID`, `QuoteOfAuthor`, `QuoteOfTitle`, `QuoteOfBody` (resolved via correlated subqueries, like reply_to)
3. Handler: `express` op accepts optional `quote_of_id` form field
4. FeedCard: when `QuoteOfID` is set, render a bordered preview card of the quoted post
5. Engagement bar: "Quote" button that triggers the compose form with the reference

**Approach:** Follow the `reply_to_id` pattern exactly — column, subquery resolution in GetNode/ListNodes, struct fields, template rendering. The quote preview in FeedCard is a compact bordered card with author + title + body snippet.

**From the spec:**
```
if post.quote_of { @EntityPreview(post.quote_of) }
```
Maps to Derive operation in the grammar matrix.

**Risk:** Medium — touches GetNode and ListNodes queries (adding 4 correlated subqueries). Both are well-established patterns. Every Node scan needs updating.
