package checkpoint

import (
	"fmt"
	"os"

	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// CheckpointSink receives checkpoint events from the loop.
//
// The loop checks for nil before calling — a nil sink means no checkpointing.
type CheckpointSink interface {
	OnBoundary(trigger BoundaryTrigger, snap LoopSnapshot)
	OnHeartbeat(snap LoopSnapshot)
}

// DefaultSink routes boundary events to Open Brain (via ThoughtStore) and
// heartbeat events to the event chain (via store.Store).
type DefaultSink struct {
	thoughts ThoughtStore
	store    store.Store
	actorID  types.ActorID
	role     string
}

// NewDefaultSink constructs a DefaultSink.
func NewDefaultSink(thoughts ThoughtStore, s store.Store, actorID types.ActorID, role string) *DefaultSink {
	return &DefaultSink{
		thoughts: thoughts,
		store:    s,
		actorID:  actorID,
		role:     role,
	}
}

// OnBoundary formats a checkpoint thought and captures it in Open Brain.
// On failure it logs a warning to stderr — it never blocks or panics.
func (d *DefaultSink) OnBoundary(trigger BoundaryTrigger, snap LoopSnapshot) {
	thought := FormatCheckpoint(trigger, snap, "", "", "")
	if err := d.thoughts.Capture(thought); err != nil {
		fmt.Fprintf(os.Stderr, "[checkpoint] warning: failed to capture boundary thought for %s: %v\n", d.role, err)
	}
}

// OnHeartbeat logs a heartbeat snapshot to stderr.
//
// Actual event emission to the chain requires the agent's signer and will be
// properly wired during loop integration (Task 7). For now we build the
// HeartbeatContent to validate the snapshot and log the iteration.
func (d *DefaultSink) OnHeartbeat(snap LoopSnapshot) {
	if d.store == nil {
		return
	}
	hb := HeartbeatFromSnapshot(snap)
	fmt.Fprintf(os.Stderr, "[checkpoint] heartbeat: %s iter %d\n", hb.Role, hb.Iteration)
}

// NopSink is a no-op CheckpointSink used when checkpointing is disabled.
type NopSink struct{}

func (NopSink) OnBoundary(BoundaryTrigger, LoopSnapshot) {}
func (NopSink) OnHeartbeat(LoopSnapshot)                 {}
