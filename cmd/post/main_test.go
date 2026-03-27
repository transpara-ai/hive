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

// TestPostCreatesDocument verifies that the post() function sends op=intend with
// kind=document, title, and description to /app/hive/op.
// Build reports are structured documents, not casual feed posts.
func TestPostCreatesDocument(t *testing.T) {
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

	nodeID, err := post("lv_testkey", srv.URL, "Fix: some bug", "## What Was Built\nFixed the bug.")
	if err != nil {
		t.Fatalf("post() error: %v", err)
	}
	if nodeID != "test-id" {
		t.Errorf("post() nodeID = %q, want %q", nodeID, "test-id")
	}

	if received["op"] != "intend" {
		t.Errorf("op = %q, want %q", received["op"], "intend")
	}
	if received["kind"] != "document" {
		t.Errorf("kind = %q, want %q", received["kind"], "document")
	}
	if received["title"] != "Fix: some bug" {
		t.Errorf("title = %q, want %q", received["title"], "Fix: some bug")
	}
	if received["description"] == "" {
		t.Error("description is empty, want non-empty build summary")
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

	if err := assertScoutGap("lv_testkey", srv.URL, nil); err != nil {
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

	err := assertScoutGap("lv_testkey", "http://localhost:9999", nil)
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

	err := assertScoutGap("lv_testkey", "http://localhost:9999", nil)
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

	err := assertScoutGap("bad_key", srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}

// TestAssertScoutGapSendsAuthHeader verifies that assertScoutGap sets the
// Authorization: Bearer header so the API can authenticate the request.
// If the header is missing, production returns 401 but mock tests pass — this
// test catches that regression.
func TestAssertScoutGapSendsAuthHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"x"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "loop", "scout.md"),
		[]byte("## SCOUT GAP REPORT — Iteration 7\n\n**Gap:** Auth header missing.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertScoutGap("lv_mykey", srv.URL, nil); err != nil {
		t.Fatalf("assertScoutGap() error: %v", err)
	}

	want := "Bearer lv_mykey"
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
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
// document node whose title comes from build.md (not just "Iteration N").
func TestBuildTitleExtractedOnPost(t *testing.T) {
	buildMD := []byte("# Build: Fix: Observer AllowedTools missing knowledge.search\n\n## What Was Built\nFixed it.")

	var receivedTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &payload)
		if payload["op"] == "intend" && payload["kind"] == "document" {
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

	if _, err := post("lv_testkey", srv.URL, title, string(buildMD)); err != nil {
		t.Fatalf("post() error: %v", err)
	}

	want := "Fix: Observer AllowedTools missing knowledge.search"
	if receivedTitle != want {
		t.Errorf("post title = %q, want %q", receivedTitle, want)
	}
}

// TestExtractCritiqueTitle verifies title extraction from critique.md.
func TestExtractCritiqueTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard format",
			input: "# Critique: Fix: causes field missing on subtasks\n\n**Verdict:** PASS\n",
			want:  "Critique: Fix: causes field missing on subtasks",
		},
		{
			name:  "no heading",
			input: "**Verdict:** PASS\n",
			want:  "",
		},
		{
			name:  "multi-hash heading",
			input: "## Critique: Some thing\n",
			want:  "Critique: Some thing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCritiqueTitle([]byte(tt.input))
			if got != tt.want {
				t.Errorf("extractCritiqueTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAssertCritiqueCreatesClaimNode verifies that assertCritique reads
// critique.md and POSTs op=assert kind=claim to /app/hive/op.
func TestAssertCritiqueCreatesClaimNode(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"claim-456"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	critiqueContent := "# Critique: Fix: missing causes field\n\n**Verdict:** PASS\n\nAll tests pass.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "critique.md"), []byte(critiqueContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertCritique("lv_testkey", srv.URL, nil); err != nil {
		t.Fatalf("assertCritique() error: %v", err)
	}

	if received["op"] != "assert" {
		t.Errorf("op = %q, want %q", received["op"], "assert")
	}
	if received["kind"] != "claim" {
		t.Errorf("kind = %q, want %q", received["kind"], "claim")
	}
	if received["title"] != "Critique: Fix: missing causes field" {
		t.Errorf("title = %q, want %q", received["title"], "Critique: Fix: missing causes field")
	}
	if !strings.Contains(received["body"], "PASS") {
		t.Errorf("body %q does not contain verdict", received["body"])
	}
}

// TestAssertCritiqueMissingFile verifies that assertCritique returns an error
// when critique.md does not exist.
func TestAssertCritiqueMissingFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	err := assertCritique("lv_testkey", "http://localhost:9999", nil)
	if err == nil {
		t.Fatal("expected error for missing critique.md, got nil")
	}
}

// TestAssertCritiqueCarriesTaskNodeIDasCause verifies that assertCritique passes
// the task node ID as the causes field, so the critique is causally linked to
// the build task it reviews (Invariant 2: CAUSALITY).
func TestAssertCritiqueCarriesTaskNodeIDasCause(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"critique-999"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	critiqueContent := "# Critique: Fix: causes=[] on all critique nodes\n\n**Verdict:** PASS\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "critique.md"), []byte(critiqueContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	taskNodeID := "task-node-abc123"
	if err := assertCritique("lv_testkey", srv.URL, []string{taskNodeID}); err != nil {
		t.Fatalf("assertCritique() error: %v", err)
	}

	if received["causes"] != taskNodeID {
		t.Errorf("causes = %q, want %q (task node ID must be declared as cause)", received["causes"], taskNodeID)
	}
}

// TestExtractLatestReflection verifies that extractLatestReflection returns
// the first ## section from reflections.md (the most recent entry).
func TestExtractLatestReflection(t *testing.T) {
	input := `# Reflection Log

## 2026-03-27

**COVER:** Something was built.

**BLIND:** Something was missed.

## Iteration 1 — 2026-03-22

**COVER:** Earlier entry.
`
	title, body := extractLatestReflection([]byte(input))
	if title != "2026-03-27" {
		t.Errorf("title = %q, want %q", title, "2026-03-27")
	}
	if !strings.Contains(body, "Something was built") {
		t.Errorf("body %q does not contain expected content", body)
	}
	if strings.Contains(body, "Earlier entry") {
		t.Error("body should not contain content from the second entry")
	}
}

// TestExtractLatestReflectionNoEntry verifies that extractLatestReflection
// returns empty strings when there are no ## sections.
func TestExtractLatestReflectionNoEntry(t *testing.T) {
	input := "# Reflection Log\n\nNo entries yet.\n"
	title, body := extractLatestReflection([]byte(input))
	if title != "" || body != "" {
		t.Errorf("expected empty title and body, got title=%q body=%q", title, body)
	}
}

// TestAssertLatestReflectionCreatesDocument verifies that assertLatestReflection
// reads reflections.md and POSTs op=intend kind=document to /app/hive/op.
func TestAssertLatestReflectionCreatesDocument(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"doc-789"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	reflContent := "# Reflection Log\n\n## 2026-03-27\n\n**COVER:** Build was clean.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "reflections.md"), []byte(reflContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertLatestReflection("lv_testkey", srv.URL, nil); err != nil {
		t.Fatalf("assertLatestReflection() error: %v", err)
	}

	if received["op"] != "intend" {
		t.Errorf("op = %q, want %q", received["op"], "intend")
	}
	if received["kind"] != "document" {
		t.Errorf("kind = %q, want %q", received["kind"], "document")
	}
	if received["title"] != "Reflection: 2026-03-27" {
		t.Errorf("title = %q, want %q", received["title"], "Reflection: 2026-03-27")
	}
	if !strings.Contains(received["description"], "Build was clean") {
		t.Errorf("description %q does not contain reflection content", received["description"])
	}
}

// TestAssertLatestReflectionMissingFile verifies that assertLatestReflection
// returns an error when reflections.md does not exist.
func TestAssertLatestReflectionMissingFile(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	err := assertLatestReflection("lv_testkey", "http://localhost:9999", nil)
	if err == nil {
		t.Fatal("expected error for missing reflections.md, got nil")
	}
}

// TestCreateTaskSendsKindTask verifies that createTask() sends op=intend with
// kind=task. The fix in this iteration was adding explicit kind=task — without
// it all 491 board nodes appeared as kind=task only by coincidence of the
// server default, not because the client requested it. This test pins the fix.
func TestCreateTaskSendsKindTask(t *testing.T) {
	var intendPayload map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/hive/op" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		json.Unmarshal(body, &payload)
		// Capture the intend call (task creation), ignore the complete call.
		if payload["op"] == "intend" {
			intendPayload = payload
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"task-abc"}}`))
	}))
	defer srv.Close()

	_, err := createTask("lv_testkey", srv.URL, "Fix: observer audit", "build details here")
	if err != nil {
		t.Fatalf("createTask() error: %v", err)
	}

	if intendPayload == nil {
		t.Fatal("no intend request received — createTask did not create a task")
	}
	if intendPayload["op"] != "intend" {
		t.Errorf("op = %q, want %q", intendPayload["op"], "intend")
	}
	if intendPayload["kind"] != "task" {
		t.Errorf("kind = %q, want %q — explicit kind=task must be sent so board nodes have the correct kind",
			intendPayload["kind"], "task")
	}
	if intendPayload["title"] != "Fix: observer audit" {
		t.Errorf("title = %q, want %q", intendPayload["title"], "Fix: observer audit")
	}
}

// TestCreateTaskReturnsNodeID verifies that createTask returns the task node ID
// from the server response. This is the critical new behaviour: the caller needs
// this ID to pass as causes to assertCritique so the critique is causally linked
// to the build task (Invariant 2: CAUSALITY).
func TestCreateTaskReturnsNodeID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/hive/op" {
			http.NotFound(w, r)
			return
		}
		// Both intend and complete respond with a node ID.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"task-node-xyz"}}`))
	}))
	defer srv.Close()

	nodeID, err := createTask("lv_testkey", srv.URL, "Fix: causality gap", "details")
	if err != nil {
		t.Fatalf("createTask() error: %v", err)
	}
	if nodeID != "task-node-xyz" {
		t.Errorf("createTask() nodeID = %q, want %q — node ID must be returned so critique can declare it as a cause",
			nodeID, "task-node-xyz")
	}
}

// TestCreateTaskEmptyResponseIDReturnsEmpty verifies that createTask returns
// ("", nil) when the server responds with an empty node ID. This happens when
// the server doesn't return a node in the response body. The caller (main) must
// fall back gracefully — the task was created but the ID is unknown.
func TestCreateTaskEmptyResponseIDReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`)) // no node.id
	}))
	defer srv.Close()

	nodeID, err := createTask("lv_testkey", srv.URL, "Fix: something", "details")
	if err != nil {
		t.Fatalf("createTask() unexpected error: %v", err)
	}
	if nodeID != "" {
		t.Errorf("createTask() nodeID = %q, want empty string when response has no node ID", nodeID)
	}
}

// TestCreateTaskSendsCompleteOp verifies that createTask sends a second request
// to complete the task after creating it. The complete op must carry the node_id
// returned by the intend op — without this the task stays in-progress on the board.
func TestCreateTaskSendsCompleteOp(t *testing.T) {
	var requests []map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/hive/op" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		json.Unmarshal(body, &payload)
		requests = append(requests, payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"task-888"}}`))
	}))
	defer srv.Close()

	if _, err := createTask("lv_testkey", srv.URL, "Fix: task complete", "details"); err != nil {
		t.Fatalf("createTask() error: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (intend + complete), got %d", len(requests))
	}
	complete := requests[1]
	if complete["op"] != "complete" {
		t.Errorf("second request op = %q, want %q", complete["op"], "complete")
	}
	if complete["node_id"] != "task-888" {
		t.Errorf("complete node_id = %q, want %q — must use the ID returned by intend", complete["node_id"], "task-888")
	}
}

// TestEnsureSpaceExisting verifies that ensureSpace returns nil (without creating)
// when the API responds with 200 OK.
func TestEnsureSpaceExisting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/app/hive" {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := ensureSpace("lv_testkey", srv.URL); err != nil {
		t.Fatalf("ensureSpace() error: %v", err)
	}
}

// TestEnsureSpaceCreates verifies that ensureSpace POSTs to /app/new when
// the space does not exist (404 response).
func TestEnsureSpaceCreates(t *testing.T) {
	var createCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/app/hive":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == "POST" && r.URL.Path == "/app/new":
			createCalled = true
			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["kind"] != "community" {
				t.Errorf("create space kind = %q, want %q", payload["kind"], "community")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"space":{"slug":"hive"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	if err := ensureSpace("lv_testkey", srv.URL); err != nil {
		t.Fatalf("ensureSpace() error: %v", err)
	}
	if !createCalled {
		t.Error("expected POST /app/new to create space, but it was not called")
	}
}

// TestEnsureSpaceCreateError verifies that ensureSpace returns an error when
// the create POST fails with HTTP 4xx.
func TestEnsureSpaceCreateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	err := ensureSpace("bad_key", srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 403 on create, got nil")
	}
}

// TestSyncMindStateSuccess verifies that syncMindState sends a PUT request
// with the state content and Authorization header.
func TestSyncMindStateSuccess(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotPayload map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	stateContent := "## Loop State\n\nIteration 42"
	if err := syncMindState("lv_testkey", srv.URL, stateContent); err != nil {
		t.Fatalf("syncMindState() error: %v", err)
	}

	if gotMethod != "PUT" {
		t.Errorf("method = %q, want %q", gotMethod, "PUT")
	}
	if gotPath != "/api/mind-state" {
		t.Errorf("path = %q, want %q", gotPath, "/api/mind-state")
	}
	if gotAuth != "Bearer lv_testkey" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer lv_testkey")
	}
	if gotPayload["key"] != "loop_state" {
		t.Errorf("key = %q, want %q", gotPayload["key"], "loop_state")
	}
	if gotPayload["value"] != stateContent {
		t.Errorf("value = %q, want %q", gotPayload["value"], stateContent)
	}
}

// TestSyncMindStateError verifies that syncMindState returns an error when
// the server responds with HTTP 4xx.
func TestSyncMindStateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	err := syncMindState("bad_key", srv.URL, "state content")
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
}

// TestAssertCritiqueSendsCauses verifies that assertCritique includes the
// causes field in the JSON payload when causeIDs are provided.
// This ensures claim nodes are causally linked to the build that generated them
// (Invariant 2: CAUSALITY).
func TestAssertCritiqueSendsCauses(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"claim-456"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	critiqueContent := "# Critique: Fix: causes field missing\n\n**Verdict:** PASS\n\nAll tests pass.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "critique.md"), []byte(critiqueContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertCritique("lv_testkey", srv.URL, []string{"build-doc-id-123"}); err != nil {
		t.Fatalf("assertCritique() error: %v", err)
	}

	if received["causes"] != "build-doc-id-123" {
		t.Errorf("causes = %q, want %q", received["causes"], "build-doc-id-123")
	}
}

// TestAssertScoutGapSendsCauses verifies that assertScoutGap includes the
// causes field in the JSON payload when causeIDs are provided.
func TestAssertScoutGapSendsCauses(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"claim-789"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	scoutContent := "## SCOUT GAP REPORT — Iteration 99\n\n**Gap:** Causes field missing on claims.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "scout.md"), []byte(scoutContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertScoutGap("lv_testkey", srv.URL, []string{"build-doc-id-456"}); err != nil {
		t.Fatalf("assertScoutGap() error: %v", err)
	}

	if received["causes"] != "build-doc-id-456" {
		t.Errorf("causes = %q, want %q", received["causes"], "build-doc-id-456")
	}
}

// TestPostReturnsBuildDocID verifies that post() returns the node ID from
// the server response, so it can be used as a cause for subsequent claims.
func TestPostReturnsBuildDocID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"node":{"id":"build-doc-abc"},"op":"intend"}`))
	}))
	defer srv.Close()

	nodeID, err := post("lv_testkey", srv.URL, "Fix: causality gap", "build details")
	if err != nil {
		t.Fatalf("post() error: %v", err)
	}
	if nodeID != "build-doc-abc" {
		t.Errorf("post() nodeID = %q, want %q", nodeID, "build-doc-abc")
	}
}

// TestAssertLatestReflectionSendsCauses verifies that assertLatestReflection
// includes the causes field in the JSON payload when causeIDs are provided.
// The build updated all three assert functions to carry causes; this pins the
// reflection path (the other two have their own SendsCauses tests).
func TestAssertLatestReflectionSendsCauses(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"doc-789"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	reflContent := "# Reflection Log\n\n## 2026-03-28\n\n**COVER:** Causes wired up.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "reflections.md"), []byte(reflContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	if err := assertLatestReflection("lv_testkey", srv.URL, []string{"build-doc-id-999"}); err != nil {
		t.Fatalf("assertLatestReflection() error: %v", err)
	}

	if received["causes"] != "build-doc-id-999" {
		t.Errorf("causes = %q, want %q", received["causes"], "build-doc-id-999")
	}
}

// TestAssertCauseIDsMultipleJoined verifies that when multiple causeIDs are
// provided they are comma-joined in the payload (the server expects a CSV
// string, not a JSON array).
func TestAssertCauseIDsMultipleJoined(t *testing.T) {
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
		w.Write([]byte(`{"node":{"id":"claim-multi"}}`))
	}))
	defer srv.Close()

	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	critiqueContent := "# Critique: Multi-cause test\n\n**Verdict:** PASS\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "critique.md"), []byte(critiqueContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	causeIDs := []string{"id-aaa", "id-bbb", "id-ccc"}
	if err := assertCritique("lv_testkey", srv.URL, causeIDs); err != nil {
		t.Fatalf("assertCritique() error: %v", err)
	}

	want := "id-aaa,id-bbb,id-ccc"
	if received["causes"] != want {
		t.Errorf("causes = %q, want %q", received["causes"], want)
	}
}

// TestPostEmptyResponseReturnsEmptyID verifies that post() returns ("", nil)
// when the server responds with 200 but no node in the JSON body. This happens
// when the server is an older version that doesn't return node IDs. main()
// guards against this (skips causeIDs when buildDocID == ""), so the path
// must not error.
func TestPostEmptyResponseReturnsEmptyID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{}`)) // no node.id
	}))
	defer srv.Close()

	nodeID, err := post("lv_testkey", srv.URL, "Fix: something", "body")
	if err != nil {
		t.Fatalf("post() unexpected error: %v", err)
	}
	if nodeID != "" {
		t.Errorf("post() nodeID = %q, want empty string when response has no node", nodeID)
	}
}

// TestAssertCritiqueNoTitle verifies that assertCritique returns an error
// when critique.md exists but contains no heading (no title to extract).
func TestAssertCritiqueNoTitle(t *testing.T) {
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "loop"), 0755); err != nil {
		t.Fatal(err)
	}
	// critique.md with no heading — only body content.
	critiqueContent := "**Verdict:** PASS\n\nAll tests pass. No issues found.\n"
	if err := os.WriteFile(filepath.Join(tmp, "loop", "critique.md"), []byte(critiqueContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	err := assertCritique("lv_testkey", "http://localhost:9999", nil)
	if err == nil {
		t.Fatal("expected error when critique.md has no heading, got nil")
	}
	if !strings.Contains(err.Error(), "critique title") {
		t.Errorf("error %q should mention critique title", err.Error())
	}
}

// TestSyncClaimsMultipleCauses verifies that when a claim has multiple cause IDs,
// all of them are comma-joined in the **Causes:** line in claims.md.
// This exercises the strings.Join path added in this iteration.
func TestSyncClaimsMultipleCauses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"claims": []map[string]any{
				{
					"title":  "Multi-cause claim",
					"body":   "This claim has two causes.",
					"causes": []string{"build-doc-aaa", "build-doc-bbb"},
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

	if !strings.Contains(content, "build-doc-aaa") {
		t.Error("claims.md missing first cause ID")
	}
	if !strings.Contains(content, "build-doc-bbb") {
		t.Error("claims.md missing second cause ID")
	}
	// Both must appear on the same Causes line.
	if !strings.Contains(content, "build-doc-aaa, build-doc-bbb") {
		t.Errorf("expected causes joined as %q, not found in:\n%s", "build-doc-aaa, build-doc-bbb", content)
	}
}

// TestSyncClaimsWritesCauses verifies that syncClaims includes the causes field
// in claims.md when the API returns claims with causes populated.
func TestSyncClaimsWritesCauses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"claims": []map[string]any{
				{
					"title":      "Ship what you build",
					"body":       "Every build iteration should deploy.",
					"state":      "verified",
					"author":     "Builder",
					"created_at": "2026-03-01T00:00:00Z",
					"causes":     []string{"build-doc-abc123"},
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

	if !strings.Contains(content, "build-doc-abc123") {
		t.Error("claims.md missing causes — agents cannot trace claim provenance without them")
	}
	if !strings.Contains(content, "**Causes:**") {
		t.Error("claims.md missing **Causes:** label")
	}
}
