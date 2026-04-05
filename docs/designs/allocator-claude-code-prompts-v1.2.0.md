# Allocator Implementation — Claude Code Task Prompts

**Version:** 1.1.0
**Last Updated:** 2026-04-04
**Status:** Active
**Versioning:** Independent of all other documents. Major version increments reflect fundamental restructuring of the implementation approach; minor versions reflect adjustments from reconnaissance or implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-03 | Initial prompt sequence: 8 prompts (recon + 7 implementation PRs). |
| 1.1.0 | 2026-04-04 | Post-recon: added Prompt 0.5 (BudgetRegistry infrastructure + agent.budget.adjusted event type). Updated all subsequent prompts to use BudgetRegistry as data source. Corrected WatchPatterns from budget.* to agent.budget.*. Marked Prompt 0 COMPLETE. |
| 1.2.0 | 2026-04-04 | Marked Prompt 0.5 and Prompt 1 COMPLETE. Design spec bumped to self-contained v1.2.0. |

---

## Usage

Feed these to Claude Code **ONE AT A TIME**, in order. Wait for each to
complete and verify before moving to the next. Do not skip ahead. Do not combine.

## Prerequisites

- Design spec `docs/designs/allocator-design.md` (v1.1.0) is committed to `lovyou-ai-hive`
- SysMon is graduated and running (health.report events flowing)
- You are in the `lovyou-ai-hive` repo root
- You have access to `lovyou-ai-eventgraph` (as a sibling directory or Go module)

---

## Prompt 0 — Reconnaissance (COMPLETE)

Key findings:
- MaxIterations is immutable (Budget has no setters, all fields private)
- BudgetSnapshot lacks limits (consumed values only, no max)
- Runtime doesn't store Loops (no cross-agent visibility)
- Event namespace is agent.budget.* not budget.* (WatchPatterns fix)
- agent.budget.adjusted doesn't exist (must create in eventgraph)
- SysMon enrichment is self-referential only (own budget, own events)
- No cross-agent budget mutation mechanism exists

All seven findings incorporated into design spec v1.1.0.

---

## Prompt 0.5 — Infrastructure: BudgetRegistry + Event Type (COMPLETE)

Committed c448b10 (hive) + f9b4cdc (eventgraph). BudgetRegistry in pkg/resources
with Register/Snapshot/AdjustMaxIterations/SetAgentState/TotalPool/TotalUsed.
Budget.SetMaxIterations() and MaxIterations() added. agent.budget.adjusted event
type created. 12 tests passing. BudgetRegistry in pkg/resources (not pkg/hive)
to avoid circular imports.

<details><summary>Original prompt (for reference)</summary>

```
Read the Allocator design spec at docs/designs/allocator-design.md (v1.2.0),
paying close attention to sections "Recon Findings" (R2, R3, R4, R5) and
section 8 (BudgetRegistry). Then read the actual code for context:

- pkg/hive/resources/budget.go — the Budget struct and Snapshot()
- pkg/hive/runtime.go — how spawnAgent() creates Loops
- pkg/loop/loop.go — the Loop struct and Config

This prompt creates two things: (1) a BudgetRegistry for cross-agent budget
visibility and mutation, and (2) the agent.budget.adjusted event type.

PART 1: Budget.SetMaxIterations()

1. In pkg/hive/resources/budget.go, add a method:

   func (b *Budget) SetMaxIterations(n int) {
       b.mu.Lock()
       defer b.mu.Unlock()
       b.maxIterations = n
   }

   Also add a getter if one doesn't exist:

   func (b *Budget) MaxIterations() int {
       b.mu.RLock()
       defer b.mu.RUnlock()
       return b.maxIterations
   }

   Check if Budget has a mutex. If not, check how thread safety is currently
   handled (it may use atomic operations or be single-threaded). Follow the
   existing pattern.

PART 2: BudgetRegistry

2. Create pkg/hive/budget_registry.go:

   type BudgetEntry struct {
       Name          string
       Budget        *resources.Budget
       MaxIterations int
       AgentState    string
   }

   type BudgetRegistry struct {
       mu      sync.RWMutex
       entries map[string]*BudgetEntry
   }

   Methods:
   - NewBudgetRegistry() *BudgetRegistry
   - Register(name string, budget *resources.Budget, maxIter int)
   - Snapshot() []BudgetEntry (returns copies, read-locked)
   - AdjustMaxIterations(name string, delta int, floor int, ceiling int) (int, int, error)
     → calls Budget.SetMaxIterations() on the target, updates entry.MaxIterations
     → returns (previousMax, newMax, error)
     → error if agent not found
     → clamps to floor/ceiling (does not error on clamp)
   - SetAgentState(name string, state string)
   - TotalPool() int (sum of MaxIterations)
   - TotalUsed() int (sum of each entry's Budget.Snapshot().Iterations)

3. Wire into Runtime:

   a. Add budgetRegistry *BudgetRegistry field to Runtime struct
   b. Initialize in New() or Run(): r.budgetRegistry = NewBudgetRegistry()
   c. In spawnAgent(), after creating the Budget, register:
      r.budgetRegistry.Register(def.Name, budget, def.MaxIterations)
   d. Add BudgetRegistry field to loop.Config:
      type Config struct {
          // ... existing fields ...
          BudgetRegistry *hive.BudgetRegistry  // may need interface to avoid circular import
      }
   e. Pass registry when creating Loop config:
      cfg.BudgetRegistry = r.budgetRegistry

   IMPORTANT: Check for circular import issues. If pkg/loop imports pkg/hive
   and pkg/hive imports pkg/loop, you'll need an interface:

   // In pkg/loop/loop.go or a new pkg/loop/types.go
   type BudgetRegistryReader interface {
       Snapshot() []hive.BudgetEntry  // or a loop-local type
       TotalPool() int
       TotalUsed() int
   }
   type BudgetRegistryWriter interface {
       AdjustMaxIterations(name string, delta int, floor int, ceiling int) (int, int, error)
   }

   The exact approach depends on the import graph. Investigate before coding.

PART 3: agent.budget.adjusted Event Type

4. In lovyou-ai-eventgraph, create the event type:

   a. Register event type "agent.budget.adjusted"
   b. Create content struct (following AgentBudgetAllocatedContent pattern):

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

   c. Register in the event type registry (follow the pattern used by
      agent.budget.allocated and agent.budget.exhausted)

   d. If eventgraph has multi-language type generation (TS, Python, Rust, .NET),
      generate or note what needs updating. The Go type is the source of truth;
      other languages can be updated later.

PART 4: Tests

5. Create pkg/hive/budget_registry_test.go:

   - TestRegisterAndSnapshot — register 3 agents, snapshot returns all 3
   - TestAdjustMaxIterations_Increase — delta +50 from 100 → 150
   - TestAdjustMaxIterations_Decrease — delta -30 from 100 → 70
   - TestAdjustMaxIterations_FloorClamp — delta -90 from 100, floor 20 → 20
   - TestAdjustMaxIterations_CeilingClamp — delta +600 from 100, ceiling 500 → 500
   - TestAdjustMaxIterations_UnknownAgent — returns error
   - TestSetAgentState — set to "Quiesced", verify in snapshot
   - TestTotalPool — 3 agents with 100, 150, 200 → 450
   - TestTotalUsed — verify sums Snapshot().Iterations across agents
   - TestConcurrentAccess — goroutines reading and writing simultaneously

6. If you added SetMaxIterations to Budget, add a test:

   - TestBudget_SetMaxIterations — set new value, verify via MaxIterations()

Run all tests including existing ones. Run the linter. Nothing breaks.

Commit with: "feat: add budget registry for cross-agent budget visibility and mutation

- BudgetRegistry: register, snapshot, adjust, state tracking
- Budget.SetMaxIterations() for runtime budget mutation
- agent.budget.adjusted event type in eventgraph
- Wire registry into Runtime and Loop config
- 10+ tests, concurrent safety verified

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

</details>

## Prompt 1 — Types, Thresholds, and Config (COMPLETE)

Committed b38f664. Created pkg/budget/types.go (5 structs with JSON tags),
pkg/budget/config.go (Config struct, DefaultConfig, LoadConfig with ALLOCATOR_*
env vars), pkg/budget/types_test.go (11 tests passing). 474 lines added.

<details><summary>Original prompt (for reference)</summary>

```
Read the Allocator design spec at docs/designs/allocator-design.md (v1.2.0),
sections 11 (Configuration) and 12 (Hive-Local Types).

Create the pure types and configuration for the Allocator:

1. Create pkg/budget/types.go:
   - AgentBudgetState struct
   - PoolState struct
   - AdjustmentRecord struct
   - BudgetReport struct
   - SysMonSummary struct
   - Config struct
   All fields as specified in the design spec.

2. Create pkg/budget/config.go:
   - DefaultConfig() — returns defaults matching section 11
   - LoadConfig() — reads ALLOCATOR_* env vars, falls back to defaults
   Follow the exact pattern from pkg/health/thresholds.go (SysMon).

3. Create pkg/budget/types_test.go:
   - JSON round-trip for each struct
   - DefaultConfig returns expected values
   - LoadConfig reads env vars correctly
   - LoadConfig falls back to defaults when vars missing

Run tests. Run linter.

Commit with: "feat: add allocator budget types and configuration

- AgentBudgetState, PoolState, AdjustmentRecord, BudgetReport structs
- Config with ALLOCATOR_* env var loading and sensible defaults

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

</details>

---

## Prompt 2 — Monitoring Logic (PR 2)

```
Read the Allocator design spec at docs/designs/allocator-design.md (v1.2.0),
section 12 (Monitoring Functions). Then read pkg/health/monitor.go for the
pattern to follow.

Create pkg/budget/monitor.go with these pure functions:

1. CheckConcentration(agents, pool, config) → []string warnings
2. CheckExhaustion(agents, config) → []string warnings
3. CheckIdleAgents(agents, config) → []string warnings
   CRITICAL: Quiesced agents EXCLUDED from idle warnings
4. CheckDailyBurnRate(pool, config) → *string warning or nil
5. CooldownRemaining(agent, history, currentIter, config) → int
6. GlobalCooldownRemaining(history, currentIter, config) → int
7. InStabilizationWindow(currentIter, config) → bool
8. BuildReport(agents, pool, sysmon, history, config, currentIter) → BudgetReport

Create pkg/budget/monitor_test.go with 17+ tests:
- Concentration: flags at 50%/40% threshold, clear at 30%
- Exhaustion: flags at 85%/80% threshold, clear at 50%
- Idle: flags active at 5%/10% threshold, SKIPS quiesced at 0%
- Daily burn: flags at 95%/90% threshold, clear at 60%
- Cooldown: active (3 ago, cooldown 10 → 7), clear (15 ago → 0), no history → 0
- Global cooldown: active and clear
- Stabilization: inside (iter 5/window 10), outside (iter 10), boundary (iter 9)
- BuildReport: all fields populated

Target: >= 80% coverage on pkg/budget/.

Commit with: "feat: add allocator monitoring logic with pure functions

- Quiesced agents excluded from idle warnings (SysMon graduation lesson)

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 3 — Agent Prompt and Site Persona (PR 3)

```
Read the Allocator design spec at docs/designs/allocator-design.md (v1.2.0),
sections 6 (Prompt File) and 10 (Site Persona).

1. Create agents/allocator.md with the exact content from section 6.
   Verify format matches agents/sysmon.md and agents/guardian.md.

2. Create site persona file following SysMon's persona pattern.
   Use exact content from section 10.

3. Read back both files to verify.

Commit with: "feat: add allocator agent prompt and site persona

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 3.5 — Framework Glue: /budget Command + Observation Enrichment (PR 3.5)

```
Read the Allocator design spec at docs/designs/allocator-design.md (v1.2.0),
sections 7 (/budget Command) and 13 (Observation Enrichment). Then read
pkg/loop/health.go for the SysMon pattern.

Create pkg/loop/budget.go with:

1. BudgetCommand struct (Agent, Action, Amount, Reason)

2. parseBudgetCommand(response string) *BudgetCommand
   Follow parseHealthCommand() pattern exactly.

3. validateBudgetCommand(cmd *BudgetCommand) error
   Safety checks in order:
   a. Stabilization window (l.iteration < config.StabilizationWindow)
   b. Agent exists (check BudgetRegistry)
   c. Amount > 0
   d. Global cooldown
   e. Agent cooldown
   f. Pool headroom (for increases)
   g. Floor/ceiling clamp (log but don't reject)

4. applyBudgetAdjustment(cmd *BudgetCommand) error
   a. Call l.budgetRegistry.AdjustMaxIterations()
   b. Emit agent.budget.adjusted event on chain
   c. Record adjustment for cooldown tracking

5. enrichBudgetObservation(obs string) string
   Data source: l.budgetRegistry.Snapshot() (NOT SysMon path)
   Format: === BUDGET METRICS === block matching section 6

6. Wire into loop.go:
   a. enrichBudgetObservation() in observe/buildPrompt path
   b. parseBudgetCommand() + validate + apply at line 224-227,
      between health commands and signal checking

7. Create pkg/loop/budget_test.go:
   - parseBudgetCommand: valid, no command, malformed JSON, buried in output
   - validateBudgetCommand: passes, stabilization, agent cooldown, global
     cooldown, unknown agent, insufficient pool, floor clamp
   - enrichBudgetObservation: formats correctly, skips non-allocator

Run all tests. Run linter. Nothing breaks.

Commit with: "feat: add /budget command parsing, validation, and budget adjustment

- Framework-level safety prevents thrashing (SysMon graduation lesson)
- Uses BudgetRegistry for cross-agent reads and writes

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 4 — Wire into StarterAgents (PR 4)

```
Using the Allocator design spec at docs/designs/allocator-design.md (v1.2.0),
wire Allocator into the hive bootstrap.

1. In pkg/hive/agentdef.go, modify StarterAgents():

   ADD Allocator AgentDef at index 2 (after guardian, sysmon):
   {
       Name:          "allocator",
       Role:          "allocator",
       Model:         ModelHaiku,
       SystemPrompt:  [loaded from agents/allocator.md],
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

   Verify boot order: guardian → sysmon → allocator → strategist → planner → implementer

2. Run full test suite. Fix breakage from agent count change (5 → 6).

3. Run linter and typecheck.

Commit with: "feat: wire allocator into hive bootstrap as third agent

- Boot order: guardian → sysmon → allocator → strategist → planner → implementer
- Watches: health.report, agent.budget.*, hive.*, agent.state.*

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 5 — Guardian Prompt Update (PR 5)

```
Update agents/guardian.md:

Add a ## Allocator Awareness section near the existing ## SysMon Awareness.

Content: Absence of agent.budget.adjusted events is NOT concerning — the
Allocator may correctly determine no adjustment is needed. However, if the
Allocator shows NO activity at all (no state changes, no /signal IDLE, no
/budget commands) for approximately 25 iterations, that IS concerning and
should be escalated.

Do NOT change Guardian's WatchPatterns.

Commit with: "feat: add allocator-absence awareness to guardian prompt

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 6 — Integration Tests (PR 6)

```
Create integration tests for the complete Allocator flow. Follow the pattern
from SysMon's integration tests.

Tier 1:
- TestAllocatorBootsInLegacyMode — index 2, Haiku model
- TestBudgetCommandToEvent — /budget → agent.budget.adjusted on chain
- TestObservationEnrichmentFormat — === BUDGET METRICS === block present
- TestObservationEnrichmentSkipsNonAllocator

Tier 2 (if mock provider available):
- TestStabilizationWindowBlocks — first 10 iterations rejected
- TestCooldownEnforcement — too-soon rejected, after cooldown accepted
- TestBudgetFloorEnforced — clamped to 20, not rejected
- TestPoolConservation — increase decreases pool remaining

Run all tests.

Commit with: "test: add allocator framework and integration tests

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Post-Implementation Verification

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

## Prompt Dependency Map

```
Prompt 0 (Recon) — COMPLETE
    │
    └── Prompt 0.5 (BudgetRegistry + event type) — COMPLETE
            │
            ├── Prompt 1 (types/config) — COMPLETE
            │       │
            │       └── Prompt 2 (monitoring logic) ← YOU ARE HERE
            │
            ├── Prompt 3 (prompt + persona) — independent, can run now
            │
            └── Prompt 3.5 (glue code) — depends on 2
                    │
                    └── Prompt 4 (StarterAgents) — depends on 3, 3.5
                            │
                            └── Prompt 5 (Guardian update)
                                    │
                                    └── Prompt 6 (integration tests)
```

---

*v1.2.0 updated to match self-contained design spec v1.2.0. Prompts 0, 0.5,
and 1 are COMPLETE. Prompt 2 is the next implementation step.*
