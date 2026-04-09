package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lovyou-ai/k3s-monitor/pkg/anomaly"
	"github.com/lovyou-ai/k3s-monitor/pkg/store"
)

// --- mocks ---

type mockStore struct {
	latest    *store.Snapshot
	snapshots []store.Snapshot
}

func (m *mockStore) Latest() *store.Snapshot     { return m.latest }
func (m *mockStore) Range(_, _ time.Time) []store.Snapshot { return m.snapshots }

type mockDetector struct {
	alerts []anomaly.Alert
}

func (m *mockDetector) Active() []anomaly.Alert { return m.alerts }

// --- fixtures ---

var (
	fixedTime = time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)

	sampleNodes = []store.NodeMetrics{
		{Name: "node-1", CPUPercent: 45.2, MemoryPercent: 62.0, DiskPercent: 30.0, CPUCores: 4, MemoryBytes: 8 << 30, DiskBytes: 100 << 30},
		{Name: "node-2", CPUPercent: 78.5, MemoryPercent: 81.0, DiskPercent: 55.0, CPUCores: 8, MemoryBytes: 16 << 30, DiskBytes: 200 << 30},
	}

	samplePods = []store.PodMetrics{
		{Name: "api-7b4f", Namespace: "default", CPUPercent: 12.0, MemoryPercent: 25.0, MemoryBytes: 256 << 20, Status: "Running", Restarts: 0},
		{Name: "worker-9c2a", Namespace: "jobs", CPUPercent: 88.0, MemoryPercent: 70.0, MemoryBytes: 512 << 20, Status: "Running", Restarts: 3},
	}

	sampleSnapshot = &store.Snapshot{
		Timestamp: fixedTime,
		Nodes:     sampleNodes,
		Pods:      samplePods,
	}

	warningAlert = anomaly.Alert{
		ID: "alert-1", Severity: anomaly.SeverityWarning,
		Resource: "node/node-2", Metric: "memory", Value: 81.0, Threshold: 80.0,
		Message: "node-2 memory at 81.0%", Since: fixedTime,
	}

	criticalAlert = anomaly.Alert{
		ID: "alert-2", Severity: anomaly.SeverityCritical,
		Resource: "pod/jobs/worker-9c2a", Metric: "cpu", Value: 88.0, Threshold: 80.0,
		Message: "worker-9c2a CPU at 88.0%", Since: fixedTime,
	}
)

// --- tests ---

func TestHealthEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		latest     *store.Snapshot
		alerts     []anomaly.Alert
		wantStatus string
		wantNodes  int
		wantPods   int
		wantAlerts int
	}{
		{
			name:       "healthy cluster with data",
			latest:     sampleSnapshot,
			alerts:     nil,
			wantStatus: "healthy",
			wantNodes:  2,
			wantPods:   2,
			wantAlerts: 0,
		},
		{
			name:       "no data collected yet",
			latest:     nil,
			alerts:     nil,
			wantStatus: "healthy",
			wantNodes:  0,
			wantPods:   0,
			wantAlerts: 0,
		},
		{
			name:       "degraded with warning",
			latest:     sampleSnapshot,
			alerts:     []anomaly.Alert{warningAlert},
			wantStatus: "degraded",
			wantNodes:  2,
			wantPods:   2,
			wantAlerts: 1,
		},
		{
			name:       "unhealthy with critical",
			latest:     sampleSnapshot,
			alerts:     []anomaly.Alert{warningAlert, criticalAlert},
			wantStatus: "unhealthy",
			wantNodes:  2,
			wantPods:   2,
			wantAlerts: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&mockStore{latest: tt.latest}, &mockDetector{alerts: tt.alerts})

			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/health", nil))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var resp HealthResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if resp.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", resp.Status, tt.wantStatus)
			}
			if resp.Nodes != tt.wantNodes {
				t.Errorf("nodes = %d, want %d", resp.Nodes, tt.wantNodes)
			}
			if resp.Pods != tt.wantPods {
				t.Errorf("pods = %d, want %d", resp.Pods, tt.wantPods)
			}
			if resp.Alerts != tt.wantAlerts {
				t.Errorf("alerts = %d, want %d", resp.Alerts, tt.wantAlerts)
			}
			if resp.Timestamp == "" {
				t.Error("timestamp should not be empty")
			}
		})
	}
}

func TestNodesEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		latest    *store.Snapshot
		wantCount int
	}{
		{name: "with nodes", latest: sampleSnapshot, wantCount: 2},
		{name: "no data yet", latest: nil, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&mockStore{latest: tt.latest}, &mockDetector{})

			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/nodes", nil))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var resp NodesResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if len(resp.Nodes) != tt.wantCount {
				t.Errorf("node count = %d, want %d", len(resp.Nodes), tt.wantCount)
			}
			if resp.Timestamp == "" {
				t.Error("timestamp should not be empty")
			}
		})
	}
}

func TestPodsEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		latest    *store.Snapshot
		wantCount int
	}{
		{name: "with pods", latest: sampleSnapshot, wantCount: 2},
		{name: "no data yet", latest: nil, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&mockStore{latest: tt.latest}, &mockDetector{})

			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/pods", nil))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var resp PodsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if len(resp.Pods) != tt.wantCount {
				t.Errorf("pod count = %d, want %d", len(resp.Pods), tt.wantCount)
			}
		})
	}
}

func TestHistoryEndpoint(t *testing.T) {
	snaps := []store.Snapshot{
		{Timestamp: fixedTime.Add(-30 * time.Minute), Nodes: sampleNodes, Pods: samplePods},
		{Timestamp: fixedTime, Nodes: sampleNodes, Pods: samplePods},
	}

	tests := []struct {
		name       string
		query      string
		snapshots  []store.Snapshot
		wantCode   int
		wantCount  int
		wantRange  string
	}{
		{name: "default range", query: "", snapshots: snaps, wantCode: 200, wantCount: 2, wantRange: "1h"},
		{name: "explicit range", query: "range=2h", snapshots: snaps, wantCode: 200, wantCount: 2, wantRange: "2h"},
		{name: "day range", query: "range=7d", snapshots: snaps, wantCode: 200, wantCount: 2, wantRange: "7d"},
		{name: "empty store", query: "range=1h", snapshots: nil, wantCode: 200, wantCount: 0, wantRange: "1h"},
		{name: "invalid range", query: "range=abc", snapshots: nil, wantCode: 400},
		{name: "negative range", query: "range=-1h", snapshots: nil, wantCode: 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&mockStore{snapshots: tt.snapshots}, &mockDetector{})

			url := "/api/v1/history"
			if tt.query != "" {
				url += "?" + tt.query
			}

			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", url, nil))

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tt.wantCode, w.Body.String())
			}

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp HistoryResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if resp.Count != tt.wantCount {
				t.Errorf("count = %d, want %d", resp.Count, tt.wantCount)
			}
			if resp.Range != tt.wantRange {
				t.Errorf("range = %q, want %q", resp.Range, tt.wantRange)
			}
		})
	}
}

func TestAlertsEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		alerts    []anomaly.Alert
		wantCount int
	}{
		{name: "no alerts", alerts: nil, wantCount: 0},
		{name: "with alerts", alerts: []anomaly.Alert{warningAlert, criticalAlert}, wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := New(&mockStore{}, &mockDetector{alerts: tt.alerts})

			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/alerts", nil))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var resp AlertsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if resp.Count != tt.wantCount {
				t.Errorf("count = %d, want %d", resp.Count, tt.wantCount)
			}
			if len(resp.Alerts) != tt.wantCount {
				t.Errorf("alert slice len = %d, want %d", len(resp.Alerts), tt.wantCount)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{input: "1h", want: time.Hour},
		{input: "30m", want: 30 * time.Minute},
		{input: "2h30m", want: 2*time.Hour + 30*time.Minute},
		{input: "7d", want: 7 * 24 * time.Hour},
		{input: "1d", want: 24 * time.Hour},
		{input: "", wantErr: true},
		{input: "abc", wantErr: true},
		{input: "-1h", wantErr: true},
		{input: "0s", wantErr: true},
		{input: "0d", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseDuration(%q) error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContentTypeJSON(t *testing.T) {
	srv := New(&mockStore{}, &mockDetector{})

	endpoints := []string{
		"/api/v1/health",
		"/api/v1/nodes",
		"/api/v1/pods",
		"/api/v1/history",
		"/api/v1/alerts",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, httptest.NewRequest("GET", ep, nil))

			ct := w.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}
		})
	}
}

func TestHealthStatus(t *testing.T) {
	tests := []struct {
		name   string
		alerts []anomaly.Alert
		want   string
	}{
		{name: "no alerts", alerts: nil, want: "healthy"},
		{name: "empty slice", alerts: []anomaly.Alert{}, want: "healthy"},
		{name: "warning only", alerts: []anomaly.Alert{warningAlert}, want: "degraded"},
		{name: "critical", alerts: []anomaly.Alert{criticalAlert}, want: "unhealthy"},
		{name: "mixed", alerts: []anomaly.Alert{warningAlert, criticalAlert}, want: "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := healthStatus(tt.alerts)
			if got != tt.want {
				t.Errorf("healthStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
