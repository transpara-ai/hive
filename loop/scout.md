# Scout Report — Iteration 22

## What I Found

API key auth (iteration 21) deployed but unusable. Every graph handler returns HTML templates or HTTP redirects. An agent sending `Authorization: Bearer lv_...` with `Accept: application/json` gets HTML back. The API surface has no JSON mode.

Specific gaps:
1. **Read endpoints** (board, feed, threads, activity, node detail, people) — all render templ templates, no JSON path
2. **Write endpoints** (create space, grammar ops, node state/update/delete) — all return redirects or HTMX fragments, no JSON responses
3. **Request parsing** — all POST handlers use `r.FormValue()`, which only parses form-encoded bodies. Agents sending JSON bodies get empty values.

## What I Recommend

Add JSON content negotiation to all existing graph endpoints. Same URLs, different representation based on `Accept: application/json`. For write endpoints, also accept `Content-Type: application/json` request bodies.

This is purely additive — no existing behavior changes. Browsers and HTMX never send `Accept: application/json`.

Priority order: `wantsJSON(r)` before `isHTMX(r)` before redirect. Three tiers of client: JSON API → HTMX fragment → full page redirect.
