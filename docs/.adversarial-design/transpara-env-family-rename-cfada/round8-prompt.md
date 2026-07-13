You are the CFADA auditor (cross-family adversarial design audit, TLC stage 5), ROUND 8, for a Claude-authored design packet you audited in rounds 1-7. Audit READ-ONLY: no file changes, no state-changing commands. Read-only commands OK (git show/grep/hash-object/diff, rg, sha256sum, go doc).

WHAT CHANGED SINCE ROUND 7 — the operator narrowed scope. Round 7 returned FAIL(3): (R7-1) the residual mis-grouped the council runner (reached via ungated `runCouncilCmd`) with the pipeline-gated runners; (R7-2) packet D5 ("skill dialects change by textual rename only") contradicted the new D9/AC-10 semantic edits; (R7-3) FO R8 ("no known-false operator claims in edited docs") was unbounded — AC-10 checked two claims but the skill files hold more (you found a third, the webhook bind claim). After seven rounds where the design core passed but the empty-key characterization and operator-doc accuracy kept generating findings, the operator (Michael) chose to NARROW this FO:

- This FO now does ONLY: the LOVYOU_* → TRANSPARA_* rename (R1-R4), the committed-credential removal (R5), the operator runbook (R6), and governed-records protection (R7).
- **R8 (doc-accuracy) is REMOVED** from the FO; packet **D9 and AC-10 are REMOVED**; the gate predicate is back to AC-1…AC-9.
- The D-1 "un-gated empty-key readers" residual is SHRUNK to a minimal accurate statement, and BOTH the exhaustive per-site reachability characterization AND the correction of the stale operator-doc claims (council `--api` default, mcp-graph key doc-comment, webhook bind) are deferred to a named follow-up governed order, owner Michael.

This is a SUBTRACTIVE revision: it removes scope, it does not add claims.

NEW OBJECTS UNDER AUDIT (bind FIRST; any blob mismatch ⇒ STOP):
- Design packet v0.10.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/design-transpara-env-family-rename-v0.10.0.md — expected blob `1a658fd475c5179cb60c003c47d9471727bd50fb`.
- Factory Order v0.11.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/FO-hive-transpara-api-key-rename-v0.11.0.md — expected blob `a055a071ea596b767b633a1ad4b9339847fe84f0`. The packet's `answers_factory_order.blob_sha` must equal this.
- Immutable sources (unchanged): intake-2026-07-12.md sha256 `76b30d68d9dbfbd29ba93607ca9dd3fd4fbdf482092463294c1fe3bcf529c30d`; intake-restatement-2026-07-12.md sha256 `1e6a52b03125f6ee4ced06433af8276df10b0ab10e0d92b227a45ba32ec85f35`; intake-confirmation-2026-07-12.md sha256 `b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310`.
- Author-side IADA round 9 (context only): iada/iada.result-r9.md.
- Subject repo: /Transpara/transpara-ai/repos/hive, base main `bf3f126`.

SCOPE NOTE (read before auditing): the doc-accuracy work was removed by an explicit operator scope decision and deferred to a follow-up order with owner Michael. Do NOT re-raise "the skill/mcp-graph docs still contain false claims" as a design blocker per se — that scope is intentionally out. DO, however, flag as a blocker: (a) any residual that is author-dismissed rather than owner-signed-deferred; (b) any NEW falsehood or defect the rename itself introduces (as opposed to a pre-existing one it merely renames); (c) any internal contradiction, broken FO trace, false claim, or gate hole WITHIN the narrowed scope (R1-R7, AC-1…AC-9); (d) any place the shrunk residual states something still checkably wrong.

ROUND 8 TASKS:
1. Re-bind (blob equality + packet↔FO binding).
2. Confirm the round-7 blockers are resolved by the narrowing: (R7-1) the residual asserts no per-site gating classification — verify it only says reachability "depends on operator `--api`/base config"; (R7-2) D9 is gone and D5 stands uncontradicted; (R7-3) R8 is gone from FO requirements (R1-R7 only), the Verification-Plan R8 item is gone, AC-10 is gone (AC-1…AC-9), and the gate predicate reads AC-1…AC-9.
3. Verify the shrunk residual is ACCURATE: only `cmd/mcp-graph` defaults to the remote on an empty key (`TRANSPARA_BASE_URL`→transpara.ai, no `--api` flag); council/pipeline/role `--api` default `localhost:8082` (router.go:286/173/234); the residual claims nothing more specific that could be wrong. Verify the deferral is owner-signed (owner Michael), not author-dismissed.
4. Confirm no regression WITHIN scope: bindings; AC-1 scan (base-liveness 94 lines, exit semantics); AC-7 status allowlist; AC-1…AC-9 and the default-deny predicate; D4 truth table and AC-9 proof-map assignments; the R1 rename inventory; R5 credential-removal check.
5. Fresh adversarial sweep of the narrowed documents for NEW defects introduced by the subtraction: any dangling reference to the removed R8/D9/AC-10 in live (non-revision-history) text; any now-orphaned cross-reference; any AC whose answered-requirement disappeared; any inconsistency the deletions created. Be adversarial within the narrowed scope.
6. Output: numbered findings (DESIGN-BLOCKER / OBSERVATION with section + evidence), ending with exactly one line:
   CFADA VERDICT: PASS (0 design blockers)
   or
   CFADA VERDICT: FAIL (<n> design blockers)
