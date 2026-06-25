package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
)

type issueScanIssueLister interface {
	ListIssues(ctx context.Context, repo string, limit int, labels []string) ([]hive.GitHubIssueCandidate, error)
}

type ghIssueLister struct{}

func (ghIssueLister) ListIssues(ctx context.Context, repo string, limit int, labels []string) ([]hive.GitHubIssueCandidate, error) {
	return scanGitHubRepoIssues(ctx, repo, limit, labels)
}

type issueScanScannerConfig struct {
	OperatorID     string
	Repos          []string
	Labels         []string
	Limit          int
	MaxIterations  int
	MaxCostUSD     float64
	MaxNewRuns     int
	Interval       time.Duration
	AuthorityScope string
}

type issueScanScannerCycleResult struct {
	ScannedIssues     int
	SkippedExisting   int
	SkippedNotPRReady int
	Queued            bool
	QueuedRunID       string
	QueuedIssue       hive.GitHubIssueCandidate
}

func scanGitHubIssuesWith(ctx context.Context, repos []string, limit int, labels []string, lister issueScanIssueLister) ([]hive.GitHubIssueCandidate, error) {
	if lister == nil {
		lister = ghIssueLister{}
	}
	var out []hive.GitHubIssueCandidate
	for _, repo := range repos {
		issues, err := lister.ListIssues(ctx, repo, limit, labels)
		if err != nil {
			return nil, err
		}
		out = append(out, issues...)
	}
	return out, nil
}

func runIssueScanScannerLoop(ctx context.Context, fc *factoryContext, config issueScanScannerConfig, lister issueScanIssueLister) {
	if config.Interval <= 0 {
		return
	}
	queuedRuns := 0
	run := func() {
		if config.MaxNewRuns > 0 && queuedRuns >= config.MaxNewRuns {
			fmt.Fprintf(os.Stderr, "Issue-scan scanner: max new run cap reached (%d); no scan attempted\n", config.MaxNewRuns)
			return
		}
		result, err := runIssueScanScannerCycle(ctx, fc, config, lister)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: issue-scan scanner failed closed: %v\n", err)
			return
		}
		if result.Queued {
			queuedRuns++
			fmt.Fprintf(os.Stderr, "Issue-scan scanner: queued %s for %s#%d, skipped %d existing issue(s), and skipped %d non-PR-ready issue(s)\n", result.QueuedRunID, result.QueuedIssue.Repo, result.QueuedIssue.Number, result.SkippedExisting, result.SkippedNotPRReady)
			return
		}
		fmt.Fprintf(os.Stderr, "Issue-scan scanner: scanned %d issue(s); skipped %d non-PR-ready issue(s); no new issue-scan FactoryOrder queued\n", result.ScannedIssues, result.SkippedNotPRReady)
	}

	run()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func runIssueScanScannerCycle(ctx context.Context, fc *factoryContext, config issueScanScannerConfig, lister issueScanIssueLister) (issueScanScannerCycleResult, error) {
	var result issueScanScannerCycleResult
	if fc == nil || fc.store == nil || fc.factory == nil || fc.signer == nil {
		return result, fmt.Errorf("factory context is required")
	}
	if config.OperatorID == "" {
		return result, fmt.Errorf("operator_id is required")
	}
	if len(config.Repos) == 0 {
		return result, fmt.Errorf("issue-scan repos are required")
	}
	if config.Limit <= 0 {
		return result, fmt.Errorf("issue-scan limit must be greater than zero")
	}
	if config.MaxIterations <= 0 {
		return result, fmt.Errorf("issue-scan max_iterations must be greater than zero")
	}
	if config.MaxCostUSD < 0 {
		return result, fmt.Errorf("issue-scan max_cost_usd must be zero or greater")
	}
	if config.MaxNewRuns < 0 {
		return result, fmt.Errorf("issue-scan max_new_runs must be zero or greater")
	}

	issues, err := scanGitHubIssuesWith(ctx, config.Repos, config.Limit, config.Labels, lister)
	if err != nil {
		return result, err
	}
	result.ScannedIssues = len(issues)
	if len(issues) == 0 {
		return result, nil
	}
	issues, result.SkippedNotPRReady = filterIssueScanPRReadyCandidates(issues)
	if len(issues) == 0 {
		return result, nil
	}
	existing, err := issueScanRequestedDedupeKeys(fc.store)
	if err != nil {
		return result, err
	}
	candidates := make([]hive.GitHubIssueCandidate, 0, len(issues))
	for _, issue := range issues {
		if issueScanCandidateAlreadyRequested(issue, existing) {
			result.SkippedExisting++
			continue
		}
		candidates = append(candidates, issue)
	}
	if len(candidates) == 0 {
		return result, nil
	}

	scope := strings.TrimSpace(config.AuthorityScope)
	if scope == "" {
		scope = "transpara-ai issue scan to ready-for-Human PR; no merge or deploy"
	}
	queued, err := hive.QueueIssueScanRunLaunch(fc.store, fc.factory, fc.signer, fc.humanID, factoryOrderConversation("issue_scan_scanner"), hive.IssueScanRunLaunchRequest{
		OperatorID: config.OperatorID,
		Issues:     candidates,
		Authority: hive.RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        scope,
			PolicyRef:    hive.IssueScanDefaultPolicyRef,
			Rationale:    "Civilization daemon scanned Transpara-AI GitHub issues and selected one for governed factory execution.",
		},
		Budget: hive.RunLaunchBudget{MaxIterations: config.MaxIterations, MaxCostUSD: config.MaxCostUSD},
	}, nil)
	if err != nil {
		return result, fmt.Errorf("queue issue-scan run launch: %w", err)
	}
	result.Queued = true
	result.QueuedRunID = queued.RunID
	result.QueuedIssue = queued.Selected
	return result, nil
}

func filterIssueScanPRReadyCandidates(issues []hive.GitHubIssueCandidate) ([]hive.GitHubIssueCandidate, int) {
	out := make([]hive.GitHubIssueCandidate, 0, len(issues))
	skipped := 0
	for _, issue := range issues {
		if issueScanCandidatePRReady(issue) {
			out = append(out, issue)
			continue
		}
		skipped++
	}
	return out, skipped
}

func issueScanCandidatePRReady(issue hive.GitHubIssueCandidate) bool {
	labels := map[string]struct{}{}
	for _, label := range issue.Labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized != "" {
			labels[normalized] = struct{}{}
		}
	}
	if _, ok := labels["cc:pr-ready"]; !ok {
		return false
	}
	if _, ok := labels["cc:pr-deferred"]; ok {
		return false
	}
	if _, ok := labels["cc:needs-human-scope"]; ok {
		return false
	}
	return true
}

func issueScanCandidateAlreadyRequested(issue hive.GitHubIssueCandidate, existing map[string]struct{}) bool {
	for _, key := range issueScanCandidateDedupeKeys(issue) {
		if _, ok := existing[key]; ok {
			return true
		}
	}
	return false
}

func issueScanCandidateDedupeKeys(issue hive.GitHubIssueCandidate) []string {
	keys := []string{}
	if intakeID := strings.TrimSpace(hive.IssueScanIntakeID(issue)); intakeID != "" {
		keys = append(keys, "intake:"+intakeID)
	}
	if key := issueScanRepoNumberKey(issue.Repo, issue.Number); key != "" {
		keys = append(keys, "issue:"+key)
	}
	return keys
}

func issueScanRequestedDedupeKeys(s store.Store) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if s == nil {
		return out, nil
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(hive.EventTypeFactoryRunRequested, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch factory.run.requested events: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(hive.FactoryRunRequestedContent)
			if !ok || !factoryRunRequestedIsIssueScan(content) {
				continue
			}
			addIssueScanRequestedDedupeKeys(out, content)
		}
		if !page.HasMore() {
			return out, nil
		}
		cursor = page.Cursor()
	}
}

func addIssueScanRequestedDedupeKeys(out map[string]struct{}, content hive.FactoryRunRequestedContent) {
	if out == nil {
		return
	}
	if intakeID := strings.TrimSpace(content.IntakeID); intakeID != "" {
		out["intake:"+intakeID] = struct{}{}
	}
	for _, source := range content.Sources {
		if key := issueScanSourceIssueKey(source); key != "" {
			out["issue:"+key] = struct{}{}
		}
	}
}

func issueScanSourceIssueKey(source hive.RunLaunchSource) string {
	if strings.TrimSpace(source.Type) != "github.issue" {
		return ""
	}
	if key := issueScanIssueKeyFromURL(source.Ref); key != "" {
		return key
	}
	if key := issueScanIssueKeyFromSourceID(source.ID); key != "" {
		return key
	}
	return ""
}

func issueScanIssueKeyFromURL(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimRight(value, "/")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "github.com/")
	parts := strings.Split(value, "/")
	if len(parts) != 4 || parts[2] != "issues" {
		return ""
	}
	number, ok := issueScanPositiveNumber(parts[3])
	if !ok {
		return ""
	}
	return issueScanRepoNumberKey(parts[0]+"/"+parts[1], number)
}

func issueScanIssueKeyFromSourceID(id string) string {
	value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "github_issue_")
	parts := strings.Split(value, "_")
	if len(parts) < 3 || parts[0] != "transpara" || parts[1] != "ai" {
		return ""
	}
	number, ok := issueScanPositiveNumber(parts[len(parts)-1])
	if !ok {
		return ""
	}
	repo := "transpara-ai/" + strings.Join(parts[2:len(parts)-1], "_")
	return issueScanRepoNumberKey(repo, number)
}

func issueScanRepoNumberKey(repo string, number int) string {
	repo = strings.ToLower(strings.TrimSpace(repo))
	if !hive.ValidTransparaAIRepo(repo) || number <= 0 {
		return ""
	}
	return fmt.Sprintf("%s#%d", repo, number)
}

func issueScanPositiveNumber(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var number int
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		number = number*10 + int(r-'0')
		if number <= 0 {
			return 0, false
		}
	}
	return number, number > 0
}

func factoryRunRequestedIsIssueScan(content hive.FactoryRunRequestedContent) bool {
	raw := strings.TrimSpace(string(content.Brief))
	if raw == "" || raw[0] != '{' {
		return false
	}
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return false
	}
	return strings.TrimSpace(meta.Kind) == "transpara_ai_github_issue_scan"
}
