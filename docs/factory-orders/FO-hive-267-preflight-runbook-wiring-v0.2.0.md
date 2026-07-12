---
doc_id: FO-HIVE-267-PREFLIGHT-RUNBOOK-WIRING
title: Factory Order — Wire the hive-lifecycle Dialect Runbooks to the Tested Unit-Posture Verifier
doc_type: factory-order
status: proposal
version: 0.2.0
created: 2026-07-12
updated: 2026-07-12
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
source_issue: channel A operator directive 2026-07-12 (sha256 e67e9f0e4b3dbb2e557e3ebf9bc22a685225583d38ba0f64a53e93fb1a80599f); lineage transpara-ai/hive#267; verifier delivery transpara-ai/hive#277
authority: repository documentation/skill-source and test-only changes; no hive.service start/stop/restart, daemon launch, runtime execution, service restart, deploy, public exposure, authentication change, protected settings change, production EventGraph read/query/write, Work runtime write, Test 001 GREEN, value allocation, autonomy increase, or wiki work
---

# Factory Order — Wire the hive-lifecycle Dialect Runbooks to the Tested Unit-Posture Verifier

## Immutable Source Citations

| Source | Pin | Role |
|---|---|---|
| Operator directive, Michael Saucier, 2026-07-12 | content sha256 `e67e9f0e4b3dbb2e557e3ebf9bc22a685225583d38ba0f64a53e93fb1a80599f` (verbatim text archived below) | Channel A raw intake: implement the unit preflight as a tested Go subcommand, then "shrink both dialect runbooks to invoke it" |
| [transpara-ai/hive#267](https://github.com/transpara-ai/hive/pull/267) | merged; runbook lineage per FO-HIVE-265-LIFECYCLE-SKILL-HOME v0.58.0 | CFAR lineage: the rounds that first hardened, then deliberately **removed**, the inline bash preflight — replacing it with a human gate, a minimal post-start posture probe, and the promise "a mechanical verifier belongs in a tested Go subcommand (tracked as separate work)" |
| [transpara-ai/hive#277](https://github.com/transpara-ai/hive/pull/277) | merge commit `77739de059e81112b4337d51d7e3a7ebd0684ff1` (commits `f057496`, `e97c0b0`); Codex-authored, Claude CFAR pass at draft head `e97c0b0` and ready head `03ea9b6` | Delivered verifier: `hive factory preflight-hive-unit` — read-only merged-property + `/proc/<MainPID>/environ` posture report, fail-closed, whole-domain table-driven tests |
| `.claude/skills/hive-lifecycle/SKILL.md` | git blob `15a34bfc69d3bf68307e57176151eda6f0beb462` at `bf3f126` (origin/main) | Claude dialect physical file (feature home reaches it via the `skills/hive-lifecycle/claude` symlink per FO-HIVE-265 R2) — pre-change state this FO amends |
| `skills/hive-lifecycle/codex/SKILL.md` | git blob `3b8ca8ffa6885fef016b10da6bfffb05f1d1f0b0` at `bf3f126` (origin/main) | Codex dialect physical file — pre-change state this FO amends |

## Intake Adjudication (channel A)

The directive names two halves. The first — "implements `hive factory preflight
--unit hive` (or similar) in Go with table-driven tests covering the whole
input domain" — is **already delivered and merged** as
`hive factory preflight-hive-unit` (PR #277, merged 2026-07-12T11:53Z, before
this FO was crafted). The directive's inventory of the historical bash
preflight ("Merged EFFECTIVE properties", canonical-launcher allowlist, …)
describes a state that PR #267's later CFAR rounds intentionally superseded:
the accepted design is a **human gate before start/restart** (pre-start proof
was adjudicated impossible from a shell runbook) plus **post-start posture
confirmation**, which is exactly the contract PR #277's verifier implements.
This FO therefore covers only the outstanding second half: shrink both dialect
runbooks to invoke the delivered verifier and retire the now-stale
"tracked as separate work" promises. Design-packet stages (design, IADA,
CFADA, Human Design Review) are waived for this slice under the directive's
explicit stage enumeration ("FO, feat/ branch, TDD, draft PR, IAR+CFAR");
residual risk is low — documentation and test changes only, wiring two
already-CFAR'd components together with no runtime-semantics change. CFAR
remains in force.

## Requirements

- **R1 — Stale promises retired.** Both dialects' verifier promises — "a
  mechanical verifier belongs in a tested Go subcommand (tracked as separate
  work), not here" (protected-action gate paragraph) and "the mechanical
  verifier belongs in a tested Go subcommand — tracked separately" (Hive
  Restart comment) — are replaced with references to the delivered
  `hive factory preflight-hive-unit` subcommand. No text in either dialect
  continues to describe the verifier as future work.
- **R2 — Post-start posture confirmation invokes the verifier.** In both
  dialects, the post-start (post-restart) posture-confirmation block replaces
  the inline shell probe (`systemctl … MainPID` + `tr '\0' '\n'
  </proc/$pid/environ` + `grep '^LOVYOU_API_KEY='`) with an invocation of the
  tested subcommand (`go run ./cmd/hive factory preflight-hive-unit` from the
  hive checkout), plus interpretation guidance mapping its output to the
  operator decision: `credential_posture=PRESENT` = production-connected (stop
  the unit unless that posture was approved); `ABSENT`/`EMPTY` = local-only;
  nonzero exit / `overall=UNKNOWN` = fail closed (if local-only was intended,
  stop the unit). The guidance states each posture's provenance per the
  verifier's contract: credential posture from the RUNNING process's
  environment; autonomy posture from the CONFIGURED merged `ExecStart`, not
  the live argv — a unit changed since start can run different flags, so live
  autonomy is treated as unproven and deferred to the human gate when the two
  could differ (v0.2.0). The runbook never re-derives credential posture for
  `hive.service` from shell.
- **R3 — Human-gate semantics preserved, not weakened.** The pre-start human
  gate is unchanged: explicit current-turn approval naming both postures
  before `systemctl --user start|restart hive`; worst-case
  production-connected assumption pre-start (the verifier requires a running
  `MainPID` and fails closed otherwise, so it is verification-only and cannot
  pre-clear a start; its credential posture reads the running process while
  autonomy posture reflects the configured merged `ExecStart` — v0.2.0); the
  foreground local-only alternative remains. Secret-safety is preserved or
  improved: the verifier never prints credential values (asserted by its
  merged tests), and removing the shell probe removes the only place a
  credential value transited a shell variable in this flow.
- **R4 — Consistency is tested (VERIFIED invariant).** A table-driven test in
  `cmd/hive` reads both dialect files and asserts, per dialect: (a) the file
  invokes `factory preflight-hive-unit`; (b) the stale promise phrases from R1
  are absent; (c) no inline `grep '^LOVYOU_API_KEY='` probe remains; and (d)
  the `skills/hive-lifecycle/claude` symlink still resolves to the physical
  Claude-dialect file (one-physical-copy invariant from FO-HIVE-265 R2). The
  test is written first and observed RED against the unedited runbooks, then
  GREEN after the edits.

## CFAR Round 1 Repair (v0.2.0)

CFAR round 1 (Codex, PR #283 head `07bc172d`) found one P1: the runbooks'
post-start guidance broadened the verifier's configured-unit autonomy report
into confirmation of the RUNNING process's autonomy ("compares the RUNNING
runtime against the posture the user approved"), which is false when
`ExecStart` and live argv differ (unit edited/reloaded after start, wrapper
expansion) — the verifier derives autonomy solely from
`systemctl show -p ExecStart` and reads the running process only for
credential presence. Accepted and repaired doc-side per the finding's
scoping option: both dialects and R2/R3 above now state per-posture
provenance, and live autonomy is deferred to the human gate whenever the
configured unit may have changed since start. Extending the verifier to
reconcile `/proc/<MainPID>/cmdline` was rejected for this slice: it is a
verifier contract change (Non-Goal 1) and the configured-vs-live gap is
PR #277's explicitly accepted, bounded residual (its ready-state CFAR,
disposition 1). If live-argv reconciliation is wanted, that is a new
governed slice against the verifier.

## Non-Goals

- No change to the verifier itself (`cmd/hive/factory_preflight_hive_unit.go`)
  — its contract, output format, and tests are PR #277's delivered scope.
- No replacement of the `hive-ops-api` writer-mode or catalog probes in the
  Codex dialect: those inspect a different unit and different variables
  (`HIVE_OPS_HUMAN_ACTOR`, `HIVE_OPS_CATALOG`) outside the verifier's
  contract. If a preflight for that unit is wanted, that is a new slice.
- No change to Hive Restart's branching logic: its merged-`ActiveState` read
  drives shell control flow (provably-stopped allowlist) and stays; only its
  stale verifier comment is updated (R1).
- No new subcommand, no flag changes, no unit mutation of any kind.

## Verification Plan

- `go test ./cmd/hive -count=1` — RED before the runbook edits (new
  consistency test fails on all stale markers), GREEN after.
- `go vet ./...` and `staticcheck ./cmd/hive` clean.
- `LOVYOU_API_KEY= make verify` clean.
- `git diff --check` clean; both dialect diffs reviewed for R3 (no gate
  weakening) at IAR and CFAR.

## Non-Authorizations

This FO states intent only. It grants no authority to start, stop, restart,
or signal any unit; no runtime execution; no deploy; no production writes; no
merge (stage 12 stays with Michael); no autonomy increase. The verifier
invocation it documents is itself read-only by its merged contract.

## Archived Channel A Directive (verbatim)

The cited sha256 is over exactly the single paragraph inside the fence below
plus one trailing newline (`printf '%s\n' "<line>" | sha256sum`); the fence
preserves the original bytes unwrapped so the hash is reproducible.

```text
During hive#267 (skills/hive-lifecycle dialect home in /Transpara/transpara-ai/repos/hive), 18 CFAR rounds progressively hardened a bash preflight embedded in both dialect SKILL.md files (search for "Merged EFFECTIVE properties" in .claude/skills/hive-lifecycle/SKILL.md and skills/hive-lifecycle/codex/SKILL.md). It now checks: merged systemd properties (Environment, EnvironmentFiles, UnsetEnvironment, ExecStart, WorkingDirectory), manager environment, /proc/PID/environ (NUL-safe, fail-closed), credential injection via ExecStart wrappers, a canonical-launcher allowlist, full-autonomy flags, and a three-way verdict. A shell runbook is the wrong home for a security verifier of this complexity — propose a governed slice that implements `hive factory preflight --unit hive` (or similar) in Go with table-driven tests covering the whole input domain (every source, every fail direction), then shrinks both dialect runbooks to invoke it. Governed transpara-ai work: FO, feat/ branch, TDD, draft PR, IAR+CFAR.
```
