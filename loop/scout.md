# Scout Report — Iteration 194

## Gap: Following feed tab (Phase 3 item 1 — composition begins)

**Source:** social-spec.md SquareMode — "Following / For You / Trending" tabs. First Phase 3 item.

**Current state:** Feed shows ALL posts in a space, unfiltered. Following someone changes a count on their profile but doesn't affect what you see. Reposts record a relation but don't surface content.

**What's needed:**
1. Store: `ListFollowedIDs(userID) []string` — IDs of users the current user follows
2. Feed handler: read `?tab=following` query param, filter posts to followed authors
3. Feed template: All / Following tabs above the feed
4. Include reposts: when on Following tab, also show posts reposted by followed users

**Why this first:** Follow (iter 191) is useless without a Following feed. Repost (iter 193) is useless without surfacing in followers' feeds. This one feature activates both.

**Approach:** Add `ListFollowedIDs` store method. In the Feed handler, when `tab=following`, build a set of followed user IDs and filter posts client-side (post-query). Also query reposts by followed users and merge them into the timeline. Add tab links above the feed.

**From the spec:**
```
Action(label: "Following", style: if mode == "following" then "active"),
Action(label: "For You", style: if mode == "foryou" then "active"),
Action(label: "Trending", style: if mode == "trending" then "active")
```

**Scoping:** Ship "All" and "Following" tabs. "For You" and "Trending" require algorithmic ranking — Phase 3+.
