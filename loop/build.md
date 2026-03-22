# Build Report — Iteration 43

## What was built

**Server-side auto-reply (Mind)** — a background goroutine in the site server that automatically responds to conversations where an agent is a participant.

### Files changed

1. **`graph/mind.go`** (new, ~250 lines) — The Mind:
   - `NewMind(db, store, claudeToken)` — constructor
   - `Run(ctx)` — polling loop, 10-second interval, 5-second startup delay
   - `findUnreplied()` — SQL query: finds conversations where an agent is a participant and the most recent message is not from that agent
   - `replyTo()` — builds conversation context, calls Claude, inserts response node + records op
   - `callClaude()` — raw HTTP POST to Anthropic Messages API with OAuth token, no SDK dependency
   - Soul prompt matches the existing `cmd/reply` soul

2. **`cmd/site/main.go`** (edited) — Starts Mind goroutine when `CLAUDE_CODE_OAUTH_TOKEN` env var is set. Gracefully shuts down on SIGINT/SIGTERM.

### Architecture decisions

- **DB-direct, not API round-trip** — Mind queries the DB directly for unreplied conversations and inserts responses. No HTTP overhead, no auth token needed for the site's own API.
- **Raw HTTP to Claude API** — No Anthropic Go SDK import. Just `net/http` + `encoding/json`. Keeps the site's dependency tree clean.
- **OAuth token** — Uses the Claude Code OAuth token (fixed-cost, Max plan). Set as Fly secret `CLAUDE_CODE_OAUTH_TOKEN`.
- **10-second poll** — Fast enough for conversational feel, cheap enough (just a SQL query).
- **Graceful with auto-stop** — Works with `min_machines_running = 0`. Mind runs while the machine is awake (which is whenever someone is on the site). When the machine auto-stops, Mind stops. This is fine: the Mind only needs to be awake when someone is chatting.

### Deployed

- `flyctl deploy --remote-only` ✓
- `flyctl secrets set CLAUDE_CODE_OAUTH_TOKEN=...` ✓
- Verified in logs: `mind enabled (CLAUDE_CODE_OAUTH_TOKEN set)` and `mind: started (polling every 10s)` ✓
- No errors from DB polling ✓
