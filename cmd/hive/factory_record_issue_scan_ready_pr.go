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

func cmdFactoryRecordIssueScanDraftPR(args []string) error {
	fs := flag.NewFlagSet("factory record-issue-scan-draft-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	receiptFile := fs.String("receipt-file", "", "Path to Transpara-AI draft PR receipt JSON (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}, {"receipt-file", *receiptFile}}); err != nil {
		return err
	}
	raw, err := os.ReadFile(*receiptFile)
	if err != nil {
		return fmt.Errorf("read receipt-file: %w", err)
	}
	var receipt hive.TransparaAIDraftPRReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return fmt.Errorf("decode receipt-file: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	result, err := rt.RecordIssueScanDraftPRReceipt(*runID, receipt)
	if err != nil {
		return fmt.Errorf("record issue-scan draft PR receipt: %w", err)
	}
	fmt.Printf("issue-scan draft PR receipt recorded=%v already_recorded=%v artifact=%s pr=%s#%d head=%s\n", result.Recorded, result.DraftPRReceiptAlreadyRecorded, result.DraftPRReceiptArtifactID, result.Repository, result.PRNumber, result.HeadSHA)
	fmt.Println("boundary: this links draft PR creation evidence only; ready-for-review, ready-state review, Human approval, merge, and deploy remain separate governed steps")
	return nil
}

func cmdFactoryRecordIssueScanReadyPR(args []string) error {
	fs := flag.NewFlagSet("factory record-issue-scan-ready-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	evidenceFile := fs.String("evidence-file", "", "Path to issue-scan ready PR evidence JSON (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}, {"evidence-file", *evidenceFile}}); err != nil {
		return err
	}
	raw, err := os.ReadFile(*evidenceFile)
	if err != nil {
		return fmt.Errorf("read evidence-file: %w", err)
	}
	var evidence hive.IssueScanReadyPREvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return fmt.Errorf("decode evidence-file: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	result, err := rt.RecordIssueScanReadyPREvidence(*runID, evidence)
	if err != nil {
		return fmt.Errorf("record issue-scan ready PR evidence: %w", err)
	}
	fmt.Printf("issue-scan ready PR evidence recorded=%v already_recorded=%v artifact=%s pr=%s#%d head=%s draft_receipt=%s\n", result.Recorded, result.ReadyPREvidenceAlreadyRecorded, result.ReadyPREvidenceArtifactID, result.Repository, result.PRNumber, result.HeadSHA, result.DraftPRReceiptRef)
	progress, err := rt.ProgressIssueScanRunLifecycle(*runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after ready PR evidence: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this records ready-for-Human PR evidence only; Human approval, merge, and deploy remain separate governed steps")
	return nil
}
