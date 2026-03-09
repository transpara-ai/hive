// Command hive runs the product factory.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"

	"github.com/lovyou-ai/hive/pkg/pipeline"
)

func main() {
	name := flag.String("name", "", "Product name (used for repo and directory)")
	idea := flag.String("idea", "", "Product idea (natural language description)")
	url := flag.String("url", "", "URL to research for product idea")
	spec := flag.String("spec", "", "Path to Code Graph spec file")
	model := flag.String("model", "claude-sonnet-4-6", "Model to use for inference")
	workdir := flag.String("workdir", "products", "Directory for generated products")
	flag.Parse()

	if *idea == "" && *url == "" && *spec == "" {
		fmt.Fprintln(os.Stderr, "Usage: hive --idea 'description' | --url 'https://...' | --spec path/to/spec.cg")
		os.Exit(1)
	}

	ctx := context.Background()

	// Create intelligence provider
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}

	provider, err := intelligence.New(intelligence.Config{
		Provider: "anthropic",
		Model:    *model,
		APIKey:   apiKey,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider: %v\n", err)
		os.Exit(1)
	}

	// Create shared event graph
	s := store.NewInMemoryStore()

	// Create and run pipeline
	p, err := pipeline.New(ctx, pipeline.Config{
		Store:    s,
		Provider: provider,
		WorkDir:  *workdir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline: %v\n", err)
		os.Exit(1)
	}

	input := pipeline.ProductInput{
		Name:        *name,
		URL:         *url,
		Description: *idea,
		SpecFile:    *spec,
	}

	// Run Guardian watch in background
	go func() {
		if err := p.GuardianWatch(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "guardian: %v\n", err)
		}
	}()

	// Run the pipeline
	if err := p.Run(ctx, input); err != nil {
		fmt.Fprintf(os.Stderr, "pipeline failed: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	count, _ := s.Count()
	fmt.Printf("\nEvents recorded: %d\n", count)
	fmt.Printf("Agents active: %d\n", len(p.Agents()))
}
