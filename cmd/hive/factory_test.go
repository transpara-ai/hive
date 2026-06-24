package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
	hiveregistry "github.com/transpara-ai/hive/pkg/registry"
)

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

func TestFactoryRecordIssueScanReviewRequiresHumanBeforeFileRead(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "record-issue-scan-review", "--run", "run_issue_001", "--review-file", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
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
		LifecycleVersion: "civilization_issue_to_human_ready_pr_v0.4",
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
		LifecycleVersion:          "civilization_issue_to_human_ready_pr_v0.4",
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
		LifecycleVersion:            "civilization_issue_to_human_ready_pr_v0.4",
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
		LifecycleVersion:     "civilization_issue_to_human_ready_pr_v0.4",
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
		LifecycleVersion: "civilization_issue_to_human_ready_pr_v0.4",
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
	rt, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
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
				Labels: []string{"civilization"},
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
	rt, fc, err := openFactoryRuntime(ctx, "", "Michael", ".", "")
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
