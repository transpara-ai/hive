---
doc_id: FO-HIVE-CIVILIZATION-DARK-FACTORY-V1
title: Functional Civilization and Dark Factory v1
doc_type: factory-order
status: approved
version: 1.0.0
created: 2026-08-04
updated: 2026-08-04
owner: Michael Saucier
steward: Codex/OpenAI
canonical: false
source_channel: human_request
source_content_sha256: c20bcfe317dd82a870893be4978991ba54725442a48739515807521fd08db38a
---

# Functional Civilization and Dark Factory v1

## Source and Human confirmation

This Factory Order is the structured reading of the Human request archived at:

```text
/home/transpara/.codex/attachments/dd7bf64a-c49a-4a33-86a4-09ec672a5a79/pasted-text-1.txt
SHA-256 c20bcfe317dd82a870893be4978991ba54725442a48739515807521fd08db38a
```

The source explicitly approves the v1 product definition and implementation
direction, authorizes non-production implementation and demonstration actions
inside this scope, and says not to return for repeated design or merge-by-merge
approval. It does not authorize production deployment, branch-protection
changes, false evidence, credential disclosure, or mutation of consumed
evidence.

## Intent

Deliver and operate one recoverable, non-production Civilization/Dark Factory
v1 on `ubunturecovery`. The normal successful output is an exact-head,
ready-to-approve pull request with passing required checks and inspectable
evidence.

## Requirements

### R1 — Continuous issue intake

The running stack must scan eligible issues across the configured core
`transpara-ai` repositories on an interval, apply the existing fail-closed
`cc:pr-ready` eligibility rules, select bounded work, and admit it without a
manual one-shot scan.

Rationale: issue intake is an operating capability, not an operator ritual.

### R2 — Human-idea refinement

The stack must accept a free-form Human idea, persist every refinement, show
the current candidate to the Human, validate it as a FactoryOrder, and submit
only the Human-approved candidate.

Rationale: unstructured intent must not silently become executable scope.

### R3 — Completed FactoryOrder intake

The stack must accept a prebuilt FactoryOrder, validate the same contract used
by refined ideas and issue intake, and durably admit it without another
refinement loop.

Rationale: callers that already have a valid order need a direct path.

### R4 — One canonical durable queue

All three channels must produce the same immutable `FactoryOrder` markdown
truth object and the same append-only accepted-order event. Work must contain a
linked task and artifact for that exact order and source hash.

Rationale: channel-specific queues create divergent recovery and authority
semantics.

### R5 — Concurrent bounded scheduling

The scheduler must hold at least three accepted orders in flight concurrently,
assign named collaborating peers, and preserve per-order authority, budget,
attempt, evidence, and recovery state.

Rationale: a factory must exploit independent work without mixing order scope.

### R6 — TLC execution

Every software-PR order must advance through the TLC sequence:

```text
ingest work -> craft Factory Order -> design -> IADA -> CFADA
-> Human Design Review -> write code -> create draft PR -> IAR -> CFAR
-> mark PR ready -> Human Review
```

Every completed stage must cite durable evidence. Cross-family gates must name
the author and reviewer families and bind to the exact design blob or PR head.

Rationale: ready-PR output without its governed trace is not a Dark Factory
result.

### R7 — Honest Mission Control

Site must show every accepted order, intake channel, current TLC stage, elapsed
time, assigned peers, gate state, evidence references, blocker, next action,
and one Human-intervention queue. It must visibly distinguish progressing,
blocked, and Human-required work and must fail closed on unavailable data.

Rationale: operator truth is a product requirement, not optional telemetry.

### R8 — Intervention path

The Hive API and Site must expose one bounded intervention-resolution path.
Resolution must append a Human-attributed event and cause the waiting order to
resume; it may not mutate stage state invisibly.

Rationale: exceptional Human action should be explicit and auditable.

### R9 — Restart recovery and idempotency

If the stack restarts with queued or in-flight orders, it must rebuild state
from EventGraph and Work, reconcile interrupted external stages before retry,
and avoid duplicate commits, branches, pull requests, ready transitions, or
stage completion events.

Rationale: restart survival is necessary for continuous operation.

### R10 — Ready-to-approve PR output

A successful order must end at Human Review with an open, non-draft PR whose
head equals the reviewed exact head, whose required checks pass, and whose
evidence remains inspectable. The factory must not merge demonstration output
PRs.

Rationale: the factory stops at a truthful Human approval boundary.

### R11 — Immutable prior evidence

Operation #86 series `operation-86-organic-v1-series-85cc6b4a0bb5` and every
consumed or failed series must remain unchanged and must not be reused as v1
runtime state.

Rationale: completed evidence is not a scratch database.

### R12 — Fresh isolated local configuration

The v1 stack must use fresh runtime state and secret-safe local configuration.
It must not copy or expose tracked credential-looking values from existing
configuration.

Rationale: the checkpoint audit identified potentially live tracked values
that are outside this Factory Order's permission to disclose or reuse.

## Acceptance criteria

| ID | Criterion | Verification method | Risk class |
| --- | --- | --- | --- |
| AC1 | One issue-scan order is admitted by a daemon interval, not the one-shot command. | Scanner log plus source event and accepted-order event with `channel=issue_scan`. | medium |
| AC2 | One Human idea has at least two visible persisted revisions before its approved FactoryOrder is submitted. | API/EventGraph revision history and captured Mission Control render. | medium |
| AC3 | One prebuilt valid FactoryOrder is accepted through the direct endpoint. | Endpoint receipt, document hash, accepted-order event, and Work task/artifact. | medium |
| AC4 | All three orders are simultaneously in flight for a nonzero interval. | Two timestamped projections bounding a positive overlap interval. | high |
| AC5 | Mission Control truthfully shows progress, blocked, and Human-required states. | Live API payload plus screenshot/captured render with matching event IDs. | high |
| AC6 | One deliberately bounded intervention is requested and resolved through Site/Hive. | Request and Human resolution event IDs plus resumed-stage evidence. | high |
| AC7 | A restart occurs during in-flight work and recovers without duplicate external effects. | Pre-stop/post-start projections, recovery events, idempotency reconciliation, unique branch/PR/stage receipts. | critical |
| AC8 | Every order reaches each required TLC stage and stops at Human Review. | Per-order ordered stage ledger with exact evidence references. | high |
| AC9 | Every order produces an exact-head ready-to-approve PR with passing required checks. | GitHub PR/head/check reads and exact-head IAR/CFAR evidence. | critical |
| AC10 | Final evidence pins commits, trees, PR heads, tests, EventGraph/Work records, service state, and live console renders. | Machine-readable final acceptance report plus referenced files. | high |
| AC11 | Operation #86 evidence is unchanged. | Recompute checkpoint audit SHA and all referenced hashes after the demo. | critical |
| AC12 | No tracked credential-looking value is copied into v1 config/evidence. | Secret-safe config review and repository diff scan without printing secret values. | critical |

## Constraints

- Non-production and loopback-only.
- Use `origin`, never push to `upstream`.
- Preserve EventGraph hash-chain and Work event-sourcing invariants.
- Site renders and submits commands; Hive owns orchestration and writes.
- Use deterministic branch/order/idempotency identities for recovery.
- Do not disable required checks or branch protection.
- Do not mark a failed gate as passing or fabricate reviewer independence.
- Implementation PRs may merge through approved exact-head paths; three
  demonstration PRs remain ready for Human approval.

## Non-goals

- Production deployment or `CanOperate` expansion.
- Merge or deployment of demonstration outputs.
- Doctrine, taxonomy, historical reconciliation, broad refactors, or UI polish
  beyond the acceptance demonstration.
- Reuse, reset, or modification of Operation #86 or other consumed evidence.
- Credential rotation or publication of existing tracked credentials; those
  require a separate secure owner workflow.

## Required outputs

1. Executable acceptance tests and a versioned FactoryOrder/TLC projection
   contract.
2. A recoverable local Hive/Work/EventGraph/PostgreSQL stack and supervisor.
3. Three normalized intake adapters and a three-worker durable scheduler.
4. A TLC-to-ready-PR runner with exact-head evidence and reconciliation.
5. A live Site Mission Control surface and intervention endpoint.
6. Three ready-to-approve demonstration PRs.
7. A final machine-readable evidence report and captured live console renders.

