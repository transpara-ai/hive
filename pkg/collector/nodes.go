package collector

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// NodeMetrics holds resource usage, capacity, and health for a single node.
type NodeMetrics struct {
	NodeName       string          `json:"node_name"`
	Timestamp      time.Time       `json:"timestamp"`
	CPUUsageMilli  int64           `json:"cpu_usage_milli"`  // millicores
	CPUCapMilli    int64           `json:"cpu_cap_milli"`
	MemUsageBytes  int64           `json:"mem_usage_bytes"`
	MemCapBytes    int64           `json:"mem_cap_bytes"`
	DiskUsageBytes int64           `json:"disk_usage_bytes"` // from node exporter; -1 if unavailable
	DiskCapBytes   int64           `json:"disk_cap_bytes"`   // from node exporter; -1 if unavailable
	NetRxBytes     int64           `json:"net_rx_bytes"`     // cumulative from node exporter; -1 if unavailable
	NetTxBytes     int64           `json:"net_tx_bytes"`     // cumulative from node exporter; -1 if unavailable
	Conditions     []NodeCondition `json:"conditions"`
	Allocatable    ResourceSet     `json:"allocatable"`
}

// NodeCondition represents a node health condition.
type NodeCondition struct {
	Type   string `json:"type"`   // "MemoryPressure", "DiskPressure", "PIDPressure", "Ready"
	Status bool   `json:"status"` // true = condition active (pressure exists / node ready)
}

// ResourceSet holds allocatable CPU and memory for a node.
type ResourceSet struct {
	CPUMilli int64 `json:"cpu_milli"`
	MemBytes int64 `json:"mem_bytes"`
}

// NodeSpec represents a node's metadata, capacity, and conditions.
// Mirrors the fields we need from corev1.Node without importing client-go.
type NodeSpec struct {
	Name        string
	InternalIP  string // for reaching node exporter
	CPUCapMilli int64  // capacity in millicores
	MemCapBytes int64  // capacity in bytes
	Allocatable ResourceSet
	Conditions  []NodeCondition
}

// NodeUsage represents live resource consumption from the metrics API.
type NodeUsage struct {
	Name      string
	Timestamp time.Time
	CPUMilli  int64
	MemBytes  int64
}

// NodeLister lists nodes in the cluster.
type NodeLister interface {
	ListNodes(ctx context.Context) ([]NodeSpec, error)
}

// NodeMetricsLister lists node-level metrics.
type NodeMetricsLister interface {
	ListNodeMetrics(ctx context.Context) ([]NodeUsage, error)
}

// NodeExporterFetcher fetches raw Prometheus metrics text from a node exporter endpoint.
type NodeExporterFetcher interface {
	// FetchNodeExporter fetches metrics from http://<addr>:9100/metrics.
	// Returns the response body as a string, or an error if unreachable.
	FetchNodeExporter(ctx context.Context, addr string) (string, error)
}

// NodeClients bundles the clients needed for node metrics collection.
type NodeClients struct {
	Nodes        NodeLister
	Metrics      NodeMetricsLister
	NodeExporter NodeExporterFetcher // nil = skip node exporter scraping
}

// ScrapeNodes collects node metrics by joining the core node API (specs/conditions)
// with the metrics API (CPU/memory usage) and optionally node exporter (disk/network).
// Disk and network fields are set to -1 when node exporter is unavailable.
func ScrapeNodes(ctx context.Context, clients *NodeClients) ([]NodeMetrics, error) {
	if clients == nil || clients.Nodes == nil || clients.Metrics == nil {
		return nil, fmt.Errorf("collector: nil node clients")
	}

	nodes, err := clients.Nodes.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("collector: list nodes: %w", err)
	}

	usage, err := clients.Metrics.ListNodeMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("collector: list node metrics: %w", err)
	}

	// Index metrics by node name for O(1) lookup during the join.
	usageMap := make(map[string]*NodeUsage, len(usage))
	for i := range usage {
		u := &usage[i]
		usageMap[u.Name] = u
	}

	result := make([]NodeMetrics, 0, len(nodes))
	for _, node := range nodes {
		nm := NodeMetrics{
			NodeName:    node.Name,
			CPUCapMilli: node.CPUCapMilli,
			MemCapBytes: node.MemCapBytes,
			Conditions:  node.Conditions,
			Allocatable: node.Allocatable,
			// Default to -1 for node exporter metrics (unavailable).
			DiskUsageBytes: -1,
			DiskCapBytes:   -1,
			NetRxBytes:     -1,
			NetTxBytes:     -1,
		}

		if u, ok := usageMap[node.Name]; ok {
			nm.Timestamp = u.Timestamp
			nm.CPUUsageMilli = u.CPUMilli
			nm.MemUsageBytes = u.MemBytes
		}

		// Attempt node exporter scrape for disk/network.
		if clients.NodeExporter != nil && node.InternalIP != "" {
			body, fetchErr := clients.NodeExporter.FetchNodeExporter(ctx, node.InternalIP)
			if fetchErr == nil {
				disk := parseNodeExporterDisk(body)
				nm.DiskCapBytes = disk.capBytes
				nm.DiskUsageBytes = disk.usageBytes
				net := parseNodeExporterNetwork(body)
				nm.NetRxBytes = net.rxBytes
				nm.NetTxBytes = net.txBytes
			}
			// fetchErr != nil → leave at -1 (unreachable).
		}

		result = append(result, nm)
	}

	return result, nil
}

// diskMetrics holds parsed filesystem metrics from node exporter.
type diskMetrics struct {
	capBytes   int64
	usageBytes int64
}

// netMetrics holds parsed network metrics from node exporter.
type netMetrics struct {
	rxBytes int64
	txBytes int64
}

// parseNodeExporterDisk extracts root filesystem size and usage from Prometheus text.
// Looks for node_filesystem_size_bytes and node_filesystem_avail_bytes with mountpoint="/".
func parseNodeExporterDisk(body string) diskMetrics {
	var sizeBytes, availBytes int64
	var foundSize, foundAvail bool

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "node_filesystem_size_bytes{") && hasLabel(line, "mountpoint", "/") {
			if v, ok := parsePromValue(line); ok {
				sizeBytes = v
				foundSize = true
			}
		} else if strings.HasPrefix(line, "node_filesystem_avail_bytes{") && hasLabel(line, "mountpoint", "/") {
			if v, ok := parsePromValue(line); ok {
				availBytes = v
				foundAvail = true
			}
		}
	}

	if foundSize && foundAvail {
		return diskMetrics{capBytes: sizeBytes, usageBytes: sizeBytes - availBytes}
	}
	return diskMetrics{capBytes: -1, usageBytes: -1}
}

// parseNodeExporterNetwork extracts cumulative rx/tx bytes from Prometheus text.
// Sums across all devices except "lo" (loopback).
func parseNodeExporterNetwork(body string) netMetrics {
	var rxTotal, txTotal int64
	var found bool

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "node_network_receive_bytes_total{") && !hasLabel(line, "device", "lo") {
			if v, ok := parsePromValue(line); ok {
				rxTotal += v
				found = true
			}
		} else if strings.HasPrefix(line, "node_network_transmit_bytes_total{") && !hasLabel(line, "device", "lo") {
			if v, ok := parsePromValue(line); ok {
				txTotal += v
				found = true
			}
		}
	}

	if found {
		return netMetrics{rxBytes: rxTotal, txBytes: txTotal}
	}
	return netMetrics{rxBytes: -1, txBytes: -1}
}

// hasLabel checks whether a Prometheus metric line contains a specific label=value pair.
// Example line: `node_filesystem_size_bytes{device="/dev/sda1",mountpoint="/"} 1.07e+10`
func hasLabel(line, key, value string) bool {
	// Look for key="value" within the label set.
	target := key + `="` + value + `"`
	return strings.Contains(line, target)
}

// parsePromValue extracts the float64 value from the end of a Prometheus metric line
// and converts it to int64. Returns (0, false) if parsing fails.
// Example: `metric_name{labels} 1.073741824e+10` → 10737418240
func parsePromValue(line string) (int64, bool) {
	// The value is the last space-separated field.
	idx := strings.LastIndexByte(line, ' ')
	if idx < 0 || idx >= len(line)-1 {
		return 0, false
	}
	// Also check for tab separator (some exporters use tabs).
	tabIdx := strings.LastIndexByte(line, '\t')
	if tabIdx > idx {
		idx = tabIdx
	}

	valStr := strings.TrimSpace(line[idx+1:])
	f, err := strconv.ParseFloat(valStr, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return int64(math.Round(f)), true
}
