package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

const organicGovernancePageSize = 200

type organicGovernanceState struct {
	role string

	gap         event.Event
	gapContent  event.GapDetectedContent
	proposal    event.Event
	proposalDef event.RoleProposedContent
	approval    event.Event
	rejection   event.Event
	budget      event.Event

	proposalCount int
	decisionCount int
	budgetCount   int
}

// normalizeOrganicGovernanceRole is intentionally byte-for-byte equivalent to
// the runtime admission normalizer. It turns a human gap label into the one
// canonical role name the Spawner is permitted to propose.
func normalizeOrganicGovernanceRole(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	separator := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			separator = false
			b.WriteRune(r)
		case r == '-', r == '_', r == ' ', r == '\t', r == '\n', r == '\r':
			separator = true
		default:
			separator = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func exactSingleCause(ev event.Event, want types.EventID) bool {
	causes := ev.Causes()
	return len(causes) == 1 && causes[0] == want
}

func (l *Loop) organicSourceRole(ev event.Event) (string, error) {
	if l.config.ActorResolver == nil {
		return "", fmt.Errorf("actor resolver is required")
	}
	displayName := l.config.ActorResolver(ev.Source())
	role := normalizeOrganicGovernanceRole(displayName)
	if role == "" {
		return "", fmt.Errorf("source %s has no verified display name", ev.Source())
	}
	return role, nil
}

func (l *Loop) organicConversationEvents() ([]event.Event, error) {
	cursor := types.None[types.Cursor]()
	var events []event.Event
	for {
		page, err := l.agent.Graph().Store().ByConversation(
			l.agent.ConversationID(), organicGovernancePageSize, cursor,
		)
		if err != nil {
			return nil, fmt.Errorf("read current conversation: %w", err)
		}
		events = append(events, page.Items()...)
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
	return events, nil
}

// readOrganicGovernanceState projects the one organic-v1 governance sequence
// from durable events. Every anchor is source-verified and every cross-actor
// edge is exact and direct. Duplicate normalized CTO gaps are allowed, but the
// earliest is immutable; duplicate proposals, decisions, or grants are errors.
func (l *Loop) readOrganicGovernanceState() (organicGovernanceState, error) {
	var state organicGovernanceState
	if !l.config.EnforceOrganicGovernanceCausality {
		return state, nil
	}
	if l.agent == nil {
		return state, fmt.Errorf("agent is required")
	}
	events, err := l.organicConversationEvents()
	if err != nil {
		return state, err
	}

	for _, ev := range events {
		switch content := ev.Content().(type) {
		case event.GapDetectedContent:
			role := normalizeOrganicGovernanceRole(content.MissingRole)
			if role == "" {
				continue
			}
			sourceRole, err := l.organicSourceRole(ev)
			if err != nil || sourceRole != "cto" {
				return state, fmt.Errorf("gap %s has unverified CTO source", ev.ID())
			}
			if state.gap.ID().IsZero() {
				state.gap = ev
				state.gapContent = content
				state.role = role
			}

		case event.RoleProposedContent:
			state.proposalCount++
			if state.gap.ID().IsZero() {
				return state, fmt.Errorf("proposal %s precedes a genuine CTO gap", ev.ID())
			}
			sourceRole, err := l.organicSourceRole(ev)
			if err != nil || sourceRole != "spawner" || normalizeOrganicGovernanceRole(content.ProposedBy) != "spawner" {
				return state, fmt.Errorf("proposal %s has unverified Spawner source", ev.ID())
			}
			if content.Name != state.role {
				return state, fmt.Errorf("proposal %s name %q does not equal frozen role %q", ev.ID(), content.Name, state.role)
			}
			if !exactSingleCause(ev, state.gap.ID()) {
				return state, fmt.Errorf("proposal %s is not caused directly and only by gap %s", ev.ID(), state.gap.ID())
			}
			if state.proposalCount > 1 {
				return state, fmt.Errorf("conversation has multiple role proposals")
			}
			state.proposal = ev
			state.proposalDef = content

		case event.RoleApprovedContent:
			state.decisionCount++
			if err := l.validateOrganicDecisionEvent(ev, content.Name, content.ApprovedBy, "guardian", state); err != nil {
				return state, err
			}
			if state.decisionCount > 1 {
				return state, fmt.Errorf("conversation has multiple Guardian decisions")
			}
			state.approval = ev

		case event.RoleRejectedContent:
			state.decisionCount++
			if err := l.validateOrganicDecisionEvent(ev, content.Name, content.RejectedBy, "guardian", state); err != nil {
				return state, err
			}
			if state.decisionCount > 1 {
				return state, fmt.Errorf("conversation has multiple Guardian decisions")
			}
			state.rejection = ev

		case event.AgentBudgetAdjustedContent:
			// Ordinary bootstrap-role reallocations are outside the candidate
			// sequence. Only the frozen role's pre-admission grant is projected.
			if state.role == "" || normalizeOrganicGovernanceRole(content.AgentName) != state.role {
				continue
			}
			state.budgetCount++
			if state.approval.ID().IsZero() || !state.rejection.ID().IsZero() {
				return state, fmt.Errorf("budget %s has no unique prior Guardian approval", ev.ID())
			}
			sourceRole, err := l.organicSourceRole(ev)
			if err != nil || sourceRole != "allocator" {
				return state, fmt.Errorf("budget %s has unverified Allocator source", ev.ID())
			}
			if !content.AdjustsIterations() || content.Action != "set" || content.PreviousBudget != 0 {
				return state, fmt.Errorf("budget %s is not an exact pre-admission iteration set", ev.ID())
			}
			if !exactSingleCause(ev, state.approval.ID()) {
				return state, fmt.Errorf("budget %s is not caused directly and only by approval %s", ev.ID(), state.approval.ID())
			}
			if state.budgetCount > 1 {
				return state, fmt.Errorf("conversation has multiple pre-admission budgets for %q", state.role)
			}
			state.budget = ev
		}
	}
	return state, nil
}

func (l *Loop) validateOrganicDecisionEvent(ev event.Event, name, claimedBy, wantSource string, state organicGovernanceState) error {
	if state.proposal.ID().IsZero() {
		return fmt.Errorf("decision %s precedes the unique proposal", ev.ID())
	}
	sourceRole, err := l.organicSourceRole(ev)
	if err != nil || sourceRole != wantSource || normalizeOrganicGovernanceRole(claimedBy) != wantSource {
		return fmt.Errorf("decision %s has unverified Guardian source", ev.ID())
	}
	if name != state.role {
		return fmt.Errorf("decision %s name %q does not equal frozen role %q", ev.ID(), name, state.role)
	}
	if !exactSingleCause(ev, state.proposal.ID()) {
		return fmt.Errorf("decision %s is not caused directly and only by proposal %s", ev.ID(), state.proposal.ID())
	}
	return nil
}

func (l *Loop) organicDecisionAnchor(name string) (event.Event, error) {
	state, err := l.readOrganicGovernanceState()
	if err != nil {
		return event.Event{}, err
	}
	if state.proposal.ID().IsZero() || state.proposalCount != 1 {
		return event.Event{}, fmt.Errorf("no unique Spawner proposal")
	}
	if state.decisionCount != 0 || !state.budget.ID().IsZero() {
		return event.Event{}, fmt.Errorf("proposal %s already has a decision or budget", state.proposal.ID())
	}
	if name != state.role {
		return event.Event{}, fmt.Errorf("decision name %q does not equal frozen role %q", name, state.role)
	}
	return state.proposal, nil
}

type organicProposalContext struct {
	GapEventID      string   `json:"gap_event_id"`
	RoleName        string   `json:"role_name"`
	RequiredName    string   `json:"required_proposal_name"`
	ProposalPresent bool     `json:"proposal_present"`
	SourceVerified  bool     `json:"source_verified"`
	WatchPatterns   []string `json:"watch_patterns,omitempty"`
}

type organicDecisionContext struct {
	ProposalEventID string   `json:"proposal_event_id"`
	Name            string   `json:"name"`
	Model           string   `json:"model"`
	CanOperate      bool     `json:"can_operate"`
	MaxIterations   int      `json:"max_iterations"`
	Prompt          string   `json:"prompt"`
	WatchPatterns   []string `json:"watch_patterns"`
	Reason          string   `json:"reason"`
	SourceVerified  bool     `json:"source_verified"`
	DecisionState   string   `json:"decision_state"`
}

type organicBudgetContext struct {
	ProposalEventID string `json:"proposal_event_id"`
	ApprovalEventID string `json:"approval_event_id"`
	RoleName        string `json:"role_name"`
	IterationBudget int    `json:"proposed_iteration_budget"`
	SourceVerified  bool   `json:"source_verified"`
	BudgetState     string `json:"budget_state"`
}

func appendOrganicContext(obs string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode organic governance context: %w", err)
	}
	return obs + "\n=== ORGANIC GOVERNANCE CONTEXT ===\n" + string(payload) + "\n===\n", nil
}

func (l *Loop) enrichOrganicGovernanceObservation(obs string) (string, error) {
	if !l.config.EnforceOrganicGovernanceCausality {
		return obs, nil
	}
	state, err := l.readOrganicGovernanceState()
	if err != nil {
		return "", err
	}
	switch string(l.agent.Role()) {
	case "spawner":
		if state.gap.ID().IsZero() {
			return obs, nil
		}
		return appendOrganicContext(obs, organicProposalContext{
			GapEventID: state.gap.ID().Value(), RoleName: state.role,
			RequiredName: state.role, ProposalPresent: !state.proposal.ID().IsZero(),
			SourceVerified: true,
		})
	case "guardian":
		if state.proposal.ID().IsZero() {
			return obs, nil
		}
		decisionState := "pending"
		if !state.approval.ID().IsZero() {
			decisionState = "approved"
		} else if !state.rejection.ID().IsZero() {
			decisionState = "rejected"
		}
		return appendOrganicContext(obs, organicDecisionContext{
			ProposalEventID: state.proposal.ID().Value(), Name: state.proposalDef.Name,
			Model: state.proposalDef.Model, CanOperate: state.proposalDef.CanOperate,
			MaxIterations: state.proposalDef.MaxIterations, Prompt: state.proposalDef.Prompt,
			WatchPatterns: state.proposalDef.WatchPatterns, Reason: state.proposalDef.Reason,
			SourceVerified: true, DecisionState: decisionState,
		})
	case "allocator":
		if state.proposal.ID().IsZero() || state.approval.ID().IsZero() {
			return obs, nil
		}
		budgetState := "absent"
		if !state.budget.ID().IsZero() {
			budgetState = "granted"
		}
		return appendOrganicContext(obs, organicBudgetContext{
			ProposalEventID: state.proposal.ID().Value(), ApprovalEventID: state.approval.ID().Value(),
			RoleName: state.role, IterationBudget: state.proposalDef.MaxIterations,
			SourceVerified: true, BudgetState: budgetState,
		})
	default:
		return obs, nil
	}
}
