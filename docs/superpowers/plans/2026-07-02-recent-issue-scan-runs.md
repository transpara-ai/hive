# Recent Issue-Scan Runs Projection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fold recent issue-scan runs (queued / in-flight / parked / recorded) into a new `recent_issue_scan_runs` section of the civilization assembly projection, served by hive-ops-api with singleflight.

**Architecture:** Spec is `docs/designs/recent-issue-scan-runs-projection-v0.1.0.md` (internal v0.4.0, CFADA PASS — read it FIRST; its D1/D2/D3 tables are binding). New fold in `pkg/hive/civilization_recent_issue_scan.go` consumes the parked page (shared with the existing board fold), one new `factory.run.requested` page filtered by `isIssueScanRunLaunch`, and existing factory-order/work-evidence outputs. hive-ops-api projection endpoints gain request-collapsing singleflight.

**Tech Stack:** Go; in-memory store seeding patterns from `pkg/hive/operator_projection_test.go`; `golang.org/x/sync/singleflight` (already in module graph — promote from indirect if needed).

## Global Constraints

- Fail-closed state allowlist per packet D1: `parked`/`human_action` (shared parked-run helper), `queued` (isIssueScanRunLaunch predicate + no parked + no work evidence + NON-truncated parked page), `in_flight` (queued proof + factoryOrderIDForRunLaunch join carrying ≥1 work-task evidence), `recorded` (proven existence, unproven position). NO ready_for_human. No default that assigns a healthy state.
- Precedence parked/human_action > in_flight > queued > recorded; dedupe latest-parked-wins-whole with (timestamp, event ID) tie-break; blank run_id excluded; truncated parked page → unmatched requested-runs degrade to `recorded`; `truncated` flag per packet D3.
- Exactly ONE new store query (`ByType(EventTypeFactoryRunRequested, …)`).
- Existing `issue_scan_projection` output byte-compatible; existing tests untouched in semantics.
- Schema constant → `"1.6.0"`.
- Commits: conventional, each ending `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Verify per task: `go build ./... && go vet ./pkg/hive/ && go test ./pkg/hive/ -count=1` (full `go test ./...` in the final task).

---

### Task 1: Rail fold + contract types + builder wiring + full-domain tests

**Files:**
- Create: `pkg/hive/civilization_recent_issue_scan.go`
- Modify: `pkg/hive/civilization_assembly_projection.go` (struct field + schema constant + builder call; the parked-page fetch refactors so ONE fetched page feeds both the existing board fold and the new rail fold — do not fetch it twice)
- Test: create `pkg/hive/civilization_recent_issue_scan_test.go`

**Interfaces:**
- Consumes: `EventTypeIssueScanRunParked`/`IssueScanRunParkedContent`, `EventTypeFactoryRunRequested` + `isIssueScanRunLaunch(content)`, `factoryOrderIDForRunLaunch(runID)`, existing operator-projection factory-order/work-evidence outputs, store `ByType` paging (mirror the existing truncation-detection pattern).
- Produces (site consumes this JSON contract verbatim):

```go
type CivilizationRecentIssueScanRuns struct {
	Status    string                                `json:"status"` // "available" | "unavailable"
	Summary   string                                `json:"summary,omitempty"`
	Truncated bool                                  `json:"truncated,omitempty"`
	Runs      []CivilizationRecentIssueScanRun      `json:"runs,omitempty"`
}

type CivilizationRecentIssueScanRun struct {
	RunID          string   `json:"run_id"`
	FactoryOrderID string   `json:"factory_order_id,omitempty"`
	Repo           string   `json:"repo"`
	IssueNumber    int      `json:"issue_number"`
	IssueURL       string   `json:"issue_url,omitempty"`
	IssueTitle     string   `json:"issue_title,omitempty"`
	State          string   `json:"state"` // parked|human_action|queued|in_flight|recorded
	FirstEventAt   string   `json:"first_event_at,omitempty"` // RFC3339 UTC; omitted when unproven
	LastEventAt    string   `json:"last_event_at,omitempty"`
	BlockerType    string   `json:"blocker_type,omitempty"`
	RequiredAction string   `json:"required_action,omitempty"`
	StageID        string   `json:"stage_id,omitempty"`
	SourceRefs     []string `json:"source_refs,omitempty"`
}
```

- New field on `CivilizationAssemblyProjection`: `RecentIssueScanRuns CivilizationRecentIssueScanRuns \`json:"recent_issue_scan_runs,omitempty"\`` — note struct `omitempty` needs the field to be a pointer OR always-populated; DECISION: always populate (status unavailable when empty store) and drop `omitempty` on the field so consumers can rely on `status` — document this in the field comment.

- [ ] **Step 1: Write failing tests.** Read `operator_projection_test.go` seeding helpers first (how tests append parked events / factory.run.requested events / work evidence to an in-memory store, e.g. the fixtures used by `TestBuildCivilizationAssemblyProjectionProjectsParkedIssueScanCanary` ~line 415 and the queued-lifecycle test ~line 1283). Then write a table-driven `TestCivilizationRecentIssueScanRunsStateDomain` covering EVERY packet-D1 row: parked (state + blocker fields present, identical to the board's card for the same run — assert cross-section consistency); human_action mapping; queued (issue-scan requested, no parked, no work evidence); in_flight (requested + order summary with work-task evidence — seed via the same fixtures the factory-order fold tests use); recorded (requested + truncated parked page); generic non-issue-scan factory.run.requested EXCLUDED entirely; blank run_id excluded; two parked events for one run → latest wins whole + tie-break by event ID on equal timestamps; parked beats queued with refs unioned; ordering by last_event_at desc; truncated flags. Plus `TestCivilizationRecentIssueScanRunsEmptyStoreUnavailable` (status unavailable, zero runs, honest summary).
- [ ] **Step 2: Run to verify failure** (`go test ./pkg/hive/ -run RecentIssueScan -count=1` — expect undefined symbols).
- [ ] **Step 3: Implement** the fold per packet D1-D3: pure function `civilizationRecentIssueScanRuns(parkedPage <the page type used by the existing fold>, parkedTruncated bool, requestedEvents …, requestedTruncated bool, orderSummaries …) CivilizationRecentIssueScanRuns` (exact parameter types to match what the builder already has in scope — keep it PURE, I/O stays in the builder); a SHARED normalized parked-run helper used by BOTH the existing board fold and this fold (refactor the board fold to consume it — output must stay byte-identical, existing tests prove it); wire into `BuildCivilizationAssemblyProjection` with the single new `ByType(EventTypeFactoryRunRequested, limit, …)` fetch; bump schema constant to 1.6.0.
- [ ] **Step 4: Run** `go test ./pkg/hive/ -count=1` — ALL tests green (existing + new).
- [ ] **Step 5: Commit** `feat(ops): project recent issue-scan runs in civilization assembly projection`.

### Task 2: hive-ops-api singleflight + endpoint test

**Files:**
- Modify: `pkg/hive/operator_api.go` (both projection endpoints), `go.mod`/`go.sum` if x/sync needs promoting.
- Test: extend the operator_api test file (find it: `grep -l "operator-projection" pkg/hive/*_test.go`).

**Interfaces:** `golang.org/x/sync/singleflight.Group`, keyed `"operator-projection"` / `"civilization-assembly-projection"`, applied AFTER auth, sharing the marshaled projection value (or the projection struct) — each request writes its own response. Fresh computation per flight; NO caching of results beyond the in-flight window.

- [ ] **Step 1: Write failing tests:** (a) endpoint serves `recent_issue_scan_runs` JSON end-to-end (seeded store, httptest, assert section present with expected state); (b) singleflight collapse: instrument with a counting store wrapper (wrap store.Store, count ByType calls) + fire N=4 concurrent requests → assert computation ran fewer than N times (≥1, <N) AND all 4 responses are valid identical JSON.
- [ ] **Step 2: verify failure.**
- [ ] **Step 3: Implement** (singleflight after the bearer check; do NOT hold the flight across response writing).
- [ ] **Step 4:** `go test ./pkg/hive/ -run 'OperatorAPI|Singleflight|RecentIssueScan' -count=1` green.
- [ ] **Step 5: Commit** `feat(ops-api): singleflight projection endpoints and serve recent issue-scan runs`.

### Task 3: Timed fold budget + full verification

- [ ] **Step 1:** Add `TestRecentIssueScanFoldLatencyBudget`: seed a store with ~200 mixed events (parked + requested + generic + work evidence via existing fixtures); time `BuildCivilizationAssemblyProjection` WITH the fold vs a baseline (time the builder, then time the pure fold function alone on the same inputs); assert the pure fold's wall-clock < 250ms AND < 10% of the builder's total. Use generous margins to avoid flake (e.g. skip under `-short`).
- [ ] **Step 2:** `go build ./... && go vet ./... && go test ./... -count=1` — full module green.
- [ ] **Step 3: Commit** `test(ops): recent issue-scan fold latency budget`.
