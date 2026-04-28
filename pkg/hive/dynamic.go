package hive

import (
	"context"
	"fmt"
	"sync"

	"github.com/transpara-ai/hive/pkg/modelconfig"
)

// dynamicAgentTracker manages the lifecycle of agents spawned after boot.
// Bootstrap agents run inside RunConcurrent (one-shot WaitGroup). Dynamic
// agents need their own WaitGroup so Run() can wait for both cohorts.
type dynamicAgentTracker struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	agents map[string]context.CancelFunc // name → cancel func
}

func newDynamicAgentTracker() *dynamicAgentTracker {
	return &dynamicAgentTracker{
		agents: make(map[string]context.CancelFunc),
	}
}

// Track registers an agent for lifecycle tracking.
// If name is already tracked, this is a no-op (dedup guard).
func (d *dynamicAgentTracker) Track(name string, cancel context.CancelFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.agents[name]; exists {
		return
	}
	d.agents[name] = cancel
}

// IsTracked returns true if an agent with the given name has been registered.
func (d *dynamicAgentTracker) IsTracked(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, exists := d.agents[name]
	return exists
}

// Wait blocks until all tracked dynamic agent goroutines have finished.
func (d *dynamicAgentTracker) Wait() {
	d.wg.Wait()
}

// mapModelName resolves a model name (alias or full ID) to its canonical catalog ID.
// Returns an error if the model is not found — validateSpawnCommand should have
// rejected unknown models before this point, so a miss here indicates a bug.
func mapModelName(name string, cat *modelconfig.ModelCatalog) (string, error) {
	if cat == nil {
		cat = modelconfig.DefaultCatalog()
	}
	entry, ok := cat.Lookup(name)
	if !ok {
		return "", fmt.Errorf("model %q not found in catalog (validation should have caught this)", name)
	}
	return entry.ID, nil
}
