You are the CFADA auditor (cross-family adversarial design audit, TLC stage 5), ROUND 5, for a Claude-authored design packet you audited in rounds 1-4. Audit READ-ONLY: no file changes, no state-changing commands. Read-only commands OK (git show/grep/hash-object/diff, rg, sha256sum, go doc).

ROUND 4 RECAP — you returned FAIL (3 design blockers) on packet v0.6.0 / FO v0.7.0. Each was the SAME false claim the round-3 repair fixed in one section but left standing in a sibling:
- R4-1: packet D6 still said every affected path is "fail-closed or default-safe, never open" (§7/Deployment Window were fixed, D6 was not).
- R4-2: the governing FO R3 still said the webhook uses TrimSpace "while every other site compares the raw value to empty" (packet D4 was fixed, FO R3 was not). False: council/critic/the seven runners don't compare at all.
- R4-3: the class residual claimed the local `--api` pin mitigates the WHOLE class, but `cmd/mcp-graph` takes no `--api` flag — it reads `LOVYOU_BASE_URL`/`TRANSPARA_BASE_URL` (default `https://transpara.ai`, main.go:112-126) and neither the skill commands nor `loop/mcp-graph.json` supply a local base. The controls must be split.
- Obs 11 (nonblocking): FO Non-Goals still carried the unverifiable "remediation session opened separately 2026-07-12" note.
Your round-4 observations CONFIRMED sound: bindings (obs 5), critic reclassification + `TestToolRespond_postsToAPI` + skill `dev`/`--api` facts + the fixed ingest/role/pipeline citations (obs 5), and the gate structure — AC-1 scan, inventory, AC-7 allowlist, AC-1…AC-9, default-deny predicate (obs 6).

The author repaired all three blockers plus obs 11 into the NEW bytes, and — because round 4 was a class-not-instance failure — performed a four-class sweep of BOTH documents to remove every live instance. All repairs are claim-accuracy; no behavior changed.

NEW OBJECTS UNDER AUDIT (bind FIRST; any blob mismatch ⇒ STOP):
- Design packet v0.7.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/design-transpara-env-family-rename-v0.7.0.md — expected blob `0d2838f2c69b6e75a4cb56b17d784db09afd5a48`.
- Factory Order v0.8.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/FO-hive-transpara-api-key-rename-v0.8.0.md — expected blob `d39e38a5734f262a41aca9c26d03aac1df046f4d`. The packet's `answers_factory_order.blob_sha` must equal this.
- Immutable sources (unchanged): intake-2026-07-12.md sha256 `76b30d68d9dbfbd29ba93607ca9dd3fd4fbdf482092463294c1fe3bcf529c30d`; intake-restatement-2026-07-12.md sha256 `1e6a52b03125f6ee4ced06433af8276df10b0ab10e0d92b227a45ba32ec85f35`; intake-confirmation-2026-07-12.md sha256 `b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310`.
- Author-side IADA round 6 (context only): iada/iada.result-r6.md.
- Subject repo: /Transpara/transpara-ai/repos/hive, base main `bf3f126` — read base bytes with `git -C /Transpara/transpara-ai/repos/hive show bf3f126:<path>`.

ROUND 5 TASKS:
1. Re-bind (blob equality + packet↔FO binding).
2. Verify EACH round-4 blocker is repaired EVERYWHERE it occurred — do the class sweep yourself, do not trust the reported section alone:
   - R4-1: grep both docs for "never open"/"never fail-open"/"fail-closed or default-safe, never" — every live (non-revision-history) occurrence must be gone; confirm packet D6 now carves out the empty-key-contacts-remote class.
   - R4-2: grep both docs for "every other site"/"all other sites compare" — confirm the only live normative statements (FO R3 and packet D4) now say the gating sites compare raw-empty AND several sites don't compare at all; historical revision entries may remain.
   - R4-3: confirm the mitigation is split path-by-path in EVERY affected section of BOTH docs (FO Named Residual class, FO Deployment Window, packet §7 deployment + class residual): `--api` pin for council/critic, a local `TRANSPARA_BASE_URL` for `cmd/mcp-graph`. Verify against the code that mcp-graph has no `--api` flag (reads LOVYOU_BASE_URL at cmd/mcp-graph/main.go:114; no `flag.*` parsing) and that critic's curl targets the pipeline's `--api` base (`buildCriticInstruction` gets `r.cfg.APIBase`). Confirm no bare whole-class `--api` mitigation remains live.
   - Obs 11: confirm no live "remediation session opened separately" remains (revision-history mention of its removal is fine).
3. Re-confirm no regression in the round-4-sound items (bindings; AC-1 scan; AC-7 allowlist; the D4 table; AC-1…AC-9 and the gate predicate; critic inspection classification).
4. Fresh adversarial sweep of the CHANGED text for NEW defects: any new contradiction among D4 / D6 / §7 / Deployment Window / FO R3; any new false universal; any citation or code claim your independent read contradicts; any AC-9 row left unproved. Be adversarial.
5. Output: numbered findings (DESIGN-BLOCKER / OBSERVATION with section + evidence), ending with exactly one line:
   CFADA VERDICT: PASS (0 design blockers)
   or
   CFADA VERDICT: FAIL (<n> design blockers)
