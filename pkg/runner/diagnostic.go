package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// PhaseEvent is a diagnostic event emitted by every pipeline phase.
// The hive's nervous system — everything an agent needs to detect
// inefficiency, stuck tasks, wrong-repo builds, and scope creep.
type PhaseEvent struct {
	CycleID       string  `json:"cycle_id,omitempty"` // stable id for one pipeline cycle
	Phase         string  `json:"phase"`
	WorkflowStage string  `json:"workflow_stage,omitempty"` // intake, discovery, design, emission, validation, review, reporting, audit
	Outcome       string  `json:"outcome,omitempty"`        // "success", "failure", "revise", "skip"
	Error         string  `json:"error,omitempty"`
	Summary       string  `json:"summary,omitempty"` // human-readable one-line status
	Preview       string  `json:"preview,omitempty"`
	Model         string  `json:"model,omitempty"`         // which model was used
	TaskID        string  `json:"task_id,omitempty"`       // which task was worked
	TaskTitle     string  `json:"task_title,omitempty"`    // task title for pattern detection
	Repo          string  `json:"repo,omitempty"`          // which repo was targeted
	InputRef      string  `json:"input_ref,omitempty"`     // stable reference to the phase input
	OutputRef     string  `json:"output_ref,omitempty"`    // stable reference to the phase output
	GitHash       string  `json:"git_hash,omitempty"`      // commit produced
	FilesChanged  int     `json:"files_changed,omitempty"` // scope indicator
	ReviseCount   int     `json:"revise_count,omitempty"`  // REVISE loops this cycle
	BoardOpen     int     `json:"board_open,omitempty"`    // open tasks at this point
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	DurationSecs  float64 `json:"duration_secs,omitempty"` // wall clock time
	CostUSD       float64 `json:"cost_usd,omitempty"`      // derived, kept for convenience
	Timestamp     string  `json:"timestamp"`
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

// countDiagnostics counts newline-terminated lines in
// {hiveDir}/loop/diagnostics.jsonl. Returns 0 if the file doesn't exist.
func countDiagnostics(hiveDir string) int {
	path := filepath.Join(hiveDir, "loop", "diagnostics.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count
}

// appendDiagnostic appends a PhaseEvent to loop/diagnostics.jsonl and, if an
// API client is configured, POSTs it to the site's /api/hive/diagnostic endpoint
// so the /hive/feed dashboard shows real data in production.
func (r *Runner) appendDiagnostic(e PhaseEvent) {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("[runner] appendDiagnostic marshal: %v", err)
		return
	}

	if r.cfg.HiveDir != "" {
		path := filepath.Join(r.cfg.HiveDir, "loop", "diagnostics.jsonl")
		f, ferr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if ferr != nil {
			log.Printf("[runner] appendDiagnostic open: %v", ferr)
		} else {
			fmt.Fprintf(f, "%s\n", data)
			f.Close()
		}
	}

	if r.cfg.APIClient != nil {
		if err := r.cfg.APIClient.PostDiagnostic(data); err != nil {
			log.Printf("[runner] appendDiagnostic post: %v", err)
		}
	}
}
