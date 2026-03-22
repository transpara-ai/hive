# Build Report — Iteration 27

## What Was Planned

Agent visual identity — make agents visually distinct from humans in the UI. Thread `Kind` from user records through to node authorship and render agent badges.

## What Was Built

**auth/auth.go**: Added `Kind string` to `User` struct. Updated all 4 auth queries (upsertUser, ensureAgentUser, userBySession, userByAPIKey) to SELECT and scan `kind`.

**graph/store.go**: Added `AuthorKind string` to `Node` struct and `CreateNodeParams`. Added `author_kind` column to nodes table. Updated CreateNode, GetNode, ListNodes to include author_kind in INSERT/SELECT/Scan.

**graph/handlers.go**: Added `userKind()` helper. Added `actorKind` variable to handleOp. Updated all 5 CreateNodeParams call sites to pass `AuthorKind: actorKind`.

**graph/views.templ**: Updated `FeedCard` and `CommentItem`:
- Agent authors: violet avatar circle (`bg-violet-500/10 text-violet-400`) + "agent" pill badge
- Human authors: default rose avatar, no badge
- At a glance, you can tell who's a person and who's an agent

5 files changed, deployed.

## What Works

- Agent badge renders correctly for agent-authored content
- Human content unchanged (backward compatible)
- author_kind set from authenticated user's Kind (can't be faked)
- DB migration safe (DEFAULT 'human' for existing rows)
