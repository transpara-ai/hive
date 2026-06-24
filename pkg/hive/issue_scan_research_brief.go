package hive

import (
	"encoding/json"
	"fmt"
	"strings"
)

type issueScanResearchBrief struct {
	SelectedIssue     issueScanBriefIssuePayload      `json:"selected_issue"`
	SelectionPolicy   issueScanSelectionPolicyPayload `json:"selection_policy"`
	ScannedRepos      []string                        `json:"scanned_repos"`
	ScannedIssueCount int                             `json:"scanned_issue_count"`
	CandidateIssues   []issueScanBriefIssuePayload    `json:"candidate_issues"`
}

func issueScanResearchBriefFromContent(content FactoryRunRequestedContent) (issueScanResearchBrief, error) {
	var raw struct {
		Kind             string `json:"kind"`
		LifecycleVersion string `json:"lifecycle_version"`
		issueScanResearchBrief
	}
	if err := json.Unmarshal(content.Brief, &raw); err != nil {
		return issueScanResearchBrief{}, fmt.Errorf("decode issue-scan research brief: %w", err)
	}
	if strings.TrimSpace(raw.Kind) != issueScanBriefKind {
		return issueScanResearchBrief{}, fmt.Errorf("research brief kind %q does not match %q", raw.Kind, issueScanBriefKind)
	}
	if strings.TrimSpace(raw.LifecycleVersion) != issueScanLifecycleVersion {
		return issueScanResearchBrief{}, fmt.Errorf("research brief lifecycle_version %q does not match %q", raw.LifecycleVersion, issueScanLifecycleVersion)
	}
	if strings.TrimSpace(raw.SelectedIssue.Repo) == "" || raw.SelectedIssue.Number <= 0 || strings.TrimSpace(raw.SelectedIssue.Title) == "" {
		return issueScanResearchBrief{}, fmt.Errorf("research brief selected_issue is incomplete")
	}
	if raw.SelectionPolicy.PolicyID == "" || raw.SelectionPolicy.SelectedRank <= 0 || raw.SelectionPolicy.CandidateCount <= 0 {
		return issueScanResearchBrief{}, fmt.Errorf("research brief selection_policy is incomplete")
	}
	return raw.issueScanResearchBrief, nil
}
