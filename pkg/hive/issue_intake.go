package hive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const (
	IssueScanDefaultPolicyRef     = "civilization/repo-issue-scan-to-factory-order-v0.1"
	IssueScanPRReadyLabel         = "cc:pr-ready"
	IssueScanPRDeferredLabel      = "cc:pr-deferred"
	IssueScanNeedsHumanScopeLabel = "cc:needs-human-scope"
	IssueScanProtectedActionLabel = "cc:protected-action"
	issueScanBriefKind            = "transpara_ai_github_issue_scan"
	issueScanLifecycleVersionV02  = "civilization_issue_to_human_ready_pr_v0.2"
	issueScanLifecycleVersionV03  = "civilization_issue_to_human_ready_pr_v0.3"
	issueScanLifecycleVersionV04  = "civilization_issue_to_human_ready_pr_v0.4"
	issueScanLifecycleVersionV05  = "civilization_issue_to_human_ready_pr_v0.5"
	issueScanLifecycleVersionV06  = "civilization_issue_to_human_ready_pr_v0.6"
	issueScanLifecycleVersionV07  = "civilization_issue_to_human_ready_pr_v0.7"
	issueScanLifecycleVersionV08  = "civilization_issue_to_human_ready_pr_v0.8"
	// This version is a persisted brief contract. Bump it when lifecycle or
	// agent-plan text, policy metadata, evidence, roles, boundaries, gates, or
	// step order change.
	issueScanLifecycleVersion = "civilization_issue_to_human_ready_pr_v0.9"
	issueScanSourceType       = "github.issue"
)

type issueScanBriefIssuePayload struct {
	Rank        int      `json:"rank,omitempty"`
	Repo        string   `json:"repo"`
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	State       string   `json:"state,omitempty"`
	StateReason string   `json:"state_reason,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Body        string   `json:"body_excerpt,omitempty"`
}

type issueScanSelectionPolicyPayload struct {
	PolicyID       string   `json:"policy_id"`
	SelectedRank   int      `json:"selected_rank"`
	CandidateCount int      `json:"candidate_count"`
	RankingInputs  []string `json:"ranking_inputs"`
	Rationale      string   `json:"rationale"`
}

type issueScanRoleSeparationPolicyPayload struct {
	PolicyID        string                               `json:"policy_id"`
	PolicyVersion   string                               `json:"policy_version"`
	ActorPolicies   []issueScanRoleSeparationActorPolicy `json:"actor_policies"`
	GlobalNonClaims []string                             `json:"global_non_claims"`
}

type issueScanRoleSeparationActorPolicy struct {
	ActorClass            string   `json:"actor_class"`
	MapsToStageRoles      []string `json:"maps_to_stage_roles,omitempty"`
	CanOperateBranch      bool     `json:"can_operate_branch"`
	CanMutateGitHub       bool     `json:"can_mutate_github"`
	CanOpenPullRequest    bool     `json:"can_open_pull_request"`
	CanApproveOrMerge     bool     `json:"can_approve_or_merge"`
	AuthorityBoundary     string   `json:"authority_boundary"`
	RequiredEvidence      []string `json:"required_evidence,omitempty"`
	ForbiddenWithoutHuman []string `json:"forbidden_without_human,omitempty"`
}

type issueScanAutonomyGuardPolicyPayload struct {
	PolicyID                 string   `json:"policy_id"`
	PolicyVersion            string   `json:"policy_version"`
	RecommendationPosture    string   `json:"recommendation_posture"`
	AutonomyCeiling          string   `json:"autonomy_ceiling"`
	EvidenceInputs           []string `json:"evidence_inputs"`
	FailClosedOutputs        []string `json:"fail_closed_outputs"`
	HumanScopeTriggers       []string `json:"human_scope_triggers"`
	ForbiddenAuthorityClaims []string `json:"forbidden_authority_claims"`
}

type issueScanHumanRequiredClassificationPolicyPayload struct {
	PolicyID              string   `json:"policy_id"`
	PolicyVersion         string   `json:"policy_version"`
	ClassificationPosture string   `json:"classification_posture"`
	HumanRequiredTriggers []string `json:"human_required_triggers"`
	EscalationOutputs     []string `json:"escalation_outputs"`
	ForbiddenWithoutHuman []string `json:"forbidden_without_human"`
	NonAuthorityClaims    []string `json:"non_authority_claims"`
}

type issueScanAuthorityRecommendationPolicyPayload struct {
	PolicyID                 string   `json:"policy_id"`
	PolicyVersion            string   `json:"policy_version"`
	RecommendationPosture    string   `json:"recommendation_posture"`
	DecisionBoundary         string   `json:"decision_boundary"`
	EvidenceInputs           []string `json:"evidence_inputs"`
	RecommendationOutputs    []string `json:"recommendation_outputs"`
	RequiredHumanAuthority   []string `json:"required_human_authority"`
	ForbiddenAuthorityClaims []string `json:"forbidden_authority_claims"`
}

type issueScanValueAllocationBoundaryPolicyPayload struct {
	PolicyID                 string   `json:"policy_id"`
	PolicyVersion            string   `json:"policy_version"`
	BoundaryPosture          string   `json:"boundary_posture"`
	DisplayMode              string   `json:"display_mode"`
	ClassificationOutputs    []string `json:"classification_outputs"`
	RequiredHumanAuthority   []string `json:"required_human_authority"`
	ForbiddenSystemActions   []string `json:"forbidden_system_actions"`
	ForbiddenAuthorityClaims []string `json:"forbidden_authority_claims"`
	NonAuthorityClaims       []string `json:"non_authority_claims"`
}

type issueScanLifecycleStage struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	RequiredRoles     []string `json:"required_roles"`
	RequiredEvidence  []string `json:"required_evidence"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

type issueScanAgentPlanStep struct {
	ID                string   `json:"id"`
	StageID           string   `json:"stage_id"`
	Role              string   `json:"role"`
	CanOperate        bool     `json:"can_operate"`
	Objective         string   `json:"objective"`
	RequiredInputs    []string `json:"required_inputs"`
	RequiredOutputs   []string `json:"required_outputs"`
	AuthorityBoundary string   `json:"authority_boundary"`
	CompletionGate    string   `json:"completion_gate"`
}

// GitHubIssueCandidate is one open Transpara-AI repo issue found by a scanner.
// Scanner ordering is meaningful: QueueIssueScanRunLaunch selects the first
// candidate after validating the whole batch, so a deterministic scanner policy
// can rank candidates before this package records the run request.
type GitHubIssueCandidate struct {
	Repo        string
	Number      int
	Title       string
	URL         string
	Body        string
	State       string
	StateReason string
	Labels      []string
}

// IssueScanRunLaunchRequest records a scan result as a normal Hive run launch.
// DispatchQueuedRunLaunches then turns the queued launch into a FactoryOrder.
type IssueScanRunLaunchRequest struct {
	OperatorID     string
	IntakeID       string
	Issues         []GitHubIssueCandidate
	Authority      RunLaunchAuthority
	Budget         RunLaunchBudget
	ModelOverrides []ModelOverrideRequest
}

type IssueScanRunLaunchResult struct {
	RunID        string
	FirstEventID types.EventID
	Selected     GitHubIssueCandidate
	IntakeID     string
	Title        string
	TargetRepos  []string
}

// QueueIssueScanRunLaunch appends source.ingested, brief.derived, and
// factory.run.requested for the highest-ranked issue candidate. It records
// queued intent only; the existing run-launch dispatcher creates the Work task.
func QueueIssueScanRunLaunch(
	s store.Store,
	factory *event.EventFactory,
	signer event.Signer,
	human types.ActorID,
	conv types.ConversationID,
	req IssueScanRunLaunchRequest,
	modelSelection OperatorModelSelectionSource,
) (IssueScanRunLaunchResult, error) {
	if s == nil {
		return IssueScanRunLaunchResult{}, fmt.Errorf("store is required")
	}
	if factory == nil {
		return IssueScanRunLaunchResult{}, fmt.Errorf("event factory is required")
	}
	if signer == nil {
		return IssueScanRunLaunchResult{}, fmt.Errorf("signer is required")
	}
	raw, selected, err := buildIssueScanRunLaunchRequest(req)
	if err != nil {
		return IssueScanRunLaunchResult{}, err
	}
	launch, err := validateRunLaunchRequest(raw, modelSelection)
	if err != nil {
		return IssueScanRunLaunchResult{}, err
	}
	writer := &operatorRunLaunchWriter{factory: factory, signer: signer, human: human, conv: conv}
	result, err := appendRunLaunchEvents(s, writer, launch)
	if err != nil {
		return IssueScanRunLaunchResult{}, err
	}
	return IssueScanRunLaunchResult{
		RunID:        result.RunID,
		FirstEventID: result.FirstEventID,
		Selected:     selected,
		IntakeID:     launch.IntakeID,
		Title:        launch.Title,
		TargetRepos:  append([]string(nil), launch.TargetRepos...),
	}, nil
}

func buildIssueScanRunLaunchRequest(req IssueScanRunLaunchRequest) (operatorRunLaunchRequest, GitHubIssueCandidate, error) {
	operatorID := strings.TrimSpace(req.OperatorID)
	if operatorID == "" {
		return operatorRunLaunchRequest{}, GitHubIssueCandidate{}, fmt.Errorf("operator_id is required")
	}
	candidates, err := normalizeIssueScanCandidates(req.Issues)
	if err != nil {
		return operatorRunLaunchRequest{}, GitHubIssueCandidate{}, err
	}
	selected := candidates[0]
	intakeID := strings.TrimSpace(req.IntakeID)
	if intakeID == "" {
		intakeID = IssueScanIntakeID(selected)
	}
	authority := normalizeIssueScanAuthority(req.Authority)
	budget := req.Budget
	if budget.MaxIterations <= 0 {
		return operatorRunLaunchRequest{}, GitHubIssueCandidate{}, fmt.Errorf("budget.max_iterations must be greater than zero")
	}
	if budget.MaxCostUSD < 0 {
		return operatorRunLaunchRequest{}, GitHubIssueCandidate{}, fmt.Errorf("budget.max_cost_usd must be zero or greater")
	}
	brief, err := issueScanBriefJSON(candidates, selected)
	if err != nil {
		return operatorRunLaunchRequest{}, GitHubIssueCandidate{}, err
	}
	maxIterations := budget.MaxIterations
	maxCost := budget.MaxCostUSD
	return operatorRunLaunchRequest{
		OperatorID:     operatorID,
		IntakeID:       intakeID,
		Title:          issueScanTitle(selected),
		Brief:          brief,
		Sources:        issueScanSources(candidates),
		Authority:      authority,
		Budget:         runLaunchBudgetRequest{MaxIterations: &maxIterations, MaxCostUSD: &maxCost},
		ModelOverrides: append([]ModelOverrideRequest(nil), req.ModelOverrides...),
		TargetRepos:    []string{selected.Repo},
	}, selected, nil
}

func normalizeIssueScanCandidates(candidates []GitHubIssueCandidate) ([]GitHubIssueCandidate, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("issues is required")
	}
	out := make([]GitHubIssueCandidate, 0, len(candidates))
	// Validate every supplied candidate before filtering so malformed scanner
	// data fails closed even when mixed with a PR-ready issue.
	for i, candidate := range candidates {
		normalized := GitHubIssueCandidate{
			Repo:        strings.ToLower(strings.TrimSpace(candidate.Repo)),
			Number:      candidate.Number,
			Title:       strings.TrimSpace(candidate.Title),
			URL:         strings.TrimSpace(candidate.URL),
			Body:        normalizeIssueBody(candidate.Body),
			State:       strings.ToLower(strings.TrimSpace(candidate.State)),
			StateReason: strings.ToLower(strings.TrimSpace(candidate.StateReason)),
			Labels:      trimRunLaunchStrings(candidate.Labels),
		}
		if normalized.State == "" {
			normalized.State = "open"
		}
		switch {
		case !ValidTransparaAIRepo(normalized.Repo):
			return nil, fmt.Errorf("issues[%d].repo must be a transpara-ai owner/repo name", i)
		case normalized.Number <= 0:
			return nil, fmt.Errorf("issues[%d].number must be greater than zero", i)
		case normalized.Title == "":
			return nil, fmt.Errorf("issues[%d].title is required", i)
		case hasControlRune(normalized.Title) || hasControlRune(normalized.URL):
			return nil, fmt.Errorf("issues[%d] contains control characters", i)
		}
		if normalized.URL == "" {
			normalized.URL = fmt.Sprintf("https://github.com/%s/issues/%d", normalized.Repo, normalized.Number)
		}
		for _, label := range normalized.Labels {
			if hasControlRune(label) {
				return nil, fmt.Errorf("issues[%d].labels contains control characters", i)
			}
		}
		out = append(out, normalized)
	}
	var skippedNotPRReady int
	out, skippedNotPRReady = FilterIssueScanPRReadyCandidates(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("issues has no PR-ready candidates; skipped %d non-PR-ready issue(s); require %s without %s or %s", skippedNotPRReady, IssueScanPRReadyLabel, IssueScanPRDeferredLabel, IssueScanNeedsHumanScopeLabel)
	}
	rankIssueScanCandidates(out)
	return out, nil
}

// FilterIssueScanPRReadyCandidates keeps only issues that the change-control
// labels mark as ready for PR work.
func FilterIssueScanPRReadyCandidates(issues []GitHubIssueCandidate) ([]GitHubIssueCandidate, int) {
	out := make([]GitHubIssueCandidate, 0, len(issues))
	skipped := 0
	for _, issue := range issues {
		if IssueScanCandidatePRReady(issue) {
			out = append(out, issue)
			continue
		}
		skipped++
	}
	return out, skipped
}

// IssueScanCandidatePRReady reports whether a GitHub issue carries the
// change-control labels required before the issue-scan path may queue work.
func IssueScanCandidatePRReady(issue GitHubIssueCandidate) bool {
	labels := map[string]struct{}{}
	for _, label := range issue.Labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized != "" {
			labels[normalized] = struct{}{}
		}
	}
	if _, ok := labels[IssueScanPRReadyLabel]; !ok {
		return false
	}
	if _, ok := labels[IssueScanPRDeferredLabel]; ok {
		return false
	}
	if _, ok := labels[IssueScanNeedsHumanScopeLabel]; ok {
		return false
	}
	return true
}

func rankIssueScanCandidates(candidates []GitHubIssueCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return issueScanCandidateActionabilityScore(candidates[i]) > issueScanCandidateActionabilityScore(candidates[j])
	})
}

func issueScanCandidateActionabilityScore(issue GitHubIssueCandidate) int {
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		labels = append(labels, strings.ToLower(strings.TrimSpace(label)))
	}
	text := strings.ToLower(strings.Join(append([]string{issue.Title, issue.Body}, labels...), "\n"))
	score := 0
	for _, signal := range []struct {
		Needle string
		Weight int
	}{
		{"civilization", 90},
		{"issue-scan", 85},
		{"issue scan", 85},
		{"factoryorder", 80},
		{"factory order", 80},
		{"ready-for-human", 80},
		{"ready for human", 80},
		{"runtime visualization", 75},
		{"runtime evidence", 70},
		{"adversarial review", 70},
		{"zero blockers", 65},
		{"transpara-ai repos", 65},
		{"blocker", 60},
		{"regression", 55},
		{"failing", 50},
		{"failure", 50},
		{"bug", 45},
		{"security", 45},
		{"test", 25},
		{"implementation", 20},
	} {
		if strings.Contains(text, signal.Needle) {
			score += signal.Weight
		}
	}
	for _, label := range labels {
		switch label {
		case "duplicate", "wontfix", "invalid", "question", "discussion", "needs-info", "needs info", "blocked":
			score -= 100
		case "bug", "regression", "blocker", "security", "incident":
			score += 50
		case "civilization", "factory", "autonomy", "adversarial-review", "runtime-evidence":
			score += 45
		}
	}
	if strings.TrimSpace(issue.Body) != "" {
		score += 5
	}
	return score
}

// ValidTransparaAIRepo reports whether repo is a safe transpara-ai/owner repo
// slug accepted by the issue-scan intake path.
func ValidTransparaAIRepo(repo string) bool {
	if !validTargetRepo(repo) {
		return false
	}
	owner, _, ok := strings.Cut(repo, "/")
	return ok && owner == "transpara-ai"
}

func normalizeIssueScanAuthority(authority RunLaunchAuthority) RunLaunchAuthority {
	normalized := normalizeRunLaunchAuthority(authority)
	if strings.TrimSpace(string(normalized.InitialLevel)) == "" {
		normalized.InitialLevel = event.AuthorityLevelRequired
	}
	if normalized.Scope == "" {
		normalized.Scope = "transpara-ai issue scan to ready-for-Human PR; no merge or deploy"
	}
	if normalized.PolicyRef == "" {
		normalized.PolicyRef = IssueScanDefaultPolicyRef
	}
	if normalized.Rationale == "" {
		normalized.Rationale = "Civilization selected a Transpara-AI GitHub issue for governed factory execution."
	}
	return normalized
}

func issueScanTitle(issue GitHubIssueCandidate) string {
	title := fmt.Sprintf("Resolve %s#%d: %s", issue.Repo, issue.Number, issue.Title)
	return truncateRunLaunchText(title, 180)
}

func issueScanSources(candidates []GitHubIssueCandidate) []RunLaunchSource {
	sources := make([]RunLaunchSource, 0, len(candidates))
	for _, issue := range candidates {
		sources = append(sources, RunLaunchSource{
			ID:    issueScanSourceID(issue),
			Type:  issueScanSourceType,
			Ref:   issue.URL,
			Title: fmt.Sprintf("%s#%d: %s", issue.Repo, issue.Number, issue.Title),
		})
	}
	return sources
}

func issueScanBriefJSON(candidates []GitHubIssueCandidate, selected GitHubIssueCandidate) (json.RawMessage, error) {
	lifecycle := issueScanDevelopmentLifecycle()
	agentPlan, err := issueScanAgentExecutionPlan(lifecycle)
	if err != nil {
		return nil, err
	}
	brief := struct {
		Kind                              string                                            `json:"kind"`
		LifecycleVersion                  string                                            `json:"lifecycle_version"`
		SelectedIssue                     issueScanBriefIssuePayload                        `json:"selected_issue"`
		SelectionPolicy                   issueScanSelectionPolicyPayload                   `json:"selection_policy"`
		RoleSeparationPolicy              issueScanRoleSeparationPolicyPayload              `json:"role_separation_policy"`
		AutonomyGuardPolicy               issueScanAutonomyGuardPolicyPayload               `json:"autonomy_guard_policy"`
		HumanRequiredClassificationPolicy issueScanHumanRequiredClassificationPolicyPayload `json:"human_required_classification_policy"`
		AuthorityRecommendationPolicy     issueScanAuthorityRecommendationPolicyPayload     `json:"authority_recommendation_policy"`
		ValueAllocationBoundaryPolicy     issueScanValueAllocationBoundaryPolicyPayload     `json:"value_allocation_boundary_policy"`
		ScannedRepos                      []string                                          `json:"scanned_repos"`
		ScannedIssueCount                 int                                               `json:"scanned_issue_count"`
		CandidateIssues                   []issueScanBriefIssuePayload                      `json:"candidate_issues"`
		RequiredAgentFlow                 []string                                          `json:"required_agent_flow"`
		DevelopmentLifecycle              []issueScanLifecycleStage                         `json:"development_lifecycle"`
		AgentExecutionPlan                []issueScanAgentPlanStep                          `json:"agent_execution_plan"`
		AuthorityBoundaries               []string                                          `json:"authority_boundaries"`
	}{
		Kind:                              issueScanBriefKind,
		LifecycleVersion:                  issueScanLifecycleVersion,
		SelectedIssue:                     issueScanBriefIssue(selected, 1),
		SelectionPolicy:                   issueScanSelectionPolicy(candidates),
		RoleSeparationPolicy:              issueScanRoleSeparationPolicy(),
		AutonomyGuardPolicy:               issueScanAutonomyGuardPolicy(),
		HumanRequiredClassificationPolicy: issueScanHumanRequiredClassificationPolicy(),
		AuthorityRecommendationPolicy:     issueScanAuthorityRecommendationPolicy(),
		ValueAllocationBoundaryPolicy:     issueScanValueAllocationBoundaryPolicy(),
		ScannedRepos:                      issueScanRepos(candidates),
		ScannedIssueCount:                 len(candidates),
		RequiredAgentFlow:                 issueScanLifecycleFlow(lifecycle),
		DevelopmentLifecycle:              lifecycle,
		AgentExecutionPlan:                agentPlan,
		AuthorityBoundaries: []string{
			"no_merge",
			"no_deploy",
			"no_protected_action_without_recorded_authority",
			"ready_for_Human_result_PR_only",
		},
	}
	for i, issue := range candidates {
		brief.CandidateIssues = append(brief.CandidateIssues, issueScanBriefIssue(issue, i+1))
	}
	encoded, err := json.Marshal(brief)
	if err != nil {
		return nil, fmt.Errorf("marshal issue scan brief: %w", err)
	}
	return encoded, nil
}

func issueScanLifecycleFlow(lifecycle []issueScanLifecycleStage) []string {
	flow := make([]string, 0, len(lifecycle))
	for _, stage := range lifecycle {
		flow = append(flow, stage.ID)
	}
	return flow
}

func issueScanAgentExecutionPlan(lifecycle []issueScanLifecycleStage) ([]issueScanAgentPlanStep, error) {
	roles := StarterRoleDefinitions()
	steps := make([]issueScanAgentPlanStep, 0, 18)
	add := func(stageID, role string, inputs, outputs []string, objective string) error {
		stage, ok := issueScanLifecycleStageByID(lifecycle, stageID)
		if !ok {
			return fmt.Errorf("agent execution plan stage %q is not in lifecycle", stageID)
		}
		roleDef := roles[role]
		if roleDef == nil {
			return fmt.Errorf("agent execution plan role %q is not a starter role", role)
		}
		if !containsIssueScanString(stage.RequiredRoles, role) {
			return fmt.Errorf("agent execution plan role %q is not required for stage %q", role, stageID)
		}
		steps = append(steps, issueScanAgentPlanStep{
			ID:                fmt.Sprintf("%02d_%s_%s", len(steps)+1, stageID, role),
			StageID:           stageID,
			Role:              role,
			CanOperate:        roleDef.CanOperate,
			Objective:         objective,
			RequiredInputs:    append([]string(nil), inputs...),
			RequiredOutputs:   append([]string(nil), outputs...),
			AuthorityBoundary: stage.AuthorityBoundary,
			CompletionGate:    stage.CompletionGate,
		})
		return nil
	}
	additions := []struct {
		stageID   string
		role      string
		inputs    []string
		outputs   []string
		objective string
	}{
		{"research_issue_and_repo_context", "strategist", []string{"selected_issue", "candidate_issues"}, []string{"issue_priority_rationale", "risk_and_scope_notes"}, "Decide why this issue should enter the Civilization factory loop now."},
		{"research_issue_and_repo_context", "planner", []string{"selected_issue", "target_repo"}, []string{"repo_context_packet", "candidate_validation_commands"}, "Map the target repository context and identify the smallest viable validation surface."},
		{"debate_with_correct_civic_roles", "strategist", []string{"issue_priority_rationale", "repo_context_packet"}, []string{"strategist_position"}, "Argue the value, sequencing, and scope of the proposed work."},
		{"debate_with_correct_civic_roles", "planner", []string{"repo_context_packet"}, []string{"planner_position"}, "Argue the feasible implementation path and dependency order."},
		{"debate_with_correct_civic_roles", "reviewer", []string{"candidate_validation_commands"}, []string{"reviewer_position", "test_risk_notes"}, "Identify correctness risks and the review evidence needed before ready state."},
		{"debate_with_correct_civic_roles", "guardian", []string{"risk_and_scope_notes"}, []string{"guardian_position", "authority_boundary_disposition"}, "Identify authority, safety, and no-merge/no-deploy boundaries."},
		{"select_and_design_approach", "planner", []string{"role_positions"}, []string{"selected_approach", "definition_of_done", "implementation_task_plan"}, "Select the concrete approach and convert debate output into implementable work."},
		{"select_and_design_approach", "reviewer", []string{"selected_approach"}, []string{"acceptance_criteria", "test_plan"}, "Define the reviewable acceptance and validation criteria before implementation starts."},
		{"select_and_design_approach", "guardian", []string{"selected_approach", "authority_boundary_disposition"}, []string{"authority_gate_requirements"}, "Confirm implementation can proceed only inside the recorded authority envelope."},
		{"implement_on_branch", "implementer", []string{"implementation_task_plan", "acceptance_criteria"}, []string{"branch_name", "commit_sha", "validation_output"}, "Implement the selected approach on a branch and record exact validation evidence."},
		{"run_adversarial_review", "reviewer", []string{"commit_sha", "validation_output"}, []string{"exact_head_review_artifact"}, "Run an exact-head adversarial review and produce a durable review artifact."},
		{"run_adversarial_review", "guardian", []string{"exact_head_review_artifact"}, []string{"finding_disposition"}, "Classify findings against authority boundaries and reject or accept with evidence."},
		{"drive_blockers_to_zero", "implementer", []string{"accepted_findings"}, []string{"blocker_fixes", "rerun_validation"}, "Fix accepted blockers and rerun the relevant validation commands."},
		{"drive_blockers_to_zero", "reviewer", []string{"blocker_fixes", "rerun_validation"}, []string{"rerun_review"}, "Re-review the repaired exact head until blockers reach zero."},
		{"drive_blockers_to_zero", "guardian", []string{"rerun_review"}, []string{"zero_blocker_gate_disposition"}, "Confirm all accepted blockers are resolved or rejected with evidence."},
		{"surface_ready_for_Human_result_PR", "strategist", []string{"zero_blocker_gate_disposition"}, []string{"human_ready_summary"}, "Summarize the ready result for Human approval without merging or deploying."},
		{"surface_ready_for_Human_result_PR", "reviewer", []string{"ready_pr_url", "human_ready_summary"}, []string{"ready_state_review"}, "Run or verify the ready-state exact-head review evidence."},
		{"surface_ready_for_Human_result_PR", "guardian", []string{"ready_state_review"}, []string{"human_approval_boundary_check"}, "Confirm the result PR waits for Human approval and does not merge itself."},
	}
	for _, addition := range additions {
		if err := add(addition.stageID, addition.role, addition.inputs, addition.outputs, addition.objective); err != nil {
			return nil, err
		}
	}
	if err := validateIssueScanAgentPlanCoverage(lifecycle, steps); err != nil {
		return nil, err
	}
	return steps, nil
}

func validateIssueScanAgentPlanCoverage(lifecycle []issueScanLifecycleStage, steps []issueScanAgentPlanStep) error {
	byStage := map[string]map[string]struct{}{}
	for _, step := range steps {
		if byStage[step.StageID] == nil {
			byStage[step.StageID] = map[string]struct{}{}
		}
		if _, exists := byStage[step.StageID][step.Role]; exists {
			return fmt.Errorf("agent execution plan has duplicate role %q for stage %q", step.Role, step.StageID)
		}
		byStage[step.StageID][step.Role] = struct{}{}
	}
	for _, stage := range lifecycle {
		roles := byStage[stage.ID]
		if len(roles) != len(stage.RequiredRoles) {
			return fmt.Errorf("agent execution plan stage %q has %d roles, want %d", stage.ID, len(roles), len(stage.RequiredRoles))
		}
		for _, role := range stage.RequiredRoles {
			if _, ok := roles[role]; !ok {
				return fmt.Errorf("agent execution plan stage %q missing role %q", stage.ID, role)
			}
		}
	}
	return nil
}

func issueScanLifecycleStageByID(stages []issueScanLifecycleStage, id string) (issueScanLifecycleStage, bool) {
	for _, stage := range stages {
		if stage.ID == id {
			return stage, true
		}
	}
	return issueScanLifecycleStage{}, false
}

func containsIssueScanString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func issueScanDevelopmentLifecycle() []issueScanLifecycleStage {
	return []issueScanLifecycleStage{
		{
			ID:            "research_issue_and_repo_context",
			Name:          "Research issue and repo context",
			RequiredRoles: []string{"strategist", "planner"},
			RequiredEvidence: []string{
				"issue_snapshot",
				"repo_context",
				"risk_and_scope_notes",
			},
			AuthorityBoundary: "read_only",
			CompletionGate:    "context_packet_recorded",
		},
		{
			ID:            "debate_with_correct_civic_roles",
			Name:          "Debate with correct civic roles",
			RequiredRoles: []string{"strategist", "planner", "reviewer", "guardian"},
			RequiredEvidence: []string{
				"role_positions",
				"decision_record",
				"dissent_or_no_dissent_record",
			},
			AuthorityBoundary: "proposal_only_no_mutation",
			CompletionGate:    "decision_record_has_reviewer_and_guardian_disposition",
		},
		{
			ID:            "select_and_design_approach",
			Name:          "Select and design approach",
			RequiredRoles: []string{"planner", "reviewer", "guardian"},
			RequiredEvidence: []string{
				"selected_approach",
				"definition_of_done",
				"acceptance_criteria",
				"test_plan",
			},
			AuthorityBoundary: "implementation_waits_for_authorized_task",
			CompletionGate:    "implementation_task_has_readiness_artifacts",
		},
		{
			ID:            "implement_on_branch",
			Name:          "Implement on branch",
			RequiredRoles: []string{"implementer"},
			RequiredEvidence: []string{
				"branch_name",
				"commit_sha",
				"changed_files",
				"validation_output",
			},
			AuthorityBoundary: "no_merge_no_deploy",
			CompletionGate:    "implementation_changes_validated_on_branch",
		},
		{
			ID:            "run_adversarial_review",
			Name:          "Run adversarial review",
			RequiredRoles: []string{"reviewer", "guardian"},
			RequiredEvidence: []string{
				"exact_head_review_artifact",
				"finding_disposition",
			},
			AuthorityBoundary: "review_is_blocking",
			CompletionGate:    "review_artifact_returned_and_findings_classified",
		},
		{
			ID:            "drive_blockers_to_zero",
			Name:          "Drive blockers to zero",
			RequiredRoles: []string{"implementer", "reviewer", "guardian"},
			RequiredEvidence: []string{
				"blocker_fixes",
				"rerun_validation",
				"rerun_review",
			},
			AuthorityBoundary: "accepted_findings_resolved_or_rejected_with_evidence",
			CompletionGate:    "exact_head_review_has_zero_blockers",
		},
		{
			ID:            "surface_ready_for_Human_result_PR",
			Name:          "Surface ready-for-Human result PR",
			RequiredRoles: []string{"strategist", "reviewer", "guardian"},
			RequiredEvidence: []string{
				"ready_pr_url",
				"ready_state_review",
				"human_ready_summary",
			},
			AuthorityBoundary: "human_approval_required_no_merge",
			CompletionGate:    "ready_pr_has_exact_head_evidence_and_waits_for_human",
		},
	}
}

func issueScanDevelopmentLifecycleV02() ([]issueScanLifecycleStage, error) {
	stages := issueScanDevelopmentLifecycle()
	readyStage, ok := issueScanLifecycleStageByID(stages, "surface_ready_for_Human_result_PR")
	if !ok {
		return nil, fmt.Errorf("issue scan v0.2 lifecycle ready stage not found")
	}
	evidence := append([]string(nil), readyStage.RequiredEvidence...)
	for i, item := range evidence {
		if item == "ready_pr_url" {
			evidence[i] = "draft_pr_url"
		}
	}
	for i := range stages {
		if stages[i].ID == readyStage.ID {
			stages[i].RequiredEvidence = evidence
			break
		}
	}
	return stages, nil
}

func issueScanSelectionPolicy(candidates []GitHubIssueCandidate) issueScanSelectionPolicyPayload {
	return issueScanSelectionPolicyPayload{
		PolicyID:       "deterministic_actionability_rank_v0.2",
		SelectedRank:   1,
		CandidateCount: len(candidates),
		RankingInputs: []string{
			"validated_transpara_ai_repo_scope",
			"actionability_keyword_score",
			"actionability_label_score",
			"scanner_return_order_tie_breaker",
			"label_filters",
			"repo_scan_order",
		},
		Rationale: "Rank validated open Transpara-AI issues by explicit actionability signals before selecting rank 1; preserve all candidates and ranks for later civic debate.",
	}
}

func issueScanRoleSeparationPolicy() issueScanRoleSeparationPolicyPayload {
	return issueScanRoleSeparationPolicyPayload{
		PolicyID:      "civilization_issue_scan_role_separation_v0.1",
		PolicyVersion: "v0.1",
		ActorPolicies: []issueScanRoleSeparationActorPolicy{
			{
				ActorClass:         "scanner",
				CanOperateBranch:   false,
				CanMutateGitHub:    false,
				CanOpenPullRequest: false,
				CanApproveOrMerge:  false,
				AuthorityBoundary:  "read_only_issue_state",
				RequiredEvidence: []string{
					"validated_transpara_ai_repo_scope",
					"issue_snapshot",
					"cc_pr_ready_label_without_deferred_or_human_scope",
				},
				ForbiddenWithoutHuman: []string{
					"github_mutation",
					"branch_creation",
					"pull_request_creation",
					"runtime_execution",
				},
			},
			{
				ActorClass:         "recommender",
				MapsToStageRoles:   []string{"strategist", "planner"},
				CanOperateBranch:   false,
				CanMutateGitHub:    false,
				CanOpenPullRequest: false,
				CanApproveOrMerge:  false,
				AuthorityBoundary:  "recommendation_is_not_authority",
				RequiredEvidence: []string{
					"selection_policy",
					"issue_priority_rationale",
					"repo_context_packet",
				},
				ForbiddenWithoutHuman: []string{
					"implementation_start",
					"protected_action",
					"autonomy_increase",
				},
			},
			{
				ActorClass:         "implementer",
				MapsToStageRoles:   []string{"implementer"},
				CanOperateBranch:   true,
				CanMutateGitHub:    false,
				CanOpenPullRequest: false,
				CanApproveOrMerge:  false,
				AuthorityBoundary:  "branch_only_no_merge_no_deploy",
				RequiredEvidence: []string{
					"implementation_task_plan",
					"branch_name",
					"commit_sha",
					"validation_output",
				},
				ForbiddenWithoutHuman: []string{
					"default_branch_push",
					"merge",
					"deploy",
					"protected_action",
				},
			},
			{
				ActorClass:         "reviewer",
				MapsToStageRoles:   []string{"reviewer"},
				CanOperateBranch:   false,
				CanMutateGitHub:    false,
				CanOpenPullRequest: false,
				CanApproveOrMerge:  false,
				AuthorityBoundary:  "blocking_exact_head_review_only",
				RequiredEvidence: []string{
					"exact_head_review_artifact",
					"ready_state_review",
				},
				ForbiddenWithoutHuman: []string{
					"self_approval",
					"merge",
					"deploy",
				},
			},
			{
				ActorClass:         "guardian",
				MapsToStageRoles:   []string{"guardian"},
				CanOperateBranch:   false,
				CanMutateGitHub:    false,
				CanOpenPullRequest: false,
				CanApproveOrMerge:  false,
				AuthorityBoundary:  "authority_boundary_disposition_only",
				RequiredEvidence: []string{
					"authority_boundary_disposition",
					"finding_disposition",
					"human_approval_boundary_check",
				},
				ForbiddenWithoutHuman: []string{
					"protected_action_approval",
					"merge",
					"deploy",
				},
			},
			{
				ActorClass:         "human_authority",
				CanOperateBranch:   false,
				CanMutateGitHub:    true,
				CanOpenPullRequest: true,
				CanApproveOrMerge:  true,
				AuthorityBoundary:  "external_committee_or_current_human_delegation_required",
				RequiredEvidence: []string{
					"current_authority_record",
					"exact_head_cfar_pass",
					"green_validation",
				},
				ForbiddenWithoutHuman: []string{
					"hive_self_authorized_merge",
					"hive_self_authorized_deploy",
					"hive_self_authorized_autonomy_increase",
				},
			},
		},
		GlobalNonClaims: []string{
			"github_issue_is_not_authority",
			"recommendation_is_not_implementation_permission",
			"pull_request_is_not_merge_permission",
			"no_autonomy_increase",
			"no_test_001_green",
		},
	}
}

func issueScanAutonomyGuardPolicy() issueScanAutonomyGuardPolicyPayload {
	return issueScanAutonomyGuardPolicyPayload{
		PolicyID:              "civilization_issue_scan_autonomy_guard_v0.1",
		PolicyVersion:         "v0.1",
		RecommendationPosture: "recommendation_only_no_authority",
		AutonomyCeiling:       "level_0_no_autonomy_increase",
		EvidenceInputs: []string{
			"selected_issue_scope_evidence",
			"selected_issue_cc_labels",
			"selected_issue_authority_boundary",
			"role_separation_policy",
			"source_evidence",
		},
		FailClosedOutputs: []string{
			"recommendation_only",
			"human_scope_required",
			"protected_action_blocked",
			"autonomy_increase_blocked",
		},
		HumanScopeTriggers: []string{
			"protected_action",
			"autonomy_increase",
			"runtime_execution",
			"github_mutation",
			"eventgraph_write",
			"merge",
			"deploy",
			"value_allocation",
			"residual_risk_closure",
			"test_001_green",
			"docs_172_closure",
		},
		ForbiddenAuthorityClaims: []string{
			"issue_text_is_authority",
			"recommendation_authorizes_implementation",
			"recommendation_authorizes_protected_action",
			"recommendation_authorizes_runtime_execution",
			"recommendation_authorizes_autonomy_increase",
			"recommendation_authorizes_merge",
			"recommendation_closes_residual_risk",
			"recommendation_marks_test_001_green",
		},
	}
}

func issueScanHumanRequiredClassificationPolicy() issueScanHumanRequiredClassificationPolicyPayload {
	return issueScanHumanRequiredClassificationPolicyPayload{
		PolicyID:              "civilization_issue_scan_human_required_classification_v0.1",
		PolicyVersion:         "v0.1",
		ClassificationPosture: "human_required_for_protected_or_authority_sensitive_work",
		HumanRequiredTriggers: []string{
			"protected_action",
			"repo_settings_change",
			"runtime_execution",
			"deployment",
			"value_allocation",
			"authority_decision_required",
			"eventgraph_write",
			"secret_access",
			"test_001_green_claim",
			"docs_172_closure",
			"autonomy_increase",
			"residual_risk_closure",
		},
		EscalationOutputs: []string{
			"human_scope_required",
			"authority_decision_required",
			"issue_parking_required",
			"no_token_burn",
			"no_action_taken",
		},
		ForbiddenWithoutHuman: []string{
			"mutate_github",
			"create_or_merge_pr",
			"execute_runtime",
			"write_eventgraph_truth",
			"deploy",
			"allocate_value",
			"mark_test_001_green",
			"close_docs_172",
			"increase_autonomy",
			"close_residual_risk",
		},
		NonAuthorityClaims: []string{
			"classification_is_not_authority_decision",
			"issue_text_is_not_authority",
			"no_protected_action_execution",
			"no_github_mutation_authority",
			"no_runtime_execution_authority",
			"no_eventgraph_truth_write_authority",
		},
	}
}

func issueScanAuthorityRecommendationPolicy() issueScanAuthorityRecommendationPolicyPayload {
	return issueScanAuthorityRecommendationPolicyPayload{
		PolicyID:              "civilization_issue_scan_authority_recommendation_v0.1",
		PolicyVersion:         "v0.1",
		RecommendationPosture: "recommendation_only_not_authority_decision",
		DecisionBoundary:      "external_human_scoped_authority_decision_required",
		EvidenceInputs: []string{
			"selected_issue_scope_evidence",
			"selected_issue_cc_labels",
			"selected_issue_pr_ready_state",
			"selected_issue_source_evidence",
			"selected_issue_authority_boundary",
			"role_separation_policy",
			"autonomy_guard_policy",
			"human_required_classification_policy",
		},
		RecommendationOutputs: []string{
			"recommendation_only",
			"no_authority_decision",
			"human_authority_required",
			"blocked_until_authority_packet",
			"no_write_no_action",
		},
		RequiredHumanAuthority: []string{
			"external_authority_decision_ref",
			"exact_scope_packet_ref",
			"protected_action_authority_ref",
			"human_approval_ref",
		},
		ForbiddenAuthorityClaims: []string{
			"issue_text_authorizes_work",
			"pr_ready_label_authorizes_protected_action",
			"recommendation_is_authority_decision",
			"recommendation_authorizes_github_mutation",
			"recommendation_authorizes_runtime_execution",
			"recommendation_authorizes_eventgraph_write",
			"recommendation_authorizes_merge",
			"recommendation_marks_test_001_green",
			"recommendation_closes_docs_172",
			"recommendation_increases_autonomy",
			"recommendation_allocates_value",
			"recommendation_closes_residual_risk",
		},
	}
}

func issueScanValueAllocationBoundaryPolicy() issueScanValueAllocationBoundaryPolicyPayload {
	return issueScanValueAllocationBoundaryPolicyPayload{
		PolicyID:        "civilization_issue_scan_value_allocation_boundary_v0.1",
		PolicyVersion:   "v0.1",
		BoundaryPosture: "deny_by_default_human_required",
		DisplayMode:     "pull_display_only_not_routing_or_action",
		ClassificationOutputs: []string{
			"value_allocation_candidate",
			"human_required",
			"display_only",
			"no_allocation_authority",
		},
		RequiredHumanAuthority: []string{
			"external_committee_value_allocation_decision",
			"issue_scoped_authority_packet",
			"human_approval_ref",
		},
		ForbiddenSystemActions: []string{
			"allocate_value",
			"approve_value_allocation",
			"route_value_allocation_candidate",
			"assign_value_allocation_candidate",
			"notify_as_action",
			"prioritize_scarce_resources",
			"bill_or_pay",
			"create_entitlement",
			"set_pricing",
			"compensate_or_grant_equity",
			"create_external_commitment",
		},
		ForbiddenAuthorityClaims: []string{
			"display_authorizes_allocation",
			"classification_authorizes_allocation",
			"candidate_status_authorizes_routing",
			"issue_text_authorizes_value_allocation",
			"human_required_label_is_approval",
		},
		NonAuthorityClaims: []string{
			"display_is_not_authority_decision",
			"classification_is_not_authority_decision",
			"no_value_allocation_execution",
			"no_automated_routing",
			"no_notification_as_action",
			"no_runtime_execution_authority",
		},
	}
}

func issueScanBriefIssue(issue GitHubIssueCandidate, rank int) issueScanBriefIssuePayload {
	return issueScanBriefIssuePayload{
		Rank:        rank,
		Repo:        issue.Repo,
		Number:      issue.Number,
		Title:       issue.Title,
		URL:         issue.URL,
		State:       issue.State,
		StateReason: issue.StateReason,
		Labels:      append([]string(nil), issue.Labels...),
		Body:        truncateRunLaunchText(issue.Body, 1200),
	}
}

func issueScanRepos(candidates []GitHubIssueCandidate) []string {
	seen := make(map[string]struct{}, len(candidates))
	var repos []string
	for _, issue := range candidates {
		if _, ok := seen[issue.Repo]; ok {
			continue
		}
		seen[issue.Repo] = struct{}{}
		repos = append(repos, issue.Repo)
	}
	return repos
}

func normalizeIssueBody(body string) string {
	body = strings.TrimSpace(body)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			switch r {
			case '\n', '\r', '\t':
				return r
			default:
				return -1
			}
		}
		return r
	}, body)
}

func issueScanSourceID(issue GitHubIssueCandidate) string {
	return "github_issue_" + safeRunLaunchID(fmt.Sprintf("%s_%d", issue.Repo, issue.Number))
}

// IssueScanIntakeID derives a stable, safe intake id for one selected issue.
func IssueScanIntakeID(issue GitHubIssueCandidate) string {
	return "intake_" + issueScanSourceID(issue)
}

// IssueScanOperatorID derives a safe operator id from a human/operator label.
func IssueScanOperatorID(label string) string {
	id := safeRunLaunchID(label)
	if id == "" {
		return "operator"
	}
	return "operator_" + id
}

func safeRunLaunchID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		var next rune
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			next = r
		case r == '-' || r == '_' || r == '.' || r == '/' || unicode.IsSpace(r):
			next = '_'
		default:
			continue
		}
		if next == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		b.WriteRune(next)
	}
	return strings.Trim(b.String(), "_")
}

func truncateRunLaunchText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}
