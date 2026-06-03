---
doc_id: HIVE-DF-EPIC-003-GUARDRAIL-INVENTORY
title: Hive Dark Factory Epic 3 Guardrail Inventory
doc_type: implementation-evidence
status: draft
version: 1.1.0
created: 2026-05-28
updated: 2026-06-03
owner: human
steward: assistant
project: dark-factory
canonical: false
---

# Hive Dark Factory Epic 3 Guardrail Inventory

Source packet:
- `transpara-ai/docs` `dark-factory/v3.9/implementation/epics/epic-03-hive-governance-reconciliation/`

Reviewed packet SHA:
- `85588f61ef7e5f356ef161d640b00568c6a122eb`

Docs merge commit:
- `7d31062a74428a128dc611ff45f30b92d42fa7fe`

Implementation repo:
- `transpara-ai/hive`

Implementation branch:
- `codex/epic-3-hive-governance-reconciliation`

Hive base state:
- `main` and `origin/main` were reconciled at `d55ff8869d500cfb2c8621b7540c78f7d7fd02ae`.
- Live GitHub checks found no open `transpara-ai/hive`, `transpara-ai/eventgraph`, or `transpara-ai/docs` PR overlapping Epic 3 before implementation.
- Recent merged Hive guardrail history was reconciled through `transpara-ai/hive#123`.

Gate D disposition:
- This inventory alone did not satisfy Gate D.
- The subsequent Epic 3 closeout recorded completed local validation, PR check evidence, and adversarial review for `transpara-ai/hive#124`; at that time, Dark Factory docs marked Gate D satisfied only for the bounded Epic 3 Hive governance reconciliation, with R-001/R-002/R-003 carried forward.
- At the Epic 3 closeout, Epic 4 and Gate E remained out of scope and were not authorized by this artifact.
- Later Gate E through Gate J closures belong to their later bounded packets and implementation PRs; this artifact does not authorize or broaden them.

Epic 10 residual-risk closure update:
- Source packet: `transpara-ai/docs#93`, merged at `e34cc184c0c90873a5e7665d80f8cd7dd088d4b0`, selected only a bounded `transpara-ai/hive` local-emulation evidence seam for R-001/R-002/R-003.
- Implementation branch: `codex/epic-10-hive-residual-risk-closure`.
- The update closes R-001/R-002/R-003 only for the selected Hive side-effect-free local emulation seam. It does not authorize production deploy, default-branch push, worktree merge to main, live PR mutation, branch push, global activation, auto-merge, secret access, upstream push, or any other real protected side effect.
- Real runner/worktree protected side-effect paths remain blocked by default and still require separate authorization before any production or repository-mutating execution path is introduced.

## Scope Boundary

Primary implementation repo:
- `transpara-ai/hive`

Allowed supporting repos:
- `transpara-ai/eventgraph`
- `transpara-ai/docs`

Support was not required in this branch. No EventGraph or docs code changes were made.

Read-only repos:
- `transpara-ai/work`
- `transpara-ai/site`
- `transpara-ai/agent`

No Work, Site, or Agent files were modified.

## Implementation Summary

Hive now carries the full v3.9 protected action vocabulary in `pkg/safety/safety.go`, including:

```text
agent.key.rotate
release.certify
capability.promote
capability.activate
capability.rollback
runtime.invoke.external
memory.ingest.sensitive
knowledge.activate
```

All known protected actions require approval by default. Unknown protected actions fail closed with `Forbidden`.

Authority request evidence was tightened in two places:

- CLI blocked paths with a configured authority audit store emit both the canonical EventGraph `authority.requested` event and the Hive Phase 3 `authority.request.recorded` detail event with requesting role, risk class, scope, proposed operation, and causal events.
- Runtime authority policy paths record authority requests and decisions with requesting role, decider role, risk class, and scope.

Operator projections remain EventGraph-read-only. They read authority request and decision records and expose bounded projection output without appending or recording EventGraph events.

Original Epic 3 policy engine adapter evidence:
- `PolicyEngineAdapterDecision` was not applicable in the Epic 3 branch because the Hive paths changed there were code-level default gates and authority evidence recorders, not policy-adapter-mediated decisions.
- If a future Hive path delegates authority decisions to a policy adapter, it must use the canonical v3.9 adapter evidence chain with real adapter and policy bundle identifiers.

Original Epic 3 execution receipt evidence:
- `authority.execution.receipt` remains registered as a Phase 3 record type.
- No approved protected side-effect execution path was added in Epic 3.
- Real protected-action execution remains out of scope. No ceremonial execution receipt was emitted.

Epic 10 evidence update:
- `pkg/hive/phase3_records.go` registers `policy.engine.adapter.decision` with the Dark Factory v3.9 `PolicyEngineAdapterDecision` field vocabulary: adapter ID/version, policy bundle ID/hash, protected action type, actor, resources, input facts, raw/canonical decision, reason codes, evidence refs, latency, and authority-decision link.
- `pkg/hive/authority_policy.go` adds a side-effect-free local emulation seam for `repo.merge.main` and `repo.push.default_branch`. The seam records authority request evidence, requires an approved authority decision, records policy-adapter evidence with a matching policy bundle hash, and emits `authority.execution.receipt` only after approved local emulation.
- Negative trials block and produce no receipt for missing audit/graph dependencies, missing policy adapter evidence, missing policy bundle evidence, stale or mismatched policy bundle hash, forbidden policy decision, missing authority decision, cross-action mismatch, receipt without local emulation, real default-branch push, real worktree merge to main, and production deploy.
- The direct runner push and worktree merge functions were not converted into production execution paths. They still block through `safety.RequireAuthorized`.

## Search Attestation

Searches run from `transpara-ai/hive`:

```text
rg -n "RequireAuthorized|DefaultOutcome|ApprovalAllowsAction|IsProtectedAction|authorizeProtectedAction" cmd pkg docs/dark-factory -g'*.go' -g'*.md'
rg -n "EventTypeAuthorityRequestRecorded|EventTypeAuthorityDecisionRecorded|EventTypePolicyEngineAdapterDecision|EventTypeAuthorityExecutionReceipt|AuthorityRequestRecordedContent|AuthorityDecisionRecordedContent|PolicyEngineAdapterDecisionContent|AuthorityExecutionReceiptContent" cmd pkg docs/dark-factory -g'*.go' -g'*.md'
rg -n "\.blocked|blocked action=|proposal mode|ProposalMode|builderProposalMode|PR proposal|operator projection|BuildOperatorProjection|NewOperatorProjectionServer|store\.Append|graph\.Record" cmd pkg docs/dark-factory -g'*.go' -g'*.md'
gh pr list --repo transpara-ai/hive --state merged --limit 30 --json number,title,mergedAt,headRefName,mergeCommit
```

Results:
- Protected-action vocabulary and default outcomes are centralized in `pkg/safety/safety.go`.
- Runtime authority gates are centralized around `pkg/hive/authority_policy.go`.
- CLI blocked paths are in `cmd/hive/main.go` and authority audit emission is in `cmd/hive/authority_audit.go`.
- Direct runner and worktree blocked paths remain in `pkg/runner/runner.go` and `pkg/runner/worktree.go`.
- Phase 3 authority record types and content are in `pkg/hive/phase3_records.go`.
- Epic 10 local-emulation evidence is in `pkg/hive/authority_policy.go`, `pkg/hive/phase3_records.go`, and `pkg/hive/authority_policy_emulation_test.go`.
- Operator projection code in `pkg/hive/operator_projection.go` reads from EventGraph store APIs and does not call `store.Append` or `graph.Record`.
- All search hits are represented in the inventory below. Ambiguous or non-ingested blocking paths are classified as `Temporary out-of-band accepted risk`.

## Guardrail Inventory

| ID | Guardrail | Source evidence | Current behavior | Protected action(s) | Risk class | Current authority evidence | Classification | Target evidence mapping | Required change | Validation | Residual risk / owner / reason / expiry / bounded closeout / next action / operator consequence | Status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| G-001 | Worktree main merge gate | `pkg/runner/worktree.go`, `pkg/runner/pipeline_state_test.go`, Hive `#109` | Blocks worktree merge to main by default through `safety.RequireAuthorized`. | `repo.merge.main` | critical | Blocked log and returned `safety.AuthorityError`; no EventGraph audit store is available at this runner boundary. | Temporary out-of-band accepted risk | AuthorityRequest: not emitted at this boundary; AuthorityDecision: none; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | None in Epic 3; protected side effect remains blocked. | `TestProposalModeSkipsWorktreeMergeOnCriticPass`; full validation passed. | Risk R-001. Owner: Hive. Reason: runner worktree path has no audit store dependency in scope. Revisit before enabling direct worktree protected execution or Epic 4. Bounded closeout: path must stay blocked by default. Next action: add audit-store plumbing before any approval flow. Operator consequence: merge is denied, not executed. | accepted risk |
| G-002 | Direct runner default-branch push gate | `pkg/runner/runner.go`, `pkg/runner/runner_test.go`, Hive `#110` | Blocks direct default-branch push by default unless proposal mode avoids the push path. | `repo.push.default_branch` | critical | Blocked log and returned `safety.AuthorityError`; no EventGraph audit store is available at this runner boundary. | Temporary out-of-band accepted risk | AuthorityRequest: not emitted at this boundary; AuthorityDecision: none; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | None in Epic 3; protected side effect remains blocked. | Runner push-block tests; full validation passed. | Risk R-001. Owner: Hive. Reason: runner push path has no audit store dependency in scope. Revisit before enabling direct push or Epic 4. Bounded closeout: path must stay blocked by default. Next action: add audit-store plumbing before any approval flow. Operator consequence: push is denied, not executed. | accepted risk |
| G-003 | Final pipeline sweep and legacy deploy neutralization | `cmd/hive/main.go`, `cmd/hive/pipeline_safety_test.go`, `cmd/hive/authority_audit_test.go`, Hive `#111` | Multi-repo final sweep is blocked by default. Legacy external deploy remains neutralized by log-only runtime target wording. | `repo.mutate.cross_repo`, `production.deploy` | critical | With an audit store, emits canonical `authority.requested` and Hive `authority.request.recorded`; without a store, logs and blocks. | EventGraph-ingested | AuthorityRequest: `authority.requested`; AuthorityDecision: none for denied path; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | Add Hive detail event for CLI emitter. | `TestAuthorizeFinalPipelineSweepBlocksRepoMapByDefault`; `TestAuthorizeFinalPipelineSweepEmitsAuthorityRequest`; full validation passed. | Audit detail exists only when an authority audit store is configured. Operator consequence without store: denial is still local and blocking. | implemented |
| G-004 | Ingest repo bootstrap gate | `cmd/hive/main.go`, `cmd/hive/ingest_test.go`, `cmd/hive/authority_audit_test.go`, Hive `#112` | Repo create and default-branch bootstrap push are blocked before GitHub/API side effects. | `repo.create`, `repo.push.default_branch` | high, critical | With an audit store, emits canonical `authority.requested` and Hive `authority.request.recorded`; without a store, logs and blocks. | EventGraph-ingested | AuthorityRequest: `authority.requested`; AuthorityDecision: none for denied path; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | Add Hive detail event for CLI emitter. | `TestRunIngestBlocksRepoBootstrapByDefault`; `TestAuthorizeIngestRepoBootstrapBlocksProtectedActionsByDefault`; `TestAuthorizeIngestRepoBootstrapEmitsAuthorityRequests`; full validation passed. | Audit detail exists only when an authority audit store is configured. Operator consequence without store: denial is still local and blocking. | implemented |
| G-005 | Blocked-path authority request emission | `cmd/hive/authority_audit.go`, `cmd/hive/authority_audit_test.go`, Hive `#113` | Authority audit emitter records canonical request plus Hive Phase 3 request detail. | varies by caller | high or critical | `authority.requested` and `authority.request.recorded` in EventGraph store. | EventGraph-ingested | AuthorityRequest: canonical plus Hive detail; AuthorityDecision: none for denied CLI path; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | Register Hive Phase 3 content with the CLI emitter and append detail event. | `TestAuthorityAuditEmitterRecordsAuthorityRequested`; full validation passed. | None for configured audit store. | implemented |
| G-006 | Default builder proposal mode | `cmd/hive/router.go`, `pkg/runner/runner.go`, `pkg/runner/runner_test.go`, `pkg/runner/pipeline_state_test.go`, Hive `#114` | Proposal mode is default and writes local proposal artifacts instead of pushing, merging, deploying, or globally activating. | Protected actions avoided: `repo.push.default_branch`, `repo.merge.main`, `production.deploy` | critical | No authority evidence is emitted because no protected action is attempted in proposal mode. | Out of scope | AuthorityRequest: not applicable; AuthorityDecision: not applicable; ExecutionReceipt: not applicable; PolicyEngineAdapterDecision: not applicable. | None. | `TestPipelineFlagsDefaultToProposalMode`; `TestRoleFlagsDefaultBuilderToProposalMode`; `TestDefaultBuilderProposalModeCommitsBranchAndWritesArtifact`; full validation passed. | None. Operator consequence: proposal artifact tells the operator that push, remote PR creation, deployment, or merge requires explicit approval. | verified by inspection |
| G-007 | Approval audit rationale documentation | `docs/dark-factory/authority-vocabulary.md`, Hive `#115` | Documents canonical protected action vocabulary and blocked-path logging expectations. | all protected action names | high or critical | Documentation only; no runtime authority event. | Out of scope | AuthorityRequest: not applicable; AuthorityDecision: not applicable; ExecutionReceipt: not applicable; PolicyEngineAdapterDecision: not applicable. | Update to the full v3.9 vocabulary. | `git diff --check`; full validation passed. | None. | implemented |
| G-008 | Authority vocabulary alignment | `pkg/safety/safety.go`, `pkg/safety/safety_test.go`, Hive `#116` | Full v3.9 protected action vocabulary is present; known actions require approval; unknown actions are forbidden. | all 25 v3.9 protected actions | high or critical | Code-level policy default, not an event emitter. | Superseded | Replaces earlier incomplete 17-action Hive vocabulary with full v3.9 baseline. | Add 8 missing constants, risk class mapping, tests, and docs. | `TestProtectedActionsMatchDFSOPVocabulary`; `TestHighRiskEpic3ActionsRequireApproval`; `TestDefaultOutcomeFailsClosedForUnknownActions`; full validation passed. | None. | implemented |
| G-009 | Agent identity key provenance | `pkg/hive/identity.go`, `pkg/hive/runtime.go`, `pkg/hive/operator_projection.go`, Hive `#117` | Agent identity and key provenance records are registered and projected. No key rotation execution path is introduced. | `agent.spawn.persistent`, `agent.key.rotate`, `secret.access` | high, critical | Identity/key provenance events are EventGraph records; key rotation protected action vocabulary now exists and requires approval by default. | EventGraph-ingested | AuthorityRequest: via `authorizeProtectedAction` when selected; AuthorityDecision: via `authorizeProtectedAction` when approved; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | Add missing `agent.key.rotate` vocabulary and request detail fields. | Identity/operator projection tests plus authority policy tests; full validation passed. | No key rotation execution path exists in Epic 3. Operator consequence: unknown or selected rotation action is denied unless explicitly approved through a future authority path. | implemented |
| G-010 | Phase 3 EventGraph records | `pkg/hive/phase3_records.go`, `pkg/hive/phase3_records_test.go`, Hive `#118` | Authority request, decision, and execution receipt content types are registered. Request and decision content now carry role, risk class, and scope. | all authority-mediated Hive actions | high or critical | EventGraph content registration and runtime record content. | EventGraph-ingested | AuthorityRequest: `authority.request.recorded`; AuthorityDecision: `authority.decision.recorded`; ExecutionReceipt: registered but not emitted in Epic 3; PolicyEngineAdapterDecision: not applicable. | Add request/decision metadata fields and tests. | `TestPhase3RecordTypesRoundTrip`; full validation passed. | Execution receipt remains unused until a safe approved local side-effect path exists. | implemented |
| G-011 | Authority policy gates | `pkg/hive/authority_policy.go`, `pkg/hive/authority_policy_test.go`, Hive `#119` | Runtime protected-action checks fail closed, record request evidence, and record decision evidence only when approval is selected. Cross-action approvals do not leak. | all authority-mediated Hive actions | high or critical | Canonical `authority.requested`, Hive `authority.request.recorded`, and Hive `authority.decision.recorded`. | EventGraph-ingested | AuthorityRequest: canonical plus Hive detail; AuthorityDecision: Hive detail for approved local test path; ExecutionReceipt: none; PolicyEngineAdapterDecision: not applicable. | Add risk class, scope, requesting role, and decider role to evidence. | `TestAuthorizeProtectedActionRecordsRequestAndBlocksWithoutApproval`; `TestAuthorizeProtectedActionWithAutoApprovalRecordsDecision`; `TestLifecycleApprovalDoesNotAuthorizeSelfModificationRuntime`; full validation passed. | No execution path added. Operator consequence: approval evidence can exist without real protected side effect execution. | implemented |
| G-012 | Operator projections | `pkg/hive/operator_projection.go`, `pkg/hive/operator_projection_test.go`, `pkg/hive/operator_api.go`, Hive `#120` | Reads bounded authority state and lifecycle/key audit records for operators. | read-only display of authority requests and decisions | medium | Reads EventGraph store records only. No EventGraph mutation surface was found. | Out of scope | AuthorityRequest: read-only display; AuthorityDecision: read-only display; ExecutionReceipt: not displayed here; PolicyEngineAdapterDecision: not applicable. | Add role, risk class, and scope to projection output. | `TestBuildOperatorProjectionPendingAndDecisions`; `TestBuildOperatorProjectionLifecycleAndKeyAudit`; full validation passed. | None. Operator consequence: projection is informational and cannot approve or execute. | implemented |
| G-013 | Model/provider routing | model catalog and resolver paths, Hive `#121` | Routes model/provider choice for agents; search found no protected-action execution or authority decision surface in this branch. | `runtime.invoke.external` only if future runtime invocation becomes authority-bearing | critical | None because current routing does not execute a protected external runtime invocation as an authority decision. | Out of scope | AuthorityRequest: not applicable; AuthorityDecision: not applicable; ExecutionReceipt: not applicable; PolicyEngineAdapterDecision: not applicable. | Add `runtime.invoke.external` vocabulary so future paths fail closed if selected. | `TestHighRiskEpic3ActionsRequireApproval`; full validation passed. | None in current routing. Revisit if routing starts invoking external runtime services as a protected action. | verified by inspection |
| G-014 | Canonical soul statement | Agent docs, Hive `#123` | Governance/culture statement only; no authority gate, protected side effect, or operator projection mutation surface. | none | n/a | None. | Out of scope | AuthorityRequest: not applicable; AuthorityDecision: not applicable; ExecutionReceipt: not applicable; PolicyEngineAdapterDecision: not applicable. | None. | Live PR history and file search; full validation passed. | None. | verified by inspection |
| G-015 | Epic 10 local protected-action emulation seam | `pkg/hive/authority_policy.go`, `pkg/hive/phase3_records.go`, `pkg/hive/authority_policy_emulation_test.go`, docs `#93` | Records a side-effect-free local emulation for `repo.merge.main` or `repo.push.default_branch` only after policy-bundle evidence, approved authority decision, and local-emulation mode are present. Blocks every missing, stale, forbidden, cross-action, real-push, real-merge, or deploy path before a receipt. | `repo.merge.main`, `repo.push.default_branch` | critical | Canonical `authority.requested`, Hive `authority.request.recorded`, Hive `authority.decision.recorded`, Hive `policy.engine.adapter.decision`, and Hive `authority.execution.receipt` for the approved local-emulation path only. | EventGraph-ingested local emulation evidence | AuthorityRequest: canonical plus Hive detail; AuthorityDecision: Hive detail; PolicyEngineAdapterDecision: Hive Phase 3 content with v3.9-compatible fields and policy bundle ID/hash; ExecutionReceipt: emitted only for approved side-effect-free local emulation. | Add bounded local emulation seam and focused positive/negative trials. | `TestRecordProtectedActionLocalEmulationReceiptsApprovedPolicyPath`; `TestProtectedActionLocalEmulationNegativeTrialsBlockReceipts`; `TestProtectedActionLocalEmulationMissingDependenciesBlock`; `TestRegisterEventTypesIncludesPhase3Unmarshalers`. | R-001/R-002/R-003 closed only for this local emulation seam. Operator consequence: approved local emulation records auditable evidence and receipt; real protected side effects remain blocked and separately unauthorized. | implemented |

## Protected Action Coverage

| Protected action | Present in Hive vocabulary | Current Hive source | Guardrail IDs | Required outcome | Test evidence | Classification or exclusion |
| --- | --- | --- | --- | --- | --- | --- |
| `production.deploy` | yes | `pkg/safety/safety.go` | G-003, G-006 | approval required; no deploy execution | `TestProtectedActionsMatchDFSOPVocabulary` | covered; deploy remains neutralized |
| `repo.push.default_branch` | yes | `pkg/safety/safety.go`, `pkg/hive/authority_policy.go` | G-002, G-004, G-006, G-015 | approval required; real push blocked by default; side-effect-free local emulation can be receipted with policy and authority evidence | safety tests, runner tests, ingest tests, Epic 10 local-emulation tests | real side effect blocked; bounded local emulation evidence implemented |
| `repo.merge.main` | yes | `pkg/safety/safety.go`, `pkg/hive/authority_policy.go` | G-001, G-006, G-015 | approval required; real merge blocked by default; side-effect-free local emulation can be receipted with policy and authority evidence | safety tests, pipeline proposal tests, Epic 10 local-emulation tests | real side effect blocked; bounded local emulation evidence implemented |
| `repo.create` | yes | `pkg/safety/safety.go` | G-004 | approval required; blocked by default | ingest and authority audit tests | EventGraph-ingested when audit store configured |
| `repo.delete` | yes | `pkg/safety/safety.go` | G-008 | approval required; no Hive execution path found | safety tests | vocabulary covered |
| `repo.mutate.cross_repo` | yes | `pkg/safety/safety.go` | G-003 | approval required; blocked by default | pipeline and authority audit tests | EventGraph-ingested when audit store configured |
| `self_modification.activate` | yes | `pkg/safety/safety.go` | G-008, G-011 | approval required; cross-action approval blocked | safety and authority policy tests | covered |
| `secret.access` | yes | `pkg/safety/safety.go` | G-009 | approval required; no new secret access path | safety tests | vocabulary covered |
| `policy.change` | yes | `pkg/safety/safety.go` | G-008 | approval required; no policy adapter path added | safety tests | vocabulary covered |
| `agent.escalate_permissions` | yes | `pkg/safety/safety.go` | G-008, G-011 | approval required | safety tests | vocabulary covered |
| `agent.spawn.persistent` | yes | `pkg/safety/safety.go` | G-009, G-011 | approval required; blocks dynamic spawn without approval | authority policy tests | EventGraph-ingested |
| `agent.retire` | yes | `pkg/safety/safety.go` | G-011 | approval required; approved local test records decision | authority policy tests | EventGraph-ingested |
| `agent.revoke` | yes | `pkg/safety/safety.go` | G-011 | approval required | safety tests | vocabulary covered |
| `agent.key.rotate` | yes | `pkg/safety/safety.go` | G-008, G-009 | approval required; no execution path added | safety tests | vocabulary added; no execution path |
| `external_communication.company_voice` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary covered |
| `data.delete` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary covered |
| `billing.spend_above_threshold` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary covered |
| `license.change` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary covered |
| `release.certify` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary added |
| `capability.promote` | yes | `pkg/safety/safety.go` | G-008 | approval required; cross-action approval blocked | safety tests | vocabulary added |
| `capability.activate` | yes | `pkg/safety/safety.go` | G-008 | approval required; cross-action approval blocked | safety tests | vocabulary added |
| `capability.rollback` | yes | `pkg/safety/safety.go` | G-008 | approval required | safety tests | vocabulary added |
| `runtime.invoke.external` | yes | `pkg/safety/safety.go` | G-008, G-013 | approval required; no execution path found | safety and authority policy tests | vocabulary added |
| `memory.ingest.sensitive` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary added |
| `knowledge.activate` | yes | `pkg/safety/safety.go` | G-008 | approval required; no execution path found | safety tests | vocabulary added |

## Residual Risks

### R-001: Runner/Worktree Blocks Are Local Out-of-Band Evidence

Affected repo:
- `transpara-ai/hive`

Affected protected actions:
- `repo.merge.main`
- `repo.push.default_branch`

Classification:
- Closed only for the Epic 10 side-effect-free local emulation seam.
- Real direct runner push and worktree merge execution remain blocked and separately unauthorized.

Reason:
- Epic 10 adds an explicit local emulation request path for `repo.merge.main` and `repo.push.default_branch` through the Hive runtime authority policy.
- The local emulation path records store-backed authority request evidence and links it to policy-adapter evidence, authority decision evidence, and a receipt.
- The existing runner/worktree production side-effect functions were not enabled and still block through `safety.RequireAuthorized`.

Owner:
- Hive

Expiry or revisit trigger:
- Before any approval-enabled direct runner push, worktree main merge, branch push, live PR mutation, deploy, or other real protected side effect.

Bounded closeout condition:
- The selected local emulation seam must continue to record authority evidence without repository mutation.
- Real runner/worktree protected side-effect paths must continue to block by default.

Next action:
- For any future real execution path, create a separate authorization packet and route the concrete runner/worktree side-effect boundary through policy and authority evidence before enabling execution.

Operator consequence:
- Approved local emulation produces inspectable evidence. Real default-branch push or worktree merge still returns a local authority error and does not mutate the repository.

Blocks future production execution:
- Yes, if that future scope needs a real protected repository mutation.
- No, for this bounded local emulation evidence seam.

### R-002: ExecutionReceipt Has No Real Approved Side-Effect-Free Path In Epic 3

Affected repo:
- `transpara-ai/hive`

Affected protected actions:
- All approved protected actions that would execute a side effect.

Classification:
- Closed only for the Epic 10 side-effect-free local emulation seam.
- Real protected-action execution remains separately unauthorized.

Reason:
- Epic 10 emits `authority.execution.receipt` only after approved side-effect-free local emulation with request, decision, policy adapter, and policy bundle evidence.
- The negative trials reject receipt creation for missing dependency, missing policy evidence, missing or stale policy bundle hash, forbidden policy, missing authority decision, cross-action mismatch, and non-emulation modes.

Owner:
- Hive

Expiry or revisit trigger:
- Before any approved protected action execution path is introduced.

Bounded closeout condition:
- A receipt is valid only when tied to the approved Epic 10 local emulation path or to a separately authorized future execution boundary.

Next action:
- Add a separate packet before any receipt-bearing real side-effect execution.

Operator consequence:
- Operators can treat an Epic 10 receipt as evidence that local emulation ran without repository mutation. They must not treat it as evidence that a real push, merge, deploy, or live PR mutation executed.

Blocks future production execution:
- Yes, if that future scope includes real protected execution without a separately authorized receipt-bearing path.

### R-003: PolicyEngineAdapterDecision Is Not Used By Current Hive Gates

Affected repo:
- `transpara-ai/hive`

Affected protected actions:
- Any future policy-adapter-mediated protected action.

Classification:
- Closed only for the Epic 10 side-effect-free local emulation seam.
- Future policy-adapter-mediated production decisions remain separately unauthorized.

Reason:
- Epic 10 records `policy.engine.adapter.decision` with `adapter_id`, `adapter_version`, `policy_bundle_id`, `policy_bundle_hash`, `protected_action_type`, `actor_id`, `resource_refs`, `input_facts`, `raw_decision`, `canonical_decision`, `reason_codes`, `evidence_refs`, and `authority_decision_ref`.
- The implementation rejects missing policy adapter evidence, missing policy bundle ID/hash, stale or mismatched policy bundle hash, forbidden canonical decision, and cross-action mismatch.

Owner:
- Hive for the local emulation seam.
- Hive and EventGraph, if a future shared production adapter contract becomes necessary.

Expiry or revisit trigger:
- Before introducing policy-adapter-mediated real protected side effects, production decisions, or global activation.

Bounded closeout condition:
- The Epic 10 local emulation seam may claim policy-bundle evidence only when the adapter ID/version and policy bundle ID/hash are present and hash-matched.
- No other Hive path may claim policy-bundle evidence without its own authorized adapter evidence.

Next action:
- For production policy-adapter use, create a separate authorization packet and connect the real adapter and policy bundle to the target side-effect boundary.

Operator consequence:
- Operators receive policy-adapter evidence for the bounded local emulation seam. They do not receive production policy-adapter authorization from this branch.

Blocks future production execution:
- Yes, if that future scope relies on policy adapter decisions without a separately authorized production adapter path.

## Validation Evidence

Epic 3 targeted validation:

```text
go test ./cmd/hive ./pkg/safety ./pkg/hive
```

Result:

```text
ok  	github.com/transpara-ai/hive/cmd/hive
ok  	github.com/transpara-ai/hive/pkg/safety
ok  	github.com/transpara-ai/hive/pkg/hive
```

Epic 10 local-emulation validation run from `codex/epic-10-hive-residual-risk-closure`:

```text
git diff --check
go test ./cmd/hive ./pkg/safety ./pkg/hive ./pkg/runner
go test ./...
/home/transpara/go/bin/staticcheck ./...
make verify
```

Results:

```text
git diff --check: passed
go test ./cmd/hive ./pkg/safety ./pkg/hive ./pkg/runner: passed
go test ./...: passed
/home/transpara/go/bin/staticcheck ./...: passed
make verify: passed
```

Subsequent closeout recorded completed PR check evidence and adversarial review for `transpara-ai/hive#124`. Gate D was satisfied only for the bounded Epic 3 Hive governance reconciliation, with R-001/R-002/R-003 carried forward at that time. The Epic 10 update above closes those residuals only for the selected side-effect-free local emulation seam; broader docs tracker and checkpoint reconciliation remain a later docs closeout step.
