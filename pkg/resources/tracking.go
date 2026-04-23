package resources

import (
	"context"
	"fmt"
	"sync"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
)

// TrackingProvider wraps an intelligence.Provider and accumulates token usage
// across all calls. Thread-safe.
type TrackingProvider struct {
	inner intelligence.Provider

	mu    sync.Mutex
	calls int
	total decision.TokenUsage
}

// NewTrackingProvider wraps a provider to track token usage.
func NewTrackingProvider(inner intelligence.Provider) *TrackingProvider {
	return &TrackingProvider{inner: inner}
}

func (t *TrackingProvider) Name() string  { return t.inner.Name() }
func (t *TrackingProvider) Model() string { return t.inner.Model() }

func (t *TrackingProvider) Reason(ctx context.Context, prompt string, history []event.Event) (decision.Response, error) {
	resp, err := t.inner.Reason(ctx, prompt, history)
	if err != nil {
		return resp, err
	}

	u := resp.Usage()
	t.mu.Lock()
	t.calls++
	t.total.InputTokens += u.InputTokens
	t.total.OutputTokens += u.OutputTokens
	t.total.CacheReadTokens += u.CacheReadTokens
	t.total.CacheWriteTokens += u.CacheWriteTokens
	t.total.CostUSD += u.CostUSD
	t.mu.Unlock()

	return resp, nil
}

// Operate delegates to the inner provider's Operate method if it implements IOperator.
func (t *TrackingProvider) Operate(ctx context.Context, task decision.OperateTask) (decision.OperateResult, error) {
	op, ok := t.inner.(decision.IOperator)
	if !ok {
		return decision.OperateResult{}, fmt.Errorf("provider %s does not support Operate", t.inner.Name())
	}

	result, err := op.Operate(ctx, task)
	if err != nil {
		return result, err
	}

	u := result.Usage
	t.mu.Lock()
	t.calls++
	t.total.InputTokens += u.InputTokens
	t.total.OutputTokens += u.OutputTokens
	t.total.CacheReadTokens += u.CacheReadTokens
	t.total.CacheWriteTokens += u.CacheWriteTokens
	t.total.CostUSD += u.CostUSD
	t.mu.Unlock()

	return result, nil
}

// Snapshot returns the accumulated token usage.
func (t *TrackingProvider) Snapshot() BudgetSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return BudgetSnapshot{
		TokensUsed:       t.total.Total(),
		InputTokens:      t.total.InputTokens,
		OutputTokens:     t.total.OutputTokens,
		CacheReadTokens:  t.total.CacheReadTokens,
		CacheWriteTokens: t.total.CacheWriteTokens,
		CostUSD:          t.total.CostUSD,
		Iterations:       t.calls,
	}
}

// Compile-time check.
var _ intelligence.Provider = (*TrackingProvider)(nil)
