package hive

import (
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

const issueScanDraftPRAuthorityRequestContextKind = "issue_scan_draft_pr_authority_request_context"

// IssueScanDraftPRAuthorityRequestContext is the bounded packet a guardian uses
// to request Human approval for the draft PR that will carry an issue-scan
// implementation result. It is a request packet only; it does not create,
// ready, approve, merge, or deploy a pull request.
type IssueScanDraftPRAuthorityRequestContext struct {
	Kind                 string                        `json:"kind"`
	LifecycleVersion     string                        `json:"lifecycle_version"`
	RunID                string                        `json:"run_id"`
	FactoryOrderID       string                        `json:"factory_order_id"`
	Repository           string                        `json:"repository"`
	ReadyStageTaskID     string                        `json:"ready_stage_task_id"`
	BlockerStageTaskID   string                        `json:"blocker_stage_task_id"`
	ImplementationTaskID string                        `json:"implementation_task_id"`
	SelectedIssue        IssueScanStageRoleOutputIssue `json:"selected_issue"`
	OperateBranch        string                        `json:"operate_branch"`
	OperateCommit        string                        `json:"operate_commit"`
	OperateRange         string                        `json:"operate_range,omitempty"`
	ChangedFilesSummary  string                        `json:"changed_files_summary,omitempty"`
	DraftPRTitle         string                        `json:"draft_pr_title"`
	DraftPRBody          string                        `json:"draft_pr_body"`
	DraftPRTarget        DraftPRTarget                 `json:"draft_pr_target"`
	BoundaryDisclaimers  []string                      `json:"boundary_disclaimers,omitempty"`
}

// IssueScanDraftPRAuthorityRequestResult summarizes a protected draft-PR
// request raised for an issue-scan run. A held request is successful progress:
// Human approval is still required before any PR can be created.
type IssueScanDraftPRAuthorityRequestResult struct {
	RunID                string
	FactoryOrderID       string
	Repository           string
	ReadyStageTaskID     types.EventID
	ImplementationTaskID types.EventID
	RequestID            types.EventID
	DraftPRTarget        DraftPRTarget
	DraftPRTitle         string
	DraftPRBody          string
	Raised               bool
	AlreadyRaised        bool
	HeldPendingApproval  bool
	AutoApproved         bool
}

func (r *Runtime) IssueScanDraftPRAuthorityRequestContext(runID, baseRef, baseSHA, nonce string) (IssueScanDraftPRAuthorityRequestContext, error) {
	requestContext, ready, err := r.issueScanDraftPRAuthorityRequestContext(runID, baseRef, baseSHA, nonce)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, err
	}
	if !ready {
		return IssueScanDraftPRAuthorityRequestContext{}, fmt.Errorf("issue-scan run %q is not ready for draft-PR authority request", strings.TrimSpace(runID))
	}
	return requestContext, nil
}

func (r *Runtime) RaiseIssueScanDraftPRAuthorityRequest(runID, baseRef, baseSHA, nonce string) (IssueScanDraftPRAuthorityRequestResult, error) {
	requestContext, ready, err := r.issueScanDraftPRAuthorityRequestContext(runID, baseRef, baseSHA, nonce)
	result := IssueScanDraftPRAuthorityRequestResult{RunID: strings.TrimSpace(runID)}
	if err != nil {
		return result, err
	}
	if !ready {
		return result, fmt.Errorf("issue-scan run %q is not ready for draft-PR authority request", strings.TrimSpace(runID))
	}
	result.RunID = requestContext.RunID
	result.FactoryOrderID = requestContext.FactoryOrderID
	result.Repository = requestContext.Repository
	result.DraftPRTarget = requestContext.DraftPRTarget
	result.DraftPRTitle = requestContext.DraftPRTitle
	result.DraftPRBody = requestContext.DraftPRBody
	if readyStageID, err := types.NewEventID(requestContext.ReadyStageTaskID); err == nil {
		result.ReadyStageTaskID = readyStageID
	}
	if implementationTaskID, err := types.NewEventID(requestContext.ImplementationTaskID); err == nil {
		result.ImplementationTaskID = implementationTaskID
	}

	if existing, existingTarget, ok, err := r.findIssueScanDraftPRAuthorityRequest(requestContext.DraftPRTarget); err != nil {
		return result, err
	} else if ok {
		result.RequestID = existing.RequestID
		result.DraftPRTarget = existingTarget
		result.AlreadyRaised = true
		return result, nil
	}
	if r.graph == nil || r.factory == nil || r.signer == nil {
		return result, fmt.Errorf("runtime graph dependencies are required to raise issue-scan draft-PR authority request")
	}

	requestID, err := r.RaiseDraftPRAuthorityRequest(
		requestContext.DraftPRTarget,
		r.humanID,
		fmt.Sprintf("Issue-scan run %s requests one governed draft PR for %s at head %s", requestContext.RunID, requestContext.Repository, requestContext.OperateCommit),
	)
	result.RequestID = requestID
	if err != nil {
		if issueScanDraftPRAuthorityHeld(err) && !requestID.IsZero() {
			result.Raised = true
			result.HeldPendingApproval = true
			return result, nil
		}
		return result, fmt.Errorf("raise issue-scan draft-PR authority request: %w", err)
	}
	result.Raised = true
	result.AutoApproved = true
	return result, nil
}

func (r *Runtime) issueScanDraftPRAuthorityRequestContext(runID, baseRef, baseSHA, nonce string) (IssueScanDraftPRAuthorityRequestContext, bool, error) {
	baseRef = valueOr(strings.TrimSpace(baseRef), "main")
	baseSHA = strings.TrimSpace(baseSHA)
	nonce = strings.TrimSpace(nonce)
	if baseSHA == "" {
		return IssueScanDraftPRAuthorityRequestContext{}, false, fmt.Errorf("base_sha is required")
	}
	if nonce == "" {
		return IssueScanDraftPRAuthorityRequestContext{}, false, fmt.Errorf("nonce is required")
	}
	content, orderID, _, readyStage, err := r.issueScanReadyStageTarget(runID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(readyStage.TaskID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	if stageCompleted {
		return IssueScanDraftPRAuthorityRequestContext{}, false, nil
	}
	blockerStage, implementationTaskID, implementation, err := r.issueScanReadyPrerequisites(content, orderID, readyStage)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	if _, ready, err := r.issueScanReadyStageEvidence(content, orderID, implementationTaskID, blockerStage, readyStage); err != nil || ready {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	readyArtifacts, err := r.tasks.ListArtifacts(readyStage.TaskID)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, fmt.Errorf("list ready stage artifacts: %w", err)
	}
	draftReceipts, err := issueScanDraftPRReceiptArtifacts(readyArtifacts)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	if len(draftReceipts) > 0 {
		return IssueScanDraftPRAuthorityRequestContext{}, false, nil
	}
	repo, err := issueScanReadyRunnerRepository(content)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	selectedIssue := issueScanStageRoleOutputIssueFromBriefIssue(brief.SelectedIssue)
	title := issueScanDraftPRTitle(selectedIssue)
	body := issueScanDraftPRBody(content, orderID, readyStage.TaskID, blockerStage.TaskID, implementationTaskID, implementation, selectedIssue)
	target := DraftPRTarget{
		Repository:       repo,
		BaseRef:          baseRef,
		BaseSHA:          baseSHA,
		HeadRef:          implementation.OperateBranch,
		HeadSHA:          implementation.OperateCommit,
		TitleHash:        sha256HexPrefixed([]byte(title)),
		BodyHash:         sha256HexPrefixed([]byte(body)),
		PolicyBundleID:   TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash: TransparaAIDraftPRPolicyBundleHash(),
		SingleUseNonce:   nonce,
	}
	normalizedTarget, err := normalizeTransparaAIDraftPRTarget(target)
	if err != nil {
		return IssueScanDraftPRAuthorityRequestContext{}, false, err
	}
	return IssueScanDraftPRAuthorityRequestContext{
		Kind:                 issueScanDraftPRAuthorityRequestContextKind,
		LifecycleVersion:     issueScanLifecycleVersion,
		RunID:                strings.TrimSpace(content.RunID),
		FactoryOrderID:       orderID,
		Repository:           repo,
		ReadyStageTaskID:     readyStage.TaskID.Value(),
		BlockerStageTaskID:   blockerStage.TaskID.Value(),
		ImplementationTaskID: implementationTaskID.Value(),
		SelectedIssue:        selectedIssue,
		OperateBranch:        implementation.OperateBranch,
		OperateCommit:        implementation.OperateCommit,
		OperateRange:         implementation.OperateRange,
		ChangedFilesSummary:  implementation.ChangedFilesSummary,
		DraftPRTitle:         title,
		DraftPRBody:          body,
		DraftPRTarget:        normalizedTarget,
		BoundaryDisclaimers: compactStrings([]string{
			"authority request is not PR creation",
			"authority request is not ready-for-review state",
			"authority request is not Human approval",
			"authority request is not merge or deploy authorization",
			"approved head_sha must match operate_commit before draft PR creation",
			"base branch may advance before draft PR creation; head_sha is the pinned authority invariant",
		}),
	}, true, nil
}

func (r *Runtime) findIssueScanDraftPRAuthorityRequest(target DraftPRTarget) (AuthorityRequestRecordedContent, DraftPRTarget, bool, error) {
	if r == nil || r.store == nil {
		return AuthorityRequestRecordedContent{}, DraftPRTarget{}, false, fmt.Errorf("runtime store is required")
	}
	events, err := eventsByTypePaginated(r.store, EventTypeAuthorityRequestRecorded, defaultOperatorProjectionLimit)
	if err != nil {
		return AuthorityRequestRecordedContent{}, DraftPRTarget{}, false, fmt.Errorf("load authority requests: %w", err)
	}
	for _, ev := range events {
		content, ok := ev.Content().(AuthorityRequestRecordedContent)
		if !ok || content.ActionName != string(safety.ActionRepoPullRequestCreate) {
			continue
		}
		existingTarget, err := ParseDraftPRScope(content.Scope)
		if err != nil {
			continue
		}
		if sameIssueScanDraftPRAuthorityTarget(existingTarget, target) {
			return content, existingTarget, true, nil
		}
	}
	return AuthorityRequestRecordedContent{}, DraftPRTarget{}, false, nil
}

func issueScanDraftPRAuthorityHeld(err error) bool {
	var authErr safety.AuthorityError
	return errors.As(err, &authErr) && authErr.Outcome == safety.ApprovalRequired
}

func issueScanDraftPRTitle(issue IssueScanStageRoleOutputIssue) string {
	repo := strings.ToLower(strings.TrimSpace(issue.Repo))
	title := firstNonEmptyLine(issue.Title)
	if title == "" {
		title = "Issue-scan result"
	}
	prefix := "[codex] Resolve "
	if repo != "" && issue.Number > 0 {
		prefix += fmt.Sprintf("%s#%d: ", repo, issue.Number)
	}
	return truncateIssueScanDraftPRTitle(prefix + title)
}

func issueScanDraftPRBody(content FactoryRunRequestedContent, orderID string, readyStageID, blockerStageID, implementationTaskID types.EventID, implementation issueScanOperateCompletionEvidence, issue IssueScanStageRoleOutputIssue) string {
	var b strings.Builder
	b.WriteString("## Summary\n")
	fmt.Fprintf(&b, "Issue-scan run `%s` has completed implementation, exact-head review, and blocker repair gates for FactoryOrder `%s`.\n\n", strings.TrimSpace(content.RunID), strings.TrimSpace(orderID))
	b.WriteString("This draft PR request surfaces the implementation result for Human review only.\n\n")
	b.WriteString("## Source Issue\n")
	if strings.TrimSpace(issue.Repo) != "" {
		fmt.Fprintf(&b, "- Repository: `%s`\n", strings.ToLower(strings.TrimSpace(issue.Repo)))
	}
	if issue.Number > 0 {
		fmt.Fprintf(&b, "- Issue: #%d\n", issue.Number)
	}
	if strings.TrimSpace(issue.Title) != "" {
		fmt.Fprintf(&b, "- Title: %s\n", strings.TrimSpace(issue.Title))
	}
	if strings.TrimSpace(issue.URL) != "" {
		fmt.Fprintf(&b, "- URL: %s\n", strings.TrimSpace(issue.URL))
	}
	b.WriteString("\n## Implementation Evidence\n")
	fmt.Fprintf(&b, "- Implementation task: `%s`\n", implementationTaskID.Value())
	fmt.Fprintf(&b, "- Ready stage task: `%s`\n", readyStageID.Value())
	fmt.Fprintf(&b, "- Blocker stage task: `%s`\n", blockerStageID.Value())
	fmt.Fprintf(&b, "- Branch: `%s`\n", strings.TrimSpace(implementation.OperateBranch))
	fmt.Fprintf(&b, "- Head SHA: `%s`\n", strings.TrimSpace(implementation.OperateCommit))
	if strings.TrimSpace(implementation.OperateRange) != "" {
		fmt.Fprintf(&b, "- Range: `%s`\n", strings.TrimSpace(implementation.OperateRange))
	}
	if strings.TrimSpace(implementation.ChangedFilesSummary) != "" {
		b.WriteString("\n## Changed Files\n")
		b.WriteString("```text\n")
		b.WriteString(strings.TrimSpace(implementation.ChangedFilesSummary))
		b.WriteString("\n```\n")
	}
	b.WriteString("\n## Governance Boundary\n")
	b.WriteString("- Create one draft PR only after this protected request is approved.\n")
	b.WriteString("- Draft PR creation must use the approved repository, base, head, title hash, body hash, policy bundle, and nonce.\n")
	b.WriteString("- The base branch may advance before draft PR creation; the approved head SHA remains the pinned authority invariant.\n")
	b.WriteString("- This request does not grant ready-for-review state, Human approval, merge, deploy, production migration, protected setting changes, or broader runtime authority.\n")
	b.WriteString("- Ready-for-Human PR evidence remains a later governed stage after the draft PR is created, marked ready, checked, and exact-head reviewed.\n")
	return strings.TrimSpace(b.String())
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateIssueScanDraftPRTitle(title string) string {
	title = strings.TrimSpace(strings.Join(strings.Fields(title), " "))
	const maxTitleLen = 120
	runes := []rune(title)
	if len(runes) <= maxTitleLen {
		return title
	}
	return strings.TrimSpace(string(runes[:maxTitleLen-3])) + "..."
}

func sameIssueScanDraftPRAuthorityTarget(a, b DraftPRTarget) bool {
	// BaseSHA and nonce are intentionally excluded. A zero-blocker issue-scan
	// run gets one draft-PR authority request for its implementation head; if
	// the base branch advances while Human approval is pending, the first
	// recorded target remains the authoritative request.
	return strings.EqualFold(strings.TrimSpace(a.Repository), strings.TrimSpace(b.Repository)) &&
		strings.TrimSpace(a.BaseRef) == strings.TrimSpace(b.BaseRef) &&
		strings.TrimSpace(a.HeadRef) == strings.TrimSpace(b.HeadRef) &&
		strings.TrimSpace(a.HeadSHA) == strings.TrimSpace(b.HeadSHA) &&
		strings.TrimSpace(a.TitleHash) == strings.TrimSpace(b.TitleHash) &&
		strings.TrimSpace(a.BodyHash) == strings.TrimSpace(b.BodyHash) &&
		strings.TrimSpace(a.PolicyBundleID) == strings.TrimSpace(b.PolicyBundleID) &&
		strings.TrimSpace(a.PolicyBundleHash) == strings.TrimSpace(b.PolicyBundleHash)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
