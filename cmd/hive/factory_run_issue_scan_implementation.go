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

func cmdFactoryRunIssueScanImplementation(args []string) error {
	fs := flag.NewFlagSet("factory run-issue-scan-implementation", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; dispatches queued run and requires a concrete implementation task)")
	runner := fs.String("runner", "", "Executable implementation runner (required; receives JSON context on stdin and returns Operate result JSON on stdout)")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, *human, *repoPath, *repoWorkspaceRoot)
	if err != nil {
		return err
	}
	defer fc.close()

	runnerContext, err := rt.IssueScanImplementationRunnerContext(*runID)
	if err != nil {
		return fmt.Errorf("build issue-scan implementation context: %w", err)
	}
	runnerResult, err := issueScanImplementationCommandRunner(*runner, runnerArgs, *timeout)(ctx, runnerContext)
	if err != nil {
		return err
	}
	recorded, err := rt.RecordIssueScanImplementationRunnerResult(*runID, runnerResult)
	if err != nil {
		return fmt.Errorf("record issue-scan implementation result from runner: %w", err)
	}
	fmt.Printf("issue-scan implementation runner recorded branch %s commit %s with Operate artifact %s and completion %s (FactoryOrder %s)\n", recorded.OperateBranch, recorded.OperateCommit, recorded.OperateArtifactID, recorded.CompletionEventID, recorded.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after implementation runner: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this runs a configured implementation command and records a Work Operate result only; review, PR creation, PR readiness, Human approval, merge, and deploy remain separate governed steps")
	return nil
}

func issueScanImplementationCommandRunner(runner string, args []string, timeout time.Duration) hive.IssueScanImplementationRunner {
	runner = strings.TrimSpace(runner)
	copiedArgs := append([]string(nil), args...)
	return func(ctx context.Context, runnerContext hive.IssueScanImplementationRunnerContext) (hive.IssueScanImplementationRunnerResult, error) {
		return runIssueScanImplementationRunner(ctx, runner, copiedArgs, runnerContext, timeout)
	}
}

func runIssueScanImplementationRunner(ctx context.Context, runner string, args []string, runnerContext hive.IssueScanImplementationRunnerContext, timeout time.Duration) (hive.IssueScanImplementationRunnerResult, error) {
	runner = strings.TrimSpace(runner)
	if runner == "" {
		return hive.IssueScanImplementationRunnerResult{}, fmt.Errorf("runner is required")
	}
	if timeout <= 0 {
		return hive.IssueScanImplementationRunnerResult{}, fmt.Errorf("timeout must be greater than zero")
	}
	payload, err := json.MarshalIndent(runnerContext, "", "  ")
	if err != nil {
		return hive.IssueScanImplementationRunnerResult{}, fmt.Errorf("marshal implementation context: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runner, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"HIVE_ISSUE_SCAN_IMPLEMENTATION_CONTEXT=stdin",
		"HIVE_ISSUE_SCAN_RUN_ID="+runnerContext.RunID,
		"HIVE_ISSUE_SCAN_FACTORY_ORDER_ID="+runnerContext.FactoryOrderID,
		"HIVE_ISSUE_SCAN_REPOSITORY="+runnerContext.Repository,
		"HIVE_ISSUE_SCAN_IMPLEMENTATION_TASK_ID="+runnerContext.ImplementationTaskID,
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
			return hive.IssueScanImplementationRunnerResult{}, fmt.Errorf("issue-scan implementation runner timed out after %s", timeout)
		}
		return hive.IssueScanImplementationRunnerResult{}, fmt.Errorf("issue-scan implementation runner failed: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	var result hive.IssueScanImplementationRunnerResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return hive.IssueScanImplementationRunnerResult{}, fmt.Errorf("parse issue-scan implementation runner stdout as result JSON: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	return result, nil
}
