package modelconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name         string
		pricing      ModelPricing
		inputTokens  int
		outputTokens int
		wantCost     float64
	}{
		{
			name:         "1M input + 1M output at opus prices",
			pricing:      ModelPricing{InputPerMillion: 15.0, OutputPerMillion: 75.0},
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			wantCost:     90.0, // 15 + 75
		},
		{
			name:         "zero tokens returns 0",
			pricing:      ModelPricing{InputPerMillion: 15.0, OutputPerMillion: 75.0},
			inputTokens:  0,
			outputTokens: 0,
			wantCost:     0.0,
		},
		{
			name:         "realistic call: 10k input, 2k output at sonnet prices",
			pricing:      ModelPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0},
			inputTokens:  10_000,
			outputTokens: 2_000,
			wantCost:     0.06, // 0.03 + 0.03
		},
		{
			name:         "haiku pricing: 500k input, 100k output",
			pricing:      ModelPricing{InputPerMillion: 0.8, OutputPerMillion: 4.0},
			inputTokens:  500_000,
			outputTokens: 100_000,
			wantCost:     0.8, // 0.4 + 0.4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pricing.EstimateCost(tt.inputTokens, tt.outputTokens)
			assert.InDelta(t, tt.wantCost, got, 0.0001)
		})
	}
}
