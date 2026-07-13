You are the CFADA auditor (cross-family adversarial design audit, TLC stage 5), ROUND 7, for a Claude-authored design packet you audited in rounds 1-6. Audit READ-ONLY: no file changes, no state-changing commands. Read-only commands OK (git show/grep/hash-object/diff, rg, sha256sum, go doc).

ROUND 6 RECAP — you returned FAIL (3 design blockers) on packet v0.8.0 / FO v0.9.0:
- R6-1: the "un-gated empty-key" class named only council/mcp-graph/critic, but the seven non-branching runners (architect, council, pm, reflector, scout, scribe, spawner) also read the key ungated per the D4 truth table.
- R6-2: the pipeline/role locality gate is `strings.Contains(apiBase,"localhost") || strings.Contains(apiBase,"127.0.0.1")` (main.go:445-450,601-607) — a SUBSTRING test, so `https://localhost.example.invalid` passes; the "critic never transmits to a remote" claim was false.
- R6-3: the edited operator docs ship known-false claims — `skills/hive-lifecycle/{codex/SKILL.md,.claude/skills/hive-lifecycle/SKILL.md}` say council's `--api` defaults to `https://transpara.ai` (router.go:286 + router_test.go prove it is `http://localhost:8082`), and `cmd/mcp-graph/main.go:17` calls the key "required" though 112-126/317-367 make it optional. You asked for claim-accuracy edits + an acceptance check.
You CONFIRMED sound (obs 4-6): bindings + intake hashes; the round-5 fixes persist only-in-revision-history where prohibited; FO Constraints narrowed; §7 named the governing FO; the gate machinery (AC-1 scan, AC-7 allowlist, AC-1…AC-9, default-deny predicate, AC-9 assignments, critic inspection) intact.

The operator (Michael) chose "right-size": make the residual accurate + complete but hand the exhaustive per-site analysis to the deferred future order; fix the false docs in-scope; add an acceptance check. The author repaired accordingly. All repairs are claim-accuracy; no runtime behavior changed.

NEW OBJECTS UNDER AUDIT (bind FIRST; any blob mismatch ⇒ STOP):
- Design packet v0.9.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/design-transpara-env-family-rename-v0.9.0.md — expected blob `66d969277da95d9526f9e82b9956a68e97456cc7`.
- Factory Order v0.10.0: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/FO-hive-transpara-api-key-rename-v0.10.0.md — expected blob `7fd4e7bad3b317a43258bc8162140fe98edc4717`. The packet's `answers_factory_order.blob_sha` must equal this.
- Immutable sources (unchanged): intake-2026-07-12.md sha256 `76b30d68d9dbfbd29ba93607ca9dd3fd4fbdf482092463294c1fe3bcf529c30d`; intake-restatement-2026-07-12.md sha256 `1e6a52b03125f6ee4ced06433af8276df10b0ab10e0d92b227a45ba32ec85f35`; intake-confirmation-2026-07-12.md sha256 `b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310`.
- Author-side IADA round 8 (context only): iada/iada.result-r8.md.
- Subject repo: /Transpara/transpara-ai/repos/hive, base main `bf3f126`.

ROUND 7 TASKS:
1. Re-bind (blob equality + packet↔FO binding).
2. Verify each round-6 blocker is repaired:
   - R6-1: confirm the un-gated class in FO Named Residual and packet §7 now enumerates council, mcp-graph, critic, AND the seven runners; confirm this matches the D4 truth table's runner rows.
   - R6-2: confirm FO Named Residual + packet §7 + D4 pipeline/role rows state the exact `strings.Contains` substring predicate and that no live text still claims critic/runners "never" reach a remote; verify the predicate against main.go:445-450,601-607.
   - R6-3: confirm new FO R8 + packet D9 + AC-10 require correcting (a) the council `--api` default claim in both skill dialects and (b) the mcp-graph "required"-key doc-comment; confirm the gate predicate now spans AC-1…AC-10. Verify the underlying facts: router.go:286 + router_test.go (council default localhost:8082); mcp-graph main.go:17 vs 112-126/317-367 (key optional).
3. Confirm the "right-size" scoping is legitimate, not an evasion: the residual states the accurate + complete enumeration and the exact predicate (it does NOT omit or hand-wave a checkable fact); only the further remediation/analysis is deferred. Flag as a blocker any place a currently-checkable, in-scope claim is deferred instead of stated.
4. Re-confirm no regression (bindings; AC-1 scan; AC-7 allowlist; AC-1…AC-10; default-deny predicate; D4; critic inspection).
5. Fresh adversarial sweep of the changed text for NEW defects across the whole document domain — any contradiction, any remaining false universal, any code/citation claim your independent read contradicts, any AC row unproved, any R8/AC-10 gap. Be adversarial.
6. Output: numbered findings (DESIGN-BLOCKER / OBSERVATION with section + evidence), ending with exactly one line:
   CFADA VERDICT: PASS (0 design blockers)
   or
   CFADA VERDICT: FAIL (<n> design blockers)
