# IADA Result (round 7) — DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME

- **assessor:** claude (Claude/Anthropic — author family; self-directed)
- **assessed_at:** 2026-07-12T21:24:46Z
- **branch:** none — pre-code, TLC stage 4 rerun after the CFADA round-5 repair
  set (round-6 IADA credit stranded on blobs `0d2838f2…`/`d39e38a5…`)
- **issue:** none — channel A; sources hash-pinned (`76b30d68…`, `1e6a52b0…`,
  `b6b06615…`)

## Packet binding

| Object | doc_id | version | git blob SHA |
|---|---|---|---|
| Design packet (assessed) | DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME | 0.8.0 | `01e91082a3304acbd0e46e8941cc3981a96be3a9` |
| Factory Order (answered) | FO-HIVE-TRANSPARA-API-KEY-RENAME | 0.9.0 | `b47b0bcee98bf348a581410d80c38d3c85fb6722` |

Binding: packet `answers_factory_order.blob_sha` == `git hash-object` of the FO
(`b47b0bce…`), verified by direct comparison; no stale `d39e38a5…` remains.

## Verdict

**PASS — 0 author-repairable design blockers.** Ready for CFADA round 6.

## The substantive correction (R5-2), verified against code

Round 5 found the empty-key **reachability was overstated** — my residual said
council + mcp-graph + critic all reach the default remote on an empty key.
Verified at `bf3f126`, the truth is narrower, and I mapped every `--api` default
in `cmd/hive/router.go` to its owning command to be sure:

| Command | `--api` default | Empty-key behavior | Reaches remote on empty key? |
|---|---|---|---|
| `cmdCivilizationRun` (router.go:131) | `https://transpara.ai` | Site client **disabled** (main.go:1284-1288) | No — gated |
| `cmdIngest` (router.go:266) | `https://transpara.ai` | **required error** (main.go:134-136) | No — gated |
| `pipelineFlags` (router.go:173) | `localhost:8082` | required error if non-local base | No — gated + local default |
| `roleFlags` (router.go:234) | `localhost:8082` | required error if non-local base | No — gated + local default |
| `newCouncilFlagSet` (router.go:286) | `localhost:8082` | **no gate**; posts to `--api` | Only under an explicit remote `--api` |
| `cmd/mcp-graph` (no `--api`; `TRANSPARA_BASE_URL`, main.go:114) | `https://transpara.ai` | **no gate**; always `client.Do` | **Yes — the one path** |
| critic (inside pipeline) | inherits pipeline `--api` | empty bearer in curl, path gated to local | No — pipeline-gated to local |

So **only `cmd/mcp-graph`** contacts the default remote on an empty key. The
class is renamed **"un-gated empty-key paths"** and D-1 block, Named Residual,
Deployment Window, Non-Goals, D4 council row, D6, §5 council/critic inspection,
and §7 deployment + class residual are all restated with this accurate
per-path reachability. The R4-3 mitigation split (which Codex confirmed
accurate) is retained.

## R5 blockers — all repaired, swept by MEANING

- **R5-1** (Constraints "Fail-closed defaults throughout") — narrowed to the
  order's own changes with a carve-out; grep for `fail-closed.*throughout`
  returns only the revision-history line describing the fix. Zero live.
- **R5-2** (reachability overstatement) — corrected everywhere per the table
  above; grep for any live line claiming council/critic reach the remote by
  default returns none (only accurate "council reaches a remote only under an
  explicit remote `--api`" and historical revision entries).
- **R5-3** (packet §7 heading stale FO version) — rebound; the heading now
  reads "inherited from FO v0.9.0", matching the binding.

## No new defect

- **Delta scope.** Claim-accuracy only. FO R1-R7, packet AC-1…AC-9, the R1 Scan
  Protocol, and the allowlist gate predicate are present and unchanged
  (grep-confirmed). No behavior claim changed beyond precision.
- **Coherence.** D4, D6, §5, §7's two residuals, the FO D-1 block, Deployment
  Window, Non-Goals, and Constraints now tell one consistent story: the gated
  paths fail safe; only mcp-graph reaches the default remote on an empty key;
  council defaults local; critic is pipeline-gated local; all preserved,
  mitigated path-by-path, deferred as a class.
- **Historical entries.** Revision-history lines that state the earlier
  overstatement or the earlier class name are records of prior versions; Codex
  round 5 audited the same historical entries and flagged only live text, so
  they are correctly treated as non-normative.

## Ready for CFADA

**Yes — 0 blockers.** CFADA round 6 runs at packet `01e91082…` / FO
`b47b0bce…`. This IADA does not satisfy CFADA and authorizes nothing.
