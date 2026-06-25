package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/transpara-ai/hive/pkg/hive"
)

func cmdFactoryIssueScanRunnerContexts(args []string) error {
	fs := flag.NewFlagSet("factory issue-scan-runner-contexts", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	includePayload := fs.Bool("include-payload", false, "Include ready runner JSON context payloads")
	format := fs.String("format", "json", "Output format (json)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for issue-scan targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}}); err != nil {
		return err
	}
	if *format != "json" {
		return fmt.Errorf("--format %q is not supported (want json)", *format)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	doc, err := rt.ProbeIssueScanRunnerContexts(*runID, *includePayload)
	if err != nil {
		return err
	}
	body, err := issueScanRunnerContextsJSON(doc)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(body, '\n'))
	return err
}

func issueScanRunnerContextsJSON(doc hive.IssueScanRunnerContextProbeDocument) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}
