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

// GitHubIssueCandidate is one open Transpara-AI repo issue found by a scanner.
// Scanner ordering is meaningful: QueueIssueScanRunLaunch selects the first
// valid candidate so a deterministic scanner policy can rank candidates before
// this package records the run request.
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
	brief := struct {
		Kind                string                       `json:"kind"`
		SelectedIssue       issueScanBriefIssuePayload   `json:"selected_issue"`
		ScannedRepos        []string                     `json:"scanned_repos"`
		ScannedIssueCount   int                          `json:"scanned_issue_count"`
		CandidateIssues     []issueScanBriefIssuePayload `json:"candidate_issues"`
		RequiredAgentFlow   []string                     `json:"required_agent_flow"`
		AuthorityBoundaries []string                     `json:"authority_boundaries"`
	}{
		Kind:              "transpara_ai_github_issue_scan",
		SelectedIssue:     issueScanBriefIssue(selected),
		ScannedRepos:      issueScanRepos(candidates),
		ScannedIssueCount: len(candidates),
		RequiredAgentFlow: []string{
			"research_issue_and_repo_context",
			"debate_with_correct_civic_roles",
			"select_and_design_approach",
			"implement_on_branch",
			"run_adversarial_review",
			"drive_blockers_to_zero",
			"surface_ready_for_Human_result_PR",
		},
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
