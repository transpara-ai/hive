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
| hive runtime | `hive.service` or manual `go run` | webhook ALL interfaces `:8081` when running (unauthenticated `POST /event`) | On-demand multi-agent loop; unit may be disabled |

Use these canonical local paths:

```bash
HIVE_REPO=/Transpara/transpara-ai/repos/hive
WORK_REPO=/Transpara/transpara-ai/repos/work
```

## Authentication

Claude CLI subscription auth uses `~/.claude/.credentials.json` and needs no Anthropic API key. Verify the shell is clean before runtime operations:

```bash
env | cut -d= -f1 | grep -i anthropic || echo "clean — no ANTHROPIC* variables set"   # nonzero grep is the GOOD case; the || arm keeps strict shells alive
```

Check names only — never print environment values into the transcript. The command should print nothing. If it prints `ANTHROPIC_API_KEY` or `HIVE_ANTHROPIC_API_KEY`, stop and ask the user before changing shell profile files. For the current shell only, unset them before runtime commands:

```bash
unset ANTHROPIC_API_KEY HIVE_ANTHROPIC_API_KEY
```

The checked-in `catalog-mixed.yaml` is a mixed-provider catalog. Claude/Codex use subscription auth and Ollama is local, but OpenRouter roles require `OPENROUTER_API_KEY`. For a Claude-CLI-only run with no provider keys, omit `--catalog` so Hive uses built-in Claude defaults. This is DEFAULT ROUTING ONLY: a durable role-model policy stored in Postgres (set via the role-policy endpoint) still overrides the built-in defaults and can route roles to Codex or API-key models. For strict provider isolation, inspect the assembly projection's model selection first and clear or repoint stored role policies.

API tokens:

- `HIVE_OPS_API_KEY`, default `dev`, for `hive-ops-api`.
- `WORK_API_KEY`, usually in `/home/transpara/.config/hive/hive.env`, for work-server telemetry routes.

Load tokens for shell probes:

```bash
case $- in *x*) __xt=1; set +x;; esac   # never trace credential assignments
set -a
. /home/transpara/.config/hive/hive.env 2>/dev/null || true
set +a
[ "${__xt:-}" = 1 ] && set -x; unset __xt
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
( cd /Transpara/transpara-ai/repos/hive || exit   # subshell: a missing checkout stops the block without touching the caller's shell
  go run ./cmd/hive --help
  go run ./cmd/hive '<verb>' --help
)
```

## Hive Status

Use this for read-only status checks:

```bash
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # STATUS BLOCK CONTRACT: nonzero exits below are INFORMATION (a down service, an absent key, a clean env) — never errors; relax strict shells for the whole block
case $- in *x*) __xt=1; set +x;; esac   # never trace credential assignments
set -a
. /home/transpara/.config/hive/hive.env 2>/dev/null || true
set +a
[ "${__xt:-}" = 1 ] && set -x; unset __xt

echo "=== services ==="
systemctl --user is-active work-server hive-ops-api
systemctl --user --no-pager status work-server hive-ops-api | grep -E 'Active:|Main PID:'

echo "=== hive runtime ==="
# Merged-property read, not is-active: `activating (auto-restart)` returns
# nonzero from is-active yet systemd WILL relaunch the unit shortly.
hive_state=$(systemctl --user show hive -p ActiveState --value 2>/dev/null)
if [ "$hive_state" = "active" ]; then
  echo "hive.service: active"
elif [ "$hive_state" != "inactive" ] && [ "$hive_state" != "failed" ]; then
  echo "hive.service: ${hive_state:-unreadable} — pending auto-restart or transition; treat as a MANAGED runtime"
elif hive_pids=$(pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) echo "$pid";; esac; done) && [ -n "$hive_pids" ]; then   # comm-verified like the kill loops: argv mentions alone are not a runtime
  echo "manual runtime: RUNNING"
  printf '%s\n' "$hive_pids" | xargs -r ps -o pid=,comm= -p 2>/dev/null   # PIDs + executable names only — full argv may contain sensitive --idea text or credential assignments
else
  echo "runtime: stopped"
fi

ss -tlnp 2>/dev/null | grep -q ':8081 ' && echo "hive webhook :8081: listening" || echo "hive webhook :8081: not listening"

echo "=== postgres ==="
docker ps --filter name=hive-postgres-1 --format '{{.Names}} {{.Status}}'
docker exec hive-postgres-1 psql -U hive -d hive -c 'SELECT count(*) FROM events' 2>/dev/null || echo "DB unreachable"

echo "=== endpoint health ==="
curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'work-server  /health           HTTP %{http_code}\n' http://localhost:8080/health
curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'hive-ops-api /health           HTTP %{http_code}\n' http://localhost:8085/health
( set +x 2>/dev/null; curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' -H "Authorization: Bearer ${WORK_API_KEY:-}" http://localhost:8080/telemetry/status )
)
```

If `systemctl --user is-active` reports `activating` or `auto-restart`, inspect logs read-only first:

```bash
journalctl --user -u hive-ops-api -n 40 --no-pager
```

Starting Postgres is a mutating recovery action, not part of status — run it only after the user explicitly confirms:

```bash
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done
```

## Hive Up

Start only when the user explicitly asks to bring Hive up. Start Postgres first and wait until it accepts connections:

```bash
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
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
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (nothing running, service already down) must not abort the block mid-way in strict shells
systemctl --user stop hive 2>/dev/null || true
# Identity-verified kills: comm must be hive|go, so a stray argv match (an
# editor, grep, or agent session mentioning the pattern) is never signaled.
pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) kill -INT "$pid" 2>/dev/null;; esac; done
sleep 3
pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) kill -KILL "$pid" 2>/dev/null;; esac; done

systemctl --user stop hive-ops-api work-server

# Stray MANUAL services are the OPERATOR's to stop — they know what they
# launched, and a name-only kill could hit an unrelated binary that merely
# shares the name. LIST candidates (pid + executable) and leave the decision
# with the human; the port check below surfaces anything still bound:
for name in work-server hive-ops-api; do
  pgrep -x "$name" | while read -r pid; do printf 'manual %s candidate: pid=%s exe=%s\n' "$name" "$pid" "$(readlink /proc/"$pid"/exe 2>/dev/null || echo unreadable)"; done
done

lsof -i :8080 -i :8081 -i :8085 2>/dev/null && echo "^ a port is still bound; inspect before clearing it" || echo "ports clear"
)
```

Full Postgres stop, only when explicitly requested:

```bash
cd /Transpara/transpara-ai/repos/hive && docker compose down
```

## Hive Restart

Restart means true down-to-up for APIs after Postgres is available. Preserve manual runtime command flags instead of guessing them.

```bash
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done

# Everything below is gated on Postgres readiness — restarting services or the
# runtime against a dead DB only produces crash loops; on timeout, stop here.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  systemctl --user restart work-server hive-ops-api
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
  elif hive_pids=$(pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) echo "$pid";; esac; done) && [ -n "$hive_pids" ]; then   # comm-verified like the kill loops
    # Do NOT terminate yet — killing first loses the workload and its
    # governance flags with nothing to relaunch. Get the user's original
    # command (argv is never echoed: it may contain sensitive --idea text or
    # credential assignments) or explicit stop-without-restore authorization,
    # THEN stop with the identity-verified kill loops from Hive Down and relaunch their command.
    echo "manual runtime detected — restart needs the user's original command first:"
    printf '%s\n' "$hive_pids" | xargs -r ps -o pid=,comm= -p 2>/dev/null   # PIDs + executable names only
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
( set +x 2>/dev/null   # secret-bearing expansions below must never reach an xtrace transcript
pid=$(systemctl --user show hive-ops-api -p MainPID --value 2>/dev/null || true)
if [ "${pid:-0}" -gt 0 ] 2>/dev/null && envlines=$(tr '\0' '\n' </proc/"$pid"/environ 2>/dev/null) && [ -n "$envlines" ]; then
  actor=$(printf '%s\n' "$envlines" | grep '^HIVE_OPS_HUMAN_ACTOR=' | head -1 | cut -d= -f2- || true)   # no match = read-only, the expected default: never trip errexit/pipefail
  if [ -n "$actor" ]; then
    echo "HIVE_OPS_HUMAN_ACTOR set and non-empty — WRITER MODE possible (an invalid actor id still yields read-only at startup; check journal for 'HIVE_OPS_HUMAN_ACTOR invalid')"
  else
    echo "HIVE_OPS_HUMAN_ACTOR absent or empty — read-only (opsWriterOptions requires a non-empty valid actor id)"
  fi
else
  echo "cannot read process environment — mode UNKNOWN; do not POST"
fi
)
```

The probe reads this one variable's value (an operator actor id, not a secret) because presence alone over-claims: `opsWriterOptions` stays read-only for an empty or invalid id. NUL separators are converted to newlines **inside** the substitution (`tr` is the sole command, so a failed `/proc` read fails the whole condition — no pipeline masks it, and no NUL bytes are lost to command substitution, which strips them). Service down or unreadable — the mode is unknown; do not POST.

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
( set +ex 2>/dev/null; set +o pipefail 2>/dev/null; curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -H "Authorization: Bearer ${HIVE_OPS_API_KEY:-dev}" http://localhost:8085/api/hive/operator-projection | jq . )   # jq INSIDE the subshell: the relaxed pipefail must cover the whole pipeline
( set +ex 2>/dev/null; set +o pipefail 2>/dev/null; curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -H "Authorization: Bearer ${WORK_API_KEY:-}" http://localhost:8080/telemetry/status | jq . )
```

## Model Catalog

- `hive-ops-api` uses `HIVE_OPS_CATALOG`. Verify the actual resolved path from the running process's effective environment — this variable's value only (a filepath, not a secret; unrelated values are never printed; `/proc` unreadable = UNKNOWN, fail closed):

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
- Hive runtime uses `--catalog <path> --catalog-reload-interval 1m`.
- `council` accepts `--catalog <path>` only; do not pass `--catalog-reload-interval` to `council`.
- If avoiding provider keys, omit `--catalog` for built-in Claude defaults — default routing only; durable role-model policies stored in Postgres still override it.

## On-demand Runtime

Runtime execution is not a status check. Run only after explicit user request.

```bash
# Subshell + exit: if the canonical checkout is missing, NOTHING below may run
# from the caller's directory — a stale checkout would launch the wrong
# runtime against the live database.
( cd /Transpara/transpara-ai/repos/hive || exit

LOVYOU_API_KEY= go run ./cmd/hive civilization run \
  --human Michael \
  --idea "..." \
  --store postgres://hive:hive@localhost:5432/hive

LOVYOU_API_KEY= go run ./cmd/hive civilization daemon \
  --human Michael \
  --store postgres://hive:hive@localhost:5432/hive

LOVYOU_API_KEY=dev go run ./cmd/hive pipeline run --api http://localhost:8082 --repo .
LOVYOU_API_KEY=dev go run ./cmd/hive role '<name>' run --api http://localhost:8082 --repo .
LOVYOU_API_KEY=dev go run ./cmd/hive council --api http://localhost:8082 --topic "..."
)
```

The runtime's webhook binds `:8081` on ALL interfaces with an unauthenticated event-writing `POST /event` (the bind address is not flag-configurable); launching the runtime on a host reachable by untrusted peers requires the user's explicit acknowledgment of that exposure or host-level firewalling.

`civilization run`/`civilization daemon` also default their Site API to `https://transpara.ai`: with an ambient `LOVYOU_API_KEY` present they enable a reconciliation loop and task-completion mirror posts against production. The blank `LOVYOU_API_KEY=` prefix above disables that client for local runs; crossing to the production Site API requires the user's explicit authorization.

`hive.service` start/restart is a **protected action — a human gate, not automated forensics**. A shell runbook cannot reliably prove a systemd unit safe: environment sources, ExecStart wrappers, exec phases, and variable expansion all provide places for authority to hide, and a mechanical verifier belongs in a tested Go subcommand (tracked as separate work), not here. Before `systemctl --user start|restart hive`:

1. Obtain the user's explicit current-turn approval naming BOTH postures: credential (an ambient `LOVYOU_API_KEY` anywhere in the unit's effective configuration means production Site integration) and autonomy (the packaged unit's `ExecStart` includes `--approve-requests --approve-roles`, so starting it RESUMES FULL AUTONOMY).
2. For a local-only runtime, do NOT start the unit at all — use the foreground `LOVYOU_API_KEY= go run ./cmd/hive civilization daemon …` form above, where both postures are explicit in the command itself. The unit's reconciliation loop begins its first cycle immediately on start, before any post-start check can run, so pre-start approval must always cover the worst-case production-connected posture.
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

Warning: `council` ALWAYS attempts to POST up to 2000 characters of the deliberation report to its `--api` endpoint — `api.New` never returns nil, so an empty or wrong key merely fails authentication AFTER the report has been transmitted. `--api` defaults to `https://transpara.ai`, so only the local `--api` pin keeps the report local. Replacing any ambient remote credential with the non-secret local `dev` credential as shown additionally keeps the remote bearer out of every council agent's prompt (and matches the local API's `--api-key dev`). Remote publishing requires the user's explicit authorization in the current turn.

Full autonomy is an explicit opt-in with `--approve-requests --approve-roles`. Do not add those flags unless the user explicitly authorizes that mode in the current turn.

Flag reminders:

- `civilization run`: `--human`, `--idea` or `--spec`, `--store`, `--repo`, `--catalog`, `--approve-requests`, `--approve-roles`.
- `civilization daemon`: same except the seed flag is `--seed-spec`; there is no `--idea` or `--spec`.
- Warning: `--spec`/`--seed-spec` are NOT local-only seeds — both call the remote ingest path before the runtime starts (repository bootstrap, then a required `LOVYOU_API_KEY` and a POST to `--api`, default `https://transpara.ai`); a blank credential fails after possible bootstrap activity and a real one writes remotely. Seed locally with `--idea` (run) or post-start `inject-file` (daemon); these flags need explicit ingest/production authorization plus a deliberate `--api`/credential pairing.
- `pipeline` and `role`: `--api`, `--space`, `--repo`, `--agent-id`; no `--human` or `--idea`.
- Always confirm with `go run ./cmd/hive <verb> --help` when composing a new invocation.

## Operator Actions

Approving proposed roles should use the CLI because it emits the role approval and budget events the runtime needs. Warning: this CLI also allocates budget — it emits `agent.budget.adjusted` with an initial budget of 200 for the new role (`cmd/approve-role/main.go`). Disclose that amount and obtain the user's explicit approval for both the role and the initial budget before running it:

```bash
( cd /Transpara/transpara-ai/repos/hive || exit
  go run ./cmd/approve-role --role '<name>' --store postgres://hive:hive@localhost:5432/hive
)
```

Injecting a spec/file requires the runtime webhook to be running:

```bash
( cd /Transpara/transpara-ai/repos/hive || exit
  go run ./cmd/inject-file --title "..." --priority medium '<file>'
)
```

## Local Offline API

For local pipeline/role commands with no external dependency:

```bash
( cd /Transpara/transpara-ai/repos/hive || exit
  go run ./cmd/localapi --addr localhost:8082 --api-key dev
)
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
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (nothing running, service already down) must not abort the block mid-way in strict shells
pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) kill -INT "$pid" 2>/dev/null;; esac; done
systemctl --user stop hive 2>/dev/null || true
sleep 3
pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) kill -KILL "$pid" 2>/dev/null;; esac; done

docker exec hive-postgres-1 psql -U hive -d hive -c "
  DELETE FROM telemetry_agent_snapshots;
  DELETE FROM telemetry_hive_snapshots;
  DELETE FROM telemetry_event_stream;"
)
```

## Nuclear Option

This destroys the event chain. Do not run it unless the user explicitly asks for a destructive database reset and acknowledges that events, tasks, and audit trail are erased.

```bash
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (nothing running, service already down) must not abort the block mid-way in strict shells
pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) kill -INT "$pid" 2>/dev/null;; esac; done
sleep 3
pgrep -f '[h]ive (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|go) kill -KILL "$pid" 2>/dev/null;; esac; done
systemctl --user stop hive hive-ops-api work-server 2>/dev/null || true
# Manual APIs also write the chain — sweep them, then GATE the reset on
# quiescence. The broad argv pattern is safe here: a false match only ABORTS.
# Name-exact (comm) kills cover go-run children AND direct binaries; the
# go-run driver exits with its child. No argv matching: a `go test ./cmd/...`
# or `go build ./cmd/...` job must never be killed by a lifecycle sweep.
# No name-based kills here: our units were stopped above, and ANY other
# client still attached shows up in the database refusal list below for the
# operator to stop.
# Quiescence is adjudicated by the DATABASE, not by enumerating client
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

| Symptom | Likely cause | First action |
|---|---|---|
| `curl` to `:8085` or `:8080` is refused | service stopped or crash-looping | `systemctl --user status <service>` |
| Service is `activating (auto-restart)` | Postgres down or unreachable | inspect `journalctl --user -u hive-ops-api`; propose starting Postgres and wait for explicit user confirmation |
| `401 unauthorized` | missing or wrong bearer token | load `/home/transpara/.config/hive/hive.env` and retry |
| `Invalid API key` on agents | Anthropic env var overrides CLI auth | unset `ANTHROPIC_API_KEY` and `HIVE_ANTHROPIC_API_KEY` for the shell |
| agents show zero cost just after startup | runtime has not completed iterations | wait a few minutes and re-check telemetry |
| port conflict | old manual process or unrelated service | inspect `lsof -i :8080 -i :8081 -i :8085`; do not blind-kill by port |
