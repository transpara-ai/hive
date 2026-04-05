# Telemetry & Mission Control — Claude Code Task Prompts

**Version:** 0.4.1
**Date:** 2026-04-04
**Status:** Active — Prompts 0-4 complete, ready for Prompt 5
**Versioning:** Independent of all other documents. Major version increments reflect fundamental restructuring of the implementation approach; minor versions reflect adjustments from reconnaissance or implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 0.1.0 | 2026-04-04 | Initial prompt sequence: 6 prompts (recon + 5 implementation PRs) based on telemetry-mission-control-design-v0.1.0.md |
| 0.2.0 | 2026-04-04 | Pre-recon update: SysMon and Allocator graduated. Added recon items 11-14. Updated seed data, agent counts, test fixtures, conditional LastResponse. |
| 0.3.0 | 2026-04-04 | Post-recon: all corrections and gaps resolved. RegisterAgent pattern, process boundary, exact code paths. |
| 0.3.1 | 2026-04-04 | Post Prompt 1: marked Prompt 1 COMPLETE. Added Prompt 1.1 (seed fix) — ON CONFLICT DO NOTHING → DO UPDATE SET with COALESCE for timestamps. One-time cleanup SQL for prior-run remnants. |
| 0.3.2 | 2026-04-04 | Post Prompt 2: marked complete, 5 deviations documented, updated Prompt 3 context. |
| 0.4.0 | 2026-04-04 | Dashboard moved from lovyou-ai-work to lovyou-ai-summary. Prompt 4 rewritten for standalone HTML with URL-param config. |
| 0.4.1 | 2026-04-04 | Prompts 3 and 4 COMPLETE. No deviations. Prompt 3: 480-line telemetry.go in work-server, 6 endpoints, nullable pointer types, 42P01 error code check. Prompt 4: 450-line dashboard.html in lovyou-ai-summary, URL-param config, createElement (no innerHTML), security hook passed. |

---

## Usage

Feed these to Claude Code **ONE AT A TIME**, in order. Wait for each to
complete and verify before moving to the next. Do not skip ahead. Do not combine.

**Prompts 1–2 execute in the `lovyou-ai-hive` repo.**
**Prompt 3 executes in the `lovyou-ai-work` repo.**
**Prompt 4 executes in the `lovyou-ai-summary` repo.**
**Prompt 5 spans all three repos.**

## Prerequisites

- Design spec `telemetry-mission-control-design-v0.4.1.md` is available (upload or commit)
- You have all three repos checked out:
  - `lovyou-ai-hive` — telemetry writer, pruner, types, LastResponse, agent registration (Prompts 1-2, DONE)
  - `lovyou-ai-work` — telemetry API endpoints (Prompt 3)
  - `lovyou-ai-summary` — mission control dashboard (Prompt 4)
- hive-postgres is running (`postgres://hive:hive@localhost:5432/hive`)

---

## Prompt 0 — Reconnaissance (COMPLETE)

14 areas investigated. Key findings:

- **No agent registry** — `Runtime.defs` is private; after `Run()`, agents exist only in Loop goroutines. TelemetryWriter uses `RegisterAgent()` called during spawn.
- **LastResponse absent** — Loop struct has 9 fields, none store LLM response. Insertion point: `loop.go:232`, after Reason/Operate call.
- **Iterations from BudgetRegistry** — `Agent` struct has no Iterations field. Use `BudgetRegistry.Snapshot() → BudgetEntry.Budget.Snapshot().Iterations`.
- **Model not on Agent** — `AgentDef.Model` is available only at spawn time. Pass to writer via registration.
- **Trust score** — edges table query; nullable for v1.
- **Event type** — Allocator emits `agent.budget.adjusted` (not `budget.allocated`). 0 such events in DB yet.
- **Process boundary** — writer in hive process, API in work-server process, postgres is the bridge.
- **Table creation** — `CREATE TABLE IF NOT EXISTS` in Go const strings, no migration files.
- **Store** — `store.Count()` returns `(int, error)` directly; `store.VerifyChain()` returns `ChainVerifiedContent{Valid, Length, Duration}`.
- **Bus** — `bus.Subscribe("*", handler)` is goroutine-safe, buffered channel, panic recovery.
- **6 agents** in StarterAgents(): guardian, sysmon, allocator, strategist, planner, implementer.
- **18 event types**, 1,008 events in live DB. 40 `health.report` events confirm SysMon active.

---

## Prompt 1 — Schema + Types (COMPLETE)

Committed. Created `pkg/telemetry/schema.go` (EnsureTables, const schema, seed data),
`pkg/telemetry/types.go` (4 structs with JSON tags), `pkg/telemetry/types_test.go`
(5 tests passing). Wired `EnsureTables(ctx, pool)` into `cmd/hive/main.go` after
actor store creation — logs warning and continues on failure.

---

## Prompt 1.1 — Seed Data Fix (COMPLETE)

Committed 3b354b5. Changed `ON CONFLICT (phase) DO NOTHING` to `ON CONFLICT (phase) DO UPDATE SET`
with COALESCE for timestamps. Seed data is authoritative for label/status/notes;
manual timestamp updates from graduation ceremonies survive restarts.

---

## Prompt 2 — Telemetry Writer + LastResponse (COMPLETE)

Committed 4b5b4bf on `feat/telemetry-schema-writer`. Created:
- `pkg/telemetry/writer.go` — Writer, AgentRegistration, snapshot collection, event stream capture
- `pkg/telemetry/pruner.go` — 24h/7d/1000-row retention
- `pkg/telemetry/writer_test.go` — 5 tests, all pass
- `pkg/loop/loop.go` — `lastResponse` field + `LastResponse()` getter
- `pkg/loop/loop_test.go` — 2 new tests (getter + truncation)
- `pkg/hive/runtime.go` — TelemetryWriter on Config/Runtime, agent registration during spawn, response capture via OnIteration
- `cmd/hive/main.go` — Writer + Pruner creation when postgres available

### Design Deviations (5)

**1. No Loop pointer on AgentRegistration.** RunConcurrent() creates Loop instances
as goroutine-local variables — they're never exposed. Writer uses
`RecordResponse(agentName, response)` fed by Runtime's OnIteration callback.
AgentRegistration struct has Name, Role, Model, Agent, MaxIterations (no Loop).

**2. Writer wired via hive.Config.TelemetryWriter**, not standalone. Runtime calls
SetBudgetRegistry(), RegisterAgent(), chains RecordResponse() into OnIteration,
and starts the writer goroutine + bus subscription before RunConcurrent().

**3. Chain verification cached every 5 minutes.** store.VerifyChain() walks the
full chain (1,008+ events) — too expensive for 10s interval. chainOK holds the
last known value between verifications.

**4. Event rate nil for v1.** Marked TODO in writer.

**5. Daily cap nil for v1.** Marked TODO in writer.

---

## Prompt 3 — Telemetry API (COMPLETE)

**Repo: `lovyou-ai-work`**

Committed. Created `cmd/work-server/telemetry.go` (480 lines): 4 response types,
6 handlers, 3 shared query helpers. Nullable fields as `*float64`/`*string`
(serialize as `null`). Error handling: `42P01` → 503 "tables not initialized",
empty tables → 200 with empty arrays, pool nil → 503. Added `pool *pgxpool.Pool`
to server struct in `main.go`, registered 6 `GET /telemetry/*` routes.

---

## Prompt 4 — Mission Control Dashboard (COMPLETE)

**Repo: `lovyou-ai-summary`** (`github.com/transpara-ai/lovyou-ai-summary`)

Committed. Created `dashboard.html` (450 lines): standalone vanilla JS + inline CSS,
no framework, no build step. URL-param configuration (`?api=...&key=...`), config
screen when params missing. createElement/textContent/appendChild (no innerHTML —
passed security hook). Five sections: connection indicator (1s tick), phase timeline,
agent grid with expandable cards, hive health, event stream with auto-scroll.
503 shows "Telemetry not initialized." CORS errors show human-readable message.
Updated `README.md` with usage documentation.

---

## Prompt 5 — Integration, Phase Updates, and Polish (PR 5)

**Repos: `lovyou-ai-work` (primary) + `lovyou-ai-hive` + `lovyou-ai-summary` (verification)**

```
Read the telemetry design spec at docs/designs/telemetry-mission-control-design-v0.4.1.md,
focusing on Sections 8-9.

This is the integration and polish pass. Prompts 1-4 should be working
individually. This prompt adds phase updates, verifies end-to-end, and documents.

1. PHASE UPDATE ENDPOINT (in lovyou-ai-work)

   Add to cmd/work-server/telemetry.go:

   func (sv *server) updatePhase(w http.ResponseWriter, r *http.Request) {
       // Parse phase number from path
       phaseStr := r.PathValue("phase")
       phase, err := strconv.Atoi(phaseStr)
       if err != nil {
           writeErr(w, http.StatusBadRequest, "invalid phase number")
           return
       }

       // Parse body
       var body struct {
           Status string `json:"status"`
           Notes  string `json:"notes"`
       }
       if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
           writeErr(w, http.StatusBadRequest, "invalid request body")
           return
       }

       // Validate status
       validStatuses := map[string]bool{"blocked": true, "in_progress": true, "complete": true}
       if !validStatuses[body.Status] {
           writeErr(w, http.StatusBadRequest, "status must be: blocked, in_progress, complete")
           return
       }

       // Build UPDATE query with conditional timestamp logic:
       // - started_at = now() when status changes to in_progress (only if currently null)
       // - completed_at = now() when status changes to complete
       // - completed_at = null when status changes to anything other than complete

       // ... implement the SQL update ...

       // Return updated phase
   }

   Register route in main.go:
   mux.HandleFunc("POST /telemetry/phases/{phase}", sv.auth(sv.updatePhase))

2. END-TO-END VERIFICATION SCRIPT

   Create a verification checklist (as comments in telemetry.go or a
   separate docs/telemetry.md):

   a. Verify tables exist:
      docker exec hive-postgres-1 psql -U hive -d hive -c "\dt telemetry_*"

   b. Verify seed data:
      docker exec hive-postgres-1 psql -U hive -d hive \
        -c "SELECT phase, label, status FROM telemetry_phases ORDER BY phase"

   c. Verify API responds:
      curl -s -H "Authorization: Bearer $API_KEY" http://localhost:8080/telemetry/phases | jq .

   d. Verify dashboard loads:
      Open dashboard.html?api=http://nucbuntu:8080&key=$API_KEY in browser
      (serve from lovyou-ai-summary repo via any static file server, or
      open directly if CORS allows file:// origin)

   e. When hive is running, verify:
      - Agent cards appear within 15 seconds
      - Event stream shows health.report events from SysMon
      - Hive health shows accurate chain_length (~1,008+)

   f. Test phase update:
      curl -X POST -H "Authorization: Bearer $API_KEY" \
           -H "Content-Type: application/json" \
           -d '{"status":"in_progress","notes":"CTO implementation started"}' \
           http://localhost:8080/telemetry/phases/2
      # Verify dashboard reflects change on next poll

3. DOCUMENTATION

   Add to lovyou-ai-work README (or create docs/telemetry.md):

   - What the telemetry system does (one paragraph)
   - Dashboard: in lovyou-ai-summary — open dashboard.html?api=URL&key=KEY
   - API endpoints with curl examples
   - Phase update instructions
   - Configuration: TELEMETRY_INTERVAL env var (default 10s)
   - Pruning: 24h agent snapshots, 7d hive snapshots, 1000 event cap
   - Architecture note: writer in hive, API in work-server, dashboard in
     lovyou-ai-summary — postgres is the bridge

4. FINAL CHECKS

   - Run full test suite in both repos
   - Run linter in both repos
   - Verify docker compose up brings up everything cleanly
   - Verify hive starts with telemetry writer active (check logs for
     "telemetry" or "snapshot" messages)
   - Verify work-server serves /telemetry/ dashboard and all API endpoints

Run all tests. Run the linter.

Commit with: "feat: phase updates, verification, and telemetry documentation

- POST /telemetry/phases/{phase} with timestamp auto-management
- End-to-end verification checklist
- Telemetry system documentation

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Post-Implementation Verification

After all PRs are merged in all three repos, run this final check:

```
Bring up the full stack on nucbuntu:

  docker compose up -d
  # Start hive and work-server

Then verify the following:

1. Telemetry tables exist in hive-postgres with seed data for phases 0-8
2. Dashboard loads from lovyou-ai-summary:
   Open dashboard.html?api=http://nucbuntu:8080&key=$API_KEY
3. Missing URL params show configuration screen (not a broken page)
4. Connection indicator is green (or gray if hive agents aren't running)
5. Phase timeline shows:
   - Phase 0 (Foundation): complete (green)
   - Phase 1 (Operational infrastructure): complete (green)
   - Phase 2 (Technical leadership): blocked (gray)
   - Phases 3-8: blocked (gray)
6. When hive agents are running:
   a. Agent cards appear within 15 seconds (6 agents: guardian, sysmon,
      allocator, strategist, planner, implementer)
   b. Each card shows correct model (haiku/sonnet/opus), state, iterations
   c. Event stream populates — health.report from SysMon, agent.state.changed
      from all agents, work.task.* from strategist/planner/implementer
   d. Hive health card shows chain_length ~1,008+, chain_ok true
   e. Clicking an agent card reveals its last LLM message
   f. SysMon card shows haiku model, health.report as last event
   g. Allocator card shows haiku model, agent.state.changed as last event
      (no budget adjustments triggered yet in production)
7. When hive agents are NOT running:
   a. Dashboard shows last known state with stale indicator
   b. No errors in browser console
   c. No panics in work-server logs
8. Phase update works:
   curl -X POST -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d '{"status":"in_progress","notes":"CTO work started"}' \
        http://localhost:8080/telemetry/phases/2
   Dashboard reflects the change on next poll
9. After 1+ hours, pruner has run at least once (check hive logs)

Report back with:
- Screenshot or description of the dashboard
- Any issues encountered
- Telemetry is graduated and operational, or list what needs fixing
```
