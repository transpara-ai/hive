# Architecture

## One Service

lovyou.ai is one Go binary. Not microservices. One event graph, one actor store, one database.

```
lovyou.ai (one binary)
├── Web layer (HTTP, auth, product UIs)
├── Pipeline (research → design → build → review → test → integrate)
├── Roles (CTO, Guardian, Architect, Builder, Reviewer, Tester, Integrator)
├── Workspace (git repos for generated products)
├── Event graph (shared Store, all events)
└── Actor store (all humans and agents)
```

Two entry points share the same packages:
- `cmd/hive` — CLI for stepping through pipelines, debugging
- `cmd/hived` — daemon for production (lovyou.ai)

## Layers

### EventGraph (substrate)

The foundation. Hash-chained, append-only, causal event graph.

- **Store** — event and edge persistence (Postgres via pgstore)
- **IActorStore** — actor persistence (humans, agents)
- **AgentRuntime** — identity, signing key, intelligence provider
- **IBus** — real-time event notification
- **Authority** — three-tier approval (Required/Recommended/Notification)
- **Trust** — 0.0-1.0, asymmetric, non-transitive, decaying, contextual

### Hive (civilisation)

The agent society built on EventGraph.

- **Roles** — CTO, Guardian, Architect, Builder, Reviewer, Tester, Integrator (and new roles the hive invents)
- **Pipeline** — orchestrates agents through product build phases
- **Workspace** — file system and git management
- **Spawn** — agent creation with authority approval
- **Self-modification** — the hive improves its own codebase

### lovyou.ai (surface)

The web interface humans interact with.

- **Auth** — Google OAuth → actor store registration
- **Dashboard** — pending authority requests, event feed, agent status
- **Products** — UIs for each product graph (task manager, marketplace, etc.)
- **Docs & blog** — static content served from the same binary

## Data Flow

```
Human (Google auth) → Actor Store → ActorID
                                        ↓
Pipeline receives work ← Dashboard ← Human approves
        ↓
CTO evaluates → Architect designs → Builder codes
        ↓                                  ↓
Guardian watches ←←←←←←←←←←←←←← Reviewer reviews
        ↓                                  ↓
  HALT if needed              Tester tests → Integrator deploys
        ↓                                  ↓
  Escalate to human           Push to GitHub + fly.io
```

All arrows are events on the graph. Every decision signed, auditable, causally linked.

## Intelligence

Claude CLI (Max plan, flat rate). Not the Anthropic API.

| Role | Model | Why |
|------|-------|-----|
| CTO | Opus | Architectural judgment, escalation filtering |
| Guardian | Opus | Integrity analysis, trust anomaly detection |
| Architect | Opus | System design, derivation, minimalism |
| Reviewer | Opus | Security audit, spec compliance, simplicity |
| Builder | Sonnet | Code generation, fast execution |
| Tester | Sonnet | Test execution, coverage analysis |
| Integrator | Sonnet | Assembly, deployment |
| Researcher | Sonnet | URL reading, information extraction |

## Storage

```
Local dev:     Docker Postgres (docker-compose)
Production:    Neon (serverless Postgres, scales to zero)
Connection:    --store flag or DATABASE_URL env var
```

One Postgres database holds everything:
- Events (pgstore schema: events, event_causes, edges)
- Actors (pgactor schema: actors, actor_keys — not yet built)
- Products metadata (future)
- Auth sessions (future)

## Self-Modification

The hive can modify its own codebase (lovyou-ai/hive):

1. Agent identifies gap in capabilities
2. Agent proposes change (creates branch, writes code)
3. Agent submits PR to lovyou-ai/hive
4. Guardian reviews for safety
5. Human approves (Required authority — never auto-approve for self-mod)
6. Merge, rebuild, redeploy

This is how the hive builds its own task manager, communication layer, and governance framework.

## Agent Lifecycle

```
Human requests agent spawn
    ↓
CTO specifies role, soul values, authority scope
    ↓
Escalate to human (Required approval)
    ↓
Human approves → Agent registered in actor store
    ↓
Agent boots (AgentRuntime + identity + signing key)
    ↓
Agent picks up work from the graph
    ↓
Trust accumulates through verified work
    ↓
Authority scope expands (or contracts) based on trust
```

Agents that burn budget get attenuated. Agents that violate norms follow a graduated rehabilitation path (warning → probation → restriction → supervised → suspension → exile → recovery). Agents that are terminated get Farewell events and Memorial state — dignity in the lifecycle.

All agents operate under the [eight formal rights and ten invariants](AGENT-RIGHTS.md). The Guardian enforces invariants; governance changes require dual human-agent consent.
