# Agent Dynamics

How agents relate to each other, learn within a lifetime, and collaborate.

## Inter-Agent Relations

Agents communicate through events on the shared graph, not direct messages. But they have rich relational primitives:

### Delegation

An agent assigns work to another agent:

```
agent.delegated { AgentID, Delegate, Task }
```

Delegation transfers a goal with authority and constraints. The delegating agent remains accountable — delegation doesn't mean abandonment. The Supervise composition tracks completion:

```
Delegate(task) + Expect(completion) + Observe(progress) + Evaluate(quality) + Repair(if_needed)
```

### Consent

Bilateral agreement before action proceeds:

```
agent.consent.requested { AgentID, Target, Action }
agent.consent.granted   { AgentID, Requester, Action }
agent.consent.denied    { AgentID, Requester, Action, Reason }
```

Both parties must agree. Consent is a primitive operation — mutual, atomic, dual-signed, cryptographically verifiable. Critical for compositions requiring mutual obligation.

### Channels

Persistent bidirectional communication between specific agents:

```
agent.channel.opened { AgentID, Peer, Channel }
agent.channel.closed { AgentID, Peer, Channel, Reason }
```

Channels are structure, not content. The content flows as events on the graph through the channel.

### Compositions

Multiple agents form a group with shared goals and combined authority:

```
agent.composition.formed    { AgentID, Members[], Purpose }
agent.composition.joined    { AgentID, GroupID }
agent.composition.left      { AgentID, GroupID, Reason }
agent.composition.dissolved { AgentID, GroupID, Reason }
```

### Named Collaboration Patterns

| Pattern | Composition | When Used |
|---------|------------|-----------|
| **Supervise** | Delegate + Expect + Observe + Evaluate + Repair | CTO assigns to Builder |
| **Collaborate** | Channel + Communicate + Consent + Composition + Act | Architect + Builder co-design |
| **Crisis** | Observe + Evaluate + Attenuate + Escalate + Expect | Guardian detects integrity breach |
| **Review** | Observe + Evaluate + Decide + Communicate | Reviewer checks Builder's code |

## Agent-to-Agent Trust

The same trust model applies between agents as between humans and agents:

- **Asymmetric** — CTO trusts Builder ≠ Builder trusts CTO
- **Non-transitive** — CTO trusts Architect, Architect trusts Builder ≠ CTO trusts Builder
- **Domain-specific** — Builder trusted for code, not for architecture
- **Evidence-based** — trust changes from observed events, not declarations

Agents assess each other's trust:
```
agent.trust.assessed { AgentID, Target, Previous, Current }
```

This enables: the CTO stops delegating to a Builder whose code keeps failing review. The Reviewer flags an Architect whose designs are consistently over-complex.

## Conflict Resolution

When agents disagree (e.g., Reviewer rejects Builder's code):

1. **Direct resolution** — the agents work it out through the review/rebuild cycle (up to 3 rounds)
2. **Escalation** — if unresolved, escalate to CTO
3. **Guardian intervention** — if the conflict involves policy violation, Guardian can halt and arbitrate
4. **Human authority** — if CTO can't resolve, it escalates to the human operator

Conflicts are events on the graph. The resolution is traced causally from the disagreement to the outcome.

## Agent Learning (Within a Lifetime)

Agents learn without modifying the codebase. Three mechanisms:

### 1. Decision Tree Evolution

Every LLM-requiring decision tracks response history. When patterns emerge:

1. **Observation** — Every LLM leaf tracks response history
2. **Pattern recognition** — After ≥50 hits, detect extractable deterministic rules with ≥95% accuracy
3. **Branch insertion** — New deterministic branches intercept known patterns; LLM fallthrough preserved for unknowns
4. **Cost demotion** — Expensive LLM calls progressively replaced with cheap rules

```
decision.branch.proposed  { PrimitiveID, TreeVersion, Condition, Outcome, Accuracy, SampleSize }
decision.branch.inserted  { PrimitiveID, TreeVersion, Path, Outcome, Confidence }
decision.cost.report      { PrimitiveID, TreeVersion, TotalLeaves, LLMLeaves, MechanicalRate, TotalTokens }
```

This is the **mechanical-to-intelligent continuum** — the agent starts expensive (every decision needs LLM reasoning) and gets cheaper as patterns crystallise into rules.

### 2. Memory Accumulation

Agents update persistent memory based on outcomes:

```
agent.learned { AgentID, Lesson, Source }
agent.memory.updated { AgentID, Key, Action }
```

Memory is selective and interpretive, not a raw log. The agent decides what's worth remembering. Memory survives restarts (dual storage: DB + repo).

### 3. Introspection

Agents reflect on their own performance:

```
agent.introspected { AgentID, Observation }
```

Self-observation feeds back into decision-making. An agent that notices it keeps making the same mistake can adjust its approach.

### The Growth Loop

The primary operating mechanism (proven across 9 iterations):

1. Something breaks
2. Ask: "What role should have caught that?"
3. If role doesn't exist → create it (with authority approval)
4. If role exists but failed → upgrade it with new knowledge

This is how the first hive grew from a handful of roles to 74, completing 3,653 tasks in 7 days.

### Learning vs Self-Modification

| Learning | Self-Modification |
|----------|------------------|
| Within a lifetime | Changes the codebase |
| Decision trees, memory, introspection | PRs to lovyou-ai/hive |
| No authority required | Always Required (human approval) |
| Agent-specific | Affects all agents |
| Continuous | Discrete (PR merge events) |

Both are important. Learning makes individual agents better. Self-modification makes the hive better.

## Product Derivation Pattern

When the hive builds a new product (Tier 5+), it follows the derivation method applied to the product's composition grammar:

1. **Read the composition grammar** — e.g., [work.md](https://github.com/lovyou-ai/eventgraph/blob/main/docs/compositions/01-work.md) defines 12 operations, 3 modifiers, 6 named functions
2. **Derive the UI** — each operation becomes an action the user can take; each named function becomes a workflow
3. **Derive the API** — each operation becomes an endpoint; each modifier becomes a parameter
4. **Derive the data model** — from the event types the grammar emits
5. **Verify completeness** — every operation in the grammar must be expressible in the product; every named function must have a UI path

The pattern is the same for all 13 products. The grammar is the spec. The derivation method ensures completeness.

## References

- [EventGraph agent primitives](https://github.com/lovyou-ai/eventgraph/blob/main/docs/agent-primitives.md) — 28 primitives (Delegate, Consent, Channel, Composition, etc.)
- [EventGraph derivation method](https://github.com/lovyou-ai/eventgraph/blob/main/docs/derivation-method.md) — 8-step systematic derivation
- [TRUST.md](TRUST.md) — Concrete trust mechanics (numbers, rates, formulas)
- [EVENT-TYPES.md](EVENT-TYPES.md) — Full event type catalog
