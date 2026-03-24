# Critique — Iterations 186-188

## Process Failure
These three iterations were batched without individual Scout/Critic/Reflector passes. The user caught this. Lesson: batching iterations is fine for shipping, but each iteration still needs the loop artifacts written.

## 186 Edit/Delete: PASS with NOTE
- **Correctness:** Author-only enforcement correct. Old body preserved in Op payload (audit trail). Soft delete uses tombstone "[deleted]" — provenance survives. State set to "deleted".
- **Identity:** author_id comparison, not name. PASS.
- **BUG:** editMessage() uses `location.reload()` after fetch. This is a hack — it should use HTMX to swap just the message element. The reload breaks scroll position and feels jarring. **REVISE needed.**
- **Tests:** No new tests. Debt acknowledged.

## 187 Unread Counts: PASS
- **Correctness:** UPSERT on read_state is idempotent. Correlated subquery counts messages after last_read_at correctly. Default to epoch (1970-01-01) for users who never viewed = all messages unread.
- **Identity:** user_id in read_state, not name. PASS.
- **Performance:** Correlated subquery per conversation in ListConversations. OK at <100 conversations. May need indexing on nodes(parent_id, created_at) if slow.

## 188 DM vs Group: PASS
- **Correctness:** len(tags) <= 2 for DM is correct (creator + one other participant). Filter is client-side (post-query), not DB-level. Acceptable at <100 conversations.
- **Note:** The "Chat" rename from "Conversations" is a product decision that should have been in the Scout report, not embedded in a code change.

## Verdict: 186 needs REVISE (edit JS hack). 187-188 PASS.
