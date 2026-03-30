package membrane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPServiceClientPoll(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/leads" {
			t.Errorf("path = %q, want /api/leads", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	client := NewHTTPServiceClient(srv.URL, "api_key", map[string]string{"key": "test-key"})
	resp, err := client.Get(context.Background(), "/api/leads")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !called {
		t.Error("server not called")
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
}

func TestHTTPServiceClientPost(t *testing.T) {
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	client := NewHTTPServiceClient(srv.URL, "bearer", map[string]string{"token": "test-token"})
	body := map[string]string{"action": "approve"}
	resp, err := client.Post(context.Background(), "/api/action", body)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
	if receivedBody["action"] != "approve" {
		t.Errorf("body action = %q, want approve", receivedBody["action"])
	}
}

func TestHTTPServiceClientAuthHeaders(t *testing.T) {
	tests := []struct {
		method     string
		config     map[string]string
		wantHeader string
		wantValue  string
	}{
		{"api_key", map[string]string{"key": "sk-123"}, "X-Api-Key", "sk-123"},
		{"bearer", map[string]string{"token": "tok-456"}, "Authorization", "Bearer tok-456"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got := r.Header.Get(tt.wantHeader)
				if got != tt.wantValue {
					t.Errorf("header %q = %q, want %q", tt.wantHeader, got, tt.wantValue)
				}
				json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
			}))
			defer srv.Close()

			client := NewHTTPServiceClient(srv.URL, tt.method, tt.config)
			client.Get(context.Background(), "/test")
		})
	}
}

func TestHTTPServiceClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPServiceClient(srv.URL, "api_key", map[string]string{"key": "k"})
	client.Timeout = 100 * time.Millisecond

	ctx := context.Background()
	_, err := client.Get(ctx, "/slow")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
