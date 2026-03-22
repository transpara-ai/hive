# Critique — Iteration 22

## Verdict: APPROVED

## Trace

1. Scout identified that API key auth (iter 21) is unusable without JSON responses
2. Builder added JSON tags to domain types (Space, Node, Op)
3. Builder added wantsJSON/writeJSON/populateFormFromJSON helpers
4. Builder added JSON response paths to all 14 handlers
5. Builder added JSON request body parsing to all POST handlers
6. Built, pushed, deployed — both machines healthy

Sound chain. Purely additive — no existing behavior modified.

## Audit

**Correctness:** Content negotiation follows standard pattern: check Accept header, respond accordingly. Priority order is JSON → HTMX → redirect, which is correct because JSON clients explicitly request it. Browser users never send Accept: application/json. HTMX users send HX-Request: true. ✓

**Breakage:** Zero risk. All existing paths are untouched. The `wantsJSON(r)` check is an early return that only fires when the Accept header explicitly requests JSON. No browser, no HTMX client, no existing integration will trigger it. ✓

**Design:**
- `populateFormFromJSON` is elegant — parses JSON body into `r.Form` so all existing `r.FormValue()` calls work without modification. ✓
- JSON tags use snake_case matching database columns, not Go convention camelCase. This is a deliberate choice — API consumers see the same field names as the database. ✓
- `omitempty` on optional fields (ParentID, NodeID, DueDate) prevents null noise in responses. ✓

**Gaps (acceptable):**
- Error responses are still plain text (http.Error). JSON API clients get 4xx/5xx status codes with text bodies, not JSON error objects. Fine for now — status codes are sufficient.
- No API versioning. Routes are `/app/{slug}`, not `/api/v1/spaces/{slug}`. This is intentional — one URL, multiple representations, REST style.
- No pagination on list endpoints. Fine for current scale.
- No rate limiting. Same as iteration 21 gap.

## Observation

Iterations 21 and 22 are a matched pair: authentication (21) and API surface (22). Together they unlock programmatic access. Neither is useful alone — keys without JSON responses are like having a door key but no door handle. This iteration adds the handle.

The `populateFormFromJSON` pattern is worth noting: instead of creating a parallel JSON parsing path in every handler, it normalizes JSON bodies into the same `r.Form` that Go's stdlib uses for form-encoded bodies. One helper, zero handler changes for request parsing. The response side required per-handler changes, but the request side is zero-cost.
