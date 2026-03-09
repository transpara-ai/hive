// Command hive runs the product factory.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/statestore"
	"github.com/lovyou-ai/eventgraph/go/pkg/statestore/pgstate"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/pipeline"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "", "Product name (derived by CTO if not provided)")
	human := flag.String("human", "", "Human operator name (required)")
	idea := flag.String("idea", "", "Product idea (natural language description)")
	url := flag.String("url", "", "URL to research for product idea")
	spec := flag.String("spec", "", "Path to Code Graph spec file")
	workdir := flag.String("workdir", "products", "Directory for generated products")
	storeDSN := flag.String("store", "", "Store connection string (postgres://... or empty for in-memory)")
	flag.Parse()

	if *idea == "" && *url == "" && *spec == "" {
		return fmt.Errorf("usage: hive --human name [--store postgres://...] [--name product-name] --idea 'description' | --url 'https://...' | --spec path/to/spec.cg")
	}
	if *human == "" {
		return fmt.Errorf("--human is required (the name of the human operator)")
	}

	ctx := context.Background()

	// Resolve store DSN: flag > DATABASE_URL env > in-memory
	dsn := *storeDSN
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	// Open shared pool for Postgres, or nil for in-memory.
	var pool *pgxpool.Pool
	if dsn != "" {
		fmt.Printf("Postgres: %s\n", dsn)
		var err error
		pool, err = pgxpool.New(ctx, dsn)
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		defer pool.Close()
	}

	s, err := openStore(ctx, pool)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "store close: %v\n", err)
		}
	}()

	// Actor store — Postgres when pool provided, in-memory otherwise.
	actors, err := openActorStore(ctx, pool)
	if err != nil {
		return fmt.Errorf("actor store: %w", err)
	}

	// State store — durable KV for primitive state, trust, agent memory.
	states, err := openStateStore(ctx, pool)
	if err != nil {
		return fmt.Errorf("state store: %w", err)
	}

	// Trust model — load persisted state if available.
	// Only save trust on exit if load succeeded, to avoid overwriting real data
	// with an empty model after a transient DB error.
	trustModel := trust.NewDefaultTrustModel()
	trustLoaded := true
	if err := loadTrust(trustModel, states); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load trust state: %v\n", err)
		trustLoaded = false
	}

	// Bootstrap: register the human operator in the actor store.
	// In production this happens via Google auth — this is the CLI bootstrap path.
	humanID, err := registerHuman(actors, *human)
	if err != nil {
		return fmt.Errorf("register human: %w", err)
	}

	// Authority gate — human approves agent spawns via CLI.
	gate := authority.NewGate(cliApprover())

	// Create and run pipeline — uses Claude CLI (Max plan, flat rate)
	// CTO, Architect, Reviewer, Guardian → Opus (high judgment)
	// Builder, Tester, Integrator, Researcher → Sonnet (execution)
	p, err := pipeline.New(ctx, pipeline.Config{
		Store:   s,
		Actors:  actors,
		HumanID: humanID,
		WorkDir: *workdir,
		Gate:    gate,
	})
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	// Run the pipeline — Guardian checks run automatically after each phase
	if err := p.Run(ctx, pipeline.ProductInput{
		Name:        *name,
		URL:         *url,
		Description: *idea,
		SpecFile:    *spec,
	}); err != nil {
		return fmt.Errorf("pipeline failed: %w", err)
	}

	// Persist trust state for next run (only if load succeeded — avoids
	// overwriting real data with empty model after a transient DB error).
	if trustLoaded {
		if err := saveTrust(trustModel, states); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save trust state: %v\n", err)
		}
	}

	// Print summary
	count, _ := s.Count()
	fmt.Printf("\nEvents recorded: %d\n", count)
	fmt.Printf("Agents active: %d\n", len(p.Agents()))
	return nil
}

// openStore creates a Store from a shared pool.
// nil pool → in-memory. Non-nil → PostgresStore (shared pool).
func openStore(ctx context.Context, pool *pgxpool.Pool) (store.Store, error) {
	if pool == nil {
		fmt.Println("Store: in-memory")
		return store.NewInMemoryStore(), nil
	}
	fmt.Println("Store: postgres")
	return pgstore.NewPostgresStoreFromPool(ctx, pool)
}

// openActorStore creates an IActorStore from a shared pool.
// nil pool → in-memory. Non-nil → PostgresActorStore (shared pool).
func openActorStore(ctx context.Context, pool *pgxpool.Pool) (actor.IActorStore, error) {
	if pool == nil {
		fmt.Println("Actor store: in-memory")
		return actor.NewInMemoryActorStore(), nil
	}
	fmt.Println("Actor store: postgres")
	return pgactor.NewPostgresActorStoreFromPool(ctx, pool)
}

// openStateStore creates an IStateStore from a shared pool.
// nil pool → in-memory. Non-nil → PostgresStateStore (shared pool).
func openStateStore(ctx context.Context, pool *pgxpool.Pool) (statestore.IStateStore, error) {
	if pool == nil {
		fmt.Println("State store: in-memory")
		return statestore.NewInMemoryStateStore(), nil
	}
	fmt.Println("State store: postgres")
	return pgstate.NewPostgresStateStoreFromPool(ctx, pool)
}

const trustStateScope = "trust"
const trustStateKey = "model"

// loadTrust restores trust state from the state store.
func loadTrust(model *trust.DefaultTrustModel, states statestore.IStateStore) error {
	data, err := states.Get(trustStateScope, trustStateKey)
	if err != nil {
		return err
	}
	if data == nil {
		return nil // no persisted state yet
	}
	return model.ImportJSON(data)
}

// saveTrust persists the trust model to the state store.
func saveTrust(model *trust.DefaultTrustModel, states statestore.IStateStore) error {
	data, err := model.ExportJSON()
	if err != nil {
		return err
	}
	return states.Put(trustStateScope, trustStateKey, data)
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
