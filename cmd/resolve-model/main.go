// Command resolve-model is a debug tool for model resolution.
// It builds a ResolutionInput from flags and prints the resolved config as JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lovyou-ai/hive/pkg/modelconfig"
)

func main() {
	role := flag.String("role", "", "Agent role (required)")
	model := flag.String("model", "", "Model ID or alias override")
	provider := flag.String("provider", "", "Provider override")
	profile := flag.String("profile", "", "Named profile to apply")
	canOperate := flag.Bool("can-operate", false, "Force claude-cli provider")
	estimateCost := flag.Bool("estimate-cost", false, "Compute and print cost estimate")
	inputTokens := flag.Int("input-tokens", 0, "Input tokens for cost estimate")
	outputTokens := flag.Int("output-tokens", 0, "Output tokens for cost estimate")

	flag.Parse()

	if *role == "" {
		fmt.Fprintf(os.Stderr, "error: --role is required\n")
		flag.Usage()
		os.Exit(1)
	}

	input := modelconfig.ResolutionInput{
		Role:          *role,
		AgentDefModel: *model,
		CanOperate:    *canOperate,
	}

	if *provider != "" || *profile != "" {
		input.Policy = &modelconfig.RoleModelPolicy{
			Provider: *provider,
			Profile:  *profile,
		}
	}

	resolver := modelconfig.DefaultResolver()
	resolved, err := resolver.Resolve(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))

	if *estimateCost && (*inputTokens > 0 || *outputTokens > 0) {
		pricing := resolved.Entry.Pricing
		inputCost := float64(*inputTokens) / 1_000_000 * pricing.InputPerMillion
		outputCost := float64(*outputTokens) / 1_000_000 * pricing.OutputPerMillion
		totalCost := inputCost + outputCost
		fmt.Printf("\nCost estimate:\n")
		fmt.Printf("  Input:  %d tokens x $%.2f/M = $%.6f\n", *inputTokens, pricing.InputPerMillion, inputCost)
		fmt.Printf("  Output: %d tokens x $%.2f/M = $%.6f\n", *outputTokens, pricing.OutputPerMillion, outputCost)
		fmt.Printf("  Total:  $%.6f\n", totalCost)
	}
}
