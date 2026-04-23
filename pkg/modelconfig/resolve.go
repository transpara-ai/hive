package modelconfig

import "fmt"

// ResolvedConfig is the fully-resolved, provider-agnostic model configuration
// ready to be adapted into an intelligence.Config.
type ResolvedConfig struct {
	Model        string            `json:"model"`
	Provider     string            `json:"provider"`
	BaseURL      string            `json:"base_url,omitempty"`
	APIKey       string            `json:"-"` // never serialized
	AuthMode     AuthMode          `json:"auth_mode"`
	MaxTokens    int               `json:"max_tokens,omitempty"`
	Temperature  float64           `json:"temperature,omitempty"`
	MaxBudgetUSD float64           `json:"max_budget_usd,omitempty"`
	Entry        ModelCatalogEntry `json:"entry"`
	Trace        []string          `json:"trace"`
}

// ResolutionInput collects all the sources the resolver considers.
type ResolutionInput struct {
	Role          string           // agent role name
	AgentDefModel string           // legacy AgentDef.Model field
	Policy        *RoleModelPolicy // role-declared policy (may be nil)
	TaskOverride  *RoleModelPolicy // allocator per-task override (may be nil)
	CanOperate    bool             // forces claude-cli provider
}

// ResolverDefaults configures the base layer of the precedence chain.
type ResolverDefaults struct {
	Provider   string               // default provider (e.g. "claude-cli")
	Model      string               // fallback model ID
	TierModels map[ModelTier]string  // tier -> default model ID
	RoleModels map[string]string     // role -> model alias
}

// Resolver resolves model configuration through a deterministic precedence chain.
type Resolver struct {
	catalog  *ModelCatalog
	profiles map[string]ModelProfile
	defaults ResolverDefaults
}

// NewResolver creates a Resolver with the given catalog, profiles, and defaults.
func NewResolver(catalog *ModelCatalog, profiles map[string]ModelProfile, defaults ResolverDefaults) *Resolver {
	return &Resolver{
		catalog:  catalog,
		profiles: profiles,
		defaults: defaults,
	}
}

// Defaults returns the resolver's default configuration.
func (r *Resolver) Defaults() ResolverDefaults {
	return r.defaults
}

// Resolve applies the precedence chain and returns a ResolvedConfig.
//
// Precedence (later layers win):
//  1. System defaults (Provider, fallback model)
//  2. Role defaults (RoleModels map)
//  3. AgentDef.Model (legacy field)
//  4. Named profile (if Policy.Profile is set)
//  5. Role policy (Policy.Model, Policy.Provider)
//  6. Task override (TaskOverride fields)
//  7. CanOperate constraint (forces Provider="claude-cli")
func (r *Resolver) Resolve(input ResolutionInput) (ResolvedConfig, error) {
	var rc ResolvedConfig
	var modelName string

	// Layer 1: System defaults
	rc.Provider = r.defaults.Provider
	modelName = r.defaults.Model
	rc.Trace = append(rc.Trace, fmt.Sprintf("provider: system default → %s", rc.Provider))
	rc.Trace = append(rc.Trace, fmt.Sprintf("model: system default → %s", modelName))

	// Layer 2: Role defaults
	if alias, ok := r.defaults.RoleModels[input.Role]; ok {
		modelName = alias
		rc.Trace = append(rc.Trace, fmt.Sprintf("model: role default (%s→%s)", input.Role, alias))
	}

	// Layer 3: AgentDef.Model (legacy)
	if input.AgentDefModel != "" {
		modelName = input.AgentDefModel
		rc.Trace = append(rc.Trace, fmt.Sprintf("model: AgentDef.Model → %s", modelName))
	}

	// Layer 4+5: Policy (profile, then explicit fields)
	if input.Policy != nil {
		r.applyPolicy(&rc, &modelName, input.Policy, "policy")
	}

	// Layer 6: Task override
	if input.TaskOverride != nil {
		r.applyPolicy(&rc, &modelName, input.TaskOverride, "task-override")
	}

	// Resolve model name to catalog entry
	entry, ok := r.catalog.Lookup(modelName)
	if !ok {
		// If modelName looks like a tier, resolve via tier defaults
		if tierModel, tierOK := r.defaults.TierModels[ModelTier(modelName)]; tierOK {
			entry, ok = r.catalog.Lookup(tierModel)
			if ok {
				rc.Trace = append(rc.Trace, fmt.Sprintf("model: tier %s → %s", modelName, tierModel))
			}
		}
		if !ok {
			return ResolvedConfig{}, fmt.Errorf("model %q not found in catalog", modelName)
		}
	}

	rc.Model = entry.ID
	rc.Entry = entry
	rc.AuthMode = entry.AuthMode

	// Apply catalog entry defaults where not already set
	if rc.BaseURL == "" && entry.BaseURL != "" {
		rc.BaseURL = entry.BaseURL
	}
	if rc.Provider == r.defaults.Provider && entry.Provider != "" {
		// Only override provider from entry if we're still on system default
		// (explicit policy/override already set it otherwise)
	}

	// Layer 7: CanOperate constraint
	if input.CanOperate {
		if rc.Provider != "claude-cli" {
			rc.Trace = append(rc.Trace, fmt.Sprintf("provider: CanOperate forced claude-cli (was %s)", rc.Provider))
		}
		rc.Provider = "claude-cli"
		rc.AuthMode = AuthSubscription
	}

	// Capability validation
	if input.Policy != nil && len(input.Policy.RequiredCapabilities) > 0 {
		if missing := ValidateCapabilities(entry, input.Policy.RequiredCapabilities); len(missing) > 0 {
			return ResolvedConfig{}, fmt.Errorf("model %s missing capabilities: %v", entry.ID, missing)
		}
	}
	if input.TaskOverride != nil && len(input.TaskOverride.RequiredCapabilities) > 0 {
		if missing := ValidateCapabilities(entry, input.TaskOverride.RequiredCapabilities); len(missing) > 0 {
			return ResolvedConfig{}, fmt.Errorf("model %s missing capabilities for task: %v", entry.ID, missing)
		}
	}
	if input.CanOperate {
		if err := ValidateForOperate(entry); err != nil {
			return ResolvedConfig{}, err
		}
	}

	rc.Trace = append(rc.Trace, fmt.Sprintf("resolved: %s/%s [%s]", rc.Provider, rc.Model, rc.AuthMode))
	return rc, nil
}

// applyPolicy merges a RoleModelPolicy into the in-progress resolution.
func (r *Resolver) applyPolicy(rc *ResolvedConfig, modelName *string, policy *RoleModelPolicy, source string) {
	// Profile expansion first (lower precedence than explicit fields)
	if policy.Profile != "" {
		if profile, ok := r.profiles[policy.Profile]; ok {
			if profile.Model != "" {
				*modelName = profile.Model
				rc.Trace = append(rc.Trace, fmt.Sprintf("model: %s profile %q → %s", source, policy.Profile, profile.Model))
			}
			if profile.Provider != "" {
				rc.Provider = profile.Provider
				rc.Trace = append(rc.Trace, fmt.Sprintf("provider: %s profile %q → %s", source, policy.Profile, profile.Provider))
			}
			if profile.BaseURL != "" {
				rc.BaseURL = profile.BaseURL
			}
			if profile.MaxTokens != nil {
				rc.MaxTokens = *profile.MaxTokens
			}
			if profile.Temperature != nil {
				rc.Temperature = *profile.Temperature
			}
			if profile.MaxBudgetUSD != nil {
				rc.MaxBudgetUSD = *profile.MaxBudgetUSD
			}
		}
	}

	// Explicit policy fields override profile
	if policy.Model != "" {
		*modelName = policy.Model
		rc.Trace = append(rc.Trace, fmt.Sprintf("model: %s explicit → %s", source, policy.Model))
	}
	if policy.Provider != "" {
		rc.Provider = policy.Provider
		rc.Trace = append(rc.Trace, fmt.Sprintf("provider: %s explicit → %s", source, policy.Provider))
	}

	// Tier-based resolution: if PreferredTier is set and no explicit model, resolve tier
	if policy.PreferredTier != "" && policy.Model == "" && policy.Profile == "" {
		if tierModel, ok := r.defaults.TierModels[policy.PreferredTier]; ok {
			*modelName = tierModel
			rc.Trace = append(rc.Trace, fmt.Sprintf("model: %s tier %s → %s", source, policy.PreferredTier, tierModel))
		}
	}
}
