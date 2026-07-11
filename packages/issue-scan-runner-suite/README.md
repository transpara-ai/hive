# Issue-Scan Runner-Suite Package (inert v1 scaffold)

The first implementation slice of the runner-suite packaging contract
(`docs/designs/issue-scan-runner-suite-packaging-v0.1.0.md`, source intent for
[hive#262](https://github.com/transpara-ai/hive/issues/262)). This package is
**inert**: it contains a manifest and synthetic fixtures only. There are no
runner executables, no catalog records, and nothing here authorizes runner
execution, daemon configuration, live issue scanning, or any GitHub, Work, or
EventGraph mutation.

## Validate locally

```bash
hive factory validate-issue-scan-runner-suite --package packages/issue-scan-runner-suite
```

The command reads files only and fails closed: missing or unknown manifest
fields, unknown component ids, kind mismatches, malformed or unparseable
fixtures, non-local paths, and forbidden environment declarations are all
errors. It never executes the `command` placeholders. The same validation runs
in CI via `go test ./cmd/hive` (`TestValidateIssueScanRunnerSuitePackage*`).

## How the manifest maps to `hive factory issue-scan-runner-contracts`

Every value here is cross-checked in-process against that command's document —
the manifest cannot drift from the contracts without failing validation:

| Manifest field | Contracts document source |
|---|---|
| `lifecycle_version` | `lifecycle_version` (exact match required) |
| `terminal_stage_path` | `terminal_stage_paths[].id` (allowlist; this package uses the recommended `managed_ready_pr_finalizer`) |
| `components[].id` | `external_runner_contracts[].id` (allowlist; the required set is derived from `full_chain_daemon_flags` minus the terminal path's `mutually_exclusive_with`, plus the path's own flags — for this posture exactly the five external runners) |
| `components[].stdin_kind` | that contract's `stdin_context_kind` (exact match) |
| `components[].stdout_kind` | that contract's `stdout_contract_type` (exact match) |
| `components[].authority_boundaries` | must equal that contract's `authority_boundaries` exactly (validated both directions: a dropped boundary hides a limit, an added one could grant authority — operational notes belong in this README, not in authority metadata) |
| `examples/<id>/stdin.json` | strictly decodes into that contract's `stdin_context_type` Go type |
| `examples/<id>/stdout.json` | strictly decodes into that contract's `stdout_contract_type` Go type and satisfies its `stdout_required_fields` |

The managed draft-PR authority requester, draft-PR creator, and ready-PR
finalizer stay inside Hive (daemon flags), so they have no package components.
The generic `ready_pr_evidence_runner` terminal adapter is mutually exclusive
with the managed finalizer; declaring both in one package fails validation.

## Fixtures

All fixture content is synthetic and public-safe: no secrets, no production
data, no private network addresses, no live issue contents. `command` entries
are package-local placeholders (`runners/*.placeholder`) that do not exist and
are never resolved or executed by this slice.

`required_env` is empty for every component; `forbidden_env` must include at
least `ANTHROPIC_API_KEY` and `HIVE_ANTHROPIC_API_KEY` (setting either breaks
the Claude CLI subscription auth the runtime uses) and this package also
forbids `GITHUB_TOKEN` for all five external runners, since GitHub mutation is
outside every external runner's authority boundary.

## What later slices add (separate risk classes, separate issues)

Runner-wrapper executables under `runners/`, `catalog/` provider records,
executable-availability checks, daemon admission wiring, live rehearsal, and
ready-state remediation are future child issues per the design packet — they
are deliberately not part of this package.
