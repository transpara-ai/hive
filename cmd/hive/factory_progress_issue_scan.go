package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/work"
)

func cmdFactoryProgressIssueScan(args []string) error {
	fs := flag.NewFlagSet("factory progress-issue-scan", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	runID := fs.String("run", "", "Issue-scan run ID to progress (required)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	repoWorkspaceRoot := fs.String("repo-workspace-root", "", "Path to directory containing Transpara-AI repo checkouts for implementation targets")
	runConfiguredRunners := fs.Bool("run-configured-runners", false, "Invoke configured issue-scan external runners for this run only")
	stageRoleRunner := fs.String("issue-scan-stage-role-runner", "", "Executable planning-stage issue-scan role-output runner; receives JSON context on stdin and returns role outputs JSON")
	stageRoleTimeout := fs.Duration("issue-scan-stage-role-timeout", 15*time.Minute, "Maximum runtime for --issue-scan-stage-role-runner")
	stageRoleRunnerArgs := repeatedStringFlag{}
	fs.Var(&stageRoleRunnerArgs, "issue-scan-stage-role-runner-arg", "Argument passed to --issue-scan-stage-role-runner (repeatable)")
	implementationRunner := fs.String("issue-scan-implementation-runner", "", "Executable issue-scan implementation runner; receives JSON context on stdin and returns Operate result JSON")
	implementationTimeout := fs.Duration("issue-scan-implementation-timeout", 15*time.Minute, "Maximum runtime for --issue-scan-implementation-runner")
	implementationRunnerArgs := repeatedStringFlag{}
	fs.Var(&implementationRunnerArgs, "issue-scan-implementation-runner-arg", "Argument passed to --issue-scan-implementation-runner (repeatable)")
	reviewRunner := fs.String("issue-scan-review-runner", "", "Executable exact-head issue-scan adversarial review runner; receives JSON context on stdin and returns receipt JSON")
	reviewTimeout := fs.Duration("issue-scan-review-timeout", 15*time.Minute, "Maximum runtime for --issue-scan-review-runner")
	reviewRunnerArgs := repeatedStringFlag{}
	fs.Var(&reviewRunnerArgs, "issue-scan-review-runner-arg", "Argument passed to --issue-scan-review-runner (repeatable)")
	blockerRepairRunner := fs.String("issue-scan-blocker-repair-runner", "", "Executable issue-scan blocker repair runner; receives JSON context on stdin and returns Operate result JSON")
	blockerRepairTimeout := fs.Duration("issue-scan-blocker-repair-timeout", 15*time.Minute, "Maximum runtime for --issue-scan-blocker-repair-runner")
	blockerRepairRunnerArgs := repeatedStringFlag{}
	fs.Var(&blockerRepairRunnerArgs, "issue-scan-blocker-repair-runner-arg", "Argument passed to --issue-scan-blocker-repair-runner (repeatable)")
	issueScanDraftPRRequest := fs.Bool("issue-scan-draft-pr-request", false, "Raise issue-scan draft PR authority requests after zero blockers; does not create PRs or approve requests")
	issueScanDraftPRRequestBase := fs.String("issue-scan-draft-pr-request-base", "main", "Base branch ref for --issue-scan-draft-pr-request")
	issueScanDraftPRCreate := fs.Bool("issue-scan-draft-pr-create", false, "Create approved issue-scan draft PRs after recorded Human approval; requires GITHUB_TOKEN")
	readyPRRunner := fs.String("issue-scan-ready-pr-runner", "", "Executable terminal ready-PR evidence runner; receives JSON context on stdin and returns draft receipt plus ready evidence JSON")
	readyPRMarkReady := fs.Bool("issue-scan-ready-pr-mark-ready", false, "Mark created issue-scan draft PRs ready, run exact-head ready-state review, then record ready-for-Human evidence; requires GITHUB_TOKEN")
	readyPRReviewRunner := fs.String("issue-scan-ready-pr-review-runner", "", "Executable ready-state exact-head review runner used with --issue-scan-ready-pr-mark-ready; receives JSON context on stdin and returns review receipt JSON")
	readyPRTimeout := fs.Duration("issue-scan-ready-pr-timeout", 15*time.Minute, "Maximum runtime for --issue-scan-ready-pr-runner or --issue-scan-ready-pr-review-runner")
	readyPRRunnerArgs := repeatedStringFlag{}
	readyPRReviewRunnerArgs := repeatedStringFlag{}
	fs.Var(&readyPRRunnerArgs, "issue-scan-ready-pr-runner-arg", "Argument passed to --issue-scan-ready-pr-runner (repeatable)")
	fs.Var(&readyPRReviewRunnerArgs, "issue-scan-ready-pr-review-runner-arg", "Argument passed to --issue-scan-ready-pr-review-runner (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	humanName := strings.TrimSpace(*human)
	if humanName == "" {
		return fmt.Errorf("--human is required")
	}
	if strings.TrimSpace(*runID) == "" {
		return fmt.Errorf("--run is required")
	}
	configuredFlagSet := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "issue-scan-stage-role-runner", "issue-scan-stage-role-runner-arg", "issue-scan-stage-role-timeout",
			"issue-scan-implementation-runner", "issue-scan-implementation-runner-arg", "issue-scan-implementation-timeout",
			"issue-scan-review-runner", "issue-scan-review-runner-arg", "issue-scan-review-timeout",
			"issue-scan-blocker-repair-runner", "issue-scan-blocker-repair-runner-arg", "issue-scan-blocker-repair-timeout",
			"issue-scan-draft-pr-request", "issue-scan-draft-pr-request-base", "issue-scan-draft-pr-create",
			"issue-scan-ready-pr-runner", "issue-scan-ready-pr-runner-arg",
			"issue-scan-ready-pr-mark-ready", "issue-scan-ready-pr-review-runner", "issue-scan-ready-pr-review-runner-arg", "issue-scan-ready-pr-timeout":
			configuredFlagSet = true
		}
	})
	if configuredFlagSet && !*runConfiguredRunners {
		return fmt.Errorf("--run-configured-runners is required when issue-scan runner or PR finalizer flags are set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var opts []factoryRuntimeOption
	if *runConfiguredRunners {
		if strings.TrimSpace(*stageRoleRunner) == "" {
			if len(stageRoleRunnerArgs) > 0 {
				return fmt.Errorf("--issue-scan-stage-role-runner is required when --issue-scan-stage-role-runner-arg is set")
			}
		} else {
			if *stageRoleTimeout <= 0 {
				return fmt.Errorf("--issue-scan-stage-role-timeout must be greater than zero")
			}
			if err := requireIssueScanRunnerExecutable("--issue-scan-stage-role-runner", *stageRoleRunner); err != nil {
				return err
			}
			runner := issueScanStageRoleOutputCommandRunner(*stageRoleRunner, stageRoleRunnerArgs, *stageRoleTimeout)
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanStageRoleOutputRunner = runner })
		}
		if strings.TrimSpace(*implementationRunner) == "" {
			if len(implementationRunnerArgs) > 0 {
				return fmt.Errorf("--issue-scan-implementation-runner is required when --issue-scan-implementation-runner-arg is set")
			}
		} else {
			if *implementationTimeout <= 0 {
				return fmt.Errorf("--issue-scan-implementation-timeout must be greater than zero")
			}
			if err := requireIssueScanRunnerExecutable("--issue-scan-implementation-runner", *implementationRunner); err != nil {
				return err
			}
			runner := issueScanImplementationCommandRunner(*implementationRunner, implementationRunnerArgs, *implementationTimeout)
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanImplementationRunner = runner })
		}
		if strings.TrimSpace(*reviewRunner) == "" {
			if len(reviewRunnerArgs) > 0 {
				return fmt.Errorf("--issue-scan-review-runner is required when --issue-scan-review-runner-arg is set")
			}
		} else {
			if *reviewTimeout <= 0 {
				return fmt.Errorf("--issue-scan-review-timeout must be greater than zero")
			}
			if err := requireIssueScanRunnerExecutable("--issue-scan-review-runner", *reviewRunner); err != nil {
				return err
			}
			runner := issueScanReviewCommandRunner(*reviewRunner, reviewRunnerArgs, *reviewTimeout)
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanAdversarialReviewRunner = runner })
		}
		if strings.TrimSpace(*blockerRepairRunner) == "" {
			if len(blockerRepairRunnerArgs) > 0 {
				return fmt.Errorf("--issue-scan-blocker-repair-runner is required when --issue-scan-blocker-repair-runner-arg is set")
			}
		} else {
			if *blockerRepairTimeout <= 0 {
				return fmt.Errorf("--issue-scan-blocker-repair-timeout must be greater than zero")
			}
			if err := requireIssueScanRunnerExecutable("--issue-scan-blocker-repair-runner", *blockerRepairRunner); err != nil {
				return err
			}
			runner := issueScanBlockerRepairCommandRunner(*blockerRepairRunner, blockerRepairRunnerArgs, *blockerRepairTimeout)
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanBlockerRepairRunner = runner })
		}
		issueScanDraftPRRequestBaseSet := false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "issue-scan-draft-pr-request-base" {
				issueScanDraftPRRequestBaseSet = true
			}
		})
		if issueScanDraftPRRequestBaseSet && !*issueScanDraftPRRequest {
			return fmt.Errorf("--issue-scan-draft-pr-request is required when --issue-scan-draft-pr-request-base is set")
		}
		if *issueScanDraftPRRequest {
			if strings.TrimSpace(*issueScanDraftPRRequestBase) == "" {
				return fmt.Errorf("--issue-scan-draft-pr-request-base is required when --issue-scan-draft-pr-request is enabled")
			}
			requester := issueScanDraftPRAuthorityRequestLocalGitRunner(*issueScanDraftPRRequestBase)
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanDraftPRAuthorityRequester = requester })
		}
		if *issueScanDraftPRCreate {
			token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
			if token == "" {
				return fmt.Errorf("GITHUB_TOKEN is required when --issue-scan-draft-pr-create is enabled")
			}
			creator := work.NewEpic11GitHubPullRequestCreator(token)
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanDraftPRCreator = creator })
		}
		var readyRunner hive.IssueScanReadyPRRunner
		if *readyPRMarkReady {
			if strings.TrimSpace(*readyPRRunner) != "" || len(readyPRRunnerArgs) > 0 {
				return fmt.Errorf("--issue-scan-ready-pr-mark-ready cannot be combined with --issue-scan-ready-pr-runner")
			}
			if strings.TrimSpace(*readyPRReviewRunner) == "" {
				return fmt.Errorf("--issue-scan-ready-pr-review-runner is required when --issue-scan-ready-pr-mark-ready is enabled")
			}
			if *readyPRTimeout <= 0 {
				return fmt.Errorf("--issue-scan-ready-pr-timeout must be greater than zero")
			}
			if err := requireIssueScanRunnerExecutable("--issue-scan-ready-pr-review-runner", *readyPRReviewRunner); err != nil {
				return err
			}
			token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
			if token == "" {
				return fmt.Errorf("GITHUB_TOKEN is required when --issue-scan-ready-pr-mark-ready is enabled")
			}
			readyRunner = hive.NewIssueScanReadyPRFinalizerRunner(
				newIssueScanReadyPRGitHubClient(token),
				issueScanReadyStateReviewCommandRunner(*readyPRReviewRunner, readyPRReviewRunnerArgs, *readyPRTimeout),
			)
		} else if strings.TrimSpace(*readyPRReviewRunner) != "" || len(readyPRReviewRunnerArgs) > 0 {
			return fmt.Errorf("--issue-scan-ready-pr-mark-ready is required when --issue-scan-ready-pr-review-runner is set")
		} else if strings.TrimSpace(*readyPRRunner) == "" {
			if len(readyPRRunnerArgs) > 0 {
				return fmt.Errorf("--issue-scan-ready-pr-runner is required when --issue-scan-ready-pr-runner-arg is set")
			}
		} else {
			if *readyPRTimeout <= 0 {
				return fmt.Errorf("--issue-scan-ready-pr-timeout must be greater than zero")
			}
			if err := requireIssueScanRunnerExecutable("--issue-scan-ready-pr-runner", *readyPRRunner); err != nil {
				return err
			}
			readyRunner = issueScanReadyPRCommandRunner(*readyPRRunner, readyPRRunnerArgs, *readyPRTimeout)
		}
		if readyRunner != nil {
			opts = append(opts, func(cfg *hive.Config) { cfg.IssueScanReadyPRRunner = readyRunner })
		}
	}

	rt, fc, err := openFactoryRuntime(ctx, *storeDSN, humanName, *repoPath, *repoWorkspaceRoot, opts...)
	if err != nil {
		return err
	}
	defer fc.close()

	var progress hive.IssueScanLifecycleProgress
	if *runConfiguredRunners {
		progress, err = rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(ctx, *runID)
	} else {
		progress, err = rt.ProgressIssueScanRunLifecycleContext(ctx, *runID)
	}
	if err != nil {
		return fmt.Errorf("progress issue-scan lifecycle: %w", err)
	}
	printIssueScanLifecycleProgress(progress)
	return nil
}
