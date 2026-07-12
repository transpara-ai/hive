---
name: hive-lifecycle
description: "Manage the transpara-ai hive stack lifecycle on nucbuntu — the systemd --user services (work-server, hive-ops-api), the Dockerized Postgres, and the on-demand civilization runtime. Use whenever the user says 'hive up', 'hive down', 'hive status', 'hive restart', 'hive run', 'hive help', 'start/stop/restart the hive', 'run the hive', 'run agents on', 'hive idea', 'run pipeline', 'run builder', 'run scout', 'is the hive running', 'kill the hive', 'bounce the hive', or mentions stuck agents, a frozen hive, or needing a clean slate."
---

# Hive Lifecycle Management

Start, stop, restart, and inspect the transpara-ai hive stack on **nucbuntu**. The APIs run as **systemd `--user` services** over a Dockerized Postgres; the multi-agent runtime is started on demand.

Infrastructure and runtime are separate intents: `hive up` starts only Postgres and the API services. Never launch the Civilization runtime unless the user separately requests it. Every runtime verb must receive an explicit, validated `--repo` target because its default is the current directory and this runbook executes from the canonical Hive checkout.

> On-prem / private (no public URLs). The Claude CLI path needs no key; the default mixed catalog also routes some roles to OpenRouter — see **Authentication**.

## Hive Help

For **"hive help" / "hive commands"**: print the section list below — Stack Components, Authentication, Hive Up / Down / Restart / Status, Endpoint Reference, Model Catalog, On-demand Runtime, Operator Actions, Logs, Common Problems — and **stop** (do not start/stop/restart anything). For CLI usage: `go run ./cmd/hive --help` or `go run ./cmd/hive <verb> --help`.

## Stack Components

| Component | Unit / process | Bind | Notes |
|---|---|---|---|
| **Postgres** | docker container `hive-postgres-1` | ALL interfaces `:5432` (compose publishes `5432:5432`) | dev credentials — network exposure is real; `docker compose up -d postgres` (compose file in `repos/hive`); DSN `postgres://hive:hive@localhost:5432/hive` |
| **work-server** | `work-server.service` (enabled) | ALL interfaces `:8080` (binds `":"+PORT`) | builds + runs `repos/work/work-server`; serves the telemetry API, task API, and dashboard |
| **hive-ops-api** | `hive-ops-api.service` (enabled) |  `loopback :8085` | runs `repos/hive/hive-ops-api`; serves the operator projection API; loads a model catalog |
| **hive runtime** | `hive.service` (**disabled** — on-demand) | webhook ALL interfaces `:8081` when running (unauthenticated `POST /event`) | `hive civilization daemon …`; the multi-agent civilization loop |

Paths: `HIVE_REPO=/Transpara/transpara-ai/repos/hive`, `WORK_REPO=/Transpara/transpara-ai/repos/work`.

## Authentication

The **Claude CLI** path (Max plan, `~/.claude/.credentials.json`) needs **no Anthropic API key** — and setting one breaks it.

**CRITICAL — never set these:** `ANTHROPIC_API_KEY`, `HIVE_ANTHROPIC_API_KEY`. They override the working CLI auth and cause "Invalid API key" on every Claude agent. Verify clean:

```bash
env | cut -d= -f1 | grep -i anthropic || echo "clean — no ANTHROPIC* variables set"   # names only, never values; nonzero grep is the GOOD case and the || arm keeps strict shells alive
```

⚠ **The default `catalog-mixed.yaml` is a MIXED-provider catalog** (Claude CLI + Codex CLI + Ollama + OpenRouter). Claude/Codex use `subscription` auth and Ollama is `local`, but the **OpenRouter** roles (e.g. `reviewer`, `strategist`) use `auth_mode: api-key` and require **`OPENROUTER_API_KEY`**. For a Claude-CLI-only run with no provider keys, **omit `--catalog`** — the runtime falls back to its built-in Claude defaults (there is no checked-in Claude-only catalog). ⚠ This is DEFAULT ROUTING ONLY: a durable role-model policy stored in Postgres (set via the role-policy endpoint) still overrides the built-in defaults and can route roles to Codex or API-key models — for strict provider isolation, inspect the assembly projection's model selection first and clear or repoint stored role policies.

API bearer tokens (for the endpoints below):
- **hive-ops-api**: `HIVE_OPS_API_KEY` (default `dev`).
- **work-server**: `WORK_API_KEY` (lives in `~/.config/hive/hive.env`, not exported to your shell).

To run the `curl` examples below from a plain shell, load the tokens first:

```bash
__xt=0; case $- in *x*) __xt=1; set +x;; esac   # freshly derived from $-: a stale inherited __xt must never re-enable xtrace
set -a; . /home/transpara/.config/hive/hive.env 2>/dev/null; set +a   # populates WORK_API_KEY (HIVE_OPS_API_KEY defaults to `dev`)
[ "${__xt:-}" = 1 ] && set -x; unset __xt
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

```

`Hive Up` is infrastructure-only. Runtime startup, full-autonomy flags, webhook exposure, target-repository selection, and Site credential posture are separate protected decisions handled under **On-demand Runtime** below.

**hive.service start/restart is a protected action — a human gate, not automated forensics.** A shell runbook cannot reliably prove a systemd unit safe: environment sources, ExecStart wrappers, exec phases, and variable expansion all provide places for authority to hide, and a mechanical verifier belongs in a tested Go subcommand (tracked as separate work), not here. Before `systemctl --user start|restart hive`:

1. Obtain the user's explicit current-turn approval naming BOTH postures: credential (an ambient `LOVYOU_API_KEY` anywhere in the unit's effective configuration means production Site integration) and autonomy (the packaged unit's `ExecStart` includes `--approve-requests --approve-roles`, so starting it RESUMES FULL AUTONOMY).
2. For a local-only runtime, do NOT start the unit at all — use the foreground `LOVYOU_API_KEY= go run ./cmd/hive civilization daemon …` form under **On-demand Runtime** below, where both postures are explicit in the command itself. The unit's reconciliation loop begins its first cycle immediately on start, before any post-start check can run, so pre-start approval must always cover the worst-case production-connected posture.
3. After any approved start, confirm the running process matches the posture the user approved (probe below) — verification only; it cannot undo the first reconciliation cycle.

Post-start (or post-restart) posture confirmation — compares the RUNNING runtime against the posture the user approved (this variable's value only; it is a bearer credential, so only presence/emptiness is judged and the value itself is never printed):

```bash
( set +x 2>/dev/null   # secret-bearing expansions below must never reach an xtrace transcript
pid=$(systemctl --user show hive -p MainPID --value 2>/dev/null || true)
if [ "${pid:-0}" -gt 0 ] 2>/dev/null && envlines=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null) && [ -n "$envlines" ]; then
  keyval=$(printf '%s\n' "$envlines" | grep '^LOVYOU_API_KEY=' | head -1 | cut -d= -f2- || true)   # no match is the expected local-only case: never trip errexit/pipefail
  if [ -n "$keyval" ]; then
    echo "runtime is PRODUCTION-CONNECTED (non-empty LOVYOU_API_KEY) — if the user approved the production posture, this MATCHES; otherwise STOP the unit now"
  else
    echo "runtime is local-only (LOVYOU_API_KEY absent or empty — an empty value leaves the Site client disabled)"
  fi
else
  echo "cannot read runtime process environment — posture UNKNOWN; if local-only was intended, STOP the unit"
fi
)
```

## Hive Down

```bash
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (nothing running, service already down) must not abort the block mid-way in strict shells
# Stop the runtime first, then the API services.
systemctl --user stop hive 2>/dev/null                    # if started as the unit
# Confirm the unit actually stopped: a failed stop (or one that could not
# cancel a queued auto-restart) must be REPORTED, not discarded — otherwise
# the port check below can read clear moments before a full-autonomy relaunch.
hive_state=$(systemctl --user show hive -p ActiveState --value 2>/dev/null || true)
case "$hive_state" in inactive|failed) : ;; *) echo "hive.service is still ${hive_state:-unreadable} after stop — a queued relaunch may fire; re-run: systemctl --user stop hive, then re-check";; esac
# Identity-verified kills: comm must be exact/versioned Hive or the go-run
# driver, so a stray argv match is never signaled.
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -INT "$pid" 2>/dev/null;; esac; done   # argv verb + comm identity; includes versioned binaries such as hive-test001-*
sleep 3
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -KILL "$pid" 2>/dev/null;; esac; done   # sweep any survivor
systemctl --user stop hive-ops-api work-server
# Stray MANUAL services are the OPERATOR's to stop — they know what they
# launched, and a name-only kill could hit an unrelated binary that merely
# shares the name. LIST candidates (pid + executable) and leave the decision
# with the human; the port check below surfaces anything still bound:
for name in work-server hive-ops-api; do
  pgrep -x "$name" | while read -r pid; do printf 'manual %s candidate: pid=%s exe=%s\n' "$name" "$pid" "$(readlink /proc/"$pid"/exe 2>/dev/null || echo unreadable)"; done
done
lsof -i :8080 -i :8081 -i :8085 2>/dev/null && echo "^ a port is still bound — inspect (may be non-hive) and clear it manually" || echo "ports clear"
)
# Postgres usually stays up (data persists). Full stop:
#   cd /Transpara/transpara-ai/repos/hive && docker compose down
```

## Hive Restart

```bash
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (a down unit, an unreadable manager) must not abort the block mid-way in strict shells
restart_status=0   # 0 = restarted, 1 = FAILED, 2 = runtime decision PENDING the human gate — the exit status must say which
# Ensure Postgres is up + accepting connections first (restart = a true down→up):
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done
# EVERYTHING below is gated on Postgres readiness — restarting services or the
# runtime against a dead DB only produces crash loops; on timeout, stop here.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  systemctl --user restart work-server hive-ops-api || { echo "RESTART FAILED (work-server/hive-ops-api) — inspect: journalctl --user -u work-server -u hive-ops-api"; restart_status=1; }
  for svc in work-server hive-ops-api; do   # per-unit: is-active with multiple units exits 0 if ANY is active
    svc_state=$(systemctl --user is-active "$svc" 2>/dev/null || true)
    [ "$svc_state" = "active" ] || { echo "$svc is ${svc_state:-unreadable} after restart — do not report success"; restart_status=1; }
  done
  # The hive runtime is on-demand (unit disabled). `restart` would START the disabled
  # unit and launch the daemon unexpectedly — so bounce it ONLY if already running.
  # Explicit if/else so a real restart FAILURE surfaces (not masked as "not running"):
  # Merged-property read, not is-active: a unit in `activating (auto-restart)`
  # reads as "stopped" via is-active, yet systemd will relaunch the encoded
  # (full-autonomy) command shortly — bypassing this gate. Allowlist only the
  # provably-stopped states; anything else (incl. unreadable) is MANAGED.
  # Runtime adjudication is a HUMAN gate, not shell forensics (a runbook
  # cannot reliably prove systemd transition states; the mechanical verifier
  # belongs in a tested Go subcommand — tracked separately). Read the merged
  # state, REPORT it verbatim, mutate nothing:
  hive_state=$(systemctl --user show hive -p ActiveState --value 2>/dev/null)
  hive_sub=$(systemctl --user show hive -p SubState --value 2>/dev/null)
  if [ "$hive_state" != "inactive" ] && [ "$hive_state" != "failed" ]; then
    # active/reloading: running now. activating/auto-restart: systemd WILL
    # relaunch the encoded full-autonomy command shortly. Unreadable: unknown.
    # In every one of these the decision is the human's — including whether
    # to cancel a queued relaunch (systemctl --user stop hive), which must be
    # CONFIRMED by re-reading ActiveState afterward, never assumed.
    echo "hive.service state=${hive_state:-unreadable}/${hive_sub:-unreadable} — MANAGED runtime (running, or a queued relaunch)."
    echo "protected action: any stop/cancel/restart needs the user's explicit current-turn approval"
    echo "(credential + autonomy postures). To cancel a queued auto-restart after approval:"
    echo "  systemctl --user stop hive   # then re-read ActiveState to CONFIRM before relying on it"
    [ "$restart_status" -eq 0 ] && restart_status=2   # the requested restart is INCOMPLETE pending the human decision
  elif hive_pids=$(pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) echo "$pid";; esac; done) && [ -n "$hive_pids" ]; then   # argv verb + comm identity; includes versioned binaries
    # Do NOT terminate yet — killing first would lose the workload and its
    # governance flags with nothing to relaunch. Get the user's original
    # command (argv is never echoed: it may contain sensitive --idea text or
    # credential assignments) or explicit stop-without-restore authorization,
    # THEN stop with the identity-verified kill loops from "Hive Down" and relaunch their command.
    echo "MANUAL (go run) runtime detected — restart needs the user's original command first:"
    printf '%s\n' "$hive_pids" | xargs -r ps -o pid=,comm= -p 2>/dev/null   # PIDs + executable names only
    [ "$restart_status" -eq 0 ] && restart_status=2   # the requested restart is INCOMPLETE pending the user's command
  else
    echo "hive runtime not running (on-demand) — left stopped"
  fi
else
  echo "postgres not ready — NOT restarting services or runtime; fix postgres first"
  restart_status=1
fi
exit "$restart_status"   # subshell exit: 0 restarted / 1 failed / 2 pending the human runtime gate — strict callers see the truth, the operator's terminal survives
)
```

## Hive Status

```bash
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # STATUS BLOCK CONTRACT: nonzero exits below are INFORMATION (a down service, an absent key, a clean env) — never errors; relax strict shells for the whole block
__xt=0; case $- in *x*) __xt=1; set +x;; esac   # freshly derived from $-: a stale inherited __xt must never re-enable xtrace
set -a; . /home/transpara/.config/hive/hive.env 2>/dev/null; set +a   # load WORK_API_KEY for the telemetry probe
[ "${__xt:-}" = 1 ] && set -x; unset __xt
echo "=== services ==="
systemctl --user is-active work-server hive-ops-api
systemctl --user --no-pager status work-server hive-ops-api | grep -E 'Active:|Main PID:'

echo "=== hive runtime (systemd unit OR manual go-run) ==="
hive_state=$(systemctl --user show hive -p ActiveState --value 2>/dev/null)   # merged-property read: is-active reads `activating (auto-restart)` as stopped
if [ "$hive_state" = "active" ]; then echo "hive.service: active"
elif [ "$hive_state" != "inactive" ] && [ "$hive_state" != "failed" ]; then echo "hive.service: ${hive_state:-unreadable} — pending auto-restart or transition; treat as a MANAGED runtime"
elif hive_pids=$(pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) echo "$pid";; esac; done) && [ -n "$hive_pids" ]; then echo "manual runtime: RUNNING"   # argv verb + comm identity; includes versioned binaries
else echo "runtime: stopped"; fi
ss -tlnp 2>/dev/null | grep -q ':8081 ' && echo "hive webhook :8081: listening" || echo "hive webhook :8081: not listening"

echo "=== postgres ==="
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'
docker exec hive-postgres-1 psql -U hive -d hive -c 'SELECT count(*) FROM events' 2>/dev/null || echo "DB unreachable"

echo "=== endpoint health ==="
curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'work-server  /health           HTTP %{http_code}\n' http://localhost:8080/health
curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'hive-ops-api /health           HTTP %{http_code}\n' http://localhost:8085/health
( set +x 2>/dev/null; printf 'header = "Authorization: Bearer %s"\n' "${WORK_API_KEY:-}" | curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' -K - http://localhost:8080/telemetry/status )   # bearer via stdin config: argv is world-readable in /proc
)
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
> ( set +x 2>/dev/null   # secret-bearing expansions below must never reach an xtrace transcript
> pid=$(systemctl --user show hive-ops-api -p MainPID --value 2>/dev/null || true)
> if [ "${pid:-0}" -gt 0 ] 2>/dev/null && envlines=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null) && [ -n "$envlines" ]; then
>   actor=$(printf '%s\n' "$envlines" | grep '^HIVE_OPS_HUMAN_ACTOR=' | head -1 | cut -d= -f2- || true)   # no match = read-only, the expected default: never trip errexit/pipefail
>   if [ -n "$actor" ]; then
>     echo "HIVE_OPS_HUMAN_ACTOR set and non-empty — WRITER MODE possible (an invalid actor id still yields read-only at startup; check journal for 'HIVE_OPS_HUMAN_ACTOR invalid')"
>   else
>     echo "HIVE_OPS_HUMAN_ACTOR absent or empty — read-only (opsWriterOptions requires a non-empty valid actor id)"
>   fi
> else
>   echo "cannot read process environment — mode UNKNOWN"
> fi
> )
> ```
>
> The probe reads this one variable's value (an operator actor id, not a secret) because presence alone over-claims: `opsWriterOptions` stays read-only for an empty or invalid id. NULs are converted to newlines **inside** the substitution — `tr` is the sole command, so a failed `/proc` read fails the whole condition (no pipeline masks it) and no NUL bytes are lost to command substitution (which strips them). Service down or unreadable = mode unknown, fail closed.

```bash
( set +ex 2>/dev/null; set +o pipefail 2>/dev/null; printf 'header = "Authorization: Bearer %s"\n' "${HIVE_OPS_API_KEY:-dev}" | curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -K - http://localhost:8085/api/hive/operator-projection | jq . )   # bearer via stdin config (argv is world-readable in /proc); jq inside the relaxed subshell
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
( set +ex 2>/dev/null; set +o pipefail 2>/dev/null; printf 'header = "Authorization: Bearer %s"\n' "${WORK_API_KEY:-}" | curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -K - http://localhost:8080/telemetry/status | jq . )   # bearer via stdin config (argv is world-readable in /proc)
```

## Model Catalog

The operator API and the runtime select models from a catalog YAML (hot-reloaded):
- **hive-ops-api**: `HIVE_OPS_CATALOG` — the unit **resolves to `repos/hive/catalog-mixed.yaml`**; `HIVE_OPS_CATALOG_RELOAD_INTERVAL=1m`. Verify the actual resolved path from the running process's effective environment (this variable's value only — a filepath, not a secret; unrelated values are never printed; `/proc` unreadable = UNKNOWN, fail closed):

  ```bash
  ( set +x 2>/dev/null   # secret-bearing expansions below must never reach an xtrace transcript
  pid=$(systemctl --user show hive-ops-api -p MainPID --value 2>/dev/null || true)
  if [ "${pid:-0}" -gt 0 ] 2>/dev/null && args=$(tr '\0' '\n' </proc/"$pid"/cmdline 2>/dev/null) && [ -n "$args" ] && envlines=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null) && [ -n "$envlines" ]; then
    # the --catalog/-catalog flag OVERRIDES the env var (Go flags accept one or two dashes; the flag's default is the env value)
    # Flag PRESENCE is tracked separately from its value: an explicit empty
    # --catalog= clears the env-derived default and selects the built-ins.
    if printf '%s\n' "$args" | grep -qE -x -- '--?catalog(=.*)?'; then
      flagval=$({ printf '%s\n' "$args" | grep -A1 -E -x -- '--?catalog' | tail -1 || true; printf '%s\n' "$args" | grep -E -- '^--?catalog=' | cut -d= -f2- || true; })   # split/= forms are alternatives: a no-match must not trip errexit/pipefail
      echo "catalog (from --catalog flag): ${flagval:-<empty — built-in defaults>}"
    else
      printf '%s\n' "$envlines" | grep '^HIVE_OPS_CATALOG=' || echo "HIVE_OPS_CATALOG unset and no --catalog flag (built-in defaults)"
    fi
  else
    echo "cannot read process cmdline/environment — catalog UNKNOWN"
  fi
  )
  ```
- **hive runtime (daemon)**: `--catalog <path> --catalog-reload-interval 1m`. **`council`** takes `--catalog <path>` only — **no** `--catalog-reload-interval` (e.g. `council --catalog ./catalog-mixed.yaml`).

Only `catalog-mixed.yaml` is checked into `repos/hive` (a missing, uncommitted `catalog-codex.yaml` referenced by `hive.service`'s `ExecStart` is the likely cause if that unit fails to start). For a manual Claude-only run, **omit `--catalog`** (built-in Claude defaults — default routing only; durable role-model policies stored in Postgres still override it); add `--catalog ./catalog-mixed.yaml` **only** when a local Ollama model is running and `OPENROUTER_API_KEY` is set.

## On-demand Runtime (`cmd/hive` verbs)

```bash
# Subshell + exit: if the canonical checkout is missing, NOTHING below may run
# from the caller's directory — a stale checkout would launch the wrong
# runtime against the live database.
( cd /Transpara/transpara-ai/repos/hive || exit

# Every runtime verb defaults --repo to the current directory. Because this
# block runs from the canonical Hive checkout, omitting --repo would make Hive
# itself the Operate target. Require an explicit clean target and reject every
# worktree backed by Hive's own git common directory.
TARGET_REPO=/absolute/path/to/clean-disposable-target
target_common=$(git -C "$TARGET_REPO" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) && target_common=$(realpath -e "$target_common" 2>/dev/null) || { echo "TARGET_REPO is not a Git repository — NOT launching"; exit 1; }
hive_common=$(git -C /Transpara/transpara-ai/repos/hive rev-parse --path-format=absolute --git-common-dir 2>/dev/null) && hive_common=$(realpath -e "$hive_common" 2>/dev/null) || { echo "canonical Hive checkout unavailable — NOT launching"; exit 1; }
[ "$target_common" != "$hive_common" ] || { echo "TARGET_REPO is Hive or one of its worktrees — NOT launching"; exit 1; }
[ -z "$(git -C "$TARGET_REPO" status --porcelain)" ] || { echo "TARGET_REPO is dirty — NOT launching"; exit 1; }
echo "Operate target: $TARGET_REPO at $(git -C "$TARGET_REPO" rev-parse HEAD)"

LOVYOU_API_KEY= go run ./cmd/hive civilization run    --human Michael --idea "…" \
       --repo "$TARGET_REPO" --store postgres://hive:hive@localhost:5432/hive   # one-shot multi-agent (--store required to persist; omit only for a throwaway in-memory run)
LOVYOU_API_KEY= go run ./cmd/hive civilization daemon --human Michael \
       --repo "$TARGET_REPO" --store postgres://hive:hive@localhost:5432/hive   # long-running (add --approve-requests --approve-roles for full autonomy)
LOVYOU_API_KEY=dev go run ./cmd/hive pipeline run        --api http://localhost:8082 --repo "$TARGET_REPO"   # Scout → Builder → Critic (needs the local API up — see "Local / Offline"; no --idea)
LOVYOU_API_KEY=dev go run ./cmd/hive role '<name>' run     --api http://localhost:8082 --repo "$TARGET_REPO"   # single agent
LOVYOU_API_KEY=dev go run ./cmd/hive council --api http://localhost:8082 --repo "$TARGET_REPO" --topic "…"   # one deliberation (add --catalog ./catalog-mixed.yaml only with Ollama + OPENROUTER_API_KEY)
# ⚠ council ALWAYS attempts to POST up to 2000 chars of the deliberation report
#   to its --api endpoint (api.New never returns nil — an empty/wrong key merely
#   fails auth AFTER the report has been transmitted). --api DEFAULTS to
#   https://transpara.ai, so ONLY the local --api pin keeps the report local.
#   Replacing any ambient remote credential with the non-secret local `dev`
#   credential as shown additionally keeps the remote bearer out of every
#   council agent's prompt (and matches the local API's --api-key dev).
#   Remote publishing requires the user's explicit authorization in the turn.
go run ./cmd/hive ingest --priority normal '<file.md>'   # registered-repo API flow: needs LOVYOU_API_KEY (set HIVE_INGEST_SKIP_REPO=1 to skip the repo bootstrap) — check `--help` before use
)
```

The runtime's webhook binds `:8081` on ALL interfaces with an unauthenticated event-writing `POST /event` (the bind address is not flag-configurable); launching the runtime on a host reachable by untrusted peers requires the user's explicit acknowledgment of that exposure or host-level firewalling.

`civilization run`/`civilization daemon` also default their Site API to `https://transpara.ai`: with an ambient `LOVYOU_API_KEY` present they enable a reconciliation loop and task-completion mirror posts against production. The blank `LOVYOU_API_KEY=` prefix above disables that client for local runs; crossing to the production Site API requires the user's explicit authorization.

Flags are **per-verb**. `civilization run`: `--human` (required), `--idea`/`--spec` (seed), `--store` (or `DATABASE_URL`), `--repo`, `--catalog`, `--approve-requests`, `--approve-roles`. `civilization daemon`: the same **except** its seed flag is `--seed-spec` (there is no `--idea`/`--spec`). ⚠ `--spec`/`--seed-spec` are NOT local-only seeds: both call the remote ingest path BEFORE the runtime starts (repository bootstrap, then a required `LOVYOU_API_KEY` and a POST to `--api`, default `https://transpara.ai`) — with the blank credential the command fails after possible bootstrap activity, and with a credential it writes remotely. Seed locally with `--idea` (run) or post-start `inject-file` (daemon); `--spec`/`--seed-spec` need explicit ingest/production authorization plus a deliberate `--api`/credential pairing. `pipeline`/`role`: `--api`, `--space`, `--repo`, `--agent-id` (no `--human`/`--idea`; for the local stack pass `--api http://localhost:8082`). `council`: `--api`, `--space`, `--repo`, `--topic`, and `--catalog` (no `--catalog-reload-interval`). Always confirm with `go run ./cmd/hive <verb> --help`.

## Operator Actions

- **Approve a proposed role** — use the CLI (it emits the `hive.role.approved` + budget events the runtime needs): `go run ./cmd/approve-role --role <name> --store postgres://hive:hive@localhost:5432/hive`. ⚠ This CLI **also allocates budget**: it emits `agent.budget.adjusted` with an initial budget of **200** for the new role (`cmd/approve-role/main.go`). Disclose that amount and get the user's explicit approval for **both** the role and the initial budget before running it. `POST /api/hive/operator-decision` only decides **draft-PR-create** authority requests — not role proposals.
- **Inject a spec/file as a task** — posts to the civilization daemon **webhook** (`:8081/event`), so the runtime must be running: `go run ./cmd/inject-file --title "…" --priority medium <file>`.

## Local / Offline

Run the pipeline against a local API with no external dependency:

```bash
( cd /Transpara/transpara-ai/repos/hive || exit
  go run ./cmd/localapi --addr localhost:8082 --api-key dev   # add --site-db to read the site DB instead of hive
)
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
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (nothing running, service already down) must not abort the block mid-way in strict shells
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -INT "$pid" 2>/dev/null;; esac; done
systemctl --user stop hive 2>/dev/null; sleep 3
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -KILL "$pid" 2>/dev/null;; esac; done
docker exec hive-postgres-1 psql -U hive -d hive -c "
    DELETE FROM telemetry_agent_snapshots;
    DELETE FROM telemetry_hive_snapshots;
    DELETE FROM telemetry_event_stream;"
)
```

## Nuclear Option (destroys the event chain)

**WARNING: erases all events / tasks / audit trail. Only for a corrupted chain.**

```bash
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (nothing running, service already down) must not abort the block mid-way in strict shells
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -INT "$pid" 2>/dev/null;; esac; done   # argv verb + comm identity; includes versioned binaries
sleep 3
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -KILL "$pid" 2>/dev/null;; esac; done
systemctl --user stop hive hive-ops-api work-server 2>/dev/null
# No name-based kills here: our units were stopped above, and ANY other
# client still attached shows up in the database refusal list below for the
# operator to stop. Quiescence is adjudicated by the DATABASE, not by enumerating client
# process names — any enumeration is incomplete (localapi, social-server,
# psql, future services), and killing by bare name could hit an instance
# serving a DIFFERENT database. `down -v` destroys the whole cluster volume,
# so require ZERO client connections across ALL databases; anything still
# connected is the operator's to stop explicitly.
conns=$(docker exec hive-postgres-1 psql -U hive -d postgres -Atc "SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'client backend' AND pid <> pg_backend_pid()" 2>/dev/null || true)   # maintenance DB: this recovery path must not depend on the hive DB it recreates
if [ "$conns" = "0" ]; then
  # The && chain is load-bearing: if the repo path is missing or unmounted, a
  # bare cd would leave the shell in the CALLER's directory and `docker compose
  # down -v` would destroy an unrelated project's volumes.
  # Postgres is left DOWN deliberately: a zero-session snapshot does not
  # prove client processes are gone, and a survivor reconnecting to a
  # freshly recreated database would find it unmigrated (clients migrate at
  # THEIR startup). Recovery is the normal Hive Up flow, run explicitly
  # after any remaining clients are stopped or deliberately restarted.
  cd /Transpara/transpara-ai/repos/hive && docker compose down -v &&
    echo "cluster volume destroyed; postgres left DOWN — stop/restart clients, then bring the stack back with the Hive Up flow"
else
  echo "postgres reports ${conns:-unreadable} live client connection(s) — NOT resetting; stop these first:"
  docker exec hive-postgres-1 psql -U hive -d postgres -Atc "SELECT datname||'  '||coalesce(application_name,'?')||'  pid='||pid FROM pg_stat_activity WHERE backend_type = 'client backend' AND pid <> pg_backend_pid()" 2>/dev/null
fi
)
```

## Common Problems

| Symptom | Cause | Fix |
|---|---|---|
| Service `activating (auto-restart)` | Postgres down — DB connection fails on start | inspect `journalctl --user -u hive-ops-api`; propose starting Postgres and wait for explicit user confirmation |
| `curl :8085` / `:8080` connection refused | that service not running / crash-looping | `systemctl --user status <svc>`; see crash-loop diagnosis |
| `401 unauthorized` from `:8085` | missing/wrong bearer | send the bearer via stdin config as in the probe examples (never in argv) |
| "Invalid API key" on all agents | `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` set | `unset` them; remove from shell profile |
| All agents `$0.000` cost | runtime just started, no iterations yet | wait 2–3 min |
| Port `:8080` conflict on start | another process bound it | `lsof -i :8080`; stop the conflict |
