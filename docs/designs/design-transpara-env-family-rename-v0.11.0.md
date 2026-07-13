---
doc_id: DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME
title: Design Packet — Retire the LOVYOU_* Environment Variable Family in transpara-ai/hive
doc_type: design
status: proposal
version: 0.11.0
created: 2026-07-12
updated: 2026-07-13
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
answers_factory_order:
  doc_id: FO-HIVE-TRANSPARA-API-KEY-RENAME
  version: 0.11.0
  blob_sha: a055a071ea596b767b633a1ad4b9339847fe84f0
  path: artifacts/hive/fo-transpara-api-key-rename/FO-hive-transpara-api-key-rename-v0.11.0.md
related_docs:
  - FO-HIVE-265-LIFECYCLE-SKILL-HOME (skill-dialect delta convention)
  - transpara-ai/hive#283 (in-flight, touches skill dialects)
authority: none — this packet proposes; it authorizes nothing
---

# Design Packet — Retire the LOVYOU_* Environment Variable Family

## 1. What this answers

FO-HIVE-TRANSPARA-API-KEY-RENAME **v0.11.0** at git blob
`a055a071ea596b767b633a1ad4b9339847fe84f0` (channel A; intake, plain-language
restatement, and confirmation all archived and hash-pinned in the FO):
rename `LOVYOU_API_KEY` → `TRANSPARA_API_KEY`, `LOVYOU_BASE_URL` →
`TRANSPARA_BASE_URL`, `LOVYOU_SPACE` → `TRANSPARA_SPACE` across all
functional surfaces of `transpara-ai/hive`; hard cutover, no fallback;
committed credential removed; operator runbook shipped; governed and
historical records untouched; no history rewrite.

## 2. Base assumption (stated per FO constraint)

Designed against `transpara-ai/hive` main `bf3f126`. Tracked inventory at
this base: **31 files, 121 matching lines, 124 occurrences** of the three
names — 26 functional files (renamed) and 5 R7-protected historical files
(retained: `docs/factory-orders/FO-hive-265-lifecycle-skill-home-v0.58.0.md`,
`docs/superpowers/plans/2026-04-14-offline-pipeline.md`,
`docs/superpowers/plans/2026-05-14-route-claude-p-through-modelconfig-plan.md`,
`docs/superpowers/specs/2026-04-18-hive-cli-redesign-design.md`,
`loop/reflections.md`). Functional references to the family exist only in
this repo (the FO's repo-set sweep row): `site` consumes hive as a git
submodule pin and every other appearance in the workspace is a copy or a
historical record. PR **#283** (`feat/hive-preflight-runbook-wiring`, head
`57f6c36`, OPEN at design time) touches both skill dialects and replaces the
runbooks' inline credential grep probe with an invocation of the preflight
verifier. **Merge-order assumption: the rename branch is cut from whatever
main holds at implementation time and AC-1 re-runs the inventory and the
base-liveness check at that base.** If #283 lands first, the skill dialects
contain fewer legacy-name references — the requirements and gate predicate
are unchanged. No design element depends on #283's presence or absence.

## 3. Design decisions

- **D1 — Mechanical rename map, including operator-facing strings.** One
  commit-wide substitution per name, applied only to functional surfaces:
  Go sources/tests, both skill dialects, `agents/CONTEXT.md`,
  `loop/mcp-graph.json`, `loop/hive-runtime-spec.md`. Error messages, flag
  help (`--webhook-require-auth`'s "Bearer LOVYOU_API_KEY"), and log lines
  are IN scope: they name the variable the operator must set, so they must
  match the `os.Getenv` calls or the diagnostics lie. Historical docs and
  untracked scratch worktrees are OUT (FO R7).
- **D2 — Credential externalization by environment inheritance.**
  `cmd/mind` spawns the MCP server with `cmd.Env = os.Environ()` then appends
  `cfg.Env` entries (`cmd/mind/mcp.go:47-54`); Go's `exec` gives the LAST
  duplicate entry precedence, so a config entry always overrides the
  inherited environment. Therefore `loop/mcp-graph.json` **deletes the
  API-key entry entirely** — the spawned `mcp-graph` inherits
  `TRANSPARA_API_KEY` from the operator's shell (FO R5 "environment
  indirection"). An empty-string entry is explicitly REJECTED: it would
  silently override a real inherited key with empty (fail-open footgun).
  The non-secret `"TRANSPARA_SPACE": "hive"` entry stays. Add
  `loop/*.local.json` to `.gitignore` so operator-local variants that do
  carry values cannot be committed **accidentally** (forced adds remain
  possible; the class fix is against the accidental path, and the AC-2 scan
  still catches any value that lands). Observed but out of scope: the file's
  `command` path is a stale personal path from the upstream author's
  machine; not a `LOVYOU_*` name, not a secret — left as-is, noted for a
  future chore.
- **D3 — Preflight probe: constant change only, properties re-proven.** The
  hive-unit credential probe's match prefix becomes `TRANSPARA_API_KEY=`;
  the four reviewed properties are preserved and re-proven by the existing
  test matrix updated to the new name
  (`TestEvaluateHiveUnitPreflightCoversCredentialPostures`,
  `TestParseHiveUnitPropertiesFailsClosedOnEmptyUnreadableAndMalformedInputs`,
  `TestRunHiveUnitPreflightRedactsPresentCredentialValue`,
  `TestRunHiveUnitPreflightReportsUnknownAndNonzero`): exact-name prefix
  with trailing `=`, exact-length compare distinguishing EMPTY from PRESENT,
  posture-only output (value never emitted), fail-closed UNKNOWN on
  unreadable environ. A sibling-name non-match case
  (`TRANSPARA_API_KEY_BACKUP=` must not read as the credential) is cited if
  already asserted at the implementation base, otherwise added as
  `TestPreflightPrefixCollisionSafety` — checked at the base, decided there,
  recorded in evidence (no review-time either/or).
- **D4 — Normative per-call-site truth table; behavior frozen.** The rename
  preserves EXACTLY the current behavior at every reading call site,
  including currently-odd behavior (flagged as observations for future
  orders, never altered here). Verified at base `bf3f126`:

  | Call site | Key set (non-empty) | Key empty / absent |
  |---|---|---|
  | `cmd/hive` ingest (`runIngest`, `cmd/hive/main.go:134-136`) | proceeds | error naming the variable as required |
  | pipeline run/daemon (`runPipeline`, `cmd/hive/main.go:597,601-608`) | used | `apiBase` contains `localhost`/`127.0.0.1` (substring) ⇒ `dev` fallback + log; else ⇒ required error |
  | role run/daemon (`runRunner`, `cmd/hive/main.go:439,443-451`) | used | same substring-`localhost` ⇒ `dev` fallback; else required error |
  | council (`cmd/hive/main.go:506-512`) | used | client constructed with the verbatim env value — INCLUDING empty (comment "nil-safe if no key"); no `dev` fallback and no empty-key gate — posts to `--api` (default `localhost:8082`, `cmd/hive/router.go:286`) regardless of key; current behavior preserved verbatim (FO D-1: preserve; un-gated empty-key class residual §7) |
  | civilization run/daemon (`cmd/hive/main.go:1284-1288`) | Site client created | Site client nil ⇒ disabled (local-only) |
  | webhook auth (`cmd/hive/factory.go:239,246,453-458`) | non-BLANK value ⇒ bearer token | with `--webhook-require-auth`: BLANK (empty or whitespace-only; `strings.TrimSpace`) ⇒ error — the ONLY site with a blank-rejecting partition; the other *gating* sites compare the raw value to empty (no TrimSpace), and several sites (council, critic, the seven runners) do not compare the key at all — they interpolate it verbatim |
  | preflight probe (`cmd/hive/factory_preflight_hive_unit.go`) | PRESENT | EMPTY / ABSENT / UNKNOWN postures, fail-closed |
  | runner: observer (`pkg/runner/observer.go:46,248,282`) | graph-audit sections included | empty ⇒ graph audit skipped, no-key output format (three gates) |
  | runner: critic (`pkg/runner/critic.go:115,210-239`) | API `curl` with `Bearer <key>` interpolated into the instruction | empty ⇒ API `curl` section STILL present with an empty bearer (no empty-key branch — same shape as council); `TestBuildCriticInstructionWithoutAPIKeyKeepsReviewContract` pins that the review-contract text survives the empty-key path, NOT that the API section is absent |
  | runner: architect, council, pm, reflector, scout, scribe, spawner (7 files) | value interpolated verbatim into Operate instructions | empty string interpolated verbatim; NO key-conditional branch in these seven |
  | `cmd/post` (`cmd/post/main.go:34-44`) | posts | "not set, skipping post" soft-skip (no error); `TRANSPARA_BASE_URL` defaults to `https://transpara.ai` |
  | `cmd/reply` (`cmd/reply/main.go:67-75`) | proceeds | "not set" error; `TRANSPARA_BASE_URL` defaults to `https://transpara.ai` |
  | `cmd/republish-lessons` (`cmd/republish-lessons/main.go:28-32`) | proceeds | "not set" to stderr + exit 1 |
  | `cmd/mcp-graph` (`newServer()` `main.go:112-126`; `apiGet`/`apiPost` `main.go:317-367`) | used; sets Authorization header | empty ⇒ server still constructed AND `apiGet`/`apiPost` STILL call `client.Do` (empty key only omits the Authorization header), so tool GETs/POST bodies reach the default `https://transpara.ai` (proven empty-key by `TestToolRespond_postsToAPI`); `TRANSPARA_BASE_URL`→`https://transpara.ai`, `TRANSPARA_SPACE`→`hive` |
  | `cmd/mind` (`cmd/mind/main.go:21`) | doc-comment example only | rename the text |

- **D5 — Skill dialects change by textual rename only**, under FO-265's
  "seeded + enumerated deltas" convention: this packet's delta set is
  "legacy variable names → new names" in both dialects; posture language is
  renamed, never deleted (FO R3). The claude dialect is edited through its
  physical file `.claude/skills/hive-lifecycle/SKILL.md` (the
  `skills/hive-lifecycle/claude` symlink target).
- **D6 — Operator migration runbook (shipped as
  `docs/runbooks/transpara-env-rename-migration.md`).** Ordered, each step
  with a named check: (1) edit `hive.env` on nucbuntu — rename the key(s);
  (2) `systemctl --user daemon-reload` and restart the affected stack
  services; (3) run `hive factory preflight-hive-unit` — expect credential
  posture PRESENT under `TRANSPARA_API_KEY`; (4) confirm no `LOVYOU_` name
  remains in the effective unit environment (verifier report, names only);
  (5) re-sync local skill installs (`rsync -a --delete` repo →
  `~/.claude/skills/`, `~/.codex/skills/`); (6) update any operator-local
  MCP config copies. **Affected unit names and `hive.env` consumers are
  resolved live at runbook-authoring time** (`systemctl --user list-units`,
  grep of `~/.config/systemd/user/` for the env file) and the runbook
  carries a "resolved live on <date>" note. Executed by the operator only,
  after merge (FO R6). Until executed, the key-*gating* paths sit in the D4
  table's empty/absent column — fail-closed or default-safe; the
  un-gated empty-key class (§7: only `cmd/mcp-graph` reaches the default remote
  on an empty key; council/critic default to a local base) is not fail-closed
  on an empty key and is unchanged by the rename.
- **D7 — Governed-doc placement in the PR.** The PR carries byte-identical
  copies (`git hash-object` equality is the check) of: the FO into
  `docs/factory-orders/`, this packet into `docs/designs/`, IADA/CFADA
  evidence into `docs/.adversarial-design/transpara-env-family-rename-iada/`
  and `…-cfada/`, and the runbook into `docs/runbooks/`. The
  `.adversarial-design` location is the hive-repo adaptation of the docs-repo
  precedent (which keeps it at the repo root); the adapted path is named
  here so it is not mistaken for the precedent itself. These docs quote the
  legacy names by necessity, which is why the R1 Scan Protocol excludes
  exactly these destinations plus the R7 historical set.
- **D8 — Scan protocol discipline.** The false-pass class found in v0.2.0
  was a pipe-containing regex quoted inside a markdown table cell: the
  required `\|` escaping produced a command that matches nothing on the
  unrenamed tree. Rule: no command containing an alternation pipe appears in
  any table cell of the FO or this packet; the single normative rename scan
  is the FO's fenced **R1 Scan Protocol**, and every AC references the
  protocol rather than restating the regex. (AC-2's `lv_[0-9a-f]{16,}`
  pattern contains no character altered by table escaping and is therefore
  safe to quote in place.) Protocol semantics are fixed: PASS = empty output
  + exit 1; exit 0 = FAIL; exit ≥ 2 = DENY; plus the base-liveness
  precondition (the same command at the branch base must exit 0 with ≥ 1
  match — at `bf3f126` it prints 94 matching lines).

## 4. Requirements → Acceptance Criteria

| ID | Answers | Criterion (satisfied only when…) | verification_method | risk_class |
|---|---|---|---|---|
| AC-1 | R1, R2 | The R1 Scan Protocol (FO fenced block; D8 semantics) passes at the PR head AND its base-liveness precondition is recorded at the actual branch base; exit ≥ 2 anywhere ⇒ deny | protocol run (base + head) with output + exit codes recorded in PR evidence | high |
| AC-2 | R5 | `git grep -E 'lv_[0-9a-f]{16,}'` at the PR head prints nothing and exits 1 (baseline exactly one hit — the deleted line; exit ≥ 2 ⇒ deny), AND `loop/mcp-graph.json` contains no API-key entry at all (inheritance, not empty string), AND `.gitignore` covers `loop/*.local.json` | command run + file inspection in PR evidence | high |
| AC-3 | R2, R3 | `go build ./...` and `go test ./...` pass at the PR head, including every named test in §5 | CI/local run output in PR evidence | medium |
| AC-4 | R4 | The preflight matrix (the four named existing tests, updated to the new name) proves all four probe properties, plus the sibling-name non-match case per D3's base-decided rule | named tests green in AC-3's run + D3 decision recorded | high |
| AC-5 | R3 | Both skill dialects state the same set/empty/absent posture language under the new names; `grep -RE 'LOVYOU_' skills/` (symlink-traversing) prints nothing | command run + text diff review | medium |
| AC-6 | R6 | The runbook file exists at `docs/runbooks/transpara-env-rename-migration.md`, has the six D6 steps in order, every step names its check command, and its unit names carry a "resolved live on \<date\>" note | file review in CFAR | medium |
| AC-7 | R7 | `git diff --name-status --no-renames` against the base (rename detection OFF — an `R082` in hive history masks a protected-FO mutation) shows, under `docs/factory-orders/`, `docs/designs/`, `docs/superpowers/`, `docs/.adversarial-design/`, `docs/runbooks/`, and `loop/reflections.md`, ONLY status `A`, and only on paths in D7's enumerated allowlist; any other status letter under those trees — `M`, `D`, `R*`, `C*`, `T`, or anything else — fails (status allowlist) | command + output recorded in PR evidence | high |
| AC-8 | R7 + Constraints (truth-object carry) | The PR carries the FO, this packet, and the IADA/CFADA evidence at the exact versions and blob SHAs cited in PR evidence, placed per D7; artifact-to-repo byte identity proven by `git hash-object` equality | blob-SHA read-back in PR evidence | low |
| AC-9 | R3 | Every row of D4's truth table carries its assigned proof per the §5 proof map — a green named test where a test file + seam exist, otherwise a code-inspection citation at file:line recorded in PR evidence; no row unproved, no method swapped at review time | proof-map execution recorded row-by-row in PR evidence | high |
## 5. Proof map and named TestCases

**Named tests (existing at base — updated in body to the new name):**

- `TestRunIngest_MissingAPIKey` and `TestRunIngest` (`cmd/hive/ingest_test.go`) — ingest required-error / with-key paths.
- `TestResolveWebhookBearerTokenWholeDomain` (`cmd/hive/router_test.go:134`) — webhook auth over its whole input domain.
- `TestEvaluateHiveUnitPreflightCoversCredentialPostures`,
  `TestParseHiveUnitPropertiesFailsClosedOnEmptyUnreadableAndMalformedInputs`,
  `TestRunHiveUnitPreflightRedactsPresentCredentialValue`,
  `TestRunHiveUnitPreflightReportsUnknownAndNonzero`
  (`cmd/hive/factory_preflight_hive_unit_test.go`) — the probe matrix.
- `TestSpaceFor_fallsBackToDefault` (`cmd/mcp-graph/main_test.go:34`) — space default.
- `TestToolRespond_postsToAPI` (`cmd/mcp-graph/main_test.go:97`) — existing: an
  empty-key `mcp-graph` tool call still POSTs its body to the API endpoint —
  mcp-graph is the one un-gated path defaulting to the remote, preserved
  verbatim by the rename.
- `TestBuildPart2Instruction`, `TestBuildOutputInstructionNoAntiPatternWhenNoKey`,
  `TestBuildPart2InstructionMetaTaskItemSkippedWhenNoKey`,
  `TestBuildOutputInstructionNoCausesWhenNoKey`, `TestBuildObserverInstruction`
  (`pkg/runner/observer_test.go`) — observer's empty-key gating.
- `TestBuildCriticInstructionWithAPIKeyAndCauses`,
  `TestBuildCriticInstructionWithoutAPIKeyKeepsReviewContract`
  (`pkg/runner/critic_test.go`) — existing coverage that `buildCriticInstruction`'s
  text is stable for a set vs empty key argument; they assert the review-contract
  text, NOT that the API `curl` is gated (it is not — critic is a
  verbatim-interpolation inspection row below). These tests pass the key as a
  literal argument, so the env-var rename does not touch them.
- Mechanical `t.Setenv` updates in `cmd/hive/router_test.go` and other tests
  that set the variable incidentally.

**Named test extension (asserting current behavior only):**

- `TestResolveWebhookBearerTokenWholeDomain` gains a whitespace-only input
  case (currently absent), so the test's whole-domain name matches the
  site's actual blank-rejecting partition (`strings.TrimSpace`).

**Named tests (new):**

- `TestNewServerDefaults` (`cmd/mcp-graph`) — via the existing `newServer()`
  seam with `t.Setenv`: unset `TRANSPARA_BASE_URL` ⇒ `https://transpara.ai`;
  unset `TRANSPARA_SPACE` ⇒ `hive`.
- `TestPreflightPrefixCollisionSafety` — only if the sibling-name case is
  absent at the implementation base (D3's base-decided rule).
- `TestRunbookDialectsNameNoLegacyVariable` — extends hive#283's runbook
  consistency test if present at base, otherwise standalone: neither dialect
  contains a `LOVYOU_` token.

**Inspection rows (per FO R3's three-way rule — either no key-conditional
branch, or no test file/seam at base; code inspection cited at file:line in
PR evidence):** pipeline (`runPipeline`, `cmd/hive/main.go:597,601-608`) and
role (`runRunner`, `cmd/hive/main.go:439,443-451`) localhost-`dev`/required
paths (no seam), council verbatim-env client construction
(`cmd/hive/main.go:506-512` — FO D-1 resolved: preserve verbatim, no behavior
change; empty ⇒ no empty-key gate, posts to `--api` which defaults local, so
remote only under an explicit remote `--api` — un-gated empty-key class
residual §7), critic's unconditional API-`curl` interpolation
(`pkg/runner/critic.go:115,210-239` — verbatim interpolation, no key-conditional
branch; empty ⇒ empty `Bearer` in the curl; reachability per the §7 residual;
same un-gated class as the seven runners), civilization
Site-client gate (`cmd/hive/main.go:1284-1288` — no seam), `cmd/post`
soft-skip + base-URL default (`cmd/post/main.go:34-44` — env read inline in
`main`), `cmd/reply` required error + base-URL default
(`cmd/reply/main.go:67-75` — no test file), `cmd/republish-lessons` required
error (`cmd/republish-lessons/main.go:28-32` — `main_test.go` exists but tests
the arg-taking helpers; the env-read + `os.Exit` are inline in `main` with no
seam), the seven
non-branching runner interpolation sites (architect, council, pm, reflector,
scout, scribe, spawner — verbatim interpolation, no key-conditional branch
to test), `cmd/mind` doc example (`cmd/mind/main.go:21` — comment text).

## 6. Gate predicate (allowlist — satisfied only when)

The design is satisfied **only when AC-1 through AC-9 are ALL individually
evidenced at the exact PR head under review**. Any AC unevidenced, any scan
non-empty or erroring (exit ≥ 2), any test red, any `M`/`D` under a protected
path, any addition outside D7's allowlist, any truth-table row unproved, or
any ambiguity in the evidence ⇒ **not satisfied** (default deny). There is no
partial credit and no compensating evidence path.

## 7. Residual risks and named non-goals (inherited from FO v0.11.0)

- Exposed `lv_b7fb22…` remains in public history permanently (operator
  answer: no rewrite); operator rotation is the sole neutralizer and is not
  performed, authorized, or assumed done by this work (rotation is a separate
  operator action tracked outside this work). Owner: Michael.
- Deployment window between merge and D6 execution: the key-*gated* paths sit
  in D4's empty/absent column — required-error, soft-skip-with-notice, Site
  client disabled, or built-in default. The un-gated empty-key readers below are
  preserved, not introduced: on an empty key only `cmd/mcp-graph` defaults to
  the remote (mitigated by a local `TRANSPARA_BASE_URL`); the others'
  reachability depends on operator `--api`/base config (see the residual);
  unchanged by the rename, deferred by D-1.
- **Un-gated empty-key readers (FO D-1 resolved 2026-07-12: (a) preserve;
  deferred as a class).** Several readers — council (`cmd/hive/main.go:506-512`),
  `cmd/mcp-graph` (`apiGet`/`apiPost`), critic, and the runner
  instruction-builders — read the API key without a uniform empty-key gate and
  interpolate or send it regardless (unlike ingest, civilization, and webhook,
  which fail safe on an empty key). On an empty key, `cmd/mcp-graph` defaults to
  the remote (`TRANSPARA_BASE_URL`→`https://transpara.ai`) while the others'
  remote-reachability depends on the operator's `--api`/base configuration. This
  is a pre-existing confidentiality concern, independent of and unchanged by
  this rename (D-1: preserve); its exhaustive per-site characterization and any
  remediation — and the correction of the stale operator-doc claims found during
  design (a skill warning's council `--api` default, mcp-graph's key doc-comment,
  the webhook bind description) — are the scope of the deferred future governed
  order, not this rename. Interim operator mitigation: pin `--api` to a local
  host and set a local `TRANSPARA_BASE_URL` for mcp-graph. Owner: Michael. This
  packet proposes no behavior change here.
- Stale personal `command` path in `loop/mcp-graph.json` — observed, out of
  scope, candidate future chore.

## 8. Non-authorizations

This packet proposes a design. It authorizes no code, no PR, no merge, no
key rotation, no history rewrite, no runtime execution, no deploy, no issue
mutation, no EventGraph write, no autonomy change. Code begins only after
IADA → CFADA → Human Design Review approve this packet (TLC stages 4–6).

## Revision History

- **v0.11.0 (2026-07-13, CFADA round-8 repair; single finding)** — §5's
  `cmd/republish-lessons` inspection rationale said "no test file", but
  `cmd/republish-lessons/main_test.go` exists (it tests the arg-taking helpers).
  Inspection remains the correct proof-map assignment — the empty-key exit
  (`main.go:28-32`) is inline in `main()` with no seam — so the rationale is
  corrected to cite the absent seam, matching `cmd/post`'s phrasing. Class-swept
  the other inspection rows: `cmd/reply` genuinely has no test file (claim
  correct); `cmd/post` already cites "inline in `main`" (correct). FO v0.11.0
  binding unchanged (blob `a055a071ea596b767b633a1ad4b9339847fe84f0`). No
  requirement, AC, scan, proof-map rule, or gate predicate changed.
- **v0.10.0 (2026-07-13, CFADA round-7 repairs; operator chose "narrow")** —
  Rebound to FO **v0.11.0** at blob
  `a055a071ea596b767b633a1ad4b9339847fe84f0`. Per the operator's scope
  narrowing after seven rounds: removed **D9** and **AC-10** (the doc-accuracy
  decision/criterion) and reverted the gate predicate to AC-1…AC-9 — a rename
  is not a doc audit. Shrank the §7 un-gated-readers residual to a minimal
  accurate statement (readers read the key without a uniform empty-key gate;
  only mcp-graph defaults remote; the others' reachability depends on operator
  `--api`/base config), deferring the exhaustive characterization AND the
  stale-operator-doc corrections (council default, mcp-graph key doc, webhook
  bind) to the follow-up order; the §5 critic clause and §7 deployment residual
  are simplified to match. Resolves round-7 blockers 1-3 (no council-runner
  gating classification, no D5/D9 contradiction, no unbounded doc-accuracy). No
  requirement, scan protocol, or proof-map rule changed.
- **v0.9.0 (2026-07-12, CFADA round-6 repairs; operator chose "right-size")**
  — Rebound to FO **v0.10.0** at blob
  `7fd4e7bad3b317a43258bc8162140fe98edc4717`. (R6-1) the un-gated class now
  enumerates all readers — council, mcp-graph, critic, and the seven runners.
  (R6-2) D4 pipeline/role rows and the §7 residual state the exact
  `strings.Contains(apiBase,"localhost"|"127.0.0.1")` substring gate; the "critic
  never reaches a remote" overstatement is removed; the residual is right-sized
  (accurate + complete, but the exhaustive analysis is handed to the deferred
  order). (R6-3) new **D9 + AC-10** (answering FO R8): the edited operator docs'
  false claims are corrected — council's `--api` default (localhost:8082) and
  mcp-graph's optional key — claim-accuracy only, no runtime change; gate
  predicate now spans AC-1…AC-10. No scan protocol or proof-map rule changed.
- **v0.8.0 (2026-07-12, CFADA round-5 repairs; findings by the Codex
  auditor)** — Rebound to FO **v0.9.0** at blob
  `b47b0bcee98bf348a581410d80c38d3c85fb6722`. (R5-2, substantive) empty-key
  reachability was OVERSTATED: `cmd/hive/router.go:286` defaults council's
  `--api` to `localhost:8082` and the pipeline/role runners reject an empty key
  against a non-local base, so **only `cmd/mcp-graph`** (defaults
  `TRANSPARA_BASE_URL`→remote, no `--api` flag) contacts the default remote on
  an empty key; council defaults local and critic is pipeline-gated to local.
  Class renamed "un-gated empty-key paths"; the D4 council row, D6, §5
  council/critic inspection, and §7 deployment + class residual are restated
  with accurate per-path reachability (the R4-3 mitigation split retained —
  Codex confirmed it accurate). (R5-3) the §7 heading rebound to the current FO
  version. (R5-1 was a FO Constraints fix, in FO v0.9.0.) No AC, proof-map
  rule, scan, or gate predicate changed.
- **v0.7.0 (2026-07-12, CFADA round-4 repairs; findings by the Codex
  auditor)** — Rebound to FO **v0.8.0** at blob
  `d39e38a5734f262a41aca9c26d03aac1df046f4d`. Fixed the round-3 repairs'
  sibling-section misses (the "fix the class, not the instance" failure):
  packet D6's "never open" claim (R4-1), and split the class mitigation by
  path — `--api` pin for council/critic, local `TRANSPARA_BASE_URL` for
  `cmd/mcp-graph` which takes no `--api` flag (R4-3) — in the §7 deployment and
  class residuals. The FO R3 universal (R4-2) and FO Non-Goals note (obs) were
  governing-doc fixes made in FO v0.8.0. A four-class sweep of both documents
  confirms zero remaining live false claims. No AC, proof-map rule, scan, or
  gate predicate changed.
- **v0.6.0 (2026-07-12, CFADA round-3 repairs; findings by the Codex
  auditor)** — Rebound to FO **v0.7.0** at blob
  `1885e16feed37d84d80a1b9b3f993cdf362b1f96`. Four blockers, all claim-accuracy
  (no behavior change): (R3-F1) D4's critic row falsely said "empty ⇒ without
  API section" — corrected: the API `curl` is unconditional (empty bearer on
  empty key), critic reclassified as a verbatim-interpolation inspection row,
  and the named critic tests re-described as instruction-text coverage that
  does NOT assert gating. (R3-F2) the empty-key conflict was falsely scoped to
  council — `cmd/mcp-graph` (`apiGet`/`apiPost` always `client.Do`) and critic's
  curl reach the default remote on an empty key too; D4's mcp-graph row, §7's
  residual, and the deployment-window entry re-scoped to the class; the D4
  mcp-graph row and §5 cite `TestToolRespond_postsToAPI` for the empty-key POST.
  (R3-F3) D4's webhook row "all other sites compare raw to empty" was a false
  universal — corrected (several sites don't compare at all). (R3-F4) §7's
  mitigation was wrong — the skill sets the key to `dev` (not blank) and it is
  the local `--api` pin, not the key, that keeps content local; corrected in §7.
  (Obs 9) pipeline/role citations were swapped and ingest was line 123 — fixed
  to `runPipeline`@597/601-608, `runRunner`@439/443-451, ingest 134-136. No AC,
  proof-map rule, scan, or gate predicate changed.
- **v0.5.0 (2026-07-12, operator decision D-1 resolved)** — Rebound to FO
  **v0.6.0** at blob `e05e18b43e8a33cf3e148046e8c7116cf50d72e2`. Michael chose
  **(a) preserve** for the council empty-key conflict: §7's OPEN/FAIL-until-
  decided D-1 bullet becomes a resolved Named Residual Risk (behavior carried
  verbatim, alignment deferred to a dedicated future governed order); the D4
  council row
  and the §5 inspection note record the resolution. No AC, proof map, scan, or
  gate predicate changed — this is a decision-resolution and rebind bump only.
- **v0.4.0 (2026-07-12, CFADA round-2 repairs; findings by the Codex
  auditor)** — (R2-F3) AC-7 hardened to a status allowlist under
  `--no-renames` (only `A` on D7-allowlisted paths; `R*`/`C*`/`T` and
  unknown statuses fail). (R2-F4) D4's webhook row records the site's real
  blank-rejecting partition (TrimSpace) and §5 extends
  `TestResolveWebhookBearerTokenWholeDomain` with the missing
  whitespace-only case. (R2-F5) The false "runners have no gate/seam" row is
  split per verified reality: observer gates empty keys (three sites, five
  named existing tests), critic's set/empty instruction contract is pinned
  by two named existing tests, and the remaining seven runners interpolate
  verbatim with no key-conditional branch (inspection per the FO's refined
  three-way rule). (R2-F6) FO D-1 recorded as an OPEN operator decision in
  §7 — the packet could not pass CFADA until Michael picked preserve or align
  for council's empty-key behavior (resolved (a) preserve in v0.5.0). Rebound
  to FO v0.5.0 at blob `740a94ce…`.
- **v0.3.0 (2026-07-12, CFADA round-1 repairs; findings by the Codex
  auditor)** — (F2) AC-1 no longer embeds a regex in a table cell (the
  escaped pipes made a command that false-passes on the unrenamed tree);
  new D8 fixes the single fenced protocol, exit-code semantics, and the
  base-liveness self-test. (F3) AC-7's protected domain extended to all
  governed-doc trees including `docs/designs/` (18 existing tracked files) —
  previously an existing design packet could be modified or deleted while
  both AC-1 (which excludes the tree) and AC-7 (which did not protect it)
  passed. (F4) D4 truth table corrected to verified per-call-site reality —
  council constructs a client with the verbatim (possibly empty) env value,
  civilization alone gates the Site client, `cmd/post` soft-skips — and §5
  rebuilt so every claimed test exists at base by its real name
  (`TestResolveWebhookBearerTokenWholeDomain`, the four preflight tests,
  `TestRunIngest_MissingAPIKey`, `TestSpaceFor_fallsBackToDefault`); new
  AC-9 executes the row-by-row proof map. (F5) The false "no test seam"
  claim for `cmd/mcp-graph` corrected — `newServer()` is a seam with an
  existing test file; defaults get `TestNewServerDefaults`; inspection is
  reserved for the sites verified seamless. (F6 is repaired in the FO:
  restatement archived and hash-pinned; scope justified by the repo-set
  sweep.) (Obs 7) counts corrected to 31 files / 121 matching lines / 124
  occurrences. (Obs 9) `.gitignore` claim softened to accidental-commit
  prevention; D7 names its path as a hive adaptation of the docs-repo root
  precedent; AC-8 re-traced to R7 + the truth-object constraint instead of
  R1.
- **v0.2.0 (2026-07-12, IADA repair)** — Exclusion-listed scan; AC-7
  no-`M`/`D` + allowlisted additions; AC-9 determinism; D6 live unit
  resolution; D7 placement; rebind to FO v0.3.0.
- **v0.1.0 (2026-07-12)** — Birth: six decisions, AC-1–AC-8, seven named
  TestCases, allowlist gate predicate.
