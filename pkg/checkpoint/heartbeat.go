package checkpoint

// Import note: heartbeat.go intentionally imports eventgraph packages (store,
// types, event) to put the heartbeat event on the chain. This is separate from
// thought.go which is stdlib-only for Open Brain capture.
//
// Circular import analysis:
//   checkpoint → eventgraph (store, types, event) — OK (eventgraph has no hive dep)
//   hive → checkpoint (for registration) — OK (checkpoint does not import hive)
// No cycle exists. EventTypeAgentHeartbeat is defined here so checkpoint is
// self-contained; events.go in pkg/hive imports it from here.

import (
	"encoding/json"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// EventTypeAgentHeartbeat is the event type for periodic agent heartbeat
// snapshots written to the event chain.
var EventTypeAgentHeartbeat = types.MustEventType("hive.agent.heartbeat")

// heartbeatContent is embedded in HeartbeatContent to satisfy the
// event.EventContent interface. Uses no-op Accept — same pattern as
// pkg/hive content types (e.g. ProgressContent).
type heartbeatContent struct{}

func (heartbeatContent) Accept(event.EventContentVisitor) {}

// HeartbeatContent is written to the event chain at each agent loop checkpoint.
// It captures the live state of a running agent loop iteration.
type HeartbeatContent struct {
	heartbeatContent
	Role          string  `json:"role"`
	Iteration     int     `json:"iteration"`
	MaxIterations int     `json:"max_iterations"`
	TokensUsed    int     `json:"tokens_used"`
	CostUSD       float64 `json:"cost_usd"`
	Signal        string  `json:"signal"`
	CurrentTaskID string  `json:"current_task_id,omitempty"`
}

// EventTypeName satisfies event.EventContent.
func (c HeartbeatContent) EventTypeName() string { return "hive.agent.heartbeat" }

// HeartbeatFromSnapshot converts a LoopSnapshot into a HeartbeatContent
// ready to be written to the event chain.
func HeartbeatFromSnapshot(snap LoopSnapshot) HeartbeatContent {
	return HeartbeatContent{
		Role:          snap.Role,
		Iteration:     snap.Iteration,
		MaxIterations: snap.MaxIterations,
		TokensUsed:    snap.TokensUsed,
		CostUSD:       snap.CostUSD,
		Signal:        snap.Signal,
		CurrentTaskID: snap.CurrentTaskID,
	}
}

// QueryLatestHeartbeat searches the event chain for the most recent heartbeat
// from the given role that was recorded after the `after` timestamp.
//
// Returns nil, nil when:
//   - s is nil (no store configured)
//   - no matching heartbeat is found
func QueryLatestHeartbeat(s store.Store, role string, after time.Time) (*HeartbeatContent, error) {
	if s == nil {
		return nil, nil
	}

	cursor := types.None[types.Cursor]()

	for {
		page, err := s.ByType(EventTypeAgentHeartbeat, 100, cursor)
		if err != nil {
			return nil, err
		}

		items := page.Items()
		if len(items) == 0 {
			break
		}

		for _, ev := range items {
			ts := ev.Timestamp().Value()

			// ByType returns reverse-chronological order. Once we see events
			// older than the cutoff, nothing further will match.
			if ts.Before(after) {
				return nil, nil
			}

			// Deserialize the content.
			raw, err := json.Marshal(ev.Content())
			if err != nil {
				continue
			}
			var hb HeartbeatContent
			if err := json.Unmarshal(raw, &hb); err != nil {
				continue
			}

			if hb.Role == role {
				return &hb, nil
			}
		}

		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}

	return nil, nil
}
