package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	issueScanRunnerContextProbeKind   = "issue_scan_runner_context_probe"
	issueScanResearchStageID          = "research_issue_and_repo_context"
	issueScanImplementationStageID    = "implement_on_branch"
	issueScanAdversarialReviewStageID = "run_adversarial_review"
	issueScanBlockerRepairStageID     = "drive_blockers_to_zero"
	issueScanReadyForHumanPRStageID   = "surface_ready_for_Human_result_PR"
)

// IssueScanRunnerContextProbeDocument summarizes the current stored context
// packets available to issue-scan external runners for one queued run.
type IssueScanRunnerContextProbeDocument struct {
	Kind                 string                        `json:"kind"`
	LifecycleVersion     string                        `json:"lifecycle_version"`
	RunID                string                        `json:"run_id"`
	FactoryOrderID       string                        `json:"factory_order_id"`
	Repository           string                        `json:"repository,omitempty"`
	ReadyCount           int                           `json:"ready_count"`
	ErrorCount           int                           `json:"error_count"`
	ContextBuildBehavior []string                      `json:"context_build_behavior,omitempty"`
	Contexts             []IssueScanRunnerContextProbe `json:"contexts"`
	BoundaryDisclaimers  []string                      `json:"boundary_disclaimers,omitempty"`
}

type IssueScanRunnerContextProbe struct {
	ID                string          `json:"id"`
	Stage             string          `json:"stage"`
	StandaloneCommand string          `json:"standalone_command,omitempty"`
	ContextKind       string          `json:"context_kind"`
	ContextType       string          `json:"context_type"`
	Ready             bool            `json:"ready"`
	NotReadyReason    string          `json:"not_ready_reason,omitempty"`
	Error             string          `json:"error,omitempty"`
	FactoryOrderID    string          `json:"factory_order_id,omitempty"`
	Repository        string          `json:"repository,omitempty"`
	RepoPath          string          `json:"repo_path,omitempty"`
	StageID           string          `json:"stage_id,omitempty"`
	StageTaskID       string          `json:"stage_task_id,omitempty"`
	TaskID            string          `json:"task_id,omitempty"`
	GeneratedBy       string          `json:"generated_by,omitempty"`
	ContextPayload    json.RawMessage `json:"context_payload,omitempty"`
}

// ProbeIssueScanRunnerContexts returns ready/not-ready status for the concrete
// JSON contexts that can be passed to the issue-scan runner chain. This is a
// live context-builder probe: it may dispatch the queued run and materialize
// missing FactoryOrder/stage-task scaffolding, but it does not invoke runners or
// record runner results.
func (r *Runtime) ProbeIssueScanRunnerContexts(runID string, includePayload bool) (IssueScanRunnerContextProbeDocument, error) {
	return r.ProbeIssueScanRunnerContextsContext(context.Background(), runID, includePayload)
}

// ProbeIssueScanRunnerContextsContext is the cancellable form of
// ProbeIssueScanRunnerContexts.
func (r *Runtime) ProbeIssueScanRunnerContextsContext(ctx context.Context, runID string, includePayload bool) (IssueScanRunnerContextProbeDocument, error) {
	if err := issueScanRunnerContextProbeCheckContext(ctx); err != nil {
		return IssueScanRunnerContextProbeDocument{}, err
	}
	content, orderID, err := r.issueScanRunnerProbeBase(runID)
	if err != nil {
		return IssueScanRunnerContextProbeDocument{}, err
	}
	repository := issueScanRunnerProbeRepository(content)
	doc := IssueScanRunnerContextProbeDocument{
		Kind:             issueScanRunnerContextProbeKind,
		LifecycleVersion: issueScanLifecycleVersion,
		RunID:            strings.TrimSpace(content.RunID),
		FactoryOrderID:   orderID,
		Repository:       repository,
		ContextBuildBehavior: []string{
			"context builders may dispatch the queued issue-scan run and create or repair FactoryOrder/stage-task scaffolding before reporting status",
			"probe does not execute external runners or record runner output",
			"probe does not create PRs, mark PRs ready, approve, merge, deploy, or perform production migrations",
		},
		BoundaryDisclaimers: []string{
			"ready context means only that the next bounded runner input can be built from stored runtime state",
			"not-ready context means earlier governed evidence or authority is still missing",
			"ready-for-Human PR evidence remains separate from Human approval, merge, deploy, or production migration authority",
		},
	}
	for _, build := range []func() IssueScanRunnerContextProbe{
		func() IssueScanRunnerContextProbe {
			return r.probeIssueScanStageRoleOutputRunnerContext(runID, includePayload)
		},
		func() IssueScanRunnerContextProbe {
			return r.probeIssueScanImplementationRunnerContext(runID, includePayload)
		},
		func() IssueScanRunnerContextProbe {
			return r.probeIssueScanAdversarialReviewRunnerContext(runID, includePayload)
		},
		func() IssueScanRunnerContextProbe {
			return r.probeIssueScanBlockerRepairRunnerContext(runID, includePayload)
		},
		func() IssueScanRunnerContextProbe { return r.probeIssueScanReadyPRRunnerContext(runID, includePayload) },
		issueScanReadyStateReviewContextProbe,
	} {
		if err := issueScanRunnerContextProbeCheckContext(ctx); err != nil {
			return IssueScanRunnerContextProbeDocument{}, err
		}
		doc.Contexts = append(doc.Contexts, build())
	}
	for _, probe := range doc.Contexts {
		if probe.Ready {
			doc.ReadyCount++
		}
		if strings.TrimSpace(probe.Error) != "" {
			doc.ErrorCount++
		}
	}
	return doc, nil
}

func issueScanRunnerContextProbeCheckContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (r *Runtime) issueScanRunnerProbeBase(runID string) (FactoryRunRequestedContent, string, error) {
	runID = strings.TrimSpace(runID)
	if r == nil || r.store == nil || r.tasks == nil {
		return FactoryRunRequestedContent{}, "", fmt.Errorf("runtime store and task store are required")
	}
	if runID == "" {
		return FactoryRunRequestedContent{}, "", fmt.Errorf("run_id is required")
	}
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return FactoryRunRequestedContent{}, "", err
	}
	if len(requests) == 0 {
		return FactoryRunRequestedContent{}, "", fmt.Errorf("queued run %q not found", runID)
	}
	content, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		return FactoryRunRequestedContent{}, "", fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return FactoryRunRequestedContent{}, "", fmt.Errorf("queued run %q is not an issue-scan run", runID)
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return FactoryRunRequestedContent{}, "", err
	}
	return content, orderID, nil
}

func issueScanRunnerProbeRepository(content FactoryRunRequestedContent) string {
	brief, err := issueScanResearchBriefFromContent(content)
	if err == nil {
		repo := strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo))
		if repo != "" {
			return repo
		}
	}
	if len(content.TargetRepos) > 0 {
		return strings.ToLower(strings.TrimSpace(content.TargetRepos[0]))
	}
	return ""
}

func (r *Runtime) probeIssueScanStageRoleOutputRunnerContext(runID string, includePayload bool) IssueScanRunnerContextProbe {
	probe := IssueScanRunnerContextProbe{
		ID:                "stage_role_output_runner",
		Stage:             "research/debate/select/design planning stages",
		StandaloneCommand: "hive factory run-issue-scan-stage-role-output",
		ContextKind:       issueScanStageRoleOutputRunnerContextKind,
		ContextType:       "hive.IssueScanStageRoleOutputRunnerContext",
		NotReadyReason:    "no eligible planning stage currently needs role-output evidence",
	}
	runnerContext, ready, err := r.issueScanStageRoleOutputRunnerContext(runID)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	if !ready {
		return probe
	}
	probe.Ready = true
	probe.NotReadyReason = ""
	probe.FactoryOrderID = runnerContext.FactoryOrderID
	probe.Repository = runnerContext.Repository
	probe.RepoPath = runnerContext.RepoPath
	probe.StageID = runnerContext.StageID
	probe.StageTaskID = runnerContext.StageTaskID
	probe.ContextPayload = issueScanRunnerContextProbePayload(includePayload, runnerContext, &probe)
	return probe
}

func (r *Runtime) probeIssueScanImplementationRunnerContext(runID string, includePayload bool) IssueScanRunnerContextProbe {
	probe := IssueScanRunnerContextProbe{
		ID:                "implementation_runner",
		Stage:             "implementation",
		StandaloneCommand: "hive factory run-issue-scan-implementation",
		ContextKind:       issueScanImplementationRunnerContextKind,
		ContextType:       "hive.IssueScanImplementationRunnerContext",
		NotReadyReason:    "select/design is not complete or the concrete implementation task is not currently unblocked",
	}
	runnerContext, ready, err := r.issueScanImplementationRunnerContext(runID)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	if !ready {
		return probe
	}
	probe.Ready = true
	probe.NotReadyReason = ""
	probe.FactoryOrderID = runnerContext.FactoryOrderID
	probe.Repository = runnerContext.Repository
	probe.RepoPath = runnerContext.RepoPath
	probe.StageID = issueScanImplementationStageID
	probe.StageTaskID = runnerContext.ImplementationStageTaskID
	probe.TaskID = runnerContext.ImplementationTaskID
	probe.ContextPayload = issueScanRunnerContextProbePayload(includePayload, runnerContext, &probe)
	return probe
}

func (r *Runtime) probeIssueScanAdversarialReviewRunnerContext(runID string, includePayload bool) IssueScanRunnerContextProbe {
	probe := IssueScanRunnerContextProbe{
		ID:                "adversarial_review_runner",
		Stage:             issueScanAdversarialReviewStageID,
		StandaloneCommand: "hive factory run-issue-scan-review",
		ContextKind:       issueScanAdversarialReviewContextKind,
		ContextType:       "hive.IssueScanAdversarialReviewContext",
		NotReadyReason:    "implementation completion evidence has not reached an exact-head review point, or a review already covers the latest completion",
	}
	ready, err := r.issueScanAdversarialReviewRunnerShouldRun(runID)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	if !ready {
		return probe
	}
	reviewContext, err := r.IssueScanAdversarialReviewRunContext(runID)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	probe.Ready = true
	probe.NotReadyReason = ""
	probe.FactoryOrderID = reviewContext.FactoryOrderID
	probe.Repository = reviewContext.Repository
	probe.RepoPath = reviewContext.RepoPath
	probe.StageID = issueScanAdversarialReviewStageID
	probe.StageTaskID = reviewContext.ReviewStageTaskID
	probe.TaskID = reviewContext.ImplementationTaskID
	probe.ContextPayload = issueScanRunnerContextProbePayload(includePayload, reviewContext, &probe)
	return probe
}

func (r *Runtime) probeIssueScanBlockerRepairRunnerContext(runID string, includePayload bool) IssueScanRunnerContextProbe {
	probe := IssueScanRunnerContextProbe{
		ID:                "blocker_repair_runner",
		Stage:             "repair_blockers",
		StandaloneCommand: "hive factory run-issue-scan-blocker-repair",
		ContextKind:       issueScanBlockerRepairRunnerContextKind,
		ContextType:       "hive.IssueScanBlockerRepairRunnerContext",
		NotReadyReason:    "latest review is not request_changes or the reopened implementation task is not ready",
	}
	runnerContext, ready, err := r.issueScanBlockerRepairRunnerContext(runID)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	if !ready {
		return probe
	}
	probe.Ready = true
	probe.NotReadyReason = ""
	probe.FactoryOrderID = runnerContext.FactoryOrderID
	probe.Repository = runnerContext.Repository
	probe.RepoPath = runnerContext.RepoPath
	probe.StageID = issueScanBlockerRepairStageID
	probe.StageTaskID = runnerContext.BlockerStageTaskID
	probe.TaskID = runnerContext.ImplementationTaskID
	probe.ContextPayload = issueScanRunnerContextProbePayload(includePayload, runnerContext, &probe)
	return probe
}

func (r *Runtime) probeIssueScanReadyPRRunnerContext(runID string, includePayload bool) IssueScanRunnerContextProbe {
	probe := IssueScanRunnerContextProbe{
		ID:                "ready_pr_evidence_runner",
		Stage:             "ready_for_human_pr",
		StandaloneCommand: "hive factory run-issue-scan-ready-pr",
		ContextKind:       issueScanReadyPRRunnerContextKind,
		ContextType:       "hive.IssueScanReadyPRRunnerContext",
		NotReadyReason:    "zero-blocker evidence, draft PR receipt, or terminal ready stage prerequisites are not complete",
	}
	readyContext, ready, err := r.issueScanReadyPRRunnerContext(runID)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	if !ready {
		return probe
	}
	probe.Ready = true
	probe.NotReadyReason = ""
	probe.FactoryOrderID = readyContext.FactoryOrderID
	probe.Repository = readyContext.Repository
	probe.StageID = issueScanReadyForHumanPRStageID
	probe.StageTaskID = readyContext.ReadyStageTaskID
	probe.TaskID = readyContext.ImplementationTaskID
	probe.ContextPayload = issueScanRunnerContextProbePayload(includePayload, readyContext, &probe)
	return probe
}

func issueScanReadyStateReviewContextProbe() IssueScanRunnerContextProbe {
	return IssueScanRunnerContextProbe{
		ID:             "ready_state_review_runner",
		Stage:          "ready_for_human_pr finalizer review",
		ContextKind:    issueScanReadyStateReviewContextKind,
		ContextType:    "hive.IssueScanReadyStateReviewContext",
		Ready:          false,
		GeneratedBy:    "managed_ready_pr_finalizer",
		NotReadyReason: "generated inside the managed ready-PR finalizer after the approved draft PR is marked ready; it is not independently available from stored run state",
	}
}

func issueScanRunnerContextProbePayload(include bool, value any, probe *IssueScanRunnerContextProbe) json.RawMessage {
	if !include {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		probe.Error = fmt.Sprintf("marshal context payload: %v", err)
		return nil
	}
	return body
}
