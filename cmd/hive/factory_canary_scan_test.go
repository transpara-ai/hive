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
	if second.Issues[0].FidelityGuidance == nil {
		t.Fatal("already-parked issue missing fidelity guidance")
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
	if report.Issues[0].FidelityGuidance != nil {
		t.Fatalf("PR-ready issue guidance = %+v, want nil", report.Issues[0].FidelityGuidance)
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
	if report.Issues[0].FidelityGuidance == nil {
		t.Fatal("protected parked issue missing fidelity guidance")
	}
	if got := report.Issues[0].FidelityGuidance.BlockedByLabels; !containsString(got, hive.IssueScanProtectedActionLabel) {
		t.Fatalf("blocked labels = %v, want %s", got, hive.IssueScanProtectedActionLabel)
	}
	if events := canaryParkedEventsForTest(t, fc); len(events) != 1 {
		t.Fatalf("parked event count = %d, want 1", len(events))
	}
}

func TestBuildLevel1CanaryReportGuidesSparseNonReadyIssue(t *testing.T) {
	ctx := context.Background()
	fc, err := openFactoryContext(ctx, "", "Michael")
	if err != nil {
		t.Fatalf("openFactoryContext: %v", err)
	}
	defer fc.close()

	report, err := buildLevel1CanaryReport(ctx, fc, []hive.GitHubIssueCandidate{{
		Repo:   "transpara-ai/matlab-client",
		Number: 1,
		Title:  "Make this better",
		State:  "open",
		Labels: nil,
	}}, level1CanaryReportOptions{
		Repos:       []string{"transpara-ai/matlab-client"},
		Limit:       10,
		MaxDuration: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("buildLevel1CanaryReport: %v", err)
	}
	if report.ParkedIssues != 1 || report.PRReadyIssues != 0 {
		t.Fatalf("report = %+v, want one parked non-ready issue", report)
	}
	guidance := report.Issues[0].FidelityGuidance
	if guidance == nil {
		t.Fatal("parked issue missing fidelity guidance")
	}
	if guidance.State != issueScanFidelityStateNeedsFidelity {
		t.Fatalf("guidance state = %q", guidance.State)
	}
	for _, want := range []string{"problem", "goal", "PR-Ready-When", "authority/protected-action boundary"} {
		if !containsString(guidance.MissingFields, want) {
			t.Fatalf("missing fields = %v, want %q", guidance.MissingFields, want)
		}
	}
	for _, want := range []string{issueScanIntakeLabel} {
		if !containsString(guidance.RequiredLabels, want) {
			t.Fatalf("required labels = %v, want %q", guidance.RequiredLabels, want)
		}
	}
	if guidance.PromotionHint != issueScanReadyPromotionHint {
		t.Fatalf("promotion hint = %q, want %q", guidance.PromotionHint, issueScanReadyPromotionHint)
	}
	if containsString(guidance.RequiredLabels, issueScanCivilizationPresenceLabel) {
		t.Fatalf("required labels = %v, %s is optional and must not be universal", guidance.RequiredLabels, issueScanCivilizationPresenceLabel)
	}
	if len(guidance.NextQuestions) == 0 || !strings.Contains(guidance.NextQuestions[0], "problem") {
		t.Fatalf("next questions = %v, want problem question first", guidance.NextQuestions)
	}
	if !strings.Contains(report.Issues[0].RequiredAction, "missing change-control fidelity fields") {
		t.Fatalf("required action = %q, want fidelity guidance", report.Issues[0].RequiredAction)
	}
	events := canaryParkedEventsForTest(t, fc)
	if len(events) != 1 {
		t.Fatalf("parked event count = %d, want 1", len(events))
	}
	if !strings.Contains(events[0].RequiredAction, "missing change-control fidelity fields") {
		t.Fatalf("persisted required action = %q, want fidelity guidance", events[0].RequiredAction)
	}
}

func TestCanaryIssueFidelityGuidanceDetectsPartialChecklist(t *testing.T) {
	issue := hive.GitHubIssueCandidate{
		Repo:   "transpara-ai/hive",
		Number: 257,
		Body: strings.Join([]string{
			"## Problem",
			"Hive parks non-ready issues without enough operator guidance.",
			"## Goal",
			"Return a shepherd checklist.",
			"## Affected repos",
			"- transpara-ai/hive",
			"## Primary repo",
			"- transpara-ai/hive",
			"## Acceptance criteria",
			"- Canary output carries missing fields.",
			"## PR-Ready-When",
			"- Human verifies the checklist is complete.",
			"## Civilization Presence",
			"Yes.",
		}, "\n"),
		Labels: []string{issueScanIntakeLabel, issueScanCivilizationPresenceLabel},
	}

	guidance := canaryIssueFidelityGuidance(issue, hive.IssueScanParkBlockerNotPRReady)
	for _, want := range []string{"problem", "goal", "affected repos", "primary repo", "acceptance criteria", "PR-Ready-When", "Civilization Presence"} {
		if !containsString(guidance.PresentFields, want) {
			t.Fatalf("present fields = %v, want %q", guidance.PresentFields, want)
		}
	}
	for _, want := range []string{"scope boundaries", "evidence and test plan", "authority/protected-action boundary"} {
		if !containsString(guidance.MissingFields, want) {
			t.Fatalf("missing fields = %v, want %q", guidance.MissingFields, want)
		}
	}
	if containsString(guidance.RequiredLabels, issueScanIntakeLabel) || containsString(guidance.RequiredLabels, issueScanCivilizationPresenceLabel) {
		t.Fatalf("required labels = %v, should not repeat labels already present", guidance.RequiredLabels)
	}
	if guidance.PromotionHint != issueScanReadyPromotionHint {
		t.Fatalf("promotion hint = %q, want %q", guidance.PromotionHint, issueScanReadyPromotionHint)
	}
}

func TestBuildLevel1CanaryReportKeepsFullFidelityPromotionHintParked(t *testing.T) {
	ctx := context.Background()
	fc, err := openFactoryContext(ctx, "", "Michael")
	if err != nil {
		t.Fatalf("openFactoryContext: %v", err)
	}
	defer fc.close()

	report, err := buildLevel1CanaryReport(ctx, fc, []hive.GitHubIssueCandidate{{
		Repo:   "transpara-ai/hive",
		Number: 257,
		Title:  "Full fidelity but not labeled PR-ready",
		State:  "open",
		Body:   canaryIssueFullChecklistBodyForTest(),
		Labels: []string{issueScanIntakeLabel},
	}}, level1CanaryReportOptions{
		Repos:       []string{"transpara-ai/hive"},
		Limit:       10,
		MaxDuration: 2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("buildLevel1CanaryReport: %v", err)
	}
	if report.PRReadyIssues != 0 || report.ParkedIssues != 1 {
		t.Fatalf("report counts = pr_ready:%d parked:%d, want parked full-fidelity issue", report.PRReadyIssues, report.ParkedIssues)
	}
	result := report.Issues[0]
	if result.Classification != "parked" {
		t.Fatalf("classification = %q, want parked", result.Classification)
	}
	if result.FidelityGuidance == nil || result.FidelityGuidance.State != issueScanFidelityStateReadyForHuman {
		t.Fatalf("guidance = %+v, want ready-for-human advisory state", result.FidelityGuidance)
	}
	if !strings.Contains(result.RequiredAction, "human may apply "+hive.IssueScanPRReadyLabel) {
		t.Fatalf("required action = %q, want human promotion action", result.RequiredAction)
	}
	events := canaryParkedEventsForTest(t, fc)
	if len(events) != 1 {
		t.Fatalf("parked event count = %d, want 1", len(events))
	}
	if !strings.Contains(events[0].RequiredAction, "human may apply "+hive.IssueScanPRReadyLabel) {
		t.Fatalf("persisted required action = %q, want human promotion action", events[0].RequiredAction)
	}
}

func TestCanaryIssueFidelityGuidanceIdentifiesHumanPromotionReadyState(t *testing.T) {
	issue := hive.GitHubIssueCandidate{
		Repo:   "transpara-ai/hive",
		Number: 257,
		Body:   canaryIssueFullChecklistBodyForTest(),
		Labels: []string{issueScanIntakeLabel},
	}

	guidance := canaryIssueFidelityGuidance(issue, hive.IssueScanParkBlockerNotPRReady)
	if guidance.State != issueScanFidelityStateReadyForHuman {
		t.Fatalf("guidance state = %q, want ready_for_human_pr_ready_label; missing=%v blocked=%v required=%v", guidance.State, guidance.MissingFields, guidance.BlockedByLabels, guidance.RequiredLabels)
	}
	if len(guidance.MissingFields) != 0 {
		t.Fatalf("missing fields = %v, want none", guidance.MissingFields)
	}
	if guidance.PromotionHint != issueScanReadyPromotionHint {
		t.Fatalf("promotion hint = %q, want %q", guidance.PromotionHint, issueScanReadyPromotionHint)
	}
	if containsString(guidance.RequiredLabels, issueScanIntakeLabel) {
		t.Fatalf("required labels = %v, should not require already-present intake label", guidance.RequiredLabels)
	}
	if containsString(guidance.RequiredLabels, issueScanCivilizationPresenceLabel) {
		t.Fatalf("required labels = %v, %s is optional when the body answers Civilization Presence", guidance.RequiredLabels, issueScanCivilizationPresenceLabel)
	}
	if !containsString(guidance.ReadyWhen, "cc:intake label is present") {
		t.Fatalf("ready_when = %v, want intake label condition", guidance.ReadyWhen)
	}
}

func TestCanaryIssueFidelityGuidanceKeepsBlockingLabelOutOfPromotionState(t *testing.T) {
	for _, blockingLabel := range []string{
		hive.IssueScanNeedsHumanScopeLabel,
		hive.IssueScanProtectedActionLabel,
		hive.IssueScanPRDeferredLabel,
	} {
		t.Run(blockingLabel, func(t *testing.T) {
			issue := hive.GitHubIssueCandidate{
				Repo:   "transpara-ai/hive",
				Number: 257,
				Body:   canaryIssueFullChecklistBodyForTest(),
				Labels: []string{issueScanIntakeLabel, blockingLabel},
			}

			guidance := canaryIssueFidelityGuidance(issue, hive.IssueScanParkBlockerNotPRReady)
			if guidance.State != issueScanFidelityStateNeedsFidelity {
				t.Fatalf("guidance state = %q, want blocked guidance state", guidance.State)
			}
			if !containsString(guidance.BlockedByLabels, blockingLabel) {
				t.Fatalf("blocked labels = %v, want %s", guidance.BlockedByLabels, blockingLabel)
			}
			if len(guidance.MissingFields) != 0 {
				t.Fatalf("missing fields = %v, want none; blocking label alone should prevent promotion", guidance.MissingFields)
			}
		})
	}
}

func TestCanaryIssueBodyHasFieldDetectsFirstLineListItems(t *testing.T) {
	if !canaryIssueBodyHasField("- Problem: first line list item", "problem") {
		t.Fatal("first-line list item field was not detected")
	}
	if canaryIssueBodyHasField("## Problematic behavior", "problem") {
		t.Fatal("superstring heading should not satisfy problem field")
	}
	if canaryIssueBodyHasField("Goal posts shifted in the planning thread.", "goal") {
		t.Fatal("ordinary prose starting with a field word should not satisfy goal field")
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
			actionText:  hive.IssueScanPRReadyLabel,
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func canaryIssueFullChecklistBodyForTest() string {
	return strings.Join([]string{
		"## Problem",
		"Hive parks non-ready issues without enough operator guidance.",
		"## Goal",
		"Return a shepherd checklist.",
		"## Affected repos",
		"- transpara-ai/hive",
		"## Primary repo",
		"- transpara-ai/hive",
		"## Scope boundaries",
		"- Report-only guidance; no live mutation.",
		"## Acceptance criteria",
		"- Canary output carries missing fields.",
		"## Evidence and test plan",
		"- Run focused canary tests.",
		"## PR-Ready-When",
		"- Human verifies the checklist is complete.",
		"## Touched substrate",
		"- Hive canary issue intake.",
		"## Aggregation guidance",
		"- Standalone.",
		"## Civilization Presence",
		"Yes.",
		"## Authority/protected-action boundary",
		"No protected action is authorized.",
	}, "\n")
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
