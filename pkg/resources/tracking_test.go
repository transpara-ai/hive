package resources

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// mockProvider is a test double for intelligence.Provider.
type mockProvider struct {
	resp decision.Response
	err  error
}

func (m *mockProvider) Name() string  { return "mock" }
func (m *mockProvider) Model() string { return "mock-model" }
func (m *mockProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	return m.resp, m.err
}

// mockOperatorProvider extends mockProvider to also implement decision.IOperator.
type mockOperatorProvider struct {
	mockProvider
	opResult decision.OperateResult
	opErr    error
}

func (m *mockOperatorProvider) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	return m.opResult, m.opErr
}

func makeResponse(input, output int, cost float64) decision.Response {
	usage := decision.TokenUsage{InputTokens: input, OutputTokens: output, CostUSD: cost}
	return decision.NewResponse("ok", types.MustScore(0.9), usage)
}

func TestTrackingProviderSnapshot(t *testing.T) {
	mock := &mockProvider{resp: makeResponse(100, 50, 0.5)}
	p := NewTrackingProvider(mock)
	ctx := context.Background()

	// Initial snapshot is zero.
	snap := p.Snapshot()
	if snap.Iterations != 0 {
		t.Errorf("initial Iterations = %d, want 0", snap.Iterations)
	}
	if snap.TokensUsed != 0 {
		t.Errorf("initial TokensUsed = %d, want 0", snap.TokensUsed)
	}

	// Call 1.
	if _, err := p.Reason(ctx, "prompt1", nil); err != nil {
		t.Fatalf("Reason 1: %v", err)
	}
	snap = p.Snapshot()
	if snap.InputTokens != 100 {
		t.Errorf("after call 1: InputTokens = %d, want 100", snap.InputTokens)
	}
	if snap.OutputTokens != 50 {
		t.Errorf("after call 1: OutputTokens = %d, want 50", snap.OutputTokens)
	}
	if snap.TokensUsed != 150 {
		t.Errorf("after call 1: TokensUsed = %d, want 150", snap.TokensUsed)
	}
	if snap.CostUSD != 0.5 {
		t.Errorf("after call 1: CostUSD = %v, want 0.5", snap.CostUSD)
	}
	if snap.Iterations != 1 {
		t.Errorf("after call 1: Iterations = %d, want 1", snap.Iterations)
	}

	// Call 2 with different usage — values accumulate.
	mock.resp = makeResponse(200, 80, 0.25)
	if _, err := p.Reason(ctx, "prompt2", nil); err != nil {
		t.Fatalf("Reason 2: %v", err)
	}
	snap = p.Snapshot()
	if snap.InputTokens != 300 {
		t.Errorf("after call 2: InputTokens = %d, want 300", snap.InputTokens)
	}
	if snap.OutputTokens != 130 {
		t.Errorf("after call 2: OutputTokens = %d, want 130", snap.OutputTokens)
	}
	if snap.TokensUsed != 430 {
		t.Errorf("after call 2: TokensUsed = %d, want 430", snap.TokensUsed)
	}
	if snap.CostUSD != 0.75 {
		t.Errorf("after call 2: CostUSD = %v, want 0.75", snap.CostUSD)
	}
	if snap.Iterations != 2 {
		t.Errorf("after call 2: Iterations = %d, want 2", snap.Iterations)
	}
}

func TestTrackingProviderOperate(t *testing.T) {
	usage := decision.TokenUsage{InputTokens: 500, OutputTokens: 300, CostUSD: 0.5}
	mock := &mockOperatorProvider{
		opResult: decision.OperateResult{Summary: "done", Usage: usage},
	}
	p := NewTrackingProvider(mock)
	ctx := context.Background()

	result, err := p.Operate(ctx, decision.OperateTask{
		WorkDir:     "/tmp",
		Instruction: "build something",
	})
	if err != nil {
		t.Fatalf("Operate: %v", err)
	}
	if result.Summary != "done" {
		t.Errorf("Summary = %q, want %q", result.Summary, "done")
	}

	snap := p.Snapshot()
	if snap.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", snap.InputTokens)
	}
	if snap.OutputTokens != 300 {
		t.Errorf("OutputTokens = %d, want 300", snap.OutputTokens)
	}
	if snap.Iterations != 1 {
		t.Errorf("Iterations = %d, want 1", snap.Iterations)
	}
}

func TestTrackingProviderOperate_NotSupported(t *testing.T) {
	// A provider that does NOT implement IOperator — Operate must return an error.
	p := NewTrackingProvider(&mockProvider{})
	ctx := context.Background()

	_, err := p.Operate(ctx, decision.OperateTask{})
	if err == nil {
		t.Fatal("expected error when inner provider does not implement IOperator")
	}
}
