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

func cmdFactoryRecordIssueScanReview(args []string) error {
	fs := flag.NewFlagSet("factory record-issue-scan-review", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; dispatches queued run if needed so stage tasks exist)")
	reviewFile := fs.String("review-file", "", "JSON file containing issue-scan adversarial review receipt (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}, {"review-file", *reviewFile}}); err != nil {
		return err
	}

	raw, err := os.ReadFile(*reviewFile)
	if err != nil {
		return fmt.Errorf("read --review-file: %w", err)
	}
	var receipt hive.IssueScanAdversarialReviewReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return fmt.Errorf("parse --review-file JSON: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	result, err := rt.RecordIssueScanAdversarialReview(*runID, receipt)
	if err != nil {
		return fmt.Errorf("record issue-scan adversarial review: %w", err)
	}
	state := "recorded"
	if result.ReviewAlreadyRecorded {
		state = "already recorded"
	}
	fmt.Printf("issue-scan adversarial review for run %s: %s verdict %s for head %s with receipt %s and review event %s (FactoryOrder %s)\n", result.RunID, state, result.Verdict, result.ReviewedHeadSHA, result.ReceiptArtifactID, result.ReviewEventID, result.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after review receipt: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this records exact-head review evidence only; blocker repair, ready PR evidence, Human approval, merge, and deploy remain separate governed steps")
	return nil
}
