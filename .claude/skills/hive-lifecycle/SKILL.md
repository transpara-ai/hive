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
| **Postgres** | docker container `hive-postgres-1` | ALL interfaces `:5432` (compose publishes `5432:5432`) | dev credentials — network exposure is real; `docker compose up -d postgres` (compose file in `repos/hive`); DSN `postgres://hive:hive@localhost:5432/hive` |
| **work-server** | `work-server.service` (enabled) | ALL interfaces `:8080` (binds `":"+PORT`) | builds + runs `repos/work/work-server`; serves the telemetry API, task API, and dashboard |
| **hive-ops-api** | `hive-ops-api.service` (enabled) |  `loopback :8085` | runs `repos/hive/hive-ops-api`; serves the operator projection API; loads a model catalog |
| **hive runtime** | `hive.service` (**disabled** — on-demand) | — | `hive civilization daemon …`; the multi-agent civilization loop |

Paths: `HIVE_REPO=/Transpara/transpara-ai/repos/hive`, `WORK_REPO=/Transpara/transpara-ai/repos/work`.

## Authentication

The **Claude CLI** path (Max plan, `~/.claude/.credentials.json`) needs **no Anthropic API key** — and setting one breaks it.

**CRITICAL — never set these:** `ANTHROPIC_API_KEY`, `HIVE_ANTHROPIC_API_KEY`. They override the working CLI auth and cause "Invalid API key" on every Claude agent. Verify clean:

```bash
env | cut -d= -f1 | grep -i anthropic   # names only — never print values; must print nothing
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
# 1. Postgres (Docker) — everything else depends on it. WAIT until it accepts connections
#    (compose returns while the container may still be "health: starting").
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do
  tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }
  echo "waiting for postgres…"; sleep 1
done
docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null && echo "postgres ready"

# 2. The API services (systemd --user) — gated on Postgres readiness: starting them
#    against a dead Postgres just puts both into crash loops. Dirty start? Run
#    "Hive Down" first to clear any stale manual go-run process holding :8080/:8085.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  systemctl --user start work-server hive-ops-api
  systemctl --user is-active work-server hive-ops-api   # expect: active / active
else
  echo "postgres not ready — NOT starting services; fix postgres first"
fi

# 3. (Optional) the multi-agent runtime — on demand, foreground for visibility.
#    GATED on Postgres readiness like step 2: launching the daemon with a Postgres
#    store while the DB is down only produces a crashing runtime.
#    DEFAULT is human-in-the-loop: authority requests + role proposals BLOCK for approval.
#    catalog-mixed.yaml routes reviewer/strategist to OpenRouter → needs OPENROUTER_API_KEY
#    This omits --catalog → built-in CLAUDE-only defaults (Max plan, no provider keys);
#    add `--catalog ./catalog-mixed.yaml --catalog-reload-interval 1m` ONLY if a local
#    Ollama model is running AND OPENROUTER_API_KEY is set.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  cd /Transpara/transpara-ai/repos/hive
  LOVYOU_API_KEY= go run ./cmd/hive civilization daemon \
      --human Michael \
      --store postgres://hive:hive@localhost:5432/hive
else
  echo "postgres not ready — NOT launching the runtime"
fi
#   Full autonomy (auto-approve everything) is an EXPLICIT opt-in: add
#       --approve-requests --approve-roles
#   The packaged unit `systemctl --user start hive` runs in FULL-AUTONOMY mode —
#   its ExecStart already includes --approve-requests --approve-roles.
#   ⚠ civilization run/daemon default their Site API to https://transpara.ai:
#   an ambient LOVYOU_API_KEY enables a reconciliation loop + task-completion
#   mirror posts against PRODUCTION. The blank LOVYOU_API_KEY= prefix disables
#   that client for local runs; production crossing needs explicit user
#   authorization. Before systemctl --user start/restart hive, run the
#   "hive.service credential preflight" below — the unit's env comes from unit
#   config, EnvironmentFiles, and the user manager, not this shell.
```

**hive.service credential preflight** (read-only, names only; any hit or unreadable source = do NOT start without explicit production authorization):

```bash
# Merged EFFECTIVE properties via `systemctl show` — systemd resolves unit
# fragments, drop-ins, list resets, whitespace, and quoting authoritatively,
# so no text-parsing of unit files is needed (parsing `systemctl cat` output
# missed quoted/spaced assignments and ignored later-fragment list resets).
cred=0; unknown=0; auto=0
if unitenv=$(systemctl --user show hive -p Environment --value 2>/dev/null); then
  printf '%s\n' "$unitenv" | tr ' ' '\n' | tr -d '"'"'"'"' | grep -q '^LOVYOU_API_KEY=' && { echo "unit Environment sets LOVYOU_API_KEY"; cred=1; }
else
  echo "cannot read unit properties"; unknown=1
fi
for ef in $(systemctl --user show hive -p EnvironmentFiles --value 2>/dev/null | sed 's/ (ignore_errors=[^)]*)$//' | sed 's/^-//'); do
  if [ -r "$ef" ]; then
    grep -Eq "^[[:space:]]*(export[[:space:]]+)?[\"']?LOVYOU_API_KEY[\"']?[[:space:]]*=" "$ef" && { echo "$ef sets LOVYOU_API_KEY"; cred=1; }
  else
    echo "cannot read $ef — UNKNOWN"; unknown=1
  fi
done
systemctl --user show-environment | cut -d= -f1 | grep -qx LOVYOU_API_KEY && { echo "user-manager environment sets LOVYOU_API_KEY"; cred=1; }
systemctl --user show hive -p ExecStart --value 2>/dev/null | grep -qE -- '--approve-(requests|roles)' && { echo "ExecStart carries full-autonomy flags"; auto=1; }
cleared=0
case " $(systemctl --user show hive -p UnsetEnvironment --value 2>/dev/null) " in
  *" LOVYOU_API_KEY "*) cleared=1;;
esac
if [ "$unknown" -ne 0 ]; then
  echo "VERDICT: a source is unreadable — do NOT start"
elif [ "$cred" -ne 0 ] && [ "$cleared" -eq 0 ]; then
  echo "VERDICT: credential present and not cleared — do NOT start without explicit production authorization (or apply the clearing drop-in)"
elif [ "$auto" -ne 0 ]; then
  echo "VERDICT: full-autonomy flags in ExecStart — starting RESUMES FULL AUTONOMY; start ONLY with explicit current-turn approval"
else
  echo "VERDICT: OK to start"
fi
```

If a source sets the key and the user still wants a local-only runtime: apply a clearing drop-in (mutating config — confirm with the user first), re-run the preflight (its verdict reads the merged effective `UnsetEnvironment` property, so an active drop-in — and any later list reset — is judged correctly), and after start verify via the Endpoint Reference effective-environment check with unit `hive` / variable `LOVYOU_API_KEY` — `UnsetEnvironment` strips the variable from the final environment regardless of source, but the post-start check is the authoritative proof:

```bash
mkdir -p ~/.config/systemd/user/hive.service.d
printf '[Service]\nUnsetEnvironment=LOVYOU_API_KEY\n' > ~/.config/systemd/user/hive.service.d/no-remote-credential.conf
systemctl --user daemon-reload
```

## Hive Down

```bash
# Stop the runtime first, then the API services.
systemctl --user stop hive 2>/dev/null                    # if started as the unit
pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null             # graceful; matches both `go run` and its compiled child
sleep 3; pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null    # sweep any survivor
systemctl --user stop hive-ops-api work-server
# Sweep stray MANUAL hive/work runtimes from the old non-systemd flow — BY IDENTITY, not by
# port (a blind port-kill could hit pgadmin or a dev server):
pkill -f '[c]md/work-server|[e]xe/work-server' 2>/dev/null
pkill -f '[c]md/hive-ops-api|[e]xe/hive-ops-api' 2>/dev/null
lsof -i :8080 -i :8081 -i :8085 2>/dev/null && echo "^ a port is still bound — inspect (may be non-hive) and clear it manually" || echo "ports clear"
# Postgres usually stays up (data persists). Full stop:
#   cd /Transpara/transpara-ai/repos/hive && docker compose down
```

## Hive Restart

```bash
# Ensure Postgres is up + accepting connections first (restart = a true down→up):
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done
# EVERYTHING below is gated on Postgres readiness — restarting services or the
# runtime against a dead DB only produces crash loops; on timeout, stop here.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  systemctl --user restart work-server hive-ops-api
  # The hive runtime is on-demand (unit disabled). `restart` would START the disabled
  # unit and launch the daemon unexpectedly — so bounce it ONLY if already running.
  # Explicit if/else so a real restart FAILURE surfaces (not masked as "not running"):
  if systemctl --user is-active --quiet hive; then
    # Restarting hive.service re-launches whatever the unit encodes. Run the
    # "hive.service credential preflight" (Hive Up section) — its VERDICT covers
    # credentials AND full-autonomy flags from the merged effective properties.
    echo "hive.service is active — run the hive.service credential preflight;"
    echo "restart only on 'VERDICT: OK to start' (or with the explicit approval"
    echo "the verdict names): systemctl --user restart hive"
  elif pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' >/dev/null; then
    echo "MANUAL (go run) runtime detected — stopping it; re-run YOUR exact command to bring it back (its verb/flags are shown below, so the workload + governance mode are preserved):"
    pgrep -af '[h]ive (--human|civilization|pipeline|role|council|factory)'
    pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)'; sleep 3
    pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null
  else
    echo "hive runtime not running (on-demand) — left stopped"
  fi
else
  echo "postgres not ready — NOT restarting services or runtime; fix postgres first"
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
elif pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' >/dev/null; then echo "manual runtime: RUNNING"
else echo "runtime: stopped"; fi
ss -tlnp 2>/dev/null | grep -q ':8081 ' && echo "hive webhook :8081: listening" || echo "hive webhook :8081: not listening"

echo "=== postgres ==="
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'
docker exec hive-postgres-1 psql -U hive -d hive -c 'SELECT count(*) FROM events' 2>/dev/null || echo "DB unreachable"

echo "=== endpoint health ==="
curl -s --connect-timeout 3 --max-time 10 -o /dev/null -w 'work-server  /health           HTTP %{http_code}\n' http://localhost:8080/health
curl -s --connect-timeout 3 --max-time 10 -o /dev/null -w 'hive-ops-api /health           HTTP %{http_code}\n' http://localhost:8085/health
curl -s --connect-timeout 3 --max-time 10 -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/status
```

**Crash-loop diagnosis.** If `is-active` reports `activating (auto-restart)`, the service is crash-looping — almost always because **Postgres is down** (both `work-server` and `hive-ops-api` fail their DB connection on start). Diagnose read-only:

```bash
journalctl --user -u hive-ops-api -n 20 --no-pager   # look for: dial tcp 127.0.0.1:5432: connect: connection refused
```

Starting Postgres is a **mutating recovery action, not part of status** — perform it only after the user confirms:

```bash
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres   # bring the DB back (recreates it if it was `compose down`-ed); services self-heal on their next restart tick
```

## Endpoint Reference

### hive-ops-api — `http://localhost:8085` (Bearer `HIVE_OPS_API_KEY`, default `dev`)

| Method · Path | Purpose |
|---|---|
| `GET /health` | liveness (no auth) |
| `GET /api/hive/operator-projection` | pending approvals, authority decisions, lifecycle, key traces |
| `GET /api/hive/civilization/assembly-projection` | role/agent topology, org tiers, model selection |
| `POST /api/hive/operator-decision` † | decide a **draft-PR-create** authority request (`approved` \| `denied`) — other request types are rejected |
| `POST /api/hive/runs` † | launch an operator-initiated run |
| `POST /api/hive/model-selection/role-policy` † | update a role's model policy |

> **†** Write routes exist **only in writer mode** — when `hive-ops-api` is started with `HIVE_OPS_HUMAN_ACTOR` set. The default `hive-ops-api.service` does **not** set it — but do **not** assume read-only from the unit file: variables inherited from the systemd `--user` manager don't appear in unit `Environment=` lines. Verify from the running process's effective environment (names only, never values) before treating the API as read-only:
>
> ```bash
> pid=$(systemctl --user show hive-ops-api -p MainPID --value)
> if [ "${pid:-0}" -gt 0 ] 2>/dev/null && names=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null) && [ -n "$names" ]; then
>   printf '%s\n' "$names" | cut -d= -f1 | grep -cx HIVE_OPS_HUMAN_ACTOR
> else
>   echo "cannot read process environment — mode UNKNOWN"
> fi
> ```
>
> `0` = read-only; non-zero = **writer mode**. NULs are converted to newlines **inside** the substitution — `tr` is the sole command, so a failed `/proc` read fails the whole condition (no pipeline masks it) and no NUL bytes are lost to command substitution (which strips them). Service down or unreadable = mode unknown, fail closed.

```bash
curl -s --connect-timeout 3 --max-time 10 -H "Authorization: Bearer ${HIVE_OPS_API_KEY:-dev}" \
     http://localhost:8085/api/hive/operator-projection | jq .
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
curl -s --connect-timeout 3 --max-time 10 -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/status | jq .
```

## Model Catalog

The operator API and the runtime select models from a catalog YAML (hot-reloaded):
- **hive-ops-api**: `HIVE_OPS_CATALOG` — the unit **resolves to `repos/hive/catalog-mixed.yaml`**; `HIVE_OPS_CATALOG_RELOAD_INTERVAL=1m`. Verify the actual resolved path from the running process's effective environment (this variable's value only — a filepath, not a secret; unrelated values are never printed; `/proc` unreadable = UNKNOWN, fail closed):

  ```bash
  pid=$(systemctl --user show hive-ops-api -p MainPID --value)
  if [ "${pid:-0}" -gt 0 ] 2>/dev/null && envnames=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null); then
    printf '%s\n' "$envnames" | grep '^HIVE_OPS_CATALOG=' || echo "HIVE_OPS_CATALOG unset (built-in defaults)"
  else
    echo "cannot read process environment — catalog UNKNOWN"
  fi
  ```
- **hive runtime (daemon)**: `--catalog <path> --catalog-reload-interval 1m`. **`council`** takes `--catalog <path>` only — **no** `--catalog-reload-interval` (e.g. `council --catalog ./catalog-mixed.yaml`).

Only `catalog-mixed.yaml` is checked into `repos/hive` (a missing, uncommitted `catalog-codex.yaml` referenced by `hive.service`'s `ExecStart` is the likely cause if that unit fails to start). For a manual Claude-only run, **omit `--catalog`** (built-in Claude defaults); add `--catalog ./catalog-mixed.yaml` **only** when a local Ollama model is running and `OPENROUTER_API_KEY` is set.

## On-demand Runtime (`cmd/hive` verbs)

```bash
cd /Transpara/transpara-ai/repos/hive
LOVYOU_API_KEY= go run ./cmd/hive civilization run    --human Michael --idea "…" \
       --store postgres://hive:hive@localhost:5432/hive                      # one-shot multi-agent (--store required to persist; omit only for a throwaway in-memory run)
LOVYOU_API_KEY= go run ./cmd/hive civilization daemon --human Michael \
       --store postgres://hive:hive@localhost:5432/hive                      # long-running (add --approve-requests --approve-roles for full autonomy)
LOVYOU_API_KEY=dev go run ./cmd/hive pipeline run        --api http://localhost:8082 --repo .   # Scout → Builder → Critic (needs the local API up — see "Local / Offline"; no --idea)
LOVYOU_API_KEY=dev go run ./cmd/hive role <name> run     --api http://localhost:8082 --repo .   # single agent
LOVYOU_API_KEY=dev go run ./cmd/hive council --api http://localhost:8082 --topic "…"   # one deliberation (add --catalog ./catalog-mixed.yaml only with Ollama + OPENROUTER_API_KEY)
# ⚠ council's --api DEFAULTS to https://transpara.ai and, when LOVYOU_API_KEY is set,
#   POSTS up to 2000 chars of the deliberation report to the remote social feed —
#   AND interpolates that key into every council agent's prompt (exposing the bearer
#   to model providers). For local runs, always pin --api to the local endpoint AND
#   replace any ambient remote credential with the non-secret local `dev` credential
#   as shown; remote publishing requires the user's explicit authorization in the
#   current turn.
go run ./cmd/hive ingest --priority normal <file.md>   # registered-repo API flow: needs LOVYOU_API_KEY (set HIVE_INGEST_SKIP_REPO=1 to skip the repo bootstrap) — check `--help` before use
```

Flags are **per-verb**. `civilization run`: `--human` (required), `--idea`/`--spec` (seed), `--store` (or `DATABASE_URL`), `--repo`, `--catalog`, `--approve-requests`, `--approve-roles`. `civilization daemon`: the same **except** its seed flag is `--seed-spec` (there is no `--idea`/`--spec`). `pipeline`/`role`: `--api`, `--space`, `--repo`, `--agent-id` (no `--human`/`--idea`; for the local stack pass `--api http://localhost:8082`). Always confirm with `go run ./cmd/hive <verb> --help`.

## Operator Actions

- **Approve a proposed role** — use the CLI (it emits the `hive.role.approved` + budget events the runtime needs): `go run ./cmd/approve-role --role <name> --store postgres://hive:hive@localhost:5432/hive`. ⚠ This CLI **also allocates budget**: it emits `agent.budget.adjusted` with an initial budget of **200** for the new role (`cmd/approve-role/main.go`). Disclose that amount and get the user's explicit approval for **both** the role and the initial budget before running it. `POST /api/hive/operator-decision` only decides **draft-PR-create** authority requests — not role proposals.
- **Inject a spec/file as a task** — posts to the civilization daemon **webhook** (`:8081/event`), so the runtime must be running: `go run ./cmd/inject-file --title "…" --priority medium <file>`.

## Local / Offline

Run the pipeline against a local API with no external dependency:

```bash
cd /Transpara/transpara-ai/repos/hive
go run ./cmd/localapi --addr localhost:8082 --api-key dev   # add --site-db to read the site DB instead of hive
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
pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null; systemctl --user stop hive 2>/dev/null; sleep 3; pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null
docker exec hive-postgres-1 psql -U hive -d hive -c "
    DELETE FROM telemetry_agent_snapshots;
    DELETE FROM telemetry_hive_snapshots;
    DELETE FROM telemetry_event_stream;"
```

## Nuclear Option (destroys the event chain)

**WARNING: erases all events / tasks / audit trail. Only for a corrupted chain.**

```bash
pkill -INT -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null; sleep 3; pkill -KILL -f '[h]ive (--human|civilization|pipeline|role|council|factory)' 2>/dev/null   # kill any manual go-run runtime first
systemctl --user stop hive hive-ops-api work-server 2>/dev/null
cd /Transpara/transpara-ai/repos/hive
docker compose down -v && docker compose up -d postgres
```

## Common Problems

| Symptom | Cause | Fix |
|---|---|---|
| Service `activating (auto-restart)` | Postgres down — DB connection fails on start | inspect `journalctl --user -u hive-ops-api`; propose starting Postgres and wait for explicit user confirmation |
| `curl :8085` / `:8080` connection refused | that service not running / crash-looping | `systemctl --user status <svc>`; see crash-loop diagnosis |
| `401 unauthorized` from `:8085` | missing/wrong bearer | add `-H "Authorization: Bearer $HIVE_OPS_API_KEY"` (default `dev`) |
| "Invalid API key" on all agents | `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` set | `unset` them; remove from shell profile |
| All agents `$0.000` cost | runtime just started, no iterations yet | wait 2–3 min |
| Port `:8080` conflict on start | another process bound it | `lsof -i :8080`; stop the conflict |
