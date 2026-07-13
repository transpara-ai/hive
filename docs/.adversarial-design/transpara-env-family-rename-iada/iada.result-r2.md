# IADA Result (round 2) — DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME

- **assessor:** claude (Claude/Anthropic — author family; self-directed)
- **assessed_at:** 2026-07-12T20:00:33Z
- **branch:** none — pre-code, TLC stage 4 rerun (packet materially changed
  after round 1 by the CFADA round-1 repair set; stale round-1 credit is
  stranded on blobs `42ff97c0…`/`9080f0e0…` per the blob-binding rule)
- **issue:** none — channel A; intake, restatement, and confirmation archived
  and hash-pinned (`76b30d68…`, `1e6a52b0…`, `b6b06615…`)

## Packet binding

| Object | doc_id | version | git blob SHA |
|---|---|---|---|
| Design packet (assessed) | DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME | 0.3.0 | `81b2b32515645af768da6b572bc4945dce36af87` |
| Factory Order (answered) | FO-HIVE-TRANSPARA-API-KEY-RENAME | 0.4.0 | `9de5cc74be25b56e91cae43df8e7f397332a9ed3` |

## Scope

Unchanged from round 1 except as repaired: family rename across 26 functional
tracked files at hive base `bf3f126` (31 files / 121 matching lines / 124
occurrences total; 5 historical files retained), hard cutover, credential
entry deleted from `loop/mcp-graph.json`, operator runbook, governed-doc
carry per D7.

## Verdict

**PASS — 0 open design blockers** at the bound SHAs above. This round
assessed the repair set for CFADA round-1 findings F2–F6 and observations
7/9. Author-side supporting evidence only; does not satisfy CFADA.

## What this round checked (evidence)

1. **F2 repair sound.** The fenced R1 Scan Protocol's exact command was
   executed at base `bf3f126`: exit 0 with 94 matching lines (live), and the
   v0.2.0 escaped-pipe variant reproduces the false-pass (0 lines) — the
   protocol's base-liveness precondition detects that class. Exit semantics
   (1 = pass, 0 = fail, ≥2 = deny) stated in FO and D8. Residual instance
   audited: the only regex remaining in a table cell (AC-2's
   `lv_[0-9a-f]{16,}`) contains no character altered by markdown table
   escaping; D8 restated to exactly that claim (no false universal).
2. **F3 repair sound.** AC-7/R7 protected domain now covers
   `docs/factory-orders/`, `docs/designs/` (18 existing tracked files
   verified via `git ls-files`), `docs/superpowers/`,
   `docs/.adversarial-design/`, `docs/runbooks/`, `loop/reflections.md`;
   no-`M`/`D` + allowlisted-`A` check named. Whole-domain argument: every
   governed tree is either protected from mutation or excluded-from-scan
   with a named justification, and both lists are the same list (D7 ↔ R1
   protocol exclusions ↔ AC-7 domain).
3. **F4 repair sound.** D4 truth table re-verified against code at base:
   pipeline/role `dev`-fallback (`cmd/hive/main.go:443-451,601-608`), council
   verbatim-env client (`:506-512`), civilization Site-client gate
   (`:1284-1288`), post soft-skip (`cmd/post/main.go:34-44`), reply required
   (`cmd/reply/main.go:67-75`), republish-lessons exit 1
   (`cmd/republish-lessons/main.go:28-32`), mcp-graph defaults in
   `newServer()` (`cmd/mcp-graph/main.go:112-126`). §5 names only tests
   verified to exist (`TestRunIngest_MissingAPIKey`,
   `TestResolveWebhookBearerTokenWholeDomain` at `router_test.go:134`, the
   four preflight tests, `TestSpaceFor_fallsBackToDefault` at
   `main_test.go:34`) plus three enumerated new tests; AC-9 executes the
   proof map row-by-row with no review-time method swap.
4. **F5 repair sound.** The "no test seam" claim is corrected: mcp-graph
   defaults get a named test through the verified `newServer()` seam;
   inspection is reserved for sites verified seamless (post/reply env reads
   inline in `main`/`run`; reply has no test file — `ls cmd/reply/`).
5. **F6 repair sound.** The plain-language restatement Michael answered
   "1. yes" to is archived verbatim and hash-pinned (`1e6a52b0…`) with the
   FO-version chain (v0.1.0 blob `df8aa1c2…` → v0.2.0 blob `c76846e8…`);
   the FO cites it as the confirmed-reading referent, and hive-only scope is
   grounded in the repo-set sweep row (functional refs only in hive; site is
   a submodule pin). Externalization, runbook, and historical-protection all
   appear verbatim in the confirmed restatement (items 5, 6, 7).
6. **Obs 7/9 repairs.** Counts now 31 files / 121 matching lines / 124
   occurrences everywhere; `.gitignore` claim softened to accidental-commit
   prevention; D7 names its path as a hive adaptation of the docs-repo root
   precedent; AC-8 re-traced to R7 + the truth-object constraint.
7. **Recurrence sweep (fix the class).** Searched both documents for other
   universal quantifiers resting on unverified facts: the remaining
   universals ("every functional reference", "no code path reads", "ALL
   individually evidenced") are each backed by a named whole-domain check
   (R1 protocol, AC-3 test run, gate predicate). No further instance found.

## Scope audit — what the packet does NOT do

Unchanged from round 1: no code, no branch, no PR, no merge, no runtime, no
deploy, no rotation, no history rewrite, no issue mutation, no EventGraph
write, no autonomy change, no upstream contact, no site change, no behavior
change (council empty-bearer oddity explicitly preserved and flagged).

## Residual risks (carried, not closed)

1. Exposed `lv_b7fb22…` in permanent public history — operator rotation is
   the sole neutralizer (separate session; outcome not assumed).
2. hive#283 merge-order — bounded by §2's assumption and AC-1's re-inventory
   + base-liveness at the actual base.
3. Fail-closed deployment window between merge and runbook execution.

## Ready for CFADA (round 2)

**Yes** — 0 open design blockers at packet blob `81b2b325…` / FO blob
`9de5cc74…`. This IADA is self-directed and does **not** replace CFADA; it
produces no gate status.
