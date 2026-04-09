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
