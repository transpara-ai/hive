# Dark Factory Authority Vocabulary

Date: 2026-05-08

Source of truth: `transpara-ai/docs` `dark-factory/DF-SOP-0001-authority-gated-side-effects.md`.

Hive owns the current runtime constants for the shared Phase 1 authority vocabulary in `pkg/safety`.

## Authority Outcomes

```text
Autonomous
Notify
ApprovalRequired
Forbidden
```

## Protected Actions

Hive must use these exact protected action names when evaluating, logging, or emitting authority requests:

```text
production.deploy
repo.create
repo.delete
repo.push.default_branch
repo.merge.main
repo.mutate.cross_repo
agent.spawn.persistent
agent.retire
agent.escalate_permissions
policy.change
secret.access
external_communication.company_voice
data.delete
self_modification.activate
billing.spend_above_threshold
license.change
```

## Local Alignment Notes

- `pkg/safety.ProtectedActions` mirrors the SOP baseline.
- `repo.mutate.cross_repo` is the canonical action name. Do not use the older `repo.mutate_cross_repo` spelling.
- Blocked-path log names should derive from the protected action where practical, for example `repo.mutate.cross_repo.blocked`.
- `authority.requested` event content must carry the canonical action string in `Action`.
