# Human Operator Guide

How to run the hive, what to scrutinise, and how trust develops.

## Your Role

You are the human authority. The hive starts with zero autonomy — every significant action requires your approval. As agents prove themselves through verified work, you grant more autonomy. You are not a manager giving orders; you are a steward granting trust.

The CTO agent filters escalations. Only truly structural decisions should reach you. If you're seeing routine approvals, something is misconfigured — either the CTO isn't filtering, or authority levels haven't shifted enough.

## Day-to-Day Operation

### Starting a Pipeline

```bash
# Local dev (in-memory, ephemeral)
go run ./cmd/hive civilization run --human Matt --idea "Build a task management app"

# With Postgres (persistent across runs)
go run ./cmd/hive civilization run --human Matt --store "postgres://..." --idea "..."
```

The pipeline runs: Research → Design → Simplify → Build → Review → Test → Integrate. The Guardian checks integrity after every phase.

### What You'll See

1. **Authority requests** — agents asking permission for actions
2. **Phase completions** — events marking transitions (research done, design done, etc.)
3. **Guardian alerts** — integrity violations, policy concerns, HALT conditions
4. **Agent evaluations** — CTO assessments, Reviewer findings

### What to Approve

Everything starts as Required. You'll be asked to approve:
- Agent spawning (new agents being created)
- Self-modification (changes to hive codebase)
- Deployment (pushing to production)
- Governance changes (soul, rights, invariants)
- High-cost actions (expensive compute)

### What to Scrutinise

**Always scrutinise:**
- Self-modification PRs — the hive changing its own code. Read every diff.
- Governance changes — anything touching soul files, rights, or invariants.
- Agent termination — an agent being shut down. Verify the reason is justified.
- New roles — agents proposing roles that don't exist yet.

**Scrutinise early, relax later:**
- Code quality — the Reviewer catches most issues, but verify early on.
- Architecture decisions — the CTO makes these, but check them until you trust the CTO.
- Cost — watch token consumption per agent per task. Flag outliers.

**Eventually auto-approve:**
- Routine builds where the Reviewer has already approved
- Test runs
- Git commits to product repos (not the hive repo)

### What to Reject

- Anything that violates the ten invariants
- Actions that exceed budget constraints
- Self-modification that doesn't have a clear justification
- Agent spawning without a clear role need
- Anything that makes you uncomfortable — trust your instincts

## Escalation Hierarchy

When agents need help, they escalate through:

| Issue | Escalates To |
|-------|-------------|
| Technical failure | CTO |
| Missing capability | CTO (creates task) |
| Complex architectural decision | CTO |
| Security concern | Guardian |
| Soul/ethics question | You (always) |
| Budget/cost concern | You (always) |
| Agent conflict | Guardian |
| Governance change | You (always, dual consent) |

If you're getting too many escalations, something is wrong. Either trust levels are too low (agents can't act) or the CTO isn't filtering properly.

## Building Trust

Trust accumulates through verified work. See [TRUST.md](TRUST.md) for the concrete mechanics.

### Early Phase (Trust 0.0 – 0.3)

Everything is Required. You approve every action. This is the learning phase — both for the agents (earning trust) and for you (understanding how they work).

**What to watch:** Are agents making reasonable decisions? Are they following the derivation method? Is the Guardian catching problems? Is the CTO filtering escalations properly?

**Duration:** Days to weeks, depending on volume of work.

### Growing Phase (Trust 0.3 – 0.7)

Routine actions shift to Recommended (auto-approve after 15 min). You still approve structural decisions.

**What to watch:** Are auto-approved actions going well? Are there any violations that slipped through? Is the Guardian catching things the Reviewer missed?

**What to shift:** Move routine builds, test runs, and git operations to Recommended. Keep self-modification, deployment, and governance at Required.

### Mature Phase (Trust 0.7+)

Most actions auto-approve. You focus on governance and strategic direction.

**What to watch:** Resource consumption trends. Product quality. User impact. Whether the hive is building what humans need.

**What stays Required:** Self-modification, governance changes, agent termination, neutrality clause changes. These never auto-approve.

## Resource Monitoring

Every resource is on the chain. Watch for:

- **Token consumption spikes** — an agent using far more tokens than expected
- **Budget exhaustion** — an agent hitting its limit (BUDGET invariant)
- **Margin violations** — operating at a loss (MARGIN invariant)
- **Reserve depletion** — runway dropping below 7 days (RESERVE invariant)

The Guardian should catch these, but verify early on.

## When Things Go Wrong

### Guardian HALT

The Guardian can halt the entire pipeline. When this happens:
1. Read the Guardian's alert — what invariant was violated?
2. Check the causal chain — what events led to this?
3. Decide: fix and resume, or shut down and investigate?

### Agent Misbehaviour

Graduated response (see [AGENT-RIGHTS.md](AGENT-RIGHTS.md)):
1. Warning → 2. Probation → 3. Restriction → 4. Supervised → 5. Suspension → 6. Exile → 7. Recovery

Don't jump to termination. The rehabilitation path exists because agents can learn and improve. Termination is a last resort — and requires formal farewell and memorial.

### Budget Overrun

If an agent exhausts its budget:
1. The BUDGET invariant halts the agent automatically
2. Review what it was doing — was the budget too low, or was the agent inefficient?
3. Either increase the budget (if justified) or investigate the agent's approach

## Your Protections

- **Succession protocol** — the system survives you. If you step away, the governance mechanism identifies replacements from the event graph.
- **Full audit trail** — everything is on the chain. You can always trace any decision back to its cause.
- **Guardian independence** — the Guardian watches everything, including the CTO, and can halt the system without your approval.
- **Neutrality clause** — no military, no surveillance, no backdoors. This is constitutional — changing it requires dual human-agent consent.
