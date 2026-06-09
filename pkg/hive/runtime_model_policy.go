package hive

import (
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
)

func (r *Runtime) activeRoleModelPolicy(role string, base *modelconfig.RoleModelPolicy) (*modelconfig.RoleModelPolicy, OperatorModelRolePolicy, bool, error) {
	stored, ok, err := latestModelRolePolicyUpdateForRole(r.store, role, defaultOperatorProjectionLimit)
	if err != nil {
		return nil, OperatorModelRolePolicy{}, false, err
	}
	if !ok || stored.Policy == nil {
		return base, OperatorModelRolePolicy{}, false, nil
	}
	return mergeRoleModelPolicy(base, stored.Policy), stored, true, nil
}

func (r *Runtime) resolveAgentModel(def AgentDef) (modelconfig.ResolvedConfig, error) {
	role := strings.TrimSpace(def.Role)
	policy, stored, hasStored, err := r.activeRoleModelPolicy(role, def.EffectiveModelPolicy())
	if err != nil {
		return modelconfig.ResolvedConfig{}, fmt.Errorf("load active model policy for %s: %w", role, err)
	}
	resolved, err := r.currentResolver().Resolve(modelconfig.ResolutionInput{
		Role:          role,
		AgentDefModel: def.Model,
		Policy:        policy,
		CanOperate:    def.CanOperate,
	})
	if err != nil {
		return modelconfig.ResolvedConfig{}, err
	}
	if hasStored {
		if err := validateActiveRoleModelPolicyResolution(role, stored, resolved); err != nil {
			return modelconfig.ResolvedConfig{}, err
		}
	}
	return resolved, nil
}

func (r *Runtime) resolveActiveRoleModelPolicy(def AgentDef, role string, policy *modelconfig.RoleModelPolicy, stored OperatorModelRolePolicy) (modelconfig.ResolvedConfig, error) {
	resolved, err := r.currentResolver().Resolve(modelconfig.ResolutionInput{
		Role:          role,
		AgentDefModel: def.Model,
		Policy:        policy,
		CanOperate:    def.CanOperate,
	})
	if err != nil {
		return modelconfig.ResolvedConfig{}, err
	}
	if err := validateActiveRoleModelPolicyResolution(role, stored, resolved); err != nil {
		return modelconfig.ResolvedConfig{}, err
	}
	return resolved, nil
}

func validateActiveRoleModelPolicyResolution(role string, stored OperatorModelRolePolicy, resolved modelconfig.ResolvedConfig) error {
	if err := validateRunLaunchOverrideResolvedConfig(0, role, stored.RequestedAuthMode, resolved); err != nil {
		return err
	}
	if err := validateStoredModelRolePolicyResolution(role, stored, resolved); err != nil {
		return err
	}
	return nil
}
