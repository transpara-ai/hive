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

func cmdFactoryRunIssueScanReadyPR(args []string) error {
	fs := flag.NewFlagSet("factory run-issue-scan-ready-pr", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required)")
	runner := fs.String("runner", "", "Executable terminal ready-PR evidence runner (required; receives JSON context on stdin and returns draft receipt plus ready evidence JSON on stdout)")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	readyContext, err := rt.IssueScanReadyPRRunnerContext(*runID)
	if err != nil {
		return fmt.Errorf("build issue-scan ready PR context: %w", err)
	}
	runnerResult, err := issueScanReadyPRCommandRunner(*runner, runnerArgs, *timeout)(ctx, readyContext)
	if err != nil {
		return err
	}
	draftResult, err := rt.RecordIssueScanDraftPRReceipt(*runID, runnerResult.DraftPRReceipt)
	if err != nil {
		return fmt.Errorf("record issue-scan draft PR receipt from ready PR runner: %w", err)
	}
	readyResult, err := rt.RecordIssueScanReadyPREvidence(*runID, runnerResult.ReadyPREvidence)
	if err != nil {
		return fmt.Errorf("record issue-scan ready PR evidence from ready PR runner: %w", err)
	}
	fmt.Printf("issue-scan ready PR runner recorded pr %s#%d at head %s with draft receipt %s and ready evidence %s (FactoryOrder %s)\n", readyResult.Repository, readyResult.PRNumber, readyResult.HeadSHA, draftResult.DraftPRReceiptArtifactID, readyResult.ReadyPREvidenceArtifactID, readyResult.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycle(*runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after ready PR runner: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this runs a configured ready-PR evidence command and records PR readiness only; Human approval, merge, and deploy remain separate governed steps")
	return nil
}

func issueScanReadyPRCommandRunner(runner string, args []string, timeout time.Duration) hive.IssueScanReadyPRRunner {
	runner = strings.TrimSpace(runner)
	copiedArgs := append([]string(nil), args...)
	return func(ctx context.Context, readyContext hive.IssueScanReadyPRRunnerContext) (hive.IssueScanReadyPRRunnerResult, error) {
		return runIssueScanReadyPRRunner(ctx, runner, copiedArgs, readyContext, timeout)
	}
}

func runIssueScanReadyPRRunner(ctx context.Context, runner string, args []string, readyContext hive.IssueScanReadyPRRunnerContext, timeout time.Duration) (hive.IssueScanReadyPRRunnerResult, error) {
	runner = strings.TrimSpace(runner)
	if runner == "" {
		return hive.IssueScanReadyPRRunnerResult{}, fmt.Errorf("runner is required")
	}
	if timeout <= 0 {
		return hive.IssueScanReadyPRRunnerResult{}, fmt.Errorf("timeout must be greater than zero")
	}
	payload, err := json.MarshalIndent(readyContext, "", "  ")
	if err != nil {
		return hive.IssueScanReadyPRRunnerResult{}, fmt.Errorf("marshal ready PR context: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runner, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"HIVE_ISSUE_SCAN_READY_PR_CONTEXT=stdin",
		"HIVE_ISSUE_SCAN_RUN_ID="+readyContext.RunID,
		"HIVE_ISSUE_SCAN_FACTORY_ORDER_ID="+readyContext.FactoryOrderID,
		"HIVE_ISSUE_SCAN_REPOSITORY="+readyContext.Repository,
		"HIVE_ISSUE_SCAN_OPERATE_COMMIT="+readyContext.OperateCommit,
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return hive.IssueScanReadyPRRunnerResult{}, fmt.Errorf("issue-scan ready PR runner timed out after %s", timeout)
		}
		return hive.IssueScanReadyPRRunnerResult{}, fmt.Errorf("issue-scan ready PR runner failed: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	var result hive.IssueScanReadyPRRunnerResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return hive.IssueScanReadyPRRunnerResult{}, fmt.Errorf("parse issue-scan ready PR runner stdout as result JSON: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	return result, nil
}
