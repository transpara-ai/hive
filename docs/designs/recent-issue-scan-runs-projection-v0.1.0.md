# Recent Issue-Scan Runs Projection — Design Packet

- **doc_id:** HIVE-RECENT-ISSUE-SCAN-RUNS-DESIGN-001
- **version:** v0.1.0
- **status:** IADA applied → CFADA
- **issues:** https://github.com/transpara-ai/hive/issues/240 (this repo) · https://github.com/transpara-ai/site/issues/204 (consumer)
- **base:** hive main @ 02ae3d4; site consumer stacks on site PR #203
- **scope:** projection-only fold in `pkg/hive/civilization_assembly_projection.go` + hive-ops-api serving; site renders a rail. NO new event types, NO writes, NO scanner/lifecycle behavior change.

## 1. Problem

The civilization assembly projection's issue-scan section folds only `hive.issuescan.run.parked` events (`civilization_assembly_projection.go` ~530). Runs that were queued, are in flight, or finished never reach the Intake surface — the operator sees only what is stuck. The events for the full lifecycle already exist (`factory.run.requested`, factory-order/work-task evidence, parked events); the projection simply drops them.

## 2. Decisions

### D1 — Projection-only; states are proven, never guessed

New top-level section `recent_issue_scan_runs` (contract in hive#240, restated in §3). Fail-closed state allowlist, each state bound to explicit event evidence:

| state | proof required |
|---|---|
| `parked` / `human_action` | `hive.issuescan.run.parked` content — EXACTLY the mapping the existing board fold uses (shared helper, not a re-derivation) |
| `queued` | issue-scan `factory.run.requested` present AND no parked event for the run AND no work evidence joined |
| `in_flight` | queued proof + the run's factory order has work-task evidence in the folds the builder ALREADY computes (join by FactoryOrderID; no new queries) |
| `ready_for_human` | final-stage completion evidence already present in the builder's existing folds — **implementer must verify this is provable without new store queries; if not provable, DROP this state from v1** and let such runs surface as `in_flight`/`recorded`; scope reduction is the fail-safe direction (IADA-1) |
| `recorded` | fallback for a run whose existence is proven but whose lifecycle position is not — honest catch-all; NEVER a healthy-looking default |

Precedence when evidence conflicts: parked/human_action > ready_for_human > in_flight > queued > recorded (a terminal parked fact beats an older queued fact). Dedupe by `run_id`, keeping the highest-precedence state and merging source_refs (IADA-2).

### D2 — Reuse fetched pages; at most one new query

The endpoint already costs ~5s live. The fold MUST reuse: the parked-events page (already fetched), the queued-run fold (`factory.run.requested`, already fetched for QueuedRunRequest), and the factory-order/work evidence (already folded). If `ready_for_human` needs one additional `ByType` page, that is the single permitted new query and must be measured (IADA-3: a timed test comparing builder duration with/without the new fold on a seeded store; assert the delta is bounded — no order-of-magnitude regression).

### D3 — Timestamps and ordering

`first_event_at`/`last_event_at` from the folded events' store timestamps (UTC RFC3339). Single-event runs: first == last. Order runs by `last_event_at` descending. `truncated: true` when any contributing page hit `limit` (mirror the existing truncation-signal pattern). Unparseable/zero timestamps: the run still lists, with the field omitted — the site treats missing timestamps as unageable, not as now (IADA-4).

### D4 — Schema versioning

`civilizationAssemblyProjectionSchemaVersion` 1.5.0 → 1.6.0. Additive + `omitempty`: a 1.5.0 consumer sees no change; the site (v1.x check) accepts 1.6.0. Site MUST render byte-identically against a 1.5.0 payload (regression-tested on the site side).

### D5 — Site rail (consumer half; site#204)

- View-model derivation in `buildConsoleIssueScan`: rail data only when surface freshness usable AND section `status == "available"` (allowlist); absent field / unknown status / unavailable → no rail, board unchanged.
- Rail renders inside the polled intake fragment (refreshes with the board; inherits the B1 drawer-reset semantics untouched).
- State→style map is an allowlist (amber for parked/human_action, emerald for ready_for_human, neutral for queued/in_flight/recorded); unknown state values render escaped text with neutral style — `default` is neutral, never a healthy color.
- Drawer links ONLY for runs whose (run_id, stage_id) exists on the rendered board (site-side index at build time); everything else unlinked. No dead links, no fabricated targets (IADA-5).
- Relative age from `last_event_at`; unparseable → age omitted.

## 3. Contract (source of truth)

As specified in hive#240: top-level `recent_issue_scan_runs: {status: "available"|"unavailable", summary, truncated, runs: [{run_id, factory_order_id?, repo, issue_number, issue_url?, issue_title?, state, first_event_at?, last_event_at?, blocker_type?, required_action?, stage_id?, source_refs}]}`. States: the D1 allowlist. Empty store or fold failure → `status: "unavailable"` + honest summary + zero runs.

## 4. Non-goals

- No new event emission; no scanner changes; no single-issue enqueue.
- No caching/latency rework of hive-ops-api (B3 covers work-server; hive-ops-api latency work is out of scope).
- No governed writes on the site; no changes to the board section's existing shape.

## 5. TDD plan

Hive: state-domain table test (every D1 state + recorded fallback + precedence conflicts + dedupe + blank run_id skip); consistency test (parked run appears in board AND rail with identical state/ref via shared helper); truncation test; empty-store unavailable test; handler test (hive-ops-api serves the section); timed fold-delta test (D2). Site: 1.5.0 byte-compat regression; rail rendering (states, order, truncation marker); link-only-when-on-board test; hostile-field escaping incl. unknown state neutral-styling; read-only guard extension.

## 6. IADA record (v0.1.0, 2026-07-02)

- **IADA-1 (unprovable ready state):** if final-stage completion cannot be proven from existing folds without new queries, the state is dropped from v1 rather than approximated — scope reduction over guessing.
- **IADA-2 (conflicting evidence):** same run in parked page and queued fold → explicit precedence + dedupe by run_id; without this, the rail could show one run twice in two states.
- **IADA-3 (latency budget):** the endpoint is borderline against the site's 5s client timeout TODAY (measured 5.2s live); the fold reuses fetched pages, permits at most one new query, and carries a timed regression test.
- **IADA-4 (timestamp honesty):** zero/unparseable event timestamps must not default to `now` (would fake recency); fields omitted, site renders unageable.
- **IADA-5 (dead links):** rail entries link to drawers only for (run_id, stage_id) pairs present on the rendered board; a link to a non-existent drawer target would render an honest not-found drawer but still be a fabricated affordance.
