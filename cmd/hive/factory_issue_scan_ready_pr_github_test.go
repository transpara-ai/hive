package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive"
)

func TestIssueScanReadyPRGitHubClientMarksDraftReadyAndRefetches(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	getPRCalls := 0
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			getPRCalls++
			draft := getPRCalls <= 2
			mergeableState := "clean"
			if draft {
				mergeableState = "draft"
			}
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           draft,
				"mergeable_state": mergeableState,
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			writeJSON(t, w, map[string]any{"data": map[string]any{"markPullRequestReadyForReview": map[string]any{"pullRequest": map[string]any{"id": "PR_kwDOtest", "isDraft": false}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	state, err := client.MarkReadyForReview(context.Background(), mutation)
	if err != nil {
		t.Fatalf("MarkReadyForReview: %v", err)
	}
	if graphQLCalls != 1 || getPRCalls != 3 {
		t.Fatalf("calls graphql/getPR = %d/%d, want 1/3", graphQLCalls, getPRCalls)
	}
	if state.Draft || !state.ReadyForReview || state.HeadSHA != mutation.HeadSHA || state.CIStatus != "success" {
		t.Fatalf("state = %+v, want non-draft ready success at approved head", state)
	}
}

func TestIssueScanReadyPRGitHubClientRejectsMovedHeadBeforeGraphQL(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           true,
				"mergeable_state": "draft",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": "dddddddddddddddddddddddddddddddddddddddd"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			t.Fatalf("GraphQL mutation must not run after moved head")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	_, err := client.MarkReadyForReview(context.Background(), mutation)
	if err == nil || !strings.Contains(err.Error(), "head") {
		t.Fatalf("MarkReadyForReview error = %v, want moved-head refusal", err)
	}
	if !errors.Is(err, hive.ErrIssueScanMarkReadyNotMutated) {
		t.Fatalf("a refusal before any GraphQL mutation must prove not-mutated, got %v", err)
	}
	if graphQLCalls != 0 {
		t.Fatalf("graphql calls = %d, want 0", graphQLCalls)
	}
}

func TestIssueScanReadyPRGitHubClientRejectsRetargetBeforeGraphQL(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           true,
				"mergeable_state": "draft",
				"base":            map[string]string{"ref": "release", "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			t.Fatalf("GraphQL mutation must not run after retargeted base")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	if _, err := client.MarkReadyForReview(context.Background(), mutation); err == nil || !strings.Contains(err.Error(), "base_ref") {
		t.Fatalf("MarkReadyForReview error = %v, want base_ref refusal", err)
	}
	if graphQLCalls != 0 {
		t.Fatalf("graphql calls = %d, want 0", graphQLCalls)
	}
}

func TestIssueScanReadyPRGitHubClientRejectsFailingCIBeforeGraphQL(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           true,
				"mergeable_state": "draft",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "failure", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 0, "check_runs": []map[string]string{}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			t.Fatalf("GraphQL mutation must not run when CI is failing")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	if _, err := client.MarkReadyForReview(context.Background(), mutation); err == nil || !strings.Contains(err.Error(), "ci_status") {
		t.Fatalf("MarkReadyForReview error = %v, want ci_status refusal", err)
	}
	if graphQLCalls != 0 {
		t.Fatalf("graphql calls = %d, want 0", graphQLCalls)
	}
}

func TestIssueScanReadyPRGitHubClientSkipsGraphQLWhenAlreadyReady(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	getPRCalls := 0
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			getPRCalls++
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           false,
				"mergeable_state": "clean",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			t.Fatalf("GraphQL mutation must not run when PR is already ready")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	state, err := client.MarkReadyForReview(context.Background(), mutation)
	if err != nil {
		t.Fatalf("MarkReadyForReview: %v", err)
	}
	if getPRCalls != 1 || graphQLCalls != 0 {
		t.Fatalf("calls getPR/graphql = %d/%d, want 1/0", getPRCalls, graphQLCalls)
	}
	if state.Draft || !state.ReadyForReview {
		t.Fatalf("state = %+v, want already-ready PR", state)
	}
}

// TestIssueScanReadyPRGitHubClientReDraftsDespiteFailingChecks proves re-draft
// is failure REMEDIATION: it must issue the GraphQL mutation precisely when
// the ready-state health checks (CI, merge state, exact head) are failing —
// the states it exists to remediate (CFAR hive#272 round 1, finding 4).
func TestIssueScanReadyPRGitHubClientReDraftsDespiteFailingChecks(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	getPRCalls := 0
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			getPRCalls++
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           getPRCalls > 1,
				"mergeable_state": "dirty",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": "dddddddddddddddddddddddddddddddddddddddd"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "failure", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 0, "check_runs": []map[string]string{}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			writeJSON(t, w, map[string]any{"data": map[string]any{"convertPullRequestToDraft": map[string]any{"pullRequest": map[string]any{"id": "PR_kwDOtest", "isDraft": true}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	state, err := client.ConvertToDraft(context.Background(), mutation)
	if err != nil {
		t.Fatalf("ConvertToDraft with failing checks: %v", err)
	}
	if graphQLCalls != 1 {
		t.Fatalf("graphql calls = %d, want 1 (re-draft must run despite failing checks)", graphQLCalls)
	}
	if !state.Draft {
		t.Fatalf("state = %+v, want draft after conversion", state)
	}
}

// TestIssueScanReadyPRGitHubClientReDraftsDuringCIEndpointOutage proves the
// re-draft path has no dependency on the commit-status or check-runs
// endpoints at all: a verification outage on those endpoints is a state the
// remediation must survive, so it never queries them (CFAR hive#272 round 3,
// finding 2).
func TestIssueScanReadyPRGitHubClientReDraftsDuringCIEndpointOutage(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	getPRCalls := 0
	graphQLCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			getPRCalls++
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           getPRCalls > 1,
				"mergeable_state": "unknown",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/status") || strings.HasSuffix(r.URL.Path, "/check-runs")):
			t.Fatalf("re-draft must not depend on CI endpoints (queried %s)", r.URL.Path)
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			graphQLCalls++
			writeJSON(t, w, map[string]any{"data": map[string]any{"convertPullRequestToDraft": map[string]any{"pullRequest": map[string]any{"id": "PR_kwDOtest", "isDraft": true}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	state, err := client.ConvertToDraft(context.Background(), mutation)
	if err != nil {
		t.Fatalf("ConvertToDraft during CI endpoint outage: %v", err)
	}
	if graphQLCalls != 1 || !state.Draft {
		t.Fatalf("graphql calls = %d, state = %+v; want one conversion to draft", graphQLCalls, state)
	}
}

// TestIssueScanReadyPRGitHubClientReconcilesFailedMutation proves the
// fail-safe classification of mark-ready mutation errors: only a successful
// reconcile fetch showing the PR still draft proves not-mutated; a failed
// reconcile stays indeterminate (CFAR hive#272 round 1, finding 3).
func TestIssueScanReadyPRGitHubClientReconcilesFailedMutation(t *testing.T) {
	cases := []struct {
		name           string
		reconcileOK    bool
		wantNotMutated bool
	}{
		{name: "reconcile shows still draft", reconcileOK: true, wantNotMutated: true},
		{name: "reconcile fetch fails", reconcileOK: false, wantNotMutated: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutation := readyPRGitHubMutationForTest()
			getPRCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
					getPRCalls++
					if getPRCalls > 2 && !tc.reconcileOK {
						w.WriteHeader(http.StatusBadGateway)
						writeJSON(t, w, map[string]string{"message": "temporary failure"})
						return
					}
					writeJSON(t, w, map[string]any{
						"number":          321,
						"html_url":        mutation.PRURL,
						"node_id":         "PR_kwDOtest",
						"state":           "open",
						"draft":           true,
						"mergeable_state": "draft",
						"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
						"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
					})
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
					writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
					writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
				case r.Method == http.MethodPost && r.URL.Path == "/graphql":
					w.WriteHeader(http.StatusBadGateway)
					writeJSON(t, w, map[string]string{"message": "mutation transport failure"})
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
				}
			}))
			defer srv.Close()

			client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
			_, err := client.MarkReadyForReview(context.Background(), mutation)
			if err == nil {
				t.Fatal("expected mutation failure")
			}
			if got := errors.Is(err, hive.ErrIssueScanMarkReadyNotMutated); got != tc.wantNotMutated {
				t.Fatalf("errors.Is(err, NotMutated) = %t, want %t (err=%v)", got, tc.wantNotMutated, err)
			}
		})
	}
}

// TestIssueScanReadyPRGitHubClientPostMutationFetchFailureIsIndeterminate
// proves a successful mutation whose verification fetch fails is never
// reported as proven-unmutated.
func TestIssueScanReadyPRGitHubClientPostMutationFetchFailureIsIndeterminate(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	getPRCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			getPRCalls++
			if getPRCalls > 2 {
				w.WriteHeader(http.StatusBadGateway)
				writeJSON(t, w, map[string]string{"message": "temporary failure"})
				return
			}
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           true,
				"mergeable_state": "draft",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			writeJSON(t, w, map[string]any{"data": map[string]any{"markPullRequestReadyForReview": map[string]any{"pullRequest": map[string]any{"id": "PR_kwDOtest", "isDraft": false}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	_, err := client.MarkReadyForReview(context.Background(), mutation)
	if err == nil {
		t.Fatal("expected verification fetch failure")
	}
	if errors.Is(err, hive.ErrIssueScanMarkReadyNotMutated) {
		t.Fatalf("a post-mutation fetch failure must stay indeterminate, got proven-unmutated: %v", err)
	}
}

func TestIssueScanReadyPRGitHubClientFetchesReviewDecision(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/transpara-ai/hive/pulls/321":
			writeJSON(t, w, map[string]any{
				"number":          321,
				"html_url":        mutation.PRURL,
				"node_id":         "PR_kwDOtest",
				"state":           "open",
				"draft":           false,
				"mergeable_state": "blocked",
				"base":            map[string]string{"ref": mutation.BaseRef, "sha": mutation.BaseSHA},
				"head":            map[string]string{"ref": mutation.HeadRef, "sha": mutation.HeadSHA},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			writeJSON(t, w, map[string]any{"total_count": 1, "check_runs": []map[string]string{{"status": "completed", "conclusion": "success"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			writeJSON(t, w, map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]string{"reviewDecision": "REVIEW_REQUIRED"}}}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	state, err := client.FetchReadyPRState(context.Background(), mutation)
	if err != nil {
		t.Fatalf("FetchReadyPRState: %v", err)
	}
	if state.ReviewDecision != "REVIEW_REQUIRED" {
		t.Fatalf("review decision = %q, want REVIEW_REQUIRED", state.ReviewDecision)
	}
}

func TestIssueScanReadyPRGitHubClientPaginatesCheckRunsAndFailsOnLaterPage(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			page := r.URL.Query().Get("page")
			if page == "2" {
				writeJSON(t, w, map[string]any{"total_count": 101, "check_runs": []map[string]string{{"status": "completed", "conclusion": "failure"}}})
				return
			}
			runs := make([]map[string]string, 100)
			for i := range runs {
				runs[i] = map[string]string{"status": "completed", "conclusion": "success"}
			}
			writeJSON(t, w, map[string]any{"total_count": 101, "check_runs": runs})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	status, err := client.fetchCIStatus(context.Background(), "transpara-ai", "hive", mutation.HeadSHA)
	if err != nil {
		t.Fatalf("fetchCIStatus: %v", err)
	}
	if status != "failure" {
		t.Fatalf("status = %q, want failure from second check-runs page", status)
	}
}

func TestIssueScanReadyPRGitHubClientTreatsCheckRunAPIErrorAsHardFailure(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			writeJSON(t, w, map[string]any{"state": "success", "total_count": 1})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/check-runs"):
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(t, w, map[string]string{"message": "temporary failure"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	if _, err := client.fetchCIStatus(context.Background(), "transpara-ai", "hive", mutation.HeadSHA); err == nil || !strings.Contains(err.Error(), "check-runs") {
		t.Fatalf("fetchCIStatus error = %v, want hard check-runs failure", err)
	}
}

func TestIssueScanReadyPRGitHubClientTreatsStatusAPIErrorAsHardFailure(t *testing.T) {
	mutation := readyPRGitHubMutationForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(t, w, map[string]string{"message": "temporary failure"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := &issueScanReadyPRGitHubClient{token: "token", baseURL: srv.URL, http: srv.Client()}
	if _, err := client.fetchCIStatus(context.Background(), "transpara-ai", "hive", mutation.HeadSHA); err == nil || !strings.Contains(err.Error(), "commit status") {
		t.Fatalf("fetchCIStatus error = %v, want hard commit status failure", err)
	}
}

func TestCombineGitHubCIStatusPrioritizesFailureOverPending(t *testing.T) {
	if got := combineGitHubCIStatus("pending", "failure"); got != "failure" {
		t.Fatalf("combineGitHubCIStatus pending/failure = %q, want failure", got)
	}
	if got := combineGitHubCIStatus("success", "pending"); got != "pending" {
		t.Fatalf("combineGitHubCIStatus success/pending = %q, want pending", got)
	}
	if got := combineGitHubCIStatus("success", "success"); got != "success" {
		t.Fatalf("combineGitHubCIStatus success/success = %q, want success", got)
	}
}

func readyPRGitHubMutationForTest() hive.IssueScanReadyPRFinalizerMutation {
	return hive.IssueScanReadyPRFinalizerMutation{
		Kind:                  "issue_scan_ready_pr_finalizer_mutation",
		LifecycleVersion:      "issue-scan-lifecycle-v1",
		RunID:                 "run_issue_001",
		FactoryOrderID:        "fo_run_issue_001",
		Repository:            "transpara-ai/hive",
		PRNumber:              321,
		PRURL:                 "https://github.com/transpara-ai/hive/pull/321",
		BaseRef:               "main",
		BaseSHA:               "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		HeadRef:               "codex/run-issue-001-repair",
		HeadSHA:               "cccccccccccccccccccccccccccccccccccccccc",
		DraftPRReceiptRef:     "evt_receipt",
		HumanApprovalRequired: true,
		NoMergeOrDeployClaim:  true,
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
