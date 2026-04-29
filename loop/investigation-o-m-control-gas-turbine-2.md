# Investigation: O&M Control Center — Gas Turbine 2 Performance Analysis

**Date:** 2026-04-29
**Task ID:** 1777466411
**Branch:** hive/feat/20260429-investigate-refinery-gaps-o-m-control-ce-1777466411
**Investigator:** hive (Builder)
**Parent state at spawn:** requirement.investigating
**Missing gates at spawn:** Definition of Done, Test Plan, Acceptance Criteria

---

## 1. What the Parent Spec Implies

**Title:** O&M Control Center: Gas Turbine 2 Performance Analysis

Parsed semantics:
- **O&M Control Center** — an operational monitoring UI or dashboard surface within the Transpara product
- **Gas Turbine 2** — a specific asset entity (not a category; the "2" implies at least one other turbine exists, likely GT-1 and GT-2 in a refinery or power plant)
- **Performance Analysis** — not just current status display; implies historical trending, deviation detection, comparative analysis, and root-cause surfacing

The spec title is a feature scope label. No spec body was recoverable from local files. The investigation therefore proceeds from:
1. Transpara MCP tool schemas (authoritative capability surface)
2. The absence of a `gas_turbine` performance profile in the profile catalog (found via `list_profiles`)
3. Standard O&M patterns for rotating equipment

---

## 2. Evidence Gathered

### 2.1 Transpara MCP Profile Catalog

Queried `list_profiles` (2026-04-29). Result — 7 profiles found:

| Profile ID | Asset Type | Framework |
|---|---|---|
| compressor_reliability | compressor | reliability |
| heat_exchanger | heat_exchanger | — |
| line | line | — |
| line_oee | line | oee |
| mine | mine | — |
| mining_division | Mining Division | — |
| plant | plant | — |

**Critical finding:** No `gas_turbine` profile exists. The `explain_entity_performance` and `rank_entities` tools require a `profile_id` to score an entity. Without a registered profile, Gas Turbine 2 performance analysis will fall back to no-profile mode (raw KPI display only, no deviation scoring, no peer comparison, no root-cause waterfall).

### 2.2 API Connectivity

`get_operations_overview` and `get_kpi_status` both returned API errors from this environment. Live KPI values for Gas Turbine 2 could not be retrieved. This does not affect the spec — it means the Test Plan must include steps executed against a live or staging Transpara instance.

### 2.3 Standard Gas Turbine O&M KPIs

From established O&M practice, a gas turbine performance analysis typically requires:
- **Heat Rate** (BTU/kWh or kJ/kWh) — primary efficiency metric
- **Power Output** (MW) vs. rated capacity
- **Exhaust Temperature** — leading indicator of combustion health
- **Compressor Discharge Pressure** — flow and compression health
- **Vibration** (x/y axes at bearing) — rotating equipment integrity
- **Inlet Temperature / Inlet Filter Delta-P** — ambient and filtration effects
- **NOx Emissions** — regulatory compliance
- **Starts/Hours** — maintenance scheduling input
- **Availability** — percentage of scheduled hours online

---

## 3. Gate Assessment

### 3.1 Definition of Done — ABSENT, drafted below

No DoD was present in the spec. The following is concrete and verifiable:

**Definition of Done — Gas Turbine 2 Performance Analysis:**

1. A `gas_turbine` performance profile is registered in the Transpara profile catalog with at minimum: heat rate, power output, exhaust temperature, and availability as scored dimensions.
2. `explain_entity_performance` returns a structured deviation waterfall for Gas Turbine 2 using the `gas_turbine` profile, covering at minimum the last 7 days.
3. A dashboard (created via `create_dashboard`) is accessible from the O&M Control Center surface showing: current KPI status, trend sparklines for each scored KPI, top-3 deviation contributors, and attention items for the entity.
4. `find_attention_items` for the Gas Turbine 2 root node returns results (no API error), correctly scoring proximity-to-limit, slope, drift, and time-to-limit signals.
5. `detect_regressions` for Gas Turbine 2 returns a split-window trend comparison with at minimum one prior window for baseline.
6. The dashboard renders with no broken panels when Gas Turbine 2 is within normal operating range.
7. The dashboard renders with at least one alarm indicator when a simulated HH (high-high) condition is injected.
8. All above verified in staging. Build + tests pass (if any Go code was modified in `hive` or `site` repos as implementation support).

### 3.2 Acceptance Criteria — ABSENT, drafted below

**Acceptance Criteria — Gas Turbine 2 Performance Analysis:**

| # | Criterion | Verifiable by |
|---|---|---|
| AC-1 | Opening the O&M Control Center for Gas Turbine 2 surfaces current KPI status within 3 seconds | Manual browser test or integration test against staging |
| AC-2 | The performance score for Gas Turbine 2 uses the `gas_turbine` profile (not a fallback or generic profile) | `list_profiles` returns `gas_turbine`; `explain_entity_performance` uses it |
| AC-3 | Heat rate, power output, and exhaust temperature trend lines are visible and show at minimum 7 days of history | `get_kpi_history` returns non-empty series; dashboard renders sparklines |
| AC-4 | Any KPI approaching its H or HH limit within 24 hours appears in the attention items panel | `find_attention_items` returns it at threshold ≤ 0.3 |
| AC-5 | "Gas Turbine 2" and "Gas Turbine 1" (or peer turbines) are comparable via `find_comparable_entities` | Tool returns peer list including GT-2; peer score displayed in dashboard |
| AC-6 | A user can navigate from the O&M Control Center summary to the Gas Turbine 2 detail view in one click | Manual UI test |
| AC-7 | Regression detection identifies a 10%+ heat rate degradation injected in the test data window | `detect_regressions` returns the degradation event |
| AC-8 | No personally identifying data or credentials appear in the dashboard output | Security review / output inspection |

### 3.3 Test Plan — ABSENT, drafted below

**Test Plan — Gas Turbine 2 Performance Analysis:**

#### Phase 1: Profile Registration Verification
- **Test:** Call `list_profiles` and assert `gas_turbine` profile is present with at minimum heat_rate, power_output, exhaust_temperature, availability as dimensions
- **Expected:** HTTP 200, `gas_turbine` entry in response
- **Fail condition:** Profile absent → implementation incomplete

#### Phase 2: Entity Existence and Hierarchy
- **Test:** Call `get_group_children` on the refinery/plant root node and traverse until Gas Turbine 2 entity ID is found
- **Expected:** Entity exists with `asset_type: gas_turbine` (or equivalent)
- **Fail condition:** Entity not found → data model gap

#### Phase 3: KPI Data Coverage
- **Test:** Call `get_kpi_history` for each scored KPI on Gas Turbine 2 for the window `[now-30d, now]`
- **Expected:** Non-empty time series for each KPI; no gaps > 4 hours
- **Fail condition:** Empty series or excessive gaps → data ingestion gap (out of scope for this spec but must be noted as a blocker)

#### Phase 4: Performance Explanation
- **Test:** Call `explain_entity_performance` for Gas Turbine 2, `start=now-7d`, `end=now`, `profile_id=gas_turbine`
- **Expected:** Structured response with `deviations`, `contributors`, `trend_deterioration`
- **Fail condition:** Error or empty response → profile scoring gap

#### Phase 5: Attention Item Detection
- **Test:** Call `find_attention_items` for Gas Turbine 2 root node, `horizon=24h`, `threshold=0.3`
- **Expected:** Returns ranked KPI list; items near limits appear first
- **Fail condition:** API error → connectivity or entity configuration gap

#### Phase 6: Regression Detection
- **Test:** In staging, inject synthetic heat rate degradation of 12% over 7 days. Call `detect_regressions` for Gas Turbine 2
- **Expected:** Regression event returned identifying heat rate with approximate degradation magnitude
- **Fail condition:** Regression not detected → scoring calibration gap

#### Phase 7: Dashboard Render
- **Test:** Call `create_dashboard` for Gas Turbine 2 and verify rendered output includes all AC-required panels
- **Expected:** Dashboard JSON/HTML with KPI status, sparklines, attention items, peer comparison
- **Fail condition:** Missing panels → dashboard template gap

#### Phase 8: Alarm State
- **Test:** Inject a simulated HH condition on exhaust temperature. Verify dashboard shows alarm indicator
- **Expected:** Red/HH badge on exhaust temperature panel
- **Fail condition:** No indicator → alarm propagation gap

---

## 4. Unresolved Ambiguities — Return to Clarify

These cannot be resolved by investigation alone. They must go back to the requester:

| # | Ambiguity | Why it matters |
|---|---|---|
| AMB-1 | **Which physical plant/site does Gas Turbine 2 belong to?** The entity ID is unknown. "GT-2" could be in any refinery in the Transpara deployment. | The root_id for all MCP tool calls depends on this. Without it, no tool call can be scoped correctly. |
| AMB-2 | **What is the rated capacity and design heat rate of Gas Turbine 2?** These are required to define "normal range" for deviation scoring in the gas_turbine profile. | The profile cannot be calibrated without nameplate data. |
| AMB-3 | **Is "Performance Analysis" real-time (< 1 min refresh), near-real-time (1–15 min), or historical (batch)?** The MCP tools suggest near-real-time polling. | Refresh frequency determines infra cost and UX design. |
| AMB-4 | **Who is the primary user of this view?** O&M engineer, plant manager, or executive? | Drives what summary-level vs. detail-level information is shown. The `explain_entity_performance` output is highly technical — it may need a consumer-facing translation layer for non-engineers. |
| AMB-5 | **Is Gas Turbine 1 a required peer for comparison, or is peer comparison optional?** | Determines whether `find_comparable_entities` must return GT-1 specifically, or any comparable turbine entity. |
| AMB-6 | **Are NOx emissions and regulatory thresholds in scope?** Emissions monitoring has compliance implications — a separate compliance gate may be required. | If in scope: adds a compliance AC and requires regulatory limit data per jurisdiction. |

---

## 5. Recommended Next FSM State

**Evidence:** The spec has sufficient title clarity to draft all three gates (DoD, AC, Test Plan) — now done above. The six ambiguities require requester response before implementation can begin. No gate is permanently blocked; each has a viable draft.

**Recommended transition:** `requirement.investigating` → **`requirement.clarifying`**

Rationale: The investigation found genuine gate absence (not poor naming), produced concrete draft content for all three gates, and identified six specific ambiguities that require requester input. The spec should move to Clarify to resolve AMB-1 through AMB-6, then merge the drafted gates into the spec body, and then transition to `requirement.specified` for Architect pickup.

---

## 6. Source Evidence

| Source | Used for |
|---|---|
| `list_profiles` response (2026-04-29) | Confirmed no `gas_turbine` profile registered (DoD item 1, AC-2) |
| MCP tool schemas (deferred tool manifest) | Derived available capability surface; grounded AC and Test Plan in real tool parameters |
| Standard gas turbine O&M KPI taxonomy | Grounded KPI list in established industry practice |
| Transpara profile pattern (compressor_reliability) | Used as reference schema for gas_turbine profile structure |
| Hive CLAUDE.md invariants 12 (VERIFIED) and 13 (BOUNDED) | Test Plan requires test coverage before any code ships; every tool call has a scoped root_id |
