---
doc_id: HIVE-VALUE-ALLOCATION-DISPLAY-ROUTING-DESIGN
title: Hive Value-Allocation Display And Routing Surface Design
doc_type: design
status: proposal
version: 0.1.0
created: 2026-06-29
updated: 2026-06-29
owner: Michael Saucier
steward: codex
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#235
interrogation_ref: https://github.com/transpara-ai/hive/issues/235#issuecomment-4829442266
canonical_arc: dark-factory/v4.0
authority: design-only; no implementation, routing, issue mutation outside normal governed hive#235 design closeout, Hive wake/action API use, EventGraph write, Work write, runtime execution, deploy, Test 001 GREEN/closure, residual-risk closure, production go-live, value allocation, or autonomy increase
---

<!-- df:artifact id=HIVE-VALUE-ALLOCATION-DISPLAY-ROUTING-DESIGN type=design version=0.1.0 status=proposal -->
<!-- df:scope project=dark-factory v4.0 hive-235 hive-235-design-closeout-only value-allocation display-routing design-only interrogation human-required deny-by-default pull-display-only no-implementation no-routing no-issue-mutation-outside-governed-design-closeout no-hive-wake no-eventgraph-write no-work-write no-deploy no-test-001-green no-residual-risk-closure no-value-allocation no-autonomy-increase -->
<!-- df:ingest mcp=true chunking=heading hidden_headers=true -->

# Hive Value-Allocation Display And Routing Surface Design

## Summary

This document defines the Hive-owned design boundary for a future
value-allocation display and routing surface. It is a design-only closeout for
`transpara-ai/hive#235`.

The selected Interrogation record for this issue supports one design-only PR.
It does not authorize implementation, runtime execution, routing, issue
mutation outside normal governed `hive#235` design closeout, EventGraph or Work
writes, deploy, value allocation, Test 001 closure, residual-risk closure,
production go-live, or autonomy increase.

## Source Reconciliation

| Source | Role | Material decision |
|---|---|---|
| `transpara-ai/hive#235` | Issue-source intent | Requests a Hive-owned design for value-allocation display/routing while preserving human-required boundaries. |
| `hive#235` Interrogation comment | Scope record | Records the selected design-only path and explicitly rejects implementation/routing authority. |
| `dark-factory/v4.0` Event 22 | Canonical doctrine | Records value-allocation as deny-by-default, permanently human, and pull/display-only. |
| `pkg/hive/issue_intake.go` | Existing Hive contract | Emits `civilization_issue_scan_value_allocation_boundary_v0.1` in issue-scan briefs. |
| `pkg/hive/operator_projection.go` | Existing Hive projection | Projects that policy as read-only operator state when present. |

The current Hive code already carries a value-allocation boundary policy in
issue-scan and operator-projection payloads. This design does not replace that
code. It names the intended display semantics so future Site, Work, EventGraph,
or Hive implementation proposals have one reviewable boundary.

## Boundary Model

| Field | Required value |
|---|---|
| Default posture | `deny_by_default_human_required` |
| Display mode | `pull_display_only_not_routing_or_action` |
| Candidate status | `value_allocation_candidate` |
| Decision status without human ref | `human_required` |
| Authority status without human ref | `no_allocation_authority` |
| Required human authority | `external_committee_value_allocation_decision`, `issue_scoped_authority_packet`, `human_approval_ref` |

No displayed row, candidate label, issue text, PR-ready label, model
recommendation, rank, alert, status, or stale/available state can become
authority to allocate value.

## Candidate Inputs

A future implementation may propose candidate inputs only from explicitly
allowed, cited records:

| Candidate input | Display treatment | Stop condition |
|---|---|---|
| GitHub issue or PR ref with value-allocation language | Show repo, number, title excerpt, labels, and source ref. | Missing source ref, closed/superseded source, or no human scope. |
| Hive issue-scan brief value-allocation policy | Show policy ID, version, posture, and non-authority claims. | Missing policy, invalid policy, or legacy brief without policy. |
| Authority request or recommendation | Show requested decision and evidence refs only. | No External Committee decision ref. |
| Human approval ref | Show exact approving actor/ref/date when public-safe. | Approval does not match candidate, scope, or head. |
| Evidence age and source freshness | Show current, stale, unavailable, or projection-only. | Source age unknown or source cannot be verified. |

Exact amounts, customer/vendor identities, compensation, equity, credit,
billing, pricing, legal commitment, or scarce-resource priority details may be
shown only when the future authority packet explicitly permits the exact field
and public/private boundary. Otherwise the display must use a redacted or
omitted-source row.

## Interrogation Step

Every human-required value-allocation decision must pass through an
Interrogation step before any child implementation issue, PR, or display claim
can proceed.

The Interrogation record must include:

| Required field | Meaning |
|---|---|
| Decision question | The exact human decision needed. |
| Candidate choices | At least park, design-only, and implementation/action paths when applicable. |
| Selected decision | The selected bounded path and whether it is human-approved or Codex-recommended only. |
| Authority refs | GitHub issue/PR/comment, commit, or docs packet refs that make the decision reviewable. |
| Forbidden actions | The action list that remains unauthorized after the decision. |
| Evidence freshness | Current, stale, unavailable, fixture/local, projection-only, or deployed-reference where applicable. |
| Stop condition | What must happen before any broader action can resume. |

If a selected decision is absent, stale, contradicted, or broader than its
evidence, the surface must fail closed as `human_required` and
`no_allocation_authority`.

## Display Semantics

The surface is for operator awareness, not action. It may show:

- candidate class;
- source ref;
- policy ID and version;
- current boundary state;
- last update;
- evidence freshness;
- missing authority;
- selected Interrogation decision;
- required next human decision.

It must not show:

- approve, allocate, route, assign, pay, bill, price, compensate, grant,
  prioritize, merge, wake, deploy, or execute controls;
- traffic-light-only status with no evidence text;
- a green state that implies value was approved;
- a PR-ready state unless a separate issue-scoped authority packet permits it;
- rank/order that implies scarce-resource priority;
- notification-as-action, assignment-as-action, or routing-as-action.

## Routing Semantics

Routing is denied by default.

A future implementation may display a human review venue only when a cited
authority packet names the venue. Displaying the venue is not routing. It must
not create assignments, notifications, labels, Work tasks, EventGraph writes,
Hive actions, or issue mutations unless a separate authority packet authorizes
the exact action.

## Tufte Visualization Requirements

The future surface should use a compact evidence table or small status rows. The
operator needs to compare candidate state, evidence freshness, and missing
authority, not inspect decorative charts.

Required display properties:

- sort by decision urgency and evidence freshness, not alphabetically;
- use direct text labels instead of legends;
- show source, last update, boundary, and evidence ref in the same row;
- use one focal accent only for the selected row or blocker;
- distinguish current, stale, unavailable, fixture/local, and projection-only;
- avoid gauges, 3D effects, pie/donut charts, oversized KPI cards, and
  traffic-light dots without evidence text.

## Acceptance Criteria

- The design records value-allocation as permanently human and deny-by-default.
- The design records display as pull/display-only, not routing or action.
- The design identifies candidate inputs, display semantics, routing semantics,
  human-required decisions, evidence refs, stop conditions, and forbidden
  automated actions.
- The design adds an Interrogation step for human decisions.
- The design does not authorize implementation, value allocation, automated
  routing, issue mutation outside normal governed `hive#235` design closeout,
  runtime execution, Hive wake/action API use, EventGraph write, Work write,
  deploy, Test 001 GREEN/closure, residual-risk closure, production go-live, or
  autonomy increase.

## Future Implementation Preconditions

Any future child implementation must start from a new issue-scoped
AuthorityDecision that specifies:

- primary repo and exact path allowlist;
- exact display fields and public/private data boundary;
- exact source records and freshness rules;
- Interrogation record format and required human approval refs;
- negative tests for no allocation, approval, routing, assignment,
  notification-as-action, payment, billing, entitlement, pricing, compensation,
  equity/credit, external commitment, or scarce-resource prioritization;
- negative tests proving any displayed human review venue is non-interactive
  unless the exact interaction is separately authorized;
- fail-closed behavior for stale, missing, conflicting, or unverifiable
  authority;
- CFADA before implementation and CFAR before merge;
- issue closure and label mutation boundaries.

Until that packet exists, `hive#235` can close only as a design issue. No child
implementation issue is PR-ready from this document alone.

## Validation Plan

- `git diff --check`
- Hive documentation/path check if present.
- Confirm the diff touches only this design document.
- Confirm no text claims implementation authority, value allocation,
  automated routing, issue mutation outside normal governed `hive#235` design
  closeout, Hive wake/action API use, EventGraph write, Work write, deploy,
  Test 001 GREEN/closure, residual-risk closure, production go-live, or
  autonomy increase.
