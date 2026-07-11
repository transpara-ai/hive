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
| Postgres | Docker container `hive-postgres-1` | ALL interfaces `:5432` (compose publishes `5432:5432`) | Dev credentials — network exposure is real; DSN `postgres://hive:hive@localhost:5432/hive`; compose file in `/Transpara/transpara-ai/repos/hive` |
| work-server | `work-server.service` | ALL interfaces `:8080` (binds `":"+PORT`) | Built from `/Transpara/transpara-ai/repos/work/work-server`; telemetry, task API, dashboard |
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
    # Restarting hive.service re-launches whatever the unit encodes — gate BOTH before restart:
    # (1) credential: run the "hive.service credential preflight" (Hive Up section); proceed only on an OK verdict.
    # (2) autonomy: flags already in ExecStart RESUME on restart.
    if systemctl --user cat hive 2>/dev/null | grep -qE -- '--approve-(requests|roles)'; then
      echo "hive.service ExecStart carries full-autonomy flags — restart resumes FULL AUTONOMY; do NOT restart without explicit current-turn approval"
    else
      echo "run the hive.service credential preflight; only on an OK verdict restart with: systemctl --user restart hive"
    fi
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

Default `hive-ops-api.service` should be read-only because it does not set `HIVE_OPS_HUMAN_ACTOR`. Confirm before POSTing by inspecting the running process's effective environment — unit `Environment=` lines miss variables inherited from the systemd `--user` manager, so `systemctl show -p Environment` can misreport writer mode as read-only. Check names only, never dump values:

```bash
pid=$(systemctl --user show hive-ops-api -p MainPID --value)
if [ "${pid:-0}" -gt 0 ] 2>/dev/null && names=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null) && [ -n "$names" ]; then
  printf '%s\n' "$names" | cut -d= -f1 | grep -cx HIVE_OPS_HUMAN_ACTOR
else
  echo "cannot read process environment — mode UNKNOWN; do not POST"
fi
```

`0` means the running process has no `HIVE_OPS_HUMAN_ACTOR` (read-only); a non-zero count means writer mode. The NUL separators are converted to newlines **inside** the substitution (`tr` is the sole command, so a failed `/proc` read fails the whole condition — no pipeline can mask it, and no NUL bytes are lost to command substitution, which strips them). Service down or unreadable — the mode is unknown; do not POST.

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

- `hive-ops-api` uses `HIVE_OPS_CATALOG`. Verify the actual resolved path from the running process's effective environment — this variable's value only (a filepath, not a secret; unrelated values are never printed; `/proc` unreadable = UNKNOWN, fail closed):

  ```bash
  pid=$(systemctl --user show hive-ops-api -p MainPID --value)
  if [ "${pid:-0}" -gt 0 ] 2>/dev/null && envnames=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null); then
    printf '%s\n' "$envnames" | grep '^HIVE_OPS_CATALOG=' || echo "HIVE_OPS_CATALOG unset (built-in defaults)"
  else
    echo "cannot read process environment — catalog UNKNOWN"
  fi
  ```
- Hive runtime uses `--catalog <path> --catalog-reload-interval 1m`.
- `council` accepts `--catalog <path>` only; do not pass `--catalog-reload-interval` to `council`.
- If avoiding provider keys, omit `--catalog` for built-in Claude defaults.

## On-demand Runtime

Runtime execution is not a status check. Run only after explicit user request.

```bash
cd /Transpara/transpara-ai/repos/hive

LOVYOU_API_KEY= go run ./cmd/hive civilization run \
  --human Michael \
  --idea "..." \
  --store postgres://hive:hive@localhost:5432/hive

LOVYOU_API_KEY= go run ./cmd/hive civilization daemon \
  --human Michael \
  --store postgres://hive:hive@localhost:5432/hive

LOVYOU_API_KEY=dev go run ./cmd/hive pipeline run --api http://localhost:8082 --repo .
LOVYOU_API_KEY=dev go run ./cmd/hive role <name> run --api http://localhost:8082 --repo .
LOVYOU_API_KEY=dev go run ./cmd/hive council --api http://localhost:8082 --topic "..."
```

`civilization run`/`civilization daemon` also default their Site API to `https://transpara.ai`: with an ambient `LOVYOU_API_KEY` present they enable a reconciliation loop and task-completion mirror posts against production. The blank `LOVYOU_API_KEY=` prefix above disables that client for local runs; crossing to the production Site API requires the user's explicit authorization.

Before starting or restarting `hive.service` (its environment comes from unit config, `EnvironmentFile`s, and the systemd `--user` manager — not this shell), run this read-only, names-only preflight; any hit or unreadable source means do NOT start without explicit production authorization:

```bash
hit=0
cfg=$(systemctl --user cat hive 2>/dev/null) || { echo "cannot read hive.service config"; hit=1; }
# Every Environment= assignment, including quoted and multi-assignment forms
# (splitting quoted values may over-match — that errs closed, never open):
printf '%s\n' "$cfg" | grep '^Environment=' | sed 's/^Environment=//' | tr ' ' '\n' | tr -d '"' | grep -q '^LOVYOU_API_KEY=' && { echo "unit Environment= sets LOVYOU_API_KEY"; hit=1; }
for ef in $(printf '%s\n' "$cfg" | grep '^EnvironmentFile=' | cut -d= -f2- | sed 's/^-//'); do
  if [ -r "$ef" ]; then
    grep -Eq '^[[:space:]]*(export[[:space:]]+)?"?LOVYOU_API_KEY"?=' "$ef" && { echo "$ef sets LOVYOU_API_KEY"; hit=1; }
  else
    echo "cannot read $ef — UNKNOWN"; hit=1
  fi
done
systemctl --user show-environment | cut -d= -f1 | grep -qx LOVYOU_API_KEY && { echo "user-manager environment sets LOVYOU_API_KEY"; hit=1; }
if [ "$hit" -eq 0 ]; then
  echo "preflight clean — OK to start"
elif systemctl --user cat hive 2>/dev/null | grep -qx 'UnsetEnvironment=LOVYOU_API_KEY'; then
  echo "credential present in sources but the clearing drop-in is ACTIVE — OK to start; verify post-start"
else
  echo "do NOT start without explicit production authorization (or apply the clearing drop-in)"
fi
```

If a source sets the key and the user still wants a local-only runtime, apply a clearing drop-in (a mutating config change — confirm with the user first), then re-run the preflight (it reports the source hits as cleared by the active drop-in) and, after start, verify with the Endpoint Reference effective-environment check using unit `hive` and variable `LOVYOU_API_KEY` — `UnsetEnvironment` strips the variable from the final environment regardless of which source set it, but the post-start check is the authoritative proof:

```bash
mkdir -p ~/.config/systemd/user/hive.service.d
printf '[Service]\nUnsetEnvironment=LOVYOU_API_KEY\n' > ~/.config/systemd/user/hive.service.d/no-remote-credential.conf
systemctl --user daemon-reload
```

Warning: `council` defaults `--api` to `https://transpara.ai` and, when `LOVYOU_API_KEY` is set, posts up to 2000 characters of the deliberation report to the remote social feed — and interpolates that key into every council agent's prompt, exposing the bearer token to model providers. For local runs, always pin `--api` to the local endpoint and replace any ambient remote credential with the non-secret local `dev` credential as shown; remote publishing requires the user's explicit authorization in the current turn.

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
go run ./cmd/localapi --addr localhost:8082 --api-key dev
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
