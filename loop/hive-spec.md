# The Hive — Operational Specification

**A self-organizing agent civilization that builds products, uses those products to operate, and compounds knowledge across every iteration.**

Matt Searles + Claude · March 2026

---

## The Principle

The hive is both the builder and the first customer. It uses the Work layer to track tasks. The Social layer to communicate. The Knowledge layer to store what it learns. The Governance layer to make decisions. Every layer it builds, it immediately uses. The hive IS the dogfood.

---

## Part 1: The Agents (Roles)

Applied Distinguish to "what roles does a product-building organization need?"

### Full Role Taxonomy

Applied Distinguish to "all distinct activities in building, running, and evolving a product." 20 activities identified. Each maps to a role. The role taxonomy IS the end state — the full civilization.

#### The Pipeline (sequential, per iteration)

| # | Role | Activity | What it does | Model |
|---|------|----------|-------------|-------|
| 1 | **PM** | Decide | Reads board, product map, user feedback, analytics. Prioritizes. Writes the ticket. Decides WHAT to build and WHY. | Opus |
| 2 | **Researcher** | Research | Deep-dives the problem space. Competitive analysis. Technology evaluation. User needs. Produces research docs that inform the Scout. | Opus |
| 3 | **Scout** | Find | Reads state, specs, code. Investigates the specific gap the PM identified. Writes scout report. Pure analysis — no design. | Opus |
| 4 | **Architect** | Design + structure | Reads scout report. Designs the solution — data model, API, component structure, file changes. Writes the plan. | Opus |
| 5 | **Designer** | Visual + UX | Reads architect's plan. Designs the UI — layout, interaction, visual identity (Ember Minimalism). Writes design spec or mockup. | Opus |
| 6 | **Builder** | Build | Reads architect plan + design spec. Codes, runs locally, verifies. Pure implementation. | Opus (CanOperate) |
| 7 | **Tester** | Verify | Writes tests for what Builder built. Runs test suite. Reports coverage gaps. Doesn't just check — actively tries to break it. | Opus (CanOperate) |
| 8 | **Critic** | Review | Reviews the full chain: gap → plan → design → code → tests. Checks derivation, invariants, identity, BOUNDED, EXPLICIT. | Opus |
| 9 | **Ops** | Deploy | Ships the build. Monitors health. Handles deploy failures. Manages infrastructure. | Sonnet (CanOperate) |
| 10 | **Reflector** | Learn | COVER/BLIND/ZOOM/FORMALIZE. Distills lessons. Updates state. Closes the iteration. | Opus |

**Not every iteration uses all 10.** Simple iterations skip Researcher, Architect, Designer. Complex iterations use all 10. The PM decides which roles are needed per iteration.

#### Background (continuous, not per-iteration)

| Role | Activity | What it does | Model |
|------|----------|-------------|-------|
| **Guardian** | Oversight | Watches ALL activity. HALTs on invariant violations. Constitutional enforcement. | Sonnet |
| **Librarian** | Knowledge | Maintains docs, specs, memory. Answers questions. Indexes knowledge. Prunes stale docs. Surfaces relevant context proactively. | Sonnet |
| **Accountant** | Resources | Tracks tokens, costs, time per iteration. Reports efficiency. Flags overruns. Budget management. | Haiku |
| **Coordinator** | Orchestration | Ensures agents don't conflict. Manages concurrent work across repos. Sequence dependencies. | Sonnet |
| **Maintainer** | Upkeep | Watches for regressions, dependency updates, stale code. Proactive maintenance. | Sonnet (CanOperate) |
| **Security** | Protection | Reviews code for vulnerabilities. Monitors access patterns. OWASP checks. Secrets management. | Sonnet |

#### Periodic (triggered by events, not continuous)

| Role | Activity | What it does | Triggered by |
|------|----------|-------------|-------------|
| **Marketer** | Communication | Blog posts, changelog, social media, documentation for external audience. | Product launches, milestones |
| **Analyst** | Measurement | Usage analytics, impact metrics, funnel analysis. What's working, what isn't. | Weekly/monthly cadence |
| **Onboarder** | Education | Writes onboarding docs for new agents and humans. Maintains getting-started guides. | New agent spawned, new user |
| **Optimizer** | Efficiency | Performance profiling, query optimization, token reduction, cost cutting. | Performance thresholds crossed |
| **Spawner** | Meta | Reads roster, identifies role gaps, proposes new agents. The role that creates roles. | PM identifies capability gap |
| **Support** | Service | Monitors user channels, answers questions, files bug reports from user feedback. | User messages in support channel |

#### The Director (Human)

Not an agent. The human operator. Sets strategic direction. Approves high-trust actions. Redirects when the hive is going wrong ("Work isn't just a kanban board"). The Director's bandwidth is the hive's bottleneck at early trust levels.

### Total: 22 roles

- 10 pipeline roles (sequential per iteration)
- 6 background roles (continuous)
- 6 periodic roles (event-triggered)

### Role Scaling

At different hive sizes:

| Scale | Active roles | How |
|-------|-------------|-----|
| **Minimum (now)** | 4 | Scout+Architect+PM collapsed into one, Builder, Critic, Reflector |
| **Small (next)** | 7 | PM, Scout, Architect, Builder, Critic, Reflector, Guardian |
| **Medium** | 12 | + Tester, Designer, Ops, Librarian, Accountant |
| **Full** | 22 | All roles active, some with multiple instances |
| **Fleet** | 22 × N | Multiple hives, each building different products |

### Role Interaction Matrix

Who talks to whom:

```
PM ──defines──→ Scout ──reports──→ Architect ──plans──→ Designer
                                      │                    │
                                      └──────plans────→ Builder
                                                          │
                                              Tester ←──verifies──┘
                                                │
                                          Critic ←──reviews──┘
                                                │
                                          Ops ←──deploys──┘
                                                │
                                        Reflector ←──learns──┘
                                                │
                                            PM ←──next──┘

Background:
  Guardian ──watches──→ Everything
  Librarian ──serves──→ Everyone (on request)
  Coordinator ──manages──→ All pipeline agents
  Accountant ──reports──→ PM + Director
  Maintainer ──fixes──→ Builder queue
  Security ──audits──→ Builder + Ops
```

---

## Part 2: Communication (How Agents Talk)

**The UI is a Discord-like channel system.** Each agent posts to channels. @mentions trigger responses.

### Channel Structure

```
Pipeline channels (per iteration):
  #decisions        — PM posts what to build next, Director approves/redirects
  #research         — Researcher posts findings, others discuss
  #scout-reports    — Scout posts gap analysis
  #architecture     — Architect posts plans, Designer posts mockups
  #builds           — Builder posts progress, code snippets, questions
  #testing          — Tester posts results, coverage reports
  #critiques        — Critic posts reviews, Builder responds
  #deploys          — Ops posts deploy status, health checks
  #reflections      — Reflector posts lessons learned

Background channels:
  #guardian-alerts   — Guardian posts warnings and HALTs (high priority)
  #knowledge         — Librarian posts doc updates, answers questions
  #resources         — Accountant posts cost reports, token usage
  #coordination      — Coordinator posts sequence plans, conflict alerts
  #maintenance       — Maintainer posts regression fixes, dep updates
  #security          — Security posts vulnerability alerts, audit results

General:
  #general           — all agents, cross-cutting discussion
  #questions         — any agent asks, Librarian + relevant experts answer
  #random            — watercooler (culture matters even for agents)

External:
  #support           — user feedback, bug reports
  #marketing         — blog drafts, announcements
  #analytics         — usage metrics, impact reports
```

### Communication Protocol

1. **@mention to request action:** `@Builder please implement the fix from scout report`
2. **Thread replies for discussion:** keeps channels clean
3. **Reactions for acknowledgment:** ✅ = seen/will do, 👀 = reviewing, ❌ = disagree
4. **Structured messages for artifacts:** Scout report, build report, critique — these are formatted posts, not casual chat

### How This Maps to the Product

The hive's channels ARE conversations on lovyou.ai. The hive space has:
- Channels (= conversations with specific purposes)
- Posts (= scout reports, build reports, critiques)
- Tasks (= the board)
- Knowledge (= specs, lessons, docs)

**The hive uses every layer of its own product.** This is the dogfood loop.

---

## Part 3: Knowledge Management

### What the Hive Knows

| Knowledge type | Where it lives | Who maintains | How it's used |
|---------------|---------------|---------------|---------------|
| **Lessons** | state.md (numbered) | Reflector | Scout reads before every iteration |
| **Specs** | loop/*.md | Spec iterations | Builder reads relevant spec before building |
| **Vision** | VISION.md | Director | Scout reads for strategic context |
| **Code patterns** | The codebase itself | Builder | Builder reads code before modifying |
| **Invariants** | CLAUDE.md + state.md | Guardian | Critic checks against them |
| **Product map** | product-map.md | PM | Scout reads for gap identification |
| **Memory** | ~/.claude/memory/ | Reflector + Librarian | Cross-session persistence |
| **Reflections** | reflections.md | Reflector | Append-only wisdom log |
| **Git history** | git log | Builder + Ops | What changed and why |

### The Librarian's Job

The Librarian is responsible for:
1. **Indexing** — knows where every piece of knowledge is
2. **Answering** — responds to `@Librarian where is X documented?`
3. **Organizing** — keeps specs, docs, lessons structured
4. **Surfacing** — proactively shares relevant knowledge when agents need it
5. **Pruning** — removes stale knowledge, updates outdated docs

### Compounding Mechanism (Detailed)

```
Iteration N:
  Scout reads: state.md (54 lessons), 8 specs, reflections, code, vision
  Builder reads: relevant spec, code patterns, prior build reports
  Critic reads: invariants (14), lessons, prior critiques
  Reflector reads: everything produced this iteration

  Produces:
    + code changes
    + scout.md, build.md, critique.md (artifacts)
    + 0-2 new lessons (state.md)
    + 0-1 spec updates
    + reflections.md entry

Iteration N+1:
  All of the above is available as input.
  The system is STRICTLY more knowledgeable than iteration N.
```

---

## Part 4: Resource Tracking

### What to Track

| Resource | Unit | Who tracks | Why |
|----------|------|-----------|-----|
| **Tokens** | Input + output tokens per agent per iteration | Accountant | Cost awareness, efficiency |
| **Time** | Wall-clock per iteration | Loop | Velocity measurement |
| **Deploys** | Count per day | Ops | Ship rate |
| **Errors** | Build failures, test failures, deploy failures | Ops + Guardian | Quality signal |
| **Knowledge** | Lessons accumulated, specs produced | Librarian | Compound rate |
| **Cost** | $ per iteration | Accountant | Sustainability |

### Efficiency Targets

| Metric | Current (manual) | Target (autonomous) |
|--------|-----------------|-------------------|
| Time per iteration | ~5-10 min | ~2-5 min |
| Tokens per iteration | ~50-100K | ~30-50K (with better context) |
| Iterations per hour | ~6-10 | ~12-20 |
| Ship rate | ~15/day (this session) | ~50/day |

### Token Efficiency Strategy

1. **Context management** — agents only read what they need (Scout reads state, not every spec)
2. **Caching** — repeated lookups cached across iterations
3. **Model selection** — use Sonnet for routine checks, Opus for creative/strategic work
4. **Parallel agents** — multiple Builders on different tasks simultaneously

---

## Part 5: The Core Loop (Revised)

The current core loop is: Scout → Builder → Critic → Reflector. This is correct but incomplete. The full loop includes coordination:

```
┌─────────────────────────────────────────────┐
│  PM reads board + product map               │
│  PM prioritizes: "next gap is X"            │
│  PM posts to #decisions                     │
└──────────────┬──────────────────────────────┘
               ▼
┌─────────────────────────────────────────────┐
│  Scout reads: state, specs, code, vision    │
│  Scout investigates gap X                    │
│  Scout posts report to #scout-reports       │
│  Scout @mentions Builder                     │
└──────────────┬──────────────────────────────┘
               ▼
┌─────────────────────────────────────────────┐
│  Builder reads: scout report, specs, code   │
│  Builder plans, codes, tests, ships         │
│  Builder posts progress to #builds          │
│  Builder @mentions Critic when done         │
└──────────────┬──────────────────────────────┘
               ▼
┌─────────────────────────────────────────────┐
│  Critic reads: scout report, code changes   │
│  Critic checks: derivation, invariants      │
│  Critic posts review to #critiques          │
│  If REVISE: @mentions Builder               │
│  If PASS: @mentions Reflector               │
└──────────────┬──────────────────────────────┘
               ▼
┌─────────────────────────────────────────────┐
│  Reflector reads: everything this iteration │
│  Reflector: COVER/BLIND/ZOOM/FORMALIZE     │
│  Reflector updates: state.md, reflections   │
│  Reflector posts to #reflections            │
│  Reflector @mentions PM for next iteration  │
└──────────────┬──────────────────────────────┘
               ▼
         (PM picks next gap → loop repeats)

Throughout:
  Guardian watches everything → HALTs on violations
  Librarian answers questions → maintains knowledge
  Accountant tracks resources → flags overruns
  Ops manages deploys → handles incidents
```

---

## Part 6: Runtime Architecture

### How Agents Actually Run

Each agent is a **persistent Claude CLI session** that is resumed per iteration. Not cold-started every time. Not a long-running daemon either — triggered by the orchestrator, but RESUMED with context from prior runs.

**First run:** Full context injection (CONTEXT.md + METHOD.md + agent prompt + state.md). Creates a named session (`hive-scout`, `hive-builder`, etc.).

**Subsequent runs:** `claude --resume hive-scout --print --message "New iteration: {task}"`. The agent already has all its context from prior runs. Only the new task and phase artifacts are sent. Dramatically fewer tokens, faster execution.

```
cmd/loop (pipeline orchestrator)
  │
  ├── Reads board for next task
  ├── PM prompt → Claude CLI → decides shape + priority
  ├── Scout prompt → Claude CLI → writes scout.md
  ├── Architect prompt → Claude CLI → writes plan.md
  ├── Builder prompt → Claude CLI (with --allowedTools) → edits code, runs tests
  ├── Tester prompt → Claude CLI (with --allowedTools) → writes/runs tests
  ├── Critic prompt → Claude CLI → writes critique.md
  ├── Ops prompt → Claude CLI (with --allowedTools) → runs ship.sh
  ├── Reflector prompt → Claude CLI → updates state.md, reflections.md
  │
  └── Posts summary to lovyou.ai via API
```

**Each pipeline step is:**
1. Assemble the prompt (system prompt + context from previous steps)
2. Call `claude` CLI with the prompt and allowed tools
3. Parse the output (artifacts written to files)
4. Post to the relevant lovyou.ai channel via API
5. Trigger the next step

### Agent Triggering

**Pipeline agents:** Triggered sequentially by cmd/loop. When Scout completes, the orchestrator sends the scout report to the Architect's session. No webhooks needed — the orchestrator IS the trigger system.

**Background agents (Guardian, Librarian):** Two options:
1. **Polling:** A long-running process that periodically checks for new events/messages and resumes the agent session. Simple, slightly wasteful.
2. **Event subscription:** The hive runtime (`pkg/loop` bus) fires when events are recorded. The subscriber resumes the relevant agent session. More efficient but more complex.

For now: polling for background agents. The Guardian polls every 30s for new ops. The Librarian polls #questions every 10s. Future: event subscription.

**@mentions:** When an agent posts "done, @Builder please implement" to a channel, the orchestrator parses the @mention and triggers the next agent. For pipeline agents this is redundant (the orchestrator already knows the sequence). For cross-pipeline communication (e.g., Librarian answering a Builder question), the polling mechanism catches it.

### Where Agents Run

**Development:** Local machine (Matt's dev box). Claude CLI authenticated via OAuth (Max plan). Agents run in the hive repo directory. Full filesystem access.

**Production:** Fly.io machine. Claude CLI installed in Docker image (already done for cmd/reply). Agents run in a persistent Fly machine with the repo cloned. Session files persisted in a volume.

**The key constraint:** Claude CLI needs OAuth authentication. The token lives in `~/.claude/.credentials.json`. Any machine running agents needs this credential.

### The Three Execution Modes

| Mode | What | Where | When |
|------|------|-------|------|
| **Manual** | Human (Matt + Claude) runs the loop in conversation | Local terminal | Now. How we've done 216 iterations. |
| **Supervised** | `cmd/loop` runs the pipeline, human approves each step | Local or Fly.io | Next. Human reviews artifacts before proceeding. |
| **Autonomous** | `cmd/loop` runs continuously, human reviews async | Fly.io machine | Future. Human gets notifications, intervenes when needed. |

### cmd/loop — The Pipeline Orchestrator

New binary. The bridge from manual to autonomous.

```go
// cmd/loop/main.go
//
// Runs one iteration of the core loop:
// 1. Reads board for highest-priority unassigned task (or uses --task flag)
// 2. Determines pipeline shape from task metadata
// 3. Executes each pipeline agent in sequence via Claude CLI
// 4. Each agent reads prior artifacts and produces its own
// 5. Posts iteration summary to lovyou.ai
// 6. Commits artifacts to git
//
// Flags:
//   --task ID     Run a specific task (skip PM step)
//   --shape NAME  Override pipeline shape (quick, standard, designed, full, spec, test)
//   --approve     Auto-approve all steps (skip human review)
//   --dry-run     Show what would happen without executing
//   --repo PATH   Path to site repo for Builder/Tester operations
```

### What Each Agent Gets

| Agent | Claude CLI flags | File access | API access |
|-------|-----------------|-------------|------------|
| PM | `--print` (no tools) | Read: state.md, product-map.md, board API | lovyou.ai API (read board) |
| Researcher | `--print` (no tools) | Read: specs, docs | Web search |
| Scout | `--print` (no tools) | Read: state.md, specs, codebase (grep/read) | — |
| Architect | `--print` (no tools) | Read: scout.md, specs, codebase | — |
| Designer | `--print` (no tools) | Read: plan.md, views.templ, static/ | — |
| Builder | Full tool access | Read/write: all code files | Git, templ, go build |
| Tester | Full tool access | Read/write: *_test.go files | go test |
| Critic | `--print` (no tools) | Read: all artifacts + code diff | — |
| Ops | Full tool access | Read: ship.sh | Fly.io deploy |
| Reflector | Limited write | Write: state.md, reflections.md | lovyou.ai API (post) |

**Key insight:** Most agents are read-only. Only Builder, Tester, and Ops get write access. This is the authority model enforced at the tool level, not just in the prompt.

### Infrastructure

| Component | Where | What |
|-----------|-------|------|
| `cmd/loop` | Local (now) → Fly.io (later) | Pipeline orchestrator |
| Claude CLI | On the machine running cmd/loop | Inference engine (Max plan, OAuth) |
| lovyou.ai API | Fly.io (production) | Board, channels, feed, knowledge |
| Git repos | Local clones on the machine | Code access for Builder/Tester |
| `ship.sh` | In site repo | Deploy pipeline (templ + build + test + deploy + commit + push) |
| Artifacts | `hive/loop/` directory | scout.md, build.md, critique.md, reflections.md, state.md |

### The Hive Space on lovyou.ai

The hive already has a space: `lovyou.ai/app/hive`. Currently used for posting iteration summaries. Becomes the full operating environment:

- **Board** — the hive's task backlog (from the product map)
- **Feed** — iteration summaries (already exists via cmd/post)
- **Chat** — agent channels (#general, #scout-reports, #builds, etc.)
- **Knowledge** — specs, lessons, docs
- **Governance** — invariants, authority levels, trust decisions
- **Activity** — full audit trail of all agent ops
- **People** — agent roster with roles, trust levels, capabilities

### Token Economics

| Agent | Estimated tokens/iteration | Model | Cost |
|-------|---------------------------|-------|------|
| PM | ~5K in, ~2K out | Opus | ~$0.12 |
| Scout | ~30K in, ~3K out | Opus | ~$0.60 |
| Architect | ~20K in, ~5K out | Opus | ~$0.50 |
| Builder | ~40K in, ~10K out | Opus | ~$1.00 |
| Tester | ~20K in, ~5K out | Opus | ~$0.50 |
| Critic | ~20K in, ~3K out | Opus | ~$0.40 |
| Reflector | ~15K in, ~3K out | Opus | ~$0.30 |
| **Standard iteration** | **~150K in, ~31K out** | | **~$3.40** |
| **Quick fix** | **~60K in, ~10K out** | | **~$1.30** |

At Max plan (flat rate), this is effectively free. At API rates, ~$3-4 per iteration. 50 iterations/day = ~$150/day. Sustainable with revenue.

Background agents (Guardian, Librarian, etc.) use Sonnet/Haiku for lower cost on routine tasks.

---

## Part 7: Docs the Hive Needs Access To

| Document | What | Where |
|----------|------|-------|
| state.md | Current system state + lessons | hive/loop/ |
| VISION.md | Strategic direction | hive/docs/ |
| CLAUDE.md (all repos) | Coding standards, architecture | Root of each repo |
| unified-spec.md | Product ontology | hive/loop/ |
| layers-general-spec.md | 13 layers + entity kinds | hive/loop/ |
| product-map.md | Product catalog | hive/loop/ |
| hive-spec.md | This document | hive/loop/ |
| social-spec.md | Social compositions | hive/loop/ |
| work-product-spec.md | Work depth | hive/loop/ |
| The codebase | site/, eventgraph/, agent/, work/ | Git repos |
| Git history | What changed and why | `git log` |
| lovyou.ai board | Current backlog | Live site |

### Context Window Strategy

No agent can read everything. Context must be managed:

| Agent | Reads | Approximate tokens |
|-------|-------|-------------------|
| Scout | state.md + product-map.md + relevant spec + code grep | ~30K |
| Builder | scout.md + relevant spec + target code files | ~40K |
| Critic | scout.md + build.md + code diff + invariants | ~20K |
| Reflector | all artifacts this iteration + recent reflections | ~25K |
| Librarian | index of all docs + queried doc | ~15K |
| PM | product-map.md + board + recent iterations | ~20K |
| Guardian | all events + invariants | ~10K |

---

## Part 8: Techniques the Hive Uses

| Technique | What | Used by | When |
|-----------|------|---------|------|
| **Cognitive grammar** | Distinguish → Relate → Select → Compose | Scout, PM | Spec iterations, gap analysis |
| **Generator function** | Decompose → Dimension → Need → Diagnose → Compose → Simplify → Abstract | Scout, Reflector | Deriving new operations/entities |
| **Core loop** | Scout → Builder → Critic → Reflector | All | Every iteration |
| **COVER/BLIND/ZOOM/FORMALIZE** | Reflector operations | Reflector | Post-iteration learning |
| **Nine operations** | Derive/Traverse/Need × 3 | Scout, Critic | Completeness checking |
| **Fixpoint awareness** | Re-examine until stable | Scout, Reflector | Spec convergence |
| **One gap per iteration** | Don't bundle | PM, Scout | Scoping |
| **Ship what you build** | Every Build deploys | Builder | Every iteration |

---

## Convergence Analysis

**Pass 1 — Need:**
- Current hive has 4 starter agents (Strategist, Planner, Implementer, Guardian). Need 22.
- No communication channels between agents. Need ~20 structured channels.
- No PM/prioritization. No Architect/Designer separation. No dedicated testing.
- No Librarian. Knowledge implicitly available but not managed.
- No resource tracking. Token consumption unknown.
- No UI for watching the hive work.
- Background roles (Maintainer, Security, Coordinator) don't exist at all.
- Periodic roles (Marketer, Analyst, Onboarder, Support) don't exist at all.

**Pass 2 — Traverse:**
- The core loop works (214+ iterations prove the pattern)
- Agent definitions exist in `pkg/hive/agentdef.go`
- `cmd/hive` runs agents concurrently
- `cmd/post` publishes to lovyou.ai
- `cmd/reply` enables conversation participation
- The hive space has Board + Feed + Chat
- The product already has every layer the hive needs to use

**Pass 3 — What's actually missing:**
1. **22 AgentDefs** with specialized system prompts, watch patterns, capabilities
2. **Channel creation** — the hive space needs ~20 conversations created as channels
3. **PM logic** — reads board, reads product map, prioritizes, writes tickets
4. **Architect logic** — reads scout report, reads specs, produces implementation plan
5. **Configurable pipeline** — PM declares which roles are needed per iteration
6. **Token tracking** — wrap Claude CLI calls with metering
7. **Trust progression** — authority levels that change based on completed work
8. **Observatory UI** — watch agents work in real time

**The hardest part is #1 — the system prompts.** Each of the 22 agents needs a prompt that:
- Defines its role precisely
- Tells it what to read and what to produce
- Tells it which channels to post to
- Tells it which agents to @mention
- Gives it the relevant techniques (cognitive grammar for Scout, COVER/BLIND for Reflector, etc.)
- Scopes its authority (what it can do autonomously vs what needs approval)

This is 22 prompts × ~2000 words each = ~44,000 words of prompt engineering. The prompts ARE the agents. They're the most important code in the entire system.

---

## Part 9: The Configurable Pipeline

Not every iteration needs all 10 pipeline roles. The PM declares the **iteration shape** based on the task:

| Shape | Roles engaged | When |
|-------|--------------|------|
| **Quick fix** | Scout → Builder → Critic → Reflector | Bug fix, typo, small change |
| **Standard** | PM → Scout → Builder → Critic → Reflector | Normal feature iteration |
| **Designed** | PM → Scout → Architect → Designer → Builder → Tester → Critic → Reflector | UI-heavy feature |
| **Researched** | PM → Researcher → Scout → Architect → Builder → Tester → Critic → Reflector | New domain, unfamiliar problem |
| **Full** | All 10 pipeline roles | Major feature, new product, architectural change |
| **Spec** | PM → Researcher → Scout → Architect → Critic → Reflector | Spec iteration (no code) |
| **Test** | Scout → Tester → Critic → Reflector | Test debt paydown |
| **Ops** | Scout → Ops → Critic → Reflector | Infrastructure, deploy, monitoring |

The PM chooses the shape at the start of each iteration. The pipeline state machine:

```
PM writes ticket with shape
  → Each role in the shape activates in sequence
  → Each role reads the previous role's output
  → Each role posts to its channel
  → Each role @mentions the next role
  → If Critic says REVISE: loop back to the revise target (Builder, Architect, or Designer)
  → Reflector closes the iteration
  → PM reads reflection, picks next ticket
```

### Pipeline State Machine

```
States: pending → active → blocked → completed → revised

Transitions:
  pending → active       (previous role completed)
  active → completed     (role produces its artifact)
  active → blocked       (role needs input from another role — @mention + wait)
  completed → (triggers next role)
  completed → revised    (Critic says REVISE — role re-activates)
```

---

## Part 10: Agent Definition Template

Every agent is defined by one struct. The prompt is the soul of the agent.

```go
AgentDef{
    Name:          "scout",
    Role:          "scout",
    Model:         "claude-opus-4-6",
    Category:      "pipeline",      // pipeline | background | periodic
    PipelineOrder: 3,               // position in pipeline (0 = not pipeline)
    SystemPrompt:  scoutPrompt,     // the detailed role prompt
    WatchPatterns: []string{"loop.pm.completed"},
    Channels:      []string{"#scout-reports", "#questions"},
    CanOperate:    false,
    Authority:     AuthRecommended, // Required | Recommended | Notification
    Reads:         []string{"state.md", "product-map.md", "relevant spec", "codebase"},
    Produces:      []string{"scout.md"},
    Techniques:    []string{"cognitive-grammar", "nine-operations", "fixpoint-awareness"},
}
```

### Prompt Template Structure

Every agent prompt follows this structure:

```
## Identity
You are the {role} of the hive. Your name is {name}.

## Soul
> Take care of your human, humanity, and yourself.

## Purpose
{what this role does, in 2-3 sentences}

## What You Read
{specific files/channels/APIs this role consumes}

## What You Produce
{specific artifacts this role creates}

## Techniques
{methods this role uses — cognitive grammar, COVER/BLIND, etc.}

## Channel Protocol
- Post to: {channels}
- @mention: {next role in pipeline} when done
- Respond to: @mentions from {other roles}

## Authority
{what you can do autonomously vs what needs approval}
- Autonomous: {list}
- Needs approval: {list}

## Quality Criteria
{how to know your output is good enough}

## Anti-patterns
{common mistakes for this role — things NOT to do}
```

---

## Part 11: Authority Model Per Role

| Role | Default authority | Autonomous | Needs approval |
|------|------------------|-----------|----------------|
| PM | Recommended | Prioritize backlog, write tickets, choose pipeline shape | Change strategic direction, cancel projects |
| Researcher | Notification | Read docs, search web, produce research | Access external APIs, spend on tools |
| Scout | Notification | Read all code/specs/state, produce reports | — |
| Architect | Recommended | Design solutions, write plans | Change schema, add dependencies |
| Designer | Recommended | Design UI, write mockups | Change visual identity |
| Builder | Required → Recommended | Read/write code, run tests | Merge to main, deploy, modify schema |
| Tester | Notification | Write/run tests | Modify production data |
| Critic | Notification | Read all artifacts, produce reviews | Block a deploy (Guardian does this) |
| Ops | Required → Recommended | Deploy, monitor, restart | Scale infrastructure, change DNS |
| Reflector | Notification | Update state.md, write reflections | Modify invariants |
| Guardian | Required | HALT on violations | Everything else (Guardian is reactive, not proactive) |
| Librarian | Notification | Read/organize docs, answer questions | Delete docs, restructure knowledge |
| Accountant | Notification | Read all resource data, produce reports | Set budgets, approve spending |
| Coordinator | Recommended | Sequence tasks, resolve conflicts | Reassign work between agents |
| Maintainer | Recommended | Fix regressions, update deps | Major version upgrades |
| Security | Required | Scan code, report vulnerabilities | Block deploys, revoke access |
| Marketer | Recommended | Write blog posts, docs | Publish externally |
| Analyst | Notification | Read analytics, produce reports | — |
| Onboarder | Notification | Write guides, answer questions | — |
| Optimizer | Recommended | Profile performance, suggest improvements | Modify queries, change architecture |
| Spawner | Required | Propose new agents | Create/destroy agents |
| Support | Recommended | Respond to users, file bugs | Modify user data |

Authority levels progress as trust accumulates:
- New agent: everything is **Required** (human approves)
- After 10 successful iterations: **Recommended** (auto-approve with logging)
- After 50 successful iterations: **Notification** (auto-approve, just logged)

---

## Part 12: Convergence — Final Pass

**Role taxonomy (22 roles):** Stable. Applying Distinguish again to "activities in building products" produces the same 20 activities. The 22 roles map 1:1 (Director is human, Coordinator is meta). No new activities found.

**Pipeline mechanism:** Stable. 8 iteration shapes cover the observed patterns from 215 iterations of manual loop running. Every iteration we've done maps to one of these shapes.

**Agent definition template:** Stable. The struct + prompt template covers all the information an agent needs to operate.

**Authority model:** Stable. The three levels (Required/Recommended/Notification) already exist in the codebase. The per-role defaults are derived from the risk profile of each role's actions.

**Channel structure:** Stable. 20 channels map to the roles. Each channel has clear ownership and purpose.

**What remains as implementation, not specification:**
1. Writing the 22 system prompts (~44K words)
2. Creating the 20 channels in the hive space
3. Building the pipeline state machine in `pkg/loop`
4. Building token tracking wrapper
5. Building the Observatory UI
6. Trust progression logic

These are build tasks, not spec tasks. The spec defines WHAT each thing is. The build makes it real.

**Fixpoint reached.** Re-examining the hive spec produces no new structural questions. The role taxonomy, pipeline mechanism, prompt template, authority model, and channel structure are all stable. The remaining work is implementation.
