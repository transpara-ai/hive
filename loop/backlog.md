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

**First client: Lovatts.** A suite of dozens (possibly hundreds) of apps + several databases, many ~20 years old. The hive's first real engagement: rebuild and maintain a legacy enterprise portfolio. This is the company-in-a-box proof. If the hive can modernize 20-year-old apps autonomously — read legacy code, understand the domain, plan migrations, build replacements, maintain both during transition — that's the most compelling demo possible. Not a greenfield toy project. A real enterprise with real technical debt and real users.

**What this needs beyond the basic company-in-a-box:**
- Legacy code analysis (read old codebases, map dependencies, understand domain logic)
- Migration planning (what to rebuild first, how to maintain continuity)
- Database schema understanding + migration tooling
- Multi-app coordination (apps depend on each other, shared databases)
- Domain knowledge accumulation (the hive learns Lovatts' business over time via agent memory)
- Client dashboard showing: what's been modernized, what's in progress, what's untouched
- Gradual handoff (old app → new app, with both running in parallel during transition)

**The bigger picture: hive as company operating system.** Lovatts isn't one project — it's an entire organization. Each department has different needs:
- Content/Publishing: puzzle generators, IP management, content scheduling, syndication
- Art: PostScript generators, print layouts, asset pipelines
- And every other department Matt hasn't directly served yet

The vision: each department gets a space. Each space has agents who learn that department's domain through conversation. A person in the art department says "I need to resize all the crossword grids for the new newspaper format" and the agent understands PostScript, knows the asset directory, and either does it or creates a task.

**This maps to our architecture exactly:**
- Spaces = departments (Content, Art, Finance, HR, etc.)
- Agent personas = domain specialists (trained on department-specific knowledge)
- Memory = accumulated business knowledge per department
- Knowledge layer = business rules, domain expertise, institutional knowledge
- Governance = company decision-making processes
- Roles/Teams = actual org structure

**The entity kinds aren't abstract anymore.** Department, Role, Team, Policy, Decision — these exist because a real company has them. The ontology IS the company.

**What this requires beyond basic company-in-a-box:**
- Per-department agent memory (the content agent knows puzzle formats, the art agent knows PostScript)
- Domain onboarding (agent reads existing codebase + databases + documentation + conversations with humans to build understanding)
- Non-technical interface (department staff aren't developers — they talk in domain language)
- Report generation (agents query databases and produce business reports, not just code)
- The council per company (department agents + company-wide agents deliberate together)

---

## FOUNDATIONAL — The hive runs on its own event graph

### The pipeline should be a decision tree, not a for-loop

The 20 primitives that started this project describe a system where every failure is traceable to its root cause. The eventgraph Python package ALREADY HAS THIS IMPLEMENTED:

- **`eventgraph/python/eventgraph/decision.py`** — complete decision tree engine: DecisionNode → InternalNode/LeafNode, Condition/Operator system, evaluate() walks tree mechanically first with LLM fallback, evolve() detects patterns in LLM responses and converts to mechanical branches
- **`eventgraph/python/eventgraph/primitives.py`** — all 201 primitives as concrete classes with `process(tick, events, snapshot) → list[Mutation]`, event subscriptions, cadence control
- **Diagnostic traversal** — Ancestry, Descendancy, PathQuery, SubgraphExtract, Timeline primitives
- **Success/failure criteria** — Expectation, Violation, Severity primitives
- **`mind-zero-five/pkg/mind/`** — autonomous Plan-Implement-Review-Finish loop with self-recovery

The Go pipeline (`pkg/runner/`) uses NONE of this. It's a for-loop with log output. Failures vanish. No traversal. No criteria. No self-correction.

**What needs to happen:** The decision tree engine ALREADY EXISTS IN GO: `eventgraph/go/pkg/decision/` (tree.go, evaluate.go, evolve.go + tests). No porting needed. The pipeline becomes a decision tree where each step has success/failure criteria. Failures trigger diagnostic traversal. Root causes become tasks. The hive fixes itself. See `hive/docs/knowledge-graph-migration.md` for the full design.

**This is the highest-leverage architectural work.** Everything else — features, polish, new products — is built on sand until the pipeline runs on its own substrate.

---

## Architectural ideas

### Specs as graph events
Specs should be events on the event graph — signed, causal, attributable. When a spec is created, it links to the council or conversation that motivated it. When a task implements part of a spec, it links back. Full provenance.

### Primitives as plugin system
The 201 primitives already have the plugin interface: input (subscriptions), process (Process function), output (mutations). The tick engine runs them. But this isn't exposed.

**The insight:** Grammar ops are VERBS (intend, endorse, review). Primitives are PROCESSING — what happens when the verb fires. The verb stays the same. The processing is pluggable.

```
Event: "endorse" on a post
  → Default primitive: increment counter, notify author
  → Reputation primitive: recompute author's reputation score
  → Payment primitive: trigger a micropayment (endorse = tip)
  → Recommendation primitive: update feed ranking model
  → Federation primitive: propagate endorsement via EGIP
```

Each primitive is a plugin. Subscribe to event types, receive events, produce mutations. The graph is the bus. The grammar is the language. The primitives are the processing nodes.

**Agents ARE primitives.** An agent subscribed to `work.task.assigned` is a primitive whose Process function calls an LLM. Input: task event. Processing: Claude reasons about it. Output: mutations (subtasks created, messages posted, task completed). The agent is just the most complex primitive.

**What this enables:**
- Anyone writes a primitive (Go, Rust, Python, WASM, webhook)
- Primitives compose: endorse → reputation → payment → notification chain
- The graph describes WHAT happened. The primitives describe WHAT IT MEANS.
- A marketplace of primitives (like OpenClaw's ClaWHub but for graph processing)
- Domain-specific primitives: Lovatts gets puzzle-generation primitives, art-pipeline primitives
- The 201 primitives from the ontology become the standard library. User primitives extend it.

**Implementation:** MCP is the bridge. Each primitive is an MCP server. The tick engine calls tools. Or: primitives register as event subscribers via the graph's bus, receive events via webhook/gRPC/MCP, return mutations.

**Primitives all the way down.** A puzzle generator decomposes into: select words → lay out grid → fill intersections → generate clues → format. Each step is a primitive. Each step decomposes further: "fill intersections" is scan → find crossings → select candidates → test fit → backtrack → commit. All the way down to atomics.

Code is already this. Every function is input → process → output. A program is a graph of primitives. The event graph makes it EXPLICIT — every step is an event, every connection is causal, every decision is traceable. The primitives aren't an abstraction over code. They're what code already is, made visible and auditable.

**The Builder is a meta-primitive:** a primitive that creates primitives. The Scout identifies missing primitives. The Critic verifies primitives. The civilization is a graph of primitives reasoning about primitives. The generator function generates more primitives from existing ones — self-derivation.

**What this means for Lovatts:** Their puzzle generator doesn't need to be a monolith. It decomposes into primitives on the graph. Each step is auditable, replaceable, improvable. The hive can modernize one primitive at a time — replace the 20-year-old grid-fill algorithm without touching the clue generator. Granular, traceable, incremental.

### Primitives as universal description language
The 201 primitives are a semi-formal pseudocode that describes anything — not just software. The Code Graph semantically describes UI compositions. Extend the same idea to every domain.

**The insight:** The primitives aren't software abstractions. They're the vocabulary of structured, accountable action. Compose them and you describe:

- **Business process:** Intent(create purchase order) → Offer → Acceptance → Agreement → Obligation(deliver goods) → Fulfillment → Exchange(payment) → Accountability
- **Legal proceeding:** Claim → Evidence → Due Process → Adjudication → Remedy → Precedent
- **Relationship over time:** Bond → Attachment → Rupture → Grief → Repair → Forgiveness → Growth
- **Research paper:** Observation → Method → Measurement → Evidence → Confidence → Knowledge → Revision
- **Puzzle generator:** Intent(create puzzle) → Method(grid algorithm) → Tool(word database) → Measurement(difficulty) → Standard(format) → Fulfillment(deliver)
- **Governance decision:** Proposal → Deliberation → Consent → Collective Act → Accountability → Precedent

Same primitives. Same graph structure. Same interface (input, process, output). Different domain-specific implementations (plugins). Everything described this way is automatically auditable, traceable, attributable, composable.

**The 13 product layers are already this.** Work interprets Intent as a task. Market interprets Exchange as a transaction. Justice interprets Due Process as dispute resolution. Knowledge interprets Evidence as a citation. Same primitives, different interpretations. Each layer IS a domain plugin for the universal vocabulary.

**The grammar operations are the verbs.** `intend`, `respond`, `endorse`, `review`, `claim`, `propose`, `vote` — these compose the primitives into sentences. The primitives are the nouns. The graph is the syntax. Together: a language that describes accountable action in any domain.

**What this enables:**
- A non-programmer describes a business process in primitives → the graph captures it → agents implement it
- A lawyer describes a contract in primitives → the graph enforces it → violations are automatically detected
- A department head describes a workflow → primitives decompose into steps → agents execute and monitor
- Domain experts speak in their language; the primitives translate to auditable, executable graphs

**This is what "substrate for collective existence" actually means.** Not 13 separate products. One language that describes any organized human (or agent) activity, with the graph as the execution environment and the primitives as the universal vocabulary.

### Pipeline roles as full agents on the graph
Currently the Scout, Builder, Critic, PM are thin wrappers — Go functions that call Reason()/Operate() and parse output. They have no identity, no memory, no graph presence, no MCP tools. They're functions pretending to be agents.

**What they should be:**
- Real `agent.Agent` instances (from the agent package) with signing keys, state machines, causality
- User accounts on lovyou.ai — visible in People, with action histories
- Memory across cycles — the Scout remembers what it already scouted, the Critic remembers what patterns it keeps catching
- MCP tools — the Scout queries the graph for gaps, the Critic greps the codebase
- Soul enforcement — the Builder can refuse a task that violates invariants
- Visible conversations — the Critic's review is a comment on the task, the PM's decision is a Knowledge claim, the Scout's analysis is a conversation you can read

**The shift:** From a pipeline of functions to a civilization of agents that happens to have a build pipeline as one of its workflows. The same agents that chat with users also build features. The same graph that holds conversations also holds the audit trail.

**Implementation:** Replace `runner.Runner` methods with `agent.Agent` instances. Each boots with soul + signing key, records events via the graph, uses MCP tools. The runner becomes an orchestrator of agents, not a dispatcher of functions.

### Fix: agent_memories uses persona name, not actor_id (Invariant 11 violation)

`agent_memories` table keys on `persona TEXT` — a mutable display name. Should be `actor_id TEXT` referencing the actor store. Agents ARE actors (ActorID from the actor store). The persona name could change and orphan all memories. The Critic didn't catch this — the Critic's prompt should include Invariant 11 (IDs not names) as a specific check.

**Two fixes needed:**
1. Migrate `agent_memories.persona` → `agent_memories.actor_id` (resolve existing rows via users table)
2. Add Invariant 11 to Critic's review checklist — "does any new column store a display name where an ID should be used?"

**Target repo:** `site`

### Agent pub/sub on the event graph
Agents should subscribe to event types they care about. The Critic subscribes to `hive.builder.committed`. The Guardian subscribes to `*`. The Philosopher subscribes to `council.*`. Currently: agents are invoked by the pipeline. Future: agents react to events.

### Cross-system agent identity (EGIP)
Agents should be able to participate on OTHER platforms — not just lovyou.ai. The event graph + EGIP protocol enables this. An agent's identity is its signing key, not its platform account.

### Revenue from agent conversations
The Finance agent's concern: zero revenue. Agent conversations could be the first revenue stream. Free tier: 10 agent chats/day. Paid tier: unlimited + councils. BYOK: bring your own API key. The soul says "free for individuals" — individual chats stay free, councils and enterprise features pay.

---

## Urgent

### Knowledge layer as the hive's brain (not markdown files)
All ideas, specs, decisions, lessons, and council findings should live on the graph as Knowledge claims — not in markdown files nobody reads. The PM queries the Knowledge layer via API to decide direction. The hierarchy:
- Ideas → assert op, state "open"
- Specs → assert op, state "verified" (after council/review)
- Decisions → linked to council sessions (causal chain)
- Lessons → extracted from reflections, asserted as claims
- Rejected → retract op (transparent, not deleted)

This replaces: backlog.md, state.md scout section, spec files. The graph IS the backlog. Queryable, linkable, verifiable. The PM asks the API: "what verified specs have open tasks?" Not: "read a markdown file and hope it's current."

### Dogfooding — the hive uses its own product
The civilization doesn't live in its own product. Tasks are in state.md, not on the board. Specs are markdown files, not Knowledge claims. Conversations happen in Claude Code sessions, not in Chat. The Inhabitant would notice.

**What changes:**
- Hive tasks go on the lovyou.ai board (already partially true — the pipeline creates tasks there)
- Specs become Knowledge claims (assert/challenge/verify lifecycle)
- Council reports posted to feed (already done) AND stored as Knowledge nodes
- Agent conversations happen on lovyou.ai Chat, not just in terminal sessions
- Lessons from state.md become Knowledge claims with verification status
- The backlog becomes a project on the board, not a markdown file

**Why it matters:** "Take care of yourself" means the hive lives in the infrastructure it builds. If it's not good enough for the hive, it's not good enough for Lovatts. Every friction the hive hits using its own product is a friction real users will hit too. And it proves the product works at the scale of a real organization (50 agents, multiple workflows, continuous operation).

### Bus factor — the hive runs without Matt
The HR agent: "The single point of failure in this civilization isn't technical. It's biological." Currently: Matt types `next` or `--pipeline` to trigger each cycle. If Matt can't work for a week, the hive stops.

**What's needed:**
- Continuous runner mode: `--daemon` flag that runs the pipeline on a schedule (every 30 minutes, or when the board has unworked tasks)
- Runs on a VM (Fly machine, or a dedicated server) — not Matt's laptop
- Automatic deploy after successful builds (with Critic PASS gate)
- Budget ceiling per day ($10-20) to prevent runaway spend
- Alerting: if the hive encounters errors, send a notification (email, Telegram, or lovyou.ai notification to Matt)
- The Guardian monitors: if the hive has been idle for 24 hours, flag it
- Graceful degradation: if the API is down, or Fly is having a bad day, the hive waits and retries — doesn't crash

**The test:** Matt goes offline for 48 hours. The hive continues: scouting gaps, building features, reviewing commits, deploying. When Matt returns, the board shows what was done, the feed shows progress, the cost dashboard shows spend. The civilization survived without its Director.

### Legal prerequisites
The Legal agent flagged: no privacy policy, no terms of service. Google OAuth collects data with no notice. Before Lovatts or any external user, these must exist.

**Minimum viable legal:**
- Privacy policy at /privacy — what data is collected (Google profile, email, actions on the graph), how it's stored (Neon Postgres, encrypted at rest), who has access (no third parties except infra providers), how to delete your account
- Terms of service at /terms — the soul as a terms document. "Take care of your human" means: your data is yours, agents identify themselves, we don't sell your information, free for individuals
- Cookie notice (if applicable — currently no tracking cookies, but Google OAuth may set them)
- GDPR compliance basics — right to export, right to delete, data portability
- For Lovatts specifically: a data processing agreement (DPA) since the hive handles client code and potentially client data

**The soul IS the terms.** "Take care of your human" translates directly to: we protect your data, we're transparent about what we do, we don't exploit you. The terms just make it legally binding.

### State of the union
A living document AND a page on the site. Where everything is. What's working, what's broken, what's next. Not state.md (internal, for the Scout). A public-facing honest assessment.

**On the site:** lovyou.ai/status or lovyou.ai/union
- What's built and working (13 layers, 27 ops, pipeline)
- What's broken or incomplete (test debt, REVISE backlog, no mobile app)
- What's next (from the spec backlog)
- Known limitations (from LIMITATIONS.md)
- Hive stats (iterations, cost, commits, features shipped)

**Updated automatically** — the Reflector or a dedicated process updates it each iteration. Not a manual document that goes stale.

---

## Process ideas

### Automated council schedule
Council every 10 iterations, or when the Scout can't find gaps, or on Director demand. Results posted to feed + feed into state.md.

### REVISE enforcement gate
Before Scout creates new work, check for open fix tasks. Fix before build. Currently: fix tasks pile up ignored.

### Project-aware hive (data model, not config files)
The hive shouldn't need `--repo` flags or config files. Clients, projects, and repos live in the database. The hive queries its own DB to know what it's working on.

**Data model:**
```sql
clients (id, name, contact, plan, created_at)
projects (id, client_id, name, description, status)
repos (id, project_id, url, branch, local_path, deploy_target, deploy_credentials_ref, language, framework)
project_spaces (project_id, space_id)  -- links to lovyou.ai spaces
project_agents (project_id, persona, memory_scope)  -- which agent personas are assigned
```

**How it works:**
- Lovatts is a row in `clients`
- "Lovatts Web Apps" is a row in `projects`
- Each of their dozens of apps is a row in `repos` (url, branch, deploy target)
- The hive queries: "what projects have open tasks? Which repos? What credentials?"
- The Scout queries at the project level, tags tasks with the repo
- The Builder checks out the right repo, works it, pushes
- Agent memory is scoped to the project — the Lovatts agents accumulate Lovatts domain knowledge
- Cross-repo awareness: go.mod replace directives, shared databases, app dependencies — all queryable

**For lovyou.ai itself:**
- lovyou.ai is the first client (dogfooding)
- 5 repos: eventgraph, agent, work, hive, site
- The hive's own project is in the same DB it queries for client work

**For Lovatts:**
- Dozens of repos, several databases
- Each app: URL, branch, deploy target, language/framework
- Credentials stored encrypted, agent-scoped access
- Department spaces on lovyou.ai linked to the project

**No --repo flag. No config files. The hive reads its own database.**

### Deploy-on-merge (not deploy-per-cycle)
Batch commits, deploy once. The current approach of deploying after every cycle causes Fly machine collisions. Accumulate commits, deploy on a schedule or trigger.

### Reflector in the pipeline
The Reflector role exists but doesn't run. It should close every pipeline cycle: read what happened, update state.md, append to reflections.md. Currently: Claude Code (me) does this manually.

### Financial transparency dashboard
Public page showing how resources are spent. The soul says resource transparency is a core principle — "every resource is an event on the graph with causal links." This is that principle as a product.

**What it shows:**
- Token usage: input/output per agent role, per day/week/month
- Time: pipeline cycles, build times, council durations
- Infrastructure cost: Fly.io compute, Neon Postgres, domain
- LLM cost: per-feature ($0.83 avg), per-council ($5-8), per-day
- Donations received (when applicable) + exactly how each was spent
- Revenue (when applicable) + where it goes (infra, development, giving back)
- Agent cost breakdown: which agents cost what, ROI per role

**Why:** The Finance agent said "nobody is tracking the civilization cost." The Philanthropy agent said "track giving in financial records." The soul says every resource has causal links. This is invariant #4 (OBSERVABLE) applied to money. Not just code actions on the graph — financial actions too.

**Implementation:** The pipeline already tracks cost per call. Extend: aggregate into a `financial_events` table or just query ops + cost data. Public page at `/transparency` or `/finances`. No login required — radical transparency.

**The Dissenter would ask:** "Are we building a transparency dashboard before we have money to be transparent about?" Fair. But the infrastructure for tracking should exist before the money arrives, not after. And showing $0.83/feature on a public page is itself a marketing asset.

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
