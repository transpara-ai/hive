package checkpoint

import (
	"errors"
	"testing"
	"time"
)

// errThoughtStore is a ThoughtStore stub that always returns an error from SearchRecent.
type errThoughtStore struct{}

func (errThoughtStore) SearchRecent(_ string, _ time.Duration) ([]Thought, error) {
	return nil, errors.New("openbrain: connection refused")
}

func (errThoughtStore) Capture(_ string) error {
	return errors.New("openbrain: connection refused")
}

// captureCheckpointFor creates a properly formatted checkpoint thought in the
// given stub for the named role/iteration/intent/taskID combination.
func captureCheckpointFor(t *testing.T, stub *StubThoughtStore, role string, iteration int, intent, taskID string) {
	t.Helper()
	snap := LoopSnapshot{
		Role:          role,
		Iteration:     iteration,
		MaxIterations: 50,
		TokensUsed:    1000,
		CostUSD:       0.15,
		Signal:        "ACTIVE",
		CurrentTaskID: taskID,
	}
	text := FormatCheckpoint(TaskAssigned, snap, intent, "", "")
	if err := stub.Capture(text); err != nil {
		t.Fatalf("Capture: %v", err)
	}
}

// TestRecoverAll_WarmStart verifies that an agent with a recent checkpoint thought
// is recovered in ModeWarm with the correct iteration and intent.
func TestRecoverAll_WarmStart(t *testing.T) {
	stub := NewStubThoughtStore()
	captureCheckpointFor(t, stub, "implementer", 12, "finish REST API handler", "task-42")

	agents := []string{"implementer"}
	result, err := RecoverAll(agents, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	rs, ok := result["implementer"]
	if !ok {
		t.Fatal("implementer not in result map")
	}
	if rs.Mode != ModeWarm {
		t.Errorf("Mode: got %v, want warm", rs.Mode)
	}
	if rs.Iteration != 12 {
		t.Errorf("Iteration: got %d, want 12", rs.Iteration)
	}
	if rs.Intent != "finish REST API handler" {
		t.Errorf("Intent: got %q, want %q", rs.Intent, "finish REST API handler")
	}
	if rs.CurrentTaskID != "task-42" {
		t.Errorf("CurrentTaskID: got %q, want %q", rs.CurrentTaskID, "task-42")
	}
}

// TestRecoverAll_ColdStart_NoThoughts verifies that all agents are ModeCold when
// the thought store is empty.
func TestRecoverAll_ColdStart_NoThoughts(t *testing.T) {
	stub := NewStubThoughtStore()

	agents := []string{"implementer", "cto", "spawner", "reviewer", "guardian"}
	result, err := RecoverAll(agents, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	for _, role := range agents {
		rs, ok := result[role]
		if !ok {
			t.Errorf("%s: not in result map", role)
			continue
		}
		if rs.Mode != ModeCold {
			t.Errorf("%s: Mode = %v, want cold", role, rs.Mode)
		}
		if rs.Iteration != 0 {
			t.Errorf("%s: Iteration = %d, want 0", role, rs.Iteration)
		}
		if rs.Intent != "" {
			t.Errorf("%s: Intent = %q, want empty", role, rs.Intent)
		}
	}
}

// TestRecoverAll_Mixed verifies that implementer goes warm when it has a checkpoint
// thought, while cto (with no thought) stays cold.
func TestRecoverAll_Mixed(t *testing.T) {
	stub := NewStubThoughtStore()
	captureCheckpointFor(t, stub, "implementer", 7, "write unit tests", "task-99")

	agents := []string{"implementer", "cto"}
	result, err := RecoverAll(agents, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	impl, ok := result["implementer"]
	if !ok {
		t.Fatal("implementer not in result map")
	}
	if impl.Mode != ModeWarm {
		t.Errorf("implementer: Mode = %v, want warm", impl.Mode)
	}
	if impl.Iteration != 7 {
		t.Errorf("implementer: Iteration = %d, want 7", impl.Iteration)
	}

	cto, ok := result["cto"]
	if !ok {
		t.Fatal("cto not in result map")
	}
	if cto.Mode != ModeCold {
		t.Errorf("cto: Mode = %v, want cold", cto.Mode)
	}
}

// TestRecoverAll_ThoughtStoreError verifies that a failing ThoughtStore causes all
// agents to cold-start without panicking, and RecoverAll itself returns no error
// (degraded-but-functional behaviour).
func TestRecoverAll_ThoughtStoreError(t *testing.T) {
	agents := []string{"implementer", "cto", "spawner"}
	result, err := RecoverAll(agents, errThoughtStore{}, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: expected no error (degraded-functional), got: %v", err)
	}
	if result == nil {
		t.Fatal("result map is nil")
	}
	for _, role := range agents {
		rs, ok := result[role]
		if !ok {
			t.Errorf("%s: not in result map", role)
			continue
		}
		if rs.Mode != ModeCold {
			t.Errorf("%s: Mode = %v, want cold", role, rs.Mode)
		}
	}
}

// TestRecoverAll_StaleThought verifies that a thought captured more than the
// staleness window ago does not trigger a warm start.
func TestRecoverAll_StaleThought(t *testing.T) {
	stub := NewStubThoughtStore()

	// Manually insert a thought with a timestamp 5 hours in the past.
	snap := LoopSnapshot{
		Role:          "implementer",
		Iteration:     3,
		MaxIterations: 50,
		TokensUsed:    500,
		CostUSD:       0.05,
		Signal:        "ACTIVE",
		CurrentTaskID: "task-11",
	}
	text := FormatCheckpoint(TaskAssigned, snap, "some old intent", "", "")
	stub.Thoughts = append(stub.Thoughts, Thought{
		Content:    text,
		CapturedAt: time.Now().Add(-5 * time.Hour),
	})

	// Staleness window is 2 hours — the 5-hour-old thought should be ignored.
	agents := []string{"implementer"}
	result, err := RecoverAll(agents, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	rs, ok := result["implementer"]
	if !ok {
		t.Fatal("implementer not in result map")
	}
	if rs.Mode != ModeCold {
		t.Errorf("Mode: got %v, want cold (thought is stale)", rs.Mode)
	}
}

// TestExtractTaskID verifies the task ID extraction helper handles various formats.
func TestExtractTaskID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"task-77 -- finish handler -- in-progress", "task-77"},
		{"task-1 -- single -- done", "task-1"},
		{"task-42", "task-42"},
		{"", ""},
	}
	for _, tc := range cases {
		got := extractTaskID(tc.input)
		if got != tc.want {
			t.Errorf("extractTaskID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestRecoveryMode_String verifies String() returns the expected labels.
func TestRecoveryMode_String(t *testing.T) {
	if ModeCold.String() != "cold" {
		t.Errorf("ModeCold.String() = %q, want %q", ModeCold.String(), "cold")
	}
	if ModeWarm.String() != "warm" {
		t.Errorf("ModeWarm.String() = %q, want %q", ModeWarm.String(), "warm")
	}
}
