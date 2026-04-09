---
name: hive-run
description: "Run the hive with an idea and monitor agent output. Use when the user says 'hive run', 'run the hive', 'start a hive run', 'run agents on', 'hive idea', or any variation of running the hive with a task/idea. Also trigger for 'run pipeline', 'run builder', 'run scout', or single-agent runner mode."
disable-model-invocation: true
---

# Hive Run

Run the hive runtime or individual agent runners. This skill handles the command construction — the user provides the idea or task.

## Prerequisites

Before running, verify:

```bash
# 1. No poisoned API keys
if [ -n "${ANTHROPIC_API_KEY:-}" ] || [ -n "${HIVE_ANTHROPIC_API_KEY:-}" ]; then
    echo "ERROR: API key in environment will poison auth. Run: unset ANTHROPIC_API_KEY HIVE_ANTHROPIC_API_KEY"
    exit 1
fi

# 2. Postgres is up
docker exec hive-postgres-1 pg_isready -U hive 2>/dev/null || echo "WARNING: Postgres not running. Run /hive-lifecycle to start it."

# 3. Work-server is up (needed for task API)
curl -sf http://localhost:8080/health > /dev/null 2>&1 || echo "WARNING: Work-server not running. Run /hive-lifecycle to start it."
```

## Paths

```
HIVE_REPO=~/transpara-ai/repos/lovyou-ai-hive
```

## Mode 1: Legacy Runtime (multi-agent coordination)

The original hive mode — Strategist, Planner, Implementer, Guardian, SysMon, Allocator, CTO, Spawner, Reviewer all coordinate via the event graph.

```bash
cd $HIVE_REPO
go run ./cmd/hive \
    --human Michael \
    --store postgres://hive:hive@localhost:5432/hive \
    --idea "<USER'S IDEA HERE>"
```

### Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--human` | (required) | Human operator name — always `Michael` |
| `--idea` | (required) | Seed idea for the agent civilization |
| `--store` | in-memory | Postgres DSN for persistence |
| `--yes` | false | Auto-approve all authority requests (dev/testing) |
| `--repo` | current dir | Path to repo for Implementer's Operate |

### Common invocations

```bash
# Standard run with persistence
go run ./cmd/hive --human Michael --store postgres://hive:hive@localhost:5432/hive --idea "Build a REST API for user management"

# Auto-approve (faster iteration, less supervision)
go run ./cmd/hive --human Michael --store postgres://hive:hive@localhost:5432/hive --yes --idea "Add error handling to the API"

# Point at a specific repo for code changes
go run ./cmd/hive --human Michael --store postgres://hive:hive@localhost:5432/hive --yes --repo /path/to/target/repo --idea "Refactor the auth module"

# In-memory (ephemeral, no persistence across restarts)
go run ./cmd/hive --human Michael --idea "Prototype a kanban board"
```

## Mode 2: Runner (single-agent or pipeline)

Run individual agents or the Scout-Builder-Critic pipeline against lovyou.ai tasks.

```bash
# Single agent — works one task matching its role
go run ./cmd/hive --role builder --space hive --one-shot

# Full pipeline — Scout finds work, Builder implements, Critic reviews
go run ./cmd/hive --pipeline --space hive

# Daemon — pipeline on a loop
go run ./cmd/hive --daemon --interval 30m --space hive

# Council — deliberation session
go run ./cmd/hive --council --topic "Should we add a caching layer?"
```

### Runner flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--role` | | Single agent: builder, scout, critic, monitor |
| `--pipeline` | false | Scout + Builder + Critic in sequence |
| `--daemon` | false | Loop pipeline at `--interval` |
| `--interval` | 30m | Daemon cycle interval |
| `--council` | false | Convene all agents for deliberation |
| `--topic` | | Focus question for council |
| `--space` | hive | lovyou.ai space slug |
| `--budget` | 10.0 | Daily budget in USD |
| `--one-shot` | false | Work one task then exit |
| `--pr` | false | Create feature branch + PR instead of pushing to main |
| `--worktrees` | false | Each Builder task gets its own git worktree |
| `--repo` | | Path to repo for Operate |
| `--repos` | | Named repos: `name=path,name=path` |
| `--auto-clone` | false | Clone missing repos from registry URLs |

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

## After a Run

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
