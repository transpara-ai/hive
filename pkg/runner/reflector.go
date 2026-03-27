package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// markerCandidates returns all format variants the LLM might use for a section key.
// Callers pick the earliest-occurring match across the set.
func markerCandidates(key string) []string {
	return []string{
		"**" + key + ":**",
		"**" + key + "**:",
		"**" + key + "** :",
		"### " + key + ":",
		"## " + key + ":",
		key + ":",
		strings.ToLower(key) + ":",
	}
}

// jsonReflectorOutput is the expected shape when an LLM returns JSON instead of
// the text-marker format. Field names are lowercase as commonly produced by LLMs.
type jsonReflectorOutput struct {
	Cover     string `json:"cover"`
	Blind     string `json:"blind"`
	Zoom      string `json:"zoom"`
	Formalize string `json:"formalize"`
}

// parseReflectorOutputJSON attempts to parse content as JSON with COVER/BLIND/ZOOM/FORMALIZE
// fields. Handles flat objects, {"reflection":{...}} wrappers, and prose preambles
// (by scanning for the first '{' that begins a valid JSON object).
// Returns nil if no valid JSON with at least COVER present is found.
func parseReflectorOutputJSON(content string) map[string]string {
	// Strip markdown code fences.
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		if nl := strings.IndexByte(content, '\n'); nl >= 0 {
			content = strings.TrimSpace(content[nl+1:])
		}
	}
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSpace(content[:len(content)-3])
	}

	// Scan for each '{' — handles prose preamble before the JSON block.
	for i, ch := range content {
		if ch != '{' {
			continue
		}
		sub := content[i:]

		var ref jsonReflectorOutput
		if err := json.Unmarshal([]byte(sub), &ref); err == nil && ref.Cover != "" {
			return map[string]string{
				"COVER":     ref.Cover,
				"BLIND":     ref.Blind,
				"ZOOM":      ref.Zoom,
				"FORMALIZE": ref.Formalize,
			}
		}

		var wrapper struct {
			Reflection jsonReflectorOutput `json:"reflection"`
		}
		if err := json.Unmarshal([]byte(sub), &wrapper); err == nil && wrapper.Reflection.Cover != "" {
			ref = wrapper.Reflection
			return map[string]string{
				"COVER":     ref.Cover,
				"BLIND":     ref.Blind,
				"ZOOM":      ref.Zoom,
				"FORMALIZE": ref.Formalize,
			}
		}
	}
	return nil
}

// parseReflectorOutput extracts COVER/BLIND/ZOOM/FORMALIZE sections from
// reflector LLM output. Tries JSON first, then text-marker formats. Picks the
// earliest-occurring match per key. The same candidate set is used for boundary
// detection so sections using any variant are correctly terminated.
// Returns a map of section name → trimmed content.
func parseReflectorOutput(content string) map[string]string {
	// Try JSON first — handles LLM responses that return raw JSON objects or
	// {"reflection":{...}} wrappers instead of the text-marker format.
	if result := parseReflectorOutputJSON(content); result != nil {
		return result
	}
	keys := []string{"COVER", "BLIND", "ZOOM", "FORMALIZE"}
	result := map[string]string{}

	for i, key := range keys {
		// Find the earliest-occurring marker among all candidates.
		bestIdx := -1
		bestEnd := -1
		for _, candidate := range markerCandidates(key) {
			if idx := strings.Index(content, candidate); idx >= 0 {
				if bestIdx < 0 || idx < bestIdx {
					bestIdx = idx
					bestEnd = idx + len(candidate)
				}
			}
		}
		if bestIdx < 0 {
			continue
		}

		start := bestEnd

		// Find where this section ends (start of next section).
		// Check all candidate formats for each subsequent key so that a section
		// using e.g. "## BLIND:" is found as the boundary for COVER.
		end := len(content)
		for _, nextKey := range keys[i+1:] {
			for _, nextCandidate := range markerCandidates(nextKey) {
				if nextIdx := strings.Index(content[start:], nextCandidate); nextIdx >= 0 {
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

Produce a reflection entry as a JSON object with exactly these four fields:

{
  "cover": "What was accomplished? How does it connect to prior work?",
  "blind": "What was missed? What is invisible to the current process?",
  "zoom": "Step back. What is the larger pattern across iterations?",
  "formalize": "If a new lesson emerged, state it as a numbered principle. Otherwise write: No new lesson."
}

Return ONLY the JSON object. No preamble, no explanation, no markdown code fences. Keep it concise — 10-15 lines total. BLIND is the most important: actively look for absences.`, sharedCtx, scout, build, critique, recentReflections)
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
		preview := resp.Content()
		if len(preview) > 500 {
			preview = preview[:500]
		}
		log.Printf("[reflector] empty sections in response: %s", preview)
		usage := resp.Usage()
		r.appendDiagnostic(PhaseEvent{
			Phase:        "reflector",
			Outcome:      "empty_sections",
			Preview:      preview,
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
