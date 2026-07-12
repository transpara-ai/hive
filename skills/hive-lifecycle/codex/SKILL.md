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
- Never rely on a runtime verb's default repository. `civilization`, `pipeline`, `role`, and `council` default to the current directory; from the canonical Hive checkout that would make Hive itself the Operate target. Require an explicit, validated `--repo` target for every run.
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
__xt=0; case $- in *x*) __xt=1; set +x;; esac   # freshly derived from $-: a stale inherited __xt must never re-enable xtrace
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
__xt=0; case $- in *x*) __xt=1; set +x;; esac   # freshly derived from $-: a stale inherited __xt must never re-enable xtrace
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
elif hive_pids=$(pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) echo "$pid";; esac; done) && [ -n "$hive_pids" ]; then   # argv verb + comm identity; includes versioned binaries such as hive-test001-*
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
( set +x 2>/dev/null; printf 'header = "Authorization: Bearer %s"\n' "${WORK_API_KEY:-}" | curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -o /dev/null -w 'telemetry    /telemetry/status HTTP %{http_code}\n' -K - http://localhost:8080/telemetry/status )   # bearer via stdin config: argv is world-readable in /proc
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
# Confirm the unit actually stopped: a failed stop (or one that could not
# cancel a queued auto-restart) must be REPORTED, not discarded — otherwise
# the port check below can read clear moments before a full-autonomy relaunch.
hive_state=$(systemctl --user show hive -p ActiveState --value 2>/dev/null || true)
case "$hive_state" in inactive|failed) : ;; *) echo "hive.service is still ${hive_state:-unreadable} after stop — a queued relaunch may fire; re-run: systemctl --user stop hive, then re-check";; esac
# Identity-verified kills: comm must be exact/versioned Hive or the go-run
# driver, so a stray argv match is never signaled.
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -INT "$pid" 2>/dev/null;; esac; done
sleep 3
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -KILL "$pid" 2>/dev/null;; esac; done

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
( set +e 2>/dev/null; set +o pipefail 2>/dev/null   # BLOCK CONTRACT: expected no-match/nonzero exits (a down unit, an unreadable manager) must not abort the block mid-way in strict shells
restart_status=0   # 0 = restarted, 1 = FAILED, 2 = runtime decision PENDING the human gate — the exit status must say which
cd /Transpara/transpara-ai/repos/hive && docker compose up -d postgres
tries=60; until docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; do tries=$((tries-1)); [ "$tries" -le 0 ] && { echo "postgres not ready after 60s — inspect: docker compose logs postgres"; break; }; sleep 1; done

# Everything below is gated on Postgres readiness — restarting services or the
# runtime against a dead DB only produces crash loops; on timeout, stop here.
if docker exec hive-postgres-1 pg_isready -U hive -q 2>/dev/null; then
  systemctl --user restart work-server hive-ops-api || { echo "RESTART FAILED (work-server/hive-ops-api) — inspect: journalctl --user -u work-server -u hive-ops-api"; restart_status=1; }
  for svc in work-server hive-ops-api; do   # per-unit: is-active with multiple units exits 0 if ANY is active
    svc_state=$(systemctl --user is-active "$svc" 2>/dev/null || true)
    [ "$svc_state" = "active" ] || { echo "$svc is ${svc_state:-unreadable} after restart — do not report success"; restart_status=1; }
  done
  # Merged-property read, not is-active: a unit in `activating (auto-restart)`
  # reads as "stopped" via is-active, yet systemd will relaunch the encoded
  # (full-autonomy) command shortly — bypassing this gate. Allowlist only the
  # provably-stopped states; anything else (incl. unreadable) is MANAGED.
  # Runtime adjudication is a HUMAN gate, not shell forensics (a runbook
  # cannot reliably prove systemd transition states; posture reporting lives
  # in the tested `hive factory preflight-hive-unit` subcommand). Read the
  # merged state, REPORT it verbatim, mutate nothing:
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
    # Do NOT terminate yet — killing first loses the workload and its
    # governance flags with nothing to relaunch. Get the user's original
    # command (argv is never echoed: it may contain sensitive --idea text or
    # credential assignments) or explicit stop-without-restore authorization,
    # THEN stop with the identity-verified kill loops from Hive Down and relaunch their command.
    echo "manual runtime detected — restart needs the user's original command first:"
    printf '%s\n' "$hive_pids" | xargs -r ps -o pid=,comm= -p 2>/dev/null   # PIDs + executable names only
    [ "$restart_status" -eq 0 ] && restart_status=2   # the requested restart is INCOMPLETE pending the user's command
  else
    echo "hive runtime not running; left stopped"
  fi
else
  echo "postgres not ready — NOT restarting services or runtime; fix postgres first"
  restart_status=1
fi
exit "$restart_status"   # subshell exit: 0 restarted / 1 failed / 2 pending the human runtime gate — strict callers see the truth, the operator's terminal survives
)
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
( set +ex 2>/dev/null; set +o pipefail 2>/dev/null; printf 'header = "Authorization: Bearer %s"\n' "${HIVE_OPS_API_KEY:-dev}" | curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -K - http://localhost:8085/api/hive/operator-projection | jq . )   # bearer via stdin config (argv is world-readable in /proc); jq inside the relaxed subshell
( set +ex 2>/dev/null; set +o pipefail 2>/dev/null; printf 'header = "Authorization: Bearer %s"\n' "${WORK_API_KEY:-}" | curl -s --noproxy '*' --connect-timeout 3 --max-time 10 -K - http://localhost:8080/telemetry/status | jq . )   # bearer via stdin config (argv is world-readable in /proc)
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

# Every runtime verb defaults --repo to the current directory. Because this
# block runs from the canonical Hive checkout, omitting --repo would make Hive
# itself the Operate target. Require an explicit clean target and reject every
# worktree backed by Hive's own git common directory. An independent full clone
# has a different common directory, so choose a disposable non-Hive project;
# the common-dir guard protects the canonical checkout and its worktrees.
TARGET_REPO=/absolute/path/to/clean-disposable-target
target_common=$(git -C "$TARGET_REPO" rev-parse --path-format=absolute --git-common-dir 2>/dev/null) && target_common=$(realpath -e "$target_common" 2>/dev/null) || { echo "TARGET_REPO is not a Git repository — NOT launching"; exit 1; }
hive_common=$(git -C /Transpara/transpara-ai/repos/hive rev-parse --path-format=absolute --git-common-dir 2>/dev/null) && hive_common=$(realpath -e "$hive_common" 2>/dev/null) || { echo "canonical Hive checkout unavailable — NOT launching"; exit 1; }
[ "$target_common" != "$hive_common" ] || { echo "TARGET_REPO is Hive or one of its worktrees — NOT launching"; exit 1; }
target_status=$(git -C "$TARGET_REPO" status --porcelain 2>/dev/null) || { echo "TARGET_REPO status is unreadable — NOT launching"; exit 1; }
[ -z "$target_status" ] || { echo "TARGET_REPO is dirty — NOT launching"; exit 1; }
echo "Operate target: $TARGET_REPO at $(git -C "$TARGET_REPO" rev-parse HEAD)"

LOVYOU_API_KEY= go run ./cmd/hive civilization run \
  --human Michael \
  --idea "..." \
  --repo "$TARGET_REPO" \
  --store postgres://hive:hive@localhost:5432/hive

LOVYOU_API_KEY= go run ./cmd/hive civilization daemon \
  --human Michael \
  --repo "$TARGET_REPO" \
  --store postgres://hive:hive@localhost:5432/hive

LOVYOU_API_KEY=dev go run ./cmd/hive pipeline run --api http://localhost:8082 --repo "$TARGET_REPO"
LOVYOU_API_KEY=dev go run ./cmd/hive role '<name>' run --api http://localhost:8082 --repo "$TARGET_REPO"
LOVYOU_API_KEY=dev go run ./cmd/hive council --api http://localhost:8082 --repo "$TARGET_REPO" --topic "..."
)
```

The runtime's webhook binds `:8081` on ALL interfaces with an unauthenticated event-writing `POST /event` (the bind address is not flag-configurable); launching the runtime on a host reachable by untrusted peers requires the user's explicit acknowledgment of that exposure or host-level firewalling.

`civilization run`/`civilization daemon` also default their Site API to `https://transpara.ai`: with an ambient `LOVYOU_API_KEY` present they enable a reconciliation loop and task-completion mirror posts against production. The blank `LOVYOU_API_KEY=` prefix above disables that client for local runs; crossing to the production Site API requires the user's explicit authorization.

`hive.service` start/restart is a **protected action — a human gate, not automated forensics**. A shell runbook cannot reliably prove a systemd unit safe: environment sources, ExecStart wrappers, exec phases, and variable expansion all provide places for authority to hide — mechanical posture adjudication lives in the tested, read-only `hive factory preflight-hive-unit` subcommand (PR #277), invoked below, never in shell. Before `systemctl --user start|restart hive`:

1. Obtain the user's explicit current-turn approval naming BOTH postures: credential (an ambient `LOVYOU_API_KEY` anywhere in the unit's effective configuration means production Site integration) and autonomy (the packaged unit's `ExecStart` includes `--approve-requests --approve-roles`, so starting it RESUMES FULL AUTONOMY).
2. For a local-only runtime, do NOT start the unit at all — use the foreground `LOVYOU_API_KEY= go run ./cmd/hive civilization daemon …` form above, where both postures are explicit in the command itself. The unit's reconciliation loop begins its first cycle immediately on start, before any post-start check can run, so pre-start approval must always cover the worst-case production-connected posture.
3. After any approved start, run the verifier below to check the approved posture — verification only; it cannot undo the first reconciliation cycle.

Post-start (or post-restart) posture confirmation via the tested read-only verifier (fail-closed, whole-domain tests). Provenance differs per posture: credential posture reads the RUNNING process (`LOVYOU_API_KEY` presence/emptiness only from `/proc/<MainPID>/environ`; the value is never printed, and no secret transits a shell variable here), while autonomy posture reads the CONFIGURED merged `ExecStart` — not the live argv — so a unit edited, reloaded, or wrapper-expanded since start can run different flags than reported:

```bash
( cd /Transpara/transpara-ai/repos/hive && go run ./cmd/hive factory preflight-hive-unit )
```

Reading the report: `credential_posture=PRESENT` is PRODUCTION-CONNECTED — if the user approved the production posture this MATCHES, otherwise STOP the unit now; `ABSENT` or `EMPTY` is local-only (an empty value leaves the Site client disabled). `autonomy_posture` reports the configured merged `ExecStart`'s `--approve-requests`/`--approve-roles` flags (`FULL` = both) — configured-unit posture, not proof of the live process's flags; if the unit may have changed since start (edit, reload, wrapper), treat live autonomy as UNPROVEN and fall back to the human gate (stop the unit if in doubt). A nonzero exit with `overall=UNKNOWN` is fail-closed — unit, autonomy, or credential state was unreadable; if local-only was intended, STOP the unit.

Warning: `council` ALWAYS attempts to POST up to 2000 characters of the deliberation report to its `--api` endpoint — `api.New` never returns nil, so an empty or wrong key merely fails authentication AFTER the report has been transmitted. `--api` defaults to `https://transpara.ai`, so only the local `--api` pin keeps the report local. Replacing any ambient remote credential with the non-secret local `dev` credential as shown additionally keeps the remote bearer out of every council agent's prompt (and matches the local API's `--api-key dev`). Remote publishing requires the user's explicit authorization in the current turn.

Full autonomy is an explicit opt-in with `--approve-requests --approve-roles`. Do not add those flags unless the user explicitly authorizes that mode in the current turn.

Flag reminders:

- `civilization run`: `--human`, `--idea` or `--spec`, `--store`, `--repo`, `--catalog`, `--approve-requests`, `--approve-roles`.
- `civilization daemon`: same except the seed flag is `--seed-spec`; there is no `--idea` or `--spec`.
- Warning: `--spec`/`--seed-spec` are NOT local-only seeds — both call the remote ingest path before the runtime starts (repository bootstrap, then a required `LOVYOU_API_KEY` and a POST to `--api`, default `https://transpara.ai`); a blank credential fails after possible bootstrap activity and a real one writes remotely. Seed locally with `--idea` (run) or post-start `inject-file` (daemon); these flags need explicit ingest/production authorization plus a deliberate `--api`/credential pairing.
- `pipeline` and `role`: `--api`, `--space`, `--repo`, `--agent-id`; no `--human` or `--idea`.
- `council`: `--api`, `--space`, `--repo`, `--topic`, `--catalog`; no `--catalog-reload-interval`.
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
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -INT "$pid" 2>/dev/null;; esac; done
systemctl --user stop hive 2>/dev/null || true
sleep 3
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -KILL "$pid" 2>/dev/null;; esac; done

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
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -INT "$pid" 2>/dev/null;; esac; done
sleep 3
pgrep -f '(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)' | while read -r pid; do case "$(ps -o comm= -p "$pid")" in hive|hive-*|hive_*|go) kill -KILL "$pid" 2>/dev/null;; esac; done
systemctl --user stop hive hive-ops-api work-server 2>/dev/null || true
# The runtime loops above require BOTH a supported Hive verb in argv and an
# exact/versioned Hive or go-run comm identity. A `go test ./cmd/...` or
# `go build ./cmd/...` process has no runtime verb after its Hive path and is
# never signaled. Do not add bare-name kills for manual APIs: our units were
# stopped above, and any other client still attached shows up in the database
# refusal list below for the operator to stop.
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
| `401 unauthorized` | missing or wrong bearer token | load `/home/transpara/.config/hive/hive.env` and retry via the stdin-config probe form (never put the bearer in argv) |
| `Invalid API key` on agents | Anthropic env var overrides CLI auth | unset `ANTHROPIC_API_KEY` and `HIVE_ANTHROPIC_API_KEY` for the shell |
| agents show zero cost just after startup | runtime has not completed iterations | wait a few minutes and re-check telemetry |
| port conflict | old manual process or unrelated service | inspect `lsof -i :8080 -i :8081 -i :8085`; do not blind-kill by port |
