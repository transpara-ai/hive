package anomaly

import (
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

func defaultDetector() *Detector {
	return NewDetector(config.DefaultAlertThreshold())
}

func baseSnapshot() store.Snapshot {
	return store.Snapshot{Timestamp: time.Now()}
}

func TestDetect_NodeMemoryPressure(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "MemoryPressure"}},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyNodeMemPressure)
	if found == nil {
		t.Fatal("expected MemoryPressure anomaly")
	}
	if found.Severity != "critical" {
		t.Errorf("expected critical severity, got %s", found.Severity)
	}
	if found.NodeName != "node-1" {
		t.Errorf("expected node-1, got %s", found.NodeName)
	}
}

func TestDetect_NodeDiskPressure(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "DiskPressure"}},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyNodeDiskPressure)
	if found == nil {
		t.Fatal("expected DiskPressure anomaly")
	}
	if found.Severity != "critical" {
		t.Errorf("expected critical severity, got %s", found.Severity)
	}
}

func TestDetect_NodePIDPressure(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, Conditions: []string{"Ready", "PIDPressure"}},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyNodePIDPressure)
	if found == nil {
		t.Fatal("expected PIDPressure anomaly")
	}
	if found.Severity != "warning" {
		t.Errorf("expected warning severity, got %s", found.Severity)
	}
}

func TestDetect_NodeNotReady(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: false, Conditions: []string{}},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyNodeNotReady)
	if found == nil {
		t.Fatal("expected NodeNotReady anomaly")
	}
	if found.Severity != "critical" {
		t.Errorf("expected critical severity, got %s", found.Severity)
	}
}

func TestDetect_HighCPU_Warning(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, CPUUsage: 0.85},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighCPU)
	if found == nil {
		t.Fatal("expected HighCPU anomaly")
	}
	if found.Severity != "warning" {
		t.Errorf("expected warning, got %s", found.Severity)
	}
	if found.Value != 0.85 {
		t.Errorf("expected value 0.85, got %f", found.Value)
	}
}

func TestDetect_HighCPU_Critical(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, CPUUsage: 0.96},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighCPU)
	if found == nil {
		t.Fatal("expected HighCPU anomaly")
	}
	if found.Severity != "critical" {
		t.Errorf("expected critical, got %s", found.Severity)
	}
}

func TestDetect_HighMem_Warning(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, MemoryUsage: 0.82},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighMem)
	if found == nil {
		t.Fatal("expected HighMem anomaly")
	}
	if found.Severity != "warning" {
		t.Errorf("expected warning, got %s", found.Severity)
	}
}

func TestDetect_HighMem_Critical(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, MemoryUsage: 0.97},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighMem)
	if found == nil {
		t.Fatal("expected HighMem anomaly")
	}
	if found.Severity != "critical" {
		t.Errorf("expected critical, got %s", found.Severity)
	}
}

func TestDetect_HighDisk_Warning(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, DiskUsage: 0.90},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighDisk)
	if found == nil {
		t.Fatal("expected HighDisk anomaly")
	}
	if found.Severity != "warning" {
		t.Errorf("expected warning, got %s", found.Severity)
	}
}

func TestDetect_HighDisk_Critical(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, DiskUsage: 0.96},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighDisk)
	if found == nil {
		t.Fatal("expected HighDisk anomaly")
	}
	if found.Severity != "critical" {
		t.Errorf("expected critical, got %s", found.Severity)
	}
}

func TestDetect_HighDisk_SkippedWhenNegative(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, DiskUsage: -1},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyHighDisk)
	if found != nil {
		t.Fatal("expected no HighDisk anomaly when disk=-1")
	}
}

func TestDetect_PodEvicted(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Failed", Reason: "Evicted"},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyPodEvicted)
	if found == nil {
		t.Fatal("expected PodEvicted anomaly")
	}
	if found.PodName != "web-1" {
		t.Errorf("expected pod web-1, got %s", found.PodName)
	}
	if found.Namespace != "default" {
		t.Errorf("expected namespace default, got %s", found.Namespace)
	}
}

func TestDetect_PodEvicted_NotTriggeredOnRunning(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Running", Reason: ""},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyPodEvicted)
	if found != nil {
		t.Fatal("expected no PodEvicted anomaly for running pod")
	}
}

func TestDetect_LimitExceeded_CPU(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Running", CPUUsage: 0.95, CPULimit: 1.0},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyLimitExceeded)
	if found == nil {
		t.Fatal("expected LimitExceeded anomaly for CPU")
	}
	if found.Value < 0.90 {
		t.Errorf("expected value >= 0.90, got %f", found.Value)
	}
}

func TestDetect_LimitExceeded_Memory(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "db-1", Namespace: "data", Node: "node-1", Phase: "Running",
			MemoryBytes: 950_000_000, MemoryLimit: 1_000_000_000},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyLimitExceeded)
	if found == nil {
		t.Fatal("expected LimitExceeded anomaly for memory")
	}
}

func TestDetect_LimitExceeded_NotTriggeredBelowThreshold(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Running",
			CPUUsage: 0.5, CPULimit: 1.0, MemoryBytes: 400_000_000, MemoryLimit: 1_000_000_000},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyLimitExceeded)
	if found != nil {
		t.Fatal("expected no LimitExceeded anomaly below threshold")
	}
}

func TestDetect_LimitExceeded_SkippedWhenNoLimit(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Running",
			CPUUsage: 2.0, CPULimit: 0, MemoryBytes: 8_000_000_000, MemoryLimit: 0},
	}

	anomalies := d.Detect(snap)
	found := findByType(anomalies, AnomalyLimitExceeded)
	if found != nil {
		t.Fatal("expected no LimitExceeded anomaly when no limit set")
	}
}

func TestDetect_NoAnomalies_HealthyCluster(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, CPUUsage: 0.3, MemoryUsage: 0.4, DiskUsage: 0.5},
	}
	snap.Pods = []store.PodMetrics{
		{Name: "web-1", Namespace: "default", Node: "node-1", Phase: "Running",
			CPUUsage: 0.1, CPULimit: 1.0, MemoryBytes: 100_000_000, MemoryLimit: 1_000_000_000},
	}

	anomalies := d.Detect(snap)
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies for healthy cluster, got %d", len(anomalies))
	}
}

func TestDetect_EmptySnapshot(t *testing.T) {
	d := defaultDetector()
	anomalies := d.Detect(store.Snapshot{})
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies for empty snapshot, got %d", len(anomalies))
	}
}

func TestDetect_BelowWarningThreshold(t *testing.T) {
	d := defaultDetector()
	snap := baseSnapshot()
	snap.Nodes = []store.NodeMetrics{
		{Name: "node-1", Ready: true, CPUUsage: 0.79, MemoryUsage: 0.79, DiskUsage: 0.84},
	}

	anomalies := d.Detect(snap)
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies below threshold, got %d", len(anomalies))
	}
}

// findByType returns the first anomaly matching the given type, or nil.
func findByType(anomalies []Anomaly, t AnomalyType) *Anomaly {
	for i := range anomalies {
		if anomalies[i].Type == t {
			return &anomalies[i]
		}
	}
	return nil
}
