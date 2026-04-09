// Package store provides in-memory storage for Kubernetes cluster health snapshots.
package store

import "time"

// ClusterSummary is an aggregate view of cluster health at a point in time.
type ClusterSummary struct {
	// Legacy fields — kept for backward compatibility with existing consumers.
	Nodes       int       `json:"nodes"`
	Pods        int       `json:"pods"`
	Namespaces  int       `json:"namespaces"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage float64   `json:"memory_usage"`
	Healthy     bool      `json:"healthy"`
	Severity    string    `json:"severity"`
	CollectedAt time.Time `json:"collected_at"`

	// Detailed resource accounting.
	TotalNodes    int   `json:"total_nodes"`
	ReadyNodes    int   `json:"ready_nodes"`
	TotalCPUMilli int64 `json:"total_cpu_milli"`
	UsedCPUMilli  int64 `json:"used_cpu_milli"`
	TotalMemBytes int64 `json:"total_mem_bytes"`
	UsedMemBytes  int64 `json:"used_mem_bytes"`
	TotalPods     int   `json:"total_pods"`
	RunningPods   int   `json:"running_pods"`
	PendingPods   int   `json:"pending_pods"`
	FailedPods    int   `json:"failed_pods"`
	AllocatedCPU  int64 `json:"allocated_cpu"`  // sum of all pod CPU requests (milli)
	AllocatedMem  int64 `json:"allocated_mem"`  // sum of all pod memory requests (bytes)
}

// NodeMetrics holds resource usage for a single node.
type NodeMetrics struct {
	Name            string    `json:"name"`
	CPUCores        int       `json:"cpu_cores"`
	CPUUsage        float64   `json:"cpu_usage"`
	MemoryBytes     int64     `json:"memory_bytes"`
	MemoryUsage     float64   `json:"memory_usage"`
	DiskUsage       float64   `json:"disk_usage"` // Fraction 0.0-1.0; -1 means unavailable
	PodCount        int       `json:"pod_count"`
	Conditions      []string  `json:"conditions"`
	Ready           bool      `json:"ready"`
	CollectedAt     time.Time `json:"collected_at"`
}

// PodMetrics holds resource usage for a single pod.
type PodMetrics struct {
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	Node        string    `json:"node"`
	Phase       string    `json:"phase"`
	Reason      string    `json:"reason,omitempty"` // e.g. "Evicted", "OOMKilled"
	CPUUsage    float64   `json:"cpu_usage"`
	CPULimit    float64   `json:"cpu_limit,omitempty"`    // CPU limit in cores; 0 means no limit
	MemoryBytes int64     `json:"memory_bytes"`
	MemoryLimit int64     `json:"memory_limit,omitempty"` // Memory limit in bytes; 0 means no limit
	Restarts    int       `json:"restarts"`
	Ready       bool      `json:"ready"`
	CollectedAt time.Time `json:"collected_at"`
}

// Snapshot is a complete point-in-time capture of cluster state.
type Snapshot struct {
	Cluster   ClusterSummary `json:"cluster"`
	Nodes     []NodeMetrics  `json:"nodes"`
	Pods      []PodMetrics   `json:"pods"`
	Timestamp time.Time      `json:"timestamp"`
}
