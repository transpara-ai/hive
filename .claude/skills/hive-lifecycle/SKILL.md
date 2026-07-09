---
name: hive-lifecycle
description: "Manage the transpara-ai hive stack lifecycle on nucbuntu — the systemd --user services (work-server, hive-ops-api), the Dockerized Postgres, and the on-demand civilization runtime. Use whenever the user says 'hive up', 'hive down', 'hive status', 'hive restart', 'start/stop/restart the hive', 'run the hive', 'run agents on', 'hive idea', 'is the hive running', 'kill the hive', 'bounce the hive', or mentions stuck agents, a frozen hive, or needing a clean slate."
---

# Hive Lifecycle Management

Start, stop, restart, and inspect the transpara-ai hive stack on **nucbuntu**. The APIs run as **systemd `--user` services** over a Dockerized Postgres; the multi-agent runtime is started on demand.

> On-prem / private (no public URLs). Intelligence runs through the Claude CLI (Max plan) — no API keys.

## Stack Components

| Component | Unit / process | Bind | Notes |
|---|---|---|---|
| **Postgres** | docker container `hive-postgres-1` | `:5432` | `docker compose up -d postgres` (compose file in `repos/hive`); DSN `postgres://hive:hive@localhost:5432/hive` |
| **work-server** | `work-server.service` (enabled) | `:8080` | builds + runs `repos/work/work-server`; serves the telemetry API, task API, and dashboard |
| **hive-ops-api** | `hive-ops-api.service` (enabled) | `127.0.0.1:8085` | runs `repos/hive/hive-ops-api`; serves the operator projection API; loads a model catalog |
| **hive runtime** | `hive.service` (**disabled** — on-demand) | — | `hive civilization daemon …`; the multi-agent civilization loop |

Paths: `HIVE_REPO=/Transpara/transpara-ai/repos/hive`, `WORK_REPO=/Transpara/transpara-ai/repos/work`.

## Authentication

Intelligence runs through the **Claude CLI** (Max plan) via `~/.claude/.credentials.json`. **No Anthropic API key is used.**

**CRITICAL — never set these:** `ANTHROPIC_API_KEY`, `HIVE_ANTHROPIC_API_KEY`. They override the working CLI auth and cause "Invalid API key" on every agent. Verify clean:

```bash
env | grep -i anthropic   # must print nothing
```

API bearer tokens (for the endpoints below):
- **hive-ops-api**: `HIVE_OPS_API_KEY` (default `dev`).
- **work-server**: `WORK_API_KEY` (from `~/.config/hive/hive.env`).

## Hive Up

```bash
# 1. Postgres (Docker) — everything else depends on it
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'   # expect "Up"

# 2. The API services (systemd --user)
systemctl --user start work-server hive-ops-api
systemctl --user is-active work-server hive-ops-api   # expect: active / active

# 3. (Optional) the multi-agent runtime — on demand, foreground for visibility
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/hive civilization daemon \
    --human Michael --approve-requests --approve-roles \
    --catalog ./catalog-mixed.yaml --catalog-reload-interval 1m \
    --store postgres://hive:hive@localhost:5432/hive
#   …or run the packaged unit:  systemctl --user start hive
```

## Hive Down

```bash
# Stop the runtime first, then the API services.
systemctl --user stop hive 2>/dev/null              # if started as the unit
pkill -INT -f 'cmd/hive civilization' 2>/dev/null    # if started via go run (graceful SIGINT)
systemctl --user stop hive-ops-api work-server
# Postgres usually stays up (data persists). Full stop:
#   cd /Transpara/transpara-ai/repos/hive && docker compose down
```

## Hive Restart

```bash
systemctl --user restart work-server hive-ops-api
systemctl --user restart hive 2>/dev/null   # only if you run the runtime as a unit
```

## Hive Status

```bash
echo "=== services ==="
systemctl --user is-active work-server hive-ops-api hive
systemctl --user --no-pager status work-server hive-ops-api | grep -E 'Active:|Main PID:'

echo "=== postgres ==="
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'
docker exec hive-postgres-1 psql -U hive -d hive -c 'SELECT count(*) FROM events' 2>/dev/null || echo "DB unreachable"

echo "=== endpoint health ==="
curl -s -o /dev/null -w 'work-server  /health           HTTP %{http_code}\n' http://localhost:8080/health
curl -s -o /dev/null -w 'hive-ops-api /health           HTTP %{http_code}\n' http://127.0.0.1:8085/health
curl -s -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' http://localhost:8080/telemetry/status
```

**Crash-loop diagnosis.** If `is-active` reports `activating (auto-restart)`, the service is crash-looping — almost always because **Postgres is down** (both `work-server` and `hive-ops-api` fail their DB connection on start). Confirm and fix:

```bash
journalctl --user -u hive-ops-api -n 20 --no-pager   # look for: dial tcp 127.0.0.1:5432: connect: connection refused
docker start hive-postgres-1                          # bring the DB back; the services self-heal on their next restart tick
```

## Endpoint Reference

### hive-ops-api — `http://127.0.0.1:8085` (Bearer `HIVE_OPS_API_KEY`, default `dev`)

| Method · Path | Purpose |
|---|---|
| `GET /health` | liveness (no auth) |
| `GET /api/hive/operator-projection` | pending approvals, authority decisions, lifecycle, key traces |
| `GET /api/hive/civilization/assembly-projection` | role/agent topology, org tiers, model selection |
| `POST /api/hive/operator-decision` | record a human authority decision (`approved` \| `denied`) |
| `POST /api/hive/runs` | launch an operator-initiated run |
| `POST /api/hive/model-selection/role-policy` | update a role's model policy |

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
- **hive-ops-api**: `HIVE_OPS_CATALOG` (service uses `repos/hive/catalog-mixed.yaml`), `HIVE_OPS_CATALOG_RELOAD_INTERVAL=1m`.
- **hive runtime / council**: `--catalog <path> --catalog-reload-interval 1m` (e.g. `council --catalog ./catalog-mixed.yaml`).

`catalog-mixed.yaml` is the catalog checked into `repos/hive`. (The `hive.service` unit references `catalog-codex.yaml`; if it is absent, point `--catalog` at `catalog-mixed.yaml`.)

## On-demand Runtime (`cmd/hive` verbs)

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/hive civilization run    --human Michael --idea "…"            # one-shot multi-agent
go run ./cmd/hive civilization daemon --human Michael --approve-requests --approve-roles \
       --store postgres://hive:hive@localhost:5432/hive                      # long-running
go run ./cmd/hive pipeline run        --human Michael --idea "…"            # Scout → Builder → Critic
go run ./cmd/hive role <name> run     --human Michael                       # single agent
go run ./cmd/hive council --topic "…" --catalog ./catalog-mixed.yaml        # one deliberation
go run ./cmd/hive ingest <file.md>                                          # post a spec as a task
```

Core flags: `--human` (required), `--idea` / `--spec` (seed), `--store <dsn>` (or `DATABASE_URL`), `--repo <path>`, `--approve-requests`, `--approve-roles`. Per-verb flags: `go run ./cmd/hive <verb> --help`.

## Operator Actions

- **Approve a proposed role** — via the API (preferred): `POST /api/hive/operator-decision`. Or the CLI: `go run ./cmd/approve-role --role <name> --store postgres://hive:hive@localhost:5432/hive`.
- **Inject a spec/file as a task**: `go run ./cmd/inject-file --title "…" --priority medium <file>`.

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
pkill -INT -f 'cmd/hive civilization' 2>/dev/null; systemctl --user stop hive 2>/dev/null; sleep 3
docker exec hive-postgres-1 psql -U hive -d hive -c "
    DELETE FROM telemetry_agent_snapshots;
    DELETE FROM telemetry_hive_snapshots;
    DELETE FROM telemetry_event_stream;"
```

## Nuclear Option (destroys the event chain)

**WARNING: erases all events / tasks / audit trail. Only for a corrupted chain.**

```bash
systemctl --user stop hive hive-ops-api work-server 2>/dev/null
cd /Transpara/transpara-ai/repos/hive
docker compose down -v && docker compose up -d postgres
```

## Common Problems

| Symptom | Cause | Fix |
|---|---|---|
| Service `activating (auto-restart)` | Postgres down — DB connection fails on start | `docker start hive-postgres-1`; services self-heal |
| `curl :8085` / `:8080` connection refused | that service not running / crash-looping | `systemctl --user status <svc>`; see crash-loop diagnosis |
| `401 unauthorized` from `:8085` | missing/wrong bearer | add `-H "Authorization: Bearer $HIVE_OPS_API_KEY"` (default `dev`) |
| "Invalid API key" on all agents | `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` set | `unset` them; remove from shell profile |
| All agents `$0.000` cost | runtime just started, no iterations yet | wait 2–3 min |
| Port `:8080` conflict on start | another process bound it | `lsof -i :8080`; stop the conflict |
