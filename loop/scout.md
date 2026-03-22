# Scout Report — Iteration 23

## What I Found

API key auth (iter 21) and JSON API (iter 22) are deployed, but there's no way to create an API key from the browser. The `POST /auth/api-keys` endpoint requires session auth, and there's no UI pointing to it. Matt would have to craft curl commands with session cookies to generate a key — possible but clunky. This blocks the first real agent interaction.

## What I Recommend

Build an API key management page at `/app/keys`:
- List existing keys (name, created date)
- Create form with HTMX (show raw key exactly once)
- Revoke button per key
- Usage instructions

This is the last prerequisite before the first agent interaction. Once Matt can generate a key from the browser, agents can use it programmatically.
