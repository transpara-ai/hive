package hive

import (
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/checkpoint"
	"github.com/lovyou-ai/hive/pkg/membrane"
)

// Hive event types — runtime lifecycle and agent coordination.
var (
	EventTypeRunStarted    = types.MustEventType("hive.run.started")
	EventTypeRunCompleted  = types.MustEventType("hive.run.completed")
	EventTypeAgentSpawned  = types.MustEventType("hive.agent.spawned")
	EventTypeAgentStopped  = types.MustEventType("hive.agent.stopped")
	EventTypeProgress      = types.MustEventType("hive.progress")
)

func allHiveEventTypes() []types.EventType {
	return []types.EventType{
		EventTypeRunStarted, EventTypeRunCompleted,
		EventTypeAgentSpawned, EventTypeAgentStopped,
		EventTypeProgress,
		// Agent loop heartbeat (pkg/checkpoint).
		checkpoint.EventTypeAgentHeartbeat,
		// Site webhook bridge events (dispatch.go).
		EventTypeSiteRespond, EventTypeSiteExpress,
		EventTypeSiteAssert, EventTypeSiteProgress,
	}
}

// hiveContent is embedded in all hive content types. Uses no-op Accept
// (same pattern as pipeline/work content) since these are hive-specific.
type hiveContent struct{}

func (hiveContent) Accept(event.EventContentVisitor) {}

// RunStartedContent is emitted when a hive run begins.
type RunStartedContent struct {
	hiveContent
	Idea     string `json:"Idea,omitempty"`
	RepoPath string `json:"RepoPath,omitempty"`
}

func (c RunStartedContent) EventTypeName() string { return "hive.run.started" }

// RunCompletedContent is emitted when a hive run ends.
type RunCompletedContent struct {
	hiveContent
	AgentCount int     `json:"AgentCount"`
	DurationMs int64   `json:"DurationMs"`
	TotalCost  float64 `json:"TotalCost"`
}

func (c RunCompletedContent) EventTypeName() string { return "hive.run.completed" }

// AgentSpawnedContent is emitted when an agent starts running.
type AgentSpawnedContent struct {
	hiveContent
	Name    string `json:"Name"`
	Role    string `json:"Role"`
	Model   string `json:"Model"`
	ActorID string `json:"ActorID"`
}

func (c AgentSpawnedContent) EventTypeName() string { return "hive.agent.spawned" }

// AgentStoppedContent is emitted when an agent's loop ends.
type AgentStoppedContent struct {
	hiveContent
	Name       string `json:"Name"`
	Role       string `json:"Role"`
	StopReason string `json:"StopReason"`
	Iterations int    `json:"Iterations"`
	Detail     string `json:"Detail,omitempty"`
}

func (c AgentStoppedContent) EventTypeName() string { return "hive.agent.stopped" }

// ProgressContent is emitted for general runtime progress messages.
type ProgressContent struct {
	hiveContent
	Message string `json:"Message"`
}

func (c ProgressContent) EventTypeName() string { return "hive.progress" }

// RegisterEventTypes registers hive content unmarshalers for Postgres
// deserialization. Call before querying hive events from the store.
func RegisterEventTypes() {
	event.RegisterContentUnmarshaler("hive.run.started", event.Unmarshal[RunStartedContent])
	event.RegisterContentUnmarshaler("hive.run.completed", event.Unmarshal[RunCompletedContent])
	event.RegisterContentUnmarshaler("hive.agent.spawned", event.Unmarshal[AgentSpawnedContent])
	event.RegisterContentUnmarshaler("hive.agent.stopped", event.Unmarshal[AgentStoppedContent])
	event.RegisterContentUnmarshaler("hive.progress", event.Unmarshal[ProgressContent])
	event.RegisterContentUnmarshaler("hive.agent.heartbeat", event.Unmarshal[checkpoint.HeartbeatContent])
}

// RegisterWithRegistry registers all hive event types with the given registry
// and registers content unmarshalers for Postgres deserialization.
func RegisterWithRegistry(registry *event.EventTypeRegistry) {
	for _, et := range allHiveEventTypes() {
		registry.Register(et, nil)
	}
	RegisterEventTypes()
	membrane.RegisterWithRegistry(registry)
}
