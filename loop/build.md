# Build Report — Iteration 23

## What I planned

Build a browser-based API key management page so users can create and revoke API keys without crafting curl commands.

## What I built

Changes to 5 files in the site repo: `graph/views.templ`, `graph/handlers.go`, `auth/auth.go`, `cmd/site/main.go`, plus generated `graph/views_templ.go`.

### API Keys page (`/app/keys`)
- Full standalone page using simpleHeader/simpleFooter pattern (like SpaceIndex)
- Lists existing keys: name, created date, revoke button per key
- Create form with name input
- Usage instructions with curl example
- Empty state when no keys exist

### HTMX create flow
- Create form uses `hx-post="/auth/api-keys"` with `hx-target="#key-result"`
- Modified `handleCreateAPIKey` in auth.go to detect HTMX requests
- Returns HTML fragment showing the raw key with "Save this — you won't see it again" warning
- JSON response preserved for API clients (unchanged behavior)

### Navigation
- "API Keys" link added to SpaceIndex page (top right)
- Delete redirects back to `/app/keys` (was `/app`)

### Wiring
- Route registered in main.go inside the auth block (where authService is accessible)
- Handler calls `authService.ListAPIKeys`, maps to `graph.ViewAPIKey`, renders templ view
- `ViewAPIKey` type added to graph/handlers.go (follows ViewUser pattern)

## Verification

- `templ generate` — 7 updates
- `go build -o /tmp/site.exe ./cmd/site/` — success
- Committed and pushed to main
- Deployed to Fly.io — both machines healthy
