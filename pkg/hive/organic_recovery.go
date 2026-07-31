package hive

import (
	"bytes"
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

// loadOrganicRecoveryCandidates validates, without writing, every spawned role
// from the latest prior run that explicitly recorded organic-v1. Legacy or
// implicit-profile conversations are never inferred into recovery.
func (r *Runtime) loadOrganicRecoveryCandidates() ([]organicGrowthCandidate, error) {
	starts, err := eventsByTypePaginated(r.store, EventTypeRunStarted, defaultOperatorProjectionLimit)
	if err != nil {
		return nil, err
	}
	var priorConversation types.ConversationID
	for _, ev := range starts { // ByType pagination is reverse chronological.
		content, ok := ev.Content().(RunStartedContent)
		if !ok || content.BootstrapProfile != BootstrapProfileOrganicV1 {
			continue
		}
		priorConversation = ev.ConversationID()
		break
	}
	if priorConversation.Value() == "" {
		return nil, nil
	}

	events, err := eventsByConversationChronological(r.store, priorConversation)
	if err != nil {
		return nil, err
	}

	definitions := make([]event.Event, 0)
	seenDefinitions := make(map[types.EventID]struct{})
	for _, ev := range events {
		content, ok := ev.Content().(RoleDefinitionContent)
		if !ok || content.Origin != "spawned" {
			continue
		}
		if content.CanOperate {
			return nil, fmt.Errorf("organic recovery definition %s has CanOperate=true", ev.ID())
		}
		seenDefinitions[ev.ID()] = struct{}{}
		definitions = append(definitions, ev)
	}
	// A recovered start deliberately does not duplicate the historical role
	// definition into its new conversation. Follow its exact definition cause
	// so consecutive daemon restarts recover the same durable creation record.
	recoveredSpawnsByDefinition := make(map[types.EventID]types.EventID)
	recoveredSpawnsByRole := make(map[string]types.EventID)
	for _, ev := range events {
		spawned, ok := ev.Content().(AgentSpawnedContent)
		if !ok || !spawned.Recovered {
			continue
		}
		var causedDefinition event.Event
		for _, causeID := range ev.Causes() {
			cause, getErr := r.store.Get(causeID)
			if getErr != nil {
				return nil, fmt.Errorf("organic recovery spawn %s cause %s: %w", ev.ID(), causeID, getErr)
			}
			content, ok := cause.Content().(RoleDefinitionContent)
			if !ok || content.Origin != "spawned" {
				continue
			}
			if !causedDefinition.ID().IsZero() {
				return nil, fmt.Errorf("organic recovery spawn %s has multiple spawned-definition causes", ev.ID())
			}
			causedDefinition = cause
		}
		if causedDefinition.ID().IsZero() {
			return nil, fmt.Errorf("organic recovery spawn %s has no spawned-definition cause", ev.ID())
		}
		content := causedDefinition.Content().(RoleDefinitionContent)
		if normalizeOrganicRole(content.Name) != normalizeOrganicRole(spawned.Role) ||
			normalizeOrganicRole(spawned.Name) != normalizeOrganicRole(spawned.Role) {
			return nil, fmt.Errorf("organic recovery spawn %s does not match definition role", ev.ID())
		}
		role := normalizeOrganicRole(spawned.Role)
		if causedDefinition.ConversationID() == priorConversation {
			return nil, fmt.Errorf(
				"organic recovery spawn %s references same-run definition %s",
				ev.ID(),
				causedDefinition.ID(),
			)
		}
		if prior, duplicate := recoveredSpawnsByDefinition[causedDefinition.ID()]; duplicate {
			return nil, fmt.Errorf(
				"organic recovery definition %s has duplicate recovered spawns %s and %s",
				causedDefinition.ID(),
				prior,
				ev.ID(),
			)
		}
		if prior, duplicate := recoveredSpawnsByRole[role]; duplicate {
			return nil, fmt.Errorf(
				"organic recovery role %q has duplicate recovered spawns %s and %s",
				role,
				prior,
				ev.ID(),
			)
		}
		recoveredSpawnsByDefinition[causedDefinition.ID()] = ev.ID()
		recoveredSpawnsByRole[role] = ev.ID()
		if _, duplicate := seenDefinitions[causedDefinition.ID()]; duplicate {
			return nil, fmt.Errorf(
				"organic recovery spawn %s duplicates definition %s already materialized in the latest run",
				ev.ID(),
				causedDefinition.ID(),
			)
		}
		seenDefinitions[causedDefinition.ID()] = struct{}{}
		definitions = append(definitions, causedDefinition)
	}
	if len(definitions) > OrganicV1MaximumDynamicActors {
		return nil, fmt.Errorf("organic recovery found %d spawned definitions, maximum is %d", len(definitions), OrganicV1MaximumDynamicActors)
	}

	seenRoles := make(map[string]struct{}, len(definitions))
	candidates := make([]organicGrowthCandidate, 0, len(definitions))
	for _, definition := range definitions {
		defContent := definition.Content().(RoleDefinitionContent)
		role := normalizeOrganicRole(defContent.Name)
		if role == "" {
			return nil, fmt.Errorf("organic recovery definition %s has empty normalized role", definition.ID())
		}
		if _, duplicate := seenRoles[role]; duplicate {
			return nil, fmt.Errorf("organic recovery has duplicate role %q", role)
		}
		seenRoles[role] = struct{}{}
		if r.bootstrapRoleExists(role) {
			return nil, fmt.Errorf("organic recovery role %q collides with bootstrap", role)
		}

		creationEvents, err := eventsByConversationChronological(r.store, definition.ConversationID())
		if err != nil {
			return nil, fmt.Errorf("organic recovery role %q creation conversation: %w", role, err)
		}
		index := make(map[types.EventID]int, len(creationEvents))
		for i, ev := range creationEvents {
			index[ev.ID()] = i
		}
		bindings, err := r.recoveryBootstrapActorBindings(creationEvents)
		if err != nil {
			return nil, fmt.Errorf("organic recovery role %q bootstrap identity: %w", role, err)
		}
		candidate, err := r.reconstructOrganicCandidate(creationEvents, index, definition, role, bindings)
		if err != nil {
			return nil, err
		}
		if candidate.ProposalContent.CanOperate || defContent.CanOperate {
			return nil, fmt.Errorf("organic recovery role %q crosses CanOperate ceiling", role)
		}
		if _, err := mapModelName(candidate.ProposalContent.Model, r.currentResolver().Catalog()); err != nil {
			return nil, fmt.Errorf("organic recovery role %q: %w", role, err)
		}
		if err := r.validateOrganicRecoveryIdentity(creationEvents, index, candidate); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (r *Runtime) recoveryBootstrapActorBindings(events []event.Event) (map[string]types.ActorID, error) {
	required := map[string]struct{}{
		"guardian":  {},
		"allocator": {},
		"cto":       {},
		"spawner":   {},
	}
	bindings := make(map[string]types.ActorID, len(required))
	for _, ev := range events {
		content, ok := ev.Content().(AgentIdentityRegisteredContent)
		if !ok {
			continue
		}
		role := normalizeOrganicRole(content.Role)
		if _, needed := required[role]; !needed {
			continue
		}
		if normalizeOrganicRole(content.DisplayName) != role || content.ActorID.IsZero() {
			return nil, fmt.Errorf("role %q has malformed identity evidence %s", role, ev.ID())
		}
		if prior, duplicate := bindings[role]; duplicate {
			return nil, fmt.Errorf("role %q has duplicate identity evidence (%s and %s)", role, prior, content.ActorID)
		}
		registered, err := r.actors.Get(content.ActorID)
		if err != nil {
			return nil, fmt.Errorf("role %q actor %s is not registered: %w", role, content.ActorID, err)
		}
		if registered.ID() != content.ActorID ||
			normalizeOrganicRole(registered.DisplayName()) != role ||
			!bytes.Equal(registered.PublicKey().Bytes(), content.PublicKey.Bytes()) {
			return nil, fmt.Errorf("role %q identity evidence does not match current actor store", role)
		}
		bindings[role] = content.ActorID
	}
	for role := range required {
		if _, ok := bindings[role]; !ok {
			return nil, fmt.Errorf("missing immutable bootstrap ActorID binding for %q", role)
		}
	}
	return bindings, nil
}

func (r *Runtime) reconstructOrganicCandidate(events []event.Event, index map[types.EventID]int, definition event.Event, role string, bindings map[string]types.ActorID) (organicGrowthCandidate, error) {
	candidate := organicGrowthCandidate{
		Definition:     definition,
		NormalizedRole: role,
	}
	definitionIndex, ok := index[definition.ID()]
	if !ok {
		return candidate, fmt.Errorf("organic recovery role %q definition %s is missing from its creation conversation", role, definition.ID())
	}

	var sameRoleProposals []event.Event
	for _, ev := range events[:definitionIndex] {
		content, ok := ev.Content().(event.RoleProposedContent)
		if !ok || normalizeOrganicRole(content.Name) != role {
			continue
		}
		sameRoleProposals = append(sameRoleProposals, ev)
	}
	if len(sameRoleProposals) == 0 {
		return candidate, fmt.Errorf("organic recovery role %q has no proposal", role)
	}
	if len(sameRoleProposals) > 2 {
		return candidate, fmt.Errorf("organic recovery role %q exceeded the one-refinement proposal limit", role)
	}
	candidate.Proposal = sameRoleProposals[len(sameRoleProposals)-1]
	candidate.ProposalContent = candidate.Proposal.Content().(event.RoleProposedContent)
	for i := 0; i < len(sameRoleProposals)-1; i++ {
		priorIndex := index[sameRoleProposals[i].ID()]
		nextIndex := index[sameRoleProposals[i+1].ID()]
		rejected := false
		for _, ev := range events[priorIndex+1 : nextIndex] {
			content, ok := ev.Content().(event.RoleRejectedContent)
			if ok &&
				normalizeOrganicRole(content.Name) == role &&
				normalizeOrganicRole(content.RejectedBy) == "guardian" &&
				sourceHasBoundRole(ev, "guardian", bindings) {
				rejected = true
				break
			}
		}
		if !rejected {
			return candidate, fmt.Errorf("organic recovery role %q has multiple pending or admitted proposals", role)
		}
	}
	if !eventCauses(definition, candidate.Proposal.ID()) ||
		!sourceHasBoundRole(candidate.Proposal, "spawner", bindings) ||
		normalizeOrganicRole(candidate.ProposalContent.ProposedBy) != "spawner" {
		return candidate, fmt.Errorf("organic recovery role %q has invalid Spawner proposal linkage", role)
	}
	proposalIndex := index[candidate.Proposal.ID()]

	for _, ev := range events[:proposalIndex] {
		content, ok := ev.Content().(event.GapDetectedContent)
		if !ok || normalizeOrganicRole(content.MissingRole) != role || !sourceHasBoundRole(ev, "cto", bindings) {
			continue
		}
		candidate.Gap = ev
		candidate.GapContent = content
		break
	}
	if candidate.Gap.ID().IsZero() {
		return candidate, fmt.Errorf("organic recovery role %q has no genuine preceding CTO gap", role)
	}

	for _, ev := range events[proposalIndex+1 : definitionIndex] {
		if content, ok := ev.Content().(event.RoleRejectedContent); ok &&
			normalizeOrganicRole(content.Name) == role &&
			sourceHasBoundRole(ev, "guardian", bindings) &&
			normalizeOrganicRole(content.RejectedBy) == "guardian" {
			return candidate, fmt.Errorf("organic recovery role %q proposal was rejected by Guardian", role)
		}
		content, ok := ev.Content().(event.RoleApprovedContent)
		if !ok ||
			normalizeOrganicRole(content.Name) != role ||
			!sourceHasBoundRole(ev, "guardian", bindings) ||
			normalizeOrganicRole(content.ApprovedBy) != "guardian" {
			continue
		}
		candidate.Approval = ev
		candidate.ApprovalContent = content
		break
	}
	if candidate.Approval.ID().IsZero() {
		return candidate, fmt.Errorf("organic recovery role %q has no Guardian approval", role)
	}
	approvalIndex := index[candidate.Approval.ID()]

	for _, ev := range events[approvalIndex+1 : definitionIndex] {
		content, ok := ev.Content().(event.AgentBudgetAdjustedContent)
		if !ok ||
			!content.AdjustsIterations() ||
			normalizeOrganicRole(content.AgentName) != role ||
			!sourceHasBoundRole(ev, "allocator", bindings) {
			continue
		}
		candidate.Budget = ev
		candidate.BudgetContent = content
		break
	}
	if candidate.Budget.ID().IsZero() {
		return candidate, fmt.Errorf("organic recovery role %q has no Allocator iteration budget", role)
	}
	if candidate.Gap.Source() == candidate.Proposal.Source() ||
		candidate.Gap.Source() == candidate.Approval.Source() ||
		candidate.Gap.Source() == candidate.Budget.Source() ||
		candidate.Proposal.Source() == candidate.Approval.Source() ||
		candidate.Proposal.Source() == candidate.Budget.Source() ||
		candidate.Approval.Source() == candidate.Budget.Source() {
		return candidate, fmt.Errorf("organic recovery role %q has non-distinct prerequisite actors", role)
	}

	requestID, decision, requestFound, decisionFound := r.findOrganicAuthority(events, candidate)
	if !requestFound || !decisionFound ||
		!eventCauses(definition, decision.ID()) {
		return candidate, fmt.Errorf("organic recovery role %q has invalid %s authority linkage", role, safety.ActionAgentSpawnPersistent)
	}
	candidate.AuthorityRequest = requestID
	candidate.AuthorityDecision = decision

	spawnCount := 0
	for _, ev := range events[definitionIndex+1:] {
		content, ok := ev.Content().(AgentSpawnedContent)
		if !ok ||
			normalizeOrganicRole(content.Name) != role ||
			normalizeOrganicRole(content.Role) != role ||
			content.Recovered ||
			!eventCauses(ev, definition.ID()) {
			continue
		}
		spawnCount++
		candidate.Spawned = ev
	}
	if spawnCount > 1 {
		return candidate, fmt.Errorf("organic recovery role %q has %d fresh spawn events", role, spawnCount)
	}
	return candidate, nil
}

func (r *Runtime) validateOrganicRecoveryIdentity(events []event.Event, index map[types.EventID]int, candidate organicGrowthCandidate) error {
	definitionIndex, ok := index[candidate.Definition.ID()]
	if !ok {
		return fmt.Errorf("organic recovery role %q definition is outside its creation conversation", candidate.NormalizedRole)
	}
	limit := len(events)
	if !candidate.Spawned.ID().IsZero() {
		limit = index[candidate.Spawned.ID()] + 1
	}
	var identities []AgentIdentityRegisteredContent
	for _, ev := range events[definitionIndex+1 : limit] {
		content, ok := ev.Content().(AgentIdentityRegisteredContent)
		if !ok || normalizeOrganicRole(content.Role) != candidate.NormalizedRole {
			continue
		}
		identities = append(identities, content)
	}
	if len(identities) > 1 {
		return fmt.Errorf("organic recovery role %q has %d identity registrations", candidate.NormalizedRole, len(identities))
	}
	if candidate.Spawned.ID().IsZero() {
		if len(identities) == 0 && r.actorDisplayNameExists(candidate.NormalizedRole) {
			return fmt.Errorf("organic recovery role %q has an uncorrelated actor identity collision", candidate.NormalizedRole)
		}
		return nil
	}
	if len(identities) != 1 {
		return fmt.Errorf("organic recovery role %q has %d identity registrations, want 1", candidate.NormalizedRole, len(identities))
	}
	identity := identities[0]
	spawned := candidate.Spawned.Content().(AgentSpawnedContent)
	if identity.ActorID.Value() != spawned.ActorID ||
		normalizeOrganicRole(identity.DisplayName) != candidate.NormalizedRole ||
		normalizeOrganicRole(identity.Role) != candidate.NormalizedRole {
		return fmt.Errorf("organic recovery role %q has mismatched identity/spawn evidence", candidate.NormalizedRole)
	}
	registered, err := r.actors.Get(identity.ActorID)
	if err != nil {
		return fmt.Errorf("organic recovery role %q actor %s is not registered: %w", candidate.NormalizedRole, identity.ActorID, err)
	}
	if normalizeOrganicRole(registered.DisplayName()) != candidate.NormalizedRole ||
		!bytes.Equal(registered.PublicKey().Bytes(), identity.PublicKey.Bytes()) {
		return fmt.Errorf("organic recovery role %q identity does not match actor store", candidate.NormalizedRole)
	}
	return nil
}
