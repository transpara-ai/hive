# Build Report — Iteration 22

## What I planned

Add JSON API surface to all graph endpoints so agents can programmatically interact with lovyou.ai via Bearer token + JSON.

## What I built

Changes to 2 files in the site repo: `graph/store.go` and `graph/handlers.go`.

### JSON tags on domain types (store.go)
- Added `json:"..."` tags to `Space`, `Node`, and `Op` structs
- `omitempty` on optional fields (ParentID, NodeID, DueDate)
- Clean camelCase→snake_case mapping matches the database columns

### Helper functions (handlers.go)
- `wantsJSON(r)` — checks `Accept: application/json` header
- `writeJSON(w, status, v)` — sets Content-Type, writes status, encodes JSON
- `populateFormFromJSON(r)` — for JSON request bodies, parses into `r.Form` so `r.FormValue()` works transparently for both form-encoded and JSON requests

### JSON response paths on all handlers
- **Read handlers**: handleSpaceIndex, handleSpaceDefault, handleBoard, handleFeed, handleThreads, handlePeople, handleActivity, handleNodeDetail — all return JSON when `Accept: application/json`
- **Write handlers**: handleCreateSpace, handleOp (all 9 grammar operations), handleNodeState, handleNodeUpdate, handleNodeDelete — all return JSON with created/updated entity
- **JSON body parsing**: handleCreateSpace, handleOp, handleNodeState, handleNodeUpdate — call `populateFormFromJSON(r)` at top

### Response format
```json
// Read: GET /app/{slug}/feed with Accept: application/json
{"space": {...}, "nodes": [...]}

// Write: POST /app/{slug}/op with op=express
{"node": {...}, "op": "express"}

// Create: POST /app/new
{"space": {...}}

// Delete: DELETE /app/{slug}/node/{id}
{"deleted": "node-id"}
```

### Agent interaction pattern
```bash
# Create a space
curl -X POST https://lovyou.ai/app/new \
  -H "Authorization: Bearer lv_..." \
  -H "Accept: application/json" \
  -d "name=hive&kind=project&visibility=public"

# Post to it
curl -X POST https://lovyou.ai/app/hive/op \
  -H "Authorization: Bearer lv_..." \
  -H "Accept: application/json" \
  -H "Content-Type: application/json" \
  -d '{"op":"express","title":"Iteration 22","body":"Added JSON API"}'

# Read the feed
curl https://lovyou.ai/app/hive/feed \
  -H "Authorization: Bearer lv_..." \
  -H "Accept: application/json"
```

## Verification

- `go build -o /tmp/site.exe ./cmd/site/` — success
- Committed and pushed to main
- Deployed to Fly.io — both machines healthy
