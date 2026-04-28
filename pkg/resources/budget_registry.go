package resources

import (
	"fmt"
	"sync"
)

// BudgetEntry represents one agent's budget state in the registry.
type BudgetEntry struct {
	Name          string
	Budget        *Budget
	MaxIterations int
	AgentState    string // "Active", "Quiesced", "Stopped"
	ResolvedModel string // canonical model ID from resolver (for cost estimation)
}

// BudgetRegistry provides cross-agent budget visibility and mutation.
// The Allocator reads snapshots to assess consumption; the framework
// writes adjustments when /budget commands are validated.
// Safe for concurrent use.
type BudgetRegistry struct {
	mu      sync.RWMutex
	entries map[string]*BudgetEntry
}

// NewBudgetRegistry creates an empty registry.
func NewBudgetRegistry() *BudgetRegistry {
	return &BudgetRegistry{
		entries: make(map[string]*BudgetEntry),
	}
}

// Register adds an agent's budget to the registry. Called during agent spawn.
// resolvedModel is the canonical model ID from the resolver (used for cost estimation).
func (r *BudgetRegistry) Register(name string, budget *Budget, maxIter int, resolvedModel string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[name] = &BudgetEntry{
		Name:          name,
		Budget:        budget,
		MaxIterations: maxIter,
		AgentState:    "Active",
		ResolvedModel: resolvedModel,
	}
}

// Snapshot returns a copy of all agents' budget states.
// The returned slice contains value copies — safe to read without locking.
func (r *BudgetRegistry) Snapshot() []BudgetEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]BudgetEntry, 0, len(r.entries))
	for _, e := range r.entries {
		result = append(result, BudgetEntry{
			Name:          e.Name,
			Budget:        e.Budget,
			MaxIterations: e.MaxIterations,
			AgentState:    e.AgentState,
			ResolvedModel: e.ResolvedModel,
		})
	}
	return result
}

// AdjustMaxIterations modifies a specific agent's iteration limit by delta.
// Clamps to [floor, ceiling]. Returns (previousMax, newMax, error).
// Error if the agent is not found; clamps do not produce errors.
func (r *BudgetRegistry) AdjustMaxIterations(name string, delta int, floor int, ceiling int) (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[name]
	if !ok {
		return 0, 0, fmt.Errorf("unknown agent: %s", name)
	}
	prev := e.MaxIterations
	newMax := prev + delta

	// Clamp to bounds.
	if newMax < floor {
		newMax = floor
	}
	if newMax > ceiling {
		newMax = ceiling
	}

	e.MaxIterations = newMax
	e.Budget.SetMaxIterations(newMax)
	return prev, newMax, nil
}

// SetAgentState updates an agent's operational state.
func (r *BudgetRegistry) SetAgentState(name string, state string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[name]; ok {
		e.AgentState = state
	}
}

// TotalPool returns the sum of MaxIterations across all registered agents.
func (r *BudgetRegistry) TotalPool() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, e := range r.entries {
		total += e.MaxIterations
	}
	return total
}

// TotalUsed returns the sum of consumed iterations across all registered agents.
func (r *BudgetRegistry) TotalUsed() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, e := range r.entries {
		total += e.Budget.Snapshot().Iterations
	}
	return total
}
