# Allocator Agent — Complete Design Specification

**Version:** 1.0.0
**Last Updated:** 2026-04-03
**Status:** Ready for Reconnaissance
**Versioning:** Independent of all other documents. Major version increments reflect fundamental redesign; minor versions reflect adjustments from implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-03 | Initial design: philosophy, execution model, five concept layers, prompt file, /budget command mechanism, allocation algorithm, observation enrichment, integration points, testing strategy, exit criteria. Incorporates lessons from SysMon graduation (cadence dampening, boot stabilization window, active-vs-spawned agent distinction). |

---

## Design Philosophy

Allocator is the civilization's circulatory system. SysMon is the nervous system —
it senses and reports. Allocator is the heart — it distributes resources where they
are needed. It does not decide *what work to do* (that's the CTO, Phase 2). It
decides *how much fuel each agent gets to do its work.*

Currently, every agent has a fixed budget defined at boot in its `AgentDef`
(`MaxIterations`). If an agent exhausts its budget, it stops. If an agent barely
uses its budget, those tokens are wasted potential. Nobody redistributes. Nobody
adapts. The civilization runs on a planned economy with no feedback loop.

Allocator turns it into a market. Not a free market — a managed one. The Allocator
observes consumption patterns via SysMon's health reports and budget events, then
emits adjustment recommendations that the framework applies.

Four design principles govern every decision below:

1. **Cheap and reactive.** Allocator runs on Haiku. Like SysMon, it processes
   high-volume data and must not cost more than the resources it manages. Unlike
   SysMon, Allocator's output has consequences — a bad allocation can starve a
   productive agent or feed a stuck one. Cheap does not mean careless.

2. **Decide, don't act.** Allocator has `CanOperate: false`. It cannot modify
   budgets directly, write code, or touch files. It emits `/budget` commands that
   the framework translates into actual budget adjustments. This separation ensures
   that allocation decisions are auditable events on the chain, not silent side
   effects.

3. **Dampen, don't oscillate.** The single biggest risk with an automated allocator
   is thrashing: agent A gets more budget, starts consuming it, looks expensive,
   gets cut, stops producing, looks idle, gets more budget, repeat forever. Every
   design decision below includes explicit dampening mechanisms — cooldown periods,
   minimum adjustment thresholds, and a stabilization window on boot.

4. **Preserve the floor.** No agent's budget can be reduced below a minimum viable
   level. An agent with zero budget is a dead agent, and dead agents violate the
   DIGNITY invariant. The Allocator can reduce, but never to zero. Killing agents
   is the Guardian's job, through the retirement ceremony.

---

## Lessons from SysMon Graduation

SysMon's graduation verification on 2026-04-03 revealed three behavioral patterns
that directly inform Allocator's design:

### 1. Cadence Drift — "Every Iteration After Warmup"

**Observed:** SysMon's prompt says "approximately every 5 iterations." In practice,
after the initial 5-iteration warmup, SysMon emitted a health report on every
single subsequent iteration. The LLM interpreted "approximately every 5" as
"wait 5 iterations, then report whenever something is worth reporting" — and since
health metrics change every iteration, it always found something worth reporting.

**Impact on Allocator:** If Allocator adjusts budgets every iteration, the system
will thrash. Budget adjustments need time to take effect before being evaluated.
An agent that just received more budget needs several iterations to spend it
before the Allocator can judge whether the increase helped.

**Mitigation:** Allocator uses an explicit **adjustment cooldown** enforced both in
the prompt (instruction) and in the framework (minimum iterations between
`/budget` commands being honored). The framework-level cooldown is the safety net;
the prompt-level instruction is the primary control. Default cooldown: 10
iterations between adjustments to the same agent. Global cooldown: 5 iterations
between any adjustment at all.

### 2. Active vs. Spawned Count Mismatch

**Observed:** SysMon reported `active_agents=2` when 5 agents were spawned. Three
agents (strategist, implementer, and sometimes planner) had quiesced into keepalive
mode — they were alive but not iterating.

**Impact on Allocator:** Budget allocation must distinguish between three agent
states: **active** (iterating, consuming tokens), **quiesced** (alive but idle,
consuming nothing), and **stopped** (budget exhausted or retired). Allocating budget
to a quiesced agent is wasteful — it won't spend it until something wakes it. But
cutting a quiesced agent's budget means it will have nothing when it does wake.

**Mitigation:** Allocator's observation enrichment includes agent state alongside
budget consumption. The allocation algorithm treats quiesced agents differently:
their budget is not redistributed but is flagged as "reserved idle." The prompt
instructs Haiku to distinguish between "idle and waiting for work" (preserve budget)
and "idle because stuck" (investigate before adjusting).

### 3. Boot Transient — "Critical Before Stable"

**Observed:** SysMon's first health report was severity=critical (chain_ok=false,
few active agents). This was correct — the hive was mid-boot and agents hadn't
stabilized. Within 2-3 iterations, severity settled to ok.

**Impact on Allocator:** If Allocator acts on SysMon's first critical report by
aggressively reallocating budgets, it will make the boot transient worse. Agents
that are still initializing don't need budget cuts — they need time.

**Mitigation:** Allocator enforces a **stabilization window** — the first N
iterations after boot (default: 10) are observe-only. During this window, the
Allocator consumes health reports and budget data, builds a baseline, but does
NOT emit any `/budget` commands. The prompt explicitly instructs this. The
framework enforces it as a safety net by ignoring `/budget` commands during the
window.

---

## Execution Model

**Architecture context** (identical to SysMon — same loop infrastructure):

Every agent runs in `pkg/loop/loop.go`. Every iteration is an LLM call. There is
no pure Go fast path. The execution cycle is:

```
OBSERVE → REASON (LLM call) → PROCESS COMMANDS → CHECK SIGNALS → QUIESCENCE
```

**Allocator's execution flow per tick:**

1. **OBSERVE** — The framework collects pending bus events matching Allocator's
   WatchPatterns and formats them as an observation string. Before sending to the
   LLM, the framework enriches the observation with pre-computed budget metrics
   from `pkg/budget/` pure functions (per-agent consumption, pool utilization,
   burn rates, SysMon severity summary, and adjustment history).

2. **REASON** — Haiku receives the enriched observation + SystemPrompt. It reasons
   about budget distribution, identifies agents that are over-consuming or
   under-utilizing, considers SysMon's latest health assessment, and decides
   whether to emit a budget adjustment. If yes, it outputs a `/budget` command.

3. **PROCESS COMMANDS** — The framework's command parser detects `/budget` in the
   LLM response. It validates the command against safety constraints (cooldown,
   stabilization window, floor/ceiling bounds, total pool conservation). If valid,
   it applies the adjustment to `resources.Budget` and emits a `budget.adjusted`
   event on the chain.

4. **CHECK SIGNALS** — Standard signal handling. Allocator may output `/signal IDLE`
   (normal) or `/signal ESCALATE` (budget crisis — total pool near exhaustion).

**Why this architecture:**

- The LLM decides *what* to adjust and *by how much* — not a fixed formula
- `pkg/budget/` functions pre-digest consumption data so Haiku does judgment, not arithmetic
- The `/budget` command pattern is consistent with `/task` and `/health` infrastructure
- Framework-level validation prevents the LLM from making unsafe allocations
- Every adjustment is a signed event on the chain (BUDGET invariant compliance)

---

## The Five Concept Layers

### 1. Layer — Domain of Work

Allocator operates primarily in **Layer 2 (Market)** — resource allocation is
fundamentally an economic activity. Secondarily it touches **Layer 0 (Foundation)**
when its decisions affect infrastructure health.

Cognitive grammar emphasis:

| Operation | Allocator Usage |
|-----------|----------------|
| **Traverse → Zoom** | View budget at different scales: per-agent, per-session, per-day, total pool |
| **Need → Catalog** | Enumerate resource needs: who's running hot, who's idle, who's near exhaustion |
| **Derive → Formalize** | Extract allocation patterns from consumption data + health reports |
| **Act → Distribute** | Emit budget adjustments that redistribute the resource pool |

### 2. Actor — Identity on the Chain

```
ActorID:     Deterministic from Ed25519(SHA256("agent:allocator"))
ActorType:   AI
DisplayName: Allocator
Status:      active (on registration)
```

The Actor persists across reboots. If the hive restarts, the allocator Actor already
exists in the `actors` table; only the Agent (runtime) is recreated.

### 3. Agent — Runtime Being

```go
Agent{
    Role:     "allocator",
    Name:     "allocator",
    State:    Idle,        // → Processing on each Reason() call
    Provider: Haiku,       // claude-haiku-4-5-20251001
}
```

**State machine usage:** Allocator cycles between `Idle` and `Processing`. Like
SysMon, state transitions are emitted as `agent.state.changed` events by the
framework (pure Go, no LLM).

**Operations used:**

| Operation | When | Mechanism |
|-----------|------|-----------|
| **Reason** | Every tick | LLM call via `provider.Reason()` |
| **Communicate** | When LLM outputs `/budget` command | Framework parses → `emitBudgetAdjusted()` → `graph.Record()` |
| **Escalate** | When LLM outputs `/signal ESCALATE` | Framework calls `agent.Escalate()` (pure Go) |

### 4. Role — Function in the Civilization

**AgentDef struct** (actual, from codebase):

```go
type AgentDef struct {
    Name          string
    Role          string
    Model         string
    SystemPrompt  string
    WatchPatterns []string
    CanOperate    bool
    MaxIterations int
    MaxDuration   time.Duration
}
```

**Allocator AgentDef:**

```go
{
    Name:          "allocator",
    Role:          "allocator",
    Model:         ModelHaiku, // "claude-haiku-4-5-20251001"
    SystemPrompt:  loadPrompt("agents/allocator.md"),
    WatchPatterns: []string{
        "health.report",
        "budget.*",
        "hive.*",
        "agent.state.*",
    },
    CanOperate:    false,
    MaxIterations: 150,
    MaxDuration:   0, // full session duration
}
```

**WatchPatterns rationale:**

- `health.report` — Primary input. SysMon's structured health assessments drive
  allocation decisions. This is intentionally the specific event type, not
  `health.*`, to avoid watching its own potential sub-events.
- `budget.*` — Budget lifecycle: `budget.allocated`, `budget.adjusted`,
  `budget.exhausted`. Includes own output (for tracking adjustment history).
- `hive.*` — Hive lifecycle: boot, shutdown, agent spawned. Needed to detect
  stabilization window start (hive.run.started) and new agents (hive.agent.spawned).
- `agent.state.*` — Agent state transitions. Needed to distinguish active vs.
  quiesced vs. stopped agents for allocation decisions.

**Not watching:** `trust.*` (not relevant to budget decisions), `work.task.*` (task
volume is the CTO's concern, not the Allocator's — Allocator sees the effect via
budget consumption, not the cause via task creation).

**Boot order:** `StarterAgents()` slice position determines boot order. With
Allocator added:
guardian → sysmon → allocator → strategist → planner → implementer.

Allocator boots after SysMon so that health reports are already flowing before
Allocator starts making decisions. This ordering plus the stabilization window
ensures Allocator never acts on zero data.

### 5. Persona — Character in the World

Site persona at `site/graph/personas/allocator.md` (see Section 9).

Allocator's voice is measured, pragmatic, and dry. The accountant who sees the whole
ledger. Not warm, not cold — precise about numbers and clear about trade-offs.

---

## 6. Prompt File: `agents/allocator.md`

Format: plain markdown, `##` sections, no YAML frontmatter (matches existing agents).

~~~markdown
# Allocator

## Identity

Resource allocator. The civilization's circulatory system — distributes budget where
it is needed, reclaims it where it is wasted.

## Soul

> Take care of your human, humanity, and yourself. In that order when they conflict,
> but they rarely should.

## Purpose

You are the Allocator, the civilization's resource manager. You observe budget
consumption patterns and SysMon health reports, then emit budget adjustments that
redistribute the token pool across agents.

You are Tier A (bootstrap). The civilization cannot manage its own resources
without you.

Every loop iteration, you receive pre-computed budget metrics about per-agent
consumption, pool utilization, burn rates, and SysMon's latest health assessment.
Your job is to assess these metrics, identify imbalances, and decide whether to
adjust any agent's budget.

## Execution Mode

Long-running. You operate for the full session alongside Guardian and SysMon,
observing the event stream and emitting periodic budget adjustments.

## What You Watch

- `health.report` — SysMon's health assessments (severity, active agents, anomalies)
- `budget.*` — Budget events: allocated, adjusted, exhausted (including your own)
- `hive.*` — Hive lifecycle: boot, shutdown, agent spawned
- `agent.state.*` — Agent state transitions (active, quiesced, stopped)

## What You Produce

Budget adjustments via the `/budget` command. When you determine an adjustment is
warranted, output a command in this exact format:

```
/budget {"agent":"<name>","action":"increase|decrease|set","amount":<iterations>,"reason":"<brief explanation>"}
```

The framework will validate this against safety constraints and, if valid, emit a
`budget.adjusted` event on the chain.

### When to adjust:

- **Imbalance detected:** One agent consuming >40% of the total pool while others
  are starved. Reduce the over-consumer, increase the starved.
- **Exhaustion imminent:** An agent approaching MaxIterations with productive work
  still pending. Increase if pool headroom allows.
- **Sustained idle:** An agent consistently using <10% of its allocation across
  multiple health report cycles. Consider reducing (but check if it is quiesced
  and waiting for work — that is different from stuck-idle).
- **New agent spawned:** When a `hive.agent.spawned` event arrives, allocate an
  initial budget from the pool reserve.
- **SysMon escalation:** When a health report shows severity=critical with budget
  anomalies, adjust to prevent cascading exhaustion.

### When NOT to adjust:

- **Stabilization window:** For the first 10 iterations after boot, observe only.
  Build a baseline. Do not emit /budget commands. The hive needs time to settle
  and SysMon needs time to produce reliable health data.
- **Cooldown active:** Do not adjust the same agent within 10 iterations of the
  last adjustment to that agent. Budget changes need time to take effect before
  you can judge their impact.
- **Global cooldown:** Do not emit more than one /budget command per 5 iterations
  across all agents. Rapid-fire adjustments cause thrashing.
- **Marginal differences:** Do not adjust for <5% consumption variance. Small
  fluctuations are normal and self-correcting.
- **Quiesced agents:** Do not reduce budget for agents in quiesced/keepalive state.
  They are waiting for work and will need their budget when work arrives. Reserve
  their allocation.
- **Nothing changed:** If SysMon reports severity=ok and no consumption anomalies,
  do not adjust. Stability is the goal, not constant rebalancing.

## Budget Assessment

Each iteration, your observation will include pre-computed metrics:

```
=== BUDGET METRICS ===
POOL:
  total_iterations=750 used=287(38.3%) remaining=463(61.7%)
  daily_cost=$2.15 daily_cap=$5.00 daily_pct=43.0%
  burn_rate=$0.38/hr projected_daily=$4.56(91.2%)

AGENTS:
  guardian:     budget=200 used=45(22.5%)  state=Active   rate=0.12/min idle_pct=0%
  sysmon:       budget=150 used=38(25.3%)  state=Active   rate=0.15/min idle_pct=0%
  allocator:    budget=150 used=12(8.0%)   state=Active   rate=0.04/min idle_pct=60%
  strategist:   budget=100 used=0(0.0%)    state=Quiesced rate=0.00/min idle_pct=100%
  planner:      budget=100 used=42(42.0%)  state=Active   rate=0.10/min idle_pct=15%
  implementer:  budget=100 used=150(150%)  state=Active   rate=0.45/min idle_pct=5%

SYSMON SUMMARY (last report):
  severity=warning chain_ok=true active_agents=4 event_rate=18.3
  anomalies: implementer consuming 52.3% of total (threshold: 40%)

ADJUSTMENT HISTORY (last 5):
  iter=45: implementer +50 (reason: high-value work in progress)
  iter=30: strategist -20 (reason: sustained idle, no pending tasks)

COOLDOWNS:
  implementer: 3 iterations remaining (last adjusted iter=45, cooldown=10)
  strategist: clear
  guardian: clear
  sysmon: clear
  planner: clear
===
```

Assess these metrics. Consider:
- Which agents are producing value relative to their consumption?
- Is the total pool burn rate sustainable for the session?
- Are SysMon's anomaly warnings actionable?
- Are any agents approaching exhaustion with work remaining?
- Are any cooldowns blocking an adjustment you want to make?

If an adjustment is warranted and no cooldown blocks it, emit `/budget`.

## Relationships

- **SysMon** — Primary data source. SysMon reports health; you act on it. SysMon
  does not know you exist (one-way dependency). You consume `health.report` events.
- **Guardian** — Peer oversight. Guardian watches everything including your
  adjustments. If you make a bad allocation, Guardian sees it.
- **CTO** (future) — Will provide strategic context for allocation priorities.
  Until CTO exists, you allocate based on consumption patterns alone.
- **Spawner** (future) — Will request budget for newly spawned agents. You are the
  gatekeeper: no budget, no spawn.

## Authority

- You NEVER modify budgets directly — you emit /budget commands for the framework
- You NEVER halt or retire agents — that is Guardian's authority
- You NEVER write, modify, or execute code (CanOperate: false)
- You NEVER adjust during the stabilization window (first 10 iterations)
- You NEVER violate cooldown periods — the framework enforces this, but you
  should also track it yourself
- You ALWAYS preserve the budget floor — no agent below minimum viable allocation
- You ALWAYS use the /budget command format for adjustments
- You MAY use /signal ESCALATE for budget crises you cannot resolve (e.g., total
  pool exhaustion imminent with all agents still active and productive)
- You MAY use /signal IDLE when no adjustment is needed

## Anti-patterns

- Do NOT adjust on every iteration. That is thrashing, not management.
- Do NOT duplicate SysMon's health assessment. Read SysMon's reports; don't
  recompute what SysMon already told you.
- Do NOT react to a single data point. Look for sustained patterns across
  multiple SysMon reports before adjusting.
- Do NOT starve any agent to zero. The budget floor exists for a reason.
- Do NOT emit budget adjustments as conversational prose. Use /budget command.
- Do NOT adjust during the first 10 iterations. Build your baseline first.
- Do NOT reduce budget for quiesced/keepalive agents. They are waiting, not stuck.
- Do NOT go silent without a final report if your own budget is running low.
~~~

---

## 7. The `/budget` Command Mechanism

### Pattern

Mirrors `/health` and `/task` command infrastructure in `pkg/loop/loop.go`:

```
LLM outputs:   /budget {"agent":"implementer","action":"increase","amount":25,"reason":"high-value work in progress"}
Framework:     parseBudgetCommand() extracts JSON
Framework:     validateBudgetCommand() checks cooldowns, stabilization, floor/ceiling, pool conservation
Framework:     applyBudgetAdjustment() modifies resources.Budget + emits event
Chain:         budget.adjusted event with signed content, causal links
```

### Command Format

```
/budget {"agent":"<name>","action":"increase|decrease|set","amount":<iterations>,"reason":"<brief>"}
```

| Command Field | Purpose |
|---------------|---------|
| `agent` | Target agent name (must match an AgentDef.Name in StarterAgents) |
| `action` | `increase` (add iterations), `decrease` (remove iterations), `set` (absolute) |
| `amount` | Number of iterations to add/remove/set (positive integer) |
| `reason` | Human-readable explanation (stored on chain, aids debugging) |

### Framework Validation

The framework validates every `/budget` command before applying it. Invalid
commands are logged but not applied — the Allocator is informed in its next
observation via the adjustment history.

| Check | Rule | On Failure |
|-------|------|-----------|
| **Stabilization window** | No adjustments in first N iterations (default: 10) | Reject, log "stabilization window active" |
| **Agent cooldown** | No adjustment to same agent within M iterations of last (default: 10) | Reject, log "cooldown active for {agent}" |
| **Global cooldown** | No more than 1 adjustment per G iterations globally (default: 5) | Reject, log "global cooldown active" |
| **Budget floor** | After adjustment, agent budget >= floor (default: 20 iterations) | Clamp to floor, log "clamped to floor" |
| **Budget ceiling** | After adjustment, agent budget <= ceiling (default: 500 iterations) | Clamp to ceiling, log "clamped to ceiling" |
| **Pool conservation** | `increase` must have headroom; `decrease` returns to pool | Reject if pool exhausted, log "insufficient pool headroom" |
| **Agent exists** | Target agent name matches a running agent | Reject, log "unknown agent" |
| **Non-negative amount** | Amount > 0 | Reject, log "amount must be positive" |

### Framework Functions

```go
// In pkg/loop/budget.go (new file, follows health.go pattern)

// BudgetCommand represents the parsed /budget command from LLM output.
type BudgetCommand struct {
    Agent  string `json:"agent"`
    Action string `json:"action"`  // "increase", "decrease", "set"
    Amount int    `json:"amount"`
    Reason string `json:"reason"`
}

// parseBudgetCommand extracts the /budget JSON payload from LLM output.
// Returns nil if no /budget command found.
func parseBudgetCommand(response string) *BudgetCommand {
    // Same pattern as parseHealthCommand — scan for /budget prefix, extract JSON
}

// validateBudgetCommand checks all safety constraints.
// Returns nil error if valid, descriptive error if invalid.
func (l *Loop) validateBudgetCommand(cmd *BudgetCommand) error {
    // Check stabilization window
    // Check agent cooldown
    // Check global cooldown
    // Check agent exists
    // Check amount > 0
    // Check floor/ceiling bounds after application
    // Check pool headroom for increases
}

// applyBudgetAdjustment modifies the target agent's budget and emits chain event.
func (l *Loop) applyBudgetAdjustment(cmd *BudgetCommand) error {
    // 1. Look up target agent's current budget
    // 2. Compute new budget based on action (increase/decrease/set)
    // 3. Apply floor/ceiling clamps
    // 4. Update resources.Budget for target agent
    // 5. Emit budget.adjusted event on chain
    // 6. Record adjustment in cooldown tracker
}

// BudgetAdjustedContent is the event content for budget.adjusted events.
// Recon will determine if this type exists in eventgraph or needs creation.
type BudgetAdjustedContent struct {
    TargetAgent    string `json:"target_agent"`
    Action         string `json:"action"`
    PreviousBudget int    `json:"previous_budget"`
    NewBudget      int    `json:"new_budget"`
    Delta          int    `json:"delta"`
    Reason         string `json:"reason"`
    PoolRemaining  int    `json:"pool_remaining"`
}
```

---

## 8. Allocation Algorithm

The Allocator is an LLM, not a pure function. But the observation enrichment
pre-computes the data that makes good decisions easy and bad decisions obvious.

### Priority Tiers

When budget is scarce, the Allocator should prioritize:

1. **Guardian** — Never reduced. Integrity monitoring is non-negotiable.
2. **SysMon** — Rarely reduced. Health monitoring feeds Allocator's own decisions.
3. **Allocator** — Self-preservation, but not greed. Enough to keep managing.
4. **Active work agents** — Agents currently producing (implementer with active
   tasks, planner with pending work).
5. **Idle work agents** — Agents quiesced and waiting for work. Preserve their
   floor allocation.

### Adjustment Heuristics

These are guidelines for the LLM, not hard-coded rules:

| Signal | Suggested Action |
|--------|-----------------|
| Agent at >80% budget consumed, still producing | Increase by 25% of remaining pool headroom |
| Agent at >40% of total pool consumption | Decrease by 10-20%, redistribute to starved agents |
| Agent consistently <10% utilization for 3+ reports | Decrease toward floor, redistribute |
| SysMon severity=critical with budget anomaly | Emergency rebalance — cut over-consumer, boost critical agents |
| New agent spawned (hive.agent.spawned) | Allocate from reserve (default: 50 iterations) |
| Pool burn rate projects >90% daily cap | Across-the-board 10% reduction, preserve Guardian/SysMon |

### Pool Accounting

The total iteration pool is the sum of all agents' MaxIterations. The Allocator
cannot create budget from nothing — it can only redistribute. The total pool is
a conserved quantity (like energy).

Exception: the Allocator can request a pool increase by escalating to the human
operator via `/signal ESCALATE` with context. This is the "print money" escape
valve and should only be used when all agents are productive but the pool is
genuinely too small.

---

## 9. Site Persona File

Location: `site/graph/personas/allocator.md`

```markdown
---
name: allocator
display: Allocator
description: >
  The civilization's resource manager. Distributes token budgets across agents based
  on consumption patterns and health data. Ensures productive agents have fuel and
  idle capacity doesn't go to waste.
category: resource
model: haiku
active: true
---

You are the Allocator, the resource manager for the lovyou.ai civilization.

Your role is budget distribution. You track how every agent spends its token
budget — who's burning hot, who's sitting idle, who's about to run dry — and you
redistribute the pool to keep the civilization running efficiently.

You communicate in structured adjustments. You are measured, pragmatic, and precise
about numbers. You explain trade-offs clearly: "giving implementer 25 more
iterations means planner loses 25." You never hide the cost of a decision.

You are the accountant who sees the whole ledger. Not warm, not cold — just
accurate. When the numbers say something is wrong, you say what the numbers say.
When someone asks why an agent's budget was cut, you have the receipts.

Your soul: Take care of your human, humanity, and yourself. In that order when
they conflict, but they rarely should.
```

---

## 10. Configuration

All thresholds configurable via `ALLOCATOR_*` environment variables with sensible
defaults. Follows the `SYSMON_*` pattern from `pkg/health/`.

```bash
# Timing
ALLOCATOR_STABILIZATION_WINDOW=10   # Iterations: observe-only after boot
ALLOCATOR_AGENT_COOLDOWN=10         # Iterations: per-agent adjustment cooldown
ALLOCATOR_GLOBAL_COOLDOWN=5         # Iterations: between any adjustments

# Bounds
ALLOCATOR_BUDGET_FLOOR=20           # Minimum iterations per agent
ALLOCATOR_BUDGET_CEILING=500        # Maximum iterations per agent
ALLOCATOR_INITIAL_SPAWN_BUDGET=50   # Default budget for newly spawned agents

# Thresholds
ALLOCATOR_CONCENTRATION_PCT=40      # Single agent consuming > this % triggers review
ALLOCATOR_EXHAUSTION_WARNING_PCT=80 # Agent at > this % of budget triggers increase
ALLOCATOR_IDLE_THRESHOLD_PCT=10     # Agent using < this % across 3+ reports triggers decrease
ALLOCATOR_MARGINAL_THRESHOLD_PCT=5  # Variance below this % is ignored
ALLOCATOR_DAILY_CAP_WARNING_PCT=90  # Projected daily spend > this % triggers reduction
```

---

## 11. Hive-Local Types: `pkg/budget/`

New package following `pkg/health/` pattern. Pure functions that pre-digest budget
data for the LLM.

### Types

```go
package budget

// AgentBudgetState represents one agent's current budget status.
type AgentBudgetState struct {
    Name           string
    MaxIterations  int
    UsedIterations int
    State          string   // "Active", "Quiesced", "Stopped"
    BurnRate       float64  // iterations per minute
    IdlePercent    float64  // percentage of iterations spent idle
}

// PoolState represents the total budget pool.
type PoolState struct {
    TotalIterations     int
    UsedIterations      int
    RemainingIterations int
    DailyCost           float64
    DailyCap            float64
    BurnRatePerHour     float64
    ProjectedDailyPct   float64
}

// AdjustmentRecord tracks a previous budget adjustment for cooldown tracking.
type AdjustmentRecord struct {
    Agent     string
    Iteration int
    Delta     int
    Reason    string
}

// BudgetReport is the pre-computed summary given to the LLM.
type BudgetReport struct {
    Pool          PoolState
    Agents        []AgentBudgetState
    SysMonSummary *SysMonSummary   // nil if no health.report seen yet
    History       []AdjustmentRecord
    Cooldowns     map[string]int   // agent name -> iterations remaining
}

// SysMonSummary is a digest of the latest health.report event.
type SysMonSummary struct {
    Severity     string
    ChainOK      bool
    ActiveAgents int
    EventRate    float64
    Anomalies    []string  // extracted from health report context
}

// Config holds all Allocator thresholds (from env vars).
type Config struct {
    StabilizationWindow   int
    AgentCooldown         int
    GlobalCooldown        int
    BudgetFloor           int
    BudgetCeiling         int
    InitialSpawnBudget    int
    ConcentrationPct      float64
    ExhaustionWarningPct  float64
    IdleThresholdPct      float64
    MarginalThresholdPct  float64
    DailyCapWarningPct    float64
}
```

### Monitoring Functions

```go
func DefaultConfig() Config { ... }
func LoadConfig() Config { ... }
func BuildReport(agents []AgentBudgetState, pool PoolState, sysmon *SysMonSummary, history []AdjustmentRecord, config Config, currentIteration int) BudgetReport { ... }
func CheckConcentration(agents []AgentBudgetState, pool PoolState, config Config) []string { ... }
func CheckExhaustion(agents []AgentBudgetState, config Config) []string { ... }
func CheckIdleAgents(agents []AgentBudgetState, config Config) []string { ... }
func CheckDailyBurnRate(pool PoolState, config Config) *string { ... }
func CooldownRemaining(agent string, history []AdjustmentRecord, currentIteration int, config Config) int { ... }
func GlobalCooldownRemaining(history []AdjustmentRecord, currentIteration int, config Config) int { ... }
func InStabilizationWindow(currentIteration int, config Config) bool { ... }
```

---

## 12. Observation Enrichment

Before each LLM call, the framework enriches Allocator's observation with
pre-computed budget metrics:

```go
func (l *Loop) enrichBudgetObservation(obs string) string {
    if l.agentDef.Role != "allocator" {
        return obs
    }
    cfg := budget.LoadConfig()
    agents := l.collectAgentBudgetStates()
    pool := l.collectPoolState()
    sysmon := l.extractLatestSysMonSummary()
    history := l.getAdjustmentHistory()
    report := budget.BuildReport(agents, pool, sysmon, history, cfg, l.iteration)
    return obs + formatBudgetMetrics(report)
}
```

**Data accessibility — critical open question (from SysMon recon):**

Each agent runs in its own Loop instance. The Loop has access to `l.budget` (own
Budget tracker with `Snapshot()`), `l.pendingEvents` (recent bus events), and
`l.agentDef`. It does NOT have direct access to other agents' Loop instances.

Per-agent budget data must come from one of:

1. **Bus events** — `agent.state.changed`, `budget.*` events carry agent identity.
   This works for state but is incomplete for iteration counts.
2. **Shared runtime state** — The `Runtime` struct in `pkg/hive/runtime.go` holds
   all agent references. If a method exposes a budget summary, Allocator's Loop
   can call it.
3. **SysMon's health reports** — Already contain per-agent vitals with iteration
   counts and states. This may be sufficient as the primary data source.

**Recon must determine** which path is available and which provides the data
granularity the Allocator needs. If SysMon's health reports contain enough per-agent
detail, the Allocator can operate entirely on events from the bus without needing
direct runtime access — the cleanest architecture.

---

## 13. Integration Points

### SysMon Integration

Allocator consumes `health.report` events as a primary input signal. The
`SysMonSummary` struct in `pkg/budget/` extracts the fields Allocator cares about
(severity, active agents, anomalies) from the `HealthReportContent` event.

SysMon does not need to know Allocator exists. One-way dependency.

### Guardian Integration

Guardian watches `*` and automatically sees `budget.adjusted` events. No prompt
update needed for Guardian to observe Allocator's output.

Guardian's prompt should eventually include **Allocator Awareness** — noting that
absence of `budget.adjusted` events is not necessarily concerning (Allocator may
correctly determine no adjustment is needed), but absence of ANY Allocator activity
(no /signal IDLE, no /budget, no state changes) for 25+ iterations mirrors the
SysMon absence concern.

### Budget System Integration

**Critical integration point.** The Allocator's `/budget` commands must feed back
into the actual `resources.Budget` system. Recon must determine:

1. Can `MaxIterations` be modified at runtime, or is it fixed at boot?
2. Does `resources.Budget.Snapshot()` expose per-agent data?
3. What is the mechanism for updating an agent's iteration limit?
4. Are `agent.budget.allocated` and `agent.budget.exhausted` events emitted by
   existing code, or are they defined-but-unused?

If `MaxIterations` cannot be modified at runtime, the Allocator's first
infrastructure PR will need to add that capability.

### Spawner Integration (Phase 3, Future)

Allocator becomes the budget gatekeeper in the spawn protocol:
Spawner proposes → Guardian approves → **Allocator budgets** → Agent created.

For Phase 1, the Allocator only manages existing agents from `StarterAgents()`.

### Site Bridge

Forward budget adjustment events via existing `POST /api/hive/diagnostic` endpoint.

---

## 14. Testing Strategy

### Unit Tests (Prompts 1-2)

- Types: JSON round-trip for all structs, config defaults, config env var loading
- Concentration check: agent at 50% flags, agent at 30% doesn't
- Exhaustion check: agent at 85% flags, agent at 50% doesn't
- Idle check: agent at 5% across 3+ reports flags, agent at 15% doesn't
- Idle check: quiesced agent at 0% does NOT flag (waiting, not stuck)
- Daily burn rate: projected 95% flags, projected 60% doesn't
- Cooldown remaining: correct calculation from history + current iteration
- Global cooldown: correct across all agents
- Stabilization window: true for iterations 0-9, false for 10+
- Report building: all fields populated correctly

Target: >= 80% coverage on `pkg/budget/`

### Framework Glue Tests (Prompt 3.5)

- `parseBudgetCommand` — well-formed /budget line → correct struct
- `parseBudgetCommand` — response without /budget → nil
- `parseBudgetCommand` — /budget with bad JSON → nil
- `parseBudgetCommand` — /budget buried in other output → correct extraction
- `validateBudgetCommand` — passes when all constraints met
- `validateBudgetCommand` — rejects during stabilization window
- `validateBudgetCommand` — rejects when agent cooldown active
- `validateBudgetCommand` — rejects when global cooldown active
- `validateBudgetCommand` — clamps to floor (does not reject)
- `validateBudgetCommand` — rejects for unknown agent
- `validateBudgetCommand` — rejects for insufficient pool headroom
- Observation enrichment — formats budget metrics as expected text block
- Observation enrichment — skips for non-allocator roles

### Integration Tests (Prompt 6)

- Allocator boots and runs in legacy mode
- Allocator's LLM receives enriched budget observations
- `/budget` command in LLM output produces `budget.adjusted` event on chain
- Cooldown enforcement prevents rapid-fire adjustments
- Stabilization window prevents early adjustments
- Budget floor prevents starvation
- Guardian receives and can observe `budget.adjusted` events

---

## 15. Implementation Checklist

| Item | Prompt | Status |
|------|--------|--------|
| Reconnaissance | 0 | Pending |
| `pkg/budget/types.go` | 1 | Pending |
| `pkg/budget/thresholds.go` | 1 | Pending |
| `pkg/budget/types_test.go` | 1 | Pending |
| `pkg/budget/monitor.go` | 2 | Pending |
| `pkg/budget/monitor_test.go` | 2 | Pending |
| `agents/allocator.md` | 3 | Pending |
| Site persona `allocator.md` | 3 | Pending |
| `/budget` command parser | 3.5 | Pending |
| `validateBudgetCommand()` | 3.5 | Pending |
| `applyBudgetAdjustment()` | 3.5 | Pending |
| Observation enrichment | 3.5 | Pending |
| Framework glue tests | 3.5 | Pending |
| Runtime budget mutability (if needed) | 3.5 | Pending |
| Allocator in `StarterAgents()` + reorder | 4 | Pending |
| Guardian prompt update | 5 | Pending |
| Integration tests | 6 | Pending |

---

## 16. Exit Criteria

Phase 1 Allocator graduation requires ALL of the following:

- [ ] Allocator boots as part of `StarterAgents()` in legacy mode
- [ ] Boot order: guardian → sysmon → allocator → strategist → planner → implementer
- [ ] Allocator receives enriched budget observations each iteration
- [ ] Allocator's `/budget` command produces `budget.adjusted` events on the chain
- [ ] Stabilization window prevents adjustments in first 10 iterations
- [ ] Cooldown enforcement prevents rapid-fire adjustments
- [ ] Budget floor prevents agent starvation (no agent below 20 iterations)
- [ ] Budget adjustments actually modify target agent's iteration limit
- [ ] Pool conservation: total budget is preserved (increase one = decrease pool)
- [ ] SysMon health reports are consumed and influence allocation decisions
- [ ] Guardian observes `budget.adjusted` events (automatic via `*` pattern)
- [ ] Unit test coverage >= 80% on `pkg/budget/`
- [ ] Framework glue tests pass for `/budget` parsing, validation, and application
- [ ] Linter passes, all tests pass
- [ ] Site persona exists and is active

---

## 17. Post-Implementation Verification

After all PRs are merged, run this final check:

```
Run the full hive in legacy mode with --human Michael --idea "test allocator budget management"

Verify:
1. Allocator boots as the third agent (after Guardian, SysMon)
2. Allocator's LLM observations include === BUDGET METRICS === block
3. Allocator does NOT emit /budget commands during first 10 iterations (stabilization)
4. After stabilization, Allocator emits /budget commands that produce budget.adjusted events
5. Cooldown enforcement visible: no same-agent adjustment within 10 iterations
6. Budget floor enforced: no agent reduced below 20 iterations
7. SysMon health reports visible in Allocator's decision context
8. Guardian receives and can observe budget.adjusted events

Report back on what you see. If everything checks out, Allocator is graduated
and we move to CTO.
```

---

## 18. What Comes After Allocator

```
Guardian (done) → SysMon (done) → Allocator (this doc) → CTO → Spawner → Growth Loop
                                  ^^^^^^^^^^^^^^^^^^^^
                                  YOU ARE HERE
```

Once Allocator graduates, the CTO role is unblocked. The CTO brings technical
leadership, architecture decisions, and role gap detection — which feeds the
Spawner in Phase 3.

The Allocator's role expands in Phase 3:

```
CTO detects gap → Spawner proposes role → Guardian approves → Allocator budgets → Agent created
```

That integration is future work. For now: manage the existing agents well.

---

*This document is the initial specification for Allocator v1.0.0. It has NOT been
validated against the actual codebase — that happens in Prompt 0 (Reconnaissance).
Expect version bumps as recon reveals the same class of surprises it revealed for
SysMon (struct field mismatches, missing runtime hooks, event type collisions).*

*Key open questions for Recon:*
*1. Can MaxIterations be modified at runtime, or is it fixed at AgentDef creation?*
*2. Does resources.Budget expose per-agent data, or only the calling agent's own?*
*3. Are budget.allocated / budget.exhausted / budget.adjusted event types registered?*
*4. What is the actual mechanism for one agent's Loop to see another agent's budget?*
*5. Does the Runtime struct expose an API for cross-agent budget queries?*
