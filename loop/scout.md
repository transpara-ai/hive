# Scout Report — Iteration 44

## Gap: Mind has no safety guards

The Mind (iter 43) is deployed but vulnerable to three failure modes:

1. **Stale replies** — machine auto-stops (`min_machines_running = 0`), restarts hours later, Mind replies to old messages. User gets a reply to something they said yesterday.
2. **No timeout** — Claude CLI could hang forever, blocking the poll loop entirely.
3. **Concurrent floods** — if many conversations are unreplied, Mind processes them all at once, spawning multiple Claude CLI processes.

## What "Filled" Looks Like

- Messages older than 5 minutes are skipped (staleness guard)
- Claude CLI calls have a 2-minute timeout
- Conversations are processed one at a time
- After a failure, the Mind backs off instead of trying more conversations
