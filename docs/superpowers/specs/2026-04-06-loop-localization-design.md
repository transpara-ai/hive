# Loop Localization — Design Specification

**Version:** 1.1.0
**Date:** 2026-04-06
**Status:** Approved
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-06 | Initial design with hardcoded transpara-ai paths |
| 1.1.0 | 2026-04-06 | Parameterized via loop/config.env; added site repo; fixed Option C reference |

---

## Problem

The hive's `run.sh` loop was built for the original developer's machine (`/c/src/matt/lovyou3/`) and assumes:

1. Direct access to `lovyou.ai` for posting iteration summaries
2. Fly.io deployment (`fly status`, `./ship.sh`)
3. Push to `origin main` (lovyou-ai upstream repo)
4. The `site` repo as the primary work target

These are hardcoded values. Replacing them with different hardcoded values (e.g., transpara-ai paths) repeats the same mistake. The fix is to parameterize the deployment so any operator can configure the loop for their environment.

## Design Principle

**Declarative config, not hardcoded paths.** All environment-specific values live in `loop/config.env`. Prompts and scripts read from config. The config file ships with sensible defaults and is overridden per-deployment.

## Constraints (Non-Negotiable for transpara-ai deployment)

These are hard rules from CLAUDE.md and the operator. They are enforced by the config values, not by hardcoding:

1. **Never push to upstream** — the `GIT_REMOTE` config determines push target, never `origin` unless configured.
2. **Never commit to `main`** — always use a `feat/` branch.
3. **Never push without being asked** — close.sh commits and stops.
4. **Never post to external APIs** — controlled by `POST_ENABLED` config.
5. **Never deploy** — controlled by `DEPLOY_ENABLED` config.
6. **Always create a PR** — PRs are the integration mechanism.

## Scope

### In Scope

| File | Change |
|------|--------|
| `loop/config.env` | **New file.** Declarative config for all environment-specific values |
| `loop/scout-prompt.txt` | Read repo paths from config, remove hardcoded paths and external service checks |
| `loop/builder-prompt.txt` | Read repo paths from config, remove deploy step |
| `loop/reflector-prompt.txt` | Remove close.sh lovyou.ai references, reference config |
| `loop/run.sh` | Source config.env, remove lovyou.ai post block, update comment paths |
| `loop/close.sh` | Rewrite: source config.env, use parameterized remote/org, safety gates |
| `loop/state.md` | Reset focus to building missing hive agents (primary section at line 7; archived section at ~line 774 is historical and left intact) |
| `CLAUDE.md` | Add Local Loop Guardrails section |

### Out of Scope

- `loop/critic-prompt.txt` — no external references, no changes needed
- Reviewer agent — separate design/PR (follows this work)
- Pipeline mode (`--pipeline`) — different system, not affected
- Legacy runtime mode (`--human --idea`) — not affected
- `cmd/post/` — not present in this repo; the `run.sh` post block that referenced it is being removed

## Changes

### 0. run.sh — Source Config, Remove Post Block

Source config.env at the top. Remove the lovyou.ai post block (lines 80-86). Replace with config-driven post. Update comment path on line 12.

### 1. scout-prompt.txt

Replace hardcoded `/c/src/matt/lovyou3/` path and external service checks with config.env references. Include all five repos: hive, eventgraph, agent, work, site.

### 2. builder-prompt.txt

Replace hardcoded deploy/ship.sh instruction with config-driven test-and-commit instruction.

### 3. reflector-prompt.txt

Replace lovyou.ai close.sh instruction with config-driven local commit instruction.

### 4. close.sh — Full Rewrite

Source config.env. Parameterize remote, org, repo name, protected branches. Add safety gates for protected branches and remote verification. Commit locally, print push/PR instructions using config values. Never push.

### 5. state.md — Focus Reset

Replace focus section to target hive repo and building missing agents. Reference Reviewer design spec with Option C.

### 6. CLAUDE.md — Local Loop Guardrails

Add section documenting that loop is configured via config.env, with defaults for transpara-ai deployment.

## Testing

1. `shellcheck loop/close.sh loop/run.sh`
2. `source loop/config.env` loads correct values
3. `close.sh` refuses to run on `main`
4. `close.sh` commits but does not push
5. No banned patterns in active loop files
6. `go build ./...` and `go test ./...` pass
