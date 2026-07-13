# IADA Result (round 8) — DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME

- **assessor:** claude (Claude/Anthropic — author family; self-directed)
- **assessed_at:** 2026-07-12T22:24:09Z
- **branch:** none — pre-code, TLC stage 4 rerun after the CFADA round-6 repair
  set (operator chose "right-size"); round-7 IADA credit stranded on prior blobs
- **issue:** none — channel A; sources hash-pinned (`76b30d68…`, `1e6a52b0…`,
  `b6b06615…`)

## Packet binding

| Object | doc_id | version | git blob SHA |
|---|---|---|---|
| Design packet (assessed) | DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME | 0.9.0 | `66d969277da95d9526f9e82b9956a68e97456cc7` |
| Factory Order (answered) | FO-HIVE-TRANSPARA-API-KEY-RENAME | 0.10.0 | `7fd4e7bad3b317a43258bc8162140fe98edc4717` |

Binding verified: packet `answers_factory_order.blob_sha` == `git hash-object`
of the FO (`7fd4e7ba…`); no stale `b47b0bce…` remains.

## Verdict

**PASS — 0 author-repairable design blockers.** Ready for CFADA round 7.

## Approach: "right-size" (operator direction)

Round 6 showed the residual was over-precisely pinning behavior this rename
doesn't change, which invites an endless precision hunt. Per Michael's "right-
size" choice, the residual is now **accurate + complete** (enumerates every
un-gated reader; states the exact locality predicate) but **hands the
exhaustive per-site analysis and any remediation to the deferred future order**
rather than fully resolving it here.

## R6 blocker repairs — each verified against base bf3f126

- **R6-1 (class incomplete).** The un-gated class now enumerates council,
  `cmd/mcp-graph`, critic, AND the seven other runner instruction-builders
  (architect, council, pm, reflector, scout, scribe, spawner) — matching the D4
  truth-table rows that already classify those seven as verbatim-interpolation /
  no-branch. FO Named Residual + Non-Goals + D-1 block and packet §5/§7 all
  updated.
- **R6-2 (locality overstatement).** The gate is verified to be
  `strings.Contains(apiBase,"localhost") || strings.Contains(apiBase,"127.0.0.1")`
  (`cmd/hive/main.go:445-450,601-607`) — a substring test. The residual and D4
  pipeline/role rows now state that exact predicate and note a crafted host like
  `https://localhost.example.invalid` passes; the false "critic never reaches a
  remote" claim is removed (grep confirms no live instance).
- **R6-3 (false operator docs) → new FO R8 + packet D9/AC-10.** Verified:
  `cmd/hive/router.go:286` sets council `--api` default to `defaultLocalAPIBase`
  = `http://localhost:8082`, and `cmd/hive/router_test.go` asserts the bare
  council default is exactly that — so the skill dialects' "`--api` defaults to
  `https://transpara.ai`" is false. `cmd/mcp-graph/main.go:17` calls the key
  "required" though `main.go:112-126,317-367` make it optional. R8 requires the
  code-stage edit to correct both (claim-accuracy only, no runtime change),
  AC-10 verifies it, and the gate predicate now spans AC-1…AC-10. This is
  authorized by the operator's option-1 choice ("fix the false skill/mcp-graph
  docs in place, add the check").

## No new defect

- **Delta scope.** Adds R8 / D9 / AC-10 (AC count 9→10) plus claim-accuracy
  restatements; the R1 Scan Protocol, R1-R7, AC-1…AC-9, and the allowlist gate
  predicate are otherwise unchanged (grep-confirmed). No runtime behavior claim
  changed beyond precision.
- **Coherence.** The D-1 block, Named Residual, Deployment Window, Non-Goals,
  D4, D6, §5, and §7 now tell one consistent story: the gated paths fail safe;
  the un-gated readers (council, mcp-graph, critic, 7 runners) are enumerated;
  only mcp-graph defaults remote; the substring locality caveat is stated once
  and referenced; the exhaustive analysis is the deferred order's scope.
- **R8 is in-scope, not scope-creep.** The files R8 corrects are the same
  operator docs R1 already renames; a rename-only edit would re-ship the false
  claims under the new name. The edit is documentation-accuracy with no runtime
  change, consistent with D-1 (preserve).

## Ready for CFADA

**Yes — 0 blockers.** CFADA round 7 runs at packet `66d96927…` / FO
`7fd4e7ba…`. This IADA does not satisfy CFADA and authorizes nothing.
