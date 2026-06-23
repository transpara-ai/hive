# INC-001 Hive Runtime Missing-Evidence Finding

## Purpose

This packet records the Hive-side runtime and deployment evidence posture for
the Test 001 cross-repo runtime-doctrine drift tabletop tracked by
`transpara-ai/operation`.

It is a missing-evidence finding, not a runtime observation, deployment record,
operator projection capture, EventGraph export, authority artifact, source-repo
adoption claim, or incident closure artifact.

## Finding

```text
finding_id: inc-001-hive-runtime-missing-evidence-2026-06-18
incident: INC-001 / Test 001 Cross-Repo Runtime-Doctrine Drift Tabletop
runtime_repo: transpara-ai/hive
runtime_evidence_status: MISSING_RUNTIME_EVIDENCE_ACCEPTED
runtime_evidence_status_meaning: missing incident-specific runtime or deployment evidence recorded, not runtime proof or authority signoff
source_commit: e0541dc7e765cb0f98696533b7d1ac8516ab8194
correction_type: NO_CHANGE
human_authorization_required: no
human_authorization_evidence: none
```

Hive has source-defined runtime commands, EventGraph-backed runtime event types,
and a read-only operator projection contract that can distinguish queued launch
intent from observed runtime events. This packet does not cite an actual Hive
process, environment, deployment identifier, store DSN, EventGraph store export,
runtime projection response, incident-specific event IDs, authority decision, or
production observation for INC-001.

The `human_authorization_required` value above is scoped only to recording this
missing-evidence finding. It does not grant authority to start Hive, approve a
protected action, change runtime behavior, deploy to production, publish a
correction, or close INC-001 as `GREEN`.

## Surfaces Reviewed

| Surface | Class | Source anchors at `source_commit` | Finding |
| --- | --- | --- | --- |
| Hive runtime ownership and UI boundary | `RUNTIME_CONTRACT`, `UI_BOUNDARY` | `README.md:13-35` | Hive owns runtime orchestration, diagnostics, events, and projections, while Site owns browser UI rendering; quick-start commands exist, but no incident-specific command execution or deployment proof is cited here. |
| Operator UI contract frontmatter and endpoint list | `RUNTIME_CONTRACT`, `SITE_HANDOFF` | `docs/OPERATOR-UI-CONTRACT.md:1-55` | The contract names Hive as runtime repo, Site as UI repo, and `/ops/hive` as canonical route; endpoint definitions are contract evidence, not proof an endpoint was live for INC-001. |
| Runtime evidence projection schema | `RUNTIME_EVIDENCE_CONTRACT` | `docs/OPERATOR-UI-CONTRACT.md:91-229`; `pkg/hive/operator_projection.go:20-27`; `pkg/hive/operator_projection.go:100-159` | `runtime_evidence` fields exist for status, last run, queued request, artifacts, run events, and causal graph; this packet cites no captured projection response. |
| Runtime event type registry | `EVENT_TYPE_REGISTRY` | `pkg/hive/events.go:12-24`; `pkg/hive/events.go:53-81`; `pkg/hive/events.go:270-285` | Hive defines and registers runtime lifecycle and queued-request event types; this packet cites no incident-specific event IDs or store export. |
| Runtime evidence boundaries | `EVIDENCE_BOUNDARY` | `docs/OPERATOR-UI-CONTRACT.md:250-278` | The contract states queued launch intent is not runtime-start proof and observed Hive runtime events prove EventGraph event emission, not production deployment, human authorization, source-repo adoption, or protected side effects. |
| Runtime evidence projection implementation | `READ_MODEL`, `EVENTGRAPH_DERIVED` | `pkg/hive/operator_projection.go:264-270`; `pkg/hive/operator_projection.go:546-693` | The implementation builds `runtime_evidence` from EventGraph store reads and defaults to `not_observed`; this packet does not run that read model against an INC-001 store. |
| Runtime evidence tests | `VALIDATION_SOURCE` | `pkg/hive/operator_projection_test.go:223-393` | Tests prove queued requests stay `not_observed`, pre-start events are ignored, runtime starts anchor evidence, and queued intent is not joined to runtime conversation IDs; tests are source validation, not incident runtime evidence. |
| Launch-record model override boundaries | `REQUEST_METADATA_BOUNDARY` | `docs/OPERATOR-UI-CONTRACT.md:301-316` | Accepted launch overrides are request metadata and do not mutate global role defaults, start agents, rebind running providers, or prove runtime execution. |

The source anchors above were spot-checked in the local checkout before PR
review. They are commit-bounded source-location hints, not runtime observations,
deployment evidence, EventGraph export evidence, authority evidence, or
machine-validated proof that a Hive runtime was active for INC-001.

## Missing Evidence For INC-001

This packet records the following evidence as missing for the incident:

- exact Hive process invocation used for INC-001
- deployment environment or deployment identifier
- store DSN, store snapshot, or bounded EventGraph export for the incident
- `hive.run.started`, `hive.agent.spawned`, `hive.agent.stopped`, or
  `hive.run.completed` event IDs tied to the incident
- `/api/hive/operator-projection` response captured for the incident
- operator approval or authority decision tied to the incident
- production runtime observation, health check, or deploy log
- source-repo adoption evidence for any target repo touched by a Hive run

## Boundaries

This packet does not prove:

- that Hive was running during INC-001
- that a Hive runtime event exists for INC-001
- that a queued run request started execution
- that any Hive process used a persistent or production store
- that any Site `/ops/hive` route rendered live Hive data
- that any operator approved a protected action
- that any target repository was touched by Hive
- that any deployment, correction, rollback, or containment happened
- that EventGraph contains incident-dispositive runtime records
- that Test 001 is `GREEN`

Hive remains the runtime owner for Hive behavior. EventGraph remains the audit
record for emitted events. Site remains the browser UI owner for `/ops/hive`
and related operator routes. `operation` remains the incident
record owner for cross-repository tabletop reconciliation.

## Validation Plan

The owning repo validation for this documentation packet is:

```bash
make verify
```

The packet should be cited by `operation` only after the PR that
adds it has passed local validation, GitHub CI, exact-head adversarial review,
and has been merged to `origin/main`.
