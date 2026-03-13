package work

import (
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// TaskPriority represents the urgency level of a task.
type TaskPriority string

const (
	// PriorityLow is for tasks that can wait.
	PriorityLow TaskPriority = "low"
	// PriorityMedium is the default priority when none is specified.
	PriorityMedium TaskPriority = "medium"
	// PriorityHigh is for tasks that should be addressed soon.
	PriorityHigh TaskPriority = "high"
	// PriorityCritical is for tasks that must be addressed immediately.
	PriorityCritical TaskPriority = "critical"

	// DefaultPriority is the priority assigned when none is specified at creation.
	DefaultPriority = PriorityMedium
)

// Work Graph event types — Layer 1 of the thirteen-product roadmap.
var (
	EventTypeTaskCreated         = types.MustEventType("work.task.created")
	EventTypeTaskAssigned        = types.MustEventType("work.task.assigned")
	EventTypeTaskCompleted       = types.MustEventType("work.task.completed")
	EventTypeTaskDependencyAdded = types.MustEventType("work.task.dependency.added")
	EventTypeTaskPrioritySet     = types.MustEventType("work.task.priority.set")
	EventTypeTaskComment         = types.MustEventType("work.task.comment")
)

// allWorkEventTypes returns all work event types for registration.
func allWorkEventTypes() []types.EventType {
	return []types.EventType{
		EventTypeTaskCreated, EventTypeTaskAssigned, EventTypeTaskCompleted,
		EventTypeTaskDependencyAdded, EventTypeTaskPrioritySet, EventTypeTaskComment,
	}
}

// workContent is embedded in all work content types. Work events use
// no-op Accept (same pattern as pipeline content) since they are hive-specific.
type workContent struct{}

func (workContent) Accept(event.EventContentVisitor) {}

// --- Content structs ---

// TaskCreatedContent is emitted when a new task is created.
type TaskCreatedContent struct {
	workContent
	Title       string        `json:"Title"`
	Description string        `json:"Description,omitempty"`
	CreatedBy   types.ActorID `json:"CreatedBy"`
	Priority    TaskPriority  `json:"Priority,omitempty"`
}

func (c TaskCreatedContent) EventTypeName() string { return "work.task.created" }

// TaskAssignedContent is emitted when a task is assigned to an actor.
type TaskAssignedContent struct {
	workContent
	TaskID     types.EventID `json:"TaskID"`
	AssignedTo types.ActorID `json:"AssignedTo"`
	AssignedBy types.ActorID `json:"AssignedBy"`
}

func (c TaskAssignedContent) EventTypeName() string { return "work.task.assigned" }

// TaskCompletedContent is emitted when a task is completed.
type TaskCompletedContent struct {
	workContent
	TaskID      types.EventID `json:"TaskID"`
	CompletedBy types.ActorID `json:"CompletedBy"`
	Summary     string        `json:"Summary,omitempty"`
}

func (c TaskCompletedContent) EventTypeName() string { return "work.task.completed" }

// TaskDependencyContent is emitted when a dependency is declared between two tasks.
// It records that TaskID depends on DependsOnID — TaskID cannot start until DependsOnID completes.
type TaskDependencyContent struct {
	workContent
	TaskID      types.EventID `json:"TaskID"`
	DependsOnID types.EventID `json:"DependsOnID"`
	AddedBy     types.ActorID `json:"AddedBy"`
}

func (c TaskDependencyContent) EventTypeName() string { return "work.task.dependency.added" }

// TaskPrioritySetContent is emitted when a task's priority is updated post-creation.
type TaskPrioritySetContent struct {
	workContent
	TaskID   types.EventID `json:"TaskID"`
	Priority TaskPriority  `json:"Priority"`
	SetBy    types.ActorID `json:"SetBy"`
}

func (c TaskPrioritySetContent) EventTypeName() string { return "work.task.priority.set" }

// CommentContent is emitted when a freeform note is added to a task.
type CommentContent struct {
	workContent
	TaskID   types.EventID `json:"TaskID"`
	Body     string        `json:"Body"`
	AuthorID types.ActorID `json:"AuthorID"`
}

func (c CommentContent) EventTypeName() string { return "work.task.comment" }

// RegisterEventTypes registers work content unmarshalers for Postgres
// deserialization. Call this before querying work events from the store.
func RegisterEventTypes() {
	event.RegisterContentUnmarshaler("work.task.created", event.Unmarshal[TaskCreatedContent])
	event.RegisterContentUnmarshaler("work.task.assigned", event.Unmarshal[TaskAssignedContent])
	event.RegisterContentUnmarshaler("work.task.completed", event.Unmarshal[TaskCompletedContent])
	event.RegisterContentUnmarshaler("work.task.dependency.added", event.Unmarshal[TaskDependencyContent])
	event.RegisterContentUnmarshaler("work.task.priority.set", event.Unmarshal[TaskPrioritySetContent])
	event.RegisterContentUnmarshaler("work.task.comment", event.Unmarshal[CommentContent])
}

// RegisterWithRegistry registers all work event types with the given registry
// and registers content unmarshalers for Postgres deserialization.
func RegisterWithRegistry(registry *event.EventTypeRegistry) {
	for _, et := range allWorkEventTypes() {
		registry.Register(et, nil)
	}
	RegisterEventTypes()
}
