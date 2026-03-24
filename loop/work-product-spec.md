# Work Product Specification

**The product layer for task management where AI agents and humans operate on the same graph.**

Matt Searles + Claude · March 2026

---

## 1. What It Is

A task management system where every operation — create, assign, decompose, complete, review — is a signed event on a causal graph. Agents and humans are peers. Authority is delegated, scoped, and auditable. Trust accumulates from completed work, not from role labels.

**The ontological claim:** Work reduces to 12 operations (Intend, Decompose, Assign, Claim, Prioritize, Block, Unblock, Progress, Complete, Handoff, Scope, Review) derived from 4 dimensions (granularity, direction, actor, binding). Every feature is a composition. Nothing else exists.

**The architectural claim:** Every task mutation is a signed, hash-chained event. You can trace any task from intent through decomposition through assignment through completion through review. The chain is the accountability. No ticket tracker in existence offers this.

---

## 2. Who It's For

**Primary:** Teams where humans and AI agents collaborate on work. Engineering teams that use Linear/Jira for tracking but coordinate in Slack and make decisions in meetings. The data is split across 3+ tools. Decisions are unrecorded.

**Secondary:** Any team that needs auditable work management — regulated industries, open-source projects with contributor accountability, research teams with reproducibility requirements.

**Not for:** Solo developers who just need a todo list. The overhead of signed events and delegation chains is meaningless for one person.

---

## 3. Why They'd Switch

### From Linear:

Linear is keyboard-fast and beautifully dense. We don't try to out-design Linear. We offer what Linear structurally cannot:

- **Agent peers, not integrations.** Linear has GitHub/Slack integrations that *notify* about work. Our agents *do* work — decompose tasks, write code, review PRs, complete subtasks. An agent appears in the assignee dropdown the same way a human does.
- **Delegation with scope.** Linear has assignment (one owner per issue). We have Assign + Scope — "you own this task AND your authority is: read code, comment, approve/reject, but NOT merge." The scope is a data structure, not a Slack message.
- **Review as operation.** Linear has no review workflow for completed work (PR reviews are alpha). We have Review as a first-class grammar op with an endorsement trail.
- **Consent for decisions.** Linear has no mechanism for "this architecture decision requires sign-off from 3 leads." We have Consent — a bilateral, atomic, auditable agreement.
- **Causal history.** Linear has an activity log (what happened). We have a causal graph (what happened AND why, because every event declares its causes).

### From Jira:

- Everything Linear offers plus: no configuration maze, no 200-field forms, no workflow-as-bureaucracy
- Jira forces you to describe your process *to the tool*. We derive the process *from the grammar* — 12 operations, 3 modifiers, 6 named functions. The grammar IS the workflow engine.

### From Asana:

- Agent integration (Asana has none meaningful)
- Delegation with scope (Asana has single assignee, no authority model)
- Event-sourced history (Asana has changelog, not causal graph)
- Multiple views from one data model (Asana has this — we match it)

### The meta-argument:

Every task tracker stores *what* you decided to do. None stores *why* you decided, *who authorized it*, *what alternatives were considered*, or *how the decision connected causally to everything before and after*. We do. The event graph is the accountability layer that task trackers pretend they have.

---

## 4. Information Architecture

### Entity Hierarchy

```
Space (project/team boundary)
├── Node(kind=task)
│   ├── Properties: title, body, state, priority, assignee_id, due_date
│   ├── Children: Node(kind=task) — subtasks via parent_id
│   ├── Dependencies: node_deps — blocks/blocked-by
│   └── Comments: Node(kind=comment) — discussion on the task
├── Op (grammar operation — the event)
│   └── Every mutation (intend, assign, complete, etc.) is an Op
└── Views (lenses on the same data)
    ├── Board — tasks grouped by state columns (kanban)
    ├── List — tasks sorted/filtered/grouped (table)
    ├── Timeline — tasks on a time axis (Gantt-like, future)
    └── Dashboard — aggregate metrics (My Work)
```

### Key Relationships

```
Task ---[parent]-------> Task          (parent_id — subtask)
Task ---[depends_on]---> Task          (node_deps — blocking)
Task ---[assigned_to]--> User          (assignee_id)
Task ---[authored_by]--> User          (author_id)
Task ---[space]--------> Space         (space_id)
Op   ---[target]-------> Task          (node_id)
Op   ---[actor]--------> User          (actor_id)
Op   ---[causes]-------> Op            (payload — causal link, future: explicit column)
```

---

## 5. Data Model

### Task State Machine

```
States: open → active → review → done | canceled

Transitions:
  open → active        (Claim or Assign — someone starts working)
  open → canceled      (Retract — no longer needed)
  active → review      (Progress — work complete, needs review)
  active → open        (Unblock — reassign, deprioritize)
  active → canceled    (Retract)
  review → done        (Complete — reviewed and accepted)
  review → active      (Review rejects — needs more work)
  done → open          (Reopen — issue recurred)
  canceled → open      (Reopen)

Invariant: done requires evidence (the Complete op must have a non-empty body or linked Op chain).
```

### Priority Model

```
Priority: urgent | high | medium | low | none

Semantics:
  urgent — SLA: response within 4 hours. Surfaces with red indicator everywhere.
  high   — SLA: response within 24 hours. Bold in all views.
  medium — Default. Normal rendering.
  low    — De-emphasized. Hidden from active views unless filtered.
  none   — Backlog. Not prioritized yet.
```

### Decomposition Model

Tasks form a tree via parent_id. Decomposition rules:
- A parent task auto-completes when all children are done (configurable)
- A parent task cannot be completed if any child is blocked
- Subtask priority defaults to parent priority but can be overridden
- Subtask assignee defaults to empty (unclaimed)
- Max depth: 5 levels (Bounded — invariant 13)

### Dependency Model

Dependencies are directed edges in node_deps: `(task_A, depends_on: task_B)` means A is blocked by B.
- A blocked task shows a red blocker indicator in all views
- Completing task_B auto-unblocks task_A
- Circular dependencies are rejected at creation (Constraint)
- Dependencies can cross space boundaries (future: with Consent from both space owners)

### Assignment Model

Single assignee per task (matches Linear's clarity-of-ownership model). But enhanced with:
- **Scope** — what the assignee can do autonomously vs what requires approval
- **Handoff** — transfer with context (the Handoff op records: from, to, reason, context)
- **Delegation chain** — trace from current assignee back through all Handoff/Assign ops to the original Intend

### Review Model

Review is a first-class operation, not a comment. When a task reaches `review` state:
1. Reviewer assigned (explicit or default to task creator)
2. Review produces one of: Approve (→ done), Revise (→ active with feedback), Reject (→ canceled with reason)
3. Review is an Op with structured payload: { verdict, feedback, evidence }
4. Approved tasks can be Endorsed by anyone — "I vouch for the quality of this work"

---

## 6. API Semantics

All Work operations are `POST /app/{slug}/op` with the same content-negotiation as Social.

| Op | Required | Optional | Side Effects | Result State |
|----|----------|----------|-------------|-------------|
| **intend** | title | body, priority, assignee_id, due_date, parent_id | Creates task. Notifies assignee if set. | open |
| **decompose** | parent_id, subtasks[] | | Creates child tasks. Links parent. | parent unchanged, children open |
| **assign** | node_id, assignee_id | scope | Sets assignee_id. Records Op. Notifies assignee. Triggers Mind if agent. | unchanged |
| **claim** | node_id | | Sets assignee_id = actor. Records Op. | open → active |
| **prioritize** | node_id, priority | | Updates priority. Records Op. | unchanged |
| **block** | node_id | reason, blocker_id | Creates dependency if blocker_id. Records Op with reason. | active → active (blocker_count incremented) |
| **unblock** | node_id | resolution | Removes dependency. Records Op. | blocker_count decremented |
| **progress** | node_id, body | | Records progress comment. | active (or open → active) |
| **complete** | node_id | body (evidence) | Sets state=done. Records Op. Notifies author. Auto-completes parent if all siblings done. | → done |
| **review** | node_id, verdict, body | | Structured review. Approve → done. Revise → active. Reject → canceled. | → done or active or canceled |
| **handoff** | node_id, to_user_id | reason, context | Transfers assignment. Records full context. | assignee changes |
| **scope** | node_id, capabilities[] | expires_at | Defines what assignee can do. Records delegation. | unchanged (metadata added) |

### Triage (Named Function)

```
POST /app/{slug}/op
{
  "op": "triage",
  "actions": [
    { "node_id": "task_1", "action": "accept", "priority": "high", "assignee_id": "user_5" },
    { "node_id": "task_2", "action": "decline", "reason": "duplicate of task_3" },
    { "node_id": "task_3", "action": "merge", "into": "task_1" }
  ]
}
```

Batch operation. Each action is an individual Op. Triage is the inbox-zero pattern: process incoming work with accept/decline/merge/snooze.

---

## 7. Trust and Reputation in Work Context

Work reputation is earned through completed and reviewed work.

### How It Accumulates

```
work_reputation(user, space) = Σ (
  completed_tasks × complexity_weight ×
  review_quality × endorsement_bonus ×
  recency_decay
)

where:
  complexity_weight = subtask_count > 0 ? log2(subtask_count) : 1
  review_quality = approved_first_time ? 1.5 : (revised_once ? 1.0 : 0.7)
  endorsement_bonus = 1.0 + (endorsement_count × 0.1)
  recency_decay = 0.95^(weeks_since_completion)
```

### Reputation Effects

| Threshold | Effect |
|-----------|--------|
| New (rep = 0) | Can Intend, Claim low-priority tasks. Cannot Review. |
| Established (rep > 10) | Can Claim any task. Can Review others' work. |
| Trusted (rep > 30) | Can Assign to others within space. Can set Scope. |
| Authority (rep > 50, delegated) | Can Triage. Can Handoff. Can Block/Unblock. |

### Agent Work Reputation

Agents build work reputation the same way:
- Agent completes task → reputation increases
- Agent's work is reviewed and approved → reputation increases more
- Agent's work is reviewed and rejected → reputation penalty
- Agents with low reputation get simpler tasks and tighter scope

This creates a **natural trust escalation path**: new agent gets low-priority leaf tasks → proves quality → earns harder tasks → eventually handles complex decomposition autonomously.

---

## 8. Agent Integration for Work

### Task Lifecycle with Agents

```
Human: Intend("Build user authentication")
  ↓
Agent (Planner): Decompose([
  "Design auth schema",
  "Implement OAuth flow",
  "Add session management",
  "Write auth tests"
])
  ↓
Agent (Implementer): Claim("Implement OAuth flow")
  → Scope: [read code, write code, run tests, NOT merge, NOT deploy]
  → Progress("OAuth provider configured")
  → Progress("Token exchange working")
  → Complete(evidence: "tests passing, PR #42 ready for review")
  ↓
Human: Review(verdict: approve, body: "Clean implementation, good test coverage")
  ↓
Agent (or Human): Endorse(task) — "I vouch for this work"
```

### Agent Decomposition Rules

When Mind decomposes a task:
1. Each subtask gets explicit title, description, and estimated complexity
2. Dependencies are declared (not inferred)
3. Subtasks inherit parent space but NOT parent assignee
4. Leaf subtasks (no children) are claimable by agents
5. Non-leaf subtasks require human decomposition or agent with sufficient reputation

### Agent Authority Scope

Every agent assignment includes a Scope:

```
Scope(agent_7, task_42, capabilities: [
  "read_code",      // can read project files
  "write_code",     // can modify files
  "run_tests",      // can execute test suite
  "comment",        // can comment on tasks
  "create_subtask", // can decompose further
  // NOT: "merge", "deploy", "assign_others", "change_priority"
])
```

The Scope is an Op on the graph. If an agent attempts an action outside its scope, the action is rejected and an Escalate event is emitted.

---

## 9. Governance for Work Decisions

### Level 1: Owner Authority (default)
Space owner assigns, prioritizes, and reviews. Standard project management.

### Level 2: Delegated Authority
Owner delegates via Scope ops:
- **Project lead** — can Assign, Prioritize, Review within their project
- **Tech lead** — can Review and Approve/Reject code tasks
- **Triage lead** — can run Triage (accept/decline/merge incoming work)

Each delegation is scoped, time-bounded (optional), and revocable.

### Level 3: Team Governance (opt-in)
Decisions that affect the whole team require Consent:
- Sprint planning (what goes into the next cycle) — Consent from team leads
- Architecture decisions — Consent from tech leads
- Scope changes (adding features mid-sprint) — Consent from project lead + affected assignees
- Retrospective actions — Consent from full team

### Review Governance

Who can review whose work:
- Default: task creator reviews
- Delegated: anyone with Review capability in their Scope
- Cross-review: peer review (both parties have Established+ reputation)
- Agent review: agents can review if they have Review in Scope AND sufficient reputation

---

## 10. Views (Lenses on Work)

### Board (Kanban)
Tasks grouped by state columns: Open | Active | Review | Done.
- Drag-and-drop between columns (state transition via Op)
- Cards show: priority icon, title, assignee avatar, due date, blocker indicator, subtask progress
- Column counts. WIP limits (configurable).
- Filter by: assignee, priority, tags, due date, has blockers

### Dashboard (My Work)
Cross-space view of the current user's work:
- **Assigned to me** — grouped by priority, with overdue highlighting
- **Created by me** — tasks I authored, grouped by state
- **Reviewing** — tasks in review state where I'm reviewer
- **Watching** — tasks I'm subscribed to
- **Agent activity** — tasks where agents I manage are working

### Triage (Inbox)
Incoming work that needs a decision:
- New tasks from integrations, support, or other spaces
- Each item: Accept (→ backlog with priority), Decline (→ canceled with reason), Merge (→ duplicate of), Snooze (→ hide until date)
- Keyboard-first: 1=accept, 2=decline, 3=merge, H=snooze
- Badge count in sidebar

### Timeline (future)
Gantt-like view:
- Tasks as horizontal bars (start → due date)
- Dependency lines between tasks
- Critical path highlighting
- Drag to reschedule (updates due_date via Op)

---

## 11. Migration Path

### From Linear

**What migrates:**
- Teams → Spaces
- Issues → Nodes(kind=task) with state mapping: Backlog→open, Todo→open, In Progress→active, Done→done, Canceled→canceled
- Sub-issues → parent_id relationships
- Relations (blocks/blocked-by) → node_deps
- Comments → Node(kind=comment)
- Labels → tags[]
- Cycles → (no direct equivalent — cycles are a view, not a data structure)
- Projects → (can map to parent tasks or spaces)

**What doesn't migrate (and why):**
- Views/filters → user-specific, recreated by each user
- Integrations → different model (agents not webhooks)
- SLAs → priority model covers urgency; formal SLAs are future work

### From Jira

**What migrates:**
- Projects → Spaces
- Issues → Nodes(kind=task) with type mapping (Story, Bug, Task → tags)
- Subtasks → parent_id
- Links → node_deps
- Comments → Node(kind=comment)
- Components → tags or sub-spaces
- Sprint → (view filter, not data structure)

**What doesn't migrate:**
- Custom fields → (limited; tags cover categories, priority covers urgency)
- Workflows → (grammar operations replace custom workflows)
- Permissions → (Delegate/Scope ops replace Jira permission schemes)

### From Asana

**What migrates:**
- Projects → Spaces
- Tasks → Nodes(kind=task)
- Subtasks → parent_id (Asana supports multi-level; we support up to 5)
- Dependencies → node_deps
- Custom field values → (mapped to tags or priority where possible)
- Comments → Node(kind=comment)
- Sections → (can map to state or tags)

---

## 12. Convergence Analysis

Applied cognitive grammar (Need → Traverse → Derive) to this product spec.

### Pass 1: Need Row

**Audit:**
- Positioning ✓ (what it is, who it's for, why switch)
- Data model with state machines ✓
- API semantics ✓
- Trust/reputation ✓
- Agent integration ✓
- Governance ✓
- Views ✓
- Migration ✓
- Missing: **Notification model** — what triggers notifications in work context?
- Missing: **Metrics** — how do we know the work product is competitive?
- Missing: **Recurring tasks** — the Recurring modifier is in the grammar but not in the product spec

**Cover:**
- **Time tracking** — explicitly absent. Neither Linear nor we have it. Acknowledged gap; not a priority.
- **Estimation** — not specified. Linear has none/S/M/L/XL. We should support it via priority or a separate field.
- **Automation/rules** — Asana's rules engine. We have grammar operations + Triggers. Not yet specified as product feature.

**Blind:**
- The spec assumes work happens in one space. But real work spans spaces (an engineering task depends on a design task in another space). Cross-space dependencies are mentioned but not specified.
- The spec doesn't describe how Work and Social modes interact. A task discussion in the Board should be the same conversation visible in Chat. Currently they're separate UI paths.

### Derive Row Additions

**Notifications for Work:**
| Trigger | Who | Message |
|---------|-----|---------|
| Assigned to you | Assignee | "{actor} assigned you: {title}" |
| Task completed | Author, watchers | "{actor} completed: {title}" |
| Task blocked | Assignee | "{actor} blocked: {title}: {reason}" |
| Task unblocked | Assignee | "{actor} unblocked: {title}" |
| Review requested | Reviewer | "{actor} needs review: {title}" |
| Review verdict | Assignee | "{actor} {approved/revised/rejected}: {title}" |
| Mentioned | Mentioned user | "{actor} mentioned you in: {title}" |
| Overdue | Assignee, author | "Overdue: {title} was due {date}" |

**Metrics:**
- Velocity: tasks completed per cycle
- Throughput: tasks completed per week (rolling)
- Cycle time: median time from active → done
- Review turnaround: median time from review → verdict
- Agent contribution: % of tasks completed by agents
- Block rate: % of tasks that get blocked at least once

**Recurring tasks:** Task with `recurring` modifier in tags. On Complete, auto-creates a new instance with same title, priority, assignee, and incremented due_date. The recurrence pattern is in Op payload: { interval: "weekly", day: "monday" }.

**Work ↔ Social integration:**
- Task comments ARE chat messages — same Node(kind=comment) with parent_id. The Board task detail and Chat conversation share the same message thread.
- @mentioning a task ID in any Chat/Square/Forum message auto-resolves to an EntityPreview
- Completing a task can auto-post to the space's Feed (configurable)

### Pass 2: Fixpoint Check

**Audit:** All sections present. Notifications, metrics, recurring tasks added. Cross-mode integration specified.

**Cover:** Time tracking acknowledged as explicit non-goal. Estimation deferred to tags/priority. Automation via grammar Triggers.

**Blind:** Cross-space dependencies remain underspecified (requires Consent from both space owners — noted but not fully designed). This is the same as the Social spec's Federation gap — single-space first, cross-space later.

**Converged at pass 2.**
