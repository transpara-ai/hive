package hive

import (
	"context"
	"fmt"
	"sync"

	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
)

// dynamicAgentTracker manages the lifecycle of agents spawned after boot.
// Bootstrap agents run inside RunConcurrent (one-shot WaitGroup). Dynamic
// agents need their own WaitGroup so Run() can wait for both cohorts.
type dynamicAgentTracker struct {
	mu           sync.Mutex
	wg           sync.WaitGroup
	maximum      int
	agents       map[string]context.CancelFunc
	recoveries   map[string]bool
	reservations map[string]struct{}
	limitEvents  map[string]dynamicLimitEventState
	recovered    int
	created      int
}

type dynamicSlotResult int

const (
	dynamicSlotReserved dynamicSlotResult = iota
	dynamicSlotDuplicate
	dynamicSlotLimitReached
)

type dynamicLimitEventState int

const (
	dynamicLimitEventInFlight dynamicLimitEventState = iota
	dynamicLimitEventCommitted
)

func newDynamicAgentTracker(maximum ...int) *dynamicAgentTracker {
	limit := 0
	if len(maximum) > 0 {
		limit = maximum[0]
	}
	return &dynamicAgentTracker{
		maximum:      limit,
		agents:       make(map[string]context.CancelFunc),
		recoveries:   make(map[string]bool),
		reservations: make(map[string]struct{}),
		limitEvents:  make(map[string]dynamicLimitEventState),
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
	d.created++
}

// IsTracked returns true if an agent with the given name has been registered.
func (d *dynamicAgentTracker) IsTracked(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, tracked := d.agents[name]
	_, reserved := d.reservations[name]
	return tracked || reserved
}

// Reserve atomically holds one capacity slot for a normalized dynamic role.
// maximum=0 retains the historical unbounded full-profile posture.
func (d *dynamicAgentTracker) Reserve(name string) dynamicSlotResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.agents[name]; ok {
		return dynamicSlotDuplicate
	}
	if _, ok := d.reservations[name]; ok {
		return dynamicSlotDuplicate
	}
	if d.maximum > 0 && len(d.agents)+len(d.reservations) >= d.maximum {
		return dynamicSlotLimitReached
	}
	d.reservations[name] = struct{}{}
	return dynamicSlotReserved
}

// Release releases a reservation only when no actor has been committed.
func (d *dynamicAgentTracker) Release(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.reservations, name)
}

// Attach commits a reserved actor to runtime ownership before its goroutine is
// released. It returns false if the role was not reserved or was already
// committed.
func (d *dynamicAgentTracker) Attach(name string, cancel context.CancelFunc, recovered bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.agents[name]; ok {
		return false
	}
	if _, ok := d.reservations[name]; !ok {
		return false
	}
	delete(d.reservations, name)
	d.agents[name] = cancel
	d.recoveries[name] = recovered
	if recovered {
		d.recovered++
	} else {
		d.created++
	}
	d.wg.Add(1)
	return true
}

func (d *dynamicAgentTracker) Done() {
	d.wg.Done()
}

func (d *dynamicAgentTracker) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.agents)
}

// OccupiedCount includes committed actors and in-flight reservations. It is
// the atomic admission count used when a concurrent proposal observes a full
// cap; completion evidence continues to use Count(), which reports only actors
// actually owned by the runtime.
func (d *dynamicAgentTracker) OccupiedCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.agents) + len(d.reservations)
}

func (d *dynamicAgentTracker) RecoveredCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.recovered
}

func (d *dynamicAgentTracker) NewCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.created
}

// BeginLimitEvent claims one idempotency tuple for recording. A concurrent
// observer sees the in-flight claim and does not duplicate the write.
func (d *dynamicAgentTracker) BeginLimitEvent(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.limitEvents[key]; exists {
		return false
	}
	d.limitEvents[key] = dynamicLimitEventInFlight
	return true
}

// FinishLimitEvent commits a successful record or releases a failed in-flight
// claim so the next bounded poll can retry. A committed tuple is permanent.
func (d *dynamicAgentTracker) FinishLimitEvent(key string, committed bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, exists := d.limitEvents[key]
	if !exists {
		return
	}
	if committed {
		d.limitEvents[key] = dynamicLimitEventCommitted
		return
	}
	if state == dynamicLimitEventInFlight {
		delete(d.limitEvents, key)
	}
}

func (d *dynamicAgentTracker) MarkLimitEventCommitted(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limitEvents[key] = dynamicLimitEventCommitted
}

func (d *dynamicAgentTracker) CancelAll() {
	d.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(d.agents))
	for _, cancel := range d.agents {
		cancels = append(cancels, cancel)
	}
	d.mu.Unlock()
	for _, cancel := range cancels {
		if cancel != nil {
			cancel()
		}
	}
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
