# Reboot Survival Design

**Version:** 1.0.0
**Date:** 2026-04-09
**Branch:** feat/reviewer-design
**Status:** Approved

---

## Problem

The hive loses all agent runtime state on reboot. Iteration counters reset to zero, cooldown timers clear, review cycles restart, and stabilization windows re-engage. A hive that was running smoothly loses 15+ iterations per agent to stabilization alone. Dynamic agents wait for event triple re-discovery. No agent remembers what it was doing or why.

The event chain persists *what happened* but not *what agents were thinking*. Replaying thousands of raw events to reconstruct intent is wasteful and architecturally wrong — the event chain is a transaction log, not a reasoning log.

## Decision

Two-tier recovery: Open Brain for intent, event chain for mechanical state.

- **Open Brain** captures agent intent and context at meaningful boundaries (task start/complete, blocker, strategy change). On reboot, agents read their last thought to warm-start with context.
- **Event chain** provides mechanical state fallback (budget adjustments, cooldowns, rejections, review cycles). Used only when Open Brain has no recent thought for an agent.
- **Heartbeat events** on the chain fill the gap between boundary thoughts — lightweight mechanical snapshots, never written to Open Brain.

## Architecture

### Two Tiers

**Tier 1 — Chain Replay (cold start / fallback)**

Extend the existing `knowledge.ReplayFromStore()` pattern to replay four additional event families on startup:

| Event Family | Events to Replay | State Reconstructed |
|---|---|---|
| Budget | `agent.budget.adjusted` | Per-agent budget allocations, adjustment history for cooldown enforcement |
| CTO | `hive.gap.detected`, `hive.gap.emitted`, `hive.directive.emitted` | Gap/directive emission history, cooldown maps |
| Spawner | `hive.role.proposed`, `hive.role.approved`, `hive.role.rejected` | Rejection history, processed gaps, pending proposals |
| Reviewer | `work.task.completed`, code review events | Review round counts per task, completed task records |

Each gets a `ReplayXFromStore(store, state)` function following the knowledge replay pattern: fetch by type, sort chronologically, replay into in-memory struct.

Tier 1 runs **only when Tier 2 fails** (no Open Brain thought found, thought is stale, or Open Brain is unreachable).

**Tier 2 — Open Brain Recovery (normal startup)**

On startup, each agent queries Open Brain for its most recent checkpoint thought. If found and recent (within the configurable staleness threshold, default 2 hours), the thought contains enough structured state to skip chain replay:

- Approximate iteration count
- Budget consumed / remaining
- Current task and progress
- Intent and next steps
- Last signal state

The runtime also queries for the last hive summary thought to give all agents situational awareness.

### Recovery Sequence

```
Boot
 |-- Initialize stores (event store, actor store, knowledge store)
 |-- Replay knowledge from chain (existing -- no change)
 |-- For each registered agent (starters + discovered dynamic agents):
 |    |-- Query Open Brain: search "checkpoint {role}" limit 1
 |    |-- IF thought found AND recent (within staleness threshold):
 |    |     |-- Parse structured fields (STATUS, BUDGET, TASK, INTENT, CONTEXT)
 |    |     |-- Seed loop state:
 |    |     |     iteration = extracted iteration estimate
 |    |     |     intent = INTENT field (injected into first-iteration prompt)
 |    |     |     currentTask = TASK field (skip auto-assign, resume this task)
 |    |     |-- Query chain for heartbeat events AFTER the thought's timestamp
 |    |     |-- If newer heartbeat found, update iteration/budget from heartbeat
 |    |     |-- Mark agent as "warm-started"
 |    |-- IF no thought found OR thought stale (exceeds staleness threshold):
 |         |-- Fall back to Tier 1 chain replay for this agent
 |         |-- ReplayBudgetFromStore(), ReplayCTOFromStore(), etc.
 |         |-- Mark agent as "cold-started"
 |-- Query Open Brain: search "hive summary" limit 1
 |    |-- If found and recent: inject into all agents' first-iteration context
 |-- Start agents (warm-started agents skip stabilization window)
 |-- Launch watchForApprovedRoles() for dynamic agents
```

Each agent recovers independently. If Open Brain has a thought for the Implementer but not the CTO, the Implementer warm-starts and the CTO cold-starts from chain.

### Stabilization Windows

- **Warm-started agents skip stabilization.** They have context — the stabilization window exists to prevent uninformed agents from thrashing.
- **Cold-started agents still enter stabilization.** No context means the caution is warranted.

A hive that was running smoothly, reboots, and comes back warm loses zero iterations to stabilization.

## Thought Structure

Thoughts are human-readable natural language with consistent semantic anchors for machine parsing. Not JSON — Open Brain uses embedding-based search, and natural language has better semantic signal.

### Agent Checkpoint Thought

```
[CHECKPOINT] {role} agent -- iteration ~{N}, {timestamp}

STATUS: {signal state -- ACTIVE/IDLE/ESCALATE}
BUDGET: {consumed}/{max} iterations, {tokens} tokens, ${cost}
TASK: {task ID} -- {title} -- {status: assigned/in-progress/reviewing/blocked}
INTENT: {what I'm doing and why, 1-2 sentences}
NEXT: {what I plan to do next iteration, 1 sentence}
CONTEXT: {any state that matters for resumption}
```

**Example — Implementer:**
```
[CHECKPOINT] implementer agent -- iteration ~34, 2026-04-09T14:22:00Z

STATUS: ACTIVE
BUDGET: 34/50 iterations, 142k tokens, $0.83
TASK: task-77 -- Add error handling to REST API -- in-progress
INTENT: Implementing retry logic for database connections. Tests passing for happy path, working on timeout scenarios.
NEXT: Write timeout test cases, then mark task complete.
CONTEXT: Operating on /home/transpara/repos/lovyou-ai-work. Made 2 commits so far on feat/api-errors branch.
```

**Example — Reviewer:**
```
[CHECKPOINT] reviewer agent -- iteration ~19, 2026-04-09T14:18:00Z

STATUS: ACTIVE
BUDGET: 19/50 iterations
TASK: task-77 -- reviewing -- round 2 of 3
INTENT: Second review pass on implementer's error handling PR. Round 1 found missing test coverage, implementer addressed it. Checking fix quality.
NEXT: If round 2 passes, approve. If not, escalate (round 3 is the max).
CONTEXT: Review history -- round 1 at iter 12 (REVISE, 2 issues), round 2 at iter 19 (in progress).
```

**Example — CTO:**
```
[CHECKPOINT] cto agent -- iteration ~28, 2026-04-09T14:20:00Z

STATUS: ACTIVE
BUDGET: 28/50 iterations
TASK: none -- monitoring hive health
INTENT: Scanning for architectural gaps. Emitted gap for "testing" role at iteration 22, directive to implementer about error handling patterns at iteration 25.
NEXT: Waiting for spawner response to testing gap. Will re-evaluate at iteration 35 if no action.
CONTEXT: Cooldowns -- gap:testing expires iter 37, directive:implementer expires iter 30. No stabilization restrictions.
```

### Hive Summary Thought

Captured by the runtime on meaningful state changes (agent spawned/stopped, task completed, budget threshold crossed).

```
[HIVE SUMMARY] -- {agent_count} agents active, {dynamic_count} dynamic, {timestamp}

AGENTS: {role(state)},...
TASKS: {open_count} open ({details}), {completed_count} completed
BUDGET: ${total_spend} total spend, ${remaining} remaining daily cap
DYNAMIC: {dynamic agent details if any}
HEALTH: {HALTs, escalations, chain health}
```

## Capture Triggers

### Boundary Triggers — Write to Open Brain

Agents capture a checkpoint thought when a meaningful event occurs:

| Trigger | Why | Which Agents |
|---|---|---|
| Task assigned to self | Captures intent at start of work | All with tasks |
| Task completed | Records outcome + what's next | All with tasks |
| Task blocked / ESCALATE signal | Captures the blocker for resumption | All |
| Strategy change | "I was doing X, now switching to Y because Z" | Strategist, CTO, Planner |
| Review round completed | Records verdict, issues found, round number | Reviewer |
| Role proposed | Records which gap triggered it and why | Spawner |
| Role approved/rejected | Records Guardian's decision and reasoning | Guardian |
| Gap or directive emitted | Records what was emitted and cooldown state | CTO |
| Budget adjustment | Records old/new budget and reason | Allocator |
| HALT signal | Critical -- captures what went wrong | Guardian |

**Hive summary triggers** (captured by runtime, not agents):
- Agent spawned or stopped
- Task completed
- Budget threshold crossed (25%, 50%, 75%, 90%)
- HALT or ESCALATE signal from any agent

### Heartbeat — Write to Event Chain

A new `hive.agent.heartbeat` event type, emitted every 10 iterations if no boundary thought was captured in the last 10 iterations.

Content: iteration count, budget snapshot, signal state, current task ID. Structured event data, not natural language. Exists solely to fill the gap between boundary thoughts for mechanical state recovery.

On recovery, heartbeat events more recent than the last Open Brain thought update the iteration/budget estimate.

## Coupling Boundaries

Four independent components connected by interfaces and data, not imports.

### Interfaces

**CheckpointSink** — the loop's only connection to checkpointing:

```go
type CheckpointSink interface {
    OnBoundary(trigger BoundaryTrigger, state LoopSnapshot)
    OnHeartbeat(state LoopSnapshot)
}
```

The loop calls `sink.OnBoundary()` when a trigger fires and `sink.OnHeartbeat()` every N iterations. A nil sink is valid — checkpointing is optional.

**LoopSnapshot** — flat struct of exported values, no loop internals:

```go
type LoopSnapshot struct {
    Role          string
    Iteration     int
    MaxIterations int
    TokensUsed    int
    CostUSD       float64
    Signal        string  // ACTIVE, IDLE, ESCALATE, HALT
    CurrentTaskID string
    CurrentTask   string  // title
    TaskStatus    string  // assigned, in-progress, reviewing, blocked
}
```

**ThoughtStore** — recovery's only connection to Open Brain:

```go
type ThoughtStore interface {
    SearchRecent(query string, maxAge time.Duration) ([]Thought, error)
}

type Thought struct {
    Content    string
    CapturedAt time.Time
}
```

Real implementation wraps Open Brain's HTTP API. Tests use a stub.

### Who Knows What

| Component | Knows About | Does Not Know About |
|---|---|---|
| Loop | CheckpointSink interface, LoopSnapshot struct | Open Brain, event chain details, thought format |
| Checkpoint package | LoopSnapshot, ThoughtStore interface, event store interface | Loop internals, Open Brain HTTP details |
| Open Brain adapter | Open Brain HTTP API, Thought struct | Loop, checkpoint logic, event chain |
| Runtime | All interfaces (wires them together) | Internal implementations |

### Intent Injection

When an agent warm-starts, recovery produces a plain string prepended to the agent's first-iteration context:

```
You are resuming after a restart. Your last checkpoint:
[CHECKPOINT] implementer agent -- iteration ~34, 2026-04-09T14:22:00Z
STATUS: ACTIVE
TASK: task-77 -- Add error handling to REST API -- in-progress
INTENT: Implementing retry logic for database connections...
NEXT: Write timeout test cases, then mark task complete.

Hive context: 9 agents active, task-77 in review, $2.41 spent.

Resume from where you left off. Do not restart completed work.
```

The loop doesn't parse this string — it passes it through. The LLM interprets it.

## Implementation Scope

### New Code

| Component | Location | Purpose |
|---|---|---|
| `pkg/checkpoint/thought.go` | New package | Build checkpoint thought strings from LoopSnapshot. Parse them back on recovery. Field extraction by prefix. |
| `pkg/checkpoint/recover.go` | New package | Recovery orchestrator — queries ThoughtStore, falls back to chain replay, merges heartbeat state, returns RecoveryState per agent |
| `pkg/checkpoint/heartbeat.go` | New package | `hive.agent.heartbeat` event type, emit/query helpers |
| `pkg/checkpoint/replay.go` | New package | Tier 1 chain replay: `ReplayBudgetFromStore()`, `ReplayCTOFromStore()`, `ReplaySpawnerFromStore()`, `ReplayReviewerFromStore()` |
| `pkg/checkpoint/sink.go` | New package | CheckpointSink implementation — routes boundaries to Open Brain (agent-mediated), heartbeats to event chain |
| `pkg/checkpoint/openbrain.go` | New package | ThoughtStore adapter — HTTP client for Open Brain API |

### Modified Code

| File | Change |
|---|---|
| `pkg/loop/loop.go` | Accept RecoveryState in config. Seed iteration, skip stabilization if warm. Framework-guaranteed sink.OnBoundary() calls after completeTask(), assignTask(), and signal transitions. sink.OnHeartbeat() every N iterations. |
| `pkg/loop/cto.go` | Export CTOCooldowns initialization from recovered state |
| `pkg/loop/spawner.go` | Export spawnerState initialization from recovered state |
| `pkg/loop/review.go` | Export reviewerState initialization from recovered state |
| `pkg/hive/runtime.go` | Call checkpoint.RecoverAll() after knowledge replay, before agent start. Pass RecoveryState + CheckpointSink into each loop config. Wire hive summary triggers. |
| `pkg/hive/events.go` | Register `hive.agent.heartbeat` event type in allHiveEventTypes() |

### Not Changed

- Event store interface — no schema changes, just new event types
- Telemetry — continues as observability, not recovery
- Knowledge replay — untouched
- AgentDef struct — no changes
- Open Brain API — used as-is

### Capture Path

Boundary capture uses a two-layer guarantee model:

- **Framework-guaranteed boundaries:** The loop itself calls `sink.OnBoundary()` after key operations complete — `completeTask()`, `assignTask()`, signal transitions (ESCALATE, HALT). These fire regardless of whether the agent remembered to checkpoint. The framework builds a thought from the `LoopSnapshot` — mechanical state plus whatever context is available from the operation that just completed. This is the safety net: boundaries are never missed.
- **Agent-mediated enrichment:** The agent's prompt includes instructions to capture richer checkpoint thoughts to Open Brain during its LLM turn. These thoughts contain the agent's reasoning, strategy, and contextual understanding that the framework can't observe. When both fire for the same boundary, the agent-mediated thought is richer and takes precedence on recovery. When the agent skips the checkpoint (because it prioritized the task), the framework-guaranteed thought covers the gap.
- **Heartbeat events:** Direct. The loop emits `hive.agent.heartbeat` events to the event chain via the existing store.Append() path. Mechanical state only, never written to Open Brain.
- **Recovery queries:** HTTP client. The runtime queries Open Brain at startup before agents are running — agents can't mediate their own recovery.

**Precedence on recovery:** Agent-mediated thought (richest) > framework-guaranteed thought (reliable) > heartbeat (mechanical). The recovery orchestrator picks the most recent thought for a given boundary, preferring agent-mediated when timestamps overlap.

## Failure Modes

| Failure | Behavior |
|---|---|
| Open Brain unreachable at startup | All agents cold-start from chain replay (Tier 1). Warning logged. |
| Open Brain returns stale thought (exceeds staleness threshold) | Treated as cold-start. Stale intent is worse than no intent. |
| First boot (no prior state anywhere) | All agents cold-start. No Open Brain thoughts, no heartbeats, no chain history. Degrades cleanly to today's behavior — identical to current boot sequence. |
| Event store unreachable | Fatal — hive cannot start without its event store. Same as today. |
| Thought parse error (malformed) | That agent cold-starts. Others unaffected. Warning logged. |
| Heartbeat event missing | Use thought's state as-is. Iteration estimate may be off by up to 10. Acceptable. |
| Open Brain unreachable during run | Boundary thoughts silently dropped. Agent continues. Heartbeats still land on chain. Next reboot will cold-start this agent. |
| Agent never hits a boundary trigger | Heartbeat on chain every 10 iterations provides minimum recovery data. Cold-start path uses chain replay. |

## Reboot Survival Classification

Three-state model for dashboard visibility:

| Level | Label | Meaning |
|---|---|---|
| **full** | Role + state + intent survives | Agent warm-starts from Open Brain thought. Skips stabilization. Resumes task with context. |
| **role-only** | Re-spawns fresh from chain | Agent cold-starts. Mechanical state replayed from chain. Enters stabilization. No intent. |
| **none** | Lost on restart | Agent does not re-spawn (incomplete event triple for dynamic agents). |

**Current state (before this work):** Every agent is "role-only" at best.

**After this work:** Every agent with a recent Open Brain checkpoint thought is "full". Agents without a thought degrade to "role-only". Dynamic agents with incomplete triples remain "none".

Add `reboot_survival TEXT` column to `telemetry_role_definitions` with values `'full'`, `'role-only'`, `'none'`. Updated by the recovery orchestrator after each boot based on actual recovery path taken.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `CHECKPOINT_STALENESS` | `2h` | Maximum age of an Open Brain thought before it's treated as stale. Hives that restart frequently should use shorter values (e.g., `30m`). Long-running hives can increase (e.g., `6h`). |
| `CHECKPOINT_HEARTBEAT_INTERVAL` | `10` | Iterations between heartbeat events on the chain when no boundary trigger has fired. |

Both are read from environment variables via `loop/config.env`, consistent with existing loop configuration.

## Cost

- **Open Brain capture:** One API call per boundary event. Typical agent: 5-15 thoughts per run. Negligible vs LLM inference.
- **Open Brain recovery:** One search query per agent + one for hive summary on startup. ~10 queries total.
- **Heartbeat events:** One event per agent every 10 iterations. Lightweight append to existing chain.
- **Chain replay (Tier 1):** Same cost as existing knowledge replay. Only runs when Open Brain misses.
- **Stabilization savings:** Warm-started agents save 15 iterations each. For 9 agents, that's 135 iterations of productive work recovered per reboot.
