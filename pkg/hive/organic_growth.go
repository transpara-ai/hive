package hive

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/loop"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/hive/pkg/safety"
	"github.com/transpara-ai/hive/pkg/telemetry"
)

var gapDetectedType = types.MustEventType("hive.gap.detected")

type organicGrowthCandidate struct {
	Gap               event.Event
	GapContent        event.GapDetectedContent
	Proposal          event.Event
	ProposalContent   event.RoleProposedContent
	Approval          event.Event
	ApprovalContent   event.RoleApprovedContent
	Budget            event.Event
	BudgetContent     event.AgentBudgetAdjustedContent
	AuthorityRequest  types.EventID
	AuthorityDecision event.Event
	Definition        event.Event
	Spawned           event.Event
	NormalizedRole    string
}

// OrganicAdmissionError is terminal for the current organic runtime. Once
// persistent-spawn work has begun, retrying inside the same run can duplicate
// durable state or leak an actor. The daemon must stop, join owned loops, and
// recover from the chain on its next start.
type OrganicAdmissionError struct {
	Role             string
	Stage            string
	RecoveryRequired bool
	Err              error
}

func (e *OrganicAdmissionError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("organic-v1 admission for %q failed at %s: %v", e.Role, e.Stage, e.Err)
}

func (e *OrganicAdmissionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (r *Runtime) organicSpawnCheckpoint(stage string) error {
	if r.organicSpawnHook == nil {
		return nil
	}
	return r.organicSpawnHook(stage)
}

func normalizeOrganicRole(value string) string {
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

func eventsByConversationChronological(s interface {
	ByConversation(types.ConversationID, int, types.Option[types.Cursor]) (types.Page[event.Event], error)
}, convID types.ConversationID) ([]event.Event, error) {
	cursor := types.None[types.Cursor]()
	var events []event.Event
	for {
		page, err := s.ByConversation(convID, defaultOperatorProjectionLimit, cursor)
		if err != nil {
			return nil, err
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

func (r *Runtime) bindBootstrapActor(role string, id types.ActorID) {
	role = normalizeOrganicRole(role)
	if role == "" || id.IsZero() {
		return
	}
	r.bootstrapActorsMu.Lock()
	defer r.bootstrapActorsMu.Unlock()
	r.bootstrapActors[role] = id
}

func (r *Runtime) bootstrapActorBindings() map[string]types.ActorID {
	r.bootstrapActorsMu.RLock()
	defer r.bootstrapActorsMu.RUnlock()
	bindings := make(map[string]types.ActorID, len(r.bootstrapActors))
	for role, id := range r.bootstrapActors {
		bindings[role] = id
	}
	return bindings
}

func sourceHasBoundRole(ev event.Event, expected string, bindings map[string]types.ActorID) bool {
	expected = normalizeOrganicRole(expected)
	if expected == "" {
		return false
	}
	id, ok := bindings[expected]
	return ok && !id.IsZero() && ev.Source() == id
}

func (r *Runtime) sourceHasRole(ev event.Event, expected string) bool {
	return sourceHasBoundRole(ev, expected, r.bootstrapActorBindings())
}

func eventIDSliceEqual(left, right []types.EventID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func eventCauses(ev event.Event, id types.EventID) bool {
	for _, cause := range ev.Causes() {
		if cause == id {
			return true
		}
	}
	return false
}

func (r *Runtime) validateOrganicCandidate(proposalID types.EventID, requireAuthority bool) (organicGrowthCandidate, error) {
	var candidate organicGrowthCandidate
	events, err := eventsByConversationChronological(r.store, r.convID)
	if err != nil {
		return candidate, err
	}
	positions := make(map[types.EventID]int, len(events))
	for i, ev := range events {
		positions[ev.ID()] = i
	}

	proposalIndex := -1
	var sameRoleProposals []int
	for i, ev := range events {
		content, ok := ev.Content().(event.RoleProposedContent)
		if !ok {
			continue
		}
		if ev.ID() == proposalID {
			proposalIndex = i
			candidate.Proposal = ev
			candidate.ProposalContent = content
			candidate.NormalizedRole = normalizeOrganicRole(content.Name)
		}
	}
	if proposalIndex < 0 {
		return candidate, fmt.Errorf("proposal %s is not in current run conversation", proposalID)
	}
	if candidate.NormalizedRole == "" {
		return candidate, fmt.Errorf("proposal %s has an empty normalized role", proposalID)
	}
	if candidate.ProposalContent.CanOperate {
		return candidate, fmt.Errorf("proposal %s requested CanOperate=true", proposalID)
	}
	if !r.sourceHasRole(candidate.Proposal, "spawner") ||
		normalizeOrganicRole(candidate.ProposalContent.ProposedBy) != "spawner" {
		return candidate, fmt.Errorf("proposal %s was not emitted by Spawner", proposalID)
	}
	for i, ev := range events {
		content, ok := ev.Content().(event.RoleProposedContent)
		if ok && normalizeOrganicRole(content.Name) == candidate.NormalizedRole {
			sameRoleProposals = append(sameRoleProposals, i)
		}
	}
	if len(sameRoleProposals) == 0 || sameRoleProposals[len(sameRoleProposals)-1] != proposalIndex {
		return candidate, fmt.Errorf("proposal %s is superseded by a later same-role proposal", proposalID)
	}
	if len(sameRoleProposals) > 2 {
		return candidate, fmt.Errorf("role %q exceeded the one-refinement proposal limit", candidate.NormalizedRole)
	}
	for proposalOrdinal, priorIndex := range sameRoleProposals[:len(sameRoleProposals)-1] {
		nextIndex := sameRoleProposals[proposalOrdinal+1]
		rejected := false
		for _, ev := range events[priorIndex+1 : nextIndex] {
			content, ok := ev.Content().(event.RoleRejectedContent)
			if !ok ||
				normalizeOrganicRole(content.Name) != candidate.NormalizedRole ||
				normalizeOrganicRole(content.RejectedBy) != "guardian" ||
				!r.sourceHasRole(ev, "guardian") {
				continue
			}
			rejected = true
			break
		}
		if !rejected {
			return candidate, fmt.Errorf(
				"role %q has an earlier proposal that is still pending or admitted",
				candidate.NormalizedRole,
			)
		}
	}
	if _, err := mapModelName(candidate.ProposalContent.Model, r.currentResolver().Catalog()); err != nil {
		return candidate, err
	}
	if r.bootstrapRoleExists(candidate.NormalizedRole) {
		return candidate, fmt.Errorf("role %q collides with a bootstrap actor", candidate.NormalizedRole)
	}
	if r.persistedRoleExists(candidate.NormalizedRole) {
		return candidate, fmt.Errorf("role %q already has a persisted definition", candidate.NormalizedRole)
	}
	if r.actorDisplayNameExists(candidate.NormalizedRole) {
		return candidate, fmt.Errorf("role %q already has a registered identity", candidate.NormalizedRole)
	}

	for i, ev := range events[:proposalIndex] {
		content, ok := ev.Content().(event.GapDetectedContent)
		if !ok || normalizeOrganicRole(content.MissingRole) != candidate.NormalizedRole {
			continue
		}
		if !r.sourceHasRole(ev, "cto") {
			continue
		}
		candidate.Gap = ev
		candidate.GapContent = content
		_ = i
		break // chronological input freezes the earliest valid duplicate gap
	}
	if candidate.Gap.ID().IsZero() {
		return candidate, fmt.Errorf("proposal %s has no preceding genuine CTO gap", proposalID)
	}

	for _, ev := range events[proposalIndex+1:] {
		if content, ok := ev.Content().(event.RoleRejectedContent); ok &&
			normalizeOrganicRole(content.Name) == candidate.NormalizedRole &&
			r.sourceHasRole(ev, "guardian") &&
			normalizeOrganicRole(content.RejectedBy) == "guardian" {
			return candidate, fmt.Errorf("proposal %s was rejected by Guardian", proposalID)
		}
		content, ok := ev.Content().(event.RoleApprovedContent)
		if !ok || normalizeOrganicRole(content.Name) != candidate.NormalizedRole {
			continue
		}
		if !r.sourceHasRole(ev, "guardian") || normalizeOrganicRole(content.ApprovedBy) != "guardian" {
			continue
		}
		candidate.Approval = ev
		candidate.ApprovalContent = content
		break
	}
	if candidate.Approval.ID().IsZero() {
		return candidate, fmt.Errorf("proposal %s has no later Guardian approval", proposalID)
	}
	approvalIndex := positions[candidate.Approval.ID()]

	for _, ev := range events[approvalIndex+1:] {
		content, ok := ev.Content().(event.AgentBudgetAdjustedContent)
		if !ok || !content.AdjustsIterations() ||
			normalizeOrganicRole(content.AgentName) != candidate.NormalizedRole {
			continue
		}
		if !r.sourceHasRole(ev, "allocator") {
			continue
		}
		candidate.Budget = ev
		candidate.BudgetContent = content
		break
	}
	if candidate.Budget.ID().IsZero() {
		return candidate, fmt.Errorf("proposal %s has no later Allocator iteration budget", proposalID)
	}
	if candidate.Gap.Source() == candidate.Proposal.Source() ||
		candidate.Gap.Source() == candidate.Approval.Source() ||
		candidate.Gap.Source() == candidate.Budget.Source() ||
		candidate.Proposal.Source() == candidate.Approval.Source() ||
		candidate.Proposal.Source() == candidate.Budget.Source() ||
		candidate.Approval.Source() == candidate.Budget.Source() {
		return candidate, fmt.Errorf("proposal %s prerequisite roles do not have distinct actors", proposalID)
	}

	requestID, decision, requestFound, decisionFound := r.findOrganicAuthority(events, candidate)
	candidate.AuthorityRequest = requestID
	candidate.AuthorityDecision = decision
	if requireAuthority {
		if !requestFound {
			return candidate, fmt.Errorf("proposal %s has no exact persistent-spawn authority request", proposalID)
		}
		if !decisionFound {
			return candidate, fmt.Errorf("proposal %s has no approved persistent-spawn authority decision", proposalID)
		}
	}
	return candidate, nil
}

func (r *Runtime) findOrganicAuthority(events []event.Event, candidate organicGrowthCandidate) (types.EventID, event.Event, bool, bool) {
	causalIDs := []types.EventID{
		candidate.Gap.ID(),
		candidate.Proposal.ID(),
		candidate.Approval.ID(),
		candidate.Budget.ID(),
	}
	target := "agent:" + candidate.NormalizedRole
	var requestID types.EventID
	requestPosition := -1
	for i, ev := range events {
		content, ok := ev.Content().(AuthorityRequestRecordedContent)
		if !ok ||
			content.ActionName != string(safety.ActionAgentSpawnPersistent) ||
			content.Target != target ||
			!eventIDSliceEqual(content.CausalEventIDs, causalIDs) ||
			ev.Source() != r.humanID {
			continue
		}
		requestID = content.RequestID
		requestPosition = i
		break
	}
	if requestPosition < 0 {
		return types.EventID{}, event.Event{}, false, false
	}
	for _, ev := range events[requestPosition+1:] {
		content, ok := ev.Content().(AuthorityDecisionRecordedContent)
		if !ok ||
			content.RequestID != requestID ||
			content.Outcome != "approved" ||
			content.ApprovedAction != string(safety.ActionAgentSpawnPersistent) ||
			content.ApprovedTarget != target ||
			content.ApproverActor != r.humanID ||
			ev.Source() != r.humanID ||
			!eventCauses(ev, requestID) {
			continue
		}
		return requestID, ev, true, true
	}
	return requestID, event.Event{}, true, false
}

func (r *Runtime) bootstrapRoleExists(role string) bool {
	for _, def := range r.defs {
		if normalizeOrganicRole(def.Role) == role || normalizeOrganicRole(def.Name) == role {
			return true
		}
	}
	return false
}

func (r *Runtime) persistedRoleExists(role string) bool {
	events, err := eventsByTypePaginated(r.store, EventTypeRoleDefinition, defaultOperatorProjectionLimit)
	if err != nil {
		return true
	}
	for _, ev := range events {
		content, ok := ev.Content().(RoleDefinitionContent)
		if ok && normalizeOrganicRole(content.Name) == role {
			return true
		}
	}
	return false
}

func (r *Runtime) actorDisplayNameExists(role string) bool {
	cursor := types.None[types.Cursor]()
	for {
		page, err := r.actors.List(actor.ActorFilter{Limit: defaultOperatorProjectionLimit, After: cursor})
		if err != nil {
			return true
		}
		for _, registered := range page.Items() {
			if normalizeOrganicRole(registered.DisplayName()) == role {
				return true
			}
		}
		if !page.HasMore() {
			return false
		}
		cursor = page.Cursor()
	}
}

func (r *Runtime) processOrganicApprovedRoles(ctx context.Context) error {
	events, err := eventsByConversationChronological(r.store, r.convID)
	if err != nil {
		return fmt.Errorf("organic-v1 read current conversation: %w", err)
	}
	for _, ev := range events {
		if err := ctx.Err(); err != nil {
			return nil
		}
		proposal, ok := ev.Content().(event.RoleProposedContent)
		if !ok {
			continue
		}
		role := normalizeOrganicRole(proposal.Name)
		if role == "" || r.dynamic.IsTracked(role) {
			continue
		}

		candidate, err := r.validateOrganicCandidate(ev.ID(), false)
		if err != nil {
			continue
		}
		if candidate.AuthorityRequest.IsZero() {
			_, authErr := r.authorizeProtectedAction(protectedActionRequest{
				Action:            safety.ActionAgentSpawnPersistent,
				RequestingActor:   candidate.Approval.Source(),
				RequestingRole:    "runtime",
				Target:            "agent:" + candidate.NormalizedRole,
				Environment:       string(AgentIdentityEnvironmentProduction),
				RequestedOutcome:  "create bounded non-operating persistent agent",
				Justification:     fmt.Sprintf("organic-v1 tuple admits role %q", candidate.NormalizedRole),
				RiskSummary:       "persistent non-operating actor joins the bounded organic runtime",
				Scope:             []string{string(safety.ActionAgentSpawnPersistent), OrganicV1GrowthPolicyVersion},
				EvidenceReviewed:  []types.EventID{candidate.Gap.ID(), candidate.Proposal.ID(), candidate.Approval.ID(), candidate.Budget.ID()},
				ProposedOperation: "spawnOrganicDynamicAgent",
				CausalEventIDs:    []types.EventID{candidate.Gap.ID(), candidate.Proposal.ID(), candidate.Approval.ID(), candidate.Budget.ID()},
			})
			if authErr != nil {
				continue
			}
		}

		candidate, err = r.validateOrganicCandidate(ev.ID(), true)
		if err != nil {
			continue
		}
		switch r.dynamic.Reserve(candidate.NormalizedRole) {
		case dynamicSlotDuplicate:
			continue
		case dynamicSlotLimitReached:
			if err := r.emitOrganicGrowthLimit(candidate); err != nil {
				fmt.Fprintf(os.Stderr, "[organic-v1] %v; retrying on the next bounded poll\n", err)
			}
			continue
		}
		if err := r.spawnOrganicDynamicAgent(ctx, candidate, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) emitOrganicGrowthLimit(candidate organicGrowthCandidate) error {
	key := r.convID.Value() + "|" + candidate.Proposal.ID().Value() + "|" + r.growthPolicyVersion
	persisted, err := r.hasOrganicGrowthLimit(candidate.Proposal.ID())
	if err != nil {
		return err
	}
	if persisted {
		r.dynamic.MarkLimitEventCommitted(key)
		return nil
	}
	if !r.dynamic.BeginLimitEvent(key) {
		return nil
	}
	committed := false
	defer func() {
		r.dynamic.FinishLimitEvent(key, committed)
	}()
	_, err = r.graph.Record(
		EventTypeGrowthLimitReached,
		r.humanID,
		GrowthLimitReachedContent{
			GrowthPolicyVersion:  r.growthPolicyVersion,
			MaximumDynamicActors: r.maximumDynamicActors,
			DynamicActorCount:    r.dynamic.OccupiedCount(),
			NormalizedRole:       candidate.NormalizedRole,
			ProposalID:           candidate.Proposal.ID(),
			ConversationID:       r.convID,
			Outcome:              "rejected",
		},
		[]types.EventID{candidate.Proposal.ID()},
		r.convID,
		r.signer,
	)
	if err != nil {
		return fmt.Errorf("record growth limit for %q: %w", candidate.NormalizedRole, err)
	}
	committed = true
	return nil
}

func (r *Runtime) hasOrganicGrowthLimit(proposalID types.EventID) (bool, error) {
	events, err := eventsByTypePaginated(r.store, EventTypeGrowthLimitReached, defaultOperatorProjectionLimit)
	if err != nil {
		return false, err
	}
	for _, ev := range events {
		content, ok := ev.Content().(GrowthLimitReachedContent)
		if ok &&
			content.ConversationID == r.convID &&
			content.ProposalID == proposalID &&
			content.GrowthPolicyVersion == r.growthPolicyVersion {
			return true, nil
		}
	}
	return false, nil
}

func (r *Runtime) spawnOrganicDynamicAgent(ctx context.Context, reserved organicGrowthCandidate, recovered bool) (err error) {
	candidate, err := r.validateOrganicCandidate(reserved.Proposal.ID(), true)
	if err != nil {
		r.dynamic.Release(reserved.NormalizedRole)
		return err
	}
	return r.startOrganicCandidate(ctx, candidate, recovered, true)
}

func (r *Runtime) startOrganicCandidate(ctx context.Context, candidate organicGrowthCandidate, recovered, emitDefinition bool) (err error) {
	role := candidate.NormalizedRole
	reservationReleasable := !recovered
	defer func() {
		if err != nil && reservationReleasable {
			r.dynamic.Release(role)
		}
	}()

	maxIterations := candidate.ProposalContent.MaxIterations
	if candidate.BudgetContent.NewBudget > 0 {
		maxIterations = candidate.BudgetContent.NewBudget
	}
	modelID, err := mapModelName(candidate.ProposalContent.Model, r.currentResolver().Catalog())
	if err != nil {
		return err
	}
	def := AgentDef{
		Name:          role,
		Role:          role,
		Model:         modelID,
		SystemPrompt:  composeSpawnedPrompt(candidate.ProposalContent.Prompt),
		WatchPatterns: append([]string(nil), candidate.ProposalContent.WatchPatterns...),
		CanOperate:    false,
		MaxIterations: maxIterations,
		Tier:          TierB,
		RoleDefinition: &modelconfig.RoleDefinition{
			Name:        role,
			Description: candidate.ProposalContent.Reason,
			Category:    "spawned",
			Tier:        TierB,
			CanOperate:  false,
		},
	}
	if def.CanOperate || def.RoleDefinition.CanOperate {
		return fmt.Errorf("organic role %q crossed the non-operating ceiling", role)
	}

	definition := candidate.Definition
	if emitDefinition {
		definition, err = r.graph.Record(
			EventTypeRoleDefinition,
			r.humanID,
			RoleDefinitionContent{
				Name:        role,
				Description: def.RoleDefinition.Description,
				Category:    def.RoleDefinition.Category,
				Tier:        TierB,
				CanOperate:  false,
				Origin:      "spawned",
			},
			[]types.EventID{candidate.AuthorityDecision.ID(), candidate.Proposal.ID()},
			r.convID,
			r.signer,
		)
		if err != nil {
			return fmt.Errorf("record spawned role definition: %w", err)
		}
		// The definition is the first persistent spawn side effect. Any later
		// failure is ambiguous and must retain its reservation for recovery.
		reservationReleasable = false
	} else if definition.ID().IsZero() {
		return fmt.Errorf("recovered organic role %q has no historical definition", role)
	}
	if err := r.organicSpawnCheckpoint("definition"); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "definition",
			RecoveryRequired: true,
			Err:              err,
		}
	}

	agent, resolvedModel, identity, err := r.constructAgent(ctx, def)
	if err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "identity",
			RecoveryRequired: true,
			Err:              err,
		}
	}
	if err := r.organicSpawnCheckpoint("identity"); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "identity",
			RecoveryRequired: true,
			Err:              err,
		}
	}
	if err := r.emitAgentIdentityRegistered(agent.ID(), def, identity); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "identity-evidence",
			RecoveryRequired: true,
			Err:              fmt.Errorf("record dynamic identity provenance: %w", err),
		}
	}
	if err := r.organicSpawnCheckpoint("identity-evidence"); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "identity-evidence",
			RecoveryRequired: true,
			Err:              err,
		}
	}

	budgetCfg := resources.BudgetConfig{
		MaxIterations: def.EffectiveMaxIterations(),
		MaxDuration:   def.EffectiveMaxDuration(),
	}
	agentBudget := resources.NewBudget(budgetCfg)
	r.budgetRegistry.Register(def.Name, agentBudget, def.EffectiveMaxIterations(), resolvedModel)
	if err := r.organicSpawnCheckpoint("budget"); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "budget",
			RecoveryRequired: true,
			Err:              err,
		}
	}
	if r.telemetryWriter != nil {
		r.telemetryWriter.RegisterAgent(telemetry.AgentRegistration{
			Name:          def.Name,
			Role:          def.Role,
			Model:         resolvedModel,
			Agent:         agent,
			MaxIterations: def.EffectiveMaxIterations(),
			WatchPatterns: def.WatchPatterns,
			CanOperate:    false,
			Tier:          TierB,
			Origin:        "spawned",
		})
	}
	if err := r.organicSpawnCheckpoint("telemetry"); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "telemetry",
			RecoveryRequired: true,
			Err:              err,
		}
	}

	resolver := r.currentResolver()
	cfg := loop.Config{
		Agent:                             agent,
		HumanID:                           r.humanID,
		Budget:                            budgetCfg,
		BudgetInstance:                    agentBudget,
		BudgetRegistry:                    r.budgetRegistry,
		AllowPreAdmissionRoleBudget:       r.bootstrapProfile == BootstrapProfileOrganicV1,
		EnforceOrganicGovernanceCausality: r.enforceOrganicGovernanceCausality,
		Bus:                               r.graph.Bus(),
		TaskStore:                         r.tasks,
		PhaseGateStore:                    r.phaseGates,
		ConvID:                            r.convID,
		TaskScope:                         r.oneShotTaskScope(),
		TaskWorkspace:                     r.oneShotTaskWorkspace(),
		OnTaskCompleted:                   r.handleTaskCompletion,
		OnTaskCommandsExecuted:            r.progressIssueScanLifecycleAfterTaskCommands,
		OnReviewCompleted:                 r.progressIssueScanLifecycleAfterReview,
		CanOperate:                        false,
		RepoPath:                          r.repoPath,
		Keepalive:                         r.loop,
		KnowledgeStore:                    r.knowledgeStore,
		Catalog:                           resolver.Catalog(),
		ActorResolver: func(id types.ActorID) string {
			a, lookupErr := r.actors.Get(id)
			if lookupErr != nil {
				return ""
			}
			return a.DisplayName()
		},
		OnIteration: func(iteration int, response string) {
			fmt.Fprintf(os.Stderr, "[%s] iteration %d (%d chars)\n", def.Name, iteration, len(response))
			if r.telemetryWriter != nil {
				r.telemetryWriter.RecordResponse(def.Name, response)
			}
		},
	}
	productionLoop, err := loop.New(cfg)
	if err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "loop",
			RecoveryRequired: true,
			Err:              fmt.Errorf("construct dynamic loop: %w", err),
		}
	}
	if err := r.organicSpawnCheckpoint("loop"); err != nil {
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "loop",
			RecoveryRequired: true,
			Err:              err,
		}
	}
	startBarrier := make(chan struct{})
	agentCtx, cancel := context.WithCancel(ctx)
	if !r.dynamic.Attach(role, cancel, recovered) {
		cancel()
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "attach",
			RecoveryRequired: true,
			Err:              fmt.Errorf("dynamic role %q lost its reserved slot", role),
		}
	}
	go func() {
		defer r.dynamic.Done()
		select {
		case <-agentCtx.Done():
			return
		case <-startBarrier:
		}
		productionLoop.Run(agentCtx)
	}()
	if err := r.organicSpawnCheckpoint("attach"); err != nil {
		cancel()
		close(startBarrier)
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "attach",
			RecoveryRequired: true,
			Err:              err,
		}
	}

	spawned, err := r.graph.Record(
		EventTypeAgentSpawned,
		r.humanID,
		AgentSpawnedContent{
			Name:      role,
			Role:      role,
			Model:     resolvedModel,
			ActorID:   agent.ID().Value(),
			Recovered: recovered,
		},
		[]types.EventID{definition.ID()},
		r.convID,
		r.signer,
	)
	if err != nil {
		cancel()
		close(startBarrier)
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "spawn-event",
			RecoveryRequired: true,
			Err:              fmt.Errorf("record dynamic spawn: %w", err),
		}
	}
	if err := r.organicSpawnCheckpoint("spawn-event"); err != nil {
		cancel()
		close(startBarrier)
		return &OrganicAdmissionError{
			Role:             role,
			Stage:            "spawn-event",
			RecoveryRequired: true,
			Err:              err,
		}
	}
	close(startBarrier)
	fmt.Fprintf(os.Stderr, "dynamic organic agent spawned: %s (model=%s, maxIter=%d, event=%s)\n",
		role, resolvedModel, def.EffectiveMaxIterations(), spawned.ID())
	return nil
}
