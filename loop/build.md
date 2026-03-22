# Build Report — Iteration 49

## What was built

**ID-based identity throughout** — eliminated all 13 name-as-identifier bugs.

### Code changes (site repo)
- `store.go`: Added `author_id` column to nodes, `actor_id` column to ops. All queries use ID-based JOINs. `ListConversations` matches on userID only. `HasAgentParticipant` matches on ID. `RecordOp` takes actorID.
- `handlers.go`: All handlers pass userID to CreateNode and RecordOp. `converse` resolves participant names to IDs. Mind triggers pass senderID.
- `mind.go`: `findAgentParticipant` returns (id, name). `buildMessages` compares on AuthorID. `replyTo` uses agent ID.
- `views.templ`: `chatMessage` compares on AuthorID, not name.
- Tests updated throughout.

### Loop changes (hive repo)
- `CORE-LOOP.md`: Added Identity and Tests checks to Critic's AUDIT checklist.
- `CLAUDE.md` (hive): Added "IDs are identity, names are display" to coding standards. Added invariants 11 (IDENTITY) and 12 (VERIFIED).
- `CLAUDE.md` (workspace): Added identity and tests to Critic description.

### Data migration
- Existing conversations updated to include Matt's user ID in tags.
