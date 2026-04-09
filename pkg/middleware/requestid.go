// Package middleware provides reusable HTTP middleware for the hive user-api.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Header name for the request ID.
const HeaderRequestID = "X-Request-ID"

// requestIDKey is an unexported type to prevent context key collisions.
type requestIDKey struct{}

// RequestID returns middleware that injects X-Request-ID into the request
// context and response header. If the incoming request already carries the
// header, its value is reused; otherwise a new UUID v4 is generated.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = uuid.New().String()
		}

		// Propagate to response so callers can correlate.
		w.Header().Set(HeaderRequestID, id)

		// Store in context for downstream handlers and middleware.
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from the context. Returns "" if none.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
