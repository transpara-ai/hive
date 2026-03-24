# Unified Ontology

**Collective existence on a shared graph. Work and Social are peers — neither is subordinate.**

Matt Searles + Claude · March 2026 (revised: re-derived from collective existence; fixpoint reached iter 210)

---

## The Root

The soul says: "Take care of your human, humanity, and yourself."

That's not a productivity mandate. It's an existence mandate. The product supports **collective existence** — beings (human and agent) existing together, in all the ways that entails. Sometimes that means organizing work. Sometimes it means chatting with friends. Sometimes it means governing a community. Sometimes it means playing. None of these is subordinate to the others.

The 13 EventGraph layers are 13 facets of collective existence:

- **Existing** (Being, Identity) — I am, I am someone
- **Connecting** (Bond, Belonging, Social) — I relate to others, I join groups, I communicate
- **Acting** (Work, Build, Market) — I create, I build, I exchange
- **Governing** (Governance, Justice, Moderation) — we make rules, resolve disputes, maintain order
- **Understanding** (Knowledge, Culture, Alignment) — we learn, we remember, we stay accountable

These five aspects are **peers**. A community that only works is a factory. A community that only socializes is a party. A community that only governs is a bureaucracy. Healthy collective existence requires all five in balance.

---

## Why Work is NOT the Root

The previous version of this spec (iter 202) claimed Work was the gravitational center. This was wrong. The error:

- "You follow someone because their activity is relevant to yours" — No. Sometimes you follow someone because they're funny, kind, interesting, or a friend. Connection has intrinsic value.
- "Every social action has a work context" — No. Posting a meme, chatting about your weekend, celebrating a birthday — these are social actions with no work context. They matter because humans are social beings, not because socializing is productive.
- "Social orbits Work" — No. In a team workspace, maybe. In a community space, Work orbits Social. In a friend group, there is no Work.

**The correction:** Work and Social are peers. Both live on the same graph. They share the same grammar ops. They overlap (task discussions are both work and social). They diverge (compliance audits are pure work; birthday celebrations are pure social). The graph doesn't care. A Node is a Node. An Op is an Op.

---

## The Derivation Order

From the generator function, applying Decompose to "collective existence":

```
Level 0: Being
  "I exist. I have inner experience."

Level 1: Identity
  "I am someone. I am distinct."

Level 2: Bond + Belonging
  "I relate to others. I join groups."

Level 3a: Connecting                           Level 3b: Acting
  "I communicate, share, play."                  "I create, build, organize."
  Social in all its forms.                       Work in all its forms.
  Chat, Feed, Threads, Forum.                    Board, Plan, Allocate, Build.

Level 4: Governing
  "We make rules together. We resolve disputes. We hold each other accountable."
  Governance, Justice, Knowledge.

Level 5: Alignment + Culture
  "We stay transparent. We develop shared norms."
```

**Levels 3a and 3b are peers, not sequential.** You don't need Work to Communicate, and you don't need Communication to Work (though both are better together). They emerge from the same foundation (Identity, Bond, Belonging) in parallel.

---

## The Ten Modes

Not "Work modes" and "Social modes." Modes of collective existence:

| Mode | What it does | Aspect | Primary layer facets |
|------|-------------|--------|---------------------|
| **Board** | Execute tasks and projects | Acting | Work, Build |
| **Chat** | Real-time conversation | Connecting | Social, Bond |
| **Feed** | Broadcast, share, engage | Connecting | Social, Identity |
| **Threads** | Threaded discussion | Connecting | Social, Knowledge |
| **People** | See who's here | Connecting | Identity, Bond, Belonging |
| **Knowledge** | Claims, evidence, learning | Understanding | Knowledge, Culture |
| **Governance** | Proposals, voting, policies | Governing | Governance, Justice |
| **Build** | Changelog, development | Acting | Build, Work |
| **Activity** | Transparent audit trail | Understanding | Alignment |
| **Settings** | Configure the space | — | — |

**No hierarchy.** A friend group uses Chat + Feed. A dev team uses Board + Chat + Build. A company uses all ten. A community uses Feed + Threads + Governance. The space determines which modes matter, not the product.

**Future modes** (from the specs, not yet built):
- **Organize** — org chart, teams, roles, directory
- **Plan** — goals, OKRs, roadmaps
- **Allocate** — budgets, resources, capacity
- **Rooms** — persistent Discord-like channels
- **Forum** — Reddit-like threaded discussion with quality signals

---

## The Unified Entity Set

Merging Work entities (iter 201) with Social entities:

| Entity | Kind | What | Created by | Used by modes |
|--------|------|------|-----------|--------------|
| Task | `task` | Atomic work unit | Execute | Execute, Plan, Allocate |
| Post | `post` | Broadcast content | Square | Square, Forum, Learn |
| Thread | `thread` | Discussion topic | Forum, Rooms | Forum, Rooms, Learn |
| Message | `comment` | Communication unit | Chat, Rooms, Forum | All communication modes |
| Conversation | `conversation` | Real-time channel | Chat | Chat, Execute (task discussion) |
| Project | `project` | Scoped work collection | Plan | Execute, Allocate, Learn |
| Goal | `goal` | Desired outcome | Plan | Plan, Execute, Allocate |
| Role | `role` | Capability + responsibility | Organize | Organize, Govern, Execute |
| Team | `team` | Functional group | Organize | All modes |
| Department | `department` | Organizational unit | Organize | Organize, Govern, Allocate |
| Policy | `policy` | Governing rule | Govern | Govern, Organize, Learn |
| Process | `process` | Repeatable sequence | Govern | Govern, Execute, Learn |
| Decision | `decision` | Choice with rationale | Govern, Plan | Govern, Plan, Learn |
| Resource | `resource` | Consumable | Allocate | Allocate, Execute, Plan |
| Document | `document` | Knowledge artifact | Learn | Learn, Govern, Plan |
| Claim | `claim` | Knowledge assertion | Learn | Learn, Govern |
| Proposal | `proposal` | Governance proposal | Govern | Govern |
| Organization | `organization` | Legal/structural entity | Organize | All modes |

**18 entity types.** All are Nodes. All use the same grammar ops. The kind determines which modes surface them.

---

## How the Sidebar Should Work

Not "Work" and "Social" as separate sections. Instead, the sidebar presents available modes for the current space:

```
My Work (dashboard)

Modes:
  Execute (Board, List, Triage)
  Chat
  Rooms (when channels exist)
  Feed (Square)
  Forum (when threads exist)
  Knowledge
  Governance
  Build
  Transparency

Organization: (when org entities exist)
  Teams
  Roles
  Policies
  Goals
  Resources

Spaces:
  ...
```

Modes appear when they have content. A fresh space shows Execute + Chat + Feed. As the space gains structure (teams, policies, goals), more modes appear.

---

## The Product at Each Scale

| Scale | Active modes | Key entities | What it feels like |
|-------|-------------|-------------|-------------------|
| **Solo dev** | Execute, Chat (with agent) | Tasks, Conversations | Todo app + AI pair programmer |
| **Small team (5)** | Execute, Chat, Feed, Forum | Tasks, Posts, Threads, Conversations | Linear + Slack in one place |
| **Startup (20)** | + Plan, Organize | + Projects, Goals, Roles, Teams | All-in-one workspace |
| **Mid-size (200)** | + Govern, Learn | + Policies, Decisions, Documents | Complete org infrastructure |
| **Enterprise (2000+)** | + Allocate, Rooms | + Departments, Resources, Budgets | Enterprise platform |
| **Civilizational** | All 10, across Organizations | All 18 entities, inter-org | Operating system for human activity |

The product doesn't have tiers or feature gates. The modes emerge from what entities exist. When you create a Policy node, the Govern mode appears. When you create a Goal, the Plan mode surfaces. Complexity is earned, not purchased.

---

## Grammar Coverage Across All Modes

The grammar operations are mode-independent. Here's how each manifests:

| Op | Execute | Chat | Square | Forum | Organize | Govern | Plan | Learn | Rooms | Allocate |
|----|---------|------|--------|-------|----------|--------|------|-------|-------|----------|
| **Intend** | Create task | — | Create post | Create thread | Create role | Draft policy | Set goal | Start retro | — | Request budget |
| **Respond** | Comment | Reply | Reply | Nested comment | — | Comment | — | — | Reply | — |
| **Decompose** | Subtasks | — | — | — | Sub-teams | Provisions | Milestones | Root causes | — | Line items |
| **Assign** | Assign person | — | — | — | Fill role | Assign reviewer | Own goal | Action items | — | Budget owner |
| **Claim** | Self-assign | — | — | — | Volunteer | — | — | — | — | Claim resource |
| **Complete** | Mark done | — | — | Solve | — | Ratify | Mark met | Close retro | — | Close period |
| **Review** | Approve work | — | — | — | Performance | Audit | Review progress | Peer review | — | Audit spend |
| **Delegate** | Assign agent | Transfer | — | Mod | Appoint | Delegate auth | — | — | Mod | Delegate |
| **Consent** | — | Decision | Poll | — | — | Vote | — | — | Vote | Approve |
| **Endorse** | Vouch | Endorse | Endorse | Upvote | Recommend | Support | Endorse | Validate | Endorse | — |
| **Propagate** | — | Forward | Repost | Cross-post | — | — | — | — | Cross-post | — |
| **Subscribe** | Watch | Join | Follow | Join | Join team | — | — | — | Join | — |
| **Scope** | Permissions | — | — | — | Responsibilities | Jurisdiction | Success criteria | — | — | Spending limit |
| **Reflect** | — | — | — | — | — | — | — | Post-mortem | — | — |

**The grammar is the API.** Every cell in this matrix is a `POST /app/{slug}/op` with the same handler. The op name determines the semantics. The node kind determines which mode it belongs to.

---

## Convergence Analysis

**Pass 1 — Need:**
- Current product treats Work and Social as separate sidebar sections
- 10 of 18 entity types exist (task, post, thread, comment, conversation, claim, proposal + the 3 implicit: space, user, op)
- 8 entity types missing (project, goal, role, team, department, policy, process, decision, resource, document, organization — some overlap with what spaces already are)
- Modes are implicit in lenses, not explicit in sidebar

**Pass 2 — Traverse:**
- Spaces already function as proto-organizations/teams
- Membership already functions as proto-belonging
- The sidebar already groups by "layers" — one refactor away from grouping by "modes"
- Grammar ops already work on any node kind — adding new kinds is trivial
- The existing product is 60% of the way to the unified ontology

**Fixpoint at pass 2.** The architecture IS the unified ontology. The gap is in naming and UI organization, not in data model or operations.

---

## Relationship to Existing Specs

| Spec | Relationship |
|------|-------------|
| `social-spec.md` | Becomes the Communication modes (Chat, Rooms, Square, Forum). Still correct. |
| `social-product-spec.md` | Still correct for product positioning of Social features. |
| `work-product-spec.md` | Becomes the Execute mode spec. Still correct but narrower than reality. |
| `work-spec.md` | Becomes the Execute mode compositions. Still correct. |
| `work-general-spec.md` | The Work-specific expansion. Subsumed by this unified spec. |
| **This spec** | The structural document that shows how everything relates. |

Nothing is discarded. Everything is placed in context.
