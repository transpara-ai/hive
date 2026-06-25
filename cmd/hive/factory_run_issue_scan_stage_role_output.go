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

func cmdFactoryRunIssueScanStageRoleOutput(args []string) error {
	fs := flag.NewFlagSet("factory run-issue-scan-stage-role-output", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan queued run id (required; dispatches queued run and releases the current planning stage if needed)")
	runner := fs.String("runner", "", "Executable planning-stage role-output runner (required; receives JSON context on stdin and returns role outputs JSON on stdout)")
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

	runnerContext, err := rt.IssueScanStageRoleOutputRunnerContext(*runID)
	if err != nil {
		return fmt.Errorf("build issue-scan stage role-output context: %w", err)
	}
	runnerResult, err := issueScanStageRoleOutputCommandRunner(*runner, runnerArgs, *timeout)(ctx, runnerContext)
	if err != nil {
		return err
	}
	if err := requireIssueScanStageRoleOutputRunnerOutputs(runnerContext.StageID, runnerResult.RoleOutputs); err != nil {
		return err
	}
	recorded := []hive.IssueScanStageRoleOutputResult{}
	for _, output := range runnerResult.RoleOutputs {
		result, err := rt.RecordIssueScanStageRoleOutput(*runID, runnerContext.StageID, output)
		if err != nil {
			return fmt.Errorf("record issue-scan stage role output from runner: %w", err)
		}
		recorded = append(recorded, result)
	}
	fmt.Printf("issue-scan stage role-output runner recorded %d role outputs for stage %s (FactoryOrder %s)\n", countRecordedIssueScanRoleOutputsForCLI(recorded), runnerContext.StageID, runnerContext.FactoryOrderID)
	progress, err := rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle after stage role-output runner: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	fmt.Println("boundary: this runs a configured planning-stage role-output command and records role evidence only; stage completion, implementation, PR readiness, Human approval, merge, and deploy remain separate governed steps")
	return nil
}

func requireIssueScanStageRoleOutputRunnerOutputs(stageID string, outputs []hive.IssueScanStageRoleOutputEvidence) error {
	if len(outputs) == 0 {
		return fmt.Errorf("issue-scan stage role-output runner returned no role outputs for stage %q", strings.TrimSpace(stageID))
	}
	return nil
}

func issueScanStageRoleOutputCommandRunner(runner string, args []string, timeout time.Duration) hive.IssueScanStageRoleOutputRunner {
	runner = strings.TrimSpace(runner)
	copiedArgs := append([]string(nil), args...)
	return func(ctx context.Context, runnerContext hive.IssueScanStageRoleOutputRunnerContext) (hive.IssueScanStageRoleOutputRunnerResult, error) {
		return runIssueScanStageRoleOutputRunner(ctx, runner, copiedArgs, runnerContext, timeout)
	}
}

func runIssueScanStageRoleOutputRunner(ctx context.Context, runner string, args []string, runnerContext hive.IssueScanStageRoleOutputRunnerContext, timeout time.Duration) (hive.IssueScanStageRoleOutputRunnerResult, error) {
	runner = strings.TrimSpace(runner)
	if runner == "" {
		return hive.IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("runner is required")
	}
	if timeout <= 0 {
		return hive.IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("timeout must be greater than zero")
	}
	payload, err := json.MarshalIndent(runnerContext, "", "  ")
	if err != nil {
		return hive.IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("marshal stage role-output context: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, runner, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"HIVE_ISSUE_SCAN_STAGE_ROLE_OUTPUT_CONTEXT=stdin",
		"HIVE_ISSUE_SCAN_RUN_ID="+runnerContext.RunID,
		"HIVE_ISSUE_SCAN_FACTORY_ORDER_ID="+runnerContext.FactoryOrderID,
		"HIVE_ISSUE_SCAN_REPOSITORY="+runnerContext.Repository,
		"HIVE_ISSUE_SCAN_STAGE_ID="+runnerContext.StageID,
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
			return hive.IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("issue-scan stage role-output runner timed out after %s", timeout)
		}
		return hive.IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("issue-scan stage role-output runner failed: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	var result hive.IssueScanStageRoleOutputRunnerResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return hive.IssueScanStageRoleOutputRunnerResult{}, fmt.Errorf("parse issue-scan stage role-output runner stdout as result JSON: %w%s", err, runnerStderrSuffix(stderr.String()))
	}
	return result, nil
}
