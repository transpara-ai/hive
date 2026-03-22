# Build Report — Iteration 10

## What I planned

Add canonical host redirect from fly.dev to lovyou.ai.

## What I built

1. **canonicalHost middleware** — 301 redirects any non-lovyou.ai hostname to lovyou.ai. Preserves request path and query string. Skips localhost and 127.0.0.1 for local dev.

2. **Health check bypass** — first deploy failed because Fly's health check uses internal IP (not lovyou.ai). Added `/health` path exclusion. Second deploy succeeded.

3. Built, committed (2 commits: feature + fix), pushed, deployed. Verified: `curl -sI https://lovyou-ai.fly.dev/blog` returns `301 → https://lovyou.ai/blog`.

## Key finding

Infrastructure changes that interact with deploy tooling (health checks, DNS) need careful attention to side effects. The loop caught and fixed the health check issue within the same iteration.
