---
doc_id: FO-HIVE-TRANSPARA-API-KEY-RENAME
title: Factory Order — Retire the LOVYOU_* Environment Variable Family (TRANSPARA_* Rename) and Externalize the Committed Credential
doc_type: factory-order
status: proposal
version: 0.11.0
created: 2026-07-12
updated: 2026-07-13
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
source_issue: none — channel A human order (intake, restatement, and confirmation archived and hash-pinned below)
authority: repository code/documentation rename and credential externalization only; no key rotation, no git-history rewrite, no Hive start/stop/restart, no runtime execution, no deploy, no default-branch mutation, no merge, no upstream (lovyou-ai) contact, no EventGraph write, no issue mutation
---

# Factory Order — Retire the LOVYOU_* Environment Variable Family and Externalize the Committed Credential

## Immutable Source Citations

| Source | Pin | Role |
|---|---|---|
| Michael Saucier, in-session channel A order, 2026-07-12 | verbatim intake archived at `artifacts/hive/fo-transpara-api-key-rename/intake-2026-07-12.md`, sha256 `76b30d68d9dbfbd29ba93607ca9dd3fd4fbdf482092463294c1fe3bcf529c30d` | Raw intake — "craft a Factory Order to rename LOVYOU_API_KEY to TRANSPARA_API_KEY" |
| Michael Saucier, plain-language restatement he was asked to confirm, 2026-07-12 | verbatim restatement archived at `artifacts/hive/fo-transpara-api-key-rename/intake-restatement-2026-07-12.md`, sha256 `1e6a52b03125f6ee4ced06433af8276df10b0ab10e0d92b227a45ba32ec85f35`; chain: restates FO v0.1.0 (blob `df8aa1c2ea64f0e5b95f82fb245446a69e8f2e94`) | The exact reading answer "1. yes" confirms — pins credential removal (item 5), the operator runbook (item 6), historical-records protection (item 7), and the hive-repo scope (item 1) to an immutable referent |
| Michael Saucier, in-session channel A intake confirmation, 2026-07-12 | verbatim answers archived at `artifacts/hive/fo-transpara-api-key-rename/intake-confirmation-2026-07-12.md`, sha256 `b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310` | "1. yes" (reading confirmed); "2. all 3 old name variables" (family scope); "3. leave it" (no history rewrite) |
| `transpara-ai/docs:designs/hive-kpi-dogfood-prompt-2.1-post-rename-chores-output-v0.1.0.md` (lines 85, 264) and `…prompt-2.2-profile-shim-and-memory-output-v0.1.0.md` (line 164) | as merged in transpara-ai/docs main at intake time | Prior recorded observation that plaintext `LOVYOU_API_KEY` remained in `hive.env` after the repo/container rename chores — the rename this FO completes |
| `transpara-ai/hive@3ff5e54` (2026-03-26, `[hive:builder] Implement agent personas infrastructure phase 1`) | commit that introduced `loop/mcp-graph.json` carrying a literal credential value (`lv_b7fb22…`, 64 hex chars) into the PUBLIC hive repo | Exposure evidence grounding R5 and the named residual risk; discovered during 2026-07-12 intake investigation in this session |
| `transpara-ai/hive@bf3f126` (main HEAD at intake) | tracked-file inventory (`git grep -lE`, `-nE`, `-oE` over the three names): **31 files, 121 matching lines, 124 occurrences** — 26 functional files (21 Go files under `cmd/` and `pkg/`; both skill dialects; `agents/CONTEXT.md`; `loop/mcp-graph.json`; `loop/hive-runtime-spec.md`) and 5 historical/immutable files (`docs/factory-orders/FO-hive-265-lifecycle-skill-home-v0.58.0.md`, `docs/superpowers/plans/2026-04-14-offline-pipeline.md`, `docs/superpowers/plans/2026-05-14-route-claude-p-through-modelconfig-plan.md`, `docs/superpowers/specs/2026-04-18-hive-cli-redesign-design.md`, `loop/reflections.md`) | Scope inventory the requirements below enumerate; the 5 historical files are R7-protected and stay |
| Repo-set sweep, 2026-07-12 session (persisted search over `/Transpara/transpara-ai`) | functional (executable/config) references to the three names exist ONLY in `transpara-ai/hive`; all other appearances are copies of hive (worktrees, the `site` `third_party/hive` git submodule pin) or historical records (wiki Open Brain exports, docs design outputs) | Scope justification: the hive-only scope in restatement item 1 (confirmed "yes") covers the entire functional domain; no other repo has anything to rename |

## Intake Reading (channel A — CONFIRMED 2026-07-12 against the pinned restatement)

Structured reading of the order, confirmed by the operator against the
hash-pinned restatement above: retire the lovyou-branded environment variable
family in `transpara-ai/hive` — rename `LOVYOU_API_KEY` → `TRANSPARA_API_KEY`,
`LOVYOU_BASE_URL` → `TRANSPARA_BASE_URL`, and `LOVYOU_SPACE` →
`TRANSPARA_SPACE` — across every functional surface, as a hard cutover with no
fallback, with the committed credential value removed from
`loop/mcp-graph.json`, historical records untouched, and no git-history
rewrite. `LOVYOU_API_KEY` is the Bearer-token credential the hive CLI and
runner agents send to the transpara.ai JSON API; `LOVYOU_BASE_URL` and
`LOVYOU_SPACE` are optional companions (API origin, default space slug) with
safe built-in defaults. All three names are inherited from the lovyou-ai
upstream fork and predate the client's retargeting to transpara.ai.

Both v0.1.0 open intake questions are settled by the archived confirmation:

- **Q1 — Family scope: ANSWERED "all 3".** `LOVYOU_BASE_URL` and `LOVYOU_SPACE`
  are in scope (R1, R3).
- **Q2 — History purge: ANSWERED "leave it".** No git-history rewrite; the
  exposed value is neutralized by operator rotation (named residual), and the
  historical bytes remain.
- **D-1 — RESOLVED (operator decision, 2026-07-12): (a) preserve.** The Codex
  auditor (round 2 finding 6) surfaced that the confirmed restatement's
  general posture (item 3, "an empty key means run locally, don't talk to the
  website") conflicts with items 2-3's "behavior stays exactly the same —
  only the name changes" at the un-gated empty-key sites — council
  (`cmd/hive/main.go:506-512`) applies no empty-key gate (it constructs
  `api.New(apiBase, "")` and posts to its `--api` target, default local,
  regardless of key) unlike the civilization runtime, which disables its Site
  client on an empty key (`cmd/hive/main.go:1284-1288`); `cmd/mcp-graph`, critic,
  and the runner instruction-builders are the other un-gated readers (see the
  class residual below). Michael chose **(a) preserve**: this order renames only and changes
  no behavior, so every one of these empty-key paths is carried verbatim and
  recorded as a Named Residual Risk below; aligning them to the empty-key
  posture is a behavior change deferred as a class to its own future governed
  order (owner Michael) so it is not lost. This resolution restores the
  hard-cutover / behavior-preserving spine (R2/R3); D-1 no longer blocks the
  design gate.

## The R1 Scan Protocol

To prevent the escaped-regex false-pass class (a regex copied out of a
markdown table with `\|` matches nothing and records fake success), the scan
command is defined ONCE, here, in a fenced block, and every reference to "the
R1 scan" means exactly these bytes:

```sh
git grep -nE 'LOVYOU_(API_KEY|BASE_URL|SPACE)' -- . \
  ':(exclude)docs/factory-orders' \
  ':(exclude)docs/superpowers' \
  ':(exclude)docs/designs' \
  ':(exclude)docs/.adversarial-design' \
  ':(exclude)docs/runbooks/transpara-env-rename-migration.md' \
  ':(exclude)loop/reflections.md'
```

Interpretation is fixed and fail-closed:

- **PASS** only when the command prints nothing AND exits with status **1**
  (`git grep`'s no-match status).
- Exit **0** (matches printed) ⇒ FAIL — legacy names remain.
- Exit **≥ 2** (error) ⇒ DENY — the scan did not run; never treat as pass.
- **Base-liveness precondition:** the identical command run at the branch
  base MUST exit 0 with ≥ 1 match, recorded in evidence. A scan that finds
  nothing before the rename is a broken scan, not a clean tree (at
  `bf3f126` it prints 94 matching lines).

The exclusions are each either an R7-protected immutable record or a
governance/runbook doc added by this order that must quote the legacy names
to do its job. Everything else tracked — including files that do not exist at
design time — must be clean.

## Requirements

- **R1 — Complete family rename in functional surfaces.** Every functional
  reference to the three old names in `transpara-ai/hive` is renamed
  (`LOVYOU_API_KEY` → `TRANSPARA_API_KEY`, `LOVYOU_BASE_URL` →
  `TRANSPARA_BASE_URL`, `LOVYOU_SPACE` → `TRANSPARA_SPACE`):
  Go sources and tests (`cmd/hive/{main,factory,factory_preflight_hive_unit}.go`,
  `cmd/hive/{ingest_test,router_test,factory_preflight_hive_unit_test}.go`,
  `cmd/{mind,post,reply,mcp-graph,republish-lessons}/main.go`,
  `pkg/api/client.go`,
  `pkg/runner/{architect,council,critic,observer,pm,reflector,scout,scribe,spawner}.go`),
  both hive-lifecycle skill dialects (`.claude/skills/hive-lifecycle/SKILL.md`
  reached via the `skills/hive-lifecycle/claude` symlink, and
  `skills/hive-lifecycle/codex/SKILL.md`), `agents/CONTEXT.md`,
  `loop/mcp-graph.json` (both keys), and current-facing operational docs that
  describe runtime behavior (`loop/hive-runtime-spec.md`).
  *Verify:* the R1 Scan Protocol above (PASS definition and base-liveness
  precondition included), and `go build ./...` passes.
  *Rationale:* the variable family is the last executable lovyou-brand remnant
  after the 2026-04-22 repo/container renames; operator confirmed family-wide
  scope on 2026-07-12.
- **R2 — Hard cutover, no fallback.** After the change no code path reads any
  of the three old names: no dual-read, alias, or deprecation shim. Absent or
  empty new-name values behave exactly as absent/empty old-name values do
  today, per the call-site truth table fixed in the design packet.
  *Verify:* R1's scan plus `go test ./...` green including the named tests of
  the design packet's proof map.
  *Rationale:* a fallback keeps the legacy channel alive indefinitely
  (fail-open); operator standard is env-var secrets with no fallback.
- **R3 — Semantics preserved per variable and per call site.** The behavioral
  meaning of set / empty / absent at EVERY reading call site survives the
  rename verbatim under the new names, as does each built-in default
  (`TRANSPARA_BASE_URL` → `https://transpara.ai`; `TRANSPARA_SPACE` → `hive`).
  The design packet fixes a normative per-call-site truth table — recording
  each site's EXACT input partition (e.g. the webhook gate rejects blank
  values, empty or whitespace-only, via TrimSpace, while the other *gating*
  sites compare the raw value to empty and several sites — council, critic, and
  the seven non-branching runners — do not compare the key at all, interpolating
  it verbatim) — and a determinate proof map assigning
  exactly one proof per site: (a) a named test where the site has
  key-conditional behavior and a test file + seam exist (extending the
  existing test where one already covers the site — extensions may add
  missing input-partition cases such as whitespace-only, asserting current
  behavior only); (b) code inspection cited at file:line where the site has
  no key-conditional branch (verbatim interpolation) or no test file/seam
  exists. The assignment is fixed in the packet; nothing is left as an
  either/or at review time, and no behavioral change of any kind rides the
  rename (including preserving currently-odd behaviors, which may be flagged
  as observations for future orders but not altered here).
  *Verify:* the proof map executed — every truth-table row carries either a
  green named test or a file:line inspection citation; skill-text scan shows
  posture language updated, not deleted.
- **R4 — Preflight credential probe renamed with collision-safety preserved.**
  The hive-unit preflight probe matches the prefix `TRANSPARA_API_KEY=`
  (trailing `=` to exclude sibling names; exact-length compare distinguishing
  EMPTY from PRESENT), reports posture only (`PRESENT/EMPTY/ABSENT/UNKNOWN`),
  never emits the value, and stays fail-closed (UNKNOWN on unreadable environ).
  *Verify:* the existing preflight test matrix
  (`TestEvaluateHiveUnitPreflightCoversCredentialPostures`,
  `TestParseHiveUnitPropertiesFailsClosedOnEmptyUnreadableAndMalformedInputs`,
  `TestRunHiveUnitPreflightRedactsPresentCredentialValue`,
  `TestRunHiveUnitPreflightReportsUnknownAndNonzero`) renamed to the new
  variable and green, plus a sibling-name non-match case
  (`TRANSPARA_API_KEY_BACKUP=` must not read as the credential).
  *Rationale:* these properties were established across CFAR rounds on
  hive#277-era work and must not regress in the rename.
- **R5 — Committed credential removed and externalized.** `loop/mcp-graph.json`
  carries no credential value and no credential key entry: the spawned MCP
  server inherits `TRANSPARA_API_KEY` from the operator's environment; the
  literal `lv_b7fb22…` line is deleted.
  *Verify:* `git grep -E 'lv_[0-9a-f]{16,}'` over the tracked tree prints
  nothing and exits 1 at the PR head (exit ≥ 2 ⇒ deny; baseline: exactly one
  hit, the line this requirement deletes; all governance docs keep the token
  truncated below the 16-hex threshold).
  *Rationale:* the value is live in a PUBLIC repo; the rename touches this
  exact file, so leaving the value while renaming its key would be absurd.
  Rotation of the exposed credential is an operator action outside this FO
  (see Named Residual Risks).
- **R6 — Operator migration runbook shipped.** The PR includes a documented,
  verifiable migration sequence for the live nucbuntu deployment: rename the
  key(s) in `hive.env`, update any systemd `--user` unit environment naming
  any of the three variables, re-sync local skill installs
  (`rsync -a --delete` per the FO-265 convention), and update local MCP
  config; post-migration verification is the preflight posture reading
  PRESENT under `TRANSPARA_API_KEY` with no old name present in the effective
  unit environment. Unit names and env-file consumers are resolved live at
  runbook-authoring time, never from a memorized list. The runbook is
  executed by the operator, never by this FO's PR.
  *Verify:* runbook present in the PR at
  `docs/runbooks/transpara-env-rename-migration.md`; each step names its
  check command; unit names carry a "resolved live on <date>" note.
- **R7 — Governed and historical records untouched.** No modification or
  deletion of ANY existing tracked file under `docs/factory-orders/`,
  `docs/designs/`, `docs/superpowers/`, `docs/.adversarial-design/`, or
  `docs/runbooks/`, nor of `loop/reflections.md` (nor of CFAR/evidence
  artifacts and wiki open-brain exports in other repos). Prior Factory
  Orders and design packets keep their bytes (gate credit binds to blob
  SHA); hive currently tracks 18 files under `docs/designs/` and they are
  all protected. The only additions permitted under the protected trees are
  this order's own governance docs: its FO into `docs/factory-orders/`, its
  design packet into `docs/designs/`, its IADA/CFADA evidence into
  `docs/.adversarial-design/`, and its runbook into `docs/runbooks/` —
  nothing else.
  *Verify:* `git diff --name-status --no-renames` against the base (rename
  detection disabled so a rename cannot mask as `R*` — hive history shows a
  protected Factory Order mutation reported as `R082` that `--no-renames`
  resolves to `D` + `A`). Under the protected trees the ONLY permitted
  status is `A`, and only for paths in the enumerated allowlist of this
  order's docs; any other status letter under a protected path — `M`, `D`,
  `R*`, `C*`, `T`, `U`, or anything else `git diff` can emit — fails the
  check (status allowlist, not a status denylist).

## Non-Goals

- Any repo in the `lovyou-ai` org, or any upstream contact — never in scope.
- Git-history rewrite/purge of the exposed value — **settled by operator
  answer 2026-07-12 ("leave it")**: history stays; rotation supersedes.
- Rotation of the exposed `lv_b7fb22…` credential on the transpara.ai service —
  urgent operator action, independent of and not blocked by this FO (a separate
  operator action tracked outside this FO).
- Behavioral changes of any kind, including "fixing" currently-odd behaviors
  observed during design (e.g. the un-gated empty-key class — council,
  `cmd/mcp-graph`, critic, and the runner curls; see Named Residual Risks) —
  per D-1 all are preserved verbatim here and deferred as a class to a future
  order.
- Server-side transpara.ai API changes (the service validates bearer tokens;
  it does not read these client-side variable names).
- `transpara-ai/site` submodule pin bump (`third_party/hive` is a submodule,
  not a copy — a routine pin advance after merge picks up the fix; no site
  content change exists to order).
- Rewriting wiki/Open Brain history or any provenance record.
- Scratch worktrees under `.claude/worktrees/` (untracked local copies).

## Constraints

- Full TLC arc: design packet → IADA → CFADA → Human Design Review → code →
  draft PR → IAR → CFAR → ready → Human Review (Michael merges). Author family
  Claude/Anthropic → reviewer family Codex/OpenAI per the role rule.
- One PR to `transpara-ai/hive` from a `feat/` branch; conventional commits;
  no direct push to `main`; merge is Michael's alone.
- Fail-closed defaults for this order's own changes — the rename adds no new
  fail-open path; no secret values in code, no default values for secret
  fields; `.env`-style files never committed. (Pre-existing un-gated empty-key
  paths are named and preserved, not introduced — see Named Residual Risks.)
- The skill dialects changed by R1 are governed content previously delivered
  under FO-HIVE-265-LIFECYCLE-SKILL-HOME; this FO is the governing order for
  the rename delta, and the design packet must cross-reference FO-265's R2
  "seeded + enumerated deltas" convention so neither order rejects the other's
  delivered bytes.
- In-flight interaction: `transpara-ai/hive#283`
  (`feat/hive-preflight-runbook-wiring`) touches both skill dialects and the
  runbook probe language. The design packet must re-inventory at its actual
  base and state the merge-order assumption; if #283 lands first, the skill
  and runbook line references above shift but the requirements are unchanged.
- The truth-object copies carried by the PR (FO, design packet, evidence)
  must be byte-identical to the cited artifacts — `git hash-object` equality.

## Verification Plan

At the PR head, in `transpara-ai/hive`:

1. The R1 Scan Protocol: base-liveness precondition recorded (exit 0, ≥ 1
   match at base), then PASS at the head (empty output, exit 1); exit ≥ 2
   anywhere ⇒ deny (R1, R2).
2. `git grep -E 'lv_[0-9a-f]{16,}'` → empty output, exit 1 (R5).
3. `go build ./...` and `go test ./...` → pass, including every named test in
   the design packet's proof map (R2, R3, R4).
4. `grep -R 'TRANSPARA_API_KEY' skills/` (symlink-traversing) shows both
   dialects updated; `grep -RE 'LOVYOU_' skills/` → no matches (R1, R3).
5. `git diff --name-status --no-renames` against base: under
   `docs/factory-orders/`, `docs/designs/`, `docs/superpowers/`,
   `docs/.adversarial-design/`, `docs/runbooks/`, and `loop/reflections.md`
   the only status present is `A`, and every `A` path is in the enumerated
   governance-doc allowlist; any other status under those paths fails (R7).
6. Post-merge, operator-executed: runbook steps from R6 with their named
   checks (preflight posture PRESENT under the new name; no `LOVYOU_`
   variable in the effective unit environment).

## Named Residual Risks

- **Exposed credential in public history.** `lv_b7fb22…` has been public in
  `transpara-ai/hive` history since 2026-03-26 (committed autonomously by a
  hive builder agent). This FO removes it from the tree (R5) but cannot close
  the exposure: only operator rotation on the transpara.ai service kills the
  value. Per the operator's 2026-07-12 answer, history is not rewritten, so
  the historical bytes remain public permanently; rotation is therefore the
  sole neutralizer. Residual owner: Michael (rotation is a separate operator
  action tracked outside this FO; its outcome is not assumed here). This FO
  neither performs nor authorizes rotation.
- **Un-gated empty-key readers (D-1: preserve; deferred as a class).** Several
  readers — council (`cmd/hive/main.go:506-512`), `cmd/mcp-graph`
  (`apiGet`/`apiPost`), critic, and the runner instruction-builders — read the
  API key without a uniform empty-key gate and interpolate or send it regardless
  (unlike ingest, civilization, and webhook, which fail safe on an empty key).
  On an empty key, `cmd/mcp-graph` defaults to the remote
  (`TRANSPARA_BASE_URL`→`https://transpara.ai`) while the others' remote-
  reachability depends on the operator's `--api`/base configuration. This is a
  pre-existing confidentiality concern, independent of and unchanged by this
  rename (D-1: preserve). Its **exhaustive per-site characterization and any
  remediation** — and the correction of the stale operator-doc claims found
  during design (e.g. a skill warning's council `--api` default, mcp-graph's key
  doc-comment, the webhook bind description) — are the scope of the deferred
  future governed order, not this rename. Interim operator mitigation: pin
  `--api` to a local host and set a local `TRANSPARA_BASE_URL` for mcp-graph.
  Owner: Michael. This FO neither performs nor authorizes the behavior change.
- **Deployment window.** Between merge and the R6 operator migration, a
  freshly built binary reads the `TRANSPARA_*` names while the live `hive.env`
  still exports `LOVYOU_API_KEY`. Under the design packet's truth table the
  key-*gated* paths (ingest, pipeline, role, civilization, webhook) are
  fail-closed or default-safe in that window (required-error, skip-with-notice,
  Site client disabled, built-in defaults). The un-gated empty-key readers named
  above are preserved, not introduced: on an empty key only `cmd/mcp-graph`
  defaults to the remote (mitigated by a local `TRANSPARA_BASE_URL`); the
  others' reachability depends on operator `--api`/base config (see the
  residual). This is unchanged by the rename. The runbook orders the migration
  steps to make the window zero for the managed services.

## Non-Authorizations

This Factory Order states intent and grants nothing: no key rotation, no
git-history rewrite, no issue mutation, no automatic PR creation beyond the
TLC-governed PR itself, no default-branch mutation, no merge, no runtime
execution, no service restart, no deploy, no EventGraph write, no Hive
authority/write/action API use, no value allocation, no autonomy increase,
no residual-risk closure, and no Test 001 closure. Merging stays with Michael
at stage 12.

## Revision History

- **v0.11.0 (2026-07-13, CFADA round-7 repairs; operator chose "narrow")** —
  After seven rounds where the design core passed but the empty-key
  characterization and operator-doc accuracy kept generating findings, the
  operator narrowed scope. Dropped **R8** (doc-accuracy) and its Verification
  Plan item — a rename is not a doc audit. Shrank the D-1 Named Residual to a
  minimal accurate statement (the un-gated readers read the key without a
  uniform empty-key gate; on an empty key only mcp-graph defaults remote, the
  others' reachability depends on operator `--api`/base config), and deferred
  BOTH the exhaustive per-site characterization AND the correction of the stale
  operator-doc claims (council `--api` default, mcp-graph key doc-comment,
  webhook bind description) to the follow-up order the residual names. This
  removes the two round-over-round churn sources — over-precise reachability
  classification and unbounded doc-accuracy. Resolves the round-7 blockers: (1)
  the residual no longer classifies council-runner gating (avoiding the
  `runCouncilCmd` mis-grouping); (2) no D5/D9 contradiction (D9 removed); (3)
  R8's unbounded doc-accuracy removed. The rename (R1-R4), credential removal
  (R5), runbook (R6), and governed-records protection (R7) are unchanged; AC
  count back to 9.
- **v0.10.0 (2026-07-12, CFADA round-6 repairs; operator chose "right-size")**
  — (R6-1) the un-gated empty-key class enumerated only three readers; it now
  includes the seven non-branching runner instruction-builders (architect,
  council, pm, reflector, scout, scribe, spawner) that also interpolate the key
  ungated. (R6-2) the pipeline/role empty-key locality gate is a
  `strings.Contains(apiBase,"localhost"|"127.0.0.1")` SUBSTRING test — so
  "critic never reaches a remote" was itself an overstatement (a crafted host
  like `https://localhost.example.invalid` passes); the residual now states the
  exact predicate. Per the operator's "right-size" direction, the residual
  gives the accurate + complete enumeration and the exact predicate but hands
  the exhaustive per-site reachability analysis and any remediation to the
  deferred future order rather than pinning unchanged behavior here. (R6-3) new
  **R8** — the edited operator docs must not ship known-false runtime claims:
  the hive-lifecycle dialects' council `--api` default (localhost:8082, not
  transpara.ai) and mcp-graph's "required" key doc-comment (the key is optional)
  are corrected (claim-accuracy only, no runtime change), with a
  Verification-Plan check and packet AC-10. No scan protocol or gate-predicate
  structure changed (AC count 9→10).
- **v0.9.0 (2026-07-12, CFADA round-5 repairs; findings by the Codex
  auditor)** — (R5-1) FO Constraints' "Fail-closed defaults throughout"
  universal contradicted the preserved un-gated paths — narrowed to the order's
  own changes, with a carve-out. (R5-2, substantive) the empty-key reachability
  was OVERSTATED: `cmd/hive/router.go:173,234,286` default council/pipeline/role
  `--api` to `http://localhost:8082`, and `runRunner`/`runPipeline`
  (`main.go:445-451,601-607`) reject an empty key against a non-local base — so
  **only `cmd/mcp-graph`** (which defaults to `TRANSPARA_BASE_URL`→remote and
  has no `--api` flag) contacts the default remote on an empty key; council
  defaults local (remote only under an explicit remote `--api`) and critic's
  empty-key path is pipeline-gated to local. The class is renamed **"un-gated
  empty-key paths"** and every section (D-1 block, Named Residual, Deployment
  Window, Non-Goals) restated with accurate per-path reachability; the R4-3
  mitigation split is retained (Codex confirmed it accurate). (R5-3) the
  packet's §7 heading named a stale FO version — fixed in packet v0.8.0. No
  requirement, scan protocol, verification step, or gate predicate changed.
- **v0.8.0 (2026-07-12, CFADA round-4 repairs; findings by the Codex
  auditor)** — Three blockers, each the SAME false claim the round-3 repair
  fixed in one section but left standing in a sibling — the "fix the class,
  not the instance" failure: (R4-1) the "never fail-open" claim survived in
  packet D6 though the Deployment Window/§7 were fixed — corrected. (R4-2) the
  false "every other site compares raw to empty" universal survived in this
  FO's R3 requirement text though packet D4 was fixed — corrected. (R4-3) the
  class mitigation over-claimed a single `--api` pin for the whole class, but
  `cmd/mcp-graph` takes no `--api` flag (it reads `TRANSPARA_BASE_URL`,
  default `https://transpara.ai`) — the mitigation is now split path-by-path
  (`--api` pin for council/critic; local `TRANSPARA_BASE_URL` for mcp-graph)
  in every affected section of both docs. (Obs) the "remediation session
  opened separately" note survived in Non-Goals — removed. A full four-class
  sweep of both documents confirms zero remaining live instances. No
  requirement, scan protocol, verification step, or gate predicate changed.
- **v0.7.0 (2026-07-12, CFADA round-3 repairs; findings by the Codex
  auditor)** — Four blockers, all claim-accuracy defects (no behavior change;
  the rename still preserves everything verbatim): (R3-F2) the empty-key
  conflict was falsely scoped to council — `cmd/mcp-graph` (`apiGet`/`apiPost`
  always `client.Do`) and critic's unconditional curl reach the default remote
  on an empty key too; the D-1 block, Named Residual Risk, Non-Goals bullet,
  and Deployment Window are re-scoped to the empty-key-contacts-remote CLASS
  and the deferred order now covers the class. (R3-F4) the residual's mitigation
  was wrong — the hive-lifecycle skill sets the key to `dev` (not blank) and it
  is the local `--api` pin, not the key, that keeps content local; corrected.
  (R3-F1, R3-F3) the packet's critic row and the webhook row's "all other
  sites compare raw to empty" universal were false; corrected there (see packet
  v0.6.0). (Obs) the "rotation remediation session opened separately" note lost
  its unverifiable identifier. No requirement, scan protocol, verification step,
  or gate predicate changed.
- **v0.6.0 (2026-07-12, operator decision D-1 resolved)** — Michael chose
  **(a) preserve** for the council empty-key conflict the Codex round-2
  auditor raised (round 2 finding 6). This order renames only and preserves
  council's verbatim empty-bearer construction; the oddity is now recorded as
  a Named Residual Risk (owner Michael) for the deferred one-site
  behavior-alignment. D-1 no longer blocks CFADA; the
  hard-cutover / behavior-preserving spine (R2/R3) is intact. No requirement,
  scan protocol, verification step, or gate predicate changed — this is a
  decision-resolution and residual-recording bump only.
- **v0.5.0 (2026-07-12, CFADA round-2 repairs; findings by the Codex
  auditor)** — (R2-F3) R7 verification hardened from a status denylist
  (no `M`/`D`) to a status ALLOWLIST under `--no-renames`: only `A` on
  enumerated paths passes; `R*`/`C*`/`T`/unknown statuses now fail (hive
  history shows an `R082` rename masking a protected-FO mutation). (R2-F4)
  R3 now records each site's exact input partition — the webhook gate
  rejects blank (TrimSpace), all other sites compare raw-empty — and
  sanctions asserting-current-behavior extensions of existing tests (the
  whole-domain webhook test gains a whitespace-only case). (R2-F5) The R3
  proof rule is three-way determinate: key-conditional + test file/seam ⇒
  named test; non-branching interpolation or no test file/seam ⇒
  inspection — eliminating the false "no runner seams" classification
  (observer and critic have key-behavior tests; the other seven runners
  interpolate without branching). (R2-F6) New OPEN OPERATOR DECISION D-1:
  council's empty-key client construction vs the confirmed empty-key
  posture — preserve or align; blocked the design gate until Michael
  answered (resolved (a) preserve in v0.6.0).
- **v0.4.0 (2026-07-12, CFADA round-1 repairs; findings by the Codex
  auditor)** — (F2) R1 scan moved out of prose/table into the fenced R1 Scan
  Protocol with fixed PASS semantics (empty output + exit 1; exit ≥ 2 ⇒
  deny) and a base-liveness precondition, killing the escaped-regex
  false-pass class. (F3) R7 protected domain extended to ALL governed-doc
  trees (`docs/designs/` with its 18 existing files, `docs/.adversarial-design/`,
  `docs/runbooks/`) — previously a PR could edit an existing design packet
  and pass. (F4) R3 rewritten around a normative per-call-site truth table
  and determinate proof map (behavior-preserving, including currently-odd
  behaviors). (F5) R4 verification names the real existing preflight tests.
  (F6) The plain-language restatement Michael confirmed is archived and
  hash-pinned (`1e6a52b0…`) with the FO-version chain, making the confirmed
  reading immutable; hive-only scope justified by the repo-set sweep row.
  (Obs 7) Inventory corrected to 31 files / 121 matching lines / 124
  occurrences. (Obs 9) Truth-object byte-identity constraint added.
- **v0.3.0 (2026-07-12, IADA repair)** — Exclusion-listed scan replacing the
  unsatisfiable repo-wide scan; R7 rewritten to no-`M`/`D` + allowlisted
  additions; R3 determinism; R5 "no credential key entry"; R6 runbook path +
  live unit resolution; full inventory in source citations.
- **v0.2.0 (2026-07-12)** — Channel A intake confirmed (archived,
  hash-pinned). Scope expanded per operator answer to the full `LOVYOU_*`
  family; history-purge question settled as "leave it"; verification switched
  to tracked-file `git grep`; in-flight interaction note added for hive#283.
  Title generalized.
- **v0.1.0 (2026-07-12)** — Birth: API-key rename, credential
  externalization, operator runbook, historical-records protection; two open
  intake questions.
