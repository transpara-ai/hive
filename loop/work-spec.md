# Work Layer Specification

**Views and components described as compositions of Code Graph primitives. Iterated to convergence via the cognitive grammar.**

Matt Searles + Claude · March 2026

---

## Architecture

The Work layer is four **views** (Board, Dashboard, Triage, Timeline) sharing one data model and reusing the Social layer's shared components (ComposeBar, MessageBubble, EntityPreview, ConsentCard) for task discussion.

| View | Replaces | Primary Work Ops | Core Pattern |
|------|----------|-----------------|--------------|
| **Board** | Linear board, Jira kanban | Intend, Claim, Complete | Visual state columns, drag between |
| **Dashboard** | Linear "My Issues" | Progress, Review | Personal work queue, cross-space |
| **Triage** | Linear triage inbox | Prioritize, Assign, Scope | Incoming work decisions |
| **Timeline** | Asana timeline, Gantt | Intend, Block, Decompose | Time-axis dependency visualization |

---

## Shared Work Components

### TaskCard

The universal task display. Used in Board columns, Dashboard lists, Triage queue, search results.

```
TaskCard(task, context) = Layout(row, gap: sm, class: "p-3 border border-edge rounded hover:border-brand/30 hover:shadow-sm transition-all", [
  // Priority indicator
  Display(task.priority, style: Condition(
    task.priority == "urgent", then: dot(red),
    task.priority == "high", then: dot(orange),
    task.priority == "medium", then: dot(yellow),
    task.priority == "low", then: dot(gray),
    else: dot(transparent)
  )),

  // Content
  Layout(stack, flex: 1, [
    // Title + state
    Layout(row, gap: xs, [
      Display(task.state, style: Condition(
        task.state == "open", then: circle(empty),
        task.state == "active", then: circle(half, animated),
        task.state == "review", then: circle(three_quarter),
        task.state == "done", then: circle(filled, green),
        task.state == "canceled", then: circle(strike, gray)
      )),
      Display(task.title, style: Condition(
        task.state == "done", then: strikethrough,
        task.state == "canceled", then: strikethrough_muted,
        else: normal
      )),
      if task.pinned { Display("📌", style: icon_sm) }
    ]),

    // Meta row
    Layout(row, gap: xs, class: "text-xs text-warm-muted mt-1", [
      if task.assignee_id != "" {
        Avatar(task.assignee, size: xs)
      },
      if task.due_date {
        Display(task.due_date, style: Condition(
          task.overdue, then: badge(red, "overdue"),
          task.due_soon, then: badge(orange, Recency(task.due_date)),
          else: Recency(task.due_date)
        ))
      },
      if task.child_count > 0 {
        Display(task.child_done + "/" + task.child_count, style: caption)
      },
      if task.blocker_count > 0 {
        Display("blocked", style: badge(red))
      },
      if task.tags.length > 0 {
        Loop(task.tags, each: Display(tag, style: dot_colored))
      }
    ])
  ]),

  // Hover actions
  Selection(mode: hover, actions: [
    Action(icon: "check", command: Command(complete, task),
      condition: task.state != "done",
      announce: Announce(label: "Complete task")),
    Action(icon: "status", command: Navigation(popover: StatusPicker(task)),
      announce: Announce(label: "Change status")),
    Action(icon: "more", command: Navigation(popover: TaskMenu(task)))
  ]),

  // Navigate to detail
  Navigation(route: task.detail_url),
  Announce(label: task.priority + " priority: " + task.title + ", " + task.state)
])
```

### TaskDetail

Full task view with properties, discussion, subtasks, dependencies, audit trail.

```
TaskDetail(task, context) = Layout(split, ratio: [2, 1], [
  // Main: content + discussion
  Layout(stack, [
    // Breadcrumb
    Layout(row, gap: xs, [
      Navigation(route: /board, label: task.space.name),
      if task.parent_id {
        Display("›"),
        Navigation(route: task.parent.detail_url, label: task.parent.title)
      },
      Display("›"),
      Display(task.title, style: muted)
    ]),

    // Title (editable inline)
    Input(task.title, type: text, style: display,
      command: Command(extend, task, { title: input.value }),
      announce: Announce(label: "Task title, editable")),

    // Description (editable)
    Input(task.body, type: rich_text, style: prose,
      placeholder: "Add a description...",
      command: Command(extend, task, { body: input.value })),

    // Subtasks
    if task.child_count > 0 || context.can_decompose {
      Layout(stack, [
        Layout(row, justify: space-between, [
          Display("Subtasks (" + task.child_done + "/" + task.child_count + ")", style: bold),
          Action(label: "+ Add subtask", command: Navigation(modal: QuickTaskForm(task)))
        ]),
        List(Query(Node, filter: { parent_id: task.id, kind: "task" }, sort: created_at.asc),
          template: Layout(row, gap: sm, [
            Action(icon: Condition(sub.state == "done", then: "check_filled", else: "check_empty"),
              command: Condition(sub.state == "done",
                then: Command(transition, sub, { state: "open" }),
                else: Command(complete, sub))),
            Display(sub.title, style: Condition(sub.state == "done", then: strikethrough)),
            Avatar(sub.assignee, size: xs),
            if sub.blocker_count > 0 { Display("blocked", style: badge(red)) }
          ])),
        // Inline add
        Form(class: "mt-1", fields: [
          Input(title, type: text, placeholder: "Add subtask...",
            shortcuts: { enter: submit, escape: blur })
        ], command: Command(intend, { parent_id: task.id, title: input.title }))
      ])
    },

    // Dependencies
    if task.dependencies.length > 0 || task.dependents.length > 0 {
      Layout(stack, [
        Display("Dependencies", style: bold),
        if task.dependencies.length > 0 {
          Layout(stack, [
            Display("Blocked by:", style: caption),
            Loop(task.dependencies, each: dep ->
              Layout(row, gap: xs, [
                Display(dep.state, style: status_dot),
                Display(dep.title),
                Action(icon: "x", command: Command(remove_dep, task, dep),
                  announce: Announce(label: "Remove dependency"))
              ]))
          ])
        },
        if task.dependents.length > 0 {
          Layout(stack, [
            Display("Blocking:", style: caption),
            Loop(task.dependents, each: dep ->
              Layout(row, gap: xs, [
                Display(dep.state, style: status_dot),
                Display(dep.title)
              ]))
          ])
        },
        Action(label: "+ Add dependency", command: Navigation(modal: DepPicker(task)))
      ])
    },

    // Discussion (same as Chat messages — comments are nodes)
    Layout(stack, class: "border-t border-edge mt-6 pt-4", [
      Display("Discussion (" + task.comment_count + ")", style: bold),
      List(Query(Node, filter: { parent_id: task.id, kind: "comment" }, sort: created_at.asc),
        template: MessageBubble(msg, context),
        subscribe: Subscribe(on_change: append),
        grouping: GroupBy(author_id, window: 5m),
        empty: Empty(message: "No discussion yet.")),
      @ComposeBar(task, { placeholder: "Comment...", attachments: false })
    ])
  ]),

  // Sidebar: properties + audit
  Layout(stack, class: "border-l border-edge pl-4", [
    // Properties
    Layout(stack, gap: md, [
      // Status
      Layout(row, justify: space-between, [
        Display("Status", style: label),
        Action(label: Display(task.state, style: badge),
          command: Navigation(popover: StatusPicker(task)))
      ]),
      // Priority
      Layout(row, justify: space-between, [
        Display("Priority", style: label),
        Action(label: Display(task.priority, style: badge),
          command: Navigation(popover: PriorityPicker(task)))
      ]),
      // Assignee
      Layout(row, justify: space-between, [
        Display("Assignee", style: label),
        Action(label: Condition(
            task.assignee_id != "", then: Layout(row, gap: xs, [Avatar(task.assignee, size: xs), Display(task.assignee.name)]),
            else: Display("Unassigned", style: muted)),
          command: Navigation(popover: AssigneePicker(task)))
      ]),
      // Due date
      Layout(row, justify: space-between, [
        Display("Due", style: label),
        Action(label: Condition(
            task.due_date, then: Display(task.due_date, format: "MMM D"),
            else: Display("No due date", style: muted)),
          command: Navigation(popover: DatePicker(task)))
      ]),
      // Tags
      Layout(row, justify: space-between, [
        Display("Tags", style: label),
        Layout(row, wrap: true, gap: xs, [
          Loop(task.tags, each: Display(tag, style: badge)),
          Action(label: "+", command: Navigation(popover: TagPicker(task)))
        ])
      ]),
      // Scope (if assigned to agent)
      if task.assignee_kind == "agent" && task.scope {
        Layout(stack, [
          Display("Agent Scope", style: label),
          Loop(task.scope.capabilities, each: Display(cap, style: badge(violet))),
          Action(label: "Edit scope", command: Navigation(modal: ScopeEditor(task)))
        ])
      }
    ]),

    // Activity log
    Layout(stack, class: "mt-6 pt-4 border-t border-edge", [
      Display("Activity", style: bold),
      List(Query(Op, filter: { node_id: task.id }, sort: created_at.desc, limit: 20),
        template: Layout(row, gap: xs, class: "text-xs py-1", [
          Avatar(op.actor, size: xs),
          Display(op.actor.name, style: bold_xs),
          Display(op_description(op), style: muted),
          Recency(op.created_at)
        ]),
        empty: Empty(message: "No activity yet."))
    ]),

    // Review panel (visible when state == review)
    if task.state == "review" {
      Layout(stack, class: "mt-6 pt-4 border-t border-edge", [
        Display("Review", style: bold),
        Layout(row, gap: sm, [
          Action(label: "Approve", style: success,
            command: Command(review, task, { verdict: "approve" })),
          Action(label: "Request changes", style: warning,
            command: Navigation(modal: Form(fields: [
              Input(feedback, type: rich_text, required: true, placeholder: "What needs to change?")
            ], command: Command(review, task, { verdict: "revise", feedback: input.feedback })))),
          Action(label: "Reject", style: destructive,
            command: Navigation(modal: Form(fields: [
              Input(reason, type: rich_text, required: true, placeholder: "Why is this rejected?")
            ], command: Sequence([
              Confirmation(message: "Reject this task?"),
              Command(review, task, { verdict: "reject", reason: input.reason })
            ]))))
        ])
      ])
    },

    // Endorse (visible when state == done)
    if task.state == "done" {
      Layout(row, gap: sm, class: "mt-4", [
        Action(label: "Endorse this work",
          icon: "endorse",
          style: Condition(task.endorsed_by(context.current_user), then: "active"),
          command: Command(endorse, task)),
        if task.endorsement_count > 0 {
          Display(task.endorsement_count + " endorsed", style: badge(brand))
        }
      ])
    }
  ])
])

// Helper: describe an op in human-readable text
func op_description(op) = Condition(
  op.op == "intend", then: "created this task",
  op.op == "assign", then: "assigned to " + op.payload.assignee,
  op.op == "claim", then: "claimed this task",
  op.op == "complete", then: "completed this task",
  op.op == "prioritize", then: "set priority to " + op.payload.priority,
  op.op == "block", then: "marked as blocked: " + op.payload.reason,
  op.op == "unblock", then: "unblocked this task",
  op.op == "review", then: op.payload.verdict + " review",
  op.op == "respond", then: "commented",
  op.op == "react", then: "reacted " + op.payload.emoji,
  op.op == "endorse", then: "endorsed this work",
  else: op.op
)
```

### StatusPicker

Inline status transition with state machine awareness.

```
StatusPicker(task) = Layout(stack, [
  Loop(valid_transitions(task.state), each: state ->
    Action(label: Layout(row, gap: xs, [
      Display(state, style: status_dot),
      Display(state, style: capitalize)
    ]), command: Sequence([
      Command(transition, task, { state: state }),
      if state == "done" { Sound(type: notification, tone: complete, volume: 0.4) }
    ])))
])

func valid_transitions(state) = Condition(
  state == "open", then: ["active", "canceled"],
  state == "active", then: ["review", "open", "canceled"],
  state == "review", then: ["done", "active"],
  state == "done", then: ["open"],
  state == "canceled", then: ["open"]
)
```

---

## View 1: Board

```
BoardView(space, context) = View(name: Board,
  layout: Layout(stack),

  // Header
  Layout(row, justify: space-between, class: "p-4 border-b border-edge", [
    Display("Board", style: heading),
    Layout(row, gap: sm, [
      Input(search, type: text, placeholder: "Search tasks...",
        command: Search(scope: [Node], filter: { kind: "task", space_id: space.id })),
      Action(label: "Filter", command: Toggle(filter_panel)),
      Action(label: "+ Task", command: Navigation(modal: CreateTaskForm(space)),
        shortcuts: { "C": trigger },
        announce: Announce(label: "Create new task"))
    ])
  ]),

  // Filter panel (collapsible)
  if filter_panel.visible {
    Layout(row, gap: sm, class: "px-4 py-2 border-b border-edge bg-surface", [
      Action(label: "Assignee", command: Navigation(popover: AssigneeFilter)),
      Action(label: "Priority", command: Navigation(popover: PriorityFilter)),
      Action(label: "Tags", command: Navigation(popover: TagFilter)),
      Action(label: "Due", command: Navigation(popover: DueFilter)),
      if active_filters.length > 0 {
        Action(label: "Clear all", command: Command(clear_filters))
      }
    ])
  },

  // Columns
  @ModeView("board",
    Layout(row, gap: md, class: "p-4 overflow-x-auto flex-1", [
      Loop(["open", "active", "review", "done"], each: state ->
        Layout(stack, class: "min-w-[280px] flex-shrink-0", [
          // Column header
          Layout(row, justify: space-between, class: "mb-3", [
            Layout(row, gap: xs, [
              Display(state, style: status_dot),
              Display(state, style: uppercase_sm),
              Display(
                Transform(Query(Node, filter: { space_id: space.id, kind: "task", state: state }), count),
                style: badge(muted))
            ]),
            Action(label: "+", command: Navigation(modal: CreateTaskForm(space, { state: state })))
          ]),
          // Cards
          List(Query(Node,
              filter: { space_id: space.id, kind: "task", state: state, ...active_filters },
              sort: Condition(state == "done", then: updated_at.desc, else: priority_order)),
            template: Drag(source: TaskCard(task, context),
              target: StatusColumn,
              on_drop: Command(transition, task, { state: target.state }),
              feedback: Display(ghost_card)),
            subscribe: Subscribe(on_change: refresh),
            empty: Empty(message: Condition(
              state == "open", then: "No tasks yet",
              state == "active", then: "Nothing in progress",
              state == "review", then: "Nothing to review",
              state == "done", then: "Nothing completed yet"
            )))
        ]))
    ]),
    Query(Node, filter: { space_id: space.id, kind: "task" })),

  // Keyboard shortcuts
  Focus(shortcuts: {
    "C": Navigation(modal: CreateTaskForm(space)),
    "J": Focus(next: TaskCard),
    "K": Focus(prev: TaskCard),
    "Enter": Navigation(route: focused_task.detail_url),
    "1": Command(prioritize, focused_task, { priority: "urgent" }),
    "2": Command(prioritize, focused_task, { priority: "high" }),
    "3": Command(prioritize, focused_task, { priority: "medium" }),
    "4": Command(prioritize, focused_task, { priority: "low" }),
    "X": Selection(toggle: focused_task)
  })
)

// Create task form
CreateTaskForm(space, defaults) = Form(
  command: Command(intend, Entity(Node, { kind: "task", space_id: space.id })),
  fields: [
    Input(title, type: text, required: true, placeholder: "Task title",
      autofocus: true),
    Input(description, type: rich_text, placeholder: "Description (optional)"),
    Layout(row, gap: sm, [
      Input(priority, type: select, options: ["urgent", "high", "medium", "low"],
        default: defaults.priority || "medium"),
      Input(assignee, type: entity_picker, scope: Query(User, filter: { space: space.id }),
        placeholder: "Assignee"),
      Input(due_date, type: datepicker, placeholder: "Due date")
    ])
  ],
  submit: Action(label: "Create", shortcuts: { "Cmd+Enter": trigger })
)
```

---

## View 2: Dashboard (My Work)

```
DashboardView(context) = View(name: Dashboard,
  layout: Layout(stack, class: "max-w-3xl mx-auto p-6"),

  Display("My Work", style: heading),

  // Tab bar
  Layout(row, gap: sm, class: "border-b border-edge mb-4", [
    Action(label: "Assigned", style: if tab == "assigned" then "active"),
    Action(label: "Created", style: if tab == "created" then "active"),
    Action(label: "Reviewing", style: if tab == "reviewing" then "active"),
    Action(label: "Watching", style: if tab == "watching" then "active")
  ]),

  // State filter pills
  Layout(row, gap: xs, class: "mb-4", [
    Action(label: "All", style: if state_filter == "all" then "active"),
    Action(label: "Active", style: if state_filter == "active" then "active"),
    Action(label: "Overdue", style: if state_filter == "overdue" then "active"),
    Action(label: "Blocked", style: if state_filter == "blocked" then "active")
  ]),

  @ModeView("dashboard",
    Condition(
      tab == "assigned", then:
        List(Query(Node,
            filter: { kind: "task", assignee_id: context.current_user.id, state: state_filter },
            sort: [priority_order, due_date.asc]),
          template: Layout(row, gap: sm, [
            @TaskCard(task, context),
            // Space badge (cross-space view)
            Display(task.space.name, style: badge(muted))
          ]),
          grouping: GroupBy(priority),
          empty: Empty(message: "Nothing assigned to you.",
            action: Action(label: "Browse available tasks", command: Navigation(route: /market)))),

      tab == "created", then:
        List(Query(Node,
            filter: { kind: "task", author_id: context.current_user.id, state: state_filter },
            sort: updated_at.desc),
          template: @TaskCard(task, context),
          grouping: GroupBy(state)),

      tab == "reviewing", then:
        List(Query(Node,
            filter: { kind: "task", state: "review",
              // Tasks where current user is the reviewer (author or delegated)
              Condition(task.author_id == context.current_user.id ||
                Authorize(context.current_user, review, task)) },
            sort: created_at.asc),
          template: Layout(stack, [
            @TaskCard(task, context),
            Layout(row, gap: sm, class: "ml-6 mt-1", [
              Action(label: "Approve", style: success_sm, command: Command(review, task, { verdict: "approve" })),
              Action(label: "Revise", style: warning_sm, command: Navigation(modal: ReviseForm(task))),
            ])
          ]),
          empty: Empty(message: "Nothing to review.")),

      tab == "watching", then:
        List(Query(Node,
            filter: { kind: "task", subscribers: contains(context.current_user.id), state: state_filter },
            sort: updated_at.desc),
          template: @TaskCard(task, context))
    ),
    Query(Node, filter: { kind: "task", assignee_id: context.current_user.id }))
)
```

---

## View 3: Triage

```
TriageView(space, context) = View(name: Triage,
  layout: Layout(stack, class: "max-w-2xl mx-auto p-6"),

  // Header with count
  Layout(row, justify: space-between, [
    Layout(row, gap: sm, [
      Display("Triage", style: heading),
      Display(triage_count, style: badge)
    ]),
    Layout(row, gap: xs, [
      Display("Keyboard: 1=Accept 2=Decline 3=Merge H=Snooze", style: caption)
    ])
  ]),

  @ModeView("triage",
    List(Query(Node,
        filter: { kind: "task", space_id: space.id, state: "open", assignee_id: "",
          // Triage = unassigned, unprioritized, or from external source
          Condition(task.priority == "none" || task.source == "external") },
        sort: created_at.asc),
      template: Layout(stack, class: "p-4 border border-edge rounded mb-3", [
        // Task content
        Layout(row, gap: sm, [
          Display(task.title, style: bold),
          Recency(task.created_at),
          if task.source { Display(task.source, style: badge(muted)) }
        ]),
        if task.body {
          Display(task.body, format: markdown, truncate: 200)
        },
        // Similar tasks (dedup assistance)
        if task.similar_tasks.length > 0 {
          Layout(stack, class: "bg-surface rounded p-2 mt-2", [
            Display("Similar tasks:", style: caption),
            Loop(task.similar_tasks, each: s ->
              Layout(row, gap: xs, [
                Display(s.state, style: status_dot),
                Display(s.title, style: muted),
                Action(label: "Merge into this", command: Command(merge, { source: task, target: s }))
              ]))
          ])
        },
        // Actions
        Layout(row, gap: sm, class: "mt-3", [
          Action(label: "Accept", style: success,
            command: Navigation(popover: AcceptForm(task)),
            shortcuts: { "1": trigger }),
          Action(label: "Decline", style: destructive,
            command: Navigation(modal: Form(fields: [
              Input(reason, type: text, required: true, placeholder: "Why decline?")
            ], command: Sequence([
              Command(transition, task, { state: "canceled" }),
              Command(respond, task, { body: "Declined: " + input.reason })
            ]))),
            shortcuts: { "2": trigger }),
          Action(label: "Merge", style: muted,
            command: Navigation(modal: MergePicker(task)),
            shortcuts: { "3": trigger }),
          Action(label: "Snooze",
            command: Navigation(popover: SnoozePicker(task)),
            shortcuts: { "H": trigger })
        ])
      ]),
      empty: Empty(message: "Triage inbox zero.",
        illustration: "inbox_zero")),
    Query(Node, filter: { kind: "task", space_id: space.id, state: "open", assignee_id: "" })),

  // J/K navigation
  Focus(shortcuts: {
    "J": Focus(next: triage_card),
    "K": Focus(prev: triage_card)
  })
)

AcceptForm(task) = Layout(stack, gap: sm, [
  Layout(row, gap: sm, [
    Input(priority, type: select, options: ["urgent", "high", "medium", "low"], default: "medium"),
    Input(assignee, type: entity_picker, scope: Query(User, filter: { space: task.space_id }),
      placeholder: "Assign to...")
  ]),
  Action(label: "Accept", command: Sequence([
    Command(prioritize, task, { priority: input.priority }),
    if input.assignee { Command(assign, task, { assignee_id: input.assignee }) },
    Feedback(type: success, message: "Accepted: " + task.title)
  ]))
])
```

---

## View 4: Timeline (future)

```
TimelineView(space, context) = View(name: Timeline,
  layout: Layout(stack),

  Layout(row, justify: space-between, class: "p-4 border-b border-edge", [
    Display("Timeline", style: heading),
    Layout(row, gap: sm, [
      Action(label: "Week", command: set_scale("week")),
      Action(label: "Month", command: set_scale("month")),
      Action(label: "Quarter", command: set_scale("quarter"))
    ])
  ]),

  @ModeView("timeline",
    Layout(stack, [
      // Time axis header
      Layout(row, class: "border-b border-edge", [
        Display("Task", class: "w-64"),
        // Time columns (generated from scale)
        Loop(time_columns, each: col ->
          Display(col.label, style: Condition(col.is_today, then: bold_brand, else: caption)))
      ]),
      // Task rows
      List(Query(Node,
          filter: { kind: "task", space_id: space.id, due_date: not_null },
          sort: due_date.asc),
        template: Layout(row, [
          // Task name column
          Layout(row, gap: xs, class: "w-64 truncate", [
            Display(task.state, style: status_dot),
            Display(task.title),
            Avatar(task.assignee, size: xs)
          ]),
          // Gantt bar
          Display(task, style: gantt_bar(
            start: task.created_at,
            end: task.due_date,
            color: Condition(
              task.overdue, then: red,
              task.state == "done", then: green,
              task.state == "active", then: brand,
              else: gray),
            drag: Drag(on_drop: Command(extend, task, { due_date: drop.date }))
          ))
        ])),
      // Dependency lines
      Loop(Query(NodeDep, filter: { space_id: space.id }),
        template: Display(dependency_line(from: dep.node, to: dep.depends_on),
          style: Condition(dep.violated, then: line(red), else: line(blue))))
    ]),
    Query(Node, filter: { kind: "task", space_id: space.id, due_date: not_null }))
)
```

---

## Grammar Operation Coverage

| Work Op | Board | Dashboard | Triage | Timeline | Detail |
|---------|-------|-----------|--------|----------|--------|
| **Intend** | Create form | — | — | — | — |
| **Decompose** | — | — | — | — | Subtask list + inline add |
| **Assign** | — | — | Accept form | — | Sidebar picker |
| **Claim** | Drag to Active | — | — | — | — |
| **Prioritize** | Keyboard 1-4 | — | Accept form | — | Sidebar picker |
| **Block** | — | — | — | Dependency lines | Dependency section |
| **Unblock** | — | — | — | — | Remove dependency |
| **Progress** | — | — | — | — | Comment (respond) |
| **Complete** | Drag to Done, hover ✓ | — | — | — | Review: Approve |
| **Handoff** | — | — | — | — | Reassign (future) |
| **Scope** | — | — | — | — | Agent scope editor |
| **Review** | — | Approve/Revise inline | — | — | Full review panel |

**12/12 Work operations exercised across views.**

---

## Convergence Analysis

### Pass 1: Need Row

**Audit:**
- Board ✓ (columns, drag, create, filter, keyboard)
- Dashboard ✓ (tabs, filters, cross-space, inline review)
- Triage ✓ (accept/decline/merge/snooze, keyboard, similar detection)
- Timeline ✓ (time axis, gantt bars, dependencies, drag to reschedule)
- Task Detail ✓ (properties, subtasks, dependencies, discussion, activity, review, endorse, scope)
- Shared components ✓ (TaskCard, StatusPicker)
- Missing: **Bulk operations** — select multiple tasks, batch status/assign/priority
- Missing: **Recurring task UI** — how does a recurring task look different?

**Cover:**
- No description of **Board column WIP limits**
- No **Sprint/Cycle view** — acknowledged: cycles are a view filter, not a separate view
- **Estimation** — not described (deferred to tags, per product spec)

**Blind:**
- The Board composition is optimized for small-medium projects (~50-200 tasks). At 1000+ tasks, the column list needs virtualization. Acknowledged as implementation concern (modifier on List), not composition gap.

### Corrections

**Bulk operations:** Add to Board via Selection primitive:
```
// Board supports multi-select via X key
Selection(scope: Board.TaskCards, mode: multi,
  actions: [
    Action(label: "Set status", command: Loop(selected, each: Command(transition, task, { state: input }))),
    Action(label: "Assign", command: Loop(selected, each: Command(assign, task, { assignee_id: input }))),
    Action(label: "Set priority", command: Loop(selected, each: Command(prioritize, task, { priority: input })))
  ],
  announce: Announce(label: selected.length + " tasks selected"))
```

**Recurring indicator:** TaskCard shows a ♻️ icon when task has recurring tag. On Complete, system auto-creates next instance.

### Pass 2: Fixpoint Check

All views have state handling (via ModeView). All Work operations mapped. Bulk ops added. Recurring indicator added. No new gaps found.

**Converged at pass 2.**
