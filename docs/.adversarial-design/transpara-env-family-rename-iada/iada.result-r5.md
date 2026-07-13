# IADA Result (round 5) — DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME

- **assessor:** claude (Claude/Anthropic — author family; self-directed)
- **assessed_at:** 2026-07-12T20:59:40Z
- **branch:** none — pre-code, TLC stage 4 rerun after the CFADA round-3 repair
  set (round-4 IADA credit stranded on blobs `baa1525a…`/`e05e18b4…`)
- **issue:** none — channel A; sources hash-pinned (`76b30d68…`, `1e6a52b0…`,
  `b6b06615…`)

## Packet binding

| Object | doc_id | version | git blob SHA |
|---|---|---|---|
| Design packet (assessed) | DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME | 0.6.0 | `027fea3dabe95b134363a463983d14e7048b81b7` |
| Factory Order (answered) | FO-HIVE-TRANSPARA-API-KEY-RENAME | 0.7.0 | `1885e16feed37d84d80a1b9b3f993cdf362b1f96` |

Binding integrity: packet `answers_factory_order.blob_sha` == `git hash-object`
of the FO (`1885e16f…`), verified by direct comparison; all three FO-blob
references in the packet updated; no stale `e05e18b4…`/`740a94ce…` remains.

## Verdict

**PASS — 0 author-repairable design blockers.** Ready for CFADA round 4.

## Method correction (why round-4 IADA existed at all)

Round-3 CFADA found 4 blockers my round-4 IADA missed, because that IADA
scoped itself to the D-1 delta and trusted the prior session's R2-F4/F5 code
claims without re-deriving them from source. Corrected here: **every repaired
claim below was re-verified against `git show bf3f126:<file>` before writing.**

## R3 blocker repairs — each verified against base bf3f126

1. **R3-F1 (critic).** `git show bf3f126:pkg/runner/critic.go` — `buildCriticInstruction`
   (210-239) is a single `Sprintf` whose template **unconditionally** contains
   the `curl … -H "Authorization: Bearer %s" …` line; there is no empty-key
   branch. `critic_test.go:139-155` (`…WithoutAPIKeyKeepsReviewContract`)
   asserts only review-contract strings, never the curl's absence. **Repair:**
   D4 critic row now states the curl is always present (empty `Bearer` on empty
   key, same shape as council); critic is classified as a verbatim-interpolation
   **inspection** row (FO R3 rule b: no key-conditional branch); the named
   critic tests are re-described as instruction-text coverage that does NOT
   assert gating, and noted as untouched by the env rename (they pass the key as
   a literal arg).
2. **R3-F2 (mcp-graph — the class).** `cmd/mcp-graph/main.go` — `apiGet`/`apiPost`
   (317-367) set the Authorization header only `if s.apiKey != ""` but call
   `s.client.Do(req)` **unconditionally**; `newServer` (112-126) defaults
   `baseURL` to `https://transpara.ai`. `main_test.go` — `newTestServer(_, "")`
   (empty key) drives `TestToolRespond_postsToAPI`, which asserts the POST body
   is received. So an empty-key mcp-graph tool call reaches the default remote.
   **Repair:** the empty-key-contacts-remote issue is re-scoped from "council
   only" to a **class** (council + mcp-graph + critic) across the FO D-1 block,
   FO Named Residual Risk, FO Non-Goals, FO Deployment Window, and the packet's
   D4 mcp-graph row, §5, and §7; the deferred future order now covers the class;
   the D4 mcp-graph row + §5 cite `TestToolRespond_postsToAPI`.
3. **R3-F3 (false universal).** The D4 webhook row's "all other sites compare
   the raw value to empty" is contradicted by council (pass-through), critic
   (unconditional interpolation), and the seven runners (no comparison).
   **Repair:** reworded — the other *gating* sites compare raw-empty; several
   sites don't compare at all (verbatim interpolation).
4. **R3-F4 (mitigation).** `skills/hive-lifecycle/codex/SKILL.md:386-388` and
   `.claude/skills/hive-lifecycle/SKILL.md:343-345` run council as
   `LOVYOU_API_KEY=dev … council --api http://localhost:8082 …`; the skill's own
   warning says council posts to `--api` regardless of key. So the key is set to
   `dev` (not blank) and the **local `--api` pin** is the control. **Repair:**
   both docs corrected — `dev` not blank, `--api` pin not key.
5. **Obs 9 (citations).** Verified `runRunner` (role) reads the key at 439/443-451
   and `runPipeline` (pipeline) at 597/601-608 — the packet had them swapped —
   and ingest reads at 134-136, not 123. **Repair:** D4 and §5 citations fixed.
6. **Obs 11 (unverifiable note).** The "rotation remediation session opened
   separately 2026-07-12" identifier was removed from both docs.

## Fresh sweep / no new defect

- **Delta scope.** The repairs are claim-accuracy only: no requirement, AC,
  scan protocol, proof-map rule, or gate predicate changed (FO R1-R7, packet
  AC-1…AC-9, the fenced R1 Scan Protocol, and the allowlist gate predicate all
  present and unchanged — grep-confirmed). The rename still preserves every
  behavior verbatim; the class is preserved, not altered.
- **AC-9 one-proof-per-row holds.** critic → inspection (rule b); mcp-graph →
  `TestNewServerDefaults` (env-read rename) + `TestToolRespond_postsToAPI`
  (send-on-empty); no row unproved, no method swapped.
- **Coherence.** No live section states D-1 as open; the two remaining
  "OPEN"/"blocks" hits are historical revision entries carrying explicit
  forward-pointers to the preserve resolution (codex round-3 obs 10 confirmed
  this pattern). The class residual, D4 rows, Non-Goals, and Deployment Window
  now tell one consistent story.
- **No over-claim.** No residual names an artifact an auditor cannot reach
  (the "filed follow-up" and "remediation session" phrasings are both gone).

## Residual risks (carried)

Rotation-only neutralization of the exposed key (owner Michael); the
empty-key-contacts-remote **class** residual (council + mcp-graph + critic,
owner Michael, deferred as a class); hive#283 merge-order assumption; the
corrected deployment-window statement.

## Ready for CFADA

**Yes — 0 blockers.** CFADA round 4 runs at packet `027fea3d…` / FO
`1885e16f…`. This IADA does not satisfy CFADA and authorizes nothing; Human
Design Review (stage 6) follows a clean CFADA.
