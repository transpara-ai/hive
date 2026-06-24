package hive

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// TestApplyPerCallBudgetFloor verifies the per-call budget floor: the default
// model catalog leaves MaxBudgetUSD=0, which makes claude-cli fall back to its
// $1/call default — too low for an opus implementer Operate. The floor fills the
// unset case; an explicit catalog value (even below the floor) always wins.
func TestApplyPerCallBudgetFloor(t *testing.T) {
	const floor = 10.0
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "unset (0) gets the floor", in: 0, want: floor},
		{name: "negative gets the floor", in: -1, want: floor},
		{name: "catalog value below floor is preserved", in: 2.5, want: 2.5},
		{name: "catalog value above floor is preserved", in: 50, want: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyPerCallBudgetFloor(intelligence.Config{MaxBudgetUSD: tt.in}, floor)
			if got.MaxBudgetUSD != tt.want {
				t.Errorf("applyPerCallBudgetFloor(MaxBudgetUSD=%v) = %v, want %v", tt.in, got.MaxBudgetUSD, tt.want)
			}
		})
	}
}

func TestNewWiresIssueScanStageRoleOutputRunner(t *testing.T) {
	ctx := context.Background()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	runner := func(context.Context, IssueScanStageRoleOutputRunnerContext) (IssueScanStageRoleOutputRunnerResult, error) {
		return IssueScanStageRoleOutputRunnerResult{
			RoleOutputs: []IssueScanStageRoleOutputEvidence{{Role: "strategist"}},
		}, nil
	}
	rt, err := New(ctx, Config{
		Store:                          store.NewInMemoryStore(),
		Actors:                         actors,
		HumanID:                        humanID,
		IssueScanStageRoleOutputRunner: runner,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if rt.issueScanStageRoleOutputRunner == nil {
		t.Fatal("issueScanStageRoleOutputRunner was not wired from Config")
	}
	result, err := rt.issueScanStageRoleOutputRunner(ctx, IssueScanStageRoleOutputRunnerContext{})
	if err != nil {
		t.Fatalf("configured runner returned error: %v", err)
	}
	if len(result.RoleOutputs) != 1 || result.RoleOutputs[0].Role != "strategist" {
		t.Fatalf("configured runner result = %+v", result)
	}
}

func TestNewWiresIssueScanImplementationRunner(t *testing.T) {
	ctx := context.Background()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	runner := func(context.Context, IssueScanImplementationRunnerContext) (IssueScanImplementationRunnerResult, error) {
		return IssueScanImplementationRunnerResult{
			OperateResultBody: "branch: codex/test\ncommit: abc123\n\npkg/hive/file.go | 1 +",
			CompletionSummary: "validation output: go test ./pkg/hive passed",
		}, nil
	}
	rt, err := New(ctx, Config{
		Store:                         store.NewInMemoryStore(),
		Actors:                        actors,
		HumanID:                       humanID,
		IssueScanImplementationRunner: runner,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if rt.issueScanImplementationRunner == nil {
		t.Fatal("issueScanImplementationRunner was not wired from Config")
	}
	result, err := rt.issueScanImplementationRunner(ctx, IssueScanImplementationRunnerContext{})
	if err != nil {
		t.Fatalf("configured runner returned error: %v", err)
	}
	if !strings.Contains(result.OperateResultBody, "branch:") || result.CompletionSummary == "" {
		t.Fatalf("configured runner result = %+v", result)
	}
}

func TestNewWiresIssueScanBlockerRepairRunner(t *testing.T) {
	ctx := context.Background()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	runner := func(context.Context, IssueScanBlockerRepairRunnerContext) (IssueScanBlockerRepairRunnerResult, error) {
		return IssueScanBlockerRepairRunnerResult{
			OperateResultBody: "branch: codex/test-repair\ncommit: def456\n\npkg/hive/file.go | 1 +",
			CompletionSummary: "validation output: go test ./pkg/hive passed after repair",
		}, nil
	}
	rt, err := New(ctx, Config{
		Store:                        store.NewInMemoryStore(),
		Actors:                       actors,
		HumanID:                      humanID,
		IssueScanBlockerRepairRunner: runner,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if rt.issueScanBlockerRepairRunner == nil {
		t.Fatal("issueScanBlockerRepairRunner was not wired from Config")
	}
	result, err := rt.issueScanBlockerRepairRunner(ctx, IssueScanBlockerRepairRunnerContext{})
	if err != nil {
		t.Fatalf("configured runner returned error: %v", err)
	}
	if !strings.Contains(result.OperateResultBody, "branch:") || result.CompletionSummary == "" {
		t.Fatalf("configured runner result = %+v", result)
	}
}

func TestNewWiresIssueScanDraftPRAuthorityRequester(t *testing.T) {
	ctx := context.Background()
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	requester := func(context.Context, IssueScanDraftPRAuthorityRequestRunnerContext) (IssueScanDraftPRAuthorityRequestRunnerResult, error) {
		return IssueScanDraftPRAuthorityRequestRunnerResult{
			BaseRef: "main",
			BaseSHA: "abc123",
			Nonce:   "nonce-test",
		}, nil
	}
	rt, err := New(ctx, Config{
		Store:                              store.NewInMemoryStore(),
		Actors:                             actors,
		HumanID:                            humanID,
		IssueScanDraftPRAuthorityRequester: requester,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if rt.issueScanDraftPRAuthorityRequester == nil {
		t.Fatal("issueScanDraftPRAuthorityRequester was not wired from Config")
	}
	result, err := rt.issueScanDraftPRAuthorityRequester(ctx, IssueScanDraftPRAuthorityRequestRunnerContext{})
	if err != nil {
		t.Fatalf("configured requester returned error: %v", err)
	}
	if result.BaseSHA == "" || result.Nonce == "" {
		t.Fatalf("configured requester result = %+v", result)
	}
}

func TestSpawnAgent_WarnsWhenCanOperateButProviderLacksIOperator(t *testing.T) {
	tests := []struct {
		name          string
		canOperate    bool
		providerOps   bool
		expectWarning bool
	}{
		{name: "CanOperate=true + non-IOperator → warn", canOperate: true, providerOps: false, expectWarning: true},
		{name: "CanOperate=false + non-IOperator → no warn", canOperate: false, providerOps: false, expectWarning: false},
		{name: "CanOperate=true + IOperator → no warn", canOperate: true, providerOps: true, expectWarning: false},
		{name: "CanOperate=false + IOperator → no warn", canOperate: false, providerOps: true, expectWarning: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emitWarning := canOperateMismatch(tt.canOperate, tt.providerOps)
			if emitWarning != tt.expectWarning {
				t.Errorf("canOperateMismatch returned %v; want %v", emitWarning, tt.expectWarning)
			}
		})
	}
}

func TestRuntimeModelCatalogReloadAffectsNextSpawn(t *testing.T) {
	ctx := context.Background()
	initialCatalogTime := time.Unix(1_700_000_000, 0).UTC()
	reloadTime := initialCatalogTime.Add(100 * time.Second)
	catalogPath := writeRoleDefaultCatalog(t, "guardian", "haiku")
	forceCatalogModTime(t, catalogPath, initialCatalogTime)
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	r, err := New(ctx, Config{
		Store:                 store.NewInMemoryStore(),
		Actors:                actors,
		HumanID:               humanID,
		CatalogPath:           catalogPath,
		CatalogReloadInterval: time.Minute,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	manager, err := NewOperatorModelSelectionManager(catalogPath, initialCatalogTime, true)
	if err != nil {
		t.Fatalf("NewOperatorModelSelectionManager: %v", err)
	}
	r.modelSelectionManager = manager
	r.setResolver(manager.Snapshot().Resolver)

	var captures []intelligence.Config
	r.providerFactory = func(cfg intelligence.Config) (intelligence.Provider, error) {
		captures = append(captures, cfg)
		return &runtimeOverrideTestProvider{name: cfg.Provider, model: cfg.Model}, nil
	}

	firstDef := hotReloadTestAgentDef("guardian-before-reload")
	_, firstModel, err := r.spawnAgent(ctx, firstDef)
	if err != nil {
		t.Fatalf("first spawnAgent: %v", err)
	}
	if firstModel == "" {
		t.Fatal("first spawn resolved empty model")
	}

	writeRoleDefaultCatalogAt(t, catalogPath, "guardian", "sonnet")
	forceCatalogModTime(t, catalogPath, reloadTime)
	changed, err := r.reloadModelCatalogOnce(reloadTime)
	if err != nil {
		t.Fatalf("reloadModelCatalogOnce: %v", err)
	}
	if !changed {
		t.Fatal("reloadModelCatalogOnce changed = false, want true after catalog edit")
	}

	secondDef := hotReloadTestAgentDef("guardian-after-reload")
	_, secondModel, err := r.spawnAgent(ctx, secondDef)
	if err != nil {
		t.Fatalf("second spawnAgent: %v", err)
	}
	if secondModel == "" {
		t.Fatal("second spawn resolved empty model")
	}
	if firstModel == secondModel {
		t.Fatalf("next spawn kept old model after reload: first=%q second=%q", firstModel, secondModel)
	}
	if len(captures) != 2 {
		t.Fatalf("provider captures = %d, want 2", len(captures))
	}
	if captures[0].Model != firstModel || captures[1].Model != secondModel {
		t.Fatalf("provider captures = %s then %s, want %s then %s", captures[0].Model, captures[1].Model, firstModel, secondModel)
	}
	if captures[0].Provider != "claude-cli" || captures[1].Provider != "claude-cli" {
		t.Fatalf("providers = %s then %s, want claude-cli subscription path", captures[0].Provider, captures[1].Provider)
	}
}

func hotReloadTestAgentDef(name string) AgentDef {
	return AgentDef{
		Name:                name,
		Role:                "guardian",
		SystemPrompt:        "watch the hive",
		CanOperate:          false,
		RoleDefinition:      StarterRoleDefinitions()["guardian"],
		IdentityEnvironment: AgentIdentityEnvironmentTest,
		IdentityMode:        AgentIdentityModeDeterministicFixture,
	}
}

func forceCatalogModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("advance catalog modtime: %v", err)
	}
}

type runtimeOverrideTestProvider struct {
	name  string
	model string
}

func (p runtimeOverrideTestProvider) Name() string  { return p.name }
func (p runtimeOverrideTestProvider) Model() string { return p.model }
func (p runtimeOverrideTestProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	score, _ := types.NewScore(0.8)
	return decision.NewResponse("ok", score, decision.TokenUsage{}), nil
}
func (p runtimeOverrideTestProvider) Operate(_ context.Context, _ decision.OperateTask) (decision.OperateResult, error) {
	return decision.OperateResult{Summary: "ok"}, nil
}

var (
	_ intelligence.Provider = (*runtimeOverrideTestProvider)(nil)
	_ decision.IOperator    = (*runtimeOverrideTestProvider)(nil)
)

func TestTaskOperateProviderForUsesFactoryOrderOverride(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	maxCost := 4.5
	task, err := work.SeedFactoryOrder(ts, source, work.FactoryOrder{
		ID:                 "fo_model_override",
		Title:              "Use override",
		Intent:             "Exercise the override path",
		DefinitionOfDone:   "done",
		AcceptanceCriteria: "accepted",
		TestPlan:           "go test",
		ModelOverrides: []work.FactoryOrderModelOverride{
			{
				Role:              "implementer",
				Model:             "sonnet",
				RequestedAuthMode: "subscription",
				MaxCostPerCallUSD: &maxCost,
				ResolvedModel:     "claude-sonnet-4-6",
				ResolvedProvider:  "claude-cli",
				AuthMode:          "subscription",
			},
		},
	}, []types.EventID{cause}, conv)
	if err != nil {
		t.Fatalf("SeedFactoryOrder: %v", err)
	}

	var captured intelligence.Config
	r := &Runtime{
		tasks: ts,
		providerFactory: func(cfg intelligence.Config) (intelligence.Provider, error) {
			captured = cfg
			return &runtimeOverrideTestProvider{name: cfg.Provider, model: cfg.Model}, nil
		},
	}
	r.setResolver(modelconfig.DefaultResolver())
	def := AgentDef{
		Name:           "implementer",
		Role:           "implementer",
		SystemPrompt:   "system prompt",
		CanOperate:     true,
		RoleDefinition: StarterRoleDefinitions()["implementer"],
	}

	got, err := r.taskOperateProviderFor(def)(context.Background(), task, "implementer")
	if err != nil {
		t.Fatalf("taskOperateProviderFor: %v", err)
	}
	if !got.Applied {
		t.Fatal("TaskOperateProvider applied = false, want true")
	}
	if _, ok := got.Provider.(decision.IOperator); !ok {
		t.Fatal("selected provider does not implement IOperator")
	}
	if captured.Provider != "claude-cli" || captured.Model != "claude-sonnet-4-6" {
		t.Fatalf("captured provider/model = %s/%s, want claude-cli/claude-sonnet-4-6", captured.Provider, captured.Model)
	}
	if captured.MaxBudgetUSD != maxCost {
		t.Fatalf("captured MaxBudgetUSD = %v, want override cap %v", captured.MaxBudgetUSD, maxCost)
	}
	if captured.SystemPrompt != "system prompt" {
		t.Fatalf("captured SystemPrompt = %q, want runtime def prompt", captured.SystemPrompt)
	}
}

func TestResolveTaskModelOverrideRejectsStoredResolutionDrift(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	task, err := work.SeedFactoryOrder(ts, source, work.FactoryOrder{
		ID:                 "fo_model_drift",
		Title:              "Use override",
		Intent:             "Exercise drift detection",
		DefinitionOfDone:   "done",
		AcceptanceCriteria: "accepted",
		TestPlan:           "go test",
		ModelOverrides: []work.FactoryOrderModelOverride{
			{
				Role:              "implementer",
				Model:             "sonnet",
				RequestedAuthMode: "subscription",
				ResolvedModel:     "claude-opus-4-6",
				ResolvedProvider:  "claude-cli",
				AuthMode:          "subscription",
			},
		},
	}, []types.EventID{cause}, conv)
	if err != nil {
		t.Fatalf("SeedFactoryOrder: %v", err)
	}
	r := &Runtime{tasks: ts}
	r.setResolver(modelconfig.DefaultResolver())
	def := AgentDef{
		Role:           "implementer",
		CanOperate:     true,
		RoleDefinition: StarterRoleDefinitions()["implementer"],
	}

	_, _, applied, err := r.resolveTaskModelOverride(task, def, "implementer")
	if err == nil {
		t.Fatal("resolveTaskModelOverride error = nil, want stored resolution drift failure")
	}
	if !applied {
		t.Fatal("applied = false, want true because the matching override must fail closed")
	}
	if !strings.Contains(err.Error(), "stored resolved_model") {
		t.Fatalf("error = %q, want stored resolved_model drift", err.Error())
	}
}

func TestResolveTaskModelOverrideRejectsAPIKeyWithoutExplicitOptIn(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	task, err := work.SeedFactoryOrder(ts, source, work.FactoryOrder{
		ID:                 "fo_api_key_opt_in",
		Title:              "Use API model",
		Intent:             "Exercise api-key opt-in",
		DefinitionOfDone:   "done",
		AcceptanceCriteria: "accepted",
		TestPlan:           "go test",
		ModelOverrides: []work.FactoryOrderModelOverride{
			{
				Role:             "guardian",
				Model:            "api-sonnet",
				ResolvedModel:    "api-claude-sonnet-4-6",
				ResolvedProvider: "anthropic",
				AuthMode:         "api-key",
			},
		},
	}, []types.EventID{cause}, conv)
	if err != nil {
		t.Fatalf("SeedFactoryOrder: %v", err)
	}
	r := &Runtime{tasks: ts}
	r.setResolver(modelconfig.DefaultResolver())
	def := AgentDef{
		Role:           "guardian",
		CanOperate:     false,
		RoleDefinition: StarterRoleDefinitions()["guardian"],
	}

	_, _, applied, err := r.resolveTaskModelOverride(task, def, "guardian")
	if err == nil {
		t.Fatal("resolveTaskModelOverride error = nil, want api-key opt-in failure")
	}
	if !applied {
		t.Fatal("applied = false, want true because the matching override must fail closed")
	}
	if !strings.Contains(err.Error(), "set auth_mode to \"api-key\"") {
		t.Fatalf("error = %q, want explicit api-key opt-in failure", err.Error())
	}
}

func TestResolveAgentModelUsesHiveOwnedRolePolicyEvent(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeModelRolePolicyUpdated, ModelRolePolicyUpdatedContent{
		Role:              "guardian",
		Model:             "api-sonnet",
		RequestedAuthMode: "api-key",
		ResolvedModel:     "api-claude-sonnet-4-6",
		ResolvedProvider:  "anthropic",
		AuthMode:          "api-key",
	})
	r := &Runtime{store: s}
	r.setResolver(modelconfig.DefaultResolver())
	def := AgentDef{
		Role:           "guardian",
		CanOperate:     false,
		RoleDefinition: StarterRoleDefinitions()["guardian"],
	}

	resolved, err := r.resolveAgentModel(def)
	if err != nil {
		t.Fatalf("resolveAgentModel: %v", err)
	}
	if resolved.Provider != "anthropic" || resolved.Model != "api-claude-sonnet-4-6" || resolved.AuthMode != modelconfig.AuthAPIKey {
		t.Fatalf("resolved = %s/%s [%s], want anthropic/api-claude-sonnet-4-6 [api-key]", resolved.Provider, resolved.Model, resolved.AuthMode)
	}
}

func TestResolveAgentModelRejectsHiveOwnedRolePolicyWithoutAPIKeyOptIn(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeModelRolePolicyUpdated, ModelRolePolicyUpdatedContent{
		Role:             "guardian",
		Model:            "api-sonnet",
		ResolvedModel:    "api-claude-sonnet-4-6",
		ResolvedProvider: "anthropic",
		AuthMode:         "api-key",
	})
	r := &Runtime{store: s}
	r.setResolver(modelconfig.DefaultResolver())
	def := AgentDef{
		Role:           "guardian",
		CanOperate:     false,
		RoleDefinition: StarterRoleDefinitions()["guardian"],
	}

	_, err := r.resolveAgentModel(def)
	if err == nil {
		t.Fatal("resolveAgentModel error = nil, want api-key opt-in failure")
	}
	if !strings.Contains(err.Error(), "set auth_mode to \"api-key\"") {
		t.Fatalf("error = %q, want explicit api-key opt-in failure", err.Error())
	}
}

func TestResolveTaskModelOverrideUsesHiveOwnedRolePolicyWithoutFactoryOverride(t *testing.T) {
	s, _, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeModelRolePolicyUpdated, ModelRolePolicyUpdatedContent{
		Role:              "implementer",
		Model:             "sonnet",
		RequestedAuthMode: "subscription",
		ResolvedModel:     "claude-sonnet-4-6",
		ResolvedProvider:  "claude-cli",
		AuthMode:          "subscription",
	})
	r := &Runtime{store: s}
	r.setResolver(modelconfig.DefaultResolver())
	def := AgentDef{
		Role:           "implementer",
		CanOperate:     true,
		RoleDefinition: StarterRoleDefinitions()["implementer"],
	}

	resolved, override, applied, err := r.resolveTaskModelOverride(work.Task{}, def, "implementer")
	if err != nil {
		t.Fatalf("resolveTaskModelOverride: %v", err)
	}
	if !applied {
		t.Fatal("applied = false, want active role policy to apply")
	}
	if override.Role != "" {
		t.Fatalf("override = %+v, want no FactoryOrder override payload", override)
	}
	if resolved.Provider != "claude-cli" || resolved.Model != "claude-sonnet-4-6" || resolved.AuthMode != modelconfig.AuthSubscription {
		t.Fatalf("resolved = %s/%s [%s], want claude-cli/claude-sonnet-4-6 [subscription]", resolved.Provider, resolved.Model, resolved.AuthMode)
	}
}
