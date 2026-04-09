package monitor

import (
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

func defaultDetector() *AnomalyDetector {
	return NewAnomalyDetector(config.DefaultAlertThreshold())
}

func baseSnapshot() store.Snapshot {
	return store.Snapshot{Timestamp: time.Now()}
}

// ── Node condition tests ───────────────────────────────────────────────

func TestDetect_MemoryPressure(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "MemoryPressure"}},
	}

	anomalies := d.Detect(snap)
	a := findAnomaly(anomalies, AnomalyMemoryPressure)
	if a == nil {
		t.Fatal("expected MemoryPressure anomaly")
	}
	if a.Severity != AlertCritical {
		t.Errorf("severity = %s, want critical", a.Severity)
	}
	if a.NodeName != "node-1" {
		t.Errorf("node = %s, want node-1", a.NodeName)
	}
}

func TestDetect_DiskPressure(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "DiskPressure"}},
	}

	anomalies := d.Detect(snap)
	a := findAnomaly(anomalies, AnomalyDiskPressure)
	if a == nil {
		t.Fatal("expected DiskPressure anomaly")
	}
	if a.Severity != AlertCritical {
		t.Errorf("severity = %s, want critical", a.Severity)
	}
}

func TestDetect_PIDPressure(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "PIDPressure"}},
	}

	anomalies := d.Detect(snap)
	a := findAnomaly(anomalies, AnomalyPIDPressure)
	if a == nil {
		t.Fatal("expected PIDPressure anomaly")
	}
	if a.Severity != AlertWarning {
		t.Errorf("severity = %s, want warning", a.Severity)
	}
}

func TestDetect_NodeNotReady(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: false, Conditions: []string{}},
	}

	anomalies := d.Detect(snap)
	a := findAnomaly(anomalies, AnomalyNodeNotReady)
	if a == nil {
		t.Fatal("expected NodeNotReady anomaly")
	}
	if a.Severity != AlertCritical {
		t.Errorf("severity = %s, want critical", a.Severity)
	}
}

func TestDetect_MultiplePressureConditions(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "MemoryPressure", "DiskPressure"}},
	}

	anomalies := d.Detect(snap)

	if findAnomaly(anomalies, AnomalyMemoryPressure) == nil {
		t.Error("expected MemoryPressure anomaly")
	}
	if findAnomaly(anomalies, AnomalyDiskPressure) == nil {
		t.Error("expected DiskPressure anomaly")
	}
}

func TestDetect_HealthyNode_NoAnomalies(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.40, DiskUsage: 0.50,
			Conditions: []string{"Ready"}},
	}

	anomalies := d.Detect(snap)
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(anomalies))
	}
}

// ── Pod eviction tests ─────────────────────────────────────────────────

func TestDetect_PodEvicted(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Failed", Reason: "Evicted"},
	}

	anomalies := d.Detect(snap)
	a := findAnomaly(anomalies, AnomalyPodEvicted)
	if a == nil {
		t.Fatal("expected PodEvicted anomaly")
	}
	if a.PodName != "web-1" {
		t.Errorf("pod = %s, want web-1", a.PodName)
	}
	if a.Namespace != "default" {
		t.Errorf("namespace = %s, want default", a.Namespace)
	}
	if a.NodeName != "node-1" {
		t.Errorf("node = %s, want node-1", a.NodeName)
	}
	if a.Severity != AlertWarning {
		t.Errorf("severity = %s, want warning", a.Severity)
	}
}

func TestDetect_PodEvicted_NotTriggeredOnRunning(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Running"},
	}

	anomalies := d.Detect(snap)
	if findAnomaly(anomalies, AnomalyPodEvicted) != nil {
		t.Error("expected no PodEvicted for running pod")
	}
}

func TestDetect_PodEvicted_NotTriggeredOnOOMKill(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Failed", Reason: "OOMKilled"},
	}

	anomalies := d.Detect(snap)
	if findAnomaly(anomalies, AnomalyPodEvicted) != nil {
		t.Error("expected no PodEvicted for OOMKilled pod")
	}
}

// ── Resource contention tests ──────────────────────────────────────────

func TestDetect_ResourceContention_CPU(t *testing.T) {
	d := defaultDetector()
	// 3 out of 4 nodes above CPU warning (0.80) = contention
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: true, CPUUsage: 0.85, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n2", Ready: true, CPUUsage: 0.88, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n3", Ready: true, CPUUsage: 0.90, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n4", Ready: true, CPUUsage: 0.20, MemoryUsage: 0.30, DiskUsage: 0.30},
	}

	anomalies := d.Detect(snap)
	var contention *Anomaly
	for i := range anomalies {
		if anomalies[i].Kind == AnomalyResourceContention && anomalies[i].Resource == ResourceCPU {
			contention = &anomalies[i]
			break
		}
	}
	if contention == nil {
		t.Fatal("expected CPU resource contention anomaly")
	}
	if contention.Severity != AlertWarning {
		t.Errorf("severity = %s, want warning", contention.Severity)
	}
}

func TestDetect_ResourceContention_Critical(t *testing.T) {
	d := defaultDetector()
	// All 3 nodes above critical (0.95) — majority critical = critical severity
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: true, CPUUsage: 0.96, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n2", Ready: true, CPUUsage: 0.97, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n3", Ready: true, CPUUsage: 0.98, MemoryUsage: 0.30, DiskUsage: 0.30},
	}

	anomalies := d.Detect(snap)
	var contention *Anomaly
	for i := range anomalies {
		if anomalies[i].Kind == AnomalyResourceContention && anomalies[i].Resource == ResourceCPU {
			contention = &anomalies[i]
			break
		}
	}
	if contention == nil {
		t.Fatal("expected CPU resource contention anomaly")
	}
	if contention.Severity != AlertCritical {
		t.Errorf("severity = %s, want critical", contention.Severity)
	}
}

func TestDetect_ResourceContention_Memory(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.85, DiskUsage: 0.30},
		{Name: "n2", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.90, DiskUsage: 0.30},
		{Name: "n3", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.20, DiskUsage: 0.30},
	}

	anomalies := d.Detect(snap)
	var contention *Anomaly
	for i := range anomalies {
		if anomalies[i].Kind == AnomalyResourceContention && anomalies[i].Resource == ResourceMemory {
			contention = &anomalies[i]
			break
		}
	}
	if contention == nil {
		t.Fatal("expected memory resource contention anomaly")
	}
}

func TestDetect_ResourceContention_NotTriggeredSingleNode(t *testing.T) {
	d := defaultDetector()
	// Only 1 node above threshold — contention requires at least 2
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: true, CPUUsage: 0.96, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n2", Ready: true, CPUUsage: 0.20, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n3", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.30, DiskUsage: 0.30},
	}

	anomalies := d.Detect(snap)
	for _, a := range anomalies {
		if a.Kind == AnomalyResourceContention {
			t.Errorf("unexpected contention anomaly: %s", a.Message)
		}
	}
}

func TestDetect_ResourceContention_DiskSkipsUnavailable(t *testing.T) {
	d := defaultDetector()
	// 2 nodes with disk=-1, 1 with high disk — should not trigger contention
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.30, DiskUsage: -1},
		{Name: "n2", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.30, DiskUsage: -1},
		{Name: "n3", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.30, DiskUsage: 0.92},
	}

	anomalies := d.Detect(snap)
	for _, a := range anomalies {
		if a.Kind == AnomalyResourceContention && a.Resource == ResourceDisk {
			t.Error("expected no disk contention when most nodes have unavailable metrics")
		}
	}
}

func TestDetect_ResourceContention_NoNodes(t *testing.T) {
	d := defaultDetector()
	anomalies := d.Detect(store.Snapshot{})
	for _, a := range anomalies {
		if a.Kind == AnomalyResourceContention {
			t.Error("expected no contention anomaly for empty snapshot")
		}
	}
}

// ── Contention ratio configuration ─────────────────────────────────────

func TestSetContentionRatio_Valid(t *testing.T) {
	d := defaultDetector()
	if err := d.SetContentionRatio(0.75); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetContentionRatio_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
	}{
		{"negative", -0.1},
		{"above 1", 1.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := defaultDetector()
			if err := d.SetContentionRatio(tt.ratio); err == nil {
				t.Error("expected error for invalid ratio")
			}
		})
	}
}

func TestSetContentionRatio_AffectsDetection(t *testing.T) {
	d := defaultDetector()
	// With default 0.5 ratio and 4 nodes, need 2 stressed. Only 2 are stressed → triggers.
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: true, CPUUsage: 0.85, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n2", Ready: true, CPUUsage: 0.88, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n3", Ready: true, CPUUsage: 0.20, MemoryUsage: 0.30, DiskUsage: 0.30},
		{Name: "n4", Ready: true, CPUUsage: 0.20, MemoryUsage: 0.30, DiskUsage: 0.30},
	}

	anomalies := d.Detect(snap)
	if findContentionForResource(anomalies, ResourceCPU) == nil {
		t.Fatal("expected CPU contention with default ratio")
	}

	// Raise ratio to 0.9 — now need 4 stressed out of 4. Should not trigger.
	if err := d.SetContentionRatio(0.9); err != nil {
		t.Fatal(err)
	}
	anomalies = d.Detect(snap)
	if findContentionForResource(anomalies, ResourceCPU) != nil {
		t.Error("expected no CPU contention with 0.9 ratio")
	}
}

// ── State management ───────────────────────────────────────────────────

func TestAnomalies_ReturnsCopy(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: false},
	}

	d.Detect(snap)
	a1 := d.Anomalies()
	a2 := d.Anomalies()

	if len(a1) == 0 {
		t.Fatal("expected anomalies")
	}

	a1[0].NodeName = "mutated"
	if a2[0].NodeName == "mutated" {
		t.Error("Anomalies() should return a copy")
	}
}

func TestDetect_ReplacesAnomalies(t *testing.T) {
	d := defaultDetector()

	// First: unhealthy
	snap1 := baseSnapshot()
	snap1.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: false},
	}
	d.Detect(snap1)
	if len(d.Anomalies()) == 0 {
		t.Fatal("expected anomalies after first detect")
	}

	// Second: healthy — anomalies should be replaced, not accumulated
	snap2 := baseSnapshot()
	snap2.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, CPUUsage: 0.30, MemoryUsage: 0.30, DiskUsage: 0.30,
			Conditions: []string{"Ready"}},
	}
	d.Detect(snap2)
	if len(d.Anomalies()) != 0 {
		t.Errorf("expected 0 anomalies after healthy snapshot, got %d", len(d.Anomalies()))
	}
}

// ── Empty/edge cases ───────────────────────────────────────────────────

func TestDetect_EmptySnapshot(t *testing.T) {
	d := defaultDetector()
	anomalies := d.Detect(store.Snapshot{})
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies for empty snapshot, got %d", len(anomalies))
	}
}

func TestDetect_FullClusterAnomaly(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "n1", Ready: false, CPUUsage: 0.96, MemoryUsage: 0.97, DiskUsage: 0.98,
			Conditions: []string{"MemoryPressure", "DiskPressure"}},
		{Name: "n2", Ready: false, CPUUsage: 0.97, MemoryUsage: 0.98, DiskUsage: 0.99,
			Conditions: []string{"MemoryPressure", "DiskPressure", "PIDPressure"}},
	}
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "n1", Phase: "Failed", Reason: "Evicted"},
	}

	anomalies := d.Detect(snap)

	// Should have: 2x MemPressure, 2x DiskPressure, 1x PIDPressure, 2x NotReady,
	// 1x PodEvicted, and contention for CPU+Mem+Disk (3 resources)
	if len(anomalies) < 10 {
		t.Errorf("expected at least 10 anomalies for a failing cluster, got %d", len(anomalies))
	}

	// Verify contention detected for all three resources
	if findContentionForResource(anomalies, ResourceCPU) == nil {
		t.Error("expected CPU contention")
	}
	if findContentionForResource(anomalies, ResourceMemory) == nil {
		t.Error("expected memory contention")
	}
	if findContentionForResource(anomalies, ResourceDisk) == nil {
		t.Error("expected disk contention")
	}
}

// ── Helpers ────────────────────────────────────────────────────────────

func findAnomaly(anomalies []Anomaly, kind AnomalyKind) *Anomaly {
	for i := range anomalies {
		if anomalies[i].Kind == kind {
			return &anomalies[i]
		}
	}
	return nil
}

func findContentionForResource(anomalies []Anomaly, res ResourceKind) *Anomaly {
	for i := range anomalies {
		if anomalies[i].Kind == AnomalyResourceContention && anomalies[i].Resource == res {
			return &anomalies[i]
		}
	}
	return nil
}
