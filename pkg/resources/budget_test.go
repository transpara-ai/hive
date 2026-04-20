package resources

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
)

func TestBudgetWithinLimits(t *testing.T) {
	b := NewBudget(BudgetConfig{
		MaxTokens:     1000,
		MaxCostUSD:    5.0,
		MaxIterations: 10,
	})

	b.Record(100, 0.50)

	if err := b.Check(); err != nil {
		t.Fatalf("expected within budget, got: %v", err)
	}

	snap := b.Snapshot()
	if snap.TokensUsed != 100 {
		t.Errorf("tokens = %d, want 100", snap.TokensUsed)
	}
	if snap.CostUSD != 0.50 {
		t.Errorf("cost = %.2f, want 0.50", snap.CostUSD)
	}
	if snap.Iterations != 1 {
		t.Errorf("iterations = %d, want 1", snap.Iterations)
	}
}

func TestBudgetTokensExceeded(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 100})
	b.Record(100, 0)

	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceTokens {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceTokens)
	}
}

func TestBudgetCostExceeded(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxCostUSD: 1.0})
	b.Record(0, 1.0)

	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceCost {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceCost)
	}
}

func TestBudgetIterationsExceeded(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxIterations: 3})
	b.Record(10, 0.1)
	b.Record(10, 0.1)
	b.Record(10, 0.1)

	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceIterations {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceIterations)
	}
}

func TestBudgetDurationExceeded(t *testing.T) {
	// Set start time 1 second in the past — no time.Sleep needed.
	pastStart := time.Now().Add(-1 * time.Second)
	b := NewBudgetForTest(BudgetConfig{MaxDuration: 500 * time.Millisecond}, pastStart)

	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceDuration {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceDuration)
	}
}

func TestBudgetUnlimited(t *testing.T) {
	b := NewBudget(BudgetConfig{}) // all zeros = unlimited
	b.Record(999999, 999.99)

	if err := b.Check(); err != nil {
		t.Fatalf("unlimited budget should not exceed: %v", err)
	}
}

func TestBudgetConcurrentRecords(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 100000})

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			b.Record(10, 0.01)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	snap := b.Snapshot()
	if snap.TokensUsed != 1000 {
		t.Errorf("tokens = %d, want 1000", snap.TokensUsed)
	}
	if snap.Iterations != 100 {
		t.Errorf("iterations = %d, want 100", snap.Iterations)
	}
}

func TestBudgetRecordUsage(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 10000})

	usage := decision.TokenUsage{
		InputTokens:      200,
		OutputTokens:     100,
		CacheReadTokens:  50,
		CacheWriteTokens: 25,
		CostUSD:          0.75,
	}
	b.RecordUsage(usage)

	snap := b.Snapshot()
	if snap.TokensUsed != 300 { // Total() = input + output
		t.Errorf("TokensUsed = %d, want 300", snap.TokensUsed)
	}
	if snap.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", snap.InputTokens)
	}
	if snap.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", snap.OutputTokens)
	}
	if snap.CacheReadTokens != 50 {
		t.Errorf("CacheReadTokens = %d, want 50", snap.CacheReadTokens)
	}
	if snap.CacheWriteTokens != 25 {
		t.Errorf("CacheWriteTokens = %d, want 25", snap.CacheWriteTokens)
	}
	if snap.CostUSD != 0.75 {
		t.Errorf("CostUSD = %.2f, want 0.75", snap.CostUSD)
	}
	if snap.Iterations != 1 {
		t.Errorf("Iterations = %d, want 1", snap.Iterations)
	}
}

func TestBudgetRecordUsage_Cumulative(t *testing.T) {
	b := NewBudget(BudgetConfig{})

	b.RecordUsage(decision.TokenUsage{InputTokens: 100, OutputTokens: 50, CostUSD: 0.10})
	b.RecordUsage(decision.TokenUsage{InputTokens: 200, OutputTokens: 80, CostUSD: 0.20})
	b.RecordUsage(decision.TokenUsage{InputTokens: 300, OutputTokens: 120, CostUSD: 0.30})

	snap := b.Snapshot()
	if snap.InputTokens != 600 {
		t.Errorf("InputTokens = %d, want 600", snap.InputTokens)
	}
	if snap.OutputTokens != 250 {
		t.Errorf("OutputTokens = %d, want 250", snap.OutputTokens)
	}
	if snap.TokensUsed != 850 {
		t.Errorf("TokensUsed = %d, want 850", snap.TokensUsed)
	}
	if snap.Iterations != 3 {
		t.Errorf("Iterations = %d, want 3", snap.Iterations)
	}
}

func TestBudgetCumulativeCost(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxCostUSD: 1.0})

	b.Record(10, 0.30)
	b.Record(10, 0.30)
	if err := b.Check(); err != nil {
		t.Fatalf("should be within budget at $0.60: %v", err)
	}

	b.Record(10, 0.40) // now at $1.00
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded at $1.00")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceCost {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceCost)
	}
}

func TestBudgetExactTokenLimit(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 100})

	b.Record(99, 0)
	if err := b.Check(); err != nil {
		t.Fatalf("99 tokens should be within budget of 100: %v", err)
	}

	b.Record(1, 0) // now at exactly 100
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded at exactly 100 tokens (>= limit)")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceTokens {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceTokens)
	}
}

func TestBudgetExactIterationLimit(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxIterations: 3})

	b.Record(1, 0)
	b.Record(1, 0)
	if err := b.Check(); err != nil {
		t.Fatalf("2 iterations should be within budget of 3: %v", err)
	}

	b.Record(1, 0) // 3rd iteration
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded at exactly 3 iterations (>= limit)")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceIterations {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceIterations)
	}
}

func TestBudgetDurationWithinLimit(t *testing.T) {
	// Start time is now — duration has barely elapsed.
	b := NewBudget(BudgetConfig{MaxDuration: 1 * time.Hour})
	if err := b.Check(); err != nil {
		t.Fatalf("should be within 1h duration budget: %v", err)
	}
}

func TestBudgetMultipleLimits_TokensExceededFirst(t *testing.T) {
	b := NewBudget(BudgetConfig{
		MaxTokens:     100,
		MaxCostUSD:    10.0,
		MaxIterations: 50,
	})

	// Exceed tokens but not cost or iterations.
	b.Record(100, 0.50)
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	// Tokens are checked first in Check(), so tokens should be the resource.
	if budgetErr.Resource != ResourceTokens {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceTokens)
	}
}

func TestBudgetMultipleLimits_CostExceededFirst(t *testing.T) {
	b := NewBudget(BudgetConfig{
		MaxTokens:     10000,
		MaxCostUSD:    1.0,
		MaxIterations: 50,
	})

	// Exceed cost but not tokens or iterations.
	b.Record(500, 1.0)
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceCost {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceCost)
	}
}

func TestBudgetExceededError_Message(t *testing.T) {
	err := &BudgetExceededError{
		Resource: ResourceTokens,
		Used:     "1500",
		Limit:    "1000",
	}
	msg := err.Error()
	if !strings.Contains(msg, string(ResourceTokens)) {
		t.Errorf("error message missing resource name: %s", msg)
	}
	if !strings.Contains(msg, "1500") {
		t.Errorf("error message missing used value: %s", msg)
	}
	if !strings.Contains(msg, "1000") {
		t.Errorf("error message missing limit value: %s", msg)
	}
}

func TestBudgetZeroConfig_AllUnlimited(t *testing.T) {
	// Zero values for all limits mean unlimited — even huge consumption is allowed.
	b := NewBudget(BudgetConfig{
		MaxTokens:     0,
		MaxCostUSD:    0,
		MaxIterations: 0,
		MaxDuration:   0,
	})

	b.Record(1000000, 9999.99)
	for i := 0; i < 100; i++ {
		b.Record(1, 0.01)
	}

	if err := b.Check(); err != nil {
		t.Fatalf("zero-config budget should never exceed: %v", err)
	}
}

func TestBudgetSnapshot_ElapsedPositive(t *testing.T) {
	b := NewBudget(BudgetConfig{})
	snap := b.Snapshot()
	if snap.Elapsed < 0 {
		t.Errorf("Elapsed = %v, want >= 0", snap.Elapsed)
	}
}

func TestBudgetRecordUsage_ExceedsTokenLimit(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 500})

	b.RecordUsage(decision.TokenUsage{InputTokens: 300, OutputTokens: 200, CostUSD: 0.10})
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded at 500 tokens (>= limit)")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceTokens {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceTokens)
	}
}

func TestBudgetSetMaxIterations_EnforcesNewLimit(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxIterations: 100})

	// Use 5 iterations.
	for i := 0; i < 5; i++ {
		b.Record(1, 0)
	}

	// Lower the limit to 5 — should now be at exactly the limit.
	b.SetMaxIterations(5)
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded after lowering limit to match usage")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceIterations {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceIterations)
	}

	// Raise the limit — should be back in budget.
	b.SetMaxIterations(10)
	if err := b.Check(); err != nil {
		t.Fatalf("expected within budget after raising limit: %v", err)
	}
}

func TestBudgetConcurrentRecordUsage(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 1000000})

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			b.RecordUsage(decision.TokenUsage{
				InputTokens:  10,
				OutputTokens: 5,
				CostUSD:      0.01,
			})
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	snap := b.Snapshot()
	if snap.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", snap.InputTokens)
	}
	if snap.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", snap.OutputTokens)
	}
	if snap.TokensUsed != 1500 {
		t.Errorf("TokensUsed = %d, want 1500", snap.TokensUsed)
	}
	if snap.Iterations != 100 {
		t.Errorf("Iterations = %d, want 100", snap.Iterations)
	}
}

// TestBudgetSeedConsumed verifies that a restarted agent cannot sidestep the
// BUDGET invariant by re-acquiring a fresh budget: SeedConsumed puts the
// counters where the previous run left them, and any further Record pushes
// over the limit, as expected.
func TestBudgetSeedConsumed(t *testing.T) {
	b := NewBudget(BudgetConfig{
		MaxTokens:     1000,
		MaxCostUSD:    5.0,
		MaxIterations: 10,
	})

	b.SeedConsumed(7, 800, 3.50)

	snap := b.Snapshot()
	if snap.Iterations != 7 {
		t.Errorf("Iterations = %d, want 7", snap.Iterations)
	}
	if snap.TokensUsed != 800 {
		t.Errorf("TokensUsed = %d, want 800", snap.TokensUsed)
	}
	if snap.CostUSD != 3.50 {
		t.Errorf("CostUSD = %v, want 3.50", snap.CostUSD)
	}

	// Still within budget after seeding.
	if err := b.Check(); err != nil {
		t.Fatalf("unexpected budget error after seed: %v", err)
	}

	// Additional consumption on top of the seed tips over the token cap.
	b.Record(300, 0.1)
	err := b.Check()
	if err == nil {
		t.Fatal("expected budget exceeded after Record past seeded total")
	}
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %T", err)
	}
	if budgetErr.Resource != ResourceTokens {
		t.Errorf("resource = %q, want %q", budgetErr.Resource, ResourceTokens)
	}
}

// TestBudgetSeedConsumed_Overwrites verifies SeedConsumed replaces (rather than
// adds to) existing counters — restart recovery is idempotent even if called
// after some consumption has already been recorded.
func TestBudgetSeedConsumed_Overwrites(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxTokens: 1000, MaxCostUSD: 5.0, MaxIterations: 10})

	b.Record(100, 0.5)
	b.SeedConsumed(3, 200, 1.0)

	snap := b.Snapshot()
	if snap.TokensUsed != 200 {
		t.Errorf("TokensUsed = %d, want 200 (seed should overwrite, not add)", snap.TokensUsed)
	}
	if snap.Iterations != 3 {
		t.Errorf("Iterations = %d, want 3", snap.Iterations)
	}
	if snap.CostUSD != 1.0 {
		t.Errorf("CostUSD = %v, want 1.0", snap.CostUSD)
	}
}
