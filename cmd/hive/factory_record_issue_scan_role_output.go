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

func cmdFactoryRecordIssueScanRoleOutput(args []string) error {
	fs := flag.NewFlagSet("factory record-issue-scan-role-output", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; dispatches queued run if needed so stage tasks exist)")
	stageID := fs.String("stage", "", "Lifecycle stage id to record against (default: next incomplete stage)")
	outputFile := fs.String("output-file", "", "JSON file containing issue-scan role output evidence (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}, {"output-file", *outputFile}}); err != nil {
		return err
	}

	raw, err := os.ReadFile(*outputFile)
	if err != nil {
		return fmt.Errorf("read --output-file: %w", err)
	}
	var output hive.IssueScanStageRoleOutputEvidence
	if err := json.Unmarshal(raw, &output); err != nil {
		return fmt.Errorf("parse --output-file JSON: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	result, err := rt.RecordIssueScanStageRoleOutput(*runID, *stageID, output)
	if err != nil {
		return fmt.Errorf("record issue-scan role output: %w", err)
	}
	state := "recorded"
	if result.AlreadyRecorded {
		state = "already recorded"
	}
	fmt.Printf("issue-scan role output for run %s: %s role %s on stage %s task %s with artifact %s (FactoryOrder %s)\n", result.RunID, state, result.Role, result.StageID, result.StageTaskID, result.OutputArtifactID, result.FactoryOrderID)
	fmt.Println("boundary: this records one role output only; it is not stage completion, PR readiness, Human approval, merge, or deploy evidence")
	return nil
}
