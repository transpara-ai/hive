package loop

import (
	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
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

// isObservable reports the TYPE-LEVEL decision for whether an event should be
// queued into pendingEvents for the agent's next observation, even when it does
// not wake the agent.
//
// agent.observed is pure churn (dropped). agent.state.changed is suppressed at
// the type level because most of its volume is the Idle⇄Processing toggle — but
// onEvent overrides this for SIGNIFICANT lifecycle transitions (see
// isChurnStateChange), so a peer's Suspended/Retiring/Retired is NOT hidden.
// agent.evaluated (a peer judgment) and every substantive event are kept.
func isObservable(eventType types.EventType) bool {
	switch eventType {
	case event.EventTypeAgentStateChanged,
		event.EventTypeAgentObserved:
		return false
	default:
		return true
	}
}

// isChurnStateChange reports whether an agent.state.changed event is pure
// high-volume operational churn — the Idle⇄Processing toggle that formed the
// 2026-06-06 wakeup storm — rather than a significant lifecycle/governance
// transition (Suspended, Retiring, Retired, Waiting, Escalating, Refusing). Only
// churn is suppressed; significant transitions stay visible to peers and wake
// idle governance agents. For OBSERVABILITY the fail-safe default is to KEEP an
// event, so an unknown or unparseable state is treated as significant (not
// churn) — hiding a lifecycle change is the risk to avoid.
func isChurnStateChange(ev event.Event) bool {
	c, ok := ev.Content().(event.AgentStateChangedContent)
	if !ok {
		return false
	}
	switch c.Current {
	case egagent.StateIdle.String(), egagent.StateProcessing.String():
		return true
	default:
		return false
	}
}
