# Allocator Agent — Complete Design Specification

**Version:** 1.2.0
**Last Updated:** 2026-04-04
**Status:** Implementation In Progress
**Versioning:** Independent of all other documents. Major version increments reflect fundamental redesign; minor versions reflect adjustments from implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-03 | Initial design: philosophy, execution model, five concept layers, prompt file, /budget command mechanism, allocation algorithm, observation enrichment, integration points, testing strategy, exit criteria. Incorporates lessons from SysMon graduation (cadence dampening, boot stabilization window, active-vs-spawned agent distinction). |
| 1.1.0 | 2026-04-04 | Post-recon (Prompt 0): WatchPatterns corrected from budget.* to agent.budget.*. MaxIterations confirmed immutable — added BudgetRegistry. budget.adjusted confirmed missing — must create as agent.budget.adjusted. SysMon enrichment confirmed self-referential — Allocator needs BudgetRegistry. Added Prompt 0.5 for infrastructure. |
| 1.2.0 | 2026-04-04 | Self-contained revision: all "unchanged from v1.0.0" references replaced with actual content. Marked Prompt 0.5 and Prompt 1 COMPLETE. BudgetRegistry lives in pkg/resources (not pkg/hive) per implementation. Persona format corrected to match actual codebase (no YAML frontmatter). |

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
single subsequent iteration.

**Mitigation:** Allocator uses an explicit **adjustment cooldown** enforced both in
the prompt (instruction) and in the framework (minimum iterations between
`/budget` commands being honored). Default cooldown: 10 iterations between
adjustments to the same agent. Global cooldown: 5 iterations between any
adjustment at all.

### 2. Active vs. Spawned Count Mismatch

**Observed:** SysMon reported `active_agents=2` when 5 agents were spawned. Three
agents had quiesced into keepalive mode.

**Mitigation:** Allocator's observation enrichment includes agent state alongside
budget consumption. Quiesced agents are treated as "reserved idle" — budget
preserved, not redistributed.

### 3. Boot Transient — "Critical Before Stable"

**Observed:** SysMon's first health report was severity=critical during boot.

**Mitigation:** Allocator enforces a **stabilization window** — the first 10
iterations after boot are observe-only. No `/budget` commands emitted.

---

## Recon Findings (Prompt 0, 2026-04-04)

Seven findings that reshaped the implementation approach:

### R1. Event Namespace Mismatch
Actual budget event types are `agent.budget.allocated` and
`agent.budget.exhausted`, NOT `budget.*`. WatchPatterns use `agent.budget.*`.

### R2. MaxIterations Is Immutable
`resources.Budget` fields are all private. No setter existed.
**Resolved:** `SetMaxIterations(int)` and `MaxIterations()` added to Budget
in Prompt 0.5 (committed c448b10).

### R3. BudgetSnapshot Lacks Limits
`Snapshot()` returns consumed values only (TokensUsed, Iterations, CostUSD, etc.).
**Resolved:** BudgetRegistry stores MaxIterations from AgentDef alongside the
Budget reference. Limits come from registry, consumption from Snapshot().

### R4. Runtime Doesn't Store Loops — BudgetRegistry Added
Runtime created Loops and passed them to RunConcurrent() without retaining
references. Each Loop was completely isolated.
**Resolved:** BudgetRegistry created in pkg/resources (committed c448b10).
Runtime creates registry at boot, registers each agent's Budget at spawn,
passes registry to Loop via Config.BudgetRegistry.

### R5. agent.budget.adjusted Event Type Missing
**Resolved:** Created in eventgraph with AgentBudgetAdjustedContent struct
(committed f9b4cdc).

### R6. SysMon Enrichment Is Self-Referential Only
enrichHealthObservation() uses only own budget and own events. Allocator uses
BudgetRegistry instead — completely different data source.

### R7. EmitBudgetAllocated Doesn't Set TimeLimit
Minor inconsistency. No impact on Allocator.

---

## Execution Model

Every agent runs in `pkg/loop/loop.go`. Every iteration is an LLM call. There is
no pure Go fast path. The execution cycle is:

```
OBSERVE → REASON (LLM call) → PROCESS COMMANDS → CHECK SIGNALS → QUIESCENCE
```

**Allocator's execution flow per tick:**

1. **OBSERVE** — The framework collects pending bus events matching Allocator's
   WatchPatterns and formats them as an observation string. Before sending to the
   LLM, the framework enriches the observation with pre-computed budget metrics
   from `pkg/budget/` pure functions, using data from the **BudgetRegistry**.

2. **REASON** — Haiku receives the enriched observation + SystemPrompt. It reasons
   about budget distribution and decides whether to emit a budget adjustment.

3. **PROCESS COMMANDS** — The framework's command parser detects `/budget` in the
   LLM response (inserted at line 224-227 in loop.go, between health command
   processing and signal checking). It validates against safety constraints,
   then calls `BudgetRegistry.AdjustMaxIterations()` and emits an
   `agent.budget.adjusted` event on the chain.

4. **CHECK SIGNALS** — Standard signal handling.

---

## The Five Concept Layers

### 1. Layer — Domain of Work

Allocator operates primarily in **Layer 2 (Market)** — resource allocation.

### 2. Actor — Identity on the Chain

```
ActorID:     Deterministic from Ed25519(SHA256("agent:allocator"))
ActorType:   AI
DisplayName: Allocator
Status:      active (on registration)
```

### 3. Agent — Runtime Being

```go
Agent{
    Role:     "allocator",
    Name:     "allocator",
    State:    Idle,
    Provider: Haiku,       // claude-haiku-4-5-20251001
}
```

### 4. Role — Function in the Civilization

**Allocator AgentDef:**

```go
{
    Name:          "allocator",
    Role:          "allocator",
    Model:         ModelHaiku,
    SystemPrompt:  loadPrompt("agents/allocator.md"),
    WatchPatterns: []string{
        "health.report",
        "agent.budget.*",
        "hive.*",
        "agent.state.*",
    },
    CanOperate:    false,
    MaxIterations: 150,
    MaxDuration:   0,
}
```

**Note:** SysMon validation found that SystemPrompt is inlined in agentdef.go,
not loaded from agents/*.md at runtime. The agents/*.md file exists as
documentation; the runtime source is the inline mission template in StarterAgents().
Follow whichever pattern Claude Code finds in the codebase.

**Boot order:** guardian → sysmon → allocator → strategist → planner → implementer.

### 5. Persona — Character in the World

Allocator's voice is measured, pragmatic, and dry. The accountant who sees the
whole ledger.

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
- `agent.budget.*` — Budget events: allocated, adjusted, exhausted (including your own)
- `hive.*` — Hive lifecycle: boot, shutdown, agent spawned
- `agent.state.*` — Agent state transitions (active, quiesced, stopped)

## What You Produce

Budget adjustments via the `/budget` command. When you determine an adjustment is
warranted, output a command in this exact format:

```
/budget {"agent":"<n>","action":"increase|decrease|set","amount":<iterations>,"reason":"<brief explanation>"}
```

The framework will validate this against safety constraints and, if valid, emit an
`agent.budget.adjusted` event on the chain.

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
  Build a baseline. Do not emit /budget commands.
- **Cooldown active:** Do not adjust the same agent within 10 iterations of the
  last adjustment to that agent.
- **Global cooldown:** Do not emit more than one /budget command per 5 iterations.
- **Marginal differences:** Do not adjust for <5% consumption variance.
- **Quiesced agents:** Do not reduce budget for agents in quiesced/keepalive state.
  They are waiting for work. Reserve their allocation.
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
  guardian:     max=200 used=45(22.5%)  state=Active   rate=0.12/min idle_pct=0%
  sysmon:       max=150 used=38(25.3%)  state=Active   rate=0.15/min idle_pct=0%
  allocator:    max=150 used=12(8.0%)   state=Active   rate=0.04/min idle_pct=60%
  strategist:   max=100 used=0(0.0%)    state=Quiesced rate=0.00/min idle_pct=100%
  planner:      max=100 used=42(42.0%)  state=Active   rate=0.10/min idle_pct=15%
  implementer:  max=100 used=150(150%)  state=Active   rate=0.45/min idle_pct=5%

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
- **Spawner** (future) — Will request budget for newly spawned agents. You are the
  gatekeeper: no budget, no spawn.

## Authority

- You NEVER modify budgets directly — you emit /budget commands for the framework
- You NEVER halt or retire agents — that is Guardian's authority
- You NEVER write, modify, or execute code (CanOperate: false)
- You NEVER adjust during the stabilization window (first 10 iterations)
- You NEVER violate cooldown periods
- You ALWAYS preserve the budget floor — no agent below minimum viable allocation
- You ALWAYS use the /budget command format for adjustments
- You MAY use /signal ESCALATE for budget crises you cannot resolve
- You MAY use /signal IDLE when no adjustment is needed

## Anti-patterns

- Do NOT adjust on every iteration. That is thrashing, not management.
- Do NOT duplicate SysMon's health assessment. Read SysMon's reports.
- Do NOT react to a single data point. Look for sustained patterns.
- Do NOT starve any agent to zero. The budget floor exists for a reason.
- Do NOT emit budget adjustments as conversational prose. Use /budget command.
- Do NOT adjust during the first 10 iterations. Build your baseline first.
- Do NOT reduce budget for quiesced/keepalive agents. They are waiting, not stuck.
- Do NOT go silent without a final report if your own budget is running low.
~~~

---

## 7. The `/budget` Command Mechanism

### Pattern

Mirrors `/health` and `/task` command infrastructure in `pkg/loop/loop.go`.
Insertion point: between health command processing (line 224) and signal
checking (line 227).

```
LLM outputs:   /budget {"agent":"implementer","action":"increase","amount":25,"reason":"high-value work in progress"}
Framework:     parseBudgetCommand() extracts JSON
Framework:     validateBudgetCommand() checks cooldowns, stabilization, floor/ceiling, pool
Framework:     BudgetRegistry.AdjustMaxIterations() modifies target agent's limit
Framework:     emitBudgetAdjusted() records agent.budget.adjusted event on chain
```

### Command Format

```
/budget {"agent":"<n>","action":"increase|decrease|set","amount":<iterations>,"reason":"<brief>"}
```

| Command Field | Purpose |
|---------------|---------|
| `agent` | Target agent name (must match a BudgetRegistry entry) |
| `action` | `increase` (add iterations), `decrease` (remove iterations), `set` (absolute) |
| `amount` | Number of iterations to add/remove/set (positive integer) |
| `reason` | Human-readable explanation (stored on chain, aids debugging) |

### Framework Validation

| Check | Rule | On Failure |
|-------|------|-----------|
| **Stabilization window** | No adjustments in first 10 iterations | Reject, log |
| **Agent cooldown** | No same-agent adjustment within 10 iterations | Reject, log |
| **Global cooldown** | Max 1 adjustment per 5 iterations | Reject, log |
| **Budget floor** | Result >= 20 iterations | Clamp to floor, log |
| **Budget ceiling** | Result <= 500 iterations | Clamp to ceiling, log |
| **Pool conservation** | Increase must have headroom | Reject if exhausted |
| **Agent exists** | Name matches registry entry | Reject, log |
| **Non-negative amount** | Amount > 0 | Reject, log |

### Framework Functions

```go
// In pkg/loop/budget.go

type BudgetCommand struct {
    Agent  string `json:"agent"`
    Action string `json:"action"`
    Amount int    `json:"amount"`
    Reason string `json:"reason"`
}

func parseBudgetCommand(response string) *BudgetCommand { ... }

func (l *Loop) validateBudgetCommand(cmd *BudgetCommand) error {
    // Uses l.budgetRegistry for agent existence and pool checks
    // Uses l.adjustmentHistory for cooldown checks
    // Uses l.iteration for stabilization window check
}

func (l *Loop) applyBudgetAdjustment(cmd *BudgetCommand) error {
    // 1. Call l.budgetRegistry.AdjustMaxIterations(cmd.Agent, delta, floor, ceiling)
    // 2. Emit agent.budget.adjusted event on chain
    // 3. Record adjustment in cooldown tracker
}
```

### Event Type: `agent.budget.adjusted`

Created in eventgraph (committed f9b4cdc). Content struct:

```go
type AgentBudgetAdjustedContent struct {
    AgentID        types.ActorID `json:"agent_id"`
    AgentName      string        `json:"agent_name"`
    Action         string        `json:"action"`
    PreviousBudget int           `json:"previous_budget"`
    NewBudget      int           `json:"new_budget"`
    Delta          int           `json:"delta"`
    Reason         string        `json:"reason"`
    PoolRemaining  int           `json:"pool_remaining"`
}
```

---

## 8. BudgetRegistry — Infrastructure Layer

Created in Prompt 0.5 (committed c448b10). Lives in `pkg/resources/`
(not `pkg/hive/`) to avoid circular imports. The import chain is one-way:
`pkg/hive` → `pkg/loop` → `pkg/resources`.

### Purpose

The BudgetRegistry solves four problems simultaneously:
1. **Cross-agent reads** — Allocator needs all agents' budget states
2. **Cross-agent writes** — Allocator needs to modify other agents' limits
3. **Limits visibility** — Budget.Snapshot() lacks limits; registry stores them
4. **State tracking** — Registry tracks agent operational state for enrichment

### Implementation

```go
// pkg/resources/budget_registry.go

type BudgetEntry struct {
    Name          string
    Budget        *Budget
    MaxIterations int       // mutable — Allocator adjusts via AdjustMaxIterations
    AgentState    string    // "Active", "Quiesced", "Stopped"
}

type BudgetRegistry struct {
    mu      sync.RWMutex
    entries map[string]*BudgetEntry
}
```

### API

```go
func NewBudgetRegistry() *BudgetRegistry
func (r *BudgetRegistry) Register(name string, budget *Budget, maxIter int)
func (r *BudgetRegistry) Snapshot() []BudgetEntry         // returns copies, read-locked
func (r *BudgetRegistry) AdjustMaxIterations(name string, delta int, floor int, ceiling int) (int, int, error)
func (r *BudgetRegistry) SetAgentState(name string, state string)
func (r *BudgetRegistry) TotalPool() int                  // sum of MaxIterations
func (r *BudgetRegistry) TotalUsed() int                  // sum of Snapshot().Iterations
```

### Wiring

1. Runtime creates BudgetRegistry in `Run()`
2. Each `spawnAgent()` creates Budget externally, registers it, passes to Loop
   via `Config.BudgetInstance`
3. Loop receives registry via `Config.BudgetRegistry`
4. When `BudgetInstance` is set in Config, Loop uses it directly instead of
   creating a new Budget (ensures registry and Loop share the same reference)

### Budget Mutation Path

`AdjustMaxIterations()` calls `Budget.SetMaxIterations()` internally. The Loop's
budget check on each iteration immediately reflects the new limit because the
registry's Budget pointer and the Loop's Budget pointer are the same object.

### Tests

12 tests in `pkg/resources/budget_registry_test.go` (committed c448b10):
register/snapshot, increase/decrease, floor clamp, ceiling clamp, unknown agent
error, underlying Budget update, agent state, total pool, total used, concurrent
access, SetMaxIterations on Budget.

---

## 9. Allocation Algorithm

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

## 10. Site Persona File

Location: `site/graph/personas/allocator.md`

**Note:** SysMon validation confirmed that persona files do NOT use YAML
frontmatter. They use plain markdown matching the format of existing personas
(e.g., `sysmon.md`, `guardian.md`). Follow the actual format found in the repo.

```markdown
# Allocator

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

## 11. Configuration

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

## 12. Hive-Local Types: `pkg/budget/`

Pure functions that pre-digest budget data for the LLM. No side effects.
Data comes from the BudgetRegistry (cross-agent visibility) and bus events.

### Types (Prompt 1 — COMPLETE, committed b38f664)

```go
package budget

// AgentBudgetState represents one agent's current budget status.
// MaxIterations and State come from BudgetRegistry.Snapshot().
// UsedIterations comes from Budget.Snapshot().Iterations.
type AgentBudgetState struct {
    Name           string  `json:"name"`
    MaxIterations  int     `json:"max_iterations"`
    UsedIterations int     `json:"used_iterations"`
    State          string  `json:"state"`
    BurnRate       float64 `json:"burn_rate"`
    IdlePercent    float64 `json:"idle_percent"`
}

// PoolState represents the total budget pool.
type PoolState struct {
    TotalIterations     int     `json:"total_iterations"`
    UsedIterations      int     `json:"used_iterations"`
    RemainingIterations int     `json:"remaining_iterations"`
    DailyCost           float64 `json:"daily_cost"`
    DailyCap            float64 `json:"daily_cap"`
    BurnRatePerHour     float64 `json:"burn_rate_per_hour"`
    ProjectedDailyPct   float64 `json:"projected_daily_pct"`
}

// AdjustmentRecord tracks a previous budget adjustment for cooldown tracking.
type AdjustmentRecord struct {
    Agent     string `json:"agent"`
    Iteration int    `json:"iteration"`
    Delta     int    `json:"delta"`
    Reason    string `json:"reason"`
}

// SysMonSummary is a digest of the latest health.report event.
type SysMonSummary struct {
    Severity     string   `json:"severity"`
    ChainOK      bool     `json:"chain_ok"`
    ActiveAgents int      `json:"active_agents"`
    EventRate    float64  `json:"event_rate"`
    Anomalies    []string `json:"anomalies,omitempty"`
}

// BudgetReport is the pre-computed summary given to the LLM.
type BudgetReport struct {
    Pool          PoolState          `json:"pool"`
    Agents        []AgentBudgetState `json:"agents"`
    SysMonSummary *SysMonSummary     `json:"sysmon_summary,omitempty"`
    History       []AdjustmentRecord `json:"history"`
    Cooldowns     map[string]int     `json:"cooldowns"`
}
```

### Monitoring Functions (Prompt 2)

```go
// DefaultConfig returns sensible defaults for all thresholds.
func DefaultConfig() Config

// LoadConfig reads ALLOCATOR_* env vars, falling back to defaults.
func LoadConfig() Config

// BuildReport assembles a BudgetReport from runtime data.
func BuildReport(
    agents []AgentBudgetState,
    pool PoolState,
    sysmon *SysMonSummary,
    history []AdjustmentRecord,
    config Config,
    currentIteration int,
) BudgetReport

// CheckConcentration flags agents consuming > threshold % of the pool.
func CheckConcentration(agents []AgentBudgetState, pool PoolState, config Config) []string

// CheckExhaustion flags agents approaching their iteration limit.
func CheckExhaustion(agents []AgentBudgetState, config Config) []string

// CheckIdleAgents flags agents with sustained low utilization.
// Agents with State=="Quiesced" are EXCLUDED — they are waiting, not stuck.
func CheckIdleAgents(agents []AgentBudgetState, config Config) []string

// CheckDailyBurnRate flags if projected daily spend exceeds warning threshold.
func CheckDailyBurnRate(pool PoolState, config Config) *string

// CooldownRemaining returns iterations until an agent can be adjusted again.
func CooldownRemaining(agent string, history []AdjustmentRecord, currentIter int, config Config) int

// GlobalCooldownRemaining returns iterations until any adjustment is allowed.
func GlobalCooldownRemaining(history []AdjustmentRecord, currentIter int, config Config) int

// InStabilizationWindow returns true if still in the observe-only boot phase.
func InStabilizationWindow(currentIter int, config Config) bool
```

---

## 13. Observation Enrichment

Before each LLM call, the framework enriches Allocator's observation with
pre-computed budget metrics. Data source is BudgetRegistry (NOT the self-referential
SysMon path).

```go
// In pkg/loop/budget.go

func (l *Loop) enrichBudgetObservation(obs string) string {
    if l.agentDef.Role != "allocator" {
        return obs
    }
    // Data source: BudgetRegistry
    entries := l.budgetRegistry.Snapshot()
    // Build AgentBudgetState slice from entries
    // Build PoolState from registry totals + daily budget data
    // Extract SysMonSummary from recent health.report events in pendingEvents
    // Format as === BUDGET METRICS === block (see section 6 for format)
    return obs + formatBudgetMetrics(report)
}
```

---

## 14. Integration Points

### SysMon Integration

Allocator consumes `health.report` events as a primary input signal. The
`SysMonSummary` struct in `pkg/budget/` extracts the fields Allocator cares about
(severity, active agents, anomalies) from the `HealthReportContent` event.

SysMon does not need to know Allocator exists. One-way dependency. This is
identical to how SysMon relates to Guardian — emit reports, let consumers decide.

### Guardian Integration

Guardian watches `*` and automatically sees `agent.budget.adjusted` events. No
WatchPattern change needed for Guardian to observe Allocator's output.

Guardian's prompt should eventually include **Allocator Awareness** — noting that
absence of `agent.budget.adjusted` events is NOT necessarily concerning (the
Allocator may correctly determine no adjustment is needed), but absence of ANY
Allocator activity (no state changes, no /signal IDLE, no /budget commands) for
25+ iterations mirrors the SysMon absence concern.

### Budget System Integration

The Allocator's `/budget` commands feed back through the BudgetRegistry:

1. `parseBudgetCommand()` extracts the command from LLM output
2. `validateBudgetCommand()` checks constraints using registry data
3. `BudgetRegistry.AdjustMaxIterations()` modifies the target agent's limit
   (calls `Budget.SetMaxIterations()` internally — same object reference)
4. `agent.budget.adjusted` event emitted on chain with full audit trail

### Spawner Integration (Phase 3, Future)

When the Spawner proposes a new agent (via `hive.role.proposed`), the Allocator is
part of the approval chain: Spawner proposes → Guardian reviews → **Allocator
budgets** → runtime registers. The Allocator must respond to spawn proposals with
a budget allocation from the pool reserve (default: 50 iterations).

For Phase 1, the Allocator only manages existing agents from `StarterAgents()`.

### Site Bridge

Forward budget adjustment events via existing `POST /api/hive/diagnostic` endpoint,
same as SysMon health reports.

---

## 15. Testing Strategy

### Unit Tests — pkg/budget/ (Prompts 1-2)

- Types: JSON round-trip for all 5 structs (including nil SysMonSummary)
- Config: DefaultConfig matches spec values, all defaults non-zero
- Config: LoadConfig reads ALLOCATOR_* env vars correctly
- Config: LoadConfig falls back to defaults when vars missing
- Config: Invalid env var values ignored (use defaults)
- Concentration: agent at 50% flags (threshold 40%), agent at 30% clear
- Exhaustion: agent at 85% flags (threshold 80%), agent at 50% clear
- Idle: active agent at 5% flags (threshold 10%), agent at 15% clear
- Idle: quiesced agent at 0% does NOT flag (waiting for work, not stuck)
- Daily burn rate: projected 95% flags (threshold 90%), projected 60% clear
- Cooldown remaining: adjusted 3 iterations ago (cooldown 10) → 7 remaining
- Cooldown remaining: adjusted 15 iterations ago (cooldown 10) → 0 (clear)
- Cooldown remaining: no history for agent → 0 (clear)
- Global cooldown: any adjustment 2 iterations ago → remaining
- Global cooldown: last adjustment 10 iterations ago → 0 (clear)
- Stabilization window: iteration 5 (window 10) → true (inside)
- Stabilization window: iteration 10 (window 10) → false (outside)
- Stabilization window: iteration 9 (window 10) → true (boundary)
- BuildReport: all fields populated correctly

Target: >= 80% coverage on `pkg/budget/`

### BudgetRegistry Tests — pkg/resources/ (Prompt 0.5 — COMPLETE)

12 tests covering: register/snapshot, increase/decrease, floor clamp, ceiling
clamp, unknown agent error, underlying Budget update, agent state, total pool,
total used, concurrent access, SetMaxIterations on Budget.

### Framework Glue Tests — pkg/loop/ (Prompt 3.5)

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

- Allocator boots and runs in legacy mode (index 2, Haiku model)
- Allocator's LLM receives enriched budget observations (=== BUDGET METRICS ===)
- `/budget` command in LLM output produces `agent.budget.adjusted` event on chain
- Cooldown enforcement prevents rapid-fire adjustments
- Stabilization window prevents early adjustments
- Budget floor prevents starvation (no agent below 20 iterations)
- Guardian receives and can observe `agent.budget.adjusted` events

---

## 16. Implementation Checklist

| Item | Prompt | Status |
|------|--------|--------|
| Reconnaissance | 0 | ✅ COMPLETE |
| BudgetRegistry in pkg/resources | 0.5 | ✅ Committed c448b10 |
| Budget.SetMaxIterations() | 0.5 | ✅ Committed c448b10 |
| agent.budget.adjusted event type | 0.5 | ✅ Committed f9b4cdc |
| BudgetRegistry tests (12 tests) | 0.5 | ✅ Committed c448b10 |
| `pkg/budget/types.go` | 1 | ✅ Committed b38f664 |
| `pkg/budget/config.go` | 1 | ✅ Committed b38f664 |
| `pkg/budget/types_test.go` (11 tests) | 1 | ✅ Committed b38f664 |
| `pkg/budget/monitor.go` | 2 | Pending |
| `pkg/budget/monitor_test.go` | 2 | Pending |
| `agents/allocator.md` | 3 | Pending |
| Site persona `allocator.md` | 3 | Pending |
| `/budget` command parser | 3.5 | Pending |
| `validateBudgetCommand()` | 3.5 | Pending |
| `applyBudgetAdjustment()` | 3.5 | Pending |
| Observation enrichment | 3.5 | Pending |
| Framework glue tests | 3.5 | Pending |
| Allocator in `StarterAgents()` + reorder | 4 | Pending |
| Guardian prompt update | 5 | Pending |
| Integration tests | 6 | Pending |

---

## 17. Exit Criteria

Phase 1 Allocator graduation requires ALL of the following:

- [x] BudgetRegistry created and wired into Runtime
- [x] Budget.SetMaxIterations() method exists
- [x] agent.budget.adjusted event type registered in eventgraph
- [x] BudgetRegistry tests pass (12 tests)
- [ ] Allocator boots as part of `StarterAgents()` in legacy mode
- [ ] Boot order: guardian → sysmon → allocator → strategist → planner → implementer
- [ ] Allocator receives enriched budget observations each iteration
- [ ] Allocator's `/budget` command produces `agent.budget.adjusted` events on the chain
- [ ] Stabilization window prevents adjustments in first 10 iterations
- [ ] Cooldown enforcement prevents rapid-fire adjustments
- [ ] Budget floor prevents agent starvation (no agent below 20 iterations)
- [ ] Budget adjustments actually modify target agent's iteration limit
- [ ] Pool conservation: total budget is preserved (increase one = decrease pool)
- [ ] SysMon health reports are consumed and influence allocation decisions
- [ ] Guardian observes `agent.budget.adjusted` events (automatic via `*` pattern)
- [ ] Unit test coverage >= 80% on `pkg/budget/`
- [ ] Framework glue tests pass for `/budget` parsing, validation, and application
- [ ] Linter passes, all tests pass
- [ ] Site persona exists and is active

---

## 18. Post-Implementation Verification

After all PRs are merged, run this final check:

```
Run the full hive in legacy mode with --human Michael --idea "test allocator budget management"

Verify:
1. Allocator boots as the third agent (after Guardian, SysMon)
2. Allocator's LLM observations include === BUDGET METRICS === block
3. Allocator does NOT emit /budget commands during first 10 iterations (stabilization)
4. After stabilization, Allocator emits /budget commands that produce agent.budget.adjusted events
5. Cooldown enforcement visible: no same-agent adjustment within 10 iterations
6. Budget floor enforced: no agent reduced below 20 iterations
7. SysMon health reports visible in Allocator's decision context
8. Guardian receives and can observe agent.budget.adjusted events

Report back on what you see. If everything checks out, Allocator is graduated
and we move to CTO.
```

---

## 19. What Comes After Allocator

```
Guardian (done) → SysMon (done) → Allocator (this doc) → CTO → Spawner → Growth Loop
                                  ^^^^^^^^^^^^^^^^^^^^
                                  YOU ARE HERE
```

---

*v1.2.0 is fully self-contained — no references to previous versions required.
All content validated against codebase via Prompt 0 reconnaissance. BudgetRegistry
infrastructure (Prompt 0.5) and types/config (Prompt 1) are complete. Prompt 2
is the next implementation step.*
