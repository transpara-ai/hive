# Factory Order: Civilization production v1

- Version: 0.1.0
- Status: implementation authorized; consequential effects disabled
- Source: Human decisions recorded 2026-09-03
- Route: Critical

## Outcome

Ship an internally operated Civilization that accepts a natural-language Issue
or Human idea, uses canonical TLC v0.1.1 as an external workflow, asks Codex to
produce and test a repository-confined change, presents the result clearly to a
Human, and may auto-merge eligible Routine changes when separately enabled.

## Scope

- Hive: durable orchestration, Codex provider, repository and PR effects,
  default-off Routine auto-merge, recovery, and operational API.
- Site: natural-language intake and a Human Review Workbench.
- Work and EventGraph: durable causal records and replay.
- Platform: internal Docker Compose deployment, release BOM, health, backup,
  restore, and rollback.
- Docs: current operator workflow and production acceptance procedure.

## Non-goals

- Public multi-tenant service.
- Autonomous merge for Designed, Critical, unknown, or malformed routes.
- Autonomous deployment.
- Fable participation unless explicitly selected by a Human.
- Rewriting or deleting historical Factory v1 evidence.

## Critical observations

1. `credentials_auth_crypto_secrets`: provider, GitHub, database, and Site
   credentials are required by the production stack.
2. `production_runtime_enforcement`: Docker Compose starts persistent services
   that can create pull requests and, when separately enabled, merge Routine
   changes.
3. `governance_review_approval_repo_settings`: Routine auto-merge changes who
   may perform a consequential repository effect.

No Critical effect is authorized by this document. Credentials, stack startup,
auto-merge activation, push, pull-request mutation, and production deployment
require separate Human authority immediately before the effect.

## Acceptance

1. TLC policy is not embedded in Hive; unknown future TLC fields survive the
   boundary and source repository identity cannot be overridden.
2. Codex is the first provider behind a replaceable, digest-pinned command
   interface. It runs in a repository-confined worktree with bounded output and
   no dangerous sandbox bypass. Independent verification is offline,
   credential-blind, digest-pinned, and unable to modify the reviewed worktree.
   The reviewed implementation is bound to exact content, modes, paths, and
   base—not merely a provider-supplied filename list.
3. Routine auto-merge is default-off and fails closed unless repository,
   provenance, route, exact head, review, tests, and required checks all match.
4. Site accepts plain language and presents outcome, scope, diff, tests,
   blockers, next action, and Human decisions before technical receipts.
5. A clean PostgreSQL deployment survives restart and can export/import
   historical evidence without treating it as live work.
6. Issue, idea, and approved Critical Factory Order paths each produce a useful
   ready pull request; concurrent work and restart recovery are verified.
   Human blocker answers are fed into the retry, and review remediation reruns
   both implementation and review.
7. Versioned images, release BOM, backup/restore, health, rollback, and operator
   documentation are tested together.

## Relevant tests

- Repository unit and verification suites.
- TLC boundary injection and forward-compatibility tests.
- Provider executable/hash/output/sandbox failure tests.
- Auto-merge negative matrix and exact-head race tests.
- Site rendering, accessibility, authentication, and mutation tests.
- Docker Compose configuration, health, clean bootstrap, restart, backup,
  restore, and rollback tests.
- Current-head three-channel end-to-end acceptance with real repository work.

## Next action

Implement and review locally in isolated worktrees. Stop before every separately
gated effect.
