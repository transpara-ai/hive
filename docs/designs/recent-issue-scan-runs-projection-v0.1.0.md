# Recent Issue-Scan Runs Projection — Design Packet

- **doc_id:** HIVE-RECENT-ISSUE-SCAN-RUNS-DESIGN-001
- **version:** v0.2.0 (CFADA round 1 resolved)
- **status:** CFADA round 2
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
| `parked` / `human_action` | `hive.issuescan.run.parked` content — via a SHARED normalized parked-run helper feeding BOTH the board fold and the rail (one record: state, issue ref, blocker fields, refs, link key — CFADA1-adv4) |
| `queued` | an issue-scan `factory.run.requested` event (identified by the issue-scan brief/lifecycle predicate — the same predicate the queued-lifecycle fold uses; generic factory runs are EXCLUDED entirely, they never even reach `recorded` — CFADA1-adv1) with no parked event for the run and no work evidence joined |
| `in_flight` | queued proof + the run's `FactoryOrderID` resolves to a factory-order summary ALREADY built by the operator projection AND that summary carries ≥1 work-task evidence record. If the order summary is absent (limit/truncation), the run stays `queued` — under-claiming is the safe direction |
| `recorded` | fallback for a run whose existence is proven (issue-scan predicate matched) but whose lifecycle position is not — honest catch-all; NEVER a healthy-looking default |

**`ready_for_human` is DROPPED from v1 (CFADA1-2).** Hive's own stage-evidence code states stage evidence is not PR-readiness or human-approval proof (`pkg/hive/issue_scan_stage_runtime_evidence.go:18-20,63-67`), and the all-stages-complete check is runtime logic, not a projection fold output. Approximating it from visible final-stage work evidence could paint an incomplete run healthy. Runs in that position surface as `in_flight`. A future packet may add the state when the projection folds an explicit, non-truncated all-stage completion proof.

**Evidence sourcing (CFADA1-1 — corrected reuse claim):** the operator projection exposes only `LastQueuedRunRequest` (`operator_projection.go:679-719`) — the queued fold is NOT reusable for a multi-run rail. The rail fold is therefore authorized exactly ONE new store query: a `ByType(EventTypeFactoryRunRequested, limit, …)` page, filtered by the issue-scan predicate. The parked page and the factory-order/work evidence are reused from the builder's existing outputs. Total new store round-trips: one.

Precedence when evidence conflicts: parked/human_action > in_flight > queued > recorded. Dedupe by `run_id` (CFADA1-3):
- runs whose `run_id` is blank after TrimSpace are EXCLUDED from the rail (cannot be deduped or linked; the board's own handling of such events is unchanged);
- multiple parked events for one run → the record with the LATEST event timestamp wins whole (no field-mixing between events); `source_refs` are the union;
- parked vs queued for one run → parked wins whole, refs unioned.

### D2 — Latency: one new query, absolute fold budget, and a consumer-side timeout fix (CFADA1-4)

The endpoint measured 5.2s live against the site's 5s intake client timeout — the surface already flaps to honest-unavailable TODAY, and no hive-side delta test alone can fix that. Two-part resolution:

- **Hive:** the rail fold adds exactly ONE store query (the `factory.run.requested` page, D1) and otherwise consumes already-fetched pages/folds. Timed regression test on a seeded store asserts the builder's added wall-clock from the new fold is < 250ms absolute AND < 10% relative — a hard budget, not just "no order-of-magnitude regression".
- **Site (companion half, site#204):** the civilization-projection HTTP client timeout is raised from 5s to **9s** — above the measured 5.2s with headroom, deliberately BELOW the intake surface's 10s poll interval so polls can never overlap. This is a consumer-resilience fix that stands on its own (the intake board benefits immediately) and is required for the rail to be usable at all. The console's honest-unavailable behavior on timeout is unchanged.

### D3 — Timestamps and ordering

`first_event_at`/`last_event_at` from the folded events' store timestamps (UTC RFC3339). Single-event runs: first == last. Order runs by `last_event_at` descending. `truncated: true` when any contributing page hit `limit` (mirror the existing truncation-signal pattern). Unparseable/zero timestamps: the run still lists, with the field omitted — the site treats missing timestamps as unageable, not as now (IADA-4).

### D4 — Schema versioning

`civilizationAssemblyProjectionSchemaVersion` 1.5.0 → 1.6.0. Additive + `omitempty`: a 1.5.0 consumer sees no change; the site (v1.x check) accepts 1.6.0. Site MUST render byte-identically against a 1.5.0 payload (regression-tested on the site side).

### D5 — Site rail (consumer half; site#204)

- View-model derivation in `buildConsoleIssueScan`: rail data only when the surface freshness is one of the EXISTING usable states — `current`, `stale`, or `partial`, exactly the set that renders the board (CFADA1-adv2: the rail and board share one freshness decision; they can never diverge) — AND the section's own `status == "available"` (allowlist); absent field / unknown status / unavailable → no rail, board unchanged.
- Civilization-projection client timeout 5s → 9s (D2).
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

## 6. CFADA record

### Round 1 (codex, 2026-07-02, via its CFADA governance skill) — VERDICT: BLOCKERS (4) → all resolved in v0.2.0

- **CFADA1-1 (false reuse claim):** the queued-run fold exposes only `LastQueuedRunRequest`; a multi-run rail cannot reuse it. Resolved: exactly one new authorized store query (`factory.run.requested` page with the issue-scan predicate); parked page + factory-order/work evidence reused.
- **CFADA1-2 (`ready_for_human` unprovable):** stage evidence is explicitly not PR-readiness proof, and the all-stage completion check is runtime logic, not a fold output. Resolved: state DROPPED from v1; such runs surface as `in_flight`.
- **CFADA1-3 (dedupe underspecified):** multiple parked events per run and blank run_ids had no contract. Resolved: latest-parked-event-wins-whole (no field mixing), refs unioned; blank run_id excluded from the rail.
- **CFADA1-4 (latency budget insufficient):** the endpoint already exceeds the site's 5s client timeout. Resolved: hard hive fold budget (<250ms absolute, <10% relative, timed test) PLUS site civilization-client timeout 5s → 9s (bounded under the 10s poll to prevent overlap).
- Advisories adopted: issue-scan predicate excludes generic factory runs entirely (adv1); rail freshness bound to the board's exact usable-state set (adv2); 1.5.0 byte-compat kept (adv3); shared normalized parked-run helper feeding board + rail (adv4).

## 7. IADA record (v0.1.0, 2026-07-02)

- **IADA-1 (unprovable ready state):** if final-stage completion cannot be proven from existing folds without new queries, the state is dropped from v1 rather than approximated — scope reduction over guessing.
- **IADA-2 (conflicting evidence):** same run in parked page and queued fold → explicit precedence + dedupe by run_id; without this, the rail could show one run twice in two states.
- **IADA-3 (latency budget):** the endpoint is borderline against the site's 5s client timeout TODAY (measured 5.2s live); the fold reuses fetched pages, permits at most one new query, and carries a timed regression test.
- **IADA-4 (timestamp honesty):** zero/unparseable event timestamps must not default to `now` (would fake recency); fields omitted, site renders unageable.
- **IADA-5 (dead links):** rail entries link to drawers only for (run_id, stage_id) pairs present on the rendered board; a link to a non-existent drawer target would render an honest not-found drawer but still be a fabricated affordance.
