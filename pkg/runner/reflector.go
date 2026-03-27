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

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
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

// normalizeReflectorResponse strips markdown code fences from LLM output
// so the parsers see clean content regardless of how the model wrapped it.
func normalizeReflectorResponse(content string) string {
	content = strings.TrimSpace(content)
	// Strip opening fence line: ```json, ```text, or plain ```
	if strings.HasPrefix(content, "```") {
		nl := strings.IndexByte(content, '\n')
		if nl >= 0 {
			content = strings.TrimSpace(content[nl+1:])
		}
	}
	// Strip closing fence
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSpace(content[:len(content)-3])
	}
	return content
}

// parseReflectorJSON attempts to parse content as JSON with cover/blind/zoom/formalize
// fields. Handles flat objects, {"reflection":{...}} wrappers, and prose preambles
// (by scanning for the first '{' that begins a valid JSON object).
// Returns nil if no valid JSON with at least cover present is found.
func parseReflectorJSON(content string) map[string]string {
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
// reflector LLM output. Normalizes fences first, then tries JSON, then falls
// back to text-marker formats. Picks the earliest-occurring match per key.
// The same candidate set is used for boundary detection so sections using any
// variant are correctly terminated.
// Returns a map of section name → trimmed content.
func parseReflectorOutput(content string) map[string]string {
	content = normalizeReflectorResponse(content)
	// Try JSON first — handles LLM responses that return raw JSON objects or
	// {"reflection":{...}} wrappers instead of the text-marker format.
	if result := parseReflectorJSON(content); result != nil {
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

// truncateArtifact limits artifact text to max bytes, appending a truncation marker if cut.
func truncateArtifact(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}

// buildReflectorPrompt assembles the prompt sent to the Reflector agent.
// The JSON-only constraint is front-loaded — before any context or artifacts —
// to prevent "lost in the middle" failures on long prompts.
// Artifact sizes are capped inline (separate from the global 8000-char cap).
func buildReflectorPrompt(scout, build, critique, recentReflections, sharedCtx string) string {
	build = truncateArtifact(build, 3000)
	critique = truncateArtifact(critique, 2000)
	scout = truncateArtifact(scout, 2000)
	sharedCtx = truncateArtifact(sharedCtx, 4000)

	return fmt.Sprintf(`Return ONLY a JSON object with exactly these four fields — no preamble, no explanation, no markdown code fences.

{
  "cover": "What was accomplished? How does it connect to prior work?",
  "blind": "What was missed? What is invisible to the current process?",
  "zoom": "Step back. What is the larger pattern across iterations?",
  "formalize": "If a new lesson emerged, state it as a numbered principle. Otherwise write: No new lesson."
}

You are the Reflector. You close each iteration by extracting what was learned.

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
Return the JSON object shown above. BLIND is the most important: actively look for absences. Keep it concise — 10-15 lines total.`, sharedCtx, scout, build, critique, recentReflections)
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

	// Check critique verdict from graph first.
	critique := r.readFromGraph("Critique:")
	if critique == "" {
		critique = readLoopArtifact(r.cfg.HiveDir, "critique.md")
	}
	if parseVerdict(critique) == "REVISE" {
		log.Printf("[reflector] critique verdict is REVISE — skipping reflection")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// Use Operate() so the Reflector can search prior reflections,
	// read artifacts from the graph, and assert lessons as claims.
	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if canOperate {
		r.runReflectorOperate(ctx, op)
	} else {
		r.runReflectorReason(ctx)
	}

	if r.cfg.OneShot {
		r.done = true
	}
}

// runReflectorOperate uses Operate() — searches knowledge, reads graph, writes reflections.
func (r *Runner) runReflectorOperate(ctx context.Context, op decision.IOperator) {
	apiKey := os.Getenv("LOVYOU_API_KEY")
	instruction := fmt.Sprintf(`You are the Reflector. Reflect on this iteration using COVER/BLIND/ZOOM/FORMALIZE.

## Your Tools
- Use knowledge.search to find prior reflections and lessons
- Use knowledge.get to read the backlog and design docs
- Use Bash to read the current loop artifacts:
  - cat loop/scout.md (gap report)
  - cat loop/build.md (build report)
  - cat loop/critique.md (critique)

## Steps
1. Read the current iteration's artifacts (scout, build, critique)
2. Search knowledge for prior reflections to avoid repeating insights
3. Reflect using the four sections:
   - **COVER** — what was explored, what was covered
   - **BLIND** — what was missed, what blind spots remain
   - **ZOOM** — was the scale right, should we zoom in or out
   - **FORMALIZE** — lessons to extract as verifiable knowledge

4. Write the reflection to loop/reflections.md (append)
5. Assert each lesson as a claim on the graph:
   curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "https://lovyou.ai/app/%s/op" -d '{"op":"assert","title":"Lesson: <LESSON>","body":"<DETAILS>"}'
6. Post the full reflection as a document:
   curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "https://lovyou.ai/app/%s/op" -d '{"op":"intend","kind":"document","title":"Reflection: <DATE>","description":"<FULL REFLECTION>"}'

Search first. Reflect deeply. Assert what you learn.`, apiKey, r.cfg.SpaceSlug, apiKey, r.cfg.SpaceSlug)

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.HiveDir,
		Instruction: instruction,
	})
	if err != nil {
		log.Printf("[reflector] Operate error: %v", err)
		return
	}
	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[reflector] Operate done (cost=$%.4f)", result.Usage.CostUSD)
}

// runReflectorReason is the legacy fallback.
func (r *Runner) runReflectorReason(ctx context.Context) {
	scout := r.readFromGraph("Scout Report:")
	if scout == "" {
		scout = readLoopArtifact(r.cfg.HiveDir, "scout.md")
	}
	build := r.readFromGraph("Build:")
	if build == "" {
		build = readLoopArtifact(r.cfg.HiveDir, "build.md")
	}
	critique := r.readFromGraph("Critique:")
	if critique == "" {
		critique = readLoopArtifact(r.cfg.HiveDir, "critique.md")
	}

	recentReflections := readRecentReflections(r.cfg.HiveDir)
	sharedCtx := LoadSharedContext(r.cfg.HiveDir)

	prompt := buildReflectorPrompt(scout, build, critique, recentReflections, sharedCtx)
	resp, err := r.cfg.Provider.Reason(ctx, prompt, nil)
	if err != nil {
		log.Printf("[reflector] Reason error: %v", err)
		return
	}

	r.cost.Record(resp.Usage())
	r.dailyBudget.Record(resp.Usage().CostUSD)
	log.Printf("[reflector] Reason done (cost=$%.4f)", resp.Usage().CostUSD)

	sections := parseReflectorOutput(resp.Content())

	emptySections := false
	for _, key := range []string{"COVER", "BLIND", "ZOOM", "FORMALIZE"} {
		if sections[key] == "" {
			emptySections = true
			break
		}
	}
	if emptySections {
		return
	}

	date := time.Now().Format("2006-01-02")
	entry := formatReflectionEntry(date, sections["COVER"], sections["BLIND"], sections["ZOOM"], sections["FORMALIZE"])
	if err := r.appendReflection(entry); err != nil {
		log.Printf("[reflector] append error: %v", err)
	}

	if formalize := sections["FORMALIZE"]; formalize != "" && r.cfg.APIClient != nil {
		title := fmt.Sprintf("Lesson: %s", date)
		if _, err := r.cfg.APIClient.AssertClaim(r.cfg.SpaceSlug, title, formalize); err != nil {
			log.Printf("[reflector] assert lesson error: %v", err)
		}
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

// readFromGraph reads the latest node matching a title prefix from the graph.
// Only returns nodes created within the last 2 hours (avoids stale data from prior cycles).
// Returns body content, or "" if not found, stale, or API unavailable.
func (r *Runner) readFromGraph(titlePrefix string) string {
	if r.cfg.APIClient == nil {
		return ""
	}
	node := r.cfg.APIClient.LatestByTitle(r.cfg.SpaceSlug, titlePrefix)
	if node == nil {
		return ""
	}
	// Ignore stale nodes — only use data from the current cycle window.
	if created, err := time.Parse(time.RFC3339, node.CreatedAt); err == nil {
		if time.Since(created) > 2*time.Hour {
			return ""
		}
	}
	return node.Body
}

// appendReflection appends an entry to loop/reflections.md (creates if absent).
func (r *Runner) appendReflection(entry string) error {
	path := filepath.Join(r.cfg.HiveDir, "loop", "reflections.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + entry); err != nil {
		return err
	}

	// Also post to graph as a document.
	if r.cfg.APIClient != nil {
		title := fmt.Sprintf("Reflection: %s", time.Now().UTC().Format("2006-01-02"))
		_, _ = r.cfg.APIClient.CreateDocument(r.cfg.SpaceSlug, title, entry)
	}
	return nil
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
