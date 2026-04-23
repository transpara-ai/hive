package membrane

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestEventTypesRegistered(t *testing.T) {
	reg := event.NewEventTypeRegistry()
	RegisterWithRegistry(reg)

	eventTypes := []types.EventType{
		EventActionCreated,
		EventActionApproved,
		EventActionRejected,
		EventActionTimeout,
		EventServicePolled,
		EventServiceError,
		EventNotificationSent,
		EventModeChanged,
		EventBridgeAdoption,
	}

	for _, et := range eventTypes {
		if !reg.IsRegistered(et) {
			t.Errorf("event type %q not registered", et)
		}
	}
}

func TestBridgeAdoptionContentEventTypeName(t *testing.T) {
	c := BridgeAdoptionContent{
		AgentName:  "sdr",
		MetricType: "decision_response_time",
		Value:      4.5,
		Timestamp:  "2026-03-30T12:00:00Z",
	}
	if got := c.EventTypeName(); got != "membrane.bridge.adoption" {
		t.Errorf("EventTypeName() = %q, want %q", got, "membrane.bridge.adoption")
	}
}
