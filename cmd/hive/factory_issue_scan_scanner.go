package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/work"
)

type issueScanIssueLister interface {
	ListIssues(ctx context.Context, repo string, limit int, labels []string) ([]hive.GitHubIssueCandidate, error)
}

type ghIssueLister struct{}

func (ghIssueLister) ListIssues(ctx context.Context, repo string, limit int, labels []string) ([]hive.GitHubIssueCandidate, error) {
	return scanGitHubRepoIssues(ctx, repo, limit, labels)
}

type issueScanScannerConfig struct {
	OperatorID           string
	Repos                []string
	Labels               []string
	Limit                int
	MaxIterations        int
	MaxCostUSD           float64
	MaxNewRuns           int
	MaxDuration          time.Duration
	KillSwitchPath       string
	OneActive            bool
	ReviewQueueThreshold int
	ReviewQueueInspector issueScanReviewQueueInspector
	Interval             time.Duration
	AuthorityScope       string
}

type issueScanScannerCycleResult struct {
	ScannedIssues        int
	SkippedExisting      int
	SkippedNotPRReady    int
	SkippedActive        int
	SkippedReviewQueue   int
	ReviewQueueThreshold int
	ReviewQueueEventID   string
	Queued               bool
	QueuedRunID          string
	QueuedIssue          hive.GitHubIssueCandidate
}

var errIssueScanKillSwitchActive = errors.New("issue-scan kill switch active")

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
	if config.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.MaxDuration)
		defer cancel()
	}
	queuedRuns := 0
	run := func() bool {
		if err := issueScanKillSwitchError(config.KillSwitchPath); err != nil {
			if errors.Is(err, errIssueScanKillSwitchActive) {
				fmt.Fprintf(os.Stderr, "Issue-scan scanner: halted by kill switch: %v\n", err)
				return false
			}
			fmt.Fprintf(os.Stderr, "WARNING: issue-scan scanner skipped this tick while checking kill switch: %v\n", err)
			return true
		}
		if config.MaxNewRuns > 0 && queuedRuns >= config.MaxNewRuns {
			fmt.Fprintf(os.Stderr, "Issue-scan scanner: max new run cap reached (%d); no scan attempted\n", config.MaxNewRuns)
			return true
		}
		result, err := runIssueScanScannerCycle(ctx, fc, config, lister)
		if err != nil {
			if errors.Is(err, errIssueScanKillSwitchActive) {
				fmt.Fprintf(os.Stderr, "Issue-scan scanner: halted by kill switch: %v\n", err)
				return false
			}
			fmt.Fprintf(os.Stderr, "WARNING: issue-scan scanner failed closed: %v\n", err)
			return true
		}
		if result.Queued {
			queuedRuns++
			fmt.Fprintf(os.Stderr, "Issue-scan scanner: queued %s for %s#%d, skipped %d existing issue(s), and skipped %d non-PR-ready issue(s)\n", result.QueuedRunID, result.QueuedIssue.Repo, result.QueuedIssue.Number, result.SkippedExisting, result.SkippedNotPRReady)
			return true
		}
		if result.SkippedActive > 0 {
			fmt.Fprintf(os.Stderr, "Issue-scan scanner: one-active guard skipped scan while %d issue-scan run(s) remain active\n", result.SkippedActive)
			return true
		}
		if result.SkippedReviewQueue > 0 {
			eventSuffix := ""
			if result.ReviewQueueEventID != "" {
				eventSuffix = fmt.Sprintf(" (event %s)", result.ReviewQueueEventID)
			}
			fmt.Fprintf(os.Stderr, "Issue-scan scanner: review-capacity throttle skipped work-start while %d open PR(s) are counted as unproven exact-head review load (threshold %d)%s\n", result.SkippedReviewQueue, result.ReviewQueueThreshold, eventSuffix)
			return true
		}
		fmt.Fprintf(os.Stderr, "Issue-scan scanner: scanned %d issue(s); skipped %d non-PR-ready issue(s); no new issue-scan FactoryOrder queued\n", result.ScannedIssues, result.SkippedNotPRReady)
		return true
	}

	if !run() {
		return
	}
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if config.MaxDuration > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "Issue-scan scanner: hard duration cap reached after %s; stopping scanner loop\n", config.MaxDuration)
			}
			return
		case <-ticker.C:
			if !run() {
				return
			}
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
	if config.MaxDuration < 0 {
		return result, fmt.Errorf("issue-scan max_duration must be zero or greater")
	}
	if config.ReviewQueueThreshold < 0 {
		return result, fmt.Errorf("issue-scan review queue threshold must be zero or greater")
	}
	reviewQueueThreshold := config.ReviewQueueThreshold
	if reviewQueueThreshold == 0 {
		reviewQueueThreshold = issueScanDefaultReviewQueueThreshold
	}
	if err := issueScanKillSwitchError(config.KillSwitchPath); err != nil {
		return result, err
	}
	if config.OneActive {
		active, err := issueScanActiveRunIDs(fc.store)
		if err != nil {
			return result, err
		}
		if len(active) > 0 {
			result.SkippedActive = len(active)
			return result, nil
		}
	}
	if reviewQueueThreshold > 0 {
		decision, err := issueScanReviewQueueThrottle(ctx, config.Repos, reviewQueueThreshold, config.ReviewQueueInspector)
		if err != nil {
			return result, err
		}
		result.ReviewQueueThreshold = decision.Threshold
		if decision.Throttled {
			result.SkippedReviewQueue = decision.OpenPRCount
			eventID, err := recordIssueScanReviewCapacityThrottled(fc, config.OperatorID, config.Repos, decision)
			if err != nil {
				return result, err
			}
			result.ReviewQueueEventID = eventID.Value()
			return result, nil
		}
	}

	issues, err := scanGitHubIssuesWith(ctx, config.Repos, config.Limit, config.Labels, lister)
	if err != nil {
		return result, err
	}
	result.ScannedIssues = len(issues)
	if len(issues) == 0 {
		return result, nil
	}
	issues, result.SkippedNotPRReady = hive.FilterIssueScanPRReadyCandidates(issues)
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

func recordIssueScanReviewCapacityThrottled(fc *factoryContext, operatorID string, repos []string, decision issueScanReviewQueueDecision) (types.EventID, error) {
	if fc == nil || fc.store == nil || fc.factory == nil || fc.signer == nil {
		return types.EventID{}, fmt.Errorf("factory context is required")
	}
	content := hive.IssueScanReviewCapacityThrottledContent{
		OperatorID:   strings.TrimSpace(operatorID),
		Repos:        append([]string(nil), repos...),
		Threshold:    decision.Threshold,
		OpenPRCount:  decision.OpenPRCount,
		Reason:       "review capacity is full; refusing new issue-scan work-start until exact-head human review queue drops below threshold",
		SourceRefs:   append([]string(nil), decision.SourceRefs...),
		PullRequests: issueScanReviewQueueEventRefs(decision.PullRequests),
		ThrottledBy:  fc.humanID,
	}
	ev, err := fc.factory.Create(hive.EventTypeIssueScanReviewCapacityThrottled, fc.humanID, content, fc.headCauses(), factoryOrderConversation("issue_scan_review_capacity_throttle"), fc.store, fc.signer)
	if err != nil {
		return types.EventID{}, fmt.Errorf("create issue-scan review-capacity throttle event: %w", err)
	}
	stored, err := fc.store.Append(ev)
	if err != nil {
		return types.EventID{}, fmt.Errorf("append issue-scan review-capacity throttle event: %w", err)
	}
	return stored.ID(), nil
}

func issueScanKillSwitchError(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: %s", errIssueScanKillSwitchActive, path)
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return fmt.Errorf("check issue-scan kill switch %s: %w", path, err)
	}
}

func issueScanActiveRunIDs(s store.Store) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	parked, err := issueScanParkedRunIDs(s)
	if err != nil {
		return nil, err
	}
	terminal, err := issueScanTerminalRunIDs(s)
	if err != nil {
		return nil, err
	}
	active := map[string]struct{}{}
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
			runID := strings.TrimSpace(content.RunID)
			if runID == "" || parked[runID] || terminal[runID] {
				continue
			}
			active[runID] = struct{}{}
		}
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	out := make([]string, 0, len(active))
	for runID := range active {
		out = append(out, runID)
	}
	sort.Strings(out)
	return out, nil
}

func issueScanParkedRunIDs(s store.Store) (map[string]bool, error) {
	out := map[string]bool{}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(hive.EventTypeIssueScanRunParked, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch hive.issuescan.run.parked events: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(hive.IssueScanRunParkedContent)
			if !ok {
				continue
			}
			if runID := strings.TrimSpace(content.RunID); runID != "" {
				out[runID] = true
			}
		}
		if !page.HasMore() {
			return out, nil
		}
		cursor = page.Cursor()
	}
}

func issueScanTerminalRunIDs(s store.Store) (map[string]bool, error) {
	out := map[string]bool{}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(work.EventTypeTaskArtifact, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch work.task.artifact events: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(work.TaskArtifactContent)
			if !ok {
				continue
			}
			if strings.TrimSpace(content.Label) != hive.IssueScanReadyPREvidenceArtifactLabel {
				continue
			}
			runID, err := issueScanTerminalReadyEvidenceRunID(content.Body)
			if err != nil {
				return nil, fmt.Errorf("parse terminal ready evidence artifact %s: %w", ev.ID().Value(), err)
			}
			if runID != "" {
				out[runID] = true
			}
		}
		if !page.HasMore() {
			return out, nil
		}
		cursor = page.Cursor()
	}
}

func issueScanTerminalReadyEvidenceRunID(body string) (string, error) {
	raw := strings.TrimSpace(body)
	if raw == "" {
		return "", fmt.Errorf("%s artifact has empty body", hive.IssueScanReadyPREvidenceArtifactLabel)
	}
	var payload struct {
		Kind  string `json:"kind"`
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", fmt.Errorf("decode %s artifact body: %w", hive.IssueScanReadyPREvidenceArtifactLabel, err)
	}
	if kind := strings.TrimSpace(payload.Kind); kind != "" && kind != hive.IssueScanReadyPREvidenceArtifactLabel {
		return "", fmt.Errorf("kind %q does not match %q", kind, hive.IssueScanReadyPREvidenceArtifactLabel)
	}
	return strings.TrimSpace(payload.RunID), nil
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
