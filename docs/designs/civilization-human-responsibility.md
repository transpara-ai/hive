# Human responsibility in the Workbench

Route: Designed. Outcome: operators can see and assign a named responsible person
for human confirmations, questions and reviews across parallel workstreams.

An optional work-level assignment applies to current and future human handoffs.
Existing and new work starts unassigned; assignment and return to the shared queue
are explicit, durable events. This field routes responsibility. It does not change
who may confirm, resolve, publish or approve, and it executes no provider work.
The authenticated Site caller supplies its signed-in operator ID as `assigned_by`.

`POST /api/civilization/v1/work/{workID}/human-owner` accepts `owner_id`,
`assigned_by`, and `previous_assignment_id`. Empty owner returns work to the queue.
The work projection exposes `human_owner_id`, `human_owner_assigned_by`, and
`human_owner_assignment_id`. A stale assignment version returns 409. Assignment
events have one idempotency key per previous version, so competing changes cannot
silently overwrite one another. Existing records need no migration.

Parallelism already uses independent work IDs, worktrees, persisted events and
per-work locks. The reconciler defaults to three concurrent work items through
`CIVILIZATION_RECONCILE_CONCURRENCY`; the configured runtime value is not exposed
by the work API. Stage is not a process heartbeat. Runtime agent events are a
separate projection and do not currently identify these work IDs.

Tests cover restart/replay, two competing assignees, stale HTTP writes, malformed
assignment, and preservation of execution state and confirmation. Site provides
live rollup and assignment controls in its own repository. Permission enforcement,
notifications, deployment and runtime configuration are outside this increment.
