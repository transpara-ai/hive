You are the CFADA auditor (cross-family adversarial design audit, TLC stage 5), ROUND 4, for a Claude-authored design packet you audited in rounds 1-3. Audit READ-ONLY: do not modify, create, or delete any file; no state-changing commands. You may read files and run read-only commands (git show/grep/hash-object/diff, rg, sha256sum, go doc).

ROUND 3 RECAP — you returned FAIL (4 design blockers) on packet v0.5.0 / FO v0.6.0:
- R3-F1: D4's critic row claimed "empty ⇒ review contract kept without API section (pinned by existing tests)" — false; `buildCriticInstruction` (critic.go:210-239) always includes the API curl with `Bearer %s`, and `TestBuildCriticInstructionWithoutAPIKeyKeepsReviewContract` only checks the review contract, never the curl's absence.
- R3-F2: D-1 falsely scoped the empty-key-contacts-remote conflict to council alone; `cmd/mcp-graph` (`apiGet`/`apiPost` main.go:317-367 always call `client.Do`; empty key only omits the header) and critic's curl also reach the default `https://transpara.ai` on an empty key. `TestToolRespond_postsToAPI` proves the empty-key POST. Therefore the deployment-window "never fail-open" universal was false.
- R3-F3: D4's webhook row "all other sites compare the raw value to empty" is a false universal (council/critic/the seven runners don't compare at all).
- R3-F4: the D-1 residual said the hive-lifecycle skill "blanks the key for council"; the skill actually runs `LOVYOU_API_KEY=dev … council --api http://localhost:8082` (key set to `dev`, not blank) and its own warning says only the local `--api` pin keeps the report local.
- Non-blocking obs 9 (pipeline/role citations swapped; ingest at 134-136 not 123) and obs 11 (an unverifiable "remediation session" note).
Your round-3 observations also CONFIRMED repaired-and-sound: bindings (obs 5), F2/AC-1 (obs 6), R2-F3/AC-7 (obs 7), the webhook whitespace extension + observer/seven-runner proofs (obs 8), the rest of the D4 table (obs 9), and that D-1 is structurally resolved without new authority (obs 10).

The author (Claude) has repaired all four blockers plus obs 9/11 into the NEW bytes below. All repairs are claim-accuracy corrections — the author asserts NO behavior changed (the rename still preserves everything verbatim; the empty-key-contacts-remote behavior is now framed as a preserved CLASS).

NEW OBJECTS UNDER AUDIT (bind FIRST; any blob mismatch ⇒ STOP and report):
- Design packet v0.6.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/design-transpara-env-family-rename-v0.6.0.md — expected `git hash-object` blob `027fea3dabe95b134363a463983d14e7048b81b7`.
- Factory Order v0.7.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/FO-hive-transpara-api-key-rename-v0.7.0.md — expected blob `1885e16feed37d84d80a1b9b3f993cdf362b1f96`. The packet's `answers_factory_order.blob_sha` must equal this.
- Immutable sources (unchanged): intake-2026-07-12.md sha256 `76b30d68d9dbfbd29ba93607ca9dd3fd4fbdf482092463294c1fe3bcf529c30d`; intake-restatement-2026-07-12.md sha256 `1e6a52b03125f6ee4ced06433af8276df10b0ab10e0d92b227a45ba32ec85f35`; intake-confirmation-2026-07-12.md sha256 `b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310`.
- Author-side IADA round 5 (context only, grants nothing): iada/iada.result-r5.md.
- Subject repo: /Transpara/transpara-ai/repos/hive, design base main `bf3f126` — read base bytes with `git -C /Transpara/transpara-ai/repos/hive show bf3f126:<path>`.

ROUND 4 TASKS:
1. Re-bind everything above (blob equality + packet↔FO binding).
2. Verify EACH round-3 blocker is now actually repaired in the current bytes, against the code at base bf3f126:
   - R3-F1: D4 critic row now says the API curl is always present with an empty bearer (no gate); critic is an inspection row (rule b, no key-conditional branch); the named critic tests are described as instruction-text coverage that does NOT assert gating. Confirm against critic.go:210-239 and critic_test.go.
   - R3-F2: confirm the empty-key-contacts-remote CLASS (council + mcp-graph + critic) is stated in FO D-1, FO Named Residual Risk, FO Non-Goals, FO Deployment Window, and packet D4 mcp-graph row / §5 / §7; confirm mcp-graph's apiGet/apiPost always Do and that `TestToolRespond_postsToAPI` (main_test.go, empty key) proves the POST; confirm the deferred order now covers the class; confirm no remaining "never fail-open" blanket claim.
   - R3-F3: confirm the webhook row no longer claims "all other sites compare raw to empty" and correctly says several sites don't compare at all.
   - R3-F4: confirm both docs now say the skill sets the key to `dev` (not blank) and that the local `--api` pin — not the key — is the control; check against the skill files at base (skills/hive-lifecycle/codex/SKILL.md and .claude/skills/hive-lifecycle/SKILL.md).
   - Obs 9: confirm D4/§5 now cite runPipeline@597/601-608, runRunner@439/443-451, ingest@134-136.
3. Re-confirm the round-3 SOUND items did not regress (bindings; AC-1 scan; AC-7 status allowlist; the rest of the D4 table; AC-1…AC-9 and the gate predicate unchanged).
4. Fresh adversarial sweep of the CHANGED text for NEW defects: any contradiction among the class residual, D4 rows, Non-Goals, and Deployment Window; any new false universal or unverifiable claim; any AC-9 row left unproved or with a swapped method by the critic reclassification; any citation you can refute. Be adversarial — hunt for exact claims your independent read of the code contradicts.
5. Same output format: numbered findings (DESIGN-BLOCKER / OBSERVATION with section + evidence), ending with exactly one line:
   CFADA VERDICT: PASS (0 design blockers)
   or
   CFADA VERDICT: FAIL (<n> design blockers)
