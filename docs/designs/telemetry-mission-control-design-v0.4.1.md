# Telemetry & Mission Control — Design Document

**Version:** 0.4.1 · **Date:** 2026-04-04
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier
**Status:** Design — recon complete, ready for implementation

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 0.1.0 | 2026-04-04 | Initial design: three components, postgres schema, writer/API/dashboard specs, implementation sequence, infrastructure context |
| 0.2.0 | 2026-04-04 | Pre-recon update: SysMon and Allocator graduated — Phase 1 complete. Updated seed data, agent counts (4→6+), example JSON, infrastructure context |
| 0.3.0 | 2026-04-04 | Post-recon (Prompt 0): 5 corrections, 3 gap resolutions. No agent registry — RegisterAgent(). Iterations from BudgetRegistry. LastResponse absent. Event type corrected. Trust nullable. Model at registration. Process boundary clarified. |
| 0.3.1 | 2026-04-04 | Post Prompt 1: Changed phase seed strategy from ON CONFLICT DO NOTHING to ON CONFLICT DO UPDATE SET. Seed data is authoritative for label/status/notes; manual timestamp updates from graduation ceremonies survive restarts via COALESCE. Added one-time cleanup SQL for prior-run remnants. |
| 0.3.2 | 2026-04-04 | Post Prompt 2: 5 design deviations. No Loop pointer, Config.TelemetryWriter wiring, chain verify cached 5min, event rate nil, daily cap nil. |
| 0.4.0 | 2026-04-04 | Dashboard moved from work to summary. Process boundary updated to three repos. |
| 0.4.1 | 2026-04-04 | Prompts 3 and 4 complete. No deviations from design. |

---

## 1. What This Is

A framework-level telemetry writer and live dashboard for the **Transpara AI hive deployment on nucbuntu**. This is the internal mission control — not the branded LovYou deployment on Fly.io, and not the static GitHub Pages architecture poster at `transpara-ai/summary`.

The system has three components:

1. **Telemetry writer** — Pure Go, no LLM. Runs inside the **hive process** (`hive`). Snapshots agent state, hive health, and recent events to new postgres tables every 10–15 seconds.
2. **Telemetry API** — HTTP endpoints serving the snapshot data as JSON. Added to the existing **work-server** (`work`). Reads from the same hive-postgres the writer writes to.
3. **Mission control dashboard** — Standalone HTML file in **summary**. Configurable API endpoint via URL parameters. Polls the telemetry API and renders live state. Served from nucbuntu (file server, GitHub Pages, or opened directly in browser).

### Process Boundary

The three components live in **three separate repos**:

```
hive               work               summary
(cmd/hive)                    (cmd/work-server)            (dashboard.html)
┌──────────────────┐          ┌────────────────────┐       ┌──────────────────┐
│  Hive Runtime    │          │  Work-Server       │       │  Mission Control │
│  ├─ Agent loops  │          │  ├─ /tasks/*       │       │  (static HTML)   │
│  ├─ Event bus    │ postgres │  ├─ /telemetry/*   │ HTTP  │  ├─ Polls API    │
│  ├─ Writer ──────┼───→──────┼──┤                 │←──────┼──┤  via fetch()  │
│  └─ Pruner       │ (5432)   │  └─ CORS + auth   │       │  └─ URL params   │
└──────────────────┘          └────────────────────┘       └──────────────────┘
```

The writer writes to postgres, the API reads from postgres, and the dashboard polls the API. Each component can be deployed, updated, and versioned independently.

---

## 2. Why Framework-Level, Not an Agent

The telemetry writer is pure Go infrastructure — not a hive agent, not an LLM consumer.

**The argument:** Telemetry is the one thing that absolutely cannot fail when everything else is failing. If Guardian is stuck and SysMon is silent, the telemetry writer still has to be scribbling state to postgres every 10 seconds. You can't get that guarantee from something that depends on an LLM call completing. It's the flight data recorder — it survives the crash precisely because it's dumber than everything around it.

**Relationship to SysMon:** SysMon is the intelligent health assessor — it runs on Haiku, consumes tokens, makes severity judgments, and emits `health.report` events on the chain. SysMon is graduated and running (40 `health.report` events confirmed in live DB). The telemetry writer is the dumb pipe that makes raw state queryable for the dashboard. They complement each other: SysMon reads the same reality the dashboard reads, but SysMon has opinions about it. The dashboard just shows the numbers.

**Relationship to the Allocator:** The Allocator is graduated and running, managing token budgets. It emits `agent.budget.adjusted` events when it rebalances budgets (though no adjustments have been triggered in production yet — the Allocator has been stable across all runs). The telemetry writer captures these events in the event stream and snapshots the budget state the Allocator manages. The Allocator acts on budget data; telemetry records it.

**Relationship to the event chain:** The telemetry writer does NOT write to the event graph. It writes to orthogonal postgres tables. The event chain is the civilization's auditable memory; the telemetry tables are ephemeral operational data that gets pruned. Different purposes, different storage.

---

## 3. What the Dashboard Shows

The dashboard has five views, matching the static architecture poster but driven by live data:

### 3.1 Expansion Phases

The phase timeline from the architecture poster, but reflecting actual deployment state. Each phase shows status (blocked / in_progress / complete), start and completion timestamps, and which agents graduated or are pending.

**Data source:** `telemetry_phases` table, updated when agents graduate (via `POST /telemetry/phases/{phase}` or direct SQL).

### 3.2 Role Tiers

The four-tier agent grid (A/B/C/D), but with live status dots based on actual runtime state:

- **Running** (green) — Agent has a current snapshot with `state` != empty, heartbeat within last 60s
- **Defined** (amber) — Agent has a prompt file and/or persona but no runtime snapshot exists
- **Designed** (gray) — Agent exists only in spec/ROLES.md
- **Missing** (red) — Architecture requires it but nothing exists

**Data source:** `telemetry_agent_snapshots` (latest per agent) cross-referenced with the static list of designed/defined agents.

### 3.3 Live Agent Status

Per-agent detail cards showing:

- Current FSM state (Idle, Processing, Waiting, Escalating, Refusing, Suspended, Retiring, Retired)
- Iteration count / max iterations (with percentage)
- Token usage and cost
- Trust score (nullable — may be absent for some agents)
- Last event emitted (type + timestamp)
- Last LLM output (truncated to ~500 chars) — the "last message" the user requested

With 6 agents running (guardian, sysmon, allocator, strategist, planner, implementer), the grid should accommodate growth as new agents are added in Phase 2+.

**Data source:** `telemetry_agent_snapshots` (latest per agent).

### 3.4 Hive Health

Aggregate metrics:

- Active agents / total registered actors
- Chain length and integrity status
- Event rate (events per minute)
- Daily cost vs. daily cap (with percentage)
- Overall severity (ok / warning / critical)

**Data source:** `telemetry_hive_snapshots` (latest).

### 3.5 Live Event Stream

A scrolling feed of recent events — the civilization's heartbeat. Each entry shows timestamp, actor role, event type, and a human-readable summary. With SysMon and Allocator running, expect `health.report` and `agent.budget.adjusted` events alongside `work.task.*` and `agent.state.*` events.

**Data source:** `telemetry_event_stream` (last N rows, newest first).

---

## 4. Postgres Schema

All tables live in **hive-postgres** (the hive's existing database, currently 4 tables: `events`, `event_causes`, `edges`, `actors`). These are operational/ephemeral — they get pruned, they're not part of the auditable event chain.

```sql
-- Point-in-time snapshot of every agent, written every N seconds
CREATE TABLE telemetry_agent_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent_role      TEXT NOT NULL,
    actor_id        TEXT NOT NULL,
    state           TEXT NOT NULL,           -- Idle, Processing, Waiting, etc. (8 FSM states)
    model           TEXT NOT NULL,           -- claude-haiku-4-5-20251001, claude-sonnet-4-6, etc.
    iteration       INT NOT NULL,
    max_iterations  INT NOT NULL,
    tokens_used     BIGINT NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(10,6) NOT NULL DEFAULT 0,
    trust_score     NUMERIC(4,3),            -- nullable; edges table query, may be absent
    last_event_type TEXT,                    -- last event this agent emitted
    last_message    TEXT,                    -- last LLM output (truncated ~500 chars)
    errors          INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_telemetry_agent_latest
    ON telemetry_agent_snapshots (agent_role, recorded_at DESC);

-- Hive-level health snapshots
CREATE TABLE telemetry_hive_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    active_agents   INT NOT NULL,
    total_actors    INT NOT NULL,
    chain_length    BIGINT NOT NULL,
    chain_ok        BOOLEAN NOT NULL,
    event_rate      NUMERIC(8,2),            -- events per minute
    daily_cost      NUMERIC(10,4),
    daily_cap       NUMERIC(10,4),
    severity        TEXT NOT NULL DEFAULT 'ok'
);

-- Expansion phase tracking
CREATE TABLE telemetry_phases (
    phase           INT PRIMARY KEY,
    label           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'blocked',  -- blocked, in_progress, complete
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    notes           TEXT
);

-- Recent event stream (ring buffer, pruned to last N)
CREATE TABLE telemetry_event_stream (
    id              BIGSERIAL PRIMARY KEY,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    event_type      TEXT NOT NULL,
    actor_role      TEXT NOT NULL,
    summary         TEXT,                    -- human-readable one-liner
    raw_content     JSONB
);
CREATE INDEX idx_telemetry_stream_recent
    ON telemetry_event_stream (recorded_at DESC);
```

### Table Creation Pattern

Following the existing codebase convention: no migration tool, no .sql files. Tables are created via `CREATE TABLE IF NOT EXISTS` in Go const schema strings, executed at startup. The telemetry package provides an `EnsureTables(ctx, pool)` function that follows the same `NewPostgresStoreFromPool()` / `NewPostgresActorStoreFromPool()` pattern.

### Seed Data Strategy

Phase seed data uses `INSERT ... ON CONFLICT (phase) DO UPDATE SET` — not `DO NOTHING`. This ensures restarts apply corrected seed data instead of silently preserving stale values.

The `DO UPDATE` is selective:
- **label, status, notes** — always overwritten from seed. These are structural and should track the codebase version.
- **started_at, completed_at** — use `COALESCE(existing, seed)`. If a human set a real timestamp via `POST /telemetry/phases/{phase}` (graduation ceremony), that value is preserved. If the timestamp is still null, the seed value wins.

This means seed data is authoritative for structure, but manual phase updates survive restarts.

### Seed Data for Phases

```sql
INSERT INTO telemetry_phases (phase, label, status, started_at, completed_at, notes) VALUES
    (0, 'Foundation',                    'complete',    '2026-03-01', '2026-03-15', 'Strategist, Planner, Implementer, Guardian running. 6 agents functional, 8 hive runs.'),
    (1, 'Operational infrastructure',    'complete',    '2026-03-20', '2026-04-04', 'SysMon + Allocator graduated and running. 40 health.report events confirm SysMon active.'),
    (2, 'Technical leadership',          'blocked',     NULL, NULL, 'CTO + Reviewer — no AgentDefs, no code, only ROLES.md specs.'),
    (3, 'The growth loop',               'blocked',     NULL, NULL, 'Spawner — needs Phase 2. THE UNLOCK.'),
    (4, 'Tier B emergence',              'blocked',     NULL, NULL, 'Organic via growth loop'),
    (5, 'Production deployment',         'blocked',     NULL, NULL, 'Integrator — trust-gated (>0.7)'),
    (6, 'Business operations (Tier C)',   'blocked',     NULL, NULL, 'PM, Finance, CustomerService, SRE, DevOps, Legal'),
    (7, 'Self-governance (Tier D)',       'blocked',     NULL, NULL, 'Philosopher, RoleArchitect, Harmony, Politician'),
    (8, 'Emergent civilization',          'blocked',     NULL, NULL, 'Formalize 31 emergent roles');
```

---

## 5. Telemetry Writer

### Location in Codebase

New package: `hive/pkg/telemetry/`

Files:
- `writer.go` — Core snapshot writer, event stream recorder, agent registration
- `pruner.go` — Background goroutine that prunes old data
- `types.go` — Shared types for snapshots

### Agent Registration Pattern

**Critical finding from recon:** There is no central agent registry at runtime. `Runtime.defs` is private, and after `Run()` spawns agents, they exist only inside their Loop goroutines. The telemetry writer cannot discover agents — it must be told about them.

**Implementation reality (from Prompt 2):** The writer is wired via `hive.Config.TelemetryWriter`, not as a standalone component. The Runtime:
- Calls `SetBudgetRegistry()` when the registry is created in `Run()`
- Calls `RegisterAgent()` for each agent during spawn
- Chains `RecordResponse()` into the existing `OnIteration` callback
- Starts the writer goroutine and bus subscription before `RunConcurrent()`

The `RegisterAgent()` method receives:

```go
type AgentRegistration struct {
    Name          string
    Role          string
    Model         string              // from AgentDef.Model — not available on Agent at runtime
    Agent         *hiveagent.Agent    // for State(), LastEvent()
    MaxIterations int                 // from AgentDef.MaxIterations
}
```

**No Loop pointer.** The original design called for a `*loop.Loop` reference per agent to call `Loop.LastResponse()`. This doesn't work because `RunConcurrent()` creates Loop instances as goroutine-local variables — they're never exposed. Instead, the writer provides `RecordResponse(agentName, response string)`, called from the Runtime's `OnIteration` callback which already receives the response string.

This gives the writer access to:
- `reg.Agent.State()` — current FSM state (mutex-safe getter)
- `reg.Agent.LastEvent()` — last emitted event ID
- `RecordResponse()` — last LLM output (fed by OnIteration callback)
- `reg.Model` — model string (captured at registration time)
- `reg.MaxIterations` — max iterations (captured at registration time)

### Behavior

- Runs as a goroutine started by the hive runtime at boot
- Every **10–15 seconds**, iterates all registered agents and writes one row per agent to `telemetry_agent_snapshots`
- Every **10–15 seconds**, writes one row to `telemetry_hive_snapshots` with aggregate metrics
- On every event bus emission, writes one row to `telemetry_event_stream` with a summary
- **Pruning schedule** (every hour):
  - `telemetry_agent_snapshots`: keep last 24 hours
  - `telemetry_hive_snapshots`: keep last 7 days
  - `telemetry_event_stream`: keep last 1000 rows

### Data Collection

The writer reads from runtime structs. Data sources confirmed by recon:

| Data | Source | How |
|------|--------|-----|
| Agent FSM state | `Agent.State()` | Mutex-safe getter on registered Agent reference |
| Agent iterations | `BudgetRegistry.Snapshot()` | `BudgetEntry.Budget.Snapshot().Iterations` — NOT on Agent struct |
| Agent model | `AgentRegistration.Model` | Captured at registration time from AgentDef |
| Token usage | `BudgetRegistry.Snapshot()` | `BudgetEntry.Budget.Snapshot().TokensUsed` |
| Cost | `BudgetRegistry.Snapshot()` | `BudgetEntry.Budget.Snapshot().CostUSD` |
| Agent state (Active/Quiesced/Stopped) | `BudgetRegistry.Snapshot()` | `BudgetEntry.AgentState` |
| Max iterations | `AgentRegistration.MaxIterations` | Captured at registration time |
| Trust score | **nil for v1** | TODO: `SELECT weight FROM edges WHERE to_actor = $1 AND edge_type = 'trust'` |
| Last LLM output | `Writer.RecordResponse()` | Fed by Runtime's OnIteration callback (Loop pointer not accessible) |
| Last event | `Agent.LastEvent()` | Mutex-safe getter, returns `types.EventID` |
| Chain length | `store.Count()` | Direct method on Store — returns `(int, error)` |
| Chain integrity | `store.VerifyChain()` | **Cached every 5 minutes** — full chain walk too expensive for 10s interval |
| Event stream | `bus.Subscribe("*", handler)` | Goroutine-safe, dedicated goroutine per subscriber, buffered channel |
| Active agent count | `BudgetRegistry.Snapshot()` | Count entries with `AgentState == "Active"` |
| Daily cost aggregate | `BudgetRegistry.TotalUsed()` | Returns aggregate across all agents |
| Event rate | **nil for v1** | TODO: compute from telemetry_event_stream timestamps |
| Daily cap | **nil for v1** | TODO: wire to configuration or budget system |

### Last Response Capture

**Two-path approach (from Prompt 2 implementation):**

1. **`lastResponse` field on Loop** — Added to `pkg/loop/loop.go` with a public `LastResponse()` getter, truncated to 500 chars, protected by the existing mutex. Useful if anyone has a direct Loop reference.

2. **`Writer.RecordResponse(agentName, response)`** — The primary path. Called from the Runtime's `OnIteration` callback which already receives the response string. This was necessary because `RunConcurrent()` creates Loop instances as goroutine-local variables — they're never exposed to external code. The Runtime chains the telemetry recording into the existing callback pipeline.

---

## 6. Telemetry API

### Process Boundary

The telemetry API runs in the **work-server** (`work/cmd/work-server/`), a separate process from the hive. It reads from hive-postgres, which is the same database the work-server already uses for task data. No new database connection needed — the existing `DATABASE_URL` / pool serves both task and telemetry queries.

### Endpoints

```
GET  /telemetry/status         → Full status snapshot (latest agent + hive + phases)
GET  /telemetry/agents         → Latest snapshot per agent with last_message
GET  /telemetry/agents/{role}  → Single agent detail with recent history
GET  /telemetry/stream         → Last N events (default 50, max 200)
GET  /telemetry/phases         → Current expansion phase status
GET  /telemetry/health         → Latest hive health snapshot
POST /telemetry/phases/{phase} → Update phase status (graduation ceremonies)
```

Note: The dashboard is NOT served by the work-server. It lives in `summary` as a standalone HTML file that polls these endpoints.

### Auth

Same pattern as work-server: `Authorization: Bearer <key>`. The dashboard HTML page gets the key injected at serve time (same `{{API_KEY}}` replacement pattern).

### CORS

Already handled by work-server middleware (`Access-Control-Allow-Origin: *`).

### Response Shape: `GET /telemetry/status`

```json
{
  "timestamp": "2026-04-04T14:32:00Z",
  "hive": {
    "active_agents": 6,
    "total_actors": 7,
    "chain_length": 1008,
    "chain_ok": true,
    "event_rate": 23.0,
    "daily_cost": 0.42,
    "daily_cap": 5.00,
    "severity": "ok"
  },
  "agents": [
    {
      "role": "guardian",
      "actor_id": "actor_00d5d8ac...",
      "state": "Idle",
      "model": "claude-sonnet-4-6",
      "iteration": 30,
      "max_iterations": 200,
      "tokens_used": 12450,
      "cost_usd": 0.089,
      "trust_score": null,
      "last_event_type": "hive.integrity.verified",
      "last_event_at": "2026-04-04T14:31:42Z",
      "last_message": "Chain integrity verified. 6 agents active. No anomalies detected...",
      "errors": 0
    },
    {
      "role": "sysmon",
      "actor_id": "actor_a1b2c3d4...",
      "state": "Processing",
      "model": "claude-haiku-4-5-20251001",
      "iteration": 45,
      "max_iterations": 150,
      "tokens_used": 8320,
      "cost_usd": 0.012,
      "trust_score": null,
      "last_event_type": "health.report",
      "last_event_at": "2026-04-04T14:31:55Z",
      "last_message": "/health {\"severity\":\"ok\",\"chain_ok\":true,\"active_agents\":6,\"event_rate\":23.0}",
      "errors": 0
    },
    {
      "role": "allocator",
      "actor_id": "actor_e5f6a7b8...",
      "state": "Idle",
      "model": "claude-haiku-4-5-20251001",
      "iteration": 22,
      "max_iterations": 150,
      "tokens_used": 5140,
      "cost_usd": 0.008,
      "trust_score": null,
      "last_event_type": "agent.state.changed",
      "last_event_at": "2026-04-04T14:30:10Z",
      "last_message": "All budgets within normal parameters. No adjustments needed this cycle...",
      "errors": 0
    }
  ],
  "phases": [
    {
      "phase": 0,
      "label": "Foundation",
      "status": "complete",
      "started_at": "2026-03-01T00:00:00Z",
      "completed_at": "2026-03-15T00:00:00Z"
    },
    {
      "phase": 1,
      "label": "Operational infrastructure",
      "status": "complete",
      "started_at": "2026-03-20T00:00:00Z",
      "completed_at": "2026-04-04T00:00:00Z"
    },
    {
      "phase": 2,
      "label": "Technical leadership",
      "status": "blocked",
      "started_at": null,
      "completed_at": null
    }
  ],
  "recent_events": [
    {
      "event_type": "health.report",
      "actor_role": "sysmon",
      "summary": "Health OK: 6 agents active, chain intact",
      "at": "2026-04-04T14:31:55Z"
    },
    {
      "event_type": "agent.state.changed",
      "actor_role": "implementer",
      "summary": "State: Processing → Idle",
      "at": "2026-04-04T14:31:12Z"
    }
  ]
}
```

---

## 7. Dashboard

### Repository

_Hosting TBD — the previously-planned `summary` repo is retired._

### Serving

The dashboard is a **standalone HTML file** — no server-side rendering, no build step, no framework. It can be served from:
- A simple file server on nucbuntu (e.g., `python3 -m http.server`)
- GitHub Pages at `transpara-ai.github.io/summary/dashboard.html`
- Opened directly as a local file in a browser (with CORS caveats)
- Any static hosting

### Configuration via URL Parameters

Since the dashboard is not served by the work-server, it cannot use the `{{API_KEY}}` injection pattern. Instead, configuration is passed via URL parameters:

```
dashboard.html?api=http://nucbuntu:8080&key=YOUR_API_KEY
```

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `api` | Yes | — | Work-server base URL (e.g., `http://nucbuntu:8080`) |
| `key` | Yes | — | Bearer token for API authentication |

The dashboard reads these from `URLSearchParams` on load. If either is missing, it shows a configuration prompt instead of the dashboard.

### Tech

Single HTML file. Vanilla JS. Polls `GET {api}/telemetry/status` every 10 seconds. CSS variables for dark mode (default).

### Visual Design

Based on the architecture poster already in the repo, but driven by live data:

- **Phase timeline:** Green dots for complete, blue for in_progress, gray for blocked. Timestamps shown.
- **Agent grid:** Each chip shows live state dot. Click to expand and see last message, iterations, cost, trust. Grid accommodates 6+ agents today, more as phases advance.
- **Hive health:** Summary cards — active agents, chain status, daily cost vs. cap, severity.
- **Event stream:** Scrolling list at the bottom, auto-updating. Newest first. Shows actor role, event type, summary, relative timestamp. Event type badges visually distinguish `health.report`, `agent.budget.adjusted`, `work.task.*`, and `agent.state.*` categories.
- **Connection indicator:** Green pulsing dot when polling succeeds. Gray with "Last update: Xs ago" when stale. Red when polling fails.

### Offline Behavior

When the hive is not running, the dashboard shows the last known state with a stale indicator. The postgres tables retain data, so the API still returns data — it just stops updating. No fake green lights.

---

## 8. Implementation Sequence

Detailed Claude Code prompts are in the companion document: `telemetry-claude-code-prompts-v0.4.1.md`. The summary below outlines each step.

### Prompt 0: Reconnaissance — COMPLETE

14 areas investigated. Key findings: no agent registry (RegisterAgent pattern needed), LastResponse absent (insertion point identified at loop.go:232), iterations from BudgetRegistry not Agent, model string from AgentDef at registration, trust score nullable for v1, process boundary confirmed (writer in hive, API in work-server), 18 event types across 1,008 events in live DB.

### Prompt 1: Schema + Types

- Create `EnsureTables(ctx, pool)` in `pkg/telemetry/` following the `CREATE TABLE IF NOT EXISTS` const schema pattern
- Create `pkg/telemetry/types.go` with Go structs matching the schema
- Seed the `telemetry_phases` table with Phases 0–8 (Phase 0+1 complete)
- Unit tests for type serialization

### Prompt 2: Telemetry Writer + LastResponse

- Create `pkg/telemetry/writer.go` — snapshot goroutine with `RegisterAgent()` pattern
- Create `pkg/telemetry/pruner.go` — cleanup goroutine
- Add `lastResponse` field to Loop struct with public `LastResponse()` getter
- Wire agent registration into `Runtime.Run()` at agent spawn (~runtime.go:164-177)
- Wire `bus.Subscribe("*", ...)` for event stream capture
- Wire writer + pruner startup into hive boot, respecting ctx cancellation
- Unit tests for snapshot collection, registration, and pruning logic

### Prompt 3: Telemetry API (in work)

- Add `/telemetry/*` endpoints to `cmd/work-server/main.go`
- Implement the JSON response shapes from Section 6
- Reads from hive-postgres via the existing pool (same DATABASE_URL)
- Graceful handling of missing telemetry tables and empty data
- Test endpoints manually with curl

### Prompt 4: Dashboard (in summary)

- Create the mission control dashboard as a standalone HTML file in `summary`
- Configuration via URL parameters (`?api=...&key=...`) — no server-side injection
- Five-section layout: connection indicator, phase timeline, agent grid (6 agents), hive health, event stream
- Polling loop with connection indicator
- Agent detail expansion (click chip → see last message)
- Event stream with auto-scroll and event type badges

### Prompt 5: Integration + Polish

- End-to-end test: hive running → telemetry writing → work-server reading → dashboard rendering
- Verify pruning works on schedule
- Add `POST /telemetry/phases/{phase}` for graduation ceremony updates
- Docker Compose integration (ensure telemetry tables are created on hive-postgres startup)
- Documentation

---

## 9. Existing Infrastructure Context (Post-Recon)

### Network Topology

- **nucbuntu:** Internal server at `palmela.transpara.com` subnet
- **Tailscale:** All dev machines connected, nucbuntu reachable
- **No public internet exposure** for the Transpara AI hive
- **GitHub Pages** (`transpara-ai.github.io/summary`) hosts the static architecture poster — completely separate from this system

### Docker Compose (Confirmed)

| Service | Image | Ports | Status |
|---------|-------|-------|--------|
| postgres | postgres:16-alpine | 5432:5432 | Up 4 days (healthy) |
| pgadmin | dpage/pgadmin4:latest | 8080:80 | Up 4 days |

Container names: `hive-postgres-1`, `site-postgres-1` (separate).
DSN: `postgres://hive:hive@localhost:5432/hive`

### Hive Runtime on nucbuntu

- Docker Compose deployment
- Hive-postgres: 4 existing tables (`events`, `event_causes`, `edges`, `actors`) — 1,008 events confirmed
- Work-server: REST API with CORS, 18 routes, 2 inline HTML dashboards, auth via Bearer token (port 8080)
- Legacy mode: **6 concurrent agents** (guardian, sysmon, allocator, strategist, planner, implementer)
- Pipeline mode: 8-phase sequential (scout, architect, builder, tester, critic, reflector, observer, PM)
- Single `pgxpool.Pool` created in `cmd/hive/main.go:725`, shared via `FromPool()` constructors

### Agent Roster (Confirmed from StarterAgents())

| # | Name | Role | Model | CanOperate | MaxIter | WatchPatterns |
|---|------|------|-------|------------|---------|---------------|
| 1 | guardian | guardian | Sonnet | false | 200 | `[]` (= `*` all) |
| 2 | sysmon | sysmon | Haiku | false | 150 | `hive.*, budget.*, health.*, agent.state.*, agent.escalated, trust.*` |
| 3 | allocator | allocator | Haiku | false | 150 | `health.report, agent.budget.*, hive.*, agent.state.*` |
| 4 | strategist | strategist | Opus | false | 50 | `work.task.completed, hive.*` |
| 5 | planner | planner | Opus | false | 50 | `work.task.created` |
| 6 | implementer | implementer | Opus | true | 100 | `work.task.created, work.task.assigned` |

### Event Type Vocabulary (18 types, 1,008 events in live DB)

| Event Type | Count | Source |
|-----------|-------|--------|
| agent.state.changed | 499 | All agents (FSM transitions) |
| agent.evaluated | 204 | Loop evaluation cycles |
| agent.model.bound | 41 | Bootstrap |
| hive.agent.spawned | 41 | Runtime spawning |
| agent.soul.imprinted | 41 | Bootstrap |
| agent.authority.granted | 41 | Bootstrap |
| health.report | 40 | SysMon |
| hive.agent.stopped | 19 | Runtime shutdown |
| work.task.created | 15 | Strategist/Planner |
| work.task.assigned | 15 | Implementer |
| work.task.completed | 15 | Implementer |
| agent.acted | 11 | Loop actions |
| hive.run.started | 8 | Runtime |
| work.task.comment | 5 | Various |
| agent.learned | 4 | Reflection |
| hive.run.completed | 4 | Runtime |
| work.task.dependency.added | 4 | Planner |
| system.bootstrapped | 1 | First boot |

**Notable:** 0 `agent.budget.adjusted` events in DB — Allocator hasn't triggered adjustments in production yet (likely staying within stabilization window).

### What Already Exists That This Builds On

| Component | Status | Relevance |
|-----------|--------|-----------|
| `resources.BudgetSnapshot` | Working | TokensUsed, CostUSD, Iterations, Elapsed |
| `resources.BudgetRegistry` | Working | Cross-agent visibility: Snapshot() → []BudgetEntry, TotalUsed(), AdjustMaxIterations() |
| `agent.Agent` with State(), LastEvent() | Working | FSM state (mutex-safe), last event ID |
| Event bus (`bus.Subscribe("*", handler)`) | Working | Goroutine-safe, dedicated goroutine per subscriber, buffered channel, panic recovery |
| `store.Count()` | Working | Direct event count — SELECT COUNT |
| `store.VerifyChain()` | Working | Returns `ChainVerifiedContent{Valid, Length, Duration}` |
| Work-server with CORS + dashboard pattern | Working | 2 inline HTML dashboards, `{{API_KEY}}` injection, 10s polling |
| `pkg/health/` (SysMon) | **Graduated** | Types, thresholds, monitor logic (6 files) |
| `pkg/loop/health.go` | **Implemented** | parseHealthCommand, emitHealthReport, enrichHealthObservation (sysmon only) |
| `pkg/loop/budget.go` | **Implemented** | parseBudgetCommand, validateBudgetCommand, applyBudgetAdjustment, enrichBudgetObservation (allocator only) |
| `pkg/budget/` | **Implemented** | Config, types, monitor (6 files) — Allocator infrastructure |
| SysMon agent | **Running** | 40 health.report events in live DB |
| Allocator agent | **Running** | Wired in StarterAgents, budget monitoring active |

### What Does NOT Exist Yet

| Component | Repo | Needed For | Status |
|-----------|------|-----------|--------|
| `telemetry_*` tables in hive-postgres | hive | All telemetry storage | **Done (Prompt 1)** — EnsureTables() |
| `pkg/telemetry/` package | hive | Writer, pruner, types | **Done (Prompts 1+2)** |
| `lastResponse` field on Loop | hive | Last-message capture | **Done (Prompt 2)** — field + getter + OnIteration wiring |
| `RegisterAgent()` call in Runtime | hive | Agent registration for telemetry | **Done (Prompt 2)** — via Config.TelemetryWriter |
| `/telemetry/*` HTTP endpoints | work | Dashboard data source | **Done (Prompt 3)** |
| `POST /telemetry/phases/{phase}` | work | Graduation ceremony updates | Prompt 5 |
| Dashboard HTML | summary | The actual UI (standalone, URL-param config) | **Done (Prompt 4)** |

---

## 10. Design Principles

1. **The flight data recorder survives the crash.** Telemetry is pure Go. No LLM dependency. No token budget. If every agent is broken, telemetry still writes snapshots.

2. **Ephemeral, not auditable.** Telemetry tables are operational data that gets pruned. The event chain is the permanent record. Don't conflate them.

3. **Read existing state, don't create new state.** The telemetry writer reads from runtime structs that already exist. The only new field is `lastResponse` on the Loop (added in Prompt 2, fed via OnIteration callback). Everything else is already in memory via Agent, BudgetRegistry, and Store.

4. **Honest about staleness.** When the hive is offline, the dashboard shows the last known state with a clear timestamp. No fake green lights. No estimated data. Stale data is honest data.

5. **Phase transitions are human judgments (for now).** A `POST /telemetry/phases/2` with `{"status":"in_progress"}` when CTO work starts is fine. Machine-checkable graduation criteria can automate this later.

6. **Same patterns, same codebase.** The telemetry API follows the same patterns as the work-server (CORS, bearer auth, HTML dashboard with injected API key, inline const string). No new frameworks. No new dependencies.

7. **Reuse before reinventing.** BudgetRegistry already provides cross-agent visibility for iterations, cost, and agent state. The telemetry writer reads from it rather than reimplementing per-agent tracking.

8. **Explicit registration, not discovery.** Agents register with the telemetry writer during spawn via `hive.Config.TelemetryWriter`. Responses are captured via the `OnIteration` callback pipeline. No runtime reflection, no registry lookup, no exposed Loop pointers.

---

## 11. What Comes After

Once the telemetry system is running:

- SysMon's `health.report` events can be cross-referenced with telemetry snapshots for richer diagnostics
- The dashboard becomes the primary way to observe hive operations during development
- Phase transitions can be partially automated as graduation criteria become machine-checkable
- Historical telemetry data enables trend analysis (agent efficiency, cost per task, trust accumulation rates)
- The CTO agent (Phase 2, next after telemetry) will have immediate visibility into hive operations from day one via the dashboard
- Trust score population: once the trust model is more actively used, the edges-table query can be wired in to replace the current nullable approach
- The static GitHub Pages poster can optionally be updated with a "last known state" badge via a GitHub Action that reads the telemetry API — but this is polish, not priority

---

*This document captures the design state as of 2026-04-04. Prompts 0–4 complete. Only Prompt 5 (integration, phase updates, polish) remains. Companion: telemetry-claude-code-prompts-v0.4.1.md.*
