# Build Report — Iteration 194

## Following Feed Tab

**Store:**
- `ListFollowedIDs(userID) []string` — IDs of users the current user follows
- `ListRepostedNodeIDs(userIDs, limit) []string` — node IDs reposted by any of the given users

**Handler:**
- Feed handler reads `?tab=following` query param
- When following: builds follow set + repost set, filters posts to those by followed authors OR reposted by followed users
- Passes `feedTab` to FeedView

**Template:**
- `FeedView`: accepts `feedTab string`
- Tab pills: All / Following — above search bar, only for logged-in users
- Search form preserves tab via hidden input
- Following-specific empty state: "No posts from people you follow" with guidance to follow users
- Tabs match existing DM/Group filter pill pattern (brand/10 active, edge inactive)

**Composition:** This makes Follow (iter 191) and Repost (iter 193) actually work together. Following someone now changes what you see. Reposting a post surfaces it to followers.

**Files changed:**
- `graph/store.go` — `ListFollowedIDs`, `ListRepostedNodeIDs`
- `graph/handlers.go` — feed handler tab filtering
- `graph/views.templ` — FeedView tabs, empty state
