package loop

import (
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// isWakeWorthy reports whether an event should wake a quiescent (keepalive)
// agent blocked in waitForEvents.
//
// With N always-on agents, per-iteration agent lifecycle/telemetry events
// (state changes, evaluations, observations, actions) form a wakeup storm: on
// the 2026-06-06 run, 907 agent.state.changed + 443 agent.evaluated events kept
// the overhead roles cycling (reviewer 149 iterations) while the implementer
// did real work only 8 times. Suppressing those wakeups lets idle governance
// agents stay asleep until substantive work arrives, without breaking keepalive
// — they remain subscribed and always-on.
//
// Fail-safe: only the known high-volume churn types are suppressed; every other
// (including unknown/future) event type wakes, so no substantive work is missed.
//
// Deliberately NOT suppressed: agent.acted. Despite being an agent.* event it is
// an action annotation for SIGNIFICANT work (Agent.Act marks e.g. "write_code",
// "integrate") and may be the only signal for an action that emits no work/task
// event — suppressing it could blind sleeping governance agents.
func isWakeWorthy(eventType types.EventType) bool {
	switch eventType {
	case event.EventTypeAgentStateChanged,
		event.EventTypeAgentObserved,
		event.EventTypeAgentEvaluated:
		return false
	default:
		return true
	}
}
