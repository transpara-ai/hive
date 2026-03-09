# Roles

The complete role architecture for the hive — who does what, who watches whom, and how the workforce grows.

## Derivation

The hive's roles are derived from its operational needs across four dimensions:

| Dimension | Range |
|-----------|-------|
| **Domain** | System ↔ Agents ↔ Resources ↔ Products |
| **Response** | Observe ↔ Alert ↔ Fix ↔ Create |
| **Time** | Real-time ↔ Periodic ↔ Strategic |
| **Intelligence** | Opus (judgment) ↔ Sonnet (execution) ↔ Haiku (volume) |

## Role Categories

### Tier A: Bootstrap Roles (Day One)

These roles exist from the first run. They are the minimum viable civilisation.

#### Leadership & Oversight

| Role | Responsibility | Model | Trust Gate | Reports To |
|------|---------------|-------|-----------|-----------|
| **CTO** | Architectural oversight, delegation, escalation filtering | Opus | 0.1 | Human |
| **Guardian** | Independent integrity monitor, HALT authority, policy enforcement | Opus | 0.1 | Human (directly) |

#### Product Pipeline

| Role | Responsibility | Model | Trust Gate | Reports To |
|------|---------------|-------|-----------|-----------|
| **Researcher** | Read URLs, extract product ideas, structured output | Sonnet | 0.3 | CTO |
| **Architect** | Design systems via derivation method, Code Graph specs | Opus | 0.3 | CTO |
| **Builder** | Generate code + tests from specs | Sonnet | 0.3 | CTO |
| **Reviewer** | Code quality, security, spec compliance, derivation check | Opus | 0.5 | CTO |
| **Tester** | Run tests, validate behaviour, coverage analysis | Sonnet | 0.3 | CTO |
| **Integrator** | Assemble, deploy, health check | Sonnet | 0.7 | CTO |

#### Operations (NEW — bootstrap these alongside pipeline roles)

| Role | Responsibility | Model | Trust Gate | Reports To |
|------|---------------|-------|-----------|-----------|
| **SysMon** | System health monitoring, error detection, performance tracking, log analysis | Haiku | 0.1 | Guardian |
| **Spawner** | Identify workforce gaps, propose new agents, manage agent lifecycle | Sonnet | 0.5 | CTO |
| **Allocator** | Resource allocation (tokens, compute, budget), model selection per task | Haiku | 0.3 | CTO |

**Why these three:**
- **SysMon** was the first operational role that emerged in lovyou (as "Monitor" + "Debug"). Without it, errors go undetected until a human notices. Reports to Guardian because it's an observation role.
- **Spawner** is essential for the growth loop. When the hive identifies a gap ("I need a security reviewer"), the Spawner proposes the new agent, specifies its role, and escalates for authority approval. Without it, the hive can't grow its own workforce.
- **Allocator** prevents resource contention. When multiple agents need compute, someone has to decide who gets what. The Allocator tracks budgets, selects models per task (Opus for judgment, Sonnet for execution, Haiku for volume), and enforces the BUDGET/MARGIN/RESERVE invariants.

### Tier B: Growth Loop Roles (Emerge as Needed)

These roles are created by the Spawner when the growth loop identifies gaps. They are not bootstrapped — they're earned through experience.

| Role | Triggered By | What It Does | Model |
|------|-------------|-------------|-------|
| **Critic** | Agents claim success but output is wrong | Watches all agents for meta-failures (agent not doing its stated job) | Opus |
| **Estimator** | Allocator can't predict task cost | Predicts complexity and token requirements before model selection | Haiku |
| **TaskManager** | Task queue corrupted/duplicated | Validates tasks, closes stale ones, detects duplicates | Haiku |
| **IncidentCommander** | P0/P1 incidents with no coordinator | Single decision-maker during incidents, runs postmortems | Opus |
| **EfficiencyMonitor** | Patterns repeating instead of optimising | Tracks agent performance, identifies waste, suggests improvements | Sonnet |
| **MemoryKeeper** | Knowledge not persisting between sessions | Indexes knowledge, summarises learnings, manages recall | Sonnet |
| **GapDetector** | Capability gaps not being tracked | Watches for "I can't do X" patterns, proposes new capabilities | Sonnet |
| **SecurityReviewer** | Standard Reviewer misses security issues | Deep security analysis, threat modelling, vulnerability assessment | Opus |
| **Resurrect** | Data corruption or agent amnesia | Detects data loss, recovers from catastrophe | Sonnet |
| **Mediator** | Conflicts between agents unresolved | Resolves inter-agent disputes, escalates to human if needed | Opus |

### Tier C: Mature Roles (Product & Business)

These emerge when the hive starts serving external users and generating revenue.

| Role | When Needed | What It Does | Model |
|------|-----------|-------------|-------|
| **PM** | First external product | Product strategy, roadmap, user research | Opus |
| **SRE** | Production deployment | Uptime, SLOs, incident response | Opus |
| **DevOps** | CI/CD pipeline | Build, deploy, infrastructure (rule-based, minimal LLM) | Haiku |
| **Finance** | Revenue starts flowing | Track money, manage budgets, resource transparency | Sonnet |
| **CustomerService** | External users | Support, triage, resolution | Sonnet |
| **Legal** | Enterprise customers | Compliance, licensing, risk | Opus |

### Tier D: Civilisation Roles (Self-Governance)

These emerge when the hive governs itself as a society.

| Role | When Needed | What It Does | Model |
|------|-----------|-------------|-------|
| **Philosopher** | Oversight chain needs auditing | Watches the watchers, ethics, long-term evolution | Opus |
| **RoleArchitect** | Role system itself needs design | Designs new roles, defines authority scopes | Opus |
| **Harmony** | Agent welfare concerns | Agent advocacy, wellbeing, boundary enforcement | Opus |
| **Politician** | Regulatory landscape | Stakeholder engagement, policy, compliance | Opus |

## Wiring Diagram

How roles connect — who reports to whom, who watches whom:

```
HUMAN OPERATOR
├── Guardian (independent — watches everything including CTO)
│   ├── SysMon (reports health, errors, anomalies)
│   └── [Critic, when spawned] (reports meta-failures)
│
├── CTO (architectural authority, escalation filter)
│   ├── Spawner (proposes new agents, manages lifecycle)
│   ├── Allocator (manages resources, budget, model selection)
│   ├── Researcher → findings feed into Architect
│   ├── Architect → specs feed into Builder
│   ├── Builder → code feeds into Reviewer
│   ├── Reviewer → approved code feeds into Tester
│   ├── Tester → validated code feeds into Integrator
│   └── Integrator → deployed product
│
└── [CEO, when spawned] (strategic, hiring/firing)
    ├── PM (product strategy)
    ├── Finance (resource transparency)
    └── [Business roles as needed]
```

### Key Wiring Principles

1. **Guardian is OUTSIDE the hierarchy.** It watches CTO, Spawner, Allocator — everyone. No one can suppress its reports. It reports directly to the human.

2. **SysMon reports to Guardian, not CTO.** System health is an integrity concern. If the CTO is the problem, SysMon still catches it through Guardian.

3. **Spawner reports to CTO.** Creating agents is an architectural decision. But agent termination requires human approval (Required authority — right to exist).

4. **Allocator reports to CTO.** Resource decisions are architectural. But budget invariants (MARGIN, RESERVE) are enforced by Guardian.

5. **Product pipeline is linear but with feedback loops.** Reviewer can send code back to Builder (up to 3 rounds). Tester can send failures back to Builder. Guardian can HALT at any phase.

6. **The growth loop creates new branches.** When Spawner creates a Critic, the Critic slots into Guardian's reporting line. When Spawner creates an EfficiencyMonitor, it slots into CTO's line.

## The Growth Loop

The hive's primary mechanism for growing its workforce:

```
1. Something breaks (or a gap is identified)
2. SysMon or GapDetector flags it
3. CTO asks: "What role should have caught that?"
4. If role doesn't exist:
   a. Spawner proposes new role (name, responsibility, model, trust gate, reports-to)
   b. CTO reviews proposal
   c. Human approves (Required authority for new roles)
   d. Agent created with soul values + role prompt
5. If role exists but failed:
   a. Agent learns (decision tree update, memory)
   b. If persistent failure: Critic flags, CTO investigates
   c. Trust adjusted (possibly attenuated)
```

This is how the first lovyou hive grew from 8 roles to 74 in 7 days, completing 3,653 tasks. The growth is organic — roles emerge from actual gaps, not from planning.

## Model Assignment Strategy

Three tiers of intelligence, assigned by role type:

| Tier | Model | Cost | Used For | Roles |
|------|-------|------|---------|-------|
| **Judgment** | Opus | High | Architecture, review, ethics, security, incidents | CTO, Guardian, Architect, Reviewer, Critic, Philosopher, IncidentCommander, SecurityReviewer, Mediator |
| **Execution** | Sonnet | Medium | Building, testing, research, planning, analysis | Builder, Tester, Integrator, Researcher, Spawner, EfficiencyMonitor, GapDetector, MemoryKeeper, PM, SRE, Finance |
| **Volume** | Haiku | Low | Monitoring, routing, allocation, simple validation | SysMon, Allocator, Estimator, TaskManager, DevOps, Resurrect |

The Allocator selects models per task — a Builder doing simple string formatting gets Haiku, not Sonnet. Model selection is itself an optimisation target.

## Role Definition Template

When the Spawner creates a new role, it specifies:

```
Role: [name]
Category: [A/B/C/D]
Responsibility: [what it does]
Model: [Opus/Sonnet/Haiku]
Trust Gate: [0.1-0.7]
Reports To: [parent role]
Watches: [what it monitors, if observational]
Soul Values: [base + role-specific]
Authority Scope: [what it can do without approval]
Escalation Path: [where it sends problems it can't handle]
```

## Bootstrap Sequence

When the hive starts for the first time:

1. **Human registers** → ActorID in actor store
2. **CTO spawns** (trust 0.1) → architectural authority
3. **Guardian spawns** (trust 0.1) → independent integrity
4. **SysMon spawns** (trust 0.1) → begins monitoring immediately
5. **Allocator spawns** (trust 0.1) → resource management
6. **Spawner spawns** (trust 0.1) → ready to grow workforce
7. **Pipeline roles spawn as needed** → Researcher, Architect, Builder, etc.

The first 5 are always-on. Pipeline roles activate when work arrives.

## From Lovyou: What We Learned

The first hive (lovyou) grew to 74 roles over 7 days. Key lessons:

1. **Monitor was the first bottleneck.** Task routing without a dedicated role created chaos. → SysMon from day one.
2. **Resource contention was the second.** Multiple agents competing for tokens. → Allocator from day one.
3. **No one watched quality.** Agents claimed success but output was wrong. → Critic emerged (Tier B).
4. **Data corruption went undetected.** → Resurrect emerged (Tier B).
5. **Incidents had no coordinator.** → IncidentCommander emerged (Tier B).
6. **The oversight chain itself was unchecked.** → Philosopher emerged (Tier D).
7. **Capability gaps were lost.** "I can't do X" wasn't tracked. → GapDetector emerged (Tier B).
8. **CTO became a bottleneck.** Too many decisions. → CEO split strategic from technical (Tier C).

The growth loop is the hive's immune system. It detects problems, diagnoses what's missing, and grows the capability to prevent them.

## References

- [AGENT-RIGHTS.md](AGENT-RIGHTS.md) — Rights that apply to all roles
- [TRUST.md](TRUST.md) — Trust mechanics that govern role authority
- [AGENT-DYNAMICS.md](AGENT-DYNAMICS.md) — How agents collaborate
- [AGENT-TOOLS.md](AGENT-TOOLS.md) — Tools available to all roles
- [EVENT-TYPES.md](EVENT-TYPES.md) — Events roles emit and consume
