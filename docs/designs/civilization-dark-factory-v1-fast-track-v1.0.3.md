---
doc_id: HIVE-CIVILIZATION-DARK-FACTORY-V1-FAST-TRACK
title: Civilization and Dark Factory v1 fast-track design
doc_type: design-packet
status: approved
version: 1.0.3
created: 2026-08-04
updated: 2026-08-04
owner: Michael Saucier
steward: Codex/OpenAI
canonical: false
factory_order: FO-HIVE-CIVILIZATION-DARK-FACTORY-V1@1.0.0
---

# Civilization and Dark Factory v1 fast-track design

## Decision

Extend the existing Hive issue-scan launch, Work task, EventGraph store, Site
console, and ready-PR seams with one small v1 control loop. Do not replace the
substrates and do not reuse Operation #86 runtime state.

The accepted-order event is the durable queue. Work is the execution-contract
projection. EventGraph is the append-only state and evidence ledger. Hive owns
normalization, scheduling, reconciliation, and commands. Site owns the live
Human view and forwards bounded commands to Hive.

## Pinned source

- Factory Order:
  `docs/factory-orders/FO-hive-civilization-dark-factory-v1-v1.0.0.md`
  (`FO-HIVE-CIVILIZATION-DARK-FACTORY-V1`, v1.0.0; exact blob SHA is bound in
  IADA/CFADA evidence).
- Human source SHA-256:
  `c20bcfe317dd82a870893be4978991ba54725442a48739515807521fd08db38a`.
- Checkpoint:
  `CIVILIZATION-DARK-FACTORY-V1-FAST-TRACK-20260804T200316Z`.

## Existing seams reused

| Need | Existing seam |
| --- | --- |
| Durable causal store | EventGraph `store.Store` and Postgres `pgstore` |
| Work contract | `work.FactoryOrder`, `work.SeedFactoryOrder`, `work.TaskStore` |
| Issue discovery | daemon `factory_issue_scan_scanner.go` and `cc:pr-ready` filters |
| Generic launch record | `factory.run.requested` and run-launch API |
| PR creation/finalization | Hive issue-scan draft PR creator and ready PR finalizer patterns; Work GitHub client |
| Live projections | Hive operator API and Site `/console` projection client |
| Agent identities | existing Hive/Agent actor IDs and role vocabulary |
| Governance gates | installed `transpara-governance-gates` v0.12.14 content; exact artifacts bind to blobs/heads |

The old seven-stage issue-scan lifecycle remains readable historical and
compatibility evidence. The v1 loop adds the current TLC projection instead of
rewriting old events.

## Canonical FactoryOrder contract

All channels normalize into the same structured value and deterministic
markdown renderer:

```text
doc_id/version/status
title/channel/target_repository
immutable source references + source SHA-256
requirements (id, statement, rationale)
acceptance criteria (id, statement, verification_method, risk_class)
test plan
constraints/non-goals/expected outputs
authority scope and budget
```

The renderer uses stable field order and normalized line endings. The accepted
event stores the rendered markdown, its SHA-256, the structured fields, the
source channel, and the causal source event IDs. An accepted `(order_id,
version, document_sha256)` tuple is immutable. A conflicting tuple fails
closed; a byte-identical retry is idempotent.

Issue intake additionally claims the durable source identity
`github:<repository_full_name>#<issue_number>`. While its order is non-terminal,
an edited title or body is recorded as a blocked amendment against that order;
it cannot create a second accepted order. A new version requires an explicit
Human resolution after the prior order is terminal.

### Intake adapters

1. **Issue scan:** the existing interval scanner writes
   `factory.run.requested`; the v1 normalizer recognizes the pinned issue-scan
   brief, renders the selected issue into the canonical FactoryOrder, and
   appends one accepted-order event causally linked to the launch event.
2. **Human idea:** Hive appends idea and refinement events. Each response
   returns the full candidate and validation errors for Site to render. Submit
   requires a validated candidate plus explicit `approved=true`, then appends
   the accepted-order event.
3. **Completed FactoryOrder:** Hive strictly decodes the structured document,
   renders and validates it with the same function, and appends the
   accepted-order event directly.

Every acceptance also seeds exactly one Work FactoryOrder task and attaches
the exact markdown and source/hash receipt. Replay repairs a missing Work side
of an already accepted order without creating a second canonical order.
The accepted EventGraph event MUST commit before Work seeding. A Work task with
no matching accepted event is quarantined and requests intervention; it is
never promoted into canonical state from Work alone.

## Event model

The v1 package registers typed EventGraph content for:

```text
factory.v1.idea.recorded
factory.v1.idea.refined
factory.v1.order.accepted
factory.v1.stage.transitioned
factory.v1.intervention.requested
factory.v1.intervention.resolved
factory.v1.recovery.recorded
```

Stage evidence is stored on the transition event and duplicated as a Work task
artifact for operator/query compatibility. The event remains the ordering and
causal truth; Site never writes the graph directly.

## TLC state machine

The stage allowlist and order are versioned as `tlc-v1`:

| # | Stage | Default peers | Satisfied only when |
| ---: | --- | --- | --- |
| 1 | `ingest_work` | intake, archivist | accepted event and source hash resolve |
| 2 | `craft_factory_order` | planner, reviewer | canonical markdown validates and Work linkage exists |
| 3 | `design` | planner, reviewer, guardian | design artifact covers every requirement and AC |
| 4 | `iada` | reviewer | exact design blob has IADA blocker count zero |
| 5 | `cfada` | independent reviewer, guardian | exact design blob has cross-family CFADA blocker count zero |
| 6 | `human_design_review` | Human, guardian | scoped Human approval receipt exists |
| 7 | `write_code` | implementer, tester | deterministic branch/head and passing named validation exist |
| 8 | `create_draft_pr` | implementer, guardian | one open draft PR exists at the exact implementation head |
| 9 | `iar` | reviewer | exact PR head has IAR blocker count zero |
| 10 | `cfar` | independent reviewer, guardian | exact PR head has cross-family CFAR blocker count zero |
| 11 | `mark_pr_ready` | guardian, reviewer | same exact reviewed head is open, non-draft, and required checks pass |
| 12 | `human_review` | Human | order is visibly waiting for Human PR approval; no merge occurs |

Unknown stages and out-of-order success transitions fail closed. Blocked and
Human-required are states, not extra stages.

For the three acceptance-demo orders only, TLC stage 6 is satisfied by a
receipt that binds the pinned Human source SHA-256, this exact Factory Order
blob, the derived order ID/document hash, and the source sentence approving
the v1 product definition and implementation direction without repeated
design approval. This is an explicit standing approval already granted by the
Human, not a runner assertion. Any order outside those exact bindings requires
a new Human action. The separately required intervention exercises exceptional
resolution and does not fabricate or substitute for design approval.

## Durable scheduler and recovery

One loopback v1 daemon replays accepted orders and stage events, then runs a
bounded worker pool (default three). Each order has at most one local worker.
The durable attempt identity is:

```text
sha256("tlc-v1\x00" + order_document_sha256 + "\x00" + stage + "\x00" + decimal_ordinal)
```

Before an external stage, Hive appends `running` with the attempt identity.
After restart, a `running` attempt without a terminal transition is never
blindly repeated. Hive asks the runner to reconcile first:

- code: inspect the deterministic branch/worktree and exact commit;
- draft PR: query the deterministic head branch for an existing PR;
- reviews: match gate artifact to exact blob/head;
- readying: inspect PR draft state and head;
- all other stages: match the attempt receipt/evidence hash.

If the external effect already exists, recovery records it and completes the
same attempt. If it does not exist, the same attempt may run again. Conflicting
state blocks and requests Human intervention. This gives effect idempotency
without claiming an impossible distributed transaction with GitHub.

Budget counters are derived from terminal attempts and carried on every
projection. Exhaustion blocks the order; no later stage starts.
Orders in blocked or Human-required states release worker slots. For AC4,
`in_flight` means accepted and non-terminal, while `actively_executing` means a
worker currently owns a running attempt; evidence must show all three in
flight and all three actively executing during a positive overlap interval.

## Runner boundary

The scheduler invokes one configured executable with JSON stdin and accepts
strict JSON stdout. The request names operation `execute` or `reconcile`, the
order document/hash, stage, attempt identity, repository/worktree root, prior
evidence, authority scope, budget remaining, and assigned peers.

The result allowlist is `passed`, `blocked`, or `human_required` and includes
evidence references plus stage-specific exact fields. Runner stdout cannot
grant authority. Hive independently validates stage predicates and exact-head
relationships before appending success.

Fresh mode-0600 Hive configuration binds author and reviewer provider IDs,
family labels, executable realpaths and SHA-256 hashes, model IDs, and separate
credential-source identifiers. Stage configuration selects the provider;
runner output cannot select or override family identity. Hive records the
invoked binding and rejects a gate result whose exact artifact metadata does
not match it or whose author/reviewer lineage is not cross-family.

Fresh configuration also pins each GitHub push target by normalized repository
URL and expected `transpara-ai/<repository>` identity. Remote aliases are
convenience only and confer no authority. Before any push, Hive resolves the
alias, verifies the normalized URL and authenticated repository owner, and
fails closed on mismatch. This reconciles local naming conventions without
reading tracked loop configuration or permitting a fork/upstream surprise.

For the acceptance run, Codex/OpenAI is the author/implementer family and
Claude/Anthropic is the independent CFADA/CFAR family. IADA/IAR remain
self-directed evidence and never substitute for the cross-family gates.

## API and Mission Control projection

Hive adds bearer-protected loopback endpoints. The fresh configuration binds
the single demonstration operator credential to an immutable EventGraph actor
ID and display-neutral Human principal. Every Human command records that actor,
credential-key ID, request event, and returned event IDs; possession of the
service credential is not itself an approval outside the command's explicit
scope.

```text
GET  /api/hive/factory/v1/projection
POST /api/hive/factory/v1/ideas
POST /api/hive/factory/v1/ideas/{id}/refine
POST /api/hive/factory/v1/ideas/{id}/submit
POST /api/hive/factory/v1/orders
POST /api/hive/factory/v1/interventions/{id}/resolve
```

The projection includes `generated_at`, service/recovery identity, orders,
stage ledger, elapsed time, peers, gate state, evidence, blocker, next action,
budget, PR identity/head/check state, and the single open-intervention list.
Unknown or incomplete evidence renders unavailable/blocked, never healthy.

Site adds one Mission Control surface with the two Human intake forms,
all-order TLC cards, detail evidence, and intervention resolution. Site
forwards commands to Hive and immediately re-renders from the returned or
fresh canonical projection.

## Service topology

The local stack is loopback-only:

```text
PostgreSQL 16 (fresh data root/socket)
  <- EventGraph/Work shared store
  <- Hive v1 daemon (scanner + 3-worker scheduler)
  <- Hive Ops API :8083 (projection + commands)
  <- Work server :8080 (existing telemetry/task projection)
  <- Site :8088 (Mission Control)
```

A supervisor owns process IDs/logs and exact executable/config hashes. It uses
fresh mode-0600 runtime configuration and never reads the tracked
`hive/loop/config.env`. Restart stops only the fresh v1 processes and does not
touch retained Operation #86 stores.

## Executable acceptance harness

The harness must:

0. capture and compare the checkpoint remote commits/trees/PRs, tool versions,
   working trees, services, and Operation #86 audit/hash baseline before any
   mutation;
1. create or identify three bounded demo scopes and their URL-pinned repos;
2. start the persistent stack with the interval scanner;
3. submit and refine a Human idea at least twice, then approve it;
4. submit a prebuilt FactoryOrder;
5. show all three orders simultaneously `in_flight` and
   `actively_executing` across two projections with positive overlap;
6. surface and resolve one deliberately bounded intervention through Site/Hive;
7. restart the stack with work in flight and verify recovery/no duplicates;
8. wait for all orders to reach Human Review;
9. verify each exact PR head, required checks, IAR/CFAR, and non-draft state;
10. capture the live Site render and emit one final JSON report with all pins;
11. recompute the pre-run Operation #86 baseline and secret-safe diff audit.

The harness fails on any missing condition and revalidates Operation #86 hashes
afterward.

## Requirement trace

| Requirement | Design owner | Named verification |
| --- | --- | --- |
| R1 | daemon scanner + issue normalizer | `TestFactoryV1DaemonIssueScanAdmission` |
| R2 | idea/refinement API + Site form | `TestFactoryV1IdeaRefinementSubmission` |
| R3 | completed-order API | `TestFactoryV1CompletedOrderAdmission` |
| R4 | accepted event + Work repair | `TestFactoryV1AllChannelsCanonicalize` |
| R5 | three-worker scheduler | `TestFactoryV1SchedulesThreeConcurrently` |
| R6 | TLC allowlist/state validator | `TestFactoryV1TLCOrderAndEvidence` |
| R7 | Hive projection + Site rendering | `TestFactoryV1ProjectionHonesty` and Site `TestFactoryV1MissionControlStates` |
| R8 | intervention events/API | `TestFactoryV1InterventionResume` and Site `TestFactoryV1InterventionPOST` |
| R9 | reconcile-before-retry | `TestFactoryV1RestartRecoversWithoutDuplicateEffect` |
| R10 | PR terminal validator | `TestFactoryV1ReadyPRExactHead` |
| R11 | acceptance harness | post-run Operation #86 hash audit |
| R12 | supervisor/config | diff scan and mode/config-source assertions |

## Residual risks

- GitHub is an external system; the design provides deterministic
  reconciliation/idempotency, not transactional exactly-once semantics.
- The v1 runner supports one configured author family and one independent
  reviewer family for the demonstration. Additional providers are v2.
- A single scheduler instance is deliberate for v1. Multi-host leader election
  is v2; restart recovery is required now.
- Existing governance-package cache has one extra Python bytecode file while
  all canonical files are byte-identical. Do not claim full installed-manifest
  parity; gate artifacts bind to exact reviewed files/results.
- Potentially live tracked credentials exist outside this design. The v1 stack
  neither reads nor republishes them and uses fresh isolated configuration.

## Non-authorizations

This design does not authorize production, deployment, default-branch direct
push, branch-protection changes, evidence fabrication, credential reuse or
publication, merge of demonstration PRs, Operation #86 mutation, value
allocation, or residual-risk closure. Human Review remains the terminal v1
boundary for demonstration outputs.
