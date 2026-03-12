package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"

	"github.com/lovyou-ai/hive/pkg/loop"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
)

// ════════════════════════════════════════════════════════════════════════
// Guardian
// ════════════════════════════════════════════════════════════════════════

// guardianCheck runs a Guardian integrity check after a pipeline phase.
// Returns true if the Guardian issues a HALT (pipeline should stop).
func (p *Pipeline) guardianCheck(ctx context.Context, phase string) bool {
	if p.skipGuardian {
		return false
	}
	events, err := p.guardian.Runtime.Memory(200)
	if err != nil || len(events) == 0 {
		return false
	}

	var summary strings.Builder
	for _, ev := range events {
		summary.WriteString(fmt.Sprintf("[%s] %s: %s\n", ev.Type().Value(), ev.Source().Value(), ev.ID().Value()))
	}

	prompt := fmt.Sprintf(`Review these recent events (after %s phase) for policy violations, trust anomalies, or authority overreach.

Report ONLY confirmed policy violations. If none found, respond: No violations detected.

Events:
%s`,
		phase, summary.String())

	rawProvider, err := p.providerForRoleWithModel(roles.RoleGuardian, p.guardianCheckModel())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Guardian check failed: %v\n", err)
		p.emitWarning("", "Guardian check failed: %v", err)
		return false
	}
	tracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleGuardian] = tracker

	resp, err := tracker.Reason(ctx, prompt, nil)
	// Accumulate Guardian usage before error check — partial usage on failure
	// is still real spend and should appear in telemetry.
	p.telemetry.accumulateGuardianUsage(tracker)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Guardian check failed: %v\n", err)
		p.emitWarning("", "Guardian check failed: %v", err)
		return false
	}
	eval := resp.Content()

	if loop.ContainsSignal(eval, "HALT") {
		fmt.Fprintf(os.Stderr, "Guardian HALT (after %s):\n%s\n", phase, eval)
		p.emitWarning(Phase(phase), "Guardian HALT: %s", eval)
		// NOTE: Emit is context-unaware (eventgraph Runtime.Emit doesn't take ctx).
		// This is acceptable — the HALT event is best-effort observability, not
		// control flow. The pipeline stops regardless of whether the event persists.
		if _, err := p.guardian.Runtime.Emit(event.AgentEscalatedContent{
			AgentID:   p.guardian.Runtime.ID(),
			Authority: p.humanID,
			Reason:    fmt.Sprintf("[HALT after %s] %s", phase, eval),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: HALT escalation event failed: %v\n", err)
		}
		return true
	}

	if containsAlert(eval) {
		fmt.Fprintf(os.Stderr, "Guardian Alert (after %s):\n%s\n", phase, eval)
		p.emitWarning(Phase(phase), "Guardian Alert: %s", eval)
		if p.telemetry != nil {
			p.telemetry.addGuardianAlert(fmt.Sprintf("[%s phase] %s", phase, eval))
		}
		if _, err := p.guardian.Runtime.Emit(event.AgentEscalatedContent{
			AgentID:   p.guardian.Runtime.ID(),
			Authority: p.humanID,
			Reason:    fmt.Sprintf("[%s phase] %s", phase, eval),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: alert escalation event failed: %v\n", err)
		}
	}

	return false
}

// guardianCheckModel returns the model to use for Guardian integrity checks.
// Defaults to Sonnet — Guardian's task is a binary HALT/pass classification
// (read phase events, emit HALT or pass), not deep architectural reasoning.
func (p *Pipeline) guardianCheckModel() string {
	if p.guardianModel != "" {
		return p.guardianModel
	}
	return "claude-sonnet-4-6"
}

// architectDesignModel returns the model to use for Architect design and simplify calls.
// Defaults to Sonnet — spec-writing and simplification are structured generation tasks,
// not deep architectural reasoning.
func (p *Pipeline) architectDesignModel() string {
	if p.architectModel != "" {
		return p.architectModel
	}
	return "claude-sonnet-4-6"
}

// fullBuilderModel returns the model to use for full-pipeline Builder code generation.
// Defaults to Sonnet — code generation from a spec is structured output, not deep
// architectural reasoning. Up to 4 calls (build + 3 rebuild rounds) make the ~5x
// cost difference vs Opus significant. Override with Config.BuilderModel.
func (p *Pipeline) fullBuilderModel() string {
	if p.builderModel != "" {
		return p.builderModel
	}
	return "claude-sonnet-4-6"
}

// fullReviewerModel returns the model to use for full-pipeline code reviews.
// Defaults to Sonnet — pass/fail classification with specific feedback doesn't
// require Opus-level reasoning. Override with Config.ReviewerModel.
func (p *Pipeline) fullReviewerModel() string {
	if p.reviewerModel != "" {
		return p.reviewerModel
	}
	return "claude-sonnet-4-6"
}

// fullTesterModel returns the model to use for Tester analysis and test-failure
// evaluation calls. Defaults to Sonnet — pass/fail classification with root-cause
// analysis doesn't require Opus-level reasoning. Override with Config.TesterModel
// if needed (no override field yet; extend Pipeline struct when required).
func (p *Pipeline) fullTesterModel() string {
	return "claude-sonnet-4-6"
}

// containsAlert checks if the Guardian's evaluation contains an alert directive.
// Uses line-start matching (via ContainsSignal) to avoid false positives from
// prose like "NO VIOLATIONS DETECTED".
// HALT is handled separately by the explicit HALT check in guardianCheck —
// it is not included here to avoid duplicate event emission.
func containsAlert(eval string) bool {
	for _, keyword := range []string{"ALERT", "VIOLATION", "QUARANTINE"} {
		if loop.ContainsSignal(eval, keyword) {
			return true
		}
	}
	return false
}

// ════════════════════════════════════════════════════════════════════════
// Review / language detection
// ════════════════════════════════════════════════════════════════════════

// detectApproval checks if a review response ends with APPROVED (not CHANGES NEEDED).
func detectApproval(review string) bool {
	lines := strings.Split(review, "\n")
	// Look at the last 20 lines for the verdict
	start := len(lines) - 20
	if start < 0 {
		start = 0
	}
	verdict := strings.ToUpper(strings.Join(lines[start:], "\n"))

	hasChanges := strings.Contains(verdict, "CHANGES NEEDED") ||
		strings.Contains(verdict, "CHANGES REQUIRED") ||
		strings.Contains(verdict, "REJECT")
	hasApproved := strings.Contains(verdict, "APPROVED")
	return hasApproved && !hasChanges
}

// detectLanguage infers the language from existing project files.
func detectLanguage(files map[string]string) string {
	if _, ok := files["go.mod"]; ok {
		return "go"
	}
	if _, ok := files["package.json"]; ok {
		return "typescript"
	}
	if _, ok := files["Cargo.toml"]; ok {
		return "rust"
	}
	if _, ok := files["requirements.txt"]; ok {
		return "python"
	}
	if _, ok := files["setup.py"]; ok {
		return "python"
	}
	if _, ok := files["pyproject.toml"]; ok {
		return "python"
	}
	for path := range files {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			return "go"
		case ".rs":
			return "rust"
		case ".py":
			return "python"
		case ".ts", ".tsx":
			return "typescript"
		case ".js", ".jsx":
			return "javascript"
		case ".cs":
			return "csharp"
		}
	}
	return "go"
}

// ════════════════════════════════════════════════════════════════════════
// File listing / key files
// ════════════════════════════════════════════════════════════════════════

// buildFileListing creates a compact listing of files with line counts.
// This gives the CTO enough context to identify relevant files without
// sending the full codebase (~90% token reduction vs full content).
func buildFileListing(files map[string]string) string {
	var b strings.Builder
	b.WriteString("Files:\n")
	for path, content := range files {
		lines := strings.Count(content, "\n") + 1
		b.WriteString(fmt.Sprintf("  %s (%d lines)\n", path, lines))
	}
	return b.String()
}

// extractKeyFiles returns the content of project-level context files
// (CLAUDE.md, README, etc.) that help the CTO understand the project
// without needing the full codebase. Each file is capped at 60 lines —
// the CTO only needs to identify which files to change, not read full docs.
func extractKeyFiles(files map[string]string) string {
	const maxLines = 60
	keyNames := []string{"CLAUDE.md", "README.md", "README", "SPEC.md", "ARCHITECTURE.md"}
	var b strings.Builder
	for _, name := range keyNames {
		content, ok := files[name]
		if !ok {
			continue
		}
		lines := strings.Split(content, "\n")
		if len(lines) > maxLines {
			content = strings.Join(lines[:maxLines], "\n") +
				fmt.Sprintf("\n... [truncated: %d lines omitted]", len(lines)-maxLines)
		}
		b.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", name, content))
	}
	return b.String()
}

// parseRelevantFiles extracts file paths from CTO analysis output.
// If the output contains a FILES_TO_CHANGE: section (as emitted by the CTO
// system prompt), only paths from that section are returned — stopping at the
// first blank line or non-path line after the header. This prevents files
// mentioned in reasoning or risk sections from polluting the result.
//
// Falls back to a word-by-word scan for backward compatibility when no
// FILES_TO_CHANGE: section is present (e.g., pre-computed self-improve analysis).
func parseRelevantFiles(ctoAnalysis string) []string {
	seen := make(map[string]bool)
	var paths []string

	lines := strings.Split(ctoAnalysis, "\n")

	// Structured extraction: look for FILES_TO_CHANGE: section header.
	for i, line := range lines {
		if strings.TrimSpace(line) == "FILES_TO_CHANGE:" {
			for _, entry := range lines[i+1:] {
				trimmed := strings.TrimSpace(entry)
				if trimmed == "" {
					break // blank line ends the section
				}
				// Each entry is "path — description" or just "path".
				word := strings.Fields(trimmed)[0]
				word = strings.Trim(word, "-*•:,;()[]`\"'")
				if !looksLikeFilePath(word) {
					break // non-path line ends the section
				}
				if !seen[word] {
					seen[word] = true
					paths = append(paths, word)
				}
			}
			return paths
		}
	}

	// Fallback: word-by-word scan across the entire output.
	// Used when the CTO analysis predates the structured section format
	// (e.g., self-improve pre-computed CTOAnalysis).
	for _, line := range lines {
		for _, word := range strings.Fields(line) {
			word = strings.Trim(word, "-*•:,;()[]`\"'")
			if looksLikeFilePath(word) && !seen[word] {
				seen[word] = true
				paths = append(paths, word)
			}
		}
	}
	return paths
}

// buildRelevantFileContext returns the concatenated contents of files from the
// provided map, limited to the paths in relevantPaths and formatted with
// --- FILE: path --- markers. Used to embed pre-filtered source context into
// builder instructions, reducing agentic-mode file reads and stabilising
// builder token usage across self-improve iterations.
func buildRelevantFileContext(files map[string]string, relevantPaths []string) string {
	var b strings.Builder
	for _, path := range relevantPaths {
		content, ok := files[path]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n\n", path, content))
	}
	return b.String()
}

// looksLikeFilePath returns true if s looks like a source file path.
// Accepts paths with a letter-prefixed extension (e.g. ".go", ".md") and
// rejects URLs, version strings (e.g. "v1.2.3"), and empty strings.
func looksLikeFilePath(s string) bool {
	if s == "" || strings.HasPrefix(s, "http") || strings.HasPrefix(s, "//") {
		return false
	}
	ext := filepath.Ext(s)
	if len(ext) < 2 || len(ext) > 6 {
		return false
	}
	// Extension must start with a letter (rejects version strings like "1.2.3")
	c := ext[1]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ════════════════════════════════════════════════════════════════════════
// String utilities
// ════════════════════════════════════════════════════════════════════════

// sanitizeBranchName converts a description into a valid git branch name.
func sanitizeBranchName(desc string) string {
	// Truncate at word boundary before 40 chars, lowercase, replace non-alphanumeric with hyphens
	s := strings.ToLower(desc)
	if len(s) > 40 {
		// Find last space/separator at or before 40 chars
		cut := strings.LastIndexAny(s[:40], " _/")
		if cut > 0 {
			s = s[:cut]
		} else {
			// First word exceeds 40 chars — hard truncate
			s = s[:40]
		}
	}
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if c == ' ' || c == '_' || c == '/' {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "change"
	}
	return result
}

// truncate returns s truncated to n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ════════════════════════════════════════════════════════════════════════
// File parsing utilities
// ════════════════════════════════════════════════════════════════════════

// sanitizeGoMod fixes common LLM go.mod corruption.
// The most common issue is newlines embedded in the module path string,
// producing `module "github.com/\nfoo/bar"` which Go rejects.
// Fix: rejoin any line that looks like a continuation of a module directive.
func sanitizeGoMod(files map[string]string) {
	content, ok := files["go.mod"]
	if !ok {
		return
	}

	lines := strings.Split(content, "\n")
	var cleaned []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// If a module/go/require line is split across multiple lines, rejoin.
		if strings.HasPrefix(trimmed, "module ") || trimmed == "module" {
			// Collect continuation lines until we have the full module path.
			joined := trimmed
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next == "" || strings.HasPrefix(next, "go ") ||
					strings.HasPrefix(next, "require") ||
					strings.HasPrefix(next, "replace") ||
					strings.HasPrefix(next, "module ") {
					break
				}
				// This line is a continuation of the module directive.
				joined += next
				i++
			}
			// Remove any quotes around the module path.
			joined = strings.ReplaceAll(joined, "\"", "")
			cleaned = append(cleaned, joined)
		} else {
			cleaned = append(cleaned, line)
		}
	}
	files["go.mod"] = strings.Join(cleaned, "\n")
}

// parseFiles extracts files from builder output using --- FILE: path --- markers.
// Strips markdown code fences (```lang / ```) that LLMs sometimes wrap around file content.
func parseFiles(output string) map[string]string {
	files := make(map[string]string)
	lines := strings.Split(output, "\n")

	var currentPath string
	var currentContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- FILE:") && strings.HasSuffix(trimmed, "---") {
			// Save previous file if any
			if currentPath != "" {
				files[currentPath] = stripMarkdownFences(strings.TrimRight(currentContent.String(), "\n"))
			}
			// Extract new path
			path := strings.TrimSpace(trimmed[len("--- FILE:") : len(trimmed)-len("---")])
			currentPath = path
			currentContent.Reset()
		} else if currentPath != "" {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}
	// Save last file
	if currentPath != "" {
		files[currentPath] = stripMarkdownFences(strings.TrimRight(currentContent.String(), "\n"))
	}

	return files
}

// stripMarkdownFences removes markdown code fences and trailing prose from file content.
// Handles: opening ```lang on first line, closing ``` on last code line,
// and any trailing markdown text after the closing fence.
func stripMarkdownFences(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	start := 0
	end := len(lines)

	// Strip leading markdown fence (```go, ```python, etc.)
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		start = 1
	}

	// Find and strip trailing markdown fence + any prose after it
	for i := end - 1; i >= start; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "```" {
			end = i
			break
		}
	}

	if start >= end {
		return content // safety: don't return empty if something went wrong
	}

	return strings.Join(lines[start:end], "\n")
}

// ════════════════════════════════════════════════════════════════════════
// Language utilities
// ════════════════════════════════════════════════════════════════════════

// langMarkerFile returns the build-system marker file for a language.
// Used by findModuleDir to locate the module root.
func langMarkerFile(lang string) string {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return "go.mod"
	case "typescript", "ts", "javascript", "js":
		return "package.json"
	case "python", "py":
		return "pyproject.toml"
	case "rust", "rs":
		return "Cargo.toml"
	case "csharp", "c#", "cs":
		// *.csproj — use a glob pattern handled specially below.
		return "*.csproj"
	default:
		return "go.mod"
	}
}

// findModuleDir locates the directory containing the language's module marker
// file (e.g. go.mod for Go). It checks productDir first, then walks one level
// of subdirectories. Returns productDir as fallback if no marker is found.
// When multiple subdirectories contain the marker, the first alphabetically
// is returned for determinism.
func findModuleDir(productDir, lang string) string {
	marker := langMarkerFile(lang)

	// Check root first.
	if markerExists(productDir, marker) {
		return productDir
	}

	// Walk one level of subdirectories.
	entries, err := os.ReadDir(productDir)
	if err != nil {
		return productDir
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(productDir, entry.Name())
		if markerExists(subdir, marker) {
			return subdir
		}
	}

	return productDir
}

// markerExists checks whether a marker file exists in dir.
// Supports glob patterns (e.g. "*.csproj").
func markerExists(dir, marker string) bool {
	if strings.Contains(marker, "*") {
		matches, err := filepath.Glob(filepath.Join(dir, marker))
		return err == nil && len(matches) > 0
	}
	_, err := os.Stat(filepath.Join(dir, marker))
	return err == nil
}

// langExtension returns the default file extension for a language.
func langExtension(lang string) string {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return ".go"
	case "typescript", "ts":
		return ".ts"
	case "javascript", "js":
		return ".js"
	case "python", "py":
		return ".py"
	case "rust", "rs":
		return ".rs"
	case "csharp", "c#", "cs":
		return ".cs"
	default:
		return ".go"
	}
}

// langTestCommand returns the test command and args for a language.
func langTestCommand(lang string) (string, []string) {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return "go", []string{"test", "./..."}
	case "typescript", "ts":
		return "npx", []string{"vitest", "run"}
	case "javascript", "js":
		return "npm", []string{"test"}
	case "python", "py":
		return "python", []string{"-m", "pytest"}
	case "rust", "rs":
		return "cargo", []string{"test"}
	case "csharp", "c#", "cs":
		return "dotnet", []string{"test"}
	default:
		return "go", []string{"test", "./..."}
	}
}

// ════════════════════════════════════════════════════════════════════════
// Token summaries
// ════════════════════════════════════════════════════════════════════════

// PrintTokenSummary prints per-agent token usage from tracking providers.
func (p *Pipeline) PrintTokenSummary() {
	fmt.Fprintln(os.Stderr, "\n═══ Token Usage Summary ═══")
	fmt.Fprintf(os.Stderr, "  %-12s %-8s %8s %8s %8s %10s %10s %10s\n",
		"Role", "Model", "Input", "Output", "Total", "CacheRead", "CacheWrite", "Cost")
	fmt.Fprintf(os.Stderr, "  %-12s %-8s %8s %8s %8s %10s %10s %10s\n",
		"────", "─────", "─────", "──────", "─────", "─────────", "──────────", "────")

	var totalIn, totalOut, totalTokens, totalCacheRead, totalCacheWrite int
	var totalCost float64

	for role, tracker := range p.trackers {
		s := tracker.Snapshot()
		fmt.Fprintf(os.Stderr, "  %-12s %-8s %8d %8d %8d %10d %10d %10s\n",
			role, tracker.Model(), s.InputTokens, s.OutputTokens, s.TokensUsed,
			s.CacheReadTokens, s.CacheWriteTokens, fmt.Sprintf("$%.4f", s.CostUSD))
		p.emitTelemetryEntry(string(role), tracker.Model(), s.InputTokens, s.OutputTokens, s.TokensUsed, s.CacheReadTokens, s.CacheWriteTokens, s.CostUSD)
		totalIn += s.InputTokens
		totalOut += s.OutputTokens
		totalTokens += s.TokensUsed
		totalCacheRead += s.CacheReadTokens
		totalCacheWrite += s.CacheWriteTokens
		totalCost += s.CostUSD
	}

	fmt.Fprintf(os.Stderr, "  %-12s %-8s %8d %8d %8d %10d %10d %10s\n",
		"TOTAL", "", totalIn, totalOut, totalTokens,
		totalCacheRead, totalCacheWrite, fmt.Sprintf("$%.4f", totalCost))
}

// printTokenSummary prints an aggregate token usage table from loop results.
func printTokenSummary(results []loop.AgentResult) {
	var totalIn, totalOut, totalCacheRead, totalCacheWrite, totalTokens int
	var totalCost float64

	fmt.Fprintln(os.Stderr, "\n═══ Token Usage Summary ═══")
	fmt.Fprintf(os.Stderr, "  %-12s %8s %8s %8s %10s %10s %10s\n",
		"Role", "Input", "Output", "Total", "CacheRead", "CacheWrite", "Cost")
	fmt.Fprintf(os.Stderr, "  %-12s %8s %8s %8s %10s %10s %10s\n",
		"────", "─────", "──────", "─────", "─────────", "──────────", "────")

	for _, ar := range results {
		b := ar.Result.Budget
		fmt.Fprintf(os.Stderr, "  %-12s %8d %8d %8d %10d %10d %10s\n",
			ar.Role, b.InputTokens, b.OutputTokens, b.TokensUsed,
			b.CacheReadTokens, b.CacheWriteTokens, fmt.Sprintf("$%.4f", b.CostUSD))
		totalIn += b.InputTokens
		totalOut += b.OutputTokens
		totalTokens += b.TokensUsed
		totalCacheRead += b.CacheReadTokens
		totalCacheWrite += b.CacheWriteTokens
		totalCost += b.CostUSD
	}

	fmt.Fprintf(os.Stderr, "  %-12s %8d %8d %8d %10d %10d %10s\n",
		"TOTAL", totalIn, totalOut, totalTokens,
		totalCacheRead, totalCacheWrite, fmt.Sprintf("$%.4f", totalCost))
}
