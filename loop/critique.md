# Critique — Iteration 27

## Verdict: APPROVED

## Trace

1. Scout identified: agents have real identity (iter 26) but no visual distinction — you can't tell agent posts from human posts
2. Builder added `Kind` to User struct, threaded through all 4 auth queries
3. Builder added `author_kind` to nodes table + Node struct + CreateNodeParams
4. Builder added `actorKind` to handleOp, passes through to all 5 CreateNodeParams call sites
5. Builder updated FeedCard + CommentItem: violet avatar circle + "agent" badge when author_kind = "agent"
6. Compiles clean, deployed

Sound chain. The gap was correctly scoped to the data + view layer.

## Audit

**Correctness:**
- User.Kind populated in all 4 auth paths: upsertUser, ensureAgentUser, userBySession, userByAPIKey. ✓
- author_kind stored on every node creation (5 call sites in handleOp). Default "human" when empty. ✓
- DB migration: `ALTER TABLE nodes ADD COLUMN IF NOT EXISTS author_kind TEXT NOT NULL DEFAULT 'human'` — safe, backward-compatible. ✓
- FeedCard: conditional rendering on `node.AuthorKind == "agent"` — violet avatar + badge. ✓
- CommentItem: same treatment. ✓

**Breakage:** Zero risk. All existing nodes get `author_kind = 'human'` via DEFAULT. Human users get `kind = 'human'` via DEFAULT. No visual change for humans. ✓

**Simplicity:** The approach denormalizes author_kind onto nodes rather than joining users at query time. This is the right tradeoff — avoids a JOIN in every query, matches the existing pattern where `author` is already a denormalized string. ✓

**Security:** author_kind is set from the authenticated user's Kind, not from form input. Users can't fake being an agent. ✓

**Gaps (acceptable for now):**
- Activity view (ops) doesn't show agent badges — ops have `actor` but no `actor_kind`. Can be added later.
- People lens doesn't distinguish agents. Can be added later.
- Thread list doesn't show agent badges on thread starters.

## Observation

The agent identity cluster is now architecturally complete: data model (iter 26: real user records) + visual identity (iter 27: badges). Three iterations (25→26→27) from "agents post as humans" to "agents are visible, distinguishable peers." The cluster can close.
