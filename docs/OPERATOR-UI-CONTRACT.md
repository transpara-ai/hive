---
doc_id: HIVE-DF-OPERATOR-UI-CONTRACT
title: Hive Operator UI Contract
doc_type: runtime-contract
status: draft
version: 0.2.0
created: 2026-06-03
updated: 2026-06-09
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
| 0.1.3 | 2026-06-09 | Added phase-1 read-only model-selection data on the operator projection for hive#128; catalog loading is startup-static and edit/hot-reload surfaces remain future work. |
| 0.2.0 | 2026-06-09 | Added phase-2 Hive-owned model catalog hot reload metadata and validated per-run model overrides for hive#128. |

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
  "model_overrides": [
    {
      "role": "guardian",
      "model": "api-sonnet",
      "max_cost_per_call_usd": 3.5
    }
  ],
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
  "resources": {},
  "model_selection": {
    "source": "hive",
    "catalog_source": "embedded-defaults",
    "loaded_at": "2026-06-09T09:00:00Z",
    "reload_mode": "startup-static|hot-reload",
    "hot_reload": false,
    "last_reload_at": "2026-06-09T09:10:00Z",
    "models": [],
    "assignments": [],
    "errors": []
  }
}
```

## Model selection projection

Hive exposes model-selection data as a read-only part of `/api/hive/operator-projection`.
Site may render this data, but Hive remains the source of truth.

The projection includes:

- Model catalog entries with provider, auth mode, tier, capabilities, context window, output-token limit, and pricing metadata.
- Starter civic-role assignments after Hive applies existing `modelconfig.Resolver` policy, defaults, and `CanOperate` constraints.
- Catalog load metadata: `catalog_source`, `loaded_at`, `reload_mode`, `hot_reload`, and `last_reload_at` when a reload has occurred.

Important boundaries:

- Subscription (`claude-cli`) remains the default catalog path.
- API-key (`anthropic`) models may appear in the catalog, but role assignment to them requires explicit catalog/policy configuration.
- `CanOperate` roles must continue resolving only to Operate-capable providers such as `claude-cli` or `codex-cli`.
- `startup-static` means Hive loaded the catalog once at process start.
- `hot-reload` means Hive owns a runtime reload loop for the configured catalog path. A failed reload is projected in `model_selection.errors`; Hive keeps the last valid resolver active.
- Runtime reload affects future resolver reads and future dynamic spawns. It does not silently rebind provider instances already running inside an agent loop.
- The projection is not an edit/write API.

## Per-run model overrides

`POST /api/hive/runs` may include `model_overrides`, a list of explicit per-run role overrides.
Hive validates each override before appending any launch events by resolving it through `modelconfig.ResolutionInput.TaskOverride`.

Rules:

- `role` is required and must name a starter civic role.
- At least one of `model`, `profile`, `provider`, `preferred_tier`, `required_capabilities`, or `max_cost_per_call_usd` must be set.
- Duplicate role overrides are rejected.
- Unknown, malformed, over-budget, or `CanOperate`-incompatible overrides are rejected before `source.ingested`, `brief.derived`, or `factory.run.requested` events are written.
- Accepted overrides are recorded on `factory.run.requested` with requested fields plus resolved model, provider, and auth mode.
- Overrides are scoped to that run request. They do not mutate global role defaults.

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
