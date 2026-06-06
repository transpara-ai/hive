package loop

import (
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// The wake decision and the queue decision are SEPARATE concerns, governed by
// two predicates:
//
//   - isWakeWorthy: should this event WAKE a quiescent keepalive agent?
//   - isObservable: should this event be QUEUED into pendingEvents for the
//     agent's next observation (even if it does not wake the agent)?
//
// With N always-on agents, per-iteration lifecycle/telemetry events form a
// wakeup storm: on the 2026-06-06 run, 907 agent.state.changed + 443
// agent.evaluated events kept the overhead roles cycling (reviewer 149
// iterations) while the implementer did real work only 8 times. Suppressing
// those WAKEUPS lets idle governance agents stay asleep until substantive work
// arrives, without breaking keepalive — they remain subscribed and always-on.
//
// But suppressing the wake must not also discard governance signal. A peer's
// agent.evaluated is another agent's judgment — sleeping peers must still see it
// in their next observation. So agent.evaluated is queued (observable) even
// though it does not wake. Only pure high-volume lifecycle telemetry
// (state changes, observations) is dropped from observation entirely, because
// queuing it would re-introduce the very context churn the wake filter removes.

// isWakeWorthy reports whether an event should wake a quiescent (keepalive)
// agent blocked in waitForEvents.
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

// isObservable reports whether an event should be queued into pendingEvents for
// the agent's next observation, even when it does not wake the agent.
//
// Everything observable EXCEPT pure lifecycle telemetry: agent.state.changed and
// agent.observed are high-volume, content-free churn with no governance value,
// so they are dropped from observation context entirely. agent.evaluated (a peer
// judgment) and every substantive event are kept — visibility is preserved, only
// the wake is suppressed.
func isObservable(eventType types.EventType) bool {
	switch eventType {
	case event.EventTypeAgentStateChanged,
		event.EventTypeAgentObserved:
		return false
	default:
		return true
	}
}
