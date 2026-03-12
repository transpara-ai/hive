package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/resources"
	"github.com/lovyou-ai/hive/pkg/roles"
	"github.com/lovyou-ai/hive/pkg/workspace"
)

// RunTargeted executes the targeted pipeline for modifying existing code.
// Skips research/design/simplify — goes straight to understand → modify → review → test.
func (p *Pipeline) RunTargeted(ctx context.Context, input ProductInput) error {
	if input.RepoPath == "" {
		return fmt.Errorf("RunTargeted requires RepoPath")
	}
	if input.Description == "" {
		return fmt.Errorf("RunTargeted requires Description (what to change)")
	}

	pipelineStart := time.Now()
	// Merge into pre-populated telemetry (e.g., CTO analysis cost from self-improve)
	// rather than discarding it. Always re-set Mode, Description, and StartedAt.
	if p.telemetry == nil {
		p.telemetry = &PipelineResult{}
	}
	p.telemetry.Mode = "targeted"
	p.telemetry.InputDescription = input.Description
	p.telemetry.StartedAt = pipelineStart
	p.trackers = make(map[roles.Role]*resources.TrackingProvider)
	defer func() {
		p.telemetry.collectTokenUsage(p.trackers)
		writeTelemetry(p.telemetryBaseDir(), p.telemetry)
		p.telemetry = nil
	}()
	type phaseTiming struct {
		name     string
		duration time.Duration
	}
	var timings []phaseTiming

	// Open existing repo
	product, err := workspace.OpenRepo(input.RepoPath)
	if err != nil {
		return p.failPhase("Context Load", fmt.Errorf("open repo: %w", err))
	}
	p.product = product
	fmt.Printf("Repo: %s\n", product.Dir)

	// ── Phase 1: Context Load ──
	fmt.Println("═══ Phase 1: Context Load ═══")
	phaseStart := time.Now()
	var existingFiles map[string]string
	if input.ContextFiles != nil {
		// Use pre-filtered files supplied by the caller (e.g. self-improve
		// passes pipeline-scoped files to avoid sending the full codebase to
		// the Builder, preventing context bloat from unrelated packages).
		existingFiles = input.ContextFiles
		fmt.Printf("Using %d pre-filtered source files (context scoped by caller).\n", len(existingFiles))
	} else {
		var err error
		existingFiles, err = product.ReadSourceFiles()
		if err != nil {
			return p.failPhase("Context Load", fmt.Errorf("read source files: %w", err))
		}
		fmt.Printf("Loaded %d source files.\n", len(existingFiles))
	}

	gitLog, _ := product.GitLog(10)
	if gitLog != "" {
		fmt.Printf("Recent history:\n%s\n", gitLog)
	}

	// Build a lightweight file listing for CTO (not full contents)
	fileListing := buildFileListing(existingFiles)

	// Detect language from existing files
	lang := detectLanguage(existingFiles)
	fmt.Printf("Detected language: %s\n", lang)

	// Include key context files (CLAUDE.md, README, etc.) for CTO — not the full codebase
	keyContext := extractKeyFiles(existingFiles)

	contextLoadDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Context Load", contextLoadDuration})
	p.telemetry.addPhaseTiming("Context Load", contextLoadDuration)

	// ── Phase 2: Understand ──
	fmt.Println("═══ Phase 2: Understand ═══")
	phaseStart = time.Now()
	var ctoAnalysis string
	if input.CTOAnalysis != "" {
		ctoAnalysis = input.CTOAnalysis
		fmt.Println("Using pre-computed CTO analysis (skipped Understand).")
		fmt.Printf("CTO Analysis:\n%s\n", ctoAnalysis)
	} else {
		model := p.selfImproveCTOModel()
		rawProvider, err := p.providerForRoleWithModel(roles.RoleCTO, model)
		if err != nil {
			return p.failPhase("Understand", fmt.Errorf("CTO provider: %w", err))
		}
		ctoTracker := resources.NewTrackingProvider(rawProvider)
		p.trackers[roles.RoleCTO] = ctoTracker
		fmt.Printf("  ↳ targeted understand using %s\n", model)

		ctoPrompt := fmt.Sprintf(`Analyze this change request. Be BRIEF — the Builder reads files itself.

Output ONLY:
- Which files to change (paths + what to do in each, 1 line per file)
- Key risks (1-2 sentences max)
- Nothing else. No tables, no code blocks, no headers.

Change request: %s

Git history:
%s

Project structure:
%s

%s`, input.Description, gitLog, fileListing, keyContext)

		ctoResp, err := ctoTracker.Reason(ctx, ctoPrompt, nil)
		if err != nil {
			return p.failPhase("Understand", fmt.Errorf("CTO analysis: %w", err))
		}
		ctoAnalysis = ctoResp.Content()
		fmt.Printf("CTO Analysis:\n%s\n", ctoAnalysis)
	}

	// Early-exit if the CTO analysis identifies no relevant files — the change
	// is already implemented or cannot be mapped to specific files.
	if len(parseRelevantFiles(ctoAnalysis)) == 0 {
		fmt.Println("CTO analysis identified no relevant files — change may already be implemented. Skipping.")
		return nil
	}

	// Create branch for the changes
	branchName := "hive/" + sanitizeBranchName(input.Description)
	if err := product.CreateBranch(branchName); err != nil {
		return p.failPhase("Understand", fmt.Errorf("create branch: %w", err))
	}
	fmt.Printf("Branch: %s\n", branchName)

	// Capture base commit before building — reviewer diffs against this.
	baseCommit, err := product.HeadCommit()
	if err != nil {
		return p.failPhase("Understand", fmt.Errorf("capture base commit: %w", err))
	}

	understandDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Understand", understandDuration})
	p.telemetry.addPhaseTiming("Understand", understandDuration)

	// ── Phase 3: Modify ──
	fmt.Println("═══ Phase 3: Modify ═══")
	phaseStart = time.Now()
	files, err := p.modify(ctx, existingFiles, ctoAnalysis, input.Description, lang)
	if err != nil {
		return p.failPhase("Modify", fmt.Errorf("modify: %w", err))
	}
	if halt := p.guardianCheck(ctx, "modify"); halt {
		return p.failPhase("Modify", fmt.Errorf("guardian halted pipeline after modify phase"))
	}

	modifyDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Modify", modifyDuration})
	p.telemetry.addPhaseTiming("Modify", modifyDuration)

	// ── Phase 4: Review ──
	phaseStart = time.Now()
	if p.skipReviewer {
		fmt.Println("═══ Phase 4: Review (skipped) ═══")
	} else {
		const maxReviewRounds = 3
		for round := 1; round <= maxReviewRounds; round++ {
			fmt.Printf("═══ Phase 4: Review (round %d) ═══\n", round)
			feedback, approved, err := p.reviewTargeted(ctx, baseCommit, ctoAnalysis, input.Description, lang)
			if err != nil {
				return p.failPhase("Review", fmt.Errorf("review round %d: %w", round, err))
			}
			p.telemetry.addReviewSignal(approved)

			if approved {
				fmt.Println("Changes approved by reviewer.")
				break
			}

			if round == maxReviewRounds {
				fmt.Println("Max review rounds reached — proceeding with current code.")
				break
			}

			fmt.Printf("═══ Phase 4b: Revise from feedback (round %d) ═══\n", round)
			files, err = p.revise(ctx, files, feedback, input.Description, lang)
			if err != nil {
				return p.failPhase("Review", fmt.Errorf("revise round %d: %w", round, err))
			}
		}
	}
	reviewDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Review", reviewDuration})
	p.telemetry.addPhaseTiming("Review", reviewDuration)

	// ── Phase 5: Test ──
	fmt.Println("═══ Phase 5: Test ═══")
	phaseStart = time.Now()
	err = p.test(ctx, files, lang)
	if err != nil {
		return p.failPhase("Test", fmt.Errorf("test: %w", err))
	}

	testDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"Test", testDuration})
	p.telemetry.addPhaseTiming("Test", testDuration)

	// ── Phase 6: PR ──
	fmt.Println("═══ Phase 6: PR ═══")
	phaseStart = time.Now()
	prURL, prErr := p.openPR(ctx, product, branchName, input.Description, ctoAnalysis)
	if prErr != nil {
		fmt.Printf("PR creation failed (may need manual push): %v\n", prErr)
		return prErr
	}
	p.telemetry.PRURL = prURL
	prDuration := time.Since(phaseStart)
	timings = append(timings, phaseTiming{"PR", prDuration})
	p.telemetry.addPhaseTiming("PR", prDuration)

	// ── Phase 7: Merge ──
	if prURL != "" {
		fmt.Println("═══ Phase 7: Merge ═══")
		phaseStart = time.Now()
		integrator, mergeErr := p.ensureAgent(ctx, roles.RoleIntegrator, "integrator")
		if mergeErr != nil {
			fmt.Printf("Merge phase skipped (integrator unavailable): %v\n", mergeErr)
		} else {
			approved := true
			if p.gate != nil {
				approved = p.requestMergeApproval(prURL)
			}
			if approved {
				if err := p.mergePR(ctx, product, prURL); err != nil {
					fmt.Printf("PR merge failed (may need manual merge): %v\n", err)
				} else {
					p.telemetry.Merged = true
					if _, err := integrator.Runtime.Act(ctx, ActionMergePR, prURL); err != nil {
						fmt.Printf("warning: merge_pr action event failed: %v\n", err)
					}
				}
			} else {
				fmt.Println("PR merge skipped — approval denied.")
			}
		}
		mergeDuration := time.Since(phaseStart)
		timings = append(timings, phaseTiming{"Merge", mergeDuration})
		p.telemetry.addPhaseTiming("Merge", mergeDuration)
	}

	fmt.Println("═══ Pipeline Complete ═══")
	p.PrintTokenSummary()

	totalDuration := time.Since(pipelineStart)
	fmt.Println("\n═══ Timing Summary ═══")
	fmt.Printf("  %-16s %s\n", "Phase", "Duration")
	fmt.Printf("  %-16s %s\n", "─────", "────────")
	for _, t := range timings {
		fmt.Printf("  %-16s %s\n", t.name, t.duration.Round(time.Millisecond))
	}
	fmt.Printf("  %-16s %s\n", "TOTAL", totalDuration.Round(time.Millisecond))
	return nil
}

// modify uses the builder in agentic mode to modify existing code directly.
// Falls back to text-based modify if the provider doesn't support Operate.
func (p *Pipeline) modify(ctx context.Context, existingFiles map[string]string, ctoAnalysis string, changeReq string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	// Targeted builds use a temporary Sonnet provider — CTO-directed changes
	// are small and well-scoped. The builder agent is still ensured normally
	// (for Runtime.Act event emissions), but Operate calls go through Sonnet.
	model := p.targetedBuilderModel()
	rawProvider, err := p.providerForRoleWithModel(roles.RoleBuilder, model)
	if err != nil {
		return nil, fmt.Errorf("builder provider: %w", err)
	}
	tracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleBuilder] = tracker
	fmt.Printf("  ↳ targeted build using %s\n", model)

	// Try agentic mode first — builder reads/writes files directly
	{
		relevantFiles := parseRelevantFiles(ctoAnalysis)
		instruction := fmt.Sprintf(`You are working in a %s repository. Implement the following change:

CHANGE REQUEST: %s

CTO ANALYSIS: %s

Read the existing code, make the changes, and run tests to verify they pass.
If tests fail, fix the issues and re-run until tests pass.
Use the project's existing test commands (e.g., go test ./... for Go).
Preserve existing code style and conventions.
Do NOT add unnecessary changes beyond what's requested.`, lang, changeReq, ctoAnalysis)
		if len(relevantFiles) > 0 {
			instruction += "\nFocus ONLY on these files (identified by the CTO):\n" + strings.Join(relevantFiles, "\n") + "\nDo NOT read other source files unless a dependency forces it."
		}

		// Embed relevant file contents from the pre-filtered context so the
		// builder does not need to read the full codebase from disk. Applies
		// the same file-filtering pattern used for CTO analysis to stabilise
		// builder token usage across self-improve iterations.
		if fileContext := buildRelevantFileContext(existingFiles, relevantFiles); fileContext != "" {
			instruction += "\n\nRELEVANT FILE CONTENTS (pre-loaded — read these instead of re-reading from disk):\n" + fileContext
		}

		result, err := tracker.Operate(ctx, decision.OperateTask{
			WorkDir:     p.product.Dir,
			Instruction: instruction,
		})
		if err == nil {
			fmt.Printf("Builder (agentic): %s\n", truncate(result.Summary, 200))

			// Record the action event
			if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "agentic modification"); err != nil {
				fmt.Printf("warning: write_code action event failed: %v\n", err)
			}

			// Stage and commit whatever the builder changed.
			// CommitIfStaged returns nil when nothing was staged — the builder
			// correctly determined the change was already implemented.
			_ = p.product.StageAll()
			if commitErr := p.product.CommitIfStaged(fmt.Sprintf("feat: %s", truncate(changeReq, 60))); commitErr != nil {
				return nil, fmt.Errorf("commit changes: %w", commitErr)
			}

			// Re-read files from disk (builder may have changed anything)
			updatedFiles, readErr := p.product.ReadSourceFiles()
			if readErr != nil {
				return existingFiles, nil
			}
			return updatedFiles, nil
		}
		// If Operate isn't supported, fall through to text mode
		fmt.Printf("Agentic mode unavailable (%v), falling back to text mode.\n", err)
	}

	// Fallback: text-based modify — filter to relevant files to reduce token usage.
	// Parse the CTO analysis for file paths and keep only those plus key doc files.
	filteredFiles := existingFiles
	if relevantPaths := parseRelevantFiles(ctoAnalysis); len(relevantPaths) > 0 {
		relevant := make(map[string]bool, len(relevantPaths)+2)
		for _, rp := range relevantPaths {
			relevant[rp] = true
		}
		relevant["CLAUDE.md"] = true
		relevant["go.mod"] = true

		filtered := make(map[string]string)
		for path, content := range existingFiles {
			if relevant[path] {
				filtered[path] = content
			}
		}
		if len(filtered) > 0 {
			filteredFiles = filtered
			fmt.Printf("  ↳ text mode: %d/%d files (filtered by CTO analysis)\n", len(filteredFiles), len(existingFiles))
		}
	}

	var codeContext strings.Builder
	for path, content := range filteredFiles {
		codeContext.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n\n", path, content))
	}
	return p.modifyText(ctx, existingFiles, codeContext.String(), ctoAnalysis, changeReq, lang)
}

// modifyText is the text-based fallback for modify when Operate isn't available.
func (p *Pipeline) modifyText(ctx context.Context, existingFiles map[string]string, existingCode string, ctoAnalysis string, changeReq string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	prompt := fmt.Sprintf(`Modify this existing %s codebase to implement the requested change.

CHANGE REQUEST:
%s

CTO ANALYSIS:
%s

EXISTING CODEBASE:
%s

CRITICAL OUTPUT FORMAT RULES:
- Output ONLY file blocks using --- FILE: path --- markers
- Inside each file block, output ONLY raw source code — no markdown fences, no prose, no explanations
- Include ONLY files that changed or are new — do not re-output unchanged files
- Every line of your response must be inside a file block
- Preserve existing code style and conventions`, lang, changeReq, ctoAnalysis, existingCode)

	_, code, err := builder.Runtime.Evaluate(ctx, "code_modification", prompt)
	if err != nil {
		return nil, fmt.Errorf("builder modify: %w", err)
	}

	if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "targeted modification"); err != nil {
		fmt.Printf("warning: write_code action event failed: %v\n", err)
	}

	changedFiles := parseFiles(code)
	if len(changedFiles) == 0 {
		return nil, fmt.Errorf("builder produced no parseable file output")
	}

	sanitizeGoMod(changedFiles)

	// Merge: start with existing files, overlay changes
	merged := make(map[string]string, len(existingFiles)+len(changedFiles))
	for k, v := range existingFiles {
		merged[k] = v
	}
	for k, v := range changedFiles {
		merged[k] = v
	}

	for path, content := range changedFiles {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit(fmt.Sprintf("feat: %s", truncate(changeReq, 60))); err != nil {
		return nil, fmt.Errorf("commit changes: %w", err)
	}

	fmt.Printf("Modified %d files, committed.\n", len(changedFiles))
	return merged, nil
}

// targetedBuilderModel returns the model to use for targeted builds.
// Defaults to Sonnet — targeted builds are CTO-directed with small,
// well-scoped modifications that don't need Opus-level reasoning.
func (p *Pipeline) targetedBuilderModel() string {
	if p.builderModel != "" {
		return p.builderModel
	}
	return "claude-sonnet-4-6"
}

// targetedReviewModel returns the model to use for targeted reviews.
// Defaults to Sonnet — targeted reviews only check a focused diff.
func (p *Pipeline) targetedReviewModel() string {
	if p.reviewerModel != "" {
		return p.reviewerModel
	}
	return "claude-sonnet-4-6"
}

// reviewTargeted reviews changes against the original codebase using git diff.
// Uses a temporary Sonnet provider — targeted reviews only check a focused diff,
// not deep architectural reasoning. Override with Config.ReviewerModel.
// The provider is created fresh each call, avoiding the agent cache entirely.
func (p *Pipeline) reviewTargeted(ctx context.Context, baseCommit string, ctoAnalysis string, changeReq string, lang string) (string, bool, error) {
	// Use git diff from the base commit — only the builder's changes, not history.
	diff, err := p.product.GitDiff(baseCommit)
	if err != nil {
		return "", false, fmt.Errorf("git diff from %s: %w", baseCommit, err)
	}
	if diff == "" {
		return "No changes to review.", true, nil
	}

	// Targeted reviews use a temporary Sonnet provider — no need for a full
	// reviewer agent. This avoids the agent cache entirely (an Opus reviewer
	// cached from a greenfield run would silently ignore a model override).
	model := p.targetedReviewModel()
	rawProvider, err := p.providerForRoleWithModel(roles.RoleReviewer, model)
	if err != nil {
		return "", false, fmt.Errorf("review provider: %w", err)
	}
	// Wrap in tracking so token usage appears in the summary.
	tracker := resources.NewTrackingProvider(rawProvider)
	p.trackers[roles.RoleReviewer] = tracker
	fmt.Printf("  ↳ targeted review using %s\n", model)

	prompt := fmt.Sprintf(`Review this diff to a %s codebase. Be CONCISE — no tables, no headers.

CHANGE REQUEST: %s
CTO ANALYSIS: %s

DIFF:
%s

Only BLOCK for: correctness bugs, logic errors, security issues, or broken tests.
Do NOT block for: style nits, missing tests, doc comments, scope creep, or nice-to-haves.
Mention non-blocking concerns briefly but still approve.

End with: APPROVED or CHANGES NEEDED: <specific blocking issues>`, lang, changeReq, ctoAnalysis, diff)

	// Reviewer must ingest full diff + codebase context + CTO analysis before
	// producing structured output. The ambient 5-minute cap is too tight (Run 35
	// took 3m9s). Override to 10 minutes for this step only.
	reviewCtx, reviewCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer reviewCancel()
	resp, err := tracker.Reason(reviewCtx, prompt, nil)
	if err != nil {
		return "", false, fmt.Errorf("review: %w", err)
	}
	review := resp.Content()

	fmt.Printf("Review:\n%s\n", review)

	return review, detectApproval(review), nil
}

// revise applies reviewer feedback — agentic mode preferred, text fallback.
func (p *Pipeline) revise(ctx context.Context, files map[string]string, feedback string, changeReq string, lang string) (map[string]string, error) {
	builder, err := p.ensureAgent(ctx, roles.RoleBuilder, "builder")
	if err != nil {
		return nil, err
	}

	// Targeted revisions use the same Sonnet provider as modify — CTO-directed
	// changes are small and well-scoped. Reuse the tracker already in p.trackers
	// (set by modify()), or create a fresh one if revise is called independently.
	tracker := p.trackers[roles.RoleBuilder]
	if tracker == nil {
		model := p.targetedBuilderModel()
		rawProvider, provErr := p.providerForRoleWithModel(roles.RoleBuilder, model)
		if provErr != nil {
			return nil, fmt.Errorf("builder provider: %w", provErr)
		}
		tracker = resources.NewTrackingProvider(rawProvider)
		p.trackers[roles.RoleBuilder] = tracker
		fmt.Printf("  ↳ targeted revision using %s\n", model)
	}

	// Try agentic mode
	{
		instruction := fmt.Sprintf(`You are working in a %s repository. Fix the reviewer's feedback:

ORIGINAL CHANGE REQUEST: %s

REVIEWER FEEDBACK:
%s

Read the current code, apply the fixes, and run tests to verify they pass.`, lang, changeReq, feedback)

		result, err := tracker.Operate(ctx, decision.OperateTask{
			WorkDir:     p.product.Dir,
			Instruction: instruction,
		})
		if err == nil {
			fmt.Printf("Builder (agentic revision): %s\n", truncate(result.Summary, 200))

			// Record the action event
			if _, actErr := builder.Runtime.Act(ctx, writeCodeAction(lang), "agentic revision"); actErr != nil {
				fmt.Printf("warning: write_code action event failed: %v\n", actErr)
			}

			_ = p.product.StageAll()
			if commitErr := p.product.Commit("fix: address reviewer feedback"); commitErr != nil {
				return nil, fmt.Errorf("commit revision: %w", commitErr)
			}

			updatedFiles, readErr := p.product.ReadSourceFiles()
			if readErr != nil {
				return files, nil
			}
			return updatedFiles, nil
		}
		fmt.Printf("Agentic mode unavailable (%v), falling back to text mode.\n", err)
	}

	// Fallback: text-based revise
	var codeSummary strings.Builder
	for path, content := range files {
		codeSummary.WriteString(fmt.Sprintf("--- FILE: %s ---\n%s\n\n", path, content))
	}

	prompt := fmt.Sprintf(`Fix the reviewer's issues with these changes to an existing %s codebase.

ORIGINAL CHANGE REQUEST: %s

REVIEWER FEEDBACK:
%s

CURRENT CODE (after modifications):
%s

Output ONLY the files that need further changes using --- FILE: path --- markers.`, lang, changeReq, feedback, codeSummary.String())

	// Text-based revise must re-evaluate the full diff + codebase + feedback.
	// The ambient 5-minute cap is too tight — Run 34 hit context deadline exceeded
	// here (revise: evaluate reasoning: claude CLI error: context deadline exceeded).
	// Override to 10 minutes for this step only.
	reviseCtx, reviseCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer reviseCancel()
	_, code, err := builder.Runtime.Evaluate(reviseCtx, "code_revision", prompt)
	if err != nil {
		return nil, fmt.Errorf("revise: %w", err)
	}

	if _, err := builder.Runtime.Act(ctx, writeCodeAction(lang), "text revision"); err != nil {
		fmt.Printf("warning: write_code action event failed: %v\n", err)
	}

	revisedFiles := parseFiles(code)
	if len(revisedFiles) == 0 {
		return files, nil
	}

	sanitizeGoMod(revisedFiles)

	for k, v := range revisedFiles {
		files[k] = v
	}
	for path, content := range revisedFiles {
		if err := p.product.WriteFile(path, content); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := p.product.Commit("fix: address reviewer feedback"); err != nil {
		return nil, fmt.Errorf("commit revision: %w", err)
	}

	fmt.Printf("Revised %d files from feedback, committed.\n", len(revisedFiles))
	return files, nil
}

// openPR pushes the branch and opens a pull request.
// Returns the PR URL on success (empty string on failure).
// Retries up to 3 times on transient GitHub errors (502, 504, network).
func (p *Pipeline) openPR(ctx context.Context, product *workspace.Product, branch string, changeReq string, analysis string) (string, error) {
	// Push the branch
	if err := product.PushBranch(); err != nil {
		return "", fmt.Errorf("push branch: %w", err)
	}

	// Open PR via gh CLI with retry for transient errors.
	title := truncate(changeReq, 70)
	body := fmt.Sprintf("## Change Request\n%s\n\n## CTO Analysis\n%s\n\n---\nGenerated by hive", changeReq, analysis)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.Command("gh", "pr", "create", "--title", title, "--body", body)
		cmd.Dir = product.Dir
		out, err := cmd.CombinedOutput()
		if err == nil {
			prURL := strings.TrimSpace(string(out))
			fmt.Printf("PR created: %s\n", prURL)
			return prURL, nil
		}
		lastErr = fmt.Errorf("gh pr create: %s: %w", string(out), err)
		if !isTransientGHError(string(out)) {
			return "", lastErr
		}
		fmt.Printf("PR creation attempt %d failed (transient), retrying in %ds...\n", attempt, attempt*5)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(attempt*5) * time.Second):
		}
	}
	return "", lastErr
}

// mergePR squash-merges a pull request via gh CLI.
// Non-fatal — logs and returns error if merge fails (e.g., branch protection).
// Retries up to 3 times on transient GitHub errors (502, 504, "base branch was modified").
func (p *Pipeline) mergePR(ctx context.Context, product *workspace.Product, prURL string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.Command("gh", "pr", "merge", prURL, "--squash")
		cmd.Dir = product.Dir
		out, err := cmd.CombinedOutput()
		if err == nil {
			fmt.Printf("PR merged: %s\n", strings.TrimSpace(string(out)))
			return nil
		}
		lastErr = fmt.Errorf("gh pr merge: %s: %w", string(out), err)
		outStr := string(out)
		if !isTransientGHError(outStr) && !strings.Contains(outStr, "Base branch was modified") {
			return lastErr
		}
		fmt.Printf("PR merge attempt %d failed (transient), retrying in %ds...\n", attempt, attempt*5)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt*5) * time.Second):
		}
	}
	return lastErr
}

// isTransientGHError returns true if the gh CLI output suggests a transient
// GitHub API error that's worth retrying (502, 504, network timeouts).
func isTransientGHError(output string) bool {
	transient := []string{"502", "504", "Gateway Timeout", "Bad Gateway", "ETIMEDOUT", "ECONNRESET"}
	for _, t := range transient {
		if strings.Contains(output, t) {
			return true
		}
	}
	return false
}

// requestMergeApproval emits an authority.requested event for PR merge and
// checks approval through the Gate. Returns true if approved.
func (p *Pipeline) requestMergeApproval(prURL string) bool {
	// Emit authority.requested on the graph.
	action := fmt.Sprintf("%s: %s", ActionMergePR, prURL)
	reqEventID, err := p.emitAuthorityRequested(action, "PR passed review and tests — requesting merge approval")
	if err != nil {
		fmt.Printf("warning: authority.requested event failed: %v\n", err)
		// Fall through — still check the gate for human approval.
		// Use a zero EventID; the gate doesn't depend on it.
	}

	authReq := authority.Request{
		ID:            reqEventID,
		Action:        action,
		Actor:         p.humanID,
		Level:         event.AuthorityLevelRequired,
		Justification: "PR passed review and tests — requesting merge approval",
		CreatedAt:     time.Now(),
	}
	resolution := p.gate.Check(authReq)

	// Emit authority.resolved — causally linked to authority.requested.
	if reqEventID != (types.EventID{}) {
		if _, err := p.emitAuthorityResolved(reqEventID, resolution); err != nil {
			fmt.Printf("warning: authority.resolved event failed: %v\n", err)
		}
	}

	return resolution.Approved
}
