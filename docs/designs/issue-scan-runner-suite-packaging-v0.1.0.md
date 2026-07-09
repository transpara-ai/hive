---
doc_id: HIVE-ISSUE-SCAN-RUNNER-SUITE-PACKAGING
title: Hive Issue-Scan Runner Suite Design and Packaging Contract
doc_type: design
status: proposal
version: 0.1.0
created: 2026-07-09
updated: 2026-07-09
owner: Michael Saucier
steward: codex
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#260
authority: documentation-only; no executable runner implementation, Hive wake/start/action API use, live issue-scan dispatch, runtime execution, production EventGraph write, Work write, deploy, service restart, protected settings change, Test 001 GREEN, production go-live, value allocation, autonomy increase, or wiki work
---

<!-- df:artifact id=HIVE-ISSUE-SCAN-RUNNER-SUITE-PACKAGING type=design version=0.1.0 status=proposal -->
<!-- df:scope project=dark-factory v4.0 hive-260 issue-scan runner-suite packaging design-only no-runtime-execution no-hive-action-api no-production-eventgraph-write no-work-write no-deploy no-autonomy-increase no-wiki-work -->
<!-- df:ingest mcp=true chunking=heading hidden_headers=true -->

# Hive Issue-Scan Runner Suite Design and Packaging Contract

## Summary

Hive already has the issue-scan lifecycle and runner attachment points needed to
move a selected GitHub issue toward a ready-for-Human pull request. The missing
piece is a canonical runner-suite packaging contract: the operator-facing shape
that says which executables are required, how they are rehearsed, how they are
wired into named-run progress or the daemon, and which actions remain outside
runner authority.

This packet records that shape for `transpara-ai/hive#260`. It is a design and
packaging contract only. It does not add runner executables, change daemon
configuration, start Hive, scan issues, dispatch runs, create pull requests, mark
pull requests ready, approve, merge, deploy, or write production truth.

## Source Reconciliation

| Source | Role | Material decision |
|---|---|---|
| `transpara-ai/hive#260` | Issue-source intent | Requests one governed runner-suite design/packaging packet for a working Hive issue-scan factory path. The issue's first PR-ready scope is design/docs only; executable runner implementation is deferred to future child issues. |
| `cmd/hive/factory.go` | Operator command spine | Defines the issue-scan command family, full-chain daemon flags, named progress flags, and `--issue-scan-require-full-chain` admission guard. |
| `cmd/hive/factory_issue_scan_runner_contracts.go` | Machine-readable runner contract | Emits the external runner, managed boundary, and ready-PR finalizer contracts for lifecycle version `civilization_issue_to_human_ready_pr_v0.9`. |
| `cmd/hive/factory_issue_scan_runner_contexts.go` | Rehearsal probe | Builds ready/not-ready context probes for one stored run; may dispatch/scaffold the run but does not invoke external runners. |
| `cmd/hive/factory_scan_issues.go` | Manual issue-scan queue path | Reads GitHub issues, filters to PR-ready candidates, applies review-capacity throttle, queues one run, and optionally dispatches it. |
| `cmd/hive/factory_issue_scan_scanner.go` | Daemon scanner work-start guard | Applies kill switch, one-active, review-capacity, PR-ready, and dedupe guards before queueing a new issue-scan run. |
| `pkg/hive/issue_intake.go` | Change-control intake contract | Requires `cc:pr-ready` and treats `cc:pr-deferred`, `cc:needs-human-scope`, and `cc:protected-action` as work-start blockers. |
| `docs/designs/review-capacity-throttle-v0.1.0.md` | Review-load guard | Records that open PRs are conservative unproven exact-head review load and that the throttle only prevents new issue-scan work-start. |
| `docs/designs/change-control-label-plane-invariants-v0.1.0.md` | Label-plane invariant | Records human-owned `cc:*` state movement and future stale-claim-release prerequisites. |

## Packaging Problem

The runner chain exists as several validated attachment points, but an operator
still has to infer the package:

- which runners are external executables versus Hive-managed boundaries;
- which command proves a context is ready before a runner is invoked;
- which path is single-run rehearsal and which path is daemon admission;
- where Human approval is required before draft PR creation;
- where exact-head review is required before ready-for-Human PR evidence;
- where the chain must stop instead of mutating GitHub, Work, or production
  EventGraph truth.

The v1 package must make these boundaries explicit so future implementation can
install and verify a runner suite without re-solving the governance contract.

## v1 Runner Suite Shape

The v1 suite is a set of executable adapters plus Hive-managed boundary steps.
Each external adapter receives one JSON context on stdin, returns one JSON result
on stdout, and may write diagnostics to stderr. Hive remains responsible for
validating the result against the stored run, selected repository, lifecycle
stage, FactoryOrder, task, commit, pull request, and authority records before
recording evidence.

| Component | Required for full-chain daemon | Existing Hive contract | Authority boundary |
|---|---:|---|---|
| Stage-role output runner | Yes | `issue_scan_stage_role_output_runner_context` -> `hive.IssueScanStageRoleOutputRunnerResult` | Planning evidence only; no code implementation, PR creation, approval, merge, or deploy. |
| Implementation runner | Yes | `issue_scan_implementation_runner_context` -> `hive.IssueScanImplementationRunnerResult` | May modify only the supplied target repo context; no PR creation, readying, approval, merge, or deploy. |
| Adversarial review runner | Yes | `issue_scan_adversarial_review_context` -> `hive.IssueScanAdversarialReviewReceipt` | Exact-head review evidence only; no repair, PR mutation, approval, merge, or deploy. |
| Blocker repair runner | Yes | `issue_scan_blocker_repair_runner_context` -> `hive.IssueScanBlockerRepairRunnerResult` | May repair only after review blockers reopen implementation work; no PR creation, readying, approval, merge, or deploy. |
| Draft PR authority requester | Yes, managed by Hive | `issue_scan_draft_pr_authority_request_runner_context` -> `hive.IssueScanDraftPRAuthorityRequestRunnerResult` | Raises a Human approval request only; does not create a PR. |
| Draft PR creator | Yes, managed by Hive | `issue_scan_draft_pr_creation_runner_context` -> `hive.IssueScanDraftPRCreationResult` | Creates a draft PR only after matching Human approval; no readying, approval, merge, or deploy. |
| Ready-state review runner | Yes in managed finalizer posture | `issue_scan_ready_state_review_context` -> `hive.IssueScanReadyStateReviewReceipt` | Reviews the exact PR head after the managed finalizer performs the draft-to-ready transition; no approval, merge, or deploy. |
| Ready PR finalizer | Yes, managed by Hive through `--issue-scan-ready-pr-mark-ready` | `issue_scan_ready_pr_runner_context` -> `hive.IssueScanReadyPRRunnerResult` | Transitions only the approved draft PR to ready, invokes ready-state review on that exact head, and records ready-for-Human evidence only after review passes; no Human approval, merge, or deploy. |
| Generic ready PR evidence runner | Alternative terminal adapter through `--issue-scan-ready-pr-runner` only | `issue_scan_ready_pr_runner_context` -> `hive.IssueScanReadyPRRunnerResult` | Mutually exclusive with the managed finalizer; records externally supplied ready evidence only. |

The recommended v1 package uses the managed ready-PR finalizer. The generic
ready-PR evidence runner remains an adapter escape hatch for a separately
reviewed environment, not the default.

## Package Contents

A future implementation package should be auditable from the filesystem before
any daemon uses it. The expected contents are:

- `manifest.json`: suite id, lifecycle version, component list, executable path
  or command, argv, timeout, required environment variables, forbidden
  environment variables, stdin kind, stdout kind, authority boundary, and
  validation command.
- `runners/`: external runner executables or wrapper scripts for stage-role
  output, implementation, adversarial review, blocker repair, and ready-state
  review. The managed draft PR requester, draft PR creator, and ready finalizer
  stay inside Hive and are configured by daemon flags.
- `catalog/`: model/provider selection records for runner execution when needed.
  A package may name Codex or Claude subscription providers, but must not embed
  secrets.
- `examples/`: inert stdin context fixtures and expected stdout receipts for
  local parser validation. Fixtures must be synthetic or public-safe source
  records, not production data.
- `checks/`: local validation commands that prove each runner is executable,
  accepts a fixture on stdin, returns parseable JSON on stdout, and refuses
  unsupported stages or missing required fields.
- `README.md`: operator wiring for rehearsal and daemon admission, including
  non-authorizations and stop conditions.

No package content may include credentials, production EventGraph connection
strings, private target data, literal private network addresses, or wiki work.

## Operator Flow

### 1. Intake and readiness

Hive reads GitHub issues from configured repos or `repos.json`. A source issue is
eligible for issue-scan work-start only when it carries `cc:pr-ready` and does
not carry `cc:pr-deferred`, `cc:needs-human-scope`, or `cc:protected-action`.
Unreadable issue state fails closed.

Non-FactoryOrder-ready issues belong in the canary/fidelity path. Hive may
surface missing fidelity guidance, but must not treat those issues as work
orders until a Transpara Team member moves them into a PR-ready state.

### 2. Queue and dispatch

`hive factory scan-issues` and the daemon scanner queue a run request only after
review-capacity, active-run, label-readiness, and dedupe guards pass. Queueing a
run is not implementation, not PR readiness, not Human approval, not merge, and
not deploy.

### 3. Rehearse a named run

Before daemonizing a suite, the operator should rehearse one stored run:

1. Probe context readiness with `hive factory issue-scan-runner-contexts --run
   <run_id>`. Use `--include-payload` only for local debugging and PR-visible
   redacted evidence when appropriate.
2. Invoke standalone `hive factory run-issue-scan-*` commands against the named
   run to validate one runner at a time.
3. Use `hive factory progress-issue-scan --run-configured-runners --run <run_id>`
   as the bounded same-run rehearsal of the configured chain.

The rehearsal path is still runtime execution if it invokes runners. It requires
separate authorization before use. This packet only defines the path.

### 4. Admit to daemon

The daemon may use the suite only when `--issue-scan-require-full-chain` passes.
That guard requires:

- a positive issue-scan interval;
- either explicit issue-scan repos or `--issue-scan-registry`;
- a repo workspace root;
- external runner commands for stage-role output, implementation, adversarial
  review, blocker repair, and ready-state review;
- draft PR request, draft PR create, and managed ready-PR mark-ready flags;
- no generic ready-PR runner conflict with the managed finalizer.

Daemon admission proves executable availability and flag completeness. It does
not prove that a run should start, that a PR should be created, that a PR should
be marked ready, or that a merge is authorized.

The managed terminal sequence is deliberately ordered: Hive consumes the
approved draft PR receipt, transitions that draft PR to ready, obtains the exact
post-transition head, runs ready-state review on that head, and records
ready-for-Human evidence only after the review passes. During the gap between
GitHub draft-to-ready mutation and recorded ready-for-Human evidence, the PR is
not represented by Hive as Human-ready.

## Evidence Outputs

The suite must leave evidence at every handoff:

- planning stage role outputs recorded as `issue_scan_stage_role_output` and
  stage runtime evidence only when stage evidence is complete;
- implementation Operate result artifact and implementation task completion;
- adversarial review receipt and `code.review.submitted` event;
- blocker repair Operate result and a fresh review requirement;
- draft PR authority request record held for Human approval;
- draft PR receipt only after matching Human approval;
- ready-for-Human PR evidence only after exact-head ready-state review passes;
- terminal readiness evidence that still requires Human merge approval.

Absence of evidence is not success. Failed, missing, malformed, or mismatched
runner output must stop at the current stage and remain visible to the operator.

## Stop Conditions

The runner suite must stop if:

- the selected issue is no longer open, no longer PR-ready, or carries a blocker
  `cc:*` label;
- review-capacity throttle is at or above threshold or unreadable;
- the active-run guard detects another unparked non-terminal issue-scan run when
  one-active mode is enabled;
- a runner executable is missing or returns malformed stdout;
- a runner result does not match the run id, FactoryOrder id, selected repo,
  task id, lifecycle version, or expected commit/head;
- adversarial review returns blockers;
- Human approval for draft PR creation is absent, stale, denied, or does not
  match the exact derived target;
- the PR head changes after exact-head review;
- any step would require production EventGraph read/query/write, RuntimeBroker
  execution, Hive wake/start/action API use, deploy, service restart, protected
  settings changes, Test 001 GREEN, production go-live, value allocation,
  autonomy increase, or wiki work without separate authority.

## Future Implementation Readiness

A future implementation PR may become ready only after a child issue names its
exact scope. That child issue must decide whether it is packaging-only,
runner-wrapper implementation, daemon configuration, live rehearsal, or
production hardening. Those are separate risk classes and should not be merged
into one broad PR by default.

Minimum implementation criteria:

- package manifest schema and tests;
- fixture-based parser tests for every runner stdin/stdout contract;
- executable availability checks that fail closed before daemon entry;
- no embedded secrets or production connection strings;
- no literal private network addresses in package docs or fixtures;
- no automatic merge, approval, deploy, protected settings mutation, or value
  allocation path;
- explicit evidence for every command that can mutate GitHub, Work, or
  EventGraph state;
- CFADA/CFAR on the exact PR head before merge consideration.

## Acceptance Criteria

- The runner-suite components and authority boundaries are recorded against the
  existing Hive command contracts.
- Rehearsal, named-run progress, and full-chain daemon admission are separated.
- Non-FactoryOrder-ready issues remain parked for fidelity guidance rather than
  becoming work orders.
- Future implementation readiness criteria are defined without authorizing
  implementation.
- Protected actions remain outside this packet.

## Non-Authorizations

This packet does not authorize executable runner implementation, Hive wake/start
or action API use, live issue scanning, route fetch, private fetch,
authentication, runtime execution, RuntimeBroker execution, production
EventGraph reads/queries/writes, Work writes, GitHub mutation beyond this PR
flow, draft PR creation, PR readying, PR approval, PR merge, deploy, service
restart, protected settings changes, Test 001 GREEN, operation#26 closure,
operation#57 closure, production go-live, value allocation, autonomy increase,
or wiki work.
