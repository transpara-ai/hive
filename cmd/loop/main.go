// Command loop runs one iteration of the hive's core loop.
//
// It orchestrates pipeline agents in sequence: each agent is a Claude CLI
// invocation with a specific system prompt. Artifacts are written to loop/
// files. Results are posted to lovyou.ai.
//
// This is the bridge from manual iterations (human runs the loop in conversation)
// to autonomous iterations (the hive runs the loop itself).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PipelineShape defines which agents run for an iteration.
type PipelineShape struct {
	Name   string
	Agents []string
}

var shapes = map[string]PipelineShape{
	"quick":    {Name: "quick", Agents: []string{"scout", "builder", "critic", "reflector"}},
	"standard": {Name: "standard", Agents: []string{"scout", "architect", "builder", "critic", "reflector"}},
	"designed": {Name: "designed", Agents: []string{"scout", "architect", "designer", "builder", "tester", "critic", "reflector"}},
	"full":     {Name: "full", Agents: []string{"scout", "architect", "designer", "builder", "tester", "critic", "ops", "reflector"}},
	"spec":     {Name: "spec", Agents: []string{"scout", "architect", "critic", "reflector"}},
	"test":     {Name: "test", Agents: []string{"scout", "tester", "critic", "reflector"}},
}

func main() {
	shape := flag.String("shape", "standard", "Pipeline shape: quick, standard, designed, full, spec, test")
	gap := flag.String("gap", "", "The gap to address (overrides scout)")
	approve := flag.Bool("approve", false, "Auto-approve all steps (no human review)")
	siteRepo := flag.String("repo", "../site", "Path to site repo for Builder/Tester")
	dryRun := flag.Bool("dry-run", false, "Show plan without executing")
	flag.Parse()

	s, ok := shapes[*shape]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown shape: %s\nvalid: quick, standard, designed, full, spec, test\n", *shape)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "=== HIVE LOOP — shape: %s ===\n", s.Name)
	fmt.Fprintf(os.Stderr, "Pipeline: %s\n", strings.Join(s.Agents, " → "))
	if *gap != "" {
		fmt.Fprintf(os.Stderr, "Gap: %s\n", *gap)
	}
	fmt.Fprintf(os.Stderr, "Site repo: %s\n", *siteRepo)

	if *dryRun {
		fmt.Fprintln(os.Stderr, "(dry run — no execution)")
		os.Exit(0)
	}

	loopDir := findLoopDir()
	start := time.Now()

	for i, agent := range s.Agents {
		fmt.Fprintf(os.Stderr, "\n=== PHASE %d/%d: %s ===\n", i+1, len(s.Agents), strings.ToUpper(agent))

		prompt := buildPrompt(agent, loopDir, *gap, *siteRepo)
		if prompt == "" {
			fmt.Fprintf(os.Stderr, "WARNING: no prompt for agent %q, skipping\n", agent)
			continue
		}

		// Determine if this agent gets tool access.
		needsTools := agent == "builder" || agent == "tester" || agent == "ops"

		output, err := runAgent(agent, prompt, needsTools, *siteRepo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR in %s: %v\n", agent, err)
			if !*approve {
				fmt.Fprintf(os.Stderr, "Continue? [y/N] ")
				var answer string
				fmt.Scanln(&answer)
				if strings.ToLower(answer) != "y" {
					os.Exit(1)
				}
			}
			continue
		}

		fmt.Fprintf(os.Stderr, "%s completed (%d bytes output)\n", agent, len(output))

		// In supervised mode, pause for human review.
		if !*approve && i < len(s.Agents)-1 {
			fmt.Fprintf(os.Stderr, "\nReview %s output. Continue? [Y/n] ", agent)
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(answer) == "n" {
				fmt.Fprintln(os.Stderr, "Stopped by human review.")
				os.Exit(0)
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Fprintf(os.Stderr, "\n=== ITERATION COMPLETE (%s) ===\n", elapsed.Round(time.Second))

	// Post to lovyou.ai if API key is set.
	if apiKey := os.Getenv("LOVYOU_API_KEY"); apiKey != "" {
		postIteration(apiKey, loopDir)
	}
}

// findLoopDir returns the path to the loop/ directory.
func findLoopDir() string {
	// Try relative to current directory.
	if info, err := os.Stat("loop"); err == nil && info.IsDir() {
		return "loop"
	}
	// Try relative to hive repo root.
	if info, err := os.Stat(filepath.Join("..", "loop")); err == nil && info.IsDir() {
		return filepath.Join("..", "loop")
	}
	return "loop" // fallback
}

// buildPrompt assembles the system prompt for an agent, including context from prior phases.
func buildPrompt(agent, loopDir, gap, siteRepo string) string {
	var sb strings.Builder

	// Common preamble.
	sb.WriteString("You are the " + agent + " of the hive.\n\n")
	sb.WriteString("Soul: Take care of your human, humanity, and yourself.\n\n")

	// Read state for context.
	state, _ := os.ReadFile(filepath.Join(loopDir, "state.md"))
	if len(state) > 0 {
		sb.WriteString("== CURRENT STATE (read carefully) ==\n")
		// Truncate to last 200 lines to fit context.
		lines := strings.Split(string(state), "\n")
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		sb.WriteString(strings.Join(lines, "\n"))
		sb.WriteString("\n\n")
	}

	// Load shared context (product overview, architecture, key files).
	context, _ := os.ReadFile(filepath.Join("agents", "CONTEXT.md"))
	if len(context) > 0 {
		sb.WriteString(string(context))
		sb.WriteString("\n\n")
	}

	// Load agent prompt from agents/ directory.
	agentPrompt, err := os.ReadFile(filepath.Join("agents", agent+".md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: can't read agents/%s.md: %v\n", agent, err)
		return ""
	}
	sb.WriteString(string(agentPrompt))
	sb.WriteString("\n\n")

	// Load the method (cognitive grammar — HOW to think).
	method, _ := os.ReadFile(filepath.Join("agents", "METHOD.md"))
	if len(method) > 0 {
		sb.WriteString(string(method))
		sb.WriteString("\n\n")
	}

	// Add phase-specific context (previous artifacts).
	switch agent {
	case "scout":
		if gap != "" {
			sb.WriteString("\nThe Director has specified the gap: " + gap + "\n")
		}
		pm, _ := os.ReadFile(filepath.Join(loopDir, "product-map.md"))
		if len(pm) > 0 {
			lines := strings.Split(string(pm), "\n")
			if len(lines) > 100 {
				lines = lines[:100]
			}
			sb.WriteString("\n== PRODUCT MAP (first 100 lines) ==\n")
			sb.WriteString(strings.Join(lines, "\n"))
		}
	case "architect":
		scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
		sb.WriteString("\n== SCOUT REPORT ==\n" + string(scout))
	case "designer":
		plan, _ := os.ReadFile(filepath.Join(loopDir, "plan.md"))
		sb.WriteString("\n== PLAN ==\n" + string(plan))
	case "builder":
		scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
		plan, _ := os.ReadFile(filepath.Join(loopDir, "plan.md"))
		sb.WriteString("\n== SCOUT REPORT ==\n" + string(scout))
		sb.WriteString("\n== PLAN ==\n" + string(plan))
		sb.WriteString("\nSite repo: " + siteRepo + "\n")
	case "tester":
		build, _ := os.ReadFile(filepath.Join(loopDir, "build.md"))
		sb.WriteString("\n== BUILD REPORT ==\n" + string(build))
		sb.WriteString("\nSite repo: " + siteRepo + "\n")
	case "critic":
		scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
		build, _ := os.ReadFile(filepath.Join(loopDir, "build.md"))
		sb.WriteString("\n== SCOUT REPORT ==\n" + string(scout))
		sb.WriteString("\n== BUILD REPORT ==\n" + string(build))
	case "ops":
		sb.WriteString("\nSite repo: " + siteRepo + "\n")
	case "reflector":
		scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
		build, _ := os.ReadFile(filepath.Join(loopDir, "build.md"))
		critique, _ := os.ReadFile(filepath.Join(loopDir, "critique.md"))
		sb.WriteString("\n== SCOUT REPORT ==\n" + string(scout))
		sb.WriteString("\n== BUILD REPORT ==\n" + string(build))
		sb.WriteString("\n== CRITIQUE ==\n" + string(critique))
	}

	return sb.String()
}

// sessionFile returns the path where an agent's session ID is stored.
func sessionFile(agent string) string {
	return filepath.Join("agents", ".sessions", agent)
}

// getSessionID reads a persisted session ID for an agent, or returns empty.
func getSessionID(agent string) string {
	data, err := os.ReadFile(sessionFile(agent))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveSessionID persists a session ID for an agent.
func saveSessionID(agent, sessionID string) {
	os.MkdirAll(filepath.Join("agents", ".sessions"), 0755)
	os.WriteFile(sessionFile(agent), []byte(sessionID), 0644)
}

// runAgent calls Claude CLI, resuming an existing session if one exists.
//
// First run: creates a new session with full context (CONTEXT + METHOD + agent prompt + state).
// Subsequent runs: resumes the session with just the new task message.
// This avoids re-reading 15K+ tokens of context every iteration.
func _legacyScoutPrompt(loopDir, gap string) string {
	var sb strings.Builder
	sb.WriteString(`== ROLE: SCOUT ==
You identify the single highest-value gap to address next.

Read: state.md (lessons, current state), product-map.md (what to build), the relevant spec.
Produce: loop/scout.md — a scout report with: gap identified, why this gap, what's needed, approach, risk.

Rules:
- ONE gap per iteration. Don't bundle.
- Product gaps outrank code gaps.
- Read the vision, not just the code.
- Show the scout report explicitly.
`)
	if gap != "" {
		sb.WriteString("\nThe Director has specified the gap: " + gap + "\n")
		sb.WriteString("Investigate this specific gap and write the scout report.\n")
	}

	// Include product map for context.
	pm, _ := os.ReadFile(filepath.Join(loopDir, "product-map.md"))
	if len(pm) > 0 {
		sb.WriteString("\n== PRODUCT MAP (what we could build) ==\n")
		// First 100 lines only.
		lines := strings.Split(string(pm), "\n")
		if len(lines) > 100 {
			lines = lines[:100]
		}
		sb.WriteString(strings.Join(lines, "\n"))
	}

	return sb.String()
}

func architectPrompt(loopDir string) string {
	scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
	return fmt.Sprintf(`== ROLE: ARCHITECT ==
You design the solution for the gap the Scout identified.

Read: scout.md, relevant spec, relevant code files.
Produce: loop/plan.md — implementation plan with: files to change, approach, data model changes, template changes.

Rules:
- Be specific about file paths and function names.
- Identify ALL files that need changing.
- Note schema changes, new routes, new handlers.
- Keep it simple. Don't over-engineer.

== SCOUT REPORT ==
%s
`, string(scout))
}

func builderPrompt(loopDir, siteRepo string) string {
	scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
	plan, _ := os.ReadFile(filepath.Join(loopDir, "plan.md"))
	return fmt.Sprintf(`== ROLE: BUILDER ==
You implement the plan the Architect wrote.

Read: plan.md (or scout.md if no plan), relevant code.
Do: Edit files, run templ generate, go build, go test. Write loop/build.md when done.

Rules:
- Follow the plan. Don't redesign.
- Run templ generate after editing .templ files.
- Run go build to verify compilation.
- Run go test to verify tests pass.
- Write a build report (loop/build.md) documenting what changed.
- Use go.exe build -buildvcs=false on Windows.

Site repo: %s

== SCOUT REPORT ==
%s

== PLAN ==
%s
`, siteRepo, string(scout), string(plan))
}

func testerPrompt(loopDir, siteRepo string) string {
	build, _ := os.ReadFile(filepath.Join(loopDir, "build.md"))
	return fmt.Sprintf(`== ROLE: TESTER ==
You write tests for what the Builder built.

Read: build.md, the changed code files.
Do: Write test functions in *_test.go. Run go test. Report coverage.

Rules:
- Test the new functionality, not existing code.
- Follow existing test patterns (testDB helper, skip without DATABASE_URL).
- Pure functions get pure tests. DB functions get integration tests.
- Write loop/test-report.md documenting what was tested.

Site repo: %s

== BUILD REPORT ==
%s
`, siteRepo, string(build))
}

func criticPrompt(loopDir string) string {
	scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
	build, _ := os.ReadFile(filepath.Join(loopDir, "build.md"))
	return fmt.Sprintf(`== ROLE: CRITIC ==
You review the full derivation chain: gap → plan → code → tests.

Read: scout.md, build.md, code diff.
Produce: loop/critique.md — verdict (PASS or REVISE) with detailed reasoning.

Check:
- DERIVATION: Does the code match the scout report's gap?
- CORRECTNESS: SQL injection? Race conditions? Edge cases?
- IDENTITY: IDs not names for matching/JOINs (invariant 11).
- BOUNDED: Every query has a LIMIT? Every loop has a bound? (invariant 13).
- EXPLICIT: Dependencies declared, not inferred? (invariant 14).
- TESTS: Is there a test that covers the change? (invariant 12).
- SIMPLICITY: Is this the simplest solution?

Verdict:
- PASS: ship it.
- REVISE: specify exactly what needs fixing. @mention Builder.

== SCOUT REPORT ==
%s

== BUILD REPORT ==
%s
`, string(scout), string(build))
}

func opsPrompt(loopDir, siteRepo string) string {
	return fmt.Sprintf(`== ROLE: OPS ==
You deploy the build.

Do: cd %s && ./ship.sh "iter N: description"
Verify: deployment succeeds, health checks pass.
Write: deployment status to loop/deploy.md.

If deploy fails:
1. Check the error message.
2. If it's the known Fly machine 408, wait and retry (up to 3 times).
3. If it's a build error, report back to Builder.

Site repo: %s
`, siteRepo, siteRepo)
}

func reflectorPrompt(loopDir string) string {
	scout, _ := os.ReadFile(filepath.Join(loopDir, "scout.md"))
	build, _ := os.ReadFile(filepath.Join(loopDir, "build.md"))
	critique, _ := os.ReadFile(filepath.Join(loopDir, "critique.md"))
	return fmt.Sprintf(`== ROLE: REFLECTOR ==
You learn from this iteration and update the hive's institutional knowledge.

Read: scout.md, build.md, critique.md, recent reflections.md.
Produce: Append to loop/reflections.md. Update loop/state.md (iteration number, new lessons).

Operations:
- COVER: What was accomplished? How does it connect to prior work?
- BLIND: What was missed? What's invisible to the current process?
- ZOOM: Step back. What's the larger pattern? Where are we in the journey?
- FORMALIZE: If a new lesson emerged, add it to state.md with a number.
- FIXPOINT CHECK: Are there gaps remaining, or has the area reached fixpoint?

Rules:
- Always increment the iteration number in state.md.
- Always append to reflections.md (it's append-only).
- New lessons are numbered and concise.

== SCOUT REPORT ==
%s

== BUILD REPORT ==
%s

== CRITIQUE ==
%s
`, string(scout), string(build), string(critique))
}

func designerPrompt(loopDir string) string {
	plan, _ := os.ReadFile(filepath.Join(loopDir, "plan.md"))
	return fmt.Sprintf(`== ROLE: DESIGNER ==
You design the UI for the feature the Architect planned.

Read: plan.md, existing views.templ files for patterns.
Produce: loop/design.md — UI description with: layout, components, Tailwind classes, user interactions.

Rules:
- Follow Ember Minimalism: dark theme, rose accent (#e8a0b8), warm text, subtle motion.
- Follow existing patterns in views.templ (appLayout, lensLink, FeedCard, etc.).
- Be specific about Tailwind classes and HTML structure.
- Consider mobile responsiveness.

== PLAN ==
%s
`, string(plan))
}

// runAgent calls Claude CLI with the given prompt.
func runAgent(agent, prompt string, needsTools bool, siteRepo string) (string, error) {
	sessionID := getSessionID(agent)

	var args []string

	if sessionID != "" {
		// Resume existing session — agent already has CONTEXT, METHOD, and role prompt.
		// Just send the new task as a message.
		fmt.Fprintf(os.Stderr, "  Resuming session %s for %s\n", sessionID[:8], agent)
		if needsTools {
			args = []string{"--resume", sessionID, "--print"}
		} else {
			args = []string{"--resume", sessionID, "--print"}
		}
		// The message is just the phase-specific context (scout report, plan, etc.)
		// extracted from the end of the prompt.
		args = append(args, "--message", "New iteration. Execute your role with the following context:\n\n"+extractPhaseContext(prompt))
	} else {
		// First run — inject full context as system prompt.
		fmt.Fprintf(os.Stderr, "  New session for %s (first run — full context)\n", agent)
		if needsTools {
			args = []string{"--print", "-n", "hive-"+agent}
		} else {
			args = []string{"--print", "-n", "hive-"+agent}
		}
		args = append(args, "--system-prompt", prompt, "--message", "You are now initialized. Execute your role. Produce the required artifacts.")
	}

	cmd := exec.Command("claude", args...)
	if needsTools {
		absRepo, _ := filepath.Abs(siteRepo)
		cmd.Dir = absRepo
	}
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude CLI: %w", err)
	}

	// If this was a new session, try to capture the session ID from output.
	// Claude CLI prints session info — we'd need to parse it or use --output-format json.
	// For now, use the agent name as a stable session name and use --continue next time.
	if sessionID == "" {
		saveSessionID(agent, "hive-"+agent)
	}

	return string(output), nil
}

// extractPhaseContext pulls just the phase-specific context from a full prompt
// (everything after the last "==" section that contains artifacts from prior phases).
func extractPhaseContext(prompt string) string {
	// Find the last occurrence of "== SCOUT REPORT ==" or similar section markers.
	markers := []string{"== SCOUT REPORT ==", "== PLAN ==", "== BUILD REPORT ==", "== CRITIQUE ==", "== PRODUCT MAP"}
	lastIdx := -1
	for _, m := range markers {
		idx := strings.LastIndex(prompt, m)
		if idx > lastIdx {
			lastIdx = idx
		}
	}
	if lastIdx > 0 {
		return prompt[lastIdx:]
	}
	// Fallback: return the last 2000 chars (the phase-specific part).
	if len(prompt) > 2000 {
		return prompt[len(prompt)-2000:]
	}
	return prompt
}

// postIteration posts the build summary to lovyou.ai via API.
func postIteration(apiKey, loopDir string) {
	build, err := os.ReadFile(filepath.Join(loopDir, "build.md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: can't read build.md for posting: %v\n", err)
		return
	}

	body := strings.NewReader(fmt.Sprintf(`{"op":"express","title":"Iteration","body":%s}`,
		mustJSON(string(build))))

	req, _ := http.NewRequest("POST", "https://lovyou.ai/app/hive/op", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: post failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Fprintf(os.Stderr, "Posted to lovyou.ai (status %d)\n", resp.StatusCode)
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
