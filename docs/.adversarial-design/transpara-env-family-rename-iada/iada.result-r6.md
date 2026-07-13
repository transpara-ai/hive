# IADA Result (round 6) — DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME

- **assessor:** claude (Claude/Anthropic — author family; self-directed)
- **assessed_at:** 2026-07-12T21:11:06Z
- **branch:** none — pre-code, TLC stage 4 rerun after the CFADA round-4 repair
  set (round-5 IADA credit stranded on blobs `027fea3d…`/`1885e16f…`)
- **issue:** none — channel A; sources hash-pinned (`76b30d68…`, `1e6a52b0…`,
  `b6b06615…`)

## Packet binding

| Object | doc_id | version | git blob SHA |
|---|---|---|---|
| Design packet (assessed) | DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME | 0.7.0 | `0d2838f2c69b6e75a4cb56b17d784db09afd5a48` |
| Factory Order (answered) | FO-HIVE-TRANSPARA-API-KEY-RENAME | 0.8.0 | `d39e38a5734f262a41aca9c26d03aac1df046f4d` |

Binding: packet `answers_factory_order.blob_sha` == `git hash-object` of the FO
(`d39e38a5…`), verified by direct comparison; no stale `1885e16f…` remains.

## Verdict

**PASS — 0 author-repairable design blockers.** Ready for CFADA round 5.

## Method correction #2 (why round-5 IADA existed)

Round-4 CFADA found the round-3 repairs were each fixed in the reported section
but left standing in a sibling — the "fix the class, not the instance"
failure (Michael's CLAUDE.md). Round-5 IADA's central check is therefore a
**four-class sweep of BOTH documents**, not a spot re-read: for each corrected
claim-class, `grep` every occurrence and confirm every remaining hit is a
historical revision-history line, never a live normative claim.

## R4 blocker repairs — verified, each swept across BOTH docs

1. **R4-1 ("never fail-open" survived in D6).** Fixed packet D6; sweep for
   `never (fail-)?open` / `fail-closed or default-safe, never` across FO + packet
   returns only the two revision-history entries describing the fix. Zero live.
2. **R4-2 (false universal survived in FO R3).** Fixed FO R3 body (now "the
   other *gating* sites compare raw-empty and several sites — council, critic,
   the seven runners — do not compare at all"); sweep for
   `every other site` / `all other sites compare` returns only revision-history
   lines. Zero live. (The packet D4 row was already correct at round 4.)
3. **R4-3 (`--api` pin over-claimed for the whole class).** Verified against
   `git show bf3f126:cmd/mcp-graph/main.go`: it reads `LOVYOU_BASE_URL`
   (→`TRANSPARA_BASE_URL`) at line 114, defaults to `https://transpara.ai`, and
   has NO `flag.*` parsing → no `--api` flag. So the `--api` pin cannot mitigate
   mcp-graph. The mitigation is now **split path-by-path in every affected
   section of both docs** (FO Named Residual class, FO Deployment Window, packet
   §7 deployment + class residual): `--api` pin for council/critic (critic's
   curl targets the pipeline's `--api` base), a local `TRANSPARA_BASE_URL` for
   `cmd/mcp-graph`. Sweep for a bare whole-class `--api` mitigation returns zero
   live over-claims; the mcp-graph carve-out appears in both docs.
4. **Obs 11 (remediation-session note survived in Non-Goals).** Removed the last
   live instance (FO Non-Goals); sweep for `remediation session opened
   separately` returns only the revision-history line that records its removal.
   Zero live.

## Fresh sweep / no new defect

- **Delta scope.** Claim-accuracy only. FO R1-R7, packet AC-1…AC-9, the fenced
  R1 Scan Protocol, and the allowlist gate predicate are all present and
  unchanged (grep-confirmed). No behavior claim changed beyond precision; the
  rename still preserves everything verbatim.
- **Coherence.** D6, the Deployment Window, §7's two residuals, D4, and the FO
  D-1 block now tell one consistent path-split story: key-gating paths are
  fail-closed/default-safe; the empty-key-contacts-remote class
  (council + mcp-graph + critic) is not, is preserved, and is mitigated
  path-by-path in the interim. No section contradicts another.
- **Historical entries.** The revision-history lines that mention "OPEN",
  "never open", or "all other sites compare raw-empty" are records of prior
  versions' content; the live text supersedes them and the D-1 ones carry
  forward-pointers (codex round-3 obs 10 confirmed that pattern is acceptable).

## Residual risks (carried)

Rotation-only neutralization of the exposed key (owner Michael); the
empty-key-contacts-remote class residual (council + mcp-graph + critic, owner
Michael, mitigated path-by-path, deferred as a class); hive#283 merge-order
assumption; the corrected deployment-window statement.

## Ready for CFADA

**Yes — 0 blockers.** CFADA round 5 runs at packet `0d2838f2…` / FO
`d39e38a5…`. This IADA does not satisfy CFADA and authorizes nothing.
