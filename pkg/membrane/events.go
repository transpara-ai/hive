package membrane

import (
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

var (
	EventActionCreated    = types.MustEventType("membrane.action.created")
	EventActionApproved   = types.MustEventType("membrane.action.approved")
	EventActionRejected   = types.MustEventType("membrane.action.rejected")
	EventActionTimeout    = types.MustEventType("membrane.action.timeout")
	EventServicePolled    = types.MustEventType("membrane.service.polled")
	EventServiceError     = types.MustEventType("membrane.service.error")
	EventNotificationSent = types.MustEventType("membrane.notification.sent")
	EventModeChanged      = types.MustEventType("membrane.mode.changed")
	EventBridgeAdoption   = types.MustEventType("membrane.bridge.adoption")
)

func allMembraneEventTypes() []types.EventType {
	return []types.EventType{
		EventActionCreated, EventActionApproved, EventActionRejected, EventActionTimeout,
		EventServicePolled, EventServiceError, EventNotificationSent, EventModeChanged,
		EventBridgeAdoption,
	}
}

// membraneContent is embedded in all membrane content types (same pattern as hiveContent).
type membraneContent struct{}

func (membraneContent) Accept(event.EventContentVisitor) {}

// ActionCreatedContent records a pending human action.
type ActionCreatedContent struct {
	membraneContent
	AgentName     string      `json:"AgentName"`
	ActionType    string      `json:"ActionType"`   // "approval", "handoff", "escalation"
	Summary       string      `json:"Summary"`
	Authority     string      `json:"Authority"`    // "required", "recommended", "notification"
	TargetHuman   string      `json:"TargetHuman"`  // actor ID
	DomainContext interface{} `json:"DomainContext"` // instance-specific payload
}

func (c ActionCreatedContent) EventTypeName() string { return "membrane.action.created" }

// ActionDecidedContent records a human decision.
type ActionDecidedContent struct {
	membraneContent
	AgentName string `json:"AgentName"`
	ActionID  string `json:"ActionID"`
	Decision  string `json:"Decision"` // "approved", "rejected", "edited", "redirected"
	DecidedBy string `json:"DecidedBy"`
	Notes     string `json:"Notes,omitempty"`
}

func (c ActionDecidedContent) EventTypeName() string { return "membrane.action.decided" }

// ServicePolledContent records a service poll result.
type ServicePolledContent struct {
	membraneContent
	AgentName   string `json:"AgentName"`
	Endpoint    string `json:"Endpoint"`
	EventsFound int    `json:"EventsFound"`
	ErrorCount  int    `json:"ErrorCount"`
}

func (c ServicePolledContent) EventTypeName() string { return "membrane.service.polled" }

// ServiceErrorContent records a service communication failure.
type ServiceErrorContent struct {
	membraneContent
	AgentName string `json:"AgentName"`
	Endpoint  string `json:"Endpoint"`
	Error     string `json:"Error"`
	Attempt   int    `json:"Attempt"`
}

func (c ServiceErrorContent) EventTypeName() string { return "membrane.service.error" }

// NotificationSentContent records an outbound notification.
type NotificationSentContent struct {
	membraneContent
	AgentName   string `json:"AgentName"`
	ActionID    string `json:"ActionID"`
	Channel     string `json:"Channel"` // "email", "teams"
	TargetHuman string `json:"TargetHuman"`
}

func (c NotificationSentContent) EventTypeName() string { return "membrane.notification.sent" }

// ModeChangedContent records a service operating mode change.
type ModeChangedContent struct {
	membraneContent
	AgentName    string  `json:"AgentName"`
	PreviousMode string  `json:"PreviousMode"`
	NewMode      string  `json:"NewMode"`
	TrustScore   float64 `json:"TrustScore"`
	Reason       string  `json:"Reason"`
}

func (c ModeChangedContent) EventTypeName() string { return "membrane.mode.changed" }

// BridgeAdoptionContent records an agent-bridge self-tracking metric.
type BridgeAdoptionContent struct {
	membraneContent
	AgentName  string  `json:"AgentName"`
	MetricType string  `json:"MetricType"` // "decision_response_time", "dashboard_visit", "action_created"
	Value      float64 `json:"Value"`
	Timestamp  string  `json:"Timestamp"` // RFC3339
}

func (c BridgeAdoptionContent) EventTypeName() string { return "membrane.bridge.adoption" }

// RegisterMembraneEventTypes registers content unmarshalers for Postgres deserialization.
func RegisterMembraneEventTypes() {
	event.RegisterContentUnmarshaler("membrane.action.created", event.Unmarshal[ActionCreatedContent])
	event.RegisterContentUnmarshaler("membrane.action.approved", event.Unmarshal[ActionDecidedContent])
	event.RegisterContentUnmarshaler("membrane.action.rejected", event.Unmarshal[ActionDecidedContent])
	event.RegisterContentUnmarshaler("membrane.action.timeout", event.Unmarshal[ActionDecidedContent])
	event.RegisterContentUnmarshaler("membrane.service.polled", event.Unmarshal[ServicePolledContent])
	event.RegisterContentUnmarshaler("membrane.service.error", event.Unmarshal[ServiceErrorContent])
	event.RegisterContentUnmarshaler("membrane.notification.sent", event.Unmarshal[NotificationSentContent])
	event.RegisterContentUnmarshaler("membrane.mode.changed", event.Unmarshal[ModeChangedContent])
	event.RegisterContentUnmarshaler("membrane.bridge.adoption", event.Unmarshal[BridgeAdoptionContent])
}

// RegisterWithRegistry registers all membrane event types with the given registry
// and registers content unmarshalers. Follows the same pattern as hive.RegisterWithRegistry.
func RegisterWithRegistry(registry *event.EventTypeRegistry) {
	for _, et := range allMembraneEventTypes() {
		registry.Register(et, nil)
	}
	RegisterMembraneEventTypes()
}
