# Build Report — Iteration 24

## What I planned

Build the first agent interaction — a tool that makes the hive loop post its own iteration summaries to lovyou.ai.

## What I built

New file: `cmd/post/main.go` in the hive repo. Modified: `loop/run.sh`.

### cmd/post — iteration publisher

A standalone Go program that posts iteration summaries to lovyou.ai using the JSON API and Bearer token auth built in iterations 21-22.

**Flow:**
1. Check `LOVYOU_API_KEY` env var — skip gracefully if unset (exit 0)
2. Read `loop/state.md` — extract iteration number via regex
3. Read `loop/build.md` — the build report becomes the post body
4. `GET /app/hive` with `Accept: application/json` — check if hive space exists
5. If 404: `POST /app/new` with JSON body — create "hive" community space (public)
6. `POST /app/hive/op` with `op=express` — post the build report to the feed

**Configuration:**
- `LOVYOU_API_KEY` — required, the `lv_...` Bearer token
- `LOVYOU_BASE_URL` — optional, defaults to `https://lovyou.ai`

**Usage:**
```bash
cd /c/src/matt/lovyou3/hive
LOVYOU_API_KEY=lv_... go run ./cmd/post/
```

### run.sh integration

After all four phases complete (scout → builder → critic → reflector), run.sh now calls `go run ./cmd/post/`. If `LOVYOU_API_KEY` is not set, the tool prints "skipping post" and exits 0 — the loop doesn't break.

### Why Go, not bash/curl

- JSON escaping: build.md contains markdown with quotes, backticks, newlines. `json.Marshal` handles all of it. Bash string escaping would be fragile.
- Error handling: HTTP status checks, readable error messages.
- No dependencies: stdlib only (net/http, encoding/json, os, regexp).
- Consistent: hive repo is Go. This is a Go binary alongside cmd/hive.

## Verification

- `go build -o /tmp/post.exe ./cmd/post/` — success
- `/tmp/post.exe` without LOVYOU_API_KEY — prints "skipping" and exits 0
- Cannot test end-to-end yet (no API key generated) — requires Matt to log in and create a key at /app/keys
