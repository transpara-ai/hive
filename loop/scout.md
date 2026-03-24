# Scout Report — Iteration 203

## Gap: Sidebar still presents Work and Social as separate products

**Source:** unified-spec.md — "Not 'Work' and 'Social' as separate sections. Instead, the sidebar presents available modes."

**Current state:** Sidebar has "Work" (→ Board) then expandable "Social" (→ Feed/Threads/Chat/People) then Knowledge, Governance, Build, Transparency, Settings. This framing says "here are two products." The unified ontology says they're all modes of one thing.

**What's needed:** Reorganize sidebar into modes of organized activity. Group by purpose, not by product line.

**Proposed sidebar:**
```
My Work (dashboard)

Activity:
  Execute (Board/List)
  Chat
  Feed
  Threads
  Forum (future)

Structure: (future — when org entities exist)
  Teams
  Roles
  Policies
  Goals

Knowledge:
  Knowledge
  Build (changelog)

Governance:
  Governance

Transparency:
  Activity

Settings
```

**Simpler first step:** Remove the Work/Social division. Make all modes top-level under one "Modes" or unlabeled nav section. Keep existing routes — just reorganize the sidebar.

**Risk:** Low. Template-only change. No handler/store changes. No route changes.
