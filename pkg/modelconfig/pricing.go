package modelconfig

import (
	"fmt"
	"strings"
)

// ModelPricing holds static per-million-token costs.
// No network calls — all values embedded or loaded from YAML.
type ModelPricing struct {
	InputPerMillion      float64 `yaml:"input_per_million"`
	OutputPerMillion     float64 `yaml:"output_per_million"`
	CacheReadPerMillion  float64 `yaml:"cache_read_per_million,omitempty"`
	CacheWritePerMillion float64 `yaml:"cache_write_per_million,omitempty"`
}

// EstimateCost computes projected cost in USD for given token counts.
func (p ModelPricing) EstimateCost(inputTokens, outputTokens int) float64 {
	input := float64(inputTokens) / 1_000_000 * p.InputPerMillion
	output := float64(outputTokens) / 1_000_000 * p.OutputPerMillion
	return input + output
}

// CostEstimate projects the cost of a single call.
type CostEstimate struct {
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// AgentCostSummary describes the per-iteration cost estimate for an agent.
type AgentCostSummary struct {
	Agent         string  `json:"agent"`
	Model         string  `json:"model"`
	Tier          string  `json:"tier"`
	CostPerCallUSD float64 `json:"cost_per_call_usd"`
}

// EstimateAgentCosts produces cost summaries for a set of agent roles, using
// the resolver to look up each agent's model and the catalog for pricing.
// avgInputTokens and avgOutputTokens are the assumed per-call token counts.
func EstimateAgentCosts(resolver *Resolver, roles []string, avgInputTokens, avgOutputTokens int) []AgentCostSummary {
	var results []AgentCostSummary
	for _, role := range roles {
		rc, err := resolver.Resolve(ResolutionInput{Role: role})
		if err != nil {
			continue
		}
		cost := rc.Entry.Pricing.EstimateCost(avgInputTokens, avgOutputTokens)
		results = append(results, AgentCostSummary{
			Agent:          role,
			Model:          rc.Model,
			Tier:           string(rc.Entry.Tier),
			CostPerCallUSD: cost,
		})
	}
	return results
}

// FormatCostSummary produces a human-readable cost table for inclusion in
// the allocator's observation.
func FormatCostSummary(summaries []AgentCostSummary) string {
	if len(summaries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nMODEL COSTS (estimated per call, 10k input + 2k output tokens):\n")
	for _, s := range summaries {
		b.WriteString(fmt.Sprintf("  %-14s %-24s tier=%-10s $%.6f/call\n",
			s.Agent+":", s.Model, s.Tier, s.CostPerCallUSD))
	}
	return b.String()
}
