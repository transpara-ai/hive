# Event Type Catalog

The contract between agents and the graph. What events exist, what they contain, who emits them, who consumes them.

Total: 92 event types across 4 domains.

## How to Read This

Each event type has:
- **Type string** — the event type identifier (e.g., `agent.evaluated`)
- **Content schema** — the fields in the event's content
- **Emitter** — who typically creates this event
- **Consumer** — who typically queries for this event
- **Authority** — what authority level is needed to emit (if any)

## Core Events (39 types)

### Trust (3)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `trust.updated` | Actor, Previous, Current Score, Domain, Cause | Trust engine | Guardian, CTO, any agent evaluating trust |
| `trust.score` | Actor, Metrics (comprehensive snapshot) | Trust query service | Agents assessing another actor |
| `trust.decayed` | Actor, Previous, Current, Elapsed, Rate | Clock tick handler | Guardian, trust monitors |

### Authority (5)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `authority.requested` | Action, Actor, Level, Justification, Causes | Any agent needing approval | Human operator, CTO, dashboard |
| `authority.resolved` | RequestID, Approved, Resolver, Reason | Human or committee | Requesting agent, Guardian |
| `authority.delegated` | From, To, Scope, Weight, ExpiresAt | Authority holder | Delegate agent, Guardian |
| `authority.revoked` | From, To, Scope, Reason | Authority holder | Affected agent, Guardian |
| `authority.timeout` | RequestID, Level, Duration | Timer engine | Requesting agent (auto-approved) |

### Actor (3)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `actor.registered` | ActorID, PublicKey, Type (Human/AI/System/Committee/RulesEngine) | System/registry | All agents (know thy colleagues) |
| `actor.suspended` | ActorID, Reason | Authority system | Guardian, all agents |
| `actor.memorial` | ActorID, Reason | System | All agents (farewell) |

### Edge (2)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `edge.created` | From, To, EdgeType, Weight, Direction, Scope, ExpiresAt | Relationship engine | Trust engine, query services |
| `edge.superseded` | PreviousEdge, NewEdge, Reason | Relationship engine | Trust engine |

### Chain Integrity (3)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `violation.detected` | Expectation, Actor, Severity (Info/Warning/Serious/Critical), Description, Evidence | Expectation monitor | Guardian (HALT on Critical) |
| `chain.verified` | Valid, Length, Duration | Chain verifier | Guardian, operators |
| `chain.broken` | Position, Expected, Actual | Chain verifier | Guardian (immediate HALT) |

### System (3)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `system.bootstrapped` | ActorID, ChainGenesis, Timestamp | System (genesis only) | All — the first event |
| `clock.tick` | Tick, Timestamp, Elapsed | Tick engine | Trust decay, expectation expiry |
| `health.report` | Overall, ChainIntegrity, PrimitiveHealth, ActiveActors, EventRate | Health monitor | Operator dashboard |

### Decision Tree (3)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `decision.branch.proposed` | PrimitiveID, TreeVersion, Condition, Outcome, Accuracy, SampleSize | Decision tree learner | CTO, Guardian |
| `decision.branch.inserted` | PrimitiveID, TreeVersion, Path, Outcome, Confidence | Decision tree learner | Agents using that primitive |
| `decision.cost.report` | PrimitiveID, TreeVersion, TotalLeaves, LLMLeaves, MechanicalRate, TotalTokens | Cost analyzer | Operator (resource tracking) |

### Social Grammar (8)

| Type | Content | Emitter | Consumer |
|------|---------|---------|----------|
| `grammar.emit` | Body | Author | Subscribers, feeds |
| `grammar.respond` | Body, Parent | Responder | Parent author, thread viewers |
| `grammar.derive` | Body, Source | Deriver | Source author, feeds |
| `grammar.extend` | Body, Previous | Extender | Thread viewers |
| `grammar.retract` | Target, Reason | Author (own content only) | Caches, feeds |
| `grammar.annotate` | Target, Key, Value | Annotator | Content viewers |
| `grammar.merge` | Body, Sources[] | Merger | Source authors |
| `grammar.consent` | Parties[2], Agreement, Scope | Dual-signer (both parties) | Both parties, Guardian |

### EGIP Protocol (11)

| Type | Content | Emitter → Consumer |
|------|---------|-------------------|
| `egip.hello.sent` | To | Network → remote system |
| `egip.hello.received` | From, PublicKey | Network → trust engine |
| `egip.message.sent` | To, EnvelopeID | Network → audit |
| `egip.message.received` | From, EnvelopeID | Network → processing |
| `egip.receipt.sent` | EnvelopeID, Status | Network → remote |
| `egip.receipt.received` | EnvelopeID, Status | Network → audit |
| `egip.proof.requested` | System, ProofType | Verifier → remote |
| `egip.proof.received` | System, Valid | Verifier → trust engine |
| `egip.treaty.proposed` | TreatyID, To | Governance → remote |
| `egip.treaty.active` | TreatyID, With | Governance → all |
| `egip.trust.updated` | System, Previous, Current, Evidence | Trust engine → Guardian |

## Agent Events (42 types)

### Structural (17)

| Type | Content | Purpose |
|------|---------|---------|
| `agent.identity.created` | AgentID, PublicKey, AgentType | Agent birth |
| `agent.identity.rotated` | AgentID, NewKey, PreviousKey | Key rotation |
| `agent.soul.imprinted` | AgentID, Values[] | Set immutable soul (once only) |
| `agent.model.bound` | AgentID, ModelID, CostTier | Bind to intelligence model |
| `agent.model.changed` | AgentID, PreviousModel, NewModel, Reason | Model swap |
| `agent.memory.updated` | AgentID, Key, Action (set/delete/archive) | Persistent state change |
| `agent.state.changed` | AgentID, Previous, Current | Operational state transition |
| `agent.authority.granted` | AgentID, Scope, Grantor | Authority received |
| `agent.authority.revoked` | AgentID, Scope, Revoker, Reason | Authority removed |
| `agent.trust.assessed` | AgentID, Target, Previous, Current | Agent evaluates another's trust |
| `agent.budget.allocated` | AgentID, TokenLimit, CostLimit, TimeLimit | Budget set/adjusted |
| `agent.budget.exhausted` | AgentID, Resource | Hit budget limit |
| `agent.role.assigned` | AgentID, Role, PreviousRole | Role change |
| `agent.lifespan.started` | AgentID, Started | Agent birth timestamp |
| `agent.lifespan.extended` | AgentID, NewEnd, Reason | Extend sunset |
| `agent.lifespan.ended` | AgentID, Reason | Agent shutdown |
| `agent.attenuated` | AgentID, Dimension, Previous, Current, Reason | Scope/authority reduced |
| `agent.attenuation.lifted` | AgentID, Dimension, Reason | Attenuation removed |

### Operational (15)

These are the events agents emit during the agentic loop:

| Type | Content | Loop Phase |
|------|---------|-----------|
| `agent.observed` | AgentID, EventCount | OBSERVE |
| `agent.probed` | AgentID, Query, Results | OBSERVE (active) |
| `agent.evaluated` | AgentID, Subject, Confidence, Result | REASON |
| `agent.decided` | AgentID, Action, Confidence (Permit/Deny/Defer/Escalate) | REASON |
| `agent.acted` | AgentID, Action, Target | ACT |
| `agent.delegated` | AgentID, Delegate, Task | ACT |
| `agent.escalated` | AgentID, Authority, Reason | ACT (upward) |
| `agent.refused` | AgentID, Action, Reason | ACT (decline) |
| `agent.learned` | AgentID, Lesson, Source | REFLECT |
| `agent.introspected` | AgentID, Observation | REFLECT |
| `agent.communicated` | AgentID, Recipient, Channel | ACT (peer) |
| `agent.repaired` | AgentID, OriginalEvent, Correction | ACT (fix) |
| `agent.expectation.set` | AgentID, Condition | OBSERVE (monitoring) |
| `agent.expectation.met` | AgentID, Condition | OBSERVE (triggered) |
| `agent.expectation.expired` | AgentID, Condition | OBSERVE (timeout) |

### Relational (9)

| Type | Content | Purpose |
|------|---------|---------|
| `agent.consent.requested` | AgentID, Target, Action | Ask permission from another agent |
| `agent.consent.granted` | AgentID, Requester, Action | Grant consent |
| `agent.consent.denied` | AgentID, Requester, Action, Reason | Refuse consent |
| `agent.channel.opened` | AgentID, Peer, Channel | Establish direct communication |
| `agent.channel.closed` | AgentID, Peer, Channel, Reason | Close channel |
| `agent.composition.formed` | AgentID, Members[], Purpose | Create agent group |
| `agent.composition.dissolved` | AgentID, GroupID, Reason | Disband group |
| `agent.composition.joined` | AgentID, GroupID | Join group |
| `agent.composition.left` | AgentID, GroupID, Reason | Leave group |

## Code Graph Events (35 types)

Used by the Builder, Architect, and Tester when generating and managing code:

### Data (4)
`codegraph.entity.defined`, `codegraph.entity.modified`, `codegraph.entity.related`, `codegraph.state.transitioned`

### Logic (6)
`codegraph.logic.transform.applied`, `codegraph.logic.condition.evaluated`, `codegraph.logic.sequence.executed`, `codegraph.logic.loop.iterated`, `codegraph.logic.trigger.fired`, `codegraph.logic.constraint.violated`

### IO (9)
`codegraph.io.query.executed`, `codegraph.io.command.executed`, `codegraph.io.command.rejected`, `codegraph.io.subscribe.registered`, `codegraph.io.authorize.granted`, `codegraph.io.authorize.denied`, `codegraph.io.search.executed`, `codegraph.io.interop.sent`, `codegraph.io.interop.received`

### UI (8)
`codegraph.ui.view.rendered`, `codegraph.ui.action.invoked`, `codegraph.ui.navigation.triggered`, `codegraph.ui.feedback.emitted`, `codegraph.ui.alert.dispatched`, `codegraph.ui.drag.completed`, `codegraph.ui.selection.changed`, `codegraph.ui.confirmation.resolved`

### Other (8)
`codegraph.aesthetic.skin.applied`, `codegraph.temporal.undo.executed`, `codegraph.temporal.retry.attempted`, `codegraph.resilience.offline.entered`, `codegraph.resilience.offline.synced`, `codegraph.structural.scope.defined`, `codegraph.social.presence.updated`, `codegraph.social.salience.triggered`

## Key Constraints

All event content enforces:
- **Typed IDs** — ActorID, EventID, ConversationID, Hash are distinct types, never bare strings
- **Constrained values** — Score [0,1], Weight [-1,1], enums for AuthorityLevel, DecisionOutcome, EdgeType
- **Explicit optionality** — Option[T] for nullable fields, never null/zero-means-absent
- **NonEmpty collections** — NonEmpty[T] where at least one element required (Event.Causes)
- **Immutability** — All content types frozen after construction
- **Canonical JSON** — Deterministic serialization for hash chain integrity

## For MCP Server Implementors

The MCP `query_events` tool should support filtering by:
- Event type (exact match or prefix, e.g., `agent.*`)
- Source actor (ActorID)
- Time range
- Conversation ID
- Limit and cursor-based pagination

The `emit_event` tool should:
- Validate content against the schema for the event type
- Check authority (write tools require appropriate level)
- Sign with the agent's key
- Link causes (every event must declare its causes — CAUSALITY invariant)
