package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
)

func cmdFactoryRunIssueScanReview(args []string) error {
	fs := flag.NewFlagSet("factory run-issue-scan-review", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; dispatches queued run if needed so stage tasks exist)")
	runner := fs.String("runner", "", "Executable adversarial review runner (required; receives JSON context on stdin and returns JSON receipt on stdout)")
	timeout := fs.Duration("timeout", 15*time.Minute, "Maximum runtime for --runner")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	runnerArgs := repeatedStringFlag{}
	fs.Var(&runnerArgs, "runner-arg", "Argument passed to --runner (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireFlags([]requiredFlag{{"human", *human}, {"run", *runID}, {"runner", *runner}}); err != nil {
		return err
	}
	if *timeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if err := requireIssueScanRunnerExecutable("--runner", *runner); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	reviewContext, err := rt.IssueScanAdversarialReviewRunContext(*runID)
	if err != nil {
		return fmt.Errorf("build issue-scan adversarial review context: %w", err)
	}
	receipt, err := issueScanReviewCommandRunner(*runner, runnerArgs, *timeout)(ctx, reviewContext)
	if err != nil {
		return err
	}
	result, err := rt.RecordIssueScanAdversarialReview(*runID, receipt)
	if err != nil {
		return fmt.Errorf("record issue-scan adversarial review: %w", err)
	}
	fmt.Printf("issue-scan adversarial review runner recorded verdict %s for head %s with receipt %s and review event %s (FactoryOrder %s)\n", result.Verdict, result.ReviewedHeadSHA, result.ReceiptArtifactID, result.ReviewEventID, result.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after review runner: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this runs a configured review command and records exact-head review evidence only; blocker repair, ready PR evidence, Human approval, merge, and deploy remain separate governed steps")
	return nil
}

func issueScanReviewCommandRunner(runner string, args []string, timeout time.Duration) hive.IssueScanAdversarialReviewRunner {
	runner = strings.TrimSpace(runner)
	copiedArgs := append([]string(nil), args...)
	return func(ctx context.Context, reviewContext hive.IssueScanAdversarialReviewContext) (hive.IssueScanAdversarialReviewReceipt, error) {
		return runIssueScanReviewRunner(ctx, runner, copiedArgs, reviewContext, timeout)
	}
}

func runIssueScanReviewRunner(ctx context.Context, runner string, args []string, reviewContext hive.IssueScanAdversarialReviewContext, timeout time.Duration) (hive.IssueScanAdversarialReviewReceipt, error) {
	runner = strings.TrimSpace(runner)
	if runner == "" {
		return hive.IssueScanAdversarialReviewReceipt{}, fmt.Errorf("runner is required")
	}
	if timeout <= 0 {
		return hive.IssueScanAdversarialReviewReceipt{}, fmt.Errorf("timeout must be greater than zero")
	}
	payload, err := json.MarshalIndent(reviewContext, "", "  ")
	if err != nil {
		return hive.IssueScanAdversarialReviewReceipt{}, fmt.Errorf("marshal review context: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runner, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"HIVE_ISSUE_SCAN_REVIEW_CONTEXT=stdin",
		"HIVE_ISSUE_SCAN_RUN_ID="+reviewContext.RunID,
		"HIVE_ISSUE_SCAN_FACTORY_ORDER_ID="+reviewContext.FactoryOrderID,
		"HIVE_ISSUE_SCAN_REPOSITORY="+reviewContext.Repository,
		"HIVE_ISSUE_SCAN_OPERATE_COMMIT="+reviewContext.OperateCommit,
	)
	if reviewContext.RepoPath != "" {
		cmd.Dir = reviewContext.RepoPath
		cmd.Env = append(cmd.Env, "HIVE_ISSUE_SCAN_REPO_PATH="+reviewContext.RepoPath)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return hive.IssueScanAdversarialReviewReceipt{}, fmt.Errorf("issue-scan adversarial review runner timed out after %s", timeout)
		}
		return hive.IssueScanAdversarialReviewReceipt{}, fmt.Errorf("issue-scan adversarial review runner failed: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	var receipt hive.IssueScanAdversarialReviewReceipt
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &receipt); err != nil {
		return hive.IssueScanAdversarialReviewReceipt{}, fmt.Errorf("parse issue-scan adversarial review runner stdout as receipt JSON: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	return receipt, nil
}

func runnerStderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return ": " + stderr
}
