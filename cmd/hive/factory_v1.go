package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

func cmdFactoryV1(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: hive factory-v1 daemon [flags]", errUsage)
	}
	switch args[0] {
	case "daemon":
		return cmdFactoryV1Daemon(args[1:])
	case "-h", "--help":
		fmt.Println("usage: hive factory-v1 daemon [flags]")
		return nil
	default:
		return fmt.Errorf("unknown factory-v1 subverb %q (want daemon)", args[0])
	}
}

func cmdFactoryV1Daemon(args []string) error {
	fs := flag.NewFlagSet("factory-v1 daemon", flag.ContinueOnError)
	human := fs.String("human", "", "Human operator display name (required)")
	storeDSN := fs.String("store", "", "EventGraph/Work Postgres DSN; defaults to DATABASE_URL")
	repositoryWorkspace := fs.String("repo-workspace-root", envOrDefault("FACTORY_V1_REPO_WORKSPACE_ROOT", ""), "Absolute root containing target repository checkouts (required)")
	pollInterval := fs.Duration("poll-interval", 2*time.Second, "Durable normalization/scheduler poll interval")
	runnerTimeout := fs.Duration("runner-timeout", 15*time.Minute, "Per-stage external runner timeout")
	runnerOutputLimit := fs.Int("runner-output-limit", 2*1024*1024, "Maximum strict JSON stdout/stderr bytes per runner invocation")

	authorRunner := fs.String("author-runner", os.Getenv("FACTORY_V1_AUTHOR_RUNNER"), "Exact author runner executable (required)")
	authorRunnerSHA := fs.String("author-runner-sha256", os.Getenv("FACTORY_V1_AUTHOR_RUNNER_SHA256"), "Pinned author runner executable SHA-256 (required)")
	authorProviderID := fs.String("author-provider-id", envOrDefault("FACTORY_V1_AUTHOR_PROVIDER_ID", "codex-cli"), "Author provider identity")
	authorFamily := fs.String("author-family", envOrDefault("FACTORY_V1_AUTHOR_FAMILY", "OpenAI/Codex"), "Author provider family")
	authorModel := fs.String("author-model", os.Getenv("FACTORY_V1_AUTHOR_MODEL"), "Pinned author model identity (required)")
	authorCredentialSource := fs.String("author-credential-source-id", os.Getenv("FACTORY_V1_AUTHOR_CREDENTIAL_SOURCE_ID"), "Author credential-source identity, never the credential value")
	authorArgs := repeatedStringFlag{}
	authorEnvironment := repeatedStringFlag{}
	fs.Var(&authorArgs, "author-runner-arg", "Fixed author runner argument (repeatable)")
	fs.Var(&authorEnvironment, "author-env-key", "Additional author runner environment key name; values are copied only at process start (repeatable)")

	reviewerRunner := fs.String("reviewer-runner", os.Getenv("FACTORY_V1_REVIEWER_RUNNER"), "Exact independent reviewer runner executable (required)")
	reviewerRunnerSHA := fs.String("reviewer-runner-sha256", os.Getenv("FACTORY_V1_REVIEWER_RUNNER_SHA256"), "Pinned reviewer runner executable SHA-256 (required)")
	reviewerProviderID := fs.String("reviewer-provider-id", envOrDefault("FACTORY_V1_REVIEWER_PROVIDER_ID", "claude-cli"), "Independent reviewer provider identity")
	reviewerFamily := fs.String("reviewer-family", envOrDefault("FACTORY_V1_REVIEWER_FAMILY", "Anthropic/Claude"), "Independent reviewer family")
	reviewerModel := fs.String("reviewer-model", os.Getenv("FACTORY_V1_REVIEWER_MODEL"), "Pinned independent reviewer model identity (required)")
	reviewerCredentialSource := fs.String("reviewer-credential-source-id", os.Getenv("FACTORY_V1_REVIEWER_CREDENTIAL_SOURCE_ID"), "Reviewer credential-source identity, never the credential value")
	reviewerArgs := repeatedStringFlag{}
	reviewerEnvironment := repeatedStringFlag{}
	fs.Var(&reviewerArgs, "reviewer-runner-arg", "Fixed reviewer runner argument (repeatable)")
	fs.Var(&reviewerEnvironment, "reviewer-env-key", "Additional reviewer runner environment key name; values are copied only at process start (repeatable)")
	standingApprovalFile := fs.String("standing-approval", os.Getenv("FACTORY_V1_STANDING_APPROVAL_FILE"), "Optional strict JSON standing-approval binding file")

	issueScanInterval := fs.Duration("issue-scan-interval", 0, "Legacy GitHub issue scan interval; 0 only normalizes already queued requests")
	issueScanLimit := fs.Int("issue-scan-limit", 10, "Maximum open issues read per configured repository")
	issueScanMaxNewRuns := fs.Int("issue-scan-max-new-runs", 3, "Maximum issue requests queued by this daemon; 0 is unlimited")
	issueScanMaxIterations := fs.Int("issue-scan-max-iterations", 30, "Per-order attempt budget recorded by the scanner")
	issueScanMaxCostUSD := fs.Float64("issue-scan-max-cost-usd", 25, "Per-order cost budget recorded by the scanner")
	issueScanRepos := repeatedStringFlag{}
	issueScanLabels := repeatedStringFlag{}
	fs.Var(&issueScanRepos, "issue-scan-repo", "Exact transpara-ai/REPO scanned by the legacy interval scanner (repeatable)")
	fs.Var(&issueScanLabels, "issue-scan-label", "GitHub issue label filter (repeatable)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*human) == "" {
		return errors.New("--human is required")
	}
	if strings.TrimSpace(*repositoryWorkspace) == "" {
		return errors.New("--repo-workspace-root is required")
	}
	workspace, err := filepath.Abs(*repositoryWorkspace)
	if err != nil {
		return fmt.Errorf("resolve --repo-workspace-root: %w", err)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return fmt.Errorf("resolve --repo-workspace-root realpath: %w", err)
	}
	if info, statErr := os.Stat(workspace); statErr != nil || !info.IsDir() {
		return errors.New("--repo-workspace-root must be an existing directory")
	}

	authorBinding, err := hive.ResolveFactoryV1ProviderBinding(*authorProviderID, *authorFamily, *authorRunner, *authorRunnerSHA, *authorModel, *authorCredentialSource)
	if err != nil {
		return fmt.Errorf("author binding: %w", err)
	}
	reviewerBinding, err := hive.ResolveFactoryV1ProviderBinding(*reviewerProviderID, *reviewerFamily, *reviewerRunner, *reviewerRunnerSHA, *reviewerModel, *reviewerCredentialSource)
	if err != nil {
		return fmt.Errorf("reviewer binding: %w", err)
	}
	if authorBinding.ProviderID == reviewerBinding.ProviderID {
		return errors.New("author and reviewer provider IDs must differ")
	}
	if authorBinding.Family == reviewerBinding.Family {
		return errors.New("author and reviewer families must differ")
	}
	runner, err := hive.NewFactoryV1ExternalRunner([]hive.FactoryV1RunnerProvider{
		{Binding: authorBinding, Args: authorArgs, EnvironmentAllowlist: authorEnvironment, Timeout: *runnerTimeout},
		{Binding: reviewerBinding, Args: reviewerArgs, EnvironmentAllowlist: reviewerEnvironment, Timeout: *runnerTimeout},
	}, *runnerOutputLimit)
	if err != nil {
		return err
	}

	stageProviders := make(map[factoryv1.Stage]factoryv1.ProviderBinding, len(factoryv1.TLCStages))
	for _, stage := range factoryv1.TLCStages {
		stageProviders[stage] = authorBinding
	}
	stageProviders[factoryv1.StageCFADA] = reviewerBinding
	stageProviders[factoryv1.StageCFAR] = reviewerBinding
	standingApproval, err := loadFactoryV1StandingApproval(*standingApprovalFile)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fc, err := openFactoryContext(ctx, *storeDSN, *human)
	if err != nil {
		return err
	}
	defer fc.close()
	conversation := factoryOrderConversation("factory_v1_daemon")
	graph, err := hive.NewFactoryV1EventGraphStore(fc.store, fc.factory, fc.signer, fc.humanID, conversation)
	if err != nil {
		return err
	}
	workStore, err := hive.NewFactoryV1WorkStore(fc.store, fc.factory, fc.signer, fc.humanID, conversation)
	if err != nil {
		return err
	}
	intake, err := factoryv1.NewIntake(graph, workStore, factoryv1.WallClock{})
	if err != nil {
		return err
	}
	normalizer, err := hive.NewFactoryV1IssueNormalizer(fc.store, intake, fc.humanID.Value())
	if err != nil {
		return err
	}
	scheduler, err := factoryv1.NewScheduler(graph, workStore, runner, factoryv1.WallClock{}, factoryv1.SchedulerConfig{
		WorkerCount: factoryv1.DefaultWorkerCount, StageProviders: stageProviders, AuthorFamily: authorBinding.Family,
		StandingApproval: standingApproval,
		RepositoryRoot: func(order factoryv1.FactoryOrder) string {
			parts := strings.Split(order.TargetRepository, "/")
			if len(parts) != 2 {
				return filepath.Join(workspace, "invalid-factory-v1-repository")
			}
			return filepath.Join(workspace, parts[1])
		},
	})
	if err != nil {
		return err
	}
	daemon, err := hive.NewFactoryV1Daemon(normalizer, scheduler, hive.FactoryV1DaemonConfig{
		PollInterval: *pollInterval,
		OnError: func(cycleErr error) {
			log.Printf("factory-v1 recoverable cycle error: %v", cycleErr)
		},
	})
	if err != nil {
		return err
	}

	if *issueScanInterval > 0 {
		repositories, normalizeErr := normalizeIssueScanRepos(issueScanRepos)
		if normalizeErr != nil {
			return normalizeErr
		}
		if len(repositories) == 0 {
			return errors.New("--issue-scan-repo is required when --issue-scan-interval is enabled")
		}
		scannerConfig := issueScanScannerConfig{
			OperatorID: fc.humanID.Value(), Repos: repositories, Labels: append([]string(nil), issueScanLabels...),
			Limit: *issueScanLimit, MaxIterations: *issueScanMaxIterations, MaxCostUSD: *issueScanMaxCostUSD,
			MaxNewRuns: *issueScanMaxNewRuns, Interval: *issueScanInterval,
			AuthorityScope: "factory v1 issue scan to exact-head ready-for-Human PR; no merge or deploy",
		}
		go runIssueScanScannerLoop(ctx, fc, scannerConfig, ghIssueLister{})
	}
	log.Printf("factory-v1 daemon started with %d workers; author=%s/%s reviewer=%s/%s", factoryv1.DefaultWorkerCount, authorBinding.ProviderID, authorBinding.ModelID, reviewerBinding.ProviderID, reviewerBinding.ModelID)
	return daemon.Run(ctx)
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

type factoryV1StandingApprovalFile struct {
	ActorID               string `json:"actor_id"`
	CredentialKeyID       string `json:"credential_key_id"`
	SourceSHA256          string `json:"source_sha256"`
	FactoryOrderBlobSHA   string `json:"factory_order_blob_sha"`
	ApprovalSentence      string `json:"approval_sentence"`
	ApprovalSourceEventID string `json:"approval_source_event_id"`
}

func loadFactoryV1StandingApproval(path string) (*factoryv1.StandingApprovalBinding, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat standing approval binding: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("standing approval binding must be a private regular file (mode 0600 or stricter)")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open standing approval binding: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var input factoryV1StandingApprovalFile
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("decode standing approval binding: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return nil, errors.New("decode standing approval binding: trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode standing approval binding trailing data: %w", err)
	}
	return &factoryv1.StandingApprovalBinding{
		ActorID: input.ActorID, CredentialKeyID: input.CredentialKeyID, SourceSHA256: input.SourceSHA256,
		FactoryOrderBlobSHA: input.FactoryOrderBlobSHA, ApprovalSentence: input.ApprovalSentence,
		ApprovalSourceEventID: input.ApprovalSourceEventID,
	}, nil
}
