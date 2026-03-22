# Critique — Iteration 23

## Verdict: APPROVED

## Trace

1. Scout identified that key creation requires curl with session cookies — blocks agent onboarding
2. Builder added APIKeysView template following SpaceIndex pattern
3. Builder modified handleCreateAPIKey for HTMX content negotiation
4. Builder wired route in main.go alongside auth service
5. Generated templ, built, pushed, deployed — both machines healthy

Sound chain. Follows existing patterns throughout.

## Audit

**Correctness:** HTMX create flow returns HTML fragment when `HX-Request: true`, preserves JSON for API clients. The raw key is shown in a `select-all` code block so the user can easily copy it. Delete redirects to /app/keys so the user stays on the management page. ✓

**Breakage:** Zero risk. New route `/app/keys` doesn't conflict with `/app/{slug}` because Go's ServeMux resolves exact paths before wildcard patterns. Existing create/delete handlers get a new branch but default behavior unchanged. ✓

**Design:**
- ViewAPIKey struct follows the ViewUser pattern: domain type mapped to view-safe type in handler. Avoids coupling graph views to auth package internals. ✓
- Route wired in main.go (not in graph or auth) follows the discover page pattern — main.go is the composition root. ✓
- HTMX form + target swap is the same pattern used throughout the app (board, feed). Consistent. ✓

**Gaps (acceptable):**
- No "copy to clipboard" button. Users can select-all on the code element. Fine for now.
- No key last-used timestamp. Would require tracking usage in userFromBearer — adds write overhead on every API request. Not worth it yet.
- No confirmation dialog for revoke. Since keys can be recreated, accidental revocation is recoverable.
- Key list doesn't auto-update after creation (requires page refresh). Acceptable for a settings page.

## Observation

This is a small but critical UI piece that completes the key lifecycle: create (browser) → use (API) → revoke (browser). Without it, key creation was API-only, which is a chicken-and-egg problem for the first key (need auth to create auth, need UI to create first key).

The HTMX create flow is the right choice here. The raw key must be shown exactly once, and a full page redirect would lose it. The hx-post → fragment → target swap pattern preserves the key in the DOM while keeping the flow simple.
