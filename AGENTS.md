# AGENTS.md

## Purpose
Hive runtime and orchestration layer: a trust-gated civilisation of agents built on EventGraph, Agent, and Work.

## Commands
- Build: `make build`
- Test: `make test`
- Vet: `make vet`
- Verify: `make verify`
- Local Postgres: `docker compose up -d postgres`

## Rules
- Preserve human approval requirements for significant decisions and self-modification.
- Guardian or integrity-watch behavior must remain outside normal hierarchy suppression paths.
- Agents coordinate through events and tasks; avoid hidden side channels.
- Do not weaken trust, authority, identity, budget, or observability invariants without explicit approval.
- Do not push to `upstream`; `origin` is the writable fork.

## Exit Criteria
- `make verify` passes, or the blocker is explicit.
- Changes that alter autonomy, approval, self-modification, or agent spawning include tests and operator-facing notes.
- Cross-repo dependency changes are called out for agent, work, eventgraph, site, or docs.
