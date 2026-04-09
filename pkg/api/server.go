package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lovyou-ai/hive/pkg/anomaly"
	"github.com/lovyou-ai/hive/pkg/store"
)

// NewServer creates an HTTP server with all health API routes.
// The server reads from the store and detector; it never writes.
func NewServer(addr string, st *store.Store, detector *anomaly.Detector) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /api/v1/cluster", clusterHandler(st))
	mux.HandleFunc("GET /api/v1/nodes", nodesHandler(st))
	mux.HandleFunc("GET /api/v1/nodes/{name}", nodeHandler(st))
	mux.HandleFunc("GET /api/v1/pods", podsHandler(st))
	mux.HandleFunc("GET /api/v1/anomalies", anomaliesHandler(detector))
	mux.HandleFunc("GET /api/v1/history", historyHandler(st))
	mux.HandleFunc("GET /api/v1/history/nodes/{name}", nodeHistoryHandler(st))

	return &http.Server{
		Addr:    addr,
		Handler: logMiddleware(mux),
	}
}

// ListenAndServe starts the server and handles graceful shutdown on SIGINT/SIGTERM.
func ListenAndServe(srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("received %v, draining...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// --- handlers ---

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func clusterHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		c := st.Cluster()
		if c == nil {
			writeErr(w, http.StatusNotFound, "no cluster data available")
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func nodesHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		nodes := st.Nodes()
		if nodes == nil {
			nodes = []store.NodeMetrics{}
		}
		writeJSON(w, http.StatusOK, nodes)
	}
}

func nodeHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		n := st.Node(name)
		if n == nil {
			writeErr(w, http.StatusNotFound, fmt.Sprintf("node %q not found", name))
			return
		}
		writeJSON(w, http.StatusOK, n)
	}
}

func podsHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pods []store.PodMetrics
		ns := r.URL.Query().Get("namespace")
		node := r.URL.Query().Get("node")
		switch {
		case ns != "":
			pods = st.PodsByNamespace(ns)
		case node != "":
			pods = st.PodsByNode(node)
		default:
			pods = st.Pods()
		}
		if pods == nil {
			pods = []store.PodMetrics{}
		}
		writeJSON(w, http.StatusOK, pods)
	}
}

func anomaliesHandler(d *anomaly.Detector) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		a := d.Anomalies()
		if a == nil {
			a = []anomaly.Anomaly{}
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func historyHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, to, err := parseTimeRange(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		snaps := st.History(from, to)
		if snaps == nil {
			snaps = []store.Snapshot{}
		}
		writeJSON(w, http.StatusOK, snaps)
	}
}

func nodeHistoryHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		from, to, err := parseTimeRange(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		nodes := st.NodeHistory(name, from, to)
		if nodes == nil {
			nodes = []store.NodeMetrics{}
		}
		writeJSON(w, http.StatusOK, nodes)
	}
}

// --- helpers ---

func parseTimeRange(r *http.Request) (time.Time, time.Time, error) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("both 'from' and 'to' query parameters are required (RFC3339)")
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid 'from' time: %w", err)
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid 'to' time: %w", err)
	}
	return from, to, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// statusWriter captures the response status code for logging.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

func logMiddleware(next http.Handler) http.Handler {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start))
	})
}
