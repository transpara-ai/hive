package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

func defaultEngine() *Engine {
	return NewEngine(config.DefaultAlertThreshold())
}

func TestClassify_States(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		warn      float64
		crit      float64
		wantState AlertState
	}{
		{"below warning", 0.50, 0.80, 0.95, AlertOK},
		{"at warning", 0.80, 0.80, 0.95, AlertWarning},
		{"between warning and critical", 0.90, 0.80, 0.95, AlertWarning},
		{"at critical", 0.95, 0.80, 0.95, AlertCritical},
		{"above critical", 0.99, 0.80, 0.95, AlertCritical},
		{"zero value", 0.00, 0.80, 0.95, AlertOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := classify(tt.value, tt.warn, tt.crit, ResourceCPU, TargetNode, "test-node", "", time.Now())
			if a.State != tt.wantState {
				t.Errorf("classify(%.2f, %.2f, %.2f) = %s, want %s", tt.value, tt.warn, tt.crit, a.State, tt.wantState)
			}
		})
	}
}

func TestEvaluate_NodeAlerts(t *testing.T) {
	e := defaultEngine()
	snap := store.Snapshot{
		Nodes: []store.NodeMetrics{
			{Name: "node-ok", CPUUsage: 0.50, MemoryUsage: 0.40, DiskUsage: 0.30},
			{Name: "node-warn", CPUUsage: 0.85, MemoryUsage: 0.82, DiskUsage: 0.87},
			{Name: "node-crit", CPUUsage: 0.97, MemoryUsage: 0.96, DiskUsage: 0.98},
		},
		Timestamp: time.Now(),
	}

	alerts := e.Evaluate(context.Background(), snap)

	// 3 nodes x 3 resources = 9 alerts total
	if len(alerts) != 9 {
		t.Fatalf("got %d alerts, want 9", len(alerts))
	}

	states := countStates(alerts)
	if states[AlertOK] != 3 {
		t.Errorf("OK count = %d, want 3", states[AlertOK])
	}
	if states[AlertWarning] != 3 {
		t.Errorf("Warning count = %d, want 3", states[AlertWarning])
	}
	if states[AlertCritical] != 3 {
		t.Errorf("Critical count = %d, want 3", states[AlertCritical])
	}
}

func TestEvaluate_DiskUnavailable(t *testing.T) {
	e := defaultEngine()
	snap := store.Snapshot{
		Nodes: []store.NodeMetrics{
			{Name: "no-disk", CPUUsage: 0.50, MemoryUsage: 0.40, DiskUsage: -1},
		},
	}

	alerts := e.Evaluate(context.Background(), snap)
	// Should get CPU + Memory but NOT disk
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2 (disk skipped)", len(alerts))
	}
	for _, a := range alerts {
		if a.Resource == ResourceDisk {
			t.Error("disk alert should not be generated when DiskUsage is -1")
		}
	}
}

func TestEvaluate_PodAlerts(t *testing.T) {
	e := defaultEngine()
	snap := store.Snapshot{
		Pods: []store.PodMetrics{
			{Name: "pod-ok", Namespace: "default", CPUUsage: 0.1, CPULimit: 1.0, MemoryBytes: 100, MemoryLimit: 1000},
			{Name: "pod-warn", Namespace: "default", CPUUsage: 0.85, CPULimit: 1.0, MemoryBytes: 850, MemoryLimit: 1000},
			{Name: "pod-no-limit", Namespace: "default", CPUUsage: 0.5, CPULimit: 0, MemoryBytes: 500, MemoryLimit: 0},
		},
	}

	alerts := e.Evaluate(context.Background(), snap)

	// pod-ok: 2 (cpu ok, mem ok), pod-warn: 2 (cpu warn, mem warn), pod-no-limit: 0 (no limits)
	if len(alerts) != 4 {
		t.Fatalf("got %d alerts, want 4", len(alerts))
	}
}

func TestAlertRule_Override(t *testing.T) {
	e := defaultEngine()

	err := e.AddRule(AlertRule{
		TargetKind: TargetNode,
		TargetName: "high-cpu-node",
		Resource:   ResourceCPU,
		Warning:    0.50,
		Critical:   0.70,
	})
	if err != nil {
		t.Fatal(err)
	}

	snap := store.Snapshot{
		Nodes: []store.NodeMetrics{
			{Name: "high-cpu-node", CPUUsage: 0.60, MemoryUsage: 0.10, DiskUsage: 0.10},
			{Name: "normal-node", CPUUsage: 0.60, MemoryUsage: 0.10, DiskUsage: 0.10},
		},
	}

	alerts := e.Evaluate(context.Background(), snap)

	// Find the CPU alert for each node.
	for _, a := range alerts {
		if a.Resource != ResourceCPU {
			continue
		}
		switch a.TargetName {
		case "high-cpu-node":
			if a.State != AlertWarning {
				t.Errorf("high-cpu-node should be Warning with custom rule, got %s", a.State)
			}
		case "normal-node":
			if a.State != AlertOK {
				t.Errorf("normal-node should be OK with default thresholds, got %s", a.State)
			}
		}
	}
}

func TestAlertRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    AlertRule
		wantErr bool
	}{
		{"valid", AlertRule{TargetKind: TargetNode, Resource: ResourceCPU, Warning: 0.80, Critical: 0.95}, false},
		{"warning > critical", AlertRule{TargetKind: TargetNode, Resource: ResourceCPU, Warning: 0.95, Critical: 0.80}, true},
		{"warning out of range", AlertRule{TargetKind: TargetNode, Resource: ResourceCPU, Warning: 1.5, Critical: 0.95}, true},
		{"missing resource", AlertRule{TargetKind: TargetNode, Warning: 0.80, Critical: 0.95}, true},
		{"missing target kind", AlertRule{Resource: ResourceCPU, Warning: 0.80, Critical: 0.95}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWebhook_Delivery(t *testing.T) {
	var mu sync.Mutex
	var received *WebhookPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type application/json")
		}
		if r.Header.Get("X-Custom") != "test-value" {
			t.Error("expected custom header X-Custom: test-value")
		}

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode webhook payload: %v", err)
			return
		}
		mu.Lock()
		received = &payload
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := defaultEngine()
	err := e.AddWebhook(WebhookConfig{
		URL:     srv.URL,
		Headers: map[string]string{"X-Custom": "test-value"},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	snap := store.Snapshot{
		Nodes: []store.NodeMetrics{
			{Name: "hot-node", CPUUsage: 0.97, MemoryUsage: 0.40, DiskUsage: -1},
		},
	}

	e.Evaluate(context.Background(), snap)

	mu.Lock()
	defer mu.Unlock()
	if received == nil {
		t.Fatal("webhook was not called")
	}
	if received.AlertCount != 1 {
		t.Errorf("webhook alert_count = %d, want 1", received.AlertCount)
	}
	if received.Alerts[0].State != AlertCritical {
		t.Errorf("webhook alert state = %s, want critical", received.Alerts[0].State)
	}
}

func TestWebhook_NotFiredForOK(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := defaultEngine()
	if err := e.AddWebhook(WebhookConfig{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}

	snap := store.Snapshot{
		Nodes: []store.NodeMetrics{
			{Name: "healthy", CPUUsage: 0.10, MemoryUsage: 0.10, DiskUsage: 0.10},
		},
	}

	e.Evaluate(context.Background(), snap)

	if called {
		t.Error("webhook should not fire when all alerts are OK")
	}
}

func TestWebhook_EmptyURL(t *testing.T) {
	e := defaultEngine()
	err := e.AddWebhook(WebhookConfig{URL: ""})
	if err == nil {
		t.Error("expected error for empty webhook URL")
	}
}

func TestAlerts_ReturnsCopy(t *testing.T) {
	e := defaultEngine()
	snap := store.Snapshot{
		Nodes: []store.NodeMetrics{
			{Name: "n1", CPUUsage: 0.50, MemoryUsage: 0.50, DiskUsage: 0.50},
		},
	}
	e.Evaluate(context.Background(), snap)

	a1 := e.Alerts()
	a2 := e.Alerts()

	if len(a1) == 0 {
		t.Fatal("expected alerts")
	}

	// Mutating the returned slice should not affect the engine's internal state.
	a1[0].State = "mutated"
	if a2[0].State == "mutated" {
		t.Error("Alerts() should return a copy, not a reference")
	}
}

func countStates(alerts []Alert) map[AlertState]int {
	m := make(map[AlertState]int)
	for _, a := range alerts {
		m[a.State]++
	}
	return m
}
