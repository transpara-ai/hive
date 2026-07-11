---
name: hive-lifecycle
description: "Manage the local transpara-ai Hive stack lifecycle on nucbuntu: status, help, safe startup/shutdown/restart of Docker Postgres and systemd --user services, read-only telemetry probes, on-demand Hive runtime commands, logs, and clean-slate diagnostics. Use when the user says hive up, hive down, hive status, hive restart, hive help, start/stop/restart/run the hive, run agents, run a hive pipeline, run builder/scout/council, asks whether Hive is running, reports stuck agents or crash-looping Hive services, or needs a clean local Hive slate."
---

# Hive Lifecycle Management

Use this skill to inspect and operate the local Hive stack on nucbuntu. The stack is private/on-prem only. Do not assume any public URL or public proof surface.

Default to read-only inspection. Start, stop, restart, clean slate, runtime launch, POST routes, or destructive database actions require explicit user intent in the current turn.

## Safety Rules

- For `hive help` or `hive commands`, print the section list in this skill and stop. Do not run status, start, stop, or restart commands.
- For `hive status`, run only read-only probes.
- For `hive up`, `hive down`, `hive restart`, `run the hive`, or `clean slate`, perform only the named operation and state what was changed.
- Treat the runtime as on-demand. Do not start `hive.service` unless the user explicitly asks for the runtime or daemon.
- Do not set `ANTHROPIC_API_KEY` or `HIVE_ANTHROPIC_API_KEY`; those break the Claude CLI subscription path.
- Do not use public-internet exposure, deploy, service restart, runtime execution, Hive wake/start/action APIs, production EventGraph writes, protected settings changes, Test 001 GREEN, autonomy increase, value allocation, or wiki work unless separately authorized.
- Prefer hostnames and `localhost`; do not introduce literal private network addresses in work products.

## Stack Components

| Component | Unit or process | Bind | Notes |
|---|---|---|---|
| Postgres | Docker container `hive-postgres-1` | `localhost:5432` | Compose file in `/Transpara/transpara-ai/repos/hive`; DSN `postgres://hive:hive@localhost:5432/hive` |
| work-server | `work-server.service` | `localhost:8080` | Built from `/Transpara/transpara-ai/repos/work/work-server`; telemetry, task API, dashboard |
| hive-ops-api | `hive-ops-api.service` | loopback `:8085` | Operator projection API; normally read-only |
| hive runtime | `hive.service` or manual `go run` | webhook `:8081` when running | On-demand multi-agent loop; unit may be disabled |

Use these canonical local paths:

```bash
HIVE_REPO=/Transpara/transpara-ai/repos/hive
WORK_REPO=/Transpara/transpara-ai/repos/work
```

## Authentication

Claude CLI subscription auth uses `~/.claude/.credentials.json` and needs no Anthropic API key. Verify the shell is clean before runtime operations:

```bash
env | cut -d= -f1 | grep -i anthropic
```

Check names only — never print environment values into the transcript. The command should print nothing. If it prints `ANTHROPIC_API_KEY` or `HIVE_ANTHROPIC_API_KEY`, stop and ask the user before changing shell profile files. For the current shell only, unset them before runtime commands:

```bash
unset ANTHROPIC_API_KEY HIVE_ANTHROPIC_API_KEY
```

The checked-in `catalog-mixed.yaml` is a mixed-provider catalog. Claude/Codex use subscription auth and Ollama is local, but OpenRouter roles require `OPENROUTER_API_KEY`. For a Claude-CLI-only run with no provider keys, omit `--catalog` so Hive uses built-in Claude defaults.

API tokens:

- `HIVE_OPS_API_KEY`, default `dev`, for `hive-ops-api`.
- `WORK_API_KEY`, usually in `/home/transpara/.config/hive/hive.env`, for work-server telemetry routes.

Load tokens for shell probes:

```bash
set -a
. /home/transpara/.config/hive/hive.env 2>/dev/null || true
set +a
```

## Hive Help

For help requests, summarize these sections and stop:

- Stack Components
- Authentication
- Hive Up
- Hive Down
- Hive Restart
- Hive Status
- Endpoint Reference
- Model Catalog
- On-demand Runtime
- Operator Actions
- Logs
- Clean Slate
- Common Problems

For CLI help, use:

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/hive --help
go run ./cmd/hive <verb> --help
```

## Hive Status

Use this for read-only status checks:

```bash
set -a
. /home/transpara/.config/hive/hive.env 2>/dev/null || true
set +a

echo "=== services ==="
systemctl --user is-active work-server hive-ops-api
systemctl --user --no-pager status work-server hive-ops-api | grep -E 'Active:|Main PID:'

echo "=== hive runtime ==="
if systemctl --user is-active --quiet hive; then
  echo "hive.service: active"
elif pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' >/dev/null; then
  echo "manual runtime: RUNNING"
  pgrep -af '[h]ive (--human|civilization|pipeline|role|council|factory)'
else
  echo "runtime: stopped"
fi

ss -tlnp 2>/dev/null | grep -q ':8081 ' && echo "hive webhook :8081: listening" || echo "hive webhook :8081: not listening"

echo "=== postgres ==="
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'
docker exec hive-postgres-1 psql -U hive -d hive -c 'SELECT count(*) FROM events' 2>/dev/null || echo "DB unreachable"

echo "=== endpoint health ==="
curl -s --connect-timeout 3 --max-time 10 -o /dev/null -w 'work-server  /health           HTTP %{http_code}\n' http://localhost:8080/health
curl -s --connect-timeout 3 --max-time 10 -o /dev/null -w 'hive-ops-api /health           HTTP %{http_code}\n' http://localhost:8085/health
curl -s --connect-timeout 3 --max-time 10 -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' -H "Authorization: Bearer ${WORK_API_KEY:-}" http://localhost:8080/telemetry/status
```

If `systemctl --user is-active` reports `activating` or `auto-restart`, inspect logs read-only first:

```bash
journalctl --user -u hive-ops-api -n 40 --no-pager
```

Starting Postgres is a mutating recovery action, not part of status — run it only after the user explicitly confirms:

```bash
cd /Transpara/transpara-ai/repos/hive
docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done
```

## Hive Up

Start only when the user explicitly asks to bring Hive up. Start Postgres first and wait until it accepts connections:

```bash
cd /Transpara/transpara-ai/repos/hive
docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do
  tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }
  echo "waiting for postgres..."
  sleep 1
done
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  echo "postgres ready"
  systemctl --user start work-server hive-ops-api
  systemctl --user is-active work-server hive-ops-api
else
  echo "postgres not ready — NOT starting services; fix postgres first"
fi
```

Do not start the multi-agent runtime as part of ordinary `hive up` unless the user explicitly asks for runtime/daemon execution.

## Hive Down

Stop the runtime first, then API services. Do not stop Postgres unless the user asks for full compose down.

```bash
systemctl --user stop hive 2>/dev/null || true
pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true
sleep 3
pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true

systemctl --user stop hive-ops-api work-server

pkill -f '[c]md/work-server|[e]xe/work-server' 2>/dev/null || true
pkill -f '[c]md/hive-ops-api|[e]xe/hive-ops-api' 2>/dev/null || true

lsof -i :8080 -i :8081 -i :8085 2>/dev/null && echo "^ a port is still bound; inspect before clearing it" || echo "ports clear"
```

Full Postgres stop, only when explicitly requested:

```bash
cd /Transpara/transpara-ai/repos/hive
docker compose down
```

## Hive Restart

Restart means true down-to-up for APIs after Postgres is available. Preserve manual runtime command flags instead of guessing them.

```bash
cd /Transpara/transpara-ai/repos/hive
docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done

# Everything below is gated on Postgres readiness — restarting services or the
# runtime against a dead DB only produces crash loops; on timeout, stop here.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  systemctl --user restart work-server hive-ops-api
  if systemctl --user is-active --quiet hive; then
    systemctl --user restart hive
  elif pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' >/dev/null; then
    echo "manual runtime detected; stopping it and preserving command for the user to rerun:"
    pgrep -af '[h]ive (--human|civilization|pipeline|role|council|factory)'
    pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)'
    sleep 3
    pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true
  else
    echo "hive runtime not running; left stopped"
  fi
else
  echo "postgres not ready — NOT restarting services or runtime; fix postgres first"
fi
```

## Endpoint Reference

`hive-ops-api` on `http://localhost:8085`, bearer `${HIVE_OPS_API_KEY:-dev}`:

| Method and path | Purpose |
|---|---|
| `GET /health` | liveness, no auth |
| `GET /api/hive/operator-projection` | pending approvals, authority decisions, lifecycle, traces |
| `GET /api/hive/civilization/assembly-projection` | role/agent topology, org tiers, model selection |
| `POST /api/hive/operator-decision` | writer-mode only; decide draft-PR-create authority requests |
| `POST /api/hive/runs` | writer-mode only; launch operator-initiated run |
| `POST /api/hive/model-selection/role-policy` | writer-mode only; update role model policy |

Default `hive-ops-api.service` should be read-only because it does not set `HIVE_OPS_HUMAN_ACTOR`. Confirm before POSTing — check variable names only, never dump values (the Environment line carries the ops API key and DSN):

```bash
systemctl --user show hive-ops-api -p Environment | grep -o '[A-Z0-9_]\+=' | tr -d '=' | grep -v '^Environment$'
```

`HIVE_OPS_HUMAN_ACTOR` absent from that name list means read-only mode.

`work-server` on `http://localhost:8080`, bearer `$WORK_API_KEY`:

| Path | Purpose |
|---|---|
| `GET /health` | liveness, no auth |
| `GET /telemetry/status` | agents, hive health, phases, recent events |
| `GET /telemetry/health`, `/telemetry/overview` | health and architecture |
| `GET /telemetry/agents`, `/telemetry/roles`, `/telemetry/phases` | telemetry slices |
| `GET /telemetry/pipeline/report` | compact pipeline status |
| `GET /telemetry/stream`, `/telemetry/sse` | recent/live events |
| `GET/POST /tasks`, `/phase-gates` | Work task and phase-gate API |

Read-only probes:

```bash
curl -s --connect-timeout 3 --max-time 10 -H "Authorization: Bearer ${HIVE_OPS_API_KEY:-dev}" http://localhost:8085/api/hive/operator-projection | jq .
curl -s --connect-timeout 3 --max-time 10 -H "Authorization: Bearer ${WORK_API_KEY:-}" http://localhost:8080/telemetry/status | jq .
```

## Model Catalog

- `hive-ops-api` uses `HIVE_OPS_CATALOG`; confirm the name is present with `systemctl --user show hive-ops-api -p Environment | grep -o '[A-Z0-9_]\+=' | tr -d '=' | grep -v '^Environment$'` (names only — never print values).
- Hive runtime uses `--catalog <path> --catalog-reload-interval 1m`.
- `council` accepts `--catalog <path>` only; do not pass `--catalog-reload-interval` to `council`.
- If avoiding provider keys, omit `--catalog` for built-in Claude defaults.

## On-demand Runtime

Runtime execution is not a status check. Run only after explicit user request.

```bash
cd /Transpara/transpara-ai/repos/hive

go run ./cmd/hive civilization run \
  --human Michael \
  --idea "..." \
  --store postgres://hive:hive@localhost:5432/hive

go run ./cmd/hive civilization daemon \
  --human Michael \
  --store postgres://hive:hive@localhost:5432/hive

go run ./cmd/hive pipeline run --api http://localhost:8082 --repo .
go run ./cmd/hive role <name> run --api http://localhost:8082 --repo .
LOVYOU_API_KEY=dev go run ./cmd/hive council --api http://localhost:8082 --topic "..."
```

Warning: `council` defaults `--api` to `https://transpara.ai` and, when `LOVYOU_API_KEY` is set, posts up to 2000 characters of the deliberation report to the remote social feed — and interpolates that key into every council agent's prompt, exposing the bearer token to model providers. For local runs, always pin `--api` to the local endpoint and blank `LOVYOU_API_KEY` as shown; remote publishing requires the user's explicit authorization in the current turn.

Full autonomy is an explicit opt-in with `--approve-requests --approve-roles`. Do not add those flags unless the user explicitly authorizes that mode in the current turn.

Flag reminders:

- `civilization run`: `--human`, `--idea` or `--spec`, `--store`, `--repo`, `--catalog`, `--approve-requests`, `--approve-roles`.
- `civilization daemon`: same except the seed flag is `--seed-spec`; there is no `--idea` or `--spec`.
- `pipeline` and `role`: `--api`, `--space`, `--repo`, `--agent-id`; no `--human` or `--idea`.
- Always confirm with `go run ./cmd/hive <verb> --help` when composing a new invocation.

## Operator Actions

Approving proposed roles should use the CLI because it emits the role approval and budget events the runtime needs. Warning: this CLI also allocates budget — it emits `agent.budget.adjusted` with an initial budget of 200 for the new role (`cmd/approve-role/main.go`). Disclose that amount and obtain the user's explicit approval for both the role and the initial budget before running it:

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/approve-role --role <name> --store postgres://hive:hive@localhost:5432/hive
```

Injecting a spec/file requires the runtime webhook to be running:

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/inject-file --title "..." --priority medium <file>
```

## Local Offline API

For local pipeline/role commands with no external dependency:

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/localapi --addr :8082 --api-key dev
```

## Logs

```bash
journalctl --user -u work-server -f
journalctl --user -u hive-ops-api -f
journalctl --user -u hive -f
cd /Transpara/transpara-ai/repos/hive && docker compose logs -f postgres
```

## Clean Slate

This removes telemetry snapshots but keeps the event chain. Use only when explicitly requested.

```bash
pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true
systemctl --user stop hive 2>/dev/null || true
sleep 3
pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true

docker exec hive-postgres-1 psql -U hive -d hive -c "
  DELETE FROM telemetry_agent_snapshots;
  DELETE FROM telemetry_hive_snapshots;
  DELETE FROM telemetry_event_stream;"
```

## Nuclear Option

This destroys the event chain. Do not run it unless the user explicitly asks for a destructive database reset and acknowledges that events, tasks, and audit trail are erased.

```bash
pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true
sleep 3
pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null || true
systemctl --user stop hive hive-ops-api work-server 2>/dev/null || true
cd /Transpara/transpara-ai/repos/hive
docker compose down -v
docker compose up -d postgres
```

## Common Problems

| Symptom | Likely cause | First action |
|---|---|---|
| `curl` to `:8085` or `:8080` is refused | service stopped or crash-looping | `systemctl --user status <service>` |
| Service is `activating (auto-restart)` | Postgres down or unreachable | inspect `journalctl --user -u hive-ops-api`; propose starting Postgres and wait for explicit user confirmation |
| `401 unauthorized` | missing or wrong bearer token | load `/home/transpara/.config/hive/hive.env` and retry |
| `Invalid API key` on agents | Anthropic env var overrides CLI auth | unset `ANTHROPIC_API_KEY` and `HIVE_ANTHROPIC_API_KEY` for the shell |
| agents show zero cost just after startup | runtime has not completed iterations | wait a few minutes and re-check telemetry |
| port conflict | old manual process or unrelated service | inspect `lsof -i :8080 -i :8081 -i :8085`; do not blind-kill by port |
