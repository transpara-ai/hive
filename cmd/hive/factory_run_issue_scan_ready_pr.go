package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
)

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
