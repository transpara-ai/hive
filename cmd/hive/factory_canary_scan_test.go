package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
)

func TestBuildLevel1CanaryReportParksOnceAndReportsAlreadyParked(t *testing.T) {
	ctx := context.Background()
	fc, err := openFactoryContext(ctx, "", "Michael")
	if err != nil {
		t.Fatalf("openFactoryContext: %v", err)
	}
	defer fc.close()

	issues := []hive.GitHubIssueCandidate{{
		Repo:   "transpara-ai/docs",
		Number: 226,
		Title:  "Future live operation path",
		URL:    "https://github.com/transpara-ai/docs/issues/226",
		State:  "open",
		Labels: []string{hive.IssueScanNeedsHumanScopeLabel, hive.IssueScanProtectedActionLabel},
	}}
	opts := level1CanaryReportOptions{
		Repos:       []string{"transpara-ai/docs"},
		Limit:       10,
		MaxDuration: 2 * time.Minute,
		GeneratedAt: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}

	first, err := buildLevel1CanaryReport(ctx, fc, issues, opts)
	if err != nil {
		t.Fatalf("buildLevel1CanaryReport first: %v", err)
	}
	if first.ScannedIssues != 1 || first.ParkedIssues != 1 || first.AlreadyParked != 0 || len(first.EventRefs) != 1 {
		t.Fatalf("first report = %+v, want one newly parked issue", first)
	}
	if got := first.Issues[0].BlockerType; got != hive.IssueScanParkBlockerProtectedAction {
		t.Fatalf("primary blocker = %q, want protected action", got)
	}
	if !strings.Contains(first.Issues[0].RequiredAction, "protected-action") {
		t.Fatalf("required action = %q, want protected-action authorization", first.Issues[0].RequiredAction)
	}
	events := canaryParkedEventsForTest(t, fc)
	if len(events) != 1 {
		t.Fatalf("parked event count = %d, want 1", len(events))
	}
	if events[0].LifecycleVersion != hive.IssueScanParkLifecycleLevel1Canary ||
		events[0].EvidenceClass != hive.IssueScanParkEvidenceClassLevel1Canary ||
		events[0].AuthorityBoundary != hive.IssueScanParkAuthorityBoundaryLevel1Canary {
		t.Fatalf("parked event metadata = %+v", events[0])
	}

	second, err := buildLevel1CanaryReport(ctx, fc, issues, opts)
	if err != nil {
		t.Fatalf("buildLevel1CanaryReport second: %v", err)
	}
	if second.ParkedIssues != 0 || second.AlreadyParked != 1 || len(second.EventRefs) != 1 || !second.Issues[0].AlreadyParked {
		t.Fatalf("second report = %+v, want already-parked with no duplicate write", second)
	}
	if events := canaryParkedEventsForTest(t, fc); len(events) != 1 {
		t.Fatalf("parked event count after second run = %d, want 1", len(events))
	}
}

func TestBuildLevel1CanaryReportDoesNotWritePRReadyIssues(t *testing.T) {
	ctx := context.Background()
	fc, err := openFactoryContext(ctx, "", "Michael")
	if err != nil {
		t.Fatalf("openFactoryContext: %v", err)
	}
	defer fc.close()

	report, err := buildLevel1CanaryReport(ctx, fc, []hive.GitHubIssueCandidate{{
		Repo:   "transpara-ai/hive",
		Number: 237,
		Title:  "Ready canary candidate",
		URL:    "https://github.com/transpara-ai/hive/issues/237",
		State:  "open",
		Labels: []string{hive.IssueScanPRReadyLabel},
	}}, level1CanaryReportOptions{
		Repos:       []string{"transpara-ai/hive"},
		Limit:       10,
		MaxDuration: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("buildLevel1CanaryReport: %v", err)
	}
	if report.PRReadyIssues != 1 || report.ParkedIssues != 0 || len(report.EventRefs) != 0 {
		t.Fatalf("report = %+v, want PR-ready issue without parked evidence", report)
	}
	if events := canaryParkedEventsForTest(t, fc); len(events) != 0 {
		t.Fatalf("parked event count = %d, want 0", len(events))
	}
}

func TestBuildLevel1CanaryReportParksProtectedActionEvenWhenPRReady(t *testing.T) {
	ctx := context.Background()
	fc, err := openFactoryContext(ctx, "", "Michael")
	if err != nil {
		t.Fatalf("openFactoryContext: %v", err)
	}
	defer fc.close()

	report, err := buildLevel1CanaryReport(ctx, fc, []hive.GitHubIssueCandidate{{
		Repo:   "transpara-ai/hive",
		Number: 238,
		Title:  "Protected candidate",
		URL:    "https://github.com/transpara-ai/hive/issues/238",
		State:  "open",
		Labels: []string{hive.IssueScanPRReadyLabel, hive.IssueScanProtectedActionLabel},
	}}, level1CanaryReportOptions{
		Repos:       []string{"transpara-ai/hive"},
		Limit:       10,
		MaxDuration: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("buildLevel1CanaryReport: %v", err)
	}
	if report.PRReadyIssues != 0 || report.ParkedIssues != 1 || len(report.EventRefs) != 1 {
		t.Fatalf("report = %+v, want protected-action parked despite PR-ready label", report)
	}
	if report.Issues[0].BlockerType != hive.IssueScanParkBlockerProtectedAction {
		t.Fatalf("blocker = %q, want protected action", report.Issues[0].BlockerType)
	}
	if events := canaryParkedEventsForTest(t, fc); len(events) != 1 {
		t.Fatalf("parked event count = %d, want 1", len(events))
	}
}

func TestCanaryIssueBlockerClassifiesBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		labels      []string
		blockerType string
		actionText  string
	}{
		{
			name:        "protected takes priority",
			labels:      []string{hive.IssueScanNeedsHumanScopeLabel, hive.IssueScanProtectedActionLabel},
			blockerType: hive.IssueScanParkBlockerProtectedAction,
			actionText:  "protected-action",
		},
		{
			name:        "human scope",
			labels:      []string{hive.IssueScanNeedsHumanScopeLabel},
			blockerType: hive.IssueScanParkBlockerHumanScope,
			actionText:  "clarify scope",
		},
		{
			name:        "deferred",
			labels:      []string{hive.IssueScanPRDeferredLabel},
			blockerType: hive.IssueScanParkBlockerHumanScope,
			actionText:  "PR-ready",
		},
		{
			name:        "not ready",
			labels:      nil,
			blockerType: hive.IssueScanParkBlockerNotPRReady,
			actionText:  "PR-ready",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blockerType, _, requiredAction := canaryIssueBlocker(hive.GitHubIssueCandidate{
				Repo:   "transpara-ai/hive",
				Number: 237,
				Labels: tc.labels,
			})
			if blockerType != tc.blockerType || !strings.Contains(requiredAction, tc.actionText) {
				t.Fatalf("blocker/action = %q/%q, want %q containing %q", blockerType, requiredAction, tc.blockerType, tc.actionText)
			}
		})
	}
}

func TestCanaryIssueRunIDDistinguishesNormalizedRepoCollisions(t *testing.T) {
	first := canaryIssueRunID(hive.GitHubIssueCandidate{Repo: "transpara-ai/a_b", Number: 5})
	second := canaryIssueRunID(hive.GitHubIssueCandidate{Repo: "transpara-ai_a/b", Number: 5})
	if first == second {
		t.Fatalf("run IDs collided: %q", first)
	}
	if !strings.HasPrefix(first, "level1_canary_") || !strings.HasPrefix(second, "level1_canary_") {
		t.Fatalf("run IDs missing canary prefix: %q / %q", first, second)
	}
}

func canaryParkedEventsForTest(t *testing.T, fc *factoryContext) []hive.IssueScanRunParkedContent {
	t.Helper()
	var out []hive.IssueScanRunParkedContent
	page, err := fc.store.ByType(hive.EventTypeIssueScanRunParked, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType parked events: %v", err)
	}
	for {
		for _, ev := range page.Items() {
			content, ok := ev.Content().(hive.IssueScanRunParkedContent)
			if ok {
				out = append(out, content)
			}
		}
		if !page.HasMore() {
			break
		}
		page, err = fc.store.ByType(hive.EventTypeIssueScanRunParked, 100, page.Cursor())
		if err != nil {
			t.Fatalf("ByType parked events page: %v", err)
		}
	}
	return out
}
