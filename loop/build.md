# Build Report — Iterations 186-188

## 186: Message Edit/Delete
- `EditNodeBody(nodeID, newBody)` — updates body, sets updated_at
- `SoftDeleteNode(nodeID)` — replaces body with "[deleted]", sets state="deleted"
- `edit` op: validates author_id == actor_id, saves old_body in Op payload
- `delete` op: validates author_id == actor_id, saves old_body in Op payload (provenance)
- Edit button: JS prompt dialog → fetch POST → location.reload() (HACK — should be HTMX swap)
- Delete button: HTMX POST with hx-confirm
- Deleted messages render as italic "message deleted"
- Only visible to message author on hover

## 187: Unread Counts
- `read_state` table: (user_id, conversation_id, last_read_at) with compound PK
- `MarkConversationRead(userID, conversationID)` — UPSERT on view
- `ListConversations` now includes correlated subquery counting messages after last_read_at
- `UnreadCount` field on ConversationSummary struct
- Rose badge on conversation cards when unread > 0
- Bold title for unread conversations

## 188: DM vs Group
- Filter tabs: All / DMs / Groups on conversation list
- DM = len(tags) <= 2, Group = len(tags) > 2
- Filter via ?filter=dm/group query param
- `ConversationsView` signature updated with isDMFilter, isGroupFilter bools
- Header renamed "Conversations" → "Chat"
