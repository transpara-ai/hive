package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLogger_EmitsJSONLine(t *testing.T) {
	// Capture stderr output.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)

	// Inject a request ID via context so the logger can read it.
	ctx := context.WithValue(req.Context(), requestIDKey{}, "test-req-123")
	req = req.WithContext(ctx)

	Logger(inner).ServeHTTP(rr, req)

	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}

	if entry.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", entry.Method)
	}
	if entry.Path != "/api/v1/users" {
		t.Errorf("path = %q, want /api/v1/users", entry.Path)
	}
	if entry.Status != http.StatusCreated {
		t.Errorf("status = %d, want 201", entry.Status)
	}
	if entry.RequestID != "test-req-123" {
		t.Errorf("request_id = %q, want test-req-123", entry.RequestID)
	}
	if entry.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
	if entry.DurationMS < 0 {
		t.Errorf("duration_ms = %d, want >= 0", entry.DurationMS)
	}
}

func TestLogger_DefaultStatus200(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	// Handler that writes body but never calls WriteHeader.
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	Logger(inner).ServeHTTP(rr, req)

	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	var entry logEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", entry.Status)
	}
}
