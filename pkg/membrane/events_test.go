package membrane

import (
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
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
	}

	for _, et := range eventTypes {
		if !reg.IsRegistered(et) {
			t.Errorf("event type %q not registered", et)
		}
	}
}
