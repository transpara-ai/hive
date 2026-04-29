package telemetry

import (
	"encoding/json"
	"time"
)

// AgentSnapshot is a point-in-time capture of a single agent's state.
type AgentSnapshot struct {
	ID            int64      `json:"id"`
	RecordedAt    time.Time  `json:"recorded_at"`
	AgentRole     string     `json:"agent_role"`
	ActorID       string     `json:"actor_id"`
	State         string     `json:"state"`
	Model         string     `json:"model"`
	Iteration     int        `json:"iteration"`
	MaxIterations int        `json:"max_iterations"`
	TokensUsed    int64      `json:"tokens_used"`
	CostUSD       float64    `json:"cost_usd"`
	TrustScore    *float64   `json:"trust_score"`
	LastEventType string     `json:"last_event_type"`
	LastEventAt   *time.Time `json:"last_event_at"`
	LastMessage   string     `json:"last_message"`
	Errors        int        `json:"errors"`
}

// HiveSnapshot is a point-in-time capture of hive-level health.
type HiveSnapshot struct {
	ID           int64     `json:"id"`
	RecordedAt   time.Time `json:"recorded_at"`
	ActiveAgents int       `json:"active_agents"`
	TotalActors  int       `json:"total_actors"`
	ChainLength  int64     `json:"chain_length"`
	ChainOK      bool      `json:"chain_ok"`
	EventRate    *float64  `json:"event_rate"`
	DailyCost    *float64  `json:"daily_cost"`
	DailyCap     *float64  `json:"daily_cap"`
	Severity     string    `json:"severity"`
}

// Phase tracks expansion phase status.
type Phase struct {
	Phase        int        `json:"phase"`
	Label        string     `json:"label"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	Notes        string     `json:"notes"`
	ExitCriteria *string    `json:"exit_criteria"`
}

// EventStreamEntry is a single event in the live event stream.
type EventStreamEntry struct {
	ID         int64           `json:"id"`
	RecordedAt time.Time       `json:"recorded_at"`
	EventType  string          `json:"event_type"`
	ActorRole  string          `json:"actor_role"`
	Summary    string          `json:"summary"`
	RawContent json.RawMessage `json:"raw_content"`
}

// PipelinePhase is one completed pipeline phase with dashboard-friendly
// workflow metadata.
type PipelinePhase struct {
	ID            int64     `json:"id"`
	RecordedAt    time.Time `json:"recorded_at"`
	CycleID       string    `json:"cycle_id"`
	Phase         string    `json:"phase"`
	WorkflowStage string    `json:"workflow_stage"`
	Outcome       string    `json:"outcome"`
	Repo          string    `json:"repo,omitempty"`
	TaskID        string    `json:"task_id,omitempty"`
	TaskTitle     string    `json:"task_title,omitempty"`
	DurationSecs  float64   `json:"duration_secs,omitempty"`
	CostUSD       float64   `json:"cost_usd"`
	InputTokens   int       `json:"input_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	BoardOpen     int       `json:"board_open"`
	ReviseCount   int       `json:"revise_count"`
	Summary       string    `json:"summary,omitempty"`
	Error         string    `json:"error,omitempty"`
	InputRef      string    `json:"input_ref,omitempty"`
	OutputRef     string    `json:"output_ref,omitempty"`
}
