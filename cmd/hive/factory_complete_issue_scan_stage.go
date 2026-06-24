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

func cmdFactoryCompleteIssueScanStage(args []string) error {
	fs := flag.NewFlagSet("factory complete-issue-scan-stage", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; dispatches queued run if needed so stage tasks exist)")
	stageID := fs.String("stage", "", "Lifecycle stage id to complete (default: next incomplete stage)")
	evidenceFile := fs.String("evidence-file", "", "JSON file containing issue-scan stage runtime evidence (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	advanceNext := fs.Bool("advance-next", false, "Release the next issue-scan stage after completion")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}, {"evidence-file", *evidenceFile}}); err != nil {
		return err
	}

	raw, err := os.ReadFile(*evidenceFile)
	if err != nil {
		return fmt.Errorf("read --evidence-file: %w", err)
	}
	var evidence hive.IssueScanStageRuntimeEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return fmt.Errorf("parse --evidence-file JSON: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath)
	if err != nil {
		return err
	}
	defer fc.close()

	result, err := rt.CompleteIssueScanLifecycleStage(*runID, *stageID, evidence, *advanceNext)
	if err != nil {
		return fmt.Errorf("complete issue-scan stage: %w", err)
	}
	fmt.Printf("issue-scan stage %s for run %s: completed task %s with evidence %s (FactoryOrder %s)\n", result.StageID, result.RunID, result.StageTaskID, result.EvidenceArtifactID, result.FactoryOrderID)
	if result.Advance.Released {
		fmt.Printf("stage scheduling barrier released before completion: %s\n", result.StageID)
	} else if result.Advance.AlreadyReady {
		fmt.Printf("stage was already ready before completion: %s\n", result.StageID)
	}
	if result.NextAdvance != nil {
		status := "already ready"
		if result.NextAdvance.Released {
			status = "released"
		}
		fmt.Printf("next stage %s: %s task %s\n", result.NextAdvance.StageID, status, result.NextAdvance.StageTaskID)
	} else if result.LifecycleComplete {
		fmt.Println("issue-scan lifecycle complete: no later stage remains")
	}
	fmt.Println("boundary: this command may materialize an undispatched issue-scan run so stage tasks exist; it records issue-scan stage evidence only; FactoryOrder completion, PR readiness, Human approval, merge, and deploy remain separate governed steps")
	return nil
}
