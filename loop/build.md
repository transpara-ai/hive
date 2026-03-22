# Build Report — Iteration 44

## What was built

**Mind hardening** — three safety guards added to `graph/mind.go`:

1. **Staleness guard** — `maxAge: 5 * time.Minute`. The `findUnreplied` query now returns `last_message_at` (MAX of child timestamps or conversation created_at). `poll()` skips conversations where the last message is older than 5 minutes. Logs the skip.

2. **Reply timeout** — `replyTimeout: 2 * time.Minute`. `replyTo()` wraps the Claude CLI call in a `context.WithTimeout`. If Claude hangs, the context cancels and the poll loop continues.

3. **Failure backoff** — `poll()` returns immediately after the first failed reply. Prevents cascading failures when Claude is down. The next poll (10 seconds later) will retry.

4. **Sequential processing** — conversations are processed one at a time (was already the case, but now with explicit ordering by `last_message_at DESC` — most recent first).

### Files changed

- `graph/mind.go` — 31 insertions, 8 deletions. No new dependencies.

### Deployed

- `flyctl deploy --remote-only` ✓
