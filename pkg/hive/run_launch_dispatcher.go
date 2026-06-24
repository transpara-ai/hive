package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	defaultRunLaunchDispatchLimit    = 100
	defaultRunLaunchDispatchInterval = 15 * time.Second

	IssueScanExecutionPlanArtifactLabel     = "issue_scan_execution_plan"
	IssueScanLifecycleStageArtifactPrefix   = "issue_scan_lifecycle_stage_"
	issueScanExecutionPlanArtifactMediaType = "application/json"
)

type issueScanDispatchArtifact struct {
	Label     string
	MediaType string
	Body      string
}

// RunLaunchDispatchResult summarizes one dispatcher pass over queued
// factory.run.requested events.
type RunLaunchDispatchResult struct {
	Scanned              int
	Dispatched           int
	AlreadyDispatched    int
	SkippedNonQueued     int
	Failed               int
	DispatchedTaskIDs    []types.EventID
	DispatchedOrderIDs   []string
	AlreadyDispatchedIDs []string
}

// DispatchQueuedRunLaunches binds queued POST /api/hive/runs requests into the
// Work task path. Model overrides are re-resolved through the Runtime's active
// resolver before the FactoryOrder is seeded, and the later Operate path
// revalidates the same structured override artifact before provider creation.
func (r *Runtime) DispatchQueuedRunLaunches(limit int) (RunLaunchDispatchResult, error) {
	return r.dispatchQueuedRunLaunches(limit, "")
}

// DispatchQueuedRunLaunch binds one queued factory.run.requested event into the
// Work task path. It is intended for operator commands that queue a single run
// and want to dispatch only that run instead of flushing the daemon backlog.
func (r *Runtime) DispatchQueuedRunLaunch(runID string) (RunLaunchDispatchResult, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return RunLaunchDispatchResult{}, fmt.Errorf("run_id is required")
	}
	return r.dispatchQueuedRunLaunches(defaultRunLaunchDispatchLimit, runID)
}

func (r *Runtime) dispatchQueuedRunLaunches(limit int, onlyRunID string) (RunLaunchDispatchResult, error) {
	var result RunLaunchDispatchResult
	if r == nil || r.store == nil || r.tasks == nil {
		return result, nil
	}
	r.runLaunchDispatchMu.Lock()
	defer r.runLaunchDispatchMu.Unlock()

	if limit <= 0 {
		limit = defaultRunLaunchDispatchLimit
	}

	dispatched, err := dispatchedFactoryOrderIDs(r.store)
	if err != nil {
		return result, err
	}
	requests, err := fetchFactoryRunRequestedEvents(r.store, limit)
	if onlyRunID != "" {
		requests, err = fetchFactoryRunRequestedEventByRunID(r.store, onlyRunID)
	}
	if err != nil {
		return result, err
	}

	var errs []error
	matchedRequestedRun := onlyRunID == ""
	for _, request := range requests {
		result.Scanned++
		content, ok := request.Content().(FactoryRunRequestedContent)
		if !ok {
			continue
		}
		if onlyRunID != "" && content.RunID != onlyRunID {
			continue
		}
		matchedRequestedRun = true
		if status := strings.TrimSpace(content.Status); status != "" && !strings.EqualFold(status, "queued") {
			result.SkippedNonQueued++
			continue
		}
		orderID, err := factoryOrderIDForRunLaunch(content.RunID)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}
		if taskID, ok := dispatched[orderID]; ok {
			planArtifactBody, hasPlanArtifact, err := issueScanExecutionPlanArtifactBody(content)
			if err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
				continue
			}
			stageArtifacts, err := issueScanLifecycleStageArtifacts(content)
			if err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
				continue
			}
			if hasPlanArtifact {
				if err := r.ensureIssueScanExecutionPlanArtifact(content, request.ID(), taskID, planArtifactBody); err != nil {
					result.Failed++
					errs = append(errs, fmt.Errorf("run %q: repair issue-scan execution plan artifact: %w", content.RunID, err))
					continue
				}
			}
			if err := r.ensureIssueScanDispatchArtifacts(content, request.ID(), taskID, stageArtifacts); err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: repair issue-scan lifecycle stage artifacts: %w", content.RunID, err))
				continue
			}
			result.AlreadyDispatched++
			result.AlreadyDispatchedIDs = append(result.AlreadyDispatchedIDs, taskID.Value())
			continue
		}
		if err := r.validateRunLaunchDispatchModelOverrides(content); err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}
		stageArtifacts, err := issueScanLifecycleStageArtifacts(content)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}
		planArtifactBody, hasPlanArtifact, err := issueScanExecutionPlanArtifactBody(content)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: %w", content.RunID, err))
			continue
		}

		convID := runLaunchConversationID(content.RunID, r.convID)
		task, err := work.SeedFactoryOrder(r.tasks, r.humanID, factoryOrderFromRunLaunch(content, orderID), []types.EventID{request.ID()}, convID)
		if err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: seed factory order: %w", content.RunID, err))
			continue
		}
		dispatched[orderID] = task.ID
		if hasPlanArtifact {
			if err := r.ensureIssueScanExecutionPlanArtifact(content, request.ID(), task.ID, planArtifactBody); err != nil {
				result.Failed++
				errs = append(errs, fmt.Errorf("run %q: attach issue-scan execution plan artifact: %w", content.RunID, err))
				continue
			}
		}
		if err := r.ensureIssueScanDispatchArtifacts(content, request.ID(), task.ID, stageArtifacts); err != nil {
			result.Failed++
			errs = append(errs, fmt.Errorf("run %q: attach issue-scan lifecycle stage artifacts: %w", content.RunID, err))
			continue
		}
		result.Dispatched++
		result.DispatchedTaskIDs = append(result.DispatchedTaskIDs, task.ID)
		result.DispatchedOrderIDs = append(result.DispatchedOrderIDs, orderID)
	}
	if !matchedRequestedRun {
		errs = append(errs, fmt.Errorf("queued run %q not found", onlyRunID))
	}

	return result, errors.Join(errs...)
}

func (r *Runtime) ensureIssueScanExecutionPlanArtifact(content FactoryRunRequestedContent, requestID, taskID types.EventID, body string) error {
	return r.ensureIssueScanDispatchArtifact(content, requestID, taskID, issueScanDispatchArtifact{
		Label:     IssueScanExecutionPlanArtifactLabel,
		MediaType: issueScanExecutionPlanArtifactMediaType,
		Body:      body,
	})
}

func (r *Runtime) ensureIssueScanDispatchArtifacts(content FactoryRunRequestedContent, requestID, taskID types.EventID, artifacts []issueScanDispatchArtifact) error {
	for _, artifact := range artifacts {
		if err := r.ensureIssueScanDispatchArtifact(content, requestID, taskID, artifact); err != nil {
			return fmt.Errorf("%s: %w", artifact.Label, err)
		}
	}
	return nil
}

func (r *Runtime) ensureIssueScanDispatchArtifact(content FactoryRunRequestedContent, requestID, taskID types.EventID, artifact issueScanDispatchArtifact) error {
	label := strings.TrimSpace(artifact.Label)
	if label == "" {
		return fmt.Errorf("artifact label is required")
	}
	artifacts, err := r.tasks.ListArtifacts(taskID)
	if err != nil {
		return fmt.Errorf("list task artifacts: %w", err)
	}
	for _, existing := range artifacts {
		if existing.Label == label {
			return nil
		}
	}
	return r.tasks.AddArtifact(
		r.humanID,
		taskID,
		label,
		artifact.MediaType,
		artifact.Body,
		[]types.EventID{requestID, taskID},
		runLaunchConversationID(content.RunID, r.convID),
	)
}

func IssueScanLifecycleStageArtifactLabel(stageID string) string {
	return IssueScanLifecycleStageArtifactPrefix + safeRunLaunchID(stageID)
}

func issueScanExecutionPlanArtifactBody(content FactoryRunRequestedContent) (string, bool, error) {
	raw := bytes.TrimSpace(content.Brief)
	if len(raw) == 0 {
		return "", false, nil
	}
	if raw[0] != '{' {
		return "", false, nil
	}
	var meta struct {
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", false, fmt.Errorf("decode run launch brief for execution plan artifact: %w", err)
	}
	if len(meta.Kind) == 0 {
		return "", false, nil
	}
	var kind string
	if err := json.Unmarshal(meta.Kind, &kind); err != nil {
		return "", false, nil
	}
	if strings.TrimSpace(kind) != issueScanBriefKind {
		return "", false, nil
	}
	if _, _, lifecycle, agentPlan, err := queuedRunLifecycleFromBrief(raw); err != nil {
		return "", false, fmt.Errorf("validate issue-scan execution plan artifact: %w", err)
	} else if len(lifecycle) == 0 && len(agentPlan) == 0 {
		return "", false, nil
	}
	var encoded bytes.Buffer
	if err := json.Indent(&encoded, raw, "", "  "); err != nil {
		return "", false, fmt.Errorf("format issue-scan execution plan artifact: %w", err)
	}
	return encoded.String(), true, nil
}

func issueScanLifecycleStageArtifacts(content FactoryRunRequestedContent) ([]issueScanDispatchArtifact, error) {
	raw := bytes.TrimSpace(content.Brief)
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] != '{' {
		return nil, nil
	}
	var meta struct {
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("decode run launch brief for lifecycle stage artifacts: %w", err)
	}
	if len(meta.Kind) == 0 {
		return nil, nil
	}
	var kind string
	if err := json.Unmarshal(meta.Kind, &kind); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(kind) != issueScanBriefKind {
		return nil, nil
	}
	briefKind, lifecycleVersion, lifecycle, agentPlan, err := queuedRunLifecycleFromBrief(raw)
	if err != nil {
		return nil, fmt.Errorf("derive issue-scan lifecycle stage artifacts: %w", err)
	}
	if briefKind != issueScanBriefKind || len(lifecycle) == 0 {
		return nil, nil
	}
	return issueScanLifecycleStageArtifactsFromLifecycle(content.RunID, lifecycleVersion, lifecycle, agentPlan)
}

func issueScanLifecycleStageArtifactsFromLifecycle(runID, lifecycleVersion string, lifecycle []OperatorQueuedRunLifecycleStage, agentPlan []OperatorQueuedRunAgentPlanStep) ([]issueScanDispatchArtifact, error) {
	planByStage := map[string][]OperatorQueuedRunAgentPlanStep{}
	for _, step := range agentPlan {
		planByStage[step.StageID] = append(planByStage[step.StageID], step)
	}
	out := make([]issueScanDispatchArtifact, 0, len(lifecycle))
	labelsByStageID := map[string]string{}
	for i, stage := range lifecycle {
		label := IssueScanLifecycleStageArtifactLabel(stage.ID)
		if label == IssueScanLifecycleStageArtifactPrefix {
			return nil, fmt.Errorf("stage[%d] id %q does not produce a usable artifact label", i, stage.ID)
		}
		if existingStageID, ok := labelsByStageID[label]; ok {
			return nil, fmt.Errorf("stage[%d] id %q collides with stage id %q for artifact label %q", i, stage.ID, existingStageID, label)
		}
		labelsByStageID[label] = stage.ID
		body := struct {
			Kind               string                           `json:"kind"`
			LifecycleVersion   string                           `json:"lifecycle_version"`
			RunID              string                           `json:"run_id"`
			StageIndex         int                              `json:"stage_index"`
			StageCount         int                              `json:"stage_count"`
			Stage              OperatorQueuedRunLifecycleStage  `json:"stage"`
			AgentExecutionPlan []OperatorQueuedRunAgentPlanStep `json:"agent_execution_plan,omitempty"`
			EvidenceKind       string                           `json:"evidence_kind"`
			EvidenceStatus     string                           `json:"evidence_status"`
		}{
			Kind:               "issue_scan_lifecycle_stage",
			LifecycleVersion:   lifecycleVersion,
			RunID:              runID,
			StageIndex:         i + 1,
			StageCount:         len(lifecycle),
			Stage:              stage,
			AgentExecutionPlan: append([]OperatorQueuedRunAgentPlanStep(nil), planByStage[stage.ID]...),
			EvidenceKind:       "stage_declaration_not_completion",
			EvidenceStatus:     "pending_runtime_evidence",
		}
		encoded, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal issue-scan lifecycle stage artifact %q: %w", stage.ID, err)
		}
		out = append(out, issueScanDispatchArtifact{
			Label:     label,
			MediaType: issueScanExecutionPlanArtifactMediaType,
			Body:      string(encoded),
		})
	}
	return out, nil
}

func (r *Runtime) runRunLaunchDispatchLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := r.DispatchQueuedRunLaunches(defaultRunLaunchDispatchLimit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: run-launch dispatch failed closed: %v\n", err)
			}
			if result.Dispatched > 0 {
				fmt.Fprintf(os.Stderr, "Run-launch dispatcher: seeded %d FactoryOrder task(s)\n", result.Dispatched)
			}
		}
	}
}

func effectiveRunLaunchDispatchInterval(configured time.Duration) time.Duration {
	if configured < 0 {
		return 0
	}
	if configured == 0 {
		return defaultRunLaunchDispatchInterval
	}
	return configured
}

func fetchFactoryRunRequestedEvents(s store.Store, limit int) ([]event.Event, error) {
	if limit <= 0 {
		limit = defaultRunLaunchDispatchLimit
	}
	var out []event.Event
	cursor := types.None[types.Cursor]()
	for len(out) < limit {
		pageSize := limit - len(out)
		if pageSize > 100 {
			pageSize = 100
		}
		page, err := s.ByType(EventTypeFactoryRunRequested, pageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch factory.run.requested events: %w", err)
		}
		out = append(out, page.Items()...)
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	return out, nil
}

func fetchFactoryRunRequestedEventByRunID(s store.Store, runID string) ([]event.Event, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil
	}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(EventTypeFactoryRunRequested, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch factory.run.requested events: %w", err)
		}
		for _, item := range page.Items() {
			content, ok := item.Content().(FactoryRunRequestedContent)
			if ok && content.RunID == runID {
				return []event.Event{item}, nil
			}
		}
		if !page.HasMore() {
			return nil, nil
		}
		cursor = page.Cursor()
	}
}

func dispatchedFactoryOrderIDs(s store.Store) (map[string]types.EventID, error) {
	out := map[string]types.EventID{}
	cursor := types.None[types.Cursor]()
	for {
		page, err := s.ByType(work.EventTypeTaskCreated, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch work.task.created events: %w", err)
		}
		for _, ev := range page.Items() {
			content, ok := ev.Content().(work.TaskCreatedContent)
			if !ok {
				continue
			}
			orderID := strings.TrimSpace(content.FactoryOrderID)
			if orderID != "" {
				out[orderID] = ev.ID()
			}
		}
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	return out, nil
}

func (r *Runtime) validateRunLaunchDispatchModelOverrides(content FactoryRunRequestedContent) error {
	if len(content.ModelOverrides) == 0 {
		return nil
	}
	raw := make([]ModelOverrideRequest, 0, len(content.ModelOverrides))
	for _, override := range content.ModelOverrides {
		raw = append(raw, ModelOverrideRequest{
			Role:                 override.Role,
			Model:                override.Model,
			Provider:             override.Provider,
			Profile:              override.Profile,
			AuthMode:             override.RequestedAuthMode,
			PreferredTier:        override.PreferredTier,
			RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
			MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(override.MaxCostPerCallUSD),
		})
	}
	source := modelSelectionSourceWithRolePolicyUpdates(r.store, func() OperatorModelSelectionConfig {
		return OperatorModelSelectionConfig{
			Resolver:      r.currentResolver(),
			CatalogSource: "runtime-dispatcher",
			LoadedAt:      types.Now().Value(),
			ReloadMode:    operatorModelCatalogReloadMode,
			HotReload:     r.catalogReloadInterval > 0,
		}
	}, defaultOperatorProjectionLimit)
	validated, err := ValidateModelOverrides(raw, source)
	if err != nil {
		return fmt.Errorf("model overrides failed dispatch-time validation: %w", err)
	}
	if len(validated) != len(content.ModelOverrides) {
		return fmt.Errorf("model overrides validation count changed from %d to %d", len(content.ModelOverrides), len(validated))
	}
	for i := range validated {
		if err := compareRunLaunchModelOverride(content.ModelOverrides[i], validated[i]); err != nil {
			return fmt.Errorf("model_overrides[%d] for role %q failed dispatch-time validation: %w", i, content.ModelOverrides[i].Role, err)
		}
	}
	return nil
}

func compareRunLaunchModelOverride(stored, current RunLaunchModelOverride) error {
	switch {
	case !sameRole(stored.Role, current.Role):
		return fmt.Errorf("stored role %q but current validation produced %q", stored.Role, current.Role)
	case strings.TrimSpace(stored.ResolvedModel) != strings.TrimSpace(current.ResolvedModel):
		return fmt.Errorf("stored resolved_model %q but current resolver produced %q", stored.ResolvedModel, current.ResolvedModel)
	case strings.TrimSpace(stored.ResolvedProvider) != strings.TrimSpace(current.ResolvedProvider):
		return fmt.Errorf("stored resolved_provider %q but current resolver produced %q", stored.ResolvedProvider, current.ResolvedProvider)
	case strings.TrimSpace(stored.AuthMode) != strings.TrimSpace(current.AuthMode):
		return fmt.Errorf("stored auth_mode %q but current resolver produced %q", stored.AuthMode, current.AuthMode)
	default:
		return nil
	}
}

func factoryOrderFromRunLaunch(content FactoryRunRequestedContent, orderID string) work.FactoryOrder {
	return work.FactoryOrder{
		Kind:               work.OrderSoftwarePR,
		ID:                 orderID,
		Title:              content.Title,
		Intent:             runLaunchIntent(content),
		Cell:               "implementation",
		RiskClass:          runLaunchRiskClass(content.Authority.InitialLevel),
		DefinitionOfDone:   runLaunchDefinitionOfDone(content),
		AcceptanceCriteria: runLaunchAcceptanceCriteria(content),
		TestPlan:           runLaunchTestPlan(content),
		ExpectedOutputs:    runLaunchExpectedOutputs(content),
		ModelOverrides:     workModelOverridesFromRunLaunch(content.ModelOverrides),
	}
}

func runLaunchExpectedOutputs(content FactoryRunRequestedContent) []string {
	if isIssueScanRunLaunch(content) {
		return []string{
			"ready-for-Human result pull request",
			"exact-head adversarial review with zero blockers",
			"validation evidence and operator-facing status update",
		}
	}
	return []string{
		"draft pull request or governed execution artifact",
		"validation evidence",
		"operator-facing status update",
	}
}

func isIssueScanRunLaunch(content FactoryRunRequestedContent) bool {
	var brief struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(content.Brief, &brief); err != nil {
		return false
	}
	return strings.TrimSpace(brief.Kind) == issueScanBriefKind
}

func workModelOverridesFromRunLaunch(overrides []RunLaunchModelOverride) []work.FactoryOrderModelOverride {
	if len(overrides) == 0 {
		return nil
	}
	out := make([]work.FactoryOrderModelOverride, 0, len(overrides))
	for _, override := range overrides {
		out = append(out, work.FactoryOrderModelOverride{
			Role:                 override.Role,
			Model:                override.Model,
			Provider:             override.Provider,
			Profile:              override.Profile,
			RequestedAuthMode:    override.RequestedAuthMode,
			PreferredTier:        override.PreferredTier,
			RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
			MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(override.MaxCostPerCallUSD),
			ResolvedModel:        override.ResolvedModel,
			ResolvedProvider:     override.ResolvedProvider,
			AuthMode:             override.AuthMode,
		})
	}
	return out
}

func runLaunchIntent(content FactoryRunRequestedContent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Operator-requested Hive run %s.\n\n", content.RunID)
	if len(content.TargetRepos) > 0 {
		fmt.Fprintf(&b, "Target repositories: %s.\n\n", strings.Join(content.TargetRepos, ", "))
	}
	if len(content.Sources) > 0 {
		b.WriteString("Sources:\n")
		for _, source := range content.Sources {
			label := strings.TrimSpace(source.Title)
			if label == "" {
				label = strings.TrimSpace(source.Ref)
			}
			fmt.Fprintf(&b, "- %s: %s\n", strings.TrimSpace(source.Type), label)
		}
		b.WriteString("\n")
	}
	if brief := prettyRunLaunchBrief(content.Brief); brief != "" {
		b.WriteString("Brief:\n")
		b.WriteString(brief)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func runLaunchDefinitionOfDone(content FactoryRunRequestedContent) string {
	return fmt.Sprintf("Complete the operator-requested Hive run %s against the declared target repositories without exceeding the recorded authority and budget envelope.", content.RunID)
}

func runLaunchAcceptanceCriteria(content FactoryRunRequestedContent) string {
	var b strings.Builder
	b.WriteString("- Work is causally traceable to the factory.run.requested event.\n")
	b.WriteString("- Changes stay within the declared authority scope and target repositories.\n")
	b.WriteString("- Any model overrides remain structured FactoryOrder state and pass dispatch-time and Operate-time resolver validation.\n")
	if content.Budget.MaxIterations > 0 {
		fmt.Fprintf(&b, "- Iterations stay within the requested maximum of %d.\n", content.Budget.MaxIterations)
	}
	if content.Budget.MaxCostUSD >= 0 {
		fmt.Fprintf(&b, "- Cost stays within the requested maximum of %.2f USD.\n", content.Budget.MaxCostUSD)
	}
	return strings.TrimSpace(b.String())
}

func runLaunchTestPlan(content FactoryRunRequestedContent) string {
	if len(content.TargetRepos) == 0 {
		return "Run the repository-native validation commands for every touched repository and record the evidence."
	}
	return fmt.Sprintf("Run repository-native validation for %s and record the exact commands and results.", strings.Join(content.TargetRepos, ", "))
}

func prettyRunLaunchBrief(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	encoded, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func runLaunchRiskClass(level event.AuthorityLevel) string {
	switch strings.ToLower(strings.TrimSpace(string(level))) {
	case strings.ToLower(string(event.AuthorityLevelNotification)):
		return "low"
	case strings.ToLower(string(event.AuthorityLevelRecommended)):
		return "medium"
	case strings.ToLower(string(event.AuthorityLevelRequired)):
		return "high"
	default:
		return "medium"
	}
}

func factoryOrderIDForRunLaunch(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	suffix := strings.TrimPrefix(runID, "run_")
	var b strings.Builder
	for _, r := range suffix {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
		case r == '-':
			b.WriteByte('_')
		}
	}
	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		return "", fmt.Errorf("run_id %q does not contain a usable factory-order suffix", runID)
	}
	return "fo_run_" + normalized, nil
}

func cloneRunLaunchFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
