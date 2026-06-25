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

func cmdFactoryRunIssueScanBlockerRepair(args []string) error {
	fs := flag.NewFlagSet("factory run-issue-scan-blocker-repair", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; requires request_changes review and reopened implementation task)")
	runner := fs.String("runner", "", "Executable blocker repair runner (required; receives JSON context on stdin and returns Operate result JSON on stdout)")
	timeout := fs.Duration("timeout", 15*time.Minute, "Maximum runtime for --runner")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for issue-scan targets")
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

	runnerContext, err := rt.IssueScanBlockerRepairRunnerContext(*runID)
	if err != nil {
		return fmt.Errorf("build issue-scan blocker repair context: %w", err)
	}
	runnerResult, err := issueScanBlockerRepairCommandRunner(*runner, runnerArgs, *timeout)(ctx, runnerContext)
	if err != nil {
		return err
	}
	recorded, err := rt.RecordIssueScanBlockerRepairRunnerResult(*runID, runnerResult)
	if err != nil {
		return fmt.Errorf("record issue-scan blocker repair result from runner: %w", err)
	}
	fmt.Printf("issue-scan blocker repair runner recorded branch %s commit %s with Operate artifact %s and completion %s (FactoryOrder %s)\n", recorded.OperateBranch, recorded.OperateCommit, recorded.OperateArtifactID, recorded.CompletionEventID, recorded.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after blocker repair runner: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this runs a configured blocker repair command and records a Work Operate result only; zero-blocker proof, review, PR readiness, Human approval, merge, and deploy remain separate governed steps")
	return nil
}

func issueScanBlockerRepairCommandRunner(runner string, args []string, timeout time.Duration) hive.IssueScanBlockerRepairRunner {
	runner = strings.TrimSpace(runner)
	copiedArgs := append([]string(nil), args...)
	return func(ctx context.Context, runnerContext hive.IssueScanBlockerRepairRunnerContext) (hive.IssueScanBlockerRepairRunnerResult, error) {
		return runIssueScanBlockerRepairRunner(ctx, runner, copiedArgs, runnerContext, timeout)
	}
}

func runIssueScanBlockerRepairRunner(ctx context.Context, runner string, args []string, runnerContext hive.IssueScanBlockerRepairRunnerContext, timeout time.Duration) (hive.IssueScanBlockerRepairRunnerResult, error) {
	runner = strings.TrimSpace(runner)
	if runner == "" {
		return hive.IssueScanBlockerRepairRunnerResult{}, fmt.Errorf("runner is required")
	}
	if timeout <= 0 {
		return hive.IssueScanBlockerRepairRunnerResult{}, fmt.Errorf("timeout must be greater than zero")
	}
	payload, err := json.MarshalIndent(runnerContext, "", "  ")
	if err != nil {
		return hive.IssueScanBlockerRepairRunnerResult{}, fmt.Errorf("marshal blocker repair context: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runner, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"HIVE_ISSUE_SCAN_BLOCKER_REPAIR_CONTEXT=stdin",
		"HIVE_ISSUE_SCAN_RUN_ID="+runnerContext.RunID,
		"HIVE_ISSUE_SCAN_FACTORY_ORDER_ID="+runnerContext.FactoryOrderID,
		"HIVE_ISSUE_SCAN_REPOSITORY="+runnerContext.Repository,
		"HIVE_ISSUE_SCAN_IMPLEMENTATION_TASK_ID="+runnerContext.ImplementationTaskID,
		"HIVE_ISSUE_SCAN_REQUEST_CHANGES_REVIEW_ID="+runnerContext.RequestChangesReviewEventID,
	)
	if runnerContext.RepoPath != "" {
		cmd.Dir = runnerContext.RepoPath
		cmd.Env = append(cmd.Env, "HIVE_ISSUE_SCAN_REPO_PATH="+runnerContext.RepoPath)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return hive.IssueScanBlockerRepairRunnerResult{}, fmt.Errorf("issue-scan blocker repair runner timed out after %s", timeout)
		}
		return hive.IssueScanBlockerRepairRunnerResult{}, fmt.Errorf("issue-scan blocker repair runner failed: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	var result hive.IssueScanBlockerRepairRunnerResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return hive.IssueScanBlockerRepairRunnerResult{}, fmt.Errorf("parse issue-scan blocker repair runner stdout as result JSON: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	return result, nil
}
