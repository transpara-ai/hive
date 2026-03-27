package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard format", "# Build: Fix: foo bar\n\nmore content", "Fix: foo bar"},
		{"heading only", "# Some Title\nbody", "Some Title"},
		{"leading blank lines", "\n\n# Build: Hello\n", "Hello"},
		{"empty input", "", ""},
		{"whitespace only", "   \n  \n", ""},
		{"multi-hash", "## Build: Nested", "Nested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTitle([]byte(tt.input))
			if got != tt.want {
				t.Errorf("buildTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestPostCreatesNode verifies that the post() function sends op=express with
// kind=post, title, and body to /app/hive/op.
func TestPostCreatesNode(t *testing.T) {
	var received map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/hive/op" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"test-id"}}`))
	}))
	defer srv.Close()

	err := post("lv_testkey", srv.URL, "Fix: some bug", "## What Was Built\nFixed the bug.")
	if err != nil {
		t.Fatalf("post() error: %v", err)
	}

	if received["op"] != "express" {
		t.Errorf("op = %q, want %q", received["op"], "express")
	}
	if received["kind"] != "post" {
		t.Errorf("kind = %q, want %q", received["kind"], "post")
	}
	if received["title"] != "Fix: some bug" {
		t.Errorf("title = %q, want %q", received["title"], "Fix: some bug")
	}
	if received["body"] == "" {
		t.Error("body is empty, want non-empty build summary")
	}
}

// TestBuildTitleExtractedOnPost verifies that buildTitle + post produces a
// feed node whose title comes from build.md (not just "Iteration N").
func TestBuildTitleExtractedOnPost(t *testing.T) {
	buildMD := []byte("# Build: Fix: Observer AllowedTools missing knowledge.search\n\n## What Was Built\nFixed it.")

	var receivedTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &payload)
		if payload["op"] == "express" {
			receivedTitle = payload["title"]
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"test-id"}}`))
	}))
	defer srv.Close()

	title := buildTitle(buildMD)
	if title == "" {
		t.Fatal("buildTitle returned empty for valid build.md")
	}

	if err := post("lv_testkey", srv.URL, title, string(buildMD)); err != nil {
		t.Fatalf("post() error: %v", err)
	}

	want := "Fix: Observer AllowedTools missing knowledge.search"
	if receivedTitle != want {
		t.Errorf("post title = %q, want %q", receivedTitle, want)
	}
}
