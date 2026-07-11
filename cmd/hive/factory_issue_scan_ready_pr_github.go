package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
)

type issueScanReadyPRGitHubClient struct {
	token   string
	baseURL string
	http    *http.Client
}

func newIssueScanReadyPRGitHubClient(token string) *issueScanReadyPRGitHubClient {
	return &issueScanReadyPRGitHubClient{
		token:   strings.TrimSpace(token),
		baseURL: "https://api.github.com",
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *issueScanReadyPRGitHubClient) MarkReadyForReview(ctx context.Context, mutation hive.IssueScanReadyPRFinalizerMutation) (hive.IssueScanReadyPRLiveState, bool, error) {
	// Failures before the GraphQL mutation provably leave the PR un-mutated
	// and are wrapped in ErrIssueScanMarkReadyNotMutated; anything at or after
	// the mutation stays unwrapped so the finalizer fails safe toward durable
	// blocked evidence. The bool result is true only once the managed GraphQL
	// mutation has been issued — an already-ready early return is NOT a
	// transition this run performed.
	if c == nil || strings.TrimSpace(c.token) == "" {
		return hive.IssueScanReadyPRLiveState{}, false, fmt.Errorf("%w: github ready PR client: empty token", hive.ErrIssueScanMarkReadyNotMutated)
	}
	state, _, err := c.fetchPullRequestState(ctx, mutation)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, false, fmt.Errorf("%w: %w", hive.ErrIssueScanMarkReadyNotMutated, err)
	}
	if err := validateGitHubReadyPRTarget("preflight", mutation, state); err != nil {
		return hive.IssueScanReadyPRLiveState{}, false, fmt.Errorf("%w: %w", hive.ErrIssueScanMarkReadyNotMutated, err)
	}
	if !state.Draft {
		return state, false, nil
	}
	state, nodeID, err := c.fetchPullRequestState(ctx, mutation)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, false, fmt.Errorf("%w: %w", hive.ErrIssueScanMarkReadyNotMutated, err)
	}
	if err := validateGitHubReadyPRTarget("pre-mutation", mutation, state); err != nil {
		return hive.IssueScanReadyPRLiveState{}, false, fmt.Errorf("%w: %w", hive.ErrIssueScanMarkReadyNotMutated, err)
	}
	if !state.Draft {
		return state, false, nil
	}
	if err := c.markPullRequestReadyForReview(ctx, nodeID); err != nil {
		// The mutation was DISPATCHED and failed: no client-side read can
		// prove which request produced the observed state (a timed-out
		// mutation can still commit after a reconcile GET; a third party can
		// flip the PR inside the window). Preserve the uncertainty: not the
		// proven-unmutated sentinel (so blocked evidence is recorded) and
		// not mutated=true (so remediation never re-drafts a transition this
		// run cannot prove it performed).
		return hive.IssueScanReadyPRLiveState{}, false, err
	}
	state, _, err = c.fetchPullRequestState(ctx, mutation)
	return state, true, err
}

func (c *issueScanReadyPRGitHubClient) FetchReadyPRState(ctx context.Context, mutation hive.IssueScanReadyPRFinalizerMutation) (hive.IssueScanReadyPRLiveState, error) {
	if c == nil || strings.TrimSpace(c.token) == "" {
		return hive.IssueScanReadyPRLiveState{}, fmt.Errorf("github ready PR client: empty token")
	}
	state, _, err := c.fetchPullRequestState(ctx, mutation)
	if err != nil {
		return state, err
	}
	owner, repo, err := issueScanReadyPROwnerRepo(mutation.Repository)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, err
	}
	decision, err := c.fetchPullRequestReviewDecision(ctx, owner, repo, mutation.PRNumber)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, err
	}
	state.ReviewDecision = decision
	return state, err
}

func (c *issueScanReadyPRGitHubClient) fetchPullRequestReviewDecision(ctx context.Context, owner, repo string, number int) (string, error) {
	payload := map[string]any{
		"query": `query($owner: String!, $repo: String!, $number: Int!) {
			repository(owner: $owner, name: $repo) {
				pullRequest(number: $number) {
					reviewDecision
				}
			}
		}`,
		"variables": map[string]any{
			"owner":  owner,
			"repo":   repo,
			"number": number,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphQLURL(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewDecision string `json:"reviewDecision"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("github ready PR client: decode review decision response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := ""
		if len(result.Errors) > 0 {
			msg = result.Errors[0].Message
		}
		return "", fmt.Errorf("github ready PR client: review decision returned %s: %s", resp.Status, msg)
	}
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("github ready PR client: review decision graphql error: %s", result.Errors[0].Message)
	}
	return strings.TrimSpace(result.Data.Repository.PullRequest.ReviewDecision), nil
}

func (c *issueScanReadyPRGitHubClient) fetchPullRequestState(ctx context.Context, mutation hive.IssueScanReadyPRFinalizerMutation) (hive.IssueScanReadyPRLiveState, string, error) {
	state, nodeID, err := c.fetchPullRequestIdentity(ctx, mutation)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, "", err
	}
	owner, repo, err := issueScanReadyPROwnerRepo(mutation.Repository)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, "", err
	}
	ciStatus, err := c.fetchCIStatus(ctx, owner, repo, state.HeadSHA)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, "", err
	}
	state.CIStatus = ciStatus
	return state, nodeID, nil
}

// fetchPullRequestIdentity reads PR identity and open/draft state from the
// pull-request endpoint ONLY — no commit-status or check-runs calls. The
// re-draft remediation rides this fetch so a CI-endpoint outage (a state it
// exists to remediate) can never prevent returning the PR to draft.
func (c *issueScanReadyPRGitHubClient) fetchPullRequestIdentity(ctx context.Context, mutation hive.IssueScanReadyPRFinalizerMutation) (hive.IssueScanReadyPRLiveState, string, error) {
	owner, repo, err := issueScanReadyPROwnerRepo(mutation.Repository)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, "", err
	}
	var gh struct {
		Number         int    `json:"number"`
		HTMLURL        string `json:"html_url"`
		NodeID         string `json:"node_id"`
		State          string `json:"state"`
		Draft          bool   `json:"draft"`
		MergeableState string `json:"mergeable_state"`
		Base           struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, mutation.PRNumber), &gh); err != nil {
		return hive.IssueScanReadyPRLiveState{}, "", fmt.Errorf("github ready PR client: pull request: %w", err)
	}
	mergeState := strings.TrimSpace(gh.MergeableState)
	if mergeState == "" {
		mergeState = "unknown"
	}
	state := hive.IssueScanReadyPRLiveState{
		Repository:       strings.ToLower(strings.TrimSpace(mutation.Repository)),
		PRNumber:         gh.Number,
		PRURL:            strings.TrimSpace(gh.HTMLURL),
		BaseRef:          strings.TrimSpace(gh.Base.Ref),
		BaseSHA:          strings.TrimSpace(gh.Base.SHA),
		HeadRef:          strings.TrimSpace(gh.Head.Ref),
		HeadSHA:          strings.TrimSpace(gh.Head.SHA),
		State:            strings.TrimSpace(gh.State),
		Draft:            gh.Draft,
		ReadyForReview:   !gh.Draft,
		MergeStateStatus: mergeState,
		SourceRefs:       []string{strings.TrimSpace(gh.HTMLURL)},
	}
	return state, strings.TrimSpace(gh.NodeID), nil
}

func (c *issueScanReadyPRGitHubClient) fetchCIStatus(ctx context.Context, owner, repo, sha string) (string, error) {
	statusState := ""
	var combined struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/repos/%s/%s/commits/%s/status", c.baseURL, owner, repo, sha), &combined); err != nil {
		return "", fmt.Errorf("github ready PR client: commit status: %w", err)
	}
	if combined.TotalCount > 0 {
		statusState = strings.ToLower(strings.TrimSpace(combined.State))
	}
	checkState, err := c.fetchCheckRunStatus(ctx, owner, repo, sha)
	if err != nil {
		return "", err
	}
	return combineGitHubCIStatus(statusState, checkState), nil
}

func (c *issueScanReadyPRGitHubClient) fetchCheckRunStatus(ctx context.Context, owner, repo, sha string) (string, error) {
	const perPage = 100
	seen := 0
	total := -1
	pending := false
	for page := 1; ; page++ {
		var checks struct {
			TotalCount int `json:"total_count"`
			CheckRuns  []struct {
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"check_runs"`
		}
		url := fmt.Sprintf("%s/repos/%s/%s/commits/%s/check-runs?per_page=%d&page=%d", c.baseURL, owner, repo, sha, perPage, page)
		if err := c.getJSON(ctx, url, &checks); err != nil {
			return "", fmt.Errorf("github ready PR client: check-runs: %w", err)
		}
		if checks.TotalCount == 0 {
			return "", nil
		}
		total = checks.TotalCount
		if len(checks.CheckRuns) == 0 {
			pending = true
			break
		}
		for _, check := range checks.CheckRuns {
			if !strings.EqualFold(strings.TrimSpace(check.Status), "completed") {
				pending = true
				continue
			}
			switch strings.ToLower(strings.TrimSpace(check.Conclusion)) {
			case "success", "skipped", "neutral":
			case "":
				pending = true
			default:
				return "failure", nil
			}
		}
		seen += len(checks.CheckRuns)
		if total <= 0 || seen >= total {
			break
		}
	}
	if pending || seen < total {
		return "pending", nil
	}
	return "success", nil
}

func combineGitHubCIStatus(statusState, checkState string) string {
	states := []string{strings.ToLower(strings.TrimSpace(statusState)), strings.ToLower(strings.TrimSpace(checkState))}
	hasSuccess := false
	hasPending := false
	for _, state := range states {
		switch state {
		case "failure", "error", "cancelled", "timed_out", "action_required", "startup_failure":
			return "failure"
		case "pending", "queued", "in_progress", "requested", "waiting":
			hasPending = true
		case "success", "passed", "green":
			hasSuccess = true
		}
	}
	if hasPending {
		return "pending"
	}
	if hasSuccess {
		return "success"
	}
	return "unknown"
}

// ConvertToDraft returns the already-mutated PR to draft state. It runs only
// under a recorded human mark-ready approval whose ReDraftOnFailure flag is
// set (the finalizer enforces that); it never approves, merges, or deploys.
func (c *issueScanReadyPRGitHubClient) ConvertToDraft(ctx context.Context, mutation hive.IssueScanReadyPRFinalizerMutation) (hive.IssueScanReadyPRLiveState, error) {
	if c == nil || strings.TrimSpace(c.token) == "" {
		return hive.IssueScanReadyPRLiveState{}, fmt.Errorf("github ready PR client: empty token")
	}
	state, nodeID, err := c.fetchPullRequestIdentity(ctx, mutation)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, err
	}
	if err := validateGitHubReadyPRIdentityOpen("re-draft", mutation, state); err != nil {
		return hive.IssueScanReadyPRLiveState{}, err
	}
	if state.Draft {
		return state, nil
	}
	// Human authority is supreme: never convert a human-approved PR back to
	// draft, on any caller path. The review-decision query is GraphQL-only
	// (no commit-status/check-runs dependency).
	owner, repo, err := issueScanReadyPROwnerRepo(mutation.Repository)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, err
	}
	decision, err := c.fetchPullRequestReviewDecision(ctx, owner, repo, mutation.PRNumber)
	if err != nil {
		return hive.IssueScanReadyPRLiveState{}, fmt.Errorf("github ready PR client: cannot verify review decision before re-draft: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(decision), "approved") {
		return hive.IssueScanReadyPRLiveState{}, fmt.Errorf("github ready PR client: PR #%d is human-approved; refusing to re-draft over human authority", mutation.PRNumber)
	}
	if err := c.convertPullRequestToDraft(ctx, nodeID); err != nil {
		return hive.IssueScanReadyPRLiveState{}, err
	}
	state, _, err = c.fetchPullRequestIdentity(ctx, mutation)
	return state, err
}

func (c *issueScanReadyPRGitHubClient) convertPullRequestToDraft(ctx context.Context, nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return fmt.Errorf("github ready PR client: pull request node_id is required")
	}
	payload := map[string]any{
		"query": "mutation($id: ID!) { convertPullRequestToDraft(input: { pullRequestId: $id }) { pullRequest { id isDraft } } }",
		"variables": map[string]string{
			"id": nodeID,
		},
	}
	return c.postGraphQLMutation(ctx, payload)
}

func (c *issueScanReadyPRGitHubClient) markPullRequestReadyForReview(ctx context.Context, nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return fmt.Errorf("github ready PR client: pull request node_id is required")
	}
	payload := map[string]any{
		"query": "mutation($id: ID!) { markPullRequestReadyForReview(input: { pullRequestId: $id }) { pullRequest { id isDraft } } }",
		"variables": map[string]string{
			"id": nodeID,
		},
	}
	return c.postGraphQLMutation(ctx, payload)
}

// postGraphQLMutation posts one GraphQL mutation payload and fails on any
// transport, HTTP, or GraphQL-level error.
func (c *issueScanReadyPRGitHubClient) postGraphQLMutation(ctx context.Context, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphQLURL(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("github ready PR client: decode graphql mutation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := ""
		if len(result.Errors) > 0 {
			msg = result.Errors[0].Message
		}
		return fmt.Errorf("github ready PR client: graphql returned %s: %s", resp.Status, msg)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("github ready PR client: graphql error: %s", result.Errors[0].Message)
	}
	return nil
}

func (c *issueScanReadyPRGitHubClient) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var ghErr struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&ghErr)
		return fmt.Errorf("github returned %s: %s", resp.Status, ghErr.Message)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *issueScanReadyPRGitHubClient) graphQLURL() string {
	base := strings.TrimRight(c.baseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return base + "/graphql"
}

func issueScanReadyPROwnerRepo(repository string) (string, string, error) {
	owner, repo, ok := strings.Cut(strings.TrimSpace(repository), "/")
	if !ok || strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return "", "", fmt.Errorf("repository %q is not owner/repo", repository)
	}
	return owner, repo, nil
}

func validateGitHubReadyPRTarget(label string, mutation hive.IssueScanReadyPRFinalizerMutation, state hive.IssueScanReadyPRLiveState) error {
	if state.PRNumber != mutation.PRNumber {
		return fmt.Errorf("%s github PR number %d does not match %d", label, state.PRNumber, mutation.PRNumber)
	}
	if baseRef := strings.TrimSpace(mutation.BaseRef); baseRef != "" && !strings.EqualFold(strings.TrimSpace(state.BaseRef), baseRef) {
		return fmt.Errorf("%s github PR base_ref %q does not match %q", label, state.BaseRef, baseRef)
	}
	if !strings.EqualFold(strings.TrimSpace(state.HeadSHA), mutation.HeadSHA) {
		return fmt.Errorf("%s github PR head %q does not match %q", label, state.HeadSHA, mutation.HeadSHA)
	}
	if strings.ToLower(strings.TrimSpace(state.State)) != "open" {
		return fmt.Errorf("%s github PR state %q is not open", label, state.State)
	}
	acceptedMergeStates := []string{"clean", "blocked"}
	if state.Draft {
		acceptedMergeStates = append(acceptedMergeStates, "draft")
	}
	if !issueScanReadyStatusOKForCLI(state.MergeStateStatus, acceptedMergeStates) {
		return fmt.Errorf("%s github PR merge_state_status %q is not clean", label, state.MergeStateStatus)
	}
	if !issueScanReadyStatusOKForCLI(state.CIStatus, []string{"success", "passed", "green"}) {
		return fmt.Errorf("%s github PR ci_status %q is not successful", label, state.CIStatus)
	}
	return nil
}

// validateGitHubReadyPRIdentityOpen checks only PR identity and openness.
// Re-draft is failure REMEDIATION: it must work precisely when ready-state
// health checks (CI, merge state, exact head) are failing — the states it
// exists to remediate — and draft is the strictly safer PR state, so unlike
// validateGitHubReadyPRTarget it never requires ready-state success.
func validateGitHubReadyPRIdentityOpen(label string, mutation hive.IssueScanReadyPRFinalizerMutation, state hive.IssueScanReadyPRLiveState) error {
	if state.PRNumber != mutation.PRNumber {
		return fmt.Errorf("%s github PR number %d does not match %d", label, state.PRNumber, mutation.PRNumber)
	}
	if strings.ToLower(strings.TrimSpace(state.State)) != "open" {
		return fmt.Errorf("%s github PR state %q is not open", label, state.State)
	}
	return nil
}

func issueScanReadyStatusOKForCLI(got string, accepted []string) bool {
	got = strings.ToLower(strings.TrimSpace(got))
	for _, want := range accepted {
		if got == strings.ToLower(strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
