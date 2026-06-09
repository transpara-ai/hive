package hive

import (
	"context"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
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
