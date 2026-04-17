package checkpoint

import (
	"errors"
	"testing"
	"time"
)

// failingThoughtStore is a ThoughtStore that always returns an error.
type failingThoughtStore struct{}

func (f *failingThoughtStore) Capture(_ string) error {
	return errors.New("capture failed")
}

func (f *failingThoughtStore) SearchRecent(_ string, _ time.Duration) ([]Thought, error) {
	return nil, errors.New("search failed")
}

func TestDefaultSink_OnBoundary_CallsCapture(t *testing.T) {
	stub := NewStubThoughtStore()
	sink := NewDefaultSink(stub, nil, "implementer")

	snap := LoopSnapshot{
		Role:          "implementer",
		Iteration:     3,
		MaxIterations: 10,
		Signal:        SignalActive,
	}
	sink.OnBoundary(TaskCompleted, snap)

	if len(stub.Thoughts) != 1 {
		t.Fatalf("expected 1 captured thought, got %d", len(stub.Thoughts))
	}
}

func TestDefaultSink_OnBoundary_EmitsHeartbeat(t *testing.T) {
	stub := NewStubThoughtStore()
	var emittedSnap LoopSnapshot
	emitted := false
	emitter := func(snap LoopSnapshot) error {
		emitted = true
		emittedSnap = snap
		return nil
	}
	sink := NewDefaultSink(stub, emitter, "implementer")

	snap := LoopSnapshot{
		Role:          "implementer",
		Iteration:     5,
		MaxIterations: 100,
		Signal:        SignalActive,
		CurrentTaskID: "task-42",
	}
	sink.OnBoundary(TaskCompleted, snap)

	if !emitted {
		t.Fatal("OnBoundary should emit a heartbeat event")
	}
	if emittedSnap.Role != "implementer" {
		t.Errorf("heartbeat role = %q, want %q", emittedSnap.Role, "implementer")
	}
	if emittedSnap.Iteration != 5 {
		t.Errorf("heartbeat iteration = %d, want 5", emittedSnap.Iteration)
	}
	if emittedSnap.CurrentTaskID != "task-42" {
		t.Errorf("heartbeat task = %q, want %q", emittedSnap.CurrentTaskID, "task-42")
	}
	// Thought should also be captured.
	if len(stub.Thoughts) != 1 {
		t.Fatalf("expected 1 captured thought, got %d", len(stub.Thoughts))
	}
}

func TestDefaultSink_OnBoundary_HeartbeatFailure_DoesNotPanic(t *testing.T) {
	stub := NewStubThoughtStore()
	emitter := func(snap LoopSnapshot) error {
		return errors.New("chain write failed")
	}
	sink := NewDefaultSink(stub, emitter, "guardian")

	snap := LoopSnapshot{Role: "guardian", Iteration: 1, Signal: "HALT"}
	// Must not panic — heartbeat failure should not block boundary capture.
	sink.OnBoundary(HaltSignal, snap)

	// Thought should still be captured even when heartbeat fails.
	if len(stub.Thoughts) != 1 {
		t.Fatalf("expected 1 captured thought despite heartbeat failure, got %d", len(stub.Thoughts))
	}
}

func TestDefaultSink_OnBoundary_NilEmitter_StillCaptures(t *testing.T) {
	stub := NewStubThoughtStore()
	sink := NewDefaultSink(stub, nil, "planner")

	snap := LoopSnapshot{Role: "planner", Iteration: 2, Signal: "IDLE"}
	// Must not panic with nil emitter.
	sink.OnBoundary(TaskAssigned, snap)

	if len(stub.Thoughts) != 1 {
		t.Fatalf("expected 1 captured thought, got %d", len(stub.Thoughts))
	}
}

func TestDefaultSink_OnBoundary_CaptureFailure(t *testing.T) {
	failing := &failingThoughtStore{}
	sink := NewDefaultSink(failing, nil, "guardian")

	snap := LoopSnapshot{
		Role:      "guardian",
		Iteration: 1,
		Signal:    "HALT",
	}

	// Must not panic.
	sink.OnBoundary(HaltSignal, snap)
}

func TestDefaultSink_OnHeartbeat_CallsEmitter(t *testing.T) {
	var called bool
	emitter := func(snap LoopSnapshot) error {
		called = true
		return nil
	}
	sink := NewDefaultSink(NewStubThoughtStore(), emitter, "sysmon")
	snap := LoopSnapshot{Role: "sysmon", Iteration: 10, Signal: SignalActive}
	sink.OnHeartbeat(snap)
	if !called {
		t.Error("OnHeartbeat should have called the emitter")
	}
}

func TestDefaultSink_OnHeartbeat_EmitterFailure(t *testing.T) {
	emitter := func(snap LoopSnapshot) error {
		return errors.New("emit failed")
	}
	sink := NewDefaultSink(NewStubThoughtStore(), emitter, "sysmon")
	snap := LoopSnapshot{Role: "sysmon", Iteration: 10, Signal: SignalActive}
	// Must not panic.
	sink.OnHeartbeat(snap)
}

func TestDefaultSink_OnHeartbeat_NilEmitter(t *testing.T) {
	sink := NewDefaultSink(NewStubThoughtStore(), nil, "sysmon")
	snap := LoopSnapshot{Role: "sysmon", Iteration: 10, Signal: SignalActive}
	// Must not panic.
	sink.OnHeartbeat(snap)
}

func TestNopSink(t *testing.T) {
	var s NopSink

	snap := LoopSnapshot{Role: "planner", Iteration: 5, Signal: "IDLE"}

	// Both methods must not panic.
	s.OnBoundary(TaskAssigned, snap)
	s.OnHeartbeat(snap)
}
