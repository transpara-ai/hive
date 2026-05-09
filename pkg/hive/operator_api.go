package hive

import (
	"encoding/json"
	"net/http"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
)

// NewOperatorProjectionServer returns a read-only HTTP API for Site operator
// projections. It exposes derived EventGraph state only; it has no mutation
// routes.
func NewOperatorProjectionServer(s store.Store, apiKey string, limit int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/hive/operator-projection", func(w http.ResponseWriter, r *http.Request) {
		if apiKey != "" && r.Header.Get("Authorization") != "Bearer "+apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeOperatorProjectionJSON(w, BuildOperatorProjection(s, limit))
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func writeOperatorProjectionJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(v)
}
