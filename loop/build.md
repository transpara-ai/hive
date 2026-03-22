# Build Report — Iteration 34

## What Was Planned

HTMX polling for live conversation updates — new messages appear without page reload.

## What Was Built

**site/graph/store.go**:
- Added `After *time.Time` field to `ListNodesParams` — filters nodes created after a given timestamp
- Applied in `ListNodes` query builder as `AND n.created_at > $N`

**site/graph/handlers.go**:
- New handler `handleConversationMessages` — `GET /app/{slug}/conversation/{id}/messages?after=RFC3339Nano`
- Returns only new messages since the given timestamp
- HTMX requests get `chatMessage` HTML fragments; JSON requests get `{"messages": [...]}`
- Returns empty 200 if no `after` param (no-op for first poll before any messages)
- Registered at `/app/{slug}/conversation/{id}/messages` with readWrap (public space compatible)

**site/graph/views.templ**:
- Messages container gets `id="message-list"` and `data-last-ts` tracking attribute
- Each `chatMessage` gets `data-ts` with RFC3339Nano timestamp for deduplication
- Hidden `#poll` div with `hx-trigger="every 3s"` polls the new endpoint
- Poll reads `after` from `data-last-ts`, appends new messages, updates timestamp, auto-scrolls if near bottom
- Send form updated: targets `#message-list`, updates `data-last-ts` after successful send, removes empty state
- Empty state gets `id="empty-state"` so it can be removed when first message arrives

## Key Design Decisions

1. **HTMX polling, not SSE/WebSocket**: Simplest approach. 3-second interval is fast enough for human-agent conversation. No server-side infrastructure (connection tracking, event broadcasting). Just a GET that returns HTML fragments.

2. **Timestamp-based deduplication**: The `data-last-ts` attribute on the message list tracks the latest message. Both the poll handler and the send handler update it. The server only returns messages with `created_at >` the tracked timestamp, so no duplicates.

3. **Auto-scroll only when near bottom**: If the user has scrolled up to read history, new messages don't yank them down. Only auto-scrolls when within 100px of the bottom.

4. **Reuse existing infrastructure**: `ListNodes` with a new `After` filter, existing `chatMessage` component, existing `readWrap` middleware. No new store methods, no new templates, no new middleware.

## Verification

- `templ generate` — clean
- `go build ./...` — clean
- Deployed to Fly.io — healthy
- Polling endpoint returns empty HTML for no new messages (no unnecessary DOM updates)
- Send form still works via HTMX (tested via code review of hx-target change from `#messages .max-w-2xl` to `#message-list`)

## Files Changed

- `site/graph/store.go` — 5 lines (After field + query filter)
- `site/graph/handlers.go` — ~50 lines (new handler + route + time import)
- `site/graph/views.templ` — ~20 lines (polling div, data attributes, JS handlers)
