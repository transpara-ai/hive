---
doc_id: FO-HIVE-262-ISSUE-SCAN-RUNNER-SUITE-PACKAGE
title: Factory Order — Issue-Scan Runner-Suite Package Manifest and Fixture Validation Harness
doc_type: factory-order
status: proposal
version: 0.1.0
created: 2026-07-11
updated: 2026-07-11
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#262
authority: local non-mutating validation implementation only; no runner execution, Hive wake/start/action API use, live issue scanning, daemon configuration, service restart, GitHub mutation beyond normal PR flow, Work write, production EventGraph read/query/write, deploy, protected settings change, Test 001 GREEN, production go-live, value allocation, autonomy increase, or wiki work
---

# Factory Order — Issue-Scan Runner-Suite Package Manifest and Fixture Validation Harness

## Immutable Source Citations

| Source | Pin | Role |
|---|---|---|
| [transpara-ai/hive#262](https://github.com/transpara-ai/hive/issues/262) | issue body as of 2026-07-11 (created 2026-07-10T02:54:33Z; labels `cc:intake`, `cc:pr-ready`, `cc:protected-action`, `cc:civilization-presence`; no comments at FO crafting time) | Raw intake — channel A: named directly by the human operator (Michael, 2026-07-11 session order "finish 262 … to pr ready state full autonomy") |
| `docs/designs/issue-scan-runner-suite-packaging-v0.1.0.md` | blob SHA `3e2fcc3ace24a0729e50074f3f2fd21fb05ad259` (doc_id `HIVE-ISSUE-SCAN-RUNNER-SUITE-PACKAGING` v0.1.0) | Approved design packet — merged to `main` via [hive#261](https://github.com/transpara-ai/hive/pull/261) (merge commit `209efcc`); the merge is the pinned Human Design Review approval |
| `cmd/hive/factory_issue_scan_runner_contracts.go` | `issueScanRunnerContracts()` document, lifecycle `civilization_issue_to_human_ready_pr_v0.9` | In-process machine-readable contract the package must map to |

## Design-Stage Fidelity Adjudication

This slice adds no new design packet. Every requirement below traces to a
section of the already-approved packet (Package Contents, Future Implementation
Readiness minimum criteria "package manifest schema and tests" and
"fixture-based parser tests for every runner stdin/stdout contract"). The
design → IADA → CFADA → Human Design Review stages for this shape are credited
to the merged packet blob above; re-running design for the same shape would
duplicate an approved truth object.

## Requirements

Each requirement is individually verifiable by a named test in
`cmd/hive/factory_validate_runner_suite_test.go` (test names recorded in the PR
evidence).

- **R1 — Manifest schema.** A package manifest (`manifest.json`) records suite
  id, lifecycle version, terminal stage path, component ids, command path and
  argv placeholders, timeout, stdin kind, stdout kind, required environment
  variables, forbidden environment variables, authority boundaries, fixture
  paths, and validation command. Unknown manifest fields are rejected (strict
  decode).
- **R2 — Contract mapping.** Validation cross-checks the manifest against the
  in-process `issueScanRunnerContracts()` document: lifecycle version must
  equal the document's; each component id must be a known external runner
  contract id; each component's stdin/stdout kind must exactly equal that
  contract's `stdin_context_kind` / `stdout_contract_type`; the component set
  must exactly equal the set the declared terminal stage path requires, derived
  from the document's daemon flags and terminal-path mutual exclusions.
- **R3 — Synthetic fixtures.** For every manifest component, inert stdin
  context and expected stdout fixtures exist under `examples/`, strictly
  decode into the concrete `pkg/hive` context/result Go types, and satisfy the
  contract's `stdout_required_fields` specs. Fixtures contain no secrets,
  production data, private network addresses, or live issue contents.
- **R4 — Fail-closed validation.** Validation fails (non-nil error naming the
  defect) for: missing required manifest fields; unknown manifest fields;
  unknown component ids; duplicate component ids; unsupported or mismatched
  stdin/stdout kinds; unknown terminal stage path; component set
  missing/excess for the declared path; malformed fixture JSON; unknown fields
  in fixtures; missing fixture files; fixture paths escaping the package
  directory; non-positive or unparseable timeouts; forbidden environment
  declarations (a required env var that is also forbidden, or a forbidden list
  missing the canonical minimum `ANTHROPIC_API_KEY`,
  `HIVE_ANTHROPIC_API_KEY`); and any `stdout_required_fields` spec the checker
  grammar does not recognise (unknown spec syntax is an error, never a skip).
- **R5 — No execution.** The harness never executes external runner commands;
  command entries are inert placeholders and are checked only as declarations.
- **R6 — Operator surface.** `hive factory validate-issue-scan-runner-suite
  --package <dir>` runs the same validation locally and non-mutatingly and is
  recorded as the manifest's `validation_command`.
- **R7 — Mapping record.** The package `README.md` records how manifest fields
  map to `hive factory issue-scan-runner-contracts` output and restates the
  package non-authorizations.

## Non-Goals

- No executable runner implementations, wrappers, or `runners/` binaries.
- No runner invocation against live or stored issue-scan runs.
- No daemon configuration, daemon start, service restart, or Hive
  wake/start/action API use.
- No `catalog/` provider records (later slice; nothing in this slice needs
  model selection).
- No aggregation with managed ready-PR finalizer remediation, daemon
  configuration, live rehearsal, or production hardening (separate risk
  classes per the design packet and hive#262).

## Verification Plan

- `go test ./cmd/hive` (new table-driven tests) and `go test ./pkg/hive`
  (untouched, regression guard).
- `make verify` (canonical paths, build, test, vet) plus `staticcheck`.
- `git diff --check`.
- IAR then CFAR (Codex reviewer; Claude author) at the exact PR head before
  ready transition; merge consideration remains Michael's.

## Non-Authorizations

This Factory Order states intent and grants nothing. It does not authorize
runtime execution, runner execution, live issue scans, GitHub mutation beyond
normal PR flow, Work writes, production EventGraph reads/queries/writes,
deploy, service restart, protected settings changes, Test 001 GREEN,
production go-live, value allocation, autonomy increase, or wiki work.
