# Knowledge Graph Migration: From Files to Graph

## The Vision

Agents work in the dark. They get a flat context string — `LoadSharedContext()` concatenates a few files — and then reason without the ability to look anything up. No light switch. They can't say "I need to understand how Knowledge was built" and navigate from concept to commits. They can't say "what primitive does this map to?" and drill into the ontology.

The hive has multiple types of memory, each with different access patterns:

| Memory Type | What It Holds | Examples | Access Pattern |
|---|---|---|---|
| **Declarative** | What is true | Documents, specs, blog posts, reference | Read when building understanding |
| **Procedural** | What to do | Tasks, pipeline state, directives, goals | Read when deciding action |
| **Episodic** | What happened | Reflections, git log, conversations, councils | Read when learning from the past |
| **Semantic** | What things mean | 201 primitives, 13 grammars, ontology layers | Read when mapping concepts to architecture |

Currently ALL of these are either flat files (backlog.md, state.md, reflections.md) or invisible (blog, reference, primitives). Agents get a truncated subset shoved into their prompt. They can't explore. They can't drill. They can't follow a thread from "Council" → "Layer 3: Society" → "Collective Act primitive" → "the `propose` + `vote` grammar ops" → "how /app/{slug}/council should work."

### The Tree

Knowledge should be a hierarchical tree that agents navigate via MCP tools:

```
Root
├── Ontology (semantic memory — what things mean)
│   ├── Layer 0: Foundation (44 primitives)
│   │   ├── Event, EventStore, Clock, Hash, Self...
│   │   └── [drill: full definitions, derivations, relationships]
│   ├── Layer 1: Agency → Work grammar
│   │   ├── Grammar ops: intend, assign, complete, decompose...
│   │   └── Product: task management, board, projects
│   ├── Layer 6: Information → Knowledge grammar
│   │   ├── Grammar ops: assert, challenge, verify, ask...
│   │   └── Product: documents, Q&A, claims, agent auto-answer
│   └── ... (13 layers, 201 primitives, 13 grammars)
├── Architecture (declarative memory — how the system works)
│   ├── Site (Go + templ + HTMX + Tailwind, routes, handlers)
│   ├── Hive (pipeline, agents, runner, MCP)
│   ├── EventGraph (foundation library, stores, types)
│   └── Infrastructure (Fly.io, Postgres, auth, deploy)
├── Business (declarative memory — context and strategy)
│   ├── Lovatts (first client, legacy apps, department structure)
│   ├── Pricing (free individuals, paid corps, councils)
│   └── ZeroPoint (integration point)
├── History (episodic memory — what happened and why)
│   ├── Blog posts (derivation stories, the "why")
│   ├── Reflections (lessons per iteration, COVER/BLIND/ZOOM)
│   ├── Council records (deliberation transcripts, consensus)
│   └── Git history (changes, who, when)
└── Work (procedural memory — current state)
    ├── Active goals (PM directives)
    ├── Open tasks (board state)
    ├── Backlog (ideas waiting to become work)
    └── Completed (recent history, what's done)
```

### MCP Tools for Knowledge Navigation

Agents interact with the tree via MCP tools — the light switch:

```
knowledge.list_topics(parent?)      → top-level tree or subtopics
knowledge.drill(topic)              → children + summaries + detail level
knowledge.get(node_id)              → full content of a knowledge node
knowledge.search(query, type?)      → semantic search across all memory types
knowledge.context(topic, depth?)    → everything about a topic, N levels deep
knowledge.related(node_id)          → linked nodes (causal, parent, tag)
knowledge.recent(type?, limit?)     → latest entries of a memory type
```

The PM doesn't read a 6000-char file. It calls `knowledge.list_topics("Work")` → sees active goals, open tasks, backlog. It calls `knowledge.drill("Work/Active goals")` → sees the current directives. It calls `knowledge.context("Ontology/Layer 3", depth=2)` → understands what Society primitives are relevant to the Council feature.

The Scout doesn't parse markdown. It calls `knowledge.search("what's missing?", type="declarative")` and navigates the tree to find gaps.

The Builder calls `knowledge.context("Architecture/Site/handlers", depth=1)` to understand the codebase structure before implementing.

Every agent has the light switch. The depth of exploration is proportional to the agent's need — the PM drills shallow and wide, the Builder drills narrow and deep.

## The Flat File Problem (Current State)

The hive's institutional knowledge lives in `.md` files read/written with `os.ReadFile` and `os.WriteFile`. This is fragile:

- **PM reads `backlog.md`** as a 6000-char truncated string, does regex on `state.md` to replace directives. Just broke: duplicate section headers, PM couldn't see existing open tasks, redirected to a new priority while Council tasks were still active.
- **Scout reads `state.md`** scout section via string index. Reads target repo `CLAUDE.md` for context.
- **Architect reads `scout.md`** as a flat file to decompose into tasks.
- **Reflector appends to `reflections.md`** (unbounded growth) and does string replacement on `state.md` iteration counter.
- **LoadSharedContext()** concatenates `agents/CONTEXT.md` + lessons from `state.md` + directive from `state.md`.

Meanwhile, the site's Knowledge product (Documents, Questions, auto-answer) is live and graph-native. The hive should eat its own dog food.

## The Primitives

From Layer 6 (Information): **Symbol, Language, Encoding, Record, Channel, Copy, Noise, Redundancy, Data, Computation, Algorithm, Entropy.**

The hive's knowledge is currently Symbol without Record — ideas expressed in language but not recorded on the graph. No Channel (agents read files, not each other's published knowledge). No Redundancy (one copy, local filesystem). Full Noise (string parsing, truncation, regex).

The migration moves knowledge onto the graph: Symbol + Record + Channel. Each piece of knowledge becomes a node with ID, author, timestamp, causal links. Queryable. Versionable. Auditable.

## Entity Kind Mapping

| Current File | Graph Kind | How Created | How Queried |
|---|---|---|---|
| `backlog.md` | `document` (tagged `backlog`) | Human or PM creates via `intend` op | `GetDocuments(slug, tag="backlog")` |
| `state.md` directive | `goal` (state=active) | PM creates via `intend` op (kind=goal) | `GetGoals(slug, state="active")` |
| `state.md` lessons | `document` (tagged `lesson`) | Reflector creates via `intend` op | `GetDocuments(slug, tag="lesson")` |
| `scout.md` | `document` (tagged `scout-report`) | Scout creates via `intend` op | `GetDocuments(slug, tag="scout-report", limit=1)` |
| `build.md` | `document` (tagged `build-report`) | Builder creates via `intend` op | By ID (passed from pipeline) |
| `critique.md` | `document` (tagged `critique`) | Critic creates via `intend` op | By ID (passed from pipeline) |
| `reflections.md` | `document` (tagged `reflection`) | Reflector creates one per iteration | `GetDocuments(slug, tag="reflection", limit=5)` |
| `agents/CONTEXT.md` | `document` (tagged `context`, pinned) | Human creates once | `GetDocuments(slug, tag="context", pinned=true)` |
| `agents/{role}.md` | `document` (tagged `role-prompt:{role}`) | Human creates per role | `GetDocuments(slug, tag="role-prompt:scout")` |

## New API Methods

The hive client (`pkg/api/client.go`) needs these additions:

```go
// CreateDocument creates a document node in the space.
func (c *Client) CreateDocument(slug, title, body string, tags []string) (*Node, error)

// GetDocuments returns documents matching optional tag filter.
func (c *Client) GetDocuments(slug string, tag string, limit int) ([]Node, error)

// CreateGoal creates a goal node (active directive).
func (c *Client) CreateGoal(slug, title, body string) (*Node, error)

// GetActiveGoals returns goals in non-done state.
func (c *Client) GetActiveGoals(slug string) ([]Node, error)

// CompleteGoal marks a goal as done.
func (c *Client) CompleteGoal(slug, nodeID string) error

// GetNode returns a single node by ID.
func (c *Client) GetNode(slug, nodeID string) (*Node, error)
```

The site already supports all these via existing ops:
- `intend` with `kind=document` or `kind=goal` creates nodes
- `edit` updates them
- `complete` closes them
- Board/knowledge endpoints return them with JSON Accept header

What's needed on the site: a `GET /app/{slug}/documents?tag=X&limit=N` endpoint that filters by tag. The knowledge handler already lists documents; it just needs tag filtering.

## Runner Migration (Role by Role)

### PM (pm.go)

**Before:**
```go
backlog := r.readBacklog()                    // os.ReadFile("loop/backlog.md")[:6000]
currentDirective := r.readScoutSection()       // string index on state.md
completedWork := r.completedTasksSummary()     // API call (already graph-native)
// ... LLM decides ...
r.updateScoutDirective(directive)              // string replace on state.md
```

**After:**
```go
backlog := r.getDocuments("backlog", 5)        // API: tagged documents
activeGoals := r.getActiveGoals()              // API: open goal nodes
completedWork := r.completedTasksSummary()     // unchanged (already graph-native)
// ... LLM decides ...
r.completeGoal(oldGoalID)                      // API: close old directive
r.createGoal(title, directive)                 // API: create new goal node
```

No more string replacement. No more duplicate sections. The PM closes the old goal and creates a new one. Goals are nodes with state — you can query "what's active?" instead of parsing markdown.

### Scout (scout.go)

**Before:**
```go
directive := r.readScoutSection()              // string index on state.md
repoContext := r.readRepoContext()             // os.ReadFile(CLAUDE.md)
// ... LLM scouts ...
os.WriteFile("loop/scout.md", report)          // flat file
```

**After:**
```go
goals := r.getActiveGoals()                    // API: what should I focus on?
repoContext := r.readRepoContext()             // still local (target repo isn't on the graph)
// ... LLM scouts ...
r.createDocument("Scout Report: ...", report, ["scout-report", "iter:248"])
```

The scout report is now a document on the graph. Other agents reference it by ID, not by reading a local file.

### Architect (architect.go)

**Before:**
```go
scoutReport := r.readLoopArtifact("scout.md")  // os.ReadFile
```

**After:**
```go
scoutReports := r.getDocuments("scout-report", 1)  // API: latest scout report
scoutReport := scoutReports[0]                       // structured node with ID
```

Tasks created by the Architect can reference the scout report node as their parent or via tags, creating causal links on the graph.

### Reflector (reflector.go)

**Before:**
```go
r.appendReflection(entry)                      // append to reflections.md
r.advanceIterationCounter()                    // regex on state.md
```

**After:**
```go
r.createDocument("Reflection: Iter 248", entry, ["reflection", "iter:248"])
// Iteration counter: count reflection documents, or use a pinned "iteration" node
```

No unbounded append-only file. Each reflection is a separate document, queryable by iteration tag.

### LoadSharedContext (context.go)

**Before:**
```go
func LoadSharedContext(hiveDir string) string {
    // Read CONTEXT.md + lessons from state.md + directive from state.md
    // Concatenate into one big string
}
```

**After:**
```go
func (r *Runner) LoadSharedContext() string {
    context := r.getDocuments("context", 1)      // pinned context doc
    lessons := r.getDocuments("lesson", 20)       // recent lessons
    goals := r.getActiveGoals()                   // active directives
    // Assemble from structured data, not string parsing
}
```

## Pipeline Data Flow (After Migration)

```
PM
 ├─ Reads: active goals, backlog docs, completed tasks, recent documents
 ├─ Writes: completes old goal, creates new goal node
 └─ Output: goal node ID

Scout
 ├─ Reads: active goals (from PM), repo CLAUDE.md (local)
 ├─ Writes: scout report document (tagged scout-report)
 └─ Output: document node ID

Architect
 ├─ Reads: scout report document (by ID or tag)
 ├─ Writes: task nodes (children of scout report or standalone)
 └─ Output: task node IDs

Builder
 ├─ Reads: assigned tasks (already graph-native)
 ├─ Writes: code changes + build report document
 └─ Output: build report node ID

Critic
 ├─ Reads: recent commits + build report document
 ├─ Writes: critique document + Fix: tasks if REVISE
 └─ Output: critique node ID + verdict

Reflector
 ├─ Reads: scout report, build report, critique (all by ID)
 ├─ Writes: reflection document (tagged reflection)
 └─ Output: reflection node ID
```

Every artifact is a node. Every node has an ID. Agents pass IDs through the pipeline, not file paths. The graph is the single source of truth.

## What Stays Local

- **Target repo code** — the Builder still operates on local files via `Operate()`. Code isn't on the graph.
- **Role prompts** — could move to graph, but they're static and version-controlled. Low priority.
- **CLAUDE.md** — repo-specific context. Stays in the repo.

## Implementation Order

### Phase 1: MCP Knowledge Server (the light switch)

Build `cmd/mcp-knowledge/` — an MCP server that agents connect to for knowledge navigation. This is the highest-leverage piece because it makes ALL existing knowledge accessible without migrating anything yet.

Sources the MCP server indexes (read-only to start):
- `/site/content/reference/` — 201 primitives, 13 layers, 13 grammars (semantic memory)
- `/site/content/posts/` — blog posts (episodic memory — derivation stories)
- `/hive/loop/` — state, backlog, reflections, scout/build/critique reports (procedural + episodic)
- `/hive/agents/` — role prompts, CONTEXT.md (declarative)
- `/hive/docs/` — specs, design docs (declarative)
- lovyou.ai API — tasks, goals, documents, conversations (procedural + declarative)

The tree is built from directory structure + content parsing + API queries. Agents call MCP tools to navigate. No migration needed — the server reads from all existing sources.

**Tools:**
```
knowledge.topics(parent?)           → tree branches at this level
knowledge.drill(topic)              → children + summaries
knowledge.get(id)                   → full content (file, node, or post)
knowledge.search(query, type?)      → cross-source semantic search
knowledge.context(topic, depth?)    → assembled context for a topic
knowledge.related(id)               → linked/nearby knowledge
knowledge.recent(type?, limit?)     → latest entries by memory type
knowledge.primitives(layer?)        → primitives, optionally filtered by layer
knowledge.grammar(name)             → grammar ops for a product layer
```

**Why MCP first:** The existing `cmd/mcp-graph` wraps the lovyou.ai REST API. This new server wraps ALL knowledge sources — files, API, reference content, blog. Agents get one unified interface. The tree structure is computed, not stored. No schema changes. No migration. Just a server that reads what already exists and presents it as a navigable hierarchy.

### Phase 2: Graph-native artifacts (move writes to API)

Once agents can READ from the knowledge tree, migrate WRITES:

1. **Site: tag-filtered document endpoint** — `GET /app/{slug}/documents?tag=X&limit=N` returns JSON.
2. **Hive: API client methods** — `CreateDocument`, `GetDocuments`, `CreateGoal`, `GetActiveGoals`, `GetNode`. ~50 lines.
3. **Hive: PM migration** — Goals, not string replacement. ~40 lines.
4. **Hive: Scout/Architect/Reflector migration** — Documents, not files. ~60 lines.
5. **Hive: Pipeline ID passing** — Node IDs between roles.
6. **Hive: Context via MCP** — `LoadSharedContext` calls MCP knowledge server instead of reading files.

### Phase 3: Knowledge tree as graph nodes

Once reads and writes are graph-native, the tree itself becomes graph nodes:

1. **Topic nodes** — the tree structure stored as `document` nodes with parent/child relationships.
2. **Auto-indexing** — new documents automatically appear in the right part of the tree based on tags/kind.
3. **Cross-referencing** — primitives linked to the features they ground. Blog posts linked to the iterations they describe.
4. **The ontology as live data** — primitives aren't static markdown. They're nodes on the graph that agents can annotate, extend, and query relationally.

Phase 3 is when the hive fully eats its own dog food — the knowledge tree IS the product's Knowledge layer.

## The Ontology Agent (Role Gap)

No agent currently watches /blog or /reference. The 201 primitives are the architecture's foundation — every product decision should be grounded in them. The blog tells the derivation story. The reference page IS the spec.

**The gap:** When the PM decides "build invitations," no agent checks whether invitations map cleanly to the primitives (they do: Invitation = Offer + Acceptance from Layer 2/Exchange). When the Scout identifies a gap, no agent verifies it against the ontology layers. The architecture has 13 layers and 201 primitives, but the pipeline operates as if they don't exist.

**What's needed:** An agent (or a responsibility added to an existing role) that:
1. Grounds Scout reports in the ontology — "this gap maps to Layer N, primitives X/Y/Z"
2. Validates Architect task decomposition against grammar ops — "this should use `propose` + `vote`, not a custom flow"
3. Keeps /reference content as context — the primitives aren't just documentation, they're the type system
4. Flags when new features don't map to any existing primitive (gap in the ontology itself)

This could be the Philosopher agent (already exists in the 50-agent roster) or a new Ontologist role. The key: the 201 primitives and 13 grammars should be loaded as context for at least one agent in every pipeline run.

**Practical implementation:** Load `/site/content/reference/grammars/*.md` and `/site/content/reference/fundamentals/layer-*.md` as part of the Scout's or PM's context. The blog posts (`/site/content/posts/`) contain derivation reasoning that informs WHY primitives exist — useful for the Philosopher but too large for every-tick injection.

## What This Enables

- **PM sees the board** — goals are nodes, not strings in a file. No more duplicate sections. No more "can't see existing tasks."
- **Agents reference each other's work by ID** — the Critic can cite the exact scout report node. Reflections link to the iteration's artifacts.
- **Knowledge is auditable** — every document, goal, report is timestamped, authored, and causally linked.
- **Blog/reference integration path** — once knowledge is graph-native, blog posts and reference primitives could become document nodes too. The /reference page could query the graph instead of loading static files. The primitives would be live, linkable, and referenceable by agents.
- **The hive uses its own product** — dogfooding. The Knowledge Q&A feature was just shipped. The hive should be the first heavy user.

## Grounding: The Primitives

From the blog posts:

> "These 20 primitives are sufficient to model: company structure, agent communication, software behaviour..." — Post 1

> "A hive of AI agents, left to run on its own, decided it needed concepts for self, trust, uncertainty, blind spots, deception, and health." — Post 2

The hive's current file-based knowledge has no Self (no identity on its own artifacts), no Trust (no verification of what it reads), no Integrity (no hash chain on its knowledge), and critical Blind Spots (the PM literally can't see existing tasks). Moving to the graph fixes all of these — because the graph was designed from these primitives.

Layer 0 gives us: Event (something happens → recorded), EventStore (it's stored), Hash (tamper-proof), Self (the hive knows it produced this).

Layer 6 gives us: Record (knowledge is persisted), Channel (agents read each other's published work), Redundancy (replicated on the server, not one local file).

The migration isn't a refactor. It's the hive growing into its own architecture.

## Future: Memory Agent + Pipeline Parallelism

Not needed yet. Build after the knowledge layer is connected and the hive can reflect on its own needs. Captured here so someone picks it up when the time is right.

### Memory Agent (8th pipeline role)

The Reflector asks "what did we learn?" The Memory agent asks "where does this fit in what we know?" Different questions, different judgment.

**Runs twice per cycle:**
- **Pre-cycle:** Prepare context for PM/Scout. Surface relevant knowledge from the tree. "Last time we built a sidebar feature, the Critic caught X — here's that reflection."
- **Post-cycle:** Index what was built. Connect to primitives. Update the knowledge tree. "Council shipped → Layer 3: Society → Collective Act primitive."

The Memory agent is the natural owner of the MCP knowledge server — it both reads from and writes to the tree.

### Parallel Builders

The sequential pipeline is the time bottleneck (~10 min/cycle). Most of it is the Builder. With 6 open tasks, serial = 30-40 minutes. Three parallel Builders = 10-14 minutes.

```
PM ─┐
    ├→ Scout → Architect → Builder×N (parallel) → Critic → Reflector ─┐
Memory ┘                                                        Memory ┘
```

**Implementation shape:** `runner.RunParallel(ctx, []Config{...})` — goroutine per Builder with `sync.WaitGroup`. Each Builder works one task on its own git branch. Critic reviews each branch's diff. Squash-merge after PASS. Push once at end.

**Complication:** Repo conflicts if two Builders edit the same file. Mitigated by: Architect decomposes into non-overlapping tasks, Go compilation catches conflicts, Critic reviews the merge.

**When:** After the knowledge layer is live and agents can self-reflect on pipeline performance. The Memory agent will likely be the one to notice "we're spending 40 minutes on 6 tasks that could run in parallel" and create the task itself.

### Event-Driven Pipeline (replace polling)

The pipeline is poll-based. Every run: check board → run PM → Scout → Architect → Builder → Critic → Reflector. When Architect produces no tasks, Builder/Critic/Reflector run on nothing. We added a stopgap (skip downstream roles when board is empty), but the architecture is wrong.

**The problem:** No-op cycles cost $0.40-0.90 to discover there's no work. The daemon mode polls every 30 minutes. That's $20-40/day just checking if there's something to do.

**What it should be:** Events on the graph trigger agents. No events = no cost.

```
PM creates goal         → event: goal.created    → triggers Scout
Scout creates report    → event: document.created(tag=scout-report) → triggers Architect
Architect creates tasks → event: task.created     → triggers Builder
Builder commits         → event: task.completed   → triggers Critic
Critic reviews          → event: review.created   → triggers Reflector (or Builder on REVISE)
```

No tasks = nothing triggers = zero cost. The graph is the bus. Agents are subscribers.

**This is what the eventgraph was designed for.** The `event` package has event types, the `store` has append/subscribe, the bus exists. The pipeline bypasses its own architecture because the runner was built as a quick loop, not as an event consumer.

**Implementation:** Each agent registers event subscriptions (like `AgentDef.WatchPatterns`). The daemon becomes an event loop: poll the graph for new events since last cursor, dispatch to matching agents. Or use webhooks from lovyou.ai — the site posts events, the hive reacts.

**Dependency:** Requires the knowledge graph migration (Phase 2) — once PM directives are goal nodes and Scout reports are document nodes on the graph, their creation IS the event that triggers the next agent. The .md files can't emit events. The graph can.

**When:** After Phase 2 of the knowledge graph migration ships. The transition: poll loop → event loop. Same agents, same logic, different trigger mechanism.
