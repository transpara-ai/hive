// Package collector scrapes Kubernetes APIs and produces structured metrics.
package collector

import (
	"context"
	"fmt"
	"time"
)

// PodMetrics holds resource usage and allocation for a single pod.
type PodMetrics struct {
	Namespace     string             `json:"namespace"`
	PodName       string             `json:"pod_name"`
	NodeName      string             `json:"node_name"`
	Timestamp     time.Time          `json:"timestamp"`
	Containers    []ContainerMetrics `json:"containers"`
	TotalCPUMilli int64              `json:"total_cpu_milli"`
	TotalMemBytes int64              `json:"total_mem_bytes"`
	Phase         string             `json:"phase"` // Running, Pending, Failed, Succeeded
	RequestCPU    int64              `json:"request_cpu"`
	RequestMem    int64              `json:"request_mem"`
	LimitCPU      int64              `json:"limit_cpu"` // 0 = unlimited
	LimitMem      int64              `json:"limit_mem"`
}

// ContainerMetrics holds resource usage and allocation for a single container.
type ContainerMetrics struct {
	Name       string `json:"name"`
	CPUMilli   int64  `json:"cpu_milli"`
	MemBytes   int64  `json:"mem_bytes"`
	RequestCPU int64  `json:"request_cpu"`
	RequestMem int64  `json:"request_mem"`
	LimitCPU   int64  `json:"limit_cpu"`
	LimitMem   int64  `json:"limit_mem"`
}

// PodSpec represents a pod's metadata and resource configuration.
// Mirrors the fields we need from corev1.Pod without importing client-go.
type PodSpec struct {
	Name       string
	Namespace  string
	NodeName   string
	Phase      string // Running, Pending, Failed, Succeeded
	Containers []ContainerSpec
}

// ContainerSpec represents a container's resource requests and limits.
type ContainerSpec struct {
	Name       string
	RequestCPU int64 // millicores
	RequestMem int64 // bytes
	LimitCPU   int64 // millicores; 0 = no limit
	LimitMem   int64 // bytes; 0 = no limit
}

// PodUsage represents live resource consumption from the metrics API.
type PodUsage struct {
	Name       string
	Namespace  string
	Timestamp  time.Time
	Containers []ContainerUsage
}

// ContainerUsage represents a single container's live resource consumption.
type ContainerUsage struct {
	Name     string
	CPUMilli int64
	MemBytes int64
}

// PodLister lists pods across namespaces.
type PodLister interface {
	ListPods(ctx context.Context, namespace string) ([]PodSpec, error)
}

// PodMetricsLister lists pod metrics across namespaces.
type PodMetricsLister interface {
	ListPodMetrics(ctx context.Context, namespace string) ([]PodUsage, error)
}

// PodClients bundles the Kubernetes API clients needed for pod metrics collection.
type PodClients struct {
	Pods    PodLister
	Metrics PodMetricsLister
}

// podKey builds the map key for joining pod specs and metrics.
func podKey(namespace, name string) string {
	return namespace + "/" + name
}

// ScrapePods collects pod metrics by joining the core pod API (specs) with the
// metrics API (live usage). Pods without metrics (recently started) are included
// with zero usage values.
func ScrapePods(ctx context.Context, clients *PodClients) ([]PodMetrics, error) {
	if clients == nil || clients.Pods == nil || clients.Metrics == nil {
		return nil, fmt.Errorf("collector: nil clients")
	}

	// List all pods across all namespaces.
	pods, err := clients.Pods.ListPods(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("collector: list pods: %w", err)
	}

	// List all pod metrics across all namespaces.
	usage, err := clients.Metrics.ListPodMetrics(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("collector: list pod metrics: %w", err)
	}

	// Index metrics by namespace/name for O(1) lookup during the join.
	usageMap := make(map[string]*PodUsage, len(usage))
	for i := range usage {
		u := &usage[i]
		usageMap[podKey(u.Namespace, u.Name)] = u
	}

	result := make([]PodMetrics, 0, len(pods))
	for _, pod := range pods {
		pm := PodMetrics{
			Namespace: pod.Namespace,
			PodName:   pod.Name,
			NodeName:  pod.NodeName,
			Phase:     pod.Phase,
		}

		// Build container metrics by joining spec (requests/limits) with usage.
		u := usageMap[podKey(pod.Namespace, pod.Name)]

		// Index container usage by name for the per-container join.
		containerUsage := make(map[string]*ContainerUsage)
		if u != nil {
			pm.Timestamp = u.Timestamp
			for i := range u.Containers {
				cu := &u.Containers[i]
				containerUsage[cu.Name] = cu
			}
		}

		pm.Containers = make([]ContainerMetrics, 0, len(pod.Containers))
		for _, cs := range pod.Containers {
			cm := ContainerMetrics{
				Name:       cs.Name,
				RequestCPU: cs.RequestCPU,
				RequestMem: cs.RequestMem,
				LimitCPU:   cs.LimitCPU,
				LimitMem:   cs.LimitMem,
			}

			if cu, ok := containerUsage[cs.Name]; ok {
				cm.CPUMilli = cu.CPUMilli
				cm.MemBytes = cu.MemBytes
			}
			// No metrics for this container → zero usage (graceful handling).

			pm.Containers = append(pm.Containers, cm)

			// Accumulate pod-level totals.
			pm.TotalCPUMilli += cm.CPUMilli
			pm.TotalMemBytes += cm.MemBytes
			pm.RequestCPU += cs.RequestCPU
			pm.RequestMem += cs.RequestMem
			pm.LimitCPU += cs.LimitCPU
			pm.LimitMem += cs.LimitMem
		}

		result = append(result, pm)
	}

	return result, nil
}
