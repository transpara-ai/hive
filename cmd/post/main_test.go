package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestSyncClaimsWritesFile verifies that syncClaims fetches claims from the API
// and writes them as markdown to the given output path.
func TestSyncClaimsWritesFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/hive/knowledge" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"claims": []map[string]any{
				{
					"title":      "Absence is invisible to traversal",
					"body":       "The Scout traverses what exists. Tests don't exist, so the Scout never encounters them.",
					"state":      "claimed",
					"author":     "Reflector",
					"created_at": "2026-03-01T00:00:00Z",
				},
				{
					"title":      "Ship what you build",
					"body":       "Every build iteration should deploy.",
					"state":      "verified",
					"author":     "Builder",
					"created_at": "2026-03-02T00:00:00Z",
				},
			},
		})
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "claims.md")
	if err := syncClaims("lv_testkey", srv.URL, outPath); err != nil {
		t.Fatalf("syncClaims() error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("claims.md not written: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "# Knowledge Claims") {
		t.Error("missing heading")
	}
	if !strings.Contains(content, "Absence is invisible to traversal") {
		t.Error("missing first claim title")
	}
	if !strings.Contains(content, "Ship what you build") {
		t.Error("missing second claim title")
	}
	if !strings.Contains(content, "Every build iteration should deploy") {
		t.Error("missing second claim body")
	}
	if !strings.Contains(content, "**State:** verified") {
		t.Error("missing state for verified claim")
	}
}

// TestSyncClaimsEmptyDoesNotWrite verifies that syncClaims does not write a
// file when the API returns zero claims.
func TestSyncClaimsEmptyDoesNotWrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"claims": []any{}})
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "claims.md")
	if err := syncClaims("lv_testkey", srv.URL, outPath); err != nil {
		t.Fatalf("syncClaims() error: %v", err)
	}

	if _, err := os.Stat(outPath); err == nil {
		t.Error("claims.md should not be written when there are no claims")
	}
}

// TestExtractGapTitle verifies that extractGapTitle parses the **Gap:** line
// from scout.md correctly.
func TestExtractGapTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "standard scout format",
			input: "## SCOUT GAP REPORT — Iteration 354\n\n**Gap:** The hive cannot scale collective decision-making.\n",
			want: "The hive cannot scale collective decision-making.",
		},
		{
			name:  "no gap line",
			input: "## SCOUT GAP REPORT\n\nSome content without a gap line.",
			want:  "",
		},
		{
			name:  "gap with extra whitespace",
			input: "**Gap:**   Missing quorum logic.   \n",
			want:  "Missing quorum logic.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGapTitle([]byte(tt.input))
			if got != tt.want {
				t.Errorf("extractGapTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExtractIterationFromScout verifies iteration number parsing from scout.md.
func TestExtractIterationFromScout(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard format",
			input: "## SCOUT GAP REPORT — Iteration 354\n",
			want:  "354",
		},
		{
			name:  "no iteration",
			input: "## SCOUT GAP REPORT\n",
			want:  "unknown",
		},
		{
			name:  "iteration in body text",
			input: "Last updated: Iteration 100\n",
			want:  "100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIterationFromScout([]byte(tt.input))
			if got != tt.want {
				t.Errorf("extractIterationFromScout() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAssertScoutGapCreatesClaimNode verifies that assertScoutGap reads scout.md,
// extracts the gap, and POSTs op=assert with a claim title and body containing
// the gap title and iteration number to /app/hive/op.
func TestAssertScoutGapCreatesClaimNode(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"claim-123"}}`))
	}))
	defer srv.Close()

	// Write a temporary scout.md that assertScoutGap will read.
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	scoutContent := "## SCOUT GAP REPORT — Iteration 42\n\n**Gap:** Governance lacks quorum logic.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "scout.md"), []byte(scoutContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertScoutGap("lv_testkey", srv.URL); err != nil {
		t.Fatalf("assertScoutGap() error: %v", err)
	}

	if received["op"] != "assert" {
		t.Errorf("op = %q, want %q", received["op"], "assert")
	}
	if received["kind"] != "claim" {
		t.Errorf("kind = %q, want %q", received["kind"], "claim")
	}
	if received["title"] != "Governance lacks quorum logic." {
		t.Errorf("title = %q, want %q", received["title"], "Governance lacks quorum logic.")
	}
	if !strings.Contains(received["body"], "Iteration 42") {
		t.Errorf("body %q does not contain iteration number", received["body"])
	}
	if !strings.Contains(received["body"], "Governance lacks quorum logic.") {
		t.Errorf("body %q does not contain gap title", received["body"])
	}
}

// TestAssertScoutGapMissingFile verifies that assertScoutGap returns an error
// when scout.md does not exist, without crashing.
func TestAssertScoutGapMissingFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	err := assertScoutGap("lv_testkey", "http://localhost:9999")
	if err == nil {
		t.Fatal("expected error for missing scout.md, got nil")
	}
}

// TestAssertScoutGapNoGapLine verifies that assertScoutGap returns an error
// when scout.md exists but contains no **Gap:** line.
func TestAssertScoutGapNoGapLine(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	// Valid scout.md header but missing the Gap line.
	scoutContent := "## SCOUT GAP REPORT — Iteration 99\n\nNo gap identified this iteration.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "scout.md"), []byte(scoutContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	err := assertScoutGap("lv_testkey", "http://localhost:9999")
	if err == nil {
		t.Fatal("expected error when scout.md has no Gap line, got nil")
	}
	if !strings.Contains(err.Error(), "gap title") {
		t.Errorf("error %q should mention gap title", err.Error())
	}
}

// TestAssertScoutGapAPIError verifies that assertScoutGap returns an error
// when the server responds with HTTP 4xx.
func TestAssertScoutGapAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	scoutContent := "## SCOUT GAP REPORT — Iteration 10\n\n**Gap:** Missing auth check.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "scout.md"), []byte(scoutContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	err := assertScoutGap("bad_key", srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}

// TestSyncClaimsAPIError verifies that syncClaims returns an error and does not
// write a file when the API responds with HTTP 4xx.
func TestSyncClaimsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "claims.md")
	err := syncClaims("bad_key", srv.URL, outPath)
	if err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Error("claims.md should not be written on API error")
	}
}

// TestSyncClaimsClaimWithNoMetadata verifies that syncClaims writes claim body
// without the state/author line when both fields are empty.
func TestSyncClaimsClaimWithNoMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"claims": []map[string]any{
				{
					"title":      "Body-only claim",
					"body":       "This claim has no state or author.",
					"state":      "",
					"author":     "",
					"created_at": "2026-03-01T00:00:00Z",
				},
			},
		})
	}))
	defer srv.Close()

	outPath := filepath.Join(t.TempDir(), "claims.md")
	if err := syncClaims("lv_testkey", srv.URL, outPath); err != nil {
		t.Fatalf("syncClaims() error: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	if !strings.Contains(content, "Body-only claim") {
		t.Error("missing claim title")
	}
	if !strings.Contains(content, "This claim has no state or author.") {
		t.Error("missing claim body")
	}
	if strings.Contains(content, "**State:**") {
		t.Error("state/author line should not appear when both are empty")
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
