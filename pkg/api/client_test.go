package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureBody records the last request body sent to the test server.
func captureBody(t *testing.T) (*httptest.Server, *[]byte) {
	t.Helper()
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		body = data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"node-123","kind":"task","title":"test","created_at":"","updated_at":""}}`))
	}))
	return srv, &body
}

func TestCreateTaskSendsCauses(t *testing.T) {
	srv, body := captureBody(t)
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.CreateTask("hive", "Fix: something", "details", "high", []string{"cause-node-1"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(*body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v\nbody: %s", err, *body)
	}

	rawCauses, ok := fields["causes"]
	if !ok {
		t.Fatalf("causes field missing from request body: %s", *body)
	}
	causes, ok := rawCauses.([]any)
	if !ok {
		t.Fatalf("causes is not an array: %T", rawCauses)
	}
	if len(causes) != 1 {
		t.Fatalf("causes len = %d, want 1", len(causes))
	}
	if causes[0] != "cause-node-1" {
		t.Errorf("causes[0] = %v, want %q", causes[0], "cause-node-1")
	}
}

func TestCreateTaskNilCausesOmitted(t *testing.T) {
	srv, body := captureBody(t)
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.CreateTask("hive", "New task", "", "medium", nil)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(*body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if _, ok := fields["causes"]; ok {
		t.Error("causes field should be absent when nil is passed")
	}
}

func TestCreateDocumentSendsCauses(t *testing.T) {
	srv, body := captureBody(t)
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.CreateDocument("hive", "Build: feat", "body text", []string{"task-abc"})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(*body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	rawCauses, ok := fields["causes"]
	if !ok {
		t.Fatalf("causes field missing: %s", *body)
	}
	causes, ok := rawCauses.([]any)
	if !ok || len(causes) != 1 || causes[0] != "task-abc" {
		t.Errorf("causes = %v, want [task-abc]", rawCauses)
	}
}

func TestAssertClaimSendsCauses(t *testing.T) {
	srv, body := captureBody(t)
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.AssertClaim("hive", "Lesson: foo", "details", []string{"claim-xyz"})
	if err != nil {
		t.Fatalf("AssertClaim: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(*body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	rawCauses, ok := fields["causes"]
	if !ok {
		t.Fatalf("causes field missing: %s", *body)
	}
	causes, ok := rawCauses.([]any)
	if !ok || len(causes) != 1 || causes[0] != "claim-xyz" {
		t.Errorf("causes = %v, want [claim-xyz]", rawCauses)
	}
}

func TestNextLessonNumberFromServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("op") != "max_lesson" {
			http.Error(w, "unexpected op", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Server reports max lesson 109 — next should be 110.
		_, _ = w.Write([]byte(`{"max_lesson":109}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	got := c.NextLessonNumber("hive")
	if got != 110 {
		t.Errorf("NextLessonNumber = %d, want 110 (max lesson 109 + 1)", got)
	}
}

func TestNextLessonNumberNoLessons(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"max_lesson":0}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	got := c.NextLessonNumber("hive")
	if got != 1 {
		t.Errorf("NextLessonNumber = %d, want 1 (no existing lessons)", got)
	}
}

func TestNextLessonNumberAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	got := c.NextLessonNumber("hive")
	if got != 1 {
		t.Errorf("NextLessonNumber on API error = %d, want 1 (safe default)", got)
	}
}

func TestNextLessonNumberMalformedJSON(t *testing.T) {
	// Simulates a proxy or CDN returning HTML on a 200 (e.g. rate-limit page).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html>Rate limited</html>`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	got := c.NextLessonNumber("hive")
	if got != 1 {
		t.Errorf("NextLessonNumber on malformed JSON = %d, want 1 (safe default)", got)
	}
}

func TestPostOpStringFieldsPreserved(t *testing.T) {
	srv, body := captureBody(t)
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.PostOp("hive", map[string]string{
		"op":      "claim",
		"node_id": "node-1",
	})
	if err != nil {
		t.Fatalf("PostOp: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(*body, &fields); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if fields["op"] != "claim" {
		t.Errorf("op = %v, want claim", fields["op"])
	}
	if fields["node_id"] != "node-1" {
		t.Errorf("node_id = %v, want node-1", fields["node_id"])
	}
}

// TestPostDiagnostic_SendsPayload verifies that PostDiagnostic hits the correct
// path with the raw payload, correct Content-Type, and Bearer auth.
func TestPostDiagnostic_SendsPayload(t *testing.T) {
	var (
		capturedPath string
		capturedCT   string
		capturedAuth string
		capturedBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedCT = r.Header.Get("Content-Type")
		capturedAuth = r.Header.Get("Authorization")
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	payload := []byte(`{"phase":"builder","outcome":"success","cost_usd":0.42}`)
	c := New(srv.URL, "test-key")
	if err := c.PostDiagnostic(payload); err != nil {
		t.Fatalf("PostDiagnostic: %v", err)
	}

	if capturedPath != "/api/hive/diagnostic" {
		t.Errorf("path = %q, want /api/hive/diagnostic", capturedPath)
	}
	if capturedCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedCT)
	}
	if capturedAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", capturedAuth)
	}
	if string(capturedBody) != string(payload) {
		t.Errorf("body = %q, want %q", capturedBody, payload)
	}
}

// TestPostDiagnostic_Error4xx verifies that a 4xx response is returned as an error.
func TestPostDiagnostic_Error4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New(srv.URL, "bad-key")
	err := c.PostDiagnostic([]byte(`{"phase":"builder"}`))
	if err == nil {
		t.Error("PostDiagnostic with 401: expected error, got nil")
	}
}

// TestNodeExists_Returns200_ReturnsTrue verifies that a 200 response means the node exists.
func TestNodeExists_Returns200_ReturnsTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"node-abc","kind":"task"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if !c.NodeExists("hive", "node-abc") {
		t.Error("NodeExists with 200: expected true")
	}
}

// TestNodeExists_Returns404_ReturnsFalse verifies that a 404 response means the node does not exist.
func TestNodeExists_Returns404_ReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if c.NodeExists("hive", "ghost-node") {
		t.Error("NodeExists with 404: expected false")
	}
}

// TestNodeExists_Returns500_ReturnsFalse verifies that server errors are treated as non-existence.
func TestNodeExists_Returns500_ReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if c.NodeExists("hive", "some-node") {
		t.Error("NodeExists with 500: expected false")
	}
}

// TestNodeExists_URLFormat verifies the request hits the correct path with format=json.
func TestNodeExists_URLFormat(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	c.NodeExists("hive", "node-xyz")

	wantPath := "/app/hive/node/node-xyz"
	if capturedPath != wantPath {
		t.Errorf("request path = %q, want %q", capturedPath, wantPath)
	}
	if capturedQuery != "format=json" {
		t.Errorf("query = %q, want format=json", capturedQuery)
	}
}

// TestNodeExists_SendsBearerAuth verifies that the Authorization header is sent.
func TestNodeExists_SendsBearerAuth(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "lv_testkey")
	c.NodeExists("hive", "node-1")

	if capturedAuth != "Bearer lv_testkey" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer lv_testkey")
	}
}

// TestNodeExists_UsesGETMethod verifies the request uses GET (not POST).
func TestNodeExists_UsesGETMethod(t *testing.T) {
	var capturedMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	c.NodeExists("hive", "node-1")

	if capturedMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", capturedMethod)
	}
}

// TestNodeExists_NetworkError verifies that network errors are treated as non-existence.
// This guards against the client incorrectly raising an error when a node can't be validated.
func TestNodeExists_NetworkError(t *testing.T) {
	// Use a listener that immediately closes to simulate a network failure.
	c := New("http://invalid-host-that-does-not-exist-12345.local", "test-key")
	if c.NodeExists("hive", "node-1") {
		t.Error("NodeExists with network error: expected false")
	}
}
