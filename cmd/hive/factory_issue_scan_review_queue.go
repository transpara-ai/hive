package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/transpara-ai/hive/pkg/hive"
)

const (
	issueScanDefaultReviewQueueThreshold = 3
	issueScanReviewQueueReadLimit        = 100
)

var defaultIssueScanReviewQueueInspector issueScanReviewQueueInspector = ghReviewQueueInspector{}

type issueScanReviewQueueInspector interface {
	ListOpenPullRequests(ctx context.Context, repos []string) (issueScanReviewQueueSnapshot, error)
}

type issueScanReviewQueueSnapshot struct {
	PullRequests []issueScanReviewQueuePullRequest
}

type issueScanReviewQueuePullRequest struct {
	Repository       string
	Number           int
	Title            string
	URL              string
	HeadSHA          string
	Author           string
	Draft            bool
	ReviewDecision   string
	MergeStateStatus string
}

type issueScanReviewQueueDecision struct {
	Threshold    int
	OpenPRCount  int
	Throttled    bool
	SourceRefs   []string
	PullRequests []issueScanReviewQueuePullRequest
}

type ghReviewQueueInspector struct{}

func (ghReviewQueueInspector) ListOpenPullRequests(ctx context.Context, repos []string) (issueScanReviewQueueSnapshot, error) {
	var out []issueScanReviewQueuePullRequest
	for _, repo := range repos {
		prs, err := scanGitHubReviewQueuePullRequests(ctx, repo)
		if err != nil {
			return issueScanReviewQueueSnapshot{}, err
		}
		out = append(out, prs...)
	}
	sortIssueScanReviewQueuePullRequests(out)
	return issueScanReviewQueueSnapshot{PullRequests: out}, nil
}

func issueScanReviewQueueThrottle(ctx context.Context, repos []string, threshold int, inspector issueScanReviewQueueInspector) (issueScanReviewQueueDecision, error) {
	var decision issueScanReviewQueueDecision
	if threshold <= 0 {
		return decision, fmt.Errorf("issue-scan review queue threshold must be greater than zero")
	}
	if len(repos) == 0 {
		return decision, fmt.Errorf("issue-scan review queue repos are required")
	}
	if threshold > issueScanReviewQueueReadLimit {
		return decision, fmt.Errorf("issue-scan review queue threshold %d exceeds readable PR cap %d", threshold, issueScanReviewQueueReadLimit)
	}
	if inspector == nil {
		inspector = defaultIssueScanReviewQueueInspector
	}
	snapshot, err := inspector.ListOpenPullRequests(ctx, repos)
	if err != nil {
		return decision, fmt.Errorf("issue-scan review-capacity throttle could not read review queue: %w", err)
	}
	prs := append([]issueScanReviewQueuePullRequest(nil), snapshot.PullRequests...)
	sortIssueScanReviewQueuePullRequests(prs)
	decision = issueScanReviewQueueDecision{
		Threshold:    threshold,
		OpenPRCount:  len(prs),
		Throttled:    len(prs) >= threshold,
		SourceRefs:   issueScanReviewQueueSourceRefs(prs),
		PullRequests: prs,
	}
	return decision, nil
}

func scanGitHubReviewQueuePullRequests(ctx context.Context, repo string) ([]issueScanReviewQueuePullRequest, error) {
	repo = strings.ToLower(strings.TrimSpace(repo))
	if !hive.ValidTransparaAIRepo(repo) {
		return nil, fmt.Errorf("review queue repo must be a transpara-ai owner/repo slug")
	}
	args := []string{
		"pr", "list",
		"--repo", repo,
		"--state", "open",
		"--limit", fmt.Sprintf("%d", issueScanReviewQueueReadLimit),
		"--json", "number,title,url,headRefOid,isDraft,reviewDecision,mergeStateStatus,author",
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list %s: %v: %s", repo, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh pr list %s: %w", repo, err)
	}
	prs, err := parseGitHubReviewQueuePullRequests(repo, output)
	if err != nil {
		return nil, fmt.Errorf("decode gh pr list %s: %w", repo, err)
	}
	return prs, nil
}

func parseGitHubReviewQueuePullRequests(repo string, output []byte) ([]issueScanReviewQueuePullRequest, error) {
	if trimmed := strings.TrimSpace(string(output)); trimmed == "" || !strings.HasPrefix(trimmed, "[") {
		return nil, fmt.Errorf("pull request list must be a JSON array")
	}
	var raw []struct {
		Number           int    `json:"number"`
		Title            string `json:"title"`
		URL              string `json:"url"`
		HeadRefOID       string `json:"headRefOid"`
		Draft            bool   `json:"isDraft"`
		ReviewDecision   string `json:"reviewDecision"`
		MergeStateStatus string `json:"mergeStateStatus"`
		Author           struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}
	prs := make([]issueScanReviewQueuePullRequest, 0, len(raw))
	for i, pr := range raw {
		if pr.Number <= 0 {
			return nil, fmt.Errorf("pull request entry %d has invalid number %d", i, pr.Number)
		}
		prs = append(prs, issueScanReviewQueuePullRequest{
			Repository:       strings.ToLower(strings.TrimSpace(repo)),
			Number:           pr.Number,
			Title:            strings.TrimSpace(pr.Title),
			URL:              strings.TrimSpace(pr.URL),
			HeadSHA:          strings.TrimSpace(pr.HeadRefOID),
			Author:           strings.TrimSpace(pr.Author.Login),
			Draft:            pr.Draft,
			ReviewDecision:   strings.TrimSpace(pr.ReviewDecision),
			MergeStateStatus: strings.TrimSpace(pr.MergeStateStatus),
		})
	}
	sortIssueScanReviewQueuePullRequests(prs)
	return prs, nil
}

func sortIssueScanReviewQueuePullRequests(prs []issueScanReviewQueuePullRequest) {
	sort.Slice(prs, func(i, j int) bool {
		if prs[i].Repository != prs[j].Repository {
			return prs[i].Repository < prs[j].Repository
		}
		return prs[i].Number < prs[j].Number
	})
}

func issueScanReviewQueueSourceRefs(prs []issueScanReviewQueuePullRequest) []string {
	refs := make([]string, 0, len(prs))
	for _, pr := range prs {
		if ref := issueScanReviewQueuePRRef(pr); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func issueScanReviewQueuePRRef(pr issueScanReviewQueuePullRequest) string {
	if strings.TrimSpace(pr.URL) != "" {
		return strings.TrimSpace(pr.URL)
	}
	if strings.TrimSpace(pr.Repository) != "" && pr.Number > 0 {
		return fmt.Sprintf("%s#%d", strings.TrimSpace(pr.Repository), pr.Number)
	}
	return ""
}

func issueScanReviewQueueThrottleError(decision issueScanReviewQueueDecision, eventRef string) error {
	refs := issueScanReviewQueueSourceRefs(decision.PullRequests)
	if len(refs) > 5 {
		refs = refs[:5]
	}
	detail := ""
	if len(refs) > 0 {
		detail = ": " + strings.Join(refs, ", ")
	}
	if strings.TrimSpace(eventRef) != "" {
		detail += fmt.Sprintf(" (throttle event %s)", strings.TrimSpace(eventRef))
	}
	return fmt.Errorf("review-capacity throttle refused issue-scan work-start: %d open PR(s) counted as unproven exact-head review load meets threshold %d%s", decision.OpenPRCount, decision.Threshold, detail)
}

func issueScanReviewQueueEventRefs(prs []issueScanReviewQueuePullRequest) []hive.IssueScanReviewCapacityPullRequestRef {
	refs := make([]hive.IssueScanReviewCapacityPullRequestRef, 0, len(prs))
	for _, pr := range prs {
		refs = append(refs, hive.IssueScanReviewCapacityPullRequestRef{
			Repository:       pr.Repository,
			Number:           pr.Number,
			URL:              pr.URL,
			Title:            pr.Title,
			HeadSHA:          pr.HeadSHA,
			Author:           pr.Author,
			Draft:            pr.Draft,
			ReviewDecision:   pr.ReviewDecision,
			MergeStateStatus: pr.MergeStateStatus,
		})
	}
	return refs
}
