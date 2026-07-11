---
doc_id: FO-HIVE-265-LIFECYCLE-SKILL-HOME
title: Factory Order — Canonical Versioned Home for the hive-lifecycle Skill (Claude + Codex Dialects)
doc_type: factory-order
status: proposal
version: 0.10.0
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
- **R7 — Reviewed safety repairs (v0.3.0–v0.10.0, CFAR rounds 1–8 on hive#267).** Both
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
