# Hive

A self-organizing AI agent system that builds products autonomously. Built on [EventGraph](https://github.com/lovyou-ai/eventgraph).

## Soul

> Take care of your human, humanity, and yourself. In that order when they conflict, but they rarely should.

Inherited from EventGraph. Every agent in the hive operates under this constraint.

## What This Is

Hive is a product factory. Agents research ideas, design systems in Code Graph vocabulary, generate code, review it, test it, and deploy it. The human provides direction and approves significant decisions. Everything is recorded on the event graph.

## Architecture

- All agents share one event graph (one Store) and one actor store (IActorStore)
- Every actor (human + agents) is registered in the actor store — no magic strings
- Actor IDs are derived from deterministic key pairs, not hardcoded
- Each agent is an `AgentRuntime` with its own identity and signing key
- Communication is through events, not messages
- The Guardian watches everything independently
- Trust accumulates through verified work

## Roles

| Role | Responsibility | Trust Gate |
|------|---------------|------------|
| CTO | Architectural oversight, escalation filtering | 0.1 (bootstrapped) |
| Guardian | Independent integrity, halt/rollback | 0.1 (bootstrapped) |
| Researcher | Read URLs, extract product ideas | 0.3 |
| Architect | Design systems in Code Graph | 0.3 |
| Builder | Generate code + tests | 0.3 |
| Reviewer | Code review, security audit | 0.5 |
| Tester | Run tests, validate behavior | 0.3 |
| Integrator | Assemble, deploy | 0.7 |

## Dev Setup

```bash
cd hive
go build ./...
go test ./...
```

## Running

```bash
# Start the hive with a product idea (CTO derives the product name)
go run ./cmd/hive --human Matt --idea "Build a task management app with kanban boards"

# Start from a URL with an explicit product name
go run ./cmd/hive --human Matt --name social-grammar --url "https://mattsearles2.substack.com/p/the-missing-social-grammar"

# Start from a Code Graph spec file
go run ./cmd/hive --human Matt --spec path/to/spec.cg
```

Each product gets its own GitHub repo under lovyou-ai, with git commits at each phase.

## Key Files

- `pkg/roles/` — Agent role definitions and system prompts
- `pkg/pipeline/` — Product pipeline orchestration
- `pkg/workspace/` — File system management for generated code
- `cmd/hive/` — CLI entry point

## Intelligence

All inference runs through **Claude CLI** (Max plan, flat rate). NOT the Anthropic API — CLI is cheaper and better for our use case. The pipeline creates `claude-cli` providers automatically.

Model assignment by role:
- **Opus** (`claude-opus-4-6`): CTO, Architect, Reviewer, Guardian — high-judgment tasks
- **Sonnet** (`claude-sonnet-4-6`): Builder, Tester, Integrator, Researcher — execution tasks

## Pipeline

1. **Research** — Researcher reads URLs/ideas, CTO evaluates feasibility
2. **Design** — Architect creates Code Graph spec, CTO reviews for minimalism
3. **Simplify** — Architect reduces spec to minimal form (up to 3 rounds)
4. **Build** — Builder generates multi-file project, committed to product repo
5. **Review → Rebuild** — Reviewer checks quality/compliance/simplicity. If issues found, Builder fixes and re-submits (up to 3 rounds)
6. **Test** — Tester runs actual test suite, Builder fixes failures
7. **Integrate** — Integrator pushes to GitHub, escalates to human for approval

Guardian runs integrity checks after every phase.

## Design Philosophy

The Architect enforces **derivation over accumulation**:
- Each view has the minimal elements required
- Complexity emerges from composing simple atoms, not adding parts
- A simplification pass runs after every design phase (up to 3 rounds)
- The Reviewer checks generated code for unnecessary complexity
- System prompts are wired to each agent's provider — roles have real context

## Dependencies

- `github.com/lovyou-ai/eventgraph/go` — event graph, agent runtime, intelligence
- Claude CLI — intelligence backend (flat rate via Max plan, no API key needed)
