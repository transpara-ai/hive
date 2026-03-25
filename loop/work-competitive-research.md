# Work Layer — Competitive Research

**What makes Linear, Asana, Monday, Notion, and GitHub Projects great, feature by feature. Every specific interaction. Mapped to our grammar. Where agents-as-peers and the event graph are genuinely better — not just different.**

Matt Searles + Claude · March 2026

---

## Methodology

For each product: enumerate every feature that matters to users. Be specific — not "has kanban" but "what happens when you drag a card." Map each to our grammar. Mark: ✓ we have this, ~ we partially have it, ✗ we don't have it, + we have something better.

The goal isn't to clone. It's to understand what makes these products *feel* great so we know what we need to match, what we can skip, and where we're genuinely better.

---

## 1. LINEAR

Linear is the current benchmark for developer-facing task management. Built by engineers, for engineers. Its core thesis: a task tracker should be fast, opinionated, and stay out of your way.

### 1.1 Navigation & Speed

**Feature: Keyboard-first navigation**
- `G then I` → My Issues
- `G then A` → All Issues
- `G then P` → Projects
- `C` → Create issue (from anywhere)
- `/` → Open search/command palette
- `←→` arrow keys to move between states in board view
- `E` → edit issue title inline
- Every action has a keybinding. Power users never touch the mouse.

Mapped to us: We have `Cmd+K` palette (iter 162), `?` help overlay + `G+B/F/C/A/K` shortcuts (iter 169). We partially have this. **Gap: we don't have issue-creation shortcut that pre-fills context (team, project, cycle). Our `C` key equivalent doesn't exist.**

**Feature: Instant search with fuzzy matching**
- Results appear as you type, no search button
- Scoped by context (within team, across org)
- Searches issues, comments, project names, user names in one box
- `Cmd+K` shows recent issues + recently visited

Mapped to us: We have `/search` (iter 145) and `Cmd+K` (iter 162). **Partial match — our search is a full page navigation, not an in-context dropdown overlay. Linear's search never leaves the current context.**

**Feature: Workflow speed — no page reload between issues**
- Single-page app. Navigating between issues doesn't re-render the shell.
- List → detail is a split-pane or slide-over, not a page navigation
- State changes are optimistic (UI updates immediately, syncs in background)

Mapped to us: HTMX + server-rendered. We do full HTMX swaps. **Structural gap: Linear's speed comes from SPA architecture. We can approximate with HTMX but there's inherent round-trip latency. Our optimistic chat send (iter 181) shows the pattern — we need it on all state changes.**

---

### 1.2 Issues (Tasks)

**Feature: Issue ID as first-class identifier**
- Every issue gets `ENG-123` — team prefix + sequential number
- IDs persist through all UI surfaces (board, list, notifications, git commits)
- Referencing `ENG-123` in any text auto-links
- IDs are short enough to type in commit messages (`fix ENG-123`)

Mapped to us: We use UUID node_ids. **Gap: no human-readable short IDs. UUIDs in commit messages are unusable. Linear's ID system is the reason dev teams adopt it — it bridges code and project management.**

**Feature: Issue states with custom workflows**
- Default states: Backlog, Todo, In Progress, In Review, Done, Canceled
- Teams can add custom states within each category (Todo → "Waiting for Design")
- States are ordered and colored per team
- Moving an issue to "Done" triggers a confetti animation (ceremony)

Mapped to us: We have open/active/review/done/canceled in our state machine. **Gap: no custom states. Linear's custom states let teams model their actual process (engineering: In Review, design: In Feedback, PM: Needs Spec). We've spec'd this but haven't built it.**

**Feature: Assignee + sub-assignees**
- One primary assignee (owner)
- Additional participants can be added (get notified but don't "own")
- Unassigned issues get surfaced in triage

Mapped to us: Single assignee_id. **Gap: no participants/watchers model. Our tags[] could hold watcher IDs but we haven't exposed this.**

**Feature: Labels/tags with color**
- Arbitrary labels, per-team color coding
- Labels are searchable, filterable
- Common: Bug, Feature, Improvement, Design, Documentation
- Labels can have parent labels (hierarchical)

Mapped to us: We have tags[]. **Gap: tags have no color, no hierarchy, no UI to manage them. They're strings, not entities.**

**Feature: Estimates (story points)**
- No points/S/M/L/XL — Linear made the deliberate choice: no estimates
- Reasoning: estimation is waste, completion rate and cycle time are the real metrics
- OPINION: this is correct for solo/small teams. Large teams need some estimation proxy.

Mapped to us: We have priority. **We match Linear here — priority is a better proxy for effort than arbitrary points.**

**Feature: Due dates with overdue highlighting**
- Optional due date per issue
- Board/list views highlight overdue issues in red
- Overdue notification sent to assignee

Mapped to us: We have due dates and overdue highlighting (iter 152-153). **✓ We have this.**

**Feature: Issue relations (blocks/blocked-by, duplicate, related)**
- `ENG-123 blocks ENG-456` — creates a directed dependency
- Blocked issues show a red "blocked" indicator everywhere
- "Duplicate of" relation — links issues, neither is deleted
- "Related to" — soft link, no blocking semantics

Mapped to us: We have node_deps (blocks/blocked-by) and full CRUD (iter 122-123, 130). **✓ We have this. Gap: no "duplicate" or "related" relation types — only blocking.**

**Feature: Subtasks**
- Linear calls them "sub-issues" — any issue can be a child of another
- Parent shows subtask progress bar (X/Y completed)
- Subtasks can have their own assignee, priority, due date
- Max depth: approximately unlimited in practice (no enforced limit)

Mapped to us: We have parent_id and subtask tree (iter 62-72). **✓ We have this. Our max depth is 5 (bounded invariant). Linear's unlimited depth is a footgun — our limit is correct.**

**Feature: Attachments and images inline**
- Drag-and-drop files onto issue detail
- Images render inline in description
- Files stored in Linear's CDN, linked in the event log

Mapped to us: **✗ We have no file attachment support. Not critical for MVP but necessary for teams that attach design files, screenshots, logs.**

**Feature: Issue templates**
- Team-level templates for common issue types (Bug report: steps to reproduce, expected, actual)
- Templates pre-fill title, description, labels, assignee, priority
- Reduces friction for common workflows

Mapped to us: **✗ No templates. Our new-task form is blank. Templates are a "team setup" feature — important for adoption.**

---

### 1.3 Cycles (Sprints)

**Feature: Time-boxed work periods**
- Cycles are the Linear equivalent of sprints
- Fixed duration (1 week default, configurable)
- Issues are added to cycles manually or via automation
- In-progress work at cycle end auto-carries to next cycle

Mapped to us: **✗ We have no cycles/sprints. This is a gap for teams that work in iterations. Our board is a perpetual backlog, not a time-boxed view.**

**Feature: Cycle overview**
- Progress bar: X issues completed / Y total
- Velocity: issues completed this cycle vs. last cycle
- Scope creep indicator: issues added after cycle started
- Health status: on track / at risk / off track

Mapped to us: **✗ No cycle-level metrics. We have per-issue overdue highlighting but no sprint health view.**

**Feature: Auto-carryover**
- At cycle end, incomplete issues automatically move to next cycle
- No manual work to "close" a sprint
- Historical cycle views remain (what shipped in Sprint 42?)

Mapped to us: **✗ No automatic carryover. Our time construct is due_date on individual tasks, not a container.**

---

### 1.4 Projects

**Feature: Projects as meta-containers**
- Projects group issues across multiple teams
- A product launch project can span Engineering + Design + Marketing teams
- Projects have: description, target date, status, lead, icon, color
- Projects show progress aggregate (% of issues done)

Mapped to us: We have KindProject. **Partial match — our project nodes exist but have no cross-space/cross-team aggregation. A project in our system is a node within one space, not a container spanning spaces.**

**Feature: Project health updates**
- Weekly prompt: "Add a project update"
- Status: On Track / At Risk / Off Track / Completed / Paused
- Free-text update (rendered as timeline of status changes)
- Stakeholders subscribe to project updates

Mapped to us: We have the `progress` op which records progress updates. **Partial match — our progress op works but isn't surfaced as "project status updates" with the at-risk/on-track framing.**

**Feature: Project milestones**
- Sub-containers within a project
- Issues can be assigned to milestones
- Milestone has target date
- Project roadmap view = milestones on a timeline

Mapped to us: **✗ No milestones. Our hierarchy is Space → Project → Task → Subtask but milestones are a mid-level grouping we don't have.**

---

### 1.5 Triage

**Feature: Inbox for incoming issues**
- New issues assigned to you or your team land in Triage
- One-at-a-time processing (inbox zero model)
- For each: Assign to cycle, label, prioritize, or move to team backlog
- Keyboard-driven: select action and next issue auto-loads

Mapped to us: We have the `triage` named function in our spec but haven't built the UI. **✗ Gap: Triage is how teams get control of incoming work. Without it, every new issue becomes noise.**

---

### 1.6 Views

**Feature: My Issues**
- All issues assigned to the current user, across all teams
- Grouped by: Team, Project, Cycle, Priority, State
- Quickly switch between groups

Mapped to us: We have the Personal Dashboard (iter 125). **✓ Partial match — we have "Assigned to me" on the dashboard. Gap: our dashboard doesn't group by team/project/cycle the way Linear does.**

**Feature: Board view (Kanban)**
- Columns = states (Backlog, Todo, In Progress, Done)
- Cards show: priority icon, issue ID, title, assignee avatar, label chips, cycle indicator
- WIP limits (e.g., max 3 in "In Progress")
- Group by: Assignee, Project, Priority (switch between groupings)
- Filter: any combination of assignee/label/priority/milestone

Mapped to us: We have Board view with DnD (iter 163). **✓ Core kanban. Gap: WIP limits, group-by switching, filter combinations are missing.**

**Feature: List view**
- Tabular: issue ID, title, assignee, status, priority, cycle, label chips, due date
- Sortable columns
- Inline editing: click cell to edit (status, assignee, priority)
- Selectable rows for bulk actions (re-prioritize, assign, move to cycle)

Mapped to us: We have List view (iter 200) with sortable columns. **✓ Partial match. Gap: inline cell editing (our list view links to detail pages) and bulk actions.**

**Feature: Timeline view (Gantt-like)**
- Issues on a horizontal time axis
- Dependency arrows between issues
- Drag bars to reschedule
- Critical path highlighting (which chain of dependencies determines the deadline)
- Group by: Team, Project, Milestone

Mapped to us: Spec'd but not built. **✗ Gap: timeline view is the "show me the whole picture" view PMs live in.**

---

### 1.7 Integrations

**Feature: Git integration (GitHub/GitLab)**
- Link branch/commit/PR to issue
- Auto-update issue status when PR is merged (issue → Done)
- Show open PRs directly on issue detail
- Filter issues by: has PR, PR status (open/merged/review)

Mapped to us: **✗ No git integration. For a dev-focused product this is a major gap. Our agents can fill some of this (agent marks task complete when it commits code) but the GitHub webhook model is what devs expect.**

**Feature: Slack integration**
- `/linear create` slash command from Slack
- Issue preview unfurl when URL is pasted
- Notifications in Slack channel (configurable: which events, which channel)
- Reply to Linear notification thread to comment on issue

Mapped to us: **✗ No Slack integration. Not critical pre-product-market-fit but important for enterprise adoption.**

---

### 1.8 What Makes Linear *Feel* Great

1. **Speed is a value, not a metric.** Every interaction is under 100ms locally. Keyboard shortcuts eliminate every mouse click. Linear made speed a design principle, not a feature.

2. **Opinionated simplicity.** Linear removed estimates, timeboxed cycles are optional, no custom field explosion. Fewer choices = faster adoption. Teams configure by using, not by configuring.

3. **The issue ID as commit-friendly reference.** `ENG-123` in a commit message closes the loop between code and work. This is why dev teams don't switch off Linear.

4. **Ceremony on completion.** The confetti animation when you hit Done is tiny but meaningful. Completing work feels good. Linear understands the emotional layer.

5. **No bloat.** Linear didn't add "goals" or "documents" or "wikis" until recently (and when they did, users complained). Restraint is a feature.

---

## 2. ASANA

Asana is the task management platform for teams that need more structure than a simple kanban. Its core thesis: every team at every company should be able to manage work the same way — standardized, auditable, integrated.

### 2.1 Projects and Sections

**Feature: Projects as views + tasks containers**
- Every project has multiple views: List, Board, Timeline, Calendar, Files
- Within a list view, tasks are grouped into Sections (collapsible headers)
- Sections function as lightweight sprint containers or workflow stages

Mapped to us: Spaces + Board/List lenses. **Partial match — our space has multiple lenses. Gap: Sections within a view are a key organizing layer we don't have. Users want to group tasks within a view without creating sub-spaces.**

**Feature: Task home** (personal, cross-project view)
- My Tasks is an Asana-wide view of all tasks assigned to you
- Sections in My Tasks: Recently assigned, Today, Upcoming, Later
- Drag tasks between sections to reschedule relative importance
- "Do today" concept = tasks you're committing to complete today

Mapped to us: Personal Dashboard (iter 125). **Partial match — we show assigned tasks. Gap: the "today" grouping and intentional scheduling of which tasks you're doing NOW is a unique Asana feature. It's a personal planning interface.**

**Feature: Section rules (automations)**
- When a task moves to a section, trigger: assign to person, add label, set due date, notify
- When a field changes, trigger: move to section, set field value, create subtask
- No-code automation builder (IF trigger THEN action)
- 50+ built-in triggers and actions

Mapped to us: **✗ No automation rules. This is a major gap for medium/large teams. Linear has this too (called "workflows"). Our grammar operations are expressive but require explicit user action — there's no "when X happens, automatically do Y."**

**Agent opportunity:** This is exactly where agents-as-peers win. Instead of a rules engine (static if-then), an agent watches events on the graph and reasons about what to do. An agent can handle "when a task is marked urgent, decompose it into subtasks, assign the first one to the on-call person, and notify the lead" — no rule configuration required. The agent has context, judgment, and can explain its actions.

---

### 2.2 Timeline View

**Feature: Gantt chart with dependencies**
- Horizontal bars = task duration (start → due)
- Colored by: assignee, project, milestone
- Connecting arrows = dependencies (drag to create)
- Drag bars to reschedule (updates dates, shifts dependent tasks automatically)
- Zoom: day / week / month / quarter

Mapped to us: Spec'd, not built. **✗ For project managers tracking multi-week deliverables this is essential.**

**Feature: Critical path**
- Highlight the chain of dependencies that determines the earliest possible completion date
- Dimmed tasks = not on critical path
- Red highlighting = critical path task that's at risk (overdue or no assignee)

Mapped to us: **✗ Critical path requires dependency graph traversal + date math. Algorithmically straightforward but UI-intensive.**

---

### 2.3 Portfolios

**Feature: Portfolio = collection of projects**
- Portfolio shows: each project's status, owner, timeline progress, percent complete
- Portfolio health view: all projects on one screen, health status (On Track / At Risk / Off Track)
- Portfolio is the C-suite view: what are we building, is it on track, who owns it

Mapped to us: **✗ No portfolios. Our Spaces are flat — there's no "container of spaces" with aggregate status. This is a mid-market/enterprise requirement.**

**Agent opportunity:** An agent can maintain a portfolio status by reading across spaces and generating a weekly summary. No manual portfolio updates required. Human updates their Linear/Asana status weekly — our agents do it automatically from event data.

---

### 2.4 Goals (OKRs)

**Feature: Goals linked to work**
- Goals have: title, owner, due date, metric target (revenue $X, users Y, etc.)
- Goals link to projects (projects contribute to goals)
- Goals link to tasks (tasks contribute to goals)
- Progress auto-updates from linked work (% of linked tasks done)

Mapped to us: We have KindGoal (iter 222). **Partial match — we have the entity. Gap: goals don't auto-update from linked tasks. The linkage is manual and not reflected in goal progress.**

**Feature: Goal hierarchy (OKR tree)**
- Company goals → Department goals → Team goals → Individual goals
- Parent goal progress = aggregate of child goal progress
- Misalignment view: goals with no linked work (pure intention, no execution)

Mapped to us: **Partial match — our node hierarchy via parent_id supports goal trees. Gap: no progress aggregation, no alignment view.**

---

### 2.5 Forms (Intake)

**Feature: Public intake forms that create tasks**
- Design a form with custom fields
- Share link publicly (no Asana account required to submit)
- Submission creates a task in a specific project with section and field values pre-filled
- Useful for: bug reports, feature requests, IT tickets, support requests

Mapped to us: **✗ No intake forms. This is how non-technical stakeholders create work without learning the task system.**

**Agent opportunity:** Our agents can be the intake layer — a conversation with an agent creates a task. "I have a bug to report" → agent asks clarifying questions → agent creates the task with proper fields. This is better than a form: dynamic, clarifying, context-aware.

---

### 2.6 Workload View

**Feature: Capacity planning**
- View all team members' tasks on a weekly calendar
- Overloaded: more tasks than configured capacity (hours)
- Underloaded: capacity available
- Drag tasks between people to rebalance
- Integrates with "task effort" estimates

Mapped to us: **✗ No workload/capacity view. This is the manager's view: "who has bandwidth?"**

**Agent opportunity:** An agent can perform workload balancing automatically — monitor who's overloaded, suggest task redistribution, alert when a team member has nothing to do. This is better than manual drag-and-drop.

---

### 2.7 Status Updates (Asana feature unique to its market)

**Feature: Structured project status updates**
- Any project member can post a status update: On Track / At Risk / Off Track
- Status update has: title, body (rich text), key accomplishments, next steps, blockers
- Status history is visible: can see how a project evolved over time
- @mention stakeholders to notify them
- Executive summary mode: auto-generates a summary from tasks

Mapped to us: We have the `progress` op. **Partial match — our progress op records updates. Gap: no structured "status update" format with accomplishments/blockers/next steps. And critically: no "executive summary" view that aggregates updates across projects.**

---

### 2.8 What Makes Asana *Feel* Great

1. **Multiple views from one data set.** The same project shows as List, Board, Timeline, Calendar without reconfiguration. Power users switch based on the question they're answering (what's the sequence? → Timeline; who's doing what? → Workload; what's next? → Board).

2. **No code automation.** Asana rules (IF/THEN) let non-technical users automate workflows. This is huge for ops teams, marketing teams, HR — anyone who isn't a developer.

3. **Portfolio as executive view.** VPs need to see all projects at once. Asana's portfolio view doesn't require any changes to how individual teams work — it's just a lens on existing data.

4. **Public forms for intake.** Decouples the work requestor from the work tracker. You don't need to know Asana to create work in Asana. This dramatically expands who can use the tool.

5. **Task "Home" as daily planning.** Asana's My Tasks with Today/Upcoming/Later sections is a personal commitment interface. You're not just tracking tasks — you're deciding what you're doing today. Small but powerful.

---

## 3. MONDAY.COM

Monday is the most configurable of the task trackers. Its core thesis: all work is structured data, and every team can model their process as a board with custom columns.

### 3.1 Boards and Items

**Feature: Items as rows, columns as fields**
- Everything is a "board" — a table with custom columns
- Each "item" is a row (equivalent to a task, but could be anything: contact, deal, inventory item)
- Column types: text, number, date, person, status, dropdown, checkbox, tags, formula, link, file, ratings, progress bar, vote, time tracking, phone, email, location, color

Mapped to us: Nodes with fixed fields. **Structural difference — Monday's radical configurabiilty (any column type) is both its strength and weakness. Our fixed schema (title, body, state, priority, assignee, due_date, tags) is more opinionated and less configurable. We bet on "right columns" vs "any columns."**

**Feature: Status column (customizable per board)**
- Each board has its own status columns with custom labels and colors
- "Done" column turns item green, triggers automations
- Status columns can be non-linear (items don't have to flow through states in order)
- Multi-status boards: a single item can have multiple status columns (e.g., "Development Status" and "Design Status")

Mapped to us: Single state machine (open → active → review → done). **Gap: multi-status columns are a genuine need for cross-functional work where design and development track status independently on the same task.**

**Agent opportunity:** Instead of multi-status columns, an agent can maintain "parallel workstreams" on a task — automatically tracking where design and dev are independently via comments and state transitions. The event graph captures this better than static columns.

---

### 3.2 Automations

**Feature: No-code automation (200+ recipes)**
- Recipes: "When status changes to Done, notify someone in Slack"
- "When date arrives, assign item to person"
- "When item is created in this board, send email to client"
- "Every Monday at 9am, create a new item from template"
- Custom automation builder: any trigger + any action

Mapped to us: **✗ No automations. Monday's automation library is extensive and deeply integrated with their column types. This is a significant product capability gap.**

**Agent opportunity:** Instead of static automation recipes, agent-based triggers. "Watch this space. When any task stays in 'active' state for more than 3 days with no progress update, create a 'follow up' task for the manager and post a comment asking for a status." Agents can reason about patterns, not just fire simple rules.

---

### 3.3 Views and Dashboards

**Feature: Workspaces and boards**
- Hierarchical: Account → Workspace → Board → Groups → Items
- Groups = sections within a board (collapsible, can be different colors)
- Items can be in multiple boards simultaneously (linked items)

Mapped to us: Account → Space → Node. **Gap: no groups within spaces (sections). Items in multiple boards is powerful but complex — our spaces are exclusive containers.**

**Feature: Dashboard widgets**
- Dashboards aggregate data from multiple boards
- Widgets: battery chart (progress toward goal), workload (per-person), timeline, calendar, chart (custom metrics), numbers (count/sum/average), text (status updates)
- Dashboards can be shared with external stakeholders (view-only link)

Mapped to us: We have Activity feed and Dashboard views. **Gap: no custom metric widgets, no aggregate views across spaces with visual charts.**

**Feature: Calendar view**
- Items plotted on a calendar by date field
- Month/week/day zoom
- Drag items to reschedule
- Filter by assignee, status

Mapped to us: **✗ No calendar view.**

---

### 3.4 Forms (WorkForms)

**Feature: Submission forms with conditional logic**
- Build forms with conditional fields (if X is selected, show Y)
- Branching questions based on answers
- File upload in forms
- Submissions appear as board items immediately
- Approval workflow: submitted items go to "Review" state, manager approves

Mapped to us: **✗ No forms. More powerful than Asana's forms due to conditional logic.**

---

### 3.5 What Makes Monday *Feel* Great

1. **Infinite configurability.** Monday can model ANY process — from software development to HR onboarding to construction project management. The column system is the key abstraction.

2. **Automations reduce toil.** Monday's automation recipes handle repetitive work that every team has but nobody wants to do manually. "When status = Done, move to Done board, notify client, log in CRM."

3. **External stakeholder access.** Monday lets external parties (clients, vendors) see specific boards without full accounts. This is a B2B superpower.

4. **The visual density.** Monday boards are dense with information — every column type is visually distinct, progress bars and color coding give instant status at a glance.

5. **Templates for every workflow.** Monday has 200+ templates for specific industries and processes. You don't start from blank. You start from "Software development sprint" and customize.

---

## 4. NOTION

Notion is not a task tracker — it's a structured knowledge base that also manages tasks. Its core thesis: documents, databases, and wikis are the same thing — structured data with flexible views.

### 4.1 Databases

**Feature: Any content can be a database**
- Create a database: it's a table where each row is a page
- Switch views: Table, Board, List, Gallery, Timeline, Calendar
- Database properties: Text, Number, Select, Multi-Select, Date, Person, Checkbox, URL, Email, Phone, Formula, Relation, Rollup, Created Time, Last Edited, Files & Media
- Multiple views of the same database with different filters/sorts/groupings

Mapped to us: Nodes as typed objects, multiple lenses. **Our architecture is equivalent — every Node is a page, every lens is a view. Key gap: we don't support custom properties per-kind. Our nodes have fixed fields; Notion's databases have arbitrary properties.**

**Feature: Relations and rollups**
- Relation: link rows in database A to rows in database B
- Rollup: calculate a value from related records (count, sum, average, earliest date, etc.)
- Example: Project database with Tasks database linked. Projects roll up "count of done tasks" as progress percentage.
- Bidirectional by default

Mapped to us: parent_id + node_deps. **Partial match — we have parent/dependency relationships. Gap: rollup calculations (a project's "% done" calculated from linked tasks) aren't automatic. We have the data, not the derived views.**

---

### 4.2 Wiki and Documents

**Feature: Hierarchical page tree**
- Infinite nesting of pages
- Sidebar shows page tree (collapsible, drag-to-reorder)
- Pages can be databases, documents, or both
- Linked pages: embed any page as a block in another page

Mapped to us: Our space as container + KindDocument. **Gap: we don't have a page tree sidebar. Our Knowledge lens shows a flat list of documents, not a navigable hierarchy.**

**Feature: Block-based editor**
- Every page is composed of blocks: paragraph, heading, bulleted list, numbered list, toggle, quote, callout, code, image, video, file, bookmark, equation, table, database embed
- Drag blocks to reorder
- `/` command to insert any block type
- Nested blocks: any block can contain child blocks

Mapped to us: Body as markdown text. **Major gap: our node body is plain markdown (rendered, not block-editable). Block editors are powerful but complex to build. Notion spent years on this.**

**Agent opportunity:** An agent can write to our knowledge nodes via the existing op interface. The markdown body IS an agent-writable knowledge store. We don't need a block editor to have agent-contributed documents.

---

### 4.3 Task Management

**Feature: Tasks as database rows with properties**
- Task database: checkbox (done), assignee (person), due (date), project (relation), tags (multi-select), status (select), priority (select), estimate (number)
- Board view groups by status
- Timeline view groups by project, shows due dates
- Filter: any combination of properties
- Sort: any property

Mapped to us: Board + List views on task nodes. **Partial match — our views exist. Gap: Notion's filter/sort UI is infinitely flexible because all task properties are first-class columns. Our filters are hardcoded.**

**Feature: Linked databases**
- Show a filtered view of a database inside a different page
- Example: on a "Project Alpha" page, embed a view of the Tasks database filtered to Project Alpha
- Changes in the embed update the source database

Mapped to us: **✗ No linked database views. In our system, tasks belong to one space. You can't embed a filtered task view inside a document page.**

---

### 4.4 Notion AI

**Feature: AI writing assistance**
- `/AI` command generates content in any block
- Summarize selection, fix grammar, make shorter/longer, change tone
- "Ask AI about this page" — Q&A grounded in page content
- AI autofills database properties: "fill description based on title"

Mapped to us: Our Mind already does this. **✓ We have AI grounded in space context. Gap: we don't have inline AI writing assistance (Cmd+K style AI on any text selection).**

**Agent opportunity:** Our agents are peers, not tools. Notion AI is a writing assistant. Our agents can be collaborators who write documents, create tasks from documents, maintain the knowledge base. Qualitatively different.

---

### 4.5 What Makes Notion *Feel* Great

1. **Flexible structure.** No schema is fixed. A team meeting note can also be a task (add a "Checkbox" property). A product spec can embed a task database. The tool bends to how the team thinks, not how the tool thinks.

2. **The block editor is delightful.** Drag-and-drop blocks, `/` command, callouts, toggles, code blocks — creating beautiful structured documents is fast and enjoyable.

3. **Database as first-class.** Notion treats structured data as a fundamental primitive, not an afterthought. Every list is queryable, sortable, filterable.

4. **Single source of truth.** A company can genuinely put everything in Notion — docs, tasks, wikis, databases. Fewer tools = less context-switching.

5. **Templates as starting points.** Notion's template gallery (community + official) means you don't have to invent your system from scratch. Copy a "Product Roadmap" template and it Just Works.

---

## 5. GITHUB PROJECTS

GitHub Projects is the task tracker for software teams that want to stay in their code repository. Its core thesis: task management should be where the code is.

### 5.1 Issues and PRs as First-Class

**Feature: Issues are linked to code**
- Issue created in repo, associated with repo, assignees are repo contributors
- Close with commit: `Fixes #123` in commit message auto-closes issue
- Issue body supports markdown including checkboxes (`- [ ] item`)
- Checkboxes in issues become tracked "tasks" (shows 3/5 complete on issue list)

Mapped to us: **✗ No git integration. This is GitHub's core moat — you can't have this integration without being in the repo. Agent teams commit code and can annotate with task IDs — partial substitute.**

**Feature: Issue templates**
- `.github/ISSUE_TEMPLATE/` directory
- Bug report template: environment, steps to reproduce, expected, actual
- Feature request template: user story, acceptance criteria
- Custom templates: any team process
- Template chooser when creating new issue

Mapped to us: **✗ No templates. See Linear section.**

**Feature: Milestones**
- Milestone = time-boxed collection of issues
- Milestone has: title, description, due date
- Progress bar shows open vs. closed issues
- Can filter issues by milestone

Mapped to us: **✗ No milestones. See Linear/Asana sections.**

---

### 5.2 Projects (v2)

**Feature: Tables and boards from GitHub issues**
- GitHub Projects (v2) is a database view over issues and PRs
- Fields: assignees, labels, milestone, status, priority (custom), iteration (sprint), any custom field
- Views: Table (default), Board (group by field), Roadmap (timeline)
- Filter, sort, group by any field

Mapped to us: Our board and list views. **Partial match. Gap: GitHub Projects can pull issues from ANY repo in your org, creating cross-repo task views. Our spaces are isolated.**

**Feature: Iterations (sprints)**
- Iteration field: define sprint periods (e.g., 2-week sprints)
- Assign issues to iterations
- Burndown chart: issues remaining in iteration vs. time
- Automatic: current iteration highlighted, auto-advance on cycle end

Mapped to us: **✗ No iterations/sprints. See Cycles section.**

---

### 5.3 What Makes GitHub Projects *Feel* Great

1. **Zero context switch.** You write code, you see issues, you commit fixes. The workflow never leaves GitHub. For engineers, this is the killer feature.

2. **Close with commit.** `Fixes #123` is the most natural task completion flow ever built. No clicking, no navigating — just write a good commit message.

3. **Issues as first-class markdown with checkboxes.** A GitHub issue with 5 checklist items IS a task breakdown. Crude but immediately useful.

4. **Pull request as the unit of review.** PRs are the work-review loop already baked in. Every task has a review cycle embedded in the code change.

5. **Free for open source.** The entire open-source ecosystem lives here. If you're building open source, GitHub is the only real option.

---

## 6. CROSS-CUTTING FEATURES (GAPS WE ALL HAVE)

Features present in multiple products that we should prioritize:

### 6.1 Sprint/Cycle Management
**Linear (Cycles), Asana (Sections as pseudo-sprints), GitHub Projects (Iterations)**
- Time-boxed containers for work
- Progress tracking within the container
- Carryover at the end
- **Our gap:** No time-boxed view. This is a significant gap for teams that work in sprints.

### 6.2 Timeline/Gantt View
**Linear (Timeline, in development), Asana (Timeline), Monday (Timeline), GitHub Projects (Roadmap)**
- Dependencies visualized on a time axis
- Drag-to-reschedule
- Critical path
- **Our gap:** Spec'd but not built. High priority for project managers.

### 6.3 Workload/Capacity View
**Asana (Workload), Monday (Workload widget)**
- Per-person capacity visualization
- Overloaded/underloaded indicators
- Drag-to-rebalance
- **Our gap:** Not built. Agent-based rebalancing is our differentiator here.

### 6.4 Automations/Rules
**Asana (Rules), Monday (Automations), GitHub Actions (adjacent)**
- IF trigger THEN action
- Common: assignment on state change, notifications on due date, status updates
- **Our gap:** No rule engine. This is where agents-as-peers are genuinely better — see §7.

### 6.5 Short Issue IDs
**Linear (ENG-123), GitHub Projects (#123), Jira (ENG-123)**
- Human-readable, copy-able, commit-message-friendly IDs
- **Our gap:** UUIDs only. Major usability issue for dev teams.

### 6.6 Templates
**Asana, Monday, Notion, GitHub Issues**
- Project templates, task templates, form templates
- **Our gap:** No templates. Increases onboarding friction significantly.

### 6.7 Cross-Project/Cross-Space Views
**Monday (multi-board dashboards), GitHub Projects (cross-repo), Notion (linked databases)**
- See all your work regardless of which project it's in
- **Our gap:** Our Personal Dashboard is a start. Cross-space task views need to be a first-class feature, not a sidebar thought.

---

## 7. WHERE AGENTS-AS-PEERS WIN

These are our genuine differentiators — features that agents-as-peers make structurally better, not just equivalently different.

### 7.1 Automation is Intelligence, Not Rules

**The problem with rules engines:** Every automation product (Asana rules, Monday automations, Zapier) encodes IF/THEN logic. Teams spend time configuring rules. Rules break when workflows change. Rules can't reason about context — they match patterns.

**Our advantage:** Agents watch the event stream and reason about what to do. "When a task stays in active for 3 days with no progress update, look at the task's context, decide if it's blocked or just slow, and act accordingly." No configuration. No rule maintenance. The agent has judgment, not just triggers.

**Specific features competitors can't replicate:**
- **Adaptive prioritization:** Agent watches what's happening across spaces, surfaces urgent work before the human notices, explains why it surfaced it
- **Smart decomposition:** Agent decomposes tasks based on the actual codebase and team's history, not a fixed template
- **Pattern learning:** Agent notices that task X is always blocked by task Y, suggests adding a dependency rule
- **Context-aware assignment:** Agent assigns tasks based on who has bandwidth AND who has the relevant skills (from work reputation), not just who has the lowest count

---

### 7.2 Review is a Peer Operation, Not a Widget

**The problem with competitor review flows:** Asana has no review workflow. Linear is adding it. GitHub has PR review (code only). In all cases, "review" is a workflow state, not a first-class actor.

**Our advantage:** Review is a grammar operation. The reviewer is an actor on the graph, with identity, history, and reputation. An agent can be a reviewer — it can read the task, read the code changes, evaluate correctness, and produce a structured verdict (approve/revise/reject + explanation + evidence). This verdict is signed, attributable, and permanent.

**Specific capabilities:**
- Agent reviews code tasks before human review (catches obvious issues)
- Agent can be a mandatory reviewer for certain task types (tests required, no magic strings, etc.)
- Review reputation: agents that review accurately get higher trust, agents that miss things get lower trust
- Review as training data: the event graph records what was caught in review and what wasn't — this feeds back into the agent's judgment

---

### 7.3 Delegation is Declarative, Not Email

**The problem with competitor delegation:** "Assigning" a task to someone means they're the owner. What they can DO with that task is implicit (governed by role + convention). When you delegate to an agent, you're trusting it to behave correctly without any mechanism to define "correctly."

**Our advantage:** The `assign` op with `scope` parameter. An agent can be assigned a task with explicit capabilities: `[read_code, write_code, run_tests, comment, NOT merge, NOT deploy]`. The scope is a data structure on the graph. If the agent tries to act outside scope, the action is rejected and an escalation event is emitted.

**Specific capabilities:**
- Fine-grained agent permissions per task (not per agent globally)
- Scope can expire (time-bounded delegation)
- Scope can be escalated (agent requests broader capability, human approves)
- Full delegation chain: who authorized this agent to do this, when, under what terms

---

### 7.4 History is Causal, Not Chronological

**The problem with competitor history:** Every task tracker has an "activity log" — a chronological list of what happened. Linear's activity log is comprehensive. Asana's is decent. But they all answer "what happened" not "why it happened."

**Our advantage:** Every event declares its causes. A task completion Op declares what informed it. A scope expansion declares the task that required it. An escalation declares what was blocked. You can trace any state from its current value back through every decision and cause to its origin.

**Specific capabilities:**
- "Why is this task urgent now?" — trace back to the event that triggered the urgency change, and the event that triggered that
- "Who authorized the agent to do this?" — trace the scope grant back to the human who approved it
- Full audit trail for regulated industries: every action is signed, attributable, and causally linked
- Historical counterfactuals: "what would have happened if this task was assigned to person B instead of person A?" (agents can reason over the causal graph)

---

### 7.5 Trust Accumulates From Evidence, Not Title

**The problem with competitor permissions:** Role-based access (admin/member/viewer). Permissions are declared, not earned. An agent is either "allowed" or not, based on configuration.

**Our advantage:** Work reputation from completed tasks. An agent that completes 50 tasks with high review quality earns the trust to take harder tasks with broader scope. New agents get simple tasks. Trust is earned, not assigned.

**Specific capabilities:**
- Natural trust escalation path for agents (starts with leaf tasks, earns complex decomposition)
- Reputation is cross-space: a trusted agent in one space gets a head start in a new space
- Reputation decay: agents that haven't worked recently start with less trust
- Human oversight without micromanagement: trust thresholds determine when human review is required vs. when agent can proceed autonomously

---

### 7.6 Agent Work Is Explained, Not Just Done

**The problem with competitor "AI features":** Notion AI generates text. Monday AI fills columns. Linear AI triages issues. These are tools — they act, but they don't explain. There's no audit trail. No way to trace why the AI did what it did.

**Our advantage:** Every agent action is an Op on the graph. The Op has: actor_id (the agent), payload (the action + reasoning), causes (what triggered it). When an agent decomposes a task, you can see: what task it decomposed, what subtasks it created, what its reasoning was (in the Op body), and what caused it to do this (the assignment event).

**Specific capabilities:**
- "Why did the agent create this subtask?" — read the Decompose op, it has the reasoning
- "Did the agent's scope allow this?" — trace the Scope op
- "Who assigned the agent to this task?" — trace the Assign op
- Agents can explain their work in natural language in the same interface humans use

---

## 8. BUILD PRIORITY MATRIX

Based on this research, ranked by: competitive necessity × user value × agent advantage.

| Feature | Competitive Necessity | User Value | Agent Advantage | Build Priority |
|---------|----------------------|------------|-----------------|----------------|
| Short issue IDs (ENG-123) | Critical (Linear, GitHub) | High | Low | **P0** |
| Sprint/Cycle management | High (Linear, Asana, GitHub) | High | Medium | **P1** |
| Inline automations/triggers | High (Asana, Monday) | High | Agent replaces this | **P1 (agent-native)** |
| Timeline/Gantt view | High (all) | High | Low | **P1** |
| Workload/capacity view | Medium (Asana, Monday) | High | High (agent rebalancing) | **P1 (agent-enhanced)** |
| Custom states per space | Medium (Linear) | Medium | Low | **P2** |
| Task sections within view | Medium (Asana, Monday) | Medium | Low | **P2** |
| Intake forms | Medium (Asana, Monday) | Medium | Agent replaces | **P2 (agent-native)** |
| Cross-space aggregate views | Medium (Monday, Notion) | High | High | **P2** |
| Task templates | Medium (all) | Medium | Low | **P2** |
| Milestone containers | Low (Linear, Asana, GitHub) | Medium | Low | **P3** |
| Git integration | High (GitHub) | High for devs | Medium | **P3 (later)** |
| External stakeholder access | Low (Monday) | High for agencies | Low | **P3** |
| Block editor | Low (Notion) | Medium | Low | **P3 (complex, skip for now)** |

### Agent-Native Replacements (don't build the feature, build the agent):

| Missing Feature | Agent Replacement | Why It's Better |
|----------------|------------------|-----------------|
| Automation rules | Observer agent watching event stream | Adapts to context, explains actions, no config |
| Intake forms | Conversational intake via chat | Dynamic questions, context-aware, no UI to build |
| Portfolio status updates | Observer agent generating weekly summaries | Automatic, grounded in actual event data |
| Workload rebalancing | Allocator agent suggesting and explaining moves | Considers skill, reputation, task context |
| Daily standup | Observer agent generating daily digest per person | From event data, no manual updates required |

---

## 9. WHAT WE MUST NOT COPY

These are features where competitors are wrong and we should consciously not replicate them:

1. **Endless custom fields.** Monday's infinite column types produce configuration sprawl. Teams end up with 40 columns, 90% unused. Our fixed schema + tags is more opinionated and actually better for most teams.

2. **Block editors.** Notion's block editor is powerful but complex. Our markdown body is good enough for 90% of task descriptions. Save the engineering cost for things that matter more.

3. **Time tracking.** Neither Linear nor we have it. Estimation + cycle time is a better proxy. Don't add time tracking.

4. **Custom notification rules.** Every competitor has this ("notify me when..."). Our event stream + agents makes this better: an agent learns your notification preferences from feedback, not from configuration.

5. **Unlimited nested tasks.** No enforced depth limit (Linear) causes accidental 10-level hierarchies that nobody can navigate. Our 5-level limit is correct.

6. **Per-user themes and personalizations.** This is maintenance cost that doesn't make the product better. Ember Minimalism is our identity. It's not configurable.

---

## 10. THE ONE-LINE PITCH AGAINST EACH COMPETITOR

| Competitor | Why They'd Switch |
|-----------|------------------|
| **Linear** | Agents that DO the work, not just track it. Causal history that explains why, not just what. Scoped delegation that makes agent autonomy safe. |
| **Asana** | Agents replace rules engines — smarter, no configuration. Automation that explains its reasoning and can be challenged. Reviews that agents can participate in as peers. |
| **Monday** | Opinionated schema + agent intelligence beats infinite columns + static recipes. Your agent learns your process; you don't configure it. |
| **Notion** | A knowledge base where agents contribute, maintain, and answer questions — not just a writing assistant that completes your sentences. |
| **GitHub Projects** | Agents participate in the same repo-adjacent workflow: they create issues, claim tasks, commit code, complete tasks, and explain what they did. They're contributors, not bots. |

---

*Generated: March 2026. Method: feature enumeration + grammar mapping + agent advantage analysis.*
