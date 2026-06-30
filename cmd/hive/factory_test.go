package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/hive"
	hiveregistry "github.com/transpara-ai/hive/pkg/registry"
)

var testIssueScanOpenTargetStateResolverOption factoryRuntimeOption = func(cfg *hive.Config) {
	cfg.IssueScanTargetStateResolver = func(_ context.Context, repo string, number int) (hive.IssueScanTargetState, error) {
		return hive.IssueScanTargetState{
			Repository: repo,
			Number:     number,
			State:      "open",
			Labels:     []string{hive.IssueScanPRReadyLabel},
		}, nil
	}
}

// TestFactoryVerbIsRegistered asserts the router knows the "factory" verb and,
// when invoked with no subcommand, returns a usage error that names it.
func TestFactoryVerbIsRegistered(t *testing.T) {
	err := routeAndDispatch([]string{"factory"})
	if err == nil || !strings.Contains(err.Error(), "factory") {
		t.Fatalf("expected factory usage error, got %v", err)
	}
}

// TestFactoryOrderRequiresHuman asserts the --human guard fires BEFORE any side
// effect: the spec path does not exist, so if validation ran in the wrong order
// we would see a file/store/loop error instead of a missing-human error.
func TestFactoryOrderRequiresHuman(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "order", "--spec", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryScanIssuesRequiresHumanBeforeGitHub(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "scan-issues", "--repo", "transpara-ai/hive"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryScanIssuesRequiresRepoBeforeGitHub(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "scan-issues", "--human", "Michael"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing --repo error, got %v", err)
	}
}

func TestFactoryScanIssuesRejectsMissingModelOverrideCatalogBeforeGitHub(t *testing.T) {
	missingCatalog := filepath.Join(t.TempDir(), "missing-model-catalog.yaml")
	err := routeAndDispatch([]string{
		"factory", "scan-issues",
		"--human", "Michael",
		"--repo", "transpara-ai/hive",
		"--catalog", missingCatalog,
		"--model", "implementer=codex",
	})
	if err == nil {
		t.Fatal("expected missing catalog error")
	}
	for _, want := range []string{"load catalog for model overrides", missingCatalog} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing catalog error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "gh issue list") || strings.Contains(err.Error(), "no open GitHub issues") {
		t.Fatalf("catalog validation should run before GitHub scanning, got %v", err)
	}
}

func TestFactoryProgressIssueScanRequiresHumanBeforeStore(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "progress-issue-scan", "--run", "run_issue_001"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryProgressIssueScanRequiresRunBeforeStore(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "progress-issue-scan", "--human", "Michael"})
	if err == nil || !strings.Contains(err.Error(), "--run") {
		t.Fatalf("expected missing --run error, got %v", err)
	}
}

func TestFactoryProgressIssueScanRequiresConfiguredRunnersSwitch(t *testing.T) {
	err := routeAndDispatch([]string{
		"factory", "progress-issue-scan",
		"--human", "Michael",
		"--run", "run_issue_001",
		"--issue-scan-stage-role-runner", "/bin/true",
	})
	if err == nil || !strings.Contains(err.Error(), "--run-configured-runners") {
		t.Fatalf("expected configured runners switch error, got %v", err)
	}
}

func TestFactoryProgressIssueScanConfiguredRunnerGuards(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	okRunner := issueScanExecutableForTest(t)
	base := []string{
		"factory", "progress-issue-scan",
		"--human", "Michael",
		"--run", "run_issue_001",
		"--run-configured-runners",
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "draft PR create requires token",
			args: []string{"--issue-scan-draft-pr-create"},
			want: "GITHUB_TOKEN",
		},
		{
			name: "mark ready requires token",
			args: []string{"--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner", okRunner},
			want: "GITHUB_TOKEN",
		},
		{
			name: "mark ready requires ready-state review runner",
			args: []string{"--issue-scan-ready-pr-mark-ready"},
			want: "--issue-scan-ready-pr-review-runner",
		},
		{
			name: "mark ready conflicts with generic ready runner",
			args: []string{"--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-runner", okRunner},
			want: "--issue-scan-ready-pr-runner",
		},
		{
			name: "ready review runner requires mark ready",
			args: []string{"--issue-scan-ready-pr-review-runner", okRunner},
			want: "--issue-scan-ready-pr-mark-ready",
		},
		{
			name: "runner arg requires runner",
			args: []string{"--issue-scan-review-runner-arg", "--json"},
			want: "--issue-scan-review-runner",
		},
		{
			name: "timeout must be positive",
			args: []string{"--issue-scan-review-runner", okRunner, "--issue-scan-review-timeout", "0s"},
			want: "--issue-scan-review-timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := routeAndDispatch(append(append([]string(nil), base...), tt.args...))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("routeAndDispatch error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestFactoryProgressIssueScanConfiguredRunnerRejectsMissingExecutableBeforeRuntime(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{name: "stage role runner", flag: "--issue-scan-stage-role-runner"},
		{name: "implementation runner", flag: "--issue-scan-implementation-runner"},
		{name: "review runner", flag: "--issue-scan-review-runner"},
		{name: "blocker repair runner", flag: "--issue-scan-blocker-repair-runner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missingRunner := filepath.Join(t.TempDir(), "missing-runner")
			err := routeAndDispatch([]string{
				"factory", "progress-issue-scan",
				"--human", "Michael",
				"--run", "run_issue_001",
				"--run-configured-runners",
				tt.flag, missingRunner,
			})
			if err == nil {
				t.Fatal("expected missing runner executable error")
			}
			for _, want := range []string{tt.flag, missingRunner} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing executable error missing %q: %v", want, err)
				}
			}
			if strings.Contains(err.Error(), "progress issue-scan lifecycle") || strings.Contains(err.Error(), "queued run") {
				t.Fatalf("runner executable preflight should run before runtime progress, got %v", err)
			}
		})
	}
}

func TestFactoryProgressIssueScanReadyReviewRunnerRejectsMissingExecutableBeforeGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	missingRunner := filepath.Join(t.TempDir(), "missing-ready-review-runner")
	err := routeAndDispatch([]string{
		"factory", "progress-issue-scan",
		"--human", "Michael",
		"--run", "run_issue_001",
		"--run-configured-runners",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner", missingRunner,
	})
	if err == nil {
		t.Fatal("expected missing ready-state review runner executable error")
	}
	for _, want := range []string{"--issue-scan-ready-pr-review-runner", missingRunner} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing ready review executable error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("ready-state review executable preflight should run before GitHub token gate, got %v", err)
	}
}

func TestFactoryProgressIssueScanGenericReadyRunnerRejectsMissingExecutableBeforeRuntime(t *testing.T) {
	missingRunner := filepath.Join(t.TempDir(), "missing-ready-pr-runner")
	err := routeAndDispatch([]string{
		"factory", "progress-issue-scan",
		"--human", "Michael",
		"--run", "run_issue_001",
		"--run-configured-runners",
		"--issue-scan-ready-pr-runner", missingRunner,
	})
	if err == nil {
		t.Fatal("expected missing generic ready PR runner executable error")
	}
	for _, want := range []string{"--issue-scan-ready-pr-runner", missingRunner} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing generic ready runner executable error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "progress issue-scan lifecycle") || strings.Contains(err.Error(), "queued run") {
		t.Fatalf("generic ready runner executable preflight should run before runtime progress, got %v", err)
	}
}

func TestFactoryIssueScanRunnerContractsDocumentsFullChain(t *testing.T) {
	doc := issueScanRunnerContracts()
	if doc.Kind != "issue_scan_runner_contracts" {
		t.Fatalf("contract kind = %q", doc.Kind)
	}
	if doc.LifecycleVersion != "civilization_issue_to_human_ready_pr_v0.9" {
		t.Fatalf("contract lifecycle version = %q", doc.LifecycleVersion)
	}
	for _, want := range []string{
		"--issue-scan-require-full-chain",
		"--issue-scan-repo",
		"--issue-scan-registry",
		"--issue-scan-stage-role-runner",
		"--issue-scan-implementation-runner",
		"--issue-scan-review-runner",
		"--issue-scan-blocker-repair-runner",
		"--issue-scan-draft-pr-request",
		"--issue-scan-draft-pr-create",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner",
	} {
		if !slices.Contains(doc.FullChainDaemonFlags, want) {
			t.Fatalf("full-chain flags missing %q: %+v", want, doc.FullChainDaemonFlags)
		}
	}
	contracts := map[string]issueScanRunnerContract{}
	for _, contract := range doc.ExternalRunnerContracts {
		contracts[contract.ID] = contract
	}
	tests := []struct {
		id          string
		contextKind string
		stdoutType  string
		required    string
	}{
		{"stage_role_output_runner", "issue_scan_stage_role_output_runner_context", "hive.IssueScanStageRoleOutputRunnerResult", "role_outputs[]"},
		{"implementation_runner", "issue_scan_implementation_runner_context", "hive.IssueScanImplementationRunnerResult", "operate_result_body"},
		{"adversarial_review_runner", "issue_scan_adversarial_review_context", "hive.IssueScanAdversarialReviewReceipt", "reviewed_head_sha"},
		{"blocker_repair_runner", "issue_scan_blocker_repair_runner_context", "hive.IssueScanBlockerRepairRunnerResult", "operate_result_body"},
		{"ready_pr_evidence_runner", "issue_scan_ready_pr_runner_context", "hive.IssueScanReadyPRRunnerResult", "ready_pr_evidence.kind=issue_scan_ready_pr_evidence"},
		{"ready_state_review_runner", "issue_scan_ready_state_review_context", "hive.IssueScanReadyStateReviewReceipt", "status"},
	}
	for _, tt := range tests {
		contract, ok := contracts[tt.id]
		if !ok {
			t.Fatalf("contract %q missing from %+v", tt.id, contracts)
		}
		if contract.StdinContextKind != tt.contextKind {
			t.Fatalf("%s context kind = %q, want %q", tt.id, contract.StdinContextKind, tt.contextKind)
		}
		if contract.StdoutContractType != tt.stdoutType {
			t.Fatalf("%s stdout type = %q, want %q", tt.id, contract.StdoutContractType, tt.stdoutType)
		}
		if !slices.Contains(contract.StdoutRequiredFields, tt.required) {
			t.Fatalf("%s required fields missing %q: %+v", tt.id, tt.required, contract.StdoutRequiredFields)
		}
	}
	managedContracts := map[string]issueScanRunnerContract{}
	for _, contract := range doc.ManagedBoundaryContracts {
		managedContracts[contract.ID] = contract
	}
	for _, tt := range []struct {
		id          string
		contextKind string
		stdoutType  string
		required    string
		command     string
	}{
		{"draft_pr_authority_requester", "issue_scan_draft_pr_authority_request_runner_context", "hive.IssueScanDraftPRAuthorityRequestRunnerResult", "base_sha", "hive factory request-issue-scan-pr"},
		{"draft_pr_creation_runner", "issue_scan_draft_pr_creation_runner_context", "hive.IssueScanDraftPRCreationResult", "draft_pr_receipt", "hive factory create-issue-scan-draft-pr"},
	} {
		contract, ok := managedContracts[tt.id]
		if !ok {
			t.Fatalf("managed contract %q missing from %+v", tt.id, managedContracts)
		}
		if contract.StandaloneCommand != tt.command {
			t.Fatalf("%s managed command = %q, want %q", tt.id, contract.StandaloneCommand, tt.command)
		}
		if contract.StdinContextKind != tt.contextKind {
			t.Fatalf("%s managed context kind = %q, want %q", tt.id, contract.StdinContextKind, tt.contextKind)
		}
		if contract.StdoutContractType != tt.stdoutType {
			t.Fatalf("%s managed stdout type = %q, want %q", tt.id, contract.StdoutContractType, tt.stdoutType)
		}
		if !slices.Contains(contract.StdoutRequiredFields, tt.required) {
			t.Fatalf("%s managed required fields missing %q: %+v", tt.id, tt.required, contract.StdoutRequiredFields)
		}
	}
	if doc.InternalFinalizerContract == nil || doc.InternalFinalizerContract.ID != "ready_pr_finalizer" {
		t.Fatalf("missing ready PR finalizer contract: %+v", doc.InternalFinalizerContract)
	}
	if !documentedTerminalPathIncludes(doc.TerminalStagePaths, "managed_ready_pr_finalizer", "--issue-scan-ready-pr-mark-ready") {
		t.Fatalf("managed ready PR finalizer path missing mark-ready flag: %+v", doc.TerminalStagePaths)
	}
	if !documentedTerminalPathIncludes(doc.TerminalStagePaths, "external_ready_pr_evidence_runner", "--issue-scan-ready-pr-runner") {
		t.Fatalf("external ready PR runner path missing generic runner flag: %+v", doc.TerminalStagePaths)
	}
	if !slices.Contains(doc.OperatorNotes, "Full-chain daemon startup resolves every configured external runner executable before entering the daemon loop.") {
		t.Fatalf("operator notes missing full-chain executable preflight: %+v", doc.OperatorNotes)
	}
	if !slices.Contains(doc.OperatorNotes, "Named configured progress resolves every supplied external runner executable before opening the runtime or invoking runners.") {
		t.Fatalf("operator notes missing named progress executable preflight: %+v", doc.OperatorNotes)
	}
	if !slices.Contains(doc.OperatorNotes, "Standalone run-issue-scan-* commands resolve --runner before opening the runtime or building runner context.") {
		t.Fatalf("operator notes missing standalone runner executable preflight: %+v", doc.OperatorNotes)
	}
}

func TestFactoryIssueScanRunnerContractsIsDiscoverable(t *testing.T) {
	err := routeAndDispatch([]string{"factory"})
	if err == nil || !strings.Contains(err.Error(), "issue-scan-runner-contracts") {
		t.Fatalf("factory usage should mention issue-scan-runner-contracts, got %v", err)
	}
	if !strings.Contains(helpText(), "issue-scan-runner-contracts") {
		t.Fatalf("top-level help should mention issue-scan-runner-contracts")
	}
}

func TestFactoryIssueScanRunnerContextsIsDiscoverable(t *testing.T) {
	err := routeAndDispatch([]string{"factory"})
	if err == nil || !strings.Contains(err.Error(), "issue-scan-runner-contexts") {
		t.Fatalf("factory usage should mention issue-scan-runner-contexts, got %v", err)
	}
	if !strings.Contains(helpText(), "issue-scan-runner-contexts") {
		t.Fatalf("top-level help should mention issue-scan-runner-contexts")
	}
}

func TestFactoryIssueScanRunnerContractsRouteEmitsJSON(t *testing.T) {
	stdout, err := captureFactoryStdout(t, func() error {
		return routeAndDispatch([]string{"factory", "issue-scan-runner-contracts"})
	})
	if err != nil {
		t.Fatalf("issue-scan-runner-contracts route: %v", err)
	}
	var doc issueScanRunnerContractsDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("issue-scan-runner-contracts stdout is not JSON: %v\n%s", err, stdout)
	}
	if doc.Kind != "issue_scan_runner_contracts" {
		t.Fatalf("stdout contract kind = %q", doc.Kind)
	}
}

func TestFactoryIssueScanRunnerContextsRequiresHumanBeforeStore(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "issue-scan-runner-contexts", "--run", "run_issue_001"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryIssueScanRunnerContextsRequiresRunBeforeStore(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "issue-scan-runner-contexts", "--human", "Michael"})
	if err == nil || !strings.Contains(err.Error(), "--run") {
		t.Fatalf("expected missing --run error, got %v", err)
	}
}

func TestFactoryIssueScanRunnerContextsRejectsUnknownFormat(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "issue-scan-runner-contexts", "--human", "Michael", "--run", "run_issue_001", "--format", "yaml"})
	if err == nil || !strings.Contains(err.Error(), "--format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestFactoryIssueScanRunnerContextsRouteEmitsJSON(t *testing.T) {
	ctx := context.Background()
	rt, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	t.Cleanup(fc.close)
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)
	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {{
				Repo:   "transpara-ai/hive",
				Number: 214,
				Title:  "Probe issue-scan runner contexts",
				URL:    "https://github.com/transpara-ai/hive/issues/214",
				Labels: []string{"cc:pr-ready"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}
	queued, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("runIssueScanScannerCycle: %v", err)
	}
	if !queued.Queued || queued.QueuedRunID == "" {
		t.Fatalf("queued = %+v, want run id", queued)
	}

	original := openFactoryRuntimeForIssueScanRunnerContexts
	openFactoryRuntimeForIssueScanRunnerContexts = func(context.Context, string, string, string, string, ...factoryRuntimeOption) (*hive.Runtime, *factoryContext, error) {
		return rt, &factoryContext{}, nil
	}
	t.Cleanup(func() { openFactoryRuntimeForIssueScanRunnerContexts = original })

	stdout, err := captureFactoryStdout(t, func() error {
		return routeAndDispatch([]string{"factory", "issue-scan-runner-contexts", "--human", "Michael", "--run", queued.QueuedRunID, "--include-payload"})
	})
	if err != nil {
		t.Fatalf("issue-scan-runner-contexts route: %v", err)
	}
	var doc hive.IssueScanRunnerContextProbeDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("issue-scan-runner-contexts stdout is not JSON: %v\n%s", err, stdout)
	}
	if doc.Kind != "issue_scan_runner_context_probe" || doc.RunID != queued.QueuedRunID || doc.ReadyCount != 1 {
		t.Fatalf("stdout probe = %+v, queued=%+v", doc, queued)
	}
	stage := hive.IssueScanRunnerContextProbe{}
	for _, probe := range doc.Contexts {
		if probe.ID == "stage_role_output_runner" {
			stage = probe
			break
		}
	}
	if !stage.Ready || len(stage.ContextPayload) == 0 {
		t.Fatalf("stage role probe = %+v, want ready payload", stage)
	}
}

func TestFactoryIssueScanRunnerContractsRequiredFieldsMatchExportedJSONTags(t *testing.T) {
	doc := issueScanRunnerContracts()
	var canonicalReadyEvidence hive.IssueScanReadyPREvidence
	readyBody, err := hive.IssueScanReadyPREvidenceArtifactBody(hive.IssueScanReadyPREvidence{})
	if err != nil {
		t.Fatalf("IssueScanReadyPREvidenceArtifactBody: %v", err)
	}
	if err := json.Unmarshal([]byte(readyBody), &canonicalReadyEvidence); err != nil {
		t.Fatalf("decode canonical ready PR evidence body: %v", err)
	}
	if canonicalReadyEvidence.LifecycleVersion != doc.LifecycleVersion {
		t.Fatalf("contract lifecycle_version = %q, want canonical ready evidence version %q", doc.LifecycleVersion, canonicalReadyEvidence.LifecycleVersion)
	}
	if !contractFieldContains(doc.ExternalRunnerContracts, "ready_pr_evidence_runner", "ready_pr_evidence.kind="+canonicalReadyEvidence.Kind) {
		t.Fatalf("ready PR evidence runner contract does not document canonical kind %q", canonicalReadyEvidence.Kind)
	}

	assertTypeName(t, reflect.TypeOf(hive.IssueScanStageRoleOutputRunnerContext{}), "hive.IssueScanStageRoleOutputRunnerContext")
	assertTypeName(t, reflect.TypeOf(hive.IssueScanImplementationRunnerContext{}), "hive.IssueScanImplementationRunnerContext")
	assertTypeName(t, reflect.TypeOf(hive.IssueScanAdversarialReviewContext{}), "hive.IssueScanAdversarialReviewContext")
	assertTypeName(t, reflect.TypeOf(hive.IssueScanBlockerRepairRunnerContext{}), "hive.IssueScanBlockerRepairRunnerContext")
	assertTypeName(t, reflect.TypeOf(hive.IssueScanReadyPRRunnerContext{}), "hive.IssueScanReadyPRRunnerContext")
	assertTypeName(t, reflect.TypeOf(hive.IssueScanReadyStateReviewContext{}), "hive.IssueScanReadyStateReviewContext")

	assertJSONField(t, reflect.TypeOf(hive.IssueScanStageRoleOutputRunnerResult{}), "role_outputs")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanImplementationRunnerResult{}), "operate_result_body")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanImplementationRunnerResult{}), "completion_summary")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanAdversarialReviewReceipt{}), "reviewed_head_sha")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanAdversarialReviewReceipt{}), "review_ref")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanBlockerRepairRunnerResult{}), "operate_result_body")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanBlockerRepairRunnerResult{}), "completion_summary")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanReadyPRRunnerResult{}), "draft_pr_receipt")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanReadyPRRunnerResult{}), "ready_pr_evidence")
	assertJSONField(t, reflect.TypeOf(hive.TransparaAIDraftPRReceipt{}), "kind")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanReadyPREvidence{}), "kind")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanStageRoleOutputEvidence{}), "role")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanStageRoleOutputEvidence{}), "summary")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanStageRoleOutputEvidence{}), "outputs")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanAdversarialReviewReceipt{}), "verdict")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanAdversarialReviewReceipt{}), "summary")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanAdversarialReviewReceipt{}), "confidence")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanReadyStateReviewReceipt{}), "review_ref")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanReadyStateReviewReceipt{}), "reviewed_head_sha")
	assertJSONField(t, reflect.TypeOf(hive.IssueScanReadyStateReviewReceipt{}), "status")
}

func TestFactoryIssueScanRunnerContractsFlagsAreRegistered(t *testing.T) {
	doc := issueScanRunnerContracts()
	daemonFlags := documentedIssueScanFlags(doc.FullChainDaemonFlags, doc.TerminalStagePaths)
	for _, flag := range daemonFlags {
		t.Run("daemon "+flag, func(t *testing.T) {
			args := appendFlagValue([]string{"factory", "daemon"}, flag, issueScanFlagTestValue(flag))
			err := routeAndDispatch(args)
			if err != nil && strings.Contains(err.Error(), "flag provided but not defined") {
				t.Fatalf("daemon flag %s is not registered: %v", flag, err)
			}
		})
	}

	progressFlags := documentedIssueScanFlags(doc.NamedProgressFlags, doc.TerminalStagePaths)
	for _, flag := range progressFlags {
		t.Run("progress "+flag, func(t *testing.T) {
			args := appendFlagValue([]string{"factory", "progress-issue-scan"}, flag, issueScanFlagTestValue(flag))
			err := routeAndDispatch(args)
			if err != nil && strings.Contains(err.Error(), "flag provided but not defined") {
				t.Fatalf("progress flag %s is not registered: %v", flag, err)
			}
		})
	}
}

func TestFactoryIssueScanRunnerContractsRejectsUnknownFormat(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "issue-scan-runner-contracts", "--format", "yaml"})
	if err == nil || !strings.Contains(err.Error(), "--format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestFactoryRecordIssueScanReviewRequiresHumanBeforeFileRead(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "record-issue-scan-review", "--run", "run_issue_001", "--review-file", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func documentedTerminalPathIncludes(paths []issueScanTerminalPath, id, flag string) bool {
	for _, path := range paths {
		if path.ID == id && slices.Contains(path.Flags, flag) {
			return true
		}
	}
	return false
}

func contractFieldContains(contracts []issueScanRunnerContract, id, field string) bool {
	for _, contract := range contracts {
		if contract.ID == id && slices.Contains(contract.StdoutRequiredFields, field) {
			return true
		}
	}
	return false
}

func assertJSONField(t *testing.T, typ reflect.Type, field string) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == field {
			return
		}
	}
	t.Fatalf("%s has no json field %q", typ, field)
}

func assertTypeName(t *testing.T, typ reflect.Type, want string) {
	t.Helper()
	if typ.PkgPath() == "" {
		t.Fatalf("type %s has no package path", typ)
	}
	got := typ.PkgPath() + "." + typ.Name()
	if got != "github.com/transpara-ai/hive/pkg/"+want {
		t.Fatalf("type name = %q, want package type %q", got, want)
	}
}

func appendFlagValue(args []string, flag, value string) []string {
	args = append(args, flag)
	if value != "" {
		args = append(args, value)
	}
	return args
}

func documentedIssueScanFlags(values []string, terminalPaths []issueScanTerminalPath) []string {
	seen := map[string]bool{}
	var out []string
	add := func(flag string) {
		if strings.HasPrefix(flag, "--") && !seen[flag] {
			seen[flag] = true
			out = append(out, flag)
		}
	}
	for _, flag := range values {
		add(flag)
	}
	for _, path := range terminalPaths {
		for _, flag := range path.Flags {
			add(flag)
		}
		for _, flag := range path.MutuallyExclusiveWith {
			add(flag)
		}
	}
	return out
}

func issueScanFlagTestValue(flag string) string {
	switch flag {
	case "--issue-scan-interval":
		return "1m"
	case "--issue-scan-repo":
		return "transpara-ai/hive"
	case "--repo-workspace-root":
		return "/tmp"
	case "--issue-scan-stage-role-runner",
		"--issue-scan-implementation-runner",
		"--issue-scan-review-runner",
		"--issue-scan-blocker-repair-runner",
		"--issue-scan-ready-pr-review-runner",
		"--issue-scan-ready-pr-runner":
		return "/bin/true"
	case "--run":
		return "run_issue_001"
	default:
		return ""
	}
}

func issueScanExecutableForTest(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return path
}

func captureFactoryStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	original := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	type readResult struct {
		body []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		body, err := io.ReadAll(read)
		readDone <- readResult{body: body, err: err}
	}()
	os.Stdout = write
	defer func() {
		os.Stdout = original
		_ = write.Close()
		_ = read.Close()
	}()
	callErr := fn()
	if err := write.Close(); err != nil && callErr == nil {
		callErr = err
	}
	result := <-readDone
	if result.err != nil && callErr == nil {
		callErr = result.err
	}
	return string(result.body), callErr
}

func TestFactoryRunIssueScanReviewRequiresHumanBeforeRunner(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "run-issue-scan-review", "--run", "run_issue_001", "--runner", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRunIssueScanBlockerRepairRequiresHumanBeforeRunner(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "run-issue-scan-blocker-repair", "--run", "run_issue_001", "--runner", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRunIssueScanStageRoleOutputRequiresHumanBeforeRunner(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "run-issue-scan-stage-role-output", "--run", "run_issue_001", "--runner", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRunIssueScanImplementationRequiresHumanBeforeRunner(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "run-issue-scan-implementation", "--run", "run_issue_001", "--runner", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRecordIssueScanDraftPRRequiresHumanBeforeFileRead(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "record-issue-scan-draft-pr", "--run", "run_issue_001", "--receipt-file", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRecordIssueScanReadyPRRequiresHumanBeforeFileRead(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "record-issue-scan-ready-pr", "--run", "run_issue_001", "--evidence-file", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRunIssueScanReadyPRRequiresHumanBeforeRunner(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "run-issue-scan-ready-pr", "--run", "run_issue_001", "--runner", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryRunIssueScanStandaloneRunnersRejectMissingExecutableBeforeRuntime(t *testing.T) {
	tests := []struct {
		name       string
		subcommand string
	}{
		{name: "stage role output", subcommand: "run-issue-scan-stage-role-output"},
		{name: "implementation", subcommand: "run-issue-scan-implementation"},
		{name: "review", subcommand: "run-issue-scan-review"},
		{name: "blocker repair", subcommand: "run-issue-scan-blocker-repair"},
		{name: "ready PR", subcommand: "run-issue-scan-ready-pr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missingRunner := filepath.Join(t.TempDir(), "missing-runner")
			err := routeAndDispatch([]string{
				"factory", tt.subcommand,
				"--human", "Michael",
				"--run", "run_issue_001",
				"--runner", missingRunner,
			})
			if err == nil {
				t.Fatal("expected missing runner executable error")
			}
			for _, want := range []string{"--runner", missingRunner} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing runner executable error missing %q: %v", want, err)
				}
			}
			for _, notWant := range []string{"build issue-scan", "progress issue-scan lifecycle", "queued run"} {
				if strings.Contains(err.Error(), notWant) {
					t.Fatalf("runner executable preflight should run before runtime/context work; got %v", err)
				}
			}
		})
	}
}

func TestFactoryDaemonRequiresReviewRunnerBeforeRunnerArg(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-review-runner-arg=--json"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-review-runner") {
		t.Fatalf("expected missing review runner error, got %v", err)
	}
}

func TestFactoryDaemonRequiresStageRoleRunnerBeforeRunnerArg(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-stage-role-runner-arg=--json"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-stage-role-runner") {
		t.Fatalf("expected missing stage role runner error, got %v", err)
	}
}

func TestFactoryDaemonRequiresImplementationRunnerBeforeRunnerArg(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-implementation-runner-arg=--json"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-implementation-runner") {
		t.Fatalf("expected missing implementation runner error, got %v", err)
	}
}

func TestFactoryDaemonRequiresBlockerRepairRunnerBeforeRunnerArg(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-blocker-repair-runner-arg=--json"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-blocker-repair-runner") {
		t.Fatalf("expected missing blocker repair runner error, got %v", err)
	}
}

func TestFactoryDaemonRequiresReadyPRRunnerBeforeRunnerArg(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-runner-arg=--json"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-runner") {
		t.Fatalf("expected missing ready PR runner error, got %v", err)
	}
}

func TestFactoryDaemonRequiresDraftPRRequestBeforeBase(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-request-base=release/2026-06"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-draft-pr-request") {
		t.Fatalf("expected missing draft PR request flag error, got %v", err)
	}
}

func TestFactoryDaemonRequireFullChainRejectsMissingPieces(t *testing.T) {
	err := routeAndDispatch([]string{
		"factory", "daemon",
		"--human", "Michael",
		"--issue-scan-require-full-chain",
		"--issue-scan-stage-role-runner", "/bin/true",
	})
	if err == nil {
		t.Fatal("expected full-chain preflight error")
	}
	for _, want := range []string{
		"--issue-scan-require-full-chain",
		"--issue-scan-interval",
		"--issue-scan-repo or --issue-scan-registry",
		"--repo-workspace-root",
		"--issue-scan-implementation-runner",
		"--issue-scan-review-runner",
		"--issue-scan-blocker-repair-runner",
		"--issue-scan-draft-pr-request",
		"--issue-scan-draft-pr-create",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("full-chain error missing %q: %v", want, err)
		}
	}
}

func TestFactoryDaemonRequireFullChainRejectsGenericReadyRunner(t *testing.T) {
	err := routeAndDispatch([]string{
		"factory", "daemon",
		"--human", "Michael",
		"--issue-scan-require-full-chain",
		"--issue-scan-interval", "1m",
		"--issue-scan-repo", "transpara-ai/hive",
		"--repo-workspace-root", "/Transpara/transpara-ai/repos",
		"--issue-scan-stage-role-runner", "/bin/true",
		"--issue-scan-implementation-runner", "/bin/true",
		"--issue-scan-review-runner", "/bin/true",
		"--issue-scan-blocker-repair-runner", "/bin/true",
		"--issue-scan-draft-pr-request",
		"--issue-scan-draft-pr-create",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner", "/bin/true",
		"--issue-scan-ready-pr-runner", "/bin/true",
	})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-runner") {
		t.Fatalf("expected generic ready runner rejection, got %v", err)
	}
}

func TestFactoryDaemonRequireFullChainRequiresScannerActivation(t *testing.T) {
	err := routeAndDispatch([]string{
		"factory", "daemon",
		"--human", "Michael",
		"--issue-scan-require-full-chain",
		"--repo-workspace-root", "/Transpara/transpara-ai/repos",
		"--issue-scan-stage-role-runner", "/bin/true",
		"--issue-scan-implementation-runner", "/bin/true",
		"--issue-scan-review-runner", "/bin/true",
		"--issue-scan-blocker-repair-runner", "/bin/true",
		"--issue-scan-draft-pr-request",
		"--issue-scan-draft-pr-create",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner", "/bin/true",
	})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-interval") || !strings.Contains(err.Error(), "--issue-scan-repo or --issue-scan-registry") {
		t.Fatalf("expected scanner activation preflight error, got %v", err)
	}
}

func TestRequireIssueScanRunnerExecutableAcceptsBarePathCommand(t *testing.T) {
	dir := t.TempDir()
	runnerName := "issue-scan-runner-for-path-test"
	runnerPath := filepath.Join(dir, runnerName)
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write test runner: %v", err)
	}
	t.Setenv("PATH", dir)

	if err := requireIssueScanRunnerExecutable("--issue-scan-implementation-runner", runnerName); err != nil {
		t.Fatalf("expected PATH runner to resolve, got %v", err)
	}
}

func TestRequireIssueScanRunnerExecutableRejectsEmbeddedArgs(t *testing.T) {
	dir := t.TempDir()
	runnerName := "issue-scan-runner-with-args-test"
	runnerPath := filepath.Join(dir, runnerName)
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write test runner: %v", err)
	}
	t.Setenv("PATH", dir)

	err := requireIssueScanRunnerExecutable("--issue-scan-implementation-runner", runnerName+" --json")
	if err == nil {
		t.Fatal("expected embedded-args runner string to be rejected")
	}
	if !strings.Contains(err.Error(), "--issue-scan-implementation-runner") || !strings.Contains(err.Error(), runnerName+" --json") {
		t.Fatalf("embedded-args error should name flag and command, got %v", err)
	}
}

func TestRequireIssueScanRunnerExecutableRejectsNonExecutableFile(t *testing.T) {
	runnerPath := filepath.Join(t.TempDir(), "issue-scan-runner-not-executable")
	if err := os.WriteFile(runnerPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable runner: %v", err)
	}

	err := requireIssueScanRunnerExecutable("--issue-scan-review-runner", runnerPath)
	if err == nil {
		t.Fatal("expected non-executable runner file to be rejected")
	}
	if !strings.Contains(err.Error(), "--issue-scan-review-runner") || !strings.Contains(err.Error(), runnerPath) {
		t.Fatalf("non-executable error should name flag and path, got %v", err)
	}
}

func TestFactoryDaemonRequireFullChainRejectsMissingRunnerExecutable(t *testing.T) {
	missingRunner := filepath.Join(t.TempDir(), "missing-runner")
	okRunner := issueScanExecutableForTest(t)
	err := routeAndDispatch([]string{
		"factory", "daemon",
		"--human", "Michael",
		"--issue-scan-require-full-chain",
		"--issue-scan-interval", "1m",
		"--issue-scan-repo", "transpara-ai/hive",
		"--repo-workspace-root", "/Transpara/transpara-ai/repos",
		"--issue-scan-stage-role-runner", okRunner,
		"--issue-scan-implementation-runner", missingRunner,
		"--issue-scan-review-runner", okRunner,
		"--issue-scan-blocker-repair-runner", okRunner,
		"--issue-scan-draft-pr-request",
		"--issue-scan-draft-pr-create",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner", okRunner,
	})
	if err == nil {
		t.Fatal("expected missing runner executable error")
	}
	for _, want := range []string{
		"--issue-scan-require-full-chain",
		"cannot find executable",
		"--issue-scan-implementation-runner",
		missingRunner,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("full-chain executable error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("runner executable preflight should run before GitHub token gate, got %v", err)
	}
}

func TestFactoryDaemonRequireFullChainReachesGitHubTokenGateWhenConfigured(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	okRunner := issueScanExecutableForTest(t)
	err := routeAndDispatch([]string{
		"factory", "daemon",
		"--human", "Michael",
		"--issue-scan-require-full-chain",
		"--issue-scan-interval", "1m",
		"--issue-scan-repo", "transpara-ai/hive",
		"--repo-workspace-root", "/Transpara/transpara-ai/repos",
		"--issue-scan-stage-role-runner", okRunner,
		"--issue-scan-implementation-runner", okRunner,
		"--issue-scan-review-runner", okRunner,
		"--issue-scan-blocker-repair-runner", okRunner,
		"--issue-scan-draft-pr-request",
		"--issue-scan-draft-pr-create",
		"--issue-scan-ready-pr-mark-ready",
		"--issue-scan-ready-pr-review-runner", okRunner,
	})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("expected configured full chain to reach GitHub token gate, got %v", err)
	}
	if strings.Contains(err.Error(), "--issue-scan-require-full-chain requires") {
		t.Fatalf("full-chain preflight should pass when all flags are present, got %v", err)
	}
}

func TestFactoryDaemonReadyPRMarkReadyRequiresReviewRunner(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-review-runner") {
		t.Fatalf("expected missing ready PR review runner error, got %v", err)
	}
}

func TestFactoryDaemonReadyPRMarkReadyRequiresGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner", "/bin/true"})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("expected missing GITHUB_TOKEN error, got %v", err)
	}
}

func TestFactoryDaemonReadyPRMarkReadyRejectsGenericRunner(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner", "/bin/true", "--issue-scan-ready-pr-runner", "/bin/true"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-runner") {
		t.Fatalf("expected generic runner conflict error, got %v", err)
	}
}

func TestFactoryDaemonReadyPRReviewRunnerRequiresMarkReady(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-review-runner", "/bin/true"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-mark-ready") {
		t.Fatalf("expected mark-ready required error, got %v", err)
	}
}

func TestFactoryDaemonReadyPRMarkReadyRequiresPositiveTimeout(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner", "/bin/true", "--issue-scan-ready-pr-timeout", "0s"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-ready-pr-timeout") {
		t.Fatalf("expected ready PR timeout error, got %v", err)
	}
}

func TestFactoryDaemonReadyPRMarkReadyRejectsAutoApproveRequests(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner", "/bin/true", "--approve-requests"})
	if err == nil || !strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("expected auto-approval guard error, got %v", err)
	}
}

func TestFactoryDaemonReadyPRMarkReadyRejectsAutoApproveRoles(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner", "/bin/true", "--approve-roles"})
	if err == nil || !strings.Contains(err.Error(), "--approve-roles") {
		t.Fatalf("expected auto-role guard error, got %v", err)
	}
}

func TestFactoryDaemonDraftPRCreateRequiresGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-create"})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("expected missing GITHUB_TOKEN error, got %v", err)
	}
}

func TestFactoryDaemonDraftPRCreateRejectsAutoApproveRequests(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-create", "--approve-requests"})
	if err == nil || !strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("expected auto-approval guard error, got %v", err)
	}
}

func TestFactoryDaemonDraftPRCreateRejectsAutoApproveRoles(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "token")
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-create", "--approve-roles"})
	if err == nil || !strings.Contains(err.Error(), "--approve-roles") {
		t.Fatalf("expected auto-role guard error, got %v", err)
	}
}

func TestFactoryDaemonDraftPRRequestRejectsAutoApproveRequests(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-request", "--approve-requests"})
	if err == nil || !strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("expected auto-approval guard error, got %v", err)
	}
}

func TestFactoryDaemonDraftPRRequestRejectsAutoApproveRoles(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-request", "--approve-roles"})
	if err == nil || !strings.Contains(err.Error(), "--approve-roles") {
		t.Fatalf("expected auto-role guard error, got %v", err)
	}
}

func TestFactoryDaemonDraftPRRequestRequiresBase(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-draft-pr-request", "--issue-scan-draft-pr-request-base", ""})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-draft-pr-request-base") {
		t.Fatalf("expected draft PR request base error, got %v", err)
	}
}

func TestIssueScanDraftPRBaseRefNormalization(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		branch string
		remote string
	}{
		{name: "default", input: "", branch: "main", remote: "origin/main"},
		{name: "branch", input: "release/2026-06", branch: "release/2026-06", remote: "origin/release/2026-06"},
		{name: "origin", input: "origin/main", branch: "main", remote: "origin/main"},
		{name: "heads", input: "refs/heads/main", branch: "main", remote: "origin/main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitBaseBranchRef(tt.input); got != tt.branch {
				t.Fatalf("gitBaseBranchRef(%q) = %q, want %q", tt.input, got, tt.branch)
			}
			if got := gitRemoteBaseRef(tt.input); got != tt.remote {
				t.Fatalf("gitRemoteBaseRef(%q) = %q, want %q", tt.input, got, tt.remote)
			}
		})
	}
}

func TestIssueScanDraftPRBaseRefValidation(t *testing.T) {
	valid := []string{"main", "release/2026-06", "feature.issue_scan_2026"}
	for _, base := range valid {
		t.Run("valid_"+strings.ReplaceAll(base, "/", "_"), func(t *testing.T) {
			if err := validateGitBaseBranchRef(base); err != nil {
				t.Fatalf("validateGitBaseBranchRef(%q): %v", base, err)
			}
		})
	}
	invalid := []string{"-main", "release//2026", "/main", "main/", "release..main", "release@{1}", "main.lock", "main:prod", "main\nprod"}
	for _, base := range invalid {
		t.Run("invalid_"+strings.NewReplacer("/", "_", "\n", "_").Replace(base), func(t *testing.T) {
			if err := validateGitBaseBranchRef(base); err == nil {
				t.Fatalf("validateGitBaseBranchRef(%q) succeeded, want error", base)
			}
		})
	}
}

func TestIssueScanDraftPRGitBaseCommitSHAFetchesRemoteBase(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	source := filepath.Join(root, "source")
	clone := filepath.Join(root, "clone")

	runGitForTest(t, "", "init", "--bare", remote)
	runGitForTest(t, "", "init", source)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("issue-scan base\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitForTest(t, source, "add", "README.md")
	runGitForTest(t, source, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	runGitForTest(t, source, "branch", "-M", "main")
	runGitForTest(t, source, "remote", "add", "origin", remote)
	runGitForTest(t, source, "push", "origin", "main")
	runGitForTest(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitForTest(t, "", "clone", remote, clone)

	want := strings.TrimSpace(runGitOutputForTest(t, source, "rev-parse", "HEAD"))
	got, err := gitBaseCommitSHA(ctx, clone, "refs/heads/main")
	if err != nil {
		t.Fatalf("gitBaseCommitSHA: %v", err)
	}
	if got != want {
		t.Fatalf("gitBaseCommitSHA = %q, want %q", got, want)
	}
}

func TestIssueScanDraftPRAuthorityRequestLocalGitRunnerRequiresRepoPath(t *testing.T) {
	runner := issueScanDraftPRAuthorityRequestLocalGitRunner("main")
	_, err := runner(context.Background(), hive.IssueScanDraftPRAuthorityRequestRunnerContext{})
	if err == nil || !strings.Contains(err.Error(), "resolved repository checkout") {
		t.Fatalf("issueScanDraftPRAuthorityRequestLocalGitRunner error = %v, want resolved checkout guard", err)
	}
}

func TestIssueScanDraftPRAuthorityNonceIsDeterministic(t *testing.T) {
	requestContext := hive.IssueScanDraftPRAuthorityRequestRunnerContext{
		RunID:          "run_issue_001",
		FactoryOrderID: "order_issue_001",
		Repository:     "transpara-ai/hive",
		OperateBranch:  "codex/run-issue-001",
		OperateCommit:  "cccccccccccccccccccccccccccccccccccccccc",
	}
	first := issueScanDraftPRAuthorityNonce(requestContext, "main", "dddddddddddddddddddddddddddddddddddddddd")
	second := issueScanDraftPRAuthorityNonce(requestContext, "main", "dddddddddddddddddddddddddddddddddddddddd")
	movedBase := issueScanDraftPRAuthorityNonce(requestContext, "main", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	if first == "" || !strings.HasPrefix(first, "issue-scan-") {
		t.Fatalf("nonce = %q, want issue-scan prefix", first)
	}
	if first != second {
		t.Fatalf("nonce not deterministic: %q then %q", first, second)
	}
	if movedBase == first {
		t.Fatalf("nonce did not change when base SHA changed: %q", movedBase)
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runGitOutputForTest(t, dir, args...)
}

func runGitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func TestFactoryDaemonIssueScanIntervalRequiresRepoBeforeStart(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-interval", "1m"})
	if err == nil || !strings.Contains(err.Error(), "--registry") {
		t.Fatalf("expected missing issue-scan repo or registry error, got %v", err)
	}
}

func TestFactoryDaemonIssueScanIntervalRejectsAutoApproveRequests(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-interval", "1m", "--issue-scan-repo", "transpara-ai/hive", "--approve-requests"})
	if err == nil || !strings.Contains(err.Error(), "--approve-requests") {
		t.Fatalf("expected auto-approval guard error, got %v", err)
	}
}

func TestFactoryDaemonIssueScanIntervalRejectsAutoApproveRoles(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-interval", "1m", "--issue-scan-repo", "transpara-ai/hive", "--approve-roles"})
	if err == nil || !strings.Contains(err.Error(), "--approve-roles") {
		t.Fatalf("expected auto-role guard error, got %v", err)
	}
}

func TestFactoryDaemonIssueScanIntervalRequiresPositiveMaxNewRuns(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "daemon", "--human", "Michael", "--issue-scan-interval", "1m", "--issue-scan-repo", "transpara-ai/hive", "--issue-scan-max-new-runs", "0"})
	if err == nil || !strings.Contains(err.Error(), "--issue-scan-max-new-runs") {
		t.Fatalf("expected max-new-runs guard error, got %v", err)
	}
}

func TestRunIssueScanStageRoleOutputRunnerPassesContextAndParsesResult(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	runner := filepath.Join(dir, "runner.sh")
	script := `#!/bin/sh
cat > "$1"
printf '%s\n' '{"role_outputs":[{"role":"strategist","summary":"selected issue is high priority","evidence_refs":["artifact://stage-role/strategist"],"outputs":[{"key":"issue_priority_rationale","summary":"issue is in scope","evidence_refs":["artifact://stage-role/strategist/priority"]},{"key":"risk_and_scope_notes","summary":"bounded to planning","evidence_refs":["artifact://stage-role/strategist/scope"]}]}]}'
`
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	runnerContext := hive.IssueScanStageRoleOutputRunnerContext{
		Kind:             "issue_scan_stage_role_output_runner_context",
		LifecycleVersion: "civilization_issue_to_human_ready_pr_v0.8",
		RunID:            "run_issue_001",
		FactoryOrderID:   "fo_run_issue_001",
		Repository:       "transpara-ai/hive",
		StageID:          "research_issue_and_repo_context",
		StageTaskID:      "01911111-1111-7111-8111-111111111111",
	}

	result, err := runIssueScanStageRoleOutputRunner(context.Background(), runner, []string{contextPath}, runnerContext, time.Second)
	if err != nil {
		t.Fatalf("runIssueScanStageRoleOutputRunner: %v", err)
	}
	if len(result.RoleOutputs) != 1 || result.RoleOutputs[0].Role != "strategist" || len(result.RoleOutputs[0].Outputs) != 2 {
		t.Fatalf("result = %+v", result)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read runner context: %v", err)
	}
	var sent hive.IssueScanStageRoleOutputRunnerContext
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode runner context: %v", err)
	}
	if sent.RunID != runnerContext.RunID || sent.StageID != runnerContext.StageID || sent.Repository != runnerContext.Repository {
		t.Fatalf("sent context = %+v, want run/stage/repo from %+v", sent, runnerContext)
	}
}

func TestRequireIssueScanStageRoleOutputRunnerOutputsRejectsEmpty(t *testing.T) {
	err := requireIssueScanStageRoleOutputRunnerOutputs("research_issue_and_repo_context", nil)
	if err == nil || !strings.Contains(err.Error(), "no role outputs") {
		t.Fatalf("expected empty role-output guard error, got %v", err)
	}
}

func TestRunIssueScanImplementationRunnerPassesContextAndParsesResult(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	runner := filepath.Join(dir, "runner.sh")
	script := `#!/bin/sh
cat > "$1"
printf '%s\n' '{"operate_result_body":"branch: codex/run-issue-001\ncommit: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\nrange: origin/main..HEAD\n\npkg/hive/issue_scan.go | 12 ++++++++++++","completion_summary":"validation output: go test ./pkg/hive passed"}'
`
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	runnerContext := hive.IssueScanImplementationRunnerContext{
		Kind:                      "issue_scan_implementation_runner_context",
		LifecycleVersion:          "civilization_issue_to_human_ready_pr_v0.8",
		RunID:                     "run_issue_001",
		FactoryOrderID:            "fo_run_issue_001",
		Repository:                "transpara-ai/hive",
		RepoPath:                  dir,
		ImplementationTaskID:      "01911111-1111-7111-8111-111111111111",
		ImplementationStageTaskID: "01922222-2222-7222-8222-222222222222",
	}

	result, err := runIssueScanImplementationRunner(context.Background(), runner, []string{contextPath}, runnerContext, time.Second)
	if err != nil {
		t.Fatalf("runIssueScanImplementationRunner: %v", err)
	}
	if !strings.Contains(result.OperateResultBody, "branch: codex/run-issue-001") || !strings.Contains(result.CompletionSummary, "go test ./pkg/hive passed") {
		t.Fatalf("result = %+v", result)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read runner context: %v", err)
	}
	var sent hive.IssueScanImplementationRunnerContext
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode runner context: %v", err)
	}
	if sent.RunID != runnerContext.RunID || sent.ImplementationTaskID != runnerContext.ImplementationTaskID || sent.Repository != runnerContext.Repository {
		t.Fatalf("sent context = %+v, want run/task/repo from %+v", sent, runnerContext)
	}
}

func TestRunIssueScanBlockerRepairRunnerPassesContextAndParsesResult(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	runner := filepath.Join(dir, "runner.sh")
	script := `#!/bin/sh
cat > "$1"
printf '%s\n' '{"operate_result_body":"branch: codex/run-issue-001-repair\ncommit: cccccccccccccccccccccccccccccccccccccccc\nrange: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb..cccccccccccccccccccccccccccccccccccccccc\n\npkg/hive/issue_scan.go | 12 ++++++++++++","completion_summary":"validation output: go test ./pkg/hive passed after blocker repair"}'
`
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	runnerContext := hive.IssueScanBlockerRepairRunnerContext{
		Kind:                        "issue_scan_blocker_repair_runner_context",
		LifecycleVersion:            "civilization_issue_to_human_ready_pr_v0.8",
		RunID:                       "run_issue_001",
		FactoryOrderID:              "fo_run_issue_001",
		Repository:                  "transpara-ai/hive",
		RepoPath:                    dir,
		ImplementationTaskID:        "01911111-1111-7111-8111-111111111111",
		BlockerStageTaskID:          "01922222-2222-7222-8222-222222222222",
		RequestChangesReviewEventID: "01933333-3333-7333-8333-333333333333",
		RequestChangesReviewIssues:  []string{"missing regression test"},
		PreviousOperateCommit:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	result, err := runIssueScanBlockerRepairRunner(context.Background(), runner, []string{contextPath}, runnerContext, time.Second)
	if err != nil {
		t.Fatalf("runIssueScanBlockerRepairRunner: %v", err)
	}
	if !strings.Contains(result.OperateResultBody, "codex/run-issue-001-repair") || !strings.Contains(result.CompletionSummary, "blocker repair") {
		t.Fatalf("result = %+v", result)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read runner context: %v", err)
	}
	var sent hive.IssueScanBlockerRepairRunnerContext
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode runner context: %v", err)
	}
	if sent.RunID != runnerContext.RunID || sent.RequestChangesReviewEventID != runnerContext.RequestChangesReviewEventID || sent.PreviousOperateCommit != runnerContext.PreviousOperateCommit {
		t.Fatalf("sent context = %+v, want repair context from %+v", sent, runnerContext)
	}
}

func TestRunIssueScanReviewRunnerPassesContextAndParsesReceipt(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	runner := filepath.Join(dir, "runner.sh")
	script := `#!/bin/sh
cat > "$1"
printf '%s\n' '{"repository":"transpara-ai/hive","review_ref":"artifact://review/result.md","reviewed_head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","verdict":"approve","summary":"no blockers","issues":[],"confidence":0.91}'
`
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	reviewContext := hive.IssueScanAdversarialReviewContext{
		Kind:                 "issue_scan_adversarial_review_context",
		LifecycleVersion:     "civilization_issue_to_human_ready_pr_v0.8",
		RunID:                "run_issue_001",
		FactoryOrderID:       "fo_run_issue_001",
		Repository:           "transpara-ai/hive",
		ImplementationTaskID: "evt_task",
		OperateCommit:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	receipt, err := runIssueScanReviewRunner(context.Background(), runner, []string{contextPath}, reviewContext, time.Second)
	if err != nil {
		t.Fatalf("runIssueScanReviewRunner: %v", err)
	}
	if receipt.ReviewRef != "artifact://review/result.md" || receipt.Verdict != "approve" || receipt.ReviewedHeadSHA != reviewContext.OperateCommit {
		t.Fatalf("receipt = %+v", receipt)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read runner context: %v", err)
	}
	var sent hive.IssueScanAdversarialReviewContext
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode runner context: %v", err)
	}
	if sent.RunID != reviewContext.RunID || sent.OperateCommit != reviewContext.OperateCommit {
		t.Fatalf("sent context = %+v, want run/head from %+v", sent, reviewContext)
	}
}

func TestRunIssueScanReadyPRRunnerPassesContextAndParsesResult(t *testing.T) {
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	runner := filepath.Join(dir, "runner.sh")
	script := `#!/bin/sh
cat > "$1"
printf '%s\n' '{"draft_pr_receipt":{"kind":"transpara_ai_draft_pr_receipt","repository":"transpara-ai/hive","pr_number":321,"pr_url":"https://github.com/transpara-ai/hive/pull/321","base_ref":"main","base_sha":"dddddddddddddddddddddddddddddddddddddddd","head_ref":"codex/run-issue-001","head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","remote_head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","changed_files":["README.md"],"draft":true,"state":"open","policy_bundle_id":"transpara-ai-issue-scan-draft-pr-create-only-v0.1","policy_bundle_hash":"test","authority_nonce":"nonce","human_approval_required":true,"no_merge_or_deploy_claim":true,"ready_for_review_required":true},"ready_pr_evidence":{"repository":"transpara-ai/hive","pr_number":321,"pr_url":"https://github.com/transpara-ai/hive/pull/321","head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","state":"open","draft":false,"ready_for_review":true,"merge_state_status":"clean","ci_status":"success","ready_state_review_ref":"https://github.com/transpara-ai/hive/pull/321#issuecomment-ready","ready_state_review_status":"passed","human_approval_required":true}}'
`
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	readyContext := hive.IssueScanReadyPRRunnerContext{
		Kind:             "issue_scan_ready_pr_runner_context",
		LifecycleVersion: "civilization_issue_to_human_ready_pr_v0.8",
		RunID:            "run_issue_001",
		FactoryOrderID:   "fo_run_issue_001",
		Repository:       "transpara-ai/hive",
		OperateCommit:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	result, err := runIssueScanReadyPRRunner(context.Background(), runner, []string{contextPath}, readyContext, time.Second)
	if err != nil {
		t.Fatalf("runIssueScanReadyPRRunner: %v", err)
	}
	if result.DraftPRReceipt.PRNumber != 321 || result.ReadyPREvidence.PRNumber != 321 || result.ReadyPREvidence.HeadSHA != readyContext.OperateCommit {
		t.Fatalf("result = %+v", result)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read runner context: %v", err)
	}
	var sent hive.IssueScanReadyPRRunnerContext
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode runner context: %v", err)
	}
	if sent.RunID != readyContext.RunID || sent.OperateCommit != readyContext.OperateCommit {
		t.Fatalf("sent context = %+v, want run/head from %+v", sent, readyContext)
	}
}

func TestRunIssueScanReadyStateReviewRunnerScrubsGitHubTokenAndParsesReceipt(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "write-capable-token")
	t.Setenv("GH_TOKEN", "alternate-write-capable-token")
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	envPath := filepath.Join(dir, "env.txt")
	runner := filepath.Join(dir, "runner.sh")
	script := `#!/bin/sh
cat > "$1"
env > "$2"
printf '%s\n' '{"review_ref":"artifact://ready-state/review.md","reviewed_head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","status":"passed","summary":"no blockers"}'
`
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	reviewContext := hive.IssueScanReadyStateReviewContext{
		Kind:             "issue_scan_ready_state_review_context",
		LifecycleVersion: "civilization_issue_to_human_ready_pr_v0.8",
		RunID:            "run_issue_001",
		FactoryOrderID:   "fo_run_issue_001",
		Repository:       "transpara-ai/hive",
		PRNumber:         321,
		PRURL:            "https://github.com/transpara-ai/hive/pull/321",
		OperateCommit:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}

	receipt, err := runIssueScanReadyStateReviewRunner(context.Background(), runner, []string{contextPath, envPath}, reviewContext, time.Second)
	if err != nil {
		t.Fatalf("runIssueScanReadyStateReviewRunner: %v", err)
	}
	if receipt.ReviewRef != "artifact://ready-state/review.md" || receipt.Status != "passed" || receipt.ReviewedHeadSHA != reviewContext.OperateCommit {
		t.Fatalf("receipt = %+v", receipt)
	}
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read runner context: %v", err)
	}
	var sent hive.IssueScanReadyStateReviewContext
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decode runner context: %v", err)
	}
	if sent.RunID != reviewContext.RunID || sent.PRNumber != reviewContext.PRNumber || sent.OperateCommit != reviewContext.OperateCommit {
		t.Fatalf("sent context = %+v, want run/PR/head from %+v", sent, reviewContext)
	}
	envRaw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read runner env: %v", err)
	}
	envText := string(envRaw)
	if strings.Contains(envText, "GITHUB_TOKEN=") || strings.Contains(envText, "GH_TOKEN=") {
		t.Fatalf("ready-state review runner inherited GitHub token env:\n%s", envText)
	}
	for _, want := range []string{
		"HIVE_ISSUE_SCAN_READY_STATE_REVIEW_CONTEXT=stdin",
		"HIVE_ISSUE_SCAN_RUN_ID=" + reviewContext.RunID,
		"HIVE_ISSUE_SCAN_REPOSITORY=" + reviewContext.Repository,
		"HIVE_ISSUE_SCAN_PR_NUMBER=321",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf("runner env missing %q:\n%s", want, envText)
		}
	}
}

func TestResolveIssueScanReposLoadsTransparaAIReposFromRegistry(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(registryPath, []byte(`{
		"repos": [
			{"name": "site", "url": "https://github.com/transpara-ai/site"},
			{"name": "hive", "url": "git@github.com:transpara-ai/hive.git"},
			{"name": "hive-copy", "url": "https://github.com/transpara-ai/hive.git"},
			{"name": "outside", "url": "https://github.com/example/outside"},
			{"name": "work"}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write repos.json: %v", err)
	}

	repos, err := resolveIssueScanRepos(nil, true, registryPath)
	if err != nil {
		t.Fatalf("resolveIssueScanRepos: %v", err)
	}
	got := strings.Join(repos, ",")
	want := "transpara-ai/site,transpara-ai/hive,transpara-ai/work"
	if got != want {
		t.Fatalf("repos = %q, want %q", got, want)
	}
}

func TestResolveIssueScanReposPrefersExplicitReposOverRegistry(t *testing.T) {
	repos, err := resolveIssueScanRepos([]string{"transpara-ai/hive", "transpara-ai/hive"}, true, "/does/not/exist")
	if err != nil {
		t.Fatalf("resolveIssueScanRepos explicit: %v", err)
	}
	if got := strings.Join(repos, ","); got != "transpara-ai/hive" {
		t.Fatalf("repos = %q, want explicit hive only", got)
	}
}

func TestResolveIssueScanReposRequiresRepoOrRegistry(t *testing.T) {
	_, err := resolveIssueScanRepos(nil, false, "")
	if err == nil || !strings.Contains(err.Error(), "--registry") {
		t.Fatalf("expected repo or registry error, got %v", err)
	}
}

func TestRunIssueScanScannerCycleQueuesOnceAndSkipsExistingIntake(t *testing.T) {
	ctx := context.Background()
	rt, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "", testIssueScanOpenTargetStateResolverOption)
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)

	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {{
				Repo:   "transpara-ai/hive",
				Number: 192,
				Title:  "Teach Civilization to scan Transpara-AI repos",
				URL:    "https://github.com/transpara-ai/hive/issues/192",
				Labels: []string{"civilization", "cc:pr-ready"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}

	first, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("first runIssueScanScannerCycle: %v", err)
	}
	if !first.Queued || first.QueuedRunID == "" || first.QueuedIssue.Number != 192 {
		t.Fatalf("first cycle = %+v, want queued issue 192", first)
	}
	progress, err := rt.ProgressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("ProgressIssueScanLifecycle: %v", err)
	}
	if progress.Dispatch.Dispatched != 1 {
		t.Fatalf("dispatch = %+v, want one dispatched FactoryOrder", progress.Dispatch)
	}

	second, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("second runIssueScanScannerCycle: %v", err)
	}
	if second.Queued {
		t.Fatalf("second cycle queued duplicate: %+v", second)
	}
	if second.SkippedExisting != 1 {
		t.Fatalf("second skipped existing = %d, want 1", second.SkippedExisting)
	}
}

func TestRunIssueScanScannerCycleQueuedRunCanBeRedrivenByRuntimeProgress(t *testing.T) {
	ctx := context.Background()
	rt, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "", testIssueScanOpenTargetStateResolverOption)
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)

	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {{
				Repo:   "transpara-ai/hive",
				Number: 193,
				Title:  "Redrive queued issue-scan run",
				URL:    "https://github.com/transpara-ai/hive/issues/193",
				Labels: []string{"cc:pr-ready"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}

	queuedOnly, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("queued-only runIssueScanScannerCycle: %v", err)
	}
	if !queuedOnly.Queued {
		t.Fatalf("queued-only cycle = %+v, want queued without inline dispatch", queuedOnly)
	}

	progress, err := rt.ProgressIssueScanLifecycle()
	if err != nil {
		t.Fatalf("ProgressIssueScanLifecycle: %v", err)
	}
	if progress.Dispatch.Dispatched != 1 {
		t.Fatalf("redrive dispatch = %+v, want one dispatched FactoryOrder", progress.Dispatch)
	}

	second, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("second runIssueScanScannerCycle: %v", err)
	}
	if second.Queued || second.SkippedExisting != 1 {
		t.Fatalf("second cycle = %+v, want skipped existing queued run", second)
	}
}

func TestRunIssueScanScannerCycleSkipsExistingSourceWhenIntakeMissing(t *testing.T) {
	ctx := context.Background()
	_, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)

	content := hive.FactoryRunRequestedContent{
		RunID:    "run_existing_source_only",
		IntakeID: "",
		Status:   "queued",
		Sources: []hive.RunLaunchSource{{
			ID:    "github_issue_transpara_ai_docs_172",
			Type:  "github.issue",
			Ref:   "https://github.com/transpara-ai/docs/issues/172",
			Title: "transpara-ai/docs#172",
		}},
		Brief: json.RawMessage(`{"kind":"transpara_ai_github_issue_scan"}`),
	}
	ev, err := fc.factory.Create(hive.EventTypeFactoryRunRequested, fc.humanID, content, fc.headCauses(), factoryOrderConversation("existing_source_only"), fc.store, fc.signer)
	if err != nil {
		t.Fatalf("create factory.run.requested: %v", err)
	}
	if _, err := fc.store.Append(ev); err != nil {
		t.Fatalf("append factory.run.requested: %v", err)
	}

	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/docs": {{
				Repo:   "transpara-ai/docs",
				Number: 172,
				Title:  "Gate S closeout PR #171 approval artifact residual",
				URL:    "https://github.com/transpara-ai/docs/issues/172",
				Labels: []string{"cc:pr-ready"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/docs"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}

	result, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("runIssueScanScannerCycle: %v", err)
	}
	if result.Queued || result.SkippedExisting != 1 {
		t.Fatalf("cycle = %+v, want source-ref dedupe skip", result)
	}
}

func TestRunIssueScanScannerCycleIgnoresCanaryParkedEvidenceForQueueDedupe(t *testing.T) {
	ctx := context.Background()
	_, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)

	issue := hive.GitHubIssueCandidate{
		Repo:   "transpara-ai/hive",
		Number: 206,
		Title:  "Canary parked issue later becomes PR-ready",
		URL:    "https://github.com/transpara-ai/hive/issues/206",
		State:  "open",
		Labels: []string{"cc:intake"},
	}
	if _, err := recordCanaryIssueParked(
		ctx,
		fc,
		issue,
		"level1_canary_transpara-ai_hive_206_0000",
		hive.IssueScanParkBlockerNotPRReady,
		"transpara-ai/hive#206 lacks cc:pr-ready",
		"human must mark the issue PR-ready before Hive may continue",
	); err != nil {
		t.Fatalf("recordCanaryIssueParked: %v", err)
	}

	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {{
				Repo:   "transpara-ai/hive",
				Number: 206,
				Title:  "Canary parked issue later becomes PR-ready",
				URL:    "https://github.com/transpara-ai/hive/issues/206",
				State:  "open",
				Labels: []string{"cc:intake", "cc:pr-ready"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}

	result, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("runIssueScanScannerCycle: %v", err)
	}
	if !result.Queued || result.QueuedIssue.Number != 206 || result.SkippedExisting != 0 {
		t.Fatalf("cycle = %+v, want canary parked evidence ignored for queue dedupe", result)
	}
}

func TestRunIssueScanScannerCycleSkipsNonPRReadyIssues(t *testing.T) {
	ctx := context.Background()
	_, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)

	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {{
				Repo:   "transpara-ai/hive",
				Number: 204,
				Title:  "Human-required value-allocation surface candidate",
				URL:    "https://github.com/transpara-ai/hive/issues/204",
				Labels: []string{"cc:intake", "cc:needs-human-scope", "cc:protected-action"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}

	result, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("runIssueScanScannerCycle: %v", err)
	}
	if result.Queued {
		t.Fatalf("cycle queued non-PR-ready issue: %+v", result)
	}
	if result.ScannedIssues != 1 || result.SkippedNotPRReady != 1 {
		t.Fatalf("cycle = %+v, want one scanned non-PR-ready skip", result)
	}
}

func TestRunIssueScanScannerCycleQueuesOnlyPRReadyIssue(t *testing.T) {
	ctx := context.Background()
	_, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
	if err != nil {
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)

	lister := fakeIssueScanLister{
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {
				{
					Repo:   "transpara-ai/hive",
					Number: 204,
					Title:  "Human-required value-allocation surface candidate",
					URL:    "https://github.com/transpara-ai/hive/issues/204",
					Labels: []string{"cc:intake", "cc:needs-human-scope", "cc:protected-action"},
				},
				{
					Repo:   "transpara-ai/hive",
					Number: 205,
					Title:  "PR-ready issue-scan hardening",
					URL:    "https://github.com/transpara-ai/hive/issues/205",
					Labels: []string{"cc:intake", "cc:pr-ready"},
				},
			},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
	}

	result, err := runIssueScanScannerCycle(ctx, scannerContext, config, lister)
	if err != nil {
		t.Fatalf("runIssueScanScannerCycle: %v", err)
	}
	if !result.Queued || result.QueuedIssue.Number != 205 {
		t.Fatalf("cycle = %+v, want queued PR-ready issue 205", result)
	}
	if result.SkippedNotPRReady != 1 {
		t.Fatalf("skipped non-PR-ready = %d, want 1", result.SkippedNotPRReady)
	}
}

func TestIssueScanCandidatePRReadyRequiresPositiveReadyLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   bool
	}{
		{name: "ready", labels: []string{"cc:intake", "cc:pr-ready"}, want: true},
		{name: "missing ready", labels: []string{"cc:intake"}, want: false},
		{name: "deferred wins", labels: []string{"cc:pr-ready", "cc:pr-deferred"}, want: false},
		{name: "needs human scope wins", labels: []string{"cc:pr-ready", "cc:needs-human-scope"}, want: false},
		{name: "protected action wins", labels: []string{"cc:pr-ready", "cc:protected-action"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hive.IssueScanCandidatePRReady(hive.GitHubIssueCandidate{Labels: tt.labels})
			if got != tt.want {
				t.Fatalf("issueScanCandidatePRReady(%v) = %v, want %v", tt.labels, got, tt.want)
			}
		})
	}
}

func TestParseGitHubIssueCandidatesMapsLabels(t *testing.T) {
	raw := []byte(`[
		{
			"number": 205,
			"title": "PR-ready issue-scan hardening",
			"url": "https://github.com/transpara-ai/hive/issues/205",
			"body": "ready to implement",
			"labels": [
				{"name": "cc:intake"},
				{"name": "cc:pr-ready"}
			]
		}
	]`)

	issues, err := parseGitHubIssueCandidates("transpara-ai/hive", raw)
	if err != nil {
		t.Fatalf("parseGitHubIssueCandidates: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues length = %d, want 1", len(issues))
	}
	got := strings.Join(issues[0].Labels, ",")
	if got != "cc:intake,cc:pr-ready" {
		t.Fatalf("labels = %q, want cc:intake,cc:pr-ready", got)
	}
	if !hive.IssueScanCandidatePRReady(issues[0]) {
		t.Fatalf("parsed issue should be PR-ready: %+v", issues[0])
	}
}

func TestParseGitHubIssueTargetStateMapsLabelsAndClosedState(t *testing.T) {
	raw := []byte(`{
		"number": 225,
		"url": "https://github.com/transpara-ai/hive/issues/225",
		"state": "CLOSED",
		"stateReason": "COMPLETED",
		"labels": [
			{"name": "cc:pr-ready"},
			{"name": "cc:needs-human-scope"}
		]
	}`)

	state, err := parseGitHubIssueTargetState("transpara-ai/hive", raw)
	if err != nil {
		t.Fatalf("parseGitHubIssueTargetState: %v", err)
	}
	if state.Repository != "transpara-ai/hive" || state.Number != 225 || state.URL == "" {
		t.Fatalf("state identity = %+v", state)
	}
	if state.State != "closed" || state.StateReason != "completed" {
		t.Fatalf("state = %q reason = %q; want closed/completed", state.State, state.StateReason)
	}
	if got := strings.Join(state.Labels, ","); got != "cc:pr-ready,cc:needs-human-scope" {
		t.Fatalf("labels = %q", got)
	}
}

func TestFactoryScanIssuesRejectsNonPRReadyGitHubIssueBeforeFactoryOpen(t *testing.T) {
	dir := t.TempDir()
	ghPath := filepath.Join(dir, "gh")
	script := `#!/bin/sh
printf '%s\n' '[{"number":204,"title":"Human-required value-allocation surface candidate","url":"https://github.com/transpara-ai/hive/issues/204","body":"needs human scope","labels":[{"name":"cc:intake"},{"name":"cc:needs-human-scope"},{"name":"cc:protected-action"}]}]'
`
	if err := os.WriteFile(ghPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := routeAndDispatch([]string{"factory", "scan-issues", "--human", "Michael", "--repo", "transpara-ai/hive", "--label", "cc:intake", "--limit", "1"})
	if err == nil || !strings.Contains(err.Error(), "no PR-ready GitHub issues found") {
		t.Fatalf("expected no PR-ready issue error, got %v", err)
	}
	if !strings.Contains(err.Error(), "skipped 1 non-PR-ready") {
		t.Fatalf("expected skipped count in error, got %v", err)
	}
}

func TestRunIssueScanScannerLoopStopsOnCancelAndHonorsMaxNewRunCap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
	if err != nil {
		cancel()
		t.Fatalf("openFactoryRuntime: %v", err)
	}
	defer fc.close()
	scannerContext := newIssueScanScannerContext(fc.store, fc.actors, fc.humanID)
	lister := &countingIssueScanLister{
		first: make(chan struct{}),
		issues: map[string][]hive.GitHubIssueCandidate{
			"transpara-ai/hive": {{
				Repo:   "transpara-ai/hive",
				Number: 194,
				Title:  "Loop scanner cap",
				URL:    "https://github.com/transpara-ai/hive/issues/194",
				Labels: []string{"cc:pr-ready"},
			}},
		},
	}
	config := issueScanScannerConfig{
		OperatorID:    hive.IssueScanOperatorID("Michael"),
		Repos:         []string{"transpara-ai/hive"},
		Limit:         10,
		MaxIterations: 30,
		MaxCostUSD:    25,
		MaxNewRuns:    1,
		Interval:      time.Millisecond,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runIssueScanScannerLoop(ctx, scannerContext, config, lister)
	}()
	select {
	case <-lister.first:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("scanner did not perform initial scan")
	}
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop after cancellation")
	}
	if calls := lister.Calls(); calls != 1 {
		t.Fatalf("scan calls = %d, want 1 because max_new_runs caps later ticks", calls)
	}
}

func TestIssueScanRegistryPathRejectsUntrustedCWD(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := issueScanRegistryPath()
	if err == nil || !strings.Contains(err.Error(), "agents directory") {
		t.Fatalf("expected untrusted cwd registry path error, got %v", err)
	}
}

func TestIssueScanRegistryPathAcceptsExactHiveModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "agents"), 0o700); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/transpara-ai/hive\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	t.Chdir(dir)

	got, err := issueScanRegistryPath()
	if err != nil {
		t.Fatalf("issueScanRegistryPath: %v", err)
	}
	if got != filepath.Join(dir, "repos.json") {
		t.Fatalf("registry path = %q, want %q", got, filepath.Join(dir, "repos.json"))
	}
}

func TestIssueScanRegistryPathRejectsMissingGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "agents"), 0o700); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	t.Chdir(dir)

	_, err := issueScanRegistryPath()
	if err == nil || !strings.Contains(err.Error(), "read go.mod") {
		t.Fatalf("expected missing go.mod error, got %v", err)
	}
}

func TestIssueScanRegistryPathRejectsLookalikeModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "agents"), 0o700); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/transpara-ai/hive-evil\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	t.Chdir(dir)

	_, err := issueScanRegistryPath()
	if err == nil || !strings.Contains(err.Error(), "not the Hive repo root") {
		t.Fatalf("expected lookalike module rejection, got %v", err)
	}
}

func TestIssueScanRepoSlugFromRegistryRepoNormalizesGitHubURL(t *testing.T) {
	tests := map[string]string{
		"https://github.com/transpara-ai/site":           "transpara-ai/site",
		"HTTPS://github.com/transpara-ai/site.git/":      "transpara-ai/site",
		"https://github.com/transpara-ai/site.git/.git":  "transpara-ai/site",
		"https://github.com/transpara-ai/site.git/.git/": "transpara-ai/site",
		"git@github.com:transpara-ai/hive.git":           "transpara-ai/hive",
		"ssh://git@github.com/transpara-ai/work":         "transpara-ai/work",
		"http://github.com/transpara-ai/agent.git":       "transpara-ai/agent",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got := issueScanRepoSlugFromRegistryRepo(registryRepoForTest(raw))
			if got != want {
				t.Fatalf("slug = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveIssueScanReposRejectsEmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(registryPath, []byte(`{
		"repos": [
			{"name": "outside", "url": "https://github.com/example/outside"},
			{"name": "subpath", "url": "https://github.com/transpara-ai/hive/tree/main"}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write repos.json: %v", err)
	}

	_, err := resolveIssueScanRepos(nil, true, registryPath)
	if err == nil || !strings.Contains(err.Error(), "no scannable Transpara-AI GitHub repos") {
		t.Fatalf("expected empty registry error, got %v", err)
	}
}

func TestResolveIssueScanReposFailsClosedWhenRegistryMissing(t *testing.T) {
	_, err := resolveIssueScanRepos(nil, true, filepath.Join(t.TempDir(), "missing-repos.json"))
	if err == nil || !strings.Contains(err.Error(), "load issue-scan repo registry") {
		t.Fatalf("expected registry load error, got %v", err)
	}
}

func TestResolveIssueScanReposFailsClosedWhenRegistryMalformed(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(registryPath, []byte(`{"repos": [`), 0o600); err != nil {
		t.Fatalf("write repos.json: %v", err)
	}

	_, err := resolveIssueScanRepos(nil, true, registryPath)
	if err == nil || !strings.Contains(err.Error(), "load issue-scan repo registry") {
		t.Fatalf("expected registry parse error, got %v", err)
	}
}

func TestNormalizeIssueScanReposRejectsOutsideOrg(t *testing.T) {
	_, err := normalizeIssueScanRepos([]string{"example/hive"})
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("expected transpara-ai repo rejection, got %v", err)
	}
}

func TestNormalizeIssueScanReposRejectsUnsafeRepoBeforeGitHub(t *testing.T) {
	_, err := normalizeIssueScanRepos([]string{"transpara-ai/../hive"})
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("expected unsafe repo rejection, got %v", err)
	}
}

func TestNormalizeIssueScanReposRejectsMultiSegmentSlug(t *testing.T) {
	_, err := normalizeIssueScanRepos([]string{"transpara-ai/hive/tree/main"})
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("expected multi-segment repo rejection, got %v", err)
	}
}

func TestSafeIssueScanOperatorIDRejectsWhitespace(t *testing.T) {
	if safeIssueScanOperatorID("operator michael") {
		t.Fatal("operator id with whitespace was accepted")
	}
}

func TestFactoryScanIssuesParsesModelOverridesBeforeGitHub(t *testing.T) {
	err := routeAndDispatch([]string{
		"factory", "scan-issues",
		"--human", "Michael",
		"--repo", "transpara-ai/hive",
		"--model", "implementer=codex",
		"--model", "implementer=o3",
	})
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate model override error before GitHub, got %v", err)
	}
}

func registryRepoForTest(url string) hiveregistry.Repo {
	return hiveregistry.Repo{URL: url}
}

func TestParseFactoryOrderModelOverrideFlags(t *testing.T) {
	flags := factoryOrderModelOverrideFlags{}
	if err := flags.models.Set("guardian=api-sonnet"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := flags.authModes.Set("guardian=api-key"); err != nil {
		t.Fatalf("set auth-mode: %v", err)
	}
	if err := flags.requiredCapabilities.Set("guardian=reasoning,coding"); err != nil {
		t.Fatalf("set required capability: %v", err)
	}
	if err := flags.maxCosts.Set("guardian=0.25"); err != nil {
		t.Fatalf("set max cost: %v", err)
	}

	overrides, err := parseFactoryOrderModelOverrideFlags(flags)
	if err != nil {
		t.Fatalf("parseFactoryOrderModelOverrideFlags: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", overrides)
	}
	override := overrides[0]
	if override.Role != "guardian" || override.Model != "api-sonnet" || override.AuthMode != "api-key" {
		t.Fatalf("override = %+v, want guardian api-sonnet api-key", override)
	}
	if got := strings.Join(override.RequiredCapabilities, ","); got != "reasoning,coding" {
		t.Fatalf("required capabilities = %q, want reasoning,coding", got)
	}
	if override.MaxCostPerCallUSD == nil || *override.MaxCostPerCallUSD != 0.25 {
		t.Fatalf("max cost = %v, want 0.25", override.MaxCostPerCallUSD)
	}
}

func TestParseFactoryOrderModelOverrideFlagsRejectsDuplicateScalar(t *testing.T) {
	flags := factoryOrderModelOverrideFlags{}
	_ = flags.models.Set("guardian=sonnet")
	_ = flags.models.Set("guardian=opus")

	_, err := parseFactoryOrderModelOverrideFlags(flags)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate scalar error, got %v", err)
	}
}

func TestValidateFactoryOrderModelOverridesPreservesCanOperateGuardrail(t *testing.T) {
	_, err := validateFactoryOrderModelOverrides("", []hive.ModelOverrideRequest{
		{Role: "implementer", Model: "api-sonnet", AuthMode: "api-key"},
	})
	if err == nil || !strings.Contains(err.Error(), "CanOperate") {
		t.Fatalf("expected CanOperate guardrail error, got %v", err)
	}
}

func TestValidateFactoryOrderModelOverridesAcceptsCodexForCanOperate(t *testing.T) {
	overrides, err := validateFactoryOrderModelOverrides("", []hive.ModelOverrideRequest{
		{Role: "implementer", Model: "codex"},
	})
	if err != nil {
		t.Fatalf("validateFactoryOrderModelOverrides: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", overrides)
	}
	override := overrides[0]
	if override.Role != "implementer" || override.ResolvedModel != "gpt-5.5" || override.ResolvedProvider != "codex-cli" || override.AuthMode != "subscription" {
		t.Fatalf("override = %+v, want implementer codex-cli/gpt-5.5 subscription", override)
	}
}

func TestFactoryMixedCatalogCodexImplementerCanOperate(t *testing.T) {
	source, err := factoryModelSelectionSource(filepath.Join("..", "..", "catalog-mixed.yaml"))
	if err != nil {
		t.Fatalf("factoryModelSelectionSource: %v", err)
	}
	config := source()
	resolved, err := config.Resolver.Resolve(modelconfig.ResolutionInput{
		Role:       "implementer",
		CanOperate: true,
	})
	if err != nil {
		t.Fatalf("resolve mixed catalog implementer: %v", err)
	}
	if resolved.Provider != "codex-cli" || resolved.Model != "gpt-5.5" || resolved.AuthMode != modelconfig.AuthSubscription {
		t.Fatalf("resolved = %+v, want codex-cli/gpt-5.5 subscription", resolved)
	}
	if missing := modelconfig.ValidateCapabilities(resolved.Entry, []modelconfig.Capability{modelconfig.CapOperate}); len(missing) > 0 {
		t.Fatalf("resolved implementer missing operate capability: %+v", missing)
	}
}

func TestValidateFactoryOrderModelOverridesSkipsCatalogWithoutOverrides(t *testing.T) {
	overrides, err := validateFactoryOrderModelOverrides(filepath.Join(t.TempDir(), "missing-model-catalog.yaml"), nil)
	if err != nil {
		t.Fatalf("validateFactoryOrderModelOverrides without overrides: %v", err)
	}
	if overrides != nil {
		t.Fatalf("overrides = %+v, want nil", overrides)
	}
}

func TestValidateFactoryOrderModelOverridesAcceptsAuthModeOnlyOverride(t *testing.T) {
	overrides, err := validateFactoryOrderModelOverrides("", []hive.ModelOverrideRequest{
		{Role: "guardian", AuthMode: "subscription"},
	})
	if err != nil {
		t.Fatalf("validateFactoryOrderModelOverrides: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", overrides)
	}
	override := overrides[0]
	if override.Role != "guardian" || override.RequestedAuthMode != "subscription" || override.AuthMode != "subscription" {
		t.Fatalf("override = %+v, want guardian subscription auth-mode", override)
	}
}

// TestFactoryUnknownSubverb asserts an unrecognized subcommand is reported.
func TestFactoryUnknownSubverb(t *testing.T) {
	err := cmdFactory([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got %v", err)
	}
}

// TestFactoryRequestPRRequiresRepo asserts request-pr validates required flags
// before opening a store or touching GitHub. With no --repo the command must
// fail fast with a flag-name error.
func TestFactoryRequestPRRequiresRepo(t *testing.T) {
	err := cmdFactory([]string{"request-pr"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing --repo error, got %v", err)
	}
}

// TestFactoryRequestPRRequiresFlags asserts that request-pr validates all
// required flags before any store open or network access. requireFlags lists
// --repo first, so calling request-pr with NO flags must produce an error
// mentioning "repo" — the very first missing required flag.
func TestFactoryRequestPRRequiresFlags(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "request-pr"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing-flag error mentioning repo, got %v", err)
	}
}

func TestFactoryRequestIssueScanPRRequiresHuman(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "request-issue-scan-pr"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing-flag error mentioning human, got %v", err)
	}
}

func TestFactoryCreateIssueScanDraftPRRequiresHuman(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "create-issue-scan-draft-pr"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing-flag error mentioning human, got %v", err)
	}
}

func TestFactoryCreateIssueScanDraftPRRequiresGitHubTokenBeforeRuntime(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	err := routeAndDispatch([]string{"factory", "create-issue-scan-draft-pr", "--human", "Michael", "--run", "run_issue_001", "--request", "evt_issue_001"})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("expected missing GITHUB_TOKEN error, got %v", err)
	}
}

// TestFactoryCreatePRRequiresRequest asserts create-pr validates --request
// before opening a store or calling GitHub.
func TestFactoryCreatePRRequiresRequest(t *testing.T) {
	err := cmdFactory([]string{"create-pr"})
	if err == nil || !strings.Contains(err.Error(), "request") {
		t.Fatalf("expected missing --request error, got %v", err)
	}
}

type fakeIssueScanLister struct {
	issues map[string][]hive.GitHubIssueCandidate
}

func (l fakeIssueScanLister) ListIssues(_ context.Context, repo string, _ int, _ []string) ([]hive.GitHubIssueCandidate, error) {
	return append([]hive.GitHubIssueCandidate(nil), l.issues[repo]...), nil
}

type countingIssueScanLister struct {
	mu     sync.Mutex
	once   sync.Once
	first  chan struct{}
	calls  int
	issues map[string][]hive.GitHubIssueCandidate
}

func (l *countingIssueScanLister) ListIssues(_ context.Context, repo string, _ int, _ []string) ([]hive.GitHubIssueCandidate, error) {
	l.mu.Lock()
	l.calls++
	l.once.Do(func() { close(l.first) })
	l.mu.Unlock()
	return append([]hive.GitHubIssueCandidate(nil), l.issues[repo]...), nil
}

func (l *countingIssueScanLister) Calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}
