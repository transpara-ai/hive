# Retire legacy Factory v1 runtime

Status: implementation design

Version: v0.1.0

Date: 2026-09-04

## Outcome

Remove the former twelve-stage TLC runtime from the active Civilization while
preserving the canonical `tlc-envelope/v1` boundary, the current Civilization
engine, the general Hive operator projection API, and product-domain Work
gates. TLC workflow policy remains owned by `transpara-ai/tlc` and can change
without a Hive or Site refactor.

## Observed deployment state

The retirement prerequisite was checked before implementation:

- `hive-ops-api.service` is not installed or active in this host's user
  systemd instance.
- No user unit matching Hive, Factory, or `work-server` is installed or active.
- No process matching `hive-ops-api`, `factory-v1-demo-runner`,
  `hive factory-v1`, or `civilization-dark-factory` is running.
- `/home/transpara/transpara-ai/runtime/civilization-dark-factory-v1` is absent.
- GitHub reports no deployment or environment for `transpara-ai/hive`.

These observations prove that the bounded environment available to this change
does not deploy the legacy runtime. They do not claim knowledge of unregistered
machines. The change therefore removes source and configuration only; it does
not stop a process, alter a service, delete runtime data, or deploy anything.

## Recoverable source identity

Git history is the archive. At Hive base
`d4dad33a5c9e81a7178ad04e7d422212d204cb6e`, the 36-file legacy-runtime set has
Git-tree listing SHA-256
`9ba50c82c7a4dddcbdbb0d31fe8dabd843ce56d3b5ac8ec621e83f7b2a83012a`.
At Site base `f72c0ef6780cfdc2dd62b810bbbb82a624f992e6`, the four-file legacy UI set has
Git-tree listing SHA-256
`2aedea30cffc7adc02ce3fdbd2b23108e123e4343d8369193a30c9f99654534b`.

## Design

### Keep

- `pkg/hive/tlcbridge`: the small, versioned transport parser and binder.
- `pkg/hive/civilization`: persistence, effects, exact-head safety, and Human
  effect boundaries driven by the external TLC envelope.
- `hive-ops-api`: its health, assembly, Mission Control, model-selection, and
  governed operator endpoints that do not expose Factory v1.
- Work phase gates and SaaS security gates. They are product-domain APIs and
  no active caller was found using them to select or enforce TLC routes.
- Historical commits and design records.

### Remove from Hive

- `pkg/hive/factoryv1`, including its fixed stage list and scheduler.
- Factory v1 adapters, daemon, external runner, runtime snapshot, and operator
  HTTP endpoints.
- `hive factory-v1`, `factory-v1-demo-runner`, and the local supervisor scripts.
- Factory v1 inputs and assumptions from the current Mission Control
  projection. Missing legacy input must no longer make the aggregate unhealthy.

### Remove from Site

- The `/console/factory-v1` History surface and its mutation proxy.
- Factory v1 navigation and links. Current Mission Control remains the single
  end-to-end UI for canonical Workbench activity.

### Compatibility and safety

- Existing EventGraph records are not rewritten or deleted.
- No database schema migration is required.
- Removed HTTP routes return the ordinary not-found response.
- The canonical engine continues to accept only `tlc-envelope/v1`; it does not
  learn TLC stages or read `POLICY.md`.
- No runtime or settings effect is part of this change.

## Tests

- Hive `go test ./...` and `go vet ./...`.
- Site `go test ./...` and `go vet ./...`.
- Repository scans find no active former-stage names, Factory v1 route,
  runtime command, supervisor, or copied TLC implementation.
- Focused canonical bridge and Civilization engine tests remain green.
- Mission Control tests prove a complete aggregate without the retired Factory
  projection.

## Rollout

1. Land the Hive source retirement.
2. Land the Site UI retirement after or with Hive.
3. Keep both pull requests draft until native CI passes.
4. Perform no process, deployment, database, or settings action in either pull
   request.

Rollback is a Git revert of the exact retirement commits. Restoring a runtime
deployment or its data would be a separate, explicitly authorized operation.
