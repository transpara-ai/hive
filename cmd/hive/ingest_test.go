package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRunIngest(t *testing.T) {
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
				if tt.errContain != "" && !containsStr(err.Error(), tt.errContain) {
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
			if gotPayload["op"] != "intend" {
				t.Errorf("op = %q, want %q", gotPayload["op"], "intend")
			}
			if gotPayload["kind"] != "task" {
				t.Errorf("kind = %q, want %q", gotPayload["kind"], "task")
			}
			if gotPayload["priority"] != tt.priority {
				t.Errorf("priority = %q, want %q", gotPayload["priority"], tt.priority)
			}
		})
	}
}

func TestRunIngest_MissingAPIKey(t *testing.T) {
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
	if !containsStr(err.Error(), "LOVYOU_API_KEY required") {
		t.Errorf("error %q should mention LOVYOU_API_KEY", err.Error())
	}
}

func TestRunIngest_MissingFile(t *testing.T) {
	t.Setenv("LOVYOU_API_KEY", "test-key")
	err := runIngest("/nonexistent/spec.md", "hive", "http://localhost:9999", "high")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
