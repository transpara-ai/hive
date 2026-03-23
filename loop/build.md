# Build Report — Iteration 127

## Activity context — node titles in activity feed and dashboard

### Changes

**store.go:**
- Added `NodeTitle string` to `Op` struct
- `ListOps`: LEFT JOIN nodes to get title, scan into NodeTitle
- `ListUserAgentActivity`: same JOIN + scan

**views.templ:**
- `opItem`: shows node title as clickable link after op type ("Matt **intend** Fix the login bug")
- `dashboardAgentRow`: shows node title instead of space name when available

### Impact
Activity lens and dashboard both now show WHAT happened, not just WHO did WHAT TYPE of thing.

### Deployed
`ship.sh`
