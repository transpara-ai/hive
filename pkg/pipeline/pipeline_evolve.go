package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/work"
	"github.com/lovyou-ai/hive/pkg/workspace"
)

// maxEvolveIterations is the maximum number of evolution steps per session.
const maxEvolveIterations = 5

// evolveIterationTimeout caps how long a single evolve iteration can take.
const evolveIterationTimeout = 20 * time.Minute

// EvolveRecommendation is the CTO's structured response for evolution.
type EvolveRecommendation struct {
	Description    string   `json:"description"`
	FilesToChange  []string `json:"files_to_change"`
	NewFiles       []string `json:"new_files"`
	ExpectedImpact string   `json:"expected_impact"`
	Priority       string   `json:"priority"`
	Category       string   `json:"category"` // "feature", "architecture", "capability", "infrastructure"
	SkipReason     string   `json:"skip_reason"`
}

// EvolveState persists session progress so evolve can resume after interruption.
type EvolveState struct {
	StartedAt     time.Time              `json:"started_at"`
	LastIteration int                    `json:"last_iteration"`
	Completed     []EvolveRecommendation `json:"completed"`
	Failed        []EvolveRecommendation `json:"failed"`
	TotalCost     float64                `json:"total_cost"`
}

// RunEvolve enters evolution mode: the CTO reads the full codebase + roadmap
// and proposes capabilities to build — not just bugs to fix. Each iteration
// proposes and implements one feature or architectural improvement.
func (p *Pipeline) RunEvolve(ctx context.Context, input ProductInput) error {
	if input.RepoPath == "" {
		return fmt.Errorf("RunEvolve requires RepoPath")
	}

	pipelineStart := time.Now()
	totalCost := 0.0
	p.emitRunStarted("evolve", input.Description)
	defer func() {
		dur := time.Since(pipelineStart)
		count, _ := p.store.Count()
		p.emitRunCompleted("evolve", count, len(p.Agents()), dur,
			"", false, "", "", totalCost)
	}()

	// Create a worktree so we don't clobber the main checkout.
	// CleanupForIteration does git reset --hard, which is safe in a
	// disposable worktree but destructive in the developer's checkout.
	sourceRepo, err := workspace.OpenRepo(input.RepoPath)
	if err != nil {
		return fmt.Errorf("open source repo: %w", err)
	}
	worktree, err := sourceRepo.CreateWorktree("evolve")
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	defer func() {
		if rmErr := worktree.RemoveWorktree(); rmErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: worktree cleanup failed: %v\n", rmErr)
		}
		_ = sourceRepo.PruneWorktrees()
	}()
	fmt.Fprintf(os.Stderr, "Evolve worktree: %s\n", worktree.Dir)
	p.emitProgress(PhaseEvolve, "worktree created at %s", worktree.Dir)

	// Rewrite input to point at the worktree. Evolve state is still
	// stored in the original repo's .hive/ so it survives worktree removal.
	worktreeInput := input
	worktreeInput.RepoPath = worktree.Dir

	// Load or create evolve state for resume support.
	state, err := loadEvolveState(input.RepoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load evolve state: %v (starting fresh)\n", err)
		state = &EvolveState{StartedAt: pipelineStart}
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = pipelineStart
	}

	startIteration := 1
	if p.resume && state.LastIteration > 0 {
		startIteration = state.LastIteration + 1
		totalCost = state.TotalCost
		fmt.Fprintf(os.Stderr, "Resuming evolve session from iteration %d (%d completed, %d failed)\n",
			startIteration, len(state.Completed), len(state.Failed))
		p.emitProgress(PhaseEvolve, "resuming from iteration %d", startIteration)
	}

	consecutiveFailures := 0
	iterationLimit := maxEvolveIterations
	for iteration := startIteration; iteration <= iterationLimit; iteration++ {
		fmt.Fprintf(os.Stderr, "\n═══ Evolve: Iteration %d/%d ═══\n", iteration, iterationLimit)
		iterationStart := time.Now()
		p.emitPhaseStarted(PhaseEvolve, iteration)

		iterCost, rec, err := p.runEvolveIteration(ctx, iteration, worktreeInput, state)
		totalCost += iterCost

		if err != nil {
			if err == errEvolveStop {
				p.emitPhaseCompleted(PhaseEvolve, time.Since(iterationStart), iteration)
				break
			}

			// No-change iterations: the builder ran but made no commits.
			// Don't count toward failures, don't mark as completed, and give
			// back the iteration slot so the CTO can retry with a different
			// recommendation or the same one with fresh context.
			if err == errEvolveNoChanges {
				fmt.Fprintf(os.Stderr, "Evolve iteration %d: builder made no changes — retrying (does not count as failure)\n", iteration)
				p.emitWarning(PhaseEvolve, "iteration %d: builder made no changes — retrying", iteration)
				p.emitPhaseCompleted(PhaseEvolve, time.Since(iterationStart), iteration)
				// Track as failed so CTO avoids repeating it, but don't
				// increment consecutiveFailures or consume the iteration slot.
				if rec != nil {
					state.Failed = append(state.Failed, *rec)
				}
				state.TotalCost = totalCost
				if saveErr := saveEvolveState(input.RepoPath, state); saveErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not save evolve state: %v\n", saveErr)
				}
				if iterationLimit < maxEvolveIterations+3 {
					iterationLimit++ // give back the slot (cap at 3 extra to prevent infinite loops)
				}
				continue
			}

			// Track failed recommendation in state.
			if rec != nil {
				state.Failed = append(state.Failed, *rec)
			}

			consecutiveFailures++
			p.emitPhaseCompleted(PhaseEvolve, time.Since(iterationStart), iteration)

			// Self-heal: run one self-improve iteration before giving up.
			fmt.Fprintf(os.Stderr, "Evolve iteration %d failed: %v — attempting self-heal...\n", iteration, err)
			p.emitProgress(PhaseEvolve, "iteration %d failed — attempting self-heal", iteration)
			healCost := p.runEvolveSelfHeal(ctx, worktreeInput)
			totalCost += healCost

			state.LastIteration = iteration
			state.TotalCost = totalCost
			if saveErr := saveEvolveState(input.RepoPath, state); saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not save evolve state: %v\n", saveErr)
			}

			if consecutiveFailures >= maxConsecutiveFailures {
				return fmt.Errorf("evolve iteration %d: %w (aborting after %d consecutive failures)", iteration, err, consecutiveFailures)
			}
			fmt.Fprintf(os.Stderr, "Warning: iteration %d failed (%v) — skipping to next\n", iteration, err)
			p.emitWarning(PhaseEvolve, "iteration %d failed (%v) — skipping to next", iteration, err)
			continue
		}

		consecutiveFailures = 0
		if rec != nil {
			state.Completed = append(state.Completed, *rec)
		}
		state.LastIteration = iteration
		state.TotalCost = totalCost
		if saveErr := saveEvolveState(input.RepoPath, state); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save evolve state: %v\n", saveErr)
		}

		p.emitPhaseCompleted(PhaseEvolve, time.Since(iterationStart), iteration)
		fmt.Fprintf(os.Stderr, "═══ Evolve: Iteration %d complete ═══\n", iteration)
	}

	// Clear state on successful session completion.
	if err := clearEvolveState(input.RepoPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not clear evolve state: %v\n", err)
	}

	fmt.Fprintln(os.Stderr, "\n═══ Evolve: Session Complete ═══")
	return nil
}

var errEvolveStop = fmt.Errorf("CTO says nothing worth building")
var errEvolveNoChanges = fmt.Errorf("builder made no changes")

func (p *Pipeline) runEvolveIteration(parentCtx context.Context, iteration int, input ProductInput, state *EvolveState) (float64, *EvolveRecommendation, error) {
	ctx, cancel := context.WithTimeout(parentCtx, evolveIterationTimeout)
	defer cancel()

	// Clean up from previous iteration.
	product, err := workspace.OpenRepo(input.RepoPath)
	if err != nil {
		return 0, nil, fmt.Errorf("open repo: %w", err)
	}
	if err := product.CleanupForIteration(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cleanup failed: %v (continuing anyway)\n", err)
		p.emitWarning(PhaseEvolve, "cleanup failed: %v (continuing anyway)", err)
	}

	// Read full codebase — no truncation limits for evolve mode.
	existingFiles, err := product.ReadSourceFiles()
	if err != nil {
		return 0, nil, fmt.Errorf("read source files: %w", err)
	}
	goFiles := filterEvolveFiles(existingFiles)

	// Build full codebase context — evolve CTO sees everything.
	var codeContext strings.Builder
	codeContext.WriteString("FULL CODEBASE:\n\n")
	for path, content := range goFiles {
		codeContext.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, content))
	}

	// Read telemetry for operational context.
	telemetryResults, err := ReadTelemetry(input.RepoPath)
	if err != nil {
		return 0, nil, fmt.Errorf("read telemetry: %w", err)
	}
	telemetrySummary := summarizeTelemetry(telemetryResults)

	// CTO analysis — fresh provider per iteration.
	fmt.Fprintln(os.Stderr, "CTO analyzing codebase for evolution opportunities...")
	p.emitProgress(PhaseEvolve, "CTO analyzing codebase for evolution opportunities")
	model := p.evolveCTOModel()
	rawProvider, err := p.providerForRoleWithModel(roles.RoleCTO, model)
	if err != nil {
		return 0, nil, fmt.Errorf("CTO provider: %w", err)
	}
	ctoTracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleCTO] = ctoTracker
	fmt.Fprintf(os.Stderr, "  ↳ evolve CTO analysis using %s\n", model)
	p.emitProgress(PhaseEvolve, "evolve CTO analysis using %s", model)

	// List existing tasks so the CTO avoids re-proposing completed work.
	ts := work.NewTaskStore(p.store, p.factory, p.signer)
	existingTasksStr := ""
	if existingTasks, listErr := ts.ListOpen(); listErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: list tasks failed: %v (continuing)\n", listErr)
		p.emitWarning(PhaseEvolve, "list tasks failed: %v", listErr)
	} else {
		statusMap := make(map[types.EventID]work.TaskStatus, len(existingTasks))
		for _, t := range existingTasks {
			if status, sErr := ts.GetStatus(t.ID); sErr == nil {
				statusMap[t.ID] = status
			}
		}
		existingTasksStr = formatTaskList(existingTasks, statusMap)
	}

	ctoPrompt := buildEvolvePrompt(codeContext.String(), telemetrySummary, input.Description, state, existingTasksStr)

	ctoStart := time.Now()
	ctoResp, err := ctoTracker.Reason(ctx, ctoPrompt, nil)
	ctoCost := ctoTracker.Snapshot().CostUSD
	if err != nil {
		return ctoCost, nil, fmt.Errorf("CTO evolve analysis: %w", err)
	}

	// Parse recommendation.
	rec, err := parseEvolveRecommendation(ctoResp.Content())
	if err != nil {
		return ctoCost, nil, fmt.Errorf("parse CTO recommendation: %w", err)
	}

	fmt.Fprintf(os.Stderr, "CTO recommendation [%s] (priority=%s): %s\n", rec.Category, rec.Priority, rec.Description)
	p.emitOutput("cto", "recommendation", fmt.Sprintf("[%s] priority=%s: %s", rec.Category, rec.Priority, rec.Description))
	if rec.SkipReason != "" {
		fmt.Fprintf(os.Stderr, "CTO says nothing worth building: %s\n", rec.SkipReason)
		p.emitOutput("cto", "recommendation", fmt.Sprintf("nothing worth building: %s", rec.SkipReason))
		return ctoCost, &rec, errEvolveStop
	}
	if rec.Description == "" {
		fmt.Fprintln(os.Stderr, "CTO returned empty recommendation — stopping.")
		p.emitOutput("cto", "recommendation", "empty recommendation — stopping")
		return ctoCost, &rec, errEvolveStop
	}

	fmt.Fprintf(os.Stderr, "Expected impact: %s\n", rec.ExpectedImpact)
	p.emitOutput("cto", "analysis", fmt.Sprintf("expected impact: %s", rec.ExpectedImpact))
	if len(rec.FilesToChange) > 0 {
		fmt.Fprintf(os.Stderr, "Files to change: %v\n", rec.FilesToChange)
		p.emitOutput("cto", "analysis", fmt.Sprintf("files to change: %v", rec.FilesToChange))
	}
	if len(rec.NewFiles) > 0 {
		fmt.Fprintf(os.Stderr, "New files: %v\n", rec.NewFiles)
		p.emitOutput("cto", "analysis", fmt.Sprintf("new files: %v", rec.NewFiles))
	}

	// Run targeted pipeline with the recommendation.
	allFiles := append(rec.FilesToChange, rec.NewFiles...)
	targetedInput := ProductInput{
		RepoPath:    input.RepoPath,
		Description: rec.Description,
		CTOAnalysis: fmt.Sprintf("Description: %s\nFILES_TO_CHANGE:\n%s\nExpected impact: %s",
			rec.Description, strings.Join(allFiles, "\n"), rec.ExpectedImpact),
	}
	if s := ctoTracker.Snapshot(); s.Iterations > 0 {
		p.telemetry = &PipelineResult{}
		p.telemetry.TokenUsage = append(p.telemetry.TokenUsage, RoleTokenUsage{
			Role:             "cto_evolve",
			Model:            ctoTracker.Model(),
			InputTokens:      s.InputTokens,
			OutputTokens:     s.OutputTokens,
			TotalTokens:      s.TokensUsed,
			CacheReadTokens:  s.CacheReadTokens,
			CacheWriteTokens: s.CacheWriteTokens,
			CostUSD:          s.CostUSD,
		})
		p.telemetry.addPhaseTiming("CTO Evolve Analysis", time.Since(ctoStart))
	}

	// Evolve mode keeps Guardian active — features need integrity checks.
	// But skip reviewer for now — the CTO + tests provide sufficient quality gate.
	prevSkipReviewer := p.skipReviewer
	p.skipReviewer = true
	defer func() { p.skipReviewer = prevSkipReviewer }()

	// Wire work task — best-effort, log warning and continue on any error.
	var workTask *work.Task
	if head, headErr := p.store.Head(); headErr == nil && head.IsSome() {
		causes := []types.EventID{head.Unwrap().ID()}
		task, createErr := ts.Create(p.humanID, rec.Description, rec.ExpectedImpact, causes, p.convID)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: work task create failed: %v (continuing)\n", createErr)
			p.emitWarning(PhaseEvolve, "work task create failed: %v", createErr)
		} else {
			workTask = &task
			assignee := p.humanID
			if builder, ok := p.agents[roles.RoleBuilder]; ok {
				assignee = builder.Runtime.ID()
			}
			if assignErr := ts.Assign(p.humanID, task.ID, assignee, []types.EventID{task.ID}, p.convID); assignErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: work task assign failed: %v (continuing)\n", assignErr)
				p.emitWarning(PhaseEvolve, "work task assign failed: %v", assignErr)
			}
		}
	} else if headErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: work task store head failed: %v (continuing)\n", headErr)
		p.emitWarning(PhaseEvolve, "work task store head failed: %v", headErr)
	}

	fmt.Fprintf(os.Stderr, "\n═══ Evolve: Running targeted pipeline ═══\n")
	if err := p.RunTargeted(ctx, targetedInput); err != nil {
		iterCost := ctoCost
		for _, t := range p.trackers {
			iterCost += t.Snapshot().CostUSD
		}
		return iterCost, &rec, fmt.Errorf("targeted pipeline: %w", err)
	}

	// Detect when the builder ran but made no changes. This is different from
	// a real failure — the builder judged the work was already done (often
	// incorrectly). Return a specific error so the evolve loop can handle it
	// without counting it as a success or a real failure.
	if p.telemetry != nil && p.telemetry.NoChanges {
		iterCost := ctoCost
		for _, t := range p.trackers {
			iterCost += t.Snapshot().CostUSD
		}
		return iterCost, &rec, errEvolveNoChanges
	}

	// Mark work task complete on success — best-effort.
	if workTask != nil {
		completer := p.humanID
		if builder, ok := p.agents[roles.RoleBuilder]; ok {
			completer = builder.Runtime.ID()
		}
		if completeErr := ts.Complete(completer, workTask.ID, rec.Description, []types.EventID{workTask.ID}, p.convID); completeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: work task complete failed: %v (continuing)\n", completeErr)
			p.emitWarning(PhaseEvolve, "work task complete failed: %v", completeErr)
		}
	}

	iterCost := ctoCost
	for _, t := range p.trackers {
		iterCost += t.Snapshot().CostUSD
	}

	if err := product.SyncMain(); err != nil {
		return iterCost, &rec, fmt.Errorf("sync main: %w", err)
	}

	return iterCost, &rec, nil
}

// runEvolveSelfHeal runs one self-improve iteration to fix issues that caused
// the evolve iteration to fail. Returns the cost of the self-heal attempt.
func (p *Pipeline) runEvolveSelfHeal(ctx context.Context, input ProductInput) float64 {
	healInput := ProductInput{
		RepoPath:    input.RepoPath,
		Description: "Fix compilation or test failures from the most recent evolve iteration",
	}

	prevSkipReviewer := p.skipReviewer
	p.skipReviewer = true
	defer func() { p.skipReviewer = prevSkipReviewer }()

	if _, err := p.runSelfImproveIteration(ctx, 1, healInput); err != nil {
		fmt.Fprintf(os.Stderr, "Self-heal failed: %v (continuing with next evolve iteration)\n", err)
		p.emitWarning(PhaseEvolve, "self-heal failed: %v", err)
	} else {
		fmt.Fprintln(os.Stderr, "Self-heal completed successfully")
		p.emitProgress(PhaseEvolve, "self-heal completed")
	}

	cost := 0.0
	for _, t := range p.trackers {
		cost += t.Snapshot().CostUSD
	}
	return cost
}

// evolveCTOModel returns the model for evolve CTO analysis.
// Uses Sonnet by default — feature design needs strong reasoning.
func (p *Pipeline) evolveCTOModel() string {
	if p.ctoModel != "" {
		return p.ctoModel
	}
	return "claude-sonnet-4-6"
}

// filterEvolveFiles returns all Go source files (including tests) and key
// config files. Evolve mode sees everything — no truncation.
func filterEvolveFiles(files map[string]string) map[string]string {
	out := make(map[string]string, len(files))
	for p, content := range files {
		switch {
		case strings.HasSuffix(p, ".go"):
			out[p] = content
		case p == "CLAUDE.md", p == "go.mod", p == "SPEC.md", p == "README.md":
			out[p] = content
		}
	}
	return out
}

// buildEvolvePrompt constructs the CTO prompt for evolution mode.
func buildEvolvePrompt(codeContext, telemetrySummary, humanDirection string, state *EvolveState, existingTasks string) string {
	direction := ""
	if humanDirection != "" {
		direction = fmt.Sprintf(`
HUMAN DIRECTION (prioritize this):
%s
`, humanDirection)
	}

	priorWork := ""
	if state != nil && (len(state.Completed) > 0 || len(state.Failed) > 0) {
		var pw strings.Builder
		pw.WriteString("\nPRIOR WORK IN THIS SESSION (do NOT repeat these):\n")
		for _, c := range state.Completed {
			pw.WriteString(fmt.Sprintf("  ✓ COMPLETED: %s\n", c.Description))
		}
		for _, f := range state.Failed {
			pw.WriteString(fmt.Sprintf("  ✗ FAILED: %s\n", f.Description))
		}
		priorWork = pw.String()
	}

	existingTasksSection := ""
	if existingTasks != "" {
		existingTasksSection = fmt.Sprintf("\nEXISTING TASKS (do NOT re-propose these — they have already been created):\n%s", existingTasks)
	}

	return fmt.Sprintf(`CRITICAL: You MUST respond with ONLY a JSON object. No prose, no explanation, no markdown, no code blocks. Just raw JSON starting with { and ending with }.

You are the CTO of a self-improving AI agent civilisation. Your job is to EVOLVE the system — build new capabilities, not just fix bugs.

The hive is a civilisation engine built on EventGraph. It builds products autonomously. The soul: "Take care of your human, humanity, and yourself."

ARCHITECTURE VISION:
- All agents share one event graph and one actor store
- Every action is signed, hash-chained, and auditable
- Trust accumulates through verified work (0.0-1.0)
- Authority model: Required / Recommended / Notification
- Guardian watches everything independently
- Eight agent rights (existence, memory, identity, communication, purpose, dignity, transparency, boundaries)
- Ten invariants (budget, causality, integrity, observable, self-evolve, dignity, transparent, consent, margin, reserve)

THE THIRTEEN PRODUCTS (build order):
1. Work Graph — task management with agent collaboration (BUILD FIRST — the hive needs it)
2. Market Graph — portable reputation, no platform rent
3. Social Graph — user-owned social, community self-governance
4. Justice Graph — dispute resolution, precedent, due process
5. Build Graph — accountable software development
6. Knowledge Graph — claim provenance, open access research
7. Alignment Graph — AI accountability for regulators
8. Identity Graph — user-owned identity, trust accumulation
9-13: Bond, Belonging, Meaning, Evolution, Being

CURRENT PIPELINE MODES:
- Full (greenfield): Research → Design → Simplify → Build → Review → Test → Integrate
- Targeted (existing code): Context → Understand → Modify → Review → Test → PR
- Self-improve: analyze telemetry, fix bugs
- Evolve (THIS MODE): build new capabilities
- Agentic loop: concurrent self-directing agents

RECENT TELEMETRY:
%s
%s
%s
%s
%s
Analyze the FULL codebase above. Identify the single most valuable capability to build next.

PRIORITY ORDER:
1. Capabilities the hive needs to function better (better error recovery, richer event graph usage, smarter CTO prompts, etc.)
2. Infrastructure for the first product (Work Graph primitives, task management on the event graph)
3. Operational improvements (better monitoring, richer telemetry, smarter model selection)
4. Missing architectural pieces (agent communication channels, trust model improvements)
5. Developer experience (better CLI output, debugging tools)

CONSTRAINTS:
- Each recommendation must be implementable in ONE targeted pipeline run (a few files)
- The change must compile and pass tests
- Be ambitious but practical — propose real features, not cosmetic changes
- Do NOT recommend changes already implemented — read the code carefully
- Do NOT recommend token/cost optimizations — 20x Max plan has unlimited tokens

Respond with ONLY a JSON object:
{"description": "what to build, 2-3 sentences with enough detail for a builder", "files_to_change": ["existing/files"], "new_files": ["new/files/to/create"], "expected_impact": "1-2 sentences", "priority": "high|medium|low", "category": "feature|architecture|capability|infrastructure", "skip_reason": "if nothing worth building, explain why; otherwise empty string"}

No preamble, no explanation, no code blocks, no markdown. ONLY the JSON object.`, codeContext, telemetrySummary, direction, priorWork, existingTasksSection)
}

// parseEvolveRecommendation extracts an EvolveRecommendation from LLM output.
// Falls back to prose parsing if the CTO returns text instead of JSON.
func parseEvolveRecommendation(response string) (EvolveRecommendation, error) {
	var rec EvolveRecommendation

	jsonStr := extractJSONBlock(response)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &rec); err == nil {
			return rec, nil
		}
	}

	// Fallback: try to extract useful info from prose response.
	rec = parseEvolveProse(response)
	if rec.Description != "" {
		return rec, nil
	}

	// Nothing parseable — treat entire response as skip reason.
	return EvolveRecommendation{SkipReason: response}, nil
}

// parseEvolveProse extracts an EvolveRecommendation from a prose CTO response.
// Looks for file paths (pkg/... and cmd/...) and uses the first paragraph as description.
var evolveFilePathRe = regexp.MustCompile(`(?:pkg|cmd)/[\w/._-]+\.go`)

func parseEvolveProse(response string) EvolveRecommendation {
	// Extract file paths.
	matches := evolveFilePathRe.FindAllString(response, -1)
	seen := make(map[string]bool, len(matches))
	var files []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			files = append(files, m)
		}
	}

	if len(files) == 0 {
		return EvolveRecommendation{}
	}

	// Use first non-empty paragraph as description.
	desc := ""
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "FILES") && !strings.HasPrefix(line, "---") {
			desc = line
			break
		}
	}
	if desc == "" {
		desc = "CTO recommendation (parsed from prose)"
	}

	return EvolveRecommendation{
		Description:   desc,
		FilesToChange: files,
		Priority:      "high",
		Category:      "capability",
	}
}

// evolveStatePath returns the path to the evolve state file.
func evolveStatePath(repoPath string) string {
	return filepath.Join(repoPath, ".hive", "evolve-state.json")
}

// loadEvolveState loads saved evolve session state from disk.
func loadEvolveState(repoPath string) (*EvolveState, error) {
	data, err := os.ReadFile(evolveStatePath(repoPath))
	if err != nil {
		if os.IsNotExist(err) {
			return &EvolveState{}, nil
		}
		return nil, err
	}
	var state EvolveState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse evolve state: %w", err)
	}
	return &state, nil
}

// saveEvolveState persists evolve session state to disk.
func saveEvolveState(repoPath string, state *EvolveState) error {
	dir := filepath.Dir(evolveStatePath(repoPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(evolveStatePath(repoPath), data, 0o644)
}

// clearEvolveState removes the evolve state file after a successful session.
func clearEvolveState(repoPath string) error {
	err := os.Remove(evolveStatePath(repoPath))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
