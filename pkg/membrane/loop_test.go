package membrane

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

// mockServiceClient implements ServiceClient for testing.
type mockServiceClient struct {
	getResponses map[string]json.RawMessage
	getCalled    atomic.Int32
}

func (m *mockServiceClient) Get(ctx context.Context, path string) (json.RawMessage, error) {
	m.getCalled.Add(1)
	if resp, ok := m.getResponses[path]; ok {
		return resp, nil
	}
	return json.RawMessage(`{}`), nil
}

func (m *mockServiceClient) Post(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	return json.RawMessage(`{"status":"ok"}`), nil
}

func TestMembraneLoopPolls(t *testing.T) {
	mock := &mockServiceClient{
		getResponses: map[string]json.RawMessage{
			"/api/status": json.RawMessage(`{"state":"ok"}`),
		},
	}

	cfg := LoopConfig{
		PollInterval: 50 * time.Millisecond,
		PollPath:     "/api/status",
		Service:      mock,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result := RunMembraneLoop(ctx, cfg)

	calls := mock.getCalled.Load()
	if calls < 2 {
		t.Errorf("expected at least 2 polls, got %d", calls)
	}
	if result.Reason != LoopStopCancelled {
		t.Errorf("reason = %q, want %q", result.Reason, LoopStopCancelled)
	}
}

func TestMembraneLoopStopsOnMaxIterations(t *testing.T) {
	mock := &mockServiceClient{
		getResponses: map[string]json.RawMessage{
			"/api/status": json.RawMessage(`{"state":"ok"}`),
		},
	}

	cfg := LoopConfig{
		PollInterval:  10 * time.Millisecond,
		PollPath:      "/api/status",
		Service:       mock,
		MaxIterations: 3,
	}

	ctx := context.Background()
	result := RunMembraneLoop(ctx, cfg)

	if result.Reason != LoopStopBudget {
		t.Errorf("reason = %q, want %q", result.Reason, LoopStopBudget)
	}
	if result.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", result.Iterations)
	}
}
