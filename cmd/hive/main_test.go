package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"testing"
)

type logCaptureMain struct {
	original io.Writer
	buf      *bytes.Buffer
}

func captureLogMain(t *testing.T) *logCaptureMain {
	t.Helper()
	buf := &bytes.Buffer{}
	orig := log.Writer()
	log.SetOutput(buf)
	return &logCaptureMain{original: orig, buf: buf}
}

func (l *logCaptureMain) String() string { return l.buf.String() }
func (l *logCaptureMain) restore()       { log.SetOutput(l.original) }

func writeMinimalCatalog(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/catalog.yaml"
	body := `models:
  - id: claude-sonnet-4-6
    aliases: [sonnet]
    provider: claude-cli
    auth_mode: subscription
    tier: execution
    capabilities: [tools, reasoning]
    context_window: 200000
    max_output_tokens: 8192
    pricing:
      input_per_million: 3.00
      output_per_million: 15.00

tier_defaults:
  judgment: sonnet
  execution: sonnet
  volume: sonnet
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildCouncilResolver_CatalogPrecedence(t *testing.T) {
	tests := []struct {
		name           string
		catalogPath    string
		envValue       string
		wantErr        bool
		logMustContain string
	}{
		{name: "no catalog, no env: default resolver", catalogPath: "", envValue: "", wantErr: false, logMustContain: ""},
		{name: "env set, no catalog: legacy mode logged", catalogPath: "", envValue: "sonnet", wantErr: false, logMustContain: "legacy mode: COUNCIL_MODEL=sonnet"},
		{name: "unknown COUNCIL_MODEL: error", catalogPath: "", envValue: "not-a-real-model", wantErr: true, logMustContain: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("COUNCIL_MODEL", tt.envValue)
			} else {
				os.Unsetenv("COUNCIL_MODEL")
			}
			logBuf := captureLogMain(t)
			defer logBuf.restore()

			_, err := buildCouncilResolver(tt.catalogPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildCouncilResolver: err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.logMustContain != "" && !strings.Contains(logBuf.String(), tt.logMustContain) {
				t.Errorf("log missing %q; got: %s", tt.logMustContain, logBuf.String())
			}
		})
	}

	// Separate subtest for catalog + env coexistence; uses a real temp catalog file.
	t.Run("catalog and env: env ignored, note logged", func(t *testing.T) {
		catalogPath := writeMinimalCatalog(t)
		t.Setenv("COUNCIL_MODEL", "sonnet")
		logBuf := captureLogMain(t)
		defer logBuf.restore()

		if _, err := buildCouncilResolver(catalogPath); err != nil {
			t.Fatalf("buildCouncilResolver: %v", err)
		}
		if !strings.Contains(logBuf.String(), "COUNCIL_MODEL=sonnet env var ignored") {
			t.Errorf("log missing the env-ignored note; got: %s", logBuf.String())
		}
	})
}
