package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/anomaly"
	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

func seedStore() *store.Store {
	s := store.New()
	s.Push(store.Snapshot{
		Cluster: store.ClusterSummary{
			Nodes: 2, Pods: 3, Namespaces: 2,
			CPUUsage: 0.5, MemoryUsage: 0.6,
			Healthy: true, Severity: "ok",
			CollectedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Nodes: []store.NodeMetrics{
			{Name: "node-a", CPUCores: 4, CPUUsage: 0.3, MemoryBytes: 8e9, MemoryUsage: 0.4, PodCount: 2, Ready: true, CollectedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "node-b", CPUCores: 8, CPUUsage: 0.7, MemoryBytes: 16e9, MemoryUsage: 0.8, PodCount: 1, Ready: true, CollectedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		Pods: []store.PodMetrics{
			{Name: "web-1", Namespace: "default", Node: "node-a", Phase: "Running", CPUUsage: 0.1, MemoryBytes: 1e8, Ready: true, CollectedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "db-1", Namespace: "data", Node: "node-a", Phase: "Running", CPUUsage: 0.2, MemoryBytes: 2e8, Ready: true, CollectedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "cache-1", Namespace: "default", Node: "node-b", Phase: "Running", CPUUsage: 0.05, MemoryBytes: 5e7, Ready: true, CollectedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	return s
}

func request(t *testing.T, srv *http.Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)
	return rr
}

func TestHealthz(t *testing.T) {
	srv := NewServer(":0", store.New(), anomaly.NewDetector(config.DefaultAlertThreshold()))
	rr := request(t, srv, "GET", "/healthz")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if !body["ok"] {
		t.Fatal("expected ok=true")
	}
}

func TestCluster(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/cluster")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var c store.ClusterSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &c); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if c.Nodes != 2 {
		t.Fatalf("expected 2 nodes, got %d", c.Nodes)
	}
}

func TestClusterEmpty(t *testing.T) {
	srv := NewServer(":0", store.New(), anomaly.NewDetector(config.DefaultAlertThreshold()))
	rr := request(t, srv, "GET", "/api/v1/cluster")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestNodes(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/nodes")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var nodes []store.NodeMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestNodeByName(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/nodes/node-a")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var n store.NodeMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &n); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if n.Name != "node-a" {
		t.Fatalf("expected node-a, got %s", n.Name)
	}
}

func TestNodeNotFound(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/nodes/nonexistent")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestPods(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/pods")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var pods []store.PodMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &pods); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(pods) != 3 {
		t.Fatalf("expected 3 pods, got %d", len(pods))
	}
}

func TestPodsFilterByNamespace(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/pods?namespace=default")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var pods []store.PodMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &pods); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods in default ns, got %d", len(pods))
	}
}

func TestPodsFilterByNode(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/pods?node=node-b")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var pods []store.PodMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &pods); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod on node-b, got %d", len(pods))
	}
}

func TestAnomalies(t *testing.T) {
	d := anomaly.NewDetector(config.DefaultAlertThreshold())
	d.Record(anomaly.Anomaly{
		Severity: "warning", Category: "node", Resource: "node-x",
		Description: "CPU high", DetectedAt: time.Now(),
	})
	srv := NewServer(":0", store.New(), d)

	rr := request(t, srv, "GET", "/api/v1/anomalies")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var anomalies []anomaly.Anomaly
	if err := json.Unmarshal(rr.Body.Bytes(), &anomalies); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(anomalies) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(anomalies))
	}
}

func TestAnomaliesEmpty(t *testing.T) {
	srv := NewServer(":0", store.New(), anomaly.NewDetector(config.DefaultAlertThreshold()))
	rr := request(t, srv, "GET", "/api/v1/anomalies")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "[]\n" {
		t.Fatalf("expected empty array, got %s", rr.Body.String())
	}
}

func TestHistory(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/history?from=2024-01-01T00:00:00Z&to=2026-01-01T00:00:00Z")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var snaps []store.Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snaps); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
}

func TestHistoryMissingParams(t *testing.T) {
	srv := NewServer(":0", store.New(), anomaly.NewDetector(config.DefaultAlertThreshold()))
	rr := request(t, srv, "GET", "/api/v1/history")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHistoryBadTime(t *testing.T) {
	srv := NewServer(":0", store.New(), anomaly.NewDetector(config.DefaultAlertThreshold()))
	rr := request(t, srv, "GET", "/api/v1/history?from=not-a-time&to=2026-01-01T00:00:00Z")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestNodeHistory(t *testing.T) {
	st := seedStore()
	srv := NewServer(":0", st, anomaly.NewDetector(config.DefaultAlertThreshold()))

	rr := request(t, srv, "GET", "/api/v1/history/nodes/node-a?from=2024-01-01T00:00:00Z&to=2026-01-01T00:00:00Z")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var nodes []store.NodeMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node history entry, got %d", len(nodes))
	}
	if nodes[0].Name != "node-a" {
		t.Fatalf("expected node-a, got %s", nodes[0].Name)
	}
}

func TestNodesEmpty(t *testing.T) {
	srv := NewServer(":0", store.New(), anomaly.NewDetector(config.DefaultAlertThreshold()))
	rr := request(t, srv, "GET", "/api/v1/nodes")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "[]\n" {
		t.Fatalf("expected empty array, got %s", rr.Body.String())
	}
}
