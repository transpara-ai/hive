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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	daemon := flag.Bool("daemon", false, "Run pipeline in a loop at --interval until interrupted")
	interval := flag.Duration("interval", 30*time.Minute, "Daemon cycle interval (used with --daemon)")
	council := flag.Bool("council", false, "Convene all agents for deliberation")
	councilTopic := flag.String("topic", "", "Focus the council on a specific question")
	space := flag.String("space", "hive", "lovyou.ai space slug")
	apiBase := flag.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	budget := flag.Float64("budget", 10.0, "Daily budget in USD")
	agentID := flag.String("agent-id", "", "Agent's lovyou.ai user ID (filters task assignment)")
	oneShot := flag.Bool("one-shot", false, "Work one task then exit (for testing)")
	prMode := flag.Bool("pr", false, "Create a feature branch and open a PR instead of pushing directly to main")

	// Shared flags.
	repo := flag.String("repo", "", "Path to repo for Operate (default: current dir)")
	repos := flag.String("repos", "", "Named repos for pipeline: name=path,name=path (e.g. site=../site,hive=.)")

	// Legacy runtime mode flags.
	human := flag.String("human", "", "Human operator name (legacy runtime mode)")
	idea := flag.String("idea", "", "Seed idea for agents (legacy runtime mode)")
	storeDSN := flag.String("store", "", "Store DSN (legacy runtime mode)")
	autoApprove := flag.Bool("yes", false, "Auto-approve authority (legacy runtime mode)")
	flag.Parse()

	if *council {
		return runCouncilCmd(*space, *apiBase, *repo, *budget, *councilTopic)
	}
	if *daemon {
		repoMap := parseRepos(*repos, *repo)
		return runDaemon(*space, *apiBase, *repo, *budget, *agentID, repoMap, *interval, *prMode)
	}
	if *pipeline || *role == "pipeline" {
		repoMap := parseRepos(*repos, *repo)
		return runPipeline(*space, *apiBase, *repo, *budget, *agentID, repoMap, *prMode)
	}
	if *role != "" {
		return runRunner(*role, *space, *apiBase, *repo, *budget, *agentID, *oneShot, *prMode)
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

func runRunner(role, space, apiBase, repoPath string, budget float64, agentID string, oneShot, prMode bool) error {
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
		PRMode:     prMode,
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
func runPipeline(space, apiBase, repoPath string, budget float64, agentID string, repoMap map[string]string, prMode bool) error {
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

	// Log available repos.
	if len(repoMap) > 0 {
		var names []string
		for name := range repoMap {
			names = append(names, name)
		}
		log.Printf("[pipeline] repos available: %v (default: %s)", names, absRepo)
	}
	client := api.New(apiBase, apiKey)

	// Generate MCP config for knowledge server so agents can search.
	mcpConfigPath := writeMCPConfig(hiveDir, repoMap)
	if mcpConfigPath != "" {
		log.Printf("[pipeline] MCP knowledge server configured: %s", mcpConfigPath)
	}

	// Resolve target repo from graph or state.md.
	activeRepo := absRepo
	if len(repoMap) > 0 {
		if resolved := resolveTargetRepo(hiveDir, repoMap, client, space); resolved != "" {
			activeRepo = resolved
			log.Printf("[pipeline] target repo from directive: %s", filepath.Base(activeRepo))
		}
	}

	// Create a runner that all state machine transitions use.
	makeRunner := func(role string) (*runner.Runner, error) {
		model := runner.ModelForRole(role)
		providerCfg := intelligence.Config{
			Provider:     "claude-cli",
			Model:        model,
			MaxBudgetUSD: budget,
		}
		if mcpConfigPath != "" {
			providerCfg.MCPConfigPath = mcpConfigPath
		}
		provider, err := intelligence.New(providerCfg)
		if err != nil {
			return nil, fmt.Errorf("provider for %s: %w", role, err)
		}

		return runner.New(runner.Config{
			Role:       role,
			AgentID:    agentID,
			SpaceSlug:  space,
			RepoPath:   activeRepo,
			HiveDir:    hiveDir,
			APIClient:  client,
			Provider:   provider,
			RolePrompt: runner.LoadRolePrompt(hiveDir, role),
			BudgetUSD:  budget,
			OneShot:    true,
			NoPush:     role == "builder",
			PRMode:     role == "builder" && prMode,
			RepoMap:    repoMap,
		}), nil
	}

	// Run the pipeline as a state machine.
	// Events drive transitions. Transitions invoke agents. No for-loop.
	smRunner, err := makeRunner("builder") // base runner — state machine overrides Role per state
	if err != nil {
		return err
	}
	sm := runner.NewPipelineStateMachine(smRunner)

	if err := sm.Run(ctx); err != nil {
		log.Printf("[pipeline] state machine error: %v", err)
	}

	// Re-resolve repo in case PM changed it during the cycle.
	if len(repoMap) > 0 {
		if resolved := resolveTargetRepo(hiveDir, repoMap, client, space); resolved != "" {
			activeRepo = resolved
		}
	}

	// Push ALL repos that have uncommitted changes — not just activeRepo.
	// The Builder may have modified any repo via Operate().
	log.Printf("[pipeline] ── pushing ──")
	needsDeploy := false
	for name, repoPath := range repoMap {
		pusher := runner.New(runner.Config{RepoPath: repoPath})
		if pusher.HasChanges() {
			log.Printf("[pipeline] %s has changes — committing and pushing", name)
			commitMsg := fmt.Sprintf("[hive:pipeline] autonomous changes in %s", name)
			if err := pusher.CommitAll(commitMsg); err != nil {
				log.Printf("[pipeline] %s commit error: %v", name, err)
				continue
			}
			if err := pusher.Push(); err != nil {
				log.Printf("[pipeline] %s push error: %v", name, err)
			} else {
				log.Printf("[pipeline] %s pushed", name)
				if name == "site" {
					needsDeploy = true
				}
			}
		}
	}
	// Also push activeRepo if not in the map.
	if _, inMap := repoMap[filepath.Base(activeRepo)]; !inMap {
		pusher := runner.New(runner.Config{RepoPath: activeRepo})
		if err := pusher.Push(); err != nil {
			log.Printf("[pipeline] push error: %v", err)
		}
	}

	// Deploy if site was modified. Ship what you build.
	if needsDeploy || filepath.Base(activeRepo) == "site" {
		log.Printf("[pipeline] ── deploying site ──")
		deployCmd := exec.CommandContext(ctx, filepath.Join(os.Getenv("HOME"), ".fly", "bin", "flyctl"), "deploy", "--remote-only")
		deployCmd.Dir = activeRepo
		deployCmd.Stdout = os.Stderr
		deployCmd.Stderr = os.Stderr
		if err := deployCmd.Run(); err != nil {
			log.Printf("[pipeline] deploy error: %v", err)
		} else {
			log.Printf("[pipeline] deployed to production")
		}
	}

	log.Printf("[pipeline] ── cycle complete ──")
	return nil
}

// ─── Daemon mode ─────────────────────────────────────────────────────

const (
	daemonMaxConsecFailures = 3
	daemonBackoffInterval   = 5 * time.Minute
)

// runDaemon loops runPipeline at the given interval until SIGINT/SIGTERM.
// On pipeline failure it retries after a short backoff. After 3 consecutive
// failures it halts. Writes loop/daemon.status after each cycle.
func runDaemon(space, apiBase, repoPath string, budget float64, agentID string, repoMap map[string]string, interval time.Duration, prMode bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hiveDir := findHiveDir()
	statusPath := filepath.Join(hiveDir, "loop", "daemon.status")

	log.Printf("[daemon] starting: interval=%v space=%s", interval, space)
	cycle := 0
	consecFailures := 0

	for {
		cycle++
		start := time.Now()
		log.Printf("[daemon] ── cycle %d start ──", cycle)

		if prMode {
			daemonResetToMain(repoPath)
		}

		pipelineErr := runPipeline(space, apiBase, repoPath, budget, agentID, repoMap, prMode)

		elapsed := time.Since(start)

		var statusLine string
		if pipelineErr != nil {
			consecFailures++
			log.Printf("[daemon] ████ cycle %d FAILED (%d/%d consecutive): %v ████",
				cycle, consecFailures, daemonMaxConsecFailures, pipelineErr)
			statusLine = fmt.Sprintf("cycle=%d error: %v", cycle, pipelineErr)
			writeDaemonStatus(statusPath, statusLine)

			if consecFailures >= daemonMaxConsecFailures {
				return fmt.Errorf("[daemon] halting after %d consecutive failures — last error: %w",
					daemonMaxConsecFailures, pipelineErr)
			}

			log.Printf("[daemon] backing off %v before retry (cycle %d)", daemonBackoffInterval, cycle+1)
			select {
			case <-ctx.Done():
				log.Printf("[daemon] shutdown during backoff after cycle %d", cycle)
				return nil
			case <-time.After(daemonBackoffInterval):
			}
			continue
		}

		consecFailures = 0
		next := time.Now().Add(interval)
		log.Printf("[daemon] cycle %d complete in %v, next run at %s", cycle, elapsed.Round(time.Second), next.Format("15:04:05"))
		statusLine = fmt.Sprintf("cycle=%d ok", cycle)
		writeDaemonStatus(statusPath, statusLine)

		select {
		case <-ctx.Done():
			log.Printf("[daemon] shutdown after cycle %d", cycle)
			return nil
		case <-time.After(interval):
		}
	}
}

// daemonResetToMain fetches and resets the repo to origin/main before each
// PRMode cycle so stale feature branches don't accumulate across iterations.
func daemonResetToMain(repoPath string) {
	gitCmd := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[daemon] git %s failed: %v\n%s", args[0], err, string(out))
		}
		return err
	}

	// Log current branch for visibility.
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = repoPath
	branchOut, _ := branchCmd.Output()
	before := strings.TrimSpace(string(branchOut))
	log.Printf("[daemon] branch before reset: %s", before)

	if err := gitCmd("fetch", "origin"); err != nil {
		return
	}
	if err := gitCmd("checkout", "main"); err != nil {
		return
	}
	if err := gitCmd("pull", "origin", "main"); err != nil {
		return
	}

	log.Printf("[daemon] branch after reset: main (was: %s)", before)
}

// writeDaemonStatus writes a one-line status to the daemon status file.
func writeDaemonStatus(path, line string) {
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		log.Printf("[daemon] warning: could not write status file %s: %v", path, err)
	}
}

// writeMCPConfig generates a temporary MCP config JSON file pointing at the
// knowledge server. Returns the absolute path, or "" if it can't be created.
func writeMCPConfig(hiveDir string, repoMap map[string]string) string {
	if hiveDir == "" {
		return ""
	}

	siteDir := ""
	workspace := filepath.Dir(hiveDir)
	if p, ok := repoMap["site"]; ok {
		siteDir = p
	} else {
		siteDir = filepath.Join(workspace, "site")
	}

	// Use go run to launch the knowledge server — no pre-build needed.
	goPath := "go"
	if p, err := exec.LookPath("go.exe"); err == nil {
		goPath = p
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"knowledge": map[string]any{
				"command": goPath,
				"args":    []string{"run", "-buildvcs=false", filepath.Join(hiveDir, "cmd", "mcp-knowledge")},
				"env": map[string]string{
					"HIVE_DIR":  hiveDir,
					"SITE_DIR":  siteDir,
					"WORKSPACE": workspace,
				},
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return ""
	}

	configPath := filepath.Join(hiveDir, "loop", "mcp-knowledge.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return ""
	}

	abs, _ := filepath.Abs(configPath)
	return abs
}

// parseRepos parses a "name=path,name=path" string into a map of absolute paths.
// Falls back to a single-entry map using the --repo flag if --repos is empty.
func parseRepos(reposFlag, defaultRepo string) map[string]string {
	m := make(map[string]string)
	if reposFlag != "" {
		for _, entry := range strings.Split(reposFlag, ",") {
			parts := strings.SplitN(strings.TrimSpace(entry), "=", 2)
			if len(parts) == 2 {
				abs, err := filepath.Abs(strings.TrimSpace(parts[1]))
				if err == nil {
					m[strings.TrimSpace(parts[0])] = abs
				}
			}
		}
	}
	// Always include the default repo if not already mapped.
	if len(m) == 0 && defaultRepo != "" {
		abs, _ := filepath.Abs(defaultRepo)
		m[filepath.Base(abs)] = abs
	}
	return m
}

// resolveTargetRepo reads the current Scout directive from state.md and
// looks for "Target repo: <name>" (case-insensitive). Returns the absolute
// path if found in repoMap, or "" if not found.
// resolveTargetRepo determines which repo the pipeline should target.
// Reads from the graph first (latest milestone body), falls back to state.md.
func resolveTargetRepo(hiveDir string, repoMap map[string]string, apiClient *api.Client, spaceSlug string) string {
	if len(repoMap) == 0 {
		return ""
	}

	// Graph-first: check latest milestone on the board for "target repo:" in body or title.
	if apiClient != nil {
		tasks, err := apiClient.GetTasks(spaceSlug, "")
		if err == nil {
			for _, t := range tasks {
				if t.Kind != "task" || t.State == "done" || t.State == "closed" {
					continue
				}
				// Check title and body for repo name.
				if repo := extractRepoName(t.Title, repoMap); repo != "" {
					return repo
				}
				if repo := extractRepoName(t.Body, repoMap); repo != "" {
					return repo
				}
			}
		}
	}

	// Fallback: read state.md.
	if hiveDir != "" {
		data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md"))
		if err == nil {
			if repo := extractRepoName(string(data), repoMap); repo != "" {
				return repo
			}
		}
	}
	return ""
}

// extractRepoName finds "target repo: <name>" in text and resolves to an absolute path.
func extractRepoName(text string, repoMap map[string]string) string {
	lower := strings.ToLower(text)
	for _, prefix := range []string{"target repo:", "**target repo:**", "target repo: `", "**target repo:** `"} {
		idx := strings.Index(lower, prefix)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(text[idx+len(prefix):])
		name := strings.TrimLeft(rest, " `")
		if end := strings.IndexAny(name, " \t\n\r`"); end > 0 {
			name = name[:end]
		}
		name = strings.TrimRight(name, "`.,;:")
		name = strings.ToLower(strings.TrimSpace(name))
		if abs, ok := repoMap[name]; ok {
			return abs
		}
	}
	return ""
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
