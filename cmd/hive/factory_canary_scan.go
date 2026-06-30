package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
)

type level1CanaryReport struct {
	Kind           string                    `json:"kind"`
	GeneratedAt    string                    `json:"generated_at"`
	Repos          []string                  `json:"repos"`
	Limit          int                       `json:"limit"`
	MaxDuration    string                    `json:"max_duration"`
	MaxCostUSD     float64                   `json:"max_cost_usd"`
	ScannedIssues  int                       `json:"scanned_issues"`
	PRReadyIssues  int                       `json:"pr_ready_issues"`
	ParkedIssues   int                       `json:"parked_issues"`
	AlreadyParked  int                       `json:"already_parked"`
	EventRefs      []string                  `json:"event_refs,omitempty"`
	Issues         []level1CanaryIssueResult `json:"issues"`
	Boundary       []string                  `json:"boundary"`
	StopConditions []string                  `json:"stop_conditions"`
}

type level1CanaryIssueResult struct {
	Repo           string   `json:"repo"`
	Number         int      `json:"number"`
	URL            string   `json:"url,omitempty"`
	Title          string   `json:"title,omitempty"`
	State          string   `json:"state,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	Classification string   `json:"classification"`
	BlockerType    string   `json:"blocker_type,omitempty"`
	RequiredAction string   `json:"required_action,omitempty"`
	RunID          string   `json:"run_id,omitempty"`
	EventRef       string   `json:"event_ref,omitempty"`
	AlreadyParked  bool     `json:"already_parked,omitempty"`
}

type level1CanaryReportOptions struct {
	Repos        []string
	Limit        int
	MaxDuration  time.Duration
	MaxCostUSD   float64
	GeneratedAt  time.Time
	Boundary     []string
	StopBoundary []string
}

func cmdFactoryCanaryScan(args []string) error {
	fs := flag.NewFlagSet("factory canary-scan", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	limit := fs.Int("limit", 10, "Maximum open issues to read per repo")
	maxDuration := fs.Duration("max-duration", 2*time.Minute, "Hard wall-clock cap for the canary scan")
	maxCostUSD := fs.Float64("max-cost-usd", 0, "Recorded cost cap for this canary command; the scanner itself does not call LLMs")
	output := fs.String("output", "", "Optional JSON report path")
	repos := repeatedStringFlag{}
	labels := repeatedStringFlag{}
	fs.Var(&repos, "repo", "Transpara-AI repo slug to scan, e.g. transpara-ai/docs (repeatable; required)")
	fs.Var(&labels, "label", "GitHub issue label filter passed to gh issue list (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*human) == "" {
		return fmt.Errorf("--human is required")
	}
	if len(repos) == 0 {
		return fmt.Errorf("--repo is required")
	}
	if *limit <= 0 {
		return fmt.Errorf("--limit must be greater than zero")
	}
	if *maxDuration <= 0 {
		return fmt.Errorf("--max-duration must be greater than zero")
	}
	if *maxCostUSD < 0 {
		return fmt.Errorf("--max-cost-usd must be zero or greater")
	}
	normalizedRepos, err := resolveIssueScanRepos(repos, false, "")
	if err != nil {
		return err
	}

	baseCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()
	ctx, cancel := context.WithTimeout(baseCtx, *maxDuration)
	defer cancel()

	fc, err := openFactoryContext(ctx, *storeDSN, *human)
	if err != nil {
		return err
	}
	defer fc.close()

	issues, err := scanGitHubIssues(ctx, normalizedRepos, *limit, labels)
	if err != nil {
		return err
	}
	report, err := buildLevel1CanaryReport(ctx, fc, issues, level1CanaryReportOptions{
		Repos:       normalizedRepos,
		Limit:       *limit,
		MaxDuration: *maxDuration,
		MaxCostUSD:  *maxCostUSD,
		GeneratedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if *output != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*output, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write canary report: %w", err)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func buildLevel1CanaryReport(ctx context.Context, fc *factoryContext, issues []hive.GitHubIssueCandidate, opts level1CanaryReportOptions) (level1CanaryReport, error) {
	generatedAt := opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	report := level1CanaryReport{
		Kind:          "level1_dark_factory_canary_issue_discovery",
		GeneratedAt:   generatedAt.UTC().Format(time.RFC3339Nano),
		Repos:         append([]string(nil), opts.Repos...),
		Limit:         opts.Limit,
		MaxDuration:   opts.MaxDuration.String(),
		MaxCostUSD:    opts.MaxCostUSD,
		ScannedIssues: len(issues),
		Boundary: []string{
			"read-only GitHub issue discovery",
			"EventGraph writes limited to hive.issuescan.run.parked canary evidence",
			"no issue closure, label change, PR merge, deploy, runtime wake, value allocation, or autonomy increase",
		},
		StopConditions: []string{
			"timeout or signal",
			"unclear authority",
			"GitHub discovery failure",
			"EventGraph parked evidence cannot be recorded",
			"write outside canary-scoped parked evidence",
		},
	}
	for _, issue := range issues {
		result := level1CanaryIssueResult{
			Repo:   issue.Repo,
			Number: issue.Number,
			URL:    issue.URL,
			Title:  issue.Title,
			State:  valueOrCLI(issue.State, "open"),
			Labels: append([]string(nil), issue.Labels...),
		}
		if hive.IssueScanCandidatePRReady(issue) {
			result.Classification = "pr_ready"
			result.RequiredAction = "eligible for a separately authorized one-active-work-item issue-scan run"
			report.PRReadyIssues++
			report.Issues = append(report.Issues, result)
			continue
		}
		blockerType, detail, requiredAction := canaryIssueBlocker(issue)
		if blockerType == "" {
			return level1CanaryReport{}, fmt.Errorf("issue %s#%d could not be classified safely", issue.Repo, issue.Number)
		}
		result.Classification = "parked"
		result.BlockerType = blockerType
		result.RequiredAction = requiredAction
		result.RunID = canaryIssueRunID(issue)
		existing, eventID, err := canaryIssueParkedEvent(fc, result.RunID, issue.Repo, issue.Number)
		if err != nil {
			return level1CanaryReport{}, err
		}
		if existing {
			result.EventRef = eventID.Value()
			result.AlreadyParked = true
			report.AlreadyParked++
			report.EventRefs = append(report.EventRefs, eventID.Value())
			report.Issues = append(report.Issues, result)
			continue
		}
		eventID, err = recordCanaryIssueParked(ctx, fc, issue, result.RunID, blockerType, detail, requiredAction)
		if err != nil {
			return level1CanaryReport{}, err
		}
		result.EventRef = eventID.Value()
		report.ParkedIssues++
		report.EventRefs = append(report.EventRefs, eventID.Value())
		report.Issues = append(report.Issues, result)
	}
	report.EventRefs = compactCLIStrings(report.EventRefs)
	return report, nil
}

func recordCanaryIssueParked(ctx context.Context, fc *factoryContext, issue hive.GitHubIssueCandidate, runID, blockerType, detail, requiredAction string) (types.EventID, error) {
	select {
	case <-ctx.Done():
		return types.EventID{}, ctx.Err()
	default:
	}
	if existing, eventID, err := canaryIssueParkedEvent(fc, runID, issue.Repo, issue.Number); err != nil {
		return types.EventID{}, err
	} else if existing {
		return eventID, nil
	}
	content := hive.IssueScanRunParkedContent{
		RunID:             runID,
		Repository:        issue.Repo,
		IssueNumber:       issue.Number,
		LifecycleVersion:  hive.IssueScanParkLifecycleLevel1Canary,
		EvidenceClass:     hive.IssueScanParkEvidenceClassLevel1Canary,
		AuthorityBoundary: hive.IssueScanParkAuthorityBoundaryLevel1Canary,
		BlockerType:       blockerType,
		Detail:            detail,
		RequiredAction:    requiredAction,
		SourceRefs:        compactCLIStrings([]string{issue.URL, "canary://level1-dark-factory/issue-discovery"}),
		ParkedBy:          fc.humanID,
		TargetIssueState:  valueOrCLI(issue.State, "open"),
		TargetIssueLabels: compactCLIStrings(issue.Labels),
	}
	conv, err := types.NewConversationID("conv_level1_canary_issue_scan_000001")
	if err != nil {
		return types.EventID{}, err
	}
	ev, err := fc.factory.Create(hive.EventTypeIssueScanRunParked, fc.humanID, content, fc.headCauses(), conv, fc.store, fc.signer)
	if err != nil {
		return types.EventID{}, fmt.Errorf("create canary parked event: %w", err)
	}
	stored, err := fc.store.Append(ev)
	if err != nil {
		return types.EventID{}, fmt.Errorf("append canary parked event: %w", err)
	}
	return stored.ID(), nil
}

func canaryIssueParkedEvent(fc *factoryContext, runID, repo string, number int) (bool, types.EventID, error) {
	repo = strings.ToLower(strings.TrimSpace(repo))
	runID = strings.TrimSpace(runID)
	events, err := fc.store.ByType(hive.EventTypeIssueScanRunParked, 1000, types.None[types.Cursor]())
	if err != nil {
		return false, types.EventID{}, fmt.Errorf("query existing canary parked events: %w", err)
	}
	for {
		for _, ev := range events.Items() {
			content, ok := ev.Content().(hive.IssueScanRunParkedContent)
			if !ok {
				continue
			}
			if !canaryParkedContentIsLevel1Canary(content) {
				continue
			}
			if repo != "" && number > 0 && strings.EqualFold(strings.TrimSpace(content.Repository), repo) && content.IssueNumber == number {
				return true, ev.ID(), nil
			}
			if repo == "" && number <= 0 && strings.TrimSpace(content.RunID) == runID {
				return true, ev.ID(), nil
			}
		}
		if !events.HasMore() {
			break
		}
		events, err = fc.store.ByType(hive.EventTypeIssueScanRunParked, 1000, events.Cursor())
		if err != nil {
			return false, types.EventID{}, fmt.Errorf("query existing canary parked events: %w", err)
		}
	}
	return false, types.EventID{}, nil
}

func canaryParkedContentIsLevel1Canary(content hive.IssueScanRunParkedContent) bool {
	if strings.TrimSpace(content.EvidenceClass) == hive.IssueScanParkEvidenceClassLevel1Canary {
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(content.RunID), "level1_canary_") {
		return true
	}
	for _, ref := range content.SourceRefs {
		if strings.TrimSpace(ref) == "canary://level1-dark-factory/issue-discovery" {
			return true
		}
	}
	return false
}

func canaryIssueBlocker(issue hive.GitHubIssueCandidate) (string, string, string) {
	labels := map[string]bool{}
	for _, label := range issue.Labels {
		labels[strings.ToLower(strings.TrimSpace(label))] = true
	}
	switch {
	case labels[hive.IssueScanProtectedActionLabel]:
		return hive.IssueScanParkBlockerProtectedAction,
			fmt.Sprintf("%s#%d is labeled %s", issue.Repo, issue.Number, hive.IssueScanProtectedActionLabel),
			"human must authorize the protected-action boundary before Hive may continue"
	case labels[hive.IssueScanNeedsHumanScopeLabel]:
		return hive.IssueScanParkBlockerHumanScope,
			fmt.Sprintf("%s#%d is labeled %s", issue.Repo, issue.Number, hive.IssueScanNeedsHumanScopeLabel),
			"human must clarify scope and remove the human-scope blocker before Hive may continue"
	case labels[hive.IssueScanPRDeferredLabel]:
		return hive.IssueScanParkBlockerHumanScope,
			fmt.Sprintf("%s#%d is labeled %s", issue.Repo, issue.Number, hive.IssueScanPRDeferredLabel),
			"human must move the issue to PR-ready before Hive may continue"
	default:
		return hive.IssueScanParkBlockerNotPRReady,
			fmt.Sprintf("%s#%d lacks %s", issue.Repo, issue.Number, hive.IssueScanPRReadyLabel),
			"human must mark the issue PR-ready before Hive may continue"
	}
}

func canaryIssueRunID(issue hive.GitHubIssueCandidate) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(issue.Repo))))
	return "level1_canary_" + canarySafeToken(strings.ReplaceAll(issue.Repo, "/", "_")) + "_" + fmt.Sprintf("%d", issue.Number) + "_" + hex.EncodeToString(sum[:4])
}

func valueOrCLI(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func compactCLIStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func canarySafeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '/' || r == '.':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_-")
	if out == "" {
		return "issue"
	}
	return out
}
