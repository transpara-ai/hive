// Command hive runs the product factory.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	repo := flag.String("repo", "", "Path to existing repo (targeted mode — modify existing code)")
	workdir := flag.String("workdir", "products", "Directory for generated products")
	storeDSN := flag.String("store", "", "Store connection string (postgres://... or empty for in-memory)")
	loopMode := flag.Bool("loop", false, "Use agentic loop mode (concurrent self-directing agents)")
	autoApprove := flag.Bool("yes", false, "Auto-approve all authority requests (dev/testing only)")
	skipGuardian := flag.Bool("skip-guardian", false, "Skip Guardian integrity checks after each phase (dev/testing only)")
	skipSimplify := flag.Bool("skip-simplify", false, "Skip the simplification loop after design (dev/testing only)")
	reviewerModel := flag.String("reviewer-model", "", "Override model for targeted reviews (default: claude-sonnet-4-6)")
	builderModel := flag.String("builder-model", "", "Override model for targeted builds (default: claude-sonnet-4-6)")
	ctoModel := flag.String("cto-model", "", "Override model for self-improve CTO analysis (default: claude-haiku-4-5-20251001)")
	guardianModel := flag.String("guardian-model", "", "Override model for Guardian integrity checks (default: claude-sonnet-4-6)")
	architectModel := flag.String("architect-model", "", "Override model for architect design and simplify calls (default: claude-sonnet-4-6)")
	selfImprove := flag.Bool("self-improve", false, "Self-improvement mode: analyze telemetry + codebase and apply fixes")
	flag.Parse()

	if *idea == "" && *url == "" && *spec == "" && !*selfImprove {
		return fmt.Errorf("usage: hive --human name [--store postgres://...] [--name product-name] --idea 'description' | --url 'https://...' | --spec path/to/spec.cg | --repo path --idea 'change' | --self-improve")
	}
	if *human == "" {
		return fmt.Errorf("--human is required (the name of the human operator)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	// WARNING: CLI bootstrap derives keys from display name — not safe for
	// production Postgres stores where anyone who knows the name can impersonate.
	if pool != nil {
		fmt.Fprintln(os.Stderr, "WARNING: CLI key derivation is insecure for persistent Postgres stores.")
		fmt.Fprintln(os.Stderr, "         Production should use Google auth. Proceeding for development.")
	}
	humanID, err := registerHuman(actors, *human)
	if err != nil {
		return fmt.Errorf("register human: %w", err)
	}

	// Bootstrap the event graph if it has no genesis event.
	if err := bootstrapGraph(s, humanID); err != nil {
		return fmt.Errorf("bootstrap graph: %w", err)
	}

	// Authority gate — human approves agent spawns via CLI.
	var approver authority.Approver
	if *autoApprove {
		fmt.Println("Auto-approve: ON (all authority requests auto-approved)")
		approver = func(req authority.Request) (bool, string) {
			fmt.Printf("  [auto-approved] %s\n", req.Action)
			return true, "auto-approved (--yes flag)"
		}
	} else {
		approver = cliApprover()
	}
	gate := authority.NewGate(approver)

	// Create and run pipeline — uses Claude CLI (Max plan, flat rate)
	// CTO, Architect, Reviewer, Guardian, Builder → Opus (high judgment + code gen)
	// Tester, Integrator, Researcher → Sonnet (execution)
	p, err := pipeline.New(ctx, pipeline.Config{
		Store:        s,
		Actors:       actors,
		Trust:        trustModel,
		HumanID:      humanID,
		WorkDir:      *workdir,
		Gate:         gate,
		SkipGuardian:  *skipGuardian,
		SkipSimplify:  *skipSimplify,
		AutoApprove:   *autoApprove,
		ReviewerModel: *reviewerModel,
		BuilderModel:  *builderModel,
		CTOModel:      *ctoModel,
		GuardianModel:  *guardianModel,
		ArchitectModel: *architectModel,
	})
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	input := pipeline.ProductInput{
		Name:        *name,
		URL:         *url,
		Description: *idea,
		SpecFile:    *spec,
		RepoPath:    *repo,
	}

	if *selfImprove {
		// Self-improvement mode — analyze telemetry and apply fixes
		fmt.Println("Mode: self-improve")
		if input.RepoPath == "" {
			input.RepoPath = "."
		}
		if err := p.RunSelfImprove(ctx, input); err != nil {
			return fmt.Errorf("self-improve failed: %w", err)
		}
	} else if *repo != "" {
		// Targeted mode — modify existing code
		fmt.Println("Mode: targeted (modify existing code)")
		if err := p.RunTargeted(ctx, input); err != nil {
			return fmt.Errorf("targeted pipeline failed: %w", err)
		}
	} else if *loopMode {
		// Agentic loop mode — concurrent self-directing agents
		fmt.Println("Mode: agentic loop (concurrent)")
		results, err := p.RunLoop(ctx, input, pipeline.DefaultLoopConfig())
		if err != nil {
			return fmt.Errorf("agentic loop failed: %w", err)
		}
		for _, ar := range results {
			fmt.Printf("  %s (%s): %s (%d iterations)\n", ar.Role, ar.Name, ar.Result.Reason, ar.Result.Iterations)
		}
	} else {
		// Sequential pipeline mode — fixed phase sequence
		if err := p.Run(ctx, input); err != nil {
			return fmt.Errorf("pipeline failed: %w", err)
		}
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

// bootstrapGraph emits the genesis event if the store is empty.
// Idempotent — does nothing if the graph already has events.
func bootstrapGraph(s store.Store, humanID types.ActorID) error {
	head, err := s.Head()
	if err != nil {
		return fmt.Errorf("check head: %w", err)
	}
	if head.IsSome() {
		return nil // already bootstrapped
	}

	fmt.Println("Bootstrapping event graph...")
	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)

	// Use a deterministic signer for the bootstrap event — the human's
	// derived key. This matches registerHuman's derivation.
	signer := &bootstrapSigner{humanID: humanID}
	bootstrap, err := bsFactory.Init(humanID, signer)
	if err != nil {
		return fmt.Errorf("create genesis event: %w", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		return fmt.Errorf("append genesis event: %w", err)
	}
	fmt.Println("Event graph bootstrapped.")
	return nil
}

// bootstrapSigner provides a minimal Signer for the genesis event.
type bootstrapSigner struct {
	humanID types.ActorID
}

func (b *bootstrapSigner) Sign(data []byte) (types.Signature, error) {
	// Use a deterministic key derived from the human's actor ID.
	h := sha256.Sum256([]byte("signer:" + b.humanID.Value()))
	priv := ed25519.NewKeyFromSeed(h[:])
	sig := ed25519.Sign(priv, data)
	return types.NewSignature(sig)
}

// registerHuman bootstraps a human operator in the actor store.
// This is the CLI bootstrap path — in production, humans are registered via Google auth.
// WARNING: derives key from display name — insecure for production persistent stores.
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
