package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/workspace"
)

// maxSelfImproveIterations is the maximum number of improvements per session.
const maxSelfImproveIterations = 3

// selfImproveIterationTimeout caps how long a single self-improve iteration
// (CTO analysis + targeted pipeline run) can take before being killed.
const selfImproveIterationTimeout = 15 * time.Minute

// telemetryDetailRunLimit is the number of most-recent runs that get full detail
// in summarizeTelemetry(). Older runs get a one-line summary to cap CTO input tokens.
const telemetryDetailRunLimit = 3

// maxTelemetryRuns caps the total number of run summaries sent to the CTO.
// Older runs beyond this limit are dropped entirely — the one-liner format
// already strips most signal from older runs, so dropping them has negligible
// impact on recommendation quality while cutting CTO input tokens significantly.
const maxTelemetryRuns = 20

// SelfImproveRecommendation is the CTO's structured response from telemetry analysis.
type SelfImproveRecommendation struct {
	Description  string   `json:"description"`
	FilesToChange []string `json:"files_to_change"`
	ExpectedImpact string `json:"expected_impact"`
	Priority     string   `json:"priority"`
	SkipReason   string   `json:"skip_reason"`
}

// RunSelfImprove enters self-improvement mode: reads telemetry + codebase,
// has the CTO identify improvements, then runs targeted pipeline iterations.
// Loops up to maxSelfImproveIterations times, stopping when the CTO says
// nothing is worth fixing.
func (p *Pipeline) RunSelfImprove(ctx context.Context, input ProductInput) error {
	if input.RepoPath == "" {
		return fmt.Errorf("RunSelfImprove requires RepoPath")
	}

	for iteration := 1; iteration <= maxSelfImproveIterations; iteration++ {
		fmt.Printf("\n═══ Self-Improve: Iteration %d/%d ═══\n", iteration, maxSelfImproveIterations)

		if err := p.runSelfImproveIteration(ctx, iteration, input); err != nil {
			if err == errSelfImproveStop {
				break
			}
			return fmt.Errorf("self-improve iteration %d: %w", iteration, err)
		}

		fmt.Printf("═══ Self-Improve: Iteration %d complete ═══\n", iteration)
	}

	fmt.Println("\n═══ Self-Improve: Session Complete ═══")
	return nil
}

// errSelfImproveStop is returned by runSelfImproveIteration when the CTO
// says nothing is worth fixing. The caller should stop iterating.
var errSelfImproveStop = fmt.Errorf("CTO says nothing worth fixing")

// runSelfImproveIteration runs a single self-improve iteration with a timeout.
// Returns errSelfImproveStop if the CTO says nothing is worth fixing.
func (p *Pipeline) runSelfImproveIteration(parentCtx context.Context, iteration int, input ProductInput) error {
	ctx, cancel := context.WithTimeout(parentCtx, selfImproveIterationTimeout)
	defer cancel()

	// Step 0: Clean up from any previous failed iteration — discard uncommitted
	// changes, delete stale hive/* branches, and sync main with remote.
	product, err := workspace.OpenRepo(input.RepoPath)
	if err != nil {
		return fmt.Errorf("open repo for cleanup: %w", err)
	}
	if err := product.CleanupForIteration(); err != nil {
		fmt.Printf("Warning: cleanup failed: %v (continuing anyway)\n", err)
	}

	// Step 1: Read telemetry
	telemetryResults, err := ReadTelemetry(input.RepoPath)
	if err != nil {
		return fmt.Errorf("read telemetry: %w", err)
	}
	fmt.Printf("Telemetry: %d past run(s) found.\n", len(telemetryResults))

	// Step 2: Load codebase context (reuse targeted mode Phase 1).
	// Filter to pipeline-scoped files only — all 41 self-improve iterations have
	// targeted pkg/pipeline/ exclusively; sending the full codebase wastes ~65%
	// of CTO input tokens on packages that have never been touched.
	existingFiles, err := product.ReadSourceFiles()
	if err != nil {
		return fmt.Errorf("read source files: %w", err)
	}
	pipelineFiles := filterSelfImproveFiles(existingFiles)
	fileListing := buildFileListing(pipelineFiles)
	keyContext := extractKeyFiles(pipelineFiles)

	// Step 3: Build telemetry summary for CTO
	telemetrySummary := summarizeTelemetry(telemetryResults)

	// Step 4: CTO analysis — fresh provider per iteration to avoid accumulating
	// prior conversation as input context (each prompt already contains full
	// telemetry + codebase, so prior messages are pure waste).
	fmt.Println("CTO analyzing telemetry + codebase...")
	model := p.selfImproveCTOModel()
	rawProvider, err := p.providerForRoleWithModel(roles.RoleCTO, model)
	if err != nil {
		return fmt.Errorf("CTO provider: %w", err)
	}
	ctoTracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleCTO] = ctoTracker
	fmt.Printf("  ↳ self-improve CTO analysis using %s\n", model)

	ctoPrompt := fmt.Sprintf(`You are analyzing this codebase and its pipeline telemetry to identify the single highest-impact improvement.

TELEMETRY DATA (from past pipeline runs):
%s

PROJECT STRUCTURE:
%s

%s

Look for:
- Recurring Guardian alerts (same alert across multiple runs = wasted spend)
- Slow phases (which phases take disproportionate time?)
- Code quality issues visible in the codebase itself
- Missing error handling or robustness gaps
- Opportunities to reduce complexity or remove dead code

IMPORTANT constraints:
- Token variance between runs is NORMAL — larger changes use more tokens. Do NOT recommend "fixing context bloat" or "reducing builder tokens" unless you see a clear code bug causing it.
- Do NOT recommend changes that are already implemented. Check the codebase before recommending.
- If skip_reason is empty, your recommendation MUST describe a concrete code change, not a diagnosis task.

Respond with ONLY a JSON object: {"description": "what to change, 1-2 sentences", "files_to_change": ["path/to/file"], "expected_impact": "1 sentence", "priority": "high|medium|low", "skip_reason": "if nothing is worth fixing, explain why here; otherwise empty string"}. No preamble, no explanation, no code blocks, no markdown.`, telemetrySummary, fileListing, keyContext)

	ctoResp, err := ctoTracker.Reason(ctx, ctoPrompt, nil)
	if err != nil {
		return fmt.Errorf("CTO self-improve analysis: %w", err)
	}
	ctoResponse := ctoResp.Content()

	// Step 5: Parse recommendation
	rec, err := parseSelfImproveRecommendation(ctoResponse)
	if err != nil {
		return fmt.Errorf("parse CTO recommendation: %w", err)
	}

	fmt.Printf("CTO recommendation (priority=%s): %s\n", rec.Priority, rec.Description)
	if rec.SkipReason != "" {
		fmt.Printf("CTO says nothing worth fixing: %s\n", rec.SkipReason)
		return errSelfImproveStop
	}
	if rec.Description == "" {
		fmt.Println("CTO returned empty recommendation — stopping.")
		return errSelfImproveStop
	}

	fmt.Printf("Expected impact: %s\n", rec.ExpectedImpact)
	fmt.Printf("Files to change: %v\n", rec.FilesToChange)

	// Step 6: Run targeted pipeline with the recommendation.
	// Pre-populate p.telemetry with CTO analysis cost so RunTargeted includes it —
	// the targeted pipeline resets p.trackers on entry, losing ctoTracker.
	// Pass ContextFiles so the Builder only sees pipeline-scoped files — the
	// same filterSelfImproveFiles() set used for CTO analysis. Supplying them
	// directly avoids a second ReadSourceFiles() call in RunTargeted and prevents
	// context bloat from unrelated packages reaching the Builder.
	targetedInput := ProductInput{
		RepoPath:    input.RepoPath,
		Description: rec.Description,
		// Use FILES_TO_CHANGE: structured section so parseRelevantFiles uses the
		// reliable structured extraction path rather than the word-scan fallback.
		CTOAnalysis:  fmt.Sprintf("Description: %s\nFILES_TO_CHANGE:\n%s\nExpected impact: %s", rec.Description, strings.Join(rec.FilesToChange, "\n"), rec.ExpectedImpact),
		ContextFiles: pipelineFiles,
	}
	if s := ctoTracker.Snapshot(); s.Iterations > 0 {
		p.telemetry = &PipelineResult{}
		p.telemetry.TokenUsage = append(p.telemetry.TokenUsage, RoleTokenUsage{
			Role:             "cto_analysis",
			Model:            ctoTracker.Model(),
			InputTokens:      s.InputTokens,
			OutputTokens:     s.OutputTokens,
			TotalTokens:      s.TokensUsed,
			CacheReadTokens:  s.CacheReadTokens,
			CacheWriteTokens: s.CacheWriteTokens,
			CostUSD:          s.CostUSD,
		})
	}
	// Skip Guardian checks for self-improve iterations — Guardian generates zero
	// alerts across all self-improve runs (Builder + test validation is
	// sufficient for internal pipeline code), burning 22% of iteration cost.
	prevSkipGuardian := p.skipGuardian
	p.skipGuardian = true
	defer func() { p.skipGuardian = prevSkipGuardian }()

	// Skip reviewer for self-improve iterations — reviewer consistently signals
	// APPROVED with zero friction detected, consuming 22 seconds and $0.0392
	// per iteration while the code is tested before merge.
	prevSkipReviewer := p.skipReviewer
	p.skipReviewer = true
	defer func() { p.skipReviewer = prevSkipReviewer }()

	fmt.Printf("\n═══ Self-Improve: Running targeted pipeline ═══\n")
	if err := p.RunTargeted(ctx, targetedInput); err != nil {
		return fmt.Errorf("targeted pipeline: %w", err)
	}

	// Sync local main with remote after merge so the next iteration
	// branches from the up-to-date main, not the stale pre-merge state.
	if err := product.SyncMain(); err != nil {
		return fmt.Errorf("sync main: %w", err)
	}

	return nil
}

// selfImproveCTOModel returns the model to use for self-improve CTO analysis.
// Defaults to Haiku — the task is structured JSON output from telemetry data
// (identify one improvement, list files, output JSON), not deep architectural reasoning.
// Use --cto-model claude-sonnet-4-6 to escalate if recommendation quality drops.
func (p *Pipeline) selfImproveCTOModel() string {
	if p.ctoModel != "" {
		return p.ctoModel
	}
	return "claude-haiku-4-5-20251001"
}

// filterSelfImproveFiles returns a filtered copy of files containing only
// pipeline-scoped paths: pkg/pipeline/*.go, cmd/hive/main.go, and CLAUDE.md.
// Cuts CTO analysis input tokens by ~65% — all self-improve iterations have
// targeted pkg/pipeline/ exclusively; mcp, spawn, loop, authority, workspace,
// roles, resources, and docs are noise that drives cost with no signal value.
// Each matched file is truncated to the first maxSelfImproveFileLines lines to
// prevent input token drift as pipeline files grow across iterations.
const maxSelfImproveFileLines = 150

func filterSelfImproveFiles(files map[string]string) map[string]string {
	out := make(map[string]string, len(files))
	for p, content := range files {
		switch {
		case strings.HasPrefix(p, "pkg/pipeline/") && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go"):
			out[p] = truncateLines(content, maxSelfImproveFileLines)
		case p == "cmd/hive/main.go", p == "CLAUDE.md":
			out[p] = truncateLines(content, maxSelfImproveFileLines)
		}
	}
	return out
}

// truncateLines returns the first n lines of s. If s has more than n lines,
// a truncation notice is appended. Mirrors the pattern in extractKeyFiles().
func truncateLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") +
		fmt.Sprintf("\n... [truncated: %d lines omitted]", len(lines)-n)
}

// summarizeTelemetry builds a human-readable summary of past pipeline runs for the CTO.
// To cap CTO input tokens, only the last telemetryDetailRunLimit runs get full detail
// (phase timings, per-role tokens, guardian alert text, review signals). Older runs
// get a one-line summary with cost, alert count, and merge status.
func summarizeTelemetry(results []PipelineResult) string {
	if len(results) == 0 {
		return "No telemetry data available (first run)."
	}

	// Cap to the most recent maxTelemetryRuns runs — older entries add tokens
	// with negligible signal value given the one-liner format already strips detail.
	if len(results) > maxTelemetryRuns {
		results = results[len(results)-maxTelemetryRuns:]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d past pipeline run(s):\n\n", len(results)))

	// Index where full detail begins.
	detailStart := len(results) - telemetryDetailRunLimit
	if detailStart < 0 {
		detailStart = 0
	}

	for i, r := range results {
		if i < detailStart {
			// One-line summary for older runs.
			var totalCost float64
			for _, tu := range r.TokenUsage {
				totalCost += tu.CostUSD
			}
			sb.WriteString(fmt.Sprintf("Run %d: mode=%s, cost=$%.4f, alerts=%d, merged=%v — %s\n",
				i+1, r.Mode, totalCost, len(r.GuardianAlerts), r.Merged, truncate(r.InputDescription, 80)))
			continue
		}

		sb.WriteString(fmt.Sprintf("--- Run %d (mode=%s, %s) ---\n", i+1, r.Mode, r.StartedAt.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("  Input: %s\n", r.InputDescription))
		if r.FailedPhase != "" {
			sb.WriteString(fmt.Sprintf("  FAILED at %s: %s\n", r.FailedPhase, r.FailureReason))
		}
		if r.PRURL != "" {
			sb.WriteString(fmt.Sprintf("  PR: %s (merged=%v)\n", r.PRURL, r.Merged))
		}

		// Phase timings
		if len(r.PhaseTimings) > 0 {
			sb.WriteString("  Phase timings:\n")
			for _, pt := range r.PhaseTimings {
				sb.WriteString(fmt.Sprintf("    %-16s %s\n", pt.Phase, pt.Duration.Round(time.Millisecond)))
			}
		}

		// Token usage
		if len(r.TokenUsage) > 0 {
			var totalCost float64
			sb.WriteString("  Token usage by role:\n")
			for _, tu := range r.TokenUsage {
				sb.WriteString(fmt.Sprintf("    %-12s %s: %d tokens, $%.4f\n", tu.Role, tu.Model, tu.TotalTokens, tu.CostUSD))
				totalCost += tu.CostUSD
			}
			sb.WriteString(fmt.Sprintf("  Total cost: $%.4f\n", totalCost))
		}

		// Guardian alerts
		if len(r.GuardianAlerts) > 0 {
			sb.WriteString(fmt.Sprintf("  Guardian alerts (%d):\n", len(r.GuardianAlerts)))
			for _, a := range r.GuardianAlerts {
				sb.WriteString(fmt.Sprintf("    - %s\n", a))
			}
		}

		// Review signals
		if len(r.ReviewSignals) > 0 {
			sb.WriteString(fmt.Sprintf("  Review signals: %s\n", strings.Join(r.ReviewSignals, ", ")))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// parseSelfImproveRecommendation extracts a SelfImproveRecommendation from LLM output.
// Handles JSON embedded in markdown code blocks or plain text.
func parseSelfImproveRecommendation(response string) (SelfImproveRecommendation, error) {
	var rec SelfImproveRecommendation

	// Try direct unmarshal first.
	if err := json.Unmarshal([]byte(response), &rec); err == nil {
		return rec, nil
	}

	// Extract JSON from markdown code blocks or surrounding text.
	jsonStr := extractJSONBlock(response)
	if jsonStr == "" {
		// CTO responded in prose without JSON — treat as graceful stop.
		return SelfImproveRecommendation{SkipReason: response}, nil
	}

	if err := json.Unmarshal([]byte(jsonStr), &rec); err != nil {
		return rec, fmt.Errorf("parse recommendation JSON: %w", err)
	}
	return rec, nil
}

// extractJSONBlock finds the first JSON object in a string, handling markdown
// code blocks (```json ... ```) or bare JSON.
func extractJSONBlock(s string) string {
	// Try markdown code block first.
	if idx := strings.Index(s, "```json"); idx != -1 {
		start := idx + len("```json")
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx != -1 {
		start := idx + len("```")
		if end := strings.Index(s[start:], "```"); end != -1 {
			candidate := strings.TrimSpace(s[start : start+end])
			if len(candidate) > 0 && candidate[0] == '{' {
				return candidate
			}
		}
	}

	// Try to find bare JSON object.
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	// Find matching closing brace.
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
