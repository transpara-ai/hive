// Package api provides the HTTP server for metrics queries and health checks.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lovyou-ai/k3s-monitor/pkg/anomaly"
	"github.com/lovyou-ai/k3s-monitor/pkg/store"
)

// Server serves the k3s-monitor REST API.
type Server struct {
	store    store.Store
	detector anomaly.Detector
	handler  http.Handler
}

// New creates an API server wired to the given store and anomaly detector.
func New(s store.Store, d anomaly.Detector) *Server {
	srv := &Server{
		store:    s,
		detector: d,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", srv.handleHealth)
	mux.HandleFunc("GET /api/v1/nodes", srv.handleNodes)
	mux.HandleFunc("GET /api/v1/pods", srv.handlePods)
	mux.HandleFunc("GET /api/v1/history", srv.handleHistory)
	mux.HandleFunc("GET /api/v1/alerts", srv.handleAlerts)
	srv.handler = mux

	return srv
}

// Handler returns the http.Handler for use with http.Server or testing.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// --- response types ---

// HealthResponse is the JSON body for GET /api/v1/health.
type HealthResponse struct {
	Status    string `json:"status"` // "healthy", "degraded", "unhealthy"
	Nodes     int    `json:"nodes"`
	Pods      int    `json:"pods"`
	Alerts    int    `json:"alerts"`
	Timestamp string `json:"timestamp"`
}

// NodesResponse is the JSON body for GET /api/v1/nodes.
type NodesResponse struct {
	Nodes     []store.NodeMetrics `json:"nodes"`
	Timestamp string              `json:"timestamp"`
}

// PodsResponse is the JSON body for GET /api/v1/pods.
type PodsResponse struct {
	Pods      []store.PodMetrics `json:"pods"`
	Timestamp string             `json:"timestamp"`
}

// HistoryResponse is the JSON body for GET /api/v1/history.
type HistoryResponse struct {
	Snapshots []store.Snapshot `json:"snapshots"`
	Range     string           `json:"range"`
	Count     int              `json:"count"`
}

// AlertsResponse is the JSON body for GET /api/v1/alerts.
type AlertsResponse struct {
	Alerts []anomaly.Alert `json:"alerts"`
	Count  int             `json:"count"`
}

// --- handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	alerts := s.detector.Active()

	resp := HealthResponse{
		Status:    healthStatus(alerts),
		Alerts:    len(alerts),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if snap := s.store.Latest(); snap != nil {
		resp.Nodes = len(snap.Nodes)
		resp.Pods = len(snap.Pods)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeJSON(w, http.StatusOK, NodesResponse{
			Nodes:     []store.NodeMetrics{},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	writeJSON(w, http.StatusOK, NodesResponse{
		Nodes:     snap.Nodes,
		Timestamp: snap.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeJSON(w, http.StatusOK, PodsResponse{
			Pods:      []store.PodMetrics{},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	writeJSON(w, http.StatusOK, PodsResponse{
		Pods:      snap.Pods,
		Timestamp: snap.Timestamp.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "1h"
	}

	dur, err := parseDuration(rangeStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid range %q: %v", rangeStr, err),
		})
		return
	}

	now := time.Now().UTC()
	snapshots := s.store.Range(now.Add(-dur), now)

	writeJSON(w, http.StatusOK, HistoryResponse{
		Snapshots: snapshots,
		Range:     rangeStr,
		Count:     len(snapshots),
	})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := s.detector.Active()
	if alerts == nil {
		alerts = []anomaly.Alert{}
	}

	writeJSON(w, http.StatusOK, AlertsResponse{
		Alerts: alerts,
		Count:  len(alerts),
	})
}

// --- helpers ---

// healthStatus derives overall cluster status from active alerts.
func healthStatus(alerts []anomaly.Alert) string {
	if len(alerts) == 0 {
		return "healthy"
	}
	for _, a := range alerts {
		if a.Severity == anomaly.SeverityCritical {
			return "unhealthy"
		}
	}
	return "degraded"
}

// parseDuration extends time.ParseDuration to also support "d" (days) suffix.
func parseDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	// Handle day suffix: "7d" → 7 * 24h.
	if s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err != nil {
			return 0, fmt.Errorf("invalid day duration %q: %w", s, err)
		}
		if days <= 0 {
			return 0, fmt.Errorf("days must be positive, got %d", days)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %s", d)
	}
	return d, nil
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v) // encoding in-memory structs; error is unreachable
}
