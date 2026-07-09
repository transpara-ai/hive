package hive

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPlanIssueScanSourceIssueMarkerAcquiredUsesProjectionBoundary(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition: IssueScanSourceIssueMarkerAcquired,
		Issue: GitHubIssueCandidate{
			Repo:   "transpara-ai/docs",
			Number: 256,
			Title:  "Factory-order acquisition marker and source-of-truth boundary",
			URL:    "https://github.com/transpara-ai/docs/issues/256",
			Labels: []string{IssueScanPRReadyLabel, "cc:civilization-presence"},
		},
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
		StageID:        "research_issue_and_repo_context",
		StageState:     "ready",
		ActorRole:      "dispatcher",
		WorkRefs:       []string{"work:task:tsk_issue_scan_docs_256_research"},
		EventGraphRefs: []string{"eventgraph:issuescan.run.projected:run_docs_256"},
		EvidenceRefs:   []string{"github:https://github.com/transpara-ai/docs/issues/256"},
		GeneratedAt:    time.Date(2026, 7, 3, 10, 45, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}

	if plan.Repo != "transpara-ai/docs" || plan.IssueNumber != 256 {
		t.Fatalf("target = %s#%d, want transpara-ai/docs#256", plan.Repo, plan.IssueNumber)
	}
	if len(plan.AddLabels) != 1 || plan.AddLabels[0] != IssueScanFactoryStatusLabelAcquired {
		t.Fatalf("add labels = %+v, want acquired factory label", plan.AddLabels)
	}
	for _, label := range append(append([]string(nil), plan.AddLabels...), plan.RemoveLabels...) {
		if strings.HasPrefix(label, "cc:") {
			t.Fatalf("marker plan mutates change-control label %q", label)
		}
	}
	for _, wantRemoved := range []string{
		IssueScanFactoryStatusLabelParked,
		IssueScanFactoryStatusLabelReadyForHuman,
		IssueScanFactoryStatusLabelCompleted,
		IssueScanFactoryStatusLabelAbandoned,
		IssueScanFactoryStatusLabelSuperseded,
	} {
		if !containsIssueScanValue(plan.RemoveLabels, wantRemoved) {
			t.Fatalf("remove labels = %+v, want %s", plan.RemoveLabels, wantRemoved)
		}
	}
	for _, want := range []string{
		"<!-- " + plan.IdempotencyKey + " -->",
		"Factory issue-scan marker: acquired",
		"run_id: `issue-scan-docs-256`",
		"factory_order_id: `fo_issue_scan_docs_256`",
		"stage_id: `research_issue_and_repo_context`",
		"work:task:tsk_issue_scan_docs_256_research",
		"eventgraph:issuescan.run.projected:run_docs_256",
		"Do not parse this comment as workflow state or authority.",
		"does not authorize protected actions",
	} {
		if !strings.Contains(plan.CommentBody, want) {
			t.Fatalf("comment body missing %q:\n%s", want, plan.CommentBody)
		}
	}
	if !IssueScanSourceIssueMarkerCommentExists([]string{"unrelated", plan.CommentBody}, plan) {
		t.Fatalf("planned marker was not detected by idempotency key")
	}
	if IssueScanSourceIssueMarkerCommentExists([]string{"unrelated"}, plan) {
		t.Fatalf("unrelated comments matched marker idempotency key")
	}
}

func TestPlanIssueScanSourceIssueMarkerHumanActionUsesParkedStatus(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerHumanAction,
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
		StageID:        "implement_on_branch",
		StageState:     "policy_blocked",
		EvidenceRefs:   []string{"github:https://github.com/transpara-ai/docs/issues/256"},
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}
	if len(plan.AddLabels) != 1 || plan.AddLabels[0] != IssueScanFactoryStatusLabelParked {
		t.Fatalf("add labels = %+v, want parked label for human-action marker", plan.AddLabels)
	}
	if !containsIssueScanValue(plan.RemoveLabels, IssueScanFactoryStatusLabelAcquired) {
		t.Fatalf("remove labels = %+v, want acquired removed when parking", plan.RemoveLabels)
	}
	if !strings.Contains(plan.CommentBody, "Factory issue-scan marker: human_action") || !strings.Contains(plan.CommentBody, "stage_state: `policy_blocked`") {
		t.Fatalf("human-action comment body missing transition/state:\n%s", plan.CommentBody)
	}
}

func TestPlanIssueScanSourceIssueMarkerTransitionsUseExpectedLabels(t *testing.T) {
	for _, tc := range []struct {
		name       string
		transition IssueScanSourceIssueMarkerTransition
		wantLabel  string
	}{
		{name: "parked", transition: IssueScanSourceIssueMarkerParked, wantLabel: IssueScanFactoryStatusLabelParked},
		{name: "ready_for_human", transition: IssueScanSourceIssueMarkerReadyForHuman, wantLabel: IssueScanFactoryStatusLabelReadyForHuman},
		{name: "completed", transition: IssueScanSourceIssueMarkerCompleted, wantLabel: IssueScanFactoryStatusLabelCompleted},
		{name: "abandoned", transition: IssueScanSourceIssueMarkerAbandoned, wantLabel: IssueScanFactoryStatusLabelAbandoned},
		{name: "superseded", transition: IssueScanSourceIssueMarkerSuperseded, wantLabel: IssueScanFactoryStatusLabelSuperseded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := markerTestInput()
			input.Transition = tc.transition
			plan, err := PlanIssueScanSourceIssueMarker(input)
			if err != nil {
				t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
			}
			if len(plan.AddLabels) != 1 || plan.AddLabels[0] != tc.wantLabel {
				t.Fatalf("add labels = %+v, want %s", plan.AddLabels, tc.wantLabel)
			}
			if containsIssueScanValue(plan.RemoveLabels, tc.wantLabel) {
				t.Fatalf("remove labels = %+v, must not remove current label %s", plan.RemoveLabels, tc.wantLabel)
			}
			if !strings.Contains(plan.CommentBody, "Factory issue-scan marker: "+string(tc.transition)) {
				t.Fatalf("comment body missing transition %q:\n%s", tc.transition, plan.CommentBody)
			}
		})
	}
}

func TestPlanIssueScanSourceIssueMarkerRejectsIncompleteCanonicalRefs(t *testing.T) {
	_, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition: IssueScanSourceIssueMarkerAcquired,
		Issue:      markerTestIssue(),
		RunID:      "issue-scan-docs-256",
	})
	if err == nil || !strings.Contains(err.Error(), "factory_order_id is required") {
		t.Fatalf("missing factory order error = %v, want factory_order_id required", err)
	}

	_, err = PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerTransition("step_update"),
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown issue-scan source marker transition") {
		t.Fatalf("unknown transition error = %v", err)
	}
}

func TestApplyIssueScanSourceIssueMarkerAddsLabelsAndSkipsDuplicateComment(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerAcquired,
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}
	client := &fakeIssueScanMarkerClient{}
	result, err := ApplyIssueScanSourceIssueMarker(context.Background(), client, plan)
	if err != nil {
		t.Fatalf("ApplyIssueScanSourceIssueMarker first: %v", err)
	}
	if !result.CommentCreated || result.CommentSkipped {
		t.Fatalf("first apply result = %+v, want comment created", result)
	}
	if len(client.comments) != 1 || client.comments[0] != plan.CommentBody {
		t.Fatalf("comments = %+v, want one planned comment", client.comments)
	}
	if strings.Join(client.addedLabels, ",") != strings.Join(plan.AddLabels, ",") {
		t.Fatalf("added labels = %+v, want %+v", client.addedLabels, plan.AddLabels)
	}
	if strings.Join(client.removedLabels, ",") != strings.Join(plan.RemoveLabels, ",") {
		t.Fatalf("removed labels = %+v, want %+v", client.removedLabels, plan.RemoveLabels)
	}
	addedCount := len(client.addedLabels)
	removedCount := len(client.removedLabels)

	result, err = ApplyIssueScanSourceIssueMarker(context.Background(), client, plan)
	if err != nil {
		t.Fatalf("ApplyIssueScanSourceIssueMarker replay: %v", err)
	}
	if result.CommentCreated || !result.CommentSkipped {
		t.Fatalf("replay result = %+v, want comment skipped", result)
	}
	if len(client.comments) != 1 {
		t.Fatalf("comments after replay = %+v, want no duplicate", client.comments)
	}
	if len(client.addedLabels) != addedCount || len(client.removedLabels) != removedCount {
		t.Fatalf("label mutations changed on replay: added %d->%d removed %d->%d", addedCount, len(client.addedLabels), removedCount, len(client.removedLabels))
	}
}

func TestApplyIssueScanSourceIssueMarkerIgnoresHostileMarkerLookalike(t *testing.T) {
	plan, err := PlanIssueScanSourceIssueMarker(IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerAcquired,
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
	})
	if err != nil {
		t.Fatalf("PlanIssueScanSourceIssueMarker: %v", err)
	}
	client := &fakeIssueScanMarkerClient{
		comments: []string{
			"### Factory issue-scan marker: acquired\n\n<!-- wrong-key -->\nThis looks like a marker but does not carry the planned idempotency key.",
		},
	}
	result, err := ApplyIssueScanSourceIssueMarker(context.Background(), client, plan)
	if err != nil {
		t.Fatalf("ApplyIssueScanSourceIssueMarker: %v", err)
	}
	if !result.CommentCreated || result.CommentSkipped {
		t.Fatalf("apply result = %+v, want a new comment because prose is not canonical", result)
	}
	if len(client.comments) != 2 {
		t.Fatalf("comments = %+v, want hostile lookalike preserved plus planned marker", client.comments)
	}
	if client.comments[1] != plan.CommentBody {
		t.Fatalf("created comment = %q, want planned marker body", client.comments[1])
	}
}

func TestBridgeIssueScanSourceIssueMarkerDryRunsWithoutActivationEvenWithClient(t *testing.T) {
	client := &fakeIssueScanMarkerClient{}
	rt := &Runtime{issueScanSourceIssueMarkerClient: client}
	result, err := rt.BridgeIssueScanSourceIssueMarker(context.Background(), markerTestInput())
	if err != nil {
		t.Fatalf("BridgeIssueScanSourceIssueMarker: %v", err)
	}
	if !result.DryRun || result.Applied || !result.Refused {
		t.Fatalf("bridge result = %+v, want refused dry-run without activation", result)
	}
	if !strings.Contains(result.RefusalReason, "dry-run-only") {
		t.Fatalf("refusal = %q, want dry-run-only boundary", result.RefusalReason)
	}
	if len(client.comments) != 0 || len(client.addedLabels) != 0 || len(client.removedLabels) != 0 {
		t.Fatalf("client mutations = comments=%d add=%d remove=%d, want none", len(client.comments), len(client.addedLabels), len(client.removedLabels))
	}
}

func TestBridgeIssueScanSourceIssueMarkerAppliesOnlyWithMockedActivation(t *testing.T) {
	client := &fakeIssueScanMarkerClient{}
	rt := &Runtime{
		issueScanSourceIssueMarkerClient:     client,
		issueScanSourceIssueMarkerActivation: mockedIssueScanSourceIssueMarkerActivation("transpara-ai/docs", 256),
	}
	result, err := rt.BridgeIssueScanSourceIssueMarker(context.Background(), markerTestInput())
	if err != nil {
		t.Fatalf("BridgeIssueScanSourceIssueMarker: %v", err)
	}
	if result.DryRun || result.Refused || !result.Applied || !result.CommentCreated {
		t.Fatalf("bridge result = %+v, want mocked apply", result)
	}
	if result.ActivationMode != IssueScanSourceIssueMarkerActivationMockedImplementation || result.ClientKind != IssueScanSourceIssueMarkerMockClient {
		t.Fatalf("activation/client = %s/%s, want mocked activation with mock client", result.ActivationMode, result.ClientKind)
	}
	if result.AuthorityRef == "" {
		t.Fatalf("authority ref missing from result: %+v", result)
	}
	if len(client.comments) != 1 || !strings.Contains(client.comments[0], "Factory issue-scan marker: acquired") {
		t.Fatalf("client comments = %+v, want acquired marker", client.comments)
	}
}

func TestBridgeIssueScanSourceIssueMarkerRefusesLiveActivation(t *testing.T) {
	client := &fakeIssueScanMarkerClient{}
	rt := &Runtime{
		issueScanSourceIssueMarkerClient: client,
		issueScanSourceIssueMarkerActivation: IssueScanSourceIssueMarkerActivation{
			Mode:             IssueScanSourceIssueMarkerActivationLive,
			AuthorityRef:     "hive#249-live-activation-not-authorized",
			Actor:            "Codex",
			Environment:      "test",
			CredentialSource: "none",
			AllowedIssues:    []IssueScanSourceIssueMarkerIssueScope{{Repo: "transpara-ai/docs", Number: 256}},
			AllowComments:    true,
			AllowLabels:      true,
			StopConditions:   []string{"any live activation request requires future human authority"},
		},
	}
	result, err := rt.BridgeIssueScanSourceIssueMarker(context.Background(), markerTestInput())
	if err != nil {
		t.Fatalf("BridgeIssueScanSourceIssueMarker: %v", err)
	}
	if !result.DryRun || !result.Refused || result.Applied {
		t.Fatalf("bridge result = %+v, want refused dry-run for live activation", result)
	}
	if !strings.Contains(result.RefusalReason, "live source issue marker activation is not implemented or authorized") {
		t.Fatalf("refusal = %q, want live activation refusal", result.RefusalReason)
	}
	if len(client.comments) != 0 || len(client.addedLabels) != 0 || len(client.removedLabels) != 0 {
		t.Fatalf("client mutations = comments=%d add=%d remove=%d, want none", len(client.comments), len(client.addedLabels), len(client.removedLabels))
	}
}

func TestBridgeIssueScanSourceIssueMarkerRefusesOutOfScopeMockedActivation(t *testing.T) {
	client := &fakeIssueScanMarkerClient{}
	rt := &Runtime{
		issueScanSourceIssueMarkerClient:     client,
		issueScanSourceIssueMarkerActivation: mockedIssueScanSourceIssueMarkerActivation("transpara-ai/hive", 249),
	}
	result, err := rt.BridgeIssueScanSourceIssueMarker(context.Background(), markerTestInput())
	if err != nil {
		t.Fatalf("BridgeIssueScanSourceIssueMarker: %v", err)
	}
	if !result.DryRun || !result.Refused || result.Applied {
		t.Fatalf("bridge result = %+v, want refused dry-run for out-of-scope issue", result)
	}
	if !strings.Contains(result.RefusalReason, "outside activation scope") {
		t.Fatalf("refusal = %q, want scope refusal", result.RefusalReason)
	}
	if len(client.comments) != 0 || len(client.addedLabels) != 0 || len(client.removedLabels) != 0 {
		t.Fatalf("client mutations = comments=%d add=%d remove=%d, want none", len(client.comments), len(client.addedLabels), len(client.removedLabels))
	}
}

func TestBridgeIssueScanSourceIssueMarkerRefusesInvalidMockedActivationPackets(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*IssueScanSourceIssueMarkerActivation)
		client     IssueScanSourceIssueMarkerClient
		wantReason string
	}{
		{
			name: "unknown_mode",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.Mode = IssueScanSourceIssueMarkerActivationMode("auto_apply")
			},
			wantReason: "unknown source issue marker activation mode",
		},
		{
			name: "missing_authority_ref",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.AuthorityRef = ""
			},
			wantReason: "requires authority_ref",
		},
		{
			name: "missing_actor",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.Actor = ""
			},
			wantReason: "requires actor",
		},
		{
			name: "missing_environment",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.Environment = ""
			},
			wantReason: "requires environment",
		},
		{
			name: "empty_credential_source",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.CredentialSource = ""
			},
			wantReason: "requires credential_source none",
		},
		{
			name: "token_credential_source",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.CredentialSource = "GITHUB_TOKEN"
			},
			wantReason: "requires credential_source none",
		},
		{
			name: "live_evidence_allowed",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.LiveEvidenceAllowed = true
			},
			wantReason: "cannot allow live evidence",
		},
		{
			name: "missing_stop_conditions",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.StopConditions = nil
			},
			wantReason: "requires stop conditions",
		},
		{
			name: "comments_not_allowed",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.AllowComments = false
			},
			wantReason: "comment mutation is not allowed",
		},
		{
			name: "labels_not_allowed",
			mutate: func(a *IssueScanSourceIssueMarkerActivation) {
				a.AllowLabels = false
			},
			wantReason: "label mutation is not allowed",
		},
		{
			name:       "missing_client",
			client:     nil,
			wantReason: "client is not configured",
		},
		{
			name:       "live_client_kind",
			client:     &liveIssueScanMarkerClient{},
			wantReason: "requires a mock client",
		},
		{
			name:       "external_mock_claim_without_package_marker",
			client:     &selfAttestingIssueScanMarkerClient{},
			wantReason: "requires package-local mock client evidence",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.client
			if client == nil && tc.name != "missing_client" {
				client = &fakeIssueScanMarkerClient{}
			}
			activation := mockedIssueScanSourceIssueMarkerActivation("transpara-ai/docs", 256)
			if tc.mutate != nil {
				tc.mutate(&activation)
			}
			rt := &Runtime{
				issueScanSourceIssueMarkerClient:     client,
				issueScanSourceIssueMarkerActivation: activation,
			}
			result, err := rt.BridgeIssueScanSourceIssueMarker(context.Background(), markerTestInput())
			if err != nil {
				t.Fatalf("BridgeIssueScanSourceIssueMarker: %v", err)
			}
			if !result.DryRun || !result.Refused || result.Applied {
				t.Fatalf("bridge result = %+v, want refused dry-run", result)
			}
			if !strings.Contains(result.RefusalReason, tc.wantReason) {
				t.Fatalf("refusal = %q, want %q", result.RefusalReason, tc.wantReason)
			}
			switch c := client.(type) {
			case *fakeIssueScanMarkerClient:
				if len(c.comments) != 0 || len(c.addedLabels) != 0 || len(c.removedLabels) != 0 {
					t.Fatalf("fake client mutated on refusal: comments=%d add=%d remove=%d", len(c.comments), len(c.addedLabels), len(c.removedLabels))
				}
			case *liveIssueScanMarkerClient:
				if c.calls != 0 {
					t.Fatalf("live client received %d calls on refusal, want none", c.calls)
				}
			case *selfAttestingIssueScanMarkerClient:
				if c.calls != 0 {
					t.Fatalf("self-attesting client received %d calls on refusal, want none", c.calls)
				}
			}
		})
	}
}

func markerTestInput() IssueScanSourceIssueMarkerInput {
	return IssueScanSourceIssueMarkerInput{
		Transition:     IssueScanSourceIssueMarkerAcquired,
		Issue:          markerTestIssue(),
		RunID:          "issue-scan-docs-256",
		FactoryOrderID: "fo_issue_scan_docs_256",
		StageID:        "research_issue_and_repo_context",
		WorkRefs:       []string{"work:task:tsk_issue_scan_docs_256_research"},
		EventGraphRefs: []string{"eventgraph:issuescan.run.projected:run_docs_256"},
		EvidenceRefs:   []string{"github:https://github.com/transpara-ai/docs/issues/256"},
		GeneratedAt:    time.Date(2026, 7, 3, 10, 45, 0, 0, time.UTC),
	}
}

func markerTestIssue() GitHubIssueCandidate {
	return GitHubIssueCandidate{
		Repo:   "transpara-ai/docs",
		Number: 256,
		Title:  "Factory-order acquisition marker and source-of-truth boundary",
		URL:    "https://github.com/transpara-ai/docs/issues/256",
		Labels: []string{IssueScanPRReadyLabel, "cc:civilization-presence"},
	}
}

func mockedIssueScanSourceIssueMarkerActivation(repo string, number int) IssueScanSourceIssueMarkerActivation {
	return IssueScanSourceIssueMarkerActivation{
		Mode:             IssueScanSourceIssueMarkerActivationMockedImplementation,
		AuthorityRef:     "hive#249 mocked implementation authority, Codex thread 2026-07-09",
		Actor:            "Codex under Michael supervision",
		Environment:      "local unit test",
		CredentialSource: "none",
		AllowedIssues: []IssueScanSourceIssueMarkerIssueScope{{
			Repo:   repo,
			Number: number,
		}},
		AllowComments:  true,
		AllowLabels:    true,
		StopConditions: []string{"no live GitHub marker mutation is authorized by mocked activation"},
	}
}

type fakeIssueScanMarkerClient struct {
	addedLabels   []string
	removedLabels []string
	comments      []string
}

func (c *fakeIssueScanMarkerClient) SourceIssueMarkerClientKind() IssueScanSourceIssueMarkerClientKind {
	return IssueScanSourceIssueMarkerMockClient
}

func (c *fakeIssueScanMarkerClient) issueScanSourceIssueMarkerMockClient() {}

func (c *fakeIssueScanMarkerClient) AddLabels(_ context.Context, _ string, _ int, labels []string) error {
	c.addedLabels = append(c.addedLabels, labels...)
	return nil
}

func (c *fakeIssueScanMarkerClient) RemoveLabels(_ context.Context, _ string, _ int, labels []string) error {
	c.removedLabels = append(c.removedLabels, labels...)
	return nil
}

func (c *fakeIssueScanMarkerClient) ListCommentBodies(_ context.Context, _ string, _ int) ([]string, error) {
	return append([]string(nil), c.comments...), nil
}

func (c *fakeIssueScanMarkerClient) CreateComment(_ context.Context, _ string, _ int, body string) error {
	c.comments = append(c.comments, body)
	return nil
}

type failingIssueScanMarkerClient struct {
	err error
}

func (c *failingIssueScanMarkerClient) SourceIssueMarkerClientKind() IssueScanSourceIssueMarkerClientKind {
	return IssueScanSourceIssueMarkerMockClient
}

func (c *failingIssueScanMarkerClient) issueScanSourceIssueMarkerMockClient() {}

func (c *failingIssueScanMarkerClient) AddLabels(context.Context, string, int, []string) error {
	return c.err
}

func (c *failingIssueScanMarkerClient) RemoveLabels(context.Context, string, int, []string) error {
	return c.err
}

func (c *failingIssueScanMarkerClient) ListCommentBodies(context.Context, string, int) ([]string, error) {
	return nil, c.err
}

func (c *failingIssueScanMarkerClient) CreateComment(context.Context, string, int, string) error {
	return c.err
}

type liveIssueScanMarkerClient struct {
	calls int
}

func (c *liveIssueScanMarkerClient) SourceIssueMarkerClientKind() IssueScanSourceIssueMarkerClientKind {
	return IssueScanSourceIssueMarkerLiveGitHubClient
}

func (c *liveIssueScanMarkerClient) AddLabels(context.Context, string, int, []string) error {
	c.calls++
	return nil
}

func (c *liveIssueScanMarkerClient) RemoveLabels(context.Context, string, int, []string) error {
	c.calls++
	return nil
}

func (c *liveIssueScanMarkerClient) ListCommentBodies(context.Context, string, int) ([]string, error) {
	c.calls++
	return nil, nil
}

func (c *liveIssueScanMarkerClient) CreateComment(context.Context, string, int, string) error {
	c.calls++
	return nil
}

type selfAttestingIssueScanMarkerClient struct {
	calls int
}

func (c *selfAttestingIssueScanMarkerClient) SourceIssueMarkerClientKind() IssueScanSourceIssueMarkerClientKind {
	return IssueScanSourceIssueMarkerMockClient
}

func (c *selfAttestingIssueScanMarkerClient) AddLabels(context.Context, string, int, []string) error {
	c.calls++
	return nil
}

func (c *selfAttestingIssueScanMarkerClient) RemoveLabels(context.Context, string, int, []string) error {
	c.calls++
	return nil
}

func (c *selfAttestingIssueScanMarkerClient) ListCommentBodies(context.Context, string, int) ([]string, error) {
	c.calls++
	return nil, nil
}

func (c *selfAttestingIssueScanMarkerClient) CreateComment(context.Context, string, int, string) error {
	c.calls++
	return nil
}
