package hive

import (
	"context"
	"sync"
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

// mapModelName maps a model tier name ("haiku", "sonnet", "opus") or full model
// identifier to the canonical model constant used in AgentDef.Model.
// Accepts both the short tier name and the full identifier (since
// RoleProposedContent.Model stores the resolved full string).
// Defaults to ModelSonnet for unrecognised inputs.
func mapModelName(name string) string {
	switch name {
	case "haiku", ModelHaiku:
		return ModelHaiku
	case "sonnet", ModelSonnet:
		return ModelSonnet
	case "opus", ModelOpus:
		return ModelOpus
	default:
		return ModelSonnet
	}
}
