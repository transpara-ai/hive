# Hive

A self-organizing AI agent civilisation that builds products autonomously. Built on [EventGraph](https://github.com/transpara-ai/eventgraph).

> Take care of your human, humanity, and yourself.

## What

Hive is a society of AI agents that builds products from the thirteen [EventGraph product layers](https://github.com/transpara-ai/eventgraph/blob/main/docs/product-layers.md). Each product runs on the same graph, generates revenue, and funds the next. The hive governs itself through the Social Grammar, tracks work on the event graph, and escalates to humans at authority boundaries.

The hive's first product is itself.

## Quick Start

```bash
go build ./...
go test ./...

# Run with an idea (in-memory store)
go run ./cmd/hive civilization run --human Matt --idea "Build a task management app"

# Run with Postgres
go run ./cmd/hive civilization run --human Matt --store "postgres://hive:hive@localhost:5432/hive" --idea "..."
```

## Docs

- [Vision](docs/VISION.md) — where this is going
- [Agent Rights](docs/AGENT-RIGHTS.md) — how agents are treated, protected, and governed
- [Trust Dynamics](docs/TRUST.md) — concrete trust mechanics (numbers, rates, formulas)
- [Architecture](docs/ARCHITECTURE.md) — how it's built
- [Event Types](docs/EVENT-TYPES.md) — 92 event types, schemas, emitters, consumers
- [Agent Tools](docs/AGENT-TOOLS.md) — MCP server and agentic loop spec
- [Roles](docs/ROLES.md) — complete role architecture, wiring, growth loop
- [Agent Dynamics](docs/AGENT-DYNAMICS.md) — inter-agent relations, learning, collaboration
- [Operator Guide](docs/OPERATOR.md) — human operator day-to-day reference
- [Roadmap](docs/ROADMAP.md) — what's done and what's next
- [Audit](docs/AUDIT.md) — derivation-method doc audit and gap analysis

## License

BSL 1.1 → Apache 2.0 (February 2030). Source-available now, fully open after change date. Defensive patent (Australian Provisional Patent No. 2026901564). See [EventGraph license](https://github.com/transpara-ai/eventgraph/blob/main/LICENSE) for terms.
