# Civilization production v1 design

## Boundary

TLC is a portable workflow, not a Hive state machine. It returns a versioned
transport envelope containing a route and short brief. Hive validates only the
stable transport projection, preserves all TLC-owned fields as canonical JSON,
and binds repository effects to independently captured source provenance.

Hive runtime states are operational facts:

`routing -> queued -> implementing -> validating -> reviewing -> publishing -> ready -> merge_queued -> completed`

`blocked` and `human_required` may interrupt any effectful transition. These
states do not reproduce TLC policy or add TLC gates.

## Provider

The first adapter invokes stable `codex exec` non-interactive mode with:

- a pinned executable realpath and SHA-256;
- a pinned model and optional profile;
- an exact repository worktree as `--cd`;
- `workspace-write`, `--approve-for-me`, `--ephemeral`, and strict structured
  output;
- bounded time and output;
- a named environment allowlist; and
- no dangerous sandbox or hook-trust bypass.

Codex edits and tests inside the worktree. Hive, not Codex, owns branch, commit,
push, pull-request, and merge effects and records their receipts.

Before review, Hive binds the implementation result to a SHA-256 over the exact
base, changed paths, file types, executable modes, and contents. The same digest
must survive review, independent verification, staging, commit, and restart
recovery; matching filenames alone are insufficient.

Before any publication, Hive reruns the repository's allowlisted verification
command through a separately pinned Codex sandbox profile. That profile has no
provider or GitHub environment, denies filesystem reads by default, exposes the
worktree read-only, allows writes only to an ephemeral build scratch directory,
uses a preloaded module cache with `GOPROXY=off`, and disables direct network
access. Both the sandbox executable and profile file are digest-bound and
rechecked immediately before execution.

## Routine auto-merge

The merge executor is inert unless explicitly enabled. Eligibility requires:

- route exactly `Routine`;
- a Civilization-created PR in an explicit `transpara-ai` repository allowlist;
- exact current head equal to the reviewed and validated head;
- passing required checks and ordinary review;
- no unresolved blocker or Human intervention;
- no protected-path match; and
- no branch-protection bypass or force operation.

Designed, Critical, unknown, stale, externally created, or ambiguous work fails
closed to Human review. Activation and credentials are production effects and
remain separately authorized.

## Human experience

Site starts with outcome and action, not evidence internals. Human intake is
plain language. The workbench shows the brief, progress, diff summary, tests,
review, blockers, and approve/reject/redirect controls. JSON, paths, hashes, and
raw event evidence live in collapsed technical details. When work is blocked,
the Human answer is recorded and included as data in the next relevant provider
attempt. Review findings return to implementation and review instead of looping
at publication.

## Deployment and data

Platform owns a pinned Docker Compose release of PostgreSQL, the Hive
Civilization API, and Site. Hive uses Work and EventGraph as libraries rather
than operating a second browser or orchestration service. Services use internal
networking, non-root containers, read-only filesystems where practical, health
checks, secrets mounted as files, and named durable volumes. A new production
database is bootstrapped cleanly. Historical evidence remains in its existing
store and is exposed only through an optional read-only legacy API.

Backup, restore, restart, upgrade, rollback, and exact image identities are
acceptance requirements, not post-release chores.

## Migration

The historical Factory v1 implementation and its 12-stage records remain
readable evidence. The production runtime does not append new legacy stage
events. Active work migrates only at a safe boundary; prior evidence is never
rewritten.
