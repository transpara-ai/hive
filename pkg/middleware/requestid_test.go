package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id == "" {
			t.Fatal("expected non-empty request ID in context")
		}
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	RequestID(inner).ServeHTTP(rr, req)

	got := rr.Header().Get(HeaderRequestID)
	if got == "" {
		t.Fatal("expected X-Request-ID in response header")
	}
}

func TestRequestID_PreservesExisting(t *testing.T) {
	const existing = "my-trace-id-123"

	var ctxID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = GetRequestID(r.Context())
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, existing)

	RequestID(inner).ServeHTTP(rr, req)

	if ctxID != existing {
		t.Fatalf("context ID = %q, want %q", ctxID, existing)
	}
	if got := rr.Header().Get(HeaderRequestID); got != existing {
		t.Fatalf("response header = %q, want %q", got, existing)
	}
}

func TestGetRequestID_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := GetRequestID(req.Context()); id != "" {
		t.Fatalf("expected empty string, got %q", id)
	}
}
