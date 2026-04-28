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
	OnBoundaryWithContext(trigger BoundaryTrigger, snap LoopSnapshot, ctx BoundaryContext)
	OnHeartbeat(snap LoopSnapshot)
}

// BoundaryContext carries resumable reasoning context for an Open Brain
// checkpoint. Empty fields are valid for purely mechanical checkpoints.
type BoundaryContext struct {
	Intent  string
	Next    string
	Context string
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

// SetHeartbeatEmitter wires the heartbeat callback after construction.
// Called by the loop once it has access to the agent's event emission path.
func (d *DefaultSink) SetHeartbeatEmitter(emitHB HeartbeatEmitter) {
	d.emitHB = emitHB
}

// OnBoundary formats a checkpoint thought and captures it in Open Brain.
// It also emits a heartbeat event to the event chain so every boundary
// is visible on the graph (not just periodic heartbeats).
// On failure it logs a warning to stderr — it never blocks or panics.
func (d *DefaultSink) OnBoundary(trigger BoundaryTrigger, snap LoopSnapshot) {
	d.OnBoundaryWithContext(trigger, snap, BoundaryContext{})
}

// OnBoundaryWithContext formats a checkpoint thought with resumable reasoning
// context and captures it in Open Brain.
func (d *DefaultSink) OnBoundaryWithContext(trigger BoundaryTrigger, snap LoopSnapshot, ctx BoundaryContext) {
	thought := FormatCheckpoint(trigger, snap, ctx.Intent, ctx.Next, ctx.Context)
	if err := d.thoughts.Capture(thought); err != nil {
		fmt.Fprintf(os.Stderr, "[checkpoint] warning: failed to capture boundary thought for %s: %v\n", d.role, err)
	}

	// Also emit a heartbeat event so the boundary is recorded on the chain.
	if d.emitHB != nil {
		if err := d.emitHB(snap); err != nil {
			fmt.Fprintf(os.Stderr, "[checkpoint] warning: boundary heartbeat emission failed for %s: %v\n", d.role, err)
		}
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

func (NopSink) OnBoundary(BoundaryTrigger, LoopSnapshot)                             {}
func (NopSink) OnBoundaryWithContext(BoundaryTrigger, LoopSnapshot, BoundaryContext) {}
func (NopSink) OnHeartbeat(LoopSnapshot)                                             {}
