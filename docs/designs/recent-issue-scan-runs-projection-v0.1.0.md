# Recent Issue-Scan Runs Projection — Design Packet

- **doc_id:** HIVE-RECENT-ISSUE-SCAN-RUNS-DESIGN-001
- **version:** v0.5.0 (CFADA rounds 1-3 + CFAR round 1 resolved)
- **status:** post-CFAR round 1 (PR #241)
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
| `queued` | an issue-scan `factory.run.requested` event — identified by **`isIssueScanRunLaunch(content)`**, the exact kind predicate hive dispatch itself uses (CFADA2-2; NOT the `queuedRunLifecycleFromBrief` lifecycle parser, whose legacy behavior differs); generic factory runs are EXCLUDED entirely, they never even reach `recorded` — with no parked event for the run and no work evidence joined, AND the parked page was NOT truncated (see truncation rule below) |
| `in_flight` | queued proof + the run's factory order — derived via **`factoryOrderIDForRunLaunch(run_id)`** and joined against the operator projection's existing `FactoryOrderSummary`/work-evidence outputs (CFADA2-adv2) — carries ≥1 work-task evidence record. If the order summary is absent (limit/truncation), the run stays `queued` — under-claiming is the safe direction |
| `recorded` | fallback for a run whose existence is proven (issue-scan predicate matched) but whose lifecycle position is not — honest catch-all; NEVER a healthy-looking default |

**`ready_for_human` is DROPPED from v1 (CFADA1-2).** Hive's own stage-evidence code states stage evidence is not PR-readiness or human-approval proof (`pkg/hive/issue_scan_stage_runtime_evidence.go:18-20,63-67`), and the all-stages-complete check is runtime logic, not a projection fold output. Approximating it from visible final-stage work evidence could paint an incomplete run healthy. Runs in that position surface as `in_flight`. A future packet may add the state when the projection folds an explicit, non-truncated all-stage completion proof.

**Evidence sourcing (CFADA1-1 — corrected reuse claim):** the operator projection exposes only `LastQueuedRunRequest` (`operator_projection.go:679-719`) — the queued fold is NOT reusable for a multi-run rail. The rail fold is therefore authorized exactly ONE new store query: a `ByType(EventTypeFactoryRunRequested, limit, …)` page, filtered by the issue-scan predicate. The parked page and the factory-order/work evidence are reused from the builder's existing outputs. Total new store round-trips: one.

Precedence when evidence conflicts: parked/human_action > in_flight > queued > recorded. Dedupe by `run_id` (CFADA1-3):
- runs whose `run_id` is blank after TrimSpace are EXCLUDED from the rail (cannot be deduped or linked; the board's own handling of such events is unchanged);
- multiple parked events for one run → the record with the LATEST event timestamp wins whole (no field-mixing between events); ties and zero timestamps break deterministically by lexicographically greater event ID (CFADA2-adv3); `source_refs` are the union;
- parked vs queued for one run → parked wins whole, refs unioned.

**Truncation × precedence rule (CFADA2-3):** "no parked event for this run" is only provable when the parked page did NOT hit its limit. When the parked page IS truncated, every requested-run not matched to a parked event degrades to `recorded` (its parked-absence is unproven — it must not claim `queued`/`in_flight`), and `truncated: true` is set. When the requested-run page is truncated, `truncated: true` is set and older runs are simply absent (absence from the rail is honest; a wrong state is not).

### D2 — Latency: one new query, absolute fold budget, singleflight, and a consumer timeout with corrected premise (CFADA1-4, CFADA2-4)

Corrected premise (CFADA2-adv1): the site's `civilizationOpsProjectionClient` timeout is **8s** today (`graph/ops.go:1119`), not 5s; the endpoint measures 5.2s solo. The live flapping comes from CONCURRENCY: with site PR #203, an open drawer self-refreshes every 10s in addition to the board's 10s poll — two overlapping full-projection computations contend in Postgres and push each other past 8s. Three-part resolution:

- **Hive fold budget:** the rail fold adds exactly ONE store query (the `factory.run.requested` page, D1) and otherwise consumes already-fetched pages/folds. Timed regression test on a seeded store asserts the pure fold's wall-clock is < 250ms absolute. (Build-phase amendment: the original additional '< 10% relative' criterion was falsified empirically — in-memory test stores eliminate the I/O denominator that dominates the production builder, making a relative percentage structurally meaningless in tests; the absolute bound against the 8-9s endpoint budget is the honest guard. Observation for the record: isIssueScanRunLaunch fully parses each ~19KB brief JSON per requested event — pre-existing predicate cost, correctly reused per D1, potential future optimization target.)
- **Hive singleflight (CFADA2-4):** hive-ops-api wraps the two projection endpoints in `golang.org/x/sync/singleflight` (already in the module graph) keyed by endpoint — concurrent identical requests share ONE fresh computation. This is honesty-preserving (every response is a real computation with its true `generated_at`; sharers receive the same fresh result — no TTL cache, no staleness introduced) and collapses the board+drawer concurrent-duplicate load to a single upstream computation.
- **Site client timeout 8s → 9s** for the civilization client only: headroom above the 5.2s solo measurement, deliberately below every poller's own 10s interval so no poller can self-overlap. With singleflight upstream, concurrent pollers no longer multiply computation, so 9s is adequate. Honest-unavailable behavior on timeout unchanged.

### D3 — Timestamps and ordering

`first_event_at`/`last_event_at` from the folded events' store timestamps (UTC RFC3339). Single-event runs: first == last. Order runs by `last_event_at` descending. `truncated: true` when any contributing page hit `limit` (mirror the existing truncation-signal pattern). Unparseable/zero timestamps: the run still lists, with the field omitted — the site treats missing timestamps as unageable, not as now (IADA-4).

### D4 — Schema versioning

`civilizationAssemblyProjectionSchemaVersion` 1.5.0 → 1.6.0. Additive + `omitempty`: a 1.5.0 consumer sees no change; the site (v1.x check) accepts 1.6.0. Site MUST render byte-identically against a 1.5.0 payload (regression-tested on the site side).

### D5 — Site rail (consumer half; site#204)

- View-model derivation in `buildConsoleIssueScan`: rail data only when the surface freshness is one of the EXISTING usable states — `current`, `stale`, or `partial`, exactly the set that renders the board (CFADA1-adv2: the rail and board share one freshness decision; they can never diverge) — AND the section's own `status == "available"` (allowlist); absent field / unknown status / unavailable → no rail, board unchanged.
- Civilization-projection client timeout 8s → 9s (D2; `graph/ops.go:1119`).
- Rail renders inside the polled intake fragment (refreshes with the board; inherits the B1 drawer-reset semantics untouched).
- State→style map is an allowlist over the v1 state set ONLY (amber for parked/human_action, neutral for queued/in_flight/recorded — `ready_for_human` does not exist in v1 and gets NO mapping; CFADA2-1); unknown state values (including a future ready state) render escaped text with neutral style — `default` is neutral, never a healthy color.
- Drawer links ONLY for runs whose (run_id, stage_id) exists on the rendered board (site-side index at build time); everything else unlinked. No dead links, no fabricated targets (IADA-5).
- Relative age from `last_event_at`; unparseable → age omitted.

## 3. Contract (source of truth)

As specified in hive#240: top-level `recent_issue_scan_runs: {status: "available"|"unavailable", summary, truncated, runs: [{run_id, factory_order_id?, repo, issue_number, issue_url?, issue_title?, state, first_event_at?, last_event_at?, blocker_type?, required_action?, stage_id?, source_refs}]}`. States: the D1 allowlist. Empty store or fold failure → `status: "unavailable"` + honest summary + zero runs.

## 4. Non-goals

- No new event emission; no scanner changes; no single-issue enqueue.
- No TTL caching or precomputation of projections (D2's singleflight is request-collapsing over fresh computations, not caching; broader hive-ops-api latency rework beyond D2 stays out of scope; B3 covers work-server).
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

### Round 2 (codex, 2026-07-02) — VERDICT: BLOCKERS (4) → all resolved in v0.3.0

- **CFADA2-1 (ready state leaked through the consumer):** packet D5 still mapped `ready_for_human` to emerald and site#204 still said "ready". Resolved: scrubbed everywhere; the style allowlist covers the v1 state set only; a future ready state renders neutral until a provable fold exists.
- **CFADA2-2 (predicate imprecision):** resolved — the predicate is `isIssueScanRunLaunch(content)`, the kind predicate hive dispatch uses; the lifecycle parser is explicitly rejected.
- **CFADA2-3 (truncation × precedence):** a truncated parked page makes parked-absence unprovable. Resolved: unmatched requested-runs degrade to `recorded` when the parked page is truncated; requested-page truncation yields honest absence, never a wrong state.
- **CFADA2-4 (concurrent pollers defeat a bare timeout bump):** board + drawer each poll at 10s, overlapping computations contend past the client timeout. Resolved: hive-ops-api singleflight on the projection endpoints (honesty-preserving — no TTL cache) + corrected 8s→9s premise (adv1).
- Advisories adopted: real 8s base documented (adv1); `factoryOrderIDForRunLaunch` join spelled out (adv2); deterministic (timestamp, event ID) tie-breaker (adv3).

## 7. IADA record (v0.1.0, 2026-07-02)

- **IADA-1 (unprovable ready state):** if final-stage completion cannot be proven from existing folds without new queries, the state is dropped from v1 rather than approximated — scope reduction over guessing.
- **IADA-2 (conflicting evidence):** same run in parked page and queued fold → explicit precedence + dedupe by run_id; without this, the rail could show one run twice in two states.
- **IADA-3 (latency budget):** the endpoint is borderline against the site's 5s client timeout TODAY (measured 5.2s live); the fold reuses fetched pages, permits at most one new query, and carries a timed regression test.
- **IADA-4 (timestamp honesty):** zero/unparseable event timestamps must not default to `now` (would fake recency); fields omitted, site renders unageable.
- **IADA-5 (dead links):** rail entries link to drawers only for (run_id, stage_id) pairs present on the rendered board; a link to a non-existent drawer target would render an honest not-found drawer but still be a fabricated affordance.

### Round 3 (codex, 2026-07-02) — VERDICT: BLOCKERS (1) → resolved in v0.4.0

- **CFADA3-1 (internal inconsistency):** §4 non-goals still forbade "caching/latency rework" that D2 mandates, and D5 kept the stale 5s premise. Resolved: non-goals now distinguishes singleflight (request-collapsing over fresh computations — in scope) from TTL caching/precomputation (out of scope); D5 says 8s → 9s.
- Advisories: both predicates confirmed real (`isIssueScanRunLaunch` = brief.kind == transpara_ai_github_issue_scan; `factoryOrderIDForRunLaunch` = fo_run_<suffix>); singleflight implementation notes adopted — applied AFTER auth, keyed per endpoint, fresh computation, per-request response writing.

### CFAR round 1 (codex, on PR #241) — 3 blockers → resolved

Implementation-stage adversarial review (post-CFADA, against the merged fold in `civilization_assembly_projection.go`/`civilization_recent_issue_scan.go`) found the fold did not fully carry through the D1 fail-closed matrix and evidence-completeness principles to every evidence source and every row shape. All three resolved in one change:

- **Evidence-read uncertainty not propagated (P2):** the builder already fails closed when the PARKED page cannot be read (`parkedFetched=false` → rail unavailable), but the other two evidence sources had no equivalent gate. The `factory.run.requested` query's own error (`requestedErr`) was recorded in `operatorProjection.Errors` but never reached the rail fold, so a failed requested-run query silently produced an empty (not unavailable) rail — indistinguishable from a healthy empty store. Separately, the reused factory-order/work-evidence computation's own failure or truncation signals (`factoryOrdersQueryFailed`, `factoryOrdersTruncated` — already computed by `civilizationAssemblyFactoryOrders` for the board's `FactoryOrderSummary`/`WorkEvidenceSummary` sections) were never consulted by the rail fold's in_flight/queued promotion, so a requested run could be promoted to `queued` (asserting "no work evidence exists") when the truth was "work evidence could not be read." Resolved, completing the fail-closed matrix over all three evidence sources:
  - `civilizationRecentIssueScanRuns` gained a `requestedFetched` parameter (mirroring `parkedFetched`): `!requestedFetched` → whole rail unavailable, zero runs, honest summary — same shape as the existing parked-fetch-failure rule.
  - `civilizationRecentIssueScanRuns` gained `workEvidenceQueryFailed`/`workEvidenceTruncated` parameters, wired from the builder's existing `civilizationAssemblyFactoryOrders` return values. When either is true, `in_flight`/`queued` are unprovable for requested runs not otherwise resolved by parked evidence (can prove neither work-evidence-absence nor work-evidence-presence-and-completeness) — those rows degrade to `recorded` (existence proven via the issue-scan brief predicate match, position unproven). `Truncated=true` is set specifically when `workEvidenceTruncated` caused the degrade (a pure query failure is already surfaced via `operatorProjection.Errors`/`FailureReasons`, so it does not also claim truncation). Parked/human_action rows are unaffected — their evidence (`hive.issuescan.run.parked`) is independent of work-task evidence.
  - Tests: `TestCivilizationRecentIssueScanRunsRequestedFetchFailureIsUnavailable` (erroring-store wrapper for `EventTypeFactoryRunRequested` → rail unavailable), `TestCivilizationRecentIssueScanRunsFactoryOrderEvidenceFailureDegradesToRecorded` (reused `factoryOrderReadFailureStore` → requested row `recorded`), `TestCivilizationRecentIssueScanRunsFactoryOrderEvidenceTruncationDegradesToRecorded` (work.task.created page truncated → requested row `recorded`, rail `Truncated=true`).

- **in_flight rows omit their proving evidence (P2):** the in_flight promotion joined the requested event against the factory order's stage work-task evidence (`civilizationRecentIssueScanFactoryOrdersWithStageEvidence`) only to decide a boolean (promote or not) — the winning row's `source_refs`/`first_event_at`/`last_event_at` still carried only the `factory.run.requested` event, so the row asserted a state (`in_flight`) its own evidence didn't substantiate. Resolved: `civilizationRecentIssueScanFactoryOrdersWithStageEvidence` now returns, per factory order, the actual PROVING stage-task record (`civilizationRecentIssueScanStageProof`: task reference — `CanonicalTaskID` when present, else the task's EventGraph event ID — plus `StageID` and the task event's own timestamp, derived from its UUIDv7 via `EventID.TimestampMS()` since `CivilizationAssemblyTaskEvidence` carries no timestamp field). When more than one qualifying stage task exists for an order, the LATEST wins. The in_flight row now appends the proof's task reference to `source_refs`, sets `stage_id` from the proof, and uses `max(requested-event timestamp, proof timestamp)` for `last_event_at` — `first_event_at` stays the requested event's timestamp (the run's origin never moves).
  - Test: `TestCivilizationRecentIssueScanRunsInFlightCarriesTaskProof` — asserts the stage task's `CanonicalTaskID` is present in `source_refs`, `stage_id` matches the stage, and `last_event_at` (sleeping past a full second boundary so the RFC3339-second-formatted strings provably differ) reflects the later stage-task evidence timestamp while `first_event_at` remains the requested event's.

- **non-parked rows drop issue fields (P2):** `queued`/`in_flight`/`recorded` rows left `IssueNumber`/`IssueURL`/`IssueTitle` at zero values even though the issue-scan brief already carries the selected issue (the same `selected_issue` payload `isIssueScanRunLaunch` partially parses just to read `kind`). Resolved: added `civilizationRecentIssueScanParseBrief`, which decodes `content.Brief` ONCE per requested event, extracting both the issue-scan kind predicate (replacing the separate `isIssueScanRunLaunch` call inside the fold — same predicate, same brief-kind constant, no behavior change for the exclusion rule) and the selected issue's repo/number/url/title into a `civilizationRecentIssueScanBrief`. The shared `civilizationRecentIssueScanRequestedRun` constructor now populates `Repo`/`IssueNumber`/`IssueURL`/`IssueTitle` from the parsed brief, keeping the existing `TargetRepos[0]` fallback for `Repo` only when the brief has no selected issue (a brief with the right kind but no `selected_issue` payload still parses `ok=true` with a zero-value brief, so the fallback path is unchanged).
  - Tests: `TestCivilizationRecentIssueScanRunsBriefSourcedIssueFields` (queued row asserts real `issue_number`/`issue_url`/`issue_title` from the brief), `TestCivilizationRecentIssueScanRunsBriefMissingIssueFallsBackToTargetRepos` (brief with no selected issue still falls back to `TargetRepos[0]` for `Repo`, `IssueNumber` stays 0).
