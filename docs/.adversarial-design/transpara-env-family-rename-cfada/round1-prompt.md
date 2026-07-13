You are the CFADA auditor (cross-family adversarial design audit, TLC stage 5) for a Claude-authored design packet. You are the materially independent reviewer family (Codex/OpenAI). Audit READ-ONLY: do not modify, create, or delete any file; do not run any state-changing command (no git commit/push/checkout, no network mutations). You may read files and run read-only commands (git log/show/grep/hash-object, rg, sha256sum) inside this workspace.

Objects under audit (bind FIRST — recompute and confirm; any mismatch = binding failure, report and stop):

- Design packet: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/design-transpara-env-family-rename-v0.2.0.md — doc_id DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME, version 0.2.0, expected git blob SHA 42ff97c0b139fbc4f77eb5f8d607d9f2119a3b56 (verify: git hash-object <file>).
- Factory Order it answers: /Transpara/transpara-ai/artifacts/hive/fo-transpara-api-key-rename/FO-hive-transpara-api-key-rename-v0.3.0.md — doc_id FO-HIVE-TRANSPARA-API-KEY-RENAME, version 0.3.0, expected blob SHA 9080f0e0f0cb2882e1be8c173248bcf0ad3453a5.
- Source-of-intent archives (immutable): same directory, intake-2026-07-12.md expected sha256 76b30d68d9dbfbd29ba93607ca9dd3fd4fbdf482092463294c1fe3bcf529c30d; intake-confirmation-2026-07-12.md expected sha256 b6b06615e4c7becdbfd012d3e378316bc38ea7979b293cb62e2ed2f7c340f310 (verify: sha256sum).
- Author-side IADA (context only; it grants nothing and is NOT the gate): iada/iada.result.md in the same directory.
- Subject repo: /Transpara/transpara-ai/repos/hive — packet claims design base main bf3f126 (verify: git -C ... rev-parse HEAD).

Audit per the CFADA standard:

1. Binding: the four hashes and the repo base above.
2. Three fidelities, each between pinned objects:
   a. internal coherence — every AC has verification_method + risk_class; named TestCases exist; the gate predicate is an allowlist (default deny); no contradiction between sections (pay attention to whether the AC-1 scan exclusions, AC-7 path protection, and D7 doc placement are mutually consistent — this failed in v0.1.0 and was repaired; check the repair is actually coherent).
   b. packet-vs-FO — every FO requirement R1–R7 traces to at least one AC; nothing material in the FO silently dropped; nothing in the packet exceeds the FO without being named as new scope (e.g. is the .gitignore addition in D2 within the FO? is D7 within the FO?).
   c. FO-vs-source — the FO faithfully derives from the archived verbatim order ("craft a Factory Order to rename LOVYOU_API_KEY to TRANSPARA_API_KEY") plus the archived confirmation answers ("1. yes / 2. all 3 old name variables / 3. leave it"). Flag anything in the FO that the source does not support, and anything the source ordered that the FO dropped.
3. Claim-dense single-pass audit — check EVERY exact claim against the repo at the claimed base in one pass:
   - the 31-file / 121-occurrence tracked inventory of LOVYOU_(API_KEY|BASE_URL|SPACE), and the 26-functional / 5-historical split with the five historical files named;
   - the R1/D1 functional file lists (21 Go files, two skill dialects, agents/CONTEXT.md, loop/mcp-graph.json, loop/hive-runtime-spec.md);
   - cmd/mind/mcp.go lines 47-54: env built as os.Environ() + append(cfg.Env), and the packet's "last duplicate wins" claim about Go exec env handling;
   - exactly ONE tracked hit for regex lv_[0-9a-f]{16,} at loop/mcp-graph.json:5;
   - commit 3ff5e54 introduced loop/mcp-graph.json with the credential (2026-03-26);
   - defaults: LOVYOU_BASE_URL -> https://transpara.ai and LOVYOU_SPACE -> "hive" at their claimed call sites (cmd/mcp-graph/main.go, cmd/post/main.go, cmd/reply/main.go);
   - behavior claims: required-error paths (ingest, --webhook-require-auth), localhost dev default, empty-key-disables-Site-client;
   - the proposed AC-1 exclusion-listed git grep command: run it mentally/actually against the CURRENT tree MINUS the planned renames — would it really return zero after the rename, and does it really fail (non-zero) today? Is any tracked file with legacy names neither renamed by R1/D1 nor excluded by AC-1 (a gap that would make the gate unsatisfiable), and is any exclusion broader than justified (a blind spot)?
   - PR #283 open at head 57f6c36 (verify read-only if possible; if unverifiable without network, mark unverifiable — not blocking by itself).
4. Defect catalog: denylist predicates; ACs covering one failure mode of a larger domain; smuggled authority/autonomy grants; claims resting on mutable discussion; memorized target sets instead of live resolution; missing/incorrect blob binding; broken FO trace; empty/unknown/future inputs defaulting open.
5. OUTPUT FORMAT (mandatory): a numbered findings list — each finding with: severity DESIGN-BLOCKER or OBSERVATION; the packet/FO section it hits; the evidence you checked (file:line or command + output); why it blocks or does not. If a claimed fact is wrong, quote the actual fact. End with exactly one line:
   CFADA VERDICT: PASS (0 design blockers)
   or
   CFADA VERDICT: FAIL (<n> design blockers)

Be adversarial. Hunt specifically for what the author family missed — especially exact claims your independent read of the code contradicts, scan-domain gaps, and any way the gate predicate could pass while the Factory Order's intent is not actually delivered.
