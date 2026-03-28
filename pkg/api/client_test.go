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
