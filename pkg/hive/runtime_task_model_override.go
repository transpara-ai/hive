package hive

import (
	"context"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/loop"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/work"
)

func (r *Runtime) taskOperateProviderFor(def AgentDef) loop.TaskOperateProviderFunc {
	return func(ctx context.Context, task work.Task, role string) (loop.TaskOperateProviderResult, error) {
		resolved, override, applied, err := r.resolveTaskModelOverride(task, def, role)
		if err != nil || !applied {
			return loop.TaskOperateProviderResult{}, err
		}

		cfg := modelconfig.ToIntelligenceConfig(resolved, def.SystemPrompt)
		if override.MaxCostPerCallUSD != nil {
			cfg.MaxBudgetUSD = *override.MaxCostPerCallUSD
		} else {
			cfg = applyPerCallBudgetFloor(cfg, defaultPerCallBudgetUSD)
		}

		provider, err := r.newProvider(cfg)
		if err != nil {
			return loop.TaskOperateProviderResult{}, fmt.Errorf("create override provider for role %q model %q: %w", role, resolved.Model, err)
		}
		if _, ok := provider.(decision.IOperator); !ok {
			return loop.TaskOperateProviderResult{}, fmt.Errorf("override provider %s/%s does not support Operate", provider.Name(), provider.Model())
		}

		return loop.TaskOperateProviderResult{
			Applied:  true,
			Provider: resources.NewTrackingProvider(provider),
		}, nil
	}
}

func (r *Runtime) resolveTaskModelOverride(task work.Task, def AgentDef, role string) (modelconfig.ResolvedConfig, work.FactoryOrderModelOverride, bool, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		role = def.Role
	}
	canonicalRole, _, ok := canonicalStarterRole(role)
	if !ok {
		return modelconfig.ResolvedConfig{}, work.FactoryOrderModelOverride{}, false, nil
	}
	if !sameRole(def.Role, canonicalRole) {
		return modelconfig.ResolvedConfig{}, work.FactoryOrderModelOverride{}, false, nil
	}
	if r.tasks == nil {
		return modelconfig.ResolvedConfig{}, work.FactoryOrderModelOverride{}, false, nil
	}

	projection, err := r.tasks.ProjectTask(task.ID)
	if err != nil {
		return modelconfig.ResolvedConfig{}, work.FactoryOrderModelOverride{}, false, fmt.Errorf("project task %s for model overrides: %w", task.ID.Value(), err)
	}
	override, ok := factoryOrderModelOverrideForRole(projection.ModelOverrides, canonicalRole)
	if !ok {
		return modelconfig.ResolvedConfig{}, work.FactoryOrderModelOverride{}, false, nil
	}

	request := modelOverrideRequestFromFactoryOrder(override)
	policy, recorded, err := runLaunchOverridePolicy(0, request)
	if err != nil {
		return modelconfig.ResolvedConfig{}, override, true, err
	}
	resolved, err := r.currentResolver().Resolve(modelconfig.ResolutionInput{
		Role:         canonicalRole,
		Policy:       def.EffectiveModelPolicy(),
		TaskOverride: policy,
		CanOperate:   def.CanOperate,
	})
	if err != nil {
		return modelconfig.ResolvedConfig{}, override, true, fmt.Errorf("model override for role %q is unsafe: %w", canonicalRole, err)
	}
	if err := validateRunLaunchOverrideResolvedConfig(0, canonicalRole, recorded.RequestedAuthMode, resolved); err != nil {
		return modelconfig.ResolvedConfig{}, override, true, err
	}
	if err := validateStoredFactoryOrderResolution(canonicalRole, override, resolved); err != nil {
		return modelconfig.ResolvedConfig{}, override, true, err
	}
	return resolved, override, true, nil
}

func modelOverrideRequestFromFactoryOrder(override work.FactoryOrderModelOverride) ModelOverrideRequest {
	return ModelOverrideRequest{
		Role:                 override.Role,
		Model:                override.Model,
		Provider:             override.Provider,
		Profile:              override.Profile,
		AuthMode:             override.RequestedAuthMode,
		PreferredTier:        override.PreferredTier,
		RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
		MaxCostPerCallUSD:    cloneTaskModelOverrideFloat64Ptr(override.MaxCostPerCallUSD),
	}
}

func cloneTaskModelOverrideFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func factoryOrderModelOverrideForRole(overrides []work.FactoryOrderModelOverride, role string) (work.FactoryOrderModelOverride, bool) {
	for _, override := range overrides {
		if sameRole(override.Role, role) {
			return override, true
		}
	}
	return work.FactoryOrderModelOverride{}, false
}

func canonicalStarterRole(role string) (string, *modelconfig.RoleDefinition, bool) {
	for name, def := range StarterRoleDefinitions() {
		if sameRole(name, role) {
			return name, def, true
		}
	}
	return "", nil, false
}

func sameRole(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func validateStoredFactoryOrderResolution(role string, override work.FactoryOrderModelOverride, resolved modelconfig.ResolvedConfig) error {
	if override.ResolvedModel != "" && override.ResolvedModel != resolved.Model {
		return fmt.Errorf("model override for role %q stored resolved_model %q but current resolver produced %q", role, override.ResolvedModel, resolved.Model)
	}
	if override.ResolvedProvider != "" && override.ResolvedProvider != resolved.Provider {
		return fmt.Errorf("model override for role %q stored resolved_provider %q but current resolver produced %q", role, override.ResolvedProvider, resolved.Provider)
	}
	if override.AuthMode != "" && override.AuthMode != string(resolved.AuthMode) {
		return fmt.Errorf("model override for role %q stored auth_mode %q but current resolver produced %q", role, override.AuthMode, resolved.AuthMode)
	}
	return nil
}
