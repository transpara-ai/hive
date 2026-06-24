package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/transpara-ai/work"
)

func cmdFactoryCreateIssueScanDraftPR(args []string) error {
	fs := flag.NewFlagSet("factory create-issue-scan-draft-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	requestID := fs.String("request", "", "Approved draft-PR authority request id (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{
		{"human", *human}, {"run", *runID}, {"request", *requestID},
	}); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	client := work.NewEpic11GitHubPullRequestCreator(os.Getenv("GITHUB_TOKEN"))
	result, err := rt.CreateIssueScanDraftPRFromApprovedRequest(ctx, *runID, *requestID, client)
	if err != nil {
		return fmt.Errorf("create issue-scan draft PR from approved request: %w", err)
	}
	fmt.Printf("created issue-scan draft PR %s#%d at head %s with receipt %s (FactoryOrder %s)\n", result.Repository, result.PRNumber, result.HeadSHA, result.DraftPRReceipt.DraftPRReceiptArtifactID, result.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after draft PR creation: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this creates and records a draft PR receipt only; ready-for-review, ready-state review, Human approval, merge, and deploy remain separate governed steps")
	return nil
}
