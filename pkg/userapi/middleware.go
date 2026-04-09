package userapi

import (
	"log"
	"net/http"
	"os"
	"time"
)

// statusWriter captures the response status code for logging.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

// LogMiddleware logs method, path, status, and duration for each request.
func LogMiddleware(next http.Handler) http.Handler {
	logger := log.New(os.Stderr, "user-api ", log.LstdFlags)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start))
	})
}

// RecoverMiddleware catches panics and returns 500 instead of crashing.
func RecoverMiddleware(next http.Handler) http.Handler {
	logger := log.New(os.Stderr, "user-api ", log.LstdFlags)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Printf("PANIC: %s %s: %v", r.Method, r.URL.Path, err)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain applies middleware in order: the first middleware wraps the outermost
// layer, so it executes first on the request path.
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
