---
doc_id: HIVE-CHANGE-CONTROL-LABEL-PLANE-INVARIANTS
title: Hive Change-Control Label Plane Invariants
doc_type: design
status: proposal
version: 0.1.0
created: 2026-07-07
updated: 2026-07-07
owner: Michael Saucier
steward: codex
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#251
authority: documentation-only; no label mutation, issue mutation, runtime execution, EventGraph write, Work write, Hive wake/start/action API use, deploy, Test 001 GREEN, production go-live, value allocation, or autonomy increase
---

<!-- df:artifact id=HIVE-CHANGE-CONTROL-LABEL-PLANE-INVARIANTS type=design version=0.1.0 status=proposal -->
<!-- df:scope project=dark-factory v4.0 hive-251 cc-label-plane single-mover stale-claim-release documentation-only no-label-mutation no-issue-mutation no-runtime-execution no-eventgraph-write no-work-write no-hive-wake no-deploy no-test-001-green no-production-go-live no-value-allocation no-autonomy-increase -->
<!-- df:ingest mcp=true chunking=heading hidden_headers=true -->

# Hive Change-Control Label Plane Invariants

## Summary

The `cc:*` label plane is the GitHub issue change-control plane. It tells
Hive and other Civilization tooling which issues are visible, parked, scoped,
or ready for governed PR work.

This document records two invariants for that plane:

1. **Single mover:** each `cc:*` state has a bounded mover allowlist, and
   exactly one allowlisted actor may execute a given transition instance.
2. **Stale claim release:** a claimed issue with no linked branch or PR
   activity inside a bounded window must be released only through an
   allowlisted, auditable release path.

This is a documentation contract for `transpara-ai/hive#251`. It does not add
claim enforcement, change labels, create a claim label, or authorize Hive to
mutate human-reserved `cc:*` labels.

## Source Reconciliation

| Source | Role | Material decision |
|---|---|---|
| `transpara-ai/hive#251` | Issue-source intent | Requests documentation of single-mover and stale-claim-release invariants for the `cc:*` label plane. |
| `pkg/hive/issue_intake.go` | Current Hive behavior | `IssueScanCandidatePRReady` requires `cc:pr-ready` and returns false when `cc:pr-deferred`, `cc:needs-human-scope`, or `cc:protected-action` is present. `FilterIssueScanPRReadyCandidates` skips non-ready candidates. |
| `cmd/hive/factory_test.go` and `pkg/hive/issue_intake_test.go` | Current Hive test evidence | `TestIssueScanCandidatePRReadyRequiresPositiveReadyLabel` covers missing-ready and blocker-wins cases; `TestQueueIssueScanRunLaunchRejectsNonPRReadyIssue` proves a protected/human-scope issue does not queue a FactoryOrder; `TestQueueIssueScanRunLaunchFiltersNonPRReadyCandidatesBeforeSelection` proves non-ready candidates are filtered before selection. |
| `cmd/hive/factory_issue_scan_scanner.go` | Unreadable source behavior | `scanGitHubIssuesWith` returns the listing error instead of producing candidates, so unreadable issue state fails closed at scan time. |
| `pkg/hive/issue_scan_source_issue_marker.go` | Current marker boundary | `PlanIssueScanSourceIssueMarker` keeps `cc:*` labels out of operational status and uses `factory:*` labels for human-visible lifecycle projection. |
| `pkg/hive/issue_scan_source_issue_marker_test.go` | Current marker test evidence | `TestPlanIssueScanSourceIssueMarkerAcquiredUsesProjectionBoundary` fails if a planned marker adds or removes any `cc:*` label; `TestApplyIssueScanSourceIssueMarkerAddsLabelsAndSkipsDuplicateComment` proves the applied mutation matches the planned `factory:*` marker labels. |
| Live Civilization intake labels | Intake vocabulary | Live governed issues such as `transpara-ai/docs#226` and `transpara-ai/operation#26` use `cc:intake` and `cc:civilization-presence` as routing/projection labels. This document records them as intake labels, not Hive runtime constants. |

GitHub labels are not the source of execution truth. Work and EventGraph remain
the canonical runtime/provenance records. GitHub labels are human-visible intake
and projection signals.

## Actor Allowlist

| Actor class | May do | Must not do |
|---|---|---|
| Transpara Team member | Set or clear `cc:*` labels inside the issue's reviewed scope, subject to each label's set-by and moved-out allowlists in the Label State Table; set `cc:pr-ready` after `PR-Ready-When` evidence is satisfied. | Use a label as merge, deploy, runtime, EventGraph-write, value-allocation, Test 001, production go-live, or autonomy authority. |
| Human change-control registrar | Apply intake/routing labels when creating or triaging issues under an explicit change-control scope, subject to each label's set-by and moved-out allowlists in the Label State Table. Today this is a human Transpara Team sub-role, not an autonomous Hive role. | Mark `cc:pr-ready` without human verification, clear blocker labels when the Label State Table does not allow registrar movement, clear blocker labels without the required human decision or recorded deferral-resolution evidence, or mutate protected settings. |
| Hive issue-scan/runtime | Read `cc:*` labels and fail closed when the label set is not actionable. | Set, clear, or reinterpret human-reserved `cc:*` labels; treat issue text or labels as authority. |
| Source issue marker bridge | Apply only explicitly authorized `factory:*` marker labels and comments through its marker boundary. | Mutate `cc:*` labels or derive canonical workflow state from GitHub labels/comments. |

If no actor class is allowlisted for a transition, the transition is blocked.

## Label State Table

| Label | State meaning | Set by allowlist | Moved out by allowlist | Hive behavior today |
|---|---|---|---|---|
| `cc:intake` | Issue is in durable change-control intake. | Transpara Team member or human registrar. | Transpara Team member or human registrar, by adding a more specific readiness/blocker label or closing/superseding the issue through normal governed closeout. | Read as ordinary issue context; not required by the current issue-scan PR-ready predicate. |
| `cc:civilization-presence` | Issue should remain visible to Civilization intake/projection. | Transpara Team member or human registrar. | Transpara Team member or human registrar, after the issue is no longer Civilization-relevant, is closed, or is superseded. | Read/project only; not a work-start authority. |
| `cc:pr-ready` | Human-reviewed scope is ready for governed PR work under normal gates. | Transpara Team member after the issue's `PR-Ready-When` evidence is satisfied. | Transpara Team member today. A future single work owner may move it only after a reviewed claim/PR authority packet creates that exact transition. | Required for issue-scan to queue work; defeated by `cc:pr-deferred`, `cc:needs-human-scope`, or `cc:protected-action`. |
| `cc:pr-deferred` | PR work is deliberately parked for sequencing, evidence, timing, or dependency reasons. | Transpara Team member or human registrar. | Transpara Team member or human registrar after the deferral condition is resolved and recorded. | Blocks issue-scan work even if `cc:pr-ready` is also present. |
| `cc:needs-human-scope` | Human scoping or authority clarification is missing. | Transpara Team member or human registrar when human scope is required. | Transpara Team member after the scope decision is recorded. | Blocks issue-scan work even if `cc:pr-ready` is also present. |
| `cc:protected-action` | The issue may require protected authority, such as deploy, runtime execution, EventGraph truth write, settings/secret/ruleset mutation, Test 001 GREEN, production go-live, value allocation, or autonomy increase. | Transpara Team member or human registrar when protected-action risk is detected. | Transpara Team member after a matching authority decision removes or narrows the protected-action boundary. | Blocks issue-scan work even if `cc:pr-ready` is also present. |

This table is the closed `cc:*` vocabulary for this proposal. A new `cc:*`
label must land through a reviewed revision that names its state meaning,
set-by allowlist, moved-out allowlist, and Hive behavior. Until that revision
exists, unknown `cc:*` labels are non-actionable for issue-scan work-start and
must not create implied readiness. Under current Hive code, unknown `cc:*`
labels also do not block work-start; only `cc:pr-deferred`,
`cc:needs-human-scope`, and `cc:protected-action` defeat `cc:pr-ready` today.

## Single-Mover Invariant

Every state transition needs one current mover. A label may have a small
allowlist of possible mover classes, but each transition instance must have one
selected actor from that allowlist, not several competing movers.

Rules:

- A label state must name the actor class allowlist for moving it out of that
  state.
- The transition record must identify the one actor that actually moved it.
  Today's human transition record is the GitHub issue timeline; future
  automated movers must produce their own authority-scoped evidence output.
- A human registrar may clear a blocker label only when that label's moved-out
  allowlist names the registrar and the recorded evidence satisfies that
  label's exit condition; otherwise a Transpara Team decision is required.
- A future automated mover must have an issue-scoped authority packet, exact
  transition allowlist, idempotency key, and evidence output before it can
  mutate labels.
- `cc:pr-ready` is human-set. Automation may consume it only as read-only
  readiness evidence unless a later reviewed design explicitly adds an
  automated transition.
- Blocker labels win over readiness labels. If `cc:pr-ready` is present with
  `cc:pr-deferred`, `cc:needs-human-scope`, or `cc:protected-action`, Hive must
  refuse work-start.
- Missing `cc:pr-ready`, a defined blocker label paired with `cc:pr-ready`, or
  unreadable issue listing state fails closed at scan time; none becomes
  implied readiness. Future per-issue label-state enforcement must add
  unreadable-state tests before claiming a broader fail-closed guarantee.

## Stale-Claim Release Invariant

The current Hive label vocabulary does not define a `cc:*` claim label. Hive's
current claim/projection surfaces are Work/EventGraph records plus `factory:*`
source issue marker labels. A future enforcement arc may add a label-plane or
marker-plane claim mechanism, but that mechanism must satisfy this release
contract before it can start work autonomously.

Required release contract:

| Field | Required value |
|---|---|
| Claim operation | Atomic claim-before-work. The claim record or marker is written before branch creation, implementation, or PR creation. |
| Claim owner | Exactly one owner actor, run id, and FactoryOrder id. |
| Activity evidence | Linked branch, PR, Work task, or EventGraph stage evidence with last activity timestamp. |
| Stale window | Bounded and configurable; default recommendation is 24 hours unless a later authority packet chooses another value. |
| Release operation | Idempotent release to the exact state allowed by the future authority packet, with a release event/comment that names the stale claim and evidence checked. If no such state is explicitly allowed, release degrades to human action. |
| Release authority | The claim loop may release only its own stale claim or a claim class explicitly delegated to it. Human-owned blockers stay human-owned. |
| Non-claims | Release does not close the issue, mark `cc:pr-ready`, approve a PR, merge, deploy, execute runtime, write production EventGraph truth, write Work beyond its own explicitly authorized claim-release record, mark Test 001 GREEN, allocate value, or increase autonomy. |

If branch/PR/activity evidence cannot be read, release must fail closed to a
human-action state instead of silently requeueing the issue.

Because GitHub labels do not provide compare-and-swap semantics, a future claim
arc must name an atomicity mechanism before implementation. Acceptable shapes
include Work/EventGraph claim truth with labels as projection, or another
reviewed ownership record with recheck and idempotency. A label-only claim loop
is not enough authority for autonomous work-start.

## Acceptance Criteria

- The `cc:*` vocabulary has an allowlisted state table.
- Single-mover and stale-claim-release invariants are explicitly recorded.
- Current Hive behavior is stated honestly: Hive reads `cc:*` labels and fails
  closed, while source marker projection uses `factory:*` labels rather than
  mutating `cc:*`.
- Enforcement is deferred to a future governed arc with tests over claim,
  timeout, unreadable activity state, and release idempotency.
- This record does not authorize label mutation, issue mutation, runtime
  execution, EventGraph writes, Work writes, Hive wake/start/action APIs,
  deploy, Test 001 GREEN, production go-live, value allocation, or autonomy
  increase.

## Future Enforcement Preconditions

A future implementation PR must include:

- an issue-scoped authority packet naming the exact labels or markers it may
  mutate;
- unit tests for below-window, at-window, above-window, unreadable activity
  state, idempotent release, and wrong-owner refusal;
- PR-visible evidence that no human-reserved label can be set or cleared by
  automation without matching authority;
- tests proving unknown `cc:*` labels are non-actionable until a reviewed
  vocabulary revision defines them;
- an atomic claim mechanism, such as Work/EventGraph claim truth with GitHub
  labels as projection, or a reviewed equivalent with ownership recheck;
- CFADA and CFAR on the exact PR head before merge consideration.
