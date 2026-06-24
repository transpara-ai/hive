package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/transpara-ai/hive/pkg/hive"
)

func cmdFactoryRequestIssueScanPR(args []string) error {
	fs := flag.NewFlagSet("factory request-issue-scan-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Guardian/operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	baseRef := fs.String("base", "main", "Base branch ref")
	baseSHA := fs.String("base-sha", "", "Base commit SHA (required)")
	nonce := fs.String("nonce", "", "Single-use authority nonce (required)")
	bodyOut := fs.String("body-out", "", "Optional path to write the generated PR body for later create-pr")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{
		{"human", *human}, {"run", *runID}, {"base-sha", *baseSHA}, {"nonce", *nonce},
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

	result, err := rt.RaiseIssueScanDraftPRAuthorityRequest(*runID, *baseRef, *baseSHA, *nonce)
	if err != nil {
		return fmt.Errorf("request issue-scan draft PR authority: %w", err)
	}
	if *bodyOut != "" {
		if err := os.WriteFile(*bodyOut, []byte(result.DraftPRBody), 0o600); err != nil {
			return fmt.Errorf("write --body-out: %w", err)
		}
	}
	printIssueScanDraftPRAuthorityRequest(result, *bodyOut)
	return nil
}

func printIssueScanDraftPRAuthorityRequest(result hive.IssueScanDraftPRAuthorityRequestResult, bodyOut string) {
	state := "raised"
	switch {
	case result.AlreadyRaised:
		state = "already-raised"
	case result.HeldPendingApproval:
		state = "held-pending-approval"
	case result.AutoApproved:
		state = "auto-approved"
	}
	fmt.Printf("issue-scan draft PR authority request %s: %s\n", state, result.RequestID)
	fmt.Printf("title: %s\n", result.DraftPRTitle)
	fmt.Printf("target: repo=%s base=%s@%s head=%s@%s\n", result.DraftPRTarget.Repository, result.DraftPRTarget.BaseRef, result.DraftPRTarget.BaseSHA, result.DraftPRTarget.HeadRef, result.DraftPRTarget.HeadSHA)
	if bodyOut != "" {
		fmt.Printf("body-file: %s\n", bodyOut)
	}
	fmt.Println("boundary: this raises approval for draft PR creation only; ready state, Human approval, merge, and deploy remain separate governed steps")
}
