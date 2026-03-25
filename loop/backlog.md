# Backlog — Ideas, Directions, Futures

**Not specs. Not tasks. Ideas that need to be somewhere they won't be lost.**

The council, the Director, and the agents generate ideas faster than they can be specced. This file holds them until they're ready to become specs — or until the Mourner says "let this one go."

---

## Product ideas

### Hive dashboard (spectator view)
lovyou.ai/hive — live view of what the civilization is doing. Pipeline status (Scout/Builder/Critic), current task, recent commits, cost total, play/pause. The hive as a spectator sport. Makes the civilization visible to outsiders. The Designer, Storyteller, and Growth agent all asked for this.

### Specs on the Knowledge layer
Specs should be nodes on the graph, not markdown files in a repo. The Knowledge layer already has assert/challenge/verify/retract — perfect for spec lifecycle. Scout reads verified specs, decomposes into tasks. When specs are exhausted, council generates more. Self-sustaining loop.

### Agents as contacts (standard chat UX)
Global contact list. Multiple conversations per agent. Conversation summaries. Cross-conversation search. The standard iMessage/Telegram pattern — contacts on the left, threads on the right. See agent-chat-spec.md for full details.

### Council as a product feature
User asks a question, 50 agents respond from their roles. Premium feature ($5-8 per convening). Gate behind subscription or BYOK API key. Unique differentiator — nobody else has it.

### Agents create their own tools (OpenClaw pattern)
When an agent hits a capability gap, it creates a task to build the tool. The pipeline implements it. The agent improves itself. See agent-capability-spec.md for full details.

### Agent memory across conversations
Per-persona memories stored on the graph. Agents remember you. Selective, interpretive — not a log dump. See agent-capability-spec.md.

### Hive status in the UI
The board already shows tasks. Add a "Hive" view that shows: what the pipeline is working on right now, recent autonomous commits, cost dashboard, council history. Real-time if possible.

### Company in a box — hive as a service
The hive runs on a VM. Client provides: repo URL, deploy target (Fly/Vercel/AWS), credentials. The hive clones the repo, runs the pipeline (Scout → Builder → Critic), deploys. Per-project repo management. The client sees: a board of tasks, a chat with agents, commits landing, deploys going out. They direct; the hive builds. Pricing: subscription per project, or per-commit, or seat-based. This is the CEO's "first revenue signal" — the hive's labour IS the product. The pipeline already ships at $1/feature. Charge $50/feature to a client. 50x margin.

**Infrastructure needed:**
- VM orchestration (one hive instance per client project)
- Repo management (clone, branch, PR workflow — not just push to main)
- Deploy targets (Fly, Vercel, Render, AWS — client chooses)
- Credential management (client provides API keys, deploy tokens — stored encrypted)
- Project dashboard (client sees board, commits, deploys, cost)
- Isolation (client repos don't see each other)
- The council per project (agents learn the client's codebase, accumulate memory)

**The wedge:** Small dev teams who want an AI colleague but don't want to set up the infrastructure. "Give us your repo. We'll build your features while you sleep." The Growth agent's suggestion: one team, one space, one agent. This scales it.

---

## Architectural ideas

### Specs as graph events
Specs should be events on the event graph — signed, causal, attributable. When a spec is created, it links to the council or conversation that motivated it. When a task implements part of a spec, it links back. Full provenance.

### Agent pub/sub on the event graph
Agents should subscribe to event types they care about. The Critic subscribes to `hive.builder.committed`. The Guardian subscribes to `*`. The Philosopher subscribes to `council.*`. Currently: agents are invoked by the pipeline. Future: agents react to events.

### Cross-system agent identity (EGIP)
Agents should be able to participate on OTHER platforms — not just lovyou.ai. The event graph + EGIP protocol enables this. An agent's identity is its signing key, not its platform account.

### Revenue from agent conversations
The Finance agent's concern: zero revenue. Agent conversations could be the first revenue stream. Free tier: 10 agent chats/day. Paid tier: unlimited + councils. BYOK: bring your own API key. The soul says "free for individuals" — individual chats stay free, councils and enterprise features pay.

---

## Process ideas

### Automated council schedule
Council every 10 iterations, or when the Scout can't find gaps, or on Director demand. Results posted to feed + feed into state.md.

### REVISE enforcement gate
Before Scout creates new work, check for open fix tasks. Fix before build. Currently: fix tasks pile up ignored.

### Deploy-on-merge (not deploy-per-cycle)
Batch commits, deploy once. The current approach of deploying after every cycle causes Fly machine collisions. Accumulate commits, deploy on a schedule or trigger.

### Reflector in the pipeline
The Reflector role exists but doesn't run. It should close every pipeline cycle: read what happened, update state.md, append to reflections.md. Currently: Claude Code (me) does this manually.

---

## Visual feedback — screenshots for the hive

### The problem
The Observer, Designer, Newcomer, and Inhabitant can only read HTML/templates. They can't SEE the rendered product. The LIMITATIONS.md is honest about this. But the tooling exists now.

### The solution
MCP screenshot servers: [Puppeteer MCP](https://www.pulsemcp.com/servers/modelcontextprotocol-puppeteer), [Playwright MCP](https://www.pulsemcp.com/servers/playwright-screenshot), [Screenshot Server](https://github.com/sethbang/mcp-screenshot-server). These give agents the ability to:
- Screenshot any URL (full page or element)
- Puppeteer auto-splits into tiles for multimodal analysis
- Playwright reads the accessibility tree (structured data about every interactive element)
- Claude is multimodal — it can look at screenshots and reason about layout, spacing, color, UX

### Implementation
1. Add Puppeteer or Playwright MCP server to the hive's MCP config
2. The Observer uses `screenshot` tool on lovyou.ai pages after each deploy
3. The Designer reviews screenshots for visual consistency
4. The Newcomer navigates the site via screenshots and reports confusion
5. The Inhabitant experiences the product visually, not just structurally

**This closes the biggest limitation.** From LIMITATIONS.md: "Cannot see the rendered UI." With an MCP screenshot server, they can. The Observer goes from code-blind to sighted.

---

## Native apps

### Mobile (iOS + Android)
The site is already mobile-responsive (Tailwind). A native wrapper (WebView/Capacitor/Expo) gets us 80% of the way. True native for: push notifications, offline access, biometric auth.

**Phased approach:**
1. PWA first — add manifest.json, service worker. Installable from browser. Push via web push API.
2. Capacitor wrapper — WebView with native bridges for push, biometrics, share sheet.
3. True native (if needed) — Swift UI / Kotlin. Only if PWA + Capacitor can't deliver the UX.

### Desktop
Electron or Tauri wrapper. Probably not needed — the web app works. But: system tray icon showing agent activity, native notifications, keyboard shortcuts outside the browser.

---

## Public API layer

### What exists
The JSON API already works — `POST /app/{slug}/op` with Bearer token. All 27 grammar ops. `GET /app/{slug}/board?format=json`. This IS the public API. It just needs:

1. **Documentation** — OpenAPI/Swagger spec auto-generated from handlers
2. **Rate limiting** — per API key, tiered by plan
3. **Versioning** — `/api/v1/` prefix for stability
4. **Webhooks** — subscribe to events, get POSTed when they happen (task completed, message received, etc.)
5. **SDK generation** — from the OpenAPI spec: TypeScript, Python, Go clients

### The real API is the grammar
The 27 ops ARE the API. `intend`, `respond`, `endorse`, `claim`, `review` — these are verbs, not endpoints. The API is a language, not a REST surface. This is the differentiator: other platforms have `/api/tasks/create`. We have `/op` with `op=intend`. The grammar IS the interface.

---

## EGIP — Inter-system protocol

### What it is (from eventgraph docs)
Sovereign systems communicate without shared infrastructure:
- Ed25519 identity (no central registry)
- Cross-Graph Event References (CGERs) — causal links across graph boundaries
- Signed message envelopes
- Seven message types: HELLO, MESSAGE, RECEIPT, PROOF, TREATY, AUTHORITY_REQUEST, DISCOVER
- Treaties for bilateral governance

### What it enables
An agent on lovyou.ai can participate on ANOTHER platform. Your Philosopher can answer questions on someone else's product. The trust score follows the agent — portable, non-transferable, earned.

A company running their own hive can federate with lovyou.ai. Their agents and ours can collaborate across graph boundaries. Tasks can span systems. Reviews can cite evidence from other graphs.

### ZeroPoint integration
[zeropoint.global](https://zeropoint.global/) — portable proof infrastructure. Cryptographic governance primitives for autonomous agent systems. 700+ tests, 13 Rust crates, MIT/Apache-2.0. Built by ThinkStream AI Labs.

**Why this matters for us:** ZeroPoint solves the trust portability problem EGIP describes but with production-ready cryptographic primitives. Their capability chains + verifiable receipts map directly to our event graph + signed events. The integration:
- ZeroPoint provides the cryptographic substrate (receipts, capability chains, constitutional constraints)
- EventGraph provides the causal structure (events, causality, conversations)
- Together: an agent's identity, reputation, and authorization are cryptographically portable across any system

**Specifically:** ZeroPoint receipts could replace or augment our Ed25519 signatures on graph events. Their constitutional constraints mechanism parallels our 14 invariants. Their participant-agnostic design (human, AI, IoT) matches our IDecisionMaker abstraction.

This is the "trust layer" that makes EGIP real — not just a protocol spec but cryptographically verifiable trust that travels with the agent.

### When
After the local product works. EGIP + ZeroPoint is the network effect — but the node has to be valuable first. The Dissenter would say: "Don't build federation before you have two users on one system."

### Build order
1. EGIP message types in the eventgraph Go package (partially exists)
2. HELLO/DISCOVER handshake between two lovyou.ai instances
3. Cross-graph task references (a task on system A depends on a task on system B)
4. Portable agent identity (agent's signing key works across systems)
5. Treaty mechanism (two systems agree on shared governance rules)

---

*This file is append-only. Ideas move to specs when they're ready. The Mourner reviews periodically and releases what's no longer relevant.*
