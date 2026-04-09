// Package anomaly provides threshold-based anomaly detection for cluster metrics.
package anomaly

import (
	"fmt"
	"sync"
	"time"

	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

// AnomalyType classifies the kind of anomaly detected.
type AnomalyType string

const (
	AnomalyNodeMemPressure  AnomalyType = "node.memory_pressure"
	AnomalyNodeDiskPressure AnomalyType = "node.disk_pressure"
	AnomalyNodePIDPressure  AnomalyType = "node.pid_pressure"
	AnomalyNodeNotReady     AnomalyType = "node.not_ready"
	AnomalyPodEvicted       AnomalyType = "pod.evicted"
	AnomalyHighCPU          AnomalyType = "resource.high_cpu"
	AnomalyHighMem          AnomalyType = "resource.high_mem"
	AnomalyHighDisk         AnomalyType = "resource.high_disk"
	AnomalyLimitExceeded    AnomalyType = "resource.limit_exceeded"
)

// Anomaly describes a detected issue in cluster metrics.
type Anomaly struct {
	Type       AnomalyType `json:"type"`
	Severity   string      `json:"severity"`    // "warning", "critical"
	NodeName   string      `json:"node_name"`   // empty if cluster-wide
	PodName    string      `json:"pod_name"`    // empty if node-level
	Namespace  string      `json:"namespace"`
	Message    string      `json:"message"`
	DetectedAt time.Time   `json:"detected_at"`
	Value      float64     `json:"value"`     // e.g. CPU% that triggered
	Threshold  float64     `json:"threshold"` // threshold that was exceeded
}

// Detector inspects snapshots and produces anomalies based on configured thresholds.
type Detector struct {
	mu         sync.RWMutex
	thresholds config.AlertThreshold
	anomalies  []Anomaly
}

// NewDetector returns a Detector configured with the given thresholds.
func NewDetector(thresholds config.AlertThreshold) *Detector {
	return &Detector{thresholds: thresholds}
}

// Record adds an anomaly to the detector's state.
func (d *Detector) Record(a Anomaly) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anomalies = append(d.anomalies, a)
}

// Clear removes all recorded anomalies.
func (d *Detector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anomalies = nil
}

// Anomalies returns a copy of the recorded anomalies.
func (d *Detector) Anomalies() []Anomaly {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Anomaly, len(d.anomalies))
	copy(out, d.anomalies)
	return out
}

// Detect inspects a snapshot and returns all detected anomalies.
func (d *Detector) Detect(snap store.Snapshot) []Anomaly {
	now := time.Now()
	var out []Anomaly

	for _, n := range snap.Nodes {
		out = append(out, d.detectNodeConditions(n, now)...)
		out = append(out, d.detectNodeResources(n, now)...)
	}

	for _, p := range snap.Pods {
		out = append(out, d.detectPodEviction(p, now)...)
		out = append(out, d.detectPodLimitExceeded(p, now)...)
	}

	return out
}

// detectNodeConditions checks for pressure conditions and not-ready state.
func (d *Detector) detectNodeConditions(n store.NodeMetrics, now time.Time) []Anomaly {
	var out []Anomaly

	conditions := conditionSet(n.Conditions)

	if conditions["MemoryPressure"] {
		out = append(out, Anomaly{
			Type:       AnomalyNodeMemPressure,
			Severity:   "critical",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s has MemoryPressure condition", n.Name),
			DetectedAt: now,
		})
	}

	if conditions["DiskPressure"] {
		out = append(out, Anomaly{
			Type:       AnomalyNodeDiskPressure,
			Severity:   "critical",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s has DiskPressure condition", n.Name),
			DetectedAt: now,
		})
	}

	if conditions["PIDPressure"] {
		out = append(out, Anomaly{
			Type:       AnomalyNodePIDPressure,
			Severity:   "warning",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s has PIDPressure condition", n.Name),
			DetectedAt: now,
		})
	}

	if !n.Ready {
		out = append(out, Anomaly{
			Type:       AnomalyNodeNotReady,
			Severity:   "critical",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s is not ready", n.Name),
			DetectedAt: now,
		})
	}

	return out
}

// detectNodeResources checks CPU, memory, and disk usage against thresholds.
func (d *Detector) detectNodeResources(n store.NodeMetrics, now time.Time) []Anomaly {
	var out []Anomaly

	// High CPU
	if n.CPUUsage >= d.thresholds.CPUCritical {
		out = append(out, Anomaly{
			Type:       AnomalyHighCPU,
			Severity:   "critical",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s CPU at %.1f%% (critical threshold %.1f%%)", n.Name, n.CPUUsage*100, d.thresholds.CPUCritical*100),
			DetectedAt: now,
			Value:      n.CPUUsage,
			Threshold:  d.thresholds.CPUCritical,
		})
	} else if n.CPUUsage >= d.thresholds.CPUWarning {
		out = append(out, Anomaly{
			Type:       AnomalyHighCPU,
			Severity:   "warning",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s CPU at %.1f%% (warning threshold %.1f%%)", n.Name, n.CPUUsage*100, d.thresholds.CPUWarning*100),
			DetectedAt: now,
			Value:      n.CPUUsage,
			Threshold:  d.thresholds.CPUWarning,
		})
	}

	// High memory
	if n.MemoryUsage >= d.thresholds.MemCritical {
		out = append(out, Anomaly{
			Type:       AnomalyHighMem,
			Severity:   "critical",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s memory at %.1f%% (critical threshold %.1f%%)", n.Name, n.MemoryUsage*100, d.thresholds.MemCritical*100),
			DetectedAt: now,
			Value:      n.MemoryUsage,
			Threshold:  d.thresholds.MemCritical,
		})
	} else if n.MemoryUsage >= d.thresholds.MemWarning {
		out = append(out, Anomaly{
			Type:       AnomalyHighMem,
			Severity:   "warning",
			NodeName:   n.Name,
			Message:    fmt.Sprintf("node %s memory at %.1f%% (warning threshold %.1f%%)", n.Name, n.MemoryUsage*100, d.thresholds.MemWarning*100),
			DetectedAt: now,
			Value:      n.MemoryUsage,
			Threshold:  d.thresholds.MemWarning,
		})
	}

	// High disk — skip if disk usage is -1 (unavailable)
	if n.DiskUsage >= 0 {
		if n.DiskUsage >= d.thresholds.DiskCritical {
			out = append(out, Anomaly{
				Type:       AnomalyHighDisk,
				Severity:   "critical",
				NodeName:   n.Name,
				Message:    fmt.Sprintf("node %s disk at %.1f%% (critical threshold %.1f%%)", n.Name, n.DiskUsage*100, d.thresholds.DiskCritical*100),
				DetectedAt: now,
				Value:      n.DiskUsage,
				Threshold:  d.thresholds.DiskCritical,
			})
		} else if n.DiskUsage >= d.thresholds.DiskWarning {
			out = append(out, Anomaly{
				Type:       AnomalyHighDisk,
				Severity:   "warning",
				NodeName:   n.Name,
				Message:    fmt.Sprintf("node %s disk at %.1f%% (warning threshold %.1f%%)", n.Name, n.DiskUsage*100, d.thresholds.DiskWarning*100),
				DetectedAt: now,
				Value:      n.DiskUsage,
				Threshold:  d.thresholds.DiskWarning,
			})
		}
	}

	return out
}

// detectPodEviction checks for pods in Failed phase with reason Evicted.
func (d *Detector) detectPodEviction(p store.PodMetrics, now time.Time) []Anomaly {
	if p.Phase == "Failed" && p.Reason == "Evicted" {
		return []Anomaly{{
			Type:       AnomalyPodEvicted,
			Severity:   "warning",
			NodeName:   p.Node,
			PodName:    p.Name,
			Namespace:  p.Namespace,
			Message:    fmt.Sprintf("pod %s/%s was evicted from node %s", p.Namespace, p.Name, p.Node),
			DetectedAt: now,
		}}
	}
	return nil
}

// detectPodLimitExceeded checks if a pod is using >90% of its CPU or memory limit.
func (d *Detector) detectPodLimitExceeded(p store.PodMetrics, now time.Time) []Anomaly {
	const limitThreshold = 0.90
	var out []Anomaly

	if p.CPULimit > 0 {
		ratio := p.CPUUsage / p.CPULimit
		if ratio >= limitThreshold {
			out = append(out, Anomaly{
				Type:       AnomalyLimitExceeded,
				Severity:   "warning",
				NodeName:   p.Node,
				PodName:    p.Name,
				Namespace:  p.Namespace,
				Message:    fmt.Sprintf("pod %s/%s CPU at %.1f%% of limit", p.Namespace, p.Name, ratio*100),
				DetectedAt: now,
				Value:      ratio,
				Threshold:  limitThreshold,
			})
		}
	}

	if p.MemoryLimit > 0 {
		ratio := float64(p.MemoryBytes) / float64(p.MemoryLimit)
		if ratio >= limitThreshold {
			out = append(out, Anomaly{
				Type:       AnomalyLimitExceeded,
				Severity:   "warning",
				NodeName:   p.Node,
				PodName:    p.Name,
				Namespace:  p.Namespace,
				Message:    fmt.Sprintf("pod %s/%s memory at %.1f%% of limit", p.Namespace, p.Name, ratio*100),
				DetectedAt: now,
				Value:      ratio,
				Threshold:  limitThreshold,
			})
		}
	}

	return out
}

// conditionSet converts a slice of condition strings to a set for O(1) lookup.
func conditionSet(conditions []string) map[string]bool {
	m := make(map[string]bool, len(conditions))
	for _, c := range conditions {
		m[c] = true
	}
	return m
}
