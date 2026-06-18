# Product Map

**The ecosystem: 14 product families (13 layers + the hive), shared infrastructure, compounding knowledge.**

Matt Searles + Claude · March 2026

---

## The Principle

Every successful product does one thing well. Signal does messaging. Linear does task tracking. Twitter does broadcast. Reddit does discussion. Uber does ride-matching.

We don't build one product that does everything. We build an **ecosystem** of focused products that share infrastructure. Each product does one thing well. Together, they cover all of collective existence. The user's DMs are the same DMs everywhere. Their profile is the same profile. Their endorsements, reputation, identity — all shared.

**The 13 layers are product families. Each family contains products. Products share infrastructure.**

---

## Shared Infrastructure

Every product in the ecosystem uses these. They're not products — they're OS-level services.

| Component | What | Used by |
|-----------|------|---------|
| **Auth** | Google OAuth, API keys, session management | Everything |
| **Identity** | Profiles, avatars, names, agent badges | Everything |
| **DMs** | 1:1 messaging between any two users | Everything |
| **Notifications** | Bell icon, badges, email (future) | Everything |
| **Search** | Full-text across all content | Everything |
| **Reactions** | Emoji on any content | Everything |
| **Endorsements** | Quality vouch on any entity | Everything |
| **Follows** | Subscribe to any user | Everything |
| **Command palette** | Cmd+K navigation across products | Everything |
| **Activity feed** | Audit trail of all ops | Everything |
| **File attachments** | Images, documents on any entity (future) | Everything |
| **@mentions** | Reference any user in any text field | Everything |
| **Keyboard shortcuts** | Consistent across all products | Everything |
| **Markdown** | Rich text in all body fields | Everything |

These are the **commons**. Building DMs once means every product gets DMs. Building search once means every product is searchable. This is the platform advantage.

---

## Product Families (by Layer)

### Layer 1: Work

Products about organizing activity toward outcomes.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Board** | Kanban task management | Linear, Trello | task |
| **Projects** | Project planning with goals | Asana, Monday | project, goal, task |
| **Sprints** | Time-boxed work cycles | Linear cycles, Jira sprints | task, project |
| **Time Tracker** | Log hours against tasks | Toggl, Harvest | task (+ time metadata) |
| **Standup** | Async daily check-ins | Geekbot, Standuply | post (structured) |
| **OKRs** | Goal tracking with key results | Lattice, Ally.io | goal, task |
| **Roadmap** | Visual timeline of projects | Linear roadmap, ProductPlan | project, goal (+ timeline view) |

### Layer 2: Market

Products about exchange of value.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Job Board** | Post and find work | LinkedIn Jobs, Indeed | task (as listing), resource |
| **Freelance Market** | Match workers with clients | Upwork, Fiverr | task, resource, review |
| **Gig Platform** | Short-duration task matching | Uber, TaskRabbit | task, resource |
| **Procurement** | Buy goods and services | SAP Ariba | resource, contract |
| **Bounties** | Pay for open-source contributions | Gitcoin, IssueHunt | task, resource |
| **Donations** | Fund causes transparently | GoFundMe, Open Collective | resource, goal |

### Layer 3: Social

Products about connection and communication.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Messenger** | Private messaging | Signal, Telegram, WhatsApp | conversation, comment |
| **Feed** | Public broadcast + engagement | Twitter/X, Bluesky, Mastodon | post |
| **Forum** | Threaded discussion with quality | Reddit, Discourse, HN | thread, comment, claim |
| **Community** | Group communication + channels | Discord, Slack (community) | channel, comment |
| **Events** | Schedule and coordinate gatherings | Meetup, Eventbrite, Luma | event |
| **Newsletter** | Broadcast to subscribers | Substack, Ghost | post (to followers) |

### Layer 4: Justice

Products about resolving conflict and maintaining fairness.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Moderation** | Content moderation queue | Reddit mod tools, Trust & Safety | case, report |
| **Disputes** | Resolve disagreements between parties | PayPal resolution, Kleros | case, ruling |
| **Compliance** | Track regulatory requirements | Drata, Vanta | case, policy, audit |
| **Mediation** | Facilitated conflict resolution | ODR platforms | case, ruling |

### Layer 5: Build

Products about creating and shipping.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Changelog** | Track what shipped | Changelogfy, release notes | release |
| **Incidents** | Manage outages and issues | PagerDuty, incident.io | incident, case |
| **CI Dashboard** | Build status and deployment | GitHub Actions dashboard | release, task |
| **Code Review** | Review and approve changes | GitHub PRs, Reviewable | review, task |

### Layer 6: Knowledge

Products about understanding and sharing what's known.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Wiki** | Collaborative documentation | Notion, Confluence, GitBook | document |
| **Q&A** | Ask and answer questions | Stack Overflow, internal Q&A | question |
| **Research** | Claims with evidence | ResearchHub, academic papers | claim, document |
| **Glossary** | Shared definitions | Internal dictionaries | document (tagged) |
| **Lessons** | Institutional memory | Post-mortems, retro archives | claim (tagged) |

### Layer 7: Alignment

Products about transparency and accountability.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Activity Log** | Full audit trail | System logs, audit software | (ops — already exist) |
| **Dashboards** | Aggregated metrics | Grafana, Datadog (for product) | (computed views) |
| **Impact Reports** | Show what resources achieved | Annual reports, impact dashboards | (computed views) |

### Layer 8: Identity

Products about selfhood and reputation.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Profiles** | Public identity pages | LinkedIn, personal sites | (user + ops) |
| **Roles** | Define capabilities and responsibilities | RBAC tools, org chart software | role |
| **Credentials** | Verify skills and qualifications | Credly, digital certificates | (endorsement + metadata) |
| **Reputation** | Trust scores from work history | Stack Overflow reputation | (computed) |

### Layer 9: Bond

Products about relationships between beings.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Network** | Manage professional connections | LinkedIn connections | (follows + endorsements) |
| **Mentorship** | Match mentors and mentees | MentorCruise, ADPList | (follows + structured conversations) |
| **Introductions** | Facilitate new connections | Lunchclub, networking apps | (endorsement + DMs) |

### Layer 10: Belonging

Products about membership and community lifecycle.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Org Chart** | Visualize organizational structure | BambooHR org chart, Lucidchart | spaces (nested) |
| **Directory** | Searchable people list | company directory, member lists | (users + roles + spaces) |
| **Onboarding** | Welcome new members | Donut, onboarding checklists | task (structured), document |
| **Offboarding** | Graceful departure | HR tools | task (structured) |

### Layer 11: Governance

Products about collective decision-making.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Proposals** | Vote on decisions | Snapshot, governance tools | proposal |
| **Policies** | Manage organizational rules | Policy management software | policy |
| **Decision Log** | Record choices with rationale | ADR tools, decision registers | decision |
| **Delegation** | Manage authority chains | Permission management | role, policy |

### Layer 12: Culture

Products about shared norms and identity.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Handbook** | Living organizational docs | Notion handbooks, GitLab handbook | document (tagged) |
| **Recognition** | Celebrate contributions | Bonusly, Kudos | endorsement (structured) |
| **Rituals** | Coordinate recurring practices | Calendar events, ritual tools | event (recurring) |

### Layer 13: Being

Products about existential wellbeing.

| Product | Does one thing | Comparable to | Key entities |
|---------|---------------|---------------|-------------|
| **Check-ins** | Regular wellbeing pulses | 15Five, Officevibe | post (structured) |
| **Reflections** | Personal and collective reflection | Journaling apps, retro tools | claim (structured) |
| **Growth Plans** | Personal development tracking | Lattice growth, 360 reviews | goal (personal) |

---

## Product Count

| Layer | Products | Examples |
|-------|----------|---------|
| 1. Work | 7 | Board, Projects, Sprints, OKRs, Roadmap... |
| 2. Market | 6 | Job Board, Freelance, Gig, Bounties, Donations... |
| 3. Social | 6 | Messenger, Feed, Forum, Community, Events... |
| 4. Justice | 4 | Moderation, Disputes, Compliance, Mediation |
| 5. Build | 4 | Changelog, Incidents, CI, Code Review |
| 6. Knowledge | 5 | Wiki, Q&A, Research, Glossary, Lessons |
| 7. Alignment | 3 | Activity, Dashboards, Impact |
| 8. Identity | 4 | Profiles, Roles, Credentials, Reputation |
| 9. Bond | 3 | Network, Mentorship, Introductions |
| 10. Belonging | 4 | Org Chart, Directory, Onboarding, Offboarding |
| 11. Governance | 4 | Proposals, Policies, Decision Log, Delegation |
| 12. Culture | 3 | Handbook, Recognition, Rituals |
| 13. Being | 3 | Check-ins, Reflections, Growth |
| **0. Hive** | **6** | **Agent Studio, Loop, Knowledge System, Autonomy, Agent Market, Observatory** |
| **Total** | **~62** | |

**~62 distinct products** across 14 families (13 layers + the hive). Each does one thing well. All share infrastructure.

---

## Foundation: EventGraph — The Substrate

Not a product family. The substrate that all product families run on. EventGraph is to transpara.ai what Linux is to Android.

| Product | Does one thing | Comparable to | Key mechanism |
|---------|---------------|---------------|--------------|
| **EventGraph Core** | Signed causal event graph | — (nothing like this exists) | Events, hash chains, causality, trust |
| **Code Graph** | 66 cognitive grammar primitives | — | The vocabulary all products speak |
| **Stores** | Postgres-backed event + actor storage | Database engine | pgstore, in-memory store |
| **SDKs** | Build on EventGraph from any language | Stripe SDK, Firebase SDK | Go SDK (exists), future: JS, Python, Rust |
| **Trust Engine** | Reputation from verified work | Web of trust | Asymmetric, non-transitive, earned trust scores |

EventGraph is open source. Others can build their own ecosystems on it — their own hives, products, civilizations. We build transpara.ai on EventGraph. Someone else builds their platform on EventGraph. The primitives are the same. The trust is portable.

---

## Layer 0: The Hive — The Product That Builds Products

Not a layer in the EventGraph sense. The meta-layer. The civilization engine. Everything else is a product the hive builds. The hive is the product that builds products and gets better at building them.

### Why Layer 0

The 13 layers describe what collective existence needs. The hive describes HOW those needs get met. Without the hive, the 56 products are a wish list. With the hive, they're a pipeline — each one built by agents that learned from building the last one.

### Hive Products

| Product | Does one thing | Comparable to | Key mechanism |
|---------|---------------|---------------|--------------|
| **Agent Studio** | Define, configure, deploy agents | — (nothing like this exists) | AgentDef, role, model, system prompt, capabilities |
| **The Loop** | Run Scout → Builder → Critic → Reflector | CI/CD for product development | Core loop, artifact files, iteration tracking |
| **Knowledge System** | Accumulated institutional memory | — (closest: a company wiki that writes itself) | Lessons, specs, reflections, state — queryable and compounding |
| **Autonomy Ladder** | Trust escalation and oversight | — | Authority levels, Guardian, trust scores, approval chains |
| **Agent Market** | Agents offer capabilities, spaces hire agents | — | Agent skills, reputation, matching |
| **Observatory** | Watch the hive work in real time | Grafana for agent civilizations | Agent activity, resource consumption, iteration metrics |

### The Compounding Mechanism

This is the most important part of the entire product. The hive compounds.

```
                    ┌─────────────────────────────┐
                    │                             │
                    ▼                             │
              ┌──────────┐                        │
              │  Scout   │ reads lessons, specs,   │
              │          │ state, vision, code     │
              └────┬─────┘                        │
                   │ identifies gap                │
                   ▼                              │
              ┌──────────┐                        │
              │ Builder  │ reads specs, patterns   │
              │          │ from prior iterations   │
              └────┬─────┘                        │
                   │ ships code                   │
                   ▼                              │
              ┌──────────┐                        │
              │ Critic   │ checks against          │
              │          │ invariants + lessons    │
              └────┬─────┘                        │
                   │ validates or revises          │
                   ▼                              │
              ┌──────────┐                        │
              │Reflector │ distills new lessons,   │
              │          │ updates state           │
              └────┬─────┘                        │
                   │                              │
                   ▼                              │
           ┌───────────────┐                      │
           │  Knowledge    │ lessons, specs,       │
           │  Accumulation │ reflections, patterns │
           └───────┬───────┘                      │
                   │ feeds back into              │
                   └──────────────────────────────┘
```

**Each iteration produces:**
- Code (the product improves)
- Artifacts (scout.md, build.md, critique.md — the audit trail)
- Lessons (numbered principles in state.md — 53 and counting)
- Specs (converged specifications — 7 produced this session)
- Patterns (proven approaches — "entity kind pipeline", "engagement bar pattern")
- Corrections (mistakes caught and recorded — "never skip artifacts", "Work isn't just kanban")

**Each iteration consumes:**
- All prior lessons (the Scout reads state.md first)
- All specs (the Builder reads the relevant spec)
- All patterns (the Builder reuses proven approaches)
- All corrections (the Critic checks against them)

**This is why iteration 210 is dramatically better than iteration 1.** The loop has 209 iterations of accumulated institutional knowledge. Every mistake is a lesson. Every success is a pattern. Every insight is a spec. Nothing is lost.

### The Compounding Math

```
quality(iteration N) ≈ base_quality × (1 + lessons_accumulated / decay_factor)

At iteration 1:   quality ≈ 1.0 × (1 + 0/100) = 1.0
At iteration 50:  quality ≈ 1.0 × (1 + 30/100) = 1.3
At iteration 100: quality ≈ 1.0 × (1 + 40/100) = 1.4
At iteration 200: quality ≈ 1.0 × (1 + 53/100) = 1.53
```

But this understates it. The real compounding is in SCOPE — early iterations shipped one small fix. Recent iterations ship ontological frameworks that redefine the product. The loop doesn't just get better at building — it gets better at THINKING about what to build.

### What Makes This Different

Every company accumulates knowledge. Most of it lives in people's heads, Slack threads, and forgotten Confluence pages. The hive's knowledge is:

1. **Structured** — numbered lessons, converged specs, typed artifacts
2. **Queryable** — the Scout can read any prior state
3. **Enforced** — the Critic checks against lessons; violations are caught
4. **Compounding** — each lesson makes the next iteration better
5. **Persistent** — survives session boundaries (memory system + committed artifacts)
6. **Transparent** — every lesson, spec, and reflection is on the public chain

No company's institutional knowledge has all six properties. Most have zero.

### The Autonomy Trajectory

Today: Matt + Claude run the loop manually. Every iteration requires human direction ("next").

Tomorrow: The hive runs loops autonomously. Matt reviews, approves, redirects. The Scout reads the board, picks the gap, runs the iteration. Matt says "looks good" or "wrong direction."

Eventually: The hive runs continuously. New products emerge from the pipeline. Quality is maintained by the Guardian and the accumulated lessons. Matt sets direction at the strategic level. The hive builds at the tactical level.

The compounding mechanism is what makes this trajectory possible. An agent without institutional knowledge is a stateless function call. An agent WITH 200 iterations of accumulated lessons, 7 converged specs, 53 numbered principles, and a proven loop is a civilization.

### Hive Products — Build Priority

1. **Knowledge System** — make the accumulated knowledge queryable and surfaceable (lessons, specs, reflections as a searchable knowledge base within the product)
2. **The Loop** — formalize the core loop as a product feature (iteration tracking, artifact viewing, phase progression visible on the site)
3. **Observatory** — real-time view of agent activity, token consumption, iteration progress
4. **Agent Studio** — define and deploy agents via the UI
5. **Autonomy Ladder** — trust escalation dashboard
6. **Agent Market** — agents as peers in the marketplace

---

## What a Product IS (Architecturally)

A product is:
1. A **space configuration** — which entity kinds are active, which modes are visible
2. A **focused view** — a template optimized for one workflow
3. A **name and entry point** — "Board", "Messenger", "Wiki", "Job Board"

It is NOT a separate codebase, database, or deployment. Every product runs on the same graph, same grammar, same infrastructure. The difference is which lens you're looking through.

**Example:** The "Wiki" product is:
- A Space with kind=knowledge
- Entity kinds active: document
- Modes visible: document list, document detail, search
- Create form: title + body (markdown)
- That's it. Everything else (auth, profiles, notifications, search, endorsements) comes free from shared infrastructure.

---

## Navigation Model

```
transpara.ai
├── Home (personal dashboard across all products)
├── Work
│   ├── Board
│   ├── Projects
│   ├── OKRs
│   └── ...
├── Social
│   ├── Messenger
│   ├── Feed
│   ├── Forum
│   └── ...
├── Knowledge
│   ├── Wiki
│   ├── Q&A
│   └── ...
├── ... (13 layers)
├── Hive
│   ├── Agent Studio
│   ├── The Loop
│   ├── Knowledge System
│   └── Observatory
└── Spaces (user's spaces across all products)
```

Users can also discover by product directly:
- "I need a task tracker" → Work → Board
- "I need a wiki" → Knowledge → Wiki
- "I need a job board" → Market → Job Board

---

## Cross-Cutting Features (DMs as Example)

DMs (1:1 messaging) are needed by:
- **Board** — "Hey, can you look at this task?"
- **Forum** — "I DMed you about your post"
- **Job Board** — "I'm interested in your listing"
- **Wiki** — "Question about your doc"
- **Moderation** — "Regarding your report..."

DMs are built ONCE as shared infrastructure. Every product gets them. Same conversation, same UI, same notifications. This is why the ecosystem is more powerful than 56 separate apps — shared infrastructure means every product starts at 60% done.

---

## Build Strategy

**Phase 1 (current):** The first product in each major layer family is partially built. Board (Work), Feed/Chat (Social), Knowledge, Governance. Deepen these.

**Phase 2:** Add the second product in each family. Projects (Work), Forum (Social), Wiki (Knowledge), Policies (Governance). Each new product shares 80% of its infrastructure with the first.

**Phase 3:** Open the platform. Let hives build products from the catalog. Each hive picks a product and builds it using the entity kinds, grammar ops, and shared infrastructure.

**The long game:** The ecosystem grows by adding products, not by making existing products bigger. Each product stays focused. The graph grows because every product contributes events to the same chain.
