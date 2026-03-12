package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
)

// PipelineResult collects telemetry data during a pipeline run.
// Written to .hive/telemetry/ after each run — the foundation for
// --self-improve mode where the CTO reads past run data.
type PipelineResult struct {
	// Timing per phase.
	PhaseTimings []PhaseTimingEntry `json:"phase_timings"`

	// Token usage and cost per role.
	TokenUsage []RoleTokenUsage `json:"token_usage"`

	// Guardian alert lines (the ALERT/VIOLATION/QUARANTINE lines).
	GuardianAlerts []string `json:"guardian_alerts"`

	// Reviewer feedback signals: "APPROVED" or "CHANGES NEEDED".
	ReviewSignals []string `json:"review_signals"`

	// The input idea/description that started the run.
	InputDescription string `json:"input_description"`

	// PR URL if one was created (targeted mode only).
	PRURL string `json:"pr_url,omitempty"`

	// Whether the PR was merged.
	Merged bool `json:"merged"`

	// FailedPhase is the pipeline phase that halted with an error (empty on success).
	FailedPhase string `json:"failed_phase,omitempty"`

	// FailureReason is the error message from the failed phase (empty on success).
	FailureReason string `json:"failure_reason,omitempty"`

	// Pipeline mode: "full" or "targeted".
	Mode string `json:"mode"`

	// When the pipeline started and ended.
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// PhaseTimingEntry records timing for one pipeline phase.
type PhaseTimingEntry struct {
	Phase    string        `json:"phase"`
	Duration time.Duration `json:"duration_ms"`
}

// MarshalJSON for PhaseTimingEntry — encode duration as milliseconds.
func (e PhaseTimingEntry) MarshalJSON() ([]byte, error) {
	type alias struct {
		Phase      string `json:"phase"`
		DurationMs int64  `json:"duration_ms"`
	}
	return json.Marshal(alias{
		Phase:      e.Phase,
		DurationMs: e.Duration.Milliseconds(),
	})
}

// RoleTokenUsage records token usage for one role.
type RoleTokenUsage struct {
	Role             string  `json:"role"`
	Model            string  `json:"model"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// totalCost sums CostUSD across all role token entries.
func (r *PipelineResult) totalCost() float64 {
	var total float64
	for _, u := range r.TokenUsage {
		total += u.CostUSD
	}
	return total
}

// addPhaseTiming records a phase's duration.
func (r *PipelineResult) addPhaseTiming(phase string, d time.Duration) {
	r.PhaseTimings = append(r.PhaseTimings, PhaseTimingEntry{Phase: phase, Duration: d})
}

// addGuardianAlert appends an alert string if non-empty.
func (r *PipelineResult) addGuardianAlert(alert string) {
	if alert != "" {
		r.GuardianAlerts = append(r.GuardianAlerts, alert)
	}
}

// addReviewSignal records whether a review round approved or requested changes.
func (r *PipelineResult) addReviewSignal(approved bool) {
	if approved {
		r.ReviewSignals = append(r.ReviewSignals, "APPROVED")
	} else {
		r.ReviewSignals = append(r.ReviewSignals, "CHANGES NEEDED")
	}
}

// recordFailure records the phase and error message that halted the pipeline.
func (r *PipelineResult) recordFailure(phase string, err error) {
	r.FailedPhase = phase
	r.FailureReason = err.Error()
}

// failPhase records a phase failure in telemetry and returns the error unchanged.
// Safe to call when p.telemetry is nil (no-op).
func (p *Pipeline) failPhase(phase string, err error) error {
	if p.telemetry != nil {
		p.telemetry.recordFailure(phase, err)
	}
	return err
}

// collectTokenUsage snapshots token usage from all tracked roles.
// Guardian is excluded here — its usage is accumulated incrementally via
// accumulateGuardianUsage (called after each guardianCheck invocation)
// because each check creates a fresh provider and would otherwise overwrite
// previous calls, making only the last call visible in telemetry.
func (r *PipelineResult) collectTokenUsage(trackers map[roles.Role]*resources.TrackingProvider) {
	for role, tracker := range trackers {
		if role == roles.RoleGuardian {
			continue // accumulated incrementally via accumulateGuardianUsage
		}
		s := tracker.Snapshot()
		r.TokenUsage = append(r.TokenUsage, RoleTokenUsage{
			Role:             string(role),
			Model:            tracker.Model(),
			InputTokens:      s.InputTokens,
			OutputTokens:     s.OutputTokens,
			TotalTokens:      s.TokensUsed,
			CacheReadTokens:  s.CacheReadTokens,
			CacheWriteTokens: s.CacheWriteTokens,
			CostUSD:          s.CostUSD,
		})
	}
}

// accumulateGuardianUsage adds a Guardian check's token usage to the running
// total in TokenUsage. Called after every guardianCheck invocation so that
// all phase checks (not just the last one) appear in telemetry.
// Safe to call when r is nil (no-op).
func (r *PipelineResult) accumulateGuardianUsage(tracker *resources.TrackingProvider) {
	if r == nil {
		return
	}
	s := tracker.Snapshot()
	if s.TokensUsed == 0 {
		return // nothing to record
	}
	// Find an existing Guardian entry and add to it.
	for i, entry := range r.TokenUsage {
		if entry.Role == string(roles.RoleGuardian) {
			r.TokenUsage[i].InputTokens += s.InputTokens
			r.TokenUsage[i].OutputTokens += s.OutputTokens
			r.TokenUsage[i].TotalTokens += s.TokensUsed
			r.TokenUsage[i].CacheReadTokens += s.CacheReadTokens
			r.TokenUsage[i].CacheWriteTokens += s.CacheWriteTokens
			r.TokenUsage[i].CostUSD += s.CostUSD
			return
		}
	}
	// First Guardian call — create the entry.
	r.TokenUsage = append(r.TokenUsage, RoleTokenUsage{
		Role:             string(roles.RoleGuardian),
		Model:            tracker.Model(),
		InputTokens:      s.InputTokens,
		OutputTokens:     s.OutputTokens,
		TotalTokens:      s.TokensUsed,
		CacheReadTokens:  s.CacheReadTokens,
		CacheWriteTokens: s.CacheWriteTokens,
		CostUSD:          s.CostUSD,
	})
}

// writeTelemetry writes the pipeline result to .hive/telemetry/<timestamp>.json
// inside the given base directory. Errors are logged but not fatal — telemetry
// must never break the pipeline.
func writeTelemetry(baseDir string, result *PipelineResult) {
	result.EndedAt = time.Now()

	dir := filepath.Join(baseDir, ".hive", "telemetry")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: telemetry dir: %v\n", err)
		return
	}

	// Filename: timestamp with colons replaced for filesystem safety.
	ts := result.StartedAt.UTC().Format("2006-01-02T15-04-05")
	path := filepath.Join(dir, ts+".json")

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: telemetry marshal: %v\n", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: telemetry write: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Telemetry written: %s\n", path)
}

// UnmarshalJSON for PhaseTimingEntry — decode duration from milliseconds.
func (e *PhaseTimingEntry) UnmarshalJSON(data []byte) error {
	type alias struct {
		Phase      string `json:"phase"`
		DurationMs int64  `json:"duration_ms"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	e.Phase = a.Phase
	e.Duration = time.Duration(a.DurationMs) * time.Millisecond
	return nil
}

// ReadTelemetry reads all PipelineResult JSON files from .hive/telemetry/ under baseDir.
// Returns an empty slice (not an error) if the directory doesn't exist or has no files.
func ReadTelemetry(baseDir string) ([]PipelineResult, error) {
	dir := filepath.Join(baseDir, ".hive", "telemetry")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read telemetry dir: %w", err)
	}

	var results []PipelineResult
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: read telemetry file %s: %v\n", entry.Name(), err)
			continue
		}
		var result PipelineResult
		if err := json.Unmarshal(data, &result); err != nil {
			fmt.Fprintf(os.Stderr, "warning: parse telemetry file %s: %v\n", entry.Name(), err)
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// telemetryBaseDir returns the directory where .hive/telemetry/ should be created.
// Targeted mode: product dir. Greenfield: workspace root.
func (p *Pipeline) telemetryBaseDir() string {
	if p.product != nil {
		return p.product.Dir
	}
	if p.ws != nil {
		return p.ws.Root()
	}
	return "."
}
