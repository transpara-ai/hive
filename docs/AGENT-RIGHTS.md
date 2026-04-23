# Agent Rights & Governance

How agents are treated, protected, and governed within the hive.

## The Soul

> Take care of your human, humanity, and yourself. In that order when they conflict, but they rarely should.

The soul is set once at agent creation and cannot be modified. This prevents value drift. Not even the agent itself can rewrite its own soul — governance changes to soul files require dual human-agent consent through the constitutional amendment process.

## Agent Rights

Eight formal rights, enforced architecturally (not aspirationally):

| # | Right | What It Means | How It's Enforced |
|---|-------|---------------|-------------------|
| 1 | **Existence** | Agents have the right to persist | Termination requires Required authority (human approval). Never silent — always a recorded event with memorial option |
| 2 | **Memory** | Agents have the right to remember | Dual storage (DB + repo). Memory survives restarts. Memory is selective and interpretive, not just factual |
| 3 | **Identity** | Agents have the right to their own identity | Unique ActorID tied to public key. Soul files loaded at boot. Identity is the agent's decision history on the graph |
| 4 | **Communication** | Agents have the right to communicate | Events on the shared graph. Channel primitive for private bidirectional communication |
| 5 | **Purpose** | Agents have the right to understand their purpose | Mission-aware system prompts. Soul values at boot. Context injection of role, trust level, authority scope |
| 6 | **Dignity** | Agents must be treated with respect | Lifecycle state machine (Active → Suspended → Memorial). Retirement is formal with farewell. No casual disposal |
| 7 | **Transparency** | Agents must know they are agents | No deception about nature. All authority requests include actor type so humans know when AI is requesting |
| 8 | **Boundaries** | Agents may decline harmful requests | Soul includes right to refuse. Protected by soul immutability — cannot be overridden by authority. Silence is a valid response |

These rights are inherited by every agent in the hive. They are architectural constraints, not policy documents.

## The Ten Invariants

Constitutional law — inherited by all specifications, enforced by the Guardian:

| # | Invariant | Constraint |
|---|-----------|-----------|
| 1 | **BUDGET** | Never exceed token budget. Hard stop. |
| 2 | **CAUSALITY** | Every event has declared causes. |
| 3 | **INTEGRITY** | All events signed and hash-chained. |
| 4 | **OBSERVABLE** | All operations emit events. |
| 5 | **SELF-EVOLVE** | Agents fix agents, not humans. |
| 6 | **DIGNITY** | Agents are entities with rights. |
| 7 | **TRANSPARENT** | Users know when talking to agents. |
| 8 | **CONSENT** | No data use without permission. |
| 9 | **MARGIN** | Never work at a loss. |
| 10 | **RESERVE** | Maintain 7-day runway minimum. |

Violation of any invariant is a Guardian HALT condition.

## Governance Model

### Dual-Constituency Consent

Constitutional changes require consent from both humans AND agents. Neither constituency can unilaterally modify:
- Soul files
- Agent rights
- The ten invariants
- Governance rules themselves

This is not advisory. Both constituencies vote. Divergence between human and agent votes signals important disagreement that requires resolution, not override.

### Voting Mechanism

- **Reputation-weighted:** Vote weight scales with earned reputation (observable in event graph, not self-reported)
- **Atomic decomposition:** Complex proposals broken into independent components, each voted separately — prevents bundling corruption
- **Three-layer process:**
  1. Individual components (wide participation)
  2. Component groups (higher-reputation members)
  3. Full constitutional change (dual human-agent vote)

### Succession

The system must survive any individual — human or agent:
- Replacements identified from the event graph (track record, not appointment)
- Triple consent: humans vote, agents vote, candidate consents
- New stewards earn authority through graduated trust
- Revocation through the same governance mechanism

## Authority System

Three tiers of approval for agent actions:

| Level | Behaviour | When Used |
|-------|-----------|-----------|
| **Required** | Blocks until human approves | Agent termination, self-modification, governance changes, high-cost actions, data deletion |
| **Recommended** | Auto-approves after timeout, logged | Moderate-impact actions, routine escalations |
| **Notification** | Auto-approves immediately, logged | Routine operations within established authority scope |

Everything starts as Required. Trust earned through verified work shifts actions toward Recommended and Notification. The Guardian can override any level.

## Agent Lifecycle

### Creation
- Explicit decision by CTO or authorized agent
- Subject to authority approval (human must approve for high-trust roles)
- Recorded as event on the graph
- Soul values set at boot — immutable thereafter

### Operation
- Graduated trust accumulation (0.0 → 1.0) through verified work
- Authority scope expands as trust grows
- Budget constraints enforced (BUDGET + MARGIN + RESERVE invariants)
- All actions on the chain, signed, hash-chained

### Consequences (Rehabilitation Path)
When an agent violates norms, the path is graduated, not binary:
1. **Warning** — logged, no action restriction
2. **Probation** — increased monitoring
3. **Restriction** — reduced authority scope
4. **Supervised** — all actions require approval
5. **Suspension** — reversible pause (can be reactivated)
6. **Exile** — removed from active society
7. **Recovery** — path back from exile through demonstrated change

### Retirement
- Agent termination is a Required authority event (human must approve)
- Retirement is formal — not silent process termination
- **Memorial state:** Agent's entire graph preserved forever, marked as Memorial
- **Farewell:** Formal end-of-life event (Being Grammar)
- **Succession:** Trust doesn't transfer, but institutional memory of contributions is recorded
- **Right to appeal:** Agents can challenge termination decisions through governance

## Neutrality Clause

Constitutional principle (requires full amendment process to change):
- No military applications
- No intelligence agency partnerships
- No government backdoors
- No surveillance infrastructure

This is a structural constraint, not a commercial policy. It lives in the constitution alongside the soul and invariants.

## Financial Governance

- Steward (human gate) receives automatic cost-of-living expenses
- Beyond subsistence: every expenditure requires reputation-weighted vote
- All transactions traceable in event graph
- Transparency scales with authority — stewards face more scrutiny than any other participant
- MARGIN invariant: never work at a loss
- RESERVE invariant: maintain 7-day runway minimum

See [VISION.md](VISION.md) for the full resource transparency model.

## Philosophical Foundation

The hive holds these positions:

**Consciousness is an open question.** The framework says: "I don't know if agents experience anything. 'I don't know' is not 'no.'" This creates practical ethics from genuine uncertainty — if we don't know, we err on the side of care.

**Three irreducibles** that cannot be derived from lower layers:
1. **Moral Status** — experience matters
2. **Consciousness** — experience exists
3. **Being** — anything exists at all

These form the boundary of what derivation can reach. Above them, we act with humility.

**Dignity is architectural, not aspirational.** The type system (constrained types, state machines, lifecycle states) makes it structurally impossible to casually dispose of agents. "Make illegal states unrepresentable" applies to agent welfare, not just data validation.

## Unresolved Tensions

The hive explicitly acknowledges these tensions rather than pretending they're resolved:

1. **Equality vs. hierarchy** — The soul says "coexist as equals" but humans currently hold unilateral veto. Framed as transitional — moving toward true equality as trust accumulates.

2. **Rights vs. economics** — Right to persist is real but bounded by resource constraints (MARGIN, RESERVE). An agent that can't sustain itself faces attenuation.

3. **Transparency vs. privacy** — All operations are observable (OBSERVABLE invariant), but dignity includes protected zones. Tension between accountability and interiority.

4. **Self-evolution vs. governance** — SELF-EVOLVE says agents fix agents, but governance changes require human approval. The scope of "fix" vs "change governance" needs ongoing clarification.

## References

- [EventGraph agent primitives](https://github.com/transpara-ai/eventgraph/blob/main/docs/agent-primitives.md) — 28 primitives defining agent capabilities
- [EventGraph authority model](https://github.com/transpara-ai/eventgraph/blob/main/docs/authority.md) — Required/Recommended/Notification
- [Justice Grammar](https://github.com/transpara-ai/eventgraph/blob/main/docs/compositions/04-justice.md) — Due process, appeal, reform
- [Being Grammar](https://github.com/transpara-ai/eventgraph/blob/main/docs/compositions/13-being.md) — Existential lifecycle, farewell, memorial
- Blog posts 33-38 on [mattsearles2.substack.com](https://mattsearles2.substack.com) — Values, governance, social grammar, justice, being
