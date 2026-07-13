package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer creates a server pointed at the given test URL.
func newTestServer(baseURL, apiKey string) *server {
	return &server{
		apiKey:   apiKey,
		baseURL:  baseURL,
		defSpace: "hive",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// ── spaceFor ──────────────────────────────────────────────────────────────────

func TestSpaceFor_usesArg(t *testing.T) {
	s := newTestServer("", "")
	space := s.spaceFor(map[string]any{"space": "myspace"})
	if space != "myspace" {
		t.Fatalf("want myspace, got %q", space)
	}
}

func TestSpaceFor_fallsBackToDefault(t *testing.T) {
	s := newTestServer("", "")
	space := s.spaceFor(map[string]any{})
	if space != "hive" {
		t.Fatalf("want hive, got %q", space)
	}
}

// ── toolIntend ────────────────────────────────────────────────────────────────

func TestToolIntend_missingTitle(t *testing.T) {
	s := newTestServer("", "")
	result := s.toolIntend(map[string]any{})
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(result.Content[0].Text, "title") {
		t.Fatalf("expected title error, got %q", result.Content[0].Text)
	}
}

func TestToolIntend_postsToAPI(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id":"node-1"}`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "test-key")
	result := s.toolIntend(map[string]any{
		"title":       "My Task",
		"description": "Do the thing",
		"kind":        "task",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content[0].Text)
	}
	if gotPath != "/app/hive/op" {
		t.Fatalf("wrong path: %q", gotPath)
	}
	if gotBody["op"] != "intend" {
		t.Fatalf("wrong op: %q", gotBody["op"])
	}
	if gotBody["title"] != "My Task" {
		t.Fatalf("wrong title: %q", gotBody["title"])
	}
}

// ── toolRespond ───────────────────────────────────────────────────────────────

func TestToolRespond_missingFields(t *testing.T) {
	s := newTestServer("", "")
	result := s.toolRespond(map[string]any{"parent_id": "x"})
	if !result.IsError {
		t.Fatal("expected error when body missing")
	}
}

func TestToolRespond_postsToAPI(t *testing.T) {
	var gotBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		fmt.Fprint(w, `{"id":"node-2"}`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	result := s.toolRespond(map[string]any{
		"parent_id": "node-1",
		"body":      "hello",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content[0].Text)
	}
	if gotBody["op"] != "respond" {
		t.Fatalf("wrong op: %q", gotBody["op"])
	}
	if gotBody["parent_id"] != "node-1" {
		t.Fatalf("wrong parent_id: %q", gotBody["parent_id"])
	}
}

// ── toolSearch ────────────────────────────────────────────────────────────────

func TestToolSearch_missingQuery(t *testing.T) {
	s := newTestServer("", "")
	result := s.toolSearch(map[string]any{})
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestToolSearch_encodesQueryParam(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `[]`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	result := s.toolSearch(map[string]any{"query": "hello world & more"})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content[0].Text)
	}
	// The query param must be URL-encoded — spaces become %20, & becomes %26.
	if !strings.Contains(gotQuery, "hello+world") && !strings.Contains(gotQuery, "hello%20world") {
		t.Fatalf("query not encoded properly: %q", gotQuery)
	}
	if strings.Contains(gotQuery, " ") {
		t.Fatalf("query contains raw space (not encoded): %q", gotQuery)
	}
}

// ── toolGetBoard ──────────────────────────────────────────────────────────────

func TestToolGetBoard_returnsBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":"t1"}]`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	result := s.toolGetBoard(map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "t1") {
		t.Fatalf("unexpected body: %q", result.Content[0].Text)
	}
}

// ── toolGetNode ───────────────────────────────────────────────────────────────

func TestToolGetNode_missingID(t *testing.T) {
	s := newTestServer("", "")
	result := s.toolGetNode(map[string]any{})
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestToolGetNode_rejectsSlashInID(t *testing.T) {
	s := newTestServer("", "")
	result := s.toolGetNode(map[string]any{"node_id": "../../etc/passwd"})
	if !result.IsError {
		t.Fatal("expected error for path-traversal node_id")
	}
}

func TestToolGetNode_encodesPath(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":"abc-123"}`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	result := s.toolGetNode(map[string]any{"node_id": "abc-123"})
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content[0].Text)
	}
	if gotPath != "/app/hive/node/abc-123" {
		t.Fatalf("wrong path: %q", gotPath)
	}
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func TestAPIGet_setsAuthHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, `ok`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "lv_test")
	s.apiGet("/test")
	if gotAuth != "Bearer lv_test" {
		t.Fatalf("wrong auth header: %q", gotAuth)
	}
}

func TestAPIGet_returnsErrorOn4xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `not found`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	_, err := s.apiGet("/missing")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error should mention status code: %v", err)
	}
}

func TestAPIGet_boundedRead(t *testing.T) {
	// Server streams more than maxResponseBytes — should truncate, not OOM.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write 2 MiB
		chunk := strings.Repeat("x", 64*1024)
		for i := 0; i < 32; i++ {
			fmt.Fprint(w, chunk)
		}
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	body, err := s.apiGet("/big")
	// No error expected (200 OK), but body should be capped.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) > maxResponseBytes {
		t.Fatalf("body exceeded cap: got %d bytes", len(body))
	}
}

func TestAPIPost_setsContentType(t *testing.T) {
	var gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		fmt.Fprint(w, `{}`)
	}))
	defer ts.Close()

	s := newTestServer(ts.URL, "")
	s.apiPost("/op", []byte(`{}`))
	if gotCT != "application/json" {
		t.Fatalf("wrong content-type: %q", gotCT)
	}
}

// ── protocol helpers ──────────────────────────────────────────────────────────

func TestOkResult(t *testing.T) {
	r := okResult("hello")
	if r.IsError {
		t.Fatal("should not be error")
	}
	if r.Content[0].Text != "hello" {
		t.Fatalf("wrong text: %q", r.Content[0].Text)
	}
}

func TestErrResult(t *testing.T) {
	r := errResult("boom")
	if !r.IsError {
		t.Fatal("should be error")
	}
	if r.Content[0].Text != "boom" {
		t.Fatalf("wrong text: %q", r.Content[0].Text)
	}
}

// TestNewServerDefaults pins the built-in defaults newServer applies when the
// (renamed) environment variables are unset: base URL falls back to the remote
// default and space to "hive"; an unset key stays empty (FO R3, packet AC-9).
func TestNewServerDefaults(t *testing.T) {
	t.Setenv("TRANSPARA_API_KEY", "")
	t.Setenv("TRANSPARA_BASE_URL", "")
	t.Setenv("TRANSPARA_SPACE", "")

	s := newServer()
	if s.baseURL != "https://transpara.ai" {
		t.Errorf("baseURL = %q, want https://transpara.ai", s.baseURL)
	}
	if s.defSpace != "hive" {
		t.Errorf("defSpace = %q, want hive", s.defSpace)
	}
	if s.apiKey != "" {
		t.Errorf("apiKey = %q, want empty", s.apiKey)
	}
}
