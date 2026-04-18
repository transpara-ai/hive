---
name: hive-lifecycle
description: "Manage the lovyou-ai hive stack lifecycle on nucbuntu. Use whenever the user says 'hive up', 'hive down', 'hive run', 'hive status', 'hive restart', 'hive help', 'start the hive', 'stop the hive', 'run the hive', 'run agents on', 'hive idea', 'run pipeline', 'run builder', 'run scout', 'is the hive running', 'kill the hive', 'bounce the hive', or any variation of starting, stopping, restarting, running, or checking the status of the hive runtime, work-server, or postgres. Also trigger when the user mentions stuck agents, frozen hive, or needs a clean slate."
---

# Hive Lifecycle Management

Commands for starting, stopping, restarting, running, and monitoring the lovyou-ai hive stack on nucbuntu.

## Hive Help

When the user says `hive help`, `hive --help`, or `hive commands`, print this summary and stop — do not execute anything:

```
Hive Lifecycle Commands:

  hive up                  Start postgres + work-server + hive runtime
  hive down                Graceful shutdown (SIGINT → force kill → verify)
  hive restart             Down then up
  hive status              Check all components (docker, processes, ports, auth)
  hive run <idea>          Start hive with an idea (legacy multi-agent mode)
  hive run --pipeline      Scout → Builder → Critic pipeline (loops at --interval, default 30m)
  hive run --role <role>   Single agent mode (builder/scout/critic/monitor)
  hive run --council       Convene all agents for deliberation

  hive ingest <file>       Ingest a spec markdown file as a board task
  hive tasks               List open tasks on the board
  hive agents              Show agent roster with models and session state
  hive budget <agent> <n>  Override an agent's iteration budget at runtime
  hive council [<topic>]   Convene all agents for deliberation on a topic

  hive monitor             Live telemetry, phases, event stream
  hive logs                Tail hive + work-server logs
  hive approve <role>      Manually approve a pending role proposal
  hive inject <task>       Send work to running hive via webhook

  hive clean               Nuke telemetry, keep event chain
  hive nuke                Full reset (destroys chain — use with caution)

  hive help                This message

Environment (not CLI flags):
  OPEN_BRAIN_URL           Checkpoint recovery via Open Brain
  OPEN_BRAIN_KEY           Open Brain MCP access key
  CHECKPOINT_STALENESS     Max thought age before cold-start (default: 2h)
  CHECKPOINT_HEARTBEAT_INTERVAL  Iterations between heartbeats (default: 10)
```

## Stack Components

| Component | Process | Repo | How It Runs |
|-----------|---------|------|-------------|
| **postgres** | Docker container `hive-postgres-1` | docker-compose.yml | `docker compose up -d postgres` |
| **pgadmin** | Docker container | docker-compose.yml | `docker compose up -d pgadmin` |
| **hive** | Go binary (foreground or background) | lovyou-ai-hive | `go run ./cmd/hive` |
| **work-server** | Go binary (foreground or background) | lovyou-ai-work | `go run ./cmd/work-server` |
| **localapi** | Go binary (background) | lovyou-ai-hive | `go run ./cmd/localapi` |
| **dashboard** | Static HTML (no process) | lovyou-ai-summary | Served via GitHub Pages or file server |

## Paths

```
HIVE_REPO=~/transpara-ai/repos/lovyou-ai-hive
WORK_REPO=~/transpara-ai/repos/lovyou-ai-work
COMPOSE_DIR=~/transpara-ai/repos/lovyou-ai-hive   # or wherever docker-compose.yml lives
```

Verify these paths before running commands. If they differ, adjust accordingly.

## Authentication

The hive uses the `claude-cli` provider, which authenticates via the Claude Max subscription stored in `~/.claude/`. **No API key is needed.**

**CRITICAL — DO NOT set any of these environment variables:**
- `ANTHROPIC_API_KEY` — poisons both Claude Code and the hive
- `HIVE_ANTHROPIC_API_KEY` — legacy variable, no longer used

If either is set in `.bashrc`, `.profile`, or `.env`, **remove it**. The `claude` CLI binary handles authentication automatically using the Max subscription. Setting any API key overrides the working auth and causes "Invalid API key" errors for every agent.

Before starting the hive, verify the environment is clean:

```bash
env | grep -i anthropic
# Should return NOTHING. If it returns anything, unset it.
```

---

## Hive Up

Start the full stack in dependency order. **Always kill zombies first** — previous runs
may leave orphaned processes holding ports 8080/8081.

```bash
# 0. Kill zombies from previous runs
pkill -f "cmd/hive" 2>/dev/null
# Matches both old --human form and new "civilization" form via cmd/hive
pkill -f "/hive --human" 2>/dev/null
pkill -f "cmd/work-server" 2>/dev/null
pkill -f "work-server-fre" 2>/dev/null
sleep 2
# Verify ports are free
ss -tlnp 2>/dev/null | grep -E "8080|8081" && echo "WARNING: ports still held" || echo "Ports clear"

# 1. Verify no API keys are set (CRITICAL)
if [ -n "${ANTHROPIC_API_KEY:-}" ] || [ -n "${HIVE_ANTHROPIC_API_KEY:-}" ]; then
    echo "ERROR: Anthropic API key detected in environment. Unset it first."
    echo "  unset ANTHROPIC_API_KEY HIVE_ANTHROPIC_API_KEY"
    return 1 2>/dev/null || exit 1
fi

# 2. Postgres (if not already running)
cd $COMPOSE_DIR
docker compose up -d postgres
# Wait for healthy
docker compose ps  # should show "healthy"

# 3. Work-server (background, logs to file)
# DATABASE_URL connects it to the same postgres as the hive (required for telemetry).
# WORK_API_KEY=dev is a stable key so the dashboard can connect with a known token.
cd $WORK_REPO
WORK_HUMAN=Michael WORK_API_KEY=dev DATABASE_URL=postgres://hive:hive@localhost:5432/hive nohup go run ./cmd/work-server > /tmp/work-server.log 2>&1 &
echo $! > /tmp/work-server.pid
echo "Work-server started (PID: $(cat /tmp/work-server.pid))"

# 4. Hive (foreground — so you can see output and Ctrl+C to stop)
cd $HIVE_REPO
go run ./cmd/hive civilization run \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

To run the hive in the background instead:

```bash
cd $HIVE_REPO
nohup go run ./cmd/hive civilization run \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here" \
    > /tmp/hive.log 2>&1 &
echo $! > /tmp/hive.pid
echo "Hive started (PID: $(cat /tmp/hive.pid))"
```

### Hive Up with Auto-Approve

Pass `--approve-requests` to auto-approve authority requests (file writes, git ops).
Pass `--approve-roles` to auto-approve role proposals (bypasses Guardian `/approve`).

```bash
go run ./cmd/hive civilization run \
    --human Michael \
    --approve-requests \
    --approve-roles \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

When `--approve-roles` is set, the watcher automatically emits `hive.role.approved` and
`agent.budget.adjusted` for each `hive.role.proposed` event — no Guardian or Allocator
involvement needed. New agents spawn within 5 seconds of being proposed.

### Hive Up with Loop

Use `civilization daemon` to keep agents alive when idle — they block on the bus instead of
quiescing. Useful for long-running hives where work arrives via the webhook.

```bash
go run ./cmd/hive civilization daemon \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive
```

### Full Autonomy (growth + daemon + approve)

All flags combined for maximum autonomy:

```bash
go run ./cmd/hive civilization daemon \
    --human Michael \
    --approve-requests \
    --approve-roles \
    --store postgres://hive:hive@localhost:5432/hive
```

Then inject work via curl:

```bash
curl -X POST http://localhost:8081/event \
  -H "Content-Type: application/json" \
  -d '{"id":"seed-001","node_title":"Build X","op":"intend","actor":"Michael","actor_kind":"human","payload":{"description":"details"}}'
```

### Verification After Up

```bash
# Postgres
docker exec hive-postgres-1 psql -U hive -d hive -c "SELECT COUNT(*) FROM events"

# Work-server
curl -s http://localhost:8080/health | jq .

# Telemetry API
curl -s -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/phases | jq .

# Dashboard
echo "Open: http://nucbuntu:8080/telemetry/"
```

---

## Hive Run (Runner / Pipeline Mode)

Run individual agents or the Scout-Builder-Critic pipeline against lovyou.ai tasks.
This is an alternative to the legacy multi-agent runtime above.

```bash
# Single agent — works one task matching its role
go run ./cmd/hive role builder run --space hive

# Full pipeline — loops at --interval (default 30m)
go run ./cmd/hive pipeline daemon --space hive

# Pipeline with custom interval
go run ./cmd/hive pipeline daemon --interval 15m --space hive

# Single pipeline cycle (no loop)
go run ./cmd/hive pipeline run --space hive

# Council — deliberation session
go run ./cmd/hive council --topic "Should we add a caching layer?"
```

### Local Pipeline (no lovyou.ai dependency)

Requires the local API server (`cmd/localapi`) running alongside postgres.

```bash
# 1. Start localapi (if not already running)
cd $HIVE_REPO
nohup go run ./cmd/localapi/ > /tmp/localapi.log 2>&1 &
echo $! > /tmp/localapi.pid
echo "localapi started (PID: $(cat /tmp/localapi.pid))"

# Verify
curl -s http://localhost:8082/health  # → "ok"

# 2. Seed a task for the pipeline to work on
curl -s -X POST http://localhost:8082/app/hive/op \
  -H "Authorization: Bearer dev" \
  -H "Content-Type: application/json" \
  -d '{"op":"intend","kind":"task","title":"Build a habit tracker CLI","description":"Go CLI: habit add, habit check, habit streak. SQLite storage. Include tests.","priority":"high"}'

# 3. Run the pipeline against local API
go run ./cmd/hive pipeline daemon \
    --api http://localhost:8082 \
    --space hive \
    --store postgres://hive:hive@localhost:5432/hive \
    --pr
```

No `LOVYOU_API_KEY` needed — defaults to "dev" for localhost.

---

## Hive Down

Graceful shutdown in reverse dependency order. **Always verify zombies are dead** —
`go run` spawns child processes that survive parent kills.

```bash
# 1. Stop hive (graceful — sends SIGINT, agents run retirement ceremonies)
pkill -INT -f "cmd/hive" 2>/dev/null
# Matches both old --human form and new "civilization" form via cmd/hive
pkill -INT -f "/hive --human" 2>/dev/null
sleep 5

# 2. Kill any zombies (go run children that survived SIGINT)
pkill -9 -f "cmd/hive" 2>/dev/null
# Matches both old --human form and new "civilization" form via cmd/hive
pkill -9 -f "/hive --human" 2>/dev/null
sleep 2

# 3. Stop work-server
pkill -f "cmd/work-server" 2>/dev/null
pkill -f "work-server-fre" 2>/dev/null
sleep 2

# 4. Verify everything is dead and ports are free
pgrep -a -f "cmd/hive\|/hive --human" 2>/dev/null | grep -v pgrep && echo "WARNING: hive zombies remain" || echo "Hive: stopped"
pgrep -a -f "cmd/work-server\|work-server-fre" 2>/dev/null | grep -v pgrep && echo "WARNING: work-server zombies remain" || echo "Work-server: stopped"
ss -tlnp 2>/dev/null | grep -E "8080|8081" && echo "WARNING: ports still held — kill PIDs shown above" || echo "Ports 8080/8081: clear"
rm -f /tmp/hive.pid /tmp/work-server.pid

# 5. Postgres — usually leave running (data persists)
# Only stop if you want a full shutdown:
# cd $COMPOSE_DIR && docker compose down
```

### Force Kill (if graceful fails)

If zombie processes survive, kill by PID from `ss -tlnp` output:

```bash
# Find PIDs holding the ports
ss -tlnp | grep -E "8080|8081"
# Kill them directly
kill -9 <PID> <PID>
```

---

## Hive Restart

Full Hive Down then Hive Up. Always kill zombies between.

```bash
# 1. Kill everything (graceful then force)
pkill -INT -f "cmd/hive" 2>/dev/null; pkill -INT -f "/hive --human" 2>/dev/null
sleep 5
pkill -9 -f "cmd/hive" 2>/dev/null; pkill -9 -f "/hive --human" 2>/dev/null
pkill -f "cmd/work-server" 2>/dev/null; pkill -f "work-server-fre" 2>/dev/null
sleep 2

# 2. Verify clean
ss -tlnp 2>/dev/null | grep -E "8080|8081" && echo "WARNING: ports held" || echo "Clean"

# 3. Start fresh (same commands as Hive Up steps 3-4)
cd $WORK_REPO
WORK_HUMAN=Michael WORK_API_KEY=dev DATABASE_URL=postgres://hive:hive@localhost:5432/hive nohup go run ./cmd/work-server > /tmp/work-server.log 2>&1 &
echo $! > /tmp/work-server.pid

cd $HIVE_REPO
go run ./cmd/hive civilization run \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "your task description here"
```

---

## Hive Status

```bash
echo "=== Docker ==="
docker compose ps 2>/dev/null || echo "Docker compose not found"

echo ""
echo "=== Hive Process ==="
pgrep -a -f "cmd/hive" || echo "Not running"

echo ""
echo "=== Work-Server Process ==="
pgrep -a -f "cmd/work-server" || echo "Not running"

echo ""
echo "=== Postgres Connection ==="
docker exec hive-postgres-1 psql -U hive -d hive -c "SELECT COUNT(*) as events FROM events" 2>/dev/null || echo "Cannot connect"

echo ""
echo "=== Telemetry API ==="
curl -s -o /dev/null -w "HTTP %{http_code}" http://localhost:8080/telemetry/health 2>/dev/null || echo "Not responding"

echo ""
echo "=== Agent Count ==="
docker exec hive-postgres-1 psql -U hive -d hive -t -c "SELECT COUNT(DISTINCT agent_role) FROM telemetry_agent_snapshots WHERE recorded_at > now() - interval '5 minutes'" 2>/dev/null || echo "No recent snapshots"

echo ""
echo "=== Local API ==="
curl -s -o /dev/null -w "HTTP %{http_code}" http://localhost:8082/health 2>/dev/null || echo "Not running"

echo ""
echo "=== Auth Check ==="
if [ -n "${ANTHROPIC_API_KEY:-}" ] || [ -n "${HIVE_ANTHROPIC_API_KEY:-}" ]; then
    echo "WARNING: API key in environment — will override Max subscription auth!"
else
    echo "Clean — using Claude Max subscription via claude-cli"
fi
```

---

## Monitoring a Run

While the hive is running:

```bash
# Live telemetry (agent states, iterations, cost)
curl -s -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/agents | jq .

# Phase progress
curl -s -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/phases | jq .

# Event stream (last 20 events)
curl -s -H "Authorization: Bearer $WORK_API_KEY" http://localhost:8080/telemetry/events | jq '.[-20:]'

# Dashboard
echo "Open: http://nucbuntu:8080/telemetry/"
```

### After a Run

```bash
# Check what was built
cd $HIVE_REPO
git log --oneline -10

# Check task completion
docker exec hive-postgres-1 psql -U hive -d hive -t -c \
    "SELECT status, COUNT(*) FROM tasks GROUP BY status"

# Total cost
docker exec hive-postgres-1 psql -U hive -d hive -t -c \
    "SELECT SUM(cost_usd) FROM telemetry_agent_snapshots WHERE recorded_at > now() - interval '1 hour'"
```

---

## Flag Reference

### `civilization run` (multi-agent, one-shot)

| Flag | Default | Purpose |
|------|---------|---------|
| `--human` | (required) | Human operator name — always `Michael` |
| `--idea` | "" | Freeform seed |
| `--spec` | "" | Path to markdown spec file (POSTed via ingest channel) |
| `--store` | in-memory | Postgres DSN for persistence |
| `--repo` | current dir | Path to repo for Implementer's Operate |
| `--approve-requests` | false | Auto-approve authority requests |
| `--approve-roles` | false | Auto-approve role proposals |
| `--space`, `--api` | `hive`, `https://lovyou.ai` | lovyou.ai targeting |

### `civilization daemon` (multi-agent, long-running)

Same flags as `civilization run` except `--idea` becomes `--seed-spec PATH` (optional initial spec; daemon survives past quiescence regardless).

### `pipeline run` / `pipeline daemon`

| Flag | Default | Purpose | Where |
|------|---------|---------|-------|
| `--space` | `hive` | lovyou.ai space slug | both |
| `--api` | `https://lovyou.ai` | lovyou.ai API base URL | both |
| `--repo` | current dir | Path to repo | both |
| `--repos` | "" | Named repos: `name=path,...` | both |
| `--budget` | 10.0 | Daily budget in USD | both |
| `--agent-id` | "" | Agent's lovyou.ai user ID | both |
| `--store` | in-memory | Postgres DSN | both |
| `--pr` | false | Branch + PR instead of pushing to main | both |
| `--worktrees` | false | Per-task worktrees | both |
| `--auto-clone` | false | Clone missing repos | both |
| `--interval` | 30m | Cycle interval | `daemon` only |

### `role <name> run` / `role <name> daemon`

| Flag | Default | Purpose |
|------|---------|---------|
| `--space`, `--api`, `--repo`, `--agent-id`, `--budget`, `--pr` | (as above) | Same as pipeline |

`run` runs a single task with throttling disabled. `daemon` runs continuously with throttled phases.

### `council`

| Flag | Default | Purpose |
|------|---------|---------|
| `--topic` | "" | Focus question |
| `--space`, `--api`, `--repo`, `--budget` | (as pipeline) | |

### `ingest <file>`

| Flag | Default | Purpose |
|------|---------|---------|
| `--space`, `--api` | (as pipeline) | |
| `--priority` | `high` | Task priority: low\|medium\|high\|critical |

### Environment Variables (not CLI flags)

| Variable | Default | Purpose |
|----------|---------|---------|
| `OPEN_BRAIN_URL` | (none) | Open Brain MCP endpoint for checkpoint recovery |
| `OPEN_BRAIN_KEY` | (none) | Open Brain MCP access key |
| `CHECKPOINT_STALENESS` | 2h | Max thought age before cold-start |
| `CHECKPOINT_HEARTBEAT_INTERVAL` | 10 | Iterations between heartbeat events on chain |
| `DATABASE_URL` | (none) | Alternative to `--store` |

---

## Clean Slate (nuke telemetry, keep chain)

When telemetry data from a stuck or zombie run is polluting the dashboard:

```bash
# Stop the hive first
pkill -INT -f "cmd/hive" 2>/dev/null
sleep 5

# Clear telemetry tables (event chain is untouched)
docker exec hive-postgres-1 psql -U hive -d hive -c "
    DELETE FROM telemetry_agent_snapshots;
    DELETE FROM telemetry_hive_snapshots;
    DELETE FROM telemetry_event_stream;
"
echo "Telemetry cleared. Chain intact."

# Restart hive — fresh telemetry will populate
```

## Nuclear Option (full reset including chain)

**WARNING: This destroys the entire event history. All agent events, all work tasks, all audit trail. Only use if the chain is corrupted or you want a genuinely fresh start.**

```bash
pkill -INT -f "cmd/hive" 2>/dev/null
pkill -f "cmd/work-server" 2>/dev/null
sleep 5

cd $COMPOSE_DIR
docker compose down -v   # -v removes the postgres volume
docker compose up -d postgres
# Wait for healthy, then restart hive + work-server
```

---

## Logs

```bash
# Hive (if backgrounded)
tail -f /tmp/hive.log

# Work-server (if backgrounded)
tail -f /tmp/work-server.log

# Postgres
docker compose logs -f postgres
```

---

## Approve Role

When the Spawner proposes a new role, it requires three events before the agent spawns:
1. `hive.role.proposed` — emitted by Spawner (automatic)
2. `hive.role.approved` — emitted by Guardian (`/approve` command) or human via CLI
3. `agent.budget.adjusted` — emitted by Allocator or human via CLI

The Guardian's prompt includes role governance instructions and will `/approve` or `/reject`
proposals automatically. If the Guardian doesn't act (budget exhausted, missed event), approve
manually with the `approve-role` CLI tool:

```bash
cd $HIVE_REPO
go run ./cmd/approve-role \
    --role researcher \
    --store postgres://hive:hive@localhost:5432/hive \
    --reason "Human operator approved: filling CTO-identified gap"
```

This emits both `hive.role.approved` and `agent.budget.adjusted` (budget=200) in one shot.
The watcher polls every 5 seconds — the new agent spawns on the next poll cycle.

To check pending proposals:

```bash
docker exec lovyou-ai-hive-postgres-1 psql -U hive -d hive -t -c \
    "SELECT content_json->>'name' as role, content_json->>'reason' as reason
     FROM events WHERE event_type = 'hive.role.proposed'
     AND content_json->>'name' NOT IN (
       SELECT content_json->>'name' FROM events WHERE event_type = 'hive.role.approved'
     )"
```

---

## Inject Work

Send tasks to a running hive via the webhook listener on `:8081`.
The `op: "intend"` creates a task that agents discover and claim.

```bash
curl -X POST http://localhost:8081/event \
  -H "Content-Type: application/json" \
  -d '{
    "id": "task-001",
    "node_title": "Build a CLI tool that tracks daily habits",
    "op": "intend",
    "actor": "Michael",
    "actor_kind": "human",
    "payload": {
      "description": "A Go CLI: habit add, habit check, habit streak. SQLite storage. Include tests."
    }
  }'
```

Supported ops: `intend` (creates task), `respond`, `express`, `assert`, `progress` (emitted as `hive.site.*` events). The `assign` and `complete` ops are stubbed.

Verify the listener is up first:

```bash
curl -s http://localhost:8081/health
# Should return: ok
```

---

## Common Problems

| Symptom | Cause | Fix |
|---------|-------|-----|
| "Invalid API key" on all agents | `ANTHROPIC_API_KEY` or `HIVE_ANTHROPIC_API_KEY` set in environment | `unset ANTHROPIC_API_KEY HIVE_ANTHROPIC_API_KEY` and remove from `.bashrc` |
| "connection refused" on :8080 | Work-server not running | Start it (Hive Up step 3) |
| Dashboard shows "57m ago" everywhere | Hive died but telemetry data persists | Clean Slate, then restart |
| All agents show $0.000 cost | Hive just started, no iterations yet | Wait 2-3 minutes |
| "database unavailable" in API | Postgres not running | `docker compose up -d postgres` |
| Hive won't start — port conflict | pgadmin or another process on :8080 | Check `lsof -i :8080`, kill the conflict |
| Agent stuck in Processing state | LLM call hanging or rate limited | Restart hive (graceful) |
| Hive won't start — "unmarshal content" | Corrupted head event in chain | Check `SELECT content_json FROM events ORDER BY seq DESC LIMIT 1` — fix or Nuclear Option |
| Zombie processes after hive down | `go run` child processes survive parent kill | `ss -tlnp \| grep 8081` to find PID, then `kill -9 <PID>` |
| Role proposed but never spawns | Missing `hive.role.approved` or `agent.budget.adjusted` | Run `go run ./cmd/approve-role --role <name> --store postgres://...` |
| Webhook returns "connection refused" on :8081 | Event listener not started or zombie holding port | Kill zombies, restart hive |
| All agents cold-start on reboot | `OPEN_BRAIN_URL` not set | Export `OPEN_BRAIN_URL` and `OPEN_BRAIN_KEY` in environment |
