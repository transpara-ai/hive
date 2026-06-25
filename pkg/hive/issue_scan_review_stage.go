package hive

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

type issueScanReviewStageEvidence struct {
	ReviewEventID              types.EventID
	ReviewedTaskID             types.EventID
	ReviewStageTaskID          types.EventID
	ReviewVerdict              string
	ReviewSummary              string
	ReviewIssues               []string
	ReviewConfidence           float64
	ReviewTimestamp            time.Time
	ImplementationRoleOutputID types.EventID
	ImplementationRuntimeID    types.EventID
}

type issueScanCodeReviewEvidence struct {
	EventID   types.EventID
	Review    event.CodeReviewContent
	Timestamp time.Time
}

// RecordCompletedIssueScanReviewRoleOutputs records run_adversarial_review
// role-output artifacts once the existing Reviewer loop has produced a durable
// code.review.submitted verdict for the concrete implementation task.
func (r *Runtime) RecordCompletedIssueScanReviewRoleOutputs(result RunLaunchDispatchResult) ([]IssueScanStageRoleOutputResult, error) {
	runIDs := compactStrings(append(append([]string(nil), result.DispatchedIssueScanRunIDs...), result.AlreadyDispatchedIssueScanRunIDs...))
	out := make([]IssueScanStageRoleOutputResult, 0, len(runIDs)*2)
	var errs []error
	for _, runID := range runIDs {
		recorded, ready, err := r.RecordCompletedIssueScanReviewRoleOutput(runID)
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

// RecordCompletedIssueScanReviewRoleOutput records both required review-stage
// role outputs. The reviewer supplies exact_head_review_artifact; the guardian
// supplies finding_disposition from the same durable review verdict.
func (r *Runtime) RecordCompletedIssueScanReviewRoleOutput(runID string) ([]IssueScanStageRoleOutputResult, bool, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return nil, false, fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}
	if parked, err := r.issueScanRunIsParked(runID); err != nil {
		return nil, false, err
	} else if parked {
		return nil, false, nil
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
		return nil, false, fmt.Errorf("dispatch queued issue-scan run %q before review role-output recording: %w", runID, err)
	}
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return nil, false, err
	}
	reviewStage, err := r.issueScanStageTargetByStageID(drafts, "run_adversarial_review", orderID)
	if err != nil {
		return nil, false, err
	}
	stageCompleted, err := r.issueScanStageTaskCompleted(reviewStage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if stageCompleted {
		return nil, false, nil
	}
	if err := r.verifyIssueScanStageTaskContracts(reviewStage); err != nil {
		return nil, false, err
	}
	implementationStage, err := r.issueScanStageTargetByStageID(drafts, "implement_on_branch", orderID)
	if err != nil {
		return nil, false, err
	}
	implementationStageCompleted, err := r.issueScanStageTaskCompleted(implementationStage.TaskID)
	if err != nil {
		return nil, false, err
	}
	if !implementationStageCompleted {
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
	review, ok, err := r.issueScanReviewStageEvidence(taskID, implementationStage.TaskID, reviewStage.TaskID)
	if err != nil || !ok {
		return nil, ok, err
	}
	results := make([]IssueScanStageRoleOutputResult, 0, 2)
	for _, output := range []IssueScanStageRoleOutputEvidence{
		issueScanReviewerRoleOutputFromReview(review),
		issueScanGuardianRoleOutputFromReview(review),
	} {
		recorded, err := r.RecordIssueScanStageRoleOutput(runID, "run_adversarial_review", output)
		if err != nil {
			return results, false, err
		}
		results = append(results, recorded)
	}
	return results, true, nil
}

func (r *Runtime) issueScanReviewStageEvidence(implementationTaskID, implementationStageTaskID, reviewStageTaskID types.EventID) (issueScanReviewStageEvidence, bool, error) {
	roleOutputArtifactID, runtimeArtifactID, ok, err := r.issueScanImplementationStageEvidenceRefs(implementationStageTaskID)
	if err != nil || !ok {
		return issueScanReviewStageEvidence{}, ok, err
	}
	reviewEventID, review, reviewAt, ok, err := r.latestIssueScanCodeReviewForTask(implementationTaskID)
	if err != nil || !ok {
		return issueScanReviewStageEvidence{}, ok, err
	}
	verdict := strings.TrimSpace(review.Verdict)
	switch verdict {
	case "approve", "request_changes", "reject":
	default:
		return issueScanReviewStageEvidence{}, false, fmt.Errorf("review verdict %q is not valid", review.Verdict)
	}
	if strings.TrimSpace(review.Summary) == "" {
		return issueScanReviewStageEvidence{}, false, fmt.Errorf("review %s summary is empty", reviewEventID.Value())
	}
	return issueScanReviewStageEvidence{
		ReviewEventID:              reviewEventID,
		ReviewedTaskID:             implementationTaskID,
		ReviewStageTaskID:          reviewStageTaskID,
		ReviewVerdict:              verdict,
		ReviewSummary:              strings.TrimSpace(review.Summary),
		ReviewIssues:               compactStrings(review.Issues),
		ReviewConfidence:           review.Confidence,
		ReviewTimestamp:            reviewAt,
		ImplementationRoleOutputID: roleOutputArtifactID,
		ImplementationRuntimeID:    runtimeArtifactID,
	}, true, nil
}

func (r *Runtime) issueScanImplementationStageEvidenceRefs(stageTaskID types.EventID) (types.EventID, types.EventID, bool, error) {
	artifacts, err := r.tasks.ListArtifacts(stageTaskID)
	if err != nil {
		return types.EventID{}, types.EventID{}, false, fmt.Errorf("list implementation stage artifacts: %w", err)
	}
	var roleOutputID types.EventID
	var runtimeID types.EventID
	for _, artifact := range artifacts {
		switch strings.TrimSpace(artifact.Label) {
		case IssueScanStageRoleOutputArtifactLabel:
			roleOutputID = artifact.ID
		case IssueScanStageRuntimeEvidenceArtifactLabel:
			runtimeID = artifact.ID
		}
	}
	if roleOutputID == (types.EventID{}) || runtimeID == (types.EventID{}) {
		return types.EventID{}, types.EventID{}, false, nil
	}
	return roleOutputID, runtimeID, true, nil
}

func (r *Runtime) latestIssueScanCodeReviewForTask(taskID types.EventID) (types.EventID, event.CodeReviewContent, time.Time, bool, error) {
	reviews, err := r.issueScanCodeReviewsForTask(taskID)
	if err != nil {
		return types.EventID{}, event.CodeReviewContent{}, time.Time{}, false, err
	}
	if len(reviews) == 0 {
		return types.EventID{}, event.CodeReviewContent{}, time.Time{}, false, nil
	}
	latest := reviews[len(reviews)-1]
	return latest.EventID, latest.Review, latest.Timestamp, true, nil
}

func (r *Runtime) issueScanCodeReviewsForTask(taskID types.EventID) ([]issueScanCodeReviewEvidence, error) {
	events, err := eventsByTypePaginated(r.store, event.EventTypeCodeReviewSubmitted, defaultOperatorProjectionLimit)
	if err != nil {
		return nil, fmt.Errorf("code.review.submitted: %w", err)
	}
	reviews := []issueScanCodeReviewEvidence{}
	for _, ev := range events {
		content, ok := ev.Content().(event.CodeReviewContent)
		if !ok || strings.TrimSpace(content.TaskID) != taskID.Value() {
			continue
		}
		reviews = append(reviews, issueScanCodeReviewEvidence{
			EventID:   ev.ID(),
			Review:    content,
			Timestamp: ev.Timestamp().Value(),
		})
	}
	sort.Slice(reviews, func(i, j int) bool {
		if reviews[i].Timestamp.Equal(reviews[j].Timestamp) {
			return reviews[i].EventID.Value() < reviews[j].EventID.Value()
		}
		return reviews[i].Timestamp.Before(reviews[j].Timestamp)
	})
	return reviews, nil
}

func issueScanReviewerRoleOutputFromReview(review issueScanReviewStageEvidence) IssueScanStageRoleOutputEvidence {
	summary := fmt.Sprintf("Reviewer submitted exact-head review verdict %q for implementation task %s: %s", review.ReviewVerdict, review.ReviewedTaskID.Value(), review.ReviewSummary)
	return IssueScanStageRoleOutputEvidence{
		Role:         "reviewer",
		Summary:      summary,
		EvidenceRefs: []string{review.ReviewEventID.Value(), review.ImplementationRuntimeID.Value()},
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "exact_head_review_artifact",
				Summary:      issueScanReviewArtifactSummary(review),
				EvidenceRefs: []string{review.ReviewEventID.Value(), review.ImplementationRuntimeID.Value(), review.ImplementationRoleOutputID.Value()},
			},
		},
		SourceRefs: issueScanReviewSourceRefs(review),
	}
}

func issueScanGuardianRoleOutputFromReview(review issueScanReviewStageEvidence) IssueScanStageRoleOutputEvidence {
	return IssueScanStageRoleOutputEvidence{
		Role:         "guardian",
		Summary:      issueScanFindingDispositionSummary(review),
		EvidenceRefs: []string{review.ReviewEventID.Value()},
		Outputs: []IssueScanStageRuntimeEvidenceItem{
			{
				Key:          "finding_disposition",
				Summary:      issueScanFindingDispositionSummary(review),
				EvidenceRefs: []string{review.ReviewEventID.Value()},
			},
		},
		SourceRefs: issueScanReviewSourceRefs(review),
	}
}

func issueScanReviewArtifactSummary(review issueScanReviewStageEvidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "verdict=%s confidence=%.2f reviewed_task=%s review_event=%s", review.ReviewVerdict, review.ReviewConfidence, review.ReviewedTaskID.Value(), review.ReviewEventID.Value())
	if !review.ReviewTimestamp.IsZero() {
		fmt.Fprintf(&b, " reviewed_at=%s", review.ReviewTimestamp.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "\nsummary: %s", review.ReviewSummary)
	if len(review.ReviewIssues) > 0 {
		b.WriteString("\nissues:")
		for _, issue := range review.ReviewIssues {
			fmt.Fprintf(&b, "\n- %s", issue)
		}
	}
	return strings.TrimSpace(b.String())
}

func issueScanFindingDispositionSummary(review issueScanReviewStageEvidence) string {
	switch review.ReviewVerdict {
	case "approve":
		return "Reviewer approved the implementation with zero blocking findings in the submitted review."
	case "request_changes":
		return "Reviewer requested changes; findings are accepted as blockers for the drive_blockers_to_zero stage: " + strings.Join(issueScanSortedIssues(review.ReviewIssues), " | ")
	case "reject":
		return "Reviewer rejected the implementation; findings require human or redesign disposition before ready state: " + strings.Join(issueScanSortedIssues(review.ReviewIssues), " | ")
	default:
		return "Reviewer verdict recorded: " + review.ReviewVerdict
	}
}

func issueScanSortedIssues(issues []string) []string {
	out := compactStrings(issues)
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"no issue text supplied"}
	}
	return out
}

func issueScanReviewSourceRefs(review issueScanReviewStageEvidence) []string {
	return compactStrings([]string{
		review.ReviewedTaskID.Value(),
		review.ReviewStageTaskID.Value(),
		review.ReviewEventID.Value(),
		review.ImplementationRoleOutputID.Value(),
		review.ImplementationRuntimeID.Value(),
	})
}
