---
name: hive-lifecycle
description: "Manage the transpara-ai hive stack lifecycle on nucbuntu — the systemd --user services (work-server, hive-ops-api), the Dockerized Postgres, and the on-demand civilization runtime. Use whenever the user says 'hive up', 'hive down', 'hive status', 'hive restart', 'hive run', 'hive help', 'start/stop/restart the hive', 'run the hive', 'run agents on', 'hive idea', 'run pipeline', 'run builder', 'run scout', 'is the hive running', 'kill the hive', 'bounce the hive', or mentions stuck agents, a frozen hive, or needing a clean slate."
---

# Hive Lifecycle Management

Start, stop, restart, and inspect the transpara-ai hive stack on **nucbuntu**. The APIs run as **systemd `--user` services** over a Dockerized Postgres; the multi-agent runtime is started on demand.

> On-prem / private (no public URLs). The Claude CLI path needs no key; the default mixed catalog also routes some roles to OpenRouter — see **Authentication**.

## Hive Help

For **"hive help" / "hive commands"**: print the section list below — Stack Components, Authentication, Hive Up / Down / Restart / Status, Endpoint Reference, Model Catalog, On-demand Runtime, Operator Actions, Logs, Common Problems — and **stop** (do not start/stop/restart anything). For CLI usage: `go run ./cmd/hive --help` or `go run ./cmd/hive <verb> --help`.

## Stack Components

| Component | Unit / process | Bind | Notes |
|---|---|---|---|
| **Postgres** | docker container `hive-postgres-1` | `:5432` | `docker compose up -d postgres` (compose file in `repos/hive`); DSN `postgres://hive:hive@localhost:5432/hive` |
| **work-server** | `work-server.service` (enabled) | `:8080` | builds + runs `repos/work/work-server`; serves the telemetry API, task API, and dashboard |
| **hive-ops-api** | `hive-ops-api.service` (enabled) | `127.0.0.1:8085` | runs `repos/hive/hive-ops-api`; serves the operator projection API; loads a model catalog |
| **hive runtime** | `hive.service` (**disabled** — on-demand) | — | `hive civilization daemon …`; the multi-agent civilization loop |

Paths: `HIVE_REPO=/Transpara/transpara-ai/repos/hive`, `WORK_REPO=/Transpara/transpara-ai/repos/work`.

## Authentication

The **Claude CLI** path (Max plan, `~/.claude/.credentials.json`) needs **no Anthropic API key** — and setting one breaks it.

**CRITICAL — never set these:** `ANTHROPIC_API_KEY`, `HIVE_ANTHROPIC_API_KEY`. They override the working CLI auth and cause "Invalid API key" on every Claude agent. Verify clean:

```bash
env | grep -i anthropic   # must print nothing
```

⚠ **The default `catalog-mixed.yaml` is a MIXED-provider catalog** (Claude CLI + Codex CLI + Ollama + OpenRouter). Claude/Codex use `subscription` auth and Ollama is `local`, but the **OpenRouter** roles (e.g. `reviewer`, `strategist`) use `auth_mode: api-key` and require **`OPENROUTER_API_KEY`**. For a Claude-CLI-only run with no provider keys, **omit `--catalog`** — the runtime falls back to its built-in Claude defaults (there is no checked-in Claude-only catalog).

API bearer tokens (for the endpoints below):
- **hive-ops-api**: `HIVE_OPS_API_KEY` (default `dev`).
- **work-server**: `WORK_API_KEY` (lives in `~/.config/hive/hive.env`, not exported to your shell).

To run the `curl` examples below from a plain shell, load the tokens first:

```bash
set -a; . /home/transpara/.config/hive/hive.env 2>/dev/null; set +a   # populates WORK_API_KEY (HIVE_OPS_API_KEY defaults to `dev`)
```

## Hive Up

```bash
# 1. Postgres (Docker) — everything else depends on it
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'   # expect "Up"

# 2. The API services (systemd --user). Dirty start? Run "Hive Down" first to clear
#    any stale manual go-run process still holding :8080/:8085.
systemctl --user start work-server hive-ops-api
systemctl --user is-active work-server hive-ops-api   # expect: active / active

# 3. (Optional) the multi-agent runtime — on demand, foreground for visibility.
#    DEFAULT is human-in-the-loop: authority requests + role proposals BLOCK for approval.
#    catalog-mixed.yaml routes reviewer/strategist to OpenRouter → needs OPENROUTER_API_KEY
#    This omits --catalog → built-in CLAUDE-only defaults (Max plan, no provider keys).
#    catalog-mixed.yaml (what the packaged hive.service uses) routes some roles to Ollama +
#    OpenRouter; add `--catalog ./catalog-mixed.yaml --catalog-reload-interval 1m` ONLY if a
#    local Ollama model is running AND OPENROUTER_API_KEY is set.
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/hive civilization daemon \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive
#   Full autonomy (auto-approve everything) is an EXPLICIT opt-in: add
#       --approve-requests --approve-roles
#   The packaged unit `systemctl --user start hive` runs in FULL-AUTONOMY mode —
#   its ExecStart already includes --approve-requests --approve-roles.
```

## Hive Down

```bash
# Stop the runtime first, then the API services.
systemctl --user stop hive 2>/dev/null                    # if started as the unit
pkill -INT -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null             # graceful; matches both `go run` and its compiled child
sleep 3; pkill -KILL -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null    # sweep any survivor
systemctl --user stop hive-ops-api work-server
# Sweep stray MANUAL runtimes from the old non-systemd flow that may hold the ports —
# fuser -k kills whatever holds the TCP port (the go wrapper AND its compiled child):
for p in 8080 8081 8085; do fuser -k -n tcp "$p" 2>/dev/null; done
lsof -i :8080 -i :8081 -i :8085 2>/dev/null || echo "ports clear"
# Postgres usually stays up (data persists). Full stop:
#   cd /Transpara/transpara-ai/repos/hive && docker compose down
```

## Hive Restart

```bash
systemctl --user restart work-server hive-ops-api
# The hive runtime is on-demand (unit disabled). `restart` would START the disabled
# unit and launch the daemon unexpectedly — so bounce it ONLY if already running.
# Explicit if/else so a real restart FAILURE surfaces (not masked as "not running"):
if systemctl --user is-active --quiet hive; then
  systemctl --user restart hive
elif pgrep -f 'hive (civilization|pipeline|role|council|factory)' >/dev/null; then
  echo "bouncing the MANUAL (go run) runtime → background (logs: /tmp/hive.log); relaunches the civilization daemon"
  pkill -INT -f 'hive (civilization|pipeline|role|council|factory)'; sleep 3
  pkill -KILL -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null
  ( cd /Transpara/transpara-ai/repos/hive && nohup go run ./cmd/hive civilization daemon \
      --human Michael --store postgres://hive:hive@localhost:5432/hive > /tmp/hive.log 2>&1 & )
  echo "restarted in the background (if you were running a different verb, relaunch that one instead)"
else
  echo "hive runtime not running (on-demand) — left stopped"
fi
```

## Hive Status

```bash
set -a; . /home/transpara/.config/hive/hive.env 2>/dev/null; set +a   # load WORK_API_KEY for the telemetry probe
echo "=== services ==="
systemctl --user is-active work-server hive-ops-api
systemctl --user --no-pager status work-server hive-ops-api | grep -E 'Active:|Main PID:'

echo "=== hive runtime (systemd unit OR manual go-run) ==="
if systemctl --user is-active --quiet hive; then echo "hive.service: active"
elif pgrep -f 'hive (civilization|pipeline|role|council|factory)' >/dev/null; then echo "manual runtime: RUNNING"
else echo "runtime: stopped"; fi
ss -tlnp 2>/dev/null | grep -q ':8081 ' && echo "hive webhook :8081: listening" || echo "hive webhook :8081: not listening"

echo "=== postgres ==="
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'
docker exec hive-postgres-1 psql -U hive -d hive -c 'SELECT count(*) FROM events' 2>/dev/null || echo "DB unreachable"

echo "=== endpoint health ==="
curl -s -o /dev/null -w 'work-server  /health           HTTP %{http_code}\n' http://localhost:8080/health
curl -s -o /dev/null -w 'hive-ops-api /health           HTTP %{http_code}\n' http://127.0.0.1:8085/health
curl -s -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/status
```

**Crash-loop diagnosis.** If `is-active` reports `activating (auto-restart)`, the service is crash-looping — almost always because **Postgres is down** (both `work-server` and `hive-ops-api` fail their DB connection on start). Confirm and fix:

```bash
journalctl --user -u hive-ops-api -n 20 --no-pager   # look for: dial tcp 127.0.0.1:5432: connect: connection refused
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres   # bring the DB back (recreates it if it was `compose down`-ed); services self-heal on their next restart tick
```

## Endpoint Reference

### hive-ops-api — `http://127.0.0.1:8085` (Bearer `HIVE_OPS_API_KEY`, default `dev`)

| Method · Path | Purpose |
|---|---|
| `GET /health` | liveness (no auth) |
| `GET /api/hive/operator-projection` | pending approvals, authority decisions, lifecycle, key traces |
| `GET /api/hive/civilization/assembly-projection` | role/agent topology, org tiers, model selection |
| `POST /api/hive/operator-decision` † | decide a **draft-PR-create** authority request (`approved` \| `denied`) — other request types are rejected |
| `POST /api/hive/runs` † | launch an operator-initiated run |
| `POST /api/hive/model-selection/role-policy` † | update a role's model policy |

> **†** Write routes exist **only in writer mode** — when `hive-ops-api` is started with `HIVE_OPS_HUMAN_ACTOR` set. The default `hive-ops-api.service` does **not** set it, so the deployed API is **read-only** (GET routes only).

```bash
curl -s -H "Authorization: Bearer ${HIVE_OPS_API_KEY:-dev}" \
     http://127.0.0.1:8085/api/hive/operator-projection | jq .
```

### work-server — `http://localhost:8080` (Bearer `WORK_API_KEY`)

| Path | Purpose |
|---|---|
| `GET /health` | liveness (no auth) |
| `GET /telemetry/status` | full snapshot: agents + hive health + phases + recent events |
| `GET /telemetry/health` · `GET /telemetry/overview` | hive health / architecture overview |
| `GET /telemetry/agents` · `/agents/{role}` · `/agents/history` | agent snapshots + 24h history |
| `GET /telemetry/roles` · `/roles/{name}` · `/actors` · `/layers` | roster / actors / layer view |
| `GET /telemetry/phases` · `POST /telemetry/phases/{phase}` | phase status / update |
| `GET /telemetry/pipeline/report` | compact pipeline status line + phase rows |
| `GET /telemetry/stream` · `GET /telemetry/sse` | recent events (JSON) / live event stream |
| `GET\|POST /tasks`, `/tasks/{id}/…`, `/phase-gates` | Work task + phase-gate API |

```bash
curl -s -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/status | jq .
```

## Model Catalog

The operator API and the runtime select models from a catalog YAML (hot-reloaded):
- **hive-ops-api**: `HIVE_OPS_CATALOG` — the unit **resolves to `repos/hive/catalog-mixed.yaml`** (confirm with `systemctl --user show hive-ops-api -p Environment`), `HIVE_OPS_CATALOG_RELOAD_INTERVAL=1m`.
- **hive runtime (daemon)**: `--catalog <path> --catalog-reload-interval 1m`. **`council`** takes `--catalog <path>` only — **no** `--catalog-reload-interval` (e.g. `council --catalog ./catalog-mixed.yaml`).

Only `catalog-mixed.yaml` is checked into `repos/hive` (a missing, uncommitted `catalog-codex.yaml` referenced by `hive.service`'s `ExecStart` is the likely cause if that unit fails to start). For a manual Claude-only run, **omit `--catalog`** (built-in Claude defaults); add `--catalog ./catalog-mixed.yaml` **only** when a local Ollama model is running and `OPENROUTER_API_KEY` is set.

## On-demand Runtime (`cmd/hive` verbs)

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/hive civilization run    --human Michael --idea "…" \
       --store postgres://hive:hive@localhost:5432/hive                      # one-shot multi-agent (--store required to persist; omit only for a throwaway in-memory run)
go run ./cmd/hive civilization daemon --human Michael \
       --store postgres://hive:hive@localhost:5432/hive                      # long-running (add --approve-requests --approve-roles for full autonomy)
go run ./cmd/hive pipeline run        --api http://localhost:8082 --repo .   # Scout → Builder → Critic (needs the local API up — see "Local / Offline"; no --idea)
go run ./cmd/hive role <name> run     --api http://localhost:8082 --repo .   # single agent
go run ./cmd/hive council --topic "…" --catalog ./catalog-mixed.yaml        # one deliberation
go run ./cmd/hive ingest --priority normal <file.md>   # registered-repo API flow: needs LOVYOU_API_KEY (set HIVE_INGEST_SKIP_REPO=1 to skip the repo bootstrap) — check `--help` before use
```

Flags are **per-verb**. `civilization run`: `--human` (required), `--idea`/`--spec` (seed), `--store` (or `DATABASE_URL`), `--repo`, `--catalog`, `--approve-requests`, `--approve-roles`. `civilization daemon`: the same **except** its seed flag is `--seed-spec` (there is no `--idea`/`--spec`). `pipeline`/`role`: `--api`, `--space`, `--repo`, `--agent-id` (no `--human`/`--idea`; for the local stack pass `--api http://localhost:8082`). Always confirm with `go run ./cmd/hive <verb> --help`.

## Operator Actions

- **Approve a proposed role** — use the CLI (it emits the `hive.role.approved` + budget events the runtime needs): `go run ./cmd/approve-role --role <name> --store postgres://hive:hive@localhost:5432/hive`. `POST /api/hive/operator-decision` only decides **draft-PR-create** authority requests — not role proposals.
- **Inject a spec/file as a task** — posts to the civilization daemon **webhook** (`:8081/event`), so the runtime must be running: `go run ./cmd/inject-file --title "…" --priority medium <file>`.

## Local / Offline

Run the pipeline against a local API with no external dependency:

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/localapi --addr :8082 --api-key dev   # add --site-db to read the site DB instead of hive
```

## Logs

```bash
journalctl --user -u work-server  -f
journalctl --user -u hive-ops-api -f
journalctl --user -u hive -f            # if the runtime runs as the unit
cd /Transpara/transpara-ai/repos/hive && docker compose logs -f postgres
```

## Clean Slate (nuke telemetry, keep the chain)

```bash
pkill -INT -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null; systemctl --user stop hive 2>/dev/null; sleep 3; pkill -KILL -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null
docker exec hive-postgres-1 psql -U hive -d hive -c "
    DELETE FROM telemetry_agent_snapshots;
    DELETE FROM telemetry_hive_snapshots;
    DELETE FROM telemetry_event_stream;"
```

## Nuclear Option (destroys the event chain)

**WARNING: erases all events / tasks / audit trail. Only for a corrupted chain.**

```bash
pkill -INT -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null; sleep 3; pkill -KILL -f 'hive (civilization|pipeline|role|council|factory)' 2>/dev/null   # kill any manual go-run runtime first
systemctl --user stop hive hive-ops-api work-server 2>/dev/null
cd /Transpara/transpara-ai/repos/hive
docker compose down -v && docker compose up -d postgres
```

## Common Problems

| Symptom | Cause | Fix |
|---|---|---|
| Service `activating (auto-restart)` | Postgres down — DB connection fails on start | `docker compose up -d postgres` (from `repos/hive`); services self-heal |
| `curl :8085` / `:8080` connection refused | that service not running / crash-looping | `systemctl --user status <svc>`; see crash-loop diagnosis |
| `401 unauthorized` from `:8085` | missing/wrong bearer | add `-H "Authorization: Bearer $HIVE_OPS_API_KEY"` (default `dev`) |
| "Invalid API key" on all agents | `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` set | `unset` them; remove from shell profile |
| All agents `$0.000` cost | runtime just started, no iterations yet | wait 2–3 min |
| Port `:8080` conflict on start | another process bound it | `lsof -i :8080`; stop the conflict |
