package middleware

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// logEntry is the structured JSON line emitted for each request.
type logEntry struct {
	Timestamp  string `json:"ts"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	RequestID  string `json:"request_id,omitempty"`
}

// statusWriter captures the response status code for logging.
// WriteHeader is called at most once; the default is 200.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

// Logger returns middleware that writes one JSON line per request to stderr.
// It reads the request ID from context (set by RequestID middleware), so
// RequestID should be applied first in the middleware chain.
func Logger(next http.Handler) http.Handler {
	enc := json.NewEncoder(os.Stderr)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}

		next.ServeHTTP(sw, r)

		_ = enc.Encode(logEntry{
			Timestamp:  start.UTC().Format(time.RFC3339),
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     sw.code,
			DurationMS: time.Since(start).Milliseconds(),
			RequestID:  GetRequestID(r.Context()),
		})
	})
}
