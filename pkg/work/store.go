package work

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// TaskStatus represents the current lifecycle state of a task.
type TaskStatus string

const (
	// StatusPending means the task has been created but not yet assigned.
	StatusPending TaskStatus = "pending"
	// StatusAssigned means the task has been assigned to an actor but not yet completed.
	StatusAssigned TaskStatus = "assigned"
	// StatusCompleted means the task has been marked complete.
	StatusCompleted TaskStatus = "completed"
)

// Task represents a work item derived from a work.task.created event.
type Task struct {
	ID          types.EventID
	Title       string
	Description string
	CreatedBy   types.ActorID
	Priority    TaskPriority
}

// TaskSummary extends Task with computed state fields for efficient list views.
// Status, Assignee, and Blocked are populated by ListSummaries using batch store scans.
type TaskSummary struct {
	Task
	Status   TaskStatus
	Assignee types.ActorID // zero value if unassigned
	Blocked  bool
}

// TaskStore creates and queries tasks as auditable events on the shared graph.
type TaskStore struct {
	store   store.Store
	factory *event.EventFactory
	signer  event.Signer
}

// NewTaskStore creates a new TaskStore backed by the given event store.
func NewTaskStore(s store.Store, factory *event.EventFactory, signer event.Signer) *TaskStore {
	return &TaskStore{store: s, factory: factory, signer: signer}
}

// Create records a work.task.created event on the graph and returns the task.
// The caller must supply at least one cause (typically the current chain head).
// An optional priority may be passed as the last argument; defaults to PriorityMedium.
func (ts *TaskStore) Create(
	source types.ActorID,
	title, description string,
	causes []types.EventID,
	convID types.ConversationID,
	priority ...TaskPriority,
) (Task, error) {
	if title == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	p := DefaultPriority
	if len(priority) > 0 && priority[0] != "" {
		p = priority[0]
	}
	content := TaskCreatedContent{
		Title:       title,
		Description: description,
		CreatedBy:   source,
		Priority:    p,
	}
	ev, err := ts.factory.Create(EventTypeTaskCreated, source, content, causes, convID, ts.store, ts.signer)
	if err != nil {
		return Task{}, fmt.Errorf("create task event: %w", err)
	}
	stored, err := ts.store.Append(ev)
	if err != nil {
		return Task{}, fmt.Errorf("append task event: %w", err)
	}
	return Task{
		ID:          stored.ID(),
		Title:       title,
		Description: description,
		CreatedBy:   source,
		Priority:    p,
	}, nil
}

// List returns up to limit work.task.created events as Tasks.
func (ts *TaskStore) List(limit int) ([]Task, error) {
	if limit <= 0 {
		limit = 20
	}
	page, err := ts.store.ByType(EventTypeTaskCreated, limit, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	tasks := make([]Task, 0, len(page.Items()))
	for _, ev := range page.Items() {
		c, ok := ev.Content().(TaskCreatedContent)
		if !ok {
			continue
		}
		p := c.Priority
		if p == "" {
			p = DefaultPriority
		}
		tasks = append(tasks, Task{
			ID:          ev.ID(),
			Title:       c.Title,
			Description: c.Description,
			CreatedBy:   c.CreatedBy,
			Priority:    p,
		})
	}
	return tasks, nil
}

// Assign records a work.task.assigned event on the graph.
// source is the actor performing the assignment (may equal assignee for self-assignment).
func (ts *TaskStore) Assign(
	source types.ActorID,
	taskID types.EventID,
	assignee types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	content := TaskAssignedContent{
		TaskID:     taskID,
		AssignedTo: assignee,
		AssignedBy: source,
	}
	ev, err := ts.factory.Create(EventTypeTaskAssigned, source, content, causes, convID, ts.store, ts.signer)
	if err != nil {
		return fmt.Errorf("create assign event: %w", err)
	}
	if _, err := ts.store.Append(ev); err != nil {
		return fmt.Errorf("append assign event: %w", err)
	}
	return nil
}

// Complete records a work.task.completed event on the graph.
// source is the actor completing the task (typically the assignee).
func (ts *TaskStore) Complete(
	source types.ActorID,
	taskID types.EventID,
	summary string,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	content := TaskCompletedContent{
		TaskID:      taskID,
		CompletedBy: source,
		Summary:     summary,
	}
	ev, err := ts.factory.Create(EventTypeTaskCompleted, source, content, causes, convID, ts.store, ts.signer)
	if err != nil {
		return fmt.Errorf("create complete event: %w", err)
	}
	if _, err := ts.store.Append(ev); err != nil {
		return fmt.Errorf("append complete event: %w", err)
	}
	return nil
}

// GetStatus reconstructs the current status of a task by scanning
// work.task.completed and work.task.assigned events for the given task ID.
// Returns StatusCompleted if a completed event exists, StatusAssigned if an
// assigned event exists, and StatusPending otherwise.
func (ts *TaskStore) GetStatus(taskID types.EventID) (TaskStatus, error) {
	// Check for completed event first.
	completedPage, err := ts.store.ByType(EventTypeTaskCompleted, 1000, types.None[types.Cursor]())
	if err != nil {
		return StatusPending, fmt.Errorf("fetch completed events: %w", err)
	}
	for _, ev := range completedPage.Items() {
		c, ok := ev.Content().(TaskCompletedContent)
		if ok && c.TaskID == taskID {
			return StatusCompleted, nil
		}
	}

	// Check for assigned event.
	assignedPage, err := ts.store.ByType(EventTypeTaskAssigned, 1000, types.None[types.Cursor]())
	if err != nil {
		return StatusPending, fmt.Errorf("fetch assigned events: %w", err)
	}
	for _, ev := range assignedPage.Items() {
		c, ok := ev.Content().(TaskAssignedContent)
		if ok && c.TaskID == taskID {
			return StatusAssigned, nil
		}
	}

	return StatusPending, nil
}

// AddDependency records a work.task.dependency.added event, declaring that taskID
// depends on dependsOnID — taskID is blocked until dependsOnID completes.
func (ts *TaskStore) AddDependency(
	source types.ActorID,
	taskID, dependsOnID types.EventID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	content := TaskDependencyContent{
		TaskID:      taskID,
		DependsOnID: dependsOnID,
		AddedBy:     source,
	}
	ev, err := ts.factory.Create(EventTypeTaskDependencyAdded, source, content, causes, convID, ts.store, ts.signer)
	if err != nil {
		return fmt.Errorf("create dependency event: %w", err)
	}
	if _, err := ts.store.Append(ev); err != nil {
		return fmt.Errorf("append dependency event: %w", err)
	}
	return nil
}

// GetDependencies returns all task IDs that the given taskID depends on.
func (ts *TaskStore) GetDependencies(taskID types.EventID) ([]types.EventID, error) {
	page, err := ts.store.ByType(EventTypeTaskDependencyAdded, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch dependency events: %w", err)
	}
	var deps []types.EventID
	for _, ev := range page.Items() {
		c, ok := ev.Content().(TaskDependencyContent)
		if ok && c.TaskID == taskID {
			deps = append(deps, c.DependsOnID)
		}
	}
	return deps, nil
}

// IsBlocked returns true if taskID has any declared dependency that is not yet completed.
func (ts *TaskStore) IsBlocked(taskID types.EventID) (bool, error) {
	deps, err := ts.GetDependencies(taskID)
	if err != nil {
		return false, err
	}
	if len(deps) == 0 {
		return false, nil
	}

	// Collect all completed task IDs once.
	completedPage, err := ts.store.ByType(EventTypeTaskCompleted, 1000, types.None[types.Cursor]())
	if err != nil {
		return false, fmt.Errorf("fetch completed events: %w", err)
	}
	completedIDs := make(map[types.EventID]bool, len(completedPage.Items()))
	for _, ev := range completedPage.Items() {
		c, ok := ev.Content().(TaskCompletedContent)
		if ok {
			completedIDs[c.TaskID] = true
		}
	}

	for _, depID := range deps {
		if !completedIDs[depID] {
			return true, nil
		}
	}
	return false, nil
}

// ListOpen returns all tasks that do not have a matching work.task.completed event
// and are not blocked by an incomplete dependency.
// It fetches up to 1000 tasks and filters out completed and blocked tasks.
func (ts *TaskStore) ListOpen() ([]Task, error) {
	// Collect all completed task IDs.
	completedPage, err := ts.store.ByType(EventTypeTaskCompleted, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch completed events: %w", err)
	}
	completedIDs := make(map[types.EventID]bool, len(completedPage.Items()))
	for _, ev := range completedPage.Items() {
		c, ok := ev.Content().(TaskCompletedContent)
		if ok {
			completedIDs[c.TaskID] = true
		}
	}

	// Collect all dependency edges: taskID → dependsOnID.
	depPage, err := ts.store.ByType(EventTypeTaskDependencyAdded, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch dependency events: %w", err)
	}
	// blockedBy maps a taskID to the set of its uncompleted dependency IDs.
	blockedBy := make(map[types.EventID][]types.EventID)
	for _, ev := range depPage.Items() {
		c, ok := ev.Content().(TaskDependencyContent)
		if !ok {
			continue
		}
		if !completedIDs[c.DependsOnID] {
			blockedBy[c.TaskID] = append(blockedBy[c.TaskID], c.DependsOnID)
		}
	}

	// List all tasks and filter out completed and blocked ones.
	all, err := ts.List(1000)
	if err != nil {
		return nil, err
	}
	open := make([]Task, 0, len(all))
	for _, t := range all {
		if !completedIDs[t.ID] && len(blockedBy[t.ID]) == 0 {
			open = append(open, t)
		}
	}
	return open, nil
}

// GetByAssignee returns tasks assigned to the given actor.
// It scans work.task.assigned events and joins each to its work.task.created event.
func (ts *TaskStore) GetByAssignee(assignee types.ActorID) ([]Task, error) {
	// Fetch all assigned events; no SQL join available in-memory so filter in code.
	page, err := ts.store.ByType(EventTypeTaskAssigned, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch assigned events: %w", err)
	}
	var tasks []Task
	for _, ev := range page.Items() {
		c, ok := ev.Content().(TaskAssignedContent)
		if !ok || c.AssignedTo != assignee {
			continue
		}
		created, err := ts.store.Get(c.TaskID)
		if err != nil {
			continue // task event missing — skip
		}
		cc, ok := created.Content().(TaskCreatedContent)
		if !ok {
			continue
		}
		cp := cc.Priority
		if cp == "" {
			cp = DefaultPriority
		}
		tasks = append(tasks, Task{
			ID:          created.ID(),
			Title:       cc.Title,
			Description: cc.Description,
			CreatedBy:   cc.CreatedBy,
			Priority:    cp,
		})
	}
	return tasks, nil
}

// SetPriority records a work.task.priority.set event, updating the priority of taskID.
func (ts *TaskStore) SetPriority(
	source types.ActorID,
	taskID types.EventID,
	priority TaskPriority,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	content := TaskPrioritySetContent{
		TaskID:   taskID,
		Priority: priority,
		SetBy:    source,
	}
	ev, err := ts.factory.Create(EventTypeTaskPrioritySet, source, content, causes, convID, ts.store, ts.signer)
	if err != nil {
		return fmt.Errorf("create priority event: %w", err)
	}
	if _, err := ts.store.Append(ev); err != nil {
		return fmt.Errorf("append priority event: %w", err)
	}
	return nil
}

// GetPriority returns the effective priority of a task by scanning
// work.task.priority.set events for the most recent override, falling back
// to the priority recorded in the work.task.created event.
func (ts *TaskStore) GetPriority(taskID types.EventID) (TaskPriority, error) {
	// Scan all priority-set events for this task; events are returned newest-first,
	// so the first match is the most recent override.
	page, err := ts.store.ByType(EventTypeTaskPrioritySet, 1000, types.None[types.Cursor]())
	if err != nil {
		return DefaultPriority, fmt.Errorf("fetch priority events: %w", err)
	}
	for _, ev := range page.Items() {
		c, ok := ev.Content().(TaskPrioritySetContent)
		if ok && c.TaskID == taskID {
			return c.Priority, nil
		}
	}

	// Fall back to the creation event's priority.
	ev, err := ts.store.Get(taskID)
	if err != nil {
		return DefaultPriority, fmt.Errorf("get task event: %w", err)
	}
	c, ok := ev.Content().(TaskCreatedContent)
	if !ok {
		return DefaultPriority, nil
	}
	if c.Priority == "" {
		return DefaultPriority, nil
	}
	return c.Priority, nil
}

// batchStatus enriches a slice of Tasks with computed Status, Assignee, and Blocked
// fields using three batch store scans rather than N per-task queries.
func (ts *TaskStore) batchStatus(tasks []Task) ([]TaskSummary, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	// Scan 1: completed events → completedIDs set.
	completedPage, err := ts.store.ByType(EventTypeTaskCompleted, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch completed events: %w", err)
	}
	completedIDs := make(map[types.EventID]bool, len(completedPage.Items()))
	for _, ev := range completedPage.Items() {
		if c, ok := ev.Content().(TaskCompletedContent); ok {
			completedIDs[c.TaskID] = true
		}
	}

	// Scan 2: assigned events (newest-first) → current assignee per task.
	assignedPage, err := ts.store.ByType(EventTypeTaskAssigned, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch assigned events: %w", err)
	}
	assigneeMap := make(map[types.EventID]types.ActorID, len(assignedPage.Items()))
	for _, ev := range assignedPage.Items() {
		if c, ok := ev.Content().(TaskAssignedContent); ok {
			if _, seen := assigneeMap[c.TaskID]; !seen {
				assigneeMap[c.TaskID] = c.AssignedTo
			}
		}
	}

	// Scan 3: dependency events → blocked set (reuses completedIDs from scan 1).
	depPage, err := ts.store.ByType(EventTypeTaskDependencyAdded, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch dependency events: %w", err)
	}
	blockedMap := make(map[types.EventID]bool)
	for _, ev := range depPage.Items() {
		if c, ok := ev.Content().(TaskDependencyContent); ok {
			if !completedIDs[c.DependsOnID] {
				blockedMap[c.TaskID] = true
			}
		}
	}

	summaries := make([]TaskSummary, 0, len(tasks))
	for _, t := range tasks {
		status := StatusPending
		if completedIDs[t.ID] {
			status = StatusCompleted
		} else if _, assigned := assigneeMap[t.ID]; assigned {
			status = StatusAssigned
		}
		summaries = append(summaries, TaskSummary{
			Task:     t,
			Status:   status,
			Assignee: assigneeMap[t.ID],
			Blocked:  blockedMap[t.ID],
		})
	}
	return summaries, nil
}

// ListSummaries returns up to limit tasks with Status, Assignee, and Blocked
// populated via three batch store scans (rather than N per-task queries).
func (ts *TaskStore) ListSummaries(limit int) ([]TaskSummary, error) {
	tasks, err := ts.List(limit)
	if err != nil {
		return nil, err
	}
	return ts.batchStatus(tasks)
}
