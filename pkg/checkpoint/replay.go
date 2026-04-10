package checkpoint

import (
	"encoding/json"
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

// ReplayIterationFromStore scans heartbeat and agent.stopped events to find the
// highest known iteration for each agent role. This fills the cold-start gap
// where iteration counters reset to 0, breaking cooldown windows, stabilization
// gates, and budget accounting.
func ReplayIterationFromStore(s store.Store) (map[string]int, error) {
	result := make(map[string]int)
	if s == nil {
		return result, nil
	}

	// Scan heartbeat events — these carry the most granular iteration data.
	heartbeats, err := fetchAllByType(s, EventTypeAgentHeartbeat)
	if err != nil {
		// Heartbeat events may not exist yet (first run). Not fatal.
		heartbeats = nil
	}
	for _, ev := range heartbeats {
		c, ok := ev.Content().(HeartbeatContent)
		if !ok {
			continue
		}
		if c.Iteration > result[c.Role] {
			result[c.Role] = c.Iteration
		}
	}

	// Scan agent.stopped events — these record the final iteration count.
	// AgentStoppedContent is defined in pkg/hive (can't import — circular).
	// Use structural typing: any content with Name() and Iterations() fields.
	stopped, err := fetchAllByType(s, types.MustEventType("hive.agent.stopped"))
	if err != nil {
		// May not exist in fresh stores. Not fatal.
		stopped = nil
	}
	for _, ev := range stopped {
		// AgentStoppedContent has json fields Name and Iterations.
		// Since we can't import the type, extract via json round-trip.
		// AgentStoppedContent is in pkg/hive — can't import (circular).
		// Marshal the content to JSON and extract Name + Iterations.
		raw, mErr := json.Marshal(ev.Content())
		if mErr != nil {
			continue
		}
		var fields struct {
			Name       string `json:"Name"`
			Iterations int    `json:"Iterations"`
		}
		if uErr := json.Unmarshal(raw, &fields); uErr != nil {
			continue
		}
		if fields.Name != "" && fields.Iterations > result[fields.Name] {
			result[fields.Name] = fields.Iterations
		}
	}

	return result, nil
}

// ReplayDynamicAgentsFromStore scans hive.role.approved events to discover
// dynamically spawned agent names. Returns the deduplicated list of approved
// role names so they can be included in RecoverAll alongside starter agents.
func ReplayDynamicAgentsFromStore(s store.Store) ([]string, error) {
	if s == nil {
		return nil, nil
	}

	events, err := fetchAllByType(s, event.EventTypeRoleApproved)
	if err != nil {
		return nil, fmt.Errorf("fetch hive.role.approved: %w", err)
	}

	var names []string
	seen := make(map[string]bool)
	for _, ev := range events {
		c, ok := ev.Content().(event.RoleApprovedContent)
		if !ok {
			continue
		}
		if !seen[c.Name] {
			seen[c.Name] = true
			names = append(names, c.Name)
		}
	}

	return names, nil
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
