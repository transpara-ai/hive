package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func cmdFactoryAdvanceIssueScan(args []string) error {
	fs := flag.NewFlagSet("factory advance-issue-scan", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	stageID := fs.String("stage", "", "Lifecycle stage id to advance (default: next incomplete stage)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}}); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath)
	if err != nil {
		return err
	}
	defer fc.close()

	result, err := rt.AdvanceIssueScanLifecycleStage(*runID, *stageID)
	if err != nil {
		return fmt.Errorf("advance issue-scan stage: %w", err)
	}
	status := "already ready"
	if result.Released {
		status = "released"
	}
	fmt.Printf("issue-scan stage %s for run %s: %s task %s (FactoryOrder %s)\n", result.StageID, result.RunID, status, result.StageTaskID, result.FactoryOrderID)
	if result.PreviousStageID != "" {
		fmt.Printf("previous stage: %s task %s\n", result.PreviousStageID, result.PreviousStageTaskID)
	}
	fmt.Printf("dispatch scanned=%d dispatched=%d already_dispatched=%d failed=%d\n", result.Dispatch.Scanned, result.Dispatch.Dispatched, result.Dispatch.AlreadyDispatched, result.Dispatch.Failed)
	return nil
}
