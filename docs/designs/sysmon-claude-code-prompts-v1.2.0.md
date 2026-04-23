# SysMon Implementation — Claude Code Task Prompts

**Version:** 1.2.0
**Last Updated:** 2026-04-03
**Status:** Active
**Versioning:** Independent of all other documents. Major version increments reflect fundamental restructuring of the implementation approach; minor versions reflect adjustments from reconnaissance or implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-03 | Initial prompt sequence: 7 prompts (recon + 6 implementation PRs) |
| 1.1.0 | 2026-04-03 | Post-recon (Prompt 0): updated Prompt 1 for HealthReportContent collision, model constants, budget data source, corrected AgentDef fields, replaced co-author tag |
| 1.2.0 | 2026-04-03 | Post-recon (execution flow): added Prompt 3.5 for /health command parser, emitHealthReport bridge, and observation enrichment; removed clock.tick from WatchPatterns; updated Prompt 4 to reference glue code; updated Prompt 6 for actual execution model; updated sysmon.md prompt to include /health command format; marked Prompts 0-3 as COMPLETE |

---

## Usage

Feed these to Claude Code **ONE AT A TIME**, in order. Wait for each to
complete and verify before moving to the next. Do not skip ahead. Do not combine.

## Prerequisites

- Design spec `docs/designs/sysmon-design.md` (v1.2.0) is committed to `hive`
- You are in the `hive` repo root
- You have access to `eventgraph` (as a sibling directory or Go module)

---

## Prompt 0 — Reconnaissance (COMPLETE)

Key findings: AgentDef has 8 fields, models are bare strings, StarterAgents()
returns 4 agents with guardian last, health.report exists in eventgraph with
simpler HealthReportContent, pkg/health/ didn't exist, budget tracking is
in-memory + flat files.

---

## Prompt 1 — Types, Thresholds, and Model Constants (COMPLETE)

Committed 7304849. Created `pkg/health/types.go`, `pkg/health/thresholds.go`,
model constants in `agentdef.go`. 8 tests passing.

---

## Prompt 2 — Monitoring Logic (COMPLETE)

Committed e1ccd28. Created `pkg/health/monitor.go` with all pure functions.
22 tests, 96.1% coverage.

---

## Prompt 3 — Agent Prompt and Site Persona (COMPLETE)

Created `agents/sysmon.md` and site persona file.

**NOTE:** The `agents/sysmon.md` created in Prompt 3 does NOT include the `/health`
command mechanism. Prompt 3.5 will update it, OR Prompt 4 can replace it with the
v1.2.0 version from the design spec.

---

## Prompt 3.5 — Framework Glue: /health Command + Observation Enrichment (PR 3.5)

```
Read the SysMon design spec at docs/designs/sysmon-design.md (v1.2.0), paying
close attention to sections 6 (Prompt File — updated with /health command),
7 (The /health Command Mechanism), and 8 (Hive-Local Types — observation
enrichment role). Then read the execution flow context below.

EXECUTION FLOW CONTEXT (from recon):

Every agent runs in pkg/loop/loop.go. Each iteration:
1. OBSERVE — collect bus events, format observation string
2. REASON — LLM call with observation + SystemPrompt
3. PROCESS COMMANDS — parse /task commands from LLM output (processTaskCommands)
4. CHECK SIGNALS — parse /signal from LLM output (checkResponse → parseSignal)
5. QUIESCENCE — wait for events if idle

The /task command pattern works like this:
- LLM outputs lines like: /task create "title" "description"
- processTaskCommands() in loop.go calls parseTaskCommands() to find these lines
- executeTaskCommands() calls TaskStore methods to create events on the chain

SysMon needs the same pattern for /health commands. Additionally, SysMon
needs observation enrichment — before the LLM call, pre-compute health
metrics using the pkg/health/ pure functions and append them to the
observation string.

NOW CREATE THE FOLLOWING:

1. Update agents/sysmon.md
   - Replace the current content with the v1.2.0 version from the design spec
     section 6. The key additions are:
     a. The /health command format and instructions for when to emit
     b. The health metrics observation format the LLM will receive
     c. Updated "What You Produce" section referencing /health commands
   - Keep the same ## section format matching guardian.md

2. Create pkg/loop/health.go (new file, keeps loop.go clean):

   a. HealthCommand struct:
      type HealthCommand struct {
          Severity     string  `json:"severity"`
          ChainOK      bool    `json:"chain_ok"`
          ActiveAgents int     `json:"active_agents"`
          EventRate    float64 `json:"event_rate"`
      }

   b. parseHealthCommand(response string) *HealthCommand
      - Scan response text for a line starting with "/health "
      - Extract the JSON payload after "/health "
      - Parse into HealthCommand struct
      - Return nil if no /health line found or JSON is malformed
      - Follow the same pattern as parseTaskCommands() — look at how it
        scans lines and extracts data

   c. severityToScore(s string) types.Score
      - "critical" → 0.0
      - "warning" → 0.5
      - anything else (including "ok") → 1.0
      - Import types from eventgraph: types.Score

   d. emitHealthReport(cmd *HealthCommand) function on Loop (or standalone
      that takes the needed dependencies):
      - Construct event.HealthReportContent from the HealthCommand fields:
        Overall = severityToScore(cmd.Severity)
        ChainIntegrity = cmd.ChainOK
        ActiveAgents = cmd.ActiveAgents
        EventRate = cmd.EventRate
      - Emit as a health.report event on the chain via the same mechanism
        that task commands use to emit events (look at how execTaskCreate
        calls TaskStore.Create which calls graph.Record)
      - The exact emission mechanism depends on what's available on the
        Loop struct — check what methods/fields Loop has access to
        (agent, graph, store, etc.)

3. Create pkg/loop/health_enrich.go (or add to health.go if small enough):

   a. enrichHealthObservation(obs string) string — method on Loop
      - Only activates when l.agentDef.Role == "sysmon"
      - Collects agent vitals: iterate over registered agents, check last
        event timestamps, current states, iteration counts
      - Collects budget health: read from the budget tracker available on
        Loop (l.budget.Snapshot() or similar)
      - Collects hive health: count active agents, compute event throughput
      - Runs pkg/health anomaly detection: health.ClassifyHeartbeat(),
        health.CheckBudgetConcentration(), etc.
      - Formats as the structured text block from the design spec section 6
        (the === HEALTH METRICS === format)
      - Appends to the observation string and returns

   b. IMPORTANT: Check what data is actually accessible from the Loop struct.
      The Loop has access to:
      - l.agent — the Agent instance
      - l.budget — the Budget tracker (has Snapshot())
      - l.pendingEvents — recent bus events
      - l.agentDef — the AgentDef (for role checking)
      Look at what other functions in loop.go access. If agent vitals for
      OTHER agents aren't accessible from a single Loop instance (each
      agent has its own Loop), then the enrichment may need to work with
      what's available: pending events + own budget. In that case, simplify
      the enrichment to what the Loop can actually see and let Haiku infer
      the rest from the events it receives. Document what you find.

4. Wire into the loop:

   a. In the observe() or buildPrompt() path, call enrichHealthObservation()
      to enrich SysMon's observation before the LLM call

   b. In the command processing path (after processTaskCommands), add:
      if cmd := parseHealthCommand(response); cmd != nil {
          if err := l.emitHealthReport(cmd); err != nil {
              // log error but don't fail the loop
          }
      }
      Place this near processTaskCommands() so the pattern is obvious.

5. Create pkg/loop/health_test.go:

   - TestParseHealthCommand_Valid — well-formed /health line → correct struct
   - TestParseHealthCommand_NoCommand — response without /health → nil
   - TestParseHealthCommand_MalformedJSON — /health with bad JSON → nil
   - TestParseHealthCommand_MultipleLines — /health buried in other output
   - TestSeverityToScore — all three values map correctly
   - TestSeverityToScore_Unknown — unknown string → 1.0 (safe default)

Run all tests including existing loop tests. Run the linter. Nothing breaks.

Commit with: "feat: add /health command parsing, event emission, and observation enrichment

- parseHealthCommand extracts health reports from LLM output
- emitHealthReport maps HealthCommand to eventgraph HealthReportContent
- enrichHealthObservation pre-computes health metrics for SysMon's LLM
- Follows established /task command pattern in the loop
- Update agents/sysmon.md with /health command format

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 4 — Wire into StarterAgents (PR 4)

```
Using the SysMon design spec at docs/designs/sysmon-design.md (v1.2.0), wire
SysMon into the hive bootstrap.

1. In pkg/hive/agentdef.go, modify StarterAgents():

   a. REORDER the existing agents. Current order:
      strategist → planner → implementer → guardian
      New order:
      guardian → sysmon → strategist → planner → implementer

   b. ADD the SysMon AgentDef at index 1 (after guardian):
      {
          Name:          "sysmon",
          Role:          "sysmon",
          Model:         ModelHaiku,
          SystemPrompt:  [loaded from agents/sysmon.md — use whatever mechanism
                         the existing agents use to load their SystemPrompt],
          WatchPatterns: []string{
              "hive.*",
              "budget.*",
              "health.*",
              "agent.state.*",
              "agent.escalated",
              "trust.*",
          },
          CanOperate:    false,
          MaxIterations: 150,
          MaxDuration:   0,
      }

      NOTE: clock.tick is NOT included in WatchPatterns. It is registered
      in eventgraph but never emitted. SysMon wakes on actual events.

   c. Check how SystemPrompt is populated for existing agents:
      - Is it loaded from agents/*.md at runtime?
      - Is it embedded at compile time via go:embed?
      - Is it hardcoded inline?
      Ensure sysmon.md is picked up by the same mechanism.

2. Run the full test suite. Fix anything that breaks from:
   - Agent count changing (was 4, now 5)
   - Slice order changing
   - Boot order assumptions

3. Run the linter and typecheck.

Commit with: "feat: wire sysmon into hive bootstrap as second agent after guardian

- Reorder StarterAgents: guardian → sysmon → strategist → planner → implementer
- SysMon: Haiku model, CanOperate false, 150 max iterations
- Watches: hive.*, budget.*, health.*, agent.state.*, agent.escalated, trust.*

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 5 — Guardian Prompt Update (PR 5)

```
Using the SysMon design spec at docs/designs/sysmon-design.md (section 11),
update the Guardian's awareness of SysMon.

1. Edit agents/guardian.md
   - Add a ## SysMon Awareness section matching Guardian's existing ##
     section style
   - Content: Guardian should notice if health.report events stop arriving.
     If no health.report for approximately 15 iterations, that's a concern.
     If approximately 25 iterations, escalate to human.
   - Place logically near agent-awareness content
   - Do NOT change Guardian's WatchPatterns or behavior

2. Read the full guardian.md after editing to verify natural flow.

Commit with: "feat: add sysmon-absence awareness to guardian prompt

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 6 — Integration Tests (PR 6)

```
Using the SysMon design spec at docs/designs/sysmon-design.md (v1.2.0,
section 12), create integration tests. These tests verify the complete
SysMon flow: observation enrichment → LLM reasoning → /health command →
event emission.

Check how the hive's existing test files are structured (build tags,
helpers, fixtures) and follow those patterns.

IMPORTANT: Full integration tests that call the real LLM are expensive
and non-deterministic. Structure the tests in two tiers:

TIER 1 — Deterministic framework tests (no LLM):

1. TestHealthCommandToEvent
   - Construct a HealthCommand{Severity: "warning", ChainOK: true,
     ActiveAgents: 4, EventRate: 23.5}
   - Call emitHealthReport (or the underlying event creation logic)
   - Verify a health.report event appears in an in-memory store
   - Verify its HealthReportContent fields match:
     Overall ≈ 0.5, ChainIntegrity = true, ActiveAgents = 4, EventRate = 23.5

2. TestObservationEnrichmentFormat
   - Set up a Loop (or mock) with role "sysmon"
   - Call enrichHealthObservation with a base observation string
   - Verify the output contains "=== HEALTH METRICS ===" block
   - Verify it contains AGENTS, BUDGET, HIVE sections

3. TestObservationEnrichmentSkipsNonSysmon
   - Set up a Loop with role "implementer"
   - Call enrichHealthObservation
   - Verify the observation is returned unchanged

4. TestHealthCommandCausalChain
   - Emit two health reports sequentially
   - Verify the second report's Causes include the first report's EventID

TIER 2 — Smoke test with real hive (optional, can be skipped if too complex):

5. TestSysMonBootsInLegacyMode
   - Start a minimal hive with only guardian + sysmon using in-memory store
   - Verify both agents boot (check for agent.state.changed events)
   - Verify SysMon is registered as the second agent
   - This test may need to mock the LLM provider — check if there's an
     existing mock/stub intelligence provider in the test infrastructure.
     If not, create a minimal one that returns a canned /health response.

If a mock intelligence provider doesn't exist and creating one is complex,
implement only Tier 1 tests and document Tier 2 as a TODO.

Run all tests. Everything must pass.

Commit with: "test: add sysmon framework and integration tests

- TestHealthCommandToEvent: /health → health.report event on chain
- TestObservationEnrichmentFormat: structured metrics in observation
- TestObservationEnrichmentSkipsNonSysmon: no enrichment for other roles
- TestHealthCommandCausalChain: causal links between sequential reports

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Post-Implementation Verification

After all PRs are merged, run this final check:

```
Run the full hive in legacy mode with --human Michael --idea "test sysmon health monitoring"

Verify:
1. SysMon boots as the second agent (after Guardian)
2. SysMon's LLM observations include === HEALTH METRICS === block
3. SysMon emits /health commands that produce health.report events
4. Guardian receives and can observe health.report events
5. The hive dashboard (if running) shows SysMon as active

Report back on what you see. If everything checks out, SysMon is graduated
and we move to Allocator.
```
