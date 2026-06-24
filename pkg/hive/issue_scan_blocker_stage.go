package hive

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

type issueScanBlockerStageEvidence struct {
	ReviewedTaskID       types.EventID
	ReviewStageTaskID    types.EventID
	BlockerStageTaskID   types.EventID
	LatestApproveReview  issueScanCodeReviewEvidence
	RequestChangesReview issueScanCodeReviewEvidence
	HasBlockerRound      bool
	Reopen               issueScanReopenEvidence
	Completion           issueScanOperateCompletionEvidence
	ReviewRuntimeID      types.EventID
}

type issueScanReopenEvidence struct {
	EventID        types.EventID
	TaskID         types.EventID
	Reason         string
	Issues         []string
	CompletionRefs []types.EventID
	Timestamp      time.Time
}

// RecordCompletedIssueScanBlockerRoleOutputs records drive_blockers_to_zero
// role-output artifacts once the durable Work review loop has either produced
// a zero-blocker approval or completed a request_changes -> reopen -> repair ->
// approval round for the concrete implementation task.
func (r *Runtime) RecordCompletedIssueScanBlockerRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputResult, 0, len(runIDs)*3)
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RecordCompletedIssueScanBlockerRoleOutput(runID)
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

// RecordCompletedIssueScanBlockerRoleOutput records implementer/reviewer/
// guardian evidence for the blocker-resolution stage. It returns ready=false
// for normal waiting states: no approval yet, task reopened but not re-completed,
// or the prior review stage not yet completed.
func (r *Runtime) RecordCompletedIssueScanBlockerRoleOutput(runID string) ([]IssueScanStageRoleOutputResult, bool, error) {
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
		return nil, false, fmt.Errorf("dispatch queued issue-scan run %q before blocker role-output recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, false, err
	}
	blockerStage, err := r.issueScanStageTargetByStageID(drafts, "drive_blockers_to_zero", orderID)
	if err != nil {
		return nil, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(blockerStage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if stageCompleted {
		return nil, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(blockerStage); err != nil {
		return nil, false, err
	}
	reviewStage, err := r.issueScanStageTargetByStageID(drafts, "run_adversarial_review", orderID)
	if err != nil {
		return nil, false, err
	}
	reviewStageCompleted, err := r.issueScanStageTaskCompleted(reviewStage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if !reviewStageCompleted {
		return nil, false, nil
	}
	taskID, factoryOrderID, exists, err := workTaskByCanonicalTaskID(r.store, issueScanImplementationTaskCanonicalID(order.ID))
	if err != nil {
		return nil, false, fmt.Errorf("find concrete implementation task: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	if strings.TrimSpace(factoryOrderID) != orderID {
		return nil, false, fmt.Errorf("implementation task belongs to factory order %q, want %q", factoryOrderID, orderID)
	}
	evidence, ready, err := r.issueScanBlockerStageEvidence(taskID, reviewStage.TaskID, blockerStage.TaskID)
	if err != nil || !ready {
		return nil, ready, err
	}
	results := make([]IssueScanStageRoleOutputResult, 0, 3)
	for _, output := range []IssueScanStageRoleOutputEvidence{
		issueScanBlockerImplementerRoleOutput(evidence),
		issueScanBlockerReviewerRoleOutput(evidence),
		issueScanBlockerGuardianRoleOutput(evidence),
	} {
		recorded, err := r.RecordIssueScanStageRoleOutput(runID, "drive_blockers_to_zero", output)
		if err != nil {
			return results, false, err
		}
		results = append(results, recorded)
	}
	return results, true, nil
}

func (r *Runtime) issueScanBlockerStageEvidence(implementationTaskID, reviewStageTaskID, blockerStageTaskID types.EventID) (issueScanBlockerStageEvidence, bool, error) {
	_, runtimeArtifactID, ok, err := r.issueScanImplementationStageEvidenceRefs(reviewStageTaskID)
	if err != nil || !ok {
		return issueScanBlockerStageEvidence{}, ok, err
	}
	completion, ok, err := r.issueScanImplementationCompletionEvidence(implementationTaskID, blockerStageTaskID)
	if err != nil || !ok {
		return issueScanBlockerStageEvidence{}, ok, err
	}
	reviews, err := r.issueScanCodeReviewsForTask(implementationTaskID)
	if err != nil {
		return issueScanBlockerStageEvidence{}, false, err
	}
	if len(reviews) == 0 {
		return issueScanBlockerStageEvidence{}, false, nil
	}
	latest := reviews[len(reviews)-1]
	latestVerdict := strings.TrimSpace(latest.Review.Verdict)
	switch latestVerdict {
	case "approve":
	case "request_changes", "reject":
		return issueScanBlockerStageEvidence{}, false, nil
	default:
		return issueScanBlockerStageEvidence{}, false, fmt.Errorf("review verdict %q is not valid", latest.Review.Verdict)
	}
	if strings.TrimSpace(latest.Review.Summary) == "" {
		return issueScanBlockerStageEvidence{}, false, fmt.Errorf("review %s summary is empty", latest.EventID.Value())
	}
	if !completion.CompletionTimestamp.IsZero() && latest.Timestamp.Before(completion.CompletionTimestamp) {
		return issueScanBlockerStageEvidence{}, false, nil
	}
	evidence := issueScanBlockerStageEvidence{
		ReviewedTaskID:      implementationTaskID,
		ReviewStageTaskID:   reviewStageTaskID,
		BlockerStageTaskID:  blockerStageTaskID,
		LatestApproveReview: latest,
		Completion:          completion,
		ReviewRuntimeID:     runtimeArtifactID,
	}
	for i := len(reviews) - 2; i >= 0; i-- {
		verdict := strings.TrimSpace(reviews[i].Review.Verdict)
		switch verdict {
		case "request_changes":
			if strings.TrimSpace(reviews[i].Review.Summary) == "" {
				return issueScanBlockerStageEvidence{}, false, fmt.Errorf("review %s summary is empty", reviews[i].EventID.Value())
			}
			evidence.RequestChangesReview = reviews[i]
			evidence.HasBlockerRound = true
			i = -1
		case "approve":
			// Earlier approvals are superseded by later review rounds.
		case "reject":
			return issueScanBlockerStageEvidence{}, false, fmt.Errorf("review %s rejected implementation before latest approval; human or redesign disposition is required", reviews[i].EventID.Value())
		default:
			return issueScanBlockerStageEvidence{}, false, fmt.Errorf("review verdict %q is not valid", reviews[i].Review.Verdict)
		}
	}
	if !evidence.HasBlockerRound {
		return evidence, true, nil
	}
	reopen, ok, err := r.latestIssueScanReopenForTaskAfter(implementationTaskID, evidence.RequestChangesReview.Timestamp)
	if err != nil || !ok {
		return issueScanBlockerStageEvidence{}, ok, err
	}
	if len(reopen.CompletionRefs) == 0 {
		return issueScanBlockerStageEvidence{}, false, fmt.Errorf("reopen %s has no superseded completion refs", reopen.EventID.Value())
	}
	evidence.Reopen = reopen
	return evidence, true, nil
}

func (r *Runtime) latestIssueScanReopenForTaskAfter(taskID types.EventID, at time.Time) (issueScanReopenEvidence, bool, error) {
	page, err := r.store.ByType(work.EventTypeTaskReopened, 1000, types.None[types.Cursor]())
	if err != nil {
		return issueScanReopenEvidence{}, false, fmt.Errorf("work.task.reopened: %w", err)
	}
	var reopens []issueScanReopenEvidence
	for _, ev := range page.Items() {
		content, ok := ev.Content().(work.TaskReopenedContent)
		if !ok || content.TaskID != taskID {
			continue
		}
		timestamp := ev.Timestamp().Value()
		if timestamp.Before(at) {
			continue
		}
		reopens = append(reopens, issueScanReopenEvidence{
			EventID:        ev.ID(),
			TaskID:         content.TaskID,
			Reason:         strings.TrimSpace(content.Reason),
			Issues:         compactStrings(content.Issues),
			CompletionRefs: compactEventIDs(content.CompletionRefs),
			Timestamp:      timestamp,
		})
	}
	if len(reopens) == 0 {
		return issueScanReopenEvidence{}, false, nil
	}
	best := reopens[0]
	for _, reopen := range reopens[1:] {
		if reopen.Timestamp.After(best.Timestamp) || (reopen.Timestamp.Equal(best.Timestamp) && reopen.EventID.Value() > best.EventID.Value()) {
			best = reopen
		}
	}
	return best, true, nil
}

func issueScanBlockerImplementerRoleOutput(evidence issueScanBlockerStageEvidence) IssueScanStageRoleOutputEvidence {
	refs := issueScanBlockerEvidenceRefs(evidence)
	fixSummary := issueScanBlockerFixSummary(evidence)
	return IssueScanStageRoleOutputEvidence{
		Role:         "implementer",
		Summary:      fixSummary,
		EvidenceRefs: refs,
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "blocker_fixes",
				Summary:      fixSummary,
				EvidenceRefs: refs,
			},
			{
				Key:          "rerun_validation",
				Summary:      evidence.Completion.CompletionSummary,
				EvidenceRefs: []string{evidence.Completion.CompletionEventID.Value(), evidence.Completion.OperateArtifactID.Value()},
			},
		},
		SourceRefs: issueScanBlockerSourceRefs(evidence),
	}
}

func issueScanBlockerReviewerRoleOutput(evidence issueScanBlockerStageEvidence) IssueScanStageRoleOutputEvidence {
	summary := fmt.Sprintf("Reviewer approved the exact head after blocker-resolution evaluation: %s", strings.TrimSpace(evidence.LatestApproveReview.Review.Summary))
	if !evidence.HasBlockerRound {
		summary = fmt.Sprintf("Reviewer approved the exact head with zero blocking findings; no repair review was required: %s", strings.TrimSpace(evidence.LatestApproveReview.Review.Summary))
	}
	return IssueScanStageRoleOutputEvidence{
		Role:         "reviewer",
		Summary:      summary,
		EvidenceRefs: []string{evidence.LatestApproveReview.EventID.Value(), evidence.ReviewRuntimeID.Value()},
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "rerun_review",
				Summary:      issueScanBlockerRerunReviewSummary(evidence),
				EvidenceRefs: issueScanBlockerReviewRefs(evidence),
			},
		},
		SourceRefs: issueScanBlockerSourceRefs(evidence),
	}
}

func issueScanBlockerGuardianRoleOutput(evidence issueScanBlockerStageEvidence) IssueScanStageRoleOutputEvidence {
	summary := "Zero-blocker gate satisfied: latest review approved the implementation and no accepted blocker remains open."
	if evidence.HasBlockerRound {
		summary = fmt.Sprintf("Zero-blocker gate satisfied after request_changes repair. Accepted findings were reopened through Work and the latest review approved the repaired exact head: %s", strings.TrimSpace(evidence.LatestApproveReview.Review.Summary))
	}
	return IssueScanStageRoleOutputEvidence{
		Role:         "guardian",
		Summary:      summary,
		EvidenceRefs: issueScanBlockerEvidenceRefs(evidence),
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "zero_blocker_gate_disposition",
				Summary:      summary,
				EvidenceRefs: issueScanBlockerEvidenceRefs(evidence),
			},
		},
		SourceRefs: issueScanBlockerSourceRefs(evidence),
	}
}

func issueScanBlockerFixSummary(evidence issueScanBlockerStageEvidence) string {
	if !evidence.HasBlockerRound {
		return fmt.Sprintf("No blocker fixes were required because review %s approved the implementation with zero blocking findings.", evidence.LatestApproveReview.EventID.Value())
	}
	issues := strings.Join(issueScanSortedIssues(evidence.RequestChangesReview.Review.Issues), " | ")
	if len(evidence.Reopen.Issues) > 0 {
		issues = strings.Join(issueScanSortedIssues(evidence.Reopen.Issues), " | ")
	}
	changed := strings.TrimSpace(evidence.Completion.ChangedFilesSummary)
	if changed == "" {
		changed = "changed files recorded in Operate result"
	}
	return fmt.Sprintf("Accepted blockers from review %s were routed through reopen %s and repaired by completion %s on branch %s at %s. Findings: %s. Changed files: %s",
		evidence.RequestChangesReview.EventID.Value(),
		evidence.Reopen.EventID.Value(),
		evidence.Completion.CompletionEventID.Value(),
		evidence.Completion.OperateBranch,
		evidence.Completion.OperateCommit,
		issues,
		changed,
	)
}

func issueScanBlockerRerunReviewSummary(evidence issueScanBlockerStageEvidence) string {
	if !evidence.HasBlockerRound {
		return fmt.Sprintf("Initial exact-head review %s approved with zero blockers: %s", evidence.LatestApproveReview.EventID.Value(), strings.TrimSpace(evidence.LatestApproveReview.Review.Summary))
	}
	return fmt.Sprintf("Rerun exact-head review %s approved after request_changes review %s and reopen %s: %s",
		evidence.LatestApproveReview.EventID.Value(),
		evidence.RequestChangesReview.EventID.Value(),
		evidence.Reopen.EventID.Value(),
		strings.TrimSpace(evidence.LatestApproveReview.Review.Summary),
	)
}

func issueScanBlockerEvidenceRefs(evidence issueScanBlockerStageEvidence) []string {
	refs := []string{
		evidence.LatestApproveReview.EventID.Value(),
		evidence.Completion.CompletionEventID.Value(),
		evidence.Completion.OperateArtifactID.Value(),
		evidence.ReviewRuntimeID.Value(),
	}
	if evidence.HasBlockerRound {
		refs = append(refs,
			evidence.RequestChangesReview.EventID.Value(),
			evidence.Reopen.EventID.Value(),
		)
		for _, ref := range evidence.Reopen.CompletionRefs {
			refs = append(refs, ref.Value())
		}
	}
	return compactStrings(refs)
}

func issueScanBlockerReviewRefs(evidence issueScanBlockerStageEvidence) []string {
	refs := []string{evidence.LatestApproveReview.EventID.Value()}
	if evidence.HasBlockerRound {
		refs = append(refs, evidence.RequestChangesReview.EventID.Value(), evidence.Reopen.EventID.Value())
	}
	return compactStrings(refs)
}

func issueScanBlockerSourceRefs(evidence issueScanBlockerStageEvidence) []string {
	refs := []string{
		evidence.ReviewedTaskID.Value(),
		evidence.ReviewStageTaskID.Value(),
		evidence.BlockerStageTaskID.Value(),
		evidence.LatestApproveReview.EventID.Value(),
		evidence.Completion.CompletionEventID.Value(),
		evidence.Completion.OperateArtifactID.Value(),
		evidence.ReviewRuntimeID.Value(),
	}
	if evidence.HasBlockerRound {
		refs = append(refs,
			evidence.RequestChangesReview.EventID.Value(),
			evidence.Reopen.EventID.Value(),
		)
		for _, ref := range evidence.Reopen.CompletionRefs {
			refs = append(refs, ref.Value())
		}
	}
	return compactStrings(refs)
}
