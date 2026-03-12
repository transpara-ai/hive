package pipeline

import (
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// Pipeline event types — operational output as graph events.
var (
	EventTypeRunStarted     = types.MustEventType("pipeline.run.started")
	EventTypeRunCompleted   = types.MustEventType("pipeline.run.completed")
	EventTypePhaseStarted   = types.MustEventType("pipeline.phase.started")
	EventTypePhaseCompleted = types.MustEventType("pipeline.phase.completed")
	EventTypeAgentSpawned   = types.MustEventType("pipeline.agent.spawned")
	EventTypeProgress       = types.MustEventType("pipeline.progress")
	EventTypeOutput         = types.MustEventType("pipeline.output")
	EventTypeWarning        = types.MustEventType("pipeline.warning")
	EventTypeTelemetry      = types.MustEventType("pipeline.telemetry")
)

// allPipelineEventTypes returns all pipeline event types for registration.
func allPipelineEventTypes() []types.EventType {
	return []types.EventType{
		EventTypeRunStarted, EventTypeRunCompleted,
		EventTypePhaseStarted, EventTypePhaseCompleted,
		EventTypeAgentSpawned, EventTypeProgress,
		EventTypeOutput, EventTypeWarning, EventTypeTelemetry,
	}
}

// pipelineContent is embedded in all pipeline content types. Pipeline events
// use no-op Accept (same pattern as agentContent in eventgraph) — they bypass
// the base EventContentVisitor since they're hive-specific.
type pipelineContent struct{}

func (pipelineContent) Accept(event.EventContentVisitor) {}

// --- Content structs ---

// RunStartedContent is emitted when a pipeline run begins.
type RunStartedContent struct {
	pipelineContent
	Mode        string `json:"Mode"`
	Description string `json:"Description"`
}

func (c RunStartedContent) EventTypeName() string { return "pipeline.run.started" }

// RunCompletedContent is emitted when a pipeline run ends.
type RunCompletedContent struct {
	pipelineContent
	Mode         string  `json:"Mode"`
	EventCount   int     `json:"EventCount"`
	AgentCount   int     `json:"AgentCount"`
	DurationMs   int64   `json:"DurationMs"`
	PRURL        string  `json:"PRURL,omitempty"`
	Merged       bool    `json:"Merged,omitempty"`
	FailedPhase  string  `json:"FailedPhase,omitempty"`
	FailReason   string  `json:"FailReason,omitempty"`
	TotalCostUSD float64 `json:"TotalCostUSD"`
}

func (c RunCompletedContent) EventTypeName() string { return "pipeline.run.completed" }

// PhaseStartedContent is emitted when a pipeline phase begins.
type PhaseStartedContent struct {
	pipelineContent
	Phase string `json:"Phase"`
	Round int    `json:"Round,omitempty"`
}

func (c PhaseStartedContent) EventTypeName() string { return "pipeline.phase.started" }

// PhaseCompletedContent is emitted when a pipeline phase ends.
type PhaseCompletedContent struct {
	pipelineContent
	Phase      string `json:"Phase"`
	DurationMs int64  `json:"DurationMs"`
	Round      int    `json:"Round,omitempty"`
}

func (c PhaseCompletedContent) EventTypeName() string { return "pipeline.phase.completed" }

// AgentSpawnedContent is emitted when a new agent is bootstrapped.
type AgentSpawnedContent struct {
	pipelineContent
	Role    string `json:"Role"`
	ActorID string `json:"ActorID"`
	Model   string `json:"Model"`
}

func (c AgentSpawnedContent) EventTypeName() string { return "pipeline.agent.spawned" }

// ProgressContent is emitted for general pipeline progress messages.
type ProgressContent struct {
	pipelineContent
	Phase   string            `json:"Phase,omitempty"`
	Message string            `json:"Message"`
	Data    map[string]string `json:"Data,omitempty"`
}

func (c ProgressContent) EventTypeName() string { return "pipeline.progress" }

// OutputContent is emitted for LLM response display (CTO analysis, reviews, etc).
type OutputContent struct {
	pipelineContent
	Role    string `json:"Role"`
	Kind    string `json:"Kind"`
	Content string `json:"Content"`
}

func (c OutputContent) EventTypeName() string { return "pipeline.output" }

// WarningContent is emitted for non-fatal pipeline warnings.
type WarningContent struct {
	pipelineContent
	Phase   string `json:"Phase,omitempty"`
	Message string `json:"Message"`
}

func (c WarningContent) EventTypeName() string { return "pipeline.warning" }

// TelemetryContent is emitted for per-agent token usage.
type TelemetryContent struct {
	pipelineContent
	Role             string  `json:"Role"`
	Model            string  `json:"Model"`
	InputTokens      int     `json:"InputTokens"`
	OutputTokens     int     `json:"OutputTokens"`
	TotalTokens      int     `json:"TotalTokens"`
	CacheReadTokens  int     `json:"CacheReadTokens"`
	CacheWriteTokens int     `json:"CacheWriteTokens"`
	CostUSD          float64 `json:"CostUSD"`
}

func (c TelemetryContent) EventTypeName() string { return "pipeline.telemetry" }

// RegisterEventTypes registers pipeline content unmarshalers for Postgres
// deserialization. Call this before querying pipeline events from the store.
func RegisterEventTypes() {
	event.RegisterContentUnmarshaler("pipeline.run.started", event.Unmarshal[RunStartedContent])
	event.RegisterContentUnmarshaler("pipeline.run.completed", event.Unmarshal[RunCompletedContent])
	event.RegisterContentUnmarshaler("pipeline.phase.started", event.Unmarshal[PhaseStartedContent])
	event.RegisterContentUnmarshaler("pipeline.phase.completed", event.Unmarshal[PhaseCompletedContent])
	event.RegisterContentUnmarshaler("pipeline.agent.spawned", event.Unmarshal[AgentSpawnedContent])
	event.RegisterContentUnmarshaler("pipeline.progress", event.Unmarshal[ProgressContent])
	event.RegisterContentUnmarshaler("pipeline.output", event.Unmarshal[OutputContent])
	event.RegisterContentUnmarshaler("pipeline.warning", event.Unmarshal[WarningContent])
	event.RegisterContentUnmarshaler("pipeline.telemetry", event.Unmarshal[TelemetryContent])
}

// registerPipelineEvents registers all pipeline event types with the given
// registry and registers content unmarshalers for Postgres deserialization.
func registerPipelineEvents(registry *event.EventTypeRegistry) {
	for _, et := range allPipelineEventTypes() {
		registry.Register(et, nil)
	}
	RegisterEventTypes()
}
