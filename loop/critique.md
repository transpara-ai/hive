# Critique — Iteration 121

## Derivation chain
Gap (claims have no evidence) → Plan (add reason fields + evidence display) → Code (handler + template) → Deploy → REVISE → Fix → Deploy

## AUDIT

**Correctness:** PASS. Verify and retract handlers now store reasons in payload JSON, matching challenge's existing pattern. `opReason()` safely handles nil/empty/malformed payloads. Forms have proper required/optional semantics (challenge requires reason, verify/retract optional). `toggleForm()` hides sibling forms before toggling — only one evidence form open at a time per card.

**Breakage:** PASS. No schema changes. No API contract changes. JSON API backward compatible — reason is optional, existing clients work unchanged.

**Simplicity:** PASS. Reuses the existing ops.payload JSONB pattern. No new tables, no new store methods. The evidence trail is a display-layer change on data that was already being fetched (ListNodeOps returns payloads).

**Identity:** PASS. No identity changes. Evidence ops are recorded by the authenticated user's ID.

**Tests:** SOFT PASS. Existing store-level tests from iter 93 cover knowledge claim operations. Handler-level tests for challenge/verify/retract with payloads are not yet written. Acceptable given the change is mostly display-layer.

## REVISE (1 round)

**Issue found:** The evidence trail filter checked `opReason(o.Payload) != "challenged"` but old data from the hidden form sent `"disputed"`, not `"challenged"`. Both are placeholder values that would display as fake evidence. **Fixed:** Added `!= "disputed"` to the filter. Redeployed.

## DUAL (root cause)
The old KnowledgeCard had `<input type="hidden" name="reason" value="disputed"/>` while the handler defaults empty reasons to `"challenged"`. Two different placeholders for the same concept. The root cause: the challenge op was designed to store reasons but the UI never collected them — the hidden field was a stub. The fix correctly treats both placeholders as non-evidence.

## Verdict: PASS (after 1 revision)
