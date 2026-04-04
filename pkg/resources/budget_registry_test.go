package resources

import (
	"sync"
	"testing"
)

func TestRegisterAndSnapshot(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("guardian", NewBudget(BudgetConfig{MaxIterations: 200}), 200)
	reg.Register("sysmon", NewBudget(BudgetConfig{MaxIterations: 150}), 150)
	reg.Register("implementer", NewBudget(BudgetConfig{MaxIterations: 100}), 100)

	snap := reg.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot len = %d, want 3", len(snap))
	}

	names := make(map[string]bool)
	for _, e := range snap {
		names[e.Name] = true
	}
	for _, want := range []string{"guardian", "sysmon", "implementer"} {
		if !names[want] {
			t.Errorf("missing agent %q in snapshot", want)
		}
	}
}

func TestAdjustMaxIterations_Increase(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("impl", NewBudget(BudgetConfig{MaxIterations: 100}), 100)

	prev, newMax, err := reg.AdjustMaxIterations("impl", 50, 20, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev != 100 {
		t.Errorf("prev = %d, want 100", prev)
	}
	if newMax != 150 {
		t.Errorf("newMax = %d, want 150", newMax)
	}
}

func TestAdjustMaxIterations_Decrease(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("impl", NewBudget(BudgetConfig{MaxIterations: 100}), 100)

	prev, newMax, err := reg.AdjustMaxIterations("impl", -30, 20, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev != 100 {
		t.Errorf("prev = %d, want 100", prev)
	}
	if newMax != 70 {
		t.Errorf("newMax = %d, want 70", newMax)
	}
}

func TestAdjustMaxIterations_FloorClamp(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("impl", NewBudget(BudgetConfig{MaxIterations: 100}), 100)

	prev, newMax, err := reg.AdjustMaxIterations("impl", -90, 20, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev != 100 {
		t.Errorf("prev = %d, want 100", prev)
	}
	if newMax != 20 {
		t.Errorf("newMax = %d, want 20 (floor clamp)", newMax)
	}
}

func TestAdjustMaxIterations_CeilingClamp(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("impl", NewBudget(BudgetConfig{MaxIterations: 100}), 100)

	prev, newMax, err := reg.AdjustMaxIterations("impl", 600, 20, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev != 100 {
		t.Errorf("prev = %d, want 100", prev)
	}
	if newMax != 500 {
		t.Errorf("newMax = %d, want 500 (ceiling clamp)", newMax)
	}
}

func TestAdjustMaxIterations_UnknownAgent(t *testing.T) {
	reg := NewBudgetRegistry()

	_, _, err := reg.AdjustMaxIterations("nonexistent", 50, 20, 500)
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestAdjustMaxIterations_UpdatesBudget(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxIterations: 100})
	reg := NewBudgetRegistry()
	reg.Register("impl", b, 100)

	_, _, err := reg.AdjustMaxIterations("impl", 50, 20, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the underlying Budget was also updated.
	if got := b.MaxIterations(); got != 150 {
		t.Errorf("Budget.MaxIterations() = %d, want 150", got)
	}
}

func TestSetAgentState(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("guardian", NewBudget(BudgetConfig{MaxIterations: 200}), 200)

	reg.SetAgentState("guardian", "Quiesced")

	snap := reg.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	if snap[0].AgentState != "Quiesced" {
		t.Errorf("state = %q, want %q", snap[0].AgentState, "Quiesced")
	}
}

func TestTotalPool(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("a", NewBudget(BudgetConfig{MaxIterations: 100}), 100)
	reg.Register("b", NewBudget(BudgetConfig{MaxIterations: 150}), 150)
	reg.Register("c", NewBudget(BudgetConfig{MaxIterations: 200}), 200)

	if got := reg.TotalPool(); got != 450 {
		t.Errorf("TotalPool = %d, want 450", got)
	}
}

func TestTotalUsed(t *testing.T) {
	reg := NewBudgetRegistry()

	b1 := NewBudget(BudgetConfig{MaxIterations: 100})
	b1.Record(10, 0.1) // 1 iteration
	b1.Record(10, 0.1) // 2 iterations

	b2 := NewBudget(BudgetConfig{MaxIterations: 150})
	b2.Record(10, 0.1) // 1 iteration

	reg.Register("a", b1, 100)
	reg.Register("b", b2, 150)

	if got := reg.TotalUsed(); got != 3 {
		t.Errorf("TotalUsed = %d, want 3", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	reg := NewBudgetRegistry()
	reg.Register("agent", NewBudget(BudgetConfig{MaxIterations: 1000}), 1000)

	var wg sync.WaitGroup
	// Concurrent readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reg.Snapshot()
			_ = reg.TotalPool()
			_ = reg.TotalUsed()
		}()
	}
	// Concurrent writers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			reg.AdjustMaxIterations("agent", 1, 20, 5000)
			reg.SetAgentState("agent", "Active")
		}(i)
	}
	wg.Wait()

	// Just verify no panic/deadlock — exact values depend on scheduling.
	snap := reg.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
}

func TestBudget_SetMaxIterations(t *testing.T) {
	b := NewBudget(BudgetConfig{MaxIterations: 100})
	if got := b.MaxIterations(); got != 100 {
		t.Fatalf("initial MaxIterations = %d, want 100", got)
	}

	b.SetMaxIterations(200)
	if got := b.MaxIterations(); got != 200 {
		t.Errorf("after set MaxIterations = %d, want 200", got)
	}

	// Verify Check() uses the new limit.
	for i := 0; i < 150; i++ {
		b.Record(1, 0.01)
	}
	if err := b.Check(); err != nil {
		t.Errorf("should be within new budget of 200 after 150 iterations: %v", err)
	}
}
