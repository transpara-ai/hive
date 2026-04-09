// Package store provides in-memory time-series storage for collected metrics.
package store

import "time"

// NodeMetrics holds resource usage for a single cluster node.
type NodeMetrics struct {
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
	DiskPercent   float64 `json:"diskPercent"`
	CPUCores      int64   `json:"cpuCores"`
	MemoryBytes   int64   `json:"memoryBytes"`
	DiskBytes     int64   `json:"diskBytes"`
}

// PodMetrics holds resource usage for a single pod.
type PodMetrics struct {
	Name          string  `json:"name"`
	Namespace     string  `json:"namespace"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryPercent float64 `json:"memoryPercent"`
	MemoryBytes   int64   `json:"memoryBytes"`
	Status        string  `json:"status"`
	Restarts      int32   `json:"restarts"`
}

// Snapshot is a point-in-time collection of all node and pod metrics.
type Snapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	Nodes     []NodeMetrics `json:"nodes"`
	Pods      []PodMetrics  `json:"pods"`
}

// Store provides read access to collected metrics.
type Store interface {
	// Latest returns the most recent metric snapshot, or nil if none collected yet.
	Latest() *Snapshot

	// Range returns snapshots within the given time window, ordered by timestamp.
	Range(from, to time.Time) []Snapshot
}
