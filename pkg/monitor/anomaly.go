package monitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

// AnomalyKind classifies the type of anomaly detected.
type AnomalyKind string

const (
	// Node pressure conditions — reported by kubelet when resources are scarce.
	AnomalyMemoryPressure AnomalyKind = "node.memory_pressure"
	AnomalyDiskPressure   AnomalyKind = "node.disk_pressure"
	AnomalyPIDPressure    AnomalyKind = "node.pid_pressure"
	AnomalyNodeNotReady   AnomalyKind = "node.not_ready"

	// Pod lifecycle anomalies.
	AnomalyPodEvicted AnomalyKind = "pod.evicted"

	// Resource contention — cluster-level aggregate when multiple nodes are stressed.
	AnomalyResourceContention AnomalyKind = "cluster.resource_contention"
)

// Anomaly represents a detected cluster anomaly. Unlike threshold-based alerts
// (which track individual metric values), anomalies capture conditions, state
// changes, and aggregate patterns across the cluster.
type Anomaly struct {
	Kind       AnomalyKind  `json:"kind"`
	Severity   AlertState   `json:"severity"` // warning or critical
	NodeName   string       `json:"node_name,omitempty"`
	PodName    string       `json:"pod_name,omitempty"`
	Namespace  string       `json:"namespace,omitempty"`
	Resource   ResourceKind `json:"resource,omitempty"` // set for resource contention
	Value      float64      `json:"value,omitempty"`
	Threshold  float64      `json:"threshold,omitempty"`
	Message    string       `json:"message"`
	DetectedAt time.Time    `json:"detected_at"`
}

// ContentionThreshold is the fraction of nodes (0.0-1.0) that must exceed the
// warning threshold before cluster-level resource contention is reported.
// Default: 0.5 (50% of nodes).
const DefaultContentionRatio = 0.5

// AnomalyDetector scans cluster snapshots for anomalous conditions beyond
// simple threshold breaches. It detects node pressure conditions, pod evictions,
// and cluster-wide resource contention.
//
// It is safe for concurrent use.
type AnomalyDetector struct {
	mu              sync.RWMutex
	thresholds      config.AlertThreshold
	contentionRatio float64 // fraction of nodes above threshold that triggers contention
	anomalies       []Anomaly
}

// NewAnomalyDetector returns a detector configured with the given thresholds.
func NewAnomalyDetector(thresholds config.AlertThreshold) *AnomalyDetector {
	return &AnomalyDetector{
		thresholds:      thresholds,
		contentionRatio: DefaultContentionRatio,
	}
}

// SetContentionRatio sets the fraction of nodes that must be above warning
// threshold to trigger a resource contention anomaly. Must be 0.0-1.0.
func (d *AnomalyDetector) SetContentionRatio(ratio float64) error {
	if ratio < 0 || ratio > 1 {
		return fmt.Errorf("contention ratio must be 0.0-1.0, got %.3f", ratio)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.contentionRatio = ratio
	return nil
}

// Anomalies returns a copy of the most recently detected anomalies.
func (d *AnomalyDetector) Anomalies() []Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Anomaly, len(d.anomalies))
	copy(out, d.anomalies)
	return out
}

// Detect scans a snapshot for anomalous conditions and returns all detected
// anomalies. It replaces the internal anomaly set each call.
func (d *AnomalyDetector) Detect(snap store.Snapshot) []Anomaly {
	now := time.Now()
	var out []Anomaly

	for _, n := range snap.Nodes {
		out = append(out, d.detectNodeConditions(n, now)...)
	}

	for _, p := range snap.Pods {
		out = append(out, d.detectPodEviction(p, now)...)
	}

	out = append(out, d.detectResourceContention(snap.Nodes, now)...)

	d.mu.Lock()
	d.anomalies = out
	d.mu.Unlock()

	return out
}

// ── Node condition detection ───────────────────────────────────────────

// detectNodeConditions checks kubelet-reported pressure conditions and readiness.
func (d *AnomalyDetector) detectNodeConditions(n store.NodeMetrics, now time.Time) []Anomaly {
	var out []Anomaly
	conditions := conditionSet(n.Conditions)

	if conditions["MemoryPressure"] {
		out = append(out, Anomaly{
			Kind:       AnomalyMemoryPressure,
			Severity:   AlertCritical,
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s has MemoryPressure condition", n.Name),
			DetectedAt: now,
		})
	}

	if conditions["DiskPressure"] {
		out = append(out, Anomaly{
			Kind:       AnomalyDiskPressure,
			Severity:   AlertCritical,
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s has DiskPressure condition", n.Name),
			DetectedAt: now,
		})
	}

	if conditions["PIDPressure"] {
		out = append(out, Anomaly{
			Kind:       AnomalyPIDPressure,
			Severity:   AlertWarning,
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s has PIDPressure condition", n.Name),
			DetectedAt: now,
		})
	}

	if !n.Ready {
		out = append(out, Anomaly{
			Kind:       AnomalyNodeNotReady,
			Severity:   AlertCritical,
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s is not ready", n.Name),
			DetectedAt: now,
		})
	}

	return out
}

// ── Pod eviction detection ─────────────────────────────────────────────

// detectPodEviction checks for pods that were evicted by kubelet.
func (d *AnomalyDetector) detectPodEviction(p store.PodMetrics, now time.Time) []Anomaly {
	if p.Phase != "Failed" || p.Reason != "Evicted" {
		return nil
	}

	return []Anomaly{{
		Kind:       AnomalyPodEvicted,
		Severity:   AlertWarning,
		NodeName:   p.Node,
		PodName:    p.Name,
		Namespace:  p.Namespace,
		Message:    fmt.Sprintf("pod %s/%s was evicted from node %s", p.Namespace, p.Name, p.Node),
		DetectedAt: now,
	}}
}

// ── Resource contention detection ──────────────────────────────────────

// detectResourceContention fires when a significant fraction of nodes exceed
// the warning threshold for a resource. This is a cluster-level signal — it
// means the problem isn't isolated to one node.
func (d *AnomalyDetector) detectResourceContention(nodes []store.NodeMetrics, now time.Time) []Anomaly {
	total := len(nodes)
	if total == 0 {
		return nil
	}

	d.mu.RLock()
	ratio := d.contentionRatio
	t := d.thresholds
	d.mu.RUnlock()

	// Minimum nodes required: at least 2 must be stressed for contention.
	minStressed := max(2, int(float64(total)*ratio+0.5))

	var out []Anomaly

	out = append(out, d.checkContention(nodes, total, minStressed, ResourceCPU, t.CPUWarning, t.CPUCritical,
		func(n store.NodeMetrics) float64 { return n.CPUUsage }, now)...)

	out = append(out, d.checkContention(nodes, total, minStressed, ResourceMemory, t.MemWarning, t.MemCritical,
		func(n store.NodeMetrics) float64 { return n.MemoryUsage }, now)...)

	out = append(out, d.checkContention(nodes, total, minStressed, ResourceDisk, t.DiskWarning, t.DiskCritical,
		func(n store.NodeMetrics) float64 { return n.DiskUsage }, now)...)

	return out
}

// checkContention checks whether enough nodes exceed the warning threshold for
// a given resource to constitute cluster-wide contention. If the majority of
// stressed nodes are above the critical threshold, severity is critical.
func (d *AnomalyDetector) checkContention(
	nodes []store.NodeMetrics,
	total, minStressed int,
	resource ResourceKind,
	warn, crit float64,
	getValue func(store.NodeMetrics) float64,
	now time.Time,
) []Anomaly {
	var stressedCount, critCount int
	var sumValue float64

	for _, n := range nodes {
		v := getValue(n)
		if v < 0 {
			continue // skip unavailable metrics (disk = -1)
		}
		if v >= warn {
			stressedCount++
			sumValue += v
			if v >= crit {
				critCount++
			}
		}
	}

	if stressedCount < minStressed {
		return nil
	}

	avgValue := sumValue / float64(stressedCount)
	severity := AlertWarning
	if critCount > stressedCount/2 {
		severity = AlertCritical
	}

	return []Anomaly{{
		Kind:      AnomalyResourceContention,
		Severity:  severity,
		Resource:  resource,
		Value:     avgValue,
		Threshold: warn,
		Message: fmt.Sprintf("cluster %s contention: %d/%d nodes above %.0f%% threshold (avg %.1f%%)",
			resource, stressedCount, total, warn*100, avgValue*100),
		DetectedAt: now,
	}}
}

// ── Helpers ────────────────────────────────────────────────────────────

// conditionSet converts a slice of condition strings to a set for O(1) lookup.
func conditionSet(conditions []string) map[string]bool {
	m := make(map[string]bool, len(conditions))
	for _, c := range conditions {
		m[c] = true
	}
	return m
}
