package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// errUsage marks errors that are usage messages, not real failures.
// main() prints them without an "error:" prefix.
var errUsage = errors.New("usage")

// routeAndDispatch is the entry point for the subcommand router.
// It switches on args[0] and delegates to the matching cmd* function.
// Returns an error listing the available verbs when no verb is given.
func routeAndDispatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: hive <verb> [subverb] [flags]\n\n%s", errUsage, helpText())
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "civilization":
		return cmdCivilization(rest)
	case "pipeline":
		return cmdPipeline(rest)
	case "role":
		return cmdRole(rest)
	case "ingest":
		return cmdIngest(rest)
	case "council":
		return cmdCouncil(rest)
	case "factory":
		return cmdFactory(rest)
	case "-h", "--help", "help":
		fmt.Println(helpText())
		return nil
	default:
		return fmt.Errorf("unknown verb %q\n\n%s", verb, helpText())
	}
}

// helpText returns the top-level help string. Each subcommand prints its own
// flag-level help via its FlagSet's PrintDefaults().
func helpText() string {
	var b strings.Builder
	b.WriteString("Verbs:\n")
	b.WriteString("  civilization run         Multi-agent runtime, one-shot (seed and exit at quiescence)\n")
	b.WriteString("  civilization daemon      Multi-agent runtime, long-running with governed issue-scan controls\n")
	b.WriteString("  pipeline run             Scout→Builder→Critic, one cycle\n")
	b.WriteString("  pipeline daemon          Scout→Builder→Critic, loops at --interval\n")
	b.WriteString("  role <name> run          Single agent (builder|scout|critic|monitor), one task\n")
	b.WriteString("  role <name> daemon       Single agent, continuous with throttling\n")
	b.WriteString("  ingest <file>            Post a markdown spec as a task\n")
	b.WriteString("  council [--topic ...]    Convene one deliberation\n")
	b.WriteString("  factory daemon           Always-on governing loop with governed issue-scan controls\n")
	b.WriteString("  factory order            Submit one Order into the running daemon\n")
	b.WriteString("  factory scan-issues      Scan Transpara-AI GitHub issues into a queued run\n")
	b.WriteString("  factory run-issue-scan-stage-role-output\n")
	b.WriteString("                           Run configured planning-stage role-output command\n")
	b.WriteString("  factory run-issue-scan-implementation\n")
	b.WriteString("                           Run configured implementation command\n")
	b.WriteString("  factory run-issue-scan-review\n")
	b.WriteString("                           Run configured exact-head issue-scan review command\n")
	b.WriteString("  factory run-issue-scan-blocker-repair\n")
	b.WriteString("                           Run configured request_changes blocker repair command\n")
	b.WriteString("  factory record-issue-scan-draft-pr\n")
	b.WriteString("                           Link governed draft PR receipt to issue-scan ready stage\n")
	b.WriteString("  factory record-issue-scan-ready-pr\n")
	b.WriteString("                           Record ready-for-Human PR evidence for issue-scan run\n")
	b.WriteString("  factory run-issue-scan-ready-pr\n")
	b.WriteString("                           Run configured terminal ready-PR evidence command\n")
	b.WriteString("  factory request-issue-scan-pr\n")
	b.WriteString("                           Raise issue-scan draft-PR authority request\n")
	b.WriteString("  factory create-issue-scan-draft-pr\n")
	b.WriteString("                           Create approved issue-scan draft PR and receipt\n")
	b.WriteString("  factory request-pr       Raise a draft-PR authority request (gate holds)\n")
	b.WriteString("  factory create-pr        Create the approved draft PR (gated GitHub step)\n")
	b.WriteString("\nRun 'hive <verb> --help' for verb-specific flags.\n")
	return b.String()
}

// ─── civilization ─────────────────────────────────────────────────────────────

func cmdCivilization(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: hive civilization <run|daemon> [flags]", errUsage)
	}
	subverb := args[0]
	rest := args[1:]
	switch subverb {
	case "run":
		return cmdCivilizationRun(rest)
	case "daemon":
		return cmdCivilizationDaemon(rest)
	case "-h", "--help":
		fmt.Println("usage: hive civilization <run|daemon> [flags]")
		fmt.Println("\nRun 'hive civilization run --help' or 'hive civilization daemon --help' for flags.")
		return nil
	default:
		return fmt.Errorf("unknown civilization subverb %q (want run|daemon)", subverb)
	}
}

func cmdCivilizationRun(args []string) error {
	fs := flag.NewFlagSet("civilization run", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	idea := fs.String("idea", "", "Freeform seed for the runtime")
	spec := fs.String("spec", "", "Path to markdown spec file (POSTed via hive ingest channel)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repo := fs.String("repo", "", "Path to repo for Operate (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for issue-scan implementation targets")
	catalog := fs.String("catalog", "", "Custom YAML model catalog (merged with built-in defaults)")
	approveRequests := fs.Bool("approve-requests", false, "Auto-approve authority requests")
	approveRoles := fs.Bool("approve-roles", false, "Auto-approve role proposals")
	space := fs.String("space", "hive", "transpara.ai space slug")
	apiBase := fs.String("api", "https://transpara.ai", "transpara.ai API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *human == "" {
		return fmt.Errorf("--human is required")
	}
	if *spec != "" {
		if err := runIngest(*spec, *space, *apiBase, "high"); err != nil {
			return fmt.Errorf("ingest spec: %w", err)
		}
	}
	return runLegacy(*human, *idea, *storeDSN, *approveRequests, *approveRoles, *repo, *repoWorkspaceRoot, *catalog, 0, false, nil, nil, nil, nil, nil, nil, nil, nil, *space, *apiBase)
}

func cmdCivilizationDaemon(args []string) error {
	return cmdGovernedDaemon("civilization daemon", args)
}

// ─── pipeline ─────────────────────────────────────────────────────────────────

func cmdPipeline(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: hive pipeline <run|daemon> [flags]", errUsage)
	}
	subverb := args[0]
	rest := args[1:]
	switch subverb {
	case "run":
		return cmdPipelineRun(rest)
	case "daemon":
		return cmdPipelineDaemon(rest)
	case "-h", "--help":
		fmt.Println("usage: hive pipeline <run|daemon> [flags]")
		return nil
	default:
		return fmt.Errorf("unknown pipeline subverb %q (want run|daemon)", subverb)
	}
}

func pipelineFlags(fs *flag.FlagSet) (space, apiBase, repo, agentID, storeDSN, repos *string, budget *float64, direct, prMode, worktrees, autoClone *bool) {
	space = fs.String("space", "hive", "transpara.ai space slug")
	apiBase = fs.String("api", "https://transpara.ai", "transpara.ai API base URL")
	repo = fs.String("repo", "", "Path to repo (default: current dir)")
	agentID = fs.String("agent-id", "", "Agent's transpara.ai user ID (filters task assignment)")
	storeDSN = fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repos = fs.String("repos", "", "Named repos: name=path,name=path")
	budget = fs.Float64("budget", 10.0, "Daily budget in USD")
	direct = fs.Bool("direct", false, "Use legacy direct commit/push behavior instead of local PR proposal mode")
	prMode = fs.Bool("pr", false, "Compatibility alias for legacy PR behavior when used with --direct")
	worktrees = fs.Bool("worktrees", false, "Each Builder task gets its own git worktree")
	autoClone = fs.Bool("auto-clone", false, "Clone missing repos from registry URLs before each cycle")
	return
}

func cmdPipelineRun(args []string) error {
	fs := flag.NewFlagSet("pipeline run", flag.ContinueOnError)
	space, apiBase, repo, agentID, storeDSN, repos, budget, direct, prMode, worktrees, autoClone := pipelineFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoMap := parseRepos(*repos, *repo)
	return runPipeline(*space, *apiBase, *repo, *budget, *agentID, repoMap, *direct, *prMode, *worktrees, *autoClone, *storeDSN)
}

func cmdPipelineDaemon(args []string) error {
	fs := flag.NewFlagSet("pipeline daemon", flag.ContinueOnError)
	space, apiBase, repo, agentID, storeDSN, repos, budget, direct, prMode, worktrees, autoClone := pipelineFlags(fs)
	interval := fs.Duration("interval", 30*time.Minute, "Pipeline cycle interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoMap := parseRepos(*repos, *repo)
	return runDaemon(*space, *apiBase, *repo, *budget, *agentID, repoMap, *interval, *direct, *prMode, *worktrees, *autoClone, *storeDSN)
}

// ─── role ─────────────────────────────────────────────────────────────────────

func cmdRole(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: hive role <name> <run|daemon> [flags] (role name required)", errUsage)
	}
	roleName := args[0]
	if len(args) < 2 {
		return fmt.Errorf("%w: hive role %s <run|daemon> [flags]", errUsage, roleName)
	}
	subverb := args[1]
	rest := args[2:]
	switch subverb {
	case "run":
		return cmdRoleRun(roleName, rest)
	case "daemon":
		return cmdRoleDaemon(roleName, rest)
	case "-h", "--help":
		fmt.Printf("usage: hive role %s <run|daemon> [flags]\n", roleName)
		return nil
	default:
		return fmt.Errorf("unknown role subverb %q (want run|daemon)", subverb)
	}
}

func roleFlags(fs *flag.FlagSet) (space, apiBase, repo, agentID *string, budget *float64, direct, prMode *bool) {
	space = fs.String("space", "hive", "transpara.ai space slug")
	apiBase = fs.String("api", "https://transpara.ai", "transpara.ai API base URL")
	repo = fs.String("repo", "", "Path to repo (default: current dir)")
	agentID = fs.String("agent-id", "", "Agent's transpara.ai user ID (filters task assignment)")
	budget = fs.Float64("budget", 10.0, "Daily budget in USD")
	direct = fs.Bool("direct", false, "Use legacy direct commit/push behavior instead of local PR proposal mode")
	prMode = fs.Bool("pr", false, "Compatibility alias for legacy PR behavior when used with --direct")
	return
}

func cmdRoleRun(role string, args []string) error {
	fs := flag.NewFlagSet("role "+role+" run", flag.ContinueOnError)
	space, apiBase, repo, agentID, budget, direct, prMode := roleFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runRunner(role, *space, *apiBase, *repo, *budget, *agentID, true, *direct, *prMode)
}

func cmdRoleDaemon(role string, args []string) error {
	fs := flag.NewFlagSet("role "+role+" daemon", flag.ContinueOnError)
	space, apiBase, repo, agentID, budget, direct, prMode := roleFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runRunner(role, *space, *apiBase, *repo, *budget, *agentID, false, *direct, *prMode)
}

// ─── ingest ───────────────────────────────────────────────────────────────────

func cmdIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	space := fs.String("space", "hive", "transpara.ai space slug")
	apiBase := fs.String("api", "https://transpara.ai", "transpara.ai API base URL")
	priority := fs.String("priority", "normal", "Task priority: high|normal|low")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("%w: hive ingest <file.md> [flags] (spec file required)", errUsage)
	}
	if len(rest) > 1 {
		return fmt.Errorf("ingest takes exactly one positional argument (spec file path)")
	}
	return runIngest(rest[0], *space, *apiBase, *priority)
}

// ─── council ──────────────────────────────────────────────────────────────────

func newCouncilFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("council", flag.ContinueOnError)
	fs.String("space", "hive", "transpara.ai space slug")
	fs.String("api", "https://transpara.ai", "transpara.ai API base URL")
	fs.String("repo", "", "Path to repo (default: current dir)")
	fs.Float64("budget", 10.0, "Daily budget in USD")
	fs.String("topic", "", "Focus the council on a specific question")
	fs.String("catalog", "", "Custom YAML model catalog (merged with built-in defaults)")
	return fs
}

func cmdCouncil(args []string) error {
	fs := newCouncilFlagSet()
	if err := fs.Parse(args); err != nil {
		return err
	}
	space := fs.Lookup("space").Value.String()
	apiBase := fs.Lookup("api").Value.String()
	repo := fs.Lookup("repo").Value.String()
	budget, _ := strconv.ParseFloat(fs.Lookup("budget").Value.String(), 64)
	topic := fs.Lookup("topic").Value.String()
	catalog := fs.Lookup("catalog").Value.String()
	return runCouncilCmd(space, apiBase, repo, budget, topic, catalog)
}
