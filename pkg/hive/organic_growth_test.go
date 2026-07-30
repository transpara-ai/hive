package hive

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/knowledge"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/hive/pkg/safety"
)

type organicTestProvider struct {
	runtime *Runtime
	role    string
	once    sync.Once
	seen    chan bool
}

func (p *organicTestProvider) Name() string  { return "scripted" }
func (p *organicTestProvider) Model() string { return "scripted-organic" }
func (p *organicTestProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	p.once.Do(func() {
		spawnedBeforeReason := false
		page, err := p.runtime.store.ByType(EventTypeAgentSpawned, 100, types.None[types.Cursor]())
		if err == nil {
			for _, ev := range page.Items() {
				content, ok := ev.Content().(AgentSpawnedContent)
				if ok && normalizeOrganicRole(content.Name) == p.role {
					spawnedBeforeReason = true
					break
				}
			}
		}
		p.seen <- spawnedBeforeReason
	})
	score, _ := types.NewScore(0.9)
	return decision.NewResponse(`/signal {"signal":"IDLE"}`, score, decision.TokenUsage{}), nil
}

var _ intelligence.Provider = (*organicTestProvider)(nil)

type organicRuntimeHarnessProvider struct {
	runtime   *Runtime
	role      string
	mu        sync.Mutex
	calls     int
	evaluated chan<- struct{}
	once      sync.Once
}

func (p *organicRuntimeHarnessProvider) Name() string  { return "organic-runtime-harness" }
func (p *organicRuntimeHarnessProvider) Model() string { return "scripted-organic-runtime" }

func (p *organicRuntimeHarnessProvider) Reason(ctx context.Context, _ string, _ []event.Event) (decision.Response, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	var response string
	switch p.role {
	case "cto":
		response = `/gap {"category":"technical","missing_role":"incident-observer","evidence":"the seeded unclassified failure remains without an event-class observer","severity":"serious"}`
	case "spawner":
		if organicHarnessHas(p.runtime, func(content event.EventContent) bool {
			proposal, ok := content.(event.RoleProposedContent)
			return ok && normalizeOrganicRole(proposal.Name) == "incident-observer"
		}) {
			response = `/signal {"signal":"IDLE"}`
			break
		}
		if call >= 20 {
			deadline := time.NewTimer(2 * time.Second)
			ticker := time.NewTicker(time.Millisecond)
			defer deadline.Stop()
			defer ticker.Stop()
			for !organicHarnessHas(p.runtime, func(content event.EventContent) bool {
				gap, ok := content.(event.GapDetectedContent)
				return ok && normalizeOrganicRole(gap.MissingRole) == "incident-observer"
			}) {
				select {
				case <-ctx.Done():
					return organicHarnessResponse(`/signal {"signal":"IDLE"}`), nil
				case <-deadline.C:
					return decision.Response{}, fmt.Errorf("scripted Spawner timed out waiting for genuine CTO gap")
				case <-ticker.C:
				}
			}
		}
		response = `/spawn {"name":"incident-observer","model":"haiku","watch_patterns":["work.task.created"],"can_operate":false,"max_iterations":30,"prompt":"You uphold the soul statement and inspect the cited unhandled event class. Attach deliverables as /task comment with the full document body and remain non-operating.","reason":"fill the observed event-handling gap"}`
	case "guardian":
		if organicHarnessHas(p.runtime, func(content event.EventContent) bool {
			proposal, ok := content.(event.RoleProposedContent)
			return ok && normalizeOrganicRole(proposal.Name) == "incident-observer"
		}) && !organicHarnessHas(p.runtime, func(content event.EventContent) bool {
			approval, ok := content.(event.RoleApprovedContent)
			return ok && normalizeOrganicRole(approval.Name) == "incident-observer"
		}) {
			response = `/approve {"name":"incident-observer","reason":"bounded non-operating role matches the genuine gap and soul"}`
		} else {
			response = `/signal {"signal":"IDLE"}`
		}
	case "allocator":
		if organicHarnessHas(p.runtime, func(content event.EventContent) bool {
			approval, ok := content.(event.RoleApprovedContent)
			return ok && normalizeOrganicRole(approval.Name) == "incident-observer"
		}) && !organicHarnessHas(p.runtime, func(content event.EventContent) bool {
			budget, ok := content.(event.AgentBudgetAdjustedContent)
			return ok && normalizeOrganicRole(budget.AgentName) == "incident-observer"
		}) {
			response = `/budget {"agent":"incident-observer","action":"set","amount":30,"resource":"iterations","reason":"bounded pre-admission iteration grant"}`
		} else {
			response = `/signal {"signal":"IDLE"}`
		}
	case "sysmon":
		response = `/signal {"signal":"IDLE"}`
	default:
		p.once.Do(func() {
			select {
			case p.evaluated <- struct{}{}:
			default:
			}
		})
		response = `/signal {"signal":"IDLE"}`
	}
	time.Sleep(time.Millisecond)
	return organicHarnessResponse(response), nil
}

func organicHarnessResponse(content string) decision.Response {
	score, _ := types.NewScore(0.9)
	return decision.NewResponse(content, score, decision.TokenUsage{})
}

func organicHarnessHas(rt *Runtime, match func(event.EventContent) bool) bool {
	events, err := eventsByConversationChronological(rt.store, rt.convID)
	if err != nil {
		return false
	}
	for _, ev := range events {
		if match(ev.Content()) {
			return true
		}
	}
	return false
}

type organicCapacityHarnessProvider struct {
	runtime     *Runtime
	role        string
	dynamicRole string
	roles       []string
	mu          sync.Mutex
	calls       int
	evaluated   chan<- string
	once        sync.Once
}

func (p *organicCapacityHarnessProvider) Name() string  { return "organic-capacity-harness" }
func (p *organicCapacityHarnessProvider) Model() string { return "scripted-organic-capacity" }

func (p *organicCapacityHarnessProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	response := `/signal {"signal":"IDLE"}`
	switch p.role {
	case "cto":
		role := p.roles[0]
		category := "technical"
		if call >= 15 && call < 15+len(p.roles) {
			index := call - 15
			role = p.roles[index]
			category = []string{"technical", "process", "staffing", "capability"}[index]
		}
		response = fmt.Sprintf(
			`/gap {"category":%q,"missing_role":%q,"evidence":"a distinct seeded unhandled event class remains observable","severity":"serious"}`,
			category,
			role,
		)
	case "spawner":
		if call < 20 {
			response = organicCapacitySpawnResponse(p.roles[0])
			break
		}
		for _, role := range p.roles {
			if organicHarnessRoleHas(p.runtime, role, "spawned") ||
				organicHarnessRoleHas(p.runtime, role, "limited") {
				continue
			}
			if organicHarnessRoleHas(p.runtime, role, "proposal") {
				break
			}
			if organicHarnessRoleHas(p.runtime, role, "gap") {
				response = organicCapacitySpawnResponse(role)
			}
			break
		}
	case "guardian":
		for _, role := range p.roles {
			if organicHarnessRoleHas(p.runtime, role, "proposal") &&
				!organicHarnessRoleHas(p.runtime, role, "approval") &&
				!organicHarnessRoleHas(p.runtime, role, "rejection") {
				response = fmt.Sprintf(
					`/approve {"name":%q,"reason":"bounded non-operating role matches its genuine gap"}`,
					role,
				)
				break
			}
		}
	case "allocator":
		for _, role := range p.roles {
			if organicHarnessRoleHas(p.runtime, role, "approval") &&
				!organicHarnessRoleHas(p.runtime, role, "budget") {
				response = fmt.Sprintf(
					`/budget {"agent":%q,"action":"set","amount":30,"resource":"iterations","reason":"bounded pre-admission iteration grant"}`,
					role,
				)
				break
			}
		}
	case "dynamic":
		p.once.Do(func() {
			select {
			case p.evaluated <- p.dynamicRole:
			default:
			}
		})
	}
	time.Sleep(time.Millisecond)
	return organicHarnessResponse(response), nil
}

func organicCapacitySpawnResponse(role string) string {
	return fmt.Sprintf(
		`/spawn {"name":%q,"model":"haiku","watch_patterns":["work.task.created"],"can_operate":false,"max_iterations":30,"prompt":%q,"reason":"fill the distinct observed event-handling gap"}`,
		role,
		"ROLE_ID:"+role+" You uphold the soul statement, inspect only the cited unhandled event class, attach findings as structured task comments, and remain non-operating.",
	)
}

func organicHarnessRoleHas(rt *Runtime, role, kind string) bool {
	return organicHarnessHas(rt, func(content event.EventContent) bool {
		switch typed := content.(type) {
		case event.GapDetectedContent:
			return kind == "gap" && normalizeOrganicRole(typed.MissingRole) == role
		case event.RoleProposedContent:
			return kind == "proposal" && normalizeOrganicRole(typed.Name) == role
		case event.RoleApprovedContent:
			return kind == "approval" && normalizeOrganicRole(typed.Name) == role
		case event.RoleRejectedContent:
			return kind == "rejection" && normalizeOrganicRole(typed.Name) == role
		case event.AgentBudgetAdjustedContent:
			return kind == "budget" && normalizeOrganicRole(typed.AgentName) == role
		case AgentSpawnedContent:
			return kind == "spawned" && normalizeOrganicRole(typed.Name) == role
		case GrowthLimitReachedContent:
			return kind == "limited" && normalizeOrganicRole(typed.NormalizedRole) == role
		default:
			return false
		}
	})
}

type organicSource struct {
	id     types.ActorID
	signer *ed25519Signer
}

func TestOrganicRuntimeHarnessUsesRealEvaluatorsAndHotAdds(t *testing.T) {
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "OrganicOperator")
	rt, err := New(t.Context(), Config{
		Store:                        store.NewInMemoryStore(),
		Actors:                       actors,
		HumanID:                      humanID,
		BootstrapProfile:             BootstrapProfileOrganicV1,
		GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
		Loop:                         true,
	})
	if err != nil {
		t.Fatalf("New organic runtime: %v", err)
	}
	bootstrap, err := event.NewBootstrapFactory(event.DefaultRegistry()).Init(humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	defs, err := StarterAgentsForProfile("OrganicOperator", BootstrapProfileOrganicV1)
	if err != nil {
		t.Fatalf("starter agents: %v", err)
	}
	for _, def := range defs {
		if err := rt.Register(def); err != nil {
			t.Fatalf("register %s: %v", def.Role, err)
		}
	}
	rt.approvedRolePollInterval = time.Millisecond
	evaluated := make(chan struct{}, 1)
	providers := make(map[string]*organicRuntimeHarnessProvider)
	var providersMu sync.Mutex
	rt.providerFactory = func(cfg intelligence.Config) (intelligence.Provider, error) {
		role := "dynamic"
		for _, candidate := range []string{"guardian", "sysmon", "allocator", "cto", "spawner"} {
			if strings.Contains(cfg.SystemPrompt, "== ROLE: "+strings.ToUpper(candidate)+" ==") {
				role = candidate
				break
			}
		}
		provider := &organicRuntimeHarnessProvider{
			runtime:   rt,
			role:      role,
			evaluated: evaluated,
		}
		providersMu.Lock()
		providers[role] = provider
		providersMu.Unlock()
		return provider, nil
	}

	runCtx, cancel := context.WithCancel(t.Context())
	runDone := make(chan error, 1)
	go func() {
		runDone <- rt.Run(runCtx, "An unclassified work failure persists after normal handling; diagnose the missing event-handling capability.")
	}()

	select {
	case <-evaluated:
		cancel()
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("real runtime did not evaluate a dynamically admitted actor")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Runtime.Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime.Run did not cancel and join every loop")
	}
	t.Cleanup(func() { _ = rt.graph.Close() })

	providersMu.Lock()
	ctoCalls := providers["cto"].calls
	spawnerCalls := providers["spawner"].calls
	providersMu.Unlock()
	if ctoCalls < 15 {
		t.Fatalf("CTO evaluator calls = %d, want at least 15", ctoCalls)
	}
	if spawnerCalls < 20 {
		t.Fatalf("Spawner evaluator calls = %d, want at least 20", spawnerCalls)
	}

	events, err := eventsByConversationChronological(rt.store, rt.convID)
	if err != nil {
		t.Fatalf("conversation events: %v", err)
	}
	position := make(map[types.EventID]int, len(events))
	var gap, proposal, approval, budgetEvent, definition, spawned, evaluatedEvent event.Event
	for i, ev := range events {
		position[ev.ID()] = i
		switch content := ev.Content().(type) {
		case event.GapDetectedContent:
			if normalizeOrganicRole(content.MissingRole) == "incident-observer" && gap.ID().IsZero() {
				gap = ev
			}
		case event.RoleProposedContent:
			if normalizeOrganicRole(content.Name) == "incident-observer" {
				proposal = ev
			}
		case event.RoleApprovedContent:
			if normalizeOrganicRole(content.Name) == "incident-observer" {
				approval = ev
			}
		case event.AgentBudgetAdjustedContent:
			if normalizeOrganicRole(content.AgentName) == "incident-observer" {
				budgetEvent = ev
			}
		case RoleDefinitionContent:
			if normalizeOrganicRole(content.Name) == "incident-observer" {
				definition = ev
			}
		case AgentSpawnedContent:
			if normalizeOrganicRole(content.Name) == "incident-observer" {
				spawned = ev
			}
		case event.AgentEvaluatedContent:
			if !spawned.ID().IsZero() && content.AgentID.Value() == spawned.Content().(AgentSpawnedContent).ActorID {
				evaluatedEvent = ev
			}
		}
	}
	for label, ev := range map[string]event.Event{
		"gap": gap, "proposal": proposal, "approval": approval, "budget": budgetEvent,
		"definition": definition, "spawned": spawned, "evaluated": evaluatedEvent,
	} {
		if ev.ID().IsZero() {
			t.Fatalf("real evaluator/runtime harness missing %s event", label)
		}
	}
	if !(position[gap.ID()] < position[proposal.ID()] &&
		position[proposal.ID()] < position[approval.ID()] &&
		position[approval.ID()] < position[budgetEvent.ID()] &&
		position[budgetEvent.ID()] < position[definition.ID()] &&
		position[definition.ID()] < position[spawned.ID()] &&
		position[spawned.ID()] < position[evaluatedEvent.ID()]) {
		t.Fatal("real evaluator/runtime tuple or hot-add order is not strict")
	}
	bindings := rt.bootstrapActorBindings()
	if gap.Source() != bindings["cto"] ||
		proposal.Source() != bindings["spawner"] ||
		approval.Source() != bindings["guardian"] ||
		budgetEvent.Source() != bindings["allocator"] {
		t.Fatal("real protocol facts were not emitted by the runtime-owned bootstrap ActorIDs")
	}
	requests := authorityRequestsByType[AuthorityRequestRecordedContent](t, rt, EventTypeAuthorityRequestRecorded)
	decisions := authorityRequestsByType[AuthorityDecisionRecordedContent](t, rt, EventTypeAuthorityDecisionRecorded)
	if len(requests) != 1 || len(decisions) != 1 ||
		decisions[0].ApprovedAction != string(safety.ActionAgentSpawnPersistent) {
		t.Fatalf("exact protected-action path requests=%+v decisions=%+v", requests, decisions)
	}
}

func TestOrganicRuntimeHarnessStartsThreeSequentialActorsAndRejectsFourth(t *testing.T) {
	roles := []string{"observer-one", "observer-two", "observer-three", "observer-four"}
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "OrganicOperator")
	rt, err := New(t.Context(), Config{
		Store:                        store.NewInMemoryStore(),
		Actors:                       actors,
		HumanID:                      humanID,
		BootstrapProfile:             BootstrapProfileOrganicV1,
		GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
		Loop:                         true,
	})
	if err != nil {
		t.Fatalf("New organic capacity runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	bootstrap, err := event.NewBootstrapFactory(event.DefaultRegistry()).Init(humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	defs, err := StarterAgentsForProfile("OrganicOperator", BootstrapProfileOrganicV1)
	if err != nil {
		t.Fatalf("starter agents: %v", err)
	}
	for _, def := range defs {
		if err := rt.Register(def); err != nil {
			t.Fatalf("register %s: %v", def.Role, err)
		}
	}
	rt.approvedRolePollInterval = time.Millisecond
	evaluated := make(chan string, len(roles))
	rt.providerFactory = func(cfg intelligence.Config) (intelligence.Provider, error) {
		role := "dynamic"
		dynamicRole := ""
		for _, candidate := range []string{"guardian", "sysmon", "allocator", "cto", "spawner"} {
			if strings.Contains(cfg.SystemPrompt, "== ROLE: "+strings.ToUpper(candidate)+" ==") {
				role = candidate
				break
			}
		}
		if role == "dynamic" {
			for _, candidate := range roles {
				if strings.Contains(cfg.SystemPrompt, "ROLE_ID:"+candidate) {
					dynamicRole = candidate
					break
				}
			}
			if dynamicRole == "" {
				return nil, fmt.Errorf("dynamic harness provider could not identify role")
			}
		}
		return &organicCapacityHarnessProvider{
			runtime:     rt,
			role:        role,
			dynamicRole: dynamicRole,
			roles:       roles,
			evaluated:   evaluated,
		}, nil
	}

	runCtx, cancel := context.WithCancel(t.Context())
	runDone := make(chan error, 1)
	go func() {
		runDone <- rt.Run(runCtx, "Four distinct unclassified failure classes persist; diagnose bounded event-handling capabilities.")
	}()
	seenEvaluations := make(map[string]struct{})
	deadline := time.NewTimer(10 * time.Second)
	poll := time.NewTicker(time.Millisecond)
	defer deadline.Stop()
	defer poll.Stop()
waitForCapacity:
	for {
		select {
		case role := <-evaluated:
			seenEvaluations[role] = struct{}{}
		case <-poll.C:
			if len(seenEvaluations) == OrganicV1MaximumDynamicActors &&
				organicHarnessRoleHas(rt, roles[3], "limited") {
				cancel()
				break waitForCapacity
			}
		case <-deadline.C:
			cancel()
			t.Fatalf("capacity harness stalled: evaluated=%v fourth_limited=%t",
				seenEvaluations, organicHarnessRoleHas(rt, roles[3], "limited"))
		}
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("capacity Runtime.Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("capacity Runtime.Run did not cancel and join")
	}

	for _, role := range roles[:OrganicV1MaximumDynamicActors] {
		if _, ok := seenEvaluations[role]; !ok {
			t.Fatalf("dynamic role %q never evaluated", role)
		}
		if !organicHarnessRoleHas(rt, role, "spawned") {
			t.Fatalf("dynamic role %q has no spawned evidence", role)
		}
	}
	limits := authorityRequestsByType[GrowthLimitReachedContent](t, rt, EventTypeGrowthLimitReached)
	if len(limits) != 1 ||
		normalizeOrganicRole(limits[0].NormalizedRole) != roles[3] ||
		limits[0].DynamicActorCount != OrganicV1MaximumDynamicActors {
		t.Fatalf("fourth-role limit evidence = %+v", limits)
	}
	for _, definition := range authorityRequestsByType[RoleDefinitionContent](t, rt, EventTypeRoleDefinition) {
		if normalizeOrganicRole(definition.Name) == roles[3] {
			t.Fatal("fourth role created a definition")
		}
	}
	for _, spawned := range authorityRequestsByType[AgentSpawnedContent](t, rt, EventTypeAgentSpawned) {
		if normalizeOrganicRole(spawned.Name) == roles[3] {
			t.Fatal("fourth role emitted spawned evidence")
		}
	}
	completed := authorityRequestsByType[RunCompletedContent](t, rt, EventTypeRunCompleted)
	if len(completed) != 1 ||
		completed[0].NewDynamicActorCount != OrganicV1MaximumDynamicActors ||
		completed[0].DynamicActorCount != OrganicV1MaximumDynamicActors {
		t.Fatalf("capacity completion evidence = %+v", completed)
	}
}

func newOrganicTestRuntime(t *testing.T) *Runtime {
	return newOrganicTestRuntimeWithStore(t, store.NewInMemoryStore())
}

func newOrganicTestRuntimeWithStore(t *testing.T, eventStore store.Store) *Runtime {
	t.Helper()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "OrganicOperator")
	rt, err := New(t.Context(), Config{
		Store:                        eventStore,
		Actors:                       actors,
		HumanID:                      humanID,
		BootstrapProfile:             BootstrapProfileOrganicV1,
		GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
	})
	if err != nil {
		t.Fatalf("New organic runtime: %v", err)
	}
	bootstrap, err := event.NewBootstrapFactory(event.DefaultRegistry()).Init(humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	if _, err := rt.graph.Record(
		EventTypeRunStarted,
		humanID,
		RunStartedContent{
			BootstrapProfile:             BootstrapProfileOrganicV1,
			BootstrapRoles:               []string{"guardian", "sysmon", "allocator", "cto", "spawner"},
			GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
			MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
			AutomaticallyApprovedActions: []string{string(safety.ActionAgentSpawnPersistent)},
		},
		[]types.EventID{bootstrap.ID()},
		rt.convID,
		rt.signer,
	); err != nil {
		t.Fatalf("record run start: %v", err)
	}
	defs, err := StarterAgentsForProfile("OrganicOperator", BootstrapProfileOrganicV1)
	if err != nil {
		t.Fatalf("starter agents: %v", err)
	}
	rt.defs = defs
	rt.dynamic = newDynamicAgentTracker(OrganicV1MaximumDynamicActors)
	rt.budgetRegistry = resources.NewBudgetRegistry()
	rt.knowledgeStore = knowledge.NewStore()
	rt.setResolver(modelconfig.DefaultResolver())
	t.Cleanup(func() {
		rt.dynamic.CancelAll()
		rt.dynamic.Wait()
		_ = rt.graph.Close()
	})
	return rt
}

type organicFailOnceStore struct {
	store.Store
	mu        sync.Mutex
	eventType types.EventType
	failed    bool
}

func (s *organicFailOnceStore) Append(ev event.Event) (event.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev.Type() == s.eventType && !s.failed {
		s.failed = true
		return event.Event{}, fmt.Errorf("injected append failure for %s", ev.Type())
	}
	return s.Store.Append(ev)
}

func registerOrganicSource(t *testing.T, rt *Runtime, role string) organicSource {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate %s key: %v", role, err)
	}
	publicKey, err := types.NewPublicKey(privateKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("public key %s: %v", role, err)
	}
	registered, err := rt.actors.Register(publicKey, role, event.ActorTypeAI)
	if err != nil {
		t.Fatalf("register %s: %v", role, err)
	}
	rt.bindBootstrapActor(role, registered.ID())
	head, err := rt.store.Head()
	if err != nil || !head.IsSome() {
		t.Fatalf("head before %s identity evidence: %v", role, err)
	}
	if _, err := rt.graph.Record(
		EventTypeAgentIdentityRegistered,
		rt.humanID,
		AgentIdentityRegisteredContent{
			ActorID:          registered.ID(),
			DisplayName:      role,
			Role:             role,
			PublicKey:        registered.PublicKey(),
			KeyProvenance:    string(KeyProvenanceGenerated),
			Environment:      string(AgentIdentityEnvironmentProduction),
			IdentityMode:     string(AgentIdentityModeGenerated),
			LifecycleStatus:  "active",
			AuthorityScope:   "hive",
			RegistrationPath: "hive.runtime.spawnAgent",
		},
		[]types.EventID{head.Unwrap().ID()},
		rt.convID,
		rt.signer,
	); err != nil {
		t.Fatalf("record %s identity evidence: %v", role, err)
	}
	return organicSource{id: registered.ID(), signer: &ed25519Signer{key: privateKey}}
}

func registerUnboundOrganicSource(t *testing.T, rt *Runtime, displayName string) organicSource {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate unbound %s key: %v", displayName, err)
	}
	publicKey, err := types.NewPublicKey(privateKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("unbound public key %s: %v", displayName, err)
	}
	registered, err := rt.actors.Register(publicKey, displayName, event.ActorTypeAI)
	if err != nil {
		t.Fatalf("register unbound %s: %v", displayName, err)
	}
	return organicSource{id: registered.ID(), signer: &ed25519Signer{key: privateKey}}
}

func recordOrganicEvent(t *testing.T, rt *Runtime, source organicSource, eventType types.EventType, content event.EventContent, causes ...types.EventID) event.Event {
	t.Helper()
	if len(causes) == 0 {
		head, err := rt.store.Head()
		if err != nil || !head.IsSome() {
			t.Fatalf("head before %s: %v", eventType, err)
		}
		causes = []types.EventID{head.Unwrap().ID()}
	}
	created, err := rt.factory.Create(eventType, source.id, content, causes, rt.convID, rt.store, source.signer)
	if err != nil {
		t.Fatalf("create %s: %v", eventType, err)
	}
	stored, err := rt.store.Append(created)
	if err != nil {
		t.Fatalf("append %s: %v", eventType, err)
	}
	return stored
}

func recordOrganicTuple(t *testing.T, rt *Runtime, cto, spawner, guardian, allocator organicSource, role string, duplicateGap bool) (event.Event, event.Event) {
	t.Helper()
	gap := recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
		event.GapCategoryTechnical,
		role,
		"unhandled event class remains after stabilization",
		event.SeverityLevelSerious,
	))
	if duplicateGap {
		recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
			event.GapCategoryTechnical,
			role,
			"duplicate observation of the same unhandled class",
			event.SeverityLevelSerious,
		), gap.ID())
	}
	head, err := rt.store.Head()
	if err != nil || !head.IsSome() {
		t.Fatalf("head before proposal: %v", err)
	}
	proposal := recordOrganicEvent(t, rt, spawner, roleProposedType, event.NewRoleProposedContent(
		role,
		"haiku",
		[]string{"work.task.created"},
		false,
		30,
		"You uphold the soul statement and inspect the cited unhandled event class. Attach findings as structured task comments and remain non-operating.",
		"Fill the observed event-handling gap",
		"spawner",
	), head.Unwrap().ID())
	approval := recordOrganicEvent(t, rt, guardian, roleApprovedType, event.RoleApprovedContent{
		Name:       role,
		ApprovedBy: "guardian",
		Reason:     "bounded non-operating role is consistent with the soul",
	}, proposal.ID())
	recordOrganicEvent(t, rt, allocator, budgetAdjustedType, event.AgentBudgetAdjustedContent{
		AgentID:   allocator.id,
		AgentName: role,
		Action:    "set",
		NewBudget: 30,
		Resource:  "iterations",
		Reason:    "bounded iteration grant",
	}, approval.ID())
	return gap, proposal
}

func TestOrganicRealHotAddOrdersEvidenceAndUsesEarliestGap(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	gap, proposal := recordOrganicTuple(t, rt, cto, spawner, guardian, allocator, "incident-observer", true)

	provider := &organicTestProvider{
		runtime: rt,
		role:    "incident-observer",
		seen:    make(chan bool, 1),
	}
	rt.providerFactory = func(intelligence.Config) (intelligence.Provider, error) {
		return provider, nil
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rt.processOrganicApprovedRoles(ctx)

	select {
	case spawnedBeforeReason := <-provider.seen:
		if !spawnedBeforeReason {
			t.Fatal("dynamic production loop reasoned before hive.agent.spawned was visible")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dynamic production loop did not evaluate")
	}
	cancel()
	rt.dynamic.CancelAll()
	waitDone := make(chan struct{})
	go func() {
		rt.dynamic.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("dynamic production loop did not terminate and join")
	}

	events, err := eventsByConversationChronological(rt.store, rt.convID)
	if err != nil {
		t.Fatalf("conversation events: %v", err)
	}
	position := make(map[types.EventID]int, len(events))
	var definition, spawned, evaluated event.Event
	for i, ev := range events {
		position[ev.ID()] = i
		switch content := ev.Content().(type) {
		case RoleDefinitionContent:
			if normalizeOrganicRole(content.Name) == "incident-observer" {
				if content.CanOperate || content.Origin != "spawned" {
					t.Fatalf("definition crossed non-operating ceiling: %+v", content)
				}
				definition = ev
			}
		case AgentSpawnedContent:
			if normalizeOrganicRole(content.Name) == "incident-observer" {
				if content.Recovered {
					t.Fatal("fresh hot-add recorded Recovered=true")
				}
				spawned = ev
			}
		case event.AgentEvaluatedContent:
			if content.AgentID.Value() == spawned.Content().(AgentSpawnedContent).ActorID {
				evaluated = ev
			}
		}
	}
	if definition.ID().IsZero() || spawned.ID().IsZero() || evaluated.ID().IsZero() {
		t.Fatalf("missing hot-add chain definition=%s spawned=%s evaluated=%s", definition.ID(), spawned.ID(), evaluated.ID())
	}
	if !(position[definition.ID()] < position[spawned.ID()] && position[spawned.ID()] < position[evaluated.ID()]) {
		t.Fatalf("hot-add order definition=%d spawned=%d evaluated=%d", position[definition.ID()], position[spawned.ID()], position[evaluated.ID()])
	}
	if !eventCauses(spawned, definition.ID()) {
		t.Fatal("spawned event does not cause-link its role definition")
	}

	requests := authorityRequestsByType[AuthorityRequestRecordedContent](t, rt, EventTypeAuthorityRequestRecorded)
	if len(requests) != 1 {
		t.Fatalf("authority request count = %d, want 1", len(requests))
	}
	if len(requests[0].CausalEventIDs) != 4 ||
		requests[0].CausalEventIDs[0] != gap.ID() ||
		requests[0].CausalEventIDs[1] != proposal.ID() {
		t.Fatalf("authority tuple did not freeze earliest gap/proposal: %+v", requests[0].CausalEventIDs)
	}
	decisions := authorityRequestsByType[AuthorityDecisionRecordedContent](t, rt, EventTypeAuthorityDecisionRecorded)
	if len(decisions) != 1 ||
		decisions[0].ApprovedAction != string(safety.ActionAgentSpawnPersistent) ||
		decisions[0].Rationale != "auto-approved via exact --approve-action allowlist" {
		t.Fatalf("narrow authority evidence = %+v", decisions)
	}
	for _, registration := range rt.budgetRegistry.Snapshot() {
		if registration.Name == "incident-observer" {
			return
		}
	}
	t.Fatal("dynamic role was not registered in the budget registry")
}

func TestOrganicFourthRoleEmitsOneLimitEventWithoutSpawnSideEffects(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	_, proposal := recordOrganicTuple(t, rt, cto, spawner, guardian, allocator, "capacity-observer", false)

	rt.dynamic.Track("recovered-one", func() {})
	rt.dynamic.Track("recovered-two", func() {})
	rt.dynamic.Track("recovered-three", func() {})
	rt.processOrganicApprovedRoles(t.Context())
	rt.processOrganicApprovedRoles(t.Context())

	limits := authorityRequestsByType[GrowthLimitReachedContent](t, rt, EventTypeGrowthLimitReached)
	if len(limits) != 1 {
		t.Fatalf("growth limit count = %d, want 1", len(limits))
	}
	if limits[0].ProposalID != proposal.ID() ||
		limits[0].DynamicActorCount != OrganicV1MaximumDynamicActors ||
		limits[0].Outcome != "rejected" {
		t.Fatalf("growth limit evidence = %+v", limits[0])
	}
	definitions := authorityRequestsByType[RoleDefinitionContent](t, rt, EventTypeRoleDefinition)
	for _, definition := range definitions {
		if normalizeOrganicRole(definition.Name) == "capacity-observer" {
			t.Fatal("fourth role created a definition")
		}
	}
	spawned := authorityRequestsByType[AgentSpawnedContent](t, rt, EventTypeAgentSpawned)
	for _, content := range spawned {
		if normalizeOrganicRole(content.Name) == "capacity-observer" {
			t.Fatal("fourth role emitted hive.agent.spawned")
		}
	}
}

func TestOrganicGrowthLimitAppendFailureRemainsRetryable(t *testing.T) {
	flaky := &organicFailOnceStore{
		Store:     store.NewInMemoryStore(),
		eventType: EventTypeGrowthLimitReached,
	}
	rt := newOrganicTestRuntimeWithStore(t, flaky)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	recordOrganicTuple(t, rt, cto, spawner, guardian, allocator, "capacity-retry-observer", false)
	for _, role := range []string{"one", "two", "three"} {
		rt.dynamic.Track(role, func() {})
	}

	if err := rt.processOrganicApprovedRoles(t.Context()); err != nil {
		t.Fatalf("first failed growth-limit append terminated admission: %v", err)
	}
	if limits := authorityRequestsByType[GrowthLimitReachedContent](t, rt, EventTypeGrowthLimitReached); len(limits) != 0 {
		t.Fatalf("failed append persisted %d growth-limit events", len(limits))
	}
	if err := rt.processOrganicApprovedRoles(t.Context()); err != nil {
		t.Fatalf("growth-limit retry: %v", err)
	}
	limits := authorityRequestsByType[GrowthLimitReachedContent](t, rt, EventTypeGrowthLimitReached)
	if len(limits) != 1 || limits[0].DynamicActorCount != OrganicV1MaximumDynamicActors {
		t.Fatalf("growth-limit retry evidence = %+v", limits)
	}
	if err := rt.processOrganicApprovedRoles(t.Context()); err != nil {
		t.Fatalf("growth-limit committed replay: %v", err)
	}
	if limits := authorityRequestsByType[GrowthLimitReachedContent](t, rt, EventTypeGrowthLimitReached); len(limits) != 1 {
		t.Fatalf("committed growth-limit replay count = %d, want 1", len(limits))
	}
}

func TestOrganicGrowthLimitConcurrentEvaluationEmitsExactlyOnce(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	_, proposal := recordOrganicTuple(
		t,
		rt,
		cto,
		spawner,
		guardian,
		allocator,
		"concurrent-capacity-observer",
		false,
	)
	candidate, err := rt.validateOrganicCandidate(proposal.ID(), false)
	if err != nil {
		t.Fatalf("validate concurrent limit candidate: %v", err)
	}
	causalIDs := []types.EventID{
		candidate.Gap.ID(),
		candidate.Proposal.ID(),
		candidate.Approval.ID(),
		candidate.Budget.ID(),
	}
	if _, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:           safety.ActionAgentSpawnPersistent,
		RequestingActor:  candidate.Approval.Source(),
		Target:           "agent:" + candidate.NormalizedRole,
		EvidenceReviewed: causalIDs,
		CausalEventIDs:   causalIDs,
	}); err != nil {
		t.Fatalf("authorize concurrent limit candidate: %v", err)
	}
	candidate, err = rt.validateOrganicCandidate(proposal.ID(), true)
	if err != nil {
		t.Fatalf("revalidate authorized concurrent limit candidate: %v", err)
	}
	for _, role := range []string{"occupied-one", "occupied-two", "occupied-three"} {
		if result := rt.dynamic.Reserve(role); result != dynamicSlotReserved {
			t.Fatalf("reserve %s = %v", role, result)
		}
	}

	const evaluators = 24
	var wg sync.WaitGroup
	errs := make(chan error, evaluators)
	for range evaluators {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- rt.emitOrganicGrowthLimit(candidate)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent limit evaluation: %v", err)
		}
	}

	matches := 0
	for _, content := range authorityRequestsByType[GrowthLimitReachedContent](t, rt, EventTypeGrowthLimitReached) {
		if content.ConversationID == rt.convID &&
			content.ProposalID == candidate.Proposal.ID() &&
			content.GrowthPolicyVersion == OrganicV1GrowthPolicyVersion {
			matches++
			if content.DynamicActorCount != OrganicV1MaximumDynamicActors {
				t.Fatalf("dynamic actor count = %d, want %d", content.DynamicActorCount, OrganicV1MaximumDynamicActors)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("matching concurrent growth-limit events = %d, want 1", matches)
	}
}

func TestOrganicRejectedProposalMayBeRefinedOnceWithFreshFacts(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	role := "refined-observer"
	gap := recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
		event.GapCategoryTechnical,
		role,
		"the unhandled class remains observable",
		event.SeverityLevelSerious,
	))
	first := recordOrganicEvent(t, rt, spawner, roleProposedType, event.NewRoleProposedContent(
		role, "haiku", []string{"work.task.created"}, false, 30,
		"You uphold the soul statement and inspect the unhandled class. Attach findings as structured task comments while remaining non-operating.",
		"first proposal", "spawner",
	), gap.ID())
	rejection := recordOrganicEvent(t, rt, guardian, event.EventTypeRoleRejected, event.RoleRejectedContent{
		Name:       role,
		RejectedBy: "guardian",
		Reason:     "watch scope needs refinement",
	}, first.ID())
	recordOrganicEvent(t, rt, allocator, budgetAdjustedType, event.AgentBudgetAdjustedContent{
		AgentID: allocator.id, AgentName: role, Action: "set", NewBudget: 30,
		Resource: "iterations", Reason: "stale grant for rejected proposal",
	}, rejection.ID())
	second := recordOrganicEvent(t, rt, spawner, roleProposedType, event.NewRoleProposedContent(
		role, "haiku", []string{"work.task.created", "work.task.failed"}, false, 30,
		"You uphold the soul statement and inspect only the refined unhandled event class. Attach findings as structured task comments and remain non-operating.",
		"refined after Guardian rejection", "spawner",
	))
	approval := recordOrganicEvent(t, rt, guardian, roleApprovedType, event.RoleApprovedContent{
		Name: role, ApprovedBy: "guardian", Reason: "refinement addresses the rejection",
	}, second.ID())

	if _, err := rt.validateOrganicCandidate(first.ID(), false); err == nil {
		t.Fatal("superseded rejected proposal remained admissible")
	}
	if _, err := rt.validateOrganicCandidate(second.ID(), false); err == nil {
		t.Fatal("budget fact from rejected proposal satisfied the refined candidate")
	}
	freshBudget := recordOrganicEvent(t, rt, allocator, budgetAdjustedType, event.AgentBudgetAdjustedContent{
		AgentID: allocator.id, AgentName: role, Action: "set", NewBudget: 30,
		Resource: "iterations", Reason: "fresh grant for refined proposal",
	}, approval.ID())
	candidate, err := rt.validateOrganicCandidate(second.ID(), false)
	if err != nil {
		t.Fatalf("refined candidate: %v", err)
	}
	if candidate.Proposal.ID() != second.ID() || candidate.Budget.ID() != freshBudget.ID() {
		t.Fatalf("refined tuple correlated wrong facts: proposal=%s budget=%s", candidate.Proposal.ID(), candidate.Budget.ID())
	}
}

func TestOrganicSourceRoleUsesRuntimeActorIDNotDisplayName(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	_ = registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	spoofCTO := registerUnboundOrganicSource(t, rt, "cto")
	_, proposal := recordOrganicTuple(t, rt, spoofCTO, spawner, guardian, allocator, "spoof-observer", false)
	if _, err := rt.validateOrganicCandidate(proposal.ID(), false); err == nil {
		t.Fatal("display-name-colliding CTO actor satisfied immutable source binding")
	}
}

func TestOrganicTupleRejectsOrderingReplayAndMaterializationContradictions(t *testing.T) {
	setup := func(t *testing.T) (*Runtime, organicSource, organicSource, organicSource, organicSource) {
		t.Helper()
		rt := newOrganicTestRuntime(t)
		return rt,
			registerOrganicSource(t, rt, "cto"),
			registerOrganicSource(t, rt, "spawner"),
			registerOrganicSource(t, rt, "guardian"),
			registerOrganicSource(t, rt, "allocator")
	}
	proposalContent := func(role, model string, canOperate bool) event.RoleProposedContent {
		return event.NewRoleProposedContent(
			role,
			model,
			[]string{"work.task.created"},
			canOperate,
			30,
			"You uphold the soul statement and inspect the cited event class. Attach findings as structured task comments while remaining non-operating.",
			"fill the observed event-handling gap",
			"spawner",
		)
	}
	recordApprovalBudget := func(t *testing.T, rt *Runtime, guardian, allocator organicSource, role string, cause types.EventID) {
		t.Helper()
		approval := recordOrganicEvent(t, rt, guardian, roleApprovedType, event.RoleApprovedContent{
			Name: role, ApprovedBy: "guardian", Reason: "bounded role approved",
		}, cause)
		recordOrganicEvent(t, rt, allocator, budgetAdjustedType, event.AgentBudgetAdjustedContent{
			AgentID: allocator.id, AgentName: role, Action: "set", NewBudget: 30,
			Resource: "iterations", Reason: "bounded grant",
		}, approval.ID())
	}

	t.Run("no genuine gap", func(t *testing.T) {
		rt, _, spawner, guardian, allocator := setup(t)
		role := "no-gap-observer"
		proposal := recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, "haiku", false))
		recordApprovalBudget(t, rt, guardian, allocator, role, proposal.ID())
		if _, err := rt.validateOrganicCandidate(proposal.ID(), false); err == nil {
			t.Fatal("proposal without a genuine same-run CTO gap was admitted")
		}
	})

	t.Run("approval before proposal", func(t *testing.T) {
		rt, cto, spawner, guardian, allocator := setup(t)
		role := "out-of-order-observer"
		gap := recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
			event.GapCategoryTechnical, role, "unhandled class", event.SeverityLevelSerious,
		))
		recordOrganicEvent(t, rt, guardian, roleApprovedType, event.RoleApprovedContent{
			Name: role, ApprovedBy: "guardian", Reason: "premature",
		}, gap.ID())
		proposal := recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, "haiku", false))
		recordOrganicEvent(t, rt, allocator, budgetAdjustedType, event.AgentBudgetAdjustedContent{
			AgentID: allocator.id, AgentName: role, Action: "set", NewBudget: 30, Resource: "iterations",
		}, proposal.ID())
		if _, err := rt.validateOrganicCandidate(proposal.ID(), false); err == nil {
			t.Fatal("approval preceding its proposal was admitted")
		}
	})

	t.Run("cross-conversation gap replay", func(t *testing.T) {
		rt, cto, spawner, guardian, allocator := setup(t)
		role := "cross-run-observer"
		recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
			event.GapCategoryTechnical, role, "prior-run unhandled class", event.SeverityLevelSerious,
		))
		nextConversation, err := newConversationID()
		if err != nil {
			t.Fatalf("new conversation: %v", err)
		}
		rt.convID = nextConversation
		proposal := recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, "haiku", false))
		recordApprovalBudget(t, rt, guardian, allocator, role, proposal.ID())
		if _, err := rt.validateOrganicCandidate(proposal.ID(), false); err == nil {
			t.Fatal("cross-conversation gap replay satisfied the current tuple")
		}
	})

	t.Run("multiple pending proposals", func(t *testing.T) {
		rt, cto, spawner, guardian, allocator := setup(t)
		role := "duplicate-pending-observer"
		gap := recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
			event.GapCategoryTechnical, role, "unhandled class", event.SeverityLevelSerious,
		))
		recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, "haiku", false), gap.ID())
		second := recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, "haiku", false))
		recordApprovalBudget(t, rt, guardian, allocator, role, second.ID())
		if _, err := rt.validateOrganicCandidate(second.ID(), false); err == nil {
			t.Fatal("multiple pending same-role proposals were admitted")
		}
	})

	t.Run("rejected latest proposal", func(t *testing.T) {
		rt, cto, spawner, guardian, allocator := setup(t)
		role := "rejected-observer"
		gap := recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
			event.GapCategoryTechnical, role, "unhandled class", event.SeverityLevelSerious,
		))
		proposal := recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, "haiku", false), gap.ID())
		rejection := recordOrganicEvent(t, rt, guardian, event.EventTypeRoleRejected, event.RoleRejectedContent{
			Name: role, RejectedBy: "guardian", Reason: "unsafe prompt",
		}, proposal.ID())
		recordApprovalBudget(t, rt, guardian, allocator, role, rejection.ID())
		if _, err := rt.validateOrganicCandidate(proposal.ID(), false); err == nil {
			t.Fatal("rejected latest proposal was revived by a later approval")
		}
	})

	for _, tc := range []struct {
		name       string
		model      string
		canOperate bool
	}{
		{name: "unknown model", model: "unknown-model"},
		{name: "operative proposal", model: "haiku", canOperate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, cto, spawner, guardian, allocator := setup(t)
			role := normalizeOrganicRole(tc.name) + "-observer"
			gap := recordOrganicEvent(t, rt, cto, gapDetectedType, event.NewGapDetectedContent(
				event.GapCategoryTechnical, role, "unhandled class", event.SeverityLevelSerious,
			))
			proposal := recordOrganicEvent(t, rt, spawner, roleProposedType, proposalContent(role, tc.model, tc.canOperate), gap.ID())
			recordApprovalBudget(t, rt, guardian, allocator, role, proposal.ID())
			if _, err := rt.validateOrganicCandidate(proposal.ID(), false); err == nil {
				t.Fatalf("%s contradiction was admitted", tc.name)
			}
		})
	}
}

func TestOrganicDaemonRecoveryStartsOnceWithoutDefinitionDuplication(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	recordOrganicTuple(t, rt, cto, spawner, guardian, allocator, "recovery-observer", false)

	firstProvider := &organicTestProvider{
		runtime: rt,
		role:    "recovery-observer",
		seen:    make(chan bool, 1),
	}
	rt.providerFactory = func(intelligence.Config) (intelligence.Provider, error) {
		return firstProvider, nil
	}
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	rt.processOrganicApprovedRoles(firstCtx)
	select {
	case <-firstProvider.seen:
	case <-time.After(3 * time.Second):
		t.Fatal("fresh actor did not evaluate")
	}
	cancelFirst()
	rt.dynamic.CancelAll()
	rt.dynamic.Wait()

	candidates, err := rt.loadOrganicRecoveryCandidates()
	if err != nil {
		t.Fatalf("recovery preflight: %v", err)
	}
	if len(candidates) != 1 || candidates[0].NormalizedRole != "recovery-observer" {
		t.Fatalf("recovery candidates = %+v", candidates)
	}
	definitionsBefore := authorityRequestsByType[RoleDefinitionContent](t, rt, EventTypeRoleDefinition)
	if len(definitionsBefore) != 1 {
		t.Fatalf("fresh definition count = %d, want 1", len(definitionsBefore))
	}

	newConvID, err := newConversationID()
	if err != nil {
		t.Fatalf("new conversation: %v", err)
	}
	rt.convID = newConvID
	head, err := rt.store.Head()
	if err != nil || !head.IsSome() {
		t.Fatalf("head before recovery run: %v", err)
	}
	if _, err := rt.graph.Record(
		EventTypeRunStarted,
		rt.humanID,
		RunStartedContent{BootstrapProfile: BootstrapProfileOrganicV1},
		[]types.EventID{head.Unwrap().ID()},
		rt.convID,
		rt.signer,
	); err != nil {
		t.Fatalf("record recovery run start: %v", err)
	}
	rt.dynamic = newDynamicAgentTracker(OrganicV1MaximumDynamicActors)
	rt.budgetRegistry = resources.NewBudgetRegistry()
	if result := rt.dynamic.Reserve(candidates[0].NormalizedRole); result != dynamicSlotReserved {
		t.Fatalf("recovery reserve = %v", result)
	}
	recoveryProvider := &organicTestProvider{
		runtime: rt,
		role:    "recovery-observer",
		seen:    make(chan bool, 1),
	}
	rt.providerFactory = func(intelligence.Config) (intelligence.Provider, error) {
		return recoveryProvider, nil
	}
	recoveryCtx, cancelRecovery := context.WithCancel(t.Context())
	if err := rt.startOrganicCandidate(recoveryCtx, candidates[0], true, false); err != nil {
		t.Fatalf("start recovery: %v", err)
	}
	select {
	case <-recoveryProvider.seen:
	case <-time.After(3 * time.Second):
		t.Fatal("recovered actor did not evaluate")
	}
	cancelRecovery()
	rt.dynamic.CancelAll()
	rt.dynamic.Wait()

	definitionsAfter := authorityRequestsByType[RoleDefinitionContent](t, rt, EventTypeRoleDefinition)
	if len(definitionsAfter) != len(definitionsBefore) {
		t.Fatalf("recovery duplicated definition: before=%d after=%d", len(definitionsBefore), len(definitionsAfter))
	}
	spawned := authorityRequestsByType[AgentSpawnedContent](t, rt, EventTypeAgentSpawned)
	recoveredCount := 0
	for _, content := range spawned {
		if normalizeOrganicRole(content.Name) == "recovery-observer" && content.Recovered {
			recoveredCount++
		}
	}
	if recoveredCount != 1 || rt.dynamic.RecoveredCount() != 1 {
		t.Fatalf("recovered spawn evidence=%d tracker=%d, want 1 each", recoveredCount, rt.dynamic.RecoveredCount())
	}
	nextRestart, err := rt.loadOrganicRecoveryCandidates()
	if err != nil {
		t.Fatalf("second-restart recovery plan: %v", err)
	}
	if len(nextRestart) != 1 ||
		nextRestart[0].Definition.ID() != candidates[0].Definition.ID() ||
		nextRestart[0].NormalizedRole != "recovery-observer" {
		t.Fatalf("second-restart recovery plan = %+v", nextRestart)
	}
}

func TestOrganicRecoveryIdentityAmbiguityFailsBeforeNewRunWrites(t *testing.T) {
	rt := newOrganicTestRuntime(t)
	cto := registerOrganicSource(t, rt, "cto")
	spawner := registerOrganicSource(t, rt, "spawner")
	guardian := registerOrganicSource(t, rt, "guardian")
	allocator := registerOrganicSource(t, rt, "allocator")
	recordOrganicTuple(t, rt, cto, spawner, guardian, allocator, "ambiguous-observer", false)
	rt.organicSpawnHook = func(stage string) error {
		if stage == "definition" {
			return errors.New("injected stop after durable definition")
		}
		return nil
	}
	var admissionErr *OrganicAdmissionError
	if err := rt.processOrganicApprovedRoles(t.Context()); !errors.As(err, &admissionErr) ||
		!admissionErr.RecoveryRequired {
		t.Fatalf("partial creation error = %v, want recovery-required admission error", err)
	}
	registerUnboundOrganicSource(t, rt, "ambiguous-observer")

	beforeEvents, err := rt.store.Count()
	if err != nil {
		t.Fatalf("event count before recovery: %v", err)
	}
	beforeActors := organicActorCount(t, rt.actors)
	next, err := New(t.Context(), Config{
		Store:                        rt.store,
		Actors:                       rt.actors,
		HumanID:                      rt.humanID,
		BootstrapProfile:             BootstrapProfileOrganicV1,
		GrowthPolicyVersion:          OrganicV1GrowthPolicyVersion,
		MaximumDynamicActors:         OrganicV1MaximumDynamicActors,
		AutomaticallyApprovedActions: []safety.ProtectedAction{safety.ActionAgentSpawnPersistent},
	})
	if err != nil {
		t.Fatalf("New recovery runtime: %v", err)
	}
	t.Cleanup(func() { _ = next.graph.Close() })
	defs, err := StarterAgentsForProfile("OrganicOperator", BootstrapProfileOrganicV1)
	if err != nil {
		t.Fatalf("organic starters: %v", err)
	}
	for _, def := range defs {
		if err := next.Register(def); err != nil {
			t.Fatalf("register %s: %v", def.Role, err)
		}
	}
	if err := next.Run(t.Context(), "must fail before write"); err == nil {
		t.Fatal("identity-ambiguous recovery unexpectedly started")
	}
	afterEvents, err := rt.store.Count()
	if err != nil {
		t.Fatalf("event count after recovery: %v", err)
	}
	afterActors := organicActorCount(t, rt.actors)
	if afterEvents != beforeEvents || afterActors != beforeActors {
		t.Fatalf(
			"recovery preflight mutated state: events %d→%d actors %d→%d",
			beforeEvents, afterEvents, beforeActors, afterActors,
		)
	}
}

func TestOrganicAdmissionBoundaryFailuresAreTerminalAndJoinOwnedLoops(t *testing.T) {
	stages := []string{
		"definition",
		"identity",
		"identity-evidence",
		"budget",
		"telemetry",
		"loop",
		"attach",
		"spawn-event",
	}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			rt := newOrganicTestRuntime(t)
			cto := registerOrganicSource(t, rt, "cto")
			spawner := registerOrganicSource(t, rt, "spawner")
			guardian := registerOrganicSource(t, rt, "guardian")
			allocator := registerOrganicSource(t, rt, "allocator")
			role := "boundary-" + stage
			recordOrganicTuple(t, rt, cto, spawner, guardian, allocator, role, false)
			rt.providerFactory = func(intelligence.Config) (intelligence.Provider, error) {
				return &organicTestProvider{
					runtime: rt,
					role:    role,
					seen:    make(chan bool, 1),
				}, nil
			}
			rt.organicSpawnHook = func(checkpoint string) error {
				if checkpoint == stage {
					return context.Canceled
				}
				return nil
			}

			var admissionErr *OrganicAdmissionError
			err := rt.processOrganicApprovedRoles(t.Context())
			if !errors.As(err, &admissionErr) {
				t.Fatalf("boundary error = %v, want OrganicAdmissionError", err)
			}
			if admissionErr.Stage != stage || !admissionErr.RecoveryRequired {
				t.Fatalf("boundary error = %+v", admissionErr)
			}
			rt.dynamic.CancelAll()
			joined := make(chan struct{})
			go func() {
				rt.dynamic.Wait()
				close(joined)
			}()
			select {
			case <-joined:
			case <-time.After(2 * time.Second):
				t.Fatal("owned dynamic goroutine did not join after terminal admission failure")
			}
			if completed := authorityRequestsByType[RunCompletedContent](t, rt, EventTypeRunCompleted); len(completed) != 0 {
				t.Fatalf("partial admission recorded %d run-completed events", len(completed))
			}
		})
	}
}

func organicActorCount(t *testing.T, actors actor.IActorStore) int {
	t.Helper()
	cursor := types.None[types.Cursor]()
	total := 0
	for {
		page, err := actors.List(actor.ActorFilter{
			Limit: defaultOperatorProjectionLimit,
			After: cursor,
		})
		if err != nil {
			t.Fatalf("list actors: %v", err)
		}
		total += len(page.Items())
		if !page.HasMore() {
			return total
		}
		cursor = page.Cursor()
	}
}
