// Command hive runs hive agents.
//
// Pipeline mode: runs Scout → Builder → Critic in one full cycle.
//
//	go run ./cmd/hive --pipeline --repo ../site --space hive
//
// Single role: one process per agent role, polls lovyou.ai.
//
//	go run ./cmd/hive --role builder --repo ../site --space hive
//
// Legacy mode (runtime): spawns all agents, coordinates via event graph.
//
//	go run ./cmd/hive --human Matt --idea "description"
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/api"
	"github.com/lovyou-ai/hive/pkg/hive"
	"github.com/lovyou-ai/hive/pkg/runner"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Runner mode flags.
	role := flag.String("role", "", "Agent role (builder, scout, critic, monitor). Enables runner mode.")
	pipeline := flag.Bool("pipeline", false, "Run Scout → Builder → Critic in sequence (one full cycle)")
	council := flag.Bool("council", false, "Convene all agents for deliberation")
	councilTopic := flag.String("topic", "", "Focus the council on a specific question")
	space := flag.String("space", "hive", "lovyou.ai space slug")
	apiBase := flag.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	budget := flag.Float64("budget", 10.0, "Daily budget in USD")
	agentID := flag.String("agent-id", "", "Agent's lovyou.ai user ID (filters task assignment)")
	oneShot := flag.Bool("one-shot", false, "Work one task then exit (for testing)")

	// Shared flags.
	repo := flag.String("repo", "", "Path to repo for Operate (default: current dir)")

	// Legacy runtime mode flags.
	human := flag.String("human", "", "Human operator name (legacy runtime mode)")
	idea := flag.String("idea", "", "Seed idea for agents (legacy runtime mode)")
	storeDSN := flag.String("store", "", "Store DSN (legacy runtime mode)")
	autoApprove := flag.Bool("yes", false, "Auto-approve authority (legacy runtime mode)")
	flag.Parse()

	if *council {
		return runCouncilCmd(*space, *apiBase, *repo, *budget, *councilTopic)
	}
	if *pipeline || *role == "pipeline" {
		return runPipeline(*space, *apiBase, *repo, *budget, *agentID)
	}
	if *role != "" {
		return runRunner(*role, *space, *apiBase, *repo, *budget, *agentID, *oneShot)
	}
	if *human != "" {
		return runLegacy(*human, *idea, *storeDSN, *autoApprove, *repo)
	}

	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  Pipeline:     hive --pipeline --repo ../site [--space hive] [--budget 10]")
	fmt.Fprintln(os.Stderr, "  Single role:  hive --role builder --repo ../site [--space hive] [--budget 10]")
	fmt.Fprintln(os.Stderr, "  Legacy mode:  hive --human Matt --idea 'description' [--store postgres://...]")
	return fmt.Errorf("specify --pipeline, --role, or --human")
}

// ─── Runner mode ─────────────────────────────────────────────────────

func runRunner(role, space, apiBase, repoPath string, budget float64, agentID string, oneShot bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Resolve API key.
	apiKey := os.Getenv("LOVYOU_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("LOVYOU_API_KEY required")
	}

	// Resolve repo path.
	if repoPath == "" {
		repoPath = "."
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}

	// Find hive directory (for loading role prompts).
	hiveDir := findHiveDir()

	// Create intelligence provider.
	// Role prompt is passed in the instruction (with task context), not as system prompt.
	model := runner.ModelForRole(role)
	provider, err := intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        model,
		MaxBudgetUSD: budget,
	})
	if err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	// Create API client.
	client := api.New(apiBase, apiKey)

	// Load role prompt for injection into task instructions.
	rolePrompt := runner.LoadRolePrompt(hiveDir, role)

	// Create and run the runner.
	r := runner.New(runner.Config{
		Role:       role,
		AgentID:    agentID,
		SpaceSlug:  space,
		RepoPath:   absRepo,
		HiveDir:    hiveDir,
		APIClient:  client,
		Provider:   provider,
		RolePrompt: rolePrompt,
		BudgetUSD:  budget,
		OneShot:    oneShot,
	})

	log.Printf("hive agent starting: role=%s model=%s space=%s repo=%s agent-id=%s one-shot=%v",
		role, model, space, absRepo, agentID, oneShot)
	return r.Run(ctx)
}

// ─── Council mode ────────────────────────────────────────────────────

func runCouncilCmd(space, apiBase, repoPath string, budget float64, topic string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiKey := os.Getenv("LOVYOU_API_KEY")
	client := api.New(apiBase, apiKey) // nil-safe if no key

	if repoPath == "" {
		repoPath = "."
	}
	absRepo, _ := filepath.Abs(repoPath)
	hiveDir := findHiveDir()

	// Use the best model available for council deliberation.
	councilModel := os.Getenv("COUNCIL_MODEL")
	if councilModel == "" {
		councilModel = "sonnet"
	}
	provider, err := intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        councilModel,
		MaxBudgetUSD: budget,
	})
	if err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	return runner.RunCouncil(ctx, runner.Config{
		SpaceSlug:    space,
		RepoPath:     absRepo,
		HiveDir:      hiveDir,
		APIClient:    client,
		Provider:     provider,
		BudgetUSD:    budget,
		CouncilTopic: topic,
	})
}

// ─── Pipeline mode ───────────────────────────────────────────────────

// runPipeline runs Scout → Builder → Critic in sequence. One full cycle.
func runPipeline(space, apiBase, repoPath string, budget float64, agentID string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiKey := os.Getenv("LOVYOU_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("LOVYOU_API_KEY required")
	}

	if repoPath == "" {
		repoPath = "."
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}

	hiveDir := findHiveDir()
	client := api.New(apiBase, apiKey)

	// Smart pipeline: assess the board, then run the right sequence.
	//
	// 1. Check for fix tasks → Builder fixes, Critic reviews
	// 2. Check for assigned tasks → Scout skip, Builder works, Critic reviews
	// 3. No work at all → PM decides next direction, then Scout → Builder → Critic
	//
	// This ensures: fixes first, then existing work, then new direction.

	hasFixes := false
	hasWork := false
	tasks, err := client.GetTasks(space, "")
	if err == nil {
		for _, t := range tasks {
			if t.Kind == "task" && t.State != "done" && t.State != "closed" && t.AssigneeID == agentID {
				if strings.HasPrefix(t.Title, "Fix:") {
					hasFixes = true
				} else {
					hasWork = true
				}
			}
		}
	}

	var roles []string
	if hasFixes {
		log.Printf("[pipeline] fix tasks found — Builder fixes, Critic reviews, Reflector records")
		roles = []string{"builder", "critic", "reflector"}
	} else if hasWork {
		log.Printf("[pipeline] assigned tasks found — Builder works, Critic reviews, Reflector records")
		roles = []string{"builder", "critic", "reflector"}
	} else {
		// Full pipeline: PM directs → Scout reports → Architect plans+creates tasks → Builder implements → Critic reviews
		// Scout writes a gap report (not tasks). PM reads backlog + Scout report and directs Architect.
		// Architect reads the direction and creates right-sized tasks. Builder works them.
		log.Printf("[pipeline] no work — full pipeline: PM → Scout → Architect → Builder → Critic → Reflector")
		roles = []string{"pm", "scout", "architect", "builder", "critic", "reflector"}
	}

	for _, role := range roles {
		select {
		case <-ctx.Done():
			log.Printf("[pipeline] interrupted during %s", role)
			return nil
		default:
		}

		log.Printf("[pipeline] ── %s ──", role)

		model := runner.ModelForRole(role)
		provider, err := intelligence.New(intelligence.Config{
			Provider:     "claude-cli",
			Model:        model,
			MaxBudgetUSD: budget,
		})
		if err != nil {
			return fmt.Errorf("provider for %s: %w", role, err)
		}

		r := runner.New(runner.Config{
			Role:       role,
			AgentID:    agentID,
			SpaceSlug:  space,
			RepoPath:   absRepo,
			HiveDir:    hiveDir,
			APIClient:  client,
			Provider:   provider,
			RolePrompt: runner.LoadRolePrompt(hiveDir, role),
			BudgetUSD:  budget,
			OneShot:    true,
			NoPush:     role == "builder",
		})

		if err := r.Run(ctx); err != nil {
			log.Printf("[pipeline] %s error: %v", role, err)
		}
	}

	// Push after the full cycle. Code stays local until Critic has reviewed.
	log.Printf("[pipeline] ── pushing ──")
	pusher := runner.New(runner.Config{RepoPath: absRepo})
	if err := pusher.Push(); err != nil {
		log.Printf("[pipeline] push error: %v", err)
	} else {
		log.Printf("[pipeline] pushed to remote")
	}

	log.Printf("[pipeline] ── cycle complete ──")
	return nil
}

// findHiveDir returns the hive repo directory by walking up from cwd.
func findHiveDir() string {
	// Try cwd first.
	cwd, _ := os.Getwd()
	if _, err := os.Stat(filepath.Join(cwd, "agents")); err == nil {
		return cwd
	}
	// Try parent (if running from cmd/hive).
	parent := filepath.Dir(filepath.Dir(cwd))
	if _, err := os.Stat(filepath.Join(parent, "agents")); err == nil {
		return parent
	}
	return cwd
}

// ─── Legacy runtime mode ────────────────────────────────────────────

func runLegacy(humanName, idea, dsn string, autoApprove bool, repoPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	var pool *pgxpool.Pool
	if dsn != "" {
		fmt.Fprintf(os.Stderr, "Postgres: %s\n", dsn)
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
	defer s.Close()

	actors, err := openActorStore(ctx, pool)
	if err != nil {
		return fmt.Errorf("actor store: %w", err)
	}

	if pool != nil {
		fmt.Fprintln(os.Stderr, "WARNING: CLI key derivation is insecure for persistent Postgres stores.")
	}
	humanID, err := registerHuman(actors, humanName)
	if err != nil {
		return fmt.Errorf("register human: %w", err)
	}

	if err := bootstrapGraph(s, humanID); err != nil {
		return fmt.Errorf("bootstrap graph: %w", err)
	}

	if repoPath == "" {
		repoPath = "."
	}

	rt, err := hive.New(ctx, hive.Config{
		Store:       s,
		Actors:      actors,
		HumanID:     humanID,
		AutoApprove: autoApprove,
		RepoPath:    repoPath,
	})
	if err != nil {
		return fmt.Errorf("runtime: %w", err)
	}

	for _, def := range hive.StarterAgents(humanName) {
		if err := rt.Register(def); err != nil {
			return fmt.Errorf("register %s: %w", def.Name, err)
		}
	}

	if err := rt.Run(ctx, idea); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	count, _ := s.Count()
	fmt.Fprintf(os.Stderr, "Events recorded: %d\n", count)
	return nil
}

// ─── Store helpers ───────────────────────────────────────────────────

func openStore(ctx context.Context, pool *pgxpool.Pool) (store.Store, error) {
	if pool == nil {
		fmt.Fprintln(os.Stderr, "Store: in-memory")
		return store.NewInMemoryStore(), nil
	}
	fmt.Fprintln(os.Stderr, "Store: postgres")
	return pgstore.NewPostgresStoreFromPool(ctx, pool)
}

func openActorStore(ctx context.Context, pool *pgxpool.Pool) (actor.IActorStore, error) {
	if pool == nil {
		fmt.Fprintln(os.Stderr, "Actor store: in-memory")
		return actor.NewInMemoryActorStore(), nil
	}
	fmt.Fprintln(os.Stderr, "Actor store: postgres")
	return pgactor.NewPostgresActorStoreFromPool(ctx, pool)
}

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

func bootstrapGraph(s store.Store, humanID types.ActorID) error {
	head, err := s.Head()
	if err != nil {
		return fmt.Errorf("check head: %w", err)
	}
	if head.IsSome() {
		return nil
	}

	fmt.Fprintln(os.Stderr, "Bootstrapping event graph...")
	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)

	signer := &bootstrapSigner{humanID: humanID}
	bootstrap, err := bsFactory.Init(humanID, signer)
	if err != nil {
		return fmt.Errorf("create genesis event: %w", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		return fmt.Errorf("append genesis event: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Event graph bootstrapped.")
	return nil
}

type bootstrapSigner struct {
	humanID types.ActorID
}

func (b *bootstrapSigner) Sign(data []byte) (types.Signature, error) {
	h := sha256.Sum256([]byte("signer:" + b.humanID.Value()))
	priv := ed25519.NewKeyFromSeed(h[:])
	sig := ed25519.Sign(priv, data)
	return types.NewSignature(sig)
}
