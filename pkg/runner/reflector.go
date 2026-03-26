package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// parseReflectorOutput extracts COVER/BLIND/ZOOM/FORMALIZE sections from
// reflector LLM output. Sections are delimited by "**KEY:**" or "KEY:" markers.
// Returns a map of section name → trimmed content.
func parseReflectorOutput(content string) map[string]string {
	keys := []string{"COVER", "BLIND", "ZOOM", "FORMALIZE"}
	result := map[string]string{}

	for i, key := range keys {
		// Try bold markdown first: **KEY:**
		marker := "**" + key + ":**"
		idx := strings.Index(content, marker)
		markerLen := len(marker)

		if idx < 0 {
			// Fallback: plain KEY:
			marker = key + ":"
			idx = strings.Index(content, marker)
			markerLen = len(marker)
		}
		if idx < 0 {
			continue
		}

		start := idx + markerLen

		// Find where this section ends (start of next section).
		end := len(content)
		for _, nextKey := range keys[i+1:] {
			for _, nextMarker := range []string{"**" + nextKey + ":**", nextKey + ":"} {
				if nextIdx := strings.Index(content[start:], nextMarker); nextIdx >= 0 {
					if abs := start + nextIdx; abs < end {
						end = abs
					}
				}
			}
		}

		result[key] = strings.TrimSpace(content[start:end])
	}

	return result
}

// buildReflectorPrompt assembles the prompt sent to the Reflector agent.
// Artifacts: scout report, build report, critique, recent reflections, shared context.
func buildReflectorPrompt(scout, build, critique, recentReflections, sharedCtx string) string {
	return fmt.Sprintf(`You are the Reflector. You close each iteration by extracting what was learned.

## Institutional Knowledge
%s

## Scout Report (loop/scout.md)
%s

## Build Report (loop/build.md)
%s

## Critique (loop/critique.md)
%s

## Recent Reflections (loop/reflections.md)
%s

## Instructions

Produce a reflection entry with exactly these four sections:

**COVER:** What was accomplished? How does it connect to prior work?

**BLIND:** What was missed? What is invisible to the current process?

**ZOOM:** Step back. What is the larger pattern across iterations?

**FORMALIZE:** If a new lesson emerged, state it as a numbered principle. Otherwise write "No new lesson."

Keep it concise — 10-15 lines total. BLIND is the most important: actively look for absences.`, sharedCtx, scout, build, critique, recentReflections)
}

// formatReflectionEntry formats a dated append block for loop/reflections.md.
// date should be ISO 8601 (e.g. "2026-03-26").
func formatReflectionEntry(date, cover, blind, zoom, formalize string) string {
	return fmt.Sprintf("## %s\n\n**COVER:** %s\n\n**BLIND:** %s\n\n**ZOOM:** %s\n\n**FORMALIZE:** %s\n",
		date, cover, blind, zoom, formalize)
}

// runReflector closes the loop iteration: reads artifacts, calls LLM, appends
// to reflections.md, and increments the iteration counter in state.md.
func (r *Runner) runReflector(ctx context.Context) {
	// Only run every 4th tick. Always run in one-shot mode.
	if !r.cfg.OneShot && r.tick%4 != 0 {
		return
	}

	if r.cfg.HiveDir == "" {
		log.Printf("[reflector] tick %d: no HiveDir configured", r.tick)
		return
	}

	log.Printf("[reflector] tick %d: reflecting", r.tick)

	// Read loop artifacts (all optional — tolerate missing files).
	scout := readLoopArtifact(r.cfg.HiveDir, "scout.md")
	build := readLoopArtifact(r.cfg.HiveDir, "build.md")
	critique := readLoopArtifact(r.cfg.HiveDir, "critique.md")
	recentReflections := readRecentReflections(r.cfg.HiveDir)
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)

	// Call LLM.
	prompt := buildReflectorPrompt(scout, build, critique, recentReflections, sharedCtx)
	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[reflector] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[reflector] Reason done (cost=$%.4f)", resp.Usage().CostUSD)

	// Parse the four sections.
	sections := parseReflectorOutput(resp.Content())

	// Validate that all four sections have content.
	emptySections := false
	for _, key := range []string{"COVER", "BLIND", "ZOOM", "FORMALIZE"} {
		if sections[key] == "" {
			emptySections = true
			break
		}
	}
	if emptySections {
		raw := resp.Content()
		if len(raw) > 500 {
			raw = raw[:500]
		}
		log.Printf("[reflector] empty sections in response: %s", raw)
		usage := resp.Usage()
		r.appendDiagnostic(PhaseEvent{
			Phase:        "reflector",
			Outcome:      "empty_sections",
			CostUSD:      usage.CostUSD,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
		})
		return
	}

	// Append to reflections.md.
	date := time.Now().Format("2006-01-02")
	entry := formatReflectionEntry(
		date,
		sections["COVER"],
		sections["BLIND"],
		sections["ZOOM"],
		sections["FORMALIZE"],
	)
	if err := appendReflection(r.cfg.HiveDir, entry); err != nil {
		log.Printf("[reflector] append reflections error: %v", err)
	}

	// Advance state.md iteration counter.
	if err := advanceIterationCounter(r.cfg.HiveDir, date); err != nil {
		log.Printf("[reflector] update state.md error: %v", err)
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

// readLoopArtifact reads a file from the loop/ directory. Returns empty string on error.
func readLoopArtifact(hiveDir, name string) string {
	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", name))
	if err != nil {
		return ""
	}
	return string(data)
}

// readRecentReflections returns the last 2000 bytes of loop/reflections.md.
func readRecentReflections(hiveDir string) string {
	data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "reflections.md"))
	if err != nil {
		return ""
	}
	if len(data) > 2000 {
		return "..." + string(data[len(data)-2000:])
	}
	return string(data)
}

// appendReflection appends an entry to loop/reflections.md (creates if absent).
func appendReflection(hiveDir, entry string) error {
	path := filepath.Join(hiveDir, "loop", "reflections.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n" + entry)
	return err
}

// advanceIterationCounter reads state.md, increments "Last updated: Iteration N," and writes it back.
func advanceIterationCounter(hiveDir, date string) error {
	path := filepath.Join(hiveDir, "loop", "state.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, _ := incrementIterationLine(string(data), date)
	return os.WriteFile(path, []byte(updated), 0644)
}

// incrementIterationLine finds "Last updated: Iteration N," in content and replaces it
// with N+1 and the given date. Returns updated content and new iteration number.
func incrementIterationLine(content, date string) (string, int) {
	const prefix = "Last updated: Iteration "
	idx := strings.Index(content, prefix)
	if idx < 0 {
		return content, 0
	}

	rest := content[idx+len(prefix):]
	end := strings.IndexAny(rest, ",.")
	if end < 0 {
		return content, 0
	}

	var n int
	fmt.Sscanf(rest[:end], "%d", &n)
	n++

	// Locate the end of the original line.
	lineEnd := strings.IndexByte(content[idx:], '\n')
	if lineEnd < 0 {
		lineEnd = len(content) - idx
	}

	newLine := fmt.Sprintf("Last updated: Iteration %d, %s.", n, date)
	return content[:idx] + newLine + content[idx+lineEnd:], n
}
