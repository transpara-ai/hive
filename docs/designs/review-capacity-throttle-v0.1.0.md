---
doc_id: HIVE-REVIEW-CAPACITY-THROTTLE
title: Hive Review-Capacity Throttle
doc_type: design
status: proposal
version: 0.1.0
created: 2026-07-07
updated: 2026-07-07
owner: Michael Saucier
steward: codex
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#250
authority: implementation guard; no RuntimeBroker execution, production EventGraph write, external adapter invocation, Hive wake/start/action API use, deploy, service restart, private fetch, protected settings change, Test 001 GREEN, production go-live, value allocation, autonomy increase, or wiki work
---

<!-- df:artifact id=HIVE-REVIEW-CAPACITY-THROTTLE type=design version=0.1.0 status=proposal -->
<!-- df:scope project=dark-factory v4.0 hive-250 issue-scan review-capacity fail-closed work-start no-runtime-execution no-production-eventgraph-write no-hive-action-api no-deploy no-autonomy-increase no-wiki-work -->
<!-- df:ingest mcp=true chunking=heading hidden_headers=true -->

# Hive Review-Capacity Throttle

## Summary

Hive must not start new autonomous issue-scan work when the human review queue
is already full. The review-capacity throttle is a pre-work-start guard:

- it reads open pull requests for the scoped Transpara-AI repos;
- it refuses issue-scan work-start when the open PR count is at or above the
  configured threshold;
- it fails closed when the review queue cannot be read;
- it records the throttle decision as a Hive EventGraph event in the configured
  local/store context;
- it never releases claims, changes labels, closes issues, marks PRs ready,
  approves, merges, deploys, starts Hive, or mutates GitHub.

The operator-facing default threshold is `3`.

## Source Reconciliation

| Source | Role | Material decision |
|---|---|---|
| `transpara-ai/hive#250` | Issue-source intent | Requests a fail-closed review-capacity throttle before autonomous work starts, with a default threshold around 3, unreadable-state refusal, and tests below, at, above, unreadable, and no fall-through. |
| `cmd/hive/factory_issue_scan_scanner.go` | Daemon work-start boundary | The throttle runs after local kill-switch and one-active checks, and before GitHub issue listing or `QueueIssueScanRunLaunch`. |
| `cmd/hive/factory_scan_issues.go` | Manual work-start boundary | `factory scan-issues` runs the same throttle before issue listing and records the refusal when the queue is at capacity. |
| `cmd/hive/factory_issue_scan_review_queue.go` | GitHub read-only queue inspector | Uses `gh pr list` read-only public/authenticated GitHub state. It does not call GitHub mutation APIs. |
| `pkg/hive/events.go` | Durable local/store evidence | Registers `hive.issuescan.review.capacity.throttled` with threshold, open PR count, source refs, and PR references. |
| `cmd/hive/factory_test.go` | Local validation evidence | Tests below threshold, at threshold, above threshold, unreadable queue, no fall-through to issue listing, parser mapping, and operator flag guards. |

## Conservative Queue Semantics

The throttle counts all open pull requests in the scanned repos as awaiting
exact-head human review. This is intentionally conservative.

The governed Transpara exact-head approval record is commonly a PR comment, not
only GitHub's review-decision field. A read-only `gh pr list` response can prove
that a PR is open, but it cannot prove that the current exact head has accepted
human approval. Therefore, open PRs are treated as unproven review-queue load.
This can over-throttle, but over-throttling is the safe direction because the
guard only prevents new work-start.

Future work may narrow this count to agent-authored or exact-head-unapproved
PRs only after a reviewed design names the evidence source and proves it can
read exact-head approval state without mutation.

## Work-Start Predicate

Let `threshold` be the configured positive integer and `open_pr_count` be the
read-only count of open pull requests in the scoped repos.

| Condition | Result |
|---|---|
| `threshold <= 0` | Configuration error; no work-start. |
| Review queue unreadable | Fail closed; no issue listing and no work-start. |
| `open_pr_count < threshold` | Continue to issue listing and existing PR-ready gates. |
| `open_pr_count >= threshold` | Record `hive.issuescan.review.capacity.throttled`; no issue listing and no work-start. |

This guard is not a readiness or approval signal. Passing the guard only means
review capacity is below threshold; the existing issue labels, source state,
dedupe, authority, review, and human approval gates still apply.

## Stop Conditions

The throttle stops before any issue-scan FactoryOrder can be queued when:

- `gh pr list` fails or returns unreadable JSON;
- the configured threshold is not positive;
- the conservative open PR count is at or above threshold;
- the store cannot record the throttle event for an at-capacity decision.

If recording fails, the command still refuses work-start and reports the record
failure to the operator.

## Event Shape

`hive.issuescan.review.capacity.throttled` records:

- `operator_id`
- `repos`
- `threshold`
- `open_pr_count`
- `reason`
- `source_refs`
- `pull_requests`
- `throttled_by`

The event is a local/store evidence record for the configured Hive context. This
PR does not execute the daemon and does not write production EventGraph truth.

## Non-Authorizations

This proposal and implementation do not authorize RuntimeBroker execution,
external adapter invocation, production EventGraph writes, Hive wake/start/action
API use, label mutation, issue mutation, PR approval, PR merge, deploy, service
restart, private fetch, protected settings changes, Test 001 GREEN, production
go-live, value allocation, autonomy increase, or wiki work.
