package modelconfig

import "github.com/lovyou-ai/eventgraph/go/pkg/intelligence"

// ToIntelligenceConfig converts a ResolvedConfig to an intelligence.Config
// for use with intelligence.New().
func ToIntelligenceConfig(rc ResolvedConfig, systemPrompt string) intelligence.Config {
	return intelligence.Config{
		Provider:     rc.Provider,
		Model:        rc.Model,
		APIKey:       rc.APIKey,
		BaseURL:      rc.BaseURL,
		MaxTokens:    rc.MaxTokens,
		Temperature:  rc.Temperature,
		MaxBudgetUSD: rc.MaxBudgetUSD,
		SystemPrompt: systemPrompt,
	}
}
