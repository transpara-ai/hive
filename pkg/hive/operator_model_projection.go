package hive

import (
	"fmt"
	"sort"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
)

const (
	operatorModelCatalogSourceEmbedded = "embedded-defaults"
	operatorModelCatalogReloadMode     = "startup-static"
)

var defaultOperatorModelSelectionLoadedAt = time.Now().UTC()

// OperatorModelSelectionConfig describes the model resolver Hive uses to build
// the read-only operator projection. The resolver is loaded once by the process;
// hot reload is deliberately not implied by this phase-1 view.
type OperatorModelSelectionConfig struct {
	Resolver      *modelconfig.Resolver
	CatalogSource string
	LoadedAt      time.Time
	ReloadMode    string
	HotReload     bool
}

// DefaultOperatorModelSelectionConfig returns the built-in claude-cli resolver
// projection config. Callers that loaded a custom catalog should use
// OperatorModelSelectionFromCatalogPath instead.
func DefaultOperatorModelSelectionConfig(loadedAt time.Time) OperatorModelSelectionConfig {
	if loadedAt.IsZero() {
		loadedAt = defaultOperatorModelSelectionLoadedAt
	}
	return OperatorModelSelectionConfig{
		Resolver:      modelconfig.DefaultResolver(),
		CatalogSource: operatorModelCatalogSourceEmbedded,
		LoadedAt:      loadedAt,
		ReloadMode:    operatorModelCatalogReloadMode,
		HotReload:     false,
	}
}

// OperatorModelSelectionFromCatalogPath loads the resolver for the ops API
// projection. Empty catalogPath means embedded defaults.
func OperatorModelSelectionFromCatalogPath(catalogPath string, loadedAt time.Time) (OperatorModelSelectionConfig, error) {
	if catalogPath == "" {
		return DefaultOperatorModelSelectionConfig(loadedAt), nil
	}
	resolver, err := modelconfig.ResolverFromCatalogFile(catalogPath)
	if err != nil {
		return OperatorModelSelectionConfig{}, err
	}
	return OperatorModelSelectionConfig{
		Resolver:      resolver,
		CatalogSource: catalogPath,
		LoadedAt:      loadedAt,
		ReloadMode:    operatorModelCatalogReloadMode,
		HotReload:     false,
	}, nil
}

// OperatorModelSelection is the Site-facing, read-only model catalog and role
// assignment projection. Site may render it, but Hive remains the source of
// truth.
type OperatorModelSelection struct {
	Source        string                        `json:"source"`
	CatalogSource string                        `json:"catalog_source"`
	LoadedAt      time.Time                     `json:"loaded_at"`
	ReloadMode    string                        `json:"reload_mode"`
	HotReload     bool                          `json:"hot_reload"`
	Models        []OperatorModelCatalogEntry   `json:"models"`
	Assignments   []OperatorModelRoleAssignment `json:"assignments"`
	Errors        []string                      `json:"errors,omitempty"`
}

type OperatorModelCatalogEntry struct {
	ID              string               `json:"id"`
	Aliases         []string             `json:"aliases,omitempty"`
	Provider        string               `json:"provider"`
	BaseURL         string               `json:"base_url,omitempty"`
	AuthMode        string               `json:"auth_mode"`
	Tier            string               `json:"tier"`
	Capabilities    []string             `json:"capabilities,omitempty"`
	Pricing         OperatorModelPricing `json:"pricing"`
	ContextWindow   int                  `json:"context_window"`
	MaxOutputTokens int                  `json:"max_output_tokens"`
	Deprecated      bool                 `json:"deprecated,omitempty"`
	Metadata        map[string]string    `json:"metadata,omitempty"`
}

type OperatorModelPricing struct {
	InputPerMillion      float64 `json:"input_per_million"`
	OutputPerMillion     float64 `json:"output_per_million"`
	CacheReadPerMillion  float64 `json:"cache_read_per_million,omitempty"`
	CacheWritePerMillion float64 `json:"cache_write_per_million,omitempty"`
}

type OperatorModelRoleAssignment struct {
	Role                 string   `json:"role"`
	Tier                 string   `json:"tier,omitempty"`
	CanOperate           bool     `json:"can_operate"`
	Model                string   `json:"model,omitempty"`
	Provider             string   `json:"provider,omitempty"`
	AuthMode             string   `json:"auth_mode,omitempty"`
	Profile              string   `json:"profile,omitempty"`
	PolicyModel          string   `json:"policy_model,omitempty"`
	PolicyProvider       string   `json:"policy_provider,omitempty"`
	PreferredTier        string   `json:"preferred_tier,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	SelectionStrategy    string   `json:"selection_strategy,omitempty"`
	Source               string   `json:"source"`
	Trace                []string `json:"trace,omitempty"`
	Error                string   `json:"error,omitempty"`
}

// BuildOperatorModelSelection resolves the starter role assignments against the
// configured resolver and mirrors the catalog metadata needed by an operator UI.
func BuildOperatorModelSelection(config OperatorModelSelectionConfig) OperatorModelSelection {
	config = normalizeOperatorModelSelectionConfig(config)
	projection := OperatorModelSelection{
		Source:        "hive",
		CatalogSource: config.CatalogSource,
		LoadedAt:      config.LoadedAt,
		ReloadMode:    config.ReloadMode,
		HotReload:     config.HotReload,
	}

	catalog := config.Resolver.Catalog()
	for _, entry := range catalog.All() {
		projection.Models = append(projection.Models, operatorModelCatalogEntry(entry))
	}
	sort.Slice(projection.Models, func(i, j int) bool {
		return projection.Models[i].ID < projection.Models[j].ID
	})

	roles := StarterRoleDefinitions()
	roleNames := make([]string, 0, len(roles))
	for name := range roles {
		roleNames = append(roleNames, name)
	}
	sort.Strings(roleNames)
	for _, name := range roleNames {
		roleDef := roles[name]
		assignment := operatorModelRoleAssignment(config.Resolver, roleDef)
		if assignment.Error != "" {
			projection.Errors = append(projection.Errors, fmt.Sprintf("%s: %s", assignment.Role, assignment.Error))
		}
		projection.Assignments = append(projection.Assignments, assignment)
	}
	return projection
}

func normalizeOperatorModelSelectionConfig(config OperatorModelSelectionConfig) OperatorModelSelectionConfig {
	if config.Resolver == nil {
		config.Resolver = modelconfig.DefaultResolver()
	}
	if config.CatalogSource == "" {
		config.CatalogSource = operatorModelCatalogSourceEmbedded
	}
	if config.LoadedAt.IsZero() {
		config.LoadedAt = time.Now().UTC()
	}
	if config.ReloadMode == "" {
		config.ReloadMode = operatorModelCatalogReloadMode
	}
	return config
}

func operatorModelCatalogEntry(entry modelconfig.ModelCatalogEntry) OperatorModelCatalogEntry {
	return OperatorModelCatalogEntry{
		ID:           entry.ID,
		Aliases:      append([]string(nil), entry.Aliases...),
		Provider:     entry.Provider,
		BaseURL:      entry.BaseURL,
		AuthMode:     string(entry.AuthMode),
		Tier:         string(entry.Tier),
		Capabilities: capabilityStrings(entry.Capabilities),
		Pricing: OperatorModelPricing{
			InputPerMillion:      entry.Pricing.InputPerMillion,
			OutputPerMillion:     entry.Pricing.OutputPerMillion,
			CacheReadPerMillion:  entry.Pricing.CacheReadPerMillion,
			CacheWritePerMillion: entry.Pricing.CacheWritePerMillion,
		},
		ContextWindow:   entry.ContextWindow,
		MaxOutputTokens: entry.MaxOutputTokens,
		Deprecated:      entry.Deprecated,
		Metadata:        cloneStringMap(entry.Metadata),
	}
}

func operatorModelRoleAssignment(resolver *modelconfig.Resolver, role *modelconfig.RoleDefinition) OperatorModelRoleAssignment {
	assignment := OperatorModelRoleAssignment{
		Role:       role.Name,
		Tier:       role.Tier,
		CanOperate: role.CanOperate,
		Source:     "starter-role-definition",
	}
	if role.ModelPolicy != nil {
		assignment.Profile = role.ModelPolicy.Profile
		assignment.PolicyModel = role.ModelPolicy.Model
		assignment.PolicyProvider = role.ModelPolicy.Provider
		assignment.PreferredTier = string(role.ModelPolicy.PreferredTier)
		assignment.RequiredCapabilities = capabilityStrings(role.ModelPolicy.RequiredCapabilities)
		assignment.SelectionStrategy = role.ModelPolicy.SelectionStrategy
	}
	resolved, err := resolver.Resolve(modelconfig.ResolutionInput{
		Role:       role.Name,
		Policy:     role.ModelPolicy,
		CanOperate: role.CanOperate,
	})
	if err != nil {
		assignment.Error = err.Error()
		return assignment
	}
	assignment.Model = resolved.Model
	assignment.Provider = resolved.Provider
	assignment.AuthMode = string(resolved.AuthMode)
	assignment.Trace = append([]string(nil), resolved.Trace...)
	return assignment
}

func capabilityStrings(capabilities []modelconfig.Capability) []string {
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, string(capability))
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
