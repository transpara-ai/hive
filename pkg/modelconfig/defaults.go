package modelconfig

import (
	_ "embed"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed defaults_catalog.yaml
var defaultCatalogYAML []byte

// catalogFile is the top-level YAML structure for parsing.
type catalogFile struct {
	Models       []ModelCatalogEntry  `yaml:"models"`
	TierDefaults map[string]string    `yaml:"tier_defaults"`
	RoleDefaults map[string]string    `yaml:"role_defaults"`
	Profiles     map[string]profileYAML `yaml:"profiles"`
}

// profileYAML is the YAML representation of a profile (name comes from map key).
type profileYAML struct {
	Model        string   `yaml:"model"`
	Provider     string   `yaml:"provider,omitempty"`
	BaseURL      string   `yaml:"base_url,omitempty"`
	MaxTokens    *int     `yaml:"max_tokens,omitempty"`
	Temperature  *float64 `yaml:"temperature,omitempty"`
	MaxBudgetUSD *float64 `yaml:"max_budget_usd,omitempty"`
}

// ParseCatalogYAML parses a catalog YAML file into its components.
func ParseCatalogYAML(data []byte) (*ModelCatalog, map[string]ModelProfile, ResolverDefaults, error) {
	var f catalogFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, nil, ResolverDefaults{}, fmt.Errorf("parse catalog YAML: %w", err)
	}

	catalog, err := NewCatalog(f.Models)
	if err != nil {
		return nil, nil, ResolverDefaults{}, fmt.Errorf("build catalog: %w", err)
	}

	profiles := make(map[string]ModelProfile, len(f.Profiles))
	for name, p := range f.Profiles {
		profiles[name] = ModelProfile{
			Name:         name,
			Model:        p.Model,
			Provider:     p.Provider,
			BaseURL:      p.BaseURL,
			MaxTokens:    p.MaxTokens,
			Temperature:  p.Temperature,
			MaxBudgetUSD: p.MaxBudgetUSD,
		}
	}

	tierModels := make(map[ModelTier]string, len(f.TierDefaults))
	for tier, model := range f.TierDefaults {
		tierModels[ModelTier(tier)] = model
	}

	defaults := ResolverDefaults{
		Provider:   "claude-cli",
		Model:      "claude-sonnet-4-6",
		TierModels: tierModels,
		RoleModels: f.RoleDefaults,
	}

	// Apply env var overrides to defaults.
	if p := os.Getenv("HIVE_PROVIDER"); p != "" {
		defaults.Provider = p
	}
	if m := os.Getenv("HIVE_MODEL"); m != "" {
		defaults.Model = m
	}

	return catalog, profiles, defaults, nil
}

var (
	defaultCatalogOnce sync.Once
	defaultCatalog     *ModelCatalog
	defaultProfiles    map[string]ModelProfile
	defaultDefaults    ResolverDefaults
	defaultCatalogErr  error
)

func initDefaults() {
	defaultCatalog, defaultProfiles, defaultDefaults, defaultCatalogErr = ParseCatalogYAML(defaultCatalogYAML)
}

// DefaultCatalog returns the built-in model catalog (embedded YAML).
func DefaultCatalog() *ModelCatalog {
	defaultCatalogOnce.Do(initDefaults)
	if defaultCatalogErr != nil {
		panic("modelconfig: embedded catalog is invalid: " + defaultCatalogErr.Error())
	}
	return defaultCatalog
}

var (
	defaultResolverOnce sync.Once
	defaultResolver     *Resolver
)

// DefaultResolver returns a Resolver built from the embedded defaults.
func DefaultResolver() *Resolver {
	defaultResolverOnce.Do(func() {
		catalog := DefaultCatalog()
		defaultResolver = NewResolver(catalog, defaultProfiles, defaultDefaults)
	})
	return defaultResolver
}
