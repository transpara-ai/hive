package checkpoint

import (
	"fmt"
	"os"
)

// CheckpointSink receives checkpoint events from the loop.
//
// The loop checks for nil before calling — a nil sink means no checkpointing.
type CheckpointSink interface {
	OnBoundary(trigger BoundaryTrigger, snap LoopSnapshot)
	OnHeartbeat(snap LoopSnapshot)
}

// HeartbeatEmitter is a function that writes a heartbeat event to the chain.
// Provided by the loop, which has access to the agent's event factory and signer.
type HeartbeatEmitter func(snap LoopSnapshot) error

// DefaultSink routes boundary events to Open Brain (via ThoughtStore) and
// heartbeat events to the event chain (via HeartbeatEmitter).
type DefaultSink struct {
	thoughts ThoughtStore
	emitHB   HeartbeatEmitter
	role     string
}

// NewDefaultSink constructs a DefaultSink.
// emitHB may be nil — heartbeats will be skipped with a warning.
func NewDefaultSink(thoughts ThoughtStore, emitHB HeartbeatEmitter, role string) *DefaultSink {
	return &DefaultSink{
		thoughts: thoughts,
		emitHB:   emitHB,
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

// OnHeartbeat emits a heartbeat event to the event chain via the HeartbeatEmitter.
// On failure, logs a warning and continues — never blocks.
func (d *DefaultSink) OnHeartbeat(snap LoopSnapshot) {
	if d.emitHB == nil {
		return
	}
	if err := d.emitHB(snap); err != nil {
		fmt.Fprintf(os.Stderr, "[checkpoint] heartbeat emission failed for %s: %v\n", d.role, err)
	}
}

// NopSink is a no-op CheckpointSink used when checkpointing is disabled.
type NopSink struct{}

func (NopSink) OnBoundary(BoundaryTrigger, LoopSnapshot) {}
func (NopSink) OnHeartbeat(LoopSnapshot)                 {}
