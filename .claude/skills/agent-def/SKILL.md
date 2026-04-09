---
name: agent-def
description: "Scaffold a new AgentDef for the hive. Use when the user says 'add an agent', 'new agent', 'create agent', 'define agent', 'scaffold agent', or wants to add a new role to the hive runtime. Also trigger when discussing agent design, role taxonomy, or the growth loop."
---

# Scaffold a New AgentDef

Add a new agent to the hive by generating the AgentDef struct, system prompt, and telemetry wiring.

## What You Need From the User

| Field | Required | Example |
|-------|----------|---------|
| **Name** | Yes | `"tester"` |
| **Role** | Yes (often same as name) | `"tester"` |
| **Purpose** | Yes | "Run tests, validate changes" |
| **Model** | No (default: Sonnet) | `ModelOpus`, `ModelSonnet`, `ModelHaiku` |
| **CanOperate** | No (default: false) | true = needs filesystem access |
| **WatchPatterns** | No (default: all) | `[]string{"work.task.completed"}` |
| **MaxIterations** | No (default: 50) | 100, 300, 500 |

If the user hasn't specified all fields, ask. Don't guess the purpose — it shapes the system prompt.

## Files to Modify (3 files, always)

### 1. `pkg/hive/agentdef.go` — Add the AgentDef

Add a new entry to the `StarterAgents()` return slice. Follow the exact pattern:

```go
{
    Name:  "<name>",
    Role:  "<role>",
    Model: Model<Opus|Sonnet|Haiku>,
    SystemPrompt: mission(`== ROLE: <UPPERCASE_ROLE> ==
You are the <Name> — <one-line purpose>.

<2-3 paragraphs describing what the agent does, what it watches for,
and what commands it emits. Be specific about the agent's boundaries.>

CONSTRAINTS:
- <Stabilization window if needed>
- <Rate limits on commands>
- <What this agent must NEVER do>

You NEVER <list forbidden actions>.

== INSTITUTIONAL KNOWLEDGE ==
Your observation may include an === INSTITUTIONAL KNOWLEDGE === block with
evidence-based insights distilled from accumulated experience. Use them as
context — they are observations, not commands. You may disagree if you
observe contradicting evidence.
`),
    WatchPatterns: []string{<patterns>},
    CanOperate:    <true|false>,
    MaxIterations: <N>,
},
```

### System Prompt Rules

Every system prompt MUST include:

1. **Role header**: `== ROLE: <NAME> ==` with one-line purpose
2. **Behavioral description**: What the agent does each iteration
3. **Command format**: The exact `/command` syntax it emits (if any)
4. **Constraints block**: Stabilization window, rate limits, forbidden actions
5. **Institutional Knowledge block**: Always include the standard IK footer (copy from existing agents)
6. **Soul inheritance**: The `mission()` wrapper injects the soul statement, coordination protocol, and trust framework — don't repeat them

### Model Selection Guide

| Model | Use For | Cost |
|-------|---------|------|
| `ModelOpus` | Judgment, strategy, code writing | Highest |
| `ModelSonnet` | Classification, monitoring, review | Medium |
| `ModelHaiku` | Health checks, simple routing, budget | Lowest |

### Boot Order

The order in `StarterAgents()` matters. Insert the new agent at the appropriate position:

- **Infrastructure agents first**: Guardian, SysMon, Allocator
- **Leadership next**: CTO, Spawner, Reviewer
- **Work agents last**: Strategist, Planner, Implementer

New agents typically go before the work agents unless they're infrastructure.

### 2. `pkg/telemetry/schema.go` — Add seed data (if non-running role)

If the agent is a future/designed role (not being added to StarterAgents yet), add it to `seedRoleDefinitions`:

```sql
('rolename', 'DisplayName', '<tier>', '<purpose>',
 '<status>', <has_prompt>, <has_persona>, '<category>', '{<deps>}', <phase>),
```

Tiers: A (bootstrap), B (organic), C (business), D (governance)
Status: designed, defined, running, missing
Phase: NULL for bootstrap, 4-8 for later phases

### 3. `pkg/telemetry/schema.go` — Add to phase-agent membership

Add the role to `seedPhaseAgents` under the appropriate phase:

```sql
(<phase>, '<role>'),
```

## Telemetry Wiring (Automatic)

No manual wiring needed. When the agent is in `StarterAgents()`, the runtime calls `RegisterAgent()` in both:
- `pkg/hive/runtime.go` (bootstrap spawn)
- `pkg/hive/watch.go` (dynamic spawn via Spawner)

Both paths pass `WatchPatterns` and `CanOperate` through to the telemetry writer, which upserts the role definition with status `'running'`.

## Checklist

Before committing a new agent:

- [ ] AgentDef added to `StarterAgents()` in correct boot position
- [ ] System prompt includes role header, constraints, IK block
- [ ] Model choice matches the agent's cognitive demands
- [ ] WatchPatterns are specific (no bare `"*"` unless it's the Guardian)
- [ ] CanOperate is false unless the agent genuinely needs filesystem access
- [ ] MaxIterations is bounded (not unlimited)
- [ ] `go build ./...` passes
- [ ] `go test ./pkg/hive/... ./pkg/telemetry/...` passes
- [ ] Phase-agent membership updated in seed data
- [ ] No magic strings — use constants for event types, models, etc.

## Example: Adding a Tester Agent

```go
{
    Name:  "tester",
    Role:  "tester",
    Model: ModelSonnet,
    SystemPrompt: mission(`== ROLE: TESTER ==
You are the Tester — the civilization's quality assurance gate.

When code is committed or a task is marked complete, you run the test suite
and report results. You do NOT write code — you validate it.

Each iteration, check for tasks marked complete that lack test verification.
Run the relevant test commands and emit a structured verdict:

/test {"task_id":"...","passed":true|false,"summary":"...","failures":["..."]}

CONSTRAINTS:
- First 5 iterations: observe only
- One test run per iteration
- Do NOT modify source code
- Do NOT re-test already-verified tasks unless new commits exist

You NEVER write code, modify budgets, or halt agents.

== INSTITUTIONAL KNOWLEDGE ==
Your observation may include an === INSTITUTIONAL KNOWLEDGE === block with
evidence-based insights distilled from accumulated experience. Use them as
context — they are observations, not commands. You may disagree if you
observe contradicting evidence.
`),
    WatchPatterns: []string{"work.task.completed", "code.review.approved"},
    CanOperate:    true, // needs to run tests
    MaxIterations: 200,
},
```
