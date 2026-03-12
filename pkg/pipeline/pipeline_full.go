package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/decision"

	"github.com/lovyou-ai/hive/pkg/loop"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
)

// Run executes the full product pipeline for a given input.
func (p *Pipeline) Run(ctx context.Context, input ProductInput) error {
	pipelineStart := time.Now()
	p.telemetry = &PipelineResult{
		Mode:             "full",
		InputDescription: input.Description,
		StartedAt:        pipelineStart,
	}
	defer func() {
		p.telemetry.collectTokenUsage(p.trackers)
		writeTelemetry(p.telemetryBaseDir(), p.telemetry)
		p.telemetry = nil
	}()

	// ── Phase 1: Research ──
	fmt.Println("═══ Phase 1: Research ═══")
	phaseStart := time.Now()
	spec, ctoEval, err := p.research(ctx, input)
	if err != nil {
		return p.failPhase("Research", fmt.Errorf("research: %w", err))
	}
	p.telemetry.addPhaseTiming("Research", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "research"); halt {
		return p.failPhase("Research", fmt.Errorf("guardian halted pipeline after research phase"))
	}

	// Extract product name from CTO evaluation or use provided name.
	name := input.Name
	if name == "" {
		name = extractName(ctoEval)
	}

	// Initialize product repo
	product, err := p.ws.InitProduct(name)
	if err != nil {
		return p.failPhase("Research", fmt.Errorf("init product: %w", err))
	}
	p.product = product
	fmt.Printf("Product repo: %s → %s\n", product.Dir, product.Repo)

	// ── Phase 2: Design ──
	fmt.Println("═══ Phase 2: Design ═══")
	phaseStart = time.Now()
	design, err := p.design(ctx, spec)
	if err != nil {
		return p.failPhase("Design", fmt.Errorf("design: %w", err))
	}
	if halt := p.guardianCheck(ctx, "design"); halt {
		return p.failPhase("Design", fmt.Errorf("guardian halted pipeline after design phase"))
	}

	// ── Phase 2b: Simplify ──
	if !p.skipSimplify {
		fmt.Println("═══ Phase 2b: Simplify ═══")
		design, err = p.simplify(ctx, design)
		if err != nil {
			return p.failPhase("Simplify", fmt.Errorf("simplify: %w", err))
		}
	} else {
		fmt.Println("═══ Phase 2b: Simplify — SKIPPED ═══")
	}
	p.telemetry.addPhaseTiming("Design", time.Since(phaseStart))

	// Save the final spec to the product repo
	if err := p.product.WriteFile("SPEC.md", design); err != nil {
		return p.failPhase("Design", fmt.Errorf("save spec: %w", err))
	}
	if err := p.product.Commit("docs: Code Graph specification"); err != nil {
		return p.failPhase("Design", fmt.Errorf("commit spec: %w", err))
	}
	fmt.Println("Spec committed to product repo.")

	// Extract language from the design
	lang := p.extractLanguage(design)
	fmt.Printf("Target language: %s\n", lang)

	// ── Phase 3: Build ──
	fmt.Println("═══ Phase 3: Build ═══")
	phaseStart = time.Now()
	files, err := p.build(ctx, design, lang)
	if err != nil {
		return p.failPhase("Build", fmt.Errorf("build: %w", err))
	}
	p.telemetry.addPhaseTiming("Build", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "build"); halt {
		return p.failPhase("Build", fmt.Errorf("guardian halted pipeline after build phase"))
	}

	// ── Phase 4: Review → Rebuild loop ──
	phaseStart = time.Now()
	const maxReviewRounds = 3
	for round := 1; round <= maxReviewRounds; round++ {
		fmt.Printf("═══ Phase 4: Review (round %d) ═══\n", round)
		feedback, approved, err := p.review(ctx, files, design, lang)
		if err != nil {
			return p.failPhase("Review", fmt.Errorf("review round %d: %w", round, err))
		}
		p.telemetry.addReviewSignal(approved)
		if halt := p.guardianCheck(ctx, "review"); halt {
			return p.failPhase("Review", fmt.Errorf("guardian halted pipeline after review phase"))
		}

		if approved {
			fmt.Println("Code approved by reviewer.")
			break
		}

		if round == maxReviewRounds {
			fmt.Println("Max review rounds reached — proceeding with current code.")
			break
		}

		// Rebuild with reviewer feedback
		fmt.Printf("═══ Phase 4b: Rebuild from feedback (round %d) ═══\n", round)
		files, err = p.rebuild(ctx, files, feedback, design, lang)
		if err != nil {
			return p.failPhase("Review", fmt.Errorf("rebuild round %d: %w", round, err))
		}
	}
	p.telemetry.addPhaseTiming("Review", time.Since(phaseStart))

	// ── Phase 5: Test ──
	fmt.Println("═══ Phase 5: Test ═══")
	phaseStart = time.Now()
	err = p.test(ctx, files, lang)
	if err != nil {
		return p.failPhase("Test", fmt.Errorf("test: %w", err))
	}
	p.telemetry.addPhaseTiming("Test", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "test"); halt {
		return p.failPhase("Test", fmt.Errorf("guardian halted pipeline after test phase"))
	}

	// ── Phase 6: Integrate ──
	fmt.Println("═══ Phase 6: Integrate ═══")
	phaseStart = time.Now()
	err = p.integrate(ctx)
	if err != nil {
		return p.failPhase("Integrate", fmt.Errorf("integrate: %w", err))
	}
	p.telemetry.addPhaseTiming("Integrate", time.Since(phaseStart))
	if halt := p.guardianCheck(ctx, "integrate"); halt {
		return p.failPhase("Integrate", fmt.Errorf("guardian halted pipeline after integrate phase"))
	}

	fmt.Println("═══ Pipeline Complete ═══")
	p.PrintTokenSummary()
	return nil
}

// LoopConfig configures the agentic loop mode of the pipeline.
type LoopConfig struct {
	// Budget limits for each execution agent loop.
	Budget resources.BudgetConfig

	// GuardianBudget limits for the Guardian's loop. The Guardian watches
	// everything and must outlive execution agents — defaults to 10x the
	// execution budget iterations and duration.
	GuardianBudget resources.BudgetConfig

	// OnIteration is called when any agent completes a loop iteration.
	OnIteration func(role roles.Role, iteration int, response string)
}

// DefaultLoopConfig returns sensible defaults for loop mode.
func DefaultLoopConfig() LoopConfig {
	return LoopConfig{
		Budget: resources.BudgetConfig{
			MaxIterations: 20,
			MaxCostUSD:    10.0,
		},
		GuardianBudget: resources.BudgetConfig{
			MaxIterations: 200, // 10x execution agents — Guardian outlives them
			MaxCostUSD:    20.0,
		},
	}
}

// RunLoop executes the pipeline in graph-driven agentic loop mode.
//
// Instead of a fixed phase sequence, the CTO seeds the work and agents
// self-direct by observing graph events. The Guardian runs its own loop
// watching everything. Agents communicate through events, not orchestration.
func (p *Pipeline) RunLoop(ctx context.Context, input ProductInput, cfg LoopConfig) ([]loop.AgentResult, error) {
	// Create event bus for real-time notification between agents.
	eventBus := bus.NewEventBus(p.store, 256)
	defer eventBus.Close()

	// Seed: CTO evaluates the idea and emits initial direction.
	fmt.Println("═══ Seeding: CTO evaluates idea ═══")
	seedTask, err := p.seedWork(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("seed work: %w", err)
	}

	// Bootstrap agents for the product pipeline.
	agentConfigs, err := p.buildLoopConfigs(ctx, seedTask, eventBus, cfg)
	if err != nil {
		return nil, fmt.Errorf("build loop configs: %w", err)
	}

	// Run all agent loops concurrently.
	fmt.Printf("═══ Starting %d agent loops ═══\n", len(agentConfigs))
	for _, c := range agentConfigs {
		fmt.Printf("  ↳ %s loop starting\n", c.Agent.Role)
	}

	results := loop.RunConcurrent(ctx, agentConfigs)

	fmt.Println("═══ All loops stopped ═══")
	for _, ar := range results {
		b := ar.Result.Budget
		fmt.Printf("  %s (%s): stopped=%s iterations=%d tokens=%d (in=%d out=%d cache_read=%d cache_write=%d) cost=$%.4f\n",
			ar.Role, ar.Name, ar.Result.Reason, ar.Result.Iterations,
			b.TokensUsed, b.InputTokens, b.OutputTokens, b.CacheReadTokens, b.CacheWriteTokens, b.CostUSD)
	}
	printTokenSummary(results)

	return results, nil
}

// seedWork has the CTO evaluate the input and emit seed events on the graph.
// Returns a task description that other agents will observe.
func (p *Pipeline) seedWork(ctx context.Context, input ProductInput) (string, error) {
	var spec string
	if input.SpecFile != "" {
		content, err := p.ws.ReadFile(input.SpecFile)
		if err != nil {
			return "", fmt.Errorf("read spec: %w", err)
		}
		spec = content
	} else if input.URL != "" {
		spec = fmt.Sprintf("Research and build product from: %s", input.URL)
	} else if input.Description != "" {
		spec = input.Description
	}

	// CTO evaluates and seeds direction.
	// Fresh provider per call — avoids context accumulation across pipeline phases.
	rawProvider, err := p.providerForRole(roles.RoleCTO)
	if err != nil {
		return "", fmt.Errorf("CTO provider: %w", err)
	}
	ctoTracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleCTO] = ctoTracker

	ctoResp, err := ctoTracker.Reason(ctx, fmt.Sprintf(`You are seeding a product build. Evaluate this idea and emit clear direction for the team.

What needs building? What's the architecture? What should the Builder do first?
Be specific and actionable — other agents will read your events and self-direct.

Product idea:
%s`, spec), nil)
	if err != nil {
		return "", fmt.Errorf("CTO seed: %w", err)
	}
	evaluation := ctoResp.Content()

	// Record the CTO's direction as an action event.
	_, err = p.cto.Runtime.Act(ctx, ActionSeedBuild, spec)
	if err != nil {
		return "", fmt.Errorf("CTO seed action: %w", err)
	}

	fmt.Printf("CTO direction:\n%s\n", evaluation)
	return evaluation, nil
}

// buildLoopConfigs creates loop configurations for each pipeline agent.
func (p *Pipeline) buildLoopConfigs(ctx context.Context, seedTask string, eventBus bus.IBus, cfg LoopConfig) ([]loop.Config, error) {
	var configs []loop.Config

	// Builder — generates code based on CTO direction.
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, fmt.Errorf("ensure builder: %w", err)
	}
	configs = append(configs, loop.Config{
		Agent:   builder,
		HumanID: p.humanID,
		Budget:  cfg.Budget,
		Task:    fmt.Sprintf("Build the product based on this direction:\n%s", seedTask),
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleBuilder, i, resp)
			}
		},
	})

	// Reviewer — watches for build events and reviews code.
	reviewer, err := p.ensureAgent(ctx, roles.RoleReviewer, "reviewer")
	if err != nil {
		return nil, fmt.Errorf("ensure reviewer: %w", err)
	}
	configs = append(configs, loop.Config{
		Agent:   reviewer,
		HumanID: p.humanID,
		Budget:  cfg.Budget,
		Task:    "Watch for code generation events. Review code for quality, security, and spec compliance. Report issues.",
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleReviewer, i, resp)
			}
		},
	})

	// Tester — watches for build/review events and runs tests.
	tester, err := p.ensureAgent(ctx, roles.RoleTester, "tester")
	if err != nil {
		return nil, fmt.Errorf("ensure tester: %w", err)
	}
	configs = append(configs, loop.Config{
		Agent:   tester,
		HumanID: p.humanID,
		Budget:  cfg.Budget,
		Task:    "Watch for code changes. Run tests, analyze coverage, and report gaps.",
		Bus:     eventBus,
		OnIteration: func(i int, resp string) {
			if cfg.OnIteration != nil {
				cfg.OnIteration(roles.RoleTester, i, resp)
			}
		},
	})

	// Guardian — watches everything, can HALT. Gets a larger budget than
	// execution agents so it outlives them (OBSERVABLE invariant).
	if p.guardian != nil {
		guardianBudget := cfg.GuardianBudget
		if guardianBudget == (resources.BudgetConfig{}) {
			// Fallback: scale all execution budget dimensions so Guardian outlives them.
			guardianBudget = cfg.Budget
			guardianBudget.MaxIterations *= 10
			guardianBudget.MaxCostUSD *= 2
			guardianBudget.MaxDuration *= 2
		}
		configs = append(configs, loop.Config{
			Agent:   p.guardian,
			HumanID: p.humanID,
			Budget:  guardianBudget,
			Task:    "Monitor all agent activity for policy violations, trust anomalies, and authority overreach. HALT if anything looks wrong.",
			Bus:     eventBus,
			OnIteration: func(i int, resp string) {
				if cfg.OnIteration != nil {
					cfg.OnIteration(roles.RoleGuardian, i, resp)
				}
			},
		})
	}

	return configs, nil
}

// extractName pulls a product name from the CTO's evaluation response.
// Looks for "NAME: kebab-case-name" in the text. Falls back to "product".
func extractName(ctoEval string) string {
	for _, line := range strings.Split(ctoEval, "\n") {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "NAME:") {
			name := strings.TrimSpace(trimmed[len("NAME:"):])
			name = strings.ToLower(name)
			name = strings.ReplaceAll(name, " ", "-")
			var clean []byte
			for i := 0; i < len(name); i++ {
				c := name[i]
				if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
					clean = append(clean, c)
				}
			}
			if len(clean) > 0 {
				result := string(clean)
				fmt.Printf("CTO named product: %s\n", result)
				return result
			}
		}
	}
	fmt.Println("CTO did not provide a product name — using default.")
	return "product"
}

// research gathers information about the product idea.
// Returns the spec text and the CTO's evaluation (which includes the derived product name).
func (p *Pipeline) research(ctx context.Context, input ProductInput) (spec string, ctoEval string, err error) {
	if input.SpecFile != "" {
		content, err := p.ws.ReadFile(input.SpecFile)
		if err != nil {
			return "", "", fmt.Errorf("read spec: %w", err)
		}
		spec = content
	} else if input.URL != "" {
		researcher, err := p.ensureAgent(ctx, roles.RoleResearcher, "researcher")
		if err != nil {
			return "", "", err
		}
		_, evaluation, err := researcher.Runtime.Research(ctx, input.URL,
			"extract the product idea, key entities, features, and requirements. Output in Code Graph vocabulary where possible.")
		if err != nil {
			return "", "", fmt.Errorf("research URL: %w", err)
		}
		spec = evaluation
	} else if input.Description != "" {
		spec = input.Description
	}

	// CTO evaluates feasibility and derives a product name in one call.
	// Fresh provider per call — avoids context accumulation across pipeline phases.
	rawProvider, err := p.providerForRole(roles.RoleCTO)
	if err != nil {
		return "", "", fmt.Errorf("CTO provider: %w", err)
	}
	ctoTracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleCTO] = ctoTracker

	ctoResp, err := ctoTracker.Reason(ctx, fmt.Sprintf(`Evaluate this product idea for feasibility. What agents are needed? What's the build sequence? Key risks?

On the LAST LINE of your response, output ONLY a kebab-case product name (2-4 words, lowercase, no special characters) like:
NAME: my-product-name

Product idea:
%s`, spec), nil)
	if err != nil {
		return "", "", fmt.Errorf("CTO evaluate: %w", err)
	}
	ctoEval = ctoResp.Content()

	fmt.Printf("CTO Assessment:\n%s\n", ctoEval)
	return spec, ctoEval, nil
}

// design creates a full Code Graph spec from the product idea.
// The Architect self-reviews for minimality — no separate CTO review call needed.
func (p *Pipeline) design(ctx context.Context, spec string) (string, error) {
	if _, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect"); err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`Design the full system architecture. Output a complete Code Graph spec.

CRITICAL CONSTRAINTS — review your own output before responding:
- Derive complexity from simple compositions. Each view: minimal elements needed.
- If a view feels heavy, decompose it. Elegant, simple, beautiful.
- Are views minimal? Is complexity derived from composition, not accumulated?
- Are there bloated entities or views that should be decomposed?
- Count your elements — can any be removed without losing functionality?

Specify the target language/framework at the top of your spec:
LANGUAGE: go

Product idea:
%s`, spec)

	// Fresh provider per call — avoids context accumulation across design phases.
	rawProvider, err := p.providerForRoleWithModel(roles.RoleArchitect, p.architectDesignModel())
	if err != nil {
		return "", fmt.Errorf("architect provider: %w", err)
	}
	architectTracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleArchitect] = architectTracker

	resp, err := architectTracker.Reason(ctx, prompt, nil)
	if err != nil {
		return "", fmt.Errorf("architect design: %w", err)
	}

	return resp.Content(), nil
}

// simplify reviews the Code Graph spec and reduces it to its minimal form.
func (p *Pipeline) simplify(ctx context.Context, design string) (string, error) {
	if _, err := p.ensureAgent(ctx, roles.RoleArchitect, "architect"); err != nil {
		return "", err
	}

	const maxRounds = 2
	current := design

	for round := 1; round <= maxRounds; round++ {
		// Fresh provider per round — avoids context accumulation across simplify passes.
		rawProvider, err := p.providerForRoleWithModel(roles.RoleArchitect, p.architectDesignModel())
		if err != nil {
			return "", fmt.Errorf("architect provider (round %d): %w", round, err)
		}
		architectTracker := resources.NewTrackingProvider(rawProvider)
		p.trackers[roles.RoleArchitect] = architectTracker

		resp, err := architectTracker.Reason(ctx,
			fmt.Sprintf(`Review this Code Graph spec for simplification. Apply ALL simplifications in ONE pass.

- Can any View be composed from fewer elements? Any redundant or derivable?
- Can any Entity be smaller? Properties derived instead of stored?
- Can any State machine have fewer states or transitions?

If you find simplifications, output the COMPLETE REVISED spec.
If already minimal, respond with exactly: MINIMAL

Current spec:
%s`, current), nil)
		if err != nil {
			return "", fmt.Errorf("simplify round %d: %w", round, err)
		}
		analysis := resp.Content()

		upper := strings.ToUpper(strings.TrimSpace(analysis))
		if upper == "MINIMAL" || strings.HasPrefix(upper, "MINIMAL") {
			fmt.Printf("Simplification complete after %d round(s) — spec is minimal.\n", round)
			return current, nil
		}

		fmt.Printf("Simplification round %d applied.\n", round)
		current = analysis
	}

	fmt.Printf("Simplification capped at %d rounds.\n", maxRounds)
	return current, nil
}

// extractLanguage pulls the target language from the design spec.
// Looks for "LANGUAGE: xxx" in the spec. Defaults to "go".
func (p *Pipeline) extractLanguage(design string) string {
	for _, line := range strings.Split(design, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "LANGUAGE:") {
			lang := strings.TrimSpace(line[len("LANGUAGE:"):])
			lang = strings.ToLower(lang)
			if lang != "" {
				return lang
			}
		}
	}
	return "go"
}

// build generates multi-file code from the design spec.
// Uses Evaluate (not CodeWrite) to avoid the "return ONLY code" instruction
// conflicting with our multi-file --- FILE: path --- format.
func (p *Pipeline) build(ctx context.Context, design string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`Generate production-quality %s code from this specification.

Output ALL files needed for a complete, runnable project. Use this exact format for each file:

--- FILE: path/to/file.ext ---
<file contents>

Include:
- Project config (go.mod, package.json, Cargo.toml, etc.)
- Source files (organized in packages/modules)
- Test files alongside the code they test
- A README.md with build and run instructions

CRITICAL OUTPUT FORMAT RULES:
- Every line of output must be inside a file block
- Inside file blocks, output ONLY raw source code — no markdown fences (no `+"`"+`"`+"`"+`), no prose, no explanations
- Do NOT include any text outside of file blocks

Specification:
%s`, lang, design)

	// Use a fresh Sonnet provider — full-pipeline builds generate entire codebases
	// with up to 3 rebuild rounds; ~5x cost difference vs Opus per token.
	buildModel := p.fullBuilderModel()
	buildProvider, err := p.providerForRoleWithModel(roles.RoleBuilder, buildModel)
	if err != nil {
		return nil, fmt.Errorf("builder provider: %w", err)
	}
	buildTracker := resources.NewTrackingProvider(buildProvider)
	p.trackers[roles.RoleBuilder] = buildTracker
	fmt.Printf("  ↳ build using %s\n", buildModel)

	buildResp, err := buildTracker.Reason(ctx, prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("builder code: %w", err)
	}
	code := buildResp.Content()

	// Record the build action
	if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "multi-file generation from spec"); err != nil {
		fmt.Printf("warning: write_code action event failed: %v\n", err)
	}

	// Parse multi-file output
	files := parseFiles(code)
	if len(files) == 0 {
		// Fallback: treat entire output as a single file
		ext := langExtension(lang)
		files = map[string]string{"main" + ext: code}
	}

	// Sanitize go.mod — LLMs often embed newlines in the module path.
	sanitizeGoMod(files)

	// Write all files to product repo
	for path, content := range files {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit(fmt.Sprintf("feat: initial %s code generation from spec", lang)); err != nil {
		return nil, fmt.Errorf("commit code: %w", err)
	}

	fmt.Printf("Generated %d files, committed.\n", len(files))
	return files, nil
}

// rebuild sends reviewer feedback to the builder and generates revised code.
func (p *Pipeline) rebuild(ctx context.Context, currentFiles map[string]string, feedback string, design string, lang string) (map[string]string, error) {
	var filesSummary strings.Builder
	for path, content := range currentFiles {
		filesSummary.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n", path, content))
	}

	prompt := fmt.Sprintf(`The reviewer provided feedback on the code. Fix the issues and output ALL files again using the same format.

Reviewer feedback:
%s

Original specification:
%s

Current code:
%s

Output the COMPLETE revised files using --- FILE: path --- markers. Include ALL files, not just changed ones.`, feedback, design, filesSummary.String())

	rebuildModel := p.fullBuilderModel()
	rebuildProvider, err := p.providerForRoleWithModel(roles.RoleBuilder, rebuildModel)
	if err != nil {
		return nil, fmt.Errorf("builder provider: %w", err)
	}
	rebuildTracker := resources.NewTrackingProvider(rebuildProvider)
	p.trackers[roles.RoleBuilder] = rebuildTracker
	fmt.Printf("  ↳ rebuild using %s\n", rebuildModel)

	rebuildResp, err := rebuildTracker.Reason(ctx, prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("rebuild: %w", err)
	}
	code := rebuildResp.Content()

	files := parseFiles(code)
	if len(files) == 0 {
		return currentFiles, nil // no parseable output, keep current
	}

	sanitizeGoMod(files)

	for path, content := range files {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit("fix: address reviewer feedback"); err != nil {
		return nil, fmt.Errorf("commit rebuild: %w", err)
	}

	fmt.Printf("Rebuilt %d files from feedback, committed.\n", len(files))
	return files, nil
}

// review checks code quality and spec compliance in a single LLM call.
// Returns feedback and whether approved.
func (p *Pipeline) review(ctx context.Context, files map[string]string, design string, lang string) (feedback string, approved bool, err error) {
	// Build code summary for review
	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, content))
	}
	allCode := codeSummary.String()

	// Single comprehensive review call — replaces 4 separate calls.
	// Uses a fresh Sonnet provider — pass/fail classification doesn't require Opus.
	reviewModel := p.fullReviewerModel()
	reviewProvider, err := p.providerForRoleWithModel(roles.RoleReviewer, reviewModel)
	if err != nil {
		return "", false, fmt.Errorf("reviewer provider: %w", err)
	}
	reviewTracker := resources.NewTrackingProvider(reviewProvider)
	p.trackers[roles.RoleReviewer] = reviewTracker
	fmt.Printf("  ↳ review using %s\n", reviewModel)

	reviewPrompt := fmt.Sprintf(`Review this %s code comprehensively. Cover ALL of the following in ONE response:

## 1. Code Quality
Bugs, security issues, error handling, test coverage, best practices.

## 2. Spec Compliance
Does the code match this design spec? Flag deviations.
Design:
%s

## 3. Simplicity
- Unnecessary complexity? Over-engineered patterns?
- Components that could be derived from simpler compositions?
- Extras beyond the spec?

## 4. Verdict
End with exactly one of:
- APPROVED — code is ready
- CHANGES NEEDED: followed by the specific issues to fix

Code:
%s`, lang, design, allCode)

	reviewResp, err := reviewTracker.Reason(ctx, reviewPrompt, nil)
	if err != nil {
		return "", false, fmt.Errorf("review: %w", err)
	}
	review := reviewResp.Content()

	fmt.Printf("Review:\n%s\n", review)

	approved = detectApproval(review)
	return review, approved, nil
}

// test installs deps, runs tests, and has the tester analyze failures.
// Skips the analysis LLM call if tests pass — no need to spend tokens on "looks good".
func (p *Pipeline) test(ctx context.Context, files map[string]string, lang string) error {
	// Install dependencies first
	p.installDeps(lang)

	// Run tests
	testCmd, testArgs := langTestCommand(lang)
	fmt.Printf("Running: %s %s\n", testCmd, strings.Join(testArgs, " "))

	moduleDir := findModuleDir(p.product.Dir, lang)
	cmd := exec.Command(testCmd, testArgs...)
	cmd.Dir = moduleDir
	testOutput, testErr := cmd.CombinedOutput()

	testResult := string(testOutput)
	if testErr == nil {
		fmt.Printf("Tests passed:\n%s\n", testResult)
		return nil // No need for tester analysis or builder fixes.
	}

	fmt.Printf("Tests failed:\n%s\n", testResult)

	// Tests failed — spawn the tester agent (side effect: registers agent in actor store).
	if _, err := p.ensureAgent(ctx, roles.RoleTester, "tester"); err != nil {
		return err
	}

	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, content))
	}

	// Fresh provider per call — avoids context accumulation across test iterations.
	testerModel := p.fullTesterModel()
	testerProvider, err := p.providerForRoleWithModel(roles.RoleTester, testerModel)
	if err != nil {
		return fmt.Errorf("tester provider: %w", err)
	}
	testerTracker := resources.NewTrackingProvider(testerProvider)
	p.trackers[roles.RoleTester] = testerTracker
	fmt.Printf("  ↳ test analysis using %s\n", testerModel)

	testerResp, err := testerTracker.Reason(ctx, fmt.Sprintf(`Tests are failing. Analyze the failures and identify root causes.

Test output:
%s

Code:
%s`, testResult, codeSummary.String()), nil)
	if err != nil {
		return fmt.Errorf("test analysis: %w", err)
	}
	testEval := testerResp.Content()

	fmt.Printf("Test Analysis:\n%s\n", testEval)

	// Have the builder fix the failures.
	{
		fmt.Println("Attempting to fix failing tests...")
		builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
		if err != nil {
			return err
		}

		fixed := false

		// Try agentic mode first — builder reads/writes files directly
		tracker := p.trackers[roles.RoleBuilder]
		if tracker != nil {
			instruction := fmt.Sprintf(`You are working in a %s repository. The tests are failing. Fix the code so tests pass.

Test output:
%s

Read the failing tests and the code under test, fix the issues, and run tests to verify they pass.
Use the project's existing test commands (e.g., go test ./... for Go).
Preserve existing code style and conventions.`, lang, testResult)

			result, opErr := tracker.Operate(ctx, decision.OperateTask{
				WorkDir:     moduleDir,
				Instruction: instruction,
			})
			if opErr == nil {
				fmt.Printf("Builder (agentic fix): %s\n", truncate(result.Summary, 200))

				if _, actErr := builder.Runtime.Act(ctx, writeCodeAction(lang), "agentic test fix"); actErr != nil {
					fmt.Printf("warning: write_code action event failed: %v\n", actErr)
				}

				if stageErr := p.product.StageAll(); stageErr != nil {
					return fmt.Errorf("stage test fix: %w", stageErr)
				}
				// CommitIfStaged returns nil when nothing was staged — the builder
				// may have already committed the fix internally via Operate.
				if commitErr := p.product.CommitIfStaged("fix: address failing tests"); commitErr != nil {
					return fmt.Errorf("commit test fix: %w", commitErr)
				}
				fixed = true
			} else {
				fmt.Printf("Agentic mode unavailable (%v), falling back to text mode.\n", opErr)
			}
		}

		// Fallback: text-based fix
		if !fixed {
			fixPrompt := fmt.Sprintf(`The tests are failing. Fix the code so tests pass.

Test output:
%s

Current code:
%s

Output ALL files using --- FILE: path --- markers.`, testResult, codeSummary.String())

			// Fresh provider per call — avoids context accumulation across test-fix iterations.
			fixModel := p.fullBuilderModel()
			fixProvider, err := p.providerForRoleWithModel(roles.RoleBuilder, fixModel)
			if err != nil {
				return fmt.Errorf("builder provider (test fix): %w", err)
			}
			fixTracker := resources.NewTrackingProvider(fixProvider)
			p.trackers[roles.RoleBuilder] = fixTracker
			fmt.Printf("  ↳ test fix using %s\n", fixModel)

			fixResp, err := fixTracker.Reason(ctx, fixPrompt, nil)
			if err != nil {
				return fmt.Errorf("fix tests: %w", err)
			}
			fixedCode := fixResp.Content()

			fixedFiles := parseFiles(fixedCode)
			sanitizeGoMod(fixedFiles)
			for path, content := range fixedFiles {
				if err := p.product.WriteFile(path, content); err != nil {
					return fmt.Errorf("write fix %s: %w", path, err)
				}
			}
			if len(fixedFiles) > 0 {
				if err := p.product.Commit("fix: address failing tests"); err != nil {
					return fmt.Errorf("commit test fix: %w", err)
				}
			}
		}

		// Re-run tests
		cmd2 := exec.Command(testCmd, testArgs...)
		cmd2.Dir = moduleDir
		retryOutput, retryErr := cmd2.CombinedOutput()
		if retryErr != nil {
			fmt.Printf("Tests still failing after fix attempt:\n%s\n", string(retryOutput))
			return fmt.Errorf("tests still failing after fix attempt")
		}
		fmt.Printf("Tests now passing:\n%s\n", string(retryOutput))
	}

	return nil
}

// installDeps runs dependency installation for the target language.
func (p *Pipeline) installDeps(lang string) {
	var cmd *exec.Cmd

	switch strings.ToLower(lang) {
	case "go", "golang":
		cmd = exec.Command("go", "mod", "tidy")
	case "typescript", "ts", "javascript", "js":
		cmd = exec.Command("npm", "install")
	case "python", "py":
		// Check if requirements.txt exists
		if p.product != nil {
			cmd = exec.Command("pip", "install", "-r", "requirements.txt")
		}
	case "rust", "rs":
		cmd = exec.Command("cargo", "build")
	case "csharp", "c#", "cs":
		cmd = exec.Command("dotnet", "restore")
	default:
		return
	}

	if cmd != nil && p.product != nil {
		cmd.Dir = findModuleDir(p.product.Dir, lang)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Dep install warning: %s\n", string(out))
		} else {
			fmt.Printf("Dependencies installed.\n")
		}
	}
}

// integrate assembles and prepares for deployment.
func (p *Pipeline) integrate(ctx context.Context) error {
	integrator, err := p.ensureAgent(ctx, roles.RoleIntegrator, "integrator")
	if err != nil {
		return err
	}

	_, err = integrator.Runtime.Act(ctx, ActionIntegrate, "staging")
	if err != nil {
		return fmt.Errorf("integration: %w", err)
	}

	// Push to GitHub
	if err := p.product.Push(); err != nil {
		fmt.Printf("Push failed (may need manual push): %v\n", err)
	} else {
		fmt.Printf("Pushed to https://github.com/%s\n", p.product.Repo)
	}

	// Escalate to human for production approval
	humanID := p.humanID
	_, err = integrator.Runtime.Escalate(ctx, humanID, "Product ready for human review before production deploy")
	if err != nil {
		return fmt.Errorf("escalate: %w", err)
	}

	fmt.Println("Product assembled and ready for human review.")
	return nil
}
