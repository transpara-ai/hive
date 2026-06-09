package hive

import (
	"encoding/json"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/hive/pkg/membrane"
)

// Hive event types — runtime lifecycle and agent coordination.
var (
	EventTypeRunStarted              = types.MustEventType("hive.run.started")
	EventTypeRunCompleted            = types.MustEventType("hive.run.completed")
	EventTypeAgentSpawned            = types.MustEventType("hive.agent.spawned")
	EventTypeAgentStopped            = types.MustEventType("hive.agent.stopped")
	EventTypeProgress                = types.MustEventType("hive.progress")
	EventTypeRoleDefinition          = types.MustEventType("hive.role.definition")
	EventTypeAgentIdentityRegistered = types.MustEventType("agent.identity.registered")
	EventTypeSourceIngested          = types.MustEventType("source.ingested")
	EventTypeBriefDerived            = types.MustEventType("brief.derived")
	EventTypeFactoryRunRequested     = types.MustEventType("factory.run.requested")
)

func allHiveEventTypes() []types.EventType {
	eventTypes := []types.EventType{
		EventTypeRunStarted, EventTypeRunCompleted,
		EventTypeAgentSpawned, EventTypeAgentStopped,
		EventTypeProgress, EventTypeRoleDefinition,
		EventTypeAgentIdentityRegistered,
		EventTypeSourceIngested, EventTypeBriefDerived,
		EventTypeFactoryRunRequested,
		// Agent loop heartbeat (pkg/checkpoint).
		checkpoint.EventTypeAgentHeartbeat,
		// Site webhook bridge events (dispatch.go).
		EventTypeSiteRespond, EventTypeSiteExpress,
		EventTypeSiteAssert, EventTypeSiteProgress,
	}
	eventTypes = append(eventTypes, phase3EventTypes()...)
	return eventTypes
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

// AgentIdentityRegisteredContent records the DF-SPEC-0003 identity
// registration projection Hive needs for operator/audit views. It stores only
// public key material and provenance metadata, never private key material.
type AgentIdentityRegisteredContent struct {
	hiveContent
	ActorID          types.ActorID   `json:"ActorID"`
	DisplayName      string          `json:"DisplayName"`
	Role             string          `json:"Role"`
	PublicKey        types.PublicKey `json:"PublicKey"`
	KeyProvenance    string          `json:"KeyProvenance"`
	Environment      string          `json:"Environment"`
	IdentityMode     string          `json:"IdentityMode"`
	ExternalKeyRef   string          `json:"ExternalKeyRef,omitempty"`
	LifecycleStatus  string          `json:"LifecycleStatus"`
	AuthorityScope   string          `json:"AuthorityScope"`
	RegistrationPath string          `json:"RegistrationPath"`
}

func (c AgentIdentityRegisteredContent) EventTypeName() string {
	return "agent.identity.registered"
}

// RunLaunchSource identifies an operator-supplied source that caused a Hive run
// launch request.
type RunLaunchSource struct {
	ID    string `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
	Ref   string `json:"ref"`
	Title string `json:"title,omitempty"`
}

// RunLaunchAuthority records the authority envelope the operator supplied when
// requesting a run launch.
type RunLaunchAuthority struct {
	InitialLevel event.AuthorityLevel `json:"initial_level"`
	Scope        string               `json:"scope,omitempty"`
	PolicyRef    string               `json:"policy_ref,omitempty"`
	Rationale    string               `json:"rationale,omitempty"`
}

// RunLaunchBudget records the bounded launch budget the operator supplied when
// requesting a run launch.
type RunLaunchBudget struct {
	MaxIterations int     `json:"max_iterations"`
	MaxCostUSD    float64 `json:"max_cost_usd"`
}

// SourceIngestedContent records the source payload anchoring an operator run
// launch request.
type SourceIngestedContent struct {
	hiveContent
	RunID      string            `json:"run_id"`
	IntakeID   string            `json:"intake_id"`
	OperatorID string            `json:"operator_id"`
	Title      string            `json:"title"`
	Sources    []RunLaunchSource `json:"sources"`
}

func (c SourceIngestedContent) EventTypeName() string { return "source.ingested" }

// BriefDerivedContent records the brief that was accepted for a launch request.
type BriefDerivedContent struct {
	hiveContent
	RunID         string          `json:"run_id"`
	IntakeID      string          `json:"intake_id"`
	OperatorID    string          `json:"operator_id"`
	Title         string          `json:"title"`
	Brief         json.RawMessage `json:"brief"`
	SourceEventID types.EventID   `json:"source_event_id"`
}

func (c BriefDerivedContent) EventTypeName() string { return "brief.derived" }

// FactoryRunRequestedContent records the queued Hive run launch request. It is
// intentionally a request/queue event, not proof that runtime execution has
// started.
type FactoryRunRequestedContent struct {
	hiveContent
	RunID         string             `json:"run_id"`
	IntakeID      string             `json:"intake_id"`
	OperatorID    string             `json:"operator_id"`
	Title         string             `json:"title"`
	Status        string             `json:"status"`
	Authority     RunLaunchAuthority `json:"authority"`
	Budget        RunLaunchBudget    `json:"budget"`
	TargetRepos   []string           `json:"target_repos"`
	SourceEventID types.EventID      `json:"source_event_id"`
	BriefEventID  types.EventID      `json:"brief_event_id"`
	Sources       []RunLaunchSource  `json:"sources"`
	Brief         json.RawMessage    `json:"brief"`
}

func (c FactoryRunRequestedContent) EventTypeName() string { return "factory.run.requested" }

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

// RoleDefinitionContent stores a full RoleDefinition as a first-class event
// on the chain. Emitted at bootstrap for static agents and after approval for
// dynamic agents. Makes role templates queryable and versionable.
type RoleDefinitionContent struct {
	hiveContent
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Category    string `json:"Category"`
	Tier        string `json:"Tier"`
	CanOperate  bool   `json:"CanOperate"`
	Origin      string `json:"Origin"` // "bootstrap" or "spawned"
}

func (c RoleDefinitionContent) EventTypeName() string { return "hive.role.definition" }

// RegisterEventTypes registers hive content unmarshalers for Postgres
// deserialization. Call before querying hive events from the store.
func RegisterEventTypes() {
	event.RegisterContentUnmarshaler("hive.run.started", event.Unmarshal[RunStartedContent])
	event.RegisterContentUnmarshaler("hive.run.completed", event.Unmarshal[RunCompletedContent])
	event.RegisterContentUnmarshaler("hive.agent.spawned", event.Unmarshal[AgentSpawnedContent])
	event.RegisterContentUnmarshaler("hive.agent.stopped", event.Unmarshal[AgentStoppedContent])
	event.RegisterContentUnmarshaler("hive.progress", event.Unmarshal[ProgressContent])
	event.RegisterContentUnmarshaler("hive.role.definition", event.Unmarshal[RoleDefinitionContent])
	event.RegisterContentUnmarshaler("agent.identity.registered", event.Unmarshal[AgentIdentityRegisteredContent])
	event.RegisterContentUnmarshaler("source.ingested", event.Unmarshal[SourceIngestedContent])
	event.RegisterContentUnmarshaler("brief.derived", event.Unmarshal[BriefDerivedContent])
	event.RegisterContentUnmarshaler("factory.run.requested", event.Unmarshal[FactoryRunRequestedContent])
	registerPhase3ContentUnmarshalers()
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
