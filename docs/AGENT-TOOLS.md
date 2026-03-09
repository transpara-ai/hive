# Agent Tools & Autonomy

How agents get tools, use them, and act autonomously.

## Two Layers

### Layer 1: MCP Server (the hands)

An MCP (Model Context Protocol) server written in Go that exposes the event graph, actor store, trust model, and agent identity as tools Claude CLI can call during reasoning.

When an agent reasons, it can call these tools mid-thought — Claude CLI handles the tool-call loop internally (call tool → get result → continue reasoning → call another tool → ...).

**Transport:** stdio (the MCP server is a Go binary, Claude CLI spawns it as a subprocess).

**Tools exposed:**

| Tool | Description | Reads/Writes |
|------|-------------|-------------|
| `query_events` | Query events by type, source, limit | Read |
| `get_event` | Get a single event by ID with full detail | Read |
| `get_actor` | Look up an actor by ID | Read |
| `list_actors` | List actors with filters (type, status) | Read |
| `get_trust` | Get trust score for an actor or between two actors | Read |
| `emit_event` | Record an event on the graph (authority-checked) | Write |
| `query_self` | Get own actor info, trust level, trend | Read |
| `query_human` | Get human operator info | Read |

**Not exposed (Claude CLI has these natively):** `read_file`, `write_file`, `list_files`, `run_command`. Claude CLI provides Read, Write, Edit, Bash, Glob, and Grep tools out of the box — duplicating them in the MCP server would confuse the agent with two ways to do the same thing.

**Authority:** Write tools (emit_event) are authority-checked. The agent's trust score must meet a minimum threshold. Suspended agents cannot write.

**Audit:** Every tool call (read and write) emits an `agent.acted` event with action prefix `mcp.tool_call:` so the Guardian can monitor all tool usage.

**Architecture:**

```
Pipeline (Go)
  │
  ├── writes .mcp.json with hive MCP server config
  ├── spawns Claude CLI with --mcp-config
  │     │
  │     ├── Claude CLI spawns MCP server (Go binary)
  │     │     │
  │     │     ├── query_events → store.ByType/BySource/Recent
  │     │     ├── get_actor → actors.Get
  │     │     ├── emit_event → factory.Create + store.Append (authority-checked)
  │     │     ├── query_self → actors.Get(self) + trust.Score
  │     │     └── ... other tools
  │     │
  │     └── Claude reasons, calls tools, gets results, continues
  │
  └── observes events on the graph
```

The MCP server shares the same Postgres pool as the pipeline. It's a thin adapter — translating MCP JSON-RPC calls into Go method calls on the store/actors/trust.

**Context injection:** Before each prompt, a `ContextBuilder` injects orientation context: the agent's identity, human operator, other actors with trust scores, and recent events. This gives agents basic awareness without tool calls.

### Layer 2: Agentic Loop (the brain)

The outer loop that gives agents sustained autonomy. An agent doesn't just respond to a prompt — it observes the world, decides what to do, acts, and observes again.

```
┌─────────────────────────────────────┐
│           AGENTIC LOOP              │
│                                     │
│  1. OBSERVE                         │
│     Query graph for new events      │
│     Check pending tasks             │
│     Read recent changes             │
│                                     │
│  2. REASON                          │
│     What needs doing?               │
│     What can I do with my tools?    │
│     What's beyond my authority?     │
│                                     │
│  3. ACT                             │
│     Call tools (MCP layer)          │
│     Emit events                     │
│     Write code                      │
│     Build new tools if needed       │
│                                     │
│  4. REFLECT                         │
│     Did it work?                    │
│     What changed on the graph?      │
│     Should I continue or escalate?  │
│                                     │
│  5. REPEAT or STOP                  │
│     Continue if work remains        │
│     Stop if quiescent               │
│     Escalate if uncertain           │
└─────────────────────────────────────┘
```

The AgentRuntime already has `RunTask` (Observe → Evaluate → Decide → Act → Learn) as a single pass. The agentic loop runs this repeatedly until:
- The task is complete (quiescence — no new events, nothing to do)
- The agent needs human approval (escalation)
- The Guardian halts the agent
- A budget/iteration limit is reached

**Key difference from the current pipeline:** Right now the pipeline orchestrates agents in a fixed sequence (research → design → build → ...). With the agentic loop, agents are self-directing — they observe the graph, identify what needs doing, and do it. The pipeline becomes a seed that kicks off work, then the agents take over.

## How They Work Together

1. Pipeline starts, registers agents, seeds initial work on the graph
2. CTO agent enters its agentic loop:
   - OBSERVE: reads the product idea from the graph
   - REASON: "I need to evaluate feasibility"
   - ACT: uses `query_events` to check prior work, emits feasibility assessment
   - REFLECT: "Now I need an architect"
   - ACT: emits a task for the Architect, escalates to human for agent spawn if needed
3. Architect agent enters its loop:
   - OBSERVE: picks up the task from the graph
   - REASON: "I need to design a Code Graph spec"
   - ACT: uses Claude's native Read/Write to check existing specs, emits design
   - REFLECT: "Is this minimal? Let me simplify"
   - ACT: revises the design
4. Builder agent enters its loop:
   - OBSERVE: picks up the approved design
   - ACT: uses Claude's native Write/Bash to generate code and test
   - REFLECT: "Tests failing" → fixes → retests
5. Guardian watches all events continuously, halts if needed

Each agent runs its own loop. They communicate through events on the shared graph, not through direct messages.

## Self-Improvement

When an agent lacks a tool or skill:

1. Agent identifies the gap during REASON: "I need to deploy to fly.io but I don't have a deploy tool"
2. Agent emits a task: "Build a fly.io deployment tool"
3. CTO evaluates: is this a self-modification (changes to lovyou-ai/hive) or a new tool?
4. If self-mod: agent specs the change, submits PR, human approves
5. If new tool: agent builds it as an MCP tool extension, Guardian reviews
6. The new tool becomes available to all agents

This is how the hive grows its own capabilities. The MCP server's tool list isn't static — agents can extend it.

## Implementation

### Files

```
pkg/mcp/
├── protocol.go     — JSON-RPC 2.0 types (Request, Response, Tool, etc.)
├── server.go       — stdio transport, tool registry, dispatch
├── tools.go        — tool definitions + handlers (read, self, write)
├── format.go       — event/actor/trust JSON serialization
├── authority.go    — trust-based authority checking for write tools
├── audit.go        — audit logging (every tool call → event on graph)
├── context.go      — context injection (identity, actors, events)
├── config.go       — .mcp.json writer for Claude CLI
├── server_test.go  — 12 tests
└── config_test.go  — 1 test

cmd/mcp-server/
└── main.go         — binary entry point (Postgres, stores, trust, flags)
```

### Wiring into Claude CLI

The pipeline writes `.mcp.json` before spawning Claude CLI:

```json
{
  "mcpServers": {
    "hive": {
      "command": "/path/to/mcp-server",
      "args": ["--store", "postgres://...", "--agent-id", "actor_...", "--human-id", "actor_...", "--conv-id", "conv_..."]
    }
  }
}
```

The `intelligence.Config` has `MCPConfigPath` — the claude-cli provider passes `--mcp-config <path>` automatically.

## Security

- **Read tools** are unrestricted — any agent can query the graph
- **Write tools** require authority checks (trust threshold, not suspended)
- **Agent ID injection** — emit_event always uses the authenticated agent's ID, preventing impersonation
- **Audit trail** — every tool call emits an event, visible to the Guardian
- **Self-modification tools** always require human approval (Required authority level)
- **Budget limits** prevent runaway tool use (max iterations, max tokens, max cost per loop)
