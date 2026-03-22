# Build Report — Iteration 46

## What was built

**Event-driven Mind** — replaced polling with handler-triggered auto-reply.

### Changes

1. **`graph/mind.go`** — rewritten. Removed `Run()` polling loop, `findUnreplied()` query, `maxAge`, `pollEvery`. Added `OnMessage(spaceID, slug, convo, sender)` — called by handlers when a message arrives. Added `findAgentParticipant()` to check if conversation has an agent.

2. **`graph/handlers.go`** — `Handlers` struct gets `mind *Mind` field + `SetMind()` setter. `handleOp` triggers `go h.mind.OnMessage(...)` after `respond` ops in conversations and after `converse` ops (new conversations).

3. **`cmd/site/main.go`** — removed polling goroutine, `context`, `signal`, `syscall` imports. Now just `graphHandlers.SetMind(mind)`.

4. **`graph/mind_test.go`** — updated tests for new API: `TestMindFindAgentParticipant` (3 cases), `TestMindOnMessage` (agent ignored, human without Claude), `TestMindE2E`.

### Net result

- 168 insertions, 426 deletions (net -258 lines)
- Simpler architecture: handler → Mind → Claude → insert
- No background goroutine, no polling, no staleness guard
- Reply happens immediately when human messages
