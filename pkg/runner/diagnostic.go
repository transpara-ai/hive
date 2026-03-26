package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// PhaseEvent is a diagnostic event emitted by a runner phase on error or failure.
type PhaseEvent struct {
	Phase        string  `json:"phase"`
	Outcome      string  `json:"outcome,omitempty"`
	Error        string  `json:"error,omitempty"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	Timestamp    string  `json:"timestamp"`
}

// appendDiagnostic appends a PhaseEvent as a JSON line to
// {hiveDir}/loop/diagnostics.jsonl.  It sets Timestamp if unset.
func appendDiagnostic(hiveDir string, e PhaseEvent) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal diagnostic: %w", err)
	}
	path := filepath.Join(hiveDir, "loop", "diagnostics.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open diagnostics.jsonl: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// appendDiagnostic appends a PhaseEvent to loop/diagnostics.jsonl.
// Silently skips if HiveDir is empty.
func (r *Runner) appendDiagnostic(e PhaseEvent) {
	if r.cfg.HiveDir == "" {
		return
	}
	if err := appendDiagnostic(r.cfg.HiveDir, e); err != nil {
		log.Printf("[runner] appendDiagnostic: %v", err)
	}
}
