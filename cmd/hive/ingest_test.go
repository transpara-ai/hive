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

func TestRunIngest(t *testing.T) {
	// Skip repo creation in tests — no real GitHub calls.
	t.Setenv("HIVE_INGEST_SKIP_REPO", "1")

	tests := []struct {
		name       string
		markdown   string
		wantTitle  string
		priority   string
		serverCode int
		serverBody string
		wantErr    bool
		errContain string
	}{
		{
			name:       "extracts H1 title",
			markdown:   "# My Feature Spec\n\nSome description here.",
			wantTitle:  "[SPEC] My Feature Spec",
			priority:   "high",
			serverCode: 200,
			serverBody: `{"node":{"id":"abc-123"}}`,
		},
		{
			name:       "falls back to filename when no H1",
			markdown:   "No heading here, just text.",
			wantTitle:  "[SPEC] spec",
			priority:   "high",
			serverCode: 200,
			serverBody: `{"node":{"id":"def-456"}}`,
		},
		{
			name:       "uses first H1 only",
			markdown:   "# First Heading\n\n# Second Heading\n",
			wantTitle:  "[SPEC] First Heading",
			priority:   "critical",
			serverCode: 200,
			serverBody: `{"node":{"id":"ghi-789"}}`,
		},
		{
			name:       "server error returns error",
			markdown:   "# Spec\n",
			priority:   "high",
			serverCode: 500,
			serverBody: `internal server error`,
			wantErr:    true,
			errContain: "HTTP 500",
		},
		{
			name:       "empty node ID returns error",
			markdown:   "# Spec\n",
			priority:   "high",
			serverCode: 200,
			serverBody: `{"node":{}}`,
			wantErr:    true,
			errContain: "empty node ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture what the server receives.
			var gotPayload map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &gotPayload)
				w.WriteHeader(tt.serverCode)
				_, _ = w.Write([]byte(tt.serverBody))
			}))
			defer srv.Close()

			// Write spec to a temp file.
			dir := t.TempDir()
			specPath := filepath.Join(dir, "spec.md")
			if err := os.WriteFile(specPath, []byte(tt.markdown), 0644); err != nil {
				t.Fatal(err)
			}

			// Set required env var.
			t.Setenv("LOVYOU_API_KEY", "test-key")

			err := runIngest(specPath, "hive", srv.URL, tt.priority)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify payload sent to server.
			if gotPayload["title"] != tt.wantTitle {
				t.Errorf("title = %q, want %q", gotPayload["title"], tt.wantTitle)
			}
			if gotPayload["op"] != ingestOp {
				t.Errorf("op = %q, want %q", gotPayload["op"], ingestOp)
			}
			if gotPayload["kind"] != ingestKind {
				t.Errorf("kind = %q, want %q", gotPayload["kind"], ingestKind)
			}
			if gotPayload["priority"] != tt.priority {
				t.Errorf("priority = %q, want %q", gotPayload["priority"], tt.priority)
			}
		})
	}
}

func TestRunIngest_MissingAPIKey(t *testing.T) {
	t.Setenv("HIVE_INGEST_SKIP_REPO", "1")

	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(specPath, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LOVYOU_API_KEY", "")

	err := runIngest(specPath, "hive", "http://localhost:9999", "high")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "LOVYOU_API_KEY required") {
		t.Errorf("error %q should mention LOVYOU_API_KEY", err.Error())
	}
}

func TestRunIngest_MissingFile(t *testing.T) {
	t.Setenv("HIVE_INGEST_SKIP_REPO", "1")
	t.Setenv("LOVYOU_API_KEY", "test-key")
	err := runIngest("/nonexistent/spec.md", "hive", "http://localhost:9999", "high")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantLang string
	}{
		{"go default", "Build a REST API with endpoints", "go"},
		{"typescript", "Use TypeScript and package.json for the build", "typescript"},
		{"python", "Install via requirements.txt and run pytest", "python"},
		{"rust", "Add to Cargo.toml and compile with cargo build", "rust"},
		{"node.js", "A Node.js server with express", "typescript"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang, _, _ := detectLanguage(tt.body)
			if lang != tt.wantLang {
				t.Errorf("detectLanguage() lang = %q, want %q", lang, tt.wantLang)
			}
		})
	}
}

func TestRepoInRegistry(t *testing.T) {
	dir := t.TempDir()

	// Missing file returns false, nil.
	missing := filepath.Join(dir, "nope.json")
	found, err := repoInRegistry(missing, "foo")
	if err != nil {
		t.Fatalf("missing file: unexpected error: %v", err)
	}
	if found {
		t.Error("missing file: expected false")
	}

	// Valid file with entries.
	valid := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(valid, []byte(`{"repos":[{"name":"site"},{"name":"hive"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	found, err = repoInRegistry(valid, "site")
	if err != nil {
		t.Fatalf("valid lookup: %v", err)
	}
	if !found {
		t.Error("expected site to be found")
	}
	found, err = repoInRegistry(valid, "nope")
	if err != nil {
		t.Fatalf("valid lookup: %v", err)
	}
	if found {
		t.Error("expected nope to not be found")
	}

	// Malformed file returns error.
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{not json`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = repoInRegistry(bad, "foo")
	if err == nil {
		t.Error("malformed file: expected error")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"[SPEC] MATLAB Integration Endpoint for Transpara Platform", "matlab-integration-endpoint-for-transpara-platform"},
		{"[SPEC] Work Description: Build a REST API for Users", "build-a-rest-api-for-users"},
		{"[SPEC] Simple Title", "simple-title"},
		{"[SPEC] Spec: Auth System for Mobile App", "auth-system-for-mobile-app"},
		{"[SPEC] Already-Kebab-Case", "already-kebab-case"},
		{"[SPEC] Lots   of   spaces", "lots-of-spaces"},
		{"[SPEC] Special!@#$Characters", "special-characters"},
		{"[SPEC] Support for Pagination", "support-for-pagination"},
		{"[SPEC] Support for Rate-Limiting", "support-for-rate-limiting"},
		{"", "spec"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
