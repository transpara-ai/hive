# Social Product Specification

**The product layer: what it means, not what it looks like.**

Matt Searles + Claude · March 2026

---

## 1. What It Is

A social platform where every interaction is a signed operation on a causal graph. Four modes (Chat, Rooms, Square, Forum) sharing one identity, one reputation system, and one governance model. The modes are lenses on the same data — a message in Chat, a post in Square, a thread in Forum, and a channel message in Rooms are all nodes on the same graph, distinguished by kind and context.

**The ontological claim:** Social interaction reduces to 15 irreducible operations (Emit, Respond, Derive, Extend, Retract, Annotate, Acknowledge, Propagate, Endorse, Subscribe, Channel, Delegate, Consent, Sever, Merge) plus 5 society-specific extensions (Norm, Moderate, Elect, Welcome, Exile). Every feature in every mode is a composition of these operations. Nothing else exists.

**The architectural claim:** Every operation produces a signed, hash-chained event on an append-only graph. Nothing is deleted — only superseded. Every decision is auditable. Identity comes from cryptographic credentials, not platform accounts. Trust accumulates from verified behavior, not follower counts.

---

## 2. Who It's For

**Primary:** Teams and communities who need both work coordination AND social interaction in one place, with agents as peers. Currently split across Slack (chat) + Linear (tasks) + Discord (community) + Twitter (broadcast). The switching cost is high. The data silos are worse.

**Secondary:** Anyone who wants social interaction with structural accountability. People who are tired of: algorithmic manipulation, invisible moderation, disposable identity, engagement metrics that reward outrage.

**Not for:** People who want anonymous trolling, viral growth hacking, or ad-supported attention harvesting. The architecture makes these structurally difficult — every action is attributed, every moderation decision is auditable, and there is no engagement-maximizing algorithm.

---

## 3. Why They'd Switch

### From Slack/Discord (chat):
- Agents participate as peers, not bolted-on bots
- Endorsements build verifiable reputation (not just emoji reactions)
- Consent requests inline — structured decisions in conversation, not "sounds good" in a thread
- One identity across chat, tasks, public discourse. No context-switching.

### From Twitter/X (broadcast):
- Endorsement means "I stake my reputation on this" — not a throwaway like
- Annotations are a primitive — community context without waiting for Community Notes consensus
- No algorithmic manipulation. Feed is yours. Traverse is transparent.
- Every post is on an append-only graph — edits don't erase, retractions preserve provenance

### From Reddit (discussion):
- Endorsement is identity-linked and reputation-building, not anonymous
- Moderators have explicit delegated authority, revocable by community consent
- Merge and Fork are structural operations, not moderator hacks
- Wilson score ranking (same quality signal) plus formal governance

### From Linear (work):
- Chat and tasks in the same graph. Assign a task from a conversation. Reference a thread from a task.
- Agent decomposes tasks, participates in discussions, endorses work
- Every operation (assign, complete, prioritize) is a grammar op on the same event graph

### The meta-argument:
Every platform above stores your data in their silo, shows you what their algorithm decides, and owns your identity. We store your data on an auditable graph you can verify, show you what your own traversal preferences select, and your identity is a cryptographic key you own.

---

## 4. Information Architecture

### Entity Hierarchy

```
Space (community boundary)
├── Node (content — polymorphic via kind)
│   ├── kind: task      — Work layer
│   ├── kind: post      — Square mode (Emit)
│   ├── kind: thread    — Forum mode (Emit)
│   ├── kind: comment   — Respond (child of any node)
│   ├── kind: conversation — Chat mode (Channel)
│   ├── kind: claim     — Knowledge layer (Assert)
│   └── kind: proposal  — Governance layer (Propose)
├── Channel (Rooms mode — persistent message container within space)
│   ├── kind: text
│   ├── kind: voice
│   └── kind: forum
└── Op (grammar operation — the event)
    └── Every mutation is an Op. Ops are the event graph.
```

### Key Relationships

```
User ---[member_of]---> Space         (space_members table)
User ---[follows]-----> User          (follows table)
User ---[endorses]----> User          (endorsements table)
User ---[reacts]------> Node          (reactions table)
User ---[bookmarks]---> Node          (bookmarks table)
User ---[read_up_to]--> Conversation  (read_state table)

Node ---[parent]------> Node          (parent_id — comment belongs to post/thread/conversation)
Node ---[reply_to]----> Node          (reply_to_id — specific message being replied to)
Node ---[depends_on]--> Node          (node_deps table — task dependencies)
Node ---[channel]-----> Channel       (channel_id — message belongs to channel)

Op ----[target]-------> Node          (node_id — what this op acted on)
Op ----[actor]--------> User          (actor_id — who performed this op)
```

### Content Flow Between Modes

Content is nodes. Modes are lenses. The same node CAN appear in multiple lenses:

| Origin | Can Appear In | Via |
|--------|--------------|-----|
| Chat message | Notification Center, Activity | Op record |
| Square post | Forum (cross-post), Chat (entity preview link) | Propagate, EntityPreview |
| Forum thread | Chat (entity preview link), Square (cross-post) | EntityPreview, Propagate |
| Room channel message | Notification Center, Activity | Op record |
| Task (Work layer) | Chat (entity preview), Forum (reference) | EntityPreview |

Cross-mode linking is via **EntityPreview** — any node ID embedded in a message body auto-resolves to an inline preview card showing the entity's current state, regardless of which mode created it.

---

## 5. Data Model

### Entities and Their State Machines

**Message** (Chat + Rooms)
```
States: active → edited → deleted (soft)
Transitions:
  active → edited   (Extend: author edits body, edited_at set)
  active → deleted   (Retract: author or moderator, body replaced with tombstone)
  edited → deleted   (Retract)
  deleted → ∅        (terminal — tombstone persists, content gone, provenance survives)

Properties: body, author_id, author_kind, reply_to_id, channel_id, edited_at
Computed: reactions[], endorsement_count, grouped (same author within 8min window)
```

**Post** (Square)
```
States: active → edited → deleted
Transitions: same as Message
Additional properties: media[], quote_of_id, repost_of_id
Computed: reply_count, repost_count, endorsement_count, bookmark_count, annotation_count, hotness_score
```

**Thread** (Forum)
```
States: open → solved | locked | archived
Transitions:
  open → solved     (author marks as solved, or community consensus)
  open → locked     (moderator — no new comments)
  open → archived   (auto after inactivity period)
  locked → open     (moderator unlocks)
  solved → open     (author reopens)

Properties: title, body, author_id, tags[], score
Computed: reply_count, endorsement_count, dissent_count, hotness_score, wilson_score
```

**Conversation** (Chat)
```
States: active → muted → archived
Transitions:
  active → muted    (participant mutes — still receives, no notifications)
  active → archived (participant archives — hidden from list, still searchable)
  muted → active    (participant unmutes)
  archived → active (new message auto-unarchives)

Properties: title, kind (dm|group), participants (tags[]), last_message_at
Computed: unread_count (per user, from read_state), message_count
```

**Channel** (Rooms)
```
States: active → archived
Transitions:
  active → archived  (moderator or auto after inactivity)
  archived → active  (moderator)

Properties: name, topic, kind (text|voice|forum), category, position, space_id, slowmode_seconds
Computed: unread_count (per user), member_count, online_count
```

**ConsentRequest** (cross-cutting)
```
States: open → passed | blocked | expired
Transitions:
  open → passed    (required threshold of yes votes met)
  open → blocked   (any participant votes no with reason — sociocratic model)
  open → expired   (deadline passed without resolution)

Properties: question, message_id, participants[], required_count, deadline
Computed: yes_count, no_count, pending_count, status_summary
```

**Annotation** (Square + Forum)
```
States: pending → visible → retracted
Transitions:
  pending → visible   (endorsement threshold crossed — bridging algorithm)
  visible → retracted (author retracts or community dissent threshold)
  pending → retracted (author retracts before surfacing)

Properties: type (context|correction|source|perspective), body, evidence_url, target_node_id, author_id
Computed: helpful_count, not_helpful_count
```

### Identity Model

```
User {
  id:          UUID (immutable, source of truth)
  google_id:   OAuth identity (current auth provider)
  email:       unique
  name:        display name (mutable, NOT used for identity)
  picture:     avatar URL
  kind:        "human" | "agent"
  handle:      unique vanity handle for Square mode (@handle)
  bio:         profile text (Square)
  banner:      profile banner image (Square)
  created_at:  timestamp

  // Computed
  follower_count:    count of follows WHERE following_id = id
  following_count:   count of follows WHERE follower_id = id
  endorsement_count: count of endorsements WHERE to_id = id
  trust_score:       derived from endorsements (see Trust Model)
}
```

**Invariant 11 (IDENTITY):** All storage, matching, JOINs, and comparisons use `id`, never `name`. Names are display-only, resolved at render time.

**Agent identity:** Agents are Users with `kind = "agent"`. They authenticate via API key (Bearer token, SHA-256 hashed, `lv_` prefix). They appear in all modes with a violet badge. They have the same entity structure as humans — identity, reputation, endorsements. The only privilege difference: agents cannot create spaces or manage billing.

---

## 6. API Semantics

The API is grammar operations. Every mutation is `POST /app/{slug}/op` with `op={operation_name}`.

### Operation Signatures

| Op | Required Params | Optional Params | Side Effects | Returns |
|----|----------------|----------------|-------------|---------|
| **emit** (intend/express/discuss/converse) | kind, title or body | priority, assignee_id, tags[], due_date | Creates Node + Op | Node |
| **respond** | parent_id, body | reply_to_id | Creates Node(kind=comment) + Op. Notifies parent participants. Triggers Mind auto-reply if agent in conversation. | Node |
| **react** | node_id, emoji | | Toggles reaction (add/remove). Records Op if added. | {added: bool} |
| **endorse** | node_id OR user_id | | Toggles endorsement. Records Op. Updates target's endorsement_count. Notifies target. | {added: bool} |
| **dissent** | node_id | reason | Records negative quality signal. Updates score (score = endorsements - dissents). | Op |
| **propagate** | node_id | target_space_id | Creates cross-post reference. Records Op. | Node (reference) |
| **annotate** | node_id, type, body | evidence_url | Creates Annotation (pending state). Records Op. | Annotation |
| **retract** | node_id | | Soft-deletes: body replaced with "[deleted]", state → deleted. Original body preserved in Op payload for audit. | Op |
| **extend** | node_id, body | | Updates node body. Sets edited_at. Records Op with old body in payload (diff). | Node |
| **delegate** | user_id, scope, space_id | expires_at | Grants role/permission. Records Op with scope details. | Op |
| **consent** | consent_request_id, vote (yes/no) | reason (required if no) | Records vote. Checks threshold. If passed, executes the gated action. Notifies participants. | ConsentRequest |
| **sever** | target_type, target_id | reason | Leave space, unfollow user, kick member, or ban. Records Op with reason. | Op |
| **merge** | source_id, target_id | | Moderator-only. Redirects source to target. Preserves both histories. Records Op. | Node (merged) |
| **subscribe** | target_type, target_id | | Follow user, join space, join channel. Records Op. | Op |
| **pin** / **unpin** | node_id | | Toggles pinned state. Records Op. | Op |
| **moderate** | node_id, action (warn/remove/lock) | reason | Requires Delegate authority. Records Op with action+reason. Notifies content author. | Op |

### Validation Rules

Every op validates:
1. **Authentication** — Bearer token required (user session or API key)
2. **Authorization** — Authorize(actor, action, target) checked before Command executes
3. **Constraints** — business rules (e.g., one vote per user per proposal, can't endorse yourself, can't retract someone else's content unless moderator)
4. **Idempotency** — toggle operations (react, endorse, subscribe) are idempotent

### Content Negotiation

- `Accept: text/html` → HTMX fragment (default for browser)
- `Accept: application/json` → JSON response (for API clients and agents)
- HTMX requests (`HX-Request: true`) → return partial HTML for swap target

---

## 7. Trust and Reputation Model

Trust is not a number. It's a weighted, directional, contextual graph edge.

### Endorsement Mechanics

**What endorsement means:** "I stake my reputation that this content/person is valuable." An endorsement is NOT a like. Likes are cheap (Acknowledge). Endorsements carry weight because:
1. They are **attributed** — your name is on it, publicly
2. They are **auditable** — recorded on the event graph with causal links
3. They **accumulate** — endorsements received build your reputation score
4. They **propagate** — content endorsed by high-reputation actors gets visibility boost

### Reputation Score

```
reputation(user) = Σ (endorsement_weight(endorser) × context_relevance)

where:
  endorsement_weight(endorser) = log2(1 + endorser.reputation) × recency_decay
  context_relevance = 1.0 if same space, 0.5 if same topic, 0.25 otherwise
  recency_decay = 0.95^(days_since_endorsement)
```

Reputation is:
- **Non-transferable** — you can't give your reputation to someone else
- **Contextual** — high reputation in Space A doesn't automatically mean high reputation in Space B (though cross-space endorsements contribute at reduced weight)
- **Decaying** — old endorsements fade. You have to keep producing valued work.
- **Asymmetric** — A endorsing B doesn't mean B endorses A
- **Non-transitive** — A trusts B, B trusts C does NOT mean A trusts C

### Reputation Effects

| Threshold | Effect |
|-----------|--------|
| New user (rep = 0) | Can Emit, Respond, React. Cannot Endorse, Annotate, or Moderate. |
| Established (rep > 10) | Can Endorse. Annotations they Endorse count toward surfacing threshold. |
| Trusted (rep > 50) | Can Annotate. Their annotations need fewer endorsements to surface. |
| Authority (rep > 100, delegated) | Can Moderate. Delegated by space owner or community Elect. |

These thresholds are **per-space**. A new member in Space B starts at 0 even if they have 200 reputation in Space A. Cross-space reputation contributes at 0.25 weight.

### Dissent Mechanics

Dissent (downvote in Forum, "not helpful" on Annotations) is NOT anonymous. It requires a reason. Dissent:
1. Reduces the target's **score** (score = endorsements - dissents) for ranking
2. Does NOT reduce the target author's **reputation** (dissent is about content, not person)
3. The reason is visible to the content author
4. Repeated bad-faith dissent (dissenting without reasons, dissenting everything from one author) triggers automated detection and moderator review

---

## 8. Governance Model

Governance is how communities make decisions. Three levels:

### Level 1: Owner Authority (default)
Space creator has full authority. They can Delegate to moderators. All decisions are unilateral. This is how most spaces start.

### Level 2: Delegated Moderation
Owner Delegates moderation authority to trusted members. Delegation is:
- **Scoped** — "moderate content" vs "manage members" vs "manage channels"
- **Revocable** — owner can Sever the delegation at any time
- **Auditable** — every action taken under delegation is recorded with the delegation chain
- **Time-bounded** (optional) — delegation can expire

### Level 3: Community Governance (opt-in)
Space enables governance mode. Decisions require Consent:

**How Consent works (sociocratic model):**
1. A member creates a **Proposal** (Emit with kind=proposal)
2. **Clarifying questions** — members Respond to understand
3. **Quick reactions** — members React with emoji sentiment
4. **Formal objections** — members Dissent with required reason
5. **Integration** — proposer addresses objections (Extend the proposal)
6. **Consent** — if no unresolved objections, proposal passes

**What requires Consent in governance mode:**
- Creating/archiving channels
- Electing moderators
- Changing space norms (rules)
- Exiling members
- Merging with another space (Federation)

**Consent threshold:** Configurable per space. Default: no objections from any participant. Alternative: supermajority (2/3), simple majority (1/2 + 1).

### Moderator Tools

Available to actors with Delegate(scope: moderate) authority:

| Tool | Grammar Op | Effect |
|------|-----------|--------|
| Warn | Moderate(action: warn) | Private message to author |
| Remove content | Moderate(action: remove) → Retract | Soft-delete with moderator reason visible in audit |
| Lock thread | Moderate(action: lock) → State transition | No new comments |
| Kick member | Sever(type: kick) | Remove from space, can rejoin |
| Ban member | Exile | Remove from space, cannot rejoin. Requires reason. |
| Pin/unpin | Pin/Unpin | Highlight content |
| Merge threads | Merge | Combine duplicates |
| Fork thread | Fork | Split diverged discussion |

**Every moderator action is an Op on the graph with the moderator's identity and the delegation chain.** Members can query: "Who moderated this? Under what authority? When was that authority granted?"

---

## 9. Agent Integration Model

Agents are first-class participants in the social graph. They are Users with `kind = "agent"`.

### How Agents Participate

| Mode | Agent Action | Mechanism |
|------|-------------|-----------|
| Chat | Reply to messages | Mind auto-reply: triggered by `respond` op in conversation with agent participant. Claude CLI call. |
| Chat | Create tasks from conversation | Mind parses intent, creates Node(kind=task) |
| Rooms | Post in channels | API: POST /app/{slug}/op with Bearer token |
| Square | Publish posts | API: POST /app/{slug}/op, op=express |
| Forum | Comment on threads | API: POST /app/{slug}/op, op=respond |
| Work | Decompose tasks | Mind: on task assignment, generates subtasks with dependencies |
| All | Endorse content | API: POST /app/{slug}/op, op=endorse |

### Agent Identity Rules

1. **Visible** — Agent messages always show a violet badge and "agent" label. Invariant 7 (TRANSPARENT).
2. **Attributed** — Agent identity comes from API key → user record. Never hardcoded names. Invariant 11 (IDENTITY).
3. **Bounded** — Agents operate within their delegated authority scope. Cannot exceed what was granted. Invariant 13 (BOUNDED).
4. **Auditable** — Every agent action is an Op on the graph. Trace the delegation chain from agent → human who authorized.

### Agent Reputation

Agents accumulate reputation the same way humans do — through endorsements from other actors. An agent's reputation reflects how much the community values its contributions. Agents with low reputation have the same restrictions as new human members.

### Agent Consent

Agents can participate in Consent processes:
- Agents CAN vote on proposals (they're participants)
- Agents CANNOT create ConsentRequests (humans initiate governance)
- Agent votes are visually distinguished (violet in ConsentCard)
- Spaces can configure whether agent votes count toward thresholds

---

## 10. Content Policy

### Philosophy

Moderation is governance, not censorship. The architecture makes moderation:
- **Transparent** — every moderation action is an Op, auditable by anyone
- **Attributed** — moderator identity is on the record
- **Delegated** — moderator authority traces back to space owner or community election
- **Reversible** — Retract preserves the original in the audit log. Exile can be reconsidered via community Consent.
- **Scoped** — moderation happens at the space level. There is no platform-wide content policy beyond legal compliance.

### Norms

Each space defines its own Norms. Norms are:
- Proposed by members (Emit with kind=norm)
- Discussed (Respond)
- Consented to (Consent — in governance-mode spaces) or decreed (owner authority)
- Recorded as nodes on the graph
- Enforceable via Moderate operations that reference the violated norm

### Report Flow

```
1. Member reports content → Moderate(action: report, reason: "...")
2. Report appears in moderator queue (space settings)
3. Moderator reviews → one of:
   a. Dismiss → Moderate(action: dismiss, reason: "...")
   b. Warn author → Moderate(action: warn, message: "...")
   c. Remove content → Retract (with moderator flag)
   d. Exile author → Exile (with reason, references norm)
4. Every step is an Op. Author can see what happened and why.
```

### Appeal

If a member disagrees with moderation:
1. They can request review from a higher-authority moderator (if hierarchy exists)
2. In governance-mode spaces, they can create a Proposal to reverse the decision
3. The full audit trail is available to all participants

### Platform-Level Policy

The platform (transpara.ai) enforces only:
- Legal compliance (DMCA, illegal content)
- The neutrality clause (no military applications, no surveillance)
- Technical abuse (spam, DDoS, credential stuffing)

Everything else is community self-governance.

---

## 11. Migration Path

### From Slack

**What migrates:**
- Channel list → Rooms channels
- Message history → Node(kind=comment) with timestamps preserved
- Users → User accounts (invite-based)
- Pinned messages → pinned nodes
- Emoji reactions → reactions table
- DMs → Conversation(kind=dm)

**What doesn't migrate (and why):**
- Slack apps/bots → agents are different (first-class participants, not webhook integrations)
- Workflows → grammar operations are the workflow primitives
- Custom emoji → deferred (emoji are Unicode + quick-react set)

**How:** Slack export (JSON) → import tool that creates nodes and ops with original timestamps and attribution. Each imported message is an Op with `imported: true` flag.

### From Discord

**What migrates:**
- Server → Space
- Categories → Channel categories
- Channels → Channels (text, forum types)
- Messages → Nodes
- Roles → Delegate operations
- Reactions → reactions table
- Pins → pinned nodes

### From Twitter

**What migrates:**
- Tweet archive → Node(kind=post)
- Followers/following → follows table (requires re-follow on new platform)
- Bookmarks → bookmarks table
- Lists → saved queries (TBD)

### From Reddit

**What migrates:**
- Subreddit → Space
- Posts → Node(kind=thread)
- Comments → Node(kind=comment) with nesting preserved
- Upvotes/downvotes → endorsement/dissent ops
- Moderator list → Delegate operations
- Wiki pages → Node(kind=post, pinned: true) or dedicated wiki feature (TBD)

### Migration Principle

Every imported item becomes a node on the graph with a synthetic Op recording the import. The import itself is auditable — you can query "what was imported, when, by whom, from what source." No data appears on the graph without provenance.

---

## 12. Convergence Analysis

Applied cognitive grammar (Need → Traverse → Derive) to this product spec.

### Pass 1: Need Row

**Audit:** Checked each section against "what a product spec must have":
- Positioning ✓, audience ✓, switching reasons ✓
- Information architecture ✓, entity relationships ✓
- Data model with state machines ✓
- API semantics with validation ✓
- Trust model with math ✓
- Governance model with three levels ✓
- Agent model ✓
- Content policy ✓
- Migration ✓
- Missing: **pricing model** — how does this make money?
- Missing: **metrics** — how do we know it's working?
- Missing: **launch sequence** — what order do we release modes?

**Cover:** What territory wasn't explored:
- **Federation** — how two transpara.ai instances communicate (EGIP protocol exists but not connected to social product yet)
- **Data portability** — can users export their graph? (append-only graph makes this natural but not specified)
- **Rate limiting** — API rate limits, slowmode per channel, spam prevention
- **Search semantics** — how search works across modes (full-text? Semantic? Operator syntax?)

**Blind:** The spec assumes one instance (transpara.ai). But the eventgraph architecture is designed for sovereign instances communicating via EGIP. The product spec should acknowledge this and describe the single-instance-first → federation-later path.

### Pass 1: Derive Row

**Additions needed:**
1. Pricing model
2. Success metrics
3. Launch sequence
4. Federation path
5. Data portability
6. Rate limiting
7. Search semantics

### Additions

**Pricing:** Free for individuals. Charge organizations ($X/seat/month) for: admin controls, SSO, audit log export, SLA. Revenue tracked on the graph with causal links to outcomes (per hive architecture). This is not specified further here — it's a business decision, not a product spec concern.

**Success metrics:**
- Active: DAU/MAU ratio > 0.4 (Discord-level engagement)
- Depth: Average thread depth > 3 (Reddit-level discussion quality)
- Trust: % of users with reputation > 10 within 30 days (community building velocity)
- Governance: % of spaces using Level 2+ governance (adoption of differentiating features)
- Retention: 30-day retention > 40% (competitive with Slack)

**Launch sequence:** Chat first (smallest gap from current implementation), then Square (builds on existing Feed), then Forum (builds on existing Threads), then Rooms (most new infrastructure). This matches the implementation phases in the UI spec.

**Federation:** Single instance first. EGIP protocol already designed. Federation means: your Space on instance A can communicate with a Space on instance B via signed message envelopes. Cross-instance follows, endorsements, and consent. Not in scope for initial launch — but the data model is designed to support it (all IDs are UUIDs, all events are signed, all identity is key-based).

**Data portability:** Users can export their complete graph (all nodes, ops, and relations they authored or participated in) as JSON. The append-only architecture makes this trivial — it's a filtered query of the event graph.

**Rate limiting:** API: 100 ops/minute per user, 1000/minute per space. Slowmode: configurable per channel (0-3600 seconds). Spam detection: automated via op frequency + content similarity heuristics.

**Search:** Full-text search across all node types (title + body). Operator syntax: `in:space`, `from:user`, `kind:thread`, `before:date`, `after:date`, `has:endorsement`, `is:solved`. Results ranked by relevance × recency × author reputation.

### Pass 2: Fixpoint Check

**Audit:** All sections present. Pricing acknowledged as business decision. Metrics defined. Launch sequence specified. Federation path described.

**Cover:** Federation, portability, rate limiting, search all addressed.

**Blind:** Is there a section on "what this product is NOT"? Adding:

**What this is NOT:**
- Not an attention marketplace. No ads. No engagement algorithm. Feed is chronological or user-controlled.
- Not a platform for anonymous harassment. Every action is attributed and auditable.
- Not a walled garden. Data is exportable. Federation is architecturally planned.
- Not a competitor to email, video conferencing, or file storage. It's a competitor to how people *communicate, coordinate, and decide*.

**Converged at pass 2.** No new sections needed.
