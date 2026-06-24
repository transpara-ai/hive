package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

type issueScanResearchBrief struct {
	SelectedIssue     issueScanBriefIssuePayload      `json:"selected_issue"`
	SelectionPolicy   issueScanSelectionPolicyPayload `json:"selection_policy"`
	ScannedRepos      []string                        `json:"scanned_repos"`
	ScannedIssueCount int                             `json:"scanned_issue_count"`
	CandidateIssues   []issueScanBriefIssuePayload    `json:"candidate_issues"`
}

// RecordQueuedIssueScanResearchRoleOutputs records the first read-only
// research_issue_and_repo_context role outputs from the queued issue-scan brief.
// It converts scan facts into strategist/planner packets only; later lifecycle
// stages still require their own evidence before implementation, review, PR
// readiness, Human approval, merge, or deploy can be claimed.
func (r *Runtime) RecordQueuedIssueScanResearchRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputResult, 0, len(runIDs)*2)
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RecordQueuedIssueScanResearchRoleOutput(runID)
		if err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ready {
			out = append(out, recorded...)
		}
	}
	return out, errors.Join(errs...)
}

func (r *Runtime) RecordQueuedIssueScanResearchRoleOutput(runID string) ([]IssueScanStageRoleOutputResult, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return nil, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return nil, false, err
	}
	if len(requests) == 0 {
		return nil, false, fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return nil, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return nil, false, nil
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return nil, false, err
	}
	order := factoryOrderFromRunLaunch(content, orderID)
	if _, err := r.DispatchQueuedRunLaunch(runID); err != nil {
		return nil, false, fmt.Errorf("dispatch queued issue-scan run %q before research role-output recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, false, err
	}
	stage, err := r.issueScanStageTargetByStageID(drafts, "research_issue_and_repo_context", orderID)
	if err != nil {
		return nil, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(stage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if stageCompleted {
		return nil, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(stage); err != nil {
		return nil, false, err
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return nil, false, err
	}
	results := make([]IssueScanStageRoleOutputResult, 0, 2)
	for _, output := range []IssueScanStageRoleOutputEvidence{
		issueScanResearchStrategistRoleOutput(content, orderID, stage.TaskID, brief),
		issueScanResearchPlannerRoleOutput(content, orderID, stage.TaskID, brief),
	} {
		recorded, err := r.RecordIssueScanStageRoleOutput(runID, "research_issue_and_repo_context", output)
		if err != nil {
			return results, false, err
		}
		results = append(results, recorded)
	}
	return results, true, nil
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

func issueScanResearchStrategistRoleOutput(content FactoryRunRequestedContent, orderID string, stageTaskID types.EventID, brief issueScanResearchBrief) IssueScanStageRoleOutputEvidence {
	issue := brief.SelectedIssue
	refs := issueScanResearchEvidenceRefs(content, orderID, stageTaskID, issue)
	priority := fmt.Sprintf("Selected %s#%d at rank %d of %d under %s because the scan policy ranked the issue highest for the governed Civilization run.", issue.Repo, issue.Number, brief.SelectionPolicy.SelectedRank, brief.SelectionPolicy.CandidateCount, brief.SelectionPolicy.PolicyID)
	risk := "Scope is limited to read-only research and repo-context planning; implementation, review, PR readiness, Human approval, merge, and deploy all remain later gated stages."
	return IssueScanStageRoleOutputEvidence{
		Role:         "strategist",
		Summary:      priority,
		EvidenceRefs: refs,
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "issue_priority_rationale",
				Summary:      priority,
				EvidenceRefs: refs,
			},
			{
				Key:          "risk_and_scope_notes",
				Summary:      risk,
				EvidenceRefs: refs,
			},
			{
				Key:          "issue_snapshot",
				Summary:      issueScanResearchIssueSnapshot(issue),
				EvidenceRefs: refs,
			},
		},
		AuthorityBoundary: "read_only",
		CompletionGate:    "context_packet_recorded",
		SourceRefs:        issueScanResearchSourceRefs(content, orderID, stageTaskID, issue),
	}
}

func issueScanResearchPlannerRoleOutput(content FactoryRunRequestedContent, orderID string, stageTaskID types.EventID, brief issueScanResearchBrief) IssueScanStageRoleOutputEvidence {
	issue := brief.SelectedIssue
	refs := issueScanResearchEvidenceRefs(content, orderID, stageTaskID, issue)
	repos := compactStrings(append([]string(nil), brief.ScannedRepos...))
	if len(repos) == 0 {
		repos = []string{issue.Repo}
	}
	context := fmt.Sprintf("Target repo %s was selected from scanned repos %s with %d open candidate issue(s) preserved in the run brief.", issue.Repo, strings.Join(repos, ", "), valueOrInt(brief.ScannedIssueCount, len(brief.CandidateIssues)))
	commands := fmt.Sprintf("Planner validation seed: inspect %s at the selected issue context, identify repo-local tests before mutation, then keep implementation blocked until select_and_design_approach records acceptance criteria and test plan.", issue.Repo)
	return IssueScanStageRoleOutputEvidence{
		Role:         "planner",
		Summary:      context,
		EvidenceRefs: refs,
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "repo_context_packet",
				Summary:      context,
				EvidenceRefs: refs,
			},
			{
				Key:          "candidate_validation_commands",
				Summary:      commands,
				EvidenceRefs: refs,
			},
			{
				Key:          "repo_context",
				Summary:      context,
				EvidenceRefs: refs,
			},
		},
		AuthorityBoundary: "read_only",
		CompletionGate:    "context_packet_recorded",
		SourceRefs:        issueScanResearchSourceRefs(content, orderID, stageTaskID, issue),
	}
}

func issueScanResearchIssueSnapshot(issue issueScanBriefIssuePayload) string {
	labels := compactStrings(issue.Labels)
	labelText := "none"
	if len(labels) > 0 {
		labelText = strings.Join(labels, ", ")
	}
	return truncateRunLaunchText(fmt.Sprintf("%s#%d %q labels=%s body_excerpt=%q", issue.Repo, issue.Number, issue.Title, labelText, issue.Body), 600)
}

func issueScanResearchEvidenceRefs(content FactoryRunRequestedContent, orderID string, stageTaskID types.EventID, issue issueScanBriefIssuePayload) []string {
	return compactStrings([]string{
		content.SourceEventID.Value(),
		content.BriefEventID.Value(),
		orderID,
		stageTaskID.Value(),
		issue.URL,
		fmt.Sprintf("%s#%d", issue.Repo, issue.Number),
	})
}

func issueScanResearchSourceRefs(content FactoryRunRequestedContent, orderID string, stageTaskID types.EventID, issue issueScanBriefIssuePayload) []string {
	refs := issueScanResearchEvidenceRefs(content, orderID, stageTaskID, issue)
	for _, source := range content.Sources {
		refs = append(refs, source.Ref)
	}
	return compactStrings(refs)
}

func valueOrInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}
