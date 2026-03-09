// Command hive runs the product factory.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/pipeline"
)

func main() {
	name := flag.String("name", "", "Product name (derived by CTO if not provided)")
	human := flag.String("human", "", "Human operator name (required)")
	idea := flag.String("idea", "", "Product idea (natural language description)")
	url := flag.String("url", "", "URL to research for product idea")
	spec := flag.String("spec", "", "Path to Code Graph spec file")
	workdir := flag.String("workdir", "products", "Directory for generated products")
	flag.Parse()

	if *idea == "" && *url == "" && *spec == "" {
		fmt.Fprintln(os.Stderr, "Usage: hive --human name [--name product-name] --idea 'description' | --url 'https://...' | --spec path/to/spec.cg")
		os.Exit(1)
	}
	if *human == "" {
		fmt.Fprintln(os.Stderr, "Error: --human is required (the name of the human operator)")
		os.Exit(1)
	}

	ctx := context.Background()

	// Create shared event graph and actor store
	s := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()

	// Bootstrap: register the human operator in the actor store.
	// In production this happens via Google auth — this is the CLI bootstrap path.
	humanID, err := registerHuman(actors, *human)
	if err != nil {
		fmt.Fprintf(os.Stderr, "register human: %v\n", err)
		os.Exit(1)
	}

	// Create and run pipeline — uses Claude CLI (Max plan, flat rate)
	// CTO, Architect, Reviewer, Guardian → Opus (high judgment)
	// Builder, Tester, Integrator, Researcher → Sonnet (execution)
	p, err := pipeline.New(ctx, pipeline.Config{
		Store:   s,
		Actors:  actors,
		HumanID: humanID,
		WorkDir: *workdir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline: %v\n", err)
		os.Exit(1)
	}

	// Run the pipeline — Guardian checks run automatically after each phase
	if err := p.Run(ctx, pipeline.ProductInput{
		Name:        *name,
		URL:         *url,
		Description: *idea,
		SpecFile:    *spec,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pipeline failed: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	count, _ := s.Count()
	fmt.Printf("\nEvents recorded: %d\n", count)
	fmt.Printf("Agents active: %d\n", len(p.Agents()))
}

// registerHuman bootstraps a human operator in the actor store.
// This is the CLI bootstrap path — in production, humans are registered via Google auth.
func registerHuman(actors actor.IActorStore, displayName string) (types.ActorID, error) {
	h := sha256.Sum256([]byte("human:" + displayName))
	priv := ed25519.NewKeyFromSeed(h[:])
	pub := priv.Public().(ed25519.PublicKey)

	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		return types.ActorID{}, fmt.Errorf("public key: %w", err)
	}

	a, err := actors.Register(pk, displayName, event.ActorTypeHuman)
	if err != nil {
		return types.ActorID{}, err
	}
	return a.ID(), nil
}
