// Package resources tracks and enforces agent resource budgets.
//
// Every agent loop iteration consumes resources — tokens, cost, time.
// Budget enforcement prevents runaway agents (BUDGET invariant).
package resources

import (
	"fmt"
	"sync"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
)

// Budget tracks resource consumption for an agent loop.
// Safe for concurrent use.
type Budget struct {
	mu sync.Mutex

	// Limits (zero means unlimited).
	maxTokens     int
	maxCostUSD    float64
	maxIterations int
	maxDuration   time.Duration

	// Consumed.
	tokensUsed       int
	inputTokens      int
	outputTokens     int
	cacheReadTokens  int
	cacheWriteTokens int
	costUSD          float64
	iterations       int
	startTime        time.Time
}

// BudgetConfig configures resource limits. Zero values mean unlimited.
type BudgetConfig struct {
	MaxTokens     int           // total tokens across all iterations
	MaxCostUSD    float64       // total cost in USD
	MaxIterations int           // maximum loop iterations
	MaxDuration   time.Duration // maximum wall-clock time
}

// NewBudget creates a budget tracker with the given limits.
func NewBudget(cfg BudgetConfig) *Budget {
	return newBudgetAt(cfg, time.Now())
}

// newBudgetAt creates a budget tracker with a custom start time.
// Exported for testing via NewBudgetForTest.
func newBudgetAt(cfg BudgetConfig, start time.Time) *Budget {
	return &Budget{
		maxTokens:     cfg.MaxTokens,
		maxCostUSD:    cfg.MaxCostUSD,
		maxIterations: cfg.MaxIterations,
		maxDuration:   cfg.MaxDuration,
		startTime:     start,
	}
}

// NewBudgetForTest creates a budget with a custom start time for testing
// duration limits without time.Sleep.
func NewBudgetForTest(cfg BudgetConfig, start time.Time) *Budget {
	return newBudgetAt(cfg, start)
}

// SeedConsumed restores consumed counters from a previous run's checkpoint.
// This closes the reboot-survival gap where a restarted agent would otherwise
// get a full fresh budget, violating the BUDGET invariant. Only the iteration
// counter, token total, and cost are seeded — detailed breakdowns (input vs
// output tokens, cache tokens) are lost across restarts by design.
func (b *Budget) SeedConsumed(iterations, tokens int, costUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.iterations = iterations
	b.tokensUsed = tokens
	b.costUSD = costUSD
}

// Record adds resource consumption from one iteration.
func (b *Budget) Record(tokens int, costUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokensUsed += tokens
	b.costUSD += costUSD
	b.iterations++
}

// RecordUsage adds resource consumption from a TokenUsage breakdown.
func (b *Budget) RecordUsage(usage decision.TokenUsage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokensUsed += usage.Total()
	b.inputTokens += usage.InputTokens
	b.outputTokens += usage.OutputTokens
	b.cacheReadTokens += usage.CacheReadTokens
	b.cacheWriteTokens += usage.CacheWriteTokens
	b.costUSD += usage.CostUSD
	b.iterations++
}

// MaxIterations returns the current iteration limit. Safe for concurrent use.
func (b *Budget) MaxIterations() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maxIterations
}

// SetMaxIterations updates the iteration limit at runtime.
// Used by the Allocator to redistribute budget across agents.
func (b *Budget) SetMaxIterations(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxIterations = n
}

// Check returns nil if within budget, or an error describing what was exceeded.
func (b *Budget) Check() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.maxTokens > 0 && b.tokensUsed >= b.maxTokens {
		return &BudgetExceededError{
			Resource: ResourceTokens,
			Used:     fmt.Sprintf("%d", b.tokensUsed),
			Limit:    fmt.Sprintf("%d", b.maxTokens),
		}
	}
	if b.maxCostUSD > 0 && b.costUSD >= b.maxCostUSD {
		return &BudgetExceededError{
			Resource: ResourceCost,
			Used:     fmt.Sprintf("$%.2f", b.costUSD),
			Limit:    fmt.Sprintf("$%.2f", b.maxCostUSD),
		}
	}
	if b.maxIterations > 0 && b.iterations >= b.maxIterations {
		return &BudgetExceededError{
			Resource: ResourceIterations,
			Used:     fmt.Sprintf("%d", b.iterations),
			Limit:    fmt.Sprintf("%d", b.maxIterations),
		}
	}
	if b.maxDuration > 0 {
		elapsed := time.Since(b.startTime)
		if elapsed >= b.maxDuration {
			return &BudgetExceededError{
				Resource: ResourceDuration,
				Used:     elapsed.String(),
				Limit:    b.maxDuration.String(),
			}
		}
	}
	return nil
}

// Snapshot returns a point-in-time copy of resource consumption.
func (b *Budget) Snapshot() BudgetSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BudgetSnapshot{
		TokensUsed:       b.tokensUsed,
		InputTokens:      b.inputTokens,
		OutputTokens:     b.outputTokens,
		CacheReadTokens:  b.cacheReadTokens,
		CacheWriteTokens: b.cacheWriteTokens,
		CostUSD:          b.costUSD,
		Iterations:       b.iterations,
		Elapsed:          time.Since(b.startTime),
	}
}

// BudgetSnapshot is an immutable copy of resource consumption at a point in time.
type BudgetSnapshot struct {
	TokensUsed       int
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	Iterations       int
	Elapsed          time.Duration
}

// BudgetResource identifies which resource limit was exceeded.
type BudgetResource string

const (
	ResourceTokens     BudgetResource = "tokens"
	ResourceCost       BudgetResource = "cost"
	ResourceIterations BudgetResource = "iterations"
	ResourceDuration   BudgetResource = "duration"
)

// BudgetExceededError is returned when a resource limit is reached.
type BudgetExceededError struct {
	Resource BudgetResource
	Used     string
	Limit    string
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: %s used %s (limit %s)", e.Resource, e.Used, e.Limit)
}
