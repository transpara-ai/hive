package main

import (
	"flag"
	"fmt"
	"strings"
)

// routeAndDispatch is the entry point for the subcommand router.
// It switches on args[0] and delegates to the matching cmd* function.
// Returns an error listing the available verbs when no verb is given.
func routeAndDispatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hive <verb> [subverb] [flags]\n\n%s", helpText())
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
	b.WriteString("  civilization daemon      Multi-agent runtime, long-running (block on bus)\n")
	b.WriteString("  pipeline run             Scout→Builder→Critic, one cycle\n")
	b.WriteString("  pipeline daemon          Scout→Builder→Critic, loops at --interval\n")
	b.WriteString("  role <name> run          Single agent (builder|scout|critic|monitor), one task\n")
	b.WriteString("  role <name> daemon       Single agent, continuous with throttling\n")
	b.WriteString("  ingest <file>            Post a markdown spec as a task\n")
	b.WriteString("  council [--topic ...]    Convene one deliberation\n")
	b.WriteString("\nRun 'hive <verb> --help' for verb-specific flags.\n")
	return b.String()
}

// ─── civilization ─────────────────────────────────────────────────────────────

func cmdCivilization(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hive civilization <run|daemon> [flags]")
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
	approveRequests := fs.Bool("approve-requests", false, "Auto-approve authority requests")
	approveRoles := fs.Bool("approve-roles", false, "Auto-approve role proposals")
	space := fs.String("space", "hive", "lovyou.ai space slug")
	apiBase := fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
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
	return runLegacy(*human, *idea, *storeDSN, *approveRequests, *approveRoles, *repo, false, *space, *apiBase)
}

func cmdCivilizationDaemon(args []string) error {
	fs := flag.NewFlagSet("civilization daemon", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	seedSpec := fs.String("seed-spec", "", "Optional initial spec to ingest before daemon starts")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repo := fs.String("repo", "", "Path to repo for Operate (default: current dir)")
	approveRequests := fs.Bool("approve-requests", false, "Auto-approve authority requests")
	approveRoles := fs.Bool("approve-roles", false, "Auto-approve role proposals")
	space := fs.String("space", "hive", "lovyou.ai space slug")
	apiBase := fs.String("api", "https://lovyou.ai", "lovyou.ai API base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *human == "" {
		return fmt.Errorf("--human is required")
	}
	if *seedSpec != "" {
		if err := runIngest(*seedSpec, *space, *apiBase, "high"); err != nil {
			return fmt.Errorf("ingest seed-spec: %w", err)
		}
	}
	return runLegacy(*human, "", *storeDSN, *approveRequests, *approveRoles, *repo, true, *space, *apiBase)
}

// Stubs — replaced by real implementations in later tasks.
func cmdPipeline(args []string) error { return fmt.Errorf("pipeline: not implemented") }
func cmdRole(args []string) error     { return fmt.Errorf("role: not implemented") }
func cmdIngest(args []string) error   { return fmt.Errorf("ingest: not implemented") }
func cmdCouncil(args []string) error  { return fmt.Errorf("council: not implemented") }
