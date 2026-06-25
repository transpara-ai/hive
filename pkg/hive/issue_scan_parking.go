package hive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	IssueScanParkBlockerStaleTarget     = "stale_target"
	IssueScanParkBlockerHumanScope      = "needs_human_scope"
	IssueScanParkBlockerProtectedAction = "protected_action"
	IssueScanParkBlockerDuplicateChain  = "duplicate_chain"
)

// IssueScanTargetState is optional live target issue state used to park queued
// runs after their target issue changes outside the original scan snapshot.
type IssueScanTargetState struct {
	Repository  string
	Number      int
	State       string
	StateReason string
	URL         string
	Labels      []string
}

// IssueScanTargetStateResolver resolves live issue state for an issue-scan
// target. It must not mutate GitHub or Work state.
type IssueScanTargetStateResolver func(context.Context, string, int) (IssueScanTargetState, error)

// IssueScanRunParkResult summarizes one parked or already-parked run observed
// during lifecycle progress.
type IssueScanRunParkResult struct {
	RunID             string
	FactoryOrderID    string
	Repository        string
	IssueNumber       int
	StageID           string
	BlockerType       string
	Detail            string
	RequiredAction    string
	TargetIssueState  string
	TargetIssueLabels []string
	Parked            bool
	AlreadyParked     bool
	ParkEventID       types.EventID
}

type issueScanRunParkingDecision struct {
	FactoryOrderID    string
	Repository        string
	IssueNumber       int
	StageID           string
	BlockerType       string
	Detail            string
	RequiredAction    string
	TargetIssueState  string
	TargetIssueLabels []string
	SourceRefs        []string
}

func (r *Runtime) parkBlockedIssueScanRuns(ctx context.Context, dispatch RunLaunchDispatchResult) ([]IssueScanRunParkResult, RunLaunchDispatchResult, error) {
	active := dispatch
	runIDs := compactStrings(append(append([]string(nil), dispatch.DispatchedIssueScanRunIDs...), dispatch.AlreadyDispatchedIssueScanRunIDs...))
	if len(runIDs) == 0 {
		return nil, active, nil
	}
	inactive := map[string]bool{}
	results := make([]IssueScanRunParkResult, 0)
	var errs []error
	for _, runID := range runIDs {
		existing, eventID, ok, err := r.issueScanRunParked(runID)
		if err != nil {
			inactive[runID] = true
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if ok {
			inactive[runID] = true
			results = append(results, issueScanRunParkResultFromContent(existing, true, false, eventID))
			continue
		}
		decision, shouldPark, err := r.issueScanRunParkingDecision(ctx, runID)
		if err != nil {
			inactive[runID] = true
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		if !shouldPark {
			continue
		}
		eventID, err = r.recordIssueScanRunParked(runID, decision)
		if err != nil {
			inactive[runID] = true
			errs = append(errs, fmt.Errorf("run %q: %w", runID, err))
			continue
		}
		inactive[runID] = true
		results = append(results, IssueScanRunParkResult{
			RunID:             runID,
			FactoryOrderID:    decision.FactoryOrderID,
			Repository:        decision.Repository,
			IssueNumber:       decision.IssueNumber,
			StageID:           decision.StageID,
			BlockerType:       decision.BlockerType,
			Detail:            decision.Detail,
			RequiredAction:    decision.RequiredAction,
			TargetIssueState:  decision.TargetIssueState,
			TargetIssueLabels: append([]string(nil), decision.TargetIssueLabels...),
			Parked:            true,
			ParkEventID:       eventID,
		})
	}
	if len(inactive) > 0 {
		active = filterIssueScanDispatchRuns(dispatch, inactive)
	}
	return results, active, errors.Join(errs...)
}

func (r *Runtime) issueScanRunParkingDecision(ctx context.Context, runID string) (issueScanRunParkingDecision, bool, error) {
	requests, err := fetchFactoryRunRequestedEventByRunID(r.store, runID)
	if err != nil {
		return issueScanRunParkingDecision{}, false, err
	}
	if len(requests) == 0 {
		return issueScanRunParkingDecision{}, false, fmt.Errorf("queued run %q not found", runID)
	}
	request := requests[0]
	content, ok := request.Content().(FactoryRunRequestedContent)
	if !ok {
		return issueScanRunParkingDecision{}, false, fmt.Errorf("queued run %q event has unexpected content", runID)
	}
	if !isIssueScanRunLaunch(content) {
		return issueScanRunParkingDecision{}, false, nil
	}
	brief, err := issueScanResearchBriefFromContent(content)
	if err != nil {
		return issueScanRunParkingDecision{}, false, err
	}
	orderID, err := factoryOrderIDForRunLaunch(content.RunID)
	if err != nil {
		return issueScanRunParkingDecision{}, false, err
	}
	state := IssueScanTargetState{
		Repository:  strings.ToLower(strings.TrimSpace(brief.SelectedIssue.Repo)),
		Number:      brief.SelectedIssue.Number,
		State:       strings.ToLower(strings.TrimSpace(brief.SelectedIssue.State)),
		StateReason: strings.ToLower(strings.TrimSpace(brief.SelectedIssue.StateReason)),
		URL:         strings.TrimSpace(brief.SelectedIssue.URL),
		Labels:      compactStrings(brief.SelectedIssue.Labels),
	}
	if state.State == "" {
		state.State = "open"
	}
	if r.issueScanTargetStateResolver != nil {
		resolved, err := r.issueScanTargetStateResolver(ctx, state.Repository, state.Number)
		if err != nil {
			return issueScanRunParkingDecision{}, false, fmt.Errorf("resolve target issue state: %w", err)
		}
		state = mergeIssueScanTargetState(state, resolved)
	}
	labels := issueScanLabelSet(state.Labels)
	base := issueScanRunParkingDecision{
		FactoryOrderID:    orderID,
		Repository:        state.Repository,
		IssueNumber:       state.Number,
		TargetIssueState:  state.State,
		TargetIssueLabels: append([]string(nil), state.Labels...),
		SourceRefs:        compactStrings([]string{request.ID().Value(), state.URL}),
	}
	if issueScanTargetStateClosed(state) {
		base.BlockerType = IssueScanParkBlockerStaleTarget
		base.Detail = fmt.Sprintf("%s#%d is %s", state.Repository, state.Number, valueOr(state.StateReason, state.State))
		base.RequiredAction = "confirm the issue is still in scope or queue a fresh run against a live target"
		return base, true, nil
	}
	if _, ok := labels[IssueScanNeedsHumanScopeLabel]; ok {
		base.BlockerType = IssueScanParkBlockerHumanScope
		base.Detail = fmt.Sprintf("%s#%d is labeled %s", state.Repository, state.Number, IssueScanNeedsHumanScopeLabel)
		base.RequiredAction = "human must clarify scope and remove the human-scope blocker before Hive may continue"
		return base, true, nil
	}
	if _, ok := labels[IssueScanPRDeferredLabel]; ok {
		base.BlockerType = IssueScanParkBlockerHumanScope
		base.Detail = fmt.Sprintf("%s#%d is labeled %s", state.Repository, state.Number, IssueScanPRDeferredLabel)
		base.RequiredAction = "human must move the issue to PR-ready before Hive may continue"
		return base, true, nil
	}
	if _, ok := labels[IssueScanProtectedActionLabel]; ok {
		base.BlockerType = IssueScanParkBlockerProtectedAction
		base.Detail = fmt.Sprintf("%s#%d is labeled %s", state.Repository, state.Number, IssueScanProtectedActionLabel)
		base.RequiredAction = "human must authorize the protected-action boundary before Hive may continue"
		return base, true, nil
	}
	duplicate, duplicateRefs, err := r.issueScanDuplicateStageChain(content, orderID)
	if err != nil {
		return issueScanRunParkingDecision{}, false, err
	}
	if duplicate.StageID != "" {
		base.BlockerType = IssueScanParkBlockerDuplicateChain
		base.StageID = duplicate.StageID
		base.Detail = fmt.Sprintf("stage %s has %d canonical task records for %s", duplicate.StageID, duplicate.Count, duplicate.CanonicalTaskID)
		base.RequiredAction = "repair Work canonical issue-scan chain before Hive may continue"
		base.SourceRefs = compactStrings(append(base.SourceRefs, duplicateRefs...))
		return base, true, nil
	}
	return issueScanRunParkingDecision{}, false, nil
}

func (r *Runtime) recordIssueScanRunParked(runID string, decision issueScanRunParkingDecision) (types.EventID, error) {
	if r == nil || r.store == nil || r.factory == nil {
		return types.EventID{}, fmt.Errorf("runtime store and event factory are required")
	}
	if _, eventID, ok, err := r.issueScanRunParked(runID); err != nil {
		return types.EventID{}, err
	} else if ok {
		return eventID, nil
	}
	content := IssueScanRunParkedContent{
		RunID:             strings.TrimSpace(runID),
		FactoryOrderID:    strings.TrimSpace(decision.FactoryOrderID),
		Repository:        strings.TrimSpace(decision.Repository),
		IssueNumber:       decision.IssueNumber,
		StageID:           strings.TrimSpace(decision.StageID),
		BlockerType:       strings.TrimSpace(decision.BlockerType),
		Detail:            strings.TrimSpace(decision.Detail),
		RequiredAction:    strings.TrimSpace(decision.RequiredAction),
		SourceRefs:        compactStrings(decision.SourceRefs),
		ParkedBy:          r.humanID,
		TargetIssueState:  strings.TrimSpace(decision.TargetIssueState),
		TargetIssueLabels: compactStrings(decision.TargetIssueLabels),
	}
	ev, err := r.factory.Create(EventTypeIssueScanRunParked, r.humanID, content, issueScanParkingCauses(decision.SourceRefs), runLaunchConversationID(runID, r.convID), r.store, r.signer)
	if err != nil {
		return types.EventID{}, fmt.Errorf("create issue-scan parked event: %w", err)
	}
	stored, err := r.store.Append(ev)
	if err != nil {
		return types.EventID{}, fmt.Errorf("append issue-scan parked event: %w", err)
	}
	return stored.ID(), nil
}

func (r *Runtime) issueScanRunParked(runID string) (IssueScanRunParkedContent, types.EventID, bool, error) {
	events, err := eventsByTypePaginated(r.store, EventTypeIssueScanRunParked, defaultOperatorProjectionLimit)
	if err != nil {
		return IssueScanRunParkedContent{}, types.EventID{}, false, fmt.Errorf("fetch issue-scan parked events: %w", err)
	}
	for _, ev := range events {
		content, ok := ev.Content().(IssueScanRunParkedContent)
		if ok && strings.TrimSpace(content.RunID) == strings.TrimSpace(runID) {
			return content, ev.ID(), true, nil
		}
	}
	return IssueScanRunParkedContent{}, types.EventID{}, false, nil
}

func (r *Runtime) issueScanRunIsParked(runID string) (bool, error) {
	_, _, parked, err := r.issueScanRunParked(runID)
	return parked, err
}

type issueScanDuplicateStageChain struct {
	StageID         string
	CanonicalTaskID string
	Count           int
}

func (r *Runtime) issueScanDuplicateStageChain(content FactoryRunRequestedContent, orderID string) (issueScanDuplicateStageChain, []string, error) {
	order := factoryOrderFromRunLaunch(content, orderID)
	drafts, err := issueScanLifecycleStageTaskDrafts(content, order)
	if err != nil {
		return issueScanDuplicateStageChain{}, nil, err
	}
	for _, draft := range drafts {
		matches, err := workTasksByCanonicalTaskID(r.store, draft.Options.CanonicalTaskID)
		if err != nil {
			return issueScanDuplicateStageChain{}, nil, err
		}
		if len(matches) > 1 {
			refs := make([]string, 0, len(matches))
			for _, match := range matches {
				refs = append(refs, match.Value())
			}
			return issueScanDuplicateStageChain{StageID: draft.StageID, CanonicalTaskID: draft.Options.CanonicalTaskID, Count: len(matches)}, refs, nil
		}
	}
	return issueScanDuplicateStageChain{}, nil, nil
}

func workTasksByCanonicalTaskID(s store.Store, canonicalTaskID string) ([]types.EventID, error) {
	canonicalTaskID = strings.TrimSpace(canonicalTaskID)
	if s == nil || canonicalTaskID == "" {
		return nil, nil
	}
	var out []types.EventID
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
			if strings.TrimSpace(content.CanonicalTaskID) == canonicalTaskID {
				out = append(out, ev.ID())
			}
		}
		if !page.HasMore() {
			return out, nil
		}
		cursor = page.Cursor()
	}
}

func mergeIssueScanTargetState(base, resolved IssueScanTargetState) IssueScanTargetState {
	out := base
	if strings.TrimSpace(resolved.Repository) != "" {
		out.Repository = strings.ToLower(strings.TrimSpace(resolved.Repository))
	}
	if resolved.Number > 0 {
		out.Number = resolved.Number
	}
	if strings.TrimSpace(resolved.State) != "" {
		out.State = strings.ToLower(strings.TrimSpace(resolved.State))
	}
	if strings.TrimSpace(resolved.StateReason) != "" {
		out.StateReason = strings.ToLower(strings.TrimSpace(resolved.StateReason))
	}
	if strings.TrimSpace(resolved.URL) != "" {
		out.URL = strings.TrimSpace(resolved.URL)
	}
	if len(resolved.Labels) > 0 {
		out.Labels = compactStrings(resolved.Labels)
	}
	return out
}

func issueScanTargetStateClosed(state IssueScanTargetState) bool {
	switch strings.ToLower(strings.TrimSpace(state.State)) {
	case "closed", "resolved", "done", "completed":
		return true
	default:
		return false
	}
}

func issueScanLabelSet(labels []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if label != "" {
			out[label] = struct{}{}
		}
	}
	return out
}

func issueScanParkingCauses(refs []string) []types.EventID {
	causes := make([]types.EventID, 0, len(refs))
	for _, ref := range refs {
		id, err := types.NewEventID(strings.TrimSpace(ref))
		if err == nil {
			causes = append(causes, id)
		}
	}
	return compactEventIDs(causes)
}

func issueScanRunParkResultFromContent(content IssueScanRunParkedContent, already, parked bool, eventID types.EventID) IssueScanRunParkResult {
	return IssueScanRunParkResult{
		RunID:             strings.TrimSpace(content.RunID),
		FactoryOrderID:    strings.TrimSpace(content.FactoryOrderID),
		Repository:        strings.TrimSpace(content.Repository),
		IssueNumber:       content.IssueNumber,
		StageID:           strings.TrimSpace(content.StageID),
		BlockerType:       strings.TrimSpace(content.BlockerType),
		Detail:            strings.TrimSpace(content.Detail),
		RequiredAction:    strings.TrimSpace(content.RequiredAction),
		TargetIssueState:  strings.TrimSpace(content.TargetIssueState),
		TargetIssueLabels: compactStrings(content.TargetIssueLabels),
		Parked:            parked,
		AlreadyParked:     already,
		ParkEventID:       eventID,
	}
}

func filterIssueScanDispatchRuns(dispatch RunLaunchDispatchResult, parked map[string]bool) RunLaunchDispatchResult {
	if len(parked) == 0 {
		return dispatch
	}
	filter := func(values []string) []string {
		out := make([]string, 0, len(values))
		for _, value := range values {
			if !parked[strings.TrimSpace(value)] {
				out = append(out, value)
			}
		}
		return out
	}
	dispatch.DispatchedIssueScanRunIDs = filter(dispatch.DispatchedIssueScanRunIDs)
	dispatch.AlreadyDispatchedIssueScanRunIDs = filter(dispatch.AlreadyDispatchedIssueScanRunIDs)
	return dispatch
}
