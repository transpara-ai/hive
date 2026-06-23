package hive

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const (
	IssueScanDefaultPolicyRef = "civilization/repo-issue-scan-to-factory-order-v0.1"
	issueScanBriefKind        = "transpara_ai_github_issue_scan"
	issueScanLifecycleVersion = "civilization_issue_to_human_ready_pr_v0.3"
	issueScanSourceType       = "github.issue"
)

type issueScanBriefIssuePayload struct {
	Repo   string   `json:"repo"`
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Labels []string `json:"labels,omitempty"`
	Body   string   `json:"body_excerpt,omitempty"`
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
	Repo   string
	Number int
	Title  string
	URL    string
	Body   string
	Labels []string
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
	for i, candidate := range candidates {
		normalized := GitHubIssueCandidate{
			Repo:   strings.ToLower(strings.TrimSpace(candidate.Repo)),
			Number: candidate.Number,
			Title:  strings.TrimSpace(candidate.Title),
			URL:    strings.TrimSpace(candidate.URL),
			Body:   normalizeIssueBody(candidate.Body),
			Labels: trimRunLaunchStrings(candidate.Labels),
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
	return out, nil
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
		Kind                 string                       `json:"kind"`
		LifecycleVersion     string                       `json:"lifecycle_version"`
		SelectedIssue        issueScanBriefIssuePayload   `json:"selected_issue"`
		ScannedRepos         []string                     `json:"scanned_repos"`
		ScannedIssueCount    int                          `json:"scanned_issue_count"`
		CandidateIssues      []issueScanBriefIssuePayload `json:"candidate_issues"`
		RequiredAgentFlow    []string                     `json:"required_agent_flow"`
		DevelopmentLifecycle []issueScanLifecycleStage    `json:"development_lifecycle"`
		AgentExecutionPlan   []issueScanAgentPlanStep     `json:"agent_execution_plan"`
		AuthorityBoundaries  []string                     `json:"authority_boundaries"`
	}{
		Kind:                 issueScanBriefKind,
		LifecycleVersion:     issueScanLifecycleVersion,
		SelectedIssue:        issueScanBriefIssue(selected),
		ScannedRepos:         issueScanRepos(candidates),
		ScannedIssueCount:    len(candidates),
		RequiredAgentFlow:    issueScanLifecycleFlow(lifecycle),
		DevelopmentLifecycle: lifecycle,
		AgentExecutionPlan:   agentPlan,
		AuthorityBoundaries: []string{
			"no_merge",
			"no_deploy",
			"no_protected_action_without_recorded_authority",
			"ready_for_Human_result_PR_only",
		},
	}
	for _, issue := range candidates {
		brief.CandidateIssues = append(brief.CandidateIssues, issueScanBriefIssue(issue))
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
	return steps, nil
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
				"draft_pr_url",
				"ready_state_review",
				"human_ready_summary",
			},
			AuthorityBoundary: "human_approval_required_no_merge",
			CompletionGate:    "ready_pr_has_exact_head_evidence_and_waits_for_human",
		},
	}
}

func issueScanBriefIssue(issue GitHubIssueCandidate) issueScanBriefIssuePayload {
	return issueScanBriefIssuePayload{
		Repo:   issue.Repo,
		Number: issue.Number,
		Title:  issue.Title,
		URL:    issue.URL,
		Labels: append([]string(nil), issue.Labels...),
		Body:   truncateRunLaunchText(issue.Body, 1200),
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
