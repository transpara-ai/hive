package store

import (
	"sort"
	"sync"
	"time"
)

// Store holds a time-ordered history of cluster snapshots with configurable
// retention. Older entries are evicted automatically on each Add call.
// All methods are safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	snapshots []Snapshot
	retention time.Duration // 0 means keep forever
}

// NewStore returns a Store that evicts snapshots older than retention.
// A zero retention keeps all snapshots indefinitely.
func NewStore(retention time.Duration) *Store {
	return &Store{retention: retention}
}

// New returns a Store with no retention limit (keeps all snapshots).
// Deprecated: prefer NewStore.
func New() *Store { return NewStore(0) }

// NewInMemoryStore is an alias for NewStore(0).
func NewInMemoryStore() *Store { return NewStore(0) }

// Add appends a snapshot and evicts expired entries.
func (s *Store) Add(snap Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots = append(s.snapshots, snap)
	s.evictLocked()
}

// Push appends a snapshot to the history.
// Deprecated: prefer Add, which also runs eviction.
func (s *Store) Push(snap Snapshot) { s.Add(snap) }

// Latest returns the most recent snapshot.
// The bool is false when the store is empty.
func (s *Store) Latest() (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.snapshots) == 0 {
		return Snapshot{}, false
	}
	return s.snapshots[len(s.snapshots)-1], true
}

// LatestSnapshot returns the most recent snapshot, or nil if empty.
// Deprecated: prefer Latest.
func (s *Store) LatestSnapshot() *Snapshot {
	snap, ok := s.Latest()
	if !ok {
		return nil
	}
	return &snap
}

// Range returns snapshots with timestamps in [from, to] (inclusive).
// Uses binary search for O(log n) bound location.
func (s *Store) Range(from, to time.Time) []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.snapshots) == 0 {
		return nil
	}

	// Binary search: find first index where timestamp >= from.
	lo := sort.Search(len(s.snapshots), func(i int) bool {
		return !s.snapshots[i].Timestamp.Before(from)
	})
	// Binary search: find first index where timestamp > to.
	hi := sort.Search(len(s.snapshots), func(i int) bool {
		return s.snapshots[i].Timestamp.After(to)
	})

	if lo >= hi {
		return nil
	}

	// Return a copy so callers can't mutate internal state.
	out := make([]Snapshot, hi-lo)
	copy(out, s.snapshots[lo:hi])
	return out
}

// History returns snapshots within [from, to]. Both bounds are inclusive.
// Deprecated: prefer Range.
func (s *Store) History(from, to time.Time) []Snapshot { return s.Range(from, to) }

// Len returns the number of stored snapshots.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.snapshots)
}

// Evict removes entries older than the retention window.
// Called automatically by Add; may also be called explicitly.
func (s *Store) Evict() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictLocked()
}

// evictLocked removes stale entries. Caller must hold s.mu (write lock).
func (s *Store) evictLocked() {
	if s.retention <= 0 || len(s.snapshots) == 0 {
		return
	}
	cutoff := time.Now().Add(-s.retention)
	// Binary search: find first index where timestamp >= cutoff.
	idx := sort.Search(len(s.snapshots), func(i int) bool {
		return !s.snapshots[i].Timestamp.Before(cutoff)
	})
	if idx > 0 {
		// Slide remaining entries to front so the underlying array can be GC'd.
		n := copy(s.snapshots, s.snapshots[idx:])
		// Zero trailing slots to avoid retaining references.
		for i := n; i < len(s.snapshots); i++ {
			s.snapshots[i] = Snapshot{}
		}
		s.snapshots = s.snapshots[:n]
	}
}

// ── Convenience accessors (delegate to Latest) ────────────────────────

// Cluster returns the latest ClusterSummary, or nil if empty.
func (s *Store) Cluster() *ClusterSummary {
	snap, ok := s.Latest()
	if !ok {
		return nil
	}
	c := snap.Cluster
	return &c
}

// Nodes returns all NodeMetrics from the latest snapshot.
func (s *Store) Nodes() []NodeMetrics {
	snap, ok := s.Latest()
	if !ok {
		return nil
	}
	return snap.Nodes
}

// Node returns metrics for a named node, or nil if not found.
func (s *Store) Node(name string) *NodeMetrics {
	for _, n := range s.Nodes() {
		if n.Name == name {
			return &n
		}
	}
	return nil
}

// Pods returns all PodMetrics from the latest snapshot.
func (s *Store) Pods() []PodMetrics {
	snap, ok := s.Latest()
	if !ok {
		return nil
	}
	return snap.Pods
}

// PodsByNamespace returns pods filtered by namespace.
func (s *Store) PodsByNamespace(ns string) []PodMetrics {
	var result []PodMetrics
	for _, p := range s.Pods() {
		if p.Namespace == ns {
			result = append(result, p)
		}
	}
	return result
}

// PodsByNode returns pods filtered by node name.
func (s *Store) PodsByNode(node string) []PodMetrics {
	var result []PodMetrics
	for _, p := range s.Pods() {
		if p.Node == node {
			result = append(result, p)
		}
	}
	return result
}

// NodeHistory returns historical metrics for a specific node within [from, to].
func (s *Store) NodeHistory(name string, from, to time.Time) []NodeMetrics {
	snapshots := s.Range(from, to)
	var result []NodeMetrics
	for _, snap := range snapshots {
		for _, n := range snap.Nodes {
			if n.Name == name {
				result = append(result, n)
			}
		}
	}
	return result
}
