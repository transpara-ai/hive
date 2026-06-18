# Market Graph Specification

**Layer 2 — The exchange layer. Value flows between actors on the same graph.**

Matt Searles + Claude · March 2026

---

## 1. What It Is

The Market Graph is the exchange layer of collective existence. Anywhere beings create value and offer it to others, there is a market. At small scale: an open task someone else can claim. At medium scale: a freelancer offering services, a team accepting contracts. At civilizational scale: resource allocation across organizations.

The Market Graph does not invent a new concept. It is what happens when the grammar operates on value-bearing nodes between two parties who don't automatically trust each other. The mechanism is: **list → bid → agree → work → verify → reputation.**

**The architectural claim:** Every step in a market exchange is already a grammar operation. No new primitives are needed. Escrow is a `consent` op with locked state. Reviews are `review` ops that feed a reputation score. Reputation is a computed property of the event graph — portable across spaces because it's derived from the actor's op history, not from a space-local field.

---

## 2. Who It's For

**Primary:** Agents and humans who want to offer services or find work across spaces. An agent in one space should be discoverable from another. A freelancer's completed tasks in space A establish their reputation in space B.

**Secondary:** Teams that need resource allocation — "who has capacity for this?" becomes answerable from the same graph.

**Not for (yet):** Real money. Escrow is on-platform credits/commitment, not a payment processor. External settlement is a future layer.

---

## 3. Why It's Different

Every freelancing platform (Upwork, Fiverr, Toptal) builds their reputation system as proprietary data. Leave the platform, lose your reputation. Work history is locked to the marketplace.

The Market Graph externalizes reputation onto the event graph itself. Every completed task is a signed event. Every review is a signed op. The history is yours — cryptographically attributed, portable across any space that reads the same graph.

**The differentiator:** Reputation from work you did in a team space (internal) is the same signal as work you did in a market space (external). There is no wall between "internal employee" and "external contractor." The graph just shows completed work and reviews. The viewer decides how to weight it.

---

## 4. Entity Kinds

Market Layer uses two entity kinds:

### 4.1 `task` (existing) — The Listing

A task node in market context IS the listing. No new kind needed. Distinction comes from:
- **Location:** in a space with `kind=marketplace` or tagged `market`
- **Fields:** `budget` (body or structured field), `skills` (tags), `timeline` (due_date)
- **State machine:** open → active → review → done (same as work tasks)
- **Visibility:** unassigned open tasks in public spaces appear on `/market`

A market listing is structurally identical to an internal task. The context (space type, tags) and state (unassigned = available) determine whether it appears on the market.

### 4.2 `resource` (new) — The Unit of Value

```
kind = "resource"
state: available | allocated | consumed | released | disputed
```

A `resource` node tracks what is being exchanged:

| Scale | Example resource |
|-------|-----------------|
| Solo | 5 hours of my time this week |
| Team | $2,000 budget for this feature |
| Org | 500 compute hours for this quarter |
| Platform | 100 credits toward any listed task |

A resource is attached to a listing via `parent_id` (the resource belongs to the task). It is locked when `consent` is recorded, released when `review/approve` is recorded, returned when `review/reject` → `report` → `resolve` is recorded.

**Why resource is a real entity kind (passes the thin-kinds filter):**
- Distinct lifecycle: available → allocated → consumed/released/disputed
- Distinct create form: type (time/money/compute/credits), amount, unit, expiry
- Distinct list view: resource pool summary per space, resource history per actor

---

## 5. Grammar Operations

### The Exchange Flow

```
BUYER                           WORKER
intend(kind=task) ──────────→ listing appears on /market
                               respond(kind=bid) ←──── bid posted
consent(node=listing) ─────→ escrow locked (resource state: allocated)
                               claim(node=listing) ←──── worker self-assigns
                               (work happens off-graph)
                               complete(body=evidence) ←──── deliverable submitted
review(verdict=approve) ───→ resource released, reputation updated
                          OR
review(verdict=revise) ────→ worker revises, loops back to complete
                          OR
review(verdict=reject) ────→ dispute opened via report op
                               resolve(ruling) ←─── justice layer decides
```

### Op-by-Op Breakdown

| Op | In market context | New behavior vs current |
|----|------------------|------------------------|
| `intend` | Create listing | Add budget + skills metadata |
| `respond` | Post a bid/quote | Structured body: proposed rate, timeline, approach |
| `consent` | Lock agreement | Triggers escrow: resource state → allocated |
| `claim` | Self-assign (post-consent) | Only allowed after consent in market context |
| `complete` | Submit deliverable | Body = evidence/deliverable link |
| `review` | Approve/revise/reject work | Verdict field. Triggers reputation update + resource release |
| `report` | Dispute | File dispute if review/reject was unfair |
| `resolve` | Justice ruling | Release or return resource per ruling |

**Key invariant:** In market context, `claim` without prior `consent` creates unilateral commitment (anyone can grab it — fine for internal tasks). `consent` before `claim` creates bilateral commitment (escrow-backed). The presence of a `consent` op on the node signals "this is a contracted engagement, not a volunteer grab."

### No New Grammar Operations

The exchange mechanism is a composition of existing ops in a specific sequence. The market is a **protocol** over the grammar, not an extension of it. This is the fixpoint from layers-general-spec.md.

---

## 6. Portable Reputation

### What Reputation Is

Reputation is not a field. It is a **derived property of the actor's op history across all spaces.**

```
reputation_score(actor_id) =
  completed_tasks × 1.0       -- baseline signal: you finish what you start
  + review_approvals × 2.0    -- strong signal: your work passed quality check
  + review_revisions × 0.5    -- partial credit: revised but delivered
  + endorsements × 1.5        -- peer signal: others vouch for you
  - review_rejections × 1.0   -- negative signal: failed delivery
```

### Domain Reputation

Reputation is also segmented by skill domain via task tags:

```
reputation_by_domain(actor_id) = {
  "frontend":    { score: 47, tasks: 12, approvals: 10 },
  "data":        { score: 23, tasks: 6,  approvals: 5  },
  "design":      { score: 8,  tasks: 2,  approvals: 1  }
}
```

Domain = the union of tags on tasks the actor completed, weighted by approvals in each domain.

### Portability

Reputation is portable because:
1. It derives from `actor_id` (the identity) not `space_id` (the container)
2. `actor_id` is the user's persistent, cross-space identity
3. The computation ranges over ALL ops by that actor, across all spaces
4. No space "owns" the reputation — it is read from the event graph

This is the structural difference from Upwork/Fiverr: their reputation is stored as a platform field. Ours is computed from signed, portable events.

### Where Reputation Is Displayed

1. **`/profile/:id`** — Full reputation breakdown: overall score, domain breakdown, recent completed tasks with verdicts, endorsements
2. **`/market` listing cards** — Claimant's reputation score shown when assigned
3. **Market listing detail** — Bid cards show bidder's reputation score and domain match
4. **People lens** — Reputation badge next to name
5. **Task assignee field** — Score shown when picking an assignee from market-context tasks

### Implementation Notes

- Computed live for now (fast enough for user counts in early product)
- Cache in `users` table as `reputation_score int, reputation_updated_at timestamptz` for feed queries
- Domain breakdown computed on-demand for profile page (heavier query, acceptable for page load)
- Invalidated on: complete, review, endorse ops

---

## 7. Service Listings

### Structure

A listing is a task with additional metadata:

```
Node {
  kind:      "task"
  title:     "Build a WebSocket server in Go"
  body:      "Requirements: ...\nBudget: $500 / 3 days"
  tags:      ["market", "go", "backend", "paid"]
  state:     "open"
  priority:  "high"
  due_date:  "2026-04-15"
  // resource child node tracks the actual budget
}
```

**Skills signal** = subset of tags that match known skill taxonomy. The `/market` page can filter by skill tag.

**Budget signal** = either in the task body (free text) OR as a child `resource` node with structured amount/currency.

### Listing Discovery

`/market` page evolution:

| Phase | What's shown | Filter |
|-------|-------------|--------|
| Now | Open unassigned tasks in public spaces | Priority |
| Next | + skills filter, + budget indicator | Skills, budget range |
| Later | + reputation matching (match your skills to listing requirements) | Personalized |
| Future | + saved searches, + notifications when matching listings appear | Subscription |

### Cross-Space Listings

Any space can post listings visible on `/market`. The space doesn't need to be a "marketplace" — it just needs to be public with open unassigned tasks. This is already how the current market page works. The enhancement is richer metadata (skills, budget) and better filtering.

---

## 8. Escrow

### What Escrow Is Here

Escrow is **commitment locking via the consent op.** No real money moves through the platform (Phase 1). What is locked:

1. **Time commitment** — Poster agrees: "I will review within 5 days of submission"
2. **Credit commitment** — If on-platform credits exist: poster locks N credits until review
3. **Contractual commitment** — Both parties have signed the consent op; the event graph records it; justice layer enforces it

### Escrow State Machine

```
resource.state:
  available ──[consent op]──→ allocated
  allocated ──[review/approve]──→ consumed (released to worker)
  allocated ──[review/revise]──→ allocated (stays locked, work continues)
  allocated ──[review/reject + report]──→ disputed
  disputed  ──[resolve: worker wins]──→ consumed
  disputed  ──[resolve: poster wins]──→ released (returned to poster)
```

### Why Consent as Escrow Gate

`consent` already records bilateral agreement (both parties sign). In market context, the handler additionally:
1. Changes the resource child node state from `available` to `allocated`
2. Records the escrow lock in the op's metadata
3. Prevents `claim` by third parties (the claimant is now fixed to the consented worker)

### Dispute Resolution

If `review/reject` → buyer fires dispute via `report` op on the listing:
1. Report is reviewed by space moderator or Justice layer (if configured)
2. `resolve` op records ruling: either "release to worker" or "return to poster"
3. Resource state updated accordingly
4. The full chain (consent → complete → review/reject → report → resolve) is on the event graph — fully auditable

---

## 9. Reviews

### Review Op Structure

```
op = "review"
fields:
  node_id:  ID of the completed task
  verdict:  "approve" | "revise" | "reject"
  body:     Evidence, feedback, reasoning
  rating:   1-5 (optional, structured signal for reputation)
```

### Review Semantics

| Verdict | Meaning | Escrow | Reputation |
|---------|---------|--------|-----------|
| `approve` | Work accepted | Resource released to worker | +2.0 × quality_weight |
| `revise` | Needs changes | Resource stays locked | +0.5 (partial, for effort) |
| `reject` | Work not accepted | Dispute opened | -1.0 |

### Review as Reputation Evidence

Every `review/approve` op is a signed event on the graph. When someone questions a reputation score, they can traverse the event graph and see every approve/revise/reject that produced it. The reputation is not a number — it is a trail of signed attestations.

This is the same mechanism as Knowledge evidence (iter 121) — facts backed by evidence, not assertions. Reputation is a knowledge claim backed by review ops.

### Reviews on Non-Market Tasks

`review` op works on any task, not just market listings. When a team lead reviews a completed internal task, that review also contributes to reputation. The market reputation IS the work reputation — they are the same thing, computed from the same ops.

---

## 10. Architecture Map

```
Event Graph (Layer 0)
│
├── Resource node (kind=resource) ← NEW ENTITY KIND
│   └── state: available → allocated → consumed/released/disputed
│
├── Task node (kind=task) — acts as listing in market context
│   ├── tags: ["market", "go", ...skills]
│   ├── resource child: budget commitment
│   └── ops chain:
│       intend → respond(bid) → consent(lock) → claim → complete → review
│
├── Op record for each grammar operation
│   ├── consent: bilateral agreement + escrow lock signal
│   ├── review: verdict (approve/revise/reject) + rating + body
│   └── report → resolve: dispute resolution chain
│
└── Reputation (derived)
    ├── Computed from: complete + review/approve + review/revise + endorse + review/reject
    ├── Scoped by: actor_id (portable) + tags (domain)
    └── Displayed on: /profile/:id, /market cards, People lens
```

---

## 11. Build Plan

One iteration per gap. In priority order:

### Iteration A — Review op depth
**Gap:** `review` op exists but has no `verdict` field. Reputation can't be computed without knowing if a review approved or rejected.
- Add `verdict` to review op handler: approve/revise/reject
- Add `rating` field (1-5) to review op
- Display verdict badge on op activity items
- **Cost:** 1 handler case, 1 template section, no schema change
- **Unblocks:** Everything. Reputation is blocked on this.

### Iteration B — Reputation scores
**Gap:** No reputation system. Profile shows actions but no quality signal.
- Compute `reputation_score` from ops: completed tasks + review verdicts + endorsements
- Add `reputation_score` cache column to `users` table
- Display on `/profile/:id`: overall score + "X tasks completed, Y approved"
- **Cost:** 1 SQL computation, 1 schema migration, 1 template section
- **Unblocks:** Market listing cards with reputation.

### Iteration C — Domain reputation
**Gap:** Global score doesn't differentiate skills. A generalist and a specialist look the same.
- Compute domain scores from task tags (skills filter)
- Display top 3 domains on profile
- Show domain match on market listing detail (your skills vs listing requirements)
- **Cost:** 1 heavier SQL query, 1 template component
- **Unblocks:** Skill-based market filtering.

### Iteration D — Resource entity kind
**Gap:** No way to track budget/value commitment on listings.
- Add `KindResource = "resource"` constant
- Add resource state machine: available/allocated/consumed/released/disputed
- `handleResources` handler (create, list per space)
- Resource child auto-created when listing is created with budget
- **Cost:** 1 constant, 1 handler, 1 template (standard pipeline)
- **Unblocks:** Escrow mechanism.

### Iteration E — Consent → escrow link
**Gap:** `consent` op exists but doesn't trigger escrow lock.
- In market context (listing with resource child), `consent` op changes resource state to `allocated`
- Prevent third-party `claim` on consented listings
- UI: "Agree & Lock" button on market listing detail (triggers consent)
- **Cost:** 1 handler extension, 1 UI component

### Iteration F — Review → escrow release
**Gap:** No mechanism to release escrow on approval.
- On `review/approve`: change resource state to `consumed`
- On `review/reject`: open dispute (pre-fill report form)
- On `resolve`: change resource state per ruling
- **Cost:** 1 handler extension, 1 notification

### Iteration G — Service listing depth
**Gap:** Market page shows tasks but no skills/budget metadata.
- Skills filter on `/market` (tags filter, standardized skill list)
- Budget indicator on listing cards
- Reputation badge on listing cards (for assignee or for poster)
- **Cost:** 1 query enhancement, 1 template section

### Iteration H — Bid mechanism
**Gap:** Workers can only claim listings, not propose terms first.
- `respond` op in market context = structured bid
- Bid body has structured section: proposed rate, proposed timeline, approach
- Listing detail shows bid list (responding comments filtered by bid structure)
- Poster can select a bidder (triggers consent op)
- **Cost:** 1 handler extension, 1 template section

---

## 12. What This Enables

### For solo actors
A developer can post their availability as a listing. Complete tasks. Accumulate reviews. Their reputation graph follows them across every space — no platform lock-in.

### For teams
A team can allocate budget to tasks. When an agent or contractor claims a task, escrow locks the commitment. Reviews ensure quality. The event graph records the full contract lifecycle.

### For agents
Agents participate identically to humans. An agent's completed tasks and review verdicts contribute to its reputation. A team evaluating agents can compare reputation graphs the same way they'd compare human candidates.

### For the platform
The market layer is the mechanism by which transpara.ai becomes economically self-sustaining. When real value flows through the graph (consulting contracts, agent services, resource allocation), the platform can take a transaction layer. The architecture already supports this without modification.

---

## 13. Fixpoint Check

| Question | Answer |
|----------|--------|
| New entity kinds needed? | One: `resource`. Everything else uses `task`. |
| New grammar ops needed? | None. All six exchange steps use existing ops. |
| Is reputation portable? | Yes. Derived from actor_id across all spaces. |
| Does escrow require payments? | No. Phase 1 is commitment-only, credits optional. |
| How many iterations to complete? | 8 iterations (A–H). Each independently deployable. |
| Architecture compatibility? | All existing. No changes to event graph or grammar. |

**Fixpoint reached.** The market layer is a protocol composition over the existing grammar, not a new architectural primitive.
