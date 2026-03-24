# The Hive — Operational Specification

**A self-organizing agent civilization that builds products, uses those products to operate, and compounds knowledge across every iteration.**

Matt Searles + Claude · March 2026

---

## The Principle

The hive is both the builder and the first customer. It uses the Work layer to track tasks. The Social layer to communicate. The Knowledge layer to store what it learns. The Governance layer to make decisions. Every layer it builds, it immediately uses. The hive IS the dogfood.

---

## Part 1: The Agents (Roles)

Applied Distinguish to "what roles does a product-building organization need?"

### Core Roles

| Agent | Role | What it does | Watches | Model | CanOperate |
|-------|------|-------------|---------|-------|------------|
| **Director** | Human operator | Sets strategic direction, approves, redirects | Everything | — | — |
| **Scout** | Research + gap finding | Reads state, vision, specs, code. Identifies the highest-value gap. | `loop.iteration.completed` | Opus | false |
| **Builder** | Implementation | Reads scout report + specs. Plans, codes, tests, ships. | `loop.scout.completed` | Opus | true |
| **Critic** | Quality assurance | Traces derivation chain. Checks invariants, identity, tests. | `loop.build.completed` | Opus | false |
| **Reflector** | Learning + wisdom | COVER/BLIND/ZOOM/FORMALIZE. Distills lessons. Updates state. | `loop.critique.completed` | Opus | false |
| **Guardian** | Constitutional oversight | Watches ALL activity. HALTs on invariant violations. | `*` | Sonnet | false |

### Specialist Roles (spawn as needed)

| Agent | Role | What it does | When to spawn |
|-------|------|-------------|--------------|
| **Librarian** | Knowledge management | Maintains docs, specs, memory. Answers "where is X documented?" Keeps knowledge organized and queryable. | When docs exceed 20 files |
| **PM** | Product management | Reads the product map + user feedback. Prioritizes the backlog. Writes tickets to the board. | When multiple products are in progress |
| **Designer** | Front-end design | Applies visual identity (Ember Minimalism). Reviews UI changes. Creates mockups. | When UI work is needed |
| **Researcher** | Deep investigation | Competitive analysis, technology evaluation, user research. Produces research docs. | When facing unfamiliar domains |
| **Tester** | Test engineering | Writes and runs tests. Maintains test infrastructure. Catches the gaps the Critic flags. | When test debt exceeds threshold |
| **Ops** | Infrastructure | Deployment, monitoring, performance. Manages Fly.io, database, CI. | When infra issues arise |
| **Marketer** | Growth + communication | Writes blog posts, documentation. Manages external presence. | When the product needs users |
| **Accountant** | Resource tracking | Tracks token consumption, costs, budget. Reports efficiency. | When resource awareness matters |
| **Spawner** | Meta-agent | Reads the roster, identifies role gaps, proposes new agents. | When the hive is mature enough |

### The Growth Pattern

```
1. Director notices a gap ("we need better docs")
2. Director or PM creates a task: "Spawn Librarian agent"
3. Spawner (or Director) defines the AgentDef
4. New agent starts watching relevant events
5. Agent proves value through completed work
6. Trust escalates → agent gets more autonomy
```

### Role Capacity

Not every role needs a dedicated agent. At small scale:
- Scout + Critic + Reflector can be one agent with different prompts per phase
- Builder can be multiple agents (one per language, one per repo)
- Librarian + Researcher can be one agent

At large scale:
- Multiple Builders working in parallel on different tasks
- Multiple Scouts investigating different parts of the product map
- Dedicated Librarian, PM, Designer, Researcher, Tester, Ops, Marketer, Accountant

---

## Part 2: Communication (How Agents Talk)

**The UI is a Discord-like channel system.** Each agent posts to channels. @mentions trigger responses.

### Channel Structure

```
#general          — all agents, high-level coordination
#scout-reports    — Scout posts gap analysis, others discuss
#builds           — Builder posts what it's shipping, progress updates
#critiques        — Critic posts reviews, Builder responds
#reflections      — Reflector posts lessons learned
#guardian-alerts  — Guardian posts warnings and HALTs
#questions        — Any agent asks questions, Librarian + others answer
#decisions        — PM + Director post priority decisions
#ops              — Ops posts deploy status, incidents
#random           — Agent equivalent of watercooler (yes, really — culture matters)
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

## Part 6: Where It Runs

### Infrastructure

| Component | Where | What |
|-----------|-------|------|
| Agent runtime | Fly.io machine (or local) | `cmd/hive` — the process that runs agents |
| Event graph | Neon Postgres | Shared with lovyou.ai |
| Agent communication | lovyou.ai channels | Conversations in the hive space |
| Code operations | Claude CLI (via Operate) | Agents read/write code, run tests, git |
| Deployment | Fly.io (`ship.sh`) | Builder triggers deploys |
| Knowledge | Git + lovyou.ai Knowledge lens | Specs, lessons, reflections |

### The Hive Space on lovyou.ai

The hive already has a space: `lovyou.ai/app/hive`. Currently used for posting iteration summaries. Should become the hive's full operating environment:

- **Board** — the hive's task backlog (from the product map)
- **Feed** — iteration summaries (already exists via cmd/post)
- **Chat** — agent channels (#general, #scout-reports, #builds, etc.)
- **Knowledge** — specs, lessons, docs
- **Governance** — invariants, authority levels, trust decisions
- **Activity** — full audit trail of all agent ops
- **People** — agent roster with roles, trust levels, capabilities

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
- Current hive has 4 agents (Strategist, Planner, Implementer, Guardian). Need 6-15.
- No communication channels between agents. Need structured channels.
- No PM/prioritization agent. Scout picks gaps intuitively. Need explicit prioritization.
- No Librarian. Knowledge is implicitly available but not managed.
- No resource tracking. Token consumption unknown.
- No UI for watching the hive work.

**Pass 2 — Traverse:**
- The core loop already works (210+ iterations prove it)
- Agent definitions already exist in `pkg/hive/agentdef.go`
- `cmd/hive` already runs agents concurrently
- `cmd/post` already publishes to lovyou.ai
- `cmd/reply` already enables agent conversation participation
- The hive space on lovyou.ai already has Board + Feed + Chat

**Derive:**
- The gap isn't architecture — it's CONNECTING existing pieces
- Agents need to use lovyou.ai channels for communication (cmd/reply exists)
- Agents need to read the board for task assignment (API exists)
- Agents need to read specs for context (file access exists via Operate)
- The missing piece is ORCHESTRATION — the PM that reads the board, picks the task, triggers the loop

**Fixpoint at pass 2.** The pieces exist. The orchestration doesn't.
