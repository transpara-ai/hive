---
doc_id: HIVE-DF-OPERATOR-UI-CONTRACT
title: Hive Operator UI Contract
doc_type: runtime-contract
status: draft
version: 0.1.2
created: 2026-06-03
updated: 2026-06-08
owner: Michael Saucier
steward: assistant
project: dark-factory
canonical: false
runtime_repo: transpara-ai/hive
ui_repo: transpara-ai/site
canonical_route: /ops/hive
---

# Hive Operator UI Contract

## Revision History

| Version | Date | Description |
|---------|------|-------------|
| 0.1.0 | 2026-06-03 | Initial repo-ready runtime contract for the Dark Factory Human Operator UI. |
| 0.1.1 | 2026-06-04 | Added standard Transpara frontmatter, semver, and revision history. |
| 0.1.2 | 2026-06-08 | Committed to version control (rescued from the uncommitted working tree before age-out); owner corrected to Michael Saucier per the Dark Factory doc convention. Still a pre-acceptance draft — none of these endpoints exist yet (tracked by hive#127). |

## Boundary

Hive is not the browser UI owner. Hive owns runtime orchestration, agent loops, diagnostics, authority checks, event emission, and projections. Site owns the human operator UI.

Hive must expose structured data that Site can render:

- Intakes.
- Factory runs.
- Run projections.
- Events.
- Approval requests.
- Artifacts.
- Agent states.
- Resource usage.
- Guardian alerts.

## Required runtime endpoints

```text
POST /api/hive/intakes/{id}/derive
POST /api/hive/runs
GET  /api/hive/runs/{id}
GET  /api/hive/runs/{id}/events
GET  /api/hive/runs/{id}/approvals
POST /api/hive/approvals/{id}/resolve
```

## Launch request

```json
{
  "operator_id": "user_123",
  "intake_id": "intake_123",
  "title": "Build onboarding flow",
  "brief": {},
  "sources": [],
  "authority": { "initial_level": "required" },
  "budget": { "max_iterations": 20, "max_cost_usd": 50 },
  "target_repos": ["transpara-ai/site"]
}
```

## Launch response

```json
{
  "run_id": "run_123",
  "status": "queued",
  "first_event_id": "evt_123"
}
```

## Projection response

```json
{
  "run_id": "run_123",
  "title": "Build onboarding flow",
  "status": "active",
  "active_phase": "design",
  "guardian_state": "clear",
  "agents": [],
  "pipeline": [],
  "approvals": [],
  "events": [],
  "artifacts": [],
  "resources": {}
}
```

## Intake/factory event types

```text
source.ingested
source.parsed
brief.derived
brief.edited
brief.approved
factory.run.requested
factory.run.started
factory.phase.started
factory.phase.completed
factory.artifact.created
factory.run.completed
factory.run.halted
factory.run.abandoned
```

## Authority resolution

Site must never directly mutate authority state. It sends a resolution request to Hive. Hive records the authority event and exposes the updated projection.

```json
{
  "request_id": "auth_123",
  "approved": true,
  "resolver_id": "user_123",
  "reason": "Scope is bounded and tests are required."
}
```

## Invariants

- Every run has a source cause.
- Every artifact has causes.
- Every approval resolution has a human-visible reason.
- Every rejected approval has a reason and optional requested revision.
- Every self-modification path uses Required authority.
- Guardian/HALT state is projected immediately.
- Budget state is visible before and during execution.
