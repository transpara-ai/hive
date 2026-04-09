package collector

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- Mocks ---

type mockNodeLister struct {
	nodes []NodeSpec
	err   error
}

func (m *mockNodeLister) ListNodes(_ context.Context) ([]NodeSpec, error) {
	return m.nodes, m.err
}

type mockNodeMetricsLister struct {
	usage []NodeUsage
	err   error
}

func (m *mockNodeMetricsLister) ListNodeMetrics(_ context.Context) ([]NodeUsage, error) {
	return m.usage, m.err
}

type mockNodeExporterFetcher struct {
	bodies map[string]string // addr → response body
	err    error
}

func (m *mockNodeExporterFetcher) FetchNodeExporter(_ context.Context, addr string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	body, ok := m.bodies[addr]
	if !ok {
		return "", fmt.Errorf("unreachable: %s", addr)
	}
	return body, nil
}

// --- Prometheus parser tests ---

const sampleNodeExporterOutput = `# HELP node_filesystem_size_bytes Filesystem size in bytes.
# TYPE node_filesystem_size_bytes gauge
node_filesystem_size_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 1.073741824e+10
node_filesystem_size_bytes{device="/dev/sda2",fstype="ext4",mountpoint="/data"} 5.36870912e+09
# HELP node_filesystem_avail_bytes Filesystem space available to non-root users.
# TYPE node_filesystem_avail_bytes gauge
node_filesystem_avail_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 5.36870912e+09
node_filesystem_avail_bytes{device="/dev/sda2",fstype="ext4",mountpoint="/data"} 2.68435456e+09
# HELP node_network_receive_bytes_total Network device statistic receive_bytes.
# TYPE node_network_receive_bytes_total counter
node_network_receive_bytes_total{device="eth0"} 1.234e+09
node_network_receive_bytes_total{device="lo"} 5.678e+06
node_network_receive_bytes_total{device="eth1"} 1e+08
# HELP node_network_transmit_bytes_total Network device statistic transmit_bytes.
# TYPE node_network_transmit_bytes_total counter
node_network_transmit_bytes_total{device="eth0"} 9.87e+08
node_network_transmit_bytes_total{device="lo"} 5.678e+06
node_network_transmit_bytes_total{device="eth1"} 5e+07
`

func TestParseNodeExporterDisk_RootFilesystem(t *testing.T) {
	disk := parseNodeExporterDisk(sampleNodeExporterOutput)

	// Size = 1.073741824e+10 = 10GB
	if disk.capBytes != 10737418240 {
		t.Fatalf("capBytes = %d, want 10737418240", disk.capBytes)
	}
	// Avail = 5.36870912e+09 = 5GB → usage = 10GB - 5GB = 5GB
	if disk.usageBytes != 5368709120 {
		t.Fatalf("usageBytes = %d, want 5368709120", disk.usageBytes)
	}
}

func TestParseNodeExporterDisk_NoRootMountpoint(t *testing.T) {
	body := `# HELP node_filesystem_size_bytes Filesystem size in bytes.
node_filesystem_size_bytes{device="/dev/sda2",fstype="ext4",mountpoint="/data"} 5.36870912e+09
node_filesystem_avail_bytes{device="/dev/sda2",fstype="ext4",mountpoint="/data"} 2.68435456e+09
`
	disk := parseNodeExporterDisk(body)

	if disk.capBytes != -1 {
		t.Fatalf("capBytes = %d, want -1 (no root mountpoint)", disk.capBytes)
	}
	if disk.usageBytes != -1 {
		t.Fatalf("usageBytes = %d, want -1 (no root mountpoint)", disk.usageBytes)
	}
}

func TestParseNodeExporterDisk_OnlySizeNoAvail(t *testing.T) {
	body := `node_filesystem_size_bytes{device="/dev/sda1",fstype="ext4",mountpoint="/"} 1e+10
`
	disk := parseNodeExporterDisk(body)

	if disk.capBytes != -1 || disk.usageBytes != -1 {
		t.Fatalf("expected -1 when avail is missing, got cap=%d usage=%d", disk.capBytes, disk.usageBytes)
	}
}

func TestParseNodeExporterDisk_EmptyBody(t *testing.T) {
	disk := parseNodeExporterDisk("")
	if disk.capBytes != -1 || disk.usageBytes != -1 {
		t.Fatalf("expected -1 for empty body, got cap=%d usage=%d", disk.capBytes, disk.usageBytes)
	}
}

func TestParseNodeExporterNetwork_SumsNonLoopback(t *testing.T) {
	net := parseNodeExporterNetwork(sampleNodeExporterOutput)

	// RX: eth0=1.234e+09 + eth1=1e+08 = 1334000000 (lo excluded)
	wantRx := int64(1234000000 + 100000000)
	if net.rxBytes != wantRx {
		t.Fatalf("rxBytes = %d, want %d", net.rxBytes, wantRx)
	}

	// TX: eth0=9.87e+08 + eth1=5e+07 = 1037000000 (lo excluded)
	wantTx := int64(987000000 + 50000000)
	if net.txBytes != wantTx {
		t.Fatalf("txBytes = %d, want %d", net.txBytes, wantTx)
	}
}

func TestParseNodeExporterNetwork_NoNetworkMetrics(t *testing.T) {
	body := `# Only filesystem metrics
node_filesystem_size_bytes{device="/dev/sda1",mountpoint="/"} 1e+10
`
	net := parseNodeExporterNetwork(body)

	if net.rxBytes != -1 || net.txBytes != -1 {
		t.Fatalf("expected -1 when no network metrics, got rx=%d tx=%d", net.rxBytes, net.txBytes)
	}
}

func TestParseNodeExporterNetwork_OnlyLoopback(t *testing.T) {
	body := `node_network_receive_bytes_total{device="lo"} 1000
node_network_transmit_bytes_total{device="lo"} 2000
`
	net := parseNodeExporterNetwork(body)

	if net.rxBytes != -1 || net.txBytes != -1 {
		t.Fatalf("expected -1 when only loopback, got rx=%d tx=%d", net.rxBytes, net.txBytes)
	}
}

func TestHasLabel(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		key   string
		value string
		want  bool
	}{
		{
			name:  "exact match",
			line:  `node_filesystem_size_bytes{device="/dev/sda1",mountpoint="/"} 1e10`,
			key:   "mountpoint", value: "/", want: true,
		},
		{
			name:  "no match",
			line:  `node_filesystem_size_bytes{device="/dev/sda1",mountpoint="/data"} 1e10`,
			key:   "mountpoint", value: "/", want: false,
		},
		{
			name:  "device lo",
			line:  `node_network_receive_bytes_total{device="lo"} 1000`,
			key:   "device", value: "lo", want: true,
		},
		{
			name:  "device eth0 not lo",
			line:  `node_network_receive_bytes_total{device="eth0"} 1000`,
			key:   "device", value: "lo", want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasLabel(tt.line, tt.key, tt.value)
			if got != tt.want {
				t.Fatalf("hasLabel(%q, %q, %q) = %v, want %v", tt.line, tt.key, tt.value, got, tt.want)
			}
		})
	}
}

func TestParsePromValue(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   int64
		wantOK bool
	}{
		{
			name:   "scientific notation",
			line:   `node_filesystem_size_bytes{mountpoint="/"} 1.073741824e+10`,
			want:   10737418240,
			wantOK: true,
		},
		{
			name:   "integer value",
			line:   `node_network_receive_bytes_total{device="eth0"} 12345`,
			want:   12345,
			wantOK: true,
		},
		{
			name:   "zero value",
			line:   `some_metric{} 0`,
			want:   0,
			wantOK: true,
		},
		{
			name:   "no space separator",
			line:   `broken_metric`,
			want:   0,
			wantOK: false,
		},
		{
			name:   "non-numeric value",
			line:   `some_metric{} NaN`,
			want:   0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parsePromValue(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("parsePromValue ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("parsePromValue = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- ScrapeNodes tests ---

func TestScrapeNodes_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	clients := &NodeClients{
		Nodes: &mockNodeLister{nodes: []NodeSpec{
			{
				Name: "node-a", InternalIP: "10.0.0.1",
				CPUCapMilli: 4000, MemCapBytes: 8 << 30,
				Allocatable: ResourceSet{CPUMilli: 3800, MemBytes: 7 << 30},
				Conditions: []NodeCondition{
					{Type: "Ready", Status: true},
					{Type: "MemoryPressure", Status: false},
				},
			},
		}},
		Metrics: &mockNodeMetricsLister{usage: []NodeUsage{
			{Name: "node-a", Timestamp: now, CPUMilli: 1500, MemBytes: 4 << 30},
		}},
		NodeExporter: &mockNodeExporterFetcher{bodies: map[string]string{
			"10.0.0.1": sampleNodeExporterOutput,
		}},
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result))
	}

	nm := result[0]
	if nm.NodeName != "node-a" {
		t.Fatalf("NodeName = %q, want %q", nm.NodeName, "node-a")
	}
	if !nm.Timestamp.Equal(now) {
		t.Fatalf("Timestamp = %v, want %v", nm.Timestamp, now)
	}
	if nm.CPUUsageMilli != 1500 {
		t.Fatalf("CPUUsageMilli = %d, want 1500", nm.CPUUsageMilli)
	}
	if nm.CPUCapMilli != 4000 {
		t.Fatalf("CPUCapMilli = %d, want 4000", nm.CPUCapMilli)
	}
	if nm.MemUsageBytes != 4<<30 {
		t.Fatalf("MemUsageBytes = %d, want %d", nm.MemUsageBytes, int64(4<<30))
	}
	if nm.MemCapBytes != 8<<30 {
		t.Fatalf("MemCapBytes = %d, want %d", nm.MemCapBytes, int64(8<<30))
	}

	// Node exporter disk metrics.
	if nm.DiskCapBytes != 10737418240 {
		t.Fatalf("DiskCapBytes = %d, want 10737418240", nm.DiskCapBytes)
	}
	if nm.DiskUsageBytes != 5368709120 {
		t.Fatalf("DiskUsageBytes = %d, want 5368709120", nm.DiskUsageBytes)
	}

	// Node exporter network metrics (eth0+eth1, excluding lo).
	wantRx := int64(1234000000 + 100000000)
	if nm.NetRxBytes != wantRx {
		t.Fatalf("NetRxBytes = %d, want %d", nm.NetRxBytes, wantRx)
	}
	wantTx := int64(987000000 + 50000000)
	if nm.NetTxBytes != wantTx {
		t.Fatalf("NetTxBytes = %d, want %d", nm.NetTxBytes, wantTx)
	}

	// Conditions and allocatable.
	if len(nm.Conditions) != 2 {
		t.Fatalf("got %d conditions, want 2", len(nm.Conditions))
	}
	if nm.Allocatable.CPUMilli != 3800 {
		t.Fatalf("Allocatable.CPUMilli = %d, want 3800", nm.Allocatable.CPUMilli)
	}
}

func TestScrapeNodes_NodeExporterUnreachable(t *testing.T) {
	now := time.Now()
	clients := &NodeClients{
		Nodes: &mockNodeLister{nodes: []NodeSpec{
			{Name: "node-b", InternalIP: "10.0.0.2", CPUCapMilli: 2000, MemCapBytes: 4 << 30},
		}},
		Metrics: &mockNodeMetricsLister{usage: []NodeUsage{
			{Name: "node-b", Timestamp: now, CPUMilli: 800, MemBytes: 2 << 30},
		}},
		NodeExporter: &mockNodeExporterFetcher{
			err: fmt.Errorf("connection refused"),
		},
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result))
	}

	nm := result[0]
	if nm.CPUUsageMilli != 800 {
		t.Fatalf("CPUUsageMilli = %d, want 800", nm.CPUUsageMilli)
	}
	if nm.DiskUsageBytes != -1 || nm.DiskCapBytes != -1 {
		t.Fatalf("disk should be -1 when exporter unreachable, got usage=%d cap=%d",
			nm.DiskUsageBytes, nm.DiskCapBytes)
	}
	if nm.NetRxBytes != -1 || nm.NetTxBytes != -1 {
		t.Fatalf("net should be -1 when exporter unreachable, got rx=%d tx=%d",
			nm.NetRxBytes, nm.NetTxBytes)
	}
}

func TestScrapeNodes_NilNodeExporter(t *testing.T) {
	now := time.Now()
	clients := &NodeClients{
		Nodes: &mockNodeLister{nodes: []NodeSpec{
			{Name: "node-c", InternalIP: "10.0.0.3", CPUCapMilli: 2000, MemCapBytes: 4 << 30},
		}},
		Metrics: &mockNodeMetricsLister{usage: []NodeUsage{
			{Name: "node-c", Timestamp: now, CPUMilli: 500, MemBytes: 1 << 30},
		}},
		NodeExporter: nil,
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nm := result[0]
	if nm.DiskUsageBytes != -1 || nm.NetRxBytes != -1 {
		t.Fatalf("expected -1 with nil NodeExporter, got disk=%d net=%d",
			nm.DiskUsageBytes, nm.NetRxBytes)
	}
}

func TestScrapeNodes_NoInternalIP(t *testing.T) {
	clients := &NodeClients{
		Nodes: &mockNodeLister{nodes: []NodeSpec{
			{Name: "node-d", InternalIP: "", CPUCapMilli: 2000, MemCapBytes: 4 << 30},
		}},
		Metrics:      &mockNodeMetricsLister{usage: nil},
		NodeExporter: &mockNodeExporterFetcher{bodies: map[string]string{}},
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nm := result[0]
	if nm.DiskUsageBytes != -1 || nm.NetRxBytes != -1 {
		t.Fatalf("expected -1 with no InternalIP, got disk=%d net=%d",
			nm.DiskUsageBytes, nm.NetRxBytes)
	}
}

func TestScrapeNodes_MissingMetrics(t *testing.T) {
	clients := &NodeClients{
		Nodes: &mockNodeLister{nodes: []NodeSpec{
			{Name: "node-e", CPUCapMilli: 4000, MemCapBytes: 8 << 30},
		}},
		Metrics:      &mockNodeMetricsLister{usage: nil},
		NodeExporter: nil,
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result))
	}

	nm := result[0]
	if nm.CPUUsageMilli != 0 || nm.MemUsageBytes != 0 {
		t.Fatalf("expected zero usage when metrics missing, got cpu=%d mem=%d",
			nm.CPUUsageMilli, nm.MemUsageBytes)
	}
	if nm.Timestamp != (time.Time{}) {
		t.Fatalf("expected zero timestamp when metrics missing, got %v", nm.Timestamp)
	}
	if nm.CPUCapMilli != 4000 {
		t.Fatalf("CPUCapMilli = %d, want 4000", nm.CPUCapMilli)
	}
}

func TestScrapeNodes_EmptyCluster(t *testing.T) {
	clients := &NodeClients{
		Nodes:   &mockNodeLister{nodes: nil},
		Metrics: &mockNodeMetricsLister{usage: nil},
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d nodes, want 0", len(result))
	}
}

func TestScrapeNodes_NilClients(t *testing.T) {
	tests := []struct {
		name    string
		clients *NodeClients
	}{
		{"nil clients", nil},
		{"nil nodes lister", &NodeClients{Nodes: nil, Metrics: &mockNodeMetricsLister{}}},
		{"nil metrics lister", &NodeClients{Nodes: &mockNodeLister{}, Metrics: nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ScrapeNodes(context.Background(), tt.clients)
			if err == nil {
				t.Fatal("expected error for nil clients")
			}
		})
	}
}

func TestScrapeNodes_NodeListError(t *testing.T) {
	clients := &NodeClients{
		Nodes:   &mockNodeLister{err: context.DeadlineExceeded},
		Metrics: &mockNodeMetricsLister{},
	}

	_, err := ScrapeNodes(context.Background(), clients)
	if err == nil {
		t.Fatal("expected error when node list fails")
	}
}

func TestScrapeNodes_MetricsListError(t *testing.T) {
	clients := &NodeClients{
		Nodes:   &mockNodeLister{nodes: []NodeSpec{{Name: "n"}}},
		Metrics: &mockNodeMetricsLister{err: context.DeadlineExceeded},
	}

	_, err := ScrapeNodes(context.Background(), clients)
	if err == nil {
		t.Fatal("expected error when metrics list fails")
	}
}

func TestScrapeNodes_MultipleNodes(t *testing.T) {
	now := time.Now()
	clients := &NodeClients{
		Nodes: &mockNodeLister{nodes: []NodeSpec{
			{Name: "node-1", InternalIP: "10.0.0.1", CPUCapMilli: 4000, MemCapBytes: 8 << 30},
			{Name: "node-2", InternalIP: "10.0.0.2", CPUCapMilli: 8000, MemCapBytes: 16 << 30},
			{Name: "node-3", InternalIP: "10.0.0.3", CPUCapMilli: 2000, MemCapBytes: 4 << 30},
		}},
		Metrics: &mockNodeMetricsLister{usage: []NodeUsage{
			{Name: "node-1", Timestamp: now, CPUMilli: 1000, MemBytes: 3 << 30},
			{Name: "node-2", Timestamp: now, CPUMilli: 4000, MemBytes: 10 << 30},
			// node-3 has no metrics (just added to cluster).
		}},
		NodeExporter: nil,
	}

	result, err := ScrapeNodes(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d nodes, want 3", len(result))
	}

	for _, nm := range result {
		switch nm.NodeName {
		case "node-1":
			if nm.CPUUsageMilli != 1000 {
				t.Fatalf("node-1 CPU = %d, want 1000", nm.CPUUsageMilli)
			}
		case "node-2":
			if nm.CPUUsageMilli != 4000 {
				t.Fatalf("node-2 CPU = %d, want 4000", nm.CPUUsageMilli)
			}
			if nm.CPUCapMilli != 8000 {
				t.Fatalf("node-2 CPUCap = %d, want 8000", nm.CPUCapMilli)
			}
		case "node-3":
			if nm.CPUUsageMilli != 0 {
				t.Fatalf("node-3 CPU = %d, want 0 (no metrics)", nm.CPUUsageMilli)
			}
		default:
			t.Fatalf("unexpected node: %s", nm.NodeName)
		}
	}
}
