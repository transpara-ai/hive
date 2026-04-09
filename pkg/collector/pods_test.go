package collector

import (
	"context"
	"testing"
	"time"
)

// --- Mocks ---

type mockPodLister struct {
	pods []PodSpec
	err  error
}

func (m *mockPodLister) ListPods(_ context.Context, _ string) ([]PodSpec, error) {
	return m.pods, m.err
}

type mockMetricsLister struct {
	usage []PodUsage
	err   error
}

func (m *mockMetricsLister) ListPodMetrics(_ context.Context, _ string) ([]PodUsage, error) {
	return m.usage, m.err
}

// --- Tests ---

func TestScrapePods_HappyPath(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	clients := &PodClients{
		Pods: &mockPodLister{pods: []PodSpec{
			{
				Name: "web-1", Namespace: "default", NodeName: "node-a", Phase: "Running",
				Containers: []ContainerSpec{
					{Name: "nginx", RequestCPU: 100, RequestMem: 128 << 20, LimitCPU: 500, LimitMem: 256 << 20},
					{Name: "sidecar", RequestCPU: 50, RequestMem: 64 << 20, LimitCPU: 200, LimitMem: 128 << 20},
				},
			},
		}},
		Metrics: &mockMetricsLister{usage: []PodUsage{
			{
				Name: "web-1", Namespace: "default", Timestamp: now,
				Containers: []ContainerUsage{
					{Name: "nginx", CPUMilli: 80, MemBytes: 100 << 20},
					{Name: "sidecar", CPUMilli: 20, MemBytes: 30 << 20},
				},
			},
		}},
	}

	result, err := ScrapePods(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d pods, want 1", len(result))
	}

	pm := result[0]
	if pm.PodName != "web-1" || pm.Namespace != "default" {
		t.Fatalf("identity mismatch: %s/%s", pm.Namespace, pm.PodName)
	}
	if pm.NodeName != "node-a" {
		t.Fatalf("NodeName = %q, want %q", pm.NodeName, "node-a")
	}
	if pm.Phase != "Running" {
		t.Fatalf("Phase = %q, want %q", pm.Phase, "Running")
	}
	if !pm.Timestamp.Equal(now) {
		t.Fatalf("Timestamp = %v, want %v", pm.Timestamp, now)
	}

	// Container-level checks.
	if len(pm.Containers) != 2 {
		t.Fatalf("got %d containers, want 2", len(pm.Containers))
	}
	nginx := pm.Containers[0]
	if nginx.Name != "nginx" || nginx.CPUMilli != 80 || nginx.MemBytes != 100<<20 {
		t.Fatalf("nginx usage mismatch: cpu=%d mem=%d", nginx.CPUMilli, nginx.MemBytes)
	}
	if nginx.RequestCPU != 100 || nginx.LimitCPU != 500 {
		t.Fatalf("nginx requests/limits mismatch: req=%d lim=%d", nginx.RequestCPU, nginx.LimitCPU)
	}

	// Pod-level totals.
	if pm.TotalCPUMilli != 100 {
		t.Fatalf("TotalCPUMilli = %d, want 100", pm.TotalCPUMilli)
	}
	if pm.TotalMemBytes != (100+30)<<20 {
		t.Fatalf("TotalMemBytes = %d, want %d", pm.TotalMemBytes, (100+30)<<20)
	}
	if pm.RequestCPU != 150 {
		t.Fatalf("RequestCPU = %d, want 150", pm.RequestCPU)
	}
	if pm.RequestMem != (128+64)<<20 {
		t.Fatalf("RequestMem = %d, want %d", pm.RequestMem, (128+64)<<20)
	}
	if pm.LimitCPU != 700 {
		t.Fatalf("LimitCPU = %d, want 700", pm.LimitCPU)
	}
	if pm.LimitMem != (256+128)<<20 {
		t.Fatalf("LimitMem = %d, want %d", pm.LimitMem, (256+128)<<20)
	}
}

func TestScrapePods_MissingMetrics(t *testing.T) {
	// Pod exists but metrics API has nothing for it (recently started).
	clients := &PodClients{
		Pods: &mockPodLister{pods: []PodSpec{
			{
				Name: "new-pod", Namespace: "staging", NodeName: "node-b", Phase: "Pending",
				Containers: []ContainerSpec{
					{Name: "app", RequestCPU: 200, RequestMem: 256 << 20, LimitCPU: 0, LimitMem: 0},
				},
			},
		}},
		Metrics: &mockMetricsLister{usage: nil}, // no metrics at all
	}

	result, err := ScrapePods(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d pods, want 1", len(result))
	}

	pm := result[0]
	if pm.Phase != "Pending" {
		t.Fatalf("Phase = %q, want %q", pm.Phase, "Pending")
	}
	if pm.TotalCPUMilli != 0 || pm.TotalMemBytes != 0 {
		t.Fatalf("expected zero usage, got cpu=%d mem=%d", pm.TotalCPUMilli, pm.TotalMemBytes)
	}
	if pm.Timestamp != (time.Time{}) {
		t.Fatalf("expected zero timestamp for pod without metrics, got %v", pm.Timestamp)
	}
	// Requests/limits should still be populated.
	if pm.RequestCPU != 200 {
		t.Fatalf("RequestCPU = %d, want 200", pm.RequestCPU)
	}
	if pm.LimitCPU != 0 {
		t.Fatalf("LimitCPU = %d, want 0 (unlimited)", pm.LimitCPU)
	}
}

func TestScrapePods_EmptyCluster(t *testing.T) {
	clients := &PodClients{
		Pods:    &mockPodLister{pods: nil},
		Metrics: &mockMetricsLister{usage: nil},
	}

	result, err := ScrapePods(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %d pods, want 0", len(result))
	}
}

func TestScrapePods_MultiplePodsMixedMetrics(t *testing.T) {
	now := time.Now()

	clients := &PodClients{
		Pods: &mockPodLister{pods: []PodSpec{
			{Name: "running", Namespace: "prod", NodeName: "n1", Phase: "Running",
				Containers: []ContainerSpec{{Name: "c1", RequestCPU: 100, RequestMem: 64 << 20}}},
			{Name: "pending", Namespace: "prod", NodeName: "", Phase: "Pending",
				Containers: []ContainerSpec{{Name: "c1", RequestCPU: 50, RequestMem: 32 << 20}}},
			{Name: "failed", Namespace: "dev", NodeName: "n2", Phase: "Failed",
				Containers: []ContainerSpec{{Name: "c1", RequestCPU: 100, RequestMem: 64 << 20}}},
		}},
		Metrics: &mockMetricsLister{usage: []PodUsage{
			{Name: "running", Namespace: "prod", Timestamp: now,
				Containers: []ContainerUsage{{Name: "c1", CPUMilli: 75, MemBytes: 50 << 20}}},
			// "pending" and "failed" have no metrics.
		}},
	}

	result, err := ScrapePods(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d pods, want 3", len(result))
	}

	// Verify the running pod got metrics.
	if result[0].TotalCPUMilli != 75 {
		t.Fatalf("running pod CPU = %d, want 75", result[0].TotalCPUMilli)
	}
	// Verify pending pod has zero usage but populated requests.
	if result[1].TotalCPUMilli != 0 {
		t.Fatalf("pending pod CPU = %d, want 0", result[1].TotalCPUMilli)
	}
	if result[1].RequestCPU != 50 {
		t.Fatalf("pending pod RequestCPU = %d, want 50", result[1].RequestCPU)
	}
	// Verify failed pod included.
	if result[2].Phase != "Failed" {
		t.Fatalf("third pod Phase = %q, want Failed", result[2].Phase)
	}
}

func TestScrapePods_NilClients(t *testing.T) {
	tests := []struct {
		name    string
		clients *PodClients
	}{
		{"nil clients", nil},
		{"nil pods lister", &PodClients{Pods: nil, Metrics: &mockMetricsLister{}}},
		{"nil metrics lister", &PodClients{Pods: &mockPodLister{}, Metrics: nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ScrapePods(context.Background(), tt.clients)
			if err == nil {
				t.Fatal("expected error for nil clients")
			}
		})
	}
}

func TestScrapePods_PodListError(t *testing.T) {
	clients := &PodClients{
		Pods:    &mockPodLister{err: context.DeadlineExceeded},
		Metrics: &mockMetricsLister{},
	}

	_, err := ScrapePods(context.Background(), clients)
	if err == nil {
		t.Fatal("expected error when pod list fails")
	}
}

func TestScrapePods_MetricsListError(t *testing.T) {
	clients := &PodClients{
		Pods:    &mockPodLister{pods: []PodSpec{{Name: "p", Namespace: "ns"}}},
		Metrics: &mockMetricsLister{err: context.DeadlineExceeded},
	}

	_, err := ScrapePods(context.Background(), clients)
	if err == nil {
		t.Fatal("expected error when metrics list fails")
	}
}

func TestScrapePods_CrossNamespaceJoin(t *testing.T) {
	// Same pod name in different namespaces — metrics must match correctly.
	now := time.Now()
	clients := &PodClients{
		Pods: &mockPodLister{pods: []PodSpec{
			{Name: "api", Namespace: "prod", NodeName: "n1", Phase: "Running",
				Containers: []ContainerSpec{{Name: "app", RequestCPU: 100}}},
			{Name: "api", Namespace: "staging", NodeName: "n2", Phase: "Running",
				Containers: []ContainerSpec{{Name: "app", RequestCPU: 50}}},
		}},
		Metrics: &mockMetricsLister{usage: []PodUsage{
			{Name: "api", Namespace: "prod", Timestamp: now,
				Containers: []ContainerUsage{{Name: "app", CPUMilli: 90}}},
			{Name: "api", Namespace: "staging", Timestamp: now,
				Containers: []ContainerUsage{{Name: "app", CPUMilli: 30}}},
		}},
	}

	result, err := ScrapePods(context.Background(), clients)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d pods, want 2", len(result))
	}

	// prod/api should have CPU 90, staging/api should have CPU 30.
	for _, pm := range result {
		switch pm.Namespace {
		case "prod":
			if pm.TotalCPUMilli != 90 {
				t.Fatalf("prod/api CPU = %d, want 90", pm.TotalCPUMilli)
			}
		case "staging":
			if pm.TotalCPUMilli != 30 {
				t.Fatalf("staging/api CPU = %d, want 30", pm.TotalCPUMilli)
			}
		default:
			t.Fatalf("unexpected namespace: %s", pm.Namespace)
		}
	}
}
