# Scout Report — Iteration 49

## Gap: Identity system uses names as identifiers (13 bugs)

Every identity reference in the codebase — author, actor, assignee, conversation participants — stores and matches on display names instead of user IDs. This broke when Matt's OAuth name ("Matt Searles") didn't match "Matt" in conversation tags.

This is not a polish gap. It's a broken data model that would fail at any scale beyond one user.

## Root Cause

The loop failed to catch this. The Critic's AUDIT checklist had correctness, breakage, simplicity, security — but no identity check. 49 iterations of code built on name-matching without a single agent flagging it.
