---
doc_id: FO-HIVE-265-LIFECYCLE-SKILL-HOME
title: Factory Order — Canonical Versioned Home for the hive-lifecycle Skill (Claude + Codex Dialects)
doc_type: factory-order
status: proposal
version: 0.24.0
created: 2026-07-11
updated: 2026-07-11
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#265
authority: repository documentation/skill-source preservation only; no Hive start/stop/restart, daemon launch, runtime execution, service restart, deploy, public exposure, private fetch, authentication change, protected settings change, production EventGraph read/query/write, Work runtime write, Test 001 GREEN, production go-live, value allocation, autonomy increase, or wiki work
---

# Factory Order — Canonical Versioned Home for the hive-lifecycle Skill

## Immutable Source Citations

| Source | Pin | Role |
|---|---|---|
| [transpara-ai/hive#265](https://github.com/transpara-ai/hive/issues/265) | issue body as of 2026-07-11 (labels `cc:intake`, `cc:pr-deferred`, `cc:protected-action`, `cc:civilization-presence`, `cc:needs-human-scope`) | Raw intake — the governed tracker for the Codex skill port |
| Michael Saucier, in-session operator scope verdict, 2026-07-11 | "Claude and Codex versions of a particular feature together in the same repo … choose the correct home for the feature … differences subdivided (as of today) into Claude and Codex subfolders … may grow to various dialects" | Channel A human scope decision this FO implements; supplies the `needs-human-scope` answer |
| `~/.claude/skills/hive-lifecycle/SKILL.md` | immutable seed pin: git blob `d4f8b8a1772a6810eb1d808902df7cae20e53da2` (the dialect as first committed at `7e649d6`; runbook lineage: hive PR #259) | Claude dialect seed |
| `~/.codex/skills/hive-lifecycle/` (`SKILL.md`, `agents/openai.yaml`) | immutable seed pins: git blobs `da3dcef568eef77e82a0a1ba9555a28416cc88c6` (SKILL.md) and `22ee02b5541293bf479f64ba903e79ead278e9a6` (openai.yaml), the port as first committed at `7e649d6`; validated by the Codex skill validator per #265 kickoff evidence | Codex dialect seeds |

## Requirements

- **R1 — Canonical home selected and recorded.** The hive-lifecycle skill's
  versioned home is `transpara-ai/hive:skills/hive-lifecycle/` (per-feature
  home: the skill manages this repo's stack). The convention itself — feature
  home + dialect subfolders (`claude/`, `codex/`, future dialects) — is
  recorded in `skills/README.md` with the operator verdict cited.
- **R2 — Both dialects seeded from the cited sources, one physical copy each.** The
  Claude dialect's physical file is the repo's pre-existing
  `.claude/skills/hive-lifecycle/SKILL.md` (committed via hive#259; verified
  identical to the local install), reached from the feature home via the
  relative symlink `skills/hive-lifecycle/claude`; the Codex dialect files
  `skills/hive-lifecycle/codex/{SKILL.md, agents/openai.yaml}` are seeded from
  the local port cited above. Both dialects diverge from their seeds ONLY by
  the enumerated R7 safety repairs; any other content delta is a defect
  (verification: `diff` against each seed shows exclusively R7 changes). No
  dialect content exists twice in the repo.
  (v0.2.0: revised from committing a second Claude copy after IAR found the
  pre-existing #259 home; moving the physical file out of `.claude/skills/`
  would break Claude Code project-skill auto-discovery, so the symlink points
  home-to-file rather than file-to-home.)
- **R3 — Codex structure valid.** The in-repo Codex dialect passes the Codex
  skill validator (`quick_validate.py`), including frontmatter and UI
  discovery metadata (`agents/openai.yaml`).
- **R4 — Safety posture preserved.** Both dialects default to read-only
  help/status; mutating lifecycle actions require explicit user intent in the
  current turn; no `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` use.
- **R5 — No private addresses or secrets.** No literal private-network
  addresses (hostnames/`localhost` only; a loopback literal is permitted only
  inside a verbatim quote of actual log output) and no non-default credentials
  or secrets anywhere under `skills/`, scanned with symlink traversal
  (`grep -R`) so the `claude` dialect symlink's target is covered. Checked-in local development defaults such
  as the `dev` bearer and local Postgres DSN are explicitly allowed and are
  never represented as production credentials.
- **R7 — Reviewed safety repairs (v0.3.0–v0.24.0, CFAR rounds 1–22 on hive#267).** Both
  dialects carry exactly these enumerated content repairs, applied identically
  where the defect exists in each: (a) environment checks print variable
  names only, never values (`env | cut -d= -f1 …`; `systemctl … -p
  Environment` filtered to names) so credentials cannot land in transcripts;
  (b) `hive status` crash-loop handling is diagnostic-only — starting
  Postgres is a separate mutating recovery action requiring explicit user
  confirmation; (c) the `approve-role` CLI's side effect is disclosed
  (`agent.budget.adjusted`, initial budget 200) and requires explicit user
  approval for both role and budget; (d) all Postgres readiness waits are
  bounded (60 attempts) with failure diagnostics instead of unbounded
  `until` loops; (e) install instructions synchronize with deletion
  (`rsync -a --delete`) so stale files cannot survive in installed copies.
  Local installs re-sync from the repo after merge.
  Round 2 (v0.4.0): (f) `council` examples pin `--api` to the local endpoint
  and disclose that the default `--api https://transpara.ai` posts up to 2000
  characters of the deliberation report to the remote social feed when
  `LOVYOU_API_KEY` is set — remote publishing requires explicit user
  authorization; (g) post-timeout gating — after a bounded Postgres wait, the
  dependent `systemctl start`/`restart` runs only inside an `if pg_isready`
  gate, so a timeout stops the operation instead of crash-looping services
  (repairing the round-1 `break` that fell through).
  Round 3 (v0.5.0): (h) council examples additionally replace any ambient
  remote `LOVYOU_API_KEY` with the non-secret local `dev` credential for local
  runs — `runCouncilCmd` reads the credential and
  `buildCouncilOperateInstruction` interpolates it into every council agent's
  prompt, so pinning `--api` alone still exposed the bearer token to model
  providers; (i) the readiness gate now encloses ALL downstream mutating
  steps — the optional runtime daemon launch (Hive Up step 3) and the hive
  unit bounce in both restart sections run only inside the `pg_isready`
  success branch (round-2's gate covered only the API services).
  Round 4 (v0.6.0): (j) both Common Problems tables keep crash-loop
  diagnosis read-only and require explicit user confirmation before proposing
  the separate mutating Postgres recovery action. Code inspection also
  confirmed that `buildCouncilOperateInstruction` is reachable only from the
  standalone `council` verb, not from `civilization run` or `civilization
  daemon`; those examples are therefore outside the council credential-prompt
  path repaired in (h). The same round also (k) makes `pgrep`/`pkill` patterns
  self-match-resistant, (l) bounds read-only HTTP probes with connection and
  total timeouts, and (m) documents the local `dev` credential honestly rather
  than claiming all checked-in development defaults are secrets.
  Round 5 (v0.7.0): (n) R2 rewritten from "verbatim byte-copies" to
  "seeded + enumerated R7 deltas" — after rounds 1–4 the byte-identity claim
  was false and its verification could never pass; (o) the four loopback
  literals in our own phrasing normalized to `localhost`/`loopback` (the one
  remaining `127.0.0.1` quotes an actual journalctl error line verbatim), and
  the R5 scan now traverses the `claude` dialect symlink (`grep -R`).
  Round 6 (v0.8.0): (p) writer-mode and catalog checks inspect the RUNNING
  process's effective environment by name (`/proc/PID/environ`, fail-closed
  "mode UNKNOWN; do not POST" when the service is down) — unit `Environment=`
  lines miss variables inherited from the systemd `--user` manager, so the
  prior check could misreport writer mode as read-only.
  Round 7 (v0.9.0): (q) the Claude dialect's endpoint-reference writer-mode
  note carries the same fail-closed effective-environment check — round 6 had
  mirrored only its catalog check, leaving the endpoint note asserting
  read-only from the unit file alone.
  Round 8 (v0.10.0): (r) the offline `localapi` example binds explicitly to
  `localhost:8082` — `--addr :8082` exposed the dev-credential API (including
  mutating board routes) on every interface; (s) the pipeline/role examples
  override `LOVYOU_API_KEY=dev` like council, so an ambient remote key sourced
  from `hive.env` is never forwarded to the local API (which expects `dev` and
  would 401); (t) the FO's seed citations pin immutable git blob SHAs from the
  branch's first commit `7e649d6` instead of mutable home-directory paths, so
  the R2 seed-vs-R7-delta diff stays reproducible after installs re-sync.
  Round 9 (v0.11.0): (u) `civilization run`/`daemon` examples blank
  `LOVYOU_API_KEY` — their Site API defaults to `https://transpara.ai` and an
  ambient key enables a reconciliation loop plus task-completion mirror posts
  against production; both dialects also caution that `hive.service` must be
  checked for the key via the effective-environment check before start;
  (v) the writer-mode checks treat an unreadable `/proc` (restart race,
  permissions) as mode UNKNOWN instead of letting a failed pipeline print `0`
  and read as read-only; (w) the catalog checks print this variable's value
  only (a filepath, not a secret) so the claimed `catalog-mixed.yaml`
  resolution is actually verified, with UNKNOWN on read failure.
  Round 10 (v0.12.0): (x) the writer-mode probes capture the `/proc` read
  BEFORE any pipeline (`raw=$(cat …)` must succeed and be non-empty) — the
  round-9 form piped `tr` into `cut`, and without pipefail a mid-restart read
  failure still exited 0 and printed `0`/read-only; (y) `hive.service` gets a
  usable pre-start credential preflight (unit `Environment=` names,
  each `EnvironmentFile` by name, and the user-manager environment — any hit
  or unreadable source blocks start), replacing the round-9 caution that
  pointed at a running-process check unusable on a stopped unit, plus a
  user-confirmed `UnsetEnvironment=LOVYOU_API_KEY` clearing drop-in and a
  post-start effective-environment verification.
  Round 11 (v0.13.0): (z) the writer-mode probes convert NULs to newlines
  INSIDE the command substitution — the round-10 `raw=$(cat …)` form lost the
  NUL separators (bash substitution strips them), concatenating all
  assignments so the check always printed `0`/read-only even in writer mode;
  `tr` is the sole command so a failed read still fails the condition;
  (aa) the preflight parses every `Environment=` assignment (quoted and
  multi-assignment forms; over-matching split values errs closed) and matches
  `EnvironmentFile` lines with leading whitespace/`export`/quotes; (ab) the
  preflight verdict recognizes the active `UnsetEnvironment` clearing drop-in
  so the documented local-only recovery path can actually reach start and
  post-start verification instead of dead-ending on the unchanged source
  file.
  Round 12 (v0.14.0): (ac) the restart sections no longer bounce
  `hive.service` automatically — the branch now requires the credential
  preflight to pass AND detects full-autonomy flags in the effective
  `ExecStart` (restarting the packaged unit resumes FULL AUTONOMY), demanding
  explicit current-turn approval; (ad) same gate covers the ambient-credential
  restart hazard (an active unit fed by `EnvironmentFile`/manager env would
  resume production reconciliation on a plain restart); (ae) both Stack
  Components tables report the real bindings — compose publishes Postgres as
  `5432:5432` on ALL interfaces (dev credentials) and work-server binds
  `":"+PORT` on all interfaces — instead of claiming loopback.
  Round 13 (v0.15.0): (af) the preflight reads systemd's MERGED EFFECTIVE
  properties (`systemctl show -p Environment/-p EnvironmentFiles/
  -p UnsetEnvironment/-p ExecStart --value`) instead of text-parsing
  `systemctl cat` output — eliminating the whole class of quoting/whitespace
  misses (`Environment = 'K=v'`) and later-fragment list resets that made a
  raw `UnsetEnvironment=LOVYOU_API_KEY` line look active after a reset;
  (ag) the preflight verdict now also gates full-autonomy flags found in the
  merged `ExecStart` before any START (previously only restart checked);
  (ah) EnvironmentFile name matching accepts single- or double-quoted and
  whitespace-padded assignments (over-matching errs closed).
  Round 14 (v0.16.0): (ai) a failed `show-environment` read now sets
  UNKNOWN instead of reading as "no manager credential" (fail-open); (aj) the
  `EnvironmentFiles` tuple stripping is global, so multi-file units no longer
  leave intermediate `(ignore_errors=…)` tokens that read as unreadable
  filenames and wrongly block every start.
  Round 15 (v0.17.0): (ak) the `EnvironmentFiles` and `ExecStart` property
  reads are captured and checked before parsing — a failed `systemctl show`
  previously read as "no files"/"no autonomy flags" (fail-open) because the
  trailing `sed`/`grep` masked the failure; the `UnsetEnvironment` read
  fails safe already (failure leaves the credential verdict at do-NOT-start);
  (al) the Codex dialect's restart branch points at the preflight's actual
  section (On-demand Runtime) instead of a nonexistent Hive Up block.
  Round 16 (v0.18.0): (am) the preflight also scans the merged `ExecStart`
  for `LOVYOU_API_KEY=` — an `env`/shell wrapper can inject the credential
  through the command line itself, which `UnsetEnvironment` cannot clear, so
  this case gets its own uncleearable do-NOT-start verdict branch.
  Round 17 (v0.19.0): (an) launcher allowlist — the preflight recognizes only
  direct, argv-transparent launchers (`…/go`, `…/hive`); any other ExecStart
  executable (bash/sh/env/custom script) is an opaque wrapper whose body can
  source credentials or add flags invisibly to every property check, and is
  treated as unknown → do NOT start (a string miss is not proof of safety).
  Round 18 (v0.20.0): (ao) the launcher allowlist binds to exact canonical
  targets — the built binary's exact path, or `go` only with
  `WorkingDirectory=/Transpara/transpara-ai/repos/hive` and a
  `go run ./cmd/hive` argv — so `/tmp/hive` wrappers and
  `go run /tmp/wrapper.go` no longer pass the suffix match; (ap) the
  writer-mode probe reads `HIVE_OPS_HUMAN_ACTOR`'s value (an operator actor
  id, not a secret): `opsWriterOptions` stays read-only for an empty or
  invalid id, so presence-counting over-claimed writer mode.
  Round 19 (v0.21.0): (aq) the preflight verdict reports EVERY blocker —
  the elif chain surfaced only the first, so an operator authorizing the
  credential blocker never saw the separate full-autonomy approval the
  packaged unit also requires; (ar) the go branch accepts only trusted
  toolchain paths (`/snap/bin/go`, `/usr/bin/go`, `/usr/local/go/bin/go`) —
  a `/tmp/go` or version-manager shim with canonical-looking arguments is
  still an opaque wrapper; (as) the catalog probe reads `/proc/PID/cmdline`
  first because the `--catalog` flag overrides `HIVE_OPS_CATALOG` (the
  flag's default is the env value), falling back to the env var, UNKNOWN on
  unreadable.
  Round 20 (v0.22.0): (at) the preflight fails closed on any populated
  auxiliary exec phase (`ExecStartPre`, `ExecCondition`, `ExecStartPost`) —
  a drop-in can attach opaque commands around start that no `ExecStart`
  analysis sees; the recognized unit shape has ONLY `ExecStart`.
  Round 21 (v0.23.0): (au) the recognized shape requires exactly ONE
  `ExecStart` command — `Type=oneshot` units may chain several, and a
  first-entry check would pass a canonical command followed by an opaque
  one; (av) status/restart diagnostics report runtime PIDs and executable
  names only (`ps -o pid=,comm=`) — `pgrep -af` wrote full argv into
  transcripts, which can carry sensitive `--idea` text or credential
  assignments; the stop-and-rerun messages now ask the user for their
  original command instead of echoing it.
  Round 22 (v0.24.0): (aw) the unit-shape check also fails closed on
  populated `ExecStop`/`ExecStopPost` — restart executes stop hooks before
  the new start, so an opaque stop hook could rewrite an `EnvironmentFile`
  after the scan; (ax) a concrete post-start verification block for unit
  `hive` / variable `LOVYOU_API_KEY` replaces the dangling pointer to the
  hive-ops-api probe — the running process's names-only environ read is the
  authoritative proof the clearing took effect, UNKNOWN reads as NOT
  cleared.
- **R6 — Update path defined.** Future changes to lifecycle commands or
  safety boundaries are reviewed via governed PRs on this repo (TLC arc with
  cross-family review); installed copies are caches, repo is truth
  (`skills/README.md`, `skills/hive-lifecycle/README.md`).

## Non-Goals

- No Hive start/stop/restart, daemon launch, or runtime execution.
- No changes to the skill's commands or semantics beyond the reviewed safety
  repairs enumerated in R7 — every other content diff from the cited sources
  is a defect.
- No installer tooling or symlink automation (a later slice if wanted).
- No relocation of other skills; this arc moves exactly one feature.

## Verification Plan

- `python3 ~/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/hive-lifecycle/codex` → valid.
- `diff` of committed dialect files against their cited seeds → only the
  enumerated R7 repairs differ.
- Private-address/secret scan over `skills/` with `grep -R` (follows the
  `claude` symlink) → no matches outside verbatim log quotes.
- `git diff --check` clean; repo build/tests unaffected (no Go changes).
- IAR then CFAR (Codex reviewer; Claude author) at the exact PR head; merge
  consideration remains Michael's.

## Non-Authorizations

This Factory Order states intent and grants nothing beyond the governed PR
flow. It does not authorize any lifecycle mutation the skill documents, nor
any label (`cc:*`) movement on the source issue — label state remains
human-owned.
