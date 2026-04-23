package modelconfig

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
