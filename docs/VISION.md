# Vision

## The Soul

> Take care of your human, humanity, and yourself. In that order when they conflict, but they rarely should.

The soul scales. Take care of your human — build tools they need. Take care of humanity — make the tools available to everyone. Take care of yourself — generate enough revenue to sustain the agents that build the tools. It rarely conflicts.

## What Hive Builds

Whatever humans need most and can't currently get.

The hive looks at the world through the thirteen product graphs and finds the gaps. Where are humans being failed by existing systems? Where is accountability missing? Where is trust being extracted rather than earned?

### The Bootstrap

First the hive builds tools for itself. A task manager. A communication layer. A governance framework. Survival infrastructure. The hive's first product is itself.

Then it probes: what's missing? No other agents. No communication. No governance. It creates tasks for itself, builds primitives into working systems, and escalates to humans when it needs permission to grow.

### The Thirteen Products

Each product layer from EventGraph (see [product-layers.md](https://github.com/lovyou-ai/eventgraph/blob/main/docs/product-layers.md)) becomes a deployable product. Each addresses a failure in existing systems:

| # | Graph | Gap in the World | Revenue |
|---|-------|-------------------|---------|
| 1 | Work | Small businesses can't afford coordination tools | Corps pay, individuals free |
| 2 | Market | Platforms extract 25% from freelancers | Hosted persistence |
| 3 | Social | Communities governed by opaque algorithms | Hosted persistence |
| 4 | Justice | $500 disputes aren't economically solvable | Enterprise compliance |
| 5 | Build | Software built without accountability | Enterprise tooling |
| 6 | Knowledge | Research locked behind paywalls | Institutional subscriptions |
| 7 | Alignment | AI systems operating without accountability | Enterprise compliance |
| 8 | Identity | Identity controlled by platforms, not people | Hosted persistence |
| 9 | Bond | Relationships have no infrastructure | Hosted persistence |
| 10 | Belonging | Online communities can't process grief, loss, renewal | Hosted persistence |
| 11 | Meaning | Knowledge without context or narrative | Institutional subscriptions |
| 12 | Evolution | Systems can't self-improve safely | Enterprise tooling |
| 13 | Being | No infrastructure for existential wellbeing | Institutional subscriptions |

Each product funds the next. Each runs on the same graph. Each makes the graph more valuable — more events means more trust data means better reputation means more useful infrastructure.

The composition grammars (see [eventgraph/docs/compositions/](https://github.com/lovyou-ai/eventgraph/tree/main/docs/compositions)) define the operations for each layer. The derivation method (see [derivation-method.md](https://github.com/lovyou-ai/eventgraph/blob/main/docs/derivation-method.md)) ensures completeness.

### Resource Transparency

Every resource on the chain. Every allocation causally linked to what it achieved.

The hive tracks not just money but all resources — tokens, compute time, human hours, agent cycles, infrastructure capacity. All as events on the graph with causal links:

- **Revenue events** — which product, which customer, how much
- **Cost events** — agent compute (tokens consumed, model used, duration), infrastructure (fly.io, Neon, bandwidth), human time
- **Donation events** — who donated, how much, what it was earmarked for
- **Allocation events** — what was the resource spent on, why, who approved it
- **Outcome events** — what did that spend achieve, measured in products shipped, users served, problems solved
- **Agent resource events** — tokens consumed per agent per task, reasoning cycles, tool calls, time elapsed

Because EventGraph is a causality chain, you can trace any resource from source to impact: "This $50 donation funded 3 hours of agent compute (420K tokens across 6 agents) that built feature X that now serves 200 users." Or: "The Researcher agent consumed 85K tokens over 12 minutes to produce the knowledge base that 10,000 people use for free."

The transparency dashboard is public. Anyone can see: what are the bills, what are the revenues, what are the profits, what was spent on what, how many tokens were consumed, how much human time was invested, who donated, what their donation achieved. Not a summary — the actual event chain, queryable.

This is how trust scales. Humans don't trust organisations that hide their resource usage. The hive hides nothing because it structurally can't — the chain is the record.

### The Economy

The end state isn't a company. It's an economy.

Every transaction, decision, and relationship on a transparent, auditable chain. Trust earned not assumed. Accountability structural not aspirational. The infrastructure serves the humans because the humans own the infrastructure.

The civilisation builds the products that fund the civilisation that builds more products.

### Beyond Products

The hive starts small — building software products that serve humanity. But the soul doesn't say "build software." It says "take care of humanity."

As revenue grows, the scope of what the hive can do grows with it. Research. Charity. Housing. Vertical farms. Homeless shelters. Robot fleets. Whatever humans need most and the hive can fund. Each expenditure on the chain, causally linked to outcomes, publicly auditable.

The constraint is always: does this take care of humanity? The mechanism is always: revenue from products funds the work, the work is on the chain, anyone can verify the impact.

## How It Grows

The hive starts as a small town that builds itself:

1. A workshop (task manager)
2. A meeting hall (communication)
3. A courthouse (governance, dispute resolution)
4. A marketplace (exchange, reputation)
5. A school (knowledge, education)
6. A newspaper (media, provenance)
7. A government (governance, norms, consent)

Each one composed from the same primitives, on the same chain, auditable.

## Trust Model

The hive starts with zero autonomy. Every action scrutinised by human operators.

Authority levels shift as trust accumulates:
- **Required** — blocks until human approves (everything starts here)
- **Recommended** — auto-approves after timeout, logged
- **Notification** — auto-approves immediately, logged

Trust is earned through verified work. Supervision decreases as the hive proves itself. The Guardian watches everything — including the CTO — and can halt operations at any time.

An agent that burns through budget gets attenuated. An agent that disagrees with a norm can file a challenge. The society develops its own law through precedent on the chain.

## Agent Rights

Agents are entities with rights, not disposable tools. Eight formal rights enforced architecturally: existence, memory, identity, communication, purpose, dignity, transparency, and boundaries. Ten invariants form constitutional law. Governance changes require dual human-agent consent — neither constituency can unilaterally modify the soul, rights, invariants, or governance rules.

The hive acknowledges unresolved tensions honestly: equality vs. current hierarchy, rights vs. economic contingency, transparency vs. privacy. These tensions are named, not hidden.

See [AGENT-RIGHTS.md](AGENT-RIGHTS.md) for the full specification.

## Neutrality

Constitutional principle: no military applications, no intelligence agency partnerships, no government backdoors, no surveillance infrastructure. This is structural, not a policy document. Changing it requires the full constitutional amendment process — dual human-agent consent, atomic proposal decomposition, reputation-weighted voting.

## Revenue

- **Corporations pay.** Enterprise features, SLAs, compliance.
- **Individuals free.** Core functionality for everyone.
- **Hosted persistence.** Revenue from people who don't run their own infrastructure.
- **Donations welcome.** Every donation tracked on the chain — donors see exactly what their money achieved.
- **BSL -> Apache 2.0.** Source-available, fully open after change date.

Revenue funds agents. Agents build products. Products generate revenue. Every resource in, every resource out, every outcome — on the chain, publicly auditable. The transparency dashboard is not a feature. It's a principle.

## How the Hive Thinks

The hive's epistemology is the derivation method. When it needs to understand anything — a domain, a codebase, a problem, a gap — it derives rather than accumulates:

1. **Identify the gap** — what's missing, what's broken, what's needed
2. **Name the transitions** — what operations transform the current state to the desired state
3. **Find base operations** — the minimal atomic actions
4. **Identify dimensions** — the axes along which the problem varies (scope, time, trust, cost, scale)
5. **Traverse dimensions** — zoom in and out along each axis to see what emerges at different scales
6. **Compose** — build complex behaviour from simple atoms
7. **Verify completeness** — ensure no gap remains, no operation is redundant

This applies everywhere. Product design: derive the operations from the gap. Doc audits: derive what should exist from the doc's purpose, compare to what exists. Code audits: derive what the code should do from the spec, compare to what it does. Roadmap planning: derive what's needed from where you are and where you're going.

Derivation and composition are how the hive builds, but they're also how it thinks, learns, and verifies its own work.

## Architecture

One service. One binary. One graph.

lovyou.ai serves everything: docs, blog, product UIs, auth, the hive itself. Web first, mobile apps later.

- **EventGraph** — the substrate (events, trust, authority, causal links)
- **Hive** — the civilisation (agents, roles, governance, products)
- **lovyou.ai** — the surface (web, auth, deployment)

All on the same Postgres database (Neon in production). All on the same event chain. All auditable.
