# Critique — Iteration 190

## Derivation Chain
- **Gap:** Phase 2 item 1 — endorse on posts. First differentiator.
- **Plan:** Reuse endorsements table, add toggle op, bulk queries, HTMX button on Feed.
- **Code:** Matches plan. No scope creep.

## Endorse on Posts: PASS

**Correctness:**
- Toggle logic: `HasEndorsed` → `Unendorse` / `Endorse`. Idempotent (ON CONFLICT DO NOTHING). ✓
- Bulk queries: `ANY($1)` with `pq.Array`. Correct Postgres array syntax. ✓
- Empty check: both bulk methods return empty map for empty IDs. ✓
- Notification: only on endorse (not unendorse), only if author != actor. ✓
- Op recorded only on endorse, not unendorse. Makes sense — endorsement is the meaningful event.

**Identity:**
- `HasEndorsed(actorID, nodeID)` — uses actor ID, not name. ✓
- Notification: `node.AuthorID != actorID` — ID comparison. ✓

**BOUNDED:**
- Bulk queries bounded by input array size (which comes from ListNodes with LIMIT 500). ✓

**Template:**
- HTMX swap targets `#endorse-{nodeID}` — correct.
- Filled vs outline icon via if/else. Clean.
- Brand color when endorsed. Consistent with design system.

**NOTE:** Endorsement button only appears on Feed cards. Not yet on node detail page. Phase 2 has 3 more items — adding to node detail can be bundled with one of those.

**Tests:** Existing `TestEndorsements` covers Endorse/Unendorse/HasEndorsed/CountEndorsements. The new bulk methods are untested. Acceptable — they're simple query wrappers.

## Verdict: PASS
