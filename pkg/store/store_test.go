package store

import (
	"sync"
	"testing"
	"time"
)

// snap creates a minimal Snapshot at the given time.
func snap(t time.Time) Snapshot {
	return Snapshot{Timestamp: t}
}

func TestAddAndLatest(t *testing.T) {
	s := NewStore(0)

	if _, ok := s.Latest(); ok {
		t.Fatal("expected empty store to return ok=false")
	}

	now := time.Now()
	s.Add(snap(now))

	got, ok := s.Latest()
	if !ok {
		t.Fatal("expected ok=true after Add")
	}
	if !got.Timestamp.Equal(now) {
		t.Fatalf("got timestamp %v, want %v", got.Timestamp, now)
	}
}

func TestLen(t *testing.T) {
	s := NewStore(0)
	if s.Len() != 0 {
		t.Fatalf("empty store Len = %d, want 0", s.Len())
	}
	now := time.Now()
	for i := 0; i < 5; i++ {
		s.Add(snap(now.Add(time.Duration(i) * time.Second)))
	}
	if s.Len() != 5 {
		t.Fatalf("Len = %d, want 5", s.Len())
	}
}

func TestRetentionEviction(t *testing.T) {
	retention := 10 * time.Second
	s := NewStore(retention)

	now := time.Now()
	// Add entries spanning 30 seconds into the past.
	for i := 30; i >= 0; i-- {
		s.snapshots = append(s.snapshots, snap(now.Add(-time.Duration(i)*time.Second)))
	}

	// Manually evict — entries older than 10s should be removed.
	s.Evict()

	for _, sn := range s.snapshots {
		age := now.Sub(sn.Timestamp)
		if age > retention+time.Second { // 1s tolerance for test execution time
			t.Fatalf("snapshot age %v exceeds retention %v", age, retention)
		}
	}

	if s.Len() == 0 {
		t.Fatal("expected some snapshots to survive eviction")
	}
}

func TestAddEvictsAutomatically(t *testing.T) {
	retention := 5 * time.Second
	s := NewStore(retention)

	// Insert old entries directly to simulate passage of time.
	old := time.Now().Add(-10 * time.Second)
	s.snapshots = append(s.snapshots, snap(old))

	// Adding a current entry should evict the old one.
	s.Add(snap(time.Now()))

	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1 after eviction", s.Len())
	}
}

func TestZeroRetentionKeepsAll(t *testing.T) {
	s := NewStore(0)

	ancient := time.Now().Add(-365 * 24 * time.Hour)
	s.Add(snap(ancient))
	s.Add(snap(time.Now()))

	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (zero retention keeps all)", s.Len())
	}
}

func TestRangeInclusive(t *testing.T) {
	s := NewStore(0)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		s.Add(snap(base.Add(time.Duration(i) * time.Minute)))
	}

	from := base.Add(3 * time.Minute)
	to := base.Add(6 * time.Minute)
	got := s.Range(from, to)

	if len(got) != 4 {
		t.Fatalf("Range returned %d snapshots, want 4 (minutes 3,4,5,6)", len(got))
	}
	if !got[0].Timestamp.Equal(from) {
		t.Fatalf("first snapshot time = %v, want %v", got[0].Timestamp, from)
	}
	if !got[len(got)-1].Timestamp.Equal(to) {
		t.Fatalf("last snapshot time = %v, want %v", got[len(got)-1].Timestamp, to)
	}
}

func TestRangeEmpty(t *testing.T) {
	s := NewStore(0)
	from := time.Now()
	to := from.Add(time.Hour)

	if got := s.Range(from, to); got != nil {
		t.Fatalf("Range on empty store returned %d snapshots, want nil", len(got))
	}
}

func TestRangeNoMatch(t *testing.T) {
	s := NewStore(0)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Add(snap(base))

	// Query a range entirely after the single snapshot.
	from := base.Add(time.Hour)
	to := base.Add(2 * time.Hour)
	if got := s.Range(from, to); got != nil {
		t.Fatalf("Range returned %d snapshots, want nil", len(got))
	}
}

func TestRangeSingleMatch(t *testing.T) {
	s := NewStore(0)
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	s.Add(snap(ts))

	got := s.Range(ts, ts)
	if len(got) != 1 {
		t.Fatalf("Range returned %d, want 1 for exact match", len(got))
	}
}

func TestRangeReturnsCopy(t *testing.T) {
	s := NewStore(0)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Add(snap(base))
	s.Add(snap(base.Add(time.Minute)))

	got := s.Range(base, base.Add(time.Hour))
	// Mutate returned slice — should not affect the store.
	got[0].Timestamp = time.Time{}

	snap2, _ := s.Latest()
	internal := s.Range(base, base.Add(time.Hour))
	if internal[0].Timestamp.IsZero() {
		t.Fatal("Range returned a reference to internal state, not a copy")
	}
	_ = snap2
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore(time.Minute)
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ts := time.Now().Add(time.Duration(i) * time.Millisecond)
			s.Add(snap(ts))
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Latest()
			s.Len()
			s.Range(time.Now().Add(-time.Hour), time.Now())
		}()
	}

	wg.Wait()

	if s.Len() != 50 {
		t.Fatalf("Len = %d, want 50 after concurrent writes", s.Len())
	}
}

func TestHistoryBackwardCompat(t *testing.T) {
	s := New()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Push(snap(base))
	s.Push(snap(base.Add(time.Minute)))

	got := s.History(base, base.Add(time.Hour))
	if len(got) != 2 {
		t.Fatalf("History returned %d, want 2", len(got))
	}
}

func TestLatestSnapshotBackwardCompat(t *testing.T) {
	s := New()
	if s.LatestSnapshot() != nil {
		t.Fatal("expected nil on empty store")
	}
	now := time.Now()
	s.Push(snap(now))
	got := s.LatestSnapshot()
	if got == nil || !got.Timestamp.Equal(now) {
		t.Fatalf("LatestSnapshot returned %v, want %v", got, now)
	}
}

func TestConvenienceAccessors(t *testing.T) {
	s := NewStore(0)

	// Empty store returns nil for all accessors.
	if s.Cluster() != nil {
		t.Fatal("Cluster should be nil on empty store")
	}
	if s.Nodes() != nil {
		t.Fatal("Nodes should be nil on empty store")
	}
	if s.Pods() != nil {
		t.Fatal("Pods should be nil on empty store")
	}
	if s.Node("x") != nil {
		t.Fatal("Node should be nil on empty store")
	}

	s.Add(Snapshot{
		Timestamp: time.Now(),
		Cluster:   ClusterSummary{Nodes: 3, Healthy: true},
		Nodes: []NodeMetrics{
			{Name: "node-1", Ready: true},
			{Name: "node-2", Ready: false},
		},
		Pods: []PodMetrics{
			{Name: "pod-a", Namespace: "default", Node: "node-1"},
			{Name: "pod-b", Namespace: "kube-system", Node: "node-1"},
			{Name: "pod-c", Namespace: "default", Node: "node-2"},
		},
	})

	if c := s.Cluster(); c == nil || c.Nodes != 3 {
		t.Fatal("Cluster() mismatch")
	}
	if len(s.Nodes()) != 2 {
		t.Fatalf("Nodes() = %d, want 2", len(s.Nodes()))
	}
	if n := s.Node("node-1"); n == nil || !n.Ready {
		t.Fatal("Node(node-1) not found or not ready")
	}
	if n := s.Node("nonexistent"); n != nil {
		t.Fatal("Node(nonexistent) should be nil")
	}
	if len(s.PodsByNamespace("default")) != 2 {
		t.Fatal("PodsByNamespace(default) mismatch")
	}
	if len(s.PodsByNode("node-1")) != 2 {
		t.Fatal("PodsByNode(node-1) mismatch")
	}
}

func TestClusterSummaryDetailedFields(t *testing.T) {
	s := NewStore(0)

	s.Add(Snapshot{
		Timestamp: time.Now(),
		Cluster: ClusterSummary{
			Nodes: 3, Healthy: true, Severity: "ok",
			TotalNodes: 3, ReadyNodes: 2,
			TotalCPUMilli: 12000, UsedCPUMilli: 6500,
			TotalMemBytes: 64e9, UsedMemBytes: 32e9,
			TotalPods: 50, RunningPods: 45, PendingPods: 3, FailedPods: 2,
			AllocatedCPU: 8000, AllocatedMem: 48e9,
		},
	})

	c := s.Cluster()
	if c == nil {
		t.Fatal("expected non-nil Cluster")
	}
	if c.TotalNodes != 3 || c.ReadyNodes != 2 {
		t.Fatalf("TotalNodes=%d ReadyNodes=%d, want 3, 2", c.TotalNodes, c.ReadyNodes)
	}
	if c.TotalCPUMilli != 12000 || c.UsedCPUMilli != 6500 {
		t.Fatalf("CPU milli mismatch: total=%d used=%d", c.TotalCPUMilli, c.UsedCPUMilli)
	}
	if c.TotalMemBytes != 64e9 || c.UsedMemBytes != 32e9 {
		t.Fatal("memory bytes mismatch")
	}
	if c.RunningPods != 45 || c.PendingPods != 3 || c.FailedPods != 2 {
		t.Fatal("pod count mismatch")
	}
	if c.AllocatedCPU != 8000 || c.AllocatedMem != 48e9 {
		t.Fatal("allocated resources mismatch")
	}
}

func TestNodeHistory(t *testing.T) {
	s := NewStore(0)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		s.Add(Snapshot{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Nodes: []NodeMetrics{
				{Name: "node-1", CPUUsage: float64(i) * 0.1},
				{Name: "node-2", CPUUsage: float64(i) * 0.2},
			},
		})
	}

	// Query node-1 history for minutes 1-3.
	got := s.NodeHistory("node-1", base.Add(time.Minute), base.Add(3*time.Minute))
	if len(got) != 3 {
		t.Fatalf("NodeHistory returned %d entries, want 3", len(got))
	}
	if got[0].CPUUsage != 0.1 {
		t.Fatalf("first entry CPUUsage = %f, want 0.1", got[0].CPUUsage)
	}

	// Non-existent node returns empty.
	if got := s.NodeHistory("node-99", base, base.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("expected 0 for non-existent node, got %d", len(got))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	// Stress test: writers and range-readers running simultaneously.
	s := NewStore(time.Minute)
	var wg sync.WaitGroup
	base := time.Now()

	// 100 concurrent writers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Add(Snapshot{
				Timestamp: base.Add(time.Duration(i) * time.Millisecond),
				Nodes:     []NodeMetrics{{Name: "n", CPUUsage: float64(i)}},
			})
		}(i)
	}

	// 100 concurrent readers doing range queries.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Range(base.Add(-time.Hour), base.Add(time.Hour))
			_ = s.Len()
			_, _ = s.Latest()
			_ = s.Cluster()
			_ = s.Nodes()
			_ = s.Pods()
		}()
	}

	wg.Wait()

	if s.Len() != 100 {
		t.Fatalf("Len = %d, want 100", s.Len())
	}
}

func TestEvictLargeSlice(t *testing.T) {
	retention := time.Second
	s := NewStore(retention)

	now := time.Now()
	// Add 1000 old entries and 10 recent ones.
	for i := 1000; i > 0; i-- {
		s.snapshots = append(s.snapshots, snap(now.Add(-time.Duration(i)*time.Minute)))
	}
	for i := 0; i < 10; i++ {
		s.snapshots = append(s.snapshots, snap(now.Add(-time.Duration(i)*100*time.Millisecond)))
	}

	s.Evict()

	if s.Len() != 10 {
		t.Fatalf("after eviction Len = %d, want 10", s.Len())
	}
}
