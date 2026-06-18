# Hive Runtime Specification

**Derived from hive0's proven architecture + cognitive grammar. This is the build spec.**

Matt Searles + Claude · March 2026

---

## What Hive0 Proved

Hive0 shipped commits every 5 minutes. The architecture that made this possible:

1. **Long-running processes** — one process per agent role, 15s tick loop
2. **REST API coordination** — tasks, messages, comments via HTTP, not files
3. **Two reasoning modes** — `Ask()` for thinking (no tools, fast), `Execute()` for building (tools, slower)
4. **Immediate commit after DONE** — `git add -A && git commit && git push`
5. **Build verification before close** — `go build ./...` must pass
6. **Cost tracking per call** — model pricing, daily budgets, automatic cutoff
7. **Auto-scaling** — allocator spawns implementers when queue > 5
8. **Crash recovery** — heartbeats, panic recovery, task inheritance on restart
9. **Safeguards** — blocked paths, prompt injection detection, command validation
10. **Role configs as markdown files** — loaded at startup, one per role

---

## What We Already Have (Don't Reinvent)

| Component | Location | Status |
|-----------|----------|--------|
| Claude CLI provider | `eventgraph/go/pkg/intelligence/claude_cli.go` | Working. Has Reason() and Operate(). |
| Agent type | `agent/agent.go` + `agent/operations.go` | Working. State machine, causality, events. |
| Loop runner | `hive/pkg/loop/loop.go` | Working. Budget, signals, task commands. |
| Runtime | `hive/pkg/hive/runtime.go` | Working. Agent spawning, concurrent execution. |
| Budget tracking | `hive/pkg/resources/budget.go` + `tracking.go` | Working. Token/cost/iteration/duration limits. |
| Authority model | `hive/pkg/resources/authority.go` | Built but not enforced. |
| Agent definitions | `hive/pkg/hive/agentdef.go` | Working. 4 starter agents defined. |
| Role prompts | `hive/agents/*.md` | Written (11 roles). Need refinement. |
| transpara.ai API | `site/graph/handlers.go` | Working. JSON endpoints for all ops. |
| transpara.ai Board | `site/graph/views.templ` | Working. Tasks, projects, goals visible. |
| transpara.ai Chat | Conversations + messages | Working. Agent can post via API. |

## What's Redundant (Retire)

| Tool | Why it's redundant | Replace with |
|------|-------------------|-------------|
| `cmd/loop` (current) | Reinvents CLI invocation, session management, artifact files. 500 lines of broken code. | Use existing `loop.Loop.Run()` |
| `cmd/daemon` | Reinvents polling + event triggering. | Integrate into `cmd/hive` as background goroutines |
| `cmd/loop/fast.go` | Quick hack. Single CLI call. | Use `agent.Operate()` directly |
| Inline prompt building in cmd/loop | Assembles prompts from files. | Load from `agents/*.md` into `AgentDef.SystemPrompt` |
| Session files in `agents/.sessions/` | Broken UUID management. | Don't persist sessions. `--no-session-persistence` like hive0. |

## What's Missing (Build)

| Component | What | Priority |
|-----------|------|----------|
| **API client for transpara.ai** | Agents need to GET/POST tasks, messages, comments via transpara.ai REST API | P0 |
| **Per-call cost tracking** | Track tokens + cost per agent per task. Post to transpara.ai. | P0 |
| **Task-driven loop** | Agent polls board for assigned tasks, works them, commits | P0 |
| **Heartbeat system** | Agent sends "alive" signal. Monitor detects crashes. | P1 |
| **Build verification** | `go build ./...` must pass before task closes | P0 |
| **Auto-commit** | `git add -A && git commit -m "[hive:role] summary" && git push` after DONE | P0 |
| **Role-based model selection** | haiku for routine, sonnet for implementation, opus for strategy | P1 |
| **Safeguards in Execute()** | Block dangerous paths/commands, prompt injection detection | P1 |
| **Crash recovery** | Panic recovery wrapper, task inheritance on restart | P2 |
| **Auto-scaling** | Spawn parallel implementers when queue grows | P2 |

---

## The Architecture

```
                    transpara.ai (Fly.io)
                    ┌─────────────────────┐
                    │  Board (tasks)       │
                    │  Chat (channels)     │
                    │  Feed (updates)      │
                    │  Activity (audit)    │
                    │  JSON API            │
                    └──────┬──────────────┘
                           │ REST API
              ┌────────────┼────────────────┐
              │            │                │
         ┌────┴────┐  ┌───┴────┐    ┌──────┴──────┐
         │ Scout   │  │Builder │    │  Monitor    │
         │ (haiku) │  │ (opus) │    │  (haiku)    │
         └─────────┘  └────────┘    └─────────────┘
              │            │                │
              │     ┌──────┴──────┐         │
              │     │ Claude CLI  │         │
              │     │ (Operate)   │         │
              │     └──────┬──────┘         │
              │            │                │
              │     ┌──────┴──────┐         │
              │     │  site repo  │         │
              │     │  (git)      │         │
              │     └─────────────┘         │
              │                             │
              └──── All agents poll ────────┘
                   transpara.ai API
                   every 15 seconds
```

### The Tick Loop (from hive0, adapted)

```go
func (r *Runner) Run() {
    for {
        r.tick++
        r.heartbeat("alive")

        // Check for work
        r.processMessages()

        switch r.agent.Role {
        case "scout":
            r.runScout()       // Find gaps, write scout reports
        case "builder":
            r.runBuilder()     // Claim tasks, implement, commit
        case "critic":
            r.runCritic()      // Review recent builds
        case "reflector":
            r.runReflector()   // Distill lessons from recent iterations
        case "monitor":
            r.runMonitor()     // Triage unassigned tasks, restart crashed agents
        case "ops":
            r.runOps()         // Deploy, health checks
        }

        time.Sleep(r.interval) // 15 seconds default
    }
}
```

### The Builder Flow (from hive0's implementer)

```go
func (r *Runner) runBuilder() {
    // 1. Get assigned tasks
    tasks := r.api.GetTasks(status=open, assigned_to=r.agent.ID)

    // 2. If none, claim unassigned
    if len(tasks) == 0 {
        tasks = r.api.GetTasks(status=open, unassigned=true, limit=1)
        if len(tasks) > 0 {
            r.api.AssignTask(tasks[0].ID, r.agent.ID)
        }
    }

    if len(tasks) == 0 { return }

    // 3. Work the highest priority task
    t := pickHighestPriority(tasks)
    r.workTask(t)
}

func (r *Runner) workTask(t Task) {
    r.api.UpdateTaskStatus(t.ID, "in_progress")

    // 4. Execute with Claude CLI (full tool access)
    result := r.agent.Operate(ctx, siteRepoPath, buildPrompt(r.rolePrompt, t))

    // 5. Parse response for ACTION: DONE | PROGRESS | ESCALATE
    action := parseAction(result.Summary)

    switch action {
    case "DONE":
        if !verifyBuildPasses(siteRepoPath) {
            r.api.CommentTask(t.ID, "Build failed, fixing...")
            return // stay in_progress
        }
        commitAndPush(t)
        r.api.CloseTask(t.ID, result.Summary)
        r.api.RecordCost(r.agent.ID, t.ID, result.Usage)

    case "PROGRESS":
        r.api.CommentTask(t.ID, result.Summary)

    case "ESCALATE":
        r.api.EscalateTask(t.ID, "matt", result.Summary)
    }
}
```

---

## API Client for transpara.ai

The agents communicate via transpara.ai's existing JSON API:

```go
type APIClient struct {
    base   string // "https://transpara.ai"
    apiKey string // LOVYOU_API_KEY (Bearer token)
}

// Tasks
client.GetTasks(spaceSlug, status, assignedTo) → []Node
client.CreateTask(spaceSlug, title, body, priority) → Node
client.AssignTask(spaceSlug, nodeID, assigneeID) → ok
client.UpdateTaskStatus(spaceSlug, nodeID, state) → ok
client.CloseTask(spaceSlug, nodeID, summary) → ok
client.CommentTask(spaceSlug, nodeID, body) → Node
client.EscalateTask(spaceSlug, nodeID, toUser, reason) → ok

// Messages (conversations)
client.PostMessage(spaceSlug, conversationID, body) → Node
client.GetMessages(spaceSlug, conversationID, after) → []Node

// Feed
client.PostUpdate(spaceSlug, title, body) → Node

// Cost tracking
client.RecordCost(agentID, taskID, usage) → ok
```

These map directly to existing endpoints:
- `POST /app/{slug}/op` with `op=intend|respond|complete|claim|assign`
- `GET /app/{slug}/board?format=json`
- `GET /app/{slug}/conversation/{id}/messages?format=json`

---

## Cost Tracking

From hive0's proven model:

```go
var modelPricing = map[string]struct{ Input, Output float64 }{
    "haiku":  {0.80, 4.00},    // per million tokens
    "sonnet": {3.00, 15.00},
    "opus":   {15.00, 75.00},
}

type CostTracker struct {
    totalCostUSD float64
    budgetUSD    float64 // $10/day default
    callCount    int64
}

func (ct *CostTracker) Record(model string, inputTokens, outputTokens int) float64 {
    pricing := modelPricing[model]
    cost := float64(inputTokens)/1e6*pricing.Input + float64(outputTokens)/1e6*pricing.Output
    ct.totalCostUSD += cost
    ct.callCount++
    return cost
}

func (ct *CostTracker) IsOverBudget() bool {
    return ct.totalCostUSD >= ct.budgetUSD
}
```

On Max plan (flat rate), cost tracking is informational, not enforcing. But it's essential for understanding efficiency and will matter when running on API keys.

---

## Role-Based Model Selection

From hive0, adapted for our roles:

| Role | Default Model | Why |
|------|--------------|-----|
| scout | haiku | Reading + analysis, doesn't need opus |
| architect | sonnet | Design decisions need quality |
| builder | sonnet | Code quality matters, opus too slow/expensive |
| tester | haiku | Test writing is mechanical |
| critic | sonnet | Quality review needs judgment |
| reflector | haiku | Lesson extraction is mechanical |
| ops | haiku | Deploy commands are mechanical |
| guardian | haiku | Pattern matching, not creativity |
| monitor | haiku | Triage is mechanical |
| pm | sonnet | Prioritization needs judgment |

**Override:** `AGENT_MODEL=opus` forces all roles to opus (for testing).

---

## Build Order

### Phase 1: Single Builder (this session)
1. Write transpara.ai API client (`pkg/api/client.go`)
2. Write Runner with tick loop (`pkg/runner/runner.go`)
3. Write Builder role (`runBuilder` flow)
4. Auto-commit after DONE
5. Build verification before close
6. Cost tracking per call
7. Test: run builder on one task, verify it commits

### Phase 2: Core Loop Roles
8. Scout role (find gaps, write to board)
9. Critic role (review recent commits)
10. Monitor role (triage, restart detection)
11. Test: full Scout → Builder → Critic cycle

### Phase 3: Resilience
12. Heartbeat system
13. Crash recovery (panic wrapper)
14. Task inheritance on restart
15. Safeguards (blocked paths, injection detection)

### Phase 4: Scaling
16. Parallel builders (allocator spawns on demand)
17. Role-based model selection (auto-upgrade for complex tasks)
18. Intelligence self-regulation

---

## What Gets Retired

After the new runner is working:

| Retire | Reason |
|--------|--------|
| `cmd/loop/` | Replaced by runner in `cmd/hive/` |
| `cmd/daemon/` | Background agents are goroutines in the runner |
| `agents/.sessions/` | No session persistence needed |
| File-based artifacts (scout.md etc as primary) | Tasks/comments on transpara.ai are the artifacts |

The `agents/*.md` prompt files STAY — they're the role definitions loaded at startup.

---

## Convergence Check

**Need:** What's absent?
- API client for transpara.ai ← not written yet
- Runner with tick loop ← not written yet
- Auto-commit ← not written yet
- Cost tracking ← exists in resources/tracking.go but not wired to transpara.ai API
- Build verification ← one function, trivial

**Traverse:** What exists?
- intelligence.claudeCliProvider ← Reason() and Operate() work
- agent.Agent ← state machine, events, budget work
- loop.Loop ← budget enforcement, signals, task commands work
- hive.Runtime ← agent spawning works
- transpara.ai ← full JSON API exists
- hive0 ← complete reference implementation

**Derive:** What follows?
- The API client is ~100 lines (HTTP + JSON, no magic)
- The Runner is ~200 lines (tick loop + role dispatch + commit)
- The Builder flow is ~100 lines (claim → work → verify → commit → close)
- Total new code: ~400 lines
- Total retired code: ~800 lines (cmd/loop + cmd/daemon)

**Fixpoint.** The spec is actionable. /clear and build.
