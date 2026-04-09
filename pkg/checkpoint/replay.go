package checkpoint

import (
	"fmt"
	"sort"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/work"
)

// BudgetState holds the recovered budget state for a single agent.
type BudgetState struct {
	AgentName       string
	CurrentBudget   int
	AdjustmentCount int
}

// CTORecoveredState holds the recovered CTO agent state.
type CTORecoveredState struct {
	GapByCategory     map[string]int  // category -> approx iteration
	DirectiveByTarget map[string]int  // target -> approx iteration
	EmittedGaps       map[string]bool // role -> already emitted
}

// SpawnerRecoveredState holds the recovered Spawner agent state.
type SpawnerRecoveredState struct {
	RecentRejections map[string]bool // role -> was rejected
	ProcessedGaps    map[string]bool // gap ID -> processed
	PendingProposal  string
}

// ReviewerRecoveredState holds the recovered Reviewer agent state.
type ReviewerRecoveredState struct {
	ReviewCounts   map[string]int  // task ID -> round count
	CompletedTasks map[string]bool // task ID -> completed
}

// ReplayBudgetFromStore replays agent.budget.adjusted events to reconstruct
// the current budget state for each agent.
func ReplayBudgetFromStore(s store.Store) (map[string]BudgetState, error) {
	result := make(map[string]BudgetState)
	if s == nil {
		return result, nil
	}

	events, err := fetchAllByType(s, event.EventTypeAgentBudgetAdjusted)
	if err != nil {
		return nil, fmt.Errorf("fetch agent.budget.adjusted: %w", err)
	}

	sortChronological(events)

	for _, ev := range events {
		c, ok := ev.Content().(event.AgentBudgetAdjustedContent)
		if !ok {
			continue
		}
		state := result[c.AgentName]
		state.AgentName = c.AgentName
		state.CurrentBudget = c.NewBudget
		state.AdjustmentCount++
		result[c.AgentName] = state
	}

	return result, nil
}

// ReplayCTOFromStore replays hive.gap.detected and hive.directive.issued
// events to reconstruct the CTO agent's mechanical state.
func ReplayCTOFromStore(s store.Store) (*CTORecoveredState, error) {
	state := &CTORecoveredState{
		GapByCategory:     make(map[string]int),
		DirectiveByTarget: make(map[string]int),
		EmittedGaps:       make(map[string]bool),
	}
	if s == nil {
		return state, nil
	}

	gapEvents, err := fetchAllByType(s, event.EventTypeGapDetected)
	if err != nil {
		return nil, fmt.Errorf("fetch hive.gap.detected: %w", err)
	}

	directiveEvents, err := fetchAllByType(s, event.EventTypeDirectiveIssued)
	if err != nil {
		return nil, fmt.Errorf("fetch hive.directive.issued: %w", err)
	}

	all := append(gapEvents, directiveEvents...)
	sortChronological(all)

	for i, ev := range all {
		switch c := ev.Content().(type) {
		case event.GapDetectedContent:
			cat := string(c.Category)
			state.GapByCategory[cat] = i
			state.EmittedGaps[c.MissingRole] = true
		case event.DirectiveIssuedContent:
			state.DirectiveByTarget[c.Target] = i
		}
	}

	return state, nil
}

// ReplaySpawnerFromStore replays hive.role.proposed, hive.role.approved, and
// hive.role.rejected events to reconstruct the Spawner agent's mechanical state.
func ReplaySpawnerFromStore(s store.Store) (*SpawnerRecoveredState, error) {
	state := &SpawnerRecoveredState{
		RecentRejections: make(map[string]bool),
		ProcessedGaps:    make(map[string]bool),
	}
	if s == nil {
		return state, nil
	}

	proposedEvents, err := fetchAllByType(s, event.EventTypeRoleProposed)
	if err != nil {
		return nil, fmt.Errorf("fetch hive.role.proposed: %w", err)
	}

	approvedEvents, err := fetchAllByType(s, event.EventTypeRoleApproved)
	if err != nil {
		return nil, fmt.Errorf("fetch hive.role.approved: %w", err)
	}

	rejectedEvents, err := fetchAllByType(s, event.EventTypeRoleRejected)
	if err != nil {
		return nil, fmt.Errorf("fetch hive.role.rejected: %w", err)
	}

	all := append(proposedEvents, approvedEvents...)
	all = append(all, rejectedEvents...)
	sortChronological(all)

	for _, ev := range all {
		switch c := ev.Content().(type) {
		case event.RoleProposedContent:
			state.PendingProposal = c.Name
		case event.RoleApprovedContent:
			// Approved: clear pending proposal if it matches, clear rejection flag.
			if state.PendingProposal == c.Name {
				state.PendingProposal = ""
			}
			delete(state.RecentRejections, c.Name)
		case event.RoleRejectedContent:
			state.RecentRejections[c.Name] = true
			if state.PendingProposal == c.Name {
				state.PendingProposal = ""
			}
		}
	}

	return state, nil
}

// ReplayReviewerFromStore replays work.task.completed and code.review.submitted
// events to reconstruct the Reviewer agent's mechanical state.
func ReplayReviewerFromStore(s store.Store) (*ReviewerRecoveredState, error) {
	state := &ReviewerRecoveredState{
		ReviewCounts:   make(map[string]int),
		CompletedTasks: make(map[string]bool),
	}
	if s == nil {
		return state, nil
	}

	completedEvents, err := fetchAllByType(s, work.EventTypeTaskCompleted)
	if err != nil {
		return nil, fmt.Errorf("fetch work.task.completed: %w", err)
	}

	reviewEvents, err := fetchAllByType(s, event.EventTypeCodeReviewSubmitted)
	if err != nil {
		return nil, fmt.Errorf("fetch code.review.submitted: %w", err)
	}

	all := append(completedEvents, reviewEvents...)
	sortChronological(all)

	for _, ev := range all {
		switch c := ev.Content().(type) {
		case work.TaskCompletedContent:
			taskID := c.TaskID.String()
			state.CompletedTasks[taskID] = true
		case event.CodeReviewContent:
			state.ReviewCounts[c.TaskID]++
		}
	}

	return state, nil
}

// fetchAllByType pages through all events of a given type using cursor pagination.
// ByType returns events in reverse-chronological order; callers must sort if needed.
func fetchAllByType(s store.Store, et types.EventType) ([]event.Event, error) {
	const pageSize = 1000
	var all []event.Event
	cursor := types.None[types.Cursor]()

	for {
		page, err := s.ByType(et, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		items := page.Items()
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}

	return all, nil
}

// sortChronological sorts events by CreatedAt ascending (oldest first).
func sortChronological(events []event.Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp().Value().Before(events[j].Timestamp().Value())
	})
}
