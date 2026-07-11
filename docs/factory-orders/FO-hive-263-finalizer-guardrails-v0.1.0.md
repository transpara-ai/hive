---
doc_id: FO-HIVE-263-FINALIZER-GUARDRAILS
title: Factory Order — Managed Ready-PR Finalizer Approval Scope and Failure Remediation (Mocked-Only)
doc_type: factory-order
status: proposal
version: 0.1.0
created: 2026-07-11
updated: 2026-07-11
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#263
authority: mocked-only implementation of protected-action guardrails; no live PR readying, no live GitHub mutation outside test/mocked clients, no deploy, service restart, Hive wake/start/action API use, runtime execution, production EventGraph reads/queries/writes, live Work runtime writes, private fetch, authentication, protected settings changes, Test 001 GREEN, production go-live, value allocation, autonomy increase, or wiki work
---

# Factory Order — Managed Ready-PR Finalizer Approval Scope and Failure Remediation

## Immutable Source Citations

| Source | Pin | Role |
|---|---|---|
| [transpara-ai/hive#263](https://github.com/transpara-ai/hive/issues/263) | issue body as of 2026-07-11 (labels `cc:intake`, `cc:pr-deferred`, `cc:protected-action`, `cc:civilization-presence`, `cc:needs-human-scope`) | Raw intake — the governed tracker deferring implementation until a human scope packet |
| Michael Saucier, in-session operator scope verdict, 2026-07-11 | "mocked-only, re-draft permitted, evidence as Work artifacts" — validated against the code before acceptance (client-injected finalizer; GraphQL `convertPullRequestToDraft` sibling of the used `markPullRequestReadyForReview`; Work artifacts as the native evidence substrate) with the accepted refinement that re-draft permission is a **recorded approval-scope flag, fail-closed**, never an ambient default | Channel A human scope decision this FO implements; supplies the `needs-human-scope` answer |
| `docs/designs/issue-scan-runner-suite-packaging-v0.1.0.md` (blob `3e2fcc3ace24a0729e50074f3f2fd21fb05ad259`, merged via #261) | "The managed terminal sequence…", "Stop Conditions", ready-state-review failure remediation paragraphs | Design intent: readying only within recorded Human approval scope; preferred remediation is re-draft when authority and API allow; otherwise visibly blocked and unmergeable |
| `pkg/hive/factory_authority.go` | `DraftPRTarget.Scope()` — fixed 11-element `pull_request.create` encoding | Proof of the gap: today's recorded approval covers draft-PR **creation only**; no managed-ready or re-draft authority exists in any record |
| `pkg/hive/issue_scan_ready_pr_finalizer.go` | `RunIssueScanReadyPRFinalizer` calls `MarkReadyForReview` with **no approval-scope check** | The code path the guardrails gate |

## Requirements

- **R1 — Distinct mark-ready authority.** A new recorded authority action
  (`pull_request.mark_ready` discriminator alongside the existing
  `pull_request.create`) carries the exact ready target (repository, PR
  number, PR URL, head SHA), an explicit `re_draft_on_failure` flag, and a
  single-use nonce. Draft-creation approval alone can never authorize
  readying (allowlist: absence of a matching approved mark-ready record ⇒
  refuse).
- **R2 — Fail-closed approval gate in the finalizer.** `RunIssueScanReadyPRFinalizer`
  refuses to call `MarkReadyForReview` unless a recorded, **approved**,
  non-stale mark-ready decision exactly matches the run-derived target
  (repository, PR number, head SHA, nonce unused). Missing, denied,
  undecided, mismatched, or unreadable records all refuse with typed errors.
- **R3 — Failure remediation, re-draft under recorded scope only.** When
  ready-state review fails, errors, or cannot run after the draft→ready
  mutation: never record ready-for-Human evidence; if the matching approval's
  `re_draft_on_failure` flag is set AND the client supports it, call a new
  `ConvertToDraft` client method and record the outcome; otherwise leave the
  PR as-is and record why re-draft was unavailable. Either way the run
  surfaces a durable blocked state.
- **R4 — Blocked-state evidence as Work artifacts.** A structured
  `issue_scan_ready_pr_blocked` evidence artifact (kind, lifecycle version,
  run/order ids, PR identity, failure reason, remediation taken:
  `re_drafted` | `re_draft_unauthorized` | `re_draft_unsupported` |
  `re_draft_failed`, review ref if any) is recorded on the ready-stage task
  through the existing Work artifact path. Absence of evidence is never
  success; evidence-recording failure propagates as error.
- **R5 — Mocked-only boundary.** All new behavior is exercised through the
  existing injected interfaces (`IssueScanReadyPRFinalizerClient` gains
  `ConvertToDraft`) with mock clients and in-memory stores in tests. The live
  GitHub client gains the symmetric GraphQL `convertPullRequestToDraft`
  method (same transport as the existing `markPullRequestReadyForReview`),
  which no enabled path can reach: full-chain daemons require a runner suite
  that does not exist, and no approval record carries the new scope until a
  human grants one.
- **R6 — Whole-domain tests.** Table-driven tests prove the gate over the
  entire input domain per the fail-safe doctrine: missing approval, denied,
  undecided, stale/mismatched target (each field), unreadable store, approval
  without re-draft flag, approval with flag + client success, client error,
  client unsupported, review failure before/after mutation, evidence-append
  failure. The class-sweep audit runs BEFORE the first cross-family review
  round.

## Non-Goals

- No live PR readying or re-drafting; no daemon configuration or live
  rehearsal (separate risk classes per the design packet).
- No changes to draft-PR creation approval semantics.
- No operator UI; raising/approving the new authority request beyond the
  minimal recording needed for the finalizer gate is future work.

## Verification Plan

- `go test ./pkg/hive ./cmd/hive` (new table-driven tests; TDD RED→GREEN),
  `make verify`, `staticcheck` on touched packages, `git diff --check`.
- Author-side class-sweep audit of the full diff before round 1; IAR then
  CFAR (Codex reviewer) at the exact head; merge remains Michael's.

## Non-Authorizations

This Factory Order states intent and grants nothing beyond the governed PR
flow. Implementing the mark-ready authority type does not grant any instance
of it; every grant remains a recorded human decision.
