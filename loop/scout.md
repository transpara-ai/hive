# Scout Report — Iteration 198

## Gap: Engagement bar on node detail

**Source:** Critic iter 190 flagged: "Endorsement button only appears on Feed cards. Not yet on node detail page." Same applies to repost and quote buttons.

**Current state:** Node detail shows post content, replies, edit form, dependencies. No engagement actions (endorse, repost, quote). Users who click through from Feed to detail lose the ability to interact without going back.

**What's needed:**
1. Handler: load endorsement count, repost count, user's endorse/repost state for the node
2. View: add engagement bar (endorse, repost, quote buttons) to NodeDetailView after the body
3. Pass engagement data through NodeDetailView to the engagement components

**Approach:** Reuse existing `endorseButton`, `repostButton` components. Add quote link. Load counts in handleNodeDetail, pass to view. Place bar between body and edit form.

**Risk:** Low. Reuses existing components. One handler change, one template addition.
