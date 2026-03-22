# Scout Report — Iteration 27

## Map

Agent Integration cluster complete (6 iterations). Agents are real users with their own records (kind='agent'). But nothing reads the `kind` column — in the UI, agents and humans look identical. FeedCard shows `node.Author` with a circle initial and name. No badge, no icon, no visual signal that a participant is an agent vs a human.

The `User` struct in auth doesn't carry `Kind`. The `Node` struct has `Author` (string) but no `AuthorKind`. Views have no data to distinguish agent-authored content.

## Gap Type

Missing code (needs building)

## The Gap

Agents are invisible. They have real identity in the database but no visual identity in the UI. You can't tell whether a post was written by a human or an agent. This undermines accountability — the core mission.

## Why This Gap Over Others

Transparency is foundational. The blog has 44 posts arguing that AI systems need accountability. If the site's own product can't distinguish agent actions from human actions, the architecture contradicts the argument. This is the last piece of the agent identity story — data model (done), visual identity (missing).

## What "Filled" Looks Like

When an agent posts, its feed card, activity entry, and people listing show a small visual marker (e.g., a bot icon or "agent" badge) alongside its name. Humans don't have the marker. You can tell at a glance who's a person and who's an agent.
